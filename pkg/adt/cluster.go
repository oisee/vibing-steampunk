package adt

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/datacluster"
)

// Cluster tables — BALDAT, INDX, STXL and every table built like them — hold
// data that ABAP wrote with EXPORT ... TO DATABASE and that only IMPORT reads
// back. The table itself is ordinary: a key, a sequence number SRTF2, a byte
// count CLUSTR and a RAW column CLUSTD, and ADT's data preview reads all of
// them. What ADT will not do is decode CLUSTD, which is why this file exists:
// it reads the fragments with free SQL, joins them per key, and hands the
// stream to pkg/datacluster.

// ClusterTableInfo describes how a cluster table is keyed.
type ClusterTableInfo struct {
	Name string
	// Client is the client column, when the table has one.
	Client string
	// Keys are the columns that identify one cluster: the key without the
	// client and without SRTF2, in table order. RELID is the first of them.
	Keys []string
}

// ClusterTable reads a table's field list from DD03L and checks that it is a
// cluster table: keyed with SRTF2, carrying CLUSTR and CLUSTD. DD03L rather
// than the DDL source, because a key that lives in an include — LTDX keeps
// its whole key in one — is a line `include ltdxkey;` in the DDL and a row
// per field in DD03L.
func (c *Client) ClusterTable(ctx context.Context, table string) (*ClusterTableInfo, error) {
	table = strings.ToUpper(strings.TrimSpace(table))
	query := fmt.Sprintf("SELECT position, fieldname, keyflag, rollname, datatype FROM dd03l WHERE tabname = '%s' AND as4local = 'A' ORDER BY position", sqlQuote(table))
	res, err := c.RunQuery(ctx, query, 1000)
	if err != nil {
		return nil, fmt.Errorf("reading the fields of %s: %w", table, err)
	}
	if res == nil || len(res.Rows) == 0 {
		return nil, fmt.Errorf("%s is not an active table in DDIC", table)
	}
	info := &ClusterTableInfo{Name: table}
	hasSeq, hasLen, hasData := false, false, false
	for _, row := range res.Rows {
		name := cell(row, "FIELDNAME")
		if strings.HasPrefix(name, ".") {
			continue // .INCLUDE / .APPEND markers; their fields follow
		}
		switch name {
		case "SRTF2":
			hasSeq = true
			continue
		case "CLUSTR":
			hasLen = true
			continue
		case "CLUSTD":
			hasData = true
			continue
		}
		if cell(row, "KEYFLAG") != "X" {
			continue
		}
		if cell(row, "DATATYPE") == "CLNT" || name == "MANDT" || name == "MANDANT" || name == "CLIENT" {
			info.Client = name
			continue
		}
		info.Keys = append(info.Keys, name)
	}
	if !hasSeq || !hasLen || !hasData {
		return nil, fmt.Errorf("%s is not a cluster table: it needs SRTF2, CLUSTR and CLUSTD columns", table)
	}
	if len(info.Keys) == 0 {
		return nil, fmt.Errorf("%s has no key column besides the client and SRTF2", table)
	}
	return info, nil
}

// ClusterRecords is what ReadClusterRecords returns: the joined clusters and
// whether the row limit cut the read short.
type ClusterRecords struct {
	Table   *ClusterTableInfo
	Records []datacluster.Record
	// Fragments is how many database rows were read.
	Fragments int
	// Truncated is set when the row limit was reached. The last record is
	// then dropped rather than returned incomplete, and a caller who wants it
	// narrows the WHERE clause or raises the limit.
	Truncated bool
}

// ReadClusterRecords reads the fragments matching the WHERE clause (which
// may be empty) and joins them per key, newest keys in whatever order the
// database returns them, fragments in SRTF2 order. maxRows caps the database
// rows read, not the clusters returned.
func (c *Client) ReadClusterRecords(ctx context.Context, table, where string, maxRows int) (*ClusterRecords, error) {
	info, err := c.ClusterTable(ctx, table)
	if err != nil {
		return nil, err
	}
	if maxRows <= 0 {
		maxRows = 500
	}
	cols := append(append([]string{}, info.Keys...), "SRTF2", "CLUSTR", "CLUSTD")
	query := "SELECT " + strings.ToLower(strings.Join(cols, ", ")) + " FROM " + strings.ToLower(info.Name)
	if where = strings.TrimSpace(where); where != "" {
		query += " WHERE " + where
	}
	query += " ORDER BY " + strings.ToLower(strings.Join(append(append([]string{}, info.Keys...), "SRTF2"), ", "))

	res, err := c.RunQuery(ctx, query, maxRows)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", info.Name, err)
	}
	out := &ClusterRecords{Table: info}
	if res == nil {
		return out, nil
	}
	out.Fragments = len(res.Rows)
	out.Truncated = len(res.Rows) >= maxRows

	type group struct {
		key   []datacluster.KeyValue
		frags []datacluster.Fragment
	}
	var groups []*group
	byKey := map[string]*group{}
	for _, row := range res.Rows {
		var kv []datacluster.KeyValue
		var id []string
		for _, k := range info.Keys {
			v := cell(row, k)
			kv = append(kv, datacluster.KeyValue{Column: k, Value: v})
			id = append(id, v)
		}
		key := strings.Join(id, "\x00")
		g, ok := byKey[key]
		if !ok {
			g = &group{key: kv}
			byKey[key] = g
			groups = append(groups, g)
		}
		seq, _ := strconv.Atoi(cell(row, "SRTF2"))
		length, _ := strconv.Atoi(cell(row, "CLUSTR"))
		data, err := datacluster.DecodeHex(cell(row, "CLUSTD"))
		if err != nil {
			return nil, fmt.Errorf("%s %s fragment %d: %w", info.Name, datacluster.Record{Key: kv}.KeyString(), seq, err)
		}
		g.frags = append(g.frags, datacluster.Fragment{Seq: seq, Length: length, Data: data})
	}
	if out.Truncated && len(groups) > 0 {
		groups = groups[:len(groups)-1]
	}
	for _, g := range groups {
		blob, err := datacluster.Join(g.frags)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", info.Name, datacluster.Record{Key: g.key}.KeyString(), err)
		}
		out.Records = append(out.Records, datacluster.Record{Key: g.key, Blob: blob, Parts: len(g.frags)})
	}
	return out, nil
}
