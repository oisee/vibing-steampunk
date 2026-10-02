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
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"
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

	// locks is the owning client's lock window. While it holds a handle,
	// stateless requests are kept out of the stateful context (see do).
	locks *lockWindow

	// contextGate admits one request at a time into the stateful context, and
	// keeps a stateless request that is allowed to end the context from
	// racing one that is using it. contextInFlight counts the stateful
	// requests under way or waiting (see do). Set by NewTransportWithClient.
	contextGate     contextGate
	contextInFlight atomic.Int32

	// lockOutstanding, when set by the owning Client, reports whether that
	// client holds a lock handle. Cookie-file recovery is refused while it
	// does: reloading would replace the session the lock belongs to.
	lockOutstanding func() bool

	// identity enforces Config.Expect: nil when nothing is pinned.
	identity *identityGate
	// authGen counts changes of the credentials the transport sends (see
	// credentialsChanged); the identity pin's verdict belongs to one value.
	authGen atomic.Uint64
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
		config:      cfg,
		httpClient:  client,
		contextGate: newContextGate(),
	}
	if cfg.Cache {
		t.cache = newResponseCache(cfg.CacheStore, cfg.CacheTTL)
	}
	if cfg.Expect != nil {
		t.identity = &identityGate{pin: *cfg.Expect}
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

	// noBasicAuthRetry returns a 401 on a password logon at once, without the
	// CSRF refresh and retry: the identity preflight, which must not turn one
	// wrong password into several failed logons.
	noBasicAuthRetry bool
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
			if err := t.fetchCSRFTokenWithReauth(ctx, !t.config.ReauthReadOnly, opts.Stateful); err != nil {
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
	resp, err := t.do(req)
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
		if err := t.requireSafeReauth(opts, path, nil); err != nil {
			return nil, err
		}
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
		if err := t.fetchCSRFTokenWithReauth(ctx, !t.config.ReauthReadOnly, opts.Stateful); err != nil {
			return nil, fmt.Errorf("refreshing CSRF token: %w", err)
		}

		// Retry the request
		return t.retryRequest(ctx, path, opts)
	}

	t.rememberSession(resp)

	// Check for error status codes
	if resp.StatusCode >= 400 {
		apiErr := &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
			Path:       path,
		}

		// Handle session timeout - refresh session and retry once
		if apiErr.IsSessionExpired() {
			if err := t.requireSafeReauth(opts, path, apiErr); err != nil {
				return nil, err
			}
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
			if opts.noBasicAuthRetry && t.config.HasBasicAuth() {
				return nil, apiErr
			}
			if err := t.requireSafeReauth(opts, path, apiErr); err != nil {
				return nil, err
			}
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
	resp, err := t.do(req)
	if err != nil {
		return nil, fmt.Errorf("executing retry request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	traceHTTPResponse(resp, body)
	// A retry runs exactly when SAP has just issued a new session — after a
	// CSRF refresh, a session expiry or an SSO re-auth — so it reads back what
	// Request reads back. Without this the cookies still name the dead session
	// while the jar holds the new one, and the next write is answered with a
	// CSRF failure for a token that belongs to the other session.
	t.adoptServerCookies(resp)
	t.rememberSession(resp)

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

// rememberSession keeps the CSRF token and the session id a response carries.
// Request and retryRequest both call it, so the two paths cannot drift apart
// again.
func (t *Transport) rememberSession(resp *http.Response) {
	if token := resp.Header.Get("X-CSRF-Token"); token != "" && token != "Required" {
		t.setCSRFToken(token)
	}
	if sessionID := t.extractSessionID(resp); sessionID != "" {
		t.setSessionID(sessionID)
	}
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
	resp, err := t.do(req)
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

// requireSafeReauth keeps externally refreshed cookie files out of writes and
// lock windows. The request already left the process, so continuing it with a
// new session would turn an observable failure into an unprovable outcome.
// The session kind is the one the request was sent with: a client-wide
// stateful session makes every request stateful (see the session header in
// setDefaultHeaders), not only those that ask for it. cause, when not nil, is
// the server's answer that showed the session gone; the refusal wraps it.
func (t *Transport) requireSafeReauth(opts *RequestOptions, path string, cause error) error {
	if !t.config.ReauthReadOnly {
		return nil
	}
	method := http.MethodGet
	if opts != nil && opts.Method != "" {
		method = opts.Method
	}
	if t.lockOutstanding != nil && t.lockOutstanding() {
		return refusal(cause, "session expired on %s %s: refusing cookie-file recovery while a lock is open, because reloading would replace the session the lock belongs to; "+
			"retry after the lock is released (unlock the object, or wait for the SAP session timeout), or restart the server", method, path)
	}
	stateful := t.config.SessionType == SessionStateful || (opts != nil && opts.Stateful)
	if opts != nil && !stateful && (opts.Method == http.MethodGet || opts.Method == http.MethodHead) {
		return nil
	}
	return refusal(cause, "session expired on %s %s: refusing cookie-file recovery and replay because the remote result is unknown", method, path)
}

// refusal formats a recovery refusal, wrapping cause when there is one.
func refusal(cause error, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if cause == nil {
		return errors.New(msg)
	}
	return fmt.Errorf("%s: %w", msg, cause)
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
	// Through do, like every other request: while another chain holds a lock
	// or a stateful request is under way, the release goes without the
	// context id and leaves that chain's context alone.
	resp, err := t.do(req)
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
	if len(cookies) == 0 {
		return fmt.Errorf("re-authentication returned no cookies")
	}

	// Make the accepted source snapshot the sole authentication state before
	// asking SAP for a token. The callback has already validated its input; an
	// error before this point leaves the old session untouched.
	t.cookiesMu.Lock()
	t.config.Cookies = cloneCookies(cookies)
	// Another session now, perhaps another user's: the identity pin checks it
	// again before the work that triggered this is retried.
	t.credentialsChanged()
	t.cookiesMu.Unlock()
	t.setCSRFToken("")
	t.setSessionID("")
	// The jar still holds what the expired session's server set — including its
	// own SAP_SESSIONID, which would ride along beside the new one and leave the
	// server to pick between them.
	t.resetCookieJar()
	if t.cache != nil {
		t.cache.invalidate()
	}

	// The token fetch is part of establishing the session, not work: it is
	// not held to the identity pin (a 401 inside it would otherwise re-enter
	// this function under reauthMu). The retried request is.
	reauthCtx = context.WithValue(reauthCtx, preflightKey{}, t)

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
			// Not a credential change for the identity pin: the verified
			// system issued this cookie itself, and its verdict covers it.
			// The pin guards against operator misconfiguration, not a
			// hostile server; re-verifying here would preflight on every
			// answer of an SSO system that refreshes its cookie.
			if t.config.Verbose {
				fmt.Fprintf(os.Stderr, "[AUTH] server reissued %s — using the new one\n", c.Name)
			}
		}
	}
}

// resetCookieJar discards cookies accumulated under a previous session.
//
// The client built by Config.NewHTTPClient keeps one resettableJar for its
// lifetime, and clearing it is safe while other requests are under way.
// Assigning client.Jar instead would race every concurrent Do, which reads
// the field; that fallback is left only for a caller-supplied client with a
// jar of its own.
func (t *Transport) resetCookieJar() {
	client, ok := t.httpClient.(*http.Client)
	if !ok || client.Jar == nil {
		return
	}
	if jar, ok := client.Jar.(*resettableJar); ok {
		jar.reset()
		return
	}
	if jar, err := cookiejar.New(nil); err == nil {
		client.Jar = jar
	}
}

// resettableJar is an http.CookieJar that can be emptied while in use. The
// http.Client holds the same resettableJar throughout; reset swaps the jar
// inside it under a lock.
type resettableJar struct {
	mu    sync.RWMutex
	inner http.CookieJar
}

func newResettableJar() *resettableJar {
	jar, _ := cookiejar.New(nil) // cookiejar.New never fails with nil options
	return &resettableJar{inner: jar}
}

func (j *resettableJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.mu.RLock()
	inner := j.inner
	j.mu.RUnlock()
	inner.SetCookies(u, cookies)
}

func (j *resettableJar) Cookies(u *url.URL) []*http.Cookie {
	j.mu.RLock()
	inner := j.inner
	j.mu.RUnlock()
	return inner.Cookies(u)
}

// reset drops every cookie. A request that took the old jar just before
// reset may still set its response cookies there; they are discarded with it.
func (j *resettableJar) reset() {
	jar, _ := cookiejar.New(nil)
	j.mu.Lock()
	j.inner = jar
	j.mu.Unlock()
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
	t.config.Cookies = cloneCookies(cookies)
	t.credentialsChanged()
}

func cloneCookies(cookies map[string]string) map[string]string {
	if len(cookies) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(cookies))
	for name, value := range cookies {
		cloned[name] = value
	}
	return cloned
}

// addCookies adds user-provided cookies to a request under cookiesMu read lock.
func (t *Transport) addCookies(req *http.Request) {
	t.cookiesMu.RLock()
	defer t.cookiesMu.RUnlock()
	for name, value := range t.config.Cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}
}

// stripContextID removes a non-empty sap-contextid cookie from req.
func stripContextID(req *http.Request) {
	cookies := req.Cookies()
	kept := cookies[:0]
	stripped := false
	for _, c := range cookies {
		if c.Name == "sap-contextid" && c.Value != "" {
			stripped = true
			continue
		}
		kept = append(kept, c)
	}
	if !stripped {
		return
	}
	req.Header.Del("Cookie")
	for _, c := range kept {
		req.AddCookie(c)
	}
}

// do sends a request, keeping concurrent callers of one client from breaking
// each other's lock chains. Two things end or break the stateful ADT context a
// lock handle is bound to, and both are the ordinary work of another caller --
// a second agent sharing this client:
//
//   - A stateless request ends the context it arrives in, and the jar puts
//     the context's sap-contextid on every request. One sent between LOCK and
//     the write turns the write into 423 ExceptionResourceInvalidLockHandle.
//     So while a lock is outstanding, or a stateful request is under way, a
//     stateless request goes without the jar: it carries the session's other
//     cookies but not sap-contextid, and the cookies its response sets are
//     not learned. The stateful context is neither ended nor replaced.
//   - The ICM serves a stateful context one request at a time and answers a
//     second concurrent one with 400. The session recovery that follows drops
//     the cookies and orphans the context together with the enqueue it holds,
//     which then refuses every later LOCK on the object ("already editing")
//     until the session times out. So requests into the context -- stateful
//     ones, and unmarked ones such as the CSRF probe -- go one at a time.
//     Chains still interleave: one context holds several locks.
//
// A stateless request outside any lock window still goes into the context and
// ends it, as before -- that is how a finished chain's context is retired. It
// holds the gate shared while it does, so a LOCK cannot open a window in the
// context it is about to end; stateless requests still run side by side. A
// stateless request never waits for the gate: when a request into the context
// holds it or waits for it, the stateless one goes isolated at once. Requests
// into the context wait their turn only as long as their own context lasts.
func (t *Transport) do(req *http.Request) (*http.Response, error) {
	// The identity pin, before anything leaves: the first request runs the
	// preflight, and after a mismatch nothing is sent at all.
	if err := t.admit(req); err != nil {
		return nil, err
	}
	if req.Header.Get("X-sap-adt-sessiontype") == "stateless" {
		if t.contextGate.tryShared() {
			if t.contextInFlight.Load() == 0 && (t.locks == nil || !t.locks.present()) {
				defer t.contextGate.releaseShared()
				return t.send(req)
			}
			t.contextGate.releaseShared()
		}
		// Cookies supplied with the configuration (browser or SAML logon)
		// were put on the request already, sap-contextid among them; it
		// would end the context just the same. The empty one the proxy guard
		// sends to switch the context off stays.
		stripContextID(req)
		client, ok := t.httpClient.(*http.Client)
		if !ok || client.Jar == nil {
			return t.send(req)
		}
		for _, c := range client.Jar.Cookies(req.URL) {
			if c.Name != "sap-contextid" {
				req.AddCookie(c)
			}
		}
		isolated := *client
		isolated.Jar = nil
		if callDeadlineGoverns(req.Context()) {
			isolated.Timeout = 0
		}
		return isolated.Do(req)
	}

	stateful := req.Header.Get("X-sap-adt-sessiontype") == "stateful"
	if stateful {
		t.contextInFlight.Add(1)
		defer t.contextInFlight.Add(-1)
	}
	if err := t.contextGate.lock(req.Context()); err != nil {
		return nil, &url.Error{Op: urlErrorOp(req.Method), URL: req.URL.String(), Err: err}
	}
	defer t.contextGate.unlock()
	return t.send(req)
}

// urlErrorOp names the method the way net/http does in its *url.Error.
func urlErrorOp(method string) string {
	if method == "" {
		return "Get"
	}
	return method[:1] + strings.ToLower(method[1:])
}

// contextGateSlots is the gate's weight: one slot per stateless request that
// holds it shared, all of them for a request into the context. A million
// concurrent stateless requests on one client is out of reach.
const contextGateSlots = 1 << 20

// contextGate is a readers-writer gate over a weighted semaphore. Readers
// never wait: tryShared takes one slot with TryAcquire, which fails while a
// writer holds the gate or is queued for it, so readers cannot starve a
// writer. Writers take every slot with Acquire, which queues them in arrival
// order and gives up with their context; a writer that gives up passes the
// turn on to the next in the queue. The zero value is not usable; see
// newContextGate.
type contextGate struct {
	sem *semaphore.Weighted
}

func newContextGate() contextGate {
	return contextGate{sem: semaphore.NewWeighted(contextGateSlots)}
}

// tryShared takes the gate shared if no writer holds it or waits for it.
func (g contextGate) tryShared() bool { return g.sem.TryAcquire(1) }

func (g contextGate) releaseShared() { g.sem.Release(1) }

// lock takes the gate exclusively, or gives up with ctx's error. A gate
// handed over just as ctx ended is given back: the caller is not to go on.
func (g contextGate) lock(ctx context.Context) error {
	if err := g.sem.Acquire(ctx, contextGateSlots); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		g.sem.Release(contextGateSlots)
		return err
	}
	return nil
}

func (g contextGate) unlock() { g.sem.Release(contextGateSlots) }
