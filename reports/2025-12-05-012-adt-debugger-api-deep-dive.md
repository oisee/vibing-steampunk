# ADT External Debugger REST API - Deep Dive Analysis

**Date:** 2025-12-05
**Report ID:** 012
**Subject:** Complete analysis of SAP ADT Debugger REST API for external debugging integration
**Related Documents:** Report 005 (Native ADT Features)

---

## Executive Summary

The SAP ADT Debugger provides a **complete REST API** for external debugging that enables:
- Attaching to running ABAP processes
- Setting breakpoints (line, statement, exception, message)
- Inspecting variables and call stack
- Executing debug actions (step, continue, commit, rollback)
- Generating VALUE statements from runtime data

**Feasibility Assessment:** External debugging via MCP is **technically possible** but requires careful architectural consideration due to the synchronous, session-bound nature of debugging.

---

## 1. API Architecture Overview

### 1.1 Key Classes Discovered

| Class | Package | Purpose |
|-------|---------|---------|
| `CL_TPDA_ADT_RES_APP` | STPDA_ADT | REST API router - registers all debugger endpoints |
| `CL_TPDA_ADT_RES_DEBUGGER` | STPDA_ADT | Main debugger resource controller |
| `CL_TPDA_ADT_RES_LISTENERS` | STPDA_ADT | Debug session attachment (start/stop listeners) |
| `CL_TPDA_ADT_RES_BREAKPOINTS` | STPDA_ADT | Breakpoint management |
| `CL_TPDA_ADT_RES_VARIABLES` | STPDA_ADT | Variable inspection |
| `CL_TPDA_ADT_RES_STACK` | STPDA_ADT | Call stack navigation |
| `CL_TPDA_ADT_RES_ACTIONS` | STPDA_ADT | Debug actions (step, commit, etc.) |
| `CL_TPDA_ADT_RES_WATCHPOINTS` | STPDA_ADT | Watchpoint management |
| `CL_TPDA_ADT_RES_SYSTEM_AREAS` | STPDA_ADT | System variables (SY-*) |

### 1.2 API Classes (TPDAPI)

| Class | Purpose |
|-------|---------|
| `CL_TPDAPI_SERVICE` | Main debugger service entry point |
| `CL_TPDAPI_SESSION` | Debug session management |
| `CL_TPDAPI_BP_SERVICES` | Breakpoint services |
| `CL_TPDAPI_DATA_SERVICES` | Variable/data access |
| `CL_TPDAPI_BP` | Single breakpoint |
| `CL_TPDAPI_DATA` | Variable/symbol representation |
| `CL_TPDAPI_WP` | Watchpoint |

---

## 2. Complete REST API Endpoints

### 2.1 Main Debugger
```
/sap/bc/adt/debugger
```
Root resource for debugger operations.

### 2.2 Debug Session Listeners
```
POST   /sap/bc/adt/debugger/listeners  - Start debug listener (attach)
GET    /sap/bc/adt/debugger/listeners  - Get active listeners
DELETE /sap/bc/adt/debugger/listeners  - Stop debug listener (detach)
```

**Query Parameters:**
| Parameter | Description |
|-----------|-------------|
| `debuggingMode` | `user` or `terminal` |
| `requestUser` | User to debug (for user mode) |
| `terminalId` | Terminal ID (for terminal mode) |
| `ideId` | IDE identifier |
| `timeout` | Listen timeout in seconds (default: 240) |
| `checkConflict` | Check for debugging conflicts |
| `isNotifiedOnConflict` | Get notified on conflicts |

**Debugging Modes:**
- `user` - Debug all processes of a specific user
- `terminal` - Debug processes from a specific terminal
- `deactivated` - Stop debugging

### 2.3 Breakpoints
```
POST   /sap/bc/adt/debugger/breakpoints              - Create/sync breakpoints
GET    /sap/bc/adt/debugger/breakpoints              - Get breakpoints
DELETE /sap/bc/adt/debugger/breakpoints/{id}         - Delete breakpoint
PUT    /sap/bc/adt/debugger/breakpoints/{id}         - Update breakpoint

GET    /sap/bc/adt/debugger/breakpoints/statements   - Statement breakpoint types
GET    /sap/bc/adt/debugger/breakpoints/messagetypes - Message types for breakpoints
POST   /sap/bc/adt/debugger/breakpoints/conditions   - Validate breakpoint condition
POST   /sap/bc/adt/debugger/breakpoints/validations  - Validate breakpoint position
```

