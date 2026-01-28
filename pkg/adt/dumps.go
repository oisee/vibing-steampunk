package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Short Dumps / Runtime Errors (RABAX) Operations ---

// RuntimeDump represents an ABAP runtime error (short dump).
type RuntimeDump struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Category      string `json:"category"`
	ExceptionType string `json:"exceptionType"`
	Program       string `json:"program"`
	Include       string `json:"include,omitempty"`
	Line          int    `json:"line,omitempty"`
	User          string `json:"user"`
	Client        string `json:"client"`
	Host          string `json:"host,omitempty"`
	Timestamp     string `json:"timestamp"`
	URI           string `json:"uri"`
}

// DumpDetails contains full details of a runtime error.
type DumpDetails struct {
	RuntimeDump
	StackTrace   []StackFrame      `json:"stackTrace,omitempty"`
	Variables    []DumpVariable    `json:"variables,omitempty"`
	SourceCode   string            `json:"sourceCode,omitempty"`
	ErrorDetails map[string]string `json:"errorDetails,omitempty"`
	RawHTML      string            `json:"rawHtml,omitempty"`
}

// StackFrame represents a single frame in the stack trace.
type StackFrame struct {
	Program string `json:"program"`
	Include string `json:"include,omitempty"`
	Line    int    `json:"line"`
	Event   string `json:"event,omitempty"`
}

// DumpVariable represents a variable captured in the dump.
type DumpVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  string `json:"type,omitempty"`
}

// DumpQueryOptions configures the dump list query.
type DumpQueryOptions struct {
	User          string // Filter by user
	ExceptionType string // Filter by exception type (e.g., "CX_SY_ZERODIVIDE")
	Program       string // Filter by program name
	Package       string // Filter by package
	DateFrom      string // Date from (YYYYMMDD format)
	DateTo        string // Date to (YYYYMMDD format)
	MaxResults    int    // Maximum results (default 100)
}

// GetDumps retrieves a list of runtime errors (short dumps) from the SAP system.
func (c *Client) GetDumps(ctx context.Context, opts *DumpQueryOptions) ([]RuntimeDump, error) {
	if opts == nil {
		opts = &DumpQueryOptions{MaxResults: 100}
	}

	params := url.Values{}

	// Build FQL (Filter Query Language) filter
	var filters []string
	if opts.User != "" {
		filters = append(filters, fmt.Sprintf("user eq '%s'", opts.User))
	}
	if opts.ExceptionType != "" {
		filters = append(filters, fmt.Sprintf("exceptionType eq '%s'", opts.ExceptionType))
	}
	if opts.Program != "" {
		filters = append(filters, fmt.Sprintf("program eq '%s'", opts.Program))
	}
	if opts.Package != "" {
		filters = append(filters, fmt.Sprintf("package eq '%s'", opts.Package))
	}
	if opts.DateFrom != "" {
		filters = append(filters, fmt.Sprintf("datetime ge '%s000000'", opts.DateFrom))
	}
	if opts.DateTo != "" {
		filters = append(filters, fmt.Sprintf("datetime le '%s235959'", opts.DateTo))
	}

	if len(filters) > 0 {
		params.Set("$filter", strings.Join(filters, " and "))
	}

	if opts.MaxResults > 0 {
		params.Set("$top", fmt.Sprintf("%d", opts.MaxResults))
	}

	endpoint := "/sap/bc/adt/runtime/dumps"
	if len(params) > 0 {
		endpoint = endpoint + "?" + params.Encode()
	}

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/atom+xml;type=feed",
	})
	if err != nil {
		return nil, fmt.Errorf("getting dumps: %w", err)
	}

	return parseDumpsFeed(resp.Body)
}

