package hanadump

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
)

// HANA takes at most 32767 parameters in one statement (SQL error 589), and
// a long chain of LIKE predicates can exhaust the memory for compiling the
// statement before that (SQL error 4). Name patterns are therefore never
// sent in one statement: exact names go in IN lists of inChunks' size, and
// patterns with a wildcard in OR chains of likeChunk.
const likeChunk = 100

// cond is one WHERE fragment of bounded size and its arguments.
type cond struct {
	where string
	args  []any
}

// likeLiteral is the name a LIKE pattern matches when it has no wildcard.
func likeLiteral(p string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		switch c := p[i]; c {
		case '\\':
			if i+1 < len(p) {
				i++
				b.WriteByte(p[i])
				continue
			}
			b.WriteByte(c)
		case '%', '_':
			return "", false
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), true
}

// nameConds selects the rows whose col matches one of patterns, as bounded
// WHERE fragments. Fragments may select the same row: dumpConds writes it once.
func nameConds(col string, patterns []string) []cond {
	var lits, wild []string
	seen := map[string]bool{}
	for _, p := range patterns {
		if seen[p] {
			continue
		}
		seen[p] = true
		if lit, ok := likeLiteral(p); ok {
			lits = append(lits, lit)
		} else {
			wild = append(wild, p)
		}
	}
	var out []cond
	_ = inChunks(lits, func(ph string, args []any) error {
		out = append(out, cond{fmt.Sprintf(`%q IN (%s)`, col, ph), args})
		return nil
	})
	for at := 0; at < len(wild); at += likeChunk {
		w, a := likeAny(col, wild[at:min(at+likeChunk, len(wild))])
		out = append(out, cond{w, a})
	}
	return out
}

// rowSet remembers the rows already written to a table's file.
type rowSet map[[16]byte]struct{}

// tableWriter writes one table's file from several statements.
type tableWriter struct {
	ctx    context.Context
	db     *sql.DB
	opts   Options
	table  string
	opened bool   // the file exists: later statements append
	seen   rowSet // nil: the statements cannot select a row twice
	n      int
}

// dump runs one fragment, with extra (and its arguments) ANDed before it. missing is only ever true
// for the table's first statement.
func (t *tableWriter) dump(c cond, extra string, extraArgs ...any) (missing bool, err error) {
	where, args := c.where, c.args
	if extra != "" {
		where = extra + " AND (" + where + ")"
		args = append(append([]any{}, extraArgs...), args...)
	}
	n, missing, err := dumpRows(t.ctx, t.db, t.opts, t.table, where, args, t.opened, t.seen)
	t.opened, t.n = true, t.n+n
	return missing, err
}

func rowKey(line []byte) [16]byte {
	s := sha256.Sum256(line)
	var k [16]byte
	copy(k[:], s[:16])
	return k
}

// escapeAll turns names into the LIKE patterns that match exactly them.
func escapeAll(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = EscapeLike(n)
	}
	return out
}

// strings1Query reads the first column of a query's rows.
func strings1Query(ctx context.Context, db *sql.DB, q string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
