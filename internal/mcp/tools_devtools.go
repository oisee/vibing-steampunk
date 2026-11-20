// Package mcp provides the MCP server implementation for ABAP ADT tools.
// tools_devtools.go registers syntax check, activation, ATC, unit test and code quality tools.
package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// registerDevTools registers development tools (syntax check, activate, ATC, etc.)
func (s *Server) registerDevTools(shouldRegister func(string) bool) {
	if shouldRegister("SyntaxCheck") {
		s.mcpServer.AddTool(mcp.NewTool("SyntaxCheck",
			mcp.WithDescription("Check ABAP source code for syntax errors"),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
			),
			mcp.WithString("content",
				mcp.Required(),
				mcp.Description("ABAP source code to check"),
			),
		), s.handleSyntaxCheck)
	}

	if shouldRegister("Activate") {
		s.mcpServer.AddTool(mcp.NewTool("Activate",
			mcp.WithDescription("Activate an ABAP object"),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
			),
			mcp.WithString("object_name",
				mcp.Required(),
				mcp.Description("Technical name of the object (e.g., ZTEST)"),
			),
			mcp.WithNumber("timeout",
				mcp.Description(callTimeoutDescription),
			),
		), s.handleActivate)
	}

	if shouldRegister("ActivateMultiple") {
		s.mcpServer.AddTool(mcp.NewTool("ActivateMultiple",
			mcp.WithDescription("Activate multiple ABAP objects in a single ADT request, resolving mutual dependencies between them (same behaviour as Eclipse ADT). Use when includes and their main program must be activated together."),
			mcp.WithArray("objects",
				mcp.Required(),
				mcp.Description(`Objects to activate. Each item can be:
  - {"url": "/sap/bc/adt/programs/programs/zprog", "name": "ZPROG"}
  - "TYPE NAME" shorthand, e.g. "PROG ZPROG", "INCL ZPROG_TOP", "CLAS ZCL_X"`),
			),
			mcp.WithNumber("timeout",
				mcp.Description(callTimeoutDescription),
			),
		), s.handleActivateMultiple)
	}

	if shouldRegister("ActivatePackage") {
		s.mcpServer.AddTool(mcp.NewTool("ActivatePackage",
			mcp.WithDescription("Activate all inactive objects. Objects are sorted by dependency order (interfaces before classes). If no package specified, activates ALL inactive objects for current user."),
			mcp.WithString("package",
				mcp.Description("Package name to filter (optional, empty = all packages)"),
			),
			mcp.WithNumber("max_objects",
				mcp.Description("Maximum number of objects to activate (default: 100)"),
			),
			mcp.WithNumber("timeout",
				mcp.Description(callTimeoutDescription),
			),
		), s.handleActivatePackage)
	}

	if shouldRegister("RunUnitTests") {
		s.mcpServer.AddTool(mcp.NewTool("RunUnitTests",
			mcp.WithDescription("Run ABAP Unit tests for an object. Answers JSON: ok (a test method ran, nothing failed and every test class ran), counts {classes, methods, passed, failed, classFailures, warnings, notRun}, notRunClasses (test classes ABAP Unit listed but did not run, e.g. for their risk level), and classes (each with name, parentName, alerts filed on the class, and testMethods with name and alerts: kind, severity, title, details). only_failures lists just the failed methods and classes with alerts, without URIs or stacks."),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/oo/classes/ZCL_TEST)"),
			),
			mcp.WithBoolean("include_dangerous",
				mcp.Description("Include dangerous risk level tests (default: false)"),
			),
			mcp.WithBoolean("include_long",
				mcp.Description("Include long duration tests (default: false)"),
			),
			mcp.WithNumber("timeout",
				mcp.Description(callTimeoutDescription),
			),
			mcp.WithBoolean("only_failures",
				mcp.Description("List only failed test methods (and classes with alerts of their own), plus the counts for the whole run (default: false)"),
			),
		), s.handleRunUnitTests)
	}

	if shouldRegister("RunATCCheck") {
		s.mcpServer.AddTool(mcp.NewTool("RunATCCheck",
			mcp.WithDescription("Run ATC (ABAP Test Cockpit) code quality check on an object. Returns findings with priority, check title, message, and location. Priority: 1=Error, 2=Warning, 3=Info."),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/oo/classes/ZCL_TEST)"),
			),
			mcp.WithString("variant",
				mcp.Description("Check variant name (empty = use system default)"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of findings to return (default: 100)"),
			),
			mcp.WithNumber("timeout",
				mcp.Description(callTimeoutDescription),
			),
		), s.handleRunATCCheck)
	}

	if shouldRegister("GetATCCustomizing") {
		s.mcpServer.AddTool(mcp.NewTool("GetATCCustomizing",
			mcp.WithDescription("Get ATC system configuration including default check variant and exemption reasons"),
		), s.handleGetATCCustomizing)
	}

	if shouldRegister("PrettyPrint") {
		s.mcpServer.AddTool(mcp.NewTool("PrettyPrint",
			mcp.WithDescription("Format ABAP source code using the pretty printer"),
			mcp.WithString("source",
				mcp.Required(),
				mcp.Description("ABAP source code to format"),
			),
		), s.handlePrettyPrint)
	}

	if shouldRegister("GetPrettyPrinterSettings") {
		s.mcpServer.AddTool(mcp.NewTool("GetPrettyPrinterSettings",
			mcp.WithDescription("Get the current pretty printer (code formatter) settings"),
		), s.handleGetPrettyPrinterSettings)
	}

	if shouldRegister("SetPrettyPrinterSettings") {
		s.mcpServer.AddTool(mcp.NewTool("SetPrettyPrinterSettings",
			mcp.WithDescription("Update the pretty printer (code formatter) settings"),
			mcp.WithBoolean("indentation",
				mcp.Required(),
				mcp.Description("Enable automatic indentation"),
			),
			mcp.WithString("style",
				mcp.Required(),
				mcp.Description("Keyword style: toLower, toUpper, keywordUpper, keywordLower, keywordAuto, none"),
			),
		), s.handleSetPrettyPrinterSettings)
	}

	if shouldRegister("GetInactiveObjects") {
		s.mcpServer.AddTool(mcp.NewTool("GetInactiveObjects",
			mcp.WithDescription("Get all inactive objects for the current user - objects that have been modified but not yet activated"),
		), s.handleGetInactiveObjects)
	}

	// ExecuteABAP - execute arbitrary ABAP code via unit test wrapper (Expert mode only)
	if shouldRegister("ExecuteABAP") {
		s.mcpServer.AddTool(mcp.NewTool("ExecuteABAP",
			mcp.WithDescription("Execute arbitrary ABAP code via unit test wrapper. Creates temp program, injects code into test method, runs via RunUnitTests, extracts results from assertion messages, cleans up. Use lv_result variable to return output, or call RETURN_VALUE( x ) once per value to return several (handed back at once, so a later RETURN or CHECK keeps it; structures and tables come back as JSON). Answers JSON; result_text holds the full value (an array for several). WARNING: Powerful tool - use responsibly."),
			mcp.WithString("code",
				mcp.Required(),
				mcp.Description("ABAP code to execute. Set lv_result, or call RETURN_VALUE( x ) for each value to return."),
			),
			mcp.WithString("risk_level",
				mcp.Description("Risk level: harmless (default, no DB writes), dangerous (can write to DB), critical (full access)"),
			),
			mcp.WithString("return_variable",
				mcp.Description("Name of the variable to return (default: lv_result)"),
			),
			mcp.WithBoolean("keep_program",
				mcp.Description("Don't delete temp program after execution (for debugging)"),
			),
			mcp.WithString("program_prefix",
				mcp.Description("Prefix for temp program name (default: ZTEMP_EXEC_)"),
			),
			mcp.WithNumber("timeout",
				mcp.Description(callTimeoutDescription),
			),
		), s.handleExecuteABAP)
	}
}

