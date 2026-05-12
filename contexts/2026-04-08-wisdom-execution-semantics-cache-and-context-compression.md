# 2026-04-08 Wisdom — execution semantics, cache, and context compression

## What Was Decided

This session clarified the next architectural layer above the current graph MVP.

The key decision is to keep **three layers separate**:

1. topology
2. contracts
3. execution/effect semantics

And then compute **transitive behavior** as a derived report, not as first-class topology.

This is the right direction for AI context compression and for future refactoring/risk analysis.

## Core Architectural Position

### 1. Do not lead with “pure or not”

For ABAP, a single `pure` boolean is too blunt and too easy to overclaim.

Prefer explicit local facts:

- reads DB
- writes DB
- commits LUW
- starts update task
- starts async task
- crosses RFC boundary
- uses APC/session state
- reads/writes instance/class/global state
- uses dynamic calls

Then derive higher-level labels later if needed.

### 2. Runtime behavior should be modeled as execution semantics plus effect profile

The useful split is:

- `CodeUnitContract`
- `ExecutionSemantics`
- `EffectProfile`
- `TransitiveEffectProfile`

This applies not only to methods, but also to:

- function modules
- FORM routines
- reports/program entrypoints
- transaction entrypoints
- APC callbacks

### 3. “Transitive” belongs to bounded propagation, not primary storage

The local facts are durable and explainable.

Transitive facts like:

- eventually writes DB
- eventually commits
- eventually crosses RFC boundary

depend on:

- traversal depth
- graph completeness
- dynamic edge policy
- analyzer version

So they should be query-time summaries or explicitly versioned cached artifacts, not casual permanent truth.

### 4. Methods need a better context pack, not just a deeper graph

The next high-value artifact is a `MethodContext` bundle containing:

- self contract
- self local effects
- sibling method contracts
- direct caller examples
- direct callee summaries
- a few lightweight metrics

That is much more useful to an editing agent than dumping more raw graph depth.

### 5. Keep extraction and propagation in Go

Do not move the primary runtime model to a graph database.

Reason:

- ABAP-aware extraction is the unique part and already lives here
- the first propagation algorithms are small and commodity
- freshness and trust matter more than fancy graph queries

Graph DB export can be a downstream interoperability feature later, not a dependency now.

### 6. SQLite is justified, but only as an opt-in analysis cache

The repo already has `pkg/cache/sqlite.go`, but the needed clarification was scope.

SQLite should cache:

- graph artifacts
- source fingerprints
- contracts
- effect summaries
- transport-derived facts
- analysis provenance

SQLite should **not** become:

- source of truth over SAP
- default raw request archive
- a dump of unconstrained transitive claims

## Documents Added This Session

Primary design documents:

1. [docs/adr/004-execution-semantics-and-effect-profiles.md](/home/alice/dev/vibing-steampunk/docs/adr/004-execution-semantics-and-effect-profiles.md)
2. [docs/adr/005-opt-in-sqlite-analysis-cache.md](/home/alice/dev/vibing-steampunk/docs/adr/005-opt-in-sqlite-analysis-cache.md)
3. [reports/2026-04-08-002-context-contract-and-effects-sprint-guidance.md](/home/alice/dev/vibing-steampunk/reports/2026-04-08-002-context-contract-and-effects-sprint-guidance.md)
4. [reports/2026-04-08-003-execution-semantics-steering-plan.md](/home/alice/dev/vibing-steampunk/reports/2026-04-08-003-execution-semantics-steering-plan.md)

Read those before starting implementation.

## Recommended Implementation Order

1. shared `CodeUnitContract`
2. local `ExecutionSemantics` + `EffectProfile`
3. one `MethodContext` result shape
4. method-first workflow validation
5. one non-method generalization
6. bounded transitive effect rollups
7. opt-in SQLite wiring for graph/facts/provenance

## What To Avoid

- do not introduce many new edge types for semantic facts
- do not claim LUW safety or purity as a hard theorem too early
- do not start with unrestricted transitive propagation
- do not make SQLite the primary engine
- do not persist raw request/response archives by default
- do not try to support methods, reports, transactions, FMs, and APC equally on day one

## Best Immediate Next Slice

If continuing implementation, the best thin slice is:

1. define `CodeUnitContract`
2. define `ExecutionSemantics`
3. define `EffectProfile`
4. define `MethodContext`
5. adapt current method-signature output into the new contract model

That lays the foundation without prematurely wiring all effect extraction.
