package mcp

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TestHandleEditSource_PassesObjectURLUnchanged: EditSourceWithOptions copes
// with an uppercase object_url itself (#118), so the handler hands the URL on
// as given. A namespaced class keeps its name uppercase in vsp's own URLs, and
// lowercasing it here would send /sap/bc/adt/oo/classes/%2fdmo%2fcl_flight.
func TestHandleEditSource_PassesObjectURLUnchanged(t *testing.T) {
	var mu sync.Mutex
	var reads []string
	sap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "TOKEN")
		if r.Method == http.MethodGet {
			mu.Lock()
			reads = append(reads, r.URL.EscapedPath())
			mu.Unlock()
			// The old string is not in here, so the edit stops after the read.
			_, _ = w.Write([]byte("CLASS ltc DEFINITION FOR TESTING.\nENDCLASS."))
		}
	}))
	defer sap.Close()

	const objectURL = "/sap/bc/adt/oo/classes/%2FDMO%2FCL_FLIGHT/includes/testclasses"
	s := NewServer(&Config{BaseURL: sap.URL, Username: "TESTUSER", Client: "001", Mode: "expert"})
	if _, err := s.handleEditSource(t.Context(), newRequest(map[string]any{
		"object_url":   objectURL,
		"old_string":   "not in the source",
		"new_string":   "x",
		"syntax_check": false,
	})); err != nil {
		t.Fatalf("handleEditSource: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(reads) == 0 || reads[0] != objectURL {
		t.Errorf("source read = %v, want %q first", reads, objectURL)
	}
}
