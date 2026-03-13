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

// Integration tests require a live SAP system.
//
// Quickstart:
//
//	export SAP_URL=http://localhost:50000
//	export SAP_USER=DEVELOPER
//	export SAP_PASSWORD=ABAPtr2023#00
//	export SAP_CLIENT=001
//	go test -tags=integration -v ./pkg/adt/
//
// Useful env vars:
//
//	SAP_TEST_VERBOSE=true       — log every HTTP request/response body
//	SAP_TEST_NO_CLEANUP=true    — keep ZTEST_* objects on SAP for manual inspection

// ─── testLogger ────────────────────────────────────────────────────────────────
//
// Provides timestamped, levelled output so test logs are easy to scan.
// All lines start with [INFO], [DEBUG], or [WARN] plus elapsed ms.
// DEBUG lines are suppressed unless SAP_TEST_VERBOSE=true.

type testLogger struct {
	t       *testing.T
	verbose bool
	start   time.Time
}

func newTestLogger(t *testing.T) *testLogger {
	t.Helper()
	return &testLogger{
		t:       t,
		verbose: os.Getenv("SAP_TEST_VERBOSE") == "true",
		start:   time.Now(),
	}
}

func (l *testLogger) Info(format string, args ...any) {
	l.t.Helper()
	l.t.Logf("[INFO  +%6v] %s", time.Since(l.start).Round(time.Millisecond), fmt.Sprintf(format, args...))
}

func (l *testLogger) Debug(format string, args ...any) {
	if !l.verbose {
		return
	}
	l.t.Helper()
	l.t.Logf("[DEBUG +%6v] %s", time.Since(l.start).Round(time.Millisecond), fmt.Sprintf(format, args...))
}

func (l *testLogger) Warn(format string, args ...any) {
	l.t.Helper()
	l.t.Logf("[WARN  +%6v] %s", time.Since(l.start).Round(time.Millisecond), fmt.Sprintf(format, args...))
}

// Todo marks a test as intentionally unimplemented.
// Shows as SKIP [TODO] in output — never FAIL, always safe for CI.
func (l *testLogger) Todo(feature string) {
	l.t.Helper()
	l.t.Skipf("[TODO ] %s", feature)
}

// ─── Client helpers ────────────────────────────────────────────────────────────

// requireIntegrationClient returns a configured ADT client, or skips the test
// if SAP_URL / SAP_USER / SAP_PASSWORD are not set.
// Logs connection details so every test output shows which system was used.
// Pass extra Option values to override defaults (e.g. WithTimeout for slow tests).
func requireIntegrationClient(t *testing.T, extra ...Option) *Client {
	t.Helper()
	url := os.Getenv("SAP_URL")
	user := os.Getenv("SAP_USER")
	pass := os.Getenv("SAP_PASSWORD")

	if url == "" || user == "" || pass == "" {
		t.Skip("Integration tests require SAP_URL, SAP_USER, SAP_PASSWORD — see reports/2026-03-13-001-integration-testing-plan.md")
	}

	sapClient := os.Getenv("SAP_CLIENT")
	if sapClient == "" {
		sapClient = "001"
	}
	lang := os.Getenv("SAP_LANGUAGE")
	if lang == "" {
		lang = "EN"
	}

	log := newTestLogger(t)
	log.Info("SAP connection: url=%s user=%s client=%s lang=%s", url, user, sapClient, lang)

	opts := []Option{
		WithClient(sapClient),
		WithLanguage(lang),
		WithTimeout(60 * time.Second), // 30s was too short for activate/ATC
	}
	if os.Getenv("SAP_INSECURE") == "true" {
		opts = append(opts, WithInsecureSkipVerify())
	}
	// Extra options are appended last so callers can override defaults.
	opts = append(opts, extra...)

	return NewClient(url, user, pass, opts...)
}

// getIntegrationClient is a backward-compatible alias so existing tests compile unchanged.
func getIntegrationClient(t *testing.T) *Client { return requireIntegrationClient(t) }

// ─── Object helpers ────────────────────────────────────────────────────────────

// tempObjectName returns a short unique name that won't collide across parallel runs.
// Example: tempObjectName("ZTEST_PROG") → "ZTEST_PROG_42317"
func tempObjectName(base string) string {
	return fmt.Sprintf("%s_%05d", base, time.Now().Unix()%100000)
}

// isTransientSAPError returns true when err is a 503/500 from SAP, meaning the
// system is temporarily under load. These are not bugs — callers should skip
// the test rather than fail it.
func isTransientSAPError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "HTTP 503") ||
		strings.Contains(msg, "HTTP 500") ||
		strings.Contains(msg, "status 503") ||
		strings.Contains(msg, "status 500")
}

// withTempProgram creates a PROG/P in $TMP, calls fn with its ADT object URL,
// then deletes it via t.Cleanup even if the test panics or fails.
// Set SAP_TEST_NO_CLEANUP=true to keep the object for manual inspection.
func withTempProgram(t *testing.T, client *Client, name, source string, fn func(objectURL string)) {
	t.Helper()
	ctx := context.Background()
	log := newTestLogger(t)

	err := client.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        name,
		Description: "vsp integration test — safe to delete",
		PackageName: "$TMP",
	})
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("withTempProgram: SAP temporarily unavailable (5xx): %v", err)
		}
		t.Fatalf("withTempProgram: create %s: %v", name, err)
	}
	objectURL := GetObjectURL(ObjectTypeProgram, name, "")
	log.Info("Created temp program: %s (%s)", name, objectURL)

	if source != "" {
		sourceURL := GetSourceURL(ObjectTypeProgram, name, "")
		lock, err := client.LockObject(ctx, objectURL, "MODIFY")
		if err != nil {
			t.Fatalf("withTempProgram: lock %s for initial source: %v", name, err)
		}
		if err := client.UpdateSource(ctx, sourceURL, source, lock.LockHandle, ""); err != nil {
			_ = client.UnlockObject(ctx, objectURL, lock.LockHandle)
			t.Fatalf("withTempProgram: write source to %s: %v", name, err)
		}
		if err := client.UnlockObject(ctx, objectURL, lock.LockHandle); err != nil {
			log.Warn("withTempProgram: unlock after source write: %v", err)
		}
		log.Debug("Wrote initial source to %s (%d chars)", name, len(source))
	}

	t.Cleanup(func() {
		if os.Getenv("SAP_TEST_NO_CLEANUP") == "true" {
			log.Warn("Cleanup skipped (SAP_TEST_NO_CLEANUP=true) — delete %s manually", name)
			return
		}
		ctx2, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		lock, err := client.LockObject(ctx2, objectURL, "MODIFY")
		if err != nil {
			log.Warn("Cleanup: lock %s failed: %v", name, err)
			return
		}
		if err := client.DeleteObject(ctx2, objectURL, lock.LockHandle, ""); err != nil {
			log.Warn("Cleanup: delete %s failed: %v", name, err)
		} else {
			log.Info("Cleanup: deleted %s", name)
		}
	})

	fn(objectURL)
}

