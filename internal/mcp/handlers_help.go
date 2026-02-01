// ABOUTME: Provides help documentation for the SAP MCP tool.
// ABOUTME: Returns structured documentation based on action type.
package mcp

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// handleHelp returns documentation for the SAP tool
func handleHelp(target string) *mcp.CallToolResult {
	target = strings.TrimSpace(target)
	targetLower := strings.ToLower(target)

	// Handle "ABAP <keyword>" for ABAP language help
	if strings.HasPrefix(targetLower, "abap ") {
		keyword := strings.TrimSpace(target[5:])
		return handleAbapKeywordHelp(keyword)
	}

	switch targetLower {
	case "read":
		return mcp.NewToolResultText(`READ - Retrieve source code and metadata

Source objects:
  read CLAS <name>     - Class (combined definition + implementation)
  read INTF <name>     - Interface
  read PROG <name>     - Program/Report
  read INCL <name>     - Include
  read FUNC <name>     - Function module (needs params.function_group)
  read DDLS <name>     - CDS DDL Source
  read TABL <name>     - Table definition
  read DTEL <name>     - Data element
  read DOMA <name>     - Domain

Metadata:
  read FUGR <name>     - Function group (JSON)
  read DEVC <name>     - Package contents (JSON)
  read MSAG <name>     - Message class (JSON)
  read TRAN <name>     - Transaction (JSON)
  read CLAS_INFO <name> - Class metadata (JSON)
  read TYPE_INFO <name> - Type information (JSON)

Class includes:
  read CLAS_INCLUDE <class> params.include_type=testclasses|locals_def|locals_imp`)

	case "edit":
		return mcp.NewToolResultText(`EDIT - Modify source code and metadata

High-level (recommended):
  edit CLAS <name>     params.source="..." [params.test_source, params.package, params.description]
  edit INTF <name>     params.source="..." [params.package, params.description]
  edit PROG <name>     params.source="..." [params.package, params.description]
  edit DDLS <name>     params.source="..."
  edit BDEF <name>     params.source="..."
  edit SRVD <name>     params.source="..."
  edit SRVB <name>     params.source="..."

  Uses upsert mode: creates if not exists, updates if exists.
  Default package: $TMP

Method-level update (class only):
  edit CLAS <name>     params.source="METHOD...ENDMETHOD", params.method="<method_name>"

  Updates only the specified method. Source must be the complete METHOD...ENDMETHOD block.

Description-only update (no source change):
  edit CLAS <name>     params.description="New description"
  edit INTF <name>     params.description="New description"
  edit PROG <name>     params.description="New description"

Low-level (for fine control):
  edit LOCK            params.object_url="/sap/bc/adt/..." → returns lock_handle
  edit UPDATE_SOURCE   params.object_url, params.source, params.lock_handle
  edit UNLOCK          params.object_url, params.lock_handle
  edit MOVE            params.object_type, params.object_name, params.new_package

Class includes:
  edit CLAS_INCLUDE <class>  params.include_type, params.source, params.lock_handle`)

	case "create":
		return mcp.NewToolResultText(`CREATE - Create new objects

Objects:
  create OBJECT        params.object_type="CLAS/OC"|"INTF/OI"|"PROG/P"|..., params.name, params.description, params.package_name
  create DEVC <name>   params.description [params.parent] - Create package (name must start with $)
  create TABL <name>   params.description, params.fields (JSON array)
  create CLONE         params.object_type, params.source_name, params.target_name, params.package

Class includes:
  create CLAS_TEST_INCLUDE <class>  params.lock_handle

Object types for create OBJECT:
  CLAS/OC - Class
  INTF/OI - Interface
  PROG/P  - Program
  PROG/I  - Include
  FUGR/F  - Function Group
  DDLS/DF - CDS DDL Source
  BDEF/BDO - Behavior Definition
  SRVD/SRV - Service Definition
  SRVB/SVB - Service Binding`)

	case "delete":
		return mcp.NewToolResultText(`DELETE - Remove objects

  delete OBJECT        params.object_url, params.lock_handle [params.transport]

Note: Lock the object first with 'edit LOCK' to get lock_handle`)

	case "search":
		return mcp.NewToolResultText(`SEARCH - Find objects

  search <query>       - Search by name pattern
  search <query>       params.max_results=N, params.object_type="CLAS"|"PROG"|...`)

	case "test":
		return mcp.NewToolResultText(`TEST - Run tests

Unit tests:
  test                 params.object_url="/sap/bc/adt/oo/classes/<name>"
                       [params.with_coverage=true]

ATC (ABAP Test Cockpit):
  test CLAS <name>     params.type="atc" [params.variant, params.max_results]
  test PROG <name>     params.type="atc"
  test INTF <name>     params.type="atc"
  test FUGR <name>     params.type="atc"
  test DDLS <name>     params.type="atc"
  test BDEF <name>     params.type="atc"

Example:
  test CLAS ZCL_CSV params.type="atc" params.variant="DEFAULT"`)

	case "query":
		return mcp.NewToolResultText(`QUERY - Database queries

  query TABL_CONTENTS <table>  [params.max_rows, params.sql_query]
  query SQL                    params.sql_query [params.max_rows]`)

	case "system":
		return mcp.NewToolResultText(`SYSTEM - System operations

  system info                          - System information
  system               params.type=install_zadt_vsp [params.package, params.check_only]
  system               params.type=install_abapgit [params.edition, params.check_only]
  system               params.type=list_dependencies
  system               params.type=install_dummy_test [params.check_only, params.cleanup]`)

	case "grep":
		return mcp.NewToolResultText(`GREP - Search code

  grep <pattern>       params.package="ZPACKAGE" [params.object_type, params.max_results]`)

	case "abap":
		return mcp.NewToolResultText(`ABAP - ABAP language help

  help ABAP <keyword>  - Get help URL and search query for ABAP keyword

Examples:
  help ABAP SELECT     - SELECT statement documentation
  help ABAP LOOP       - LOOP statement documentation
  help ABAP DATA       - DATA statement documentation

Returns:
  - URL to SAP Help Portal
  - Search query to use with web search for detailed documentation`)

	case "debug":
		return mcp.NewToolResultText(`DEBUG - Debugging operations

TWO BREAKPOINT MECHANISMS:

1. TPDAPI Breakpoints (ABDBG_EXTDBPS table - Eclipse ADT style):
  debug set_breakpoint     params.program, params.line [params.kind, params.method]
  debug get_breakpoints    - List TPDAPI breakpoints
  debug delete_breakpoint  <breakpoint_id>
  → Checked by: Eclipse ADT (RFC), SAP GUI with external debugging enabled
  → Use with: listen/attach for REST-based debugging

2. HTTP Breakpoints (ABDBG_BPS table - CL_ABAP_DEBUGGER):
  debug set_http_breakpoint   params.program, params.line, params.method (required for classes)
  debug get_http_breakpoints  - List HTTP breakpoints
  debug delete_http_breakpoints - Delete all HTTP breakpoints
  → Checked by: HTTP execution (classrun, REST calls, unit tests via HTTP)
  → Opens: SAP GUI debugger directly (not REST session)

Code execution:
  debug classrun <class>   - Execute via ADT classrun endpoint
  debug run_report         params.report [params.variant] - Trigger via background job

RFC (WebSocket):
  debug call_rfc           params.function [params.params (JSON)]

REST debug session (for TPDAPI breakpoints only):
  debug listen             [params.user, params.timeout] - Wait for debuggee
  debug attach             params.debuggee_id - Attach to debuggee
  debug detach             - Detach from session
  debug step               params.step_type=stepInto|stepOver|stepReturn|stepContinue
  debug get_stack          - Show call stack
  debug get_variables      [params.variable_ids] - Inspect variables

WORKFLOW - HTTP Breakpoints (SAP GUI):
  1. Set breakpoint:
     debug set_http_breakpoint params.program="ZCL_TEST" params.method="MY_METHOD" params.line=5
  2. Verify: debug get_http_breakpoints
  3. Trigger: debug classrun ZCL_TEST
  4. SAP GUI debugger opens automatically at the breakpoint
  Note: For classes, params.method is REQUIRED to resolve the include name.
        Line number is relative to method start (line 1 = first line of method).

WORKFLOW - TPDAPI Breakpoints (REST session):
  1. Set breakpoint: debug set_breakpoint params.program="ZPROG" params.line=10
  2. Listen: debug listen params.timeout=60
  3. (Trigger execution in another session)
  4. Attach: debug attach params.debuggee_id="<from listen>"
  5. Step: debug step params.step_type="stepOver"

BREAKPOINT TABLE COMPARISON:
  | Type | Table | Set via | Debugger |
  |------|-------|---------|----------|
  | TPDAPI | ABDBG_EXTDBPS | set_breakpoint | REST (listen/attach) |
  | HTTP | ABDBG_BPS | set_http_breakpoint | SAP GUI (auto-opens) |`)

	default:
		return mcp.NewToolResultText(`SAP ABAP Development Tool

ACTIONS:
  help [topic]  - Show this help or help for specific action
  read          - Retrieve source code and metadata
  edit          - Modify source code (high-level with params.source, or low-level with lock/unlock)
  create        - Create new objects
  delete        - Remove objects
  search        - Find objects by name
  query         - Database queries
  test          - Run unit tests
  grep          - Search code content
  debug         - Debugging operations
  system        - System operations and installations

COMMON PATTERNS:
  read CLAS ZCL_TEST                    - Read class source
  edit CLAS ZCL_TEST params.source="..."  - Write class source (upsert)
  edit INTF ZIF_TEST params.source="..."  - Write interface source
  search ZCL_*                          - Search for classes
  create OBJECT params.object_type="CLAS/OC" params.name="ZCL_NEW" ...

Run 'help <action>' for detailed documentation, e.g., 'help edit'`)
	}
}

