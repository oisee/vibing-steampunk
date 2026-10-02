// handlers_help_unhandled.go builds the answer for a call no route matched.
package mcp

import (
	"fmt"
	"strings"
)

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
		sb.WriteString("Supported read targets: CLAS, PROG, INTF, FUNC, FUGR, INCL, DDLS, BDEF, SRVD, TABL, TABL_CONTENTS, DEVC, MSAG, TRAN, TYPE_INFO, STRUCT, CDS_DEPS, IDOC\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"read\") for examples.")
	case "edit":
		sb.WriteString("Supported edit targets: CLAS, CLAS_INCLUDE, PROG, INTF, FUNC, DDLS, BDEF, SRVD, TABL, LOCK, UNLOCK, UPDATE_SOURCE, ACTIVATE, ACTIVATE_PACKAGE, EDITSOURCE, PUBLISH_SERVICE, UNPUBLISH_SERVICE\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"edit\") for examples.")
	case "create":
		sb.WriteString("Supported create targets: OBJECT, DEVC, TABL, CLONE, PROGRAM, CLASS_WITH_TESTS, CLAS_TEST_INCLUDE\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"create\") for examples.")
	case "query":
		sb.WriteString("Query needs a statement or a table:\n")
		sb.WriteString("  SAP(action=\"query\", params={\"sql_query\": \"SELECT * FROM T000\", \"max_rows\": 10})\n")
		sb.WriteString("  SAP(action=\"query\", target=\"TABL_CONTENTS T000\", params={\"max_rows\": 50})\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"query\") for examples.")
	case "search":
		sb.WriteString("Search needs a query: SAP(action=\"search\", target=\"ZCL_*\")\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"search\") for examples.")
	case "grep":
		sb.WriteString("Grep needs a pattern and something to search:\n")
		sb.WriteString("  params={\"package_name\": \"$TMP\", \"pattern\": \"SELECT\"}\n")
		sb.WriteString("  target=\"CLAS ZCL_TEST\", params={\"pattern\": \"MODIFY\"}\n")
		sb.WriteString("  params={\"object_url\": \"/sap/bc/adt/oo/classes/zcl_test\", \"pattern\": \"MODIFY\"}\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"grep\") for examples.")
	case "test":
		sb.WriteString("Test needs an object:\n")
		sb.WriteString("  Unit tests: params={\"object_url\": \"/sap/bc/adt/oo/classes/zcl_test\"}\n")
		sb.WriteString("  ATC:        target=\"ATC\", params={\"object_url\": \"/sap/bc/adt/oo/classes/zcl_test\"}\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"test\") for examples.")
	case "analyze":
		sb.WriteString("Common types (params.type): syntax_check, call_graph, callers, callees,\n")
		sb.WriteString("object_structure, check_boundaries, impact, health, cds_impact, loads, effects, parse_abap,\n")
		sb.WriteString("execute_abap, check_abap, list_dumps, list_traces, abap_help\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"analyze\") for examples.")
	case "system":
		sb.WriteString("Supported system targets: INFO, COMPONENTS, CONNECTION, FEATURES\n")
		sb.WriteString("Types (params.type): system_info, components, connection, features,\n")
		sb.WriteString("list_transports, get_transport, create_transport, release_transport, delete_transport,\n")
		sb.WriteString("get_user_transports, get_transport_info, git_types, git_export, install_zadt_vsp,\n")
		sb.WriteString("list_dependencies, deploy_zip,\n")
		sb.WriteString("save_to_file, deploy_from_file, rename\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"system\") for examples.")
	case "delete":
		sb.WriteString("Supported delete targets: <TYPE> <NAME> (" + deleteNameTypesLine + "), OBJECT with params.object_url, UI5_FILE, UI5_APP\n")
		sb.WriteString("Example: SAP(action=\"delete\", target=\"PROG ZDEMO_REPORT\") -- no lock_handle needed\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"delete\") for examples.")
	case "debug":
		sb.WriteString("Supported debug targets: SET_BREAKPOINT, GET_BREAKPOINTS, DELETE_BREAKPOINT, LISTEN, ATTACH, DETACH, STEP, GET_STACK, GET_VARIABLES, CALL_RFC, MOVE, RUN_REPORT, GET_VARIANTS, GET_TEXT_ELEMENTS, SET_TEXT_ELEMENTS, AMDP_ADT_*, AMDP_*\n")
		sb.WriteString("Use SAP(action=\"help\", target=\"debug\") for examples.")
	default:
		sb.WriteString(validActionsLine)
		sb.WriteString("Use SAP(action=\"help\") for full documentation.")
	}

	return sb.String()
}
