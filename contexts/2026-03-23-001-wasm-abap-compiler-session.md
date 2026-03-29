# WASM-to-ABAP Compiler — Session Report & Next Session Seed

**Date:** 2026-03-23
**Report ID:** 001
**Subject:** Native ABAP WASM compiler: from zero to `factorial(10) = 3,628,800` on SAP

---

## What Was Built

### Deliverables

| Component | Location | LOC |
|-----------|----------|----:|
| Go test WASM generator | `pkg/wasmcomp/generate_test.go` | 474 |
| 12 test .wasm binaries | `pkg/wasmcomp/testdata/*.wasm` | — |
| QuickJS WASM | `pkg/wasmcomp/testdata/quickjs_eval.wasm` | 1.2MB |
| ABAP WASM reader | `embedded/abap/wasm_compiler/zcl_wasm_reader.clas.abap` | 112 |
| ABAP WASM module parser | `embedded/abap/wasm_compiler/zcl_wasm_module.clas.abap` | 350 |
| ABAP WASM codegen | `embedded/abap/wasm_compiler/zcl_wasm_codegen.clas.abap` | 650+ |
| Interactive report | `embedded/abap/wasm_compiler/zwasm_compiler.prog.abap` | 290 |
| Unit tests | `embedded/abap/wasm_compiler/zcl_wasm_utest.clas.testclasses.abap` | 170 |
| Makefile release target | `Makefile` | 24 |

### Test WASM Binaries (12 functions)
add, factorial, fibonacci, gcd, is_prime, abs, max, min, negate, sum_to, collatz, pow2

### Unit Test Results on SAP (a4h-105-adt)

| Test | Time | Status |
|------|------|--------|
| test_parse_add | 0s | PASS |
| test_compile_add | 0s | PASS |
| test_execute_add (2+3=5) | 0s | PASS |
| test_execute_factorial (10!=3628800) | 0s | PASS |
| test_parse_quickjs (1.2MB) | 0.3s | PASS |
| test_compile_quickjs (244K lines) | 8s | PASS |
| test_generate_quickjs | 8s | FAIL (nesting) |

---

## Architecture

```
┌──────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  .wasm file  │────▶│ zcl_wasm_reader  │────▶│ zcl_wasm_module │
│  (binary)    │     │ (LEB128 cursor)  │     │ (section parser)│
└──────────────┘     └──────────────────┘     └────────┬────────┘
                                                       │
                                              ┌────────▼────────┐
                                              │ zcl_wasm_codegen│
                                              │ (SSA compiler)  │
                                              └────────┬────────┘
                                                       │
                     ┌─────────────────┐     ┌────────▼────────┐
                     │ GENERATE        │◀────│ ABAP source     │
                     │ SUBROUTINE POOL │     │ (FORM-based)    │
                     └────────┬────────┘     └─────────────────┘
                              │
                     ┌────────▼────────┐
                     │ PERFORM ADD     │
                     │ USING 2 3       │
                     │ CHANGING result │──▶ 5
                     └─────────────────┘
```

### Codegen Strategy (SSA Virtual Stack)

```abap
" WASM bytecodes:     ABAP output:
" local.get 0    →    lv_s0 = p0.
" local.get 1    →    lv_s1 = p1.
" i32.add        →    lv_s0 = lv_s0 + lv_s1.
" (implicit ret) →    rv = lv_s0.
```

- Compile-time stack: `push()` returns `lv_sN`, `pop()` returns top
- Only declares stack vars actually used (`mv_max_stack`)
- All types unified to `int8` (covers i32 + i64, avoids PERFORM type mismatches)
- `FORM WASM_INIT` for lazy initialization (memory, globals, data segments)

---

## Lessons Learned (ABAP Gotchas)

### 1. XSTRING Write Position
```abap
" WRONG: variable offset on xstring write side
gv_mem+iv_addr(4) = lv_val.  " → compile error

" CORRECT: use REPLACE SECTION
REPLACE SECTION OFFSET iv_addr LENGTH 4 OF gv_mem WITH lv_val IN BYTE MODE.
```

