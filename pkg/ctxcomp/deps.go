package ctxcomp

import (
	"regexp"
	"strings"
)

// namePattern matches ABAP object names including namespaced ones like /DMF/CL_FOO.
const namePattern = `([a-z/][a-z0-9_/]*)`

// Pre-compiled regexes for dependency extraction.
var (
	reTypeRefTo    = regexp.MustCompile(`(?i)\bTYPE\s+REF\s+TO\s+` + namePattern)
	reNew          = regexp.MustCompile(`(?i)\bNEW\s+` + namePattern + `\s*\(`)
	reStaticCall   = regexp.MustCompile(`(?i)` + namePattern + `=>`)
	reIntfMethod   = regexp.MustCompile(`(?i)` + namePattern + `~`)
	reInheriting   = regexp.MustCompile(`(?i)\bINHERITING\s+FROM\s+` + namePattern)
	reInterfaces   = regexp.MustCompile(`(?i)\bINTERFACES\s+` + namePattern)
	reCallFunction = regexp.MustCompile(`(?i)\bCALL\s+FUNCTION\s+'([^']+)'`)
	reCast         = regexp.MustCompile(`(?i)\bCAST\s+` + namePattern + `\s*\(`)
	reRaising      = regexp.MustCompile(`(?i)\bRAISING\s+` + namePattern)
	reExceptionRef = regexp.MustCompile(`(?i)\b(z[a-z]*cx_[a-z0-9_]+|ycx_[a-z0-9_]+)`)  // ZCX_*, ZFCX_*, YCX_* exception classes
)

// builtinTypes are ABAP built-in types that should never appear as dependencies.
var builtinTypes = map[string]bool{
	"STRING": true, "I": true, "C": true, "N": true,
	"D": true, "T": true, "X": true, "XSTRING": true,
	"ABAP_BOOL": true, "ABAP_TRUE": true, "ABAP_FALSE": true,
	"ANY": true, "DATA": true, "REF": true, "OBJECT": true,
	"INT8": true, "DECFLOAT16": true, "DECFLOAT34": true, "P": true, "F": true,
	"CLIKE": true, "CSEQUENCE": true, "XSEQUENCE": true, "NUMERIC": true,
	"SIMPLE": true,
}

// standardPrefixes are SAP standard prefixes that are typically too large
// and not useful for context compression.
var standardSkipPrefixes = []string{
	"CL_ABAP_", "IF_ABAP_",
	"CX_SY_", "CX_DYNAMIC_CHECK", "CX_STATIC_CHECK", "CX_NO_CHECK",
}

// ExtractDependencies scans ABAP source and returns referenced external objects.
// It deduplicates by name, skips built-in types, and classifies each dependency.
func ExtractDependencies(source string) []Dependency {
	seen := make(map[string]*Dependency)
	lines := strings.Split(source, "\n")

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1

		// Skip comment lines
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "\"") {
			continue
		}

		// TYPE REF TO <name>
		for _, m := range reTypeRefTo.FindAllStringSubmatch(line, -1) {
			addDep(seen, m[1], inferKind(m[1]), lineNum)
		}

		// NEW <name>(
		for _, m := range reNew.FindAllStringSubmatch(line, -1) {
			addDep(seen, m[1], KindClass, lineNum)
		}

		// <name>=>
		for _, m := range reStaticCall.FindAllStringSubmatch(line, -1) {
			addDep(seen, m[1], inferKind(m[1]), lineNum)
		}

		// <name>~
		for _, m := range reIntfMethod.FindAllStringSubmatch(line, -1) {
			addDep(seen, m[1], KindInterface, lineNum)
		}

		// INHERITING FROM <name>
		for _, m := range reInheriting.FindAllStringSubmatch(line, -1) {
			addDep(seen, m[1], KindClass, lineNum)
		}

		// INTERFACES <name>
		for _, m := range reInterfaces.FindAllStringSubmatch(line, -1) {
			addDep(seen, m[1], KindInterface, lineNum)
		}

		// CALL FUNCTION '<name>'
		for _, m := range reCallFunction.FindAllStringSubmatch(line, -1) {
			addDep(seen, m[1], KindFunction, lineNum)
		}

		// CAST <name>(
		for _, m := range reCast.FindAllStringSubmatch(line, -1) {
			addDep(seen, m[1], inferKind(m[1]), lineNum)
		}

		// RAISING <name> (same line)
		for _, m := range reRaising.FindAllStringSubmatch(line, -1) {
			addDep(seen, m[1], KindClass, lineNum)
		}

		// Exception class references (ZCX_*, YCX_* — often on their own line after RAISING)
		for _, m := range reExceptionRef.FindAllStringSubmatch(line, -1) {
			addDep(seen, m[1], KindClass, lineNum)
		}
	}

	// Collect and sort: custom (Z*/Y*) first, then others
	result := make([]Dependency, 0, len(seen))
	for _, dep := range seen {
		result = append(result, *dep)
	}

	return result
}

func addDep(seen map[string]*Dependency, name string, kind DependencyKind, line int) {
	upper := strings.ToUpper(name)
	if shouldSkip(upper) {
		return
	}
	if existing, exists := seen[upper]; !exists {
		seen[upper] = &Dependency{Name: upper, Kind: kind, Line: line}
	} else {
		// INTF wins over CLAS when there's conflicting evidence (e.g. INTERFACES keyword)
		if kind == KindInterface && existing.Kind == KindClass {
			existing.Kind = KindInterface
		}
	}
}

func shouldSkip(name string) bool {
	if builtinTypes[name] {
		return true
	}
	for _, prefix := range standardSkipPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// inferKind guesses whether a name is a class or interface based on naming conventions.
func inferKind(name string) DependencyKind {
	upper := strings.ToUpper(name)
	// Interface patterns: ZIF_*, IF_*, /NS/IF_*
	if strings.HasPrefix(upper, "ZIF_") || strings.HasPrefix(upper, "YIF_") || strings.HasPrefix(upper, "IF_") {
		return KindInterface
	}
	// Check for /NS/IF_ pattern
	if idx := strings.LastIndex(upper, "/"); idx >= 0 {
		after := upper[idx+1:]
		if strings.HasPrefix(after, "IF_") {
			return KindInterface
		}
	}
	return KindClass
}
