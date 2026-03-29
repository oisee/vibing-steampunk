# WASM-to-ABAP Roadmap — Paths to Native ABAP Execution

**Date:** 2026-03-19
**Report ID:** 004
**Subject:** Architecture options for running WASM on SAP — Go compiler, ABAP-native compiler, HANA/AMDP engine

---

## 1. Current Architecture (What We Built Today)

```
.wasm binary → Go compiler (pkg/wasmcomp) → .abap source → deploy to SAP
```

**Proven:** QuickJS (1,410 functions) → 101K lines ABAP, 5.5x packed.
**Proven:** abaplint (26.5MB WASM) → 396K lines ABAP.
**Proven:** Runtime primitives pass on SAP A4H (16/16 tests).

---

## 2. Why 26.5 MB? The QuickJS Tax

The abaplint WASM contains:
- **QuickJS engine** (~1.2 MB WASM code, 1,410 functions)
- **abaplint JS bundle** (~3 MB, stored in 58,351 data segments)
- **WASM overhead** (section headers, type info, etc.)

QuickJS is a **JS interpreter inside WASM**. We're transpiling an interpreter to ABAP, then it interprets JavaScript. Three levels:

```
SAP ABAP VM → executes compiled ABAP → which is QuickJS → which interprets abaplint JS
```

This works but is inefficient. QuickJS was designed for small embedded systems, not for performance.

### More Compact Alternatives

| Approach | Skip QuickJS? | WASM Size | ABAP Lines (est.) | Effort |
|----------|:---:|:---------:|:--:|:---:|
| **Javy/QuickJS** (current) | No | 26.5 MB | 396K | Done |
| **QuickJS slim** (strip unused) | No | ~10 MB | ~200K | Medium |
| **AssemblyScript** | Yes | ~3 MB | ~50K | Rewrite abaplint in AS |
| **Rust/wasm-bindgen** | Yes | ~1 MB | ~20K | Rewrite parser in Rust |
| **Hand-written WASM** | Yes | ~0.1 MB | ~5K | Write parser from scratch |
| **Direct ABAP** | N/A | 0 | ~10K | Write parser natively |

**Best near-term path:** Keep QuickJS for now (it works), optimize the data segment loading, and explore AssemblyScript for a v2.

---

## 3. Path A: Native ABAP WASM→ABAP Compiler (Self-Hosting)

### The Idea

Write the WASM binary parser + ABAP codegen **in ABAP itself**. Then:

```
1. Deploy the compiler to SAP (small: ~3-5K lines of ABAP)
2. Upload any .wasm as XSTRING (base64 or ZIP)
3. Compiler parses WASM → generates ABAP source → creates objects via GENERATE SUBROUTINE POOL
```

### Architecture

```abap
CLASS zcl_wasm_compiler DEFINITION.
  PUBLIC SECTION.
    METHODS compile
      IMPORTING iv_wasm    TYPE xstring
                iv_name    TYPE string
      RETURNING VALUE(rv_source) TYPE string.
ENDCLASS.

" Usage:
DATA(lo_compiler) = NEW zcl_wasm_compiler( ).
DATA(lv_abap) = lo_compiler->compile( iv_wasm = lv_wasm_bytes iv_name = 'ZQJS' ).
" Now lv_abap contains the generated ABAP source
" Deploy via GENERATE SUBROUTINE POOL or INSERT REPORT
```

### Components (estimated ~5K lines of ABAP)

| Component | Lines | Description |
|-----------|:-----:|-------------|
| Binary parser | ~1,500 | Read WASM sections, decode LEB128, parse bytecode |
| Stack→SSA | ~800 | Convert stack ops to named variables |
| ABAP emitter | ~1,500 | Generate FORM/METHOD source text |
| Runtime helpers | ~700 | zcl_wasm_rt (already written) |
| WASI shim | ~300 | fd_write, clock_time_get, etc. |
| **Total** | **~4,800** | |

### Bootstrap Sequence

