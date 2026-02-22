# Plan: Intelligence Layer — 4 Tools for AI Code Review

**Created:** 2026-02-22
**Status:** ✅ COMPLETE — All 4 phases implemented + audit fixes + PAL review (46 tests, 7 new files)

---

## Context

After completing all 4 phases of ADT Gap Analysis (138 tools, 297 tests), we're adding the **Intelligence Layer** — tools that compose existing ADT capabilities into higher-level analysis.

**Goal:** Enable AI code review by answering four questions:
1. "If I change this object, what breaks?" → **GetImpactAnalysis**
2. "Is this SQL efficient?" → **AnalyzeSQLPerformance**
3. "What anti-patterns are in this ABAP code?" → **AnalyzeABAPCode**
4. "Did this change break existing behavior?" → **CheckRegression**

User requirements:
1. Account for **dynamic calls** (CALL METHOD (var), CALL FUNCTION var, PERFORM (var))
2. Account for **configuration-driven calls** (BAdI, enhancements, user exits, NACE, workflow, substitutions, determinations)
3. **Regression detection is the highest priority** — old code breaking matters more than new code quality

ADT REST API cannot directly enumerate most config-driven registrations. Strategy: combine static cross-references + source pattern analysis + optional customizing table queries via RunQuery.

## Audit Results (incorporated)

Two independent reviews (architect + lead-auditor) produced 1 CRITICAL, 6 HIGH, 7 MEDIUM findings. All addressed below:

| Finding | Severity | Resolution |
|---------|----------|------------|
| Line-by-line scanner doesn't work for ABAP | CRITICAL | **Two-pass: statement assembler + rule engine** |
| AnalyzeABAPCode scope creep (4 concerns in 1 tool) | HIGH | **Split into AnalyzeABAPCode + CheckRegression. Drop ATC integration** |
| Layer 3 dynamic call search is noise | HIGH | **Search for target object NAME as string literal, not generic patterns** |
| Missing checkSafety OpType specification | HIGH | **Explicit OpRead for all tools, Layer 4b relies on RunQuery's OpFreeSQL** |
| Regression doesn't work for FM signatures | HIGH | **Document as limitation — FM params are in metadata, not source** |
| False positive rates (select_star, missing_authority_check) | HIGH | **Demote to info, tighten hardcoded_credentials regex** |
| Missing rules (commit_in_loop, catch_cx_root) | MEDIUM | **Added to rule catalog** |
| "Latest 2 revisions" semantically wrong | MEDIUM | **Added base_version parameter, smarter default** |
| No handling for 0/1 revisions | MEDIUM | **Graceful degradation with warnings** |
| No HANA check for AnalyzeSQLPerformance | MEDIUM | **Feature detection + SQL text fallback** |
| No timeout for GetImpactAnalysis | MEDIUM | **context.WithTimeout default 30s** |
| Handler naming convention | MEDIUM | **Split into domain-specific handler files** |
| Missing tests for Layer 4b + regression edges | MEDIUM | **Added 6 additional tests** |

---

## Phase 0: Save Documentation ✅

- [x] Save plan to `docs/PLAN.md`
- [x] Save audit findings to `docs/AUDIT.md`
- [x] Update `docs/FUTURE-PLAN.md`
- [x] Update `MEMORY.md`

---

## Phase 1: AnalyzeSQLPerformance (simpler, pure Go analysis) ✅

**New files:**
- `pkg/adt/sqlperf.go` — types + analysis engine
- `pkg/adt/sqlperf_test.go` — 7 tests

**Modified files:**
- `internal/mcp/handlers_intelligence.go` (NEW — handlers for both tools)
- `internal/mcp/server.go` — register tool

### Step 1.1: Create `pkg/adt/sqlperf.go`

Types: `SQLPerformanceAnalysis`, `SQLPerformanceFinding`, `SQLPerfSummary`

**IMPORTANT: ABAP SQL vs Native SQL distinction**

