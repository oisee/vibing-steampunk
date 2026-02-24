# NW 7.40 ZADT_VSP Compatibility Analysis

**Date:** 2026-02-24
**Report ID:** 001
**Subject:** Architectural limitations of ZADT_VSP on NetWeaver 7.40 and solution paths
**Related Documents:** `reports/2026-01-06-002-amc-async-architecture-analysis.md`

---

## Executive Summary

ZADT_VSP's APC handler (`zcl_vsp_apc_handler`) inherits from `cl_apc_wsp_ext_stateful_base`, which requires kernel-level WebSocket support not available on NW 7.40. This report analyzes which tools are affected (20 of 122), confirms that downporting the APC infrastructure is not viable, and recommends an ICF HTTP handler as the backward-compatible transport.

---

## 1. The Core Problem

`zcl_vsp_apc_handler` (line 6) inherits from `cl_apc_wsp_ext_stateful_base`. On NW 7.40:

- **APC (ABAP Push Channels)** were introduced in 7.40 SP02 but with limited support. Stateful APC handlers may not be available or may be severely restricted depending on the exact SP level and kernel patch.
- Even when APC is technically present, `cl_apc_wsp_ext_stateful_base` requires the ABAP work process to maintain state across messages, which on 7.40 can conflict with work process management.
- The **AMC (ABAP Messaging Channels)** infrastructure that stateful APC relies on internally was also immature on 7.40.

---

## 2. Why Downporting APC Classes Is Not Viable

The APC handler classes are not self-contained ABAP code. They are a thin ABAP layer on top of deep kernel infrastructure:

```
CL_APC_WSP_EXT_STATEFUL_BASE
  +-- IF_APC_WSP_EXTENSION (interface)
  |     +-- IF_APC_WSP_SERVER_CONTEXT
  |     +-- IF_APC_WSP_MESSAGE_MANAGER
  |           +-- IF_APC_WSP_MESSAGE
  +-- CL_APC_WSP_EXT_STATEFUL_BASE (abstract superclass)
  |     +-- AMC internal messaging layer
  |           +-- Shared memory segments for AMC
  |           +-- CL_AMC_CHANNEL_MANAGER
  |           +-- AMC tables (SAPC configuration)
  +-- ICM (Internet Communication Manager) - KERNEL level
  |     +-- WebSocket frame parser (C code in kernel)
  |     +-- WebSocket upgrade handler (HTTP->WS protocol switch)
  |     +-- APC-specific ICF service node handling
  +-- SAPC transaction + configuration tables
        +-- APC application registry
        +-- AMC channel definitions
```

**The critical blocker is the ICM kernel.** On 7.40, the Internet Communication Manager does not have the WebSocket protocol handler compiled into it. The HTTP-to-WebSocket upgrade (`101 Switching Protocols`) is handled at the C kernel level, not in ABAP. You cannot backport kernel-level C code by deploying ABAP classes.

Even if the ABAP classes were made to compile (by stubbing all missing interfaces), the first WebSocket connection attempt would fail because the ICM would not know what to do with the `Upgrade: websocket` header.

**Verdict:** APC/WebSocket support is a kernel+basis capability, not just ABAP classes. It requires NW 7.50 (or 7.40 SP08+ with specific kernel patches that are not universally available).

---

## 3. Current Architecture: What Uses ZADT_VSP

ZADT_VSP hosts 5 service domains, routed by `zcl_vsp_apc_handler`:

```
vsp (Go) --WebSocket--> ZADT_VSP (APC Handler)
                              |
                              +-- rfc domain      -> ZCL_VSP_RFC_SERVICE
                              +-- debug domain    -> ZCL_VSP_DEBUG_SERVICE
                              +-- amdp domain     -> ZCL_VSP_AMDP_SERVICE
                              +-- git domain      -> ZCL_VSP_GIT_SERVICE
                              +-- report domain   -> ZCL_VSP_REPORT_SERVICE
                              +-- system domain   -> APC handler itself (ping, abap_help)
```

### Statefulness Analysis by Domain

| Domain | Service Class | Stateful? | NW 7.40 Impact |
|--------|--------------|-----------|-----------------|
| **system** | APC handler itself | Minimal | `ping`, `get_abap_help` - request/response only |
| **rfc** | `zcl_vsp_rfc_service` | **No** | `call`, `search`, `getMetadata`, `runReport` - all stateless |
| **report** | `zcl_vsp_report_service` | **No** | `runReport`, `getTextElements`, `getVariants` - all stateless |
| **git** | `zcl_vsp_git_service` | **No** | `export`, `getTypes` - stateless |
| **debug** | `zcl_vsp_debug_service` | **YES** | TPDAPI refs (`mo_dbg_session`, `mt_breakpoints`) persist across messages |
| **amdp** | `zcl_vsp_amdp_service` | **YES** | Session state in `gt_sessions` (irrelevant on 7.40 - no HANA) |

