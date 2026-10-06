package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const transportCmdSystems = `{
  "default": "side",
  "systems": {
    "side": {"url": "https://sidecar.invalid", "client": "001", "transport_cmd": ["helper", "--profile", "x"]}
  }
}`

// isolateHome points HOME (and USERPROFILE) at a fresh directory and makes a
// separate, fresh working directory current.
func isolateHome(t *testing.T) (home, work string) {
	t.Helper()
	home = t.TempDir()
	work = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(work)
	return home, work
}

func TestTransportCmd_RefusedFromWorkingDirectory(t *testing.T) {
	for _, rel := range []string{".vsp.json", filepath.Join(".vsp", "systems.json")} {
		t.Run(rel, func(t *testing.T) {
			_, work := isolateHome(t)
			p := filepath.Join(work, rel)
			os.MkdirAll(filepath.Dir(p), 0o700)
			if err := os.WriteFile(p, []byte(transportCmdSystems), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, path, err := LoadSystems()
			if err != nil || cfg == nil {
				t.Fatalf("LoadSystems: %v", err)
			}
			if IsHomeConfigPath(path) {
				t.Fatalf("%s taken for a home file", path)
			}
			_, err = cfg.GetSystem("side")
			if err == nil {
				t.Fatal("transport_cmd from a working-directory file was accepted")
			}
			if !strings.Contains(err.Error(), "must not be able to make vsp run a program") {
				t.Errorf("refusal does not say why: %v", err)
			}
		})
	}
}

func TestTransportCmd_AcceptedFromHome(t *testing.T) {
	for _, rel := range []string{".vsp.json", filepath.Join(".vsp", "systems.json")} {
		t.Run(rel, func(t *testing.T) {
			home, _ := isolateHome(t)
			p := filepath.Join(home, rel)
			os.MkdirAll(filepath.Dir(p), 0o700)
			if err := os.WriteFile(p, []byte(transportCmdSystems), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("VSP_SIDE_PASSWORD", "secret") // must not be picked up
			cfg, _, err := LoadSystems()
			if err != nil || cfg == nil {
				t.Fatalf("LoadSystems: %v", err)
			}
			sys, err := cfg.GetSystem("side")
			if err != nil {
				t.Fatalf("transport_cmd from %s refused: %v", p, err)
			}
			if got := strings.Join(sys.TransportCmd, " "); got != "helper --profile x" {
				t.Errorf("TransportCmd = %q", got)
			}
			if sys.Password != "" {
				t.Error("a password from the environment was attached to a transport-command system")
			}
		})
	}
}

func TestTransportCmd_RefusedWithCredentials(t *testing.T) {
	home, _ := isolateHome(t)
	p := filepath.Join(home, ".vsp.json")
	data := `{"systems": {"side": {"url": "https://sidecar.invalid", "user": "TESTUSER", "transport_cmd": ["helper"]}}}`
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadSystems()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.GetSystem("side"); err == nil || !strings.Contains(err.Error(), "carries its own authentication") {
		t.Fatalf("want a refusal of transport_cmd with a user, got %v", err)
	}
}

func TestTransportCmd_InMemoryConfigRefused(t *testing.T) {
	cfg := &SystemsConfig{Systems: map[string]SystemConfig{"side": {URL: "https://sidecar.invalid", TransportCmd: []string{"helper"}}}}
	if _, err := cfg.GetSystem("side"); err == nil {
		t.Fatal("a config not read from the home directory ran a transport command")
	}
}