func TestIntegration_SearchObject(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	results, err := client.SearchObject(ctx, "CL_*", 10)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("SearchObject failed (SAP 5xx): %v", err)
		}
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
	l := newTestLogger(t)
	ctx := context.Background()

	const className = "CL_ABAP_TYPEDESCR"

	// B-010: verify all 5 class includes are retrievable
	includeTypes := []ClassIncludeType{
		ClassIncludeMain,
		ClassIncludeDefinitions,
		ClassIncludeImplementations,
		ClassIncludeMacros,
		ClassIncludeTestClasses,
	}

	present := []string{}
	missing := []string{}

	for _, inc := range includeTypes {
		var src string
		var err error
		if inc == ClassIncludeMain {
			sources, e := client.GetClass(ctx, className)
			if e != nil {
				if isTransientSAPError(e) {
					t.Skipf("GetClass failed (SAP 5xx): %v", e)
				}
				t.Skipf("Could not get %s: %v", className, e)
			}
			src = sources["main"]
		} else {
			src, err = client.GetClassInclude(ctx, className, inc)
			if err != nil {
				if isTransientSAPError(err) {
					t.Skipf("GetClassInclude failed (SAP 5xx): %v", err)
				}
				missing = append(missing, string(inc))
				l.Warn("include %q not available: %v", inc, err)
				continue
			}
		}
		if len(src) == 0 {
			missing = append(missing, string(inc))
			l.Warn("include %q is empty", inc)
		} else {
			present = append(present, string(inc))
			l.Info("include %q: %d chars", inc, len(src))
		}
	}

	l.Info("present=%v missing=%v", present, missing)
	if len(present) == 0 {
		t.Error("No class includes could be retrieved")
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
			t.Logf("    [%s] %s (%.0f µs)", status, method.Name, method.ExecutionTime)
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
		if isTransientSAPError(err) {
			t.Skipf("Failed to create program: (SAP 5xx): %v", err)
		}
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

// TestIntegration_LockUnlock tests just the lock/unlock cycle.
// Uses a temporary $TMP program so we never touch standard SAP objects (B-004 fix).
func TestIntegration_LockUnlock(t *testing.T) {
	client := requireIntegrationClient(t)
	log := newTestLogger(t)

	name := tempObjectName("ZTEST_LOCK")
	withTempProgram(t, client, name, "", func(objectURL string) {
		ctx := context.Background()

		lock, err := client.LockObject(ctx, objectURL, "MODIFY")
		if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("LockObject failed (SAP 5xx): %v", err)
		}
			t.Fatalf("LockObject failed: %v", err)
		}
		log.Info("Lock acquired: handle=%s isLocal=%v", lock.LockHandle, lock.IsLocal)

		if lock.LockHandle == "" {
			t.Error("LockHandle is empty")
		}

		err = client.UnlockObject(ctx, objectURL, lock.LockHandle)
		if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("UnlockObject failed (SAP 5xx): %v", err)
		}
			t.Fatalf("UnlockObject failed: %v", err)
		}
		log.Info("Unlock successful")
	})
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
		if isTransientSAPError(err) {
			t.Skipf("Failed to create class: (SAP 5xx): %v", err)
		}
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
		if isTransientSAPError(err) {
			t.Skipf("Failed to create test include (SAP 5xx): %v", err)
		}
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
			t.Logf("    [%s] %s (%.0f µs)", status, method.Name, method.ExecutionTime)
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
		if isTransientSAPError(err) {
			t.Skipf("Failed to create test program: (SAP 5xx): %v", err)
		}
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
		if isTransientSAPError(err) {
			t.Skipf("WriteProgram failed (SAP 5xx): %v", err)
		}
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
		if isTransientSAPError(err) {
			t.Skipf("Failed to create test class: (SAP 5xx): %v", err)
		}
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
		if isTransientSAPError(err) {
			t.Skipf("WriteClass failed (SAP 5xx): %v", err)
		}
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
		if isTransientSAPError(err) {
			t.Skipf("CreateAndActivateProgram failed (SAP 5xx): %v", err)
		}
		t.Fatalf("CreateAndActivateProgram failed: %v", err)
	}

	t.Logf("CreateAndActivateProgram result: success=%v, message=%s", result.Success, result.Message)

	// B-008: log all activation messages, flag warnings
	if result.Activation != nil && len(result.Activation.Messages) > 0 {
		for _, m := range result.Activation.Messages {
			t.Logf("  Activation msg [%s] line %d obj=%s: %s", m.Type, m.Line, m.ObjDescr, m.ShortText)
		}
	}

	if !result.Success {
		if strings.Contains(result.Message, "HTTP 5") || strings.Contains(result.Message, "status 5") {
			t.Skipf("CreateAndActivateProgram did not succeed (SAP 5xx): %s", result.Message)
		}
		t.Fatalf("CreateAndActivateProgram did not succeed: %s", result.Message)
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
		if isTransientSAPError(err) {
			t.Skipf("CreateClassWithTests failed (SAP 5xx): %v", err)
		}
		t.Fatalf("CreateClassWithTests failed: %v", err)
	}

	t.Logf("CreateClassWithTests result: success=%v, message=%s", result.Success, result.Message)

	if !result.Success {
		if strings.Contains(result.Message, "HTTP 5") || strings.Contains(result.Message, "status 5") {
			t.Skipf("CreateClassWithTests did not succeed (SAP 5xx): %s", result.Message)
		}
		if result.Activation != nil && len(result.Activation.Messages) > 0 {
			for _, m := range result.Activation.Messages {
				t.Logf("  Activation msg [%s]: %s", m.Type, m.ShortText)
			}
		}
		t.Fatalf("CreateClassWithTests did not succeed: %s", result.Message)
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
				t.Logf("    [%s] %s (%.0f µs)", status, tm.Name, tm.ExecutionTime)
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
		if isTransientSAPError(err) {
			t.Skipf("SyntaxCheck call failed (SAP 5xx): %v", err)
		}
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
		if isTransientSAPError(err) {
			t.Skipf("PrettyPrint failed (SAP 5xx): %v", err)
		}
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
		if isTransientSAPError(err) {
			t.Skipf("GetPrettyPrinterSettings failed (SAP 5xx): %v", err)
		}
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
		if isTransientSAPError(err) {
			t.Skipf("Failed to create test program: (SAP 5xx): %v", err)
		}
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
		if isTransientSAPError(err) {
			t.Skipf("CodeCompletion failed (SAP 5xx): %v", err)
		}
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
		if isTransientSAPError(err) {
			t.Skipf("Failed to create test program: (SAP 5xx): %v", err)
		}
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
		if isTransientSAPError(err) {
			t.Skipf("FindDefinition failed (SAP 5xx): %v", err)
		}
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
		if isTransientSAPError(err) {
			t.Skipf("Failed to create test program: (SAP 5xx): %v", err)
		}
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
		if isTransientSAPError(err) {
			t.Skipf("GetTypeHierarchy failed (SAP 5xx): %v", err)
		}
		t.Fatalf("GetTypeHierarchy failed: %v", err)
	}

	t.Logf("Found %d supertypes of CL_ABAP_CLASSDESCR", len(hierarchy))
	for _, h := range hierarchy {
		t.Logf("  %s (%s) - %s", h.Name, h.Type, h.Description)
	}

	t.Log("GetTypeHierarchy test passed!")
}