**Key insight:** 4 of 5 service domains (rfc, report, git, system) are effectively stateless. Only **debug** truly needs statefulness. The **amdp** domain is irrelevant on 7.40.

---

## 4. Complete Tool Impact Matrix

### 4.1 Tools Requiring ZADT_VSP via `amdpWSClient` (General-Purpose WS Client)

| Tool | Handler | WS Domain | Stateful? | Severity | Workaround |
|------|---------|-----------|-----------|----------|------------|
| **RunReport** | `handlers_report.go:47` | report | No | **High** | No ADT REST equivalent |
| **RunReportAsync** | `handlers_report.go:152` | report | No | **High** | No ADT REST equivalent |
| **GetVariants** | `handlers_report.go:314` | report | No | **High** | No ADT REST equivalent |
| **GetTextElements** | `handlers_report.go:349` | report | No | Medium | No ADT REST equivalent |
| **SetTextElements** | `handlers_report.go:425` | report | No | Medium | No ADT REST equivalent |
| **GitTypes** | `handlers_git.go:26` | git | No | Medium | abapGit REST API (if installed) |
| **GitExport** | `handlers_git.go:82` | git | No | Medium | abapGit REST API (if installed) |
| **GetAbapHelp** | `handlers_system.go:93` | system | No | Low | Graceful fallback already exists |

### 4.2 Tools Requiring ZADT_VSP via `debugWSClient`

| Tool | Handler | WS Domain | Stateful? | Severity | Workaround |
|------|---------|-----------|-----------|----------|------------|
| **SetBreakpoint** | `handlers_debugger.go:74-127` | debug | **YES** | Medium | ADT REST debug APIs (CSRF issues) |
| **GetBreakpoints** | `handlers_debugger.go:187` | debug | **YES** | Medium | ADT REST debug APIs |
| **DeleteBreakpoint** | `handlers_debugger.go:225` | debug | **YES** | Medium | ADT REST debug APIs |
| **DebuggerListen** | `handlers_debugger.go` | debug | **YES** | Medium | ADT REST debug APIs |
| **DebuggerAttach** | `handlers_debugger.go` | debug | **YES** | Medium | ADT REST debug APIs |
| **DebuggerStep** | `handlers_debugger.go` | debug | **YES** | Medium | ADT REST debug APIs |
| **DebuggerGetStack** | `handlers_debugger.go` | debug | **YES** | Medium | ADT REST debug APIs |
| **DebuggerGetVariables** | `handlers_debugger.go` | debug | **YES** | Medium | ADT REST debug APIs |
| **DebuggerDetach** | `handlers_debugger.go` | debug | **YES** | Medium | ADT REST debug APIs |
| **CallRFC** | `handlers_debugger.go:256` | rfc | No | Low-Med | No REST alternative |
| **MoveObject** | `handlers_crud.go:405` | rfc | No | Low-Med | TR_TADIR_INTERFACE via RFC |

### 4.3 AMDP Tools (Irrelevant on 7.40 - No HANA)

| Tool | Domain | Notes |
|------|--------|-------|
| AMDPDebuggerStart | amdp | Requires HANA database |
| AMDPDebuggerStop | amdp | Requires HANA database |
| AMDPDebuggerResume | amdp | Requires HANA database |
| AMDPDebuggerStep | amdp | Requires HANA database |
| AMDPDebuggerGetVariables | amdp | Requires HANA database |
| AMDPDebuggerSetBreakpoint | amdp | Requires HANA database |
| AMDPDebuggerGetBreakpoints | amdp | Requires HANA database |

### 4.4 Impact Summary by Capability

| Capability | Tools Lost | Severity | Workaround Available? |
|-----------|-----------|----------|----------------------|
| **Report Execution** | RunReport, RunReportAsync, GetVariants, GetTextElements, SetTextElements | **High** | No ADT REST API for report execution |
| **Interactive Debugger** | SetBreakpoint, GetBreakpoints, DeleteBreakpoint, Listen, Attach, Step, Stack, Variables, Detach | **Medium** | ADT REST debug APIs exist but have CSRF issues |
| **abapGit Export** | GitTypes, GitExport | **Medium** | Requires abapGit REST API (separate install) |
| **RFC/Misc** | CallRFC, MoveObject | **Low-Med** | CallRFC has no REST alternative |
| **ABAP Help** | GetAbapHelp | **Low** | Already has graceful fallback |
| **AMDP Debugging** | All 7 AMDP tools | **None** | Not applicable on 7.40 |

**Total: ~20 tools affected out of 122 (expert) / 81 (focused)**

---

## 5. Tools That Work Without ZADT_VSP (~100+ tools)

Everything using standard ADT REST APIs is unaffected:

- All **read** operations (GetSource, GetObjectMetadata, GetObjectStructure, etc.)
- All **CRUD** operations (CreateObject, UpdateSource, DeleteObject, Lock/Unlock)
- All **search** operations (SearchObject, Grep)
- **SyntaxCheck**, **ActivateObject**, **PrettyPrint**
- **RunUnitTests**, **ExecuteABAP**
- **ATC checks**
- **Code intelligence** (FindDefinition, FindReferences, CodeCompletion)
- **GetCallGraph**, **GetCallersOf**, **GetCalleesOf**
- **GetSystemInfo**, **GetInstalledComponents**
- **ListDumps**, **GetDump** (RABAX short dumps)
- **ListTraces**, **GetTrace** (ATRA profiler)
- **SQL Trace** tools (ST05)
- **Transport management** (5 tools)
- **Class include operations** (GetClassComponents, GetClassInclude, UpdateClassInclude)
- **Object history/versions**
- **RAP/OData** tools (DDLS, SRVD, SRVB create + publish)

---

## 6. Solution Options

### Option A: ICF HTTP Handler (Recommended for stateless domains)

Replace the WebSocket/APC transport with a plain ICF HTTP handler. Available on all NW releases back to 6.40.

```abap
CLASS zcl_vsp_http_handler DEFINITION
  PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    INTERFACES if_http_extension.
ENDCLASS.

CLASS zcl_vsp_http_handler IMPLEMENTATION.
  METHOD if_http_extension~handle_request.
    DATA(lv_body) = server->request->get_cdata( ).
    DATA(ls_message) = parse_message( lv_body ).
    DATA(ls_response) = route_message( ls_message ).
    server->response->set_cdata( serialize_response( ls_response ) ).
    server->response->set_header_field(
      name = 'Content-Type' value = 'application/json' ).
  ENDMETHOD.
ENDCLASS.
```

**What works:** rfc, report, git, system, ABAP help - all stateless domains.

**Go-side change:** Add HTTP transport mode. Each MCP tool call becomes an HTTP POST to `/sap/bc/zadt_vsp` (or similar ICF path). Auto-select when WebSocket fails.

**Effort:** Low-Medium. Service classes are already decoupled via `zif_vsp_service`.

### Option B: Stateless APC (`cl_apc_wsp_ext_stateless_pcp_base`)

Inherit from stateless base class if APC exists at all on the 7.40 system.

**Risk:** `cl_apc_wsp_ext_stateless_pcp_base` may also not exist on 7.40. PCP was introduced later.

**What works:** Same as Option A (stateless domains only).

### Option C: ICF HTTP with Stateful Session for Debug (Hybrid)

Combine Option A with SAP's built-in HTTP session management for the debug domain:

```abap
server->set_session_stateful( stateful = if_http_server=>co_enabled ).
```

ICF stateful sessions have existed since NW 6.40. The ABAP work process is reserved for the session, similar to what the current APC stateful handler does.

**Go-side:** vsp already manages stateful HTTP sessions for ADT CRUD operations (`pkg/adt/http.go` - CSRF token, session cookies). The debug WebSocket client would become an HTTP client managing a `sap-contextid` session.

**What works:** Everything including debug domain.

### Option D: Externalized State (Database/Shared Memory)

Persist session state to DB table instead of instance attributes. However, TPDAPI object references (`if_tpdapi_session`, `if_tpdapi_service`) cannot be serialized - they are process-bound.

**Verdict:** Not viable for debugging.

---

## 7. Recommendation

**Option A (ICF HTTP) + Option C (stateful HTTP for debug):**

1. Create `zcl_vsp_http_handler` implementing `if_http_extension`
2. Reuse all existing service classes unchanged (they already return `zif_vsp_service=>ty_response`)
3. Register ICF node `/sap/bc/zadt_vsp` (or under existing path)
4. On Go side, add HTTP transport mode that vsp auto-selects when WebSocket connection fails
5. For debug: use ICF stateful session + sequential HTTP, or fall back to native ADT REST debug APIs
6. Disable AMDP domain automatically on non-HANA systems

### Implementation Priority

| Phase | What | Effort | Tools Recovered |
|-------|------|--------|----------------|
| 1 | ICF HTTP handler + stateless routing | Low | RunReport, GitExport, CallRFC, MoveObject, GetAbapHelp, etc. (11 tools) |
| 2 | Go HTTP transport fallback | Medium | Same 11 tools accessible from vsp |
| 3 | Stateful HTTP for debug domain | Medium-High | All 9 debugger tools |
| 4 | Feature detection + auto-mode | Low | Clean UX on mixed-version landscapes |

---

## 8. Appendix: NW Version Capability Matrix

| Capability | NW 7.31 | NW 7.40 | NW 7.50+ |
|-----------|---------|---------|----------|
| ICF HTTP handler | Yes | Yes | Yes |
| ADT REST APIs | Partial | Yes | Yes |
| WebSocket (ICM kernel) | No | Partial (SP08+) | Yes |
| APC stateless | No | Maybe (SP08+) | Yes |
| APC stateful | No | No* | Yes |
| AMC channels | No | Partial | Yes |
| HANA/AMDP debugging | No | No | Yes |

*Stateful APC on 7.40 depends on exact SP + kernel patch level and is not reliably available.
