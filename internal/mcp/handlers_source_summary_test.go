package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// The fake program, as ADT hands it out (CRLF), and its digest, pinned by
// hand with printf ... | sha256sum and wc -c. It references a class, so a
// read with dependency context would ask SAP for ZCL_DEP too.
const (
	summaryProgSource = "REPORT zdemo_sum.\r\nDATA lo TYPE REF TO zcl_dep.\r\n"
	summaryProgSHA256 = "06c11cc632a57684f533c8c1aedbc5a78d9e496c19e5a496117ff2124fe67ab7"
	summaryProgPath   = "/sap/bc/adt/programs/programs/ZDEMO_SUM/source/main"
)

// summarySAP serves the fake program and records every request it sees.
func summarySAP(t *testing.T) (*Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("X-CSRF-Token", "t")
		if r.Method == http.MethodGet && r.URL.Path == summaryProgPath {
			_, _ = w.Write([]byte(summaryProgSource))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(ts.Close)
	s := NewServer(&Config{BaseURL: ts.URL, Username: "u", Password: "p", Client: "001", Language: "EN", Mode: "hyperfocused"})
	return s, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), seen...) }
}

func readProg(t *testing.T, s *Server, params map[string]any) (string, bool) {
	t.Helper()
	res, err := s.handleUniversalTool(context.Background(), newRequest(map[string]any{
		"action": "read", "target": "PROG ZDEMO_SUM", "params": params,
	}))
	if err != nil {
		t.Fatal(err)
	}
	return resultText(res), res.IsError
}

// One GET of the source and nothing else: the summary is the normal read
// minus the body.
func onlyTheSourceRead(t *testing.T, seen []string) {
	t.Helper()
	if len(seen) != 1 || seen[0] != "GET "+summaryProgPath {
		t.Fatalf("SAP saw %v, want exactly one GET %s", seen, summaryProgPath)
	}
}

func TestReadSummaryHasExactDigestAndNoBody(t *testing.T) {
	s, seen := summarySAP(t)
	text, isErr := readProg(t, s, map[string]any{"summary": true})
	if isErr {
		t.Fatalf("summary failed: %s", text)
	}
	if strings.Contains(text, "zcl_dep") {
		t.Fatalf("summary returned the source:\n%s", text)
	}
	var got struct {
		ObjectType, Name, URI, SHA256, SourceHash string
		Lines, Bytes                              int
		Unchanged                                 *bool
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("summary is not JSON: %v\n%s", err, text)
	}
	if got.ObjectType != "PROG" || got.Name != "ZDEMO_SUM" || got.URI != summaryProgPath {
		t.Fatalf("summary identity: %+v", got)
	}
	if got.Lines != 2 || got.Bytes != 49 || got.SHA256 != summaryProgSHA256 {
		t.Fatalf("summary lines/bytes/sha256 = %d/%d/%s, want 2/49/%s", got.Lines, got.Bytes, got.SHA256, summaryProgSHA256)
	}
	if !strings.HasPrefix(got.SourceHash, "sha256:") || got.Unchanged != nil {
		t.Fatalf("sourceHash/unchanged: %+v", got)
	}
	// The dependency context is not fetched for a summary.
	onlyTheSourceRead(t, seen())
}

func TestReadIfNoneMatch(t *testing.T) {
	t.Run("match: no body", func(t *testing.T) {
		s, seen := summarySAP(t)
		text, isErr := readProg(t, s, map[string]any{"if_none_match": strings.ToUpper(summaryProgSHA256)})
		if isErr || strings.Contains(text, "REPORT") {
			t.Fatalf("a matching digest must not return the body: %q", text)
		}
		if !strings.HasPrefix(text, "unchanged (sha256 "+summaryProgSHA256+")") {
			t.Fatalf("got %q", text)
		}
		onlyTheSourceRead(t, seen())
	})
	t.Run("mismatch: the full body", func(t *testing.T) {
		s, _ := summarySAP(t)
		other := strings.Repeat("0", 64)
		text, isErr := readProg(t, s, map[string]any{"if_none_match": other, "include_context": false})
		if isErr || text != summaryProgSource {
			t.Fatalf("a different digest must return the source as a normal read does, got %q", text)
		}
	})
	t.Run("with summary: unchanged is reported", func(t *testing.T) {
		s, _ := summarySAP(t)
		text, _ := readProg(t, s, map[string]any{"summary": true, "if_none_match": summaryProgSHA256})
		if !strings.Contains(text, `"unchanged": true`) || strings.Contains(text, "REPORT") {
			t.Fatalf("got %s", text)
		}
	})
	t.Run("a sourceHash is refused before any read", func(t *testing.T) {
		s, seen := summarySAP(t)
		text, isErr := readProg(t, s, map[string]any{"if_none_match": "sha256:" + summaryProgSHA256})
		if !isErr || !strings.Contains(text, "sourceHash") {
			t.Fatalf("got %q (error=%v)", text, isErr)
		}
		if n := len(seen()); n != 0 {
			t.Fatalf("a malformed digest cost %d round trips", n)
		}
	})
}

// A full read with include_hash carries the sha256 if_none_match takes, so
// the first read need not be followed by a summary to get it.
func TestReadIncludeHashCarriesSHA256(t *testing.T) {
	s, _ := summarySAP(t)
	text, isErr := readProg(t, s, map[string]any{"include_hash": true, "include_context": false})
	var got map[string]string
	if isErr || json.Unmarshal([]byte(text), &got) != nil {
		t.Fatalf("include_hash read: %q", text)
	}
	if got["source"] != summaryProgSource || got["sha256"] != summaryProgSHA256 {
		t.Fatalf("include_hash read: source/sha256 = %q/%q", got["source"], got["sha256"])
	}
}

// The control for onlyTheSourceRead: a normal read of the same program asks
// SAP about ZCL_DEP for its dependency context, so one GET is not a given.
func TestReadSummaryControlNormalReadFetchesContext(t *testing.T) {
	s, seen := summarySAP(t)
	if text, isErr := readProg(t, s, nil); isErr || !strings.HasPrefix(text, summaryProgSource) {
		t.Fatalf("normal read: %q", text)
	}
	if n := len(seen()); n < 2 {
		t.Fatalf("a normal read made %d requests; the fake source must cost a dependency lookup", n)
	}
}
