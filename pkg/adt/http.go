package adt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// httpTraceEnabled reports whether the VSP_HTTP_TRACE env var requests raw
// HTTP request/response dumps to stderr. Diagnostic-only — never leaves the
// binary switched on by default, and Authorization / Cookie values are
// redacted so the dump is safe to paste.
func httpTraceEnabled() bool {
	v := os.Getenv("VSP_HTTP_TRACE")
	return v == "1" || strings.EqualFold(v, "true")
}

const httpTraceBodyLimit = 4096

var (
	traceOutMu sync.Mutex
	traceOut   io.Writer
)

// traceWriter returns where HTTP trace lines go: the file named by
// VSP_TRACE_LOG (appended, created on first use), otherwise stderr. An MCP
// server's stderr is rarely visible, so the file is what makes the trace
// readable there.
func traceWriter() io.Writer {
	traceOutMu.Lock()
	defer traceOutMu.Unlock()
	if traceOut != nil {
		return traceOut
	}
	if path := strings.TrimSpace(os.Getenv("VSP_TRACE_LOG")); path != "" {
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			traceOut = f
			return traceOut
		}
	}
	traceOut = os.Stderr
	return traceOut
}

func traceHTTPRequest(req *http.Request, body []byte) {
	if !httpTraceEnabled() {
		return
	}
	w := traceWriter()
	fmt.Fprintf(w, "\n>>> HTTP %s %s %s\n", time.Now().UTC().Format(time.RFC3339Nano), req.Method, req.URL.String())
	for k, vs := range req.Header {
		for _, v := range vs {
			if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "Cookie") {
				v = "[REDACTED]"
			}
			fmt.Fprintf(w, ">>> %s: %s\n", k, v)
		}
	}
	if len(body) > 0 {
		trunc := body
		if len(trunc) > httpTraceBodyLimit {
			trunc = trunc[:httpTraceBodyLimit]
		}
		fmt.Fprintf(w, ">>> body (%d bytes):\n%s\n", len(body), string(trunc))
		if len(body) > httpTraceBodyLimit {
			fmt.Fprintf(w, ">>> ... (truncated)\n")
		}
	}
}

func traceHTTPResponse(resp *http.Response, body []byte) {
	if !httpTraceEnabled() || resp == nil {
		return
	}
	w := traceWriter()
	fmt.Fprintf(w, "<<< HTTP %d %s\n", resp.StatusCode, resp.Status)
	for k, vs := range resp.Header {
		for _, v := range vs {
			if strings.EqualFold(k, "Set-Cookie") {
				if i := strings.Index(v, "="); i > 0 {
					v = v[:i] + "=[REDACTED]"
				}
			}
			fmt.Fprintf(w, "<<< %s: %s\n", k, v)
		}
	}
	if len(body) > 0 {
		trunc := body
		if len(trunc) > httpTraceBodyLimit {
			trunc = trunc[:httpTraceBodyLimit]
		}
		fmt.Fprintf(w, "<<< body (%d bytes):\n%s\n", len(body), string(trunc))
		if len(body) > httpTraceBodyLimit {
			fmt.Fprintf(w, "<<< ... (truncated)\n")
		}
	}
	fmt.Fprintln(w)
}

// HTTPDoer is an interface for executing HTTP requests.
// This abstraction allows for easy testing with mock implementations.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Transport handles HTTP communication with SAP ADT REST API.
// It manages CSRF tokens, sessions, and authentication automatically.
type Transport struct {
	config     *Config
	httpClient HTTPDoer
	cache      *responseCache

	// CSRF token management
	csrfToken string
	csrfMu    sync.RWMutex

	// Session management
	sessionID string
	sessionMu sync.RWMutex

	// Cookie access protection: guards config.Cookies against concurrent
	// read (Request/retryRequest) and write (callReauthFunc) access.
	cookiesMu sync.RWMutex

	// Re-auth stampede protection: prevents concurrent 401 handlers
	// from triggering simultaneous SAML dances.
	reauthMu   sync.Mutex
	lastReauth time.Time
}

// NewTransport creates a new Transport with the given configuration.
func NewTransport(cfg *Config) *Transport {
	return NewTransportWithClient(cfg, cfg.NewHTTPClient())
}

