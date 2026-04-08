# Session Wisdom: Analysis Sprint — Boundaries, Effects, Reports, Exports

**Date:** 2026-04-08
**Session scope:** Directional boundary crossings, side effect extraction, health reports, graph export formats, mermaid/DOT/PlantUML/GraphML, cache config, article, release fixes

---

## What Was Done

### Changelog & Changes — E07T Bug Fix + CR Correlation
- `changelog.go`: AS4TEXT lives in E07T, not E070 — separate query added
- `changes.go`: new command grouping transports by CTS attribute (E070A)
- Validated on SAP a4h-110-adt: SAPNOTE attribute, ZCUSTOM_DEVELOPMENT package

### Health Command — Major Overhaul
- **Test discovery fixed**: was filtering by class name containing "TEST" — wrong. Now uses SAP's native unit test runner via `/sap/bc/adt/packages/<name>` URI
- **Subpackage traversal**: uses `AcquirePackageScope` (TDEVC + prefix fallback) to scan full hierarchy
- **Progress indicators**: `[1/4] Running tests...` etc.
- **Details flag**: `--details` shows full test failures + ATC findings
- **Report formats**: `--format md/html`, `--report md/html/filename.ext`
- **File naming**: `$ZLLM` → `_ZLLM.md` ($ → _), `ZLLM` → `ZLLM.md`
- **Test details grouped by parent object**: ParentName/ParentType from XML program element
- **ATC location shown**: code line reference per finding

### Directional Boundary Crossing Analysis
- `pkg/graph/crossing.go` — 8 crossing directions: UPWARD, UPWARD_SKIP, COMMON, SIBLING, DOWNWARD, COMMON_DOWN, EXTERNAL, STANDARD
- Test package exemption (sibling crossings from test packages are OK)
- Circular sibling dependency detection
- Prefix fallback when TDEVC hierarchy unavailable
- `GuessPackageFromName` — infers package from Z-naming convention for unresolved targets
- 12+ unit tests
- `vsp boundaries <package>` — standalone CLI command
- All renderers show: source/target type, edge kind, ref detail, separate package columns
- EXTERNAL classified as WARN (not OK)

### Side Effect Extraction (Phase 1)
- `pkg/graph/effects.go` — `ExtractEffects()` detects 16+ patterns
- DB operations: SELECT, INSERT, UPDATE, DELETE, MODIFY
- LUW: COMMIT WORK, ROLLBACK WORK, IN UPDATE TASK, IN BACKGROUND TASK, SET UPDATE TASK LOCAL
- Async: STARTING NEW TASK, VIA JOB, AND RETURN
- External: RFC DESTINATION, HTTP client, APC/WebSocket
- Control flow: RAISE EXCEPTION, MESSAGE TYPE E/A/X, LEAVE TO TRANSACTION
- State: me-> instance attribute access
- LUW classification: safe / participant / owner / unsafe
- `IsPure()` for pure function detection
- 14 unit tests

### Parser — New Statement Types
- `CALL TRANSACTION 'VA01'` → TRAN:VA01 (EdgeCalls)
- `LEAVE TO TRANSACTION 'SM30'` → TRAN:SM30 (EdgeCalls)
- `CALL TRANSFORMATION zxslt` → XSLT:ZXSLT (EdgeCalls)
- Dynamic variants detected for all three
- New node types: TRAN, XSLT, MSAG
- 3 new test cases in TestExtractDepsFromSource

### Graph Export Formats
- `GraphToMermaid` — package subgraphs, node shapes by type, edge colors by kind
- `CrossingToMermaid` — crossing report with directional edge colors
- `ToDOT` — Graphviz with clustered subgraphs, shapes, edge styles
- `ToPlantUML` — package grouping, typed elements, colored edges
- `ToGraphML` — XML for Gephi/yEd with node/edge attributes
- All wired to `vsp boundaries --format dot/plantuml/graphml/mermaid/html`

### Release Pipeline Fix
- GoReleaser v2.15 dropped `changelog.use: git-cliff` → `changelog.disable: true`
- `changelog.skip: true` also invalid → must be `disable`
- `RELEASE_NOTES.md` made git dirty → added to `.gitignore`
- Released v2.38.0 and v2.38.1 successfully

### Cache Config (M1 partial)
- `SystemConfig.Cache` + `SystemConfig.CachePath` fields
- Resolution: config → per-system env → global env → default off
- Cache path auto-derived: `.vsp-cache/<system>.db`
- `vsp systems` shows cache status

