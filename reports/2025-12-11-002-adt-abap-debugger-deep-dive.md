# ADT ABAP Debugger Deep Dive

**Date:** 2025-12-11
**Report ID:** 002
**Subject:** Understanding ADT External Debugging API and Designing Composite Tools
**Status:** In Progress

---

## Executive Summary

This report documents the investigation into SAP ADT's external debugging API for ABAP code. The goal is to enable AI-driven dynamic code analysis through programmatic debugging without SAP GUI.

---

## Current Understanding

### Debugging Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `user` | Debug all processes for a specific user | External IDE debugging |
| `terminal` | Debug processes from a specific terminal | Session-specific debugging |
| `deactivated` | Stop debugging | Cleanup |

### Breakpoint Scopes

| Scope | Description | Persistence |
|-------|-------------|-------------|
| `external` | External/static breakpoints | Persist across sessions |
| `debugger` | Session-bound breakpoints | Only during active debug session |

### Key Identifiers

| ID | Purpose | Generation |
|----|---------|------------|
| `terminalId` | Identifies IDE session | Must be consistent across API calls |
| `ideId` | Identifies IDE type | Usually "vsp" or "Eclipse" |
| `requestUser` | User to debug | SAP username |

---

## Investigation Findings

### Test 1: Basic Breakpoint Creation

**Attempt:** POST breakpoint with all required attributes
```xml
<?xml version="1.0" encoding="UTF-8"?>
<dbg:breakpoints xmlns:dbg="http://www.sap.com/adt/debugger"
                 xmlns:adtcore="http://www.sap.com/adt/core"
                 scope="external"
                 debuggingMode="user"
                 requestUser="TESTUSER"
                 terminalId="vsp-05191f0f4f475241"
                 ideId="vsp">
  <dbg:breakpoint kind="line" adtcore:uri="/sap/bc/adt/oo/classes/zcl_adt_debug_test/source/main#start=15"/>
</dbg:breakpoints>
```

**Result:** 200 OK but empty `<dbg:breakpoints/>` response
**Issue:** Breakpoint not actually stored

### Test 2: Various URI Formats

| URI Format | Result |
|------------|--------|
| `/sap/bc/adt/oo/classes/zcl_adt_debug_test/source/main#start=15` | 200 OK, empty |
| `/sap/bc/adt/oo/classes/zcl_adt_debug_test#start=15` | 200 OK, empty |
| `/sap/bc/adt/oo/classes/zcl_adt_debug_test/includes/testclasses#start=7` | 200 OK, empty |
| `/sap/bc/adt/programs/programs/saplselg/source/main#start=10` | 200 OK, empty |

All return 200 OK but no breakpoints are stored.

### Test 3: GET Breakpoints

**URL:** `/sap/bc/adt/debugger/breakpoints?scope=external&debuggingMode=user&requestUser=TESTUSER&terminalId=vsp-...&ideId=vsp`

**Result:** 200 OK with empty body - no breakpoints found

---

## Hypotheses

### Hypothesis A: Listener Must Start First

Based on the ADT deep dive report (Report 012), the correct sequence might be:

```
1. POST /debugger/listeners        <- Start listener FIRST
2. POST /debugger/breakpoints      <- Then set breakpoints
3. Run code (e.g., unit tests)     <- Trigger breakpoint
4. Listener returns with session   <- Debuggee caught
5. GET /debugger/stack             <- Inspect state
```

**Rationale:** The debug listener establishes a session context. External breakpoints might only work within that context.

### Hypothesis B: Session Cookies Required

The listener might set cookies that are required for breakpoint storage to work. Need to maintain the same HTTP client/session across:
- Start listener
- Set breakpoints
- Retrieve breakpoints

### Hypothesis C: Breakpoint Response Format

The empty `<dbg:breakpoints/>` response might indicate:
- Validation passed but breakpoint not actionable
- Need additional attributes (client ID, etc.)
- Line number doesn't map to executable code

---

## Next Steps

1. **Test Listener-First Sequence**
   - Start debug listener with timeout
   - Set breakpoints in same session
   - Verify breakpoints are stored
   - Run unit tests to trigger

