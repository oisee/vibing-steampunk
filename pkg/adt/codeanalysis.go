package adt

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Pre-compiled regex patterns for ABAP code analysis (compiled once at package init).
var (
	reSelectStarABAP    = regexp.MustCompile(`(?i)^SELECT\s+(SINGLE\s+)?\*\s+FROM\b`)
	reModifyDbtabAll    = regexp.MustCompile(`(?i)^(MODIFY|UPDATE)\s+\w+\s+FROM\b`)
	reHardcodedCreds    = regexp.MustCompile(`(?i)(password|passwd|secret|api_key|apikey|token)\s*=\s*'[^']{3,}'`)
	reClientSpecABAP    = regexp.MustCompile(`(?i)\bCLIENT\s+SPECIFIED\b`)
	reCatchCxRoot       = regexp.MustCompile(`(?i)\bCATCH\s+CX_ROOT\b`)
	reCallMethodDyn     = regexp.MustCompile(`(?i)CALL\s+METHOD\s+\(`)
	reCallFuncDyn       = regexp.MustCompile(`(?i)CALL\s+FUNCTION\s+(\(|\w+[^'"()])`)
	reNormalizeWS       = regexp.MustCompile(`\s+`)
	reObsoleteMove      = regexp.MustCompile(`(?i)^MOVE\s+\S+\s+TO\s+`)
	reObsoleteAdd       = regexp.MustCompile(`(?i)^ADD\s+\S+\s+TO\s+`)
	reObsoleteSubtract  = regexp.MustCompile(`(?i)^SUBTRACT\s+\S+\s+FROM\s+`)
	reObsoleteMultiply  = regexp.MustCompile(`(?i)^MULTIPLY\s+\S+\s+BY\s+`)
	reObsoleteDivide    = regexp.MustCompile(`(?i)^DIVIDE\s+\S+\s+BY\s+`)
	reObsoleteCompute   = regexp.MustCompile(`(?i)^COMPUTE\s+`)
)

const maxSourceBytes = 500 * 1024 // 500KB input size limit

// CodeAnalysisResult is the result of ABAP source code analysis.
type CodeAnalysisResult struct {
	ObjectURI    string              `json:"objectUri,omitempty"`
	ObjectName   string              `json:"objectName,omitempty"`
	Findings     []CodeFinding       `json:"findings"`
	Summary      CodeAnalysisSummary `json:"summary"`
	RulesApplied int                 `json:"rulesApplied"`
}

// CodeFinding represents a single code quality finding.
type CodeFinding struct {
	Rule        string `json:"rule"`
	Category    string `json:"category"`    // "performance", "security", "quality", "robustness"
	Severity    string `json:"severity"`    // "critical", "high", "medium", "low", "info"
	Line        int    `json:"line"`        // start line
	EndLine     int    `json:"endLine"`     // end line
	Match       string `json:"match"`       // offending statement (trimmed, max 200 chars)
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
}

// CodeAnalysisSummary contains aggregate analysis metrics.
type CodeAnalysisSummary struct {
	TotalFindings int            `json:"totalFindings"`
	BySeverity    map[string]int `json:"bySeverity"`
	ByCategory    map[string]int `json:"byCategory"`
	Score         string         `json:"score"` // "good", "warning", "critical"
}

// abapStatement is a complete ABAP statement (may span multiple source lines).
type abapStatement struct {
	Text      string
	StartLine int
	EndLine   int
	IsComment bool
}

// scanContext tracks state during the rule engine pass.
type scanContext struct {
	InLoop    bool
	LoopDepth int
	PrevStmt  *abapStatement
}