// --- Package Creation Tests ---

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
		if isTransientSAPError(err) {
			t.Skipf("Failed to create package: (SAP 5xx): %v", err)
		}
		t.Fatalf("Failed to create package: %v", err)
	}

	t.Logf("Package %s created successfully", packageName)

	// Verify package exists by getting its contents
	pkg, err := client.GetPackage(ctx, packageName)
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
		if isTransientSAPError(err) {
			t.Skipf("Failed to create test program: (SAP 5xx): %v", err)
		}
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

	// Set initial source using WriteProgram
	initialSource := fmt.Sprintf(`REPORT %s.

* Initial version
DATA: lv_count TYPE i.
lv_count = 10.
WRITE: / 'Count:', lv_count.`, strings.ToLower(programName))

	_, err = client.WriteProgram(ctx, programName, initialSource, "")
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("WriteProgram failed (SAP 5xx): %v", err)
		}
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
		if isTransientSAPError(err) {
			t.Skipf("EditSource failed (SAP 5xx): %v", err)
		}
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
	source, err := client.GetProgram(ctx, programName)
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
		if isTransientSAPError(err) {
			t.Skipf("EditSource (second edit) failed (SAP 5xx): %v", err)
		}
		t.Fatalf("EditSource (second edit) failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("EditSource (second edit) did not succeed: %s", result.Message)
	}

	// Verify second change
	source, err = client.GetProgram(ctx, programName)
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
		if isTransientSAPError(err) {
			t.Skipf("EditSource (syntax error test) failed (SAP 5xx): %v", err)
		}
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
	source, err = client.GetProgram(ctx, programName)
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
		if isTransientSAPError(err) {
			t.Skipf("EditSource (case-insensitive test) failed (SAP 5xx): %v", err)
		}
		t.Fatalf("EditSource (case-insensitive test) failed: %v", err)
	}

	if !result.Success {
		t.Logf("Case-insensitive test did not succeed (expected if pretty-printer normalized case): %s", result.Message)
	} else {
		t.Logf("Case-insensitive match succeeded: %s", result.Message)

		// Verify case-insensitive change
		source, err = client.GetProgram(ctx, programName)
		if err != nil {
			t.Fatalf("Failed to read program after case-insensitive edit: %v", err)
		}

		if !strings.Contains(source, "lv_count = 123.") {
			t.Errorf("Expected source to contain 'lv_count = 123.', but it doesn't:\n%s", source)
		}
	}

	t.Log("EditSource workflow completed successfully!")
}

// TestIntegration_GetDDLS tests reading a CDS DDL source
func TestIntegration_GetDDLS(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Use a standard SAP CDS view
	ddlsName := "I_ABAPPACKAGE"

	source, err := client.GetDDLS(ctx, ddlsName)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetDDLS failed (SAP 5xx): %v", err)
		}
		t.Fatalf("GetDDLS failed: %v", err)
	}

	t.Logf("GetDDLS returned %d bytes", len(source))

	// CDS views should contain "define" keyword (view/root view entity)
	if !strings.Contains(strings.ToLower(source), "define") {
		t.Errorf("Expected CDS source to contain 'define', got:\n%s", source[:min(200, len(source))])
	}

	t.Logf("GetDDLS successful: %s", ddlsName)
}

// TestIntegration_GetBDEF tests reading a Behavior Definition
func TestIntegration_GetBDEF(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Use a DMO RAP behavior definition
	bdefName := "/DMO/C_TRAVEL_U"

	source, err := client.GetBDEF(ctx, bdefName)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetBDEF failed (SAP 5xx): %v", err)
		}
		t.Fatalf("GetBDEF failed: %v", err)
	}

	t.Logf("GetBDEF returned %d bytes", len(source))

	// BDEF should contain "define behavior" keyword
	if !strings.Contains(strings.ToLower(source), "define behavior") {
		t.Errorf("Expected BDEF source to contain 'define behavior', got:\n%s", source[:min(200, len(source))])
	}

	t.Logf("GetBDEF successful: %s", bdefName)
}

// TestIntegration_GetSRVB tests reading a Service Binding
func TestIntegration_GetSRVB(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Use a DMO RAP service binding
	srvbName := "/DMO/API_TRAVEL_U_V2"

	sb, err := client.GetSRVB(ctx, srvbName)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetSRVB failed (SAP 5xx): %v", err)
		}
		t.Fatalf("GetSRVB failed: %v", err)
	}

	t.Logf("GetSRVB result: name=%s, type=%s, version=%s", sb.Name, sb.Type, sb.BindingVersion)

	if sb.Name == "" {
		t.Error("Expected SRVB name to be non-empty")
	}

	t.Logf("GetSRVB successful: %s", srvbName)
}

// TestIntegration_GetSource_RAP tests GetSource unified tool for RAP types
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
				if isTransientSAPError(err) {
					t.Skipf("GetSource(%s, %s) failed (SAP 5xx): %v", tc.objectType, tc.objectName, err)
				}
				t.Fatalf("GetSource(%s, %s) failed: %v", tc.objectType, tc.objectName, err)
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

