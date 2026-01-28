package adt

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Package-level compiled regex for parsing ADT location fragments.
// Matches patterns like: #start=10,5 or /path#start=10,5
var locationFragmentRegex = regexp.MustCompile(`#start=(\d+),(\d+)`)

// StripXMLNamespaces removes specified namespace prefixes from XML for easier parsing.
// Callers explicitly specify which prefixes to strip, making behavior predictable.
// Usage: xml.Unmarshal([]byte(StripXMLNamespaces(data, "dbg:", "adtcore:")), &result)
func StripXMLNamespaces(data string, prefixes ...string) string {
	for _, prefix := range prefixes {
		data = strings.ReplaceAll(data, prefix, "")
	}
	return data
}

// XMLEscape escapes special XML characters for safe inclusion in XML content.
func XMLEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// StripXMLNamespaceDeclarations removes common xmlns declarations from XML string.
// Use when namespace declarations cause parsing issues.
func StripXMLNamespaceDeclarations(xmlStr string, declarations ...string) string {
	// Default declarations if none provided
	if len(declarations) == 0 {
		declarations = []string{
			` xmlns:aunit="http://www.sap.com/adt/aunit"`,
			` xmlns:adtcore="http://www.sap.com/adt/core"`,
			` xmlns:atc="http://www.sap.com/adt/atc"`,
		}
	}
	for _, decl := range declarations {
		xmlStr = strings.ReplaceAll(xmlStr, decl, "")
	}
	return xmlStr
}

// ParseLocationFragment extracts line and column from ADT URI location fragment.
// Parses patterns like "#start=10,5" returning (10, 5).
// Returns (0, 0) if no match found.
func ParseLocationFragment(uri string) (line, column int) {
	if matches := locationFragmentRegex.FindStringSubmatch(uri); matches != nil {
		line, _ = strconv.Atoi(matches[1])
		column, _ = strconv.Atoi(matches[2])
	}
	return
}

// ParseLocationFragmentWithBase extracts base URI, line and column from ADT URI.
// Parses patterns like "/path/to/source#start=10,5" returning ("/path/to/source", 10, 5).
func ParseLocationFragmentWithBase(uri string) (base string, line, column int) {
	base, _, found := strings.Cut(uri, "#")
	if !found {
		return uri, 0, 0
	}
	line, column = ParseLocationFragment(uri)
	return
}

// Link represents a hyperlink in ADT responses.
type Link struct {
	XMLName xml.Name `xml:"link"`
	Href    string   `xml:"href,attr"`
	Rel     string   `xml:"rel,attr"`
	Type    string   `xml:"type,attr,omitempty"`
	Title   string   `xml:"title,attr,omitempty"`
}

// AtomEntry represents an Atom feed entry.
type AtomEntry struct {
	XMLName xml.Name `xml:"entry"`
	ID      string   `xml:"id"`
	Title   string   `xml:"title"`
	Updated string   `xml:"updated,omitempty"`
	Links   []Link   `xml:"link"`
	Content string   `xml:"content,omitempty"`
}

// AtomFeed represents an Atom feed.
type AtomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Updated string      `xml:"updated,omitempty"`
	Links   []Link      `xml:"link"`
	Entries []AtomEntry `xml:"entry"`
}

// SearchResult represents a single search result.
type SearchResult struct {
	URI            string `xml:"uri,attr"`
	Type           string `xml:"type,attr"`
	Name           string `xml:"name,attr"`
	PackageName    string `xml:"packageName,attr,omitempty"`
	Description    string `xml:"description,attr,omitempty"`
	ResponsiblePro string `xml:"responsiblePro,attr,omitempty"`
}

// SearchResults wraps search results from the ADT API.
type SearchResults struct {
	XMLName xml.Name       `xml:"objectReferences"`
	Results []SearchResult `xml:"objectReference"`
}

// ObjectStructure represents the structure of an ABAP object.
type ObjectStructure struct {
	XMLName      xml.Name          `xml:"objectStructure"`
	URI          string            `xml:"uri,attr"`
	Type         string            `xml:"type,attr"`
	Name         string            `xml:"name,attr"`
	TechnicalUri string            `xml:"technicalUri,attr,omitempty"`
	Includes     []ObjectStructure `xml:"objectStructure,omitempty"`
	Links        []Link            `xml:"link,omitempty"`
}

// ClassStructure represents ABAP class structure with includes.
type ClassStructure struct {
	XMLName       xml.Name `xml:"class"`
	URI           string   `xml:"uri,attr"`
	Type          string   `xml:"type,attr"`
	Name          string   `xml:"name,attr"`
	Version       string   `xml:"version,attr,omitempty"`
	Links         []Link   `xml:"link"`
	MainInclude   *Include `xml:"include,omitempty"`
	LocalIncludes []Include
}

