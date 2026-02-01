// ABOUTME: External breakpoint API for ABAP debugger.
// ABOUTME: Manages persistent breakpoints via REST API.

package adt

import (
	"context"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// terminalID is a unique identifier for this vsp session.
// IMPORTANT: This must be deterministic across MCP tool calls since each call
// may be a separate process. We use a fixed ID based on the username.
var (
	terminalIDUser   string // Set via SetTerminalIDUser for deterministic ID
	terminalIDCustom string // Set via SetTerminalID for SAP GUI compatibility
)

// SetTerminalIDUser sets the username used to generate a deterministic terminal ID.
// This should be called early in the MCP server initialization.
// If not set, a fixed default terminal ID will be used.
func SetTerminalIDUser(user string) {
	terminalIDUser = user
}

// SetTerminalID sets a custom terminal ID (e.g., from SAP GUI).
// Use the same ID as SAP GUI to enable cross-tool breakpoint sharing.
// SAP GUI stores this in: Windows Registry HKCU\Software\SAP\ABAP Debugging\TerminalID
// or on Linux/Mac: ~/.SAP/ABAPDebugging/terminalId
func SetTerminalID(id string) {
	terminalIDCustom = id
}

// getTerminalID returns a terminal ID for this vsp session.
// Priority: 1) Custom ID (SAP GUI), 2) User-based ID, 3) Default
func getTerminalID() string {
	// 1. Use custom terminal ID if set (for SAP GUI compatibility)
	if terminalIDCustom != "" {
		return terminalIDCustom
	}
	// 2. Deterministic: use hash of username for consistent ID across processes
	if terminalIDUser != "" {
		h := make([]byte, 8)
		for i, c := range terminalIDUser {
			h[i%8] ^= byte(c)
		}
		return "vsp-" + hex.EncodeToString(h)
	}
	// 3. Fixed default - better than random for MCP scenarios
	return "vsp-mcp-default"
}

// --- External Breakpoints API ---

// SetExternalBreakpoint creates an external (persistent) breakpoint.
// These breakpoints persist across sessions and trigger when the specified user runs code that hits them.
//
// For line breakpoints:
//   - objectURI: ADT URI of the object (e.g., "/sap/bc/adt/programs/programs/ZTEST")
//   - line: Source line number (1-based)
//
// For exception breakpoints:
//   - exceptionClass: Exception class name (e.g., "CX_SY_ZERODIVIDE")
//
// For statement breakpoints:
//   - statement: Statement type (e.g., "WRITE", "CALL FUNCTION")
func (c *Client) SetExternalBreakpoint(ctx context.Context, req *BreakpointRequest) (*BreakpointResponse, error) {
	if req.Scope == "" {
		req.Scope = BreakpointScopeExternal
	}
	if req.DebuggingMode == "" {
		req.DebuggingMode = DebuggingModeUser
	}

	body, err := buildBreakpointRequestXML(req)
	if err != nil {
		return nil, fmt.Errorf("building breakpoint request: %w", err)
	}

	// POST with XML body - all parameters are in the XML, no query params needed
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/debugger/breakpoints", &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(body),
		ContentType: "application/xml",
		Accept:      "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("set external breakpoint failed: %w", err)
	}

	return parseBreakpointResponse(resp.Body)
}

// GetExternalBreakpoints retrieves all external breakpoints for a user.
// user is required for external breakpoints in user debugging mode.
func (c *Client) GetExternalBreakpoints(ctx context.Context, user string) (*BreakpointResponse, error) {
	query := url.Values{}
	query.Set("scope", string(BreakpointScopeExternal))
	query.Set("debuggingMode", string(DebuggingModeUser))
	// All four parameters required - terminalId identifies this vsp session
	query.Set("requestUser", user)
	query.Set("terminalId", getTerminalID())
	query.Set("ideId", "vsp")

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/debugger/breakpoints", &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
		Query:  query,
	})
	if err != nil {
		return nil, fmt.Errorf("get external breakpoints failed: %w", err)
	}

	// Handle empty response (no breakpoints)
	if len(resp.Body) == 0 {
		return &BreakpointResponse{}, nil
	}

	return parseBreakpointResponse(resp.Body)
}

// DeleteExternalBreakpoint removes an external breakpoint by ID.
// user is required for external breakpoints in user debugging mode.
func (c *Client) DeleteExternalBreakpoint(ctx context.Context, breakpointID string, user string) error {
	query := url.Values{}
	query.Set("scope", string(BreakpointScopeExternal))
	query.Set("debuggingMode", string(DebuggingModeUser))
	// All parameters required - terminalId identifies this vsp session
	query.Set("requestUser", user)
	query.Set("terminalId", getTerminalID())
	query.Set("ideId", "vsp")

	endpoint := fmt.Sprintf("/sap/bc/adt/debugger/breakpoints/%s", url.PathEscape(breakpointID))
	_, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method: http.MethodDelete,
		Query:  query,
	})
	if err != nil {
		return fmt.Errorf("delete external breakpoint failed: %w", err)
	}

	return nil
}

