package adt

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- line mapping ---

func TestCheckABAPFindingsAreLinesOfTheSnippet(t *testing.T) {
	code := "DATA ls TYPE t000.\nDATA(s) = |{ ls }|."
	source := executeWrapperSource("ZVSP_CHK_1", "RISK LEVEL HARMLESS", "lv_result", code)
	offset := payloadOffset(source)
	// The wrapper line that holds the snippet's second line must be the one
	// that reads it — otherwise the arithmetic below proves nothing.
	if got := strings.Split(source, "\n")[offset]; got != "DATA(s) = |{ ls }|." {
		t.Fatalf("wrapper line %d = %q, want the snippet's second line", offset+1, got)
	}

	uri := "/sap/bc/adt/programs/includes/zvsp_chk_1/source/main?context=%2fsap%2fbc%2fadt%2fprograms%2fprograms%2fzvsp_chk_1"
	findings := checkABAPFindings([]SyntaxCheckResult{
		{URI: uri, Line: offset + 1, Offset: 13, Severity: "E", Text: `"LS" cannot be converted to a character-like value.`},
	}, "ZVSP_CHK_1", offset, 2)

	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one", findings)
	}
	f := findings[0]
	if f.Line != 2 || f.Column != 14 || f.Severity != "error" || f.WrapperLine != 0 {
		t.Fatalf("finding = %+v, want line 2, column 14 (1-based), error", f)
	}
}

func TestCheckABAPFindingsOutsideTheSnippetKeepTheWrapperLine(t *testing.T) {
	// An IF the snippet never closed is reported at ENDMETHOD, two lines after
	// the snippet's last line. "Your line 4" for a one-line snippet would send
	// the caller to a line they never wrote.
	findings := checkABAPFindings([]SyntaxCheckResult{
		{URI: "/sap/bc/adt/programs/includes/zvsp_chk_1/source/main", Line: 24, Offset: 2, Severity: "E", Text: "Incorrect nesting"},
	}, "ZVSP_CHK_1", 18, 1)
	if f := findings[0]; f.Line != 0 || f.WrapperLine != 24 || !f.AfterSnippet {
		t.Fatalf("finding = %+v, want snippet line 0, wrapper line 24, after the snippet", f)
	}
}

func TestCheckABAPFindingsInThePreambleAreNotAfterTheSnippet(t *testing.T) {
	findings := checkABAPFindings([]SyntaxCheckResult{
		{URI: "/sap/bc/adt/programs/includes/zvsp_chk_1/source/main", Line: 3, Severity: "W", Text: "preamble"},
	}, "ZVSP_CHK_1", 18, 1)
	if f := findings[0]; f.Line != 0 || f.WrapperLine != 3 || f.AfterSnippet {
		t.Fatalf("finding = %+v, want wrapper line 3 and not after the snippet", f)
	}
}

func TestCheckABAPFindingsRefuseAnotherObjectsLine(t *testing.T) {
	findings := checkABAPFindings([]SyntaxCheckResult{
		{URI: "/sap/bc/adt/oo/classes/zcl_other/source/main", Line: 19, Offset: 4, Severity: "W", Text: "elsewhere"},
	}, "ZVSP_CHK_1", 18, 5)
	if f := findings[0]; f.Line != 0 || f.WrapperLine != 0 || f.Column != 0 || f.Severity != "warning" {
		t.Fatalf("finding = %+v, want no position at all and severity warning", f)
	}
}

