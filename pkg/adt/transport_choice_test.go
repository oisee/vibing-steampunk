package adt

import (
	"context"
	"testing"
)

func TestParseTransportCheck(t *testing.T) {
	body := `<?xml version="1.0" encoding="utf-8"?><asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><PGMID>LIMU</PGMID><OBJECT>REPS</OBJECT><OBJECTNAME>ZDEMO_B</OBJECTNAME><OPERATION>I</OPERATION><DEVCLASS>ZDEMO</DEVCLASS><KORRFLAG>X</KORRFLAG><RESULT>S</RESULT><RECORDING>X</RECORDING><EXISTING_REQ_ONLY/><MESSAGES/><REQUESTS><CTS_REQUEST><REQ_HEADER><TRKORR>TR-EXAMPLE</TRKORR><TRFUNCTION>K</TRFUNCTION><TRSTATUS>D</TRSTATUS><AS4USER>TESTUSER</AS4USER><AS4TEXT>feature X</AS4TEXT><CLIENT>001</CLIENT></REQ_HEADER></CTS_REQUEST></REQUESTS><LOCKS/><TADIRDEVC>ZDEMO</TADIRDEVC></DATA></asx:values></asx:abap>`
	tc, err := parseTransportCheck([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if !tc.Recording || tc.Package != "ZDEMO" || tc.ObjectName != "ZDEMO_B" || len(tc.Candidates) != 1 {
		t.Fatalf("check: %+v", tc)
	}
	if c := tc.Candidates[0]; c.Number != "TR-EXAMPLE" || c.Status != "D" || c.Description != "feature X" || c.Owner != "TESTUSER" {
		t.Errorf("candidate: %+v", c)
	}
	local, err := parseTransportCheck([]byte(`<asx:abap xmlns:asx="x"><asx:values><DATA><DEVCLASS>$TMP</DEVCLASS><RECORDING/><REQUESTS/></DATA></asx:values></asx:abap>`))
	if err != nil || local.Recording || len(local.Candidates) != 0 {
		t.Errorf("local: %+v %v", local, err)
	}
}

func TestResolveWriteTransportFor(t *testing.T) {
	c := &Client{config: &Config{Safety: SafetyConfig{AllowTransportableEdits: true}}}
	// Named: as given, no note.
	if tr, note, err := c.resolveWriteTransportFor(&TransportChoice{Transport: "TR-EXAMPLE"}, "TR-NAMED", "", "op"); tr != "TR-NAMED" || note != "" || err != nil {
		t.Errorf("named: %q %q %v", tr, note, err)
	}
	// The lock's own request beats the plan.
	if tr, note, err := c.resolveWriteTransportFor(&TransportChoice{Transport: "TR-PLANNED", Reason: "reused"}, "", "TR-LOCKED", "op"); tr != "TR-LOCKED" || note != "" || err != nil {
		t.Errorf("locked: %q %q %v", tr, note, err)
	}
	// The plan, with its reason.
	if tr, note, err := c.resolveWriteTransportFor(&TransportChoice{Transport: "TR-PLANNED", Reason: "reused it"}, "", "", "op"); tr != "TR-PLANNED" || note != "reused it" || err != nil {
		t.Errorf("planned: %q %q %v", tr, note, err)
	}
	// Nothing chosen: the reason still comes back, no request.
	if tr, note, err := c.resolveWriteTransportFor(&TransportChoice{Reason: "left to SAP"}, "", "", "op"); tr != "" || note != "left to SAP" || err != nil {
		t.Errorf("none: %q %q %v", tr, note, err)
	}
	// A failed creation is the write's error.
	if _, _, err := c.resolveWriteTransportFor(&TransportChoice{Err: errTransportCreate}, "", "", "op"); err == nil {
		t.Error("creation failure swallowed")
	}
	// The policy applies to a planned request as to a named one.
	gated := &Client{config: &Config{Safety: SafetyConfig{}}}
	if _, _, err := gated.resolveWriteTransportFor(&TransportChoice{Transport: "TR-PLANNED"}, "", "", "op"); err == nil {
		t.Error("planned request bypassed the transportable-edit gate")
	}
	// nil plan: the old behaviour.
	if tr, _, err := c.resolveWriteTransportFor(nil, "", "TR-LOCKED", "op"); tr != "TR-LOCKED" || err != nil {
		t.Errorf("nil plan: %q %v", tr, err)
	}
	off := &Client{config: &Config{Safety: SafetyConfig{TransportChoice: "off"}}}
	if off.planTransport(context.Background(), "", "/sap/bc/adt/programs/programs/zdemo", "") != nil {
		t.Error("off still planned")
	}
	if c.planTransport(context.Background(), "TR-NAMED", "/sap/bc/adt/programs/programs/zdemo", "") != nil {
		t.Error("a named request still planned")
	}
}
