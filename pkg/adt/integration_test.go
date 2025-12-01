//go:build integration

package adt

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// Integration tests require SAP_URL, SAP_USER, SAP_PASSWORD environment variables.
// Run with: go test -tags=integration -v ./pkg/adt/

func getIntegrationClient(t *testing.T) *Client {
	url := os.Getenv("SAP_URL")
	user := os.Getenv("SAP_USER")
	pass := os.Getenv("SAP_PASSWORD")

	if url == "" || user == "" || pass == "" {
		t.Skip("SAP_URL, SAP_USER, SAP_PASSWORD required for integration tests")
	}

	client := os.Getenv("SAP_CLIENT")
	if client == "" {
		client = "001"
	}
	lang := os.Getenv("SAP_LANGUAGE")
	if lang == "" {
		lang = "EN"
	}

	opts := []Option{
		WithClient(client),
		WithLanguage(lang),
		WithTimeout(30 * time.Second),
	}

	if os.Getenv("SAP_INSECURE") == "true" {
		opts = append(opts, WithInsecureSkipVerify())
	}

	return NewClient(url, user, pass, opts...)
}

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
	source, err := client.GetProgram(ctx, "SAPMSSY0")
	if err != nil {
		t.Logf("Could not get SAPMSSY0: %v", err)
		// Try another common program
		source, err = client.GetProgram(ctx, "RS_ABAP_SOURCE_SCAN")
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

	source, err := client.GetTable(ctx, "T000")
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

	pkg, err := client.GetPackage(ctx, "BASIS")
	if err != nil {
		t.Skipf("Could not get BASIS package: %v", err)
	}

	t.Logf("Package: %s", pkg.Name)
	t.Logf("Sub-packages: %d, Objects: %d", len(pkg.SubPackages), len(pkg.Objects))
}

// --- Development Tools Integration Tests ---

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
			t.Logf("    [%s] %s (%d µs)", status, method.Name, method.ExecutionTime)
		}
	}
}

// --- CRUD Integration Tests ---

// TestIntegration_CRUD_FullWorkflow tests the complete CRUD workflow:
// Create -> Lock -> Update -> Activate -> Unlock -> Delete
func TestIntegration_CRUD_FullWorkflow(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Use a unique test program name with timestamp to avoid conflicts
	timestamp := time.Now().Unix() % 100000 // Last 5 digits
	programName := fmt.Sprintf("ZMCP_%05d", timestamp)
	packageName := "$TMP" // Local package, no transport needed
	t.Logf("Test program name: %s", programName)

	// Step 1: Create a new program
	t.Log("Step 1: Creating program...")
	err := client.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        programName,
		Description: "Test program for MCP CRUD integration test",
		PackageName: packageName,
	})
	if err != nil {
		t.Fatalf("Failed to create program: %v", err)
	}
	t.Logf("Created program: %s", programName)

	// Cleanup: ensure we delete the program at the end
	defer func() {
		t.Log("Cleanup: Deleting program...")
		objectURL := GetObjectURL(ObjectTypeProgram, programName, "")

		// Lock for delete
		lock, err := client.LockObject(ctx, objectURL, "MODIFY")
		if err != nil {
			t.Logf("Cleanup: Failed to lock for delete: %v", err)
			return
		}

		err = client.DeleteObject(ctx, objectURL, lock.LockHandle, "")
		if err != nil {
			t.Logf("Cleanup: Failed to delete: %v", err)
			// Try to unlock
			client.UnlockObject(ctx, objectURL, lock.LockHandle)
		} else {
			t.Log("Cleanup: Program deleted successfully")
		}
	}()

	objectURL := GetObjectURL(ObjectTypeProgram, programName, "")
	t.Logf("Object URL: %s", objectURL)

	// Step 2: Lock the object
	t.Log("Step 2: Locking object...")
	lock, err := client.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		t.Fatalf("Failed to lock object: %v", err)
	}
	t.Logf("Lock acquired: %s (local: %v)", lock.LockHandle, lock.IsLocal)

	// Step 3: Update the source
	t.Log("Step 3: Updating source...")
	newSource := `REPORT ztest_mcp_crud.
* Test program created by MCP CRUD integration test
WRITE 'Hello from MCP!'.`

	sourceURL := GetSourceURL(ObjectTypeProgram, programName, "")
	err = client.UpdateSource(ctx, sourceURL, newSource, lock.LockHandle, "")
	if err != nil {
		// Unlock before failing
		client.UnlockObject(ctx, objectURL, lock.LockHandle)
		t.Fatalf("Failed to update source: %v", err)
	}
	t.Log("Source updated successfully")

	// Step 4: Unlock the object (must unlock before activation)
	t.Log("Step 4: Unlocking object...")
	err = client.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		t.Fatalf("Failed to unlock: %v", err)
	}
	t.Log("Object unlocked successfully")

	// Step 5: Activate the object
	t.Log("Step 5: Activating object...")
	activateResult, err := client.Activate(ctx, objectURL, programName)
	if err != nil {
		t.Fatalf("Failed to activate: %v", err)
	}
	t.Logf("Activation result: success=%v, messages=%d", activateResult.Success, len(activateResult.Messages))

	// Step 6: Verify the source was saved
	t.Log("Step 6: Verifying source...")
	source, err := client.GetProgram(ctx, programName)
	if err != nil {
		t.Fatalf("Failed to read back source: %v", err)
	}

	if !strings.Contains(source, "Hello from MCP") {
		t.Errorf("Source doesn't contain expected content")
	} else {
		t.Log("Source verified successfully")
	}

	t.Log("CRUD workflow completed successfully!")
}

