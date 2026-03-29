package abaplint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// oracleFile matches the oracle.js JSON output.
type oracleFile struct {
	File       string        `json:"file"`
	TokenCount int           `json:"token_count"`
	Tokens     []oracleToken `json:"tokens"`
}

type oracleToken struct {
	Str  string `json:"str"`
	Type string `json:"type"`
	Row  int    `json:"row"`
	Col  int    `json:"col"`
}

func TestLexer_Basic(t *testing.T) {
	l := &Lexer{}
	tokens := l.Run("DATA lv_x TYPE i.")

	if len(tokens) == 0 {
		t.Fatal("no tokens produced")
	}

	// Should produce: DATA, lv_x, TYPE, i, .
	expected := []string{"DATA", "lv_x", "TYPE", "i", "."}
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}
	for i, e := range expected {
		if tokens[i].Str != e {
			t.Errorf("token %d: expected %q, got %q", i, e, tokens[i].Str)
		}
	}
}

func TestLexer_StringLiteral(t *testing.T) {
	l := &Lexer{}
	tokens := l.Run("WRITE 'hello world'.")

	strs := tokenStrs(tokens)
	expected := []string{"WRITE", "'hello world'", "."}
	assertStrs(t, strs, expected)

	// The string token should be StringToken type
	if tokens[1].Type != TokenString {
		t.Errorf("expected StringToken, got %s", tokens[1].Type)
	}
}

func TestLexer_Comment(t *testing.T) {
	l := &Lexer{}
	tokens := l.Run("* this is a comment\nDATA lv_x TYPE i.")

	if tokens[0].Type != TokenComment {
		t.Errorf("expected Comment, got %s", tokens[0].Type)
	}
	if tokens[0].Str != "* this is a comment" {
		t.Errorf("comment str: %q", tokens[0].Str)
	}
}

func TestLexer_StringTemplate(t *testing.T) {
	l := &Lexer{}
	tokens := l.Run("lv_x = |hello { lv_name } world|.")

	// Find template tokens
	var types []string
	for _, tok := range tokens {
		types = append(types, tok.Type.String())
	}
	t.Logf("tokens: %v", types)
	t.Logf("strs: %v", tokenStrs(tokens))
}

func TestLexer_Arrow(t *testing.T) {
	l := &Lexer{}
	tokens := l.Run("lo_obj->method( ).")

	strs := tokenStrs(tokens)
	t.Logf("tokens: %v", strs)
	// Should contain ->
	found := false
	for _, tok := range tokens {
		if tok.Str == "->" {
			found = true
			if tok.Type != TokenInstanceArrow && tok.Type != TokenInstanceArrowW &&
				tok.Type != TokenWInstanceArrow && tok.Type != TokenWInstanceArrowW {
				t.Errorf("arrow type: %s", tok.Type)
			}
		}
	}
	if !found {
		t.Error("no -> token found")
	}
}

