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
  SAP(action="edit", target="FUNC ZMY_FM", params={"source": "FUNCTION zmy_fm...ENDFUNCTION."})
      the function group is resolved from the module name; pass params={"parent": "ZMY_FG"} to name it
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
  SAP(action="create", target="OBJECT", params={"object_type": "FUGR/F", "name": "ZVSP_DEMO", "description": "Demo group", "package_name": "$TMP"})
  SAP(action="create", target="OBJECT", params={"object_type": "FUGR/FF", "name": "ZVSP_DEMO_FM", "parent_name": "ZVSP_DEMO", "description": "RFC demo", "package_name": "$TMP", "rfc_enabled": true, "source": "FUNCTION zvsp_demo_fm\n  IMPORTING VALUE(iv_n) TYPE i\n  EXPORTING VALUE(ev_result) TYPE i.\n  ev_result = iv_n * 2.\nENDFUNCTION."})
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

	case "info":
		return mcp.NewToolResultText(`SAP(action="info") - What am I talking to?

  SAP()
  SAP(action="info")

Both answer the same four things, in the order somebody needs them:

  which build is answering — an agent reporting a defect against "vsp"
    names nothing; against a commit it names something
  whether the session is alive and authenticated, which decides whether
    any other call is worth making
  which system, so nobody acts on production believing it is a sandbox
  what to call next

The connection check is a CSRF token fetch, not a status code: an expired
SSO session answers 200 with a logon page, and only the missing token
gives it away.

The instance number is derived from the port (80NN, 443NN, 5NN00), not
read from the system, and the card says so.`)

	case "rfc":
		return mcp.NewToolResultText(`SAP(action="rfc") - Classic RFC to the same system

Not ADT. This speaks SAP's classic Type-3 protocol to the gateway, in pure
Go. A system reachable over HTTPS is not necessarily reachable here: the
gateway is a different port and is often closed.

  SAP(action="rfc", params={"op": "info"})
  SAP(action="rfc", params={"op": "ping"})
  SAP(action="rfc", target="STFC_CONNECTION")
  SAP(action="rfc", target="Z_DOUBLE", params={"op": "call", "args": {"N": 21}})
  SAP(action="rfc", params={"op": "search", "pattern": "BAPI_USER*"})
  SAP(action="rfc", params={"op": "read_table", "table": "T000"})

Only remote-enabled function modules can be called. A module that is not
marked remote is unreachable by every transport — a property of the module,
not of the connection.`)

	case "i18n":
		return mcp.NewToolResultText(`SAP(action="i18n") - Translation texts and language comparison

Object texts in one language:
  SAP(action="i18n", params={"op": "texts", "object_url": "/sap/bc/adt/oo/classes/zcl_demo", "language": "DE"})

Data element labels — short, medium, long, heading:
  SAP(action="i18n", params={"op": "data_element_labels", "name": "ZDE_ORDER_ID", "language": "DE"})

Message class texts:
  SAP(action="i18n", params={"op": "message_class_texts", "name": "ZVSP_GIT", "language": "EN"})

A report's selection texts and text symbols — program_name here, not name:
  SAP(action="i18n", params={"op": "text_pool", "program_name": "ZDEMO_REPORT", "language": "EN"})

What differs between two languages — named separately, not as a list:
  SAP(action="i18n", params={"op": "compare_languages", "object_url": "/sap/bc/adt/oo/classes/zcl_demo", "source_language": "EN", "target_language": "DE"})

Writing needs a lock_handle from a lock taken first, and changes the system:
  SAP(action="i18n", params={"op": "write_message_texts", "name": "ZVSP_GIT", "language": "DE", "lock_handle": "...", "texts": []})

write_labels is not implemented and refuses. What it used to send was a
four-field document to a resource that takes the data element's whole
representation, so it never worked; it now says so instead of failing
opaquely. Use SE11.`)

	case "revisions", "history":
		return mcp.NewToolResultText(`SAP(action="revisions") - Version history

Name the object by type and name, or by target:
  SAP(action="revisions", params={"type": "CLAS", "name": "ZCL_DEMO"})
  SAP(action="revisions", target="CLAS ZCL_DEMO")

Read one version's source. The URI comes from the list above and is not
constructable by hand:
  SAP(action="revisions", params={"op": "source", "version_uri": "..."})

Compare two versions; version2_uri defaults to the current one:
  SAP(action="revisions", params={"op": "compare", "type": "CLAS", "name": "ZCL_DEMO", "version1_uri": "..."})

For a function module, name its group as parent.`)

	case "lint":
		return mcp.NewToolResultText(`SAP(action="lint") - Static analysis, offline

Supply the source, or name an object to read it from. One or the other —
neither is optional on its own:
  SAP(action="lint", params={"source": "REPORT zdemo.\nWRITE 'x'.\n"})
  SAP(action="lint", params={"object_type": "CLAS", "object_name": "ZCL_DEMO"})

Also reachable as analyze type=lint, because that is where somebody looks
for static analysis first.

Thirteen rules, eight on by default: empty catch blocks, over-broad
exception catches, hardcoded credentials, magic numbers, unreachable code.
No ABAP is executed, and no server is involved when source is supplied.`)

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

