package adt

import (
	"crypto/tls"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

// TestNewZADTVSPDialer_HonoursProxyEnv pins the HTTP_PROXY /
// HTTPS_PROXY fix for WebSocket connections. Mirrors the Proxy-field
// check TestNewHTTPClient runs against the HTTP transport for issue
// #13 — without http.ProxyFromEnvironment on the gorilla/websocket
// dialer, every WebSocket-backed tool (ABAP debugger, AMDP debugger,
// RFC, report execution, abapGit WebSocket export) fails to connect
// on SAP BAS destinations that require a Connectivity Proxy.
func TestNewZADTVSPDialer_HonoursProxyEnv(t *testing.T) {
	tlsCfg := &tls.Config{}
	dialer := newZADTVSPDialer(tlsCfg)

	if dialer.Proxy == nil {
		t.Error("WebSocket dialer must have Proxy set (HTTP_PROXY/HTTPS_PROXY support — same fix class as issue #13)")
	}
	if dialer.TLSClientConfig != tlsCfg {
		t.Error("dialer must carry the TLS config passed in")
	}
	if dialer.HandshakeTimeout == 0 {
		t.Error("dialer must have a non-zero handshake timeout")
	}
}

// TestNewPreAuthHTTPClient_HonoursProxyEnv extends the #13 fix to the
// transient HTTP client that the WebSocket 401-retry path uses to
// obtain SAP session cookies before the second dial attempt. The
// fallback builds its own http.Transport inline instead of reusing
// Config.NewHTTPClient (it needs a dedicated cookie jar and
// per-request Basic Auth), so the proxy fix has to be applied here
// too — otherwise the pre-auth GET fails on BAS with exactly the
// same symptom the WebSocket dial has.
func TestNewPreAuthHTTPClient_HonoursProxyEnv(t *testing.T) {
	jar, _ := cookiejar.New(nil)
	tlsCfg := &tls.Config{}
	client := newPreAuthHTTPClient(jar, tlsCfg)

	if client == nil {
		t.Fatal("newPreAuthHTTPClient returned nil")
	}
	if client.Jar != jar {
		t.Error("pre-auth client must use the jar passed in so the session cookie survives into the dialer retry")
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("pre-auth client transport must be *http.Transport, got %T", client.Transport)
	}
	if transport.Proxy == nil {
		t.Error("pre-auth HTTP transport must have Proxy set (HTTP_PROXY/HTTPS_PROXY support — same fix class as issue #13)")
	}
	if transport.TLSClientConfig != tlsCfg {
		t.Error("pre-auth transport must carry the TLS config passed in")
	}
	if client.Timeout == 0 {
		t.Error("pre-auth client must have a non-zero timeout")
	}
}
