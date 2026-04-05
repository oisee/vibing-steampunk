package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestAssembleStatements_MultiLine(t *testing.T) {
	source := `SELECT matnr maktx
  FROM mara
  INTO TABLE @DATA(lt_mara)
  WHERE matnr LIKE 'Z%'.`

	stmts := AssembleStatements(source)

	nonComment := 0
	for _, s := range stmts {
		if !s.IsComment {
			nonComment++
		}
	}
	if nonComment != 1 {
		t.Fatalf("expected 1 statement, got %d", nonComment)
	}
	if stmts[0].StartLine != 1 || stmts[0].EndLine != 4 {
		t.Errorf("expected lines 1-4, got %d-%d", stmts[0].StartLine, stmts[0].EndLine)
	}
	if !strings.Contains(stmts[0].Text, "SELECT") || !strings.Contains(stmts[0].Text, "WHERE") {
		t.Error("assembled statement should contain SELECT and WHERE")
	}
}

func TestAssembleStatements_Comments(t *testing.T) {
	source := `* Full-line comment
DATA: lv_x TYPE string. " inline comment
* Another comment`

	stmts := AssembleStatements(source)

	comments := 0
	code := 0
	for _, s := range stmts {
		if s.IsComment {
			comments++
		} else {
			code++
		}
	}
	if comments != 2 {
		t.Errorf("expected 2 comments, got %d", comments)
	}
	if code != 1 {
		t.Errorf("expected 1 code statement, got %d", code)
	}
	// Inline comment should be stripped
	for _, s := range stmts {
		if !s.IsComment && strings.Contains(s.Text, "inline comment") {
			t.Error("inline comment should be stripped from code statement")
		}
	}
}

func TestAssembleStatements_StringLiterals(t *testing.T) {
	source := `lv_str = 'Hello. World'.`

	stmts := AssembleStatements(source)

	nonComment := 0
	for _, s := range stmts {
		if !s.IsComment {
			nonComment++
		}
	}
	if nonComment != 1 {
		t.Fatalf("expected 1 statement (period inside string), got %d", nonComment)
	}
}

func TestAnalyzeABAPSource_SelectInLoop(t *testing.T) {
	source := `LOOP AT lt_items INTO DATA(ls_item).
  SELECT SINGLE matnr FROM mara INTO @DATA(lv_matnr) WHERE matnr = @ls_item-matnr.
ENDLOOP.`

	result := AnalyzeABAPSource(source)

	found := false
	for _, f := range result.Findings {
		if f.Rule == "select_in_loop" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected select_in_loop finding")
	}
}

func TestAnalyzeABAPSource_FAENoCheck(t *testing.T) {
	source := `DATA: lt_items TYPE TABLE OF mara.
SELECT * FROM mara INTO TABLE @lt_items FOR ALL ENTRIES IN @lt_keys WHERE matnr = @lt_keys-matnr.`

	result := AnalyzeABAPSource(source)

	found := false
	for _, f := range result.Findings {
		if f.Rule == "fae_no_empty_check" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected fae_no_empty_check finding when no IF check precedes FAE")
	}
}

func TestAnalyzeABAPSource_FAEWithCheck(t *testing.T) {
	source := `IF lt_keys IS NOT INITIAL.
  SELECT * FROM mara INTO TABLE @lt_items FOR ALL ENTRIES IN @lt_keys WHERE matnr = @lt_keys-matnr.
ENDIF.`

	result := AnalyzeABAPSource(source)

	for _, f := range result.Findings {
		if f.Rule == "fae_no_empty_check" {
			t.Error("should NOT report fae_no_empty_check when preceded by IS NOT INITIAL")
		}
	}
}

func TestAnalyzeABAPSource_CommitInLoop(t *testing.T) {
	source := `LOOP AT lt_items INTO DATA(ls_item).
  MODIFY ztable FROM ls_item.
  COMMIT WORK.
ENDLOOP.`

	result := AnalyzeABAPSource(source)

	found := false
	for _, f := range result.Findings {
		if f.Rule == "commit_in_loop" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected commit_in_loop finding")
	}
}

