# Plan: Upstream Merge & Fork Modernization (Migration RFC)

**Created:** 2026-04-05
**Updated:** 2026-04-12
**Status:** IN PROGRESS (Phases 1.1-1.4 complete, 1.5-1.6 remain)
**Pipeline:** migration-3032514f
**Cross-validation:** [C+O] Consensus -- Porfiry [Opus 4.6] + GPT-5.2-Pro (for+against), 8/10 confidence

---

## Context

We maintain a fork (blicksten/vibing-steampunk) of upstream (oisee/vibing-steampunk). Two upstream merges completed:
- **2026-04-05**: First merge (23 commits) — commit `46c0ead`
- **2026-04-12**: Second merge (~90 commits) — commit `59e9537`

### PR History with Upstream

| PR | Topic | Status | Upstream Response |
|----|-------|--------|-------------------|
| #97 | SAML SSO | **MERGED** | Accepted without changes. Follow-up PR #102 for review notes. |
| #95 | Fix releaseState bug | **MERGED** | Fix on top of PR #93 |
| #89 | AnalyzeABAPCode v2 (abaplint) | Cherry-picked | 3 fixes: CatchCxRoot narrowed, HardcodedCredentials tightened, categories corrected |
| #86 | Intelligence Layer | Rejected | "Does not compile, duplicates existing lexer, LLM does this natively" |
| #85 | CDS tools | Cherry-picked | GetCDSImpactAnalysis + GetCDSElementInfo merged |
| #84 | Testing tools | Cherry-picked | GetCodeCoverage + GetCheckRunResults. GetSQLExplainPlan dropped (unverified) |
| #83 | Version history | **MERGED** | Full merge |
| #82 | Refactoring tools | Rejected | Wrong ADT API URLs. Reference: `abap-adt-api/src/api/refactor.ts` |

### Issues Resolved Upstream

| Issue | Fix | Commit |
|-------|-----|--------|
| #88 Lock handle bug | Stateful sessions for lock/write/unlock | `27f4d7c` |
| #90 Auth headers on redirects | CheckRedirect preserves Authorization header | `27f4d7c` |

### New Upstream Features (gained from merge)

| Feature | Key Files |
|---------|-----------|
| Package boundaries/health/changelog | cmd/vsp/{devops,changelog,changes,acquire}.go |
| Slim V2 dead code analysis | Multi-level, hierarchical scope |
| Graph exports (DOT, PlantUML, GraphML, Mermaid) | pkg/graph/format_export.go |
| Side effect extraction + LUW | CALL TRANSACTION/TRANSFORMATION extraction |
| SQLite cache | pkg/cache/{cache,sqlite,memory}.go |
| tr-boundaries, cr-boundaries, cr-history | internal/mcp/handlers_transport_analysis.go |
| Default mode: focused → hyperfocused | cmd/vsp/main.go |
| TADIR batch (5-item limit), TFDIR fallback | pkg/adt/client.go |
| SAP_ALLOWED_PACKAGES enforcement | Mutation safety gate |

### Fork code deleted by upstream (status after merge)

