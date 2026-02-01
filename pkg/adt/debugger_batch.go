// ABOUTME: Batch debugger API for ABAP debugger (Eclipse-compatible).
// ABOUTME: Sends multiple debug operations in a single request.

package adt

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// --- Batch Debugger API (Eclipse-compatible) ---

// DebuggerBatchRequest sends multiple debugger operations in a single batch request.
// This matches the Eclipse ADT debugging protocol.
func (c *Client) DebuggerBatchRequest(ctx context.Context, operations []DebugBatchOperation) ([]DebugBatchResponse, error) {
	if len(operations) == 0 {
		return nil, fmt.Errorf("no operations provided")
	}

	// Generate unique boundary
	boundary := fmt.Sprintf("batch_%s", generateBoundary())

	// Build multipart body
	var body strings.Builder
	for _, op := range operations {
		body.WriteString("--")
		body.WriteString(boundary)
		body.WriteString("\r\n")
		body.WriteString("Content-Type: application/http\r\n")
		body.WriteString("content-transfer-encoding: binary\r\n")
		body.WriteString("\r\n")

		// HTTP request line
		method := op.Method
		if method == "" {
			method = "POST"
		}
		fmt.Fprintf(&body, "%s %s HTTP/1.1\r\n", method, op.Path)

		// Headers
		if op.Accept != "" {
			fmt.Fprintf(&body, "Accept:%s\r\n", op.Accept)
		} else {
			body.WriteString("Accept:application/xml\r\n")
		}
		if op.ContentType != "" {
			fmt.Fprintf(&body, "Content-Type:%s\r\n", op.ContentType)
		}

		body.WriteString("\r\n")

		// Body
		if op.Body != "" {
			body.WriteString(op.Body)
		}

		body.WriteString("\r\n")
	}
	body.WriteString("--")
	body.WriteString(boundary)
	body.WriteString("--")

	// Send batch request
	contentType := fmt.Sprintf("multipart/mixed; boundary=%s", boundary)
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/debugger/batch", &RequestOptions{
		Method:      http.MethodPost,
		ContentType: contentType,
		Accept:      "multipart/mixed",
		Body:        []byte(body.String()),
		Headers: map[string]string{
			"User-Agent":          "vsp/1.0 (compatible; Eclipse ADT)",
			"X-sap-adt-profiling": "server-time",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("batch request failed: %w", err)
	}

	// Parse multipart response
	respContentType := resp.Headers.Get("Content-Type")
	return parseBatchResponse(resp.Body, respContentType)
}

// generateBoundary creates a unique boundary string for multipart requests.
func generateBoundary() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(time.Now().UnixNano() >> (i * 4) & 0xff)
	}
	return hex.EncodeToString(b)
}

// parseBatchResponse parses a multipart/mixed response from a batch request.
func parseBatchResponse(body []byte, contentType string) ([]DebugBatchResponse, error) {
	// Extract boundary from Content-Type
	boundary := ""
	for _, part := range strings.Split(contentType, ";") {
		part = strings.TrimSpace(part)
		if b, found := strings.CutPrefix(part, "boundary="); found {
			boundary = b
			break
		}
	}

	if boundary == "" {
		// Not multipart, return single response
		return []DebugBatchResponse{{
			StatusCode: 200,
			Body:       body,
		}}, nil
	}

	// Split by boundary
	parts := strings.Split(string(body), "--"+boundary)
	var responses []DebugBatchResponse

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "--" {
			continue
		}

		// Parse HTTP response from part
		resp := DebugBatchResponse{StatusCode: 200}

		// Find headers/body separator
		idx := strings.Index(part, "\r\n\r\n")
		if idx == -1 {
			idx = strings.Index(part, "\n\n")
		}

		if idx != -1 {
			// Skip the content-type/transfer-encoding headers of the multipart part
			remaining := part[idx+4:]

			// Now find the actual HTTP response
			httpIdx := strings.Index(remaining, "\r\n\r\n")
			if httpIdx == -1 {
				httpIdx = strings.Index(remaining, "\n\n")
			}

			if httpIdx != -1 {
				resp.Body = []byte(remaining[httpIdx+4:])
			} else {
				resp.Body = []byte(remaining)
			}
		}

		responses = append(responses, resp)
	}

	return responses, nil
}

// DebuggerStepWithBatch performs a step operation and retrieves stack+variables in one batch.
// This matches Eclipse's behavior of combining multiple operations.
func (c *Client) DebuggerStepWithBatch(ctx context.Context, stepType DebugStepType, uri string) (*DebugStepResult, *DebugStackInfo, []DebugVariable, error) {
	operations := []DebugBatchOperation{
		{
			Path:   fmt.Sprintf("/sap/bc/adt/debugger?method=%s", stepType),
			Accept: "application/xml",
		},
		{
			Path:   "/sap/bc/adt/debugger?emode=_&semanticURIs=true&method=getStack",
			Accept: "application/xml",
		},
		{
			Path:        "/sap/bc/adt/debugger?method=getChildVariables",
			Accept:      "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.debugger.ChildVariables",
			ContentType: "application/vnd.sap.as+xml; charset=UTF-8; dataname=com.sap.adt.debugger.ChildVariables",
			Body:        `<?xml version="1.0" encoding="UTF-8" ?><asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><HIERARCHIES><STPDA_ADT_VARIABLE_HIERARCHY><PARENT_ID>@ROOT</PARENT_ID></STPDA_ADT_VARIABLE_HIERARCHY></HIERARCHIES></DATA></asx:values></asx:abap>`,
		},
		{
			Path:        "/sap/bc/adt/debugger?method=getVariables",
			Accept:      "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.debugger.Variables",
			ContentType: "application/vnd.sap.as+xml; charset=UTF-8; dataname=com.sap.adt.debugger.Variables",
			Body:        `<?xml version="1.0" encoding="UTF-8" ?><asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><STPDA_ADT_VARIABLE><ID>SY-SUBRC</ID></STPDA_ADT_VARIABLE></DATA></asx:values></asx:abap>`,
		},
	}

	if uri != "" {
		operations[0].Path = fmt.Sprintf("/sap/bc/adt/debugger?method=%s&uri=%s", stepType, url.QueryEscape(uri))
	}

	responses, err := c.DebuggerBatchRequest(ctx, operations)
	if err != nil {
		return nil, nil, nil, err
	}

	// Parse responses (best effort)
	var stepResult *DebugStepResult
	var stackInfo *DebugStackInfo
	var variables []DebugVariable

	if len(responses) > 0 && len(responses[0].Body) > 0 {
		stepResult, _ = parseStepResponse(responses[0].Body)
	}
	if len(responses) > 1 && len(responses[1].Body) > 0 {
		stackInfo, _ = parseStackResponse(responses[1].Body)
	}
	// Variables parsing would go here if needed

	return stepResult, stackInfo, variables, nil
}
