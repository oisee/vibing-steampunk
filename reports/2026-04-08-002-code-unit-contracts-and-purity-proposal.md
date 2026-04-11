# ADR: Code Unit Contracts, Purity Analysis, and Context Compression

**Date:** 2026-04-08
**Status:** Proposal
**Context:** graph engine, context compression, AI-assisted development

---

## Problem

When an AI agent (or a human) is working on a method, the useful context is not "the entire class" — it's:

1. **The contract** — what goes in, what comes out, what can go wrong
2. **The siblings** — other methods in the same class, but only their contracts (not their bodies)
3. **The callers** — who calls this method, with what parameters (usage examples)
4. **The callees** — what this method depends on, how deep the rabbit hole goes
5. **The purity** — does this method have side effects? Does it read/write state? Is it transactionally safe?

Currently we have:
- `MethodSignature` — extracts parameters from class definitions (standalone, not graph-connected)
- `Edge.RefDetail` — stores call target name but not parameter flow
- No purity/side-effect tracking at all
- Context compression (`pkg/ctxcomp/`) resolves dependencies but doesn't classify them

## Proposed Model

### 1. Code Unit Contract

Every callable code unit (method, function module, form routine) has a **contract**:

```go
type CodeUnitContract struct {
    // Identity
    Name       string `json:"name"`
    Parent     string `json:"parent"`     // class/FUGR/program
    Type       string `json:"type"`       // METHOD, FUNCTION, FORM
    Visibility string `json:"visibility"` // PUBLIC, PROTECTED, PRIVATE

    // Parameters
    Importing  []Param `json:"importing,omitempty"`
    Exporting  []Param `json:"exporting,omitempty"`
    Changing   []Param `json:"changing,omitempty"`
    Returning  *Param  `json:"returning,omitempty"`
    Raising    []string `json:"raising,omitempty"`

    // Purity (computed from body analysis + transitive propagation)
    Purity     PurityInfo `json:"purity"`

    // Metrics
    Metrics    CodeMetrics `json:"metrics,omitempty"`
}

type Param struct {
    Name     string `json:"name"`
    Type     string `json:"type"`
    Optional bool   `json:"optional,omitempty"`
    Default  string `json:"default,omitempty"`
}
```

### 2. Purity Classification

A method is **pure** if it:
- Does not read from or write to database tables
- Does not depend on instance attributes or class-data
- Does not call external systems (RFC, HTTP)
- Does not modify global state
- All its callees are also pure (transitive)

```go
type PurityInfo struct {
    IsPure        bool     `json:"isPure"`

    // Direct effects (found in this method's body)
    ReadsDB       []string `json:"readsDB,omitempty"`       // tables read (SELECT)
    WritesDB      []string `json:"writesDB,omitempty"`      // tables written (INSERT/UPDATE/DELETE/MODIFY)
    ReadsState    bool     `json:"readsState"`              // uses me->attribute or class-data
    WritesState   bool     `json:"writesState"`             // modifies me->attribute or class-data
    CallsExternal []string `json:"callsExternal,omitempty"` // RFC, HTTP, APC calls
    HasCommit     bool     `json:"hasCommit"`               // COMMIT WORK
    HasRollback   bool     `json:"hasRollback"`             // ROLLBACK WORK
    RaisesExc     bool     `json:"raisesExc"`               // RAISE EXCEPTION

    // Transitive effects (propagated from callees)
    TransitiveDB       []string `json:"transitiveDB,omitempty"`       // tables touched transitively
    TransitiveExternal []string `json:"transitiveExternal,omitempty"` // external calls transitively
    TransitiveCommit   bool     `json:"transitiveCommit"`             // any callee does COMMIT
}
```

### 3. Code Metrics

