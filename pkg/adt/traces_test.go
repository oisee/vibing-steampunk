package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// The fixtures are trimmed answers from a 7.58 system: the trace list with
// two traces cut off at their size limit and two finished hit-list traces,
// five entries of one hit list, and three database accesses.

func readTraceFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseTracesFeed_v758(t *testing.T) {
	traces, err := parseTracesFeed(readTraceFixture(t, "abaptraces-list.v758.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != 4 {
		t.Fatalf("got %d traces, want 4", len(traces))
	}

	cut := traces[0]
	if cut.ID != "6B952F02C3D211F1B8A20242AC110011" {
		t.Errorf("ID = %q, want the bare key", cut.ID)
	}
	if cut.URI != "/sap/bc/adt/runtime/traces/abaptraces/6B952F02C3D211F1B8A20242AC110011" {
		t.Errorf("URI = %q", cut.URI)
	}
	if cut.StartTime != "2026-10-09T11:13:14Z" {
		t.Errorf("StartTime = %q", cut.StartTime)
	}
	if cut.State != "S" || cut.StateText != "Size violation" || cut.Complete {
		t.Errorf("state = %q %q complete=%v, want S, Size violation, false", cut.State, cut.StateText, cut.Complete)
	}
	if cut.Aggregation != "none" || cut.SizeKB != 81927 || cut.Runtime != 1445521 {
		t.Errorf("aggregation %q size %d runtime %d", cut.Aggregation, cut.SizeKB, cut.Runtime)
	}
	if cut.Author != "TESTUSER" || cut.User != "SAPSYS" || cut.ObjectName != "ZABAPITI_REGISTRY_RUN" {
		t.Errorf("author %q user %q object %q", cut.Author, cut.User, cut.ObjectName)
	}

	if traces[1].Aggregation != "byCallStack" || traces[1].Complete {
		t.Errorf("second trace: aggregation %q complete %v", traces[1].Aggregation, traces[1].Complete)
	}
	if fin := traces[3]; fin.Aggregation != "byCallPosition" || !fin.Complete || fin.State != "R" {
		t.Errorf("last trace: aggregation %q state %q complete %v", fin.Aggregation, fin.State, fin.Complete)
	}
}

func TestParseTraceAnalysis_HitlistV758(t *testing.T) {
	a, err := parseTraceAnalysis(readTraceFixture(t, "abaptraces-hitlist.v758.xml"), "X", "hitlist")
	if err != nil {
		t.Fatal(err)
	}
	if a.TotalEntries != 5 || len(a.Entries) != 5 || a.Note != "" {
		t.Fatalf("entries %d/%d note %q", a.TotalEntries, len(a.Entries), a.Note)
	}
	for i, e := range a.Entries {
		if e.Event == "" || e.Program == "" || e.Calls == 0 {
			t.Errorf("entry %d is not filled: %+v", i, e)
		}
	}

	first := a.Entries[0]
	want := TraceEntry{
		Index:         781,
		Event:         "Append <Generic Identifier>",
		Program:       "Z_RUNTIME_ARRA_C22708FAF2E354=CP",
		CallingObject: "Z_RUNTIME_ARRA_C22708FAF2E354",
		CallingType:   "CLAS/OC",
		CallingURI:    "/sap/bc/adt/oo/classes/z_runtime_arra_c22708faf2e354/source/main#start=40",
		Line:          40,
		Calls:         26567871,
		GrossTime:     24865061,
		Percentage:    6.5639,
		NetTime:       24865061,
		NetPercentage: 6.5639,
	}
	if first != want {
		t.Errorf("first entry\n got %+v\nwant %+v", first, want)
	}

	method := a.Entries[1]
	if method.Event != "Call M. Z_SRC_ABAP_1_L_22309FB1998E15->Z_MEMBER_PROCE_57F9D1E4B4E34E" {
		t.Errorf("event = %q", method.Event)
	}
	if method.GrossTime != 82614482 || method.NetTime != 12393983 || method.CalledProgram != "Z_SRC_ABAP_1_L_22309FB1998E15=CP" {
		t.Errorf("method entry %+v", method)
	}
}

func TestParseTraceAnalysis_DBAccessesV758(t *testing.T) {
	a, err := parseTraceAnalysis(readTraceFixture(t, "abaptraces-dbaccesses.v758.xml"), "X", "dbAccesses")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(a.Entries))
	}
	tsp03 := a.Entries[1]
	if tsp03.TableName != "TSP03" || tsp03.Statement != "select single" || tsp03.Operation != "OpenSQL" ||
		tsp03.Calls != 1 || tsp03.BufferedCount != 1 || tsp03.GrossTime != 16 || tsp03.DBTime != 16 ||
		tsp03.CallingObject != "RSPO_TEST_LAYOUT" || tsp03.Line != 15 {
		t.Errorf("TSP03 entry %+v", tsp03)
	}
}

