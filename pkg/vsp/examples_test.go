package vsp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// The examples command asks CROSS who calls an object. The query carries a
// one-character type code, and a query can be refused before the table is read.
// Neither the code nor a refusal may be hidden from the reader.

// examplesStub records every query it is asked, and answers with a fixed number
// of rows — except for the tables named in refuse, which answer 500.
type examplesStub struct {
	mu      sync.Mutex
	queries []string
}

func (s *examplesStub) record(sql string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queries = append(s.queries, sql)
}

// sent returns the queries received, whitespace collapsed so a caller can match
// a predicate without pinning how the statement was wrapped.
func (s *examplesStub) sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.queries))
	for _, q := range s.queries {
		out = append(out, strings.Join(strings.Fields(q), " "))
	}
	return out
}

func newExamplesStub(t *testing.T, rows int, refuse ...string) (*httptest.Server, *examplesStub) {
	t.Helper()
	stub := &examplesStub{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "test-token")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		sql := strings.Join(strings.Fields(string(body)), " ")
		stub.record(string(body))
		for _, table := range refuse {
			if strings.Contains(sql, " FROM "+table+" ") {
				http.Error(w, "Data was lost while copying a value.", http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(examplesRowsXML(rows)))
	}))
	t.Cleanup(srv.Close)
	return srv, stub
}

// examplesRowsXML is a data-preview answer carrying n rows of ZDEMO_CALLER.
func examplesRowsXML(n int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	b.WriteString(`<dataPreview:tableData xmlns:dataPreview="http://www.sap.com/adt/dataPreview">`)
	b.WriteString(`<dataPreview:columns><dataPreview:metadata dataPreview:name="INCLUDE" ` +
		`dataPreview:type="C" dataPreview:length="40"/><dataPreview:dataSet>`)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `<dataPreview:data>ZDEMO_CALLER%d</dataPreview:data>`, i)
	}
	b.WriteString(`</dataPreview:dataSet></dataPreview:columns>`)
	b.WriteString(`</dataPreview:tableData>`)
	return b.String()
}

// examplesAgainst points the command at the stub. resolveSystemParams falls back
// to SAP_* when no --system is given, so the environment is the seam. HOME, the
// working directory and the cache are moved aside: .vsp.json is read from both
// places — the working directory first — and either can name a default system
// that would stand in for the stub.
func examplesAgainst(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("SAP_URL", srv.URL)
	t.Setenv("SAP_USER", "TESTUSER")
	t.Setenv("SAP_PASSWORD", "secret")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("VSP_CACHE", "false")
	t.Chdir(t.TempDir())
	saved := systemName
	systemName = ""
	t.Cleanup(func() { systemName = saved })
}

// setExampleFlag sets a string flag of the examples command for one test.
func setExampleFlag(t *testing.T, name, value string) {
	t.Helper()
	saved, _ := examplesCmd.Flags().GetString(name)
	if err := examplesCmd.Flags().Set(name, value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = examplesCmd.Flags().Set(name, saved) })
}

// setExampleSubmit sets the boolean --submit flag for one test.
func setExampleSubmit(t *testing.T) {
	t.Helper()
	saved, _ := examplesCmd.Flags().GetBool("submit")
	if err := examplesCmd.Flags().Set("submit", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = examplesCmd.Flags().Set("submit", fmt.Sprint(saved)) })
}

// captureStderr is captureStdout's stderr half.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()
	f()
	_ = w.Close()
	os.Stderr = orig
	return <-done
}

// runExamplesCaptured runs the real handler and reports its output and error.
// main.go turns the error into exit 1, which a test cannot observe.
func runExamplesCaptured(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	stdout = captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			err = runExamples(examplesCmd, args)
		})
	})
	return stdout, stderr, err
}