AMDP debugging over ADT (nothing installed on the server; breakpoints fire):
  SAP(action="debug", target="AMDP_ADT_START")
  SAP(action="debug", target="AMDP_ADT_BREAKPOINT", params={"class": "ZCL_X", "line": 41})
  SAP(action="debug", target="AMDP_ADT_AWAIT")   # run the AMDP method from elsewhere first
  SAP(action="debug", target="AMDP_ADT_STOP")
  Answers arrive as a queue with acknowledgements at its head, so AWAIT keeps
  asking past them. It also reports whether SAP calls the breakpoint VALID —
  a refused breakpoint and a method that never ran look identical otherwise.

AMDP debugging over the ZADT_VSP WebSocket (older route; its breakpoints have
never been observed to fire):
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

Call graph (one hop; object_type + object_name work instead of object_uri):
  SAP(action="analyze", params={"type": "call_graph", "object_uri": "/sap/bc/adt/oo/classes/zcl_test"})
  SAP(action="analyze", params={"type": "callers", "object_type": "CLAS", "object_name": "ZCL_TEST"})
    who references it — the where-used list behind SE84
  SAP(action="analyze", params={"type": "callees", "object_type": "CLAS", "object_name": "ZCL_TEST"})
    what it references — the CROSS and WBCROSSGT tables, filled at activation,
    so these are recorded references and not observed calls; needs free SQL
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

Graph queries:
  SAP(action="analyze", params={"type": "co_change", "object_type": "CLAS", "object_name": "ZCL_FOO", "top_n": 10})
  SAP(action="analyze", params={"type": "impact", "object_type": "CLAS", "object_name": "ZCL_FOO", "max_depth": 3})
  SAP(action="analyze", params={"type": "impact", "object_type": "CLAS", "object_name": "ZCL_FOO", "include_source_analysis": true})
  SAP(action="analyze", params={"type": "where_used_config", "variable": "ZKEKEKE"})
  SAP(action="analyze", params={"type": "where_used_config", "variable": "ZKEKEKE", "grep": false})
  SAP(action="analyze", params={"type": "usage_examples", "object_type": "FUNC", "object_name": "Z_MY_FM"})
  SAP(action="analyze", params={"type": "usage_examples", "object_type": "CLAS", "object_name": "ZCL_API", "method": "GET_DATA"})
  SAP(action="analyze", params={"type": "usage_examples", "object_type": "PROG", "object_name": "ZLEGACY", "form": "BUILD_OUTPUT"})

Transport analysis:
  SAP(action="analyze", params={"type": "cr_history", "object_type": "CLAS", "object_name": "ZCL_FOO"})
  SAP(action="analyze", params={"type": "tr_boundaries", "transports": "A4HK900001,A4HK900002"})
  SAP(action="analyze", params={"type": "cr_boundaries", "cr_id": "JIRA-123"})
  SAP(action="analyze", params={"type": "usage_examples", "object_type": "PROG", "object_name": "ZBATCH_RUN", "submit": true})
  SAP(action="analyze", params={"type": "health", "package": "$ZDEV"})
  SAP(action="analyze", params={"type": "health", "object_type": "CLAS", "object_name": "ZCL_ORDER_SERVICE"})

Execute ABAP:
  SAP(action="analyze", params={"type": "execute_abap", "code": "WRITE 'Hello'."})

