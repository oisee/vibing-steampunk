# ADR-004: Execution Semantics and Effect Profiles for Code Units

**Date:** 2026-04-08
**Status:** PROPOSAL
**Context:** Context compression, code-unit contracts, and transitive behavior analysis for ABAP tooling

---

## Summary

This ADR proposes a shared model for describing what an ABAP code unit does beyond its callable contract.

The core decision is:

- keep **topology** in the graph
- keep **contracts** as a shared callable-shape model
- represent runtime behavior as **execution semantics** and **effect profiles**
- compute transitive behavior as a **derived summary**, not as first-class graph topology

This is intended to support:

- AI-oriented context compression
- safer refactoring analysis
- better impact/risk summaries
- method/program/transaction/APC entrypoint understanding

---

## Context

The repository already has useful but separate pieces:

- graph topology in [pkg/graph/graph.go](/home/alice/dev/vibing-steampunk/pkg/graph/graph.go)
- method contract extraction in [pkg/graph/queries_signature.go](/home/alice/dev/vibing-steampunk/pkg/graph/queries_signature.go)
- real caller examples in [pkg/graph/queries_examples.go](/home/alice/dev/vibing-steampunk/pkg/graph/queries_examples.go)
- context-compression guidance toward a unified contract model in [docs/analysis-refactoring-guide.md](/home/alice/dev/vibing-steampunk/docs/analysis-refactoring-guide.md)
- topology-vs-enrichment separation in [reports/2026-04-05-007-graph-enrichment-signals-proposal.md](/home/alice/dev/vibing-steampunk/reports/2026-04-05-007-graph-enrichment-signals-proposal.md)

What is still missing is a stable internal model for:

- LUW behavior
- DB effects
- state access
- async/update-task behavior
- RFC/session/APC boundaries
- transitive “eventually” properties

Without this, agents can see who calls whom, but not what the execution path implies.

---

## Problem

For code editing and analysis, users often need answers like:

- does this method or program commit?
- is this LUW-safe?
- does it only read DB, or can it write?
- does it enter update task or async execution?
- does it cross RFC/session boundaries?
- is this APC callback code?
- what does it eventually do through callees?

These questions apply not only to methods, but also to:

- function modules
- FORM routines
- reports/programs
- transaction entrypoints
- APC handlers/callbacks

The current graph can represent call relationships, but it does not have a shared model for these runtime semantics.

---

## Decision

Introduce two new internal model families:

1. `ExecutionSemantics`
2. `EffectProfile`

And one derived transitive family:

3. `TransitiveEffectProfile`

These should be attachable to any executable code unit.

### Executable code units in scope

- class method
- interface method
- function module
- FORM routine
- report/program entrypoint
- transaction entrypoint
- APC callback/handler method

---

## Model

### 1. Contract stays separate

Callable shape belongs in a contract model such as:

```go
type CodeUnitContract struct {
    UnitKind    string
    OwnerType   string
    OwnerName   string
    UnitName    string
    Visibility  string
    Level       string
    Params      []ContractParam
    Raising     []string
    RawSignature string
}
```

This describes the public or callable surface only.

### 2. Execution semantics

Execution semantics describe the runtime mode or boundary characteristics of a code unit.

```go
type ExecutionSemantics struct {
    Contexts []string
    MayRunInDialog bool
    MayRunInUpdateTask bool
    MayRunInBackground bool
    MayRunAsync bool
    MayCrossRFCBoundary bool
    MayUseAPCSession bool
    Confidence string
    Notes []string
}
```

Representative context values:

- `DIALOG`
- `UPDATE_TASK`
- `BACKGROUND`
- `RFC`
- `APC_CALLBACK`
- `TRANSACTION_ENTRY`
- `REPORT_ENTRY`
- `UNKNOWN`

### 3. Effect profile

Effect profile describes what the code unit itself appears to do locally.

```go
type EffectProfile struct {
    ReadsInstanceState bool
    WritesInstanceState bool
    ReadsClassState bool
    WritesClassState bool
    ReadsGlobalState bool
    WritesGlobalState bool

    ReadsDB bool
    WritesDB bool

    CommitsLUW bool
    RollsBackLUW bool

    StartsUpdateTask bool
    StartsAsyncTask bool
    CallsRFC bool
    CallsTransaction bool
    SubmitsProgram bool
    UsesDynamicCall bool
    UsesAPCState bool

    RaisesException bool

    ControlFlow ControlFlowMetrics
    Confidence string
    Notes []string
}
```

### 4. Transitive effect profile

