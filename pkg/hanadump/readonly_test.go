package hanadump

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestCheckReadOnly(t *testing.T) {
	ok := []string{
		`SELECT 1 FROM DUMMY`,
		` with x as (select 1 a from dummy) select a from x ;  `,
		`SELECT "UPDATE", 'DELETE FROM T' FROM "S"."T" -- drop table t` + "\n",
		`/* insert */ SELECT 'it''s; drop' FROM DUMMY`,
		`SELECT REPLACE(NAME,'a','b') FROM T`,
		`(SELECT 1 FROM DUMMY)`,
	}
	for _, q := range ok {
		if err := CheckReadOnly(q); err != nil {
			t.Errorf("refused %q: %v", q, err)
		}
	}
	bad := []string{
		`INSERT INTO T VALUES (1)`, `update t set a=1`, `DELETE FROM T`, `UPSERT T VALUES (1)`,
		`MERGE INTO T USING U ON 1=1`, `CREATE TABLE T (A INT)`, `DROP TABLE T`, `ALTER USER U`,
		`TRUNCATE TABLE T`, `GRANT SELECT ON T TO U`, `CALL P()`, `SET SCHEMA X`,
		`SELECT 1 FROM DUMMY; DELETE FROM T`, `SELECT * FROM T FOR UPDATE`,
		`SELECT 1 FROM DUMMY /* unterminated`, `SELECT 1 FROM DUMMY COMMIT`, `WITH X AS (SELECT 1 A FROM DUMMY) SELECT A FROM X ROLLBACK`, `SELECT 'x FROM DUMMY`, ``, `  -- only a comment`,
	}
	for _, q := range bad {
		if err := CheckReadOnly(q); !errors.Is(err, ErrNotReadOnly) {
			t.Errorf("let through %q (%v)", q, err)
		}
	}
}

// recDriver records every statement that reaches the driver.
type recDriver struct{ seen []string }

func (d *recDriver) Open(string) (driver.Conn, error)             { return &recConn{d}, nil }
func (d *recDriver) Connect(context.Context) (driver.Conn, error) { return &recConn{d}, nil }
func (d *recDriver) Driver() driver.Driver                        { return d }

type recConn struct{ d *recDriver }

func (c *recConn) Prepare(q string) (driver.Stmt, error) {
	c.d.seen = append(c.d.seen, "prepare:"+q)
	return recStmt{}, nil
}
func (c *recConn) Close() error { return nil }
func (c *recConn) Begin() (driver.Tx, error) {
	c.d.seen = append(c.d.seen, "begin")
	return recTx{}, nil
}
func (c *recConn) ExecContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Result, error) {
	c.d.seen = append(c.d.seen, "exec:"+q)
	return driver.RowsAffected(0), nil
}
func (c *recConn) QueryContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
	c.d.seen = append(c.d.seen, "query:"+q)
	return recRows{}, nil
}

type recStmt struct{}

func (recStmt) Close() error                               { return nil }
func (recStmt) NumInput() int                              { return -1 }
func (recStmt) Exec([]driver.Value) (driver.Result, error) { return driver.RowsAffected(0), nil }
func (recStmt) Query([]driver.Value) (driver.Rows, error)  { return recRows{}, nil }

type recRows struct{}

func (recRows) Columns() []string         { return []string{"A"} }
func (recRows) Close() error              { return nil }
func (recRows) Next([]driver.Value) error { return io.EOF }

type recTx struct{}

func (recTx) Commit() error   { return nil }
func (recTx) Rollback() error { return nil }

// A non-SELECT never reaches the driver, by any database/sql path; the only
// non-query the driver sees is the session's own read-only switch.
func TestReadOnlyConnectorNothingButSelectLeaves(t *testing.T) {
	rec := &recDriver{}
	db := sql.OpenDB(ReadOnlyConnector(rec))
	defer db.Close()
	ctx := context.Background()
	writes := []string{`INSERT INTO T VALUES (1)`, `DELETE FROM T`, `CALL P()`, `CREATE TABLE T (A INT)`, `GRANT SELECT ON T TO U`, `SELECT 1 FROM DUMMY; DROP TABLE T`}
	for _, w := range writes {
		if _, err := db.ExecContext(ctx, w); err == nil {
			t.Errorf("Exec %q succeeded", w)
		}
		if _, err := db.QueryContext(ctx, w); err == nil {
			t.Errorf("Query %q succeeded", w)
		}
		if _, err := db.PrepareContext(ctx, w); err == nil {
			t.Errorf("Prepare %q succeeded", w)
		}
	}
	// A SELECT goes through Exec too, and must still not leave as an exec.
	if _, err := db.ExecContext(ctx, `SELECT 1 FROM DUMMY`); err == nil {
		t.Errorf("Exec of a SELECT succeeded")
	}
	if _, err := db.BeginTx(ctx, nil); err == nil {
		t.Errorf("read-write transaction began")
	}
	rows, err := db.QueryContext(ctx, `SELECT 1 FROM DUMMY`)
	if err != nil {
		t.Fatal(err)
	}
	rows.Close()
	for _, s := range rec.seen {
		if s == "exec:"+sessionReadOnly {
			continue
		}
		if !strings.HasPrefix(s, "query:SELECT") && !strings.HasPrefix(s, "prepare:SELECT") {
			t.Errorf("reached the driver: %q", s)
		}
	}
	if len(rec.seen) == 0 || rec.seen[0] != "exec:"+sessionReadOnly {
		t.Errorf("session not set read-only first: %v", rec.seen)
	}
}
