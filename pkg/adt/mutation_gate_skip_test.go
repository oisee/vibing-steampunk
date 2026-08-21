package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestMutationGateSkip_EditSourceNoSearchBetweenLockAndPut is the
// regression guard for the sister bug of commit 8cb45a5 (SyntaxCheck
// before Lock).
//
// The original symptom: when AllowedPackages was configured, EditSource
// produced an HTTP 423 ExceptionResourceInvalidLockHandle on the PUT
// because the inner UpdateSource gate ran getObjectPackage →
// SearchObject (a STATELESS hop) between the stateful Lock and the
// stateful PUT. SAP's ICM retired the stateful session on the stateless
// hop (Sap-Err-Id: ICMENOSESSION), and the lock handle bound to that
// dead session was then rejected by the PUT.
//
// The fix marks the context with mutationGateSkipKey after the outer
// EditSourceWithOptions gate completes; the inner UpdateSource sees
// the flag and skips its own getObjectPackage call. This test pins
// the absence of any informationsystem/search request between LOCK
// and PUT in the call sequence.
func TestMutationGateSkip_EditSourceNoSearchBetweenLockAndPut(t *testing.T) {
	const sourceBody = "REPORT ztest.\nWRITE / 'hello'.\n"
	const newSourceBody = "REPORT ztest.\nWRITE / 'world'.\n"

	mock := &methodPathMock{
		routes: []routedResponse{
			resp("", "discovery", 200, "ok"),
			// Outer gate's package resolution — happens BEFORE Lock.
			resp("", "informationsystem/search", 200, searchZTESTInTmpXML),
			// GET source/main → return current source.
			resp(http.MethodGet, "/programs/programs/ZTEST/source/main", 200, sourceBody),
			// SyntaxCheck (POST checkruns) → return clean.
			resp(http.MethodPost, "checkruns", 200, ""),
			// Lock acquisition (POST with _action=LOCK).
			resp(http.MethodPost, "/programs/programs/ZTEST", 200, lockResponseXML),
			// PUT source/main → succeed.
			resp(http.MethodPut, "/programs/programs/ZTEST/source/main", 200, ""),
			// Activate.
			resp(http.MethodPost, "/activation", 200, ""),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass",
		WithAllowedPackages("$TMP"),
	)
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.EditSourceWithOptions(
		context.Background(),
		"/sap/bc/adt/programs/programs/ZTEST",
		"WRITE / 'hello'.",
		"WRITE / 'world'.",
		&EditSourceOptions{SyntaxCheck: false},
	)
	if err != nil {
		t.Fatalf("EditSourceWithOptions failed: %v", err)
	}
	_ = result
	_ = newSourceBody

	// Walk the recorded calls, find the Lock POST and the source PUT,
	// and assert no informationsystem/search appears between them.
	lockIdx, putIdx := -1, -1
	for i, c := range mock.calls {
		if c.method == http.MethodPost && strings.HasSuffix(c.path, "/programs/programs/ZTEST") {
			// LockObject POSTs to the bare object URL with _action=LOCK
			// in the query string. The mock records the path, not the
			// query, so this is the right match.
			lockIdx = i
		}
		if c.method == http.MethodPut && strings.HasSuffix(c.path, "/programs/programs/ZTEST/source/main") {
			putIdx = i
			break
		}
	}
	if lockIdx == -1 {
		t.Fatalf("no Lock POST observed; calls: %+v", mock.calls)
	}
	if putIdx == -1 {
		t.Fatalf("no source PUT observed; calls: %+v", mock.calls)
	}
	if lockIdx >= putIdx {
		t.Fatalf("lock should precede put; lockIdx=%d putIdx=%d calls=%+v",
			lockIdx, putIdx, mock.calls)
	}

	for i := lockIdx + 1; i < putIdx; i++ {
		if strings.Contains(mock.calls[i].path, "informationsystem/search") {
			t.Errorf(
				"informationsystem/search hop appeared between Lock and PUT (index %d): %s — "+
					"this is the session-affinity regression: the stateless search "+
					"retires SAP's stateful session and invalidates the lock handle "+
					"(sister bug of commit 8cb45a5)",
				i, mock.calls[i].path,
			)
		}
	}
}

// TestMutationGateSkip_FlagSkipsInnerCheck unit-tests the
// withMutationGateAlreadyRan / mutationGateAlreadyRan plumbing in
// isolation: even with a deliberately invalid MutationContext that
// would normally fail closed under AllowedPackages (no ObjectURL
// AND no Package), checkMutation must short-circuit and return nil
// when the context has been marked.
func TestMutationGateSkip_FlagSkipsInnerCheck(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass",
		WithAllowedPackages("$TMP"),
	)
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	// Without the flag this MutationContext fails closed (line 107 of
	// mutation_gate.go: "requires either ObjectURL or Package when
	// AllowedPackages is configured").
	unmarkedErr := client.checkMutation(context.Background(), MutationContext{
		Op:     OpUpdate,
		OpName: "TestOp",
	})
	if unmarkedErr == nil {
		t.Fatal("baseline: expected failure when neither ObjectURL nor Package is set under AllowedPackages")
	}

	// With the flag set, the inner gate must short-circuit.
	markedCtx := withMutationGateAlreadyRan(context.Background())
	if !mutationGateAlreadyRan(markedCtx) {
		t.Fatal("mutationGateAlreadyRan should report true for a marked context")
	}
	markedErr := client.checkMutation(markedCtx, MutationContext{
		Op:     OpUpdate,
		OpName: "TestOp",
	})
	if markedErr != nil {
		t.Fatalf("marked context should bypass the inner gate, got error: %v", markedErr)
	}
}

// TestMutationGateSkip_FlagDoesNotLeakAcrossContexts verifies that the
// skip flag is scoped to a single context derivation chain — a sibling
// context derived from the same parent must NOT inherit the flag, so
// unrelated callers in the same goroutine cannot accidentally bypass
// the gate.
func TestMutationGateSkip_FlagDoesNotLeakAcrossContexts(t *testing.T) {
	parent := context.Background()
	marked := withMutationGateAlreadyRan(parent)

	if mutationGateAlreadyRan(parent) {
		t.Error("parent context should not be retroactively marked")
	}
	if !mutationGateAlreadyRan(marked) {
		t.Error("derived context should be marked")
	}

	// A sibling derived from the parent (not the marked context) must
	// be clean.
	sibling := context.WithValue(parent, struct{ k string }{k: "x"}, 1)
	if mutationGateAlreadyRan(sibling) {
		t.Error("sibling context derived from parent must not be marked")
	}
}
