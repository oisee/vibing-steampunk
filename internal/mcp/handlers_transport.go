// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_transport.go contains handlers for transport management operations.
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Transport Routing ---
// Routes for this module:
//   read:   TRANSPORTS <user>, TRANSPORT <number>, TRANSPORT_INFO
//   system: list_transports, execute_abap
//   create: TRANSPORT
//   delete: TRANSPORT <number>
//   deploy: release_transport

// routeTransportAction routes transport management actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeTransportAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	switch action {
	case "read":
		switch objectType {
		case "TRANSPORTS":
			// GetUserTransports: read TRANSPORTS <user>
			if objectName == "" {
				return newToolResultError("user name is required for TRANSPORTS read"), true, nil
			}
			result, err := s.handleGetUserTransports(ctx, newRequest(map[string]any{"user_name": objectName}))
			return result, true, err

		case "TRANSPORT":
			// GetTransport: read TRANSPORT <number>
			if objectName == "" {
				return newToolResultError("transport number is required"), true, nil
			}
			result, err := s.handleGetTransport(ctx, newRequest(map[string]any{"transport": objectName}))
			return result, true, err

		case "TRANSPORT_INFO":
			// GetTransportInfo: read TRANSPORT_INFO (requires object_url and dev_class in params)
			objectURL, _ := params["object_url"].(string)
			devClass, _ := params["dev_class"].(string)
			if objectURL == "" || devClass == "" {
				return newToolResultError("object_url and dev_class are required in params for TRANSPORT_INFO"), true, nil
			}
			result, err := s.handleGetTransportInfo(ctx, newRequest(map[string]any{"object_url": objectURL, "dev_class": devClass}))
			return result, true, err
		}

	case "system":
		systemType, _ := params["type"].(string)
		switch systemType {
		case "list_transports":
			user, _ := params["user"].(string)
			result, err := s.handleListTransports(ctx, newRequest(map[string]any{"user": user}))
			return result, true, err

		case "execute_abap":
			code, _ := params["code"].(string)
			if code == "" {
				return newToolResultError("code is required for execute_abap"), true, nil
			}
			args := map[string]any{"code": code}
			if riskLevel, ok := params["risk_level"].(string); ok {
				args["risk_level"] = riskLevel
			}
			if returnVar, ok := params["return_variable"].(string); ok {
				args["return_variable"] = returnVar
			}
			if keepProgram, ok := params["keep_program"].(bool); ok {
				args["keep_program"] = keepProgram
			}
			if prefix, ok := params["program_prefix"].(string); ok {
				args["program_prefix"] = prefix
			}
			result, err := s.handleExecuteABAP(ctx, newRequest(args))
			return result, true, err
		}

	case "create":
		if objectType == "TRANSPORT" {
			description, _ := params["description"].(string)
			pkg, _ := params["package"].(string)
			if description == "" || pkg == "" {
				return newToolResultError("description and package are required for TRANSPORT create"), true, nil
			}
			args := map[string]any{"description": description, "package": pkg}
			if transportLayer, ok := params["transport_layer"].(string); ok {
				args["transport_layer"] = transportLayer
			}
			if transportType, ok := params["type"].(string); ok && transportType != "" {
				args["type"] = transportType
			}
			result, err := s.handleCreateTransport(ctx, newRequest(args))
			return result, true, err
		}

	case "delete":
		if objectType == "TRANSPORT" {
			if objectName == "" {
				return newToolResultError("transport number is required for delete"), true, nil
			}
			result, err := s.handleDeleteTransport(ctx, newRequest(map[string]any{"transport": objectName}))
			return result, true, err
		}

	case "deploy":
		deployType, _ := params["type"].(string)
		if deployType == "release_transport" {
			transport, _ := params["transport"].(string)
			if transport == "" {
				transport = objectName
			}
			if transport == "" {
				return newToolResultError("transport number is required for release"), true, nil
			}
			args := map[string]any{"transport": transport}
			if ignoreLocks, ok := params["ignore_locks"].(bool); ok {
				args["ignore_locks"] = ignoreLocks
			}
			if skipATC, ok := params["skip_atc"].(bool); ok {
				args["skip_atc"] = skipATC
			}
			result, err := s.handleReleaseTransport(ctx, newRequest(args))
			return result, true, err
		}
	}

	return nil, false, nil
}

// --- Transport Management Handlers ---

func (s *Server) handleGetUserTransports(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userName, ok := request.Params.Arguments["user_name"].(string)
	if !ok || userName == "" {
		return newToolResultError("user_name is required"), nil
	}

	transports, err := s.adtClient.GetUserTransports(ctx, userName)
	if err != nil {
		return wrapErr("GetUserTransports", err), nil
	}

	// Format output
	var sb strings.Builder
	fmt.Fprintf(&sb, "Transports for user %s:\n\n", strings.ToUpper(userName))

	if len(transports.Workbench) > 0 {
		sb.WriteString("=== Workbench Requests ===\n")
		for _, tr := range transports.Workbench {
			formatTransportSummary(&sb, &tr)
		}
	} else {
		sb.WriteString("No workbench requests found.\n")
	}

	sb.WriteString("\n")

	if len(transports.Customizing) > 0 {
		sb.WriteString("=== Customizing Requests ===\n")
		for _, tr := range transports.Customizing {
			formatTransportSummary(&sb, &tr)
		}
	} else {
		sb.WriteString("No customizing requests found.\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func formatTransportSummary(sb *strings.Builder, tr *adt.TransportSummary) {
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
					fmt.Fprintf(sb, "      - %s %s %s\n", obj.PgmID, obj.Type, obj.Name)
				}
			}
		}
	}
}

