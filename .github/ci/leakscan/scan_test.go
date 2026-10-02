package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Every test value is assembled at run time: this file is scanned like any
// other, and a literal private address or cookie in it would be a hit.

func ip(a, b, c, d string) string { return strings.Join([]string{a, b, c, d}, ".") }

func utf16le(s string) []byte {
	out := make([]byte, 0, 2*len(s))
	for _, c := range []byte(s) {
		out = append(out, c, 0)
	}
	return out
}

func hasHit(hits []Hit, class, view string) *Hit {
	for i, h := range hits {
		if h.Class == class && (view == "" || h.View == view) {
			return &hits[i]
		}
	}
	return nil
}

var host = Identifier{Class: "host", Value: "sapbox.corp.invalid"}
var user = Identifier{Class: "user", Value: "JDOEPRIVATE"}

func TestTextView(t *testing.T) {
	data := []byte("line one\nurl: https://SAPBOX.corp.invalid:44300/sap\n")
	h := hasHit(scanBytes("f", data, []Identifier{host}), "identifier/host", "text")
	if h == nil || h.Line != 2 {
		t.Fatalf("want identifier/host on line 2 in the text view, got %+v", h)
	}
}

func TestUTF16LEHiddenHost(t *testing.T) {
	for _, prefix := range [][]byte{{}, {0xFF}} { // both alignments
		data := append([]byte("PK\x03\x04 header\n\n"), prefix...)
		data = append(data, utf16le("Server=sapbox.corp.invalid;Instance=00")...)
		if bytes.Contains(data, []byte(host.Value)) {
			t.Fatal("the fixture must not spell the host in ASCII")
		}
		hits := scanBytes("logon.dat", data, []Identifier{host})
		h := hasHit(hits, "identifier/host", "utf-16le")
		if h == nil {
			t.Fatalf("prefix %d: UTF-16LE host not found: %+v", len(prefix), hits)
		}
		if h.Line != 3 {
			t.Errorf("prefix %d: line %d, want 3", len(prefix), h.Line)
		}
	}
}

func TestBase64HiddenUser(t *testing.T) {
	enc := base64.StdEncoding.EncodeToString([]byte("sap-user=jdoeprivate&sap-client=100"))
	data := []byte("first\nsecond\nfixture := \"" + enc + "\"\n")
	hits := scanBytes("x_test.go", data, []Identifier{user})
	h := hasHit(hits, "identifier/user", "base64")
	if h == nil || h.Line != 3 {
		t.Fatalf("want identifier/user in the base64 view on line 3, got %+v", hits)
	}
	// The same block starting mid-token (after a path slash) still decodes.
	data = []byte("/" + enc)
	if hasHit(scanBytes("f", data, []Identifier{user}), "identifier/user", "base64") == nil {
		t.Error("misaligned base64 block not decoded")
	}
	// And UTF-16LE inside base64.
	data = []byte(base64.StdEncoding.EncodeToString(utf16le("User=JDOEPRIVATE;")))
	if hasHit(scanBytes("f", data, []Identifier{user}), "identifier/user", "base64+utf-16le") == nil {
		t.Error("UTF-16LE inside base64 not found")
	}
}

func TestHexHiddenIP(t *testing.T) {
	addr := ip("192", "168", "77", "12")
	data := []byte("a\npayload " + hex.EncodeToString([]byte("connect "+addr+":3300")) + "\n")
	hits := scanBytes("dump.txt", data, nil)
	h := hasHit(hits, "private-ip", "hex")
	if h == nil || h.Line != 2 {
		t.Fatalf("want private-ip in the hex view on line 2, got %+v", hits)
	}
	// An address only in a hex-hidden identifier list entry is found too.
	if hasHit(scanBytes("f", data, []Identifier{{Class: "ip", Value: addr}}), "identifier/ip", "hex") == nil {
		t.Error("hex-hidden listed IP not found")
	}
}