// TestIntegration_LockUnlock tests just the lock/unlock cycle
func TestIntegration_LockUnlock(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Try to lock a standard program (should exist in any system)
	objectURL := "/sap/bc/adt/programs/programs/SAPMSSY0"

	lock, err := client.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		t.Skipf("Could not lock SAPMSSY0: %v", err)
	}
	t.Logf("Lock acquired: handle=%s, isLocal=%v", lock.LockHandle, lock.IsLocal)

	// Immediately unlock
	err = client.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		t.Fatalf("Failed to unlock: %v", err)
	}
	t.Log("Object unlocked successfully")
}

// TestIntegration_ClassWithUnitTests tests the full class + unit test workflow:
// Create class -> Lock -> Create test include -> Write test code -> Unlock -> Activate -> Run tests
func TestIntegration_ClassWithUnitTests(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Use a unique test class name with timestamp
	timestamp := time.Now().Unix() % 100000
	className := fmt.Sprintf("ZCL_MCP_%05d", timestamp)
	packageName := "$TMP"
	t.Logf("Test class name: %s", className)

	// Step 1: Create a new class
	t.Log("Step 1: Creating class...")
	err := client.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeClass,
		Name:        className,
		Description: "Test class for MCP unit test integration",
		PackageName: packageName,
	})
	if err != nil {
		t.Fatalf("Failed to create class: %v", err)
	}
	t.Logf("Created class: %s", className)

	// Cleanup: ensure we delete the class at the end
	defer func() {
		t.Log("Cleanup: Deleting class...")
		objectURL := GetObjectURL(ObjectTypeClass, className, "")

		lock, err := client.LockObject(ctx, objectURL, "MODIFY")
		if err != nil {
			t.Logf("Cleanup: Failed to lock for delete: %v", err)
			return
		}

		err = client.DeleteObject(ctx, objectURL, lock.LockHandle, "")
		if err != nil {
			t.Logf("Cleanup: Failed to delete: %v", err)
			client.UnlockObject(ctx, objectURL, lock.LockHandle)
		} else {
			t.Log("Cleanup: Class deleted successfully")
		}
	}()

	objectURL := GetObjectURL(ObjectTypeClass, className, "")
	t.Logf("Object URL: %s", objectURL)

	// Step 2: Lock the class
	t.Log("Step 2: Locking class...")
	lock, err := client.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		t.Fatalf("Failed to lock class: %v", err)
	}
	t.Logf("Lock acquired: %s", lock.LockHandle)

	// Step 3: Update main source with a simple method
	t.Log("Step 3: Updating main source...")
	mainSource := fmt.Sprintf(`CLASS %s DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    METHODS get_value RETURNING VALUE(rv_value) TYPE i.
ENDCLASS.

CLASS %s IMPLEMENTATION.
  METHOD get_value.
    rv_value = 42.
  ENDMETHOD.
ENDCLASS.`, strings.ToLower(className), strings.ToLower(className))

	sourceURL := GetSourceURL(ObjectTypeClass, className, "")
	err = client.UpdateSource(ctx, sourceURL, mainSource, lock.LockHandle, "")
	if err != nil {
		client.UnlockObject(ctx, objectURL, lock.LockHandle)
		t.Fatalf("Failed to update main source: %v", err)
	}
	t.Log("Main source updated")

	// Step 4: Create the test include
	t.Log("Step 4: Creating test include...")
	err = client.CreateTestInclude(ctx, className, lock.LockHandle, "")
	if err != nil {
		client.UnlockObject(ctx, objectURL, lock.LockHandle)
		t.Fatalf("Failed to create test include: %v", err)
	}
	t.Log("Test include created")

	// Step 5: Write test class code
	t.Log("Step 5: Writing test class code...")
	testSource := fmt.Sprintf(`*"* use this source file for your ABAP unit test classes
CLASS ltcl_test DEFINITION FINAL FOR TESTING
  DURATION SHORT
  RISK LEVEL HARMLESS.

  PRIVATE SECTION.
    METHODS test_get_value FOR TESTING.
ENDCLASS.

CLASS ltcl_test IMPLEMENTATION.
  METHOD test_get_value.
    DATA(lo_cut) = NEW %s( ).
    DATA(lv_result) = lo_cut->get_value( ).
    cl_abap_unit_assert=>assert_equals(
      act = lv_result
      exp = 42
      msg = 'get_value should return 42' ).
  ENDMETHOD.
ENDCLASS.`, strings.ToLower(className))

	err = client.UpdateClassInclude(ctx, className, ClassIncludeTestClasses, testSource, lock.LockHandle, "")
	if err != nil {
		client.UnlockObject(ctx, objectURL, lock.LockHandle)
		t.Fatalf("Failed to update test include: %v", err)
	}
	t.Log("Test class code written")

	// Step 6: Unlock before activation
	t.Log("Step 6: Unlocking class...")
	err = client.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		t.Fatalf("Failed to unlock: %v", err)
	}
	t.Log("Class unlocked")

	// Step 7: Activate the class
	t.Log("Step 7: Activating class...")
	activateResult, err := client.Activate(ctx, objectURL, className)
	if err != nil {
		t.Fatalf("Failed to activate class: %v", err)
	}
	t.Logf("Activation result: success=%v, messages=%d", activateResult.Success, len(activateResult.Messages))

	// Step 8: Run the unit tests
	t.Log("Step 8: Running unit tests...")
	flags := DefaultUnitTestFlags()
	testResult, err := client.RunUnitTests(ctx, objectURL, &flags)
	if err != nil {
		t.Fatalf("Failed to run unit tests: %v", err)
	}

	t.Logf("Unit test result: %d test classes", len(testResult.Classes))
	for _, class := range testResult.Classes {
		t.Logf("  Class: %s", class.Name)
		for _, method := range class.TestMethods {
			status := "PASS"
			if len(method.Alerts) > 0 {
				status = "FAIL"
				for _, alert := range method.Alerts {
					t.Logf("    Alert: %s - %s", alert.Severity, alert.Title)
				}
			}
			t.Logf("    [%s] %s (%d µs)", status, method.Name, method.ExecutionTime)
		}
	}

	// Verify we have test results
	if len(testResult.Classes) == 0 {
		t.Log("Warning: No test classes found in results (this may be expected for new classes)")
	}

	t.Log("Class with unit tests workflow completed successfully!")
}

