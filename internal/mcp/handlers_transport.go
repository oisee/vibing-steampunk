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
	case "merge_transports":
		return s.callHandler(ctx, s.handleMergeTransports, params)
	case "move_transport_object", "move_object":
		return s.callHandler(ctx, s.handleMoveTransportObject, params)
	case "copy_to_toc", "transport_of_copies":
		return s.callHandler(ctx, s.handleCopyToTransportOfCopies, params)
	case "add_transport_object", "add_to_transport":
		return s.callHandler(ctx, s.handleAddTransportObjects, params)
	case "remove_transport_object", "remove_from_transport":
		return s.callHandler(ctx, s.handleRemoveTransportObject, params)
	case "upload_transport":
		return s.callHandler(ctx, s.handleUploadTransport, params)
	case "transport_buffer":
		return s.callHandler(ctx, s.handleTransportBuffer, params)
	case "transport_status":
		return s.callHandler(ctx, s.handleTransportStatus, params)
	case "import_status":
		return s.callHandler(ctx, s.handleImportStatus, params)
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
	return s.longCall(ctx, request, "execute_abap", s.executeABAP)
}

// executeABAP is handleExecuteABAP without the call budget (see longCall).
func (s *Server) executeABAP(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	return mcp.NewToolResultText(executeABAPJSON(result)), nil
}

// executeABAPJSON is the execute_abap answer: the run's own fields under the
// names adt.ExecuteABAPResult gives them (success, programName, output,
// executionTime, message, cleanedUp, failure), plus result_text, the returned
// value in full and unwrapped. `vsp execute --json` prints the same.
func executeABAPJSON(result *adt.ExecuteABAPResult) string {
	out, err := adt.IndentJSON(result.Lean())
	if err != nil {
		return fmt.Sprintf(`{"success": false, "message": %q}`, "could not encode the result: "+err.Error())
	}
	return string(out)
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
	ctsProject, _ := request.GetArguments()["cts_project"].(string)
	target, _ := request.GetArguments()["target"].(string)

	opts := adt.CreateTransportOptions{
		Description:    description,
		Package:        pkg,
		TransportLayer: transportLayer,
		Type:           transportType,
		CTSProject:     ctsProject,
		Target:         target,
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

// handleMergeTransports merges one or more requests into a target, the way
// SE09's Merge Requests does, through ZADT_VSP's function bridge:
// SAP(action="system", params={"type": "merge_transports", "source": ["TR-A", "TR-B"], "target": "TR-C"}).
// Each source is merged in turn; the first failure stops the rest.
func (s *Server) handleMergeTransports(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	target := getStringParam(args, "target")
	if target == "" {
		target = getStringParam(args, "into")
	}
	var sources []string
	switch v := args["source"].(type) {
	case string:
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				sources = append(sources, p)
			}
		}
	case []any:
		for _, p := range v {
			if str, ok := p.(string); ok && strings.TrimSpace(str) != "" {
				sources = append(sources, strings.TrimSpace(str))
			}
		}
	}
	if target == "" || len(sources) == 0 {
		return newToolResultError("source (one request or a list) and target are required"), nil
	}
	if err := s.checkTransportWrite("MergeTransports", append(sources, target)...); err != nil {
		return newToolResultError(err.Error()), nil
	}
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError(fmt.Sprintf("merging requests needs ZADT_VSP's function bridge: %v", err)), nil
	}
	var results []*adt.TransportMergeResult
	for _, src := range sources {
		res, err := s.adtClient.MergeTransports(ctx, s.debugWSClient, src, target)
		if res != nil {
			results = append(results, res)
		}
		if err != nil {
			return newToolResultJSON(map[string]any{"error": err.Error(), "merged": results}), nil
		}
	}
	return newToolResultJSON(map[string]any{"target": strings.ToUpper(target), "merged": results}), nil
}