```go
type CodeMetrics struct {
    Lines            int `json:"lines"`
    Statements       int `json:"statements"`
    CyclomaticComplx int `json:"cyclomaticComplexity"` // IF/CASE/LOOP/WHILE/DO branches
    MaxNesting       int `json:"maxNesting"`            // deepest IF/LOOP nesting
    ParamCount       int `json:"paramCount"`
    CallCount        int `json:"callCount"`             // outgoing calls
    CallerCount      int `json:"callerCount"`           // incoming calls (from graph)
}
```

### 4. Side Effect Detection (Source-Level)

What the parser can extract from any code unit body:

#### 4a. Database Operations

| Pattern | Effect | Category |
|---------|--------|----------|
| `SELECT ... FROM ztable` | Reads DB | ReadsDB |
| `INSERT INTO ztable` | Writes DB | WritesDB |
| `UPDATE ztable` | Writes DB | WritesDB |
| `DELETE FROM ztable` | Writes DB | WritesDB |
| `MODIFY ztable` | Writes DB | WritesDB |

#### 4b. State Access

| Pattern | Effect | Category |
|---------|--------|----------|
| `me->attribute` (read context) | Reads instance state | ReadsState |
| `me->attribute = ...` | Writes instance state | WritesState |
| Class-data access | Reads/writes shared state | ReadsState/WritesState |

#### 4c. Transactional Patterns (LUW)

| Pattern | Effect | Category | Severity |
|---------|--------|----------|----------|
| `COMMIT WORK` | Commits current LUW | HasCommit | HIGH — breaks caller's LUW |
| `COMMIT WORK AND WAIT` | Synchronous commit | HasCommit + Synchronous | HIGH |
| `ROLLBACK WORK` | Rolls back LUW | HasRollback | HIGH |
| `SET UPDATE TASK LOCAL` | Changes update behavior | UpdateTaskLocal | WARN — affects V1/V2 |

#### 4d. Async / Deferred Execution

| Pattern | Effect | Category |
|---------|--------|----------|
| `CALL FUNCTION ... IN UPDATE TASK` | V1 update (deferred to COMMIT) | UpdateTask |
| `CALL FUNCTION ... IN BACKGROUND TASK` | V2 update (async after COMMIT) | BackgroundTask |
| `CALL FUNCTION ... STARTING NEW TASK` | aRFC (async, fire-and-forget) | AsyncRFC |
| `CALL FUNCTION ... DESTINATION` | sRFC (synchronous remote call) | SyncRFC |
| `SUBMIT ... VIA JOB` | Background job scheduling | BackgroundJob |
| `SUBMIT ... AND RETURN` | Submit and continue (non-blocking) | SubmitAndReturn |
| APC: `i_apc_wsp_message=>send` / WebSocket | Push channel send | AsyncPush |

#### 4e. External System Calls

| Pattern | Effect | Category |
|---------|--------|----------|
| `CALL FUNCTION ... DESTINATION 'RFC_DEST'` | RFC to external system | CallsExternal |
| `cl_http_client=>create` / `create_by_url` | HTTP outbound call | CallsExternal |
| `CALL TRANSFORMATION ... SOURCE XML` | External data transformation | TransformIO |

#### 4f. Exception / Control Flow

| Pattern | Effect | Category |
|---------|--------|----------|
| `RAISE EXCEPTION TYPE zcx_...` | Throws exception | RaisesExc |
| `MESSAGE ... TYPE 'E'/'A'/'X'` | Error/abort/exception message | RaisesMsg |
| `LEAVE PROGRAM` / `LEAVE TO TRANSACTION` | Abrupt control transfer | LeavesContext |

**Key insight:** The LUW patterns (4c, 4d) are the most dangerous for transitive analysis. A method that calls `CALL FUNCTION ... IN UPDATE TASK` is not writing to DB *yet* — it's deferring. But whoever calls COMMIT WORK higher up will trigger all deferred writes. This is invisible coupling.

**LUW safety classification:**

