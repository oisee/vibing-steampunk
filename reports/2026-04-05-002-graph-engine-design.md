# Graph Engine Design — ZXRAY → vsp Port

**Date:** 2026-04-05
**Report ID:** 002
**Subject:** Go-native graph engine for dependency analysis, porting ZXRAY functionality
**Related:** Reports 001-007 (Dec 2025), ZXRAY source analysis

---

## 1. Motivation

ZXRAY на SAP делает:
1. Строит граф зависимостей через CROSS/WBCROSSGT
2. Кластеризует объекты по пакетам
3. Находит пересечения границ пакетов
4. Генерирует документацию через LLM

Проблема: ZXRAY привязан к SAP runtime. Перенос в vsp (Go-native) даёт:
- Работу без SAP GUI
- Интеграцию с AI-агентами через MCP
- Кеширование между вызовами
- Offline-анализ (после первичного сбора)

---

## 2. Ключевые Use Cases

### 2.1 Package Boundary Analysis (приоритет!)

**Вопрос:** "Пересекает ли разработка PROG границы пакета? Или упирается только в Стандарт?"

```
Входные данные:
  - Объект: ZPROG или пакет $ZDEV
  - Whitelist: ["$ZCOMMON", "$ZUTILS", "Z*_SHARED"]  (разрешённые Z-пакеты)

Анализ:
  1. Собрать все зависимости через CROSS/WBCROSSGT
  2. Для каждой зависимости определить пакет через TADIR
  3. Классифицировать:
     - STANDARD: зависимость на SAP-стандарт (не Z*) → ОК
     - SAME_PKG: зависимость внутри своего пакета → ОК
     - ALLOWED: зависимость на whitelist Z-пакет → ОК
     - VIOLATION: зависимость на чужой Z-пакет → ПРОБЛЕМА

Результат:
  ZDEV_MAIN_REPORT ($ZDEV):
    ✓ STANDARD: CL_GUI_ALV_GRID, BAPI_USER_GET_DETAIL
    ✓ SAME_PKG: ZCL_DEV_HELPER, ZDEV_UTILS
    ✓ ALLOWED:  ZCL_COMMON_LOGGER ($ZCOMMON)
    ✗ VIOLATION: ZCL_HR_PAYROLL ($ZHR) ← пересечение!
    ✗ VIOLATION: ZSALES_GET_DATA ($ZSALES) ← пересечение!
```

### 2.2 Impact Analysis

**Вопрос:** "Что сломается если я изменю ZCL_FOO?"

```
BuildGraph(ZCL_FOO, direction=UP, depth=3)
→ Все объекты, которые прямо или транзитивно зависят от ZCL_FOO
→ Группировка по пакетам, по критичности
```

### 2.3 Dependency Depth

**Вопрос:** "Насколько глубоко зарыта эта программа?"

```
BuildGraph(ZPROG, direction=DOWN, depth=5)
→ Дерево зависимостей с уровнями
→ "Корневые" объекты (ни от чего не зависят)
→ "Листья" (от них никто не зависит)
```

### 2.4 Cluster Detection

**Вопрос:** "Какие объекты образуют связные группы?"

```
BuildGraph($ZPACKAGE, direction=BOTH, depth=2)
→ Connected components = логические модули
→ Объекты-мосты (связывают кластеры) = точки сильной связности
```

---

## 3. Data Sources (SQL через ADT)

### 3.1 CROSS — классические перекрёстные ссылки

| TYPE | Что | Пример |
|------|-----|--------|
| F | Function Module call | `CALL FUNCTION 'Z_FM'` |
| R | Report/Program call | `SUBMIT ZPROG` |
| T | Transaction call | `CALL TRANSACTION 'ZTX'` |
| U | Subroutine/PERFORM | `PERFORM sub IN PROGRAM` |
| S | Screen reference | Dynpro reference |
| N | Message | `MESSAGE E001(ZMG)` |
| P | Type/Data | Type reference |

```sql
SELECT type, name, include FROM cross
WHERE include = 'ZPROG'
```

### 3.2 WBCROSSGT — global type перекрёстные ссылки

| OTYPE | Что | Пример |
|-------|-----|--------|
| ME | Method call | `ZCL_FOO=>METHOD()` |
| TY | Type usage | `TYPE REF TO ZCL_FOO` |
| DA | Data access | Method parameter usage |
| EV | Event | Event handler registration |
| TK | Token | Various tokens |

