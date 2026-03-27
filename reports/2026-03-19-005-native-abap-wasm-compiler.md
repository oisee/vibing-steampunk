# Native ABAP WASM→ABAP Compiler — Design Plan

**Date:** 2026-03-19
**Report ID:** 005
**Subject:** Self-hosting WASM compiler written in ABAP, deployable to any SAP system
**Branch:** `feat/wasm-abap`

---

## 1. Goal

Write the WASM→ABAP compiler **in ABAP itself**. Once deployed to a SAP system, it can compile any `.wasm` binary to native ABAP — without Go, without vsp, without external tools.

```
Upload .wasm (XSTRING) → zcl_wasm_compiler→compile( ) → ABAP source → deploy via abapGit-style APIs
```

## 2. Bootstrap Sequence

```
Step 1: Deploy zcl_wasm_compiler + zcl_wasm_rt to SAP (via vsp deploy)
Step 2: Upload any .wasm as XSTRING (base64, RFC, or file upload)
Step 3: zcl_wasm_compiler→compile( wasm_bytes ) → returns ABAP source strings
Step 4: Create objects via abapGit-style APIs:
        - SEO_CLASS_CREATE_COMPLETE (for classes)
        - INSERT REPORT (for programs/includes)
        - or GENERATE SUBROUTINE POOL (for temporary execution)
Step 5: Activate via RS_COP_GENERATE or SEO_CLASS_GENERATE_COMPLETE
```

## 3. Architecture

```abap
CLASS zcl_wasm_compiler DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    TYPES: BEGIN OF ty_output_file,
             filename TYPE string,
             source   TYPE string,
           END OF ty_output_file,
           ty_output_files TYPE STANDARD TABLE OF ty_output_file WITH DEFAULT KEY.

    " Main entry point: WASM bytes → ABAP source files
    METHODS compile
      IMPORTING iv_wasm    TYPE xstring
                iv_name    TYPE string DEFAULT 'ZWASM'
                iv_backend TYPE string DEFAULT 'FUGR'  " FUGR | CLASS
                iv_funcs_per_include TYPE i DEFAULT 80
      RETURNING VALUE(rt_files) TYPE ty_output_files.

    " Convenience: compile and deploy to SAP in one step
    METHODS compile_and_deploy
      IMPORTING iv_wasm    TYPE xstring
                iv_name    TYPE string
                iv_package TYPE devclass DEFAULT '$TMP'
      RETURNING VALUE(rv_message) TYPE string.

    " Quick execute: compile + GENERATE SUBROUTINE POOL + run
    METHODS execute
      IMPORTING iv_wasm      TYPE xstring
                iv_function  TYPE string DEFAULT '_start'
      RETURNING VALUE(rv_result) TYPE string.
ENDCLASS.
```

## 4. Components (Estimated ~5K lines ABAP)

### 4.1 Binary Parser (~1,500 lines)

Port from our Go `pkg/wasmcomp/parser.go`. Key classes:

```abap
CLASS zcl_wasm_reader DEFINITION.
  " Reads WASM binary: LEB128, sections, bytecode
  METHODS read_byte RETURNING VALUE(rv) TYPE i.
  METHODS read_u32 RETURNING VALUE(rv) TYPE i.       " unsigned LEB128
  METHODS read_i32 RETURNING VALUE(rv) TYPE i.       " signed LEB128
  METHODS read_i64 RETURNING VALUE(rv) TYPE int8.    " signed LEB128
  METHODS read_bytes IMPORTING iv_n TYPE i RETURNING VALUE(rv) TYPE xstring.
  METHODS read_string RETURNING VALUE(rv) TYPE string.
ENDCLASS.

CLASS zcl_wasm_parser DEFINITION.
  " Parses sections: types, imports, functions, memory, globals, exports, code, data
  METHODS parse IMPORTING iv_wasm TYPE xstring RETURNING VALUE(ro_module) TYPE REF TO zcl_wasm_module.
ENDCLASS.

CLASS zcl_wasm_module DEFINITION.
  " Holds parsed module: types, functions, imports, exports, memory, globals, data segments
  DATA mt_types TYPE STANDARD TABLE OF REF TO zcl_wasm_func_type.
  DATA mt_functions TYPE STANDARD TABLE OF REF TO zcl_wasm_function.
  DATA mt_imports TYPE STANDARD TABLE OF REF TO zcl_wasm_import.
  " ...
ENDCLASS.
```

### 4.2 SSA Builder (~800 lines)

Convert stack machine instructions to named variables:

```abap
CLASS zcl_wasm_ssa DEFINITION.
  " Virtual stack → named variables (s0, s1, ...)
  METHODS push RETURNING VALUE(rv_name) TYPE string.
  METHODS pop RETURNING VALUE(rv_name) TYPE string.
  METHODS peek RETURNING VALUE(rv_name) TYPE string.
ENDCLASS.
```

### 4.3 ABAP Code Generator (~1,500 lines)

Port from our Go `pkg/wasmcomp/codegen.go`:

```abap
CLASS zcl_wasm_codegen DEFINITION.
  " Emits ABAP source for each WASM function
  " Supports FUGR (FORMs) and CLASS (METHODs) backends
  METHODS emit_function
    IMPORTING io_func TYPE REF TO zcl_wasm_function
    RETURNING VALUE(rv_source) TYPE string.

  METHODS emit_instruction
    IMPORTING is_inst TYPE ty_instruction
    CHANGING  co_stack TYPE REF TO zcl_wasm_ssa
              cv_source TYPE string.
ENDCLASS.
```

### 4.4 Line Packer (~200 lines)

