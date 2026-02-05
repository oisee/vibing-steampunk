//go:build integration

package adt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Namespace regression tests for Issue #18
// Tests read, export, and import operations with namespaced objects
// Run with: go test -tags=integration -v ./pkg/adt/ -run TestNamespace

// --- Test Data: Known SAP namespaced objects ---

var namespacedTestObjects = []struct {
	objType     string
	name        string
	parent      string // for FUNC
	description string
}{
	// Classes
	{"CLAS", "/DMO/CL_FLIGHT_AMDP", "", "DMO Flight AMDP class"},
	{"CLAS", "/UI5/CL_ABAP_MESSAGE", "", "UI5 ABAP Message class"},

	// Interfaces
	{"INTF", "/UI5/IF_APPLICATION_LOG", "", "UI5 Application Log interface"},

	// Programs
	{"PROG", "/UI5/APP_INDEX_CALCULATE", "", "UI5 App Index Calculate program"},

	// Function Modules
	{"FUNC", "/AIF/ACTIVATE_DESTI_STRUCT", "/AIF/UTIL", "AIF function module"},

	// CDS Views (DDLS)
	{"DDLS", "/DMO/I_TRAVEL_U", "", "DMO Travel CDS view"},

	// Behavior Definitions (BDEF)
	{"BDEF", "/DMO/I_TRAVEL_M", "", "DMO Travel behavior definition"},
}

// --- Read Operations Tests ---

func TestNamespace_GetSource_Class(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	testCases := []struct {
		name     string
		expected string // substring expected in source
	}{
		{"/DMO/CL_FLIGHT_AMDP", "convert_currency"},
		{"/UI5/CL_ABAP_MESSAGE", "message"}, // generic check - class exists and has content
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			source, err := client.GetClassSource(ctx, tc.name)
			if err != nil {
				t.Fatalf("GetClassSource(%s) failed: %v", tc.name, err)
			}
			if source == "" {
				t.Errorf("GetClassSource(%s) returned empty source", tc.name)
			}
			if tc.expected != "" && !strings.Contains(strings.ToLower(source), strings.ToLower(tc.expected)) {
				t.Errorf("GetClassSource(%s) source does not contain expected '%s'", tc.name, tc.expected)
			}
			t.Logf("✅ GetClassSource(%s): %d bytes", tc.name, len(source))
		})
	}
}

func TestNamespace_GetSource_Interface(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	testCases := []struct {
		name     string
		expected string
	}{
		{"/UI5/IF_APPLICATION_LOG", "LOG_ERROR"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			source, err := client.GetInterface(ctx, tc.name)
			if err != nil {
				t.Fatalf("GetInterface(%s) failed: %v", tc.name, err)
			}
			if source == "" {
				t.Errorf("GetInterface(%s) returned empty source", tc.name)
			}
			if tc.expected != "" && !strings.Contains(strings.ToLower(source), strings.ToLower(tc.expected)) {
				t.Errorf("GetInterface(%s) source does not contain expected '%s'", tc.name, tc.expected)
			}
			t.Logf("✅ GetInterface(%s): %d bytes", tc.name, len(source))
		})
	}
}

func TestNamespace_GetSource_Program(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	testCases := []struct {
		name     string
		expected string
	}{
		{"/UI5/APP_INDEX_CALCULATE", "REPORT"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			source, err := client.GetProgram(ctx, tc.name)
			if err != nil {
				t.Fatalf("GetProgram(%s) failed: %v", tc.name, err)
			}
			if source == "" {
				t.Errorf("GetProgram(%s) returned empty source", tc.name)
			}
			if tc.expected != "" && !strings.Contains(strings.ToUpper(source), strings.ToUpper(tc.expected)) {
				t.Errorf("GetProgram(%s) source does not contain expected '%s'", tc.name, tc.expected)
			}
			t.Logf("✅ GetProgram(%s): %d bytes", tc.name, len(source))
		})
	}
}

