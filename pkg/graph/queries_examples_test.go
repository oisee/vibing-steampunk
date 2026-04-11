package graph

import (
	"fmt"
	"strings"
	"testing"
)

func TestFindUsageExamples_CallFunction(t *testing.T) {
	target := UsageTarget{ObjectType: "FUNC", ObjectName: "Z_CALC_TAX"}
	callers := []CallerSource{
		{
			NodeID: "PROG:ZREPORT", Name: "ZREPORT", Type: "PROG",
			Source: `REPORT zreport.
DATA lv_result TYPE string.
CALL FUNCTION 'Z_CALC_TAX'
  EXPORTING iv_amount = 100
  IMPORTING ev_tax = lv_result.
WRITE lv_result.`,
		},
	}

	result := FindUsageExamples(target, callers, 10)

	if len(result.Examples) != 1 {
		t.Fatalf("Expected 1 example, got %d", len(result.Examples))
	}
	ex := result.Examples[0]
	if ex.MatchType != "CALL_FUNCTION" {
		t.Errorf("MatchType: got %q, want CALL_FUNCTION", ex.MatchType)
	}
	if ex.Confidence != "HIGH" {
		t.Errorf("Confidence: got %q, want HIGH", ex.Confidence)
	}
	if ex.LineNumber != 3 {
		t.Errorf("LineNumber: got %d, want 3", ex.LineNumber)
	}
	if !strings.Contains(ex.Snippet, "Z_CALC_TAX") {
		t.Error("Snippet should contain FM name")
	}
	if !strings.Contains(ex.Snippet, "iv_amount") {
		t.Error("Snippet should include context lines with parameters")
	}
}

func TestFindUsageExamples_MethodCall_Static(t *testing.T) {
	target := UsageTarget{ObjectType: "CLAS", ObjectName: "ZCL_TRAVEL", Method: "GET_DATA"}
	callers := []CallerSource{
		{
			NodeID: "CLAS:ZCL_ORDER", Name: "ZCL_ORDER", Type: "CLAS",
			Source: `CLASS zcl_order IMPLEMENTATION.
  METHOD create.
    DATA(ls_travel) = zcl_travel=>get_data( iv_id = lv_id ).
    IF ls_travel IS NOT INITIAL.
      " do something
    ENDIF.
  ENDMETHOD.
ENDCLASS.`,
		},
	}

	result := FindUsageExamples(target, callers, 10)

	if len(result.Examples) != 1 {
		t.Fatalf("Expected 1 example, got %d", len(result.Examples))
	}
	if result.Examples[0].MatchType != "METHOD_CALL" {
		t.Errorf("MatchType: got %q, want METHOD_CALL", result.Examples[0].MatchType)
	}
	if result.Examples[0].LineNumber != 3 {
		t.Errorf("LineNumber: got %d, want 3", result.Examples[0].LineNumber)
	}
}

func TestFindUsageExamples_MethodCall_Instance(t *testing.T) {
	target := UsageTarget{ObjectType: "CLAS", ObjectName: "ZCL_TRAVEL", Method: "GET_DATA"}
	callers := []CallerSource{
		{
			NodeID: "PROG:ZPROG", Name: "ZPROG", Type: "PROG",
			Source: `REPORT zprog.
DATA lo TYPE REF TO zcl_travel.
CREATE OBJECT lo.
lo->get_data( EXPORTING iv_id = '001' IMPORTING es_result = ls_data ).
WRITE ls_data-name.`,
		},
	}

	result := FindUsageExamples(target, callers, 10)

	if len(result.Examples) != 1 {
		t.Fatalf("Expected 1 example, got %d", len(result.Examples))
	}
	if result.Examples[0].MatchType != "METHOD_CALL" {
		t.Errorf("MatchType: got %q, want METHOD_CALL", result.Examples[0].MatchType)
	}
}

func TestFindUsageExamples_InterfaceMethod(t *testing.T) {
	target := UsageTarget{ObjectType: "INTF", ObjectName: "ZIF_API", Method: "EXECUTE"}
	callers := []CallerSource{
		{
			NodeID: "CLAS:ZCL_CONSUMER", Name: "ZCL_CONSUMER", Type: "CLAS",
			Source: `CLASS zcl_consumer IMPLEMENTATION.
  METHOD run.
    DATA lo TYPE REF TO zif_api.
    lo = me->mo_api.
    lo->zif_api~execute( iv_param = lv_val ).
  ENDMETHOD.
ENDCLASS.`,
		},
	}

	result := FindUsageExamples(target, callers, 10)

	if len(result.Examples) != 1 {
		t.Fatalf("Expected 1 example, got %d", len(result.Examples))
	}
	if result.Examples[0].MatchType != "METHOD_CALL" {
		t.Errorf("MatchType: got %q, want METHOD_CALL", result.Examples[0].MatchType)
	}
}