// NewTransportWithClient creates a new Transport with a custom HTTP client.
// This is useful for testing with mock HTTP clients.
func NewTransportWithClient(cfg *Config, client HTTPDoer) *Transport {
	applyProxyContextIDGuardEnv(cfg)
	t := &Transport{
		config:     cfg,
		httpClient: client,
	}
	if cfg.Cache {
		t.cache = newResponseCache(cfg.CacheStore, cfg.CacheTTL)
	}
	return t
}

// RequestOptions contains options for an HTTP request.
type RequestOptions struct {
	Method      string
	Headers     map[string]string
	Query       url.Values
	Body        []byte
	ContentType string
	Accept      string

	// OverrideLanguage overrides the global session language for this request.
	// When set, the sap-language query parameter is set to this value instead
	// of the configured default. Used by i18n tools to read/write texts in
	// specific languages without changing the global session language.
	OverrideLanguage string

	// Stateful forces this request to use stateful session mode regardless
	// of the global default. This is required for lock→write→unlock sequences
	// where the lock handle is bound to a specific server-side session.
	// When set, X-sap-adt-sessiontype header is set to "stateful" for this request.
	Stateful bool

	// FreshContext asks for a brand-new stateful context for this request when
	// Config.ProxyContextIDGuard is on. LOCK sets it: a lock→write→unlock chain
	// that runs inside a context another chain already used — the proxy keeps
	// injecting the same live context — writes its inactive version but the
	// object never reaches the activation worklist, and the activation that
	// follows is refused with activationExecuted="false" and no message. The
	// empty "sap-contextid=" cookie makes SAP open a new context, and the proxy
	// re-learns it from the response.
	FreshContext bool

	// ReleaseContext lets the proxy inject its stored stateful context into a
	// stateless request when Config.ProxyContextIDGuard is on — the one case
	// where the guard cookie is deliberately left off. A stateless request
	// ends the context it arrives in, which is how a finished lock chain's
	// context is retired instead of lingering until the session timeout.
	ReleaseContext bool
}

// Response wraps an HTTP response with convenience methods.
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// Request performs an HTTP request to the ADT API, through the response
// cache when one is configured.
func (t *Transport) Request(ctx context.Context, path string, opts *RequestOptions) (*Response, error) {
	if opts == nil {
		opts = &RequestOptions{}
	}
	if opts.Method == "" {
		opts.Method = http.MethodGet
	}
	if t.cache == nil {
		return t.request(ctx, path, opts)
	}
	if !cacheable(path, opts) {
		resp, err := t.request(ctx, path, opts)
		if isModifyingMethod(opts.Method) && !strings.HasPrefix(path, "/sap/bc/adt/datapreview/") {
			t.cache.invalidate()
		}
		return resp, err
	}
	key, err := t.buildURL(path, opts.Query, opts.OverrideLanguage)
	if err != nil {
		return nil, fmt.Errorf("building URL: %w", err)
	}
	key += "\x00" + opts.Method + "\x00" + opts.Accept + "\x00" + fmt.Sprint(opts.Headers) + "\x00" + string(opts.Body)
	if resp, ok := t.cache.get(key); ok {
		return resp, nil
	}
	resp, err := t.request(ctx, path, opts)
	if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		t.cache.put(key, resp)
	}
	return resp, err
}