// Include represents an include in an ABAP object.
type Include struct {
	XMLName xml.Name `xml:"include"`
	URI     string   `xml:"uri,attr"`
	Type    string   `xml:"type,attr"`
	Name    string   `xml:"name,attr"`
	Version string   `xml:"version,attr,omitempty"`
	Links   []Link   `xml:"link"`
}

// PackageContent represents package contents response.
type PackageContent struct {
	XMLName      xml.Name        `xml:"package"`
	URI          string          `xml:"uri,attr"`
	Type         string          `xml:"type,attr"`
	Name         string          `xml:"name,attr"`
	SubPackages  []string        `json:"subPackages,omitempty"`
	Objects      []PackageObject `json:"objects,omitempty"`
	TotalObjects int             `json:"totalObjects"`        // Total count before limiting
	Truncated    bool            `json:"truncated,omitempty"` // True if objects were limited
}

// PackageObject represents an object within a package.
type PackageObject struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	URI         string `json:"uri,omitempty"`
	Description string `json:"description,omitempty"`
}

// FunctionGroup represents a function group structure.
type FunctionGroup struct {
	XMLName   xml.Name         `xml:"abapFunctionGroup"`
	URI       string           `xml:"uri,attr"`
	Type      string           `xml:"type,attr"`
	Name      string           `xml:"name,attr"`
	Version   string           `xml:"version,attr,omitempty"`
	Links     []Link           `xml:"link"`
	Functions []FunctionModule `xml:"functionModule,omitempty"`
}

// FunctionModule represents a function module.
type FunctionModule struct {
	XMLName xml.Name `xml:"functionModule"`
	URI     string   `xml:"uri,attr"`
	Type    string   `xml:"type,attr"`
	Name    string   `xml:"name,attr"`
	Links   []Link   `xml:"link"`
}

// Interface represents an ABAP interface structure.
type Interface struct {
	XMLName xml.Name `xml:"intf"`
	URI     string   `xml:"uri,attr"`
	Type    string   `xml:"type,attr"`
	Name    string   `xml:"name,attr"`
	Version string   `xml:"version,attr,omitempty"`
	Links   []Link   `xml:"link"`
}

// TableStructure represents an ABAP table/structure.
type TableStructure struct {
	XMLName     xml.Name     `xml:"blue:blueDefinition"`
	Name        string       `xml:"name,attr"`
	Description string       `xml:"description,attr,omitempty"`
	Fields      []TableField `xml:"field,omitempty"`
}

// TableField represents a field in a table/structure definition.
// Supports both XML (for GetTable) and JSON (for CreateTable) serialization.
type TableField struct {
	Name        string `xml:"name,attr" json:"name"`                             // Field name
	Type        string `xml:"type,attr" json:"type"`                             // ABAP type: CHAR, NUMC, INT4, DEC, STRING, etc.
	Length      int    `xml:"length,attr,omitempty" json:"length,omitempty"`     // Length for CHAR, NUMC, RAW, etc.
	Decimals    int    `xml:"decimals,attr,omitempty" json:"decimals,omitempty"` // Decimals for DEC, CURR, QUAN
	Description string `xml:"description,attr,omitempty" json:"description,omitempty"`
	IsKey       bool   `xml:"isKey,attr,omitempty" json:"key,omitempty"` // Primary key field
	NotNull     bool   `xml:"-" json:"notNull,omitempty"`                // NOT NULL constraint
}

// ClassObjectStructure represents the object structure of a class with methods.
// Used for method-level source operations.
type ClassObjectStructure struct {
	XMLName  xml.Name                      `xml:"objectStructureElement"`
	Name     string                        `xml:"name,attr"`
	Type     string                        `xml:"type,attr"`
	Elements []ClassObjectStructureElement `xml:"objectStructureElement"`
	Links    []Link                        `xml:"link"`
}

// ClassObjectStructureElement represents an element (method, attribute, type) in the class structure.
type ClassObjectStructureElement struct {
	Name       string `xml:"name,attr"`
	Type       string `xml:"type,attr"` // CLAS/OM for method, CLAS/OA for attribute, CLAS/OT for type
	ClifName   string `xml:"clif_name,attr,omitempty"`
	Level      string `xml:"level,attr,omitempty"`      // instance or static
	Visibility string `xml:"visibility,attr,omitempty"` // public, protected, private
	Links      []Link `xml:"link"`
}