```sql
SELECT otype, name, include FROM wbcrossgt
WHERE include = 'ZCL_FOO========CP'
```

### 3.3 D010INC — Static Load Dependencies (NEW!)

**Что:** Какие includes загружает программа при компиляции. Это **реальный граф загрузки**, не только вызовы.

```sql
SELECT master, include FROM d010inc WHERE master = 'ZABAPGIT'
```

**Результат (A4H live):** ZABAPGIT загружает 50+ includes:
- `CX_SALV_*=====CT` — class test includes
- `CX_SHM_*======CU` — class public sections
- `CX_SFW_*======CU` — switch framework classes

**Суффиксы include-имён:**
| Suffix | Meaning |
|--------|---------|
| `CP` | Class Pool (main) |
| `CU` | Class pUblic section |
| `CO` | Class prOtected section |
| `CI` | Class prIvate section |
| `CT` | Class Testclass include |
| `CM` | Class Method include |
| `FP` | Function Pool |

**Ценность для graph engine:**
- CROSS/WBCROSSGT показывают **что вызывается** (runtime deps)
- D010INC показывает **что загружается** (compile-time deps)
- Разница важна: программа может загружать class pool для TYPE определения, но не вызывать ни один метод

**Стандартный отчёт:** `RSINCL00` — "ABAP Program Reference List" — SAP GUI визуализация D010INC.

**Edge type:** `RefType: "LOAD"` — compile-time dependency, дополняет F/R/ME/TY из CROSS/WBCROSSGT.

### 3.4 TADIR — Object ↔ Package mapping

```sql
SELECT obj_name, devclass FROM tadir
WHERE pgmid = 'R3TR' AND obj_name = 'ZCL_FOO'
```

### 3.4 Verified on A4H 758

Live test выполнен 2026-04-05:
- CROSS: `ZADT_WASM_TEST` → `SSFC_BASE64_DECODE` (F, стандарт) ✓
- WBCROSSGT: `ZABAPGIT` → 50+ method/data refs ✓
- TADIR: Объекты в `$TMP`, `$Z`, `$ZGIT`, `$DEMO_SOI_DRAFT` ✓
- JOIN cross+tadir для package resolution ✓

---

## 4. Architecture

```
pkg/graph/
├── graph.go          # Core: Node, Edge, Graph (in-memory adjacency list)
├── builder.go        # Build graph from CROSS/WBCROSSGT via ADT SQL queries
├── query.go          # Query API: impact, boundaries, paths, clusters
├── boundary.go       # Package boundary analysis (whitelist logic)
├── formatter.go      # Output: text summary, JSON, DOT (graphviz)
└── graph_test.go     # Unit tests with mock data
```

### 4.1 Core Types

```go
type Node struct {
    ID      string  // "CLAS:ZCL_FOO", "PROG:ZPROG", "FUGR:Z_FM"
    Name    string  // ZCL_FOO
    Type    string  // CLAS, PROG, FUGR, TABL, etc.
    Package string  // $ZDEV
}

type Edge struct {
    From    string  // Source node ID
    To      string  // Target node ID
    RefType string  // F, R, ME, TY, DA, etc.
    Include string  // Where the reference occurs
}

type Graph struct {
    Nodes map[string]*Node
    Edges []*Edge
    // Indexes for fast lookup
    outEdges map[string][]*Edge  // from → edges
    inEdges  map[string][]*Edge  // to → edges
}
```

### 4.2 Builder (hybrid: ADT-native + SQL enrichment)

**Strategy:** Use existing ADT APIs first (already implemented!), enrich with SQL tables.

