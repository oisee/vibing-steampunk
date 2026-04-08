# GUI Debugger Design & Implementation Plan

**Date:** 2026-04-05
**Report ID:** 001
**Subject:** Issue #2 — Web GUI Debugger for ABAP, approaches and estimates
**Related:** Report 2025-12-05-014 (External Debugger Scripting Vision), Report 2025-12-05-024 (AMDP Goroutine Architecture)

---

## 1. Problem Statement

Issue #2: Users want to debug ABAP programs visually — set breakpoints, step through code, inspect variables — without SAP GUI. Current MCP tools provide all primitives (breakpoints, stepping, stack, variables) but lack a **visual interactive interface**.

### Current Pain Points
- MCP tool-based debugging requires AI to orchestrate step-by-step (slow, error-prone)
- No visual code display with breakpoint markers
- No real-time event streaming (breakpoint hit → immediate UI update)
- External breakpoints from vsp don't trigger SAP GUI runs (by design — different session context)

---

## 2. What We Have Today

### 2.1 WebSocket Debugger (ZADT_VSP) — Full Implementation

| Capability | Status | Location |
|-----------|--------|----------|
| Line breakpoints | Full | `pkg/adt/websocket_debug.go` |
| Method breakpoints | Full | `pkg/adt/websocket_debug.go` |
| Statement breakpoints | Full | `pkg/adt/websocket_debug.go` |
| Exception breakpoints | Full | `pkg/adt/websocket_debug.go` |
| Listen for debuggee | Full | `pkg/adt/websocket_debug.go` |
| Attach/Detach | Full | `pkg/adt/websocket_debug.go` |
| Step (into/over/return/continue) | Full | `pkg/adt/websocket_debug.go` |
| Call stack | Full | `pkg/adt/websocket_debug.go` |
| Variable inspection | Full | `pkg/adt/websocket_debug.go` |

### 2.2 AMDP Debugger (HANA/SQLScript) — Experimental

| Capability | Status |
|-----------|--------|
| Session lifecycle | Full |
| Breakpoints | Full |
| Step execution | Full |
| Variable inspection | Full |
| ExecuteAndDebug (atomic) | Full |

### 2.3 REST Legacy Debugger — Fallback

- Listen/Attach/Step/Variables via ADT REST API
- Breakpoint setting broken (403 CSRF in newer SAP versions)
- Kept as fallback for systems without ZADT_VSP

### 2.4 CLI Interactive Debugger

- `vsp debug` REPL command (step, next, out, continue, vars, stack, breakpoints)
- Proves the primitives work end-to-end

---

## 3. Reference: How abap-fs Does It

### 3.1 Architecture

**abap-fs** (VS Code extension by Marcello Urbani) implements ABAP debugging as a **DAP (Debug Adapter Protocol)** provider:

```
VS Code ←→ DAP ←→ abap-adt-api ←→ SAP ADT REST API
```

Key library: `abap-adt-api` (TypeScript) — wraps ADT REST endpoints for debugging.

### 3.2 ADT Endpoints Used by abap-fs

```
POST   /sap/bc/adt/debugger              — Create debug session, attach, step
GET    /sap/bc/adt/debugger/listeners     — Check listener status
POST   /sap/bc/adt/debugger/listeners     — Register listener (blocking)
DELETE /sap/bc/adt/debugger/listeners     — Deregister listener
POST   /sap/bc/adt/debugger/breakpoints   — Set breakpoints (XML body)
DELETE /sap/bc/adt/debugger/breakpoints/X — Delete breakpoint
GET    /sap/bc/adt/debugger/stack         — Get call stack
POST   /sap/bc/adt/debugger/variables     — Get variables (XML query)
```

### 3.3 abap-fs Debugger Flow

1. User sets breakpoints in VS Code editor gutter
2. DAP `launch`/`attach` → registers listener via `/debugger/listeners` (POST, blocks)
3. User triggers execution (run report, unit test, RFC) in SAP
4. Listener returns with debuggee info
5. DAP `attach` → attaches to debuggee via `/debugger` (POST)
6. VS Code shows paused state — yellow highlight on current line
7. Step/Continue → `/debugger` (POST with stepType)
8. Variables panel → `/debugger/variables` (POST)
9. Stack panel → `/debugger/stack` (GET)
10. DAP `disconnect` → detach

### 3.4 Key Insights from abap-fs

