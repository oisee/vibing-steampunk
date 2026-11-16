package hanadump

import (
	"context"
	"database/sql/driver"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLikeLiteral(t *testing.T) {
	for _, c := range []struct {
		p, lit string
		ok     bool
	}{
		{`ZCL\_A`, "ZCL_A", true}, {"ZCL_A", "", false}, {"Z%", "", false}, {`100\%`, "100%", true},
		{`A\\B`, `A\B`, true}, {EscapeLike(`/NS/X_Y%`), "/NS/X_Y%", true},
	} {
		if lit, ok := likeLiteral(c.p); lit != c.lit || ok != c.ok {
			t.Errorf("likeLiteral(%q) = %q %v", c.p, lit, ok)
		}
	}
}

func TestNameCondsAreBounded(t *testing.T) {
	var pats []string
	for i := 0; i < 1234; i++ {
		pats = append(pats, EscapeLike(fmt.Sprintf("ZP_%d", i)))
	}
	for i := 0; i < 250; i++ {
		pats = append(pats, fmt.Sprintf("ZW%d%%", i))
	}
	pats = append(pats, pats[0], pats[1300]) // repeated patterns are asked once
	conds := nameConds("C", pats)
	params := 0
	for _, c := range conds {
		if len(c.args) > 500 || strings.Count(c.where, "?") != len(c.args) {
			t.Errorf("fragment of %d parameters: %.80s", len(c.args), c.where)
		}
		params += len(c.args)
	}
	if params != 1234+250 {
		t.Errorf("%d parameters, want %d", params, 1234+250)
	}
	if len(conds) != 3+3 { // 500+500+234 literals, 100+100+50 wildcards
		t.Errorf("%d fragments", len(conds))
	}
}

// A row two fragments select is written once; the statements stay small.
func TestManyNamesTableRowsOnce(t *testing.T) {
	db, f := openFake(t,
		&fakeRule{contains: `.REPOSRC`, cols: []string{"PROGNAME", "R3STATE", "UNAM", "UDAT", "MAXLINELN", "DATA"}},
		&fakeRule{contains: `"S"."TRDIR"`, cols: []string{"NAME", "SUBC"}, rows: [][]driver.Value{{"ZP_7", "1"}, {"ZW1X", "1"}}},
		&fakeRule{contains: `FROM "S".`, cols: []string{"X"}},
	)
	opts := fakeOpts(t)
	opts.Tables = []Table{{"TRDIR", "NAME"}}
	for i := 0; i < 700; i++ {
		opts.Names = append(opts.Names, EscapeLike(fmt.Sprintf("ZP_%d", i)))
	}
	opts.Names = append(opts.Names, "ZW%")
	m, err := Run(context.Background(), db, opts)
	if err != nil {
		t.Fatal(err)
	}
	stmts := 0
	for _, q := range f.seen {
		if strings.Count(q, "?") > 500 {
			t.Errorf("statement of %d parameters", strings.Count(q, "?"))
		}
		if strings.Contains(q, `"TRDIR"`) {
			stmts++
		}
	}
	b, _ := os.ReadFile(filepath.Join(opts.Out, "tables", "TRDIR.jsonl"))
	// the fake answers both rows to each of the three statements
	if stmts != 3 || strings.Count(string(b), "\n") != 2 || m.Counts["rows_TRDIR"] != 2 {
		t.Errorf("%d statements, rows_TRDIR %d, file %q", stmts, m.Counts["rows_TRDIR"], b)
	}
}
