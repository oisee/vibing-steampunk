package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// --- cleanupPartialObject releases the lock it takes for the DELETE ---
//
// A DELETE consumes the handle but not the ENQUEUE behind it: off a
// session-holding proxy the lock stays in SM12 until it is unlocked.
// deleteGated sends that UNLOCK; deleteReleasesLock is the rule both follow.

// unlockAfterDelete is the index of the first UNLOCK after the DELETE, or -1.
func unlockAfterDelete(calls []wireCall) int {
	delAt := indexOfCall(calls, isDelete)
	if delAt < 0 {
		return -1
	}
	for i := delAt + 1; i < len(calls); i++ {
		if isUnlock(calls[i]) {
			return i
		}
	}
	return -1
}

const cleanupTestURI = "/sap/bc/adt/programs/programs/zdemo_del"

func recoverDemo(c *Client) *PartialCreateError {
	return c.RecoverFailedCreate(context.Background(), CreateObjectOptions{
		ObjectType: ObjectTypeProgram, Name: "ZDEMO_DEL", PackageName: "$ZDEMO",
	})
}

func TestCleanupPartialObject_ReleasesTheDeleteLock(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, deleteRoute(cleanupTestURI, "ZDEMO_DEL", "$ZDEMO"),
		WithAllowedPackages("$ZDEMO"))

	pce := recoverDemo(client)
	if pce == nil || !pce.CleanupOK {
		t.Fatalf("recovery did not clean up: %+v", pce)
	}
	calls := rec.snapshot()
	if indexOfCall(calls, isDelete) < 0 {
		dumpCalls(t, calls)
		t.Fatal("no DELETE reached the server")
	}
	if unlockAfterDelete(calls) < 0 {
		dumpCalls(t, calls)
		t.Error("the lock taken for the DELETE was never released: no UNLOCK after the DELETE")
	}
	if len(pce.ManualSteps) != 0 {
		t.Errorf("manual steps after a clean cleanup: %v", pce.ManualSteps)
	}
}

// Behind a session-holding proxy the DELETE retires the context and the lock
// with it; an UNLOCK there would fail and report a lock that is not there.
func TestCleanupPartialObject_NoUnlockBehindSessionProxy(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, deleteRoute(cleanupTestURI, "ZDEMO_DEL", "$ZDEMO"),
		WithAllowedPackages("$ZDEMO"), WithProxyContextIDGuard())

	pce := recoverDemo(client)
	if pce == nil || !pce.CleanupOK {
		t.Fatalf("recovery did not clean up: %+v", pce)
	}
	if calls := rec.snapshot(); unlockAfterDelete(calls) >= 0 {
		dumpCalls(t, calls)
		t.Error("UNLOCK sent after a DELETE that already retired the proxy context")
	}
}

// An UNLOCK after the DELETE that fails is reported with the stranded-lock
// advice; the object is gone all the same.
func TestCleanupPartialObject_ReportsAStrandedDeleteLock(t *testing.T) {
	rec := &adtRecorder{}
	var deleted atomic.Bool
	route := deleteRoute(cleanupTestURI, "ZDEMO_DEL", "$ZDEMO")
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			deleted.Store(true)
		case deleted.Load() && r.URL.Query().Get("_action") == "UNLOCK":
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "no authorization")
		default:
			route(w, r)
		}
	}, WithAllowedPackages("$ZDEMO"))

	pce := recoverDemo(client)
	if pce == nil || !pce.CleanupOK {
		t.Fatalf("the object was deleted, yet CleanupOK is false: %+v", pce)
	}
	if !strings.Contains(strings.Join(pce.ManualSteps, "\n"), "was left LOCKED") {
		t.Errorf("manual steps lack the stranded-lock advice: %v", pce.ManualSteps)
	}
}

// A DELETE that fails releases the lock, and a release that fails too is
// reported rather than dropped.
func TestCleanupPartialObject_FailedDeleteReportsAStrandedLock(t *testing.T) {
	rec := &adtRecorder{}
	var deleteTried atomic.Bool
	route := deleteRoute(cleanupTestURI, "ZDEMO_DEL", "$ZDEMO")
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			deleteTried.Store(true)
			w.WriteHeader(http.StatusForbidden)
		case deleteTried.Load() && r.URL.Query().Get("_action") == "UNLOCK":
			w.WriteHeader(http.StatusForbidden)
		default:
			route(w, r)
		}
	}, WithAllowedPackages("$ZDEMO"))

	pce := recoverDemo(client)
	if pce == nil || pce.CleanupOK {
		t.Fatalf("a refused DELETE reported a clean cleanup: %+v", pce)
	}
	if !strings.Contains(strings.Join(pce.ManualSteps, "\n"), "was left LOCKED") {
		t.Errorf("manual steps lack the stranded-lock advice: %v", pce.ManualSteps)
	}
}
