package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Client is the main ADT API client.
type Client struct {
	transport *Transport
	config    *Config
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

// checkTransportableEdit checks if editing objects that require transports is allowed.
func (c *Client) checkTransportableEdit(transport, opName string) error {
	return c.config.Safety.CheckTransportableEdit(transport, opName)
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
		return "", fmt.Errorf("getting program source: %w", err)
	}

	return string(resp.Body), nil
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
	resp, err := c.transport.Request(ctx, structPath, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
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
	Name            string `json:"name"`
	Type            string `json:"type"`
	Description     string `json:"description"`
	Published       bool   `json:"published"`
	BindingType     string `json:"bindingType"`     // ODATA
	BindingVersion  string `json:"bindingVersion"`  // V2, V4
	ServiceURL      string `json:"serviceUrl,omitempty"`
	ServiceDefName  string `json:"serviceDefName,omitempty"`
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
		Name:            root.Name,
		Type:            root.Type,
		Description:     root.Description,
		Published:       root.Published,
		BindingType:     root.Binding.Type,
		BindingVersion:  root.Binding.Version,
		ServiceDefName:  root.Services.Content.ServiceDef.Name,
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
		Body:        []byte(sqlQuery),
		ContentType: "text/plain",
	})
	if err != nil {
		return nil, fmt.Errorf("running query: %w", err)
	}

	return parseTableContents(resp.Body)
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
	hanaResult, err := c.RunQuery(ctx, "SELECT RELEASE FROM CVERS WHERE COMPONENT LIKE '%HDB%' OR COMPONENT LIKE '%HANA%'", 1)
	if err == nil && len(hanaResult.Rows) > 0 {
		info.DatabaseSystem = "HDB"
		info.DatabaseRelease = getString(hanaResult.Rows[0], "RELEASE")
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
}

// CallGraphOptions configures call graph retrieval.
type CallGraphOptions struct {
	Direction  string // "callers" or "callees"
	MaxDepth   int    // Maximum depth to traverse
	MaxResults int    // Maximum results to return
}

