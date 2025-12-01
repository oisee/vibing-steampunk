package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// --- Find Definition (Navigate to Definition) ---

// DefinitionLocation represents the location of a definition.
type DefinitionLocation struct {
	URL    string `json:"url"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// FindDefinition navigates to the definition of a symbol at the given position.
// sourceURL is the URL of the source file (e.g., "/sap/bc/adt/programs/programs/ZTEST/source/main")
// source is the full source code
// line is 1-based line number
// startCol and endCol define the column range of the symbol (1-based)
// implementation: if true, navigate to implementation instead of definition
// mainProgram: optional main program for includes
func (c *Client) FindDefinition(ctx context.Context, sourceURL string, source string, line int, startCol int, endCol int, implementation bool, mainProgram string) (*DefinitionLocation, error) {
	// Build the URI with position information
	ctxParam := ""
	if mainProgram != "" {
		ctxParam = "?context=" + url.QueryEscape(mainProgram)
	}
	uri := fmt.Sprintf("%s%s#start=%d,%d;end=%d,%d", sourceURL, ctxParam, line, startCol, line, endCol)

	filter := "definition"
	if implementation {
		filter = "implementation"
	}

	endpoint := fmt.Sprintf("/sap/bc/adt/navigation/target?uri=%s&filter=%s",
		url.QueryEscape(uri), filter)

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(source),
		ContentType: "text/plain",
		Accept:      "application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("find definition failed: %w", err)
	}

	return parseDefinitionLocation(resp.Body)
}

func parseDefinitionLocation(data []byte) (*DefinitionLocation, error) {
	// Response format:
	// <adtcore:objectReference adtcore:uri="/sap/bc/adt/.../source/main#start=10,5" .../>

	type objectRef struct {
		URI string `xml:"uri,attr"`
	}

	// Strip namespace prefix
	xmlStr := strings.ReplaceAll(string(data), "adtcore:", "")

	var ref objectRef
	if err := xml.Unmarshal([]byte(xmlStr), &ref); err != nil {
		return nil, fmt.Errorf("parsing definition location: %w", err)
	}

	result := &DefinitionLocation{}
	// Parse URL#start=line,column
	lineColRegex := regexp.MustCompile(`([^#]+)#start=(\d+),(\d+)`)
	if matches := lineColRegex.FindStringSubmatch(ref.URI); matches != nil {
		result.URL = matches[1]
		result.Line, _ = strconv.Atoi(matches[2])
		result.Column, _ = strconv.Atoi(matches[3])
	} else {
		result.URL = ref.URI
	}

	return result, nil
}

// --- Find References (Usage References) ---

// UsageReference represents a reference to a symbol.
type UsageReference struct {
	URI              string `json:"uri"`
	ObjectIdentifier string `json:"objectIdentifier"`
	ParentURI        string `json:"parentUri"`
	IsResult         bool   `json:"isResult"`
	CanHaveChildren  bool   `json:"canHaveChildren"`
	UsageInformation string `json:"usageInformation"`
	Responsible      string `json:"responsible"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	Description      string `json:"description"`
	PackageURI       string `json:"packageUri"`
	PackageName      string `json:"packageName"`
}

// FindReferences finds all references to a symbol.
// objectURL is the URL of the object (e.g., "/sap/bc/adt/oo/classes/ZCL_TEST")
// line and column are optional - if provided, find references to symbol at position
func (c *Client) FindReferences(ctx context.Context, objectURL string, line int, column int) ([]UsageReference, error) {
	uri := objectURL
	if line > 0 && column > 0 {
		uri = fmt.Sprintf("%s#start=%d,%d", objectURL, line, column)
	}

	body := `<?xml version="1.0" encoding="ASCII"?>
<usagereferences:usageReferenceRequest xmlns:usagereferences="http://www.sap.com/adt/ris/usageReferences">
  <usagereferences:affectedObjects/>
</usagereferences:usageReferenceRequest>`

	endpoint := fmt.Sprintf("/sap/bc/adt/repository/informationsystem/usageReferences?uri=%s",
		url.QueryEscape(uri))

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(body),
		ContentType: "application/*",
		Accept:      "application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("find references failed: %w", err)
	}

	return parseUsageReferences(resp.Body)
}

func parseUsageReferences(data []byte) ([]UsageReference, error) {
	// Strip namespace prefixes
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "usageReferences:", "")
	xmlStr = strings.ReplaceAll(xmlStr, "adtcore:", "")

	type packageRef struct {
		URI  string `xml:"uri,attr"`
		Name string `xml:"name,attr"`
	}
	type adtObject struct {
		URI         string     `xml:"uri,attr"`
		Type        string     `xml:"type,attr"`
		Name        string     `xml:"name,attr"`
		Responsible string     `xml:"responsible,attr"`
		Description string     `xml:"description,attr"`
		PackageRef  packageRef `xml:"packageRef"`
	}
	type referencedObject struct {
		URI              string    `xml:"uri,attr"`
		ObjectIdentifier string    `xml:"objectIdentifier,attr"`
		ParentURI        string    `xml:"parentUri,attr"`
		IsResult         bool      `xml:"isResult,attr"`
		CanHaveChildren  bool      `xml:"canHaveChildren,attr"`
		UsageInformation string    `xml:"usageInformation,attr"`
		AdtObject        adtObject `xml:"adtObject"`
	}
	type referencedObjects struct {
		Objects []referencedObject `xml:"referencedObject"`
	}
	type response struct {
		ReferencedObjects referencedObjects `xml:"referencedObjects"`
	}

	var resp response
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing usage references: %w", err)
	}

	var results []UsageReference
	for _, obj := range resp.ReferencedObjects.Objects {
		ref := UsageReference{
			URI:              obj.URI,
			ObjectIdentifier: obj.ObjectIdentifier,
			ParentURI:        obj.ParentURI,
			IsResult:         obj.IsResult,
			CanHaveChildren:  obj.CanHaveChildren,
			UsageInformation: obj.UsageInformation,
			Responsible:      obj.AdtObject.Responsible,
			Name:             obj.AdtObject.Name,
			Type:             obj.AdtObject.Type,
			Description:      obj.AdtObject.Description,
			PackageURI:       obj.AdtObject.PackageRef.URI,
			PackageName:      obj.AdtObject.PackageRef.Name,
		}

		// If type is not in the adtObject, try to extract from URI
		if ref.Type == "" && ref.URI != "" {
			ref.Type = extractTypeFromURI(ref.URI)
		}

		results = append(results, ref)
	}

	return results, nil
}

// extractTypeFromURI tries to extract the object type from ADT URI patterns
func extractTypeFromURI(uri string) string {
	// Common patterns: /sap/bc/adt/oo/classes/..., /sap/bc/adt/programs/programs/...
	patterns := map[string]string{
		"/oo/classes/":      "CLAS/OC",
		"/oo/interfaces/":   "INTF/OI",
		"/programs/programs": "PROG/P",
		"/programs/includes": "PROG/I",
		"/functions/groups/": "FUGR/F",
	}
	for pattern, objType := range patterns {
		if strings.Contains(uri, pattern) {
			return objType
		}
	}
	return ""
}

// --- Code Completion ---

// CompletionProposal represents a code completion suggestion.
type CompletionProposal struct {
	Kind         int    `json:"kind"`
	Identifier   string `json:"identifier"`
	Icon         int    `json:"icon"`
	SubIcon      int    `json:"subIcon"`
	Bold         int    `json:"bold"`
	Color        int    `json:"color"`
	PrefixLength int    `json:"prefixLength"`
	Role         int    `json:"role"`
	Location     int    `json:"location"`
	Grade        int    `json:"grade"`
	Visibility   int    `json:"visibility"`
	IsInherited  int    `json:"isInherited"`
}

// CodeCompletion retrieves completion proposals at a given position.
// sourceURL is the URL of the source (e.g., "/sap/bc/adt/programs/programs/ZTEST/source/main")
// source is the full source code
// line and column are 1-based positions
func (c *Client) CodeCompletion(ctx context.Context, sourceURL string, source string, line int, column int) ([]CompletionProposal, error) {
	uri := fmt.Sprintf("%s#start=%d,%d", sourceURL, line, column)

	endpoint := fmt.Sprintf("/sap/bc/adt/abapsource/codecompletion/proposal?uri=%s&signalCompleteness=true",
		url.QueryEscape(uri))

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(source),
		ContentType: "application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("code completion failed: %w", err)
	}

	return parseCompletionProposals(resp.Body)
}

func parseCompletionProposals(data []byte) ([]CompletionProposal, error) {
	// Response format is ABAP serialized XML (asx:abap)
	type completion struct {
		Kind         int    `xml:"KIND"`
		Identifier   string `xml:"IDENTIFIER"`
		Icon         int    `xml:"ICON"`
		SubIcon      int    `xml:"SUBICON"`
		Bold         int    `xml:"BOLD"`
		Color        int    `xml:"COLOR"`
		PrefixLength int    `xml:"PREFIXLENGTH"`
		Role         int    `xml:"ROLE"`
		Location     int    `xml:"LOCATION"`
		Grade        int    `xml:"GRADE"`
		Visibility   int    `xml:"VISIBILITY"`
		IsInherited  int    `xml:"IS_INHERITED"`
	}
	type completionData struct {
		Completions []completion `xml:"SCC_COMPLETION"`
	}
	type values struct {
		Data completionData `xml:"DATA"`
	}
	type response struct {
		Values values `xml:"values"`
	}

	// Strip namespace prefix
	xmlStr := strings.ReplaceAll(string(data), "asx:", "")

	var resp response
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing completion proposals: %w", err)
	}

	var results []CompletionProposal
	for _, c := range resp.Values.Data.Completions {
		// Skip end marker
		if c.Identifier == "@end" || c.Identifier == "" {
			continue
		}
		results = append(results, CompletionProposal{
			Kind:         c.Kind,
			Identifier:   c.Identifier,
			Icon:         c.Icon,
			SubIcon:      c.SubIcon,
			Bold:         c.Bold,
			Color:        c.Color,
			PrefixLength: c.PrefixLength,
			Role:         c.Role,
			Location:     c.Location,
			Grade:        c.Grade,
			Visibility:   c.Visibility,
			IsInherited:  c.IsInherited,
		})
	}

	return results, nil
}

// CodeCompletionFull retrieves the full insertion text for a completion.
// patternKey is the key from the initial completion proposal
func (c *Client) CodeCompletionFull(ctx context.Context, sourceURL string, source string, line int, column int, patternKey string) (string, error) {
	uri := fmt.Sprintf("%s#start=%d,%d", sourceURL, line, column)

	endpoint := fmt.Sprintf("/sap/bc/adt/abapsource/codecompletion/insertion?uri=%s&patternKey=%s",
		url.QueryEscape(uri), url.QueryEscape(patternKey))

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(source),
		ContentType: "application/*",
	})
	if err != nil {
		return "", fmt.Errorf("code completion full failed: %w", err)
	}

	return string(resp.Body), nil
}

// --- Pretty Printer (Code Formatter) ---

// PrettyPrinterStyle defines the keyword case style.
type PrettyPrinterStyle string

const (
	PrettyPrinterStyleLower        PrettyPrinterStyle = "toLower"
	PrettyPrinterStyleUpper        PrettyPrinterStyle = "toUpper"
	PrettyPrinterStyleKeywordUpper PrettyPrinterStyle = "keywordUpper"
	PrettyPrinterStyleKeywordLower PrettyPrinterStyle = "keywordLower"
	PrettyPrinterStyleKeywordAuto  PrettyPrinterStyle = "keywordAuto"
	PrettyPrinterStyleNone         PrettyPrinterStyle = "none"
)

// PrettyPrinterSettings represents the formatter settings.
type PrettyPrinterSettings struct {
	Indentation bool               `json:"indentation"`
	Style       PrettyPrinterStyle `json:"style"`
}

// GetPrettyPrinterSettings retrieves the current formatter settings.
func (c *Client) GetPrettyPrinterSettings(ctx context.Context) (*PrettyPrinterSettings, error) {
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/abapsource/prettyprinter/settings", nil)
	if err != nil {
		return nil, fmt.Errorf("get pretty printer settings failed: %w", err)
	}

	return parsePrettyPrinterSettings(resp.Body)
}

func parsePrettyPrinterSettings(data []byte) (*PrettyPrinterSettings, error) {
	type settings struct {
		Indentation bool   `xml:"indentation,attr"`
		Style       string `xml:"style,attr"`
	}

	// Strip namespace prefix
	xmlStr := strings.ReplaceAll(string(data), "abapformatter:", "")

	var resp settings
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing pretty printer settings: %w", err)
	}

	return &PrettyPrinterSettings{
		Indentation: resp.Indentation,
		Style:       PrettyPrinterStyle(resp.Style),
	}, nil
}

// SetPrettyPrinterSettings updates the formatter settings.
func (c *Client) SetPrettyPrinterSettings(ctx context.Context, settings *PrettyPrinterSettings) error {
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<prettyprintersettings:PrettyPrinterSettings
xmlns:prettyprintersettings="http://www.sap.com/adt/prettyprintersettings"
prettyprintersettings:indentation="%t" prettyprintersettings:style="%s"/>`,
		settings.Indentation, settings.Style)

	_, err := c.transport.Request(ctx, "/sap/bc/adt/abapsource/prettyprinter/settings", &RequestOptions{
		Method:      http.MethodPut,
		Body:        []byte(body),
		ContentType: "application/*",
	})
	if err != nil {
		return fmt.Errorf("set pretty printer settings failed: %w", err)
	}

	return nil
}

// PrettyPrint formats ABAP source code according to the user's settings.
func (c *Client) PrettyPrint(ctx context.Context, source string) (string, error) {
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/abapsource/prettyprinter", &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(source),
		ContentType: "text/plain",
		Accept:      "text/plain",
	})
	if err != nil {
		return "", fmt.Errorf("pretty print failed: %w", err)
	}

	// Return original source if response is empty
	if len(resp.Body) == 0 {
		return source, nil
	}

	return string(resp.Body), nil
}

