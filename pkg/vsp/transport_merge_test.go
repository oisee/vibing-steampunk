package vsp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
)

// upgradeAuth is the credential a WebSocket upgrade to ZADT_VSP carried.
type upgradeAuth struct {
	seen          bool
	cookie, basic string
}

// wsAuthServer refuses every request and records the credential on the
// WebSocket upgrade. The system it serves is the default of a .vsp.json in an
// empty directory, with extra spliced into its settings.
func wsAuthServer(t *testing.T, extra string) func() upgradeAuth {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", dir)
	for _, name := range envOnlyCLIVars {
		t.Setenv(name, "")
	}
	saved, savedCookies := systemName, cfg.Cookies
	t.Cleanup(func() { systemName, cfg.Cookies = saved, savedCookies })
	systemName = ""

	var mu sync.Mutex
	var got upgradeAuth
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "" {
			mu.Lock()
			got = upgradeAuth{seen: true, cookie: r.Header.Get("Cookie"), basic: r.Header.Get("Authorization")}
			mu.Unlock()
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	conf := fmt.Sprintf(`{"default":"qassys","systems":{"qassys":{"url":%q,"client":"100",%s"enable_transports":true}}}`, srv.URL, extra)
	if err := os.WriteFile(".vsp.json", []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	return func() upgradeAuth {
		mu.Lock()
		defer mu.Unlock()
		return got
	}
}

// The merge, move, add and remove commands share transportBridge. It used to
// hand its WebSocket only the global cfg.Cookies, which no CLI subcommand ever
// sets, so a profile that signs in with a cookie_string, a cookie_file or
// single sign-on reached ZADT_VSP with no credential at all.
func TestTransportBridgeUsesTheProfileSession(t *testing.T) {
	upgrade := wsAuthServer(t, `"cookie_string":"MYSAPSSO2=profile-session",`)
	cfg.Cookies = nil

	if _, _, _, err := transportBridge(transportMergeCmd); err == nil {
		t.Fatal("the server refuses every upgrade; the bridge reported it reachable")
	}
	got := upgrade()
	if !got.seen {
		t.Fatal("the bridge never attempted the WebSocket upgrade")
	}
	if got.cookie != "MYSAPSSO2=profile-session" {
		t.Errorf("the upgrade carried cookie %q, want the profile's session", got.cookie)
	}
	if got.basic != "" {
		t.Errorf("the upgrade carried Authorization %q alongside the session; it is one or the other", got.basic)
	}
}

// A profile on a password authenticates the upgrade with it -- and not with a
// cookie map that happens to be lying in the global config, which belongs to
// no system this command resolved.
func TestTransportBridgeUsesTheProfilePassword(t *testing.T) {
	upgrade := wsAuthServer(t, `"user":"TESTUSER",`)
	t.Setenv("VSP_QASSYS_PASSWORD", "s3cret")
	cfg.Cookies = map[string]string{"SAP_SESSIONID_OTHER_000": "stray"}

	if _, _, _, err := transportBridge(transportMoveCmd); err == nil {
		t.Fatal("the server refuses every upgrade; the bridge reported it reachable")
	}
	got := upgrade()
	if !got.seen {
		t.Fatal("the bridge never attempted the WebSocket upgrade")
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.SetBasicAuth("TESTUSER", "s3cret")
	if got.basic != req.Header.Get("Authorization") {
		t.Errorf("the upgrade carried Authorization %q, want the profile's basic auth", got.basic)
	}
	if got.cookie != "" {
		t.Errorf("the upgrade carried cookie %q; a password profile sends basic auth only", got.cookie)
	}
}
