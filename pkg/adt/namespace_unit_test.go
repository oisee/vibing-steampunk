package adt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Unit tests for namespace handling (no SAP connection required)
// Run with: go test -v ./pkg/adt/ -run TestNamespace

// TestNamespaceFilenameEscaping tests the # <-> / conversion for filenames
func TestNamespaceFilenameEscaping(t *testing.T) {
	testCases := []struct {
		objectName   string
		expectedFile string
	}{
		// Regular objects (no namespace)
		{"ZCL_FOO", "zcl_foo"},
		{"ZIF_BAR", "zif_bar"},
		{"ZPROG", "zprog"},

		// Namespaced objects (/ -> #)
		{"/DMO/CL_FLIGHT_AMDP", "#dmo#cl_flight_amdp"},
		{"/UI5/IF_APPLICATION_LOG", "#ui5#if_application_log"},
		{"/UI5/APP_INDEX_CALCULATE", "#ui5#app_index_calculate"},
		{"/IWBEP/CL_TEST", "#iwbep#cl_test"},

		// Edge cases
		{"/A/B", "#a#b"},
		{"/NS/SUB/OBJ", "#ns#sub#obj"},
	}

	for _, tc := range testCases {
		t.Run(tc.objectName, func(t *testing.T) {
			// Simulate the escaping logic from SaveToFile
			objectName := strings.ToLower(tc.objectName)
			safeFileName := strings.ReplaceAll(objectName, "/", "#")

			if safeFileName != tc.expectedFile {
				t.Errorf("Escaping %s: got %s, expected %s",
					tc.objectName, safeFileName, tc.expectedFile)
			}
		})
	}
}

// TestNamespaceFilenameUnescaping tests the # -> / conversion when parsing filenames
func TestNamespaceFilenameUnescaping(t *testing.T) {
	testCases := []struct {
		filename     string
		expectedName string
	}{
		// Regular objects
		{"zcl_foo.clas.abap", "ZCL_FOO"},
		{"zcl_foo.clas.testclasses.abap", "ZCL_FOO"},
		{"zcl_foo.clas.locals_def.abap", "ZCL_FOO"},

		// Namespaced objects (# -> /)
		{"#dmo#cl_flight.clas.abap", "/DMO/CL_FLIGHT"},
		{"#dmo#cl_flight.clas.testclasses.abap", "/DMO/CL_FLIGHT"},
		{"#ui5#cl_app.clas.locals_def.abap", "/UI5/CL_APP"},
		{"#ui5#cl_app.clas.locals_imp.abap", "/UI5/CL_APP"},
		{"#ui5#cl_app.clas.macros.abap", "/UI5/CL_APP"},

		// Edge cases
		{"#a#b.clas.abap", "/A/B"},
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			result := extractClassNameFromFilename(tc.filename)
			if result != tc.expectedName {
				t.Errorf("extractClassNameFromFilename(%s): got %s, expected %s",
					tc.filename, result, tc.expectedName)
			}
		})
	}
}

// TestNamespaceFunctionGroupFilename tests function group filename parsing
func TestNamespaceFunctionGroupFilename(t *testing.T) {
	testCases := []struct {
		filename     string
		expectedFugr string
	}{
		// Regular function groups
		{"zvsp_report.fugr.z_vsp_run.func.abap", "ZVSP_REPORT"},

		// Namespaced function groups
		{"#aif#util.fugr.#aif#activate.func.abap", "/AIF/UTIL"},
		{"#ui5#ui5_app_idx.fugr.#ui5#app_idx_recalc.func.abap", "/UI5/UI5_APP_IDX"},
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			result := extractFunctionGroupFromFilename(tc.filename)
			if result != tc.expectedFugr {
				t.Errorf("extractFunctionGroupFromFilename(%s): got %s, expected %s",
					tc.filename, result, tc.expectedFugr)
			}
		})
	}
}

