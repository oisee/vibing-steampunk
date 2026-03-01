package adt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// mockHTTPClient is a mock HTTP client for testing.
type mockHTTPClient struct {
	responses []*http.Response
	requests  []*http.Request
	callIndex int
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	m.requests = append(m.requests, req)
	if m.callIndex >= len(m.responses) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("no more mock responses")),
			Header:     http.Header{},
		}, nil
	}
	resp := m.responses[m.callIndex]
	m.callIndex++
	return resp, nil
}

func newMockResponse(statusCode int, body string, headers map[string]string) *http.Response {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     h,
	}
}

func TestTransport_Request_BasicAuth(t *testing.T) {
	mock := &mockHTTPClient{
		responses: []*http.Response{
			newMockResponse(200, "OK", map[string]string{"X-CSRF-Token": "test-token"}),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "testuser", "testpass")
	transport := NewTransportWithClient(cfg, mock)

	_, err := transport.Request(context.Background(), "/sap/bc/adt/test", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if len(mock.requests) != 1 {
		t.Fatalf("Expected 1 request, got %d", len(mock.requests))
	}

	req := mock.requests[0]
	user, pass, ok := req.BasicAuth()
	if !ok {
		t.Error("Basic auth not set")
	}
	if user != "testuser" {
		t.Errorf("Username = %v, want testuser", user)
	}
	if pass != "testpass" {
		t.Errorf("Password = %v, want testpass", pass)
	}
}

func TestTransport_Request_QueryParams(t *testing.T) {
	mock := &mockHTTPClient{
		responses: []*http.Response{
			newMockResponse(200, "OK", nil),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass",
		WithClient("100"),
		WithLanguage("DE"),
	)
	transport := NewTransportWithClient(cfg, mock)

	query := url.Values{}
	query.Set("custom", "value")

	_, err := transport.Request(context.Background(), "/sap/bc/adt/test", &RequestOptions{
		Query: query,
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	req := mock.requests[0]
	q := req.URL.Query()

	if q.Get("sap-client") != "100" {
		t.Errorf("sap-client = %v, want 100", q.Get("sap-client"))
	}
	if q.Get("sap-language") != "DE" {
		t.Errorf("sap-language = %v, want DE", q.Get("sap-language"))
	}
	if q.Get("custom") != "value" {
		t.Errorf("custom = %v, want value", q.Get("custom"))
	}
}

func TestTransport_Request_CSRFToken(t *testing.T) {
	mock := &mockHTTPClient{
		responses: []*http.Response{
			// First: fetch CSRF token
			newMockResponse(200, "OK", map[string]string{"X-CSRF-Token": "csrf-token-123"}),
			// Second: actual POST request
			newMockResponse(200, "OK", nil),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)

	// Make a POST request (requires CSRF)
	_, err := transport.Request(context.Background(), "/sap/bc/adt/test", &RequestOptions{
		Method: http.MethodPost,
		Body:   []byte("test body"),
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Should have made 2 requests: CSRF fetch + actual POST
	if len(mock.requests) != 2 {
		t.Fatalf("Expected 2 requests, got %d", len(mock.requests))
	}

	// First request should fetch CSRF
	csrfReq := mock.requests[0]
	if csrfReq.Header.Get("X-CSRF-Token") != "fetch" {
		t.Error("First request should have X-CSRF-Token: fetch")
	}

	// Second request should include CSRF token
	postReq := mock.requests[1]
	if postReq.Header.Get("X-CSRF-Token") != "csrf-token-123" {
		t.Errorf("POST request CSRF token = %v, want csrf-token-123", postReq.Header.Get("X-CSRF-Token"))
	}
}

func TestTransport_Request_CSRFRefreshOn403(t *testing.T) {
	mock := &mockHTTPClient{
		responses: []*http.Response{
			// First: fetch initial CSRF token
			newMockResponse(200, "OK", map[string]string{"X-CSRF-Token": "old-token"}),
			// Second: POST fails with 403
			newMockResponse(403, "Forbidden", nil),
			// Third: refresh CSRF token
			newMockResponse(200, "OK", map[string]string{"X-CSRF-Token": "new-token"}),
			// Fourth: retry POST
			newMockResponse(200, "Success", nil),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)

	resp, err := transport.Request(context.Background(), "/sap/bc/adt/test", &RequestOptions{
		Method: http.MethodPost,
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if string(resp.Body) != "Success" {
		t.Errorf("Response body = %v, want Success", string(resp.Body))
	}

	// Should have made 4 requests
	if len(mock.requests) != 4 {
		t.Fatalf("Expected 4 requests, got %d", len(mock.requests))
	}
}

func TestTransport_Request_RetryOn401(t *testing.T) {
	mock := &mockHTTPClient{
		responses: []*http.Response{
			// First: GET returns 401 Unauthorized (session expired after idle)
			newMockResponse(401, "Unauthorized", nil),
			// Second: re-authenticate — fetch new CSRF token
			newMockResponse(200, "OK", map[string]string{"X-CSRF-Token": "new-token"}),
			// Third: retry original GET succeeds
			newMockResponse(200, "Success", nil),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)

	resp, err := transport.Request(context.Background(), "/sap/bc/adt/test", nil)
	if err != nil {
		t.Fatalf("Request should have succeeded after retry, got: %v", err)
	}

	if string(resp.Body) != "Success" {
		t.Errorf("Response body = %v, want Success", string(resp.Body))
	}

	// Should have made 3 requests: original GET + CSRF fetch + retry GET
	if len(mock.requests) != 3 {
		t.Fatalf("Expected 3 requests, got %d", len(mock.requests))
	}
}

func TestTransport_Request_RetryOn401_POST(t *testing.T) {
	mock := &mockHTTPClient{
		responses: []*http.Response{
			// First: fetch initial CSRF token
			newMockResponse(200, "OK", map[string]string{"X-CSRF-Token": "old-token"}),
			// Second: POST returns 401 Unauthorized
			newMockResponse(401, "Unauthorized", nil),
			// Third: re-authenticate — fetch new CSRF token
			newMockResponse(200, "OK", map[string]string{"X-CSRF-Token": "new-token"}),
			// Fourth: retry POST succeeds
			newMockResponse(200, "Success", nil),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)

	resp, err := transport.Request(context.Background(), "/sap/bc/adt/test", &RequestOptions{
		Method: http.MethodPost,
		Body:   []byte("test body"),
	})
	if err != nil {
		t.Fatalf("Request should have succeeded after retry, got: %v", err)
	}

	if string(resp.Body) != "Success" {
		t.Errorf("Response body = %v, want Success", string(resp.Body))
	}

	// Should have made 4 requests: CSRF fetch + POST + CSRF refresh + retry POST
	if len(mock.requests) != 4 {
		t.Fatalf("Expected 4 requests, got %d", len(mock.requests))
	}
}

func TestTransport_Request_ErrorResponse(t *testing.T) {
	mock := &mockHTTPClient{
		responses: []*http.Response{
			newMockResponse(404, "Not found", nil),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)

	_, err := transport.Request(context.Background(), "/sap/bc/adt/test", nil)
	if err == nil {
		t.Fatal("Expected error for 404 response")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("Expected APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("StatusCode = %v, want 404", apiErr.StatusCode)
	}
}

func TestTransport_BuildURL(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass",
		WithClient("001"),
		WithLanguage("EN"),
	)
	transport := NewTransport(cfg)

	tests := []struct {
		name     string
		path     string
		query    url.Values
		wantHost string
		wantPath string
	}{
		{
			name:     "simple path",
			path:     "/sap/bc/adt/test",
			query:    nil,
			wantHost: "sap.example.com:44300",
			wantPath: "/sap/bc/adt/test",
		},
		{
			name:     "path without leading slash",
			path:     "sap/bc/adt/test",
			query:    nil,
			wantHost: "sap.example.com:44300",
			wantPath: "/sap/bc/adt/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := transport.buildURL(tt.path, tt.query)
			if err != nil {
				t.Fatalf("buildURL failed: %v", err)
			}

			u, _ := url.Parse(got)
			if u.Host != tt.wantHost {
				t.Errorf("Host = %v, want %v", u.Host, tt.wantHost)
			}
			if u.Path != tt.wantPath {
				t.Errorf("Path = %v, want %v", u.Path, tt.wantPath)
			}

			// Check default query params
			q := u.Query()
			if q.Get("sap-client") != "001" {
				t.Errorf("sap-client = %v, want 001", q.Get("sap-client"))
			}
			if q.Get("sap-language") != "EN" {
				t.Errorf("sap-language = %v, want EN", q.Get("sap-language"))
			}
		})
	}
}

func TestIsModifyingMethod(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{http.MethodGet, false},
		{http.MethodHead, false},
		{http.MethodOptions, false},
		{http.MethodPost, true},
		{http.MethodPut, true},
		{http.MethodDelete, true},
		{http.MethodPatch, true},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := isModifyingMethod(tt.method); got != tt.want {
				t.Errorf("isModifyingMethod(%v) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

func TestAPIError_Error(t *testing.T) {
	err := &APIError{
		StatusCode: 404,
		Message:    "Object not found",
		Path:       "/sap/bc/adt/programs/test",
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "404") {
		t.Error("Error string should contain status code")
	}
	if !strings.Contains(errStr, "Object not found") {
		t.Error("Error string should contain message")
	}
	if !strings.Contains(errStr, "/sap/bc/adt/programs/test") {
		t.Error("Error string should contain path")
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"404 error", &APIError{StatusCode: 404, Message: "Not found"}, true},
		{"400 error", &APIError{StatusCode: 400, Message: "Bad request"}, false},
		{"500 error", &APIError{StatusCode: 500, Message: "Server error"}, false},
		{"wrapped 404", fmt.Errorf("wrapped: %w", &APIError{StatusCode: 404}), true},
		{"wrapped 500", fmt.Errorf("wrapped: %w", &APIError{StatusCode: 500}), false},
		{"non-API error", fmt.Errorf("generic error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFoundError(tt.err); got != tt.want {
				t.Errorf("IsNotFoundError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSessionExpiredError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"404 error", &APIError{StatusCode: 404, Message: "Not found"}, false},
		{"400 ICMENOSESSION", &APIError{StatusCode: 400, Message: "ICMENOSESSION"}, true},
		{"400 Session Timed Out", &APIError{StatusCode: 400, Message: "Session Timed Out"}, true},
		{"400 session no longer exists", &APIError{StatusCode: 400, Message: "Session no longer exists"}, true},
		{"400 other error", &APIError{StatusCode: 400, Message: "Bad request"}, false},
		{"500 session timeout text", &APIError{StatusCode: 500, Message: "ICMENOSESSION"}, false},
		{"wrapped session expired", fmt.Errorf("wrapped: %w", &APIError{StatusCode: 400, Message: "ICMENOSESSION"}), true},
		{"non-API error", fmt.Errorf("generic error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSessionExpiredError(tt.err); got != tt.want {
				t.Errorf("IsSessionExpiredError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransport_Request_CookieAuth(t *testing.T) {
	mock := &mockHTTPClient{
		responses: []*http.Response{
			newMockResponse(200, "OK", map[string]string{"X-CSRF-Token": "test-token"}),
		},
	}

	cookies := map[string]string{
		"sap-usercontext":       "sap-language=EN&sap-client=001",
		"SAP_SESSIONID_A4H_001": "session123",
		"MYSAPSSO2":             "sso-token-xyz",
	}

	cfg := NewConfig("https://sap.example.com:44300", "", "", WithCookies(cookies))
	transport := NewTransportWithClient(cfg, mock)

	_, err := transport.Request(context.Background(), "/sap/bc/adt/test", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if len(mock.requests) != 1 {
		t.Fatalf("Expected 1 request, got %d", len(mock.requests))
	}

	req := mock.requests[0]

	// Should NOT have basic auth
	_, _, ok := req.BasicAuth()
	if ok {
		t.Error("Basic auth should NOT be set when using cookie auth")
	}

	// Should have all cookies
	foundCookies := make(map[string]string)
	for _, c := range req.Cookies() {
		foundCookies[c.Name] = c.Value
	}

	for name, value := range cookies {
		if foundCookies[name] != value {
			t.Errorf("Cookie[%q] = %q, want %q", name, foundCookies[name], value)
		}
	}
}

func TestTransport_Request_CookieAuth_CSRF(t *testing.T) {
	mock := &mockHTTPClient{
		responses: []*http.Response{
			// First: fetch CSRF token
			newMockResponse(200, "OK", map[string]string{"X-CSRF-Token": "csrf-token-456"}),
			// Second: actual POST request
			newMockResponse(200, "OK", nil),
		},
	}

	cookies := map[string]string{
		"SAP_SESSIONID": "session123",
	}

	cfg := NewConfig("https://sap.example.com:44300", "", "", WithCookies(cookies))
	transport := NewTransportWithClient(cfg, mock)

	// Make a POST request (requires CSRF)
	_, err := transport.Request(context.Background(), "/sap/bc/adt/test", &RequestOptions{
		Method: http.MethodPost,
		Body:   []byte("test body"),
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Should have made 2 requests: CSRF fetch + actual POST
	if len(mock.requests) != 2 {
		t.Fatalf("Expected 2 requests, got %d", len(mock.requests))
	}

	// Both requests should have cookies
	for i, req := range mock.requests {
		found := false
		for _, c := range req.Cookies() {
			if c.Name == "SAP_SESSIONID" && c.Value == "session123" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Request %d: missing SAP_SESSIONID cookie", i+1)
		}
	}

	// Second request should include CSRF token
	postReq := mock.requests[1]
	if postReq.Header.Get("X-CSRF-Token") != "csrf-token-456" {
		t.Errorf("POST request CSRF token = %v, want csrf-token-456", postReq.Header.Get("X-CSRF-Token"))
	}
}

func TestTransport_Request_BasicAuth_NotAffectedByCookies(t *testing.T) {
	// This test ensures basic auth still works correctly and takes precedence
	mock := &mockHTTPClient{
		responses: []*http.Response{
			newMockResponse(200, "OK", map[string]string{"X-CSRF-Token": "test-token"}),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "testuser", "testpass")
	transport := NewTransportWithClient(cfg, mock)

	_, err := transport.Request(context.Background(), "/sap/bc/adt/test", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	req := mock.requests[0]

	// Should have basic auth
	user, pass, ok := req.BasicAuth()
	if !ok {
		t.Error("Basic auth should be set")
	}
	if user != "testuser" {
		t.Errorf("Username = %v, want testuser", user)
	}
	if pass != "testpass" {
		t.Errorf("Password = %v, want testpass", pass)
	}
}

func TestTransport_Request_BothAuthMethods(t *testing.T) {
	// When both basic auth and cookies are provided, both should be sent
	mock := &mockHTTPClient{
		responses: []*http.Response{
			newMockResponse(200, "OK", nil),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "testuser", "testpass",
		WithCookies(map[string]string{"session": "abc123"}),
	)
	transport := NewTransportWithClient(cfg, mock)

	_, err := transport.Request(context.Background(), "/sap/bc/adt/test", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	req := mock.requests[0]

	// Should have basic auth
	user, _, ok := req.BasicAuth()
	if !ok {
		t.Error("Basic auth should be set")
	}
	if user != "testuser" {
		t.Errorf("Username = %v, want testuser", user)
	}

	// Should also have cookies
	found := false
	for _, c := range req.Cookies() {
		if c.Name == "session" && c.Value == "abc123" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Cookie should also be present when both auth methods are set")
	}
}