// An answer the parser does not know must say so, not pass for an empty trace.
func TestParseTraceAnalysis_UnknownRootSaysSo(t *testing.T) {
	a, err := parseTraceAnalysis([]byte(`<trc:somethingNew xmlns:trc="x"><trc:row a="1"/></trc:somethingNew>`), "X", "hitlist")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Entries) != 0 || !strings.Contains(a.Note, "somethingNew") {
		t.Errorf("entries %d note %q", len(a.Entries), a.Note)
	}

	a, err = parseTraceAnalysis([]byte(`<trc:hitlist xmlns:trc="x"><trc:row/><trc:row/><trc:row/><trc:row/><trc:row/></trc:hitlist>`), "X", "hitlist")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.Note, "no entries were recognised") {
		t.Errorf("note %q", a.Note)
	}
}

func TestNormalizeTraceID(t *testing.T) {
	const id = "81BE5D12C3E111F1B8A20242AC110011"
	for _, in := range []string{
		id,
		" " + id + " ",
		"/sap/bc/adt/runtime/traces/abaptraces/" + id,
		"/sap/bc/adt/runtime/traces/abaptraces/" + id + "/dbAccesses",
		"adt://DEV/sap/bc/adt/runtime/traces/abaptraces/" + id + "#traceTime=1791550873000",
	} {
		got, err := NormalizeTraceID(in)
		if err != nil || got != id {
			t.Errorf("NormalizeTraceID(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "abaptraces/", "..", "X%2F", "a b"} {
		if got, err := NormalizeTraceID(bad); err == nil {
			t.Errorf("NormalizeTraceID(%q) = %q, want an error", bad, got)
		}
	}
}

func TestExtractCallEdgesFromTrace_Hitlist(t *testing.T) {
	a, err := parseTraceAnalysis(readTraceFixture(t, "abaptraces-hitlist.v758.xml"), "X", "hitlist")
	if err != nil {
		t.Fatal(err)
	}
	edges := ExtractCallEdgesFromTrace(a.Entries)
	if len(edges) != 1 {
		t.Fatalf("got %d edges, want 1: %+v", len(edges), edges)
	}
	e := edges[0]
	if e.CallerName != "Z_SRC_ABAP_2_S_A60B8723A2AC62" || e.CalleeName != "Z_RUNTIME_ARRA_C22708FAF2E354" ||
		e.CalleeURI != "/sap/bc/adt/oo/classes/z_runtime_arra_c22708faf2e354" {
		t.Errorf("edge %+v", e)
	}
}

// traceServer answers the trace list entry and evaluation paths from the
// fixtures, and refuses the way 7.58 does.
func traceServer(t *testing.T) *Client {
	t.Helper()
	list := string(readTraceFixture(t, "abaptraces-list.v758.xml"))
	hitlist := readTraceFixture(t, "abaptraces-hitlist.v758.xml")

	entryFor := func(id string) string {
		for _, part := range strings.Split(list, "<atom:entry")[1:] {
			if strings.Contains(part, "<atom:id>/sap/bc/adt/runtime/traces/abaptraces/"+id+"</atom:id>") {
				part = strings.TrimSuffix(part, "</atom:feed>")
				return `<atom:entry xmlns:atom="http://www.w3.org/2005/Atom"` + part
			}
		}
		return ""
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, abapTracesPath+"/")
		id, tool, _ := strings.Cut(rest, "/")
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case strings.Contains(r.URL.Path, "/discovery"):
			w.Header().Set("X-CSRF-Token", "TOKEN")
		case r.URL.Path == abapTracesPath:
			_, _ = w.Write([]byte(list))
		case id == "6B952F02C3D211F1B8A20242AC110011" && tool == "dbAccesses":
			w.WriteHeader(http.StatusForbidden)
		case tool == "":
			if e := entryFor(id); e != "" {
				_, _ = w.Write([]byte(e))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		case id == "6B952F02C3D211F1B8A20242AC110011":
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			_, _ = w.Write([]byte(`<exc:exception xmlns:exc="http://www.sap.com/abapxml/types/communicationframework"><type id="AdtFailed"/><message lang="EN">An exception was raised</message><properties><entry key="T100KEY-ID">SY</entry><entry key="T100KEY-NO">530</entry></properties></exc:exception>`))
		case tool == "statements":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`<exc:exception xmlns:exc="http://www.sap.com/abapxml/types/communicationframework"><type id="ExceptionInvalidData"/><message lang="EN">Data is invalid and could not be converted</message><properties><entry key="com.sap.adt.communicationFramework.subType">invalidRequestForAggregatedTraces</entry></properties></exc:exception>`))
		case tool == "hitlist":
			_, _ = w.Write(hitlist)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := NewConfig(srv.URL, "TESTUSER", "secret")
	return NewClientWithTransport(cfg, NewTransport(cfg))
}

func TestGetTraceAnalysis_SortsByOwnTimeAndCuts(t *testing.T) {
	c := traceServer(t)
	a, err := c.GetTraceAnalysis(context.Background(),
		"adt://DEV/sap/bc/adt/runtime/traces/abaptraces/81BE5D12C3E111F1B8A20242AC110011#traceTime=1",
		TraceGetOptions{Top: 2})
	if err != nil {
		t.Fatal(err)
	}
	if a.TraceID != "81BE5D12C3E111F1B8A20242AC110011" || a.TotalEntries != 5 || len(a.Entries) != 2 || !a.Truncated {
		t.Fatalf("id %q total %d entries %d truncated %v", a.TraceID, a.TotalEntries, len(a.Entries), a.Truncated)
	}
	if a.SortedBy != "net" || a.Entries[0].NetTime != 24865061 || a.Entries[1].NetTime != 12393983 {
		t.Errorf("sorted by %q: %d, %d", a.SortedBy, a.Entries[0].NetTime, a.Entries[1].NetTime)
	}
	if a.Trace == nil || a.TotalTime != 378815184 || a.Trace.Aggregation != "byCallPosition" {
		t.Errorf("trace info %+v total %d", a.Trace, a.TotalTime)
	}
}

func TestGetTraceAnalysis_SizeViolationSaysSo(t *testing.T) {
	c := traceServer(t)
	_, err := c.GetTraceAnalysis(context.Background(), "6B952F02C3D211F1B8A20242AC110011", TraceGetOptions{})
	if err == nil {
		t.Fatal("want an error for a trace cut off at its size limit")
	}
	for _, want := range []string{"incomplete", "Size violation", "416"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not say %q: %v", want, err)
		}
	}
}

