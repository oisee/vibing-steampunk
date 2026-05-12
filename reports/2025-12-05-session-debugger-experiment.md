# Debugger Experiment Design

**Date:** 2025-12-05
**Report ID:** Session Report
**Subject:** AI-Powered Debugging Demo Experiment

---

## Objective

Demonstrate that the new debugger tools work end-to-end:
1. Set a breakpoint on a test program
2. Trigger the breakpoint by running the program
3. Catch the debuggee with the listener
4. Attach to the debug session
5. Inspect call stack and variables
6. Step through code
7. Detach cleanly

## Prerequisites

1. **vsp** built with new debugger tools (v2.8.0)
2. MCP server restarted to load new tools
3. Test program exists: `ZTEST_MCP_CRUD` in package `$TMP`

## New MCP Tools (6 tools)

| Tool | Description |
|------|-------------|
| `DebuggerListen` | Wait for debuggee to hit breakpoint (blocking, long-poll) |
| `DebuggerAttach` | Attach to caught debuggee |
| `DebuggerDetach` | Release debuggee and end session |
| `DebuggerStep` | Step into/over/return/continue |
| `DebuggerGetStack` | Get call stack with program/line info |
| `DebuggerGetVariables` | Get variable values |

## Experiment Steps

### Phase 1: Setup

```
1. Clean up any existing breakpoints:
   - Call GetExternalBreakpoints to list current breakpoints
   - Call DeleteExternalBreakpoint for each

2. Set a line breakpoint on ZTEST_MCP_CRUD:
   - Call SetExternalBreakpoint with:
     - kind: "line"
     - object_uri: "/sap/bc/adt/programs/programs/ZTEST_MCP_CRUD/source/main"
     - line: 15 (or first executable line)
```

### Phase 2: Trigger Breakpoint

```
3. Start debug listener (in background or with short timeout):
   - Call DebuggerListen with timeout: 60

4. In parallel (from SAP GUI or another session):
   - Execute ZTEST_MCP_CRUD (SE38 > F8)
   - The program should hit the breakpoint

5. DebuggerListen returns with debuggee info:
   - Debuggee ID (needed for attach)
   - Program name
   - Current line
```

### Phase 3: Debug Session

```
6. Attach to debuggee:
   - Call DebuggerAttach with debuggee_id from step 5
   - Should return session info and available actions

7. Get call stack:
   - Call DebuggerGetStack
   - Shows program hierarchy with current position marked

8. Get variables:
   - Call DebuggerGetVariables (defaults to @ROOT)
   - Shows all local variables with values

9. Step through code:
   - Call DebuggerStep with step_type: "stepOver"
   - Repeat a few times to see execution progress
   - After each step, call DebuggerGetStack to see new position
```

### Phase 4: Cleanup

```
10. Detach from session:
    - Call DebuggerDetach
    - Releases the debuggee (program continues or terminates)

11. Clean up breakpoints:
    - Call DeleteExternalBreakpoint to remove test breakpoint
```

## Expected Results

### DebuggerListen Output
```
Debuggee caught!

Debuggee ID: ABC123XYZ...
User: TESTUSER
Program: ZTEST_MCP_CRUD
Include: ZTEST_MCP_CRUD
Line: 15
Kind: debuggee
Attachable: true
App Server: VHCALA4HCI

Use DebuggerAttach with the debuggee_id to attach to this session.
```

### DebuggerGetStack Output
```
Call Stack:

Server: VHCALA4HCI_A4H_01
Current Stack Index: 1

→ [1] ZTEST_MCP_CRUD::ZTEST_MCP_CRUD (line 15)
      Type: REPORT, Include: ZTEST_MCP_CRUD

  [2] SAPMSSY2::SYSTEM (line 24)
      Type: REPORT, Include: SAPMSSY2
      (system program)
```

### DebuggerGetVariables Output
```
Variables:

LV_COUNT: I = 42
  MetaType: simple, Kind: LOCAL

LS_DATA: TY_DATA =
  MetaType: structure, Kind: LOCAL
  (complex type - use variable ID 'LS_DATA' to expand)

LT_RESULTS: TABLE =
  MetaType: table, Kind: LOCAL
  (complex type - use variable ID 'LT_RESULTS' to expand)
```

## Test Program Reference

If `ZTEST_MCP_CRUD` doesn't exist, create a simple test program:

```abap
REPORT ztest_mcp_crud.

DATA: lv_count TYPE i VALUE 42,
      lv_text  TYPE string VALUE 'Hello Debug'.

" Line 15 - good breakpoint location
lv_count = lv_count + 1.

WRITE: / 'Count:', lv_count.
WRITE: / 'Text:', lv_text.
```

## Challenges & Notes

1. **Timing**: The debug listener blocks until a breakpoint is hit. You need to:
   - Start listener with reasonable timeout (60s)
   - Quickly switch to SAP GUI and run the program

2. **Session Management**: Each SAP session can only have one active debugger. If you get "conflict" errors, stop existing listeners first.

3. **Breakpoint Targeting**: Line breakpoints need the exact executable line. Comments or declarations may not work.

## Success Criteria

- [ ] Breakpoint created successfully
- [ ] DebuggerListen catches debuggee
- [ ] DebuggerAttach establishes session
- [ ] DebuggerGetStack shows call stack with correct line
- [ ] DebuggerGetVariables shows variable values
- [ ] DebuggerStep advances execution
- [ ] DebuggerDetach cleanly ends session

---

## Commands Quick Reference

```
# Setup
GetExternalBreakpoints
SetExternalBreakpoint kind=line object_uri=/sap/bc/adt/programs/programs/ZTEST_MCP_CRUD/source/main line=15

# Listen (start this, then run program in SAP GUI)
DebuggerListen timeout=60

# Debug session
DebuggerAttach debuggee_id=<id-from-listen>
DebuggerGetStack
DebuggerGetVariables
DebuggerStep step_type=stepOver
DebuggerDetach

# Cleanup
DeleteExternalBreakpoint breakpoint_id=<id>
```
