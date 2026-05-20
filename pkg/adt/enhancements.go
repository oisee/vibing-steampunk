// Package adt — enhancement-framework (ENHO / ENHS) support.
//
// SAP's modern enhancement framework stores implementations as independent
// ADT objects of type ENHO with a subtype: XH (source-code plug-in), XC
// (class), XFB (function module), XD (interface), XBD (BAdI). SE80 merges
// these into the host include's displayed source at render time; the raw
// source endpoint never returns them. This file adds read-side support so
// the MCP surface can see what a developer sees in the GUI.
package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// rfcSourceFetcher is the boundary used by GetEnhancement's RFC fallback path.
// Production wiring opens a fresh DebugWebSocketClient per call (see
// defaultRFCSourceFetcher); tests substitute a stub so the unit suite never
// touches a real WebSocket.
//
// CallRFC is retained for callers that legitimately need a generic FM
// dispatch; ReadSource is the preferred path for source bodies because it
// uses native `READ REPORT` server-side and works for SUBC=I includes
// (enhancement plug-in source) where RPY_PROGRAM_READ raises CANCELLED.
type rfcSourceFetcher interface {
	CallRFC(ctx context.Context, function string, params map[string]string) (*RFCResult, error)
	ReadSource(ctx context.Context, program string) ([]string, error)
	Close() error
}

// EnhancementKind is the ENHO subtype code returned by ADT search (e.g. XH).
type EnhancementKind string

// Subtype path segments used by the ADT enhancement endpoints. The search
// API returns URIs such as `/sap/bc/adt/enhancements/enhoxh/<name>` — singular
// in the URI but plural in some source/main variants. Callers that already
// have a URI from search should use it verbatim; this map is only for
// fallback when only the name+kind are known.
var enhoSubtypePath = map[EnhancementKind]string{
	"XH":  "enhoxhs",
	"XC":  "enhoxcs",
	"XFB": "enhoxfbs",
	"XD":  "enhoxds",
	"XBD": "enhoxbds",
}

// EnhancementRef is a lightweight view over a SearchResult hit that is
// known to be an ENHO entry. Carried separately so callers don't have to
// re-parse the ADT type string.
type EnhancementRef struct {
	Name        string          `json:"name"`
	Kind        EnhancementKind `json:"kind"` // XH / XC / XFB / XD / XBD
	URI         string          `json:"uri"`
	PackageName string          `json:"packageName,omitempty"`
	Description string          `json:"description,omitempty"`
	// FullName is the XPath-style anchor location reported by ENHINCINX for
	// HOOK_IMPL plug-ins, e.g. "\PR:SAPLVKMP\FO:BEDINGUNG_PRUEFEN_901\SE:BEGIN\EI".
	// Empty unless the ENHO was discovered via the table-based fallback.
	FullName string `json:"fullName,omitempty"`
	// HostProgram is the main program (function-group main, executable program)
	// the enhancement attaches to — ENHINCINX.PROGRAMNAME. Empty for ENHOs
	// discovered only via the REST surface.
	HostProgram string `json:"hostProgram,omitempty"`
	// EnhInclude is the REPOSRC entry holding the plug-in source; conventionally
	// ENHNAME with an "E" suffix. Useful for SE80 navigation.
	EnhInclude string `json:"enhInclude,omitempty"`
}

// GetEnhancement returns the ABAP source of an enhancement implementation
// (ENHO) resolved by name. The ADT subtype (XH/XC/...) is discovered via
// SearchObject so callers don't have to pass it.
//
// Errors:
//   - not-found: no ENHO/* hit matches the name.
//   - ambiguous: more than one ENHO/* hit with the same name (rare; a name
//     plus a subtype is the unique key, so collisions across subtypes are
//     technically legal).
//
// When you already have an EnhancementRef (e.g. from ListEnhancementsForInclude)
// prefer GetEnhancementByRef — it preserves table-discovered fields like
// EnhInclude that the SearchObject-based resolver drops, which is what lets
// the RFC fallback find HOOK_IMPL plug-in sources whose REPOSRC names use
// `=`-padding rather than the simple <NAME>E convention.
func (c *Client) GetEnhancement(ctx context.Context, name string) (string, error) {
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		return "", fmt.Errorf("enhancement name is required")
	}

	ref, err := c.resolveEnhancement(ctx, name)
	if err != nil {
		return "", err
	}

	return c.GetEnhancementByRef(ctx, ref)
}

