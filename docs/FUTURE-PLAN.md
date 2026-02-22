# VSP MCP Server — Future Development Plan

**Created:** 2026-02-21
**Status:** After completing all 4 phases of ADT Gap Analysis

---

## Current State

| Metric | Value |
|--------|-------|
| Total tools | 145 (103 focused, 145 expert) |
| Unit tests | 343 |
| Integration tests | 71 (56 existing + 15 new) |
| ADT API coverage | ~87% |
| Phases completed | Refactoring, Testing, CDS/RAP, DDIC, Intelligence Layer |

### What was added (2026-02-21)

14 new tools across 4 phases:
- **Phase 1:** RenameRefactoring, ExtractMethod, GetQuickFixProposals, ApplyQuickFix, ApplyATCQuickFix
- **Phase 2:** GetCodeCoverage, GetSQLExplainPlan, GetCheckRunResults
- **Phase 3:** GetCDSImpactAnalysis, GetCDSElementInfo + DDLX/DCLS in GetSource/WriteSource
- **Phase 4:** GetSearchHelp, GetLockObject, GetTypeGroup, AddObjectToTransport

---

## Priority 1: Intelligence Layer (High ROI) — ✅ COMPLETE

**Detailed plan:** [docs/PLAN.md](PLAN.md)
**Audit results:** [docs/AUDIT.md](AUDIT.md)

### 1.1 AI Code Review Tools (4 tools, 39 tests) ✅

| Tool | Description | Status |
|------|-------------|--------|
| `AnalyzeSQLPerformance` | SQL explain plan analysis + ABAP SQL text fallback for non-HANA | ✅ Done |
| `GetImpactAnalysis` | 4-layer blast radius: static refs → transitive → dynamic → config | ✅ Done |
| `AnalyzeABAPCode` | 21-rule ABAP source analysis (two-pass statement assembler) | ✅ Done |
| `CheckRegression` | Diff-based breaking change detection (signatures, removed methods) | ✅ Done |

**New files (2026-02-22):**
- `pkg/adt/sqlperf.go` + `sqlperf_test.go` (10 tests)
- `pkg/adt/impact.go` + `impact_test.go` (7 tests)
- `pkg/adt/codeanalysis.go` + `codeanalysis_test.go` (14 tests)
- `pkg/adt/regression.go` + `regression_test.go` (8 tests)
- `internal/mcp/handlers_intelligence.go`, `handlers_codeanalysis.go`

**Key design decisions:**
- Two-pass statement assembler (ABAP statements span multiple lines)
- Dynamic calls: search for object NAME as string literal (not generic patterns)
- Config-driven calls: source analysis + optional RunQuery on SXS_INTER, MODSAP, TNAPR
- Regression > new code quality (transport release focus)
- ABAP SQL-aware text analysis (strips @host vars, INTO TABLE, FOR ALL ENTRIES, UP TO n ROWS)

### 1.2 Remaining Intelligence Ideas (deferred)

| Tool | Description | Complexity |
|------|-------------|------------|
| `FindDeadCode` | Unreferenced methods/classes by package | Medium |
| `DetectCyclicDependencies` | Find circular references between objects | Low |
| `FindDocumentationGaps` | Undocumented complex methods | Low |
| `AnalyzeTechnicalDebt` | Prioritized list: age × change freq × coverage × complexity | Medium |
| `FindCachingOpportunities` | SELECT in loop? Suggest buffering | Medium |

---

## Priority 2: Workflow Enhancement

### 2.1 DSL Improvements

Current YAML workflow engine (`pkg/dsl/workflow.go`) lacks:
- **Conditional logic:** `if: "{{ .atc_findings.critical == 0 }}"`
- **Parallel execution:** Independent tests in parallel
- **Error recovery:** Retry with backoff, compensating transactions
- **Audit trail:** Persistent workflow history

### 2.2 Pre-Built Workflows