### 2. GENERATE SUBROUTINE POOL Restrictions
- Max line length: 255 chars (use `line()` auto-splitter)
- No `RAISE EXCEPTION TYPE` → use `MESSAGE ... TYPE 'X'`
- No class references → pure procedural ABAP only
- FORM names must be UPPERCASE for dynamic PERFORM
- `USING` keyword only once per PERFORM/FORM
- `USING VALUE(p0)` needed if param is modified inside FORM

### 3. String Template Formatting
```abap
" WRONG: defaults to LEFT alignment → "100" not "001"
|{ 1 WIDTH = 3 PAD = '0' }|

" CORRECT:
|{ 1 WIDTH = 3 PAD = '0' ALIGN = RIGHT }|
```

### 4. DATA Before Executable Statements
In includes, all `DATA` declarations must come before any executable statements.
Split into TOP include (declarations) + FORM WASM_INIT (initialization).

### 5. LEB128 Overflow
```abap
" WRONG: ipow returns TYPE i, multiplication overflows
lv_result = lv_result + lv_masked * ipow( base = 2 exp = lv_shift ).

" CORRECT: cast operands to int8 first
DATA(lv_masked) = CONV int8( lv_b MOD 128 ).
DATA(lv_pw) = CONV int8( ipow( base = 2 exp = lv_shift ) ).
lv_result = lv_result + lv_masked * lv_pw.

" For i64 (shift > 30): ipow itself overflows → use manual loop
lv_pw = 1. DO lv_shift TIMES. lv_pw = lv_pw * 2. ENDDO.
```

### 6. Memory Init Performance
```abap
" WRONG: O(n) — 1.3 million concatenations
DO 1376255 TIMES. CONCATENATE gv_mem lv_z INTO gv_mem IN BYTE MODE. ENDDO.

" CORRECT: O(log n) — 37 operations via doubling
lv_pg = '00'.
DO 16 TIMES. CONCATENATE lv_pg lv_pg INTO lv_pg IN BYTE MODE. ENDDO.  " → 64KB
DO 21 TIMES. CONCATENATE gv_mem lv_pg INTO gv_mem IN BYTE MODE. ENDDO. " → 1.3MB
```

### 7. SMW0 Reading
```abap
" Function module varies by system. WWWDATA_IMPORT works everywhere:
ls_key-relid = 'MI'. ls_key-objid = 'ZQJS.WASM'.
CALL FUNCTION 'WWWDATA_IMPORT' EXPORTING key = ls_key TABLES mime = lt_mime.
```

### 8. Split to INCLUDEs
- INSERT REPORT all includes FIRST (with `PROGRAM TYPE 'I'`)
- INSERT REPORT main program LAST (with `PROGRAM TYPE 'S'`)
- GENERATE REPORT only on main program (compiles all includes)

### 9. Bitwise Operations
```abap
" No CONV x4() — x4 is not a type!
" Define custom type:
TYPES: ty_x4 TYPE x LENGTH 4.
DATA: gv_xa TYPE ty_x4, gv_xb TYPE ty_x4, gv_xr TYPE ty_x4.
gv_xa = lv_a. gv_xb = lv_b. gv_xr = gv_xa BIT-AND gv_xb. lv_r = gv_xr.
```

### 10. PERFORM Expressions
```abap
" WRONG: expressions not allowed in PERFORM USING
PERFORM mem_ld_i32 USING lv_s0 + 96 CHANGING lv_s1.

" CORRECT: pre-calculate
lv_s1 = lv_s0 + 96. PERFORM mem_ld_i32 USING lv_s1 CHANGING lv_s1.
```

---

## Systems Configuration

| System | What's there |
|--------|-------------|
| a4h-105-adt | WASM compiler ($TMP) + ZADT_VSP ($ZADT_VSP) — **primary** |
| a4h-110-adt | WASM compiler ($ZOZIK) — older, no ZADT_VSP |
| SMW0 (both) | ZEXT.WASM, ZQJS.WASM |

