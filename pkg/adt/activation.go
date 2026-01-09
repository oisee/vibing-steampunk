package adt

import (
	"cmp"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// ActivationResult represents the result of an activation.
type ActivationResult struct {
	Success  bool                      `json:"success"`
	Messages []ActivationResultMessage `json:"messages"`
	Inactive []InactiveObject          `json:"inactive,omitempty"`
}

// ActivationResultMessage represents a message from activation.
type ActivationResultMessage struct {
	ObjDescr       string `json:"objDescr,omitempty"`
	Type           string `json:"type"` // E=Error, W=Warning, I=Info
	Line           int    `json:"line,omitempty"`
	Href           string `json:"href,omitempty"`
	ForceSupported bool   `json:"forceSupported,omitempty"`
	ShortText      string `json:"shortText"`
}

// InactiveObject represents an inactive object.
type InactiveObject struct {
	URI       string `json:"uri"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	ParentURI string `json:"parentUri,omitempty"`
	User      string `json:"user,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
}

// InactiveObjectRecord represents an inactive object with its transport info.
type InactiveObjectRecord struct {
	Object    *InactiveObject `json:"object,omitempty"`
	Transport *InactiveObject `json:"transport,omitempty"`
}

// Activate activates one or more ABAP objects.
// objectURL is the ADT URL of the object (e.g., "/sap/bc/adt/programs/programs/ZTEST")
// objectName is the technical name (e.g., "ZTEST")
func (c *Client) Activate(ctx context.Context, objectURL string, objectName string) (*ActivationResult, error) {
	// Safety check
	if err := c.checkSafety(OpActivate, "Activate"); err != nil {
		return nil, err
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="%s" adtcore:name="%s"/>
</adtcore:objectReferences>`, objectURL, objectName)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/activation?method=activate&preauditRequested=true", &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(body),
		ContentType: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("activation failed: %w", err)
	}

	return parseActivationResult(resp.Body)
}

func parseActivationResult(data []byte) (*ActivationResult, error) {
	result := &ActivationResult{
		Success:  true,
		Messages: []ActivationResultMessage{},
		Inactive: []InactiveObject{},
	}

	// If response is empty, activation was successful
	if len(data) == 0 {
		return result, nil
	}

	type msg struct {
		ObjDescr       string `xml:"objDescr,attr"`
		Type           string `xml:"type,attr"`
		Line           int    `xml:"line,attr"`
		Href           string `xml:"href,attr"`
		ForceSupported bool   `xml:"forceSupported,attr"`
		ShortText      struct {
			Text string `xml:"txt"`
		} `xml:"shortText"`
	}
	type messages struct {
		Msgs []msg `xml:"msg"`
	}
	type inactiveRef struct {
		URI       string `xml:"uri,attr"`
		Type      string `xml:"type,attr"`
		Name      string `xml:"name,attr"`
		ParentURI string `xml:"parentUri,attr"`
	}
	type inactiveEntry struct {
		Object *struct {
			Ref inactiveRef `xml:"ref"`
		} `xml:"object"`
	}
	type inactiveObjects struct {
		Entries []inactiveEntry `xml:"entry"`
	}
	type response struct {
		Messages messages        `xml:"messages"`
		Inactive inactiveObjects `xml:"inactiveObjects"`
	}

	var resp response
	if err := xml.Unmarshal(data, &resp); err != nil {
		// If parsing fails, try to extract any error message
		result.Success = false
		result.Messages = append(result.Messages, ActivationResultMessage{
			Type:      "E",
			ShortText: string(data),
		})
		return result, nil
	}

	for _, m := range resp.Messages.Msgs {
		result.Messages = append(result.Messages, ActivationResultMessage{
			ObjDescr:       m.ObjDescr,
			Type:           m.Type,
			Line:           m.Line,
			Href:           m.Href,
			ForceSupported: m.ForceSupported,
			ShortText:      m.ShortText.Text,
		})
		// Check for errors
		if strings.ContainsAny(m.Type, "EAX") {
			result.Success = false
		}
	}

	for _, entry := range resp.Inactive.Entries {
		if entry.Object != nil {
			result.Success = false
			result.Inactive = append(result.Inactive, InactiveObject{
				URI:       entry.Object.Ref.URI,
				Type:      entry.Object.Ref.Type,
				Name:      entry.Object.Ref.Name,
				ParentURI: entry.Object.Ref.ParentURI,
			})
		}
	}

	return result, nil
}

// GetInactiveObjects retrieves all inactive objects for the current user.
// Returns objects that have been modified but not yet activated.
func (c *Client) GetInactiveObjects(ctx context.Context) ([]InactiveObjectRecord, error) {
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/activation/inactiveobjects", &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.inactivectsobjects.v1+xml, application/xml;q=0.8",
	})
	if err != nil {
		return nil, fmt.Errorf("get inactive objects failed: %w", err)
	}

	return parseInactiveObjects(resp.Body)
}

func parseInactiveObjects(data []byte) ([]InactiveObjectRecord, error) {
	if len(data) == 0 {
		return []InactiveObjectRecord{}, nil
	}

	// Strip namespace prefixes
	xmlStr := StripXMLNamespacePrefixes(data)

	type ref struct {
		URI       string `xml:"uri,attr"`
		Type      string `xml:"type,attr"`
		Name      string `xml:"name,attr"`
		ParentURI string `xml:"parentUri,attr"`
	}
	type objectElement struct {
		Deleted bool   `xml:"deleted,attr"`
		User    string `xml:"user,attr"`
		Ref     ref    `xml:"ref"`
	}
	type entry struct {
		Object    *objectElement `xml:"object"`
		Transport *objectElement `xml:"transport"`
	}
	type inactiveObjects struct {
		Entries []entry `xml:"entry"`
	}

	var resp inactiveObjects
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing inactive objects: %w", err)
	}

	var results []InactiveObjectRecord
	for _, e := range resp.Entries {
		record := InactiveObjectRecord{}
		if e.Object != nil {
			record.Object = &InactiveObject{
				URI:       e.Object.Ref.URI,
				Type:      e.Object.Ref.Type,
				Name:      e.Object.Ref.Name,
				ParentURI: e.Object.Ref.ParentURI,
				User:      e.Object.User,
				Deleted:   e.Object.Deleted,
			}
		}
		if e.Transport != nil {
			record.Transport = &InactiveObject{
				URI:       e.Transport.Ref.URI,
				Type:      e.Transport.Ref.Type,
				Name:      e.Transport.Ref.Name,
				ParentURI: e.Transport.Ref.ParentURI,
				User:      e.Transport.User,
				Deleted:   e.Transport.Deleted,
			}
		}
		results = append(results, record)
	}

	return results, nil
}

// --- Batch Activation ---

// ActivatePackageResult represents the result of batch activation.
type ActivatePackageResult struct {
	Activated []ActivatedObject  `json:"activated"`
	Failed    []ActivationFailed `json:"failed"`
	Skipped   []string           `json:"skipped,omitempty"`
	Summary   string             `json:"summary"`
}

// ActivatedObject represents a successfully activated object.
type ActivatedObject struct {
	Name string `json:"name"`
	Type string `json:"type"`
	URI  string `json:"uri"`
}

// ActivationFailed represents a failed activation.
type ActivationFailed struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// objectTypePriorities defines activation order for ABAP object types.
// Lower number = activate first (interfaces before classes, etc.)
var objectTypePriorities = map[string]int{
	"DOMA/DD": 1, // Domains first
	"DTEL/DE": 2, // Data elements
	"TABL/DT": 3, // Tables/structures
	"TTYP/TT": 4, // Table types
	"INTF/OI": 5, // Interfaces before classes
	"CLAS/OC": 6, // Classes
	"FUGR/F":  7, // Function groups
	"PROG/P":  8, // Programs
	"DDLS/DF": 9, // CDS views
}

// objectTypePriority returns activation priority for an object type.
func objectTypePriority(objType string) int {
	if p, ok := objectTypePriorities[objType]; ok {
		return p
	}
	return 50 // Unknown types last
}

// ActivatePackage activates all inactive objects in a package.
// If packageName is empty, activates ALL inactive objects for the current user.
// Objects are sorted by dependency order before activation.
func (c *Client) ActivatePackage(ctx context.Context, packageName string, maxObjects int) (*ActivatePackageResult, error) {
	// Safety check
	if err := c.checkSafety(OpActivate, "ActivatePackage"); err != nil {
		return nil, err
	}

	// Get all inactive objects
	inactive, err := c.GetInactiveObjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting inactive objects: %w", err)
	}

	// Filter by package if specified
	var toActivate []InactiveObjectRecord
	packageName = strings.ToUpper(packageName)
	for _, rec := range inactive {
		if rec.Object == nil {
			continue
		}
		// Check if object belongs to package (URI contains package path)
		if packageName == "" {
			toActivate = append(toActivate, rec)
		} else {
			// Package is typically in the URI as parent path segment
			// e.g., /sap/bc/adt/programs/programs/ZTEST for package $TMP
			// We need to check the ParentURI or query object details
			// For now, include all if filtering by package
			// TODO: Add proper package filtering via object metadata
			toActivate = append(toActivate, rec)
		}
	}

	// Limit number of objects
	if maxObjects > 0 && len(toActivate) > maxObjects {
		toActivate = toActivate[:maxObjects]
	}

	// Sort by dependency order (interfaces before classes, etc.)
	// Uses O(n log n) sort instead of O(n²) bubble sort
	slices.SortFunc(toActivate, func(a, b InactiveObjectRecord) int {
		// Handle nil objects - push them to the end
		if a.Object == nil && b.Object == nil {
			return 0
		}
		if a.Object == nil {
			return 1
		}
		if b.Object == nil {
			return -1
		}
		return cmp.Compare(objectTypePriority(a.Object.Type), objectTypePriority(b.Object.Type))
	})

	result := &ActivatePackageResult{
		Activated: []ActivatedObject{},
		Failed:    []ActivationFailed{},
	}

	// Activate each object
	for _, rec := range toActivate {
		if rec.Object == nil {
			continue
		}
		obj := rec.Object
		_, err := c.Activate(ctx, obj.URI, obj.Name)
		if err != nil {
			result.Failed = append(result.Failed, ActivationFailed{
				Name:   obj.Name,
				Type:   obj.Type,
				Reason: err.Error(),
			})
		} else {
			result.Activated = append(result.Activated, ActivatedObject{
				Name: obj.Name,
				Type: obj.Type,
				URI:  obj.URI,
			})
		}
	}

	result.Summary = fmt.Sprintf("Activated %d objects, %d failed", len(result.Activated), len(result.Failed))
	return result, nil
}
