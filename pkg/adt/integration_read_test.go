//go:build integration

// ABOUTME: Integration tests for read operations (GetSource, GetClass, GetTable, etc.).
// ABOUTME: Tests use standard SAP objects that exist in any system.

package adt

import (
	"context"
	"strings"
	"testing"
)

func TestIntegration_SearchObject(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	results, err := client.SearchObject(ctx, "CL_*", 10)
	if err != nil {
		t.Fatalf("SearchObject failed: %v", err)
	}

	if len(results) == 0 {
		t.Log("No results found for CL_* search")
	} else {
		t.Logf("Found %d results", len(results))
		for i, r := range results {
			if i >= 3 {
				break
			}
			t.Logf("  %s (%s) - %s", r.Name, r.Type, r.Description)
		}
	}
}

func TestIntegration_GetProgram(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Try to get a standard SAP program
	source, err := client.GetSource(ctx, "PROG", "SAPMSSY0", nil)
	if err != nil {
		t.Logf("Could not get SAPMSSY0: %v", err)
		// Try another common program
		source, err = client.GetSource(ctx, "PROG", "RS_ABAP_SOURCE_SCAN", nil)
		if err != nil {
			t.Skipf("Could not retrieve any standard program: %v", err)
		}
	}

	if len(source) == 0 {
		t.Error("Program source is empty")
	} else {
		t.Logf("Retrieved %d characters of source code", len(source))
		// Show first 200 chars
		preview := source
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		t.Logf("Preview:\n%s", preview)
	}
}

func TestIntegration_GetClass(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Try to get a standard SAP class
	sources, err := client.GetClass(ctx, "CL_ABAP_TYPEDESCR")
	if err != nil {
		t.Skipf("Could not get CL_ABAP_TYPEDESCR: %v", err)
	}

	mainSource, ok := sources["main"]
	if !ok {
		t.Error("No main source in class")
	} else if len(mainSource) == 0 {
		t.Error("Main source is empty")
	} else {
		t.Logf("Retrieved %d characters of class source", len(mainSource))
	}
}

func TestIntegration_GetTableContents(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Get contents of T000 (clients table - should exist in any system)
	contents, err := client.GetTableContents(ctx, "T000", 5, "")
	if err != nil {
		t.Skipf("Could not get T000 contents: %v", err)
	}

	t.Logf("Retrieved %d columns, %d rows", len(contents.Columns), len(contents.Rows))

	if len(contents.Columns) == 0 {
		t.Error("No columns returned")
	}
	if len(contents.Rows) == 0 {
		t.Error("No rows returned")
	} else {
		t.Logf("First row: %v", contents.Rows[0])
	}
}

func TestIntegration_GetTableContentsWithQuery(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Get contents of T000 with SQL query (must be full SELECT statement)
	contents, err := client.GetTableContents(ctx, "T000", 10, "SELECT * FROM T000 WHERE MANDT = '001'")
	if err != nil {
		t.Skipf("Could not get T000 contents with query: %v", err)
	}

	t.Logf("Retrieved %d columns, %d rows (filtered)", len(contents.Columns), len(contents.Rows))

	// All rows should have MANDT = '001'
	for i, row := range contents.Rows {
		if mandt, ok := row["MANDT"].(string); ok && mandt != "001" {
			t.Errorf("Row %d has MANDT = %s, expected 001", i, mandt)
		}
	}
}

func TestIntegration_RunQuery(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Run a simple query
	contents, err := client.RunQuery(ctx, "SELECT MANDT, MTEXT FROM T000", 10)
	if err != nil {
		t.Skipf("Could not run query: %v", err)
	}

	t.Logf("Query returned %d columns, %d rows", len(contents.Columns), len(contents.Rows))

	// Should have exactly 2 columns (MANDT and MTEXT)
	if len(contents.Columns) != 2 {
		t.Errorf("Expected 2 columns, got %d", len(contents.Columns))
	}

	if len(contents.Rows) > 0 {
		t.Logf("First row: %v", contents.Rows[0])
	}
}

func TestIntegration_GetTable(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	source, err := client.GetSource(ctx, "TABL", "T000", nil)
	if err != nil {
		t.Skipf("Could not get T000 source: %v", err)
	}

	if len(source) == 0 {
		t.Error("Table source is empty")
	} else {
		t.Logf("Retrieved %d characters of table source", len(source))
		// Show first 200 chars
		preview := source
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		t.Logf("Preview:\n%s", preview)
	}
}

func TestIntegration_GetPackage(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	pkg, err := client.GetPackage(ctx, "BASIS", nil)
	if err != nil {
		t.Skipf("Could not get BASIS package: %v", err)
	}

	t.Logf("Package: %s (total: %d, truncated: %v)", pkg.Name, pkg.TotalObjects, pkg.Truncated)
	t.Logf("Sub-packages: %d, Objects returned: %d", len(pkg.SubPackages), len(pkg.Objects))
}

