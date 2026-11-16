package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/hanadump"
)

// Object is one TADIR object in scope, classified.
type Object struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Package string `json:"package"`
	Author  string `json:"author,omitempty"`
	// Class is "customer" or "sap", by the rule in classify.
	Class        string `json:"class"`
	Generated    bool   `json:"generated,omitempty"`
	SrcSystemSAP bool   `json:"srcsystem_sap,omitempty"`
	// SourceBytes is the stored (compressed) size of the object's active
	// REPOSRC rows: an estimate of the work, not of the output.
	SourceBytes int64 `json:"source_bytes_compressed"`
	Programs    int   `json:"programs"`
}

// scope reads the TADIR objects a request selects (everything, for an empty
// request) and classifies them.
func scope(ctx context.Context, db *sql.DB, schema string, req *Request) ([]Object, error) {
	var conds []string
	var args []any
	like := func(col string, pats []string) {
		parts := make([]string, len(pats))
		for i, p := range pats {
			parts[i] = fmt.Sprintf(`%q LIKE ? ESCAPE '\'`, col)
			args = append(args, p)
		}
		conds = append(conds, "("+strings.Join(parts, " OR ")+")")
	}
	if len(req.Names) > 0 {
		like("OBJ_NAME", req.Names)
	}
	if len(req.Packages) > 0 {
		like("DEVCLASS", req.Packages)
	}
	if req.Local != nil && (req.Local.All || len(req.Local.Authors) > 0) {
		c := fmt.Sprintf(`("DEVCLASS" LIKE '$%%' OR "DEVCLASS" IN (SELECT "DEVCLASS" FROM %q."TDEVC" WHERE "DLVUNIT" = 'LOCAL'))`, schema)
		if len(req.Local.Authors) > 0 {
			c = "(" + c + ` AND "AUTHOR" IN (` + strings.TrimSuffix(strings.Repeat("?,", len(req.Local.Authors)), ",") + "))"
			for _, a := range req.Local.Authors {
				args = append(args, a)
			}
		}
		conds = append(conds, c)
	}
	var reqNames []string
	for _, names := range req.Requested {
		reqNames = append(reqNames, names...)
	}
	if len(reqNames) > 0 {
		conds = append(conds, `"OBJ_NAME" IN (`+strings.TrimSuffix(strings.Repeat("?,", len(reqNames)), ",")+")")
		for _, n := range reqNames {
			args = append(args, n)
		}
	}
	where := `"PGMID" = 'R3TR'`
	if len(conds) > 0 {
		where += " AND (" + strings.Join(conds, " OR ") + ")"
	}
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT "OBJECT", "OBJ_NAME", "DEVCLASS", "AUTHOR", "SRCSYSTEM", "GENFLAG" FROM %q."TADIR" WHERE %s ORDER BY "OBJECT", "OBJ_NAME"`, schema, where), args...)
	if err != nil {
		return nil, fail(exitUnsupported, "TADIR: %v", err)
	}
	defer rows.Close()
	var objs []Object
	var srcsys = map[int]string{}
	for rows.Next() {
		var o Object
		var src, gen sql.NullString
		var author sql.NullString
		if err := rows.Scan(&o.Type, &o.Name, &o.Package, &author, &src, &gen); err != nil {
			return nil, err
		}
		o.Type, o.Name, o.Package = strings.TrimSpace(o.Type), strings.TrimSpace(o.Name), strings.TrimSpace(o.Package)
		o.Author = strings.TrimSpace(author.String)
		o.Generated = strings.TrimSpace(gen.String) != ""
		srcsys[len(objs)] = strings.TrimSpace(src.String)
		objs = append(objs, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	ns, err := namespaces(ctx, db, schema)
	if err != nil {
		return nil, err
	}
	local, err := localPackages(ctx, db, schema)
	if err != nil {
		return nil, err
	}
	var pats []string
	if len(objs) <= 5000 { // small scope: ask for its programs only
		seen := map[string]bool{}
		for _, o := range objs {
			for _, p := range hanadump.SourcePatterns(hanadump.PackageObject{Type: o.Type, Name: o.Name, Package: o.Package}) {
				if !seen[p] {
					seen[p] = true
					pats = append(pats, p)
				}
			}
		}
	}
	sizes, err := sourceSizes(ctx, db, schema, pats, len(objs) > 5000)
	if err != nil {
		return nil, err
	}
	for i := range objs {
		o := &objs[i]
		o.Class = classify(o.Name, o.Package, ns, local)
		o.SrcSystemSAP = srcsys[i] == "SAP"
		o.Programs, o.SourceBytes = sizes.of(hanadump.SourcePatterns(hanadump.PackageObject{Type: o.Type, Name: o.Name, Package: o.Package}))
	}
	return objs, nil
}

// classify: customer when the object's name is in the customer name range,
// or its package is local, or its package's name is in the customer range.
// The customer name range is Z*, Y*, and /NS/... for every namespace this
// system is the producer of (TRNSPACE ROLE 'P'). Everything else is sap.
// TADIR SRCSYSTEM does not decide (copies between systems change it).
func classify(name, pkg string, producer map[string]bool, local map[string]bool) string {
	if customerName(name, producer) || strings.HasPrefix(pkg, "$") || local[pkg] || customerName(pkg, producer) {
		return "customer"
	}
	return "sap"
}

func customerName(name string, producer map[string]bool) bool {
	if name == "" {
		return false
	}
	switch name[0] {
	case 'Z', 'Y':
		return true
	case '/':
		if i := strings.Index(name[1:], "/"); i >= 0 {
			return producer[name[:i+2]]
		}
	}
	return false
}

func namespaces(ctx context.Context, db *sql.DB, schema string) (map[string]bool, error) {
	names, err := strings1(ctx, db, fmt.Sprintf(`SELECT "NAMESPACE" FROM %q."TRNSPACE" WHERE "ROLE" = 'P'`, schema))
	if err != nil {
		return nil, fail(exitUnsupported, "TRNSPACE: %v", err)
	}
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m, nil
}

func localPackages(ctx context.Context, db *sql.DB, schema string) (map[string]bool, error) {
	names, err := strings1(ctx, db, fmt.Sprintf(`SELECT "DEVCLASS" FROM %q."TDEVC" WHERE "DLVUNIT" = 'LOCAL'`, schema))
	if err != nil {
		return nil, fail(exitUnsupported, "TDEVC: %v", err)
	}
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m, nil
}

// progSizes is every active REPOSRC program with its stored size, sorted.
type progSizes struct {
	names []string
	size  []int64
}

func sourceSizes(ctx context.Context, db *sql.DB, schema string, patterns []string, all bool) (*progSizes, error) {
	p := &progSizes{}
	read := func(where string, args []any) error {
		rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT "PROGNAME", LENGTH("DATA") FROM %q."REPOSRC" WHERE "R3STATE" = 'A' AND %s`, schema, where), args...)
		if err != nil {
			return fail(exitUnsupported, "REPOSRC: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var n string
			var l sql.NullInt64
			if err := rows.Scan(&n, &l); err != nil {
				return err
			}
			p.names = append(p.names, strings.TrimRight(n, " "))
			p.size = append(p.size, l.Int64)
		}
		return rows.Err()
	}
	if all {
		if err := read("1 = 1", nil); err != nil {
			return nil, err
		}
	} else {
		for i := 0; i < len(patterns); i += 200 {
			chunk := patterns[i:min(i+200, len(patterns))]
			parts := make([]string, len(chunk))
			args := make([]any, len(chunk))
			for j, pat := range chunk {
				parts[j], args[j] = `"PROGNAME" LIKE ? ESCAPE '\'`, pat
			}
			if err := read("("+strings.Join(parts, " OR ")+")", args); err != nil {
				return nil, err
			}
		}
	}
	// sorted and unique by name, whatever the database's collation and
	// however the chunks overlapped
	idx := make([]int, len(p.names))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return p.names[idx[a]] < p.names[idx[b]] })
	q := &progSizes{}
	for _, j := range idx {
		if n := len(q.names); n > 0 && q.names[n-1] == p.names[j] {
			continue
		}
		q.names = append(q.names, p.names[j])
		q.size = append(q.size, p.size[j])
	}
	return q, nil
}

