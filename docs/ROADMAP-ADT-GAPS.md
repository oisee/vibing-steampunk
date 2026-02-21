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

## Phase 2: Testing & Quality (3 tools) ✅ COMPLETED

**Why second:** Coverage analysis and SQL explain complete the testing and optimization story.

**Implementation date:** 2026-02-21
**New files:** `pkg/adt/testing.go`, `internal/mcp/handlers_testing.go`, `pkg/adt/testing_test.go`
**New tests:** 13 (4 coverage + 5 SQL explain + 4 check run)
**Tools added:** GetCodeCoverage, GetSQLExplainPlan, GetCheckRunResults

### Step 2.1: Code Coverage ✅
- [x] Create `pkg/adt/testing.go` with `GetCodeCoverage()` method
  - Separate method (not modifying RunUnitTests) with `coverage active="true"` in XML body
  - Returns `CoverageResult` with statement/branch/procedure stats + per-source breakdown
- [x] Add types: `CoverageResult`, `CoverageStats`, `SourceCoverage`, `CoveredLine`
- [x] Register MCP tool: `GetCodeCoverage` with optional `include_dangerous`, `include_long` params
- [x] Unit tests: parse coverage XML, empty response, no coverage data, client-level round-trip
- [ ] Integration test: run tests with coverage on test class (requires SAP system)

### Step 2.2: SQL Explain Plan ✅
- [x] Add `GetSQLExplainPlan()` to `pkg/adt/testing.go`
  - Uses `/sap/bc/adt/datapreview/freestyle` with EXPLAIN PLAN prefix
  - Fallback to `/sap/bc/adt/datapreview/ddlServices/explain` if first fails
  - Returns tree of `SQLPlanNode` with operator, table, index, cost, rows, children
- [x] Register MCP tool: `GetSQLExplainPlan`
- [x] Unit tests: parse structured XML, empty, non-XML fallback, client round-trip, safety check
- [ ] Integration test on HANA system (requires SAP system)
- [ ] Feature detection: HANA-only (existing `FeatureHANA` probe can be used by caller)

### Step 2.3: Enhanced Check Run Results ✅
- [x] Add `GetCheckRunResults()` to `pkg/adt/testing.go`
  - GET `/sap/bc/adt/checkruns/{checkRunId}`
  - Returns messages with line/column/severity + summary counts
- [x] Register MCP tool: `GetCheckRunResults`
- [x] Unit tests: parse check run XML, empty, no messages, client round-trip

**Phase 2 total:** 3 new tools, 13 new unit tests, all passing

---

## Phase 3: CDS/RAP Completeness (4 tools) ✅ COMPLETED

**Why third:** Completes the RAP development lifecycle for S/4HANA projects.

**Implementation date:** 2026-02-21
**New files:** `pkg/adt/cds_tools.go`, `pkg/adt/cds_tools_test.go`, `internal/mcp/handlers_cds.go`
**Modified files:** `pkg/adt/client.go` (GetDDLX, GetDCLS), `pkg/adt/crud.go` (ObjectTypeDDLX, ObjectTypeDCLS), `pkg/adt/workflows.go` (GetSource/WriteSource extended), `internal/mcp/handlers_codeintel.go` (descriptions updated), `internal/mcp/server.go` (2 new tools registered)
**New tests:** 10 (5 impact analysis + 5 element info)
**Approach:** DDLX/DCLS integrated into existing GetSource/WriteSource unified tools (no separate MCP tools needed). Two new standalone MCP tools: GetCDSImpactAnalysis, GetCDSElementInfo.

### Step 3.1: Metadata Extension (DDLX) Read/Write ✅
- [x] Add `GetDDLX(ctx, name)` to `pkg/adt/client.go`
  - Endpoint: `/sap/bc/adt/ddic/ddlx/sources/{name}/source/main`
- [x] Add `ObjectTypeDDLX = "DDLX/EX"` to `objectTypes` map in `crud.go`
  - Creation path: `/sap/bc/adt/ddic/ddlx/sources`
  - Root: `ddlx:ddlxSource`, namespace: `http://www.sap.com/adt/ddic/ddlxsources`
- [x] Extend `GetSource()` with DDLX type
- [x] Extend `WriteSource()` with DDLX support (create + update)
- [x] Extend `GetObjectURL()` with DDLX
- [x] Update GetSource/WriteSource handler descriptions
- [ ] Integration test on S/4HANA (requires SAP system)

