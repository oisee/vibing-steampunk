package hanadump

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Dumps are private: directories and files are readable by their owner only,
// whatever the umask.
const (
	dirMode  = 0o700
	fileMode = 0o600
)

// Table is one repository table dumped beside the sources: its rows whose
// Column matches the name patterns.
type Table struct {
	Name   string `json:"name"`
	Column string `json:"column"`
}

// DefaultTables are the tables a converter needs to put sources back into
// objects — the object directory, program attributes, class method includes,
// function modules, classes and function groups — and to order them by what
// refers to what.
var DefaultTables = []Table{
	{"TADIR", "OBJ_NAME"},
	{"TRDIR", "NAME"},
	{"TMDIR", "CLASSNAME"},
	{"TFDIR", "FUNCNAME"},
	{"SEOCLASS", "CLSNAME"},
	// a class's attributes (category: exception, ...; unit tests; message
	// class) and its description, which abapGit's .clas.xml carries
	{"SEOCLASSDF", "CLSNAME"},
	{"SEOCLASSTX", "CLSNAME"},
	{"TLIBG", "AREA"},
	// SAP's own where-used index, by the including program: what each
	// source refers to. Dependencies come from here rather than a parser.
	{"WBCROSSGT", "INCLUDE"},
	{"WBCROSSI", "INCLUDE"},
	{"CROSS", "INCLUDE"},
}

// Connection describes where the dump was read from, for the manifest. It
// never holds a password.
type Connection struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Database string `json:"database,omitempty"`
}

// Options select what is dumped and where it goes.
type Options struct {
	Schema string
	// Names are SQL LIKE patterns ('%' any run, '_' one character), matched
	// against REPOSRC.PROGNAME and each table's Column. The caller decides
	// what is theirs; there is no built-in notion of it.
	Names    []string
	Inactive bool // also dump inactive versions (R3STATE 'I')
	Tables   []Table
	Out      string
	// Provenance is the caller's own words on where this content comes from
	// and under what rights — "A4H trial, backup 110 2026-10-15", say. It is
	// written into the manifest unchanged.
	Provenance  string
	Connection  Connection
	AllowInRepo bool // permit Out inside a git working tree
	// BlobInto returns what to pass to Scan for a BLOB column so that the
	// bytes land in *dst. Drivers that hand out LOB locators (go-hdb does)
	// need their own scanner; nil means scanning into *dst directly.
	BlobInto func(dst *[]byte) any
	// TextInto is BlobInto for character LOBs (NCLOB definitions).
	TextInto func(dst *string) any
	// Catalog also dumps the definitions of database procedures and
	// functions matching Names (AMDP methods are "<CLASS>=><METHOD>") and,
	// transitively, the views and tables they read — under catalog/.
	Catalog bool
	// Packages are LIKE patterns on TADIR's DEVCLASS: their objects' sources
	// and rows are dumped as if their names had been given (with LIKE's
	// wildcards escaped), and TADIR and TDEVC rows are taken by package.
	Packages []string
	// DDIC also dumps DDICTables for the selected names.
	DDIC bool
	// RawTables are dumped as asked, whatever the names select.
	RawTables []RawTable
	// Closure is a dependency-closure request: its source kinds are selected
	// as if named, its dictionary kinds are closed over on the server and
	// their DDIC rows dumped (DDIC is implied).
	Closure *ClosureRequest
	Log     func(format string, args ...any)
}

