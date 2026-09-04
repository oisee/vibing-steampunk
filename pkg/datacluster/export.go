package datacluster

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Record is one cluster key with its fragments joined: what one IMPORT would
// read.
type Record struct {
	// Key holds the key columns that identify the cluster — RELID and the
	// table's own key fields, the client and SRTF2 left out — in column order.
	Key   []KeyValue
	Blob  []byte
	Parts int
}

// KeyValue is one key column of a cluster record.
type KeyValue struct {
	Column string `json:"column"`
	Value  string `json:"value"`
}

// KeyString renders the key as `COL=value, COL=value`.
func (r Record) KeyString() string {
	parts := make([]string, len(r.Key))
	for i, kv := range r.Key {
		parts[i] = kv.Column + "=" + kv.Value
	}
	return strings.Join(parts, ", ")
}

// Fragment columns, named as every INDX-like table names them.
const (
	colSeq    = "SRTF2"
	colLength = "CLUSTR"
	colData   = "CLUSTD"
)

var notKeyColumns = map[string]bool{
	"MANDT": true, "MANDANT": true, "CLIENT": true, "_DATAAGING": true,
	colSeq: true, colLength: true, colData: true,
}

// ReadExport reads a cluster table as SE16, SE16H or SE16N export it: a
// delimited text file with a header line naming the columns, one fragment per
// line. Comma, semicolon, tab and pipe are accepted as the delimiter. Rows are
// grouped by every column that is neither the client, the three fragment
// columns, nor a non-key attribute the caller names in ignore.
func ReadExport(r io.Reader, ignore ...string) ([]Record, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(string(raw), "\r", "")
	text = strings.TrimPrefix(text, "\ufeff")
	lines := strings.Split(text, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("datacluster: export is empty")
	}
	delim := detectDelimiter(lines[0])
	reader := csv.NewReader(strings.NewReader(strings.Join(lines, "\n")))
	reader.Comma = delim
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("datacluster: reading export: %w", err)
	}
	header := make([]string, len(rows[0]))
	for i, h := range rows[0] {
		header[i] = strings.ToUpper(strings.TrimSpace(h))
	}
	col := func(name string) int {
		for i, h := range header {
			if h == name {
				return i
			}
		}
		return -1
	}
	seqAt, lenAt, dataAt := col(colSeq), col(colLength), col(colData)
	if dataAt < 0 {
		return nil, fmt.Errorf("datacluster: export has no %s column (columns: %s)", colData, strings.Join(header, ", "))
	}
	skip := map[string]bool{}
	for k, v := range notKeyColumns {
		skip[k] = v
	}
	for _, name := range ignore {
		skip[strings.ToUpper(name)] = true
	}
	var keyCols []int
	for i, h := range header {
		if !skip[h] {
			keyCols = append(keyCols, i)
		}
	}

	type group struct {
		key   []KeyValue
		frags []Fragment
	}
	groups := map[string]*group{}
	var order []string
	for n, row := range rows[1:] {
		if len(row) == 0 || (len(row) == 1 && strings.TrimSpace(row[0]) == "") {
			continue
		}
		if len(row) <= dataAt {
			return nil, fmt.Errorf("datacluster: line %d has %d columns, header has %d", n+2, len(row), len(header))
		}
		var kv []KeyValue
		var id []string
		for _, i := range keyCols {
			v := ""
			if i < len(row) {
				v = strings.TrimSpace(row[i])
			}
			kv = append(kv, KeyValue{header[i], v})
			id = append(id, v)
		}
		key := strings.Join(id, "\x00")
		g, ok := groups[key]
		if !ok {
			g = &group{key: kv}
			groups[key] = g
			order = append(order, key)
		}
		f := Fragment{Seq: len(g.frags)}
		if seqAt >= 0 && seqAt < len(row) {
			if f.Seq, err = strconv.Atoi(strings.TrimSpace(row[seqAt])); err != nil {
				return nil, fmt.Errorf("datacluster: line %d: %s %q is not a number", n+2, colSeq, row[seqAt])
			}
		}
		if lenAt >= 0 && lenAt < len(row) {
			if f.Length, err = strconv.Atoi(strings.TrimSpace(row[lenAt])); err != nil {
				return nil, fmt.Errorf("datacluster: line %d: %s %q is not a number", n+2, colLength, row[lenAt])
			}
		}
		if f.Data, err = DecodeHex(row[dataAt]); err != nil {
			return nil, fmt.Errorf("datacluster: line %d: %w", n+2, err)
		}
		g.frags = append(g.frags, f)
	}
	var records []Record
	for _, key := range order {
		g := groups[key]
		blob, err := Join(g.frags)
		if err != nil {
			return nil, fmt.Errorf("datacluster: %s: %w", Record{Key: g.key}.KeyString(), err)
		}
		records = append(records, Record{Key: g.key, Blob: blob, Parts: len(g.frags)})
	}
	return records, nil
}

func detectDelimiter(header string) rune {
	best, bestCount := ',', strings.Count(header, ",")
	for _, d := range []rune{';', '\t', '|'} {
		if c := strings.Count(header, string(d)); c > bestCount {
			best, bestCount = d, c
		}
	}
	return best
}
