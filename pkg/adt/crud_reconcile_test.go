package adt

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// methodPathMock is a richer test transport than the path-only
// mockTransportClient: it routes by (method, pathSubstring) so a single
// test can stage CreateObject (POST), existence probe (GET), lock
// (POST), and delete (DELETE) without ambiguity.
//
// Each entry in routes is checked against the request in order; the
// first whose method matches and whose pathSubstring is contained in
// the request path wins. A nil response means "use HTTP 500 default".
type methodPathMock struct {
	routes []routedResponse
	calls  []recordedCall
}

type routedResponse struct {
	method        string
	pathSubstring string
	status        int
	body          string
	err           error
}

type recordedCall struct {
	method string
	path   string
}

func (m *methodPathMock) Do(req *http.Request) (*http.Response, error) {
	m.calls = append(m.calls, recordedCall{method: req.Method, path: req.URL.Path})
	for _, r := range m.routes {
		if r.method != "" && r.method != req.Method {
			continue
		}
		if r.pathSubstring == "" || strings.Contains(req.URL.Path, r.pathSubstring) {
			if r.err != nil {
				return nil, r.err
			}
			h := http.Header{}
			h.Set("X-CSRF-Token", "test-token")
			return &http.Response{
				StatusCode: r.status,
				Body:       io.NopCloser(strings.NewReader(r.body)),
				Header:     h,
			}, nil
		}
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("not routed: " + req.Method + " " + req.URL.Path)),
		Header:     http.Header{},
	}, nil
}

// resp is a small helper for constructing routedResponse entries with
// fewer keystrokes — the routedResponse{} literal is verbose otherwise.
func resp(method, pathFragment string, status int, body string) routedResponse {
	return routedResponse{method: method, pathSubstring: pathFragment, status: status, body: body}
}

// lockResponseXML is the minimal lock-acquired payload the ADT lock
// flow expects to parse out of a successful POST to lock=… .
const lockResponseXML = `<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <LOCK_HANDLE>TESTHANDLE</LOCK_HANDLE>
      <CORRNR>D15K000001</CORRNR>
      <CORRUSER>TESTUSER</CORRUSER>
      <CORRTEXT>Test transport</CORRTEXT>
      <IS_LOCAL>X</IS_LOCAL>
      <IS_LINK_UP>X</IS_LINK_UP>
    </DATA>
  </asx:values>
</asx:abap>`

// packageNodeStructureXML is a minimal valid nodestructure response
// used by the mock so the CreateObject preflight `packageExists` check
// does not abort the test before our reconciliation logic runs.
const packageNodeStructureXML = `<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <TREE_CONTENT/>
      <CATEGORIES/>
      <OBJECT_TYPES/>
    </DATA>
  </asx:values>
</asx:abap>`

// searchZTESTInTmpXML mimics the ADT search endpoint response that
// resolves an object URL to its containing package. DeleteObject runs
// this lookup as a safety check; without it the package-allowed-list
// gate aborts the cleanup path before it can finish.
const searchZTESTInTmpXML = `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/ztest" adtcore:type="PROG/P" adtcore:name="ZTEST" adtcore:packageName="$TMP"/>
</adtcore:objectReferences>`

func newReconcileClient(t *testing.T, mock *methodPathMock) *Client {
	t.Helper()
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass",
		WithAllowedPackages("$TMP"),
		WithEnableTransports(),
	)
	transport := NewTransportWithClient(cfg, mock)
	return NewClientWithTransport(cfg, transport)
}

