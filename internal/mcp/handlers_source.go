// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_source.go contains handlers for source code operations
// (GetSource, WriteSource, GrepObjects, GrepPackages, ImportFromFile, ExportToFile).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/ctxcomp"
)

// routeSourceAction routes "read" for GetSource and "edit" for WriteSource/EditSource.
func (s *Server) routeSourceAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action == "read" {
		// GetSource covers: CLAS, PROG, INTF, FUNC, FUGR, INCL, DDLS, BDEF, SRVD, MSAG, VIEW, ENHO
		switch objectType {
		case "CLAS", "PROG", "INTF", "FUNC", "FUGR", "INCL", "DDLS", "BDEF", "SRVD", "MSAG", "VIEW", "ENHO":
			args := map[string]any{
				"object_type": objectType,
				"name":        objectName,
			}
			if v := getStringParam(params, "parent"); v != "" {
				args["parent"] = v
			}
			if v := getStringParam(params, "include"); v != "" {
				args["include"] = v
			}
			if v := getStringParam(params, "method"); v != "" {
				args["method"] = v
			}
			if v, ok := getBoolParam(params, "include_context"); ok {
				args["include_context"] = v
			}
			if v, ok := getFloatParam(params, "max_deps"); ok {
				args["max_deps"] = v
			}
			if v, ok := getBoolParam(params, "merged"); ok {
				args["merged"] = v
			}
			return s.callHandler(ctx, s.handleGetSource, args)
		}
	}

	if action == "edit" {
		// High-level WriteSource
		switch objectType {
		case "CLAS", "PROG", "INTF", "DDLS", "BDEF", "SRVD":
			if src := getStringParam(params, "source"); src != "" {
				args := map[string]any{
					"object_type": objectType,
					"name":        objectName,
					"source":      src,
				}
				if v := getStringParam(params, "mode"); v != "" {
					args["mode"] = v
				}
				if v := getStringParam(params, "description"); v != "" {
					args["description"] = v
				}
				if v := getStringParam(params, "package"); v != "" {
					args["package"] = v
				}
				if v := getStringParam(params, "test_source"); v != "" {
					args["test_source"] = v
				}
				if v := getStringParam(params, "transport"); v != "" {
					args["transport"] = v
				}
				if v := getStringParam(params, "method"); v != "" {
					args["method"] = v
				}
				return s.callHandler(ctx, s.handleWriteSource, args)
			}
		case "EDITSOURCE":
			// Surgical edit via EditSource
			args := map[string]any{}
			for k, v := range params {
				args[k] = v
			}
			return s.callHandler(ctx, s.handleEditSource, args)
		}
	}

	return nil, false, nil
}

// registerGetSource registers the unified GetSource tool
func (s *Server) registerGetSource() {
	s.mcpServer.AddTool(mcp.NewTool("GetSource",
		mcp.WithDescription("Unified tool for reading ABAP source code across different object types. Replaces GetProgram, GetClass, GetInterface, GetFunction, GetInclude, GetFunctionGroup, GetClassInclude."),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description("Object type: PROG (program), CLAS (class), INTF (interface), FUNC (function module), FUGR (function group), INCL (include), DDLS (CDS DDL source), VIEW (DDIC view), BDEF (behavior definition), SRVD (service definition), SRVB (service binding), MSAG (message class)"),
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
		mcp.WithBoolean("include_context",
			mcp.Description("Append compressed dependency context showing public API contracts of referenced classes/interfaces/FMs (default: true). Set to false to get raw source only."),
		),
		mcp.WithNumber("max_deps",
			mcp.Description("Maximum dependencies to resolve when include_context=true (default: 20)"),
		),
	), s.handleGetSource)
}

