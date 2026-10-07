package adt

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeOrganizer records the bridge calls and answers each with a fixed result.
type fakeOrganizer struct {
	calls []struct {
		fm     string
		params map[string]any
	}
	result RFCResult
	// answers, by function, overrides result.
	answers map[string]RFCResult
}

func (f *fakeOrganizer) CallRFC(_ context.Context, fm string, params map[string]any) (*RFCResult, error) {
	f.calls = append(f.calls, struct {
		fm     string
		params map[string]any
	}{fm, params})
	r := f.result
	if a, ok := f.answers[fm]; ok {
		r = a
	}
	return &r, nil
}

// noHeader stands in for the E070 read where no task needs classifying.
func noHeader(context.Context, string) (map[string]any, error) {
	return nil, errors.New("no header expected")
}

func entriesRequest() *TransportDetails {
	return &TransportDetails{
		TransportSummary: TransportSummary{Number: "TR-REQ"},
		Tasks: []TransportTaskV2{
			{Number: "TR-OTHER", Owner: "SOMEONE", Status: "D", Objects: []TransportObjectV2{{PgmID: "R3TR", Type: "PROG", Name: "ZDEMO"}}},
			{Number: "TR-MINE", Owner: "TESTUSER", Status: "D"},
		},
	}
}

func TestAddTransportObjects_GoesToTheCallersTaskWithTableKeys(t *testing.T) {
	bridge := &fakeOrganizer{}
	res, err := addTransportObjects(context.Background(), bridge, noHeader, entriesRequest(), "", "TESTUSER", []TransportEntry{
		{TransportObjectKey: TransportObjectKey{"limu", "rept", "zdemo"}},
		{TransportObjectKey: TransportObjectKey{"R3TR", "TABU", "ZDEMO_CONF"}, Keys: []string{"100KEY1", "100KEY2*"}},
	})
	if err != nil {
		t.Fatalf("addTransportObjects: %v", err)
	}
	if res.Task != "TR-MINE" || len(res.Added) != 2 || res.Keys != 2 {
		t.Errorf("result = %+v", res)
	}
	if len(bridge.calls) != 1 || bridge.calls[0].fm != "TR_APPEND_TO_COMM_OBJS_KEYS" {
		t.Fatalf("calls = %+v", bridge.calls)
	}
	p := bridge.calls[0].params
	if p["WI_TRKORR"] != "TR-MINE" || p["IV_DIALOG"] != "" {
		t.Errorf("WI_TRKORR %v, IV_DIALOG %q", p["WI_TRKORR"], p["IV_DIALOG"])
	}
	wantE071 := []any{
		map[string]any{"PGMID": "LIMU", "OBJECT": "REPT", "OBJ_NAME": "ZDEMO"},
		map[string]any{"PGMID": "R3TR", "OBJECT": "TABU", "OBJ_NAME": "ZDEMO_CONF", "OBJFUNC": "K"},
	}
	if !reflect.DeepEqual(p["WT_E071"], wantE071) {
		t.Errorf("WT_E071 = %v", p["WT_E071"])
	}
	wantE071K := []any{
		map[string]any{"PGMID": "R3TR", "OBJECT": "TABU", "OBJNAME": "ZDEMO_CONF", "MASTERTYPE": "TABU", "MASTERNAME": "ZDEMO_CONF", "TABKEY": "100KEY1"},
		map[string]any{"PGMID": "R3TR", "OBJECT": "TABU", "OBJNAME": "ZDEMO_CONF", "MASTERTYPE": "TABU", "MASTERNAME": "ZDEMO_CONF", "TABKEY": "100KEY2*"},
	}
	if !reflect.DeepEqual(p["WT_E071K"], wantE071K) {
		t.Errorf("WT_E071K = %v", p["WT_E071K"])
	}
}

func TestAddTransportObjects_ReportsSAPsMessage(t *testing.T) {
	bridge := &fakeOrganizer{result: RFCResult{Subrc: 99, Message: "Object ZDEMO is locked in request TR-X"}}
	_, err := addTransportObjects(context.Background(), bridge, noHeader, entriesRequest(), "", "TESTUSER", []TransportEntry{
		{TransportObjectKey: TransportObjectKey{"R3TR", "PROG", "ZDEMO"}},
	})
	if err == nil || !strings.Contains(err.Error(), "locked in request TR-X") {
		t.Errorf("err = %v, want SAP's message", err)
	}
}

