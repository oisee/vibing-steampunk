package adt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// --- Session-affinity harness (issue #91) ---
//
// The existing helpers in crud_reconcile_test.go answer "was this one request
// stateful?". This bug is about a *window*: every request between the LOCK and
// the write that consumes the handle has to stay on the same ADT session, and
// methodPathMock cannot tell a LOCK POST from an UNLOCK POST because it does
// not look at the query string. So these tests drive a real httptest server and
// record method, path, query and X-sap-adt-sessiontype in request order.

// wireCall is one outbound request as the server saw it.
type wireCall struct {
	method      string
	path        string
	query       url.Values
	sessionType string
}

func (c wireCall) String() string {
	return fmt.Sprintf("%-6s %s?%s sessiontype=%q", c.method, c.path, c.query.Encode(), c.sessionType)
}

// adtRecorder wraps an httptest handler and records what reached it.
type adtRecorder struct {
	mu    sync.Mutex
	calls []wireCall
}

func (r *adtRecorder) handler(route http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.calls = append(r.calls, wireCall{
			method:      req.Method,
			path:        req.URL.Path,
			query:       req.URL.Query(),
			sessionType: req.Header.Get("X-sap-adt-sessiontype"),
		})
		r.mu.Unlock()

		// Every ADT reply carries a token, so the CSRF probe stays out of the
		// trace unless a test deliberately provokes it.
		w.Header().Set("X-CSRF-Token", "TOKEN")
		route(w, req)
	}
}

func (r *adtRecorder) snapshot() []wireCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]wireCall, len(r.calls))
	copy(out, r.calls)
	return out
}

// dump prints the whole trace; every failure in this file wants it.
func dumpCalls(t *testing.T, calls []wireCall) {
	t.Helper()
	for i, c := range calls {
		t.Logf("  [%d] %s", i, c)
	}
}

func indexOfCall(calls []wireCall, pred func(wireCall) bool) int {
	for i, c := range calls {
		if pred(c) {
			return i
		}
	}
	return -1
}

func isLock(c wireCall) bool {
	return c.method == http.MethodPost && c.query.Get("_action") == "LOCK"
}

func isUnlock(c wireCall) bool {
	return c.method == http.MethodPost && c.query.Get("_action") == "UNLOCK"
}

func isSourcePut(c wireCall) bool {
	return c.method == http.MethodPut && strings.HasSuffix(c.path, "/source/main")
}

// assertWindowStateful is the assertion this whole bug reduces to: nothing
// between the LOCK and the request that consumes its handle may leave the
// stateful session.
//
// It is written as `!= "stateful"` rather than `== "stateless"` on purpose —
// probeCSRFToken builds its request by hand and used to send no
// X-sap-adt-sessiontype at all, which ICM treats as stateless just the same.
func assertWindowStateful(t *testing.T, calls []wireCall, from, to int) {
	t.Helper()
	for _, c := range calls[from+1 : to] {
		if c.sessionType != "stateful" {
			t.Errorf("request inside the lock window is not stateful: %s\n"+
				"  it retires the ADT session the lock handle lives in, so the write returns\n"+
				"  423 ExceptionResourceInvalidLockHandle (issue #91)", c)
		}
	}
	if t.Failed() {
		dumpCalls(t, calls)
	}
}

const testLockXML = `<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>
<LOCK_HANDLE>HANDLE-1</LOCK_HANDLE><IS_LOCAL>X</IS_LOCAL>
<MODIFICATION_SUPPORT>NoModification</MODIFICATION_SUPPORT>
</DATA></asx:values></asx:abap>`

const testEmptyCheckXML = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist"/>`