// Manifest is written last, as manifest.json, and says what the dump holds.
type Manifest struct {
	Tool       string     `json:"tool"`
	Started    time.Time  `json:"started"`
	Finished   time.Time  `json:"finished"`
	Provenance string     `json:"provenance"`
	Connection Connection `json:"connection"`
	Schema     string     `json:"schema"`
	Names      []string   `json:"names"`
	Inactive   bool       `json:"inactive"`
	Catalog    bool       `json:"catalog"`
	Tables     []Table    `json:"tables"`
	Packages   []string   `json:"packages,omitempty"`
	// PackageNames are the name patterns the packages resolved to.
	PackageNames []string       `json:"package_names,omitempty"`
	DDIC         bool           `json:"ddic,omitempty"`
	RawTables    []RawTable     `json:"raw_tables,omitempty"`
	Closure      *ClosureResult `json:"closure,omitempty"`
	Counts       map[string]int `json:"counts"`
	// MissingTables are tables the database refused to select from — not in
	// this release, or not readable by the user. A table that failed part way
	// through is not here: that fails the dump, and no manifest is written.
	MissingTables []string `json:"missing_tables,omitempty"`
}

// SourceRecord is one line of reposrc.jsonl.
type SourceRecord struct {
	Program   string `json:"program"`
	State     string `json:"state"`
	ChangedBy string `json:"changed_by,omitempty"`
	ChangedOn string `json:"changed_on,omitempty"`
	MaxLine   int    `json:"max_line"`
	Lines     int    `json:"lines"`
	SHA256    string `json:"sha256,omitempty"`
	File      string `json:"file,omitempty"`
	// Empty marks a row whose DATA is NULL or empty: no file is written,
	// because a file would show a line the database does not hold.
	Empty bool   `json:"empty,omitempty"`
	Error string `json:"error,omitempty"`
}

