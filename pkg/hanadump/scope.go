package hanadump

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// What a dump selects, beyond name patterns: packages (resolved through
// TADIR to the names of their objects), the DDIC and CDS tables, and raw
// tables the caller names. Nothing here decides what is the caller's to
// dump; the caller does.

// DDICTables are the dictionary rows of the selected objects -- tables and
// their fields, data elements, domains and their fixed values, table types,
// their short texts -- and the sources kept outside REPOSRC: CDS views
// (DDDDLSRC), metadata extensions (DDLXSRC_SRC), service definitions
// (SRVDSRC_SRC) and access controls (ACMDCLSRC), each keyed by the object's
// name. (Behaviour definitions are in REPOSRC, as "<name>====...BD".)
var DDICTables = []Table{
	{"DD02L", "TABNAME"}, {"DD02T", "TABNAME"},
	{"DD03L", "TABNAME"},
	{"DD04L", "ROLLNAME"}, {"DD04T", "ROLLNAME"},
	{"DD01L", "DOMNAME"}, {"DD01T", "DOMNAME"},
	{"DD07L", "DOMNAME"},
	{"DD40L", "TYPENAME"},
	{"DDDDLSRC", "DDLNAME"},
	{"DDLXSRC_SRC", "DDLXNAME"},
	{"SRVDSRC_SRC", "SRVDNAME"},
	{"ACMDCLSRC", "DCLNAME"},
	// search helps: header, texts, parameters, and the included ones
	{"DD30L", "SHLPNAME"}, {"DD30T", "SHLPNAME"}, {"DD31S", "SHLPNAME"}, {"DD32S", "SHLPNAME"},
}

// RawTable is a table dumped as the caller asked: Pattern is a LIKE pattern
// on the table name (so "/IWBEP/I_SBD_%" names several), and Column with
// Values filters rows (Column LIKE any of Values); no Column means every row.
type RawTable struct {
	Pattern string   `json:"pattern"`
	Column  string   `json:"column,omitempty"`
	Values  []string `json:"values,omitempty"`
}

// ParseRawTable reads NAME, or NAME:COLUMN=V1,V2 -- NAME and the values are
// LIKE patterns, COLUMN a plain column name.
func ParseRawTable(spec string) (RawTable, error) {
	name, filter, hasFilter := strings.Cut(spec, ":")
	t := RawTable{Pattern: name}
	if !isPattern(name) {
		return t, fmt.Errorf("hanadump: table %q: not a table name or LIKE pattern", name)
	}
	if !hasFilter {
		return t, nil
	}
	col, values, ok := strings.Cut(filter, "=")
	if !ok || !isIdent(col) || values == "" {
		return t, fmt.Errorf("hanadump: table filter %q: expected COLUMN=PATTERN[,PATTERN]", filter)
	}
	t.Column = col
	for _, v := range strings.Split(values, ",") {
		if v == "" {
			return t, fmt.Errorf("hanadump: table filter %q: empty pattern", filter)
		}
		t.Values = append(t.Values, v)
	}
	return t, nil
}

// isPattern admits a table name with LIKE wildcards: isIdent plus '%'.
func isPattern(s string) bool {
	return isIdent(strings.ReplaceAll(s, "%", "X"))
}

// EscapeLike makes a name match only itself in a LIKE with ESCAPE '\':
// SAP names hold '_', which LIKE reads as "any one character".
func EscapeLike(name string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(name)
}

// PackageObject is one TADIR row of a selected package.
type PackageObject struct {
	Type, Name, Package string
}

// SourcePatterns are the name patterns that select an object: its own name,
// for the tables keyed by it (SEOCLASS, TLIBG, DD02L, ...), and the REPOSRC
// program names that hold its source -- a class or interface pool's and a
// behaviour definition's includes all start with its name padded to 30 with
// '=', a function group's with SAPL / L after its namespace.
func SourcePatterns(o PackageObject) []string {
	name := EscapeLike(o.Name)
	switch o.Type {
	case "CLAS", "INTF", "BDEF":
		padded := name
		if len(o.Name) < 30 {
			padded = EscapeLike(o.Name + strings.Repeat("=", 30-len(o.Name)))
		}
		return []string{name, padded + "%"}
	case "FUGR":
		ns, group := "", o.Name
		if strings.HasPrefix(o.Name, "/") {
			if i := strings.Index(o.Name[1:], "/"); i >= 0 {
				ns, group = o.Name[:i+2], o.Name[i+2:]
			}
		}
		return []string{name, EscapeLike(ns + "SAPL" + group), EscapeLike(ns+"L"+group) + "%"}
	default:
		return []string{name}
	}
}

// resolvePackages reads the selected packages' objects from TADIR and turns
// them into name patterns.
func resolvePackages(ctx context.Context, db *sql.DB, schema string, packages []string) ([]PackageObject, []string, error) {
	where, args := likeAny("DEVCLASS", packages)
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT OBJECT, OBJ_NAME, DEVCLASS FROM %q."TADIR" WHERE "PGMID" = 'R3TR' AND %s ORDER BY OBJECT, OBJ_NAME`, schema, where), args...)
	if err != nil {
		return nil, nil, fmt.Errorf("hanadump: TADIR for packages: %w", err)
	}
	defer rows.Close()
	var objects []PackageObject
	seen := map[string]bool{}
	var patterns []string
	for rows.Next() {
		var o PackageObject
		if err := rows.Scan(&o.Type, &o.Name, &o.Package); err != nil {
			return nil, nil, err
		}
		o.Type, o.Name, o.Package = strings.TrimSpace(o.Type), strings.TrimSpace(o.Name), strings.TrimSpace(o.Package)
		objects = append(objects, o)
		for _, p := range SourcePatterns(o) {
			if !seen[p] {
				seen[p] = true
				patterns = append(patterns, p)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	sort.Strings(patterns)
	return objects, patterns, nil
}

// expandRawTable lists the schema's tables whose name matches t.Pattern.
func expandRawTable(ctx context.Context, db *sql.DB, schema string, t RawTable) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT TABLE_NAME FROM SYS.TABLES WHERE SCHEMA_NAME = ? AND TABLE_NAME LIKE ? ESCAPE '\' ORDER BY TABLE_NAME`, schema, t.Pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}
