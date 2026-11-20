package vsp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// uploadCLIEnv puts the CLI in an empty directory with a .vsp.json whose
// default system is a server that counts every request it gets -- the
// WebSocket upgrade to ZADT_VSP included -- and refuses them all. extra is
// spliced into the system's settings.
func uploadCLIEnv(t *testing.T, extra string) (hits func() int64, cofile, datafile string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", dir)
	for _, name := range envOnlyCLIVars {
		t.Setenv(name, "")
	}
	saved := systemName
	systemName = ""
	t.Cleanup(func() { systemName = saved })

	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	cfg := fmt.Sprintf(`{"default":"qassys","systems":{"qassys":{"url":%q,"client":"100","user":"TESTUSER","password":"secret"%s}}}`, srv.URL, extra)
	if err := os.WriteFile(".vsp.json", []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	cofile, datafile = filepath.Join(dir, "K900001.XYZ"), filepath.Join(dir, "R900001.XYZ")
	if err := os.WriteFile(cofile, []byte("TESTUSER K QAS 3 1 0 0 0 0 0 0 0 0\nXYZ.100 E 0000 20260101120000 h u\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(datafile, []byte{0, 1}, 0o600); err != nil {
		t.Fatal(err)
	}
	return n.Load, cofile, datafile
}

func runUpload(t *testing.T, cofile, datafile string) error {
	t.Helper()
	for k, v := range map[string]string{"cofile": cofile, "datafile": datafile} {
		if err := transportUploadCmd.Flags().Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = transportUploadCmd.Flags().Set("cofile", "")
		_ = transportUploadCmd.Flags().Set("datafile", "")
	})
	return transportUploadCmd.RunE(transportUploadCmd, nil)
}

// Every refusal comes before the system is contacted at all.
func TestTransportUploadCLI_RefusesBeforeContact(t *testing.T) {
	cases := map[string]struct {
		extra string
		env   map[string]string
		want  string
	}{
		"read_only system":           {`,"read_only":true,"enable_transports":true`, nil, "read-only"},
		"SAP_READ_ONLY":              {`,"enable_transports":true`, map[string]string{"SAP_READ_ONLY": "true"}, "read-only"},
		"transports not enabled":     {``, nil, "enable-transports"},
		"transport_read_only":        {`,"enable_transports":true,"transport_read_only":true`, nil, "transport read-only"},
		"outside allowed_transports": {`,"enable_transports":true,"allowed_transports":["ABCK*"]`, nil, "allowed"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			hits, co, da := uploadCLIEnv(t, c.extra)
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			err := runUpload(t, co, da)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want an error naming %q", err, c.want)
			}
			if n := hits(); n != 0 {
				t.Errorf("the system was contacted %d time(s) before the refusal", n)
			}
		})
	}
}

func TestTransportUploadCLI_BadFilesRefusedBeforeContact(t *testing.T) {
	hits, co, _ := uploadCLIEnv(t, `,"enable_transports":true`)
	if err := runUpload(t, co, filepath.Join(filepath.Dir(co), "R900002.XYZ")); err == nil || hits() != 0 {
		t.Errorf("unpaired files: %v, %d contact(s)", err, hits())
	}
	if err := runUpload(t, co, ""); err == nil || hits() != 0 {
		t.Errorf("one file: %v, %d contact(s)", err, hits())
	}
}

// With every gate open the command does go to the system (and here fails
// there, at the WebSocket).
func TestTransportUploadCLI_OpenGatesReachTheSystem(t *testing.T) {
	hits, co, da := uploadCLIEnv(t, `,"enable_transports":true,"allowed_transports":["XYZK9*"]`)
	err := runUpload(t, co, da)
	if err == nil || strings.Contains(err.Error(), "blocked") {
		t.Fatalf("got %v", err)
	}
	if hits() == 0 {
		t.Error("never contacted the system")
	}
}

func TestTransportDownloadCLI_RefusedUnderReadOnly(t *testing.T) {
	for name, c := range map[string]struct {
		extra string
		env   map[string]string
	}{
		"read_only system": {`,"read_only":true,"enable_transports":true`, nil},
		"SAP_READ_ONLY":    {`,"enable_transports":true`, map[string]string{"SAP_READ_ONLY": "true"}},
	} {
		t.Run(name, func(t *testing.T) {
			hits, _, _ := uploadCLIEnv(t, c.extra)
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			if err := transportDownloadCmd.Flags().Set("output", t.TempDir()); err != nil {
				t.Fatal(err)
			}
			err := transportDownloadCmd.RunE(transportDownloadCmd, []string{"XYZK900001"})
			if err == nil || !strings.Contains(err.Error(), "read-only") {
				t.Fatalf("got %v", err)
			}
			if n := hits(); n != 0 {
				t.Errorf("contacted the system %d time(s)", n)
			}
		})
	}
}

// The transport commands' WebSocket authenticates like the profile's HTTP
// client: a profile that signs in with a cookie_string (or cookie_file, or
// SSO) sends that session on the upgrade, not just the global cookies
// (PR #296 review).
func TestTransportWSUsesTheProfileSession(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", dir)
	for _, name := range envOnlyCLIVars {
		t.Setenv(name, "")
	}
	saved, savedCookies := systemName, cfg.Cookies
	systemName, cfg.Cookies = "", nil
	t.Cleanup(func() { systemName, cfg.Cookies = saved, savedCookies })

	var mu sync.Mutex
	var upgradeCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "" {
			mu.Lock()
			upgradeCookie = r.Header.Get("Cookie")
			mu.Unlock()
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	conf := fmt.Sprintf(`{"default":"qassys","systems":{"qassys":{"url":%q,"client":"100","cookie_string":"MYSAPSSO2=profile-session","enable_transports":true}}}`, srv.URL)
	if err := os.WriteFile(".vsp.json", []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = transportBufferCmd.RunE(transportBufferCmd, nil)
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(upgradeCookie, "MYSAPSSO2=profile-session") {
		t.Errorf("the WebSocket upgrade carried %q, not the profile's session", upgradeCookie)
	}
}
