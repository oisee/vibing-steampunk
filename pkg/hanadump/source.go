// Package hanadump reads ABAP repository content straight out of the HANA
// database behind an AS ABAP — no ABAP stack, no ADT, no logon. It is what is
// left when the application server will not start: an expired licence, a
// retired image, a restored backup with nothing on top of it.
//
// The package writes a flat dump: every selected REPOSRC row decompressed and
// decoded to plain text, and the rows of a few repository tables beside it,
// each record carrying where it came from. Turning that into abapGit or
// anything else is a separate step that can be rerun without the database.
//
// What counts as "the user's own" is not decided here. The caller passes name
// patterns; nothing is filtered out behind their back.
package hanadump

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/oisee/vibing-steampunk/pkg/sapcompress"
)

// DecodeSource turns the DATA column of one REPOSRC row into source lines,
// trailing blanks removed.
//
// The column holds one 0xFF byte, then a SAP-compressed stream (see
// pkg/sapcompress). Decompressed, it is UTF-16 text as a run of lines, each
// one code unit giving its length followed by that many code units. Older
// rows pad every line to the program's maximum length, so a stream of them
// reads as if every line opened with U+00FF; it is a length of 255 all the
// same. Unicode systems observed so far store it big-endian; the byte order
// is chosen by which reading consumes the stream exactly.
func DecodeSource(data []byte) ([]string, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if data[0] == 0xFF {
		data = data[1:]
	}
	raw, err := sapcompress.Decompress(data)
	if err != nil {
		return nil, err
	}
	return splitLines(raw)
}

func splitLines(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw)%2 != 0 {
		return nil, fmt.Errorf("hanadump: %d bytes is not UTF-16", len(raw))
	}
	units := make([]uint16, len(raw)/2)
	for _, order := range []binary.ByteOrder{binary.BigEndian, binary.LittleEndian} {
		for i := range units {
			units[i] = order.Uint16(raw[2*i:])
		}
		if lines, ok := cut(units); ok {
			return lines, nil
		}
	}
	return nil, errors.New("hanadump: decompressed source is not a run of length-prefixed lines")
}

// cut reads length-prefixed lines and reports whether they consume the units
// exactly. A length past maxLineLen is taken as the wrong byte order rather
// than trusted.
func cut(units []uint16) ([]string, bool) {
	var lines []string
	for i := 0; i < len(units); {
		n := int(units[i])
		i++
		if n > maxLineLen || i+n > len(units) {
			return nil, false
		}
		lines = append(lines, strings.TrimRight(string(utf16.Decode(units[i:i+n])), " "))
		i += n
	}
	return lines, true
}

// maxLineLen is generous: the editor stops at 255, the kernel's limit is
// higher for generated code.
const maxLineLen = 4096

// FileName is the dump's file name for a repository object: the name as SAP
// spells it, with the namespace slash — the one character a file name cannot
// hold — turned into '#', the way abapGit does it.
func FileName(name string) string {
	return strings.ReplaceAll(name, "/", "#")
}
