# ABAP Debugger

vsp provides AI-driven ABAP debugging through an external debugger interface. Set breakpoints, listen for debug sessions, step through code, and inspect variables — all via natural language.

## How it works

1. **Set a breakpoint** on a line, exception, or method entry
2. **Run your ABAP** (via `RunUnitTests`, RFC call, report, or any trigger)
3. **Listen** for the debug session to connect
4. **Attach** and inspect the call stack and variables
5. **Step** through execution (over, into, return, continue)

The debugger communicates via the ZADT_VSP WebSocket handler for reliable stateful connections. Classic HTTP debugging is also supported but less reliable for long sessions.

---

## Quick start

```
1. SetBreakpoint(type="line", program="ZTEST_PROGRAM", line=42)

2. RunUnitTests(object_type="PROG", object_name="ZTEST_PROGRAM")
   (this triggers the breakpoint)

3. DebuggerListen(timeout=60)
   → returns session info when debuggee connects

4. DebuggerAttach(session_id="...")

5. DebuggerGetStack()
   → shows current call frames

6. DebuggerGetVariables(frame=0)
   → shows variable values

7. DebuggerStep(type="over")  // or "into", "return", "continue"

8. DebuggerDetach()
```

---

## Breakpoint Types

| Type | Description | Required Parameters |
|------|-------------|---------------------|
| `line` | Break at specific line number | `program`, `line` |
| `exception` | Break when exception is raised | `exception` (e.g. `CX_SY_ZERODIVIDE`) |
| `statement` | Break on ABAP statement | `statement` (e.g. `MESSAGE`) |
| `method` | Break on method entry | `class`, `method` |

```
SetBreakpoint(type="exception", exception="CX_SY_ZERODIVIDE")
SetBreakpoint(type="line", program="ZCL_MY_CLASS===CP", line=105)
```

**Method-level breakpoints:** vsp automatically resolves method names to their include programs (e.g. `ZCL_MY_CLASS.METHOD_NAME` → correct include line).

---

## SAP GUI Terminal ID Integration

Breakpoints in SAP are scoped to a **terminal ID**. By default, vsp uses a terminal ID derived from your username. To share breakpoints with SAP GUI:

### Find your SAP GUI Terminal ID

- **Windows:** Registry key `HKCU\Software\SAP\ABAP Debugging\TerminalID`
- **Linux/macOS:** File `~/.SAP/ABAPDebugging/terminalId`

### Configure vsp to use the same ID

```bash
# .env
SAP_TERMINAL_ID=D0C586D015974B75BFB2A306A4A13AEA

# or CLI flag
vsp --terminal-id D0C586D015974B75BFB2A306A4A13AEA
```

With a matching terminal ID, breakpoints set by vsp are visible in SAP GUI and vice versa.

---

## Lua Scripting for Debugger Automation

For complex debugging workflows, use the built-in Lua REPL:

```bash
# Interactive REPL
vsp lua

# Run a script
vsp lua examples/scripts/debug-session.lua

# Inline
vsp lua -e 'print(json.encode(searchObject("ZCL_*", 10)))'
```

### Example: Capture test cases for AI generation

```lua
-- Set breakpoint on function module entry
local bpId = setBreakpoint("SAPL<FGROUP>", 10)

-- Capture multiple executions
local captures = {}
for i = 1, 5 do
    local event = listen(120)
    if not event then break end

    attach(event.id)
    local vars = getVariables()
    captures[i] = { inputs = vars, timestamp = os.time() }

    stepOver()
    captures[i].outputs = getVariables()
    saveCheckpoint("testcase_" .. i, captures[i])
    detach()
end

print(json.encode(captures))
```

### Available Lua functions

**Debug control:** `setBreakpoint`, `listen`, `attach`, `detach`, `stepOver`, `stepInto`, `stepReturn`, `continue_`

**Inspection:** `getStack`, `getVariables`

**Checkpoints:** `saveCheckpoint`, `getCheckpoint`, `listCheckpoints`, `injectCheckpoint`

**Source/Search:** `getSource`, `writeSource`, `editSource`, `searchObject`, `grepObjects`

**Diagnostics:** `getDumps`, `getDump`, `runUnitTests`, `syntaxCheck`

**Analysis:** `getCallGraph`, `getCallersOf`, `getCalleesOf`

---

## AI-Powered Root Cause Analysis

vsp enables AI assistants to investigate production issues autonomously:

```
User: "Investigate the ZERODIVIDE crash in production"

AI Workflow:
  1. GetDumps       → Find recent crashes by exception type
  2. GetDump        → Analyze stack trace and variable values
  3. GetSource      → Read code at crash location
  4. GetCallGraph   → Trace call hierarchy
  5. GrepPackages   → Find similar patterns
  6. Analysis       → Identify root cause
  7. WriteSource    → Generate fix + test case
```

---

## Triggering breakpoints without SAP GUI

Use `RunUnitTests` as a trigger vehicle:

1. Create a test class with a method that calls your target code
2. Set a breakpoint in the target code
3. Call `RunUnitTests` — the breakpoint fires during test execution
4. Attach with `DebuggerListen` + `DebuggerAttach`

This enables fully AI-driven debugging without any manual SAP GUI interaction.

---

## ZADT_VSP WebSocket Handler

The debugger (and several other advanced features) requires the ZADT_VSP WebSocket handler to be installed on your SAP system. Install it with one command:

```
InstallZADTVSP(package="$ZADT_VSP")
```

Or check prerequisites first:
```
InstallZADTVSP(check_only=true)
```

See the [Tool Reference](../tools/reference.md) for InstallZADTVSP parameters.