// GetEnhancementByRef returns the ABAP source for an already-resolved ENHO
// reference. Walks the same 3-step resolver as GetEnhancement (REST singular
// → REST plural → RFC) but skips the SearchObject re-resolution that
// GetEnhancement performs.
//
// The point of bypassing re-resolution: callers that obtained the ref via
// ListEnhancementsForInclude's table fallback already have ref.EnhInclude
// populated with the real REPOSRC entry name (e.g.
// "ISM_SAPLVKMP==================E" with `=`-padding). Going back through
// SearchObject would discard that and force the RFC step to fall back to
// the <NAME>E convention, which doesn't exist as an entry on classic ECC for
// most non-LEGO HOOK_IMPL plug-ins.
//
// Returns the same step-4 metadata-only error as GetEnhancement when no path
// resolves the body.
func (c *Client) GetEnhancementByRef(ctx context.Context, ref *EnhancementRef) (string, error) {
	if ref == nil || strings.TrimSpace(ref.Name) == "" {
		return "", fmt.Errorf("enhancement ref is required")
	}

	// 1) Modern REST: try the URL the search/browser returned ("singular" form).
	if uri := strings.TrimSpace(ref.URI); uri != "" {
		if src, ok := c.tryFetchEnhancementSource(ctx, strings.TrimRight(uri, "/")+"/source/main"); ok {
			return src, nil
		}
	}

	// 2) Newer plural form some NetWeaver releases use.
	if plural, ok := enhoSubtypePath[ref.Kind]; ok {
		alt := fmt.Sprintf("/sap/bc/adt/enhancements/%s/%s/source/main",
			plural, strings.ToLower(ref.Name))
		if src, ok := c.tryFetchEnhancementSource(ctx, alt); ok {
			return src, nil
		}
	}

	// 3) RFC fallback via ZADT_VSP WebSocket. Classic ECC walls off ENHO
	// bodies behind RPY_PROGRAM_READ — the only reliable way to read the
	// plug-in source on those releases. ref.EnhInclude (from ENHINCINX) is
	// the authoritative REPOSRC entry name when populated; otherwise the
	// `<name>E` convention is used as a best-effort guess.
	if src, ok := c.tryFetchEnhancementSourceViaRFC(ctx, ref); ok {
		return src, nil
	}

	// 4) No reachable source-body endpoint on this server. Surface a structured
	// error with metadata so the caller (and ultimately the user) can navigate
	// to the object in SE80 even though we can't return the body inline.
	hint := ref.EnhInclude
	if hint == "" {
		hint = ref.Name + "E"
	}
	return "", fmt.Errorf(
		"enhancement %s (%s, package %s): source body unavailable on this server "+
			"— ADT REST does not expose HOOK_IMPL plug-ins on this NetWeaver release "+
			"(SE80: see include %s). Install ZADT_VSP or grant the vsp cookie write "+
			"scope to enable inline retrieval.",
		ref.Name, ref.Kind, ref.PackageName, hint)
}

// tryFetchEnhancementSourceViaRFC opens a one-shot WebSocket to ZADT_VSP and
// reads the plug-in's REPOSRC entry via the rfc/readSource action (native
// READ REPORT on the server side). Returns ("", false) — without surfacing
// the error — when the bridge is missing, the program doesn't exist, or
// the response shape is unexpected. Step 4 (the metadata-only error)
// handles the user-facing fallback message.
//
// The program name is taken from ref.EnhInclude when populated by the
// ENHINCINX table fallback; otherwise the "<ENHNAME>E" REPOSRC convention
// is used as a best-effort guess.
//
// We deliberately avoid RPY_PROGRAM_READ: on classic ECC it raises CANCELLED
// (subrc=99 via OTHERS) inside RFC contexts because of an authorization
// dialog the framework can't render. The readSource action wraps native
// READ REPORT instead, which has none of that machinery.
func (c *Client) tryFetchEnhancementSourceViaRFC(ctx context.Context, ref *EnhancementRef) (string, bool) {
	programName := ref.EnhInclude
	if programName == "" {
		programName = ref.Name + "E"
	}
	programName = strings.ToUpper(strings.TrimSpace(programName))
	if programName == "E" {
		return "", false
	}

	factory := c.rfcFetcherFactory
	if factory == nil {
		factory = c.defaultRFCSourceFetcher
	}

	fetcher, err := factory(ctx)
	if err != nil {
		return "", false
	}
	defer fetcher.Close()

	lines, err := fetcher.ReadSource(ctx, programName)
	if err != nil || len(lines) == 0 {
		return "", false
	}
	return strings.Join(lines, "\n"), true
}

