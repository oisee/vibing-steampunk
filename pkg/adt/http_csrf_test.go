package adt

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// newCSRFTestTransport points a Transport at a test server.
func newCSRFTestTransport(t *testing.T, srv *httptest.Server) *Transport {
	t.Helper()
	cfg := NewConfig(srv.URL, "user", "pass")
	tr := NewTransport(cfg)
	return tr
}

// TestFetchCSRFTokenHeadFastPath: when HEAD answers with a token, that is the only
// request made — the fast path must not cost a second round trip.
func TestFetchCSRFTokenHeadFastPath(t *testing.T) {
	var heads, gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			heads.Add(1)
			w.Header().Set("X-CSRF-Token", "TOKEN-FROM-HEAD")
			w.WriteHeader(http.StatusOK)
		default:
			gets.Add(1)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	if err := tr.fetchCSRFToken(context.Background()); err != nil {
		t.Fatalf("fetchCSRFToken: %v", err)
	}
	if got := tr.getCSRFToken(); got != "TOKEN-FROM-HEAD" {
		t.Fatalf("token = %q", got)
	}
	if heads.Load() != 1 || gets.Load() != 0 {
		t.Fatalf("expected one HEAD and no GET, got %d/%d", heads.Load(), gets.Load())
	}
}

// TestFetchCSRFTokenGetFallback reproduces BASIS 740 / ECC EhP7: HEAD is answered
// with 400 and no token, and only GET yields one. Without the fallback vsp cannot
// talk to those systems at all.
func TestFetchCSRFTokenGetFallback(t *testing.T) {
	var heads, gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			heads.Add(1)
			w.WriteHeader(http.StatusBadRequest)
		default:
			gets.Add(1)
			w.Header().Set("X-CSRF-Token", "TOKEN-FROM-GET")
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	if err := tr.fetchCSRFToken(context.Background()); err != nil {
		t.Fatalf("fetchCSRFToken: %v", err)
	}
	if got := tr.getCSRFToken(); got != "TOKEN-FROM-GET" {
		t.Fatalf("token = %q", got)
	}
	if heads.Load() != 1 || gets.Load() != 1 {
		t.Fatalf("expected one HEAD then one GET, got %d/%d", heads.Load(), gets.Load())
	}
}

// A 401 must be reported as an authentication failure without a pointless retry.
func TestFetchCSRFTokenUnauthorizedDoesNotRetry(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	err := tr.fetchCSRFToken(context.Background())
	if err == nil {
		t.Fatal("expected an error for 401")
	}
	if requests.Load() != 1 {
		t.Fatalf("401 must not be retried, made %d requests", requests.Load())
	}
}

// The "Required" placeholder is not a token; it must trigger the fallback.
func TestFetchCSRFTokenRequiredPlaceholderFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("X-CSRF-Token", "Required")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("X-CSRF-Token", "REAL-TOKEN")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	if err := tr.fetchCSRFToken(context.Background()); err != nil {
		t.Fatalf("fetchCSRFToken: %v", err)
	}
	if got := tr.getCSRFToken(); got != "REAL-TOKEN" {
		t.Fatalf("token = %q", got)
	}
}

// A 403 on HEAD is not a verdict on the user's authorizations: some systems
// refuse the HEAD and answer the GET perfectly well. Short-circuiting there
// reintroduced exactly the unusability the GET fallback exists to prevent.
func TestFetchCSRFTokenForbiddenHeadStillTriesGet(t *testing.T) {
	var heads, gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			heads.Add(1)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		gets.Add(1)
		w.Header().Set("X-CSRF-Token", "TOKEN-FROM-GET")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	if err := tr.fetchCSRFToken(context.Background()); err != nil {
		t.Fatalf("a 403 on HEAD must not stop the GET fallback: %v", err)
	}
	if got := tr.getCSRFToken(); got != "TOKEN-FROM-GET" {
		t.Fatalf("token = %q", got)
	}
	if heads.Load() != 1 || gets.Load() != 1 {
		t.Fatalf("expected one HEAD then one GET, got %d/%d", heads.Load(), gets.Load())
	}
}

// A 403 from both methods is a real authorization failure and must say so.
func TestFetchCSRFTokenForbiddenEverywhereFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	err := newCSRFTestTransport(t, srv).fetchCSRFToken(context.Background())
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected a 403 authorization error, got %v", err)
	}
}

// A write refused with 403 is retried after a token refresh, because a stale
// token is the usual cause. When that refresh is refused too, the caller must
// still get SAP's own answer to the write, as an *APIError: a missing
// authorization would otherwise read as a token problem, and code that sorts
// a definite refusal from a lost response (lock release) would sort it wrong.
func TestForbiddenWriteKeepsSAPAnswerWhenRefreshFails(t *testing.T) {
	const refusal = "You are not authorized to change object ZDEMO_PROG (S_DEVELOP)"
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(refusal))
			return
		}
		w.WriteHeader(http.StatusForbidden) // the token probe is refused as well
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	tr.setCSRFToken("token-stale")
	_, err := tr.Request(context.Background(), "/sap/bc/adt/demo", &RequestOptions{Method: http.MethodPost})
	if err == nil {
		t.Fatal("request succeeded on a 403")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v does not carry an *APIError", err)
	}
	if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != refusal {
		t.Errorf("APIError = %d %q, want 403 %q", apiErr.StatusCode, apiErr.Message, refusal)
	}
	if !strings.Contains(err.Error(), "refreshing CSRF token") {
		t.Errorf("error %v lost the refresh failure", err)
	}
	if got := posts.Load(); got != 1 {
		t.Errorf("server saw %d writes, want 1: there is no token to retry with", got)
	}
}

// The case that made the lost *APIError matter: an UNLOCK refused with 403,
// with the token refresh refused too, is a definite answer from SAP. The
// advice must say the object was left locked, not that the release is in doubt.
func TestRefusedUnlockReadsAsLeftLockedWhenRefreshFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte("no authorization"))
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "user", "pass")
	client.transport.setCSRFToken("token-stale")
	advice := client.holdLock("/sap/bc/adt/programs/programs/zdemo_prog", "HANDLE-1").release(context.Background())
	if !strings.Contains(advice, "was left LOCKED") {
		t.Errorf("advice = %q, want the left-LOCKED advice for a definite refusal", advice)
	}
}