// searchXMLFor renders the quickSearch answer that resolves one object to one
// package. getObjectPackage matches on the canonicalised URI, so this fixture
// has to name the same object the test is mutating or the gate fails for an
// unrelated reason and the real assertion never runs.
func searchXMLFor(uri, name, pkg string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="%s" adtcore:type="PROG/P" adtcore:name="%s" adtcore:packageName="%s"/>
</adtcore:objectReferences>`, uri, name, pkg)
}

// newStubbedClient wires a client to a recording httptest server.
func newStubbedClient(t *testing.T, rec *adtRecorder, route http.HandlerFunc, opts ...Option) *Client {
	t.Helper()
	srv := httptest.NewServer(rec.handler(route))
	t.Cleanup(srv.Close)

	cfg := NewConfig(srv.URL, "TESTUSER", "secret", opts...)
	return NewClientWithTransport(cfg, NewTransport(cfg))
}

// --- The primary regression: the package lookup inside the lock window ---

func TestWriteProgram_NoStatelessRequestBetweenLockAndPut(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			_, _ = io.WriteString(w, searchXMLFor(
				"/sap/bc/adt/programs/programs/zdemo_probe", "ZDEMO_PROBE", "$TMP"))
		case strings.Contains(r.URL.Path, "/checkruns"):
			w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
			_, _ = io.WriteString(w, testEmptyCheckXML)
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockXML)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}, WithAllowedPackages("$TMP"))

	if _, err := client.WriteProgram(context.Background(), "ZDEMO_PROBE",
		"REPORT zdemo_probe.\n", ""); err != nil {
		t.Fatalf("WriteProgram: %v", err)
	}

	calls := rec.snapshot()
	lockAt := indexOfCall(calls, isLock)
	putAt := indexOfCall(calls, isSourcePut)
	if lockAt < 0 || putAt < 0 || putAt < lockAt {
		t.Fatalf("expected a LOCK followed by a source PUT; trace:\n%v", calls)
	}
	assertWindowStateful(t, calls, lockAt, putAt)
}

func TestEditSource_NoStatelessRequestBetweenLockAndPut(t *testing.T) {
	const objectURL = "/sap/bc/adt/programs/programs/ZDEMO_EDIT"

	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			_, _ = io.WriteString(w, searchXMLFor(
				"/sap/bc/adt/programs/programs/zdemo_edit", "ZDEMO_EDIT", "$TMP"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/source/main"):
			_, _ = io.WriteString(w, "REPORT zdemo_edit.\nWRITE 'old'.\n")
		case strings.Contains(r.URL.Path, "/checkruns"):
			w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
			_, _ = io.WriteString(w, testEmptyCheckXML)
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockXML)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}, WithAllowedPackages("$TMP"))

	result, err := client.EditSourceWithOptions(context.Background(), objectURL,
		"WRITE 'old'.", "WRITE 'new'.", &EditSourceOptions{SyntaxCheck: false})
	if err != nil {
		t.Fatalf("EditSourceWithOptions: %v", err)
	}
	if !result.Success {
		t.Fatalf("edit did not succeed: %s", result.Message)
	}

	calls := rec.snapshot()
	lockAt := indexOfCall(calls, isLock)
	putAt := indexOfCall(calls, isSourcePut)
	if lockAt < 0 || putAt < 0 || putAt < lockAt {
		t.Fatalf("expected a LOCK followed by a source PUT; trace:\n%v", calls)
	}
	assertWindowStateful(t, calls, lockAt, putAt)

	// And the package must have been resolved exactly once, above the lock —
	// the second lookup was pure waste even when it did not kill the session.
	searches := 0
	for _, c := range calls {
		if strings.Contains(c.path, "informationsystem/search") {
			searches++
		}
	}
	if searches != 1 {
		t.Errorf("package resolved %d times, want 1 (once, above the lock)", searches)
		dumpCalls(t, calls)
	}
}

// --- CreateTable: the mutation that was itself stateless ---

func TestCreateTable_SourcePutStaysInTheLockSession(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockXML)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}, WithAllowedPackages("$TMP"))

	err := client.CreateTable(context.Background(), CreateTableOptions{
		Name:        "ZDEMO_TAB",
		Description: "demo",
		Package:     "$TMP",
		Fields:      []TableField{{Name: "MANDT", Type: "mandt", IsKey: true}},
	})
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	calls := rec.snapshot()
	putAt := indexOfCall(calls, isSourcePut)
	if putAt < 0 {
		t.Fatalf("expected a PUT of the table source; trace:\n%v", calls)
	}
	if got := calls[putAt].sessionType; got != "stateful" {
		t.Errorf("table source PUT X-sap-adt-sessiontype = %q, want \"stateful\" — "+
			"it carries the lock handle from the stateful LOCK three lines above it "+
			"and could never match its own lock (issue #91)", got)
		dumpCalls(t, calls)
	}

	lockAt := indexOfCall(calls, isLock)
	if lockAt < 0 || lockAt > putAt {
		t.Fatalf("expected a LOCK before the source PUT; trace:\n%v", calls)
	}
	assertWindowStateful(t, calls, lockAt, putAt)
}

func TestCreateTable_RefusesPackageOutsideAllowlist(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}, WithAllowedPackages("$TMP"))

	err := client.CreateTable(context.Background(), CreateTableOptions{
		Name:        "ZDEMO_TAB",
		Description: "demo",
		Package:     "ZDEMO_PROD",
		Fields:      []TableField{{Name: "MANDT", Type: "mandt", IsKey: true}},
	})
	if err == nil {
		t.Fatal("CreateTable created a table in ZDEMO_PROD with SAP_ALLOWED_PACKAGES=$TMP — " +
			"the path only ran the op-type check, so --allowed-packages never applied to it")
	}
	if calls := rec.snapshot(); len(calls) != 0 {
		t.Errorf("a blocked CreateTable still talked to SAP:")
		dumpCalls(t, calls)
	}
}

// --- The other unconditionally-stateless mutation ---

func TestWriteMessageClassTexts_PutStaysInTheLockSession(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK" {
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockXML)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	ctx := context.Background()
	lock, err := client.LockObject(ctx, "/sap/bc/adt/messageclass/zdemo_mc", "MODIFY")
	if err != nil {
		t.Fatalf("LockObject: %v", err)
	}
	if err := client.WriteMessageClassTexts(ctx, "ZDEMO_MC", "EN",
		[]MessageClassMessage{{Number: "001", Text: "hello"}}, lock.LockHandle, ""); err != nil {
		t.Fatalf("WriteMessageClassTexts: %v", err)
	}

	calls := rec.snapshot()
	putAt := indexOfCall(calls, func(c wireCall) bool {
		return c.method == http.MethodPut && strings.Contains(c.path, "/messageclass/")
	})
	if putAt < 0 {
		t.Fatalf("expected a PUT of the message class; trace:\n%v", calls)
	}
	if got := calls[putAt].sessionType; got != "stateful" {
		t.Errorf("message class PUT X-sap-adt-sessiontype = %q, want \"stateful\" — "+
			"the lockHandle in its query came from a stateful LOCK (issue #91)", got)
		dumpCalls(t, calls)
	}
}

// --- The one hop no config gates: the CSRF refetch mid-write ---

func TestCSRFRefetchDuringStatefulWriteStaysStateful(t *testing.T) {
	rec := &adtRecorder{}
	var mu sync.Mutex
	putSeen := 0

	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		if isSourcePut(wireCall{method: r.Method, path: r.URL.Path}) {
			mu.Lock()
			putSeen++
			first := putSeen == 1
			mu.Unlock()
			if first {
				// SAP's stock "your token went stale" answer, which sends the
				// transport back to /core/discovery mid-window.
				w.Header().Set("X-CSRF-Token", "Required")
				w.WriteHeader(http.StatusForbidden)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	})

	if err := client.UpdateSource(context.Background(),
		"/sap/bc/adt/programs/programs/ZDEMO_CSRF/source/main",
		"REPORT zdemo_csrf.\n", "HANDLE-1", ""); err != nil {
		t.Fatalf("UpdateSource: %v", err)
	}

	calls := rec.snapshot()
	probes := 0
	for _, c := range calls {
		if !strings.Contains(c.path, "/core/discovery") {
			continue
		}
		probes++
		if c.sessionType != "stateful" {
			t.Errorf("CSRF probe issued for a stateful write is not stateful: %s\n"+
				"  it lands between the failed write and its retry, and the retry then\n"+
				"  presents a handle whose session the probe just retired (issue #91)", c)
		}
	}
	if probes == 0 {
		t.Fatalf("expected a CSRF probe on /core/discovery; trace:\n%v", calls)
	}
	if t.Failed() {
		dumpCalls(t, calls)
	}
}

// --- The marker must not become a policy hole ---

func markerTestClient(t *testing.T, safety SafetyConfig) *Client {
	t.Helper()
	cfg := NewConfig("https://sap.invalid", "TESTUSER", "secret", WithSafety(safety))
	return NewClientWithTransport(cfg, NewTransport(cfg))
}

func TestMutationMarker_StillEnforcesOperationPolicy(t *testing.T) {
	// The outer workflow passes OpWorkflow ('W'); the inner mutator passes
	// OpUpdate ('U'). With 'U' disallowed the inner call must still be
	// refused — this is precisely what PR #125's all-or-nothing early return
	// would have let through.
	client := markerTestClient(t, SafetyConfig{
		AllowedPackages: []string{"$TMP"},
		DisallowedOps:   "U",
	})

	const objectURL = "/sap/bc/adt/programs/programs/ZDEMO_MARK"
	ctx := withMutationPackageChecked(context.Background(), objectURL)

	err := client.checkMutation(ctx, MutationContext{
		Op:        OpUpdate,
		OpName:    "UpdateSource",
		ObjectURL: objectURL + "/source/main",
	})
	if err == nil {
		t.Fatal("a marked context skipped the operation-type check: --disallowed-ops U " +
			"no longer blocks UpdateSource once an outer OpWorkflow gate has run")
	}
	if !strings.Contains(err.Error(), "blocked by safety configuration") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMutationMarker_StillEnforcesTransportPolicy(t *testing.T) {
	client := markerTestClient(t, SafetyConfig{
		AllowedPackages: []string{"$TMP"},
		// AllowTransportableEdits stays false: a transport-bound edit is refused.
	})

	const objectURL = "/sap/bc/adt/programs/programs/ZDEMO_MARK"
	ctx := withMutationPackageChecked(context.Background(), objectURL)

	err := client.checkMutation(ctx, MutationContext{
		Op:        OpUpdate,
		OpName:    "UpdateSource",
		ObjectURL: objectURL + "/source/main",
		Transport: "TR-EXAMPLE",
	})
	if err == nil {
		t.Fatal("a marked context skipped the transportable-edit check: " +
			"--allow-transportable-edits is no longer required once an outer gate has run")
	}
	if !strings.Contains(err.Error(), "transportable objects is disabled") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMutationMarker_IsPerObject(t *testing.T) {
	// Marking ZDEMO_MARKED must do nothing for ZDEMO_OTHER, which lives in a
	// package the whitelist does not allow. A blanket "the gate ran" flag would
	// let a workflow that checked one object delegate an unchecked mutation of
	// another.
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "informationsystem/search") {
			_, _ = io.WriteString(w, searchXMLFor(
				"/sap/bc/adt/programs/programs/zdemo_other", "ZDEMO_OTHER", "ZDEMO_PROD"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}, WithAllowedPackages("$TMP"))

	ctx := withMutationPackageChecked(context.Background(),
		"/sap/bc/adt/programs/programs/ZDEMO_MARKED")

	err := client.checkMutation(ctx, MutationContext{
		Op:        OpUpdate,
		OpName:    "UpdateSource",
		ObjectURL: "/sap/bc/adt/programs/programs/ZDEMO_OTHER/source/main",
	})
	if err == nil {
		t.Fatal("the mark for ZDEMO_MARKED suppressed the package check for ZDEMO_OTHER, " +
			"which lives in ZDEMO_PROD — the marker must be per-object")
	}

	// It must have actually looked ZDEMO_OTHER up rather than guessing.
	if idx := indexOfCall(rec.snapshot(), func(c wireCall) bool {
		return strings.Contains(c.path, "informationsystem/search")
	}); idx < 0 {
		t.Error("no package lookup was performed for the unmarked object")
	}
}

func TestMutationMarker_SkipsOnlyTheLookupForTheMarkedObject(t *testing.T) {
	// The marked object must be accepted without any HTTP traffic at all —
	// that absence of a request is the whole fix.
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request for a marked object: %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusOK)
	}, WithAllowedPackages("$TMP"))

	const objectURL = "/sap/bc/adt/oo/classes/ZCL_DEMO_MARK"
	ctx := withMutationPackageChecked(context.Background(), objectURL)

	// /source/main and /includes/testclasses both resolve their package from
	// the parent class, so both are covered by the class's mark.
	for _, target := range []string{
		objectURL,
		objectURL + "/source/main",
		objectURL + "/includes/testclasses",
	} {
		if err := client.checkMutation(ctx, MutationContext{
			Op:        OpUpdate,
			OpName:    "UpdateSource",
			ObjectURL: target,
		}); err != nil {
			t.Errorf("marked object %s was rejected: %v", target, err)
		}
	}
}

// --- The function-module metadata PUT, which had no gate of its own ---

func TestSetFunctionModuleProcessingType_HonoursAllowedPackages(t *testing.T) {
	// The metadata PUT inside writeFunctionModule is a mutation that ran
	// behind no package check at all: with source empty it never reaches
	// UpdateSource, so nothing consulted SAP_ALLOWED_PACKAGES before flipping a
	// module to RFC-enabled.
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "informationsystem/search") {
			_, _ = io.WriteString(w, searchXMLFor(
				"/sap/bc/adt/functions/groups/zdemo_grp/fmodules/zdemo_fm",
				"ZDEMO_FM", "ZDEMO_PROD"))
			return
		}
		if r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK" {
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockXML)
			return
		}
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/fmodules/") {
			// GetFunctionModule must succeed, or the call fails for an
			// unrelated reason and the policy assertion proves nothing.
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<fmodule:abapFunctionModule xmlns:fmodule="http://www.sap.com/adt/functions/fmodules"
  xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:name="ZDEMO_FM" adtcore:description="demo" fmodule:processingType="normal">
  <adtcore:containerRef adtcore:name="ZDEMO_GRP" adtcore:type="FUGR/F"/>
</fmodule:abapFunctionModule>`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}, WithAllowedPackages("$TMP"))

	err := client.SetFunctionModuleProcessingType(context.Background(),
		"ZDEMO_GRP", "ZDEMO_FM", "rfc")
	if err == nil {
		t.Fatal("a function module in ZDEMO_PROD was switched to RFC-enabled with " +
			"SAP_ALLOWED_PACKAGES=$TMP — the metadata PUT had no package check")
	}

	calls := rec.snapshot()
	if idx := indexOfCall(calls, isLock); idx >= 0 {
		t.Errorf("a blocked call still locked the object (and would have to release it):")
		dumpCalls(t, calls)
	}
}

