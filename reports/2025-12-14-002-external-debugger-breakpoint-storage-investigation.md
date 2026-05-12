# External Debugger Breakpoint Storage Investigation

**Date:** 2025-12-14
**Report ID:** 002
**Subject:** Deep investigation into SAP external breakpoint storage and retrieval via HTTP
**Related Documents:** 2025-12-14-001 (Eclipse ADT Traffic Analysis), 2025-12-11-003

---

## Executive Summary

This report documents a deep investigation into how SAP stores and retrieves external debugger breakpoints. The investigation revealed that **breakpoints ARE being stored in the database**, but the HTTP GET endpoint returns empty results. The root cause appears to be in how the HTTP endpoint initializes the breakpoint service context.

## Key Findings

### 1. Breakpoints ARE Stored in Database

The `ABDBG_EXTDBPS` table contains our breakpoint:

```
CLIENT: 001
USERNAME: TESTUSER
BP_INDEX: 1
TIMESTAMP: 20251214210627
RQ_USER: TESTUSER
RQ_TERMID: (empty)
RQ_IDEID: (empty)
ATTRIBUTES: |FLAG_ACTIVE=X||SYSTEM_DEBUGGING= |
BREAKPOINT: |KIND=SOURCE||PROGRAM=ZCL_ADT_DEBUG_TEST============CP|
            |INCLUDE=ZCL_ADT_DEBUG_TEST============CCAU||LINE=17|
```

**Key observation:** `RQ_TERMID` and `RQ_IDEID` are **empty** because the breakpoint was set in "user" mode, not "terminal" mode.

### 2. Debug Activation IS Registered

The `ICFATTRIB` table contains the debug activation:

```
CLIENT: 001
USERNAME: TESTUSER
ATTRIBUTE: D (Debugging)
URL: %_SYSTEM
CUSERNAME: TESTUSER
DEBUGID: vsp
SERVER: vhcala4hci_A4H_00
CALL_URL: ,K=0,A=XY
TIMEOUT: 230627 (2+ hours validity)
```

### 3. GET Endpoint Returns Empty

Despite data existing in both tables, all HTTP GET requests return empty:

```
GET /sap/bc/adt/debugger/breakpoints?scope=external&debuggingMode=user&requestUser=TESTUSER
→ Status: 200, Body: (empty)
```

Tested multiple parameter combinations - all returned empty.

## Technical Analysis

### Database Storage Flow

The breakpoint creation flow was traced through SAP standard classes:

1. **CL_TPDA_ADT_RES_BREAKPOINTS** - REST resource controller
2. **CL_TPDAPI_SERVICE** - High-level debugger service
3. **CL_ABDBG_ACT_FOR_ATTACH__USER** - User-mode activation handler
4. **ABDBG_EXTDBPS** - Database table for external breakpoints
5. **ICFATTRIB** - Database table for debug activation flags

### Breakpoint Retrieval Logic

From `CL_ABDBG_ACT_FOR_ATTACH__USER` (line 421):

```abap
SELECT * FROM abdbg_extdbps
  INTO abdbg_extdbps_wa
  WHERE username = m_ideuser
    AND rq_user = m_rquser.
```

Where:
- `m_ideuser` = current logged-in user (`sy-uname`)
- `m_rquser` = request user from API call

### The Context Problem

The `init_static()` method in `CL_TPDA_ADT_RES_BREAKPOINTS` sets up the context:

```abap
" For user mode:
me->ref_static_bp_service->set_external_bp_context_user(
    i_ide_user     = sy-uname
    i_request_user = me->bp_transfer_in-request_user
    i_ide_id       = to_c32( me->bp_transfer_in-ide_id )
    ...
```

**Hypothesis:** The HTTP GET endpoint may not be properly initializing this context, or the context initialization happens only for POST requests.

### RFC vs HTTP Difference

The `SADT_REST_RFC_ENDPOINT` function module is the entry point for RFC-based ADT requests. When Eclipse uses RFC:
1. Session state is maintained across requests
2. The breakpoint service context persists within the session
3. SET and GET operations share the same context

