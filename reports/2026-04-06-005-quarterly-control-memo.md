# VSP Quarterly Control Memo — Q3 2026

**Date:** 2026-04-06
**Report ID:** 2026-04-06-005
**Subject:** Next-quarter priorities, architecture consolidation, long bets, passive data collection, explicit NOT-do list
**Sources:** Three parallel analysis agents (product, architecture, strategy) + prior roadmap consensus

---

## 1. STRONGEST NEXT PRODUCT MOVES

### P1. Fix BTP/Cloud ABAP Authentication (Issue #90) — BLOCKER

A real user on BTP ABAP Environment (eu10) cannot connect. `curl` works; VSP does not. Root cause: Go's `http.Client` strips `Authorization` header on cross-origin redirects. BTP is where SAP is going — every on-prem-only tool becomes irrelevant on 2-3 year horizon.

**Effort:** 1-2 weeks. Fix in `pkg/adt/http.go` (custom `CheckRedirect`).
**Impact:** Extreme. Table-stakes, not a feature.

### P2. Fix Lock Handle Reliability (Issue #88) — BLOCKER

User with working setup cannot write code: `ExceptionResourceInvalidLockHandle`. Lock from one HTTP call not recognized by subsequent write. Likely session-affinity issue on SAP_ABA 7.51.

**Effort:** 1-2 weeks. Fix in `pkg/adt/workflows_edit.go` + `crud.go` + `http.go`.
**Impact:** Very high. Converts VSP from inspector to development tool.

### P3. Transport Quality Gate — WORKFLOW

"Before I release this transport, tell me what's broken." Composes existing tools: GetTransport → RunUnitTests → RunATCCheck → CheckBoundaries → pass/fail summary.

```
vsp transport check A4HK900042
SAP(action="analyze", params={"type": "transport_check", "transport": "A4HK900042"})
```

**Effort:** 2-3 weeks. Composition of existing tools + output formatting.
**Impact:** High. Embeds VSP in daily release process.

### P4. AFF Object Type Support (Issues #27, #74) — UNBLOCK

DDLX (CDS metadata extensions), NROB, SMIM, ENHO — modern RAP object types that users need. AFF objects use REST+JSON, easier than classic types.

**Effort:** 2-3 weeks for first 5-8 AFF types.
**Impact:** High. Removes hard blockers for modern SAP developers.

### P5. Refactor Preview Family — DIFFERENTIATION

Move-method-preview, visibility-change-preview, signature-change-preview. "What breaks if I make GET_DATA protected?" No SAP tool does this.

**Effort:** 3-4 weeks. Builds on shipped rename-preview, class-sections, method-signature, impact.
**Impact:** Medium-high. The AI-assisted refactoring differentiator.

---

## 2. STRONGEST ARCHITECTURE CONSOLIDATIONS

### A1. Unify Standard/Custom Classification (0.5 days)

5 divergent implementations of "is this Z/Y custom?": `IsStandardObject`, `IsCustomObject`, `isCustomName`, `isSAPStandard`, `isCustom`. Different namespace handling. Correctness bug waiting to happen.

**Action:** One canonical `IsCustomObject` in `pkg/graph/graph.go` (the `queries_apisurface.go` version is most complete). Delete all others.

### A2. Extract Shared Data Acquisition Layer (2-3 days)

Transport fetch (E070/E071), reverse deps (WBCROSSGT/CROSS), TADIR package lookup — all duplicated between CLI and MCP. CLI co-change handler is 230 lines nearly identical to MCP version.

**Action:** New `pkg/acquire/` package with `SQLRunner` interface + concrete acquisition functions. Both CLI and MCP call shared functions.

### A3. Extract Package-Scoped Query Preamble (1-2 days)

Every package command (slim, api-surface, deps, health) independently does TADIR + batch WBCROSSGT with different batch sizes and different false-positive handling.

**Action:** `FetchPackageContents` + `FetchBatchReverseRefs` in shared acquire package. Consistent batching + post-filter against exact name set.

### A4. Consolidate ADT URL Builder + Type Mappings (1 day)

`buildADTObjectURL` in internal/mcp (inaccessible to CLI). CROSS type code tables in 4 places. SAP domain knowledge scattered.

**Action:** `pkg/graph/reftypes.go` with all mapping tables + `ADTTypeToObjectURL`.

### A5. Split cli_extra.go (1 day, AFTER A1-A4)

