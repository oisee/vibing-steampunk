package datacluster

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Fragment is one database row of a cluster table: the SRTF2 sequence number,
// the CLUSTR byte count that is valid in this row, and the CLUSTD bytes.
type Fragment struct {
	Seq    int
	Length int
	Data   []byte
}

// Join puts the fragments of one cluster key back into one stream: sorted by
// sequence, each trimmed to the length its row declares. The last row of a
// cluster is the only one that is normally shorter than the column; the
// others are padded with zeros on the database and the padding must go.
func Join(fragments []Fragment) ([]byte, error) {
	if len(fragments) == 0 {
		return nil, fmt.Errorf("datacluster: no fragments")
	}
	sorted := append([]Fragment(nil), fragments...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Seq < sorted[j].Seq })
	var out []byte
	for i, f := range sorted {
		if i > 0 && f.Seq == sorted[i-1].Seq {
			return nil, fmt.Errorf("datacluster: fragment %d appears twice", f.Seq)
		}
		if f.Seq != i {
			return nil, fmt.Errorf("datacluster: fragment %d is missing", i)
		}
		n := f.Length
		if n <= 0 || n > len(f.Data) {
			n = len(f.Data)
		}
		out = append(out, f.Data[:n]...)
	}
	return out, nil
}

// DecodeHex accepts the CLUSTD column as the data preview and SE16 exports
// deliver it: upper- or lower-case hex, possibly with whitespace.
func DecodeHex(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, s)
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("datacluster: CLUSTD is not hex: %w", err)
	}
	return b, nil
}
