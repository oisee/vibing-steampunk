package ctxcomp

import (
	"regexp"
	"strings"
)

var (
	reClassDef       = regexp.MustCompile(`(?i)^\s*CLASS\s+\S+\s+DEFINITION`)
	rePublicSection  = regexp.MustCompile(`(?i)^\s*PUBLIC\s+SECTION\s*\.`)
	reProtectedSect  = regexp.MustCompile(`(?i)^\s*PROTECTED\s+SECTION\s*\.`)
	rePrivateSection = regexp.MustCompile(`(?i)^\s*PRIVATE\s+SECTION\s*\.`)
	reEndClass       = regexp.MustCompile(`(?i)^\s*ENDCLASS\s*\.`)
	reClassImpl      = regexp.MustCompile(`(?i)^\s*CLASS\s+\S+\s+IMPLEMENTATION\s*\.`)
	reInterfaceDef   = regexp.MustCompile(`(?i)^\s*INTERFACE\s+\S+`)
	reEndInterface   = regexp.MustCompile(`(?i)^\s*ENDINTERFACE\s*\.`)
	reFunctionStart  = regexp.MustCompile(`(?i)^\s*FUNCTION\s+`)
	reEndFunction    = regexp.MustCompile(`(?i)^\s*ENDFUNCTION\s*\.`)
	reFMComment      = regexp.MustCompile(`^\*"`)
)

// ExtractContract extracts the public API surface from full ABAP source.
// For classes: CLASS DEFINITION line + PUBLIC SECTION only.
// For interfaces: full interface definition.
// For function modules: the signature comment block.
func ExtractContract(source string, kind DependencyKind) string {
	switch kind {
	case KindClass:
		return extractClassContract(source)
	case KindInterface:
		return extractInterfaceContract(source)
	case KindFunction:
		return extractFMContract(source)
	default:
		return source
	}
}

func extractClassContract(source string) string {
	lines := strings.Split(source, "\n")
	var out []string

	type state int
	const (
		stateSearchingDef state = iota
		stateInDef
		stateInPublic
		stateDone
	)

	st := stateSearchingDef
	for _, line := range lines {
		switch st {
		case stateSearchingDef:
			if reClassImpl.MatchString(line) {
				// Reached IMPLEMENTATION section — stop
				st = stateDone
			} else if reClassDef.MatchString(line) {
				out = append(out, line)
				st = stateInDef
			}

		case stateInDef:
			out = append(out, line)
			if rePublicSection.MatchString(line) {
				st = stateInPublic
			} else if reEndClass.MatchString(line) {
				// No public section found — definition-only class
				st = stateDone
			}

		case stateInPublic:
			if reProtectedSect.MatchString(line) || rePrivateSection.MatchString(line) {
				out = append(out, "ENDCLASS.")
				st = stateDone
			} else if reEndClass.MatchString(line) {
				out = append(out, line)
				st = stateDone
			} else {
				// Skip empty lines and pure comment-only lines for brevity
				trimmed := strings.TrimSpace(line)
				if trimmed == "" {
					continue
				}
				out = append(out, line)
			}

		case stateDone:
			// stop processing
		}
		if st == stateDone {
			break
		}
	}

	if len(out) == 0 {
		return ""
	}

	return strings.Join(out, "\n")
}

func extractInterfaceContract(source string) string {
	lines := strings.Split(source, "\n")
	var out []string
	inInterface := false

	for _, line := range lines {
		if !inInterface {
			if reInterfaceDef.MatchString(line) {
				inInterface = true
				out = append(out, line)
			}
			continue
		}

		out = append(out, line)
		if reEndInterface.MatchString(line) {
			break
		}
	}

	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n")
}

func extractFMContract(source string) string {
	lines := strings.Split(source, "\n")
	var out []string
	inFunction := false
	inSignatureComments := false
	pastSignature := false

	for _, line := range lines {
		if !inFunction {
			if reFunctionStart.MatchString(line) {
				inFunction = true
				out = append(out, line)
			}
			continue
		}

		if pastSignature {
			break
		}

		// FM source has *" comment lines at the top describing the interface.
		if reFMComment.MatchString(line) {
			inSignatureComments = true
			out = append(out, line)
			continue
		}

		// If we were in signature comments and hit a non-comment line, we're done
		if inSignatureComments {
			pastSignature = true
			continue
		}

		// Also capture IMPORTING/EXPORTING/CHANGING/TABLES/EXCEPTIONS keywords
		// (for FMs without *" comment blocks)
		trimmed := strings.TrimSpace(strings.ToUpper(line))
		if strings.HasPrefix(trimmed, "IMPORTING") ||
			strings.HasPrefix(trimmed, "EXPORTING") ||
			strings.HasPrefix(trimmed, "CHANGING") ||
			strings.HasPrefix(trimmed, "TABLES") ||
			strings.HasPrefix(trimmed, "EXCEPTIONS") ||
			strings.HasPrefix(trimmed, "RAISING") {
			out = append(out, line)
			continue
		}

		// If we hit ENDFUNCTION, stop
		if reEndFunction.MatchString(line) {
			break
		}

		// Not a signature line — we've reached the body
		pastSignature = true
	}

	if len(out) == 0 {
		return ""
	}
	out = append(out, "ENDFUNCTION.")
	return strings.Join(out, "\n")
}
