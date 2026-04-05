// Package mcp provides the MCP server implementation for ABAP ADT tools.
// tools_register.go contains all tool registration logic (AddTool calls).
package mcp

import (
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// registerTools registers ADT tools with the MCP server based on mode, disabled groups, and granular config.
// Mode "focused" registers essential tools.
// Mode "expert" registers all tools.
// DisabledGroups can disable specific tool groups using short codes:
//   - "5" or "U" = UI5/BSP tools (3 tools, read-only)
//   - "T" = Test tools: RunUnitTests, RunATCCheck (2 tools)
//   - "H" = HANA/AMDP debugger (7 tools)
//   - "D" = ABAP Debugger (6 session tools)
//   - "C" = CTS/Transport tools (5 tools)
//   - "G" = Git/abapGit tools (2 tools)
//   - "R" = Report tools (4 tools)
//   - "I" = Install tools (4 tools)
//   - "X" = EXPERIMENTAL: All debugger + RunReport (17 tools) - use to disable unreliable features
//
// toolsConfig from .vsp.json has highest priority:
//   - If tool is explicitly disabled (false), it will NOT be registered
//   - If tool is explicitly enabled (true), it WILL be registered (overrides focused mode)
//   - If tool is not in config, mode/disabledGroups rules apply
func (s *Server) registerTools(mode string, disabledGroups string, toolsConfig map[string]bool) {
	// Hyperfocused mode: single universal SAP tool
	if mode == "hyperfocused" {
		s.registerUniversalTool()
		return
	}

	groups := toolGroups()
	focusedTools := focusedToolSet()

	// Build set of disabled tools based on disabledGroups string
	disabledTools := make(map[string]bool)
	for _, code := range strings.ToUpper(disabledGroups) {
		if tools, ok := groups[string(code)]; ok {
			for _, tool := range tools {
				disabledTools[tool] = true
			}
		}
	}

	// Helper to check if tool should be registered
	shouldRegister := func(toolName string) bool {
		// Priority 1: Check granular tool config from .vsp.json (highest priority)
		if toolsConfig != nil {
			if enabled, exists := toolsConfig[toolName]; exists {
				return enabled // Explicit config overrides everything
			}
		}
		// Priority 2: Check if tool is disabled by group
		if disabledTools[toolName] {
			return false
		}
		// Priority 3: Check mode
		if mode == "expert" {
			return true // Expert mode: register all tools (except disabled)
		}
		return focusedTools[toolName] // Focused mode: only whitelisted tools (except disabled)
	}

	// Register all tools
	s.registerUnifiedTools(shouldRegister)
	s.registerReadTools(shouldRegister)
	s.registerSystemTools(shouldRegister)
	s.registerAnalysisTools(shouldRegister)
	s.registerDiagnosticsTools(shouldRegister)
	s.registerDebuggerTools(shouldRegister)
	s.registerSearchTools(shouldRegister)
	s.registerDevTools(shouldRegister)
	s.registerCRUDTools(shouldRegister)
	s.registerClassIncludeTools(shouldRegister)
	s.registerWorkflowTools(shouldRegister)
	s.registerFileTools(shouldRegister)
	s.registerEditTools(shouldRegister)
	s.registerGrepTools(shouldRegister)
	s.registerCodeIntelTools(shouldRegister)
	s.registerUI5Tools(shouldRegister)
	s.registerAMDPTools(shouldRegister)
	s.registerTransportTools(shouldRegister)
	s.registerGitTools(shouldRegister)
	s.registerReportTools(shouldRegister)
	s.registerInstallTools(shouldRegister)
	s.registerVersionHistoryTools(shouldRegister)
	s.registerTestingQualityTools(shouldRegister)
	s.registerCodeAnalysisTools(shouldRegister)
	s.registerI18NTools(shouldRegister)

	// Register tool aliases for common operations
	s.registerToolAliases(shouldRegister)
}

// registerUnifiedTools registers the unified GetSource/WriteSource tools.
func (s *Server) registerUnifiedTools(shouldRegister func(string) bool) {
	if shouldRegister("GetSource") {
		s.registerGetSource()
	}
	if shouldRegister("WriteSource") {
		s.registerWriteSource()
	}
}

// registerReadTools registers object read tools (GetProgram, GetClass, etc.)
func (s *Server) registerReadTools(shouldRegister func(string) bool) {
	if shouldRegister("GetProgram") {
		s.mcpServer.AddTool(mcp.NewTool("GetProgram",
			mcp.WithDescription("Retrieve ABAP program source code"),
			mcp.WithString("program_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP program"),
			),
		), s.handleGetProgram)
	}

	if shouldRegister("GetClass") {
		s.mcpServer.AddTool(mcp.NewTool("GetClass",
			mcp.WithDescription("Retrieve ABAP class source code"),
			mcp.WithString("class_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP class"),
			),
		), s.handleGetClass)
	}

	if shouldRegister("GetInterface") {
		s.mcpServer.AddTool(mcp.NewTool("GetInterface",
			mcp.WithDescription("Retrieve ABAP interface source code"),
			mcp.WithString("interface_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP interface"),
			),
		), s.handleGetInterface)
	}

	if shouldRegister("GetFunction") {
		s.mcpServer.AddTool(mcp.NewTool("GetFunction",
			mcp.WithDescription("Retrieve ABAP Function Module source code"),
			mcp.WithString("function_name",
				mcp.Required(),
				mcp.Description("Name of the function module"),
			),
			mcp.WithString("function_group",
				mcp.Required(),
				mcp.Description("Name of the function group"),
			),
		), s.handleGetFunction)
	}

	if shouldRegister("GetFunctionGroup") {
		s.mcpServer.AddTool(mcp.NewTool("GetFunctionGroup",
			mcp.WithDescription("Retrieve ABAP Function Group source code"),
			mcp.WithString("function_group",
				mcp.Required(),
				mcp.Description("Name of the function group"),
			),
		), s.handleGetFunctionGroup)
	}

	if shouldRegister("GetInclude") {
		s.mcpServer.AddTool(mcp.NewTool("GetInclude",
			mcp.WithDescription("Retrieve ABAP Include Source Code"),
			mcp.WithString("include_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP Include"),
			),
		), s.handleGetInclude)
	}

	if shouldRegister("GetTable") {
		s.mcpServer.AddTool(mcp.NewTool("GetTable",
			mcp.WithDescription("Retrieve ABAP table structure"),
			mcp.WithString("table_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP table"),
			),
		), s.handleGetTable)
	}

	if shouldRegister("GetTableContents") {
		s.mcpServer.AddTool(mcp.NewTool("GetTableContents",
			mcp.WithDescription("Retrieve contents of an ABAP table. For simple queries use table_name + max_rows. For filtered queries use sql_query parameter with ABAP SQL syntax (use ASCENDING/DESCENDING, not ASC/DESC)."),
			mcp.WithString("table_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP table"),
			),
			mcp.WithNumber("max_rows",
				mcp.Description("Maximum number of rows to retrieve (default 100). Use this instead of SQL LIMIT clause"),
			),
			mcp.WithString("sql_query",
				mcp.Description("Optional ABAP SQL SELECT statement. Uses ABAP syntax: ASCENDING/DESCENDING work, ASC/DESC fail. Example: SELECT * FROM T000 WHERE MANDT = '001' ORDER BY MANDT DESCENDING"),
			),
			mcp.WithNumber("offset",
				mcp.Description("Skip first N rows (client-side pagination). Use with max_rows for paging through results"),
			),
			mcp.WithBoolean("columns_only",
				mcp.Description("Return only column metadata (names, types, lengths) without data rows"),
			),
		), s.handleGetTableContents)
	}

	if shouldRegister("RunQuery") {
		s.mcpServer.AddTool(mcp.NewTool("RunQuery",
			mcp.WithDescription("Execute a freestyle SQL query against the SAP database. IMPORTANT: Uses ABAP SQL syntax, NOT standard SQL. Use ASCENDING/DESCENDING instead of ASC/DESC. Use max_rows parameter instead of LIMIT. GROUP BY and WHERE work normally."),
			mcp.WithString("sql_query",
				mcp.Required(),
				mcp.Description("ABAP SQL query. Example: SELECT carrid, COUNT(*) as cnt FROM sflight GROUP BY carrid ORDER BY cnt DESCENDING. Note: ASC/DESC keywords fail - use ASCENDING/DESCENDING"),
			),
			mcp.WithNumber("max_rows",
				mcp.Description("Maximum number of rows to retrieve (default 100). Use this instead of SQL LIMIT clause"),
			),
		), s.handleRunQuery)
	}

	if shouldRegister("GetCDSDependencies") {
		s.mcpServer.AddTool(mcp.NewTool("GetCDSDependencies",
			mcp.WithDescription("Retrieve CDS view FORWARD dependencies (tables/views this CDS reads FROM). Returns tree of base objects. Does NOT return reverse dependencies (where-used). Use with GetSource(DDLS) to read CDS source code."),
			mcp.WithString("ddls_name",
				mcp.Required(),
				mcp.Description("CDS DDL source name (e.g., 'ZRAY_00_I_DOC_NODE_00'). Use SearchObject to find CDS views first."),
			),
			mcp.WithString("dependency_level",
				mcp.Description("Level of dependency resolution: 'unit' (direct only) or 'hierarchy' (recursive). Default: 'hierarchy'"),
			),
			mcp.WithBoolean("with_associations",
				mcp.Description("Include modeled associations in dependency tree. Default: false"),
			),
			mcp.WithString("context_package",
				mcp.Description("Filter dependencies to specific package context"),
			),
		), s.handleGetCDSDependencies)
	}

	if shouldRegister("GetCDSImpactAnalysis") {
		s.mcpServer.AddTool(mcp.NewTool("GetCDSImpactAnalysis",
			mcp.WithDescription("Retrieve CDS view REVERSE dependencies (where-used / downstream consumers). Returns all objects that consume/reference the given CDS view. Complement of GetCDSDependencies which shows forward dependencies."),
			mcp.WithString("view_name",
				mcp.Required(),
				mcp.Description("CDS view name (e.g., 'ZTRAVEL'). Use SearchObject to find CDS views first."),
			),
		), s.handleGetCDSImpactAnalysis)
	}

	if shouldRegister("GetCDSElementInfo") {
		s.mcpServer.AddTool(mcp.NewTool("GetCDSElementInfo",
			mcp.WithDescription("Retrieve metadata for all elements (fields) of a CDS view. Returns field names, types, annotations, and semantic information from the DDL source."),
			mcp.WithString("view_name",
				mcp.Required(),
				mcp.Description("CDS view name (e.g., 'ZTRAVEL'). Use SearchObject to find CDS views first."),
			),
		), s.handleGetCDSElementInfo)
	}

	if shouldRegister("GetStructure") {
		s.mcpServer.AddTool(mcp.NewTool("GetStructure",
			mcp.WithDescription("Retrieve ABAP Structure"),
			mcp.WithString("structure_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP Structure"),
			),
		), s.handleGetStructure)
	}

	if shouldRegister("GetPackage") {
		s.mcpServer.AddTool(mcp.NewTool("GetPackage",
			mcp.WithDescription("Retrieve ABAP package details"),
			mcp.WithString("package_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP package"),
			),
		), s.handleGetPackage)
	}

	if shouldRegister("GetMessages") {
		s.mcpServer.AddTool(mcp.NewTool("GetMessages",
			mcp.WithDescription("Get all messages from an ABAP message class (SE91). Returns message number, text for all messages in the class. Use SearchObject to find message classes first."),
			mcp.WithString("message_class",
				mcp.Required(),
				mcp.Description("Name of the message class (e.g., 'ZRAY_00', 'SY')"),
			),
		), s.handleGetMessages)
	}

	if shouldRegister("GetTransaction") {
		s.mcpServer.AddTool(mcp.NewTool("GetTransaction",
			mcp.WithDescription("Retrieve ABAP transaction details"),
			mcp.WithString("transaction_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP transaction"),
			),
		), s.handleGetTransaction)
	}

	if shouldRegister("GetTypeInfo") {
		s.mcpServer.AddTool(mcp.NewTool("GetTypeInfo",
			mcp.WithDescription("Retrieve ABAP type information"),
			mcp.WithString("type_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP type"),
			),
		), s.handleGetTypeInfo)
	}

	if shouldRegister("GetAPIReleaseState") {
		s.mcpServer.AddTool(mcp.NewTool("GetAPIReleaseState",
			mcp.WithDescription("Check API release state for S/4HANA Clean Core / ABAP Cloud compatibility. Returns whether an object is released for cloud development and key user apps. Use SearchObject first to get the object URI."),
			mcp.WithString("object_uri",
				mcp.Required(),
				mcp.Description("ADT URI of the object (e.g., /sap/bc/adt/oo/classes/cl_abap_typedescr). Get this from SearchObject results."),
			),
		), s.handleGetAPIReleaseState)
	}
}

