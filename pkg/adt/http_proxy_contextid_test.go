package adt

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
)

// Session-holding proxies (SAP BAS destination proxy) strip Set-Cookie and
// inject their stored sap-contextid into every request without a Cookie
// header. These tests pin the guard behaviour: stateless requests get an
// explicit empty sap-contextid, stateful ones don't, the CSRF fetch heals
// with stateful + empty contextid, and nothing changes when the guard is
// off or when a real cookie jar is in play.

func TestProxyContextIDGuard_StatelessGetsEmptyContextID(t *testing.T) {
	mock := &mockHTTPClient{responses: []*http.Response{
		newMockResponse(200, "OK", nil), //nolint:bodyclose // mock response; Transport.Request drains and closes it
	}}
	cfg := NewConfig("https://sap.example.com", "u", "p", WithProxyContextIDGuard())
	tr := NewTransportWithClient(cfg, mock)

	if _, err := tr.Request(context.Background(), "/sap/bc/adt/x", nil); err != nil {
		t.Fatalf("Request: %v", err)
	}
	req := mock.requests[0]
	if got := req.Header.Get("X-sap-adt-sessiontype"); got != "stateless" {
		t.Fatalf("sessiontype = %q, want stateless", got)
	}
	if got := req.Header.Get("Cookie"); got != "sap-contextid=" {
		t.Errorf("Cookie = %q, want sap-contextid=", got)
	}
}

func TestProxyContextIDGuard_StatefulUntouched(t *testing.T) {
	mock := &mockHTTPClient{responses: []*http.Response{
		newMockResponse(200, "OK", nil), //nolint:bodyclose // mock response; Transport.Request drains and closes it
	}}
	cfg := NewConfig("https://sap.example.com", "u", "p", WithProxyContextIDGuard())
	tr := NewTransportWithClient(cfg, mock)

	if _, err := tr.Request(context.Background(), "/sap/bc/adt/x", &RequestOptions{Stateful: true}); err != nil {
		t.Fatalf("Request: %v", err)
	}
	req := mock.requests[0]
	if got := req.Header.Get("X-sap-adt-sessiontype"); got != "stateful" {
		t.Fatalf("sessiontype = %q, want stateful", got)
	}
	if got := req.Header.Get("Cookie"); got != "" {
		t.Errorf("Cookie = %q, want empty (proxy must inject its live context)", got)
	}
}

func TestProxyContextIDGuard_DisabledByDefault(t *testing.T) {
	t.Setenv("SAP_PROXY_CONTEXTID_GUARD", "")
	mock := &mockHTTPClient{responses: []*http.Response{
		newMockResponse(200, "OK", nil), //nolint:bodyclose // mock response; Transport.Request drains and closes it
	}}
	cfg := NewConfig("https://sap.example.com", "u", "p")
	tr := NewTransportWithClient(cfg, mock)

	if _, err := tr.Request(context.Background(), "/sap/bc/adt/x", nil); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if got := mock.requests[0].Header.Get("Cookie"); got != "" {
		t.Errorf("Cookie = %q, want none when guard disabled", got)
	}
}

func TestProxyContextIDGuard_EnabledByEnv(t *testing.T) {
	t.Setenv("SAP_PROXY_CONTEXTID_GUARD", "true")
	mock := &mockHTTPClient{responses: []*http.Response{
		newMockResponse(200, "OK", nil), //nolint:bodyclose // mock response; Transport.Request drains and closes it
	}}
	cfg := NewConfig("https://sap.example.com", "u", "p")
	tr := NewTransportWithClient(cfg, mock)

	if _, err := tr.Request(context.Background(), "/sap/bc/adt/x", nil); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if got := mock.requests[0].Header.Get("Cookie"); got != "sap-contextid=" {
		t.Errorf("Cookie = %q, want sap-contextid= (env-enabled)", got)
	}
}

func TestProxyContextIDGuard_SkippedWhenJarHasCookies(t *testing.T) {
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse("https://sap.example.com/sap/bc/adt/x")
	jar.SetCookies(u, []*http.Cookie{{Name: "sap-contextid", Value: "live", Path: "/"}})
	hc := &http.Client{Jar: jar, Transport: nil}

	cfg := NewConfig("https://sap.example.com", "u", "p", WithProxyContextIDGuard())
	tr := NewTransportWithClient(cfg, hc)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://sap.example.com/sap/bc/adt/x", nil)
	req.Header.Set("X-sap-adt-sessiontype", "stateless")
	tr.applyProxyContextIDGuard(req, nil)
	if got := req.Header.Get("Cookie"); got != "" {
		t.Errorf("Cookie = %q, want none when the jar manages the session", got)
	}
}

