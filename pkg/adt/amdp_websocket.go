package adt

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// AMDPWebSocketClient manages AMDP debugging via WebSocket (ZADT_VSP).
// This replaces the HTTP-based AMDPSessionManager for more reliable debugging.
type AMDPWebSocketClient struct {
	*BaseWebSocketClient

	// AMDP-specific state
	mu        sync.RWMutex
	contextID string
	isActive  bool

	// Event channel for async events (breakpoint hits, etc.)
	Events chan *AMDPEvent
}

// AMDPEvent represents an async event from AMDP debugger.
type AMDPEvent struct {
	Kind       string         `json:"kind"`
	ContextID  string         `json:"context_id,omitempty"`
	Position   *AMDPPosition  `json:"position,omitempty"`
	Variables  []AMDPVariable `json:"variables,omitempty"`
	StackDepth int            `json:"stack_depth,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

// NewAMDPWebSocketClient creates a new WebSocket-based AMDP client.
func NewAMDPWebSocketClient(baseURL, client, user, password string, insecure bool) *AMDPWebSocketClient {
	c := &AMDPWebSocketClient{
		BaseWebSocketClient: NewBaseWebSocketClient(baseURL, client, user, password, insecure),
		Events:              make(chan *AMDPEvent, 10),
	}

	// Set disconnect callback to clean up AMDP state
	c.BaseWebSocketClient.onDisconnect = func() {
		c.mu.Lock()
		c.isActive = false
		c.contextID = ""
		c.mu.Unlock()
	}

	return c
}

// sendRequest sends a request to the amdp domain and waits for response.
func (c *AMDPWebSocketClient) sendRequest(ctx context.Context, action string, params map[string]any) (*WSResponse, error) {
	return c.SendDomainRequest(ctx, "amdp", action, params, 60*time.Second)
}

// Start starts an AMDP debug session.
func (c *AMDPWebSocketClient) Start(ctx context.Context, cascadeMode string) error {
	if cascadeMode == "" {
		cascadeMode = "FULL"
	}

	params := map[string]any{
		"user":        c.GetUser(),
		"cascadeMode": cascadeMode,
	}

	resp, err := c.sendRequest(ctx, "start", params)
	if err != nil {
		return err
	}

	if !resp.Success {
		if resp.Error != nil {
			return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return fmt.Errorf("start failed")
	}

	c.mu.Lock()
	c.isActive = true
	c.mu.Unlock()

	return nil
}

// Stop stops the AMDP debug session.
func (c *AMDPWebSocketClient) Stop(ctx context.Context) error {
	resp, err := c.sendRequest(ctx, "stop", nil)
	if err != nil {
		return err
	}

	if !resp.Success {
		if resp.Error != nil {
			return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return fmt.Errorf("stop failed")
	}

	c.mu.Lock()
	c.isActive = false
	c.contextID = ""
	c.mu.Unlock()

	return nil
}

// Resume resumes the debugger and waits for events.
func (c *AMDPWebSocketClient) Resume(ctx context.Context) (*AMDPResumeResult, error) {
	resp, err := c.sendRequest(ctx, "resume", nil)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("resume failed")
	}

	var result AMDPResumeResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("parse resume result: %w", err)
	}

	// Update context_id if we got a breakpoint hit
	for _, event := range result.Events {
		if event.Kind == "on_break" && event.ContextID != "" {
			c.mu.Lock()
			c.contextID = event.ContextID
			c.mu.Unlock()
			break
		}
	}

	return &result, nil
}

// AMDPResumeResult contains the result of a resume operation.
type AMDPResumeResult struct {
	Events []AMDPResumeEvent `json:"events"`
}

// AMDPResumeEvent represents an event from resume.
type AMDPResumeEvent struct {
	Kind           string              `json:"kind"`
	ContextID      string              `json:"context_id,omitempty"`
	BPClientID     string              `json:"bp_client_id,omitempty"`
	ABAPPosition   *AMDPABAPPosition   `json:"abap_position,omitempty"`
	NativePosition *AMDPNativePosition `json:"native_position,omitempty"`
	VariableCount  int                 `json:"variable_count,omitempty"`
	StackDepth     int                 `json:"stack_depth,omitempty"`
	Aborted        bool                `json:"aborted,omitempty"`
}

// AMDPABAPPosition represents a position in ABAP source.
type AMDPABAPPosition struct {
	Program string `json:"program"`
	Include string `json:"include"`
	Line    int    `json:"line"`
}

// AMDPNativePosition represents a position in SQLScript.
type AMDPNativePosition struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Line   int    `json:"line"`
}

// Step performs a step operation.
func (c *AMDPWebSocketClient) Step(ctx context.Context, stepType string) error {
	if stepType == "" {
		stepType = "over"
	}

	params := map[string]any{
		"type": stepType,
	}

	resp, err := c.sendRequest(ctx, "step", params)
	if err != nil {
		return err
	}

	if !resp.Success {
		if resp.Error != nil {
			return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return fmt.Errorf("step failed")
	}

	return nil
}

// SetBreakpoint sets a breakpoint in AMDP code.
func (c *AMDPWebSocketClient) SetBreakpoint(ctx context.Context, program string, line int) error {
	params := map[string]any{
		"program": program,
		"line":    line,
	}

	resp, err := c.sendRequest(ctx, "setBreakpoint", params)
	if err != nil {
		return err
	}

	if !resp.Success {
		if resp.Error != nil {
			return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return fmt.Errorf("set breakpoint failed")
	}

	return nil
}

// GetBreakpoints returns currently set AMDP breakpoints.
func (c *AMDPWebSocketClient) GetBreakpoints(ctx context.Context) (*AMDPBreakpointsResult, error) {
	resp, err := c.sendRequest(ctx, "getBreakpoints", nil)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("get breakpoints failed")
	}

	var result AMDPBreakpointsResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("parse breakpoints result: %w", err)
	}

	return &result, nil
}

// AMDPBreakpointsResult contains breakpoint information.
type AMDPBreakpointsResult struct {
	Breakpoints []AMDPBreakpoint `json:"breakpoints"`
}

// GetVariables returns AMDP session variables.
func (c *AMDPWebSocketClient) GetVariables(ctx context.Context) (*AMDPVariablesResult, error) {
	resp, err := c.sendRequest(ctx, "getVariables", nil)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("get variables failed")
	}

	var result AMDPVariablesResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("parse variables result: %w", err)
	}

	return &result, nil
}

// AMDPVariablesResult contains variable information.
type AMDPVariablesResult struct {
	Variables []AMDPVariable `json:"variables"`
}

// GetStatus returns current session status.
func (c *AMDPWebSocketClient) GetStatus(ctx context.Context) (*AMDPStatusResult, error) {
	resp, err := c.sendRequest(ctx, "getStatus", nil)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("get status failed")
	}

	var result AMDPStatusResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("parse status result: %w", err)
	}

	return &result, nil
}

// AMDPStatusResult contains session status.
type AMDPStatusResult struct {
	Active    bool   `json:"active"`
	ContextID string `json:"context_id"`
}

// Execute runs an AMDP method within the debug session context.
// This allows breakpoints to be hit since execution is in the same session.
func (c *AMDPWebSocketClient) Execute(ctx context.Context, class, method string, count int) (*AMDPExecuteResult, error) {
	params := map[string]any{
		"class":  class,
		"method": method,
		"count":  count,
	}

	resp, err := c.sendRequest(ctx, "execute", params)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("execute failed")
	}

	var result AMDPExecuteResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("parse execute result: %w", err)
	}

	return &result, nil
}

// AMDPExecuteResult contains execution result.
type AMDPExecuteResult struct {
	Status string           `json:"status"`
	Class  string           `json:"class"`
	Method string           `json:"method"`
	Rows   int              `json:"rows"`
	Data   []AMDPExecuteRow `json:"data"`
}

// AMDPExecuteRow contains a result row.
type AMDPExecuteRow struct {
	ID     int    `json:"id"`
	Value  string `json:"value"`
	Square int    `json:"square"`
}

// ExecuteAndDebug combines start, breakpoint, execute, and resume in a single call.
// This solves the session blocking issue by running everything in one ABAP request.
// Returns debug events from hitting the breakpoint.
func (c *AMDPWebSocketClient) ExecuteAndDebug(ctx context.Context, class, method string, line, count int, cascadeMode string) (*AMDPExecuteDebugResult, error) {
	if cascadeMode == "" {
		cascadeMode = "FULL"
	}

	params := map[string]any{
		"class":       class,
		"method":      method,
		"line":        line,
		"count":       count,
		"cascadeMode": cascadeMode,
	}

	resp, err := c.sendRequest(ctx, "executeAndDebug", params)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("executeAndDebug failed")
	}

	var result AMDPExecuteDebugResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("parse executeAndDebug result: %w", err)
	}

	// Update context_id if we got a breakpoint hit
	for _, event := range result.Events {
		if event.Kind == "on_break" && event.ContextID != "" {
			c.mu.Lock()
			c.contextID = event.ContextID
			c.isActive = true
			c.mu.Unlock()
			break
		}
	}

	return &result, nil
}

// AMDPExecuteDebugResult contains the result of executeAndDebug operation.
type AMDPExecuteDebugResult struct {
	Status         string            `json:"status"`
	Class          string            `json:"class"`
	Method         string            `json:"method"`
	Line           int               `json:"line"`
	ExecutionRows  int               `json:"execution_rows"`
	ExecutionError string            `json:"execution_error,omitempty"`
	Events         []AMDPResumeEvent `json:"events"`
}

// IsActive returns true if AMDP session is active.
func (c *AMDPWebSocketClient) IsActive() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isActive
}

// GetContextID returns the current context ID.
func (c *AMDPWebSocketClient) GetContextID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.contextID
}

// GetAbapDocumentation retrieves ABAP keyword documentation via WebSocket (ZADT_VSP).
// Uses CL_ABAP_DOCU on the SAP system to get the real documentation.
func (c *AMDPWebSocketClient) GetAbapDocumentation(ctx context.Context, keyword string) (*AbapHelpResponse, error) {
	params := map[string]any{
		"keyword": keyword,
	}

	resp, err := c.SendDomainRequest(ctx, "system", "get_abap_help", params, 30*time.Second)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("get_abap_help failed: %s - %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("get_abap_help failed")
	}

	result := &AbapHelpResponse{}
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return result, nil
}
