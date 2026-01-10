package adt

import (
	"context"
	"fmt"
	"strings"
)

// --- Surgical Edit Tools ---

// EditSourceResult represents the result of editing source code.
type EditSourceResult struct {
	Success      bool              `json:"success"`
	ObjectURL    string            `json:"objectUrl"`
	ObjectName   string            `json:"objectName"`
	MatchCount   int               `json:"matchCount"`
	OldString    string            `json:"oldString,omitempty"`
	NewString    string            `json:"newString,omitempty"`
	SyntaxErrors []string          `json:"syntaxErrors,omitempty"`
	Activation   *ActivationResult `json:"activation,omitempty"`
	Message      string            `json:"message,omitempty"`
	Method       string            `json:"method,omitempty"` // Method name if method-level edit
}

// EditSourceOptions provides optional parameters for EditSource.
type EditSourceOptions struct {
	ReplaceAll      bool   // If true, replace all occurrences; if false, require unique match
	SyntaxCheck     bool   // If true, validate syntax before saving (default: true if not set)
	CaseInsensitive bool   // If true, ignore case when matching
	Method          string // For CLAS only: constrain search/replace to this method only
}

// normalizeLineEndings converts CRLF to LF for consistent matching.
// SAP ADT returns source with \r\n but AI assistants typically send \n
func normalizeLineEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// countMatches counts occurrences of substring in s (case-sensitive or case-insensitive).
// Line endings are normalized (CRLF → LF) before comparison.
func countMatches(s, substr string, caseInsensitive bool) int {
	// Normalize line endings - SAP uses CRLF, AI sends LF
	s = normalizeLineEndings(s)
	substr = normalizeLineEndings(substr)

	if !caseInsensitive {
		return strings.Count(s, substr)
	}
	// Case-insensitive count
	count := 0
	sLower := strings.ToLower(s)
	substrLower := strings.ToLower(substr)
	pos := 0
	for {
		idx := strings.Index(sLower[pos:], substrLower)
		if idx == -1 {
			break
		}
		count++
		pos += idx + len(substrLower)
	}
	return count
}

// replaceMatches replaces occurrences of old with new in s (case-sensitive or case-insensitive).
// Line endings are normalized (CRLF → LF) for consistent matching.
func replaceMatches(s, old, new string, replaceAll, caseInsensitive bool) string {
	// Normalize line endings - SAP uses CRLF, AI sends LF
	s = normalizeLineEndings(s)
	old = normalizeLineEndings(old)
	new = normalizeLineEndings(new)

	if !caseInsensitive {
		if replaceAll {
			return strings.ReplaceAll(s, old, new)
		}
		return strings.Replace(s, old, new, 1)
	}
	// Case-insensitive replace
	sLower := strings.ToLower(s)
	oldLower := strings.ToLower(old)

	var result strings.Builder
	pos := 0
	replacements := 0

	for {
		idx := strings.Index(sLower[pos:], oldLower)
		if idx == -1 {
			result.WriteString(s[pos:])
			break
		}

		// Write everything before match
		result.WriteString(s[pos : pos+idx])
		// Write replacement
		result.WriteString(new)

		pos += idx + len(old)
		replacements++

		if !replaceAll && replacements >= 1 {
			result.WriteString(s[pos:])
			break
		}
	}

	return result.String()
}

// EditSource performs surgical string replacement on ABAP source code.
// This is a backward-compatible wrapper for EditSourceWithOptions.
func (c *Client) EditSource(ctx context.Context, objectURL, oldString, newString string, replaceAll, syntaxCheck, caseInsensitive bool) (*EditSourceResult, error) {
	return c.EditSourceWithOptions(ctx, objectURL, oldString, newString, &EditSourceOptions{
		ReplaceAll:      replaceAll,
		SyntaxCheck:     syntaxCheck,
		CaseInsensitive: caseInsensitive,
	})
}