1882 lines, 15+ commands. Split into domain files: cli_graph.go, cli_analysis.go, cli_query.go, cli_format.go.

**Sequencing:** A1 → A4 → A2 → A3 → A5. Total: 5-7 days.

---

## 3. HIGHEST-UPSIDE LONG BETS

### L1. Graph Persistence via Existing pkg/cache

`pkg/cache/sqlite.go` is **fully built** with node/edge/API schema, indexes, batch PutNodes/PutEdges — but imported by nothing outside its own tests. Wire it into the MCP server so every graph query writes results to SQLite. After a week of usage, the graph contains thousands of nodes/edges that make subsequent queries instant.

**Early investment:** Add `*cache.SQLiteCache` to Server struct. After `BuildTransportGraph`/`fetchReverseDeps`, write nodes+edges to cache. Check cache freshness before querying SAP. ~2-3 days.

**Payoff:** Foundation for historical impact, smart test selection, offline analysis.

### L2. Test-to-Code Mapping

Map test classes to production objects via naming conventions + parser dep analysis + WBCROSSGT. Enable "smart test selection": change ZCL_FOO → run only tests that reference ZCL_FOO.

**Early investment:** One query function similar to `ComputeSlim` that correlates test/production objects. ~2 days.

**Payoff:** The AI-agent workflow killer feature (change → test → iterate in seconds, not minutes).

### L3. Incremental Graph from Transport Events

Instead of full rebuild per query, detect newly released transports (E070 TRSTATUS='R' watermark) and incrementally update the cached graph. Solves the scaling problem of capped SQL queries.

**Early investment:** Watermark table in SQLite + periodic delta query. ~1-2 days.

**Payoff:** Prerequisite for historical impact (time machine). Makes graph queries scale to enterprise-size systems.

---

## 4. PASSIVE DATA COLLECTION — START NOW

### D1. Log transport metadata on every co-change/impact query

`fetchTransportData` already fetches E070 headers + E071 objects, then discards them. Append to `~/.vsp/transport_log.jsonl`. 5-10 lines of code.

### D2. Cache WBCROSSGT/CROSS results as edge snapshots

`fetchReverseDeps` issues SQL queries and discards results. Write-behind to SQLite cache edge table. Zero read path yet — pure accumulation.

### D3. Fingerprint source on every GetSource

Compute SHA256 of returned source, store hash+timestamp (NOT the source itself). Enables: cache invalidation, change detection for smart test selection.

---

## 5. EXPLICITLY NOT DO YET

### X1. Graph Query Language (Cypher/GQL/custom DSL)

10 edge kinds, 8 node types. Domain queries are 50-100 lines of Go each, easy to test and extend. A generic query language adds parser + planner + formatter for less precise answers. The current approach is correct.

### X2. Cross-System Diff

Sounds simple, becomes nightmare. "Same/different" for SAP objects is undefined (active version? transport version? TADIR entry vs source?). Design work not done. Building now would mislead on real cases.

### X3. Live ABAP REPL

`ExecuteABAP` is stateless (each call is its own LUW). "Persistent variables" requires either growing programs or WebSocket session persistence. A fake REPL that resets state is worse than no REPL.

### X4. E071K Customizing Entries in Graph

TABKEY field requires per-table key parsing. Thousands of customizing tables, each with different key structures. Multi-week project disguised as a simple SQL adapter.

### X5. Auth/Role Graph (PFCG/AGR_*)

SU24 proposals ≠ AGR_1251 values ≠ runtime AUTHORITY-CHECK. Building a "mostly right" auth graph is dangerous for security decisions. Needs its own design doc and data model, not a graph extension.

---

## QUARTERLY TIMELINE

| Week | Focus |
|------|-------|
| **1-2** | P1 (BTP auth fix) + P2 (lock handle fix) + A1 (classification unify) |
| **3-4** | A2+A3 (acquisition layer) + D1+D2+D3 (passive collection) |
| **5-6** | P3 (transport quality gate) |
| **7-8** | P4 (AFF object types, first 5 types) |
| **9-10** | L1 (graph persistence wiring) + A4+A5 (type maps + file split) |
| **11-12** | P5 (refactor preview family) + L2 (test-to-code mapping) |

**Key principle:** Fix blockers first (P1, P2), consolidate architecture while it's cheap (A1-A3), start passive data collection immediately (D1-D3), then build differentiated features on a solid foundation (P3-P5, L1-L2).
