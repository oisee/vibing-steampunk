package adt

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// RegressionResult is the output of regression analysis.
type RegressionResult struct {
	ObjectURI   string            `json:"objectUri"`
	ObjectName  string            `json:"objectName,omitempty"`
	ObjectType  string            `json:"objectType,omitempty"`
	BaseVersion string            `json:"baseVersion"`
	Findings    []CodeFinding     `json:"findings"`
	Summary     RegressionSummary `json:"summary"`
	Warnings    []string          `json:"warnings,omitempty"`
}

// RegressionSummary contains aggregate regression metrics.
type RegressionSummary struct {
	TotalFindings    int    `json:"totalFindings"`
	CriticalCount    int    `json:"criticalCount"`
	HighCount        int    `json:"highCount"`
	RiskLevel        string `json:"riskLevel"` // "safe", "caution", "breaking"
	SignatureChanges int    `json:"signatureChanges"`
	RemovedMethods   int    `json:"removedMethods"`
}

// DetectSignatureChanges compares method signatures between old and new source.
// Returns findings for changed parameter types or added required parameters.
// Pure Go, exported for testing.
func DetectSignatureChanges(oldSource, newSource string) []CodeFinding {
	oldMethods := extractMethodDefs(oldSource)
	newMethods := extractMethodDefs(newSource)

	var findings []CodeFinding
	for name, oldDef := range oldMethods {
		newDef, exists := newMethods[name]
		if !exists {
			continue // handled by DetectRemovedPublicMethods
		}
		if normalizeMethodDef(oldDef) != normalizeMethodDef(newDef) {
			findings = append(findings, CodeFinding{
				Rule:        "changed_signature",
				Category:    "robustness",
				Severity:    "critical",
				Line:        0,
				Description: fmt.Sprintf("Method %s signature changed", name),
				Match:       truncStmt(fmt.Sprintf("OLD: %s | NEW: %s", oldDef, newDef), 200),
				Suggestion:  "Changing method signatures breaks all callers — add new optional params or create a new method",
			})
		}
	}
	return findings
}

// DetectRemovedPublicMethods finds public methods that exist in old source but not in new.
// Pure Go, exported for testing.
func DetectRemovedPublicMethods(oldSource, newSource string) []CodeFinding {
	oldMethods := extractPublicMethods(oldSource)
	newMethods := extractPublicMethods(newSource)

	var findings []CodeFinding
	for name := range oldMethods {
		if _, exists := newMethods[name]; !exists {
			findings = append(findings, CodeFinding{
				Rule:        "removed_public_method",
				Category:    "robustness",
				Severity:    "critical",
				Line:        0,
				Description: fmt.Sprintf("Public method %s was removed", name),
				Suggestion:  "Removing public methods breaks all callers — deprecate first or keep as empty wrapper",
			})
		}
	}
	return findings
}

// DetectInterfaceChanges detects any changes in INTERFACE definitions.
// Pure Go, exported for testing.
func DetectInterfaceChanges(oldSource, newSource string) []CodeFinding {
	oldIface := extractInterfaceDef(oldSource)
	newIface := extractInterfaceDef(newSource)

	if oldIface == "" || newIface == "" {
		return nil // not an interface, or can't extract
	}
	if normalizeWhitespace(oldIface) == normalizeWhitespace(newIface) {
		return nil
	}

	return []CodeFinding{{
		Rule:        "changed_interface_method",
		Category:    "robustness",
		Severity:    "critical",
		Line:        0,
		Description: "Interface definition changed — all implementing classes may need updates",
		Suggestion:  "Interface changes break all implementors — consider adding methods to a new interface instead",
	}}
}

