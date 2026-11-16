package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/hanadump"
)

func runArgs(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestVersionJSON(t *testing.T) {
	code, out, _ := runArgs(t, "", "--version", "--json")
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("%v: %q", err, out)
	}
	if v["schema"] != float64(1) || v["read_only"] != true || v["name"] != "osd-hana-dump" {
		t.Errorf("version: %v", v)
	}
	if strings.Count(out, "\n") != 1 {
		t.Errorf("not one line: %q", out)
	}
}

func TestNoPasswordFlag(t *testing.T) {
	code, out, _ := runArgs(t, "", "probe", "--host", "h", "--port", "1", "--user", "u", "--password", "secret")
	if code != exitUsage {
		t.Errorf("exit %d, want %d", code, exitUsage)
	}
	if strings.Contains(out, "secret") {
		t.Errorf("password echoed: %q", out)
	}
}

func TestUsageErrors(t *testing.T) {
	t.Setenv("OSD_HANA_PASSWORD", "")
	t.Setenv("OSD_HANA_USER", "")
	cases := []struct {
		stdin string
		args  []string
		want  string
	}{
		{"", nil, "no command"},
		{"", []string{"frobnicate"}, "unknown command"},
		{"", []string{"probe", "--host", "h", "--port", "1", "--user", "u"}, "no password"},
		{"", []string{"probe", "--host", "h", "--user", "u"}, "host, port and user"},
		{"", []string{"dump", "--host", "h", "--port", "1", "--user", "u", "--out", "x"}, "dump needs --request"},
		{"", []string{"list", "--password-stdin", "--request", "-"}, "both want stdin"},
		{`{"schema":1,"names":["Z%"],"bogus":true}`, []string{"list", "--request", "-"}, `unknown field "bogus"`},
		{`{"schema":2}`, []string{"list", "--request", "-"}, "schema 2"},
		{`{"schema":1,"tables":[{"name":"ZT"}]}`, []string{"list", "--request", "-"}, "tables: not implemented"},
		{`{"schema":1,"languages":["DE"]}`, []string{"list", "--request", "-"}, "languages: not implemented"},
		{`{"schema":1,"local":{"all":true,"authors":["A"]}}`, []string{"list", "--request", "-"}, "all or authors"},
		{`{"schema":1,"local":{"authors":["A%"]}}`, []string{"list", "--request", "-"}, "not a user name"},
		{`{"schema":1} {}`, []string{"list", "--request", "-"}, "trailing data"},
	}
	for _, c := range cases {
		code, out, _ := runArgs(t, c.stdin, c.args...)
		if code != exitUsage {
			t.Errorf("%v: exit %d, want %d", c.args, code, exitUsage)
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(out), &res); err != nil || res["ok"] != false {
			t.Errorf("%v: stdout not a one-line failure: %q", c.args, out)
			continue
		}
		if !strings.Contains(res["error"].(string), c.want) {
			t.Errorf("%v: error %q, want %q", c.args, res["error"], c.want)
		}
	}
}

func TestClassify(t *testing.T) {
	producer := map[string]bool{"/ACME/": true}
	local := map[string]bool{"TLOCAL": true}
	cases := []struct{ name, pkg, want string }{
		{"ZCL_A", "SAPPKG", "customer"},
		{"YPROG", "", "customer"},
		{"/ACME/CL_X", "/ACME/MAIN", "customer"},
		{"/BOBF/CL_X", "/BOBF/MAIN", "sap"},
		{"CL_ABAP_X", "SABP", "sap"},
		{"CL_ABAP_X", "$TMP", "customer"},
		{"CL_ABAP_X", "TLOCAL", "customer"},
		{"CL_ABAP_X", "ZPKG", "customer"},
		{"/ACME", "X", "sap"}, // no closing slash: not a namespace
	}
	for _, c := range cases {
		if got := classify(c.name, c.pkg, producer, local); got != c.want {
			t.Errorf("classify(%q,%q) = %s, want %s", c.name, c.pkg, got, c.want)
		}
	}
}

func TestLikeMatchAndUnlike(t *testing.T) {
	for _, c := range []struct {
		pat, s string
		want   bool
	}{
		{"Z%", "ZA", true}, {"Z%", "YA", false}, {`$DEMO\_X`, "$DEMO_X", true}, {`$DEMO\_X`, "$DEMOAX", false},
		{"$DEMO_X", "$DEMOAX", true}, {"%", "", true}, {"A_C", "ABC", true}, {"A_C", "AC", false}, {`100\%`, "100%", true},
	} {
		if got := likeMatch(c.pat, c.s); got != c.want {
			t.Errorf("likeMatch(%q,%q) = %v", c.pat, c.s, got)
		}
	}
	for _, o := range []hanadump.PackageObject{{Type: "CLAS", Name: "ZCL_A_B"}, {Type: "FUGR", Name: "/NS/G_1"}, {Type: "PROG", Name: "Z%X"}} {
		for _, p := range hanadump.SourcePatterns(o) {
			lit, prefix := unlike(p)
			if strings.HasSuffix(p, "%") && !strings.HasSuffix(p, `\%`) != prefix {
				t.Errorf("unlike(%q) prefix %v", p, prefix)
			}
			if !likeMatch(p, lit) {
				t.Errorf("pattern %q does not match its own literal %q", p, lit)
			}
		}
	}
}

