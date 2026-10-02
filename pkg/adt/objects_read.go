package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
)

// --- Program Operations ---

// GetProgram retrieves the source code of an ABAP program.
// Supports namespaced programs like /UI5/UI5_REPOSITORY_LOAD.
func (c *Client) GetProgram(ctx context.Context, programName string) (string, error) {
	src, _, err := c.getProgram(ctx, programName)
	return src, err
}

// getProgram is GetProgram that also says which ADT source served the text:
// the program's, or, after the include fallback below, the include's.
func (c *Client) getProgram(ctx context.Context, programName string) (string, string, error) {
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
				return src, fmt.Sprintf("/sap/bc/adt/programs/includes/%s/source/main", url.PathEscape(programName)), nil
			}
		}
		return "", "", fmt.Errorf("getting program source: %w", err)
	}

	return string(resp.Body), sourcePath, nil
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
	// The name may arrive already escaped from a URL; normalize to the raw
	// name so it is escaped exactly once below.
	className = strings.ToUpper(unescapeObjectName(className))

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
	// The name may arrive already escaped from a URL; normalize to the raw
	// name so it is escaped exactly once below.
	className = strings.ToUpper(unescapeObjectName(className))

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
