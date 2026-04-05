// Package adt provides shared types for ABAP code analysis tools.
package adt

// CodeFinding represents a single code quality finding.
type CodeFinding struct {
	Rule        string `json:"rule"`
	Category    string `json:"category"`    // "performance", "security", "quality", "robustness"
	Severity    string `json:"severity"`    // "critical", "high", "medium", "low", "info"
	Line        int    `json:"line"`        // start line
	EndLine     int    `json:"endLine"`     // end line
	Match       string `json:"match"`       // rule message or offending code fragment
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

// SQLExplainPlan represents the execution plan for a SQL query.
type SQLExplainPlan struct {
	Query     string        `json:"query"`
	Nodes     []SQLPlanNode `json:"nodes"`
	TotalCost float64       `json:"totalCost,omitempty"`
}

// SQLPlanNode represents a single node in the execution plan tree.
type SQLPlanNode struct {
	ID       int           `json:"id"`
	Operator string        `json:"operator"`
	Table    string        `json:"table,omitempty"`
	Index    string        `json:"index,omitempty"`
	Cost     float64       `json:"cost,omitempty"`
	Rows     int           `json:"rows,omitempty"`
	Children []SQLPlanNode `json:"children,omitempty"`
}

// truncStmt truncates a statement string to max length with ellipsis.
func truncStmt(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
