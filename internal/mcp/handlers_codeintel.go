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
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

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
		return newToolResultError(fmt.Sprintf("FindDefinition failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
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
		return newToolResultError(fmt.Sprintf("FindReferences failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
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
		return newToolResultError(fmt.Sprintf("CodeCompletion failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(proposals, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handlePrettyPrint(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	source, ok := request.Params.Arguments["source"].(string)
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
		return newToolResultError(fmt.Sprintf("SetPrettyPrinterSettings failed: %v", err)), nil
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
		return newToolResultError(fmt.Sprintf("GetTypeHierarchy failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(hierarchy, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleGetClassComponents(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	classURL, ok := request.Params.Arguments["class_url"].(string)
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

// registerGetSource registers the unified GetSource tool
func (s *Server) registerGetSource() {
	s.mcpServer.AddTool(mcp.NewTool("GetSource",
		mcp.WithDescription("Unified tool for reading ABAP source code across different object types. Replaces GetProgram, GetClass, GetInterface, GetFunction, GetInclude, GetFunctionGroup, GetClassInclude."),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description("Object type: PROG (program), CLAS (class), INTF (interface), FUNC (function module), FUGR (function group), INCL (include), TABL (table), STRU (structure), TTYP (table type), DDLS (CDS DDL source), DDLX (CDS metadata extension), VIEW (DDIC view), BDEF (behavior definition), SRVD (service definition), SRVB (service binding), MSAG (message class)"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Object name (e.g., program name, class name, function module name)"),
		),
		mcp.WithString("parent",
			mcp.Description("Function group name (required only for FUNC type)"),
		),
		mcp.WithString("include",
			mcp.Description("Class include type for CLAS: definitions, implementations, macros, testclasses (optional)"),
		),
		mcp.WithString("method",
			mcp.Description("Method name for CLAS only: returns only the METHOD...ENDMETHOD block for the specified method (optional)"),
		),
	), s.handleGetSource)
}

// registerWriteSource registers the unified WriteSource tool
func (s *Server) registerWriteSource() {
	s.mcpServer.AddTool(mcp.NewTool("WriteSource",
		mcp.WithDescription("Unified tool for writing ABAP source code with automatic create/update detection. Supports PROG, CLAS, INTF, and RAP types (DDLS, DDLX, BDEF, SRVD)."),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description("Object type: PROG (program), CLAS (class), INTF (interface), DDLS (CDS view), DDLX (CDS metadata extension), BDEF (behavior definition), SRVD (service definition)"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Object name"),
		),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("ABAP source code"),
		),
		mcp.WithString("mode",
			mcp.Description("Operation mode: upsert (default, auto-detect), create (new only), update (existing only)"),
		),
		mcp.WithString("description",
			mcp.Description("Object description (required for create mode)"),
		),
		mcp.WithString("package",
			mcp.Description("Package name (required for create mode)"),
		),
		mcp.WithString("test_source",
			mcp.Description("Test source code for CLAS (auto-creates test include and runs tests)"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number"),
		),
		mcp.WithString("method",
			mcp.Description("For CLAS only: update only this method (source must be METHOD...ENDMETHOD block). Method must already exist in the class."),
		),
	), s.handleWriteSource)
}

// handleGetSource handles the unified GetSource tool call
func (s *Server) handleGetSource(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectType, ok := request.Params.Arguments["object_type"].(string)
	if !ok || objectType == "" {
		return newToolResultError("object_type is required"), nil
	}

	name, ok := request.Params.Arguments["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}

	parent, _ := request.Params.Arguments["parent"].(string)
	include, _ := request.Params.Arguments["include"].(string)
	method, _ := request.Params.Arguments["method"].(string)

	opts := &adt.GetSourceOptions{
		Parent:  parent,
		Include: include,
		Method:  method,
	}

	source, err := s.adtClient.GetSource(ctx, objectType, name, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetSource failed: %v", err)), nil
	}

	return mcp.NewToolResultText(source), nil
}

// handleWriteSource handles the unified WriteSource tool call
func (s *Server) handleWriteSource(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectType, ok := request.Params.Arguments["object_type"].(string)
	if !ok || objectType == "" {
		return newToolResultError("object_type is required"), nil
	}

	name, ok := request.Params.Arguments["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}

	source, ok := request.Params.Arguments["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	mode, _ := request.Params.Arguments["mode"].(string)
	description, _ := request.Params.Arguments["description"].(string)
	packageName, _ := request.Params.Arguments["package"].(string)
	testSource, _ := request.Params.Arguments["test_source"].(string)
	transport, _ := request.Params.Arguments["transport"].(string)
	method, _ := request.Params.Arguments["method"].(string)

	opts := &adt.WriteSourceOptions{
		Description: description,
		Package:     packageName,
		TestSource:  testSource,
		Transport:   transport,
		Method:      method,
	}

	if mode != "" {
		opts.Mode = adt.WriteSourceMode(mode)
	}

	result, err := s.adtClient.WriteSource(ctx, objectType, name, source, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("WriteSource failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

// registerGrepObjects registers the unified GrepObjects tool
func (s *Server) registerGrepObjects() {
	s.mcpServer.AddTool(mcp.NewTool("GrepObjects",
		mcp.WithDescription("Unified tool for searching regex patterns in single or multiple ABAP objects. Replaces GrepObject."),
		mcp.WithArray("object_urls",
			mcp.Required(),
			mcp.Description("Array of ADT object URLs to search (e.g., [\"/sap/bc/adt/programs/programs/ZTEST\"])"),
			mcp.Items(map[string]interface{}{"type": "string"}),
		),
		mcp.WithString("pattern",
			mcp.Required(),
			mcp.Description("Regular expression pattern (Go regexp syntax)"),
		),
		mcp.WithBoolean("case_insensitive",
			mcp.Description("If true, perform case-insensitive matching (default: false)"),
		),
		mcp.WithNumber("context_lines",
			mcp.Description("Number of context lines before/after each match (default: 0)"),
		),
	), s.handleGrepObjects)
}

// registerGrepPackages registers the unified GrepPackages tool
func (s *Server) registerGrepPackages() {
	s.mcpServer.AddTool(mcp.NewTool("GrepPackages",
		mcp.WithDescription("Unified tool for searching regex patterns across single or multiple packages with optional recursive subpackage search. Replaces GrepPackage."),
		mcp.WithArray("packages",
			mcp.Required(),
			mcp.Description("Array of package names to search (e.g., [\"$TMP\"], [\"Z\"] for namespace search)"),
			mcp.Items(map[string]interface{}{"type": "string"}),
		),
		mcp.WithBoolean("include_subpackages",
			mcp.Description("If true, recursively search all subpackages (default: false). Enables namespace-wide searches."),
		),
		mcp.WithString("pattern",
			mcp.Required(),
			mcp.Description("Regular expression pattern (Go regexp syntax)"),
		),
		mcp.WithBoolean("case_insensitive",
			mcp.Description("If true, perform case-insensitive matching (default: false)"),
		),
		mcp.WithArray("object_types",
			mcp.Description("Filter by object types (e.g., [\"CLAS/OC\", \"PROG/P\"]). Empty = search all types."),
			mcp.Items(map[string]interface{}{"type": "string"}),
		),
		mcp.WithNumber("max_results",
			mcp.Description("Maximum number of matching objects to return (0 = unlimited, default: 0)"),
		),
	), s.handleGrepPackages)
}

// registerImportFromFile registers the ImportFromFile tool (alias for DeployFromFile)
func (s *Server) registerImportFromFile() {
	s.mcpServer.AddTool(mcp.NewTool("ImportFromFile",
		mcp.WithDescription("Import ABAP object from local file into SAP system. Auto-detects object type from file extension, creates or updates, activates. Supports: programs, classes (with includes), interfaces, function groups/modules, CDS views (DDLS), metadata extensions (DDLX), behavior definitions (BDEF), service definitions (SRVD). For class includes (.clas.testclasses.abap, .clas.locals_def.abap, etc.), the parent class must exist."),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Absolute path to ABAP source file. Supported extensions: .prog.abap, .clas.abap, .clas.testclasses.abap, .clas.locals_def.abap, .clas.locals_imp.abap, .intf.abap, .fugr.abap, .func.abap, .ddls.asddls, .ddlx.asddlxs, .bdef.asbdef, .srvd.srvdsrv"),
		),
		mcp.WithString("package_name",
			mcp.Description("Target package name (required for new objects, not needed for class includes)"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number"),
		),
	), s.handleDeployFromFile) // Reuse existing handler
}

// registerExportToFile registers the ExportToFile tool (alias for SaveToFile)
func (s *Server) registerExportToFile() {
	s.mcpServer.AddTool(mcp.NewTool("ExportToFile",
		mcp.WithDescription("Export ABAP object from SAP system to local file. Saves source code with appropriate file extension. Supports: programs, classes (with includes), interfaces, function groups/modules, CDS views (DDLS), metadata extensions (DDLX), behavior definitions (BDEF), service definitions (SRVD). For classes, use 'include' parameter to export specific includes (testclasses, definitions, implementations, macros)."),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description("Object type: PROG, CLAS, INTF, FUGR, FUNC, DDLS, DDLX, BDEF, SRVD"),
		),
		mcp.WithString("object_name",
			mcp.Required(),
			mcp.Description("Object name"),
		),
		mcp.WithString("output_dir",
			mcp.Required(),
			mcp.Description("Output directory path (must exist)"),
		),
		mcp.WithString("include",
			mcp.Description("For CLAS only: include type to export. Values: main (default), testclasses, definitions, implementations, macros. Creates abapGit-compatible files (.clas.testclasses.abap, .clas.locals_def.abap, etc.)"),
		),
		mcp.WithString("parent",
			mcp.Description("Function group name (required for FUNC type)"),
		),
	), s.handleSaveToFile) // Reuse existing handler
}

// handleGrepObjects handles the unified GrepObjects tool call
func (s *Server) handleGrepObjects(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURLsRaw, ok := request.Params.Arguments["object_urls"].([]interface{})
	if !ok || len(objectURLsRaw) == 0 {
		return newToolResultError("object_urls array is required"), nil
	}

	// Convert []interface{} to []string
	objectURLs := make([]string, len(objectURLsRaw))
	for i, v := range objectURLsRaw {
		if s, ok := v.(string); ok {
			objectURLs[i] = s
		} else {
			return newToolResultError(fmt.Sprintf("object_urls[%d] must be a string", i)), nil
		}
	}

	pattern, ok := request.Params.Arguments["pattern"].(string)
	if !ok || pattern == "" {
		return newToolResultError("pattern is required"), nil
	}

	caseInsensitive := false
	if ci, ok := request.Params.Arguments["case_insensitive"].(bool); ok {
		caseInsensitive = ci
	}

	contextLines := 0
	if cl, ok := request.Params.Arguments["context_lines"].(float64); ok {
		contextLines = int(cl)
	}

	result, err := s.adtClient.GrepObjects(ctx, objectURLs, pattern, caseInsensitive, contextLines)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GrepObjects failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

// handleGrepPackages handles the unified GrepPackages tool call
func (s *Server) handleGrepPackages(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	packagesRaw, ok := request.Params.Arguments["packages"].([]interface{})
	if !ok || len(packagesRaw) == 0 {
		return newToolResultError("packages array is required"), nil
	}

	// Convert []interface{} to []string
	packages := make([]string, len(packagesRaw))
	for i, v := range packagesRaw {
		if s, ok := v.(string); ok {
			packages[i] = s
		} else {
			return newToolResultError(fmt.Sprintf("packages[%d] must be a string", i)), nil
		}
	}

	includeSubpackages := false
	if is, ok := request.Params.Arguments["include_subpackages"].(bool); ok {
		includeSubpackages = is
	}

	pattern, ok := request.Params.Arguments["pattern"].(string)
	if !ok || pattern == "" {
		return newToolResultError("pattern is required"), nil
	}

	caseInsensitive := false
	if ci, ok := request.Params.Arguments["case_insensitive"].(bool); ok {
		caseInsensitive = ci
	}

	var objectTypes []string
	if ot, ok := request.Params.Arguments["object_types"].([]interface{}); ok {
		objectTypes = make([]string, len(ot))
		for i, v := range ot {
			if s, ok := v.(string); ok {
				objectTypes[i] = s
			}
		}
	}

	maxResults := 0
	if mr, ok := request.Params.Arguments["max_results"].(float64); ok {
		maxResults = int(mr)
	}

	result, err := s.adtClient.GrepPackages(ctx, packages, includeSubpackages, pattern, caseInsensitive, objectTypes, maxResults)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GrepPackages failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}
