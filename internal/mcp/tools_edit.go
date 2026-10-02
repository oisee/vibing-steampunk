// Package mcp provides the MCP server implementation for ABAP ADT tools.
// tools_edit.go registers class include, EditSource and file import/export tools.
package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
)

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
				mcp.Description("Optional lock handle. Omit it and this call takes and releases its own lock on the parent class (#169)."),
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
				mcp.Description("Optional lock handle. Omit it and this call takes and releases its own lock on the parent class (#169)."),
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

// registerFileTools registers file-based deployment tools.
func (s *Server) registerFileTools(shouldRegister func(string) bool) {
	if shouldRegister("DeployFromFile") {
		s.mcpServer.AddTool(mcp.NewTool("DeployFromFile",
			mcp.WithDescription("✅ RECOMMENDED - Smart deploy from file: auto-detects if object exists and creates/updates accordingly. Solves token limit problem for large generated files (ML models, 3948+ lines). Example: DeployFromFile(file_path=\"/path/to/zcl_ml_iris.clas.abap\", package_name=\"$ZAML_IRIS\") deploys any size file. Workflow: Parse → Check existence → Create or Update → Lock → SyntaxCheck → Write → Unlock → Activate. Supports .clas.abap (and its .clas.testclasses/.locals_def/.locals_imp/.macros.abap includes), .prog.abap, .incl.abap, .intf.abap, .fugr.abap, function modules as {group}.fugr.{module}.abap (abapGit) or {group}.fugr.{module}.func.abap, and the RAP suffixes. A typed file deploys to the object in its file name (# for /), never the one in its content; a file whose main statement names another object is refused. A .prog.abap whose sibling .prog.xml says SUBC I is that include. A plain {name}.abap is typed from its first statement only when that statement names {name}; otherwise it is refused (or read as an include when no REPORT/CLASS/INTERFACE/FUNCTION statement opens it). Use this for all file-based deployments."),
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
			mcp.WithString("expected_source_hash",
				mcp.Description("Optional sourceHash returned by GetSource(include_hash=true). Refuse an existing-object deployment if SAP source has changed."),
			),
			mcp.WithNumber("timeout",
				mcp.Description(callTimeoutDescription),
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
			mcp.WithString("expected_source_hash",
				mcp.Description("Optional sourceHash returned by GetSource(include_hash=true). After locking, refuse the edit if SAP source has changed."),
			),
			mcp.WithNumber("timeout",
				mcp.Description(callTimeoutDescription),
			),
		), s.handleEditSource)
	}
}