// registerWriteSource registers the unified WriteSource tool
func (s *Server) registerWriteSource() {
	s.mcpServer.AddTool(mcp.NewTool("WriteSource",
		mcp.WithDescription("Unified tool for writing ABAP source code with automatic create/update detection. Supports PROG, CLAS, INTF, and RAP types (DDLS, BDEF, SRVD)."),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description("Object type: PROG (program), CLAS (class), INTF (interface), DDLS (CDS view), BDEF (behavior definition), SRVD (service definition)"),
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
	objectType, ok := request.GetArguments()["object_type"].(string)
	if !ok || objectType == "" {
		return newToolResultError("object_type is required"), nil
	}

	name, ok := request.GetArguments()["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}

	parent, _ := request.GetArguments()["parent"].(string)
	include, _ := request.GetArguments()["include"].(string)
	method, _ := request.GetArguments()["method"].(string)
	merged, _ := request.GetArguments()["merged"].(bool)

	opts := &adt.GetSourceOptions{
		Parent:  parent,
		Include: include,
		Method:  method,
		Merged:  merged,
	}

	source, err := s.adtClient.GetSource(ctx, objectType, name, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetSource failed: %v", err)), nil
	}

	// Append dependency context (default: true, set include_context=false to disable)
	includeContext := true
	if ic, ok := request.GetArguments()["include_context"].(bool); ok {
		includeContext = ic
	}
	if includeContext {
		maxDeps := 20
		if md, ok := request.GetArguments()["max_deps"].(float64); ok && md > 0 {
			maxDeps = int(md)
		}

		provider := ctxcomp.NewMultiSourceProvider("", &adtSourceAdapter{server: s})
		compressor := ctxcomp.NewCompressor(provider, maxDeps)
		result, err := compressor.Compress(ctx, source, name, objectType)
		if err == nil && result.Prologue != "" {
			source = source + "\n\n" + result.Prologue +
				fmt.Sprintf("\n* Context stats: %d deps found, %d resolved, %d failed",
					result.Stats.DepsFound, result.Stats.DepsResolved, result.Stats.DepsFailed)
		}
	}

	// Append "Enhancements attached" footer for INCL reads (and only when the
	// caller did not opt out of contextual enrichment via include_context=false).
	// Soft-fail: any lookup or fetch error stays a comment in the footer rather
	// than replacing the source the caller already has.
	if includeContext && strings.ToUpper(objectType) == "INCL" && !merged {
		source = appendEnhancementsFooter(ctx, s, source, name)
	}

	return mcp.NewToolResultText(source), nil
}

// appendEnhancementsFooter appends a "* === Enhancements attached ===" block
// listing each ENHO that targets the include. When source-body fetch succeeds,
// the body is rendered inline (via renderEnhBlock). When it fails — common on
// classic ECC where HOOK_IMPL bodies are not exposed via REST — a placeholder
// pointing at SE80 is rendered instead.
func appendEnhancementsFooter(ctx context.Context, s *Server, source, includeName string) string {
	refs, err := s.adtClient.ListEnhancementsForInclude(ctx, includeName)
	if err != nil || len(refs) == 0 {
		return source
	}

	var b strings.Builder
	b.WriteString("\n\n* === Enhancements attached to ")
	b.WriteString(strings.ToUpper(includeName))
	b.WriteString(fmt.Sprintf(" (%d) ===\n", len(refs)))
	for _, r := range refs {
		b.WriteString(fmt.Sprintf("* ENHO/%s %s", r.Kind, r.Name))
		if r.PackageName != "" {
			b.WriteString(fmt.Sprintf(" (package %s)", r.PackageName))
		}
		if r.Description != "" {
			b.WriteString(" — ")
			b.WriteString(r.Description)
		}
		b.WriteString("\n")
		if r.HostProgram != "" {
			b.WriteString("*   host: ")
			b.WriteString(r.HostProgram)
			if r.EnhInclude != "" {
				b.WriteString("  (plugin source: ")
				b.WriteString(r.EnhInclude)
				b.WriteString(")")
			}
			b.WriteString("\n")
		}
		if r.FullName != "" {
			b.WriteString("*   anchor: ")
			b.WriteString(r.FullName)
			b.WriteString("\n")
		}

		// Pass the ref by pointer so EnhInclude (populated by the ENHINCINX
		// table fallback) survives the body fetch. GetEnhancement(name) would
		// re-resolve via SearchObject and drop EnhInclude, forcing the RFC
		// step to guess <NAME>E — which fails for HOOK_IMPL plug-ins whose
		// REPOSRC names use `=`-padding (ISM_SAPLVKMP==================E).
		refCopy := r
		body, ferr := s.adtClient.GetEnhancementByRef(ctx, &refCopy)
		if ferr != nil {
			b.WriteString(fmt.Sprintf("*   [source body unavailable: %v]\n", ferr))
			continue
		}
		b.WriteString(adtRenderEnhBlock(r, body))
	}
	return source + b.String()
}

// adtRenderEnhBlock renders an ENHO block in the same shape as the package-
// internal renderEnhBlock helper. Inlined here because the helper is
// unexported in pkg/adt; keeping a minimal copy avoids an export-only diff.
func adtRenderEnhBlock(ref adt.EnhancementRef, source string) string {
	var b strings.Builder
	kind := string(ref.Kind)
	if kind == "" {
		kind = "?"
	}
	b.WriteString("\n* vvv ENHO/")
	b.WriteString(kind)
	b.WriteString(" ")
	b.WriteString(ref.Name)
	if ref.PackageName != "" {
		b.WriteString(" (package ")
		b.WriteString(ref.PackageName)
		b.WriteString(")")
	}
	b.WriteString(" vvv\n")
	b.WriteString(source)
	if !strings.HasSuffix(source, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("* ^^^ end of ")
	b.WriteString(ref.Name)
	b.WriteString(" ^^^\n")
	return b.String()
}

// handleWriteSource handles the unified WriteSource tool call
func (s *Server) handleWriteSource(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectType, ok := request.GetArguments()["object_type"].(string)
	if !ok || objectType == "" {
		return newToolResultError("object_type is required"), nil
	}

	name, ok := request.GetArguments()["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}

	source, ok := request.GetArguments()["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	mode, _ := request.GetArguments()["mode"].(string)
	description, _ := request.GetArguments()["description"].(string)
	packageName, _ := request.GetArguments()["package"].(string)
	testSource, _ := request.GetArguments()["test_source"].(string)
	transport, _ := request.GetArguments()["transport"].(string)
	method, _ := request.GetArguments()["method"].(string)

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
		mcp.WithDescription("Import ABAP object from local file into SAP system. Auto-detects object type from file extension, creates or updates, activates. Supports: programs, classes (with includes), interfaces, function groups/modules, CDS views (DDLS), behavior definitions (BDEF), service definitions (SRVD). For class includes (.clas.testclasses.abap, .clas.locals_def.abap, etc.), the parent class must exist."),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Absolute path to ABAP source file. Supported extensions: .prog.abap, .clas.abap, .clas.testclasses.abap, .clas.locals_def.abap, .clas.locals_imp.abap, .intf.abap, .fugr.abap, .func.abap, .ddls.asddls, .bdef.asbdef, .srvd.srvdsrv"),
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
		mcp.WithDescription("Export ABAP object from SAP system to local file. Saves source code with appropriate file extension. Supports: programs, classes (with includes), interfaces, function groups/modules, CDS views (DDLS), behavior definitions (BDEF), service definitions (SRVD). For classes, use 'include' parameter to export specific includes (testclasses, definitions, implementations, macros)."),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description("Object type: PROG, CLAS, INTF, FUGR, FUNC, DDLS, BDEF, SRVD"),
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
	objectURLsRaw, ok := request.GetArguments()["object_urls"].([]interface{})
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

	pattern, ok := request.GetArguments()["pattern"].(string)
	if !ok || pattern == "" {
		return newToolResultError("pattern is required"), nil
	}

	caseInsensitive := false
	if ci, ok := request.GetArguments()["case_insensitive"].(bool); ok {
		caseInsensitive = ci
	}

	contextLines := 0
	if cl, ok := request.GetArguments()["context_lines"].(float64); ok {
		contextLines = int(cl)
	}

	includeEnhancements := readIncludeEnhancementsFlag(request.GetArguments())
	maxEnhancements := readMaxEnhancementsParam(request.GetArguments())

	var (
		result interface{}
		err    error
	)
	if includeEnhancements {
		result, err = s.adtClient.GrepObjectsWithEnhancements(ctx, objectURLs, pattern, caseInsensitive, contextLines, maxEnhancements)
	} else {
		result, err = s.adtClient.GrepObjects(ctx, objectURLs, pattern, caseInsensitive, contextLines)
	}
	if err != nil {
		return newToolResultError(fmt.Sprintf("GrepObjects failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

// handleGrepPackages handles the unified GrepPackages tool call
func (s *Server) handleGrepPackages(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	packagesRaw, ok := request.GetArguments()["packages"].([]interface{})
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
	if is, ok := request.GetArguments()["include_subpackages"].(bool); ok {
		includeSubpackages = is
	}

	pattern, ok := request.GetArguments()["pattern"].(string)
	if !ok || pattern == "" {
		return newToolResultError("pattern is required"), nil
	}

	caseInsensitive := false
	if ci, ok := request.GetArguments()["case_insensitive"].(bool); ok {
		caseInsensitive = ci
	}

	var objectTypes []string
	if ot, ok := request.GetArguments()["object_types"].([]interface{}); ok {
		objectTypes = make([]string, len(ot))
		for i, v := range ot {
			if s, ok := v.(string); ok {
				objectTypes[i] = s
			}
		}
	}

	maxResults := 0
	if mr, ok := request.GetArguments()["max_results"].(float64); ok {
		maxResults = int(mr)
	}

	includeEnhancements := readIncludeEnhancementsFlag(request.GetArguments())
	maxEnhancements := readMaxEnhancementsParam(request.GetArguments())

	var (
		result interface{}
		err    error
	)
	if includeEnhancements {
		result, err = s.adtClient.GrepPackagesWithEnhancements(ctx, packages, includeSubpackages, pattern, caseInsensitive, objectTypes, maxResults, maxEnhancements)
	} else {
		result, err = s.adtClient.GrepPackages(ctx, packages, includeSubpackages, pattern, caseInsensitive, objectTypes, maxResults)
	}
	if err != nil {
		return newToolResultError(fmt.Sprintf("GrepPackages failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}
