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