func TestProxyContextIDGuard_CSRFFetchHealsWithStatefulEmptyContext(t *testing.T) {
	mock := &mockHTTPClient{responses: []*http.Response{
		newMockResponse(400, "", map[string]string{"X-CSRF-Token": "tok"}), //nolint:bodyclose // HEAD /core/discovery; mock response, drained and closed by Transport.Request
		newMockResponse(200, "OK", nil),                                    //nolint:bodyclose // POST; mock response, drained and closed by Transport.Request
	}}
	cfg := NewConfig("https://sap.example.com", "u", "p", WithProxyContextIDGuard())
	tr := NewTransportWithClient(cfg, mock)

	if _, err := tr.Request(context.Background(), "/sap/bc/adt/x", &RequestOptions{Method: http.MethodPost, Body: []byte("<x/>")}); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if len(mock.requests) != 2 {
		t.Fatalf("requests = %d, want 2 (CSRF fetch + POST)", len(mock.requests))
	}
	head := mock.requests[0]
	if head.Method != http.MethodHead {
		t.Fatalf("first request method = %s, want HEAD", head.Method)
	}
	if got := head.Header.Get("X-sap-adt-sessiontype"); got != "stateful" {
		t.Errorf("CSRF fetch sessiontype = %q, want stateful", got)
	}
	if got := head.Header.Get("Cookie"); got != "sap-contextid=" {
		t.Errorf("CSRF fetch Cookie = %q, want sap-contextid=", got)
	}
	if got := mock.requests[1].Header.Get("X-CSRF-Token"); got != "tok" {
		t.Errorf("POST token = %q, want tok", got)
	}
}

func TestProxyContextIDGuard_ICMENOSESSIONRecovery(t *testing.T) {
	mock := &mockHTTPClient{responses: []*http.Response{
		newMockResponse(400, "ICMENOSESSION", nil),                          //nolint:bodyclose // stateless GET → dead context (proxy); mock response, drained and closed by Transport.Request
		newMockResponse(400, "", map[string]string{"X-CSRF-Token": "tok2"}), //nolint:bodyclose // heal: HEAD stateful + empty; mock response, drained and closed by Transport.Request
		newMockResponse(200, "OK", nil),                                     //nolint:bodyclose // retry; mock response, drained and closed by Transport.Request
	}}
	cfg := NewConfig("https://sap.example.com", "u", "p", WithProxyContextIDGuard())
	tr := NewTransportWithClient(cfg, mock)

	if _, err := tr.Request(context.Background(), "/sap/bc/adt/x", nil); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if len(mock.requests) != 3 {
		t.Fatalf("requests = %d, want 3", len(mock.requests))
	}
	heal := mock.requests[1]
	if heal.Header.Get("X-sap-adt-sessiontype") != "stateful" || heal.Header.Get("Cookie") != "sap-contextid=" {
		t.Errorf("heal request headers = %v", heal.Header)
	}
	if got := mock.requests[2].Header.Get("Cookie"); got != "sap-contextid=" {
		t.Errorf("retry (stateless) Cookie = %q, want sap-contextid=", got)
	}
}

func TestProxyContextIDGuard_FreshContextOnStatefulLock(t *testing.T) {
	cfg := NewConfig("https://sap.example.com", "u", "p", WithProxyContextIDGuard())
	tr := NewTransportWithClient(cfg, &mockHTTPClient{})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://sap.example.com/sap/bc/adt/x?_action=LOCK", nil)
	req.Header.Set("X-sap-adt-sessiontype", "stateful")
	tr.applyProxyContextIDGuard(req, &RequestOptions{Stateful: true, FreshContext: true})
	if got := req.Header.Get("Cookie"); got != "sap-contextid=" {
		t.Errorf("Cookie = %q, want sap-contextid= so the LOCK opens a fresh context", got)
	}
}

func TestProxyContextIDGuard_ReleaseContextLeavesCookieOff(t *testing.T) {
	cfg := NewConfig("https://sap.example.com", "u", "p", WithProxyContextIDGuard())
	tr := NewTransportWithClient(cfg, &mockHTTPClient{})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodHead, "https://sap.example.com/sap/bc/adt/core/discovery", nil)
	req.Header.Set("X-sap-adt-sessiontype", "stateless")
	tr.applyProxyContextIDGuard(req, &RequestOptions{ReleaseContext: true})
	if got := req.Header.Get("Cookie"); got != "" {
		t.Errorf("Cookie = %q, want none so the proxy injects the context to be released", got)
	}
}

func TestProxyContextIDGuard_ReleaseProxyContextIsNoopWithoutGuard(t *testing.T) {
	mock := &mockHTTPClient{}
	cfg := NewConfig("https://sap.example.com", "u", "p")
	tr := NewTransportWithClient(cfg, mock)
	tr.ReleaseProxyContext(context.Background())
	if len(mock.requests) != 0 {
		t.Errorf("ReleaseProxyContext sent %d request(s) without the guard, want 0", len(mock.requests))
	}
}