| File | Decision | Current State |
|------|----------|---------------|
| pkg/adt/codeanalysis.go | **Rewritten as v2 on abaplint** | EXISTS (219 lines, upstream cherry-pick of PR #89) |
| pkg/adt/refactoring.go | **Rewrite with correct APIs** | EXISTS (264 lines, v1 with wrong URLs — needs v2) |
| pkg/adt/quickfix.go | **Rewrite with correct APIs** | MISSING (needs v2 implementation) |
| pkg/adt/impact.go | **RE-ADD** | EXISTS (fork-only) |
| pkg/adt/regression.go | **RE-ADD** | EXISTS (fork-only) |
| pkg/adt/sqlperf.go | **RE-ADD** | EXISTS (fork-only) |
| pkg/adt/ddic_test.go | **RE-ADD** | EXISTS (fork-only) |
| internal/mcp/handlers_intelligence.go | **RE-ADD** | EXISTS (fork-only) |
| internal/mcp/handlers_codeanalysis.go | **REWRITE** | EXISTS (v2 abaplint-based) |
| internal/mcp/handlers_refactoring.go | **REWRITE** | EXISTS (v1 — needs v2 update) |
| internal/mcp/tools_register_fork.go | **Extension hook** | EXISTS (hook at tools_register.go:93) |
| .claude/ directory | **RE-ADD** | EXISTS (fork-only config) |

### Branches to clean up

| Branch | Status | Action |
|--------|--------|--------|
| feat/http-sse-transport | Already deleted | DONE |
| feat/atc-transport-timeout | Already deleted | DONE |
| feat/intelligence-layer | Already deleted | DONE |
| feat/adt-refactoring | Already deleted | DONE |
| feat/cds-tools | Already deleted | DONE |
| feat/testing-quality | Already deleted | DONE |
| feat/version-history | Already deleted | DONE |
| local-changes-backup-2026-02-18 | Already deleted | DONE |
| feat/analyze-abapcode-v2 | Local + remote (myfork) | **DELETE** (merged via PR #89) |
| saml-auth | Local + remote (myfork) | **DELETE** (merged via PR #97) |
| fix/saml-review-followup | Local + remote (myfork) | **KEEP** (active PR #102) |
| origin/feat/saml-sp-initiated-background-auth | Remote (origin) | **REVIEW** (Guzel's branch, has CRITICAL issues) |

---

## ADR-001: Merge Strategy -- Accept Deletions, Re-add as New Commits

**Status:** Accepted, Executed [C+O]

Executed successfully. Upstream deletions accepted. Fork-only code re-added as separate commits. Future merges confirmed clean — commit `59e9537` had only 6 conflicts (imports, lint rules), none in fork-only files.

## ADR-002: Extension Hook Pattern for Tool Registration

**Status:** Accepted, Implemented [C+O]

Hook at `tools_register.go:93`: `s.registerForkTools(shouldRegister)`. Fork tools registered via `tools_register_fork.go`. Pattern validated across 2 upstream merges — zero conflicts on the hook line.

## ADR-003: AnalyzeABAPCode v2 -- Build on pkg/abaplint

**Status:** Accepted, Implemented [C+O]

Upstream cherry-picked our PR #89 with 3 improvements:
1. CatchCxRootRule narrowed to CX_ROOT/CX_STATIC_CHECK/CX_DYNAMIC_CHECK/CX_NO_CHECK (CX_SY_* excluded)
2. HardcodedCredentialsRule: "token" → specific token patterns (auth_token, access_token, etc.)
3. catch_cx_root/dynamic_call_no_try categorized as "robustness" (not "security")

ColonMissingSpaceRule Row:0 bug fixed (now uses `rowIdx + 1`). All 37 tests pass.

## ADR-004: Refactoring & QuickFix v2 -- Correct ADT API URLs

**Status:** Proposed (deferred to Phase 1.5)

Reference: `abap-adt-api/src/api/refactor.ts` (592 lines). Not yet implemented.

---

## Implementation Phases

### Phase 1.1: Upstream Merge ~~(merge + accept deletions + build fix)~~

- [x] T1.1: Fetch upstream: `git fetch upstream`
- [x] T1.2: ~~Create migration branch~~ Merged directly on main (simpler for fork workflow)
- [x] T1.3: Run `git merge upstream/main` — resolved 6 conflicts:
  - `devtools.go`: kept both `net/url` (ours) and `os` (upstream) imports
  - `analysis_types.go`: kept our SQL types (used by sqlperf/regression)
  - `codeanalysis.go`: adopted upstream robustness category for catch_cx_root
  - `codeanalysis_test.go`: aligned test with upstream robustness category
  - `rules.go`: adopted upstream broadExceptions map + expanded credential names
  - `lint_test.go`: adopted upstream CX_SY_ no-issue + broad class tests
- [x] T1.4: `go build ./...` — clean
- [x] T1.5: `go test ./...` — all pass (except pkg/cache Example_withSQLite — upstream CGO issue)
- [x] T1.6: Commit merge — `59e9537`
- [x] GATE: Build + tests pass ✓

### Phase 1.2: Re-add Valuable Fork Code

- [x] T2.1: `pkg/adt/impact.go` + test — EXISTS (survived merge, fork-only)
- [x] T2.2: `pkg/adt/regression.go` + test — EXISTS
- [x] T2.3: `pkg/adt/sqlperf.go` + test — EXISTS
- [x] T2.4: `pkg/adt/ddic_test.go` — EXISTS
- [x] T2.5: `internal/mcp/handlers_intelligence.go` — EXISTS
- [x] T2.6: `internal/mcp/tools_register_fork.go` — EXISTS
- [x] T2.7: Hook in `tools_register.go:93` — EXISTS
- [x] T2.8: `internal/mcp/handlers_codeanalysis.go` — EXISTS (v2 abaplint-based)
- [x] T2.9: `.claude/` directory — EXISTS
- [x] T2.10: `go build ./...` + `go test ./...` — pass ✓
- [x] GATE: Build + tests pass, all fork tools register correctly ✓

### Phase 1.3: Branch Cleanup

- [x] T3.1: Delete original stale local branches (all 8 from original plan) — DONE (previously)
- [ ] T3.2: Delete merged branches: `feat/analyze-abapcode-v2` (local + myfork), `saml-auth` (local + myfork)
- [x] T3.3: ~~Merge migration branch to main~~ Not needed (merged directly on main)
- [ ] T3.4: Review Guzel's branch `origin/feat/saml-sp-initiated-background-auth` (CRITICAL deadlock, HIGH data race)
- [ ] GATE: `git branch -a` shows clean state (only main, fix/saml-review-followup, Guzel's branch)

### Phase 1.4: AnalyzeABAPCode v2 (abaplint-based)

- [x] T4.1: Fix ColonMissingSpaceRule Row:0 bug — FIXED (uses `rowIdx + 1`)
- [x] T4.2: New abaplint rules added:
  - hardcoded_credentials ✓ (with upstream's specific token patterns)
  - select_star ✓
  - catch_cx_root ✓ (with upstream's broadExceptions map)
  - commit_in_loop ✓
  - dynamic_call_no_try ✓
- [x] T4.3: `pkg/adt/codeanalysis.go` rewritten to use abaplint (219 lines) ✓
- [x] T4.4: `internal/mcp/handlers_codeanalysis.go` updated ✓
- [x] T4.5: Tests: 27 lint + 10 analysis tests ✓
- [x] T4.6: `go test ./pkg/abaplint/... ./pkg/adt/...` — all pass ✓
- [x] GATE: All tests pass, abaplint rules produce correct findings ✓

### Phase 1.5: Refactoring & QuickFix v2 (correct ADT APIs) — TODO

- [ ] T5.1: Research correct API URLs from `abap-adt-api/src/api/refactor.ts`
- [ ] T5.2: Implement `pkg/adt/refactoring_v2.go` with correct endpoints
- [ ] T5.3: Implement `pkg/adt/quickfix_v2.go` with correct endpoints
- [ ] T5.4: Add feature detection for refactoring API availability
- [ ] T5.5: Update `internal/mcp/handlers_refactoring.go`
- [ ] T5.6: Write unit tests with mock HTTP responses
- [ ] T5.7: Integration test against real SAP system (manual)
- [ ] GATE: Unit tests pass, API URLs verified against abap-adt-api reference

### Phase 1.6: Documentation & Final Verification — TODO

- [ ] T6.1: Update `README.md` tool tables with new upstream tools
- [ ] T6.2: Update `CLAUDE.md` tool counts and project status
- [ ] T6.3: Write migration report: `reports/2026-04-12-upstream-merge-90-commits.md`
- [ ] T6.4: Run full test suite: `go test ./...`
- [ ] T6.5: Build all platforms: `go build -o vsp ./cmd/vsp`
- [ ] T6.6: Push to GitLab: `git push origin main` (requires VPN)
- [ ] GATE: Build + all tests pass, documentation accurate

---

## Risk Register

| Risk | Severity | Status |
|------|----------|--------|
| mcp-go v0.47 API breaking changes | HIGH | **RESOLVED** — build clean after merge |
| chromedp dependency breaks CI/containers | MEDIUM | **RESOLVED** — browser auth is opt-in |
| Re-added impact/regression/sqlperf Client API drift | MEDIUM | **RESOLVED** — build + tests pass |
| Refactoring v2 API varies by SAP release | MEDIUM | OPEN — Phase 1.5 |
| abaplint parser coverage gaps (modern ABAP) | LOW | ACCEPTED — unknown stmts degrade to Unknown type |
| Future upstream filename collisions | LOW | ACCEPTED — 2 merges with zero fork-file conflicts |
| pkg/cache SQLite requires CGO | LOW | NEW — Example_withSQLite fails without CGO |
| Guzel's branch has CRITICAL deadlock | HIGH | OPEN — needs review before merge |

## Compatibility Strategy

**Principle:** Minimize divergence surface area.

1. **Fork-only code in separate files** — never modify upstream-origin files beyond the ONE registration hook
2. **Extension hook pattern** — `tools_register_fork.go` holds all fork tool registrations
3. **Separate handler files** — `handlers_intelligence.go`, `handlers_codeanalysis.go`, `handlers_refactoring.go`
4. **pkg/abaplint stays independent** — pure Go, no ADT client dependencies, upstream-portable
5. **Integration tests behind build tags** — fork-only integration tests use `//go:build integration`

**Validated merge workflow (proven across 2 merges):**

```bash
git fetch upstream
git merge upstream/main
# Conflicts limited to:
#   - Shared lint rules (credentialNames, broadExceptions) — adopt upstream
#   - Import lists (both sides add imports) — merge both
#   - go.mod (dependency drift) — normal
# Fork-only files in pkg/adt/ and internal/mcp/handlers_*.go — zero conflicts
```

## Cross-validation Summary

| Aspect | Porfiry [Opus 4.6] | GPT-5.2-Pro (for) | GPT-5.2-Pro (against) | Tag |
|--------|--------------------|--------------------|----------------------|-----|
| Accept deletions strategy | Agree | Agree (8/10) | Agree (8/10) | [C+O] |
| Extension hook pattern | Agree | Agree | Agree -- critical | [C+O] |
| Abandon codeanalysis v1 | Agree | Agree | Agree | [C+O] |
| Abandon refactoring/quickfix v1 | Agree | Agree | Agree | [C+O] |
| Re-add impact/regression/sqlperf | Agree | Agree (low risk) | Caution (client deps) | [C+O] |
| abaplint-based v2 | Agree | Agree (long-term bet) | Agree (fix Row:0 first) | [C+O] |
| Never modify upstream files | Aspirational | Aspirational | Unrealistic (need 1 seam) | [C+O] refined |

No CRITICAL disagreements. All findings incorporated into the plan above.