func TestAddTransportObjects_RefusesBeforeCalling(t *testing.T) {
	for name, entries := range map[string][]TransportEntry{
		"nothing":            nil,
		"keys on a program":  {{TransportObjectKey: TransportObjectKey{"R3TR", "PROG", "ZDEMO"}, Keys: []string{"X"}}},
		"an empty table key": {{TransportObjectKey: TransportObjectKey{"R3TR", "TABU", "ZDEMO_CONF"}, Keys: []string{" "}}},
		"no name":            {{TransportObjectKey: TransportObjectKey{"R3TR", "PROG", ""}}},
	} {
		bridge := &fakeOrganizer{}
		if _, err := addTransportObjects(context.Background(), bridge, noHeader, entriesRequest(), "", "TESTUSER", entries); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if len(bridge.calls) != 0 {
			t.Errorf("%s: called the bridge before refusing", name)
		}
	}
}

func TestRemoveTransportObject_FromTheTaskThatHoldsIt(t *testing.T) {
	bridge := &fakeOrganizer{}
	res, err := removeTransportObject(context.Background(), bridge, entriesRequest(), "", TransportObjectKey{"r3tr", "prog", "zdemo"})
	if err != nil {
		t.Fatalf("removeTransportObject: %v", err)
	}
	if !res.Removed || res.Task != "TR-OTHER" {
		t.Errorf("result = %+v", res)
	}
	p := bridge.calls[0].params
	if bridge.calls[0].fm != "TRINT_DELETE_COMM_OBJECT_KEYS" ||
		!reflect.DeepEqual(p["CS_REQUEST"], map[string]any{"H": map[string]any{"TRKORR": "TR-OTHER"}}) ||
		!reflect.DeepEqual(p["IS_E071_DELETE"], map[string]any{"PGMID": "R3TR", "OBJECT": "PROG", "OBJ_NAME": "ZDEMO"}) {
		t.Errorf("call = %+v", bridge.calls[0])
	}
}

// A method entry is found and deleted under the OBJ_NAME E071 holds -- the
// class in 30 columns, then the method -- when the caller types the two
// names with a single blank, and when the request lists it either way.
func TestRemoveTransportObject_MethodEntry(t *testing.T) {
	const e071Name = "ZCL_VSP_APC_HANDLER           CLASS_CONSTRUCTOR"
	for _, listed := range []string{e071Name, "ZCL_VSP_APC_HANDLER CLASS_CONSTRUCTOR"} {
		details := &TransportDetails{
			TransportSummary: TransportSummary{Number: "TR-REQ"},
			Tasks: []TransportTaskV2{{Number: "TR-TASK", Owner: "TESTUSER", Status: "D",
				Objects: []TransportObjectV2{{PgmID: "LIMU", Type: "METH", Name: listed}}}},
		}
		key, err := ParseTransportObject("LIMU METH ZCL_VSP_APC_HANDLER CLASS_CONSTRUCTOR")
		if err != nil {
			t.Fatal(err)
		}
		bridge := &fakeOrganizer{}
		res, err := removeTransportObject(context.Background(), bridge, details, "TR-TASK", key)
		if err != nil || !res.Removed || res.Task != "TR-TASK" {
			t.Fatalf("listed as %q: %+v %v", listed, res, err)
		}
		if got := bridge.calls[0].params["IS_E071_DELETE"]; !reflect.DeepEqual(got, map[string]any{"PGMID": "LIMU", "OBJECT": "METH", "OBJ_NAME": e071Name}) {
			t.Errorf("listed as %q: deleted %v", listed, got)
		}
	}
}

func TestRemoveTransportObject_RefusesAnEntryThatIsNotThere(t *testing.T) {
	bridge := &fakeOrganizer{}
	if _, err := removeTransportObject(context.Background(), bridge, entriesRequest(), "", TransportObjectKey{"R3TR", "PROG", "ZNOWHERE"}); err == nil {
		t.Fatal("accepted")
	}
	if len(bridge.calls) != 0 {
		t.Error("called the bridge for an entry that is not in the request")
	}
}

