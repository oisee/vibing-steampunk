package adt

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestWriteSourceUpdateResolvesActualPackageBeforeLock guards #143 at the
// public WriteSource boundary. The caller omits package; the update must use
// the object's metadata and must not repeat that lookup after LOCK (#169).
func TestWriteSourceUpdateResolvesActualPackageBeforeLock(t *testing.T) {
	const objectURL = "/sap/bc/adt/programs/programs/ZDEMO_PACKAGE"
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			_, _ = io.WriteString(w, searchXMLFor(objectURL, "ZDEMO_PACKAGE", "$TMP"))
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

	result, err := client.WriteSource(context.Background(), "PROG", "ZDEMO_PACKAGE", "REPORT zdemo_package.", &WriteSourceOptions{
		Mode: WriteModeUpdate,
	})
	if err != nil {
		t.Fatalf("WriteSource: %v", err)
	}
	if result == nil || strings.Contains(result.Message, "requires either ObjectURL or Package") {
		t.Fatalf("existing-object update was rejected before package resolution: %#v", result)
	}

	calls := rec.snapshot()
	searchAt := indexOfCall(calls, func(c wireCall) bool { return strings.Contains(c.path, "informationsystem/search") })
	lockAt := indexOfCall(calls, isLock)
	putAt := indexOfCall(calls, isSourcePut)
	if searchAt < 0 || lockAt < 0 || putAt < 0 {
		t.Fatalf("expected package search, LOCK and source PUT; trace:\n%v", calls)
	}
	if searchAt > lockAt {
		t.Errorf("package lookup ran after LOCK (search %d, lock %d)", searchAt, lockAt)
		dumpCalls(t, calls)
	}
	assertWindowStateful(t, calls, lockAt, putAt)
	searches := 0
	for _, call := range calls {
		if strings.Contains(call.path, "informationsystem/search") {
			searches++
		}
	}
	if searches != 1 {
		t.Errorf("package resolved %d times, want 1", searches)
		dumpCalls(t, calls)
	}
}

func TestWriteSourceUpdateDoesNotTrustCallerPackage(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "informationsystem/search") {
			_, _ = io.WriteString(w, searchXMLFor(
				"/sap/bc/adt/programs/programs/ZDEMO_DENIED", "ZDEMO_DENIED", "ZDENIED"))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}, WithAllowedPackages("$TMP"))

	_, err := client.WriteSource(context.Background(), "PROG", "ZDEMO_DENIED", "REPORT zdemo_denied.", &WriteSourceOptions{
		Mode:    WriteModeUpdate,
		Package: "$TMP", // create-only input; cannot grant update access.
	})
	if err == nil || !strings.Contains(err.Error(), "blocked by safety configuration") {
		t.Fatalf("update using an actual forbidden package error = %v, want package refusal", err)
	}
	for _, call := range rec.snapshot() {
		if isLock(call) || isSourcePut(call) {
			t.Errorf("forbidden update reached write path: %s", call)
		}
	}
}

func TestWriteSourceCreateKeepsExplicitPackagePolicy(t *testing.T) {
	client := NewClient("https://sap.example.invalid", "TESTUSER", "secret", WithAllowedPackages("$TMP"))

	_, err := client.WriteSource(context.Background(), "PROG", "ZDEMO_NEW", "REPORT zdemo_new.", &WriteSourceOptions{
		Mode:        WriteModeCreate,
		Description: "demo",
	})
	if err == nil || !strings.Contains(err.Error(), "requires either ObjectURL or Package") {
		t.Fatalf("create without package error = %v, want package-required refusal", err)
	}

	_, err = client.WriteSource(context.Background(), "PROG", "ZDEMO_NEW", "REPORT zdemo_new.", &WriteSourceOptions{
		Mode:        WriteModeCreate,
		Description: "demo",
		Package:     "ZDENIED",
	})
	if err == nil || !strings.Contains(err.Error(), "blocked by safety configuration") {
		t.Fatalf("create in forbidden package error = %v, want package refusal", err)
	}
}

func TestWriteSourceUpsertDoesNotCreateAfterUnansweredProbe(t *testing.T) {
	for _, tc := range []struct {
		name   string
		handle func(http.ResponseWriter, *http.Request)
	}{
		{
			name:   "forbidden",
			handle: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) },
		},
		{
			name:   "timeout",
			handle: func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() },
		},
		{
			name:   "server error",
			handle: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			created := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					created = true
				}
				tc.handle(w, r)
			}))
			defer srv.Close()
			client := NewClient(srv.URL, "TESTUSER", "secret")
			ctx := context.Background()
			if tc.name == "timeout" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 25*time.Millisecond)
				defer cancel()
			}
			result, err := client.WriteSource(ctx, "PROG", "ZDEMO_PROBE", "REPORT zdemo_probe.", &WriteSourceOptions{
				Mode:        WriteModeUpsert,
				Description: "demo",
				Package:     "$TMP",
			})
			if err != nil {
				t.Fatalf("WriteSource: %v", err)
			}
			if !strings.Contains(result.Message, "cannot tell whether") {
				t.Fatalf("upsert result = %#v, want refusal to guess", result)
			}
			if created {
				t.Fatal("unanswered existence probe attempted CreateObject")
			}
		})
	}
}

func TestWriteSourceTopLevelSafetyRejectsWithoutSAPRequests(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}, WithReadOnly())

	_, err := client.WriteSource(context.Background(), "PROG", "ZDEMO_RO", "REPORT zdemo_ro.", &WriteSourceOptions{Mode: WriteModeUpdate})
	if err == nil {
		t.Fatal("read-only WriteSource unexpectedly proceeded")
	}
	if calls := rec.snapshot(); len(calls) != 0 {
		t.Errorf("top-level safety refusal reached SAP:\n%v", calls)
	}
}