func TestNamespace_GetSource_Function(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	testCases := []struct {
		funcName  string
		groupName string
		expected  string
	}{
		{"/AIF/ACTIVATE_DESTI_STRUCT", "/AIF/UTIL", "FUNCTION"},
	}

	for _, tc := range testCases {
		t.Run(tc.funcName, func(t *testing.T) {
			source, err := client.GetFunction(ctx, tc.funcName, tc.groupName)
			if err != nil {
				t.Fatalf("GetFunction(%s, %s) failed: %v", tc.funcName, tc.groupName, err)
			}
			if source == "" {
				t.Errorf("GetFunction(%s) returned empty source", tc.funcName)
			}
			if tc.expected != "" && !strings.Contains(strings.ToUpper(source), strings.ToUpper(tc.expected)) {
				t.Errorf("GetFunction(%s) source does not contain expected '%s'", tc.funcName, tc.expected)
			}
			t.Logf("✅ GetFunction(%s): %d bytes", tc.funcName, len(source))
		})
	}
}

func TestNamespace_GetSource_DDLS(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	testCases := []struct {
		name     string
		expected string
	}{
		{"/DMO/I_TRAVEL_U", "define"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			source, err := client.GetDDLS(ctx, tc.name)
			if err != nil {
				t.Fatalf("GetDDLS(%s) failed: %v", tc.name, err)
			}
			if source == "" {
				t.Errorf("GetDDLS(%s) returned empty source", tc.name)
			}
			if tc.expected != "" && !strings.Contains(strings.ToLower(source), strings.ToLower(tc.expected)) {
				t.Errorf("GetDDLS(%s) source does not contain expected '%s'", tc.name, tc.expected)
			}
			t.Logf("✅ GetDDLS(%s): %d bytes", tc.name, len(source))
		})
	}
}

func TestNamespace_GetSource_BDEF(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	testCases := []struct {
		name     string
		expected string
	}{
		{"/DMO/I_TRAVEL_M", "managed"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			source, err := client.GetBDEF(ctx, tc.name)
			if err != nil {
				t.Fatalf("GetBDEF(%s) failed: %v", tc.name, err)
			}
			if source == "" {
				t.Errorf("GetBDEF(%s) returned empty source", tc.name)
			}
			if tc.expected != "" && !strings.Contains(strings.ToLower(source), strings.ToLower(tc.expected)) {
				t.Errorf("GetBDEF(%s) source does not contain expected '%s'", tc.name, tc.expected)
			}
			t.Logf("✅ GetBDEF(%s): %d bytes", tc.name, len(source))
		})
	}
}

// --- Export Operations Tests ---

func TestNamespace_ExportToFile(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Create temp directory for exports
	tmpDir, err := os.MkdirTemp("", "namespace_export_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testCases := []struct {
		objType      CreatableObjectType
		name         string
		expectedFile string // expected filename (with # for namespace)
	}{
		{ObjectTypeClass, "/DMO/CL_FLIGHT_AMDP", "#dmo#cl_flight_amdp.clas.abap"},
		{ObjectTypeInterface, "/UI5/IF_APPLICATION_LOG", "#ui5#if_application_log.intf.abap"},
		{ObjectTypeProgram, "/UI5/APP_INDEX_CALCULATE", "#ui5#app_index_calculate.prog.abap"},
		{ObjectTypeDDLS, "/DMO/I_TRAVEL_U", "#dmo#i_travel_u.ddls.asddls"},
		{ObjectTypeBDEF, "/DMO/I_TRAVEL_M", "#dmo#i_travel_m.bdef.asbdef"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := client.SaveToFile(ctx, tc.objType, tc.name, tmpDir)
			if err != nil {
				t.Fatalf("SaveToFile(%s, %s) failed: %v", tc.objType, tc.name, err)
			}

			if !result.Success {
				t.Fatalf("SaveToFile(%s) failed: %s", tc.name, result.Message)
			}

			// Check filename has correct format (# instead of /)
			actualFile := filepath.Base(result.FilePath)
			if actualFile != tc.expectedFile {
				t.Errorf("SaveToFile(%s) filename mismatch: got %s, expected %s",
					tc.name, actualFile, tc.expectedFile)
			}

			// Check file exists and has content
			content, err := os.ReadFile(result.FilePath)
			if err != nil {
				t.Fatalf("Failed to read exported file: %v", err)
			}
			if len(content) == 0 {
				t.Errorf("Exported file is empty: %s", result.FilePath)
			}

			t.Logf("✅ SaveToFile(%s) -> %s (%d bytes)", tc.name, actualFile, len(content))
		})
	}
}