2. **Capture Eclipse ADT Traffic**
   - Use proxy to capture exact XML Eclipse sends
   - Compare with our implementation

3. **Check Breakpoint Validation**
   - Use `/debugger/breakpoints/validations` to verify position
   - Check if line 15 maps to executable code

---

## API Reference

### Start Debug Listener
```
POST /sap/bc/adt/debugger/listeners
Query: debuggingMode=user&requestUser=USERNAME&timeout=60
Headers: x-csrf-token, Content-Type: application/xml
```

### Create Breakpoint
```
POST /sap/bc/adt/debugger/breakpoints
Headers: Content-Type: application/xml, Accept: application/xml
Body: <dbg:breakpoints...>
```

### Get Breakpoints
```
GET /sap/bc/adt/debugger/breakpoints
Query: scope=external&debuggingMode=user&requestUser=USERNAME&terminalId=XXX&ideId=vsp
```

### Available Debugging Endpoints
```
POST   /debugger/listeners              - Start/stop listener
GET    /debugger/listeners              - Get active listeners
DELETE /debugger/listeners              - Stop listener

POST   /debugger/breakpoints            - Create/sync breakpoints
GET    /debugger/breakpoints            - Get breakpoints
PUT    /debugger/breakpoints/{id}       - Update breakpoint
DELETE /debugger/breakpoints/{id}       - Delete breakpoint

GET    /debugger/breakpoints/statements - Get statement types
GET    /debugger/breakpoints/messagetypes - Get message types
POST   /debugger/breakpoints/validations - Validate breakpoint position
POST   /debugger/breakpoints/conditions - Validate condition

GET    /debugger/stack                  - Get call stack
GET    /debugger/variables              - Get variables
POST   /debugger?method=step            - Step
POST   /debugger?method=continue        - Continue
```

---

## Composite Tool Design (Draft)

### Option 1: Single Debug Session Tool

A single MCP tool that:
1. Starts listener in background goroutine
2. Sets breakpoints
3. Runs unit tests (or other trigger)
4. Returns when breakpoint hit or timeout
5. Exposes step/continue/variables as sub-operations

```
Tool: DebugWithBreakpoints
Parameters:
  - object_uri: URI of object to debug
  - breakpoints: [{line: 15}, {line: 20}]
  - trigger: "unit_tests" | "function_module" | "program"
  - trigger_target: "ZCL_TEST" | "Z_FM_NAME" | "ZPROGRAM"
  - timeout: 60

Returns:
  - session_id: For subsequent operations
  - status: "attached" | "timeout" | "error"
  - current_location: {uri, line}
  - stack: [{...}]
  - variables: [{...}]
```

### Option 2: Stateful Session with Multiple Tools

Maintain debug session state across tool calls:

```
1. DebugStartSession(user, timeout) -> session_id
2. DebugSetBreakpoint(session_id, uri, line)
3. DebugTrigger(session_id, trigger_type, target)
4. DebugWaitForBreakpoint(session_id, timeout) -> location
5. DebugGetStack(session_id) -> stack
6. DebugGetVariables(session_id, scope) -> variables
7. DebugStep(session_id, type) -> location
8. DebugContinue(session_id) -> location | finished
9. DebugStopSession(session_id)
```

### Recommendation

**Option 1 (Single Tool)** is better for MCP because:
- Each MCP call may be a separate process
- Maintaining session state across calls is challenging
- A single composite operation is more reliable

---

## Test Infrastructure

### Test Class: ZCL_ADT_DEBUG_TEST
Location: `$TMP` package
Purpose: Provides predictable code for debugging tests

