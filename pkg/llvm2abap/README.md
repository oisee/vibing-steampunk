# llvm2abap — LLVM IR → ABAP Compiler

Compiles LLVM IR text format (`.ll`) to idiomatic typed ABAP with CLASS-METHODS.

## Usage

```go
import "github.com/oisee/vibing-steampunk/pkg/llvm2abap"

src, _ := os.ReadFile("program.ll")
mod, _ := llvm2abap.Parse(string(src))
abap := llvm2abap.Compile(mod, "zcl_program")
```

## Pipeline

```
C/Rust/Swift → clang/rustc -emit-llvm → .ll → llvm2abap → ABAP → SAP
```

Generate LLVM IR:
```bash
clang -S -emit-llvm -O1 program.c -o program.ll
```

## What it produces

```abap
CLASS zcl_program DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    CLASS-METHODS add IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS factorial IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS fibonacci IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
ENDCLASS.

CLASS zcl_program IMPLEMENTATION.
  METHOD add.
    rv = a + b.
  ENDMETHOD.
  METHOD factorial.
    " Loop with phi nodes, CASE dispatcher
    ...
  ENDMETHOD.
ENDCLASS.
```

## Supported LLVM IR

| Feature | Status | ABAP mapping |
|---------|--------|-------------|
| i32/i64/float/double types | ✅ | TYPE i / int8 / f |
| add, sub, mul, div, rem | ✅ | arithmetic |
| and, or, xor, shl, ashr, lshr | ✅ | BIT-AND/OR/XOR, ipow |
| icmp (eq/ne/slt/sgt/sle/sge) | ✅ | IF/ELSE |
| select | ✅ | IF/ELSE ternary |
| br (conditional/unconditional) | ✅ | CASE lv_block dispatcher |
| phi nodes | ✅ | Assignment at branch source |
| call / tail call | ✅ | CLASS-METHOD calls |
| ret | ✅ | rv = ... RETURN |
| alloca | ✅ | DATA (stack var) |
| load / store | ✅ | PERFORM mem_ld/st_i32 |
| getelementptr (struct) | ✅ | Field offset calculation |
| getelementptr (array) | ✅ | Base + index * size |
| switch | ✅ | CASE/WHEN |
| zext / sext / trunc | ✅ | Type passthrough |
| freeze | ✅ | Passthrough |
| llvm.abs / smax / smin | ✅ | abs() / IF max/min |
| Named struct types | ✅ | Field offset mapping |

## Test Corpus

- **34-function C corpus**: add, factorial, fibonacci, gcd, is_prime, quadratic, structs, arrays
- **FatFS R0.16**: 28 functions (f_mount, f_open, f_read, f_write, ...), 8016 lines ABAP, 0 TODOs
- **SAP verified**: 5/5 tests pass via GENERATE SUBROUTINE POOL on SAP a4h-105

## Comparison: LLVM→ABAP vs WASM→ABAP

| | WASM→ABAP | LLVM→ABAP |
|---|---|---|
| Types | Everything TYPE i | i, int8, f (preserved) |
| Functions | FORM USING p0 p1 | CLASS-METHODS IMPORTING a TYPE i |
| Memory | All xstring | alloca → DATA, heap → xstring |
| Readability | Assembly-like (s0, s1) | Named vars (lv_result) |
| Struct access | Byte offsets | Field offset with comments |
