package adt

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// --- Short Dumps / Runtime Errors (RABAX) Operations ---
//
// The client methods that lived here — GetDumps, GetDump, DumpQueryOptions and
// the RuntimeDump/DumpDetails types — are gone, and none of them worked as
// advertised. GetDumps built an OData $filter that /sap/bc/adt/runtime/dumps
// ignores: checked on 7.58, a filter naming a user and a package that exist
// nowhere still returns every dump on the system, so everything but $top was
// decoration. Its feed parser read one Atom category and left the error type,
// the program and the user empty. GetDump asked for the dump as HTML and
// returned the whole page — 45 KB to nearly a megabyte — with only the <title>
// pulled out of it.
//
// The replacements are in this package and are structured rather than hopeful:
// Dumps + DumpFilter (dumps.go) for the listing, GroupDumps for what keeps
// failing, DumpDetail (dumpdetail.go) and DumpStack (dumpstack.go) for one
// dump, CorrelateDump (correlate.go) for the log around it.

// --- ABAP Profiler / Runtime Traces (ATRA) Operations ---
//
// The shapes below were read from a 7.58 system (fixtures in
// testdata/abaptraces-*.v758.xml). The earlier parser expected a flat
// <hitlist><entry program= event= netTime=...> that ADT does not send: every
// hit-list entry came back as {} and the trace list lost its start time, state
// and aggregation, which are exactly the fields that tell a usable trace from
// one cut off at its size limit.

// ABAPTrace is one trace file from /sap/bc/adt/runtime/traces/abaptraces.
type ABAPTrace struct {
	ID          string `json:"id"`  // the bare trace key, what GetTrace takes
	URI         string `json:"uri"` // the ADT path of the trace
	Title       string `json:"title"`
	Author      string `json:"author,omitempty"`     // who requested the trace
	User        string `json:"user,omitempty"`       // whose work was traced (SAPSYS for a batch job)
	ObjectName  string `json:"objectName,omitempty"` // the traced program, transaction, URL...
	StartTime   string `json:"startTime,omitempty"`
	Expiration  string `json:"expiration,omitempty"`
	Host        string `json:"host,omitempty"`
	System      string `json:"system,omitempty"`
	Client      string `json:"client,omitempty"`
	State       string `json:"state,omitempty"`     // R finished, S size limit hit, ...
	StateText   string `json:"stateText,omitempty"` // as SAP words it
	Complete    bool   `json:"complete"`            // state R; anything else ADT may refuse to evaluate
	Aggregation string `json:"aggregation"`         // none, byCallPosition (hit list), byCallStack
	SizeKB      int64  `json:"sizeKB,omitempty"`
	Runtime     int64  `json:"runtime,omitempty"` // microseconds, the whole traced run
	RuntimeABAP int64  `json:"runtimeABAP,omitempty"`
	RuntimeDB   int64  `json:"runtimeDB,omitempty"`
	RuntimeSys  int64  `json:"runtimeSystem,omitempty"`
}

// TraceAnalysis is one evaluation of a trace: its hit list, call tree or
// database accesses.
type TraceAnalysis struct {
	TraceID      string       `json:"traceId"`
	ToolType     string       `json:"toolType"` // hitlist, statements, dbAccesses
	Trace        *ABAPTrace   `json:"trace,omitempty"`
	TotalTime    int64        `json:"totalTime,omitempty"` // microseconds, the traced run
	TotalDBTime  int64        `json:"totalDbTime,omitempty"`
	TotalCalls   int64        `json:"totalCalls,omitempty"`
	TotalEntries int          `json:"totalEntries"`
	SortedBy     string       `json:"sortedBy,omitempty"`
	Truncated    bool         `json:"truncated,omitempty"` // Entries holds the top N of TotalEntries
	Entries      []TraceEntry `json:"entries,omitempty"`
	Note         string       `json:"note,omitempty"`
}

