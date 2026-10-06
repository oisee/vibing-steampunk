package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

// withShellTransportCmd makes SAP_TRANSPORT_CMD look as if the process
// environment set it (inShell) or as if a .env file did (not inShell).
func withShellTransportCmd(t *testing.T, v string, inShell bool) {
	t.Helper()
	savedV, savedIn := transportCmdShellEnv, transportCmdInShellEnv
	t.Cleanup(func() { transportCmdShellEnv, transportCmdInShellEnv = savedV, savedIn })
	t.Setenv("SAP_TRANSPORT_CMD", v)
	if inShell {
		transportCmdShellEnv, transportCmdInShellEnv = v, true
	} else {
		transportCmdShellEnv, transportCmdInShellEnv = "", false
	}
}

func TestParseTransportCmdEnv_MalformedRefused(t *testing.T) {
	for _, raw := range []string{
		`helper.exe --flag`,
		`{"cmd":"helper"}`,
		`[]`,
		`null`,
		`[""]`,
		`["helper", " "]`,
		`[1, 2]`,
		`["helper"`,
	} {
		if argv, err := parseTransportCmdEnv(raw); err == nil {
			t.Errorf("SAP_TRANSPORT_CMD=%s accepted as %q", raw, argv)
		} else if !strings.Contains(err.Error(), "JSON array of non-empty strings") {
			t.Errorf("SAP_TRANSPORT_CMD=%s: unclear error %v", raw, err)
		}
	}
	argv, err := parseTransportCmdEnv(`["C:\\tools\\helper.exe", "--profile", "a b"]`)
	if err != nil || len(argv) != 3 || argv[0] != `C:\tools\helper.exe` || argv[2] != "a b" {
		t.Fatalf("valid array: %q, %v", argv, err)
	}
}

func TestResolveTransportCmd_Sources(t *testing.T) {
	bare := &cobra.Command{Use: "x"}

	withShellTransportCmd(t, `["helper","--x"]`, true)
	argv, err := resolveTransportCmd(bare)
	if err != nil || strings.Join(argv, " ") != "helper --x" {
		t.Fatalf("from the environment: %q, %v", argv, err)
	}

	withShellTransportCmd(t, `["helper"]`, false)
	if _, err := resolveTransportCmd(bare); err == nil || !strings.Contains(err.Error(), ".env") {
		t.Fatalf("SAP_TRANSPORT_CMD from .env: want a refusal, got %v", err)
	}

	withShellTransportCmd(t, `not json`, true)
	if _, err := resolveTransportCmd(bare); err == nil {
		t.Fatal("malformed SAP_TRANSPORT_CMD accepted")
	}

	withShellTransportCmd(t, "", true)
	flagged := &cobra.Command{Use: "y"}
	flagged.Flags().String("transport-cmd", "", "")
	flagged.Flags().StringArray("transport-arg", nil, "")
	flagged.Flags().Set("transport-cmd", "/opt/helper")
	flagged.Flags().Set("transport-arg", "--a")
	flagged.Flags().Set("transport-arg", "b,c")
	argv, err = resolveTransportCmd(flagged)
	if err != nil || len(argv) != 3 || argv[2] != "b,c" {
		t.Fatalf("flags: %q, %v", argv, err)
	}

	argsOnly := &cobra.Command{Use: "z"}
	argsOnly.Flags().String("transport-cmd", "", "")
	argsOnly.Flags().StringArray("transport-arg", nil, "")
	argsOnly.Flags().Set("transport-arg", "--a")
	if _, err := resolveTransportCmd(argsOnly); err == nil {
		t.Fatal("--transport-arg without --transport-cmd accepted")
	}
}

// isolateServer prepares rootCmd for one execution in a clean directory.
func isolateServer(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	for _, name := range envOnlyCLIVars {
		t.Setenv(name, "")
	}
	for _, name := range []string{"SAP_EXPECT", "SAP_BROWSER_AUTH", "SAP_SAML_AUTH", "SAP_USERNAME", "SAP_PASS"} {
		t.Setenv(name, "")
	}
	restoreCommandFlags(t)
	saved := *cfg
	savedErr := transportCmdErr
	t.Cleanup(func() { *cfg = saved; transportCmdErr = savedErr; rootCmd.SetArgs(nil) })
	withExpectFlag(t, "")
}