// DeleteAllExternalBreakpoints removes all external breakpoints for a user.
func (c *Client) DeleteAllExternalBreakpoints(ctx context.Context, user string) error {
	// Get all breakpoints first
	bps, err := c.GetExternalBreakpoints(ctx, user)
	if err != nil {
		return fmt.Errorf("getting breakpoints for deletion: %w", err)
	}

	// Delete each one
	for _, bp := range bps.Breakpoints {
		if err := c.DeleteExternalBreakpoint(ctx, bp.ID, user); err != nil {
			return fmt.Errorf("deleting breakpoint %s: %w", bp.ID, err)
		}
	}

	return nil
}

// ValidateBreakpointCondition checks if a breakpoint condition expression is valid.
func (c *Client) ValidateBreakpointCondition(ctx context.Context, condition string) (bool, string, error) {
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<dbg:condition xmlns:dbg="http://www.sap.com/adt/debugger">%s</dbg:condition>`,
		XMLEscape(condition))

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/debugger/breakpoints/conditions", &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(body),
		ContentType: "application/xml",
		Accept:      "application/xml",
	})
	if err != nil {
		// Check if it's a validation error (400) vs other error
		return false, err.Error(), nil
	}

	// Parse validation result
	type validationResult struct {
		Valid   bool   `xml:"valid,attr"`
		Message string `xml:"message,attr"`
	}

	xmlStr := StripXMLNamespaces(string(resp.Body), "dbg:")
	var result validationResult
	if err := xml.Unmarshal([]byte(xmlStr), &result); err != nil {
		// If parsing fails but request succeeded, assume valid
		return true, "", nil
	}

	return result.Valid, result.Message, nil
}

// --- Convenience functions for creating breakpoints ---

// NewLineBreakpoint creates a line breakpoint request.
// objectURI: ADT URI (e.g., "/sap/bc/adt/programs/programs/ZTEST/source/main")
// line: Source line number (1-based)
func NewLineBreakpoint(objectURI string, line int) Breakpoint {
	return Breakpoint{
		Kind:    BreakpointKindLine,
		Enabled: true,
		URI:     objectURI,
		Line:    line,
	}
}

// NewExceptionBreakpoint creates an exception breakpoint request.
// exceptionClass: Exception class name (e.g., "CX_SY_ZERODIVIDE")
func NewExceptionBreakpoint(exceptionClass string) Breakpoint {
	return Breakpoint{
		Kind:      BreakpointKindException,
		Enabled:   true,
		Exception: exceptionClass,
	}
}

// NewStatementBreakpoint creates a statement breakpoint request.
// statement: Statement type (e.g., "WRITE", "CALL FUNCTION", "RAISE EXCEPTION")
func NewStatementBreakpoint(statement string) Breakpoint {
	return Breakpoint{
		Kind:      BreakpointKindStatement,
		Enabled:   true,
		Statement: statement,
	}
}

// NewMessageBreakpoint creates a message breakpoint request.
// messageID: Message ID (e.g., "001")
// messageType: Message type (E=Error, W=Warning, I=Info, S=Success, A=Abort)
func NewMessageBreakpoint(messageID string, messageType string) Breakpoint {
	return Breakpoint{
		Kind:        BreakpointKindMessage,
		Enabled:     true,
		MessageID:   messageID,
		MessageType: messageType,
	}
}

// --- Helper functions ---

func buildBreakpointRequestXML(req *BreakpointRequest) (string, error) {
	// Set defaults
	ideID := req.IdeID
	if ideID == "" {
		ideID = "vsp"
	}
	// Use provided terminalId or generate session-unique one
	termID := req.TerminalID
	if termID == "" {
		termID = getTerminalID()
	}

	var bpElements []string

	for _, bp := range req.Breakpoints {
		// Default enabled to true if not explicitly disabled
		enabledAttr := `enabled="true"`

		switch bp.Kind {
		case BreakpointKindLine:
			// Line breakpoint: uses adtcore:uri attribute with fragment for line number
			uri := bp.URI
			if bp.Line > 0 {
				uri = fmt.Sprintf("%s#start=%d", bp.URI, bp.Line)
			}
			attrs := fmt.Sprintf(`kind="line" %s adtcore:uri="%s"`, enabledAttr, XMLEscape(uri))
			if bp.Condition != "" {
				attrs += fmt.Sprintf(` condition="%s"`, XMLEscape(bp.Condition))
			}
			bpElements = append(bpElements, fmt.Sprintf(`<breakpoint %s/>`, attrs))

		case BreakpointKindException:
			bpElements = append(bpElements, fmt.Sprintf(`<breakpoint kind="exception" %s exceptionClass="%s"/>`,
				enabledAttr, XMLEscape(bp.Exception)))

		case BreakpointKindStatement:
			bpElements = append(bpElements, fmt.Sprintf(`<breakpoint kind="statement" %s statement="%s"/>`,
				enabledAttr, XMLEscape(bp.Statement)))

		case BreakpointKindMessage:
			bpElements = append(bpElements, fmt.Sprintf(`<breakpoint kind="message" %s msgId="%s" msgTy="%s"/>`,
				enabledAttr, XMLEscape(bp.MessageID), XMLEscape(bp.MessageType)))
		}
	}

	// Build breakpoint content
	bpContent := ""
	if len(bpElements) > 0 {
		bpContent = "\n  " + strings.Join(bpElements, "\n  ") + "\n"
	}

	// Build optional attributes
	optionalAttrs := ""
	if req.SystemDebugging {
		optionalAttrs += ` systemDebugging="true"`
	}
	if req.Deactivated {
		optionalAttrs += ` deactivated="true"`
	}

	// XML format based on Simple Transformation TPDA_ADT_BREAKPOINTS_REQUEST
	// terminalId is always included - it identifies this vsp session to SAP
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<dbg:breakpoints xmlns:dbg="http://www.sap.com/adt/debugger" xmlns:adtcore="http://www.sap.com/adt/core" debuggingMode="%s" scope="%s" requestUser="%s" terminalId="%s" ideId="%s"%s>%s</dbg:breakpoints>`,
		string(req.DebuggingMode), string(req.Scope), XMLEscape(req.User),
		XMLEscape(termID), XMLEscape(ideID),
		optionalAttrs, bpContent), nil
}