// defaultRFCSourceFetcher opens a fresh DebugWebSocketClient bound to the
// ZADT_VSP service and returns it as an rfcSourceFetcher. Used only when
// the test factory hook is not set.
func (c *Client) defaultRFCSourceFetcher(ctx context.Context) (rfcSourceFetcher, error) {
	if c.config == nil {
		return nil, fmt.Errorf("client config is nil")
	}
	if !c.config.HasBasicAuth() && !c.config.HasCookieAuth() {
		return nil, fmt.Errorf("RFC source fetch requires basic-auth credentials or cookies")
	}
	ws := NewDebugWebSocketClient(
		c.config.BaseURL,
		c.config.Client,
		c.config.Username,
		c.config.Password,
		c.config.InsecureSkipVerify,
	)
	if c.config.HasCookieAuth() {
		ws.SetCookies(c.config.Cookies)
	}
	if err := ws.Connect(ctx); err != nil {
		return nil, err
	}
	return ws, nil
}

// tryFetchEnhancementSource issues a single GET; returns (body, true) on 2xx.
// Any 4xx/5xx returns ("", false) without surfacing the error so the caller
// can fall through to the next attempt.
func (c *Client) tryFetchEnhancementSource(ctx context.Context, path string) (string, bool) {
	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method: http.MethodGet,
		Accept: "text/plain",
	})
	if err != nil {
		return "", false
	}
	if len(resp.Body) == 0 {
		return "", false
	}
	body := string(resp.Body)
	// ADT error responses are XML even on 200 from some upstream proxies; sniff
	// for the exception namespace as a safety net.
	if strings.Contains(body[:min(len(body), 256)], "exc:exception") {
		return "", false
	}
	return body, true
}

// resolveEnhancement finds exactly one ENHO/* match for name via SearchObject.
func (c *Client) resolveEnhancement(ctx context.Context, name string) (*EnhancementRef, error) {
	results, err := c.SearchObject(ctx, name, 25)
	if err != nil {
		return nil, fmt.Errorf("resolving enhancement %s: %w", name, err)
	}

	var hits []EnhancementRef
	for _, r := range results {
		if !strings.HasPrefix(strings.ToUpper(r.Type), "ENHO/") {
			continue
		}
		if !strings.EqualFold(r.Name, name) {
			continue
		}
		kind := strings.TrimPrefix(strings.ToUpper(r.Type), "ENHO/")
		hits = append(hits, EnhancementRef{
			Name:        r.Name,
			Kind:        EnhancementKind(kind),
			URI:         r.URI,
			PackageName: r.PackageName,
			Description: r.Description,
		})
	}

	switch len(hits) {
	case 0:
		return nil, fmt.Errorf("enhancement %s not found (no ENHO/* match)", name)
	case 1:
		return &hits[0], nil
	default:
		kinds := make([]string, 0, len(hits))
		for _, h := range hits {
			kinds = append(kinds, string(h.Kind))
		}
		sort.Strings(kinds)
		return nil, fmt.Errorf("enhancement %s is ambiguous across subtypes: %s", name, strings.Join(kinds, ", "))
	}
}

// enhancementBrowserResponse matches the XML shape returned by
// /sap/bc/adt/enhancements/enhoxhs — the enhancement browser's list endpoint.
// It is structurally identical to adtcore:objectReferences, so we reuse the
// SearchResult shape.
type enhancementBrowserResponse struct {
	XMLName xml.Name       `xml:"objectReferences"`
	Results []SearchResult `xml:"objectReference"`
}