// TraceEntry is one line of a hit list, call tree or database access list.
// Times are microseconds; percentages are of the whole traced run.
type TraceEntry struct {
	Index          int     `json:"index,omitempty"`
	Event          string  `json:"event,omitempty"` // "Call M. ZCL_X->RUN", "Append <...>", ...
	Program        string  `json:"program,omitempty"`
	CallingObject  string  `json:"callingObject,omitempty"`
	CallingType    string  `json:"callingType,omitempty"`
	CallingURI     string  `json:"callingUri,omitempty"`
	Line           int     `json:"line,omitempty"`
	CalledProgram  string  `json:"calledProgram,omitempty"`
	Calls          int64   `json:"calls,omitempty"`
	GrossTime      int64   `json:"grossTime,omitempty"`
	Percentage     float64 `json:"percentage,omitempty"`
	NetTime        int64   `json:"netTime,omitempty"` // own time
	NetPercentage  float64 `json:"netPercentage,omitempty"`
	RecursionDepth int     `json:"recursionDepth,omitempty"`
	CallLevel      int     `json:"callLevel,omitempty"` // call tree only
	Statement      string  `json:"statement,omitempty"` // dbAccesses: "select single", ...
	TableName      string  `json:"tableName,omitempty"`
	Operation      string  `json:"operation,omitempty"` // dbAccesses: OpenSQL, NativeSQL, ...
	BufferedCount  int64   `json:"bufferedCount,omitempty"`
	DBTime         int64   `json:"dbTime,omitempty"`
}

// TraceQueryOptions configures the trace list query.
type TraceQueryOptions struct {
	User        string // Filter by user
	ProcessType string // Filter by process type
	ObjectType  string // Filter by object type
	MaxResults  int    // Maximum results (default 100)
}

// TraceGetOptions configures one trace evaluation.
type TraceGetOptions struct {
	// ToolType is hitlist (the default), statements (the call tree; a
	// non-aggregated trace only) or dbAccesses.
	ToolType string
	// SortBy is net (own time; the default, except for a call tree, which
	// keeps SAP's order), gross, calls, or none for SAP's order.
	SortBy string
	// Top keeps the first N entries after sorting; 0 keeps all of them. A hit
	// list of a long run has tens of thousands of positions.
	Top int
}

const abapTracesPath = "/sap/bc/adt/runtime/traces/abaptraces"

// NormalizeTraceID accepts a trace key, its ADT path or an adt:// link and
// returns the bare key.
func NormalizeTraceID(s string) (string, error) {
	id := strings.TrimSpace(s)
	if i := strings.Index(id, "abaptraces/"); i >= 0 {
		id = id[i+len("abaptraces/"):]
	}
	if i := strings.IndexAny(id, "/#?"); i >= 0 {
		id = id[:i]
	}
	if id == "" {
		return "", fmt.Errorf("no trace id in %q", s)
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
			return "", fmt.Errorf("trace id %q: only letters and digits are expected", id)
		}
	}
	return id, nil
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

	endpoint := abapTracesPath
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

	traces, err := parseTracesFeed(resp.Body)
	if err != nil {
		return nil, err
	}
	// The feed comes oldest first and the collection ignores $top, so the
	// order and the cap are applied here: callers asking for one want the
	// latest.
	sort.SliceStable(traces, func(i, j int) bool { return traces[i].StartTime > traces[j].StartTime })
	if opts.MaxResults > 0 && len(traces) > opts.MaxResults {
		traces = traces[:opts.MaxResults]
	}
	return traces, nil
}

// GetTraceInfo reads one trace's list entry: state, aggregation, runtime.
func (c *Client) GetTraceInfo(ctx context.Context, traceID string) (*ABAPTrace, error) {
	id, err := NormalizeTraceID(traceID)
	if err != nil {
		return nil, err
	}
	resp, err := c.transport.Request(ctx, abapTracesPath+"/"+id, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/atom+xml;type=entry",
	})
	if err != nil {
		return nil, fmt.Errorf("reading trace %s: %w", id, err)
	}
	var e traceEntryXML
	if err := xml.Unmarshal(resp.Body, &e); err != nil {
		return nil, fmt.Errorf("parsing trace %s: %w", id, err)
	}
	t := e.toTrace()
	return &t, nil
}

// GetTrace retrieves the whole evaluation of a trace.
// toolType can be: "hitlist", "statements", "dbAccesses"
func (c *Client) GetTrace(ctx context.Context, traceID string, toolType string) (*TraceAnalysis, error) {
	return c.GetTraceAnalysis(ctx, traceID, TraceGetOptions{ToolType: toolType, SortBy: "none"})
}