ABAP SQL (Open SQL) is NOT standard SQL. Key differences:
- Host variables: `@lv_var`, `@lt_tab`
- `INTO TABLE @lt_tab`, `APPENDING TABLE`, `CORRESPONDING FIELDS OF`
- `FOR ALL ENTRIES IN @lt_tab WHERE field = @lt_tab-field`
- `UP TO n ROWS` instead of LIMIT
- `SELECT SINGLE` instead of TOP 1
- `CLIENT SPECIFIED` — cross-client access
- `WHERE field IN @lt_range` — range table conditions
- No semicolons, ends with `.` (ABAP statement terminator)

**Two input modes:**
1. **ABAP SQL** (from source code) — `AnalyzeSQLText` strips ABAP-specific syntax, then analyzes
2. **Native SQL** (for HANA explain plan) — passed directly to `GetSQLExplainPlan`

Functions:
- `AnalyzePlanNodes(nodes []SQLPlanNode) []SQLPerformanceFinding` — pure Go tree walk, exported for testing
- `AnalyzeSQLText(sqlQuery string) []SQLPerformanceFinding` — pure Go text analysis, handles **both ABAP SQL and native SQL**
- `stripABAPSQLSyntax(abapSQL string) string` — removes INTO/APPENDING clause, @host variables, FOR ALL ENTRIES clause, UP TO n ROWS; returns cleaned SQL for pattern analysis
- `calculateSQLScore(findings) string` — "good" / "warning" / "critical"
- `(c *Client) AnalyzeSQLPerformance(ctx, sqlQuery string, hanaAvailable bool) (*SQLPerformanceAnalysis, error)` — if `hanaAvailable`: attempts `GetSQLExplainPlan` with native SQL + `AnalyzePlanNodes`; always runs `AnalyzeSQLText` on the input; merges both results

**Safety:** `checkSafety(OpRead, "AnalyzeSQLPerformance")` at entry.

**HANA detection:** `FeatureProber` lives on `Server` struct (`internal/mcp/server.go`), NOT on `adt.Client`. The handler in `handlers_intelligence.go` calls `s.featureProber.IsAvailable(ctx, adt.FeatureHANA)` and passes the result as `hanaAvailable bool` to the Client method. This follows existing patterns (handlers bridge Server-level state to Client methods).

**Note on division of labor:**
- `AnalyzeSQLPerformance` — analyzes a **single SQL query** (execution plan + text patterns)
- `AnalyzeABAPCode` (Phase 3) — analyzes **full ABAP source** and catches SQL-in-context patterns (SELECT in LOOP, FAE without empty check, SELECT...ENDSELECT, etc.)
- The AI composes both: extract SQL from source, run AnalyzeSQLPerformance on interesting queries; run AnalyzeABAPCode on the full source for context-dependent patterns

**AnalyzeSQLText rules** (handles both ABAP SQL and native SQL):

| Type | Severity | Rule |
|------|----------|------|
| `select_star` | info | `SELECT *` or `SELECT SINGLE *` — prefer explicit field list |
| `missing_where` | high | SELECT without WHERE clause (after stripping INTO/FAE) |
| `client_specified` | medium | `CLIENT SPECIFIED` — cross-client data access |
| `nested_subquery` | medium | SELECT inside SELECT (subquery) |
| `no_up_to_rows` | info | Large table SELECT without `UP TO n ROWS` (ABAP SQL specific) |
| `distinct_usage` | info | `DISTINCT` — often indicates missing WHERE or design issue |

Finding rules (walk SQLPlanNode tree — HANA only):

| Type | Severity | Rule |
|------|----------|------|
| `full_table_scan` | critical | operator contains "TABLE SCAN", no index, rows > 1000 |
| `full_scan_small` | info | same but rows <= 1000 |
| `missing_index` | high | cost > 100, no index, has table |
| `nested_loop_large` | high | "NESTED LOOP" with child rows > 10000 |
| `high_cost_node` | medium | cost > 1000 |
| `cartesian_product` | critical | "CROSS JOIN" or nested loop without filter child |

### Step 1.2: Create `pkg/adt/sqlperf_test.go`

10 tests:

**Plan node analysis (pure Go):**
- [x] TestAnalyzePlanNodes_FullTableScan
- [x] TestAnalyzePlanNodes_SmallTableScan
- [x] TestAnalyzePlanNodes_NestedLoopLarge
- [x] TestAnalyzePlanNodes_GoodPlan
- [x] TestAnalyzePlanNodes_Empty

