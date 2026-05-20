package adt

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// --- Grep/Search Tools ---

// GrepMatch represents a single match in a grep search.
type GrepMatch struct {
	LineNumber    int      `json:"lineNumber"`
	MatchedLine   string   `json:"matchedLine"`
	ContextBefore []string `json:"contextBefore,omitempty"`
	ContextAfter  []string `json:"contextAfter,omitempty"`
}

// GrepObjectResult represents the result of grepping a single ABAP object.
type GrepObjectResult struct {
	Success    bool        `json:"success"`
	ObjectURL  string      `json:"objectUrl"`
	ObjectName string      `json:"objectName"`
	ObjectType string      `json:"objectType,omitempty"`
	Matches    []GrepMatch `json:"matches"`
	MatchCount int         `json:"matchCount"`
	Message    string      `json:"message,omitempty"`
}

// GrepPackageResult represents the result of grepping an ABAP package.
type GrepPackageResult struct {
	Success     bool               `json:"success"`
	PackageName string             `json:"packageName"`
	Objects     []GrepObjectResult `json:"objects"`
	TotalMatches int               `json:"totalMatches"`
	Message     string             `json:"message,omitempty"`
}

// GrepObject searches for a regex pattern in a single ABAP object's source code.
//
// Parameters:
//   - objectURL: ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)
//   - pattern: Regular expression pattern (Go regexp syntax)
//   - caseInsensitive: If true, perform case-insensitive matching
//   - contextLines: Number of lines to include before/after each match (default: 0)
//
// Returns matches with line numbers and optional context lines.
func (c *Client) GrepObject(ctx context.Context, objectURL, pattern string, caseInsensitive bool, contextLines int) (*GrepObjectResult, error) {
	result := &GrepObjectResult{
		ObjectURL: objectURL,
		Matches:   []GrepMatch{},
	}

	// Extract object name from URL
	parts := strings.Split(objectURL, "/")
	if len(parts) > 0 {
		result.ObjectName = parts[len(parts)-1]
	}

	// Compile regex pattern
	regexPattern := pattern
	if caseInsensitive {
		regexPattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(regexPattern)
	if err != nil {
		result.Message = fmt.Sprintf("Invalid regex pattern: %v", err)
		return result, nil
	}

	// Get source code
	sourceURL := objectURL
	if !strings.HasSuffix(sourceURL, "/source/main") {
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
	lines := strings.Split(source, "\n")

	// Search for matches
	for i, line := range lines {
		if re.MatchString(line) {
			match := GrepMatch{
				LineNumber:  i + 1, // 1-based line numbers
				MatchedLine: line,
			}

			// Add context lines before
			if contextLines > 0 {
				start := i - contextLines
				if start < 0 {
					start = 0
				}
				match.ContextBefore = lines[start:i]
			}

			// Add context lines after
			if contextLines > 0 {
				end := i + contextLines + 1
				if end > len(lines) {
					end = len(lines)
				}
				match.ContextAfter = lines[i+1 : end]
			}

			result.Matches = append(result.Matches, match)
		}
	}

	result.MatchCount = len(result.Matches)
	result.Success = true

	if result.MatchCount == 0 {
		result.Message = "No matches found"
	} else {
		result.Message = fmt.Sprintf("Found %d match(es) in %s", result.MatchCount, result.ObjectName)
	}

	return result, nil
}

// GrepObjectsResult represents the result of grepping multiple ABAP objects.
type GrepObjectsResult struct {
	Success      bool               `json:"success"`
	Objects      []GrepObjectResult `json:"objects"`
	TotalMatches int                `json:"totalMatches"`
	Message      string             `json:"message,omitempty"`
}

// GrepObjects searches for a regex pattern in multiple ABAP objects' source code.
// This is a unified tool that handles both single and multiple object searches.
//
// Parameters:
//   - objectURLs: Array of ADT URLs (e.g., ["/sap/bc/adt/programs/programs/ZTEST"])
//   - pattern: Regular expression pattern (Go regexp syntax)
//   - caseInsensitive: If true, perform case-insensitive matching
//   - contextLines: Number of lines to include before/after each match (default: 0)
//
// Returns aggregated matches across all objects with per-object breakdown.
func (c *Client) GrepObjects(ctx context.Context, objectURLs []string, pattern string, caseInsensitive bool, contextLines int) (*GrepObjectsResult, error) {
	result := &GrepObjectsResult{
		Objects: []GrepObjectResult{},
	}

	if len(objectURLs) == 0 {
		result.Message = "No object URLs provided"
		return result, nil
	}

	// Search each object
	for _, objectURL := range objectURLs {
		objResult, err := c.GrepObject(ctx, objectURL, pattern, caseInsensitive, contextLines)
		if err != nil {
			// Log error but continue with other objects
			continue
		}

		// Only include objects with matches
		if objResult.MatchCount > 0 {
			result.Objects = append(result.Objects, *objResult)
			result.TotalMatches += objResult.MatchCount
		}
	}

	result.Success = true
	if result.TotalMatches == 0 {
		result.Message = fmt.Sprintf("No matches found in %d object(s)", len(objectURLs))
	} else {
		result.Message = fmt.Sprintf("Found %d match(es) across %d object(s)", result.TotalMatches, len(result.Objects))
	}

	return result, nil
}

// GrepPackage searches for a regex pattern across all objects in an ABAP package.
//
// Parameters:
//   - packageName: Name of the package (e.g., $TMP, ZPACKAGE)
//   - pattern: Regular expression pattern (Go regexp syntax)
//   - caseInsensitive: If true, perform case-insensitive matching
//   - objectTypes: Filter by object types (e.g., ["CLAS/OC", "PROG/P"]). Empty = search all.
//   - maxResults: Maximum number of matching objects to return (0 = unlimited)
//
// Returns matches grouped by object with match counts.
func (c *Client) GrepPackage(ctx context.Context, packageName, pattern string, caseInsensitive bool, objectTypes []string, maxResults int) (*GrepPackageResult, error) {
	result := &GrepPackageResult{
		PackageName: packageName,
		Objects:     []GrepObjectResult{},
	}

	// Get package contents
	packageContent, err := c.GetPackage(ctx, packageName)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to read package: %v", err)
		return result, nil
	}

	// Build object type filter map
	typeFilter := make(map[string]bool)
	if len(objectTypes) > 0 {
		for _, t := range objectTypes {
			typeFilter[t] = true
		}
	}

	// Search each object in package
	objectsSearched := 0
	for _, obj := range packageContent.Objects {
		// Apply object type filter
		if len(typeFilter) > 0 && !typeFilter[obj.Type] {
			continue
		}

		// Skip non-source objects (tables, structures, etc.)
		if !isSourceObject(obj.Type) {
			continue
		}

		// Grep this object
		objResult, err := c.GrepObject(ctx, obj.URI, pattern, caseInsensitive, 0)
		if err != nil {
			continue // Skip objects that fail
		}

		// Only include objects with matches
		if objResult.MatchCount > 0 {
			objResult.ObjectType = obj.Type
			result.Objects = append(result.Objects, *objResult)
			result.TotalMatches += objResult.MatchCount

			// Check max results limit
			objectsSearched++
			if maxResults > 0 && objectsSearched >= maxResults {
				break
			}
		}
	}

	result.Success = true
	if result.TotalMatches == 0 {
		result.Message = "No matches found in package"
	} else {
		result.Message = fmt.Sprintf("Found %d match(es) across %d object(s) in package %s",
			result.TotalMatches, len(result.Objects), packageName)
	}

	return result, nil
}

// GrepPackagesResult represents the result of grepping multiple ABAP packages.
type GrepPackagesResult struct {
	Success      bool               `json:"success"`
	Packages     []string           `json:"packages"` // List of searched packages
	Objects      []GrepObjectResult `json:"objects"`
	TotalMatches int                `json:"totalMatches"`
	Message      string             `json:"message,omitempty"`
}

// GrepPackages searches for a regex pattern across multiple ABAP packages.
// This is a unified tool that handles single, multiple, and recursive package searches.
//
// Parameters:
//   - packages: Array of package names (e.g., ["$TMP"], ["$TMP", "ZLOCAL"])
//   - includeSubpackages: If true, recursively search all subpackages
//   - pattern: Regular expression pattern (Go regexp syntax)
//   - caseInsensitive: If true, perform case-insensitive matching
//   - objectTypes: Filter by object types (e.g., ["CLAS/OC", "PROG/P"]). Empty = search all.
//   - maxResults: Maximum number of matching objects to return (0 = unlimited)
//
// Returns aggregated matches across all packages with per-object breakdown.
func (c *Client) GrepPackages(ctx context.Context, packages []string, includeSubpackages bool, pattern string, caseInsensitive bool, objectTypes []string, maxResults int) (*GrepPackagesResult, error) {
	result := &GrepPackagesResult{
		Packages: []string{},
		Objects:  []GrepObjectResult{},
	}

	if len(packages) == 0 {
		result.Message = "No packages provided"
		return result, nil
	}

	// Collect all packages to search (including subpackages if requested)
	packagesToSearch := []string{}
	for _, pkg := range packages {
		if includeSubpackages {
			// Get package tree (including subpackages)
			subPackages, err := c.collectSubpackages(ctx, pkg)
			if err != nil {
				// If error getting subpackages, just search the main package
				packagesToSearch = append(packagesToSearch, pkg)
			} else {
				packagesToSearch = append(packagesToSearch, subPackages...)
			}
		} else {
			packagesToSearch = append(packagesToSearch, pkg)
		}
	}

	result.Packages = packagesToSearch

	// Search each package
	totalObjectsSearched := 0
	for _, packageName := range packagesToSearch {
		pkgResult, err := c.GrepPackage(ctx, packageName, pattern, caseInsensitive, objectTypes, maxResults-totalObjectsSearched)
		if err != nil {
			// Log error but continue with other packages
			continue
		}

		// Append results
		result.Objects = append(result.Objects, pkgResult.Objects...)
		result.TotalMatches += pkgResult.TotalMatches
		totalObjectsSearched += len(pkgResult.Objects)

		// Check if we've reached max results
		if maxResults > 0 && totalObjectsSearched >= maxResults {
			break
		}
	}

	result.Success = true
	if result.TotalMatches == 0 {
		result.Message = fmt.Sprintf("No matches found in %d package(s)", len(result.Packages))
	} else {
		result.Message = fmt.Sprintf("Found %d match(es) across %d object(s) in %d package(s)",
			result.TotalMatches, len(result.Objects), len(result.Packages))
	}

	return result, nil
}

// collectSubpackages recursively collects a package and all its subpackages.
func (c *Client) collectSubpackages(ctx context.Context, packageName string) ([]string, error) {
	packages := []string{packageName}

	// Get package contents
	content, err := c.GetPackage(ctx, packageName)
	if err != nil {
		return packages, err
	}

	// Check if package content has subpackages
	// PackageContent has a SubPackages field ([]string) if it exists
	if content.SubPackages != nil && len(content.SubPackages) > 0 {
		for _, subpkgName := range content.SubPackages {
			// Recursively collect subpackages
			subPackages, err := c.collectSubpackages(ctx, subpkgName)
			if err == nil {
				packages = append(packages, subPackages...)
			}
		}
	}

	return packages, nil
}

// isSourceObject returns true if the object type contains source code that can be searched.
func isSourceObject(objectType string) bool {
	sourceTypes := map[string]bool{
		"PROG/P":  true, // Programs
		"CLAS/OC": true, // Classes
		"INTF/OI": true, // Interfaces
		"FUGR/F":  true, // Function groups
		"FUGR/FF": true, // Function modules
		"PROG/I":  true, // Includes
	}
	return sourceTypes[objectType]
}

// --- ENHO-aware grep ---

// defaultEnhancementGrepCap bounds how many ENHO bodies a single walk fetches
// before subsequent refs are elided with a marker. ENHO body fetches go through
// the WebSocket bridge, which is roughly a 30s round-trip per body in the
// worst case, so a hard cap matters for package-scope greps.
const defaultEnhancementGrepCap = 50

// GrepEnhancementsState carries per-walk state so that a package or
// multi-object grep fetches each ENHO body at most once and stops fetching
// after a configurable cap. The zero value is a no-op (no dedup, no cap).
// Use NewGrepEnhancementsState to get sensible defaults.
type GrepEnhancementsState struct {
	Cap     int             // max ENHO bodies to fetch; 0 ⇒ defaultEnhancementGrepCap
	seen    map[string]bool // ENHO names already fetched in this walk
	fetched int
	elided  int
}

// NewGrepEnhancementsState returns a fresh state with the default cap (50).
// Pass cap=0 for the default; pass a positive value to override.
func NewGrepEnhancementsState(cap int) *GrepEnhancementsState {
	if cap <= 0 {
		cap = defaultEnhancementGrepCap
	}
	return &GrepEnhancementsState{Cap: cap, seen: map[string]bool{}}
}

// extractObjectNameFromURL pulls the object name from an ADT URL such as
// `/sap/bc/adt/programs/includes/RVKMP901` or `.../programs/programs/ZTEST`.
// Returns "" for URLs that don't fit the expected shape.
func extractObjectNameFromURL(objectURL string) string {
	u := strings.TrimRight(objectURL, "/")
	u = strings.TrimSuffix(u, "/source/main")
	idx := strings.LastIndex(u, "/")
	if idx < 0 || idx == len(u)-1 {
		return ""
	}
	return strings.ToUpper(u[idx+1:])
}

// supportsEnhancementWalk reports whether the URL points at an object whose
// ENHO attachments are expressible via D010INC ⨝ ENHINCINX (HOOK_IMPL plug-ins
// on programs and includes). Classes, interfaces, DDIC, etc. are skipped —
// they have their own enhancement mechanisms not covered by this walker.
func supportsEnhancementWalk(objectURL string) bool {
	return strings.Contains(objectURL, "/programs/includes/") ||
		strings.Contains(objectURL, "/programs/programs/")
}

// grepEnhancementsForObject walks the ENHO refs attached to an
// include/program at objectURL and greps each body. Returns extra matches
// tagged with the ENHO ref. Soft-fails throughout — fetch failures emit a
// single warning hit per ref instead of erroring the whole walk.
//
// Pre-compiled `re` is reused from the caller so we don't recompile per ENHO.
func (c *Client) grepEnhancementsForObject(
	ctx context.Context,
	objectURL string,
	re *regexp.Regexp,
	contextLines int,
	state *GrepEnhancementsState,
) []GrepMatch {
	if !supportsEnhancementWalk(objectURL) {
		return nil
	}
	hostName := extractObjectNameFromURL(objectURL)
	if hostName == "" {
		return nil
	}

	refs, err := c.ListEnhancementsForInclude(ctx, hostName)
	if err != nil || len(refs) == 0 {
		return nil
	}

	var hits []GrepMatch
	for i := range refs {
		ref := refs[i]
		if state != nil {
			if state.seen[ref.Name] {
				continue
			}
			state.seen[ref.Name] = true
			if state.Cap > 0 && state.fetched >= state.Cap {
				state.elided++
				continue
			}
			state.fetched++
		}

		body, ferr := c.GetEnhancementByRef(ctx, &ref)
		if ferr != nil {
			hits = append(hits, GrepMatch{
				LineNumber: 0,
				MatchedLine: fmt.Sprintf("[ENHO/%s %s @ %s — body unavailable: %v]",
					ref.Kind, ref.Name, hostName, ferr),
			})
			continue
		}

		bodyLines := strings.Split(body, "\n")
		for li, line := range bodyLines {
			if !re.MatchString(line) {
				continue
			}
			match := GrepMatch{
				LineNumber:  li + 1,
				MatchedLine: fmt.Sprintf("[ENHO %s @ %s] %s", ref.Name, hostName, line),
			}
			if contextLines > 0 {
				s := li - contextLines
				if s < 0 {
					s = 0
				}
				e := li + contextLines + 1
				if e > len(bodyLines) {
					e = len(bodyLines)
				}
				match.ContextBefore = bodyLines[s:li]
				match.ContextAfter = bodyLines[li+1 : e]
			}
			hits = append(hits, match)
		}
	}

	return hits
}

// finalizeEnhancementWalk appends the elision marker once at the end of a
// package walk. Caller is the GrepPackage* path; per-object grep doesn't
// surface elision because the cap only matters at walk scope.
func finalizeEnhancementWalk(state *GrepEnhancementsState) []GrepMatch {
	if state == nil || state.elided == 0 {
		return nil
	}
	return []GrepMatch{{
		LineNumber: 0,
		MatchedLine: fmt.Sprintf(
			"[ENHO walk: %d more enhancement(s) elided after cap of %d — increase max_enhancements to fetch]",
			state.elided, state.Cap),
	}}
}

// GrepObjectWithEnhancements wraps GrepObject with an ENHO body walk. When
// enabled and the URL points at a program/include, every ENHO attached to
// the object is fetched via GetEnhancementByRef and greppd with the same
// pattern. Hits are tagged `[ENHO <name> @ <host>] <line>` so they're easy
// to distinguish from base-source hits.
//
// `walk` is optional — pass nil for a self-contained call, or a shared
// state from NewGrepEnhancementsState to dedupe fetches across multiple
// GrepObjectWithEnhancements calls (e.g. a package walk that hits the same
// ENHO via several function-group includes).
func (c *Client) GrepObjectWithEnhancements(
	ctx context.Context,
	objectURL, pattern string,
	caseInsensitive bool,
	contextLines int,
	walk *GrepEnhancementsState,
) (*GrepObjectResult, error) {
	base, err := c.GrepObject(ctx, objectURL, pattern, caseInsensitive, contextLines)
	if err != nil {
		return base, err
	}

	regexPattern := pattern
	if caseInsensitive {
		regexPattern = "(?i)" + pattern
	}
	re, rErr := regexp.Compile(regexPattern)
	if rErr != nil {
		// Pattern was already validated by GrepObject; if it failed there
		// the base result already carries the message. Don't shadow it.
		return base, nil
	}

	enhHits := c.grepEnhancementsForObject(ctx, objectURL, re, contextLines, walk)
	if len(enhHits) == 0 {
		return base, nil
	}
	base.Matches = append(base.Matches, enhHits...)
	base.MatchCount = len(base.Matches)
	base.Success = true
	if base.Message == "" || strings.HasPrefix(base.Message, "No matches") {
		base.Message = fmt.Sprintf("Found %d match(es) in %s (incl. enhancements)",
			base.MatchCount, base.ObjectName)
	}
	return base, nil
}

// GrepObjectsWithEnhancements is GrepObjects with ENHO body walking. A single
// shared walk-state spans all objectURLs so the same ENHO is fetched at most
// once across the multi-object grep.
func (c *Client) GrepObjectsWithEnhancements(
	ctx context.Context,
	objectURLs []string,
	pattern string,
	caseInsensitive bool,
	contextLines int,
	maxEnhancements int,
) (*GrepObjectsResult, error) {
	result := &GrepObjectsResult{Objects: []GrepObjectResult{}}
	if len(objectURLs) == 0 {
		result.Message = "No object URLs provided"
		return result, nil
	}

	walk := NewGrepEnhancementsState(maxEnhancements)
	for _, objectURL := range objectURLs {
		objResult, oerr := c.GrepObjectWithEnhancements(ctx, objectURL, pattern, caseInsensitive, contextLines, walk)
		if oerr != nil {
			continue
		}
		if objResult.MatchCount > 0 {
			result.Objects = append(result.Objects, *objResult)
			result.TotalMatches += objResult.MatchCount
		}
	}

	if marker := finalizeEnhancementWalk(walk); len(marker) > 0 {
		result.Objects = append(result.Objects, GrepObjectResult{
			Success:    true,
			ObjectName: "[enhancement walk]",
			Matches:    marker,
			MatchCount: len(marker),
			Message: fmt.Sprintf("ENHO body cap reached (%d), %d more elided",
				walk.Cap, walk.elided),
		})
		result.TotalMatches += len(marker)
	}

	result.Success = true
	if result.TotalMatches == 0 {
		result.Message = fmt.Sprintf("No matches found in %d object(s)", len(objectURLs))
	} else {
		result.Message = fmt.Sprintf("Found %d match(es) across %d object(s) (incl. enhancements)",
			result.TotalMatches, len(result.Objects))
	}
	return result, nil
}

// GrepPackagesWithEnhancements is GrepPackages with ENHO body walking. Like
// GrepObjectsWithEnhancements, walk state is shared across all packages so
// an ENHO touching includes in multiple packages is fetched once.
func (c *Client) GrepPackagesWithEnhancements(
	ctx context.Context,
	packages []string,
	includeSubpackages bool,
	pattern string,
	caseInsensitive bool,
	objectTypes []string,
	maxResults int,
	maxEnhancements int,
) (*GrepPackagesResult, error) {
	result := &GrepPackagesResult{
		Packages: []string{},
		Objects:  []GrepObjectResult{},
	}
	if len(packages) == 0 {
		result.Message = "No packages provided"
		return result, nil
	}

	packagesToSearch := []string{}
	for _, pkg := range packages {
		if includeSubpackages {
			subPackages, err := c.collectSubpackages(ctx, pkg)
			if err != nil {
				packagesToSearch = append(packagesToSearch, pkg)
			} else {
				packagesToSearch = append(packagesToSearch, subPackages...)
			}
		} else {
			packagesToSearch = append(packagesToSearch, pkg)
		}
	}
	result.Packages = packagesToSearch

	walk := NewGrepEnhancementsState(maxEnhancements)
	typeFilter := make(map[string]bool)
	for _, t := range objectTypes {
		typeFilter[t] = true
	}

	totalObjectsSearched := 0
	for _, packageName := range packagesToSearch {
		packageContent, err := c.GetPackage(ctx, packageName)
		if err != nil {
			continue
		}
		for _, obj := range packageContent.Objects {
			if len(typeFilter) > 0 && !typeFilter[obj.Type] {
				continue
			}
			if !isSourceObject(obj.Type) {
				continue
			}
			objResult, oerr := c.GrepObjectWithEnhancements(ctx, obj.URI, pattern, caseInsensitive, 0, walk)
			if oerr != nil {
				continue
			}
			if objResult.MatchCount == 0 {
				continue
			}
			objResult.ObjectType = obj.Type
			result.Objects = append(result.Objects, *objResult)
			result.TotalMatches += objResult.MatchCount
			totalObjectsSearched++
			if maxResults > 0 && totalObjectsSearched >= maxResults {
				break
			}
		}
		if maxResults > 0 && totalObjectsSearched >= maxResults {
			break
		}
	}

	if marker := finalizeEnhancementWalk(walk); len(marker) > 0 {
		result.Objects = append(result.Objects, GrepObjectResult{
			Success:    true,
			ObjectName: "[enhancement walk]",
			Matches:    marker,
			MatchCount: len(marker),
			Message: fmt.Sprintf("ENHO body cap reached (%d), %d more elided",
				walk.Cap, walk.elided),
		})
		result.TotalMatches += len(marker)
	}

	result.Success = true
	if result.TotalMatches == 0 {
		result.Message = fmt.Sprintf("No matches found in %d package(s)", len(result.Packages))
	} else {
		result.Message = fmt.Sprintf("Found %d match(es) across %d object(s) in %d package(s) (incl. enhancements)",
			result.TotalMatches, len(result.Objects), len(result.Packages))
	}
	return result, nil
}

// GrepPackageWithEnhancements is GrepPackage with ENHO bodies walked in
// addition to base sources. Dedup state is shared across all objects in
// the package so a single ENHO touching multiple includes is fetched once.
// The walk's cap (default 50) bounds total ENHO body fetches; an elision
// marker is appended to the package result when the cap is hit.
func (c *Client) GrepPackageWithEnhancements(
	ctx context.Context,
	packageName, pattern string,
	caseInsensitive bool,
	objectTypes []string,
	maxResults int,
	maxEnhancements int,
) (*GrepPackageResult, error) {
	result := &GrepPackageResult{
		PackageName: packageName,
		Objects:     []GrepObjectResult{},
	}

	packageContent, err := c.GetPackage(ctx, packageName)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to read package: %v", err)
		return result, nil
	}

	typeFilter := make(map[string]bool)
	if len(objectTypes) > 0 {
		for _, t := range objectTypes {
			typeFilter[t] = true
		}
	}

	walk := NewGrepEnhancementsState(maxEnhancements)
	objectsSearched := 0
	for _, obj := range packageContent.Objects {
		if len(typeFilter) > 0 && !typeFilter[obj.Type] {
			continue
		}
		if !isSourceObject(obj.Type) {
			continue
		}

		objResult, oerr := c.GrepObjectWithEnhancements(ctx, obj.URI, pattern, caseInsensitive, 0, walk)
		if oerr != nil {
			continue
		}
		if objResult.MatchCount == 0 {
			continue
		}
		objResult.ObjectType = obj.Type
		result.Objects = append(result.Objects, *objResult)
		result.TotalMatches += objResult.MatchCount

		objectsSearched++
		if maxResults > 0 && objectsSearched >= maxResults {
			break
		}
	}

	// Surface elision at package scope, attached to a synthetic "object" so
	// it's visible in the JSON output without polluting per-object results.
	if marker := finalizeEnhancementWalk(walk); len(marker) > 0 {
		result.Objects = append(result.Objects, GrepObjectResult{
			Success:    true,
			ObjectName: "[enhancement walk]",
			Matches:    marker,
			MatchCount: len(marker),
			Message: fmt.Sprintf("ENHO body cap reached (%d), %d more elided",
				walk.Cap, walk.elided),
		})
		result.TotalMatches += len(marker)
	}

	result.Success = true
	if result.TotalMatches == 0 {
		result.Message = "No matches found in package"
	} else {
		result.Message = fmt.Sprintf("Found %d match(es) across %d object(s) in package %s (incl. enhancements)",
			result.TotalMatches, len(result.Objects), packageName)
	}
	return result, nil
}
