package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

// ATC configuration property name constants.
const (
	ATCPropertySystemCheckVariant = "systemCheckVariant"
)

// ATCCustomizing represents the ATC system configuration.
type ATCCustomizing struct {
	Properties []ATCProperty  `json:"properties"`
	Exemptions []ATCExemption `json:"exemptions"`
}

// ATCProperty represents an ATC configuration property.
type ATCProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ATCExemption represents an exemption reason.
type ATCExemption struct {
	ID                     string `json:"id"`
	Title                  string `json:"title"`
	JustificationMandatory bool   `json:"justificationMandatory"`
}

// ATCRunResult represents the result of starting an ATC run.
type ATCRunResult struct {
	WorklistID string       `json:"worklistId"`
	Timestamp  int64        `json:"timestamp"`
	Infos      []ATCRunInfo `json:"infos,omitempty"`
}

// ATCRunInfo represents an info message from ATC run.
type ATCRunInfo struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ATCWorklist represents the ATC findings worklist.
type ATCWorklist struct {
	ID                  string         `json:"id"`
	Timestamp           int64          `json:"timestamp"`
	UsedObjectSet       string         `json:"usedObjectSet"`
	ObjectSetIsComplete bool           `json:"objectSetIsComplete"`
	ObjectSets          []ATCObjectSet `json:"objectSets"`
	Objects             []ATCObject    `json:"objects"`
}

// ATCObjectSet represents an object set in the worklist.
type ATCObjectSet struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
}

// ATCObject represents an object with ATC findings.
type ATCObject struct {
	URI         string       `json:"uri"`
	Type        string       `json:"type"`
	Name        string       `json:"name"`
	PackageName string       `json:"packageName"`
	Author      string       `json:"author"`
	Findings    []ATCFinding `json:"findings"`
}

// ATCFinding represents a single ATC finding.
type ATCFinding struct {
	URI               string            `json:"uri"`
	Location          string            `json:"location"`
	Priority          int               `json:"priority"` // 1=Error, 2=Warning, 3=Info
	CheckID           string            `json:"checkId"`
	CheckTitle        string            `json:"checkTitle"`
	MessageID         string            `json:"messageId"`
	MessageTitle      string            `json:"messageTitle"`
	ExemptionApproval string            `json:"exemptionApproval,omitempty"`
	ExemptionKind     string            `json:"exemptionKind,omitempty"`
	QuickfixInfo      string            `json:"quickfixInfo,omitempty"`
	Line              int               `json:"line,omitempty"`
	Column            int               `json:"column,omitempty"`
	Processor         string            `json:"processor,omitempty"`
	LastChangedBy     string            `json:"lastChangedBy,omitempty"`
	Tags              map[string]string `json:"tags,omitempty"` // REF_OBJ_NAME, REF_OBJ_TYPE, REF_PACKAGE, etc.
}

// Tag name constants for ATCFinding.Tags
const (
	ATCTagRefObjName           = "REF_OBJ_NAME"           // Referenced object name (e.g., "DECIMALS", "SY-DATUM")
	ATCTagRefObjType           = "REF_OBJ_TYPE"           // Referenced object type (e.g., "DTEL")
	ATCTagRefPackage           = "REF_PACKAGE"            // Package of referenced object (e.g., "SDIC")
	ATCTagRefSoftwareComponent = "REF_SOFTWARE_COMPONENT" // Software component (e.g., "SAP_BASIS")
	ATCTagApplicationComponent = "APPLICATION_COMPONENT"  // Application component (e.g., "BC-DWB-DIC-AC")
)

// GetATCCustomizing retrieves the ATC system configuration.
func (c *Client) GetATCCustomizing(ctx context.Context) (*ATCCustomizing, error) {
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/atc/customizing", &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml, application/vnd.sap.atc.customizing-v1+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting ATC customizing: %w", err)
	}

	return parseATCCustomizing(resp.Body)
}

func parseATCCustomizing(data []byte) (*ATCCustomizing, error) {
	xmlStr := StripXMLNamespacePrefixes(data)

	type property struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	}
	type reason struct {
		ID                     string `xml:"id,attr"`
		Title                  string `xml:"title,attr"`
		JustificationMandatory string `xml:"justificationMandatory,attr"`
	}
	type customizing struct {
		Properties struct {
			Items []property `xml:"property"`
		} `xml:"properties"`
		Exemption struct {
			Reasons struct {
				Items []reason `xml:"reason"`
			} `xml:"reasons"`
		} `xml:"exemption"`
	}

	var resp customizing
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing ATC customizing: %w", err)
	}

	result := &ATCCustomizing{
		Properties: []ATCProperty{},
		Exemptions: []ATCExemption{},
	}

	for _, p := range resp.Properties.Items {
		result.Properties = append(result.Properties, ATCProperty{
			Name:  p.Name,
			Value: p.Value,
		})
	}

	for _, r := range resp.Exemption.Reasons.Items {
		result.Exemptions = append(result.Exemptions, ATCExemption{
			ID:                     r.ID,
			Title:                  r.Title,
			JustificationMandatory: r.JustificationMandatory == "true",
		})
	}

	return result, nil
}

