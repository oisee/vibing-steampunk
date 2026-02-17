package adt

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ABAPFileInfo contains parsed information about an ABAP source file.
type ABAPFileInfo struct {
	FilePath          string
	ObjectType        CreatableObjectType
	ObjectName        string
	ParentName        string           // For function modules: the function group name
	Description       string           // Parsed from comments if available
	ClassIncludeType  ClassIncludeType // For class includes (testclasses, definitions, etc.)
	HasDefinition     bool             // For classes
	HasImplementation bool
	HasTestClasses    bool
}

// extractFunctionGroupFromFilename extracts the function group name from abapGit-style filenames.
// Pattern: {fugr_name}.fugr.{func_name}.func.abap → FUGR_NAME
// Example: zvsp_report.fugr.z_vsp_run_report.func.abap → ZVSP_REPORT
// Example: #aif#util.fugr.#aif#func.func.abap → /AIF/UTIL (namespaced)
func extractFunctionGroupFromFilename(filePath string) string {
	baseName := filepath.Base(filePath)
	// Pattern: {fugr}.fugr.{func}.func.abap
	if strings.HasSuffix(strings.ToLower(baseName), ".func.abap") {
		// Find .fugr. in the filename
		lowerName := strings.ToLower(baseName)
		fugrIdx := strings.Index(lowerName, ".fugr.")
		if fugrIdx > 0 {
			name := strings.ToUpper(baseName[:fugrIdx])
			// Convert # back to / for namespaced objects (abapGit convention)
			name = strings.ReplaceAll(name, "#", "/")
			return name
		}
	}
	return ""
}

// extractClassNameFromFilename extracts the parent class name from abapGit-style filenames.
// Examples:
//   - zcl_foo.clas.testclasses.abap → ZCL_FOO
//   - zcl_foo.clas.locals_def.abap → ZCL_FOO
//   - zcl_foo.clas.locals_imp.abap → ZCL_FOO
//   - zcl_foo.clas.macros.abap → ZCL_FOO
//   - zcl_foo.clas.abap → ZCL_FOO
//   - #dmo#cl_flight.clas.abap → /DMO/CL_FLIGHT (namespaced)
func extractClassNameFromFilename(filePath string) string {
	baseName := filepath.Base(filePath)

	// Remove known suffixes in order of specificity
	suffixes := []string{
		".clas.testclasses.abap",
		".clas.locals_def.abap",
		".clas.locals_imp.abap",
		".clas.macros.abap",
		".clas.abap",
	}

	for _, suffix := range suffixes {
		if strings.HasSuffix(strings.ToLower(baseName), suffix) {
			name := baseName[:len(baseName)-len(suffix)]
			name = strings.ToUpper(name)
			// Convert # back to / for namespaced objects (abapGit convention)
			name = strings.ReplaceAll(name, "#", "/")
			return name
		}
	}

	return ""
}

