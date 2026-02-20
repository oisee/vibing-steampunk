# ADT API Gaps — Implementation Roadmap

**Created:** 2026-02-21
**Based on:** [docs/ADT-GAP-ANALYSIS.md](ADT-GAP-ANALYSIS.md), [docs/spikes/2026-02-21-adt-gaps-deep-research.md](spikes/2026-02-21-adt-gaps-deep-research.md)
**Current tools:** 103+ (some tools not counted in original analysis: GetView, GetTypeHierarchy, DebuggerSetVariableValue)
**Target:** ~115 (12 genuinely new tools)

---

## Phase Overview

| Phase | Focus | New Tools | Effort | Value |
|-------|-------|-----------|--------|-------|
| **Phase 1** | Refactoring | 5 | 15 | HIGH — AI-native code transformation |
| **Phase 2** | Testing & Quality | 3 | 12 | HIGH — Coverage + auto-fix pipeline |
| **Phase 3** | CDS/RAP Completeness | 4 | 13 | MEDIUM — Full RAP development lifecycle |
| **Phase 4** | DDIC & Misc | 3+ | 8+ | LOW — Completeness |

**Total estimated effort:** ~48 (implementation units, not hours)

---

## Phase 1: Refactoring (5 tools) ✅ COMPLETED

**Why first:** Highest AI automation value. Rename + Extract Method + Quick Fix create a transformative code modification pipeline that no competitor has.

**Implementation date:** 2026-02-21
**New files:** `pkg/adt/refactoring.go`, `pkg/adt/quickfix.go`, `internal/mcp/handlers_refactoring.go`, `pkg/adt/refactoring_test.go`, `pkg/adt/quickfix_test.go`
**New tests:** 33 (19 refactoring + 14 quickfix)
**Tools added:** RenameRefactoring, ExtractMethod, GetQuickFixProposals, ApplyQuickFix, ApplyATCQuickFix

### Step 1.1: ADT-Native Rename Refactoring ✅
> **Note:** `RenameObject` exists in `workflows.go` as a composite (copy→delete). The ADT-native refactoring endpoint does evaluate→preview→execute with full cross-reference support. This is a SEPARATE, better approach.

- [x] Create `pkg/adt/refactoring.go`
  - `RenameEvaluate(ctx, objectURI, line, col, source, newName)` → problems, changeCount
  - `RenamePreview(ctx, objectURI, line, col, source, newName)` → []RenameChange
  - `RenameExecute(ctx, objectURI, line, col, source, newName)` → result
- [x] Add types: `RenameProblem`, `RenameChange`, `RenameEvaluateResult`, `RenamePreviewResult`, `RenameExecuteResult`
- [x] Create `internal/mcp/handlers_refactoring.go`
- [x] Register MCP tool: `RenameRefactoring` (single tool, `step` parameter: evaluate/preview/execute)
- [x] Unit tests with mock XML responses (7 tests)
- [ ] Integration test: rename method in test class (requires SAP system)

### Step 1.2: Extract Method ✅
- [x] Add to `pkg/adt/refactoring.go`:
  - `ExtractMethodEvaluate(ctx, objectURI, startLine, startCol, endLine, endCol, source, methodName)`
  - `ExtractMethodPreview(ctx, objectURI, startLine, startCol, endLine, endCol, source, methodName)`
  - `ExtractMethodExecute(ctx, objectURI, startLine, startCol, endLine, endCol, source, methodName)`
- [x] Register MCP tool: `ExtractMethod` (single tool, `step` parameter)
- [x] Unit tests (5 tests)
- [ ] Integration test: extract code block into method (requires SAP system)

### Step 1.3: Quick Fix Proposals ✅
- [x] Create `pkg/adt/quickfix.go`:
  - `GetQuickFixProposals(ctx, objectURI, line, col, source)` → []QuickFixProposal
  - `ApplyQuickFix(ctx, objectURI, proposalID, line, col, source)` → newSource
