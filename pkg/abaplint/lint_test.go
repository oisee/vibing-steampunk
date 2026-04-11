package abaplint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLinter_Basic(t *testing.T) {
	l := NewLinter()
	issues := l.Run("test.abap", `REPORT ztest.
DATA lv_x TYPE i.
lv_x = 42.
.
COMPUTE lv_x = lv_x + 1.
IF lv_x EQ 10.
ENDIF.`)

	for _, iss := range issues {
		t.Logf("  [%s] r%d:c%d %s (%s)", iss.Key, iss.Row, iss.Col, iss.Message, iss.Severity)
	}

	// Should find: empty_statement (the lone .), obsolete_statement (COMPUTE), preferred_compare_operator (EQ)
	found := map[string]bool{}
	for _, iss := range issues {
		found[iss.Key] = true
	}
	for _, key := range []string{"empty_statement", "obsolete_statement", "preferred_compare_operator"} {
		if !found[key] {
			t.Errorf("expected issue from rule %q", key)
		}
	}
}

func TestLinter_LineLength(t *testing.T) {
	l := &Linter{Rules: []Rule{&LineLengthRule{MaxLength: 20}}}
	issues := l.Run("test.abap", "DATA lv_very_long_variable_name TYPE string.")

	if len(issues) == 0 {
		t.Fatal("expected line_length issue")
	}
	if issues[0].Key != "line_length" {
		t.Errorf("expected line_length, got %s", issues[0].Key)
	}
}

func TestLinter_NamingConventions(t *testing.T) {
	l := &Linter{Rules: []Rule{&LocalVariableNamesRule{ExpectedData: `^[Ll][Vv]_\w+$`}}}
	source := `METHOD do_something.
  DATA lv_good TYPE i.
  DATA bad_name TYPE i.
ENDMETHOD.`
	issues := l.Run("test.abap", source)

	if len(issues) != 1 {
		t.Fatalf("expected 1 naming issue, got %d", len(issues))
	}
	if issues[0].Row == 0 {
		t.Error("issue should have a row number")
	}
	t.Logf("issue: %s at r%d", issues[0].Message, issues[0].Row)
}

func TestLinter_ObsoleteStatement(t *testing.T) {
	l := &Linter{Rules: []Rule{&ObsoleteStatementRule{
		Compute: true, Add: true, Move: true, Refresh: true,
	}}}
	source := `COMPUTE lv_x = 42.
ADD 1 TO lv_x.
MOVE lv_y TO lv_x.
REFRESH lt_table.
lv_x = 42.`
	issues := l.Run("test.abap", source)

	if len(issues) != 4 {
		t.Fatalf("expected 4 obsolete issues, got %d", len(issues))
	}
	for _, iss := range issues {
		t.Logf("  %s: %s", iss.Key, iss.Message)
	}
}

func TestLinter_MaxOneStatement(t *testing.T) {
	l := &Linter{Rules: []Rule{&MaxOneStatementRule{}}}
	source := "DATA lv_x TYPE i. DATA lv_y TYPE i."
	issues := l.Run("test.abap", source)

	if len(issues) == 0 {
		t.Fatal("expected max_one_statement issue")
	}
}

// TestLinter_RealCorpus runs the linter on real ABAP files and reports findings.
func TestLinter_RealCorpus(t *testing.T) {
	l := NewLinter()
	dirs := []string{"../../embedded/abap", "../../abap-adt-api/testdata/src"}

	totalIssues := 0
	ruleCount := map[string]int{}

	for _, dir := range dirs {
		files, _ := filepath.Glob(filepath.Join(dir, "*.abap"))
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			issues := l.Run(filepath.Base(f), string(data))
			totalIssues += len(issues)
			for _, iss := range issues {
				ruleCount[iss.Key]++
			}
		}
	}

	t.Logf("=== LINTER CORPUS RESULTS ===")
	t.Logf("Total issues: %d", totalIssues)
	for key, count := range ruleCount {
		t.Logf("  %-30s %d", key, count)
	}
}

