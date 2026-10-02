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
	"time"
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
	// A short identifier (a SID) is matched in text, and in decoded bytes when
	// it stands there as a token.
	sid := Identifier{Class: "sid", Value: "QX7"}
	if hasHit(scanBytes("f", []byte("system QX7 client"), []Identifier{sid}), "identifier/sid", "text") == nil {
		t.Error("SID not matched in text")
	}
	for name, c := range map[string]struct {
		data []byte
		view string
		want bool
	}{
		"base64 token":          {[]byte(base64.StdEncoding.EncodeToString([]byte("sysid=QX7;client=100"))), "base64", true},
		"hex token":             {[]byte(hex.EncodeToString([]byte("logon QX7 100 EN"))), "hex", true},
		"base64 run edge":       {[]byte(base64.StdEncoding.EncodeToString([]byte("QX7 is the system id"))), "base64", true},
		"base64 utf-16le token": {[]byte(base64.StdEncoding.EncodeToString(utf16le("SID=QX7;"))), "base64+utf-16le", true},
		"base64 next to a byte": {[]byte(base64.StdEncoding.EncodeToString([]byte("\x01\x9c\x80\x81\x90QX7\xff\x02\xa0\xa1\xb0\xb1"))), "", false},
		"base64 inside a word":  {[]byte(base64.StdEncoding.EncodeToString([]byte("abcdefQX7ghijkl"))), "", false},
	} {
		got := hasHit(scanBytes("f", c.data, []Identifier{sid}), "identifier/sid", c.view) != nil
		if got != c.want {
			t.Errorf("%s: hit=%v, want %v", name, got, c.want)
		}
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
		{`"x-csrf-` + `token": "xxxxxxxxxxxxxxxxxxxx"`, "csrf-token", false},
		{`"x-csrf-` + `token": "test-csrf-token-0001"`, "csrf-token", true},
		{`{"SAP_` + `PASSWORD": "Wk7pq2Zr9"}`, "config-password", true},
		{`{"pass` + `word": "your-password"}`, "config-password", false},
		{`{"pass` + `word": "pass"}`, "config-password", false},
		// Only an exact placeholder is one: a word inside a value is not.
		{`{"pass` + `word": "MySecret2026!"}`, "config-password", true},
		{`{"pass` + `word": "testing-Kq81"}`, "config-password", true},
		{`{"pass` + `word": "<password>"}`, "config-password", false},
		{`{"pass` + `word": "${SAP_PASSWORD}"}`, "config-password", false},
		{`{"pass` + `word": "changeme"}`, "config-password", false},
		{`{"pass` + `word": "****"}`, "config-password", false},
		{`{"SAP_` + `PASSWORD": "YOUR_PASSWORD_HERE"}`, "config-password", false},
		{"Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("JDOE:MySecret2026!")), "basic-auth", true},
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
match private-ip  10\.0\.0\.[0-9]+               the documentation address range we use
`
	rules, err := parseAllow(strings.NewReader(good))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 3 {
		t.Fatalf("got %d rules", len(rules))
	}
	cases := []struct {
		h    Hit
		want bool
	}{
		{Hit{Path: "go.sum", Class: "packed-private-ip", Generic: true}, true},
		{Hit{Path: "a/b/go.sum", Class: "packed-private-ip", Generic: true}, true},
		{Hit{Path: "go.sum", Class: "private-ip", Generic: true}, false},
		{Hit{Path: "docs/x.md", Class: "csrf-token", Generic: true}, true},
		{Hit{Path: "docs/sub/x.md", Class: "csrf-token", Generic: true}, false},
		// A listed identifier is never excused, even where generic hits are.
		{Hit{Path: "docs/x.md", Class: "identifier/host"}, false},
		// A commit message has no path, so no path rule reaches it.
		{Hit{File: "commit abc (message)", Class: "csrf-token", Generic: true}, false},
		{Hit{Path: "x.go", Class: "private-ip", Generic: true, value: ip("10", "0", "0", "7")}, true},
		{Hit{Path: "x.go", Class: "private-ip", Generic: true, value: ip("10", "0", "0", "7") + "1.1"}, false},
		{Hit{Path: "x.go", Class: "private-ip", Generic: true, value: ip("10", "9", "0", "7")}, false},
	}
	for _, c := range cases {
		if got := allowed(rules, c.h); got != c.want {
			t.Errorf("%+v: allowed=%v, want %v", c.h, got, c.want)
		}
	}

	for name, bad := range map[string]string{
		"no reason":                  "path go.sum packed-private-ip\n",
		"reason too short":           "path go.sum packed-private-ip ok\n",
		"match on an identifier":     "match identifier/host sapbox\\.corp\\.invalid somebody wanted it quiet\n",
		"path on an identifier":      "path fixtures/a.json identifier/host somebody wanted it quiet\n",
		"class star":                 "path fixtures/a.json * somebody wanted it quiet\n",
		"match on everything":        "match generic .* somebody wanted it quiet\n",
		"match on anything nonempty": "match private-ip .+ somebody wanted it quiet\n",
		"path everything":            "path ** generic somebody wanted it quiet\n",
		"path every top file":        "path * private-ip somebody wanted it quiet\n",
		"path every file":            "path **/* private-ip somebody wanted it quiet\n",
		"unknown kind":               "file go.sum generic checksums are hashed bytes\n",
		"bad regex":                  "match private-ip ( an unbalanced pattern here\n",
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

// newRepo is a temporary git repository with one commit on main, and a
// function that runs git in it and returns the trimmed output.
func newRepo(t *testing.T) (string, func(args ...string) string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := t.TempDir()
	g := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	g("init", "-q", "-b", "main")
	writeFile(t, filepath.Join(root, "README.md"), "clean\n")
	g("add", ".")
	g("commit", "-qm", "base")
	return root, g
}

func TestRunAddThenDeleteInRange(t *testing.T) {
	root, g := newRepo(t)
	env := map[string]string{envList: "host: sapbox.corp.invalid"}
	g("checkout", "-qb", "topic")
	writeFile(t, filepath.Join(root, "notes/capture.md"), "one\ntwo\nconnected to sapbox.corp.invalid\n")
	g("add", ".")
	g("commit", "-qm", "add a capture")
	added := g("rev-parse", "--short", "HEAD")
	g("rm", "-q", "notes/capture.md")
	g("commit", "-qm", "remove it again")

	// The tip is clean; the history is not.
	if code, _, _ := runScan(t, env, "-root", root, "-all"); code != exitClean {
		t.Fatalf("tip: exit %d, want clean", code)
	}
	code, out, _ := runScan(t, env, "-root", root, "-diff", "main")
	if code != exitHits || !strings.Contains(out, "notes/capture.md:3: identifier/host (text) [added in "+added+"]") {
		t.Fatalf("diff: exit %d, want the line the first commit added\n%s", code, out)
	}
	if code, _, _ := runScan(t, env, "-root", root, "-range", "main..topic"); code != exitHits {
		t.Fatalf("range: exit %d", code)
	}

	// A binary added and removed again is read whole from its commit.
	g("checkout", "-q", "main")
	g("checkout", "-qb", "binary")
	writeFile(t, filepath.Join(root, "logon.bin"), string(append([]byte{0, 1, 2, 0}, utf16le("Server=sapbox.corp.invalid")...)))
	g("add", ".")
	g("commit", "-qm", "add a template")
	g("rm", "-q", "logon.bin")
	g("commit", "-qm", "remove it")
	code, out, _ = runScan(t, env, "-root", root, "-diff", "main")
	if code != exitHits || !strings.Contains(out, "logon.bin:1: identifier/host (utf-16le)") {
		t.Fatalf("binary: exit %d\n%s", code, out)
	}
}

func TestRunPushRangeCommitMessage(t *testing.T) {
	root, g := newRepo(t)
	env := map[string]string{envList: "host: sapbox.corp.invalid"}
	before := g("rev-parse", "HEAD")
	writeFile(t, filepath.Join(root, "a.go"), "package a\n")
	g("add", ".")
	g("commit", "-qm", "fix the logon\n\nreproduced against sapbox.corp.invalid")
	after := g("rev-parse", "HEAD")

	if code, _, _ := runScan(t, env, "-root", root, "-all"); code != exitClean {
		t.Fatalf("tree only: exit %d, want clean (the hit is in a message)", code)
	}
	code, out, _ := runScan(t, env, "-root", root, "-all", "-range", before+".."+after)
	if code != exitHits || !strings.Contains(out, "(message)") || !strings.Contains(out, "identifier/host") {
		t.Fatalf("push range: exit %d\n%s", code, out)
	}
	if strings.Contains(out, "sapbox") {
		t.Errorf("output prints the value:\n%s", out)
	}
}

func TestRunHostileAllowFileInPR(t *testing.T) {
	root, g := newRepo(t)
	env := map[string]string{envList: "host: sapbox.corp.invalid"}
	writeFile(t, filepath.Join(root, defaultAllw), "path **/go.sum packed-private-ip module checksums are hashed bytes\n")
	g("add", ".")
	g("commit", "-qm", "allow-file")

	g("checkout", "-qb", "pr")
	writeFile(t, filepath.Join(root, "fixtures/a.txt"), "host "+ip("10", "1", "2", "3")+"\n")
	writeFile(t, filepath.Join(root, defaultAllw), "path **/go.sum packed-private-ip module checksums are hashed bytes\n"+
		"path fixtures/a.txt private-ip a perfectly reasonable sounding excuse\n")
	g("add", ".")
	g("commit", "-qm", "add a fixture and excuse it")

	// The PR's own allow-file would excuse it...
	if code, _, _ := runScan(t, env, "-root", root, "-diff", "main"); code != exitClean {
		t.Fatalf("PR allow-file: exit %d", code)
	}
	// ...but CI reads the base's, where the rule does not exist yet.
	if code, out, _ := runScan(t, env, "-root", root, "-diff", "main", "-allow-rev", "main"); code != exitHits || !strings.Contains(out, "fixtures/a.txt:1: private-ip") {
		t.Fatalf("base allow-file: exit %d\n%s", code, out)
	}
	// A rule that excuses everything is refused outright, wherever it is read.
	writeFile(t, filepath.Join(root, defaultAllw), "path ** generic nothing to see here at all\n")
	if code, _, _ := runScan(t, env, "-root", root, "-diff", "main"); code != exitClosed {
		t.Fatalf("broad rule: exit %d, want %d", code, exitClosed)
	}
	// A base without an allow-file excuses nothing; a base that is not a
	// commit fails closed.
	if code, _, _ := runScan(t, env, "-root", root, "-diff", "main", "-allow-rev", "main~1"); code != exitHits {
		t.Fatalf("base without allow-file: exit %d", code)
	}
	if code, _, _ := runScan(t, env, "-root", root, "-diff", "main", "-allow-rev", "no-such-ref"); code != exitClosed {
		t.Fatalf("bad -allow-rev: exit %d", code)
	}
}

func TestShortEncodedSID(t *testing.T) {
	// The public developer-edition SID, assembled at run time: spelled out in
	// this file, in any encoding, it would be a hit for anyone who puts it on
	// their list. The encodings are the reviewer's exact cases for "SID=" plus
	// the SID: 14 hex digits, the same one nibble in (a stray leading digit),
	// and padded base64 with and without its "==".
	value := "A4" + "H"
	sid := Identifier{Class: "sid", Value: value}
	hexed := hex.EncodeToString([]byte("SID=" + value))
	b64 := base64.StdEncoding.EncodeToString([]byte("SID=" + value))
	for name, data := range map[string]string{
		"hex, 14 digits":          "id " + hexed + " end",
		"hex, one nibble in":      "id a" + hexed + " end",
		"base64, padded":          "id " + b64 + " end",
		"base64, padding dropped": "id " + strings.TrimRight(b64, "=") + " end",
	} {
		if hasHit(scanBytes("f", []byte(data), []Identifier{sid}), "identifier/sid", "") == nil {
			t.Errorf("%s: %q not found", name, data)
		}
	}
	if len(hexed) != 14 || !strings.HasSuffix(b64, "==") {
		t.Fatalf("fixture drifted from the reviewer's cases: %s %s", hexed, b64)
	}
	// The token boundary still holds for short runs: inside a word, no hit.
	inner := base64.StdEncoding.EncodeToString([]byte("x" + value + "y"))
	if hasHit(scanBytes("f", []byte("id "+inner+" end"), []Identifier{sid}), "identifier/sid", "") != nil {
		t.Errorf("%q: SID matched inside a word", inner)
	}
}

func TestRunPushEvent(t *testing.T) {
	root, g := newRepo(t)
	env := map[string]string{envList: "host: sapbox.corp.invalid"}
	main := g("rev-parse", "HEAD")

	// A branch whose history adds a host and deletes it again.
	g("checkout", "-qb", "feature")
	writeFile(t, filepath.Join(root, "capture.md"), "sapbox.corp.invalid\n")
	g("add", ".")
	g("commit", "-qm", "add")
	g("rm", "-q", "capture.md")
	g("commit", "-qm", "delete")
	tip := g("rev-parse", "HEAD")
	zero := strings.Repeat("0", 40)

	// New branch (all-zero BEFORE): every commit not on the base, not just the tip.
	code, out, _ := runScan(t, env, "-root", root, "-all", "-push", zero+".."+tip, "-push-base", "main")
	if code != exitHits || !strings.Contains(out, "capture.md:1: identifier/host") {
		t.Fatalf("new branch: exit %d\n%s", code, out)
	}
	if code, _, _ := runScan(t, env, "-root", root, "-all", "-push", zero+".."+tip); code != exitClosed {
		t.Fatalf("new branch without -push-base: exit %d", code)
	}
	// A normal push: BEFORE..AFTER.
	if code, _, _ := runScan(t, env, "-root", root, "-all", "-push", main+".."+tip); code != exitHits {
		t.Fatalf("push range: exit %d", code)
	}
	// A force push: BEFORE is gone from the clone. Fail closed, never "tip only".
	gone := strings.Repeat("ab", 20)
	code, _, errs := runScan(t, env, "-root", root, "-all", "-push", gone+".."+tip, "-push-base", "main")
	if code != exitClosed || !strings.Contains(errs, "force push") || !strings.Contains(errs, "ruleset") ||
		!strings.Contains(errs, "-range <last commit you trust>") || strings.Contains(errs, "empty") {
		t.Fatalf("force push: exit %d, want %d\n%s", code, exitClosed, errs)
	}
	// A clean push passes.
	g("checkout", "-q", "main")
	writeFile(t, filepath.Join(root, "ok.md"), "fine\n")
	g("add", ".")
	g("commit", "-qm", "fine")
	if code, _, errs := runScan(t, env, "-root", root, "-all", "-push", main+".."+g("rev-parse", "HEAD")); code != exitClean {
		t.Fatalf("clean push: exit %d\n%s", code, errs)
	}
}

// --- control bytes cannot blind the history scan ----------------------------

// The history scan once read `git log -p --format=%x01%h%x02%B%x03` and split
// on those bytes; a commit message or an added line holding one of them moved
// the cut, and what came after it was never matched. Each case below must be a
// hit: every byte of every message and every added line is read, whatever
// control bytes it holds.

const hiddenHost = "sapbox.corp.invalid"

func TestRunControlBytesInCommitMessage(t *testing.T) {
	env := map[string]string{envList: "host: " + hiddenHost}
	for name, msg := range map[string]string{
		"only an identifier after 0x01": "\x01" + hiddenHost,
		"only an identifier after 0x02": "\x02" + hiddenHost,
		"only an identifier after 0x03": "\x03" + hiddenHost,
		"0x01 in the subject":           "fix\x01 seen on " + hiddenHost,
		"0x03 then the identifier":      "fix the logon\n\nsee \x03" + hiddenHost + "\n",
		"0x01 0x02 0x03 around it":      "subject\n\n\x01abc\x02" + hiddenHost + "\x03tail\n",
		"0x03 0x02 0x01 reversed":       "subject\n\n\x03\x02\x01" + hiddenHost + "\n",
		"other control bytes":           "\x1b[2K\x7f\x04\x1f\x0b\x0c" + hiddenHost,
		"a fake patch in the message":   "subject\n\n\x03\ndiff --git a/x b/x\n+++ b/x\n@@ -0,0 +1 @@\n+" + hiddenHost + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			root, g := newRepo(t)
			g("checkout", "-qb", "topic")
			writeFile(t, filepath.Join(root, "a.md"), "clean\n")
			g("add", ".")
			mf := filepath.Join(t.TempDir(), "msg")
			writeFile(t, mf, msg)
			g("commit", "-q", "--cleanup=verbatim", "-F", mf)
			for _, mode := range [][]string{{"-diff", "main"}, {"-range", "main..topic"}} {
				code, out, errs := runScan(t, env, append([]string{"-root", root}, mode...)...)
				if code != exitHits || !strings.Contains(out, "(message)") || !strings.Contains(out, "identifier/host") {
					t.Fatalf("%v: exit %d, want a message hit\n%s%s", mode, code, out, errs)
				}
				if strings.Contains(out, "sapbox") {
					t.Errorf("output prints the value:\n%s", out)
				}
			}
		})
	}
}

// A NUL cannot be written into a message with `git commit`, but a commit
// object can carry one (hash-object --literally), and a pushed object is what
// is published.
func TestRunNULInCommitMessage(t *testing.T) {
	root, g := newRepo(t)
	env := map[string]string{envList: "host: " + hiddenHost}
	tree := g("rev-parse", "HEAD^{tree}")
	parent := g("rev-parse", "HEAD")
	obj := "tree " + tree + "\nparent " + parent + "\nauthor t <t@example.invalid> 0 +0000\ncommitter t <t@example.invalid> 0 +0000\n\n" +
		"subject\x00\x01" + hiddenHost + "\n"
	f := filepath.Join(t.TempDir(), "commit")
	writeFile(t, f, obj)
	sha := g("hash-object", "-t", "commit", "-w", "--literally", f)
	g("update-ref", "refs/heads/topic", sha)
	code, out, errs := runScan(t, env, "-root", root, "-range", "main..topic")
	if code != exitHits || !strings.Contains(out, "(message)") {
		t.Fatalf("exit %d, want a message hit\n%s%s", code, out, errs)
	}
}

// A message in another encoding, declared in the commit's encoding header, is
// what readers see recoded to UTF-8: a listed non-ASCII name written as
// ISO-8859-1 bytes must still be a hit.
func TestRunEncodedCommitMessage(t *testing.T) {
	root, g := newRepo(t)
	env := map[string]string{envList: "host: sapbøx.corp.invalid"}
	tree := g("rev-parse", "HEAD^{tree}")
	parent := g("rev-parse", "HEAD")
	obj := "tree " + tree + "\nparent " + parent + "\nauthor t <t@example.invalid> 0 +0000\ncommitter t <t@example.invalid> 0 +0000\n" +
		"encoding ISO-8859-1\n\nseen on sapb\xf8x.corp.invalid\n"
	f := filepath.Join(t.TempDir(), "commit")
	writeFile(t, f, obj)
	sha := g("hash-object", "-t", "commit", "-w", "--literally", f)
	g("update-ref", "refs/heads/topic", sha)
	code, out, errs := runScan(t, env, "-root", root, "-range", "main..topic")
	if code != exitHits || !strings.Contains(out, "(message)") {
		t.Fatalf("exit %d, want a message hit\n%s%s", code, out, errs)
	}
}

// A path is published as much as the bytes in it: a listed name that appears
// only in a file name is a hit, whether the file is added, or renamed away
// from (the old name) or to (the new name), and in the tree mode.
func TestRunIdentifierInPath(t *testing.T) {
	env := map[string]string{envList: "host: " + hiddenHost}
	t.Run("added", func(t *testing.T) {
		root, g := newRepo(t)
		g("checkout", "-qb", "topic")
		writeFile(t, filepath.Join(root, "notes", hiddenHost+".md"), "clean\n")
		g("add", ".")
		g("commit", "-qm", "add notes")
		for _, mode := range [][]string{{"-diff", "main"}, {"-range", "main..topic"}, {"-all"}} {
			code, out, errs := runScan(t, env, append([]string{"-root", root}, mode...)...)
			if code != exitHits || !strings.Contains(out, "(paths)") || !strings.Contains(out, "identifier/host") {
				t.Fatalf("%v: exit %d, want a path hit\n%s%s", mode, code, out, errs)
			}
			if strings.Contains(out, "sapbox") {
				t.Errorf("output prints the value:\n%s", out)
			}
		}
	})
	t.Run("added then deleted", func(t *testing.T) {
		root, g := newRepo(t)
		g("checkout", "-qb", "topic")
		writeFile(t, filepath.Join(root, hiddenHost+".log"), "clean\n")
		g("add", ".")
		g("commit", "-qm", "add")
		g("rm", "-q", hiddenHost+".log")
		g("commit", "-qm", "remove")
		if code, out, errs := runScan(t, env, "-root", root, "-diff", "main"); code != exitHits || !strings.Contains(out, "(paths)") {
			t.Fatalf("exit %d, want a path hit\n%s%s", code, out, errs)
		}
	})
	t.Run("renamed to", func(t *testing.T) {
		root, g := newRepo(t)
		g("checkout", "-qb", "topic")
		g("mv", "README.md", hiddenHost+".md")
		g("commit", "-qm", "rename")
		if code, out, errs := runScan(t, env, "-root", root, "-range", "main..topic"); code != exitHits || !strings.Contains(out, "(paths)") {
			t.Fatalf("exit %d, want a path hit\n%s%s", code, out, errs)
		}
	})
	t.Run("renamed from", func(t *testing.T) {
		root, g := newRepo(t)
		writeFile(t, filepath.Join(root, hiddenHost+".md"), "some content that stays the same\n")
		g("add", ".")
		g("commit", "-qm", "base two")
		g("checkout", "-qb", "topic")
		g("mv", hiddenHost+".md", "notes.md")
		g("commit", "-qm", "rename it away")
		if code, out, errs := runScan(t, env, "-root", root, "-range", "main..topic"); code != exitHits || !strings.Contains(out, "(paths)") {
			t.Fatalf("exit %d, want a hit on the old name\n%s%s", code, out, errs)
		}
	})
}

// A path is never printed when it could carry what the scan looks for: a
// listed name in a file name, with a newline in it or not, is reported as a
// hit (not a failure), and neither stdout nor stderr holds the name.
func TestRunPathNeverPrinted(t *testing.T) {
	env := map[string]string{envList: "host: " + hiddenHost}
	for name, file := range map[string]string{
		"newline in the name":  "notes\n" + hiddenHost + ".txt",
		"tab and the name":     "x\t" + hiddenHost,
		"plain name, dirty IP": hiddenHost + ".md",
	} {
		t.Run(name, func(t *testing.T) {
			root, g := newRepo(t)
			g("checkout", "-qb", "topic")
			writeFile(t, filepath.Join(root, file), "seen on "+ip("10", "9", "8", "7")+"\n")
			g("add", ".")
			g("commit", "-qm", "add")
			for _, mode := range [][]string{{"-diff", "main"}, {"-all"}, {"-range", "main..topic"}} {
				code, out, errs := runScan(t, env, append([]string{"-root", root}, mode...)...)
				if strings.Contains(out+errs, "sapbox") {
					t.Fatalf("%v: the output prints the name\n%s%s", mode, out, errs)
				}
				if code != exitHits || !strings.Contains(out, "identifier/host") || !strings.Contains(out, "private-ip") {
					t.Fatalf("%v: exit %d, want the path hit and the content hit\n%s%s", mode, code, out, errs)
				}
			}
		})
	}
}

// An object over the size limit fails closed, and promptly: the scanner must
// not wait on a git that is still writing the object it refused.
func TestRunOversizedObjectsFailClosedPromptly(t *testing.T) {
	env := map[string]string{envList: "host: " + hiddenHost}
	big := strings.Repeat("x", maxFileSize+1<<20)
	within := func(t *testing.T, args ...string) (int, string) {
		t.Helper()
		type res struct {
			code int
			errs string
		}
		done := make(chan res, 1)
		go func() {
			code, _, errs := runScan(t, env, args...)
			done <- res{code, errs}
		}()
		select {
		case r := <-done:
			return r.code, r.errs
		case <-time.After(60 * time.Second):
			t.Fatalf("%v: still running after 60 s", args)
		}
		return 0, ""
	}
	t.Run("message", func(t *testing.T) {
		root, g := newRepo(t)
		g("checkout", "-qb", "topic")
		mf := filepath.Join(t.TempDir(), "msg")
		writeFile(t, mf, "subject\n\n"+big+"\n")
		g("commit", "-q", "--allow-empty", "-F", mf)
		if code, errs := within(t, "-root", root, "-range", "main..topic"); code != exitClosed || !strings.Contains(errs, "limit") {
			t.Fatalf("exit %d, want %d\n%s", code, exitClosed, errs)
		}
	})
	t.Run("blob", func(t *testing.T) {
		root, g := newRepo(t)
		writeFile(t, filepath.Join(root, "big.txt"), big)
		g("add", ".")
		g("commit", "-qm", "big")
		if code, errs := within(t, "-root", root, "-all"); code != exitClosed || !strings.Contains(errs, "limit") {
			t.Fatalf("exit %d, want %d\n%s", code, exitClosed, errs)
		}
	})
}

func TestRunControlBytesInAddedLine(t *testing.T) {
	env := map[string]string{envList: "host: " + hiddenHost}
	for name, content := range map[string]string{
		"0x01 before":       "one\n\x01" + hiddenHost + "\n",
		"0x02 before":       "one\n\x02" + hiddenHost + "\n",
		"0x03 before":       "one\n\x03" + hiddenHost + "\n",
		"all three":         "one\nx\x01\x02\x03" + hiddenHost + "\n",
		"around":            "one\n\x01" + hiddenHost + "\x03\n",
		"other control":     "one\n\x1b\x7f\x04" + hiddenHost + "\n",
		"NUL (binary file)": "one\n\x00\x01" + hiddenHost + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			root, g := newRepo(t)
			g("checkout", "-qb", "topic")
			writeFile(t, filepath.Join(root, "capture.txt"), content)
			g("add", ".")
			g("commit", "-qm", "add")
			g("rm", "-q", "capture.txt")
			g("commit", "-qm", "remove it again")
			code, out, errs := runScan(t, env, "-root", root, "-diff", "main")
			if code != exitHits || !strings.Contains(out, "capture.txt:") || !strings.Contains(out, "identifier/host") {
				t.Fatalf("exit %d, want a hit in capture.txt\n%s%s", code, out, errs)
			}
		})
	}
}
