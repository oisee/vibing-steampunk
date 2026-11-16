package hanadump

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// ErrNotReadOnly is returned for any statement the read-only gate refuses.
var ErrNotReadOnly = errors.New("hanadump: refused, not a read-only statement")

// sessionReadOnly is the one statement the gate lets through that is not a
// query: it makes the HANA session itself refuse any write, a second lock
// behind the gate. HANA keeps it for the session, across autocommit.
const sessionReadOnly = "SET TRANSACTION READ ONLY"

// CheckReadOnly accepts a single SELECT (or WITH ... SELECT) statement and
// nothing else: no DML, DDL, CALL, GRANT, no second statement after a ';',
// no SELECT ... FOR UPDATE (it takes row locks). Comments are skipped;
// string literals and quoted identifiers are not looked into.
func CheckReadOnly(query string) error {
	code, err := codeOnly(query)
	if err != nil {
		return err
	}
	words := strings.FieldsFunc(strings.ToUpper(code), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '#' || r == '$')
	})
	if len(words) == 0 {
		return fmt.Errorf("%w: empty statement", ErrNotReadOnly)
	}
	if words[0] != "SELECT" && words[0] != "WITH" {
		return fmt.Errorf("%w: starts with %s", ErrNotReadOnly, words[0])
	}
	// HANA allows no data change inside a query; this is a second look, at
	// keywords outside literals and quoted names. It also catches SELECT ...
	// FOR UPDATE, which takes row locks.
	for _, w := range words {
		switch w {
		case "INSERT", "UPDATE", "DELETE", "UPSERT", "MERGE", "CREATE", "DROP", "ALTER",
			"TRUNCATE", "GRANT", "REVOKE", "CALL", "EXEC", "EXECUTE", "COMMIT", "ROLLBACK":
			return fmt.Errorf("%w: contains %s", ErrNotReadOnly, w)
		}
	}
	return nil
}

// codeOnly blanks comments, string literals and quoted identifiers, and
// refuses a ';' followed by anything but blanks.
func codeOnly(q string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(q); i++ {
		c := q[i]
		switch {
		case c == '-' && i+1 < len(q) && q[i+1] == '-':
			for i < len(q) && q[i] != '\n' {
				i++
			}
			b.WriteByte(' ')
		case c == '/' && i+1 < len(q) && q[i+1] == '*':
			end := strings.Index(q[i+2:], "*/")
			if end < 0 {
				return "", fmt.Errorf("%w: unterminated comment", ErrNotReadOnly)
			}
			i += end + 3
			b.WriteByte(' ')
		case c == '\'' || c == '"':
			j := i + 1
			for ; j < len(q); j++ {
				if q[j] == c {
					if j+1 < len(q) && q[j+1] == c { // doubled quote
						j++
						continue
					}
					break
				}
			}
			if j >= len(q) {
				return "", fmt.Errorf("%w: unterminated quote", ErrNotReadOnly)
			}
			i = j
			b.WriteString(" x ")
		case c == ';':
			if strings.TrimSpace(q[i+1:]) != "" {
				return "", fmt.Errorf("%w: more than one statement", ErrNotReadOnly)
			}
			i = len(q)
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}

// ReadOnlyConnector wraps a driver.Connector: every connection it opens is
// set read-only on the database side first, and every statement passes
// CheckReadOnly before it reaches the driver. Exec is refused outright.
func ReadOnlyConnector(base driver.Connector) driver.Connector {
	return &roConnector{base: base}
}

type roConnector struct{ base driver.Connector }

func (c *roConnector) Driver() driver.Driver { return c.base.Driver() }

func (c *roConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	ex, ok := conn.(driver.ExecerContext)
	if !ok {
		conn.Close()
		return nil, errors.New("hanadump: driver cannot set the session read-only")
	}
	if _, err := ex.ExecContext(ctx, sessionReadOnly, nil); err != nil {
		conn.Close()
		return nil, fmt.Errorf("hanadump: %s: %w", sessionReadOnly, err)
	}
	return &roConn{base: conn}, nil
}

// roConn forwards only what a reader needs. It deliberately does not
// implement driver.ExecerContext, and its Prepare and Begin are gated.
type roConn struct{ base driver.Conn }

func (c *roConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

func (c *roConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if err := CheckReadOnly(query); err != nil {
		return nil, err
	}
	var st driver.Stmt
	var err error
	if p, ok := c.base.(driver.ConnPrepareContext); ok {
		st, err = p.PrepareContext(ctx, query)
	} else {
		st, err = c.base.Prepare(query)
	}
	if err != nil {
		return nil, err
	}
	return &roStmt{base: st}, nil
}

// roStmt runs its (checked) query but never executes: database/sql turns
// an Exec on a connection without ExecerContext into Prepare + Stmt.Exec.
type roStmt struct{ base driver.Stmt }

func (s *roStmt) Close() error  { return s.base.Close() }
func (s *roStmt) NumInput() int { return s.base.NumInput() }
func (s *roStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, fmt.Errorf("%w: exec", ErrNotReadOnly)
}
func (s *roStmt) ExecContext(context.Context, []driver.NamedValue) (driver.Result, error) {
	return nil, fmt.Errorf("%w: exec", ErrNotReadOnly)
}
func (s *roStmt) Query(args []driver.Value) (driver.Rows, error) { return s.base.Query(args) } //nolint:staticcheck
func (s *roStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if q, ok := s.base.(driver.StmtQueryContext); ok {
		return q.QueryContext(ctx, args)
	}
	vals := make([]driver.Value, len(args))
	for i, a := range args {
		vals[i] = a.Value
	}
	return s.base.Query(vals) //nolint:staticcheck
}
func (s *roStmt) CheckNamedValue(nv *driver.NamedValue) error {
	if ch, ok := s.base.(driver.NamedValueChecker); ok {
		return ch.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

func (c *roConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if err := CheckReadOnly(query); err != nil {
		return nil, err
	}
	if q, ok := c.base.(driver.QueryerContext); ok {
		return q.QueryContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

func (c *roConn) Close() error { return c.base.Close() }

func (c *roConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{ReadOnly: true})
}

func (c *roConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if !opts.ReadOnly {
		return nil, fmt.Errorf("%w: read-write transaction", ErrNotReadOnly)
	}
	if b, ok := c.base.(driver.ConnBeginTx); ok {
		return b.BeginTx(ctx, opts)
	}
	return nil, fmt.Errorf("%w: driver has no read-only transactions", ErrNotReadOnly)
}

func (c *roConn) CheckNamedValue(nv *driver.NamedValue) error {
	if ch, ok := c.base.(driver.NamedValueChecker); ok {
		return ch.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

func (c *roConn) Ping(ctx context.Context) error {
	if p, ok := c.base.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

func (c *roConn) ResetSession(ctx context.Context) error {
	if r, ok := c.base.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

func (c *roConn) IsValid() bool {
	if v, ok := c.base.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}