// ListEnhancementsForInclude returns all ENHO implementations whose enhanced
// object is the named include. Returns an empty slice (not an error) when
// the include has no enhancements.
//
// Three back-ends are tried in order until one returns rows:
//  1. Modern enhancement-browser REST: GET /sap/bc/adt/enhancements/enhoxhs?enhancedObjectUri=…
//  2. Generic object-references REST: GET /sap/bc/adt/repository/informationsystem/objectreferences
//  3. Table-based fallback (D010INC ⨝ ENHINCINX) for ECC / older NetWeaver
//     systems where the REST surface is missing for HOOK_IMPL.
//
// (3) is the only path that works on classic ECC; both REST endpoints 404 on
// those systems for HOOK_IMPL plug-ins.
func (c *Client) ListEnhancementsForInclude(ctx context.Context, includeName string) ([]EnhancementRef, error) {
	includeName = strings.ToUpper(strings.TrimSpace(includeName))
	if includeName == "" {
		return nil, fmt.Errorf("include name is required")
	}

	includeURI := "/sap/bc/adt/programs/includes/" + url.PathEscape(includeName)

	// 1) Modern enhancement browser.
	if refs, err := c.queryEnhancementBrowser(ctx, includeURI); err == nil && len(refs) > 0 {
		return refs, nil
	}

	// 2) Generic object-references fallback.
	if refs, err := c.queryObjectReferencesForEnhancements(ctx, includeURI); err == nil && len(refs) > 0 {
		return refs, nil
	}

	// 3) Table-based fallback. Returns its own error (or empty slice) so
	// callers can distinguish "no enhancements attached" from "lookup failed".
	return c.listEnhancementsForIncludeViaTable(ctx, includeName)
}

func (c *Client) queryEnhancementBrowser(ctx context.Context, enhancedURI string) ([]EnhancementRef, error) {
	params := url.Values{}
	params.Set("enhancedObjectUri", enhancedURI)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/enhancements/enhoxhs", &RequestOptions{
		Method: http.MethodGet,
		Query:  params,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, err
	}

	var parsed enhancementBrowserResponse
	if xmlErr := xml.Unmarshal(resp.Body, &parsed); xmlErr != nil {
		// Try the SearchResults shape as a secondary parse — some releases
		// wrap in <adtcore:objectReferences>.
		if results, parseErr := ParseSearchResults(resp.Body); parseErr == nil {
			parsed.Results = results
		} else {
			return nil, fmt.Errorf("parsing enhancement browser response: %w", xmlErr)
		}
	}

	return filterENHO(parsed.Results), nil
}

func (c *Client) queryObjectReferencesForEnhancements(ctx context.Context, enhancedURI string) ([]EnhancementRef, error) {
	params := url.Values{}
	params.Set("uri", enhancedURI)
	params.Set("facet", "package")

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/repository/informationsystem/objectreferences", &RequestOptions{
		Method: http.MethodGet,
		Query:  params,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("fallback objectreferences query: %w", err)
	}

	results, err := ParseSearchResults(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing objectreferences response: %w", err)
	}
	return filterENHO(results), nil
}

func filterENHO(results []SearchResult) []EnhancementRef {
	refs := make([]EnhancementRef, 0, len(results))
	for _, r := range results {
		t := strings.ToUpper(r.Type)
		if !strings.HasPrefix(t, "ENHO/") {
			continue
		}
		refs = append(refs, EnhancementRef{
			Name:        r.Name,
			Kind:        EnhancementKind(strings.TrimPrefix(t, "ENHO/")),
			URI:         r.URI,
			PackageName: r.PackageName,
			Description: r.Description,
		})
	}
	return refs
}