func (t *Transport) request(ctx context.Context, path string, opts *RequestOptions) (*Response, error) {
	// Build URL
	reqURL, err := t.buildURL(path, opts.Query, opts.OverrideLanguage)
	if err != nil {
		return nil, fmt.Errorf("building URL: %w", err)
	}
	if LogOutput != nil {
		detail := ""
		if strings.HasPrefix(path, "/sap/bc/adt/datapreview/") {
			if m := fromTable.FindStringSubmatch(string(opts.Body)); m != nil {
				detail = "  FROM " + strings.ToUpper(m[1])
			}
		}
		fmt.Fprintf(LogOutput, "[adt] %s %s%s\n", opts.Method, path, detail)
	}

	// Create request
	var bodyReader io.Reader
	if opts.Body != nil {
		bodyReader = bytes.NewReader(opts.Body)
	}

	req, err := http.NewRequestWithContext(ctx, opts.Method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Set authentication - either basic auth or cookies
	if t.config.HasBasicAuth() {
		req.SetBasicAuth(t.config.Username, t.config.Password)
	}

	// Set default headers
	t.setDefaultHeaders(req, opts)

	// Add CSRF token for modifying requests
	if isModifyingMethod(opts.Method) {
		token := t.getCSRFToken()
		if token == "" {
			// Fetch CSRF token first, on the same kind of session the request
			// itself will use (issue #91).
			if err := t.fetchCSRFTokenFor(ctx, opts.Stateful); err != nil {
				return nil, fmt.Errorf("fetching CSRF token: %w", err)
			}
			token = t.getCSRFToken()
		}
		req.Header.Set("X-CSRF-Token", token)
	}

	// Attach cookies last. Fetching a token can end up re-authenticating, and a
	// token belongs to the session it was minted for: pairing a fresh one with
	// the cookies of the session it replaced is exactly the mismatch the server
	// rejects as a CSRF failure.
	t.addCookies(req)
	// The proxy guard goes on after the cookies: it only steps in when no
	// cookie of our own is on the request.
	t.applyProxyContextIDGuard(req, opts)

	// Execute request
	traceHTTPRequest(req, opts.Body)
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	traceHTTPResponse(resp, body)
	t.adoptServerCookies(resp)

	// The same expiry reaches a plain read as a successful-looking response that
	// was in fact served by the identity provider. Nothing downstream would
	// recognise the logon page it carries, so catch it here by origin.
	if resp.StatusCode < 400 && t.canReauth() && t.redirectedAwayFromSAP(resp) {
		t.setCSRFToken("")
		t.setSessionID("")
		if err := t.callReauthFunc(ctx); err != nil {
			return nil, fmt.Errorf("re-authenticating after an SSO redirect on %s: %w", path, err)
		}
		return t.retryRequest(ctx, path, opts)
	}

	// Handle CSRF token refresh on 403
	if resp.StatusCode == http.StatusForbidden && isModifyingMethod(opts.Method) {
		// Try to refresh CSRF token and retry once. The refresh has to stay on
		// the request's own session kind: for a stateful write it lands between
		// the failed attempt and the retry, and an unmarked probe there retires
		// the session the lock handle belongs to (issue #91).
		if err := t.fetchCSRFTokenFor(ctx, opts.Stateful); err != nil {
			return nil, fmt.Errorf("refreshing CSRF token: %w", err)
		}

		// Retry the request
		return t.retryRequest(ctx, path, opts)
	}

	// Store CSRF token from response
	if token := resp.Header.Get("X-CSRF-Token"); token != "" && token != "Required" {
		t.setCSRFToken(token)
	}

	// Store session ID
	if sessionID := t.extractSessionID(resp); sessionID != "" {
		t.setSessionID(sessionID)
	}

	// Check for error status codes
	if resp.StatusCode >= 400 {
		apiErr := &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
			Path:       path,
		}

		// Handle session timeout - refresh session and retry once
		if apiErr.IsSessionExpired() {
			// Clear cached CSRF token and session ID
			t.setCSRFToken("")
			t.setSessionID("")
			// Drop the stale sap-contextid / SAP_SESSIONID cookies too: the
			// stateless activation that ended the context on the SAP side left
			// them in the jar, and every retry that re-sends them is answered
			// with ICMENOSESSION again — including the /core/discovery probe
			// that is supposed to open the fresh session.
			t.resetCookieJar()
			// Fetch new CSRF token (this establishes a new session)
			if err := t.fetchCSRFTokenFor(ctx, opts.Stateful); err != nil {
				return nil, fmt.Errorf("refreshing session after timeout: %w", err)
			}
			// Retry the request
			return t.retryRequest(ctx, path, opts)
		}

		// Handle 401 Unauthorized - re-authenticate and retry once.
		// This happens after idle periods when the SAP session expires.
		// We preserve apiErr so the original path/body is not lost if re-auth itself fails.
		if resp.StatusCode == http.StatusUnauthorized {
			t.setCSRFToken("")
			t.setSessionID("")

			if !t.config.HasBasicAuth() && t.config.ReauthFunc != nil {
				// Cookie/SAML auth: re-run full auth dance to get fresh cookies.
				if err := t.callReauthFunc(ctx); err != nil {
					return nil, fmt.Errorf("re-authenticating after 401 on %s: %w (original error: %v)", path, err, apiErr)
				}
			} else {
				// Basic auth: just refresh CSRF token.
				if err := t.fetchCSRFTokenFor(ctx, opts.Stateful); err != nil {
					return nil, fmt.Errorf("re-authenticating after 401 on %s: %w (original error: %v)", path, err, apiErr)
				}
			}
			return t.retryRequest(ctx, path, opts)
		}

		return nil, apiErr
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       body,
	}, nil
}

