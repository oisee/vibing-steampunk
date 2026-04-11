# ADR-005: Opt-In SQLite Analysis Cache

**Date:** 2026-04-08
**Status:** PROPOSAL
**Context:** Persistent caching for graph extraction, effect facts, and transport-derived analysis

---

## Summary

VSP should support an **opt-in SQLite cache layer** for graph, fact, and transport analysis artifacts.

This cache is:

- a **performance accelerator**
- a **cross-session local store**
- a **derived-analysis cache**

It is **not**:

- the source of truth over SAP
- the primary graph engine
- a generic request/response archive by default

The intended architecture is:

- live SAP/ADT/source parsing produce truth
- in-process Go graph logic performs extraction and bounded propagation
- SQLite optionally stores reusable analysis artifacts and provenance

---

## Context

The repository already has SQLite cache infrastructure in [pkg/cache/sqlite.go](/home/alice/dev/vibing-steampunk/pkg/cache/sqlite.go), documented in [reports/2025-12-02-010-cache-implementation-complete.md](/home/alice/dev/vibing-steampunk/reports/2025-12-02-010-cache-implementation-complete.md).

There is also an explicit architectural push toward wiring graph persistence into the product in [reports/2026-04-06-005-quarterly-control-memo.md](/home/alice/dev/vibing-steampunk/reports/2026-04-06-005-quarterly-control-memo.md).

At the same time, newer graph/context work introduces richer analysis layers:

- graph topology
- code-unit contracts
- execution semantics
- local effect profiles
- transport-derived signals
- bounded transitive summaries

These analyses are expensive enough to benefit from persistence, but subtle enough that stale or decontextualized cache data can become misleading.

So the missing decision is not “should SQLite exist?”.
It already does.

The real decision is:

- what belongs in SQLite
- with what provenance
- under what invalidation rules
- and what must remain live-only

---

## Problem

Repeated analysis work currently pays the same costs again and again:

- package inventories
- source fingerprinting
- graph edge extraction
- transport lookups
- contract/effect derivation
- reverse dependency batches

Without persistence:

- repeated sessions re-fetch and re-derive the same data
- passive accumulation opportunities are lost
- future historical analysis has no durable substrate

With careless persistence:

- stale analysis can masquerade as truth
- transitive summaries can outlive their source basis
- security-sensitive raw payloads can be retained unnecessarily

---

## Decision

Adopt an **opt-in SQLite analysis cache** with the following boundaries:

### 1. SQLite is a cache, not a database of record

Authoritative truth remains:

- ADT responses
- SAP table reads
- live source fetches
- fresh parser output

SQLite stores **reusable derived artifacts** and **provenance needed to validate them**.

### 2. Cache graph and fact artifacts first

The first-class cached families should be:

- object inventory
- source fingerprints and fetch metadata
- graph nodes
- graph edges
- contracts
- execution/effect summaries
- transport facts and snapshots
- analysis-run provenance

### 3. Do not default to raw request archival

Raw HTTP or SQL request/response capture should not be the main cache design.

If request logging is ever added, it should be:

- explicitly enabled
- isolated from the analysis cache
- bounded and redactable

### 4. Cache rows must carry provenance

Every reusable artifact must be tied to enough context to invalidate or distrust it safely.

Minimum provenance includes some combination of:

- system or destination identity
- client if relevant
- object type/name or package scope
- source hash or equivalent fingerprint
- transport watermark or query watermark where relevant
- analyzer version
- cached timestamp

### 5. Derived transitive summaries are lower priority than local facts

Local facts and graph artifacts are better cache entries than broad transitive rollups.

Reason:

- local facts are easier to invalidate
- transitive results depend on traversal policy and depth
- transitive claims are more likely to go stale semantically

So cache local artifacts first, and only later consider caching bounded transitive summaries with explicit scope/version keys.

---

## What To Cache

### A. Object and source substrate

Safe and high-value:

- package/object inventory
- object identity normalization
- source hashes
- ADT last-modified timestamps when available
- fetch timestamps

### B. Graph substrate

Safe and high-value:

- nodes
- edges
- edge source/provenance
- scope-level graph snapshots

Examples:

- package graph fragments
- transport graph fragments
- reverse dependency batches

### C. Local semantic facts

Safe and high-value:

- `CodeUnitContract`
- `ExecutionSemantics`
- `EffectProfile`
- lightweight metrics

These are deterministic enough to cache if tied to source fingerprint plus analyzer version.

### D. Transport-derived analysis

Good fit for SQLite:

- transport membership snapshots
- object-to-transport associations
- release timestamp watermarks
- transport frequency summaries
- co-change accumulation support

### E. Analysis runs

Useful for auditability and invalidation:

- analysis type
- scope
- analyzer version
- cache hit/miss
- input fingerprint
- run timestamp

---

## What Not To Cache By Default

### 1. Raw source bodies

Default position:

