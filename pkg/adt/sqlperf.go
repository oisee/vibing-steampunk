package adt

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// SQLPerformanceAnalysis is the combined result of SQL performance analysis.
type SQLPerformanceAnalysis struct {
	Query         string                   `json:"query"`
	PlanFindings  []SQLPerformanceFinding  `json:"planFindings,omitempty"`
	TextFindings  []SQLPerformanceFinding  `json:"textFindings,omitempty"`
	Summary       SQLPerfSummary           `json:"summary"`
	HANAAvailable bool                     `json:"hanaAvailable"`
	Warnings      []string                 `json:"warnings,omitempty"`
}

// SQLPerformanceFinding represents a single performance issue found in the SQL.
type SQLPerformanceFinding struct {
	Type        string  `json:"type"`        // e.g. "full_table_scan", "select_star"
	Severity    string  `json:"severity"`    // "critical", "high", "medium", "info"
	Description string  `json:"description"` // Human-readable explanation
	Suggestion  string  `json:"suggestion"`  // How to fix
	NodeID      int     `json:"nodeId,omitempty"`
	Table       string  `json:"table,omitempty"`
	Cost        float64 `json:"cost,omitempty"`
	Rows        int     `json:"rows,omitempty"`
}

// SQLPerfSummary contains aggregate performance summary.
type SQLPerfSummary struct {
	TotalFindings int            `json:"totalFindings"`
	BySeverity    map[string]int `json:"bySeverity"`
	Score         string         `json:"score"` // "good", "warning", "critical"
}

// Pre-compiled regex patterns for SQL text analysis (compiled once at package init).
var (
	reSQLHostVar       = regexp.MustCompile(`@\w[\w~-]*`)
	reSQLIntoTable     = regexp.MustCompile(`(?i)\b(INTO\s+(CORRESPONDING\s+FIELDS\s+OF\s+)?TABLE\s+\S+|APPENDING\s+(CORRESPONDING\s+FIELDS\s+OF\s+)?TABLE\s+\S+)`)
	reSQLIntoTuple     = regexp.MustCompile(`(?i)\bINTO\s*\([^)]+\)`)
	reSQLIntoSingle    = regexp.MustCompile(`(?i)\bINTO\s+\S+`)
	reSQLUpToRows      = regexp.MustCompile(`(?i)\bUP\s+TO\s+\d+\s+ROWS\b`)
	reSQLFAE           = regexp.MustCompile(`(?i)\bFOR\s+ALL\s+ENTRIES\s+IN\s+\S+`)
	reSQLWhitespace    = regexp.MustCompile(`\s+`)
	reSQLSelectStar    = regexp.MustCompile(`(?i)\bSELECT\s+(SINGLE\s+)?\*\s+FROM\b`)
	reSQLSelectCount   = regexp.MustCompile(`(?i)\bSELECT\s+COUNT\b`)
	reSQLClientSpec    = regexp.MustCompile(`(?i)\bCLIENT\s+SPECIFIED\b`)
	reSQLSelectWord    = regexp.MustCompile(`(?i)\bSELECT\b`)
	reSQLDistinct      = regexp.MustCompile(`(?i)\bDISTINCT\b`)
)