// of counts the programs matched by LIKE patterns of the two shapes
// SourcePatterns makes: an exact (escaped) name, or an escaped prefix + '%'.
func (p *progSizes) of(patterns []string) (n int, bytes int64) {
	seen := map[int]bool{}
	for _, pat := range patterns {
		lit, prefix := unlike(pat)
		i := sort.SearchStrings(p.names, lit)
		for ; i < len(p.names); i++ {
			if prefix && !strings.HasPrefix(p.names[i], lit) || !prefix && p.names[i] != lit {
				break
			}
			if !seen[i] {
				seen[i] = true
				n++
				bytes += p.size[i]
			}
		}
	}
	return n, bytes
}

// unlike undoes hanadump.EscapeLike and reports a trailing unescaped '%'.
func unlike(pat string) (lit string, prefix bool) {
	var b strings.Builder
	for i := 0; i < len(pat); i++ {
		switch {
		case pat[i] == '\\' && i+1 < len(pat):
			i++
			b.WriteByte(pat[i])
		case pat[i] == '%' && i == len(pat)-1:
			prefix = true
		default:
			b.WriteByte(pat[i])
		}
	}
	return b.String(), prefix
}

type group struct {
	Type    string `json:"type"`
	Package string `json:"package"`
	Class   string `json:"class"`
	Count   int    `json:"count"`
	Bytes   int64  `json:"source_bytes_compressed"`
}

