// Package mcp provides the MCP server implementation for ABAP ADT tools.
// tools_read.go registers object read and version-history tools (GetSource, GetProgram, GetRevisions, ...).
package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
)

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
				mcp.Description("Maximum number of rows to retrieve (default 100). Ignored if all_rows is true. Use this instead of SQL LIMIT clause"),
			),
			mcp.WithBoolean("all_rows",
				mcp.Description("Set true to retrieve every matching row, ignoring max_rows and the 100-row default. Prefer this over max_rows: 0 for \"all rows\" — MCP clients may not transmit 0 or negative numbers reliably; a boolean has no such ambiguity."),
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
			mcp.WithDescription("Execute a freestyle SQL query against the SAP database. Uses ABAP SQL syntax, not standard SQL; common ANSI spellings are rewritten and noted (t.col to t~col, ASC/DESC to ASCENDING/DESCENDING, a closing period or semicolon dropped). Use max_rows instead of LIMIT. GROUP BY and WHERE work normally."),
			mcp.WithString("sql_query",
				mcp.Required(),
				mcp.Description("ABAP SQL query. Example: SELECT carrid, COUNT(*) as cnt FROM sflight GROUP BY carrid ORDER BY cnt DESCENDING. ASC/DESC after a sort column are rewritten to ASCENDING/DESCENDING"),
			),
			mcp.WithNumber("max_rows",
				mcp.Description("Maximum number of rows to retrieve (default 100). Ignored if all_rows is true. Use this instead of SQL LIMIT clause"),
			),
			mcp.WithBoolean("all_rows",
				mcp.Description("Set true to retrieve every matching row, ignoring max_rows and the 100-row default. Prefer this over max_rows: 0 for \"all rows\" — MCP clients may not transmit 0 or negative numbers reliably; a boolean has no such ambiguity."),
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
			mcp.WithDescription("Retrieve ABAP package details. With inventory=true: every TADIR object (type, name, author, created on), the subpackages (TDEVC) and any abapGit repository registered for the package, in one read-only call."),
			mcp.WithString("package_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP package"),
			),
			mcp.WithBoolean("inventory",
				mcp.Description("Return the package inventory: TADIR objects with author and created on, subpackages, abapGit repository. Reads TADIR/TDEVC/ZABAPGIT through the data preview; with --block-free-sql only the ADT package contents, and the result names what was skipped."),
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
			mcp.WithDescription("Retrieve ABAP transaction details: description, package, and what it starts (program, screen, parameter, called transaction, OO class/method)"),
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
