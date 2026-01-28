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
	Success      bool               `json:"success"`
	PackageName  string             `json:"packageName"`
	Objects      []GrepObjectResult `json:"objects"`
	TotalMatches int                `json:"totalMatches"`
	Message      string             `json:"message,omitempty"`
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

	// Get package contents (unlimited - need all objects for grep)
	packageContent, err := c.GetPackage(ctx, packageName, &PackageQueryOptions{MaxObjects: -1})
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

	// Get package contents (only need subpackages, limit objects)
	content, err := c.GetPackage(ctx, packageName, &PackageQueryOptions{MaxObjects: 1})
	if err != nil {
		return packages, err
	}

	// Check if package content has subpackages
	if len(content.SubPackages) > 0 {
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
