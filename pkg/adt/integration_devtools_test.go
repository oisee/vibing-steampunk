//go:build integration

// ABOUTME: Integration tests for development tools (SyntaxCheck, UnitTests, ATC).
// ABOUTME: Tests code quality and validation tools.

package adt

import (
	"context"
	"testing"
)

func TestIntegration_SyntaxCheck(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Test with valid ABAP code - using a simple report
	validCode := `REPORT ztest_syntax.
WRITE 'Hello World'.`

	results, err := client.SyntaxCheck(ctx, "/sap/bc/adt/programs/programs/ZTEST_SYNTAX", validCode)
	if err != nil {
		t.Logf("Syntax check call failed (might be expected if program doesn't exist): %v", err)
		// Try with invalid code to at least test the endpoint
		invalidCode := `REPORT ztest_syntax.
WRITEE 'Hello World'.` // intentional typo

		results, err = client.SyntaxCheck(ctx, "/sap/bc/adt/programs/programs/ZTEST_SYNTAX", invalidCode)
		if err != nil {
			t.Skipf("Syntax check endpoint not accessible: %v", err)
		}
	}

	t.Logf("Syntax check returned %d messages", len(results))
	for i, r := range results {
		if i >= 5 {
			break
		}
		t.Logf("  [%s] Line %d: %s", r.Severity, r.Line, r.Text)
	}
}

func TestIntegration_RunUnitTests(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Try to run unit tests on CL_ABAP_UNIT_ASSERT (which might have tests)
	flags := DefaultUnitTestFlags()
	result, err := client.RunUnitTests(ctx, "/sap/bc/adt/oo/classes/CL_ABAP_UNIT_ASSERT", &flags)
	if err != nil {
		// Try another common test class
		result, err = client.RunUnitTests(ctx, "/sap/bc/adt/oo/classes/CL_ABAP_TYPEDESCR", &flags)
		if err != nil {
			t.Skipf("Could not run unit tests: %v", err)
		}
	}

	t.Logf("Unit test result: %d test classes", len(result.Classes))
	for _, class := range result.Classes {
		t.Logf("  Class: %s (%s)", class.Name, class.RiskLevel)
		for _, method := range class.TestMethods {
			status := "PASS"
			if len(method.Alerts) > 0 {
				status = "FAIL"
			}
			t.Logf("    [%s] %s (%.2f ms)", status, method.Name, method.ExecutionTime*1000)
		}
	}
}

func TestIntegration_SyntaxCheckWithErrors(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	invalidCode := `REPORT ztest_syntax.
DATA lv_test TYPE stringgg.
DATA lv_bad TYPE unknowntype.
WRITE 'Hello'.`

	results, err := client.SyntaxCheck(ctx, "/sap/bc/adt/programs/programs/ZTEST_SYNTAX", invalidCode)
	if err != nil {
		t.Skipf("SyntaxCheck call failed: %v", err)
	}

	t.Logf("SyntaxCheck found %d issues", len(results))
	for _, r := range results {
		t.Logf("  [%s] Line %d, Col %d: %s", r.Severity, r.Line, r.Offset, r.Text)
	}

	// Should have at least one error
	hasError := false
	for _, r := range results {
		if r.Severity == "E" {
			hasError = true
			break
		}
	}

	if !hasError {
		t.Error("Expected at least one syntax error for invalid code")
	}

	t.Log("SyntaxCheck error detection test passed!")
}
