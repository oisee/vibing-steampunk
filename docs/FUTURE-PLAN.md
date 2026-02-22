# VSP MCP Server — Future Development Plan

**Created:** 2026-02-21
**Status:** After completing all 4 phases of ADT Gap Analysis

---

## Current State

| Metric | Value |
|--------|-------|
| Total tools | 145 (103 focused, 145 expert) |
| Unit tests | 336 |
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

## Integration Tests Backlog

All new tools need integration testing on real SAP systems:

| Phase | Tools to test | System required |
|-------|---------------|-----------------|
| ADT Gaps Phase 1 | RenameRefactoring, ExtractMethod, QuickFix | SC3 or S23 |
| ADT Gaps Phase 2 | GetCodeCoverage, GetSQLExplainPlan | S23 (HANA) |
| ADT Gaps Phase 3 | DDLX/DCLS GetSource/WriteSource, CDS tools | S23 (HANA) |
| ADT Gaps Phase 4 | DDIC reads, AddObjectToTransport | SC3 or S23 |
| Intelligence Phase 1 | AnalyzeSQLPerformance (HANA explain plan) | S23 (HANA) |
| Intelligence Phase 2 | GetImpactAnalysis (all 4 layers) | S23 (HANA + SourceSearch) |
| Intelligence Phase 3 | AnalyzeABAPCode (via object_uri) | SC3 or S23 |
| Intelligence Phase 4 | CheckRegression (version history) | SC3 or S23 |

---

## Open Items

- [ ] Re-add ALV capture for RunReport
- [ ] Test SAP GUI breakpoint sharing (set via vsp, trigger in SAP GUI)
- [ ] Fix `TestIntegration_SourceSearch` (SourceSearchResponse fields renamed)
- [ ] Integration tests for all 14 new tools