// --- Import/Parse Operations Tests ---

func TestNamespace_ParseFilename(t *testing.T) {
	// Unit test for filename parsing (no SAP connection needed)
	testCases := []struct {
		filename     string
		expectedName string
		expectedType CreatableObjectType
	}{
		// Regular objects
		{"zcl_foo.clas.abap", "ZCL_FOO", ObjectTypeClass},
		{"zif_bar.intf.abap", "ZIF_BAR", ObjectTypeInterface},
		{"zprog.prog.abap", "ZPROG", ObjectTypeProgram},

		// Namespaced objects (# = /)
		{"#dmo#cl_flight_amdp.clas.abap", "/DMO/CL_FLIGHT_AMDP", ObjectTypeClass},
		{"#ui5#if_application_log.intf.abap", "/UI5/IF_APPLICATION_LOG", ObjectTypeInterface},
		{"#ui5#app_index_calculate.prog.abap", "/UI5/APP_INDEX_CALCULATE", ObjectTypeProgram},
		{"#dmo#i_travel_u.ddls.asddls", "/DMO/I_TRAVEL_U", ObjectTypeDDLS},
		{"#dmo#i_travel_m.bdef.asbdef", "/DMO/I_TRAVEL_M", ObjectTypeBDEF},

		// Class includes with namespace
		{"#dmo#cl_flight.clas.testclasses.abap", "/DMO/CL_FLIGHT", ObjectTypeClass},
		{"#ui5#cl_app.clas.locals_def.abap", "/UI5/CL_APP", ObjectTypeClass},
		{"#ui5#cl_app.clas.locals_imp.abap", "/UI5/CL_APP", ObjectTypeClass},
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			// Create temp file with minimal content
			tmpDir, _ := os.MkdirTemp("", "parse_test")
			defer os.RemoveAll(tmpDir)

			filePath := filepath.Join(tmpDir, tc.filename)

			// Write minimal valid content based on type
			var content string
			switch tc.expectedType {
			case ObjectTypeClass:
				if strings.Contains(tc.filename, ".testclasses.") {
					content = "CLASS ltcl_test DEFINITION FOR TESTING.\nENDCLASS.\nCLASS ltcl_test IMPLEMENTATION.\nENDCLASS."
				} else {
					content = "CLASS " + strings.ToLower(tc.expectedName) + " DEFINITION.\nENDCLASS.\nCLASS " + strings.ToLower(tc.expectedName) + " IMPLEMENTATION.\nENDCLASS."
				}
			case ObjectTypeInterface:
				content = "INTERFACE " + strings.ToLower(tc.expectedName) + " PUBLIC.\nENDINTERFACE."
			case ObjectTypeProgram:
				content = "REPORT " + strings.ToLower(tc.expectedName) + "."
			case ObjectTypeDDLS:
				content = "define view entity " + tc.expectedName + " as select from dummy { key dummy }"
			case ObjectTypeBDEF:
				content = "define behavior for " + tc.expectedName + " alias Travel\nimplementation in class " + strings.ToLower(tc.expectedName) + "_bp unique;"
			}

			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			info, err := ParseABAPFile(filePath)
			if err != nil {
				t.Fatalf("ParseABAPFile(%s) failed: %v", tc.filename, err)
			}

			// For class includes, name comes from filename
			if strings.Contains(tc.filename, ".testclasses.") ||
				strings.Contains(tc.filename, ".locals_def.") ||
				strings.Contains(tc.filename, ".locals_imp.") {
				if info.ObjectName != tc.expectedName {
					t.Errorf("ParseABAPFile(%s) name mismatch: got %s, expected %s",
						tc.filename, info.ObjectName, tc.expectedName)
				}
			}

			if info.ObjectType != tc.expectedType {
				t.Errorf("ParseABAPFile(%s) type mismatch: got %s, expected %s",
					tc.filename, info.ObjectType, tc.expectedType)
			}

			t.Logf("✅ ParseABAPFile(%s) -> %s (%s)", tc.filename, info.ObjectName, info.ObjectType)
		})
	}
}