func TestServer_TransportCmdWithPasswordRefused(t *testing.T) {
	isolateServer(t)
	withShellTransportCmd(t, "", true)
	rootCmd.SetArgs([]string{"--url", "https://sidecar.invalid", "--transport-cmd", "/nonexistent/helper",
		"--user", "TESTUSER", "--password", "x"})
	err := rootCmd.Execute()
	if !errors.Is(err, adt.ErrTransportCmdAuth) {
		t.Fatalf("want the transport-cmd/credentials refusal, got %v", err)
	}
	if !strings.Contains(err.Error(), "remove user/password/cookies") {
		t.Errorf("error does not say what to do: %v", err)
	}
}

func TestServer_TransportCmdWithEnvPasswordRefused(t *testing.T) {
	isolateServer(t)
	withShellTransportCmd(t, `["/nonexistent/helper"]`, true)
	t.Setenv("SAP_USER", "TESTUSER")
	t.Setenv("SAP_PASSWORD", "x")
	rootCmd.SetArgs([]string{"--url", "https://sidecar.invalid"})
	if err := rootCmd.Execute(); !errors.Is(err, adt.ErrTransportCmdAuth) {
		t.Fatalf("want the transport-cmd/credentials refusal, got %v", err)
	}
}

func TestServer_MalformedTransportCmdEnvRefused(t *testing.T) {
	isolateServer(t)
	withShellTransportCmd(t, `/usr/bin/helper --flag`, true)
	rootCmd.SetArgs([]string{"--url", "https://sidecar.invalid"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "SAP_TRANSPORT_CMD must be a JSON array") {
		t.Fatalf("want a malformed SAP_TRANSPORT_CMD refusal, got %v", err)
	}
}

// A transport command is a complete logon: processCookieAuth must not ask
// for another method.
func TestProcessCookieAuth_TransportCmdIsEnough(t *testing.T) {
	isolateServer(t)
	cfg.Username, cfg.Password, cfg.Cookies = "", "", nil
	cfg.TransportCmd = []string{"/nonexistent/helper"}
	transportCmdErr = nil
	if err := processCookieAuth(rootCmd); err != nil {
		t.Fatalf("transport command alone: %v", err)
	}
	cfg.Cookies = map[string]string{"MYSAPSSO2": "x"}
	if err := processCookieAuth(rootCmd); !errors.Is(err, adt.ErrTransportCmdAuth) {
		t.Fatalf("transport command with cookies: %v", err)
	}
}

const cliTransportCmdSystems = `{"default": "side", "systems": {"side": {"url": "https://sidecar.invalid", "transport_cmd": ["/opt/tools/adt-helper", "--x"]}}}`

func TestCLI_TransportCmdFromHomeConfig(t *testing.T) {
	isolateServer(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.WriteFile(filepath.Join(home, ".vsp.json"), []byte(cliTransportCmdSystems), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{Use: "x"}
	params, err := resolveSystemParams(cmd)
	if err != nil {
		t.Fatalf("resolveSystemParams: %v", err)
	}
	if strings.Join(params.TransportCmd, " ") != "/opt/tools/adt-helper --x" {
		t.Fatalf("TransportCmd = %q", params.TransportCmd)
	}
	if _, err := buildClient(params); err != nil {
		t.Fatalf("buildClient: %v", err)
	}
	params.Password = "x"
	if _, err := buildClient(params); !errors.Is(err, adt.ErrTransportCmdAuth) {
		t.Fatalf("buildClient with a password: %v", err)
	}
}

func TestCLI_TransportCmdFromWorkingDirectoryRefused(t *testing.T) {
	isolateServer(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// isolateServer made a fresh working directory current; the file goes there.
	if err := os.WriteFile(".vsp.json", []byte(cliTransportCmdSystems), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := resolveSystemParams(&cobra.Command{Use: "x"})
	if err == nil || !strings.Contains(err.Error(), "must not be able to make vsp run a program") {
		t.Fatalf("want a refusal of transport_cmd from ./.vsp.json, got %v", err)
	}
}
