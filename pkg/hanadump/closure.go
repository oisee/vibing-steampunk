package hanadump

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A dependency-closure request, as an iterator writes it after a transpile
// (schema 1): names per kind, the ones already asked for earlier, and the
// gaps that stayed open. Dictionary kinds are closed over transitively on
// the server; source kinds are selected as if named.
type ClosureRequest struct {
	Schema    int                 `json:"schema"`
	Iteration int                 `json:"iteration"`
	Requested map[string][]string `json:"requested"`
	Seen      map[string][]string `json:"seen"`
}

// ClosureResult is what the dump resolved: every dictionary name it reached,
// by kind, and the requested names the dictionary does not have.
type ClosureResult struct {
	Rounds   int                 `json:"rounds"`
	Resolved map[string][]string `json:"resolved"`
	// Missing: names the dictionary has no active version of
	Missing map[string][]string `json:"missing,omitempty"`
	// Unhandled: request kinds this dumper does not close over, with their
	// names, so the asker knows its request was only partly served
	Unhandled map[string][]string `json:"unhandled,omitempty"`
}

// ReadClosureRequest reads a request file.
func ReadClosureRequest(path string) (*ClosureRequest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r ClosureRequest
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("hanadump: closure request %s: %w", path, err)
	}
	if r.Schema != 1 {
		return nil, fmt.Errorf("hanadump: closure request %s: schema %d, expected 1", path, r.Schema)
	}
	return &r, nil
}

// SourceKinds are the request kinds selected by name, like --name.
var SourceKinds = []string{"CLAS", "INTF", "FUGR", "PROG"}

// dictKinds are closed over: tables and structures, data elements, domains,
// table types, search helps.
var dictKinds = []string{"TABL", "DTEL", "DOMA", "TTYP", "SHLP"}

// newNames returns the request's names of a kind that were not already seen.
func (r *ClosureRequest) newNames(kind string) []string {
	seen := map[string]bool{}
	for _, n := range r.Seen[kind] {
		seen[strings.TrimSpace(n)] = true
	}
	var out []string
	for _, n := range r.Requested[kind] {
		n = strings.TrimSpace(n)
		if n != "" && !seen[n] {
			out = append(out, n)
		}
	}
	return out
}

// SourcePatternsOf turns the request's source kinds into name patterns.
func (r *ClosureRequest) SourcePatternsOf() []string {
	var out []string
	for _, kind := range SourceKinds {
		for _, n := range r.newNames(kind) {
			out = append(out, SourcePatterns(PackageObject{Type: kind, Name: n})...)
		}
	}
	return out
}

// inChunks runs fn over values in slices small enough for one IN list.
func inChunks(values []string, fn func(placeholders string, args []any) error) error {
	const size = 500
	for at := 0; at < len(values); at += size {
		part := values[at:min(at+size, len(values))]
		args := make([]any, len(part))
		for i, v := range part {
			args[i] = v
		}
		if err := fn(strings.TrimSuffix(strings.Repeat("?,", len(part)), ","), args); err != nil {
			return err
		}
	}
	return nil
}