// retryRequest retries a request after CSRF token refresh.
func (t *Transport) retryRequest(ctx context.Context, path string, opts *RequestOptions) (*Response, error) {
	reqURL, err := t.buildURL(path, opts.Query, opts.OverrideLanguage)
	if err != nil {
		return nil, fmt.Errorf("building URL: %w", err)
	}

	var bodyReader io.Reader
	if opts.Body != nil {
		bodyReader = bytes.NewReader(opts.Body)
	}

	req, err := http.NewRequestWithContext(ctx, opts.Method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Set authentication
	if t.config.HasBasicAuth() {
		req.SetBasicAuth(t.config.Username, t.config.Password)
	}
	t.setDefaultHeaders(req, opts)
	t.addCookies(req)
	req.Header.Set("X-CSRF-Token", t.getCSRFToken())

	// Ensure session type header is set for retry
	if t.config.SessionType == SessionStateful {
		req.Header.Set("X-sap-adt-sessiontype", "stateful")
	}
	t.applyProxyContextIDGuard(req, opts)

	traceHTTPRequest(req, opts.Body)
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing retry request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	traceHTTPResponse(resp, body)

	if resp.StatusCode >= 400 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
			Path:       path,
		}
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       body,
	}, nil
}

// fetchCSRFToken retrieves a CSRF token from the server.
//
// HEAD on /core/discovery is the fast path (milliseconds, against tens of seconds
// for a GET on /discovery on a slow system). Older releases — BASIS 740, ECC EhP7 —
// answer that HEAD with 400 and no token at all, which used to make vsp unusable
// against them, so a missing token falls back to GET.
//
// A 401 is reported at once: no method will fix a wrong password. A **403 is
// not** — some systems refuse the HEAD and answer the GET perfectly well, so
// short-circuiting there reintroduced exactly the unusability the fallback
// exists to prevent. Let GET have its turn; if it is also forbidden, the error
// below says so.
func (t *Transport) fetchCSRFToken(ctx context.Context) error {
	return t.fetchCSRFTokenFor(ctx, false)
}

// fetchCSRFTokenFor fetches a token on behalf of a request whose statefulness
// is known.
//
// A token fetch triggered from inside Request — no cached token, a 403 refresh,
// a session-expiry retry — lands in the middle of whatever that request is
// doing. If that request is the write that consumes a lock handle, a probe sent
// without the stateful marker is answered on a different ADT context and the
// stateful one is retired: the retry then presents a handle whose session has
// just been thrown away (issue #91). So the probe inherits the in-flight
// request's statefulness rather than only the client-wide default.
func (t *Transport) fetchCSRFTokenFor(ctx context.Context, stateful bool) error {
	return t.fetchCSRFTokenWithReauth(ctx, true, stateful)
}

// fetchCSRFTokenWithReauth fetches a token, optionally recovering an expired
// session on the way.
//
// allowReauth exists to break a cycle: re-authenticating ends with a token
// fetch of its own, and that fetch must not start another re-authentication.
func (t *Transport) fetchCSRFTokenWithReauth(ctx context.Context, allowReauth bool, stateful bool) error {
	token, status, redirected, err := t.probeCSRFToken(ctx, http.MethodHead, stateful)
	if err != nil {
		return err
	}
	if !isCSRFToken(token) {
		if status == http.StatusUnauthorized && !t.canReauth() {
			return fmt.Errorf("authentication failed (401): check username/password")
		}
		var getStatus int
		var getRedirected bool
		token, getStatus, getRedirected, err = t.probeCSRFToken(ctx, http.MethodGet, stateful)
		if err != nil {
			return err
		}
		if !isCSRFToken(token) {
			// An expired SSO session rarely announces itself as a 401. ICF sends
			// the request on to the identity provider, the redirect chain is
			// followed, and back comes a logon page under a perfectly ordinary
			// 200 — with no CSRF token in it, because it is not ADT answering.
			// A live ADT session always yields a token, so its absence here is
			// the signal, and a hop to a foreign host is the confirmation.
			if allowReauth && t.canReauth() && getStatus != http.StatusForbidden {
				reason := fmt.Sprintf("no CSRF token (HEAD %d, GET %d)", status, getStatus)
				if redirected || getRedirected {
					reason = "the identity provider answered instead of SAP"
				}
				if t.config.Verbose {
					fmt.Fprintf(os.Stderr, "[AUTH] session looks expired — %s; re-authenticating\n", reason)
				}
				if err := t.callReauthFunc(ctx); err != nil {
					return fmt.Errorf("re-authenticating (%s): %w", reason, err)
				}
				// callReauthFunc ends by fetching a token with the new session.
				if isCSRFToken(t.getCSRFToken()) {
					return nil
				}
				return t.fetchCSRFTokenWithReauth(ctx, false, stateful)
			}
			switch getStatus {
			case http.StatusUnauthorized:
				return fmt.Errorf("authentication failed (401): check username/password")
			case http.StatusForbidden:
				return fmt.Errorf("access forbidden (403): check user authorizations")
			default:
				return fmt.Errorf("no CSRF token in response (HEAD %d, GET %d)", status, getStatus)
			}
		}
	}

	t.setCSRFToken(token)
	return nil
}

