package adt

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// --- RFC Domain Operations ---

// RFCResult contains the result of an RFC call.
type RFCResult struct {
	Subrc   int            `json:"subrc"`
	Exports map[string]any `json:"exports"`
	Tables  map[string]any `json:"tables"`
}

// CallRFC calls a function module via WebSocket.
func (c *DebugWebSocketClient) CallRFC(ctx context.Context, function string, params map[string]string) (*RFCResult, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	id := c.GenerateID("rfc")

	// Build params as proper object (not JSON string)
	paramsObj := map[string]any{
		"function": function,
	}
	for k, v := range params {
		paramsObj[k] = v
	}

	rawMsg := map[string]any{
		"id":      id,
		"domain":  "rfc",
		"action":  "call",
		"params":  paramsObj,
		"timeout": 120000,
	}

	resp, err := c.SendRawRequest(ctx, id, rawMsg, 125*time.Second)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("RFC call failed")
	}

	var result RFCResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RunReport executes a report via background job (RFC domain, runReport action).
// This schedules the report as a background job, which runs in a separate work process
// and CAN hit external breakpoints.
func (c *DebugWebSocketClient) RunReport(ctx context.Context, report string, variant string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	id := c.GenerateID("rfc_run")

	paramsObj := map[string]any{
		"report": report,
	}
	if variant != "" {
		paramsObj["variant"] = variant
	}

	rawMsg := map[string]any{
		"id":      id,
		"domain":  "report",
		"action":  "runReport",
		"params":  paramsObj,
		"timeout": 30000,
	}

	respCh := make(chan *WSResponse, 1)
	c.RegisterPending(id, respCh)

	data, err := json.Marshal(rawMsg)
	if err != nil {
		c.UnregisterPending(id)
		return err
	}

	if err := c.WriteMessage(data); err != nil {
		c.UnregisterPending(id)
		return err
	}

	// Don't wait for response - the report might be blocked on breakpoint
	// The listener will catch the debuggee
	go func() {
		select {
		case <-respCh:
			// Report finished (no breakpoint hit or continued past)
		case <-time.After(60 * time.Second):
			c.UnregisterPending(id)
		}
	}()

	return nil
}

// RunReportSync executes a report via background job and waits for the response.
func (c *DebugWebSocketClient) RunReportSync(ctx context.Context, report string, variant string) (*WSResponse, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	id := c.GenerateID("rfc_run")

	paramsObj := map[string]any{
		"report": report,
	}
	if variant != "" {
		paramsObj["variant"] = variant
	}

	rawMsg := map[string]any{
		"id":      id,
		"domain":  "report",
		"action":  "runReport",
		"params":  paramsObj,
		"timeout": 30000,
	}

	return c.SendRawRequest(ctx, id, rawMsg, 60*time.Second)
}

// --- Package Operations ---

// MoveObjectResult contains the result of a package reassignment.
type MoveObjectResult struct {
	Success    bool   `json:"success"`
	Pgmid      string `json:"pgmid"`
	Object     string `json:"object"`
	ObjName    string `json:"obj_name"`
	NewPackage string `json:"new_package"`
	Message    string `json:"message"`
}

// MoveObject moves an ABAP object to a different package via WebSocket.
// Uses the rfc domain's moveToPackage action which calls ZADT_CL_TADIR_MOVE.
// objectType: CLAS, PROG, INTF, FUGR, etc.
// objectName: Name of the object (e.g., ZCL_TEST)
// newPackage: Target package (e.g., $ZRAY)
func (c *DebugWebSocketClient) MoveObject(ctx context.Context, objectType, objectName, newPackage string) (*MoveObjectResult, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	id := c.GenerateID("move")

	paramsObj := map[string]any{
		"object":      objectType,
		"obj_name":    objectName,
		"new_package": newPackage,
	}

	rawMsg := map[string]any{
		"id":      id,
		"domain":  "rfc",
		"action":  "moveToPackage",
		"params":  paramsObj,
		"timeout": 30000,
	}

	resp, err := c.SendRawRequest(ctx, id, rawMsg, 30*time.Second)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("move failed")
	}

	var result MoveObjectResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
