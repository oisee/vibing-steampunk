package main

import (
	"fmt"
	"os"
	"testing"
)

// TestMain gives the package an empty home directory. resolveSystemParams
// falls back to the default system in ~/.vsp.json when no -s is given, and that
// default outranks the SAP_URL a test points at its httptest server: on a
// developer machine the tests would otherwise send their requests to a live
// SAP system. Tests that need a home of their own still set HOME themselves.
// The .env child (TestDotEnvHomeChild) is left alone: init has already taken
// HOME from its .env, which is what it tests, and that home is a temp dir.
func TestMain(m *testing.M) {
	if os.Getenv("VSP_DOTENV_HOME_CHILD") == "1" {
		os.Exit(m.Run())
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