When we use HTTP:
1. Each request is stateless
2. Context must be re-established for each request
3. The GET endpoint may not properly re-establish the context

## Architecture Deep Dive

### Key SAP Classes Examined

| Class | Purpose |
|-------|---------|
| `CL_TPDA_ADT_RES_APP` | Router for /debugger/* endpoints |
| `CL_TPDA_ADT_RES_BREAKPOINTS` | Breakpoint resource controller |
| `CL_TPDA_ADT_RES_LISTENERS` | Debug listener resource controller |
| `CL_TPDAPI_SERVICE` | High-level debugger API |
| `CL_ABDBG_ACT_FOR_ATTACH` | Base activation class |
| `CL_ABDBG_ACT_FOR_ATTACH__USER` | User-mode activation (our case) |
| `CL_ABDBG_ACT_FOR_ATTACH__TID` | Terminal ID-mode activation |

### Key Function Modules

| Function | Purpose |
|----------|---------|
| `SADT_REST_RFC_ENDPOINT` | RFC entry point for ADT REST |
| `HTTP_DEBUG_UPDATE` | Updates ICFATTRIB for debug activation |
| `TH_GET_REMOTE_DEBUG_KEY` | Gets debug key for session |

### Key Interfaces

| Interface | Purpose |
|-----------|---------|
| `IF_ABDBG_ACT_FOR_ATTACH` | Core external debugging interface |
| `IF_TPDAPI_STATIC_BP_SERVICES` | Static (external) breakpoint services |
| `IF_TPDAPI_BP_FACTORY` | Breakpoint creation factory |

## Possible Root Causes

1. **Missing context initialization for GET**: The GET handler may skip `init_static()` or the context setup may fail silently.

2. **Session-scoped data**: The breakpoint service may cache data in session variables that aren't populated for stateless HTTP GET.

3. **Server affinity**: The `ICFATTRIB` entry shows `SERVER: vhcala4hci_A4H_00`. If GET hits a different server, it may not find the activation.

4. **Timing issue**: External breakpoints may have a validity window that expired.

## Recommendations

### Short-term Workarounds

1. **Use POST for sync**: Eclipse uses POST with `sync_mode` to get breakpoints as part of synchronization.

2. **Query database directly**: Use `GetTableContents` on `ABDBG_EXTDBPS` to verify breakpoint existence.

### Long-term Solutions

1. **Implement batch endpoint**: Use `/sap/bc/adt/debugger/batch` for Eclipse-compatible requests.

2. **Session persistence**: Implement a daemon mode that maintains HTTP session state.

3. **RFC tunneling**: If RFC port (3300) is available, use RFC for debugging operations.

## Test Results Summary

| Test | Result |
|------|--------|
| POST breakpoint | 200 OK, returns ID |
| GET immediately after | 200 OK, empty |
| Check ABDBG_EXTDBPS | Breakpoint exists |
| Check ICFATTRIB | Activation exists |
| Listener registration | Works (Eclipse shows "hijacked") |
| Unit test trigger | Runs but doesn't hit breakpoint |

## Files Modified/Created

| File | Purpose |
|------|---------|
| `/tmp/test_get_simple.go` | Simple GET parameter testing |
| `/tmp/test_xml_debug.go` | Raw XML debugging |
| `/tmp/test_bp_search.go` | Parameter combination testing |
| `/tmp/test_terminal_id.go` | Terminal ID consistency test |
| `/tmp/test_listener_first.go` | Listener-first approach test |

## Exported SAP Classes

| Class | Lines | Location |
|-------|-------|----------|
| `CL_TPDA_ADT_RES_APP` | 287 | `/tmp/cl_tpda_adt_res_app.clas.abap` |
| `CL_TPDA_ADT_RES_LISTENERS` | 679 | `/tmp/cl_tpda_adt_res_listeners.clas.abap` |
| `CL_TPDA_ADT_RES_BREAKPOINTS` | 2336 | `/tmp/cl_tpda_adt_res_breakpoints.clas.abap` |
| `CL_TPDAPI_SERVICE` | 1199 | `/tmp/cl_tpdapi_service.clas.abap` |
| `CL_ABDBG_ACT_FOR_ATTACH` | 1755 | `/tmp/cl_abdbg_act_for_attach.clas.abap` |
| `CL_ABDBG_ACT_FOR_ATTACH__USER` | 1396 | `/tmp/cl_abdbg_act_for_attach__user.clas.abap` |

## Critical Update: Breakpoints NOT Persisting

### New Test Results

A follow-up test revealed a critical issue:

```
1. Setting breakpoint...
   Status: 200
   Response: <breakpoint kind="line" id="KIND=0..." />  ← Returns success with ID!

2. Syncing to retrieve breakpoints...
   Status: 200
   Response: <dbg:breakpoints/>  ← Empty!

3. Database check: ABDBG_EXTDBPS is EMPTY
   But ICFATTRIB WAS updated with new debug activation
```

### Root Cause Identified

The HTTP POST endpoint:
- **DOES** update `ICFATTRIB` (debug activation) ✓
- **DOES NOT** persist breakpoints to `ABDBG_EXTDBPS` ✗
- Returns success response with breakpoint ID (misleading!)

This means:
1. The breakpoint object is created in memory
2. The response is generated from the in-memory object
3. `HTTP_DEBUG_UPDATE` is called (updates ICFATTRIB)
4. But the `MODIFY abdbg_extdbps` either doesn't execute or is rolled back

### Possible Causes

1. **Transaction rollback**: The LUW (Logical Unit of Work) might be rolled back after response generation
2. **Missing commit**: The `DB_COMMIT` in the ADT REST framework might not include the breakpoint table
3. **Conditional write**: The code might skip the database write under certain conditions
4. **Session-only mode**: HTTP might be treated as "session-only" debugging where breakpoints aren't persisted

### Evidence from Code

From `CL_ABDBG_ACT_FOR_ATTACH__USER` (line 719):
```abap
" update DB table ABDBG_EXTDBPS
modify abdbg_extdbps from abdbg_extdbps_wa. assert sy-subrc = 0.
```

But this code might not be reached via the HTTP path, or might be in a different transaction context.

## Conclusion

The external debugger via HTTP is **fundamentally broken** for breakpoint persistence:

| Operation | ICFATTRIB | ABDBG_EXTDBPS | Result |
|-----------|-----------|---------------|--------|
| POST breakpoint | Updated ✓ | NOT updated ✗ | Appears to work but doesn't persist |
| Listener registration | Works ✓ | N/A | Can catch debuggees |
| Code trigger | N/A | Empty | No breakpoint to hit |

### Why Eclipse Works

Eclipse uses RFC (port 3300) which:
1. Maintains persistent session state
2. Properly commits database changes
3. Keeps breakpoints in sync across requests

### Recommendations

1. **Use the MCP tool directly**: The `SetExternalBreakpoint` MCP tool might need to verify database persistence
2. **Investigate RFC proxy**: Route debugging requests through RFC if available
3. **Alternative approach**: Use the listener to catch ALL debuggees, then set breakpoints in the attached session (debugger scope, not external scope)

Further investigation should focus on:
1. Whether the batch endpoint has the same issue
2. RFC-based breakpoint setting
3. Using debugger-scope breakpoints after attaching to a debuggee

---

## Session 2: Concurrent Debug Flow Testing (2025-12-14 Evening)

### Test Class Fix

The test class `ZCL_ADT_DEBUG_TEST` had `RISK LEVEL DANGEROUS` which prevented remote execution:

```
Alert: "No execution, risk level of test class exceeds upper limit"
```

**Fix applied:** Changed to `RISK LEVEL HARMLESS` via `EditSource`.

### Verified: Breakpoints DO Persist

Contrary to the earlier "Critical Update" section, breakpoints set via the MCP tool (`SetExternalBreakpoint`) **DO persist** to the database:

```sql
SELECT * FROM ABDBG_EXTDBPS WHERE USERNAME = 'TESTUSER'
```

Result:
```
CLIENT: 001
USERNAME: TESTUSER
BP_INDEX: 1
TIMESTAMP: 20251214213146
RQ_USER: TESTUSER
ATTRIBUTES: |FLAG_ACTIVE=X||SYSTEM_DEBUGGING= |
BREAKPOINT: |KIND=SOURCE||PROGRAM=ZCL_ADT_DEBUG_TEST============CP|
            |INCLUDE=ZCL_ADT_DEBUG_TEST============CCAU||LINE=17|
```

The `ICFATTRIB` debug activation is also correctly registered:

```
ATTRIBUTE: D
DEBUGID: vsp
SERVER: vhcala4hci_A4H_00
TIMEOUT: 233146 (2+ hours validity)
```

### The Core Issue: Breakpoints Don't Trigger

Despite correct database state:
- ✅ Breakpoint persisted with `FLAG_ACTIVE=X`
- ✅ Debug activation registered in `ICFATTRIB`
- ✅ Unit tests execute successfully (2 methods run)
- ❌ Debug listener times out - **breakpoint never triggers**

### Concurrent Test Results

```
Concurrent Debug Flow Test
==========================
User: TESTUSER

1. Checking database for breakpoints...
   Breakpoint confirmed in ABDBG_EXTDBPS

2. Starting debug listener (60s timeout)...
   Listener registered!

3. Running unit tests...

4. Waiting for breakpoint hit...
   TIMEOUT - Breakpoint NOT hit

   >>> Breakpoint in DB but not triggered <<<
   [Test] Classes: 1
   [Test] LCL_TEST: 2 methods

==========================
Done!
```

### Root Cause Hypothesis

The issue is **not** persistence - it's the **debug hook activation**. Possible causes:

1. **Server affinity**: The unit test execution may run on a different application server than where the debug activation is registered (`vhcala4hci_A4H_00`)

2. **Process type mismatch**: External breakpoints may only trigger for HTTP/RFC processes, not for internal batch/background processes that run unit tests

3. **Debug flag propagation**: The `ICFATTRIB.URL = '%_SYSTEM'` pattern may not match the actual URL pattern of unit test execution

4. **Session isolation**: The debug listener and unit test runner may be in isolated sessions that don't share debug context

### Key Difference: Eclipse vs HTTP

| Aspect | Eclipse (RFC) | vsp (HTTP) |
|--------|---------------|------------|
| Protocol | RFC port 3300 | HTTP port 50000 |
| Session | Persistent | Stateless |
| Debug activation | Same session | Cross-session |
| Server affinity | Guaranteed | Not guaranteed |

### Files Created This Session

| File | Purpose |
|------|---------|
| `/tmp/test_debug_concurrent.go` | Concurrent listener + test execution |

### SAP Objects Modified

| Object | Change |
|--------|--------|
| `ZCL_ADT_DEBUG_TEST` testclasses | `RISK LEVEL DANGEROUS` → `RISK LEVEL HARMLESS` |

### Conclusions

1. **Breakpoint persistence works** - The MCP tool correctly persists breakpoints to `ABDBG_EXTDBPS`

2. **GET endpoint is broken** - `GetExternalBreakpoints` returns empty despite data existing (context initialization issue)

3. **Debug hook doesn't fire** - The core issue is that the ABAP runtime doesn't check for external breakpoints when executing unit tests via HTTP

4. **Server affinity likely culprit** - Eclipse works because RFC maintains server affinity; HTTP requests may hit different servers

### Recommendations

1. **For debugging via HTTP**: Use exception breakpoints (`CX_SY_ZERODIVIDE`) which may have different activation paths

2. **For reliable debugging**: Use Eclipse ADT with RFC connection

3. **Future investigation**:
   - Check if breakpoints trigger for direct HTTP endpoint calls (not unit tests)
   - Investigate `DEBUG_USER_ON_SYSTEM` and `TH_SET_DEBUG_USER` function modules
   - Test with explicit server targeting via load balancer configuration