```go
type Builder struct {
    client *adt.Client
    graph  *Graph
    cache  cache.Cache  // reuse pkg/cache
}

// Build from a starting point
func (b *Builder) BuildFromObject(ctx context.Context, objType, objName string, opts BuildOpts) error
func (b *Builder) BuildFromPackage(ctx context.Context, packageName string, opts BuildOpts) error

type BuildOpts struct {
    Direction    string   // "down", "up", "both"
    MaxDepth     int      // 1-5
    MaxNodes     int      // Safety limit (default 500)
    CrossTypes   []string // Filter CROSS types: "F", "R", "ME", etc.
    IncludeStd   bool     // Include standard objects (default: false)
    Sources      []string // Data sources: "adt", "cross", "wbcrossgt", "d010inc", "all"
}

// Data collection layers (executed in order):
//
// Layer 1: ADT-native APIs (ALREADY IMPLEMENTED in pkg/adt!)
//   → GetCallGraph(callers/callees)    — call hierarchy
//   → FindReferences()                 — where-used (generic)
//   → GetCDSDependencies()             — CDS forward deps
//   → GetCDSImpactAnalysis()           — CDS reverse deps
//
// Layer 2: SQL enrichment (NEW — fills gaps ADT API doesn't cover)
//   → CROSS table                      — FM calls, SUBMIT, transactions, messages
//   → WBCROSSGT table                  — method calls, type refs, data refs
//   → D010INC table                    — compile-time include loads
//
// Layer 3: Package resolution
//   → TADIR                            — object → package mapping
//   → D010PACKAGE                      — program → package (alternative)

```

### 4.3 Query API

```go
// Package boundary analysis
func (g *Graph) CheckBoundaries(rootPackage string, whitelist []string) *BoundaryReport

type BoundaryReport struct {
    RootPackage string
    TotalDeps   int
    Standard    []BoundaryEntry  // SAP standard deps → OK
    SamePackage []BoundaryEntry  // Same package deps → OK
    Allowed     []BoundaryEntry  // Whitelist Z-deps → OK
    Violations  []BoundaryEntry  // Cross-package Z-deps → PROBLEM
}

type BoundaryEntry struct {
    From    *Node
    To      *Node
    RefType string
    Package string
}

// Impact analysis
func (g *Graph) Impact(nodeID string) []*Node  // All transitive dependents (UP)

// Path finding
func (g *Graph) Paths(from, to string) [][]string  // All paths between nodes

// Cluster detection
func (g *Graph) Clusters() [][]string  // Connected components

// Hotspots
func (g *Graph) Hotspots(topN int) []*HotspotEntry  // Most referenced nodes
```

### 4.4 MCP Tools

```
# Build graph
SAP(action="analyze", params={
    "type": "build_graph",
    "object": "CLAS ZCL_FOO",     // or "DEVC $ZDEV"
    "direction": "both",
    "depth": 3
})

# Check package boundaries
SAP(action="analyze", params={
    "type": "check_boundaries",
    "package": "$ZDEV",
    "whitelist": ["$ZCOMMON", "$ZUTILS"]
})

# Impact analysis
SAP(action="analyze", params={
    "type": "impact",
    "object": "CLAS ZCL_FOO"
})

# Graph statistics
SAP(action="analyze", params={
    "type": "graph_stats"
})
```

### 4.5 CLI Commands

```bash
vsp graph build ZCL_FOO --depth 3 --direction both
vsp graph boundaries $ZDEV --allow '$ZCOMMON,$ZUTILS'
vsp graph impact ZCL_FOO
vsp graph export --format dot | dot -Tpng -o graph.png
```

---

## 5. Implementation Plan

### Phase 1: Core Graph + Package Boundaries (6-8h) ← revised down!

Key insight: ADT APIs already implemented. We're building a **graph layer on top**, not from scratch.

| Task | Est. | Description |
|------|------|-------------|
| `graph.go` — core types | 1h | Node, Edge, Graph, adjacency indexes |
| `builder_adt.go` — ADT API bridge | 1-2h | Wrap existing GetCallGraph/FindReferences → Graph edges |
| `builder_sql.go` — SQL enrichment | 2h | CROSS + WBCROSSGT + D010INC queries → Graph edges |
| `boundary.go` — package analysis | 1.5h | TADIR resolution, whitelist, classify, report |
| `graph_test.go` — unit tests | 1h | Mock data, boundary tests |
| MCP tool wiring | 0.5h | Wire into handlers_analysis.go |

**Why faster:** `GetCallGraph`, `FindReferences`, `GetCDSDependencies`, `FlattenCallGraph`, `AnalyzeCallGraph` — all exist. Builder just converts their output to Graph nodes/edges.

### Phase 2: Advanced Queries (5-8h)

| Task | Est. | Description |
|------|------|-------------|
| Impact analysis (transitive UP) | 2h | BFS/DFS with cycle detection |
| Path finding (A→B) | 2h | BFS shortest path + all paths |
| Cluster detection | 1-2h | Connected components via union-find |
| Hotspot analysis | 1h | Sort by in-degree + out-degree |
| Cache integration | 1h | Persist graph in pkg/cache |

