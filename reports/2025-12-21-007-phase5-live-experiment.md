# Phase 5 Live Experiment: TAS-Style Debugging

**Date:** 2025-12-21
**Report ID:** 007
**Subject:** Live experiment to validate Phase 5 recording and replay features

---

## Objective

Validate the Phase 5 TAS-Style Debugging tools with a real SAP system:
1. Record a debug session with variable tracking
2. Query the recorded history
3. Test checkpoint save/restore
4. Demonstrate Force Replay capability

---

## Prerequisites

### 1. SAP System Access

```bash
export SAP_URL="http://a4h.desude.su:50000"
export SAP_USER="TESTUSER"
export SAP_PASSWORD="2wsx@WSX2wsx"
export SAP_CLIENT="001"
```

### 2. Test Program: ZTEST_PHASE5_TAS

Create a new program for Phase 5 experiments. Use vsp or SE38:

```bash
# Via vsp:
./vsp write-source --type PROG --name ZTEST_PHASE5_TAS --package '$TMP' --source "$(cat <<'ABAP'
*&---------------------------------------------------------------------*
*& Report ZTEST_PHASE5_TAS
*& Test program for Phase 5 TAS-Style Debugging experiments
*&---------------------------------------------------------------------*
REPORT ztest_phase5_tas.

DATA: lv_counter TYPE i VALUE 0,
      lv_status  TYPE string VALUE 'INIT',
      lv_amount  TYPE p DECIMALS 2 VALUE '0.00',
      lv_name    TYPE string VALUE 'Test'.

* Entry point - set breakpoint here (line 13)
lv_status = 'STARTING'.

* Loop for variable change tracking
DO 10 TIMES.
  lv_counter = lv_counter + 1.
  lv_amount = lv_counter * 100.

  IF lv_counter = 5.
    lv_status = 'MIDPOINT'.  " State change at step 5
  ENDIF.

  IF lv_counter = 8.
    lv_status = 'NEAR_END'.  " State change at step 8
  ENDIF.

  WRITE: / 'Step:', lv_counter, 'Amount:', lv_amount, 'Status:', lv_status.
ENDDO.

lv_status = 'COMPLETE'.
WRITE: / 'Final status:', lv_status.
ABAP
)"
```

**Why this program?**
- Has a loop (10 iterations) → multiple steps to record
- `LV_COUNTER` changes every iteration → test `findChanges()`
- `LV_STATUS` changes at specific points → test `findWhenChanged()`
- `LV_AMOUNT` calculated value → test state reconstruction

**Existing ZTEST_MCP_CRUD** is too simple (no loop, no variable changes):
```abap
DATA: lv_message TYPE string.
lv_message = 'Hello from MCP CRUD test!'.
WRITE: / lv_message.
```

---

## Experiment 1: Basic Recording

### Step 1: Set Breakpoint and Start Recording

```lua
-- experiment1_record.lua

print("=== Experiment 1: Basic Recording ===")
print("")

-- Set breakpoint at line 13 (lv_status = 'STARTING')
local bp = setBreakpoint("ZTEST_PHASE5_TAS", 13)
print("Breakpoint set:", json.encode(bp))

-- Start recording
startRecording("experiment-001", "ZTEST_PHASE5_TAS")
print("Recording started")

-- Wait for program execution
print("")
print(">>> Now execute ZTEST_PHASE5_TAS in SE38 (F8) <<<")
print("Waiting for breakpoint hit (60 second timeout)...")
print("")

local event = listen(60)
if not event then
    print("ERROR: No debuggee attached within timeout")
    stopRecording()
    return
end

print("Debuggee attached:", event.debuggee_id)
attach(event.debuggee_id)
```

### Step 2: Step Through and Record

