package adt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Search Operations ---

// SetCookies replaces the session this client authenticates with.
func (c *Client) SetCookies(cookies map[string]string) {
	c.transport.SetCookies(cookies)
}

// CurrentCookies returns the session this client is authenticating with now,
// which is not necessarily the one it was given. See Transport.CurrentCookies.
func (c *Client) CurrentCookies() map[string]string {
	return c.transport.CurrentCookies()
}

// SearchObject searches for ABAP objects by name pattern.
// The query parameter supports wildcards (* for multiple chars, ? for single char).
func (c *Client) SearchObject(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	return c.SearchObjectByType(ctx, query, "", maxResults)
}

// CanonicalObjectType maps the documented short forms (CLAS, INTF, PROG, ...)
// to the ADT-canonical group codes the SAP server expects on the
// informationsystem/search endpoint. Unknown values pass through verbatim,
// covering already-canonical input ("CLAS/OC"), namespaced types, or custom codes.
// Exported so every caller (CLI, MCP, direct API) gets the same expansion;
// SearchObjectByType applies it internally.
func CanonicalObjectType(s string) string {
	switch strings.ToUpper(s) {
	case "":
		return ""
	case "CLAS":
		return "CLAS/OC"
	case "INTF":
		return "INTF/OI"
	case "PROG":
		return "PROG/P"
	case "INCL":
		return "PROG/I"
	case "FUGR":
		return "FUGR/F"
	case "FUNC":
		return "FUGR/FF"
	case "TABL":
		return "TABL/DT"
	case "DTEL":
		return "DTEL/DE"
	case "DOMA":
		return "DOMA/DD"
	case "TTYP":
		return "TTYP/DA"
	case "ENQU":
		return "ENQU/DL"
	case "DDLS":
		return "DDLS/DF"
	case "MSAG":
		return "MSAG/N"
	case "TRAN":
		return "TRAN/T"
	}
	return s
}

// SearchObjectByType searches for ABAP objects by name pattern, optionally
// constrained to a specific ADT object type code (e.g. "CLAS/OC", "PROG/P",
// "INTF/OI"). An empty objectType means "any type" and behaves identically
// to SearchObject. Server-side type filtering is required when combined with
// maxResults: filtering after the fact silently drops results that didn't
// fit in the pre-filter window.
func (c *Client) SearchObjectByType(ctx context.Context, query, objectType string, maxResults int) ([]SearchResult, error) {
	return c.searchObjectByType(ctx, query, objectType, maxResults, false)
}

// searchObjectByType is SearchObjectByType with a choice of session: stateful
// joins the context a held lock lives in.
func (c *Client) searchObjectByType(ctx context.Context, query, objectType string, maxResults int, stateful bool) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 100
	}

	// Expand documented short forms (CLAS, FUNC, INCL, ...) to ADT-canonical
	// group codes so callers can pass either form. No-op for already-canonical
	// or unknown input.
	objectType = CanonicalObjectType(objectType)

	params := url.Values{}
	params.Set("operation", "quickSearch")
	params.Set("query", query)
	params.Set("maxResults", fmt.Sprintf("%d", maxResults))
	if objectType != "" {
		params.Set("objectType", objectType)
	}

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/repository/informationsystem/search", &RequestOptions{
		Method:   http.MethodGet,
		Query:    params,
		Accept:   "application/xml",
		Stateful: stateful,
	})
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}

	return ParseSearchResults(resp.Body)
}

// exactSearchFetch is how many hits an exact-name search asks the quick
// search for before keeping the equal names.
//
// The name is sent as it is, without a wildcard. On the systems checked
// (A4H, 7.5x) the quick search then matches the whole name only, ignoring
// case, across types: CL_ABAP finds nothing where CL_ABAP* finds 599, and
// BUKRS finds its domain, data element and authorization object. The window
// is for a release that reads a bare name as a prefix instead: the equal
// names are filtered out of it, and a window that comes back full is
// reported, since an equal name can lie beyond it.
const exactSearchFetch = 1000