// --- The stranded-ENQUEUE half ---

func TestRenameObject_ReleasesOldLockWhenDeleteFails(t *testing.T) {
	const oldURL = "/sap/bc/adt/programs/programs/ZDEMO_OLD"

	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			_, _ = io.WriteString(w, searchXMLFor(
				"/sap/bc/adt/programs/programs/zdemo_old", "ZDEMO_OLD", "$TMP"))
		case strings.Contains(r.URL.Path, "nodestructure"):
			// packageExists preflight for CreateObject.
			_, _ = io.WriteString(w, `<?xml version="1.0"?><asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA/></asx:values></asx:abap>`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/source/main"):
			_, _ = io.WriteString(w, "REPORT zdemo_old.\n")
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockXML)
		case r.Method == http.MethodDelete:
			// The failure this test exists for: the old object cannot be
			// deleted, and until now the lock taken to delete it was simply
			// abandoned — on the very object the user is then told to delete
			// by hand.
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, "object is in use")
		default:
			w.WriteHeader(http.StatusOK)
		}
	}, WithAllowedPackages("$TMP"))

	result, err := client.RenameObject(context.Background(), ObjectTypeProgram,
		"ZDEMO_OLD", "ZDEMO_NEW", "$TMP", "")
	if err != nil {
		t.Fatalf("RenameObject: %v", err)
	}
	if result == nil {
		t.Fatal("RenameObject returned no result")
	}

	calls := rec.snapshot()
	deleteAt := indexOfCall(calls, func(c wireCall) bool { return c.method == http.MethodDelete })
	if deleteAt < 0 {
		t.Fatalf("expected a DELETE of the old object; trace:\n%v", calls)
	}

	released := false
	for _, c := range calls[deleteAt+1:] {
		if isUnlock(c) && strings.EqualFold(c.path, oldURL) {
			released = true
		}
	}
	if !released {
		t.Error("the failed delete left the old object locked: no UNLOCK was sent for it " +
			"after the DELETE failed, so the ENQUEUE outlives the call — on the object the " +
			"caller is being told to delete manually")
		dumpCalls(t, calls)
	}
}

