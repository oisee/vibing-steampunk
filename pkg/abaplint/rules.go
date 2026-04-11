package abaplint

import (
	"fmt"
	"regexp"
	"strings"
)

// --- line_length (63 lines TS) ---

type LineLengthRule struct {
	MaxLength int // default 120
}

func (r *LineLengthRule) GetKey() string { return "line_length" }

func (r *LineLengthRule) Run(file *ABAPFile) []Issue {
	maxLen := r.MaxLength
	if maxLen <= 0 {
		maxLen = 120
	}
	var issues []Issue
	for i, row := range file.GetRawRows() {
		line := strings.TrimRight(row, "\r")
		if len(line) > 255 {
			issues = append(issues, Issue{
				Key: r.GetKey(), Row: i + 1, Col: 1,
				Message:  fmt.Sprintf("Maximum allowed line length of 255 exceeded, currently %d", len(line)),
				Severity: "Error",
			})
		} else if len(line) > maxLen {
			issues = append(issues, Issue{
				Key: r.GetKey(), Row: i + 1, Col: 1,
				Message:  fmt.Sprintf("Reduce line length to max %d, currently %d", maxLen, len(line)),
				Severity: "Warning",
			})
		}
		if len(issues) >= 10 {
			break
		}
	}
	return issues
}

// --- empty_statement (56 lines TS) ---

type EmptyStatementRule struct{}

func (r *EmptyStatementRule) GetKey() string { return "empty_statement" }

func (r *EmptyStatementRule) Run(file *ABAPFile) []Issue {
	var issues []Issue
	for _, stmt := range file.GetStatements() {
		if stmt.Type == "Empty" {
			tok := stmt.Tokens[0]
			issues = append(issues, Issue{
				Key: r.GetKey(), Row: tok.Row, Col: tok.Col,
				Message:  "Remove empty statement",
				Severity: "Error",
			})
		}
	}
	return issues
}

// --- max_one_statement (76 lines TS) ---

type MaxOneStatementRule struct{}

func (r *MaxOneStatementRule) GetKey() string { return "max_one_statement" }

func (r *MaxOneStatementRule) Run(file *ABAPFile) []Issue {
	var issues []Issue
	// Track which rows already have a statement ending (period/comma)
	rowHasEnd := map[int]bool{}
	for _, stmt := range file.GetStatements() {
		if stmt.Type == "Comment" || stmt.Type == "Empty" || stmt.Type == "NativeSQL" {
			continue
		}
		if len(stmt.Tokens) == 0 {
			continue
		}
		// Skip chained statements (colon chains are one logical group)
		if stmt.Colon != nil {
			continue
		}
		// The end row is the last token (period)
		lastRow := stmt.Tokens[len(stmt.Tokens)-1].Row
		// The start row is the first real token
		firstRow := stmt.Tokens[0].Row

		// Check if another statement already ended on first token's row
		if rowHasEnd[firstRow] {
			issues = append(issues, Issue{
				Key: r.GetKey(), Row: firstRow, Col: stmt.Tokens[0].Col,
				Message:  "Only one statement per line",
				Severity: "Error",
			})
		}
		rowHasEnd[lastRow] = true
	}
	return issues
}

// --- preferred_compare_operator (99 lines TS) ---

type PreferredCompareOperatorRule struct {
	BadOperators []string // e.g. ["EQ", "NE", "GT", "LT", "GE", "LE", "><"]
}

func (r *PreferredCompareOperatorRule) GetKey() string { return "preferred_compare_operator" }

func (r *PreferredCompareOperatorRule) Run(file *ABAPFile) []Issue {
	bad := make(map[string]string)
	replacements := map[string]string{
		"EQ": "=", "NE": "!=", "><": "!=",
		"GT": ">", "LT": "<", "GE": ">=", "LE": "<=",
	}
	for _, op := range r.BadOperators {
		bad[strings.ToUpper(op)] = replacements[strings.ToUpper(op)]
	}

	// Only check inside conditional statements
	condStmts := map[string]bool{
		"If": true, "ElseIf": true, "While": true,
		"Check": true,
	}

	var issues []Issue
	for _, stmt := range file.GetStatements() {
		if !condStmts[stmt.Type] {
			continue
		}
		for _, tok := range stmt.Tokens {
			upper := strings.ToUpper(tok.Str)
			if repl, ok := bad[upper]; ok {
				issues = append(issues, Issue{
					Key: r.GetKey(), Row: tok.Row, Col: tok.Col,
					Message:  fmt.Sprintf("Use %q instead of %q", repl, tok.Str),
					Severity: "Error",
				})
			}
		}
	}
	return issues
}

