package adt

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

// RunReportParams contains parameters for report execution.
// Reports run as background jobs and output to spool (APC-safe).
type RunReportParams struct {
	Report  string            `json:"report"`
	Variant string            `json:"variant,omitempty"`
	Params  map[string]string `json:"params,omitempty"`
}

// RunReportResult contains report execution results.
// Reports run as background jobs - use GetJobStatus to poll completion.
type RunReportResult struct {
	Status   string `json:"status"`
	Report   string `json:"report"`
	JobName  string `json:"jobname"`
	JobCount string `json:"jobcount"`
}

// JobStatusResult contains job status information.
type JobStatusResult struct {
	JobName  string   `json:"jobname"`
	JobCount string   `json:"jobcount"`
	Status   string   `json:"status"` // scheduled, running, finished, aborted
	SpoolIDs []string `json:"spool_ids,omitempty"`
}

// SpoolOutputResult contains spool output data.
type SpoolOutputResult struct {
	SpoolID string `json:"spool_id"`
	Lines   int    `json:"lines"`
	Output  string `json:"output"`
}

// TextElements contains program text elements.
type TextElements struct {
	Program        string            `json:"program"`
	Language       string            `json:"language"`
	SelectionTexts map[string]string `json:"selection_texts"`
	TextSymbols    map[string]string `json:"text_symbols"`
	HeadingTexts   map[string]string `json:"heading_texts"`
}

// SetTextElementsParams contains parameters for setting text elements.
type SetTextElementsParams struct {
	Program        string            `json:"program"`
	Language       string            `json:"language,omitempty"`
	SelectionTexts map[string]string `json:"selection_texts,omitempty"`
	TextSymbols    map[string]string `json:"text_symbols,omitempty"`
	HeadingTexts   map[string]string `json:"heading_texts,omitempty"`
}

// SetTextElementsResult contains result of setting text elements.
type SetTextElementsResult struct {
	Status            string `json:"status"`
	Program           string `json:"program"`
	Language          string `json:"language"`
	SelectionTextsSet int    `json:"selection_texts_set"`
	TextSymbolsSet    int    `json:"text_symbols_set"`
	HeadingTextsSet   int    `json:"heading_texts_set"`
}

// ReportVariant describes a report variant.
type ReportVariant struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
}

// GetVariantsResult contains list of variants.
type GetVariantsResult struct {
	Report   string          `json:"report"`
	Variants []ReportVariant `json:"variants"`
}

// RunReport executes an ABAP report via WebSocket (ZADT_VSP report domain).
// Report runs as background job - returns job info for polling.
func (c *AMDPWebSocketClient) RunReport(ctx context.Context, params RunReportParams) (*RunReportResult, error) {
	reqParams := map[string]any{
		"report": params.Report,
	}
	if params.Variant != "" {
		reqParams["variant"] = params.Variant
	}
	if len(params.Params) > 0 {
		reqParams["params"] = params.Params
	}

	resp, err := c.sendReportRequest(ctx, "runReport", reqParams)
	if err != nil {
		return nil, err
	}

	var result RunReportResult
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &result); err != nil {
			return nil, fmt.Errorf("failed to parse result: %w", err)
		}
	}

	return &result, nil
}

// GetTextElements retrieves program text elements via WebSocket.
func (c *AMDPWebSocketClient) GetTextElements(ctx context.Context, program, language string) (*TextElements, error) {
	params := map[string]any{
		"program": program,
	}
	if language != "" {
		params["language"] = language
	}

	resp, err := c.sendReportRequest(ctx, "getTextElements", params)
	if err != nil {
		return nil, err
	}

	var result TextElements
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &result); err != nil {
			return nil, fmt.Errorf("failed to parse result: %w", err)
		}
	}

	return &result, nil
}

// SetTextElements updates program text elements via WebSocket.
func (c *AMDPWebSocketClient) SetTextElements(ctx context.Context, params SetTextElementsParams) (*SetTextElementsResult, error) {
	reqParams := map[string]any{
		"program": params.Program,
	}
	if params.Language != "" {
		reqParams["language"] = params.Language
	}
	if params.SelectionTexts != nil {
		reqParams["selection_texts"] = params.SelectionTexts
	}
	if params.TextSymbols != nil {
		reqParams["text_symbols"] = params.TextSymbols
	}
	if params.HeadingTexts != nil {
		reqParams["heading_texts"] = params.HeadingTexts
	}

	resp, err := c.sendReportRequest(ctx, "setTextElements", reqParams)
	if err != nil {
		return nil, err
	}

	var result SetTextElementsResult
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &result); err != nil {
			return nil, fmt.Errorf("failed to parse result: %w", err)
		}
	}

	return &result, nil
}

// GetVariants retrieves available variants for a report via WebSocket.
func (c *AMDPWebSocketClient) GetVariants(ctx context.Context, report string) (*GetVariantsResult, error) {
	params := map[string]any{
		"report": report,
	}

	resp, err := c.sendReportRequest(ctx, "getVariants", params)
	if err != nil {
		return nil, err
	}

	var result GetVariantsResult
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &result); err != nil {
			return nil, fmt.Errorf("failed to parse result: %w", err)
		}
	}

	return &result, nil
}

// GetJobStatus retrieves job status via WebSocket.
func (c *AMDPWebSocketClient) GetJobStatus(ctx context.Context, jobName, jobCount string) (*JobStatusResult, error) {
	params := map[string]any{
		"jobname":  jobName,
		"jobcount": jobCount,
	}

	resp, err := c.sendReportRequest(ctx, "getJobStatus", params)
	if err != nil {
		return nil, err
	}

	var result JobStatusResult
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &result); err != nil {
			return nil, fmt.Errorf("failed to parse result: %w", err)
		}
	}

	return &result, nil
}

// GetSpoolOutput retrieves spool output by ID via WebSocket.
func (c *AMDPWebSocketClient) GetSpoolOutput(ctx context.Context, spoolID string) (*SpoolOutputResult, error) {
	params := map[string]any{
		"spool_id": spoolID,
	}

	resp, err := c.sendReportRequest(ctx, "getSpoolOutput", params)
	if err != nil {
		return nil, err
	}

	var result SpoolOutputResult
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &result); err != nil {
			return nil, fmt.Errorf("failed to parse result: %w", err)
		}
	}

	return &result, nil
}

// sendReportRequest sends a request to the report domain.
func (c *AMDPWebSocketClient) sendReportRequest(ctx context.Context, action string, params map[string]any) (*WSResponse, error) {
	c.mu.RLock()
	if c.conn == nil {
		c.mu.RUnlock()
		return nil, fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	id := fmt.Sprintf("report_%d", c.msgID.Add(1))

	msg := WSMessage{
		ID:      id,
		Domain:  "report",
		Action:  action,
		Params:  params,
		Timeout: 120000, // 2 minute timeout for report execution
	}

	// Create response channel
	respCh := make(chan *WSResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	// Send message
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	c.mu.Lock()
	err = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err == nil {
		err = c.conn.WriteMessage(websocket.TextMessage, data)
	}
	c.mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Wait for response
	select {
	case resp := <-respCh:
		if !resp.Success && resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(120 * time.Second):
		return nil, fmt.Errorf("timeout waiting for report response")
	}
}