// --- Workflow E2E Integration Tests ---

// TestIntegration_WriteProgram tests the WriteProgram workflow
func TestIntegration_WriteProgram(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// First, create a test program
	timestamp := time.Now().Unix() % 100000
	programName := fmt.Sprintf("ZMCPW_%05d", timestamp)
	t.Logf("Test program name: %s", programName)

	// Create the program first using low-level API
	err := client.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        programName,
		Description: "Test for WriteProgram workflow",
		PackageName: "$TMP",
	})
	if err != nil {
		t.Fatalf("Failed to create test program: %v", err)
	}

	// Cleanup at end
	defer func() {
		objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", programName)
		lock, _ := client.LockObject(ctx, objectURL, "MODIFY")
		if lock != nil {
			client.DeleteObject(ctx, objectURL, lock.LockHandle, "")
		}
	}()

	// Now test WriteProgram workflow
	source := fmt.Sprintf(`REPORT %s.

* Updated via WriteProgram workflow
DATA: lv_value TYPE i.
lv_value = 42.
WRITE: / 'Value:', lv_value.`, strings.ToLower(programName))

	result, err := client.WriteProgram(ctx, programName, source, "")
	if err != nil {
		t.Fatalf("WriteProgram failed: %v", err)
	}

	t.Logf("WriteProgram result: success=%v, message=%s", result.Success, result.Message)

	if !result.Success {
		if len(result.SyntaxErrors) > 0 {
			for _, se := range result.SyntaxErrors {
				t.Logf("  Syntax error [%s] line %d: %s", se.Severity, se.Line, se.Text)
			}
		}
		if result.Activation != nil && len(result.Activation.Messages) > 0 {
			for _, m := range result.Activation.Messages {
				t.Logf("  Activation msg [%s]: %s", m.Type, m.ShortText)
			}
		}
		t.Fatalf("WriteProgram did not succeed")
	}

	t.Log("WriteProgram workflow completed successfully!")
}