// TestCreateObject_CleanFailureBeforePersistence covers the simple case:
// CreateObject POST returns 500, and the follow-up existence probe
// returns 404. SAP did not commit anything, so reconcileFailedCreate
// must return the original error unchanged — no PartialCreateError.
func TestCreateObject_CleanFailureBeforePersistence(t *testing.T) {
	mock := &methodPathMock{
		routes: []routedResponse{
			resp("", "discovery", 200, "ok"),
			resp(http.MethodPost, "nodestructure", 200, packageNodeStructureXML),
			// Existence probe after failure → 404 (object never persisted).
			// MUST come before the broad POST programs route so the GET
			// path matches first.
			resp(http.MethodGet, "/programs/programs/ZTEST", 404, "not found"),
			// CreateObject POST → 500.
			resp(http.MethodPost, "/programs/programs", 500, "ICM EXCEPTION"),
		},
	}
	client := newReconcileClient(t, mock)

	err := client.CreateObject(context.Background(), CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        "ZTEST",
		PackageName: "$TMP",
	})
	if err == nil {
		t.Fatal("expected create to fail")
	}

	var pce *PartialCreateError
	if errors.As(err, &pce) {
		t.Errorf("expected plain error, got PartialCreateError: %v", pce)
	}
	if !strings.Contains(err.Error(), "creating object") {
		t.Errorf("expected wrapped 'creating object' error, got: %v", err)
		for _, c := range mock.calls {
			t.Logf("  mock saw %s %s", c.method, c.path)
		}
	}
}

// TestCreateObject_PartialPersistenceCleanupOK covers the prod-incident
// scenario: CreateObject POST returns 500 but SAP already persisted the
// object. The existence probe returns 200, lock acquisition succeeds,
// delete succeeds → CleanupOK is true and the original error is wrapped.
func TestCreateObject_PartialPersistenceCleanupOK(t *testing.T) {
	mock := &methodPathMock{
		routes: []routedResponse{
			resp("", "discovery", 200, "ok"),
			resp(http.MethodPost, "nodestructure", 200, packageNodeStructureXML),
			// DeleteObject's package-allowed-list guard hits the search
			// endpoint to resolve the object URL → package name.
			resp("", "informationsystem/search", 200, searchZTESTInTmpXML),
			// Specific (with object name) before broad (creation path).
			// Lock and Create both POST to the programs endpoint, but
			// lock targets `/programs/programs/ZTEST` while create
			// targets `/programs/programs`. Path-substring-matching on
			// the longer specific URL first lets us route correctly.
			resp(http.MethodGet, "/programs/programs/ZTEST", 200, "<p/>"),
			resp(http.MethodPost, "/programs/programs/ZTEST", 200, lockResponseXML),
			resp(http.MethodDelete, "/programs/programs/ZTEST", 200, ""),
			resp(http.MethodPost, "/programs/programs", 500, "ICM EXCEPTION"),
		},
	}
	client := newReconcileClient(t, mock)

	err := client.CreateObject(context.Background(), CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        "ZTEST",
		PackageName: "$TMP",
	})
	if err == nil {
		t.Fatal("expected create to fail")
	}

	var pce *PartialCreateError
	if !errors.As(err, &pce) {
		t.Fatalf("expected PartialCreateError, got %T: %v", err, err)
	}
	if !pce.CleanupOK {
		t.Errorf("CleanupOK = false, want true; cleanup actions: %v", pce.CleanupActions)
	}
	if pce.OriginalErr == nil {
		t.Error("OriginalErr should preserve the original create error")
	}
	if pce.ObjectURL == "" {
		t.Error("ObjectURL should be populated")
	}
	if len(pce.ManualSteps) != 0 {
		t.Errorf("ManualSteps should be empty when cleanup succeeded, got %v", pce.ManualSteps)
	}
}

// TestCreateObject_PartialPersistenceLockFails covers the harder case:
// SAP persisted the object but our lock acquisition for cleanup fails.
// The reconciler must return PartialCreateError with CleanupOK=false
// and a populated ManualSteps list so the operator knows what to do
// by hand.
func TestCreateObject_PartialPersistenceLockFails(t *testing.T) {
	mock := &methodPathMock{
		routes: []routedResponse{
			resp("", "discovery", 200, "ok"),
			resp(http.MethodPost, "nodestructure", 200, packageNodeStructureXML),
			// Specific path (with name) before broad. Lock POST →
			// 403 to simulate "locked by another user".
			resp(http.MethodGet, "/programs/programs/ZTEST", 200, "<p/>"),
			resp(http.MethodPost, "/programs/programs/ZTEST", 403, "locked by another user"),
			resp(http.MethodPost, "/programs/programs", 500, "ICM EXCEPTION"),
		},
	}
	client := newReconcileClient(t, mock)

	err := client.CreateObject(context.Background(), CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        "ZTEST",
		PackageName: "$TMP",
	})
	if err == nil {
		t.Fatal("expected create to fail")
	}

	var pce *PartialCreateError
	if !errors.As(err, &pce) {
		t.Fatalf("expected PartialCreateError, got %T: %v", err, err)
	}
	if pce.CleanupOK {
		t.Error("CleanupOK = true, want false (lock acquisition failed)")
	}
	if len(pce.ManualSteps) == 0 {
		t.Error("ManualSteps should be populated when cleanup could not finish")
	}
	foundSM12 := false
	for _, step := range pce.ManualSteps {
		if strings.Contains(step, "SM12") {
			foundSM12 = true
			break
		}
	}
	if !foundSM12 {
		t.Errorf("expected SM12 hint in manual steps, got %v", pce.ManualSteps)
	}
}

