# Native Go ABAP Parser — What This Unlocks for VSP

**From:** MinZ/ABAP team
**Date:** 2026-03-17
**Status:** Working prototype, ready for integration discussion

---

## TL;DR

We built a **self-contained ABAP parser in Go** — no Node.js, no SAP system, no external dependencies. It's a single Go package that parses any ABAP source into a structured AST. This could be a game-changer for VSP.

```go
import "github.com/minz/minzc/pkg/abap"

prog, err := abap.ParseWasm(source, "zreport")
// prog.Decls: DATA, PARAMETERS, FORM, CLASS, METHOD, INTERFACE...
// prog.Events: INITIALIZATION, START-OF-SELECTION, AT-SELECTION-SCREEN...
// prog.Params: selection screen parameters
```

**Zero dependencies.** The `@abaplint/core` parser (TypeScript) is compiled to Wasm via Javy/QuickJS, embedded in Go via `go:embed`, executed via wazero (pure Go Wasm runtime). `go build` produces a single binary.

---

## What's Inside

### The Parser

Full ABAP syntax support via [abaplint](https://github.com/abaplint/abaplint) by Lars Hvam Petersen:

- **392 statement types** — DATA, WRITE, IF, WHILE, DO, LOOP, SELECT, PERFORM, CLASS, METHOD, INTERFACE, CREATE OBJECT, RAISE, TRY/CATCH, everything
- **100+ expression types** — field chains, method calls, inline declarations, string templates
- **3-layer AST** — Token → Statement → Structure (hierarchical scoping)
- **ABAP 7.02–7.57** — ECC through S/4HANA Cloud
- **MIT licensed**, no SAP dependency for parsing

### Architecture

```
ABAP source
  → @abaplint/core (TypeScript, bundled via esbuild)
  → QuickJS engine (compiled to Wasm via Javy)
  → 14MB .wasm blob (go:embed in Go binary)
  → wazero runtime (pure Go, zero CGO)
  → JSON AST
  → Go structs (abap.Program, abap.DataDecl, abap.FormDecl, ...)
```

**Parse time:** ~600ms first call (Wasm startup), ~400ms subsequent
**Binary overhead:** ~14MB added to Go binary
**Fallback:** Node.js bridge if Wasm is unavailable

---

## What This Enables for VSP

### 1. Offline LSP-Style Analysis

VSP currently needs a live SAP system for syntax check, find references, etc. With a local parser, you get **offline** capabilities:

| Feature | Today (ADT) | With Local Parser |
|---------|-------------|-------------------|
| Syntax highlighting | SAP round-trip | **Instant, local** |
| Symbol extraction | GetSource + ADT | **Parse locally, no network** |
| Structure outline | ADT Discovery | **AST walk, milliseconds** |
| Error detection | SyntaxCheck ADT call | **abaplint rules, offline** |
| Hover info | FindDefinition ADT | **AST-based, partial offline** |

This means VSP's `mzlsp` (or a new `vsp lsp` mode) could provide IDE features **without SAP connectivity** — useful for code review, offline development, or systems that are slow/unavailable.

### 2. AST-Level Code Translation

The structured AST enables **source-to-source translation**:

```
ABAP AST  →  Nanz (MinZ's systems language)
ABAP AST  →  TypeScript (for testing)
ABAP AST  →  Python (for data analysis)
ABAP AST  →  Go (for microservice extraction)
```

**Already working:** ABAP → HIR → MIR2 → Z80/QBE/native. The HIR (High-level IR) is language-agnostic — any frontend that produces HIR can target any backend.

Practical example: extract a FORM/METHOD from an SAP system via VSP's `GetSource`, parse it locally, translate to Go/TypeScript for unit testing outside SAP.

### 3. ABAP AST-Level Refactoring

With a parsed AST, refactoring becomes structural, not string-based:

- **Rename variable** — find all references in AST, rename correctly (not grep-replace)
- **Extract method** — identify statement block, wrap in FORM/METHOD with correct USING/CHANGING
- **Inline variable** — find single-use DATA, replace with expression
- **Convert FORM → METHOD** — structural transformation preserving semantics
- **Modernize syntax** — `MOVE a TO b` → `b = a`, `IF ... IS INITIAL` → `IF ... IS NOT BOUND`

VSP's `EditSource` tool is already surgical (exact string matching). With AST awareness, it could propose **semantically correct** refactorings that Claude applies via EditSource.

### 4. Context Compression for AI

This is huge for token economy. Instead of sending raw ABAP source to Claude (expensive), you send a **compressed AST summary**:

```
Raw ABAP:     2,500 tokens (50-line method)
AST summary:    200 tokens (structure + key identifiers)
Compression:    12x reduction
```

**Example AST summary:**
```json
{
  "type": "METHOD",
  "name": "GET_MATERIAL",
  "importing": [{"name": "IV_MATNR", "type": "MARA-MATNR"}],
  "returning": {"name": "RS_MARA", "type": "MARA"},
  "calls": ["SELECT SINGLE FROM MARA", "AUTHORITY-CHECK"],
  "raises": ["CX_NOT_FOUND"],
  "complexity": 3
}
```

Claude can understand the method's purpose from 200 tokens instead of 2,500. When it needs details, it requests specific sections. This directly addresses VSP's one-tool-mode token optimization effort.

### 5. Offline abapGit Object Analysis

VSP's GitExport serializes 158 object types. With a local parser, you can:

- **Analyze exported packages** without SAP connectivity
- **Diff at AST level** — semantic diff instead of line-level
- **Validate before import** — check syntax, find missing dependencies
- **Cross-reference** — build dependency graphs from parsed source

---

## Integration Options

### Option A: Import as Go Package (Simplest)

```go
import "github.com/minz/minzc/pkg/abap"

// In VSP's GetSource handler — parse after fetching
source := adtClient.GetSource(objectRef)
prog, _ := abap.ParseWasm(source, objectRef.Name)

// Now you have structured AST for analysis
for _, decl := range prog.Decls {
    switch d := decl.(type) {
    case *abap.FormDecl:
        // Extract FORM signatures for documentation
    case *abap.ClassDecl:
        // Build class hierarchy
    }
}
```

**Cost:** +14MB to vsp binary (Wasm blob)
**Benefit:** Full ABAP parsing, zero external deps

### Option B: Separate Microservice

Run the parser as a separate process / MCP tool:

```bash
echo "REPORT ztest. DATA x TYPE i." | mz --parse-only --format=json
```

**Cost:** Extra binary to distribute
**Benefit:** No size increase to vsp, can update parser independently

### Option C: Shared Library via Wasm

Export the Wasm blob as a shared `.wasm` file, load dynamically:

```go
// Load parser Wasm from disk (not embedded)
wasmBytes, _ := os.ReadFile("/usr/local/lib/abap_parser.wasm")
```

**Cost:** Extra file to manage
**Benefit:** Minimal binary size, updateable

---

## Current Status

| What | Status |
|------|--------|
| ABAP parser (Wasm) | ✅ Working — 13/13 examples parse |
| Go integration | ✅ Working — `abap.ParseWasm()` API |
| Statement coverage | ✅ 392 types (abaplint complete) |
| Expression coverage | ✅ 100+ types |
| OOP support | ✅ CLASS, METHOD, INTERFACE |
| Events | ✅ INITIALIZATION, START-OF-SELECTION, AT-SELECTION-SCREEN |
| Selection screen | ✅ PARAMETERS with defaults |
| AST → HIR lowering | ✅ Working (subset: DATA, WRITE, IF, WHILE, DO, FORM, CLASS) |
| Z80 codegen | ✅ 9/13 to CP/M .com binary |
| QBE native codegen | ✅ ABAP → AMD64 via QBE |
| SQLite integration | ✅ MARA/MAKT with JOIN, GROUP BY |
| Node.js fallback | ✅ Automatic if Wasm fails |

---

## What We're NOT

- **Not a replacement for ADT** — we parse source, we don't connect to SAP
- **Not a full ABAP runtime** — we compile to Z80/native, not ABAP VM
- **Not production-ready for refactoring** — the AST-to-HIR lowering covers a subset; full ABAP semantics is a multi-year effort

But: **the parser is complete** (abaplint handles all ABAP), and the Go integration is solid. Everything above is buildable incrementally.

---

## Next Steps (If Interested)

1. **Quick win:** Use parser in VSP's `SyntaxCheck` fallback (offline mode)
2. **Medium effort:** AST summary for context compression in MCP tools
3. **Larger effort:** Offline refactoring engine using AST + EditSource
4. **Research:** Cross-language translation (ABAP → TypeScript for testing)

Happy to discuss integration, provide the Go package, or do a joint session.

---

*Built during the MinZ ABAP marathon (March 2026). 15 commits, 3 milestones: ABAP frontend + Zork I running + Wasm-embedded parser.*

*Powered by [abaplint](https://github.com/abaplint/abaplint) by Lars Hvam Petersen — the unsung hero of open-source ABAP tooling.*