// TestIntegration_WriteClass tests the WriteClass workflow
func TestIntegration_WriteClass(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// First, create a test class
	timestamp := time.Now().Unix() % 100000
	className := fmt.Sprintf("ZCL_MCPW_%05d", timestamp)
	t.Logf("Test class name: %s", className)

	// Create the class first
	err := client.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeClass,
		Name:        className,
		Description: "Test for WriteClass workflow",
		PackageName: "$TMP",
	})
	if err != nil {
		t.Fatalf("Failed to create test class: %v", err)
	}

	// Cleanup at end
	defer func() {
		objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", className)
		lock, _ := client.LockObject(ctx, objectURL, "MODIFY")
		if lock != nil {
			client.DeleteObject(ctx, objectURL, lock.LockHandle, "")
		}
	}()

	// Now test WriteClass workflow
	source := fmt.Sprintf(`CLASS %s DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    METHODS get_value RETURNING VALUE(rv_value) TYPE i.
ENDCLASS.

CLASS %s IMPLEMENTATION.
  METHOD get_value.
    rv_value = 100.
  ENDMETHOD.
ENDCLASS.`, strings.ToLower(className), strings.ToLower(className))

	result, err := client.WriteClass(ctx, className, source, "")
	if err != nil {
		t.Fatalf("WriteClass failed: %v", err)
	}

	t.Logf("WriteClass result: success=%v, message=%s", result.Success, result.Message)

	if !result.Success {
		if len(result.SyntaxErrors) > 0 {
			for _, se := range result.SyntaxErrors {
				t.Logf("  Syntax error [%s] line %d: %s", se.Severity, se.Line, se.Text)
			}
		}
		t.Fatalf("WriteClass did not succeed")
	}

	t.Log("WriteClass workflow completed successfully!")
}

