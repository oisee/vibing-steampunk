package embedded

import (
	"strings"
	"testing"
)

// A string template on a table, structure or reference component (an ALV
// colour or style column) ends the runner's job step with
// STRG_ILLEGAL_DATA_TYPE, which CATCH cannot stop, so no rows come back.
// Those kinds must be routed away before the row loop formats anything.
func TestReportRunnerSkipsDeepComponentsBeforeFormatting(t *testing.T) {
	src := strings.ToLower(ZvspReportRunner)
	format := strings.Index(src, "|{ <gv_field> }|")
	if format < 0 {
		t.Fatal("runner no longer formats components with a string template; update this test")
	}
	for _, kind := range []string{"typekind_table", "typekind_struct1", "typekind_struct2", "typekind_oref", "typekind_dref"} {
		at := strings.Index(src, "cl_abap_typedescr=>"+kind)
		if at < 0 || at > format {
			t.Errorf("components of %s are formatted with a string template; skip them first", kind)
		}
	}
}
