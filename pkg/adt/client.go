package adt

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client is the main ADT API client.
type Client struct {
	transport *Transport
	config    *Config

	// Keep-alive goroutine management
	keepAliveCancel context.CancelFunc
	keepAliveDone   chan struct{}
	keepAliveMu     sync.Mutex

	// Lock handles believed outstanding, so the keep-alive ping does not
	// retire the session one of them is bound to. See lock_window.go.
	locks lockWindow
}

// NewClient creates a new ADT client with the given configuration.
func NewClient(baseURL, username, password string, opts ...Option) *Client {
	cfg := NewConfig(baseURL, username, password, opts...)
	return &Client{
		transport: NewTransport(cfg),
		config:    cfg,
	}
}

// NewClientWithTransport creates a new client with a custom transport.
// This is useful for testing.
func NewClientWithTransport(cfg *Config, transport *Transport) *Client {
	return &Client{
		transport: transport,
		config:    cfg,
	}
}

// StartKeepAlive starts a background goroutine that periodically pings the SAP server
// to keep the session alive. This is especially useful for cookie/browser-auth sessions
// which can time out during idle periods. The interval should be shorter than the SAP
// server's session timeout. A reasonable default is 5 minutes.
// Calling StartKeepAlive again stops any existing keep-alive before starting a new one.
func (c *Client) StartKeepAlive(interval time.Duration, verbose bool) {
	c.keepAliveMu.Lock()
	defer c.keepAliveMu.Unlock()

	// Stop existing keep-alive if running
	c.stopKeepAliveLocked()

	ctx, cancel := context.WithCancel(context.Background())
	c.keepAliveCancel = cancel
	c.keepAliveDone = make(chan struct{})

	go func() {
		defer close(c.keepAliveDone)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		if verbose {
			fmt.Fprintf(LogOutput, "[KEEPALIVE] Started (interval: %s)\n", interval)
		}

		for {
			select {
			case <-ctx.Done():
				if verbose {
					fmt.Fprintf(LogOutput, "[KEEPALIVE] Stopped\n")
				}
				return
			case <-ticker.C:
				// A ping is an ordinary request, and an ordinary request is
				// stamped stateless — which retires the session a lock handle
				// lives in. Skipping a tick costs nothing; sending it during a
				// write costs the write (#168).
				if c.lockOutstanding() {
					if verbose {
						fmt.Fprintf(LogOutput, "[KEEPALIVE] Skipped: a lock is outstanding\n")
					}
					continue
				}
				if err := c.transport.Ping(ctx); err != nil {
					if ctx.Err() != nil {
						return // context cancelled, expected
					}
					if verbose {
						fmt.Fprintf(LogOutput, "[KEEPALIVE] Ping failed: %v\n", err)
					}
				} else if verbose {
					fmt.Fprintf(LogOutput, "[KEEPALIVE] Ping OK\n")
				}
			}
		}
	}()
}

// StopKeepAlive stops the background keep-alive goroutine if running.
func (c *Client) StopKeepAlive() {
	c.keepAliveMu.Lock()
	defer c.keepAliveMu.Unlock()
	c.stopKeepAliveLocked()
}

// stopKeepAliveLocked stops the keep-alive goroutine. Must be called with keepAliveMu held.
func (c *Client) stopKeepAliveLocked() {
	if c.keepAliveCancel != nil {
		c.keepAliveCancel()
		<-c.keepAliveDone
		c.keepAliveCancel = nil
		c.keepAliveDone = nil
	}
}

// checkSafety checks if an operation is allowed by the safety configuration.
func (c *Client) checkSafety(op OperationType, opName string) error {
	return c.config.Safety.CheckOperation(op, opName)
}

// checkPackageSafety checks if operations on a package are allowed.
func (c *Client) checkPackageSafety(pkg string) error {
	return c.config.Safety.CheckPackage(pkg)
}

// checkObjectPackageSafety resolves the package for an existing object and
// validates it against the configured package whitelist.
func (c *Client) checkObjectPackageSafety(ctx context.Context, objectURL string) error {
	if len(c.config.Safety.AllowedPackages) == 0 {
		return nil
	}

	pkg, err := c.getObjectPackage(ctx, objectURL)
	if err != nil {
		return fmt.Errorf("resolving package for %s: %w", normalizeObjectURLForPackageCheck(objectURL), err)
	}

	return c.checkPackageSafety(pkg)
}

// checkTransportableEdit checks if editing objects that require transports is allowed.
func (c *Client) checkTransportableEdit(transport, opName string) error {
	return c.config.Safety.CheckTransportableEdit(transport, opName)
}

func (c *Client) getObjectPackage(ctx context.Context, objectURL string) (string, error) {
	normalized := normalizeObjectURLForPackageCheck(objectURL)
	objectName, err := objectNameFromURL(normalized)
	if err != nil {
		return "", err
	}

	results, err := c.SearchObject(ctx, objectName, 20)
	if err != nil {
		return "", err
	}

	canonicalURL := canonicalizeObjectURL(normalized)
	for _, result := range results {
		if result.PackageName == "" {
			continue
		}
		if canonicalizeObjectURL(result.URI) == canonicalURL {
			return result.PackageName, nil
		}
	}

	return "", fmt.Errorf("package metadata not found")
}

func normalizeObjectURLForPackageCheck(objectURL string) string {
	normalized := strings.TrimSuffix(objectURL, "/")

	if strings.HasSuffix(normalized, "/source/main") {
		normalized = strings.TrimSuffix(normalized, "/source/main")
	}

	// Strip /includes/... only for class sub-resources (e.g. /oo/classes/ZCL_FOO/includes/locals_def).
	// Program includes use /programs/includes/NAME where /includes/ is the collection path — don't strip.
	if idx := strings.Index(normalized, "/includes/"); idx >= 0 {
		prefix := normalized[:idx]
		if !strings.HasSuffix(prefix, "/programs") {
			return prefix
		}
	}

	return normalized
}

func canonicalizeObjectURL(objectURL string) string {
	normalized := normalizeObjectURLForPackageCheck(objectURL)
	if decoded, err := url.PathUnescape(normalized); err == nil {
		normalized = decoded
	}
	return strings.ToLower(strings.TrimSuffix(normalized, "/"))
}

func objectNameFromURL(objectURL string) (string, error) {
	normalized := normalizeObjectURLForPackageCheck(objectURL)
	parts := strings.Split(strings.Trim(normalized, "/"), "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid object URL")
	}

	name, err := url.PathUnescape(parts[len(parts)-1])
	if err != nil {
		return "", fmt.Errorf("decoding object name: %w", err)
	}
	if name == "" {
		return "", fmt.Errorf("invalid object URL")
	}

	return strings.ToUpper(name), nil
}

// Safety returns the safety configuration for checking transport operations.
func (c *Client) Safety() *SafetyConfig {
	return &c.config.Safety
}

// AllowPackageTemporarily adds a package to the allowed list for the duration of
// an install/bootstrap operation. Returns a cleanup function that removes it.
// This is used by install tools (InstallZADTVSP, InstallAbapGit) which are
// self-contained bootstrap operations that should not be blocked by
// SAP_ALLOWED_PACKAGES restrictions.
func (c *Client) AllowPackageTemporarily(pkg string) func() {
	// If no package restrictions are configured, nothing to do
	if len(c.config.Safety.AllowedPackages) == 0 {
		return func() {}
	}

	// If already allowed, nothing to do
	if c.config.Safety.IsPackageAllowed(pkg) {
		return func() {}
	}

	// Add to allowed packages
	c.config.Safety.AllowedPackages = append(c.config.Safety.AllowedPackages, pkg)

	// Return cleanup function
	return func() {
		// Remove the temporarily added package
		for i, p := range c.config.Safety.AllowedPackages {
			if strings.EqualFold(p, pkg) {
				c.config.Safety.AllowedPackages = append(
					c.config.Safety.AllowedPackages[:i],
					c.config.Safety.AllowedPackages[i+1:]...,
				)
				return
			}
		}
	}
}

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
		Method: http.MethodGet,
		Query:  params,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}

	return ParseSearchResults(resp.Body)
}

// --- Program Operations ---

// GetProgram retrieves the source code of an ABAP program.
// Supports namespaced programs like /UI5/UI5_REPOSITORY_LOAD.
func (c *Client) GetProgram(ctx context.Context, programName string) (string, error) {
	programName = strings.ToUpper(programName)

	// Go directly to source/main endpoint (URL encode for namespaced objects)
	sourcePath := fmt.Sprintf("/sap/bc/adt/programs/programs/%s/source/main", url.PathEscape(programName))
	resp, err := c.transport.Request(ctx, sourcePath, &RequestOptions{
		Method: http.MethodGet,
	})
	if err != nil {
		// TADIR calls an include a PROG, and ADT does not: an include lives at
		// /programs/includes and answers 404 at /programs/programs. So a whole
		// class of object listed in a package as a program cannot be read as
		// one — 53 of them in SBRF on a stock 7.58, every one of which has
		// source and is active, and every one of which was being reported as
		// unreadable.
		//
		// Retrying rather than looking the type up first: REPOSRC.SUBC would
		// answer authoritatively but costs a query for every program in a
		// package to save one for the few that are includes. The 404 is the
		// same information arriving later and for free.
		if isNotFound(err) {
			if src, incErr := c.GetInclude(ctx, programName); incErr == nil {
				return src, nil
			}
		}
		return "", fmt.Errorf("getting program source: %w", err)
	}

	return string(resp.Body), nil
}

// isNotFound reports whether an error is ADT saying the resource does not
// exist, as against saying anything else. It matters that this is narrow: a
// retry on an authorisation failure or a timeout would turn one clear error
// into two vague ones.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "status 404") || strings.Contains(msg, "ExceptionResourceNotFound")
}

// --- Class Operations ---

// GetClass retrieves the source code of an ABAP class.
// It returns a map of include names to source code.
// Supports namespaced classes like /UI5/CL_REPOSITORY_LOAD.
func (c *Client) GetClass(ctx context.Context, className string) (map[string]string, error) {
	className = strings.ToUpper(className)

	// Go directly to source/main endpoint (URL encode for namespaced objects)
	sourcePath := fmt.Sprintf("/sap/bc/adt/oo/classes/%s/source/main", url.PathEscape(className))
	resp, err := c.transport.Request(ctx, sourcePath, &RequestOptions{
		Method: http.MethodGet,
	})
	if err != nil {
		return nil, fmt.Errorf("getting class source: %w", err)
	}

	sources := make(map[string]string)
	sources["main"] = string(resp.Body)

	return sources, nil
}

// GetClassSource retrieves just the main source code of an ABAP class.
func (c *Client) GetClassSource(ctx context.Context, className string) (string, error) {
	sources, err := c.GetClass(ctx, className)
	if err != nil {
		return "", err
	}
	return sources["main"], nil
}

