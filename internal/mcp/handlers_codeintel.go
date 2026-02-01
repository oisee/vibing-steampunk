// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_codeintel.go contains handlers for code intelligence operations
// (find definition, references, completion, pretty print, type hierarchy, etc.).
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Code Intelligence Routing ---
// Routes for this module:
//   analyze: type=definition, type=completion, type=pretty_print, type=class_components, type=type_hierarchy, type=references
//   system: type=pretty_printer_settings, type=inactive_objects
//   edit: type=set_pretty_printer_settings

// routeCodeIntelAction routes code intelligence actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeCodeIntelAction(ctx context.Context, action, objectType, _ string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	switch action {
	case "analyze":
		analysisType, _ := params["type"].(string)
		switch analysisType {
		case "definition":
			sourceURL, _ := params["source_url"].(string)
			source, _ := params["source"].(string)
			line, _ := params["line"].(float64)
			startCol, _ := params["start_column"].(float64)
			endCol, _ := params["end_column"].(float64)
			if sourceURL == "" || source == "" {
				return newToolResultError("source_url and source are required for definition"), true, nil
			}
			args := map[string]any{
				"source_url":   sourceURL,
				"source":       source,
				"line":         line,
				"start_column": startCol,
				"end_column":   endCol,
			}
			if impl, ok := params["implementation"].(bool); ok {
				args["implementation"] = impl
			}
			if mp, ok := params["main_program"].(string); ok {
				args["main_program"] = mp
			}
			result, err := s.handleFindDefinition(ctx, newRequest(args))
			return result, true, err

		case "completion":
			sourceURL, _ := params["source_url"].(string)
			source, _ := params["source"].(string)
			line, _ := params["line"].(float64)
			column, _ := params["column"].(float64)
			if sourceURL == "" || source == "" {
				return newToolResultError("source_url and source are required for completion"), true, nil
			}
			result, err := s.handleCodeCompletion(ctx, newRequest(map[string]any{
				"source_url": sourceURL,
				"source":     source,
				"line":       line,
				"column":     column,
			}))
			return result, true, err

		case "pretty_print":
			source, _ := params["source"].(string)
			if source == "" {
				return newToolResultError("source is required for pretty_print"), true, nil
			}
			result, err := s.handlePrettyPrint(ctx, newRequest(map[string]any{"source": source}))
			return result, true, err

		case "class_components":
			classURL, _ := params["class_url"].(string)
			if classURL == "" {
				return newToolResultError("class_url is required for class_components"), true, nil
			}
			result, err := s.handleGetClassComponents(ctx, newRequest(map[string]any{"class_url": classURL}))
			return result, true, err

		case "type_hierarchy":
			sourceURL, _ := params["source_url"].(string)
			source, _ := params["source"].(string)
			line, _ := params["line"].(float64)
			column, _ := params["column"].(float64)
			if sourceURL == "" || source == "" {
				return newToolResultError("source_url and source are required for type_hierarchy"), true, nil
			}
			args := map[string]any{
				"source_url": sourceURL,
				"source":     source,
				"line":       line,
				"column":     column,
			}
			if superTypes, ok := params["super_types"].(bool); ok {
				args["super_types"] = superTypes
			}
			result, err := s.handleGetTypeHierarchy(ctx, newRequest(args))
			return result, true, err

		case "references":
			objectURL, _ := params["object_url"].(string)
			if objectURL == "" {
				return newToolResultError("object_url is required for references"), true, nil
			}
			result, err := s.handleFindReferences(ctx, newRequest(map[string]any{"object_url": objectURL}))
			return result, true, err
		}

	case "system":
		systemType, _ := params["type"].(string)
		switch systemType {
		case "pretty_printer_settings":
			result, err := s.handleGetPrettyPrinterSettings(ctx, newRequest(nil))
			return result, true, err
		case "inactive_objects":
			result, err := s.handleGetInactiveObjects(ctx, newRequest(nil))
			return result, true, err
		}

	case "edit":
		editType, _ := params["type"].(string)
		if editType == "set_pretty_printer_settings" {
			indentation, _ := params["indentation"].(bool)
			style, _ := params["style"].(string)
			if style == "" {
				return newToolResultError("style is required for set_pretty_printer_settings"), true, nil
			}
			result, err := s.handleSetPrettyPrinterSettings(ctx, newRequest(map[string]any{
				"indentation": indentation,
				"style":       style,
			}))
			return result, true, err
		}
	}

	return nil, false, nil
}

// --- Code Intelligence Handlers ---

