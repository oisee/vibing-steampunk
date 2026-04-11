# SQLite Analysis Cache — Wiring Plan & Steering

**Date:** 2026-04-08
**Type:** Executive steering plan
**Depends on:** ADR-005 (docs/adr/005-opt-in-sqlite-analysis-cache.md)
**Implements:** Cache wiring for graph, effects, contracts, and transport artifacts

---

## Executive Summary

vsp has a working SQLite cache infrastructure (`pkg/cache/sqlite.go`) and a rich analysis surface (boundaries, effects, slim, health, changelog, changes). They're not connected. Every analysis run re-fetches, re-parses, and re-derives from scratch.

**Goal:** Wire them together behind a `--cache` flag. Second run of any analysis command should be near-instant when source hasn't changed.

**Non-goal:** Replace SAP as source of truth. Cache is acceleration, not storage.

**Estimated impact:**
- `vsp boundaries '$ZLLM'` goes from ~60s (227 source fetches) to ~2s (cache hit + validation)
- `vsp health --package '$ZLLM'` benefits from cached boundary + effects data
- Cross-session persistence: graph built yesterday is available today

---

## Current State

### What exists

| Component | Status | Location |
|-----------|--------|----------|
| SQLite schema (nodes, edges, APIs) | Working | `pkg/cache/sqlite.go` |
| PutNode/GetNode/PutEdge/GetEdge | Working | `pkg/cache/sqlite.go` |
| Invalidation fields (valid, invalidated_at, reason) | Schema exists, not wired | `pkg/cache/sqlite.go` |
| Graph extraction (parser) | Working | `pkg/graph/builder_parser.go` |
| Effects extraction | Working (14 tests) | `pkg/graph/effects.go` |
| MethodSignature extraction | Working | `pkg/graph/queries_signature.go` |
| AcquirePackageScope/Objects | Working | `cmd/vsp/acquire.go` |
| Package resolution (TADIR) | Working, not cached | `cmd/vsp/devops.go:resolvePackagesCLI` |

### What's missing

| Gap | Impact |
|-----|--------|
| No cache flag on CLI commands | Users can't opt in |
| Source hash not computed | Can't detect staleness |
| Effects/contracts not cached | Re-derived every run |
| TADIR resolution not cached | Re-queried every run (100+ SQL queries) |
| No analyzer version tracking | Can't invalidate on code changes |

---

## Architecture

```
                    ┌─────────────┐
                    │   SAP/ADT   │  ← source of truth
                    └──────┬──────┘
                           │
                    ┌──────┴──────┐
                    │   vsp CLI   │
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
        ┌─────┴─────┐ ┌───┴────┐ ┌────┴─────┐
        │  Acquire   │ │ Parse  │ │ Analyze  │
        │ (TADIR,    │ │(source │ │(effects, │
        │  TDEVC)    │ │ →edges)│ │contracts)│
        └─────┬─────┘ └───┬────┘ └────┬─────┘
              │            │            │
              └────────────┼────────────┘
                           │
                    ┌──────┴──────┐
                    │   SQLite    │  ← opt-in cache
                    │  (local)    │
                    └─────────────┘

Flow:
1. Check cache: key = (system, object, source_hash, analyzer_version)
2. Cache hit + valid → use cached result
3. Cache miss or stale → fetch from SAP, derive, store in cache
```

---

## Milestones

### Milestone 1: Cache Flag + Source Fingerprinting

**Objective:** Enable `--cache` flag and compute source hashes for cache key validation.

**Deliverables:**
- Cache config in `.vsp.json` per system: `"cache": true`
- Env var: `VSP_CACHE=true` (global) or `VSP_<SYSTEM>_CACHE=true` (per-system)
- Cache path: `.vsp-cache/<system>.db` (auto-derived, or `"cache_path"` override)
- Resolution order: flag → system config → system env → global env → default off
- Source hash computation (SHA-256 of fetched source text)
- Cache open/close lifecycle in command runner

**Config example:**
```json
{
  "systems": {
    "dev": {
      "url": "http://dev:50000",
      "user": "DEVELOPER",
      "cache": true
    }
  }
}
```

