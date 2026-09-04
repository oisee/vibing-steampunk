package adt

import (
	"strings"
	"testing"
)

func TestWrapSQL(t *testing.T) {
	// A long IN list with a literal that must not be split, and a long
	// ORDER BY that the data preview once cut into "sdlt" and "ime".
	var lits []string
	for i := 0; i < 40; i++ {
		lits = append(lits, "'0aAi4G0H7{2vu4jDCI}OeW'")
	}
	q := "SELECT a, b FROM t WHERE x IN ( " + strings.Join(lits, ", ") + " ) AND y = 'a value with spaces inside' ORDER BY sdldate DESCENDING, sdltime DESCENDING"
	w := wrapSQL(q)
	for n, line := range strings.Split(w, "\n") {
		if len(line) > 200 {
			t.Errorf("line %d is %d characters", n+1, len(line))
		}
	}
	if strings.ReplaceAll(w, "\n", " ") != q {
		t.Error("wrapping changed the statement's words")
	}
	if !strings.Contains(w, "'a value with spaces inside'") {
		t.Error("a literal was broken")
	}
	if strings.Count(w, "\n") < 4 {
		t.Errorf("expected several lines, got %d", strings.Count(w, "\n")+1)
	}
	if wrapSQL("SELECT 1 FROM t") != "SELECT 1 FROM t" {
		t.Error("a short statement changed")
	}
}