// ParseABAPFile analyzes an ABAP source file and extracts metadata.
// It detects the object type from file extension and parses the content
// to extract the object name and other metadata.
func ParseABAPFile(filePath string) (*ABAPFileInfo, error) {
	// 1. Detect from extension
	ext := filepath.Ext(filePath)
	info := &ABAPFileInfo{FilePath: filePath}

	// Check for compound extensions
	// Note: More specific suffixes must come BEFORE less specific ones
	baseName := filepath.Base(filePath)
	switch {
	// Class includes (must be before .clas.abap)
	// For class includes, extract the class name from the filename, not content
	case strings.HasSuffix(baseName, ".clas.testclasses.abap"):
		info.ObjectType = ObjectTypeClass
		info.ClassIncludeType = ClassIncludeTestClasses
		info.ObjectName = extractClassNameFromFilename(filePath)
	case strings.HasSuffix(baseName, ".clas.locals_def.abap"):
		info.ObjectType = ObjectTypeClass
		info.ClassIncludeType = ClassIncludeDefinitions
		info.ObjectName = extractClassNameFromFilename(filePath)
	case strings.HasSuffix(baseName, ".clas.locals_imp.abap"):
		info.ObjectType = ObjectTypeClass
		info.ClassIncludeType = ClassIncludeImplementations
		info.ObjectName = extractClassNameFromFilename(filePath)
	case strings.HasSuffix(baseName, ".clas.macros.abap"):
		info.ObjectType = ObjectTypeClass
		info.ClassIncludeType = ClassIncludeMacros
		info.ObjectName = extractClassNameFromFilename(filePath)
	// Main class
	case strings.HasSuffix(baseName, ".clas.abap"):
		info.ObjectType = ObjectTypeClass
		info.ClassIncludeType = ClassIncludeMain
	case strings.HasSuffix(baseName, ".prog.abap"):
		info.ObjectType = ObjectTypeProgram
	case strings.HasSuffix(baseName, ".intf.abap"):
		info.ObjectType = ObjectTypeInterface
	case strings.HasSuffix(baseName, ".fugr.abap"):
		info.ObjectType = ObjectTypeFunctionGroup
	case strings.HasSuffix(baseName, ".func.abap"):
		info.ObjectType = ObjectTypeFunctionMod
		info.ParentName = extractFunctionGroupFromFilename(filePath)
	// RAP object types (ABAPGit-compatible extensions)
	case strings.HasSuffix(baseName, ".ddls.asddls"):
		info.ObjectType = ObjectTypeDDLS
	case strings.HasSuffix(baseName, ".bdef.asbdef"):
		info.ObjectType = ObjectTypeBDEF
	case strings.HasSuffix(baseName, ".srvd.srvdsrv"):
		info.ObjectType = ObjectTypeSRVD
	case ext == ".abap":
		// Generic .abap: detect from content
		return parseFromContent(filePath)
	default:
		return nil, fmt.Errorf("unsupported file extension: %s (expected .clas.abap, .clas.testclasses.abap, .clas.locals_def.abap, .clas.locals_imp.abap, .prog.abap, .intf.abap, .fugr.abap, .func.abap, .ddls.asddls, .bdef.asbdef, or .srvd.srvdsrv)", ext)
	}

	// 2. Parse file content to extract name and metadata
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	inComment := false

	for scanner.Scan() && lineNum < 200 { // Scan first 200 lines
		line := scanner.Text()
		lineNum++

		// Parse based on object type
		switch info.ObjectType {
		case ObjectTypeClass:
			// For class includes (testclasses, locals_def, etc.), the name is already
			// extracted from the filename. Only parse from content for main class files.
			if info.ObjectName == "" {
				if name := parseClassName(line); name != "" {
					info.ObjectName = name
				}
			}
			if strings.Contains(strings.ToUpper(line), "DEFINITION") {
				info.HasDefinition = true
			}
			if strings.Contains(strings.ToUpper(line), "IMPLEMENTATION") {
				info.HasImplementation = true
			}
			if strings.Contains(strings.ToUpper(line), "FOR TESTING") {
				info.HasTestClasses = true
			}

		case ObjectTypeProgram:
			if name := parseProgramName(line); name != "" {
				info.ObjectName = name
			}

		case ObjectTypeInterface:
			if name := parseInterfaceName(line); name != "" {
				info.ObjectName = name
			}

		case ObjectTypeFunctionGroup:
			if name := parseFunctionGroupName(line); name != "" {
				info.ObjectName = name
			}

		case ObjectTypeFunctionMod:
			if name := parseFunctionModuleName(line); name != "" {
				info.ObjectName = name
			}

		// RAP object types
		case ObjectTypeDDLS:
			if name := parseDDLSName(line); name != "" {
				info.ObjectName = name
			}

		case ObjectTypeBDEF:
			if name := parseBDEFName(line); name != "" {
				info.ObjectName = name
			}

		case ObjectTypeSRVD:
			if name := parseSRVDName(line); name != "" {
				info.ObjectName = name
			}
		}

		// Parse description from header comments
		trimmed := strings.TrimSpace(line)
		if info.Description == "" {
			if strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "\"") {
				comment := strings.TrimPrefix(trimmed, "*")
				comment = strings.TrimPrefix(comment, "\"")
				comment = strings.TrimSpace(comment)

				// Skip common patterns
				if comment != "" &&
					!strings.HasPrefix(comment, "-") &&
					!strings.HasPrefix(comment, "=") &&
					!strings.HasPrefix(comment, "*") &&
					!strings.Contains(strings.ToLower(comment), "author") &&
					!strings.Contains(strings.ToLower(comment), "date") &&
					len(comment) > 10 && len(comment) < 60 {
					info.Description = comment
					inComment = true
				}
			} else if inComment {
				inComment = false
			}
		}

		// Early exit if we have all required info
		if info.ObjectName != "" && info.Description != "" {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	if info.ObjectName == "" {
		return nil, fmt.Errorf("could not parse object name from file (expected CLASS/PROGRAM/INTERFACE/FUNCTION GROUP/FUNCTION statement in first 200 lines)")
	}

	// Provide default description if none found
	if info.Description == "" {
		info.Description = fmt.Sprintf("Generated from %s", filepath.Base(filePath))
	}

	return info, nil
}

// parseFromContent detects object type by scanning file content
func parseFromContent(filePath string) (*ABAPFileInfo, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.ToUpper(strings.TrimSpace(scanner.Text()))

		if strings.HasPrefix(line, "CLASS ") && strings.Contains(line, "DEFINITION") {
			// Re-parse with known type
			file.Close()
			return ParseABAPFile(filePath)
		}
		if strings.HasPrefix(line, "REPORT ") || strings.HasPrefix(line, "PROGRAM ") {
			file.Close()
			return ParseABAPFile(filePath)
		}
		if strings.HasPrefix(line, "INTERFACE ") {
			file.Close()
			return ParseABAPFile(filePath)
		}
		if strings.HasPrefix(line, "FUNCTION-POOL ") {
			file.Close()
			return ParseABAPFile(filePath)
		}
		if strings.HasPrefix(line, "FUNCTION ") {
			file.Close()
			return ParseABAPFile(filePath)
		}
	}

	return nil, fmt.Errorf("could not detect object type from file content")
}