// --- Oracle Differential Test ---

type oracleLintFile struct {
	File       string            `json:"file"`
	IssueCount int               `json:"issue_count"`
	Issues     []oracleLintIssue `json:"issues"`
}

type oracleLintIssue struct {
	Key     string `json:"key"`
	Message string `json:"message"`
	Row     int    `json:"row"`
	Col     int    `json:"col"`
}

// TestLinter_OracleDifferential compares Go linter output against TypeScript abaplint oracle.
// Compares only rules that both implement: empty_statement, obsolete_statement,
// preferred_compare_operator, line_length, max_one_statement.
func TestLinter_OracleDifferential(t *testing.T) {
	fixtureData, err := os.ReadFile("testdata/oracle_lint.json")
	if err != nil {
		t.Skipf("oracle lint fixtures not found: %v", err)
	}

	var fixtures []oracleLintFile
	if err := json.Unmarshal(fixtureData, &fixtures); err != nil {
		t.Fatalf("parse oracle JSON: %v", err)
	}

	// Use only the rules that match oracle config AND work at token/statement level.
	// preferred_compare_operator needs Expression-level AST matching (Phase 3) — excluded.
	goLinter := &Linter{Rules: []Rule{
		&LineLengthRule{MaxLength: 120},
		&EmptyStatementRule{},
		&ObsoleteStatementRule{
			Compute: true, Add: true, Subtract: true,
			Multiply: true, Divide: true, Move: true, Refresh: true,
		},
		&MaxOneStatementRule{},
	}}

	sharedRules := map[string]bool{
		"empty_statement":    true,
		"obsolete_statement": true,
		"line_length":        true,
		"max_one_statement":  true,
	}

	sourceDirs := []string{
		"../../embedded/abap",
		"../../abap-adt-api/testdata/src",
		"../../abap/src",
	}

	totalOracle := 0
	totalGo := 0
	matchedByKey := 0    // oracle issue found in Go output (same key+row)
	goOnlyByKey := 0     // Go found but oracle didn't
	oracleOnlyByKey := 0 // oracle found but Go didn't
	totalFiles := 0
	passedFiles := 0
	perRule := map[string][3]int{} // [matched, goOnly, oracleOnly]

	for _, fixture := range fixtures {
		source := findSourceFile(t, fixture.File, sourceDirs)
		if source == "" {
			continue
		}
		totalFiles++

		data, err := os.ReadFile(source)
		if err != nil {
			continue
		}

		goIssues := goLinter.Run(filepath.Base(source), string(data))

		// Filter to shared rules
		oracleSet := map[string]map[int]bool{} // key → set of rows
		for _, oi := range fixture.Issues {
			if !sharedRules[oi.Key] {
				continue
			}
			totalOracle++
			if oracleSet[oi.Key] == nil {
				oracleSet[oi.Key] = map[int]bool{}
			}
			oracleSet[oi.Key][oi.Row] = true
		}

		goSet := map[string]map[int]bool{}
		for _, gi := range goIssues {
			if !sharedRules[gi.Key] {
				continue
			}
			totalGo++
			if goSet[gi.Key] == nil {
				goSet[gi.Key] = map[int]bool{}
			}
			goSet[gi.Key][gi.Row] = true
		}

		fileMatch := 0
		fileGoOnly := 0
		fileOracleOnly := 0

		// Check Go issues against oracle
		for key, rows := range goSet {
			for row := range rows {
				if oracleSet[key] != nil && oracleSet[key][row] {
					matchedByKey++
					fileMatch++
					pr := perRule[key]
					pr[0]++
					perRule[key] = pr
				} else {
					goOnlyByKey++
					fileGoOnly++
					pr := perRule[key]
					pr[1]++
					perRule[key] = pr
				}
			}
		}

		// Check oracle issues not found by Go
		for key, rows := range oracleSet {
			for row := range rows {
				if goSet[key] == nil || !goSet[key][row] {
					oracleOnlyByKey++
					fileOracleOnly++
					pr := perRule[key]
					pr[2]++
					perRule[key] = pr
				}
			}
		}

		filePerfect := fileGoOnly == 0 && fileOracleOnly == 0
		if filePerfect {
			passedFiles++
		}

		status := "PASS"
		if !filePerfect {
			status = "FAIL"
		}

		oCount := 0
		for _, rows := range oracleSet {
			oCount += len(rows)
		}
		gCount := 0
		for _, rows := range goSet {
			gCount += len(rows)
		}

		if !filePerfect {
			t.Logf("%s %s: oracle=%d go=%d matched=%d goOnly=%d oracleOnly=%d",
				status, fixture.File, oCount, gCount, fileMatch, fileGoOnly, fileOracleOnly)
		}
	}

	t.Logf("")
	t.Logf("=== LINTER DIFFERENTIAL KPI ===")
	t.Logf("Files:       %d/%d passed (%.1f%%)", passedFiles, totalFiles, pct(passedFiles, totalFiles))
	t.Logf("Oracle issues: %d", totalOracle)
	t.Logf("Go issues:     %d", totalGo)
	t.Logf("Matched (key+row): %d (%.1f%% of oracle)", matchedByKey, pct(matchedByKey, totalOracle))
	t.Logf("Go-only:     %d", goOnlyByKey)
	t.Logf("Oracle-only: %d", oracleOnlyByKey)
	t.Logf("")
	t.Logf("Per-rule breakdown:")
	for key, counts := range perRule {
		t.Logf("  %-30s matched=%d goOnly=%d oracleOnly=%d", key, counts[0], counts[1], counts[2])
	}
}

