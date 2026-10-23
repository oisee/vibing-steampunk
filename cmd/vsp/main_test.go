package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestMain cuts the package off from every system configuration a developer
// machine has. resolveSystemParams falls back to the default system in
// .vsp.json when no -s is given, and that default outranks the SAP_URL a test
// points at its httptest server: without this, the tests would send their
// requests to a live SAP system.
//
//   - HOME and USERPROFILE point at an empty temporary directory, so
//     ~/.vsp.json and ~/.vsp/systems.json are not found.
//   - A .vsp.json or .vsp/systems.json in the package directory is read before
//     the home one, so its presence stops the run.
//   - SAP_* and VSP_* from the shell or from ./.env (loaded by init, before
//     this runs) are cleared; tests set what they need themselves.
//
// The one exception is the child process TestCLI_DotEnvHomeCannotVouchForProjectConfig
// starts: it runs in a temporary directory whose .env set HOME to ".", which
// is what it checks. It qualifies only outside the package directory, so the
// marker and HOME=. from the shell or from cmd/vsp/.env do not turn the
// isolation off.
func TestMain(m *testing.M) {
	if os.Getenv("VSP_DOTENV_HOME_CHILD") == "1" && os.Getenv("HOME") == "." && !inPackageDir() {
		os.Exit(m.Run())
	}
	for _, f := range []string{".vsp.json", ".vsp/systems.json"} {
		if _, err := os.Stat(f); err == nil {
			fmt.Fprintf(os.Stderr, "TestMain: cmd/vsp/%s would be read as the systems config and could send the tests to a live system; remove it\n", f)
			os.Exit(1)
		}
	}
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "SAP_") || strings.HasPrefix(k, "VSP_") {
			os.Unsetenv(k)
		}
	}
	home, err := os.MkdirTemp("", "vsp-test-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain:", err)
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

// inPackageDir reports whether the working directory is cmd/vsp itself, where
// go test runs the package; the .env child runs in a temporary directory.
func inPackageDir() bool {
	_, err := os.Stat("main_test.go")
	return err == nil
}