// GetClassMethods retrieves the list of methods in a class with their source line boundaries.
// This is useful for method-level source operations (GetSource with method, EditSource with method).
func (c *Client) GetClassMethods(ctx context.Context, className string) ([]MethodInfo, error) {
	className = strings.ToUpper(className)

	// Fetch objectstructure endpoint
	path := fmt.Sprintf("/sap/bc/adt/oo/classes/%s/objectstructure", url.PathEscape(className))
	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.objectstructure.v2+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting class object structure: %w", err)
	}

	structure, err := ParseClassObjectStructure(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing class object structure: %w", err)
	}

	return structure.GetMethods(), nil
}

// GetClassObjectStructure returns the full parsed class structure (methods, attributes, types, events).
func (c *Client) GetClassObjectStructure(ctx context.Context, className string) (*ClassObjectStructure, error) {
	className = strings.ToUpper(className)

	path := fmt.Sprintf("/sap/bc/adt/oo/classes/%s/objectstructure", url.PathEscape(className))
	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.objectstructure.v2+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting class object structure: %w", err)
	}

	return ParseClassObjectStructure(resp.Body)
}

// GetClassMethodSource retrieves the source code of a specific method in a class.
// Returns only the METHOD...ENDMETHOD block for the specified method.
func (c *Client) GetClassMethodSource(ctx context.Context, className, methodName string) (string, error) {
	className = strings.ToUpper(className)
	methodName = strings.ToUpper(methodName)

	// Get method boundaries
	methods, err := c.GetClassMethods(ctx, className)
	if err != nil {
		return "", fmt.Errorf("getting class methods: %w", err)
	}

	// Find the specified method
	var method *MethodInfo
	for i := range methods {
		if methods[i].Name == methodName {
			method = &methods[i]
			break
		}
	}
	if method == nil {
		return "", fmt.Errorf("method %s not found in class %s", methodName, className)
	}

	if method.ImplementationStart == 0 || method.ImplementationEnd == 0 {
		return "", fmt.Errorf("method %s has no implementation", methodName)
	}

	// Get full class source
	fullSource, err := c.GetClassSource(ctx, className)
	if err != nil {
		return "", fmt.Errorf("getting class source: %w", err)
	}

	// Extract method lines
	lines := strings.Split(fullSource, "\n")
	if method.ImplementationEnd > len(lines) {
		return "", fmt.Errorf("method line range (%d-%d) exceeds source lines (%d)",
			method.ImplementationStart, method.ImplementationEnd, len(lines))
	}

	// Line numbers are 1-based, slice indices are 0-based
	methodLines := lines[method.ImplementationStart-1 : method.ImplementationEnd]
	return strings.Join(methodLines, "\n"), nil
}

// --- Interface Operations ---

// GetInterface retrieves the source code of an ABAP interface.
// Supports namespaced interfaces like /UI5/IF_REPOSITORY_LOAD_ADPTER.
func (c *Client) GetInterface(ctx context.Context, interfaceName string) (string, error) {
	interfaceName = strings.ToUpper(interfaceName)

	// Go directly to source/main endpoint (URL encode for namespaced objects)
	sourcePath := fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s/source/main", url.PathEscape(interfaceName))
	resp, err := c.transport.Request(ctx, sourcePath, &RequestOptions{
		Method: http.MethodGet,
	})
	if err != nil {
		return "", fmt.Errorf("getting interface source: %w", err)
	}

	return string(resp.Body), nil
}

// --- Function Module Operations ---

// GetFunctionGroup retrieves the structure of a function group.
// Supports namespaced function groups like /UI5/UI5_REPOSITORY_LOAD.
func (c *Client) GetFunctionGroup(ctx context.Context, groupName string) (*FunctionGroup, error) {
	groupName = strings.ToUpper(groupName)

	// URL encode for namespaced objects
	structPath := fmt.Sprintf("/sap/bc/adt/functions/groups/%s", url.PathEscape(groupName))
	// S/4HANA rejects application/xml here (406). Use ADT vendor content types; keep
	// application/xml as a low-priority fallback for older systems.
	resp, err := c.transport.Request(ctx, structPath, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.functions.groups.v3+xml, application/vnd.sap.adt.functions.groups.v2+xml;q=0.9, application/xml;q=0.8",
	})
	if err != nil {
		return nil, fmt.Errorf("getting function group: %w", err)
	}

	var fg FunctionGroup
	if err := xml.Unmarshal(resp.Body, &fg); err != nil {
		return nil, fmt.Errorf("parsing function group: %w", err)
	}

	// The metadata document carries no modules, so the list is fetched
	// separately — see functions_list.go. A group whose modules cannot be
	// listed is still a group worth returning: the caller asked for the group,
	// and losing its metadata to a failure of the second call would be the
	// worse answer.
	if modules, err := c.ListFunctionModules(ctx, groupName); err == nil {
		fg.Functions = modules
	} else if c.config.Verbose {
		fmt.Fprintf(os.Stderr, "[WARN] function group %s: %v\n", groupName, err)
	}

	return &fg, nil
}

// GetFunctionGroupAllSources returns the concatenated source of a function group:
// the top include (source/main), every FUGR include (LxxxTOP, LxxxUXX, LxxxF01, ...),
// and every function module body. Intended for dependency analysis where the caller
// needs the full textual footprint of a FUGR, not just its metadata.
//
// The function group's objectstructure endpoint enumerates all FUGR/I (includes) and
// FUGR/FF (function modules); we resolve each child's source/main URI and concatenate.
// Individual sub-fetches that fail are skipped (best-effort) so a single broken include
// does not hide deps from the rest of the group.
//
// The second return value is what did not make it into the string, and callers
// must not throw it away. This source is fetched to be searched for
// dependencies, and an include that failed to load contributes no dependencies —
// which is indistinguishable, downstream, from an include that has none. That is
// how a boundary report comes back clean about code nobody read. The safety cap
// below lands in the same list, because a caveat written only to stderr is no
// caveat at all to an MCP caller.
func (c *Client) GetFunctionGroupAllSources(ctx context.Context, groupName string) (string, []Unsearched, error) {
	groupName = strings.ToLower(groupName)

	structPath := fmt.Sprintf("/sap/bc/adt/functions/groups/%s/objectstructure", url.PathEscape(groupName))
	resp, err := c.transport.Request(ctx, structPath, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.objectstructure.v2+xml",
	})
	if err != nil {
		return "", nil, fmt.Errorf("getting function group structure: %w", err)
	}

	type atomLink struct {
		Rel  string `xml:"rel,attr"`
		Href string `xml:"href,attr"`
	}
	type element struct {
		Name     string     `xml:"name,attr"`
		Type     string     `xml:"type,attr"`
		Links    []atomLink `xml:"link"`
		Children []element  `xml:"objectStructureElement"`
	}
	var root element
	if err := xml.Unmarshal(resp.Body, &root); err != nil {
		return "", nil, fmt.Errorf("parsing function group structure: %w", err)
	}

	seen := make(map[string]bool)
	var srcURIs []string

	// The root element is the FUGR itself — pick its source/main link so the TOP-level
	// INCLUDE skeleton is also analyzed.
	addLinks := func(e element) {
		for _, l := range e.Links {
			if strings.HasSuffix(l.Rel, "/source/definitionIdentifier") || strings.HasSuffix(l.Rel, "/definitionIdentifier") {
				if strings.Contains(l.Href, "/source/main") && !seen[l.Href] {
					seen[l.Href] = true
					srcURIs = append(srcURIs, l.Href)
				}
			}
		}
	}
	var walk func(e element)
	walk = func(e element) {
		// Include sources for the group itself (FUGR/F), its includes (FUGR/I*),
		// and its function modules (FUGR/FF).
		addLinks(e)
		for _, ch := range e.Children {
			walk(ch)
		}
	}
	walk(root)

	if len(srcURIs) == 0 {
		// Fallback: at least fetch the top-level source so we get something.
		srcURIs = []string{fmt.Sprintf("/sap/bc/adt/functions/groups/%s/source/main", url.PathEscape(groupName))}
	}
	sort.Strings(srcURIs)

	var missed []Unsearched

	// Safety cap. A pathological function group with hundreds of FMs would
	// otherwise produce a sequential fetch storm that looks like a hang. 150
	// is well above the largest normal FUGR (~50 modules) and keeps worst-
	// case latency bounded. The cut goes into missed as well as to stderr: an
	// MCP caller has no stderr, and the whole point of the cap is that the
	// analysis is partial.
	const maxFUGRSubfetches = 150
	if len(srcURIs) > maxFUGRSubfetches {
		fmt.Fprintf(os.Stderr, "    [FUGR %s] capped at %d of %d sub-URIs\n",
			strings.ToUpper(groupName), maxFUGRSubfetches, len(srcURIs))
		for _, uri := range srcURIs[maxFUGRSubfetches:] {
			missed = append(missed, Unsearched{
				Object: uri,
				Reason: fmt.Sprintf("not fetched: function group has %d sub-sources, capped at %d", len(srcURIs), maxFUGRSubfetches),
			})
		}
		srcURIs = srcURIs[:maxFUGRSubfetches]
	}
	fmt.Fprintf(os.Stderr, "    [FUGR %s] fetching %d sub-sources\n",
		strings.ToUpper(groupName), len(srcURIs))

	type fetchResult struct {
		idx  int
		body string
		err  error
	}
	const fugrWorkers = 6
	jobCh := make(chan int)
	resCh := make(chan fetchResult, len(srcURIs))

	var wg sync.WaitGroup
	for w := 0; w < fugrWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobCh {
				if ctx.Err() != nil {
					return
				}
				r, err := c.transport.Request(ctx, srcURIs[idx], &RequestOptions{
					Method: http.MethodGet,
					Accept: "text/plain",
				})
				if err != nil {
					resCh <- fetchResult{idx: idx, err: err}
					continue
				}
				resCh <- fetchResult{idx: idx, body: string(r.Body)}
			}
		}()
	}
	go func() {
		for idx := range srcURIs {
			jobCh <- idx
		}
		close(jobCh)
		wg.Wait()
		close(resCh)
	}()

	results := make([]string, len(srcURIs))
	answered := make([]bool, len(srcURIs))
	completed := 0
	for res := range resCh {
		results[res.idx] = res.body
		answered[res.idx] = true
		if res.err != nil {
			missed = append(missed, Unsearched{Object: srcURIs[res.idx], Reason: res.err.Error()})
		}
		completed++
		if completed == len(srcURIs) || completed%5 == 0 {
			fmt.Fprintf(os.Stderr, "    [FUGR %s] %d/%d sub-sources fetched\n",
				strings.ToUpper(groupName), completed, len(srcURIs))
		}
	}
	// A cancelled context stops the workers mid-queue, so some URIs never come
	// back at all — not even as an error. They are missing from the source just
	// the same, and only this pass can tell.
	for idx, ok := range answered {
		if !ok {
			reason := "not fetched: the fetch was cancelled before this sub-source was read"
			if ctx.Err() != nil {
				reason = "not fetched: " + ctx.Err().Error()
			}
			missed = append(missed, Unsearched{Object: srcURIs[idx], Reason: reason})
		}
	}

	var combined strings.Builder
	for _, body := range results {
		if body == "" {
			continue
		}
		combined.WriteString(body)
		combined.WriteString("\n")
	}
	return combined.String(), missed, nil
}