// EditSourceWithOptions performs surgical string replacement on ABAP source code with options.
//
// This tool matches the Edit pattern used for local files, enabling surgical
// edits without downloading/uploading full source each time.
//
// Workflow: GetSource → FindReplace → SyntaxCheck → Lock → UpdateSource → Unlock → Activate
//
// Parameters:
//   - objectURL: ADT URL (e.g., /sap/bc/adt/programs/programs/ZTEST)
//   - oldString: Exact string to find (must be unique in source)
//   - newString: Replacement string
//   - opts: Optional parameters (ReplaceAll, SyntaxCheck, CaseInsensitive, Method)
//
// Method-level isolation (CLAS only):
//
//	When opts.Method is set, the search is constrained to the specified method only.
//	This prevents accidental edits in other methods when the same pattern exists elsewhere.
//
// Example:
//
//	EditSourceWithOptions(ctx, "/sap/bc/adt/oo/classes/ZCL_TEST",
//	  "METHOD foo.\n  ENDMETHOD.",
//	  "METHOD foo.\n  rv_result = 42.\n  ENDMETHOD.",
//	  &EditSourceOptions{Method: "FOO"})
func (c *Client) EditSourceWithOptions(ctx context.Context, objectURL, oldString, newString string, opts *EditSourceOptions) (*EditSourceResult, error) {
	// Safety check
	if err := c.checkSafety(OpUpdate, "EditSource"); err != nil {
		return nil, err
	}

	// Default options
	if opts == nil {
		opts = &EditSourceOptions{SyntaxCheck: true}
	}

	result := &EditSourceResult{
		ObjectURL: objectURL,
		OldString: oldString,
		NewString: newString,
	}

	// Extract object name from URL for error messages
	parts := strings.Split(objectURL, "/")
	if len(parts) > 0 {
		result.ObjectName = parts[len(parts)-1]
	}

	// Detect if this is a class URL (not an include)
	isClass := strings.Contains(objectURL, "/sap/bc/adt/oo/classes/") && !strings.Contains(objectURL, "/includes/")
	var classNameForMethod string
	if isClass && opts.Method != "" {
		// Extract class name for method-level isolation
		classesPrefix := "/sap/bc/adt/oo/classes/"
		if idx := strings.Index(objectURL, classesPrefix); idx >= 0 {
			rest := objectURL[idx+len(classesPrefix):]
			if slashIdx := strings.Index(rest, "/"); slashIdx > 0 {
				classNameForMethod = rest[:slashIdx]
			} else {
				classNameForMethod = rest
			}
		}
		result.Method = opts.Method
	}

	// Detect if this is a class include (e.g., /sap/bc/adt/oo/classes/ZCL_FOO/includes/testclasses)
	isClassInclude := strings.Contains(objectURL, "/includes/")
	var className string
	var includeType ClassIncludeType
	var parentClassURL string

	if isClassInclude {
		// Parse class name and include type from URL
		// URL format: /sap/bc/adt/oo/classes/{class_name}/includes/{include_type}
		includesIdx := strings.Index(objectURL, "/includes/")
		if includesIdx > 0 {
			classesPrefix := "/sap/bc/adt/oo/classes/"
			if strings.Contains(objectURL, classesPrefix) {
				classStart := strings.Index(objectURL, classesPrefix) + len(classesPrefix)
				className = objectURL[classStart:includesIdx]
				includeType = ClassIncludeType(objectURL[includesIdx+len("/includes/"):])
				parentClassURL = objectURL[:includesIdx]
			}
		}
	}

	// 1. Get current source
	// For class includes, the source is accessed directly without /source/main suffix
	sourceURL := objectURL
	if !isClassInclude && !strings.HasSuffix(sourceURL, "/source/main") {
		sourceURL = objectURL + "/source/main"
	}

	resp, err := c.transport.Request(ctx, sourceURL, &RequestOptions{
		Method: "GET",
		Accept: "text/plain",
	})
	if err != nil {
		result.Message = fmt.Sprintf("Failed to read source: %v", err)
		return result, nil
	}
	source := string(resp.Body)

	// Method-level isolation: constrain search to the specified method only
	var methodStart, methodEnd int
	if classNameForMethod != "" && opts.Method != "" {
		var methods []MethodInfo
		methods, err = c.GetClassMethods(ctx, classNameForMethod)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to get class methods: %v", err)
			return result, nil
		}

		// Find the specified method
		var foundMethod *MethodInfo
		methodNameUpper := strings.ToUpper(opts.Method)
		for i := range methods {
			if methods[i].Name == methodNameUpper {
				foundMethod = &methods[i]
				break
			}
		}
		if foundMethod == nil {
			result.Message = fmt.Sprintf("Method %s not found in class %s", opts.Method, classNameForMethod)
			return result, nil
		}

		methodStart = foundMethod.ImplementationStart
		methodEnd = foundMethod.ImplementationEnd

		// Extract method source for match counting
		sourceLines := strings.Split(source, "\n")
		if methodEnd > len(sourceLines) {
			methodEnd = len(sourceLines)
		}
		if methodStart < 1 {
			methodStart = 1
		}
		methodSource := strings.Join(sourceLines[methodStart-1:methodEnd], "\n")

		// Count matches in method source only
		matchCount := countMatches(methodSource, oldString, opts.CaseInsensitive)
		result.MatchCount = matchCount

		if matchCount == 0 {
			if opts.CaseInsensitive {
				result.Message = fmt.Sprintf("old_string not found in method %s (case-insensitive). Check for exact match.", opts.Method)
			} else {
				result.Message = fmt.Sprintf("old_string not found in method %s. Check for exact match.", opts.Method)
			}
			return result, nil
		}

		if !opts.ReplaceAll && matchCount > 1 {
			result.Message = fmt.Sprintf("old_string matches %d locations in method %s (not unique). Set replaceAll=true or include more context.", matchCount, opts.Method)
			return result, nil
		}

		// Apply replacement only within method boundaries
		newMethodSource := replaceMatches(methodSource, oldString, newString, opts.ReplaceAll, opts.CaseInsensitive)

		// Reconstruct full source with the edited method
		var newSourceLines []string
		newSourceLines = append(newSourceLines, sourceLines[:methodStart-1]...)
		newSourceLines = append(newSourceLines, strings.Split(newMethodSource, "\n")...)
		newSourceLines = append(newSourceLines, sourceLines[methodEnd:]...)
		source = strings.Join(newSourceLines, "\n")
	} else {
		// Non-method edit: check match count in full source
		matchCount := countMatches(source, oldString, opts.CaseInsensitive)
		result.MatchCount = matchCount

		if matchCount == 0 {
			if opts.CaseInsensitive {
				result.Message = "old_string not found in source (case-insensitive). Check for exact match (including whitespace, line breaks)."
			} else {
				result.Message = "old_string not found in source. Check for exact match (including whitespace, line breaks, case)."
			}
			return result, nil
		}

		if !opts.ReplaceAll && matchCount > 1 {
			result.Message = fmt.Sprintf("old_string matches %d locations (not unique). Set replaceAll=true to replace all, or include more surrounding context to make match unique.", matchCount)
			return result, nil
		}

		// Apply replacement
		source = replaceMatches(source, oldString, newString, opts.ReplaceAll, opts.CaseInsensitive)
	}

	newSource := source

	// 4. Optional syntax check
	if opts.SyntaxCheck {
		// For class includes, pass the include URL directly - SyntaxCheck handles it
		var syntaxErrors []SyntaxCheckResult
		syntaxErrors, err = c.SyntaxCheck(ctx, objectURL, newSource)
		if err != nil {
			result.Message = fmt.Sprintf("Syntax check failed: %v", err)
			return result, nil
		}

		if len(syntaxErrors) > 0 {
			// Convert to string messages
			errorMsgs := make([]string, len(syntaxErrors))
			for i, e := range syntaxErrors {
				errorMsgs[i] = fmt.Sprintf("Line %d: %s", e.Line, e.Text)
			}
			result.SyntaxErrors = errorMsgs
			result.Message = fmt.Sprintf("Edit would introduce %d syntax errors. Changes NOT saved.", len(syntaxErrors))
			return result, nil
		}
	}

	// 5. Lock object (for class includes, lock the parent class)
	lockURL := objectURL
	if isClassInclude && parentClassURL != "" {
		lockURL = parentClassURL
	}
	lockResult, err := c.LockObject(ctx, lockURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock object: %v", err)
		return result, nil
	}

	// Ensure unlock
	unlocked := false
	defer func() {
		if !unlocked {
			_ = c.UnlockObject(ctx, lockURL, lockResult.LockHandle)
		}
	}()

	// 6. Update source
	if isClassInclude && className != "" {
		// Use UpdateClassInclude for class includes
		err = c.UpdateClassInclude(ctx, className, includeType, newSource, lockResult.LockHandle, "")
	} else {
		err = c.UpdateSource(ctx, sourceURL, newSource, lockResult.LockHandle, "")
	}
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		return result, nil
	}

	// 7. Unlock
	err = c.UnlockObject(ctx, lockURL, lockResult.LockHandle)
	unlocked = true
	if err != nil {
		result.Message = fmt.Sprintf("Source updated but unlock failed: %v", err)
		return result, nil
	}

	// 8. Activate (for class includes, activate the parent class)
	activateURL := objectURL
	activateName := result.ObjectName
	if isClassInclude && parentClassURL != "" && className != "" {
		activateURL = parentClassURL
		activateName = className
	}
	activation, err := c.Activate(ctx, activateURL, activateName)
	if err != nil {
		result.Message = fmt.Sprintf("Source updated but activation failed: %v", err)
		return result, nil
	}
	result.Activation = activation

	result.Success = true
	if opts.Method != "" {
		result.Message = fmt.Sprintf("Successfully edited method %s and activated %s", opts.Method, result.ObjectName)
	} else if opts.ReplaceAll {
		result.Message = fmt.Sprintf("Successfully replaced %d occurrences and activated %s", result.MatchCount, result.ObjectName)
	} else {
		result.Message = fmt.Sprintf("Successfully edited and activated %s", result.ObjectName)
	}
	return result, nil
}
