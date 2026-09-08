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

// routeTransportAction routes "system" with transport-related types.
func (s *Server) routeTransportAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "system" {
		return nil, false, nil
	}
	transportType := getStringParam(params, "type")
	switch transportType {
	case "list_transports":
		return s.callHandler(ctx, s.handleListTransports, params)
	case "get_transport":
		return s.callHandler(ctx, s.handleGetTransport, params)
	case "create_transport":
		return s.callHandler(ctx, s.handleCreateTransport, params)
	case "release_transport":
		return s.callHandler(ctx, s.handleReleaseTransport, params)
	case "delete_transport":
		return s.callHandler(ctx, s.handleDeleteTransport, params)
	case "get_user_transports":
		return s.callHandler(ctx, s.handleGetUserTransports, params)
	case "get_transport_info":
		return s.callHandler(ctx, s.handleGetTransportInfo, params)
	case "execute_abap":
		return s.callHandler(ctx, s.handleExecuteABAP, params)
	}
	return nil, false, nil
}

// --- Transport Management Handlers ---

// transportQueryFromArgs reads the shared listing parameters of
// get_user_transports and list_transports. userKey names the parameter that
// carries the user, which the two tools spell differently.
func transportQueryFromArgs(args map[string]any, userKey string) adt.TransportQuery {
	q := adt.TransportQuery{
		User:            getStringParam(args, userKey),
		RequestTypes:    getStringParam(args, "request_type"),
		RequestStatuses: getStringParam(args, "request_status"),
		ReleasedFrom:    getStringParam(args, "released_from"),
		ReleasedTo:      getStringParam(args, "released_to"),
		Source:          getStringParam(args, "source"),
		ConfigURI:       getStringParam(args, "config_uri"),
		Targets:         true,
	}
	if targets, ok := getBoolParam(args, "targets"); ok {
		q.Targets = targets
	}
	return q
}

// transportListingHeader is the first line of a listing: whom it is for,
// where it came from and which filters applied.
func transportListingHeader(res *adt.TransportQueryResult) string {
	user := res.Query.User
	if user == "" {
		user = "the connection user"
	}
	header := fmt.Sprintf("Transports for user %s (source: %s; %s)", user, res.Source, res.Query.Describe())
	if res.ConfigURI != "" {
		header += fmt.Sprintf("\nSearch configuration: %s", res.ConfigURI)
	}
	for _, n := range res.Notes {
		header += "\nNote: " + n
	}
	return header
}

func (s *Server) handleGetUserTransports(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	res, err := s.adtClient.QueryUserTransports(ctx, transportQueryFromArgs(args, "user_name"))
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetUserTransports failed: %v", err)), nil
	}

	var sb strings.Builder
	sb.WriteString(transportListingHeader(res))
	sb.WriteString("\n\n")

	transports := res.Transports
	if len(transports.Workbench) > 0 {
		sb.WriteString("=== Workbench Requests ===\n")
		formatTransportBuckets(&sb, transports.Workbench)
	} else {
		sb.WriteString("No workbench requests found.\n")
	}

	sb.WriteString("\n")

	if len(transports.Customizing) > 0 {
		sb.WriteString("=== Customizing Requests ===\n")
		formatTransportBuckets(&sb, transports.Customizing)
	} else {
		sb.WriteString("No customizing requests found.\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// formatTransportBuckets prints modifiable requests before released ones,
// each group under its own heading, so a D and an R are never mistaken.
func formatTransportBuckets(sb *strings.Builder, requests []adt.TransportRequest) {
	for _, bucket := range []struct{ key, title string }{
		{"modifiable", "--- Modifiable ---"},
		{"released", "--- Released ---"},
		{"", "--- Other ---"},
	} {
		var group []adt.TransportRequest
		for _, tr := range requests {
			if tr.Bucket == bucket.key {
				group = append(group, tr)
			}
		}
		if len(group) == 0 {
			continue
		}
		fmt.Fprintf(sb, "%s\n", bucket.title)
		for i := range group {
			formatTransportRequest(sb, &group[i])
		}
	}
}

func formatTransportRequest(sb *strings.Builder, tr *adt.TransportRequest) {
	fmt.Fprintf(sb, "\n%s - %s\n", tr.Number, tr.Description)
	fmt.Fprintf(sb, "  Owner: %s, Status: %s", tr.Owner, tr.Status)
	if tr.Target != "" {
		fmt.Fprintf(sb, ", Target: %s", tr.Target)
	}
	if tr.Project != "" {
		fmt.Fprintf(sb, ", Project: %s", tr.Project)
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

	// The one-line message names the failure; this is where it is spelled out.
	// A model reading "Success: false" and nothing else has to guess whether the
	// code was wrong or the system was, and it will guess.
	if result.Failure != nil {
		fmt.Fprintf(&sb, "\nFailure (%s): %s\n", result.Failure.Kind, result.Failure.Title)
		if result.Failure.Line > 0 {
			fmt.Fprintf(&sb, "  at line %d of the code you sent\n", result.Failure.Line)
		}
		for _, detail := range result.Failure.Details {
			fmt.Fprintf(&sb, "  %s\n", detail)
		}
	}

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
	args := request.GetArguments()
	res, err := s.adtClient.QueryTransports(ctx, transportQueryFromArgs(args, "user"))
	if err != nil {
		return newToolResultError(fmt.Sprintf("ListTransports failed: %v", err)), nil
	}

	rows := adt.FlattenTransports(res.Transports)
	if len(rows) == 0 {
		return mcp.NewToolResultText(transportListingHeader(res) + "\n\nNo transport requests found."), nil
	}

	out := struct {
		Source     string                 `json:"source"`
		ConfigURI  string                 `json:"configUri,omitempty"`
		Query      adt.TransportQuery     `json:"query"`
		Notes      []string               `json:"notes,omitempty"`
		Transports []adt.TransportSummary `json:"transports"`
	}{res.Source, res.ConfigURI, res.Query, res.Notes, rows}

	jsonBytes, err := json.MarshalIndent(out, "", "  ")
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