// registerSystemTools registers system information and always-on tools.
func (s *Server) registerSystemTools(shouldRegister func(string) bool) {
	if shouldRegister("GetSystemInfo") {
		s.mcpServer.AddTool(mcp.NewTool("GetSystemInfo",
			mcp.WithDescription("Get SAP system information (system ID, release, kernel, database)"),
		), s.handleGetSystemInfo)
	}

	if shouldRegister("GetInstalledComponents") {
		s.mcpServer.AddTool(mcp.NewTool("GetInstalledComponents",
			mcp.WithDescription("List installed software components with version information"),
		), s.handleGetInstalledComponents)
	}

	// GetConnectionInfo - Self-inspection tool
	// Always registered - useful for debugging and introspection
	s.mcpServer.AddTool(mcp.NewTool("GetConnectionInfo",
		mcp.WithDescription("Get current MCP connection info: user, URL, client. Useful for debugging and understanding current session context."),
	), s.handleGetConnectionInfo)

	// GetFeatures - Feature Detection (Safety Network)
	// Always registered - provides visibility into what's available
	s.mcpServer.AddTool(mcp.NewTool("GetFeatures",
		mcp.WithDescription("Probe SAP system for available features. Returns status of optional capabilities like abapGit, RAP/OData, AMDP debugging, UI5/BSP, and CTS transports. Use this to understand what features are available before attempting to use them."),
	), s.handleGetFeatures)

	// GetAbapHelp - ABAP Keyword Documentation
	// Always registered - provides URL and search query, optionally real docs via ZADT_VSP
	s.mcpServer.AddTool(mcp.NewTool("GetAbapHelp",
		mcp.WithDescription("Get ABAP keyword documentation. Returns URL to SAP Help Portal and search query. If ZADT_VSP is installed, also returns real documentation from SAP system."),
		mcp.WithString("keyword",
			mcp.Required(),
			mcp.Description("ABAP keyword (e.g., SELECT, LOOP, DATA, METHOD, READ TABLE)"),
		),
	), s.handleGetAbapHelp)
}

