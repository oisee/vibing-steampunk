package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// UI5 Filestore API endpoints
const (
	ui5FilestoreBase = "/sap/bc/adt/filestore/ui5-bsp/objects"
)

// UI5App represents a UI5/Fiori BSP application.
type UI5App struct {
	Name        string `xml:"name,attr" json:"name"`
	Type        string `xml:"type,attr" json:"type"`
	URI         string `xml:"uri,attr" json:"uri"`
	Description string `xml:"description,attr,omitempty" json:"description,omitempty"`
	Package     string `xml:"packageName,attr,omitempty" json:"package,omitempty"`
}

// UI5AppDetails contains detailed information about a UI5 application.
type UI5AppDetails struct {
	Name           string    `json:"name"`
	Description    string    `json:"description,omitempty"`
	Package        string    `json:"package,omitempty"`
	TransportLayer string    `json:"transportLayer,omitempty"`
	Files          []UI5File `json:"files,omitempty"`
	Links          []Link    `json:"-"`
}

// UI5File represents a file within a UI5 application.
type UI5File struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"` // "file" or "folder"
	Size        int64  `json:"size,omitempty"`
	ContentType string `json:"contentType,omitempty"`
}

// UI5AtomFeed wraps UI5 app search results in Atom format.
type UI5AtomFeed struct {
	XMLName xml.Name       `xml:"feed"`
	Entries []UI5AtomEntry `xml:"entry"`
}

// UI5AtomEntry represents a single UI5 app in Atom format.
type UI5AtomEntry struct {
	XMLName  xml.Name `xml:"entry"`
	ID       string   `xml:"id"`
	Title    string   `xml:"title"`
	Summary  string   `xml:"summary"`
	Category struct {
		Term string `xml:"term,attr"`
	} `xml:"category"`
}

// UI5Folder represents a folder in UI5 app structure.
type UI5Folder struct {
	XMLName xml.Name    `xml:"folder"`
	Name    string      `xml:"name,attr"`
	Path    string      `xml:"path,attr"`
	Folders []UI5Folder `xml:"folder,omitempty"`
	Files   []struct {
		Name        string `xml:"name,attr"`
		Path        string `xml:"path,attr"`
		ContentType string `xml:"contentType,attr,omitempty"`
		Size        int64  `xml:"size,attr,omitempty"`
	} `xml:"file,omitempty"`
}

// UI5Content represents the content structure of a UI5 app.
type UI5Content struct {
	XMLName xml.Name    `xml:"content"`
	Folders []UI5Folder `xml:"folder,omitempty"`
	Files   []struct {
		Name        string `xml:"name,attr"`
		Path        string `xml:"path,attr"`
		ContentType string `xml:"contentType,attr,omitempty"`
		Size        int64  `xml:"size,attr,omitempty"`
	} `xml:"file,omitempty"`
}

// UI5ListApps lists UI5/Fiori BSP applications.
// The query parameter supports wildcards (* for multiple chars).
func (c *Client) UI5ListApps(ctx context.Context, query string, maxResults int) ([]UI5App, error) {
	if err := c.checkSafety(OpRead, "UI5ListApps"); err != nil {
		return nil, err
	}

	if maxResults <= 0 {
		maxResults = 100
	}

	params := url.Values{}
	if query != "" {
		params.Set("name", query)
	}
	params.Set("maxResults", fmt.Sprintf("%d", maxResults))

	resp, err := c.transport.Request(ctx, ui5FilestoreBase, &RequestOptions{
		Method: http.MethodGet,
		Query:  params,
		Accept: "application/atom+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("listing UI5 apps: %w", err)
	}

	// Parse Atom feed response
	var feed UI5AtomFeed
	if err := xml.Unmarshal(resp.Body, &feed); err != nil {
		return nil, fmt.Errorf("parsing UI5 apps list: %w", err)
	}

	// Convert Atom entries to UI5App
	apps := make([]UI5App, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		apps = append(apps, UI5App{
			Name:        entry.Title,
			Description: entry.Summary,
			URI:         entry.ID,
		})
	}

	return apps, nil
}

// UI5GetApp retrieves details of a UI5/Fiori BSP application.
func (c *Client) UI5GetApp(ctx context.Context, appName string) (*UI5AppDetails, error) {
	if err := c.checkSafety(OpRead, "UI5GetApp"); err != nil {
		return nil, err
	}

	appName = strings.ToUpper(appName)

	// Parse the app metadata
	details := &UI5AppDetails{
		Name: appName,
	}

	// Get content structure
	contentPath := fmt.Sprintf("%s/%s/content", ui5FilestoreBase, url.PathEscape(appName))
	contentResp, err := c.transport.Request(ctx, contentPath, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/atom+xml",
	})
	if err == nil {
		var feed UI5AtomFeed
		if xml.Unmarshal(contentResp.Body, &feed) == nil {
			details.Files = extractFilesFromAtomFeed(&feed, appName)
		}
	}

	return details, nil
}

// extractFilesFromAtomFeed extracts file information from Atom feed.
func extractFilesFromAtomFeed(feed *UI5AtomFeed, appName string) []UI5File {
	var files []UI5File
	prefix := appName + "/"

	for _, entry := range feed.Entries {
		// Extract path from title (e.g., "ZDEMOABAP_CH/.classpath" -> "/.classpath")
		path := entry.Title
		if strings.HasPrefix(path, prefix) {
			path = "/" + path[len(prefix):]
		} else if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		fileType := "file"
		if entry.Category.Term == "folder" {
			fileType = "folder"
		}

		files = append(files, UI5File{
			Name: entry.Title,
			Path: path,
			Type: fileType,
		})
	}

	return files
}

