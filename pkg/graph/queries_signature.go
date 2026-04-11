package graph

import (
	"fmt"
	"sort"
	"strings"
)

// MethodParam represents a single method parameter.
type MethodParam struct {
	Name      string `json:"name"`
	Direction string `json:"direction"` // IMPORTING, EXPORTING, CHANGING, RETURNING
	Type      string `json:"type"`      // TYPE/type reference
	Optional  bool   `json:"optional,omitempty"`
	Default   string `json:"default,omitempty"` // DEFAULT value if present
}

// MethodSignature represents the full signature of a method.
type MethodSignature struct {
	ClassName  string        `json:"class_name"`
	MethodName string        `json:"method_name"`
	Visibility string        `json:"visibility,omitempty"` // PUBLIC, PROTECTED, PRIVATE
	Level      string        `json:"level,omitempty"`      // instance, static, or empty
	IsAbstract bool          `json:"is_abstract,omitempty"`
	IsFinal    bool          `json:"is_final,omitempty"`
	IsRedefined bool         `json:"is_redefined,omitempty"`
	Params     []MethodParam `json:"params"`
	Raising    []string      `json:"raising,omitempty"` // Exception classes
	RawDef     string        `json:"raw_def,omitempty"` // Original definition text
}

// ExtractMethodSignature extracts a method signature from class definition source.
// Parses the METHODS/CLASS-METHODS statement for the target method.
// Source should be the class definition (DEFINITION part), not implementation.
func ExtractMethodSignature(className, methodName, source string) *MethodSignature {
	classUpper := strings.ToUpper(strings.TrimSpace(className))
	methodUpper := strings.ToUpper(strings.TrimSpace(methodName))

	sig := &MethodSignature{
		ClassName:  classUpper,
		MethodName: methodUpper,
	}

	// Find the method definition statement in source
	// Patterns: METHODS method_name ..., CLASS-METHODS method_name ...
	rawDef := findMethodDefinition(source, methodUpper)
	if rawDef == "" {
		return sig // empty sig means method not found in definition
	}
	sig.RawDef = rawDef

	// Detect visibility from section context
	sig.Visibility = detectVisibility(source, methodUpper)

	// Detect level
	defUpper := strings.ToUpper(rawDef)
	if strings.HasPrefix(strings.TrimSpace(defUpper), "CLASS-METHODS") {
		sig.Level = "static"
	} else {
		sig.Level = "instance"
	}

	// Detect modifiers
	if strings.Contains(defUpper, " ABSTRACT") {
		sig.IsAbstract = true
	}
	if strings.Contains(defUpper, " FINAL") {
		sig.IsFinal = true
	}
	if strings.Contains(defUpper, " REDEFINITION") {
		sig.IsRedefined = true
		return sig // redefinitions have no params in definition
	}

	// Extract parameters by direction
	sig.Params = append(sig.Params, extractParamBlock(rawDef, "IMPORTING")...)
	sig.Params = append(sig.Params, extractParamBlock(rawDef, "EXPORTING")...)
	sig.Params = append(sig.Params, extractParamBlock(rawDef, "CHANGING")...)
	sig.Params = append(sig.Params, extractReturning(rawDef)...)

	// Extract RAISING
	sig.Raising = extractRaising(rawDef)

	return sig
}

// FormatMethodSignature renders a human-readable signature.
func FormatMethodSignature(sig *MethodSignature) string {
	var sb strings.Builder

	// Header
	level := ""
	if sig.Level == "static" {
		level = "CLASS-"
	}
	mods := ""
	if sig.IsAbstract {
		mods += " ABSTRACT"
	}
	if sig.IsFinal {
		mods += " FINAL"
	}
	if sig.IsRedefined {
		mods += " REDEFINITION"
	}

	sb.WriteString(fmt.Sprintf("%s %sMETHODS %s%s\n", sig.Visibility, level, sig.MethodName, mods))

	// Group params by direction
	byDir := map[string][]MethodParam{}
	for _, p := range sig.Params {
		byDir[p.Direction] = append(byDir[p.Direction], p)
	}

	for _, dir := range []string{"IMPORTING", "EXPORTING", "CHANGING", "RETURNING"} {
		params := byDir[dir]
		if len(params) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("  %s\n", dir))
		for _, p := range params {
			opt := ""
			if p.Optional {
				opt = " OPTIONAL"
			}
			def := ""
			if p.Default != "" {
				def = " DEFAULT " + p.Default
			}
			sb.WriteString(fmt.Sprintf("    %s TYPE %s%s%s\n", p.Name, p.Type, opt, def))
		}
	}

	if len(sig.Raising) > 0 {
		sb.WriteString(fmt.Sprintf("  RAISING %s\n", strings.Join(sig.Raising, " ")))
	}

	return sb.String()
}