// --- obsolete_statement (503 lines TS — simplified) ---

type ObsoleteStatementRule struct {
	Compute  bool
	Add      bool
	Subtract bool
	Multiply bool
	Divide   bool
	Move     bool
	Refresh  bool
}

func (r *ObsoleteStatementRule) GetKey() string { return "obsolete_statement" }

func (r *ObsoleteStatementRule) Run(file *ABAPFile) []Issue {
	var issues []Issue
	for _, stmt := range file.GetStatements() {
		if stmt.Type == "Comment" || stmt.Type == "Empty" || len(stmt.Tokens) == 0 {
			continue
		}
		first := strings.ToUpper(stmt.Tokens[0].Str)
		tok := stmt.Tokens[0]
		switch {
		case r.Compute && first == "COMPUTE":
			issues = append(issues, Issue{Key: r.GetKey(), Row: tok.Row, Col: tok.Col,
				Message: "COMPUTE is obsolete, use direct assignment", Severity: "Warning"})
		case r.Add && first == "ADD":
			issues = append(issues, Issue{Key: r.GetKey(), Row: tok.Row, Col: tok.Col,
				Message: "ADD is obsolete, use += operator", Severity: "Warning"})
		case r.Subtract && first == "SUBTRACT":
			issues = append(issues, Issue{Key: r.GetKey(), Row: tok.Row, Col: tok.Col,
				Message: "SUBTRACT is obsolete, use -= operator", Severity: "Warning"})
		case r.Multiply && first == "MULTIPLY":
			issues = append(issues, Issue{Key: r.GetKey(), Row: tok.Row, Col: tok.Col,
				Message: "MULTIPLY is obsolete, use *= operator", Severity: "Warning"})
		case r.Divide && first == "DIVIDE":
			issues = append(issues, Issue{Key: r.GetKey(), Row: tok.Row, Col: tok.Col,
				Message: "DIVIDE is obsolete, use /= operator", Severity: "Warning"})
		case r.Move && first == "MOVE":
			issues = append(issues, Issue{Key: r.GetKey(), Row: tok.Row, Col: tok.Col,
				Message: "MOVE is obsolete, use direct assignment", Severity: "Warning"})
		case r.Refresh && first == "REFRESH":
			issues = append(issues, Issue{Key: r.GetKey(), Row: tok.Row, Col: tok.Col,
				Message: "REFRESH is obsolete, use CLEAR", Severity: "Warning"})
		}
	}
	return issues
}

// --- colon_missing_space ---

type ColonMissingSpaceRule struct{}

func (r *ColonMissingSpaceRule) GetKey() string { return "colon_missing_space" }

func (r *ColonMissingSpaceRule) Run(file *ABAPFile) []Issue {
	var issues []Issue
	for rowIdx, row := range file.GetRawRows() {
		lineNum := rowIdx + 1 // convert to 1-based
		// Find : not followed by space (but not inside strings)
		for i := 0; i < len(row)-1; i++ {
			if row[i] == ':' && row[i+1] != ' ' && row[i+1] != '\n' {
				// Skip if inside string literal
				if isInStringLiteral(row, i) {
					continue
				}
				issues = append(issues, Issue{
					Key: r.GetKey(), Row: lineNum, Col: i + 1,
					Message:  "Missing space after colon",
					Severity: "Warning",
				})
				break // one per line
			}
		}
	}
	return issues
}

// --- double_space ---

type DoubleSpaceRule struct {
	AfterKeywords bool
}

func (r *DoubleSpaceRule) GetKey() string { return "double_space" }