// probeCSRFToken asks /core/discovery for a token with the given method and
// returns the token (empty when the server did not supply one), the status, and
// whether the answer came from somewhere other than the SAP host.
func (t *Transport) probeCSRFToken(ctx context.Context, method string, stateful bool) (token string, status int, redirected bool, err error) {
	reqURL, err := t.buildURL("/sap/bc/adt/core/discovery", nil)
	if err != nil {
		return "", 0, false, fmt.Errorf("building URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return "", 0, false, fmt.Errorf("creating request: %w", err)
	}

	if t.config.HasBasicAuth() {
		req.SetBasicAuth(t.config.Username, t.config.Password)
	}
	t.addCookies(req)
	req.Header.Set("X-CSRF-Token", "fetch")
	req.Header.Set("Accept", "*/*")
	// Only ever *add* the stateful marker; never stamp an explicit "stateless"
	// here. The keep-alive ping goes through this same probe (Ping ->
	// fetchCSRFToken), and an explicitly stateless keep-alive would retire the
	// session on a timer — the very failure this is guarding against.
	if stateful || t.config.SessionType == SessionStateful {
		req.Header.Set("X-sap-adt-sessiontype", "stateful")
	}

	// Session-holding proxy chain: open a fresh stateful context with an
	// empty contextid so the chain replaces its (possibly dead) stored
	// context with the live one from this response. Verified against SAP
	// BAS: HEAD + stateful + "Cookie: sap-contextid=" heals ICMENOSESSION
	// for all follow-up requests; without the stateful header the chain
	// keeps the dead one.
	if t.config.ProxyContextIDGuard && !t.hasJarCookies(req) && req.Header.Get("Cookie") == "" {
		req.Header.Set("X-sap-adt-sessiontype", "stateful")
		req.Header.Set("Cookie", "sap-contextid=")
	}

	traceHTTPRequest(req, nil)
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", 0, false, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()
	// Drain the body so the connection can be reused.
	_, _ = io.Copy(io.Discard, resp.Body)
	traceHTTPResponse(resp, nil)
	t.adoptServerCookies(resp)

	return resp.Header.Get("X-CSRF-Token"), resp.StatusCode, t.redirectedAwayFromSAP(resp), nil
}

// redirectedAwayFromSAP reports whether a response was ultimately served by
// some host other than the SAP system — which, for a request that asked for an
// ADT resource, means an identity provider answered instead.
func (t *Transport) redirectedAwayFromSAP(resp *http.Response) bool {
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return false
	}
	base, err := url.Parse(t.config.BaseURL)
	if err != nil || base.Host == "" {
		return false
	}
	return !strings.EqualFold(resp.Request.URL.Host, base.Host)
}

// canReauth reports whether a fresh session can be obtained without asking
// anyone for a password — that is, whether some browser or SSO flow is standing
// by to produce one.
func (t *Transport) canReauth() bool {
	return !t.config.HasBasicAuth() && t.config.ReauthFunc != nil
}

// isCSRFToken reports whether the header value is an actual token rather than the
// server's "Required" placeholder.
func isCSRFToken(v string) bool { return v != "" && v != "Required" }

