package adt

import (
	"context"
	"fmt"
	"regexp"
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

var ddlFieldLine = regexp.MustCompile(`(?im)^\s*(key\s+)?([a-z0-9_/]+)\s*:\s*([a-z0-9_/]+)`)

// ClusterTable reads a table's definition and checks that it is a cluster
// table: keyed with SRTF2 last and carrying CLUSTR and CLUSTD.
func (c *Client) ClusterTable(ctx context.Context, table string) (*ClusterTableInfo, error) {
	table = strings.ToUpper(strings.TrimSpace(table))
	ddl, err := c.GetTable(ctx, table)
	if err != nil {
		return nil, err
	}
	info := &ClusterTableInfo{Name: table}
	hasSeq, hasLen, hasData := false, false, false
	for _, m := range ddlFieldLine.FindAllStringSubmatch(ddl, -1) {
		isKey, name, typ := m[1] != "", strings.ToUpper(m[2]), strings.ToUpper(m[3])
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
		if !isKey {
			continue
		}
		if typ == "MANDT" || name == "MANDT" || name == "MANDANT" || name == "CLIENT" {
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