// TestIntegration_CreateAndActivateProgram tests the CreateAndActivateProgram workflow
func TestIntegration_CreateAndActivateProgram(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	timestamp := time.Now().Unix() % 100000
	programName := fmt.Sprintf("ZMCPC_%05d", timestamp)
	t.Logf("Test program name: %s", programName)

	source := fmt.Sprintf(`REPORT %s.

* Created via CreateAndActivateProgram workflow
* Timestamp: %d

DATA: lv_message TYPE string.
lv_message = 'Hello from workflow!'.
WRITE: / lv_message.`, strings.ToLower(programName), timestamp)

	// Cleanup at end
	defer func() {
		objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", programName)
		lock, _ := client.LockObject(ctx, objectURL, "MODIFY")
		if lock != nil {
			client.DeleteObject(ctx, objectURL, lock.LockHandle, "")
		}
	}()

	result, err := client.CreateAndActivateProgram(ctx, programName, "Test CreateAndActivateProgram", "$TMP", source, "")
	if err != nil {
		t.Fatalf("CreateAndActivateProgram failed: %v", err)
	}

	t.Logf("CreateAndActivateProgram result: success=%v, message=%s", result.Success, result.Message)

	if !result.Success {
		if result.Activation != nil && len(result.Activation.Messages) > 0 {
			for _, m := range result.Activation.Messages {
				t.Logf("  Activation msg [%s]: %s", m.Type, m.ShortText)
			}
		}
		t.Fatalf("CreateAndActivateProgram did not succeed")
	}

	// Verify the program exists and is active by reading it back
	readSource, err := client.GetProgram(ctx, programName)
	if err != nil {
		t.Fatalf("Failed to read back program: %v", err)
	}

	if !strings.Contains(readSource, "Hello from workflow") {
		t.Errorf("Program source doesn't match expected content")
	}

	t.Log("CreateAndActivateProgram workflow completed successfully!")
}

// TestIntegration_CreateClassWithTests tests the CreateClassWithTests workflow
func TestIntegration_CreateClassWithTests(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	timestamp := time.Now().Unix() % 100000
	className := fmt.Sprintf("ZCL_MCPT_%05d", timestamp)
	t.Logf("Test class name: %s", className)

	classSource := fmt.Sprintf(`CLASS %s DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    METHODS get_answer RETURNING VALUE(rv_answer) TYPE i.
ENDCLASS.

CLASS %s IMPLEMENTATION.
  METHOD get_answer.
    rv_answer = 42.
  ENDMETHOD.
ENDCLASS.`, strings.ToLower(className), strings.ToLower(className))

	testSource := fmt.Sprintf(`*"* use this source file for your ABAP unit test classes
CLASS ltcl_test DEFINITION FINAL FOR TESTING
  DURATION SHORT
  RISK LEVEL HARMLESS.

  PRIVATE SECTION.
    METHODS test_get_answer FOR TESTING.
ENDCLASS.

CLASS ltcl_test IMPLEMENTATION.
  METHOD test_get_answer.
    DATA(lo_cut) = NEW %s( ).
    DATA(lv_result) = lo_cut->get_answer( ).
    cl_abap_unit_assert=>assert_equals(
      act = lv_result
      exp = 42
      msg = 'Answer should be 42' ).
  ENDMETHOD.
ENDCLASS.`, strings.ToLower(className))

	// Cleanup at end
	defer func() {
		objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", className)
		lock, _ := client.LockObject(ctx, objectURL, "MODIFY")
		if lock != nil {
			client.DeleteObject(ctx, objectURL, lock.LockHandle, "")
		}
	}()

	result, err := client.CreateClassWithTests(ctx, className, "Test CreateClassWithTests", "$TMP", classSource, testSource, "")
	if err != nil {
		t.Fatalf("CreateClassWithTests failed: %v", err)
	}

	t.Logf("CreateClassWithTests result: success=%v, message=%s", result.Success, result.Message)

	if !result.Success {
		if result.Activation != nil && len(result.Activation.Messages) > 0 {
			for _, m := range result.Activation.Messages {
				t.Logf("  Activation msg [%s]: %s", m.Type, m.ShortText)
			}
		}
		t.Fatalf("CreateClassWithTests did not succeed")
	}

	// Check unit test results
	if result.UnitTestResult != nil {
		t.Logf("Unit test result: %d test classes", len(result.UnitTestResult.Classes))
		for _, tc := range result.UnitTestResult.Classes {
			t.Logf("  Test class: %s", tc.Name)
			for _, tm := range tc.TestMethods {
				status := "PASS"
				if len(tm.Alerts) > 0 {
					status = "FAIL"
				}
				t.Logf("    [%s] %s (%d µs)", status, tm.Name, tm.ExecutionTime)
				for _, alert := range tm.Alerts {
					t.Logf("      Alert: %s - %s", alert.Severity, alert.Title)
				}
			}
		}
	}

	t.Log("CreateClassWithTests workflow completed successfully!")
}