// registerAnalysisTools registers code analysis infrastructure tools.
func (s *Server) registerAnalysisTools(shouldRegister func(string) bool) {
	if shouldRegister("GetCallGraph") {
		s.mcpServer.AddTool(mcp.NewTool("GetCallGraph",
			mcp.WithDescription("Get call hierarchy for methods/functions. Shows callers or callees of an ABAP object."),
			mcp.WithString("object_uri",
				mcp.Required(),
				mcp.Description("ADT URI of the object (e.g., /sap/bc/adt/oo/classes/ZCL_TEST/source/main#start=10,1)"),
			),
			mcp.WithString("direction",
				mcp.Description("Direction: 'callers' (who calls this) or 'callees' (what this calls). Default: callers"),
			),
			mcp.WithNumber("max_depth",
				mcp.Description("Maximum depth of call hierarchy (default: 3)"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of results (default: 100)"),
			),
		), s.handleGetCallGraph)
	}

	if shouldRegister("GetObjectStructure") {
		s.mcpServer.AddTool(mcp.NewTool("GetObjectStructure",
			mcp.WithDescription("Get object explorer tree structure. Returns hierarchical view of object components."),
			mcp.WithString("object_name",
				mcp.Required(),
				mcp.Description("Object name (e.g., ZCL_TEST, ZPROGRAM)"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of results (default: 100)"),
			),
		), s.handleGetObjectStructure)
	}

	if shouldRegister("GetCallersOf") {
		s.mcpServer.AddTool(mcp.NewTool("GetCallersOf",
			mcp.WithDescription("Find all callers of an ABAP object (up traversal). Shows who calls this method/function. Simplified wrapper around GetCallGraph."),
			mcp.WithString("object_uri",
				mcp.Required(),
				mcp.Description("ADT URI of the object (e.g., /sap/bc/adt/oo/classes/ZCL_TEST/source/main#start=10,1)"),
			),
			mcp.WithNumber("max_depth",
				mcp.Description("Maximum depth of caller hierarchy (default: 5)"),
			),
		), s.handleGetCallersOf)
	}

	if shouldRegister("GetCalleesOf") {
		s.mcpServer.AddTool(mcp.NewTool("GetCalleesOf",
			mcp.WithDescription("Find all callees of an ABAP object (down traversal). Shows what this method/function calls. Simplified wrapper around GetCallGraph."),
			mcp.WithString("object_uri",
				mcp.Required(),
				mcp.Description("ADT URI of the object (e.g., /sap/bc/adt/oo/classes/ZCL_TEST/source/main#start=10,1)"),
			),
			mcp.WithNumber("max_depth",
				mcp.Description("Maximum depth of callee hierarchy (default: 5)"),
			),
		), s.handleGetCalleesOf)
	}

	if shouldRegister("AnalyzeCallGraph") {
		s.mcpServer.AddTool(mcp.NewTool("AnalyzeCallGraph",
			mcp.WithDescription("Analyze call graph for an object. Returns statistics: total nodes, edges, max depth, nodes by type. Use for understanding code complexity and dependencies."),
			mcp.WithString("object_uri",
				mcp.Required(),
				mcp.Description("ADT URI of the object to analyze"),
			),
			mcp.WithString("direction",
				mcp.Description("Direction: 'callers' or 'callees' (default: callees)"),
			),
			mcp.WithNumber("max_depth",
				mcp.Description("Maximum depth to analyze (default: 5)"),
			),
		), s.handleAnalyzeCallGraph)
	}

	if shouldRegister("CompareCallGraphs") {
		s.mcpServer.AddTool(mcp.NewTool("CompareCallGraphs",
			mcp.WithDescription("Compare static call graph with actual execution trace. Identifies: common paths, untested paths (static only), and dynamic calls (actual only). Use for test coverage analysis and RCA."),
			mcp.WithString("object_uri",
				mcp.Required(),
				mcp.Description("ADT URI of the root object"),
			),
			mcp.WithString("trace_data",
				mcp.Required(),
				mcp.Description("JSON array of trace edges from execution (format: [{caller_name, callee_name}, ...])"),
			),
		), s.handleCompareCallGraphs)
	}

	if shouldRegister("TraceExecution") {
		s.mcpServer.AddTool(mcp.NewTool("TraceExecution",
			mcp.WithDescription("COMPOSITE RCA TOOL: Performs traced execution analysis. 1) Builds static call graph from object, 2) Optionally runs unit tests, 3) Collects trace data, 4) Extracts actual call edges, 5) Compares static vs actual for root cause analysis."),
			mcp.WithString("object_uri",
				mcp.Required(),
				mcp.Description("ADT URI of the starting object for static call graph"),
			),
			mcp.WithNumber("max_depth",
				mcp.Description("Maximum depth for call graph traversal (default: 5)"),
			),
			mcp.WithBoolean("run_tests",
				mcp.Description("Run unit tests before collecting trace (default: false)"),
			),
			mcp.WithString("test_object_uri",
				mcp.Description("Object URI for tests to run (defaults to object_uri)"),
			),
			mcp.WithString("trace_user",
				mcp.Description("Filter traces by user (defaults to current user)"),
			),
		), s.handleTraceExecution)
	}

	// --- Graph Engine: Dependency & Boundary Analysis ---

	if shouldRegister("CheckBoundaries") {
		s.mcpServer.AddTool(mcp.NewTool("CheckBoundaries",
			mcp.WithDescription("Analyze package boundary violations. Checks if a package/object's dependencies cross package boundaries. Detects: same-package deps, SAP standard deps, whitelisted Z-package deps, violations (cross-package Z* deps), and dynamic calls. Uses embedded ABAP parser (offline) + TADIR resolution (online)."),
			mcp.WithString("package",
				mcp.Description("Package to analyze (e.g., $ZDEV). All objects in package are checked."),
			),
			mcp.WithString("object",
				mcp.Description("Single object to analyze (e.g., ZCL_TEST). Source is read from SAP."),
			),
			mcp.WithString("source",
				mcp.Description("ABAP source code to analyze offline (no SAP connection needed)."),
			),
			mcp.WithString("whitelist",
				mcp.Description("Comma-separated list of allowed Z-packages (supports glob: '$ZCOMMON,$ZUTIL*'). Dependencies on these packages are OK."),
			),
			mcp.WithNumber("depth",
				mcp.Description("Analysis depth (default: 1). Higher values follow transitive deps."),
			),
			mcp.WithString("format",
				mcp.Description("Output format: 'text' (default), 'json', 'full' (includes standard deps)"),
			),
		), s.handleCheckBoundaries)
	}

	if shouldRegister("GraphStats") {
		s.mcpServer.AddTool(mcp.NewTool("GraphStats",
			mcp.WithDescription("Extract dependency statistics from ABAP source code using embedded parser. Returns node/edge counts by type, edge kind, and source. Works offline without SAP."),
			mcp.WithString("source",
				mcp.Required(),
				mcp.Description("ABAP source code to analyze"),
			),
		), s.handleGraphStats)
	}
}

// registerDiagnosticsTools registers runtime error, profiler, and SQL trace tools.
func (s *Server) registerDiagnosticsTools(shouldRegister func(string) bool) {
	// --- Runtime Errors / Short Dumps (RABAX) ---
	if shouldRegister("ListDumps") {
		s.mcpServer.AddTool(mcp.NewTool("ListDumps",
			mcp.WithDescription("List runtime errors (short dumps) from the SAP system. Filter by user, exception type, program, date range."),
			mcp.WithString("user",
				mcp.Description("Filter by username"),
			),
			mcp.WithString("exception_type",
				mcp.Description("Filter by exception type (e.g., CX_SY_ZERODIVIDE)"),
			),
			mcp.WithString("program",
				mcp.Description("Filter by program name"),
			),
			mcp.WithString("package",
				mcp.Description("Filter by package"),
			),
			mcp.WithString("date_from",
				mcp.Description("Start date (YYYYMMDD format)"),
			),
			mcp.WithString("date_to",
				mcp.Description("End date (YYYYMMDD format)"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of results (default: 100)"),
			),
		), s.handleListDumps)
	}

	if shouldRegister("GetDump") {
		s.mcpServer.AddTool(mcp.NewTool("GetDump",
			mcp.WithDescription("Get full details of a specific runtime error (short dump) including stack trace."),
			mcp.WithString("dump_id",
				mcp.Required(),
				mcp.Description("Dump ID from ListDumps result"),
			),
		), s.handleGetDump)
	}

	// --- ABAP Profiler / Runtime Traces (ATRA) ---
	if shouldRegister("ListTraces") {
		s.mcpServer.AddTool(mcp.NewTool("ListTraces",
			mcp.WithDescription("List ABAP runtime traces (profiler results) from the SAP system."),
			mcp.WithString("user",
				mcp.Description("Filter by username"),
			),
			mcp.WithString("process_type",
				mcp.Description("Filter by process type"),
			),
			mcp.WithString("object_type",
				mcp.Description("Filter by object type"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of results (default: 100)"),
			),
		), s.handleListTraces)
	}

	if shouldRegister("GetTrace") {
		s.mcpServer.AddTool(mcp.NewTool("GetTrace",
			mcp.WithDescription("Get trace analysis (hitlist, statements, or database accesses) for a specific trace."),
			mcp.WithString("trace_id",
				mcp.Required(),
				mcp.Description("Trace ID from ListTraces result"),
			),
			mcp.WithString("tool_type",
				mcp.Description("Analysis type: 'hitlist' (default), 'statements', 'dbAccesses'"),
			),
		), s.handleGetTrace)
	}

	// --- SQL Trace (ST05) ---
	if shouldRegister("GetSQLTraceState") {
		s.mcpServer.AddTool(mcp.NewTool("GetSQLTraceState",
			mcp.WithDescription("Check if SQL trace (ST05) is currently active."),
		), s.handleGetSQLTraceState)
	}

	if shouldRegister("ListSQLTraces") {
		s.mcpServer.AddTool(mcp.NewTool("ListSQLTraces",
			mcp.WithDescription("List SQL trace files from ST05."),
			mcp.WithString("user",
				mcp.Description("Filter by username"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of results (default: 100)"),
			),
		), s.handleListSQLTraces)
	}
}

// registerDebuggerTools registers ABAP debugger session and breakpoint tools.
func (s *Server) registerDebuggerTools(shouldRegister func(string) bool) {
	if shouldRegister("SetBreakpoint") {
		s.mcpServer.AddTool(mcp.NewTool("SetBreakpoint",
			mcp.WithDescription("Set a breakpoint in ABAP code. Supports three types: 'line' (specific location), 'statement' (ABAP keyword), 'exception' (exception class). For class methods, use 'method' parameter for include-relative line numbers. Uses WebSocket connection to ZADT_VSP."),
			mcp.WithString("kind",
				mcp.Description("Breakpoint type: 'line' (default), 'statement', or 'exception'"),
			),
			mcp.WithString("program",
				mcp.Description("Program name for line breakpoints (e.g., 'ZADT_DBG_PROG' or 'ZCL_MY_CLASS')"),
			),
			mcp.WithString("method",
				mcp.Description("Method name for class breakpoints. When specified, line number is relative to method start (line 1 = first line of METHOD implementation). Enables accurate breakpoints in class methods."),
			),
			mcp.WithNumber("line",
				mcp.Description("Line number for line breakpoints. Without 'method': pool-absolute line. With 'method': relative to method start."),
			),
			mcp.WithString("statement",
				mcp.Description("ABAP statement for statement breakpoints (e.g., 'CALL FUNCTION', 'SELECT', 'LOOP', 'CALL METHOD')"),
			),
			mcp.WithString("exception",
				mcp.Description("Exception class for exception breakpoints (e.g., 'CX_SY_ZERODIVIDE', 'CX_SY_OPEN_SQL_DB')"),
			),
		), s.handleSetBreakpoint)
	}

	if shouldRegister("GetBreakpoints") {
		s.mcpServer.AddTool(mcp.NewTool("GetBreakpoints",
			mcp.WithDescription("Get all breakpoints registered in the current debug session. Uses WebSocket connection to ZADT_VSP."),
		), s.handleGetBreakpoints)
	}

	if shouldRegister("DeleteBreakpoint") {
		s.mcpServer.AddTool(mcp.NewTool("DeleteBreakpoint",
			mcp.WithDescription("Delete a breakpoint by ID. Uses WebSocket connection to ZADT_VSP."),
			mcp.WithString("breakpoint_id",
				mcp.Required(),
				mcp.Description("ID of the breakpoint to delete"),
			),
		), s.handleDeleteBreakpoint)
	}

	if shouldRegister("CallRFC") {
		s.mcpServer.AddTool(mcp.NewTool("CallRFC",
			mcp.WithDescription("Call a function module via WebSocket (ZADT_VSP). Useful for triggering ABAP code execution to hit breakpoints. Parameters are passed as key-value pairs."),
			mcp.WithString("function",
				mcp.Required(),
				mcp.Description("Function module name (e.g., 'RFC_PING', 'BAPI_USER_GET_DETAIL')"),
			),
			mcp.WithString("params",
				mcp.Description("JSON object with function parameters (e.g., '{\"IV_PARAM\":\"value\"}')"),
			),
		), s.handleCallRFC)
	}

	if shouldRegister("MoveObject") {
		s.mcpServer.AddTool(mcp.NewTool("MoveObject",
			mcp.WithDescription("Move an ABAP object to a different package. Uses ZADT_VSP WebSocket to call TR_TADIR_INTERFACE. Requires ZADT_VSP deployed."),
			mcp.WithString("object_type",
				mcp.Required(),
				mcp.Description("Object type: CLAS, PROG, INTF, FUGR, TABL, etc."),
			),
			mcp.WithString("object_name",
				mcp.Required(),
				mcp.Description("Name of the object to move (e.g., 'ZCL_TEST')"),
			),
			mcp.WithString("new_package",
				mcp.Required(),
				mcp.Description("Target package (e.g., '$ZRAY', 'ZPACKAGE')"),
			),
		), s.handleMoveObject)
	}

	if shouldRegister("DebuggerListen") {
		s.mcpServer.AddTool(mcp.NewTool("DebuggerListen",
			mcp.WithDescription("Start a debug listener that waits for a debuggee to hit a breakpoint. This is a BLOCKING call that uses long-polling. Returns when a debuggee is caught, timeout occurs, or a conflict is detected."),
			mcp.WithString("user",
				mcp.Description("User to listen for (defaults to current user)"),
			),
			mcp.WithNumber("timeout",
				mcp.Description("Timeout in seconds (default: 60, max: 240)"),
			),
		), s.handleDebuggerListen)
	}

	if shouldRegister("DebuggerAttach") {
		s.mcpServer.AddTool(mcp.NewTool("DebuggerAttach",
			mcp.WithDescription("Attach to a debuggee that has hit a breakpoint. Use the debuggee_id from DebuggerListen result."),
			mcp.WithString("debuggee_id",
				mcp.Required(),
				mcp.Description("ID of the debuggee (from DebuggerListen result)"),
			),
			mcp.WithString("user",
				mcp.Description("User for debugging (defaults to current user)"),
			),
		), s.handleDebuggerAttach)
	}

	if shouldRegister("DebuggerDetach") {
		s.mcpServer.AddTool(mcp.NewTool("DebuggerDetach",
			mcp.WithDescription("Detach from the current debug session and release the debuggee."),
		), s.handleDebuggerDetach)
	}

	if shouldRegister("DebuggerStep") {
		s.mcpServer.AddTool(mcp.NewTool("DebuggerStep",
			mcp.WithDescription("Perform a step operation in the debugger."),
			mcp.WithString("step_type",
				mcp.Required(),
				mcp.Description("Step type: 'stepInto', 'stepOver', 'stepReturn', 'stepContinue', 'stepRunToLine', 'stepJumpToLine'"),
			),
			mcp.WithString("uri",
				mcp.Description("Target URI for stepRunToLine/stepJumpToLine (e.g., '/sap/bc/adt/programs/programs/ZTEST/source/main#start=42')"),
			),
		), s.handleDebuggerStep)
	}

	if shouldRegister("DebuggerGetStack") {
		s.mcpServer.AddTool(mcp.NewTool("DebuggerGetStack",
			mcp.WithDescription("Get the current call stack during a debug session."),
		), s.handleDebuggerGetStack)
	}

	if shouldRegister("DebuggerGetVariables") {
		s.mcpServer.AddTool(mcp.NewTool("DebuggerGetVariables",
			mcp.WithDescription("Get variable values during a debug session. Use '@ROOT' to get top-level variables, or specific variable IDs to get their values."),
			mcp.WithArray("variable_ids",
				mcp.Description("Variable IDs to retrieve (e.g., ['@ROOT'] for top-level, or specific IDs like ['LV_COUNT', 'LS_DATA'])"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
		), s.handleDebuggerGetVariables)
	}
}

// registerSearchTools registers object search tools.
func (s *Server) registerSearchTools(shouldRegister func(string) bool) {
	if shouldRegister("SearchObject") {
		s.mcpServer.AddTool(mcp.NewTool("SearchObject",
			mcp.WithDescription("Search for ABAP objects using quick search"),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("Search query string (use * wildcard for partial match)"),
			),
			mcp.WithNumber("maxResults",
				mcp.Description("Maximum number of results to return (default 100)"),
			),
		), s.handleSearchObject)
	}
}

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
		), s.handleActivate)
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
		), s.handleActivatePackage)
	}

	if shouldRegister("RunUnitTests") {
		s.mcpServer.AddTool(mcp.NewTool("RunUnitTests",
			mcp.WithDescription("Run ABAP Unit tests for an object"),
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
			mcp.WithDescription("Execute arbitrary ABAP code via unit test wrapper. Creates temp program, injects code into test method, runs via RunUnitTests, extracts results from assertion messages, cleans up. Use lv_result variable to return output. WARNING: Powerful tool - use responsibly."),
			mcp.WithString("code",
				mcp.Required(),
				mcp.Description("ABAP code to execute. Set lv_result variable to return output via assertion message."),
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
		), s.handleExecuteABAP)
	}
}

// registerCRUDTools registers CRUD operations (lock, create, update, delete).
func (s *Server) registerCRUDTools(shouldRegister func(string) bool) {
	if shouldRegister("LockObject") {
		s.mcpServer.AddTool(mcp.NewTool("LockObject",
			mcp.WithDescription("Acquire an edit lock on an ABAP object"),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
			),
			mcp.WithString("access_mode",
				mcp.Description("Access mode: MODIFY (default) or READ"),
			),
		), s.handleLockObject)
	}

	if shouldRegister("UnlockObject") {
		s.mcpServer.AddTool(mcp.NewTool("UnlockObject",
			mcp.WithDescription("Release an edit lock on an ABAP object"),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
			),
			mcp.WithString("lock_handle",
				mcp.Required(),
				mcp.Description("Lock handle from LockObject"),
			),
		), s.handleUnlockObject)
	}

	if shouldRegister("UpdateSource") {
		s.mcpServer.AddTool(mcp.NewTool("UpdateSource",
			mcp.WithDescription("Write source code to an ABAP object (requires lock)"),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
			),
			mcp.WithString("source",
				mcp.Required(),
				mcp.Description("ABAP source code to write"),
			),
			mcp.WithString("lock_handle",
				mcp.Required(),
				mcp.Description("Lock handle from LockObject"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for local packages)"),
			),
		), s.handleUpdateSource)
	}

	if shouldRegister("CreateObject") {
		s.mcpServer.AddTool(mcp.NewTool("CreateObject",
			mcp.WithDescription("Create a new ABAP object. Supports: PROG/P (program), CLAS/OC (class), INTF/OI (interface), PROG/I (include), FUGR/F (function group), FUGR/FF (function module), DEVC/K (package), DDLS/DF (CDS view), BDEF/BDO (behavior definition), SRVD/SRV (service definition), SRVB/SVB (service binding)"),
			mcp.WithString("object_type",
				mcp.Required(),
				mcp.Description("Object type: PROG/P, CLAS/OC, INTF/OI, PROG/I, FUGR/F, FUGR/FF, DEVC/K, DDLS/DF, BDEF/BDO, SRVD/SRV, SRVB/SVB"),
			),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Object name (e.g., ZTEST_PROGRAM)"),
			),
			mcp.WithString("description",
				mcp.Required(),
				mcp.Description("Object description"),
			),
			mcp.WithString("package_name",
				mcp.Required(),
				mcp.Description("Package name (e.g., $TMP for local, ZPACKAGE for transportable)"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (required for non-local packages)"),
			),
			mcp.WithString("parent_name",
				mcp.Description("Parent name (required for function modules - the function group name)"),
			),
			// RAP-specific options
			mcp.WithString("service_definition",
				mcp.Description("For SRVB: the service definition name to bind"),
			),
			mcp.WithString("binding_version",
				mcp.Description("For SRVB: OData version 'V2' or 'V4' (default: V2)"),
			),
			mcp.WithString("binding_category",
				mcp.Description("For SRVB: '0' for Web API, '1' for UI (default: 0)"),
			),
		), s.handleCreateObject)
	}

	if shouldRegister("CreatePackage") {
		s.mcpServer.AddTool(mcp.NewTool("CreatePackage",
			mcp.WithDescription("Create a new ABAP package. Local packages ($*) work by default. Transportable packages require --enable-transports flag and transport parameter."),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Package name (e.g., $ZTEST for local, ZPRODUCTION for transportable)"),
			),
			mcp.WithString("description",
				mcp.Required(),
				mcp.Description("Package description"),
			),
			mcp.WithString("parent",
				mcp.Description("Parent package name (optional, e.g., $TMP, ZPROD). If not specified, creates a root-level package."),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (required for transportable packages, e.g., 'A4HK900114')"),
			),
			mcp.WithString("software_component",
				mcp.Description("Software component name (required for transportable packages, e.g., 'HOME', 'ZLOCAL'). Use GetInstalledComponents to list available components."),
			),
		), s.handleCreatePackage)
	}

	if shouldRegister("CreateTable") {
		s.mcpServer.AddTool(mcp.NewTool("CreateTable",
			mcp.WithDescription("Create a DDIC transparent table from a simple JSON definition. Handles full workflow: create → set source → activate. Supports common ABAP types: CHAR, NUMC, INT4, DEC, STRING, TIMESTAMPL, UUID, etc."),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Table name (uppercase, max 30 chars, must start with Z/Y)"),
			),
			mcp.WithString("description",
				mcp.Required(),
				mcp.Description("Short description of the table"),
			),
			mcp.WithString("package",
				mcp.Description("Target package (default: $TMP)"),
			),
			mcp.WithString("fields",
				mcp.Required(),
				mcp.Description("JSON array of fields: [{\"name\":\"ID\",\"type\":\"CHAR32\",\"key\":true},{\"name\":\"VALUE\",\"type\":\"STRING\"}]. Types: CHAR/CHARnn, NUMC/NUMCnn, INT4, DEC, STRING, TIMESTAMPL, UUID, DATS, TIMS, or data element name."),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for $TMP)"),
			),
			mcp.WithString("delivery_class",
				mcp.Description("Delivery class: A=Application (default), C=Customizing, L=Temporary"),
			),
		), s.handleCreateTable)
	}

	if shouldRegister("CompareSource") {
		s.mcpServer.AddTool(mcp.NewTool("CompareSource",
			mcp.WithDescription("Compare source code of two objects and return unified diff. Supports all object types from GetSource."),
			mcp.WithString("type1",
				mcp.Required(),
				mcp.Description("Object type of first object: PROG, CLAS, INTF, FUNC, FUGR, INCL, DDLS, BDEF, SRVD"),
			),
			mcp.WithString("name1",
				mcp.Required(),
				mcp.Description("Name of first object"),
			),
			mcp.WithString("type2",
				mcp.Required(),
				mcp.Description("Object type of second object (can be same or different)"),
			),
			mcp.WithString("name2",
				mcp.Required(),
				mcp.Description("Name of second object"),
			),
			mcp.WithString("include1",
				mcp.Description("Class include type for first object if CLAS: definitions, implementations, macros, testclasses"),
			),
			mcp.WithString("include2",
				mcp.Description("Class include type for second object if CLAS"),
			),
			mcp.WithString("parent1",
				mcp.Description("Function group for first object if FUNC"),
			),
			mcp.WithString("parent2",
				mcp.Description("Function group for second object if FUNC"),
			),
		), s.handleCompareSource)
	}

	if shouldRegister("CloneObject") {
		s.mcpServer.AddTool(mcp.NewTool("CloneObject",
			mcp.WithDescription("Copy an ABAP object to a new name. Replaces object name in source. Supports PROG, CLAS, INTF."),
			mcp.WithString("object_type",
				mcp.Required(),
				mcp.Description("Object type: PROG, CLAS, INTF"),
			),
			mcp.WithString("source_name",
				mcp.Required(),
				mcp.Description("Name of object to copy"),
			),
			mcp.WithString("target_name",
				mcp.Required(),
				mcp.Description("Name for the new object"),
			),
			mcp.WithString("package",
				mcp.Required(),
				mcp.Description("Target package (e.g., $TMP)"),
			),
		), s.handleCloneObject)
	}

	if shouldRegister("GetClassInfo") {
		s.mcpServer.AddTool(mcp.NewTool("GetClassInfo",
			mcp.WithDescription("Get class metadata without full source: methods, attributes, interfaces, superclass, abstract/final status."),
			mcp.WithString("class_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP class"),
			),
		), s.handleGetClassInfo)
	}

	if shouldRegister("DeleteObject") {
		s.mcpServer.AddTool(mcp.NewTool("DeleteObject",
			mcp.WithDescription("Delete an ABAP object (requires lock)"),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
			),
			mcp.WithString("lock_handle",
				mcp.Required(),
				mcp.Description("Lock handle from LockObject"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for local packages)"),
			),
		), s.handleDeleteObject)
	}

	// Transport-related tools
	if shouldRegister("GetUserTransports") {
		s.mcpServer.AddTool(mcp.NewTool("GetUserTransports",
			mcp.WithDescription("Get all transport requests for a user (requires --enable-transports flag). Returns both workbench and customizing requests grouped by target system."),
			mcp.WithString("user_name",
				mcp.Required(),
				mcp.Description("SAP user name (will be converted to uppercase)"),
			),
		), s.handleGetUserTransports)
	}

	if shouldRegister("GetTransportInfo") {
		s.mcpServer.AddTool(mcp.NewTool("GetTransportInfo",
			mcp.WithDescription("Get transport information for an ABAP object (requires --enable-transports flag). Returns available transports and lock status."),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
			),
			mcp.WithString("dev_class",
				mcp.Required(),
				mcp.Description("Development class/package of the object"),
			),
		), s.handleGetTransportInfo)
	}
}

