# LLVM → ABAP Compilation: Deep Research

## Executive Summary

MinZ already has two paths to ABAP:
1. **Direct**: MIR2 → mir2abap → ABAP REPORT source
2. **Proposed**: MIR2 → LLVM IR → (optimization) → ABAP

The question: can LLVM IR serve as a better intermediate step for ABAP codegen?

**Verdict**: LLVM optimization passes are valuable for MIR2 quality, but LLVM→ABAP transpilation adds complexity without clear benefit over direct mir2abap. The recommended path is **MIR2 → LLVM opt → MIR2' → mir2abap**.

---

## Current Architecture

```
                    ┌─── Z80 asm (production)
                    ├─── eZ80 asm (Agon Light)
                    ├─── QBE IL → native
Nanz/C/ABAP → HIR → MIR2 ─┼─── C99 source
                    ├─── WASM binary (wazero)
                    ├─── LLVM IR (.ll) ← NEW
                    └─── ABAP REPORT (mir2abap)
```

### What We Have Today

| Path | Status | Verified |
|------|--------|----------|
| MIR2 → LLVM IR | Working, 10/10 tests, lli verified | add(3,4)=7 native |
| MIR2 → WASM binary | Working, wazero verified | 5/5 test functions |
| MIR2 → ABAP REPORT | Working, 8 examples | hello, fibonacci, fizzbuzz, OOP, ALV, SQL |

---

## Three Candidate Architectures

### Option A: Direct (Current)
```
MIR2 → mir2abap → ABAP REPORT
```

**Pros:**
- Already implemented (14KB codegen.go)
- Minimal abstraction layers
- Full control over ABAP idioms

**Cons:**
- No optimization between MIR2 and ABAP
- ABAP output is assembly-like, not idiomatic
- Bitwise ops need `cl_abap_math` (slow)

### Option B: LLVM-Optimized
```
MIR2 → LLVM IR → opt -O2 → LLVM IR' → custom ABAP emitter
```

**Pros:**
- LLVM's 200+ optimization passes (DCE, CSE, loop unroll, vectorization)
- Constant folding already demonstrated (add(3,4) → 7 at IR level)
- Could eliminate dead code before ABAP emission

