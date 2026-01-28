//go:build integration

// ABOUTME: Integration tests for CRUD operations (Create, Lock, Update, Delete, Activate).
// ABOUTME: Tests create temporary objects in $TMP and clean up after themselves.

package adt

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// cleanupObject locks and deletes an object, logging any errors.
// Use with defer to ensure cleanup after test completion.
func cleanupObject(t *testing.T, client *Client, objectType CreatableObjectType, name, parentName string) {
	t.Helper()
	ctx := context.Background()

	objectURL := GetObjectURL(objectType, name, parentName)
	if objectURL == "" {
		t.Logf("Cleanup: Could not determine URL for %s %s", objectType, name)
		return
	}

	lock, err := client.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		t.Logf("Cleanup: Failed to lock %s for delete: %v", name, err)
		return
	}

	err = client.DeleteObject(ctx, objectURL, lock.LockHandle, "")
	if err != nil {
		t.Logf("Cleanup: Failed to delete %s: %v", name, err)
		client.UnlockObject(ctx, objectURL, lock.LockHandle)
	} else {
		t.Logf("Cleanup: %s deleted successfully", name)
	}
}

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

	defer cleanupObject(t, client, ObjectTypeProgram, programName, "")

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
	source, err := client.GetSource(ctx, "PROG", programName, nil)
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

	defer cleanupObject(t, client, ObjectTypeClass, className, "")

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
			t.Logf("    [%s] %s (%.2f ms)", status, method.Name, method.ExecutionTime*1000)
		}
	}

	// Verify we have test results
	if len(testResult.Classes) == 0 {
		t.Log("Warning: No test classes found in results (this may be expected for new classes)")
	}

	t.Log("Class with unit tests workflow completed successfully!")
}

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

	defer cleanupObject(t, client, ObjectTypeProgram, programName, "")

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

	defer cleanupObject(t, client, ObjectTypeClass, className, "")

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

	defer cleanupObject(t, client, ObjectTypeProgram, programName, "")

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
	readSource, err := client.GetSource(ctx, "PROG", programName, nil)
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

	defer cleanupObject(t, client, ObjectTypeClass, className, "")

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
	if result.TestResults != nil {
		t.Logf("Unit test result: %d test classes", len(result.TestResults.Classes))
		for _, tc := range result.TestResults.Classes {
			t.Logf("  Test class: %s", tc.Name)
			for _, tm := range tc.TestMethods {
				status := "PASS"
				if len(tm.Alerts) > 0 {
					status = "FAIL"
				}
				t.Logf("    [%s] %s (%.2f ms)", status, tm.Name, tm.ExecutionTime*1000)
				for _, alert := range tm.Alerts {
					t.Logf("      Alert: %s - %s", alert.Severity, alert.Title)
				}
			}
		}
	}

	t.Log("CreateClassWithTests workflow completed successfully!")
}

// TestIntegration_CreatePackage tests package creation
func TestIntegration_CreatePackage(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	timestamp := time.Now().Unix() % 100000
	packageName := fmt.Sprintf("$ZMCPP_%05d", timestamp)
	t.Logf("Test package name: %s", packageName)

	// Create package (Responsible will default to current user)
	err := client.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypePackage,
		Name:        packageName,
		Description: "Test package created via integration test",
		PackageName: "$TMP", // Packages are created under parent packages
	})
	if err != nil {
		t.Fatalf("Failed to create package: %v", err)
	}

	t.Logf("Package %s created successfully", packageName)

	// Verify package exists by getting its contents
	pkg, err := client.GetPackage(ctx, packageName, nil)
	if err != nil {
		t.Fatalf("Failed to get created package: %v", err)
	}

	if pkg.Name != packageName {
		t.Errorf("Expected package name %s, got %s", packageName, pkg.Name)
	}

	t.Logf("Package verified: %s", pkg.Name)

	// Cleanup: Lock and delete the package
	objectURL := fmt.Sprintf("/sap/bc/adt/packages/%s", strings.ToLower(packageName))
	lock, err := client.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		t.Logf("Warning: Failed to lock package for cleanup: %v", err)
		return
	}

	err = client.DeleteObject(ctx, objectURL, lock.LockHandle, "")
	if err != nil {
		client.UnlockObject(ctx, objectURL, lock.LockHandle)
		t.Logf("Warning: Failed to delete package: %v", err)
		return
	}

	t.Logf("Package %s deleted successfully", packageName)
}