// --- Type Hierarchy ---

// HierarchyNode represents a node in the type hierarchy.
type HierarchyNode struct {
	HasDefOrImpl bool   `json:"hasDefOrImpl"`
	URI          string `json:"uri"`
	Line         int    `json:"line"`
	Column       int    `json:"column"`
	Type         string `json:"type"`
	Name         string `json:"name"`
	ParentURI    string `json:"parentUri"`
	Description  string `json:"description"`
}

// GetTypeHierarchy retrieves the type hierarchy (supertypes or subtypes) for a symbol.
// superTypes: if true, return supertypes; if false, return subtypes
func (c *Client) GetTypeHierarchy(ctx context.Context, sourceURL string, source string, line int, column int, superTypes bool) ([]HierarchyNode, error) {
	uri := fmt.Sprintf("%s#start=%d,%d", sourceURL, line, column)

	typeParam := "subTypes"
	if superTypes {
		typeParam = "superTypes"
	}

	endpoint := fmt.Sprintf("/sap/bc/adt/abapsource/typehierarchy?uri=%s&type=%s",
		url.QueryEscape(uri), typeParam)

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(source),
		ContentType: "text/plain",
		Accept:      "application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("get type hierarchy failed: %w", err)
	}

	return parseTypeHierarchy(resp.Body)
}

