---
name: rca
description: Start root cause analysis for a runtime error or dump
allowed-tools: Bash, Read, Grep
argument-hint: [<dump_id> | <exception_type> | <program_name>]
---

Investigate a runtime error using the 4-phase Root Cause Analysis workflow.

## Workflow

1. **Discover the failure:**
   - If dump ID provided: Run **GetDump** directly
   - If exception type provided: Run **ListDumps** filtered by exception type
   - If program name provided: Run **ListDumps** filtered by program
   - If no argument: Run **ListDumps** for the current user, last 24 hours
2. Present the dump details: exception type, failing program/method, line number, stack trace
3. Read the failing source code with **GetSource** (method-level for classes)
4. Analyze the code and propose likely root cause
5. If WebSocket debugging is available (check **GetFeatures**):
   - Offer to reproduce with breakpoints (Phase 2-3 of RCA)
   - Guide through: SetExternalBreakpoint → RunUnitTests → DebuggerListen → DebuggerAttach → inspect variables
6. Propose a fix and offer to apply it

## Prerequisites

- Phase 1 (discovery + static analysis): Always available
- Phases 2-3 (reproduction + debugging): Requires ZADT_VSP WebSocket handler
- Phase 4 (fix): Requires write permissions

## Example Usage

```
/vsp:rca                           # Recent dumps for current user
/vsp:rca CX_SY_ZERODIVIDE         # Find zero-division dumps
/vsp:rca ZADT_TEST_REPORT          # Dumps from specific program
/vsp:rca 20260329_142355_DEV_001   # Specific dump ID
```