```abap
CLASS zcl_adt_debug_test DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    METHODS: simple_operation RETURNING VALUE(rv_result) TYPE i.
ENDCLASS.

CLASS zcl_adt_debug_test IMPLEMENTATION.
  METHOD simple_operation.
    DATA lv_count TYPE i.       " Line ~12
    lv_count = 1.               " Line ~13
    lv_count = lv_count + 1.    " Line ~14
    rv_result = lv_count * 10.  " Line ~15 <- Set breakpoint here
  ENDMETHOD.
ENDCLASS.

" Test class (in includes/testclasses)
CLASS lcl_test DEFINITION FOR TESTING.
  PRIVATE SECTION.
    METHODS test_simple FOR TESTING.
ENDCLASS.

CLASS lcl_test IMPLEMENTATION.
  METHOD test_simple.
    DATA lo_test TYPE REF TO zcl_adt_debug_test.
    CREATE OBJECT lo_test.
    DATA(lv_result) = lo_test->simple_operation( ).
    cl_abap_unit_assert=>assert_equals( act = lv_result exp = 20 ).
  ENDMETHOD.
ENDCLASS.
```

---

## Appendix: Terminal ID Generation

For consistent terminal IDs across MCP tool calls:

```go
func getTerminalID(user string) string {
    h := make([]byte, 8)
    for i, c := range user {
        h[i%8] ^= byte(c)
    }
    return "vsp-" + hex.EncodeToString(h)
}

// Example: "TESTUSER" -> "vsp-05191f0f4f475241"
```

---

## Additional Investigation (2025-12-11)

### Test 4: Exception and Statement Breakpoints

Tested breakpoint types that don't require specific URI:

```xml
<!-- Exception breakpoint -->
<dbg:breakpoint kind="exception" clientId="vsp_exc_1" exceptionClass="CX_SY_ZERODIVIDE"/>

<!-- Statement breakpoint -->
<dbg:breakpoint kind="statement" clientId="vsp_stmt_1" statement="WRITE"/>
```

**Result:** Both return 200 OK with empty response - same as line breakpoints.

### Test 5: Listener-First Sequence

Tested the sequence:
1. Start debug listener (background, 20s timeout)
2. Set breakpoints (in same session)
3. Run unit tests
4. Wait for listener

**Result:**
- Listener starts successfully
- Listener returns after timeout with empty body (`Content-Length: 0`)
- Breakpoints still show empty response
- Unit tests execute but don't trigger debugging

### Analysis of SAP Transformation Files

Examined XSLT transformations used by SAP:
- `TPDA_ADT_BREAKPOINTS_REQUEST` - Defines input XML format
- `TPDA_ADT_BREAKPOINTS_RESPONSE` - Defines output XML format
- `SADT_OBJECT_REFERENCE` - Defines URI/type attributes

The transformations confirm our XML format is correct. The issue is likely:
1. User authorization/profile issue
2. System configuration (debugging not enabled)
3. Some required parameter we haven't discovered

---

## Current Blocker

**Problem:** Breakpoints cannot be created via ADT API despite correct XML format and 200 OK responses.

**Possible Causes:**
1. User `TESTUSER` may not have debugging authorization
2. System may need debugging enabled via transaction `SU01` or `SM04`
3. External debugging may require SAP GUI to be logged in first
4. Some undocumented cookie or session state required

**Recommended Next Steps:**
1. **Check User Authorization** - Verify TESTUSER has `S_DEBUG` authorization
2. **Try from SAP GUI** - Set breakpoint in SAP GUI first, then query via API
3. **Capture Eclipse Traffic** - Use HTTP proxy to see exact requests Eclipse sends
4. **Check SAP Note** - Search for ADT debugger configuration notes

---

## Current vsp Debugger Tools

The following tools exist in vsp but may not work until the breakpoint issue is resolved:

| Tool | Status | Notes |
|------|--------|-------|
| `SetExternalBreakpoint` | ⚠️ Blocked | API accepts but doesn't store |
| `GetExternalBreakpoints` | ✅ Works | Returns empty (no breakpoints) |
| `DeleteExternalBreakpoint` | ✅ Works | Nothing to delete |
| `DebuggerListen` | ✅ Works | Returns after timeout |
| `DebuggerAttach` | ❓ Untested | Needs successful breakpoint first |
| `DebuggerGetStack` | ❓ Untested | Needs active debug session |
| `DebuggerGetVariables` | ❓ Untested | Needs active debug session |
| `DebuggerStep` | ❓ Untested | Needs active debug session |
| `DebuggerDetach` | ❓ Untested | Needs active debug session |

---

*Report will be updated as investigation continues.*
