package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/config"
)

// sncServerSetup writes a home ~/.vsp.json with an snc system "dev" and makes
// the snc checks believe they run on Windows.
func sncServerSetup(t *testing.T) {
	t.Helper()
	isolateServer(t)
	home, _ := os.Getwd() // isolateServer made the temp dir both HOME and cwd
	t.Cleanup(config.SetTrustedHome(home))
	exe := filepath.Join(home, "vsp.exe")
	t.Cleanup(config.SetSNCPlatform("windows",
		func() (string, error) { return exe, nil },
		func() string { return filepath.Join(home, "gx64krb5.dll") }))
	data := `{"systems": {"dev": {"snc": {"connection": "DEV - Development", "system": "DEV", "client": "100", "user": "testuser"}}}}`
	if err := os.WriteFile(filepath.Join(home, ".vsp.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	// isolateServer restores cfg afterwards but does not clear it; another
	// test's logon must not leak in.
	cfg.SystemName = "dev"
	cfg.BaseURL, cfg.Username, cfg.Password, cfg.TransportCmd = "", "", "", nil
	cfg.ReadOnly = false
}

// .mcp.json needs only ["-s", "dev"]: the server takes its logon, client,
// pin and read-only from the snc block.
func TestServer_NamedSNCSystemSuppliesTheLogon(t *testing.T) {
	sncServerSetup(t)
	if err := applyNamedSNCSystem(rootCmd, cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.TransportCmd) < 2 || cfg.TransportCmd[1] != config.SNCServeCommand {
		t.Fatalf("TransportCmd = %q", cfg.TransportCmd)
	}
	if cfg.BaseURL != config.SNCPlaceholderURL || cfg.Client != "100" || !cfg.ReadOnly {
		t.Errorf("url %q client %q read-only %v", cfg.BaseURL, cfg.Client, cfg.ReadOnly)
	}
	systems, _, _ := config.LoadSystems()
	if err := applyServerExpect(cfg, systems, false, false); err != nil {
		t.Fatal(err)
	}
	if cfg.Expect == nil || !strings.Contains(cfg.ExpectSource, "snc") {
		t.Errorf("no pin from the snc block: %+v from %q", cfg.Expect, cfg.ExpectSource)
	}
}

// A URL or password given as well is refused, not quietly overridden.
func TestServer_NamedSNCSystemRefusesAnotherLogon(t *testing.T) {
	sncServerSetup(t)
	cfg.BaseURL = "https://dev.example.local"
	cfg.Password = "secret"
	err := applyNamedSNCSystem(rootCmd, cfg)
	if err == nil || !strings.Contains(err.Error(), "URL") || !strings.Contains(err.Error(), "password") {
		t.Fatalf("err = %v", err)
	}
	if len(cfg.TransportCmd) != 0 {
		t.Error("the transport command was set despite the refusal")
	}
}

// A system without an snc block leaves the server's logon alone.
func TestServer_NamedSystemWithoutSNCUntouched(t *testing.T) {
	sncServerSetup(t)
	cfg.SystemName = "other"
	cfg.BaseURL = "https://dev.example.local"
	if err := applyNamedSNCSystem(rootCmd, cfg); err != nil || len(cfg.TransportCmd) != 0 || cfg.BaseURL != "https://dev.example.local" {
		t.Fatalf("err %v, transport %q, url %q", err, cfg.TransportCmd, cfg.BaseURL)
	}
}
