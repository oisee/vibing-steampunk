# WASM-to-ABAP AOT Compiler — Design Report

**Date:** 2026-03-19
**Report ID:** 003
**Subject:** Ahead-of-time compilation of WebAssembly bytecode to native ABAP source code
**Related:** [abap-wasm](https://github.com/abap-wasm/abap-wasm) (Lars Hvam's WASM interpreter), [MinZ compiler](https://github.com/oisee/minz) (WASM→MIR2 pipeline), [abaplint](https://github.com/abaplint/abaplint) (ABAP parser)
**Branch:** `feat/wasm-abap`

---

## 1. Motivation

We want to run the [abaplint](https://github.com/abaplint/abaplint) ABAP parser **inside SAP systems** — on ECC, S/4HANA, BTP. abaplint is TypeScript, compiled to WASM via Javy/QuickJS (~14MB .wasm blob, ~1000 functions).

**Existing approaches:**
- **Lars Hvam's abap-wasm** — WASM interpreter in ABAP. 442 instruction classes, 17,508 spec tests passing. But it's an **interpreter**: every opcode = dynamic class dispatch, every value = heap-allocated object. QuickJS runs but slowly.
- **vsp's current approach** — abaplint runs in Go (via wazero, pure Go WASM runtime). Works great for vsp, but requires Go. Can't run on SAP directly.

**Our approach:** AOT (ahead-of-time) compiler. Translate `.wasm` → `.abap` **once**, deploy the generated ABAP code to SAP. Native ABAP speed, zero interpreter overhead.

---

## 2. Architecture

```
                    Compile Time (Go, runs on developer machine)
┌─────────┐     ┌──────────────┐     ┌──────────────┐     ┌─────────────┐
│  .wasm  │────▶│ WASM Parser  │────▶│  SSA Builder │────▶│ ABAP Codegen│
│  binary │     │ (sections,   │     │ (stack→vars, │     │ (emit CLASS │
│         │     │  functions,  │     │  structured  │     │  + METHODs) │
│         │     │  bytecode)   │     │  control)    │     │             │
└─────────┘     └──────────────┘     └──────────────┘     └──────┬──────┘
                                                                  │
                                                           ┌──────▼──────┐
                                                           │  .abap      │
                                                           │  source     │
                                                           │  files      │
                                                           └──────┬──────┘
                                                                  │
                    Runtime (SAP system)                          │
                                                           ┌──────▼──────┐
                                                           │ ZCL_WASM_*  │
                                                           │ (generated) │
                                                           │ + runtime   │
                                                           │   library   │
                                                           └─────────────┘
```

### Compiler (Go)

Written in Go, lives in `pkg/wasmcomp/`. Takes a `.wasm` binary, produces `.clas.abap` files.

**Pipeline:**
1. **Parse** — read WASM binary sections (types, imports, functions, memory, globals, exports, data, elements)
2. **Build SSA** — convert stack machine to SSA form (stack slots → named variables)
3. **Emit ABAP** — generate class definition + method implementations

### Generated Code Structure

Each WASM module → one ABAP class (or a set of classes if too large for ABAP limits):

```abap
CLASS zcl_wasm_quickjs DEFINITION PUBLIC CREATE PUBLIC.
  PUBLIC SECTION.
    " Exported functions become public methods
    METHODS eval
      IMPORTING iv_code TYPE string
      RETURNING VALUE(rv_result) TYPE string.

    METHODS instantiate.  " runs data/element init + start function

  PRIVATE SECTION.
    " Linear memory
    DATA mv_memory TYPE xstring.
    DATA mv_memory_pages TYPE i.

    " Globals
    DATA mv_g0 TYPE i.   " __stack_pointer
    DATA mv_g1 TYPE i.   " etc.

    " Function table (for call_indirect)
    DATA mt_table0 TYPE STANDARD TABLE OF i WITH DEFAULT KEY.

    " ~1000 internal functions
    METHODS func_000 IMPORTING p0 TYPE i RETURNING VALUE(rv) TYPE i.
    METHODS func_001 IMPORTING p0 TYPE i p1 TYPE i.
    " ...
ENDCLASS.
```

### Mapping Rules

| WASM Concept | ABAP Equivalent |
|-------------|-----------------|
| **Function** | Method on the class |
| **Local variables** | `DATA` in method (typed: `TYPE i` for i32, `TYPE int8` for i64, `TYPE f` for f64) |
| **Value stack** | Unrolled into local variables (SSA style: `s0`, `s1`, `s2`...) |
| **Linear memory** | `XSTRING` attribute (byte-addressable, little-endian) |
| **Memory page** | 65536 bytes; `memory.grow` extends the xstring |
| **Globals** | Class attributes (`mv_g0`, `mv_g1`, ...) |
| **Function table** | `TABLE OF i` (function indices); `call_indirect` → CASE dispatch |
| **Data segments** | Initialization code in `instantiate()` |
| **Imports (WASI)** | Calls to a runtime helper class |
| **i32** | `TYPE i` (ABAP signed 32-bit) |
| **i64** | `TYPE int8` (ABAP signed 64-bit, available since 7.40) |
| **f32/f64** | `TYPE f` (ABAP IEEE 754 64-bit double) |
| **Unsigned ops** | Promote to INT8, mask, or helper methods |

---

## 3. Stack-to-SSA Conversion

The core of the compiler. WASM is a stack machine; ABAP needs named variables.

### Example

**WASM bytecode:**
```wasm
(func $add_clamped (param $a i32) (param $b i32) (result i32)
  local.get $a        ;; stack: [a]
  local.get $b        ;; stack: [a, b]
  i32.add             ;; stack: [a+b]
  local.tee $a        ;; stack: [a+b], local $a = a+b
  i32.const 255       ;; stack: [a+b, 255]
  i32.gt_u            ;; stack: [a+b > 255]
  if (result i32)
    i32.const 255     ;; stack: [255]
  else
    local.get $a      ;; stack: [a+b]
  end
)
```

**Generated ABAP:**
```abap
METHOD func_042.  " $add_clamped
  " IMPORTING p0 TYPE i  (param $a)
  "           p1 TYPE i  (param $b)
  " RETURNING VALUE(rv) TYPE i
  DATA l0 TYPE i.  " local $a (shadow of param)
  DATA s0 TYPE i.  " stack variable

  l0 = p0.  " copy param to mutable local

  s0 = l0 + p1.                           " i32.add
  l0 = s0.                                 " local.tee
  IF zcl_wasm_rt=>gt_u32( s0, 255 ).       " i32.gt_u
    rv = 255.                               " i32.const 255
  ELSE.
    rv = l0.                                " local.get $a
  ENDIF.
ENDMETHOD.
```

### SSA Algorithm

1. Walk instructions sequentially
2. Maintain a virtual stack (list of variable names)
3. Each instruction that pushes → assign to next `sN` variable, push name
4. Each instruction that pops → pop names from virtual stack
5. `block`/`loop`/`if`/`else`/`end` → map to ABAP `IF`/`WHILE`/control flow
6. `br`/`br_if` → use flags + `EXIT` from DO blocks (ABAP's closest to structured branch)

### Control Flow Mapping

WASM's structured control flow maps cleanly to ABAP:

| WASM | ABAP |
|------|------|
| `block...end` | `DO 1 TIMES. ... ENDDO.` (EXIT = br) |
| `loop...end` | `DO. ... IF <exit_cond>. EXIT. ENDIF. ENDDO.` (CONTINUE = br) |
| `if...else...end` | `IF ... ELSE ... ENDIF.` |
| `br N` | `EXIT.` (from Nth enclosing DO) — need flag variables for N>0 |
| `br_if N` | `IF cond. EXIT. ENDIF.` |
| `br_table` | `CASE index. WHEN 0. EXIT. WHEN 1. ... ENDCASE.` |
| `return` | Direct `rv = ... RETURN.` |

**The hard part:** `br N` where N > 0 (branch to outer block). ABAP's `EXIT` only exits the innermost loop. Solution: use a flag variable `lv_br_depth` and check after each inner block:

```abap
DO 1 TIMES.  " block L0
  DO 1 TIMES.  " block L1
    IF some_condition.
      lv_br_depth = 1.  " br 1 = exit L0
      EXIT.              " exit L1
    ENDIF.
  ENDDO.
  IF lv_br_depth > 0.
    lv_br_depth = lv_br_depth - 1.
    EXIT.  " exit L0
  ENDIF.
ENDDO.
```

---

## 4. Memory Operations

### Linear Memory (XSTRING)

```abap
" i32.load: read 4 bytes little-endian from memory at offset
METHOD mem_load_i32.
  DATA lv_bytes TYPE x LENGTH 4.
  lv_bytes = mv_memory+iv_addr(4).
  " Reverse for little-endian → big-endian (ABAP)
  rv_value = zcl_wasm_rt=>reverse_i32( lv_bytes ).
ENDMETHOD.

" i32.store: write 4 bytes little-endian to memory
METHOD mem_store_i32.
  DATA lv_bytes TYPE x LENGTH 4.
  lv_bytes = zcl_wasm_rt=>to_le_i32( iv_value ).
  mv_memory+iv_addr(4) = lv_bytes.
ENDMETHOD.

" memory.grow: extend by N pages (64KB each)
METHOD mem_grow.
  DATA lv_old_pages TYPE i.
  lv_old_pages = mv_memory_pages.
  DATA(lv_new_size) = ( mv_memory_pages + iv_pages ) * 65536.
  " Extend xstring
  DATA lv_zeros TYPE xstring.
  lv_zeros = repeat( val = '00' occ = iv_pages * 65536 ).
  CONCATENATE mv_memory lv_zeros INTO mv_memory IN BYTE MODE.
  mv_memory_pages = mv_memory_pages + iv_pages.
  rv_old_pages = lv_old_pages.
ENDMETHOD.
```

### Alternative: Page Table (if XSTRING performance is an issue)

```abap
TYPES: BEGIN OF ty_page,
         data TYPE x LENGTH 65536,
       END OF ty_page.
DATA mt_pages TYPE STANDARD TABLE OF ty_page.

" Access: page = offset DIV 65536, off_in_page = offset MOD 65536
```

Both approaches will be benchmarked. XSTRING is simpler to generate code for.

---

## 5. Unsigned Integer Handling

ABAP has no unsigned integers. WASM uses unsigned heavily.

### Strategy: i32 in TYPE i (signed 32-bit)

For most i32 operations, signed and unsigned produce the same bit pattern:
- `i32.add`, `i32.sub`, `i32.mul` — identical for signed/unsigned (2's complement)
- `i32.and`, `i32.or`, `i32.xor` — bitwise, no sign issue
- `i32.shl` — shift left, same regardless

Only these need special handling:
- `i32.div_u`, `i32.rem_u` — unsigned division/remainder
- `i32.shr_u` — logical shift right (vs arithmetic)
- `i32.lt_u`, `i32.le_u`, `i32.gt_u`, `i32.ge_u` — unsigned comparison
- `i32.load8_u`, `i32.load16_u` — zero-extend (vs sign-extend)

### Runtime Helper Class

```abap
CLASS zcl_wasm_rt DEFINITION PUBLIC.
  PUBLIC SECTION.
    " Unsigned comparisons (promote to INT8 for range)
    CLASS-METHODS gt_u32
      IMPORTING iv_a TYPE i iv_b TYPE i
      RETURNING VALUE(rv) TYPE abap_bool.

    " Unsigned division
    CLASS-METHODS div_u32
      IMPORTING iv_a TYPE i iv_b TYPE i
      RETURNING VALUE(rv) TYPE i.

    " Shifts
    CLASS-METHODS shr_u32
      IMPORTING iv_val TYPE i iv_shift TYPE i
      RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS shl32
      IMPORTING iv_val TYPE i iv_shift TYPE i
      RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS rotl32 ...
    CLASS-METHODS rotr32 ...
    CLASS-METHODS clz32 ...
    CLASS-METHODS ctz32 ...
    CLASS-METHODS popcnt32 ...

    " Memory helpers
    CLASS-METHODS reverse_i32 ...  " little-endian byte swap
    CLASS-METHODS to_le_i32 ...
    CLASS-METHODS reverse_i64 ...
    CLASS-METHODS to_le_i64 ...
ENDCLASS.
```

For i64, same approach using `TYPE int8` (signed 64-bit). Unsigned i64 comparisons are trickier (the sign bit matters) but QuickJS uses i64 sparingly.

---

## 6. call_indirect — Function Pointer Dispatch

WASM `call_indirect` calls a function by index from a table. The compiler generates a dispatch method:

```abap
METHOD dispatch_table0.
  " Generated CASE for all possible function indices in table 0
  CASE iv_func_idx.
    WHEN 0. rv = func_042( p0 = iv_p0 p1 = iv_p1 ).
    WHEN 1. rv = func_100( p0 = iv_p0 p1 = iv_p1 ).
    WHEN 2. rv = func_203( p0 = iv_p0 p1 = iv_p1 ).
    " ... potentially hundreds of entries
    WHEN OTHERS. RAISE EXCEPTION TYPE zcx_wasm_trap.
  ENDCASE.
ENDMETHOD.
```

**Problem:** Different table entries may have different function signatures (different param counts/types).

**Solution:** Group by type signature. Generate one dispatch method per unique type:

```abap
" Type 0: (i32, i32) -> i32
METHOD dispatch_type0.
  CASE iv_func_idx.
    WHEN 42. rv = func_042( p0 = iv_p0 p1 = iv_p1 ).
    WHEN 100. rv = func_100( p0 = iv_p0 p1 = iv_p1 ).
  ENDCASE.
ENDMETHOD.

" Type 1: (i32) -> void
METHOD dispatch_type1.
  CASE iv_func_idx.
    WHEN 203. func_203( p0 = iv_p0 ).
  ENDCASE.
ENDMETHOD.
```

---

## 7. WASI Shim

QuickJS needs ~10 WASI imports. Generated as methods on a helper class:

```abap
CLASS zcl_wasm_wasi DEFINITION PUBLIC.
  PUBLIC SECTION.
    METHODS fd_write
      IMPORTING iv_fd TYPE i iv_iovs TYPE i iv_iovs_len TYPE i
      EXPORTING ev_nwritten TYPE i
      CHANGING  cv_memory TYPE xstring.
    METHODS fd_read ...
    METHODS proc_exit ...
    METHODS environ_sizes_get ...  " return 0, 0
    METHODS environ_get ...        " return 0
    METHODS clock_time_get ...     " sy-uzeit
    METHODS args_sizes_get ...     " return 0, 0
    METHODS args_get ...           " return 0
ENDCLASS.
```

Most are stubs. `fd_write` is the only one that needs real logic (reading iov structures from linear memory, extracting strings, outputting via `WRITE` or collecting into a return buffer).

---

## 8. Implementation Plan

### Phase 1: Minimal Compiler (Go) — prove the concept

**Scope:** Compile simple .wasm files (hand-written, 5-10 functions) to ABAP.

**Deliverables:**
1. `pkg/wasmcomp/parser.go` — WASM binary parser (sections, types, functions, bytecode)
2. `pkg/wasmcomp/ssa.go` — Stack-to-SSA converter
3. `pkg/wasmcomp/codegen.go` — ABAP source emitter
4. `pkg/wasmcomp/runtime/` — ABAP runtime library source (zcl_wasm_rt)
5. `cmd/vsp/wasm2abap.go` — CLI command: `vsp wasm2abap input.wasm -o output.clas.abap`

**Test:** Compile a simple .wasm (add, fibonacci, factorial) → deploy to A4H → run → verify output.

### Phase 2: Control Flow + Memory

**Scope:** Full structured control flow (block, loop, if, br, br_table), linear memory ops, globals.

**Test:** Compile wasm-spec tests → deploy → compare results with abap-wasm.

### Phase 3: QuickJS

**Scope:** Compile the full QuickJS .wasm blob (~1000 functions, ~14MB) to ABAP.

**Challenges:**
- ABAP class size limits (max methods per class? may need to split across multiple classes)
- Memory usage (16MB+ XSTRING)
- WASI shim completeness
- Unsigned i64 edge cases

**Test:** `eval("1+1")` → `"2"` running on SAP.

### Phase 4: abaplint on SAP

**Scope:** Run abaplint parser (TypeScript compiled to QuickJS WASM) inside SAP.

**The dream:** `CALL METHOD zcl_abaplint=>parse( source ) → AST`

---

## 9. Size Estimates

| Component | Lines (Go) | Lines (ABAP) |
|-----------|:----------:|:------------:|
| WASM parser | ~800 | — |
| SSA builder | ~600 | — |
| ABAP codegen | ~1,000 | — |
| Runtime library | — | ~500 |
| WASI shim | — | ~200 |
| Generated QuickJS | — | ~50,000-100,000 (estimate) |
| **Compiler total** | **~2,400** | — |
| **Runtime total** | — | **~700** |

The compiler is small. The generated output is huge (QuickJS has ~1000 functions of compiled C code). But it's machine-generated — nobody reads it.

---

## 10. Prior Art & References

| Project | Approach | Performance |
|---------|----------|-------------|
| **abap-wasm** (Lars Hvam) | Interpreter, 442 instruction classes | Slow (dynamic dispatch per opcode) |
| **wasm2c** (WebAssembly project) | WASM → C compiler | Near-native (same idea, targets C) |
| **wasm2lua** | WASM → Lua transpiler | Decent (Lua is faster than ABAP) |
| **MinZ** (this team) | ABAP → HIR → MIR2 → Z80/native | N/A (different direction) |
| **This project** | WASM → ABAP (AOT compiler) | Expected: 10-100x faster than abap-wasm |

The `wasm2c` project is the closest reference — it does exactly what we do but targets C instead of ABAP. Similar design: one C function per WASM function, linear memory as a byte array, stack unrolled into locals.

---

## 11. Why Not WASM → MIR2 → ABAP?

Considered and rejected for v1:

- **WASM is already low-level enough.** MIR2 is designed for register-constrained targets (Z80, 6502). ABAP has unlimited variables — we don't need register allocation.
- **Extra IR = extra complexity.** WASM → ABAP is a single translation step. WASM → MIR2 → ABAP is two, with MIR2 carrying concepts (register classes, clobber sets) irrelevant to ABAP.
- **MIR2 doesn't support i64 yet.** QuickJS needs it.
- **Direct translation preserves WASM structure.** Functions stay functions, blocks stay blocks. MIR2 would flatten and restructure, then we'd have to un-flatten for ABAP.

MIR2 path makes sense if we later want to **optimize** the generated ABAP (dead code elimination, constant propagation, etc.). For now, direct is correct.