// --- Tests for new security/quality rules ---

func TestColonMissingSpaceRule_RowNotZero(t *testing.T) {
	// Bug fix: Row was always 0 before the fix.
	l := &Linter{Rules: []Rule{&ColonMissingSpaceRule{}}}
	source := "REPORT ztest.\nDATA:lv_x TYPE i."
	issues := l.Run("test.abap", source)

	if len(issues) == 0 {
		t.Fatal("expected colon_missing_space issue")
	}
	if issues[0].Row == 0 {
		t.Errorf("Row should not be 0 (bug fix check): got Row=%d", issues[0].Row)
	}
	if issues[0].Row != 2 {
		t.Errorf("expected Row=2 (colon on line 2), got Row=%d", issues[0].Row)
	}
}

func TestColonMissingSpaceRule_NoIssueWhenSpacePresent(t *testing.T) {
	l := &Linter{Rules: []Rule{&ColonMissingSpaceRule{}}}
	source := "DATA: lv_x TYPE i."
	issues := l.Run("test.abap", source)
	if len(issues) != 0 {
		t.Errorf("expected no issues for proper colon spacing, got %d", len(issues))
	}
}

func TestSelectStarRule_Detect(t *testing.T) {
	l := &Linter{Rules: []Rule{&SelectStarRule{}}}

	t.Run("select_star", func(t *testing.T) {
		issues := l.Run("test.abap", "SELECT * FROM mara INTO TABLE @lt_mara.")
		if len(issues) == 0 {
			t.Fatal("expected select_star issue")
		}
		if issues[0].Key != "select_star" {
			t.Errorf("expected select_star, got %s", issues[0].Key)
		}
		if issues[0].Row == 0 {
			t.Errorf("Row should not be 0")
		}
	})

	t.Run("select_single_star", func(t *testing.T) {
		issues := l.Run("test.abap", "SELECT SINGLE * FROM mara INTO @ls_mara WHERE matnr = @lv_matnr.")
		if len(issues) == 0 {
			t.Fatal("expected select_star issue for SELECT SINGLE *")
		}
	})

	t.Run("explicit_fields_no_issue", func(t *testing.T) {
		issues := l.Run("test.abap", "SELECT matnr maktx FROM mara INTO TABLE @lt_mara.")
		if len(issues) != 0 {
			t.Errorf("expected no select_star for explicit fields, got %d issues", len(issues))
		}
	})
}