// The expected codes are the recorded CROSS vocabulary, written as literals: a
// test that read them from the production constants would follow a changed
// constant instead of catching it.
func TestExamplesAskForTheRecordedTypeCode(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		submit   bool
		form     string
		wantCode string
		wantName string
	}{
		{name: "function module", args: []string{"FUNC", "ZDEMO_FM"}, wantCode: "F", wantName: "ZDEMO_FM"},
		{name: "submit", args: []string{"PROG", "ZDEMO_PROG"}, submit: true, wantCode: "R", wantName: "ZDEMO_PROG"},
		{name: "form", args: []string{"PROG", "ZDEMO_PROG"}, form: "ZDEMO_FORM", wantCode: "U", wantName: "ZDEMO_FORM"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, stub := newExamplesStub(t, 0)
			examplesAgainst(t, srv)
			if tc.submit {
				setExampleSubmit(t)
			}
			if tc.form != "" {
				setExampleFlag(t, "form", tc.form)
			}

			_, _, _ = runExamplesCaptured(t, tc.args...)

			var cross string
			for _, q := range stub.sent() {
				if strings.Contains(q, " FROM CROSS ") {
					cross = q
				}
			}
			if cross == "" {
				t.Fatalf("no CROSS query was sent; sent: %v", stub.sent())
			}
			if want := "TYPE = '" + tc.wantCode + "'"; !strings.Contains(cross, want) {
				t.Errorf("CROSS query does not ask for %s: %s", want, cross)
			}
			if want := "NAME = '" + tc.wantName + "'"; !strings.Contains(cross, want) {
				t.Errorf("CROSS query does not name %s: %s", want, cross)
			}
		})
	}
}

// FUNC asks one table, so a refusal there means nothing was read. The verdict
// must say so rather than report a search that never happened.
func TestExamplesReportARefusedQueryInsteadOfNoCallers(t *testing.T) {
	srv, _ := newExamplesStub(t, 0, "CROSS")
	examplesAgainst(t, srv)

	stdout, stderr, err := runExamplesCaptured(t, "FUNC", "ZDEMO_FM")
	if err == nil {
		t.Fatal("the only source was refused and the command reported success")
	}
	if !strings.Contains(err.Error(), "CROSS") {
		t.Errorf("the failure does not name what could not be read: %v", err)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("the server's own status did not survive: %v", err)
	}
	if !strings.Contains(err.Error(), "1 of 1") {
		t.Errorf("the failure does not count what was attempted: %v", err)
	}
	if strings.Contains(stdout, "No callers found.") {
		t.Errorf("a refused query was printed as an empty answer:\n%s", stdout)
	}
	if strings.Contains(stderr, "Found 0 callers.") {
		t.Errorf("a refused query printed a caller count:\n%s", stderr)
	}
}

// CLAS asks two tables, and each has its own failure path, so both are
// exercised. Together with the FUNC case above — which expects 1 of 1 — these
// verify the count is the tables actually attempted.
func TestExamplesCountWhatWasAttempted(t *testing.T) {
	cases := []struct {
		name   string
		refuse string
	}{
		{name: "CROSS fails", refuse: "CROSS"},
		{name: "WBCROSSGT fails", refuse: "WBCROSSGT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newExamplesStub(t, 1, tc.refuse)
			examplesAgainst(t, srv)
			setExampleFlag(t, "method", "RUN")

			stdout, stderr, err := runExamplesCaptured(t, "CLAS", "ZCL_DEMO")
			if err == nil {
				t.Fatalf("%s and the command reported success", tc.refuse)
			}
			if !strings.Contains(err.Error(), tc.refuse) {
				t.Errorf("the failure must name %s: %v", tc.refuse, err)
			}
			if !strings.Contains(err.Error(), "1 of 2") {
				t.Errorf("the count must be what was attempted: %v", err)
			}
			if strings.Contains(stdout, "No callers found.") {
				t.Errorf("a refused query was reported as an empty answer:\n%s", stdout)
			}
			if strings.Contains(stderr, "Found 1 callers.") {
				t.Errorf("a refused query still produced a caller verdict:\n%s", stderr)
			}
		})
	}
}

// The other side of the rule: a query that answered and matched nothing is a
// real answer, not a failure.
func TestExamplesReportNoCallersOnlyWhenTheQueryAnswered(t *testing.T) {
	srv, _ := newExamplesStub(t, 0)
	examplesAgainst(t, srv)

	stdout, _, err := runExamplesCaptured(t, "FUNC", "ZDEMO_FM")
	if err != nil {
		t.Fatalf("a query that answered is not a failure: %v", err)
	}
	if !strings.Contains(stdout, "No callers found.") {
		t.Errorf("an answered query with no matches should say so:\n%s", stdout)
	}
}
