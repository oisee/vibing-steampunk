//go:build integration

// ABOUTME: Integration tests for MCP CRUD actions (lock, unlock, create, read, edit, delete).
// ABOUTME: Tests write operations using $TMP package (no transport required).

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIntegration_LockUnlock(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Lock a standard program
	objectURL := "/sap/bc/adt/programs/programs/sapmssy0"
	lockResult, err := callSAPTool(ctx, s, "edit", "LOCK", map[string]any{
		"object_url":  objectURL,
		"access_mode": "MODIFY",
	})
	if err != nil {
		t.Fatalf("callSAPTool lock failed: %v", err)
	}

	if isErrorResult(lockResult) {
		t.Skipf("Could not lock object: %s", getResultText(lockResult))
	}

	// Extract lock handle from result
	text := getResultText(lockResult)
	t.Logf("Lock result: %s", text)

	var lockInfo map[string]any
	if err := json.Unmarshal([]byte(text), &lockInfo); err != nil {
		t.Fatalf("Could not parse lock result: %v", err)
	}

	lockHandle, ok := lockInfo["lockHandle"].(string)
	if !ok || lockHandle == "" {
		t.Skipf("No lockHandle in result: %v", lockInfo)
	}

	// Unlock the object
	unlockResult, err := callSAPTool(ctx, s, "edit", "UNLOCK", map[string]any{
		"object_url":  objectURL,
		"lock_handle": lockHandle,
	})
	if err != nil {
		t.Fatalf("callSAPTool unlock failed: %v", err)
	}

	if isErrorResult(unlockResult) {
		t.Errorf("Unlock failed: %s", getResultText(unlockResult))
	} else {
		t.Log("Lock/unlock cycle completed successfully")
	}
}

func TestIntegration_ReadClassInfo(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Read class info for a standard class
	result, err := callSAPTool(ctx, s, "read", "CLAS_INFO CL_ABAP_TYPEDESCR", nil)
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not get class info: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Class info is empty")
	} else {
		t.Logf("Class info: %d characters", len(text))
		// Should contain class-related info
		if !strings.Contains(text, "CL_ABAP_TYPEDESCR") {
			t.Error("Expected class info to contain class name")
		}
	}
}