// registerClassIncludeTools registers class include operations.
func (s *Server) registerClassIncludeTools(shouldRegister func(string) bool) {
	if shouldRegister("GetClassInclude") {
		s.mcpServer.AddTool(mcp.NewTool("GetClassInclude",
			mcp.WithDescription("Retrieve source code of a class include (definitions, implementations, macros, testclasses)"),
			mcp.WithString("class_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP class"),
			),
			mcp.WithString("include_type",
				mcp.Required(),
				mcp.Description("Include type: main, definitions, implementations, macros, testclasses"),
			),
		), s.handleGetClassInclude)
	}

	if shouldRegister("CreateTestInclude") {
		s.mcpServer.AddTool(mcp.NewTool("CreateTestInclude",
			mcp.WithDescription("Create the test classes include for a class (required before writing test code)"),
			mcp.WithString("class_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP class"),
			),
			mcp.WithString("lock_handle",
				mcp.Required(),
				mcp.Description("Lock handle from LockObject (lock the parent class first)"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for local packages)"),
			),
		), s.handleCreateTestInclude)
	}

	if shouldRegister("UpdateClassInclude") {
		s.mcpServer.AddTool(mcp.NewTool("UpdateClassInclude",
			mcp.WithDescription("Update source code of a class include (requires lock on parent class)"),
			mcp.WithString("class_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP class"),
			),
			mcp.WithString("include_type",
				mcp.Required(),
				mcp.Description("Include type: main, definitions, implementations, macros, testclasses"),
			),
			mcp.WithString("source",
				mcp.Required(),
				mcp.Description("ABAP source code to write"),
			),
			mcp.WithString("lock_handle",
				mcp.Required(),
				mcp.Description("Lock handle from LockObject (lock the parent class first)"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for local packages)"),
			),
		), s.handleUpdateClassInclude)
	}

	if shouldRegister("PublishServiceBinding") {
		s.mcpServer.AddTool(mcp.NewTool("PublishServiceBinding",
			mcp.WithDescription("Publish a service binding to make it available as OData service"),
			mcp.WithString("service_name",
				mcp.Required(),
				mcp.Description("Service binding name (e.g., ZTRAVEL_SB)"),
			),
			mcp.WithString("service_version",
				mcp.Description("Service version (default: 0001)"),
			),
		), s.handlePublishServiceBinding)
	}

	if shouldRegister("UnpublishServiceBinding") {
		s.mcpServer.AddTool(mcp.NewTool("UnpublishServiceBinding",
			mcp.WithDescription("Unpublish a service binding"),
			mcp.WithString("service_name",
				mcp.Required(),
				mcp.Description("Service binding name (e.g., ZTRAVEL_SB)"),
			),
			mcp.WithString("service_version",
				mcp.Description("Service version (default: 0001)"),
			),
		), s.handleUnpublishServiceBinding)
	}
}