- [x] Add types: `QuickFixProposal`, `QuickFixProposalsResult`, `QuickFixApplyResult`
- [x] Handlers in `internal/mcp/handlers_refactoring.go` (combined file)
- [x] Register MCP tools: `GetQuickFixProposals`, `ApplyQuickFix`
- [x] Unit tests (8 tests)
- [ ] Integration test: get proposals for syntax error, apply fix (requires SAP system)

### Step 1.4: ATC Quick Fix ✅
- [x] Add to `pkg/adt/quickfix.go`:
  - `GetATCQuickFixDetails(ctx, findingID)` → QuickFixDetails
  - `ApplyATCQuickFix(ctx, findingID)` → result
- [x] Register MCP tool: `ApplyATCQuickFix` (step: details/apply)
- [x] Unit tests (6 tests)
- [ ] Integration test (requires SAP system)

**Phase 1 total:** 5 new tools, 33 new unit tests, all passing

---

## Phase 2: Testing & Quality (3 tools)

**Why second:** Coverage analysis and SQL explain complete the testing and optimization story.

### Step 2.1: Code Coverage
- [ ] Modify `RunUnitTests()` in `pkg/adt/devtools.go`:
  - Add `requestCoverage` parameter
  - Extend `UnitTestResult` with `Coverage map[string]*SourceCoverage`
- [ ] Add XML types: `SourceCoverage`, `CoveredLine`
- [ ] Update `RunUnitTests` MCP handler: add optional `coverage` boolean parameter
- [ ] Register new MCP tool: `GetCodeCoverage` (runs tests + returns coverage summary)
- [ ] Unit tests with mock coverage XML
- [ ] Integration test: run tests with coverage on test class

**Checkpoint:** `RunUnitTests` with `coverage=true` returns line-level coverage data

### Step 2.2: SQL Explain Plan
- [ ] Add to `pkg/adt/client.go`:
  - `GetSQLExplainPlan(ctx, sqlQuery)` → *SQLExplainPlan
- [ ] Add types: `SQLExplainPlan`, `PlanNode` (operator, table, cost, rows, index, children)
- [ ] Register MCP tool: `GetSQLExplainPlan`
- [ ] Feature detection: probe endpoint in `features.go` (HANA-only)
- [ ] Unit tests
- [ ] Integration test on HANA system

**Checkpoint:** `GetSQLExplainPlan` returns execution plan for a SELECT query

### Step 2.3: Enhanced Check Run Results
- [ ] Add to `pkg/adt/devtools.go`:
  - `GetCheckRunResults(ctx, checkRunId)` → detailed findings
- [ ] Register MCP tool: `GetCheckRunResults`
- [ ] Unit tests

**Checkpoint:** SyntaxCheck → GetCheckRunResults gives detailed error list

**Phase 2 total:** 3 new tools, effort ~12

---

## Phase 3: CDS/RAP Completeness (4 tools)

**Why third:** Completes the RAP development lifecycle for S/4HANA projects.

### Step 3.1: Metadata Extension (MDE) Read/Write
- [ ] Add to `pkg/adt/client.go`:
  - `GetMDE(ctx, name)` → source string
- [ ] Add object type to `objectTypes` map in `crud.go`:
  - `ObjectTypeMDE` → path, create XML root/namespace
- [ ] Extend `GetSource()` unified handler with MDE type
- [ ] Extend `WriteSource()` with MDE support
- [ ] Register as part of existing GetSource/WriteSource (type "MDE")
- [ ] Unit tests
- [ ] Integration test on S/4HANA

**Checkpoint:** GetSource + WriteSource work for MDE objects

### Step 3.2: Access Control (DCL) Read/Write
- [ ] Same pattern as MDE:
  - Add `ObjectTypeDCLS` to objectTypes
  - Extend GetSource/WriteSource handlers
- [ ] Unit tests

**Checkpoint:** GetSource + WriteSource work for DCL objects

### Step 3.3: CDS Impact Analysis
- [ ] Add to `pkg/adt/cds.go`:
  - `GetCDSImpactAnalysis(ctx, cdsViewName)` → []ImpactedObject
- [ ] Add types: `ImpactedObject` (name, type, severity, reason)
- [ ] Register MCP tool: `GetCDSImpactAnalysis`
- [ ] Unit tests

