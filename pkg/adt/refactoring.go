package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Refactoring: Rename ---

// RenameProblem represents an issue found during rename evaluation.
type RenameProblem struct {
	Severity    string `json:"severity" xml:"severity,attr"`
	Description string `json:"description" xml:"description,attr"`
	URI         string `json:"uri,omitempty" xml:"uri,attr"`
	Line        int    `json:"line,omitempty" xml:"line,attr"`
	Column      int    `json:"column,omitempty" xml:"column,attr"`
}

// RenameEvaluateResult is the result of rename feasibility check.
type RenameEvaluateResult struct {
	Feasible    bool            `json:"feasible"`
	ChangeCount int             `json:"changeCount"`
	Problems    []RenameProblem `json:"problems,omitempty"`
}

// RenameChange represents a single text change from rename preview.
type RenameChange struct {
	URI     string `json:"uri" xml:"uri,attr"`
	Line    int    `json:"line" xml:"line,attr"`
	Column  int    `json:"column" xml:"column,attr"`
	Length  int    `json:"length" xml:"length,attr"`
	OldText string `json:"oldText" xml:"oldText,attr"`
	NewText string `json:"newText" xml:"newText,attr"`
}

// RenamePreviewResult is the result of rename preview showing all affected locations.
type RenamePreviewResult struct {
	Changes []RenameChange `json:"changes"`
}

// RenameExecuteResult is the result of rename execution.
type RenameExecuteResult struct {
	Success         bool     `json:"success"`
	AffectedObjects []string `json:"affectedObjects,omitempty"`
	Message         string   `json:"message,omitempty"`
}

// RenameEvaluate checks if a rename is feasible at the given position.
// objectURI is the ADT object URI (e.g., "/sap/bc/adt/programs/programs/ZTEST")
// line and col are 1-based position of the symbol to rename.
// source is the current source code of the object.
// newName is the proposed new name.
func (c *Client) RenameEvaluate(ctx context.Context, objectURI string, line, col int, source, newName string) (*RenameEvaluateResult, error) {
	if err := c.checkSafety(OpUpdate, "RenameRefactoring"); err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("method", "evaluate")
	q.Set("uri", fmt.Sprintf("%s#start=%d,%d", objectURI, line, col))
	q.Set("newName", newName)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/refactoring/rename", &RequestOptions{
		Method:      http.MethodPost,
		Query:       q,
		Body:        []byte(source),
		ContentType: "text/plain",
		Accept:      "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("rename evaluate failed: %w", err)
	}

	return parseRenameEvaluateResult(resp.Body)
}

func parseRenameEvaluateResult(data []byte) (*RenameEvaluateResult, error) {
	// Strip namespace prefixes for easier parsing
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "refactoring:", "")

	type problem struct {
		Severity    string `xml:"severity,attr"`
		Description string `xml:"description,attr"`
		URI         string `xml:"uri,attr"`
		Line        int    `xml:"line,attr"`
		Column      int    `xml:"column,attr"`
	}
	type evalResult struct {
		XMLName     xml.Name  `xml:"evaluateResult"`
		Feasible    bool      `xml:"feasible,attr"`
		ChangeCount int       `xml:"changeCount,attr"`
		Problems    []problem `xml:"problems>problem"`
	}

	var resp evalResult
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		// Fallback: check if response is an error message
		if strings.Contains(xmlStr, "<error") || strings.Contains(xmlStr, "exception") {
			return &RenameEvaluateResult{
				Feasible: false,
				Problems: []RenameProblem{{Severity: "E", Description: string(data)}},
			}, nil
		}
		return nil, fmt.Errorf("parsing rename evaluate result: %w", err)
	}

	result := &RenameEvaluateResult{
		Feasible:    resp.Feasible,
		ChangeCount: resp.ChangeCount,
	}
	for _, p := range resp.Problems {
		result.Problems = append(result.Problems, RenameProblem{
			Severity:    p.Severity,
			Description: p.Description,
			URI:         p.URI,
			Line:        p.Line,
			Column:      p.Column,
		})
	}
	return result, nil
}

// RenamePreview returns all locations that will be changed by the rename.
func (c *Client) RenamePreview(ctx context.Context, objectURI string, line, col int, source, newName string) (*RenamePreviewResult, error) {
	if err := c.checkSafety(OpUpdate, "RenameRefactoring"); err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("method", "preview")
	q.Set("uri", fmt.Sprintf("%s#start=%d,%d", objectURI, line, col))
	q.Set("newName", newName)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/refactoring/rename", &RequestOptions{
		Method:      http.MethodPost,
		Query:       q,
		Body:        []byte(source),
		ContentType: "text/plain",
		Accept:      "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("rename preview failed: %w", err)
	}

	return parseRenamePreviewResult(resp.Body)
}