// GetFunction retrieves the source code of a function module.
// Supports namespaced function modules like /UI5/UI5_REPOSITORY_LOAD_HTTP.
func (c *Client) GetFunction(ctx context.Context, functionName, groupName string) (string, error) {
	functionName = strings.ToUpper(functionName)
	groupName = strings.ToUpper(groupName)

	// URL encode for namespaced objects
	sourcePath := fmt.Sprintf("/sap/bc/adt/functions/groups/%s/fmodules/%s/source/main",
		url.PathEscape(groupName), url.PathEscape(functionName))

	resp, err := c.transport.Request(ctx, sourcePath, &RequestOptions{
		Method: http.MethodGet,
		Accept: "text/plain",
	})
	if err != nil {
		return "", fmt.Errorf("getting function source: %w", err)
	}

	return string(resp.Body), nil
}

// --- Include Operations ---

// GetInclude retrieves the source code of an ABAP include.
// Supports namespaced includes.
func (c *Client) GetInclude(ctx context.Context, includeName string) (string, error) {
	includeName = strings.ToUpper(includeName)

	// URL encode for namespaced objects
	sourcePath := fmt.Sprintf("/sap/bc/adt/programs/includes/%s/source/main", url.PathEscape(includeName))
	resp, err := c.transport.Request(ctx, sourcePath, &RequestOptions{
		Method: http.MethodGet,
		Accept: "text/plain",
	})
	if err != nil {
		return "", fmt.Errorf("getting include source: %w", err)
	}

	return string(resp.Body), nil
}

// --- CDS DDL Source Operations ---

// GetDDLS retrieves the source code of a CDS DDL source (CDS view definition).
func (c *Client) GetDDLS(ctx context.Context, ddlsName string) (string, error) {
	ddlsName = strings.ToUpper(ddlsName)

	// URL encode the name to handle namespaced objects like /DMO/...
	sourcePath := fmt.Sprintf("/sap/bc/adt/ddic/ddl/sources/%s/source/main", url.PathEscape(ddlsName))
	resp, err := c.transport.Request(ctx, sourcePath, &RequestOptions{
		Method: http.MethodGet,
		Accept: "text/plain",
	})
	if err != nil {
		return "", fmt.Errorf("getting DDLS source: %w", err)
	}

	return string(resp.Body), nil
}

// --- RAP Object Operations (BDEF, SRVD, SRVB) ---

// GetBDEF retrieves the source code of a Behavior Definition.
// BDEF (Behavior Definition) defines the behavior (CRUD operations, actions, validations)
// for CDS entities in the RAP (RESTful Application Programming) model.
func (c *Client) GetBDEF(ctx context.Context, bdefName string) (string, error) {
	bdefName = strings.ToUpper(bdefName)

	// URL encode the name to handle namespaced objects like /DMO/...
	// BDEF endpoint is /sap/bc/adt/bo/behaviordefinitions/{name}/source/main
	sourcePath := fmt.Sprintf("/sap/bc/adt/bo/behaviordefinitions/%s/source/main", url.PathEscape(bdefName))
	resp, err := c.transport.Request(ctx, sourcePath, &RequestOptions{
		Method: http.MethodGet,
		Accept: "text/plain",
	})
	if err != nil {
		return "", fmt.Errorf("getting BDEF source: %w", err)
	}

	return string(resp.Body), nil
}

// GetSRVD retrieves the source code of a Service Definition.
// SRVD (Service Definition) exposes CDS entities as a service in the RAP model.
func (c *Client) GetSRVD(ctx context.Context, srvdName string) (string, error) {
	srvdName = strings.ToUpper(srvdName)

	// URL encode the name to handle namespaced objects like /DMO/...
	sourcePath := fmt.Sprintf("/sap/bc/adt/ddic/srvd/sources/%s/source/main", url.PathEscape(srvdName))
	resp, err := c.transport.Request(ctx, sourcePath, &RequestOptions{
		Method: http.MethodGet,
		Accept: "text/plain",
	})
	if err != nil {
		return "", fmt.Errorf("getting SRVD source: %w", err)
	}

	return string(resp.Body), nil
}

// ServiceBinding represents an OData Service Binding metadata
type ServiceBinding struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Description    string `json:"description"`
	Published      bool   `json:"published"`
	BindingType    string `json:"bindingType"`    // ODATA
	BindingVersion string `json:"bindingVersion"` // V2, V4
	ServiceURL     string `json:"serviceUrl,omitempty"`
	ServiceDefName string `json:"serviceDefName,omitempty"`
}

// GetSRVB retrieves metadata for a Service Binding.
// SRVB (Service Binding) binds a Service Definition to a specific protocol (OData V2/V4).
func (c *Client) GetSRVB(ctx context.Context, srvbName string) (*ServiceBinding, error) {
	srvbName = strings.ToUpper(srvbName)

	// URL encode the name to handle namespaced objects like /DMO/...
	path := fmt.Sprintf("/sap/bc/adt/businessservices/bindings/%s", url.PathEscape(srvbName))
	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method: http.MethodGet,
		Accept: "*/*", // Service bindings may require accepting any format
	})
	if err != nil {
		return nil, fmt.Errorf("getting SRVB metadata: %w", err)
	}

	return parseSRVBMetadata(resp.Body)
}

func parseSRVBMetadata(data []byte) (*ServiceBinding, error) {
	// Strip namespace prefixes
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "srvb:", "")
	xmlStr = strings.ReplaceAll(xmlStr, "adtcore:", "")

	type binding struct {
		Type    string `xml:"type,attr"`
		Version string `xml:"version,attr"`
	}
	type serviceRef struct {
		URI  string `xml:"uri,attr"`
		Type string `xml:"type,attr"`
		Name string `xml:"name,attr"`
	}
	type serviceContent struct {
		ServiceDef serviceRef `xml:"serviceDefinition"`
	}
	type service struct {
		Name    string         `xml:"name,attr"`
		Content serviceContent `xml:"content"`
	}
	type srvbRoot struct {
		Name        string  `xml:"name,attr"`
		Type        string  `xml:"type,attr"`
		Description string  `xml:"description,attr"`
		Published   bool    `xml:"published,attr"`
		Binding     binding `xml:"binding"`
		Services    service `xml:"services"`
	}

	var root srvbRoot
	if err := xml.Unmarshal([]byte(xmlStr), &root); err != nil {
		return nil, fmt.Errorf("parsing SRVB metadata: %w", err)
	}

	return &ServiceBinding{
		Name:           root.Name,
		Type:           root.Type,
		Description:    root.Description,
		Published:      root.Published,
		BindingType:    root.Binding.Type,
		BindingVersion: root.Binding.Version,
		ServiceDefName: root.Services.Content.ServiceDef.Name,
	}, nil
}

// --- Message Class Operations ---

// MessageClassMessage represents a single message in a message class
type MessageClassMessage struct {
	Number string `xml:"msgno,attr" json:"number"`
	Text   string `xml:"msgtext,attr" json:"text"`
}

// MessageClass represents an ABAP message class with all its messages
type MessageClass struct {
	Name        string                `xml:"name,attr" json:"name"`
	Description string                `xml:"description,attr" json:"description"`
	Messages    []MessageClassMessage `xml:"messages" json:"messages"`
}

// GetMessageClass retrieves all messages from an ABAP message class.
// Supports namespaced message classes.
func (c *Client) GetMessageClass(ctx context.Context, msgClassName string) (*MessageClass, error) {
	msgClassName = strings.ToUpper(msgClassName)

	// URL encode for namespaced objects
	path := fmt.Sprintf("/sap/bc/adt/messageclass/%s", url.PathEscape(strings.ToLower(msgClassName)))
	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.mc.messageclass+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting message class: %w", err)
	}

	// Parse XML into struct
	var mc MessageClass
	if err := xml.Unmarshal(resp.Body, &mc); err != nil {
		return nil, fmt.Errorf("parsing message class XML: %w", err)
	}

	mc.Name = msgClassName
	return &mc, nil
}

// --- Package Operations ---