**Checkpoint:** GetCDSImpactAnalysis returns downstream consumers of a CDS view

### Step 3.4: CDS Element Info
- [ ] Add to `pkg/adt/cds.go`:
  - `GetCDSElementInfo(ctx, cdsViewName, elementPath)` → *ElementInfo
- [ ] Add types: `ElementInfo` (name, type, annotations, semantics)
- [ ] Register MCP tool: `GetCDSElementInfo`
- [ ] Unit tests

**Checkpoint:** GetCDSElementInfo returns metadata for a CDS view field

**Phase 3 total:** 4 new tools, effort ~13

---

## Phase 4: DDIC & Miscellaneous (3+ tools)

### Step 4.1: DDIC Object Type Reads
- [x] `GetView(name)` — already in `client.go:617` ✅
- [ ] Add 3 remaining client methods (follow GetTable/GetView pattern):
  - `GetSearchHelp(ctx, name)` → `/sap/bc/adt/ddic/searchhelps/{name}`
  - `GetLockObject(ctx, name)` → `/sap/bc/adt/ddic/lockobjects/{name}`
  - `GetTypeGroup(ctx, name)` → `/sap/bc/adt/ddic/typegroups/{name}`
- [ ] Add XML types for each
- [ ] Consider extending `GetSource()` unified handler instead of separate tools
- [ ] Unit tests

### Step 4.2: Transport Object Assignment
- [ ] Add to `pkg/adt/transport.go`:
  - `AddObjectToTransport(ctx, transportNumber, pgmid, object, objectName)` → error
- [ ] Register MCP tool: `AddObjectToTransport`
- [ ] **Needs validation:** exact `_action` parameter on real system
- [ ] Unit + integration tests

### Step 4.3: Activity Feeds (if validated)
- [ ] Validate endpoints exist via ADT discovery on real system
- [ ] If confirmed: reuse Atom feed parser from `revisions.go`
- [ ] Register tools: `GetObjectChangeFeed`, `GetUserActivityFeed`

**Phase 4 total:** 3+ new tools, effort ~8+

---

## Deprioritized (not planned)

| Feature | Reason |
|---------|--------|
| abapGit REST endpoints | WebSocket via ZADT_VSP is more reliable |
| Debug variable modification | Not reliably exposed in ADT API |
| SQL Trace individual details | >90% covered by existing ListSQLTraces + GetTrace |
| CDS Annotation Value Help | Niche use case, low demand |

---

## Validation Plan

Before implementing each phase, validate unconfirmed endpoints:

1. **ADT Discovery:** Run `GET /sap/bc/adt/discovery` and check for endpoint registration
2. **Eclipse network trace:** Use Fiddler proxy on Eclipse ADT to capture real request/response
3. **ABAP handler debugging:** Set breakpoint on `SADT_REST` handler classes

### Systems for testing

| System | Features | Phase |
|--------|----------|-------|
| SC3 (non-HANA) | Transport, basic CRUD, refactoring | Phase 1, 4 |
| S23 (HANA) | SQL Explain, SourceSearch, CDS, RAP, MDE, DCL | Phase 2, 3 |

---

## Dependencies & Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Refactoring endpoints may differ across SAP versions | Phase 1 blocked | Test on both SC3 and S23; implement version detection |
| MDE/DCL may need S/4HANA | Phase 3 limited | Only run CDS tests on S23 |
| Coverage response format unconfirmed | Phase 2 coverage tool | Implement without coverage first, add later |
| Activity feeds may not exist | Phase 4 incomplete | Skip if not found in ADT discovery |
| Transport assignment exact API unknown | Phase 4 risk | Validate via Eclipse trace before implementing |

---

## Success Metrics

| Metric | Current | After Phase 1 | After Phase 4 |
|--------|---------|---------------|---------------|
| Total tools | 103 | 108 | ~118 |
| ADT coverage | 73% | 78% | ~84% |
| Refactoring tools | 1 (composite) | 5 (ADT-native) | 5 |
| CDS/RAP tools | 6 | 6 | 10 |
| Testing tools | 2 | 5 | 5 |