// DetectExceptionChanges finds RAISING clause changes in method definitions.
// Pure Go, exported for testing.
func DetectExceptionChanges(oldSource, newSource string) []CodeFinding {
	oldRaising := extractRaisingClauses(oldSource)
	newRaising := extractRaisingClauses(newSource)

	var findings []CodeFinding
	for method, oldClause := range oldRaising {
		newClause, exists := newRaising[method]
		if !exists {
			continue
		}
		if normalizeWhitespace(oldClause) != normalizeWhitespace(newClause) {
			findings = append(findings, CodeFinding{
				Rule:        "changed_exception_handling",
				Category:    "robustness",
				Severity:    "high",
				Line:        0,
				Description: fmt.Sprintf("RAISING clause changed for method %s", method),
				Match:       truncStmt(fmt.Sprintf("OLD: %s | NEW: %s", oldClause, newClause), 200),
				Suggestion:  "Changed RAISING clause may require callers to handle new exceptions",
			})
		}
	}
	return findings
}

// CheckRegression compares current source against a base version and detects breaking changes.
func (c *Client) CheckRegression(ctx context.Context, objectURI, baseVersion string) (*RegressionResult, error) {
	if err := c.checkSafety(OpRead, "CheckRegression"); err != nil {
		return nil, err
	}

	if objectURI == "" {
		return nil, fmt.Errorf("object_uri is required")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result := &RegressionResult{
		ObjectURI: objectURI,
	}

	// Parse object type and name from URI
	objType, objName := parseObjectURIComponents(objectURI)
	result.ObjectType = objType
	result.ObjectName = objName

	// Fetch current source
	currentSource, err := c.getSourceForAnalysis(ctx, objectURI)
	if err != nil {
		return nil, fmt.Errorf("fetching current source: %w", err)
	}

	// Determine base version
	var baseVersionURI string
	if baseVersion != "" {
		baseVersionURI = baseVersion
	} else {
		// Auto-detect: get revisions, pick second-newest
		if objType == "" || objName == "" {
			result.Warnings = append(result.Warnings, "Could not parse object type/name from URI — provide base_version explicitly")
			result.Summary.RiskLevel = "unknown"
			return result, nil
		}
		revisions, err := c.GetRevisions(ctx, objType, objName, nil)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Could not fetch revisions: %v — provide base_version explicitly", err))
			result.Summary.RiskLevel = "unknown"
			return result, nil
		}
		if len(revisions) == 0 {
			result.Warnings = append(result.Warnings, "No version history available — cannot detect regressions")
			result.Summary.RiskLevel = "unknown"
			return result, nil
		}
		if len(revisions) == 1 {
			baseVersionURI = revisions[0].URI
			result.Warnings = append(result.Warnings, "Only 1 revision found — comparing against it")
		} else {
			// Second-newest = index 1 (revisions sorted newest-first)
			baseVersionURI = revisions[1].URI
		}
	}
	result.BaseVersion = baseVersionURI

	// Fetch base source
	baseSource, err := c.GetRevisionSource(ctx, baseVersionURI)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Could not fetch base version source: %v", err))
		result.Summary.RiskLevel = "unknown"
		return result, nil
	}

	// Run regression rules
	result.Findings = append(result.Findings, DetectSignatureChanges(baseSource, currentSource)...)
	result.Findings = append(result.Findings, DetectRemovedPublicMethods(baseSource, currentSource)...)
	result.Findings = append(result.Findings, DetectInterfaceChanges(baseSource, currentSource)...)
	result.Findings = append(result.Findings, DetectExceptionChanges(baseSource, currentSource)...)

	// Build summary
	sigChanges := 0
	removedMethods := 0
	criticalCount := 0
	highCount := 0
	for _, f := range result.Findings {
		switch f.Rule {
		case "changed_signature":
			sigChanges++
		case "removed_public_method":
			removedMethods++
		}
		switch f.Severity {
		case "critical":
			criticalCount++
		case "high":
			highCount++
		}
	}

	riskLevel := "safe"
	if criticalCount > 0 {
		riskLevel = "breaking"
	} else if highCount > 0 {
		riskLevel = "caution"
	}

	result.Summary = RegressionSummary{
		TotalFindings:    len(result.Findings),
		CriticalCount:    criticalCount,
		HighCount:        highCount,
		RiskLevel:        riskLevel,
		SignatureChanges: sigChanges,
		RemovedMethods:   removedMethods,
	}

	return result, nil
}

// --- Helper functions ---