func TestReleaseLockAfterFailure_RunsOnACancelledContext(t *testing.T) {
	// A mutation that fails *because* the context was cancelled or timed out
	// must still release its lock. Reusing the dead context, as every
	// compensating unlock in the package used to, fails inside
	// http.NewRequestWithContext and never sends a byte.
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := client.releaseLockAfterFailure(ctx, "/sap/bc/adt/programs/programs/ZDEMO_CANCEL", "HANDLE-1"); err != nil {
		t.Fatalf("releaseLockAfterFailure on a cancelled context: %v", err)
	}

	calls := rec.snapshot()
	if idx := indexOfCall(calls, isUnlock); idx < 0 {
		t.Errorf("no UNLOCK reached the server after the context was cancelled — "+
			"the lock would be stranded until SAP reaps the session; trace: %v", calls)
	}
}

func TestStrandedLockAdvice_SaysWhatTheUserNeeds(t *testing.T) {
	msg := strandedLockAdvice("/sap/bc/adt/oo/classes/ZCL_DEMO_FOO",
		fmt.Errorf("unlocking object: 423 invalid lock handle"))

	for _, want := range []string{
		"ZCL_DEMO_FOO",    // which object
		"your own user",   // not a colleague
		"SM12",            // the immediate route
		"session timeout", // it expires on its own
		"currently editing",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("advice does not mention %q:\n%s", want, msg)
		}
	}
}

