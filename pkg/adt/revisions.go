package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Object Version History (Revisions) ---

// GetRevisions retrieves the version history (revisions) of an ABAP object.
// For classes, use opts.Include to get revisions of a specific include.
//
// Supported object types: PROG, CLAS, INTF, FUNC, INCL, DDLS, BDEF, SRVD
func (c *Client) GetRevisions(ctx context.Context, objectType, name string, opts *GetSourceOptions) ([]Revision, error) {
	if err := c.checkSafety(OpRead, "GetRevisions"); err != nil {
		return nil, err
	}

	objectType = strings.ToUpper(objectType)
	name = strings.ToUpper(name)
	if opts == nil {
		opts = &GetSourceOptions{}
	}

	revisionURL, err := c.resolveRevisionURL(objectType, name, opts)
	if err != nil {
		return nil, fmt.Errorf("resolving revision URL for %s %s: %w", objectType, name, err)
	}

	resp, err := c.transport.Request(ctx, revisionURL, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/atom+xml;type=feed",
	})
	if err != nil {
		return nil, fmt.Errorf("getting revisions for %s %s: %w", objectType, name, err)
	}

	return ParseRevisionFeed(resp.Body)
}

// GetRevisionSource retrieves the source code of a specific object version.
// The versionURI comes from GetRevisions output (the URI field of a Revision entry).
func (c *Client) GetRevisionSource(ctx context.Context, versionURI string) (string, error) {
	if err := c.checkSafety(OpRead, "GetRevisionSource"); err != nil {
		return "", err
	}

	if versionURI == "" {
		return "", fmt.Errorf("versionURI is required")
	}

	resp, err := c.transport.Request(ctx, versionURI, &RequestOptions{
		Method: http.MethodGet,
		Accept: "text/plain",
	})
	if err != nil {
		return "", fmt.Errorf("getting revision source: %w", err)
	}

	return string(resp.Body), nil
}

// CompareVersions compares two versions of an ABAP object and returns a unified diff.
// version1URI and version2URI are from GetRevisions output.
// Use "current" as version2URI to compare against the active version.
func (c *Client) CompareVersions(ctx context.Context, objectType, name string, version1URI, version2URI string, opts *GetSourceOptions) (*SourceDiff, error) {
	if err := c.checkSafety(OpRead, "CompareVersions"); err != nil {
		return nil, err
	}

	source1, err := c.GetRevisionSource(ctx, version1URI)
	if err != nil {
		return nil, fmt.Errorf("getting version 1 source: %w", err)
	}

	var source2 string
	if version2URI == "current" {
		source2, err = c.GetSource(ctx, objectType, name, opts)
		if err != nil {
			return nil, fmt.Errorf("getting current source: %w", err)
		}
	} else {
		source2, err = c.GetRevisionSource(ctx, version2URI)
		if err != nil {
			return nil, fmt.Errorf("getting version 2 source: %w", err)
		}
	}

	label1 := fmt.Sprintf("%s:%s@%s", objectType, name, extractVersionLabel(version1URI))
	label2 := fmt.Sprintf("%s:%s@%s", objectType, name, extractVersionLabel(version2URI))

	result := &SourceDiff{
		Object1:   label1,
		Object2:   label2,
		Identical: source1 == source2,
	}

	if result.Identical {
		result.Diff = "Sources are identical"
		return result, nil
	}

	lines1 := strings.Split(source1, "\n")
	lines2 := strings.Split(source2, "\n")
	result.Diff = generateUnifiedDiff(label1, label2, lines1, lines2)

	for _, line := range strings.Split(result.Diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			result.AddedLines++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			result.RemovedLines++
		}
	}

	return result, nil
}

// resolveRevisionURL builds the ADT revision feed URL for a given object type.
//
// Key discovery: classes use /includes/{type}/versions (not /source/main/versions).
// Programs and other source objects use /source/main/versions.
// Interfaces also use /includes/main/versions (same pattern as classes).
func (c *Client) resolveRevisionURL(objectType, name string, opts *GetSourceOptions) (string, error) {
	encodedName := url.PathEscape(name)

	switch objectType {
	case "PROG":
		return fmt.Sprintf("/sap/bc/adt/programs/programs/%s/source/main/versions", encodedName), nil
	case "CLAS":
		include := opts.Include
		if include == "" {
			include = "main"
		}
		// Classes always use /includes/{type}/versions — even for main
		return fmt.Sprintf("/sap/bc/adt/oo/classes/%s/includes/%s/versions", encodedName, include), nil
	case "INTF":
		// Interfaces use the same /includes/main/versions pattern as classes
		return fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s/includes/main/versions", encodedName), nil
	case "FUNC":
		if opts.Parent == "" {
			return "", fmt.Errorf("parent (function group name) is required for FUNC type")
		}
		encodedParent := url.PathEscape(strings.ToUpper(opts.Parent))
		return fmt.Sprintf("/sap/bc/adt/functions/groups/%s/fmodules/%s/source/main/versions", encodedParent, encodedName), nil
	case "INCL":
		return fmt.Sprintf("/sap/bc/adt/programs/includes/%s/source/main/versions", encodedName), nil
	case "DDLS":
		return fmt.Sprintf("/sap/bc/adt/ddic/ddl/sources/%s/source/main/versions", encodedName), nil
	case "BDEF":
		return fmt.Sprintf("/sap/bc/adt/bo/behaviordefinitions/%s/source/main/versions", encodedName), nil
	case "SRVD":
		return fmt.Sprintf("/sap/bc/adt/ddic/srvd/sources/%s/source/main/versions", encodedName), nil
	default:
		return "", fmt.Errorf("unsupported object type for revisions: %s (supported: PROG, CLAS, INTF, FUNC, INCL, DDLS, BDEF, SRVD)", objectType)
	}
}

// extractVersionLabel extracts a short label from a version URI for display.
// Handles two URI formats:
//   - PROG: .../source/main?version=5 → "v5"
//   - CLAS: .../versions/20161212091747/00000/content → "v00000"
func extractVersionLabel(uri string) string {
	if uri == "current" {
		return "current"
	}
	// Format 1: query param (programs)
	if idx := strings.Index(uri, "version="); idx >= 0 {
		rest := uri[idx+8:]
		if end := strings.IndexAny(rest, "&;"); end >= 0 {
			return "v" + rest[:end]
		}
		return "v" + rest
	}
	// Format 2: path-based (classes) — .../versions/{timestamp}/{version}/content
	if idx := strings.Index(uri, "/versions/"); idx >= 0 {
		rest := uri[idx+10:] // after "/versions/"
		parts := strings.Split(rest, "/")
		if len(parts) >= 2 {
			return "v" + parts[1] // version number (e.g., "00000")
		}
		if len(parts) == 1 {
			return "v" + parts[0]
		}
	}
	parts := strings.Split(uri, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return uri
}