| Aspect | abap-fs Approach | Our Advantage |
|--------|-----------------|---------------|
| Transport | REST API only | WebSocket (faster, bidirectional) |
| Breakpoints | REST POST (XML) | WebSocket JSON (no CSRF issues) |
| Event model | Long-poll (listener blocks) | WebSocket push (instant) |
| Session | Stateful HTTP | Persistent WebSocket |
| Protocol | DAP (VS Code specific) | MCP + HTTP (universal) |
| Trigger | Manual (SAP GUI run) | Programmatic (CallRFC, RunReport) |

### 3.5 What We Can Learn

1. **DAP is proven** — VS Code's debug UI is excellent, mature, well-tested
2. **REST API works** for attach/step/variables (even if breakpoints are flaky)
3. **The hard part is UX** — code display, variable tree, responsive stepping
4. **abap-fs doesn't solve** the "trigger execution" problem — user still runs from SAP GUI

---

## 4. Three Implementation Approaches

### Approach A: Web GUI Debugger (Standalone HTML5)

**Concept:** Embed a lightweight web debugger UI served by vsp's HTTP transport.

```
Browser ←→ HTTP/WebSocket ←→ vsp HTTP server ←→ ZADT_VSP ←→ SAP
```

**Architecture:**
```
vsp serve --http :8080
  ├── /api/debug/breakpoints     — CRUD breakpoints
  ├── /api/debug/listen          — Start listening (returns SSE stream)
  ├── /api/debug/attach          — Attach to debuggee
  ├── /api/debug/step            — Step execution
  ├── /api/debug/stack           — Get call stack
  ├── /api/debug/variables       — Get/expand variables
  ├── /api/source/{program}      — Get ABAP source code
  ├── /ws/debug/events           — WebSocket event stream
  └── /ui/                       — Static HTML5 debugger UI
```

**UI Components:**
1. **Code Panel** — ABAP source with line numbers, clickable gutter for breakpoints
2. **Control Bar** — Play/Pause/StepIn/StepOver/StepOut/Stop buttons
3. **Variables Panel** — Expandable tree (locals, globals, system vars)
4. **Stack Panel** — Clickable call frames
5. **Breakpoints Panel** — List with enable/disable/delete
6. **Console/Output** — Debug messages, WRITE output

**Pros:**
- Universal (any browser, any OS)
- No VS Code dependency
- Can embed in MCP client UIs
- Full control over UX

**Cons:**
- Significant frontend development (10-20h)
- Need to build code editor component
- State management complexity

**Estimate: 20-30h total**
- Backend API layer: 4-6h (wrapping existing WebSocket calls)
- Frontend UI: 12-18h (code panel, variables tree, controls)
- Integration & testing: 4-6h

### Approach B: DAP Provider (VS Code / IDE Integration)

**Concept:** Implement Debug Adapter Protocol so VS Code can debug ABAP natively.

```
VS Code ←→ DAP (stdin/stdout) ←→ vsp dap-server ←→ ZADT_VSP ←→ SAP
```

**Architecture:**
```
vsp dap
  ├── DAP: initialize          → capabilities
  ├── DAP: setBreakpoints      → SetBreakpoint (WebSocket)
  ├── DAP: launch/attach       → Listen + Attach (WebSocket)
  ├── DAP: next/stepIn/stepOut → Step (WebSocket)
  ├── DAP: stackTrace          → GetStack (WebSocket)
  ├── DAP: scopes              → GetVariables (WebSocket)
  ├── DAP: variables           → GetVariables by parent ID
  ├── DAP: continue            → Step("continue")
  └── DAP: disconnect          → Detach (WebSocket)
```

**Pros:**
- VS Code's debug UI is production-grade (free)
- Works with any DAP-compatible IDE (JetBrains, Neovim, Emacs)
- Breakpoint gutter clicks, variable hover, call stack — all built in
- Minimal frontend code needed

**Cons:**
- Requires VS Code extension packaging
- DAP protocol has specific quirks (thread model, scope hierarchy)
- abap-fs already exists (competing, though their approach is REST-only)

**Estimate: 15-20h total**
- DAP protocol handler: 6-8h (Go DAP library + message routing)
- WebSocket bridge: 3-4h (map DAP calls to existing debug client)
- VS Code extension manifest: 2-3h (launch.json config, package.json)
- Testing & edge cases: 4-5h

### Approach C: Hybrid — MCP + Streamable HTTP Debug Events

**Concept:** Extend the existing Streamable HTTP transport with debug event streaming. MCP clients (Claude Desktop, Cursor, etc.) get debug capabilities directly.