// registerTestingQualityTools registers testing and code quality tools.
func (s *Server) registerTestingQualityTools(shouldRegister func(string) bool) {
	if shouldRegister("GetCodeCoverage") {
		s.mcpServer.AddTool(mcp.NewTool("GetCodeCoverage",
			mcp.WithDescription("Run ABAP Unit tests with code coverage enabled. Returns line-level statement, branch, and procedure coverage data per source file. Uses the same test runner as RunUnitTests but with coverage capture active."),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT URL of object to test (e.g., /sap/bc/adt/oo/classes/ZCL_TEST)"),
			),
			mcp.WithBoolean("include_dangerous",
				mcp.Description("Include tests with risk level 'dangerous' (default: false)"),
			),
			mcp.WithBoolean("include_long",
				mcp.Description("Include tests with duration 'long' (default: false)"),
			),
			mcp.WithNumber("timeout",
				mcp.Description(callTimeoutDescription),
			),
		), s.handleGetCodeCoverage)
	}

	if shouldRegister("GetCheckRunResults") {
		s.mcpServer.AddTool(mcp.NewTool("GetCheckRunResults",
			mcp.WithDescription("Get detailed results for a specific check run. Returns all messages with line numbers, severity (E=Error, W=Warning, I=Info), and summary counts. Use after SyntaxCheck or other check operations to get comprehensive error details."),
			mcp.WithString("check_run_id",
				mcp.Required(),
				mcp.Description("Check run ID (from SyntaxCheck or other check operation)"),
			),
		), s.handleGetCheckRunResults)
	}

	if shouldRegister("AnalyzeABAPCode") {
		s.mcpServer.AddTool(mcp.NewTool("AnalyzeABAPCode",
			mcp.WithDescription("Analyze ABAP source code for quality, performance, security, and robustness issues using the native Go abaplint engine. Provide source directly or specify object_type + object_name to fetch from SAP. Returns findings with severity, category, line numbers, and fix suggestions."),
			mcp.WithString("object_type",
				mcp.Description("ADT object type (e.g., PROG, CLAS, FUGR) — required when using object_name"),
			),
			mcp.WithString("object_name",
				mcp.Description("ABAP object name (e.g., ZTEST, ZCL_MY_CLASS) — fetches source from SAP"),
			),
			mcp.WithString("source",
				mcp.Description("ABAP source code to analyze directly (alternative to object_name)"),
			),
		), s.handleAnalyzeABAPCode)
	}
}