// AnalyzePlanNodes walks a SQL execution plan tree and detects performance issues.
// Pure Go, no network calls. Exported for testing.
func AnalyzePlanNodes(nodes []SQLPlanNode) []SQLPerformanceFinding {
	var findings []SQLPerformanceFinding
	var walk func(node SQLPlanNode)

	walk = func(node SQLPlanNode) {
		op := strings.ToUpper(node.Operator)

		// full_table_scan / full_scan_small
		if strings.Contains(op, "TABLE SCAN") && node.Index == "" {
			if node.Rows > 1000 {
				findings = append(findings, SQLPerformanceFinding{
					Type:        "full_table_scan",
					Severity:    "critical",
					Description: fmt.Sprintf("Full table scan on %s (%d rows, no index)", node.Table, node.Rows),
					Suggestion:  "Add an appropriate index or use a WHERE clause to limit the scan",
					NodeID:      node.ID,
					Table:       node.Table,
					Cost:        node.Cost,
					Rows:        node.Rows,
				})
			} else {
				findings = append(findings, SQLPerformanceFinding{
					Type:        "full_scan_small",
					Severity:    "info",
					Description: fmt.Sprintf("Full table scan on %s (%d rows) — small table, acceptable", node.Table, node.Rows),
					Suggestion:  "Acceptable for small tables; consider indexing if data grows",
					NodeID:      node.ID,
					Table:       node.Table,
					Cost:        node.Cost,
					Rows:        node.Rows,
				})
			}
		}

		// missing_index
		if node.Cost > 100 && node.Index == "" && node.Table != "" &&
			!strings.Contains(op, "TABLE SCAN") {
			findings = append(findings, SQLPerformanceFinding{
				Type:        "missing_index",
				Severity:    "high",
				Description: fmt.Sprintf("High cost node (%v) on %s without index", node.Cost, node.Table),
				Suggestion:  "Consider adding an index for the filtered columns",
				NodeID:      node.ID,
				Table:       node.Table,
				Cost:        node.Cost,
				Rows:        node.Rows,
			})
		}

		// nested_loop_large
		if strings.Contains(op, "NESTED LOOP") {
			maxChildRows := 0
			for _, child := range node.Children {
				if child.Rows > maxChildRows {
					maxChildRows = child.Rows
				}
			}
			if maxChildRows > 10000 {
				findings = append(findings, SQLPerformanceFinding{
					Type:        "nested_loop_large",
					Severity:    "high",
					Description: fmt.Sprintf("Nested loop join with %d rows in child — may be slow", maxChildRows),
					Suggestion:  "Consider HASH JOIN or add indexes to improve join performance",
					NodeID:      node.ID,
					Cost:        node.Cost,
					Rows:        node.Rows,
				})
			}
		}

		// high_cost_node
		if node.Cost > 1000 && !strings.Contains(op, "TABLE SCAN") &&
			!strings.Contains(op, "NESTED LOOP") {
			findings = append(findings, SQLPerformanceFinding{
				Type:        "high_cost_node",
				Severity:    "medium",
				Description: fmt.Sprintf("High cost node: %s (cost: %v)", node.Operator, node.Cost),
				Suggestion:  "Review this operation for optimization opportunities",
				NodeID:      node.ID,
				Cost:        node.Cost,
				Rows:        node.Rows,
			})
		}

		// cartesian_product
		if strings.Contains(op, "CROSS JOIN") {
			findings = append(findings, SQLPerformanceFinding{
				Type:        "cartesian_product",
				Severity:    "critical",
				Description: fmt.Sprintf("Cartesian product (CROSS JOIN) detected at node %d", node.ID),
				Suggestion:  "Add a join condition to prevent full cartesian product",
				NodeID:      node.ID,
				Cost:        node.Cost,
				Rows:        node.Rows,
			})
		}

		for _, child := range node.Children {
			walk(child)
		}
	}

	for _, node := range nodes {
		walk(node)
	}
	return findings
}

// stripABAPSQLSyntax removes ABAP SQL-specific syntax to produce a cleaner SQL
// for text analysis. Does NOT produce valid native SQL — only for pattern matching.
func stripABAPSQLSyntax(abapSQL string) string {
	s := abapSQL

	// Remove host variable markers (@)
	s = reSQLHostVar.ReplaceAllStringFunc(s, func(m string) string {
		return m[1:] // strip leading @
	})

	// Remove INTO TABLE / INTO CORRESPONDING FIELDS OF TABLE / APPENDING TABLE clauses
	s = reSQLIntoTable.ReplaceAllString(s, "")

	// Remove INTO (target, target, ...) single-row results
	s = reSQLIntoTuple.ReplaceAllString(s, "")
	// Remove INTO @struct / INTO @DATA(...)
	s = reSQLIntoSingle.ReplaceAllString(s, "")

	// Remove UP TO n ROWS
	s = reSQLUpToRows.ReplaceAllString(s, "")

	// Remove FOR ALL ENTRIES IN clause (the IN @itab part)
	s = reSQLFAE.ReplaceAllString(s, "")

	// Remove trailing period (ABAP statement terminator)
	s = strings.TrimRight(strings.TrimSpace(s), ".")

	// Collapse multiple spaces
	s = reSQLWhitespace.ReplaceAllString(s, " ")

	return strings.TrimSpace(s)
}