func TestHardcodedCredentialsRule_Detect(t *testing.T) {
	l := &Linter{Rules: []Rule{&HardcodedCredentialsRule{}}}

	t.Run("password_assignment", func(t *testing.T) {
		issues := l.Run("test.abap", "lv_password = 'SuperSecret123'.")
		if len(issues) == 0 {
			t.Fatal("expected hardcoded_credentials issue")
		}
		if issues[0].Key != "hardcoded_credentials" {
			t.Errorf("expected hardcoded_credentials, got %s", issues[0].Key)
		}
		if issues[0].Row == 0 {
			t.Errorf("Row should not be 0")
		}
	})

	t.Run("secret_assignment", func(t *testing.T) {
		issues := l.Run("test.abap", "lv_api_secret = 'abc123xyz'.")
		if len(issues) == 0 {
			t.Fatal("expected hardcoded_credentials for lv_api_secret")
		}
	})

	t.Run("non_credential_variable", func(t *testing.T) {
		issues := l.Run("test.abap", "lv_name = 'John Doe'.")
		if len(issues) != 0 {
			t.Errorf("expected no hardcoded_credentials for lv_name, got %d", len(issues))
		}
	})

	t.Run("empty_string_no_issue", func(t *testing.T) {
		// Very short strings treated as initial value, not hardcoded credential
		issues := l.Run("test.abap", "lv_password = ''.")
		if len(issues) != 0 {
			t.Errorf("expected no issue for empty password string, got %d", len(issues))
		}
	})
}

func TestCatchCxRootRule_Detect(t *testing.T) {
	l := &Linter{Rules: []Rule{&CatchCxRootRule{}}}

	t.Run("catch_cx_root", func(t *testing.T) {
		source := "TRY.\n  do_something( ).\nCATCH cx_root.\nENDTRY."
		issues := l.Run("test.abap", source)
		if len(issues) == 0 {
			t.Fatal("expected catch_cx_root issue")
		}
		if issues[0].Key != "catch_cx_root" {
			t.Errorf("expected catch_cx_root, got %s", issues[0].Key)
		}
		if issues[0].Row == 0 {
			t.Errorf("Row should not be 0, got Row=%d", issues[0].Row)
		}
	})

	t.Run("catch_cx_sy_prefix_no_issue", func(t *testing.T) {
		// CX_SY_* are specific system exceptions, not broad — should NOT trigger
		source := "TRY.\n  do_something( ).\nCATCH cx_sy_conversion_error.\nENDTRY."
		issues := l.Run("test.abap", source)
		if len(issues) != 0 {
			t.Errorf("expected no catch_cx_root for specific cx_sy_conversion_error, got %d", len(issues))
		}
	})

	t.Run("catch_cx_static_check", func(t *testing.T) {
		source := "TRY.\n  do_something( ).\nCATCH cx_static_check.\nENDTRY."
		issues := l.Run("test.abap", source)
		if len(issues) == 0 {
			t.Fatal("expected catch_cx_root issue for CX_STATIC_CHECK")
		}
	})

	t.Run("catch_cx_dynamic_check", func(t *testing.T) {
		source := "TRY.\n  do_something( ).\nCATCH cx_dynamic_check.\nENDTRY."
		issues := l.Run("test.abap", source)
		if len(issues) == 0 {
			t.Fatal("expected catch_cx_root issue for CX_DYNAMIC_CHECK")
		}
	})

	t.Run("catch_cx_no_check", func(t *testing.T) {
		source := "TRY.\n  do_something( ).\nCATCH cx_no_check.\nENDTRY."
		issues := l.Run("test.abap", source)
		if len(issues) == 0 {
			t.Fatal("expected catch_cx_root issue for CX_NO_CHECK")
		}
	})

	t.Run("specific_exception_no_issue", func(t *testing.T) {
		source := "TRY.\n  do_something( ).\nCATCH zcx_my_exception.\nENDTRY."
		issues := l.Run("test.abap", source)
		if len(issues) != 0 {
			t.Errorf("expected no catch_cx_root for specific zcx_my_exception, got %d", len(issues))
		}
	})
}

