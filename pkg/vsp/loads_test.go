package vsp

import (
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// The suffix is the only thing that says why a load exists, so it is the only
// thing worth naming — and naming it wrong is worse than leaving the code
// visible. SAP's own comment in CL_OO_CLASSNAME_SERVICE is the source, and for
// CT that comment ends in two question marks.

func TestPoolSuffixesAreQuotedNotInvented(t *testing.T) {
	for _, c := range []struct{ include, want string }{
		{"CX_ROOT======================CU", "public section"},
		{"ZCL_DEMO=====================CO", "protected section"},
		{"ZCL_DEMO=====================CI", "private section"},
		{"ZCL_DEMO=====================CP", "class pool"},
		{"ZCL_DEMO=====================CM001", "one method"},
	} {
		if got := poolKindOf(c.include); !strings.Contains(got, c.want) {
			t.Errorf("poolKindOf(%q) = %q, expected it to mention %q", c.include, got, c.want)
		}
	}
}

func TestASuffixSAPIsUnsureAboutIsReportedAsSuch(t *testing.T) {
	got := poolKindOf("CL_ABAP_TYPEDESCR============CT")
	if !strings.Contains(got, "question mark") {
		t.Errorf("SAP's own note about CT ends in ?? and the report should not be tidier than the vendor: %q", got)
	}
	if strings.Contains(strings.ToLower(got), "type declaration") {
		t.Errorf("that was the guess this replaced: %q", got)
	}
}

func TestAnUnknownSuffixIsPassedThroughRatherThanNamed(t *testing.T) {
	got := poolKindOf("ZIF_DEMO=====================IT")
	if !strings.Contains(got, "IT") {
		t.Errorf("a reader can look up a code and cannot unlearn a wrong name, got %q", got)
	}
}

func TestAnIncludeWithNoPaddingIsAPlainInclude(t *testing.T) {
	if got := poolKindOf("LZDEMO_GROUPTOP"); strings.Contains(got, "pool suffix") {
		t.Errorf("a function group include has no = padding and no pool suffix, got %q", got)
	}
}

func TestContainmentAndMachineryDoNotReachTheCommand(t *testing.T) {
	// The builder decides this, and the command must not second-guess it: a
	// class loading its own methods is containment and <SYSINI> is machinery.
	edges := loadEdgesOf([]adt.LoadRow{
		{Master: "ZCL_DEMO=====================CP", Include: "ZCL_DEMO=====================CM001"},
		{Master: "ZCL_DEMO=====================CP", Include: "<SYSINI>"},
		{Master: "ZCL_DEMO=====================CP", Include: "CX_ROOT======================CU"},
	})
	if len(edges) != 1 {
		t.Fatalf("only the load of another object is a dependency, got %d: %+v", len(edges), edges)
	}
	if !strings.Contains(edges[0].To, "CX_ROOT") {
		t.Errorf("the surviving edge should be the cross-object one, got %+v", edges[0])
	}
}

// Which end of an edge answers the question depends on the question. Printing
// the same end for both directions showed an object as its own loader, which is
// a wrong name rather than a missing one.

func TestEachDirectionNamesTheOtherParty(t *testing.T) {
	edges := []loadEdge{{From: "CLAS:ZCL_DEMO_CALLER", To: "CLAS:ZCL_DEMO_ORDER", Why: "x"}}
	report := map[string]any{}

	down := loadsText("ZCL_DEMO_ORDER", "loads", edges, nil, report)
	if !strings.Contains(down, "ZCL_DEMO_ORDER") {
		t.Errorf("what this loads is the far end:\n%s", down)
	}

	up := loadsText("ZCL_DEMO_ORDER", "loaded-by", nil, edges, report)
	if !strings.Contains(up, "ZCL_DEMO_CALLER") {
		t.Errorf("what loads this is the near end:\n%s", up)
	}
	if strings.Count(up, "ZCL_DEMO_ORDER") > 1 {
		t.Errorf("the object must not be listed as its own loader:\n%s", up)
	}
}