### Phase 3: Visualization & CLI (4-6h)

| Task | Est. | Description |
|------|------|-------------|
| DOT/Graphviz export | 1h | Generate .dot file |
| Text summary formatter | 1h | Human-readable boundary report |
| CLI `vsp graph` commands | 2h | build, boundaries, impact, export |
| JSON export | 0.5h | For external tools |

### Total: 17-24h

---

## 6. Comparison: ZXRAY (ABAP) vs vsp Graph Engine (Go)

| Aspect | ZXRAY | vsp Graph Engine |
|--------|-------|-----------------|
| Runtime | SAP kernel | Go binary (local) |
| Data source | Direct SELECT | ADT SQL queries |
| Caching | Custom Z-tables | pkg/cache (memory/SQLite) |
| Graph build | ABAP internal tables | Go maps + slices |
| LLM integration | ZLLM (ABAP) | MCP (native) |
| Package analysis | Manual reports | Automated boundary check |
| Visualization | ALV list | DOT/Graphviz + JSON |
| Offline mode | No | Yes (after first build) |
| Performance | Fast (local DB) | Network-bound (SQL via HTTP) |

### What We Port

| ZXRAY Feature | → vsp Equivalent |
|--------------|-------------------|
| `ZCL_XRAY_GRAPH::build_down()` | `Builder.BuildFromObject(DOWN)` |
| `ZCL_XRAY_GRAPH::process_down()` | `Builder.queryCross() + queryWbcrossgt()` |
| CROSS type handlers | `Edge.RefType` classification |
| WBCROSSGT otype handlers | `Edge.RefType` classification |
| Package-level summary | `Graph.CheckBoundaries()` |
| Node documentation (LLM) | Separate MCP flow (already works) |

### What We Add (not in ZXRAY)

| Feature | Value |
|---------|-------|
| Package boundary whitelist | "Allow deps on Z*COMMON, flag others" |
| Transitive impact analysis | "What breaks if I change ZCL_FOO" |
| Path finding | "How does ZPROG reach BAPI_X" |
| Persistent cache | Graph survives between sessions |
| Graphviz export | Visual dependency diagrams |
| CLI interface | `vsp graph boundaries $ZDEV` |

---

## 7. Graph Query Proposal — Package Boundary Analysis

### 7.1 MCP Tool Interface

```
# Полный анализ пакета на пересечение границ
SAP(action="analyze", params={
    "type": "check_boundaries",
    "package": "$ZDEV",
    "depth": 2,
    "whitelist": ["$ZCOMMON", "$ZUTILS"],
    "sources": ["adt", "cross", "wbcrossgt", "d010inc"]
})
```

### 7.2 Пример вывода — Package Boundary Report

```
╔══════════════════════════════════════════════════════════════╗
║  Package Boundary Analysis: $ZDEV                           ║
║  Objects: 12 | Dependencies: 87 | Depth: 2                  ║
╠══════════════════════════════════════════════════════════════╣

─── CLEAN (no boundary violations) ──────────────────────────

  ✓ ZCL_DEV_SERVICE ($ZDEV)
    → CL_GUI_ALV_GRID (STANDARD)
    → CL_SALV_TABLE (STANDARD)
    → ZCL_DEV_HELPER ($ZDEV, same package)
    → ZCL_COMMON_LOGGER ($ZCOMMON, whitelisted)

  ✓ ZCL_DEV_HELPER ($ZDEV)
    → CL_ABAP_STRUCTDESCR (STANDARD)
    → ZIF_DEV_TYPES ($ZDEV, same package)

  ✓ ZDEV_MAIN_REPORT ($ZDEV)
    → ZCL_DEV_SERVICE ($ZDEV, same package)
    → SSFC_BASE64_DECODE (STANDARD, FM)

─── VIOLATIONS (cross-package Z* dependencies) ──────────────

  ✗ ZCL_DEV_EXPORT ($ZDEV)
    → ZCL_HR_PAYROLL ($ZHR) ← VIOLATION
      via: METHOD call (WBCROSSGT ME)
      suggestion: extract interface or move to $ZCOMMON

  ✗ ZCL_DEV_IMPORT ($ZDEV)
    → ZCL_SALES_HELPER ($ZSALES) ← VIOLATION
      via: TYPE REF TO (WBCROSSGT TY)
      suggestion: depend on interface, not implementation
    → ZSALES_GET_DATA ($ZSALES) ← VIOLATION
      via: CALL FUNCTION (CROSS F)
      suggestion: wrap in local adapter

─── COMPILE-TIME LOADS (D010INC) ────────────────────────────

  ⚠ ZDEV_MAIN_REPORT loads 23 includes:
    18 standard (CX_*, CL_*=====CU) — OK
    3 same package (ZDEV_*) — OK
    2 external Z*:
      → ZCL_HR_UTILS================CP ($ZHR) ← implicit dep!
      → ZCL_SALES_TYPES=============CT ($ZSALES) ← implicit dep!

─── SUMMARY ─────────────────────────────────────────────────

  Total objects analyzed:     12
  Total dependencies:         87
  Standard (SAP):             54 (62%)
  Same package ($ZDEV):       22 (25%)
  Whitelisted ($ZCOMMON):      5 (6%)
  VIOLATIONS:                  6 (7%) ← 3 objects affected

  Packages crossed:
    $ZHR    — 3 refs (2 explicit, 1 compile-time)
    $ZSALES — 3 refs (2 explicit, 1 compile-time)

  Risk: MEDIUM — 3 out of 12 objects have cross-package deps
╚══════════════════════════════════════════════════════════════╝
```