// ErrExactSearchWindowFull is returned by SearchObjectExact when the quick
// search filled the whole window and the name itself was not among the hits:
// whether it exists is unknown, not "no".
var ErrExactSearchWindowFull = errors.New("exact search inconclusive")

// SearchObjectExact returns the objects whose name equals name, ignoring
// case, optionally of one type, at most maxResults of them (0: no limit).
// A name can belong to several objects of different types (a program and a
// class, a domain and a data element), so the answer is a list.
//
// incomplete is set when the quick search filled its window and fewer than
// maxResults equal names were found in it: other objects of that name may lie
// beyond the window. With no equal name in a full window the answer is
// ErrExactSearchWindowFull instead.
func (c *Client) SearchObjectExact(ctx context.Context, name, objectType string, maxResults int) (results []SearchResult, incomplete string, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", fmt.Errorf("exact search needs a name")
	}
	if strings.ContainsAny(name, "*?") {
		return nil, "", fmt.Errorf("exact search takes a name, not a pattern: %q (drop exact to search by pattern)", name)
	}
	hits, err := c.SearchObjectByType(ctx, name, objectType, exactSearchFetch)
	if err != nil {
		return nil, "", err
	}
	out := FilterExactName(hits, name)
	if maxResults > 0 && len(out) > maxResults {
		out = out[:maxResults]
	}
	windowFull := len(hits) >= exactSearchFetch
	if !windowFull || (maxResults > 0 && len(out) >= maxResults) {
		return out, "", nil
	}
	narrow := "pass type to narrow"
	if objectType != "" {
		narrow = "even with type " + objectType + "; the name cannot be found this way"
	}
	if len(out) == 0 {
		// A full window without the name says nothing about whether it
		// exists: it may be ranked beyond the window.
		return nil, "", fmt.Errorf("%w: %q not found within the first %d matches; %s",
			ErrExactSearchWindowFull, name, exactSearchFetch, narrow)
	}
	return out, fmt.Sprintf("the result may be incomplete: the quick search returned its full window of %d matches, and more objects named %q may lie beyond it; %s",
		exactSearchFetch, name, narrow), nil
}

// FilterExactName keeps the results whose name equals name, ignoring case.
func FilterExactName(results []SearchResult, name string) []SearchResult {
	name = strings.TrimSpace(name)
	out := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if strings.EqualFold(strings.TrimSpace(r.Name), name) {
			out = append(out, r)
		}
	}
	return out
}

// ResolveObjectRef converts a "TYPE NAME" shorthand (e.g. "INCL ZREP_F01", "PROG ZREPORT")
// into the (objectURL, objectName) pair needed for activation or other ADT operations.
// The name is returned in UPPERCASE; the URL uses lowercase path encoding.
func (c *Client) ResolveObjectRef(typeAndName string) (objectURL, objectName string, err error) {
	parts := strings.Fields(strings.ToUpper(strings.TrimSpace(typeAndName)))
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected \"TYPE NAME\", got %q", typeAndName)
	}
	objType, name := parts[0], parts[1]
	encoded := url.PathEscape(strings.ToLower(name))
	switch objType {
	case "PROG":
		return "/sap/bc/adt/programs/programs/" + encoded, name, nil
	case "INCL":
		return "/sap/bc/adt/programs/includes/" + encoded, name, nil
	case "CLAS":
		return "/sap/bc/adt/oo/classes/" + encoded, name, nil
	case "INTF":
		return "/sap/bc/adt/oo/interfaces/" + encoded, name, nil
	case "FUGR":
		return "/sap/bc/adt/function/groups/" + encoded, name, nil
	case "DDLS":
		return "/sap/bc/adt/ddic/ddl/sources/" + encoded, name, nil
	default:
		return "", "", fmt.Errorf("unsupported object type %q (supported: PROG, INCL, CLAS, INTF, FUGR, DDLS)", objType)
	}
}