// TestDeleteObject_UsesStatefulSession pins the reviewer-reported
// regression: the reconcile path acquires a session-bound lock via
// LockObject (which uses Stateful:true) and then calls DeleteObject,
// so DeleteObject MUST also request a stateful session. Without it
// SAP treats the DELETE as an unrelated request and rejects the
// session-scoped lock handle as invalid.
//
// We cannot directly observe RequestOptions.Stateful in the caller
// after the fact, so we assert it via the transport layer behaviour:
// in stateful mode the ADT transport adds the X-sap-adt-sessiontype:
// stateful header to the outbound request. The mock captures every
// request's headers; a DELETE without that header is a regression.
func TestDeleteObject_UsesStatefulSession(t *testing.T) {
	mock := &methodPathMock{
		routes: []routedResponse{
			resp("", "discovery", 200, "ok"),
			// Search resolves object → package for DeleteObject's guard.
			resp("", "informationsystem/search", 200, searchZTESTInTmpXML),
			resp(http.MethodDelete, "/programs/programs/ZTEST", 200, ""),
		},
	}
	client := newReconcileClient(t, mock)

	// We still need to capture the DELETE request, so we use a
	// tracking wrapper that reads each recorded call's headers.
	tracker := &headerCaptureMock{inner: mock}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass",
		WithAllowedPackages("$TMP"),
	)
	transport := NewTransportWithClient(cfg, tracker)
	client = NewClientWithTransport(cfg, transport)

	err := client.DeleteObject(
		context.Background(),
		"/sap/bc/adt/programs/programs/ZTEST",
		"TESTHANDLE",
		"",
	)
	if err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}

	// Find the DELETE request among captured calls and assert its
	// stateful header is set.
	var deleteReq *capturedReq
	for i := range tracker.captured {
		if tracker.captured[i].method == http.MethodDelete {
			deleteReq = &tracker.captured[i]
			break
		}
	}
	if deleteReq == nil {
		t.Fatal("no DELETE request was sent")
	}
	if got := deleteReq.sessionType; got != "stateful" {
		t.Errorf("DELETE X-sap-adt-sessiontype = %q, want \"stateful\" — session-bound lock handles require stateful mode (issue #88)", got)
	}
}

// headerCaptureMock wraps another transport-level mock and records
// every outbound request's relevant headers. Used exclusively by the
// stateful-session regression test — the main methodPathMock already
// records method+path but throws headers away.
type headerCaptureMock struct {
	inner     *methodPathMock
	captured  []capturedReq
}

type capturedReq struct {
	method      string
	path        string
	sessionType string
}

func (h *headerCaptureMock) Do(req *http.Request) (*http.Response, error) {
	h.captured = append(h.captured, capturedReq{
		method:      req.Method,
		path:        req.URL.Path,
		sessionType: req.Header.Get("X-sap-adt-sessiontype"),
	})
	return h.inner.Do(req)
}

