# VSP Roadmap 2026 — Consolidated Backlog

**Date:** 2026-04-06
**Report ID:** 2026-04-06-001
**Subject:** Consolidated roadmap from Claude, Codex, and Gemini perspectives
**Purpose:** Actionable backlog for next implementation sprints

---

## What Just Shipped (Graph Knowledge MVP)

Shipped in 5 commits (ae2f176..2a3b0a5):

- `pkg/graph/`: 12 new files — builders (transport, SQL, config), queries (co-change, impact, where-used-config), formatters (mermaid, html)
- 76+ unit tests in graph package, all passing
- CLI: `vsp graph co-change`, `vsp graph where-used-config` with text/json/mermaid/html output
- MCP: `co_change`, `impact`, `where_used_config` via analyze action
- Impact v2: CROSS/WBCROSSGT backbone + optional parser augmentation
- Design docs: 006 (graph MVP), 007 (enrichment signals)

This is the foundation everything below builds on.

---

## Priority 1: Usage Examples Tool

**Consensus:** All three agents (Claude, Codex, Gemini) independently rated this #1.

### What It Solves

| User Job | Current Tool | Gap |
|----------|-------------|-----|
| "How do I call this FM correctly?" | Grep / FindReferences | No parameter context, no snippets |
| "Show me real patterns for this method" | ADT call graph | Shows WHO calls, not HOW |
| "What parameters do callers actually pass?" | Nothing | Must read each caller manually |
| "Is anyone using this deprecated API?" | Where-used | Count only, no usage quality info |
| "How is this SUBMIT called? Which variants?" | Manual search | No automated collection |

### How It Differs From Existing Tools

- **Grep**: finds string matches including comments/strings. No semantic understanding.
- **FindReferences (ADT)**: returns where-used list with line numbers. No snippets.
- **Call graph**: shows caller tree structure. Zero source context.
- **Usage examples**: shows **actual call sites** with surrounding code, parameter blocks, and call patterns.

### MVP Design

**Input types:**
```
vsp examples FUNC Z_MY_FM
vsp examples CLAS ZCL_API METHOD GET_DATA
vsp examples INTF ZIF_API METHOD EXECUTE
vsp examples PROG ZREPORT FORM BUILD_OUTPUT
vsp examples TRAN VA01
```

**MCP:**
```
SAP(action="analyze", params={"type": "usage_examples", "object_type": "CLAS", "object_name": "ZCL_FOO", "method": "GET_DATA"})
```

**Algorithm:**
1. Reverse call graph: GetCallersOf (ADT) with WBCROSSGT/CROSS fallback
2. For top N callers (cap 10-15): GetSource with known type
3. Parser finds call sites matching target:
   - `CALL FUNCTION 'FM_NAME'` → extract EXPORTING/IMPORTING block
   - `zcl=>method( ... )` → extract parameter list
   - `PERFORM form IN PROGRAM prog` → extract USING/CHANGING
   - `SUBMIT prog WITH ...` → extract selection params
4. Return snippets with 3-5 lines context around each call site