// handleCopyToTransportOfCopies creates a transport of copies of a request and
// copies its objects into it, the way SE01 does, through ZADT_VSP's function
// bridge: SAP(action="system", params={"type": "copy_to_toc", "transport": "TR-A",
// "target": "QAS"}). "release": true releases it once filled.
func (s *Server) handleCopyToTransportOfCopies(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	source := getStringParam(args, "transport")
	if source == "" {
		source = getStringParam(args, "source")
	}
	opts := adt.TransportOfCopiesOptions{
		Target:      getStringParam(args, "target"),
		Description: getStringParam(args, "description"),
		CTSProject:  getStringParam(args, "cts_project"),
	}
	opts.Release, _ = getBoolParam(args, "release")
	if source == "" || opts.Target == "" {
		return newToolResultError("transport (the request to copy) and target (system or /GROUP/) are required"), nil
	}
	if err := s.adtClient.CheckTransportOfCopies(source); err != nil {
		return newToolResultError(err.Error()), nil
	}
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError(fmt.Sprintf("copying a request's objects needs ZADT_VSP's function bridge: %v", err)), nil
	}
	return transportOfCopiesResult(s.adtClient.CopyToTransportOfCopies(ctx, s.debugWSClient, source, opts)), nil
}

// transportOfCopiesResult is the tool result for a copy. A transport of
// copies that exists but is not what was asked for -- a list not copied, a
// release that failed -- is reported with what was done, marked as an error,
// never as success.
func transportOfCopiesResult(res *adt.TransportOfCopiesResult, err error) *mcp.CallToolResult {
	if err == nil {
		return newToolResultJSON(res)
	}
	if res == nil {
		return newToolResultError(err.Error())
	}
	out := newToolResultJSON(map[string]any{"error": err.Error(), "result": res})
	out.IsError = true
	return out
}

// handleMoveTransportObject moves one entry between requests:
// SAP(action="system", params={"type": "move_transport_object", "object": "PROG ZDEMO", "from": "TR-A", "to": "TR-B"}).
func (s *Server) handleMoveTransportObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	object := getStringParam(args, "object")
	if object == "" {
		object = strings.TrimSpace(getStringParam(args, "pgmid") + " " + getStringParam(args, "object_type") + " " + getStringParam(args, "object_name"))
	}
	key, err := adt.ParseTransportObject(object)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	from, to := getStringParam(args, "from"), getStringParam(args, "to")
	if from == "" || to == "" {
		return newToolResultError("from and to (request numbers) are required"), nil
	}
	if err = s.checkTransportWrite("MoveTransportObject", from, to); err != nil {
		return newToolResultError(err.Error()), nil
	}
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError(fmt.Sprintf("moving an object between requests needs ZADT_VSP's function bridge: %v", err)), nil
	}
	res, err := s.adtClient.MoveTransportObject(ctx, s.debugWSClient, key, from, to)
	if err != nil {
		if res != nil {
			return newToolResultJSON(map[string]any{"error": err.Error(), "result": res}), nil
		}
		return newToolResultError(err.Error()), nil
	}
	return newToolResultJSON(res), nil
}

// handleAddTransportObjects adds entries to a request, as SE09 does:
//
//	SAP(action="system", params={"type": "add_transport_object", "transport": "TR-A",
//	    "objects": ["LIMU REPT ZDEMO", "R3TR PROG ZDEMO2"]})
//	SAP(action="system", params={"type": "add_transport_object", "transport": "TR-A",
//	    "object": "R3TR TABU ZDEMO_CONF", "keys": ["100KEY1", "100KEY2*"]})
//
// "objects" may also hold {"object": "…", "keys": […]} items, for several
// tables with keys in one call.
func (s *Server) handleAddTransportObjects(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	transport := transportParam(args)
	if transport == "" {
		return newToolResultError("transport (the request or task to add to) is required"), nil
	}
	entries, err := transportEntries(args)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	if err = s.checkTransportWrite("AddTransportObjects", transport); err != nil {
		return newToolResultError(err.Error()), nil
	}
	if err = s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError(fmt.Sprintf("adding entries to a request needs ZADT_VSP's function bridge: %v", err)), nil
	}
	res, err := s.adtClient.AddTransportObjects(ctx, s.debugWSClient, transport, entries)
	if err != nil {
		if res != nil {
			return newToolResultJSON(map[string]any{"error": err.Error(), "result": res}), nil
		}
		return newToolResultError(err.Error()), nil
	}
	return newToolResultJSON(res), nil
}

