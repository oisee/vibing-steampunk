package vsp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oisee/vibing-steampunk/internal/dap"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// A single sign-on refresh replaces the side connection's cookies; what it
// returns is filtered like the first set, so a sap-contextid in it never
// reaches the next side request.
func TestDAPSideConnectionFiltersRefreshedCookies(t *testing.T) {
	var mu sync.Mutex
	deletes, refreshes := 0, 0
	var cookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Header.Get("X-Csrf-Token"), "fetch") {
			w.Header().Set("X-Csrf-Token", "token")
			return
		}
		if r.Method != http.MethodDelete {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		deletes++
		if deletes == 1 {
			w.WriteHeader(http.StatusUnauthorized) // the SSO ticket expired
			return
		}
		cookie = r.Header.Get("Cookie")
	}))
	defer srv.Close()

	refresh := func(context.Context) (map[string]string, error) {
		mu.Lock()
		refreshes++
		mu.Unlock()
		return map[string]string{"MYSAPSSO2": "fresh", "sap-contextid": "SID%3aANON%3astateful"}, nil
	}
	opts := append([]adt.Option{adt.WithClient("001"), adt.WithSessionType(adt.SessionStateless), adt.WithTimeout(10 * time.Second)},
		ssoAuthOptions(map[string]string{"MYSAPSSO2": "first"}, refresh, 10*time.Second, withoutContext)...)
	side := saprfc.HTTPStateless(adt.NewTransport(adt.NewConfig(srv.URL, "", "", opts...)))

	res, err := side.Do(context.Background(), saprfc.ADTRequest{Method: "DELETE", URI: "/sap/bc/adt/debugger/listeners?debuggingMode=user&requestUser=X"})
	if err != nil || res.Status != 200 {
		t.Fatalf("delete: %v %v", res, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if refreshes != 1 || deletes != 2 {
		t.Fatalf("want one refresh and a retried delete; refreshes=%d deletes=%d", refreshes, deletes)
	}
	if strings.Contains(cookie, "sap-contextid") || !strings.Contains(cookie, "MYSAPSSO2=fresh") {
		t.Errorf("cookies sent after the refresh: %q", cookie)
	}
}

// The opener gives the session a side connection, and that connection is
// stateless and joins no session: it sends no sap-contextid, even when the
// logon's cookies carry one, so SAP does not queue it behind the debug
// session's open listener.
func TestDAPSideConnectionIsStateless(t *testing.T) {
	dapConfigDir(t)
	sess, err := dapOpener(dapCmd)(context.Background(), dap.LaunchArgs{ListenSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	if sess.Aside == nil {
		t.Fatal("no side connection")
	}

	var mu sync.Mutex
	var sessionType, cookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Header.Get("X-Csrf-Token"), "fetch") {
			w.Header().Set("X-Csrf-Token", "token")
			return
		}
		if r.Method == http.MethodDelete {
			mu.Lock()
			sessionType, cookie = r.Header.Get("X-sap-adt-sessiontype"), r.Header.Get("Cookie")
			mu.Unlock()
		}
	}))
	defer srv.Close()

	side, err := statelessADTTransport(&systemParams{URL: srv.URL, Client: "001",
		CookieString: "sap-contextid=SID%3aANON%3aabc; MYSAPSSO2=ticket"}, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	res, err := side.Do(context.Background(), saprfc.ADTRequest{Method: "DELETE", URI: "/sap/bc/adt/debugger/listeners?debuggingMode=user&requestUser=X"})
	if err != nil || res.Status != 200 {
		t.Fatalf("delete: %v %v", res, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if sessionType != "stateless" {
		t.Errorf("session type %q, want stateless", sessionType)
	}
	if strings.Contains(cookie, "sap-contextid") || !strings.Contains(cookie, "MYSAPSSO2") {
		t.Errorf("cookies sent: %q", cookie)
	}
}

// dapConfigDir puts a .vsp.json with two systems, one read-only, in an empty
// working directory and home, so no real configuration is read. Opening a
// session makes no request: the transport is only built.
func dapConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("SAP_READ_ONLY", "")
	t.Setenv("VSP_DEV_PASSWORD", "secret")
	t.Setenv("VSP_PRD_PASSWORD", "secret")
	t.Chdir(dir)
	cfg := `{"default":"dev","systems":{
	  "dev":{"url":"http://127.0.0.1:1","user":"devuser","client":"001"},
	  "prd":{"url":"http://127.0.0.1:2","user":"prduser","client":"001","read_only":true}}}`
	if err := os.WriteFile(filepath.Join(dir, ".vsp.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	saved, savedRO, savedUser := systemName, dapReadOnly, dapUser
	t.Cleanup(func() { systemName, dapReadOnly, dapUser = saved, savedRO, savedUser })
	systemName, dapReadOnly, dapUser = "", false, ""
}

// The launch configuration picks the system and the user; the logon itself
// comes from vsp's config, and the system's read_only reaches the session.
func TestDAPOpenerUsesVspConfig(t *testing.T) {
	dapConfigDir(t)
	open := dapOpener(dapCmd)

	sess, err := open(context.Background(), dap.LaunchArgs{ListenSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	if sess.System != "dev" || sess.User != "DEVUSER" || sess.ReadOnly {
		t.Errorf("default system: %+v", sess)
	}

	sess, err = open(context.Background(), dap.LaunchArgs{System: "prd", User: "someone", ListenSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	if sess.System != "prd" || sess.User != "SOMEONE" || !sess.ReadOnly {
		t.Errorf("named read-only system: %+v", sess)
	}
}

// --read-only and SAP_READ_ONLY make any system read-only for the adapter.
func TestDAPReadOnlyFlagAndEnv(t *testing.T) {
	dapConfigDir(t)
	dapReadOnly = true
	sess, err := dapOpener(dapCmd)(context.Background(), dap.LaunchArgs{ListenSeconds: 60})
	if err != nil || !sess.ReadOnly {
		t.Fatalf("--read-only: %+v %v", sess, err)
	}
	dapReadOnly = false
	t.Setenv("SAP_READ_ONLY", "true")
	sess, err = dapOpener(dapCmd)(context.Background(), dap.LaunchArgs{ListenSeconds: 60})
	if err != nil || !sess.ReadOnly {
		t.Fatalf("SAP_READ_ONLY: %+v %v", sess, err)
	}
}

// A listener that waits longer than a request may take would read as a
// failure every time; it is refused up front.
func TestDAPListenMustFitTheTimeout(t *testing.T) {
	dapConfigDir(t)
	if _, err := dapOpener(dapCmd)(context.Background(), dap.LaunchArgs{ListenSeconds: dapTimeout}); err == nil {
		t.Fatal("a listen as long as the request timeout should be refused")
	}
}

// vsp dap is a CLI command only.
func TestDAPIsACommand(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c == dapCmd {
			found = true
		}
	}
	if !found {
		t.Fatal("vsp dap is not registered")
	}
	if dapCmd.Flags().Lookup("read-only") == nil {
		t.Fatal("vsp dap has no --read-only")
	}
}
