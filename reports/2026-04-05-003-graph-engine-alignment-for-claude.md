# Graph Engine Alignment for Claude

**Date:** 2026-04-05
**Report ID:** 003
**Subject:** How to extend existing dependency capabilities without building a second parallel graph stack
**Related:** `reports/2026-04-05-002-graph-engine-design.md`, `reports/2025-12-02-003-graph-and-api-surface-design-overview.md`, `reports/2025-12-02-007-graph-traversal-implementation-plan.md`

---

## Executive Point

The repo is **not** starting from zero.

It already has:
- a static call graph client via ADT
- a CLI fallback to `WBCROSSGT` and `CROSS`
- a package dependency classifier via `deps`
- a multi-layer dependency analyzer in `pkg/ctxcomp`
- early dynamic graph comparison helpers from trace/debug data

So the next step should **not** be "invent a general graph platform first".

The right next step is:

**Unify existing dependency layers behind one reusable Go package and ship one high-value analyzer first: package boundary analysis with whitelist rules.**

---

## What Already Exists

### 1. Static call graph via ADT

Code:
- [`pkg/adt/client.go`](/home/alice/dev/vibing-steampunk/pkg/adt/client.go)

Capabilities:
- `GetCallGraph`
- `GetCallersOf`
- `GetCalleesOf`
- `FlattenCallGraph`
- `AnalyzeCallGraph`

Implication:
- There is already a structured graph-shaped API for systems where `/sap/bc/adt/cai/callgraph` is available.

### 1b. Other ADT-native dependency surfaces already matter

Code:
- [`pkg/adt/codeintel.go`](/home/alice/dev/vibing-steampunk/pkg/adt/codeintel.go)
- [`pkg/adt/cds.go`](/home/alice/dev/vibing-steampunk/pkg/adt/cds.go)
- [`pkg/adt/cds_tools.go`](/home/alice/dev/vibing-steampunk/pkg/adt/cds_tools.go)

Capabilities:
- `FindReferences` / where-used style lookups
- CDS dependency expansion
- CDS impact-oriented analysis

Implication:
- The dependency story is already broader than just `/cai/callgraph`.
- A future graph builder should treat ADT-native references and CDS expansion as first-class inputs, not side tools.

### 2. CLI graph fallback via `WBCROSSGT` and `CROSS`

Code:
- [`cmd/vsp/cli_extra.go`](/home/alice/dev/vibing-steampunk/cmd/vsp/cli_extra.go)

Capabilities:
- `vsp graph`
- fallback from ADT call graph to SQL queries on `WBCROSSGT` and `CROSS`
- transaction-to-program resolution via `TSTC`

Implication:
- A graph-like feature already exists in product surface.
- Today it is mostly a CLI printer, not a reusable graph engine.

### 3. Package dependency classification already exists

Code:
- [`cmd/vsp/cli_deps.go`](/home/alice/dev/vibing-steampunk/cmd/vsp/cli_deps.go)

Capabilities:
- `vsp deps`
- classify references as internal / external / SAP standard
- package load from `TADIR`

Implication:
- The exact user problem "does this package cross boundaries?" is already partially implemented.
- The gap is not concept but reusability, precision, and richer policy rules.

### 4. Multi-layer dependency analysis exists in `pkg/ctxcomp`

Code:
- [`pkg/ctxcomp/analyzer.go`](/home/alice/dev/vibing-steampunk/pkg/ctxcomp/analyzer.go)

Layers:
- regex
- parser
- SCAN ABAP-SOURCE
- CROSS/WBCROSSGT
- ADT where-used

Important design insight already encoded there:
- parser/source-based analysis is the authority
- `CROSS/WBCROSSGT` can be stale
- multiple sources should be merged with confidence, not treated as a single truth source

Implication:
- This is already the seed of a serious graph/data-fusion layer.

### 4b. Static load dependencies are a separate source and should be modeled explicitly

Candidate SAP source:
- `D010INC`

Why it matters:
- `CROSS` and `WBCROSSGT` describe references and usage relations
- ADT call graph describes callable relationships
- `D010INC` describes include/load composition of programs and pools

Practical implication:
- This is useful for answering a different question:
  - not just "what does this object call?"
  - but also "what source units get pulled in / loaded / compiled together?"

This is especially valuable for:
- reports with many includes
- class pools and test includes
- function groups and generated include structures

For the graph engine, this should likely become a separate edge type:
- `LOAD`
- or `INCLUDES`

### 5. Dynamic overlay primitives already exist

Code:
- [`pkg/adt/client.go`](/home/alice/dev/vibing-steampunk/pkg/adt/client.go)
- [`abap/src/zadt_vsp/README.md`](/home/alice/dev/vibing-steampunk/abap/src/zadt_vsp/README.md)

Capabilities:
- `ExtractCallEdgesFromTrace`
- `CompareCallGraphs`
- static vs actual execution comparison

Implication:
- There is already a path toward static + dynamic graph overlay.
- This is relevant for "what really loads/runs" beyond static dependencies.

---

## Answer to the New Question

> Is there already some program that analyzes static or dynamic loading of other source units?