func TestIntegration_CompareSource(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Compare two standard programs
	result, err := callSAPTool(ctx, s, "analyze", "", map[string]any{
		"type":  "compare_source",
		"type1": "PROG",
		"name1": "SAPMSSY0",
		"type2": "PROG",
		"name2": "SAPMSSY1",
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not compare sources: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Compare result is empty")
	} else {
		t.Logf("Compare result: %d characters", len(text))
	}
}

func TestIntegration_CreateProgram(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Create a test program in $TMP
	progName := fmt.Sprintf("ZTEST_MCP_%d", time.Now().Unix()%100000)

	result, err := callSAPTool(ctx, s, "create", "OBJECT", map[string]any{
		"object_type":  "PROG/P",
		"name":         progName,
		"description":  "MCP Integration Test Program",
		"package_name": "$TMP",
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		resultText := getResultText(result)
		// If already exists, that's OK - skip the test
		if strings.Contains(resultText, "already exist") {
			t.Skipf("Program already exists: %s", resultText)
		}
		t.Fatalf("Could not create program: %s", resultText)
	}

	text := getResultText(result)
	t.Logf("Create result: %s", text)

	// Verify creation was successful
	if !strings.Contains(text, "created") {
		t.Error("Expected result to indicate creation")
	}

	// Clean up: try to delete the program
	var createInfo map[string]any
	if err := json.Unmarshal([]byte(text), &createInfo); err == nil {
		objectURL, _ := createInfo["object_url"].(string)
		if objectURL != "" {
			cleanupProgram(ctx, t, s, objectURL)
		}
	}
}

func TestIntegration_CreateClass(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Create a test class in $TMP
	className := fmt.Sprintf("ZCL_TEST_MCP_%d", time.Now().Unix()%100000)

	result, err := callSAPTool(ctx, s, "create", "OBJECT", map[string]any{
		"object_type":  "CLAS/OC",
		"name":         className,
		"description":  "MCP Integration Test Class",
		"package_name": "$TMP",
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		resultText := getResultText(result)
		if strings.Contains(resultText, "already exist") {
			t.Skipf("Class already exists: %s", resultText)
		}
		t.Fatalf("Could not create class: %s", resultText)
	}

	text := getResultText(result)
	t.Logf("Create result: %s", text)

	if !strings.Contains(text, "created") {
		t.Error("Expected result to indicate creation")
	}

	// Clean up
	var createInfo map[string]any
	if err := json.Unmarshal([]byte(text), &createInfo); err == nil {
		objectURL, _ := createInfo["object_url"].(string)
		if objectURL != "" {
			cleanupClass(ctx, t, s, objectURL)
		}
	}
}

func TestIntegration_CreateInterface(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Create a test interface in $TMP
	intfName := fmt.Sprintf("ZIF_TEST_MCP_%d", time.Now().Unix()%100000)

	result, err := callSAPTool(ctx, s, "create", "OBJECT", map[string]any{
		"object_type":  "INTF/OI",
		"name":         intfName,
		"description":  "MCP Integration Test Interface",
		"package_name": "$TMP",
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		resultText := getResultText(result)
		if strings.Contains(resultText, "already exist") {
			t.Skipf("Interface already exists: %s", resultText)
		}
		t.Fatalf("Could not create interface: %s", resultText)
	}

	text := getResultText(result)
	t.Logf("Create result: %s", text)

	if !strings.Contains(text, "created") {
		t.Error("Expected result to indicate creation")
	}

	// Clean up
	var createInfo map[string]any
	if err := json.Unmarshal([]byte(text), &createInfo); err == nil {
		objectURL, _ := createInfo["object_url"].(string)
		if objectURL != "" {
			cleanupInterface(ctx, t, s, objectURL)
		}
	}
}

func TestIntegration_EditInterfaceWithSource(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Use edit INTF with params.source (upsert mode)
	intfName := fmt.Sprintf("ZIF_TEST_EDIT_%d", time.Now().Unix()%100000)
	source := fmt.Sprintf(`INTERFACE %s PUBLIC.
  METHODS get_value RETURNING VALUE(rv_value) TYPE string.
ENDINTERFACE.`, strings.ToLower(intfName))

	result, err := callSAPTool(ctx, s, "edit", "INTF "+intfName, map[string]any{
		"source":      source,
		"description": "MCP Edit Integration Test",
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Fatalf("Edit failed: %s", getResultText(result))
	}

	text := getResultText(result)
	t.Logf("Edit result: %s", text)

	// Result should indicate success
	if !strings.Contains(text, "created") && !strings.Contains(text, "updated") {
		t.Error("Expected result to indicate created or updated")
	}

	// Read it back to verify
	readResult, err := callSAPTool(ctx, s, "read", "INTF "+intfName, nil)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if isErrorResult(readResult) {
		t.Fatalf("Read failed: %s", getResultText(readResult))
	}

	readText := getResultText(readResult)
	if !strings.Contains(strings.ToUpper(readText), "GET_VALUE") {
		t.Errorf("Expected source to contain GET_VALUE method, got: %s", readText)
	}

	// Clean up
	var writeInfo map[string]any
	if err := json.Unmarshal([]byte(text), &writeInfo); err == nil {
		objectURL, _ := writeInfo["object_url"].(string)
		if objectURL != "" {
			cleanupInterface(ctx, t, s, objectURL)
		}
	}
}

func TestIntegration_EditClassWithSource(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Use edit CLAS with params.source (upsert mode)
	className := fmt.Sprintf("ZCL_TEST_EDIT_%d", time.Now().Unix()%100000)
	source := fmt.Sprintf(`CLASS %s DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    METHODS get_name RETURNING VALUE(rv_name) TYPE string.
ENDCLASS.

CLASS %s IMPLEMENTATION.
  METHOD get_name.
    rv_name = '%s'.
  ENDMETHOD.
ENDCLASS.`, strings.ToLower(className), strings.ToLower(className), className)

	result, err := callSAPTool(ctx, s, "edit", "CLAS "+className, map[string]any{
		"source":      source,
		"description": "MCP Edit Integration Test Class",
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Fatalf("Edit failed: %s", getResultText(result))
	}

	text := getResultText(result)
	t.Logf("Edit result: %s", text)

	// Result should indicate success
	if !strings.Contains(text, "created") && !strings.Contains(text, "updated") {
		t.Error("Expected result to indicate created or updated")
	}

	// Read it back to verify
	readResult, err := callSAPTool(ctx, s, "read", "CLAS "+className, nil)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if isErrorResult(readResult) {
		t.Fatalf("Read failed: %s", getResultText(readResult))
	}

	readText := getResultText(readResult)
	if !strings.Contains(strings.ToUpper(readText), "GET_NAME") {
		t.Errorf("Expected source to contain GET_NAME method, got: %s", readText)
	}

	// Clean up
	var writeInfo map[string]any
	if err := json.Unmarshal([]byte(text), &writeInfo); err == nil {
		objectURL, _ := writeInfo["object_url"].(string)
		if objectURL != "" {
			cleanupClass(ctx, t, s, objectURL)
		}
	}
}

// cleanupProgram attempts to delete a test program
func cleanupProgram(ctx context.Context, t *testing.T, s *Server, objectURL string) {
	// Lock the object first
	lockResult, err := callSAPTool(ctx, s, "edit", "LOCK", map[string]any{
		"object_url": objectURL,
	})
	if err != nil || isErrorResult(lockResult) {
		t.Logf("Could not lock for cleanup: %v", err)
		return
	}

	var lockInfo map[string]any
	if err := json.Unmarshal([]byte(getResultText(lockResult)), &lockInfo); err != nil {
		t.Logf("Could not parse lock for cleanup: %v", err)
		return
	}

	lockHandle, _ := lockInfo["lock_handle"].(string)
	if lockHandle == "" {
		t.Log("No lock handle for cleanup")
		return
	}

	// Delete the object
	_, err = callSAPTool(ctx, s, "delete", "OBJECT", map[string]any{
		"object_url":  objectURL,
		"lock_handle": lockHandle,
	})
	if err != nil {
		t.Logf("Cleanup delete failed: %v", err)
	} else {
		t.Log("Cleanup successful")
	}
}

// cleanupClass attempts to delete a test class
func cleanupClass(ctx context.Context, t *testing.T, s *Server, objectURL string) {
	cleanupProgram(ctx, t, s, objectURL) // Same logic
}

// cleanupInterface attempts to delete a test interface
func cleanupInterface(ctx context.Context, t *testing.T, s *Server, objectURL string) {
	cleanupProgram(ctx, t, s, objectURL) // Same logic
}
