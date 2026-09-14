package adt

import (
	"context"
	"net/http"
	"strings"
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
	want := "/sap/bc/adt/oo/classes/zcl_foo/includes/testclasses"
	if got != want {
		t.Errorf("GET path = %q, want %q (objectURL must be lowercased before use)", got, want)
	}
}