// --- The exported preflight used by the MCP deploy handlers ---

func TestPrepareSourceUpdate_MarksTheObjectItChecked(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "informationsystem/search") {
			_, _ = io.WriteString(w, searchXMLFor(
				"/sap/bc/adt/programs/programs/zdemo_prep", "ZDEMO_PREP", "$TMP"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}, WithAllowedPackages("$TMP"))

	const objectURL = "/sap/bc/adt/programs/programs/ZDEMO_PREP"

	ctx, err := client.PrepareSourceUpdate(context.Background(), objectURL, "")
	if err != nil {
		t.Fatalf("PrepareSourceUpdate: %v", err)
	}
	before := len(rec.snapshot())

	if err := client.checkMutation(ctx, MutationContext{
		Op:        OpUpdate,
		OpName:    "UpdateSource",
		ObjectURL: objectURL + "/source/main",
	}); err != nil {
		t.Fatalf("gate rejected the prepared object: %v", err)
	}
	if after := len(rec.snapshot()); after != before {
		t.Errorf("the write's own gate issued %d more request(s) after PrepareSourceUpdate; "+
			"inside a lock window each one costs the lock handle", after-before)
	}

	// A different object gets no free pass: the prepared context must not
	// suppress the lookup for anything but the object it checked.
	before = len(rec.snapshot())
	_ = client.checkMutation(ctx, MutationContext{
		Op:        OpUpdate,
		OpName:    "UpdateSource",
		ObjectURL: "/sap/bc/adt/programs/programs/ZDEMO_ELSEWHERE/source/main",
	})
	if after := len(rec.snapshot()); after == before {
		t.Error("an unmarked object was accepted without resolving its package")
	}
}

