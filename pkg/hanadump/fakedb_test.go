package hanadump

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

// A database/sql driver that answers by the first rule whose fragment the
// statement contains: rows, or an error. Every statement is recorded.
type fakeRule struct {
	contains string
	cols     []string
	rows     [][]driver.Value
	err      error
	// times: answer this many statements with rows, then fail with err
	times int
}

type fakeDB struct {
	mu    sync.Mutex
	rules []*fakeRule
	seen  []string
}

var (
	fakesMu sync.Mutex
	fakes   = map[string]*fakeDB{}
)

func init() { sql.Register("hanadump-fake", fakeDriver{}) }

type fakeDriver struct{}

func (fakeDriver) Open(name string) (driver.Conn, error) {
	fakesMu.Lock()
	defer fakesMu.Unlock()
	return &fakeConn{db: fakes[name]}, nil
}

type fakeConn struct{ db *fakeDB }

func (c *fakeConn) Prepare(q string) (driver.Stmt, error) { return &fakeStmt{c: c, q: q}, nil }
func (c *fakeConn) Close() error                          { return nil }
func (c *fakeConn) Begin() (driver.Tx, error)             { return nil, errors.New("no transactions") }

type fakeStmt struct {
	c *fakeConn
	q string
}

func (s *fakeStmt) Close() error  { return nil }
func (s *fakeStmt) NumInput() int { return -1 }
func (s *fakeStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, errors.New("read only")
}
func (s *fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	db := s.c.db
	db.mu.Lock()
	defer db.mu.Unlock()
	db.seen = append(db.seen, s.q)
	for _, r := range db.rules {
		if !strings.Contains(s.q, r.contains) {
			continue
		}
		if r.times > 0 {
			r.times--
			return &fakeRows{cols: r.cols, rows: r.rows}, nil
		}
		if r.err != nil {
			return nil, r.err
		}
		return &fakeRows{cols: r.cols, rows: r.rows}, nil
	}
	// an unmatched statement reads a table this database does not have
	return nil, fmt.Errorf("259: invalid table name: %s", s.q)
}

type fakeRows struct {
	cols []string
	rows [][]driver.Value
	at   int
}

func (r *fakeRows) Columns() []string { return r.cols }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.at >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.at])
	r.at++
	return nil
}