### 7.3 ADT API Calls Behind the Scenes

Для одного `check_boundaries($ZDEV)` graph engine выполняет:

```
Step 1: List package objects
  ADT: GET /sap/bc/adt/packages/$ZDEV/objecttypes (already in vsp)
  → 12 objects: ZCL_DEV_SERVICE, ZCL_DEV_HELPER, ZDEV_MAIN_REPORT, ...

Step 2: Get call graphs (ADT-native, already implemented)
  For each object:
    ADT: POST /sap/bc/adt/cai/callgraph (direction=callees, depth=2)
    → Call trees merged into graph

Step 3: Get where-used (ADT-native, already implemented)
  ADT: POST /sap/bc/adt/repository/informationsystem/usageReferences
  → Reverse deps for impact awareness

Step 4: Enrich with SQL tables (NEW)
  SQL: SELECT type, name, include FROM cross WHERE include IN (object_includes)
  SQL: SELECT otype, name, include FROM wbcrossgt WHERE include IN (object_includes)
  SQL: SELECT master, include FROM d010inc WHERE master IN (object_programs)
  → Compile-time deps, message refs, transaction refs

Step 5: Resolve packages
  SQL: SELECT obj_name, devclass FROM tadir WHERE obj_name IN (all_dep_names)
  → Package for each dependency

Step 6: Classify
  For each edge:
    target starts with Z/Y AND target_package != $ZDEV AND target_package NOT IN whitelist
    → VIOLATION
```

### 7.4 CLI Usage

```bash
# Basic boundary check
vsp graph boundaries $ZDEV

# With whitelist
vsp graph boundaries $ZDEV --allow '$ZCOMMON,$ZUTILS,$ZFRAMEWORK'

# Include compile-time deps
vsp graph boundaries $ZDEV --sources all --allow '$ZCOMMON'

# JSON output (for CI/CD pipeline)
vsp graph boundaries $ZDEV --format json | jq '.violations'

# Specific object instead of whole package
vsp graph boundaries --object ZCL_DEV_SERVICE --depth 3

# Just violations (for quality gate)
vsp graph boundaries $ZDEV --violations-only
# exit code 0 = clean, 1 = violations found
```

### 7.5 CI/CD Quality Gate

```yaml
# In CI pipeline: fail if package boundaries are violated
- name: Check package boundaries
  run: |
    vsp graph boundaries $ZDEV \
      --allow '$ZCOMMON,$ZUTILS' \
      --violations-only \
      --format json > boundary-report.json
    
    # Fail pipeline if violations found
    VIOLATIONS=$(jq '.violation_count' boundary-report.json)
    if [ "$VIOLATIONS" -gt 0 ]; then
      echo "❌ $VIOLATIONS package boundary violations found"
      jq '.violations[]' boundary-report.json
      exit 1
    fi
```

### 7.6 Graph Query Language (future)

Beyond boundaries, the graph supports rich queries:

```
# All paths from object A to object B
vsp graph path ZCL_DEV_SERVICE BAPI_USER_GET_DETAIL

# Impact: what breaks if I change ZCL_FOO?
vsp graph impact ZCL_FOO --depth 3

# Hotspots: most-referenced objects in package
vsp graph hotspots $ZDEV --top 10

# Clusters: logical groups within package
vsp graph clusters $ZDEV

# Orphans: objects with no inbound references
vsp graph orphans $ZDEV

# Export for visualization
vsp graph export $ZDEV --format dot | dot -Tsvg -o deps.svg
```

---

## 8. Alignment with Codex Report 003

Report 003 (`graph-engine-alignment-for-claude.md`) adds critical constraints:

### 8.1 Existing Code to Reuse (NOT reinvent!)

| Existing Code | What It Does | → Graph Engine Role |
|--------------|-------------|---------------------|
| `cmd/vsp/cli_deps.go` (`vsp deps`) | Package dep classification: internal/external/standard | → Extract into `pkg/graph/boundary.go` |
| `cmd/vsp/cli_extra.go` (`vsp graph`) | CLI graph from CROSS/WBCROSSGT + TSTC resolution | → Extract into `pkg/graph/builder_sql.go` |
| `pkg/ctxcomp/analyzer.go` | Multi-layer dep analysis (regex→parser→CROSS→ADT) | → Reuse layer strategy, don't duplicate |
| `pkg/adt/client.go` (CallGraph) | ADT call graph + flatten + analyze + compare | → Wrap as `builder_adt.go` adapter |
| `pkg/adt/codeintel.go` (FindReferences) | Where-used via ADT | → Wrap as UP-direction adapter |
| `pkg/adt/cds.go` / `cds_tools.go` | CDS forward deps + reverse impact | → Wrap as CDS adapter |

### 8.2 Edge Semantics (from Report 003)

```go
type EdgeKind string

const (
    EdgeCalls           EdgeKind = "CALLS"             // FM call, method call, SUBMIT
    EdgeReferences      EdgeKind = "REFERENCES"        // TYPE REF TO, DATA TYPE
    EdgeLoads           EdgeKind = "LOADS"              // D010INC compile-time include
    EdgeContainsInclude EdgeKind = "CONTAINS_INCLUDE"   // Program structure (include hierarchy)
    EdgeDependsOnCDS    EdgeKind = "DEPENDS_ON_CDS"     // CDS view dependency
)

type EdgeSource string

const (
    SourceADTCallGraph  EdgeSource = "ADT_CALL_GRAPH"
    SourceADTWhereUsed  EdgeSource = "ADT_WHERE_USED"
    SourceADTCDSDeps    EdgeSource = "ADT_CDS_DEPS"
    SourceCROSS         EdgeSource = "CROSS"
    SourceWBCROSSGT     EdgeSource = "WBCROSSGT"
    SourceD010INC       EdgeSource = "D010INC"
    SourceParser        EdgeSource = "PARSER"
    SourceTrace         EdgeSource = "TRACE"
)
```

### 8.3 Revised Architecture (unified, not greenfield)

```
pkg/graph/
├── graph.go          # Core: Node, Edge (with Kind + Source), Graph
├── builder_adt.go    # Adapter: existing GetCallGraph/FindReferences/CDS → Graph
├── builder_sql.go    # Adapter: CROSS/WBCROSSGT/D010INC SQL → Graph
├── boundary.go       # Extracted from cmd/vsp/cli_deps.go + enhanced
├── query.go          # Impact, paths, clusters (Phase 2)
├── formatter.go      # Text/JSON/DOT output
└── graph_test.go     # Unit tests
```

### 8.4 Revised Delivery (aligned with Report 003)

| Slice | What | Est. | Risk |
|-------|------|------|------|
| **1. Refactor** | Extract from cli_deps + cli_extra into pkg/graph | 4-6h | Low (code exists) |
| **2. Boundary MVP** | Package boundary + whitelist + text/JSON report | 5-7h | Low |
| **3. Impact/Path** | Transitive callers, shortest path | 4-6h | Medium |
| **4. Dynamic overlay** | Trace edges, static vs actual comparison | 4-5h | Medium |
| **Total** | | **17-24h** | |

---

## 9. Design Decision: Include-Level vs Object-Level (from Codex)

**Decision:** Dual-level model. Store raw, present aggregated.