### Article & README
- "VSP IS ONLY 5% EXPLORED" article — fused Codex + Claude versions
- 0x101 stars P.S.
- README updated: Hot Right Now section, Key Features table, CLI commands (35+)

---

## Gotchas and Non-Obvious Decisions

### ABAP Unit Tests Are NOT Separate Objects
Test classes are embedded as local test includes inside CLAS/FUGR/PROG. `IsTestCaller` name-based filter was wrong. Correct approach: pass package URI to SAP's unit test runner which discovers embedded tests automatically. BUT — SAP's runner only covers the exact package, not subpackages. Must expand hierarchy via `AcquirePackageScope` and run per-package.

### ADT Unit Test XML Program Element
The `<program>` element in ABAP Unit XML response has `uri`, `type`, `name` attributes identifying the parent object. Our parser captures these via `prog.URI/Type/Name`. BUT — after namespace stripping (`adtcore:` removal), the attributes must match. Need real XML sample to confirm attribute names on all SAP versions.

### Package Name Guessing
`GuessPackageFromName` strips ZCL_/ZIF_/ZCX_/ZTT_ prefixes, then takes first two `_`-separated segments. Works for `ZCL_LLM_00_CACHE` → `$ZLLM_00`. Fails for non-standard naming. Used as fallback when TADIR doesn't resolve.

### TADIR Resolution Limits
`RunQuery` defaults to 100 rows when maxRows=0. We hit this with 100+ unresolved targets. Fixed: batch in chunks of 100, maxRows = len(chunk)*3.

### GoReleaser v2 Changelog Config
Three iterations: `changelog.use: git-cliff` (dropped), `changelog.skip: true` (invalid field), `changelog.disable: true` (correct). Undocumented breaking change.

### Progress Line Artifacts
`\r` progress lines leave artifacts when new line is shorter. Fixed: `%-40s` padding on object names.

---

## Architecture Decisions Made

### Graph Engine vs Graph DB
- Own graph engine in Go for extraction + bounded propagation
- Graph DB (Neo4j etc.) as optional downstream consumer via export
- Export formats: DOT, PlantUML, GraphML, Mermaid, JSON
- Rationale: extraction is SAP-specific (can't delegate), graphs are small (50-500 nodes), deployment simplicity

### Boundary Crossing Directions
- UPWARD/COMMON = OK, SIBLING/DOWNWARD/COMMON_DOWN = BAD, EXTERNAL = WARN
- _00 suffix = common/shared package by convention
- Test packages exempt from sibling violation
- Circular detection via bidirectional sibling pair tracking

### Side Effects / LUW Model
- Properties of nodes, not edges
- Local extraction first, transitive propagation later
- LUW classification: safe/participant/owner/unsafe
- Key insight: `IN UPDATE TASK` is invisible deferred coupling — whoever COMMITs triggers all deferred writes

### Cache Architecture (ADR-005)
- SQLite opt-in cache, not source of truth
- Per-system DB file: `.vsp-cache/<system>.db`
- Config → env → default off
- Source hash as primary cache key
- Analyzer version for invalidation

---

## Key Artifacts

| Path | Description |
|------|-------------|
| `pkg/graph/crossing.go` | Directional crossing classifier + `AnalyzeCrossings` |
| `pkg/graph/crossing_test.go` | 12+ tests for all directions + circular + prefix |
| `pkg/graph/effects.go` | Side effect extractor + LUW classification |
| `pkg/graph/effects_test.go` | 14 tests covering all patterns |
| `pkg/graph/format.go` | Mermaid renderers (GraphToMermaid, CrossingToMermaid) |
| `pkg/graph/format_export.go` | DOT, PlantUML, GraphML exporters |
| `pkg/graph/builder_parser.go` | CALL TRANSACTION/TRANSFORMATION extractors |
| `cmd/vsp/devops.go` | Health reports, boundaries command, crossing renderers |
| `cmd/vsp/changelog.go` | E07T fix + transport history |
| `cmd/vsp/changes.go` | CR-level correlation via E070A |
| `reports/2026-04-08-001-boundary-crossing-direction-proposal.md` | Crossing direction ADR |
| `reports/2026-04-08-002-code-unit-contracts-and-purity-proposal.md` | Contracts + effects ADR |
| `reports/2026-04-08-004-sqlite-cache-wiring-plan.md` | Cache steering plan |
| `docs/adr/004-execution-semantics-and-effect-profiles.md` | Execution semantics ADR (Codex) |
| `docs/adr/005-opt-in-sqlite-analysis-cache.md` | Cache ADR (Codex) |
| `articles/2026-04-07-vsp-only-5-percent-explored.md` | 0x101 stars article |
