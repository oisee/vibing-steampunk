// Package adt provides ABAP code analysis using the native Go abaplint lexer and parser.
//
// AnalyzeABAPCode runs all abaplint rules against ABAP source and returns findings
// with severity, category, and line-level location. Built on the oracle-verified
// pkg/abaplint foundation (100% match on 22K tokens, 3,254 statements).
package adt

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/abaplint"
)

const maxSourceBytes = 500 * 1024 // 500KB input size limit

// CodeAnalysisResult is the complete output of AnalyzeABAPCode.
type CodeAnalysisResult struct {
	ObjectURI    string              `json:"objectUri,omitempty"`
	ObjectName   string              `json:"objectName,omitempty"`
	Findings     []CodeFinding       `json:"findings"`
	Summary      CodeAnalysisSummary `json:"summary"`
	RulesApplied int                 `json:"rulesApplied"`
}

// allRules returns the full set of abaplint rules for code analysis.
func allRules() []abaplint.Rule {
	return []abaplint.Rule{
		// Quality rules (from original abaplint port)
		&abaplint.LineLengthRule{MaxLength: 130},
		&abaplint.EmptyStatementRule{},
		&abaplint.MaxOneStatementRule{},
		&abaplint.PreferredCompareOperatorRule{BadOperators: []string{"EQ", "NE", "GT", "LT", "GE", "LE", "><"}},
		&abaplint.ObsoleteStatementRule{Compute: true, Add: true, Subtract: true, Multiply: true, Divide: true, Move: true},
		&abaplint.ColonMissingSpaceRule{},
		&abaplint.DoubleSpaceRule{},
		&abaplint.LocalVariableNamesRule{},
		// Security & performance rules (v2)
		&abaplint.SelectStarRule{},
		&abaplint.HardcodedCredentialsRule{},
		&abaplint.CatchCxRootRule{},
		&abaplint.CommitInLoopRule{},
		&abaplint.DynamicCallNoTryRule{},
	}
}

// ruleSeverity maps abaplint issue severity to CodeFinding severity.
func ruleSeverity(s string) string {
	switch s {
	case "Error":
		return "high"
	case "Warning":
		return "medium"
	default:
		return "info"
	}
}

// ruleCategory maps rule keys to analysis categories.
func ruleCategory(key string) string {
	switch key {
	case "select_star", "commit_in_loop":
		return "performance"
	case "hardcoded_credentials":
		return "security"
	case "catch_cx_root", "dynamic_call_no_try":
		return "robustness"
	default:
		return "quality"
	}
}

// ruleSuggestion returns a human-readable fix suggestion for known rule keys.
func ruleSuggestion(key string) string {
	switch key {
	case "select_star":
		return "List only the fields you need to reduce data transfer"
	case "hardcoded_credentials":
		return "Use secure storage (SSF, ICM, Destination service) instead of hardcoded credentials"
	case "catch_cx_root":
		return "Catch specific exception classes instead of broad bases like CX_ROOT"
	case "commit_in_loop":
		return "Move COMMIT WORK outside the loop, process all records in one LUW"
	case "dynamic_call_no_try":
		return "Wrap dynamic calls in TRY...CATCH CX_SY_DYN_CALL_ERROR"
	case "line_length":
		return "Break long lines using ABAP line continuation"
	case "empty_statement":
		return "Remove the stray period or empty statement"
	case "max_one_statement":
		return "Split chained statements onto separate lines for readability"
	case "preferred_compare_operator":
		return "Use modern comparison operators (=, <>, <, >, etc.) instead of EQ, NE, LT, GT"
	case "obsolete_statement":
		return "Replace with modern ABAP equivalent (e.g., MOVE → assignment, ADD → +=)"
	case "colon_missing_space":
		return "Add a space after the colon in chained statements"
	case "double_space":
		return "Remove extra whitespace"
	case "local_variable_names":
		return "Use standard ABAP naming conventions (lv_, lt_, lo_, etc.)"
	default:
		return ""
	}
}

// AnalyzeABAPSource runs all abaplint rules on ABAP source text.
// Pure Go, no network calls. Exported for testing.
func AnalyzeABAPSource(source string) *CodeAnalysisResult {
	if len(source) > maxSourceBytes {
		return &CodeAnalysisResult{
			Findings: []CodeFinding{{
				Rule:        "source_too_large",
				Category:    "quality",
				Severity:    "info",
				Line:        1,
				EndLine:     1,
				Description: fmt.Sprintf("Source exceeds %dKB limit, analysis skipped", maxSourceBytes/1024),
			}},
			Summary: CodeAnalysisSummary{
				TotalFindings: 1,
				BySeverity:    map[string]int{"info": 1},
				ByCategory:    map[string]int{"quality": 1},
				Score:         "good",
			},
			RulesApplied: 0,
		}
	}

	rules := allRules()
	linter := &abaplint.Linter{Rules: rules}
	issues := linter.Run("source.abap", source)

	findings := make([]CodeFinding, 0, len(issues))
	for _, iss := range issues {
		findings = append(findings, CodeFinding{
			Rule:        iss.Key,
			Category:    ruleCategory(iss.Key),
			Severity:    ruleSeverity(iss.Severity),
			Line:        iss.Row,
			EndLine:     iss.Row,
			Match:       iss.Message,
			Description: iss.Message,
			Suggestion:  ruleSuggestion(iss.Key),
		})
	}

	// Stable sort by (line, rule) for deterministic output
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Rule < findings[j].Rule
	})

	// Build summary
	bySeverity := map[string]int{}
	byCategory := map[string]int{}
	for _, f := range findings {
		bySeverity[f.Severity]++
		byCategory[f.Category]++
	}

	return &CodeAnalysisResult{
		Findings:     findings,
		RulesApplied: len(rules),
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
// If source is provided directly, objectType/objectName are optional (used for labeling only).
// If source is empty, objectType and objectName are used to fetch source via GetSource.
func (c *Client) AnalyzeABAPCode(ctx context.Context, objectType, objectName, source string) (*CodeAnalysisResult, error) {
	if err := c.checkSafety(OpRead, "AnalyzeABAPCode"); err != nil {
		return nil, err
	}

	if source == "" && objectName == "" {
		return nil, fmt.Errorf("either source or object_name is required")
	}

	// Fetch source from SAP if not provided directly
	if source == "" {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		fetched, err := c.GetSource(ctx, objectType, objectName, nil)
		if err != nil {
			return nil, fmt.Errorf("fetching source for %s/%s: %w", objectType, objectName, err)
		}
		source = fetched
	}

	result := AnalyzeABAPSource(source)
	result.ObjectName = objectName
	return result, nil
}