- store hashes and metadata first
- do not make full-source persistence the default cache behavior

Reason:

- source may be sensitive
- source duplication increases footprint and leakage risk
- many caching benefits are achievable from fingerprints plus derived facts

If full-source caching is ever added, it should be separately gated.

### 2. Raw HTTP payload logs

Do not treat the analysis cache as a request recorder.

Reason:

- security and privacy risk
- lower signal than normalized derived facts
- harder retention story

### 3. Unversioned transitive claims

Examples to avoid caching loosely:

- “LUW-safe”
- “eventually writes DB”
- “pure”

These depend on:

- traversal depth
- graph completeness
- dynamic-edge policy
- analyzer version

If cached later, they need those inputs in the key.

### 4. Semantic truth detached from source/version

Any row that cannot answer “what source or snapshot was this derived from?” is not safe enough.

---

## Recommended Cached Tables

The exact schema may evolve, but the analysis cache should conceptually cover:

- `objects`
- `source_fingerprints`
- `graph_nodes`
- `graph_edges`
- `contracts`
- `effects`
- `transport_facts`
- `analysis_runs`

Notes:

- the existing `pkg/cache` node/edge/api schema is a good start
- new semantic tables should be additive, not a replacement
- API surface caching can remain a separate concern if that keeps invalidation cleaner

---

## Provenance Requirements

Each cached artifact should include the smallest sufficient provenance set.

### For object/source-derived facts

- system/destination
- object type
- object name
- source hash
- analyzer version
- cached_at

### For scope-level graph artifacts

- system/destination
- scope kind
- scope identifier
- scope fingerprint or build watermark
- analyzer version
- cached_at

### For transport facts

- system/destination
- transport/request ID
- release status or watermark
- extracted_at
- analyzer version

### For transitive summaries if added later

- root object/unit
- traversal depth
- traversal policy
- graph snapshot/fingerprint
- analyzer version
- cached_at

---

## Invalidation Rules

### 1. Source-based invalidation

Invalidate local semantic facts when:

- source hash changes
- ADT last-modified timestamp proves newer state

### 2. Analyzer-version invalidation

Invalidate derived semantic rows when:

- parsing logic changes
- effect extraction logic changes
- graph normalization changes

### 3. Transport watermark invalidation

Invalidate transport-derived snapshots when:

- new released transports pass the stored watermark
- the scope’s relevant transport set changes

### 4. TTL as fallback, not primary truth model

TTL is useful as a safety net.
It is not enough by itself for semantic correctness.

Preferred order:

1. fingerprint/timestamp invalidation
2. analyzer-version invalidation
3. TTL fallback

---

## Why SQLite and Not a Graph DB

SQLite is appropriate here because:

- it is already implemented
- it is opt-in and local
- it is easy to ship and inspect
- it works well for cache and snapshot data
- the primary graph work remains in Go

This ADR does **not** make SQLite the graph engine.

It remains:

- a cache and persistence substrate
- not the semantic runtime

If export to Neo4j/GraphML/DOT is later useful, that remains downstream interoperability, not a dependency.

---

## Security and Privacy Position

Default cache behavior should minimize sensitive persistence.

Principles:

- prefer fingerprints over full source bodies
- prefer normalized semantic artifacts over raw payloads
- separate debug/request logs from analysis cache
- keep opt-in explicit
- document local file location and retention expectations

---

## Consequences

### Positive

- faster repeated graph/context/effect analysis
- cross-session reuse of expensive extraction work
- substrate for historical transport and impact features
- easier passive accumulation of graph/transport facts
- lower repeated SAP load for stable artifacts

### Negative

- more invalidation complexity
- risk of stale cached semantics if provenance is weak
- local storage footprint grows over time
- cache wiring can add product complexity if done too broadly too early

---

## Implementation Guidance

Recommended implementation order:

1. wire existing `pkg/cache` as opt-in for graph node/edge accumulation
2. add source fingerprint persistence
3. add contract/effect semantic cache tables keyed by source hash + analyzer version
4. add transport watermark and transport-fact persistence
5. only later consider caching bounded transitive summaries

The first read path should prefer:

- exact fingerprint match
- compatible analyzer version
- still-valid transport or scope watermark

Otherwise:

- recompute from live data
- overwrite cache entry

---

## Explicit Non-Goals

This ADR does not propose:

- replacing live SAP reads with SQLite truth
- persisting all raw source by default
- broad raw request logging
- moving graph traversal into SQLite as the primary runtime
- storing unconstrained transitive “color propagation” results without scope/version provenance

---

## Status Rationale

This ADR is `PROPOSAL` because:

- the implementation substrate already exists
- the product-level boundary decision is now being clarified
- semantic cache tables and wiring policy are not implemented yet

It should become `ACCEPTED` when:

- one graph read/write path uses SQLite behind an opt-in flag
- source fingerprint persistence is wired
- at least one semantic artifact family is cached with proper provenance