| Classification | Meaning | Detection |
|---------------|---------|-----------|
| **LUW-safe** | No COMMIT/ROLLBACK, no UPDATE TASK registrations | No 4c or 4d patterns |
| **LUW-participant** | Registers UPDATE TASK or BACKGROUND TASK but doesn't commit | Has 4d but not 4c |
| **LUW-owner** | Contains COMMIT WORK — owns the transaction boundary | Has COMMIT WORK |
| **LUW-unsafe** | Mixes COMMIT with UPDATE TASK registration from callees | COMMIT + transitive UPDATE TASK |

**Important:** `me->` detection requires knowing we're inside a method body (not a static method). The parser already classifies method-level vs static.

### 5. Transitive Propagation

Once direct effects are computed for each node:

```
For each node N in reverse topological order:
    N.TransitiveDB = N.ReadsDB ∪ N.WritesDB
    N.TransitiveExternal = N.CallsExternal
    N.TransitiveCommit = N.HasCommit
    for each callee C of N:
        N.TransitiveDB = N.TransitiveDB ∪ C.TransitiveDB
        N.TransitiveExternal = N.TransitiveExternal ∪ C.TransitiveExternal
        N.TransitiveCommit = N.TransitiveCommit || C.TransitiveCommit
    N.IsPure = len(N.TransitiveDB)==0 && !N.ReadsState && !N.WritesState
               && len(N.TransitiveExternal)==0 && !N.TransitiveCommit
```

Circular dependencies: if A calls B and B calls A, both get the union of effects. Use SCC (strongly connected component) detection — all nodes in an SCC share the same transitive effects.

---

## Context Compression Model

When an agent asks "give me the context for method X", we build a **context frame**:

### Level 1: The Method Itself
```
CONTRACT: ZCL_ORDER=>VALIDATE_DATES
  IMPORTING iv_start TYPE d, iv_end TYPE d
  RETURNING VALUE(rv_valid) TYPE abap_bool
  RAISING zcx_order_error
  PURITY: pure (no DB, no state, no external)
  METRICS: 12 lines, complexity 3, nesting 2
  BODY: <actual source>
```

### Level 2: Siblings (Contracts Only)
```
SIBLINGS of ZCL_ORDER:
  CREATE_ORDER(IMPORTING is_data TYPE ty_order CHANGING ct_log TYPE ty_log_t)
    purity: IMPURE — writes ZORDERS, ZORDER_ITEMS; commits
  GET_ORDER(IMPORTING iv_id TYPE guid RETURNING VALUE(rs_order) TYPE ty_order)
    purity: reads ZORDERS
  DELETE_ORDER(IMPORTING iv_id TYPE guid RAISING zcx_order_error)
    purity: IMPURE — writes ZORDERS; commits
```

### Level 3: Callers (Usage Examples)
```
CALLED BY:
  ZCL_ORDER_API=>PROCESS_ORDER — passes iv_start=ls_order-start_date, iv_end=ls_order-end_date
  ZTEST_ORDER=>TEST_VALIDATION — passes iv_start='20260101', iv_end='20260401'
```

### Level 4: Callees (Dependency Depth)
```
CALLS:
  (none — this is a pure leaf method)
```

Or for an impure method:
```
CALLS:
  ZCL_ORDER_REPO=>SAVE(is_order) — depth 1, writes ZORDERS
    ZCL_DB_HELPER=>INSERT(iv_table, is_data) — depth 2, writes (dynamic), commits
  ZCL_AUDIT=>LOG(iv_action, iv_object) — depth 1, writes ZAUDIT_LOG
```

### Level 5: Subgraph Summary
```
SUBGRAPH DEPTH: 3
STANDARD SAP DEPS: CL_ABAP_TYPEDESCR, CL_SALV_TABLE (leaf — standard)
TABLES TOUCHED (transitive): ZORDERS (R/W), ZORDER_ITEMS (W), ZAUDIT_LOG (W)
EXTERNAL CALLS: none
MAX COMPLEXITY IN SUBGRAPH: 12 (ZCL_ORDER_REPO=>SAVE)
```

---

## How This Maps to Existing vsp Features

