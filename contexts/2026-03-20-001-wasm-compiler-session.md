# Session Context: WASM-to-ABAP Compiler Sprint

**Date:** 2026-03-19 → 2026-03-20
**Duration:** Extended session (~12 hours of work)
**Branch:** feat/wasm-abap → merged to main
**Releases:** v2.29.0, v2.30.0

---

## Key Discoveries

### 1. ABAP String Comparison Gotcha
`' '` (single-quote space) gets trimmed in TYPE STRING comparison. `lv_string = ' '` matches empty string!
**Fix:** Always use backtick `` ` ` `` or string template `| |` for space comparisons.
This affects ALL transpiled code from TypeScript/JavaScript to ABAP.

### 2. WASM Stack Machine → SSA is Straightforward
Each WASM stack push → new variable `s0`, `s1`, `s2`. Each pop → read previous variable.
Control flow maps cleanly: block→DO 1 TIMES, loop→DO, if→IF, br→EXIT, br_if→IF...EXIT.

### 3. ABAP Line Limit = 255 chars (not 80)
Machine-generated code can pack multiple statements per line up to 255 chars.
IF/ELSE/ENDIF/DO/ENDDO can all go on one line → 5.5x compression.

### 4. GENERATE SUBROUTINE POOL for Runtime Compilation
ABAP can compile and execute source code at runtime. This enables the self-hosting compiler:
parse WASM → generate ABAP string → GENERATE SUBROUTINE POOL → PERFORM.

### 5. Function Group = Best Container for Large Generated Code
No include size limits, shared globals via TOP, no method count limits, FORMs are unlimited.
Use PERFORM for calls (zero dispatch overhead) and direct `gv_*` for global access.

### 6. CROSS/WBCROSSGT Tables Can Be Stale
Only updated on activation. Missing for $TMP, inactive objects, unactivated changes.
Parser reads actual source = ground truth. CROSS = supplementary index.

### 7. chained DATA: syntax saves lines
`DATA: s0 TYPE i, s1 TYPE i, s2 TYPE i, lv_br TYPE i.` instead of individual DATA statements.

### 8. porffor (JS AOT) Produces 22-1000x Smaller WASM Than Javy
But porffor v0.61 is experimental — returns wrong values for recursion.
AssemblyScript works correctly but requires rewrite with i32/i64 types.
Javy/QuickJS is the only production-ready JS→WASM path today.

### 9. TS→ABAP Direct Transpilation = 800x Smaller Than WASM Path
Same functionality: 120 lines ABAP (direct) vs 101K lines (via WASM).
95% of the transpilation is mechanical (number→i, this.→me->, push→APPEND).

### 10. vsp deploy CLI for Batch Operations
`for f in *.clas.abap; do vsp -s system deploy "$f" '$PKG'; done`
40 classes deployed with zero failures.

---

## Architecture Decisions

### WASM Compiler Backend Choices
- **FUGR:** Best for large modules (QuickJS). Shared globals, no limits.
- **Class:** For small modules (<74K lines). Clean OO but hits SAP limits.
- **Hybrid:** FUGR internals + class wrapper. Best of both.

### Unified Analyzer Confidence Model
```
Parser (reads source)           = ground truth (0.9-1.0)
SCAN ABAP-SOURCE (SAP kernel)   = reliable (0.85)
CROSS/WBCROSSGT (index)         = supplementary, may be stale (0.6-0.8)
Regex (pattern matching)         = fast but may have false positives (0.3)
```

### Context Compression Strategy
Regex for fast scanning → parser validates → strips strings/comments →
compresses to PUBLIC SECTION only → 7-30x reduction per dependency.

---

## What Was Deployed to SAP A4H ($ZOZIK)

```
WASM Compiler:        zcl_wasm_reader, zcl_wasm_module, zcl_wasm_codegen
abaplint Lexer:       zcl_lexer, zcl_lexer_stream, zcl_lexer_buffer
                      zcl_position, zcl_abstract_token, 46x zcl_tok_*
Statement Parser:     zcl_statement_parser
Test Objects:         zcl_wasm_test, zcl_zozik_calculator, zcl_zozik_string_utils
                      zqjs_test, zqjs (function group shell)
```

---

## Packages Created

| Package | Purpose | Key Files |
|---------|---------|-----------|
| `pkg/wasmcomp/` | WASM→ABAP AOT compiler | parser.go, codegen.go, backend.go, compile.go |
| `pkg/ts2abap/` | TypeScript→ABAP transpiler | ts2abap.go, ts_ast.js |
| `pkg/ctxcomp/analyzer.go` | Unified 5-layer analyzer | analyzer.go |
| `embedded/abap/wasm_compiler/` | Self-hosting compiler (ABAP) | 3 classes, 785 lines |

---

## Numbers

| Metric | Value |
|--------|------:|
| New packages | 3 |
| New Go files | ~25 |
| New ABAP classes on SAP | 57 |
| Reports | 6 |
| Commits | 28 |
| Lines of new code | ~10,800 |
| SAP tests passed | 30+ |
| Releases | 2 (v2.29.0, v2.30.0) |
| Platform binaries | 9 × 2 = 18 |
