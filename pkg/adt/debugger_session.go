// ABOUTME: Debug session API for ABAP debugger.
// ABOUTME: Manages attach, step, stack, and variable operations.

package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Debug Session API ---

// DebuggerAttach attaches to a debuggee that has hit a breakpoint.
// debuggeeId: The ID of the debuggee (from ListenResult.Debuggee.ID)
// user: Optional user for user-mode debugging
func (c *Client) DebuggerAttach(ctx context.Context, debuggeeID string, user string) (*DebugAttachResult, error) {
	query := url.Values{}
	query.Set("method", "attach")
	query.Set("debuggeeId", debuggeeID)
	query.Set("dynproDebugging", "true")
	query.Set("debuggingMode", string(DebuggingModeUser))
	if user != "" {
		query.Set("requestUser", user)
	}

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/debugger", &RequestOptions{
		Method: http.MethodPost,
		Accept: "application/xml",
		Query:  query,
	})
	if err != nil {
		return nil, fmt.Errorf("debugger attach failed: %w", err)
	}

	return parseAttachResponse(resp.Body)
}

// DebuggerDetach terminates the current debug session.
// This releases the debuggee and ends the debugging session.
func (c *Client) DebuggerDetach(ctx context.Context) error {
	_, err := c.DebuggerStep(ctx, DebugTerminate, "")
	return err
}

// DebuggerStep performs a step operation in the debugger.
// stepType: One of stepInto, stepOver, stepReturn, stepContinue, stepRunToLine, stepJumpToLine, terminateDebuggee
// uri: Required for stepRunToLine and stepJumpToLine (target line URI)
func (c *Client) DebuggerStep(ctx context.Context, stepType DebugStepType, uri string) (*DebugStepResult, error) {
	query := url.Values{}
	query.Set("method", string(stepType))
	if uri != "" {
		query.Set("uri", uri)
	}

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/debugger", &RequestOptions{
		Method: http.MethodPost,
		Accept: "application/xml",
		Query:  query,
	})
	if err != nil {
		return nil, fmt.Errorf("debugger step failed: %w", err)
	}

	return parseStepResponse(resp.Body)
}

// DebuggerGetStack retrieves the current call stack.
// semanticURIs: If true, returns semantic URIs that can be used for navigation
func (c *Client) DebuggerGetStack(ctx context.Context, semanticURIs bool) (*DebugStackInfo, error) {
	query := url.Values{}
	query.Set("method", "getStack")
	query.Set("emode", "_")
	if semanticURIs {
		query.Set("semanticURIs", "true")
	}

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/debugger/stack", &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
		Query:  query,
	})
	if err != nil {
		return nil, fmt.Errorf("debugger get stack failed: %w", err)
	}

	return parseStackResponse(resp.Body)
}

// DebuggerGetVariables retrieves the values of specific variables.
// variableIDs: List of variable IDs to retrieve (e.g., ["@ROOT", "@DATAAGING", "LV_COUNT"])
func (c *Client) DebuggerGetVariables(ctx context.Context, variableIDs []string) ([]DebugVariable, error) {
	if len(variableIDs) == 0 {
		return nil, fmt.Errorf("at least one variable ID required")
	}

	// Build request body
	var varElements []string
	for _, id := range variableIDs {
		varElements = append(varElements, fmt.Sprintf("<STPDA_ADT_VARIABLE><ID>%s</ID></STPDA_ADT_VARIABLE>", XMLEscape(id)))
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>%s</DATA></asx:values></asx:abap>`,
		strings.Join(varElements, ""))

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/debugger", &RequestOptions{
		Method:      http.MethodPost,
		ContentType: "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.debugger.Variables",
		Accept:      "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.debugger.Variables",
		Query:       url.Values{"method": []string{"getVariables"}},
		Body:        []byte(body),
	})
	if err != nil {
		return nil, fmt.Errorf("debugger get variables failed: %w", err)
	}

	return parseVariablesResponse(resp.Body)
}

// DebuggerGetChildVariables retrieves child variables (for expanding structures/tables).
// parentIDs: List of parent variable IDs (e.g., ["@ROOT", "@DATAAGING"] for top-level)
func (c *Client) DebuggerGetChildVariables(ctx context.Context, parentIDs []string) (*DebugChildVariablesInfo, error) {
	if len(parentIDs) == 0 {
		parentIDs = []string{"@ROOT", "@DATAAGING"}
	}

	// Build request body
	var hierElements []string
	for _, id := range parentIDs {
		hierElements = append(hierElements, fmt.Sprintf("<STPDA_ADT_VARIABLE_HIERARCHY><PARENT_ID>%s</PARENT_ID></STPDA_ADT_VARIABLE_HIERARCHY>", XMLEscape(id)))
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA><HIERARCHIES>%s</HIERARCHIES></DATA></asx:values></asx:abap>`,
		strings.Join(hierElements, ""))

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/debugger", &RequestOptions{
		Method:      http.MethodPost,
		ContentType: "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.debugger.ChildVariables",
		Accept:      "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.debugger.ChildVariables",
		Query:       url.Values{"method": []string{"getChildVariables"}},
		Body:        []byte(body),
	})
	if err != nil {
		return nil, fmt.Errorf("debugger get child variables failed: %w", err)
	}

	return parseChildVariablesResponse(resp.Body)
}

// DebuggerSetVariableValue modifies the value of a variable during debugging.
// variableName: The name of the variable to modify
// value: The new value as a string
func (c *Client) DebuggerSetVariableValue(ctx context.Context, variableName, value string) (string, error) {
	query := url.Values{}
	query.Set("method", "setVariableValue")
	query.Set("variableName", variableName)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/debugger", &RequestOptions{
		Method: http.MethodPost,
		Query:  query,
		Body:   []byte(value),
	})
	if err != nil {
		return "", fmt.Errorf("debugger set variable value failed: %w", err)
	}

	return string(resp.Body), nil
}

// DebuggerGoToStack navigates to a specific stack entry.
// stackURI: The stack URI (e.g., "/sap/bc/adt/debugger/stack/type/ABAP/position/3")
func (c *Client) DebuggerGoToStack(ctx context.Context, stackURI string) error {
	_, err := c.transport.Request(ctx, stackURI, &RequestOptions{
		Method: http.MethodPut,
	})
	if err != nil {
		return fmt.Errorf("debugger go to stack failed: %w", err)
	}
	return nil
}