func (s *Server) handleFindDefinition(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceURL, ok := request.Params.Arguments["source_url"].(string)
	if !ok || sourceURL == "" {
		return newToolResultError("source_url is required"), nil
	}

	source, ok := request.Params.Arguments["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	lineF, ok := request.Params.Arguments["line"].(float64)
	if !ok {
		return newToolResultError("line is required"), nil
	}
	line := int(lineF)

	startColF, ok := request.Params.Arguments["start_column"].(float64)
	if !ok {
		return newToolResultError("start_column is required"), nil
	}
	startCol := int(startColF)

	endColF, ok := request.Params.Arguments["end_column"].(float64)
	if !ok {
		return newToolResultError("end_column is required"), nil
	}
	endCol := int(endColF)

	implementation := false
	if impl, ok := request.Params.Arguments["implementation"].(bool); ok {
		implementation = impl
	}

	mainProgram := ""
	if mp, ok := request.Params.Arguments["main_program"].(string); ok {
		mainProgram = mp
	}

	result, err := s.adtClient.FindDefinition(ctx, sourceURL, source, line, startCol, endCol, implementation, mainProgram)
	if err != nil {
		return wrapErr("FindDefinition", err), nil
	}
	return newToolResultJSON(result), nil
}

func (s *Server) handleFindReferences(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.Params.Arguments["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	line := 0
	column := 0
	if lineF, ok := request.Params.Arguments["line"].(float64); ok {
		line = int(lineF)
	}
	if colF, ok := request.Params.Arguments["column"].(float64); ok {
		column = int(colF)
	}

	results, err := s.adtClient.FindReferences(ctx, objectURL, line, column)
	if err != nil {
		return wrapErr("FindReferences", err), nil
	}
	return newToolResultJSON(results), nil
}

func (s *Server) handleCodeCompletion(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceURL, ok := request.Params.Arguments["source_url"].(string)
	if !ok || sourceURL == "" {
		return newToolResultError("source_url is required"), nil
	}

	source, ok := request.Params.Arguments["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	lineF, ok := request.Params.Arguments["line"].(float64)
	if !ok {
		return newToolResultError("line is required"), nil
	}
	line := int(lineF)

	colF, ok := request.Params.Arguments["column"].(float64)
	if !ok {
		return newToolResultError("column is required"), nil
	}
	column := int(colF)

	proposals, err := s.adtClient.CodeCompletion(ctx, sourceURL, source, line, column)
	if err != nil {
		return wrapErr("CodeCompletion", err), nil
	}
	return newToolResultJSON(proposals), nil
}

func (s *Server) handlePrettyPrint(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	source, ok := request.Params.Arguments["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	formatted, err := s.adtClient.PrettyPrint(ctx, source)
	if err != nil {
		return wrapErr("PrettyPrint", err), nil
	}
	return mcp.NewToolResultText(formatted), nil
}

func (s *Server) handleGetPrettyPrinterSettings(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	settings, err := s.adtClient.GetPrettyPrinterSettings(ctx)
	if err != nil {
		return wrapErr("GetPrettyPrinterSettings", err), nil
	}
	return newToolResultJSON(settings), nil
}

func (s *Server) handleSetPrettyPrinterSettings(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	indentation, ok := request.Params.Arguments["indentation"].(bool)
	if !ok {
		return newToolResultError("indentation is required"), nil
	}

	style, ok := request.Params.Arguments["style"].(string)
	if !ok || style == "" {
		return newToolResultError("style is required"), nil
	}

	settings := &adt.PrettyPrinterSettings{
		Indentation: indentation,
		Style:       adt.PrettyPrinterStyle(style),
	}

	err := s.adtClient.SetPrettyPrinterSettings(ctx, settings)
	if err != nil {
		return wrapErr("SetPrettyPrinterSettings", err), nil
	}
	return mcp.NewToolResultText("Pretty printer settings updated successfully"), nil
}

func (s *Server) handleGetTypeHierarchy(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceURL, ok := request.Params.Arguments["source_url"].(string)
	if !ok || sourceURL == "" {
		return newToolResultError("source_url is required"), nil
	}

	source, ok := request.Params.Arguments["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	lineF, ok := request.Params.Arguments["line"].(float64)
	if !ok {
		return newToolResultError("line is required"), nil
	}
	line := int(lineF)

	colF, ok := request.Params.Arguments["column"].(float64)
	if !ok {
		return newToolResultError("column is required"), nil
	}
	column := int(colF)

	superTypes := false
	if st, ok := request.Params.Arguments["super_types"].(bool); ok {
		superTypes = st
	}

	hierarchy, err := s.adtClient.GetTypeHierarchy(ctx, sourceURL, source, line, column, superTypes)
	if err != nil {
		return wrapErr("GetTypeHierarchy", err), nil
	}
	return newToolResultJSON(hierarchy), nil
}

func (s *Server) handleGetClassComponents(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	classURL, ok := request.Params.Arguments["class_url"].(string)
	if !ok || classURL == "" {
		return newToolResultError("class_url is required"), nil
	}

	components, err := s.adtClient.GetClassComponents(ctx, classURL)
	if err != nil {
		return wrapErr("GetClassComponents", err), nil
	}

	// Format output with summary
	output := formatClassComponents(components)
	return mcp.NewToolResultText(output), nil
}

// formatClassComponents creates a readable summary of class components
func formatClassComponents(comp *adt.ClassComponent) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Class: %s (%s)\n\n", comp.Name, comp.Type)

	// Group components by type
	methods := []adt.ClassComponent{}
	attributes := []adt.ClassComponent{}
	events := []adt.ClassComponent{}
	types := []adt.ClassComponent{}
	others := []adt.ClassComponent{}

	for _, c := range comp.Components {
		switch {
		case strings.Contains(c.Type, "METH"):
			methods = append(methods, c)
		case strings.Contains(c.Type, "DATA") || strings.Contains(c.Type, "ATTR"):
			attributes = append(attributes, c)
		case strings.Contains(c.Type, "EVNT") || strings.Contains(c.Type, "EVENT"):
			events = append(events, c)
		case strings.Contains(c.Type, "TYPE"):
			types = append(types, c)
		default:
			others = append(others, c)
		}
	}

	if len(methods) > 0 {
		fmt.Fprintf(&sb, "## Methods (%d)\n", len(methods))
		for _, m := range methods {
			flags := componentFlags(m)
			fmt.Fprintf(&sb, "  - %s [%s]%s\n", m.Name, m.Visibility, flags)
			if m.Description != "" {
				fmt.Fprintf(&sb, "    %s\n", m.Description)
			}
		}
		sb.WriteString("\n")
	}

	if len(attributes) > 0 {
		fmt.Fprintf(&sb, "## Attributes (%d)\n", len(attributes))
		for _, a := range attributes {
			flags := componentFlags(a)
			fmt.Fprintf(&sb, "  - %s [%s]%s\n", a.Name, a.Visibility, flags)
		}
		sb.WriteString("\n")
	}

	if len(events) > 0 {
		fmt.Fprintf(&sb, "## Events (%d)\n", len(events))
		for _, e := range events {
			fmt.Fprintf(&sb, "  - %s [%s]\n", e.Name, e.Visibility)
		}
		sb.WriteString("\n")
	}

	if len(types) > 0 {
		fmt.Fprintf(&sb, "## Types (%d)\n", len(types))
		for _, t := range types {
			fmt.Fprintf(&sb, "  - %s [%s]\n", t.Name, t.Visibility)
		}
		sb.WriteString("\n")
	}

	if len(others) > 0 {
		fmt.Fprintf(&sb, "## Other Components (%d)\n", len(others))
		for _, o := range others {
			fmt.Fprintf(&sb, "  - %s (%s) [%s]\n", o.Name, o.Type, o.Visibility)
		}
	}

	return sb.String()
}

func componentFlags(c adt.ClassComponent) string {
	var flags []string
	if c.IsStatic {
		flags = append(flags, "static")
	}
	if c.IsFinal {
		flags = append(flags, "final")
	}
	if c.IsAbstract {
		flags = append(flags, "abstract")
	}
	if c.ReadOnly {
		flags = append(flags, "read-only")
	}
	if c.Constant {
		flags = append(flags, "constant")
	}
	if len(flags) > 0 {
		return " " + strings.Join(flags, ", ")
	}
	return ""
}

func (s *Server) handleGetInactiveObjects(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objects, err := s.adtClient.GetInactiveObjects(ctx)
	if err != nil {
		return wrapErr("GetInactiveObjects", err), nil
	}

	if len(objects) == 0 {
		return mcp.NewToolResultText("No inactive objects found."), nil
	}

	// Format output
	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d inactive object(s):\n\n", len(objects))

	for _, record := range objects {
		if record.Object != nil {
			obj := record.Object
			fmt.Fprintf(&sb, "- %s (%s)\n", obj.Name, obj.Type)
			fmt.Fprintf(&sb, "  URI: %s\n", obj.URI)
			if obj.User != "" {
				fmt.Fprintf(&sb, "  User: %s\n", obj.User)
			}
			if obj.Deleted {
				sb.WriteString("  Status: DELETED\n")
			}
		}
		if record.Transport != nil {
			tr := record.Transport
			fmt.Fprintf(&sb, "  Transport: %s\n", tr.Name)
		}
		sb.WriteString("\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}