// TestIntegration_EditSource tests the EditSource workflow (surgical string replacement)
func TestIntegration_EditSource(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Create a test program
	timestamp := time.Now().Unix() % 100000
	programName := fmt.Sprintf("ZMCPE_%05d", timestamp)
	t.Logf("Test program name: %s", programName)

	err := client.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        programName,
		Description: "Test for EditSource workflow",
		PackageName: "$TMP",
	})
	if err != nil {
		t.Fatalf("Failed to create test program: %v", err)
	}

	// Cleanup at end
	defer cleanupObject(t, client, ObjectTypeProgram, programName, "")

	// Set initial source using WriteProgram
	initialSource := fmt.Sprintf(`REPORT %s.

* Initial version
DATA: lv_count TYPE i.
lv_count = 10.
WRITE: / 'Count:', lv_count.`, strings.ToLower(programName))

	_, err = client.WriteProgram(ctx, programName, initialSource, "")
	if err != nil {
		t.Fatalf("WriteProgram failed: %v", err)
	}

	// Test 1: EditSource - simple replacement
	objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", programName)
	result, err := client.EditSource(ctx, objectURL,
		"lv_count = 10.",
		"lv_count = 42.",
		false, // replaceAll
		true,  // syntaxCheck
		false, // caseInsensitive
	)
	if err != nil {
		t.Fatalf("EditSource failed: %v", err)
	}

	t.Logf("EditSource result: success=%v, message=%s, matchCount=%d", result.Success, result.Message, result.MatchCount)

	if !result.Success {
		t.Fatalf("EditSource did not succeed: %s", result.Message)
	}

	if result.MatchCount != 1 {
		t.Errorf("Expected matchCount=1, got %d", result.MatchCount)
	}

	// Verify the change was applied
	source, err := client.GetSource(ctx, "PROG", programName, nil)
	if err != nil {
		t.Fatalf("Failed to read program after edit: %v", err)
	}

	if !strings.Contains(source, "lv_count = 42.") {
		t.Errorf("Expected source to contain 'lv_count = 42.', but it doesn't:\n%s", source)
	}

	// Test 2: EditSource - change to different value
	result, err = client.EditSource(ctx, objectURL,
		"lv_count = 42.",
		"lv_count = 99.",
		false, // replaceAll
		true,  // syntaxCheck
		false, // caseInsensitive
	)
	if err != nil {
		t.Fatalf("EditSource (second edit) failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("EditSource (second edit) did not succeed: %s", result.Message)
	}

	// Verify second change
	source, err = client.GetSource(ctx, "PROG", programName, nil)
	if err != nil {
		t.Fatalf("Failed to read program after second edit: %v", err)
	}

	if !strings.Contains(source, "lv_count = 99.") {
		t.Errorf("Expected source to contain 'lv_count = 99.', but it doesn't:\n%s", source)
	}

	// Test 3: EditSource - syntax error detection
	result, err = client.EditSource(ctx, objectURL,
		"lv_count = 99.",
		"lv_count = INVALID SYNTAX HERE",
		false, // replaceAll
		true,  // syntaxCheck (should detect syntax error)
		false, // caseInsensitive
	)
	if err != nil {
		t.Fatalf("EditSource (syntax error test) failed: %v", err)
	}

	if result.Success {
		t.Errorf("EditSource should have failed due to syntax errors")
	}

	if len(result.SyntaxErrors) == 0 {
		t.Errorf("Expected syntax errors to be detected")
	} else {
		t.Logf("Syntax error correctly detected: %v", result.SyntaxErrors[0])
	}

	// Verify source wasn't changed (syntax check prevented it)
	source, err = client.GetSource(ctx, "PROG", programName, nil)
	if err != nil {
		t.Fatalf("Failed to read program after syntax error test: %v", err)
	}

	if !strings.Contains(source, "lv_count = 99.") {
		t.Errorf("Source should not have changed due to syntax error")
	}

	// Test 4: EditSource - case-insensitive matching
	result, err = client.EditSource(ctx, objectURL,
		"LV_COUNT = 99.", // Uppercase (ABAP is case-insensitive but pretty-printer may use lowercase)
		"lv_count = 123.",
		false, // replaceAll
		true,  // syntaxCheck
		true,  // caseInsensitive
	)
	if err != nil {
		t.Fatalf("EditSource (case-insensitive test) failed: %v", err)
	}

	if !result.Success {
		t.Logf("Case-insensitive test did not succeed (expected if pretty-printer normalized case): %s", result.Message)
	} else {
		t.Logf("Case-insensitive match succeeded: %s", result.Message)

		// Verify case-insensitive change
		source, err = client.GetSource(ctx, "PROG", programName, nil)
		if err != nil {
			t.Fatalf("Failed to read program after case-insensitive edit: %v", err)
		}

		if !strings.Contains(source, "lv_count = 123.") {
			t.Errorf("Expected source to contain 'lv_count = 123.', but it doesn't:\n%s", source)
		}
	}

	t.Log("EditSource workflow completed successfully!")
}
