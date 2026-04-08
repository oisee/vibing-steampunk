# Graph Knowledge MVP — Design Doc

**Date:** 2026-04-05
**Report ID:** 006
**Subject:** Extend pkg/graph from code-only deps to SAP knowledge graph (config, transports, auth)

---

## Motivation

Current `pkg/graph/` handles code dependencies only: CALLS, REFERENCES, LOADS between ABAP objects. Real-world impact analysis requires crossing into configuration, transport, and authorization layers.

**Target use case:** "STVARV ZKEKEKE changed — what programs read it, what transports carried those programs, what else changed in those transports?"

This is not achievable with grep or code-only graphs.

---

## Decision Record

| Decision | Rationale |
|----------|-----------|
| **In-memory first, no persistence** | Prove value of new edge types before fixing schema in storage |
| **Targeted queries, not general Cypher** | 5-10 purpose-built queries > generic query language |
| **Transport layer first** | Cheapest data source (structured SQL), highest immediate value |
| **SQLite only when rebuild cost is proven painful** | `pkg/cache/` ready when needed; premature persistence locks schema |
| **Object → request only (no tasks in MVP)** | Tasks are metadata on transport membership; no concrete task-level use case yet |
| **Co-change computed at query time, not stored** | Derived edges are high-cardinality, time-window-sensitive, noisy if materialized |
| **Impact = reverse-dependency traversal** | "What breaks if X changes?" always follows edges backward (who depends on X) |
| **Auth layer deferred, edge semantics provisional** | SU24 defaults ≠ runtime checks ≠ role assignments; too complex for MVP |

---

## Semantic Model Extension

### MVP Node Types (canonical)

```go
const (
    // Existing: CLAS, PROG, FUGR, INTF, TABL, DDLS, DEVC, TYPE

    // Transport layer (MVP)
    NodeTransport = "TR"       // Transport request (E070, request level only)
    // Note: Tasks (E070 strkorr→trkorr) are NOT separate nodes in MVP.
    // Task metadata (user, date) stored as Node.Meta on the parent TR.

    // Config layer (MVP)
    NodeTVARVC    = "TVARVC"   // Variant variable entry — one node per variable name
    // Display name uses STVARV convention (e.g. "ZKEKEKE"), internal ID is TVARVC:ZKEKEKE.
)
```

### Future Node Types (NOT in MVP — semantics provisional)

```go
const (
    NodeCustTable = "CUST"     // Customizing table entry (future)
    NodeTCode     = "TCODE"    // Transaction code (TSTC)
    NodeAuthObj   = "AUTH"     // Authorization object (TOBJ)
    NodeRole      = "ROLE"     // Authorization role (AGR_DEFINE)
    // WARNING: Auth edge semantics (SU24 defaults vs runtime checks vs role values)
    // are NOT equivalent. Do not define auth edges until the distinction is resolved.
)
```

### MVP Edge Kinds (canonical)

```go
const (
    // Existing: CALLS, REFERENCES, LOADS, CONTAINS_INCLUDE, DEPENDS_ON_CDS, DYNAMIC_CALL

    // Transport edges (MVP)
    EdgeInTransport  EdgeKind = "IN_TRANSPORT"     // object → transport (E071)
    // Note: EdgeCoChanged is NOT stored. Co-change is computed at query time
    // by grouping objects sharing IN_TRANSPORT edges to the same TR node.
    // This avoids high-cardinality derived edges and time-window ambiguity.

    // Config edges (MVP)
    EdgeReadsConfig  EdgeKind = "READS_CONFIG"      // program → TVARVC entry
    // Direction: PROG:ZREPORT --READS_CONFIG--> TVARVC:ZKEKEKE
    // Impact query traverses this BACKWARD: start at TVARVC:ZKEKEKE,
    // follow incoming READS_CONFIG edges to find affected programs.
)
```

### Future Edge Kinds (NOT in MVP — semantics provisional)