// AssembleStatements splits ABAP source into complete statements (period-terminated).
// Pass 1 of the two-pass architecture. Exported for testing.
func AssembleStatements(source string) []abapStatement {
	lines := strings.Split(source, "\n")
	var stmts []abapStatement
	var buf strings.Builder
	startLine := 1
	strState := ssNone

	for i, rawLine := range lines {
		lineNum := i + 1
		line := strings.TrimRight(rawLine, "\r")

		// Full-line comment (* in column 1)
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "*") {
			stmts = append(stmts, abapStatement{
				Text:      trimmed,
				StartLine: lineNum,
				EndLine:   lineNum,
				IsComment: true,
			})
			continue
		}

		// Strip inline comment (after unquoted ")
		cleaned := stripInlineComment(line, &strState)
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == "" {
			continue
		}

		if buf.Len() == 0 {
			startLine = lineNum
		}
		if buf.Len() > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(cleaned)

		// Check if statement ends with period (outside string literal)
		if strState == ssNone && strings.HasSuffix(cleaned, ".") {
			stmts = append(stmts, abapStatement{
				Text:      strings.TrimSpace(buf.String()),
				StartLine: startLine,
				EndLine:   lineNum,
			})
			buf.Reset()
		}
	}

	// Remaining buffer (unterminated statement)
	if buf.Len() > 0 {
		stmts = append(stmts, abapStatement{
			Text:      strings.TrimSpace(buf.String()),
			StartLine: startLine,
			EndLine:   startLine,
		})
	}

	return stmts
}

// stringState tracks what kind of string literal we are currently inside.
type stringState int

const (
	ssNone     stringState = iota
	ssSingle               // inside '...'
	ssBacktick             // inside `...`
	ssTemplate             // inside |...|
)

// stripInlineComment removes inline comments (text after unquoted ") from a line.
// Tracks string literal state across calls via the state pointer.
// Handles single-quote '...', backtick `...`, and string template |...| literals.
func stripInlineComment(line string, state *stringState) string {
	var result strings.Builder
	for i := 0; i < len(line); i++ {
		ch := line[i]
		switch *state {
		case ssSingle:
			result.WriteByte(ch)
			if ch == '\'' {
				if i+1 < len(line) && line[i+1] == '\'' {
					result.WriteByte(line[i+1])
					i++ // skip escaped quote ''
				} else {
					*state = ssNone
				}
			}
		case ssBacktick:
			result.WriteByte(ch)
			if ch == '`' {
				if i+1 < len(line) && line[i+1] == '`' {
					result.WriteByte(line[i+1])
					i++ // skip escaped backtick ``
				} else {
					*state = ssNone
				}
			}
		case ssTemplate:
			result.WriteByte(ch)
			if ch == '\\' && i+1 < len(line) && line[i+1] == '|' {
				result.WriteByte(line[i+1])
				i++ // skip escaped pipe \|
			} else if ch == '|' {
				*state = ssNone
			}
		default: // ssNone
			switch ch {
			case '\'':
				*state = ssSingle
				result.WriteByte(ch)
			case '`':
				*state = ssBacktick
				result.WriteByte(ch)
			case '|':
				*state = ssTemplate
				result.WriteByte(ch)
			case '"':
				// Inline comment starts here — rest of line is comment
				return result.String()
			default:
				result.WriteByte(ch)
			}
		}
	}
	return result.String()
}

