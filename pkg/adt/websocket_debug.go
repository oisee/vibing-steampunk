package adt

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// --- Breakpoint Operations ---

// setBreakpointInternal sends a breakpoint request and parses the response.
func (c *DebugWebSocketClient) setBreakpointInternal(ctx context.Context, params map[string]any) (string, error) {
	resp, err := c.sendRequest(ctx, "setBreakpoint", params)
	if err != nil {
		return "", err
	}

	if !resp.Success {
		if resp.Error != nil {
			return "", fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return "", fmt.Errorf("setBreakpoint failed")
	}

	var result struct {
		BreakpointID string `json:"breakpointId"`
		Registered   bool   `json:"registered"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return "", err
	}

	return result.BreakpointID, nil
}

// SetLineBreakpoint sets a breakpoint at a specific line in a program.
// For classes, program should be in class pool format: ZCL_TEST================CP
// The line number is pool-absolute (the line in the consolidated class source).
func (c *DebugWebSocketClient) SetLineBreakpoint(ctx context.Context, program string, line int) (string, error) {
	params := map[string]any{
		"kind":    "line",
		"program": program,
		"line":    line,
	}
	return c.setBreakpointInternal(ctx, params)
}

// SetMethodBreakpoint sets a breakpoint at a specific line within a method.
// For class methods, this uses include-relative line numbers (line 1 = first line of method).
// The method name is used to resolve the correct include for the breakpoint.
// Example: SetMethodBreakpoint(ctx, "ZCL_TEST================CP", "MY_METHOD", 5)
func (c *DebugWebSocketClient) SetMethodBreakpoint(ctx context.Context, program, method string, line int) (string, error) {
	params := map[string]any{
		"kind":    "line",
		"program": program,
		"method":  method,
		"line":    line,
	}
	return c.setBreakpointInternal(ctx, params)
}

// SetStatementBreakpoint sets a breakpoint on a specific ABAP statement.
// Example statements: "CALL FUNCTION", "SELECT", "LOOP", "CALL METHOD"
func (c *DebugWebSocketClient) SetStatementBreakpoint(ctx context.Context, statement string) (string, error) {
	params := map[string]any{
		"kind":      "statement",
		"statement": statement,
	}
	return c.setBreakpointInternal(ctx, params)
}

// SetExceptionBreakpoint sets a breakpoint that triggers when an exception is raised.
// Example exceptions: "CX_SY_ZERODIVIDE", "CX_SY_OPEN_SQL_DB"
func (c *DebugWebSocketClient) SetExceptionBreakpoint(ctx context.Context, exception string) (string, error) {
	params := map[string]any{
		"kind":      "exception",
		"exception": exception,
	}
	return c.setBreakpointInternal(ctx, params)
}

// GetBreakpoints returns all active breakpoints.
func (c *DebugWebSocketClient) GetBreakpoints(ctx context.Context) ([]map[string]any, error) {
	resp, err := c.sendRequest(ctx, "getBreakpoints", nil)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("getBreakpoints failed")
	}

	var result struct {
		Breakpoints []map[string]any `json:"breakpoints"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}

	return result.Breakpoints, nil
}

// DeleteBreakpoint removes a breakpoint by ID.
func (c *DebugWebSocketClient) DeleteBreakpoint(ctx context.Context, breakpointID string) error {
	params := map[string]any{
		"breakpointId": breakpointID,
	}

	resp, err := c.sendRequest(ctx, "deleteBreakpoint", params)
	if err != nil {
		return err
	}

	if !resp.Success {
		if resp.Error != nil {
			return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return fmt.Errorf("deleteBreakpoint failed")
	}

	return nil
}

// --- HTTP Breakpoint Operations (CL_ABAP_DEBUGGER → ABDBG_BPS table) ---
// These breakpoints are checked by HTTP-triggered execution, unlike TPDAPI breakpoints.

// SetHttpBreakpoint sets a breakpoint via CL_ABAP_DEBUGGER which writes to ABDBG_BPS.
// This breakpoint type is checked by HTTP-triggered execution (classrun, REST calls).
// For classes, the method parameter is required to resolve the include name.
func (c *DebugWebSocketClient) SetHttpBreakpoint(ctx context.Context, program string, line int, method string) (map[string]any, error) {
	params := map[string]any{
		"program": program,
		"line":    line,
	}
	if method != "" {
		params["method"] = method
	}

	resp, err := c.sendRequest(ctx, "setHttpBreakpoint", params)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("setHttpBreakpoint failed")
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetHttpBreakpoints returns all HTTP breakpoints from ABDBG_BPS table.
func (c *DebugWebSocketClient) GetHttpBreakpoints(ctx context.Context) (map[string]any, error) {
	resp, err := c.sendRequest(ctx, "getHttpBreakpoints", nil)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("getHttpBreakpoints failed")
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteHttpBreakpoints removes all HTTP breakpoints for the current user from ABDBG_BPS.
func (c *DebugWebSocketClient) DeleteHttpBreakpoints(ctx context.Context) (map[string]any, error) {
	resp, err := c.sendRequest(ctx, "deleteHttpBreakpoints", nil)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("deleteHttpBreakpoints failed")
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// --- Debugger Session Operations ---

// Listen waits for a debuggee to hit a breakpoint.
func (c *DebugWebSocketClient) Listen(ctx context.Context, timeout int) ([]DebugDebuggee, error) {
	if timeout <= 0 {
		timeout = 60
	}
	if timeout > 240 {
		timeout = 240
	}

	params := map[string]any{
		"timeout": timeout,
		"user":    c.GetUser(),
	}

	// Create a context with longer timeout for listen
	listenCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout+10)*time.Second)
	defer cancel()

	resp, err := c.sendRequest(listenCtx, "listen", params)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("listen failed")
	}

	var result struct {
		Status    string          `json:"status"`
		Debuggees []DebugDebuggee `json:"debuggees"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}

	if result.Status == "timeout" {
		return nil, nil // No debuggee caught
	}

	return result.Debuggees, nil
}

// Attach attaches to a debuggee.
func (c *DebugWebSocketClient) Attach(ctx context.Context, debuggeeID string) (*DebugStackFrame, error) {
	params := map[string]any{
		"debuggeeId": debuggeeID,
	}

	resp, err := c.sendRequest(ctx, "attach", params)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("attach failed")
	}

	var result struct {
		Attached   bool   `json:"attached"`
		DebuggeeID string `json:"debuggeeId"`
		Program    string `json:"program"`
		Include    string `json:"include"`
		Line       int    `json:"line"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.isAttached = true
	c.debuggeeID = debuggeeID
	c.mu.Unlock()

	return &DebugStackFrame{
		Program: result.Program,
		Include: result.Include,
		Line:    result.Line,
		Active:  true,
	}, nil
}

// Detach detaches from the current debuggee.
func (c *DebugWebSocketClient) Detach(ctx context.Context) error {
	resp, err := c.sendRequest(ctx, "detach", nil)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.isAttached = false
	c.debuggeeID = ""
	c.mu.Unlock()

	if !resp.Success {
		if resp.Error != nil {
			return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
	}

	return nil
}

// Step performs a step operation.
func (c *DebugWebSocketClient) Step(ctx context.Context, stepType string) (*DebugStackFrame, error) {
	params := map[string]any{
		"type": stepType,
	}

	resp, err := c.sendRequest(ctx, "step", params)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("step failed")
	}

	var result struct {
		Stepped   string `json:"stepped"`
		Program   string `json:"program"`
		Include   string `json:"include"`
		Line      int    `json:"line"`
		Procedure string `json:"procedure"`
		Ended     bool   `json:"ended"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}

	if result.Ended {
		c.mu.Lock()
		c.isAttached = false
		c.debuggeeID = ""
		c.mu.Unlock()
		return nil, nil // Debuggee ended
	}

	return &DebugStackFrame{
		Program:   result.Program,
		Include:   result.Include,
		Line:      result.Line,
		Procedure: result.Procedure,
		Active:    true,
	}, nil
}

// GetStack returns the current call stack.
func (c *DebugWebSocketClient) GetStack(ctx context.Context) ([]DebugStackFrame, error) {
	resp, err := c.sendRequest(ctx, "getStack", nil)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("getStack failed")
	}

	var result struct {
		Stack []DebugStackFrame `json:"stack"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}

	return result.Stack, nil
}

// GetVariables returns variable values.
func (c *DebugWebSocketClient) GetVariables(ctx context.Context, scope string) ([]WSDebugVariable, error) {
	if scope == "" {
		scope = "system"
	}

	params := map[string]any{
		"scope": scope,
	}

	resp, err := c.sendRequest(ctx, "getVariables", params)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("getVariables failed")
	}

	var result struct {
		Variables []WSDebugVariable `json:"variables"`
		Scope     string            `json:"scope"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}

	return result.Variables, nil
}

// GetStatus returns the current debug session status.
func (c *DebugWebSocketClient) GetStatus(ctx context.Context) (map[string]any, error) {
	resp, err := c.sendRequest(ctx, "getStatus", nil)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("getStatus failed")
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}

	return result, nil
}
