package adt

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Package Operations ---

// PackageExists returns true if SAP has a package with the given name.
// It probes /sap/bc/adt/packages/{name} directly: 200 → exists, 404 → not,
// any other outcome (5xx, network, auth) is returned as an error so the
// caller does not silently classify a transient failure as "missing".
//
// 7.40 and 7.50 have no such resource: the router answers 404 for every
// name, $TMP included. That 404 carries no ADT exception document, and then
// TDEVC, read through data preview, gives the answer instead.
//
// Unlike GetPackage, which reads the nodestructure API and cannot distinguish
// "package does not exist" from "package exists but has no children" (both
// return an empty tree), this is a definitive existence check.
func (c *Client) PackageExists(ctx context.Context, packageName string) (bool, error) {
	if packageName == "" {
		return false, fmt.Errorf("empty package name")
	}
	objectURL := fmt.Sprintf("/sap/bc/adt/packages/%s", url.PathEscape(strings.ToUpper(packageName)))
	_, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/*",
	})
	if err == nil {
		return true, nil
	}
	if IsRouterNotFound(err) {
		res, qerr := c.GetTableContents(ctx, "TDEVC", 1,
			"SELECT DEVCLASS FROM TDEVC WHERE DEVCLASS = '"+strings.ReplaceAll(strings.ToUpper(packageName), "'", "''")+"'")
		if qerr != nil {
			return false, fmt.Errorf("no packages resource on this release, and TDEVC: %w", qerr)
		}
		return len(res.Rows) > 0, nil
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, err
}

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