```
MCP Client ←→ Streamable HTTP ←→ vsp MCP server ←→ ZADT_VSP ←→ SAP
```

**New MCP Tools:**
```
DebugSession:
  - action: "start"     → registers listener, returns session ID
  - action: "status"    → check if debuggee caught
  - action: "attach"    → attach to debuggee
  - action: "step"      → step execution
  - action: "variables" → get variables (with depth expansion)
  - action: "stack"     → get call stack
  - action: "stop"      → detach and cleanup

DebugBreakpoints:
  - action: "set"       → set breakpoint
  - action: "list"      → list breakpoints
  - action: "delete"    → delete breakpoint
```

**Enhanced with MCP Notifications:**
```
// Server-initiated notifications (Streamable HTTP supports this)
→ debug/breakpointHit { program, line, debuggeeID }
→ debug/stepComplete  { program, line }
→ debug/sessionEnded  { reason }
```

**Pros:**
- Works within existing MCP infrastructure
- AI can orchestrate debugging intelligently
- No separate UI needed (MCP client renders)
- Lowest implementation effort

**Cons:**
- Depends on MCP client rendering capabilities
- No universal visual debugger
- AI-mediated (not direct user interaction)

**Estimate: 8-12h total**
- Refactor debug tools into session-based API: 3-4h
- Add MCP notifications for debug events: 2-3h
- Integration with Streamable HTTP: 2-3h
- Documentation & examples: 1-2h

---

## 5. Recommended Strategy: Phased Approach

### Phase 1: MCP Debug Session Tools (8-12h) — Approach C
- Consolidate existing debug MCP tools into session-based API
- Add event notifications via Streamable HTTP
- AI agents can debug autonomously
- **Value:** Immediate improvement for Claude/Cursor users

### Phase 2: DAP Provider (15-20h) — Approach B
- Implement DAP protocol handler in Go
- Bridge to existing WebSocket debug client
- VS Code extension for ABAP debugging via vsp
- **Value:** Professional IDE debugging experience

### Phase 3: Web GUI (20-30h) — Approach A
- Standalone browser-based debugger
- Embedded in vsp HTTP server
- Universal access, no IDE required
- **Value:** SAP-GUI-free debugging for everyone

### Total Estimate: 43-62h across all phases

---

## 6. Comparison with abap-fs

| Feature | abap-fs | vsp (Current) | vsp (Phase 1) | vsp (Phase 2) | vsp (Phase 3) |
|---------|---------|---------------|----------------|----------------|----------------|
| Breakpoints | REST (flaky) | WebSocket (solid) | WebSocket | WebSocket | WebSocket |
| Step/Continue | REST | WebSocket | MCP tools | DAP | Web UI |
| Variables | REST | WebSocket | MCP tools | DAP tree | Web tree |
| Stack | REST | WebSocket | MCP tools | DAP | Web UI |
| Code display | VS Code | None | AI renders | VS Code | Web editor |
| Trigger execution | Manual | CallRFC/RunReport | Same | Same | Web button |
| AMDP debug | No | Yes (experimental) | Yes | Yes | Yes |
| Session model | Stateful HTTP | Persistent WebSocket | Same | Same | Same |
| AI integration | No | Full (MCP) | Enhanced | Partial | Partial |

### Our Key Advantage Over abap-fs
1. **WebSocket transport** — no CSRF issues, bidirectional, faster
2. **Programmatic triggering** — CallRFC/RunReport to hit breakpoints without SAP GUI
3. **AI orchestration** — Claude can drive debugging sessions intelligently
4. **AMDP support** — HANA/SQLScript debugging (abap-fs has none)
5. **Single binary** — no Node.js/npm dependency

---

## 7. Quick Wins Before Full Implementation

### 7.1 Improve Existing Debug MCP Tools (2-3h)
- Add `GetSource` call before breakpoint set (show code context)
- Return source snippet with step results (show surrounding lines)
- Better variable formatting (table preview, structure expansion)

### 7.2 Debug Workflow Templates (1-2h)
- YAML workflow: "debug this class method"
- Sets breakpoint → runs unit test → captures vars → reports

### 7.3 Document Current Debug Capabilities (1h)
- README section with debug examples
- Common debug patterns (unit test trigger, RFC trigger)

---

## 8. Open Questions for Discussion

1. **Should Phase 2 (DAP) compete with abap-fs** or complement it?
   - Option: Provide DAP adapter that works WITH abap-fs source provider
   - Option: Full standalone debugger (requires source file mapping)

