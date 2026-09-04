package temse

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// list_unicode.hex is the TemSe object of a one-page list spool written by a
// background job on a 7.58 system: a header line with two colour commands,
// a rule of box-drawing characters, one line of output, and the page end.
func TestDecodeList(t *testing.T) {
	raw, err := os.ReadFile("testdata/list_unicode.hex")
	if err != nil {
		t.Fatal(err)
	}
	data, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	l, err := DecodeList(data, "4103")
	if err != nil {
		t.Fatal(err)
	}
	if l.Records != 4 || l.Pages != 1 || len(l.Lines) != 4 {
		t.Fatalf("records %d pages %d lines %d", l.Records, l.Pages, len(l.Lines))
	}
	head := l.Lines[0]
	if !strings.HasPrefix(head.Text, "04.09.2026") || !strings.Contains(head.Text, "vsp: deep objects to INDX for cluster parser probes") || !strings.HasSuffix(head.Text, "1") {
		t.Errorf("header line: %q", head.Text)
	}
	if strings.Join(head.Formats, ",") != "COL0N,COL0H" {
		t.Errorf("formats: %v", head.Formats)
	}
	if rule := l.Lines[1].Text; !strings.HasPrefix(rule, "───") || strings.ContainsRune(rule, escLineArt) || len([]rune(rule)) != 255 {
		t.Errorf("rule: %q (%d runes)", rule[:12], len([]rune(rule)))
	}
	if l.Lines[2].Text != "exported VSPDEEP to INDX(ZE)" {
		t.Errorf("body: %q", l.Lines[2].Text)
	}
	if l.Lines[3].Control != "P" || l.Lines[3].Text != "" {
		t.Errorf("page end: %+v", l.Lines[3])
	}
	text := l.Text()
	if !strings.HasSuffix(text, "exported VSPDEEP to INDX(ZE)\n") || strings.Count(text, "\n") != 3 {
		t.Errorf("text: %q", text)
	}
	if _, err := DecodeList(data, "1100"); err == nil {
		t.Error("single-byte code page accepted")
	}
	if _, err := DecodeList(data[:len(data)-10], "4103"); err == nil {
		t.Error("truncated data accepted")
	}
}
