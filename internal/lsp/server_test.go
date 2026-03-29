package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestTransportRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	transport := NewTransport(nil, &buf)

	msg := &Message{
		Method: "textDocument/publishDiagnostics",
		Params: json.RawMessage(`{"uri":"file:///test.abap","diagnostics":[]}`),
	}
	if err := transport.Write(msg); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read back
	readTransport := NewTransport(&buf, nil)
	got, err := readTransport.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if got.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %s", got.JSONRPC)
	}
	if got.Method != "textDocument/publishDiagnostics" {
		t.Errorf("expected method textDocument/publishDiagnostics, got %s", got.Method)
	}
}

func TestInitializeShutdownExit(t *testing.T) {
	// Build input: initialize → initialized → shutdown → exit
	var input bytes.Buffer
	writeMsg := func(method string, id *int, params string) {
		msg := `{"jsonrpc":"2.0"`
		if id != nil {
			msg += fmt.Sprintf(`,"id":%d`, *id)
		}
		msg += fmt.Sprintf(`,"method":"%s"`, method)
		if params != "" {
			msg += fmt.Sprintf(`,"params":%s`, params)
		}
		msg += "}"
		input.WriteString(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(msg), msg))
	}

	id1 := 1
	id2 := 2
	writeMsg("initialize", &id1, `{"capabilities":{}}`)
	writeMsg("initialized", nil, "")
	writeMsg("shutdown", &id2, "null")
	writeMsg("exit", nil, "")

	var output bytes.Buffer
	server := NewServer(nil, false)
	err := server.Serve(&input, &output)
	if err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}

	// Parse output messages
	outStr := output.String()
	if !strings.Contains(outStr, `"name":"vsp-lsp"`) {
		t.Error("expected server name in initialize response")
	}
	if strings.Contains(outStr, `"definitionProvider":true`) {
		t.Error("expected definitionProvider to be false/omitted with nil client")
	}
}

func TestURIToObjectURL(t *testing.T) {
	tests := []struct {
		uri        string
		wantObj    string
		wantIncl   string
	}{
		{
			"file:///src/zcl_foo.clas.abap",
			"/sap/bc/adt/oo/classes/ZCL_FOO",
			"",
		},
		{
			"file:///src/zprog.prog.abap",
			"/sap/bc/adt/programs/programs/ZPROG",
			"",
		},
		{
			"file:///src/zif_bar.intf.abap",
			"/sap/bc/adt/oo/interfaces/ZIF_BAR",
			"",
		},
		{
			"file:///src/zcl_foo.clas.testclasses.abap",
			"/sap/bc/adt/oo/classes/ZCL_FOO",
			"/sap/bc/adt/oo/classes/ZCL_FOO/includes/testclasses",
		},
		{
			"file:///src/zcl_foo.clas.locals_def.abap",
			"/sap/bc/adt/oo/classes/ZCL_FOO",
			"/sap/bc/adt/oo/classes/ZCL_FOO/includes/definitions",
		},
		{
			"file:///src/%23dmo%23cl_flight.clas.abap",
			"/sap/bc/adt/oo/classes/%2FDMO%2FCL_FLIGHT",
			"",
		},
		{
			"file:///src/unknown.txt",
			"",
			"",
		},
		{
			"file:///src/zfugr.fugr.abap",
			"/sap/bc/adt/functions/groups/ZFUGR",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			gotObj, gotIncl := uriToObjectURL(tt.uri)
			if gotObj != tt.wantObj {
				t.Errorf("objectURL = %q, want %q", gotObj, tt.wantObj)
			}
			if gotIncl != tt.wantIncl {
				t.Errorf("includeURL = %q, want %q", gotIncl, tt.wantIncl)
			}
		})
	}
}

func TestFindWordBounds(t *testing.T) {
	content := "  DATA lv_test TYPE string."
	start, end := findWordBounds(content, 0, 7) // cursor on 'v' in lv_test
	if start != 7 || end != 14 {
		t.Errorf("findWordBounds = (%d, %d), want (7, 14)", start, end)
	}
}

func TestDiagnosticSeverityMapping(t *testing.T) {
	// Verify severity constants match LSP spec
	if SeverityError != 1 {
		t.Errorf("SeverityError = %d, want 1", SeverityError)
	}
	if SeverityWarning != 2 {
		t.Errorf("SeverityWarning = %d, want 2", SeverityWarning)
	}
}
