package adt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

// GitTypes returns the list of supported abapGit object types.
func (c *AMDPWebSocketClient) GitTypes(ctx context.Context) ([]string, error) {
	resp, err := c.sendGitRequest(ctx, "getTypes", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Count int      `json:"count"`
		Types []string `json:"types"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse types: %w", err)
	}

	return result.Types, nil
}

// GitExportParams contains parameters for git export.
type GitExportParams struct {
	Packages           []string       `json:"packages,omitempty"`
	Objects            []GitObjectRef `json:"objects,omitempty"`
	IncludeSubpackages bool           `json:"includeSubpackages,omitempty"`
}

// GitObjectRef identifies a single ABAP object.
type GitObjectRef struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// GitExportResult contains the export result.
type GitExportResult struct {
	ObjectCount int           `json:"objectCount"`
	FileCount   int           `json:"fileCount"`
	ZipBase64   string        `json:"zipBase64"`
	Files       []GitFileInfo `json:"files"`
}

// GitFileInfo describes a file in the export.
type GitFileInfo struct {
	Path string `json:"path"`
	Size int    `json:"size"`
}

// GitExport exports ABAP objects as abapGit-compatible ZIP.
func (c *AMDPWebSocketClient) GitExport(ctx context.Context, params GitExportParams) (*GitExportResult, error) {
	// Build params map
	p := make(map[string]any)
	if len(params.Packages) > 0 {
		p["packages"] = params.Packages
	}
	if len(params.Objects) > 0 {
		// Convert objects to expected format
		var objs []map[string]string
		for _, obj := range params.Objects {
			objs = append(objs, map[string]string{
				"type": obj.Type,
				"name": obj.Name,
			})
		}
		p["objects"] = objs
	}
	p["includeSubpackages"] = params.IncludeSubpackages

	resp, err := c.sendGitRequest(ctx, "export", p)
	if err != nil {
		return nil, err
	}

	var result GitExportResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse export result: %w", err)
	}

	return &result, nil
}

// GitExportToBytes exports and decodes the ZIP to bytes.
func (c *AMDPWebSocketClient) GitExportToBytes(ctx context.Context, params GitExportParams) ([]byte, *GitExportResult, error) {
	result, err := c.GitExport(ctx, params)
	if err != nil {
		return nil, nil, err
	}

	zipData, err := base64.StdEncoding.DecodeString(result.ZipBase64)
	if err != nil {
		return nil, result, fmt.Errorf("failed to decode ZIP: %w", err)
	}

	return zipData, result, nil
}

// sendGitRequest sends a request to the git domain.
func (c *AMDPWebSocketClient) sendGitRequest(ctx context.Context, action string, params map[string]any) (*WSResponse, error) {
	c.mu.RLock()
	if c.conn == nil {
		c.mu.RUnlock()
		return nil, fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	id := fmt.Sprintf("git_%d", c.msgID.Add(1))

	msg := WSMessage{
		ID:      id,
		Domain:  "git",
		Action:  action,
		Params:  params,
		Timeout: 120000, // 2 minute timeout for git operations
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
		return nil, fmt.Errorf("timeout waiting for git response")
	}
}