// --- Round-trip Test (Export -> Import) ---

func TestNamespace_RoundTrip(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "namespace_roundtrip_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testCases := []struct {
		objType CreatableObjectType
		name    string
	}{
		{ObjectTypeClass, "/DMO/CL_FLIGHT_AMDP"},
		{ObjectTypeInterface, "/UI5/IF_APPLICATION_LOG"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 1. Export
			exportResult, err := client.SaveToFile(ctx, tc.objType, tc.name, tmpDir)
			if err != nil || !exportResult.Success {
				t.Fatalf("Export failed: %v - %s", err, exportResult.Message)
			}
			t.Logf("Exported to: %s", exportResult.FilePath)

			// 2. Parse the exported file
			info, err := ParseABAPFile(exportResult.FilePath)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			// 3. Verify parsed name matches original (accounting for case)
			if !strings.EqualFold(info.ObjectName, tc.name) {
				t.Errorf("Round-trip name mismatch: exported %s, parsed %s", tc.name, info.ObjectName)
			}

			// 4. Verify object type matches
			if info.ObjectType != tc.objType {
				t.Errorf("Round-trip type mismatch: expected %s, got %s", tc.objType, info.ObjectType)
			}

			t.Logf("✅ Round-trip %s: export -> parse -> %s (%s)", tc.name, info.ObjectName, info.ObjectType)
		})
	}
}

// --- URL Encoding Tests ---

func TestNamespace_URLEncoding(t *testing.T) {
	// Test that GetObjectURL properly encodes namespaced objects
	testCases := []struct {
		objType  CreatableObjectType
		name     string
		expected string // expected URL substring
	}{
		{ObjectTypeClass, "/DMO/CL_FLIGHT", "/sap/bc/adt/oo/classes/%2FDMO%2FCL_FLIGHT"},
		{ObjectTypeInterface, "/UI5/IF_LOG", "/sap/bc/adt/oo/interfaces/%2FUI5%2FIF_LOG"},
		{ObjectTypeProgram, "/UI5/PROG", "/sap/bc/adt/programs/programs/%2FUI5%2FPROG"},
		{ObjectTypeDDLS, "/dmo/i_travel", "/sap/bc/adt/ddic/ddl/sources/%2fdmo%2fi_travel"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := GetObjectURL(tc.objType, tc.name, "")
			if !strings.Contains(strings.ToLower(url), strings.ToLower(tc.expected)) {
				t.Errorf("GetObjectURL(%s, %s) = %s, expected to contain %s",
					tc.objType, tc.name, url, tc.expected)
			}
			t.Logf("✅ GetObjectURL(%s, %s) = %s", tc.objType, tc.name, url)
		})
	}
}

// --- Search Tests ---

func TestNamespace_SearchObject(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	testCases := []struct {
		query       string
		expectFound bool
	}{
		{"/DMO/*", true},
		{"/UI5/CL*", true},
		{"*UI5*IF*", true},
	}

	for _, tc := range testCases {
		t.Run(tc.query, func(t *testing.T) {
			results, err := client.SearchObject(ctx, tc.query, 10)
			if err != nil {
				t.Fatalf("SearchObject(%s) failed: %v", tc.query, err)
			}

			if tc.expectFound && len(results) == 0 {
				t.Errorf("SearchObject(%s) returned no results, expected some", tc.query)
			}

			if len(results) > 0 {
				t.Logf("✅ SearchObject(%s): found %d results", tc.query, len(results))
				for i, r := range results {
					if i >= 3 {
						t.Logf("   ... and %d more", len(results)-3)
						break
					}
					t.Logf("   - %s (%s)", r.Name, r.Type)
				}
			}
		})
	}
}