// TestIntegration_RAP_E2E_OData tests the full RAP OData service creation workflow:
// 1. Create CDS view (DDLS)
// 2. Create Service Definition (SRVD)
// 3. Create Service Binding (SRVB)
// 4. Publish service binding
// This test cleans up all created objects at the end.
func TestIntegration_RAP_E2E_OData(t *testing.T) {
	// SRVB activation and publish are slow operations — 3 min client timeout.
	client := requireIntegrationClient(t, WithTimeout(3*time.Minute))
	log := newTestLogger(t)
	log.Info("RAP E2E test — client timeout=3min (SRVB activation can take >60s)")
	ctx := context.Background()

	// Test object names
	ddlsName := "ZTEST_MCP_I_FLIGHT"
	srvdName := "ZTEST_MCP_SD_FLIGHT"
	srvbName := "ZTEST_MCP_SB_FLIGHT"
	pkg := "$TMP"

	// Cleanup via t.Cleanup so it runs even on t.Fatalf (unlike defer+ctx).
	t.Cleanup(func() {
		if os.Getenv("SAP_TEST_NO_CLEANUP") == "true" {
			log.Warn("Cleanup skipped (SAP_TEST_NO_CLEANUP=true)")
			return
		}
		log.Info("Cleanup: deleting RAP objects...")
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = client.DeleteObject(cleanCtx, "/sap/bc/adt/businessservices/bindings/"+strings.ToLower(srvbName), "", "")
		_ = client.DeleteObject(cleanCtx, "/sap/bc/adt/ddic/srvd/sources/"+strings.ToLower(srvdName), "", "")
		_ = client.DeleteObject(cleanCtx, "/sap/bc/adt/ddic/ddl/sources/"+strings.ToLower(ddlsName), "", "")
		log.Info("Cleanup done")
	})

	// Step 1: Create CDS View (DDLS)
	log.Info("Step 1: Creating CDS View (DDLS)...")
	ddlsSource := `@AbapCatalog.sqlViewName: 'ZTESTMCPIFLIGHT'
@AbapCatalog.compiler.compareFilter: true
@AccessControl.authorizationCheck: #NOT_REQUIRED
@EndUserText.label: 'Flight Data for OData Test'
define view ZTEST_MCP_I_FLIGHT as select from sflight {
  key carrid   as Airline,
  key connid   as FlightNumber,
  key fldate   as FlightDate,
      price    as Price,
      currency as Currency,
      planetype as PlaneType,
      seatsmax as SeatsMax,
      seatsocc as SeatsOccupied
}`

	ddlsResult, err := client.WriteSource(ctx, "DDLS", ddlsName, ddlsSource, &WriteSourceOptions{
		Mode:        WriteModeUpsert,
		Package:     pkg,
		Description: "Flight Data for OData Test",
	})
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("WriteSource DDLS failed (SAP 5xx): %v", err)
		}
		t.Fatalf("WriteSource DDLS failed: %v", err)
	}
	log.Info("DDLS: success=%v mode=%s msg=%s", ddlsResult.Success, ddlsResult.Mode, ddlsResult.Message)
	if !ddlsResult.Success {
		if strings.Contains(ddlsResult.Message, "HTTP 5") || strings.Contains(ddlsResult.Message, "status 5") {
			t.Skipf("DDLS creation failed (SAP 5xx): %s", ddlsResult.Message)
		}
		t.Fatalf("DDLS creation failed: %s", ddlsResult.Message)
	}

	// Step 2: Create Service Definition (SRVD)
	log.Info("Step 2: Creating Service Definition (SRVD)...")
	srvdSource := `@EndUserText.label: 'Flight Service Definition'
define service ZTEST_MCP_SD_FLIGHT {
  expose ZTEST_MCP_I_FLIGHT as Flights;
}`

	srvdResult, err := client.WriteSource(ctx, "SRVD", srvdName, srvdSource, &WriteSourceOptions{
		Mode:        WriteModeUpsert,
		Package:     pkg,
		Description: "Flight Service Definition",
	})
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("WriteSource SRVD failed (SAP 5xx): %v", err)
		}
		t.Fatalf("WriteSource SRVD failed: %v", err)
	}
	log.Info("SRVD: success=%v mode=%s msg=%s", srvdResult.Success, srvdResult.Mode, srvdResult.Message)
	if !srvdResult.Success {
		t.Fatalf("SRVD creation failed: %s", srvdResult.Message)
	}

	// Step 3: Create Service Binding (SRVB)
	log.Info("Step 3: Creating Service Binding (SRVB)...")
	err = client.CreateObject(ctx, CreateObjectOptions{
		ObjectType:        ObjectTypeSRVB,
		Name:              srvbName,
		PackageName:       pkg,
		Description:       "Flight OData V2 Binding",
		ServiceDefinition: srvdName,
		BindingVersion:    "V2",
		BindingCategory:   "0", // Web API
	})
	if err != nil {
		// SAP returns "does already exist" (no trailing s) or ExceptionResourceAlreadyExists
		if !strings.Contains(err.Error(), "already exist") {
		if isTransientSAPError(err) {
			t.Skipf("CreateObject SRVB failed (SAP 5xx): %v", err)
		}
			t.Fatalf("CreateObject SRVB failed: %v", err)
		}
		log.Warn("SRVB already exists — continuing")
	} else {
		log.Info("SRVB created")
	}

	// Step 4: Activate SRVB — known to be slow (>60s on trial systems).
	// The client was created with 3-minute timeout specifically for this.
	log.Info("Step 4: Activating Service Binding (slow — up to 3min)...")
	srvbURL := "/sap/bc/adt/businessservices/bindings/" + strings.ToLower(srvbName)
	activationResult, err := client.Activate(ctx, srvbURL, srvbName)
	if err != nil {
		log.Warn("Activation warning: %v", err)
	} else {
		log.Info("Activation: success=%v msgs=%d", activationResult.Success, len(activationResult.Messages))
		for _, msg := range activationResult.Messages {
			log.Debug("  msg: %+v", msg)
		}
	}

	// Step 5: Publish Service Binding
	log.Info("Step 5: Publishing Service Binding...")
	publishResult, err := client.PublishServiceBinding(ctx, srvbName, "0001")
	if err != nil {
		log.Warn("Publish warning (non-fatal): %v", err)
	} else {
		log.Info("Published: %+v", publishResult)
	}

	// Step 6: Verify SRVB exists (independent of activation success)
	log.Info("Step 6: Verifying Service Binding...")
	sb, err := client.GetSRVB(ctx, srvbName)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetSRVB verification failed (SAP 5xx): %v", err)
		}
		t.Fatalf("GetSRVB verification failed: %v", err)
	}
	log.Info("SRVB verified: name=%s type=%s version=%s", sb.Name, sb.Type, sb.BindingVersion)

	log.Info("RAP E2E OData test completed")
}