Templates for common multi-step operations:
1. **Feature Development:** Create transport → class + tests → ATC → deploy
2. **Bug Fix:** Analyze dump → identify root cause → fix → regression test → deploy
3. **Refactoring:** Impact analysis → backup → extract/rename → verify tests → deploy
4. **Code Quality Sprint:** Find tech debt → fix violations → add tests → deploy

**Files:** `docs/workflows/*.yaml`

---

## Priority 3: CI/CD Integration

### 3.1 GitHub/GitLab Actions

```yaml
# Example GitHub Action
- uses: oisee/vsp-action@v1
  with:
    sap_url: ${{ secrets.SAP_URL }}
    workflow: deploy-and-test.yaml
    package: $ZPROJECT
```

### 3.2 Status Reporters
- ATC findings as PR comments
- Test results as status checks
- Code coverage as badges

### 3.3 Multi-System Orchestration
- Transport route: DEV → QAS → PRD
- Approval gates between systems
- Automated promotion workflows

---

## Priority 4: AI Enhancement

### 4.1 Autonomous RCA
- Combine: runtime trace + dump + static analysis
- AI hypothesis → verification → fix suggestion
- Confidence scoring (high/medium/low)

### 4.2 Coverage-Driven Test Generation
- Run GetCodeCoverage → identify uncovered branches
- AI generates test cases for red lines
- Target: 80%+ coverage per package

### 4.3 AI-Guided Refactoring
- Complexity Reduction Advisor: suggest Extract Method for high-cyclomatic methods
- Legacy Modernization: FM → class method, dialog → OData/RAP

---

## Remaining ADT Gaps (~8%)

Low priority items not yet implemented:

| Feature | Reason to defer |
|---------|----------------|
| Debug variable modification | ADT API unreliable for this |
| SQL Trace individual details | 90%+ covered by ListSQLTraces + GetTrace |
| CDS Annotation Value Help | Niche, low demand |
| Activity Feeds | Endpoint unconfirmed |
| abapGit REST (non-WebSocket) | WebSocket via ZADT_VSP more reliable |

---

## Timeline Estimate

| Quarter | Focus | New Tools |
|---------|-------|-----------|
| Q1 2026 | Intelligence Layer (1.1) ✅ + Remaining (1.2) | 4 done + 5 deferred |
| Q2 2026 | Workflow Enhancement (2.1-2.2) | 2-3 + templates |
| Q3 2026 | CI/CD Integration (3.1-3.3) | 3-4 |
| Q4 2026 | AI Enhancement (4.1-4.3) | 3-5 |

**Current:** 145 tools, ~87% ADT coverage
**Projected total:** 155+ tools, 92%+ ADT coverage

---

## Integration Tests Backlog — ✅ WRITTEN (2026-02-22)

15 new integration tests written in `pkg/adt/integration_test.go` covering all 18 new tools:

| Phase | Tests | System required | Status |
|-------|-------|-----------------|--------|
| ADT Gaps Phase 1 | TestIntegration_RenameEvaluate, _GetQuickFixProposals | SC3 or S23 | Written |
| ADT Gaps Phase 2 | TestIntegration_GetCodeCoverage, _GetSQLExplainPlan, _GetCheckRunResults | S23 (HANA) | Written |
| ADT Gaps Phase 3 | TestIntegration_GetCDSImpactAnalysis, _GetCDSElementInfo | S23 (HANA) | Written |
| ADT Gaps Phase 4 | TestIntegration_GetSearchHelp, _GetLockObject, _GetTypeGroup, _AddObjectToTransport | SC3 or S23 | Written |
| Intelligence Phase 1 | TestIntegration_AnalyzeSQLPerformance | S23 (HANA) | Written |
| Intelligence Phase 2 | TestIntegration_GetImpactAnalysis | S23 (HANA + SourceSearch) | Written |
| Intelligence Phase 3 | TestIntegration_AnalyzeABAPCode | SC3 or S23 | Written |
| Intelligence Phase 4 | TestIntegration_CheckRegression | SC3 or S23 | Written |

