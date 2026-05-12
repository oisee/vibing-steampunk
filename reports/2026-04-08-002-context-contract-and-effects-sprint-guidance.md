# Context, Contracts, and Effects Sprint Guidance

**Date:** 2026-04-08
**Report ID:** 002
**Subject:** Recommended next sprint shape for context compression, code-unit contracts, and side-effect summaries

---

## Executive Direction

The next sprint should **not** jump straight to full transitive side-effect tracing.

The highest-leverage order is:

1. strengthen the shared `CodeUnitContract` model
2. build a method-centric `outer context` bundle on top of existing graph and examples work
3. add **local** effect summaries and state classifications
4. only then add **transitive** effect propagation and purity rollups

This matches the current repo direction:

- `method-signature` already proves contract extraction value
- `examples` already proves upward-context value
- graph docs already point toward generalized `CodeUnitContract`
- graph enrichment guidance already says signals should stay separate from topology

So the next win is not "more edges first". It is a **better context object**.

---

## Why This Order Wins

If the goal is AI/tool context compression, the main question is not only:

- what does this node call?

It is:

- what does this code unit require?
- what state does it touch?
- what are nearby alternatives and sibling entrypoints?
- how is it actually used?
- how risky is it to edit?

That is mostly a **contract + summary + examples** problem, not a raw graph-depth problem.

Full transitive side effects are valuable, but expensive and noisy early:

- confidence drops quickly
- hidden state becomes ambiguous
- cycles complicate results
- token cost grows fast
- the first UI/tool consumers usually need a compact summary, not a whole proof tree

So treat transitive side effects as a second-layer analysis, not the first abstraction.

---

## Recommended Product Shape

Define one new high-level result family:

`MethodContext`

It should bundle:

- self contract
- self local effect summary
- sibling method contracts from the same class
- immediate callees with compact contracts
- caller usage examples
- small topology metrics
- confidence notes

This is the right context-compression artifact for "I am editing this method now".

Suggested output sections:

1. `Self`
   Current method contract, visibility, static/instance, exceptions.
2. `Effects`
   Reads instance state? writes instance state? reads DB? writes DB? transactional statements? dynamic calls?
3. `Siblings`
   Same-class methods, contract only, maybe ranked by similarity or shared references.
4. `Used By`
   Real caller snippets from `examples`.
5. `Calls`
   Direct callees with compact contract/effect badges.
6. `Metrics`
   Fan-in, fan-out, direct standard depth, leaf-ness, dynamic edges, rough complexity.

That gives an agent a much better edit envelope than a plain call graph.

---

## Model Recommendation

Keep three layers distinct:

1. **Topology**
   Existing graph edges like `CALLS`, `REFERENCES`, `READS_CONFIG`.
2. **Contract**
   Stable interface shape of a code unit.
3. **Effects/Signals**
   State access, DB access, transactionality, dynamic behavior, confidence, complexity.

Do **not** overload topology with every semantic signal.

Use:

- graph edges for traversable relationships
- `CodeUnitContract` for public callable shape
- enrichment on node/edge or separate summaries for effect/risk signals

This is already consistent with [pkg/graph/graph.go](/home/alice/dev/vibing-steampunk/pkg/graph/graph.go) and [reports/2026-04-05-007-graph-enrichment-signals-proposal.md](/home/alice/dev/vibing-steampunk/reports/2026-04-05-007-graph-enrichment-signals-proposal.md).

---

## Contract Model: What To Generalize Now

The immediate foundation should be a shared internal model like:

```go
type CodeUnitContract struct {
    UnitKind     string // METHOD, FM, FORM, REPORT
    OwnerType    string // CLAS, INTF, FUGR, PROG
    OwnerName    string
    UnitName     string
    Visibility   string
    Level        string // instance, static
    Params       []ContractParam
    Raising      []string
    RawSignature string
}

type ContractParam struct {
    Name       string
    Direction  string
    Type       string
    Optional   bool
    Default    string
}
```

Then map the existing method-signature extractor into it first.

That work is already implied by [pkg/graph/queries_signature.go](/home/alice/dev/vibing-steampunk/pkg/graph/queries_signature.go) and [reports/2026-04-07-001-claude-2-day-execution-plan.md](/home/alice/dev/vibing-steampunk/reports/2026-04-07-001-claude-2-day-execution-plan.md).

After methods:

1. function module contract
2. FORM contract
3. SUBMIT/report parameter contract

This part is foundational and should happen before broad effect inference.

---

## Effect Model: What To Add Next

After the contract base exists, add a **local** effect summary model.

Suggested shape:

```go
type EffectSummary struct {
    ReadsInstanceState bool
    WritesInstanceState bool
    ReadsClassState bool
    WritesClassState bool
    ReadsDB bool
    WritesDB bool
    CommitsLUW bool
    UsesMemoryID bool
    UsesSAPMemory bool
    CallsTransaction bool
    SubmitsProgram bool
    UsesDynamicCall bool
    RaisesException bool
    ControlFlow ControlFlowMetrics
    Confidence string
    Notes []string
}
```

This should be populated from parser-detectable statements first.

Good first local signals:

- `SELECT FROM`
- `INSERT`, `UPDATE`, `MODIFY`, `DELETE`
- `COMMIT WORK`, `ROLLBACK WORK`
- `CALL TRANSACTION`
- `SUBMIT`
- dynamic FM/program/transaction/transformation calls
- reads/writes to instance attributes
- reads/writes to class attributes

These are high-value and mostly explainable.

Avoid leading with an absolute `pure/not pure` boolean. Use a small classification instead:

- `PURE_LOCAL`
- `READS_STATE`
- `WRITES_STATE`
- `READS_DB`
- `WRITES_DB`
- `TRANSACTIONAL`
- `DYNAMIC`
- `UNKNOWN`

Purity in ABAP is too contextual for a single early boolean.

---

## Outer Context Query: What It Should Actually Do

For a target method, the tool should gather:

1. the target method contract
2. the target method local effect summary
3. sibling methods in the same class with contract-only summaries
4. direct callers via existing examples query
5. direct callees from parser/graph edges
6. compact callee contracts and effect badges
7. a few structural metrics

This is the best first "AI context pack".

Recommended limits:

- siblings: max 8-12
- callees: max 8-12 ranked by custom-first and confidence
- callers/examples: max 5-8
- standard traversal depth: 1 by default
- transitive expansion: off by default

That keeps token cost under control.

---

## What To Compute Transitively Later

Only after local effect summaries are trusted, add an optional transitive rollup:

`TransitiveEffectProfile`

Example fields:

- `EventuallyReadsDB`
- `EventuallyWritesDB`
- `EventuallyCommits`
- `EventuallyCallsTransaction`
- `EventuallyUsesDynamicCall`
- `MaxCustomDepth`
- `MinDistanceToStandard`
- `MayTouchGlobalState`
- `Confidence`

Important rule:

This should be a **derived report**, not the primary stored fact.

Reason:

- it depends on traversal depth
- it depends on pruning policy
- it changes with ranking and scope
- it can be expensive to recompute

So keep local effects as source facts, and transitive effects as query-time summaries.

---

## Siblings, Examples, and "How Deep Is The Hole?"

Your instinct about siblings and upward usage context is right.

For method editing, the most useful context is usually:

- contract of current method
- contracts of sibling methods in the same class
- real caller examples
- shallow callee summaries

Not a full deep graph.

"How deep is the hole?" is still useful, but should be a metric, not the main payload:

- direct callee count
- direct standard callee count
- estimated leaf distance
- dynamic edge count
- max custom depth within small bound

These metrics help judge edit blast radius without flooding the prompt.

---

## Control-Flow Metrics: Worth It?

Yes, but only lightweight ones first.

Good initial metrics:

- statement count
- branch count
- loop count
- exception/early-exit count
- dynamic call count

Possible later metric:

- rough cyclomatic complexity

These are useful because they compress "how tricky is this method?" into a few numbers.
They are better as effect/quality signals than as graph edges.

---

## Concrete Sprint Recommendation

### Phase 1. Shared contract model

Deliver:

- internal `CodeUnitContract`
- adapter from `MethodSignature`
- no broad UX rewrite

Success:

- one common contract result shape exists

### Phase 2. Local effect summary

Deliver:

- statement detectors in parser-driven analysis
- `EffectSummary` for methods
- confidence + notes

Success:

- tool can say "reads DB, writes instance state, dynamic call present"

### Phase 3. Method outer context bundle

Deliver:

- `MethodContext` query/result
- self + siblings + callers + direct callees + metrics

Success:

- editing agent gets compact, high-signal context in one call

### Phase 4. Optional transitive rollup

Deliver:

- depth-limited transitive effect summary
- clearly marked as derived and confidence-sensitive

Success:

- tool can answer "eventually writes DB?" without pretending certainty

---

## What Not To Do Yet

Do not do these first:

- no full purity theorem
- no unrestricted transitive side-effect closure
- no many-new-edge-types explosion
- no giant whole-class summaries by default
- no standard-depth mining deeper than needed for the immediate context pack

Those will cost a lot and compress poorly.

---

## Best Immediate Next Slice

If only one slice is chosen now, it should be:

**`CodeUnitContract` + `EffectSummary` + `MethodContext(Self/Siblings/UsedBy/Calls)`**

That combination will help context compression far more than raw deeper graph traversal alone.

It gives:

- interface shape
- behavior summary
- real usage
- nearby alternatives
- bounded dependency view

That is the right next layer above the current graph MVP.