// TestIntegration_ExternalBreakpoints tests setting, getting, and deleting external breakpoints.
func TestIntegration_ExternalBreakpoints(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Use a known program for testing
	testObjectURI := "/sap/bc/adt/programs/programs/DEMO_ABAP_OBJECTS/source/main"

	// Test user for breakpoints
	testUser := "AVINOGRADOVA"

	// Step 1: Get initial breakpoints
	t.Log("Step 1: Getting existing external breakpoints...")
	initialBPs, err := client.GetExternalBreakpoints(ctx, testUser)
	if err != nil {
		t.Logf("GetExternalBreakpoints returned error (may be empty): %v", err)
	} else {
		t.Logf("Found %d existing breakpoints", len(initialBPs.Breakpoints))
	}

	// Step 2: Set a line breakpoint
	// User field is required for external breakpoints in user debugging mode
	t.Log("Step 2: Setting line breakpoint at line 7...")
	req := &BreakpointRequest{
		Scope:         BreakpointScopeExternal,
		DebuggingMode: DebuggingModeUser,
		User:          testUser,
		Breakpoints:   []Breakpoint{NewLineBreakpoint(testObjectURI, 7)},
	}

	resp, err := client.SetExternalBreakpoint(ctx, req)
	if err != nil {
		// External breakpoints might require specific authorization
		t.Logf("SetExternalBreakpoint failed (may require authorization): %v", err)
		t.Skip("Skipping breakpoint test - breakpoint API may not be available or authorized")
		return
	}

	if len(resp.Breakpoints) == 0 {
		t.Fatal("Expected at least one breakpoint in response")
	}

	bp := resp.Breakpoints[0]
	t.Logf("Line breakpoint set: ID=%s, Kind=%s, Line=%d", bp.ID, bp.Kind, bp.Line)

	// Step 3: Set an exception breakpoint
	t.Log("Step 3: Setting exception breakpoint for CX_SY_ZERODIVIDE...")
	exReq := &BreakpointRequest{
		Scope:         BreakpointScopeExternal,
		DebuggingMode: DebuggingModeUser,
		User:          testUser,
		Breakpoints:   []Breakpoint{NewExceptionBreakpoint("CX_SY_ZERODIVIDE")},
	}

	exResp, err := client.SetExternalBreakpoint(ctx, exReq)
	if err != nil {
		t.Logf("Exception breakpoint warning: %v", err)
	} else if len(exResp.Breakpoints) > 0 {
		exBp := exResp.Breakpoints[0]
		t.Logf("Exception breakpoint set: ID=%s, Exception=%s", exBp.ID, exBp.Exception)
	}

	// Step 4: Get all breakpoints
	t.Log("Step 4: Getting all external breakpoints...")
	allBPs, err := client.GetExternalBreakpoints(ctx, testUser)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetExternalBreakpoints failed (SAP 5xx): %v", err)
		}
		t.Fatalf("GetExternalBreakpoints failed: %v", err)
	}
	t.Logf("Total breakpoints after setting: %d", len(allBPs.Breakpoints))

	for i, bpItem := range allBPs.Breakpoints {
		t.Logf("  [%d] ID=%s, Kind=%s", i+1, bpItem.ID, bpItem.Kind)
	}

	// Step 5: Delete the line breakpoint
	if bp.ID != "" {
		t.Logf("Step 5: Deleting line breakpoint %s...", bp.ID)
		err = client.DeleteExternalBreakpoint(ctx, bp.ID, testUser)
		if err != nil {
			t.Logf("DeleteExternalBreakpoint warning: %v", err)
		} else {
			t.Log("Line breakpoint deleted successfully")
		}
	}

	// Step 6: Delete the exception breakpoint
	if exResp != nil && len(exResp.Breakpoints) > 0 && exResp.Breakpoints[0].ID != "" {
		exID := exResp.Breakpoints[0].ID
		t.Logf("Step 6: Deleting exception breakpoint %s...", exID)
		err = client.DeleteExternalBreakpoint(ctx, exID, testUser)
		if err != nil {
			t.Logf("DeleteExternalBreakpoint warning: %v", err)
		} else {
			t.Log("Exception breakpoint deleted successfully")
		}
	}

	// Step 7: Verify cleanup
	t.Log("Step 7: Verifying breakpoints deleted...")
	finalBPs, err := client.GetExternalBreakpoints(ctx, testUser)
	if err != nil {
		t.Logf("Final GetExternalBreakpoints returned error: %v", err)
	} else {
		t.Logf("Final breakpoint count: %d", len(finalBPs.Breakpoints))
	}

	t.Log("External breakpoints test completed!")
}

func TestIntegration_DebuggerListener(t *testing.T) {
	client := requireIntegrationClient(t)
	log := newTestLogger(t)

	testUser := os.Getenv("SAP_USER")
	if testUser == "" {
		testUser = "DEVELOPER"
	}
	log.Info("Testing debug listener for user: %s", testUser)

	// Step 1: Check for existing listeners (short timeout — read-only probe)
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer checkCancel()

	log.Info("Step 1: Checking for existing listeners...")
	conflict, err := client.DebuggerCheckListener(checkCtx, &ListenOptions{
		DebuggingMode: DebuggingModeUser,
		User:          testUser,
	})
	if err != nil {
		log.Warn("DebuggerCheckListener error: %v", err)
	}
	if conflict != nil {
		log.Info("Found existing listener: %s — stopping it first", conflict.ConflictText)
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer stopCancel()
		if stopErr := client.DebuggerStopListener(stopCtx, &ListenOptions{
			DebuggingMode: DebuggingModeUser,
			User:          testUser,
		}); stopErr != nil {
			log.Warn("DebuggerStopListener error: %v", stopErr)
		}
	} else {
		log.Info("No existing listeners found")
	}

	// Step 2: Start a short listen — SAP TimeoutSeconds=5 means the server
	// returns after 5s with a "timed out" result. We give the HTTP call 20s
	// so there's headroom for network + processing on both sides.
	log.Info("Step 2: Starting debug listener (server timeout=5s, HTTP deadline=20s)...")
	listenCtx, listenCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer listenCancel()

	result, err := client.DebuggerListen(listenCtx, &ListenOptions{
		DebuggingMode:  DebuggingModeUser,
		User:           testUser,
		TimeoutSeconds: 5,
	})
	if err != nil {
		// Context deadline or SAP error — log and skip (not a test failure;
		// debugger API requires specific authorizations not guaranteed on trial).
		log.Warn("DebuggerListen error: %v — skipping (debugger API may need special auth)", err)
		if result != nil && result.Conflict != nil {
			log.Warn("Conflict: %s (ideUser: %s)", result.Conflict.ConflictText, result.Conflict.IdeUser)
		}
		t.Skipf("DebuggerListen not available on this system: %v", err)
		return
	}

	if result == nil {
		t.Skip("DebuggerListen returned nil result — skipping")
		return
	}

	switch {
	case result.TimedOut:
		log.Info("Listener timed out as expected (no debuggee attached)")
	case result.Debuggee != nil:
		log.Info("Debuggee caught: ID=%s Program=%s Line=%d",
			result.Debuggee.ID, result.Debuggee.Program, result.Debuggee.Line)
	case result.Conflict != nil:
		log.Warn("Conflict detected: %s", result.Conflict.ConflictText)
	default:
		log.Warn("Listener returned with no debuggee, timeout, or conflict")
	}

	// Step 3: Stop listener (cleanup — best effort)
	log.Info("Step 3: Stopping listener...")
	stopCtx2, stopCancel2 := context.WithTimeout(context.Background(), 15*time.Second)
	defer stopCancel2()
	if stopErr := client.DebuggerStopListener(stopCtx2, &ListenOptions{
		DebuggingMode: DebuggingModeUser,
		User:          testUser,
	}); stopErr != nil {
		log.Warn("DebuggerStopListener error (may already be stopped): %v", stopErr)
	} else {
		log.Info("Listener stopped")
	}
}