Yes, but fragmented:

### Static
- `GetCallGraph` via ADT
- `vsp graph` fallback via `WBCROSSGT` and `CROSS`
- `vsp deps` for package-level dependency classification
- `pkg/ctxcomp` for source-level and SAP-index-based dependency extraction
- SAP include/load metadata such as `D010INC` for composition/load relationships

### Dynamic or quasi-dynamic
- trace-to-edge extraction in `pkg/adt/client.go`
- static-vs-actual comparison already modeled
- debugger/tracing roadmap in `abap/src/zadt_vsp/README.md`

What is **missing** is not raw capability.

What is missing is a **single reusable graph/domain package** that turns these scattered sources into:
- nodes
- edges
- provenance/confidence
- edge kinds for different semantics (`CALLS`, `USES_TYPE`, `WHERE_USED`, `LOADS`, `CDS_DEP`)
- package policy verdicts
- impact/path queries

---

## Main Risk

If Claude starts `pkg/graph` from scratch and ignores existing `graph`, `deps`, and `ctxcomp` behavior, the repo will end up with:
- one CLI graph implementation
- one deps implementation
- one ctxcomp dependency implementation
- one new graph engine

That would be a fourth parallel dependency stack.

This should be avoided.

---

## Recommended MVP

Do **not** start with:
- custom graph query DSL
- clustering
- visualization server
- generic platform abstractions

Start with one thin shared package and one analyzer:

### Package target

`pkg/graph/`

Initial responsibilities:
- canonical `Node`
- canonical `Edge`
- source/provenance metadata
- graph builder adapters from existing sources
- boundary analysis

### First shipped use case

`CheckPackageBoundaries(rootPackage, whitelist)`

Output categories:
- `SAME_PACKAGE`
- `STANDARD`
- `ALLOWED_CUSTOM`
- `VIOLATION`
- optionally `UNKNOWN`

This directly serves the confirmed user question:

> Does `PROG` stay within its package, or only hit standard SAP, while allowing selected shared Z packages?

---

## Implementation Advice

### 1. Reuse before inventing

Pull logic from:
- `cmd/vsp/cli_deps.go`
- `cmd/vsp/cli_extra.go`
- `pkg/ctxcomp/analyzer.go`

Do not leave the intelligence trapped in CLI commands.

### 2. Track source provenance per edge

Each edge should know:
- source: `ADT_CALL_GRAPH`, `WHERE_USED`, `WBCROSSGT`, `CROSS`, `D010INC`, `PARSER`, `TRACE`, `CDS_DEP`
- confidence
- object/include origin if available
- semantic kind

Recommended semantic kinds:
- `CALLS`
- `REFERENCES`
- `LOADS`
- `CONTAINS_INCLUDE`
- `DEPENDS_ON_CDS`

Without this, debugging false positives later will be painful.

### 3. Keep ADT graph and SQL graph separate at collection time

Do not collapse them too early.

Reason:
- ADT graph is structured but may be unavailable
- `WBCROSSGT/CROSS` is broad but noisier
- `D010INC` captures composition/load structure rather than call semantics
- parser/source analysis is freshest but local in scope

Merge only in a builder layer.

### 4. Make package policy first-class

Whitelist logic is not a formatter concern.

It should live in analysis code with explicit rules:
- allow SAP standard
- allow same package
- allow configured package patterns
- flag custom cross-package violations

### 5. Delay caching until correctness is real

Cache is valuable, but correctness matters first.

The repo already contains enough moving parts that premature caching will mask data-quality bugs.

### 6. Model include/load analysis as a separate subgraph

Do not force include composition into the same semantics as call edges.

Reason:
- a program including another source is not the same as invoking it
- class pools, test includes, and function group includes are structural dependencies
- package-boundary analysis may want to count them differently from callable dependencies

Recommendation:
- keep load/include edges queryable
- let analyzers choose whether they participate in a boundary verdict

---

## Proposed Delivery Order

### Slice 1: Internal package refactor
- extract reusable dependency collection from `graph` and `deps` commands
- define canonical graph node/edge structs
- define provenance model
- add source adapters for `D010INC` and ADT-native reference/dependency APIs

Estimate: 4-6h

### Slice 2: Boundary analyzer MVP
- root package + whitelist
- text + JSON report
- package/object entry points

Estimate: 5-7h

### Slice 3: Impact/path queries
- transitive callers/callees
- shortest path or bounded path search

Estimate: 4-6h

### Slice 4: Dynamic overlay
- import trace edges
- compare static graph vs actual execution

Estimate: 4-5h

Total realistic path:
- **13-24h** depending on how much refactoring of existing commands is done properly

This is close to the earlier estimate, but now with a safer sequencing.

---

## Recommendation to Claude

Build `pkg/graph` as the shared dependency core, but treat it as a unification effort, not a greenfield engine.

Priority order:
1. consolidate current static dependency collection
2. ship package boundary analysis
3. add impact/path traversal
4. only then consider clusters, hotspots, or a query DSL

If forced to choose between "cool graph features" and "boundary verdicts people can act on", choose boundary verdicts.
