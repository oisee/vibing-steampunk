package graph

import (
	"fmt"
	"sort"
	"strings"
)

// UsageTarget describes what we're looking for usage examples of.
type UsageTarget struct {
	ObjectType string // FUNC, CLAS, INTF, PROG
	ObjectName string // Z_CALCULATE_TAX, ZCL_TRAVEL, ZREPORT
	Method     string // GET_DATA (for class/interface methods, empty otherwise)
	Form       string // BUILD_OUTPUT (for FORM in program, empty otherwise)
}

// CanonicalPattern returns the search pattern used to find call sites in source.
func (t *UsageTarget) CanonicalPattern() string {
	switch strings.ToUpper(t.ObjectType) {
	case "FUNC":
		return t.ObjectName // CALL FUNCTION 'Z_CALCULATE_TAX'
	case "CLAS":
		if t.Method != "" {
			return t.Method // ->get_data( or =>get_data(
		}
		return t.ObjectName // NEW zcl_travel or TYPE REF TO zcl_travel
	case "INTF":
		if t.Method != "" {
			return t.Method
		}
		return t.ObjectName
	case "SUBMIT", "PROG":
		if t.Form != "" {
			return t.Form // PERFORM form IN PROGRAM
		}
		return t.ObjectName // SUBMIT prog
	default:
		return t.ObjectName
	}
}

// UsageExample represents a single usage example found in caller source code.
type UsageExample struct {
	CallerID   string `json:"caller_id"`             // CLAS:ZCL_ORDER_SERVICE
	CallerName string `json:"caller_name"`           // ZCL_ORDER_SERVICE
	CallerType string `json:"caller_type"`           // CLAS
	Package    string `json:"package,omitempty"`      // $ZDEV
	IsTest     bool   `json:"is_test"`               // true if test class/include
	Snippet    string `json:"snippet"`               // source lines with line numbers
	LineNumber int    `json:"line_number"`            // line of the call site
	MatchType  string `json:"match_type"`            // CALL_FUNCTION, METHOD_CALL, SUBMIT, PERFORM, GREP
	Confidence string `json:"confidence"`            // HIGH (parser/exact), MEDIUM (grep)
}

// UsageExamplesResult is the result of FindUsageExamples.
type UsageExamplesResult struct {
	Target       UsageTarget    `json:"target"`
	TotalCallers int            `json:"total_callers"` // total before cap
	Examples     []UsageExample `json:"examples"`
}

// CallerSource pairs a caller's identity with its source code for snippet extraction.
type CallerSource struct {
	NodeID   string // CLAS:ZCL_ORDER_SERVICE
	Name     string // ZCL_ORDER_SERVICE
	Type     string // CLAS
	Package  string // $ZDEV
	IsTest   bool   // test class/include
	Source   string // full source code
}

// FindUsageExamples extracts usage snippets from caller source code.
// This is the pure core: it takes pre-fetched caller sources and extracts
// call sites matching the target. No I/O — caller discovery and source fetch
// are the responsibility of CLI/MCP handlers.
func FindUsageExamples(target UsageTarget, callers []CallerSource, maxExamples int) *UsageExamplesResult {
	if maxExamples <= 0 {
		maxExamples = 10
	}

	result := &UsageExamplesResult{
		Target:       target,
		TotalCallers: len(callers),
	}

	var examples []UsageExample

	for _, caller := range callers {
		found := extractCallSites(target, caller)
		examples = append(examples, found...)
	}

	// Rank examples
	sort.Slice(examples, func(i, j int) bool {
		return exampleRank(examples[i]) < exampleRank(examples[j])
	})

	// Cap
	if len(examples) > maxExamples {
		examples = examples[:maxExamples]
	}

	result.Examples = examples
	return result
}

// extractCallSites finds call sites in a single caller's source that match the target.
func extractCallSites(target UsageTarget, caller CallerSource) []UsageExample {
	lines := strings.Split(caller.Source, "\n")
	targetType := strings.ToUpper(target.ObjectType)
	var examples []UsageExample

	for i, line := range lines {
		lineUpper := strings.ToUpper(line)
		lineNum := i + 1

		matchType := ""

		// Skip comment lines before any pattern matching
		if isCommentLine(line) {
			continue
		}

		switch targetType {
		case "FUNC":
			// CALL FUNCTION 'FM_NAME'
			if matchCallFunction(lineUpper, target.ObjectName) {
				matchType = "CALL_FUNCTION"
			}

		case "CLAS", "INTF":
			if target.Method != "" {
				// zcl_class=>method( or zcl_class->method( or zif~method(
				if matchMethodCall(lineUpper, target.ObjectName, target.Method) {
					matchType = "METHOD_CALL"
				}
			} else {
				// NEW zcl_class, TYPE REF TO zcl_class, CREATE OBJECT TYPE zcl_class
				if matchClassReference(lineUpper, target.ObjectName) {
					matchType = "CLASS_REFERENCE"
				}
			}

		case "SUBMIT":
			// SUBMIT prog_name
			if matchSubmit(lineUpper, target.ObjectName) {
				matchType = "SUBMIT"
			}

		case "PROG":
			if target.Form != "" {
				// PERFORM form IN PROGRAM prog
				if matchPerformInProgram(lineUpper, target.ObjectName, target.Form) {
					matchType = "PERFORM"
				}
			} else {
				// SUBMIT prog
				if matchSubmit(lineUpper, target.ObjectName) {
					matchType = "SUBMIT"
				}
			}
		}

		if matchType == "" {
			// Grep fallback: literal name match (case-insensitive)
			pattern := strings.ToUpper(target.CanonicalPattern())
			if strings.Contains(lineUpper, pattern) {
				matchType = "GREP"
			}
		}

		if matchType != "" {
			snippet := extractSnippet(lines, i, 3)
			confidence := "HIGH"
			if matchType == "GREP" {
				confidence = "MEDIUM"
			}

			examples = append(examples, UsageExample{
				CallerID:   caller.NodeID,
				CallerName: caller.Name,
				CallerType: caller.Type,
				Package:    caller.Package,
				IsTest:     caller.IsTest,
				Snippet:    snippet,
				LineNumber: lineNum,
				MatchType:  matchType,
				Confidence: confidence,
			})
		}
	}

	return examples
}

