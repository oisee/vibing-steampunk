// Package mcp provides the MCP server implementation for ABAP ADT tools.
// tools_report.go registers report execution tools.
package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// registerReportTools registers report execution tools.
func (s *Server) registerReportTools(shouldRegister func(string) bool) {
	if shouldRegister("RunReport") {
		s.mcpServer.AddTool(mcp.NewTool("RunReport",
			mcp.WithDescription("Execute an ABAP selection-screen report with parameters or variant. Runs as background job and returns spool output, plus the rows as JSON for ALV reports. Requires ZADT_VSP WebSocket handler deployed."),
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