---

## Next Session Seed

### Priority 1: Line Packing (easy, high impact)
Current: one ABAP statement per line → 244K lines for QuickJS.
Target: join statements on one line up to 250 chars → ~60K lines.

```abap
" Before (4 lines):
lv_s0 = p0.
lv_s1 = p1.
lv_s0 = lv_s0 + lv_s1.
rv = lv_s0.

" After (1 line):
lv_s0 = p0. lv_s1 = p1. lv_s0 = lv_s0 + lv_s1. rv = lv_s0.
```

Implementation: add `packing` mode to `line()` method. Accumulate statements in a buffer.
Flush at 250 chars or on control flow statements (IF/DO/ENDDO/ENDFORM).

### Priority 2: Fix LEB128 Parse Overflow
Current: 217K/453K instructions parsed. ~half of QuickJS functions have empty bodies.
Root cause: still an overflow somewhere in `read_u32` or `read_i32` for edge case values.
Approach: add more detailed TRY/CATCH in `parse_instructions` to log the exact opcode + position that fails. Then fix the specific arithmetic.

### Priority 3: Close Unclosed Blocks
When `parse_instructions` fails mid-function, open DO blocks aren't closed → ENDFORM before ENDDO.
Fix: after `emit_instructions`, check `mt_block_kinds` and emit missing ENDDO/ENDIF.

```abap
" After emit_instructions:
WHILE lines( mt_block_kinds ) > 0.
  DATA(lv_k) = mt_block_kinds[ lines( mt_block_kinds ) ].
  DELETE mt_block_kinds INDEX lines( mt_block_kinds ).
  CASE lv_k.
    WHEN c_block OR c_loop. line( |ENDDO.| ).
    WHEN c_if. line( |ENDIF.| ).
  ENDCASE.
ENDWHILE.
```

### Priority 4: WASI Stub FORMs
QuickJS requires 9 WASI imports. Generate stub FORMs:

```abap
" Minimal WASI stubs for QuickJS
FORM F0 USING VALUE(p0) TYPE int8 ... " fd_write → return 0
  rv = 0.
ENDFORM.
FORM F8 USING VALUE(p0) TYPE int8.    " proc_exit → RETURN
  RETURN.
ENDFORM.
```

Imports are indexed 0-8, before user functions. The codegen already names them F0..F8.
Just need to generate non-empty bodies instead of empty FORMs.

### Priority 5: Execute QuickJS
Once parse + generate work:
```abap
PERFORM WASM_INIT IN PROGRAM (lv_prog).
PERFORM _START IN PROGRAM (lv_prog).
" → JavaScript embedded in WASM binary executes on SAP
```

### Priority 6: Class Wrapper Polish
- Static wrapper: PROGRAM + local class, `PERFORM ... IN PROGRAM`
- Constructor calls WASM_INIT
- Deploy both via ZWASM_COMPILER report

### Stretch: Go Compiler Backend
The Go compiler (`pkg/wasmcomp/codegen.go`) already handles 100% of QuickJS opcodes.
Consider: generate ABAP via Go compiler instead of ABAP compiler for production use.
ABAP compiler → self-hosting demo. Go compiler → production quality.

---

## Git Commits This Session

```
efdc744 feat: add `make release` and `make refresh-deps` targets
480537b feat: WASM test binaries, ABAP codegen implementation, interactive compiler report
9c6d27c feat: QuickJS WASM binary, persistent program generation, class wrapper
64b9708 feat: smart DATA declarations, USING VALUE(), split to INCLUDEs
3ee62c1 fix: QuickJS compilation — xstring write, include naming, graceful parse
01c5d48 fix: ABAP codegen — xstring types, FORM WASM_INIT, DATA/init separation
0a05d2e fix: ABAP codegen — ty_x4 type for bitwise ops, no CONV x4
9bff122 feat: QuickJS GENERATE progress — uniform int8, void return, MESSAGE X
```