func (s *Server) handleGetTransportInfo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.Params.Arguments["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	devClass, ok := request.Params.Arguments["dev_class"].(string)
	if !ok || devClass == "" {
		return newToolResultError("dev_class is required"), nil
	}

	info, err := s.adtClient.GetTransportInfo(ctx, objectURL, devClass)
	if err != nil {
		return wrapErr("GetTransportInfo", err), nil
	}

	// Format output
	var sb strings.Builder
	sb.WriteString("Transport Information:\n\n")
	fmt.Fprintf(&sb, "PGMID: %s\n", info.PgmID)
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
	code, ok := request.Params.Arguments["code"].(string)
	if !ok || code == "" {
		return newToolResultError("code parameter is required"), nil
	}

	opts := &adt.ExecuteABAPOptions{}

	if riskLevel, ok := request.Params.Arguments["risk_level"].(string); ok && riskLevel != "" {
		opts.RiskLevel = riskLevel
	}

	if returnVar, ok := request.Params.Arguments["return_variable"].(string); ok && returnVar != "" {
		opts.ReturnVariable = returnVar
	}

	if keepProgram, ok := request.Params.Arguments["keep_program"].(bool); ok {
		opts.KeepProgram = keepProgram
	}

	if prefix, ok := request.Params.Arguments["program_prefix"].(string); ok && prefix != "" {
		opts.ProgramPrefix = prefix
	}

	result, err := s.adtClient.ExecuteABAP(ctx, code, opts)
	if err != nil {
		return wrapErr("ExecuteABAP", err), nil
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

	user, _ := request.Params.Arguments["user"].(string)

	transports, err := s.adtClient.ListTransports(ctx, user)
	if err != nil {
		return wrapErr("ListTransports", err), nil
	}

	if len(transports) == 0 {
		return mcp.NewToolResultText("No modifiable transports found."), nil
	}

	return newToolResultJSON(transports), nil
}

func (s *Server) handleGetTransport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	transport, ok := request.Params.Arguments["transport"].(string)
	if !ok || transport == "" {
		return newToolResultError("transport is required"), nil
	}

	// Check safety config for transport operations
	if err := s.adtClient.Safety().CheckTransport(transport, "GetTransport", false); err != nil {
		return newToolResultError(err.Error()), nil
	}

	result, err := s.adtClient.GetTransport(ctx, transport)
	if err != nil {
		return wrapErr("GetTransport", err), nil
	}
	return newToolResultJSON(result), nil
}

func (s *Server) handleCreateTransport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check safety config for transport write operations
	if err := s.adtClient.Safety().CheckTransport("", "CreateTransport", true); err != nil {
		return newToolResultError(err.Error()), nil
	}

	description, ok := request.Params.Arguments["description"].(string)
	if !ok || description == "" {
		return newToolResultError("description is required"), nil
	}

	pkg, ok := request.Params.Arguments["package"].(string)
	if !ok || pkg == "" {
		return newToolResultError("package is required"), nil
	}

	transportLayer, _ := request.Params.Arguments["transport_layer"].(string)
	transportType, _ := request.Params.Arguments["type"].(string)

	opts := adt.CreateTransportOptions{
		Description:    description,
		Package:        pkg,
		TransportLayer: transportLayer,
		Type:           transportType,
	}

	transportNumber, err := s.adtClient.CreateTransport(ctx, opts)
	if err != nil {
		return wrapErr("CreateTransport", err), nil
	}
	return mcp.NewToolResultText("Transport created: " + transportNumber), nil
}

func (s *Server) handleReleaseTransport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	transport, ok := request.Params.Arguments["transport"].(string)
	if !ok || transport == "" {
		return newToolResultError("transport is required"), nil
	}

	// Check safety config for transport write operations
	if err := s.adtClient.Safety().CheckTransport(transport, "ReleaseTransport", true); err != nil {
		return newToolResultError(err.Error()), nil
	}

	opts := adt.ReleaseTransportOptions{}
	if skipATC, _ := request.Params.Arguments["skip_atc"].(bool); skipATC {
		opts.Action = adt.ReleaseActionSkipATC
	} else if ignoreLocks, _ := request.Params.Arguments["ignore_locks"].(bool); ignoreLocks {
		opts.Action = adt.ReleaseActionIgnoreLocks
	}

	err := s.adtClient.ReleaseTransport(ctx, transport, opts)
	if err != nil {
		return wrapErr("ReleaseTransport", err), nil
	}
	return mcp.NewToolResultText("Transport " + transport + " released successfully."), nil
}

func (s *Server) handleDeleteTransport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	transport, ok := request.Params.Arguments["transport"].(string)
	if !ok || transport == "" {
		return newToolResultError("transport is required"), nil
	}

	// Check safety config for transport write operations
	if err := s.adtClient.Safety().CheckTransport(transport, "DeleteTransport", true); err != nil {
		return newToolResultError(err.Error()), nil
	}

	err := s.adtClient.DeleteTransport(ctx, transport)
	if err != nil {
		return wrapErr("DeleteTransport", err), nil
	}
	return mcp.NewToolResultText("Transport " + transport + " deleted successfully."), nil
}