// listEnhancementsForIncludeViaTable is the table-driven fallback used when
// the ADT enhancement-browser REST surface is missing (classic ECC / older
// NetWeaver). It joins:
//
//   - D010INC.INCLUDE = <name>            → MASTER (host main program)
//   - ENHINCINX.PROGRAMNAME IN (masters)  → ENHO rows
//   - ENHHEADER (optional enrichment)     → description, version, ENHTOOLTYPE
//
// Then narrows to rows whose FULL_NAME mentions a FORM that is defined in
// includeName. When the FORM-in-include check is inconclusive (we cannot tell
// for sure), the row is returned anyway — false positives are far less harmful
// than false negatives for the footer use case.
//
// Only HOOK_IMPL (Kind XH) plug-ins live in ENHINCINX; other ENHO subtypes are
// not surfaced via this fallback. That matches v1 scope.
func (c *Client) listEnhancementsForIncludeViaTable(ctx context.Context, includeName string) ([]EnhancementRef, error) {
	// Step 1: include → host main program(s).
	masters, err := c.queryD010INCMasters(ctx, includeName)
	if err != nil {
		return nil, fmt.Errorf("d010inc lookup for include %s: %w", includeName, err)
	}
	if len(masters) == 0 {
		// Include has no host program — either it's an executable/standalone
		// include with no function group, or the include name is invalid. Either
		// way, the table fallback can't help.
		return nil, nil
	}

	// Step 2: ENHINCINX rows for those host programs.
	enhRows, err := c.queryENHINCINXForMasters(ctx, masters)
	if err != nil {
		return nil, fmt.Errorf("enhincinx lookup: %w", err)
	}
	if len(enhRows) == 0 {
		return nil, nil
	}

	// Step 3: optional enrichment from ENHHEADER for descriptions / version.
	// Best-effort — failures fall back to ENHINCINX-only fields.
	descByName := c.fetchENHHEADERDescriptions(ctx, enhRows)

	// Step 4: convert to EnhancementRef. The 30-char ENHNAME truncation is
	// pragmatic-resolved by trying ENHHEADER first (its ENHNAME is the full
	// 60-char identifier), falling back to the truncated name.
	refs := make([]EnhancementRef, 0, len(enhRows))
	for _, row := range enhRows {
		fullName := descByName.fullName(row.ENHNAME)
		desc := descByName.description(row.ENHNAME)
		pkg := descByName.packageName(row.ENHNAME)

		refs = append(refs, EnhancementRef{
			Name:        fullName,
			Kind:        EnhancementKind("XH"), // ENHINCINX = HOOK_IMPL only
			URI:         "/sap/bc/adt/enhancements/enhoxh/" + strings.ToLower(fullName),
			PackageName: pkg,
			Description: desc,
			FullName:    row.FULL_NAME,
			HostProgram: row.PROGRAMNAME,
			EnhInclude:  row.ENHINCLUDE,
		})
	}
	return refs, nil
}

// enhincinxRow holds the ENHINCINX columns we read for HOOK_IMPL lookup.
type enhincinxRow struct {
	ENHNAME     string // 30-char truncated key
	PROGRAMNAME string
	FULL_NAME   string
	ENHINCLUDE  string
}

func (c *Client) queryD010INCMasters(ctx context.Context, includeName string) ([]string, error) {
	// /datapreview/ddic requires a full SELECT in the body — bare WHERE clauses
	// fail with "Only SELECT statement is allowed".
	sql := fmt.Sprintf("SELECT MASTER FROM D010INC WHERE INCLUDE = '%s'",
		strings.ReplaceAll(includeName, "'", "''"))
	res, err := c.GetTableContents(ctx, "D010INC", 100, sql)
	if err != nil {
		return nil, err
	}
	masters := make([]string, 0, len(res.Rows))
	seen := make(map[string]struct{})
	for _, r := range res.Rows {
		m, _ := r["MASTER"].(string)
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		masters = append(masters, m)
	}
	return masters, nil
}

