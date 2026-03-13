package adt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

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
	reqURL, err := t.buildURL(path, opts.Query)
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
	for name, value := range t.config.Cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}

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
			if err := t.fetchCSRFToken(ctx); err != nil {
				// Return both errors: re-auth failure wraps the original 401 context
				// so callers can see which endpoint triggered the expiry.
				return nil, fmt.Errorf("re-authenticating after 401 on %s: %w (original error: %v)", path, err, apiErr)
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
	reqURL, err := t.buildURL(path, opts.Query)
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
	for name, value := range t.config.Cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	t.setDefaultHeaders(req, opts)
	req.Header.Set("X-CSRF-Token", t.getCSRFToken())

	// Ensure session type header is set for retry
	if t.config.SessionType == SessionStateful {
		req.Header.Set("X-sap-adt-sessiontype", "stateful")
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing retry request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

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
	for name, value := range t.config.Cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	req.Header.Set("X-CSRF-Token", "fetch")
	req.Header.Set("Accept", "*/*")

	// Set session type header for stateful sessions
	if t.config.SessionType == SessionStateful {
		req.Header.Set("X-sap-adt-sessiontype", "stateful")
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Drain body to allow connection reuse
	_, _ = io.Copy(io.Discard, resp.Body)

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
func (t *Transport) buildURL(path string, query url.Values) (string, error) {
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
	if t.config.Language != "" {
		q.Set("sap-language", t.config.Language)
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

	// Set session header based on session type
	switch t.config.SessionType {
	case SessionStateful:
		req.Header.Set("X-sap-adt-sessiontype", "stateful")
	case SessionStateless:
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
		strings.Contains(msg, "session no longer exists")
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