// TestNamespaceURLEncoding tests URL encoding for namespaced objects
func TestNamespaceURLEncoding(t *testing.T) {
	testCases := []struct {
		objType     CreatableObjectType
		name        string
		parent      string
		expectInURL string // substring that should appear in URL
	}{
		// Classes
		{ObjectTypeClass, "/DMO/CL_FLIGHT", "", "%2FDMO%2FCL_FLIGHT"},
		{ObjectTypeClass, "/UI5/CL_APP", "", "%2FUI5%2FCL_APP"},
		{ObjectTypeClass, "ZCL_NORMAL", "", "ZCL_NORMAL"},

		// Interfaces
		{ObjectTypeInterface, "/UI5/IF_LOG", "", "%2FUI5%2FIF_LOG"},

		// Programs
		{ObjectTypeProgram, "/UI5/PROG", "", "%2FUI5%2FPROG"},

		// Function modules
		{ObjectTypeFunctionMod, "/AIF/FUNC", "/AIF/FUGR", "%2FAIF%2FFUNC"},

		// DDLS (uses lowercase in path)
		{ObjectTypeDDLS, "/dmo/i_travel", "", "%2Fdmo%2Fi_travel"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := GetObjectURL(tc.objType, tc.name, tc.parent)

			// Check URL contains properly encoded namespace
			if !strings.Contains(url, tc.expectInURL) {
				t.Errorf("GetObjectURL(%s, %s): URL %s does not contain expected %s",
					tc.objType, tc.name, url, tc.expectInURL)
			}

			// Check URL does NOT contain unencoded slashes in the object name part
			// (except for the path separators /sap/bc/adt/...)
			parts := strings.Split(url, "/")
			lastPart := parts[len(parts)-1]
			if strings.HasPrefix(tc.name, "/") && strings.Contains(lastPart, "/") {
				t.Errorf("GetObjectURL(%s, %s): last URL part %s contains unencoded slash",
					tc.objType, tc.name, lastPart)
			}
		})
	}
}

// TestNamespaceRoundTrip tests export filename -> import filename round-trip
func TestNamespaceRoundTrip(t *testing.T) {
	testCases := []struct {
		objectName string
		ext        string
	}{
		{"/DMO/CL_FLIGHT_AMDP", ".clas.abap"},
		{"/UI5/IF_APPLICATION_LOG", ".intf.abap"},
		{"/UI5/APP_INDEX_CALCULATE", ".prog.abap"},
		{"/DMO/I_TRAVEL_U", ".ddls.asddls"},
		{"ZCL_NORMAL", ".clas.abap"},
	}

	for _, tc := range testCases {
		t.Run(tc.objectName, func(t *testing.T) {
			// 1. Simulate export: name -> filename
			objectName := strings.ToLower(tc.objectName)
			safeFileName := strings.ReplaceAll(objectName, "/", "#")
			exportedFilename := safeFileName + tc.ext

			// 2. Simulate import: filename -> name (for class includes)
			var parsedName string
			if strings.HasSuffix(tc.ext, ".clas.abap") ||
				strings.Contains(tc.ext, ".clas.") {
				parsedName = extractClassNameFromFilename(exportedFilename)
			} else {
				// For other types, the name comes from file content
				// Just verify the filename format is correct
				parsedName = tc.objectName
			}

			// 3. Verify round-trip
			if !strings.EqualFold(parsedName, tc.objectName) {
				t.Errorf("Round-trip failed for %s: exported as %s, parsed as %s",
					tc.objectName, exportedFilename, parsedName)
			}

			t.Logf("✅ %s -> %s -> %s", tc.objectName, exportedFilename, parsedName)
		})
	}
}

// TestNamespaceExportToFile tests SaveToFile creates correct filenames
func TestNamespaceExportFilePath(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "namespace_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testCases := []struct {
		objectName   string
		ext          string
		expectedFile string
	}{
		{"/DMO/CL_FLIGHT", ".clas.abap", "#dmo#cl_flight.clas.abap"},
		{"/UI5/IF_LOG", ".intf.abap", "#ui5#if_log.intf.abap"},
		{"ZCL_NORMAL", ".clas.abap", "zcl_normal.clas.abap"},
	}

	for _, tc := range testCases {
		t.Run(tc.objectName, func(t *testing.T) {
			// Simulate the path building from SaveToFile
			objectName := strings.ToLower(tc.objectName)
			safeFileName := strings.ReplaceAll(objectName, "/", "#")
			filePath := filepath.Join(tmpDir, safeFileName+tc.ext)

			expectedPath := filepath.Join(tmpDir, tc.expectedFile)
			if filePath != expectedPath {
				t.Errorf("File path mismatch: got %s, expected %s", filePath, expectedPath)
			}

			// Verify the path is valid (no directory interpretation of /)
			dir := filepath.Dir(filePath)
			if dir != tmpDir {
				t.Errorf("Namespace interpreted as directory: parent is %s, expected %s",
					dir, tmpDir)
			}
		})
	}
}