// GetTraceAnalysis evaluates a trace, sorted and cut to opts.Top.
func (c *Client) GetTraceAnalysis(ctx context.Context, traceID string, opts TraceGetOptions) (*TraceAnalysis, error) {
	id, toolType, err := traceRequest(traceID, opts.ToolType)
	if err != nil {
		return nil, err
	}
	sortBy, err := traceSortKey(opts.SortBy, toolType)
	if err != nil {
		return nil, err
	}

	// The list entry carries the run's total time and, when the evaluation
	// fails, the reason: a trace cut off at its size limit is answered with a
	// bare 416 "An exception was raised" (SY 530).
	info, infoErr := c.GetTraceInfo(ctx, id)

	body, err := c.getTraceBody(ctx, id, toolType)
	if err != nil {
		return nil, explainTraceError(err, id, toolType, info)
	}

	analysis, err := parseTraceAnalysis(body, id, toolType)
	if err != nil {
		return nil, err
	}
	if infoErr == nil {
		analysis.Trace = info
		analysis.TotalTime = info.Runtime
	}
	sortTraceEntries(analysis.Entries, sortBy)
	if sortBy != "none" {
		analysis.SortedBy = sortBy
	}
	if opts.Top > 0 && len(analysis.Entries) > opts.Top {
		analysis.Entries = analysis.Entries[:opts.Top]
		analysis.Truncated = true
	}
	return analysis, nil
}

// GetTraceRaw returns the evaluation exactly as ADT sent it, for when the
// parser has not met the shape a release answers with.
func (c *Client) GetTraceRaw(ctx context.Context, traceID string, toolType string) ([]byte, error) {
	id, toolType, err := traceRequest(traceID, toolType)
	if err != nil {
		return nil, err
	}
	body, err := c.getTraceBody(ctx, id, toolType)
	if err != nil {
		info, _ := c.GetTraceInfo(ctx, id)
		return nil, explainTraceError(err, id, toolType, info)
	}
	return body, nil
}

func (c *Client) getTraceBody(ctx context.Context, id, toolType string) ([]byte, error) {
	resp, err := c.transport.Request(ctx, abapTracesPath+"/"+id+"/"+toolType, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// traceRequest validates the id and maps the tool type to its ADT path segment.
func traceRequest(traceID, toolType string) (string, string, error) {
	id, err := NormalizeTraceID(traceID)
	if err != nil {
		return "", "", err
	}
	switch strings.ToLower(toolType) {
	case "", "hitlist":
		return id, "hitlist", nil
	case "statements", "calltree", "call_tree":
		return id, "statements", nil
	case "dbaccesses", "db_accesses", "db":
		return id, "dbAccesses", nil
	}
	return "", "", fmt.Errorf("unknown trace tool type %q: use hitlist, statements (call tree) or dbAccesses", toolType)
}

func traceSortKey(sortBy, toolType string) (string, error) {
	switch strings.ToLower(sortBy) {
	case "":
		if toolType == "statements" {
			return "none", nil // a call tree reads in its own order
		}
		return "net", nil
	case "net", "own", "own_time":
		return "net", nil
	case "gross", "total", "total_time":
		return "gross", nil
	case "calls", "executions", "hits":
		return "calls", nil
	case "none":
		return "none", nil
	}
	return "", fmt.Errorf("unknown sort %q: use net (own time), gross, calls or none", sortBy)
}

func sortTraceEntries(entries []TraceEntry, by string) {
	var key func(e TraceEntry) int64
	switch by {
	case "net":
		// A database access has no own time apart from its access time.
		key = func(e TraceEntry) int64 {
			if e.Statement != "" || e.TableName != "" {
				return e.GrossTime
			}
			return e.NetTime
		}
	case "gross":
		key = func(e TraceEntry) int64 { return e.GrossTime }
	case "calls":
		key = func(e TraceEntry) int64 { return e.Calls }
	default:
		return
	}
	sort.SliceStable(entries, func(i, j int) bool { return key(entries[i]) > key(entries[j]) })
}

// explainTraceError turns ADT's answers for a trace it will not evaluate into
// the reason. info may be nil.
func explainTraceError(err error, id, toolType string, info *ABAPTrace) error {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("getting trace %s (%s): %w", id, toolType, err)
	}
	recordingRefused := apiErr.StatusCode == http.StatusRequestedRangeNotSatisfiable ||
		apiErr.StatusCode == http.StatusBadRequest || apiErr.StatusCode >= 500
	if info != nil && !info.Complete && recordingRefused {
		return fmt.Errorf("trace %s is incomplete (state %s %q): ADT cannot evaluate it, and Eclipse reports it as \"Trace has errors\". "+
			"Record it again with Hit List aggregation, or a larger maximum file size: %w", id, info.State, info.StateText, err)
	}
	if apiErr.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		return fmt.Errorf("trace %s cannot be evaluated (416, usually a trace cut off at its size limit): %w", id, err)
	}
	if strings.Contains(apiErr.Message, "invalidRequestForAggregatedTraces") {
		kind := "aggregated"
		if info != nil {
			kind = "aggregated " + info.Aggregation
		}
		return fmt.Errorf("trace %s is %s, and %s needs a trace recorded without aggregation; use hitlist: %w", id, kind, toolType, err)
	}
	return fmt.Errorf("getting trace %s (%s): %w", id, toolType, err)
}