**DONE:** Config fields added to `SystemConfig`, `systemParams`, env resolution wired.

**Schema additions:**
```sql
CREATE TABLE IF NOT EXISTS source_fingerprints (
    system TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_name TEXT NOT NULL,
    source_hash TEXT NOT NULL,
    fetched_at INTEGER NOT NULL,
    PRIMARY KEY (system, object_type, object_name)
);
```

**Checkpoint:** `vsp boundaries '$ZLLM' --cache` runs, stores fingerprints, second run detects "no source changed" condition.

**Effort:** Small. Plumbing only, no new analysis.

### Milestone 2: TADIR + Package Scope Caching

**Objective:** Cache the most repeated queries — TADIR package resolution and TDEVC hierarchy.

**Deliverables:**
- Cache `resolvePackagesCLI` results (object → package mapping)
- Cache `AcquirePackageScope` results (TDEVC hierarchy)
- Invalidate on TTL (24h default) or explicit `--cache-refresh`

**Schema additions:**
```sql
CREATE TABLE IF NOT EXISTS tadir_cache (
    system TEXT NOT NULL,
    object_name TEXT NOT NULL,
    object_type TEXT NOT NULL,
    package TEXT NOT NULL,
    cached_at INTEGER NOT NULL,
    PRIMARY KEY (system, object_name)
);

CREATE TABLE IF NOT EXISTS scope_cache (
    system TEXT NOT NULL,
    root_package TEXT NOT NULL,
    packages TEXT NOT NULL,  -- JSON array
    hierarchy TEXT NOT NULL, -- JSON object
    method TEXT NOT NULL,    -- exact/hierarchy/prefix
    cached_at INTEGER NOT NULL,
    PRIMARY KEY (system, root_package)
);
```

**Checkpoint:** `vsp boundaries '$ZLLM' --cache` skips all TADIR queries on second run.

**Effort:** Small. High impact — eliminates 100+ SQL queries per run.

### Milestone 3: Graph Edge + Effects Caching

**Objective:** Cache parsed dependency edges and extracted effects per object.

**Deliverables:**
- Cache `ExtractDepsFromSource` + `ExtractDynamicCalls` results per (object, source_hash)
- Cache `ExtractEffects` results per (object, source_hash, analyzer_version)
- On cache hit: skip source fetch + parse entirely
- Analyzer version: embed as const in `pkg/graph/version.go`

**Schema additions:**
```sql
CREATE TABLE IF NOT EXISTS parsed_edges (
    system TEXT NOT NULL,
    source_node TEXT NOT NULL,
    source_hash TEXT NOT NULL,
    edges TEXT NOT NULL,     -- JSON array of Edge
    analyzer_version TEXT NOT NULL,
    cached_at INTEGER NOT NULL,
    PRIMARY KEY (system, source_node, source_hash)
);

CREATE TABLE IF NOT EXISTS effects_cache (
    system TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_name TEXT NOT NULL,
    source_hash TEXT NOT NULL,
    effects TEXT NOT NULL,   -- JSON EffectInfo
    analyzer_version TEXT NOT NULL,
    cached_at INTEGER NOT NULL,
    PRIMARY KEY (system, object_type, object_name, source_hash)
);
```

**Checkpoint:** `vsp boundaries '$ZLLM' --cache` on 227 objects: first run ~60s, second run ~2s.

**Effort:** Medium. The main value milestone.

### Milestone 4: Contract Caching

**Objective:** Cache method signatures and code unit contracts.

**Deliverables:**
- Cache `ExtractMethodSignature` results per (class, method, source_hash)
- Cache aggregated class contracts (all public methods) per (class, source_hash)
- Used by `vsp context` and future context frame builder

**Effort:** Small, follows same pattern as Milestone 3.

### Milestone 5: Transport Fact Caching

**Objective:** Cache E071/E070/E07T/E070A query results for changelog/changes.

**Deliverables:**
- Cache transport-to-object associations (E071)
- Cache transport headers (E070 + E07T)
- Cache transport attributes (E070A)
- Invalidation: watermark-based — cache stores latest transport date, re-query only if newer transports exist