// registerWorkflowTools registers workflow tools (WriteProgram, WriteClass, etc.)
func (s *Server) registerWorkflowTools(shouldRegister func(string) bool) {
	if shouldRegister("WriteProgram") {
		s.mcpServer.AddTool(mcp.NewTool("WriteProgram",
			mcp.WithDescription("Update an existing program with syntax check and activation (Lock -> SyntaxCheck -> Update -> Unlock -> Activate)"),
			mcp.WithString("program_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP program"),
			),
			mcp.WithString("source",
				mcp.Required(),
				mcp.Description("ABAP source code"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for local packages)"),
			),
		), s.handleWriteProgram)
	}

	if shouldRegister("WriteClass") {
		s.mcpServer.AddTool(mcp.NewTool("WriteClass",
			mcp.WithDescription("Update an existing class with syntax check and activation (Lock -> SyntaxCheck -> Update -> Unlock -> Activate)"),
			mcp.WithString("class_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP class"),
			),
			mcp.WithString("source",
				mcp.Required(),
				mcp.Description("ABAP class source code (definition and implementation)"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for local packages)"),
			),
		), s.handleWriteClass)
	}

	if shouldRegister("CreateAndActivateProgram") {
		s.mcpServer.AddTool(mcp.NewTool("CreateAndActivateProgram",
			mcp.WithDescription("Create a new program with source code and activate it (Create -> Lock -> Update -> Unlock -> Activate)"),
			mcp.WithString("program_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP program"),
			),
			mcp.WithString("description",
				mcp.Required(),
				mcp.Description("Program description"),
			),
			mcp.WithString("package_name",
				mcp.Required(),
				mcp.Description("Package name (e.g., $TMP for local)"),
			),
			mcp.WithString("source",
				mcp.Required(),
				mcp.Description("ABAP source code"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (required for non-local packages)"),
			),
		), s.handleCreateAndActivateProgram)
	}

	if shouldRegister("CreateClassWithTests") {
		s.mcpServer.AddTool(mcp.NewTool("CreateClassWithTests",
			mcp.WithDescription("Create a new class with unit tests and run them (Create -> Lock -> Update -> CreateTestInclude -> UpdateTest -> Unlock -> Activate -> RunTests)"),
			mcp.WithString("class_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP class"),
			),
			mcp.WithString("description",
				mcp.Required(),
				mcp.Description("Class description"),
			),
			mcp.WithString("package_name",
				mcp.Required(),
				mcp.Description("Package name (e.g., $TMP for local)"),
			),
			mcp.WithString("class_source",
				mcp.Required(),
				mcp.Description("ABAP class source code (definition and implementation)"),
			),
			mcp.WithString("test_source",
				mcp.Required(),
				mcp.Description("ABAP unit test source code"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (required for non-local packages)"),
			),
		), s.handleCreateClassWithTests)
	}
}

// registerFileTools registers file-based deployment tools.
func (s *Server) registerFileTools(shouldRegister func(string) bool) {
	if shouldRegister("DeployFromFile") {
		s.mcpServer.AddTool(mcp.NewTool("DeployFromFile",
			mcp.WithDescription("✅ RECOMMENDED - Smart deploy from file: auto-detects if object exists and creates/updates accordingly. Solves token limit problem for large generated files (ML models, 3948+ lines). Example: DeployFromFile(file_path=\"/path/to/zcl_ml_iris.clas.abap\", package_name=\"$ZAML_IRIS\") deploys any size file. Workflow: Parse → Check existence → Create or Update → Lock → SyntaxCheck → Write → Unlock → Activate. Supports .clas.abap, .prog.abap, .intf.abap, .fugr.abap, .func.abap. Use this for all file-based deployments."),
			mcp.WithString("file_path",
				mcp.Required(),
				mcp.Description("Absolute path to ABAP source file"),
			),
			mcp.WithString("package_name",
				mcp.Required(),
				mcp.Description("Package name (required for new objects, e.g., $ZAML_IRIS)"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for local packages)"),
			),
		), s.handleDeployFromFile)
	}

	if shouldRegister("SaveToFile") {
		s.mcpServer.AddTool(mcp.NewTool("SaveToFile",
			mcp.WithDescription("Save ABAP object source to local file (SAP → File). Enables BIDIRECTIONAL SYNC WORKFLOW: (1) SaveToFile downloads object from SAP, (2) edit locally with vim/VS Code/AI assistants, (3) DeployFromFile uploads changes back to SAP. Example: SaveToFile(objType=\"CLAS/OC\", objectName=\"ZCL_ML_IRIS\", outputPath=\"./src/\") creates ./src/zcl_ml_iris.clas.abap. Then edit locally and use DeployFromFile to sync back. Recommended for iterative development. Auto-determines file extension."),
			mcp.WithString("objType",
				mcp.Required(),
				mcp.Description("Object type: CLAS/OC (class), PROG/P (program), INTF/OI (interface), FUGR/F (function group), FUGR/FF (function module)"),
			),
			mcp.WithString("objectName",
				mcp.Required(),
				mcp.Description("Object name (e.g., ZCL_ML_IRIS, ZAML_IRIS_DEMO)"),
			),
			mcp.WithString("outputPath",
				mcp.Description("Output file path or directory. If directory, filename is auto-generated with correct extension. If omitted, saves to current directory."),
			),
		), s.handleSaveToFile)
	}

	if shouldRegister("ImportFromFile") {
		s.registerImportFromFile()
	}

	if shouldRegister("ExportToFile") {
		s.registerExportToFile()
	}

	if shouldRegister("RenameObject") {
		s.mcpServer.AddTool(mcp.NewTool("RenameObject",
			mcp.WithDescription("Rename ABAP object by creating copy with new name and deleting old one. Useful for fixing naming conventions. Workflow: GetSource → Replace names → CreateNew → ActivateNew → DeleteOld"),
			mcp.WithString("objType",
				mcp.Required(),
				mcp.Description("Object type: CLAS/OC (class), PROG/P (program), INTF/OI (interface), FUGR/F (function group)"),
			),
			mcp.WithString("oldName",
				mcp.Required(),
				mcp.Description("Current object name"),
			),
			mcp.WithString("newName",
				mcp.Required(),
				mcp.Description("New object name"),
			),
			mcp.WithString("packageName",
				mcp.Required(),
				mcp.Description("Package name for new object (e.g., $ZAML_IRIS)"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for local packages)"),
			),
		), s.handleRenameObject)
	}
}

// registerEditTools registers surgical edit tools.
func (s *Server) registerEditTools(shouldRegister func(string) bool) {
	if shouldRegister("EditSource") {
		s.mcpServer.AddTool(mcp.NewTool("EditSource",
			mcp.WithDescription("Surgical string replacement on ABAP source code. Matches the Edit tool pattern for local files. Workflow: GetSource → FindReplace → SyntaxCheck → Lock → Update → Unlock → Activate. Example: EditSource(object_url=\"/sap/bc/adt/programs/programs/ZTEST\", old_string=\"METHOD foo.\\n  ENDMETHOD.\", new_string=\"METHOD foo.\\n  rv_result = 42.\\n  ENDMETHOD.\", replace_all=false, syntax_check=true). Requires unique match if replace_all=false. Use this for incremental edits between syntax checks - no need to download/upload full source!"),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT URL of object (e.g., /sap/bc/adt/programs/programs/ZTEST, /sap/bc/adt/oo/classes/zcl_test)"),
			),
			mcp.WithString("old_string",
				mcp.Required(),
				mcp.Description("Exact string to find and replace. Must be unique in source if replace_all=false. Include enough context (surrounding lines) to ensure uniqueness."),
			),
			mcp.WithString("new_string",
				mcp.Required(),
				mcp.Description("Replacement string. Can be multiline (use \\n). Length can differ from old_string."),
			),
			mcp.WithBoolean("replace_all",
				mcp.Description("If true, replace all occurrences. If false (default), require unique match. Default: false"),
			),
			mcp.WithBoolean("syntax_check",
				mcp.Description("If true (default), validate syntax before saving. If syntax errors found, changes are NOT saved. Default: true"),
			),
			mcp.WithBoolean("case_insensitive",
				mcp.Description("If true, ignore case when matching old_string. Useful for renaming variables regardless of case. Default: false"),
			),
			mcp.WithString("method",
				mcp.Description("For CLAS only: constrain search/replace to this method only. Prevents accidental edits in other methods. (optional)"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (required for objects not in $TMP package)"),
			),
		), s.handleEditSource)
	}
}

// registerGrepTools registers grep/search tools.
func (s *Server) registerGrepTools(shouldRegister func(string) bool) {
	if shouldRegister("GrepObject") {
		s.mcpServer.AddTool(mcp.NewTool("GrepObject",
			mcp.WithDescription("Search for regex pattern in a single ABAP object's source code. Returns matches with line numbers and optional context. Use for finding TODO comments, string literals, patterns, or code snippets before editing."),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT URL of object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
			),
			mcp.WithString("pattern",
				mcp.Required(),
				mcp.Description("Regular expression pattern (Go regexp syntax). Examples: 'TODO', 'lv_\\w+', 'SELECT.*FROM'"),
			),
			mcp.WithBoolean("case_insensitive",
				mcp.Description("If true, perform case-insensitive matching. Default: false"),
			),
			mcp.WithNumber("context_lines",
				mcp.Description("Number of lines to show before/after each match (like grep -C). Default: 0"),
			),
		), s.handleGrepObject)
	}

	if shouldRegister("GrepPackage") {
		s.mcpServer.AddTool(mcp.NewTool("GrepPackage",
			mcp.WithDescription("Search for regex pattern across all source objects in an ABAP package. Returns matches grouped by object. Use for package-wide analysis, finding patterns across multiple programs/classes."),
			mcp.WithString("package_name",
				mcp.Required(),
				mcp.Description("Package name (e.g., $TMP, ZPACKAGE)"),
			),
			mcp.WithString("pattern",
				mcp.Required(),
				mcp.Description("Regular expression pattern (Go regexp syntax). Examples: 'TODO', 'lv_\\w+', 'SELECT.*FROM'"),
			),
			mcp.WithBoolean("case_insensitive",
				mcp.Description("If true, perform case-insensitive matching. Default: false"),
			),
			mcp.WithString("object_types",
				mcp.Description("Comma-separated object types to search (e.g., 'PROG/P,CLAS/OC'). Empty = search all source objects. Valid: PROG/P, CLAS/OC, INTF/OI, FUGR/F, FUGR/FF, PROG/I"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of matching objects to return. 0 = unlimited. Default: 100"),
			),
		), s.handleGrepPackage)
	}

	if shouldRegister("GrepObjects") {
		s.registerGrepObjects()
	}

	if shouldRegister("GrepPackages") {
		s.registerGrepPackages()
	}
}