func (r *DoubleSpaceRule) Run(file *ABAPFile) []Issue {
	var issues []Issue
	for i, row := range file.GetRawRows() {
		trimmed := strings.TrimRight(row, " \t\r")
		if strings.HasPrefix(strings.TrimSpace(trimmed), "*") {
			continue // skip comments
		}
		if strings.HasPrefix(strings.TrimSpace(trimmed), "\"") {
			continue // skip inline comments
		}
		// Check for double spaces in code area (before any inline comment)
		codePart := trimmed
		if idx := strings.Index(trimmed, "\""); idx > 0 {
			codePart = trimmed[:idx]
		}
		// Skip leading whitespace
		leadingDone := false
		for j := 0; j < len(codePart)-1; j++ {
			if !leadingDone {
				if codePart[j] != ' ' && codePart[j] != '\t' {
					leadingDone = true
				}
				continue
			}
			if codePart[j] == ' ' && codePart[j+1] == ' ' {
				issues = append(issues, Issue{
					Key: r.GetKey(), Row: i + 1, Col: j + 1,
					Message:  "Remove double space",
					Severity: "Warning",
				})
				break // one per line
			}
		}
	}
	return issues
}

// --- local_variable_names ---

type LocalVariableNamesRule struct {
	ExpectedData     string // regex pattern, e.g. "^[Ll][Vv]_\\w+$"
	ExpectedConstant string
	ExpectedFS       string
}

func (r *LocalVariableNamesRule) GetKey() string { return "local_variable_names" }

func (r *LocalVariableNamesRule) Run(file *ABAPFile) []Issue {
	var issues []Issue
	inLocal := false // inside FORM/METHOD/FUNCTION

	dataRe := compilePattern(r.ExpectedData)
	constRe := compilePattern(r.ExpectedConstant)
	fsRe := compilePattern(r.ExpectedFS)

	for _, stmt := range file.GetStatements() {
		switch stmt.Type {
		case "MethodImplementation", "Form", "FunctionModule":
			inLocal = true
		case "EndMethod", "EndForm", "EndFunction":
			inLocal = false
		}
		if !inLocal {
			continue
		}

		switch stmt.Type {
		case "Data":
			// DATA lv_x TYPE ...
			if len(stmt.Tokens) >= 2 && dataRe != nil {
				name := stmt.Tokens[1].Str
				if !dataRe.MatchString(name) {
					issues = append(issues, Issue{
						Key: r.GetKey(), Row: stmt.Tokens[1].Row, Col: stmt.Tokens[1].Col,
						Message:  fmt.Sprintf("Local variable name %q does not match pattern %s", name, r.ExpectedData),
						Severity: "Warning",
					})
				}
			}
		case "Constant":
			if len(stmt.Tokens) >= 2 && constRe != nil {
				name := stmt.Tokens[1].Str
				if !constRe.MatchString(name) {
					issues = append(issues, Issue{
						Key: r.GetKey(), Row: stmt.Tokens[1].Row, Col: stmt.Tokens[1].Col,
						Message:  fmt.Sprintf("Local constant name %q does not match pattern %s", name, r.ExpectedConstant),
						Severity: "Warning",
					})
				}
			}
		case "FieldSymbol":
			// FIELD-SYMBOLS <lv_x> TYPE ...
			if len(stmt.Tokens) >= 3 && fsRe != nil {
				name := stmt.Tokens[2].Str // after FIELD - SYMBOLS
				if !fsRe.MatchString(name) {
					issues = append(issues, Issue{
						Key: r.GetKey(), Row: stmt.Tokens[2].Row, Col: stmt.Tokens[2].Col,
						Message:  fmt.Sprintf("Field symbol name %q does not match pattern %s", name, r.ExpectedFS),
						Severity: "Warning",
					})
				}
			}
		}
	}
	return issues
}

func compilePattern(pattern string) *regexp.Regexp {
	if pattern == "" {
		return nil
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil
	}
	return re
}

func isInStringLiteral(line string, pos int) bool {
	inSingle := false
	inBacktick := false
	for i := 0; i < pos; i++ {
		switch line[i] {
		case '\'':
			if !inBacktick {
				inSingle = !inSingle
			}
		case '`':
			if !inSingle {
				inBacktick = !inBacktick
			}
		}
	}
	return inSingle || inBacktick
}

// --- select_star ---

// SelectStarRule detects SELECT * and SELECT SINGLE * statements.
// Fetching all columns is wasteful; explicit field lists improve performance.
type SelectStarRule struct{}