// GetATCCheckVariant retrieves the worklist ID for a check variant.
// If variant is empty, uses the system default check variant.
func (c *Client) GetATCCheckVariant(ctx context.Context, variant string) (string, error) {
	if variant == "" {
		// Get the default system check variant from customizing
		cust, err := c.GetATCCustomizing(ctx)
		if err != nil {
			return "", fmt.Errorf("getting ATC customizing for variant: %w", err)
		}
		for _, p := range cust.Properties {
			if p.Name == ATCPropertySystemCheckVariant {
				variant = p.Value
				break
			}
		}
		if variant == "" {
			return "", fmt.Errorf("no system check variant found in ATC customizing")
		}
	}

	resp, err := c.transport.Request(ctx, fmt.Sprintf("/sap/bc/adt/atc/worklists?checkVariant=%s", variant), &RequestOptions{
		Method: http.MethodPost,
		Accept: "text/plain",
	})
	if err != nil {
		return "", fmt.Errorf("getting ATC check variant: %w", err)
	}

	return strings.TrimSpace(string(resp.Body)), nil
}

// CreateATCRun starts an ATC check run on an object.
// worklistID is from GetATCCheckVariant, objectURL is the ADT URL of the object.
// maxResults limits the number of findings returned (default 100).
func (c *Client) CreateATCRun(ctx context.Context, worklistID string, objectURL string, maxResults int) (*ATCRunResult, error) {
	if maxResults <= 0 {
		maxResults = 100
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<atc:run maximumVerdicts="%d" xmlns:atc="http://www.sap.com/adt/atc">
	<objectSets xmlns:adtcore="http://www.sap.com/adt/core">
		<objectSet kind="inclusive">
			<adtcore:objectReferences>
				<adtcore:objectReference adtcore:uri="%s"/>
			</adtcore:objectReferences>
		</objectSet>
	</objectSets>
</atc:run>`, maxResults, objectURL)

	resp, err := c.transport.Request(ctx, fmt.Sprintf("/sap/bc/adt/atc/runs?worklistId=%s", worklistID), &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(body),
		ContentType: "application/xml",
		Accept:      "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("creating ATC run: %w", err)
	}

	return parseATCRunResult(resp.Body)
}

func parseATCRunResult(data []byte) (*ATCRunResult, error) {
	xmlStr := StripXMLNamespacePrefixes(data)

	type info struct {
		Type        string `xml:"type,attr"`
		Description string `xml:"description,attr"`
	}
	type worklistRun struct {
		WorklistID        string `xml:"worklistId"`
		WorklistTimestamp string `xml:"worklistTimestamp"`
		Infos             struct {
			Items []info `xml:"info"`
		} `xml:"infos"`
	}

	var resp worklistRun
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing ATC run result: %w", err)
	}

	result := &ATCRunResult{
		WorklistID: resp.WorklistID,
		Infos:      []ATCRunInfo{},
	}

	for _, i := range resp.Infos.Items {
		result.Infos = append(result.Infos, ATCRunInfo{
			Type:        i.Type,
			Description: i.Description,
		})
	}

	return result, nil
}

// GetATCWorklist retrieves the ATC findings worklist.
// worklistID is from CreateATCRun.
// includeExempted controls whether to include exempted findings.
func (c *Client) GetATCWorklist(ctx context.Context, worklistID string, includeExempted bool) (*ATCWorklist, error) {
	url := fmt.Sprintf("/sap/bc/adt/atc/worklists/%s?includeExemptedFindings=%t", worklistID, includeExempted)

	resp, err := c.transport.Request(ctx, url, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/atc.worklist.v1+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting ATC worklist: %w", err)
	}

	return parseATCWorklist(resp.Body)
}

// GetATCWorklistRaw retrieves the raw XML of ATC findings worklist for debugging.
func (c *Client) GetATCWorklistRaw(ctx context.Context, worklistID string) (string, error) {
	url := fmt.Sprintf("/sap/bc/adt/atc/worklists/%s?includeExemptedFindings=true", worklistID)

	resp, err := c.transport.Request(ctx, url, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/atc.worklist.v1+xml",
	})
	if err != nil {
		return "", fmt.Errorf("getting ATC worklist: %w", err)
	}

	return string(resp.Body), nil
}

func parseATCWorklist(data []byte) (*ATCWorklist, error) {
	xmlStr := StripXMLNamespacePrefixes(data)

	type tag struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	}
	type finding struct {
		URI               string `xml:"uri,attr"`
		Location          string `xml:"location,attr"`
		Priority          int    `xml:"priority,attr"`
		CheckID           string `xml:"checkId,attr"`
		CheckTitle        string `xml:"checkTitle,attr"`
		MessageID         string `xml:"messageId,attr"`
		MessageTitle      string `xml:"messageTitle,attr"`
		ExemptionApproval string `xml:"exemptionApproval,attr"`
		ExemptionKind     string `xml:"exemptionKind,attr"`
		QuickfixInfo      string `xml:"quickfixInfo,attr"`
		Processor         string `xml:"processor,attr"`
		LastChangedBy     string `xml:"lastChangedBy,attr"`
		Tags              struct {
			Items []tag `xml:"tag"`
		} `xml:"tags"`
	}
	type object struct {
		URI         string `xml:"uri,attr"`
		Type        string `xml:"type,attr"`
		Name        string `xml:"name,attr"`
		PackageName string `xml:"packageName,attr"`
		Author      string `xml:"author,attr"`
		Findings    struct {
			Items []finding `xml:"finding"`
		} `xml:"findings"`
	}
	type objectSet struct {
		Name  string `xml:"name,attr"`
		Title string `xml:"title,attr"`
		Kind  string `xml:"kind,attr"`
	}
	type worklist struct {
		ID                  string `xml:"id,attr"`
		Timestamp           string `xml:"timestamp,attr"`
		UsedObjectSet       string `xml:"usedObjectSet,attr"`
		ObjectSetIsComplete string `xml:"objectSetIsComplete,attr"`
		ObjectSets          struct {
			Items []objectSet `xml:"objectSet"`
		} `xml:"objectSets"`
		Objects struct {
			Items []object `xml:"object"`
		} `xml:"objects"`
	}

	var resp worklist
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing ATC worklist: %w", err)
	}

	result := &ATCWorklist{
		ID:                  resp.ID,
		UsedObjectSet:       resp.UsedObjectSet,
		ObjectSetIsComplete: resp.ObjectSetIsComplete == "true",
		ObjectSets:          []ATCObjectSet{},
		Objects:             []ATCObject{},
	}

	for _, os := range resp.ObjectSets.Items {
		result.ObjectSets = append(result.ObjectSets, ATCObjectSet{
			Name:  os.Name,
			Title: os.Title,
			Kind:  os.Kind,
		})
	}

	for _, o := range resp.Objects.Items {
		obj := ATCObject{
			URI:         o.URI,
			Type:        o.Type,
			Name:        o.Name,
			PackageName: o.PackageName,
			Author:      o.Author,
			Findings:    []ATCFinding{},
		}

		for _, f := range o.Findings.Items {
			// Extract line and column from location fragment
			line, column := ParseLocationFragment(f.Location)

			finding := ATCFinding{
				URI:               f.URI,
				Location:          f.Location,
				Priority:          f.Priority,
				CheckID:           f.CheckID,
				CheckTitle:        f.CheckTitle,
				MessageID:         f.MessageID,
				MessageTitle:      f.MessageTitle,
				ExemptionApproval: f.ExemptionApproval,
				ExemptionKind:     f.ExemptionKind,
				QuickfixInfo:      f.QuickfixInfo,
				Line:              line,
				Column:            column,
				Processor:         f.Processor,
				LastChangedBy:     f.LastChangedBy,
			}

			// Parse tags into map
			if len(f.Tags.Items) > 0 {
				finding.Tags = make(map[string]string, len(f.Tags.Items))
				for _, t := range f.Tags.Items {
					finding.Tags[t.Name] = t.Value
				}
			}

			obj.Findings = append(obj.Findings, finding)
		}

		result.Objects = append(result.Objects, obj)
	}

	return result, nil
}

// RunATCCheck is a convenience method that runs ATC check on an object and returns findings.
// It combines GetATCCheckVariant, CreateATCRun, and GetATCWorklist into a single call.
// variant can be empty to use the system default.
func (c *Client) RunATCCheck(ctx context.Context, objectURL string, variant string, maxResults int) (*ATCWorklist, error) {
	// Get worklist ID for the variant
	worklistID, err := c.GetATCCheckVariant(ctx, variant)
	if err != nil {
		return nil, fmt.Errorf("getting check variant: %w", err)
	}

	// Create the ATC run
	runResult, err := c.CreateATCRun(ctx, worklistID, objectURL, maxResults)
	if err != nil {
		return nil, fmt.Errorf("creating ATC run: %w", err)
	}

	// Get the worklist with findings
	worklist, err := c.GetATCWorklist(ctx, runResult.WorklistID, false)
	if err != nil {
		return nil, fmt.Errorf("getting ATC worklist: %w", err)
	}

	return worklist, nil
}
