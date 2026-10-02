package adt

import (
	"crypto/sha256"
	"encoding/hex"
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
	s := SummarizeSource("prog", "zdemo_sum", nil, summaryFakeSource)
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

	got := SummarizeSource("PROG", "ZDEMO_SUM", nil, summaryFakeSource).SHA256
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