func TestAddTransportObjects_ClassifiesAnUnclassifiedTaskFirst(t *testing.T) {
	details := &TransportDetails{
		TransportSummary: TransportSummary{Number: "TR-REQ"},
		Tasks:            []TransportTaskV2{{Number: "TR-MINE", Owner: "TESTUSER", Status: "D", Type: "Unclassified"}},
	}
	header := map[string]any{"TRKORR": "TR-MINE", "TRFUNCTION": "X", "STRKORR": "TR-REQ", "KORRDEV": "SYST"}
	read := func(_ context.Context, number string) (map[string]any, error) {
		if number != "TR-MINE" {
			t.Errorf("read the header of %s, want TR-MINE", number)
		}
		return header, nil
	}
	bridge := &fakeOrganizer{}
	res, err := addTransportObjects(context.Background(), bridge, read, details, "", "TESTUSER", []TransportEntry{
		{TransportObjectKey: TransportObjectKey{"R3TR", "PROG", "ZDEMO"}},
	})
	if err != nil {
		t.Fatalf("addTransportObjects: %v", err)
	}
	fms := make([]string, 0, len(bridge.calls))
	for _, c := range bridge.calls {
		fms = append(fms, c.fm)
	}
	if !reflect.DeepEqual(fms, []string{"TR_CHANGE_TRFUNCTION", "TR_APPEND_TO_COMM_OBJS_KEYS"}) {
		t.Fatalf("calls = %v, want the classification before the append", fms)
	}
	p := bridge.calls[0].params
	if !reflect.DeepEqual(p["CS_REQUEST_HEADER"], header) || p["IV_NEW_TRFUNCTION"] != "S" {
		t.Errorf("classification = %v", p)
	}
	if res.Classified != "S" {
		t.Errorf("result does not say the task was classified: %+v", res)
	}
}

func TestTransportTree_ReadsTheRequestOfATask(t *testing.T) {
	header := `<?xml version="1.0" encoding="utf-8"?>
<tm:root xmlns:tm="http://www.sap.com/cts/adt/tm">
  <tm:request tm:number="TR-EXAMPLE" tm:owner="TESTUSER" tm:desc="Demo" tm:status="D"/>
</tm:root>`
	full := `<?xml version="1.0" encoding="utf-8"?>
<tm:root xmlns:tm="http://www.sap.com/cts/adt/tm">
  <tm:request tm:number="TR-EXAMPLE" tm:owner="TESTUSER" tm:desc="Demo" tm:status="D">
    <tm:task tm:number="TR-EXAMPLE-T" tm:owner="TESTUSER" tm:desc="Demo" tm:status="D">
      <tm:abap_object tm:pgmid="R3TR" tm:type="PROG" tm:name="ZDEMO"/>
    </tm:task>
  </tm:request>
</tm:root>`
	c, mock := newTransportTestClient(t, map[string][]string{
		"/sap/bc/adt/cts/transportrequests/TR-EXAMPLE-T": {header},
		"/sap/bc/adt/cts/transportrequests/TR-EXAMPLE":   {full},
	})
	details, err := c.transportTree(context.Background(), "TR-EXAMPLE-T")
	if err != nil {
		t.Fatal(err)
	}
	if got := holderOf(details, TransportObjectKey{PgmID: "R3TR", Object: "PROG", Name: "ZDEMO"}); got != "TR-EXAMPLE-T" {
		t.Errorf("entry found in %q, want the task TR-EXAMPLE-T", got)
	}
	if len(mock.requests) != 2 {
		t.Errorf("%d reads, want the task and then its request", len(mock.requests))
	}

	c, mock = newTransportTestClient(t, map[string][]string{"/sap/bc/adt/cts/transportrequests/TR-EXAMPLE": {full}})
	if _, err := c.transportTree(context.Background(), "TR-EXAMPLE"); err != nil {
		t.Fatal(err)
	}
	if len(mock.requests) != 1 {
		t.Errorf("a request number was read %d times, want once", len(mock.requests))
	}
}

// sequenceOrganizer answers each call with the next result in line.
type sequenceOrganizer struct {
	fms     []string
	results []RFCResult
}

func (s *sequenceOrganizer) CallRFC(_ context.Context, fm string, _ map[string]any) (*RFCResult, error) {
	s.fms = append(s.fms, fm)
	r := RFCResult{}
	if len(s.results) > 0 {
		r, s.results = s.results[0], s.results[1:]
	}
	return &r, nil
}

