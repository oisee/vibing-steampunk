# Method-Level Surgery — Read & Edit Individual ABAP Methods

**Date:** 2026-03-19
**Report ID:** 002
**Subject:** Method-level source operations with scoped context compression
**Related Documents:** [2026-03-19-001 Token Efficiency Sprint](2026-03-19-001-token-efficiency-sprint.md), [2026-01-06-001 Method-Level Source Operations](2026-01-06-001-method-level-source-operations.md)

---

## Executive Summary

vsp supports **method-level read and write operations** for ABAP classes. Instead of pulling or pushing entire class sources (often 500–2000 lines), the AI works with individual `METHOD...ENDMETHOD` blocks (~20–80 lines). Combined with scoped context compression, this delivers **6–20x token reduction** per operation.

This feature works across all three modes: focused, expert, and hyperfocused.

---

## 1. Method-Level Read

### API

```
# Granular mode
GetSource(object_type="CLAS", name="ZCL_CALCULATOR", method="FACTORIAL")

# Hyperfocused mode
SAP(action="read", target="CLAS ZCL_CALCULATOR", params={"method": "FACTORIAL"})
```

### Implementation

**File:** `pkg/adt/workflows_source.go`, line 63

```go
case "CLAS":
    if opts.Method != "" {
        return c.GetClassMethodSource(ctx, name, opts.Method)
    }
```

**Flow:**
1. `GetClassMethods(ctx, className)` — calls ADT `objectstructure` endpoint to get method list with line boundaries
2. `GetClassSource(ctx, className)` — fetches full class source
3. Extract lines from `ImplementationStart` to `ImplementationEnd`
4. Return only the `METHOD...ENDMETHOD` block

### Token Impact

| Class Size | Full Source | Method Only | Ratio |
|:----------:|:----------:|:-----------:|:-----:|
| 200 lines  | ~200 tokens | ~30 tokens  | **6.7x** |
| 500 lines  | ~500 tokens | ~40 tokens  | **12.5x** |
| 1000 lines | ~1000 tokens | ~50 tokens | **20x** |
| 2000 lines | ~2000 tokens | ~60 tokens | **33x** |

---

## 2. Method-Level Write

### API

```
# Granular mode
WriteSource(object_type="CLAS", name="ZCL_CALCULATOR", method="FACTORIAL",
            source="  METHOD factorial.\n    ...\n  ENDMETHOD.")

# Hyperfocused mode
SAP(action="edit", target="CLAS ZCL_CALCULATOR", params={
    "method": "FACTORIAL",
    "source": "  METHOD factorial.\n    ...\n  ENDMETHOD."
})
```

### Implementation

**File:** `pkg/adt/workflows_source.go`, lines 684–1027

**Function:** `writeClassMethodUpdate(ctx, className, methodName, methodSource, transport)`

**Flow:**
1. `GetClassMethods()` — get method boundaries via ADT objectstructure
2. Find the target method by name (case-insensitive)
3. Validate: method exists, has implementation lines (not abstract)
4. `GetClassSource()` — fetch full current source
5. Split into lines, replace `ImplementationStart..ImplementationEnd` with new source
6. `SyntaxCheck()` — validate the **full reconstructed source** (catches cross-method issues)
7. `LockObject()` → `UpdateSource()` → `UnlockObject()` → `Activate()`
8. Return result with method name in response

### Key Design Decisions

- **Full-source syntax check**: Even though we only changed one method, syntax check runs on the entire reconstructed class. This catches issues like renamed types that break other methods.
- **Server-side reconstruction**: The AI sends only the method block. vsp handles fetching, splicing, and pushing the full source. This means the LLM never sees or manages the full class source.
- **Line boundary detection via ADT**: Method boundaries come from SAP's `objectstructure` endpoint (same as SE24), not from regex parsing. This handles edge cases like methods with macros, comments, and nested blocks.

### Error Handling

| Scenario | Behavior |
|----------|----------|
| Method not found | Returns error: "Method X not found in class Y" |
| Abstract method (no impl) | Returns error: "Method X has no implementation lines" |
| Syntax error in new code | Returns syntax errors, source NOT saved |
| Lock conflict | Returns lock error, source NOT saved |
| Activation failure | Source saved but flagged as inactive |

---

## 3. Scoped Context Compression

### The Key Insight

When reading at method level, the **context compression scopes to the method's code**, not the entire class.