// TestLexer_OracleDifferential is the core differential test.
// It compares Go lexer output against the TypeScript abaplint oracle.
func TestLexer_OracleDifferential(t *testing.T) {
	fixtureData, err := os.ReadFile("testdata/oracle_fixtures.json")
	if err != nil {
		t.Skipf("oracle fixtures not found: %v (run: node testdata/oracle.js ...)", err)
	}

	var fixtures []oracleFile
	if err := json.Unmarshal(fixtureData, &fixtures); err != nil {
		t.Fatalf("parse oracle JSON: %v", err)
	}

	// Find the ABAP source files to lex
	sourceDirs := []string{
		"../../embedded/abap",
		"../../abap-adt-api/testdata/src",
	}

	totalTokens := 0
	matchedTokens := 0
	strMatches := 0
	typeMatches := 0
	posMatches := 0
	totalFiles := 0
	passedFiles := 0

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

		l := &Lexer{}
		goTokens := l.Run(string(data))
		oracleTokens := fixture.Tokens

		// Compare token counts
		countMatch := len(goTokens) == len(oracleTokens)

		// Token-by-token comparison (up to min length)
		minLen := len(goTokens)
		if len(oracleTokens) < minLen {
			minLen = len(oracleTokens)
		}

		fileStrMatch := 0
		fileTypeMatch := 0
		filePosMatch := 0
		fileMismatches := 0

		for i := 0; i < minLen; i++ {
			goTok := goTokens[i]
			oTok := oracleTokens[i]

			totalTokens++

			sMatch := goTok.Str == oTok.Str
			tMatch := goTok.Type.String() == oTok.Type
			pMatch := goTok.Row == oTok.Row && goTok.Col == oTok.Col

			if sMatch {
				strMatches++
				fileStrMatch++
			}
			if tMatch {
				typeMatches++
				fileTypeMatch++
			}
			if pMatch {
				posMatches++
				filePosMatch++
			}
			if sMatch && tMatch && pMatch {
				matchedTokens++
			}

			if !sMatch || !tMatch {
				fileMismatches++
				if fileMismatches <= 5 {
					t.Logf("  [%s] token %d: go={%q %s r%d:c%d} oracle={%q %s r%d:c%d} str=%v type=%v pos=%v",
						fixture.File, i,
						goTok.Str, goTok.Type, goTok.Row, goTok.Col,
						oTok.Str, oTok.Type, oTok.Row, oTok.Col,
						sMatch, tMatch, pMatch)
				}
			}
		}

		// Count extra tokens as mismatches
		if len(goTokens) > minLen {
			totalTokens += len(goTokens) - minLen
		} else if len(oracleTokens) > minLen {
			totalTokens += len(oracleTokens) - minLen
		}

		fileTotal := max(len(goTokens), len(oracleTokens))
		filePerfect := fileMismatches == 0 && countMatch
		if filePerfect {
			passedFiles++
		}

		status := "PASS"
		if !filePerfect {
			status = "FAIL"
		}

		t.Logf("%s %s: go=%d oracle=%d str=%.1f%% type=%.1f%% pos=%.1f%% mismatches=%d",
			status, fixture.File,
			len(goTokens), len(oracleTokens),
			pct(fileStrMatch, fileTotal), pct(fileTypeMatch, fileTotal), pct(filePosMatch, fileTotal),
			fileMismatches)

		if fileMismatches > 5 {
			t.Logf("  ... and %d more mismatches", fileMismatches-5)
		}
	}

	// === KPI SUMMARY ===
	t.Logf("")
	t.Logf("=== DIFFERENTIAL KPI ===")
	t.Logf("Files:   %d/%d passed (%.1f%%)", passedFiles, totalFiles, pct(passedFiles, totalFiles))
	t.Logf("Tokens:  %d total", totalTokens)
	t.Logf("  Full match:  %d (%.1f%%)", matchedTokens, pct(matchedTokens, totalTokens))
	t.Logf("  Str match:   %d (%.1f%%)", strMatches, pct(strMatches, totalTokens))
	t.Logf("  Type match:  %d (%.1f%%)", typeMatches, pct(typeMatches, totalTokens))
	t.Logf("  Pos match:   %d (%.1f%%)", posMatches, pct(posMatches, totalTokens))
}

