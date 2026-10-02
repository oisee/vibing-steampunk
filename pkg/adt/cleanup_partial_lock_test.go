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

// --- Behind the proxy, the lock counts as gone only if the context was retired ---

// proxyRoute is deleteRoute behind a session-holding proxy whose retirement
// HEAD on discovery answers headStatus once the DELETE was sent, and whose
// UNLOCKs after it answer unlockStatus with unlockBody.
func proxyRoute(headStatus, unlockStatus int, unlockBody string) http.HandlerFunc {
	var deleted atomic.Bool
	route := deleteRoute(cleanupTestURI, "ZDEMO_DEL", "$ZDEMO")
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			deleted.Store(true)
		case deleted.Load() && r.Method == http.MethodHead && strings.HasSuffix(r.URL.Path, "/core/discovery"):
			w.WriteHeader(headStatus)
		case deleted.Load() && r.URL.Query().Get("_action") == "UNLOCK":
			w.WriteHeader(unlockStatus)
			_, _ = io.WriteString(w, unlockBody)
		default:
			route(w, r)
		}
	}
}

// The retirement HEAD fails, so the session -- and the ENQUEUE in it -- may
// still be live: the cleanup UNLOCKs explicitly.
func TestCleanupPartialObject_FailedProxyRetirementUnlocks(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, proxyRoute(http.StatusBadGateway, http.StatusOK, ""),
		WithAllowedPackages("$ZDEMO"), WithProxyContextIDGuard())

	pce := recoverDemo(client)
	if pce == nil || !pce.CleanupOK {
		t.Fatalf("recovery did not clean up: %+v", pce)
	}
	if calls := rec.snapshot(); unlockAfterDelete(calls) < 0 {
		dumpCalls(t, calls)
		t.Error("the proxy context was not retired, yet no UNLOCK followed the DELETE")
	}
	if len(pce.ManualSteps) != 0 {
		t.Errorf("manual steps after a released lock: %v", pce.ManualSteps)
	}
}

// Retirement unconfirmed and the UNLOCK refused: a stranded lock, reported.
func TestCleanupPartialObject_FailedProxyRetirementAndUnlockIsReported(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, proxyRoute(http.StatusBadGateway, http.StatusForbidden, "no authorization"),
		WithAllowedPackages("$ZDEMO"), WithProxyContextIDGuard())

	pce := recoverDemo(client)
	if pce == nil || !pce.CleanupOK {
		t.Fatalf("the object was deleted, yet CleanupOK is false: %+v", pce)
	}
	if !strings.Contains(strings.Join(pce.ManualSteps, "\n"), "was left LOCKED") {
		t.Errorf("manual steps lack the stranded-lock advice: %v", pce.ManualSteps)
	}
}

// Retirement unconfirmed, and the UNLOCK says the session is gone or the
// handle invalid. That proves nothing: the proxy may have moved to a fresh
// context while the old one, with the ENQUEUE, stays live. The lock is
// reported as possibly held, never as released.
func TestCleanupPartialObject_UnconfirmedRetirementThenSessionGone(t *testing.T) {
	for name, body := range map[string]string{
		"session gone":        "ICMENOSESSION",
		"invalid lock handle": "ExceptionResourceInvalidLockHandle",
	} {
		t.Run(name, func(t *testing.T) {
			rec := &adtRecorder{}
			client := newStubbedClient(t, rec, proxyRoute(http.StatusBadGateway, http.StatusBadRequest, body),
				WithAllowedPackages("$ZDEMO"), WithProxyContextIDGuard())

			pce := recoverDemo(client)
			if pce == nil || !pce.CleanupOK {
				t.Fatalf("the object was deleted, yet CleanupOK is false: %+v", pce)
			}
			steps := strings.Join(pce.ManualSteps, "\n")
			if !strings.Contains(steps, "may still be locked") || !strings.Contains(steps, "SM12") {
				t.Errorf("manual steps lack the may-still-be-locked advice: %v", pce.ManualSteps)
			}
		})
	}
}