func parseRenamePreviewResult(data []byte) (*RenamePreviewResult, error) {
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "refactoring:", "")

	type change struct {
		URI     string `xml:"uri,attr"`
		Line    int    `xml:"line,attr"`
		Column  int    `xml:"column,attr"`
		Length  int    `xml:"length,attr"`
		OldText string `xml:"oldText,attr"`
		NewText string `xml:"newText,attr"`
	}
	type previewResult struct {
		XMLName xml.Name `xml:"previewResult"`
		Changes []change `xml:"edits>change"`
	}

	var resp previewResult
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing rename preview result: %w", err)
	}

	result := &RenamePreviewResult{}
	for _, ch := range resp.Changes {
		result.Changes = append(result.Changes, RenameChange{
			URI:     ch.URI,
			Line:    ch.Line,
			Column:  ch.Column,
			Length:  ch.Length,
			OldText: ch.OldText,
			NewText: ch.NewText,
		})
	}
	return result, nil
}

// RenameExecute applies the rename refactoring.
func (c *Client) RenameExecute(ctx context.Context, objectURI string, line, col int, source, newName string) (*RenameExecuteResult, error) {
	if err := c.checkSafety(OpUpdate, "RenameRefactoring"); err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("method", "execute")
	q.Set("uri", fmt.Sprintf("%s#start=%d,%d", objectURI, line, col))
	q.Set("newName", newName)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/refactoring/rename", &RequestOptions{
		Method:      http.MethodPost,
		Query:       q,
		Body:        []byte(source),
		ContentType: "text/plain",
		Accept:      "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("rename execute failed: %w", err)
	}

	return parseRenameExecuteResult(resp.Body)
}

func parseRenameExecuteResult(data []byte) (*RenameExecuteResult, error) {
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "refactoring:", "")

	type affectedObject struct {
		URI string `xml:"uri,attr"`
	}
	type executeResult struct {
		XMLName  xml.Name         `xml:"executeResult"`
		Success  bool             `xml:"success,attr"`
		Message  string           `xml:"message,attr"`
		Affected []affectedObject `xml:"affectedObjects>object"`
	}

	var resp executeResult
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		// SAP ADT may return non-XML "OK" on success. Include raw response for debugging.
		return &RenameExecuteResult{
			Success: true,
			Message: fmt.Sprintf("Rename executed (non-XML response: %s)", truncate(string(data), 200)),
		}, nil
	}

	result := &RenameExecuteResult{
		Success: resp.Success,
		Message: resp.Message,
	}
	for _, obj := range resp.Affected {
		result.AffectedObjects = append(result.AffectedObjects, obj.URI)
	}
	return result, nil
}

// --- Refactoring: Extract Method ---

// ExtractMethodEvaluateResult is the result of extract method feasibility check.
type ExtractMethodEvaluateResult struct {
	Feasible   bool              `json:"feasible"`
	Parameters []ExtractedParam  `json:"parameters,omitempty"`
	ReturnType string            `json:"returnType,omitempty"`
	Problems   []RenameProblem   `json:"problems,omitempty"`
}

// ExtractedParam represents a parameter inferred for the extracted method.
type ExtractedParam struct {
	Name      string `json:"name" xml:"name,attr"`
	Type      string `json:"type" xml:"type,attr"`
	Direction string `json:"direction" xml:"direction,attr"` // IMPORTING, EXPORTING, CHANGING
}

// ExtractMethodPreviewResult is the preview of the extract method refactoring.
type ExtractMethodPreviewResult struct {
	NewMethodSource   string `json:"newMethodSource"`
	ModifiedSource    string `json:"modifiedSource"`
	CallSite          string `json:"callSite"`
}

// ExtractMethodExecuteResult is the result of extract method execution.
type ExtractMethodExecuteResult struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// ExtractMethodEvaluate checks if a code range can be extracted into a new method.
// startLine, startCol, endLine, endCol define the selection range (1-based).
// TODO: Verify range query parameter format against real ADT system (Eclipse network trace).
func (c *Client) ExtractMethodEvaluate(ctx context.Context, objectURI string, startLine, startCol, endLine, endCol int, source, methodName string) (*ExtractMethodEvaluateResult, error) {
	if err := c.checkSafety(OpUpdate, "ExtractMethod"); err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("method", "evaluate")
	q.Set("uri", objectURI)
	q.Set("newMethodName", methodName)
	q.Set("range", fmt.Sprintf("%d,%d;%d,%d", startLine, startCol, endLine, endCol))

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/refactoring/extractmethod", &RequestOptions{
		Method:      http.MethodPost,
		Query:       q,
		Body:        []byte(source),
		ContentType: "text/plain",
		Accept:      "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("extract method evaluate failed: %w", err)
	}

	return parseExtractMethodEvaluateResult(resp.Body)
}