func TestIntegration_GetCDSDependencies(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Test with simple SAP standard CDS view (wraps DDDDLSRC table)
	result, err := client.GetCDSDependencies(ctx, "ACM_DDDDLSRC", CDSDependencyOptions{
		DependencyLevel:  "hierarchy",
		WithAssociations: false,
	})

	if err != nil {
		t.Skipf("GetCDSDependencies failed (CDS view might not exist): %v", err)
	}

	if result.Name == "" {
		t.Error("Expected result name, got empty")
	}

	t.Logf("CDS view: %s", result.Name)
	t.Logf("Type: %s", result.Type)
	t.Logf("Activation state: %s", result.ActivationState)
	t.Logf("Children count: %d", len(result.Children))

	// Test flattening
	flat := result.FlattenDependencies()
	t.Logf("Total dependencies (flat): %d", len(flat))

	// Test type counting
	byType := result.CountDependenciesByType()
	for typ, count := range byType {
		t.Logf("  %s: %d", typ, count)
	}

	// Test table dependencies
	tables := result.GetTableDependencies()
	t.Logf("Table dependencies: %d", len(tables))
	for _, table := range tables {
		t.Logf("  - %s", table.Name)
	}

	// Test inactive dependencies
	inactive := result.GetInactiveDependencies()
	if len(inactive) > 0 {
		t.Logf("WARNING: Inactive dependencies found: %d", len(inactive))
		for _, dep := range inactive {
			t.Logf("  - %s (state: %s)", dep.Name, dep.ActivationState)
		}
	}

	// Test cycle detection
	cycles := result.FindCycles()
	if len(cycles) > 0 {
		t.Logf("WARNING: Cycles detected: %v", cycles)
	}

	// Test dependency depth
	depth := result.GetDependencyDepth()
	t.Logf("Dependency depth: %d", depth)
}

func TestIntegration_GetDDLS(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Use a standard SAP CDS view
	ddlsName := "I_ABAPPACKAGE"

	source, err := client.GetSource(ctx, "DDLS", ddlsName, nil)
	if err != nil {
		t.Skipf("GetDDLS failed: %v", err)
	}

	t.Logf("GetDDLS returned %d bytes", len(source))

	// CDS views should contain "define" keyword (view/root view entity)
	if !strings.Contains(strings.ToLower(source), "define") {
		t.Errorf("Expected CDS source to contain 'define', got:\n%s", source[:min(200, len(source))])
	}

	t.Logf("GetDDLS successful: %s", ddlsName)
}

func TestIntegration_GetBDEF(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Use a DMO RAP behavior definition
	bdefName := "/DMO/C_TRAVEL_U"

	source, err := client.GetSource(ctx, "BDEF", bdefName, nil)
	if err != nil {
		t.Skipf("GetSource BDEF failed: %v", err)
	}

	t.Logf("GetSource BDEF returned %d bytes", len(source))

	// BDEF should contain "define behavior" keyword
	if !strings.Contains(strings.ToLower(source), "define behavior") {
		t.Errorf("Expected BDEF source to contain 'define behavior', got:\n%s", source[:min(200, len(source))])
	}

	t.Logf("GetSource BDEF successful: %s", bdefName)
}

func TestIntegration_GetSRVB(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Use a DMO RAP service binding
	srvbName := "/DMO/API_TRAVEL_U_V2"

	sb, err := client.GetSRVB(ctx, srvbName)
	if err != nil {
		t.Skipf("GetSRVB failed: %v", err)
	}

	t.Logf("GetSRVB result: name=%s, type=%s, version=%s", sb.Name, sb.Type, sb.BindingVersion)

	if sb.Name == "" {
		t.Error("Expected SRVB name to be non-empty")
	}

	t.Logf("GetSRVB successful: %s", srvbName)
}

func TestIntegration_GetSource_RAP(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		objectType string
		objectName string
		contains   string
	}{
		{"DDLS", "DDLS", "I_ABAPPACKAGE", "define"}, // CDS views contain "define view" or "define root view entity"
		{"BDEF", "BDEF", "/DMO/C_TRAVEL_U", "define behavior"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source, err := client.GetSource(ctx, tc.objectType, tc.objectName, nil)
			if err != nil {
				t.Skipf("GetSource(%s, %s) failed: %v", tc.objectType, tc.objectName, err)
			}

			t.Logf("GetSource returned %d bytes", len(source))

			if !strings.Contains(strings.ToLower(source), tc.contains) {
				t.Errorf("Expected source to contain '%s'", tc.contains)
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