// buildURL constructs the full URL for an API request.
// overrideLang, if non-empty, overrides the configured session language for
// this single request (used by i18n tools to read/write texts per-language).
func (t *Transport) buildURL(path string, query url.Values, overrideLang ...string) (string, error) {
	base := strings.TrimSuffix(t.config.BaseURL, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	u, err := url.Parse(base + path)
	if err != nil {
		return "", err
	}

	// Merge query parameters
	q := u.Query()
	if t.config.Client != "" {
		q.Set("sap-client", t.config.Client)
	}

	// Use override language if provided, otherwise fall back to config
	lang := t.config.Language
	if len(overrideLang) > 0 && overrideLang[0] != "" {
		lang = overrideLang[0]
	}
	if lang != "" {
		q.Set("sap-language", lang)
	}

	for k, v := range query {
		for _, val := range v {
			q.Add(k, val)
		}
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// setDefaultHeaders sets default headers on a request.
func (t *Transport) setDefaultHeaders(req *http.Request, opts *RequestOptions) {
	// Set Accept header - SAP ADT requires */* for many endpoints
	accept := opts.Accept
	if accept == "" {
		accept = "*/*"
	}
	req.Header.Set("Accept", accept)

	// Set Content-Type for requests with body
	if opts.Body != nil {
		contentType := opts.ContentType
		if contentType == "" {
			contentType = "application/xml"
		}
		req.Header.Set("Content-Type", contentType)
	}

	// Set custom headers
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}

	// Set session header: per-request Stateful flag overrides global default.
	// Lock→write→unlock sequences require stateful mode to maintain session
	// affinity for lock handles (issue #88).
	if opts.Stateful || t.config.SessionType == SessionStateful {
		req.Header.Set("X-sap-adt-sessiontype", "stateful")
	} else {
		req.Header.Set("X-sap-adt-sessiontype", "stateless")
	}
}

// extractSessionID extracts the session ID from response cookies.
func (t *Transport) extractSessionID(resp *http.Response) string {
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "sap-contextid" || cookie.Name == "SAP_SESSIONID" {
			return cookie.Value
		}
	}
	return ""
}

// applyProxyContextIDGuardEnv switches Config.ProxyContextIDGuard on when
// SAP_PROXY_CONTEXTID_GUARD=true is set in the environment.
func applyProxyContextIDGuardEnv(cfg *Config) {
	if cfg == nil || cfg.ProxyContextIDGuard {
		return
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("SAP_PROXY_CONTEXTID_GUARD")), "true") {
		cfg.ProxyContextIDGuard = true
	}
}

// hasJarCookies reports whether the cookie jar holds cookies for the
// request URL (i.e. we talk to SAP directly and manage the session
// ourselves). Behind a session-holding proxy chain the jar stays empty for
// sap-contextid: the chain absorbs the live Set-Cookie and only deletion
// cookies (which the jar does not keep) get through.
func (t *Transport) hasJarCookies(req *http.Request) bool {
	client, ok := t.httpClient.(*http.Client)
	if !ok || client.Jar == nil || req.URL == nil {
		return false
	}
	return len(client.Jar.Cookies(req.URL)) > 0
}

// applyProxyContextIDGuard sends an explicit empty "sap-contextid=" cookie
// on stateless requests when Config.ProxyContextIDGuard is enabled and no
// cookie (jar or user-provided) is present. The ICM honours the first
// sap-contextid in the header, so the empty value wins over whatever the
// session-holding chain appends, and the stateless request no longer ends
// the stateful context the chain keeps — which would leave every following
// request in ICMENOSESSION. Stateful requests are left alone so the chain
// keeps injecting the live context that lock handles are bound to — except
// when the request asks for a fresh context (RequestOptions.FreshContext),
// where the same empty cookie makes SAP open a new one and the chain
// re-learns it. RequestOptions.ReleaseContext leaves a stateless request
// without the cookie on purpose, so the injected context is ended.
func (t *Transport) applyProxyContextIDGuard(req *http.Request, opts *RequestOptions) {
	if !t.config.ProxyContextIDGuard {
		return
	}
	if req.Header.Get("Cookie") != "" || t.hasJarCookies(req) {
		return
	}
	if opts != nil && opts.ReleaseContext {
		return
	}
	if req.Header.Get("X-sap-adt-sessiontype") == "stateful" && (opts == nil || !opts.FreshContext) {
		return
	}
	req.Header.Set("Cookie", "sap-contextid=")
}

