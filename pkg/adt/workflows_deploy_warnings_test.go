package adt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBlockingSyntaxErrors(t *testing.T) {
	in := []SyntaxCheckResult{
		{Line: 1, Severity: "E", Text: "error"},
		{Line: 2, Severity: "W", Text: "warning"},
		{Line: 3, Severity: "I", Text: "info"},
		{Line: 4, Severity: "A", Text: "abort"},
		{Line: 5, Severity: "X", Text: "exit"},
		{Line: 6, Severity: "", Text: "unclassified"},
	}
	got := blockingSyntaxErrors(in)
	want := []SyntaxCheckResult{in[0], in[3], in[4]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("blockingSyntaxErrors = %v, want %v", got, want)
	}
	if got := blockingSyntaxErrors(nil); len(got) != 0 {
		t.Fatalf("blockingSyntaxErrors(nil) = %v, want empty", got)
	}
}

// checkRunWithMessages renders a check run answer with one message per
// severity given.
func checkRunWithMessages(objectPath string, severities ...string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0"?>
<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun">
  <chkrun:checkReport chkrun:reporter="abapCheckRun">
    <chkrun:checkMessageList>`)
	for i, sev := range severities {
		fmt.Fprintf(&sb, `
      <chkrun:checkMessage chkrun:uri="%s/source/main#start=%d,0" chkrun:type="%s" chkrun:shortText="Message %d"/>`,
			objectPath, i+1, sev, i+1)
	}
	sb.WriteString(`
    </chkrun:checkMessageList>
  </chkrun:checkReport>
</chkrun:checkRunReports>`)
	return sb.String()
}

// deployWarningsClient stands in for SAP over a deploy of ZDEMO_REPORT. The
// object exists unless missing is set, and the check run answers with the
// given severities.
func deployWarningsClient(t *testing.T, rec *adtRecorder, missing bool, severities ...string) (*Client, string) {
	t.Helper()
	const objectPath = "/sap/bc/adt/programs/programs/zdemo_report"

	file := filepath.Join(t.TempDir(), "zdemo_report.prog.abap")
	if err := os.WriteFile(file, []byte("REPORT zdemo_report.\nWRITE 'x'.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var probed atomic.Bool
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/checkruns"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, checkRunWithMessages(objectPath, severities...))
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockXML)
		case missing && r.Method == http.MethodGet && r.URL.Path == objectPath && !probed.Swap(true):
			// Only the existence probe finds nothing; the object exists once created.
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusOK)
		}
	})
	return client, file
}

func TestDeployFromFile_UpdateWithWarningsOnlyWrites(t *testing.T) {
	rec := &adtRecorder{}
	client, file := deployWarningsClient(t, rec, false, "W", "W")

	result, err := client.DeployFromFile(context.Background(), file, "$TMP", "")
	if err != nil {
		t.Fatalf("DeployFromFile: %v", err)
	}
	if !result.Success {
		t.Fatalf("warnings must not stop the deploy; message=%q", result.Message)
	}
	if indexOfCall(rec.snapshot(), isSourcePut) < 0 {
		t.Fatalf("expected a PUT on /source/main; trace:\n%v", rec.snapshot())
	}
	if len(result.SyntaxWarnings) != 2 {
		t.Fatalf("SyntaxWarnings = %v, want 2 entries", result.SyntaxWarnings)
	}
	if len(result.SyntaxErrors) != 0 {
		t.Fatalf("SyntaxErrors = %v, want none", result.SyntaxErrors)
	}
}

func TestDeployFromFile_UpdateWithErrorAndWarningRefuses(t *testing.T) {
	rec := &adtRecorder{}
	client, file := deployWarningsClient(t, rec, false, "E", "W")

	result, err := client.DeployFromFile(context.Background(), file, "$TMP", "")
	if err != nil {
		t.Fatalf("DeployFromFile: %v", err)
	}
	if result.Success {
		t.Fatal("a syntax error must still stop the deploy")
	}
	calls := rec.snapshot()
	if indexOfCall(calls, isLock) >= 0 || indexOfCall(calls, isSourcePut) >= 0 {
		t.Fatalf("no LOCK or PUT expected after a syntax error; trace:\n%v", calls)
	}
	if len(result.SyntaxErrors) != 1 {
		t.Fatalf("SyntaxErrors = %v, want 1 entry", result.SyntaxErrors)
	}
	if len(result.SyntaxWarnings) != 1 {
		t.Fatalf("SyntaxWarnings = %v, want 1 entry", result.SyntaxWarnings)
	}
}

func TestDeployFromFile_CreateWithWarningsOnlyContinues(t *testing.T) {
	rec := &adtRecorder{}
	client, file := deployWarningsClient(t, rec, true, "W")

	result, err := client.DeployFromFile(context.Background(), file, "$TMP", "")
	if err != nil {
		t.Fatalf("DeployFromFile: %v", err)
	}
	if strings.Contains(result.Message, "syntax errors") {
		t.Fatalf("a warning must not be reported as a syntax error; message=%q", result.Message)
	}
	if !result.Success || !result.Created {
		t.Fatalf("warnings must not stop the create; Success=%v Created=%v message=%q", result.Success, result.Created, result.Message)
	}
	if indexOfCall(rec.snapshot(), isSourcePut) < 0 {
		t.Fatalf("expected the flow to reach the write; trace:\n%v", rec.snapshot())
	}
	if len(result.SyntaxWarnings) != 1 {
		t.Fatalf("SyntaxWarnings = %v, want 1 entry", result.SyntaxWarnings)
	}
}