All tests handle graceful degradation (skip on 404, try multiple objects, handle non-HANA systems).

---

## ALV Capture for RunReport — Plan

### Current State
- **ABAP side** (`zcl_vsp_report_service`): ALV capture is FULLY IMPLEMENTED
  - `capture_alv=true` → `CL_SALV_BS_RUNTIME_INFO` → intercepts ALV display
  - Returns JSON: `{status, report, runtime_ms, alv_captured, columns, rows, total_rows, truncated}`
  - `max_rows` parameter (default 1000) prevents memory issues
- **Go side** (`pkg/adt/reports.go`): Does NOT use ALV capture
  - Current flow: `runReport` → job-based background execution → poll job → read spool
  - `RunReportParams` has no `CaptureALV` field
  - `RunReportResult` has no ALV data fields

### Problem
The Go handler (`handleRunReport`) sends `runReport` to WebSocket which triggers the ABAP `handle_run_report` method. The ABAP method already supports `capture_alv` and returns ALV data synchronously. But Go:
1. Does not pass `capture_alv` parameter
2. Expects job-based response (jobname/jobcount), not ALV data
3. Has no types for ALV columns/rows

### Implementation Plan

**Step 1: Extend Go types** (`pkg/adt/reports.go`)
```go
type RunReportParams struct {
    Report     string            `json:"report"`
    Variant    string            `json:"variant,omitempty"`
    Params     map[string]string `json:"params,omitempty"`
    CaptureALV bool              `json:"capture_alv,omitempty"` // NEW
    MaxRows    int               `json:"max_rows,omitempty"`    // NEW (default: 1000)
}

type RunReportResult struct {
    Status      string          `json:"status"`
    Report      string          `json:"report"`
    JobName     string          `json:"jobname,omitempty"`
    JobCount    string          `json:"jobcount,omitempty"`
    RuntimeMs   int             `json:"runtime_ms,omitempty"`   // NEW
    ALVCaptured bool            `json:"alv_captured,omitempty"` // NEW
    Columns     []ALVColumn     `json:"columns,omitempty"`      // NEW
    Rows        []map[string]string `json:"rows,omitempty"`     // NEW
    TotalRows   int             `json:"total_rows,omitempty"`   // NEW
    Truncated   bool            `json:"truncated,omitempty"`    // NEW
}

type ALVColumn struct {
    Name string `json:"name"`
    Type string `json:"type"`
}
```

**Step 2: Update RunReport WebSocket call** (`pkg/adt/reports.go`)
- Pass `capture_alv` and `max_rows` params to WebSocket message
- Parse extended response (ALV data OR job data)

**Step 3: Update handler** (`internal/mcp/handlers_report.go`)
- Add `capture_alv` (boolean) and `max_rows` (number) parameters to tool registration
- If `capture_alv=true` AND response has ALV data → format as table output
- If `capture_alv=false` OR no ALV → fall through to existing job/spool flow

**Step 4: Update tool description** (`internal/mcp/server.go`)
- Add `capture_alv` and `max_rows` parameters to RunReport tool definition
- Update description: "...optionally captures ALV grid output as structured data"

**Estimated effort:** ~100 LOC changes, 3 files, 2-3 new unit tests
**Risk:** Low — additive change, backward compatible (existing flow unchanged when `capture_alv=false`)

---

## Open Items

- [x] ~~Fix `TestIntegration_SourceSearch`~~ (was false alarm — compiles fine)
- [x] ~~Integration tests for all 18 new tools~~ (15 tests written, 2026-02-22)
- [ ] Re-add ALV capture for RunReport (plan above)
- [ ] Test SAP GUI breakpoint sharing (set via vsp, trigger in SAP GUI)
- [ ] Run integration tests on live SAP S23/SC3 systems
