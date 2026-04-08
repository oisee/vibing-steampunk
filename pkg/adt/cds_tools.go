package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- CDS Impact Analysis (Where-Used for CDS) ---

// CDSImpactedObject represents a single consumer of a CDS view.
type CDSImpactedObject struct {
	URI         string `json:"uri"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Package     string `json:"package,omitempty"`
}

// CDSImpactAnalysisResult is the result of GetCDSImpactAnalysis.
type CDSImpactAnalysisResult struct {
	ViewName        string              `json:"viewName"`
	ImpactedObjects []CDSImpactedObject `json:"impactedObjects"`
	TotalCount      int                 `json:"totalCount"`
}

// GetCDSImpactAnalysis retrieves reverse dependencies (where-used) for a CDS view.
// Returns all objects that consume/reference the given CDS view (downstream consumers).
// Uses the ADT usage references API (same as FindReferences but with CDS-specific URI).
func (c *Client) GetCDSImpactAnalysis(ctx context.Context, cdsViewName string) (*CDSImpactAnalysisResult, error) {
	if err := c.checkSafety(OpRead, "GetCDSImpactAnalysis"); err != nil {
		return nil, err
	}

	cdsViewName = strings.ToUpper(cdsViewName)

	// Build CDS view URI for the where-used query
	objectURI := fmt.Sprintf("/sap/bc/adt/ddic/ddl/sources/%s", url.PathEscape(cdsViewName))

	body := `<?xml version="1.0" encoding="ASCII"?>
<usagereferences:usageReferenceRequest xmlns:usagereferences="http://www.sap.com/adt/ris/usageReferences">
  <usagereferences:affectedObjects/>
</usagereferences:usageReferenceRequest>`

	endpoint := fmt.Sprintf("/sap/bc/adt/repository/informationsystem/usageReferences?uri=%s",
		url.QueryEscape(objectURI))

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(body),
		ContentType: "application/*",
		Accept:      "application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("CDS impact analysis failed: %w", err)
	}

	return parseCDSImpactAnalysis(resp.Body, cdsViewName)
}

func parseCDSImpactAnalysis(data []byte, viewName string) (*CDSImpactAnalysisResult, error) {
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "usageReferences:", "")
	xmlStr = strings.ReplaceAll(xmlStr, "adtcore:", "")

	type packageRef struct {
		Name string `xml:"name,attr"`
	}
	type adtObject struct {
		URI         string     `xml:"uri,attr"`
		Type        string     `xml:"type,attr"`
		Name        string     `xml:"name,attr"`
		Description string     `xml:"description,attr"`
		PackageRef  packageRef `xml:"packageRef"`
	}
	type referencedObject struct {
		URI       string    `xml:"uri,attr"`
		IsResult  bool      `xml:"isResult,attr"`
		AdtObject adtObject `xml:"adtObject"`
	}
	type referencedObjects struct {
		Objects []referencedObject `xml:"referencedObject"`
	}
	type usageReferenceResult struct {
		XMLName xml.Name          `xml:"usageReferenceResult"`
		Objects referencedObjects `xml:"referencedObjects"`
	}

	var resp usageReferenceResult
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return &CDSImpactAnalysisResult{
			ViewName: viewName,
		}, nil
	}

	result := &CDSImpactAnalysisResult{
		ViewName: viewName,
	}

	for _, obj := range resp.Objects.Objects {
		if obj.IsResult {
			result.ImpactedObjects = append(result.ImpactedObjects, CDSImpactedObject{
				URI:         obj.AdtObject.URI,
				Type:        obj.AdtObject.Type,
				Name:        obj.AdtObject.Name,
				Description: obj.AdtObject.Description,
				Package:     obj.AdtObject.PackageRef.Name,
			})
		}
	}

	result.TotalCount = len(result.ImpactedObjects)
	return result, nil
}

// --- CDS Element Info ---

// CDSElementInfo contains metadata for a CDS view element (field).
type CDSElementInfo struct {
	Name        string            `json:"name"`
	Type        string            `json:"type,omitempty"`
	Description string            `json:"description,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Semantics   string            `json:"semantics,omitempty"`
}