func (c *Client) queryENHINCINXForMasters(ctx context.Context, masters []string) ([]enhincinxRow, error) {
	if len(masters) == 0 {
		return nil, nil
	}
	// Build "PROGRAMNAME IN ('A','B',…)" inside a full SELECT — /datapreview/ddic
	// rejects bare WHERE clauses.
	quoted := make([]string, 0, len(masters))
	for _, m := range masters {
		quoted = append(quoted, "'"+strings.ReplaceAll(m, "'", "''")+"'")
	}
	sql := fmt.Sprintf(
		"SELECT ENHNAME, PROGRAMNAME, FULL_NAME, ENHINCLUDE FROM ENHINCINX WHERE PROGRAMNAME IN (%s)",
		strings.Join(quoted, ","))

	res, err := c.GetTableContents(ctx, "ENHINCINX", 200, sql)
	if err != nil {
		return nil, err
	}
	rows := make([]enhincinxRow, 0, len(res.Rows))
	for _, r := range res.Rows {
		row := enhincinxRow{
			ENHNAME:     strings.TrimSpace(asString(r["ENHNAME"])),
			PROGRAMNAME: strings.TrimSpace(asString(r["PROGRAMNAME"])),
			FULL_NAME:   strings.TrimSpace(asString(r["FULL_NAME"])),
			ENHINCLUDE:  strings.TrimSpace(asString(r["ENHINCLUDE"])),
		}
		if row.ENHNAME == "" {
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// enhHeaderInfo carries enrichment data resolved from ENHHEADER, keyed by the
// 30-char ENHINCINX name (which is what the caller has).
type enhHeaderInfo struct {
	byTruncated map[string]enhHeaderHit
}

type enhHeaderHit struct {
	FullName    string
	Description string
	PackageName string
}

func (e enhHeaderInfo) fullName(truncated string) string {
	if h, ok := e.byTruncated[truncated]; ok && h.FullName != "" {
		return h.FullName
	}
	return truncated
}

func (e enhHeaderInfo) description(truncated string) string {
	if h, ok := e.byTruncated[truncated]; ok {
		return h.Description
	}
	return ""
}

func (e enhHeaderInfo) packageName(truncated string) string {
	if h, ok := e.byTruncated[truncated]; ok {
		return h.PackageName
	}
	return ""
}

// fetchENHHEADERDescriptions enriches ENHINCINX rows with full ENHNAME (60 char)
// and metadata from ENHHEADER + TADIR. Best-effort; returns an empty map on any
// hard failure so the caller still produces useful refs.
func (c *Client) fetchENHHEADERDescriptions(ctx context.Context, rows []enhincinxRow) enhHeaderInfo {
	info := enhHeaderInfo{byTruncated: map[string]enhHeaderHit{}}
	if len(rows) == 0 {
		return info
	}
	// ENHHEADER.ENHNAME is the full key; ENHINCINX truncates to 30. Use a
	// per-row LIKE prefix query — not the most efficient but bounded by the
	// number of ENHOs targeting one function group, which is small in practice.
	for _, row := range rows {
		// ENHHEADER active version only — /datapreview/ddic requires a full SELECT.
		sql := fmt.Sprintf("SELECT ENHNAME FROM ENHHEADER WHERE ENHNAME LIKE '%s%%' AND VERSION = 'A'",
			strings.ReplaceAll(row.ENHNAME, "'", "''"))
		res, err := c.GetTableContents(ctx, "ENHHEADER", 5, sql)
		if err != nil || res == nil || len(res.Rows) == 0 {
			continue
		}
		// Prefer the row whose ENHNAME starts with the truncated key; if
		// multiple, take the first.
		hit := res.Rows[0]
		info.byTruncated[row.ENHNAME] = enhHeaderHit{
			FullName:    strings.TrimSpace(asString(hit["ENHNAME"])),
			Description: "", // ENHHEADER has SHORTTEXT_ID (a key into another table); descriptions come from search instead.
			PackageName: "",
		}
	}
	// Augment with package + description from a single ADT search per ENHO.
	// The search endpoint is reliable on D03 even when the enhancement-browser
	// REST is broken. Cap to first 25 rows to bound cost.
	for i, row := range rows {
		if i >= 25 {
			break
		}
		full := info.fullName(row.ENHNAME)
		results, err := c.SearchObject(ctx, full, 5)
		if err != nil {
			continue
		}
		for _, r := range results {
			if !strings.HasPrefix(strings.ToUpper(r.Type), "ENHO/") {
				continue
			}
			if !strings.EqualFold(r.Name, full) {
				continue
			}
			h := info.byTruncated[row.ENHNAME]
			h.FullName = r.Name
			h.Description = r.Description
			h.PackageName = r.PackageName
			info.byTruncated[row.ENHNAME] = h
			break
		}
	}
	return info
}

// asString coerces a TableContentsResult cell to a string, returning "" if
// the cell is missing or of an unexpected type.
func asString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// GetIncludeMerged returns the include's raw source with each referenced
// ENHO implementation spliced in at its anchor. The output is not valid
// ABAP you can re-upload — it's an annotated view that matches what SE80
// shows with "Display Source (Modified)" enabled.
//
// Anchor resolution is best-effort. When an anchor cannot be located, the
// corresponding ENHO source is appended at the end with an explanatory
// comment header; no error is returned for a single unresolved anchor.
//
// When the include has no enhancements this is equivalent to GetInclude.
func (c *Client) GetIncludeMerged(ctx context.Context, includeName string) (string, error) {
	raw, err := c.GetInclude(ctx, includeName)
	if err != nil {
		return "", fmt.Errorf("merged include %s: %w", includeName, err)
	}

	refs, err := c.ListEnhancementsForInclude(ctx, includeName)
	if err != nil {
		// Non-fatal: return the raw include with a warning banner so the
		// caller still gets something useful.
		return raw + "\n\n* === enhancement lookup failed: " + err.Error() + " ===\n", nil
	}
	if len(refs) == 0 {
		return raw, nil
	}

	type enhWithSource struct {
		ref    EnhancementRef
		source string
	}
	enhanced := make([]enhWithSource, 0, len(refs))
	for _, ref := range refs {
		src, ferr := c.GetEnhancement(ctx, ref.Name)
		if ferr != nil {
			// Record the failure inline but keep going.
			enhanced = append(enhanced, enhWithSource{
				ref:    ref,
				source: fmt.Sprintf("* <failed to fetch ENHO %s: %v>", ref.Name, ferr),
			})
			continue
		}
		enhanced = append(enhanced, enhWithSource{ref: ref, source: src})
	}

	merged := raw
	unresolved := make([]enhWithSource, 0)
	for _, e := range enhanced {
		ok, spliced := spliceAtAnchor(merged, e.ref, e.source)
		if ok {
			merged = spliced
			continue
		}
		unresolved = append(unresolved, e)
	}

	if len(unresolved) > 0 {
		var b strings.Builder
		b.WriteString(merged)
		if !strings.HasSuffix(merged, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n* === unresolved enhancements (anchor not found in include) ===\n")
		for _, e := range unresolved {
			b.WriteString(renderEnhBlock(e.ref, e.source, "anchor unresolved"))
		}
		merged = b.String()
	}

	return merged, nil
}

// anchorStartPattern matches the "$*$\SE:(N) Form X, Start" marker that the
// enhancement framework writes into the host include at each plug-in point.
// N is the anchor index inside the form/include. We use this for splicing
// because it is the most reliable, machine-readable anchor SAP emits.
var anchorStartPattern = regexp.MustCompile(`(?i)\$\*\$\\SE:\((\d+)\)\s+Form\s+([A-Z0-9_/]+),\s+Start`)

// spliceAtAnchor finds the first unoccupied anchor line in src that could
// host the given enhancement and inserts the enhancement's source after it.
// Returns (false, src) unchanged if no suitable anchor was found.
//
// Heuristic: we insert after the first anchor Start marker we encounter
// that does not already have an ENHANCEMENT ... ENDENHANCEMENT block
// directly below it. For includes with multiple anchors we may not pick
// the exact correct one without parsing the ENHO metadata — which is OK
// because both produce a readable merged view; the caller accepts this
// trade-off (documented on GetIncludeMerged).
func spliceAtAnchor(src string, ref EnhancementRef, enhSource string) (bool, string) {
	loc := anchorStartPattern.FindStringIndex(src)
	if loc == nil {
		return false, src
	}

	// Insert immediately after the line that contains the match.
	lineEnd := strings.IndexByte(src[loc[1]:], '\n')
	insertAt := loc[1]
	if lineEnd >= 0 {
		insertAt += lineEnd + 1
	}

	// Skip if an ENHANCEMENT block is already present just below this
	// anchor — we already rendered this one on a previous iteration, or
	// the include author inlined it manually.
	tail := src[insertAt:]
	if looksLikeEnhancementBlock(tail) {
		// Try to find the next anchor instead.
		rest := src[insertAt:]
		nextLoc := anchorStartPattern.FindStringIndex(rest)
		if nextLoc == nil {
			return false, src
		}
		// Recurse into the tail.
		ok, splicedTail := spliceAtAnchor(rest, ref, enhSource)
		if !ok {
			return false, src
		}
		return true, src[:insertAt] + splicedTail
	}

	block := renderEnhBlock(ref, enhSource, "")
	return true, src[:insertAt] + block + src[insertAt:]
}

func looksLikeEnhancementBlock(s string) bool {
	// Scan at most ~200 bytes; ENHANCEMENT/ENDENHANCEMENT are the markers.
	head := s
	if len(head) > 400 {
		head = head[:400]
	}
	return regexp.MustCompile(`(?i)^\s*ENHANCEMENT\s+\d+`).MatchString(head)
}

func renderEnhBlock(ref EnhancementRef, source, note string) string {
	var b strings.Builder
	kind := string(ref.Kind)
	if kind == "" {
		kind = "?"
	}
	b.WriteString("\n* vvv ENHO/")
	b.WriteString(kind)
	b.WriteString(" ")
	b.WriteString(ref.Name)
	if ref.PackageName != "" {
		b.WriteString(" (package ")
		b.WriteString(ref.PackageName)
		b.WriteString(")")
	}
	if note != "" {
		b.WriteString(" — ")
		b.WriteString(note)
	}
	b.WriteString(" vvv\n")
	b.WriteString(source)
	if !strings.HasSuffix(source, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("* ^^^ end of ")
	b.WriteString(ref.Name)
	b.WriteString(" ^^^\n")
	return b.String()
}
