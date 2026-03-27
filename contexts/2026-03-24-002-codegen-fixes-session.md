# WASM-to-ABAP Codegen Fixes — Session Report

**Date:** 2026-03-24
**Report ID:** 002
**Subject:** IF/ELSE stack depth fix, block DO restoration, dead code strategy, QuickJS GENERATE progress

---

## What Was Achieved

### Bug Fixes (verified on both SAP systems)

#### 1. IF/ELSE Stack Depth Bug (Go + ABAP)
**Root cause:** The codegen didn't reset `stack_depth` at `else`. The else branch wrote results to a different stack slot (s1) than the true branch (s0), causing `rv` to read the wrong value.

**Fix (Go):** Changed `blockStack` from `[]blockKind` to `[]blockEntry` with `savedDepth`. At `else`, reset `stack.depth = entry.savedDepth`.

**Fix (ABAP):** Added `mt_block_depths` parallel table. Save depth at `if`/`block`/`loop`, restore at `else`, pop at `end`.

**Impact:** 4 tests fixed on a4h-110: abs, fibonacci, gcd, max_min.

#### 2. Block Without DO Wrapper (a4h-105 only)
**Root cause:** a4h-105 codegen eliminated `DO 1 TIMES` for blocks. When `br_if N` (N>0) set `gv_br = N; EXIT`, the EXIT had no enclosing DO and exited the FORM entirely, losing rv.

**Fix:** Restored `DO 1 TIMES` / `ENDDO` for blocks, with `IF gv_br > 0. gv_br = gv_br - 1. EXIT. ENDIF.` propagation.

**Impact:** 4 tests fixed on a4h-105: collatz, is_prime, pow, sum_to.

#### 3. PERFORM USING Literal Mutation (a4h-105 test harness)
**Root cause:** FORM parameters without VALUE() receive references. When WASM functions mutate parameters (e.g., collatz: `n = n/2`), passing literals causes MOVE_TO_LIT_NOTALLOWED_NODATA.

**Fix:** Test harness uses `call1`/`call2`/`call3` wrapper methods that copy IMPORTING params to local variables before PERFORM.

#### 4. Dead Code Elimination Strategy
**Root cause:** `GENERATE SUBROUTINE POOL` rejects "Statement is not accessible" after `RETURN.` Dead code elimination was added but caused structural mismatches (ELSE without IF) in complex nested patterns.

**Failed approaches:**
- Track `mv_dead_depth` and resume at `end`/`else` — breaks when RETURN is inside nested blocks within IF
- Only set unreachable after RETURN (not br) — still had nesting issues
- Let end/else fall through to live handler — broke is_prime

**Working approach:** `IF 1 = 1. RETURN. ENDIF.` — wraps RETURN so GENERATE doesn't see code "after RETURN". No dead code elimination needed.

### Test Results

| System | Suite | QuickJS GENERATE |
|--------|-------|-----------------|
| Go (wazero) | 30/30 PASS | N/A (instantiation OK) |
| a4h-110 | 11/11 PASS | Not tested |
| a4h-105 | 11/11 PASS | rc=4, line 207,664: ENDDO without DO |

### QuickJS GENERATE Progress

| Attempt | Lines | Error | Fix Applied |
|---------|-------|-------|-------------|
| 1 | 340,495 | line 286,428: ELSE without IF | Dead code else handling |
| 2 | 293,741 | line 240,326: ELSE without IF | Removed br→unreachable |
| 3 | 293,741 | line 240,326: ELSE without IF (same) | Nested end/else fallthrough |
| 4 | 292,941 | line 207,664: ENDDO without DO | **Current blocker** |

### Wazero Execution Tests (new)
Added `execute_test.go` with `github.com/tetratelabs/wazero`:
- 30 test cases across 12 functions executed via real WASM runtime
- QuickJS 1.2MB WASM instantiates with WASI successfully
- Proves test expectations are correct (ground truth)