```lua
-- Continue from above...

local max_steps = 50
print("")
print("Stepping through " .. max_steps .. " steps...")

for i = 1, max_steps do
    -- Get current state
    local stack = getStack()
    local vars = getVariables()

    -- Log interesting changes
    if vars and vars.LV_STATUS then
        local status = vars.LV_STATUS.value
        if vars.LV_STATUS.is_changed then
            print(string.format("  Step %d: LV_STATUS changed to '%s'", i, status))
            saveCheckpoint("status_" .. status)
        end
    end

    -- Check if program ended
    if not stack or #stack == 0 then
        print("Program ended at step " .. i)
        break
    end

    stepOver()
end

-- Stop recording
local stats = stopRecording()
print("")
print("Recording complete!")
print("Stats:", json.encode(stats))

-- Detach
detach()
```

### Expected Output

```
=== Experiment 1: Basic Recording ===

Breakpoint set: {"id":"BP001","kind":"line","program":"ZTEST_PHASE5_TAS","line":12}
Recording started

>>> Now execute ZTEST_PHASE5_TAS in SE38 (F8) <<<
Waiting for breakpoint hit (60 second timeout)...

Debuggee attached: DEBUGGEE_12345

Stepping through 50 steps...
  Step 1: LV_STATUS changed to 'STARTING'
  Step 12: LV_STATUS changed to 'MIDPOINT'
  Step 20: LV_STATUS changed to 'NEAR_END'
  Step 28: LV_STATUS changed to 'COMPLETE'
Program ended at step 32

Recording complete!
Stats: {"total_steps":32,"checkpoints":4,"is_complete":true}
```

---

## Experiment 2: History Query

### Query When Variables Changed

```lua
-- experiment2_query.lua

print("=== Experiment 2: History Query ===")
print("")

-- Find when LV_STATUS became MIDPOINT
local step = findWhenChanged("LV_STATUS", "MIDPOINT")
print("LV_STATUS became 'MIDPOINT' at step: " .. step)

-- Get state at that moment
local state = getStateAtStep(step)
print("")
print("State at step " .. step .. ":")
for name, var in pairs(state) do
    print(string.format("  %s = %s (%s)", name, tostring(var.value), var.type))
end

-- Find all changes to LV_COUNTER
print("")
print("All changes to LV_COUNTER:")
local changes = findChanges("LV_COUNTER")
for _, change in ipairs(changes) do
    print(string.format("  Step %d: %s -> %s",
        change.step,
        tostring(change.old_value),
        tostring(change.new_value)))
end
```

### Expected Output

```
=== Experiment 2: History Query ===

LV_STATUS became 'MIDPOINT' at step: 12

State at step 12:
  LV_COUNTER = 5 (I)
  LV_STATUS = MIDPOINT (STRING)
  LV_AMOUNT = 500.00 (P)
  LV_NAME = Test (STRING)

All changes to LV_COUNTER:
  Step 3: 0 -> 1
  Step 5: 1 -> 2
  Step 7: 2 -> 3
  Step 9: 3 -> 4
  Step 11: 4 -> 5
  Step 14: 5 -> 6
  Step 16: 6 -> 7
  Step 18: 7 -> 8
  Step 21: 8 -> 9
  Step 24: 9 -> 10
```

---

## Experiment 3: Checkpoint Restore

### Test Checkpoint Workflow

```lua
-- experiment3_checkpoint.lua

print("=== Experiment 3: Checkpoint Restore ===")
print("")

-- List available checkpoints
local checkpoints = listCheckpoints()
print("Available checkpoints:")
for _, name in ipairs(checkpoints) do
    print("  - " .. name)
end

-- Get specific checkpoint
print("")
local cp = getCheckpoint("status_MIDPOINT")
if cp then
    print("Checkpoint 'status_MIDPOINT' contents:")
    for name, value in pairs(cp) do
        print(string.format("  %s = %s", name, tostring(value)))
    end
else
    print("Checkpoint not found")
end
```

### Expected Output

```
=== Experiment 3: Checkpoint Restore ===

Available checkpoints:
  - status_STARTING
  - status_MIDPOINT
  - status_NEAR_END
  - status_COMPLETE

Checkpoint 'status_MIDPOINT' contents:
  LV_COUNTER = 5
  LV_STATUS = MIDPOINT
  LV_AMOUNT = 500.00
  LV_NAME = Test
```

