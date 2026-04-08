package graph

import (
	"strings"
	"testing"
)

func TestCheckBoundaries_Clean(t *testing.T) {
	g := New()

	// Root package objects
	g.AddNode(&Node{ID: "CLAS:ZCL_DEV_SVC", Name: "ZCL_DEV_SVC", Type: "CLAS", Package: "$ZDEV"})
	g.AddNode(&Node{ID: "CLAS:ZCL_DEV_HELP", Name: "ZCL_DEV_HELP", Type: "CLAS", Package: "$ZDEV"})

	// Standard SAP object
	g.AddNode(&Node{ID: "CLAS:CL_GUI_ALV_GRID", Name: "CL_GUI_ALV_GRID", Type: "CLAS", Package: "SLIS"})

	// Whitelisted package
	g.AddNode(&Node{ID: "CLAS:ZCL_COMMON_LOG", Name: "ZCL_COMMON_LOG", Type: "CLAS", Package: "$ZCOMMON"})

	// Edges: all clean
	g.AddEdge(&Edge{From: "CLAS:ZCL_DEV_SVC", To: "CLAS:ZCL_DEV_HELP", Kind: EdgeCalls, Source: SourceParser})
	g.AddEdge(&Edge{From: "CLAS:ZCL_DEV_SVC", To: "CLAS:CL_GUI_ALV_GRID", Kind: EdgeReferences, Source: SourceParser})
	g.AddEdge(&Edge{From: "CLAS:ZCL_DEV_SVC", To: "CLAS:ZCL_COMMON_LOG", Kind: EdgeCalls, Source: SourceParser})

	report := g.CheckBoundaries("$ZDEV", &BoundaryOptions{
		Whitelist: []string{"$ZCOMMON"},
	})

	if !report.IsClean() {
		t.Errorf("Expected clean report, got %d violations", report.Violations)
	}
	if report.SamePackage != 1 {
		t.Errorf("SamePackage: got %d, want 1", report.SamePackage)
	}
	if report.Standard != 1 {
		t.Errorf("Standard: got %d, want 1", report.Standard)
	}
	if report.Allowed != 1 {
		t.Errorf("Allowed: got %d, want 1", report.Allowed)
	}
	if report.ExitCode() != 0 {
		t.Error("ExitCode should be 0 for clean report")
	}
}

func TestCheckBoundaries_Violations(t *testing.T) {
	g := New()

	// Root package
	g.AddNode(&Node{ID: "CLAS:ZCL_DEV_EXPORT", Name: "ZCL_DEV_EXPORT", Type: "CLAS", Package: "$ZDEV"})
	g.AddNode(&Node{ID: "CLAS:ZCL_DEV_IMPORT", Name: "ZCL_DEV_IMPORT", Type: "CLAS", Package: "$ZDEV"})

	// External Z-packages (not whitelisted)
	g.AddNode(&Node{ID: "CLAS:ZCL_HR_PAYROLL", Name: "ZCL_HR_PAYROLL", Type: "CLAS", Package: "$ZHR"})
	g.AddNode(&Node{ID: "FUGR:ZSALES_GET_DATA", Name: "ZSALES_GET_DATA", Type: "FUGR", Package: "$ZSALES"})

	// Violation edges
	g.AddEdge(&Edge{From: "CLAS:ZCL_DEV_EXPORT", To: "CLAS:ZCL_HR_PAYROLL", Kind: EdgeCalls, Source: SourceWBCROSSGT, RefDetail: "METHOD:GET_SALARY"})
	g.AddEdge(&Edge{From: "CLAS:ZCL_DEV_IMPORT", To: "FUGR:ZSALES_GET_DATA", Kind: EdgeCalls, Source: SourceCROSS, RefDetail: "FM:ZSALES_GET_DATA"})

	report := g.CheckBoundaries("$ZDEV", &BoundaryOptions{
		Whitelist: []string{"$ZCOMMON"},
	})

	if report.IsClean() {
		t.Error("Expected violations, got clean")
	}
	if report.Violations != 2 {
		t.Errorf("Violations: got %d, want 2", report.Violations)
	}
	if report.ExitCode() != 1 {
		t.Error("ExitCode should be 1 for violations")
	}
	if report.CrossedPackages["$ZHR"] != 1 {
		t.Errorf("CrossedPackages[$ZHR]: got %d, want 1", report.CrossedPackages["$ZHR"])
	}
	if report.CrossedPackages["$ZSALES"] != 1 {
		t.Errorf("CrossedPackages[$ZSALES]: got %d, want 1", report.CrossedPackages["$ZSALES"])
	}
	if len(report.ViolatingObjects) != 2 {
		t.Errorf("ViolatingObjects: got %d, want 2", len(report.ViolatingObjects))
	}
}