func openFake(t *testing.T, rules ...*fakeRule) (*sql.DB, *fakeDB) {
	t.Helper()
	f := &fakeDB{rules: rules}
	fakesMu.Lock()
	fakes[t.Name()] = f
	fakesMu.Unlock()
	db, err := sql.Open("hanadump-fake", t.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, f
}

func fakeOpts(t *testing.T) Options {
	out := filepath.Join(t.TempDir(), "dump")
	if err := prepareOut(out, true); err != nil {
		t.Fatal(err)
	}
	return Options{Schema: "S", Out: out}
}

// #M1 (claude-z): a later chunk's failure is a partial dump, never "absent".
func TestLaterChunkFailureIsNotAbsence(t *testing.T) {
	values := make([]string, 501)
	for i := range values {
		values[i] = fmt.Sprintf("F%03d", i)
	}
	db, _ := openFake(t, &fakeRule{contains: `"FUPARAREF"`, cols: []string{"FUNCNAME"}, rows: [][]driver.Value{{"F000"}},
		times: 1, err: errors.New("connection reset by peer")})
	_, missing, err := dumpByValues(context.Background(), db, fakeOpts(t), "FUPARAREF", "FUNCNAME", values)
	if err == nil || missing {
		t.Fatalf("second chunk failed: missing=%v err=%v, want a failure that is not absence", missing, err)
	}
}

func TestOnlyAnAbsentTableIsMissing(t *testing.T) {
	db, _ := openFake(t,
		&fakeRule{contains: `"GONE"`, err: errors.New(`259: invalid table name: Could not find table/view GONE`)},
		&fakeRule{contains: `"FLAKY"`, err: errors.New("connection reset by peer")})
	opts := fakeOpts(t)
	if _, missing, err := dumpTable(context.Background(), db, opts, "GONE", "1 = 1", nil); !missing || err == nil {
		t.Errorf("invalid table name: missing=%v err=%v", missing, err)
	}
	if _, missing, err := dumpTable(context.Background(), db, opts, "FLAKY", "1 = 1", nil); missing || err == nil {
		t.Errorf("a dropped connection is not absence: missing=%v err=%v", missing, err)
	}
}

// The dictionary closure follows .INCLUDE, field data elements, nested
// structures, table types, domains and search helps, and reports what is
// not there and what it does not handle.
func TestCloseDictionary(t *testing.T) {
	db, _ := openFake(t,
		// existence: everything is there except ZSTRU_NEST
		&fakeRule{contains: `SELECT DISTINCT "TABNAME"`, cols: []string{"N"}, rows: [][]driver.Value{{"ZTAB"}, {"ZSTRU_INC"}}},
		&fakeRule{contains: `SELECT DISTINCT "ROLLNAME"`, cols: []string{"N"}, rows: [][]driver.Value{{"ZDTEL1"}, {"ZDTEL_ROW"}, {"ZDTEL_SH"}}},
		&fakeRule{contains: `SELECT DISTINCT "DOMNAME"`, cols: []string{"N"}, rows: [][]driver.Value{{"ZDOMA1"}, {"ZDOMA_RAW"}}},
		&fakeRule{contains: `SELECT DISTINCT "TYPENAME"`, cols: []string{"N"}, rows: [][]driver.Value{{"ZTTYP"}}},
		&fakeRule{contains: `SELECT DISTINCT "SHLPNAME"`, cols: []string{"N"}, rows: [][]driver.Value{{"ZSH_COLL"}, {"ZSH_ELEM"}}},
		&fakeRule{contains: `FROM "S"."DD03L"`, cols: []string{"FIELDNAME", "ROLLNAME", "COMPTYPE", "PRECFIELD", "DOMNAME"}, rows: [][]driver.Value{
			{"FIELD1", "ZDTEL1", "E", "", ""},
			{".INCLUDE", "", "", "ZSTRU_INC", ""},
			{"NESTED", "ZSTRU_NEST", "S", "", ""},
			{"TABLE", "ZTTYP", "L", "", ""},
			{"RAW", "", "", "", "ZDOMA_RAW"},
		}, times: 1},
		&fakeRule{contains: `FROM "S"."DD03L"`, cols: []string{"FIELDNAME", "ROLLNAME", "COMPTYPE", "PRECFIELD", "DOMNAME"}},
		&fakeRule{contains: `"DOMNAME" FROM "S"."DD04L"`, cols: []string{"DOMNAME"}, rows: [][]driver.Value{{"ZDOMA1"}}},
		&fakeRule{contains: `FROM "S"."DD40L" WHERE "AS4LOCAL" = 'A' AND "TYPENAME"`, cols: []string{"ROWTYPE", "ROWKIND"}, rows: [][]driver.Value{{"ZDTEL_ROW", "E"}}},
		&fakeRule{contains: `"DD31S"`, cols: []string{"K", "N"}, rows: [][]driver.Value{{"SHLP", "ZSH_ELEM"}}},
		&fakeRule{contains: `"DD32S"`, cols: []string{"K", "N"}, rows: [][]driver.Value{{"DTEL", "ZDTEL_SH"}}},
	)
	req := &ClosureRequest{Schema: 1, Requested: map[string][]string{"TABL": {"ZTAB"}, "SHLP": {"ZSH_COLL"}, "VIEW": {"ZVIEW"}}}
	res, err := closeDictionary(context.Background(), db, "S", req)
	if err != nil {
		t.Fatal(err)
	}
	for k := range res.Resolved {
		sort.Strings(res.Resolved[k])
	}
	want := map[string][]string{
		"TABL": {"ZSTRU_INC", "ZTAB"}, "DTEL": {"ZDTEL1", "ZDTEL_ROW", "ZDTEL_SH"}, "DOMA": {"ZDOMA1", "ZDOMA_RAW"},
		"TTYP": {"ZTTYP"}, "SHLP": {"ZSH_COLL", "ZSH_ELEM"},
	}
	if !reflect.DeepEqual(res.Resolved, want) {
		t.Errorf("resolved %v, want %v", res.Resolved, want)
	}
	if !reflect.DeepEqual(res.Missing, map[string][]string{"TABL": {"ZSTRU_NEST"}}) {
		t.Errorf("missing %v", res.Missing)
	}
	if !reflect.DeepEqual(res.Unhandled, map[string][]string{"VIEW": {"ZVIEW"}}) {
		t.Errorf("unhandled %v", res.Unhandled)
	}
}

// A raw table SYS.TABLES listed cannot be absent: its failure fails the run,
// and no manifest is written.
func TestRawTableFailureFailsTheRun(t *testing.T) {
	db, _ := openFake(t,
		&fakeRule{contains: `.REPOSRC`, cols: []string{"PROGNAME", "R3STATE", "UNAM", "UDAT", "MAXLINELN", "DATA"}},
		&fakeRule{contains: `SYS.TABLES`, cols: []string{"TABLE_NAME"}, rows: [][]driver.Value{{"ZRAW"}}},
		&fakeRule{contains: `"ZRAW"`, err: errors.New("invalid column name: OBJNAME")},
		&fakeRule{contains: `FROM "S".`, cols: []string{"X"}},
	)
	opts := fakeOpts(t)
	opts.Names = []string{"Z%"}
	opts.RawTables = []RawTable{{Pattern: "ZRAW", Column: "OBJNAME", Values: []string{"Z%"}}}
	if _, err := Run(context.Background(), db, opts); err == nil {
		t.Fatal("a raw table's failure was not an error")
	}
	if _, err := os.Stat(filepath.Join(opts.Out, "manifest.json")); !os.IsNotExist(err) {
		t.Errorf("a failed run wrote a manifest: %v", err)
	}
}

// S3/S8 (claude-z round 1): with --name and --package, TADIR takes both the
// packages' rows and the named objects'; the manifest lists TDEVC.
func TestPackagesAndNamesTogether(t *testing.T) {
	db, f := openFake(t,
		&fakeRule{contains: `SELECT OBJECT, OBJ_NAME, DEVCLASS FROM "S"."TADIR"`, cols: []string{"OBJECT", "OBJ_NAME", "DEVCLASS"},
			rows: [][]driver.Value{{"PROG", "ZPKG_PROG", "ZPKG"}}},
		&fakeRule{contains: `.REPOSRC`, cols: []string{"PROGNAME", "R3STATE", "UNAM", "UDAT", "MAXLINELN", "DATA"}},
		&fakeRule{contains: `FROM "S".`, cols: []string{"X"}},
	)
	opts := fakeOpts(t)
	opts.Names, opts.Packages = []string{"ZOTHER"}, []string{"ZPKG"}
	m, err := Run(context.Background(), db, opts)
	if err != nil {
		t.Fatal(err)
	}
	var tadir string
	for _, q := range f.seen {
		if strings.HasPrefix(q, `SELECT * FROM "S"."TADIR"`) {
			tadir = q
		}
	}
	if !strings.Contains(tadir, `"PGMID" = 'R3TR' AND (("DEVCLASS" LIKE ?`) || !strings.Contains(tadir, `OR ("OBJ_NAME" LIKE ?`) {
		t.Errorf("TADIR is not packages OR names: %s", tadir)
	}
	names := []string{}
	for _, tb := range m.Tables {
		names = append(names, tb.Name)
	}
	if !strings.Contains(strings.Join(names, ","), "TDEVC") {
		t.Errorf("manifest.tables %v lacks TDEVC", names)
	}
}

// A closure's dictionary objects get their TADIR rows, so a converter places
// them in their packages (dell, bed run 10: closure DDIC had no TADIR rows).
func TestClosureNamesGetTheirTadirRows(t *testing.T) {
	db, _ := openFake(t,
		&fakeRule{contains: `SELECT DISTINCT "TABNAME"`, cols: []string{"N"}, rows: [][]driver.Value{{"ZTAB"}}},
		&fakeRule{contains: `SELECT DISTINCT`, cols: []string{"N"}},
		&fakeRule{contains: `FROM "S"."DD03L" WHERE "AS4LOCAL"`, cols: []string{"FIELDNAME", "ROLLNAME", "COMPTYPE", "PRECFIELD", "DOMNAME"}},
		&fakeRule{contains: `"S"."TADIR" WHERE "PGMID" = 'R3TR' AND "OBJECT" IN`, cols: []string{"PGMID", "OBJECT", "OBJ_NAME", "DEVCLASS"},
			rows: [][]driver.Value{{"R3TR", "TABL", "ZTAB", "ZSTD"}}},
		&fakeRule{contains: `FROM "S".`, cols: []string{"X"}},
	)
	opts := fakeOpts(t)
	opts.Closure = &ClosureRequest{Schema: 1, Requested: map[string][]string{"TABL": {"ZTAB"}}}
	m, err := Run(context.Background(), db, opts)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(opts.Out, "tables", "TADIR.jsonl"))
	if err != nil || !strings.Contains(string(b), `"DEVCLASS":"ZSTD"`) {
		t.Fatalf("TADIR rows of the closure: %q %v", b, err)
	}
	if m.Counts["rows_TADIR"] != 1 {
		t.Errorf("rows_TADIR = %d", m.Counts["rows_TADIR"])
	}
}

// --skip-test-includes: no CCAU source and no where-used row of one.
func TestSkipTestIncludes(t *testing.T) {
	db, f := openFake(t,
		&fakeRule{contains: `.REPOSRC`, cols: []string{"PROGNAME", "R3STATE", "UNAM", "UDAT", "MAXLINELN", "DATA"}},
		&fakeRule{contains: `FROM "S".`, cols: []string{"X"}},
	)
	opts := fakeOpts(t)
	opts.Names, opts.SkipTestIncludes = []string{"Z%"}, true
	if _, err := Run(context.Background(), db, opts); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"REPOSRC": false, `"WBCROSSGT"`: false, `"CROSS"`: false}
	for _, q := range f.seen {
		for k := range want {
			if strings.Contains(q, k) && strings.Contains(q, `SUBSTRING(`) && strings.Contains(q, `'CCAU'`) {
				want[k] = true
			}
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("%s is not filtered for test includes", k)
		}
	}
}