type listSummary struct {
	OK        bool    `json:"ok"`
	Schema    string  `json:"abap_schema"`
	Objects   int     `json:"objects"`
	Customer  int     `json:"customer"`
	SAP       int     `json:"sap"`
	Generated int     `json:"generated"`
	Bytes     int64   `json:"source_bytes_compressed"`
	Groups    []group `json:"groups"`
	File      string  `json:"file,omitempty"`
}

func summarize(schema string, objs []Object) listSummary {
	s := listSummary{OK: true, Schema: schema, Objects: len(objs), Groups: []group{}}
	idx := map[[3]string]int{}
	for _, o := range objs {
		if o.Class == "customer" {
			s.Customer++
		} else {
			s.SAP++
		}
		if o.Generated {
			s.Generated++
		}
		s.Bytes += o.SourceBytes
		k := [3]string{o.Type, o.Package, o.Class}
		i, ok := idx[k]
		if !ok {
			i = len(s.Groups)
			idx[k] = i
			s.Groups = append(s.Groups, group{Type: o.Type, Package: o.Package, Class: o.Class})
		}
		s.Groups[i].Count++
		s.Groups[i].Bytes += o.SourceBytes
	}
	sort.Slice(s.Groups, func(a, b int) bool {
		x, y := s.Groups[a], s.Groups[b]
		if x.Package != y.Package {
			return x.Package < y.Package
		}
		return x.Type < y.Type
	})
	return s
}

func runList(ctx context.Context, db *sql.DB, schema string, req *Request, out string, allowInRepo bool, w io.Writer) error {
	objs, err := scope(ctx, db, schema, req)
	if err != nil {
		return err
	}
	s := summarize(schema, objs)
	if out != "" {
		if err := hanadump.PrepareOut(out, allowInRepo); err != nil {
			return fail(exitRefused, "%v", err)
		}
		s.File = filepath.Join(out, "objects.jsonl")
		if err := writeJSONL(s.File, objs); err != nil {
			return err
		}
	}
	writeJSONLine(w, s)
	return nil
}

func writeJSONL[T any](path string, items []T) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(f)
	enc := json.NewEncoder(bw)
	for _, it := range items {
		if err := enc.Encode(it); err != nil {
			f.Close()
			return err
		}
	}
	if err := bw.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