// --- Internal parsers ---

// findMethodDefinition extracts the full METHODS/CLASS-METHODS statement for a method.
func findMethodDefinition(source, methodUpper string) string {
	lines := strings.Split(source, "\n")

	// Build a single string of all non-comment lines for multi-line statement matching
	var cleanLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "\"") {
			continue
		}
		cleanLines = append(cleanLines, line)
	}
	clean := strings.Join(cleanLines, "\n")
	cleanUpper := strings.ToUpper(clean)

	// Find "METHODS method_name" or "CLASS-METHODS method_name"
	// Search CLASS-METHODS first to avoid matching the "METHODS" substring within "CLASS-METHODS"
	patterns := []string{
		"CLASS-METHODS " + methodUpper,
		"METHODS " + methodUpper,
	}

	for _, pat := range patterns {
		idx := strings.Index(cleanUpper, pat)
		if idx < 0 {
			continue
		}

		// Verify left word boundary: must be start of line or preceded by whitespace
		if idx > 0 {
			prev := cleanUpper[idx-1]
			if prev != ' ' && prev != '\n' && prev != '\r' && prev != '\t' {
				continue // e.g., "CLASS-METHODS" matched at "METHODS" inside it
			}
		}

		// Verify right word boundary (not a prefix of another method)
		endIdx := idx + len(pat)
		if endIdx < len(cleanUpper) {
			next := cleanUpper[endIdx]
			if next != ' ' && next != '\n' && next != '\r' && next != '.' && next != ',' {
				continue
			}
		}

		// Extract from match to next period (statement end)
		remaining := clean[idx:]
		dotIdx := strings.Index(remaining, ".")
		if dotIdx < 0 {
			dotIdx = len(remaining)
		}
		return strings.TrimSpace(remaining[:dotIdx])
	}

	return ""
}

// detectVisibility finds which section a method is defined in.
func detectVisibility(source, methodUpper string) string {
	sourceUpper := strings.ToUpper(source)

	// Find section boundaries
	pubIdx := strings.Index(sourceUpper, "PUBLIC SECTION")
	proIdx := strings.Index(sourceUpper, "PROTECTED SECTION")
	priIdx := strings.Index(sourceUpper, "PRIVATE SECTION")

	// Find method position
	methodIdx := -1
	for _, pat := range []string{"METHODS " + methodUpper, "CLASS-METHODS " + methodUpper} {
		idx := strings.Index(sourceUpper, pat)
		if idx >= 0 {
			methodIdx = idx
			break
		}
	}

	if methodIdx < 0 {
		return ""
	}

	// Determine which section the method falls in
	vis := "PUBLIC"
	if pubIdx >= 0 && methodIdx > pubIdx {
		vis = "PUBLIC"
	}
	if proIdx >= 0 && methodIdx > proIdx {
		vis = "PROTECTED"
	}
	if priIdx >= 0 && methodIdx > priIdx {
		vis = "PRIVATE"
	}

	return vis
}

// extractParamBlock extracts parameters for a given direction (IMPORTING/EXPORTING/CHANGING).
func extractParamBlock(rawDef, direction string) []MethodParam {
	defUpper := strings.ToUpper(rawDef)
	dirUpper := strings.ToUpper(direction)

	idx := strings.Index(defUpper, dirUpper)
	if idx < 0 {
		return nil
	}

	// Find the block: from direction keyword to next direction/RAISING/end
	blockStart := idx + len(dirUpper)
	blockEnd := len(rawDef)

	for _, next := range []string{"IMPORTING", "EXPORTING", "CHANGING", "RETURNING", "RAISING"} {
		if next == dirUpper {
			continue
		}
		nextIdx := strings.Index(defUpper[blockStart:], next)
		if nextIdx >= 0 && blockStart+nextIdx < blockEnd {
			blockEnd = blockStart + nextIdx
		}
	}

	block := strings.TrimSpace(rawDef[blockStart:blockEnd])
	return parseParamList(block, direction)
}