func TestCommitInLoopRule_Detect(t *testing.T) {
	l := &Linter{Rules: []Rule{&CommitInLoopRule{}}}

	t.Run("commit_inside_loop", func(t *testing.T) {
		source := "LOOP AT lt_items INTO DATA(ls_item).\n  COMMIT WORK.\nENDLOOP."
		issues := l.Run("test.abap", source)
		if len(issues) == 0 {
			t.Fatal("expected commit_in_loop issue")
		}
		if issues[0].Key != "commit_in_loop" {
			t.Errorf("expected commit_in_loop, got %s", issues[0].Key)
		}
		if issues[0].Row == 0 {
			t.Errorf("Row should not be 0")
		}
	})

	t.Run("commit_inside_do_loop", func(t *testing.T) {
		source := "DO 5 TIMES.\n  COMMIT WORK.\nENDDO."
		issues := l.Run("test.abap", source)
		if len(issues) == 0 {
			t.Fatal("expected commit_in_loop issue inside DO")
		}
	})

	t.Run("commit_outside_loop", func(t *testing.T) {
		source := "LOOP AT lt_items INTO DATA(ls_item).\n  process( ls_item ).\nENDLOOP.\nCOMMIT WORK."
		issues := l.Run("test.abap", source)
		if len(issues) != 0 {
			t.Errorf("expected no commit_in_loop when COMMIT is outside loop, got %d", len(issues))
		}
	})
}

func TestDynamicCallNoTryRule_Detect(t *testing.T) {
	l := &Linter{Rules: []Rule{&DynamicCallNoTryRule{}}}

	t.Run("dynamic_call_method_no_try", func(t *testing.T) {
		source := "CALL METHOD (lv_class)=>(lv_method)."
		issues := l.Run("test.abap", source)
		if len(issues) == 0 {
			t.Fatal("expected dynamic_call_no_try for CALL METHOD (var)")
		}
		if issues[0].Key != "dynamic_call_no_try" {
			t.Errorf("expected dynamic_call_no_try, got %s", issues[0].Key)
		}
		if issues[0].Row == 0 {
			t.Errorf("Row should not be 0")
		}
	})

	t.Run("dynamic_call_function_no_try", func(t *testing.T) {
		source := "CALL FUNCTION lv_func_name."
		issues := l.Run("test.abap", source)
		if len(issues) == 0 {
			t.Fatal("expected dynamic_call_no_try for CALL FUNCTION variable")
		}
	})

	t.Run("static_call_function_no_issue", func(t *testing.T) {
		source := "CALL FUNCTION 'SOME_FUNCTION_MODULE'."
		issues := l.Run("test.abap", source)
		if len(issues) != 0 {
			t.Errorf("expected no issue for static CALL FUNCTION, got %d", len(issues))
		}
	})

	t.Run("dynamic_call_inside_try_no_issue", func(t *testing.T) {
		source := "TRY.\n  CALL METHOD (lv_class)=>(lv_method).\nCATCH cx_sy_dyn_call_error.\nENDTRY."
		issues := l.Run("test.abap", source)
		if len(issues) != 0 {
			t.Errorf("expected no issue when dynamic call is inside TRY, got %d", len(issues))
		}
	})
}

func BenchmarkLinter(b *testing.B) {
	data, err := os.ReadFile("../../embedded/abap/zcl_vsp_apc_handler.clas.abap")
	if err != nil {
		b.Skipf("file not found: %v", err)
	}
	source := string(data)
	l := NewLinter()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Run("test.abap", source)
	}
}
