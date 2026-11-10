package hanadump

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
)

// reposrcData builds a REPOSRC.DATA value the way the database holds one:
// 0xFF, the SAP LZH header, then DEFLATE behind a two-bit prefix that says no
// noise bits follow. Inside, every line is its length in one UTF-16 unit and
// then its characters; width > 0 pads each line to that length, as older rows
// are stored. The text is ours, written for the test.
func reposrcData(t *testing.T, lines []string, width int, bigEndian bool) []byte {
	t.Helper()
	var units []uint16
	for _, l := range lines {
		u := utf16.Encode([]rune(l))
		for len(u) < width {
			u = append(u, ' ')
		}
		units = append(units, uint16(len(u)))
		units = append(units, u...)
	}
	raw := make([]byte, 2*len(units))
	for i, u := range units {
		if bigEndian {
			binary.BigEndian.PutUint16(raw[2*i:], u)
		} else {
			binary.LittleEndian.PutUint16(raw[2*i:], u)
		}
	}

	var d bytes.Buffer
	w, err := flate.NewWriter(&d, flate.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(raw)
	w.Close()
	def := d.Bytes()

	// Prefix 00 in the two lowest bits: the decoder shifts the whole body
	// right by two before inflating.
	body := make([]byte, len(def)+1)
	for i, b := range def {
		body[i] |= b << 2
		body[i+1] |= b >> 6
	}

	out := []byte{0xFF}
	out = binary.LittleEndian.AppendUint32(out, uint32(len(raw)))
	out = append(out, 0x12, 0x1F, 0x9D, 0x02)
	return append(out, body...)
}

var sample = []string{
	"REPORT zhanadump_sample.",
	"",
	"WRITE / 'ÿ is a letter, and 255 is a length'.",
	"  \" trailing blanks go   ",
}

func want() []string {
	w := append([]string(nil), sample...)
	w[3] = "  \" trailing blanks go"
	return w
}

func TestDecodeSource(t *testing.T) {
	for _, width := range []int{0, 72, 255} {
		for _, be := range []bool{true, false} {
			got, err := DecodeSource(reposrcData(t, sample, width, be))
			if err != nil {
				t.Fatalf("width=%d bigEndian=%v: %v", width, be, err)
			}
			if !reflect.DeepEqual(got, want()) {
				t.Errorf("width=%d bigEndian=%v:\n got %q\nwant %q", width, be, got, want())
			}
		}
	}
}

func TestDecodeSourceRejects(t *testing.T) {
	if got, err := DecodeSource(nil); err != nil || got != nil {
		t.Errorf("empty: %q, %v", got, err)
	}
	if _, err := DecodeSource([]byte{0xFF, 1, 2, 3}); err == nil {
		t.Error("garbage decoded without an error")
	}
	// A length running past the end, in either byte order.
	if _, err := splitLines([]byte{0x00, 0x09, 0x00, 'A'}); err == nil ||
		!strings.Contains(err.Error(), "length-prefixed") {
		t.Errorf("overlong line: %v", err)
	}
}

func TestFileName(t *testing.T) {
	if got := FileName("/NS/CL_X=====CP"); got != "#NS#CL_X=====CP" {
		t.Errorf("got %q", got)
	}
}