// ReleaseProxyContext retires the stateful context a session-holding proxy
// chain currently injects, once a lock chain is finished with it. Without
// the guard there is nothing to retire and the call is a no-op. The request
// is a cheap stateless HEAD that carries no guard cookie, so the chain
// injects its stored context and SAP ends it (verified: SM04 shows no
// lingering ADT sessions afterwards). Failures are ignored: an already-dead
// context answers ICMENOSESSION, which is the state this call wants anyway,
// and the next LOCK opens a fresh context regardless.
func (t *Transport) ReleaseProxyContext(ctx context.Context) {
	if !t.config.ProxyContextIDGuard {
		return
	}
	reqURL, err := t.buildURL("/sap/bc/adt/core/discovery", nil)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, reqURL, nil)
	if err != nil {
		return
	}
	if t.config.HasBasicAuth() {
		req.SetBasicAuth(t.config.Username, t.config.Password)
	}
	t.addCookies(req)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("X-sap-adt-sessiontype", "stateless")
	traceHTTPRequest(req, nil)
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	traceHTTPResponse(resp, nil)
}

// CSRF token accessors with mutex protection
func (t *Transport) getCSRFToken() string {
	t.csrfMu.RLock()
	defer t.csrfMu.RUnlock()
	return t.csrfToken
}

func (t *Transport) setCSRFToken(token string) {
	t.csrfMu.Lock()
	defer t.csrfMu.Unlock()
	t.csrfToken = token
}

// Session ID accessors with mutex protection
func (t *Transport) getSessionID() string {
	t.sessionMu.RLock()
	defer t.sessionMu.RUnlock()
	return t.sessionID
}

func (t *Transport) setSessionID(id string) {
	t.sessionMu.Lock()
	defer t.sessionMu.Unlock()
	t.sessionID = id
}

// isModifyingMethod returns true for HTTP methods that modify server state.
func isModifyingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
		return true
	default:
		return false
	}
}

// APIError represents an error from the ADT API.
type APIError struct {
	StatusCode int
	Message    string
	Path       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("ADT API error: status %d at %s: %s", e.StatusCode, e.Path, e.Message)
}

// IsNotFound returns true if the error is a 404 Not Found error.
func (e *APIError) IsNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}

// IsSessionExpired returns true if the error indicates session timeout.
// SAP returns 400 with ICMENOSESSION or "Session Timed Out" when session expires.
func (e *APIError) IsSessionExpired() bool {
	if e.StatusCode != http.StatusBadRequest {
		return false
	}
	msg := strings.ToLower(e.Message)
	return strings.Contains(msg, "icmenosession") ||
		strings.Contains(msg, "session timed out") ||
		strings.Contains(msg, "session no longer exists") ||
		strings.Contains(msg, "session not found")
}

// IsNotFoundError checks if an error is an API 404 Not Found error.
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.IsNotFound()
	}
	return false
}

// IsSessionExpiredError checks if an error indicates SAP session timeout.
func IsSessionExpiredError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.IsSessionExpired()
	}
	return false
}

// Ping sends a lightweight HEAD request to /sap/bc/adt/core/discovery to keep the session alive.
// It refreshes the CSRF token as a side effect.
func (t *Transport) Ping(ctx context.Context) error {
	return t.fetchCSRFToken(ctx)
}

// CheckSession reports whether this client has a usable, authenticated ADT
// session, and returns why not when it does not.
//
// It is the CSRF token fetch, deliberately, rather than a status code or a
// query: a live ADT session always yields a token, and an expired SSO session
// answers 200 with a logon page that carries none. The whole of that detection
// already lives in fetchCSRFToken, and a second implementation beside it would
// be a second thing to keep right.
func (c *Client) CheckSession(ctx context.Context) error {
	return c.transport.Ping(ctx)
}

// reauthCooldown prevents concurrent 401 handlers from triggering simultaneous
// SAML dances. If a re-auth completed within this window, skip the duplicate.
const reauthCooldown = 5 * time.Second

// reauthTimeout caps the total time spent in a single re-auth attempt (SAML dance +
// CSRF fetch). Prevents concurrent 401 handlers from blocking indefinitely when the
// re-auth holder is stuck on a slow or unresponsive IdP.
const reauthTimeout = 30 * time.Second

