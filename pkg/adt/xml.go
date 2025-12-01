package adt

import (
	"encoding/xml"
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

// TableField represents a field in a table/structure.
type TableField struct {
	Name        string `xml:"name,attr"`
	Type        string `xml:"type,attr"`
	Length      int    `xml:"length,attr,omitempty"`
	Decimals    int    `xml:"decimals,attr,omitempty"`
	Description string `xml:"description,attr,omitempty"`
	IsKey       bool   `xml:"isKey,attr,omitempty"`
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