// An entry recorded twice is refused until the task is sorted and
// compressed, as SE09 would; removal does that and tries again.
func TestRemoveTransportObject_SortsAndCompressesADuplicate(t *testing.T) {
	bridge := &sequenceOrganizer{results: []RFCResult{
		{Subrc: 99, Message: "Object entry exists more than once; sort and compress first"},
		{},
		{},
	}}
	res, err := removeTransportObject(context.Background(), bridge, entriesRequest(), "", TransportObjectKey{"R3TR", "PROG", "ZDEMO"})
	if err != nil || !res.Removed {
		t.Fatalf("removeTransportObject: %+v %v", res, err)
	}
	want := []string{"TRINT_DELETE_COMM_OBJECT_KEYS", "TR_SORT_AND_COMPRESS_COMM", "TRINT_DELETE_COMM_OBJECT_KEYS"}
	if !reflect.DeepEqual(bridge.fms, want) {
		t.Errorf("calls %v, want %v", bridge.fms, want)
	}
}

// Deleting a DDIC object records it again as a deletion, and Sort and
// Compress keeps both rows. Removal then hands the function a list with the
// entry once, and gives the task's lock back by appending the object and
// taking it out again.
func TestRemoveTransportObject_RemovesAnEntryRecordedAsItsDeletion(t *testing.T) {
	twice := RFCResult{Subrc: 99, Message: "Object entry exists more than once; sort and compress first"}
	bridge := &sequenceOrganizer{results: []RFCResult{twice, {}, twice, {}, {}, {}}}
	res, err := removeTransportObject(context.Background(), bridge, entriesRequest(), "", TransportObjectKey{"R3TR", "PROG", "ZDEMO"})
	if err != nil || !res.Removed {
		t.Fatalf("removeTransportObject: %+v %v", res, err)
	}
	want := []string{
		"TRINT_DELETE_COMM_OBJECT_KEYS", "TR_SORT_AND_COMPRESS_COMM", "TRINT_DELETE_COMM_OBJECT_KEYS",
		"TRINT_DELETE_COMM_OBJECT_KEYS", "TR_APPEND_TO_COMM_OBJS_KEYS", "TRINT_DELETE_COMM_OBJECT_KEYS",
	}
	if !reflect.DeepEqual(bridge.fms, want) {
		t.Errorf("calls %v, want %v", bridge.fms, want)
	}
}

// When appending the object again fails, both rows are gone but the lock may
// not be, and the caller is told so.
func TestRemoveTransportObject_SaysWhenTheLockMayRemain(t *testing.T) {
	twice := RFCResult{Subrc: 99, Message: "Object entry exists more than once; sort and compress first"}
	bridge := &sequenceOrganizer{results: []RFCResult{twice, {}, twice, {}, {Subrc: 99, Message: "Object is locked"}}}
	_, err := removeTransportObject(context.Background(), bridge, entriesRequest(), "", TransportObjectKey{"R3TR", "PROG", "ZDEMO"})
	if err == nil || !strings.Contains(err.Error(), "may still hold its lock") {
		t.Fatalf("err = %v, want the lock named", err)
	}
}

// A task named in place of the request is the target, even when it is not
// the caller's own; taking an entry out of a named task looks only there.
func TestTransportEntries_HonourANamedTask(t *testing.T) {
	bridge := &fakeOrganizer{}
	res, err := addTransportObjects(context.Background(), bridge, noHeader, entriesRequest(), "TR-OTHER", "TESTUSER", []TransportEntry{
		{TransportObjectKey: TransportObjectKey{"R3TR", "PROG", "ZDEMO2"}},
	})
	if err != nil || res.Task != "TR-OTHER" {
		t.Fatalf("add to a named task: %+v %v", res, err)
	}
	if _, rerr := removeTransportObject(context.Background(), &sequenceOrganizer{}, entriesRequest(), "TR-MINE", TransportObjectKey{"R3TR", "PROG", "ZDEMO"}); rerr == nil || !strings.Contains(rerr.Error(), "not in TR-MINE") {
		t.Errorf("remove from a named task that does not hold it: %v", rerr)
	}
	res2, err := removeTransportObject(context.Background(), &sequenceOrganizer{}, entriesRequest(), "TR-OTHER", TransportObjectKey{"R3TR", "PROG", "ZDEMO"})
	if err != nil || res2.Task != "TR-OTHER" {
		t.Errorf("remove from the named task that holds it: %+v %v", res2, err)
	}
}