// closeDictionary follows the dictionary from the requested names until no
// new name appears: a table or structure to its fields' data elements,
// included/appended structures and nested structures and table types, a
// data element to its domain, a table type to its row type.
func closeDictionary(ctx context.Context, db *sql.DB, schema string, req *ClosureRequest) (*ClosureResult, error) {
	sets := map[string]map[string]bool{}
	for _, k := range dictKinds {
		sets[k] = map[string]bool{}
	}
	var todo = map[string][]string{}
	add := func(kind, name string) {
		name = strings.TrimSpace(name)
		if name == "" || sets[kind][name] {
			return
		}
		sets[kind][name] = true
		todo[kind] = append(todo[kind], name)
	}
	for _, k := range dictKinds {
		for _, n := range req.newNames(k) {
			add(k, n)
		}
	}
	// a field's component type (DD03L.COMPTYPE) or a row kind (DD40L.ROWKIND)
	byKind := func(kind, name string) {
		switch strings.TrimSpace(kind) {
		case "E":
			add("DTEL", name)
		case "S":
			add("TABL", name)
		case "L":
			add("TTYP", name)
		}
	}
	res := &ClosureResult{Resolved: map[string][]string{}, Missing: map[string][]string{}, Unhandled: map[string][]string{}}
	handled := map[string]bool{}
	for _, k := range append(append([]string{}, SourceKinds...), dictKinds...) {
		handled[k] = true
	}
	for kind := range req.Requested {
		if !handled[kind] {
			if names := req.newNames(kind); len(names) > 0 {
				res.Unhandled[kind] = names
			}
		}
	}
	if len(res.Unhandled) == 0 {
		res.Unhandled = nil
	}
	for len(todo["TABL"])+len(todo["DTEL"])+len(todo["TTYP"])+len(todo["SHLP"]) > 0 {
		res.Rounds++
		if res.Rounds > 50 {
			return nil, fmt.Errorf("hanadump: dictionary closure did not settle after 50 rounds")
		}
		round := todo
		todo = map[string][]string{}
		err := inChunks(round["TABL"], func(ph string, args []any) error {
			rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT "FIELDNAME", "ROLLNAME", "COMPTYPE", "PRECFIELD", "DOMNAME" FROM %q."DD03L" WHERE "AS4LOCAL" = 'A' AND "TABNAME" IN (%s)`, schema, ph), args...)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var field, roll, comp, prec, dom sql.NullString
				if err := rows.Scan(&field, &roll, &comp, &prec, &dom); err != nil {
					return err
				}
				if f := strings.TrimSpace(field.String); strings.HasPrefix(f, ".INCLU") || strings.HasPrefix(f, ".APPEND") {
					add("TABL", prec.String)
					continue
				}
				if roll.String != "" {
					byKind(comp.String, roll.String)
				} else if dom.String != "" {
					add("DOMA", dom.String)
				}
			}
			return rows.Err()
		})
		if err != nil {
			return nil, fmt.Errorf("hanadump: closure DD03L: %w", err)
		}
		err = inChunks(round["DTEL"], func(ph string, args []any) error {
			rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT "DOMNAME" FROM %q."DD04L" WHERE "AS4LOCAL" = 'A' AND "ROLLNAME" IN (%s)`, schema, ph), args...)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var dom sql.NullString
				if err := rows.Scan(&dom); err != nil {
					return err
				}
				add("DOMA", dom.String)
			}
			return rows.Err()
		})
		if err != nil {
			return nil, fmt.Errorf("hanadump: closure DD04L: %w", err)
		}
		err = inChunks(round["TTYP"], func(ph string, args []any) error {
			rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT "ROWTYPE", "ROWKIND" FROM %q."DD40L" WHERE "AS4LOCAL" = 'A' AND "TYPENAME" IN (%s)`, schema, ph), args...)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var row, kind sql.NullString
				if err := rows.Scan(&row, &kind); err != nil {
					return err
				}
				byKind(kind.String, row.String)
			}
			return rows.Err()
		})
		if err != nil {
			return nil, fmt.Errorf("hanadump: closure DD40L: %w", err)
		}
		// a collective search help's included helps (DD31S), and the data
		// elements of every help's parameters (DD32S)
		err = inChunks(round["SHLP"], func(ph string, args []any) error {
			for _, q := range []string{
				fmt.Sprintf(`SELECT 'SHLP', "SUBSHLP" FROM %q."DD31S" WHERE "AS4LOCAL" = 'A' AND "SHLPNAME" IN (%s)`, schema, ph),
				fmt.Sprintf(`SELECT 'DTEL', "ROLLNAME" FROM %q."DD32S" WHERE "AS4LOCAL" = 'A' AND "SHLPNAME" IN (%s)`, schema, ph),
			} {
				rows, err := db.QueryContext(ctx, q, args...)
				if err != nil {
					return err
				}
				for rows.Next() {
					var kind, name sql.NullString
					if err := rows.Scan(&kind, &name); err != nil {
						rows.Close()
						return err
					}
					add(kind.String, name.String)
				}
				if err := rows.Close(); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("hanadump: closure search helps: %w", err)
		}
	}
	// what the dictionary has, kind by kind: the rest is reported missing
	exists := map[string][2]string{"TABL": {"DD02L", "TABNAME"}, "DTEL": {"DD04L", "ROLLNAME"}, "DOMA": {"DD01L", "DOMNAME"}, "TTYP": {"DD40L", "TYPENAME"}, "SHLP": {"DD30L", "SHLPNAME"}}
	for _, k := range dictKinds {
		names := keys(sets[k])
		found := map[string]bool{}
		t := exists[k]
		err := inChunks(names, func(ph string, args []any) error {
			rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT DISTINCT %q FROM %q.%q WHERE "AS4LOCAL" = 'A' AND %q IN (%s)`, t[1], schema, t[0], t[1], ph), args...)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var n string
				if err := rows.Scan(&n); err != nil {
					return err
				}
				found[strings.TrimSpace(n)] = true
			}
			return rows.Err()
		})
		if err != nil {
			return nil, fmt.Errorf("hanadump: closure %s: %w", t[0], err)
		}
		for _, n := range names {
			if found[n] {
				res.Resolved[k] = append(res.Resolved[k], n)
			} else {
				res.Missing[k] = append(res.Missing[k], n)
			}
		}
	}
	if len(res.Missing) == 0 {
		res.Missing = nil
	}
	return res, nil
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// dumpByValues writes the rows whose column is one of values, in IN-list
// chunks, to tables/<table>.jsonl -- a table the name patterns cannot reach,
// such as FUPARAREF keyed by the function modules TFDIR returned.
func dumpByValues(ctx context.Context, db *sql.DB, opts Options, table, column string, values []string) (int, bool, error) {
	if len(values) == 0 {
		return 0, false, nil
	}
	total, first := 0, true
	err := inChunks(values, func(ph string, args []any) error {
		n, missing, err := dumpTableTo(ctx, db, opts, table, fmt.Sprintf(`%q IN (%s)`, column, ph), args, !first)
		first = false
		total += n
		if missing {
			return errMissing{err}
		}
		return err
	})
	if m, ok := err.(errMissing); ok {
		// only a first query can find the table absent (dumpTableTo)
		return total, true, m.err
	}
	return total, false, err
}

type errMissing struct{ err error }

func (e errMissing) Error() string { return e.err.Error() }

// columnValues reads one column of a dumped table's rows back.
func columnValues(out, table, column string) ([]string, error) {
	f, err := os.Open(filepath.Join(out, "tables", FileName(table)+".jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var row map[string]any
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			return nil, err
		}
		if v, ok := row[column].(string); ok && strings.TrimSpace(v) != "" {
			seen[strings.TrimSpace(v)] = true
		}
	}
	return keys(seen), sc.Err()
}