func TestPackedAddressInGUID(t *testing.T) {
	// A session GUID whose last bytes are a client's address, never spelled out.
	guid := "6F1A2B3C4D5E6F70" + "1122" + "3344" + "C0A84D0C"
	hits := scanBytes("trace.json", []byte(`{"guid":"`+guid+`"}`), nil)
	h := hasHit(hits, "packed-private-ip", "hex")
	if h == nil || h.value != ip("192", "168", "77", "12") {
		t.Fatalf("packed address not found: %+v", hits)
	}
	// A 40-digit run is a commit hash: no packed check there.
	sha := "0123c0a84d0c" + strings.Repeat("ab", 14)
	if len(sha) != 40 {
		t.Fatal(len(sha))
	}
	if hasHit(scanBytes("CHANGELOG.md", []byte(sha), nil), "packed-private-ip", "") != nil {
		t.Error("a SHA-1 was read as a packed address")
	}
}

func TestIdentifierBoundaries(t *testing.T) {
	for text, want := range map[string]bool{
		"user JDOEPRIVATE logged on": true,
		"JDOEPRIVATE":                true,
		"sap_user=jdoeprivate;":      true,
		"JDOEPRIVATES":               false,
		"XJDOEPRIVATE":               false,
	} {
		got := hasHit(scanBytes("f", []byte(text), []Identifier{user}), "identifier/user", "") != nil
		if got != want {
			t.Errorf("%q: hit=%v, want %v", text, got, want)
		}
	}
	// A short identifier (a SID) is matched in text, not in decoded bytes.
	sid := Identifier{Class: "sid", Value: "QX7"}
	if hasHit(scanBytes("f", []byte("system QX7 client"), []Identifier{sid}), "identifier/sid", "text") == nil {
		t.Error("SID not matched in text")
	}
	enc := base64.StdEncoding.EncodeToString([]byte("--QX7--QX7--"))
	if h := hasHit(scanBytes("f", []byte(enc), []Identifier{sid}), "identifier/sid", "base64"); h != nil {
		t.Error("a 3-character identifier matched in a decoded view")
	}
}

func TestGenericPatterns(t *testing.T) {
	cookie := "SAP_SESSIONID_" + "QX7_100"
	token := "k3J9fQ2xLw8vT1mZ0aB4cD"
	cases := []struct {
		text  string
		class string
		want  bool
	}{
		{"Cookie: " + cookie + "=" + token + "%3d; path=/", "sap-session-cookie", true},
		{"Cookie: " + cookie + "=<your-session-id>", "sap-session-cookie", false},
		{"Cookie: " + cookie + "=", "sap-session-cookie", false},
		{"MYSAP" + "SSO2=" + token + token, "sap-sso-cookie", true},
		{"MYSAP" + "SSO2=...", "sap-sso-cookie", false},
		{"Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("JDOE:Wk7#pq2!")), "basic-auth", true},
		{"Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass")), "basic-auth", false},
		{`req.Header.Set("Authorization", "Basic "+enc)`, "basic-auth", false},
		{"X-CSRF-" + "Token: " + token, "csrf-token", true},
		{"X-CSRF-" + "Token: Fetch", "csrf-token", false},
		{`"x-csrf-` + `token": "test-csrf-token-0001"`, "csrf-token", false},
		{`{"SAP_` + `PASSWORD": "Wk7pq2Zr9"}`, "config-password", true},
		{`{"pass` + `word": "your-password"}`, "config-password", false},
		{`{"pass` + `word": "pass"}`, "config-password", false},
		{"host " + ip("10", "20", "30", "40"), "private-ip", true},
		{"host " + ip("172", "20", "1", "5") + ":8000", "private-ip", true},
		{"host " + ip("172", "32", "1", "5"), "private-ip", false},
		{"bad " + ip("192", "168", "1", "300"), "private-ip", false},
		{"version 1." + ip("10", "0", "0", "1"), "private-ip", false},
		{"oid " + ip("10", "1", "2", "3") + ".4", "private-ip", false},
	}
	for _, c := range cases {
		got := hasHit(scanBytes("f", []byte(c.text), nil), c.class, "") != nil
		if got != c.want {
			t.Errorf("%q: %s hit=%v, want %v", c.text, c.class, got, c.want)
		}
	}
}