// handleRemoveTransportObject takes one entry out of a request:
// SAP(action="system", params={"type": "remove_transport_object", "transport": "TR-A", "object": "PROG ZDEMO"}).
func (s *Server) handleRemoveTransportObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	transport := transportParam(args)
	if transport == "" {
		return newToolResultError("transport (the request the entry is in) is required"), nil
	}
	object := getStringParam(args, "object")
	if object == "" {
		object = strings.TrimSpace(getStringParam(args, "pgmid") + " " + getStringParam(args, "object_type") + " " + getStringParam(args, "object_name"))
	}
	key, err := adt.ParseTransportObject(object)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	if err = s.checkTransportWrite("RemoveTransportObject", transport); err != nil {
		return newToolResultError(err.Error()), nil
	}
	if err = s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError(fmt.Sprintf("removing an entry from a request needs ZADT_VSP's function bridge: %v", err)), nil
	}
	res, err := s.adtClient.RemoveTransportObject(ctx, s.debugWSClient, transport, key)
	if err != nil {
		if res != nil {
			return newToolResultJSON(map[string]any{"error": err.Error(), "result": res}), nil
		}
		return newToolResultError(err.Error()), nil
	}
	return newToolResultJSON(res), nil
}

// checkTransportWrite runs, before the ZADT_VSP WebSocket is dialled, the
// check the client method runs for each request it writes to. The client
// method keeps its own check; this one makes a refused write refuse without
// connecting anything first.
func (s *Server) checkTransportWrite(op string, requests ...string) error {
	for _, n := range requests {
		if err := s.adtClient.Safety().CheckTransport(strings.ToUpper(strings.TrimSpace(n)), op, true); err != nil {
			return err
		}
	}
	return nil
}

func transportParam(args map[string]any) string {
	for _, k := range []string{"transport", "request", "task"} {
		if v := strings.TrimSpace(getStringParam(args, k)); v != "" {
			return v
		}
	}
	return ""
}

// transportEntries reads the entries to add: "objects" as strings or
// {"object", "keys"} items, or a single "object" with optional "keys".
func transportEntries(args map[string]any) ([]adt.TransportEntry, error) {
	var out []adt.TransportEntry
	add := func(object string, keys any) error {
		key, err := adt.ParseTransportObject(object)
		if err != nil {
			return err
		}
		e := adt.TransportEntry{TransportObjectKey: key}
		switch ks := keys.(type) {
		case nil:
		case string:
			e.Keys = []string{ks}
		case []any:
			// Strings only: a JSON number loses its leading zeroes, and a
			// key such as 001... would then name another row.
			for i, k := range ks {
				s, ok := k.(string)
				if !ok {
					return fmt.Errorf("key %d of %s is %T, not a string; give table keys as strings", i+1, object, k)
				}
				e.Keys = append(e.Keys, s)
			}
		default:
			return fmt.Errorf("keys of %s: a list of table keys", object)
		}
		out = append(out, e)
		return nil
	}
	switch objs := args["objects"].(type) {
	case nil:
	case string:
		if err := add(objs, nil); err != nil {
			return nil, err
		}
	case []any:
		for i, raw := range objs {
			switch o := raw.(type) {
			case string:
				if err := add(o, nil); err != nil {
					return nil, err
				}
			case map[string]any:
				name, _ := o["object"].(string)
				if err := add(name, o["keys"]); err != nil {
					return nil, fmt.Errorf("objects[%d]: %w", i, err)
				}
			default:
				return nil, fmt.Errorf("objects[%d]: \"PGMID TYPE NAME\" or {\"object\": …, \"keys\": […]}", i)
			}
		}
	default:
		return nil, fmt.Errorf("objects: a list of \"PGMID TYPE NAME\" strings or {\"object\", \"keys\"} items")
	}
	if object := getStringParam(args, "object"); object != "" {
		if err := add(object, args["keys"]); err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("objects (or object) to add are required, e.g. [\"LIMU REPT ZDEMO\"]")
	}
	return out, nil
}