```
Step 1: Deploy zcl_wasm_compiler + zcl_wasm_rt to SAP (via vsp)
Step 2: Upload quickjs.wasm as XSTRING
Step 3: zcl_wasm_compiler->compile( quickjs_wasm ) → ABAP source
Step 4: INSERT REPORT / GENERATE SUBROUTINE POOL → runnable code
Step 5: Now QuickJS runs natively, can execute any JS
Step 6: Upload abaplint.js → QuickJS runs it → ABAP parser on SAP!
```

### Key ABAP APIs for Dynamic Code

```abap
" Generate temporary subroutine pool
GENERATE SUBROUTINE POOL lt_source NAME lv_prog.
PERFORM f_start IN PROGRAM (lv_prog).

" Or: create persistent report
INSERT REPORT lv_name FROM lt_source.
GENERATE REPORT lv_name.

" Or: create via ADT (our own vsp could do this from within SAP!)
```

---

## 4. Path B: WASM Engine in HANA AMDP/SQLScript

### The Wild Idea

HANA SQLScript is Turing-complete (loops, variables, IF/ELSE, stored procedures). Could we run WASM inside HANA?

### SQLScript Capabilities

| Feature | SQLScript | Needed for WASM |
|---------|:---------:|:---------------:|
| Integer arithmetic | ✅ | ✅ |
| 64-bit integers | ✅ (BIGINT) | ✅ (i64) |
| Floating point | ✅ (DOUBLE) | ✅ (f32/f64) |
| Bitwise ops | ✅ (BITAND, BITOR, BITXOR) | ✅ |
| Byte arrays | ✅ (VARBINARY) | ✅ (linear memory) |
| Loops | ✅ (WHILE, FOR) | ✅ |
| IF/ELSE | ✅ | ✅ |
| Procedure calls | ✅ | ✅ (function calls) |
| Recursion | ⚠️ (limited stack) | ✅ |
| Dynamic dispatch | ❌ | ⚠️ (call_indirect) |
| Unsigned integers | ❌ (signed only) | ⚠️ (same as ABAP) |

### Two Approaches

**A) Interpreter in SQLScript**

```sql
-- WASM bytecode stored as VARBINARY
-- Interpreter loop reads opcodes and executes
CREATE PROCEDURE WASM_EXEC(IN wasm VARBINARY, OUT result BIGINT)
AS BEGIN
  DECLARE pc INTEGER = 0;
  DECLARE stack TABLE(val BIGINT);
  DECLARE memory VARBINARY(16777216);  -- 16MB

  WHILE pc < LENGTH(wasm) DO
    DECLARE op TINYINT = SUBSTR_BINARY(wasm, pc, 1);
    CASE op
      WHEN X'6A' THEN  -- i32.add
        -- pop two, push sum
      WHEN X'41' THEN  -- i32.const
        -- read LEB128, push
      ...
    END CASE;
    pc = pc + 1;
  END WHILE;
END;
```

**Problem:** Interpreter in SQL is extremely slow. Each opcode = CASE branch.

**B) AOT Compilation → SQLScript Procedures**

Same approach as our Go compiler, but target is SQLScript:

```sql
-- Generated from WASM function 42
CREATE PROCEDURE WASM_F42(IN p0 INTEGER, IN p1 INTEGER, OUT rv INTEGER)
AS BEGIN
  DECLARE s0 INTEGER; DECLARE s1 INTEGER;
  s0 = p0; s1 = p1; s0 = s0 + s1; rv = s0;
END;
```

**Advantages:**
- HANA compiles SQLScript to native code (column engine)
- Massive parallelism potential
- Direct access to HANA tables from within WASM functions
- Could process millions of rows through WASM logic

**Disadvantages:**
- SQLScript recursion limits (call_indirect is hard)
- No VARBINARY offset operations like `mem+offset(4)` — need helper functions
- 16MB+ memory as VARBINARY is unusual
- Limited to procedures (no dynamic code generation)

### AMDP Integration

```abap
CLASS zcl_wasm_hana DEFINITION.
  PUBLIC SECTION.
    INTERFACES if_amdp_marker_hdb.
    METHODS run_wasm
      IMPORTING VALUE(iv_input) TYPE string
      EXPORTING VALUE(ev_output) TYPE string.
ENDCLASS.

CLASS zcl_wasm_hana IMPLEMENTATION.
  METHOD run_wasm BY DATABASE PROCEDURE FOR HDB LANGUAGE SQLSCRIPT.
    -- Generated WASM functions as SQLScript
    CALL WASM_START(:iv_input, :ev_output);
  ENDMETHOD.
ENDCLASS.
```

