# Graph Guide

VSP now has a graph-oriented analysis layer for questions that are awkward to answer with grep or a plain call graph:

- what usually changes together with this object?
- who depends on this object?
- which programs read this TVARVC variable?

This guide explains what is available today, which data sources power it, and where the current limits are.

## What You Can Do Today

Current graph-MVP capabilities:

- transport-based co-change analysis
- reverse dependency impact analysis
- config usage analysis for `TVARVC`

Current entry points:

- CLI: `vsp graph co-change <type> <name>`
- MCP: `SAP(action="analyze", params={"type":"co_change", ...})`
- MCP: `SAP(action="analyze", params={"type":"impact", ...})`

The core implementation lives in [`pkg/graph`](/home/alice/dev/vibing-steampunk/pkg/graph).

## Mental Model

The graph uses one canonical model:

- nodes represent ABAP objects, transport requests, or config variables
- edges point from dependent to dependency

Examples:

- `PROG:ZREPORT -> CALLS -> CLAS:ZCL_FOO`
- `CLAS:ZCL_FOO -> IN_TRANSPORT -> TR:A4HK900123`
- `PROG:ZREPORT -> READS_CONFIG -> TVARVC:Z_FLAG`

That direction matters:

- co-change is computed from shared `IN_TRANSPORT` edges
- impact walks backward through `InEdges`
- config usage walks backward from `TVARVC:*` through `READS_CONFIG`

## Data Sources

Current MVP data sources are mixed on purpose.

### 1. Transport metadata

Used for co-change:

- `E070` for request/task hierarchy
- `E071` for object membership

Tasks are collapsed into parent requests during graph build, so co-change is computed at request level.

### 2. Static repository cross-references

Used for code-level impact:

- `WBCROSSGT`
- `CROSS`

These are useful as a reverse-dependency backbone, but they are still static metadata, not runtime truth.

### 3. Source parser

The graph core already contains parser-based builders and query support, but the first exposed CLI/MCP slices still lean mostly on transport and SQL-backed acquisition.

Parser-based augmentation matters for cases like:

- local procedural hops
- `PERFORM ... IN PROGRAM ...`
- include-local edges that cross-reference tables do not express well

### 4. Heuristic config evidence

Used for `TVARVC`:

- variable nodes are canonical
- `READS_CONFIG` edges are marked with `SourceTVARVC_CROSS`
- confidence is carried in edge metadata

This is intentionally modeled as heuristic evidence, not exact repository truth.

## CLI Usage

### Co-change

```bash
vsp -s a4h graph co-change CLAS ZCL_PRICING
vsp -s a4h graph co-change CLAS ZCL_PRICING --top 10
vsp -s a4h graph co-change PROG ZREPORT --format json
```

What it does:

1. find transports containing the target object
2. resolve request/task hierarchy from `E070`
3. fetch sibling objects from all related request/task entries in `E071`
4. rank co-occurring objects by shared transport count

Good for:

- hidden change bundles
- upgrade wave planning
- spotting objects that usually move together

## MCP Usage

### Co-change

```json
SAP(action="analyze", params={
  "type": "co_change",
  "object_type": "CLAS",
  "object_name": "ZCL_PRICING",
  "top_n": 10
})
```

### Impact

```json
SAP(action="analyze", params={
  "type": "impact",
  "object_type": "CLAS",
  "object_name": "ZCL_FOO",
  "max_depth": 3
})
```

Optional filter:

```json
SAP(action="analyze", params={
  "type": "impact",
  "object_type": "CLAS",
  "object_name": "ZCL_FOO",
  "max_depth": 2,
  "edge_kinds": "CALLS"
})
```

## What "Impact" Means Right Now

The current exposed MCP `impact` slice is code-level reverse dependency analysis over static cross-reference sources.

It is honest, but limited:

- good for "who statically references or calls this object?"
- not a full runtime impact engine
- not yet the final hybrid of ADT + cross-reference tables + parser augmentation

That means:

- dynamic calls can be missed
- some include-level/procedural edge cases still need parser overlay
- transport/config impact is not yet merged into the exposed `impact` acquisition path

## What "Where-Used Config" Means Right Now

The graph core supports `where-used-config` over canonical `TVARVC` nodes and `READS_CONFIG` edges.

Today this is implemented in `pkg/graph`, but not yet wired through CLI/MCP in the same way as co-change and impact.

That makes it a good next integration slice, not vaporware.

## Current Limitations

Be explicit about the current boundaries:

- co-change is transport-based, not semantic similarity
- impact is static reverse dependency, not runtime trace truth
- config reads are heuristic, not exact
- no generic Cypher/Gremlin layer is exposed
- no persistence layer yet
- no auth graph yet

This is deliberate. The MVP is trying to prove value first, not freeze a platform too early.

## Roadmap Shape

The next sensible improvements are:

1. wire `where-used-config` through CLI and MCP
2. upgrade `impact` acquisition into a hybrid:
   - `WBCROSSGT/CROSS` as reverse-index backbone
   - ADT call graph as high-confidence overlay
   - parser as local/procedural gap-filler
3. add export surfaces once the query model settles

## Practical Advice

Use the graph features when:

- transport history matters
- reverse dependency scope matters
- grep gives too much noise
- a plain call tree is not enough

Stick to simpler tools when:

- you only need one source file
- the answer is obviously local
- runtime behavior matters more than static structure

Graph analysis is strongest when you need structure, not just text search.