func (r *SelectStarRule) GetKey() string { return "select_star" }

func (r *SelectStarRule) Run(file *ABAPFile) []Issue {
	var issues []Issue
	for _, stmt := range file.GetStatements() {
		if stmt.Type == "Comment" || stmt.Type == "Empty" || len(stmt.Tokens) == 0 {
			continue
		}
		if strings.ToUpper(stmt.Tokens[0].Str) != "SELECT" {
			continue
		}
		// Skip optional SINGLE/DISTINCT after SELECT
		i := 1
		for i < len(stmt.Tokens) {
			u := strings.ToUpper(stmt.Tokens[i].Str)
			if u == "SINGLE" || u == "DISTINCT" {
				i++
				continue
			}
			break
		}
		// Only flag if * is the first field (not COUNT(*) or similar)
		if i < len(stmt.Tokens) && stmt.Tokens[i].Str == "*" {
			tok := stmt.Tokens[i]
			issues = append(issues, Issue{
				Key: r.GetKey(), Row: tok.Row, Col: tok.Col,
				Message:  "SELECT * fetches all columns — use an explicit field list",
				Severity: "Warning",
			})
		}
	}
	return issues
}

// --- hardcoded_credentials ---

// HardcodedCredentialsRule detects assignments of string literals to variables
// whose names suggest credentials (password, secret, key, token).
type HardcodedCredentialsRule struct{}

func (r *HardcodedCredentialsRule) GetKey() string { return "hardcoded_credentials" }

// credentialNames contains lowercase substrings that indicate credential variables.
var credentialNames = []string{"password", "passwd", "secret", "api_key", "apikey", "auth_token", "access_token", "bearer_token", "refresh_token", "api_token"}

func (r *HardcodedCredentialsRule) Run(file *ABAPFile) []Issue {
	var issues []Issue
	for _, stmt := range file.GetStatements() {
		if stmt.Type == "Comment" || stmt.Type == "Empty" || len(stmt.Tokens) < 3 {
			continue
		}
		// Pattern: <varname> = '<literal>' or <varname> = `<literal>`
		// Token layout: [varname] [=] [string-literal] [.]
		assignIdx := -1
		for i := 1; i < len(stmt.Tokens); i++ {
			if stmt.Tokens[i].Str == "=" {
				assignIdx = i
				break
			}
		}
		if assignIdx < 1 || assignIdx >= len(stmt.Tokens)-1 {
			continue
		}
		varName := strings.ToLower(stmt.Tokens[assignIdx-1].Str)
		isCred := false
		for _, cred := range credentialNames {
			if strings.Contains(varName, cred) {
				isCred = true
				break
			}
		}
		if !isCred {
			continue
		}
		rhsTok := stmt.Tokens[assignIdx+1]
		if rhsTok.Type != TokenString && rhsTok.Type != TokenStringTemplate &&
			rhsTok.Type != TokenStringTemplateBegin {
			continue
		}
		// Ignore empty or very short literals (e.g. '' or '' used as initial value)
		if len(rhsTok.Str) <= 3 {
			continue
		}
		issues = append(issues, Issue{
			Key: r.GetKey(), Row: rhsTok.Row, Col: rhsTok.Col,
			Message:  fmt.Sprintf("Hardcoded credential in assignment to %q", stmt.Tokens[assignIdx-1].Str),
			Severity: "Error",
		})
	}
	return issues
}

// --- catch_cx_root ---

// CatchCxRootRule detects CATCH with overly broad exception classes
// (CX_ROOT, CX_STATIC_CHECK, CX_DYNAMIC_CHECK, CX_NO_CHECK).
type CatchCxRootRule struct{}

func (r *CatchCxRootRule) GetKey() string { return "catch_cx_root" }

// broadExceptions is the set of exception classes that are too broad to catch.
var broadExceptions = map[string]bool{
	"CX_ROOT":          true,
	"CX_STATIC_CHECK":  true,
	"CX_DYNAMIC_CHECK": true,
	"CX_NO_CHECK":      true,
}