func TestAnalyzeABAPSource_MissingSysubrc(t *testing.T) {
	source := `READ TABLE lt_data INTO DATA(ls_data) WITH KEY matnr = lv_matnr.
lv_result = ls_data-werks.`

	result := AnalyzeABAPSource(source)

	found := false
	for _, f := range result.Findings {
		if f.Rule == "missing_sysubrc_read" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected missing_sysubrc_read finding when SY-SUBRC is not checked")
	}
}

func TestAnalyzeABAPSource_HardcodedCredentials(t *testing.T) {
	source := `lv_password = 'SuperSecret123'.
lv_user = 'admin'.`

	result := AnalyzeABAPSource(source)

	found := false
	for _, f := range result.Findings {
		if f.Rule == "hardcoded_credentials" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected hardcoded_credentials finding for password assignment")
	}
}

func TestAnalyzeABAPSource_DynamicCallNoTry(t *testing.T) {
	source := `CALL METHOD (lv_class_name)=>(lv_method_name).`

	result := AnalyzeABAPSource(source)

	found := false
	for _, f := range result.Findings {
		if f.Rule == "dynamic_call_no_try" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected dynamic_call_no_try finding for dynamic CALL METHOD")
	}
}

func TestAnalyzeABAPSource_CleanCode(t *testing.T) {
	source := `METHOD do_something.
  DATA(lt_data) = get_data( ).
  IF lt_data IS NOT INITIAL.
    SELECT matnr maktx FROM mara
      INTO TABLE @DATA(lt_result)
      FOR ALL ENTRIES IN @lt_data
      WHERE matnr = @lt_data-matnr.
    IF sy-subrc = 0.
      process( lt_result ).
    ENDIF.
  ENDIF.
ENDMETHOD.`

	result := AnalyzeABAPSource(source)

	criticals := 0
	for _, f := range result.Findings {
		if f.Severity == "critical" || f.Severity == "high" {
			criticals++
		}
	}
	if criticals > 0 {
		for _, f := range result.Findings {
			if f.Severity == "critical" || f.Severity == "high" {
				t.Errorf("unexpected critical/high finding in clean code: %s (line %d)", f.Rule, f.Line)
			}
		}
	}
}

func TestCodeAnalysisSummary_Score(t *testing.T) {
	tests := []struct {
		name     string
		findings []CodeFinding
		expected string
	}{
		{"good", nil, "good"},
		{"good-info", []CodeFinding{{Severity: "info"}}, "good"},
		{"warning", []CodeFinding{{Severity: "high"}}, "warning"},
		{"critical", []CodeFinding{{Severity: "critical"}, {Severity: "info"}}, "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := calculateCodeScore(tt.findings)
			if score != tt.expected {
				t.Errorf("calculateCodeScore() = %q, want %q", score, tt.expected)
			}
		})
	}
}

func TestClient_AnalyzeABAPCode_URI(t *testing.T) {
	abapSource := `REPORT ztest.
SELECT * FROM mara INTO TABLE @DATA(lt_mara).
LOOP AT lt_mara INTO DATA(ls_mara).
  SELECT SINGLE maktx FROM makt INTO @DATA(lv_text) WHERE matnr = @ls_mara-matnr.
ENDLOOP.`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"source/main": newTestResponse(abapSource),
			"discovery":   newTestResponse("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.AnalyzeABAPCode(context.Background(), "/sap/bc/adt/programs/programs/ztest", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ObjectURI != "/sap/bc/adt/programs/programs/ztest" {
		t.Errorf("ObjectURI = %q, want /sap/bc/adt/programs/programs/ztest", result.ObjectURI)
	}
	if len(result.Findings) == 0 {
		t.Error("expected findings from analysis")
	}

	// Should find select_in_loop at minimum
	found := false
	for _, f := range result.Findings {
		if f.Rule == "select_in_loop" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected select_in_loop finding from URI-based analysis")
	}
}