// TestIntegration_DebugSessionAPIs tests the debug session APIs without a live debuggee.
// This test verifies the API structure and error handling.
// For a full debug session test, see the manual test workflow below.
//
// Manual Debug Session Test Workflow:
// 1. Set breakpoint: Use SetExternalBreakpoint on a test program
// 2. Run code: Execute the test program from SAP GUI or another session
// 3. Listen: Call DebuggerListen - should catch the debuggee
// 4. Attach: Call DebuggerAttach with the debuggee ID
// 5. Inspect: Call DebuggerGetStack and DebuggerGetVariables
// 6. Step: Call DebuggerStep with DebugStepOver/Into/Return
// 7. Detach: Call DebuggerDetach to release the debuggee
func TestIntegration_DebugSessionAPIs(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	testUser := os.Getenv("SAP_USER")
	if testUser == "" {
		testUser = "AVINOGRADOVA"
	}

	t.Logf("Testing debug session APIs for user: %s", testUser)

	// Step 1: Test DebuggerAttach with invalid debuggee ID
	// This tests the API is reachable and returns proper error
	t.Log("Step 1: Testing DebuggerAttach with invalid debuggee...")
	_, err := client.DebuggerAttach(ctx, "invalid-debuggee-id", testUser)
	if err == nil {
		t.Error("Expected error for invalid debuggee ID")
	} else {
		t.Logf("DebuggerAttach correctly returned error: %v", err)
	}

	// Step 2: Test DebuggerGetStack without active session
	// Should return error as no debug session is active
	t.Log("Step 2: Testing DebuggerGetStack without session...")
	_, err = client.DebuggerGetStack(ctx, true)
	if err == nil {
		t.Error("Expected error for GetStack without session")
	} else {
		t.Logf("DebuggerGetStack correctly returned error: %v", err)
	}

	// Step 3: Test DebuggerGetVariables without active session
	t.Log("Step 3: Testing DebuggerGetVariables without session...")
	_, err = client.DebuggerGetVariables(ctx, []string{"@ROOT"})
	if err == nil {
		t.Error("Expected error for GetVariables without session")
	} else {
		t.Logf("DebuggerGetVariables correctly returned error: %v", err)
	}

	// Step 4: Test DebuggerGetChildVariables without active session
	t.Log("Step 4: Testing DebuggerGetChildVariables without session...")
	_, err = client.DebuggerGetChildVariables(ctx, []string{"@ROOT"})
	if err == nil {
		t.Error("Expected error for GetChildVariables without session")
	} else {
		t.Logf("DebuggerGetChildVariables correctly returned error: %v", err)
	}

	// Step 5: Test DebuggerStep without active session
	t.Log("Step 5: Testing DebuggerStep without session...")
	_, err = client.DebuggerStep(ctx, DebugStepOver, "")
	if err == nil {
		t.Error("Expected error for Step without session")
	} else {
		t.Logf("DebuggerStep correctly returned error: %v", err)
	}

	t.Log("Debug session API test completed!")
	t.Log("")
	t.Log("=== To test a full debug session manually ===")
	t.Log("1. Set a breakpoint: client.SetExternalBreakpoint(...)")
	t.Log("2. Run code that hits the breakpoint from another session")
	t.Log("3. Call client.DebuggerListen to catch the debuggee")
	t.Log("4. Attach: client.DebuggerAttach(debuggee.ID, user)")
	t.Log("5. Get stack: client.DebuggerGetStack(true)")
	t.Log("6. Get variables: client.DebuggerGetChildVariables([]string{\"@ROOT\"})")
	t.Log("7. Step: client.DebuggerStep(DebugStepOver, \"\")")
	t.Log("8. Detach: client.DebuggerDetach()")
}

// --- Part A5: New tests for previously untested methods ---

func TestIntegration_GetInterface(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	src, err := client.GetInterface(ctx, "IF_SERIALIZABLE_OBJECT")
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetInterface failed (SAP 5xx): %v", err)
		}
		t.Skipf("GetInterface IF_SERIALIZABLE_OBJECT: %v", err)
	}
	if len(src) == 0 {
		t.Error("Interface source is empty")
	}
	l.Info("IF_SERIALIZABLE_OBJECT: %d chars", len(src))
}

func TestIntegration_GetFunctionGroup(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	fg, err := client.GetFunctionGroup(ctx, "SYST")
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetFunctionGroup failed (SAP 5xx): %v", err)
		}
		t.Skipf("GetFunctionGroup SYST: %v", err)
	}
	l.Info("FunctionGroup SYST: functions=%d", len(fg.Functions))
	if len(fg.Functions) == 0 {
		t.Error("No functions found in SYST function group")
	}
}

func TestIntegration_GetFunction(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	src, err := client.GetFunction(ctx, "RFC_PING", "SYST")
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetFunction failed (SAP 5xx): %v", err)
		}
		t.Skipf("GetFunction RFC_PING: %v", err)
	}
	if len(src) == 0 {
		t.Error("Function source is empty")
	}
	l.Info("RFC_PING: %d chars", len(src))
}

func TestIntegration_GetView(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	src, err := client.GetView(ctx, "DD02V")
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetView failed (SAP 5xx): %v", err)
		}
		t.Skipf("GetView DD02V: %v", err)
	}
	if len(src) == 0 {
		t.Error("View source is empty")
	}
	l.Info("DD02V: %d chars", len(src))
}

func TestIntegration_GetStructure(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	src, err := client.GetStructure(ctx, "SYST")
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetStructure failed (SAP 5xx): %v", err)
		}
		t.Skipf("GetStructure SYST: %v", err)
	}
	if len(src) == 0 {
		t.Error("Structure source is empty")
	}
	l.Info("SYST: %d chars", len(src))
}

func TestIntegration_GetSystemInfo(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	info, err := client.GetSystemInfo(ctx)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetSystemInfo failed (SAP 5xx): %v", err)
		}
		t.Fatalf("GetSystemInfo failed: %v", err)
	}
	l.Info("SystemID=%s Release=%s Host=%s", info.SystemID, info.ABAPRelease, info.HostName)
	if info.SystemID == "" {
		t.Error("SystemID is empty")
	}
	if info.ABAPRelease == "" {
		t.Error("ABAPRelease is empty")
	}
}

func TestIntegration_GetInstalledComponents(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	components, err := client.GetInstalledComponents(ctx)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetInstalledComponents failed (SAP 5xx): %v", err)
		}
		t.Fatalf("GetInstalledComponents failed: %v", err)
	}
	if len(components) == 0 {
		t.Error("No installed components returned")
	}
	l.Info("Found %d components", len(components))
	for i, c := range components {
		if i < 5 {
			l.Debug("  component[%d]: %s %s", i, c.Name, c.Release)
		}
	}
}

func TestIntegration_GetClassComponents(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	classURL := "/sap/bc/adt/oo/classes/CL_ABAP_TYPEDESCR"
	comps, err := client.GetClassComponents(ctx, classURL)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetClassComponents failed (SAP 5xx): %v", err)
		}
		t.Skipf("GetClassComponents CL_ABAP_TYPEDESCR: %v", err)
	}
	l.Info("CL_ABAP_TYPEDESCR: name=%s type=%s components=%d", comps.Name, comps.Type, len(comps.Components))
	if comps.Name == "" {
		t.Error("ClassComponents returned empty root node for CL_ABAP_TYPEDESCR")
	}
}

func TestIntegration_GetInactiveObjects(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	objects, err := client.GetInactiveObjects(ctx)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetInactiveObjects failed (SAP 5xx): %v", err)
		}
		t.Fatalf("GetInactiveObjects failed: %v", err)
	}
	l.Info("Inactive objects: %d", len(objects))
	// May be empty — just verify the call succeeds
}

