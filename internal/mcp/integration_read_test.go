//go:build integration

// ABOUTME: Integration tests for MCP read actions (read PROG, CLAS, TABL, etc.).
// ABOUTME: Tests use standard SAP objects that exist in any system.

package mcp

import (
	"context"
	"strings"
	"testing"
)

func TestIntegration_ReadProgram(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Try to read a standard SAP program
	result, err := callSAPTool(ctx, s, "read", "PROG SAPMSSY0", nil)
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		// Try another common program
		result, err = callSAPTool(ctx, s, "read", "PROG RS_ABAP_SOURCE_SCAN", nil)
		if err != nil {
			t.Fatalf("callSAPTool failed: %v", err)
		}
		if isErrorResult(result) {
			t.Skipf("Could not retrieve any standard program: %s", getResultText(result))
		}
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Program source is empty")
	} else {
		t.Logf("Retrieved %d characters of source code", len(text))
	}
}

func TestIntegration_ReadClass(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Read a standard SAP class
	result, err := callSAPTool(ctx, s, "read", "CLAS CL_ABAP_TYPEDESCR", nil)
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not get CL_ABAP_TYPEDESCR: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Class source is empty")
	} else {
		t.Logf("Retrieved %d characters of class source", len(text))
		// Class source should contain class definition
		if !strings.Contains(strings.ToLower(text), "class") {
			t.Error("Expected class source to contain 'class' keyword")
		}
	}
}

func TestIntegration_ReadInterface(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Read a standard SAP interface
	result, err := callSAPTool(ctx, s, "read", "INTF IF_SERIALIZABLE_OBJECT", nil)
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not get IF_SERIALIZABLE_OBJECT: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Interface source is empty")
	} else {
		t.Logf("Retrieved %d characters of interface source", len(text))
	}
}

func TestIntegration_ReadTable(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Read table definition (T000 is the clients table)
	result, err := callSAPTool(ctx, s, "read", "TABL T000", nil)
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not get T000 table: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Table source is empty")
	} else {
		t.Logf("Retrieved %d characters of table source", len(text))
	}
}

func TestIntegration_ReadFunction(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Read a standard function module (requires function group)
	result, err := callSAPTool(ctx, s, "read", "FUNC RFC_READ_TABLE", map[string]any{
		"function_group": "SDTX",
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not get RFC_READ_TABLE: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Function source is empty")
	} else {
		t.Logf("Retrieved %d characters of function source", len(text))
	}
}

func TestIntegration_ReadFunctionGroup(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Read a function group metadata
	result, err := callSAPTool(ctx, s, "read", "FUGR SDTX", nil)
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not get SDTX function group: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Function group info is empty")
	} else {
		t.Logf("Retrieved function group info: %d characters", len(text))
		// Should contain JSON with group name
		if !strings.Contains(text, "SDTX") {
			t.Error("Expected function group info to contain group name")
		}
	}
}

func TestIntegration_ReadPackage(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Read package contents with pagination
	result, err := callSAPTool(ctx, s, "read", "DEVC BASIS", map[string]any{
		"maxResults": float64(10),
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not get BASIS package: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Package info is empty")
	} else {
		t.Logf("Retrieved package info: %d characters", len(text))
		// Should contain totalObjects field
		if !strings.Contains(text, "totalObjects") {
			t.Error("Expected package info to contain totalObjects")
		}
	}
}

func TestIntegration_ReadDDLS(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Read a CDS view definition
	result, err := callSAPTool(ctx, s, "read", "DDLS I_ABAPPACKAGE", nil)
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not get I_ABAPPACKAGE CDS view: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("DDLS source is empty")
	} else {
		t.Logf("Retrieved %d characters of CDS source", len(text))
		// CDS source should contain "define"
		if !strings.Contains(strings.ToLower(text), "define") {
			t.Error("Expected CDS source to contain 'define' keyword")
		}
	}
}

func TestIntegration_ReadMessageClass(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Read message class (00 is a standard message class)
	result, err := callSAPTool(ctx, s, "read", "MSAG 00", nil)
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not get message class 00: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Message class info is empty")
	} else {
		t.Logf("Retrieved message class info: %d characters", len(text))
	}
}

func TestIntegration_ReadTransaction(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Read transaction info (SE80 is ABAP Workbench)
	result, err := callSAPTool(ctx, s, "read", "TRAN SE80", nil)
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not get SE80 transaction: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Transaction info is empty")
	} else {
		t.Logf("Retrieved transaction info: %d characters", len(text))
		// Should contain transaction code
		if !strings.Contains(text, "SE80") {
			t.Error("Expected transaction info to contain transaction code")
		}
	}
}