// Run writes the dump: src/*.abap, reposrc.jsonl, tables/<NAME>.jsonl and
// manifest.json under opts.Out, which must not exist or be empty.
func Run(ctx context.Context, db *sql.DB, opts Options) (*Manifest, error) {
	if opts.Schema == "" || len(opts.Names)+len(opts.Packages) == 0 && opts.Closure == nil || opts.Out == "" {
		return nil, errors.New("hanadump: schema, a name or package pattern or a closure request, and an output directory are required")
	}
	if opts.Closure != nil {
		opts.DDIC = true
	}
	if !isIdent(opts.Schema) {
		return nil, fmt.Errorf("hanadump: %q is not a plain schema name", opts.Schema)
	}
	if opts.Tables == nil {
		opts.Tables = append([]Table{}, DefaultTables...)
		if opts.DDIC {
			opts.Tables = append(opts.Tables, DDICTables...)
		}
	}
	for _, r := range opts.RawTables {
		if !isPattern(r.Pattern) || r.Column != "" && (!isIdent(r.Column) || len(r.Values) == 0) {
			return nil, fmt.Errorf("hanadump: raw table %+v: not a table pattern with an optional COLUMN=PATTERN filter", r)
		}
	}
	for _, t := range opts.Tables {
		if !isIdent(t.Name) || !isIdent(t.Column) {
			return nil, fmt.Errorf("hanadump: %q.%q is not a plain table and column name", t.Name, t.Column)
		}
	}
	logf := opts.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if err := prepareOut(opts.Out, opts.AllowInRepo); err != nil {
		return nil, err
	}

	m := &Manifest{
		Tool: "vsp hana dump", Started: time.Now().UTC(), Provenance: opts.Provenance,
		Connection: opts.Connection, Schema: opts.Schema, Names: opts.Names,
		Inactive: opts.Inactive, Catalog: opts.Catalog, Tables: opts.Tables, Counts: map[string]int{},
		Packages: opts.Packages, DDIC: opts.DDIC, RawTables: opts.RawTables,
	}
	// the names the caller gave, before packages and a closure add theirs
	userNames := append([]string{}, opts.Names...)
	// the dictionary names a closure request reaches, by DDIC table column
	closureNames := map[string][]string{}
	if opts.Closure != nil {
		res, err := closeDictionary(ctx, db, opts.Schema, opts.Closure)
		if err != nil {
			return nil, err
		}
		m.Closure = res
		for kind, names := range res.Resolved {
			m.Counts["closure_"+kind] = len(names)
		}
		logf("closure: %d rounds, %v resolved, %d kinds with missing names", res.Rounds, m.Counts, len(res.Missing))
		byColumn := map[string]string{"TABL": "TABNAME", "DTEL": "ROLLNAME", "DOMA": "DOMNAME", "TTYP": "TYPENAME", "SHLP": "SHLPNAME"}
		for kind, names := range res.Resolved {
			closureNames[byColumn[kind]] = append(closureNames[byColumn[kind]], names...)
		}
		opts.Names = append(append([]string{}, opts.Names...), opts.Closure.SourcePatternsOf()...)
	}
	if len(opts.Packages) > 0 {
		objects, patterns, err := resolvePackages(ctx, db, opts.Schema, opts.Packages)
		if err != nil {
			return nil, err
		}
		if len(objects) == 0 && len(opts.Names) == 0 {
			return nil, fmt.Errorf("hanadump: no TADIR objects in packages %v", opts.Packages)
		}
		m.PackageNames, m.Counts["package_objects"] = patterns, len(objects)
		logf("packages %v: %d objects, %d name patterns", opts.Packages, len(objects), len(patterns))
		opts.Names = append(append([]string{}, opts.Names...), patterns...)
	}

	if err := dumpSources(ctx, db, opts, m, logf); err != nil {
		return nil, err
	}
	tables := opts.Tables
	if len(opts.Packages) > 0 {
		tables = append(append([]Table{}, tables...), Table{"TDEVC", "DEVCLASS"})
		m.Tables = tables
	}
	// a table is dumped once: two raw patterns may name it, or name one of
	// the tables above
	dumped := map[string]bool{}
	for _, t := range tables {
		where, args := likeAny(t.Column, opts.Names)
		if t.Name == "TFDIR" && len(opts.Names) > 0 {
			// a function module by its own name, or by its group's main
			// program (a group named, or a package's group)
			pw, pa := likeAny("PNAME", opts.Names)
			where, args = "("+where+" OR "+pw+")", append(append([]any{}, args...), pa...)
		}
		if len(opts.Packages) > 0 {
			switch t.Name {
			case "TADIR", "TDEVC":
				// by package, not by name: a padded class name's pattern
				// also matches objects of other packages
				where, args = likeAny("DEVCLASS", opts.Packages)
				if t.Name == "TADIR" {
					// and the objects named beside the packages
					if len(userNames) > 0 {
						nw, na := likeAny("OBJ_NAME", userNames)
						where, args = "("+where+" OR "+nw+")", append(append([]any{}, args...), na...)
					}
					where = `"PGMID" = 'R3TR' AND ` + where
				}
			}
		}
		var n int
		var missing bool
		var err error
		if len(opts.Names) > 0 {
			n, missing, err = dumpTable(ctx, db, opts, t.Name, where, args)
		}
		// closure names, by IN list, leaving out the rows the patterns took
		if extra := closureNames[t.Column]; !missing && err == nil && len(extra) > 0 && isDDIC(t.Name) {
			appended := n > 0 || len(opts.Names) > 0
			err = inChunks(extra, func(ph string, inArgs []any) error {
				w, a := fmt.Sprintf(`%q IN (%s)`, t.Column, ph), inArgs
				if len(opts.Names) > 0 {
					w, a = w+" AND NOT "+where, append(append([]any{}, inArgs...), args...)
				}
				k, miss, e := dumpTableTo(ctx, db, opts, t.Name, w, a, appended)
				appended, n = true, n+k
				if miss {
					missing = true
				}
				return e
			})
		}
		if missing {
			// A table the release does not have is a fact about the system,
			// not a reason to lose the sources already written.
			logf("table %s: not selectable: %v", t.Name, err)
			m.MissingTables = append(m.MissingTables, t.Name)
			continue
		}
		if err != nil {
			// Part way through: the file is incomplete. Fail, and leave no
			// manifest, so the dump cannot be mistaken for a finished one.
			return nil, fmt.Errorf("hanadump: table %s: %w", t.Name, err)
		}
		if len(opts.Names) > 0 || len(closureNames[t.Column]) > 0 && isDDIC(t.Name) {
			dumped[t.Name] = true // a file was written
		}
		m.Counts["rows_"+t.Name] = n
		logf("table %s: %d rows", t.Name, n)
	}
	// function module signatures and short texts, by the modules TFDIR gave
	if funcs, err := columnValues(opts.Out, "TFDIR", "FUNCNAME"); err != nil {
		return nil, err
	} else if len(funcs) > 0 {
		for _, t := range []Table{{"FUPARAREF", "FUNCNAME"}, {"TFTIT", "FUNCNAME"}} {
			n, missing, err := dumpByValues(ctx, db, opts, t.Name, t.Column, funcs)
			if missing {
				logf("table %s: not selectable: %v", t.Name, err)
				m.MissingTables = append(m.MissingTables, t.Name)
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("hanadump: table %s: %w", t.Name, err)
			}
			dumped[t.Name] = true
			m.Counts["rows_"+t.Name] = n
			m.Tables = append(m.Tables, t)
			logf("table %s: %d rows for %d function modules", t.Name, n, len(funcs))
		}
	}
	for _, r := range opts.RawTables {
		names, err := expandRawTable(ctx, db, opts.Schema, r)
		if err != nil {
			return nil, fmt.Errorf("hanadump: raw table %s: %w", r.Pattern, err)
		}
		if len(names) == 0 {
			logf("raw table %s: no such table", r.Pattern)
			m.MissingTables = append(m.MissingTables, r.Pattern)
			continue
		}
		where, args := "1 = 1", []any(nil)
		if r.Column != "" {
			where, args = likeAny(r.Column, r.Values)
		}
		for _, name := range names {
			if dumped[name] {
				logf("raw table %s: already dumped, skipped", name)
				continue
			}
			dumped[name] = true
			n, _, err := dumpTable(ctx, db, opts, name, where, args)
			// SYS.TABLES listed it: a failure here is never absence
			if err != nil {
				return nil, fmt.Errorf("hanadump: table %s: %w", name, err)
			}
			m.Counts["rows_"+name] = n
			logf("raw table %s: %d rows", name, n)
		}
	}

	if opts.Catalog && len(opts.Names) == 0 {
		logf("catalog: no names selected, nothing to dump")
	} else if opts.Catalog {
		if err := dumpCatalog(ctx, db, opts, m, logf); err != nil {
			return nil, err
		}
	}

	m.Finished = time.Now().UTC()
	b, _ := json.MarshalIndent(m, "", "  ")
	return m, os.WriteFile(filepath.Join(opts.Out, "manifest.json"), append(b, '\n'), fileMode)
}