**Ranking:**
- Test classes first (cleanest examples, per Gemini's insight about `LCL_TEST_CLASSES`)
- Same package callers before cross-package
- Shorter call path before deep chains
- Custom callers before standard SAP glue

**Output per example:**
```
Caller: CLAS ZCL_ORDER_SERVICE (method CREATE_ORDER) [$ZDEV]
Source: PARSER (HIGH confidence)
Snippet:
  42 |   DATA(lo_api) = NEW zcl_travel_api( ).
  43 |   lo_api->get_data(
  44 |     EXPORTING iv_id = lv_travel_id
  45 |     IMPORTING es_travel = ls_result
  46 |   ).
```

**Data sources:** WBCROSSGT/CROSS (backbone) → source fetch → parser extraction. ADT GetCallersOf as optional overlay.

**Risks:**
- Slow: 10 source fetches × 1-2s = 10-20s. Mitigate with cap + progress output.
- Noisy: popular FMs have 500+ callers. Cap at 10-15, prioritize custom.
- Parser completeness: extracting parameter blocks is non-trivial. MVP: show N lines around call site, don't parse params perfectly.

**Effort:** 2-3 days for MVP.

---

## Priority 2: Quick Wins (1 day each)

### 2a. Impact Diagram Export

Already have `CoChangeToMermaid` and `ConfigUsageToMermaid`. Need `ImpactToMermaid`:
- Root node highlighted, BFS layers shown as depth levels
- Edge labels: CALLS / REFERENCES / READS_CONFIG
- Node colors by depth

**Effort:** 2-3 hours. Same pattern as existing formatters.

### 2b. VSP Health Dashboard

```
vsp health $ZDEV
```

Orchestrates existing tools in one shot:
- `RunUnitTests` → pass/fail summary
- `RunATCCheck` → finding count by priority
- `CheckBoundaries` → violation count
- Transport recency from E070 → staleness indicator
- Object count from TADIR

Output: text table + JSON + HTML dashboard.

**Effort:** 1 day. All pieces exist, just needs orchestration.

### 2c. VSP Changelog

```
vsp changelog $ZDEV --since 20260101
```

- E070/E071/E07T for package → transports → descriptions
- Group by transport, show objects changed
- Sort by date descending
- Like `git log` but for SAP

Note: similar to existing ZBLAME program in some SAP systems.

**Effort:** 1 day. Pure SQL aggregation + formatting.

---

## Priority 3: Architecture Layer (3-5 days each)

### 3a. VSP Sketch — Architecture Diagrams

```
vsp sketch $ZDEV --format mermaid
```

- TADIR → object list
- WBCROSSGT/CROSS → deps between objects
- Generate Mermaid class diagram with:
  - Classes as nodes (with public methods)
  - Interfaces with implementation edges
  - Inheritance edges
  - Package grouping as subgraphs

**Effort:** 3 days. Graph infrastructure ready, needs class diagram formatter.

### 3b. Upgrade-Check — S/4HANA Readiness

```
vsp upgrade-check $ZDEV
```

For each object in package:
- `GetAPIReleaseState` → released / deprecated / not released
- `RunATCCheck` with cloud-readiness profile
- Cross-reference with known simplification items (bundled dataset or SAP note refs)

**Effort:** 3-5 days. GetAPIReleaseState already implemented.

### 3c. Cross-System Diff

```
vsp diff --system dev,qa
```

- Compare TADIR metadata between two configured systems
- Show objects in DEV but not QA (not transported)
- Show version mismatches
- Needs two system profiles in `.vsp.json` (already supported)

**Effort:** 3 days.

---

## Priority 4: Intelligence Layer (future)

### 4a. Historical Impact ("Time Machine")

"When this object was changed in the past, what broke?"

- E070/E071: transport history for object
- Correlate with: dumps (RABAX) around transport dates, ATC regression, co-change patterns
- Build temporal co-change graph: objects that changed together AND had incidents
- Output: risk score per dependency path

**Prerequisite:** Graph persistence (SQLite). Without it, historical analysis requires rebuilding per query.

### 4b. Semantic Diff-Impact

Not "object X changed" but "lines 42-58 of method GET_DATA changed → which callers pass values that flow through those lines?"

- Requires: source diff → affected method identification → parameter flow analysis
- Heavy: this is essentially lightweight taint analysis
- Could start simpler: "which method changed?" → impact from that method specifically

### 4c. Live ABAP REPL

Interactive `ExecuteABAP` with session state persistence:
- Execute snippets sequentially
- Variables persist across calls
- Results displayed inline

`ExecuteABAP` already works. Needs session wrapper + state serialization.

---

## What NOT to Prioritize (Explicit)

| Item | Why Not Now |
|------|------------|
| Graph DB (Neo4j/Kùzu) | No proven need. In-memory + export covers current use cases. |
| Auth/role graph layer | Semantics unresolved (SU24 ≠ runtime ≠ roles). Needs own design doc. |
| Generic query language (Cypher) | Domain queries are better. 5 targeted queries > 1 generic language. |
| D010INC/additional SQL adapters | Diminishing returns until current queries prove useful on real systems. |
| Persistence/caching | Wait until rebuild cost is proven painful. |

---

## Implementation Order (Next 2 Weeks)

| Week | What | Effort |
|------|------|--------|
| **Now** | Usage Examples MVP (CLI + MCP) | 2-3 days |
| **Now** | Impact diagram export (mermaid/html) | 2-3 hours |
| **Week 1** | VSP Health dashboard | 1 day |
| **Week 1** | VSP Changelog | 1 day |
| **Week 2** | VSP Sketch (architecture diagrams) | 3 days |
| **Week 2** | Upgrade-Check (S/4 readiness) | 3 days |
| **Later** | Historical Impact, Cross-System Diff, REPL | After stabilization |

---

## Sources

- **Claude (Opus 4.6):** Usage examples deep eval, tier 1-3 side-quests, diff-impact idea, REPL concept
- **Codex (GPT-5.4):** Graph MVP execution, co-change/impact/config implementation, conservative priority ordering, ADT overlay roadmap
- **Gemini (2.5 Pro):** "Time Machine" (historical impact), "Semantic Gravity" (co-change exploration), test-first examples idea (LCL_TEST_CLASSES), "Architecture Doctor" concept, heatmap overlay
- **ZBLAME reference:** Existing SAP program with similar changelog/blame functionality — validates the concept