func parseExtractMethodEvaluateResult(data []byte) (*ExtractMethodEvaluateResult, error) {
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "refactoring:", "")

	type param struct {
		Name      string `xml:"name,attr"`
		Type      string `xml:"type,attr"`
		Direction string `xml:"direction,attr"`
	}
	type problem struct {
		Severity    string `xml:"severity,attr"`
		Description string `xml:"description,attr"`
	}
	type evalResult struct {
		XMLName    xml.Name  `xml:"evaluateResult"`
		Feasible   bool      `xml:"feasible,attr"`
		ReturnType string    `xml:"returnType,attr"`
		Parameters []param   `xml:"parameters>parameter"`
		Problems   []problem `xml:"problems>problem"`
	}

	var resp evalResult
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		if strings.Contains(xmlStr, "<error") || strings.Contains(xmlStr, "exception") {
			return &ExtractMethodEvaluateResult{
				Feasible: false,
				Problems: []RenameProblem{{Severity: "E", Description: string(data)}},
			}, nil
		}
		return nil, fmt.Errorf("parsing extract method evaluate result: %w", err)
	}

	result := &ExtractMethodEvaluateResult{
		Feasible:   resp.Feasible,
		ReturnType: resp.ReturnType,
	}
	for _, p := range resp.Parameters {
		result.Parameters = append(result.Parameters, ExtractedParam{
			Name:      p.Name,
			Type:      p.Type,
			Direction: p.Direction,
		})
	}
	for _, p := range resp.Problems {
		result.Problems = append(result.Problems, RenameProblem{
			Severity:    p.Severity,
			Description: p.Description,
		})
	}
	return result, nil
}

// ExtractMethodPreview returns the preview of how the code will look after extraction.
func (c *Client) ExtractMethodPreview(ctx context.Context, objectURI string, startLine, startCol, endLine, endCol int, source, methodName string) (*ExtractMethodPreviewResult, error) {
	if err := c.checkSafety(OpUpdate, "ExtractMethod"); err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("method", "preview")
	q.Set("uri", objectURI)
	q.Set("newMethodName", methodName)
	q.Set("range", fmt.Sprintf("%d,%d;%d,%d", startLine, startCol, endLine, endCol))

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/refactoring/extractmethod", &RequestOptions{
		Method:      http.MethodPost,
		Query:       q,
		Body:        []byte(source),
		ContentType: "text/plain",
		Accept:      "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("extract method preview failed: %w", err)
	}

	return parseExtractMethodPreviewResult(resp.Body)
}

func parseExtractMethodPreviewResult(data []byte) (*ExtractMethodPreviewResult, error) {
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "refactoring:", "")

	type previewResult struct {
		XMLName         xml.Name `xml:"previewResult"`
		NewMethodSource string   `xml:"newMethodSource"`
		ModifiedSource  string   `xml:"modifiedSource"`
		CallSite        string   `xml:"callSite"`
	}

	var resp previewResult
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing extract method preview result: %w", err)
	}

	return &ExtractMethodPreviewResult{
		NewMethodSource: resp.NewMethodSource,
		ModifiedSource:  resp.ModifiedSource,
		CallSite:        resp.CallSite,
	}, nil
}

// ExtractMethodExecute applies the extract method refactoring.
func (c *Client) ExtractMethodExecute(ctx context.Context, objectURI string, startLine, startCol, endLine, endCol int, source, methodName string) (*ExtractMethodExecuteResult, error) {
	if err := c.checkSafety(OpUpdate, "ExtractMethod"); err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("method", "execute")
	q.Set("uri", objectURI)
	q.Set("newMethodName", methodName)
	q.Set("range", fmt.Sprintf("%d,%d;%d,%d", startLine, startCol, endLine, endCol))

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/refactoring/extractmethod", &RequestOptions{
		Method:      http.MethodPost,
		Query:       q,
		Body:        []byte(source),
		ContentType: "text/plain",
		Accept:      "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("extract method execute failed: %w", err)
	}

	return parseExtractMethodExecuteResult(resp.Body)
}

func parseExtractMethodExecuteResult(data []byte) (*ExtractMethodExecuteResult, error) {
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "refactoring:", "")

	type executeResult struct {
		XMLName xml.Name `xml:"executeResult"`
		Success bool     `xml:"success,attr"`
		Message string   `xml:"message,attr"`
	}

	var resp executeResult
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		// SAP ADT may return non-XML "OK" on success. Include raw response for debugging.
		return &ExtractMethodExecuteResult{
			Success: true,
			Message: fmt.Sprintf("Extract method executed (non-XML response: %s)", truncate(string(data), 200)),
		}, nil
	}

	return &ExtractMethodExecuteResult{
		Success: resp.Success,
		Message: resp.Message,
	}, nil
}

// truncate returns s trimmed to maxLen characters with "..." appended if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
