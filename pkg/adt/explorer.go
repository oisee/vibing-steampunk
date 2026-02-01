package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
)

// --- Object Explorer (CAI) Operations ---

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
		Accept: "application/vnd.sap.adt.cai.objectexplorer.v1+xml",
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
