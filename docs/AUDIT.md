# Intelligence Layer — Audit Results

**Date:** 2026-02-22
**Auditors:** Architect Agent + Lead Auditor Agent (independent parallel review)
**Subject:** Plan for 4 Intelligence Layer tools (AnalyzeSQLPerformance, GetImpactAnalysis, AnalyzeABAPCode, CheckRegression)

---

## Summary

| Level | Verdict |
|-------|---------|
| Architect Agent | APPROVE with 7 findings |
| Lead Auditor (Backend Specialist) | APPROVE with 6 findings |
| Chief Architect Cross-Review | APPROVE after all incorporated |

**Total findings:** 1 CRITICAL, 6 HIGH, 7 MEDIUM — all resolved in plan revision.

---

## Findings Detail

### CRITICAL

**C1: Line-by-line scanner doesn't work for ABAP**
- **Source:** Architect Agent
- **Issue:** ABAP statements span multiple lines ending with `.` (period). A line scanner would miss multi-line SELECT, CALL FUNCTION, FOR ALL ENTRIES, etc.
- **Resolution:** Redesigned to two-pass approach: Statement Assembler (join lines to periods) + Rule Engine operating on assembled statements with line tracking.

### HIGH

**H1: AnalyzeABAPCode scope creep**
- **Source:** Lead Auditor
- **Issue:** Original AnalyzeABAPCode was doing 4 things: static analysis, regression detection, ATC integration, and output merging. Violates single responsibility.
- **Resolution:** Split into AnalyzeABAPCode (static analysis) + CheckRegression (diff-based). Dropped ATC integration — AI can compose RunATCCheck + AnalyzeABAPCode itself.

**H2: Layer 3 dynamic call search produces noise**
- **Source:** Architect Agent
- **Issue:** Searching for generic patterns like `CALL METHOD (` produces too many false positives.
- **Resolution:** Search for target object NAME as a string literal in scope packages. This finds code referencing the name in a string context — prerequisite for any dynamic call.

**H3: Missing checkSafety OpType specification**
- **Source:** Lead Auditor
- **Issue:** Plan didn't specify which OpType each tool uses.
- **Resolution:** All 4 tools use `OpRead`. Layer 4b relies on RunQuery's built-in `checkSafety(OpFreeSQL)`.

**H4: Regression doesn't work for FM signatures**
- **Source:** Architect Agent
- **Issue:** Function module parameters are stored in XML metadata, not in source code. Source-level diff can't detect FM signature changes.
- **Resolution:** Documented as known limitation in tool description and PLAN.md.

**H5: False positive rates**
- **Source:** Lead Auditor
- **Issue:** `select_star` and `missing_authority_check` have high false positive rates (test classes, CDS consumption, utility methods).
- **Resolution:** Both demoted to `info` severity. `hardcoded_credentials` tightened to assignment context only.

**H6: Missing rules from ABAP best practices**
- **Source:** Architect Agent
- **Issue:** `commit_in_loop` (critical — destroys transactional integrity) and `catch_cx_root` (catches everything) were missing.
- **Resolution:** Added to rule catalog: `commit_in_loop` (critical), `catch_cx_root` (medium), `read_table_no_binary` (medium), `commit_work_and_wait` (medium).

### MEDIUM

**M1:** "Latest 2 revisions" semantically wrong for CheckRegression → Added `base_version` parameter with smart default
**M2:** No handling for 0/1 revisions → Graceful degradation with warnings
**M3:** No HANA check for AnalyzeSQLPerformance → Feature detection + SQL text fallback
**M4:** No timeout for GetImpactAnalysis → `context.WithTimeout` default 30s
**M5:** Handler naming convention → Split into domain-specific handler files
**M6:** Missing tests for Layer 4b edge cases → Added 6 additional tests
**M7:** `todo_fixme` rule always active → Only when `include_info=true`

---

## Round 3: Final Pre-Implementation Audit (2026-02-22)

**Auditors:** PAL ThinkDeep (GPT-5.2-pro) + Lead Auditor Agent (Claude Opus) + context7 (SAP ABAP docs)

### Findings

| # | Source | Severity | Finding | Resolution |
|---|--------|----------|---------|------------|
| H1 | Auditor | **HIGH** | `c.GetFeatures()` doesn't exist on `adt.Client` — FeatureProber on Server | Changed to `hanaAvailable bool` param, handler bridges |
| M1 | Auditor | MEDIUM | `FindReferences` returns `[]UsageReference`, not `[]ImpactedObject` | Added type mapping documentation |
| M2 | Auditor | MEDIUM | `GrepPackages` in `workflows.go:1820`, not `client.go` | Fixed reference table |
| M3 | Auditor+context7 | MEDIUM | String templates `\|...\|` contain periods but don't span lines | Added assembler detail + test case |
| L1 | Auditor | LOW | Tools must be added to `focusedTools` map | Added to Phase 5 |
| L2 | Auditor | LOW | Minor line number inaccuracies | Fixed (`testing.go:201`) |
| P1 | PAL | MEDIUM | Call graph visited set for cycles + stable sort | Added to Layer 2 description |
| P2 | PAL | LOW | Input size limit for assembler | Added 500KB limit |

**context7 verifications:**
- FOR ALL ENTRIES on empty table: CONFIRMED reads all rows (SAP doc `ABENWHERE_ALL_ENTRIES`)
- String templates: CONFIRMED single-line only, may contain periods
- `\|` escaping inside templates: CONFIRMED