func TestParseIdentifiers(t *testing.T) {
	ids, err := parseIdentifiers("# comment\n\nhost: sapbox.corp.invalid\r\nJDOEPRIVATE\nip:" + ip("192", "168", "1", "2") + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || ids[0].Class != "host" || ids[1].Class != "identifier" || ids[2].Class != "ip" || ids[2].Value != ip("192", "168", "1", "2") {
		t.Fatalf("got %+v", ids)
	}
	if _, err := parseIdentifiers("sid: QX\n"); err == nil {
		t.Error("a 2-character identifier was accepted")
	}
}

func TestAllowFileRules(t *testing.T) {
	good := `# comment
path  **/go.sum              packed-private-ip  module checksums are hashed bytes
path  docs/*.md              generic            documentation examples, reviewed
path  fixtures/**            *                  synthetic fixtures only, reviewed
match private-ip  10\.0\.0\.[0-9]+               the documentation address range we use
`
	rules, err := parseAllow(strings.NewReader(good))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 4 {
		t.Fatalf("got %d rules", len(rules))
	}
	cases := []struct {
		h    Hit
		want bool
	}{
		{Hit{File: "go.sum", Class: "packed-private-ip", Generic: true}, true},
		{Hit{File: "a/b/go.sum", Class: "packed-private-ip", Generic: true}, true},
		{Hit{File: "go.sum", Class: "private-ip", Generic: true}, false},
		{Hit{File: "docs/x.md", Class: "csrf-token", Generic: true}, true},
		{Hit{File: "docs/sub/x.md", Class: "csrf-token", Generic: true}, false},
		{Hit{File: "docs/x.md", Class: "identifier/host"}, false},
		{Hit{File: "fixtures/a/b.json", Class: "identifier/host"}, true},
		{Hit{File: "x.go", Class: "private-ip", Generic: true, value: ip("10", "0", "0", "7")}, true},
		{Hit{File: "x.go", Class: "private-ip", Generic: true, value: ip("10", "0", "0", "7") + "1.1"}, false},
		{Hit{File: "x.go", Class: "private-ip", Generic: true, value: ip("10", "9", "0", "7")}, false},
	}
	for _, c := range cases {
		if got := allowed(rules, c.h); got != c.want {
			t.Errorf("%+v: allowed=%v, want %v", c.h, got, c.want)
		}
	}

	for name, bad := range map[string]string{
		"no reason":              "path go.sum packed-private-ip\n",
		"reason too short":       "path go.sum packed-private-ip ok\n",
		"match on an identifier": "match identifier/host sapbox\\.corp\\.invalid somebody wanted it quiet\n",
		"match on everything":    "match * .* somebody wanted it quiet\n",
		"unknown kind":           "file go.sum * checksums are hashed bytes\n",
		"bad regex":              "match private-ip ( an unbalanced pattern here\n",
	} {
		if _, err := parseAllow(strings.NewReader(good + bad)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// --- run(): exit codes and fail-closed ---------------------------------------

func runScan(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, func(k string) string { return env[k] }, &out, &errb)
	return code, out.String(), errb.String()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunMissingListInExpectedMode(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), "nothing to see\n")
	if code, _, errs := runScan(t, nil, "-root", root, "-require-identifiers", "a.txt"); code != exitClosed {
		t.Fatalf("required list missing: exit %d, want %d\n%s", code, exitClosed, errs)
	}
	// An empty secret is a missing list, not an empty one that passes.
	if code, _, _ := runScan(t, map[string]string{envList: "\n \n"}, "-root", root, "-require-identifiers", "a.txt"); code != exitClosed {
		t.Fatalf("blank list: exit %d, want %d", code, exitClosed)
	}
	// Not required: generic only, reported as partial, never as clean.
	code, _, errs := runScan(t, nil, "-root", root, "a.txt")
	if code != exitPartial || !strings.Contains(errs, "NO identifier list") {
		t.Fatalf("optional list missing: exit %d, want %d\n%s", code, exitPartial, errs)
	}
	// An explicit -identifiers that does not exist fails closed.
	if code, _, _ := runScan(t, nil, "-root", root, "-identifiers", filepath.Join(root, "nope"), "a.txt"); code != exitClosed {
		t.Fatalf("missing -identifiers file: exit %d", code)
	}
	// With the list: clean.
	if code, _, errs := runScan(t, map[string]string{envList: "user: JDOEPRIVATE"}, "-root", root, "-require-identifiers", "a.txt"); code != exitClean {
		t.Fatalf("with list: exit %d\n%s", code, errs)
	}
}

func TestRunZeroFilesRead(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{envList: "user: JDOEPRIVATE"}
	if code, _, errs := runScan(t, env, "-root", root, "empty"); code != exitClosed {
		t.Fatalf("zero files read: exit %d, want %d\n%s", code, exitClosed, errs)
	}
	if code, _, _ := runScan(t, env, "-root", root, "does-not-exist"); code != exitClosed {
		t.Fatalf("missing path: exit %d, want %d", code, exitClosed)
	}
}

func TestRunHitsNeverPrintTheValue(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "notes.md"), "ok\nlogged on as JDOEPRIVATE at "+ip("192", "168", "3", "4")+"\n")
	code, out, _ := runScan(t, map[string]string{envList: "user: JDOEPRIVATE"}, "-root", root, "notes.md")
	if code != exitHits {
		t.Fatalf("exit %d, want %d", code, exitHits)
	}
	if !strings.Contains(out, "notes.md:2: identifier/user (text)") || !strings.Contains(out, "notes.md:2: private-ip (text)") {
		t.Errorf("output:\n%s", out)
	}
	if strings.Contains(strings.ToLower(out), "jdoeprivate") || strings.Contains(out, ip("192", "168", "3", "4")) {
		t.Errorf("output prints a value:\n%s", out)
	}
}

func TestRunAllowFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "fixtures/a.txt"), "host "+ip("10", "1", "2", "3")+"\n")
	env := map[string]string{envList: "user: JDOEPRIVATE"}
	if code, _, _ := runScan(t, env, "-root", root, "fixtures"); code != exitHits {
		t.Fatalf("without allow-file: exit %d", code)
	}
	writeFile(t, filepath.Join(root, defaultAllw), "path fixtures/** private-ip synthetic addresses in fixtures\n")
	if code, _, errs := runScan(t, env, "-root", root, "fixtures"); code != exitClean {
		t.Fatalf("with allow-file: exit %d\n%s", code, errs)
	}
	writeFile(t, filepath.Join(root, defaultAllw), "path fixtures/** private-ip\n")
	if code, _, _ := runScan(t, env, "-root", root, "fixtures"); code != exitClosed {
		t.Fatalf("allow line without a reason: exit %d, want %d", code, exitClosed)
	}
}

func TestRunDiffAndAll(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := t.TempDir()
	g := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	g("init", "-q", "-b", "main")
	writeFile(t, filepath.Join(root, "old.md"), "already public: JDOEPRIVATE\n")
	g("add", ".")
	g("commit", "-qm", "base")
	g("checkout", "-qb", "topic")
	writeFile(t, filepath.Join(root, "new.md"), "clean\n")
	g("add", ".")
	g("commit", "-qm", "add a file")
	env := map[string]string{envList: "user: JDOEPRIVATE"}

	// Only what the branch changes: the old hit is not this push's.
	if code, out, errs := runScan(t, env, "-root", root, "-diff", "main"); code != exitClean {
		t.Fatalf("diff: exit %d\n%s%s", code, out, errs)
	}
	g("commit", "-q", "--allow-empty", "-m", "seen on "+ip("192", "168", "5", "6"))
	code, out, _ := runScan(t, env, "-root", root, "-diff", "main")
	if code != exitHits || !strings.Contains(out, "(message)") {
		t.Fatalf("commit message: exit %d\n%s", code, out)
	}
	// -all reads the whole tree.
	if code, out, _ := runScan(t, env, "-root", root, "-all"); code != exitHits || !strings.Contains(out, "old.md:1: identifier/user") {
		t.Fatalf("all: exit %d\n%s", code, out)
	}
	// A push that changes no file and no commit is an empty set, not a failure.
	g("checkout", "-q", "main")
	if code, _, errs := runScan(t, env, "-root", root, "-diff", "main"); code != exitClean {
		t.Fatalf("empty diff: exit %d\n%s", code, errs)
	}
	// A base that does not exist fails closed.
	if code, _, _ := runScan(t, env, "-root", root, "-diff", "no-such-ref"); code != exitClosed {
		t.Fatalf("bad base: exit %d", code)
	}
}