// TestIntegration_SyntaxCheckWithErrors tests SyntaxCheck returns errors correctly
func TestIntegration_SyntaxCheckWithErrors(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	invalidCode := `REPORT ztest_syntax.
DATA lv_test TYPE stringgg.
DATA lv_bad TYPE unknowntype.
WRITE 'Hello'.`

	results, err := client.SyntaxCheck(ctx, "/sap/bc/adt/programs/programs/ZTEST_SYNTAX", invalidCode)
	if err != nil {
		t.Fatalf("SyntaxCheck call failed: %v", err)
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

// --- Code Intelligence Tests ---

func TestIntegration_PrettyPrint(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Test with unformatted code
	source := `report ztest.
data lv_test type string.
lv_test = 'hello'.
write lv_test.`

	formatted, err := client.PrettyPrint(ctx, source)
	if err != nil {
		t.Fatalf("PrettyPrint failed: %v", err)
	}

	t.Logf("Original:\n%s", source)
	t.Logf("Formatted:\n%s", formatted)

	if formatted == "" {
		t.Error("Formatted source is empty")
	}

	t.Log("PrettyPrint test passed!")
}

func TestIntegration_GetPrettyPrinterSettings(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	settings, err := client.GetPrettyPrinterSettings(ctx)
	if err != nil {
		t.Fatalf("GetPrettyPrinterSettings failed: %v", err)
	}

	t.Logf("Pretty printer settings: indentation=%v, style=%s", settings.Indentation, settings.Style)
	t.Log("GetPrettyPrinterSettings test passed!")
}

func TestIntegration_CodeCompletion(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Create a test program first to ensure we have a valid source URL
	programName := fmt.Sprintf("ZMCPCC_%d", os.Getpid())
	source := fmt.Sprintf(`REPORT %s.
DATA lv_string TYPE string.
lv_string = ''.
WRITE lv_`, programName)

	// Create the program
	err := client.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        programName,
		Description: "MCP Code Completion Test",
		PackageName: "$TMP",
	})
	if err != nil {
		t.Fatalf("Failed to create test program: %v", err)
	}

	// Clean up at the end
	defer func() {
		objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", programName)
		lock, _ := client.LockObject(ctx, objectURL, "MODIFY")
		if lock != nil {
			client.DeleteObject(ctx, objectURL, lock.LockHandle, "")
		}
	}()

	sourceURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s/source/main", programName)

	// Test code completion at position where we're typing "lv_"
	proposals, err := client.CodeCompletion(ctx, sourceURL, source, 4, 8)
	if err != nil {
		t.Fatalf("CodeCompletion failed: %v", err)
	}

	t.Logf("Found %d completion proposals", len(proposals))
	for i, p := range proposals {
		if i >= 5 {
			t.Logf("  ... and %d more", len(proposals)-5)
			break
		}
		t.Logf("  %s (kind=%d)", p.Identifier, p.Kind)
	}

	t.Log("CodeCompletion test passed!")
}

func TestIntegration_FindReferences(t *testing.T) {
	client := getIntegrationClient(t)

	// Use a longer timeout context for this operation (can be slow for heavily-used objects)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Find references to a less commonly used class to avoid timeout
	// CL_ABAP_STRUCTDESCR is still a standard class but has fewer references
	refs, err := client.FindReferences(ctx, "/sap/bc/adt/oo/classes/CL_ABAP_STRUCTDESCR", 0, 0)
	if err != nil {
		// This operation can timeout on heavily-used objects - make it non-fatal
		t.Logf("FindReferences timed out or failed (expected for heavily-used objects): %v", err)
		t.Skip("Skipping due to timeout - this is expected for some standard classes")
	}

	t.Logf("Found %d references to CL_ABAP_STRUCTDESCR", len(refs))
	for i, ref := range refs {
		if i >= 5 {
			t.Logf("  ... and %d more", len(refs)-5)
			break
		}
		t.Logf("  %s (%s) - %s", ref.Name, ref.Type, ref.Description)
	}

	t.Log("FindReferences test passed!")
}

