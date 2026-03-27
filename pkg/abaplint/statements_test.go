package abaplint

import (
	"encoding/json"
	"os"
	"testing"
)

type oracleStmtFile struct {
	File           string             `json:"file"`
	StatementCount int                `json:"statement_count"`
	Statements     []oracleStatement  `json:"statements"`
}

type oracleStatement struct {
	Type   string   `json:"type"`
	Tokens []string `json:"tokens"`
	Colon  bool     `json:"colon"`
}

func TestStatementParser_Basic(t *testing.T) {
	l := &Lexer{}
	p := &StatementParser{}

	tokens := l.Run("DATA lv_x TYPE i.")
	stmts := p.Parse(tokens)

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if stmts[0].ConcatTokens() != "DATA lv_x TYPE i ." {
		t.Errorf("unexpected tokens: %q", stmts[0].ConcatTokens())
	}
}

func TestStatementParser_ColonChaining(t *testing.T) {
	l := &Lexer{}
	p := &StatementParser{}

	tokens := l.Run("DATA: lv_x TYPE i, lv_y TYPE string.")
	stmts := p.Parse(tokens)

	if len(stmts) != 2 {
		t.Fatalf("expected 2 chained statements, got %d", len(stmts))
		return
	}

	// First: DATA lv_x TYPE i ,
	s0 := stmtTokenStrs(stmts[0])
	if s0[0] != "DATA" || s0[1] != "lv_x" {
		t.Errorf("stmt 0: %v", s0)
	}
	if stmts[0].Colon == nil {
		t.Error("stmt 0 should have colon")
	}

	// Second: DATA lv_y TYPE string .
	s1 := stmtTokenStrs(stmts[1])
	if s1[0] != "DATA" || s1[1] != "lv_y" {
		t.Errorf("stmt 1: %v", s1)
	}
}

func TestStatementParser_Comment(t *testing.T) {
	l := &Lexer{}
	p := &StatementParser{}

	tokens := l.Run("* this is a comment\nDATA lv_x TYPE i.")
	stmts := p.Parse(tokens)

	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(stmts))
	}
	if stmts[0].Type != "Comment" {
		t.Errorf("expected Comment, got %s", stmts[0].Type)
	}
}

func TestStatementParser_Empty(t *testing.T) {
	l := &Lexer{}
	p := &StatementParser{}

	// Lone period = empty statement
	tokens := l.Run(".")
	stmts := p.Parse(tokens)

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if stmts[0].Type != "Empty" {
		t.Errorf("expected Empty, got %s", stmts[0].Type)
	}
}

// TestStatementMatcher_OracleDifferential compares Go statement TYPE classification against oracle.
func TestStatementMatcher_OracleDifferential(t *testing.T) {
	fixtureData, err := os.ReadFile("testdata/oracle_stmts.json")
	if err != nil {
		t.Skipf("oracle statement fixtures not found: %v", err)
	}

	var fixtures []oracleStmtFile
	if err := json.Unmarshal(fixtureData, &fixtures); err != nil {
		t.Fatalf("parse oracle JSON: %v", err)
	}

	sourceDirs := []string{
		"../../embedded/abap",
		"../../abap-adt-api/testdata/src",
		"../../abap/src",
	}

	lex := &Lexer{}
	parser := &StatementParser{}
	matcher := NewStatementMatcher()

	totalStmts := 0
	typeMatches := 0
	typeMismatches := make(map[string]int) // "got→expected" → count

	for _, fixture := range fixtures {
		source := findSourceFile(t, fixture.File, sourceDirs)
		if source == "" {
			continue
		}

		data, err := os.ReadFile(source)
		if err != nil {
			continue
		}

		tokens := lex.Run(string(data))
		goStmts := parser.Parse(tokens)
		matcher.ClassifyStatements(goStmts)
		oracleStmts := fixture.Statements

		minLen := len(goStmts)
		if len(oracleStmts) < minLen {
			minLen = len(oracleStmts)
		}

		for i := 0; i < minLen; i++ {
			totalStmts++
			goType := goStmts[i].Type
			oType := oracleStmts[i].Type

			if goType == oType {
				typeMatches++
			} else {
				key := goType + "→" + oType
				typeMismatches[key]++
				if typeMismatches[key] <= 3 {
					t.Logf("  %s→%s: %s", goType, oType, goStmts[i].ConcatTokens())
				}
			}
		}
	}

	t.Logf("")
	t.Logf("=== STATEMENT TYPE KPI ===")
	t.Logf("Statements: %d total", totalStmts)
	t.Logf("Type match: %d (%.1f%%)", typeMatches, pct(typeMatches, totalStmts))
	t.Logf("")

	// Show top mismatches
	type mismatch struct {
		key   string
		count int
	}
	var mm []mismatch
	for k, v := range typeMismatches {
		mm = append(mm, mismatch{k, v})
	}
	// Sort by count desc
	for i := 0; i < len(mm); i++ {
		for j := i + 1; j < len(mm); j++ {
			if mm[j].count > mm[i].count {
				mm[i], mm[j] = mm[j], mm[i]
			}
		}
	}
	t.Logf("Top mismatches (go→oracle):")
	for i, m := range mm {
		if i >= 15 {
			break
		}
		t.Logf("  %4d  %s", m.count, m.key)
	}
}