Transitive effects are derived by bounded traversal, not stored as primitive syntax facts.

```go
type TransitiveEffectProfile struct {
    EventuallyReadsDB bool
    EventuallyWritesDB bool
    EventuallyCommitsLUW bool
    EventuallyRollsBackLUW bool
    EventuallyStartsUpdateTask bool
    EventuallyRunsAsync bool
    EventuallyCrossesRFCBoundary bool
    EventuallyUsesAPCSession bool

    MayTouchStateTransitively bool

    MaxCustomDepth int
    MinDistanceToStandard int
    DynamicEdgeCount int

    Confidence string
    Notes []string
}
```

---

## Why This Split

This split matters because these concerns are different:

- topology answers reachability
- contracts answer interface shape
- execution semantics answer runtime mode
- local effects answer what the unit itself does
- transitive effects answer what paths below it may eventually do

If all of this is collapsed into raw edges, the graph becomes noisy and less trustworthy.

---

## LUW Safety

The tool should not claim absolute LUW safety too early.

Instead, expose a classification derived from local plus bounded transitive facts:

- `SAFE_LOCAL`
- `COMMITS_LOCAL`
- `COMMITS_TRANSITIVE`
- `UPDATE_TASK_LOCAL`
- `UPDATE_TASK_TRANSITIVE`
- `ASYNC_LOCAL`
- `ASYNC_TRANSITIVE`
- `UNKNOWN`

This is more honest than a single binary `is_luw_safe`.

---

## Purity

Do not lead with a single `pure` boolean.

In ABAP, “pure” is often too ambiguous because of:

- DB reads
- hidden instance/class/global state
- update-task scheduling
- RFC/session boundaries
- dynamic calls

Instead, expose explicit effect categories and allow a higher-level classifier later.

Possible later presentation labels:

- `PURE_LOCAL`
- `READ_ONLY`
- `STATEFUL`
- `DB_WRITER`
- `TRANSACTIONAL`
- `ASYNC_BOUNDARY`
- `UNKNOWN`

---

## Transactions and Programs

The same model should apply to reports and transactions.

Important nuance:

- a transaction code is primarily an entrypoint
- the substantive effects usually belong to the program/class flow behind it

Therefore:

- the transaction node contributes entry semantics
- the report/program entry contributes direct local effects
- deeper path behavior is handled by transitive summaries

This avoids overloading TCODE nodes with behavior they do not directly own.

---

## APC

APC should be modeled explicitly.

It is not merely “async”.
It is event-driven, callback-oriented, and sessionful.

If a method is known to be an APC handler or callback, that should appear in `ExecutionSemantics`.

This matters for AI context because APC code often:

- depends on session or connection state
- has different lifecycle expectations
- is not equivalent to a plain request/response method

---

## Sources of Evidence

### Local syntax evidence

These are appropriate first local detectors:

- `SELECT`
- `INSERT`
- `UPDATE`
- `MODIFY`
- `DELETE`
- `COMMIT WORK`
- `ROLLBACK WORK`
- `CALL FUNCTION ... IN UPDATE TASK`
- `CALL FUNCTION ... STARTING NEW TASK`
- `DESTINATION`
- `CALL TRANSACTION`
- `SUBMIT`
- dynamic calls

### Structural evidence

- method or report ownership
- transaction entry association
- APC handler/interface implementation

### Derived evidence

- bounded traversal of callees
- custom-only or mixed custom/standard depth metrics
- aggregated flags over reachable callees

---

## Consequences

### Positive

- better context compression for agents
- richer edit risk summaries
- shared semantics across methods, programs, FMs, and APC handlers
- more honest modeling of LUW/async/RFC behavior
- clear path to transitive analysis without polluting topology

### Negative

- more internal model complexity
- some semantics remain heuristic
- confidence signaling becomes mandatory
- transitive analysis can be expensive if not bounded

---

## Implementation Guidance

Recommended execution order:

1. define shared contract model
2. define `ExecutionSemantics` and `EffectProfile`
3. implement local detectors only
4. expose a method-centric context bundle
5. add bounded transitive rollups later

Do not start with:

- unrestricted transitive propagation
- many new edge types
- absolute purity claims

---

## Status Rationale

This ADR is marked `PROPOSAL` because:

- the model is clear enough to guide implementation
- exact field names may still change
- parser integration and consumer shape are not implemented yet

The decision should become `ACCEPTED` once:

- one internal consumer uses the shared models
- local effect extraction exists for at least one code-unit kind
- a context result bundles contract plus effect semantics successfully