**Schema additions:**
```sql
CREATE TABLE IF NOT EXISTS transport_objects (
    system TEXT NOT NULL,
    trkorr TEXT NOT NULL,
    pgmid TEXT NOT NULL,
    object TEXT NOT NULL,
    obj_name TEXT NOT NULL,
    cached_at INTEGER NOT NULL,
    PRIMARY KEY (system, trkorr, object, obj_name)
);

CREATE TABLE IF NOT EXISTS transport_headers (
    system TEXT NOT NULL,
    trkorr TEXT NOT NULL,
    strkorr TEXT,
    trfunction TEXT,
    trstatus TEXT,
    as4user TEXT,
    as4date TEXT,
    as4text TEXT,
    cached_at INTEGER NOT NULL,
    PRIMARY KEY (system, trkorr)
);
```

**Checkpoint:** `vsp changelog '$ZLLM' --cache` instant on second run for stable packages.

**Effort:** Medium. High value for changelog/changes commands.

### Milestone 6: Cache Management CLI

**Objective:** Users can inspect, clean, and refresh the cache.

**Deliverables:**
- `vsp cache info` — show cache location, size, entry counts
- `vsp cache clean` — delete all cached data
- `vsp cache clean --package '$ZLLM'` — delete for specific package
- `vsp cache refresh` — invalidate all, force re-derive on next run
- `--cache-refresh` flag on analysis commands — ignore cache for this run, update it

**Effort:** Small. Mostly formatting + SQLite deletes.

---

## Ordering Rules

1. **Milestone 1 before anything** — the flag and fingerprint infrastructure is the foundation
2. **Milestone 2 before 3** — TADIR caching is the highest-impact, lowest-risk acceleration
3. **Milestone 3 is the main value** — this is where "second run is instant" happens
4. **Milestones 4 and 5 are parallel** — independent of each other
5. **Milestone 6 anytime after 2** — cache management is useful as soon as there's data

```
M1 (flag + fingerprint)
 │
 ├── M2 (TADIR + scope)
 │    │
 │    ├── M3 (edges + effects) ← main value
 │    │    │
 │    │    ├── M4 (contracts)
 │    │    └── M5 (transport facts)
 │    │
 │    └── M6 (cache CLI)
```

---

## Risks

| Risk | Mitigation |
|------|------------|
| Stale cache masquerades as truth | Source hash validation + analyzer version + TTL fallback |
| Cache grows unbounded | `vsp cache info` + TTL-based cleanup + `vsp cache clean` |
| CGO dependency (sqlite3) | Already in go.mod via mattn/go-sqlite3; consider modernc.org/sqlite for pure Go if CGO is problematic |
| Cache on shared filesystems | Document: cache is local, not for shared NFS/network drives |
| Breaking schema changes | Embed schema version; drop and recreate on version mismatch |

---

## What NOT To Build

1. **Remote/shared cache** — local file only, no network cache service
2. **Auto-cache without opt-in** — always requires `--cache` flag or config
3. **Raw source storage** — fingerprints only, not source text
4. **Transitive rollup caching** — until provenance model is proven on local facts
5. **Cache as primary data** — if cache is missing, everything still works (just slower)

---

## Success Criteria

| Criteria | Measurement |
|----------|-------------|
| Second run of `vsp boundaries` is 10x faster | Wall clock: ~60s → ~5s on 200+ object package |
| Cache hit rate > 90% for stable packages | Log cache hit/miss counts |
| Zero false positives from stale cache | Source hash validation catches all changes |
| No behavior change without `--cache` flag | All existing tests pass without cache |
| Cache can be deleted at any time without data loss | `rm .vsp-cache/` → everything rebuilds |

---

## Recommended First Slice

**Start with Milestone 1 + 2 together.** They're small, high-value, and prove the wiring pattern.

Concrete deliverable: `vsp boundaries '$ZLLM' --cache` that:
1. First run: fetches everything, stores fingerprints + TADIR mappings in SQLite
2. Second run: skips TADIR queries (cache hit), still fetches source (fingerprint comparison pending)
3. `vsp cache info` shows entry counts

This validates the cache lifecycle (open, read, write, close) in a real command without touching the parser or analysis logic.
