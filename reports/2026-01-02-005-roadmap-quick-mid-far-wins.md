# VSP Roadmap: Quick/Mid/Far Wins

**Date:** 2026-01-02
**Report ID:** 005
**Subject:** Prioritized feature roadmap for vsp development
**Status:** Living document - updated as features complete

---

## Quick Wins (1-4 hours each)

| Rank | Feature | Effort | Impact | Status | Notes |
|------|---------|--------|--------|--------|-------|
| 1 | `/debug` skill | 2h | High | Planned | Orchestrate existing debug tools with AI guidance |
| 2 | RunReport via FM | 2h | Medium | Planned | Create ZADT_RUN_REPORT FM to avoid APC blocking |
| 3 | GetMessages tool | 1h | Medium | Planned | Read message class texts (SE91/MSAG) |
| 4 | CompareSource tool | 2h | Medium | Planned | Diff two versions/objects |
| 5 | CloneObject tool | 2h | Medium | Planned | Copy object to new name (useful for experiments) |
| 6 | Heading texts in SetTextElements | 1h | Low | Planned | Add list/column headers support |
| 7 | Tool aliases | 1h | Low | Planned | Short names: `gs` → GetSource, `ws` → WriteSource |
| 8 | GetClassInfo tool | 1h | Low | Planned | Quick class metadata without full source |

## Mid Wins (1-3 days each)

| Rank | Feature | Effort | Impact | Status | Notes |
|------|---------|--------|--------|--------|-------|
| 1 | ATC Integration | 2d | High | Planned | Code quality checks (RunATCCheck exists, add findings navigation) |
| 2 | Basic DAP Adapter | 3d | High | Planned | Attach mode for VS Code ([Report 004](./2026-01-02-004-dap-abap-debugging-vision.md)) |
| 3 | Transport Workflow | 2d | High | Planned | Create→AddObjects→Release flow |
| 4 | Code Coverage | 2d | Medium | Planned | Unit test coverage reporting |
| 5 | Conditional Breakpoints | 1d | Medium | Planned | Extend SetBreakpoint with conditions |
| 6 | RefactorRename tool | 2d | Medium | Planned | Rename symbol across references |
| 7 | abapGit Import (pull) | 3d | Medium | Planned | Deploy from ZIP/GitHub URL |
| 8 | Batch RunUnitTests | 1d | Medium | Planned | Parallel test execution with aggregated results |
| 9 | Watch Expressions | 1d | Low | Planned | Evaluate expressions during debug |
| 10 | Documentation Generator | 2d | Low | Planned | Generate markdown from ABAP Doc comments |

## Far Wins (1-2 weeks each)

| Rank | Feature | Effort | Impact | Status | Notes |
|------|---------|--------|--------|--------|-------|
| 1 | Graph Traversal Engine | 1w | High | Designed | [Reports 005-007](./2025-12-02-005-improved-graph-architecture-design.md): call graph analysis |
| 2 | Test Intelligence | 1w | High | Designed | [Report 008](./2025-12-02-008-test-intelligence-plan.md): smart test selection |
| 3 | Full DAP + Shared State | 2w | High | Planned | Complete VS Code debugging experience |
| 4 | VS Code Extension | 2w | High | Planned | ABAP syntax, outline, diagnostics |
| 5 | Standard API Scraper | 1w | Medium | Designed | [Report 006](./2025-12-02-006-standard-api-surface-scraper.md): SAP standard usage patterns |
| 6 | Multi-System Support | 1w | Medium | Planned | DEV→QA→PROD transport chains |
| 7 | AI Code Review | 1w | Medium | Planned | Automated review with SAP best practices |
| 8 | Performance Profiler UI | 2w | Low | Planned | Visual trace analysis (ATRA data) |

---

## Recommended Priorities

### Immediate (Next Sprint)
1. **`/debug` skill** - Biggest bang for buck, uses existing tools
2. **RunReport via FM** - Unblocks report execution with ALV capture
3. **GetMessages** - Completes the "report authoring" story

### This Month
- **ATC integration** - Code quality is always high value
- **Basic DAP adapter** - Opens VS Code market
- **Transport workflow** - Enterprise customers need this

### This Quarter
- **Graph traversal** - Architectural analysis, impact assessment
- **Test intelligence** - Smart CI/CD integration

---

## Completed Features (v2.18.0)

For reference, recently completed:
- Report Execution: RunReport, GetVariants, GetTextElements, SetTextElements
- abapGit Export: GitTypes, GitExport (158 object types)
- Install Tools: InstallZADTVSP, InstallAbapGit, ListDependencies
- WebSocket Debugging: ZADT_VSP with 5 service domains
- AMDP Debugging: Goroutine+channel architecture

---

## Legend

| Status | Meaning |
|--------|---------|
| Planned | On roadmap, not started |
| Designed | Has design document/report |
| In Progress | Currently being implemented |
| Done | Completed and released |