Runtime errors (ST22) — a listing, and a post-mortem around one dump:
  SAP(action="analyze", params={"type": "list_dumps", "since": "2026-08-01", "program": "ZDEMO_POST"})
  SAP(action="analyze", params={"type": "group_dumps", "since": "2026-08-01"})
      what keeps failing, not what failed once: count, first seen, last seen, users
  SAP(action="analyze", params={"type": "get_dump", "dump_id": "latest"})
      header, termination point and call stack of one dump
  SAP(action="analyze", params={"type": "explain_dump", "dump_id": "latest", "tolerance": "5m"})
      the stack, plus application log entries ranked by the argument for each — a log written by
      the program that died is structural; one merely nearby in time is a coincidence
  SAP(action="analyze", params={"type": "similar_dumps", "dump_id": "latest", "deep": 10})
      is this new, and how often: rung 1 same line, 2 same program, 3 same component, 4 same error
  SAP(action="analyze", params={"type": "dump_impact", "dump_id": "latest"})
      who else reaches the code that failed — blast radius, not blame
  dump_id takes "latest", any part of an id from list_dumps, or a whole id.
  filters shared by all six: program, error_type, user, since, until (YYYY-MM-DD), max_results

Application log (SLG1, read with free SQL — no RFC, no gateway, no Z code):
  SAP(action="analyze", params={"type": "application_log", "program": "ZDEMO_POST", "max_results": 20})
  SAP(action="analyze", params={"type": "application_log", "user": "TESTUSER", "since": "2026-08-01"})
  SAP(action="analyze", params={"type": "application_log", "object": "ZDEMO_LOG", "subobject": "POST", "messages": true})
      headers by default; messages=true decodes the BALDAT cluster too: class, number, variables,
      text from T100, context, detail level

Cluster tables (BALDAT, INDX, STXL, ... — EXPORT data clusters decoded here, no IMPORT needed):
  SAP(action="analyze", params={"type": "cluster_read", "table": "INDX", "where": "relid = 'ZV'"})
  SAP(action="analyze", params={"type": "cluster_read", "table": "INDX", "where": "relid = 'ZD'", "layout": "ZDEMO_S_HEADER"})
  SAP(action="analyze", params={"type": "cluster_read", "table": "INDX", "where": "...", "layout": "HDR=ZDEMO_S_HEADER,ITEMS=ZDEMO_S_ITEM"})
  SAP(action="analyze", params={"type": "cluster_read", "table": "STXL", "where": "tdname = 'Z...'"})
  SAP(action="analyze", params={"type": "cluster_read", "table": "BALDAT", "where": "relid = 'AL' AND log_handle = '...'", "layout": "applog"})
      every exported object with its fields typed and decoded. The cluster holds no field names:
      without a layout they are numbered; a DDIC structure (read from DD03L, includes resolved) names
      them, per object as OBJECT=STRUCTURE, and is refused when it does not fit rather than guessed.
      stxl (the default on STXL) renders SAPscript text; applog prints BALDAT messages.
      Version 5 clusters (pre-Unicode rows, e.g. EUFUNC test data, AQLDB) are read as well.
      max_results caps database rows (fragments), 200 by default.

Profiler traces:
  SAP(action="analyze", params={"type": "list_traces"})
  SAP(action="analyze", params={"type": "get_trace", "trace_id": "..."})

SQL traces:
  SAP(action="analyze", params={"type": "sql_trace_state"})
  SAP(action="analyze", params={"type": "list_sql_traces"})

ABAP help:
  SAP(action="analyze", params={"type": "abap_help", "keyword": "SELECT"})

Dependency graph & boundary analysis:
  SAP(action="analyze", params={"type": "check_boundaries", "package": "$ZDEV", "whitelist": "$ZCOMMON,$ZUTIL*"})
  SAP(action="analyze", params={"type": "check_boundaries", "object": "ZCL_TEST", "whitelist": "$ZCOMMON"})
  SAP(action="analyze", params={"type": "check_boundaries", "source": "REPORT z.\nCALL FUNCTION 'Z_FM'.", "package": "$Z"})
  SAP(action="analyze", params={"type": "graph_stats", "source": "REPORT z.\nDATA lo TYPE REF TO zcl_x."})`)

	case "system":
		return mcp.NewToolResultText(`SAP(action="system") - System operations