func parseTypeHierarchy(data []byte) ([]HierarchyNode, error) {
	// Strip namespace prefixes
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "hierarchy:", "")
	xmlStr = strings.ReplaceAll(xmlStr, "adtcore:", "")

	type entry struct {
		URI          string `xml:"uri,attr"`
		Type         string `xml:"type,attr"`
		Name         string `xml:"name,attr"`
		ParentURI    string `xml:"parentUri,attr"`
		Description  string `xml:"description,attr"`
		HasDefOrImpl bool   `xml:"hasDefOrImpl,attr"`
	}
	type entries struct {
		Entries []entry `xml:"entry"`
	}
	type response struct {
		Entries entries `xml:"entries"`
	}

	var resp response
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing type hierarchy: %w", err)
	}

	var results []HierarchyNode
	lineColRegex := regexp.MustCompile(`([^#]+)(?:#start=(\d+)(?:,(\d+))?)?`)

	for _, e := range resp.Entries.Entries {
		node := HierarchyNode{
			HasDefOrImpl: e.HasDefOrImpl,
			Type:         e.Type,
			Name:         e.Name,
			ParentURI:    e.ParentURI,
			Description:  e.Description,
		}

		// Parse line/column from URI
		if matches := lineColRegex.FindStringSubmatch(e.URI); matches != nil {
			node.URI = matches[1]
			if matches[2] != "" {
				node.Line, _ = strconv.Atoi(matches[2])
			}
			if matches[3] != "" {
				node.Column, _ = strconv.Atoi(matches[3])
			}
		} else {
			node.URI = e.URI
		}

		results = append(results, node)
	}

	return results, nil
}
