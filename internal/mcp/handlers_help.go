// ABOUTME: Provides help documentation for the SAP MCP tool.
// ABOUTME: Returns structured documentation based on action type.
package mcp

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// handleHelp returns documentation for the SAP tool
func handleHelp(target string) *mcp.CallToolResult {
	target = strings.ToLower(strings.TrimSpace(target))

	switch target {
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
		return mcp.NewToolResultText(`EDIT - Modify source code

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
		return mcp.NewToolResultText(`TEST - Run unit tests

  test                 params.object_url="/sap/bc/adt/oo/classes/<name>"
                       [params.with_coverage=true]`)

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

	case "debug":
		return mcp.NewToolResultText(`DEBUG - Debugging operations

  debug ATTACH         params.terminal_id
  debug DETACH
  debug STEP           params.type=into|over|out|continue
  debug VARIABLES      [params.stack_level]
  debug BREAKPOINT     params.action=set|delete, params.uri, params.line`)

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
		case "CLAS", "INTF", "PROG", "DDLS", "BDEF", "SRVD", "SRVB":
			fmt.Fprintf(&sb, "Did you forget params.source? Try:\n  edit %s %s params.source=\"...\"", objectType, objectName)
		case "":
			sb.WriteString("Missing target. Examples:\n  edit CLAS ZCL_TEST params.source=\"...\"\n  edit LOCK params.object_url=\"...\"")
		default:
			fmt.Fprintf(&sb, "Unknown edit target '%s'. Valid targets:\n", objectType)
			sb.WriteString("  High-level: CLAS, INTF, PROG, DDLS, BDEF, SRVD, SRVB (with params.source)\n")
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
