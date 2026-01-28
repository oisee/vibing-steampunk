//go:build integration

// ABOUTME: Integration tests for debugging operations (breakpoints, listener, session).
// ABOUTME: Tests external breakpoints and debug listener functionality.

package adt

import (
	"context"
	"testing"
)

func TestIntegration_ExternalBreakpoints(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Use a known program for testing
	testObjectURI := "/sap/bc/adt/programs/programs/DEMO_ABAP_OBJECTS/source/main"

	// Test user for breakpoints
	testUser := getTestUser(t)

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
	client := getIntegrationClient(t)
	ctx := context.Background()

	testUser := getTestUser(t)
	t.Logf("Testing debug listener for user: %s", testUser)

	// Step 1: Check if there are existing listeners
	t.Log("Step 1: Checking for existing listeners...")
	conflict, err := client.DebuggerCheckListener(ctx, &ListenOptions{
		DebuggingMode: DebuggingModeUser,
		User:          testUser,
	})
	if err != nil {
		t.Logf("DebuggerCheckListener returned error: %v", err)
	}
	if conflict != nil {
		t.Logf("Found existing listener: %s", conflict.ConflictText)
		// Stop the existing listener first
		t.Log("Stopping existing listener...")
		err = client.DebuggerStopListener(ctx, &ListenOptions{
			DebuggingMode: DebuggingModeUser,
			User:          testUser,
		})
		if err != nil {
			t.Logf("Failed to stop existing listener: %v", err)
		}
	} else {
		t.Log("No existing listeners found")
	}

	// Step 2: Start a short listener (5 second timeout)
	t.Log("Step 2: Starting debug listener with 5s timeout...")
	result, err := client.DebuggerListen(ctx, &ListenOptions{
		DebuggingMode:  DebuggingModeUser,
		User:           testUser,
		TimeoutSeconds: 5, // Very short timeout for testing
	})
	if err != nil {
		// The listener might error if there's a conflict or other issue
		t.Logf("DebuggerListen returned error: %v", err)
		if result != nil && result.Conflict != nil {
			t.Logf("Conflict info: %s (ideUser: %s)",
				result.Conflict.ConflictText,
				result.Conflict.IdeUser)
		}
	} else if result != nil {
		if result.TimedOut {
			t.Log("Listener timed out (expected - no debuggee available)")
		} else if result.Debuggee != nil {
			t.Logf("Debuggee caught: ID=%s, Program=%s, Line=%d",
				result.Debuggee.ID, result.Debuggee.Program, result.Debuggee.Line)
		} else if result.Conflict != nil {
			t.Logf("Conflict detected: %s", result.Conflict.ConflictText)
		} else {
			t.Log("Listener returned with no debuggee or timeout")
		}
	}

	// Step 3: Stop listener (cleanup)
	t.Log("Step 3: Stopping listener...")
	err = client.DebuggerStopListener(ctx, &ListenOptions{
		DebuggingMode: DebuggingModeUser,
		User:          testUser,
	})
	if err != nil {
		t.Logf("DebuggerStopListener returned error: %v (might be expected if already stopped)", err)
	} else {
		t.Log("Listener stopped successfully")
	}

	t.Log("Debug listener test completed!")
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

	testUser := getTestUser(t)
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