**Breakpoint Types:**
| Type | Description | Example |
|------|-------------|---------|
| `line` | Source line breakpoint | Stop at ZCL_TEST line 42 |
| `statement` | Statement type breakpoint | Stop at all WRITE statements |
| `exception` | Exception class breakpoint | Stop when CX_SY_ZERODIVIDE raised |
| `message` | Message breakpoint | Stop at MESSAGE E001(ZTEST) |

**Breakpoint Scopes:**
- `external` - External/static breakpoints (persist across sessions)
- `debugger` - Session-bound breakpoints (only during debug session)

### 2.4 Variables
```
GET    /sap/bc/adt/debugger/variables                           - Get all variables
GET    /sap/bc/adt/debugger/variables/{name}/data               - Get variable value
GET    /sap/bc/adt/debugger/variables/{name}/metadata           - Get variable type info
GET    /sap/bc/adt/debugger/variables/{name}/subcomponents      - Get structure/table components
GET    /sap/bc/adt/debugger/variables/{name}/valueStatement     - Generate VALUE statement!
POST   /sap/bc/adt/debugger/variables/{name}/data/{row}         - Insert table row
DELETE /sap/bc/adt/debugger/variables/{name}/data               - Delete table rows
```

**Variable Data Export (CSV):**
```
GET /sap/bc/adt/debugger/variables/{name}/data
Accept: text/csv
Query: ?offset=1&length=100&filter=...&sortComponent=...&sortDirection=...
```

**Generate VALUE Statement:**
```
GET /sap/bc/adt/debugger/variables/{name}/valueStatement
Accept: text/plain
Query: ?rows=1-10&maxStringLength=100&maxNestingLevel=5&maxTotalSize=10000
```

This amazing feature generates ABAP VALUE statements from runtime data!

### 2.5 Call Stack
```
GET /sap/bc/adt/debugger/stack                                  - Get full stack
PUT /sap/bc/adt/debugger/stack/type/{type}/position/{pos}       - Navigate to stack frame
```

### 2.6 Debug Actions
```
POST /sap/bc/adt/debugger/actions?action={action}&value={value}
```

**Available Actions:**
| Action | Description |
|--------|-------------|
| `garbageCollector` | Trigger garbage collection |
| `updateDebugging` | Toggle update debugging |
| `commitWork` | Execute COMMIT WORK |
| `rollbackWork` | Execute ROLLBACK WORK |
| `kernelDebugger` | Start kernel debugger |
| `memorySnapshot` | Create memory snapshot |
| `stepSizeLine` | Set step mode to line |
| `stepSizeExpression` | Set step mode to expression |

### 2.7 System Areas
```
GET /sap/bc/adt/debugger/systemareas                    - List system areas
GET /sap/bc/adt/debugger/systemareas/{area}             - Get system area values
```

System areas include SY-* variables, memory info, etc.

### 2.8 Watchpoints
```
GET    /sap/bc/adt/debugger/watchpoints                 - Get all watchpoints
POST   /sap/bc/adt/debugger/watchpoints                 - Create watchpoint
DELETE /sap/bc/adt/debugger/watchpoints/{id}            - Delete watchpoint
```

### 2.9 Memory Analysis
```
GET /sap/bc/adt/debugger/memorysizes?includeAbap=true   - Get memory sizes
```

### 2.10 Batch Operations
```
POST /sap/bc/adt/debugger/batch                         - Batch multiple requests
```

---

## 3. Debug Session Lifecycle

### 3.1 External Debugging Flow