func TestFindUsageExamples_Submit(t *testing.T) {
	target := UsageTarget{ObjectType: "SUBMIT", ObjectName: "ZREPORT_EXPORT"}
	callers := []CallerSource{
		{
			NodeID: "PROG:ZSCHEDULER", Name: "ZSCHEDULER", Type: "PROG",
			Source: `REPORT zscheduler.
SUBMIT zreport_export WITH p_date = sy-datum
                       WITH p_mode = 'X'
                       AND RETURN.
WRITE 'Done'.`,
		},
	}

	result := FindUsageExamples(target, callers, 10)

	if len(result.Examples) != 1 {
		t.Fatalf("Expected 1 example, got %d", len(result.Examples))
	}
	if result.Examples[0].MatchType != "SUBMIT" {
		t.Errorf("MatchType: got %q, want SUBMIT", result.Examples[0].MatchType)
	}
	if !strings.Contains(result.Examples[0].Snippet, "p_date") {
		t.Error("Snippet should include WITH parameters")
	}
}

func TestFindUsageExamples_PerformInProgram(t *testing.T) {
	target := UsageTarget{ObjectType: "PROG", ObjectName: "ZPRICING", Form: "CALC_TAX"}
	callers := []CallerSource{
		{
			NodeID: "PROG:ZCALLER", Name: "ZCALLER", Type: "PROG",
			Source: `REPORT zcaller.
DATA lv_tax TYPE p.
PERFORM calc_tax IN PROGRAM zpricing USING lv_amount
                                     CHANGING lv_tax.
WRITE lv_tax.`,
		},
	}

	result := FindUsageExamples(target, callers, 10)

	if len(result.Examples) != 1 {
		t.Fatalf("Expected 1 example, got %d", len(result.Examples))
	}
	if result.Examples[0].MatchType != "PERFORM" {
		t.Errorf("MatchType: got %q, want PERFORM", result.Examples[0].MatchType)
	}
	if result.Examples[0].Confidence != "HIGH" {
		t.Errorf("Confidence: got %q, want HIGH", result.Examples[0].Confidence)
	}
}

func TestFindUsageExamples_GrepFallback(t *testing.T) {
	// Target method used in a way that doesn't match exact patterns
	target := UsageTarget{ObjectType: "CLAS", ObjectName: "ZCL_UTILS", Method: "FORMAT"}
	callers := []CallerSource{
		{
			NodeID: "PROG:ZPROG", Name: "ZPROG", Type: "PROG",
			Source: `REPORT zprog.
* This program uses FORMAT method from ZCL_UTILS
DATA lv TYPE string.
lv = lo_utils->format( iv_input ).`,
		},
	}

	result := FindUsageExamples(target, callers, 10)

	// Should find via grep (->format matches METHOD_CALL, but let's also verify grep works)
	if len(result.Examples) == 0 {
		t.Fatal("Expected at least 1 example via method match or grep")
	}
}

func TestFindUsageExamples_CommentSkipped(t *testing.T) {
	target := UsageTarget{ObjectType: "FUNC", ObjectName: "Z_MY_FM"}
	callers := []CallerSource{
		{
			NodeID: "PROG:ZPROG", Name: "ZPROG", Type: "PROG",
			Source: `REPORT zprog.
* CALL FUNCTION 'Z_MY_FM' — this is just a comment
" Another comment mentioning Z_MY_FM
DATA lv TYPE string.`,
		},
	}

	result := FindUsageExamples(target, callers, 10)

	// Comments should not produce CALL_FUNCTION matches
	for _, ex := range result.Examples {
		if ex.MatchType == "CALL_FUNCTION" {
			t.Error("Comment line should not match as CALL_FUNCTION")
		}
	}
}

func TestFindUsageExamples_NoMatch(t *testing.T) {
	target := UsageTarget{ObjectType: "FUNC", ObjectName: "Z_NONEXISTENT"}
	callers := []CallerSource{
		{
			NodeID: "PROG:ZPROG", Name: "ZPROG", Type: "PROG",
			Source: `REPORT zprog.
CALL FUNCTION 'BAPI_USER_GET_DETAIL'.
WRITE 'hello'.`,
		},
	}

	result := FindUsageExamples(target, callers, 10)

	if len(result.Examples) != 0 {
		t.Errorf("Expected 0 examples for non-matching target, got %d", len(result.Examples))
	}
}

func TestFindUsageExamples_EmptyCallers(t *testing.T) {
	target := UsageTarget{ObjectType: "FUNC", ObjectName: "Z_FM"}
	result := FindUsageExamples(target, nil, 10)

	if len(result.Examples) != 0 {
		t.Errorf("Expected 0 examples for nil callers, got %d", len(result.Examples))
	}
	if result.TotalCallers != 0 {
		t.Errorf("TotalCallers: got %d, want 0", result.TotalCallers)
	}
}