func TestCheckBoundaries_WhitelistGlob(t *testing.T) {
	g := New()

	g.AddNode(&Node{ID: "CLAS:ZCL_APP", Name: "ZCL_APP", Type: "CLAS", Package: "$ZAPP"})
	g.AddNode(&Node{ID: "CLAS:ZCL_UTIL_A", Name: "ZCL_UTIL_A", Type: "CLAS", Package: "$ZUTIL_CORE"})
	g.AddNode(&Node{ID: "CLAS:ZCL_UTIL_B", Name: "ZCL_UTIL_B", Type: "CLAS", Package: "$ZUTIL_EXT"})
	g.AddNode(&Node{ID: "CLAS:ZCL_OTHER", Name: "ZCL_OTHER", Type: "CLAS", Package: "$ZOTHER"})

	g.AddEdge(&Edge{From: "CLAS:ZCL_APP", To: "CLAS:ZCL_UTIL_A", Kind: EdgeCalls, Source: SourceParser})
	g.AddEdge(&Edge{From: "CLAS:ZCL_APP", To: "CLAS:ZCL_UTIL_B", Kind: EdgeCalls, Source: SourceParser})
	g.AddEdge(&Edge{From: "CLAS:ZCL_APP", To: "CLAS:ZCL_OTHER", Kind: EdgeCalls, Source: SourceParser})

	report := g.CheckBoundaries("$ZAPP", &BoundaryOptions{
		Whitelist: []string{"$ZUTIL*"}, // glob pattern
	})

	if report.Allowed != 2 {
		t.Errorf("Allowed: got %d, want 2 (both $ZUTIL_* should match)", report.Allowed)
	}
	if report.Violations != 1 {
		t.Errorf("Violations: got %d, want 1 ($ZOTHER)", report.Violations)
	}
}

func TestCheckBoundaries_DynamicCalls(t *testing.T) {
	g := New()

	g.AddNode(&Node{ID: "PROG:ZPROG", Name: "ZPROG", Type: "PROG", Package: "$ZDEV"})

	g.AddEdge(&Edge{From: "PROG:ZPROG", To: "DYNAMIC:lv_fm", Kind: EdgeDynamic, Source: SourceParser, RefDetail: "DYNAMIC_FM:lv_fm"})
	g.AddEdge(&Edge{From: "PROG:ZPROG", To: "DYNAMIC:lv_prog", Kind: EdgeDynamic, Source: SourceParser, RefDetail: "DYNAMIC_SUBMIT:lv_prog"})

	report := g.CheckBoundaries("$ZDEV", &BoundaryOptions{
		IncludeDynamic: true,
	})

	if report.Dynamic != 2 {
		t.Errorf("Dynamic: got %d, want 2", report.Dynamic)
	}
	// Dynamic calls don't count as violations but are warnings
	if report.Violations != 0 {
		t.Errorf("Violations should be 0 (dynamic are warnings), got %d", report.Violations)
	}
}

func TestCheckBoundaries_ParserEndToEnd(t *testing.T) {
	// Full end-to-end: parse ABAP source → build graph → check boundaries
	source := `REPORT zdev_main.
DATA lo_svc TYPE REF TO zcl_dev_service.
DATA lo_hr TYPE REF TO zcl_hr_payroll.
CALL FUNCTION 'Z_DEV_HELPER' EXPORTING iv = lv.
CALL FUNCTION 'BAPI_USER_GET_DETAIL' EXPORTING username = lv_user.
SUBMIT zother_pkg_report.
CALL FUNCTION lv_dynamic_fm.
zcl_common_util=>log( 'test' ).
`

	g := New()

	// Parse source for deps
	nodeID := "PROG:ZDEV_MAIN"
	edges := ExtractDepsFromSource(source, nodeID)
	dynEdges := ExtractDynamicCalls(source, nodeID)

	// Add source node
	g.AddNode(&Node{ID: nodeID, Name: "ZDEV_MAIN", Type: "PROG", Package: "$ZDEV"})

	// Add target nodes with package info (simulating TADIR resolution)
	packages := map[string]string{
		"CLAS:ZCL_DEV_SERVICE":  "$ZDEV",
		"CLAS:ZCL_HR_PAYROLL":   "$ZHR",
		"FUGR:Z_DEV_HELPER":     "$ZDEV",
		"FUGR:BAPI_USER_GET_DETAIL": "SWO1", // standard
		"PROG:ZOTHER_PKG_REPORT": "$ZOTHER",
		"CLAS:ZCL_COMMON_UTIL":  "$ZCOMMON",
	}

	for _, e := range append(edges, dynEdges...) {
		g.AddEdge(e)
		if pkg, ok := packages[e.To]; ok {
			_, typ, name := NormalizeInclude(strings.TrimPrefix(e.To, strings.Split(e.To, ":")[0]+":"))
			if typ == "" {
				parts := strings.SplitN(e.To, ":", 2)
				typ = parts[0]
				name = parts[1]
			}
			g.AddNode(&Node{ID: e.To, Name: name, Type: typ, Package: pkg})
		}
	}

	// Check boundaries
	report := g.CheckBoundaries("$ZDEV", &BoundaryOptions{
		Whitelist:      []string{"$ZCOMMON"},
		IncludeDynamic: true,
	})

	text := report.FormatText()
	t.Log("\n" + text)

	// Should find violations
	if report.Violations == 0 {
		t.Error("Expected violations (ZCL_HR_PAYROLL in $ZHR, ZOTHER_PKG_REPORT in $ZOTHER)")
	}

	// Should detect dynamic call
	if report.Dynamic == 0 {
		t.Error("Expected dynamic call warning for lv_dynamic_fm")
	}

	// Standard deps should be counted
	if report.Standard == 0 {
		t.Error("Expected at least 1 standard dep (BAPI_USER_GET_DETAIL)")
	}

	// Whitelist should work
	if !strings.Contains(text, "ALLOWED") || report.Allowed == 0 {
		t.Error("Expected ZCL_COMMON_UTIL to be in ALLOWED (whitelisted $ZCOMMON)")
	}

	// Should report crossed packages
	if _, ok := report.CrossedPackages["$ZHR"]; !ok {
		t.Error("Expected $ZHR in crossed packages")
	}
}