// --- Issue #166: the remaining compensating-unlock sites (ExecuteABAP is
// covered separately, by PR #227) ---

// TestEditSourceWithOptions_ReleasesLockAfterFailedSourceWrite is the
// regression this whole slice of #166 exists for: it cancels the caller's
// ctx at the exact moment the source PUT reaches the server (the same
// technique PR #227 uses for ExecuteABAP), so the compensating unlock has to
// run on a context that is already done. The old `_ = c.UnlockObject(ctx,
// ...)` reused that ctx and so would fail before a byte left the process; a
// version of this test using an uncancelled ctx would pass against either
// the old or the new code and would not actually pin the fix.
func TestEditSourceWithOptions_ReleasesLockAfterFailedSourceWrite(t *testing.T) {
	const objectURL = "/sap/bc/adt/programs/programs/ZDEMO_EDITFAIL"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			_, _ = io.WriteString(w, searchXMLFor(
				"/sap/bc/adt/programs/programs/zdemo_editfail", "ZDEMO_EDITFAIL", "$TMP"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/source/main"):
			_, _ = io.WriteString(w, "REPORT zdemo_editfail.\nWRITE 'old'.\n")
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockXML)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/source/main"):
			cancel()
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}, WithAllowedPackages("$TMP"))

	result, err := client.EditSourceWithOptions(ctx, objectURL,
		"WRITE 'old'.", "WRITE 'new'.", &EditSourceOptions{SyntaxCheck: false})
	if err != nil {
		t.Fatalf("EditSourceWithOptions: %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("test setup did not cancel ctx during the source PUT")
	}
	if result.Success {
		t.Fatal("expected the edit to fail — the source PUT was stubbed to return 500")
	}

	calls := rec.snapshot()
	putAt := indexOfCall(calls, isSourcePut)
	if putAt < 0 {
		t.Fatalf("expected a source PUT attempt; trace:\n%v", calls)
	}

	released := false
	for _, c := range calls[putAt+1:] {
		if isUnlock(c) && strings.EqualFold(c.path, objectURL) {
			released = true
		}
	}
	if !released {
		t.Error("the failed source write left the object locked: no UNLOCK reached the server " +
			"after ctx was cancelled, so the ENQUEUE outlives the call")
		dumpCalls(t, calls)
	}
}

