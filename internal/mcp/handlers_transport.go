// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_transport.go contains handlers for transport management operations.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Transport Management Handlers ---

func (s *Server) handleGetUserTransports(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userName, ok := request.GetArguments()["user_name"].(string)
	if !ok || userName == "" {
		return newToolResultError("user_name is required"), nil
	}

	transports, err := s.adtClient.GetUserTransports(ctx, userName)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetUserTransports failed: %v", err)), nil
	}

	// Format output
	var sb strings.Builder
	fmt.Fprintf(&sb, "Transports for user %s:\n\n", strings.ToUpper(userName))

	if len(transports.Workbench) > 0 {
		sb.WriteString("=== Workbench Requests ===\n")
		for _, tr := range transports.Workbench {
			formatTransportRequest(&sb, &tr)
		}
	} else {
		sb.WriteString("No workbench requests found.\n")
	}

	sb.WriteString("\n")

	if len(transports.Customizing) > 0 {
		sb.WriteString("=== Customizing Requests ===\n")
		for _, tr := range transports.Customizing {
			formatTransportRequest(&sb, &tr)
		}
	} else {
		sb.WriteString("No customizing requests found.\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func formatTransportRequest(sb *strings.Builder, tr *adt.TransportRequest) {
	fmt.Fprintf(sb, "\n%s - %s\n", tr.Number, tr.Description)
	fmt.Fprintf(sb, "  Owner: %s, Status: %s", tr.Owner, tr.Status)
	if tr.Target != "" {
		fmt.Fprintf(sb, ", Target: %s", tr.Target)
	}
	sb.WriteString("\n")

	if len(tr.Tasks) > 0 {
		sb.WriteString("  Tasks:\n")
		for _, task := range tr.Tasks {
			fmt.Fprintf(sb, "    %s - %s (Owner: %s, Status: %s)\n",
				task.Number, task.Description, task.Owner, task.Status)
			if len(task.Objects) > 0 {
				for _, obj := range task.Objects {
					fmt.Fprintf(sb, "      - %s %s %s\n", obj.PGMID, obj.Type, obj.Name)
				}
			}
		}
	}
}

func (s *Server) handleGetTransportInfo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.GetArguments()["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	devClass, ok := request.GetArguments()["dev_class"].(string)
	if !ok || devClass == "" {
		return newToolResultError("dev_class is required"), nil
	}

	info, err := s.adtClient.GetTransportInfo(ctx, objectURL, devClass)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetTransportInfo failed: %v", err)), nil
	}

	// Format output
	var sb strings.Builder
	sb.WriteString("Transport Information:\n\n")
	fmt.Fprintf(&sb, "PGMID: %s\n", info.PGMID)
	fmt.Fprintf(&sb, "Object: %s\n", info.Object)
	fmt.Fprintf(&sb, "Object Name: %s\n", info.ObjectName)
	fmt.Fprintf(&sb, "Operation: %s\n", info.Operation)
	fmt.Fprintf(&sb, "Dev Class: %s\n", info.DevClass)
	fmt.Fprintf(&sb, "Recording: %s\n", info.Recording)

	if info.LockedByUser != "" {
		fmt.Fprintf(&sb, "\nLocked by: %s", info.LockedByUser)
		if info.LockedInTask != "" {
			fmt.Fprintf(&sb, " in task %s", info.LockedInTask)
		}
		sb.WriteString("\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// handleExecuteABAP executes arbitrary ABAP code via unit test wrapper.
func (s *Server) handleExecuteABAP(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	code, ok := request.GetArguments()["code"].(string)
	if !ok || code == "" {
		return newToolResultError("code parameter is required"), nil
	}

	opts := &adt.ExecuteABAPOptions{}

	if riskLevel, ok := request.GetArguments()["risk_level"].(string); ok && riskLevel != "" {
		opts.RiskLevel = riskLevel
	}

	if returnVar, ok := request.GetArguments()["return_variable"].(string); ok && returnVar != "" {
		opts.ReturnVariable = returnVar
	}

	if keepProgram, ok := request.GetArguments()["keep_program"].(bool); ok {
		opts.KeepProgram = keepProgram
	}

	if prefix, ok := request.GetArguments()["program_prefix"].(string); ok && prefix != "" {
		opts.ProgramPrefix = prefix
	}

	result, err := s.adtClient.ExecuteABAP(ctx, code, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("ExecuteABAP failed: %v", err)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Program: %s\n", result.ProgramName)
	fmt.Fprintf(&sb, "Success: %t\n", result.Success)
	fmt.Fprintf(&sb, "Execution Time: %.3f s\n", result.ExecutionTime)
	fmt.Fprintf(&sb, "Cleaned Up: %t\n", result.CleanedUp)
	fmt.Fprintf(&sb, "Message: %s\n", result.Message)

	if len(result.Output) > 0 {
		sb.WriteString("\nOutput:\n")
		for i, output := range result.Output {
			fmt.Fprintf(&sb, "  [%d] %s\n", i+1, output)
		}
	}

	// Include raw alerts for debugging if no clean output was captured
	if len(result.Output) == 0 && len(result.RawAlerts) > 0 {
		sb.WriteString("\nRaw Alerts (for debugging):\n")
		for _, alert := range result.RawAlerts {
			fmt.Fprintf(&sb, "  Kind: %s, Severity: %s\n", alert.Kind, alert.Severity)
			fmt.Fprintf(&sb, "  Title: %s\n", alert.Title)
			if len(alert.Details) > 0 {
				sb.WriteString("  Details:\n")
				for _, d := range alert.Details {
					fmt.Fprintf(&sb, "    - %s\n", d)
				}
			}
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleListTransports(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check safety config for transport operations
	if err := s.adtClient.Safety().CheckTransport("", "ListTransports", false); err != nil {
		return newToolResultError(err.Error()), nil
	}

	user, _ := request.GetArguments()["user"].(string)

	transports, err := s.adtClient.ListTransports(ctx, user)
	if err != nil {
		return newToolResultError(fmt.Sprintf("ListTransports failed: %v", err)), nil
	}

	if len(transports) == 0 {
		return mcp.NewToolResultText("No modifiable transports found."), nil
	}

	jsonBytes, err := json.MarshalIndent(transports, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to format result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

func (s *Server) handleGetTransport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	transport, ok := request.GetArguments()["transport"].(string)
	if !ok || transport == "" {
		return newToolResultError("transport is required"), nil
	}

	// Check safety config for transport operations
	if err := s.adtClient.Safety().CheckTransport(transport, "GetTransport", false); err != nil {
		return newToolResultError(err.Error()), nil
	}

	result, err := s.adtClient.GetTransport(ctx, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetTransport failed: %v", err)), nil
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to format result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

func (s *Server) handleCreateTransport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check safety config for transport write operations
	if err := s.adtClient.Safety().CheckTransport("", "CreateTransport", true); err != nil {
		return newToolResultError(err.Error()), nil
	}

	description, ok := request.GetArguments()["description"].(string)
	if !ok || description == "" {
		return newToolResultError("description is required"), nil
	}

	pkg, ok := request.GetArguments()["package"].(string)
	if !ok || pkg == "" {
		return newToolResultError("package is required"), nil
	}

	transportLayer, _ := request.GetArguments()["transport_layer"].(string)
	transportType, _ := request.GetArguments()["type"].(string)

	opts := adt.CreateTransportOptions{
		Description:    description,
		Package:        pkg,
		TransportLayer: transportLayer,
		Type:           transportType,
	}

	transportNumber, err := s.adtClient.CreateTransportV2(ctx, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("CreateTransport failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Transport created: %s", transportNumber)), nil
}

func (s *Server) handleReleaseTransport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	transport, ok := request.GetArguments()["transport"].(string)
	if !ok || transport == "" {
		return newToolResultError("transport is required"), nil
	}

	// Check safety config for transport write operations
	if err := s.adtClient.Safety().CheckTransport(transport, "ReleaseTransport", true); err != nil {
		return newToolResultError(err.Error()), nil
	}

	ignoreLocks, _ := request.GetArguments()["ignore_locks"].(bool)
	skipATC, _ := request.GetArguments()["skip_atc"].(bool)

	opts := adt.ReleaseTransportOptions{
		IgnoreLocks: ignoreLocks,
		SkipATC:     skipATC,
	}

	err := s.adtClient.ReleaseTransportV2(ctx, transport, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("ReleaseTransport failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Transport %s released successfully.", transport)), nil
}

func (s *Server) handleDeleteTransport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	transport, ok := request.GetArguments()["transport"].(string)
	if !ok || transport == "" {
		return newToolResultError("transport is required"), nil
	}

	// Check safety config for transport write operations
	if err := s.adtClient.Safety().CheckTransport(transport, "DeleteTransport", true); err != nil {
		return newToolResultError(err.Error()), nil
	}

	err := s.adtClient.DeleteTransport(ctx, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("DeleteTransport failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Transport %s deleted successfully.", transport)), nil
}