### Verdict

HANA WASM is **interesting for data processing** (run WASM logic over table rows) but **impractical for QuickJS** (too much state, recursion, dynamic dispatch). Best use case: compile small, compute-heavy WASM functions to SQLScript for HANA-accelerated processing.

---

## 5. Path C: Compact WASM for ABAP Parser (Skip QuickJS)

### The Real Question

We don't need a full JavaScript engine to parse ABAP. We need **just the parser**. Options:

**C1: tree-sitter ABAP grammar → WASM (~200 KB)**

tree-sitter compiles grammars to C, which compiles to WASM. A tree-sitter ABAP grammar would produce a ~200KB WASM blob — tiny compared to 26.5MB.

```
tree-sitter ABAP grammar → C → Emscripten → .wasm (200KB) → our compiler → ABAP (~5K lines)
```

**C2: Hand-rolled ABAP tokenizer + parser in Zig → WASM (~100 KB)**

A purpose-built ABAP tokenizer (keywords, identifiers, strings, numbers) + recursive descent parser. Compiles to very compact WASM.

**C3: abaplint core compiled with AssemblyScript (~3 MB)**

AssemblyScript is a TypeScript subset that compiles directly to WASM without QuickJS. If abaplint were ported to AssemblyScript, we'd get a ~3MB WASM with native performance.

**C4: Regex-based (what we have now in ctxcomp)**

Our current `pkg/ctxcomp` uses 10 regex patterns for dependency extraction. No WASM needed. Works but limited — can't build a full AST.

### Recommendation

| Timeframe | Approach | Result |
|-----------|----------|--------|
| **Now** | QuickJS/Javy (done) | 396K lines ABAP, full abaplint |
| **Next** | tree-sitter ABAP | ~5K lines ABAP, fast parsing |
| **Future** | AssemblyScript port | ~50K lines ABAP, full abaplint features |

---

## 6. Summary: All Paths

```
                          ┌─────────────────────────┐
                          │   .wasm binary          │
                          └────────┬────────────────┘
                                   │
                    ┌──────────────┼──────────────┐
                    ▼              ▼              ▼
             ┌──────────┐  ┌──────────┐  ┌──────────┐
             │ Go/vsp   │  │ ABAP     │  │ HANA     │
             │ compiler │  │ compiler │  │ compiler │
             │ (done)   │  │ (next)   │  │ (future) │
             └────┬─────┘  └────┬─────┘  └────┬─────┘
                  ▼              ▼              ▼
           ┌──────────┐  ┌──────────┐  ┌──────────┐
           │ ABAP     │  │ ABAP     │  │ SQLScript│
           │ source   │  │ source   │  │ procs    │
           │ files    │  │ (dynamic)│  │          │
           └────┬─────┘  └────┬─────┘  └────┬─────┘
                ▼              ▼              ▼
           ┌──────────┐  ┌──────────┐  ┌──────────┐
           │ abapGit  │  │ GENERATE │  │ HANA     │
           │ deploy   │  │ REPORT   │  │ deploy   │
           └──────────┘  └──────────┘  └──────────┘
```

### Immediate TODO

1. ☐ Deploy QuickJS via abapGit ZIP to SAP
2. ☐ Test `_start` execution
3. ☐ Split INIT file for SAP include limits
4. ☐ Write ABAP-native WASM compiler (zcl_wasm_compiler)
5. ☐ Investigate tree-sitter ABAP grammar for compact parser

### Notes

- **abapGit ZIP deploy** is the path for QuickJS/abaplint deployment
- **ABAP-native compiler** enables self-hosting: upload WASM, compile in-system
- **HANA AMDP** best for data-processing WASM, not general-purpose VMs
- **wazero (in vsp)** does NOT use QuickJS — it's a pure Go WASM runtime
- **The 26.5MB WASM = QuickJS engine + abaplint JS code** (data segments)