// extractMethodDefs extracts METHODS definitions from ABAP source.
// Returns map of method_name → full definition line.
func extractMethodDefs(source string) map[string]string {
	result := make(map[string]string)
	re := regexp.MustCompile(`(?im)^\s*METHODS\s+(\w+)\b(.*)$`)
	matches := re.FindAllStringSubmatch(source, -1)
	for _, m := range matches {
		name := strings.ToUpper(m[1])
		result[name] = strings.TrimSpace(m[0])
	}
	return result
}

// extractPublicMethods extracts method names from PUBLIC SECTION.
func extractPublicMethods(source string) map[string]bool {
	result := make(map[string]bool)
	upper := strings.ToUpper(source)

	// Find PUBLIC SECTION
	pubIdx := strings.Index(upper, "PUBLIC SECTION")
	if pubIdx < 0 {
		return result
	}

	// Find the end (PROTECTED SECTION, PRIVATE SECTION, or ENDCLASS)
	rest := upper[pubIdx:]
	baseOffset := len("PUBLIC SECTION")
	endIdx := len(rest)
	for _, marker := range []string{"PROTECTED SECTION", "PRIVATE SECTION", "ENDCLASS"} {
		if idx := strings.Index(rest[baseOffset:], marker); idx >= 0 {
			absIdx := idx + baseOffset
			if absIdx < endIdx {
				endIdx = absIdx
			}
		}
	}

	publicBlock := source[pubIdx : pubIdx+endIdx]
	re := regexp.MustCompile(`(?im)\bMETHODS\s+(\w+)`)
	matches := re.FindAllStringSubmatch(publicBlock, -1)
	for _, m := range matches {
		result[strings.ToUpper(m[1])] = true
	}
	return result
}

// extractInterfaceDef extracts the interface definition block.
func extractInterfaceDef(source string) string {
	upper := strings.ToUpper(source)
	startIdx := strings.Index(upper, "INTERFACE ")
	if startIdx < 0 {
		return ""
	}
	endIdx := strings.Index(upper[startIdx:], "ENDINTERFACE")
	if endIdx < 0 {
		return ""
	}
	return source[startIdx : startIdx+endIdx+len("ENDINTERFACE")]
}

// extractRaisingClauses extracts RAISING clauses from method definitions.
// Uses (?s) dot-all flag to handle multi-line METHODS definitions where
// RAISING appears on a different line than the METHODS keyword.
func extractRaisingClauses(source string) map[string]string {
	result := make(map[string]string)
	re := regexp.MustCompile(`(?ims)METHODS\s+(\w+)\b[\s\S]*?(RAISING\s+[\w\s]+?)\.`)
	matches := re.FindAllStringSubmatch(source, -1)
	for _, m := range matches {
		name := strings.ToUpper(m[1])
		result[name] = strings.TrimSpace(m[2])
	}
	return result
}

// parseObjectURIComponents extracts type and name from an ADT URI.
func parseObjectURIComponents(uri string) (string, string) {
	parts := strings.Split(uri, "/")
	if len(parts) < 2 {
		return "", ""
	}

	name := parts[len(parts)-1]

	// Detect type from URI path
	joined := strings.Join(parts, "/")
	switch {
	case strings.Contains(joined, "/oo/classes/"):
		return "CLAS", strings.ToUpper(name)
	case strings.Contains(joined, "/oo/interfaces/"):
		return "INTF", strings.ToUpper(name)
	case strings.Contains(joined, "/programs/programs/"):
		return "PROG", strings.ToUpper(name)
	case strings.Contains(joined, "/functions/groups/"):
		return "FUGR", strings.ToUpper(name)
	case strings.Contains(joined, "/ddic/ddl/sources/"):
		return "DDLS", strings.ToUpper(name)
	}
	return "", strings.ToUpper(name)
}

func normalizeMethodDef(s string) string {
	return normalizeWhitespace(strings.ToUpper(s))
}

var reWhitespace = regexp.MustCompile(`\s+`)

func normalizeWhitespace(s string) string {
	return reWhitespace.ReplaceAllString(strings.TrimSpace(s), " ")
}