// MethodInfo represents information about a class method with source boundaries.
type MethodInfo struct {
	Name                string // Method name
	Visibility          string // public, protected, private
	Level               string // instance or static
	DefinitionStart     int    // Line number where definition starts
	DefinitionEnd       int    // Line number where definition ends
	ImplementationStart int    // Line number where implementation starts
	ImplementationEnd   int    // Line number where implementation ends
}

// ParseClassObjectStructure parses the class object structure XML.
func ParseClassObjectStructure(data []byte) (*ClassObjectStructure, error) {
	var obj ClassObjectStructure
	if err := xml.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

// GetMethods extracts method information from the class object structure.
func (c *ClassObjectStructure) GetMethods() []MethodInfo {
	var methods []MethodInfo

	for _, elem := range c.Elements {
		// Only process methods (type CLAS/OM)
		if elem.Type != "CLAS/OM" {
			continue
		}

		method := MethodInfo{
			Name:       elem.Name,
			Visibility: elem.Visibility,
			Level:      elem.Level,
		}

		// Parse line numbers from links
		for _, link := range elem.Links {
			switch link.Rel {
			case "http://www.sap.com/adt/relations/source/definitionBlock":
				method.DefinitionStart, method.DefinitionEnd = parseSourceRange(link.Href)
			case "http://www.sap.com/adt/relations/source/implementationBlock":
				method.ImplementationStart, method.ImplementationEnd = parseSourceRange(link.Href)
			}
		}

		methods = append(methods, method)
	}

	return methods
}

// parseSourceRange parses a source range from an ADT href.
// Format: ./../class/source/main#start=739,2;end=887,11
func parseSourceRange(href string) (start, end int) {
	// Find the fragment part
	_, fragment, found := strings.Cut(href, "#")
	if !found {
		return 0, 0
	}

	// Parse start=line,col;end=line,col
	var startLine, startCol, endLine, endCol int
	_, _ = fmt.Sscanf(fragment, "start=%d,%d;end=%d,%d", &startLine, &startCol, &endLine, &endCol)

	return startLine, endLine
}

// ParseSearchResults parses XML search results.
func ParseSearchResults(data []byte) ([]SearchResult, error) {
	var results SearchResults
	if err := xml.Unmarshal(data, &results); err != nil {
		return nil, err
	}
	return results.Results, nil
}

// APPService represents an Atom Publishing Protocol (APP) service document.
// Used for parsing /sap/bc/adt/discovery response.
type APPService struct {
	XMLName    xml.Name       `xml:"service"`
	Workspaces []APPWorkspace `xml:"workspace"`
}

// APPWorkspace represents a workspace in the APP service document.
type APPWorkspace struct {
	Title       string          `xml:"title"`
	Collections []APPCollection `xml:"collection"`
}

// APPCollection represents a collection (endpoint) in the APP service document.
type APPCollection struct {
	Href        string   `xml:"href,attr"`
	Title       string   `xml:"title"`
	Accept      []string `xml:"accept"`
	TemplateRef string   `xml:"templateRef,attr,omitempty"`
}

// Discovery holds parsed ADT discovery information.
type Discovery struct {
	Workspaces []APPWorkspace
	// collections maps URI paths to collection info for fast lookup
	collections map[string]*APPCollection
}

// HasCollection checks if a collection URI exists in the discovery.
// Supports both exact matching and prefix matching:
// - Exact match: "/sap/bc/adt/abapgit/repos" matches "/sap/bc/adt/abapgit/repos"
// - Prefix match: "/sap/bc/adt/ddic/ddl" matches "/sap/bc/adt/ddic/ddl/sources"
func (d *Discovery) HasCollection(uriPath string) bool {
	if d == nil || d.collections == nil {
		return false
	}
	// Exact match first
	if _, ok := d.collections[uriPath]; ok {
		return true
	}
	// Prefix match: check if any collection is a prefix of uriPath
	// or if uriPath is a prefix of any collection
	for href := range d.collections {
		if strings.HasPrefix(uriPath, href) || strings.HasPrefix(href, uriPath) {
			return true
		}
	}
	return false
}

// GetCollection returns collection info for a URI path.
func (d *Discovery) GetCollection(uriPath string) *APPCollection {
	if d == nil || d.collections == nil {
		return nil
	}
	return d.collections[uriPath]
}

// buildCollectionIndex builds the fast lookup map from workspaces.
func (d *Discovery) buildCollectionIndex() {
	d.collections = make(map[string]*APPCollection)
	for i := range d.Workspaces {
		for j := range d.Workspaces[i].Collections {
			coll := &d.Workspaces[i].Collections[j]
			d.collections[coll.Href] = coll
		}
	}
}
