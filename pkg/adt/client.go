package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Client is the main ADT API client.
type Client struct {
	transport        *Transport
	config           *Config
	discoveryCache   *Discovery
	discoveryCacheMu sync.RWMutex
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

// checkSafety checks if an operation is allowed by the safety configuration.
func (c *Client) checkSafety(op OperationType, opName string) error {
	return c.config.Safety.CheckOperation(op, opName)
}

// checkPackageSafety checks if operations on a package are allowed.
func (c *Client) checkPackageSafety(pkg string) error {
	return c.config.Safety.CheckPackage(pkg)
}

// Safety returns the safety configuration for checking transport operations.
func (c *Client) Safety() *SafetyConfig {
	return &c.config.Safety
}

// --- Search Operations ---

// SearchObject searches for ABAP objects by name pattern.
// The query parameter supports wildcards (* for multiple chars, ? for single char).
func (c *Client) SearchObject(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 100
	}

	params := url.Values{}
	params.Set("operation", "quickSearch")
	params.Set("query", query)
	params.Set("maxResults", fmt.Sprintf("%d", maxResults))

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

// --- Function Module Operations ---

// GetFunctionGroup retrieves the structure of a function group.
// Supports namespaced function groups like /UI5/UI5_REPOSITORY_LOAD.
func (c *Client) GetFunctionGroup(ctx context.Context, groupName string) (*FunctionGroup, error) {
	groupName = strings.ToUpper(groupName)

	// URL encode for namespaced objects
	structPath := fmt.Sprintf("/sap/bc/adt/functions/groups/%s", url.PathEscape(groupName))
	resp, err := c.transport.Request(ctx, structPath, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.functions.groups.v3+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting function group: %w", err)
	}

	var fg FunctionGroup
	if err := xml.Unmarshal(resp.Body, &fg); err != nil {
		return nil, fmt.Errorf("parsing function group: %w", err)
	}

	return &fg, nil
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

// --- RAP Object Operations (SRVB) ---

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
	xmlStr := StripXMLNamespaces(string(data), "srvb:", "adtcore:")

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

// PackageQueryOptions configures GetPackage behavior.
type PackageQueryOptions struct {
	MaxObjects int // Limit returned objects (default 100, 0 = unlimited)
	Offset     int // Skip first N objects (for pagination)
}

// GetPackage retrieves package contents with optional pagination.
func (c *Client) GetPackage(ctx context.Context, packageName string, opts *PackageQueryOptions) (*PackageContent, error) {
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

	// Apply defaults
	limit := 100
	offset := 0
	if opts != nil {
		if opts.MaxObjects > 0 {
			limit = opts.MaxObjects
		} else if opts.MaxObjects < 0 {
			limit = 0 // 0 = unlimited
		}
		offset = opts.Offset
	}

	// Parse the nodestructure response
	return parsePackageNodeStructure(resp.Body, packageName, limit, offset)
}

// parsePackageNodeStructure parses the nodestructure XML response into PackageContent.
func parsePackageNodeStructure(data []byte, packageName string, maxObjects, offset int) (*PackageContent, error) {
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

	// Count all objects, apply offset and limit
	objectCount := 0
	skipped := 0
	for _, node := range resp.Values.Data.TreeContent.Nodes {
		if node.ObjectName == "" {
			continue
		}
		if node.ObjectType == "DEVC/K" {
			pkg.SubPackages = append(pkg.SubPackages, node.ObjectName)
		} else {
			objectCount++
			// Apply offset
			if skipped < offset {
				skipped++
				continue
			}
			// Apply limit (0 = unlimited)
			if maxObjects == 0 || len(pkg.Objects) < maxObjects {
				pkg.Objects = append(pkg.Objects, PackageObject{
					Type:        node.ObjectType,
					Name:        node.ObjectName,
					URI:         node.ObjectURI,
					Description: node.Desc,
				})
			}
		}
	}

	pkg.TotalObjects = objectCount
	pkg.Truncated = maxObjects > 0 && (objectCount-offset) > maxObjects

	return pkg, nil
}
