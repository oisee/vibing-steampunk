package vsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// abapGit escapes an object id into a filename, and the spelling has to match
// character for character or the pair does not round-trip. The expected value
// here is not derived from reading abapGit's source: it is the filename abapGit
// itself produced for this object, compared against a file on disk.
func TestAbapGitEscapeName(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"ZORK-MINI.Z3", "zork-mini%2ez3"},
		{"ZORK1.COM", "zork1%2ecom"},
		{"ZDEMO_LOGO.PNG", "zdemo_logo%2epng"},
		{"PLAIN", "plain"},
		{"WITH SPACE.TXT", "with%20space%2etxt"},
	} {
		if got := abapGitEscapeName(tc.in); got != tc.want {
			t.Errorf("abapGitEscapeName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWriteAbapGitW3MI(t *testing.T) {
	dir := t.TempDir()
	data := []byte{0x03, 0x00, 0xFF, 0x7F}

	dataPath, xmlPath, err := writeAbapGitW3MI(dir, "ZORK-MINI.Z3", ".z3",
		"application/octet-stream", "00001", len(data), data)
	if err != nil {
		t.Fatalf("writeAbapGitW3MI: %v", err)
	}

	if filepath.Base(dataPath) != "zork-mini%2ez3.w3mi.data.z3" {
		t.Errorf("data file is %q", filepath.Base(dataPath))
	}
	if filepath.Base(xmlPath) != "zork-mini%2ez3.w3mi.xml" {
		t.Errorf("xml file is %q", filepath.Base(xmlPath))
	}

	got, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("data round-trip: got % x, want % x", got, data)
	}

	xml, err := os.ReadFile(xmlPath)
	if err != nil {
		t.Fatalf("reading xml: %v", err)
	}
	for _, want := range []string{
		"<NAME>ZORK-MINI.Z3</NAME>",
		"<item><NAME>filesize</NAME><VALUE>4</VALUE></item>",
		"<item><NAME>mimetype</NAME><VALUE>application/octet-stream</VALUE></item>",
		"LCL_OBJECT_W3MI",
	} {
		if !strings.Contains(string(xml), want) {
			t.Errorf("xml missing %q\n%s", want, xml)
		}
	}

	// The uploader's workstation path must never reach a file meant to be
	// committed. This is the assertion that keeps that true if someone later
	// decides to write every WWWPARAMS entry out "for completeness".
	if strings.Contains(strings.ToLower(string(xml)), "filename") {
		t.Errorf("the xml carries a filename parameter — that is the uploader's "+
			"workstation path and is a live identifier:\n%s", xml)
	}
}

// An extension that is missing or unusable must still produce a file, because
// refusing to write one over a cosmetic detail would be worse than ".bin".
func TestWriteAbapGitW3MI_NoExtension(t *testing.T) {
	dir := t.TempDir()
	dataPath, _, err := writeAbapGitW3MI(dir, "NOEXT", "", "application/octet-stream", "1", 1, []byte{0})
	if err != nil {
		t.Fatalf("writeAbapGitW3MI: %v", err)
	}
	if filepath.Base(dataPath) != "noext.w3mi.data.bin" {
		t.Errorf("data file is %q, want noext.w3mi.data.bin", filepath.Base(dataPath))
	}
}
