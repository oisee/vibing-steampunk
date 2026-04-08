# LLVM IR → ABAP: Compilation Strategy

**Date:** 2026-03-29
**Report ID:** 002
**Subject:** Architecture for compiling arbitrary LLVM IR to idiomatic, typed ABAP
**Based on:** WASM→ABAP experience (CLASS g, 12K methods, QuickJS GENERATE rc=0)

---

## Executive Summary

We've proven WASM→ABAP works at scale (QuickJS, 218K lines, GENERATE rc=0). LLVM IR→ABAP can be **better** because LLVM IR preserves type information, function signatures, and named values that WASM's stack machine loses. The target: typed CLASS-METHODS with clean IMPORTING/RETURNING contracts, not FORM/PERFORM soup.

---

## Architecture: One Class Per Module

```
LLVM Module (.ll)
  ↓
CLASS zcl_mod DEFINITION
  " Typed function methods with clean contracts
  CLASS-METHODS add IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
  CLASS-METHODS factorial IMPORTING n TYPE i RETURNING VALUE(rv) TYPE i.
  " Memory (heap)
  CLASS-DATA gv_mem TYPE xstring.
  " Globals
  CLASS-DATA g_counter TYPE i.
ENDCLASS.

CLASS zcl_mod IMPLEMENTATION.
  METHOD add.
    rv = a + b.
  ENDMETHOD.
  METHOD factorial.
    IF n <= 1. rv = 1. RETURN. ENDIF.
    rv = n * factorial( n - 1 ).
  ENDMETHOD.
ENDCLASS.
```

For simple functions (no complex CFG), direct translation. Clean, typed, idiomatic.

---

## Type Mapping

| LLVM IR | ABAP | Notes |
|---------|------|-------|
| `i1` | `TYPE abap_bool` | or TYPE i with 0/1 |
| `i8` | `TYPE int1` | or TYPE i with `MOD 256` masking |
| `i16` | `TYPE int2` | or TYPE i with `MOD 65536` |
| `i32` | `TYPE i` | Native 32-bit signed |
| `i64` | `TYPE int8` | Native 64-bit signed |
| `float` | `TYPE f` | ABAP f is IEEE 754 double — precision loss |
| `double` | `TYPE f` | Native match |
| `ptr` | `TYPE i` | Offset into xstring memory |
| `[N x T]` | `TYPE TABLE OF` or inline | Fixed arrays → structure fields |
| `{T1, T2}` | `TYPES: BEGIN OF...` | LLVM struct → ABAP structure |
| `void` | (no RETURNING) | Method with no return |

### Typed Structures (LLVM struct → ABAP TYPE)

```llvm
%Point = type { i32, i32 }
```
→
```abap
TYPES: BEGIN OF ty_point,
         x TYPE i,
         y TYPE i,
       END OF ty_point.
CLASS-METHODS distance IMPORTING p1 TYPE ty_point p2 TYPE ty_point
                       RETURNING VALUE(rv) TYPE f.
```

This is BETTER than WASM→ABAP where everything is `TYPE i` and structures live in flat memory.

---

## Control Flow: Basic Block Dispatch

### Problem
LLVM IR uses basic blocks with arbitrary branches (CFG). ABAP requires structured control flow (IF/WHILE/DO — no GOTO).

### Solution 1: Relooper (for simple CFGs)
Reconstruct IF/WHILE from the CFG. Emscripten's Relooper algorithm does this for WASM. Works well for most compiled code.

```llvm
entry:
  %cmp = icmp sgt i32 %n, 0
  br i1 %cmp, label %then, label %else
then:
  ret i32 %n
else:
  ret i32 0
```
→
```abap
METHOD abs_positive.
  IF n > 0.
    rv = n.
  ELSE.
    rv = 0.
  ENDIF.
ENDMETHOD.
```

### Solution 2: Block Dispatcher (for complex CFGs, irreducible loops)

Each basic block → a CLASS-METHOD. A dispatcher drives execution:

```llvm
define i32 @complex(i32 %n) {
entry:
  br label %loop
loop:
  %i = phi i32 [0, %entry], [%next, %body]
  %cmp = icmp slt i32 %i, %n
  br i1 %cmp, label %body, label %exit
body:
  %next = add i32 %i, 1
  br label %loop
exit:
  ret i32 %i
}
```
→
```abap
CLASS lcl_complex DEFINITION.
  PUBLIC SECTION.
    CLASS-METHODS run IMPORTING n TYPE i RETURNING VALUE(rv) TYPE i.
  PRIVATE SECTION.
    " SSA values as CLASS-DATA (shared between blocks)
    CLASS-DATA: v_i TYPE i, v_next TYPE i, v_cmp TYPE abap_bool.
    " Block dispatch
    CLASS-DATA: gv_block TYPE string.
    CLASS-METHODS bb_entry.
    CLASS-METHODS bb_loop.
    CLASS-METHODS bb_body.
    CLASS-METHODS bb_exit.
ENDCLASS.

CLASS lcl_complex IMPLEMENTATION.
  METHOD run.
    bb_entry( ).
    WHILE gv_block <> 'exit'.
      CASE gv_block.
        WHEN 'loop'. bb_loop( ).
        WHEN 'body'. bb_body( ).
        WHEN 'exit'. bb_exit( ).
      ENDCASE.
    ENDWHILE.
    rv = v_i.
  ENDMETHOD.

  METHOD bb_entry.
    v_i = 0.        " phi: entry → 0
    gv_block = 'loop'.
  ENDMETHOD.

  METHOD bb_loop.
    v_cmp = xsdbool( v_i < n ).
    IF v_cmp = abap_true.
      gv_block = 'body'.
    ELSE.
      gv_block = 'exit'.
    ENDIF.
  ENDMETHOD.

  METHOD bb_body.
    v_next = v_i + 1.
    v_i = v_next.   " phi: body → v_next
    gv_block = 'loop'.
  ENDMETHOD.

  METHOD bb_exit.
    " rv already set via v_i in run()
    gv_block = 'exit'. " terminate
  ENDMETHOD.
ENDCLASS.
```

### Solution 3: Hybrid (Recommended)

- **Simple functions** (no loops, no phi) → direct METHOD with IF/ELSE
- **Loops** (single back-edge) → METHOD with WHILE/DO
- **Complex CFGs** → Block dispatcher with CASE

The compiler analyzes the CFG complexity and chooses the strategy per function.

---

## Memory Model

### Stack Allocations (alloca)
LLVM `alloca` → ABAP local `DATA`:

```llvm
%x = alloca i32
store i32 42, ptr %x
%v = load i32, ptr %x
```
→
```abap
DATA lv_x TYPE i.
lv_x = 42.
DATA(lv_v) = lv_x.
```

No xstring needed for stack vars! Direct ABAP variables. This is a HUGE win over WASM where everything goes through linear memory.

### Heap: Two Strategies

#### Strategy A: xstring (WASM-compatible, untyped)
Flat byte buffer with offset arithmetic. Same as WASM→ABAP. Fallback for truly untyped memory.

```abap
CLASS-DATA gv_heap TYPE xstring.
CLASS-DATA gv_heap_ptr TYPE i.  " bump allocator
CLASS-METHODS malloc IMPORTING size TYPE i RETURNING VALUE(rv_ptr) TYPE i.
```

#### Strategy B: RTTC + CREATE DATA (typed, GC-managed) ← GAME CHANGER

ABAP has runtime type construction (`cl_abap_structdescr=>create`) and dynamic allocation (`CREATE DATA`). This maps LLVM's type system to actual ABAP types at runtime:

```llvm
%struct.Node = type { i32, ptr }           ; value + next pointer
%n = call ptr @malloc(i64 8)               ; allocate Node
%val = getelementptr %struct.Node, ptr %n, i32 0, i32 0
store i32 42, ptr %val
```
→
```abap
" Build type ONCE (cached in CLASS-DATA)
DATA(go_node_type) = cl_abap_structdescr=>create(
  VALUE #(
    ( name = 'VALUE' type = cl_abap_elemdescr=>get_i( ) )
    ( name = 'NEXT'  type = cl_abap_refdescr=>create(
                              cl_abap_typedescr=>describe_by_name( 'DATA' ) ) )
  ) ).

" Allocate — ABAP GC handles deallocation!
DATA lr_node TYPE REF TO data.
CREATE DATA lr_node TYPE HANDLE go_node_type.
ASSIGN lr_node->* TO FIELD-SYMBOL(<node>).

" getelementptr + store → typed field access
ASSIGN COMPONENT 'VALUE' OF STRUCTURE <node> TO FIELD-SYMBOL(<val>).
<val> = 42.
```

**Why this changes everything:**

| Concern | xstring path | RTTC path |
|---------|-------------|-----------|
| Allocation | Manual bump pointer | `CREATE DATA` (managed) |
| Deallocation | Manual / leak | **ABAP garbage collector** |
| Field access | `mem_ld_i32( offset )` | `ASSIGN COMPONENT 'NAME'` |
| Type safety | None (raw bytes) | **Full ABAP type checking** |
| Debugging | Hex dump | **Named fields with values** |
| Performance | Byte manipulation | **Native ABAP field access** |
| Use-after-free | Possible | **Impossible (GC)** |
| Buffer overflow | Possible | **Impossible (typed)** |

#### Pointer Model with RTTC

LLVM `ptr` → `TYPE REF TO data` (ABAP managed reference):

```llvm
%p = alloca %struct.Point
%q = getelementptr %struct.Point, ptr %p, i32 0, i32 1  ; &p.y
store i32 10, ptr %q
%v = load i32, ptr %q
```
→
```abap
" alloca → CREATE DATA
DATA lr_p TYPE REF TO data.
CREATE DATA lr_p TYPE HANDLE go_point_type.
ASSIGN lr_p->* TO FIELD-SYMBOL(<p>).

" getelementptr → ASSIGN COMPONENT
ASSIGN COMPONENT 'Y' OF STRUCTURE <p> TO FIELD-SYMBOL(<q>).

" store + load → direct access
<q> = 10.
DATA(lv_v) = <q>.
```

No offset arithmetic. No byte swapping. No bounds checking. ABAP runtime handles it all.

#### Type Cache (CLASS-CONSTRUCTOR)

Build RTTC type descriptors once and cache them:

```abap
CLASS lcl_types DEFINITION.
  PUBLIC SECTION.
    CLASS-DATA go_point TYPE REF TO cl_abap_structdescr.
    CLASS-DATA go_node  TYPE REF TO cl_abap_structdescr.
    CLASS-DATA go_array_i32 TYPE REF TO cl_abap_tabledescr.
    CLASS-METHODS class_constructor.
ENDCLASS.

CLASS lcl_types IMPLEMENTATION.
  METHOD class_constructor.
    go_point = cl_abap_structdescr=>create( VALUE #(
      ( name = 'X' type = cl_abap_elemdescr=>get_i( ) )
      ( name = 'Y' type = cl_abap_elemdescr=>get_i( ) )
    ) ).
    go_node = cl_abap_structdescr=>create( VALUE #(
      ( name = 'VALUE' type = cl_abap_elemdescr=>get_i( ) )
      ( name = 'NEXT'  type = cl_abap_refdescr=>create(
                                cl_abap_elemdescr=>get_i( ) ) )
    ) ).
    go_array_i32 = cl_abap_tabledescr=>create(
      cl_abap_elemdescr=>get_i( ) ).
  ENDMETHOD.
ENDCLASS.
```

#### Dynamic Arrays

LLVM arrays → ABAP internal tables (dynamic, bounds-checked):

```llvm
%arr = alloca [100 x i32]
%ptr = getelementptr [100 x i32], ptr %arr, i32 0, i32 42
store i32 7, ptr %ptr
```
→
```abap
DATA lt_arr TYPE STANDARD TABLE OF i WITH DEFAULT KEY.
DO 100 TIMES. APPEND 0 TO lt_arr. ENDDO.
lt_arr[ 43 ] = 7.    " 1-based indexing in ABAP
```