// parseClassName extracts class name from CLASS <name> DEFINITION
func parseClassName(line string) string {
	re := regexp.MustCompile(`(?i)^\s*CLASS\s+([a-z0-9_/]+)\s+DEFINITION`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.ToUpper(matches[1])
	}
	return ""
}

// parseProgramName extracts program name from REPORT/PROGRAM statement
func parseProgramName(line string) string {
	re := regexp.MustCompile(`(?i)^\s*(REPORT|PROGRAM)\s+([a-z0-9_/]+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 2 {
		return strings.ToUpper(matches[2])
	}
	return ""
}

// parseInterfaceName extracts interface name from INTERFACE <name> DEFINITION
func parseInterfaceName(line string) string {
	re := regexp.MustCompile(`(?i)^\s*INTERFACE\s+([a-z0-9_/]+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.ToUpper(matches[1])
	}
	return ""
}

// parseFunctionGroupName extracts function group name from FUNCTION-POOL statement
func parseFunctionGroupName(line string) string {
	re := regexp.MustCompile(`(?i)^\s*FUNCTION-POOL\s+([a-z0-9_/]+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.ToUpper(matches[1])
	}
	return ""
}

// parseFunctionModuleName extracts function module name from FUNCTION statement
func parseFunctionModuleName(line string) string {
	re := regexp.MustCompile(`(?i)^\s*FUNCTION\s+([a-z0-9_/]+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.ToUpper(matches[1])
	}
	return ""
}

// parseDDLSName extracts CDS view name from "define view [entity] <name>" or "@AbapCatalog.viewEnhancementCategory"
func parseDDLSName(line string) string {
	// Pattern: define view [entity] NAME
	re := regexp.MustCompile(`(?i)^\s*define\s+view\s+(?:entity\s+)?([a-z0-9_/]+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.ToUpper(matches[1])
	}
	return ""
}

// parseBDEFName extracts behavior definition name from "define behavior for <name>"
func parseBDEFName(line string) string {
	// Pattern: define behavior for NAME
	re := regexp.MustCompile(`(?i)^\s*define\s+behavior\s+for\s+([a-z0-9_/]+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.ToUpper(matches[1])
	}
	return ""
}

// parseSRVDName extracts service definition name from "define service <name>"
func parseSRVDName(line string) string {
	// Pattern: define service NAME
	re := regexp.MustCompile(`(?i)^\s*define\s+service\s+([a-z0-9_/]+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.ToUpper(matches[1])
	}
	return ""
}