func TestDeleteObjectGated_UnconfirmedRetirementThenInvalidHandle(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, proxyRoute(http.StatusBadGateway, http.StatusBadRequest, "ExceptionResourceInvalidLockHandle"),
		WithAllowedPackages("$ZDEMO"), WithProxyContextIDGuard())

	note, err := client.DeleteObjectGated(context.Background(), cleanupTestURI, "")
	if err != nil {
		t.Fatalf("DeleteObjectGated: %v", err)
	}
	if !strings.Contains(note, "may still be locked") {
		t.Errorf("note = %q, want the may-still-be-locked advice", note)
	}
}

// isRetireHead is the stateless HEAD on discovery that retires a proxy context.
func isRetireHead(c wireCall) bool {
	return c.method == http.MethodHead && strings.HasSuffix(c.path, "/core/discovery")
}

// While another lock chain holds a handle, no retirement HEAD is sent: the
// proxy would inject the context in use, and the HEAD would end it.
func TestRetireProxyContext_NotSentWhileAnotherLockIsHeld(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}, WithProxyContextIDGuard())
	client.noteLockOpened("OTHER-HANDLE")

	if err := client.transport.retireProxyContext(context.Background()); err == nil {
		t.Error("retirement reported as confirmed while another lock is held")
	}
	if calls := rec.snapshot(); indexOfCall(calls, isRetireHead) >= 0 {
		dumpCalls(t, calls)
		t.Error("a retirement HEAD was sent while another lock chain held the context")
	}
}

// The same, end to end: the delete chain sends no HEAD and releases its own
// lock with an UNLOCK on its own handle.
func TestDeleteObjectGated_AnotherLockHeldUnlocksInsteadOfRetiring(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, proxyRoute(http.StatusOK, http.StatusOK, ""),
		WithAllowedPackages("$ZDEMO"), WithProxyContextIDGuard())
	client.noteLockOpened("OTHER-HANDLE")

	note, err := client.DeleteObjectGated(context.Background(), cleanupTestURI, "")
	if err != nil || note != "" {
		t.Fatalf("DeleteObjectGated: note=%q err=%v", note, err)
	}
	calls := rec.snapshot()
	delAt := indexOfCall(calls, isDelete)
	if delAt < 0 {
		dumpCalls(t, calls)
		t.Fatal("no DELETE was sent")
	}
	for i := delAt + 1; i < len(calls); i++ {
		if isRetireHead(calls[i]) {
			dumpCalls(t, calls)
			t.Fatalf("retirement HEAD at %d while another lock chain held the context", i)
		}
	}
	u := unlockAfterDelete(calls)
	if u < 0 || calls[u].query.Get("lockHandle") != "HANDLE-1" {
		dumpCalls(t, calls)
		t.Error("want an UNLOCK with the delete chain's own handle after the DELETE")
	}
}

// deleteGated follows the same rule.
func TestDeleteObjectGated_FailedProxyRetirementUnlocks(t *testing.T) {
	for name, tc := range map[string]struct {
		unlockStatus int
		wantNote     bool
	}{
		"unlock works":   {http.StatusOK, false},
		"unlock refused": {http.StatusForbidden, true},
	} {
		t.Run(name, func(t *testing.T) {
			rec := &adtRecorder{}
			client := newStubbedClient(t, rec, proxyRoute(http.StatusBadGateway, tc.unlockStatus, "no authorization"),
				WithAllowedPackages("$ZDEMO"), WithProxyContextIDGuard())

			note, err := client.DeleteObjectGated(context.Background(), cleanupTestURI, "")
			if err != nil {
				t.Fatalf("DeleteObjectGated: %v", err)
			}
			if calls := rec.snapshot(); unlockAfterDelete(calls) < 0 {
				dumpCalls(t, calls)
				t.Error("the proxy context was not retired, yet no UNLOCK followed the DELETE")
			}
			if (note != "") != tc.wantNote {
				t.Errorf("note = %q, want a note: %v", note, tc.wantNote)
			}
		})
	}
}
