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
	Log     func(format string, args ...any)
}

// Manifest is written last, as manifest.json, and says what the dump holds.
type Manifest struct {
	Tool       string         `json:"tool"`
	Started    time.Time      `json:"started"`
	Finished   time.Time      `json:"finished"`
	Provenance string         `json:"provenance"`
	Connection Connection     `json:"connection"`
	Schema     string         `json:"schema"`
	Names      []string       `json:"names"`
	Inactive   bool           `json:"inactive"`
	Catalog    bool           `json:"catalog"`
	Tables     []Table        `json:"tables"`
	Counts     map[string]int `json:"counts"`
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
	Error     string `json:"error,omitempty"`
}

// Run writes the dump: src/*.abap, reposrc.jsonl, tables/<NAME>.jsonl and
// manifest.json under opts.Out, which must not exist or be empty.
func Run(ctx context.Context, db *sql.DB, opts Options) (*Manifest, error) {
	if opts.Schema == "" || len(opts.Names) == 0 || opts.Out == "" {
		return nil, errors.New("hanadump: schema, at least one name pattern and an output directory are required")
	}
	if !isIdent(opts.Schema) {
		return nil, fmt.Errorf("hanadump: %q is not a plain schema name", opts.Schema)
	}
	if opts.Tables == nil {
		opts.Tables = DefaultTables
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
	}

	if err := dumpSources(ctx, db, opts, m, logf); err != nil {
		return nil, err
	}
	for _, t := range opts.Tables {
		n, err := dumpTable(ctx, db, opts, t)
		if err != nil {
			// A table the release does not have is a fact about the system,
			// not a reason to lose the sources already written.
			logf("table %s: %v", t.Name, err)
			m.Counts["table_errors"]++
			continue
		}
		m.Counts["rows_"+t.Name] = n
		logf("table %s: %d rows", t.Name, n)
	}

	if opts.Catalog {
		if err := dumpCatalog(ctx, db, opts, m, logf); err != nil {
			return nil, err
		}
	}

	m.Finished = time.Now().UTC()
	b, _ := json.MarshalIndent(m, "", "  ")
	return m, os.WriteFile(filepath.Join(opts.Out, "manifest.json"), append(b, '\n'), 0o644)
}

func dumpSources(ctx context.Context, db *sql.DB, opts Options, m *Manifest, logf func(string, ...any)) error {
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

	if err := os.MkdirAll(filepath.Join(opts.Out, "src"), 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(opts.Out, "reposrc.jsonl"))
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)

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
		lines, derr := DecodeSource(data)
		if derr != nil {
			// Lossless even when undecodable: keep the column as stored, so
			// a better decoder can be run later without the database.
			r.Error = derr.Error()
			r.File = "undecoded/" + FileName(r.Program) + "." + strings.ToLower(r.State) + ".bin"
			if err := os.MkdirAll(filepath.Join(opts.Out, "undecoded"), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(opts.Out, r.File), data, 0o644); err != nil {
				return err
			}
			m.Counts["source_errors"]++
		} else {
			text := strings.Join(lines, "\n") + "\n"
			sum := sha256.Sum256([]byte(text))
			r.Lines, r.SHA256 = len(lines), hex.EncodeToString(sum[:])
			r.File = "src/" + FileName(r.Program)
			if r.State != "A" {
				r.File += "." + strings.ToLower(r.State)
			}
			r.File += ".abap"
			if err := os.WriteFile(filepath.Join(opts.Out, r.File), []byte(text), 0o644); err != nil {
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
func dumpTable(ctx context.Context, db *sql.DB, opts Options, t Table) (int, error) {
	where, args := likeAny(t.Column, opts.Names)
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT * FROM %q.%q WHERE %s`, opts.Schema, t.Name, where), args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Join(opts.Out, "tables"), 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(filepath.Join(opts.Out, "tables", t.Name+".jsonl"))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)

	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	n := 0
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return n, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = jsonValue(vals[i])
		}
		if err := enc.Encode(row); err != nil {
			return n, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return n, err
	}
	return n, w.Flush()
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
		parts[i] = fmt.Sprintf("%q LIKE ?", col)
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

// prepareOut makes the output directory and refuses two mistakes: writing
// over an earlier dump, and writing into a git working tree, from which a
// dump of someone's repository is one `git add .` away from being published.
func prepareOut(out string, allowInRepo bool) error {
	abs, err := filepath.Abs(out)
	if err != nil {
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
		return os.MkdirAll(abs, 0o755)
	case err != nil:
		return err
	case len(entries) > 0:
		return fmt.Errorf("hanadump: %s is not empty", abs)
	}
	return nil
}