func parseBreakpointResponse(data []byte) (*BreakpointResponse, error) {
	// Strip namespace prefixes for easier parsing
	xmlStr := StripXMLNamespaces(string(data), "dbg:", "adtcore:")

	// Response format: <breakpoints><breakpoint kind="..." id="..." uri="..."/></breakpoints>
	type xmlBreakpoint struct {
		ID             string `xml:"id,attr"`
		Kind           string `xml:"kind,attr"`
		Enabled        bool   `xml:"enabled,attr"`
		IsActive       bool   `xml:"isActive,attr"`
		URI            string `xml:"uri,attr"`       // adtcore:uri attribute
		Condition      string `xml:"condition,attr"` // condition attribute
		ExceptionClass string `xml:"exceptionClass,attr"`
		Statement      string `xml:"statement,attr"`
		MsgID          string `xml:"msgId,attr"`
		MsgTy          string `xml:"msgTy,attr"`
		ErrorMessage   string `xml:"errorMessage,attr"` // Error case
		ObjectName     string `xml:"name,attr"`         // adtcore:name attribute
	}

	// Parse root <breakpoints> element directly
	type xmlBreakpoints struct {
		XMLName     xml.Name        `xml:"breakpoints"`
		Breakpoints []xmlBreakpoint `xml:"breakpoint"`
	}

	var resp xmlBreakpoints
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing breakpoint response: %w", err)
	}

	result := &BreakpointResponse{}
	for _, bp := range resp.Breakpoints {
		// Skip breakpoints with error messages (couldn't be created)
		if bp.ErrorMessage != "" {
			continue
		}

		breakpoint := Breakpoint{
			ID:          bp.ID,
			Kind:        BreakpointKind(bp.Kind),
			Enabled:     bp.Enabled,
			IsActive:    bp.IsActive,
			URI:         bp.URI,
			Condition:   bp.Condition,
			Exception:   bp.ExceptionClass,
			Statement:   bp.Statement,
			MessageID:   bp.MsgID,
			MessageType: bp.MsgTy,
			ObjectName:  bp.ObjectName,
		}

		// Extract line number from URI fragment if present
		if strings.Contains(breakpoint.URI, "#start=") {
			parts := strings.SplitN(breakpoint.URI, "#start=", 2)
			if len(parts) == 2 {
				breakpoint.URI = parts[0]
				_, _ = fmt.Sscanf(parts[1], "%d", &breakpoint.Line)
			}
		}

		result.Breakpoints = append(result.Breakpoints, breakpoint)
	}

	return result, nil
}