// AnalyzeSQLText performs text-based analysis of a SQL query string.
// Handles both ABAP SQL and native SQL syntax.
// Pure Go, no network calls. Exported for testing.
func AnalyzeSQLText(sqlQuery string) []SQLPerformanceFinding {
	var findings []SQLPerformanceFinding

	if strings.TrimSpace(sqlQuery) == "" {
		return findings
	}

	// Normalize: strip ABAP-specific syntax for pattern analysis
	cleaned := stripABAPSQLSyntax(sqlQuery)
	upper := strings.ToUpper(cleaned)

	// select_star: SELECT * or SELECT SINGLE *
	if reSQLSelectStar.MatchString(cleaned) {
		findings = append(findings, SQLPerformanceFinding{
			Type:        "select_star",
			Severity:    "info",
			Description: "SELECT * fetches all columns — prefer explicit field list",
			Suggestion:  "List only the fields you need to reduce data transfer",
		})
	}

	// missing_where: SELECT without WHERE (after stripping INTO/FAE)
	if strings.Contains(upper, "SELECT") && strings.Contains(upper, "FROM") &&
		!strings.Contains(upper, "WHERE") &&
		!reSQLSelectCount.MatchString(cleaned) {
		findings = append(findings, SQLPerformanceFinding{
			Type:        "missing_where",
			Severity:    "high",
			Description: "SELECT without WHERE clause — may read entire table",
			Suggestion:  "Add a WHERE clause to limit the result set",
		})
	}

	// client_specified: CLIENT SPECIFIED — cross-client data access
	if reSQLClientSpec.MatchString(sqlQuery) {
		findings = append(findings, SQLPerformanceFinding{
			Type:        "client_specified",
			Severity:    "medium",
			Description: "CLIENT SPECIFIED bypasses automatic client handling — cross-client data access",
			Suggestion:  "Use CLIENT SPECIFIED only when cross-client access is intentional",
		})
	}

	// nested_subquery: SELECT inside SELECT
	selectCount := len(reSQLSelectWord.FindAllString(cleaned, -1))
	if selectCount > 1 {
		findings = append(findings, SQLPerformanceFinding{
			Type:        "nested_subquery",
			Severity:    "medium",
			Description: fmt.Sprintf("Nested subquery detected (%d SELECT statements)", selectCount),
			Suggestion:  "Consider using JOINs instead of subqueries for better performance",
		})
	}

	// no_up_to_rows: large table SELECT without UP TO n ROWS (ABAP SQL specific)
	// Check original query (before stripping) for UP TO
	upperOrig := strings.ToUpper(sqlQuery)
	if strings.Contains(upperOrig, "SELECT") && strings.Contains(upperOrig, "FROM") &&
		!strings.Contains(upperOrig, "UP TO") &&
		!strings.Contains(upperOrig, "SINGLE") &&
		!strings.Contains(upperOrig, "COUNT") &&
		!strings.Contains(upperOrig, "LIMIT") {
		findings = append(findings, SQLPerformanceFinding{
			Type:        "no_up_to_rows",
			Severity:    "info",
			Description: "SELECT without row limit (UP TO n ROWS / LIMIT) — may return large result set",
			Suggestion:  "Consider adding UP TO n ROWS if you don't need all rows",
		})
	}

	// distinct_usage: DISTINCT often indicates missing WHERE or design issue
	if reSQLDistinct.MatchString(cleaned) {
		findings = append(findings, SQLPerformanceFinding{
			Type:        "distinct_usage",
			Severity:    "info",
			Description: "DISTINCT may indicate missing WHERE clause or data model issue",
			Suggestion:  "Review if DISTINCT is necessary — it forces a sort operation",
		})
	}

	return findings
}

// calculateSQLScore determines the overall performance score based on findings.
func calculateSQLScore(findings []SQLPerformanceFinding) string {
	hasCritical := false
	hasHigh := false
	for _, f := range findings {
		switch f.Severity {
		case "critical":
			hasCritical = true
		case "high":
			hasHigh = true
		}
	}
	if hasCritical {
		return "critical"
	}
	if hasHigh {
		return "warning"
	}
	return "good"
}

// AnalyzeSQLPerformance performs comprehensive SQL performance analysis.
// Always runs text analysis on the input query.
// If hanaAvailable is true, also attempts GetSQLExplainPlan for HANA execution plan analysis.
func (c *Client) AnalyzeSQLPerformance(ctx context.Context, sqlQuery string, hanaAvailable bool) (*SQLPerformanceAnalysis, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := c.checkSafety(OpRead, "AnalyzeSQLPerformance"); err != nil {
		return nil, err
	}

	if strings.TrimSpace(sqlQuery) == "" {
		return nil, fmt.Errorf("sql_query is required")
	}

	result := &SQLPerformanceAnalysis{
		Query:         sqlQuery,
		HANAAvailable: hanaAvailable,
	}

	// Always run text-based analysis
	result.TextFindings = AnalyzeSQLText(sqlQuery)

	// If HANA available, attempt explain plan
	if hanaAvailable {
		plan, err := c.GetSQLExplainPlan(ctx, sqlQuery)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("HANA explain plan failed: %v — using text analysis only", err))
		} else if plan != nil && len(plan.Nodes) > 0 {
			result.PlanFindings = AnalyzePlanNodes(plan.Nodes)
		}
	} else {
		result.Warnings = append(result.Warnings, "HANA not available — using text analysis only (no execution plan)")
	}

	// Merge all findings for summary (avoid aliasing result.PlanFindings slice)
	allFindings := make([]SQLPerformanceFinding, 0, len(result.PlanFindings)+len(result.TextFindings))
	allFindings = append(allFindings, result.PlanFindings...)
	allFindings = append(allFindings, result.TextFindings...)
	bySeverity := map[string]int{}
	for _, f := range allFindings {
		bySeverity[f.Severity]++
	}
	result.Summary = SQLPerfSummary{
		TotalFindings: len(allFindings),
		BySeverity:    bySeverity,
		Score:         calculateSQLScore(allFindings),
	}

	return result, nil
}
