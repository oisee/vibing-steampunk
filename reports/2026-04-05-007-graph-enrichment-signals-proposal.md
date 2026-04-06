# Graph Enrichment Signals Proposal

**Date:** 2026-04-05
**Report ID:** 007
**Subject:** Separate graph facts from relevance signals for upgrade-focused analysis

---

## Motivation

The graph should answer two different questions:

1. What is connected to what?
2. Which of those connections matter more right now?

The first is topology. The second is prioritization.

If both are modeled the same way, the graph becomes noisy and semantically weak. A simplification item, a usage statistic, or a stale object flag is not the same kind of thing as `CALLS`, `READS_CONFIG`, or `IN_TRANSPORT`.

---

## Core Decision

Keep three layers separate:

1. **Topology**
   Facts and traversable relationships.
2. **Enrichment**
   Signals, annotations, and scores attached to nodes or edges.
3. **Ranking/Reporting**
   Query-time logic that uses topology plus enrichment to sort and explain results.

This avoids turning every useful hint into another edge type.

---

## What Belongs in Topology

Examples of graph facts:

- `CALLS`
- `REFERENCES`
- `READS_CONFIG`
- `WRITES_CONFIG`
- `IN_TRANSPORT`
- `STORED_IN`
- `EXPOSED_BY_TCODE`

These are structural relationships. They define reachable paths.

---

## What Belongs in Enrichment

Examples of signals that should not be modeled as edges by default:

- Simplification item applies to object
- Usage frequency is high or low
- Object has not been executed for N months
- Object is deprecated or obsolete
- Object is touched often in transports
- Object is part of productive business flow
- Object has ATC findings
- Object is standard SAP but upgrade-relevant
- Edge was discovered heuristically, not from exact metadata
- Node or path has low confidence

These should be stored as properties or annotations, not as graph structure.

---

## Proposed Model

### Node annotations

Each node may carry a set of enrichment properties such as:

- `usage_score`
- `risk_score`
- `upgrade_relevance`
- `simplification_relevance`
- `exec_frequency`
- `last_seen_transport`
- `is_deprecated`
- `is_standard`
- `confidence`

### Edge annotations

Each edge may carry properties such as:

- `confidence`
- `evidence_type`
- `last_seen`
- `frequency`
- `is_heuristic`

### Path/report annotations

Some relevance is query-specific and should be computed at report time:

- path contains simplification-relevant node
- path crosses productive transaction
- path ends in custom endpoint
- path depends on heuristic hop
- path has weak evidence

---

## Source Categories

### 1. Simplification and upgrade sources

These color nodes with upgrade significance:

- simplification lists
- released vs non-released APIs
- obsolete/deprecated object markers
- compatibility or remediation findings

### 2. Runtime and usage sources

These color nodes with operational relevance:

- usage statistics
- execution frequency
- last execution timestamp
- productive workload presence
- batch/job usage

### 3. Delivery/change sources

These color nodes with maintenance relevance:

- transport frequency
- recent change activity
- co-change density
- last changed by / last changed on

### 4. Quality sources

These color nodes with technical risk:

- ATC findings
- syntax or checkrun issues
- dead code indicators
- complexity or hotspot metrics

---

## Why This Matters

Two nodes may both be reachable from the same changed customizing entry, but they are not equally important.

Example:

- `TVARVC:ZKEKEKE -> PROG:ZREP_OLD`
- `TVARVC:ZKEKEKE -> CLAS:ZCL_ORDER_API`

Both are impacted. But if:

- `ZREP_OLD` has not run for 18 months
- `ZCL_ORDER_API` is used daily in production
- `ZCL_ORDER_API` also appears in a simplification item or remediation list

then the second path should be ranked much higher.

The graph should therefore answer not only:

- "what is impacted?"

but also:

- "what is most relevant?"
- "what is most risky for upgrade?"
- "what is likely noise?"

---

## Architectural Recommendation

Implement enrichment as separate components, not inside the core builders:

- `builder_*` creates nodes and edges
- `annotator_*` enriches nodes and edges
- `query_*` traverses topology and ranks using enrichment

Suggested future modules:

- `annotator_simplification.go`
- `annotator_usage.go`
- `annotator_transport_stats.go`
- `annotator_quality.go`

This keeps graph building deterministic and keeps ranking logic flexible.

---

## Query Implications

Queries should support both raw and ranked output.

### Raw mode

Return reachable nodes and paths without scoring bias.

### Ranked mode

Return the same graph, but ordered by relevance using enrichment signals.

Example ranked factors:

- custom endpoints above standard intermediates
- high-usage objects above stale ones
- simplification-flagged nodes above neutral nodes
- exact edges above heuristic edges

This is especially important for upgrade-focused impact analysis.

---

## MVP Guidance

Do not put enrichment into the first graph MVP implementation unless it is needed for one concrete report.

For MVP:

- keep topology minimal and correct
- allow enrichment fields in the model
- do not block graph delivery on enrichment plumbing

For pilot/next phase:

- start with one or two high-value signals only
- likely candidates: `confidence` and `usage_score`
- then add simplification relevance once a source is selected

---

## Suggested First Enrichment Signals

If the graph is being built for upgrade planning, the first five useful signals are:

1. `confidence`
   Exact metadata vs heuristic discovery.
2. `usage_score`
   How often the object or path is actually used.
3. `simplification_relevance`
   Whether the node is covered by simplification/remediation sources.
4. `change_frequency`
   How often the object appears in transports.
5. `is_standard`
   Standard SAP vs custom, for reporting and ranking.

These five are enough to make reports much more actionable without redesigning the graph.

---

## Recommendation

Treat simplification lists, usage stats, and similar inputs as enrichment layers that color the graph, not as primary graph structure.

The graph should remain compact, traversable, and semantically clean.

Use enrichment to make answers more relevant, not to redefine the topology.
