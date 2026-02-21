# VSP MCP Server — Future Development Plan

**Created:** 2026-02-21
**Status:** After completing all 4 phases of ADT Gap Analysis

---

## Current State

| Metric | Value |
|--------|-------|
| Total tools | 138 (99 focused, 138 expert) |
| Unit tests | 302+ |
| ADT API coverage | ~84% |
| Phases completed | Refactoring, Testing, CDS/RAP, DDIC |

### What was added (2026-02-21)

14 new tools across 4 phases:
- **Phase 1:** RenameRefactoring, ExtractMethod, GetQuickFixProposals, ApplyQuickFix, ApplyATCQuickFix
- **Phase 2:** GetCodeCoverage, GetSQLExplainPlan, GetCheckRunResults
- **Phase 3:** GetCDSImpactAnalysis, GetCDSElementInfo + DDLX/DCLS in GetSource/WriteSource
- **Phase 4:** GetSearchHelp, GetLockObject, GetTypeGroup, AddObjectToTransport

---

## Priority 1: Intelligence Layer (High ROI)

### 1.1 Impact Analysis & Dead Code

**Why:** AI can't safely refactor without understanding blast radius.

| Tool | Description | Complexity |
|------|-------------|------------|
| `GetImpactAnalysis` | "If I change this, what breaks?" — combines CROSS/WBCROSSGT + test coverage | Medium |
| `FindDeadCode` | Unreferenced methods/classes by package | Medium |
| `DetectCyclicDependencies` | Find circular references between objects | Low |

**Files:** `pkg/adt/analysis.go`, `internal/mcp/handlers_analysis.go`
**Depends on:** Existing GetCallGraph, GetCalleesOf, GetCallersOf

### 1.2 Semantic Code Search

**Why:** Text search finds strings; semantic search finds intent.

| Tool | Description | Complexity |
|------|-------------|------------|
| `FindPatterns` | Detect code smells, repeated patterns | Medium |
| `FindDocumentationGaps` | Undocumented complex methods (complexity + comment ratio) | Low |
| `AnalyzeTechnicalDebt` | Prioritized list: age × change freq × coverage × complexity | Medium |

**Depends on:** Existing SourceSearch (HANA), GrepPackages

### 1.3 Performance Diagnostics

**Why:** ABAP developers constantly fight performance issues.

| Tool | Description | Complexity |
|------|-------------|------------|
| `AnalyzeSQLPerformance` | Extend GetSQLExplainPlan: detect full scans, missing indexes | Low |
| `FindCachingOpportunities` | SELECT in loop? Suggest buffering | Medium |

**Depends on:** Phase 2 GetSQLExplainPlan, existing GetTrace

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
| Q1 2026 | Intelligence Layer (1.1-1.3) | 5-7 |
| Q2 2026 | Workflow Enhancement (2.1-2.2) | 2-3 + templates |
| Q3 2026 | CI/CD Integration (3.1-3.3) | 3-4 |
| Q4 2026 | AI Enhancement (4.1-4.3) | 3-5 |

**Projected total:** 150+ tools, 92%+ ADT coverage

---

## Integration Tests Backlog

All new tools from the 4-phase implementation need integration testing on real SAP systems:

| Phase | Tools to test | System required |
|-------|---------------|-----------------|
| Phase 1 | RenameRefactoring, ExtractMethod, QuickFix | SC3 or S23 |
| Phase 2 | GetCodeCoverage, GetSQLExplainPlan | S23 (HANA) |
| Phase 3 | DDLX/DCLS GetSource/WriteSource, CDS tools | S23 (HANA) |
| Phase 4 | DDIC reads, AddObjectToTransport | SC3 or S23 |

---

## Open Items

- [ ] Re-add ALV capture for RunReport
- [ ] Test SAP GUI breakpoint sharing (set via vsp, trigger in SAP GUI)
- [ ] Fix `TestIntegration_SourceSearch` (SourceSearchResponse fields renamed)
- [ ] Integration tests for all 14 new tools