**File:** `internal/mcp/handlers_source.go`, line 195

```go
// source = method block only (when method param is set)
result, err := compressor.Compress(ctx, source, name, objectType)
```

The compressor receives only the `METHOD...ENDMETHOD` block. Its regex scan finds only the types/interfaces/FMs referenced **in that method**. This means:

| Scenario | Dependencies Found | Context Size |
|----------|:------------------:|:------------:|
| Full class (20 methods, 15 deps) | 15 | ~800 tokens |
| Single method (uses 3 deps) | 3 | ~200 tokens |

### Why This Matters

A class like `ZCL_TRAVEL_PROCESSOR` might have 15 dependencies total across all its methods. But when the AI is editing `METHOD calculate_price`, it only needs to know about the 3 types used in that method. The scoped context gives the AI exactly what it needs — no noise from unrelated dependencies.

### Example: Live Test on A4H

```
SAP(action="read", target="CLAS ZCL_ZOZIK_CALCULATOR", params={"method": "FACTORIAL"})

Response (10 lines, no prologue):
  METHOD factorial.
    IF iv_n < 0.
      RAISE EXCEPTION TYPE cx_sy_arithmetic_overflow.
    ENDIF.
    IF iv_n <= 1.
      rv_result = 1.
    ELSE.
      rv_result = iv_n * factorial( iv_n - 1 ).
    ENDIF.
  ENDMETHOD.
```

No context prologue because the only dependency (`CX_SY_ARITHMETIC_OVERFLOW`) is filtered as SAP standard (`CX_SY_*` prefix). The system correctly determined this method needs zero external context.

---

## 4. End-to-End Token Analysis

### Scenario: AI Edits a Method in a Large Class

**Class:** 800 lines, 12 methods, 10 external dependencies
**Target method:** `CALCULATE_TOTAL`, 45 lines, uses 2 custom dependencies

| Step | Without Method-Level | With Method-Level | Savings |
|------|:--------------------:|:-----------------:|:-------:|
| Read source | 800 tokens | 45 tokens | 17.8x |
| Context prologue | 500 tokens (10 deps) | 120 tokens (2 deps) | 4.2x |
| AI processes | 1,300 tokens | 165 tokens | **7.9x** |
| Write back | 800 tokens (full) | 45 tokens (method) | 17.8x |
| **Total round-trip** | **2,100 tokens** | **210 tokens** | **10x** |

### Combined with Hyperfocused Mode

| Component | Expert Mode | Hyperfocused + Method | Ratio |
|-----------|:-----------:|:---------------------:|:-----:|
| Schema overhead | 40,000 | 200 | 200x |
| Read + context | 1,300 | 165 | 7.9x |
| Edit round-trip | 2,100 | 210 | 10x |
| Syntax check (LSP) | 200 | 0 (free) | ∞ |
| **Total operation** | **43,600** | **575** | **75.8x** |

---

## 5. Supported Object Types

Method-level operations are currently supported for:

| Object Type | Read Method | Write Method | Context Scoped |
|:-----------:|:----------:|:------------:|:--------------:|
| **CLAS** | ✅ | ✅ | ✅ |
| PROG | — | — | — |
| INTF | — | — | — |
| FUNC | — | — | — |

Classes are the primary use case since they are the largest ABAP objects (often 500–3000 lines) with many methods.

---

## 6. Files

| Purpose | Path | Lines |
|---------|------|:-----:|
| GetSource (method routing) | `pkg/adt/workflows_source.go:44` | — |
| GetClassMethodSource | `pkg/adt/client.go` | ~40 |
| writeClassMethodUpdate | `pkg/adt/workflows_source.go:906` | 121 |
| GetClassMethods | `pkg/adt/client.go` | ~80 |
| Handler (read routing) | `internal/mcp/handlers_source.go:32` | — |
| Handler (write routing) | `internal/mcp/handlers_source.go:70` | — |
| Universal route | `internal/mcp/handlers_source.go:17` | — |
| Context compression | `internal/mcp/handlers_source.go:195` | — |

---

## 7. Release Notes

This feature was originally introduced in **v2.21.0** (2026-01-06) for read/write. The scoped context compression was added in **v2.28.0** (2026-03-18) as part of the `ctxcomp` package. The hyperfocused mode routing was added in **v2.29.0** (2026-03-19).
