package adt

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// Common XML namespaces used in ADT responses
const (
	NSAdtCore = "http://www.sap.com/adt/core"
	NSAtom    = "http://www.w3.org/2005/Atom"
)

// ObjectReference represents a reference to an ABAP object.
type ObjectReference struct {
	XMLName     xml.Name `xml:"objectReference"`
	URI         string   `xml:"uri,attr"`
	Type        string   `xml:"type,attr"`
	Name        string   `xml:"name,attr"`
	PackageName string   `xml:"packageName,attr,omitempty"`
	Description string   `xml:"description,attr,omitempty"`
}

// Link represents a hyperlink in ADT responses.
type Link struct {
	XMLName xml.Name `xml:"link"`
	Href    string   `xml:"href,attr"`
	Rel     string   `xml:"rel,attr"`
	Type    string   `xml:"type,attr,omitempty"`
	Title   string   `xml:"title,attr,omitempty"`
	Name    string   `xml:"http://www.sap.com/adt/core name,attr,omitempty"` // adtcore:name attribute
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

// ProgramStructure represents ABAP program structure.
type ProgramStructure struct {
	XMLName xml.Name `xml:"program"`
	URI     string   `xml:"uri,attr"`
	Type    string   `xml:"type,attr"`
	Name    string   `xml:"name,attr"`
	Version string   `xml:"version,attr,omitempty"`
	Links   []Link   `xml:"link"`
}

// PackageContent represents package contents response.
type PackageContent struct {
	XMLName     xml.Name        `xml:"package"`
	URI         string          `xml:"uri,attr"`
	Type        string          `xml:"type,attr"`
	Name        string          `xml:"name,attr"`
	SubPackages []string        `json:"subPackages,omitempty"`
	Objects     []PackageObject `json:"objects,omitempty"`
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
	XMLName   xml.Name `xml:"group"`
	URI       string   `xml:"uri,attr"`
	Type      string   `xml:"type,attr"`
	Name      string   `xml:"name,attr"`
	Version   string   `xml:"version,attr,omitempty"`
	Links     []Link   `xml:"link"`
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
	Name        string `xml:"name,attr" json:"name"`                   // Field name
	Type        string `xml:"type,attr" json:"type"`                   // ABAP type: CHAR, NUMC, INT4, DEC, STRING, etc.
	Length      int    `xml:"length,attr,omitempty" json:"length,omitempty"`     // Length for CHAR, NUMC, RAW, etc.
	Decimals    int    `xml:"decimals,attr,omitempty" json:"decimals,omitempty"` // Decimals for DEC, CURR, QUAN
	Description string `xml:"description,attr,omitempty" json:"description,omitempty"`
	IsKey       bool   `xml:"isKey,attr,omitempty" json:"key,omitempty"`         // Primary key field
	NotNull     bool   `xml:"-" json:"notNull,omitempty"`                        // NOT NULL constraint
}

// ClassObjectStructure represents the object structure of a class with methods.
// Used for method-level source operations.
type ClassObjectStructure struct {
	XMLName  xml.Name                  `xml:"objectStructureElement"`
	Name     string                    `xml:"name,attr"`
	Type     string                    `xml:"type,attr"`
	Elements []ClassObjectStructureElement `xml:"objectStructureElement"`
	Links    []ClassObjectStructureLink    `xml:"link"`
}

// ClassObjectStructureElement represents an element (method, attribute, type) in the class structure.
type ClassObjectStructureElement struct {
	Name      string                     `xml:"name,attr"`
	Type      string                     `xml:"type,attr"` // CLAS/OM for method, CLAS/OA for attribute, CLAS/OT for type
	ClifName  string                     `xml:"clif_name,attr,omitempty"`
	Level     string                     `xml:"level,attr,omitempty"`     // instance or static
	Visibility string                    `xml:"visibility,attr,omitempty"` // public, protected, private
	Links     []ClassObjectStructureLink `xml:"link"`
}

// ClassObjectStructureLink represents a link in the class object structure.
type ClassObjectStructureLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr,omitempty"`
}