2. **Web GUI technology stack** — what frontend framework?
   - Minimal: Vanilla HTML/JS + CodeMirror
   - Modern: React/Svelte + Monaco Editor
   - Embedded: HTMX + server-rendered (least JS)

3. **Should the web debugger also show AMDP/SQLScript?**
   - Dual-pane: ABAP source + HANA SQL source
   - Switch between ABAP and HANA debug sessions

4. **Integration with MCP sampling** — can Claude auto-debug?
   - Set breakpoints based on error analysis
   - Step through and diagnose autonomously
   - Report findings in natural language

---

## 9. Live Experiment on A4H (2026-04-05)

### System: A4H 758, non-HANA, abapGit available

**Test Results:**

| Capability | Result | Notes |
|-----------|--------|-------|
| ZADT_VSP WebSocket | FAIL (403) | Not installed on A4H |
| SET_BREAKPOINT (WebSocket) | FAIL | Requires ZADT_VSP |
| RUN_REPORT (WebSocket) | FAIL | Requires ZADT_VSP |
| LISTEN (REST) | OK | Timed out correctly after 5s |
| System Info | OK | SAP release 758 |
| Feature Detection | OK | abapGit only |

### Key Insight: ZADT_VSP Dependency Problem

Current debug tools are **hard-coupled** to ZADT_VSP WebSocket. On systems without it:
- Breakpoints: FAIL
- Report execution: FAIL
- Listener: Works (REST fallback exists)

**Action needed for GUI debugger:**
1. REST fallback path must be first-class, not afterthought
2. ZADT_VSP install tool exists (`SAP(action="system", target="INSTALL_ZADT_VSP")`) — document as prerequisite
3. Phase 1 (MCP debug session) must work with BOTH transports

### Impact on Estimates

| Phase | With ZADT_VSP | Without ZADT_VSP (REST only) | Delta |
|-------|--------------|------------------------------|-------|
| Phase 1 (MCP) | 8-12h | +3-4h for REST parity | 11-16h |
| Phase 2 (DAP) | 15-20h | +2-3h | 17-23h |
| Phase 3 (Web) | 20-30h | +3-5h | 23-35h |

### Test Program Available

`ZADT_WASM_TEST` in `$TMP` — WASM compiler test with 12 assertions, good candidate for debug demos.

---

## 10. ADT Debugger REST API — Full Endpoint Map (from CL_TPDA_ADT_RES_APP)

Discovered by reading SAP A4H 758 source code of `CL_TPDA_ADT_RES_APP.register_resources()`.

### 10.1 Complete Endpoint Catalog

| Endpoint | Method | Purpose | Template Parameters |
|----------|--------|---------|---------------------|
| `/debugger` | POST | Main debugger control (attach, step, settings) | — |
| `/debugger/listeners` | POST | Register listener (blocks until BP hit) | `debuggingMode, requestUser, terminalId, ideId, timeout, checkConflict, isNotifiedOnConflict` |
| `/debugger/listeners` | DELETE | Stop listener | `debuggingMode, requestUser, terminalId, ideId, checkConflict, notifyConflict` |
| `/debugger/listeners` | GET | Check listener status | `debuggingMode, requestUser, terminalId, ideId, checkConflict` |
| `/debugger/breakpoints` | POST | Set/sync breakpoints | `checkConflict` |
| `/debugger/breakpoints/{id}` | DELETE | Delete breakpoint | — |
| `/debugger/breakpoints/conditions` | — | Breakpoint conditions | — |
| `/debugger/breakpoints/validations` | — | Breakpoint validation | — |
| `/debugger/breakpoints/statements` | GET | List statement types for BPs | — |
| `/debugger/breakpoints/messagetypes` | GET | List message types for BPs | — |
| `/debugger/breakpoints/vit` | — | VIT breakpoints | — |
| `/debugger/breakpoints/vit/id/{handle}` | — | VIT breakpoint by handle | — |
| `/debugger/stack` | GET | Call stack | — |
| `/debugger/stack/type/{type}/position/{pos}` | GET | Stack frame details | — |
| `/debugger/variables` | GET | Variables overview | — |
| `/debugger/variables/{name}/{part}` | GET | Variable detail | `maxLength` |
| `/debugger/variables/{name}/{part}` | GET | Subcomponents | `component, line` |
| `/debugger/variables/{name}/{part}` | GET | CSV export | `offset, length, filter, sortComponent, sortDirection, whereClause, components*` |
| `/debugger/variables/{name}/{part}` | GET | VALUE statement gen | `rows, maxStringLength, maxNestingLevel, maxTotalSize, ignoreInitialValues` |
| `/debugger/variables/{name}/{part}/{subpart}` | GET | Deep sub-variable | — |
| `/debugger/watchpoints` | POST | Create watchpoint | `variableName, condition` |
| `/debugger/watchpoints` | GET | List watchpoints | — |
| `/debugger/watchpoints/{id}` | DELETE | Delete watchpoint | — |
| `/debugger/memorysizes` | GET | Memory consumption | `includeAbap` |
| `/debugger/systemareas` | GET | System areas | — |
| `/debugger/systemareas/{area}` | GET | System area detail | `offset, length, element, isSelection, selectedLine, selectedColumn, programContext, filter` |
| `/debugger/actions` | POST | Debugger actions | `action, value` |
| **`/debugger/batch`** | POST | **Batch request** (multiple ops in one call!) | — |