// TestRecoverFailedCreate_ObjectDoesNotExist covers the idempotent
// no-op case: the operator invokes recovery on an object that is
// already gone (either never persisted, or cleaned up already). The
// call must return CleanupOK=true with an explanatory note, NEVER
// error out. This is what makes the action safe to retry.
func TestRecoverFailedCreate_ObjectDoesNotExist(t *testing.T) {
	mock := &methodPathMock{
		routes: []routedResponse{
			resp("", "discovery", 200, "ok"),
			resp(http.MethodGet, "/programs/programs/ZTEST", 404, "not found"),
		},
	}
	client := newReconcileClient(t, mock)

	pce := client.RecoverFailedCreate(context.Background(), CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        "ZTEST",
		PackageName: "$TMP",
	})
	if pce == nil {
		t.Fatal("expected PartialCreateError shell, got nil")
	}
	if !pce.CleanupOK {
		t.Errorf("CleanupOK = false, want true for idempotent no-op; actions: %v", pce.CleanupActions)
	}
	if len(pce.CleanupActions) != 1 || !strings.Contains(pce.CleanupActions[0], "nothing to recover") {
		t.Errorf("expected 'nothing to recover' action, got %v", pce.CleanupActions)
	}
}

// TestRecoverFailedCreate_CleansExistingZombie covers the main happy
// path of the operator-driven recovery: the object exists, lock is
// acquired, delete succeeds → CleanupOK=true, no manual steps.
func TestRecoverFailedCreate_CleansExistingZombie(t *testing.T) {
	mock := &methodPathMock{
		routes: []routedResponse{
			resp("", "discovery", 200, "ok"),
			resp("", "informationsystem/search", 200, searchZTESTInTmpXML),
			resp(http.MethodGet, "/programs/programs/ZTEST", 200, "<p/>"),
			resp(http.MethodPost, "/programs/programs/ZTEST", 200, lockResponseXML),
			resp(http.MethodDelete, "/programs/programs/ZTEST", 200, ""),
		},
	}
	client := newReconcileClient(t, mock)

	pce := client.RecoverFailedCreate(context.Background(), CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        "ZTEST",
		PackageName: "$TMP",
	})
	if pce == nil {
		t.Fatal("expected PartialCreateError, got nil")
	}
	if !pce.CleanupOK {
		t.Errorf("CleanupOK = false, want true; actions: %v", pce.CleanupActions)
	}
	foundDelete := false
	for _, action := range pce.CleanupActions {
		if strings.Contains(action, "deleted partially-created object") {
			foundDelete = true
			break
		}
	}
	if !foundDelete {
		t.Errorf("expected 'deleted partially-created object' in actions, got %v", pce.CleanupActions)
	}
	if len(pce.ManualSteps) != 0 {
		t.Errorf("ManualSteps should be empty when cleanup succeeded, got %v", pce.ManualSteps)
	}
}

// TestRecoverFailedCreate_LockHeldByAnother covers the realistic
// "another user is editing the zombie" case: probe says it exists,
// lock acquisition fails → return ManualSteps instead of CleanupOK.
func TestRecoverFailedCreate_LockHeldByAnother(t *testing.T) {
	mock := &methodPathMock{
		routes: []routedResponse{
			resp("", "discovery", 200, "ok"),
			resp(http.MethodGet, "/programs/programs/ZTEST", 200, "<p/>"),
			resp(http.MethodPost, "/programs/programs/ZTEST", 403, "locked by another user"),
		},
	}
	client := newReconcileClient(t, mock)

	pce := client.RecoverFailedCreate(context.Background(), CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        "ZTEST",
		PackageName: "$TMP",
	})
	if pce == nil {
		t.Fatal("expected PartialCreateError, got nil")
	}
	if pce.CleanupOK {
		t.Error("CleanupOK = true, want false — lock acquisition failed")
	}
	if len(pce.ManualSteps) == 0 {
		t.Error("ManualSteps should be populated when cleanup could not finish")
	}
}

// TestPartialCreateError_UnwrapsToOriginal verifies that errors.Is
// against the original wrapped error keeps working — important so
// existing callers' error-classification logic does not break.
func TestPartialCreateError_UnwrapsToOriginal(t *testing.T) {
	original := errors.New("transport-level failure")
	pce := &PartialCreateError{
		ObjectURL:   "/sap/bc/adt/programs/programs/ZTEST",
		OriginalErr: original,
	}
	if !errors.Is(pce, original) {
		t.Error("errors.Is failed to find the original error")
	}
	if pce.Unwrap() != original {
		t.Error("Unwrap returned wrong value")
	}
}