// traceEntryXML is one Atom entry of the trace list. Element names match by
// local name, so the atom: and trc: prefixes need no namespace here.
type traceEntryXML struct {
	ID        string `xml:"id"`
	Title     string `xml:"title"`
	Published string `xml:"published"`
	Author    struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Ext struct {
		Host            string `xml:"host"`
		Size            int64  `xml:"size"`
		Runtime         int64  `xml:"runtime"`
		RuntimeABAP     int64  `xml:"runtimeABAP"`
		RuntimeSystem   int64  `xml:"runtimeSystem"`
		RuntimeDatabase int64  `xml:"runtimeDatabase"`
		Expiration      string `xml:"expiration"`
		System          string `xml:"system"`
		Client          string `xml:"client"`
		User            string `xml:"user"`
		IsAggregated    bool   `xml:"isAggregated"`
		AggregationKind string `xml:"aggregationKind"`
		ObjectName      string `xml:"objectName"`
		State           struct {
			Value string `xml:"value,attr"`
			Text  string `xml:"text,attr"`
		} `xml:"state"`
	} `xml:"extendedData"`
}

func (e traceEntryXML) toTrace() ABAPTrace {
	id, _ := NormalizeTraceID(e.ID)
	agg := "none"
	if e.Ext.IsAggregated {
		agg = e.Ext.AggregationKind
		if agg == "" {
			agg = "aggregated"
		}
	}
	return ABAPTrace{
		ID:          id,
		URI:         e.ID,
		Title:       e.Title,
		Author:      e.Author.Name,
		User:        e.Ext.User,
		ObjectName:  e.Ext.ObjectName,
		StartTime:   e.Published,
		Expiration:  e.Ext.Expiration,
		Host:        e.Ext.Host,
		System:      e.Ext.System,
		Client:      e.Ext.Client,
		State:       e.Ext.State.Value,
		StateText:   e.Ext.State.Text,
		Complete:    e.Ext.State.Value == "R",
		Aggregation: agg,
		SizeKB:      e.Ext.Size,
		Runtime:     e.Ext.Runtime,
		RuntimeABAP: e.Ext.RuntimeABAP,
		RuntimeDB:   e.Ext.RuntimeDatabase,
		RuntimeSys:  e.Ext.RuntimeSystem,
	}
}

// parseTracesFeed parses the Atom feed of traces.
func parseTracesFeed(data []byte) ([]ABAPTrace, error) {
	var feed struct {
		XMLName xml.Name        `xml:"feed"`
		Entries []traceEntryXML `xml:"entry"`
	}
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parsing traces feed: %w", err)
	}
	result := make([]ABAPTrace, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		result = append(result, entry.toTrace())
	}
	return result, nil
}

type traceProgramXML struct {
	Context string `xml:"context,attr"` // the main program, ZCL_X=====CP
	Name    string `xml:"name,attr"`    // the object, when ADT resolved one
	Type    string `xml:"type,attr"`
	URI     string `xml:"uri,attr"`
}

type traceTimeXML struct {
	Time       int64   `xml:"time,attr"`
	Percentage float64 `xml:"percentage,attr"`
}

// traceCallXML is a hit-list entry and a call-tree statement alike: both carry
// the calling and called program and the three times.
type traceCallXML struct {
	Index          int             `xml:"index,attr"`
	HitCount       int64           `xml:"hitCount,attr"`
	RecursionDepth int             `xml:"recursionDepth,attr"`
	CallLevel      int             `xml:"callLevel,attr"`
	Description    string          `xml:"description,attr"`
	Calling        traceProgramXML `xml:"callingProgram"`
	Called         traceProgramXML `xml:"calledProgram"`
	Gross          traceTimeXML    `xml:"grossTime"`
	Net            traceTimeXML    `xml:"traceEventNetTime"`
}

