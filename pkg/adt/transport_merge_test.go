package adt

import "testing"

func TestParseTransportObject(t *testing.T) {
	k, err := ParseTransportObject("prog zdemo")
	if err != nil || k.PgmID != "R3TR" || k.Object != "PROG" || k.Name != "ZDEMO" {
		t.Errorf("two parts: %+v %v", k, err)
	}
	k, err = ParseTransportObject("LIMU METH ZCL_DEMO RUN")
	if err != nil || k.PgmID != "LIMU" || k.Name != "ZCL_DEMO RUN" {
		t.Errorf("limu: %+v %v", k, err)
	}
	if _, err := ParseTransportObject("ZDEMO"); err == nil {
		t.Error("one part accepted")
	}
}

func TestHolderAndTask(t *testing.T) {
	d := &TransportDetails{
		TransportSummary: TransportSummary{Number: "TR-REQ"},
		Objects:          []TransportObjectV2{{PgmID: "R3TR", Type: "PROG", Name: "ZDEMO_AT_REQUEST"}},
		Tasks: []TransportTaskV2{
			{Number: "TR-TASK1", Owner: "SOMEONE", Status: "D", Objects: []TransportObjectV2{{PgmID: "R3TR", Type: "PROG", Name: "ZDEMO_A"}}},
			{Number: "TR-TASK2", Owner: "TESTUSER", Status: "D"},
		},
	}
	if h := holderOf(d, TransportObjectKey{"R3TR", "PROG", "ZDEMO_A"}); h != "TR-TASK1" {
		t.Errorf("holder: %s", h)
	}
	if h := holderOf(d, TransportObjectKey{"R3TR", "PROG", "ZDEMO_AT_REQUEST"}); h != "TR-REQ" {
		t.Errorf("request-level holder: %s", h)
	}
	if h := holderOf(d, TransportObjectKey{"R3TR", "PROG", "ZDEMO_NOWHERE"}); h != "" {
		t.Errorf("absent: %s", h)
	}
	if task := taskFor(d, "testuser"); task != "TR-TASK2" {
		t.Errorf("task: %s", task)
	}
	if task := taskFor(d, "NOBODY"); task != "TR-REQ" {
		t.Errorf("no task: %s", task)
	}
}