func findSourceFile(t *testing.T, name string, dirs []string) string {
	t.Helper()
	for _, dir := range dirs {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(n) / float64(total)
}

func tokenStrs(tokens []Token) []string {
	var strs []string
	for _, t := range tokens {
		strs = append(strs, t.Str)
	}
	return strs
}

func assertStrs(t *testing.T, got, expected []string) {
	t.Helper()
	if len(got) != len(expected) {
		t.Errorf("expected %d tokens %v, got %d tokens %v", len(expected), expected, len(got), got)
		return
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("token %d: expected %q, got %q", i, expected[i], got[i])
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// BenchmarkLexer benchmarks the Go lexer on a real ABAP file.
func BenchmarkLexer(b *testing.B) {
	// Use the largest embedded ABAP file
	data, err := os.ReadFile("../../embedded/abap/zcl_vsp_apc_handler.clas.abap")
	if err != nil {
		b.Skipf("source not found: %v", err)
	}
	source := string(data)
	l := &Lexer{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Run(source)
	}
	b.ReportMetric(float64(len(l.Run(source))), "tokens/op")
}

// TestLexer_OracleBulk runs the oracle comparison against the full ABAP corpus.
func TestLexer_OracleBulk(t *testing.T) {
	if os.Getenv("ABAPLINT_BULK") == "" {
		t.Skip("set ABAPLINT_BULK=1 to run bulk oracle comparison")
	}

	// Find all .abap files
	var files []string
	for _, dir := range []string{"../../embedded/abap", "../../abap-adt-api/testdata/src", "../../abap/src"} {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.abap"))
		files = append(files, matches...)
		// Also recurse one level
		subdirs, _ := filepath.Glob(filepath.Join(dir, "*"))
		for _, sd := range subdirs {
			matches, _ := filepath.Glob(filepath.Join(sd, "*.abap"))
			files = append(files, matches...)
		}
	}

	t.Logf("Found %d ABAP files for bulk comparison", len(files))

	l := &Lexer{}
	totalTokens := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		tokens := l.Run(string(data))
		totalTokens += len(tokens)
	}

	t.Logf("Total tokens across %d files: %d", len(files), totalTokens)
	t.Logf("(Generate oracle data with: node testdata/oracle.js <files...>)")
}

// TestLexer_EdgeCases tests tricky ABAP constructs.
func TestLexer_EdgeCases(t *testing.T) {
	l := &Lexer{}

	tests := []struct {
		name   string
		source string
		check  func(t *testing.T, tokens []Token)
	}{
		{
			name:   "pragma",
			source: "DATA lv_x TYPE i ##NEEDED.",
			check: func(t *testing.T, tokens []Token) {
				for _, tok := range tokens {
					if tok.Str == "##NEEDED" {
						if tok.Type != TokenPragma {
							t.Errorf("expected Pragma, got %s", tok.Type)
						}
						return
					}
				}
				t.Error("##NEEDED token not found")
			},
		},
		{
			name:   "backtick string",
			source: "lv_x = `hello world`.",
			check: func(t *testing.T, tokens []Token) {
				for _, tok := range tokens {
					if tok.Str == "`hello world`" {
						if tok.Type != TokenString {
							t.Errorf("expected StringToken, got %s", tok.Type)
						}
						return
					}
				}
				t.Errorf("backtick string not found, got: %v", tokenStrs(tokens))
			},
		},
		{
			name:   "static arrow",
			source: "zcl_class=>method( ).",
			check: func(t *testing.T, tokens []Token) {
				for _, tok := range tokens {
					if tok.Str == "=>" {
						return
					}
				}
				t.Errorf("=> not found, got: %v", tokenStrs(tokens))
			},
		},
		{
			name:   "inline comment",
			source: "DATA lv_x TYPE i. \" inline comment",
			check: func(t *testing.T, tokens []Token) {
				last := tokens[len(tokens)-1]
				if last.Type != TokenComment {
					t.Errorf("expected Comment, got %s for %q", last.Type, last.Str)
				}
			},
		},
		{
			name:   "chained colons",
			source: "DATA: lv_a TYPE i, lv_b TYPE string.",
			check: func(t *testing.T, tokens []Token) {
				// : should be Identifier (matching abaplint)
				if tokens[1].Str != ":" {
					t.Errorf("expected colon at pos 1, got %q", tokens[1].Str)
				}
			},
		},
		{
			name:   "empty source",
			source: "",
			check: func(t *testing.T, tokens []Token) {
				if len(tokens) != 0 {
					t.Errorf("expected 0 tokens for empty source, got %d", len(tokens))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := l.Run(tt.source)
			t.Logf("tokens: %v", formatTokens(tokens))
			tt.check(t, tokens)
		})
	}
}

func formatTokens(tokens []Token) string {
	s := ""
	for i, t := range tokens {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf("{%q %s r%d:c%d}", t.Str, t.Type, t.Row, t.Col)
	}
	return s
}