// AnalyzeABAPSource runs the full rule engine on ABAP source text.
// Two-pass: assembleStatements → context-tracking rule engine.
// Pure Go, no network calls. Exported for testing.
func AnalyzeABAPSource(source string) *CodeAnalysisResult {
	stmts := AssembleStatements(source)
	var findings []CodeFinding

	ctx := &scanContext{}
	totalRules := 21

	for i, stmt := range stmts {
		if stmt.IsComment {
			findings = append(findings, checkTodoFixme(stmt)...)
			continue
		}

		upper := strings.ToUpper(stmt.Text)

		// Track loop context
		if isLoopStart(upper) {
			ctx.InLoop = true
			ctx.LoopDepth++
		}
		if isLoopEnd(upper) {
			ctx.LoopDepth--
			if ctx.LoopDepth <= 0 {
				ctx.InLoop = false
				ctx.LoopDepth = 0
			}
		}

		// Apply rules
		findings = append(findings, checkSelectInLoop(stmt, upper, ctx)...)
		findings = append(findings, checkSelectStar(stmt, upper)...)
		findings = append(findings, checkFAENoEmptyCheck(stmt, upper, ctx)...)
		findings = append(findings, checkNestedLoop(stmt, upper, ctx)...)
		findings = append(findings, checkSelectEndselect(stmt, upper)...)
		findings = append(findings, checkModifyDbtabAll(stmt, upper)...)
		findings = append(findings, checkCommitInLoop(stmt, upper, ctx)...)
		findings = append(findings, checkReadTableNoBinary(stmt, upper)...)
		findings = append(findings, checkMissingAuthorityCheck(stmt, upper)...)
		findings = append(findings, checkHardcodedCredentials(stmt, upper)...)
		findings = append(findings, checkDynamicSQLUnvalidated(stmt, upper)...)
		findings = append(findings, checkClientSpecified(stmt, upper)...)
		findings = append(findings, checkMissingSysubrcRead(stmt, upper, stmts, i)...)
		findings = append(findings, checkMissingSysubrcCall(stmt, upper, stmts, i)...)
		findings = append(findings, checkEmptyCatch(stmt, upper, stmts, i)...)
		findings = append(findings, checkCatchCxRoot(stmt, upper)...)
		findings = append(findings, checkObsoleteStatement(stmt, upper)...)
		findings = append(findings, checkDynamicCallNoTry(stmt, upper)...)
		findings = append(findings, checkPerformUsage(stmt, upper)...)
		findings = append(findings, checkCommitWorkAndWait(stmt, upper)...)

		ctx.PrevStmt = &stmts[i]
	}

	// Build summary
	bySeverity := map[string]int{}
	byCategory := map[string]int{}
	for _, f := range findings {
		bySeverity[f.Severity]++
		byCategory[f.Category]++
	}

	return &CodeAnalysisResult{
		Findings:     findings,
		RulesApplied: totalRules,
		Summary: CodeAnalysisSummary{
			TotalFindings: len(findings),
			BySeverity:    bySeverity,
			ByCategory:    byCategory,
			Score:         calculateCodeScore(findings),
		},
	}
}

func calculateCodeScore(findings []CodeFinding) string {
	for _, f := range findings {
		if f.Severity == "critical" {
			return "critical"
		}
	}
	for _, f := range findings {
		if f.Severity == "high" {
			return "warning"
		}
	}
	return "good"
}