// --- Pattern matchers ---

// matchCallFunction checks for CALL FUNCTION 'FM_NAME'
func matchCallFunction(lineUpper, fmName string) bool {
	fmUpper := strings.ToUpper(fmName)
	// CALL FUNCTION 'FM_NAME'
	return strings.Contains(lineUpper, "CALL FUNCTION") &&
		strings.Contains(lineUpper, "'"+fmUpper+"'")
}

// matchMethodCall checks for class=>method( or class->method( or intf~method(
func matchMethodCall(lineUpper, className, methodName string) bool {
	classUpper := strings.ToUpper(className)
	methodUpper := strings.ToUpper(methodName)

	// Static call: ZCL_FOO=>GET_DATA
	if strings.Contains(lineUpper, classUpper+"=>"+methodUpper) {
		return true
	}
	// Instance call with known class: ...->METHOD_NAME (less precise, but common)
	// We check if class name appears nearby AND method name is on this line
	if strings.Contains(lineUpper, "->"+methodUpper) {
		return true
	}
	// Interface method call: ZIF_API~METHOD_NAME
	if strings.Contains(lineUpper, classUpper+"~"+methodUpper) {
		return true
	}
	return false
}

// matchClassReference checks for NEW class, TYPE REF TO class, CREATE OBJECT TYPE class
func matchClassReference(lineUpper, className string) bool {
	classUpper := strings.ToUpper(className)
	if strings.Contains(lineUpper, "NEW "+classUpper) {
		return true
	}
	if strings.Contains(lineUpper, "TYPE REF TO "+classUpper) {
		return true
	}
	if strings.Contains(lineUpper, "TYPE "+classUpper) && strings.Contains(lineUpper, "CREATE OBJECT") {
		return true
	}
	return false
}

// matchSubmit checks for SUBMIT prog_name
func matchSubmit(lineUpper, progName string) bool {
	progUpper := strings.ToUpper(progName)
	return strings.Contains(lineUpper, "SUBMIT") && strings.Contains(lineUpper, progUpper)
}

// matchPerformInProgram checks for PERFORM form IN PROGRAM prog
func matchPerformInProgram(lineUpper, progName, formName string) bool {
	progUpper := strings.ToUpper(progName)
	formUpper := strings.ToUpper(formName)
	return strings.Contains(lineUpper, "PERFORM") &&
		strings.Contains(lineUpper, formUpper) &&
		strings.Contains(lineUpper, "PROGRAM") &&
		strings.Contains(lineUpper, progUpper)
}

// --- Helpers ---

// extractSnippet extracts N lines of context around a target line.
func extractSnippet(lines []string, targetIdx, contextLines int) string {
	start := targetIdx - contextLines
	if start < 0 {
		start = 0
	}
	end := targetIdx + contextLines + 1
	if end > len(lines) {
		end = len(lines)
	}

	var sb strings.Builder
	for i := start; i < end; i++ {
		sb.WriteString(fmt.Sprintf("  %4d | %s\n", i+1, lines[i]))
	}
	return sb.String()
}

// isCommentLine checks if a line is an ABAP comment.
func isCommentLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "\"")
}

// IsTestCaller detects if a caller is a test class/include by naming convention.
func IsTestCaller(name, includeType string) bool {
	upper := strings.ToUpper(name)
	if strings.Contains(upper, "TEST") {
		return true
	}
	if strings.HasPrefix(upper, "LCL_TEST") || strings.HasPrefix(upper, "LTH_") {
		return true
	}
	incUpper := strings.ToUpper(includeType)
	if incUpper == "CCAU" || incUpper == "TESTCLASSES" {
		return true
	}
	return false
}

// exampleRank returns a sort key (lower = better).
func exampleRank(e UsageExample) int {
	rank := 0

	// Test classes first (cleanest examples)
	if !e.IsTest {
		rank += 100
	}

	// HIGH confidence before MEDIUM
	if e.Confidence == "MEDIUM" {
		rank += 50
	}

	// Specific match types before grep
	switch e.MatchType {
	case "CALL_FUNCTION":
		rank += 0
	case "METHOD_CALL":
		rank += 1
	case "SUBMIT":
		rank += 2
	case "PERFORM":
		rank += 3
	case "CLASS_REFERENCE":
		rank += 10
	case "GREP":
		rank += 20
	}

	return rank
}
