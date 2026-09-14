package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// A system declared read_only was writable from every CLI subcommand, because
// only the MCP server ever handed a safety configuration to its client. The
// setting said one thing and the tool did another, which is the worst kind of
// safety feature.
func TestGetClientHonoursDeclaredSafety(t *testing.T) {
	client, err := getClient(&systemParams{
		URL: "https://sap.example:44300", User: "TESTER", Password: "secret",
		Client: "001", Language: "EN", ReadOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if safety := client.Safety(); safety == nil || !safety.ReadOnly {
		t.Fatalf("a read_only system must reach the client as read-only, got %+v", safety)
	}
}

// And a system with an allowed-package list must carry it too, or the list is
// advice rather than a restriction.
func TestGetClientCarriesAllowedPackages(t *testing.T) {
	client, err := getClient(&systemParams{
		URL: "https://sap.example:44300", User: "TESTER", Password: "secret",
		Client: "001", Language: "EN", AllowedPackages: []string{"Z*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if safety := client.Safety(); safety == nil || strings.Join(safety.AllowedPackages, ",") != "Z*" {
		t.Fatalf("allowed packages did not reach the client: %+v", safety)
	}
}

func TestGetClientUnrestrictedByDefault(t *testing.T) {
	client, err := getClient(&systemParams{
		URL: "https://sap.example:44300", User: "TESTER", Password: "secret",
		Client: "001", Language: "EN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if safety := client.Safety(); safety != nil && safety.ReadOnly {
		t.Fatal("an unrestricted system must not arrive read-only")
	}
}

func TestSplitListIgnoresBlanks(t *testing.T) {
	got := splitList(" Z*, ,Y* ")
	if strings.Join(got, "|") != "Z*|Y*" {
		t.Fatalf("got %v", got)
	}
}

// This goes through the actual root Cobra command, rather than constructing
// systemParams by hand. The fake server makes the boundary observable: a
// transportable write must be rejected before it reaches the network unless
// the resolved opt-in permits it.
func TestCLITransportableEditsReachSourceWriteWithSafePrecedence(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method)
		mu.Unlock()
		http.Error(w, "fake ADT rejects writes", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	tempDir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })
	t.Setenv("SAP_ALLOW_TRANSPORTABLE_EDITS", "")

	writeNamedConfig := func(readOnly bool, allowedPackages []string) {
		t.Helper()
		config := fmt.Sprintf(`{"default":"named","systems":{"named":{"url":%q,"user":"TESTUSER","password":"secret","allow_transportable_edits":true,"read_only":%t,"allowed_packages":%s}}}`,
			srv.URL, readOnly, jsonStringSlice(allowedPackages))
		if err := os.WriteFile(".vsp.json", []byte(config), 0600); err != nil {
			t.Fatal(err)
		}
	}
	resetRequests := func() { mu.Lock(); requests = nil; mu.Unlock() }
	requestCount := func() int { mu.Lock(); defer mu.Unlock(); return len(requests) }

	flag := rootCmd.PersistentFlags().Lookup("allow-transportable-edits")
	if flag == nil {
		t.Fatal("root persistent transportable-edits flag is missing")
	}
	oldFlagValue, oldFlagChanged, oldCfgValue, oldSystemName := flag.Value.String(), flag.Changed, cfg.AllowTransportableEdits, systemName
	t.Cleanup(func() {
		_ = flag.Value.Set(oldFlagValue)
		flag.Changed = oldFlagChanged
		cfg.AllowTransportableEdits = oldCfgValue
		systemName = oldSystemName
		rootCmd.SetArgs(nil)
	})

	runSourceWrite := func(args ...string) error {
		t.Helper()
		resetRequests()
		_ = flag.Value.Set("false")
		flag.Changed = false
		cfg.AllowTransportableEdits = false
		systemName = ""

		stdin, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.WriteString("REPORT ztest."); err != nil {
			t.Fatal(err)
		}
		_ = writer.Close()
		oldStdin := os.Stdin
		os.Stdin = stdin
		defer func() { os.Stdin = oldStdin; _ = stdin.Close() }()

		rootCmd.SetArgs(args)
		return rootCmd.Execute()
	}

	writeNamedConfig(false, nil)
	if err := runSourceWrite("source", "write", "PROG", "ZDEMO_FLAG", "--transport", "TR-EXAMPLE"); err == nil {
		t.Fatal("default-system source write unexpectedly succeeded against fake ADT")
	}
	if requestCount() == 0 {
		t.Fatal("default named system did not pass its configured opt-in to the client")
	}

	t.Setenv("SAP_ALLOW_TRANSPORTABLE_EDITS", "false")
	if err := runSourceWrite("-s", "named", "source", "write", "PROG", "ZDEMO_ENV", "--transport", "TR-EXAMPLE"); err == nil {
		t.Fatal("named-system source write unexpectedly succeeded against fake ADT")
	}
	if requestCount() != 0 {
		t.Fatal("SAP_ALLOW_TRANSPORTABLE_EDITS=false did not override named-system true before network I/O")
	}

	if err := runSourceWrite("-s", "named", "source", "write", "PROG", "ZDEMO_TRUE", "--transport", "TR-EXAMPLE", "--allow-transportable-edits=true"); err == nil {
		t.Fatal("explicit true source write unexpectedly succeeded against fake ADT")
	}
	if requestCount() == 0 {
		t.Fatal("subcommand did not accept and propagate explicit --allow-transportable-edits=true")
	}

	t.Setenv("SAP_ALLOW_TRANSPORTABLE_EDITS", "true")
	if err := runSourceWrite("-s", "named", "source", "write", "PROG", "ZDEMO_FALSE", "--transport", "TR-EXAMPLE", "--allow-transportable-edits=false"); err == nil {
		t.Fatal("explicit false source write unexpectedly succeeded against fake ADT")
	}
	if requestCount() != 0 {
		t.Fatal("explicit --allow-transportable-edits=false did not override environment true before network I/O")
	}

	writeNamedConfig(true, nil)
	if err := runSourceWrite("source", "write", "PROG", "ZDEMO_READ_ONLY", "--transport", "TR-EXAMPLE"); err == nil {
		t.Fatal("read-only source write unexpectedly succeeded")
	}
	if requestCount() != 0 {
		t.Fatal("transportable-edits opt-in bypassed read-only before network I/O")
	}

	writeNamedConfig(false, []string{"$TMP"})
	if err := runSourceWrite("source", "write", "PROG", "ZDEMO_PACKAGE", "--transport", "TR-EXAMPLE"); err == nil {
		t.Fatal("package-restricted source write unexpectedly succeeded")
	}
	if requestCount() != 0 {
		t.Fatal("transportable-edits opt-in bypassed the package gate before network I/O")
	}

	if err := os.Remove(".vsp.json"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SAP_URL", srv.URL)
	t.Setenv("SAP_USER", "TESTUSER")
	t.Setenv("SAP_PASSWORD", "secret")
	t.Setenv("SAP_ALLOW_TRANSPORTABLE_EDITS", "true")
	if err := runSourceWrite("source", "write", "PROG", "ZDEMO_ENV_ONLY", "--transport", "TR-EXAMPLE"); err == nil {
		t.Fatal("environment-only source write unexpectedly succeeded against fake ADT")
	}
	if requestCount() == 0 {
		t.Fatal("pure SAP_* environment did not pass the opt-in to the client")
	}

	t.Setenv("SAP_ALLOW_TRANSPORTABLE_EDITS", "not-a-bool")
	if err := runSourceWrite("source", "write", "PROG", "ZDEMO_INVALID", "--transport", "TR-EXAMPLE"); err == nil || !strings.Contains(err.Error(), "SAP_ALLOW_TRANSPORTABLE_EDITS must be true or false") {
		t.Fatalf("invalid environment value was not rejected clearly: %v", err)
	}
	if requestCount() != 0 {
		t.Fatal("invalid transportable-edits environment value reached fake ADT")
	}
}

func jsonStringSlice(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	return "[" + strings.Join(quoted, ",") + "]"
}