**Verdict: APPROVE** — all findings resolved in plan, no architectural changes needed.

---

## Pre-Existing Issues Found During Audit (out of Intelligence Layer scope)

PAL codereview (GPT-5.2-pro) identified issues in existing code while reviewing the plan's dependencies. These are NOT blockers for the Intelligence Layer but should be tracked:

| Severity | File | Issue |
|----------|------|-------|
| HIGH | `workflows.go:1588` | `GrepObject` missing `checkSafety(OpRead)` |
| HIGH | `workflows.go:1014` | `SaveToFile` missing `checkSafety(OpRead)` |
| HIGH | `testing.go:46` | `GetCodeCoverage` missing `checkSafety(OpTest)` |
| HIGH | `testing.go:201` | `GetSQLExplainPlan` uses `OpRead` but executes arbitrary SQL |
| HIGH | `workflows.go:531` | `CreateFromFile` FUNC URL building without parent |
| HIGH | `workflows.go:964` | `RenameObject` writes to object URL instead of source URL |
| MEDIUM | `testing.go:121` | `parseCoverageResult` swallows XML parse errors |
| MEDIUM | `features.go:149` | `FeatureProber` cache returns mutable pointer |
| LOW | `codeintel.go:77` | `strconv.Atoi` errors ignored in `parseDefinitionLocation` |

**Action:** Track in separate backlog. Do NOT mix with Intelligence Layer implementation.

---

## Research Sources

- SAP ABAP Keyword Documentation (`ABENABAP_SQL_PERFO`, `ABENWHERE_ALL_ENTRIES`)
- SAP ABAP Cleaner (github.com/sap/abap-cleaner) — 100+ cleanup rules
- SonarSource ABAP rules — performance, security, code smell categories
- SAP Clean ABAP styleguide — modern syntax recommendations
- SAP CVA (Code Vulnerability Analyzer) — security patterns

---

## Round 4: Post-Implementation Audit (2026-02-22)

**Auditors:** Code Reviewer Agent (Claude Opus) + Architect Agent (Claude Opus) + PAL codereview (GPT-5.1-codex)

### Implementation Review Findings

Two independent agents (code-reviewer + architect) audited the implemented code. All findings resolved.

| # | Source | Severity | Finding | Resolution |
|---|--------|----------|---------|------------|
| H1 | Architect | **HIGH** | `scope_packages` missing from MCP tool schema | Added `mcp.WithArray("scope_packages")` to server.go |
| H2 | Architect | **HIGH** | 500KB input size limit not implemented | Added `maxSourceBytes` check + `context.WithTimeout(30s)` |
| H3 | Architect | **HIGH** | String templates `\|...\|` not handled in assembler | Rewrote `stripInlineComment` as `stringState` FSM |
| H4 | Architect | **HIGH** | Backtick strings not handled | Added `ssBacktick` state to FSM |
| H5 | Architect | **HIGH** | Slice mutation risk in sqlperf.go | Fixed with `make` + separate appends |
| M1 | Architect | MEDIUM | `todo_fixme` rule was dead code | Moved `checkTodoFixme` before `continue` on IsComment |
| M2 | Both | MEDIUM | Regex compiled in hot path (26+ patterns) | Promoted all to package-level `var` |
| M3 | Architect | MEDIUM | `extractMethodDefs` single-line regex | Acceptable — regression analysis is not hot path |
| M5 | Architect | MEDIUM | No timeout for AnalyzeABAPCode | Added `context.WithTimeout(30s)` |
| M6 | Architect | MEDIUM | `"safe"` risk level on degraded paths | Changed to `"unknown"` on all early returns |
| M8 | Architect | MEDIUM | Unused `parentRows` in walk closure | Removed parameter |
| L1 | Reviewer | LOW | `json.MarshalIndent` errors dropped | Added error handling in all 4 handlers |
| L2 | Reviewer | LOW | `ObjectName` not populated | Added `parseObjectURIComponents` call |

### PAL Cross-Validation (GPT-5.1-codex)

PAL reviewed the fixes and found 3 additional MEDIUM issues:

| # | Severity | Finding | Resolution |
|---|----------|---------|------------|
| P1 | MEDIUM | CheckRegression missing timeout (unlike other tools) | Added `context.WithTimeout(30s)` |
| P2 | MEDIUM | Early returns in CheckRegression leave `riskLevel` empty | Added `riskLevel = "unknown"` on all 4 early return paths |
| P3 | MEDIUM | `searchDynamicPatterns` swallows GrepPackages error | Changed to `([]DynamicCallRisk, error)`, caller adds warning |

**Verdict: APPROVE** — all findings resolved. 7 new tests added. 343 total tests passing.

### Test Coverage Added for Fixes

| Test | Validates |
|------|-----------|
| `TestAssembleStatements_StringTemplate` | `\|...\|` periods don't split statements |
| `TestAssembleStatements_BacktickLiteral` | Backtick periods don't split statements |
| `TestAnalyzeABAPSource_DynamicCallFunction` | `CALL FUNCTION (var)` detection |
| `TestAnalyzeABAPSource_TodoFixme` | M1 fix: comments trigger todo_fixme |
| `TestDetectExceptionChanges_MultiLineRaising` | Multi-line RAISING clause detection |
| `TestParseObjectURIComponents` | URI → type/name parsing (6 subtests) |
| `TestSanitizeForSQL` | SQL injection prevention (5 subtests) |