---

## Experiment 4: Force Replay

### Inject Saved State into New Debug Session

```lua
-- experiment4_replay.lua

print("=== Experiment 4: Force Replay ===")
print("")

-- Set breakpoint at a later line (inside loop)
setBreakpoint("ZTEST_PHASE5_TAS", 18)  -- Inside DO loop

print(">>> Execute ZTEST_PHASE5_TAS again <<<")
print("Waiting for breakpoint...")

local event = listen(60)
if not event then
    print("No debuggee")
    return
end

attach(event.debuggee_id)
print("Attached")

-- Show current state
local vars = getVariables()
print("")
print("Current state (natural execution):")
print("  LV_COUNTER = " .. tostring(vars.LV_COUNTER.value))
print("  LV_STATUS = " .. tostring(vars.LV_STATUS.value))

-- FORCE REPLAY: Inject checkpoint state
print("")
print(">>> Injecting 'status_MIDPOINT' checkpoint <<<")
injectCheckpoint("status_MIDPOINT")

-- Verify injection
vars = getVariables()
print("")
print("State after injection:")
print("  LV_COUNTER = " .. tostring(vars.LV_COUNTER.value))
print("  LV_STATUS = " .. tostring(vars.LV_STATUS.value))

-- Or inject specific variable
print("")
print(">>> Setting LV_COUNTER = 99 <<<")
setVariable("LV_COUNTER", "99")

vars = getVariables()
print("LV_COUNTER after setVariable: " .. tostring(vars.LV_COUNTER.value))

detach()
```

### Expected Output

```
=== Experiment 4: Force Replay ===

>>> Execute ZTEST_PHASE5_TAS again <<<
Waiting for breakpoint...
Attached

Current state (natural execution):
  LV_COUNTER = 1
  LV_STATUS = STARTING

>>> Injecting 'status_MIDPOINT' checkpoint <<<

State after injection:
  LV_COUNTER = 5
  LV_STATUS = MIDPOINT

>>> Setting LV_COUNTER = 99 <<<
LV_COUNTER after setVariable: 99
```

---

## Experiment 5: Compare Recordings

### Run Program Twice and Compare

```lua
-- experiment5_compare.lua

print("=== Experiment 5: Compare Recordings ===")
print("")

-- List all recordings
local recordings = listRecordings()
print("Available recordings:")
for i, rec in ipairs(recordings) do
    print(string.format("  [%d] %s - %s (%d steps)",
        i, rec.id, rec.program, rec.total_steps))
end

-- Compare first two if available
if #recordings >= 2 then
    print("")
    print("Comparing recordings...")
    local diff = compareRecordings(recordings[1].id, recordings[2].id)

    print("Comparison result:")
    print("  Steps compared: " .. diff.steps_compared)
    print("  Paths match: " .. tostring(diff.paths_match))

    if #diff.differences > 0 then
        print("  Differences:")
        for _, d in ipairs(diff.differences) do
            print("    - " .. d.type .. ": " .. d.description)
        end
    end
else
    print("Need at least 2 recordings to compare")
end
```

---

## Complete Experiment Script

Save as `examples/scripts/phase5-experiment.lua`:

