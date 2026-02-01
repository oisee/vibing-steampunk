# SAP RFC and Debugging Research

## Problem Statement

Need RFC support from Go without CGO dependency (gorfc requires SAP NW RFC SDK C library).
Need working debugging that can be triggered programmatically from MCP.

## Key Findings

### 1. SAP NW RFC SDK and gorfc

- **gorfc** (`github.com/SAP/gorfc`) requires CGO and SAP NW RFC SDK C library
- RFC protocol is **proprietary, binary, undocumented**
- **No pure Go implementation exists**
- pysap (OWASP) has partial reverse-engineering but incomplete
- Alternatives like Purego can load C libraries without CGO build dependency, but still need SDK at runtime

### 2. WebSocket RFC (ABAP Platform 1909+)

- Uses HTTPS as transport instead of CPIC
- **Still uses the same binary RFC protocol** - just different transport
- Configured via SM59 with destination type "W"
- Parameters: `jco.client.wshost`, `jco.client.wsport`
- **Limitations**: No RFC callbacks, no forward debugging support
- Not helpful for us - same protocol complexity

### 3. Eclipse ADT Communication

ADT uses **dual communication paths**:
1. **RFC Connection** - Primary method via JCo (Java Connector)
2. **HTTP/REST Services** - Alternative via ICF

**Critical insight**: ADT REST APIs are accessible over standard HTTP(S) when ICF service `/sap/bc/adt` is activated in SICF.

### 4. ADT REST API for Debugging

**Endpoints discovered:**

```
POST /sap/bc/adt/debugger?method=attach
  Parameters: debuggeeId, dynproDebugging, debuggingMode, requestUser
  Response: XML with debugger session metadata

POST /sap/bc/adt/debugger?method=setDebuggerSettings
  Body: XML configuration (showDataAging, sharedObjectDebugging, etc.)

GET /sap/bc/adt/debugger/stack?emode=_&semanticURIs=true
  Response: XML call stack with program names, line numbers
```

**Architecture:**
- All requests go through `SADT_REST_RFC_ENDPOINT` function module
- Resource controller: `CL_TPDA_ADT_RES_DEBUGGER`
- Authorization: `S_ADT_RES` object with URI prefix matching
- Protocol: XML payloads, standard HTTP

### 5. ADT Code Execution Endpoint

**Key finding for triggering breakpoints:**

```
POST /sap/bc/adt/oo/classrun/{ClassName}
```

- Executes classes implementing `if_oo_adt_classrun` interface
- Runs in **ICF dialog work process** (not BTC)
- Should check external breakpoints (unlike background jobs)
- Handler class: `CL_OO_ADT_RES_CLASSRUN`

**Interface:**
```abap
INTERFACES: if_oo_adt_classrun

METHOD if_oo_adt_classrun~main.
  out->write( 'Output to ADT console' ).
ENDMETHOD.
```

### 6. External Breakpoints

- Stored in table `ABDBG_EXTDBPS`
- User-scoped (USERNAME field)
- Set via TPDAPI: `if_tpdapi_static_bp_services->create_line_breakpoint_by_row()`
- Context set via: `set_external_bp_context_user( i_ide_user, i_request_user )`

**Work process behavior:**
- **DIA (Dialog)**: Checks external breakpoints ✓
- **BTC (Background)**: Does NOT check external breakpoints ✗
- **ICF (HTTP)**: Should check external breakpoints (dialog context) ✓

### 7. Why Background Jobs Don't Work for Debugging

Our `RunReport` implementation uses XBP BAPIs to schedule background jobs:
- `BAPI_XBP_JOB_OPEN`
- `BAPI_XBP_JOB_ADD_ABAP_STEP`
- `BAPI_XBP_JOB_CLOSE`
- `BAPI_XBP_JOB_START_IMMEDIATELY`

These run in BTC work processes which don't check external breakpoints by design.

### 8. APC/WebSocket Context Restrictions

Inside APC (ABAP Push Channel) handlers, certain statements cause dumps:
- `SUBMIT` - forbidden
- `CALL FUNCTION ... DESTINATION` - forbidden
- `LEAVE` - forbidden
- Various GUI-related statements

This is why our WebSocket-based approach can't directly execute code.

## Solution Path

### Finding: Two Breakpoint Tables with Different Behavior

SAP has **two separate breakpoint mechanisms**:

| Table | API | Checked by |
|-------|-----|------------|
| ABDBG_EXTDBPS | TPDAPI (external breakpoints) | Eclipse ADT via RFC, SAP GUI with external debugging |
| ABDBG_BPS | CL_ABAP_DEBUGGER | HTTP execution (classrun, REST, unit tests) |