Pack multiple statements per line (up to 240 chars):

```abap
CLASS zcl_wasm_packer DEFINITION.
  METHODS add IMPORTING iv_stmt TYPE string.
  METHODS flush RETURNING VALUE(rv_line) TYPE string.
ENDCLASS.
```

### 4.5 Deployer (~700 lines)

Create ABAP objects using SAP APIs (inspired by abapGit):

```abap
CLASS zcl_wasm_deployer DEFINITION.
  METHODS deploy_class
    IMPORTING iv_name TYPE seoclsname
              iv_source TYPE string
              iv_package TYPE devclass
              iv_description TYPE string.

  METHODS deploy_fugr_include
    IMPORTING iv_fugr TYPE rs38l_area
              iv_include TYPE programm
              iv_source TYPE string.

  METHODS deploy_program
    IMPORTING iv_name TYPE programm
              iv_source TYPE string
              iv_package TYPE devclass.

  METHODS activate
    IMPORTING iv_object_url TYPE string
              iv_name TYPE string.
ENDCLASS.
```

**Key SAP APIs to use** (learned from abapGit codebase):

```abap
" Create class
CALL FUNCTION 'SEO_CLASS_CREATE_COMPLETE'
  EXPORTING devclass = iv_package
            suppress_dialog = abap_true
  CHANGING  class = ls_properties.

" Set superclass
MODIFY vseoextend FROM ls_inh.

" Activate class
CALL FUNCTION 'SEO_CLASS_GENERATE_COMPLETE'
  EXPORTING clskey = ls_clskey
            suppress_dialog = abap_true.

" Create/update program/include source
INSERT REPORT iv_name FROM lt_source.
" or
SYNTAX-CHECK FOR lt_source MESSAGE lv_msg LINE lv_line.

" Generate program
GENERATE REPORT iv_name.

" For temporary execution (no persistent object)
GENERATE SUBROUTINE POOL lt_source NAME lv_prog.
PERFORM f_start IN PROGRAM (lv_prog).

" Create function group
CALL FUNCTION 'FUNCTION_INCLUDE_INSERT'
  EXPORTING funcgroup = iv_fugr
            include   = iv_include.
```

## 5. Runtime Library (`zcl_wasm_rt`) — Already Written

The Go-generated `zcl_wasm_rt` class (from `pkg/wasmcomp/compile.go`) is directly usable. It contains:
- Unsigned 32/64-bit arithmetic
- Bitwise ops (BIT-AND/OR/XOR on XSTRING)
- Shifts, rotations, CLZ/CTZ/POPCNT
- Memory alloc, copy, fill, load/store (little-endian)
- All conversion helpers (wrap, extend, truncate, reinterpret)

This class deploys once and is reused by all compiled WASM modules.

## 6. Usage Scenarios

### Scenario A: Compile and Deploy Permanently

```abap
DATA(lo_compiler) = NEW zcl_wasm_compiler( ).
DATA(lt_files) = lo_compiler->compile(
  iv_wasm = lv_wasm_bytes
  iv_name = 'ZQUICKJS'
  iv_backend = 'FUGR' ).

" Deploy all generated files
DATA(lo_deployer) = NEW zcl_wasm_deployer( ).
LOOP AT lt_files INTO DATA(ls_file).
  lo_deployer->deploy( ls_file ).
ENDLOOP.
```

### Scenario B: Quick Execute (No Persistent Objects)

```abap
DATA(lo_compiler) = NEW zcl_wasm_compiler( ).
DATA(lv_result) = lo_compiler->execute(
  iv_wasm = lv_wasm_bytes
  iv_function = '_start' ).
" Uses GENERATE SUBROUTINE POOL internally — no objects left behind
```

### Scenario C: Remote Compilation via RFC

```abap
" From another system: send .wasm, get back ABAP source
CALL FUNCTION 'ZWASM_COMPILE' DESTINATION 'DEV'
  EXPORTING iv_wasm = lv_wasm_bytes
            iv_name = 'ZPARSER'
  IMPORTING et_files = lt_abap_files.
```

## 7. Implementation Plan

| Phase | What | Lines | Priority |
|-------|------|:-----:|:--------:|
| 1 | Binary parser (WASM reader + section parser) | ~1,500 | HIGH |
| 2 | SSA builder + codegen (instruction → ABAP) | ~2,300 | HIGH |
| 3 | Line packer + FUGR backend | ~500 | HIGH |
| 4 | Deployer (abapGit-style object creation) | ~700 | MEDIUM |
| 5 | WASI shim (fd_write, clock, environ) | ~300 | MEDIUM |
| 6 | CLI: FM wrappers for compile/deploy/execute | ~200 | LOW |
| **Total** | | **~5,500** | |

## 8. Size Comparison

| Component | Go (current) | ABAP (planned) |
|-----------|:------------:|:--------------:|
| Binary parser | 400 lines | ~1,500 lines |
| Codegen | 700 lines | ~2,300 lines |
| Line packer | 100 lines | ~200 lines |
| Backends | 400 lines | ~500 lines |
| Runtime | (generated) | 500 lines (already written) |
| **Total** | **~1,600** | **~5,000** |

ABAP is ~3x more verbose than Go, which is expected.

## 9. Why This Matters

Once the compiler is on SAP, the system becomes **self-hosting**:
- Upload any `.wasm` → compile to ABAP → deploy → run
- No external tools needed at runtime
- Any language that compiles to WASM (C, Rust, Go, AssemblyScript, Zig) can target SAP
- The compiler itself could be compiled from WASM (meta-compilation!)
