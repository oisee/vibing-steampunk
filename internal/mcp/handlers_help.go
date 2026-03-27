// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_help.go provides help documentation for the universal SAP tool.
package mcp

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// handleHelp returns help documentation for the universal SAP tool.
func handleHelp(topic string) *mcp.CallToolResult {
	topic = strings.ToLower(strings.TrimSpace(topic))

	switch topic {
	case "read":
		return mcp.NewToolResultText(`SAP(action="read") - Read source code and metadata

Read source with context (recommended):
  SAP(action="read", target="CLAS ZCL_TEST")
  SAP(action="read", target="PROG ZREPORT")
  SAP(action="read", target="INTF ZIF_TEST")
  SAP(action="read", target="FUNC ZGET_DATA", params={"parent": "ZFUNC_GROUP"})
  SAP(action="read", target="DDLS ZDDL_VIEW")
  SAP(action="read", target="BDEF ZBDEF_NAME")
  SAP(action="read", target="SRVD ZSRVD_NAME")
  SAP(action="read", target="INCL ZINCLUDE_NAME")
  SAP(action="read", target="FUGR ZFUNC_GROUP")

Read with options:
  SAP(action="read", target="CLAS ZCL_TEST", params={"include": "testclasses"})
  SAP(action="read", target="CLAS ZCL_TEST", params={"method": "GET_DATA"})
  SAP(action="read", target="CLAS ZCL_TEST", params={"include_context": false})

Read metadata:
  SAP(action="read", target="TABL ZTABLE")          - Table definition
  SAP(action="read", target="TABL_CONTENTS ZTABLE") - Table data
  SAP(action="read", target="DEVC $TMP")             - Package info
  SAP(action="read", target="MSAG ZMSG_CLASS")       - Message class
  SAP(action="read", target="TRAN SM30")              - Transaction info
  SAP(action="read", target="TYPE_INFO ZTYPE")        - Type info
  SAP(action="read", target="STRUCT ZSTRUCT")         - Structure definition
  SAP(action="read", target="CDS_DEPS ZDDL_VIEW")    - CDS dependencies

Query data:
  SAP(action="query", target="TABL_CONTENTS ZTABLE", params={"max_rows": 50})
  SAP(action="query", target="SQL", params={"sql_query": "SELECT * FROM T000", "max_rows": 10})`)

	case "edit":
		return mcp.NewToolResultText(`SAP(action="edit") - Edit source code

High-level edit (recommended - auto lock/unlock/activate):
  SAP(action="edit", target="CLAS ZCL_TEST", params={"source": "CLASS zcl_test..."})
  SAP(action="edit", target="PROG ZREPORT", params={"source": "REPORT zreport..."})
  SAP(action="edit", target="INTF ZIF_TEST", params={"source": "INTERFACE zif_test..."})
  SAP(action="edit", target="DDLS ZDDL_VIEW", params={"source": "@AbapCatalog..."})

Method-level edit (CLAS only):
  SAP(action="edit", target="CLAS ZCL_TEST", params={"source": "METHOD get_data...ENDMETHOD.", "method": "GET_DATA"})

Surgical edit (find and replace in source):
  SAP(action="edit", target="EDITSOURCE", params={"object_url": "/sap/bc/adt/oo/classes/zcl_test", "old_string": "old code", "new_string": "new code"})

Low-level edit (manual lock/unlock):
  SAP(action="edit", target="LOCK", params={"object_url": "/sap/bc/adt/oo/classes/zcl_test"})
  SAP(action="edit", target="UPDATE_SOURCE", params={"object_url": "/sap/bc/adt/oo/classes/zcl_test", "source": "...", "lock_handle": "..."})
  SAP(action="edit", target="UNLOCK", params={"object_url": "/sap/bc/adt/oo/classes/zcl_test", "lock_handle": "..."})

Activate:
  SAP(action="edit", target="ACTIVATE", params={"object_url": "/sap/bc/adt/oo/classes/zcl_test", "object_name": "ZCL_TEST"})
  SAP(action="edit", target="ACTIVATE_PACKAGE", params={"package": "$TMP"})

Service binding:
  SAP(action="edit", target="PUBLISH_SERVICE", params={"service_name": "ZSB_TEST"})
  SAP(action="edit", target="UNPUBLISH_SERVICE", params={"service_name": "ZSB_TEST"})`)

	case "create":
		return mcp.NewToolResultText(`SAP(action="create") - Create new objects

Create object:
  SAP(action="create", target="OBJECT", params={"object_type": "CLAS/OC", "name": "ZCL_NEW", "description": "New class", "package_name": "$TMP"})
  SAP(action="create", target="DEVC", params={"name": "$ZNEW", "description": "New package"})
  SAP(action="create", target="TABL", params={"name": "ZTABLE", "description": "New table", "fields": "[...]", "package": "$TMP"})
  SAP(action="create", target="CLONE", params={"object_type": "CLAS", "source_name": "ZCL_OLD", "target_name": "ZCL_NEW", "package": "$TMP"})

Class test include:
  SAP(action="create", target="CLAS_TEST_INCLUDE", params={"class_name": "ZCL_TEST", "lock_handle": "..."})

High-level create (with source):
  SAP(action="create", target="PROGRAM", params={"program_name": "ZTEST", "description": "Test", "package_name": "$TMP", "source": "REPORT ztest."})
  SAP(action="create", target="CLASS_WITH_TESTS", params={"class_name": "ZCL_TEST", "description": "Test", "package_name": "$TMP", "class_source": "...", "test_source": "..."})`)

	case "delete":
		return mcp.NewToolResultText(`SAP(action="delete") - Delete objects

  SAP(action="delete", target="OBJECT", params={"object_url": "/sap/bc/adt/oo/classes/zcl_test", "lock_handle": "..."})`)

	case "search":
		return mcp.NewToolResultText(`SAP(action="search") - Search for objects

  SAP(action="search", target="ZCL_*")
  SAP(action="search", target="ZCL_*", params={"maxResults": 50})`)

	case "query":
		return mcp.NewToolResultText(`SAP(action="query") - Database queries

Table contents:
  SAP(action="query", target="TABL_CONTENTS ZTABLE", params={"max_rows": 50})

Free SQL:
  SAP(action="query", target="SQL", params={"sql_query": "SELECT * FROM T000 WHERE MANDT = '001'", "max_rows": 100})`)

	case "test":
		return mcp.NewToolResultText(`SAP(action="test") - Run tests

Unit tests:
  SAP(action="test", target="CLAS ZCL_TEST", params={"object_url": "/sap/bc/adt/oo/classes/zcl_test"})
  SAP(action="test", params={"object_url": "/sap/bc/adt/oo/classes/zcl_test", "include_dangerous": true})

ATC check:
  SAP(action="test", params={"type": "atc", "object_url": "/sap/bc/adt/oo/classes/zcl_test"})`)

	case "grep":
		return mcp.NewToolResultText(`SAP(action="grep") - Search in source code

Grep single object:
  SAP(action="grep", params={"object_url": "/sap/bc/adt/oo/classes/zcl_test", "pattern": "SELECT.*FROM"})

Grep package:
  SAP(action="grep", params={"package_name": "$TMP", "pattern": "WRITE", "max_results": 50})

Grep multiple objects:
  SAP(action="grep", params={"object_urls": ["/sap/bc/adt/oo/classes/zcl_a", "/sap/bc/adt/oo/classes/zcl_b"], "pattern": "TODO"})

Grep multiple packages:
  SAP(action="grep", params={"packages": ["$TMP", "$ZADT"], "pattern": "RAISE", "include_subpackages": true})`)

	case "debug":
		return mcp.NewToolResultText(`SAP(action="debug") - Debugging operations

Breakpoints:
  SAP(action="debug", target="SET_BREAKPOINT", params={"program": "ZCL_TEST", "line": 10})
  SAP(action="debug", target="SET_BREAKPOINT", params={"kind": "statement", "statement": "SELECT"})
  SAP(action="debug", target="SET_BREAKPOINT", params={"kind": "exception", "exception": "CX_SY_ZERODIVIDE"})
  SAP(action="debug", target="GET_BREAKPOINTS")
  SAP(action="debug", target="DELETE_BREAKPOINT", params={"breakpoint_id": "123"})

Session:
  SAP(action="debug", target="LISTEN", params={"timeout": 60})
  SAP(action="debug", target="ATTACH", params={"debuggee_id": "..."})
  SAP(action="debug", target="DETACH")
  SAP(action="debug", target="STEP", params={"step_type": "stepInto"})
  SAP(action="debug", target="GET_STACK")
  SAP(action="debug", target="GET_VARIABLES")

RFC:
  SAP(action="debug", target="CALL_RFC", params={"function": "RFC_READ_TABLE", "params": "{\"QUERY_TABLE\": \"T000\"}"})

Move object:
  SAP(action="debug", target="MOVE", params={"object_type": "CLAS", "object_name": "ZCL_TEST", "new_package": "$TMP"})

Report execution:
  SAP(action="debug", target="RUN_REPORT", params={"report": "RSUSR002"})
  SAP(action="debug", target="GET_VARIANTS", params={"report": "RSUSR002"})
  SAP(action="debug", target="GET_TEXT_ELEMENTS", params={"program": "ZREPORT"})
  SAP(action="debug", target="SET_TEXT_ELEMENTS", params={"program": "ZREPORT", "selection_texts": "{\"P_USER\": \"Username\"}"})

AMDP debugging:
  SAP(action="debug", target="AMDP_START", params={"cascade_mode": "FULL"})
  SAP(action="debug", target="AMDP_RESUME")
  SAP(action="debug", target="AMDP_STEP", params={"step_type": "stepInto"})
  SAP(action="debug", target="AMDP_STOP")
  SAP(action="debug", target="AMDP_GET_VARIABLES")
  SAP(action="debug", target="AMDP_SET_BREAKPOINT", params={"proc_name": "...", "line": 5})
  SAP(action="debug", target="AMDP_GET_BREAKPOINTS")`)

	case "analyze":
		return mcp.NewToolResultText(`SAP(action="analyze") - Code analysis

Syntax check:
  SAP(action="analyze", params={"type": "syntax_check", "object_url": "/sap/bc/adt/oo/classes/zcl_test", "content": "..."})

Call graph:
  SAP(action="analyze", params={"type": "call_graph", "object_uri": "/sap/bc/adt/oo/classes/zcl_test"})
  SAP(action="analyze", params={"type": "callers", "object_uri": "/sap/bc/adt/oo/classes/zcl_test"})
  SAP(action="analyze", params={"type": "callees", "object_uri": "/sap/bc/adt/oo/classes/zcl_test"})
  SAP(action="analyze", params={"type": "analyze_call_graph", "object_uri": "/sap/bc/adt/oo/classes/zcl_test"})
  SAP(action="analyze", params={"type": "compare_call_graphs", "object_uri": "...", "trace_data": "[...]"})
  SAP(action="analyze", params={"type": "trace_execution", "object_uri": "..."})

Object structure:
  SAP(action="analyze", params={"type": "object_structure", "object_name": "ZCL_TEST"})

Code intelligence:
  SAP(action="analyze", params={"type": "definition", "source_url": "...", "source": "...", "line": 10, "start_column": 5, "end_column": 15})
  SAP(action="analyze", params={"type": "references", "object_url": "/sap/bc/adt/oo/classes/zcl_test"})
  SAP(action="analyze", params={"type": "completion", "source_url": "...", "source": "...", "line": 10, "column": 5})
  SAP(action="analyze", params={"type": "class_components", "class_url": "/sap/bc/adt/oo/classes/zcl_test"})
  SAP(action="analyze", params={"type": "type_hierarchy", "source_url": "...", "source": "...", "line": 10, "column": 5})
  SAP(action="analyze", params={"type": "pretty_print", "source": "REPORT ztest. WRITE 'hello'."})
  SAP(action="analyze", params={"type": "get_pretty_printer_settings"})
  SAP(action="analyze", params={"type": "set_pretty_printer_settings", "indentation": true, "style": "upperCase"})
  SAP(action="analyze", params={"type": "inactive_objects"})
  SAP(action="analyze", params={"type": "context", "object_type": "CLAS", "name": "ZCL_TEST"})

Parse ABAP (tokenize + classify statements):
  SAP(action="analyze", params={"type": "parse_abap", "source": "DATA lv_x TYPE i. lv_x = 42."})
  SAP(action="analyze", params={"type": "parse_abap", "object_type": "CLAS", "name": "ZCL_TEST"})

Analyze dependencies (unified 5-layer: regex + parser + SCAN + CROSS + ADT):
  SAP(action="analyze", params={"type": "analyze_deps", "source": "..."})
  SAP(action="analyze", params={"type": "analyze_deps", "object_type": "CLAS", "name": "ZCL_TEST"})

Execute ABAP:
  SAP(action="analyze", params={"type": "execute_abap", "code": "WRITE 'Hello'."})

Runtime errors:
  SAP(action="analyze", params={"type": "list_dumps"})
  SAP(action="analyze", params={"type": "get_dump", "dump_id": "..."})

Profiler traces:
  SAP(action="analyze", params={"type": "list_traces"})
  SAP(action="analyze", params={"type": "get_trace", "trace_id": "..."})

SQL traces:
  SAP(action="analyze", params={"type": "sql_trace_state"})
  SAP(action="analyze", params={"type": "list_sql_traces"})

ABAP help:
  SAP(action="analyze", params={"type": "abap_help", "keyword": "SELECT"})`)

	case "system":
		return mcp.NewToolResultText(`SAP(action="system") - System operations

Info:
  SAP(action="system", target="INFO")
  SAP(action="system", target="COMPONENTS")
  SAP(action="system", target="CONNECTION")
  SAP(action="system", target="FEATURES")

Transports:
  SAP(action="system", params={"type": "list_transports"})
  SAP(action="system", params={"type": "get_transport", "transport": "A4HK900001"})
  SAP(action="system", params={"type": "create_transport", "description": "...", "package": "$TMP"})
  SAP(action="system", params={"type": "release_transport", "transport": "A4HK900001"})
  SAP(action="system", params={"type": "delete_transport", "transport": "A4HK900001"})
  SAP(action="system", params={"type": "get_user_transports", "user_name": "DEVELOPER"})
  SAP(action="system", params={"type": "get_transport_info", "object_url": "...", "dev_class": "$TMP"})

Git/abapGit:
  SAP(action="system", params={"type": "git_types"})
  SAP(action="system", params={"type": "git_export", "packages": "$TMP"})

Install tools:
  SAP(action="system", params={"type": "install_zadt_vsp"})
  SAP(action="system", params={"type": "install_abapgit"})
  SAP(action="system", params={"type": "install_dummy_test"})
  SAP(action="system", params={"type": "list_dependencies"})
  SAP(action="system", params={"type": "deploy_zip", "source": "abapgit-standalone", "package": "$ZGIT"})

File operations:
  SAP(action="system", params={"type": "deploy_from_file", "file_path": "/path/to/file.prog.abap", "package_name": "$TMP"})
  SAP(action="system", params={"type": "save_to_file", "object_type": "CLAS", "object_name": "ZCL_TEST", "output_dir": "/tmp"})
  SAP(action="system", params={"type": "rename", "objType": "CLAS/OC", "oldName": "ZCL_OLD", "newName": "ZCL_NEW", "packageName": "$TMP"})`)

	default:
		return mcp.NewToolResultText(`SAP - Universal ABAP Development Tool

Actions:
  read     - Read source code, table definitions, packages, messages
  edit     - Edit source (high-level with auto lock/unlock, or low-level)
  create   - Create objects, packages, tables, clones
  delete   - Delete objects
  search   - Search for objects by name/pattern
  query    - Query table contents or run SQL
  grep     - Search patterns in source code
  test     - Run unit tests, ATC checks
  analyze  - Syntax check, call graph, code intelligence, profiler, dumps
  debug    - Breakpoints, stepping, variables, RFC calls, report execution
  system   - System info, transports, git, install tools, file operations
  help     - This help. Use SAP(action="help", target="<action>") for details.

Quick examples:
  SAP(action="read", target="CLAS ZCL_TEST")
  SAP(action="edit", target="CLAS ZCL_TEST", params={"source": "CLASS zcl_test..."})
  SAP(action="search", target="ZCL_*")
  SAP(action="test", params={"object_url": "/sap/bc/adt/oo/classes/zcl_test"})
  SAP(action="grep", params={"package_name": "$TMP", "pattern": "SELECT"})
  SAP(action="system", target="INFO")`)
	}
}