func TestGetTraceAnalysis_CallTreeOfAggregatedTrace(t *testing.T) {
	c := traceServer(t)
	_, err := c.GetTraceAnalysis(context.Background(), "81BE5D12C3E111F1B8A20242AC110011", TraceGetOptions{ToolType: "callTree"})
	if err == nil || !strings.Contains(err.Error(), "aggregated byCallPosition") || !strings.Contains(err.Error(), "use hitlist") {
		t.Errorf("err = %v", err)
	}
}

func TestGetTraceAnalysis_RefusesUnknownOptions(t *testing.T) {
	c := traceServer(t)
	if _, err := c.GetTraceAnalysis(context.Background(), "X1", TraceGetOptions{ToolType: "flamegraph"}); err == nil {
		t.Error("unknown tool type accepted")
	}
	if _, err := c.GetTraceAnalysis(context.Background(), "X1", TraceGetOptions{SortBy: "name"}); err == nil {
		t.Error("unknown sort accepted")
	}
}

// The feed is oldest first; a caller asking for one trace wants the latest.
func TestListTraces_NewestFirstBeforeTheCap(t *testing.T) {
	c := traceServer(t)
	traces, err := c.ListTraces(context.Background(), &TraceQueryOptions{MaxResults: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != 1 || traces[0].ID != "81BE5D12C3E111F1B8A20242AC110011" {
		t.Errorf("got %+v, want only the latest trace", traces)
	}
}

// A refusal that is not about the recording must not be blamed on it.
func TestGetTraceAnalysis_ForbiddenIsNotIncomplete(t *testing.T) {
	c := traceServer(t)
	_, err := c.GetTraceAnalysis(context.Background(), "6B952F02C3D211F1B8A20242AC110011", TraceGetOptions{ToolType: "dbAccesses"})
	if err == nil || strings.Contains(err.Error(), "incomplete") || !strings.Contains(err.Error(), "403") {
		t.Errorf("err = %v", err)
	}
}