// PackageExists returns true if SAP has a package with the given name.
// It probes /sap/bc/adt/packages/{name} directly: 200 → exists, 404 → not,
// any other outcome (5xx, network, auth) is returned as an error so the
// caller does not silently classify a transient failure as "missing".
//
// Unlike GetPackage, which reads the nodestructure API and cannot distinguish
// "package does not exist" from "package exists but has no children" (both
// return an empty tree), this is a definitive existence check.
func (c *Client) PackageExists(ctx context.Context, packageName string) (bool, error) {
	if packageName == "" {
		return false, fmt.Errorf("empty package name")
	}
	objectURL := fmt.Sprintf("/sap/bc/adt/packages/%s", url.PathEscape(strings.ToUpper(packageName)))
	_, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/*",
	})
	if err == nil {
		return true, nil
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, err
}

// GetPackage retrieves the contents of a package using the nodestructure API.
func (c *Client) GetPackage(ctx context.Context, packageName string) (*PackageContent, error) {
	packageName = strings.ToUpper(packageName)

	params := url.Values{}
	params.Set("parent_type", "DEVC/K")
	params.Set("parent_name", packageName)
	params.Set("withShortDescriptions", "true")

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/repository/nodestructure", &RequestOptions{
		Method: http.MethodPost,
		Query:  params,
	})
	if err != nil {
		return nil, fmt.Errorf("getting package contents: %w", err)
	}

	// Parse the nodestructure response
	return parsePackageNodeStructure(resp.Body, packageName)
}

// parsePackageNodeStructure parses the nodestructure XML response into PackageContent.
func parsePackageNodeStructure(data []byte, packageName string) (*PackageContent, error) {
	// Handle empty response (newly created packages may return no content)
	if len(data) == 0 {
		return &PackageContent{
			Name:        packageName,
			Objects:     []PackageObject{},
			SubPackages: []string{},
		}, nil
	}

	type nodeData struct {
		TreeContent struct {
			Nodes []struct {
				ObjectType string `xml:"OBJECT_TYPE"`
				ObjectName string `xml:"OBJECT_NAME"`
				ObjectURI  string `xml:"OBJECT_URI"`
				Desc       string `xml:"DESCRIPTION"`
			} `xml:"SEU_ADT_REPOSITORY_OBJ_NODE"`
		} `xml:"TREE_CONTENT"`
	}
	type abapValues struct {
		Data nodeData `xml:"DATA"`
	}
	type abapResponse struct {
		Values abapValues `xml:"values"`
	}

	var resp abapResponse
	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing nodestructure: %w", err)
	}

	pkg := &PackageContent{
		Name:        packageName,
		Objects:     []PackageObject{},
		SubPackages: []string{},
	}

	for _, node := range resp.Values.Data.TreeContent.Nodes {
		if node.ObjectName == "" {
			continue
		}
		if node.ObjectType == "DEVC/K" {
			pkg.SubPackages = append(pkg.SubPackages, node.ObjectName)
		} else {
			pkg.Objects = append(pkg.Objects, PackageObject{
				Type:        node.ObjectType,
				Name:        node.ObjectName,
				URI:         node.ObjectURI,
				Description: node.Desc,
			})
		}
	}

	return pkg, nil
}

// Language is the logon language the client was configured with, as an ISO
// code ("EN") or a SAP key; empty when none was set.
func (c *Client) Language() string {
	if c.config == nil {
		return ""
	}
	return c.config.Language
}

// --- Table Operations ---

// GetTable retrieves the source/definition of a database table.
func (c *Client) GetTable(ctx context.Context, tableName string) (string, error) {
	tableName = strings.ToUpper(tableName)

	// URL encode to handle namespaced objects like /DMO/TRAVEL
	sourcePath := fmt.Sprintf("/sap/bc/adt/ddic/tables/%s/source/main", url.PathEscape(tableName))
	resp, err := c.transport.Request(ctx, sourcePath, &RequestOptions{
		Method: http.MethodGet,
	})
	if err != nil {
		return "", fmt.Errorf("getting table source: %w", err)
	}

	return string(resp.Body), nil
}

// GetView retrieves the source/definition of a DDIC database view.
// This is for classic DDIC views (SE11), not CDS views (which use GetDDLS).
func (c *Client) GetView(ctx context.Context, viewName string) (string, error) {
	viewName = strings.ToUpper(viewName)

	// URL encode the name to handle namespaced objects like /DMO/...
	sourcePath := fmt.Sprintf("/sap/bc/adt/ddic/views/%s/source/main", url.PathEscape(viewName))
	resp, err := c.transport.Request(ctx, sourcePath, &RequestOptions{
		Method: http.MethodGet,
	})
	if err != nil {
		return "", fmt.Errorf("getting view source: %w", err)
	}

	return string(resp.Body), nil
}

// GetStructure retrieves the source/definition of a data structure.
func (c *Client) GetStructure(ctx context.Context, structName string) (string, error) {
	structName = strings.ToUpper(structName)

	// URL encode to handle namespaced objects like /DMO/...
	sourcePath := fmt.Sprintf("/sap/bc/adt/ddic/structures/%s/source/main", url.PathEscape(structName))
	resp, err := c.transport.Request(ctx, sourcePath, &RequestOptions{
		Method: http.MethodGet,
	})
	if err != nil {
		return "", fmt.Errorf("getting structure source: %w", err)
	}

	return string(resp.Body), nil
}

// --- Table Contents (Data Preview) ---

// TableContentsResult represents the result of a table contents query.
type TableContentsResult struct {
	Columns []TableColumn
	Rows    []map[string]interface{}
}

// TableColumn represents a column in table contents.
type TableColumn struct {
	Name        string
	Type        string
	Description string
	Length      int
	IsKey       bool
}

// GetTableContents retrieves data from a database table.
// Optional sqlQuery can be a full SELECT statement to filter/transform results
// (e.g., "SELECT * FROM T000 WHERE MANDT = '001'").
func (c *Client) GetTableContents(ctx context.Context, tableName string, maxRows int, sqlFilter string) (*TableContentsResult, error) {
	tableName = strings.ToUpper(tableName)
	if maxRows <= 0 {
		maxRows = 100
	}

	params := url.Values{}
	params.Set("rowNumber", fmt.Sprintf("%d", maxRows))
	params.Set("ddicEntityName", tableName)

	opts := &RequestOptions{
		Method: http.MethodPost,
		Query:  params,
		Accept: "application/*",
	}

	// Add SQL filter as request body if provided
	if sqlFilter != "" {
		opts.Body = []byte(sqlFilter)
		opts.ContentType = "text/plain"
	}

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/datapreview/ddic", opts)
	if err != nil {
		return nil, fmt.Errorf("getting table contents: %w", err)
	}

	return parseTableContents(resp.Body)
}

// RunQuery executes a freestyle SQL query against the SAP database.
// Example: "SELECT * FROM T000 WHERE MANDT = '001'"
func (c *Client) RunQuery(ctx context.Context, sqlQuery string, maxRows int) (*TableContentsResult, error) {
	// Safety check - free SQL can be dangerous
	if err := c.checkSafety(OpFreeSQL, "RunQuery"); err != nil {
		return nil, err
	}

	if sqlQuery == "" {
		return nil, fmt.Errorf("SQL query is required")
	}
	if maxRows <= 0 {
		maxRows = 100
	}

	params := url.Values{}
	params.Set("rowNumber", fmt.Sprintf("%d", maxRows))

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/datapreview/freestyle", &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		Accept:      "application/*",
		Body:        []byte(wrapSQL(sqlQuery)),
		ContentType: "text/plain",
	})
	if err != nil {
		return nil, fmt.Errorf("running query: %w", err)
	}

	return parseTableContents(resp.Body)
}

// wrapSQL keeps every line of a statement under the data preview's limit.
// The service puts the text into ABAP source lines of 255 characters, and a
// token cut by that wrap — a literal, a column name — is a syntax error that
// names half a word. Lines are broken at blanks outside quotes only, so the
// statement means the same thing.
func wrapSQL(query string) string {
	const limit = 200
	var out strings.Builder
	lineLen := 0
	inQuote := false
	start := 0
	emit := func(word string) {
		if word == "" {
			return
		}
		if lineLen > 0 && lineLen+1+len(word) > limit {
			out.WriteByte('\n')
			lineLen = 0
		} else if lineLen > 0 {
			out.WriteByte(' ')
			lineLen++
		}
		out.WriteString(word)
		lineLen += len(word)
	}
	for i := 0; i < len(query); i++ {
		switch query[i] {
		case '\'':
			inQuote = !inQuote
		case ' ', '\n':
			if inQuote {
				continue
			}
			emit(query[start:i])
			start = i + 1
			if query[i] == '\n' {
				out.WriteByte('\n')
				lineLen = 0
			}
		}
	}
	emit(query[start:])
	return out.String()
}

// parseTableContents parses the XML response for table contents.
func parseTableContents(data []byte) (*TableContentsResult, error) {
	// The ADT table data response is complex XML
	// We'll parse it into a generic structure
	type tableData struct {
		Columns []struct {
			Metadata struct {
				Name        string `xml:"name,attr"`
				Type        string `xml:"type,attr"`
				Description string `xml:"description,attr"`
				Length      int    `xml:"length,attr"`
				IsKey       bool   `xml:"keyAttribute,attr"`
			} `xml:"metadata"`
			DataSet struct {
				Data []string `xml:"data"`
			} `xml:"dataSet"`
		} `xml:"columns"`
	}

	var td tableData
	if err := xml.Unmarshal(data, &td); err != nil {
		return nil, fmt.Errorf("parsing table data: %w", err)
	}

	result := &TableContentsResult{
		Columns: make([]TableColumn, len(td.Columns)),
		Rows:    []map[string]interface{}{},
	}

	// Extract columns
	maxRows := 0
	for i, col := range td.Columns {
		result.Columns[i] = TableColumn{
			Name:        col.Metadata.Name,
			Type:        col.Metadata.Type,
			Description: col.Metadata.Description,
			Length:      col.Metadata.Length,
			IsKey:       col.Metadata.IsKey,
		}
		if len(col.DataSet.Data) > maxRows {
			maxRows = len(col.DataSet.Data)
		}
	}

	// Build rows
	for rowIdx := 0; rowIdx < maxRows; rowIdx++ {
		row := make(map[string]interface{})
		for _, col := range td.Columns {
			if rowIdx < len(col.DataSet.Data) {
				row[col.Metadata.Name] = col.DataSet.Data[rowIdx]
			}
		}
		result.Rows = append(result.Rows, row)
	}

	return result, nil
}

// --- Transaction Operations ---

// Transaction represents an SAP transaction.
type Transaction struct {
	Name        string
	Description string
	Program     string
}

// GetTransaction retrieves information about a transaction.
func (c *Client) GetTransaction(ctx context.Context, tcode string) (*Transaction, error) {
	tcode = strings.ToUpper(tcode)

	resp, err := c.transport.Request(ctx, fmt.Sprintf("/sap/bc/adt/vit/wb/object_type/TRAN/object_name/%s", tcode), &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting transaction: %w", err)
	}

	// Parse transaction info
	type tranInfo struct {
		Name        string `xml:"name,attr"`
		Description string `xml:"description,attr"`
		Program     string `xml:"program,attr"`
	}

	var ti tranInfo
	if err := xml.Unmarshal(resp.Body, &ti); err != nil {
		return nil, fmt.Errorf("parsing transaction: %w", err)
	}

	return &Transaction{
		Name:        ti.Name,
		Description: ti.Description,
		Program:     ti.Program,
	}, nil
}

// --- Type Info Operations ---

// TypeInfo represents type information.
type TypeInfo struct {
	Name        string
	Type        string
	Description string
	Length      int
	Decimals    int
}

// GetTypeInfo retrieves information about a data type.
func (c *Client) GetTypeInfo(ctx context.Context, typeName string) (*TypeInfo, error) {
	typeName = strings.ToUpper(typeName)

	resp, err := c.transport.Request(ctx, fmt.Sprintf("/sap/bc/adt/ddic/dataelements/%s", typeName), &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting type info: %w", err)
	}

	type typeData struct {
		Name        string `xml:"name,attr"`
		Type        string `xml:"type,attr"`
		Description string `xml:"description,attr"`
		Length      int    `xml:"length,attr"`
		Decimals    int    `xml:"decimals,attr"`
	}

	var td typeData
	if err := xml.Unmarshal(resp.Body, &td); err != nil {
		return nil, fmt.Errorf("parsing type info: %w", err)
	}

	return &TypeInfo{
		Name:        td.Name,
		Type:        td.Type,
		Description: td.Description,
		Length:      td.Length,
		Decimals:    td.Decimals,
	}, nil
}

// --- System Information Operations ---

// SystemInfo represents SAP system information.
type SystemInfo struct {
	SystemID        string `json:"systemId"`
	Client          string `json:"client"`
	SAPRelease      string `json:"sapRelease"`
	KernelRelease   string `json:"kernelRelease,omitempty"`
	DatabaseRelease string `json:"databaseRelease,omitempty"`
	DatabaseSystem  string `json:"databaseSystem,omitempty"`
	HostName        string `json:"hostName,omitempty"`
	InstallNumber   string `json:"installNumber,omitempty"`
	ABAPRelease     string `json:"abapRelease,omitempty"`
}

// GetSystemInfo retrieves SAP system information.
// Uses SQL queries to CVERS and T000 tables for reliable info across SAP versions.
func (c *Client) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	info := &SystemInfo{}

	// Helper to get string from row
	getString := func(row map[string]interface{}, key string) string {
		if v, ok := row[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}

	// Get client info from T000 - this is the primary query, propagate errors
	clientResult, err := c.RunQuery(ctx, "SELECT MANDT, MTEXT, LOGSYS FROM T000 WHERE MANDT = '"+c.config.Client+"'", 1)
	if err != nil {
		return nil, fmt.Errorf("getting system info: %w", err)
	}
	if len(clientResult.Rows) > 0 {
		row := clientResult.Rows[0]
		info.Client = getString(row, "MANDT")
		// LOGSYS format is typically <SID>CLNT<client>, e.g., A4HCLNT001
		if logsys := getString(row, "LOGSYS"); len(logsys) >= 3 {
			info.SystemID = logsys[:3] // First 3 chars are SID
		}
	}

	// Get SAP_BASIS version from CVERS (optional - don't fail if unavailable)
	basisResult, err := c.RunQuery(ctx, "SELECT RELEASE, EXTRELEASE FROM CVERS WHERE COMPONENT = 'SAP_BASIS'", 1)
	if err == nil && len(basisResult.Rows) > 0 {
		row := basisResult.Rows[0]
		info.SAPRelease = getString(row, "RELEASE")
		info.ABAPRelease = getString(row, "RELEASE")
	}

	// Try to get kernel info from CVERS (optional)
	kernelResult, err := c.RunQuery(ctx, "SELECT RELEASE FROM CVERS WHERE COMPONENT = 'SAP_ABA'", 1)
	if err == nil && len(kernelResult.Rows) > 0 {
		info.KernelRelease = getString(kernelResult.Rows[0], "RELEASE")
	}

	// Try to detect HANA from CVERS (optional)
	hanaResult, err := c.RunQuery(ctx,
		"SELECT RELEASE FROM CVERS WHERE COMPONENT LIKE '%HDB%' OR COMPONENT LIKE '%HANA%'", 1)
	if err == nil && len(hanaResult.Rows) > 0 {
		info.DatabaseSystem = "HDB"
		info.DatabaseRelease = getString(hanaResult.Rows[0], "RELEASE")
	} else {
		// Step 2: S4CORE in CVERS — pure S/4HANA.
		// S/4HANA implies HANA database. However its version cannot be inferred from the software component.
		// Therefore DatabaseRelease is left blank
		s4Result, err := c.RunQuery(ctx,
			"SELECT COMPONENT FROM CVERS WHERE COMPONENT = 'S4CORE'", 1)
		if err == nil && len(s4Result.Rows) > 0 {
			info.DatabaseSystem = "HDB"
		}
	}

	// If we couldn't get SystemID from T000, use fallback
	if info.SystemID == "" {
		info.SystemID = "???"
	}
	if info.Client == "" {
		info.Client = c.config.Client
	}

	return info, nil
}

// InstalledComponent represents an installed software component.
type InstalledComponent struct {
	Name        string `json:"name"`
	Release     string `json:"release"`
	SupportPack string `json:"supportPack,omitempty"`
	Description string `json:"description,omitempty"`
}

// GetInstalledComponents retrieves list of installed software components.
func (c *Client) GetInstalledComponents(ctx context.Context) ([]InstalledComponent, error) {
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/system/components", &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting installed components: %w", err)
	}

	type componentXML struct {
		Name        string `xml:"name,attr"`
		Release     string `xml:"release,attr"`
		SupportPack string `xml:"supportPack,attr"`
		Description string `xml:"description,attr"`
	}
	type componentsXML struct {
		XMLName    xml.Name       `xml:"components"`
		Components []componentXML `xml:"component"`
	}

	var comps componentsXML
	if err := xml.Unmarshal(resp.Body, &comps); err != nil {
		return nil, fmt.Errorf("parsing components: %w", err)
	}

	result := make([]InstalledComponent, len(comps.Components))
	for i, c := range comps.Components {
		result[i] = InstalledComponent{
			Name:        c.Name,
			Release:     c.Release,
			SupportPack: c.SupportPack,
			Description: c.Description,
		}
	}

	return result, nil
}

// --- Code Analysis Infrastructure (CAI) Operations ---

// CallGraphNode represents a node in the call graph.
type CallGraphNode struct {
	URI         string          `json:"uri"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Description string          `json:"description,omitempty"`
	Line        int             `json:"line,omitempty"`
	Column      int             `json:"column,omitempty"`
	Children    []CallGraphNode `json:"children,omitempty"`
	// Unsearched names what could not be read while building this node. A
	// child list is only as complete as the sources behind it, and an empty
	// or short one means nothing until you know whether a source was missing.
	Unsearched []Unsearched `json:"unsearched,omitempty"`
}

// CallGraphOptions configures call graph retrieval.
type CallGraphOptions struct {
	Direction string // "callers" or "callees"
	// MaxDepth is accepted and ignored. Both sources CallGraph reads are one
	// hop by construction — see the note on CallGraph in callees.go — and a
	// field that quietly does nothing is better than one that suggests a
	// traversal happened.
	MaxDepth   int
	MaxResults int // Maximum results to return
}

// The three methods that used to stand here — GetCallGraph, GetCallersOf and
// GetCalleesOf — asked /sap/bc/adt/cai/callgraph, and that resource does not
// exist. It is in the discovery document of none of 7.50, 7.57 and 7.58 and
// answers 404 "No suitable resource found" in both directions, checked with a
// CSRF token in hand so it is the resource that is missing and not the
// request. Everything built on them therefore reported that no object calls
// anything, on every system, silently — which is the worst way for a
// dependency query to be wrong.
//
// They are deleted rather than deprecated so that nothing can build on them
// again. What replaces them is in callees.go: WhereUsed for the up direction
// (the where-used list behind SE84), Callees for the down direction (the
// CROSS and WBCROSSGT cross-reference tables), and CallGraph over the two for
// callers who want the node shape below.

// CallGraphEdge represents a single edge in the call graph.
type CallGraphEdge struct {
	CallerURI  string `json:"caller_uri"`
	CallerName string `json:"caller_name"`
	CalleeURI  string `json:"callee_uri"`
	CalleeName string `json:"callee_name"`
	// CalleeKind is what the cross-reference row said the callee is — method,
	// function module, type, data. It decides whether an edge is something that
	// could execute, which a coverage figure has to know: a reference to a type
	// is not a path anything could take.
	CalleeKind string `json:"callee_kind,omitempty"`
	Line       int    `json:"line,omitempty"`
}

// IsExecutableKind reports whether a callee kind is something a run could
// actually reach. The vocabulary is wbCrossKind's and crossKind's, kept in one
// place so a coverage figure and a callee list cannot drift apart.
func IsExecutableKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "method", "function module", "report", "transaction", "subroutine", "program", "dialog module":
		return true
	default:
		return false
	}
}

// FlattenCallGraph converts a hierarchical call graph to a flat list of edges.
func FlattenCallGraph(root *CallGraphNode) []CallGraphEdge {
	var edges []CallGraphEdge
	if root == nil {
		return edges
	}

	var traverse func(parent *CallGraphNode)
	traverse = func(parent *CallGraphNode) {
		for _, child := range parent.Children {
			edges = append(edges, CallGraphEdge{
				CallerURI:  parent.URI,
				CallerName: parent.Name,
				CalleeURI:  child.URI,
				CalleeName: child.Name,
				CalleeKind: child.Type,
				Line:       child.Line,
			})
			childCopy := child
			traverse(&childCopy)
		}
	}
	traverse(root)
	return edges
}

// CallGraphStats provides statistics about a call graph.
type CallGraphStats struct {
	TotalNodes  int            `json:"total_nodes"`
	TotalEdges  int            `json:"total_edges"`
	MaxDepth    int            `json:"max_depth"`
	NodesByType map[string]int `json:"nodes_by_type"`
	UniqueNodes []string       `json:"unique_nodes"`
}

// AnalyzeCallGraph computes statistics for a call graph.
func AnalyzeCallGraph(root *CallGraphNode) *CallGraphStats {
	stats := &CallGraphStats{
		NodesByType: make(map[string]int),
	}
	if root == nil {
		return stats
	}

	seen := make(map[string]bool)
	var maxDepth int

	// Keyed by identity, not by URI. The callee side builds children from
	// cross-reference rows, which name an object but carry no ADT path, so every
	// child arrived with URI "" — and a dedup on that key folded all of them
	// into one. The result was a graph reporting two nodes beside its own list
	// of twenty-seven edges, in an answer that showed both.
	key := func(n *CallGraphNode) string {
		if n.URI != "" {
			return "uri:" + n.URI
		}
		return "name:" + n.Type + ":" + n.Name
	}

	var traverse func(node *CallGraphNode, depth int)
	traverse = func(node *CallGraphNode, depth int) {
		if depth > maxDepth {
			maxDepth = depth
		}
		if k := key(node); !seen[k] {
			seen[k] = true
			stats.TotalNodes++
			stats.NodesByType[node.Type]++
			stats.UniqueNodes = append(stats.UniqueNodes, node.Name)
		}
		for _, child := range node.Children {
			stats.TotalEdges++
			childCopy := child
			traverse(&childCopy, depth+1)
		}
	}
	traverse(root, 0)
	stats.MaxDepth = maxDepth
	return stats
}

// CallGraphComparison compares static and actual call graphs.
type CallGraphComparison struct {
	CommonEdges []CallGraphEdge `json:"common_edges"` // In both static and actual
	StaticOnly  []CallGraphEdge `json:"static_only"`  // In static but not executed
	ActualOnly  []CallGraphEdge `json:"actual_only"`  // Executed but not in static (dynamic calls)
	// CoverageRatio is executed invocations over recorded invocations, or -1
	// when nothing callable was recorded at all — which is not the same as
	// nothing having run.
	CoverageRatio float64 `json:"coverage_ratio"`
	// ExecutableEdges is how many of the static edges were invocations rather
	// than type or data references. The denominator, stated so a reader can
	// see what the ratio is of.
	ExecutableEdges int `json:"executable_edges"`
}

// CompareCallGraphs compares a static call graph with an actual execution trace.
func CompareCallGraphs(staticEdges, actualEdges []CallGraphEdge) *CallGraphComparison {
	comp := &CallGraphComparison{}

	// Build lookup sets
	staticSet := make(map[string]CallGraphEdge)
	for _, e := range staticEdges {
		key := e.CallerName + "->" + e.CalleeName
		staticSet[key] = e
	}

	actualSet := make(map[string]CallGraphEdge)
	for _, e := range actualEdges {
		key := e.CallerName + "->" + e.CalleeName
		actualSet[key] = e
	}

	// Find common and static-only
	for key, edge := range staticSet {
		if _, ok := actualSet[key]; ok {
			comp.CommonEdges = append(comp.CommonEdges, edge)
		} else {
			comp.StaticOnly = append(comp.StaticOnly, edge)
		}
	}

	// Find actual-only (dynamic calls)
	for key, edge := range actualSet {
		if _, ok := staticSet[key]; !ok {
			comp.ActualOnly = append(comp.ActualOnly, edge)
		}
	}

	// Coverage ratio
	// Only the edges something could execute count towards coverage. Most of a
	// class's static edges are type and data references — ABAP_BOOL, SYST,
	// TADIR — and counting those as paths that were never taken produced
	// figures like 0.037 for a run that exercised everything callable.
	executable := 0
	for _, e := range staticEdges {
		if IsExecutableKind(e.CalleeKind) {
			executable++
		}
	}
	comp.ExecutableEdges = executable
	commonExecutable := 0
	for _, e := range comp.CommonEdges {
		if IsExecutableKind(e.CalleeKind) {
			commonExecutable++
		}
	}
	if executable > 0 {
		comp.CoverageRatio = float64(commonExecutable) / float64(executable)
	} else if len(staticEdges) > 0 {
		// Nothing callable was recorded, so there is no coverage to report. A
		// zero here would read as "none of it ran".
		comp.CoverageRatio = -1
	}

	return comp
}

// ExtractCallEdgesFromTrace converts trace entries to call graph edges.
// It analyzes Program and Event fields to identify caller-callee relationships.
func ExtractCallEdgesFromTrace(entries []TraceEntry) []CallGraphEdge {
	var edges []CallGraphEdge
	seen := make(map[string]bool)

	// Group entries by program to detect call relationships
	var prevProgram string
	for _, entry := range entries {
		if entry.Program == "" {
			continue
		}

		// Event field contains call type info (PERFORM, CALL METHOD, etc.)
		// When program changes, we have a call edge
		if prevProgram != "" && prevProgram != entry.Program {
			edgeKey := prevProgram + "->" + entry.Program
			if !seen[edgeKey] {
				seen[edgeKey] = true
				edges = append(edges, CallGraphEdge{
					CallerURI:  "/sap/bc/adt/programs/programs/" + strings.ToLower(prevProgram),
					CallerName: prevProgram,
					CalleeURI:  "/sap/bc/adt/programs/programs/" + strings.ToLower(entry.Program),
					CalleeName: entry.Program,
					Line:       entry.Line,
				})
			}
		}
		prevProgram = entry.Program
	}

	return edges
}

// TraceExecutionResult contains the result of a traced execution.
type TraceExecutionResult struct {
	// Static call graph from code analysis
	StaticGraph *CallGraphNode `json:"static_graph,omitempty"`

	// Actual trace data from runtime
	Trace *TraceAnalysis `json:"trace,omitempty"`

	// Extracted call edges from trace
	ActualEdges []CallGraphEdge `json:"actual_edges,omitempty"`

	// Comparison between static and actual
	Comparison *CallGraphComparison `json:"comparison,omitempty"`

	// Statistics
	StaticStats *CallGraphStats `json:"static_stats,omitempty"`

	// Unsearched names the steps that did not run. Every field above is
	// omitempty, so a run where nothing worked marshals to almost nothing and
	// reads as "there was nothing to report". Comparison is the point of this
	// call, and it is absent both when static and actual agree and when the
	// trace never arrived.
	Unsearched []Unsearched `json:"unsearched,omitempty"`

	// Execution info
	ExecutedTests []string `json:"executed_tests,omitempty"`
	ExecutionTime int64    `json:"execution_time_us,omitempty"`
}

// TraceExecutionOptions configures traced execution.
type TraceExecutionOptions struct {
	// ObjectURI is the starting point for static call graph
	ObjectURI string

	// MaxDepth for static call graph traversal
	MaxDepth int

	// RunTests triggers unit tests before collecting trace
	RunTests bool

	// TestObjectURI specifies which object's tests to run
	TestObjectURI string

	// TraceUser filters traces by user (optional)
	TraceUser string
}

// TraceExecution performs a traced execution and compares actual vs static call graphs.
// This is the composite tool for RCA (Root Cause Analysis).
func (c *Client) TraceExecution(ctx context.Context, opts *TraceExecutionOptions) (*TraceExecutionResult, error) {
	result := &TraceExecutionResult{}

	// Step 1: Build static call graph (callees - what gets called from the
	// starting point). One hop, from the cross-reference tables: the recursive
	// resource this used to ask does not exist, and the comparison below only
	// needs the edges leaving the object under trace.
	if opts.ObjectURI != "" {
		staticGraph, err := c.CallGraph(ctx, opts.ObjectURI, &CallGraphOptions{
			Direction:  "callees",
			MaxResults: 500,
		})
		if err != nil {
			// Non-fatal for the run, but not for the reader: without the static
			// half there is nothing to compare the trace against, and the
			// comparison is simply absent rather than wrong.
			result.StaticGraph = nil
			result.Unsearched = append(result.Unsearched, Unsearched{
				Object: "static call graph", Reason: err.Error()})
		} else {
			result.StaticGraph = staticGraph
			result.StaticStats = AnalyzeCallGraph(staticGraph)
		}
	}

	// Step 2: Run unit tests if requested (to trigger execution)
	if opts.RunTests && opts.TestObjectURI != "" {
		testResult, err := c.RunUnitTests(ctx, opts.TestObjectURI, nil)
		if err != nil || testResult == nil {
			// The tests were asked for in order to make something run. If they
			// did not, the trace below is of whatever else happened to execute,
			// which is not what the caller asked to see.
			result.Unsearched = append(result.Unsearched, Unsearched{
				Object: "unit tests " + opts.TestObjectURI, Reason: errOrEmpty(err, "no test result came back")})
		} else {
			// Collect test names that ran
			for _, tc := range testResult.Classes {
				for _, tm := range tc.TestMethods {
					result.ExecutedTests = append(result.ExecutedTests,
						fmt.Sprintf("%s=>%s", tc.Name, tm.Name))
				}
			}
		}
	}

	// Step 3: Get latest trace for user
	traceUser := opts.TraceUser
	if traceUser == "" {
		// Use current user from config
		traceUser = c.config.Username
	}

	traces, err := c.ListTraces(ctx, &TraceQueryOptions{
		User:       traceUser,
		MaxResults: 5,
	})
	if err != nil || len(traces) == 0 {
		// No trace means no actual edges, so the comparison cannot be made.
		// Saying so is the difference between "the code ran as predicted" and
		// "nobody looked".
		result.Unsearched = append(result.Unsearched, Unsearched{
			Object: "runtime trace for " + traceUser,
			Reason: errOrEmpty(err, "no trace was recorded for this user; the code under trace may not have run")})
	} else {
		// Get the most recent trace
		latestTrace := traces[0]

		// Get hitlist analysis
		analysis, err := c.GetTrace(ctx, latestTrace.ID, "hitlist")
		if err != nil {
			result.Unsearched = append(result.Unsearched, Unsearched{
				Object: "trace " + latestTrace.ID, Reason: err.Error()})
		} else {
			result.Trace = analysis
			result.ExecutionTime = analysis.TotalTime

			// Step 4: Extract actual call edges from trace
			result.ActualEdges = ExtractCallEdgesFromTrace(analysis.Entries)

			// Step 5: Compare static vs actual if we have both
			if result.StaticGraph != nil {
				staticEdges := FlattenCallGraph(result.StaticGraph)
				result.Comparison = CompareCallGraphs(staticEdges, result.ActualEdges)
			}
		}
	}

	return result, nil
}

// ObjectExplorerNode represents a node in the object explorer tree.
type ObjectExplorerNode struct {
	URI         string               `json:"uri"`
	Name        string               `json:"name"`
	Type        string               `json:"type"`
	Description string               `json:"description,omitempty"`
	Children    []ObjectExplorerNode `json:"children,omitempty"`
}

// GetObjectStructureCAI returns an object's components as a tree.
//
// It used to ask /sap/bc/adt/cai/objectexplorer/objects, and that resource does
// not exist. Not "not on older releases" — it is advertised in the discovery
// document of none of 7.50, 7.57 or 7.58 and answers 404 on all of them, along
// with the rest of the /cai/ namespace, which also took the call graph down
// with it. So this had never returned anything, and its three callers —
// analyze type=object_structure among them — had never worked.
//
// The replacement was already in this file. /sap/bc/adt/oo/classes/{name}/
// objectstructure answers 200 with a richer document, and GetClassObjectStructure
// already spoke it. The callers are left alone deliberately: the fix belongs
// where the wrong URL was, not spread across everything that trusted it.
//
// maxResults is honoured because callers pass it, though the resource returns a
// whole class in one answer and there is nothing to page.
func (c *Client) GetObjectStructureCAI(ctx context.Context, objectName string, maxResults int) (*ObjectExplorerNode, error) {
	if maxResults <= 0 {
		maxResults = 100
	}
	structure, err := c.GetClassObjectStructure(ctx, objectName)
	if err != nil {
		return nil, err
	}
	if structure == nil {
		return nil, nil
	}

	root := &ObjectExplorerNode{
		Name: structure.Name,
		Type: structure.Type,
		URI:  "/sap/bc/adt/oo/classes/" + strings.ToLower(objectName),
	}
	for i, el := range structure.Elements {
		if i >= maxResults {
			break
		}
		root.Children = append(root.Children, ObjectExplorerNode{
			Name: el.Name,
			Type: el.Type,
		})
	}
	return root, nil
}

// objectExplorerNodeXML is used for parsing object explorer XML responses.
type objectExplorerNodeXML struct {
	URI         string                  `xml:"uri,attr"`
	Name        string                  `xml:"name,attr"`
	Type        string                  `xml:"type,attr"`
	Description string                  `xml:"description,attr"`
	Children    []objectExplorerNodeXML `xml:"object"`
}

// parseObjectExplorerResponse parses the object explorer XML response.
func parseObjectExplorerResponse(data []byte) (*ObjectExplorerNode, error) {
	type explorerXML struct {
		XMLName xml.Name                `xml:"objects"`
		Objects []objectExplorerNodeXML `xml:"object"`
	}

	var exp explorerXML
	if err := xml.Unmarshal(data, &exp); err != nil {
		return nil, fmt.Errorf("parsing object explorer: %w", err)
	}

	if len(exp.Objects) == 0 {
		return nil, nil
	}

	// Return the first object with its children
	return convertObjectExplorerNode(&exp.Objects[0]), nil
}

func convertObjectExplorerNode(n *objectExplorerNodeXML) *ObjectExplorerNode {
	if n == nil {
		return nil
	}
	node := &ObjectExplorerNode{
		URI:         n.URI,
		Name:        n.Name,
		Type:        n.Type,
		Description: n.Description,
	}
	for _, child := range n.Children {
		childCopy := child
		node.Children = append(node.Children, *convertObjectExplorerNode(&childCopy))
	}
	return node
}

// --- Short Dumps / Runtime Errors (RABAX) Operations ---
//
// The client methods that lived here — GetDumps, GetDump, DumpQueryOptions and
// the RuntimeDump/DumpDetails types — are gone, and none of them worked as
// advertised. GetDumps built an OData $filter that /sap/bc/adt/runtime/dumps
// ignores: checked on 7.58, a filter naming a user and a package that exist
// nowhere still returns every dump on the system, so everything but $top was
// decoration. Its feed parser read one Atom category and left the error type,
// the program and the user empty. GetDump asked for the dump as HTML and
// returned the whole page — 45 KB to nearly a megabyte — with only the <title>
// pulled out of it.
//
// The replacements are in this package and are structured rather than hopeful:
// Dumps + DumpFilter (dumps.go) for the listing, GroupDumps for what keeps
// failing, DumpDetail (dumpdetail.go) and DumpStack (dumpstack.go) for one
// dump, CorrelateDump (correlate.go) for the log around it.

// --- ABAP Profiler / Runtime Traces (ATRA) Operations ---

// ABAPTrace represents an ABAP runtime trace file.
type ABAPTrace struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	User        string `json:"user"`
	StartTime   string `json:"startTime"`
	EndTime     string `json:"endTime,omitempty"`
	Duration    int64  `json:"duration,omitempty"` // microseconds
	ProcessType string `json:"processType,omitempty"`
	ObjectType  string `json:"objectType,omitempty"`
	Status      string `json:"status,omitempty"`
	URI         string `json:"uri"`
}

// TraceAnalysis contains trace analysis results.
type TraceAnalysis struct {
	TraceID    string            `json:"traceId"`
	ToolType   string            `json:"toolType"` // hitlist, statements, dbAccesses
	TotalTime  int64             `json:"totalTime,omitempty"`
	TotalCalls int               `json:"totalCalls,omitempty"`
	Entries    []TraceEntry      `json:"entries,omitempty"`
	Summary    map[string]string `json:"summary,omitempty"`
}

// TraceEntry represents a single entry in trace analysis.
type TraceEntry struct {
	Program     string  `json:"program,omitempty"`
	Event       string  `json:"event,omitempty"`
	Line        int     `json:"line,omitempty"`
	GrossTime   int64   `json:"grossTime,omitempty"` // microseconds
	NetTime     int64   `json:"netTime,omitempty"`   // microseconds
	Calls       int     `json:"calls,omitempty"`
	Percentage  float64 `json:"percentage,omitempty"`
	Statement   string  `json:"statement,omitempty"`
	TableName   string  `json:"tableName,omitempty"`   // for dbAccesses
	Operation   string  `json:"operation,omitempty"`   // SELECT, INSERT, etc.
	RecordCount int     `json:"recordCount,omitempty"` // rows affected
}

// TraceQueryOptions configures the trace list query.
type TraceQueryOptions struct {
	User        string // Filter by user
	ProcessType string // Filter by process type
	ObjectType  string // Filter by object type
	MaxResults  int    // Maximum results (default 100)
}

// ListTraces retrieves a list of ABAP runtime traces.
func (c *Client) ListTraces(ctx context.Context, opts *TraceQueryOptions) ([]ABAPTrace, error) {
	if opts == nil {
		opts = &TraceQueryOptions{MaxResults: 100}
	}

	params := url.Values{}
	if opts.User != "" {
		params.Set("user", opts.User)
	}
	if opts.ProcessType != "" {
		params.Set("processType", opts.ProcessType)
	}
	if opts.ObjectType != "" {
		params.Set("objectType", opts.ObjectType)
	}
	if opts.MaxResults > 0 {
		params.Set("$top", fmt.Sprintf("%d", opts.MaxResults))
	}

	endpoint := "/sap/bc/adt/runtime/traces/abaptraces"
	if len(params) > 0 {
		endpoint = endpoint + "?" + params.Encode()
	}

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/atom+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("listing traces: %w", err)
	}

	return parseTracesFeed(resp.Body)
}

// GetTrace retrieves analysis of a specific trace.
// toolType can be: "hitlist", "statements", "dbAccesses"
func (c *Client) GetTrace(ctx context.Context, traceID string, toolType string) (*TraceAnalysis, error) {
	if toolType == "" {
		toolType = "hitlist"
	}

	endpoint := fmt.Sprintf("/sap/bc/adt/runtime/traces/abaptraces/%s/%s", traceID, toolType)

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting trace %s (%s): %w", traceID, toolType, err)
	}

	return parseTraceAnalysis(resp.Body, traceID, toolType)
}

// traceEntryXML is used for parsing trace feed entries.
type traceEntryXML struct {
	ID      string `xml:"id"`
	Title   string `xml:"title"`
	Summary string `xml:"summary"`
	Updated string `xml:"updated"`
	Link    struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Author struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Content struct {
		Trace struct {
			StartTime   string `xml:"startTime,attr"`
			EndTime     string `xml:"endTime,attr"`
			Duration    string `xml:"duration,attr"`
			ProcessType string `xml:"processType,attr"`
			ObjectType  string `xml:"objectType,attr"`
			Status      string `xml:"status,attr"`
		} `xml:"trace"`
	} `xml:"content"`
}

// parseTracesFeed parses the Atom feed of traces.
func parseTracesFeed(data []byte) ([]ABAPTrace, error) {
	type feedXML struct {
		XMLName xml.Name        `xml:"feed"`
		Entries []traceEntryXML `xml:"entry"`
	}

	var feed feedXML
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parsing traces feed: %w", err)
	}

	result := make([]ABAPTrace, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		var duration int64
		if entry.Content.Trace.Duration != "" {
			fmt.Sscanf(entry.Content.Trace.Duration, "%d", &duration)
		}

		trace := ABAPTrace{
			ID:          entry.ID,
			Title:       entry.Title,
			Description: entry.Summary,
			User:        entry.Author.Name,
			StartTime:   entry.Content.Trace.StartTime,
			EndTime:     entry.Content.Trace.EndTime,
			Duration:    duration,
			ProcessType: entry.Content.Trace.ProcessType,
			ObjectType:  entry.Content.Trace.ObjectType,
			Status:      entry.Content.Trace.Status,
			URI:         entry.Link.Href,
		}
		result = append(result, trace)
	}

	return result, nil
}

// parseTraceAnalysis parses trace analysis XML response.
func parseTraceAnalysis(data []byte, traceID, toolType string) (*TraceAnalysis, error) {
	analysis := &TraceAnalysis{
		TraceID:  traceID,
		ToolType: toolType,
		Summary:  make(map[string]string),
	}

	// The XML structure varies by tool type
	// For now, we extract basic information
	type hitlistXML struct {
		XMLName   xml.Name `xml:"hitlist"`
		TotalTime string   `xml:"totalTime,attr"`
		Entries   []struct {
			Program    string `xml:"program,attr"`
			Event      string `xml:"event,attr"`
			Line       string `xml:"line,attr"`
			GrossTime  string `xml:"grossTime,attr"`
			NetTime    string `xml:"netTime,attr"`
			Calls      string `xml:"calls,attr"`
			Percentage string `xml:"percentage,attr"`
		} `xml:"entry"`
	}

	var hitlist hitlistXML
	if err := xml.Unmarshal(data, &hitlist); err == nil && hitlist.XMLName.Local == "hitlist" {
		if hitlist.TotalTime != "" {
			fmt.Sscanf(hitlist.TotalTime, "%d", &analysis.TotalTime)
		}

		for _, e := range hitlist.Entries {
			var line, calls int
			var grossTime, netTime int64
			var percentage float64

			fmt.Sscanf(e.Line, "%d", &line)
			fmt.Sscanf(e.Calls, "%d", &calls)
			fmt.Sscanf(e.GrossTime, "%d", &grossTime)
			fmt.Sscanf(e.NetTime, "%d", &netTime)
			fmt.Sscanf(e.Percentage, "%f", &percentage)

			analysis.Entries = append(analysis.Entries, TraceEntry{
				Program:    e.Program,
				Event:      e.Event,
				Line:       line,
				GrossTime:  grossTime,
				NetTime:    netTime,
				Calls:      calls,
				Percentage: percentage,
			})
			analysis.TotalCalls += calls
		}
	}

	return analysis, nil
}

// --- SQL Trace (ST05) Operations ---

// The ST05 model, rewritten against what the server sends.
//
// Everything here was previously modelled from a guess and never corrected,
// because the request carried a concrete Accept that this resource answers with
// 406. Both calls failed on every invocation they had ever made, so no response
// was ever parsed and no parser was ever contradicted. Fixing the Accept made
// the requests succeed and the parsers wrong in the same minute.
//
// What the resource actually returns is broader than "is SQL trace on": a row
// per application server instance, each carrying eight independent trace types.
// SQL is one of them.

// SQLTraceState is the trace state of one application server instance.
type SQLTraceState struct {
	Instance   string `json:"instance"`
	Host       string `json:"host,omitempty"`
	IsLocal    bool   `json:"isLocal"`
	IsSelected bool   `json:"isSelected"`
	// ModifiedBy and ModifiedAt are who last changed this instance's trace
	// settings, which is the only "when" the resource offers — there is no
	// start time for a running trace.
	ModifiedBy string `json:"modifiedBy,omitempty"`
	ModifiedAt string `json:"modifiedAt,omitempty"`

	// Types is the eight switches, by their own names. A map rather than eight
	// fields because callers ask "is anything on" far more often than they ask
	// about one, and because the set has grown before.
	Types map[string]bool `json:"types"`

	// Filter narrows what is recorded. Empty strings mean unfiltered, which is
	// the ordinary state and is why they are omitted.
	TraceUser       string `json:"traceUser,omitempty"`
	TransactionCode string `json:"transactionCode,omitempty"`
	Program         string `json:"program,omitempty"`
	RFCFunction     string `json:"rfcFunction,omitempty"`
	URL             string `json:"url,omitempty"`
	WorkProcessID   string `json:"workProcessId,omitempty"`

	StackTrace     bool `json:"stackTrace"`
	AuthErrorsOnly bool `json:"authErrorsOnly"`
}

// Active reports whether any trace type is switched on for this instance.
func (s *SQLTraceState) Active() bool {
	for _, on := range s.Types {
		if on {
			return true
		}
	}
	return false
}

// SQLTraceOn reports whether the SQL trace specifically is on.
func (s *SQLTraceState) SQLTraceOn() bool { return s.Types["sql"] }

// SQLTraceDirectory is what the trace directory resource returns.
//
// On 7.58 it returns no entries at all: one URI, pointing at the STMC web
// application where the traces are read. Modelling it as a list of files and
// returning an empty one would say "this system has no traces", which is a
// different statement and not one this resource makes.
type SQLTraceDirectory struct {
	// Entries is what the resource lists, where it lists anything.
	Entries []SQLTraceEntry `json:"entries"`
	// AnalysisURL is where the release directs a reader instead. When it is set
	// and Entries is empty, the absence of entries is a property of the
	// resource and not of the system.
	AnalysisURL string `json:"analysisUrl,omitempty"`
}

// SQLTraceEntry represents a trace file in the directory.
type SQLTraceEntry struct {
	ID          string `json:"id"`
	User        string `json:"user"`
	StartTime   string `json:"startTime"`
	EndTime     string `json:"endTime,omitempty"`
	TraceType   string `json:"traceType"`
	RecordCount int    `json:"recordCount"`
	Size        int64  `json:"size,omitempty"`
	URI         string `json:"uri"`
}

// GetSQLTraceState returns the trace state of every application server
// instance this system reports.
func (c *Client) GetSQLTraceState(ctx context.Context) ([]SQLTraceState, error) {
	// Measured on 7.58: this resource answers */* and refuses everything else
	// with 406 — application/xml, text/plain, and both spellings of the vendor
	// type it might plausibly want. The concrete type here made GetSQLTraceState
	// fail on every call it had ever made. Third place this week; the debugger
	// and the RFC tunnel were the first two.
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/st05/trace/state", &RequestOptions{
		Method: http.MethodGet,
		Accept: "*/*",
	})
	if err != nil {
		return nil, fmt.Errorf("getting SQL trace state: %w", err)
	}

	return parseSQLTraceState(resp.Body)
}

