package adt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// The fake source of these tests, as ADT hands it out: CRLF, final newline.
const summaryFakeSource = "REPORT zdemo_sum.\r\nWRITE 'hello'.\r\n"

// Pinned by hand (printf ... | sha256sum, wc -c), not by the code under test.
const (
	summaryFakeSHA256     = "91531b9b6cd35eb4a8fc3943cf70b692809e538626fbb96500fabe33223e41cb"
	summaryFakeSourceHash = "sha256:5195c49ed03ab4174bfffc2b273477d4cfa52ad3538e5426c25c3b0c350e9787"
	summaryFakeBytes      = 35
	summaryFakeLines      = 2
)

func TestSummarizeSourceExactLinesBytesSHA256(t *testing.T) {
	s := SummarizeSource("prog", "zdemo_sum", nil, "", summaryFakeSource)
	if s.ObjectType != "PROG" || s.Name != "ZDEMO_SUM" {
		t.Fatalf("type/name = %s %s", s.ObjectType, s.Name)
	}
	if s.URI != "/sap/bc/adt/programs/programs/ZDEMO_SUM/source/main" {
		t.Fatalf("uri = %q", s.URI)
	}
	if s.Lines != summaryFakeLines || s.Bytes != summaryFakeBytes {
		t.Fatalf("lines/bytes = %d/%d, want %d/%d", s.Lines, s.Bytes, summaryFakeLines, summaryFakeBytes)
	}
	// No normalisation: the CRLF bytes are what is hashed.
	if s.SHA256 != summaryFakeSHA256 {
		t.Fatalf("sha256 = %s, want %s (exact bytes, not normalised)", s.SHA256, summaryFakeSHA256)
	}
	if s.SourceHash != summaryFakeSourceHash {
		t.Fatalf("sourceHash = %s, want %s", s.SourceHash, summaryFakeSourceHash)
	}
	if s.Unchanged != nil {
		t.Fatal("unchanged is only set when a digest was compared")
	}
}

func TestSourceLineCount(t *testing.T) {
	for src, want := range map[string]int{
		"":           0,
		"a":          1,
		"a\n":        1,
		"a\nb":       2,
		"a\r\nb\r\n": 2,
		"\n\n":       2,
	} {
		if got := SourceLineCount(src); got != want {
			t.Errorf("SourceLineCount(%q) = %d, want %d", src, got, want)
		}
	}
}

func TestParseIfNoneMatch(t *testing.T) {
	got, err := ParseIfNoneMatch(` "` + strings.ToUpper(summaryFakeSHA256) + `" `)
	if err != nil || got != summaryFakeSHA256 {
		t.Fatalf("ParseIfNoneMatch(upper, quoted) = %q, %v", got, err)
	}
	for _, bad := range []string{summaryFakeSourceHash, "abc", summaryFakeSHA256 + "0", "zz" + summaryFakeSHA256[2:]} {
		if _, err := ParseIfNoneMatch(bad); err == nil {
			t.Errorf("ParseIfNoneMatch(%q) accepted", bad)
		}
	}
}

// git_delete_objects' expect sha256 is not a source digest: ZADT_VSP computes
// it over the object's abapGit serialisation as the SHA-256 of the sorted
// "<file>=<sha256 of the file>" lines joined by LF (see git_versions.go). So
// the read digest never equals it; for a file whose bytes are the source
// text, the read digest is exactly the hash on that file's manifest line.
// This test follows that documented recipe for a PROG (source + XML).
func TestSourceSHA256VersusGitExpectSHA256(t *testing.T) {
	hexOf := func(b string) string { s := sha256.Sum256([]byte(b)); return hex.EncodeToString(s[:]) }
	files := map[string]string{
		"zdemo_sum.prog.abap": summaryFakeSource,
		"zdemo_sum.prog.xml":  "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<abapGit/>\n",
	}
	var lines []string
	for name, body := range files {
		lines = append(lines, name+"="+hexOf(body))
	}
	sort.Strings(lines)
	expect := hexOf(strings.Join(lines, "\n"))

	got := SummarizeSource("PROG", "ZDEMO_SUM", nil, "", summaryFakeSource).SHA256
	if got == expect {
		t.Fatal("a source digest cannot be an abapGit manifest digest; the docs would be wrong")
	}
	if want := "zdemo_sum.prog.abap=" + got; lines[0] != want {
		t.Fatalf("manifest line = %q, want %q: the read sha256 must be the per-file hash of the same bytes", lines[0], want)
	}
}