// callReauthFunc invokes config.ReauthFunc with stampede protection.
// Multiple goroutines hitting 401 simultaneously will serialize through the mutex;
// the first one performs the re-auth, subsequent ones within the cooldown window skip it.
func (t *Transport) callReauthFunc(ctx context.Context) error {
	t.reauthMu.Lock()
	defer t.reauthMu.Unlock()

	// Another goroutine already re-authed while we waited for the lock.
	if !t.lastReauth.IsZero() && time.Since(t.lastReauth) < reauthCooldown {
		return nil
	}

	// Apply a timeout so the mutex is not held indefinitely during network I/O.
	timeout := t.config.ReauthTimeout
	if timeout <= 0 {
		timeout = reauthTimeout
	}
	reauthCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cookies, err := t.config.ReauthFunc(reauthCtx)
	if err != nil {
		return err
	}

	t.cookiesMu.Lock()
	t.config.Cookies = cookies
	t.cookiesMu.Unlock()
	// The jar still holds what the expired session's server set — including its
	// own SAP_SESSIONID, which would ride along beside the new one and leave the
	// server to pick between them.
	t.resetCookieJar()

	// Fetch CSRF token with the new cookies.
	// Set lastReauth only after CSRF succeeds — if it fails, the next
	// goroutine should retry rather than hitting the cooldown skip.
	// Re-auth establishes a brand-new session; there is no lock window to
	// preserve across it, so this stays on the client-wide default.
	if err := t.fetchCSRFTokenWithReauth(reauthCtx, false, false); err != nil {
		return err
	}
	t.lastReauth = time.Now()
	return nil
}

// adoptServerCookies takes over any cookie the server just reissued that this
// client also holds explicitly.
//
// A logon ticket outlives the session it opened. When the session lapses while
// the ticket is still good, the next request authenticates on the ticket, and
// SAP quietly opens a new session and returns its id in Set-Cookie. The jar
// keeps that one; config.Cookies still holds the lapsed one, and both go out on
// the following request under the same name. The server honours one of them and
// the CSRF token belongs to the other, so a perfectly authenticated client is
// told its token is invalid — a failure that names neither the session nor the
// ticket and points at the wrong thing entirely.
//
// Taking the server's value keeps one id in play instead of two.
func (t *Transport) adoptServerCookies(resp *http.Response) {
	if resp == nil {
		return
	}
	fresh := resp.Cookies()
	if len(fresh) == 0 {
		return
	}

	t.cookiesMu.Lock()
	defer t.cookiesMu.Unlock()
	for _, c := range fresh {
		if c.Value == "" {
			continue
		}
		if held, ok := t.config.Cookies[c.Name]; ok && held != c.Value {
			t.config.Cookies[c.Name] = c.Value
			if t.config.Verbose {
				fmt.Fprintf(os.Stderr, "[AUTH] server reissued %s — using the new one\n", c.Name)
			}
		}
	}
}

// resetCookieJar discards cookies accumulated under a previous session.
func (t *Transport) resetCookieJar() {
	client, ok := t.httpClient.(*http.Client)
	if !ok || client.Jar == nil {
		return
	}
	if jar, err := cookiejar.New(nil); err == nil {
		client.Jar = jar
	}
}

// CurrentCookies returns a copy of the session this client is using now.
//
// The session is not the one it started with: an expiry replaces the whole map,
// so anything that took a snapshot at startup is holding a dead one. A caller
// that needs to authenticate elsewhere — opening a WebSocket, say — has to ask
// at the moment it connects rather than remember.
func (t *Transport) CurrentCookies() map[string]string {
	t.cookiesMu.RLock()
	defer t.cookiesMu.RUnlock()
	if len(t.config.Cookies) == 0 {
		return nil
	}
	out := make(map[string]string, len(t.config.Cookies))
	for name, value := range t.config.Cookies {
		out[name] = value
	}
	return out
}

// SetCookies replaces the session this client authenticates with. This is what
// a re-authentication does, and it is exported so a caller that obtained a
// session some other way can hand it over without rebuilding the client.
func (t *Transport) SetCookies(cookies map[string]string) {
	t.cookiesMu.Lock()
	defer t.cookiesMu.Unlock()
	t.config.Cookies = cookies
}

// addCookies adds user-provided cookies to a request under cookiesMu read lock.
func (t *Transport) addCookies(req *http.Request) {
	t.cookiesMu.RLock()
	defer t.cookiesMu.RUnlock()
	for name, value := range t.config.Cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}
}