**TPDAPI breakpoints (ABDBG_EXTDBPS)** do NOT trigger for HTTP-based execution like classrun. The REST request creates a new ICF session that doesn't inherit the breakpoint context.

### Finding: SE24 F8 DOES Trigger Breakpoints

Executing a class via SE24 (F8) in SAP GUI **does** trigger external breakpoints because:
- It runs in the **same dialog session** where breakpoints were set
- The TPDAPI breakpoint context is preserved within the session
- Uses SAP test framework (SAPLSETV, SAPLSEUT)

### Solution: HTTP Breakpoints via CL_ABAP_DEBUGGER

**HTTP breakpoints (ABDBG_BPS)** DO work for MCP-triggered execution:

```abap
" Set HTTP breakpoint - writes to ABDBG_BPS
cl_abap_debugger=>save_http_breakpoints(
  internal_bps = lt_breakpoints
).
```

For classes, the include name must be resolved using `cl_oo_include_naming`:
```abap
DATA(lo_naming) = cl_oo_include_naming=>get_instance_by_name( lv_class ).
DATA(lv_include) = lo_naming->get_include_by_mtdname( lv_method ).
```

When HTTP execution hits a breakpoint in ABDBG_BPS, SAP GUI debugger opens automatically.

### Current Recommended Workflow

**HTTP Breakpoints (fully automated via MCP):**
1. Set HTTP breakpoint: `debug set_http_breakpoint params.program="ZCL_TEST" params.method="MY_METHOD" params.line=5`
2. Verify: `debug get_http_breakpoints`
3. Trigger: `debug classrun ZCL_TEST`
4. SAP GUI debugger opens at the breakpoint

Note: Line number is relative to method start (line 1 = first line of method).

**TPDAPI Breakpoints (hybrid - manual trigger required):**
1. Set breakpoints via MCP: `debug set_breakpoint params.program="ZCL_TEST========CP" params.line=10`
2. Trigger execution manually: SE24 → F8, or SE38, or transaction
3. Listen for debuggee: `debug listen`
4. Attach and debug: `debug attach params.debuggee_id="..."`

### Technical Deep Dive: Why TPDAPI Breakpoints Don't Trigger for HTTP

This section explains why TPDAPI breakpoints (ABDBG_EXTDBPS) don't work for HTTP execution. The solution is to use HTTP breakpoints (ABDBG_BPS) instead - see above.

**What we verified:**
1. TPDAPI breakpoints persist correctly to `ABDBG_EXTDBPS` with `FLAG_ACTIVE=X`
2. Debug activation registers in `ICFATTRIB` with correct user/server
3. Listener registration works (can catch debuggees from other sessions)
4. But HTTP-triggered code **never checks** TPDAPI external breakpoints

**Root cause:** TPDAPI breakpoints are designed for RFC-based debugging (Eclipse ADT). HTTP execution uses a different code path that checks ABDBG_BPS instead.

**Eclipse (RFC) vs MCP (HTTP):**
| Aspect | Eclipse (RFC) | MCP (HTTP) |
|--------|---------------|------------|
| Protocol | RFC port 3300 | HTTP port 50000 |
| Breakpoint table | ABDBG_EXTDBPS | ABDBG_BPS |
| API | TPDAPI | CL_ABAP_DEBUGGER |
| Debugger | REST session (listen/attach) | SAP GUI (auto-opens) |

### Alternative: Dynamic Code in classrun

If SUBMIT is restricted, the runner class could:
- Use `GENERATE SUBROUTINE POOL` for dynamic code
- Call methods on other classes directly
- Execute function modules

## References

- SAP Help: Configuring ADT Backend
- GitHub: marcellourbani/abap-adt-api - ADT API client library
- GitHub: vscode_abap_remote_fs issue #156 - REST endpoint examples
- SAP Community: "How does ADT work" architecture explanation
- OWASP pysap: Partial RFC protocol reverse-engineering

## Files in This Project

- `pkg/adt/websocket_debug.go` - WebSocket debug client (TPDAPI operations)
- `pkg/adt/websocket_rfc.go` - WebSocket RFC calls and RunReport
- `internal/mcp/handlers_debugger.go` - MCP debug handlers (WebSocket-based)
- `internal/mcp/handlers_debugger_legacy.go` - MCP debug handlers (REST-based)
- `embedded/abap/zcl_vsp_debug_service.clas.abap` - ABAP debug service via TPDAPI
- `embedded/abap/zcl_vsp_report_service.clas.abap` - ABAP report execution via XBP BAPIs

## Date

2026-01-13
