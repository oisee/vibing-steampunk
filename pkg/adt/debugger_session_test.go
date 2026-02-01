package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDebuggerAttach_Mock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/core/discovery" {
			w.Header().Set("X-CSRF-Token", "test-token")
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == "/sap/bc/adt/debugger" {
			method := r.URL.Query().Get("method")
			if method == "attach" {
				w.Header().Set("Content-Type", "application/xml")
				w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<dbg:attach xmlns:dbg="http://www.sap.com/adt/debugger"
  debugSessionId="session123"
  processId="42"
  isSteppingPossible="true"
  isTerminationPossible="true">
</dbg:attach>`))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "testuser", "testpass", WithClient("001"))
	ctx := context.Background()

	result, err := client.DebuggerAttach(ctx, "debuggee123", "testuser")
	if err != nil {
		t.Fatalf("DebuggerAttach failed: %v", err)
	}

	if result.DebugSessionID != "session123" {
		t.Errorf("expected debugSessionId 'session123', got '%s'", result.DebugSessionID)
	}
	if result.ProcessID != 42 {
		t.Errorf("expected processId 42, got %d", result.ProcessID)
	}
}

func TestDebuggerStep_Mock(t *testing.T) {
	var lastMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/core/discovery" {
			w.Header().Set("X-CSRF-Token", "test-token")
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == "/sap/bc/adt/debugger" {
			lastMethod = r.URL.Query().Get("method")
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<dbg:step xmlns:dbg="http://www.sap.com/adt/debugger"
  debugSessionId="session123"
  isDebuggeeChanged="false"
  isSteppingPossible="true">
  <dbg:settings/>
</dbg:step>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "testuser", "testpass", WithClient("001"))
	ctx := context.Background()

	// Test stepOver
	result, err := client.DebuggerStep(ctx, DebugStepOver, "")
	if err != nil {
		t.Fatalf("DebuggerStep failed: %v", err)
	}
	if lastMethod != "stepOver" {
		t.Errorf("expected method 'stepOver', got '%s'", lastMethod)
	}
	if !result.IsSteppingPossible {
		t.Error("expected isSteppingPossible=true")
	}

	// Test stepInto
	_, err = client.DebuggerStep(ctx, DebugStepInto, "")
	if err != nil {
		t.Fatalf("DebuggerStep stepInto failed: %v", err)
	}
	if lastMethod != "stepInto" {
		t.Errorf("expected method 'stepInto', got '%s'", lastMethod)
	}

	// Test stepContinue
	_, err = client.DebuggerStep(ctx, DebugStepContinue, "")
	if err != nil {
		t.Fatalf("DebuggerStep stepContinue failed: %v", err)
	}
	if lastMethod != "stepContinue" {
		t.Errorf("expected method 'stepContinue', got '%s'", lastMethod)
	}
}

func TestDebuggerGetStack_Mock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/core/discovery" {
			w.Header().Set("X-CSRF-Token", "test-token")
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == "/sap/bc/adt/debugger/stack" {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<dbg:stack xmlns:dbg="http://www.sap.com/adt/debugger"
  isSameSystem="true"
  debugCursorStackIndex="1">
  <dbg:stackEntry
    stackPosition="1"
    programName="ZTEST_MCP_CRUD"
    line="15"
    eventType="REPORT"/>
</dbg:stack>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "testuser", "testpass", WithClient("001"))
	ctx := context.Background()

	result, err := client.DebuggerGetStack(ctx, true)
	if err != nil {
		t.Fatalf("DebuggerGetStack failed: %v", err)
	}

	if len(result.Stack) != 1 {
		t.Fatalf("expected 1 stack entry, got %d", len(result.Stack))
	}
	if result.Stack[0].ProgramName != "ZTEST_MCP_CRUD" {
		t.Errorf("expected programName 'ZTEST_MCP_CRUD', got '%s'", result.Stack[0].ProgramName)
	}
}

func TestDebuggerGetVariables_Mock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/core/discovery" {
			w.Header().Set("X-CSRF-Token", "test-token")
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == "/sap/bc/adt/debugger" {
			method := r.URL.Query().Get("method")
			if method == "getVariables" {
				w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
				w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <STPDA_ADT_VARIABLE>
        <ID>LV_COUNT</ID>
        <NAME>LV_COUNT</NAME>
        <META_TYPE>simple</META_TYPE>
        <VALUE>42</VALUE>
      </STPDA_ADT_VARIABLE>
    </DATA>
  </asx:values>
</asx:abap>`))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "testuser", "testpass", WithClient("001"))
	ctx := context.Background()

	result, err := client.DebuggerGetVariables(ctx, []string{"LV_COUNT"})
	if err != nil {
		t.Fatalf("DebuggerGetVariables failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 variable, got %d", len(result))
	}
	if result[0].Value != "42" {
		t.Errorf("expected value '42', got '%s'", result[0].Value)
	}
}

func TestDebuggerGetVariables_EmptyIDs(t *testing.T) {
	client := NewClient("http://localhost", "testuser", "testpass")
	ctx := context.Background()

	_, err := client.DebuggerGetVariables(ctx, []string{})
	if err == nil {
		t.Error("expected error for empty variable IDs")
	}
}
