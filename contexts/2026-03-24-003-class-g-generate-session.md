# CLASS g Architecture + QuickJS GENERATE rc=0 — Session Report

**Date:** 2026-03-24
**Report ID:** 003
**Subject:** Block-as-CLASS-METHOD codegen, QuickJS GENERATE success, WASI I/O

---

## What Was Achieved

### 1. Go Codegen: Block-as-CLASS-METHOD Architecture
- WASM blocks/loops → CLASS-METHODS on shared `CLASS g` (parameterless)
- WASM ifs → inline `DO 1 TIMES. IF cond.` (safe, no cross-nesting)
- Shared state: CLASS-DATA (params, locals, stack, br, rv)
- Inside methods: direct access (`s0`, `br`). From FORMs: `g=>s0`, `g=>br`
- 1-based br: `br N` → `br = N+1`, propagation via decrement+check
- GI include: CLASS g DEFINITION + IMPLEMENTATION (12K+ methods for QuickJS)
- All 40+ Go tests pass

### 2. ABAP Codegen: Deployed to SAP a4h-105
- Rewrote `zcl_wasm_codegen` with CLASS g architecture
- 7 iterative fixes to get QuickJS through GENERATE:

| Line | Error | Fix |
|------|-------|-----|
| 2,130 | Field "L5" unknown | Locals from 0, not from maxParams |
| 2,130 | CLASS g IMPL missing | `emit_raw_line` for METHOD/ENDMETHOD (packer reordering) |
| 15,894 | ELSE without IF | Guard ELSE on mt_block_kinds |
| 16,119 | Field "L22397" | Scan instruction local_idx for max |
| 21,898 | Unclosed IF in METHOD | Close unclosed ifs at body end |
| 206,088 | Unclosed IF in FORM | Close unclosed ifs at FORM end |
| 218,891 | **rc=0** | All fixes combined |

### 3. QuickJS on SAP
- **GENERATE SUBROUTINE POOL rc=0** — 218,891 lines, 22s compile time
- **WASM_INIT + _START run** without exception
- **WASI fd_write/fd_read** patched (source injection via SET_STDIN/GET_STDOUT)
- **stdout empty** — QuickJS can't eval JS because call_indirect is stubbed in ABAP codegen

### 4. Key Bugs Found & Fixed

#### Packer Reordering
`line(|DO 1 TIMES.|)` goes through the packer which can reorder it relative to `emit_raw_line(|IF cond.|)`. Fix: use `emit_raw_line` for DO, METHOD, ENDMETHOD.

#### Pack Buffer Leak
Block body extraction saves/restores `mv_out` but NOT `mv_pack_buf`. Packed content from the parent leaks into block bodies. Fix: save/restore `mv_pack_buf` and `mv_pack_indent`.

#### Dead Code Handler Removed
The `mv_unreachable`/`mv_dead_depth` state machine has irreconcilable bugs with block-as-method architecture. Fix: remove entirely, wrap br/return in `IF 1 = 1. ... ENDIF.` so GENERATE accepts code after them.

#### Unclosed IFs at Boundary
WASM parser may exclude an if's `end` from the block body (last instruction coincides with block boundary). Fix: after emitting block body, close any remaining ifs in `mt_block_kinds`.

---

## Architecture Decisions

### Why CLASS g, not CHANGING params?
QuickJS function 0 has 1,600+ stack vars. CHANGING params would be 17K chars per FORM signature. CLASS-DATA with `g=>` prefix avoids parameter limits entirely.

### Why not dead code elimination?
GENERATE rejects "statement not accessible" after RETURN/EXIT. Dead code elimination removes ELSE/ENDIF causing structural breaks. `IF 1 = 1. RETURN. ENDIF.` tricks GENERATE into accepting code after it. Zero state tracking needed.

### Why Go compiler is the path forward?
- 120+ opcodes vs ABAP's ~30
- call_indirect with trampoline dispatch
- Full i64, f64, WASI support
- 40+ verified tests
- ABAP codegen is a subset for quick testing only

---

## Next Session Seed

### Goal: JavaScript eval on SAP via Go-compiled QuickJS

1. **CompileSingleProgram** — `func CompileSingleProgram(mod *Module, name string) string`
   - Concatenates: PROGRAM + DATA + CLASS g (DEF+IMPL) + RT FORMs + INIT + function FORMs
   - Includes `gv_stdout`/`gv_stdin` globals + SET_STDIN/GET_STDOUT helper FORMs
   - Updates fd_write to concatenate to `gv_stdout`, fd_read to read from `gv_stdin`

2. **Deploy zcl_wasm_rt** — 306-line runtime class with 53 methods (bitwise, i64, unsigned ops)
   - Must exist on SAP before the generated program runs
   - Deploy via `SAP(action="create/edit", target="CLAS ZCL_WASM_RT")`

3. **Deploy & test simple suite** — add/factorial/fibonacci via Go codegen → SAP
   - Verify execution matches wazero results

4. **QuickJS eval** — `console.log(2+2)` → stdout = "4\n"
   - Go codegen already has call_indirect trampoline
   - WASI fd_write needs to concatenate bytes to `gv_stdout`
   - WASI fd_read needs to return `gv_stdin` bytes
   - Deploy via GENERATE SUBROUTINE POOL

### Key files
- `pkg/wasmcomp/backend.go` — add CompileSingleProgram
- `pkg/wasmcomp/codegen.go` — update emitWASICall for fd_write/fd_read
- `pkg/wasmcomp/testdata/multi/zcl_wasm_rt.clas.abap` — runtime to deploy
- `embedded/abap/wasm_compiler/zcl_wasm_codegen_v2.clas.abap` — ABAP codegen (reference)

### zcl_wasm_rt methods (53 total)
Bitwise: and32/64, or32/64, xor32/64, shl32/64, shr_s32/64, shr_u32/64, rotl32/64, rotr32/64
Counting: clz32/64, ctz32/64, popcnt32/64
Unsigned: div_u32/64, rem_u32/64, lt/gt/le/ge_u32/64
Memory: mem_ld/st_i64, mem_ld/st_f32/f64, mem_copy, mem_fill
Convert: wrap_i64, extend_u32/u64_f, extend8s/16s/32s, trunc_f_u32/u64
Reinterpret: f32↔i32, f64↔i64