// CDSElementInfoResult is the result of GetCDSElementInfo.
type CDSElementInfoResult struct {
	ViewName string           `json:"viewName"`
	Elements []CDSElementInfo `json:"elements"`
}

// GetCDSElementInfo retrieves metadata for all elements (fields) of a CDS view.
// Returns field names, types, and annotation information from the DDL source.
// Uses the ADT element info endpoint to get structured CDS metadata.
func (c *Client) GetCDSElementInfo(ctx context.Context, cdsViewName string) (*CDSElementInfoResult, error) {
	if err := c.checkSafety(OpRead, "GetCDSElementInfo"); err != nil {
		return nil, err
	}

	cdsViewName = strings.ToUpper(cdsViewName)

	// Use the ADT DDL element info endpoint
	endpoint := fmt.Sprintf("/sap/bc/adt/ddic/ddl/sources/%s", url.PathEscape(cdsViewName))

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.ddic.ddlsources.v2+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("CDS element info failed: %w", err)
	}

	return parseCDSElementInfo(resp.Body, cdsViewName)
}

func parseCDSElementInfo(data []byte, viewName string) (*CDSElementInfoResult, error) {
	xmlStr := string(data)
	// Strip common ADT namespace prefixes
	xmlStr = strings.ReplaceAll(xmlStr, "ddl:", "")
	xmlStr = strings.ReplaceAll(xmlStr, "adtcore:", "")
	xmlStr = strings.ReplaceAll(xmlStr, "atom:", "")

	// Parse the DDL source metadata response
	type annotation struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	}
	type element struct {
		Name        string       `xml:"name,attr"`
		Type        string       `xml:"type,attr"`
		Description string       `xml:"description,attr"`
		Semantics   string       `xml:"semantics,attr"`
		Annotations []annotation `xml:"annotation"`
	}
	type ddlSource struct {
		XMLName     xml.Name  `xml:"ddlSource"`
		Name        string    `xml:"name,attr"`
		Description string    `xml:"description,attr"`
		Elements    []element `xml:"content>element"`
	}

	// Try structured element parsing first
	var resp ddlSource
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		// Fallback: try simpler structure (different ADT versions return different formats)
		type simpleField struct {
			Name        string `xml:"name,attr"`
			Type        string `xml:"type,attr"`
			Description string `xml:"description,attr"`
		}
		type simpleResp struct {
			XMLName xml.Name      `xml:"ddlSource"`
			Name    string        `xml:"name,attr"`
			Fields  []simpleField `xml:"field"`
		}

		var simple simpleResp
		if err2 := xml.Unmarshal([]byte(xmlStr), &simple); err2 != nil {
			// Return empty result rather than error — the endpoint may not support element detail
			return &CDSElementInfoResult{
				ViewName: viewName,
			}, nil
		}

		result := &CDSElementInfoResult{ViewName: viewName}
		for _, f := range simple.Fields {
			result.Elements = append(result.Elements, CDSElementInfo{
				Name:        f.Name,
				Type:        f.Type,
				Description: f.Description,
			})
		}
		return result, nil
	}

	result := &CDSElementInfoResult{ViewName: viewName}
	for _, elem := range resp.Elements {
		info := CDSElementInfo{
			Name:        elem.Name,
			Type:        elem.Type,
			Description: elem.Description,
			Semantics:   elem.Semantics,
		}
		if len(elem.Annotations) > 0 {
			info.Annotations = make(map[string]string, len(elem.Annotations))
			for _, ann := range elem.Annotations {
				info.Annotations[ann.Name] = ann.Value
			}
		}
		result.Elements = append(result.Elements, info)
	}

	return result, nil
}
