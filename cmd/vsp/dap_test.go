package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/oisee/vibing-steampunk/internal/dap"
)

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