// handleAbapKeywordHelp returns help information for an ABAP keyword
func handleAbapKeywordHelp(keyword string) *mcp.CallToolResult {
	if keyword == "" {
		return mcp.NewToolResultText(`ABAP keyword help requires a keyword.

Examples:
  help ABAP SELECT
  help ABAP LOOP
  help ABAP DATA
  help ABAP METHOD`)
	}

	url := adt.GetAbapHelpURL(keyword)
	query := adt.FormatAbapHelpQuery(keyword)

	return mcp.NewToolResultText(fmt.Sprintf(`ABAP Keyword: %s

Documentation URL:
  %s

Search Query (for web search):
  %s

Tip: Use the search query with a web search tool to get detailed, AI-readable documentation.`,
		strings.ToUpper(keyword), url, query))
}

// getUnhandledErrorMessage returns a helpful error message with suggestions
func getUnhandledErrorMessage(action, objectType, objectName string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Unknown route: %s %s", action, objectType)
	if objectName != "" {
		sb.WriteString(" " + objectName)
	}
	sb.WriteString("\n\n")

	switch action {
	case "edit":
		switch objectType {
		case "CLAS", "INTF", "PROG":
			fmt.Fprintf(&sb, "Did you forget params.source or params.description? Try:\n  edit %s %s params.source=\"...\"\n  edit %s %s params.description=\"...\"", objectType, objectName, objectType, objectName)
		case "DDLS", "BDEF", "SRVD", "SRVB":
			fmt.Fprintf(&sb, "Did you forget params.source? Try:\n  edit %s %s params.source=\"...\"", objectType, objectName)
		case "":
			sb.WriteString("Missing target. Examples:\n  edit CLAS ZCL_TEST params.source=\"...\"\n  edit CLAS ZCL_TEST params.description=\"...\"\n  edit LOCK params.object_url=\"...\"")
		default:
			fmt.Fprintf(&sb, "Unknown edit target '%s'. Valid targets:\n", objectType)
			sb.WriteString("  High-level: CLAS, INTF, PROG, DDLS, BDEF, SRVD, SRVB (with params.source or params.description)\n")
			sb.WriteString("  Low-level: LOCK, UNLOCK, UPDATE_SOURCE, MOVE")
		}

	case "create":
		switch objectType {
		case "CLAS", "INTF", "PROG":
			fmt.Fprintf(&sb, "To create a %s, use:\n  create OBJECT params.object_type=\"%s/OC\" params.name=\"%s\" params.description=\"...\" params.package_name=\"$TMP\"",
				objectType, objectType, objectName)
		case "":
			sb.WriteString("Missing target. Examples:\n  create OBJECT params.object_type=\"CLAS/OC\" params.name=\"ZCL_TEST\" ...\n  create DEVC $ZPACKAGE params.description=\"...\"")
		default:
			fmt.Fprintf(&sb, "Unknown create target '%s'. Valid targets: OBJECT, DEVC, TABL, CLONE", objectType)
		}

	case "read":
		if objectType == "" {
			sb.WriteString("Missing target. Examples:\n  read CLAS ZCL_TEST\n  read PROG ZREPORT\n  read DEVC $ZPACKAGE")
		} else {
			fmt.Fprintf(&sb, "Unknown read target '%s'. Valid targets:\n", objectType)
			sb.WriteString("  Source: CLAS, INTF, PROG, INCL, FUNC, DDLS, TABL, DTEL, DOMA, BDEF, SRVD\n")
			sb.WriteString("  Metadata: FUGR, DEVC, MSAG, TRAN, CLAS_INFO, TYPE_INFO")
		}

	case "delete":
		sb.WriteString("Delete requires:\n  delete OBJECT params.object_url=\"...\" params.lock_handle=\"...\"")

	case "deploy":
		sb.WriteString("'deploy' action doesn't exist. Use 'edit' instead:\n")
		if objectType != "" && objectName != "" {
			fmt.Fprintf(&sb, "  edit %s %s params.source=\"...\"", objectType, objectName)
		} else {
			sb.WriteString("  edit CLAS ZCL_TEST params.source=\"...\"")
		}

	default:
		sb.WriteString("Run 'help' for available actions, or 'help <action>' for specific documentation.")
	}

	return sb.String()
}