**ABAP SQL text analysis (pure Go):**
- [x] TestAnalyzeSQLText_ABAPSelectStar — `SELECT * FROM mara WHERE ...` → select_star finding
- [x] TestAnalyzeSQLText_ABAPMissingWhere — `SELECT matnr FROM mara INTO TABLE @lt_result.` → missing_where
- [x] TestStripABAPSQLSyntax — strips `@`, `INTO TABLE`, `UP TO n ROWS`, `APPENDING TABLE`, host expressions (6 sub-tests)

**Scoring + end-to-end:**
- [x] TestCalculateSQLScore (4 sub-tests)
- [x] TestClient_AnalyzeSQLPerformance (non-HANA + HANA paths, mock transport with pre-set CSRF token)

### Step 1.3: Create handler + register ✅

Handler in `internal/mcp/handlers_intelligence.go`, registered in `server.go` after GetCheckRunResults.
Added to `focusedTools` map in "Testing & Quality" section.

### Step 1.4: Run tests ✅
- All 10 Phase 1 tests pass
- `go test ./...` — no regressions (only pre-existing `pkg/cache` CGO issue)
- `go build ./cmd/vsp` — compiles successfully
- **Gotcha:** POST-based tests need `transport.csrfToken = "test-token"` pre-set because `newTestResponse` literal headers bypass canonicalization

---

## Phase 2: GetImpactAnalysis (multi-layer composition) ✅

**New files:**
- `pkg/adt/impact.go` — types + orchestration
- `pkg/adt/impact_test.go` — 8+ tests

**Modified files:**
- `internal/mcp/handlers_intelligence.go` — add handler
- `internal/mcp/server.go` — register tool

### Architecture: 4 Layers

```
Layer 1: Static References (FindReferences)     ← always on, fast
Layer 2: Transitive Callers (GetCallersOf)       ← opt-in, can be slow
Layer 3: Dynamic Call Patterns (source search)   ← opt-in, needs scope
Layer 4: Config-Driven Calls (source + tables)   ← opt-in, needs scope/SQL
```

### Layer details

**Layer 1:** `FindReferences()` from `codeintel.go:110` returns `[]UsageReference`. Convert to `[]ImpactedObject` via field mapping: `URI→URI`, `Name→Name`, `Type→Type`, `PackageName→Package`. Define `ImpactedObject` as a wrapper type in `impact.go`.

**Layer 2:** `GetCallersOf()` from `client.go:1186` + `FlattenCallGraph()` → deduplicate against Layer 1 URIs via visited set (prevents cycles), tag depth. Stable sort by URI.

**Layer 3:** Search for target object NAME as string literal in scope packages via `GrepPackages()`. Higher signal than generic dynamic call syntax patterns.

**Layer 4a:** Source analysis of target object for ENHANCEMENT-SECTION, GET BADI, EXIT_, IF_EX_*/IF_BADI_*, PERFORM...IN PROGRAM

**Layer 4b:** RunQuery on SXS_INTER, MODSAP, TNAPR — graceful degradation if SAP_BLOCK_FREE_SQL

**Safety:** `checkSafety(OpRead)` at entry. Layer 4b uses RunQuery's `checkSafety(OpFreeSQL)`. Timeout: 30s default.

### Tests (8+)
- [ ] TestGetImpactAnalysis_StaticOnly
- [ ] TestGetImpactAnalysis_WithTransitive
- [ ] TestGetImpactAnalysis_EmptyResult
- [ ] TestGetImpactAnalysis_MaxResults
- [ ] TestDetectDynamicPatterns
- [ ] TestDetectConfigPatterns_Source
- [ ] TestDetectConfigPatterns_BAdIInterface
- [ ] TestImpactSummary_RiskLevel

---

## Phase 3: AnalyzeABAPCode (source-level pattern detection) ✅

**Architecture:** Two-pass statement-based (NOT line-by-line):
1. Statement Assembler — join lines between periods, respect comments & strings & string templates
2. Context-tracking rule engine — LOOP/DO/WHILE nesting, method scope, prev-statement context

