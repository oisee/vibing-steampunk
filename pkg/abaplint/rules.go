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
	for _, row := range file.GetRawRows() {
		// Find : not followed by space (but not inside strings)
		for i := 0; i < len(row)-1; i++ {
			if row[i] == ':' && row[i+1] != ' ' && row[i+1] != '\n' {
				// Skip if inside string literal
				if isInStringLiteral(row, i) {
					continue
				}
				issues = append(issues, Issue{
					Key: r.GetKey(), Row: 0, Col: i + 1,
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