func TestCheckSeverityCountsTheUnknownAsAnError(t *testing.T) {
	for in, want := range map[string]string{"E": "error", "W": "warning", "I": "info", "A": "error", "": "error"} {
		if got := checkSeverity(in); got != want {
			t.Errorf("checkSeverity(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- result parsing ---

func TestCheckRunProcessedRefusesAnUncheckedReport(t *testing.T) {
	// What SAP answers, under a 200, for content it did not check: no
	// messages, which read naively is a clean snippet.
	notProcessed := []byte(`<?xml version="1.0" encoding="utf-8"?><chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun"><chkrun:checkReport chkrun:reporter="abapCheckRun" chkrun:triggeringUri="/sap/bc/adt/oo/classes/zcl_x" chkrun:status="notProcessed" chkrun:statusText="Resource CLASS ZCL_X does not exist."/></chkrun:checkRunReports>`)
	err := checkRunProcessed(notProcessed)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("checkRunProcessed = %v, want an error carrying SAP's status text", err)
	}

	processed := []byte(`<?xml version="1.0" encoding="utf-8"?><chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun"><chkrun:checkReport chkrun:reporter="abapCheckRun" chkrun:status="processed" chkrun:statusText="Object ZX has been checked"/></chkrun:checkRunReports>`)
	if err := checkRunProcessed(processed); err != nil {
		t.Fatalf("checkRunProcessed(processed) = %v", err)
	}
	if err := checkRunProcessed([]byte(`<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun"/>`)); err == nil {
		t.Fatal("an answer with no report at all was taken as a check")
	}
	// A report that does not say it was processed is not taken as one.
	if err := checkRunProcessed([]byte(`<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun"><chkrun:checkReport chkrun:reporter="abapCheckRun"/></chkrun:checkRunReports>`)); err == nil || !strings.Contains(err.Error(), "no status") {
		t.Fatalf("checkRunProcessed(no status) = %v, want an error", err)
	}
}

// --- the workflow against a fake SAP ---

const checkRunTypeError = `<?xml version="1.0" encoding="utf-8"?><chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun"><chkrun:checkReport chkrun:reporter="abapCheckRun" chkrun:status="processed" chkrun:statusText="checked"><chkrun:checkMessageList><chkrun:checkMessage chkrun:uri="/sap/bc/adt/programs/includes/{prog}/source/main?context=x#start={line},13" chkrun:type="E" chkrun:shortText="&quot;LS&quot; cannot be converted to a character-like value."/></chkrun:checkMessageList></chkrun:checkReport></chkrun:checkRunReports>`

type checkABAPServer struct {
	cancelOnCreate context.CancelFunc
	checkStatus    int
	createStatus   int
	probeStatus    int
	lockStatus     int
	deleteStatus   int
	cancelOnRun    context.CancelFunc
	// checkBody, when set, is the checkrun answer: given the program's name in
	// lower case and the wrapper line holding the snippet's first line.
	checkBody func(prog string, firstLine int) string

	mu      sync.Mutex
	calls   []string
	content string
}

func (s *checkABAPServer) start(t *testing.T, opts ...Option) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/discovery"):
			w.Header().Set("X-CSRF-Token", "TOKEN")
		case strings.Contains(r.URL.Path, "/repository/nodestructure"):
			// CreateObject's look for a package node: a read, and not part of
			// the sequence these tests pin down.
		case strings.Contains(r.URL.Path, "/checkruns"):
			s.record("checkrun")
			body, _ := io.ReadAll(r.Body)
			if m := regexp.MustCompile(`<chkrun:content>([^<]*)</chkrun:content>`).FindSubmatch(body); m != nil {
				decoded, _ := base64.StdEncoding.DecodeString(string(m[1]))
				s.mu.Lock()
				s.content = string(decoded)
				s.mu.Unlock()
			}
			if s.cancelOnRun != nil {
				s.cancelOnRun()
			}
			if s.checkStatus != 0 {
				w.WriteHeader(s.checkStatus)
				return
			}
			prog := strings.ToLower(regexp.MustCompile(`REPORT (\S+)\.`).FindStringSubmatch(s.content)[1])
			line := payloadOffset(s.content)
			w.Header().Set("Content-Type", "application/xml")
			resp := strings.NewReplacer("{prog}", prog, "{line}", strconv.Itoa(line)).Replace(checkRunTypeError)
			if s.checkBody != nil {
				resp = s.checkBody(prog, line)
			}
			_, _ = w.Write([]byte(resp))
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			s.record("lock")
			if s.lockStatus != 0 {
				w.WriteHeader(s.lockStatus)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><LOCK_HANDLE>HANDLE-1</LOCK_HANDLE></DATA></asx:values></asx:abap>`))
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "UNLOCK":
			s.record("unlock")
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/programs/programs"):
			s.record("create")
			if s.cancelOnCreate != nil {
				// SAP commits the program, and the caller gives up before
				// the answer arrives.
				s.cancelOnCreate()
				time.Sleep(50 * time.Millisecond)
			}
			if s.createStatus != 0 {
				w.WriteHeader(s.createStatus)
			}
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/programs/programs/"):
			// The existence probe after a failed create: the program is
			// there, unless probeStatus says otherwise.
			s.record("probe")
			if s.probeStatus != 0 {
				w.WriteHeader(s.probeStatus)
			}
		case r.Method == http.MethodDelete:
			s.record("delete")
			if s.deleteStatus != 0 {
				w.WriteHeader(s.deleteStatus)
			}
		default:
			s.record(r.Method + " " + r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := NewConfig(srv.URL, "TESTUSER", "secret", opts...)
	return NewClientWithTransport(cfg, NewTransport(cfg))
}

func (s *checkABAPServer) record(call string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
}

func (s *checkABAPServer) sequence() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.calls, ",")
}

func TestCheckABAPChecksInATemporaryProgramAndRemovesIt(t *testing.T) {
	srv := &checkABAPServer{}
	result, err := srv.start(t).CheckABAP(context.Background(), "DATA ls TYPE t000.\nDATA(s) = |{ ls }|.")
	if err != nil {
		t.Fatalf("CheckABAP: %v", err)
	}
	// Create, check, delete — and nothing else: no source PUT, no
	// activation, no unit test run.
	if got, want := srv.sequence(), "create,checkrun,lock,delete"; got != want {
		t.Fatalf("requests = %s, want %s", got, want)
	}
	// The check saw the snippet inside the very wrapper ExecuteABAP uses.
	if want := executeWrapperSource(result.ProgramName, "RISK LEVEL HARMLESS", "lv_result", "DATA ls TYPE t000.\nDATA(s) = |{ ls }|."); srv.content != want {
		t.Fatalf("checked content differs from the execute wrapper:\n%s", srv.content)
	}
	if result.OK || !result.CleanedUp {
		t.Fatalf("result = %+v, want not OK and cleaned up", result)
	}
	if len(result.Findings) != 1 || result.Findings[0].Line != 1 || result.Findings[0].Column != 14 {
		t.Fatalf("findings = %+v, want one at snippet line 1, column 14", result.Findings)
	}
}

func TestCheckABAPRemovesTheProgramWhenTheCheckFails(t *testing.T) {
	srv := &checkABAPServer{checkStatus: http.StatusInternalServerError}
	result, err := srv.start(t).CheckABAP(context.Background(), "DATA lv TYPE i.")
	if err == nil {
		t.Fatal("a failed check run was reported as a check")
	}
	if got, want := srv.sequence(), "create,checkrun,lock,delete"; got != want {
		t.Fatalf("requests = %s, want %s: the temporary program must go even when the check fails", got, want)
	}
	if result == nil || !result.CleanedUp {
		t.Fatalf("result = %+v, want CleanedUp after a failed check", result)
	}
}

func TestCheckABAPRemovesTheProgramAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &checkABAPServer{cancelOnRun: cancel}
	result, _ := srv.start(t).CheckABAP(ctx, "DATA lv TYPE i.")
	if ctx.Err() == nil {
		t.Fatal("test setup did not cancel the context during the check")
	}
	if !strings.HasSuffix(srv.sequence(), "lock,delete") || result == nil || !result.CleanedUp {
		t.Fatalf("requests = %s, result = %+v: cleanup must outlive the caller's cancellation", srv.sequence(), result)
	}
}

func TestCheckABAPCreatesNothingItCouldNotDelete(t *testing.T) {
	// Allowed to create but not to delete would leave the program behind on
	// every call, so the delete is gated before the create is sent.
	srv := &checkABAPServer{}
	_, err := srv.start(t, WithSafety(SafetyConfig{DisallowedOps: "D"})).CheckABAP(context.Background(), "DATA lv TYPE i.")
	if err == nil {
		t.Fatal("CheckABAP ran although deletion is not allowed")
	}
	if got := srv.sequence(); got != "" {
		t.Fatalf("requests = %s, want none", got)
	}
}

// --- cleanup that fails ---

func TestCheckABAPReportsAFailedDeleteAndStillUnlocks(t *testing.T) {
	// The check fails too, so the cleanup warning has to ride on the error.
	srv := &checkABAPServer{checkStatus: http.StatusInternalServerError, deleteStatus: http.StatusInternalServerError}
	result, err := srv.start(t).CheckABAP(context.Background(), "DATA lv TYPE i.")
	if err == nil {
		t.Fatal("a failed check run was reported as a check")
	}
	// The DELETE is not retried, and the lock it held is given back.
	if got, want := srv.sequence(), "create,checkrun,lock,delete,unlock"; got != want {
		t.Fatalf("requests = %s, want %s", got, want)
	}
	if result == nil || result.CleanedUp {
		t.Fatalf("result = %+v, want cleanedUp=false after a failed DELETE", result)
	}
	if len(result.Warnings) != 1 ||
		!strings.Contains(result.Warnings[0], "DELETE outcome is unknown and was not retried") ||
		!strings.Contains(result.Warnings[0], result.ProgramName) {
		t.Fatalf("warnings = %q, want one naming %s and the unknown DELETE", result.Warnings, result.ProgramName)
	}
	if want := "(cleanup: " + result.Warnings[0] + ")"; !strings.HasSuffix(err.Error(), want) {
		t.Fatalf("error = %q, want it to end with %q", err, want)
	}
	if !strings.HasPrefix(err.Error(), "syntax check failed") {
		t.Fatalf("error = %q: the check's own failure must stay first", err)
	}
}

func TestCheckABAPReportsAFailedLock(t *testing.T) {
	srv := &checkABAPServer{checkStatus: http.StatusInternalServerError, lockStatus: http.StatusInternalServerError, deleteStatus: http.StatusInternalServerError}
	result, err := srv.start(t).CheckABAP(context.Background(), "DATA lv TYPE i.")
	if got, want := srv.sequence(), "create,checkrun,lock"; got != want {
		t.Fatalf("requests = %s, want %s: no DELETE without a lock", got, want)
	}
	if result == nil || result.CleanedUp {
		t.Fatalf("result = %+v, want cleanedUp=false after a failed lock", result)
	}
	if len(result.Warnings) != 1 ||
		!strings.Contains(result.Warnings[0], "could not lock the temporary program for cleanup") ||
		!strings.Contains(result.Warnings[0], result.ProgramName) {
		t.Fatalf("warnings = %q, want the lock failure naming %s", result.Warnings, result.ProgramName)
	}
	if err == nil || !strings.HasSuffix(err.Error(), "(cleanup: "+result.Warnings[0]+")") {
		t.Fatalf("error = %v, want the lock failure appended as the cleanup suffix", err)
	}
}

func TestCheckABAPLeftoverAfterACleanCheckIsAWarningNotAnError(t *testing.T) {
	// The check succeeded, so its findings are returned — but cleanedUp says
	// the program is still there, for the CLI and MCP to fail on.
	srv := &checkABAPServer{deleteStatus: http.StatusInternalServerError}
	result, err := srv.start(t).CheckABAP(context.Background(), "DATA lv TYPE i.")
	if err != nil {
		t.Fatalf("CheckABAP: %v", err)
	}
	if result.CleanedUp || len(result.Warnings) == 0 || len(result.Findings) != 1 {
		t.Fatalf("result = %+v, want the findings, cleanedUp=false and a warning", result)
	}
}

// --- the shared delete ---

func TestDeleteTemporaryProgramUnlocksAfterAFailedDelete(t *testing.T) {
	srv := &executeCleanupServer{deleteStatus: http.StatusInternalServerError}
	client := srv.start(t)
	warnings := client.deleteTemporaryProgram(context.Background(), "/sap/bc/adt/programs/programs/zvsp_chk_00000001", "ZVSP_CHK_00000001")
	if got := srv.count("unlock"); got != 1 {
		t.Fatalf("UNLOCK requests = %d, want 1 after the failed DELETE", got)
	}
	if got := srv.count("delete"); got != 1 {
		t.Fatalf("DELETE requests = %d, want exactly 1", got)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "ZVSP_CHK_00000001") {
		t.Fatalf("warnings = %q, want one naming the program", warnings)
	}
}

func TestDeleteTemporaryProgramReportsAFailedLock(t *testing.T) {
	srv := &executeCleanupServer{lockStatus: http.StatusInternalServerError}
	warnings := srv.start(t).deleteTemporaryProgram(context.Background(), "/sap/bc/adt/programs/programs/zvsp_chk_00000001", "ZVSP_CHK_00000001")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "could not lock the temporary program for cleanup") || !strings.Contains(warnings[0], "ZVSP_CHK_00000001") {
		t.Fatalf("warnings = %q, want the lock failure naming the program", warnings)
	}
	if srv.count("delete") != 0 || srv.count("unlock") != 0 {
		t.Fatalf("DELETE/UNLOCK sent without a lock")
	}
}

// --- names and collisions ---

func TestCheckABAPNeverDeletesAnObjectItCannotShowItCreated(t *testing.T) {
	// The create fails with something other than "already exists", and the
	// probe then finds a program of that name. It may be ours, committed
	// before the failure; it may be someone else's. It is reported, not
	// deleted, and not locked.
	srv := &checkABAPServer{createStatus: http.StatusInternalServerError}
	result, err := srv.start(t).CheckABAP(context.Background(), "DATA lv TYPE i.")
	if got, want := srv.sequence(), "create,probe"; got != want {
		t.Fatalf("requests = %s, want %s: no lock or delete of an object not shown to be ours", got, want)
	}
	if result != nil {
		t.Fatalf("result = %+v, want none after a failed create", result)
	}
	var pce *PartialCreateError
	if !errors.As(err, &pce) || !pce.LeftInPlace || pce.CleanupOK {
		t.Fatalf("error = %v, want a PartialCreateError left in place", err)
	}
	if !strings.Contains(err.Error(), "ZVSP_CHK_") || !strings.Contains(err.Error(), "left in place") {
		t.Fatalf("error = %q, want the program named and said to be left in place", err)
	}
}

func TestExecuteABAPNeverDeletesAnObjectItCannotShowItCreated(t *testing.T) {
	srv := &executeCleanupServer{createStatus: http.StatusInternalServerError}
	result, err := srv.start(t).ExecuteABAP(context.Background(), "lv_result = 'ok'.", nil)
	if err != nil {
		t.Fatalf("ExecuteABAP: %v", err)
	}
	if srv.count("lock") != 0 || srv.count("delete") != 0 {
		t.Fatalf("lock=%d delete=%d after a failed create, want none", srv.count("lock"), srv.count("delete"))
	}
	if result.CleanedUp || !strings.Contains(result.Message, result.ProgramName) || !strings.Contains(result.Message, "left in place") {
		t.Fatalf("result = %+v, want the program named and left in place", result)
	}
}

// --- wiring through the workflow ---

func TestCheckABAPRefusesANotProcessedRun(t *testing.T) {
	srv := &checkABAPServer{checkBody: func(prog string, _ int) string {
		return `<?xml version="1.0" encoding="utf-8"?><chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun"><chkrun:checkReport chkrun:reporter="abapCheckRun" chkrun:status="notProcessed" chkrun:statusText="Resource PROGRAM ` + strings.ToUpper(prog) + ` was not checked."/></chkrun:checkRunReports>`
	}}
	result, err := srv.start(t).CheckABAP(context.Background(), "DATA lv TYPE i.")
	if err == nil || !strings.Contains(err.Error(), "notProcessed") || !strings.Contains(err.Error(), "was not checked") {
		t.Fatalf("error = %v, want SAP's notProcessed status and text", err)
	}
	if result == nil || result.OK || !result.CleanedUp {
		t.Fatalf("result = %+v, want not OK and cleaned up", result)
	}
	if got, want := srv.sequence(), "create,checkrun,lock,delete"; got != want {
		t.Fatalf("requests = %s, want %s", got, want)
	}
}

func checkRunWith(prog string, messages ...string) string {
	return `<?xml version="1.0" encoding="utf-8"?><chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun"><chkrun:checkReport chkrun:reporter="abapCheckRun" chkrun:status="processed" chkrun:statusText="checked"><chkrun:checkMessageList>` +
		strings.Join(messages, "") + `</chkrun:checkMessageList></chkrun:checkReport></chkrun:checkRunReports>`
}

func checkMessageAt(prog string, line, col int, typ, text string) string {
	return fmt.Sprintf(`<chkrun:checkMessage chkrun:uri="/sap/bc/adt/programs/includes/%s/source/main?context=x#start=%d,%d" chkrun:type="%s" chkrun:shortText="%s"/>`, prog, line, col, typ, text)
}

func TestCheckABAPIsOKWithOnlyWarnings(t *testing.T) {
	srv := &checkABAPServer{checkBody: func(prog string, first int) string {
		return checkRunWith(prog,
			checkMessageAt(prog, first, 0, "W", "The variable LV is not used."),
			checkMessageAt(prog, first, 0, "I", "Just so you know."))
	}}
	result, err := srv.start(t).CheckABAP(context.Background(), "DATA lv TYPE i.")
	if err != nil {
		t.Fatalf("CheckABAP: %v", err)
	}
	if !result.OK || len(result.Findings) != 2 || result.Findings[0].Severity != "warning" || result.Findings[1].Severity != "info" {
		t.Fatalf("result = %+v, want OK with a warning and an info", result)
	}
}

func TestCheckABAPMapsAFindingOnTheThirdLineOfFour(t *testing.T) {
	code := "DATA lv TYPE i.\nlv = 1.\nDATA(s) = |{ ls }|.\nlv = 2."
	srv := &checkABAPServer{checkBody: func(prog string, first int) string {
		return checkRunWith(prog, checkMessageAt(prog, first+2, 13, "E", "Field &quot;LS&quot; is unknown."))
	}}
	result, err := srv.start(t).CheckABAP(context.Background(), code)
	if err != nil {
		t.Fatalf("CheckABAP: %v", err)
	}
	// The wrapper line the fake SAP pointed at really is the snippet's third.
	if got := strings.Split(srv.content, "\n")[payloadOffset(srv.content)+1]; got != "DATA(s) = |{ ls }|." {
		t.Fatalf("wrapper line under test = %q, want the snippet's third line", got)
	}
	if result.OK || len(result.Findings) != 1 {
		t.Fatalf("result = %+v, want one error", result)
	}
	if f := result.Findings[0]; f.Line != 3 || f.Column != 14 || f.WrapperLine != 0 || f.AfterSnippet {
		t.Fatalf("finding = %+v, want snippet line 3, column 14", f)
	}
}

func TestCheckABAPFindingsMatchTheProgramInThePathOnly(t *testing.T) {
	// Another include, checked in the temporary program's context: the
	// program's name is in the query, and that does not make the line ours.
	findings := checkABAPFindings([]SyntaxCheckResult{
		{URI: "/sap/bc/adt/programs/includes/zdemo_incl/source/main?context=%2fsap%2fbc%2fadt%2fprograms%2fprograms%2fzvsp_chk_1", Line: 19, Offset: 4, Severity: "E", Text: "elsewhere"},
		{URI: "/sap/bc/adt/programs/programs/zvsp_chk_1/source/main", Line: 19, Offset: 4, Severity: "E", Text: "ours"},
		{URI: "/sap/bc/adt/programs/programs/zvsp_chk_12/source/main", Line: 19, Offset: 4, Severity: "E", Text: "a longer name"},
		{URI: "/sap/bc/adt/programs/includes/zdemo_incl/source/main?context=/sap/bc/adt/programs/programs/zvsp_chk_1", Line: 19, Offset: 4, Severity: "E", Text: "unescaped query"},
	}, "ZVSP_CHK_1", 18, 5)
	if f := findings[0]; f.Line != 0 || f.WrapperLine != 0 || f.Column != 0 {
		t.Fatalf("finding = %+v: a name in the query must not place the message", f)
	}
	if f := findings[1]; f.Line != 2 {
		t.Fatalf("finding = %+v, want snippet line 2", f)
	}
	if f := findings[2]; f.Line != 0 || f.WrapperLine != 0 {
		t.Fatalf("finding = %+v: a program whose name merely starts with ours is not ours", f)
	}
	if f := findings[3]; f.Line != 0 || f.WrapperLine != 0 {
		t.Fatalf("finding = %+v: a name in an unescaped query must not place the message either", f)
	}
}

func TestCheckABAPReportsAProgramCreatedAsTheCallerCancelled(t *testing.T) {
	// Ctrl-C during the create: the POST may have landed. The probe has to
	// get out despite the cancellation, so the program is reported by name
	// rather than left behind in silence — and it is still not deleted.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &checkABAPServer{cancelOnCreate: cancel}
	_, err := srv.start(t).CheckABAP(ctx, "DATA lv TYPE i.")
	if got, want := srv.sequence(), "create,probe"; got != want {
		t.Fatalf("requests = %s, want %s", got, want)
	}
	var pce *PartialCreateError
	if !errors.As(err, &pce) || !pce.LeftInPlace {
		t.Fatalf("error = %v, want the program reported as left in place", err)
	}
}

func TestCheckABAPWarnsThatAnInterruptedCreateMayStillLand(t *testing.T) {
	// The probe answers 404, but the create was cut off by the caller, and
	// SAP can still commit it afterwards: the program is named regardless.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &checkABAPServer{cancelOnCreate: cancel, probeStatus: http.StatusNotFound}
	_, err := srv.start(t).CheckABAP(ctx, "DATA lv TYPE i.")
	if err == nil || !strings.Contains(err.Error(), "SAP may still complete it") || !strings.Contains(err.Error(), `look for ZVSP_CHK_TESTUSER_0 in $TMP`) {
		t.Fatalf("error = %v, want the program named as possibly created", err)
	}
	if got, want := srv.sequence(), "create,probe"; got != want {
		t.Fatalf("requests = %s, want %s", got, want)
	}
}