func (x traceCallXML) toEntry() TraceEntry {
	return TraceEntry{
		Index:          x.Index,
		Event:          strings.Join(strings.Fields(x.Description), " "),
		Program:        x.Calling.Context,
		CallingObject:  x.Calling.Name,
		CallingType:    x.Calling.Type,
		CallingURI:     x.Calling.URI,
		Line:           lineFromADTURI(x.Calling.URI),
		CalledProgram:  x.Called.Context,
		Calls:          x.HitCount,
		GrossTime:      x.Gross.Time,
		Percentage:     x.Gross.Percentage,
		NetTime:        x.Net.Time,
		NetPercentage:  x.Net.Percentage,
		RecursionDepth: x.RecursionDepth,
		CallLevel:      x.CallLevel,
	}
}

// lineFromADTURI reads the line from a source URI's #start=LINE[,COL].
func lineFromADTURI(uri string) int {
	i := strings.Index(uri, "#start=")
	if i < 0 {
		return 0
	}
	s := uri[i+len("#start="):]
	if j := strings.IndexAny(s, ",;&"); j >= 0 {
		s = s[:j]
	}
	n, _ := strconv.Atoi(s)
	return n
}

// parseTraceAnalysis parses an evaluation by its root element, not by what
// was asked for, and says so when it meets a root it does not know: an empty
// result must not pass for an empty trace.
func parseTraceAnalysis(data []byte, traceID, toolType string) (*TraceAnalysis, error) {
	analysis := &TraceAnalysis{TraceID: traceID, ToolType: toolType}

	var doc struct {
		XMLName     xml.Name
		TotalDBTime int64          `xml:"totalDbTime,attr"`
		Calls       []traceCallXML `xml:"entry"`
		Statements  []traceCallXML `xml:"statement"`
		DBAccesses  []struct {
			Index         int             `xml:"index,attr"`
			TableName     string          `xml:"tableName,attr"`
			Statement     string          `xml:"statement,attr"`
			Type          string          `xml:"type,attr"`
			TotalCount    int64           `xml:"totalCount,attr"`
			BufferedCount int64           `xml:"bufferedCount,attr"`
			Calling       traceProgramXML `xml:"callingProgram"`
			AccessTime    struct {
				Total    int64   `xml:"total,attr"`
				Database int64   `xml:"database,attr"`
				Ratio    float64 `xml:"ratioOfTraceTotal,attr"`
			} `xml:"accessTime"`
		} `xml:"dbAccess"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing trace %s (%s): %w", traceID, toolType, err)
	}

	switch doc.XMLName.Local {
	case "hitlist":
		for _, x := range doc.Calls {
			analysis.Entries = append(analysis.Entries, x.toEntry())
		}
	case "statements":
		for _, x := range doc.Statements {
			analysis.Entries = append(analysis.Entries, x.toEntry())
		}
	case "dbAccesses":
		analysis.TotalDBTime = doc.TotalDBTime
		for _, a := range doc.DBAccesses {
			analysis.Entries = append(analysis.Entries, TraceEntry{
				Index:         a.Index,
				TableName:     a.TableName,
				Statement:     a.Statement,
				Operation:     a.Type,
				Program:       a.Calling.Context,
				CallingObject: a.Calling.Name,
				CallingType:   a.Calling.Type,
				CallingURI:    a.Calling.URI,
				Line:          lineFromADTURI(a.Calling.URI),
				Calls:         a.TotalCount,
				BufferedCount: a.BufferedCount,
				GrossTime:     a.AccessTime.Total,
				DBTime:        a.AccessTime.Database,
				Percentage:    a.AccessTime.Ratio,
			})
		}
	default:
		analysis.Note = fmt.Sprintf("unrecognised response root <%s>; nothing was parsed. Ask for raw output to see what ADT sent", doc.XMLName.Local)
	}

	for _, e := range analysis.Entries {
		analysis.TotalCalls += e.Calls
	}
	analysis.TotalEntries = len(analysis.Entries)
	if analysis.TotalEntries == 0 && analysis.Note == "" && strings.Count(string(data), "<") > 2 {
		analysis.Note = "the response has content but no entries were recognised in it. Ask for raw output to see what ADT sent"
	}
	return analysis, nil
}
