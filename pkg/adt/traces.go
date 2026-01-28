package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
)

// --- ABAP Profiler / Runtime Traces (ATRA) Operations ---

// ABAPTrace represents an ABAP runtime trace file.
type ABAPTrace struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	User        string `json:"user"`
	StartTime   string `json:"startTime"`
	EndTime     string `json:"endTime,omitempty"`
	Duration    int64  `json:"duration,omitempty"` // microseconds
	ProcessType string `json:"processType,omitempty"`
	ObjectType  string `json:"objectType,omitempty"`
	Status      string `json:"status,omitempty"`
	URI         string `json:"uri"`
}

// TraceAnalysis contains trace analysis results.
type TraceAnalysis struct {
	TraceID    string            `json:"traceId"`
	ToolType   string            `json:"toolType"` // hitlist, statements, dbAccesses
	TotalTime  int64             `json:"totalTime,omitempty"`
	TotalCalls int               `json:"totalCalls,omitempty"`
	Entries    []TraceEntry      `json:"entries,omitempty"`
	Summary    map[string]string `json:"summary,omitempty"`
}

// TraceEntry represents a single entry in trace analysis.
type TraceEntry struct {
	Program     string  `json:"program,omitempty"`
	Event       string  `json:"event,omitempty"`
	Line        int     `json:"line,omitempty"`
	GrossTime   int64   `json:"grossTime,omitempty"` // microseconds
	NetTime     int64   `json:"netTime,omitempty"`   // microseconds
	Calls       int     `json:"calls,omitempty"`
	Percentage  float64 `json:"percentage,omitempty"`
	Statement   string  `json:"statement,omitempty"`
	TableName   string  `json:"tableName,omitempty"`   // for dbAccesses
	Operation   string  `json:"operation,omitempty"`   // SELECT, INSERT, etc.
	RecordCount int     `json:"recordCount,omitempty"` // rows affected
}

// TraceQueryOptions configures the trace list query.
type TraceQueryOptions struct {
	User        string // Filter by user
	ProcessType string // Filter by process type
	ObjectType  string // Filter by object type
	MaxResults  int    // Maximum results (default 100)
}

// ListTraces retrieves a list of ABAP runtime traces.
func (c *Client) ListTraces(ctx context.Context, opts *TraceQueryOptions) ([]ABAPTrace, error) {
	if opts == nil {
		opts = &TraceQueryOptions{MaxResults: 100}
	}

	params := url.Values{}
	if opts.User != "" {
		params.Set("user", opts.User)
	}
	if opts.ProcessType != "" {
		params.Set("processType", opts.ProcessType)
	}
	if opts.ObjectType != "" {
		params.Set("objectType", opts.ObjectType)
	}
	if opts.MaxResults > 0 {
		params.Set("$top", fmt.Sprintf("%d", opts.MaxResults))
	}

	endpoint := "/sap/bc/adt/runtime/traces/abaptraces"
	if len(params) > 0 {
		endpoint = endpoint + "?" + params.Encode()
	}

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/atom+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("listing traces: %w", err)
	}

	return parseTracesFeed(resp.Body)
}

// GetTrace retrieves analysis of a specific trace.
// toolType can be: "hitlist", "statements", "dbAccesses"
func (c *Client) GetTrace(ctx context.Context, traceID string, toolType string) (*TraceAnalysis, error) {
	if toolType == "" {
		toolType = "hitlist"
	}

	endpoint := fmt.Sprintf("/sap/bc/adt/runtime/traces/abaptraces/%s/%s", traceID, toolType)

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting trace %s (%s): %w", traceID, toolType, err)
	}

	return parseTraceAnalysis(resp.Body, traceID, toolType)
}

// traceEntryXML is used for parsing trace feed entries.
type traceEntryXML struct {
	ID      string `xml:"id"`
	Title   string `xml:"title"`
	Summary string `xml:"summary"`
	Updated string `xml:"updated"`
	Link    struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Author struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Content struct {
		Trace struct {
			StartTime   string `xml:"startTime,attr"`
			EndTime     string `xml:"endTime,attr"`
			Duration    string `xml:"duration,attr"`
			ProcessType string `xml:"processType,attr"`
			ObjectType  string `xml:"objectType,attr"`
			Status      string `xml:"status,attr"`
		} `xml:"trace"`
	} `xml:"content"`
}

// parseTracesFeed parses the Atom feed of traces.
func parseTracesFeed(data []byte) ([]ABAPTrace, error) {
	type feedXML struct {
		XMLName xml.Name        `xml:"feed"`
		Entries []traceEntryXML `xml:"entry"`
	}

	var feed feedXML
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parsing traces feed: %w", err)
	}

	result := make([]ABAPTrace, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		var duration int64
		if entry.Content.Trace.Duration != "" {
			_, _ = fmt.Sscanf(entry.Content.Trace.Duration, "%d", &duration)
		}

		trace := ABAPTrace{
			ID:          entry.ID,
			Title:       entry.Title,
			Description: entry.Summary,
			User:        entry.Author.Name,
			StartTime:   entry.Content.Trace.StartTime,
			EndTime:     entry.Content.Trace.EndTime,
			Duration:    duration,
			ProcessType: entry.Content.Trace.ProcessType,
			ObjectType:  entry.Content.Trace.ObjectType,
			Status:      entry.Content.Trace.Status,
			URI:         entry.Link.Href,
		}
		result = append(result, trace)
	}

	return result, nil
}

// parseTraceAnalysis parses trace analysis XML response.
func parseTraceAnalysis(data []byte, traceID, toolType string) (*TraceAnalysis, error) {
	analysis := &TraceAnalysis{
		TraceID:  traceID,
		ToolType: toolType,
		Summary:  make(map[string]string),
	}

	// The XML structure varies by tool type
	// For now, we extract basic information
	type hitlistXML struct {
		XMLName   xml.Name `xml:"hitlist"`
		TotalTime string   `xml:"totalTime,attr"`
		Entries   []struct {
			Program    string `xml:"program,attr"`
			Event      string `xml:"event,attr"`
			Line       string `xml:"line,attr"`
			GrossTime  string `xml:"grossTime,attr"`
			NetTime    string `xml:"netTime,attr"`
			Calls      string `xml:"calls,attr"`
			Percentage string `xml:"percentage,attr"`
		} `xml:"entry"`
	}

	var hitlist hitlistXML
	if err := xml.Unmarshal(data, &hitlist); err == nil && hitlist.XMLName.Local == "hitlist" {
		if hitlist.TotalTime != "" {
			_, _ = fmt.Sscanf(hitlist.TotalTime, "%d", &analysis.TotalTime)
		}

		for _, e := range hitlist.Entries {
			var line, calls int
			var grossTime, netTime int64
			var percentage float64

			_, _ = fmt.Sscanf(e.Line, "%d", &line)
			_, _ = fmt.Sscanf(e.Calls, "%d", &calls)
			_, _ = fmt.Sscanf(e.GrossTime, "%d", &grossTime)
			_, _ = fmt.Sscanf(e.NetTime, "%d", &netTime)
			_, _ = fmt.Sscanf(e.Percentage, "%f", &percentage)

			analysis.Entries = append(analysis.Entries, TraceEntry{
				Program:    e.Program,
				Event:      e.Event,
				Line:       line,
				GrossTime:  grossTime,
				NetTime:    netTime,
				Calls:      calls,
				Percentage: percentage,
			})
			analysis.TotalCalls += calls
		}
	}

	return analysis, nil
}