func TestEditSourceWithOptions_StrandedLockAdviceWhenUnlockAlsoFails(t *testing.T) {
	const objectURL = "/sap/bc/adt/programs/programs/ZDEMO_EDITDOUBLEFAIL"

	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			_, _ = io.WriteString(w, searchXMLFor(
				"/sap/bc/adt/programs/programs/zdemo_editdoublefail", "ZDEMO_EDITDOUBLEFAIL", "$TMP"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/source/main"):
			_, _ = io.WriteString(w, "REPORT zdemo_editdoublefail.\nWRITE 'old'.\n")
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockXML)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/source/main"):
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "UNLOCK":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}, WithAllowedPackages("$TMP"))

	result, err := client.EditSourceWithOptions(context.Background(), objectURL,
		"WRITE 'old'.", "WRITE 'new'.", &EditSourceOptions{SyntaxCheck: false})
	if err != nil {
		t.Fatalf("EditSourceWithOptions: %v", err)
	}
	if !strings.Contains(result.Message, "was left LOCKED") {
		t.Fatalf("expected the message to carry stranded-lock advice when the compensating "+
			"UNLOCK also fails, got: %q", result.Message)
	}
}

func TestCreateFromFile_ReleasesLockAfterFailedSourceWrite(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "zdemo_createfail.prog.abap")
	if err := os.WriteFile(file, []byte("REPORT zdemo_createfail.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/checkruns"):
			w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
			_, _ = io.WriteString(w, testEmptyCheckXML)
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockXML)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/source/main"):
			// Cancelled at the moment the write is seen, mirroring PR #227's
			// technique for ExecuteABAP: the compensating unlock then has to
			// run on a context that is already done, which is the failure
			// mode #166 is actually about. A test that never cancels ctx
			// cannot tell the old `_ = c.UnlockObject(ctx, ...)` apart from
			// the fix, since both send an identical request on a live ctx.
			cancel()
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	})

	result, err := client.CreateFromFile(ctx, file, "$TMP", "")
	if err != nil {
		t.Fatalf("CreateFromFile: %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("test setup did not cancel ctx during the source PUT")
	}
	if result.Success {
		t.Fatal("expected the create to fail — the source PUT was stubbed to return 500")
	}

	calls := rec.snapshot()
	putAt := indexOfCall(calls, isSourcePut)
	if putAt < 0 {
		t.Fatalf("expected a source PUT attempt; trace:\n%v", calls)
	}
	if idx := indexOfCall(calls[putAt+1:], isUnlock); idx < 0 {
		t.Error("the failed source write left the newly created object locked: no UNLOCK " +
			"reached the server after ctx was cancelled")
		dumpCalls(t, calls)
	}
}