// getUnhandledErrorMessage returns a helpful error message when no route matched.
func getUnhandledErrorMessage(action, objectType, objectName string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "No handler found for action=%q", action)
	if objectType != "" {
		fmt.Fprintf(&sb, " target=%q", objectType)
		if objectName != "" {
			fmt.Fprintf(&sb, " %q", objectName)
		}
	}
	sb.WriteString(".\n\n")

	switch action {
	case "read":
		sb.WriteString("Supported read targets: CLAS, PROG, INTF, FUNC, FUGR, INCL, DDLS, BDEF, SRVD, TABL, TABL_CONTENTS, DEVC, MSAG, TRAN, TYPE_INFO, STRUCT, CDS_DEPS\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"read\") for examples.")
	case "edit":
		sb.WriteString("Supported edit targets: CLAS, PROG, INTF, DDLS, BDEF, SRVD, LOCK, UNLOCK, UPDATE_SOURCE, ACTIVATE, ACTIVATE_PACKAGE, EDITSOURCE, PUBLISH_SERVICE, UNPUBLISH_SERVICE\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"edit\") for examples.")
	case "create":
		sb.WriteString("Supported create targets: OBJECT, DEVC, TABL, CLONE, PROGRAM, CLASS_WITH_TESTS, CLAS_TEST_INCLUDE\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"create\") for examples.")
	case "debug":
		sb.WriteString("Supported debug targets: SET_BREAKPOINT, GET_BREAKPOINTS, DELETE_BREAKPOINT, LISTEN, ATTACH, DETACH, STEP, GET_STACK, GET_VARIABLES, CALL_RFC, MOVE, RUN_REPORT, GET_VARIANTS, GET_TEXT_ELEMENTS, SET_TEXT_ELEMENTS, AMDP_*\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"debug\") for examples.")
	default:
		sb.WriteString("Valid actions: read, edit, create, delete, search, query, grep, test, analyze, debug, system, help\n")
		sb.WriteString("Use SAP(action=\"help\") for full documentation.")
	}

	return sb.String()
}
