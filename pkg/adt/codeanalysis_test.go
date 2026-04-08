package adt

import (
	"testing"
)

func TestAnalyzeABAPSource_SelectStar(t *testing.T) {
	source := "REPORT ztest.\nSELECT * FROM mara INTO TABLE @lt_mara.\n"
	result := AnalyzeABAPSource(source)
	found := findFinding(result, "select_star")
	if found == nil {
		t.Fatal("expected select_star finding")
	}
	if found.Category != "performance" {
		t.Errorf("category = %q, want performance", found.Category)
	}
}

func TestAnalyzeABAPSource_HardcodedCredentials(t *testing.T) {
	source := "REPORT ztest.\nlv_password = 'secret123'.\n"
	result := AnalyzeABAPSource(source)
	found := findFinding(result, "hardcoded_credentials")
	if found == nil {
		t.Fatal("expected hardcoded_credentials finding")
	}
	if found.Category != "security" {
		t.Errorf("category = %q, want security", found.Category)
	}
}

func TestAnalyzeABAPSource_CatchCxRoot(t *testing.T) {
	source := "REPORT ztest.\nTRY.\n  lv_x = 1.\nCATCH cx_root.\nENDTRY.\n"
	result := AnalyzeABAPSource(source)
	found := findFinding(result, "catch_cx_root")
	if found == nil {
		t.Fatal("expected catch_cx_root finding")
	}
	if found.Category != "robustness" {
		t.Errorf("category = %q, want robustness", found.Category)
	}
}

func TestAnalyzeABAPSource_CommitInLoop(t *testing.T) {
	source := "REPORT ztest.\nLOOP AT lt_data INTO ls_data.\n  COMMIT WORK.\nENDLOOP.\n"
	result := AnalyzeABAPSource(source)
	found := findFinding(result, "commit_in_loop")
	if found == nil {
		t.Fatal("expected commit_in_loop finding")
	}
	if found.Category != "performance" {
		t.Errorf("category = %q, want performance", found.Category)
	}
}

func TestAnalyzeABAPSource_DynamicCallNoTry(t *testing.T) {
	source := "REPORT ztest.\nCALL FUNCTION lv_funcname.\n"
	result := AnalyzeABAPSource(source)
	found := findFinding(result, "dynamic_call_no_try")
	if found == nil {
		t.Fatal("expected dynamic_call_no_try finding")
	}
}

func TestAnalyzeABAPSource_ObsoleteStatement(t *testing.T) {
	source := "REPORT ztest.\nMOVE lv_a TO lv_b.\n"
	result := AnalyzeABAPSource(source)
	found := findFinding(result, "obsolete_statement")
	if found == nil {
		t.Fatal("expected obsolete_statement finding")
	}
	if found.Category != "quality" {
		t.Errorf("category = %q, want quality", found.Category)
	}
}

func TestAnalyzeABAPSource_CleanCode(t *testing.T) {
	source := "REPORT ztest.\nDATA lv_count TYPE i.\nlv_count = 1.\n"
	result := AnalyzeABAPSource(source)

	for _, f := range result.Findings {
		if f.Severity == "critical" || f.Severity == "high" {
			t.Errorf("unexpected %s finding: %s", f.Severity, f.Rule)
		}
	}
	if result.Summary.Score == "critical" {
		t.Error("clean code should not score critical")
	}
}

func TestAnalyzeABAPSource_SortOrder(t *testing.T) {
	source := "REPORT ztest.\nSELECT * FROM mara INTO TABLE @lt_mara.\nMOVE lv_a TO lv_b.\n"
	result := AnalyzeABAPSource(source)
	if len(result.Findings) < 2 {
		t.Skipf("need at least 2 findings, got %d", len(result.Findings))
	}
	for i := 1; i < len(result.Findings); i++ {
		prev := result.Findings[i-1]
		curr := result.Findings[i]
		if prev.Line > curr.Line {
			t.Errorf("findings not sorted by line: %d > %d", prev.Line, curr.Line)
		}
		if prev.Line == curr.Line && prev.Rule > curr.Rule {
			t.Errorf("findings not sorted by rule within line: %s > %s", prev.Rule, curr.Rule)
		}
	}
}

func TestAnalyzeABAPSource_TooLarge(t *testing.T) {
	huge := make([]byte, maxSourceBytes+1)
	for i := range huge {
		huge[i] = ' '
	}
	result := AnalyzeABAPSource(string(huge))
	if result.RulesApplied != 0 {
		t.Errorf("RulesApplied = %d, want 0 for oversized input", result.RulesApplied)
	}
	if len(result.Findings) != 1 || result.Findings[0].Rule != "source_too_large" {
		t.Errorf("expected source_too_large finding")
	}
}

func TestAnalyzeABAPSource_RulesApplied(t *testing.T) {
	result := AnalyzeABAPSource("REPORT ztest.\n")
	want := len(allRules())
	if result.RulesApplied != want {
		t.Errorf("RulesApplied = %d, want %d", result.RulesApplied, want)
	}
}

func findFinding(result *CodeAnalysisResult, rule string) *CodeFinding {
	for i := range result.Findings {
		if result.Findings[i].Rule == rule {
			return &result.Findings[i]
		}
	}
	return nil
}
