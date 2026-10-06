package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	// existingPackage, when set, makes the fake ADT report ZDEMO_PACKAGE as an
	// existing program in that package, so the update path's package gate
	// (which resolves the object's real package, #230) has something to judge.
	existingPackage := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		pkg := existingPackage
		mu.Unlock()
		if pkg != "" && r.Method == http.MethodGet {
			w.Header().Set("X-CSRF-Token", "test-token")
			switch {
			case strings.Contains(r.URL.Path, "informationsystem/search"):
				_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/zdemo_package" adtcore:type="PROG/P" adtcore:name="ZDEMO_PACKAGE" adtcore:packageName=%q/>
</adtcore:objectReferences>`, pkg)
				return
			case strings.HasSuffix(r.URL.Path, "/programs/programs/ZDEMO_PACKAGE/source/main"):
				w.Header().Set("Content-Type", "text/plain")
				_, _ = fmt.Fprint(w, "REPORT zdemo_package.")
				return
			}
		}
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
	t.Setenv("HOME", tempDir) // no ~/.vsp.json either
	for _, name := range envOnlyCLIVars {
		t.Setenv(name, "")
	}

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

	restoreCommandFlags(t)
	flag := rootCmd.PersistentFlags().Lookup("allow-transportable-edits")
	if flag == nil {
		t.Fatal("root persistent transportable-edits flag is missing")
	}
	oldCfgValue, oldSystemName := cfg.AllowTransportableEdits, systemName
	t.Cleanup(func() {
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

	// Since #230 the package gate for an existing object judges the object's
	// real package, which takes a read (existence probe, package lookup)
	// before it can decide. So reads are expected here; what must not happen
	// is a lock or a write for an object outside the allowed packages.
	writeNamedConfig(false, []string{"$TMP"})
	mu.Lock()
	existingPackage = "ZPROD"
	mu.Unlock()
	err = runSourceWrite("source", "write", "PROG", "ZDEMO_PACKAGE", "--transport", "TR-EXAMPLE")
	mu.Lock()
	existingPackage = ""
	seen := append([]string(nil), requests...)
	mu.Unlock()
	if err == nil || !strings.Contains(err.Error(), "ZPROD") {
		t.Fatalf("an object in a package outside allowed_packages must be refused by its real package, got %v (requests %v)", err, seen)
	}
	for _, req := range seen {
		if !strings.HasPrefix(req, http.MethodGet+" ") || strings.Contains(req, "_action=LOCK") {
			t.Fatalf("transportable-edits opt-in let a lock or write past the package gate: %v", seen)
		}
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

// envOnlyCLIVars are the variables the env-only branch of resolveSystemParams
// (and getClient) reads, cleared by runEnvOnlyCLI before it applies its own.
var envOnlyCLIVars = []string{
	"SAP_URL", "SAP_USER", "SAP_PASSWORD", "SAP_CLIENT", "SAP_LANGUAGE", "SAP_INSECURE",
	"SAP_COOKIE_FILE", "SAP_COOKIE_STRING", "SAP_SSO", "SAP_SSO_SYSTEM",
	"SAP_READ_ONLY", "SAP_ALLOWED_PACKAGES", "SAP_BLOCK_FREE_SQL",
	"SAP_ALLOW_TRANSPORTABLE_EDITS", "SAP_ALLOWED_TRANSPORTS", "SAP_ENABLE_TRANSPORTS",
	"SAP_TRANSPORT_READ_ONLY", "SAP_TRANSPORT_CHOICE", "VSP_TRANSPORT_ATTRIBUTE",
	"VSP_CACHE", "VSP_CACHE_PATH", "VSP_CACHE_TTL", "SAP_TRANSPORT_CMD",
}

// restoreCommandFlags snapshots every flag on the global command tree and
// restores it when the test ends. Executing rootCmd parses into those globals,
// so without this a `--transport TR-EXAMPLE` from one test is still set on
// sourceWriteCmd when a later test calls runSourceWrite directly.
func restoreCommandFlags(t *testing.T) {
	t.Helper()
	type saved struct {
		flag    *pflag.Flag
		value   string
		slice   []string
		isSlice bool
		changed bool
	}
	var all []saved
	seen := map[*pflag.Flag]bool{}
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, set := range []*pflag.FlagSet{cmd.PersistentFlags(), cmd.Flags()} {
			set.VisitAll(func(f *pflag.Flag) {
				if seen[f] {
					return
				}
				seen[f] = true
				s := saved{flag: f, value: f.Value.String(), changed: f.Changed}
				if sv, ok := f.Value.(pflag.SliceValue); ok {
					s.isSlice, s.slice = true, append([]string(nil), sv.GetSlice()...)
				}
				all = append(all, s)
			})
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)
	t.Cleanup(func() {
		for _, s := range all {
			if s.isSlice {
				_ = s.flag.Value.(pflag.SliceValue).Replace(s.slice)
			} else {
				_ = s.flag.Value.Set(s.value)
			}
			s.flag.Changed = s.changed
		}
	})
}

// runEnvOnlyCLI runs a vsp command line through the root command with
// no systems config anywhere, so resolveSystemParams takes the SAP_* branch.
// It returns the number of requests the fake ADT server saw, and the error.
func runEnvOnlyCLI(t *testing.T, env map[string]string, args ...string) (int, error) {
	t.Helper()
	var mu sync.Mutex
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
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
	t.Setenv("HOME", tempDir) // no ~/.vsp.json either
	// Start from a clean SAP_*/VSP_* slate, so a developer's shell (a read-only
	// or package-restricted system, a cache, cookies) cannot change the result.
	for _, name := range envOnlyCLIVars {
		t.Setenv(name, "")
	}
	oldLastClient := lastClient
	t.Cleanup(func() { lastClient = oldLastClient })
	t.Setenv("SAP_URL", srv.URL)
	t.Setenv("SAP_USER", "TESTUSER")
	t.Setenv("SAP_PASSWORD", "secret")
	for k, v := range env {
		t.Setenv(k, v)
	}

	restoreCommandFlags(t)
	flag := rootCmd.PersistentFlags().Lookup("allow-transportable-edits")
	if flag == nil {
		t.Fatal("root persistent transportable-edits flag is missing")
	}
	oldCfgValue, oldSystemName := cfg.AllowTransportableEdits, systemName
	t.Cleanup(func() {
		cfg.AllowTransportableEdits = oldCfgValue
		systemName = oldSystemName
		rootCmd.SetArgs(nil)
	})
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
	err = rootCmd.Execute()
	mu.Lock()
	defer mu.Unlock()
	return requests, err
}

// With only SAP_* variables set, SAP_ALLOW_TRANSPORTABLE_EDITS=true must not
// open every transport: SAP_ALLOWED_TRANSPORTS still decides which ones.
func TestEnvOnlyTransportableEditHonoursAllowedTransports(t *testing.T) {
	env := map[string]string{
		"SAP_ALLOW_TRANSPORTABLE_EDITS": "true",
		"SAP_ALLOWED_TRANSPORTS":        "TR-EXAMPLE",
	}

	requests, err := runEnvOnlyCLI(t, env, "source", "write", "PROG", "ZDEMO_OUT", "--transport", "TR-OTHER")
	if err == nil || !strings.Contains(err.Error(), "allowed transports") {
		t.Fatalf("a transport outside SAP_ALLOWED_TRANSPORTS must be refused, got %v", err)
	}
	if requests != 0 {
		t.Fatalf("the refused write reached the server (%d requests)", requests)
	}

	requests, err = runEnvOnlyCLI(t, env, "source", "write", "PROG", "ZDEMO_IN", "--transport", "TR-EXAMPLE")
	if err == nil {
		t.Fatal("source write unexpectedly succeeded against fake ADT")
	}
	if requests == 0 {
		t.Fatalf("a listed transport must pass the safety check, got %v", err)
	}

	env["SAP_ENABLE_TRANSPORTS"] = "true"
	env["SAP_TRANSPORT_READ_ONLY"] = "true"
	env["SAP_TRANSPORT_CHOICE"] = "off"
	lastClient = nil
	if _, err := runEnvOnlyCLI(t, env, "source", "write", "PROG", "ZDEMO_IN", "--transport", "TR-EXAMPLE"); err == nil {
		t.Fatal("source write unexpectedly succeeded against fake ADT")
	}
	if lastClient == nil || lastClient.Safety() == nil {
		t.Fatal("source write did not build a client with a safety configuration")
	}
	if s := lastClient.Safety(); !s.EnableTransports || !s.TransportReadOnly || s.TransportChoice != "off" {
		t.Fatalf("SAP_ENABLE_TRANSPORTS, SAP_TRANSPORT_READ_ONLY and SAP_TRANSPORT_CHOICE must reach the client, got %+v", s)
	}
}

// SAP_BLOCK_FREE_SQL is read for a named system; an env-only config must
// honour it too, or `vsp query` runs arbitrary SQL the variable forbids.
func TestEnvOnlyConfigHonoursBlockFreeSQL(t *testing.T) {
	requests, err := runEnvOnlyCLI(t, map[string]string{"SAP_BLOCK_FREE_SQL": "true"}, "query", "T000")
	if err == nil || !strings.Contains(err.Error(), "blocked by safety") {
		t.Fatalf("free SQL must be refused by the safety configuration when SAP_BLOCK_FREE_SQL=true, got %v", err)
	}
	if requests != 0 {
		t.Fatalf("SAP_BLOCK_FREE_SQL=true did not stop the query before network I/O (%d requests): %v", requests, err)
	}
}
