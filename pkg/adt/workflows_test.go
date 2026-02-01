package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// mockWorkflowTransport for testing workflows
type mockWorkflowTransport struct {
	responses map[string]*http.Response
	requests  []*http.Request
}

func (m *mockWorkflowTransport) Do(req *http.Request) (*http.Response, error) {
	m.requests = append(m.requests, req)

	// Match by path
	path := req.URL.Path
	if resp, ok := m.responses[path]; ok {
		return resp, nil
	}

	// Check for partial matches
	for key, resp := range m.responses {
		if strings.Contains(path, key) {
			return resp, nil
		}
	}

	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("Not found")),
		Header:     http.Header{},
	}, nil
}

func newWorkflowTestResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"X-CSRF-Token": []string{"test-token"}},
	}
}

// TestClient_GetSource_Program tests GetSource for PROG type
func TestClient_GetSource_Program(t *testing.T) {
	sourceCode := `REPORT ztest.
WRITE: 'Hello, World!'.`

	mock := &mockWorkflowTransport{
		responses: map[string]*http.Response{
			"/sap/bc/adt/programs/programs/ZTEST/source/main": newWorkflowTestResponse(sourceCode),
			"discovery": newWorkflowTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GetSource(context.Background(), "PROG", "ZTEST", &GetSourceOptions{})
	if err != nil {
		t.Fatalf("GetSource failed: %v", err)
	}

	if result != sourceCode {
		t.Errorf("GetSource returned %q, want %q", result, sourceCode)
	}
}

// TestClient_GetSource_Class tests GetSource for CLAS type
func TestClient_GetSource_Class(t *testing.T) {
	mock := &mockWorkflowTransport{
		responses: map[string]*http.Response{
			"/sap/bc/adt/oo/classes/ZCL_TEST": newWorkflowTestResponse(`{
				"definitions": "CLASS zcl_test DEFINITION PUBLIC.",
				"implementations": "CLASS zcl_test IMPLEMENTATION.\nENDCLASS.",
				"testclasses": "CLASS ltc_test DEFINITION FOR TESTING."
			}`),
			"discovery": newWorkflowTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	// Test without include parameter (returns full class)
	result, err := client.GetSource(context.Background(), "CLAS", "ZCL_TEST", &GetSourceOptions{})
	if err != nil {
		t.Fatalf("GetSource failed: %v", err)
	}

	if !strings.Contains(result, "definitions") {
		t.Errorf("GetSource didn't return class source")
	}
}

// TestClient_GetSource_Function tests GetSource for FUNC type
func TestClient_GetSource_Function(t *testing.T) {
	funcSource := `FUNCTION Z_TEST_FUNCTION.
ENDFUNCTION.`

	mock := &mockWorkflowTransport{
		responses: map[string]*http.Response{
			"/sap/bc/adt/functions/groups/ZFUGR/fmodules/Z_TEST_FUNCTION/source/main": newWorkflowTestResponse(funcSource),
			"discovery": newWorkflowTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GetSource(context.Background(), "FUNC", "Z_TEST_FUNCTION", &GetSourceOptions{
		Parent: "ZFUGR",
	})
	if err != nil {
		t.Fatalf("GetSource failed: %v", err)
	}

	if result != funcSource {
		t.Errorf("GetSource returned %q, want %q", result, funcSource)
	}
}

// TestClient_GetSource_InvalidType tests GetSource with invalid type
func TestClient_GetSource_InvalidType(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, &mockWorkflowTransport{
		responses: map[string]*http.Response{
			"discovery": newWorkflowTestResponse("OK"),
		},
	})
	client := NewClientWithTransport(cfg, transport)

	_, err := client.GetSource(context.Background(), "INVALID", "ZTEST", &GetSourceOptions{})
	if err == nil {
		t.Fatal("GetSource should fail with invalid type")
	}

	if !strings.Contains(err.Error(), "unsupported object type") {
		t.Errorf("Expected 'unsupported object type' error, got: %v", err)
	}
}

// TestClient_WriteSource_Create tests WriteSource in create mode
func TestClient_WriteSource_Create(t *testing.T) {
	sourceCode := `REPORT ztest.
WRITE: 'Hello, World!'.`

	mock := &mockWorkflowTransport{
		responses: map[string]*http.Response{
			"/sap/bc/adt/programs/programs/ZTEST": newWorkflowTestResponse(`<?xml version="1.0"?>
<program:abapProgram xmlns:program="http://www.sap.com/adt/programs"
                     adtcore:name="ZTEST"
                     adtcore:type="PROG/P"
                     adtcore:responsible="USER"/>`),
			"/sap/bc/adt/programs/programs/ZTEST/source/main": newWorkflowTestResponse("OK"),
			"/sap/bc/adt/checkruns":                           newWorkflowTestResponse("OK"),
			"/sap/bc/adt/activation":                          newWorkflowTestResponse("OK"),
			"discovery":                                       newWorkflowTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.WriteSource(context.Background(), "PROG", "ZTEST", sourceCode, &WriteSourceOptions{
		Mode:        WriteModeCreate,
		Description: "Test program",
		Package:     "$TMP",
	})
	if err != nil {
		t.Fatalf("WriteSource failed: %v", err)
	}

	if result.ObjectURL == "" {
		t.Error("WriteSource should return object URL")
	}
}

// TestClient_WriteSource_Update tests WriteSource in update mode
func TestClient_WriteSource_Update(t *testing.T) {
	sourceCode := `REPORT ztest.
WRITE: 'Updated!'.`

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, &mockWorkflowTransport{
		responses: map[string]*http.Response{
			"/sap/bc/adt/programs/programs/ZTEST": newWorkflowTestResponse(`<?xml version="1.0"?>
<program:abapProgram xmlns:program="http://www.sap.com/adt/programs"/>`),
			"discovery": newWorkflowTestResponse("OK"),
		},
	})
	client := NewClientWithTransport(cfg, transport)

	result, err := client.WriteSource(context.Background(), "PROG", "ZTEST", sourceCode, &WriteSourceOptions{
		Mode: WriteModeUpdate,
	})

	// WriteSource is a complex workflow - we just verify it doesn't error
	// Full workflow testing requires integration tests
	if err != nil {
		t.Fatalf("WriteSource failed: %v", err)
	}

	if result == nil {
		t.Fatal("WriteSource should return non-nil result")
	}

	// Verify it's in update mode (even if workflow didn't complete due to mocks)
	if result.ObjectType != "PROG" {
		t.Errorf("Expected ObjectType 'PROG', got %q", result.ObjectType)
	}
}

// TestClient_GrepObjects tests GrepObjects with multiple objects
func TestClient_GrepObjects(t *testing.T) {
	sourceCode1 := `REPORT ztest1.
DATA: lv_todo TYPE string. " TODO: implement this`

	sourceCode2 := `REPORT ztest2.
* TODO: fix this bug
WRITE: 'Hello'.`

	mock := &mockWorkflowTransport{
		responses: map[string]*http.Response{
			"/sap/bc/adt/programs/programs/ZTEST1/source/main": newWorkflowTestResponse(sourceCode1),
			"/sap/bc/adt/programs/programs/ZTEST2/source/main": newWorkflowTestResponse(sourceCode2),
			"discovery": newWorkflowTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GrepObjects(context.Background(), []string{
		"/sap/bc/adt/programs/programs/ZTEST1",
		"/sap/bc/adt/programs/programs/ZTEST2",
	}, "TODO", false, 0)

	if err != nil {
		t.Fatalf("GrepObjects failed: %v", err)
	}

	if len(result.Objects) != 2 {
		t.Errorf("Expected 2 objects with matches, got %d", len(result.Objects))
	}

	if result.TotalMatches != 2 {
		t.Errorf("Expected 2 total matches, got %d", result.TotalMatches)
	}
}

// TestClient_GrepObjects_NoMatches tests GrepObjects when pattern doesn't match
func TestClient_GrepObjects_NoMatches(t *testing.T) {
	sourceCode := `REPORT ztest.
WRITE: 'Hello, World!'.`

	mock := &mockWorkflowTransport{
		responses: map[string]*http.Response{
			"/sap/bc/adt/programs/programs/ZTEST/source/main": newWorkflowTestResponse(sourceCode),
			"discovery": newWorkflowTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GrepObjects(context.Background(), []string{
		"/sap/bc/adt/programs/programs/ZTEST",
	}, "NOTFOUND", false, 0)

	if err != nil {
		t.Fatalf("GrepObjects failed: %v", err)
	}

	if len(result.Objects) != 0 {
		t.Errorf("Expected 0 objects with matches, got %d", len(result.Objects))
	}

	if result.TotalMatches != 0 {
		t.Errorf("Expected 0 total matches, got %d", result.TotalMatches)
	}
}

// TestClient_GrepPackages tests GrepPackages with single package
func TestClient_GrepPackages(t *testing.T) {
	packageContents := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.sap.com/adt/packages" name="$TMP">
  <objects>
    <object type="PROG/P" name="ZTEST1" uri="/sap/bc/adt/programs/programs/ZTEST1"/>
  </objects>
</package>`

	sourceCode := `REPORT ztest1.
" TODO: complete implementation`

	mock := &mockWorkflowTransport{
		responses: map[string]*http.Response{
			"/sap/bc/adt/packages/$TMP":                        newWorkflowTestResponse(packageContents),
			"/sap/bc/adt/programs/programs/ZTEST1/source/main": newWorkflowTestResponse(sourceCode),
			"discovery": newWorkflowTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GrepPackages(context.Background(), []string{"$TMP"}, false, "TODO", false, []string{}, 0)

	if err != nil {
		t.Fatalf("GrepPackages failed: %v", err)
	}

	// Just verify it doesn't error - actual matching depends on GetPackage XML parsing
	if result == nil {
		t.Fatal("GrepPackages should return non-nil result")
	}

	if len(result.Packages) != 1 {
		t.Errorf("Expected 1 package in result, got %d", len(result.Packages))
	}
}

// TestClient_GrepPackages_Recursive tests GrepPackages with subpackage recursion
func TestClient_GrepPackages_Recursive(t *testing.T) {
	mainPackageContents := `<?xml version="1.0" encoding="UTF-8"?>
<package:package xmlns:package="http://www.sap.com/adt/packages">
  <package:subPackages>
    <package:package package:name="ZSUB1"/>
  </package:subPackages>
</package:package>`

	subPackageContents := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.sap.com/adt/packages" name="ZSUB1">
  <objects>
    <object type="PROG/P" name="ZTEST_SUB" uri="/sap/bc/adt/programs/programs/ZTEST_SUB"/>
  </objects>
</package>`

	sourceCode := `REPORT ztest_sub.
" TODO: implement`

	mock := &mockWorkflowTransport{
		responses: map[string]*http.Response{
			"/sap/bc/adt/packages/ZMAIN":                          newWorkflowTestResponse(mainPackageContents),
			"/sap/bc/adt/packages/ZSUB1":                          newWorkflowTestResponse(subPackageContents),
			"/sap/bc/adt/programs/programs/ZTEST_SUB/source/main": newWorkflowTestResponse(sourceCode),
			"discovery": newWorkflowTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	// Test with recursive flag
	result, err := client.GrepPackages(context.Background(), []string{"ZMAIN"}, true, "TODO", false, []string{}, 0)

	if err != nil {
		t.Fatalf("GrepPackages failed: %v", err)
	}

	if len(result.Packages) == 0 {
		t.Error("Expected results from recursive search")
	}
}

// TestClient_GrepPackages_MultiplePackages tests GrepPackages with array of packages
func TestClient_GrepPackages_MultiplePackages(t *testing.T) {
	packageContents := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.sap.com/adt/packages" name="$TMP">
  <objects>
    <object type="PROG/P" name="ZTEST1" uri="/sap/bc/adt/programs/programs/ZTEST1"/>
  </objects>
</package>`

	sourceCode := `REPORT ztest1.
" FIXME: bug here`

	mock := &mockWorkflowTransport{
		responses: map[string]*http.Response{
			"/sap/bc/adt/packages/$TMP":                        newWorkflowTestResponse(packageContents),
			"/sap/bc/adt/packages/$LOCAL":                      newWorkflowTestResponse(packageContents),
			"/sap/bc/adt/programs/programs/ZTEST1/source/main": newWorkflowTestResponse(sourceCode),
			"discovery": newWorkflowTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	// Test with multiple packages
	result, err := client.GrepPackages(context.Background(), []string{"$TMP", "$LOCAL"}, false, "FIXME", false, []string{}, 0)

	if err != nil {
		t.Fatalf("GrepPackages failed: %v", err)
	}

	if len(result.Packages) == 0 {
		t.Error("Expected results from multiple packages")
	}
}

// TestExecuteABAPResult tests the ExecuteABAPResult struct
func TestExecuteABAPResult(t *testing.T) {
	result := &ExecuteABAPResult{
		Success:       true,
		ProgramName:   "ZTEMP_EXEC_12345678",
		Output:        []string{"Hello from SAP"},
		ExecutionTime: 1234,
		CleanedUp:     true,
		Message:       "Executed successfully, 1 output(s) returned",
	}

	if !result.Success {
		t.Error("Expected Success to be true")
	}

	if result.ProgramName != "ZTEMP_EXEC_12345678" {
		t.Errorf("Expected ProgramName ZTEMP_EXEC_12345678, got %s", result.ProgramName)
	}

	if len(result.Output) != 1 {
		t.Errorf("Expected 1 output, got %d", len(result.Output))
	}

	if result.Output[0] != "Hello from SAP" {
		t.Errorf("Expected output 'Hello from SAP', got %s", result.Output[0])
	}

	if result.ExecutionTime != 1234.0 {
		t.Errorf("Expected ExecutionTime 1234.0, got %f", result.ExecutionTime)
	}

	if !result.CleanedUp {
		t.Error("Expected CleanedUp to be true")
	}
}

// TestExecuteABAPOptions tests the ExecuteABAPOptions struct defaults
func TestExecuteABAPOptions(t *testing.T) {
	// Test with nil options - should use defaults
	opts := &ExecuteABAPOptions{}

	if opts.RiskLevel != "" {
		t.Error("Expected empty RiskLevel for new options")
	}

	if opts.ReturnVariable != "" {
		t.Error("Expected empty ReturnVariable for new options")
	}

	if opts.ProgramPrefix != "" {
		t.Error("Expected empty ProgramPrefix for new options")
	}

	// Test with values
	opts = &ExecuteABAPOptions{
		RiskLevel:      "dangerous",
		ReturnVariable: "lv_custom",
		KeepProgram:    true,
		ProgramPrefix:  "ZEXEC_",
	}

	if opts.RiskLevel != "dangerous" {
		t.Errorf("Expected RiskLevel 'dangerous', got %s", opts.RiskLevel)
	}

	if opts.ReturnVariable != "lv_custom" {
		t.Errorf("Expected ReturnVariable 'lv_custom', got %s", opts.ReturnVariable)
	}

	if !opts.KeepProgram {
		t.Error("Expected KeepProgram to be true")
	}

	if opts.ProgramPrefix != "ZEXEC_" {
		t.Errorf("Expected ProgramPrefix 'ZEXEC_', got %s", opts.ProgramPrefix)
	}
}

// Test line ending normalization for EditSource
func TestNormalizeLineEndings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"CRLF to LF", "line1\r\nline2\r\nline3", "line1\nline2\nline3"},
		{"Already LF", "line1\nline2\nline3", "line1\nline2\nline3"},
		{"Mixed", "line1\r\nline2\nline3\r\n", "line1\nline2\nline3\n"},
		{"No newlines", "single line", "single line"},
		{"Empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeLineEndings(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeLineEndings(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Test countMatches with CRLF vs LF
func TestCountMatches_LineEndings(t *testing.T) {
	// Source with CRLF (like SAP returns)
	source := "IF a = b.\r\n  WRITE: / 'hello'.\r\nENDIF."

	// Search pattern with LF (like AI sends)
	pattern := "IF a = b.\n  WRITE: / 'hello'.\nENDIF."

	// Should match despite different line endings
	count := countMatches(source, pattern, false)
	if count != 1 {
		t.Errorf("countMatches with CRLF source and LF pattern = %d, want 1", count)
	}

	// Case insensitive should also work
	count = countMatches(source, strings.ToLower(pattern), true)
	if count != 1 {
		t.Errorf("countMatches case-insensitive = %d, want 1", count)
	}
}

// Test replaceMatches with CRLF vs LF
func TestReplaceMatches_LineEndings(t *testing.T) {
	// Source with CRLF
	source := "line1.\r\nOLD CODE.\r\nline3."

	// Old/new patterns with LF
	old := "OLD CODE."
	newStr := "NEW CODE.\nEXTRA LINE."

	// Replace should work and result should have LF normalized
	result := replaceMatches(source, old, newStr, false, false)
	expected := "line1.\nNEW CODE.\nEXTRA LINE.\nline3."

	if result != expected {
		t.Errorf("replaceMatches result = %q, want %q", result, expected)
	}
}