// TestStatementParser_OracleDifferential compares Go statement splitter against abaplint oracle.
func TestStatementParser_OracleDifferential(t *testing.T) {
	fixtureData, err := os.ReadFile("testdata/oracle_stmts.json")
	if err != nil {
		t.Skipf("oracle statement fixtures not found: %v", err)
	}

	var fixtures []oracleStmtFile
	if err := json.Unmarshal(fixtureData, &fixtures); err != nil {
		t.Fatalf("parse oracle JSON: %v", err)
	}

	sourceDirs := []string{
		"../../embedded/abap",
		"../../abap-adt-api/testdata/src",
		"../../abap/src",
	}

	totalStmts := 0
	matchedStmts := 0
	tokenMatchStmts := 0
	colonMatchStmts := 0
	totalFiles := 0
	passedFiles := 0

	lex := &Lexer{}
	parser := &StatementParser{}

	for _, fixture := range fixtures {
		source := findSourceFile(t, fixture.File, sourceDirs)
		if source == "" {
			t.Logf("SKIP %s: source file not found", fixture.File)
			continue
		}

		totalFiles++
		data, err := os.ReadFile(source)
		if err != nil {
			t.Errorf("read %s: %v", source, err)
			continue
		}

		tokens := lex.Run(string(data))
		goStmts := parser.Parse(tokens)
		oracleStmts := fixture.Statements

		countMatch := len(goStmts) == len(oracleStmts)

		minLen := len(goStmts)
		if len(oracleStmts) < minLen {
			minLen = len(oracleStmts)
		}

		fileMismatches := 0
		fileTokenMatch := 0
		fileColonMatch := 0

		for i := 0; i < minLen; i++ {
			goS := goStmts[i]
			oS := oracleStmts[i]

			totalStmts++

			// Compare token strings
			goTokens := stmtTokenStrs(goS)
			tokMatch := strSliceEqual(goTokens, oS.Tokens)
			if tokMatch {
				tokenMatchStmts++
				fileTokenMatch++
			}

			// Compare colon
			goColon := goS.Colon != nil
			colonMatch := goColon == oS.Colon
			if colonMatch {
				colonMatchStmts++
				fileColonMatch++
			}

			if tokMatch && colonMatch {
				matchedStmts++
			}

			if !tokMatch {
				fileMismatches++
				if fileMismatches <= 3 {
					t.Logf("  [%s] stmt %d: go=%v oracle=%v colon_go=%v colon_oracle=%v",
						fixture.File, i, goTokens, oS.Tokens, goColon, oS.Colon)
				}
			}
		}

		// Count extra statements
		extraStmts := abs(len(goStmts) - len(oracleStmts))
		totalStmts += extraStmts

		fileTotal := max(len(goStmts), len(oracleStmts))
		filePerfect := fileMismatches == 0 && countMatch
		if filePerfect {
			passedFiles++
		}

		status := "PASS"
		if !filePerfect {
			status = "FAIL"
		}

		t.Logf("%s %s: go=%d oracle=%d tokens=%.1f%% colon=%.1f%% mismatches=%d",
			status, fixture.File,
			len(goStmts), len(oracleStmts),
			pct(fileTokenMatch, fileTotal), pct(fileColonMatch, fileTotal),
			fileMismatches)

		if fileMismatches > 3 {
			t.Logf("  ... and %d more mismatches", fileMismatches-3)
		}
	}

	t.Logf("")
	t.Logf("=== STATEMENT SPLITTER KPI ===")
	t.Logf("Files:       %d/%d passed (%.1f%%)", passedFiles, totalFiles, pct(passedFiles, totalFiles))
	t.Logf("Statements:  %d total", totalStmts)
	t.Logf("  Full match:   %d (%.1f%%)", matchedStmts, pct(matchedStmts, totalStmts))
	t.Logf("  Token match:  %d (%.1f%%)", tokenMatchStmts, pct(tokenMatchStmts, totalStmts))
	t.Logf("  Colon match:  %d (%.1f%%)", colonMatchStmts, pct(colonMatchStmts, totalStmts))
}

func stmtTokenStrs(s Statement) []string {
	var strs []string
	for _, t := range s.Tokens {
		strs = append(strs, t.Str)
	}
	return strs
}

func strSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