Info:
  SAP(action="system", target="INFO")
  SAP(action="revisions", target="CLAS ZCL_TEST")
  SAP(action="lint", params={"object_type": "CLAS", "object_name": "ZCL_TEST"})
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

	case "tips", "best_practices", "workflows", "best":
		return mcp.NewToolResultText(`SAP Best Practices & Workflows

=== EDITING WORKFLOW (recommended) ===
1. Read object with context:    SAP(action="read", target="CLAS ZCL_TEST")
2. Edit (auto lock/activate):   SAP(action="edit", target="CLAS ZCL_TEST", params={"source": "..."})
   → Handles lock → save → unlock → activate automatically

=== FILE-BASED WORKFLOW (for complex edits) ===
1. Export to local file:         SAP(action="system", params={"type": "save_to_file", "object_type": "CLAS", "object_name": "ZCL_TEST", "output_dir": "/tmp"})
2. Edit locally (your editor)
3. Deploy back:                  SAP(action="system", params={"type": "deploy_from_file", "file_path": "/tmp/zcl_test.clas.abap", "package_name": "$TMP"})
   → Best for large classes — edit locally, deploy per-file

=== PACKAGE ANALYSIS ===
1. Check boundaries:             SAP(action="analyze", params={"type": "check_boundaries", "package": "$ZDEV", "whitelist": "$ZCOMMON"})
2. Offline (no SAP):             SAP(action="analyze", params={"type": "check_boundaries", "source": "...", "package": "$ZDEV"})

=== TESTING ===
1. Run unit tests:               SAP(action="test", params={"object_url": "/sap/bc/adt/oo/classes/zcl_test"})
2. ATC check:                    SAP(action="test", target="ATC", params={"object_uri": "/sap/bc/adt/oo/classes/zcl_test"})
3. Syntax check:                 SAP(action="analyze", params={"type": "syntax_check", "object_url": "/sap/bc/adt/oo/classes/zcl_test"})

=== DEPENDENCY ANALYSIS ===
1. What this uses (down):        SAP(action="analyze", params={"type": "callees", "object_uri": "..."})
2. Where-used (up):              SAP(action="analyze", params={"type": "callers", "object_uri": "..."})
3. CDS dependencies:             SAP(action="read", target="CDS_DEPS ZDDL_VIEW")
4. CDS impact (consumers):      SAP(action="analyze", params={"type": "cds_impact", "cds_view": "ZDDL_VIEW"})
5. Boundary check:               SAP(action="analyze", params={"type": "check_boundaries", "package": "$ZDEV"})

=== SEARCH & GREP ===
1. Find objects:                 SAP(action="search", target="ZCL_*")
2. Grep in package:              SAP(action="grep", params={"package_name": "$TMP", "pattern": "SELECT"})
3. Grep specific object:         SAP(action="grep", params={"object_name": "ZCL_TEST", "pattern": "MODIFY"})

=== DEBUGGING ===
1. Set breakpoint:               SAP(action="debug", target="SET_BREAKPOINT", params={"program": "ZCL_TEST", "line": 10})
2. Run report:                   SAP(action="debug", target="RUN_REPORT", params={"report": "ZREPORT"})
3. Call RFC:                     SAP(action="debug", target="CALL_RFC", params={"function": "Z_MY_FM", "params": "{...}"})

=== TIPS ===
• Use "read" before "edit" — it gives context (deps, structure)
• Use deploy_from_file for classes with many methods — edit locally, deploy per-file
• Use save_to_file + export to get objects locally for offline analysis/editing
• Use check_boundaries with whitelist to enforce package architecture
• Always test after edit: syntax check → unit tests → ATC
• For large refactors: export → edit locally → deploy → test`)

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
  analyze  - Syntax check, call graph, code intelligence, profiler, dumps and the log around
             them, application log, boundary analysis
  debug    - Breakpoints, stepping, variables, RFC calls, report execution
  system   - System info, transports, git, install tools, file operations
  rfc      - Classic RFC to the same system, no gateway library
  i18n     - Translation texts, language comparison
  revisions- Version history, one version's source, two versions compared
  lint     - Static analysis of ABAP source, offline
  info     - Build, connection, system, and what to call next. Also SAP() with
             no arguments at all.
  help     - This help. Use SAP(action="help", target="<action>") for details.

Quick examples:
  SAP(action="read", target="CLAS ZCL_TEST")
  SAP(action="edit", target="CLAS ZCL_TEST", params={"source": "CLASS zcl_test..."})
  SAP(action="search", target="ZCL_*")
  SAP(action="test", params={"object_url": "/sap/bc/adt/oo/classes/zcl_test"})
  SAP(action="grep", params={"package_name": "$TMP", "pattern": "SELECT"})
  SAP(action="analyze", params={"type": "check_boundaries", "package": "$ZDEV", "whitelist": "$ZCOMMON"})
  SAP(action="system", target="INFO")