// TestUpdateClassInclude_UsesStatefulSession pins the lock-handle
// regression class reported in issues #88, #92, and #98: write
// operations that consume a session-bound lock handle MUST request
// a stateful HTTP session so the sap-contextid cookie travels with
// the request. Without it SAP routes the PUT through a different
// session and rejects the lock handle as InvalidLockHandle (HTTP
// 423) several seconds after the lock was successfully acquired.
//
// Same observability technique as TestDeleteObject_UsesStatefulSession:
// inspect the X-sap-adt-sessiontype header on the captured PUT.
func TestUpdateClassInclude_UsesStatefulSession(t *testing.T) {
	mock := &methodPathMock{
		routes: []routedResponse{
			resp("", "discovery", 200, "ok"),
			resp(http.MethodPut, "/oo/classes/ZTEST/includes/testclasses", 200, ""),
		},
	}
	tracker := &headerCaptureMock{inner: mock}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, tracker)
	client := NewClientWithTransport(cfg, transport)

	err := client.UpdateClassInclude(
		context.Background(),
		"ZTEST",
		ClassIncludeTestClasses,
		"REPORT ztest_test.",
		"TESTHANDLE",
		"",
	)
	if err != nil {
		t.Fatalf("UpdateClassInclude failed: %v", err)
	}

	var putReq *capturedReq
	for i := range tracker.captured {
		if tracker.captured[i].method == http.MethodPut {
			putReq = &tracker.captured[i]
			break
		}
	}
	if putReq == nil {
		t.Fatal("no PUT request was sent")
	}
	if got := putReq.sessionType; got != "stateful" {
		t.Errorf("PUT X-sap-adt-sessiontype = %q, want \"stateful\" — session-bound lock handles require stateful mode (issues #88/#92/#98)", got)
	}
}

// TestCreateTestInclude_UsesStatefulSession is the sibling guard for
// the create path: the editor flow first calls CreateTestInclude
// (POST) under the parent class lock, then UpdateClassInclude (PUT)
// under the same lock. Both must ride the same session.
func TestCreateTestInclude_UsesStatefulSession(t *testing.T) {
	mock := &methodPathMock{
		routes: []routedResponse{
			resp("", "discovery", 200, "ok"),
			resp(http.MethodPost, "/oo/classes/ZTEST/includes", 200, ""),
		},
	}
	tracker := &headerCaptureMock{inner: mock}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, tracker)
	client := NewClientWithTransport(cfg, transport)

	err := client.CreateTestInclude(
		context.Background(),
		"ZTEST",
		"TESTHANDLE",
		"",
	)
	if err != nil {
		t.Fatalf("CreateTestInclude failed: %v", err)
	}

	var postReq *capturedReq
	for i := range tracker.captured {
		// The CSRF prefetch is also a non-GET (HEAD) — match POST only.
		if tracker.captured[i].method == http.MethodPost &&
			strings.Contains(tracker.captured[i].path, "/includes") {
			postReq = &tracker.captured[i]
			break
		}
	}
	if postReq == nil {
		t.Fatal("no POST /includes request was sent")
	}
	if got := postReq.sessionType; got != "stateful" {
		t.Errorf("POST X-sap-adt-sessiontype = %q, want \"stateful\" — session-bound lock handles require stateful mode (issues #88/#92/#98)", got)
	}
}