func dumpSources(ctx context.Context, db *sql.DB, opts Options, m *Manifest, logf func(string, ...any)) error {
	if len(opts.Names) == 0 {
		// a dictionary-only closure: no program is asked for, and the
		// converter still finds the (empty) source list
		f, err := os.OpenFile(filepath.Join(opts.Out, "reposrc.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
		if err != nil {
			return err
		}
		return f.Close()
	}
	where, args := likeAny("PROGNAME", opts.Names)
	if !opts.Inactive {
		where += " AND R3STATE = 'A'"
	}
	q := fmt.Sprintf(`SELECT PROGNAME, R3STATE, UNAM, UDAT, MAXLINELN, DATA FROM %q.REPOSRC WHERE %s ORDER BY PROGNAME, R3STATE`,
		opts.Schema, where)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("hanadump: REPOSRC: %w", err)
	}
	defer rows.Close()

	if err := os.MkdirAll(filepath.Join(opts.Out, "src"), dirMode); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(opts.Out, "reposrc.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	used := map[string]bool{}

	for rows.Next() {
		var (
			r       SourceRecord
			by, on  sql.NullString
			maxLine sql.NullInt64
			data    []byte
		)
		var blob any = &data
		if opts.BlobInto != nil {
			blob = opts.BlobInto(&data)
		}
		if err := rows.Scan(&r.Program, &r.State, &by, &on, &maxLine, blob); err != nil {
			return fmt.Errorf("hanadump: REPOSRC row: %w", err)
		}
		r.ChangedBy, r.ChangedOn, r.MaxLine = by.String, on.String, int(maxLine.Int64)
		if len(data) == 0 {
			r.Empty = true
			m.Counts["sources_empty"]++
			if err := enc.Encode(r); err != nil {
				return err
			}
			continue
		}
		lines, derr := DecodeSource(data)
		if derr != nil {
			// Lossless even when undecodable: keep the column as stored, so
			// a better decoder can be run later without the database.
			r.Error = derr.Error()
			r.File = uniquePath(used, "undecoded/"+FileName(r.Program)+"."+strings.ToLower(r.State), ".bin", r.Program, r.State)
			if err := os.MkdirAll(filepath.Join(opts.Out, "undecoded"), dirMode); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(opts.Out, r.File), data, fileMode); err != nil {
				return err
			}
			m.Counts["source_errors"]++
		} else {
			text := strings.Join(lines, "\n") + "\n"
			sum := sha256.Sum256([]byte(text))
			r.Lines, r.SHA256 = len(lines), hex.EncodeToString(sum[:])
			base := "src/" + FileName(r.Program)
			if r.State != "A" {
				base += "." + strings.ToLower(r.State)
			}
			r.File = uniquePath(used, base, ".abap", r.Program, r.State)
			if err := os.WriteFile(filepath.Join(opts.Out, r.File), []byte(text), fileMode); err != nil {
				return err
			}
			m.Counts["sources"]++
			if m.Counts["sources"]%1000 == 0 {
				logf("sources: %d", m.Counts["sources"])
			}
		}
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("hanadump: REPOSRC: %w", err)
	}
	logf("sources: %d written, %d failed to decode", m.Counts["sources"], m.Counts["source_errors"])
	return w.Flush()
}

// dumpTable writes every column of the matching rows as JSON objects. Binary
// values become "0x" followed by hex, so a row survives the round trip.
func dumpTable(ctx context.Context, db *sql.DB, opts Options, table, where string, args []any) (n int, missing bool, err error) {
	return dumpTableTo(ctx, db, opts, table, where, args, false)
}

// tableAbsent says whether a query failed because the table is not there or
// not readable -- HANA's "invalid table name" (259) and "insufficient
// privilege" (258) -- rather than for any other reason.
func tableAbsent(err error) bool {
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "invalid table name") || strings.Contains(m, "insufficient privilege")
}