// extractReturning extracts RETURNING VALUE(...) TYPE ... parameter.
func extractReturning(rawDef string) []MethodParam {
	defUpper := strings.ToUpper(rawDef)
	idx := strings.Index(defUpper, "RETURNING")
	if idx < 0 {
		return nil
	}

	// Find block end
	blockStart := idx + len("RETURNING")
	blockEnd := len(rawDef)
	for _, next := range []string{"RAISING"} {
		nextIdx := strings.Index(defUpper[blockStart:], next)
		if nextIdx >= 0 {
			blockEnd = blockStart + nextIdx
		}
	}

	block := strings.TrimSpace(rawDef[blockStart:blockEnd])

	// RETURNING VALUE(rv_name) TYPE type_name
	blockUpper := strings.ToUpper(block)
	valueIdx := strings.Index(blockUpper, "VALUE(")
	if valueIdx < 0 {
		return nil
	}

	closeIdx := strings.Index(block[valueIdx:], ")")
	if closeIdx < 0 {
		return nil
	}

	name := strings.TrimSpace(block[valueIdx+6 : valueIdx+closeIdx])
	typeName := extractTypeName(block[valueIdx+closeIdx+1:])

	return []MethodParam{{
		Name:      strings.ToUpper(name),
		Direction: "RETURNING",
		Type:      typeName,
	}}
}

// extractRaising extracts exception class names from RAISING clause.
func extractRaising(rawDef string) []string {
	defUpper := strings.ToUpper(rawDef)
	idx := strings.Index(defUpper, "RAISING")
	if idx < 0 {
		return nil
	}

	block := strings.TrimSpace(rawDef[idx+len("RAISING"):])
	// Split by whitespace, filter to identifiers
	parts := strings.Fields(block)
	var raising []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.TrimRight(p, ".,"))
		if p != "" && isIdentifier(p) {
			raising = append(raising, strings.ToUpper(p))
		}
	}
	return raising
}

// parseParamList parses individual parameters from a block like:
// "iv_name TYPE string iv_other TYPE i OPTIONAL"
func parseParamList(block, direction string) []MethodParam {
	var params []MethodParam

	// Tokenize roughly: split by known parameter pattern "name TYPE ..."
	// This is a simplified parser — handles the common patterns
	blockUpper := strings.ToUpper(block)
	tokens := strings.Fields(block)
	tokensUpper := strings.Fields(blockUpper)

	i := 0
	for i < len(tokens) {
		// Look for: name TYPE type_ref [OPTIONAL] [DEFAULT value]
		if i+2 < len(tokensUpper) && tokensUpper[i+1] == "TYPE" {
			param := MethodParam{
				Name:      strings.ToUpper(tokens[i]),
				Direction: strings.ToUpper(direction),
				Type:      strings.ToUpper(tokens[i+2]),
			}
			i += 3

			// Check for OPTIONAL/DEFAULT
			for i < len(tokensUpper) {
				if tokensUpper[i] == "OPTIONAL" {
					param.Optional = true
					i++
				} else if tokensUpper[i] == "DEFAULT" && i+1 < len(tokens) {
					param.Default = tokens[i+1]
					i += 2
				} else {
					break
				}
			}

			params = append(params, param)
		} else if i+3 < len(tokensUpper) && tokensUpper[i+1] == "TYPE" && tokensUpper[i+2] == "REF" && tokensUpper[i+3] == "TO" && i+4 < len(tokens) {
			// name TYPE REF TO type_ref
			param := MethodParam{
				Name:      strings.ToUpper(tokens[i]),
				Direction: strings.ToUpper(direction),
				Type:      "REF TO " + strings.ToUpper(tokens[i+4]),
			}
			i += 5
			for i < len(tokensUpper) {
				if tokensUpper[i] == "OPTIONAL" {
					param.Optional = true
					i++
				} else if tokensUpper[i] == "DEFAULT" && i+1 < len(tokens) {
					param.Default = tokens[i+1]
					i += 2
				} else {
					break
				}
			}
			params = append(params, param)
		} else {
			i++ // skip unrecognized token
		}
	}

	// Sort params by name for stability
	sort.Slice(params, func(a, b int) bool { return params[a].Name < params[b].Name })

	return params
}

// extractTypeName pulls type name after "TYPE" keyword in remaining text.
func extractTypeName(text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	for i, f := range fields {
		if strings.ToUpper(f) == "TYPE" && i+1 < len(fields) {
			typeName := strings.ToUpper(strings.TrimRight(fields[i+1], ".,"))
			// Handle TYPE REF TO
			if typeName == "REF" && i+3 < len(fields) && strings.ToUpper(fields[i+2]) == "TO" {
				return "REF TO " + strings.ToUpper(strings.TrimRight(fields[i+3], ".,"))
			}
			return typeName
		}
	}
	return ""
}