// TestLockObject_PassesThroughModificationSupport pins the correct
// semantics of the MODIFICATION_SUPPORT field on the ADT lock response.
//
// The SAP standard interface IF_ADT_LOCK_RESULT defines four values:
//
//   - "ModifcationAssistant"    (CO_MOD_SUPPORT_MODASS)       — SAP/partner
//     object, Modification Assistant on.
//   - "ModificationsLoggedOnly" (CO_MOD_SUPPORT_LOGGED_ONLY)  — SAP/partner
//     object, no Modification Assistant (changes only logged).
//   - "NoModification"          (CO_MOD_SUPPORT_NOT_NEEDED)   — customer
//     namespace object; modification tracking is NOT NEEDED. This is the
//     usual response when a developer edits their own Z*/Y* code.
//   - ""                        (CO_MOD_SUPPORT_NOT_SPECIFIED) — not set.
//
// The string "NoModification" therefore means "no tracking needed", not
// "no edit allowed". An earlier commit (22517d4) hard-failed on this value
// as if it meant read-only, which broke every normal customer-code edit
// on destinations that populate the field (e.g. SAP Business Application
// Studio via Cloud Connector + Principal Propagation). This test locks
// the corrected behaviour: LockObject returns the parsed result verbatim
// for any value of MODIFICATION_SUPPORT, and callers treat it as
// advisory metadata only.
func TestLockObject_PassesThroughModificationSupport(t *testing.T) {
	cases := []struct {
		name    string
		support string
	}{
		{"customer-namespace-not-needed", "NoModification"},
		{"sap-modification-assistant", "ModifcationAssistant"},
		{"sap-logged-only", "ModificationsLoggedOnly"},
		{"not-specified", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lockXML := `<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <LOCK_HANDLE>HANDLE-X</LOCK_HANDLE>
      <CORRNR></CORRNR>
      <CORRUSER></CORRUSER>
      <CORRTEXT></CORRTEXT>
      <IS_LOCAL></IS_LOCAL>
      <IS_LINK_UP></IS_LINK_UP>
      <MODIFICATION_SUPPORT>` + tc.support + `</MODIFICATION_SUPPORT>
    </DATA>
  </asx:values>
</asx:abap>`
			mock := &methodPathMock{
				routes: []routedResponse{
					resp("", "discovery", 200, "ok"),
					resp(http.MethodPost, "/oo/classes/ZOBJ", 200, lockXML),
				},
			}
			cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
			transport := NewTransportWithClient(cfg, mock)
			client := NewClientWithTransport(cfg, transport)

			result, err := client.LockObject(
				context.Background(),
				"/sap/bc/adt/oo/classes/ZOBJ",
				"MODIFY",
			)
			if err != nil {
				t.Fatalf("LockObject(MODIFY) must not fail on MODIFICATION_SUPPORT=%q, got error: %v", tc.support, err)
			}
			if result == nil {
				t.Fatal("LockObject returned nil result")
			}
			if result.LockHandle != "HANDLE-X" {
				t.Errorf("LockHandle = %q, want HANDLE-X", result.LockHandle)
			}
			if result.ModificationSupport != tc.support {
				t.Errorf("ModificationSupport = %q, want %q (must be passed through verbatim)", result.ModificationSupport, tc.support)
			}
		})
	}
}

// TestLockObject_AllowsNoModificationOnReadLock exercises the READ access
// mode path, which skips the OpLock safety check. Kept as a companion to
// TestLockObject_PassesThroughModificationSupport so the READ branch
// keeps explicit coverage for the same metadata-only behaviour.
func TestLockObject_AllowsNoModificationOnReadLock(t *testing.T) {
	const noModLockXML = `<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <LOCK_HANDLE>HANDLE-X</LOCK_HANDLE>
      <CORRNR></CORRNR>
      <CORRUSER></CORRUSER>
      <CORRTEXT></CORRTEXT>
      <IS_LOCAL></IS_LOCAL>
      <IS_LINK_UP></IS_LINK_UP>
      <MODIFICATION_SUPPORT>NoModification</MODIFICATION_SUPPORT>
    </DATA>
  </asx:values>
</asx:abap>`
	mock := &methodPathMock{
		routes: []routedResponse{
			resp("", "discovery", 200, "ok"),
			resp(http.MethodPost, "/oo/classes/ZREADONLY", 200, noModLockXML),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.LockObject(
		context.Background(),
		"/sap/bc/adt/oo/classes/ZREADONLY",
		"READ",
	)
	if err != nil {
		t.Fatalf("LockObject(READ) should succeed even on read-only objects: %v", err)
	}
	if result.LockHandle != "HANDLE-X" {
		t.Errorf("LockHandle = %q, want HANDLE-X", result.LockHandle)
	}
}