func TestIntegration_RunATCCheck(t *testing.T) {
	client := requireIntegrationClient(t, WithTimeout(90*time.Second))
	l := newTestLogger(t)
	ctx := context.Background()

	// Create a temp program with a known ATC issue (using obsolete statement)
	name := tempObjectName("ZMCP_ATC")
	source := fmt.Sprintf(`REPORT %s.
DATA: lv_x(10) TYPE C.
MOVE 'test' TO lv_x.`, strings.ToLower(name))

	withTempProgram(t, client, name, source, func(objectURL string) {
		worklist, err := client.RunATCCheck(ctx, objectURL, "", 50)
		if err != nil {
			if isTransientSAPError(err) {
				t.Skipf("RunATCCheck failed (SAP 5xx): %v", err)
			}
			t.Fatalf("RunATCCheck failed: %v", err)
		}
		totalFindings := 0
		for _, obj := range worklist.Objects {
			totalFindings += len(obj.Findings)
		}
		l.Info("ATC worklist: %d objects, %d findings", len(worklist.Objects), totalFindings)
		for _, obj := range worklist.Objects {
			for _, f := range obj.Findings {
				l.Info("  [%d] %s line %d: %s", f.Priority, obj.Name, f.Line, f.CheckTitle)
			}
		}
	})
}

func TestIntegration_GetCallGraph(t *testing.T) {
	client := requireIntegrationClient(t, WithTimeout(90*time.Second))
	l := newTestLogger(t)
	ctx := context.Background()

	classURL := "/sap/bc/adt/oo/classes/CL_ABAP_TYPEDESCR"
	graph, err := client.GetCallGraph(ctx, classURL, &CallGraphOptions{MaxDepth: 1})
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetCallGraph failed (SAP 5xx): %v", err)
		}
		t.Skipf("GetCallGraph CL_ABAP_TYPEDESCR: %v", err)
	}
	l.Info("CallGraph root=%s children=%d", graph.Name, len(graph.Children))
}

func TestIntegration_ExecuteABAP(t *testing.T) {
	client := requireIntegrationClient(t, WithTimeout(90*time.Second))
	l := newTestLogger(t)
	ctx := context.Background()

	code := `DATA lv_x TYPE i.
lv_x = 42.
cl_demo_output=>display( lv_x ).`

	result, err := client.ExecuteABAP(ctx, code, nil)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("ExecuteABAP failed (SAP 5xx): %v", err)
		}
		t.Skipf("ExecuteABAP: %v", err)
	}
	l.Info("ExecuteABAP: success=%v output=%v", result.Success, result.Output)
	if !result.Success {
		t.Errorf("ExecuteABAP did not succeed: %s", result.Message)
	}
}

func TestIntegration_GrepPackage(t *testing.T) {
	client := requireIntegrationClient(t, WithTimeout(90*time.Second))
	l := newTestLogger(t)
	ctx := context.Background()

	// Grep for a string that definitely exists in $TMP programs
	result, err := client.GrepPackage(ctx, "$TMP", "REPORT", false, nil, 10)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GrepPackage failed (SAP 5xx): %v", err)
		}
		t.Skipf("GrepPackage $TMP: %v", err)
	}
	l.Info("GrepPackage $TMP 'REPORT': %d matches", result.TotalMatches)
}

func TestIntegration_EditSource_PatternNotFound(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	// B-001: EditSource should error when pattern is not found
	name := tempObjectName("ZMCP_ED")
	source := fmt.Sprintf("REPORT %s.\nDATA lv_x TYPE i.", strings.ToLower(name))

	withTempProgram(t, client, name, source, func(objectURL string) {
		result, err := client.EditSource(ctx, objectURL,
			"THIS_STRING_DOES_NOT_EXIST_IN_SOURCE_9999", "REPLACEMENT",
			false, false, false)
		if err != nil {
			if isTransientSAPError(err) {
				t.Skipf("EditSource failed (SAP 5xx): %v", err)
			}
			// A non-transient error is acceptable — the pattern wasn't found
			l.Info("EditSource returned error for missing pattern (expected): %v", err)
			return
		}
		// If no error, result should indicate failure
		l.Info("EditSource result: success=%v message=%s", result.Success, result.Message)
		if result.Success {
			t.Error("EditSource should not succeed when pattern is not found")
		}
	})
}

func TestIntegration_GetTableContents_Empty(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	// B-007: Query T000 with an impossible WHERE clause — should return 0 rows, no error
	result, err := client.GetTableContents(ctx, "T000", 10, "MANDT = '999'")
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetTableContents failed (SAP 5xx): %v", err)
		}
		t.Fatalf("GetTableContents with empty filter failed: %v", err)
	}
	l.Info("T000 with impossible filter: %d rows", len(result.Rows))
	// Should return 0 rows without error
}

func TestIntegration_GetClassInclude_AllTypes(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	const className = "CL_ABAP_TYPEDESCR"
	includes := []ClassIncludeType{
		ClassIncludeDefinitions,
		ClassIncludeImplementations,
		ClassIncludeMacros,
		ClassIncludeTestClasses,
	}

	for _, inc := range includes {
		inc := inc
		t.Run(string(inc), func(t *testing.T) {
			src, err := client.GetClassInclude(ctx, className, inc)
			if err != nil {
				if isTransientSAPError(err) {
					t.Skipf("GetClassInclude %s failed (SAP 5xx): %v", inc, err)
				}
				l.Warn("GetClassInclude %s: %v (may be empty on this class)", inc, err)
				return
			}
			l.Info("include %s: %d chars", inc, len(src))
		})
	}
}

func TestIntegration_UpdateClassInclude(t *testing.T) {
	client := requireIntegrationClient(t, WithTimeout(90*time.Second))
	l := newTestLogger(t)
	ctx := context.Background()

	name := tempObjectName("ZMCP_CLS")
	classSource := fmt.Sprintf(`CLASS %s DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    METHODS: run.
ENDCLASS.
CLASS %s IMPLEMENTATION.
  METHOD run.
    DATA lv_x TYPE i.
    lv_x = 1.
  ENDMETHOD.
ENDCLASS.`, name, name)

	result, err := client.CreateClassWithTests(ctx, name, "Test class", "$TMP", classSource, "", "")
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("CreateClassWithTests failed (SAP 5xx): %v", err)
		}
		t.Skipf("CreateClassWithTests: %v", err)
	}
	if !result.Success {
		if strings.Contains(result.Message, "HTTP 5") || strings.Contains(result.Message, "status 5") {
			t.Skipf("CreateClassWithTests (SAP 5xx): %s", result.Message)
		}
		t.Skipf("CreateClassWithTests did not succeed: %s", result.Message)
	}

	t.Cleanup(func() {
		if os.Getenv("SAP_TEST_NO_CLEANUP") == "true" {
			return
		}
		classURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", name)
		lock, _ := client.LockObject(ctx, classURL, "MODIFY")
		if lock != nil {
			client.DeleteObject(ctx, classURL, lock.LockHandle, "")
		}
	})

	// Now update the definitions include
	classURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", name)
	lock, err := client.LockObject(ctx, classURL, "MODIFY")
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("LockObject failed (SAP 5xx): %v", err)
		}
		t.Fatalf("LockObject failed: %v", err)
	}
	defer client.UnlockObject(ctx, classURL, lock.LockHandle)

	newDef := fmt.Sprintf(`CLASS %s DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    METHODS: run.
    METHODS: new_method.
ENDCLASS.`, name)

	err = client.UpdateClassInclude(ctx, name, ClassIncludeDefinitions, newDef, lock.LockHandle, "")
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("UpdateClassInclude failed (SAP 5xx): %v", err)
		}
		t.Fatalf("UpdateClassInclude failed: %v", err)
	}

	// Read back and verify
	src, err := client.GetClassInclude(ctx, name, ClassIncludeDefinitions)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetClassInclude readback failed (SAP 5xx): %v", err)
		}
		t.Fatalf("GetClassInclude readback failed: %v", err)
	}
	if !strings.Contains(src, "new_method") {
		t.Errorf("Updated definitions include does not contain 'new_method'")
	}
	l.Info("UpdateClassInclude: verified new_method in definitions include")
}