### 10.2 Key Discoveries

1. **Batch endpoint** (`/debugger/batch`) — can combine multiple debug ops in ONE HTTP call. Critical for performance in REST-only mode!
2. **Variable CSV export** — tables can be exported as CSV with filtering, sorting, WHERE clause
3. **VALUE statement generation** — can generate ABAP VALUE statements from live variable state (amazing for test data!)
4. **Watchpoints via REST** — full CRUD, not just via WebSocket
5. **Breakpoint conditions** — dedicated endpoint for condition management
6. **VIT breakpoints** — Variable Inspection Tracing (advanced feature)
7. **Memory sizes** — runtime memory consumption monitoring
8. **System areas** — access to SY-*, kernel variables

### 10.3 REST-Only Debug Strategy (No ZADT_VSP)

With the batch endpoint, a complete debug flow can be efficient even without WebSocket:

```
1. POST /debugger/batch
   Body: [
     { "url": "/debugger/breakpoints", "method": "POST", body: "<breakpoint xml>" },
     { "url": "/debugger/listeners?timeout=120&...", "method": "POST" }
   ]

2. (Listener returns when BP hit)

3. POST /debugger/batch
   Body: [
     { "url": "/debugger", "method": "POST", body: "<attach>" },
     { "url": "/debugger/stack", "method": "GET" },
     { "url": "/debugger/variables", "method": "GET" }
   ]
   → One call gets: attachment + stack + variables!

4. POST /debugger/batch
   Body: [
     { "url": "/debugger", "method": "POST", body: "<step over>" },
     { "url": "/debugger/stack", "method": "GET" },
     { "url": "/debugger/variables", "method": "GET" }
   ]
   → Step + refresh in one call!
```

### 10.4 Revised Estimate with REST Batch

The batch endpoint changes Phase 1 significantly — REST-only can be almost as good as WebSocket:

| Phase | Original | With REST Batch | Notes |
|-------|----------|-----------------|-------|
| Phase 1 (MCP) | 8-12h + 3-4h REST | **10-14h** | Batch makes REST nearly as efficient |
| Phase 2 (DAP) | 15-20h + 2-3h | **16-21h** | Minor — DAP maps cleanly to batch |
| Phase 3 (Web) | 20-30h + 3-5h | **22-32h** | Batch reduces round-trips dramatically |

---

## 11. SAP Kernel Debugger API (IF_TPDAPI_SERVICE + IF_TPDAPI_SESSION)

Extracted from A4H 758 system — the internal API that ADT REST endpoints call.

### 11.1 IF_TPDAPI_SERVICE — Listener & Attachment

| Method | Purpose | Key Params |
|--------|---------|------------|
| `START_LISTENER_FOR_USER` | Listen for debuggee by user | `timeout, request_user, ide_id, post_mortem, check_conflict` |
| `START_LISTENER_FOR_TERMINAL_ID` | Listen by terminal | `timeout, terminal_id, ide_id` |
| `STOP_LISTENER_FOR_USER` | Stop listener | `request_user, ide_id` |
| `STOP_LISTENER_FOR_TERMINAL_ID` | Stop listener | `terminal_id, ide_id` |
| `ATTACH_DEBUGGEE` | Attach to caught debuggee | `debuggee_id` → returns `IF_TPDAPI_SESSION` |
| `GET_WAITING_DEBUGGEES` | List caught debuggees | `terminal_id, ide_id, request_user` |
| `GET_ACTIVE_LISTENERS` | List active listeners | `debugger_mode, terminal_id` |
| `CHECK_LISTENER_CONFLICT_USER` | Check for conflicts | `user, ide_id` |
| `ACTIVATE_SESSION_FOR_EXT_DEBUG` | Enable external debug | `ide_user, terminal_id, is_embedded_sapgui` |

