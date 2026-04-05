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