func (r *CatchCxRootRule) Run(file *ABAPFile) []Issue {
	var issues []Issue
	for _, stmt := range file.GetStatements() {
		if stmt.Type == "Comment" || stmt.Type == "Empty" || len(stmt.Tokens) == 0 {
			continue
		}
		if strings.ToUpper(stmt.Tokens[0].Str) != "CATCH" {
			continue
		}
		// Check each exception class token after CATCH
		for i := 1; i < len(stmt.Tokens); i++ {
			tok := stmt.Tokens[i]
			if tok.Type == TokenPunctuation {
				break
			}
			upper := strings.ToUpper(tok.Str)
			if broadExceptions[upper] {
				issues = append(issues, Issue{
					Key: r.GetKey(), Row: tok.Row, Col: tok.Col,
					Message:  fmt.Sprintf("Catching broad exception %s — use specific exception classes", tok.Str),
					Severity: "Warning",
				})
				break
			}
		}
	}
	return issues
}

// --- commit_in_loop ---

// CommitInLoopRule detects COMMIT WORK inside LOOP/DO/WHILE blocks.
// It tracks nesting depth to determine whether COMMIT occurs inside a loop body.
type CommitInLoopRule struct{}

func (r *CommitInLoopRule) GetKey() string { return "commit_in_loop" }

func (r *CommitInLoopRule) Run(file *ABAPFile) []Issue {
	var issues []Issue
	loopDepth := 0

	for _, stmt := range file.GetStatements() {
		if stmt.Type == "Comment" || stmt.Type == "Empty" || len(stmt.Tokens) == 0 {
			continue
		}
		first := strings.ToUpper(stmt.Tokens[0].Str)

		// Track loop entry
		switch first {
		case "LOOP", "DO", "WHILE":
			loopDepth++
		case "ENDLOOP", "ENDDO", "ENDWHILE":
			if loopDepth > 0 {
				loopDepth--
			}
		}

		if loopDepth == 0 || first != "COMMIT" {
			continue
		}
		// Confirm "COMMIT WORK" by checking second token
		if len(stmt.Tokens) >= 2 && strings.ToUpper(stmt.Tokens[1].Str) == "WORK" {
			tok := stmt.Tokens[0]
			issues = append(issues, Issue{
				Key: r.GetKey(), Row: tok.Row, Col: tok.Col,
				Message:  "COMMIT WORK inside loop — destroys transactional integrity",
				Severity: "Error",
			})
		}
	}
	return issues
}

// --- dynamic_call_no_try ---

// DynamicCallNoTryRule detects dynamic CALL METHOD (var) or CALL FUNCTION var
// without a surrounding TRY/ENDTRY block.
type DynamicCallNoTryRule struct{}

func (r *DynamicCallNoTryRule) GetKey() string { return "dynamic_call_no_try" }

func (r *DynamicCallNoTryRule) Run(file *ABAPFile) []Issue {
	var issues []Issue
	tryDepth := 0

	for _, stmt := range file.GetStatements() {
		if stmt.Type == "Comment" || stmt.Type == "Empty" || len(stmt.Tokens) == 0 {
			continue
		}
		first := strings.ToUpper(stmt.Tokens[0].Str)

		// Track TRY/ENDTRY nesting
		if first == "TRY" {
			tryDepth++
		} else if first == "ENDTRY" {
			if tryDepth > 0 {
				tryDepth--
			}
		}

		if first != "CALL" || len(stmt.Tokens) < 2 {
			continue
		}
		second := strings.ToUpper(stmt.Tokens[1].Str)

		isDynamic := false
		switch second {
		case "METHOD":
			// Dynamic: CALL METHOD (var)=>method_name
			// The third token is "(" when the class is dynamic
			if len(stmt.Tokens) >= 3 && stmt.Tokens[2].Str == "(" {
				isDynamic = true
			}
		case "FUNCTION":
			// Dynamic: CALL FUNCTION lv_name (variable, not a string literal)
			if len(stmt.Tokens) >= 3 && stmt.Tokens[2].Type != TokenString {
				isDynamic = true
			}
		}

		if !isDynamic || tryDepth > 0 {
			continue
		}
		tok := stmt.Tokens[0]
		issues = append(issues, Issue{
			Key: r.GetKey(), Row: tok.Row, Col: tok.Col,
			Message:  fmt.Sprintf("Dynamic CALL %s without surrounding TRY — runtime crash if target not found", second),
			Severity: "Warning",
		})
	}
	return issues
}