---

## Current State of ABAP Codegen on a4h-105

### zcl_wasm_codegen changes (surgical edits):
1. `mt_block_depths` — IF/ELSE stack depth tracking
2. `DO 1 TIMES` for blocks — restored
3. `IF 1 = 1. RETURN. ENDIF.` — no dead code elimination
4. `else` in dead code at depth 0 resumes live (vestigial, now unused)
5. No `mv_unreachable` after `br` — only after return/unreachable

### Known Issues
- **Parse error:** func 1 has CX_SY_ARITHMETIC_OVERFLOW at pos 29351 (1 of 1410 functions)
- **ENDDO without DO:** line 207,664 in QuickJS GENERATE — needs investigation
- **Code size:** 293K lines without dead code elimination (was 340K with DO blocks)

---

## QuickJS GENERATE Deep Debugging (continued)

### Additional fixes applied to ABAP codegen on a4h-105:
5. **br_table** (opcode 14) — 555 occurrences unhandled. Added: pop index + branch default label
6. **call_indirect** (opcode 17) — 1,784 occurrences unhandled! Added: pop index + args, push result stub
7. **gv_br propagation** — changed `IF gv_br > 0. ... EXIT. ENDIF.` to `CASE gv_br. WHEN 0. WHEN OTHERS. ... EXIT. ENDCASE.` to avoid ELSE conflict
8. **Line split** — statement-aware: split only at `.` boundaries, not mid-statement
9. **Body discard** — flush packer before checking block stack corruption

### GENERATE Error Progress
| Attempt | Lines | Error | Root Cause |
|---------|-------|-------|------------|
| 1 | 340K | ELSE without IF | Dead code elimination |
| 4 | 293K | ENDDO without DO | Packer leak in discarded function |
| 5 | 293K | ELSE without IF | gv_br IF/ENDIF conflicts with WASM ELSE |
| 8 | 282K | ELSE without IF | br_table + call_indirect stack corruption |
| 9 | 282K | ELSE without IF (same) | **Unknown — needs manual ABAP inspection** |

### Remaining Blocker
GENERATE rc=4 at line 229,175: "No open IF statement exists" (ELSE).
- Not caused by: line packing, 255-char split, gv_br IF, br_table, call_indirect, dead code
- Tested with packing disabled (1.56M lines) — same error
- Tested with no line split — same error
- All self-closing IFs verified balanced
- Need: dump ABAP for the function at line 229K and manually inspect IF/ELSE/ENDIF nesting

## Next Steps

1. **Dump function at line 229K** — find which function, extract its ABAP, manually count IF/ELSE/ENDIF
2. **ROOT CAUSE FOUND: cross-nesting** — WASM allows `block; if; end_block; else; end_if` but ABAP rejects `DO. IF. ENDDO. ELSE. ENDIF.` as invalid nesting. Blocks can't use DO/ENDDO when they span IF/ELSE boundaries.
3. **Fix: FORM-per-block architecture** (next session)
   - Each WASM `block` → `PERFORM block_N.` with body in separate FORM
   - `br 0` → `RETURN.` (exits block FORM), `br N` → `gv_br = N. RETURN.`
   - No cross-nesting possible (FORM is isolated scope)
   - Requires global variables (move DATA to PROGRAM level)
   - Dead code elimination (93K→~80K lines) works correctly and should be kept
   - Tried and rejected: blocks without DO (loses nesting depth), dead-end-no-emit (unclosed DO)

---

## Systems State

| System | Status | What's deployed |
|--------|--------|-----------------|
| a4h-105 | OK | Updated codegen (all fixes), test suite 11/11, ZQJS_TEST_RUN report |
| a4h-110 | OK | Updated codegen (IF/ELSE fix only), test suite 11/11 |
| Local | OK | Go codegen fix + wazero tests committed (50ebc3b), pushed |
| GitHub | OK | Up to date |