```lua
#!/usr/bin/env lua
--[[
Phase 5 Complete Experiment
Run with: ./vsp lua examples/scripts/phase5-experiment.lua
]]

print("╔══════════════════════════════════════════════════════════════╗")
print("║     Phase 5: TAS-Style Debugging - Live Experiment           ║")
print("╚══════════════════════════════════════════════════════════════╝")
print("")

-- Check ADT connection
local function check_connection()
    local ok, result = pcall(function()
        return searchObject("ZTEST*", 1)
    end)
    return ok
end

if not check_connection() then
    print("ERROR: Cannot connect to SAP system")
    print("Please check SAP_URL, SAP_USER, SAP_PASSWORD environment variables")
    os.exit(1)
end

print("✓ SAP connection OK")
print("")

-- Menu
print("Select experiment:")
print("  1. Basic Recording (requires SE38 execution)")
print("  2. History Query (requires previous recording)")
print("  3. Checkpoint Workflow")
print("  4. Force Replay (requires SE38 execution)")
print("  5. Compare Recordings")
print("  6. Run All (automated where possible)")
print("")

io.write("Enter choice (1-6): ")
local choice = io.read("*l")

if choice == "1" then
    -- Run experiment 1
    dofile("examples/scripts/experiment1_record.lua")
elseif choice == "2" then
    dofile("examples/scripts/experiment2_query.lua")
elseif choice == "3" then
    dofile("examples/scripts/experiment3_checkpoint.lua")
elseif choice == "4" then
    dofile("examples/scripts/experiment4_replay.lua")
elseif choice == "5" then
    dofile("examples/scripts/experiment5_compare.lua")
elseif choice == "6" then
    print("Running automated tests...")
    -- Run non-interactive experiments
    dofile("examples/scripts/experiment3_checkpoint.lua")
else
    print("Invalid choice")
end

print("")
print("Experiment complete!")
```

---

## Running the Experiment

### Method 1: Interactive Lua REPL

```bash
./vsp lua

lua> setBreakpoint("ZTEST_PHASE5_TAS", 13)
lua> startRecording("test", "ZTEST_PHASE5_TAS")
lua> -- Execute program in SE38 --
lua> listen(60)
lua> attach("DEBUGGEE_xxx")
lua> stepOver()
lua> getVariables()
```

### Method 2: Script File

```bash
./vsp lua examples/scripts/phase5-experiment.lua
```

### Method 3: Unit Test Trigger

Instead of SE38, trigger via `RunUnitTests`:

```lua
-- Create class with test method, then:
runUnitTests("ZCL_TEST_TRIGGER")
-- This will hit breakpoints set on the test class
```

---

## Data Collected

After running experiments, you'll have:

| Artifact | Location | Content |
|----------|----------|---------|
| Recording JSON | `~/.vsp/recordings/*.json` | Full execution history |
| Checkpoints | In-memory | Named variable snapshots |
| Query results | Console output | History analysis |
| Comparison | Console output | Diff between runs |

### Sample Recording File

```bash
cat ~/.vsp/recordings/20251221-*.json | jq .
```

```json
{
  "id": "20251221-143215.123456789",
  "session_id": "experiment-001",
  "program": "ZTEST_PHASE5_TAS",
  "total_steps": 32,
  "frames": [
    {
      "step_number": 1,
      "location": {"program": "ZTEST_PHASE5_TAS", "line": 12},
      "variables": {
        "LV_COUNTER": {"value": 0, "type": "I"},
        "LV_STATUS": {"value": "INIT", "type": "STRING"}
      }
    }
  ],
  "checkpoints": {
    "status_STARTING": 1,
    "status_MIDPOINT": 12,
    "status_NEAR_END": 20,
    "status_COMPLETE": 28
  }
}
```

---

## Success Criteria

| Test | Pass Criteria |
|------|---------------|
| Recording | `total_steps > 0`, `is_complete = true` |
| Query | `findWhenChanged` returns correct step |
| Checkpoint | `getCheckpoint` returns saved variables |
| Force Replay | `setVariable` modifies live session |
| Compare | Detects differences between runs |

---

## Troubleshooting

### "No debuggee attached"

- Ensure ZTEST_PHASE5_TAS exists
- Execute program AFTER setting breakpoint
- Check breakpoint is on executable line (not comment/declaration)

### "Connection refused"

- Verify SAP_URL is correct
- Check network connectivity
- Ensure SAP system is running

### "Recording empty"

- Must be in active debug session
- Variables captured only after `getVariables()` call
- Check debug listener is attached

---

## Related Documents

- [Phase 5 Data Extraction Examples](2025-12-21-006-phase5-data-extraction-examples.md)
- [Phase 5 Testing Methodology](2025-12-21-005-phase5-testing-methodology.md)
- [Debugger Experiment Session](2025-12-05-session-debugger-experiment.md)
