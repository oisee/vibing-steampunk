package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"testing"
)

// paramDriver answers TADIR with one PROG per string parameter, and every
// other query with no rows; it fails a statement with more than maxParams
// parameters, standing in for HANA's own limit (SQL error 589).
type paramDriver struct{ maxSeen *int }

func (d paramDriver) Open(string) (driver.Conn, error) { return paramConn(d), nil }

type paramConn paramDriver

func (c paramConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c paramConn) Close() error                        { return nil }
func (c paramConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (c paramConn) QueryContext(_ context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	if len(args) > *c.maxSeen {
		*c.maxSeen = len(args)
	}
	if len(args) > maxParams {
		return nil, errString("SQL error 589: too many parameters are set")
	}
	r := &paramRows{}
	if strings.Contains(q, `"TADIR"`) {
		for _, a := range args {
			if s, ok := a.Value.(string); ok {
				r.names = append(r.names, s)
			}
		}
	}
	return r, nil
}

type paramRows struct {
	names []string
	at    int
}

func (r *paramRows) Columns() []string {
	return []string{"OBJECT", "OBJ_NAME", "DEVCLASS", "AUTHOR", "SRCSYSTEM", "GENFLAG"}
}
func (r *paramRows) Close() error { return nil }
func (r *paramRows) Next(dest []driver.Value) error {
	if r.at >= len(r.names) {
		return io.EOF
	}
	dest[0], dest[1], dest[2], dest[3], dest[4], dest[5] = "PROG", r.names[r.at], "$P", "U", "A4H", ""
	r.at++
	return nil
}

func TestScopeChunksParameters(t *testing.T) {
	var maxSeen int
	sql.Register("osd-param-limit", paramDriver{&maxSeen})
	db, err := sql.Open("osd-param-limit", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var names []string
	for i := 0; i < 2*maxParams+7; i++ {
		names = append(names, "ZP"+strings.Repeat("X", i%3)+string(rune('A'+i%26))+strings.Repeat("0", i/26%5)+"_"+itoa(i))
	}
	// the same names again as LIKE patterns: an object two groups select is listed once
	req := &Request{Schema: 1, Requested: map[string][]string{"PROG": names}, Names: names[:maxParams+1]}
	objs, err := scope(context.Background(), db, "S", req)
	if err != nil {
		t.Fatal(err)
	}
	if maxSeen > maxParams {
		t.Errorf("a statement carried %d parameters", maxSeen)
	}
	if len(objs) != len(names) {
		t.Errorf("%d objects, want %d", len(objs), len(names))
	}
	for i := 1; i < len(objs); i++ {
		if objs[i-1].Name >= objs[i].Name {
			t.Fatalf("not sorted or duplicated at %d: %s, %s", i, objs[i-1].Name, objs[i].Name)
		}
	}
}

func itoa(i int) string {
	s := ""
	for {
		s = string(rune('0'+i%10)) + s
		if i /= 10; i == 0 {
			return s
		}
	}
}