### 11.2 IF_TPDAPI_SESSION — Active Debug Session (29 methods!)

| Method | Purpose |
|--------|---------|
| `GET_CONTROL_SERVICES` | Step/Continue control |
| `GET_SOURCE` | Get source code at current position |
| `GET_DATA_SERVICES` | Variable inspection |
| `GET_BP_SERVICES` | Breakpoint management in session |
| `GET_WP_SERVICES` | Watchpoint management |
| `GET_STACK_HANDLER` | Call stack |
| `GET_SCRIPT_HANDLER` | Debugger scripting |
| `SET_SETTINGS` / `GET_SETTINGS` | system_debugging, update_debugging, etc. |
| `GET_SYSTEM_AREA` | SY-*, kernel vars (with filtering!) |
| `GET_LOADED_PROGRAMS` | All loaded programs |
| `GET_DEBUGGER_STATUS` | Current status |
| `IS_RFC` / `IS_SAME_SYSTEM` | Session info |
| `TOGGLE_ATRA_STATE` | Runtime analysis toggle |
| `SET_INCREMENT_MODE` | LINE or EXPRESSION stepping |

### 11.3 IF_TPDAPI_STATIC_BP_SERVICES — Breakpoint Factory

| Method | Purpose |
|--------|---------|
| `CREATE_LINE_BREAKPOINT` | Set breakpoint by line |
| `CREATE_STATEMENT_BREAKPOINT` | Set by statement type |
| `CREATE_EXCEPTION_BREAKPOINT` | Set by exception class |
| `CREATE_MESSAGE_BREAKPOINT` | Set by message type |
| `DELETE_BREAKPOINT` | Remove breakpoint |
| `GET_BREAKPOINTS` | List all breakpoints |
| `SET_EXTERNAL_BP_CONTEXT_USER` | Set external BP context (key!) |
| `SET_EXTERNAL_BP_CONTEXT_TERMID` | Set external BP context by terminal |

### 11.4 Key Event Types (IF_TPDAPI_EVENT)

```
C_ID_BREAKPOINTS          — Breakpoint hit
C_ID_WATCHPOINTS          — Watchpoint triggered
C_ID_EXC_OCCURRED         — Exception occurred
C_ID_LAYER_ENTRY/EXIT     — ABAP layer changes
C_ID_NEW_SLAVE            — New session attached
C_ID_NEW_BREAKPOINTS      — Breakpoints changed
C_ID_WP_EXPIRED           — Watchpoint expired
C_ID_ROLLAREA             — Roll area entered
C_ID_SLASHH_ACTIVATION    — /H activation
```

### 11.5 Implications for Implementation

**For Phase 2 (DAP):**
- `IF_TPDAPI_SESSION` maps almost 1:1 to DAP protocol
- Session has `GET_SOURCE` — can provide source without separate ADT call
- `SET_INCREMENT_MODE("EXPRESSION")` enables sub-statement stepping
- Script handler enables debugger automation

**For Phase 3 (Web GUI):**
- `GET_SYSTEM_AREA` with filtering = powerful SY-* inspector
- `GET_LOADED_PROGRAMS` = module browser during debug
- `TOGGLE_ATRA_STATE` = one-click profiling during debug session
- Variable CSV export + VALUE statement = data extraction tools

**New feature unlocked:** `/debugger/actions` endpoint can execute custom debugger actions (like changing variables, navigating, toggling features). This is beyond just stepping — full debugger scripting over REST!

---

## Appendix: Key Source Files

| File | Purpose |
|------|---------|
| `pkg/adt/websocket_debug.go` | WebSocket debug client (all primitives) |
| `pkg/adt/websocket_base.go` | WebSocket connection management |
| `pkg/adt/amdp_websocket.go` | AMDP/HANA debug client |
| `pkg/adt/debugger.go` | Type definitions, terminal ID management |
| `internal/mcp/handlers_debugger.go` | MCP tool handlers (WebSocket) |
| `internal/mcp/handlers_debugger_legacy.go` | MCP tool handlers (REST fallback) |
| `internal/mcp/handlers_amdp.go` | AMDP debug MCP tools |
| `cmd/vsp/debug.go` | CLI interactive debugger |
| `embedded/abap/zcl_vsp_*` | ZADT_VSP ABAP service code |