// AnalyzeABAPCode is the Client method that optionally fetches source before analysis.
func (c *Client) AnalyzeABAPCode(ctx context.Context, objectURI, source string) (*CodeAnalysisResult, error) {
	if err := c.checkSafety(OpRead, "AnalyzeABAPCode"); err != nil {
		return nil, err
	}

	if source == "" && objectURI == "" {
		return nil, fmt.Errorf("either object_uri or source is required")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if source == "" {
		fetched, err := c.getSourceForAnalysis(ctx, objectURI)
		if err != nil {
			return nil, fmt.Errorf("fetching source: %w", err)
		}
		source = fetched
	}

	if len(source) > maxSourceBytes {
		return nil, fmt.Errorf("source too large: %d bytes (max %d)", len(source), maxSourceBytes)
	}

	result := AnalyzeABAPSource(source)
	result.ObjectURI = objectURI
	if objectURI != "" {
		if _, name := parseObjectURIComponents(objectURI); name != "" {
			result.ObjectName = name
		}
	}
	return result, nil
}

// --- Loop tracking helpers ---

func isLoopStart(upper string) bool {
	return strings.HasPrefix(upper, "LOOP AT ") ||
		strings.HasPrefix(upper, "DO.") || strings.HasPrefix(upper, "DO ") ||
		strings.HasPrefix(upper, "WHILE ") ||
		(strings.HasPrefix(upper, "SELECT ") && !strings.Contains(upper, " INTO TABLE ") &&
			!strings.Contains(upper, " INTO CORRESPONDING ") &&
			!strings.Contains(upper, " APPENDING ") &&
			!strings.Contains(upper, " SINGLE "))
}

func isLoopEnd(upper string) bool {
	return strings.HasPrefix(upper, "ENDLOOP.") ||
		strings.HasPrefix(upper, "ENDDO.") ||
		strings.HasPrefix(upper, "ENDWHILE.") ||
		strings.HasPrefix(upper, "ENDSELECT.")
}

func truncStmt(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// --- Rule implementations ---

// Rule 1: select_in_loop (critical/performance)
func checkSelectInLoop(stmt abapStatement, upper string, ctx *scanContext) []CodeFinding {
	if !ctx.InLoop {
		return nil
	}
	if !strings.HasPrefix(upper, "SELECT ") {
		return nil
	}
	return []CodeFinding{{
		Rule:        "select_in_loop",
		Category:    "performance",
		Severity:    "critical",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "SELECT inside loop — causes N+1 database roundtrips",
		Suggestion:  "Move SELECT before the loop using FOR ALL ENTRIES or JOIN",
	}}
}

// Rule 2: select_star (info/performance)
func checkSelectStar(stmt abapStatement, upper string) []CodeFinding {
	if !reSelectStarABAP.MatchString(upper) {
		return nil
	}
	return []CodeFinding{{
		Rule:        "select_star",
		Category:    "performance",
		Severity:    "info",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "SELECT * fetches all columns — prefer explicit field list",
		Suggestion:  "List only the fields you need to reduce data transfer",
	}}
}

// Rule 3: fae_no_empty_check (critical/performance)
func checkFAENoEmptyCheck(stmt abapStatement, upper string, ctx *scanContext) []CodeFinding {
	if !strings.Contains(upper, "FOR ALL ENTRIES IN") {
		return nil
	}
	// Check if previous statement contains an emptiness check
	if ctx.PrevStmt != nil {
		prevUpper := strings.ToUpper(ctx.PrevStmt.Text)
		if strings.Contains(prevUpper, "IS NOT INITIAL") ||
			strings.Contains(prevUpper, "LINES(") ||
			strings.Contains(prevUpper, "LINE_EXISTS(") ||
			strings.Contains(prevUpper, "IS INITIAL") {
			return nil
		}
	}
	return []CodeFinding{{
		Rule:        "fae_no_empty_check",
		Category:    "performance",
		Severity:    "critical",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "FOR ALL ENTRIES without preceding empty check — returns ALL rows if table is empty",
		Suggestion:  "Add IF itab IS NOT INITIAL check before the SELECT with FOR ALL ENTRIES",
	}}
}

// Rule 4: nested_loop (high/performance)
func checkNestedLoop(stmt abapStatement, upper string, ctx *scanContext) []CodeFinding {
	if !ctx.InLoop || !strings.HasPrefix(upper, "LOOP AT ") {
		return nil
	}
	// We're inside a loop and starting another LOOP AT
	if ctx.LoopDepth < 2 {
		return nil // the outer LOOP context should set depth >= 1, inner LOOP starts at 2
	}
	return []CodeFinding{{
		Rule:        "nested_loop",
		Category:    "performance",
		Severity:    "high",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "Nested LOOP AT — O(n*m) complexity",
		Suggestion:  "Use SORTED/HASHED table or READ TABLE with BINARY SEARCH instead",
	}}
}

// Rule 5: select_endselect (medium/performance)
func checkSelectEndselect(stmt abapStatement, upper string) []CodeFinding {
	// Detect SELECT...ENDSELECT by checking for SELECT without INTO TABLE
	if !strings.HasPrefix(upper, "SELECT ") {
		return nil
	}
	if strings.Contains(upper, " INTO TABLE ") ||
		strings.Contains(upper, " INTO CORRESPONDING ") ||
		strings.Contains(upper, " APPENDING ") ||
		strings.Contains(upper, " SINGLE ") {
		return nil
	}
	return []CodeFinding{{
		Rule:        "select_endselect",
		Category:    "performance",
		Severity:    "medium",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "SELECT...ENDSELECT processes rows one by one — prefer INTO TABLE",
		Suggestion:  "Use SELECT ... INTO TABLE itab for bulk fetch",
	}}
}

// Rule 6: modify_dbtab_all (high/performance)
func checkModifyDbtabAll(stmt abapStatement, upper string) []CodeFinding {
	if !reModifyDbtabAll.MatchString(upper) {
		return nil
	}
	if strings.Contains(upper, " WHERE ") {
		return nil
	}
	return []CodeFinding{{
		Rule:        "modify_dbtab_all",
		Category:    "performance",
		Severity:    "high",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "MODIFY/UPDATE without WHERE — may affect all rows in the table",
		Suggestion:  "Add WHERE clause or use internal table with specific records",
	}}
}

// Rule 7: commit_in_loop (critical/performance)
func checkCommitInLoop(stmt abapStatement, upper string, ctx *scanContext) []CodeFinding {
	if !ctx.InLoop {
		return nil
	}
	if !strings.HasPrefix(upper, "COMMIT WORK") {
		return nil
	}
	return []CodeFinding{{
		Rule:        "commit_in_loop",
		Category:    "performance",
		Severity:    "critical",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "COMMIT WORK inside loop — destroys transactional integrity, causes performance issues",
		Suggestion:  "Move COMMIT WORK outside the loop, process all records in one LUW",
	}}
}

// Rule 8: read_table_no_binary (medium/performance)
func checkReadTableNoBinary(stmt abapStatement, upper string) []CodeFinding {
	if !strings.HasPrefix(upper, "READ TABLE ") {
		return nil
	}
	if !strings.Contains(upper, " WITH KEY ") {
		return nil
	}
	if strings.Contains(upper, " BINARY SEARCH") ||
		strings.Contains(upper, " WITH TABLE KEY") {
		return nil
	}
	return []CodeFinding{{
		Rule:        "read_table_no_binary",
		Category:    "performance",
		Severity:    "medium",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "READ TABLE WITH KEY without BINARY SEARCH on standard table — linear scan",
		Suggestion:  "Sort the table and add BINARY SEARCH, or use SORTED/HASHED table type",
	}}
}

// Rule 9: missing_authority_check (info/security)
func checkMissingAuthorityCheck(_ abapStatement, _ string) []CodeFinding {
	// Deliberately returns nil — too many false positives.
	// Would need full method scope tracking to be useful.
	return nil
}

// Rule 10: hardcoded_credentials (critical/security)
func checkHardcodedCredentials(stmt abapStatement, _ string) []CodeFinding {
	// Only match in assignment context: lv_password = 'literal'
	if !reHardcodedCreds.MatchString(stmt.Text) {
		return nil
	}
	return []CodeFinding{{
		Rule:        "hardcoded_credentials",
		Category:    "security",
		Severity:    "critical",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "Hardcoded credential detected in assignment",
		Suggestion:  "Use secure storage (SSF, ICM, Destination service) instead of hardcoded credentials",
	}}
}

// Rule 11: dynamic_sql_unvalidated (high/security)
func checkDynamicSQLUnvalidated(stmt abapStatement, upper string) []CodeFinding {
	// Dynamic WHERE with concatenated variable, no CL_ABAP_DYN_PRG
	if !strings.Contains(upper, "WHERE (") && !strings.Contains(upper, "WHERE(") {
		return nil
	}
	return []CodeFinding{{
		Rule:        "dynamic_sql_unvalidated",
		Category:    "security",
		Severity:    "high",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "Dynamic WHERE clause — potential SQL injection if input is not validated",
		Suggestion:  "Use CL_ABAP_DYN_PRG=>CHECK_COLUMN_NAME or parameterized queries",
	}}
}

// Rule 12: client_specified (medium/security)
func checkClientSpecified(stmt abapStatement, upper string) []CodeFinding {
	if !reClientSpecABAP.MatchString(upper) {
		return nil
	}
	return []CodeFinding{{
		Rule:        "client_specified",
		Category:    "security",
		Severity:    "medium",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "CLIENT SPECIFIED bypasses automatic client handling — cross-client data access",
		Suggestion:  "Use CLIENT SPECIFIED only when cross-client access is intentional",
	}}
}

// Rule 13: missing_sysubrc_read (medium/robustness)
func checkMissingSysubrcRead(stmt abapStatement, upper string, stmts []abapStatement, idx int) []CodeFinding {
	if !strings.HasPrefix(upper, "READ TABLE ") {
		return nil
	}
	// Check next non-comment statement for SY-SUBRC
	for j := idx + 1; j < len(stmts) && j <= idx+2; j++ {
		if stmts[j].IsComment {
			continue
		}
		nextUpper := strings.ToUpper(stmts[j].Text)
		if strings.Contains(nextUpper, "SY-SUBRC") || strings.Contains(nextUpper, "SYST-SUBRC") {
			return nil
		}
		break // only check the very next non-comment statement
	}
	return []CodeFinding{{
		Rule:        "missing_sysubrc_read",
		Category:    "robustness",
		Severity:    "medium",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "READ TABLE without SY-SUBRC check in next statement",
		Suggestion:  "Check SY-SUBRC after READ TABLE to handle not-found case",
	}}
}

// Rule 14: missing_sysubrc_call (medium/robustness)
func checkMissingSysubrcCall(stmt abapStatement, upper string, stmts []abapStatement, idx int) []CodeFinding {
	if !strings.HasPrefix(upper, "CALL FUNCTION ") {
		return nil
	}
	// Skip if EXCEPTIONS clause is present (explicit handling)
	if strings.Contains(upper, " EXCEPTIONS") {
		return nil
	}
	// Check next non-comment statement for SY-SUBRC
	for j := idx + 1; j < len(stmts) && j <= idx+2; j++ {
		if stmts[j].IsComment {
			continue
		}
		nextUpper := strings.ToUpper(stmts[j].Text)
		if strings.Contains(nextUpper, "SY-SUBRC") || strings.Contains(nextUpper, "SYST-SUBRC") {
			return nil
		}
		break
	}
	return []CodeFinding{{
		Rule:        "missing_sysubrc_call",
		Category:    "robustness",
		Severity:    "medium",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "CALL FUNCTION without SY-SUBRC check or EXCEPTIONS clause",
		Suggestion:  "Check SY-SUBRC after CALL FUNCTION or add EXCEPTIONS clause",
	}}
}

// Rule 15: empty_catch (medium/robustness)
func checkEmptyCatch(stmt abapStatement, upper string, stmts []abapStatement, idx int) []CodeFinding {
	if !strings.HasPrefix(upper, "CATCH ") {
		return nil
	}
	// Check if the next non-comment statement is ENDTRY or another CATCH (empty catch body)
	for j := idx + 1; j < len(stmts); j++ {
		if stmts[j].IsComment {
			continue
		}
		nextUpper := strings.ToUpper(stmts[j].Text)
		if strings.HasPrefix(nextUpper, "ENDTRY.") || strings.HasPrefix(nextUpper, "CATCH ") {
			return []CodeFinding{{
				Rule:        "empty_catch",
				Category:    "robustness",
				Severity:    "medium",
				Line:        stmt.StartLine,
				EndLine:     stmt.EndLine,
				Match:       truncStmt(stmt.Text, 200),
				Description: "Empty CATCH block — exceptions are silently swallowed",
				Suggestion:  "Log the exception or re-raise it; do not silently ignore errors",
			}}
		}
		break
	}
	return nil
}

// Rule 16: catch_cx_root (medium/robustness)
func checkCatchCxRoot(stmt abapStatement, upper string) []CodeFinding {
	if !reCatchCxRoot.MatchString(upper) {
		return nil
	}
	return []CodeFinding{{
		Rule:        "catch_cx_root",
		Category:    "robustness",
		Severity:    "medium",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "CATCH CX_ROOT catches all exceptions including system errors",
		Suggestion:  "Catch specific exception classes instead of CX_ROOT",
	}}
}

// Rule 17: obsolete_statement (info/quality)
func checkObsoleteStatement(stmt abapStatement, upper string) []CodeFinding {
	obsolete := []struct {
		re   *regexp.Regexp
		name string
	}{
		{reObsoleteMove, "MOVE x TO y"},
		{reObsoleteAdd, "ADD x TO y"},
		{reObsoleteSubtract, "SUBTRACT x FROM y"},
		{reObsoleteMultiply, "MULTIPLY x BY y"},
		{reObsoleteDivide, "DIVIDE x BY y"},
		{reObsoleteCompute, "COMPUTE"},
	}
	for _, o := range obsolete {
		if o.re.MatchString(upper) {
			return []CodeFinding{{
				Rule:        "obsolete_statement",
				Category:    "quality",
				Severity:    "info",
				Line:        stmt.StartLine,
				EndLine:     stmt.EndLine,
				Match:       truncStmt(stmt.Text, 200),
				Description: fmt.Sprintf("Obsolete statement: %s — use modern syntax", o.name),
				Suggestion:  "Use inline expressions: x = y + z, instead of COMPUTE/ADD/MOVE",
			}}
		}
	}
	return nil
}

// Rule 18: dynamic_call_no_try (high/quality)
func checkDynamicCallNoTry(stmt abapStatement, upper string) []CodeFinding {
	// Match: CALL METHOD (var)=>method or CALL FUNCTION (var) or CALL FUNCTION lv_name
	isDynamic := reCallMethodDyn.MatchString(upper) || reCallFuncDyn.MatchString(upper)
	if !isDynamic {
		return nil
	}
	// This is a simplified check — ideally we'd track TRY/ENDTRY scope
	return []CodeFinding{{
		Rule:        "dynamic_call_no_try",
		Category:    "quality",
		Severity:    "high",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "Dynamic CALL METHOD/FUNCTION without TRY — can crash at runtime if target doesn't exist",
		Suggestion:  "Wrap dynamic calls in TRY...CATCH CX_SY_DYN_CALL_ERROR",
	}}
}

// Rule 19: perform_usage (info/quality)
func checkPerformUsage(stmt abapStatement, upper string) []CodeFinding {
	if !strings.HasPrefix(upper, "PERFORM ") && !strings.HasPrefix(upper, "FORM ") {
		return nil
	}
	return []CodeFinding{{
		Rule:        "perform_usage",
		Category:    "quality",
		Severity:    "info",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "PERFORM/FORM is obsolete — consider migration to methods",
		Suggestion:  "Refactor FORM routines into class methods for better encapsulation and testability",
	}}
}

// Rule 20: commit_work_and_wait (medium/quality)
func checkCommitWorkAndWait(stmt abapStatement, upper string) []CodeFinding {
	if !strings.HasPrefix(upper, "COMMIT WORK") {
		return nil
	}
	if strings.Contains(upper, "AND WAIT") {
		return nil
	}
	return []CodeFinding{{
		Rule:        "commit_work_and_wait",
		Category:    "quality",
		Severity:    "medium",
		Line:        stmt.StartLine,
		EndLine:     stmt.EndLine,
		Match:       truncStmt(stmt.Text, 200),
		Description: "COMMIT WORK without AND WAIT — async commit hides update task errors",
		Suggestion:  "Use COMMIT WORK AND WAIT to ensure update task completes synchronously",
	}}
}

// Rule 21: todo_fixme (info/quality)
func checkTodoFixme(stmt abapStatement) []CodeFinding {
	if !stmt.IsComment {
		return nil
	}
	upper := strings.ToUpper(stmt.Text)
	if strings.Contains(upper, "TODO") || strings.Contains(upper, "FIXME") ||
		strings.Contains(upper, "HACK") || strings.Contains(upper, "XXX") {
		return []CodeFinding{{
			Rule:        "todo_fixme",
			Category:    "quality",
			Severity:    "info",
			Line:        stmt.StartLine,
			EndLine:     stmt.EndLine,
			Match:       truncStmt(stmt.Text, 200),
			Description: "TODO/FIXME/HACK comment found — indicates unfinished work",
			Suggestion:  "Address the TODO or create a tracking issue",
		}}
	}
	return nil
}