**Statement assembler details (verified via SAP ABAP Keyword Documentation context7):**
- Classic strings `'...'` and backtick strings `` `...` `` — single-line only, no embedded period issue
- String templates `|...|` — **single-line only** (per SAP docs: "must be closed with `|` within the same line"), BUT may contain periods inside (e.g. `|Value is { lv_val }. Done|`). The assembler must track `insideStringTemplate` state within each line and skip periods inside `|...|`
- Escaped pipe inside templates: `\|` — does NOT close the template
- Comment lines: `*` in column 1 — skip entire line
- Inline comments: `"` outside strings — strip everything after `"`
- Statement terminator: `.` at end of accumulated text, outside any string literal or template
- **Input size limit:** max 500KB source text (larger files return error, not OOM)

**New files:**
- `pkg/adt/codeanalysis.go` — statement assembler + 21-rule engine
- `pkg/adt/codeanalysis_test.go` — 15 tests

**Modified files:**
- `internal/mcp/handlers_codeanalysis.go` (NEW)
- `internal/mcp/server.go`

### Rule catalog (21 rules)

| # | Rule ID | Category | Severity |
|---|---------|----------|----------|
| 1 | `select_in_loop` | performance | critical |
| 2 | `select_star` | performance | info |
| 3 | `fae_no_empty_check` | performance | critical |
| 4 | `nested_loop` | performance | high |
| 5 | `select_endselect` | performance | medium |
| 6 | `modify_dbtab_all` | performance | high |
| 7 | `commit_in_loop` | performance | critical |
| 8 | `read_table_no_binary` | performance | medium |
| 9 | `missing_authority_check` | security | info |
| 10 | `hardcoded_credentials` | security | critical |
| 11 | `dynamic_sql_unvalidated` | security | high |
| 12 | `client_specified` | security | medium |
| 13 | `missing_sysubrc_read` | robustness | medium |
| 14 | `missing_sysubrc_call` | robustness | medium |
| 15 | `empty_catch` | robustness | medium |
| 16 | `catch_cx_root` | robustness | medium |
| 17 | `obsolete_statement` | quality | info |
| 18 | `dynamic_call_no_try` | quality | high |
| 19 | `perform_usage` | quality | info |
| 20 | `todo_fixme` | quality | info |
| 21 | `commit_work_and_wait` | quality | medium |

### Tests (16)
- [ ] TestAssembleStatements_MultiLine
- [ ] TestAssembleStatements_Comments
- [ ] TestAssembleStatements_StringLiterals
- [ ] TestAssembleStatements_StringTemplates — `|Value is { x }. Done|` period inside template not a terminator
- [ ] TestAnalyzeABAPSource_SelectInLoop
- [ ] TestAnalyzeABAPSource_FAENoCheck
- [ ] TestAnalyzeABAPSource_FAEWithCheck
- [ ] TestAnalyzeABAPSource_NestedLoop
- [ ] TestAnalyzeABAPSource_CommitInLoop
- [ ] TestAnalyzeABAPSource_MissingSysubrc
- [ ] TestAnalyzeABAPSource_HardcodedCredentials
- [ ] TestAnalyzeABAPSource_DynamicCallNoTry
- [ ] TestAnalyzeABAPSource_CleanCode
- [ ] TestCodeAnalysisSummary_Score
- [ ] TestClient_AnalyzeABAPCode_URI
- [ ] TestClient_AnalyzeABAPCode_Source

---

## Phase 4: CheckRegression (diff-based breaking change detection) ✅

**MOST CRITICAL tool.** "Did the change break existing behavior?"

**New files:**
- `pkg/adt/regression.go` — types + diff analysis
- `pkg/adt/regression_test.go` — 8 tests

**Modified files:**
- `internal/mcp/handlers_codeanalysis.go` — add handler
- `internal/mcp/server.go`

### Regression rules (4)

| # | Rule ID | Severity | Limitation |
|---|---------|----------|------------|
| 1 | `changed_signature` | critical | CLAS/INTF only, NOT FUNC |
| 2 | `removed_public_method` | critical | CLAS/INTF only |
| 3 | `changed_interface_method` | critical | INTF only |
| 4 | `changed_exception_handling` | high | CLAS/INTF only |

