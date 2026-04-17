package adt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
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

func traceHTTPRequest(req *http.Request, body []byte) {
	if !httpTraceEnabled() {
		return
	}
	fmt.Fprintf(os.Stderr, "\n>>> HTTP %s %s\n", req.Method, req.URL.String())
	for k, vs := range req.Header {
		for _, v := range vs {
			if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "Cookie") {
				v = "[REDACTED]"
			}
			fmt.Fprintf(os.Stderr, ">>> %s: %s\n", k, v)
		}
	}
	if len(body) > 0 {
		trunc := body
		if len(trunc) > httpTraceBodyLimit {
			trunc = trunc[:httpTraceBodyLimit]
		}
		fmt.Fprintf(os.Stderr, ">>> body (%d bytes):\n%s\n", len(body), string(trunc))
		if len(body) > httpTraceBodyLimit {
			fmt.Fprintf(os.Stderr, ">>> ... (truncated)\n")
		}
	}
}

func traceHTTPResponse(resp *http.Response, body []byte) {
	if !httpTraceEnabled() || resp == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "<<< HTTP %d %s\n", resp.StatusCode, resp.Status)
	for k, vs := range resp.Header {
		for _, v := range vs {
			if strings.EqualFold(k, "Set-Cookie") {
				if i := strings.Index(v, "="); i > 0 {
					v = v[:i] + "=[REDACTED]"
				}
			}
			fmt.Fprintf(os.Stderr, "<<< %s: %s\n", k, v)
		}
	}
	if len(body) > 0 {
		trunc := body
		if len(trunc) > httpTraceBodyLimit {
			trunc = trunc[:httpTraceBodyLimit]
		}
		fmt.Fprintf(os.Stderr, "<<< body (%d bytes):\n%s\n", len(body), string(trunc))
		if len(body) > httpTraceBodyLimit {
			fmt.Fprintf(os.Stderr, "<<< ... (truncated)\n")
		}
	}
	fmt.Fprintln(os.Stderr)
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
	return &Transport{
		config:     cfg,
		httpClient: cfg.NewHTTPClient(),
	}
}

// NewTransportWithClient creates a new Transport with a custom HTTP client.
// This is useful for testing with mock HTTP clients.
func NewTransportWithClient(cfg *Config, client HTTPDoer) *Transport {
	return &Transport{
		config:     cfg,
		httpClient: client,
	}
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
}

// Response wraps an HTTP response with convenience methods.
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// Request performs an HTTP request to the ADT API.
func (t *Transport) Request(ctx context.Context, path string, opts *RequestOptions) (*Response, error) {
	if opts == nil {
		opts = &RequestOptions{}
	}
	if opts.Method == "" {
		opts.Method = http.MethodGet
	}

	// Build URL
	reqURL, err := t.buildURL(path, opts.Query, opts.OverrideLanguage)
	if err != nil {
		return nil, fmt.Errorf("building URL: %w", err)
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

	// Add user-provided cookies for cookie-based authentication
	t.addCookies(req)

	// Set default headers
	t.setDefaultHeaders(req, opts)

	// Add CSRF token for modifying requests
	if isModifyingMethod(opts.Method) {
		token := t.getCSRFToken()
		if token == "" {
			// Fetch CSRF token first
			if err := t.fetchCSRFToken(ctx); err != nil {
				return nil, fmt.Errorf("fetching CSRF token: %w", err)
			}
			token = t.getCSRFToken()
		}
		req.Header.Set("X-CSRF-Token", token)
	}

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

	// Handle CSRF token refresh on 403
	if resp.StatusCode == http.StatusForbidden && isModifyingMethod(opts.Method) {
		// Try to refresh CSRF token and retry once
		if err := t.fetchCSRFToken(ctx); err != nil {
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
			// Fetch new CSRF token (this establishes a new session)
			if err := t.fetchCSRFToken(ctx); err != nil {
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
				if err := t.fetchCSRFToken(ctx); err != nil {
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
	t.addCookies(req)
	t.setDefaultHeaders(req, opts)
	req.Header.Set("X-CSRF-Token", t.getCSRFToken())

	// Ensure session type header is set for retry
	if t.config.SessionType == SessionStateful {
		req.Header.Set("X-sap-adt-sessiontype", "stateful")
	}

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
// Uses /core/discovery with HEAD for optimal performance (~25ms vs ~56s for GET on /discovery)
func (t *Transport) fetchCSRFToken(ctx context.Context) error {
	reqURL, err := t.buildURL("/sap/bc/adt/core/discovery", nil)
	if err != nil {
		return fmt.Errorf("building URL: %w", err)
	}

	// Use HEAD instead of GET for faster CSRF token fetch (~5s vs ~56s on slow systems)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// Set authentication
	if t.config.HasBasicAuth() {
		req.SetBasicAuth(t.config.Username, t.config.Password)
	}
	t.addCookies(req)
	req.Header.Set("X-CSRF-Token", "fetch")
	req.Header.Set("Accept", "*/*")

	// Set session type header for stateful sessions
	if t.config.SessionType == SessionStateful {
		req.Header.Set("X-sap-adt-sessiontype", "stateful")
	}

	traceHTTPRequest(req, nil)
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Drain body to allow connection reuse
	_, _ = io.Copy(io.Discard, resp.Body)
	traceHTTPResponse(resp, nil)

	// Note: HEAD may return 400 but still provides CSRF token in headers
	// But 401/403 indicates auth failure and won't have a valid token

	token := resp.Header.Get("X-CSRF-Token")
	if token == "" || token == "Required" {
		// Provide better error message based on status code
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("authentication failed (401): check username/password")
		case http.StatusForbidden:
			return fmt.Errorf("access forbidden (403): check user authorizations")
		default:
			return fmt.Errorf("no CSRF token in response (HTTP %d)", resp.StatusCode)
		}
	}

	t.setCSRFToken(token)
	return nil
}

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
	reauthCtx, cancel := context.WithTimeout(ctx, reauthTimeout)
	defer cancel()

	cookies, err := t.config.ReauthFunc(reauthCtx)
	if err != nil {
		return err
	}

	t.cookiesMu.Lock()
	t.config.Cookies = cookies
	t.cookiesMu.Unlock()

	// Fetch CSRF token with the new cookies.
	// Set lastReauth only after CSRF succeeds — if it fails, the next
	// goroutine should retry rather than hitting the cooldown skip.
	if err := t.fetchCSRFToken(reauthCtx); err != nil {
		return err
	}
	t.lastReauth = time.Now()
	return nil
}

// addCookies adds user-provided cookies to a request under cookiesMu read lock.
func (t *Transport) addCookies(req *http.Request) {
	t.cookiesMu.RLock()
	defer t.cookiesMu.RUnlock()
	for name, value := range t.config.Cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}
}