func TestSourceReadURI(t *testing.T) {
	cases := []struct {
		typ, name string
		opts      *GetSourceOptions
		want      string
	}{
		{"CLAS", "ZCL_X", &GetSourceOptions{Include: "testclasses"}, "/sap/bc/adt/oo/classes/ZCL_X/includes/testclasses"},
		{"CLAS", "ZCL_X", &GetSourceOptions{Method: "RUN"}, "/sap/bc/adt/oo/classes/ZCL_X/source/main"},
		{"FUNC", "Z_FM", &GetSourceOptions{Parent: "zgrp"}, "/sap/bc/adt/functions/groups/ZGRP/fmodules/Z_FM/source/main"},
		{"FUNC", "Z_FM", nil, ""},
		{"MSAG", "ZM", nil, ""},
	}
	for _, c := range cases {
		if got := SourceReadURI(c.typ, c.name, c.opts); got != c.want {
			t.Errorf("SourceReadURI(%s %s) = %q, want %q", c.typ, c.name, got, c.want)
		}
	}
}

// A PROG that ADT knows only as an include is served from /programs/includes
// (GetProgram's fallback). The summary names the URI that served the text,
// and the one asked for as requested.
func TestSummaryReportsTheURIThatServedTheText(t *testing.T) {
	const incPath = "/sap/bc/adt/programs/includes/ZDEMO_INC/source/main"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "t")
		if r.URL.Path == incPath {
			_, _ = w.Write([]byte(summaryFakeSource))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "TESTUSER", "pw")

	src, uri, err := c.GetSourceWithURI(context.Background(), "PROG", "ZDEMO_INC", nil)
	if err != nil || src != summaryFakeSource {
		t.Fatalf("GetSourceWithURI = %q, %v", src, err)
	}
	if uri != incPath {
		t.Fatalf("uri = %q, want the include that served the text %q", uri, incPath)
	}
	s := SummarizeSource("PROG", "ZDEMO_INC", nil, uri, src)
	if s.URI != incPath || s.Requested != "/sap/bc/adt/programs/programs/ZDEMO_INC/source/main" {
		t.Fatalf("uri/requested = %q/%q", s.URI, s.Requested)
	}
	if same := SummarizeSource("PROG", "ZDEMO_SUM", nil, "/sap/bc/adt/programs/programs/ZDEMO_SUM/source/main", src); same.Requested != "" {
		t.Fatalf("requested must be set only when it differs, got %q", same.Requested)
	}
}

// sourceHash is offered only where a guarded WriteSource takes it: not for a
// method-level read, whose WriteSource refuses expected_source_hash.
func TestSummarySourceHashOnlyWhereAGuardedWriteTakesIt(t *testing.T) {
	m := SummarizeSource("CLAS", "ZCL_X", &GetSourceOptions{Method: "RUN"}, "", summaryFakeSource)
	if m.SourceHash != "" || !strings.Contains(m.SourceHashNote, "method-level WriteSource") {
		t.Fatalf("method read: sourceHash=%q note=%q", m.SourceHash, m.SourceHashNote)
	}
	if f := SummarizeSource("FUNC", "Z_FM", &GetSourceOptions{Parent: "ZG"}, "", summaryFakeSource); f.SourceHash != "" || f.SourceHashNote == "" {
		t.Fatalf("FUNC read: sourceHash=%q note=%q", f.SourceHash, f.SourceHashNote)
	}
	for _, opts := range []*GetSourceOptions{nil, {Include: "testclasses"}} {
		c := SummarizeSource("CLAS", "ZCL_X", opts, "", summaryFakeSource)
		if c.SourceHash != summaryFakeSourceHash || c.SourceHashNote != "" {
			t.Fatalf("class read %+v: sourceHash=%q note=%q", opts, c.SourceHash, c.SourceHashNote)
		}
	}
}

// An ENHO body is read from the URI its search hit names or, failing that,
// from the plural alternate. The summary reports the one that served it.
func TestGetSourceWithURIReportsTheENHOEndpointThatServed(t *testing.T) {
	const body = "ENHANCEMENT 2 Y_TEST.\nENDENHANCEMENT.\n"
	const singular = "/sap/bc/adt/enhancements/enhoxh/y_test/source/main"
	const plural = "/sap/bc/adt/enhancements/enhoxhs/y_test/source/main"
	for _, served := range []string{singular, plural} {
		mock := &routedMock{byPath: map[string]*http.Response{
			"/sap/bc/adt/repository/informationsystem/search": newEnhancementSearchResponse("Y_TEST", "XH", "YSD"),
			served:                  newBody(body),
			"/sap/bc/adt/discovery": newBody("OK"),
		}}
		cfg := NewConfig("https://sap.example.com:44300", "u", "p")
		c := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

		src, uri, err := c.GetSourceWithURI(context.Background(), "ENHO", "Y_TEST", nil)
		if err != nil || src != body {
			t.Fatalf("served at %s: GetSourceWithURI = %q, %v", served, src, err)
		}
		if uri != served {
			t.Fatalf("served at %s: uri = %q", served, uri)
		}
		if s := SummarizeSource("ENHO", "Y_TEST", nil, uri, src); s.URI != served || s.Requested != "" {
			t.Fatalf("served at %s: summary uri/requested = %q/%q", served, s.URI, s.Requested)
		}
	}
}