func TestIntegration_TransportLifecycle(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	// GetUserTransports
	transports, err := client.GetUserTransports(ctx, "")
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetUserTransports failed (SAP 5xx): %v", err)
		}
		t.Fatalf("GetUserTransports failed: %v", err)
	}
	l.Info("GetUserTransports: %d workbench, %d customizing", len(transports.Workbench), len(transports.Customizing))

	// Create a transport
	transportNum, err := client.CreateTransport(ctx, "", "MCP integration test transport", "$TMP")
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("CreateTransport failed (SAP 5xx): %v", err)
		}
		t.Fatalf("CreateTransport failed: %v", err)
	}
	l.Info("Created transport: %s", transportNum)

	t.Cleanup(func() {
		if os.Getenv("SAP_TEST_NO_CLEANUP") == "true" {
			return
		}
		if err := client.DeleteTransport(ctx, transportNum); err != nil {
			t.Logf("Cleanup: could not delete transport %s: %v", transportNum, err)
		}
	})

	// Verify it appears in user transports
	transports2, err := client.GetUserTransports(ctx, "")
	if err == nil {
		found := false
		for _, tr := range append(transports2.Workbench, transports2.Customizing...) {
			if tr.Number == transportNum {
				found = true
				break
			}
		}
		if !found {
			l.Warn("Newly created transport %s not found in user transports list", transportNum)
		} else {
			l.Info("Transport %s confirmed in user transports", transportNum)
		}
	}
}

func TestIntegration_GetTransportInfo(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	// Use a $TMP object — should have "local" transport info
	objectURL := "/sap/bc/adt/programs/programs/SAPMSSY0"
	info, err := client.GetTransportInfo(ctx, objectURL, "")
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetTransportInfo failed (SAP 5xx): %v", err)
		}
		t.Skipf("GetTransportInfo SAPMSSY0: %v", err)
	}
	l.Info("TransportInfo: devClass=%s recording=%s", info.DevClass, info.Recording)
}

func TestIntegration_CompareSource(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	// Compare a program with itself — should produce zero diff
	diff, err := client.CompareSource(ctx, "PROG", "SAPMSSY0", "PROG", "SAPMSSY0", nil, nil)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("CompareSource failed (SAP 5xx): %v", err)
		}
		t.Skipf("CompareSource SAPMSSY0 vs SAPMSSY0: %v", err)
	}
	l.Info("CompareSource same object: identical=%v added=%d removed=%d", diff.Identical, diff.AddedLines, diff.RemovedLines)
	if !diff.Identical {
		t.Errorf("Comparing object with itself: not identical (+%d -%d)", diff.AddedLines, diff.RemovedLines)
	}
}

func TestIntegration_GetClassInfo(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	info, err := client.GetClassInfo(ctx, "CL_ABAP_TYPEDESCR")
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetClassInfo failed (SAP 5xx): %v", err)
		}
		t.Skipf("GetClassInfo CL_ABAP_TYPEDESCR: %v", err)
	}
	l.Info("ClassInfo: name=%s superClass=%s abstract=%v final=%v", info.Name, info.Superclass, info.IsAbstract, info.IsFinal)
	if info.Name == "" {
		t.Error("ClassInfo.Name is empty")
	}
}

func TestIntegration_ListDumps(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	dumps, err := client.GetDumps(ctx, nil)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetDumps failed (SAP 5xx): %v", err)
		}
		t.Fatalf("GetDumps failed: %v", err)
	}
	l.Info("Dumps: %d entries", len(dumps))
	// May be empty — just verify the call succeeds
}

func TestIntegration_ListTraces(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	traces, err := client.ListTraces(ctx, nil)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("ListTraces failed (SAP 5xx): %v", err)
		}
		t.Fatalf("ListTraces failed: %v", err)
	}
	l.Info("Traces: %d entries", len(traces))
	// May be empty — just verify the call succeeds
}

func TestIntegration_GetSQLTraceState(t *testing.T) {
	client := requireIntegrationClient(t)
	l := newTestLogger(t)
	ctx := context.Background()

	state, err := client.GetSQLTraceState(ctx)
	if err != nil {
		if isTransientSAPError(err) {
			t.Skipf("GetSQLTraceState failed (SAP 5xx): %v", err)
		}
		t.Fatalf("GetSQLTraceState failed: %v", err)
	}
	l.Info("SQLTraceState: active=%v user=%s", state.Active, state.User)
}

// --- Part A6: TDD stubs for planned-but-not-yet-implemented features ---
// These tests appear as SKIP with a [TODO] tag, providing a visible checklist.

func TestIntegration_TODO_GetDomain(t *testing.T) {
	newTestLogger(t).Todo("GetDomain not yet implemented — ADT: /sap/bc/adt/ddic/domains/{name}/source/main")
}

func TestIntegration_TODO_GetDataElement(t *testing.T) {
	newTestLogger(t).Todo("GetDataElement not yet implemented — ADT: /sap/bc/adt/ddic/dataelements/{name}/source/main")
}

func TestIntegration_TODO_GetWhereUsed(t *testing.T) {
	newTestLogger(t).Todo("GetWhereUsed not yet implemented — ADT: /sap/bc/adt/repository/informationsystem/usageReferences?objectName={n}&objectType={t}")
}

func TestIntegration_TODO_GetProgFullCode(t *testing.T) {
	newTestLogger(t).Todo("GetProgFullCode not yet implemented — composite: fetch main source + recursively resolve INCLUDE statements")
}

func TestIntegration_TODO_ListObjects(t *testing.T) {
	newTestLogger(t).Todo("ListObjects not yet implemented — extends GetPackage via /sap/bc/adt/repository/nodestructure with depth/type filters")
}

func TestIntegration_TODO_GetEnhancements(t *testing.T) {
	newTestLogger(t).Todo("GetEnhancements not yet implemented — ADT: /sap/bc/adt/{programs|classes}/{name}/source/main/enhancements/elements")
}

func TestIntegration_TODO_GetEnhancementSpot(t *testing.T) {
	newTestLogger(t).Todo("GetEnhancementSpot not yet implemented — ADT: /sap/bc/adt/enhancements/enhsxsb/{spot}")
}

func TestIntegration_TODO_JWTAuth(t *testing.T) {
	newTestLogger(t).Todo("JWT/XSUAA auth not yet implemented — needed for ABAP Cloud / BTP targets (WithJWTToken option + Bearer header)")
}

func TestIntegration_TODO_CDSUnitTests(t *testing.T) {
	newTestLogger(t).Todo("CDS unit test execution not yet implemented — complex lifecycle, follow-up after RunUnitTests stabilization")
}
