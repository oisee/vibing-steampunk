// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_report.go contains handlers for report execution and text elements.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Report Execution Handlers ---

func (s *Server) handleRunReport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Ensure WebSocket is connected
	if errResult := s.ensureWSConnected(ctx, "RunReport"); errResult != nil {
		return errResult, nil
	}

	// Parse parameters
	report, _ := request.GetArguments()["report"].(string)
	if report == "" {
		return newToolResultError("report parameter is required"), nil
	}

	params := adt.RunReportParams{
		Report: report,
	}

	if variant, ok := request.GetArguments()["variant"].(string); ok {
		params.Variant = variant
	}

	if paramsStr, ok := request.GetArguments()["params"].(string); ok && paramsStr != "" {
		var p map[string]string
		if err := json.Unmarshal([]byte(paramsStr), &p); err != nil {
			return newToolResultError(fmt.Sprintf("Invalid params JSON: %v", err)), nil
		}
		params.Params = p
	}

	// Step 1: Schedule background job via WebSocket
	result, err := s.amdpWSClient.RunReport(ctx, params)
	if err != nil {
		return newToolResultError(fmt.Sprintf("RunReport failed: %v", err)), nil
	}

	// Check if we got job info (new job-based approach)
	if result.JobName == "" || result.JobCount == "" {
		return newToolResultError("RunReport did not return job info - ABAP service may need updating"), nil
	}

	// Step 2: Poll for job completion (max 60 seconds)
	var jobStatus *adt.JobStatusResult
	pollCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	for {
		jobStatus, err = s.amdpWSClient.GetJobStatus(pollCtx, result.JobName, result.JobCount)
		if err != nil {
			return newToolResultError(fmt.Sprintf("GetJobStatus failed: %v", err)), nil
		}

		if jobStatus.Status == "finished" || jobStatus.Status == "aborted" {
			break
		}

		select {
		case <-pollCtx.Done():
			return newToolResultError(fmt.Sprintf("Job %s/%s timed out (status: %s)", result.JobName, result.JobCount, jobStatus.Status)), nil
		case <-time.After(500 * time.Millisecond):
			// Continue polling
		}
	}

	// Step 3: Format output
	var sb strings.Builder
	fmt.Fprintf(&sb, "Report: %s\n", result.Report)
	fmt.Fprintf(&sb, "Job: %s/%s\n", result.JobName, result.JobCount)
	fmt.Fprintf(&sb, "Status: %s\n\n", jobStatus.Status)

	// Step 4: Get spool output if available
	if len(jobStatus.SpoolIDs) > 0 {
		sb.WriteString("Spool Output:\n")
		for _, spoolID := range jobStatus.SpoolIDs {
			spoolResult, err := s.amdpWSClient.GetSpoolOutput(ctx, spoolID)
			if err != nil {
				fmt.Fprintf(&sb, "  [Spool %s: error reading - %v]\n", spoolID, err)
				continue
			}
			fmt.Fprintf(&sb, "--- Spool %s (%d lines) ---\n", spoolID, spoolResult.Lines)
			sb.WriteString(spoolResult.Output)
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("No spool output produced.\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleRunReportAsync(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Ensure WebSocket is connected
	if errResult := s.ensureWSConnected(ctx, "RunReportAsync"); errResult != nil {
		return errResult, nil
	}

	// Parse parameters
	report, _ := request.GetArguments()["report"].(string)
	if report == "" {
		return newToolResultError("report parameter is required"), nil
	}

	params := adt.RunReportParams{
		Report: report,
	}

	if variant, ok := request.GetArguments()["variant"].(string); ok {
		params.Variant = variant
	}

	if paramsStr, ok := request.GetArguments()["params"].(string); ok && paramsStr != "" {
		var p map[string]string
		if err := json.Unmarshal([]byte(paramsStr), &p); err != nil {
			return newToolResultError(fmt.Sprintf("Invalid params JSON: %v", err)), nil
		}
		params.Params = p
	}

	// Generate task ID
	s.asyncTasksMu.Lock()
	s.asyncTaskID++
	taskID := fmt.Sprintf("report_%d_%d", time.Now().Unix(), s.asyncTaskID)
	task := &AsyncTask{
		ID:        taskID,
		Type:      "report",
		Status:    "running",
		StartedAt: time.Now(),
	}
	s.asyncTasks[taskID] = task
	s.asyncTasksMu.Unlock()

	// Run report in background goroutine via WebSocket (job-based)
	go func() {
		bgCtx := context.Background()

		// Step 1: Schedule background job
		result, err := s.amdpWSClient.RunReport(bgCtx, params)
		if err != nil {
			s.asyncTasksMu.Lock()
			now := time.Now()
			task.EndedAt = &now
			task.Status = "error"
			task.Error = err.Error()
			s.asyncTasksMu.Unlock()
			return
		}

		if result.JobName == "" || result.JobCount == "" {
			s.asyncTasksMu.Lock()
			now := time.Now()
			task.EndedAt = &now
			task.Status = "error"
			task.Error = "RunReport did not return job info"
			s.asyncTasksMu.Unlock()
			return
		}

		// Step 2: Poll for job completion (max 5 minutes for async)
		pollCtx, cancel := context.WithTimeout(bgCtx, 5*time.Minute)
		defer cancel()

		var jobStatus *adt.JobStatusResult
		for {
			jobStatus, err = s.amdpWSClient.GetJobStatus(pollCtx, result.JobName, result.JobCount)
			if err != nil {
				s.asyncTasksMu.Lock()
				now := time.Now()
				task.EndedAt = &now
				task.Status = "error"
				task.Error = fmt.Sprintf("GetJobStatus failed: %v", err)
				s.asyncTasksMu.Unlock()
				return
			}

			if jobStatus.Status == "finished" || jobStatus.Status == "aborted" {
				break
			}

			select {
			case <-pollCtx.Done():
				s.asyncTasksMu.Lock()
				now := time.Now()
				task.EndedAt = &now
				task.Status = "error"
				task.Error = fmt.Sprintf("Job %s/%s timed out", result.JobName, result.JobCount)
				s.asyncTasksMu.Unlock()
				return
			case <-time.After(1 * time.Second):
				// Continue polling
			}
		}

		// Step 3: Collect spool output
		var spoolOutput strings.Builder
		if len(jobStatus.SpoolIDs) > 0 {
			for _, spoolID := range jobStatus.SpoolIDs {
				spoolResult, err := s.amdpWSClient.GetSpoolOutput(bgCtx, spoolID)
				if err != nil {
					fmt.Fprintf(&spoolOutput, "[Spool %s: error - %v]\n", spoolID, err)
					continue
				}
				fmt.Fprintf(&spoolOutput, "--- Spool %s (%d lines) ---\n", spoolID, spoolResult.Lines)
				spoolOutput.WriteString(spoolResult.Output)
				spoolOutput.WriteString("\n")
			}
		}

		// Mark complete
		s.asyncTasksMu.Lock()
		now := time.Now()
		task.EndedAt = &now
		task.Status = "completed"
		task.Result = map[string]interface{}{
			"report":       result.Report,
			"jobname":      result.JobName,
			"jobcount":     result.JobCount,
			"job_status":   jobStatus.Status,
			"spool_ids":    jobStatus.SpoolIDs,
			"spool_output": spoolOutput.String(),
		}
		s.asyncTasksMu.Unlock()
	}()

	// Return task ID immediately
	output := map[string]string{
		"task_id": taskID,
		"status":  "started",
		"message": "Report execution started in background. Use GetAsyncResult to check status.",
	}
	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	return mcp.NewToolResultText(string(outputJSON)), nil
}

func (s *Server) handleGetAsyncResult(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, _ := request.GetArguments()["task_id"].(string)
	if taskID == "" {
		return newToolResultError("task_id parameter is required"), nil
	}

	wait, _ := request.GetArguments()["wait"].(bool)

	if wait {
		// Block until complete or timeout
		timeout := time.After(60 * time.Second)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			s.asyncTasksMu.RLock()
			task, exists := s.asyncTasks[taskID]
			if !exists {
				s.asyncTasksMu.RUnlock()
				return newToolResultError(fmt.Sprintf("Task not found: %s", taskID)), nil
			}
			status := task.Status
			s.asyncTasksMu.RUnlock()

			if status != "running" {
				break
			}

			select {
			case <-timeout:
				return newToolResultError("Timeout waiting for task completion"), nil
			case <-ticker.C:
				continue
			case <-ctx.Done():
				return newToolResultError("Request cancelled"), nil
			}
		}
	}

	// Get task status
	s.asyncTasksMu.RLock()
	task, exists := s.asyncTasks[taskID]
	if !exists {
		s.asyncTasksMu.RUnlock()
		return newToolResultError(fmt.Sprintf("Task not found: %s", taskID)), nil
	}
	// Make a copy for safe access
	taskCopy := *task
	s.asyncTasksMu.RUnlock()

	// Format output
	output, _ := json.MarshalIndent(taskCopy, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleGetVariants(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if errResult := s.ensureWSConnected(ctx, "GetVariants"); errResult != nil {
		return errResult, nil
	}

	report, _ := request.GetArguments()["report"].(string)
	if report == "" {
		return newToolResultError("report parameter is required"), nil
	}

	result, err := s.amdpWSClient.GetVariants(ctx, report)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetVariants failed: %v", err)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Variants for %s:\n\n", result.Report)

	if len(result.Variants) == 0 {
		sb.WriteString("No variants found.\n")
	} else {
		for _, v := range result.Variants {
			if v.Protected {
				fmt.Fprintf(&sb, "  %s (protected)\n", v.Name)
			} else {
				fmt.Fprintf(&sb, "  %s\n", v.Name)
			}
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleGetTextElements(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if errResult := s.ensureWSConnected(ctx, "GetTextElements"); errResult != nil {
		return errResult, nil
	}

	program, _ := request.GetArguments()["program"].(string)
	if program == "" {
		return newToolResultError("program parameter is required"), nil
	}

	language, _ := request.GetArguments()["language"].(string)

	result, err := s.amdpWSClient.GetTextElements(ctx, program, language)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetTextElements failed: %v", err)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Text Elements for %s (Language: %s)\n\n", result.Program, result.Language)

	sb.WriteString("Selection Texts:\n")
	if len(result.SelectionTexts) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for key, text := range result.SelectionTexts {
			fmt.Fprintf(&sb, "  %s: %s\n", key, text)
		}
	}
	sb.WriteString("\n")

	sb.WriteString("Text Symbols:\n")
	if len(result.TextSymbols) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for key, text := range result.TextSymbols {
			fmt.Fprintf(&sb, "  TEXT-%s: %s\n", key, text)
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleSetTextElements(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if errResult := s.ensureWSConnected(ctx, "SetTextElements"); errResult != nil {
		return errResult, nil
	}

	program, _ := request.GetArguments()["program"].(string)
	if program == "" {
		return newToolResultError("program parameter is required"), nil
	}

	params := adt.SetTextElementsParams{
		Program: program,
	}

	if language, ok := request.GetArguments()["language"].(string); ok {
		params.Language = language
	}

	if selTextsStr, ok := request.GetArguments()["selection_texts"].(string); ok && selTextsStr != "" {
		var selTexts map[string]string
		if err := json.Unmarshal([]byte(selTextsStr), &selTexts); err != nil {
			return newToolResultError(fmt.Sprintf("Invalid selection_texts JSON: %v", err)), nil
		}
		params.SelectionTexts = selTexts
	}

	if textSymsStr, ok := request.GetArguments()["text_symbols"].(string); ok && textSymsStr != "" {
		var textSyms map[string]string
		if err := json.Unmarshal([]byte(textSymsStr), &textSyms); err != nil {
			return newToolResultError(fmt.Sprintf("Invalid text_symbols JSON: %v", err)), nil
		}
		params.TextSymbols = textSyms
	}

	if headTextsStr, ok := request.GetArguments()["heading_texts"].(string); ok && headTextsStr != "" {
		var headTexts map[string]string
		if err := json.Unmarshal([]byte(headTextsStr), &headTexts); err != nil {
			return newToolResultError(fmt.Sprintf("Invalid heading_texts JSON: %v", err)), nil
		}
		params.HeadingTexts = headTexts
	}

	if params.SelectionTexts == nil && params.TextSymbols == nil && params.HeadingTexts == nil {
		return newToolResultError("At least one of selection_texts, text_symbols, or heading_texts is required"), nil
	}

	result, err := s.amdpWSClient.SetTextElements(ctx, params)
	if err != nil {
		return newToolResultError(fmt.Sprintf("SetTextElements failed: %v", err)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Text Elements Updated for %s (Language: %s)\n\n", result.Program, result.Language)
	fmt.Fprintf(&sb, "Status: %s\n", result.Status)
	fmt.Fprintf(&sb, "Selection Texts Set: %d\n", result.SelectionTextsSet)
	fmt.Fprintf(&sb, "Text Symbols Set: %d\n", result.TextSymbolsSet)
	fmt.Fprintf(&sb, "Heading Texts Set: %d\n", result.HeadingTextsSet)

	return mcp.NewToolResultText(sb.String()), nil
}