```go
type Node struct {
    ID       string   // Object-level: "CLAS:ZCL_FOO"
    Name     string   // ZCL_FOO
    Type     string   // CLAS, PROG, FUGR, TABL, DDLS, ...
    Package  string   // $ZDEV
    Includes []string // Raw includes: ["ZCL_FOO========CP", "ZCL_FOO========CU", "ZCL_FOO========CM001"]
}

type Edge struct {
    From       string     // Object-level node ID
    To         string     // Object-level node ID
    Kind       EdgeKind   // CALLS, REFERENCES, LOADS, ...
    Source     EdgeSource // ADT_CALL_GRAPH, CROSS, D010INC, ...
    RawInclude string     // Original include where ref occurs (detail)
    RefDetail  string     // e.g. "METHOD:GET_DATA" or "FM:BAPI_USER_GET_DETAIL"
}
```

**Why dual-level:**
- CROSS/WBCROSSGT/D010INC all work at **include level** (`ZCL_FOO========CP`)
- Users think at **object level** (`ZCL_FOO`)
- Boundary analysis needs object→package (TADIR works at object level)
- But debugging false positives needs include detail ("which method caused this dep?")

**Normalization rules:**
```
ZCL_FOO========CP     → CLAS:ZCL_FOO
ZCL_FOO========CU     → CLAS:ZCL_FOO
ZCL_FOO========CM001  → CLAS:ZCL_FOO (method include)
SAPL_ZFUGR            → FUGR:ZFUGR (function pool)
L_ZFUGR_U01           → FUGR:ZFUGR (function include)
ZREPORT               → PROG:ZREPORT
ZREPORT_F01           → PROG:ZREPORT (INCLUDE)
```

**API default:** Object-level graph. Detail expansion on demand:
```
SAP(action="analyze", params={"type": "check_boundaries", "package": "$ZDEV"})
→ Object-level report (default)

SAP(action="analyze", params={"type": "check_boundaries", "package": "$ZDEV", "detail": "include"})
→ Include-level detail (shows which method/include caused each edge)
```

---

## 10. Implementation Status (2026-04-05)

### Slice 1+2: DONE ✅

| File | LOC | What |
|------|-----|------|
| `pkg/graph/graph.go` | ~200 | Core: Node/Edge with Kind+Source provenance, Graph with adjacency indexes, include→object normalization |
| `pkg/graph/builder_parser.go` | ~340 | Parser-based dep extraction: 8 static types (FM, SUBMIT, CREATE OBJECT, TYPE REF TO, SELECT FROM, INCLUDE, static method, INHERITS) + 3 dynamic call types (dynamic FM, SUBMIT, CREATE) |
| `pkg/graph/boundary.go` | ~250 | Package boundary analyzer: whitelist with glob, 6 verdicts (STANDARD/SAME_PACKAGE/ALLOWED/VIOLATION/DYNAMIC/UNKNOWN), text+JSON formatter, CI exit codes |
| `pkg/graph/graph_test.go` | ~210 | 6 tests: normalize include, standard detection, graph CRUD, stats, parser extraction, dynamic calls |
| `pkg/graph/boundary_test.go` | ~200 | 5 tests: clean, violations, whitelist glob, dynamic warnings, full end-to-end (parser→graph→boundary) |
| `internal/mcp/handlers_graph.go` | ~240 | MCP handlers: CheckBoundaries (3 modes), GraphStats; TADIR batch resolution |
| `internal/mcp/tools_register.go` | +30 | Tool registration for CheckBoundaries, GraphStats |
| `internal/mcp/tools_focused.go` | +1 | CheckBoundaries in focused tool set |
| `internal/mcp/handlers_analysis.go` | +4 | Universal route: check_boundaries, graph_stats |
| **Total** | **~1500** | **11 tests, 0.002s** |

### MCP Access (3 modes)

```
# Offline — analyze source code (no SAP needed!)
SAP(action="analyze", params={
    "type": "check_boundaries",
    "package": "$ZDEV",
    "source": "REPORT ztest.\nCALL FUNCTION 'Z_OTHER_FM'.\nzcl_hr=>method().",
    "whitelist": "$ZCOMMON"
})

# Online — single object (reads source from SAP)
SAP(action="analyze", params={
    "type": "check_boundaries",
    "object": "ZCL_DEV_SERVICE",
    "whitelist": "$ZCOMMON,$ZUTILS"
})

# Online — whole package (reads all objects)
SAP(action="analyze", params={
    "type": "check_boundaries",
    "package": "$ZDEV",
    "whitelist": "$ZCOMMON",
    "format": "json"
})
```