// GetDump retrieves full details of a specific runtime error.
// dumpID can be either a full URI path (from GetDumps) or just the dump identifier.
func (c *Client) GetDump(ctx context.Context, dumpID string) (*DumpDetails, error) {
	var endpoint string
	// GetDumps returns IDs like /sap/bc/adt/vit/runtime/dumps/{id}
	// But the detail endpoint is /sap/bc/adt/runtime/dump/{id} (singular, no vit)
	if strings.Contains(dumpID, "/runtime/dumps/") {
		// Extract ID from full path and use correct endpoint
		parts := strings.Split(dumpID, "/runtime/dumps/")
		if len(parts) == 2 {
			endpoint = fmt.Sprintf("/sap/bc/adt/runtime/dump/%s", parts[1])
		} else {
			endpoint = fmt.Sprintf("/sap/bc/adt/runtime/dump/%s", dumpID)
		}
	} else if strings.HasPrefix(dumpID, "/sap/bc/adt/runtime/dump/") {
		// Already correct path
		endpoint = dumpID
	} else {
		// Just the ID - construct path
		endpoint = fmt.Sprintf("/sap/bc/adt/runtime/dump/%s", dumpID)
	}

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method: http.MethodGet,
		Accept: "text/html",
	})
	if err != nil {
		return nil, fmt.Errorf("getting dump %s: %w", dumpID, err)
	}

	return parseDumpDetails(resp.Body, dumpID)
}

// dumpEntryXML is used for parsing dump feed entries.
type dumpEntryXML struct {
	ID       string `xml:"id"`
	Title    string `xml:"title"`
	Updated  string `xml:"updated"`
	Category struct {
		Term string `xml:"term,attr"`
	} `xml:"category"`
	Link struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Content struct {
		Type   string `xml:"type,attr"`
		Source struct {
			Line    string `xml:"line,attr"`
			Program string `xml:"program,attr"`
			Include string `xml:"include,attr"`
		} `xml:"source"`
		Exception struct {
			Type string `xml:"type,attr"`
		} `xml:"exception"`
		Runtime struct {
			User   string `xml:"user,attr"`
			Client string `xml:"client,attr"`
			Host   string `xml:"host,attr"`
		} `xml:"runtime"`
	} `xml:"content"`
}

// parseDumpsFeed parses the Atom feed of runtime errors.
func parseDumpsFeed(data []byte) ([]RuntimeDump, error) {
	type feedXML struct {
		XMLName xml.Name       `xml:"feed"`
		Entries []dumpEntryXML `xml:"entry"`
	}

	var feed feedXML
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parsing dumps feed: %w", err)
	}

	result := make([]RuntimeDump, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		line := 0
		if entry.Content.Source.Line != "" {
			_, _ = fmt.Sscanf(entry.Content.Source.Line, "%d", &line)
		}

		dump := RuntimeDump{
			ID:            entry.ID,
			Title:         entry.Title,
			Category:      entry.Category.Term,
			ExceptionType: entry.Content.Exception.Type,
			Program:       entry.Content.Source.Program,
			Include:       entry.Content.Source.Include,
			Line:          line,
			User:          entry.Content.Runtime.User,
			Client:        entry.Content.Runtime.Client,
			Host:          entry.Content.Runtime.Host,
			Timestamp:     entry.Updated,
			URI:           entry.Link.Href,
		}
		result = append(result, dump)
	}

	return result, nil
}

// parseDumpDetails parses the HTML dump details response.
func parseDumpDetails(data []byte, dumpID string) (*DumpDetails, error) {
	html := string(data)

	details := &DumpDetails{
		RuntimeDump: RuntimeDump{
			ID: dumpID,
		},
		RawHTML: html,
	}

	// Extract basic info from HTML (simplified parsing)
	// The actual HTML structure varies, so we store the raw HTML
	// and extract what we can

	// Try to extract title
	if idx := strings.Index(html, "<title>"); idx >= 0 {
		endIdx := strings.Index(html[idx:], "</title>")
		if endIdx > 0 {
			details.Title = strings.TrimSpace(html[idx+7 : idx+endIdx])
		}
	}

	return details, nil
}