Or for dynamic sizing:
```abap
DATA lr_arr TYPE REF TO data.
CREATE DATA lr_arr TYPE HANDLE lcl_types=>go_array_i32.
ASSIGN lr_arr->* TO FIELD-SYMBOL(<arr>).
```

### Typed Memory (optimization)
For known struct access patterns, bypass xstring entirely:

```llvm
%struct.Person = type { i32, [32 x i8] }
%p = alloca %struct.Person
%age_ptr = getelementptr %struct.Person, ptr %p, i32 0, i32 0
store i32 25, ptr %age_ptr
```
→
```abap
TYPES: BEGIN OF ty_person,
         age  TYPE i,
         name TYPE c LENGTH 32,
       END OF ty_person.
DATA ls_person TYPE ty_person.
ls_person-age = 25.
```

No pointer arithmetic, no xstring. Pure typed ABAP. This is impossible in WASM→ABAP but natural in LLVM→ABAP.

---

## SSA → ABAP Variables

LLVM IR is in SSA form (each value assigned once). Mapping options:

### Option A: One variable per SSA value
```llvm
%1 = add i32 %a, %b
%2 = mul i32 %1, %c
```
→
```abap
DATA(lv_1) = a + b.
DATA(lv_2) = lv_1 * c.
```

Clean but verbose. Many variables.

### Option B: Register-like reuse (with liveness analysis)
Analyze which SSA values are live simultaneously, reuse ABAP variables:

```abap
DATA: lv_t0 TYPE i, lv_t1 TYPE i.
lv_t0 = a + b.
lv_t1 = lv_t0 * c.   " lv_t0 dead after this
rv = lv_t1.
```

### Option C: Inline expressions (best for simple cases)
```abap
rv = ( a + b ) * c.
```

The compiler should use Option C when possible, falling back to A/B for complex expressions.

---

## PHI Nodes

LLVM phi nodes select values based on predecessor block. In ABAP:

```llvm
loop:
  %i = phi i32 [0, %entry], [%next, %body]
```

### Strategy: Assignment at branch source

In `bb_entry`: `v_i = 0.` before `gv_block = 'loop'.`
In `bb_body`: `v_i = v_next.` before `gv_block = 'loop'.`

The phi is resolved by assigning the right value before jumping to the target block.

---

## Function Calls

### Direct calls
```llvm
%r = call i32 @add(i32 3, i32 4)
```
→
```abap
DATA(lv_r) = zcl_mod=>add( a = 3 b = 4 ).
```

Clean, typed, with named parameters. Beautiful ABAP.

### Indirect calls (function pointers)
```llvm
%fp = load ptr, ptr @callback
%r = call i32 %fp(i32 5)
```
→
```abap
" Trampoline dispatch (same pattern as WASM call_indirect)
CASE lv_fp.
  WHEN 0. lv_r = func_0( p0 = 5 ).
  WHEN 1. lv_r = func_1( p0 = 5 ).
  " ...
ENDCASE.
```

---

## Global Variables

```llvm
@counter = global i32 0
```
→
```abap
CLASS-DATA gv_counter TYPE i VALUE 0.
```

For complex globals (arrays, structs):
```llvm
@buffer = global [1024 x i8] zeroinitializer
```
→
```abap
CLASS-DATA gv_buffer TYPE xstring.
" In CLASS-CONSTRUCTOR:
gv_buffer = zcl_wasm_rt=>alloc_mem( 1024 ).
```

---

## Module Architecture: Three Tiers

### Tier 1: Simple Module (< 50 functions, no complex CFG)
One global class, all functions as CLASS-METHODS:

```abap
CLASS zcl_mymod DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    CLASS-METHODS add IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS factorial IMPORTING n TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS class_constructor.
  PRIVATE SECTION.
    CLASS-DATA gv_mem TYPE xstring.
ENDCLASS.
```

### Tier 2: Large Module (50-500 functions)
Split into a main class + helper local classes:

```abap
CLASS zcl_mymod DEFINITION PUBLIC.
  PUBLIC SECTION.
    " Public API (exported functions)
    CLASS-METHODS add IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
ENDCLASS.

" Internal functions in local class
CLASS lcl_internal DEFINITION.
  PUBLIC SECTION.
    CLASS-METHODS f42 IMPORTING p0 TYPE i RETURNING VALUE(rv) TYPE i.
    " ... hundreds of internal functions ...
ENDCLASS.
```

### Tier 3: Massive Module (500+ functions, e.g., QuickJS)
CLASS g pattern (proven at 12K methods):

```abap
" Shared state
CLASS g DEFINITION.
  PUBLIC SECTION.
    CLASS-DATA: s0 TYPE i, s1 TYPE i, ...  " SSA temps
    CLASS-DATA: br TYPE i.                  " control flow
    CLASS-METHODS bb_f42_entry.             " basic blocks
    CLASS-METHODS bb_f42_loop.
    " ... thousands of block methods ...
ENDCLASS.

" Public API wrapper
CLASS zcl_mymod DEFINITION PUBLIC.
  PUBLIC SECTION.
    CLASS-METHODS add IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
ENDCLASS.
```

---

## Initialization: CLASS-CONSTRUCTOR

```abap
CLASS zcl_mymod IMPLEMENTATION.
  METHOD class_constructor.
    " Initialize heap memory
    gv_mem = zcl_wasm_rt=>alloc_mem( 65536 ).
    " Initialize globals
    gv_counter = 0.
    " Initialize data segments (string constants, etc.)
    zcl_wasm_rt=>mem_init(
      iv_off = 1024
      iv_hex = '48656C6C6F'   " "Hello"
      CHANGING cv_mem = gv_mem ).
  ENDMETHOD.
ENDCLASS.
```

CLASS-CONSTRUCTOR runs once, automatically, on first access. No explicit WASM_INIT call needed.

---

## Comparison: WASM→ABAP vs LLVM→ABAP

| Aspect | WASM→ABAP (current) | LLVM→ABAP (proposed) |
|--------|---------------------|----------------------|
| **Types** | Everything TYPE i | Typed: i, int8, f, structures |
| **Functions** | FORM + PERFORM | CLASS-METHODS + direct call |
| **Memory** | All through xstring | alloca → DATA vars, heap → xstring |
| **Structures** | Flat offsets in xstring | ABAP TYPE structures |
| **Function sig** | USING p0 p1 CHANGING rv | IMPORTING a TYPE i RETURNING VALUE(rv) |
| **Initialization** | PERFORM WASM_INIT | CLASS-CONSTRUCTOR (automatic) |
| **Control flow** | DO 1 TIMES + EXIT | IF/WHILE/CASE (structured) |
| **Readability** | Assembly-like | Near-idiomatic ABAP |
| **Debug-ability** | Opaque (s0, s1, gv_br) | Named vars, typed params |

LLVM→ABAP produces **significantly better ABAP** because it preserves the source program's type and structure information.

---

## Implementation Plan

### Phase 1: LLVM IR Parser in Go
- Parse `.ll` text format (or use LLVM C API via cgo)
- Build internal representation: functions, basic blocks, instructions
- Reuse `pkg/wasmcomp` infrastructure (compiler struct, line packer, etc.)

### Phase 2: Simple Function Compiler
- Direct translation for functions without complex CFG
- SSA → ABAP variables
- Arithmetic, comparisons, calls
- Target: `add(3,4) = 7` as a CLASS-METHOD

### Phase 3: Control Flow
- Relooper for structured CFG reconstruction
- Block dispatcher for irreducible graphs
- PHI resolution

### Phase 4: Memory & Types
- alloca → local DATA
- Typed struct access → ABAP structures
- Heap via xstring (fallback)

### Phase 5: Integration
- Deploy via vsp ADT tools
- CLASS-CONSTRUCTOR for initialization
- Multi-module support

---

## Key Insight

WASM→ABAP proved the mechanics work (CLASS-METHODS, shared state, GENERATE at scale). LLVM→ABAP applies the same patterns but with **type preservation** — producing ABAP that a human developer would recognize and maintain. The gap between "compiled output" and "idiomatic code" closes significantly.