func TestBuildReportEveryObjectOneStatus(t *testing.T) {
	objs := []Object{
		{Type: "CLAS", Name: "ZCL_A", Package: "$P"},
		{Type: "PROG", Name: "ZBROKEN", Package: "$P"},
		{Type: "PROG", Name: "ZNOSRC", Package: "$P"},
		{Type: "TABL", Name: "ZT", Package: "$P"},
		{Type: "MSAG", Name: "ZM", Package: "$P"},
		{Type: "FUGR", Name: "ZG", Package: "$P"},
	}
	recs := []hanadump.SourceRecord{
		{Program: "ZCL_A=========================CP", State: "A", File: "src/a.abap", SHA256: "aa"},
		{Program: "ZCL_A=========================CU", State: "I", File: "src/a.i.abap", SHA256: "ab"},
		{Program: "ZCL_AB========================CP", State: "A", File: "src/other.abap"}, // another class: not ZCL_A's
		{Program: "ZBROKEN", State: "A", File: "undecoded/zbroken.a.bin", Error: "odd length"},
		{Program: "SAPLZG", State: "A", File: "src/saplzg.abap"},
		{Program: "LZGTOP", State: "A", Empty: true},
	}
	req := &Request{Schema: 1, Requested: map[string][]string{"TABL": {"ZT", "ZGONE"}}}
	m := &hanadump.Manifest{Closure: &hanadump.ClosureResult{Missing: map[string][]string{"TABL": {"ZGONE"}}, Unhandled: map[string][]string{"ENHO": {"ZE"}}}}
	report, errs := buildReport(objs, recs, req, m)
	got := map[string]ReportEntry{}
	for _, e := range report {
		if _, dup := got[e.Type+" "+e.Name]; dup {
			t.Errorf("%s %s reported twice", e.Type, e.Name)
		}
		got[e.Type+" "+e.Name] = e
		if e.Status == statusSkipped && e.Reason == "" {
			t.Errorf("%s %s skipped without reason", e.Type, e.Name)
		}
	}
	check := func(key, status, reason string, files int) {
		t.Helper()
		e, ok := got[key]
		if !ok {
			t.Errorf("%s missing from report", key)
			return
		}
		if e.Status != status || e.Reason != reason || len(e.Files) != files {
			t.Errorf("%s: %s/%s with %d files, want %s/%s with %d", key, e.Status, e.Reason, len(e.Files), status, reason, files)
		}
	}
	check("CLAS ZCL_A", statusWritten, "", 2)
	check("PROG ZBROKEN", statusSkipped, reasonEncoding, 1)
	check("PROG ZNOSRC", statusSkipped, reasonNoSource, 0)
	check("TABL ZT", statusRows, "", 0)
	check("MSAG ZM", statusSkipped, reasonUnsupported, 0)
	check("FUGR ZG", statusWritten, "", 1)
	check("TABL ZGONE", statusSkipped, reasonNotFound, 0)
	check("ENHO ZE", statusSkipped, reasonUnsupported, 0)
	if got["CLAS ZCL_A"].State != "A+I" {
		t.Errorf("state %q", got["CLAS ZCL_A"].State)
	}
	if len(report) != 8 {
		t.Errorf("%d entries", len(report))
	}
	// errors: the undecodable program and the missing table; not the
	// program without source, not the unsupported kinds
	if len(errs) != 2 {
		t.Errorf("errors: %v", errs)
	}
}

func TestBuildReportDDICOff(t *testing.T) {
	report, _ := buildReport([]Object{{Type: "DTEL", Name: "ZD"}}, nil, &Request{Schema: 1}, &hanadump.Manifest{})
	if report[0].Status != statusSkipped || report[0].Reason != reasonExcluded {
		t.Errorf("%+v", report[0])
	}
}

func TestProgSizesOf(t *testing.T) {
	p := &progSizes{names: []string{"LZGF01", "LZGTOP", "SAPLZG", "ZCL_A=========================CP", "ZCL_A=========================CU"}, size: []int64{1, 2, 4, 8, 16}}
	n, b := p.of(hanadump.SourcePatterns(hanadump.PackageObject{Type: "FUGR", Name: "ZG"}))
	if n != 3 || b != 7 {
		t.Errorf("FUGR: %d %d", n, b)
	}
	n, b = p.of(hanadump.SourcePatterns(hanadump.PackageObject{Type: "CLAS", Name: "ZCL_A"}))
	if n != 2 || b != 24 {
		t.Errorf("CLAS: %d %d", n, b)
	}
}

func TestScrub(t *testing.T) {
	got := scrub(errString("dial hdb://u:p%40ss@h: p@ss refused"), "p@ss")
	if strings.Contains(got, "p@ss") || strings.Contains(got, "p%40ss") {
		t.Errorf("password left in %q", got)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
