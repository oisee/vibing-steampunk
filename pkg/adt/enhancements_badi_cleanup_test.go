package adt

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// badiRoute answers a BADI_IMPL create. post is the POST's status and body;
// the first LOCK (the one for the PUT) answers firstLock, every later LOCK
// succeeds; the PUT fails with SAP's dialog error; everything else is 200.
func badiRoute(postStatus int, postBody string, firstLock int, pkgOfObject string) http.HandlerFunc {
	var locks atomic.Int32
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			_, _ = io.WriteString(w, searchXMLFor(testEnhoxhbURL, "ZENH_DEMO", pkgOfObject))
		case r.Method == http.MethodPost && r.URL.Path == enhoxhbCollection:
			w.WriteHeader(postStatus)
			_, _ = io.WriteString(w, postBody)
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			if locks.Add(1) == 1 && firstLock != http.StatusOK {
				w.WriteHeader(firstLock)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockXML)
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "I::000 Sending of dynpro SAPLSPO1 0500 not possible")
		default:
			w.WriteHeader(http.StatusOK)
		}
	}
}

func noDelete(t *testing.T, rec *adtRecorder) {
	t.Helper()
	if calls := rec.snapshot(); indexOfCall(calls, isDelete) >= 0 {
		dumpCalls(t, calls)
		t.Error("a DELETE was sent")
	}
}

// No request supplied for a transportable package: SAP may have recorded the
// container in a request of its own choosing during the POST. When the LOCK
// then fails, that request is not known, and a DELETE without it would never
// meet --allowed-transports. The container is kept and the error says so.
func TestCreateBadiImplementation_UnknownRequestKeepsTheContainer(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec,
		badiRoute(http.StatusCreated, "", http.StatusInternalServerError, "ZDEMO"),
		WithTransportChoice("off"), WithAllowTransportableEdits(), WithAllowedTransports("TR-ALLOWED*"))

	u, err := client.CreateBadiImplementation(context.Background(), testBadiOptions("ZDEMO", ""))
	var pce *PartialCreateError
	if !errors.As(err, &pce) || pce.CleanupOK {
		t.Fatalf("err = %v, want a PartialCreateError with the container kept", err)
	}
	if u != testEnhoxhbURL {
		t.Errorf("url = %q, want the kept container's", u)
	}
	if !strings.Contains(err.Error(), "kept, not deleted") {
		t.Errorf("the error does not say the container was kept: %v", err)
	}
	if len(pce.ManualSteps) == 0 || !strings.Contains(pce.ManualSteps[0], "ZENH_DEMO") || !strings.Contains(pce.ManualSteps[0], "SE09") {
		t.Errorf("manual steps do not name what to remove: %v", pce.ManualSteps)
	}
	noDelete(t, rec)
}

// A local package has no request to miss: the container is taken away.
func TestCreateBadiImplementation_FailedLockInLocalPackageDeletes(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec,
		badiRoute(http.StatusCreated, "", http.StatusInternalServerError, "$ZDEMO"))

	u, err := client.CreateBadiImplementation(context.Background(), testBadiOptions("$ZDEMO", ""))
	var pce *PartialCreateError
	if !errors.As(err, &pce) || !pce.CleanupOK || u != "" {
		t.Fatalf("url = %q, err = %v, want the container deleted", u, err)
	}
	calls := rec.snapshot()
	if indexOfCall(calls, isDelete) < 0 || unlockAfterDelete(calls) < 0 {
		dumpCalls(t, calls)
		t.Error("want a DELETE followed by the UNLOCK of its lock")
	}
}

// Only a container this call created is deleted: a POST that failed -- an
// "already exists" for an ENHO that was there before, or any other error --
// leaves nothing of ours to take away, and nothing is locked or deleted.
func TestCreateBadiImplementation_FailedPOSTDeletesNothing(t *testing.T) {
	for name, post := range map[string]struct {
		status int
		body   string
	}{
		"already exists": {http.StatusBadRequest, `<?xml version="1.0" encoding="utf-8"?><exc:exception xmlns:exc="http://www.sap.com/abapxml/types/communicationframework">` +
			`<type id="ExceptionResourceAlreadyExists"/><message lang="EN">Enhancement Implementation ZENH_DEMO does already exist</message></exc:exception>`},
		"server error": {http.StatusInternalServerError, "boom"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := &adtRecorder{}
			client := newStubbedClient(t, rec, badiRoute(post.status, post.body, http.StatusOK, "$ZDEMO"))

			u, err := client.CreateBadiImplementation(context.Background(), testBadiOptions("$ZDEMO", ""))
			if err == nil {
				t.Fatal("a failed POST reported success")
			}
			var pce *PartialCreateError
			if errors.As(err, &pce) {
				t.Errorf("a failed POST went to cleanup: %v", err)
			}
			if u != "" {
				t.Errorf("url = %q, want none", u)
			}
			calls := rec.snapshot()
			if i := indexOfCall(calls, isLock); i >= 0 {
				dumpCalls(t, calls)
				t.Errorf("the ENHO was locked after a failed POST (call %d)", i)
			}
			noDelete(t, rec)
		})
	}
}

// The cleanup goes through DeleteObject's package gate: a container that
// turns out to live outside --allowed-packages is not deleted.
func TestCreateBadiImplementation_CleanupKeepsToAllowedPackages(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec,
		badiRoute(http.StatusCreated, "", http.StatusOK, "$ZOTHER"),
		WithAllowedPackages("$ZDEMO"))

	u, err := client.CreateBadiImplementation(context.Background(), testBadiOptions("$ZDEMO", ""))
	var pce *PartialCreateError
	if !errors.As(err, &pce) || pce.CleanupOK {
		t.Fatalf("err = %v, want the cleanup refused", err)
	}
	if u != testEnhoxhbURL {
		t.Errorf("url = %q, want the container left behind", u)
	}
	if !strings.Contains(strings.Join(pce.CleanupActions, "\n"), "mutation gate") {
		t.Errorf("cleanup actions = %v, want the gate's refusal", pce.CleanupActions)
	}
	noDelete(t, rec)
}