func TestFindUsageExamples_Ranking_TestFirst(t *testing.T) {
	target := UsageTarget{ObjectType: "FUNC", ObjectName: "Z_FM"}
	callers := []CallerSource{
		{
			NodeID: "PROG:ZPROD", Name: "ZPROD", Type: "PROG", IsTest: false,
			Source: "REPORT zprod.\nCALL FUNCTION 'Z_FM'.\n",
		},
		{
			NodeID: "CLAS:ZCL_TEST", Name: "ZCL_TEST", Type: "CLAS", IsTest: true,
			Source: "CLASS zcl_test IMPLEMENTATION.\n  METHOD test.\n    CALL FUNCTION 'Z_FM'.\n  ENDMETHOD.\nENDCLASS.\n",
		},
	}

	result := FindUsageExamples(target, callers, 10)

	if len(result.Examples) < 2 {
		t.Fatalf("Expected 2 examples, got %d", len(result.Examples))
	}
	// Test class should be ranked first
	if !result.Examples[0].IsTest {
		t.Error("Test class example should be ranked first")
	}
}

func TestFindUsageExamples_MaxCap(t *testing.T) {
	target := UsageTarget{ObjectType: "FUNC", ObjectName: "Z_FM"}
	var callers []CallerSource
	for i := 0; i < 20; i++ {
		name := strings.ToUpper(fmt.Sprintf("ZPROG_%03d", i))
		callers = append(callers, CallerSource{
			NodeID: "PROG:" + name, Name: name, Type: "PROG",
			Source: fmt.Sprintf("REPORT %s.\nCALL FUNCTION 'Z_FM'.\n", strings.ToLower(name)),
		})
	}

	result := FindUsageExamples(target, callers, 5)

	if len(result.Examples) != 5 {
		t.Errorf("Max cap: got %d examples, want 5", len(result.Examples))
	}
	if result.TotalCallers != 20 {
		t.Errorf("TotalCallers should reflect all callers: got %d, want 20", result.TotalCallers)
	}
}

func TestFindUsageExamples_MultipleCallSitesInOneSource(t *testing.T) {
	target := UsageTarget{ObjectType: "FUNC", ObjectName: "Z_FM"}
	callers := []CallerSource{
		{
			NodeID: "PROG:ZPROG", Name: "ZPROG", Type: "PROG",
			Source: `REPORT zprog.
CALL FUNCTION 'Z_FM' EXPORTING iv_mode = 'A'.
DATA lv TYPE string.
CALL FUNCTION 'Z_FM' EXPORTING iv_mode = 'B'.`,
		},
	}

	result := FindUsageExamples(target, callers, 10)

	if len(result.Examples) != 2 {
		t.Errorf("Expected 2 examples (2 call sites in same source), got %d", len(result.Examples))
	}
}

func TestFindUsageExamples_SnippetContext(t *testing.T) {
	target := UsageTarget{ObjectType: "FUNC", ObjectName: "Z_FM"}
	callers := []CallerSource{
		{
			NodeID: "PROG:ZPROG", Name: "ZPROG", Type: "PROG",
			Source: "line1\nline2\nline3\nline4\nCALL FUNCTION 'Z_FM'.\nline6\nline7\nline8\nline9\n",
		},
	}

	result := FindUsageExamples(target, callers, 10)

	if len(result.Examples) != 1 {
		t.Fatalf("Expected 1 example, got %d", len(result.Examples))
	}
	snippet := result.Examples[0].Snippet
	// Should have 3 lines before + call line + 3 lines after = 7 lines
	snippetLines := strings.Split(strings.TrimRight(snippet, "\n"), "\n")
	if len(snippetLines) != 7 {
		t.Errorf("Snippet should have 7 lines (3 context + 1 match + 3 context), got %d", len(snippetLines))
	}
	// Should contain line numbers
	if !strings.Contains(snippet, "5 |") {
		t.Error("Snippet should contain line number 5")
	}
}

func TestIsTestCaller(t *testing.T) {
	tests := []struct {
		name     string
		inclType string
		want     bool
	}{
		{"ZCL_MY_TEST", "", true},
		{"LCL_TEST_HANDLER", "", true},
		{"LTH_HELPER", "", true},
		{"ZCL_PRODUCTION", "", false},
		{"ZCL_FOO", "CCAU", true},          // test include type
		{"ZCL_FOO", "testclasses", true},
		{"ZCL_FOO", "main", false},
	}
	for _, tt := range tests {
		if got := IsTestCaller(tt.name, tt.inclType); got != tt.want {
			t.Errorf("IsTestCaller(%q, %q) = %v, want %v", tt.name, tt.inclType, got, tt.want)
		}
	}
}