// registerCodeIntelTools registers code intelligence tools.
func (s *Server) registerCodeIntelTools(shouldRegister func(string) bool) {
	if shouldRegister("FindDefinition") {
		s.mcpServer.AddTool(mcp.NewTool("FindDefinition",
			mcp.WithDescription("Navigate to the definition of a symbol at a given position in source code"),
			mcp.WithString("source_url",
				mcp.Required(),
				mcp.Description("ADT URL of the source file (e.g., /sap/bc/adt/programs/programs/ZTEST/source/main)"),
			),
			mcp.WithString("source",
				mcp.Required(),
				mcp.Description("Full source code of the file"),
			),
			mcp.WithNumber("line",
				mcp.Required(),
				mcp.Description("Line number (1-based)"),
			),
			mcp.WithNumber("start_column",
				mcp.Required(),
				mcp.Description("Start column of the symbol (1-based)"),
			),
			mcp.WithNumber("end_column",
				mcp.Required(),
				mcp.Description("End column of the symbol (1-based)"),
			),
			mcp.WithBoolean("implementation",
				mcp.Description("Navigate to implementation instead of definition (default: false)"),
			),
			mcp.WithString("main_program",
				mcp.Description("Main program for includes (optional)"),
			),
		), s.handleFindDefinition)
	}

	if shouldRegister("FindReferences") {
		s.mcpServer.AddTool(mcp.NewTool("FindReferences",
			mcp.WithDescription("Find all references to an ABAP object or symbol"),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/oo/classes/ZCL_TEST)"),
			),
			mcp.WithNumber("line",
				mcp.Description("Line number for position-based reference search (1-based, optional)"),
			),
			mcp.WithNumber("column",
				mcp.Description("Column number for position-based reference search (1-based, optional)"),
			),
		), s.handleFindReferences)
	}

	if shouldRegister("GetContext") {
		s.mcpServer.AddTool(mcp.NewTool("GetContext",
			mcp.WithDescription("Analyze ABAP source dependencies and return compressed public API contracts (prologue). Produces a compact summary of all referenced classes, interfaces, and function modules — showing only their public signatures. Use this to understand the surrounding codebase context before editing."),
			mcp.WithString("object_type",
				mcp.Required(),
				mcp.Description("ABAP object type: PROG, CLAS, INTF, FUNC, FUGR"),
			),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Object name (e.g., ZCL_ORDER_PROCESSOR)"),
			),
			mcp.WithString("source",
				mcp.Description("Source code to analyze (if omitted, fetched from SAP)"),
			),
			mcp.WithNumber("max_deps",
				mcp.Description("Maximum dependencies to resolve (default: 20)"),
			),
		), s.handleGetContext)
	}

	if shouldRegister("CodeCompletion") {
		s.mcpServer.AddTool(mcp.NewTool("CodeCompletion",
			mcp.WithDescription("Get code completion suggestions at a position in source code"),
			mcp.WithString("source_url",
				mcp.Required(),
				mcp.Description("ADT URL of the source file (e.g., /sap/bc/adt/programs/programs/ZTEST/source/main)"),
			),
			mcp.WithString("source",
				mcp.Required(),
				mcp.Description("Full source code of the file"),
			),
			mcp.WithNumber("line",
				mcp.Required(),
				mcp.Description("Line number (1-based)"),
			),
			mcp.WithNumber("column",
				mcp.Required(),
				mcp.Description("Column number (1-based)"),
			),
		), s.handleCodeCompletion)
	}

	if shouldRegister("GetTypeHierarchy") {
		s.mcpServer.AddTool(mcp.NewTool("GetTypeHierarchy",
			mcp.WithDescription("Get the type hierarchy (supertypes or subtypes) for a class/interface"),
			mcp.WithString("source_url",
				mcp.Required(),
				mcp.Description("ADT URL of the source file"),
			),
			mcp.WithString("source",
				mcp.Required(),
				mcp.Description("Full source code of the file"),
			),
			mcp.WithNumber("line",
				mcp.Required(),
				mcp.Description("Line number (1-based)"),
			),
			mcp.WithNumber("column",
				mcp.Required(),
				mcp.Description("Column number (1-based)"),
			),
			mcp.WithBoolean("super_types",
				mcp.Description("Get supertypes instead of subtypes (default: false = subtypes)"),
			),
		), s.handleGetTypeHierarchy)
	}

	if shouldRegister("GetClassComponents") {
		s.mcpServer.AddTool(mcp.NewTool("GetClassComponents",
			mcp.WithDescription("Get the structure of a class - lists all methods, attributes, events, and other components with their visibility and properties"),
			mcp.WithString("class_url",
				mcp.Required(),
				mcp.Description("ADT URL of the class (e.g., /sap/bc/adt/oo/classes/ZCL_TEST)"),
			),
		), s.handleGetClassComponents)
	}
}