// isDDIC says whether a table is one of DDICTables.
func isDDIC(name string) bool {
	for _, t := range DDICTables {
		if t.Name == name {
			return true
		}
	}
	return false
}

// dumpTableTo is dumpTable; with appendTo it adds to the table's file rather
// than creating it.
func dumpTableTo(ctx context.Context, db *sql.DB, opts Options, table, where string, args []any, appendTo bool) (n int, missing bool, err error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT * FROM %q.%q WHERE %s`, opts.Schema, table, where), args...)
	if err != nil {
		// absent only when the database says so, and only on a table's first
		// query: a later chunk's failure, or a dropped connection, is a
		// partial dump, which fails the run
		return 0, !appendTo && tableAbsent(err), err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return 0, false, err
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return 0, false, err
	}
	// Character columns may arrive as bytes (go-hdb hands VARCHAR over that
	// way); they are text and are written as text. Only binary types stay hex.
	text := make([]bool, len(cols))
	// LOB columns arrive as locators only the driver can read (go-hdb):
	// they are scanned through opts.TextInto / opts.BlobInto.
	textLob, blobLob := make([]string, len(cols)), make([][]byte, len(cols))
	isText, isBlob := make([]bool, len(cols)), make([]bool, len(cols))
	for i, t := range types {
		switch t.DatabaseTypeName() {
		case "VARCHAR", "NVARCHAR", "CHAR", "NCHAR", "ALPHANUM", "SHORTTEXT", "STRING", "NSTRING":
			text[i] = true
		case "CLOB", "NCLOB", "TEXT", "BINTEXT":
			isText[i] = opts.TextInto != nil
			text[i] = true
		case "BLOB":
			isBlob[i] = opts.BlobInto != nil
		}
	}
	if err := os.MkdirAll(filepath.Join(opts.Out, "tables"), dirMode); err != nil {
		return 0, false, err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if appendTo {
		flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	}
	f, err := os.OpenFile(filepath.Join(opts.Out, "tables", FileName(table)+".jsonl"), flags, fileMode)
	if err != nil {
		return 0, false, err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)

	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		switch {
		case isText[i]:
			ptrs[i] = opts.TextInto(&textLob[i])
		case isBlob[i]:
			ptrs[i] = opts.BlobInto(&blobLob[i])
		default:
			ptrs[i] = &vals[i]
		}
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return n, false, err
		}
		for i := range cols {
			if isText[i] {
				vals[i] = textLob[i]
			} else if isBlob[i] {
				vals[i] = nil // SQL NULL
				if blobLob[i] != nil {
					vals[i] = append([]byte{}, blobLob[i]...)
				}
			}
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok && text[i] {
				row[c] = string(b)
				continue
			}
			row[c] = jsonValue(vals[i])
		}
		if err := enc.Encode(row); err != nil {
			return n, false, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return n, false, err
	}
	return n, false, w.Flush()
}

func jsonValue(v any) any {
	switch x := v.(type) {
	case []byte:
		return "0x" + hex.EncodeToString(x)
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano)
	default:
		return x
	}
}

func likeAny(col string, patterns []string) (string, []any) {
	parts := make([]string, len(patterns))
	args := make([]any, len(patterns))
	for i, p := range patterns {
		parts[i] = fmt.Sprintf(`%q LIKE ? ESCAPE '\'`, col)
		args[i] = p
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}