**Cons:**
- Need custom LLVM IR → ABAP translator (doesn't exist)
- LLVM optimizations target machine architectures, not high-level languages
- LLVM may lower to patterns that don't map cleanly to ABAP
- Two abstraction boundaries = two places for bugs

### Option C: Hybrid (Recommended)
```
MIR2 → LLVM IR → opt -O2 → read back → MIR2' → mir2abap → ABAP
```

**Pros:**
- Leverage LLVM optimization without custom ABAP backend
- MIR2' benefits from constant folding, DCE, CSE
- mir2abap stays simple (one codegen target)
- Can A/B test optimized vs unoptimized ABAP

**Cons:**
- LLVM IR → MIR2 roundtrip is non-trivial
- Some LLVM optimizations may not survive roundtrip
- Adds complexity to build pipeline

---

## LLVM vs WASM as ABAP Bridge

### WASM → ABAP (alternative path)

WASM could theoretically serve as ABAP bridge:
```
MIR2 → WASM binary → WASM interpreter in ABAP
```

This would mean writing a WASM VM in ABAP — a meta-circular approach.

| Factor | LLVM path | WASM path |
|--------|-----------|-----------|
| **Optimization** | 200+ passes via opt | Binaryen optimizer (wasm-opt) |
| **Output readability** | SSA (readable) | Stack bytecode (opaque) |
| **ABAP mapping** | IR→ABAP possible | Need WASM VM in ABAP |
| **Tooling** | Mature (llc, opt, lli) | Growing (wasm-opt, wabt) |
| **Complexity** | Medium | High (VM implementation) |
| **Performance in ABAP** | Direct FORM calls | Interpreter overhead |

**Winner: LLVM** — more direct mapping to ABAP constructs.

---

## LLVM Optimizations Relevant to ABAP

Not all LLVM passes help ABAP. Relevant ones:

### High Value
- **Constant folding/propagation** — ABAP has no compile-time evaluation
- **Dead code elimination** — Remove unreachable FORMs
- **Common subexpression elimination** — ABAP recomputes repeatedly
- **Loop-invariant code motion** — ABAP DO/WHILE loops are naive
- **Inlining** — Eliminate PERFORM overhead (ABAP PERFORM is expensive)

### Medium Value
- **Strength reduction** — `x * 2` → `x + x` (faster in ABAP integer arithmetic)
- **Loop unrolling** — Reduce DO TIMES overhead
- **Tail call elimination** — PERFORM recursion → DO loop

### Low Value (ABAP doesn't benefit)
- **Vectorization** — ABAP has no SIMD
- **Register allocation** — ABAP uses named variables
- **Instruction scheduling** — ABAP is interpreted
- **Branch prediction hints** — No hardware to predict

---

## ABAP-Specific Challenges

### 1. Memory Model Mismatch
LLVM IR assumes flat memory with pointers. ABAP uses:
- Named DATA variables (no pointer arithmetic)
- Internal tables (dynamic arrays with header lines)
- REF TO (managed references, not raw pointers)

**Workaround (mir2abap):** Flat `xstring` buffer + FORM mem_ld8/mem_st8.

### 2. Type System
LLVM IR: i1, i8, i16, i32, i64, float, double, ptr
ABAP: TYPE i (32-bit signed), TYPE p (BCD), TYPE c/n/x (strings), TYPE REF TO

**Mapping:**
- i8/i16 → TYPE i with MOD masking
- i32 → TYPE i (native)
- ptr → TYPE i (offset into xstring)
- float → Not supported (Z80 target has no FPU)

### 3. Control Flow
LLVM IR: basic blocks + br/switch/invoke
ABAP: IF/WHILE/DO/CASE (structured only, GOTO deprecated)

**Challenge:** LLVM→ABAP must reconstruct structured control flow from arbitrary CFG. This is a solved problem (Relooper algorithm, used by Emscripten for WASM).

### 4. Function Calls
LLVM IR: `call @func(args)` with calling conventions
ABAP: `PERFORM form USING p1 p2 CHANGING p3`

**Mapping:** Direct — each LLVM function → one ABAP FORM.

---

## Deployment via VSP (Vibing Steampunk)

The compiled ABAP REPORT can be deployed to real SAP systems via the VSP MCP server:

```
MinZ source (.nanz/.c/.abap)
    ↓ mz --emit abap
ABAP REPORT source
    ↓ VSP create_object
Create report on SAP system
    ↓ VSP activate
Activate and run
```

VSP provides:
- `create_object` — create ABAP report
- `write_object` — update source code
- `activate_object` — activate (compile on server)
- `run_program` — execute and capture output
- `syntax_check` — validate before activation

This makes the full pipeline: **Nanz → MIR2 → ABAP → SAP system** automated.

---

## Recommendations

### Short Term (Now)
1. **Keep mir2abap as primary ABAP path** — it works
2. **Use `--emit llvm` + `lli` for correctness testing** — verified today
3. **Wire `--emit llvm -o program.ll && clang program.ll -o program`** for native x86_64

### Medium Term (1-2 weeks)
4. **Add `opt -O2` step** before mir2abap: MIR2 → LLVM → opt → roundtrip → ABAP
5. **Integrate VSP deployment**: `mz program.nanz --deploy sap://DEV`
6. **ABAP-specific peephole**: merge sequential WRITE into CONCATENATE+WRITE

### Long Term (research)
7. **Relooper for ABAP**: reconstruct structured control flow from arbitrary LLVM CFG
8. **ABAP-native types**: use TYPE TABLE OF instead of flat xstring for internal tables
9. **ABAP class generation**: emit CL_ classes instead of REPORT+FORM for modern ABAP

---

## Verified Results (2026-03-29)

| Test | Backend | Result |
|------|---------|--------|
| `add(3,4)` | lli (LLVM 18.1.3) | 7 (native x86_64) |
| `6 * 7` | lli | 42 |
| `double(5)` | lli | 10 |
| `leaf→middle chain(5)` | lli | 10 |
| `max_byte(10,20)` | lli | 20 |
| `add(3,4)` | wazero (WASM) | 7 |
| `max_byte(10,20)` | wazero | 20 |
| `abs_diff(3,10)` | wazero | 7 |

All 8 backends (Z80, eZ80, VIR/Z3, QBE, C, WASM, LLVM, ABAP) consume the same MIR2 SSA.
