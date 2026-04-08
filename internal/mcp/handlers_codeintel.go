// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_codeintel.go contains handlers for code intelligence operations
// (find definition, references, completion, pretty print, type hierarchy, etc.).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/vinchacho/vibing-steampunk/pkg/adt"
)

// routeCodeIntelAction routes "analyze" action for code intelligence operations.
func (s *Server) routeCodeIntelAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "analyze" {
		return nil, false, nil
	}
	analysisType := getStringParam(params, "type")
	switch analysisType {
	case "definition":
		return s.callHandler(ctx, s.handleFindDefinition, params)
	case "references":
		return s.callHandler(ctx, s.handleFindReferences, params)
	case "completion":
		return s.callHandler(ctx, s.handleCodeCompletion, params)
	case "pretty_print":
		return s.callHandler(ctx, s.handlePrettyPrint, params)
	case "get_pretty_printer_settings":
		return s.callHandler(ctx, s.handleGetPrettyPrinterSettings, params)
	case "set_pretty_printer_settings":
		return s.callHandler(ctx, s.handleSetPrettyPrinterSettings, params)
	case "type_hierarchy":
		return s.callHandler(ctx, s.handleGetTypeHierarchy, params)
	case "class_components":
		return s.callHandler(ctx, s.handleGetClassComponents, params)
	case "inactive_objects":
		return s.callHandler(ctx, s.handleGetInactiveObjects, params)
	case "abap_help":
		return s.callHandler(ctx, s.handleGetAbapHelp, params)
	}
	return nil, false, nil
}

// --- Code Intelligence Handlers ---

func (s *Server) handleFindDefinition(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceURL, ok := request.GetArguments()["source_url"].(string)
	if !ok || sourceURL == "" {
		return newToolResultError("source_url is required"), nil
	}

	source, ok := request.GetArguments()["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	lineF, ok := request.GetArguments()["line"].(float64)
	if !ok {
		return newToolResultError("line is required"), nil
	}
	line := int(lineF)

	startColF, ok := request.GetArguments()["start_column"].(float64)
	if !ok {
		return newToolResultError("start_column is required"), nil
	}
	startCol := int(startColF)

	endColF, ok := request.GetArguments()["end_column"].(float64)
	if !ok {
		return newToolResultError("end_column is required"), nil
	}
	endCol := int(endColF)

	implementation := false
	if impl, ok := request.GetArguments()["implementation"].(bool); ok {
		implementation = impl
	}

	mainProgram := ""
	if mp, ok := request.GetArguments()["main_program"].(string); ok {
		mainProgram = mp
	}

	result, err := s.adtClient.FindDefinition(ctx, sourceURL, source, line, startCol, endCol, implementation, mainProgram)
	if err != nil {
		return newToolResultError(fmt.Sprintf("FindDefinition failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleFindReferences(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.GetArguments()["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	line := 0
	column := 0
	if lineF, ok := request.GetArguments()["line"].(float64); ok {
		line = int(lineF)
	}
	if colF, ok := request.GetArguments()["column"].(float64); ok {
		column = int(colF)
	}

	results, err := s.adtClient.FindReferences(ctx, objectURL, line, column)
	if err != nil {
		return newToolResultError(fmt.Sprintf("FindReferences failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleCodeCompletion(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceURL, ok := request.GetArguments()["source_url"].(string)
	if !ok || sourceURL == "" {
		return newToolResultError("source_url is required"), nil
	}

	source, ok := request.GetArguments()["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	lineF, ok := request.GetArguments()["line"].(float64)
	if !ok {
		return newToolResultError("line is required"), nil
	}
	line := int(lineF)

	colF, ok := request.GetArguments()["column"].(float64)
	if !ok {
		return newToolResultError("column is required"), nil
	}
	column := int(colF)

	proposals, err := s.adtClient.CodeCompletion(ctx, sourceURL, source, line, column)
	if err != nil {
		return newToolResultError(fmt.Sprintf("CodeCompletion failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(proposals, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handlePrettyPrint(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	source, ok := request.GetArguments()["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	formatted, err := s.adtClient.PrettyPrint(ctx, source)
	if err != nil {
		return newToolResultError(fmt.Sprintf("PrettyPrint failed: %v", err)), nil
	}

	return mcp.NewToolResultText(formatted), nil
}

func (s *Server) handleGetPrettyPrinterSettings(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	settings, err := s.adtClient.GetPrettyPrinterSettings(ctx)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetPrettyPrinterSettings failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(settings, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleSetPrettyPrinterSettings(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	indentation, ok := request.GetArguments()["indentation"].(bool)
	if !ok {
		return newToolResultError("indentation is required"), nil
	}

	style, ok := request.GetArguments()["style"].(string)
	if !ok || style == "" {
		return newToolResultError("style is required"), nil
	}

	settings := &adt.PrettyPrinterSettings{
		Indentation: indentation,
		Style:       adt.PrettyPrinterStyle(style),
	}

	err := s.adtClient.SetPrettyPrinterSettings(ctx, settings)
	if err != nil {
		return newToolResultError(fmt.Sprintf("SetPrettyPrinterSettings failed: %v", err)), nil
	}

	return mcp.NewToolResultText("Pretty printer settings updated successfully"), nil
}

func (s *Server) handleGetTypeHierarchy(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceURL, ok := request.GetArguments()["source_url"].(string)
	if !ok || sourceURL == "" {
		return newToolResultError("source_url is required"), nil
	}

	source, ok := request.GetArguments()["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	lineF, ok := request.GetArguments()["line"].(float64)
	if !ok {
		return newToolResultError("line is required"), nil
	}
	line := int(lineF)

	colF, ok := request.GetArguments()["column"].(float64)
	if !ok {
		return newToolResultError("column is required"), nil
	}
	column := int(colF)

	superTypes := false
	if st, ok := request.GetArguments()["super_types"].(bool); ok {
		superTypes = st
	}

	hierarchy, err := s.adtClient.GetTypeHierarchy(ctx, sourceURL, source, line, column, superTypes)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetTypeHierarchy failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(hierarchy, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleGetClassComponents(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	classURL, ok := request.GetArguments()["class_url"].(string)
	if !ok || classURL == "" {
		return newToolResultError("class_url is required"), nil
	}

	components, err := s.adtClient.GetClassComponents(ctx, classURL)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetClassComponents failed: %v", err)), nil
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

func (s *Server) handleGetInactiveObjects(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objects, err := s.adtClient.GetInactiveObjects(ctx)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetInactiveObjects failed: %v", err)), nil
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
