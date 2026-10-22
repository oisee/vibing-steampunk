package vsp

import (
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// `vsp dumps --impact` prints a unit's unresolved-include gap under the unit,
// so a caller reported as an include does not pass for a whole answer.
func TestPrintDumpImpactShowsTheUnresolvedIncludeGap(t *testing.T) {
	unresolved := []adt.Unsearched{{Object: "ZDEMO_INCL", Reason: "not resolved to its main program: status 500"}}
	result := &adt.DumpImpactResult{
		Units: []adt.ImpactUnit{{
			Object: "ZDEMO_FM", Type: "FUNC", Total: 1,
			Unresolved: unresolved, Gap: adt.UnresolvedIncludesNote(unresolved),
		}},
		Exposed: []adt.ExposedCaller{{Name: "ZDEMO_INCL", Type: "PROG/I", Via: "ZDEMO_FM"}},
	}
	var out string
	_ = captureStderr(t, func() {
		out = captureStdout(t, func() { printDumpImpact(adt.Dump{Program: "SAPLZDEMO_FG"}, result) })
	})
	if !strings.Contains(out, "1 direct callers") {
		t.Errorf("the unit's count is missing:\n%s", out)
	}
	if !strings.Contains(out, "not a complete answer") || !strings.Contains(out, "ZDEMO_INCL: not resolved") {
		t.Errorf("the unit's gap is not printed:\n%s", out)
	}
}