func TestUpdateFromFileWithOptions_ReleasesLockAfterFailedSourceWrite(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "zdemo_updatefail.prog.abap")
	if err := os.WriteFile(file, []byte("REPORT zdemo_updatefail.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/checkruns"):
			w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
			_, _ = io.WriteString(w, testEmptyCheckXML)
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockXML)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/source/main"):
			// See TestCreateFromFile_ReleasesLockAfterFailedSourceWrite: cancel
			// exactly when the failing write reaches the server, so the
			// compensating unlock has to escape an already-cancelled ctx.
			cancel()
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	})

	result, err := client.UpdateFromFileWithOptions(ctx, file, "", nil)
	if err != nil {
		t.Fatalf("UpdateFromFileWithOptions: %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("test setup did not cancel ctx during the source PUT")
	}
	if result.Success {
		t.Fatal("expected the update to fail — the source PUT was stubbed to return 500")
	}

	calls := rec.snapshot()
	putAt := indexOfCall(calls, isSourcePut)
	if putAt < 0 {
		t.Fatalf("expected a source PUT attempt; trace:\n%v", calls)
	}
	if idx := indexOfCall(calls[putAt+1:], isUnlock); idx < 0 {
		t.Error("the failed source write left the object locked: no UNLOCK reached the server " +
			"after ctx was cancelled")
		dumpCalls(t, calls)
	}
}

func TestWriteSource_BDEF_ReleasesLockAfterFailedSourceWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockXML)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/source/main"):
			// See TestCreateFromFile_ReleasesLockAfterFailedSourceWrite: cancel
			// exactly when the failing write reaches the server, so the
			// compensating unlock has to escape an already-cancelled ctx.
			cancel()
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	})

	result, err := client.WriteSource(ctx, "BDEF", "ZDEMO_BDEF",
		"define behavior for ZDEMO_BDEF { }",
		&WriteSourceOptions{Mode: WriteModeCreate, Package: "$TMP", Description: "demo BDEF"})
	if err != nil {
		t.Fatalf("WriteSource: %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("test setup did not cancel ctx during the source PUT")
	}
	if result.Success {
		t.Fatal("expected the write to fail — the source PUT was stubbed to return 500")
	}
	if !strings.Contains(result.Message, "Failed to update BDEF source") {
		t.Fatalf("unexpected message: %q", result.Message)
	}

	calls := rec.snapshot()
	putAt := indexOfCall(calls, isSourcePut)
	if putAt < 0 {
		t.Fatalf("expected a source PUT attempt; trace:\n%v", calls)
	}
	if idx := indexOfCall(calls[putAt+1:], isUnlock); idx < 0 {
		t.Error("the failed BDEF source write left the object locked: no UNLOCK reached the " +
			"server after ctx was cancelled")
		dumpCalls(t, calls)
	}
}