**Known limitation:** FM signatures in XML metadata, not source.

**Diff size limit:** Before calling `generateUnifiedDiff`, check total lines < 2000 per source. If larger, return truncated warning instead of OOM risk. The existing `generateUnifiedDiff` uses O(m*n) LCS which can spike memory on large files.

**Version flow:** GetRevisions → pick base version → GetRevisionSource → compare (with size limit) → detect regressions

### Tests (8)
- [ ] TestDetectSignatureChanges_ParamAdded
- [ ] TestDetectSignatureChanges_ParamTypeChanged
- [ ] TestDetectRemovedPublicMethods_Removed
- [ ] TestDetectRemovedPublicMethods_Renamed
- [ ] TestDetectInterfaceChanges_MethodAdded
- [ ] TestDetectExceptionChanges_RaisingChanged
- [ ] TestDetectRegressions_NoChanges
- [ ] TestClient_CheckRegression

---

## Phase 5: Documentation + Commit ✅

- Add all 4 tools to `focusedTools` map in `server.go` (without this, tools only appear in expert mode)
- Update `CLAUDE.md`: tool count (142), test count (~337), project status
- Update `docs/FUTURE-PLAN.md`: mark Intelligence Layer done
- Update `docs/PLAN.md`: mark all phases complete
- Commit + push

---

## Key Reused Infrastructure

| Existing Function | File | Used By |
|-------------------|------|---------|
| `FindReferences()` | `codeintel.go:110` | GetImpactAnalysis Layer 1 |
| `GetCallersOf()` | `client.go:1186` | GetImpactAnalysis Layer 2 |
| `FlattenCallGraph()` | `client.go:1220` | GetImpactAnalysis Layer 2 |
| `GrepPackages()` | `workflows.go:1820` | GetImpactAnalysis Layer 3 |
| `GetSource()` | `workflows.go` | GetImpactAnalysis L4a, AnalyzeABAPCode, CheckRegression |
| `RunQuery()` | `client.go` | GetImpactAnalysis Layer 4b |
| `GetSQLExplainPlan()` | `testing.go:201` | AnalyzeSQLPerformance |
| `FeatureProber.IsAvailable()` | `features.go` (Server layer) | AnalyzeSQLPerformance (HANA detection, passed as `hanaAvailable bool`) |
| `GetRevisions()` | `revisions.go` | CheckRegression |
| `GetRevisionSource()` | `revisions.go` | CheckRegression |
| `generateUnifiedDiff()` | `workflows.go:3266` | CheckRegression |
| `checkSafety()` | `safety.go` | All 4 tools |

## New Files Summary

| File | Phase | Content |
|------|-------|---------|
| `pkg/adt/sqlperf.go` | 1 | SQL perf analysis + HANA fallback |
| `pkg/adt/sqlperf_test.go` | 1 | 7 tests |
| `pkg/adt/impact.go` | 2 | Impact analysis 4-layer orchestration |
| `pkg/adt/impact_test.go` | 2 | 10 tests |
| `pkg/adt/codeanalysis.go` | 3 | Statement assembler + 21-rule engine |
| `pkg/adt/codeanalysis_test.go` | 3 | 15 tests |
| `pkg/adt/regression.go` | 4 | Diff-based regression detection |
| `pkg/adt/regression_test.go` | 4 | 8 tests |
| `internal/mcp/handlers_intelligence.go` | 1-2 | SQL perf + Impact handlers |
| `internal/mcp/handlers_codeanalysis.go` | 3-4 | ABAP analysis + Regression handlers |

## Verification

1. `go test ./pkg/adt/ -run TestAnalyzePlan` — Phase 1
2. `go test ./pkg/adt/ -run TestGetImpact` — Phase 2
3. `go test ./pkg/adt/ -run "TestAnalyzeABAP|TestAssemble"` — Phase 3
4. `go test ./pkg/adt/ -run TestDetect` — Phase 4
5. `go test ./...` — no regressions (~335+ tests)
6. `go build ./cmd/vsp` — compiles