// ListSQLTraces retrieves a list of SQL trace files.
func (c *Client) ListSQLTraces(ctx context.Context, user string, maxResults int) (*SQLTraceDirectory, error) {
	params := url.Values{}
	if user != "" {
		params.Set("user", user)
	}
	if maxResults > 0 {
		params.Set("$top", fmt.Sprintf("%d", maxResults))
	}

	endpoint := "/sap/bc/adt/st05/trace/directory"
	if len(params) > 0 {
		endpoint = endpoint + "?" + params.Encode()
	}

	// Same measurement, same answer: the trace directory refuses
	// application/atom+xml with a 406 and answers */*. Both ST05 resources were
	// unreachable, and only one of them was being probed.
	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method: http.MethodGet,
		Accept: "*/*",
	})
	if err != nil {
		return nil, fmt.Errorf("listing SQL traces: %w", err)
	}

	return parseSQLTraceDirectory(resp.Body)
}

// parseSQLTraceState reads the instance table the resource actually sends.
func parseSQLTraceState(data []byte) ([]SQLTraceState, error) {
	type flags struct {
		SQL  string `xml:"sqlOn"`
		Buf  string `xml:"bufOn"`
		Enq  string `xml:"enqOn"`
		RFC  string `xml:"rfcOn"`
		HTTP string `xml:"httpOn"`
		APC  string `xml:"apcOn"`
		AMC  string `xml:"amcOn"`
		Auth string `xml:"authOn"`
	}
	type props struct {
		AuthErrorsOnly string `xml:"authErrorsOnly"`
		StackTraceOn   string `xml:"stackTraceOn"`
	}
	type filter struct {
		TraceUser       string `xml:"traceUser"`
		TransactionCode string `xml:"transactionCode"`
		Program         string `xml:"program"`
		RFCFunction     string `xml:"rfcFunction"`
		URL             string `xml:"url"`
		WpID            string `xml:"wpId"`
	}
	type instance struct {
		Instance   string `xml:"instance"`
		Host       string `xml:"host"`
		IsLocal    string `xml:"isLocal"`
		IsSelected string `xml:"isSelected"`
		ModUser    string `xml:"modificationUser"`
		ModAt      string `xml:"modificationDateTime"`
		Types      flags  `xml:"traceTypes"`
		Props      props  `xml:"traceProperties"`
		Filter     filter `xml:"traceFilter"`
	}
	var table struct {
		XMLName   xml.Name   `xml:"traceStateInstanceTable"`
		Instances []instance `xml:"traceStateInstance"`
	}
	if err := xml.Unmarshal(data, &table); err != nil {
		return nil, fmt.Errorf("parsing SQL trace state: %w", err)
	}

	out := make([]SQLTraceState, 0, len(table.Instances))
	for _, in := range table.Instances {
		out = append(out, SQLTraceState{
			Instance:   strings.TrimSpace(in.Instance),
			Host:       strings.TrimSpace(in.Host),
			IsLocal:    xmlBool(in.IsLocal),
			IsSelected: xmlBool(in.IsSelected),
			ModifiedBy: strings.TrimSpace(in.ModUser),
			ModifiedAt: strings.TrimSpace(in.ModAt),
			Types: map[string]bool{
				"sql": xmlBool(in.Types.SQL), "buffer": xmlBool(in.Types.Buf),
				"enqueue": xmlBool(in.Types.Enq), "rfc": xmlBool(in.Types.RFC),
				"http": xmlBool(in.Types.HTTP), "apc": xmlBool(in.Types.APC),
				"amc": xmlBool(in.Types.AMC), "authorization": xmlBool(in.Types.Auth),
			},
			TraceUser:       strings.TrimSpace(in.Filter.TraceUser),
			TransactionCode: strings.TrimSpace(in.Filter.TransactionCode),
			Program:         strings.TrimSpace(in.Filter.Program),
			RFCFunction:     strings.TrimSpace(in.Filter.RFCFunction),
			URL:             strings.TrimSpace(in.Filter.URL),
			WorkProcessID:   strings.TrimSpace(in.Filter.WpID),
			StackTrace:      xmlBool(in.Props.StackTraceOn),
			AuthErrorsOnly:  xmlBool(in.Props.AuthErrorsOnly),
		})
	}
	return out, nil
}