// isIdent admits the names SAP gives schemas, tables and columns, and keeps
// anything that would need quoting out of the statements built here.
func isIdent(s string) bool {
	if s == "" || len(s) > 127 {
		return false
	}
	for _, r := range s {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '/') {
			return false
		}
	}
	return true
}

// uniquePath returns base+ext, or — when another row already took that path,
// as `Z/A` and `Z#A` would — base, a dot, eight hex digits of the program
// name's hash, and ext. No source overwrites another.
func uniquePath(used map[string]bool, base, ext, program, state string) string {
	p := base + ext
	if used[p] {
		sum := sha256.Sum256([]byte(program + "\x00" + state))
		p = base + "." + hex.EncodeToString(sum[:4]) + ext
	}
	used[p] = true
	return p
}

// resolved follows symlinks on the part of path that exists, so the git
// check below looks at where the dump will really land.
func resolved(path string) (string, error) {
	rest := ""
	for d := path; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(d); err == nil {
			real, err := filepath.EvalSymlinks(d)
			if err != nil {
				return "", err
			}
			return filepath.Join(real, rest), nil
		}
		if filepath.Dir(d) == d {
			return path, nil
		}
		rest = filepath.Join(filepath.Base(d), rest)
	}
}

// prepareOut makes the output directory and refuses two mistakes: writing
// over an earlier dump, and writing into a git working tree, from which a
// dump of someone's repository is one `git add .` away from being published.
func prepareOut(out string, allowInRepo bool) error {
	abs, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	if abs, err = resolved(abs); err != nil {
		return err
	}
	if !allowInRepo {
		for d := abs; ; d = filepath.Dir(d) {
			if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
				return fmt.Errorf("hanadump: %s is inside the git working tree at %s; dumps stay out of repositories (pass AllowInRepo to override)", abs, d)
			}
			if filepath.Dir(d) == d {
				break
			}
		}
	}
	entries, err := os.ReadDir(abs)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return os.MkdirAll(abs, dirMode)
	case err != nil:
		return err
	case len(entries) > 0:
		return fmt.Errorf("hanadump: %s is not empty", abs)
	}
	return nil
}