### Key Innovation: Parser as 4th Data Source

The embedded ABAP parser (`pkg/abaplint`) provides offline dependency extraction that **catches what CROSS/WBCROSSGT miss**:

| What Parser Catches | CROSS/WBCROSSGT | Parser |
|---------------------|-----------------|--------|
| `CALL FUNCTION 'Z_FM'` (static) | ✓ | ✓ |
| `CALL FUNCTION lv_var` (dynamic) | ✗ | ✓ ← **unique** |
| `SUBMIT (lv_prog)` (dynamic) | ✗ | ✓ ← **unique** |
| `CREATE OBJECT TYPE (lv_class)` (dynamic) | ✗ | ✓ ← **unique** |
| `PERFORM sub IN PROGRAM ext_prog` (cross-program) | Sometimes ✗ | ✓ |
| `TYPE REF TO zcl_foo` | via WBCROSSGT | ✓ |
| Stale cross-references | Possible | Never (parses live source) |

---

## 11. Next Slice: Advanced Queries

### Slice 3: Impact & Path Queries (est. 4-6h)

```go
// pkg/graph/query.go

// Impact: all transitive dependents (BFS up)
func (g *Graph) Impact(nodeID string, maxDepth int) []*Node

// Paths: all paths between two nodes
func (g *Graph) Paths(from, to string, maxDepth int) [][]string

// Hotspots: most-referenced nodes
func (g *Graph) Hotspots(topN int) []HotspotEntry

// Orphans: nodes with no inbound edges
func (g *Graph) Orphans() []*Node
```

**MCP tools:**
```
SAP(action="analyze", params={"type": "impact", "object": "ZCL_FOO", "depth": 3})
SAP(action="analyze", params={"type": "path", "from": "ZCL_A", "to": "BAPI_X"})
SAP(action="analyze", params={"type": "hotspots", "package": "$ZDEV", "top": 10})
```

### Slice 4: SQL Enrichment Adapters (est. 3-4h)

```go
// pkg/graph/builder_sql.go — uses existing ADT RunQuery

func (b *SQLBuilder) AddFromCROSS(ctx context.Context, includes []string) error
func (b *SQLBuilder) AddFromWBCROSSGT(ctx context.Context, includes []string) error
func (b *SQLBuilder) AddFromD010INC(ctx context.Context, programs []string) error
func (b *SQLBuilder) ResolvePackages(ctx context.Context) error  // TADIR batch
```

### Slice 5: ADT API Adapters (est. 2-3h)

```go
// pkg/graph/builder_adt.go — wraps existing pkg/adt methods

func (b *ADTBuilder) AddFromCallGraph(ctx context.Context, objectURI string, opts CallGraphOpts) error
func (b *ADTBuilder) AddFromWhereUsed(ctx context.Context, objectURL string) error
func (b *ADTBuilder) AddFromCDSDeps(ctx context.Context, ddlsName string) error
```

### Slice 6: Visualization & Export (est. 2-3h)

```go
// pkg/graph/formatter.go
func (g *Graph) ExportDOT() string      // Graphviz DOT format
func (g *Graph) ExportJSON() string     // Full graph as JSON
func (g *Graph) ExportMermaid() string  // Mermaid diagram syntax
```

### Slice 7: Query DSL (future, est. 4-6h)

```
vsp graph query "FROM $ZDEV WHERE kind=VIOLATION"
vsp graph query "PATH ZCL_A → BAPI_X"
vsp graph query "IMPACT ZCL_FOO DEPTH 3"
vsp graph query "HOTSPOTS $ZDEV TOP 10"
```

---

## 12. Open Questions

1. **Include-level or Object-level granularity?**
   - CROSS works at include level (ZCL_FOO========CP)
   - Graph probably wants object level (ZCL_FOO)
   - Need mapping: include → object (via naming conventions or TADIR)

2. **How to handle SAP standard references?**
   - Default: exclude (focus on Z* deps)
   - Option: include standard, classify separately
   - Option: include only "boundary" standard objects (first-level)

3. **Graph persistence format?**
   - JSON (portable, readable)
   - SQLite (queryable, fast)
   - Both? (cache in SQLite, export as JSON)

4. **Maximum graph size?**
   - Large packages can have 1000+ objects
   - Need pagination/streaming for SQL queries
   - Memory limit: 500 nodes default, configurable