```
┌─────────────────┐
│  IDE/MCP Client │
└────────┬────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────┐
│ 1. Start Listener                                        │
│    POST /debugger/listeners                              │
│    ?debuggingMode=user&requestUser=DEVELOPER&timeout=300 │
└────────┬────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────┐
│ 2. Set External Breakpoints                              │
│    POST /debugger/breakpoints                            │
│    Body: { scope: "external", breakpoints: [...] }       │
└────────┬────────────────────────────────────────────────┘
         │
         │ (User runs program that hits breakpoint)
         ▼
┌─────────────────────────────────────────────────────────┐
│ 3. Debuggee Session Attached                             │
│    Listener returns with session info                    │
└────────┬────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────┐
│ 4. Inspect State                                         │
│    GET /debugger/stack                                   │
│    GET /debugger/variables                               │
│    GET /debugger/systemareas                             │
└────────┬────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────┐
│ 5. Continue/Step                                         │
│    POST /debugger?method=step                            │
│    POST /debugger?method=continue                        │
└────────┬────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────┐
│ 6. Stop Listener                                         │
│    DELETE /debugger/listeners                            │
└─────────────────────────────────────────────────────────┘
```

### 3.2 Session States

```
┌──────────────┐     Start      ┌──────────────┐
│   Inactive   │ ───Listener──▶ │   Listening  │
└──────────────┘                └──────┬───────┘
                                       │ Debuggee
                                       │ Attached
                                       ▼
┌──────────────┐     Timeout    ┌──────────────┐
│   Stopped    │ ◀───────────── │   Attached   │
└──────────────┘                └──────┬───────┘
       ▲                               │
       │         Step/Continue         │
       └───────────────────────────────┘
```

---

## 4. Implementation Challenges

### 4.1 Synchronous Nature

The debug listener is **blocking** - it waits for a debuggee to attach:

```abap
" From CL_TPDA_ADT_RES_LISTENERS:
cl_tpdapi_service=>s_get_instance( )->start_listener_for_user(
   i_request_user             = l_request_user
   i_ide_user                 = sy-uname
   i_ide_id                   = l_ide_id
   i_timeout                  = l_timeout  " Default 240 seconds!
   i_flg_check_conflict       = l_flg_check_conflict
   i_flg_notified_on_conflict = l_flg_notified  ).
```

**Challenge:** MCP tools are expected to return quickly. A 240-second blocking call doesn't fit the MCP model.

### 4.2 Session Binding

Debug operations require an **attached session**:

```abap
" From CL_TPDA_ADT_RES_VARIABLES:
me->ref_session = me->ref_main_service->get_attached_session( ).
IF me->ref_session IS INITIAL.
  RAISE EXCEPTION TYPE cx_adt_rest_data_invalid
    EXPORTING subtype = cx_tpda_adt_failure=>c_subtype-no_Session_Attached.
ENDIF.
```

**Challenge:** Each HTTP request in MCP is stateless. Debugging requires persistent session state.

### 4.3 Conflict Detection

Multiple IDEs cannot debug the same user simultaneously:

```abap
CATCH cx_abdbg_actext_conflict_lis INTO lcx_conflict_lis.
  RAISE EXCEPTION TYPE cx_tpda_adt_ser_adi_failed
    EXPORTING
      subtype = 'conflictDetected'.
```

---

## 5. Proposed MCP Integration Architecture

### 5.1 Option A: Async Polling Model

```
┌─────────────┐                    ┌─────────────┐
│  AI Agent   │                    │  SAP System │
└──────┬──────┘                    └──────┬──────┘
       │                                  │
       │ 1. StartDebugListener            │
       │    (returns immediately)         │
       │─────────────────────────────────▶│
       │                                  │ (starts background listener)
       │ 2. PollDebugStatus               │
       │    (check if attached)           │
       │─────────────────────────────────▶│
       │◀─────────────────────────────────│
       │    "waiting"                     │
       │                                  │
       │    ... (user runs program) ...   │
       │                                  │
       │ 3. PollDebugStatus               │
       │─────────────────────────────────▶│
       │◀─────────────────────────────────│
       │    "attached" + session info     │
       │                                  │
       │ 4. GetDebugState                 │
       │─────────────────────────────────▶│
       │◀─────────────────────────────────│
       │    stack, variables, position    │
```

### 5.2 Option B: Breakpoint-Only Mode (Recommended)

Focus on **external breakpoints** without interactive debugging:

```
┌─────────────────────────────────────────────────────────────┐
│                    MCP Tools (Read-Only)                     │
├─────────────────────────────────────────────────────────────┤
│ SetExternalBreakpoint  - Set breakpoint for user            │
│ GetExternalBreakpoints - List external breakpoints          │
│ DeleteExternalBreakpoint - Remove breakpoint                │
│ ValidateBreakpointCondition - Check condition syntax        │
└─────────────────────────────────────────────────────────────┘
```

This enables:
- Setting breakpoints before running a program
- User debugs interactively in ADT/SAP GUI
- AI assists with breakpoint placement based on code analysis

### 5.3 Option C: Post-Mortem Analysis

Combine with existing diagnostic tools:

```
┌───────────────────────────────────────────────────────────────┐
│              AI-Assisted Root Cause Analysis                   │
├───────────────────────────────────────────────────────────────┤
│                                                               │
│  1. GetDumps          - Find recent crashes                   │
│  2. GetDump           - Analyze dump details + stack trace    │
│  3. GetSource         - Read source at crash location         │
│  4. FindDefinition    - Navigate to related code              │
│  5. GetCallGraph      - Understand call hierarchy             │
│  6. GrepPackages      - Search for similar patterns           │
│  7. SuggestFix        - AI proposes solution                  │
│                                                               │
└───────────────────────────────────────────────────────────────┘
```

---

## 6. VALUE Statement Generation - Hidden Gem!

The debugger can **generate ABAP VALUE statements** from runtime data:

```
GET /sap/bc/adt/debugger/variables/LT_DATA/valueStatement
    ?rows=1-10
    &maxStringLength=100
    &maxNestingLevel=5
```

Returns:
```abap
VALUE #(
  ( field1 = 'ABC' field2 = 123 )
  ( field1 = 'DEF' field2 = 456 )
).
```

**Use Cases:**
- Generate test data from production values
- Create unit test fixtures
- Document data structures
- Reproduce issues with real data

---

## 7. Recommended Implementation Phases

### Phase 1: External Breakpoints (Low Complexity)
- `SetExternalBreakpoint` - Create external breakpoint
- `GetExternalBreakpoints` - List breakpoints
- `DeleteExternalBreakpoint` - Remove breakpoint
- `ValidateBreakpointCondition` - Check condition

### Phase 2: Debug State Inspection (Medium Complexity)
- `GetDebugStack` - Get current call stack (when attached)
- `GetDebugVariables` - Get variable values
- `GetDebugSystemAreas` - Get SY-* variables
- `GenerateValueStatement` - Create VALUE from runtime data

### Phase 3: Interactive Debugging (High Complexity)
- `StartDebugListener` - Begin listening (async)
- `PollDebugStatus` - Check attachment status
- `DebugStep` - Step over/into/out
- `DebugContinue` - Continue execution
- `StopDebugListener` - End debugging

---

## 8. Conclusion

The ADT Debugger REST API is **feature-complete** for external debugging. However, integrating it into MCP requires architectural decisions:

| Approach | Complexity | Value |
|----------|------------|-------|
| External Breakpoints Only | Low | Medium - helps set up debugging |
| Post-Mortem RCA | Low | **High** - leverages existing tools |
| Full Interactive Debug | High | High - but conflicts with MCP model |

**Recommendation:** Start with **Post-Mortem RCA** (Report 013) as it provides immediate value using existing tools, then add **External Breakpoints** support as a stepping stone to full debugging.

---

## Appendix: Source Code References

| File | Line | Description |
|------|------|-------------|
| `CL_TPDA_ADT_RES_APP` | register_resources | All endpoint registrations |
| `CL_TPDA_ADT_RES_DEBUGGER` | post | Main request routing |
| `CL_TPDA_ADT_RES_LISTENERS` | post | Start listener implementation |
| `CL_TPDA_ADT_RES_BREAKPOINTS` | call_bp_api | Breakpoint creation logic |
| `CL_TPDA_ADT_RES_VARIABLES` | get_value_statement | VALUE statement generation |
| `CL_TPDA_ADT_RES_ACTIONS` | post | Action execution |

---

*Report generated from analysis of SAP ADT classes in packages STPDA_ADT and STPDA_API*
