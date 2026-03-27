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