// xmlBool reads the several ways SAP writes a flag.
func xmlBool(v string) bool {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "TRUE", "X", "1":
		return true
	}
	return false
}

// sqlTraceEntryXML is one entry of the Atom-feed shape.
type sqlTraceEntryXML struct {
	ID      string `xml:"id"`
	Title   string `xml:"title"`
	Updated string `xml:"updated"`
	Link    struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Author struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Content struct {
		Trace struct {
			TraceType   string `xml:"traceType,attr"`
			StartTime   string `xml:"startTime,attr"`
			EndTime     string `xml:"endTime,attr"`
			RecordCount string `xml:"recordCount,attr"`
			Size        string `xml:"size,attr"`
		} `xml:"trace"`
	} `xml:"content"`
}

// parseSQLTraceDirectory reads the directory document.
//
// Two shapes, and which one arrives is a property of the release. Older systems
// answer with an Atom feed of trace files. 7.58 answers with a single URI
// pointing at the STMC web application, and lists nothing at all — the traces
// are there, they are simply not offered through this resource.
//
// The distinction is the whole reason this returns a struct and not a slice. An
// empty slice from here would read as "this system has no traces", which is a
// claim about the system. What is true is a claim about the resource, and the
// caller is handed the URL so the answer is still useful.
func parseSQLTraceDirectory(data []byte) (*SQLTraceDirectory, error) {
	var feed struct {
		XMLName xml.Name           `xml:"feed"`
		Entries []sqlTraceEntryXML `xml:"entry"`
	}
	if err := xml.Unmarshal(data, &feed); err == nil {
		out := &SQLTraceDirectory{Entries: make([]SQLTraceEntry, 0, len(feed.Entries))}
		for _, entry := range feed.Entries {
			var recordCount int
			var size int64
			fmt.Sscanf(entry.Content.Trace.RecordCount, "%d", &recordCount)
			fmt.Sscanf(entry.Content.Trace.Size, "%d", &size)
			out.Entries = append(out.Entries, SQLTraceEntry{
				ID:          entry.ID,
				User:        entry.Author.Name,
				StartTime:   entry.Content.Trace.StartTime,
				EndTime:     entry.Content.Trace.EndTime,
				TraceType:   entry.Content.Trace.TraceType,
				RecordCount: recordCount,
				Size:        size,
				URI:         entry.Link.Href,
			})
		}
		return out, nil
	}

	var dir struct {
		XMLName xml.Name `xml:"traceDirectory"`
		URI     string   `xml:"uri"`
	}
	if err := xml.Unmarshal(data, &dir); err != nil {
		return nil, fmt.Errorf("parsing SQL trace directory: %w", err)
	}
	return &SQLTraceDirectory{
		Entries:     []SQLTraceEntry{},
		AnalysisURL: strings.TrimSpace(dir.URI),
	}, nil
}

// --- API Release State (Clean Core) ---

// GetAPIReleaseState retrieves the API release state for an ABAP object.
// This checks whether the object is released for use in ABAP Cloud (S/4HANA Clean Core).
func (c *Client) GetAPIReleaseState(ctx context.Context, objectURI string) (*APIReleaseState, error) {
	// objectURI is the full ADT path of the OBJECT, e.g. "/sap/bc/adt/oo/classes/cl_abap_typedescr".
	// We escape it to attach it to the endpoint path.
	endpoint := fmt.Sprintf("/sap/bc/adt/apireleases/%s", url.PathEscape(objectURI))

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.apirelease.v10+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting API release state: %w", err)
	}

	body := strings.TrimSpace(string(resp.Body))

	if u, err := strconv.Unquote(body); err == nil {
		body = u
	}

	if strings.Contains(body, "&lt;") {
		body = html.UnescapeString(body)
	}

	var state APIReleaseState
	if err := xml.Unmarshal([]byte(body), &state); err != nil {
		return nil, fmt.Errorf("unmarshal API release state: %w", err)
	}

	return &state, nil
}
