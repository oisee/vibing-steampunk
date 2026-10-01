package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestEditSourceWithOptions_UppercaseClassIncludeURL_DetectedCorrectly pins
// issue #118: EditSourceWithOptions classifies a class include by matching
// lowercase literals ("/oo/classes/", "/includes/") against objectURL. An
// uppercase caller (MCP tool arguments can arrive this way — e.g. copied
// verbatim from a search/create result) used to misclassify the include as a
// plain class source and append /source/main to a URL that must not have it,
// breaking the source read before the edit itself is ever attempted.
func TestEditSourceWithOptions_UppercaseClassIncludeURL_DetectedCorrectly(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		// Any body that will not contain oldString is enough — the function
		// hits its "old_string not found" early return right after the GET,
		// so only that GET's path matters for this test.
		_, _ = w.Write([]byte("some source that will not match"))
	})

	_, err := client.EditSourceWithOptions(context.Background(),
		"/SAP/BC/ADT/OO/CLASSES/ZCL_FOO/INCLUDES/TESTCLASSES",
		"this string is not present", "replacement",
		&EditSourceOptions{SyntaxCheck: false})
	if err != nil {
		t.Fatalf("EditSourceWithOptions: %v", err)
	}

	calls := rec.snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected exactly one request (the source GET); got %d: %v", len(calls), calls)
	}

	got := calls[0].path
	if strings.HasSuffix(got, "/source/main") {
		t.Errorf("class include GET must not have /source/main appended, got %q — "+
			"an uppercase objectURL made isClassInclude misdetect it as a plain class (#118)", got)
	}
	want := "/SAP/BC/ADT/OO/CLASSES/ZCL_FOO/INCLUDES/TESTCLASSES"
	if got != want {
		t.Errorf("GET path = %q, want %q (only the classification is case-folded, not the URL)", got, want)
	}
}

// TestEditSourceWithOptions_NamespacedClassInclude_KeepsNameCase: the URL is
// classified on a lowercased copy, but the object_url itself is used as
// given. vsp's own builders keep a namespaced class name uppercase
// (/sap/bc/adt/oo/classes/%2FDMO%2FCL_FLIGHT), and that name must reach the
// source read and the lock of the parent class unchanged.
func TestEditSourceWithOptions_NamespacedClassInclude_KeepsNameCase(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if r.Method == http.MethodGet || r.URL.Query().Get("_action") == "LOCK" {
			paths = append(paths, r.Method+" "+r.URL.EscapedPath())
		}
		mu.Unlock()
		w.Header().Set("X-CSRF-Token", "TOKEN")
		switch {
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte("CLASS ltc DEFINITION FOR TESTING.\nENDCLASS."))
		case r.URL.Query().Get("_action") == "LOCK":
			// Refuse the lock: the edit stops there, with the lock URL on record.
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := NewConfig(srv.URL, "TESTUSER", "secret")
	client := NewClientWithTransport(cfg, NewTransport(cfg))

	const objectURL = "/sap/bc/adt/oo/classes/%2FDMO%2FCL_FLIGHT/includes/testclasses"
	_, _ = client.EditSourceWithOptions(context.Background(), objectURL,
		"CLASS ltc", "CLASS ltc_new", &EditSourceOptions{SyntaxCheck: false})

	mu.Lock()
	defer mu.Unlock()
	if len(paths) < 2 {
		t.Fatalf("expected the source GET and the lock; got %v", paths)
	}
	if want := "GET " + objectURL; paths[0] != want {
		t.Errorf("source read = %q, want %q (include detected, name case kept)", paths[0], want)
	}
	if want := "POST /sap/bc/adt/oo/classes/%2FDMO%2FCL_FLIGHT"; paths[1] != want {
		t.Errorf("lock = %q, want %q (the parent class, name case kept)", paths[1], want)
	}
}