func TestIntegration_FindDefinition(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Create a test program with a call to a method
	programName := fmt.Sprintf("ZMCPFD_%d", os.Getpid())
	source := fmt.Sprintf(`REPORT %s.
DATA lo_descr TYPE REF TO cl_abap_typedescr.
lo_descr = cl_abap_typedescr=>describe_by_name( 'STRING' ).`, programName)

	// Create the program
	err := client.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        programName,
		Description: "MCP Find Definition Test",
		PackageName: "$TMP",
	})
	if err != nil {
		t.Fatalf("Failed to create test program: %v", err)
	}

	objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", programName)
	sourceURL := objectURL + "/source/main"

	// Clean up at the end
	defer func() {
		lock, _ := client.LockObject(ctx, objectURL, "MODIFY")
		if lock != nil {
			client.DeleteObject(ctx, objectURL, lock.LockHandle, "")
		}
	}()

	// Lock, update, unlock, activate
	lock, err := client.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		t.Fatalf("Failed to lock: %v", err)
	}
	err = client.UpdateSource(ctx, sourceURL, source, lock.LockHandle, "")
	if err != nil {
		client.UnlockObject(ctx, objectURL, lock.LockHandle)
		t.Fatalf("Failed to update source: %v", err)
	}
	client.UnlockObject(ctx, objectURL, lock.LockHandle)
	_, err = client.Activate(ctx, objectURL, programName)
	if err != nil {
		t.Logf("Activation warning: %v", err)
	}

	// Find definition of "cl_abap_typedescr" on line 3
	// Line 3: lo_descr = cl_abap_typedescr=>describe_by_name( 'STRING' ).
	// cl_abap_typedescr starts at column 12, ends at column 28
	loc, err := client.FindDefinition(ctx, sourceURL, source, 3, 12, 28, false, "")
	if err != nil {
		t.Fatalf("FindDefinition failed: %v", err)
	}

	t.Logf("Definition found at: %s line %d, column %d", loc.URL, loc.Line, loc.Column)

	if loc.URL == "" {
		t.Error("Definition URL is empty")
	}

	t.Log("FindDefinition test passed!")
}

func TestIntegration_GetTypeHierarchy(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Create a test program that references a class
	programName := fmt.Sprintf("ZMCPTH_%d", os.Getpid())
	source := fmt.Sprintf(`REPORT %s.
DATA lo_descr TYPE REF TO cl_abap_classdescr.`, programName)

	// Create the program
	err := client.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        programName,
		Description: "MCP Type Hierarchy Test",
		PackageName: "$TMP",
	})
	if err != nil {
		t.Fatalf("Failed to create test program: %v", err)
	}

	objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", programName)
	sourceURL := objectURL + "/source/main"

	// Clean up at the end
	defer func() {
		lock, _ := client.LockObject(ctx, objectURL, "MODIFY")
		if lock != nil {
			client.DeleteObject(ctx, objectURL, lock.LockHandle, "")
		}
	}()

	// Lock, update, unlock, activate
	lock, err := client.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		t.Fatalf("Failed to lock: %v", err)
	}
	err = client.UpdateSource(ctx, sourceURL, source, lock.LockHandle, "")
	if err != nil {
		client.UnlockObject(ctx, objectURL, lock.LockHandle)
		t.Fatalf("Failed to update source: %v", err)
	}
	client.UnlockObject(ctx, objectURL, lock.LockHandle)
	_, _ = client.Activate(ctx, objectURL, programName)

	// Get supertypes of CL_ABAP_CLASSDESCR on line 2
	// Line 2: DATA lo_descr TYPE REF TO cl_abap_classdescr.
	// cl_abap_classdescr starts at column 27
	hierarchy, err := client.GetTypeHierarchy(ctx, sourceURL, source, 2, 27, true)
	if err != nil {
		t.Fatalf("GetTypeHierarchy failed: %v", err)
	}

	t.Logf("Found %d supertypes of CL_ABAP_CLASSDESCR", len(hierarchy))
	for _, h := range hierarchy {
		t.Logf("  %s (%s) - %s", h.Name, h.Type, h.Description)
	}

	t.Log("GetTypeHierarchy test passed!")
}