Use SAP(action="help", target="tips") for best practices and workflow guides.`)
	}
}

// getUnhandledErrorMessage returns a helpful error message when no route matched.
// validActionsLine is the one place the action list is written down. It had
// drifted from the dispatcher in both directions at once: "rfc" was routed but
// absent from the list, so a caller was told the feature did not exist, while
// "system" and "analyze" were listed without their target or type and so looked
// broken when they were merely under-specified.
const validActionsLine = "Valid actions: read, edit, create, delete, search, query, grep, test, analyze, debug, system, rfc, i18n, revisions, lint, info, help\n"

// actionsNeedingTarget are the actions the dispatcher can only route once it
// knows what they are aimed at.
var actionsNeedingTarget = map[string]bool{
	"read":   true,
	"edit":   true,
	"create": true,
	"delete": true,
	"system": true,
}

func actionNeedsTarget(action string) bool { return actionsNeedingTarget[action] }

func getUnhandledErrorMessage(action, objectType, objectName string) string {
	var sb strings.Builder
	// Say which of the three the caller is missing. "No handler found" reads as
	// "this action does not exist" and sends people looking for a feature that
	// is present — several actions simply need a target, or a type in params,
	// and the old message listed them as valid without ever saying so.
	switch {
	case objectType == "" && actionNeedsTarget(action):
		fmt.Fprintf(&sb, "action=%q needs a target.", action)
	case action == "analyze":
		fmt.Fprintf(&sb, "action=%q needs params={\"type\": ...}.", action)
	default:
		fmt.Fprintf(&sb, "No handler found for action=%q", action)
		if objectType != "" {
			fmt.Fprintf(&sb, " target=%q", objectType)
			if objectName != "" {
				fmt.Fprintf(&sb, " %q", objectName)
			}
		}
		sb.WriteString(".")
	}
	sb.WriteString("\n\n")

	switch action {
	case "read":
		sb.WriteString("Supported read targets: CLAS, PROG, INTF, FUNC, FUGR, INCL, DDLS, BDEF, SRVD, TABL, TABL_CONTENTS, DEVC, MSAG, TRAN, TYPE_INFO, STRUCT, CDS_DEPS\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"read\") for examples.")
	case "edit":
		sb.WriteString("Supported edit targets: CLAS, PROG, INTF, FUNC, DDLS, BDEF, SRVD, TABL, LOCK, UNLOCK, UPDATE_SOURCE, ACTIVATE, ACTIVATE_PACKAGE, EDITSOURCE, PUBLISH_SERVICE, UNPUBLISH_SERVICE\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"edit\") for examples.")
	case "create":
		sb.WriteString("Supported create targets: OBJECT, DEVC, TABL, CLONE, PROGRAM, CLASS_WITH_TESTS, CLAS_TEST_INCLUDE\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"create\") for examples.")
	case "debug":
		sb.WriteString("Supported debug targets: SET_BREAKPOINT, GET_BREAKPOINTS, DELETE_BREAKPOINT, LISTEN, ATTACH, DETACH, STEP, GET_STACK, GET_VARIABLES, CALL_RFC, MOVE, RUN_REPORT, GET_VARIANTS, GET_TEXT_ELEMENTS, SET_TEXT_ELEMENTS, AMDP_ADT_*, AMDP_*\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"debug\") for examples.")
	case "system":
		sb.WriteString("Supported system targets: INFO, COMPONENTS, CONNECTION, FEATURES\n")
		sb.WriteString("Example: SAP(action=\"system\", target=\"INFO\")")
	case "analyze":
		sb.WriteString("Supported analysis types (params.type): call_graph, object_structure, callers, callees,\n")
		sb.WriteString("analyze_call_graph, compare_call_graphs, trace_execution, check_boundaries, graph_stats,\n")
		sb.WriteString("co_change, impact, where_used_config, usage_examples, health, cr_history, tr_boundaries,\n")
		sb.WriteString("cr_boundaries\n")
		sb.WriteString("Example: SAP(action=\"analyze\", params={\"type\": \"check_boundaries\", \"package\": \"$ZDEV\"})")
	default:
		sb.WriteString(validActionsLine)
		sb.WriteString("Use SAP(action=\"help\") for full documentation.")
	}

	return sb.String()
}
