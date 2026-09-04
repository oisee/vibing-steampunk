// Package temse reads what SAP's temporary sequential store holds: the spool
// requests of TSP01, whose content sits in TST03 as blocks of an object
// described in TST01. A list spool — the output of WRITE — is a sequence of
// records, each a line of the list with a print-control character in front
// and the list format's escapes inside, and this package turns that into
// lines of text.
//
// The record framing is TST01's DRECTYP "VY4----": a two-byte big-endian
// length covering the four-byte header and the body, two bytes of flags,
// and the body in the object's code page (DCHARCOD). The body's first
// character is the print control: a blank for an ordinary line, P for the
// end of a page. In the text, U+F8FF opens a five-character format command
// such as COL0N (a colour), and U+F8FC precedes one box-drawing character.
// The escapes were read off Unicode spools; a spool in a single-byte code
// page is refused rather than guessed at.
package temse

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
)

// Line is one record of a list spool.
type Line struct {
	// Page counts from one; a P record ends the page.
	Page int `json:"page"`
	// Control is the print-control character: ' ' for a line, 'P' for the
	// end of a page, anything else kept as it was.
	Control string `json:"control,omitempty"`
	// Text is the line with the format escapes removed and trailing blanks
	// trimmed.
	Text string `json:"text"`
	// Formats are the format commands the line carried, in order.
	Formats []string `json:"formats,omitempty"`
}

// List is a decoded list spool.
type List struct {
	Pages int    `json:"pages"`
	Lines []Line `json:"lines"`
	// Records is how many TemSe records the object held, page ends included.
	Records int `json:"records"`
}

const (
	escFormat  = 0xF8FF // followed by a five-character command
	escLineArt = 0xF8FC // followed by one box-drawing character
	ctlPageEnd = 'P'
)

// DecodeList reads the records of a list spool. charcod is TST01's DCHARCOD.
func DecodeList(data []byte, charcod string) (*List, error) {
	var bigEndian bool
	switch strings.TrimSpace(charcod) {
	case "4103":
	case "4102":
		bigEndian = true
	default:
		return nil, fmt.Errorf("temse: list in code page %q; only the Unicode pages 4103 and 4102 are read", charcod)
	}
	out := &List{Pages: 1}
	pos := 0
	for pos < len(data) {
		if pos+4 > len(data) {
			return nil, fmt.Errorf("temse: %d bytes left at offset %d, not enough for a record header", len(data)-pos, pos)
		}
		length := int(binary.BigEndian.Uint16(data[pos:]))
		if length < 4 || pos+length > len(data) {
			return nil, fmt.Errorf("temse: record at offset %d claims %d bytes, %d remain", pos, length, len(data)-pos)
		}
		body := data[pos+4 : pos+length]
		pos += length
		out.Records++
		text, err := decodeUTF16(body, bigEndian)
		if err != nil {
			return nil, fmt.Errorf("temse: record %d: %w", out.Records, err)
		}
		if text == "" {
			continue
		}
		control, rest := text[:1], text[1:]
		if control == string(ctlPageEnd) {
			out.Lines = append(out.Lines, Line{Page: out.Pages, Control: control})
			out.Pages++
			continue
		}
		line := Line{Page: out.Pages, Text: ""}
		if control != " " {
			line.Control = control
		}
		line.Text, line.Formats = stripEscapes(rest)
		out.Lines = append(out.Lines, line)
	}
	// A trailing page end counted a page that has nothing on it.
	if n := len(out.Lines); n > 0 && out.Lines[n-1].Control == string(ctlPageEnd) {
		out.Pages--
	}
	return out, nil
}

// stripEscapes removes the format escapes and returns the plain text and
// the commands it carried.
func stripEscapes(s string) (string, []string) {
	var b strings.Builder
	var formats []string
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case escFormat:
			end := i + 6
			if end > len(runes) {
				end = len(runes)
			}
			formats = append(formats, string(runes[i+1:end]))
			i = end - 1
		case escLineArt:
			// the character itself follows and is kept
		default:
			b.WriteRune(runes[i])
		}
	}
	return strings.TrimRight(b.String(), " "), formats
}

// Text joins the lines with newlines, a form feed between pages.
func (l *List) Text() string {
	var b strings.Builder
	for _, line := range l.Lines {
		if line.Control == string(ctlPageEnd) {
			b.WriteString("\f")
			continue
		}
		b.WriteString(line.Text)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\f\n") + "\n"
}

func decodeUTF16(raw []byte, bigEndian bool) (string, error) {
	if len(raw)%2 != 0 {
		return "", fmt.Errorf("%d bytes is not a whole number of UTF-16 units", len(raw))
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		if bigEndian {
			units[i] = binary.BigEndian.Uint16(raw[2*i:])
		} else {
			units[i] = binary.LittleEndian.Uint16(raw[2*i:])
		}
	}
	return string(utf16.Decode(units)), nil
}
