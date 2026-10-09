// Package mcp provides the MCP server implementation for ABAP ADT tools.
// tools_system.go registers system info and diagnostics tools (dumps, traces, SQL traces).
package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
)

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

// registerDiagnosticsTools registers runtime error, profiler, and SQL trace tools.
func (s *Server) registerDiagnosticsTools(shouldRegister func(string) bool) {
	// --- Runtime Errors / Short Dumps (RABAX) ---
	if shouldRegister("ListDumps") {
		s.mcpServer.AddTool(mcp.NewTool("ListDumps",
			mcp.WithDescription("List runtime errors (short dumps) from the SAP system, newest first. Filter by error type, program, user, date range. "+
				"Grouping, correlation with the application log, similarity and blast radius are reachable through SAP(action=\"analyze\") in hyperfocused mode."),
			mcp.WithString("user",
				mcp.Description("Filter by username"),
			),
			mcp.WithString("error_type",
				mcp.Description("Filter by runtime error, e.g. CALL_FUNCTION_NOT_REMOTE or CX_SY_ZERODIVIDE (alias: exception_type)"),
			),
			mcp.WithString("program",
				mcp.Description("Filter by terminated program"),
			),
			mcp.WithString("since",
				mcp.Description("Earliest date, YYYY-MM-DD or YYYYMMDD (alias: date_from)"),
			),
			mcp.WithString("until",
				mcp.Description("Latest date, YYYY-MM-DD or YYYYMMDD (alias: date_to)"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of results (default: 100)"),
			),
		), s.handleListDumps)
	}

	if shouldRegister("GetDump") {
		s.mcpServer.AddTool(mcp.NewTool("GetDump",
			mcp.WithDescription("Get one runtime error in detail: header, termination point, application component and call stack. "+
				"A release that serves the dump feed without the detail resource says so in notes rather than failing."),
			mcp.WithString("dump_id",
				mcp.Required(),
				mcp.Description("Dump ID from ListDumps, part of one, or \"latest\""),
			),
		), s.handleGetDump)
	}

	// --- ABAP Profiler / Runtime Traces (ATRA) ---
	if shouldRegister("ListTraces") {
		s.mcpServer.AddTool(mcp.NewTool("ListTraces",
			mcp.WithDescription("List ABAP runtime traces (profiler results) with their state, aggregation, size and runtime. complete=false marks a trace ADT cannot evaluate, such as one cut off at its size limit."),
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
			mcp.WithDescription("Evaluate an ABAP runtime trace: hit list, call tree (statements) or database accesses, sorted by own time and cut to the top entries. A trace cut off at its size limit cannot be evaluated; ListTraces shows its state."),
			mcp.WithString("trace_id",
				mcp.Required(),
				mcp.Description("Trace ID from ListTraces, its ADT path, or an adt:// link"),
			),
			mcp.WithString("tool_type",
				mcp.Description("Analysis type: 'hitlist' (default), 'statements' (call tree; non-aggregated traces only), 'dbAccesses'"),
			),
			mcp.WithString("sort_by",
				mcp.Description("'net' (own time, default), 'gross', 'calls', or 'none' for SAP's order (the call tree's default)"),
			),
			mcp.WithNumber("top",
				mcp.Description("Entries to return after sorting (default 50, 0 for all)"),
			),
			mcp.WithBoolean("raw",
				mcp.Description("Return ADT's XML unparsed, capped at 256 KB"),
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