// registerUI5Tools registers UI5/Fiori BSP management tools.
func (s *Server) registerUI5Tools(shouldRegister func(string) bool) {
	if shouldRegister("UI5ListApps") {
		s.mcpServer.AddTool(mcp.NewTool("UI5ListApps",
			mcp.WithDescription("List UI5/Fiori BSP applications. Use query parameter for filtering with wildcards (*)."),
			mcp.WithString("query",
				mcp.Description("Search query (supports * wildcard, e.g., 'Z*' for custom apps)"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of results (default: 100)"),
			),
		), s.handleUI5ListApps)
	}

	if shouldRegister("UI5GetApp") {
		s.mcpServer.AddTool(mcp.NewTool("UI5GetApp",
			mcp.WithDescription("Get details of a UI5/Fiori BSP application including file structure."),
			mcp.WithString("app_name",
				mcp.Required(),
				mcp.Description("Name of the UI5 application"),
			),
		), s.handleUI5GetApp)
	}

	if shouldRegister("UI5GetFileContent") {
		s.mcpServer.AddTool(mcp.NewTool("UI5GetFileContent",
			mcp.WithDescription("Get content of a specific file within a UI5/Fiori BSP application."),
			mcp.WithString("app_name",
				mcp.Required(),
				mcp.Description("Name of the UI5 application"),
			),
			mcp.WithString("file_path",
				mcp.Required(),
				mcp.Description("Path to the file within the app (e.g., '/webapp/manifest.json')"),
			),
		), s.handleUI5GetFileContent)
	}

	if shouldRegister("UI5UploadFile") {
		s.mcpServer.AddTool(mcp.NewTool("UI5UploadFile",
			mcp.WithDescription("Upload a file to a UI5/Fiori BSP application."),
			mcp.WithString("app_name",
				mcp.Required(),
				mcp.Description("Name of the UI5 application"),
			),
			mcp.WithString("file_path",
				mcp.Required(),
				mcp.Description("Path for the file within the app (e.g., '/webapp/Component.js')"),
			),
			mcp.WithString("content",
				mcp.Required(),
				mcp.Description("File content to upload"),
			),
			mcp.WithString("content_type",
				mcp.Description("Content type (e.g., 'application/javascript', 'application/json')"),
			),
		), s.handleUI5UploadFile)
	}

	if shouldRegister("UI5DeleteFile") {
		s.mcpServer.AddTool(mcp.NewTool("UI5DeleteFile",
			mcp.WithDescription("Delete a file from a UI5/Fiori BSP application."),
			mcp.WithString("app_name",
				mcp.Required(),
				mcp.Description("Name of the UI5 application"),
			),
			mcp.WithString("file_path",
				mcp.Required(),
				mcp.Description("Path to the file to delete (e.g., '/webapp/test.js')"),
			),
		), s.handleUI5DeleteFile)
	}

	if shouldRegister("UI5CreateApp") {
		s.mcpServer.AddTool(mcp.NewTool("UI5CreateApp",
			mcp.WithDescription("Create a new UI5/Fiori BSP application."),
			mcp.WithString("app_name",
				mcp.Required(),
				mcp.Description("Name for the new UI5 application"),
			),
			mcp.WithString("description",
				mcp.Description("Description of the application"),
			),
			mcp.WithString("package",
				mcp.Required(),
				mcp.Description("Package name (e.g., '$TMP' for local, 'ZFIORI' for transportable)"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for local packages)"),
			),
		), s.handleUI5CreateApp)
	}

	if shouldRegister("UI5DeleteApp") {
		s.mcpServer.AddTool(mcp.NewTool("UI5DeleteApp",
			mcp.WithDescription("Delete a UI5/Fiori BSP application."),
			mcp.WithString("app_name",
				mcp.Required(),
				mcp.Description("Name of the UI5 application to delete"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for local packages)"),
			),
		), s.handleUI5DeleteApp)
	}
}

// registerAMDPTools registers AMDP/HANA debugger tools.
func (s *Server) registerAMDPTools(shouldRegister func(string) bool) {
	if shouldRegister("AMDPDebuggerStart") {
		s.mcpServer.AddTool(mcp.NewTool("AMDPDebuggerStart",
			mcp.WithDescription("Start an AMDP (HANA SQLScript) debug session with persistent goroutine. Creates a background goroutine that maintains the HTTP session cookies. Use AMDPDebuggerStep/AMDPGetVariables to interact, AMDPDebuggerStop to terminate."),
			mcp.WithString("user",
				mcp.Description("User to debug (defaults to current user)"),
			),
		), s.handleAMDPDebuggerStart)
	}

	if shouldRegister("AMDPDebuggerResume") {
		s.mcpServer.AddTool(mcp.NewTool("AMDPDebuggerResume",
			mcp.WithDescription("Get current AMDP debug session status. In goroutine model, this returns the current state without blocking. The session manager goroutine handles events internally."),
		), s.handleAMDPDebuggerResume)
	}

	if shouldRegister("AMDPDebuggerStop") {
		s.mcpServer.AddTool(mcp.NewTool("AMDPDebuggerStop",
			mcp.WithDescription("Stop the AMDP debug session and terminate the background goroutine. Cleans up the HTTP session on SAP server."),
		), s.handleAMDPDebuggerStop)
	}

	if shouldRegister("AMDPDebuggerStep") {
		s.mcpServer.AddTool(mcp.NewTool("AMDPDebuggerStep",
			mcp.WithDescription("Perform a step operation in the AMDP debugger. Communicates via channel to the session manager goroutine."),
			mcp.WithString("step_type",
				mcp.Required(),
				mcp.Description("Step type: 'stepInto', 'stepOver', 'stepReturn', 'stepContinue'"),
			),
		), s.handleAMDPDebuggerStep)
	}

	if shouldRegister("AMDPGetVariables") {
		s.mcpServer.AddTool(mcp.NewTool("AMDPGetVariables",
			mcp.WithDescription("Get variable values during AMDP debugging. Communicates via channel to the session manager goroutine. Returns scalar, table, and array types."),
		), s.handleAMDPGetVariables)
	}

	if shouldRegister("AMDPSetBreakpoint") {
		s.mcpServer.AddTool(mcp.NewTool("AMDPSetBreakpoint",
			mcp.WithDescription("Set a breakpoint in AMDP (SQLScript) code. Requires an active AMDP debug session. Specify the procedure name and line number."),
			mcp.WithString("proc_name",
				mcp.Required(),
				mcp.Description("AMDP procedure name (e.g., 'ZCL_TEST=>METHOD_NAME')"),
			),
			mcp.WithNumber("line",
				mcp.Required(),
				mcp.Description("Line number in the SQLScript code"),
			),
		), s.handleAMDPSetBreakpoint)
	}

	if shouldRegister("AMDPGetBreakpoints") {
		s.mcpServer.AddTool(mcp.NewTool("AMDPGetBreakpoints",
			mcp.WithDescription("Get all breakpoints registered in the current AMDP debug session. Useful for verifying breakpoints are set correctly."),
		), s.handleAMDPGetBreakpoints)
	}
}

// registerTransportTools registers CTS/Transport management tools.
func (s *Server) registerTransportTools(shouldRegister func(string) bool) {
	if shouldRegister("ListTransports") {
		s.mcpServer.AddTool(mcp.NewTool("ListTransports",
			mcp.WithDescription("List transport requests. Returns modifiable transports for a user. Requires --enable-transports OR --allow-transportable-edits flag."),
			mcp.WithString("user",
				mcp.Description("Username to list transports for (default: current user, '*' for all users)"),
			),
		), s.handleListTransports)
	}

	if shouldRegister("GetTransport") {
		s.mcpServer.AddTool(mcp.NewTool("GetTransport",
			mcp.WithDescription("Get detailed transport information including objects and tasks. Requires --enable-transports OR --allow-transportable-edits flag."),
			mcp.WithString("transport",
				mcp.Required(),
				mcp.Description("Transport request number (e.g., 'A4HK900094')"),
			),
		), s.handleGetTransport)
	}

	if shouldRegister("CreateTransport") {
		s.mcpServer.AddTool(mcp.NewTool("CreateTransport",
			mcp.WithDescription("Create a new transport request. Requires --enable-transports flag and not --transport-read-only."),
			mcp.WithString("description",
				mcp.Required(),
				mcp.Description("Transport description"),
			),
			mcp.WithString("package",
				mcp.Required(),
				mcp.Description("Target package (DEVCLASS)"),
			),
			mcp.WithString("transport_layer",
				mcp.Description("Transport layer (optional)"),
			),
			mcp.WithString("type",
				mcp.Description("Type: 'workbench' (default) or 'customizing'"),
			),
		), s.handleCreateTransport)
	}

	if shouldRegister("ReleaseTransport") {
		s.mcpServer.AddTool(mcp.NewTool("ReleaseTransport",
			mcp.WithDescription("Release a transport request. This action is IRREVERSIBLE. Requires --enable-transports flag and not --transport-read-only."),
			mcp.WithString("transport",
				mcp.Required(),
				mcp.Description("Transport request number"),
			),
			mcp.WithBoolean("ignore_locks",
				mcp.Description("Release even with locked objects (default: false)"),
			),
			mcp.WithBoolean("skip_atc",
				mcp.Description("Skip ATC quality checks (default: false)"),
			),
		), s.handleReleaseTransport)
	}

	if shouldRegister("DeleteTransport") {
		s.mcpServer.AddTool(mcp.NewTool("DeleteTransport",
			mcp.WithDescription("Delete a transport request. Only modifiable transports can be deleted. Requires --enable-transports flag and not --transport-read-only."),
			mcp.WithString("transport",
				mcp.Required(),
				mcp.Description("Transport request number"),
			),
		), s.handleDeleteTransport)
	}
}

// registerGCTSTools registers gCTS (git-enabled Change Transport System) tools.
func (s *Server) registerGCTSTools(shouldRegister func(string) bool) {
	if shouldRegister("GctsListRepositories") {
		s.mcpServer.AddTool(mcp.NewTool("GctsListRepositories",
			mcp.WithDescription("List all gCTS repositories on the SAP system. Returns repository ID, name, URL, branch, status and role."),
		), s.handleGctsListRepositories)
	}

	if shouldRegister("GctsGetRepository") {
		s.mcpServer.AddTool(mcp.NewTool("GctsGetRepository",
			mcp.WithDescription("Get details of a specific gCTS repository including configuration."),
			mcp.WithString("rid",
				mcp.Required(),
				mcp.Description("Repository ID"),
			),
		), s.handleGctsGetRepository)
	}

	if shouldRegister("GctsCreateRepository") {
		s.mcpServer.AddTool(mcp.NewTool("GctsCreateRepository",
			mcp.WithDescription("Create a new gCTS repository. Expert mode only."),
			mcp.WithString("rid",
				mcp.Required(),
				mcp.Description("Repository ID"),
			),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("url",
				mcp.Required(),
				mcp.Description("Git remote URL"),
			),
			mcp.WithString("branch",
				mcp.Description("Default branch (default: main)"),
			),
			mcp.WithString("package",
				mcp.Description("ABAP package (VSID)"),
			),
			mcp.WithString("role",
				mcp.Description("Repository role (SOURCE or TARGET)"),
			),
		), s.handleGctsCreateRepository)
	}

	if shouldRegister("GctsDeleteRepository") {
		s.mcpServer.AddTool(mcp.NewTool("GctsDeleteRepository",
			mcp.WithDescription("Delete a gCTS repository. Expert mode only."),
			mcp.WithString("rid",
				mcp.Required(),
				mcp.Description("Repository ID"),
			),
		), s.handleGctsDeleteRepository)
	}

	if shouldRegister("GctsCloneRepository") {
		s.mcpServer.AddTool(mcp.NewTool("GctsCloneRepository",
			mcp.WithDescription("Clone a gCTS repository on the SAP system. Expert mode only."),
			mcp.WithString("rid",
				mcp.Required(),
				mcp.Description("Repository ID"),
			),
		), s.handleGctsCloneRepository)
	}

	if shouldRegister("GctsPull") {
		s.mcpServer.AddTool(mcp.NewTool("GctsPull",
			mcp.WithDescription("Pull changes into a gCTS repository, optionally to a specific commit. Expert mode only."),
			mcp.WithString("rid",
				mcp.Required(),
				mcp.Description("Repository ID"),
			),
			mcp.WithString("commit_id",
				mcp.Description("Commit ID to pull to (optional)"),
			),
		), s.handleGctsPull)
	}

	if shouldRegister("GctsCommit") {
		s.mcpServer.AddTool(mcp.NewTool("GctsCommit",
			mcp.WithDescription("Create a commit in a gCTS repository. Expert mode only."),
			mcp.WithString("rid",
				mcp.Required(),
				mcp.Description("Repository ID"),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description("Commit message"),
			),
		), s.handleGctsCommit)
	}

	if shouldRegister("GctsListBranches") {
		s.mcpServer.AddTool(mcp.NewTool("GctsListBranches",
			mcp.WithDescription("List branches in a gCTS repository."),
			mcp.WithString("rid",
				mcp.Required(),
				mcp.Description("Repository ID"),
			),
		), s.handleGctsListBranches)
	}

	if shouldRegister("GctsSwitchBranch") {
		s.mcpServer.AddTool(mcp.NewTool("GctsSwitchBranch",
			mcp.WithDescription("Switch the active branch of a gCTS repository. Expert mode only."),
			mcp.WithString("rid",
				mcp.Required(),
				mcp.Description("Repository ID"),
			),
			mcp.WithString("branch",
				mcp.Required(),
				mcp.Description("Branch name to switch to"),
			),
		), s.handleGctsSwitchBranch)
	}

	if shouldRegister("GctsGetHistory") {
		s.mcpServer.AddTool(mcp.NewTool("GctsGetHistory",
			mcp.WithDescription("Get commit history of a gCTS repository."),
			mcp.WithString("rid",
				mcp.Required(),
				mcp.Description("Repository ID"),
			),
		), s.handleGctsGetHistory)
	}
}

// registerGitTools registers Git/abapGit integration tools.
func (s *Server) registerGitTools(shouldRegister func(string) bool) {
	if shouldRegister("GitTypes") {
		s.mcpServer.AddTool(mcp.NewTool("GitTypes",
			mcp.WithDescription("Get list of supported abapGit object types. Returns 158 object types that can be exported/imported via abapGit. Requires abapGit to be installed on SAP system."),
		), s.handleGitTypes)
	}

	if shouldRegister("GitExport") {
		s.mcpServer.AddTool(mcp.NewTool("GitExport",
			mcp.WithDescription("Export ABAP objects as abapGit-compatible ZIP. Supports 158 object types. Saves ZIP file to output_dir (default: current directory). Use packages OR objects parameter."),
			mcp.WithString("packages",
				mcp.Description("Comma-separated package names to export (e.g., '$ZRAY,$TMP'). Supports wildcards."),
			),
			mcp.WithString("objects",
				mcp.Description("JSON array of objects: [{\"type\":\"CLAS\",\"name\":\"ZCL_TEST\"}]"),
			),
			mcp.WithBoolean("include_subpackages",
				mcp.Description("Include subpackages when exporting by package (default: true)"),
			),
			mcp.WithString("output_dir",
				mcp.Description("Output directory for ZIP file (default: current directory)"),
			),
		), s.handleGitExport)
	}
}

// registerReportTools registers report execution tools.
func (s *Server) registerReportTools(shouldRegister func(string) bool) {
	if shouldRegister("RunReport") {
		s.mcpServer.AddTool(mcp.NewTool("RunReport",
			mcp.WithDescription("Execute an ABAP selection-screen report with parameters or variant. Runs as background job and returns spool output. Requires ZADT_VSP WebSocket handler deployed."),
			mcp.WithString("report",
				mcp.Description("Report program name (e.g., 'RFITEMGL', 'ZREPORT_TEST')"),
				mcp.Required(),
			),
			mcp.WithString("variant",
				mcp.Description("Variant name to use for selection screen (optional)"),
			),
			mcp.WithString("params",
				mcp.Description("JSON object with selection screen parameters (e.g., '{\"P_BUKRS\":\"1000\",\"S_KUNNR\":{\"SIGN\":\"I\",\"OPTION\":\"EQ\",\"LOW\":\"0000001000\"}}'). Keys are parameter names."),
			),
		), s.handleRunReport)
	}

	if shouldRegister("RunReportAsync") {
		s.mcpServer.AddTool(mcp.NewTool("RunReportAsync",
			mcp.WithDescription("Start report execution in background. Returns task_id immediately. Use GetAsyncResult to poll for completion. Useful for long-running reports that would timeout."),
			mcp.WithString("report",
				mcp.Description("Report program name"),
				mcp.Required(),
			),
			mcp.WithString("variant",
				mcp.Description("Variant name (optional)"),
			),
			mcp.WithString("params",
				mcp.Description("JSON object with selection screen parameters"),
			),
		), s.handleRunReportAsync)
	}

	if shouldRegister("GetAsyncResult") {
		s.mcpServer.AddTool(mcp.NewTool("GetAsyncResult",
			mcp.WithDescription("Get result of an async task by ID. Returns status (running/completed/error) and result when done."),
			mcp.WithString("task_id",
				mcp.Description("Task ID from RunReportAsync"),
				mcp.Required(),
			),
			mcp.WithBoolean("wait",
				mcp.Description("If true, block until task completes (max 60s). Default: false (poll)"),
			),
		), s.handleGetAsyncResult)
	}

	if shouldRegister("GetVariants") {
		s.mcpServer.AddTool(mcp.NewTool("GetVariants",
			mcp.WithDescription("Get list of available variants for a report program. Returns variant names and whether they are protected."),
			mcp.WithString("report",
				mcp.Description("Report program name"),
				mcp.Required(),
			),
		), s.handleGetVariants)
	}

	if shouldRegister("GetTextElements") {
		s.mcpServer.AddTool(mcp.NewTool("GetTextElements",
			mcp.WithDescription("Get program text elements (selection texts and text symbols). Selection texts describe parameters (P_BUKRS='Company Code'), text symbols are TEXT-001 etc."),
			mcp.WithString("program",
				mcp.Description("Program name"),
				mcp.Required(),
			),
			mcp.WithString("language",
				mcp.Description("Language key (e.g., 'E' for English, 'D' for German). Default: system language."),
			),
		), s.handleGetTextElements)
	}

	if shouldRegister("SetTextElements") {
		s.mcpServer.AddTool(mcp.NewTool("SetTextElements",
			mcp.WithDescription("Set program text elements (selection texts, text symbols, and heading texts). Use for adding descriptions to selection screen parameters, text symbols, and list/column headings."),
			mcp.WithString("program",
				mcp.Description("Program name"),
				mcp.Required(),
			),
			mcp.WithString("language",
				mcp.Description("Language key (e.g., 'E' for English, 'D' for German). Default: system language."),
			),
			mcp.WithString("selection_texts",
				mcp.Description("JSON object of selection texts (e.g., '{\"P_BUKRS\":\"Company Code\",\"S_KUNNR\":\"Customer Range\"}')"),
			),
			mcp.WithString("text_symbols",
				mcp.Description("JSON object of text symbols (e.g., '{\"001\":\"Header Text\",\"002\":\"Footer\"}')"),
			),
			mcp.WithString("heading_texts",
				mcp.Description("JSON object of heading texts for list/column headings (e.g., '{\"001\":\"Report Title\",\"002\":\"Column Header\"}')"),
			),
		), s.handleSetTextElements)
	}
}

// registerInstallTools registers install/setup tools.
func (s *Server) registerInstallTools(shouldRegister func(string) bool) {
	if shouldRegister("InstallZADTVSP") {
		s.mcpServer.AddTool(mcp.NewTool("InstallZADTVSP",
			mcp.WithDescription("Deploy ZADT_VSP WebSocket handler to SAP system. Creates package and deploys 6 ABAP objects (interface + 5 classes) that enable WebSocket debugging, RFC calls, and abapGit export. After deployment, manual SAPC and SICF setup is required."),
			mcp.WithString("package",
				mcp.Description("Target package name (default: $ZADT_VSP). Must be local package starting with $."),
			),
			mcp.WithBoolean("skip_git_service",
				mcp.Description("Skip ZCL_VSP_GIT_SERVICE deployment if abapGit is not installed (default: false, auto-detected)"),
			),
			mcp.WithBoolean("check_only",
				mcp.Description("Only check prerequisites without deploying (default: false)"),
			),
		), s.handleInstallZADTVSP)
	}

	if shouldRegister("ListDependencies") {
		s.mcpServer.AddTool(mcp.NewTool("ListDependencies",
			mcp.WithDescription("List available dependency packages that can be installed via InstallAbapGit. Shows abapGit editions and other optional dependencies."),
		), s.handleListDependencies)
	}

	if shouldRegister("InstallAbapGit") {
		s.mcpServer.AddTool(mcp.NewTool("InstallAbapGit",
			mcp.WithDescription("Deploy abapGit to SAP system from embedded ZIP. Supports standalone (single program) or developer edition (full package structure). Parses abapGit-format ZIP and deploys via WriteSource."),
			mcp.WithString("edition",
				mcp.Description("Edition to install: 'standalone' (single program ZABAPGIT) or 'dev' (full $ZGIT_DEV packages). Default: standalone"),
			),
			mcp.WithString("package",
				mcp.Description("Target package name. Default: $ABAPGIT for standalone, $ZGIT_DEV for dev edition"),
			),
			mcp.WithBoolean("check_only",
				mcp.Description("Only check prerequisites and show deployment plan without deploying (default: false)"),
			),
		), s.handleInstallAbapGit)
	}

	if shouldRegister("InstallDummyTest") {
		s.mcpServer.AddTool(mcp.NewTool("InstallDummyTest",
			mcp.WithDescription("Test tool that creates a simple interface and class to verify the Install* workflow (create, lock, update, unlock, activate, verify). Uses package $ZADT_INSTALL_TEST."),
			mcp.WithBoolean("check_only",
				mcp.Description("Only check prerequisites without deploying (default: false)"),
			),
			mcp.WithBoolean("cleanup",
				mcp.Description("Delete test objects after verification (default: false)"),
			),
		), s.handleInstallDummyTest)
	}

	if shouldRegister("DeployZip") {
		s.mcpServer.AddTool(mcp.NewTool("DeployZip",
			mcp.WithDescription("Deploy objects from an embedded abapGit-format ZIP to a SAP package. Uses ADT native deployment (PROG, CLAS, INTF, DDLS, BDEF, SRVD). For full 158 object type support, install ZADT_VSP first."),
			mcp.WithString("source",
				mcp.Required(),
				mcp.Description("Embedded dependency name: 'abapgit-standalone' (single ZABAPGIT program) or 'abapgit-full' (564 objects - full developer edition)"),
			),
			mcp.WithString("package",
				mcp.Required(),
				mcp.Description("Target SAP package name (e.g., '$ZGIT'). Package will be created if it doesn't exist."),
			),
			mcp.WithBoolean("dry_run",
				mcp.Description("Show deployment plan without deploying (default: false)"),
			),
			mcp.WithString("type_filter",
				mcp.Description("Deploy only objects of this type (e.g., 'PROG', 'CLAS', 'INTF')"),
			),
			mcp.WithString("name_filter",
				mcp.Description("Deploy only objects matching this name pattern (e.g., 'ZCL_ABAPGIT_*')"),
			),
		), s.handleDeployZip)
	}
}

// registerVersionHistoryTools registers version history and comparison tools.
func (s *Server) registerVersionHistoryTools(shouldRegister func(string) bool) {
	if shouldRegister("GetRevisions") {
		s.mcpServer.AddTool(mcp.NewTool("GetRevisions",
			mcp.WithDescription("List version history (revisions) of an ABAP object. Returns versions with dates, authors, and transport requests. Use version URIs with GetRevisionSource or CompareVersions."),
			mcp.WithString("type",
				mcp.Required(),
				mcp.Description("Object type: PROG, CLAS, INTF, FUNC, INCL, DDLS, BDEF, SRVD"),
			),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Object name"),
			),
			mcp.WithString("include",
				mcp.Description("Class include type for CLAS: main, definitions, implementations, macros, testclasses (default: main)"),
			),
			mcp.WithString("parent",
				mcp.Description("Function group name (required for FUNC type)"),
			),
		), s.handleGetRevisions)
	}

	if shouldRegister("GetRevisionSource") {
		s.mcpServer.AddTool(mcp.NewTool("GetRevisionSource",
			mcp.WithDescription("Get source code of a specific version of an ABAP object. Use version_uri from GetRevisions output."),
			mcp.WithString("version_uri",
				mcp.Required(),
				mcp.Description("Version URI from GetRevisions result (the 'uri' field of a revision entry)"),
			),
		), s.handleGetRevisionSource)
	}

	if shouldRegister("CompareVersions") {
		s.mcpServer.AddTool(mcp.NewTool("CompareVersions",
			mcp.WithDescription("Compare two versions of an ABAP object with unified diff. Use version URIs from GetRevisions. Use 'current' as version2_uri to compare against the active version."),
			mcp.WithString("type",
				mcp.Required(),
				mcp.Description("Object type: PROG, CLAS, INTF, FUNC, INCL, DDLS, BDEF, SRVD"),
			),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Object name"),
			),
			mcp.WithString("version1_uri",
				mcp.Required(),
				mcp.Description("Version URI for first (older) version, from GetRevisions"),
			),
			mcp.WithString("version2_uri",
				mcp.Description("Version URI for second (newer) version, from GetRevisions. Default: 'current' (compare against active version)"),
			),
			mcp.WithString("include",
				mcp.Description("Class include type for CLAS: main, definitions, implementations, macros, testclasses"),
			),
			mcp.WithString("parent",
				mcp.Description("Function group name (required for FUNC type)"),
			),
		), s.handleCompareVersions)
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
}

