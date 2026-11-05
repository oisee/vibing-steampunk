package main

import (
	"os"
	"testing"
)

// TestMain keeps a developer's own keychain login out of the tests: the
// env-only tests set SAP_USER and SAP_PASSWORD, and an exported
// SAP_CLIENT_CERT_CN or SAP_CLIENT_CERT_ISSUER in the shell would otherwise
// win and send them to the keychain. A test that wants one sets it itself.
func TestMain(m *testing.M) {
	os.Unsetenv("SAP_CLIENT_CERT_CN")
	os.Unsetenv("SAP_CLIENT_CERT_ISSUER")
	os.Exit(m.Run())
}