// MethodInfo represents information about a class method with source boundaries.
type MethodInfo struct {
	Name              string // Method name
	Visibility        string // public, protected, private
	Level             string // instance or static
	DefinitionStart   int    // Line number where definition starts
	DefinitionEnd     int    // Line number where definition ends
	ImplementationStart int  // Line number where implementation starts
	ImplementationEnd   int  // Line number where implementation ends
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
	idx := strings.Index(href, "#")
	if idx == -1 {
		return 0, 0
	}
	fragment := href[idx+1:]

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

// ParseObjectStructure parses XML object structure.
func ParseObjectStructure(data []byte) (*ObjectStructure, error) {
	var obj ObjectStructure
	if err := xml.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

// FindLink finds a link by relation in a slice of links.
func FindLink(links []Link, rel string) *Link {
	for i := range links {
		if links[i].Rel == rel {
			return &links[i]
		}
	}
	return nil
}

// FindLinkByType finds a link by content type in a slice of links.
func FindLinkByType(links []Link, contentType string) *Link {
	for i := range links {
		if strings.Contains(links[i].Type, contentType) {
			return &links[i]
		}
	}
	return nil
}

// ExtractSourceLink extracts the source code link from an object structure.
func ExtractSourceLink(links []Link) string {
	// Try different relation names used by ADT
	for _, rel := range []string{
		"http://www.sap.com/adt/relations/source/main",
		"http://www.sap.com/adt/relations/source",
		"source",
	} {
		if link := FindLink(links, rel); link != nil {
			return link.Href
		}
	}
	return ""
}

// --- API Release State Types (Clean Core / ABAP Cloud) ---

// APIReleaseState represents the release state of an ABAP object for Clean Core compatibility.
// This is used to check if an object is released for use in ABAP Cloud / S/4HANA Cloud.

type APIReleaseState struct {
	Object  *APIReleaseStateObj     `xml:"releasableObject" json:"releasableObject,omitempty"`
	C0      *APIReleaseStateRelease `xml:"c0Release" json:"c0,omitempty"`
	C1      *APIReleaseStateRelease `xml:"c1Release" json:"c1,omitempty"`
	C2      *APIReleaseStateRelease `xml:"c2Release" json:"c2,omitempty"`
	C3      *APIReleaseStateRelease `xml:"c3Release" json:"c3,omitempty"`
	C4      *APIReleaseStateRelease `xml:"c4Release" json:"c4,omitempty"`
	Catalog APIReleaseStateCatalog  `xml:"apiCatalogData" json:"apiCatalogData"`
}

type APIReleaseStateObj struct {
	URI  string `xml:"uri,attr" json:"uri,omitempty"`
	Type string `xml:"type,attr" json:"type,omitempty"`
	Name string `xml:"name,attr" json:"name,omitempty"`
}

type APIReleaseStateRelease struct {
	Contract              string                  `xml:"contract,attr" json:"contract,omitempty"`
	UseInKeyUserApps      bool                    `xml:"useInKeyUserApps,attr" json:"useInKeyUserApps"`
	UseInSAPCloudPlatform bool                    `xml:"useInSAPCloudPlatform,attr" json:"useInSAPCloudPlatform"`
	Name                  string                  `xml:"name,attr" json:"name,omitempty"`
	ChangedAt             string                  `xml:"changedAt,attr" json:"changedAt,omitempty"`
	ChangedBy             string                  `xml:"changedBy,attr" json:"changedBy,omitempty"`
	Status                APIReleaseStateStatus   `xml:"status" json:"status"`
	UseConceptAsSuccessor bool                    `xml:"useConceptAsSuccessor" json:"useConceptAsSuccessor"`
	Successors            []APIReleaseStateObj    `xml:"successors>successor" json:"successors,omitempty"`
	SuccessorConceptName  string                  `xml:"successorConceptName" json:"successorConceptName,omitempty"`
	StateTransitions      []APIReleaseStateStatus `xml:"stateTransitions>status" json:"stateTransitions,omitempty"`
}

type APIReleaseStateStatus struct {
	State            string `xml:"state,attr" json:"state,omitempty"`
	StateDescription string `xml:"stateDescription,attr" json:"stateDescription,omitempty"`
}

type APIReleaseStateCatalog struct {
	IsAnyAssignmentPossible bool `xml:"isAnyAssignmentPossible,attr" json:"isAnyAssignmentPossible"`
	IsAnyContractReleased   bool `xml:"isAnyContractReleased,attr" json:"isAnyContractReleased"`
}

// --- Revision (Version History) Types ---

// Revision represents a single version of an ABAP object in the revision history.
type Revision struct {
	URI          string `json:"uri"`                    // Content URL for fetching this version's source
	Version      string `json:"version"`                // Version identifier (entry ID)
	VersionTitle string `json:"versionTitle"`           // Human-readable version title
	Date         string `json:"date"`                   // ISO 8601 timestamp
	Author       string `json:"author"`                 // Username who made the change
	Transport    string `json:"transport,omitempty"`     // Transport request number
}

// revisionFeedEntry is an internal type for parsing ADT version Atom feed entries.
type revisionFeedEntry struct {
	XMLName xml.Name `xml:"entry"`
	ID      string   `xml:"id"`
	Title   string   `xml:"title"`
	Updated string   `xml:"updated"`
	Author  struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Content struct {
		Src  string `xml:"src,attr"`
		Type string `xml:"type,attr"`
	} `xml:"content"`
	Links []Link `xml:"link"`
}

// revisionFeed is an internal type for parsing ADT version Atom feeds.
type revisionFeed struct {
	XMLName xml.Name            `xml:"feed"`
	Title   string              `xml:"title"`
	Entries []revisionFeedEntry `xml:"entry"`
}

// ParseRevisionFeed parses an ADT versions Atom feed into Revision entries.
func ParseRevisionFeed(data []byte) ([]Revision, error) {
	var feed revisionFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parsing revision feed: %w", err)
	}

	revisions := make([]Revision, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		rev := Revision{
			URI:          entry.Content.Src,
			VersionTitle: entry.Title,
			Date:         entry.Updated,
			Version:      entry.ID,
			Author:       entry.Author.Name,
		}
		// Extract transport request from links (adtcore:name attribute)
		for _, link := range entry.Links {
			if strings.Contains(link.Type, "transportrequests") {
				if link.Name != "" {
					rev.Transport = link.Name
				}
				break
			}
		}
		revisions = append(revisions, rev)
	}

	return revisions, nil
}

// SourceSearchTextLine represents a single line match in source search.
type SourceSearchTextLine struct {
	URI     string `xml:"uri,attr" json:"uri,omitempty"`
	Content string `xml:"content" json:"content"`
}

// SourceSearchMainObject represents the main object info in search result.
type SourceSearchMainObject struct {
	Name        string `xml:"name,attr" json:"name"`
	Type        string `xml:"type,attr" json:"type"`
	Description string `xml:"description,attr,omitempty" json:"description,omitempty"`
	Responsible string `xml:"responsible,attr,omitempty" json:"responsible,omitempty"`
	ChangedBy   string `xml:"changedBy,attr,omitempty" json:"changedBy,omitempty"`
}

// SourceSearchObject represents a single object match from SRIS fulltext search.
type SourceSearchObject struct {
	URI        string                 `xml:"uri,attr" json:"uri"`
	ParentURI  string                 `xml:"parentUri,attr,omitempty" json:"parentUri,omitempty"`
	IsResult   string                 `xml:"isResult,attr,omitempty" json:"isResult,omitempty"`
	MainObject SourceSearchMainObject `xml:"adtMainObject" json:"mainObject"`
	TextLines  []SourceSearchTextLine `xml:"textLines>textLine" json:"textLines,omitempty"`
}

// SourceSearchResults wraps source search results from the ADT textsearch API.
type SourceSearchResults struct {
	XMLName         xml.Name             `xml:"textSearchResult"`
	NumberOfResults int                  `xml:"numberOfResults,attr" json:"numberOfResults"`
	TotalResults    int                  `xml:"totalNumberOfResults,attr" json:"totalNumberOfResults"`
	QueryTimeMillis int                  `xml:"queryTimeMillis,attr" json:"queryTimeMillis"`
	Objects         []SourceSearchObject `xml:"textSearchObjects>textSearchObject" json:"objects"`
}

// SourceSearchResponse contains the parsed results of a source search.
type SourceSearchResponse struct {
	Objects         []SourceSearchObject `json:"objects"`
	NumberOfResults int                  `json:"numberOfResults"`
	TotalResults    int                  `json:"totalResults"`
	QueryTimeMillis int                  `json:"queryTimeMillis"`
	Query           string               `json:"query"`
}

// ParseSourceSearchResults parses XML source search results from SRIS textsearch API.
func ParseSourceSearchResults(data []byte) (*SourceSearchResults, error) {
	var results SourceSearchResults
	if err := xml.Unmarshal(data, &results); err != nil {
		return nil, err
	}
	return &results, nil
}