// UI5GetFileContent retrieves the content of a specific file within a UI5 app.
// The filePath should be relative to the app root (e.g., "/.project", "/WebContent/index.html").
func (c *Client) UI5GetFileContent(ctx context.Context, appName, filePath string) ([]byte, error) {
	if err := c.checkSafety(OpRead, "UI5GetFileContent"); err != nil {
		return nil, err
	}

	appName = strings.ToUpper(appName)
	filePath = strings.TrimPrefix(filePath, "/")

	// URL structure is: /sap/bc/adt/filestore/ui5-bsp/objects/APPNAME%2fFILEPATH/content
	// The app name and file path are combined with encoded slash
	fullPath := appName + "/" + filePath
	contentPath := fmt.Sprintf("%s/%s/content", ui5FilestoreBase, url.PathEscape(fullPath))

	resp, err := c.transport.Request(ctx, contentPath, &RequestOptions{
		Method: http.MethodGet,
	})
	if err != nil {
		return nil, fmt.Errorf("getting file %s from UI5 app %s: %w", filePath, appName, err)
	}

	return resp.Body, nil
}

// UI5UploadFile uploads a single file to a UI5/Fiori BSP application.
func (c *Client) UI5UploadFile(ctx context.Context, appName, filePath string, content []byte, contentType string) error {
	if err := c.checkSafety(OpUpdate, "UI5UploadFile"); err != nil {
		return err
	}

	appName = strings.ToUpper(appName)
	filePath = strings.TrimPrefix(filePath, "/")

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// URL structure is: /sap/bc/adt/filestore/ui5-bsp/objects/APPNAME%2fFILEPATH/content
	fullPath := appName + "/" + filePath
	uploadPath := fmt.Sprintf("%s/%s/content", ui5FilestoreBase, url.PathEscape(fullPath))

	_, err := c.transport.Request(ctx, uploadPath, &RequestOptions{
		Method:      http.MethodPut,
		Body:        content,
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("uploading file %s to UI5 app %s: %w", filePath, appName, err)
	}

	return nil
}

// UI5DeleteFile deletes a file from a UI5/Fiori BSP application.
func (c *Client) UI5DeleteFile(ctx context.Context, appName, filePath string) error {
	if err := c.checkSafety(OpDelete, "UI5DeleteFile"); err != nil {
		return err
	}

	appName = strings.ToUpper(appName)
	filePath = strings.TrimPrefix(filePath, "/")

	// URL structure is: /sap/bc/adt/filestore/ui5-bsp/objects/APPNAME%2fFILEPATH
	fullPath := appName + "/" + filePath
	deletePath := fmt.Sprintf("%s/%s", ui5FilestoreBase, url.PathEscape(fullPath))

	_, err := c.transport.Request(ctx, deletePath, &RequestOptions{
		Method: http.MethodDelete,
	})
	if err != nil {
		return fmt.Errorf("deleting file %s from UI5 app %s: %w", filePath, appName, err)
	}

	return nil
}

// UI5CreateApp creates a new UI5/Fiori BSP application.
func (c *Client) UI5CreateApp(ctx context.Context, appName, description, packageName, transport string) error {
	if err := c.checkSafety(OpCreate, "UI5CreateApp"); err != nil {
		return err
	}

	if err := c.checkPackageSafety(packageName); err != nil {
		return err
	}

	appName = strings.ToUpper(appName)

	// Build XML payload for app creation
	xmlPayload := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<bsp:application xmlns:bsp="http://www.sap.com/adt/bsp"
    xmlns:adtcore="http://www.sap.com/adt/core"
    adtcore:name="%s"
    adtcore:description="%s"
    adtcore:packageName="%s">
</bsp:application>`, appName, escapeXMLAttr(description), packageName)

	params := url.Values{}
	if transport != "" {
		params.Set("corrNr", transport)
	}

	_, err := c.transport.Request(ctx, ui5FilestoreBase, &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		Body:        []byte(xmlPayload),
		ContentType: "application/xml",
	})
	if err != nil {
		return fmt.Errorf("creating UI5 app %s: %w", appName, err)
	}

	return nil
}

// UI5DeleteApp deletes a UI5/Fiori BSP application.
func (c *Client) UI5DeleteApp(ctx context.Context, appName, transport string) error {
	if err := c.checkSafety(OpDelete, "UI5DeleteApp"); err != nil {
		return err
	}

	appName = strings.ToUpper(appName)
	deletePath := fmt.Sprintf("%s/%s", ui5FilestoreBase, url.PathEscape(appName))

	params := url.Values{}
	if transport != "" {
		params.Set("corrNr", transport)
	}

	_, err := c.transport.Request(ctx, deletePath, &RequestOptions{
		Method: http.MethodDelete,
		Query:  params,
	})
	if err != nil {
		return fmt.Errorf("deleting UI5 app %s: %w", appName, err)
	}

	return nil
}

// escapeXMLAttr escapes special characters for XML attribute values.
func escapeXMLAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