func TestAssembleStatements_StringTemplate(t *testing.T) {
	// ABAP string templates use |...| and can contain periods
	source := `lv_msg = |Hello. World { lv_name }. Done|.
lv_other = 'test'.`

	stmts := AssembleStatements(source)

	nonComment := 0
	for _, s := range stmts {
		if !s.IsComment {
			nonComment++
		}
	}
	if nonComment != 2 {
		t.Fatalf("expected 2 statements (period inside |...| should not split), got %d", nonComment)
	}
	if !strings.Contains(stmts[0].Text, "Hello. World") {
		t.Error("string template content should be preserved")
	}
}

func TestAssembleStatements_BacktickLiteral(t *testing.T) {
	// Backtick strings can also contain periods
	source := "lv_str = `Contains. Period`.\nlv_x = 1."

	stmts := AssembleStatements(source)

	nonComment := 0
	for _, s := range stmts {
		if !s.IsComment {
			nonComment++
		}
	}
	if nonComment != 2 {
		t.Fatalf("expected 2 statements (period inside backtick should not split), got %d", nonComment)
	}
}

func TestAnalyzeABAPSource_DynamicCallFunction(t *testing.T) {
	// CALL FUNCTION (var) form — was a bug (H3: missed this form)
	source := `DATA: lv_fname TYPE funcname.
lv_fname = 'BAPI_USER_GET_DETAIL'.
CALL FUNCTION lv_fname EXPORTING username = 'ADMIN'.`

	result := AnalyzeABAPSource(source)

	found := false
	for _, f := range result.Findings {
		if f.Rule == "dynamic_call_no_try" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected dynamic_call_no_try for CALL FUNCTION with variable name")
	}
}

func TestAnalyzeABAPSource_StaticCallFunctionNoFalsePositive(t *testing.T) {
	// Static CALL FUNCTION with literal name should NOT trigger dynamic_call_no_try
	source := `CALL FUNCTION 'BAPI_USER_GET_DETAIL'
  EXPORTING username = 'ADMIN'
  IMPORTING return = ls_return.
IF sy-subrc <> 0.
  MESSAGE ls_return-message TYPE 'E'.
ENDIF.`

	result := AnalyzeABAPSource(source)

	for _, f := range result.Findings {
		if f.Rule == "dynamic_call_no_try" {
			t.Errorf("static CALL FUNCTION 'LITERAL' should not trigger dynamic_call_no_try, got: %s", f.Match)
		}
	}
}

func TestAnalyzeABAPSource_TodoFixme(t *testing.T) {
	// Fix M1: todo_fixme rule should fire on comment lines
	source := `* TODO: implement error handling
DATA: lv_x TYPE string.`

	result := AnalyzeABAPSource(source)

	found := false
	for _, f := range result.Findings {
		if f.Rule == "todo_fixme" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected todo_fixme finding on comment line with TODO")
	}
}

func TestClient_AnalyzeABAPCode_Source(t *testing.T) {
	source := `REPORT ztest.
DATA: lv_x TYPE string.`

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"discovery": newTestResponse("OK"),
		},
	}
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.AnalyzeABAPCode(context.Background(), "", source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RulesApplied != 26 {
		t.Errorf("RulesApplied = %d, want 26 (21 regex + 5 abaplint rules)", result.RulesApplied)
	}
}

// --- Tests for abaplint integration path ---

func TestAnalyzeABAPSource_AbaplintSelectStar(t *testing.T) {
	// select_star should be found via abaplint linter (AST-based)
	source := `SELECT * FROM mara INTO TABLE @DATA(lt_mara) WHERE matnr LIKE 'Z%'.`

	result := AnalyzeABAPSource(source)

	foundAbaplint := false
	for _, f := range result.Findings {
		if f.Rule == "select_star" && f.Line > 0 {
			foundAbaplint = true
		}
	}
	if !foundAbaplint {
		t.Error("expected select_star finding with non-zero line from abaplint linter")
	}
}

