package vsp

import (
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// `vsp graph --direction callers` prints the unresolved includes beside the
// table, so a capped or failed lookup does not read as a complete answer.
func TestPrintWhereUsedCallersSaysWhichIncludesStayedUnresolved(t *testing.T) {
	out := captureStdout(t, func() {
		printWhereUsedCallers(
			[]adt.ExposedCaller{{Name: "ZDEMO_INCL", Type: "PROG/I", Package: "$ZDEMO"}},
			[]adt.Unsearched{{Object: "ZDEMO_INCL", Reason: "not resolved to its main program: status 500"}},
		)
	})
	if !strings.Contains(out, "not a complete answer") || !strings.Contains(out, "ZDEMO_INCL: not resolved") {
		t.Errorf("output does not name the unresolved include:\n%s", out)
	}
}
