package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
)

// --- SQL Trace (ST05) Operations ---

// SQLTraceState represents the current state of SQL tracing.
type SQLTraceState struct {
	Active     bool   `json:"active"`
	User       string `json:"user,omitempty"`
	TraceType  string `json:"traceType,omitempty"`
	StartTime  string `json:"startTime,omitempty"`
	MaxRecords int    `json:"maxRecords,omitempty"`
	TraceFile  string `json:"traceFile,omitempty"`
}

// SQLTraceEntry represents a trace file in the directory.
type SQLTraceEntry struct {
	ID          string `json:"id"`
	User        string `json:"user"`
	StartTime   string `json:"startTime"`
	EndTime     string `json:"endTime,omitempty"`
	TraceType   string `json:"traceType"`
	RecordCount int    `json:"recordCount"`
	Size        int64  `json:"size,omitempty"`
	URI         string `json:"uri"`
}

// GetSQLTraceState checks if SQL trace is currently active.
func (c *Client) GetSQLTraceState(ctx context.Context) (*SQLTraceState, error) {
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/st05/trace/state", &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting SQL trace state: %w", err)
	}

	return parseSQLTraceState(resp.Body)
}

// ListSQLTraces retrieves a list of SQL trace files.
func (c *Client) ListSQLTraces(ctx context.Context, user string, maxResults int) ([]SQLTraceEntry, error) {
	params := url.Values{}
	if user != "" {
		params.Set("user", user)
	}
	if maxResults > 0 {
		params.Set("$top", fmt.Sprintf("%d", maxResults))
	}

	endpoint := "/sap/bc/adt/st05/trace/directory"
	if len(params) > 0 {
		endpoint = endpoint + "?" + params.Encode()
	}

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/atom+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("listing SQL traces: %w", err)
	}

	return parseSQLTraceDirectory(resp.Body)
}

// parseSQLTraceState parses the SQL trace state XML.
func parseSQLTraceState(data []byte) (*SQLTraceState, error) {
	type stateXML struct {
		XMLName    xml.Name `xml:"traceState"`
		Active     string   `xml:"active,attr"`
		User       string   `xml:"user,attr"`
		TraceType  string   `xml:"traceType,attr"`
		StartTime  string   `xml:"startTime,attr"`
		MaxRecords string   `xml:"maxRecords,attr"`
		TraceFile  string   `xml:"traceFile,attr"`
	}

	var state stateXML
	if err := xml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing SQL trace state: %w", err)
	}

	var maxRecords int
	if state.MaxRecords != "" {
		_, _ = fmt.Sscanf(state.MaxRecords, "%d", &maxRecords)
	}

	return &SQLTraceState{
		Active:     state.Active == "true" || state.Active == "X",
		User:       state.User,
		TraceType:  state.TraceType,
		StartTime:  state.StartTime,
		MaxRecords: maxRecords,
		TraceFile:  state.TraceFile,
	}, nil
}

// sqlTraceEntryXML is used for parsing SQL trace directory.
type sqlTraceEntryXML struct {
	ID      string `xml:"id"`
	Title   string `xml:"title"`
	Updated string `xml:"updated"`
	Link    struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Author struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Content struct {
		Trace struct {
			TraceType   string `xml:"traceType,attr"`
			StartTime   string `xml:"startTime,attr"`
			EndTime     string `xml:"endTime,attr"`
			RecordCount string `xml:"recordCount,attr"`
			Size        string `xml:"size,attr"`
		} `xml:"trace"`
	} `xml:"content"`
}

// parseSQLTraceDirectory parses the SQL trace directory feed.
func parseSQLTraceDirectory(data []byte) ([]SQLTraceEntry, error) {
	type feedXML struct {
		XMLName xml.Name           `xml:"feed"`
		Entries []sqlTraceEntryXML `xml:"entry"`
	}

	var feed feedXML
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parsing SQL trace directory: %w", err)
	}

	result := make([]SQLTraceEntry, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		var recordCount int
		var size int64
		if entry.Content.Trace.RecordCount != "" {
			_, _ = fmt.Sscanf(entry.Content.Trace.RecordCount, "%d", &recordCount)
		}
		if entry.Content.Trace.Size != "" {
			_, _ = fmt.Sscanf(entry.Content.Trace.Size, "%d", &size)
		}

		trace := SQLTraceEntry{
			ID:          entry.ID,
			User:        entry.Author.Name,
			StartTime:   entry.Content.Trace.StartTime,
			EndTime:     entry.Content.Trace.EndTime,
			TraceType:   entry.Content.Trace.TraceType,
			RecordCount: recordCount,
			Size:        size,
			URI:         entry.Link.Href,
		}
		result = append(result, trace)
	}

	return result, nil
}