```go
const (
    EdgeWritesConfig EdgeKind = "WRITES_CONFIG"     // program → TVARVC entry (rare, future)
    EdgeExposedBy    EdgeKind = "EXPOSED_BY_TCODE"  // program → tcode (TSTC, future)
    EdgeRequiresAuth EdgeKind = "REQUIRES_AUTH"      // tcode → auth object (future, semantics TBD)
    EdgeGrantedBy    EdgeKind = "GRANTED_BY_ROLE"    // auth object → role (future, semantics TBD)
    // WARNING: REQUIRES_AUTH could mean SU24 default, runtime AUTHORITY-CHECK,
    // or role assignment. These are different things. Do not implement until
    // the intended semantic is chosen and documented.
)
```

### Edge Direction Convention

All edges point FROM the dependent TO the dependency:
- `PROG:ZREPORT --CALLS--> FUGR:BAPI_USER` (ZREPORT calls BAPI_USER)
- `PROG:ZREPORT --READS_CONFIG--> TVARVC:ZKEKEKE` (ZREPORT reads ZKEKEKE)
- `CLAS:ZCL_FOO --IN_TRANSPORT--> TR:A4HK900123` (ZCL_FOO is in transport)

**Impact analysis** always traverses edges **backward** (via `InEdges`):
"What depends on X?" = follow incoming edges from X.

**Depends-on analysis** traverses edges **forward** (via `OutEdges`):
"What does X depend on?" = follow outgoing edges from X.

These are separate queries, not modes of one query.

### MVP Edge Sources (with confidence levels)

```go
const (
    // Existing: PARSER, ADT_CALL_GRAPH, ADT_WHERE_USED, CROSS, WBCROSSGT, D010INC, TRACE

    SourceE071     EdgeSource = "E071"      // Transport object list — EXACT (repository metadata)
    SourceE070     EdgeSource = "E070"      // Transport headers — EXACT (repository metadata)

    SourceTVARVC_CROSS EdgeSource = "TVARVC_CROSS" // Config variable cross-ref — HEURISTIC
    // Evidence chain: CROSS(name=TVARVC) → candidate programs → grep source for variable name.
    // This is a two-step heuristic, NOT exact repository metadata.
    // False positives possible (variable name in comment/string).
    // False negatives possible (dynamic variable name construction).
)
```

### Future Edge Sources (NOT in MVP)

```go
const (
    SourceTSTC     EdgeSource = "TSTC"      // Transaction → program mapping (future)
    SourceSU24     EdgeSource = "SU24"      // SU24 auth checks (future, semantics TBD)
    SourceAGR      EdgeSource = "AGR_1251"  // Role → auth assignments (future, semantics TBD)
)
```

---

## Data Sources (SQL via RunQuery)

### Phase 1: Transport Layer

```sql
-- Transport requests (not tasks — tasks resolved via strkorr join)
SELECT trkorr, trfunction, trstatus, as4user, as4date
FROM e070 WHERE trkorr LIKE 'A4HK%' AND trstatus IN ('D','R','N')
  AND strkorr = ''   -- requests only, not tasks

-- Transport objects (from both requests and their tasks)
SELECT trkorr, pgmid, object, obj_name
FROM e071 WHERE trkorr IN (
  SELECT trkorr FROM e070 WHERE strkorr LIKE 'A4HK%'
  UNION
  SELECT trkorr FROM e070 WHERE trkorr LIKE 'A4HK%' AND strkorr = ''
)
```

Objects link to the parent request, not the task. Task user/date stored as metadata.

### Phase 2: Code Cross-References (already planned)

```sql
-- CROSS: include-level references
SELECT otype, name, include, master FROM cross WHERE name = 'ZCL_FOO'

-- WBCROSSGT: object-level references (cleaner)
SELECT otype, name, include, direct FROM wbcrossgt WHERE name = 'ZCL_FOO'

-- D010INC: compile-time includes
SELECT master, include FROM d010inc WHERE master LIKE 'Z%'
```

### Phase 3: Config Variables