func TestAnalyzeABAPSource_AbaplintHardcodedCredentials(t *testing.T) {
	// hardcoded_credentials via abaplint linter (token-type aware)
	source := `lv_password = 'TopSecret99'.`

	result := AnalyzeABAPSource(source)

	found := false
	for _, f := range result.Findings {
		if f.Rule == "hardcoded_credentials" {
			found = true
			if f.Line == 0 {
				t.Error("abaplint hardcoded_credentials finding should have non-zero line")
			}
			break
		}
	}
	if !found {
		t.Error("expected hardcoded_credentials finding from abaplint linter")
	}
}

func TestAnalyzeABAPSource_AbaplintCatchCxRoot(t *testing.T) {
	source := "TRY.\n  do_something( ).\nCATCH cx_root.\nENDTRY."

	result := AnalyzeABAPSource(source)

	found := false
	for _, f := range result.Findings {
		if f.Rule == "catch_cx_root" {
			found = true
			if f.Line == 0 {
				t.Error("abaplint catch_cx_root finding should have non-zero line")
			}
			break
		}
	}
	if !found {
		t.Error("expected catch_cx_root finding from abaplint linter")
	}
}

func TestAnalyzeABAPSource_AbaplintCommitInLoop(t *testing.T) {
	source := "LOOP AT lt_items INTO DATA(ls_item).\n  COMMIT WORK.\nENDLOOP."

	result := AnalyzeABAPSource(source)

	found := false
	for _, f := range result.Findings {
		if f.Rule == "commit_in_loop" {
			found = true
			if f.Line == 0 {
				t.Error("abaplint commit_in_loop finding should have non-zero line")
			}
			break
		}
	}
	if !found {
		t.Error("expected commit_in_loop finding from abaplint linter")
	}
}

func TestAnalyzeABAPSource_AbaplintDynamicCallNoTry(t *testing.T) {
	source := "CALL METHOD (lv_class)=>(lv_method)."

	result := AnalyzeABAPSource(source)

	found := false
	for _, f := range result.Findings {
		if f.Rule == "dynamic_call_no_try" {
			found = true
			if f.Line == 0 {
				t.Error("abaplint dynamic_call_no_try finding should have non-zero line")
			}
			break
		}
	}
	if !found {
		t.Error("expected dynamic_call_no_try finding from abaplint linter")
	}
}

func TestRunAbaplintLinter_SeverityMapping(t *testing.T) {
	// Verify severity mappings for abaplint → CodeFinding
	source := "SELECT * FROM mara INTO TABLE @lt_mara."
	findings := runAbaplintLinter(source)

	if len(findings) == 0 {
		t.Fatal("expected at least one finding from abaplint linter")
	}
	for _, f := range findings {
		if f.Rule == "select_star" {
			// SelectStarRule emits "Warning" → should map to "medium"
			if f.Severity != "medium" {
				t.Errorf("select_star severity = %q, want medium", f.Severity)
			}
			if f.Category != "performance" {
				t.Errorf("select_star category = %q, want performance", f.Category)
			}
			if f.Suggestion == "" {
				t.Error("select_star should have a non-empty suggestion")
			}
		}
	}
}

func TestRunAbaplintLinter_StaticCallFunctionNoFalsePositive(t *testing.T) {
	// Static CALL FUNCTION 'LITERAL' must NOT be flagged as dynamic
	source := "CALL FUNCTION 'RFC_READ_TABLE' EXPORTING query_table = 'MARA'."
	findings := runAbaplintLinter(source)

	for _, f := range findings {
		if f.Rule == "dynamic_call_no_try" {
			t.Errorf("static CALL FUNCTION should not trigger dynamic_call_no_try; match: %s", f.Match)
		}
	}
}
