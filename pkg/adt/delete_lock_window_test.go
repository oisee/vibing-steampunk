package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// --- DELETE inside the lock window (issue #238) ---
//
// DeleteObject runs the full mutation gate itself, and with AllowedPackages
// configured that gate resolves the object's package through a stateless
// search. Called under a lock, that search retires the session the handle
// belongs to and the DELETE comes back 423 ExceptionResourceInvalidLockHandle.
// The fix is the one the update paths already use: gate above the LOCK, carry
// the mark into the window.

func isDelete(c wireCall) bool {
	return c.method == http.MethodDelete
}

func isSearch(c wireCall) bool {
	return strings.Contains(c.path, "informationsystem/search")
}

// lastIndexBefore returns the last call before `before` that matches pred.
func lastIndexBefore(calls []wireCall, before int, pred func(wireCall) bool) int {
	for i := before - 1; i >= 0; i-- {
		if pred(calls[i]) {
			return i
		}
	}
	return -1
}

// deleteRoute answers a delete chain for one program whose package is pkg.
func deleteRoute(uri, name, pkg string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			_, _ = io.WriteString(w, searchXMLFor(uri, name, pkg))
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockXML)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}
}

func TestRecoverFailedCreate_NoStatelessRequestBetweenLockAndDelete(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, deleteRoute(
		"/sap/bc/adt/programs/programs/zdemo_del", "ZDEMO_DEL", "$TMP"),
		WithAllowedPackages("$TMP"))

	pce := client.RecoverFailedCreate(context.Background(), CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        "ZDEMO_DEL",
		PackageName: "$TMP",
	})
	if pce == nil || !pce.CleanupOK {
		t.Fatalf("recovery did not clean up: %+v", pce)
	}

	calls := rec.snapshot()
	delAt := indexOfCall(calls, isDelete)
	if delAt < 0 {
		dumpCalls(t, calls)
		t.Fatal("no DELETE reached the server")
	}
	lockAt := lastIndexBefore(calls, delAt, isLock)
	if lockAt < 0 {
		dumpCalls(t, calls)
		t.Fatal("DELETE was not preceded by a LOCK")
	}
	assertWindowStateful(t, calls, lockAt, delAt)

	// The package is still checked — once, and before the lock.
	searchAt := indexOfCall(calls, isSearch)
	if searchAt < 0 || searchAt > lockAt {
		t.Errorf("package lookup at %d, delete LOCK at %d; want the lookup above the lock", searchAt, lockAt)
		dumpCalls(t, calls)
	}
}

func TestRecoverFailedCreate_RefusesForeignPackageBeforeLocking(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, deleteRoute(
		"/sap/bc/adt/programs/programs/zdemo_foreign", "ZDEMO_FOREIGN", "ZSOMEONE_ELSE"),
		WithAllowedPackages("$TMP"))

	pce := client.RecoverFailedCreate(context.Background(), CreateObjectOptions{
		ObjectType: ObjectTypeProgram,
		Name:       "ZDEMO_FOREIGN",
		// A caller-supplied package must not authorise an object that lives elsewhere.
		PackageName: "$TMP",
	})
	if pce == nil || pce.CleanupOK {
		t.Fatalf("recovery of an object outside the allowlist reported success: %+v", pce)
	}

	calls := rec.snapshot()
	if i := indexOfCall(calls, isLock); i >= 0 {
		t.Errorf("an object outside the allowlist was locked (call %d)", i)
		dumpCalls(t, calls)
	}
	if i := indexOfCall(calls, isDelete); i >= 0 {
		t.Errorf("an object outside the allowlist was deleted (call %d)", i)
		dumpCalls(t, calls)
	}
}

func TestPrepareDelete_MarksTheObjectItChecked(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, deleteRoute(
		"/sap/bc/adt/programs/programs/zdemo_prep", "ZDEMO_PREP", "$TMP"),
		WithAllowedPackages("$TMP"))

	const objectURL = "/sap/bc/adt/programs/programs/ZDEMO_PREP"

	ctx, err := client.PrepareDelete(context.Background(), objectURL, "")
	if err != nil {
		t.Fatalf("PrepareDelete: %v", err)
	}
	before := len(rec.snapshot())

	if err := client.DeleteObject(ctx, objectURL, "HANDLE-1", ""); err != nil {
		t.Fatalf("DeleteObject on the prepared object: %v", err)
	}
	calls := rec.snapshot()[before:]
	if len(calls) != 1 || !isDelete(calls[0]) {
		t.Errorf("DeleteObject after PrepareDelete sent %d request(s), want just the DELETE", len(calls))
		dumpCalls(t, calls)
	}

	// The mark covers this object only.
	before = len(rec.snapshot())
	_ = client.DeleteObject(ctx, "/sap/bc/adt/programs/programs/ZDEMO_ELSEWHERE", "HANDLE-1", "")
	if indexOfCall(rec.snapshot()[before:], isSearch) < 0 {
		t.Error("an unmarked object was deleted without resolving its package")
	}
}

func TestPrepareDelete_StillEnforcesOperationPolicy(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, deleteRoute(
		"/sap/bc/adt/programs/programs/zdemo_ro", "ZDEMO_RO", "$TMP"),
		WithAllowedPackages("$TMP"), WithReadOnly())

	if _, err := client.PrepareDelete(context.Background(),
		"/sap/bc/adt/programs/programs/ZDEMO_RO", ""); err == nil {
		t.Fatal("PrepareDelete accepted a delete in read-only mode")
	}
}

// A caller-supplied handle whose lock is older than the keep-alive window
// (lockWindowMaxAge) is still a lock the transport isolates for, up to
// lockRecordMaxAge. DeleteObject's package lookup must then still go out in
// the lock's stateful session, not fall back to stateless after thirty minutes.
func TestDeleteObject_SuppliedHandlePastKeepAliveWindowKeepsLookupStateful(t *testing.T) {
	const objURL = "/sap/bc/adt/programs/programs/zdemo_del"
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, deleteRoute(objURL, "ZDEMO_DEL", "$TMP"),
		WithAllowedPackages("$TMP"))
	ctx := context.Background()

	lock, err := client.LockObject(ctx, objURL, "MODIFY")
	if err != nil {
		t.Fatalf("LockObject: %v", err)
	}
	// Age the lock past the keep-alive window, short of the transport's record.
	client.locks.mu.Lock()
	for h := range client.locks.open {
		client.locks.open[h] = time.Now().Add(-lockWindowMaxAge - time.Minute)
	}
	client.locks.mu.Unlock()
	if client.lockOutstanding() || !client.locks.present() {
		t.Fatal("setup: want the lock outside the keep-alive window but still present")
	}

	if err := client.DeleteObject(ctx, objURL, lock.LockHandle, ""); err != nil {
		t.Fatalf("DeleteObject: %v", err)
	}

	calls := rec.snapshot()
	delAt := indexOfCall(calls, isDelete)
	lockAt := lastIndexBefore(calls, delAt, isLock)
	if delAt < 0 || lockAt < 0 {
		dumpCalls(t, calls)
		t.Fatal("expected a LOCK followed by a DELETE")
	}
	searchAt := -1
	for i := lockAt + 1; i < delAt; i++ {
		if isSearch(calls[i]) {
			searchAt = i
		}
	}
	if searchAt < 0 {
		dumpCalls(t, calls)
		t.Fatal("no package lookup between LOCK and DELETE")
	}
	if calls[searchAt].sessionType != "stateful" {
		t.Errorf("package lookup between LOCK and DELETE is not stateful: %s", calls[searchAt])
		dumpCalls(t, calls)
	}
}