| Feature | Current | With Contracts |
|---------|---------|----------------|
| `vsp slim` | Dead/live by reference count | Dead/live + pure/impure annotation |
| `vsp health` | Tests, ATC, boundaries | + purity violations (impure in pure chain) |
| `vsp boundaries` | Crossing direction | + contract compatibility at boundaries |
| `vsp graph` | Node + edge graph | + contract-annotated edges |
| `vsp context` (ctxcomp) | Source + deps | + contracts + purity + metrics |
| `MCP GetSource` | Source text | + method contract header |

## Implementation Plan

### Phase 1: Side Effect Extraction (Parser-Level)
- Add `ExtractSideEffects(source string) *PurityInfo` to `builder_parser.go`
- Detect: SELECT/INSERT/UPDATE/DELETE/MODIFY, COMMIT/ROLLBACK, RAISE, RFC DESTINATION
- New statement types in abaplint matcher: `InsertDB`, `UpdateDB`, `DeleteDB`, `ModifyDB`, `CommitWork`, `RollbackWork`
- Unit tests with ABAP samples containing each pattern
- **Effort: small, high value, no SAP access needed**

### Phase 2: Contract on Edges
- Attach `MethodSignature` to `Edge.Meta["contract"]` when source is available
- For FM calls: new `ExtractFMSignature` from function module source
- Store contracts on the graph node, not just edge (node = code unit)
- **Effort: medium, builds on existing MethodSignature**

### Phase 3: Metrics Extraction
- `ExtractMetrics(source string) *CodeMetrics` — line count, cyclomatic complexity, nesting depth
- Cyclomatic complexity: count IF, ELSEIF, CASE WHEN, LOOP, WHILE, DO, CATCH, AND, OR
- Max nesting: track indent depth through IF/LOOP/DO blocks
- **Effort: small, pure parser work**

### Phase 4: Transitive Propagation
- `PropagatePurity(g *Graph)` — walk graph in reverse topological order
- SCC detection for cycles (Tarjan's or Kosaraju's — both standard)
- Annotate each node with transitive DB access, external calls, commit status
- **Effort: medium, graph algorithm**

### Phase 5: Context Frame Builder
- `BuildContextFrame(g *Graph, nodeID string, depth int) *ContextFrame`
- Combines: contract + siblings + callers + callees + subgraph summary
- Output formats: text (for AI prompt), JSON (for tooling), MD (for reports)
- Integration with `vsp context` CLI command
- **Effort: medium, composition of previous phases**

---

## What This Enables

### For AI Agents
"Give me the context for ZCL_ORDER=>VALIDATE_DATES" produces a compact, structured context frame that tells the agent:
- What the method accepts and returns (contract)
- What its siblings do (without reading their code)
- How it's used (caller examples)
- Whether it's safe to modify (purity — no side effects to worry about)
- How deep the dependency hole goes (subgraph depth)

### For Humans
`vsp context ZCL_ORDER VALIDATE_DATES --depth 2` shows the same information in terminal. The purity badge is immediately useful: "this method is pure, I can refactor it safely" vs "this method transitively commits to 3 tables through 2 layers of calls, be careful."

### For Quality Gates
- `vsp health` flags: impure method in a class that claims to be a "utility" (no DB access expected)
- `vsp boundaries` shows: boundary crossing with COMMIT on the impure side = transactional coupling
- `vsp slim` enriches: dead method that's also impure = double reason to remove

---

## Decision

Start with Phase 1 (side effect extraction) — it's small, high-value, and the foundation for everything else. The parser patterns are simple (SELECT/INSERT/UPDATE/DELETE/MODIFY/COMMIT/ROLLBACK + RFC DESTINATION). Unit tests can be written without SAP access.

Phase 3 (metrics) is independent and can run in parallel.

Phases 2, 4, 5 build sequentially on Phase 1.

**Key principle:** contracts and purity are **properties of nodes**, not edges. They live on the graph node and propagate through edges. The edge carries the *binding* (which parameters are passed), the node carries the *contract* (what parameters exist and what effects the code has).