// registerCodeAnalysisTools registers ABAP code analysis tools.
func (s *Server) registerCodeAnalysisTools(shouldRegister func(string) bool) {
	if shouldRegister("AnalyzeABAPCode") {
		s.mcpServer.AddTool(mcp.NewTool("AnalyzeABAPCode",
			mcp.WithDescription("Analyze ABAP source code quality using the native abaplint lexer+parser. Detects: SELECT *, hardcoded credentials, overly broad CATCH cx_root, COMMIT WORK in loops, dynamic calls without TRY, obsolete statements (MOVE/ADD/COMPUTE), naming conventions, and more. Returns findings with line numbers, severity, and fix suggestions. Provide source directly or specify object_type+object_name to fetch from SAP."),
			mcp.WithString("object_type",
				mcp.Description("ABAP object type: PROG, CLAS, INTF, FUNC, FUGR, INCL, DDLS (used with object_name to fetch source)"),
			),
			mcp.WithString("object_name",
				mcp.Description("ABAP object name (e.g., ZCL_EXAMPLE, ZTEST_REPORT). Used with object_type to fetch source from SAP."),
			),
			mcp.WithString("source",
				mcp.Description("ABAP source code to analyze directly (alternative to object_type+object_name)"),
			),
		), s.handleAnalyzeABAPCode)
	}
}

// registerI18NTools registers i18n/translation tools.
func (s *Server) registerI18NTools(shouldRegister func(string) bool) {
	if shouldRegister("GetObjectTextsInLanguage") {
		s.mcpServer.AddTool(mcp.NewTool("GetObjectTextsInLanguage",
			mcp.WithDescription("Get the source/content of an ABAP object in a specific language. The language parameter overrides the session language for this request only, allowing you to read texts in any installed language."),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT source URL (e.g., /sap/bc/adt/programs/programs/ZTEST/source/main)"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("ISO language code (e.g., EN, DE, FR, ES, JA)"),
			),
		), s.handleGetObjectTextsInLanguage)
	}

	if shouldRegister("GetDataElementLabels") {
		s.mcpServer.AddTool(mcp.NewTool("GetDataElementLabels",
			mcp.WithDescription("Get the text labels (short/medium/long/heading) of a data element in a specific language. Useful for checking translations of DDIC texts."),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Data element name (e.g., MATNR, ZTEST_DTEL)"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("ISO language code (e.g., EN, DE, FR)"),
			),
		), s.handleGetDataElementLabels)
	}

	if shouldRegister("GetMessageClassTexts") {
		s.mcpServer.AddTool(mcp.NewTool("GetMessageClassTexts",
			mcp.WithDescription("Get all messages of a message class in a specific language. Returns message number and text for each entry."),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Message class name (e.g., ZTEST_MC)"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("ISO language code (e.g., EN, DE, FR)"),
			),
		), s.handleGetMessageClassTexts)
	}

	if shouldRegister("WriteMessageClassTexts") {
		s.mcpServer.AddTool(mcp.NewTool("WriteMessageClassTexts",
			mcp.WithDescription("Update message class texts in a specific language. Requires a lock handle from LockObject. Use for translating message class entries."),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Message class name"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("ISO language code (e.g., EN, DE, FR)"),
			),
			mcp.WithString("lock_handle",
				mcp.Required(),
				mcp.Description("Lock handle from LockObject"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for $TMP objects)"),
			),
		), s.handleWriteMessageClassTexts)
	}

	if shouldRegister("WriteDataElementLabels") {
		s.mcpServer.AddTool(mcp.NewTool("WriteDataElementLabels",
			mcp.WithDescription("Update data element labels (short/medium/long/heading) in a specific language. Requires a lock handle from LockObject."),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Data element name"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("ISO language code (e.g., EN, DE, FR)"),
			),
			mcp.WithString("lock_handle",
				mcp.Required(),
				mcp.Description("Lock handle from LockObject"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for $TMP objects)"),
			),
			mcp.WithString("short",
				mcp.Description("Short description (max 10 chars)"),
			),
			mcp.WithString("medium",
				mcp.Description("Medium description (max 20 chars)"),
			),
			mcp.WithString("long",
				mcp.Description("Long description (max 40 chars)"),
			),
			mcp.WithString("heading",
				mcp.Description("Column heading (max 55 chars)"),
			),
		), s.handleWriteDataElementLabels)
	}

	if shouldRegister("GetTextPool") {
		s.mcpServer.AddTool(mcp.NewTool("GetTextPool",
			mcp.WithDescription("Get the text pool (text elements/symbols) of a program in a specific language. Returns ID, key, and text for each entry."),
			mcp.WithString("program_name",
				mcp.Required(),
				mcp.Description("Program name (e.g., ZTEST)"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("ISO language code (e.g., EN, DE, FR)"),
			),
		), s.handleGetTextPoolInLanguage)
	}

	if shouldRegister("CompareLanguages") {
		s.mcpServer.AddTool(mcp.NewTool("CompareLanguages",
			mcp.WithDescription("Compare the text content of an ABAP object in two languages. Shows which lines differ or are missing in the target language. Useful for identifying untranslated or outdated translations."),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT source URL (e.g., /sap/bc/adt/programs/programs/ZTEST/source/main)"),
			),
			mcp.WithString("source_language",
				mcp.Required(),
				mcp.Description("Source language code (e.g., EN)"),
			),
			mcp.WithString("target_language",
				mcp.Required(),
				mcp.Description("Target language code (e.g., DE, FR)"),
			),
		), s.handleCompareObjectLanguages)
	}
}