### Step 3.2: Access Control (DCLS) Read/Write ✅
- [x] Add `GetDCLS(ctx, name)` to `pkg/adt/client.go`
  - Endpoint: `/sap/bc/adt/acm/dcl/sources/{name}/source/main`
- [x] Add `ObjectTypeDCLS = "DCLS/DL"` to `objectTypes` map in `crud.go`
  - Creation path: `/sap/bc/adt/acm/dcl/sources`
  - Root: `dcl:dclSource`, namespace: `http://www.sap.com/adt/acm/dclsources`
- [x] Extend `GetSource()` with DCLS type
- [x] Extend `WriteSource()` with DCLS support (create + update)
- [x] Extend `GetObjectURL()` with DCLS
- [ ] Integration test on S/4HANA (requires SAP system)

### Step 3.3: CDS Impact Analysis ✅
- [x] Create `pkg/adt/cds_tools.go`:
  - `GetCDSImpactAnalysis(ctx, cdsViewName)` → *CDSImpactAnalysisResult
  - Uses existing usageReferences endpoint with CDS view URI
- [x] Add types: `CDSImpactedObject`, `CDSImpactAnalysisResult`
- [x] Register MCP tool: `GetCDSImpactAnalysis`
- [x] Unit tests: parse XML, empty response, invalid XML, client round-trip, read-only check (5 tests)
- [ ] Integration test (requires SAP system)

### Step 3.4: CDS Element Info ✅
- [x] Add to `pkg/adt/cds_tools.go`:
  - `GetCDSElementInfo(ctx, cdsViewName)` → *CDSElementInfoResult
  - Uses ADT DDL source metadata endpoint with `application/vnd.sap.adt.ddic.ddlsources.v2+xml`
- [x] Add types: `CDSElementInfo`, `CDSElementInfoResult`
- [x] Register MCP tool: `GetCDSElementInfo`
- [x] Unit tests: parse elements with annotations, empty, invalid XML, client round-trip, read-only check (5 tests)
- [ ] Integration test (requires SAP system)

**Phase 3 total:** 2 new MCP tools + 2 object types added to GetSource/WriteSource, 10 new unit tests

---

## Phase 4: DDIC & Miscellaneous (4 tools) ✅ COMPLETED

**Implementation date:** 2026-02-21
**New files:** `pkg/adt/ddic_test.go`
**Modified files:** `pkg/adt/client.go` (GetSearchHelp, GetLockObject, GetTypeGroup), `pkg/adt/transport.go` (AddObjectToTransport), `internal/mcp/handlers_read.go` (3 DDIC handlers), `internal/mcp/handlers_transport.go` (AddObjectToTransport handler), `internal/mcp/server.go` (4 tools registered)
**New tests:** 8 (4 DDIC reads + 4 transport)
**Approach:** DDIC tools as separate MCP tools (expert mode). AddObjectToTransport in transport group (C).

### Step 4.1: DDIC Object Type Reads ✅
- [x] `GetView(name)` — already in `client.go` ✅
- [x] `GetSearchHelp(ctx, name)` → `/sap/bc/adt/ddic/searchhelps/{name}/source/main`
- [x] `GetLockObject(ctx, name)` → `/sap/bc/adt/ddic/lockobjects/{name}/source/main`
- [x] `GetTypeGroup(ctx, name)` → `/sap/bc/adt/ddic/typegroups/{name}/source/main`
- [x] Registered as separate MCP tools (expert mode only, simple DDIC reads)
- [x] Unit tests: round-trip, uppercase conversion (4 tests)
- [ ] Integration test (requires SAP system)

### Step 4.2: Transport Object Assignment ✅
- [x] `AddObjectToTransport(ctx, transportNumber, pgmid, objectType, objectName)` in `transport.go`
  - Uses PUT `/sap/bc/adt/cts/transportrequests/{number}` with tm:root XML body
  - Defaults pgmid to "R3TR" if empty
  - Full safety checks via `CheckTransport()`
- [x] Register MCP tool: `AddObjectToTransport` (in "C" transport group)
- [x] Unit tests: round-trip, default pgmid, missing params, read-only check (4 tests)
- [ ] Integration test (requires SAP system)

### Step 4.3: Activity Feeds — SKIPPED
- Endpoints not validated on real system
- Low priority, can be added later if ADT discovery confirms existence

**Phase 4 total:** 4 new tools, 8 new unit tests, all passing

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