// GetCallGraph retrieves the call graph for an ABAP object.
// Direction can be "callers" (who calls this) or "callees" (what this calls).
func (c *Client) GetCallGraph(ctx context.Context, objectURI string, opts *CallGraphOptions) (*CallGraphNode, error) {
	if opts == nil {
		opts = &CallGraphOptions{
			Direction:  "callees",
			MaxDepth:   3,
			MaxResults: 100,
		}
	}

	params := url.Values{}
	if opts.Direction != "" {
		params.Set("direction", opts.Direction)
	}
	if opts.MaxDepth > 0 {
		params.Set("maxDepth", fmt.Sprintf("%d", opts.MaxDepth))
	}
	if opts.MaxResults > 0 {
		params.Set("maxResults", fmt.Sprintf("%d", opts.MaxResults))
	}

	// Build request body with object URI
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<cai:callGraphRequest xmlns:cai="http://www.sap.com/adt/cai">
  <cai:objectUri>%s</cai:objectUri>
</cai:callGraphRequest>`, objectURI)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/cai/callgraph", &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		Accept:      "application/xml",
		ContentType: "application/xml",
		Body:        []byte(body),
	})
	if err != nil {
		return nil, fmt.Errorf("getting call graph: %w", err)
	}

	return parseCallGraphResponse(resp.Body)
}

// callGraphNodeXML is used for parsing call graph XML responses.
type callGraphNodeXML struct {
	URI         string             `xml:"uri,attr"`
	Name        string             `xml:"name,attr"`
	Type        string             `xml:"type,attr"`
	Description string             `xml:"description,attr"`
	Line        int                `xml:"line,attr"`
	Column      int                `xml:"column,attr"`
	Children    []callGraphNodeXML `xml:"node"`
}

// parseCallGraphResponse parses the call graph XML response.
func parseCallGraphResponse(data []byte) (*CallGraphNode, error) {
	type callGraphXML struct {
		XMLName xml.Name         `xml:"callGraph"`
		Root    callGraphNodeXML `xml:"node"`
	}

	var cg callGraphXML
	if err := xml.Unmarshal(data, &cg); err != nil {
		return nil, fmt.Errorf("parsing call graph: %w", err)
	}

	return convertCallGraphNode(&cg.Root), nil
}

func convertCallGraphNode(n *callGraphNodeXML) *CallGraphNode {
	if n == nil {
		return nil
	}
	node := &CallGraphNode{
		URI:         n.URI,
		Name:        n.Name,
		Type:        n.Type,
		Description: n.Description,
		Line:        n.Line,
		Column:      n.Column,
	}
	for _, child := range n.Children {
		childCopy := child
		node.Children = append(node.Children, *convertCallGraphNode(&childCopy))
	}
	return node
}

// GetCallersOf returns who calls the specified object (up traversal).
// This is a convenience wrapper around GetCallGraph with direction="callers".
func (c *Client) GetCallersOf(ctx context.Context, objectURI string, maxDepth int) (*CallGraphNode, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}
	return c.GetCallGraph(ctx, objectURI, &CallGraphOptions{
		Direction:  "callers",
		MaxDepth:   maxDepth,
		MaxResults: 500,
	})
}

// GetCalleesOf returns what the specified object calls (down traversal).
// This is a convenience wrapper around GetCallGraph with direction="callees".
func (c *Client) GetCalleesOf(ctx context.Context, objectURI string, maxDepth int) (*CallGraphNode, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}
	return c.GetCallGraph(ctx, objectURI, &CallGraphOptions{
		Direction:  "callees",
		MaxDepth:   maxDepth,
		MaxResults: 500,
	})
}

// CallGraphEdge represents a single edge in the call graph.
type CallGraphEdge struct {
	CallerURI  string `json:"caller_uri"`
	CallerName string `json:"caller_name"`
	CalleeURI  string `json:"callee_uri"`
	CalleeName string `json:"callee_name"`
	Line       int    `json:"line,omitempty"`
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
	TotalNodes   int            `json:"total_nodes"`
	TotalEdges   int            `json:"total_edges"`
	MaxDepth     int            `json:"max_depth"`
	NodesByType  map[string]int `json:"nodes_by_type"`
	UniqueNodes  []string       `json:"unique_nodes"`
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

	var traverse func(node *CallGraphNode, depth int)
	traverse = func(node *CallGraphNode, depth int) {
		if depth > maxDepth {
			maxDepth = depth
		}
		if !seen[node.URI] {
			seen[node.URI] = true
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
	CommonEdges    []CallGraphEdge `json:"common_edges"`    // In both static and actual
	StaticOnly     []CallGraphEdge `json:"static_only"`     // In static but not executed
	ActualOnly     []CallGraphEdge `json:"actual_only"`     // Executed but not in static (dynamic calls)
	CoverageRatio  float64         `json:"coverage_ratio"`  // Actual/Static ratio
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
	if len(staticEdges) > 0 {
		comp.CoverageRatio = float64(len(comp.CommonEdges)) / float64(len(staticEdges))
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

	// Step 1: Build static call graph (callees - what gets called from the starting point)
	if opts.ObjectURI != "" {
		depth := opts.MaxDepth
		if depth <= 0 {
			depth = 5
		}

		staticGraph, err := c.GetCalleesOf(ctx, opts.ObjectURI, depth)
		if err != nil {
			// Non-fatal: continue without static graph
			result.StaticGraph = nil
		} else {
			result.StaticGraph = staticGraph
			result.StaticStats = AnalyzeCallGraph(staticGraph)
		}
	}

	// Step 2: Run unit tests if requested (to trigger execution)
	if opts.RunTests && opts.TestObjectURI != "" {
		testResult, err := c.RunUnitTests(ctx, opts.TestObjectURI, nil)
		if err == nil && testResult != nil {
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
	if err == nil && len(traces) > 0 {
		// Get the most recent trace
		latestTrace := traces[0]

		// Get hitlist analysis
		analysis, err := c.GetTrace(ctx, latestTrace.ID, "hitlist")
		if err == nil {
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

// GetObjectStructureCAI retrieves the object structure from Code Analysis Infrastructure.
// This provides a hierarchical view of the object's components (methods, attributes, etc).
func (c *Client) GetObjectStructureCAI(ctx context.Context, objectName string, maxResults int) (*ObjectExplorerNode, error) {
	if maxResults <= 0 {
		maxResults = 100
	}

	params := url.Values{}
	params.Set("objectName", objectName)
	params.Set("maxResults", fmt.Sprintf("%d", maxResults))

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/cai/objectexplorer/objects", &RequestOptions{
		Method: http.MethodGet,
		Query:  params,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting object structure: %w", err)
	}

	return parseObjectExplorerResponse(resp.Body)
}

// GetObjectChildren retrieves the children of an object in the explorer tree.
func (c *Client) GetObjectChildren(ctx context.Context, fullname string, childType string) ([]ObjectExplorerNode, error) {
	path := fmt.Sprintf("/sap/bc/adt/cai/objectexplorer/%s/children", url.PathEscape(fullname))

	params := url.Values{}
	if childType != "" {
		params.Set("type", childType)
	}

	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method: http.MethodGet,
		Query:  params,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting object children: %w", err)
	}

	type childXML struct {
		URI         string `xml:"uri,attr"`
		Name        string `xml:"name,attr"`
		Type        string `xml:"type,attr"`
		Description string `xml:"description,attr"`
	}
	type childrenXML struct {
		XMLName  xml.Name   `xml:"children"`
		Children []childXML `xml:"child"`
	}

	var children childrenXML
	if err := xml.Unmarshal(resp.Body, &children); err != nil {
		return nil, fmt.Errorf("parsing children: %w", err)
	}

	result := make([]ObjectExplorerNode, len(children.Children))
	for i, ch := range children.Children {
		result[i] = ObjectExplorerNode{
			URI:         ch.URI,
			Name:        ch.Name,
			Type:        ch.Type,
			Description: ch.Description,
		}
	}

	return result, nil
}

// GetObjectEntryPoints retrieves the entry points for an object.
func (c *Client) GetObjectEntryPoints(ctx context.Context, fullname string) ([]ObjectExplorerNode, error) {
	path := fmt.Sprintf("/sap/bc/adt/cai/objectexplorer/%s/entrypoints", url.PathEscape(fullname))

	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting entry points: %w", err)
	}

	type entryXML struct {
		URI         string `xml:"uri,attr"`
		Name        string `xml:"name,attr"`
		Type        string `xml:"type,attr"`
		Description string `xml:"description,attr"`
	}
	type entriesXML struct {
		XMLName xml.Name   `xml:"entrypoints"`
		Entries []entryXML `xml:"entrypoint"`
	}

	var entries entriesXML
	if err := xml.Unmarshal(resp.Body, &entries); err != nil {
		return nil, fmt.Errorf("parsing entry points: %w", err)
	}

	result := make([]ObjectExplorerNode, len(entries.Entries))
	for i, e := range entries.Entries {
		result[i] = ObjectExplorerNode{
			URI:         e.URI,
			Name:        e.Name,
			Type:        e.Type,
			Description: e.Description,
		}
	}

	return result, nil
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

// RuntimeDump represents an ABAP runtime error (short dump).
type RuntimeDump struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Category      string `json:"category"`
	ExceptionType string `json:"exceptionType"`
	Program       string `json:"program"`
	Include       string `json:"include,omitempty"`
	Line          int    `json:"line,omitempty"`
	User          string `json:"user"`
	Client        string `json:"client"`
	Host          string `json:"host,omitempty"`
	Timestamp     string `json:"timestamp"`
	URI           string `json:"uri"`
}

// DumpDetails contains full details of a runtime error.
type DumpDetails struct {
	RuntimeDump
	StackTrace   []StackFrame      `json:"stackTrace,omitempty"`
	Variables    []DumpVariable    `json:"variables,omitempty"`
	SourceCode   string            `json:"sourceCode,omitempty"`
	ErrorDetails map[string]string `json:"errorDetails,omitempty"`
	RawHTML      string            `json:"rawHtml,omitempty"`
}

// StackFrame represents a single frame in the stack trace.
type StackFrame struct {
	Program string `json:"program"`
	Include string `json:"include,omitempty"`
	Line    int    `json:"line"`
	Event   string `json:"event,omitempty"`
}

// DumpVariable represents a variable captured in the dump.
type DumpVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  string `json:"type,omitempty"`
}

// DumpQueryOptions configures the dump list query.
type DumpQueryOptions struct {
	User          string // Filter by user
	ExceptionType string // Filter by exception type (e.g., "CX_SY_ZERODIVIDE")
	Program       string // Filter by program name
	Package       string // Filter by package
	DateFrom      string // Date from (YYYYMMDD format)
	DateTo        string // Date to (YYYYMMDD format)
	MaxResults    int    // Maximum results (default 100)
}

// GetDumps retrieves a list of runtime errors (short dumps) from the SAP system.
func (c *Client) GetDumps(ctx context.Context, opts *DumpQueryOptions) ([]RuntimeDump, error) {
	if opts == nil {
		opts = &DumpQueryOptions{MaxResults: 100}
	}

	params := url.Values{}

	// Build FQL (Filter Query Language) filter
	var filters []string
	if opts.User != "" {
		filters = append(filters, fmt.Sprintf("user eq '%s'", opts.User))
	}
	if opts.ExceptionType != "" {
		filters = append(filters, fmt.Sprintf("exceptionType eq '%s'", opts.ExceptionType))
	}
	if opts.Program != "" {
		filters = append(filters, fmt.Sprintf("program eq '%s'", opts.Program))
	}
	if opts.Package != "" {
		filters = append(filters, fmt.Sprintf("package eq '%s'", opts.Package))
	}
	if opts.DateFrom != "" {
		filters = append(filters, fmt.Sprintf("datetime ge '%s000000'", opts.DateFrom))
	}
	if opts.DateTo != "" {
		filters = append(filters, fmt.Sprintf("datetime le '%s235959'", opts.DateTo))
	}

	if len(filters) > 0 {
		params.Set("$filter", strings.Join(filters, " and "))
	}

	if opts.MaxResults > 0 {
		params.Set("$top", fmt.Sprintf("%d", opts.MaxResults))
	}

	endpoint := "/sap/bc/adt/runtime/dumps"
	if len(params) > 0 {
		endpoint = endpoint + "?" + params.Encode()
	}

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/atom+xml;type=feed",
	})
	if err != nil {
		return nil, fmt.Errorf("getting dumps: %w", err)
	}

	return parseDumpsFeed(resp.Body)
}

// GetDump retrieves full details of a specific runtime error.
// dumpID can be either a full URI path (from GetDumps) or just the dump identifier.
func (c *Client) GetDump(ctx context.Context, dumpID string) (*DumpDetails, error) {
	var endpoint string
	// GetDumps returns IDs like /sap/bc/adt/vit/runtime/dumps/{id}
	// But the detail endpoint is /sap/bc/adt/runtime/dump/{id} (singular, no vit)
	if strings.Contains(dumpID, "/runtime/dumps/") {
		// Extract ID from full path and use correct endpoint
		parts := strings.Split(dumpID, "/runtime/dumps/")
		if len(parts) == 2 {
			endpoint = fmt.Sprintf("/sap/bc/adt/runtime/dump/%s", parts[1])
		} else {
			endpoint = fmt.Sprintf("/sap/bc/adt/runtime/dump/%s", dumpID)
		}
	} else if strings.HasPrefix(dumpID, "/sap/bc/adt/runtime/dump/") {
		// Already correct path
		endpoint = dumpID
	} else {
		// Just the ID - construct path
		endpoint = fmt.Sprintf("/sap/bc/adt/runtime/dump/%s", dumpID)
	}

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method: http.MethodGet,
		Accept: "text/html",
	})
	if err != nil {
		return nil, fmt.Errorf("getting dump %s: %w", dumpID, err)
	}

	return parseDumpDetails(resp.Body, dumpID)
}

// dumpEntryXML is used for parsing dump feed entries.
type dumpEntryXML struct {
	ID        string `xml:"id"`
	Title     string `xml:"title"`
	Updated   string `xml:"updated"`
	Category  struct {
		Term string `xml:"term,attr"`
	} `xml:"category"`
	Link struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Content struct {
		Type   string `xml:"type,attr"`
		Source struct {
			Line          string `xml:"line,attr"`
			Program       string `xml:"program,attr"`
			Include       string `xml:"include,attr"`
		} `xml:"source"`
		Exception struct {
			Type string `xml:"type,attr"`
		} `xml:"exception"`
		Runtime struct {
			User   string `xml:"user,attr"`
			Client string `xml:"client,attr"`
			Host   string `xml:"host,attr"`
		} `xml:"runtime"`
	} `xml:"content"`
}

// parseDumpsFeed parses the Atom feed of runtime errors.
func parseDumpsFeed(data []byte) ([]RuntimeDump, error) {
	type feedXML struct {
		XMLName xml.Name       `xml:"feed"`
		Entries []dumpEntryXML `xml:"entry"`
	}

	var feed feedXML
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parsing dumps feed: %w", err)
	}

	result := make([]RuntimeDump, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		line := 0
		if entry.Content.Source.Line != "" {
			fmt.Sscanf(entry.Content.Source.Line, "%d", &line)
		}

		dump := RuntimeDump{
			ID:            entry.ID,
			Title:         entry.Title,
			Category:      entry.Category.Term,
			ExceptionType: entry.Content.Exception.Type,
			Program:       entry.Content.Source.Program,
			Include:       entry.Content.Source.Include,
			Line:          line,
			User:          entry.Content.Runtime.User,
			Client:        entry.Content.Runtime.Client,
			Host:          entry.Content.Runtime.Host,
			Timestamp:     entry.Updated,
			URI:           entry.Link.Href,
		}
		result = append(result, dump)
	}

	return result, nil
}

// parseDumpDetails parses the HTML dump details response.
func parseDumpDetails(data []byte, dumpID string) (*DumpDetails, error) {
	html := string(data)

	details := &DumpDetails{
		RuntimeDump: RuntimeDump{
			ID: dumpID,
		},
		RawHTML: html,
	}

	// Extract basic info from HTML (simplified parsing)
	// The actual HTML structure varies, so we store the raw HTML
	// and extract what we can

	// Try to extract title
	if idx := strings.Index(html, "<title>"); idx >= 0 {
		endIdx := strings.Index(html[idx:], "</title>")
		if endIdx > 0 {
			details.Title = strings.TrimSpace(html[idx+7 : idx+endIdx])
		}
	}

	return details, nil
}

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
	GrossTime   int64   `json:"grossTime,omitempty"`   // microseconds
	NetTime     int64   `json:"netTime,omitempty"`     // microseconds
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

// SQLTraceState represents the current state of SQL tracing.
type SQLTraceState struct {
	Active      bool   `json:"active"`
	User        string `json:"user,omitempty"`
	TraceType   string `json:"traceType,omitempty"`
	StartTime   string `json:"startTime,omitempty"`
	MaxRecords  int    `json:"maxRecords,omitempty"`
	TraceFile   string `json:"traceFile,omitempty"`
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

// GetSQLTraceState checks if SQL trace is currently active.
func (c *Client) GetSQLTraceState(ctx context.Context) (*SQLTraceState, error) {
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/st05/trace/state", &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting SQL trace state: %w", err)
	}

	return parseSQLTraceState(resp.Body)
}

// ListSQLTraces retrieves a list of SQL trace files.
func (c *Client) ListSQLTraces(ctx context.Context, user string, maxResults int) ([]SQLTraceEntry, error) {
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

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/atom+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("listing SQL traces: %w", err)
	}

	return parseSQLTraceDirectory(resp.Body)
}

// parseSQLTraceState parses the SQL trace state XML.
func parseSQLTraceState(data []byte) (*SQLTraceState, error) {
	type stateXML struct {
		XMLName    xml.Name `xml:"traceState"`
		Active     string   `xml:"active,attr"`
		User       string   `xml:"user,attr"`
		TraceType  string   `xml:"traceType,attr"`
		StartTime  string   `xml:"startTime,attr"`
		MaxRecords string   `xml:"maxRecords,attr"`
		TraceFile  string   `xml:"traceFile,attr"`
	}

	var state stateXML
	if err := xml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing SQL trace state: %w", err)
	}

	var maxRecords int
	if state.MaxRecords != "" {
		fmt.Sscanf(state.MaxRecords, "%d", &maxRecords)
	}

	return &SQLTraceState{
		Active:     state.Active == "true" || state.Active == "X",
		User:       state.User,
		TraceType:  state.TraceType,
		StartTime:  state.StartTime,
		MaxRecords: maxRecords,
		TraceFile:  state.TraceFile,
	}, nil
}

// sqlTraceEntryXML is used for parsing SQL trace directory.
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

// parseSQLTraceDirectory parses the SQL trace directory feed.
func parseSQLTraceDirectory(data []byte) ([]SQLTraceEntry, error) {
	type feedXML struct {
		XMLName xml.Name           `xml:"feed"`
		Entries []sqlTraceEntryXML `xml:"entry"`
	}

	var feed feedXML
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parsing SQL trace directory: %w", err)
	}

	result := make([]SQLTraceEntry, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		var recordCount int
		var size int64
		if entry.Content.Trace.RecordCount != "" {
			fmt.Sscanf(entry.Content.Trace.RecordCount, "%d", &recordCount)
		}
		if entry.Content.Trace.Size != "" {
			fmt.Sscanf(entry.Content.Trace.Size, "%d", &size)
		}

		trace := SQLTraceEntry{
			ID:          entry.ID,
			User:        entry.Author.Name,
			StartTime:   entry.Content.Trace.StartTime,
			EndTime:     entry.Content.Trace.EndTime,
			TraceType:   entry.Content.Trace.TraceType,
			RecordCount: recordCount,
			Size:        size,
			URI:         entry.Link.Href,
		}
		result = append(result, trace)
	}

	return result, nil
}