```sql
-- All custom TVARVC entries
SELECT name, type, low, high FROM tvarvc WHERE name LIKE 'Z%'

-- Step 1: Who references TVARVC table at all (from CROSS)
SELECT include, master FROM cross WHERE name = 'TVARVC' AND otype = 'DA'
-- Step 2: Grep source of candidate programs for specific variable name
-- Step 3: Build READS_CONFIG edge only if grep confirms reference
--
-- CONFIDENCE: heuristic. Source is marked TVARVC_CROSS to distinguish
-- from exact repository metadata. Consumers should treat these edges
-- as "likely references" not "guaranteed dependencies".
```

### Phase 4: Auth/Access (future, NOT in MVP)

```sql
-- Transaction → program mapping
SELECT tcode, pgmna FROM tstc WHERE tcode LIKE 'Z%'

-- SU24: tcode → auth object defaults
SELECT object, auth, field, low FROM usobx_c WHERE name = 'ZTCODE'

-- Role → auth values
SELECT agr_name, object, auth, field, low, high FROM agr_1251 WHERE agr_name LIKE 'Z%'

-- WARNING: These sources represent different things:
-- USOBX_C = SU24 defaults (what the developer intended)
-- AGR_1251 = role values (what the admin configured)
-- Runtime AUTHORITY-CHECK = what the code actually checks
-- These are NOT interchangeable. Auth layer needs its own design doc.
```

---

## MVP Queries (3 initial)

### 1. `what-changes-with` (co-change analysis)

**Input:** object name (e.g., `CLAS ZCL_PRICING`)
**Algorithm:**
1. Find all transports containing this object (follow OutEdges of type IN_TRANSPORT)
2. For each transport, find all other objects (follow InEdges of type IN_TRANSPORT on the TR node)
3. Group by object, count co-occurrence across transports
4. Rank by frequency

**Note:** Co-change is computed at query time from IN_TRANSPORT edges. No CO_CHANGED edges are stored — this avoids cardinality explosion and time-window ambiguity. Callers can optionally filter by transport date range.

**Output:** "These objects usually change together with ZCL_PRICING"

**Value:** Upgrade wave planning, hidden module detection.

### 2. `impact` (multi-hop reverse-dependency traversal)

**Input:** object or config entry + max depth + optional edge kind filter
**Algorithm:**
1. Start from target node (e.g., `TVARVC:ZKEKEKE` or `CLAS:ZCL_FOO`)
2. BFS through **incoming** edges (`InEdges`) — "who depends on this?"
3. For each hop, record the edge kind and path
4. Optionally cross layer boundaries (config → code → transport)
5. Return subgraph with paths and depth

**Edge direction clarification:**
- `TVARVC:ZKEKEKE` ← `READS_CONFIG` ← `PROG:ZREPORT` (ZREPORT reads ZKEKEKE)
- Impact starts at ZKEKEKE, traverses incoming READS_CONFIG to find ZREPORT
- Then optionally continues: who calls ZREPORT? (incoming CALLS edges)

**Separate from depends-on:** `depends-on` would traverse OutEdges instead. These are two distinct queries.

**Output:** "Changing X affects these N objects across M packages"

**Value:** Change risk assessment, regression scope.

### 3. `where-used-config` (config variable impact)

**Input:** TVARVC variable name (e.g., `ZKEKEKE`)
**Algorithm:**
1. Find candidate programs referencing TVARVC table (from CROSS table, otype='DA')
2. For each candidate, grep source code for the literal string `ZKEKEKE`
3. Only build `READS_CONFIG` edge if grep confirms reference
4. Mark edge source as `TVARVC_CROSS` (heuristic, not exact metadata)
5. Optionally extend: who calls those programs? (traverse incoming CALLS edges)

**Confidence:** This is a heuristic query. False positives possible (variable name in comments). False negatives possible (dynamic name construction). Edge source `TVARVC_CROSS` signals this to consumers.

**Output:** "Variable ZKEKEKE is likely read by 3 programs in 2 packages"

