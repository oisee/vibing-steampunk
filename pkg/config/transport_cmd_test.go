package config

import (
	"os"
	"path/filepath"
	"runtime"
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
	t.Cleanup(SetTrustedHome(home))
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

// A project .env with HOME=. fills HOME when the process had none. The
// home-directory rule must go by what the process started with, so the
// working directory's .vsp.json never passes for ~/.vsp.json.
func TestTransportCmd_HomeFromDotEnvDoesNotCount(t *testing.T) {
	_, work := isolateHome(t)
	t.Cleanup(SetTrustedHome("")) // HOME was unset at start-up
	t.Setenv("HOME", ".")         // ...and .env set it to the project
	t.Setenv("USERPROFILE", ".")
	if err := os.WriteFile(filepath.Join(work, ".vsp.json"), []byte(transportCmdSystems), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadSystems()
	if err != nil || cfg == nil {
		t.Fatalf("LoadSystems: %v", err)
	}
	if _, err := cfg.GetSystem("side"); err == nil {
		t.Fatal("transport_cmd from ./.vsp.json accepted with HOME=. from .env")
	}
}

func TestTransportCmd_RelativeHomeRefused(t *testing.T) {
	_, work := isolateHome(t)
	t.Cleanup(SetTrustedHome("."))
	t.Setenv("HOME", ".")
	t.Setenv("USERPROFILE", ".")
	if err := os.WriteFile(filepath.Join(work, ".vsp.json"), []byte(transportCmdSystems), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadSystems()
	if err != nil || cfg == nil {
		t.Fatalf("LoadSystems: %v", err)
	}
	_, err = cfg.GetSystem("side")
	if err == nil || !strings.Contains(err.Error(), "not an absolute path") {
		t.Fatalf("relative home: want a refusal, got %v", err)
	}
}

func TestTransportCmd_GroupWritableHomeFileRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not checked on Windows")
	}
	home, _ := isolateHome(t)
	p := filepath.Join(home, ".vsp.json")
	if err := os.WriteFile(p, []byte(transportCmdSystems), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o620); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadSystems()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.GetSystem("side"); err == nil || !strings.Contains(err.Error(), "writable by group or others") {
		t.Fatalf("group-writable ~/.vsp.json: want a refusal, got %v", err)
	}
}

// ~/.vsp.json may itself be a symlink (dotfile managers); it still counts.
func TestTransportCmd_SymlinkedHomeFileAccepted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	home, _ := isolateHome(t)
	target := filepath.Join(t.TempDir(), "dotfiles-vsp.json")
	if err := os.WriteFile(target, []byte(transportCmdSystems), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, ".vsp.json")); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadSystems()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.GetSystem("side"); err != nil {
		t.Fatalf("symlinked ~/.vsp.json refused: %v", err)
	}
}
