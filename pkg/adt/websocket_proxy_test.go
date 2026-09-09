package adt

import (
	"crypto/tls"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

// TestWebSocketDialerHonoursProxyEnv guards the regression where the ZADT_VSP
// WebSocket dialer carried no proxy resolver: behind a corporate proxy the upgrade
// failed with no useful diagnosis, while every plain HTTP call went through, because
// the ADT client sets Proxy and this one did not.
//
// It asserts the wiring rather than a resolution, because http.ProxyFromEnvironment
// reads the environment once per process — a test cannot meaningfully vary it.
func TestWebSocketDialerHonoursProxyEnv(t *testing.T) {
	d := newWebSocketDialer(&tls.Config{}) //nolint:gosec // test config, no handshake
	if d.Proxy == nil {
		t.Fatal("the WebSocket dialer must resolve a proxy from the environment")
	}
	if d.HandshakeTimeout == 0 {
		t.Fatal("the dialer must keep its handshake timeout")
	}
}

// TestPreAuthClientHonoursProxyEnv pins the same wiring on the pre-auth client:
// the request that fetches session cookies after a 401 on the upgrade must go
// through the same proxy as the upgrade itself, keep the jar it was given (the
// retry dials with those cookies) and carry the TLS config and a timeout.
func TestPreAuthClientHonoursProxyEnv(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	tlsCfg := &tls.Config{} //nolint:gosec // test config, no handshake

	client := newPreAuthHTTPClient(jar, tlsCfg)

	if client.Jar != jar {
		t.Fatal("the pre-auth client must use the jar it was given, so the cookies reach the dialer's retry")
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("the pre-auth client's transport must be an *http.Transport, got %T", client.Transport)
	}
	if transport.Proxy == nil {
		t.Fatal("the pre-auth client must resolve a proxy from the environment")
	}
	if transport.TLSClientConfig != tlsCfg {
		t.Fatal("the pre-auth client must carry the TLS config it was given")
	}
	if client.Timeout == 0 {
		t.Fatal("the pre-auth client must have a timeout")
	}
}