**Value:** Config change impact — the STVARV use case.

---

## MCP Integration

```
SAP(action="analyze", params={"type": "what_changes_with", "object": "CLAS ZCL_PRICING"})
SAP(action="analyze", params={"type": "impact", "object": "CLAS ZCL_PRICING", "depth": 3})
SAP(action="analyze", params={"type": "impact", "object": "TVARVC ZKEKEKE", "depth": 2})
SAP(action="analyze", params={"type": "where_used_config", "variable": "ZKEKEKE"})
```

CLI:
```bash
vsp graph impact CLAS ZCL_PRICING --depth 3
vsp graph co-change CLAS ZCL_PRICING --top 20
vsp graph config-impact ZKEKEKE
```

---

## Implementation Order

| Step | What | Effort | Depends on |
|------|------|--------|------------|
| 1 | Extend `graph.go` with MVP NodeKind/EdgeKind constants | 2h | — |
| 2 | Transport adapter: `builder_transport.go` (E070/E071 → graph) | 4h | Step 1 |
| 3 | `what-changes-with` query (computed from IN_TRANSPORT, not stored) | 3h | Step 2 |
| 4 | CROSS/WBCROSSGT adapter: `builder_sql.go` | 4h | Step 1 |
| 5 | `impact` query (BFS on InEdges with depth limit) | 3h | Step 2+4 |
| 6 | Config adapter: `builder_config.go` (TVARVC, heuristic) | 4h | Step 4 |
| 7 | `where-used-config` query | 3h | Step 6 |
| 8 | MCP handlers + CLI commands | 4h | Steps 3+5+7 |
| **Total** | | **~27h** | |

### When to add SQLite persistence

Add `store_sqlite.go` when ANY of these become true:
- Full graph rebuild takes >10 seconds
- Users want offline graph queries (no SAP connection)
- Cross-session graph comparison is needed (diff between dates)
- Co-change history across months of transports is requested

Until then: in-memory per-request, rebuild each time.

---

## Enrichment / Signals Layer

**Principle:** Topology separate from enrichment separate from ranking.

Graph facts (edges) define reachable paths. Relevance signals (annotations) define priority. These must not be conflated — see [007: Graph Enrichment Signals Proposal](2026-04-05-007-graph-enrichment-signals-proposal.md).

**Implementation:**
- `Node.Meta map[string]any` carries per-node signals
- `builder_*` creates topology, `annotator_*` enriches nodes/edges
- Queries support raw mode (topology only) and ranked mode (with signals)

**MVP signals** (interface + constants only, one concrete annotator):
- `confidence` — exact metadata vs heuristic
- `is_standard` — SAP standard vs custom
- `last_transport_date` — from transport stats (computed from IN_TRANSPORT edges)

**Future signals** (post-MVP):
- `usage_score`, `exec_frequency` — from runtime stats
- `simplification_relevance` — from simplification lists
- `atc_finding_count` — from ATC checks
- `api_release_state` — from GetAPIReleaseState

---

## Limitations (MVP)

- **No full historical graph** — transports queried per-request, not accumulated over time
- **No generic query language** — 3 purpose-built queries, not Cypher/Gremlin
- **No auth layer** — deferred until edge semantics (SU24 vs runtime vs role) are resolved
- **No task-level analysis** — transport tasks collapsed into parent request
- **No stored co-change edges** — computed at query time to avoid cardinality issues
- **Config edges are heuristic** — TVARVC_CROSS source signals lower confidence than CROSS/E071
- **No ranked output in MVP** — enrichment framework defined, actual ranking deferred

## Open Questions (to resolve during implementation)

1. Should `what-changes-with` support time-range filtering on transports? (Probably yes, but adds complexity to query API)
2. Is NormalizeInclude sufficient for CROSS→object resolution, or do we need TADIR lookup? (Test with real CROSS data)
3. How to handle namespaced objects (`/UI5/CL_*`) in transport object lists? (E071.obj_name may have different formatting)
