# Plan: Upstream Merge & Fork Modernization (Migration RFC)

**Created:** 2026-04-05
**Status:** PROPOSED
**Pipeline:** migration-3032514f
**Cross-validation:** [C+O] Consensus -- Porfiry [Opus 4.6] + GPT-5.2-Pro (for+against), 8/10 confidence

---

## Context

We maintain a fork (blicksten/vibing-steampunk) of upstream (oisee/vibing-steampunk). Upstream has 23 new commits we need to merge. Three of our five PRs were accepted (fully or partially), two were rejected. Upstream deleted all our Intelligence Layer and Refactoring code.

### Upstream inventory (to gain)

| Feature | Files | Impact |
|---------|-------|--------|
| mcp-go v0.17 to v0.47 | go.mod, go.sum, server.go | Streamable HTTP + SSE transport |
| Browser SSO (chromedp) | pkg/adt/browser_auth.go + test | New dependency: chromedp |
| 10 gCTS tools | pkg/adt/gcts.go + test | Git-enabled CTS |
| 7 i18n tools | pkg/adt/i18n.go + test | Translation management |
| GetAPIReleaseState | client.go or devtools.go | Clean Core checks |
| GetTableContents offset/columns_only | client.go | Enhanced query |
| GetCodeCoverage + GetCheckRunResults | (from PR #84) | Testing quality |
| CDS impact analysis + element info | (from PR #85) | CDS tools |
| Various fixes | websocket_base.go, config.go, http.go | Verbose, WebSocket auth, stateless default |

### Fork code deleted by upstream

| File | Why deleted | Our decision |
|------|-------------|-------------|
| pkg/adt/codeanalysis.go + test | PR #86 -- regex-based, owner wants abaplint | **ABANDON v1, rewrite as v2 on abaplint** |
| pkg/adt/refactoring.go + test | PR #82 -- wrong ADT API URLs | **ABANDON, rewrite with correct APIs** |
| pkg/adt/quickfix.go + test | PR #82 -- wrong ADT API URLs | **ABANDON, rewrite with correct APIs** |
| pkg/adt/impact.go + test | Deleted along with Intelligence Layer | **RE-ADD** (pure Go, uses ADT cross-refs correctly) |
| pkg/adt/regression.go + test | Deleted along with Intelligence Layer | **RE-ADD** (pure Go source comparison) |
| pkg/adt/sqlperf.go + test | Deleted along with Intelligence Layer | **RE-ADD** (pure Go SQL plan analysis) |
| pkg/adt/integration_test.go | Upstream has own version | **ACCEPT upstream version** + merge our extra tests |
| pkg/adt/ddic_test.go | Deleted | **RE-ADD** (fork-only test) |
| internal/mcp/handlers_intelligence.go | Handler for deleted tools | **RE-ADD** (for impact/regression/sqlperf) |
| internal/mcp/handlers_codeanalysis.go | Handler for deleted tools | **REWRITE** (for v2 abaplint-based AnalyzeABAPCode) |
| internal/mcp/handlers_refactoring.go | Handler for deleted tools | **REWRITE** (for v2 correct-API refactoring) |
| .claude/ directory | Our team config | **RE-ADD** (fork-only, gitignored by upstream) |

### Branches to clean up

| Branch | Status | Action |
|--------|--------|--------|
| feat/http-sse-transport | Upstream has mcp-go v0.47 SSE | **DELETE** |
| feat/atc-transport-timeout | Upstream has RunATCCheckTransport | **DELETE** |
| feat/intelligence-layer | Merged to main already | **DELETE** |
| feat/adt-refactoring | Uses wrong API URLs | **DELETE** |
| feat/cds-tools | Upstream accepted PR #85 | **DELETE** |
| feat/testing-quality | Upstream accepted PR #84 | **DELETE** |
| feat/version-history | Upstream accepted PR #83 | **DELETE** |
| local-changes-backup-2026-02-18 | Old backup | **DELETE** |

---

## ADR-001: Merge Strategy -- Accept Deletions, Re-add as New Commits

**Status:** Proposed

**Decision:** Use `git merge upstream/main` and intentionally accept upstream deletion of all rejected files. Re-add valuable fork-only code as NEW commits after merge.

**Rationale:**
- If we keep deleted files during merge, upstream will re-delete them in every future merge (delete/modify conflict loop)
- Accepting deletions then re-adding creates a clean merge base -- future upstream merges will not touch our re-added files
- Re-adding as separate commits creates clear git history showing deliberate intent
- Cross-validated [C+O] at 8/10 confidence

**Alternatives rejected:**
- **Cherry-pick upstream commits** -- More labor, higher risk of missing coupling changes across transport refactor
- **Rebase onto upstream** -- Force-push risk, painful with large deltas + published fork
- **Keep files during merge** -- Perpetual delete/modify conflicts, accumulating merge debt

## ADR-002: Extension Hook Pattern for Tool Registration

**Status:** Proposed

**Decision:** Create `internal/mcp/tools_register_fork.go` with a single `registerForkTools(shouldRegister)` function. Add ONE call to this from `tools_register.go`. All fork-only tool registrations live in the fork file, never in upstream registration functions.

**Rationale:**
- Tool registration requires at least one integration point in upstream code
- Minimizing upstream-touching changes to ONE added line reduces future merge conflicts
- Fork-specific handler files (handlers_*.go) already exist as separate files -- this extends the pattern to registration
- Cross-validated [C+O]: both models agreed this is the correct integration pattern

**The one seam (added to tools_register.go):**

```go
// After upstream last registerXxxTools call:
s.registerForkTools(shouldRegister)  // fork-only tools (intelligence, refactoring v2)
```

**Contents of tools_register_fork.go:**

```go
func (s *Server) registerForkTools(shouldRegister func(string) bool) {
    s.registerIntelligenceTools(shouldRegister)  // impact, regression, sqlperf, AnalyzeABAPCode v2
    s.registerRefactoringToolsV2(shouldRegister) // rename, quickfix (correct ADT APIs)
    s.registerRunATCCheckTransport(shouldRegister)
    s.registerCDSExtTools(shouldRegister)
}
```

## ADR-003: AnalyzeABAPCode v2 -- Build on pkg/abaplint

**Status:** Proposed

**Decision:** Rewrite AnalyzeABAPCode to use the native Go abaplint lexer + statement parser instead of the current regex-based line scanner.

**Current state (v1):**
- `pkg/adt/codeanalysis.go` uses `AssembleStatements()` -- hand-written period-terminated line joiner
- 14 regex patterns for rule matching
- No statement type awareness (cannot distinguish IF from SELECT context)

**Target state (v2):**
- Use `abaplint.NewABAPFile(filename, source)` which runs lexer -> statement parser -> classifier
- 91 classified statement types already available (Data, If, Select, MethodImplementation, etc.)
- 8 lint rules already exist (line_length, empty_statement, obsolete_statement, max_one_statement, preferred_compare_operator, colon_missing_space, double_space, local_variable_names)
- Add ABAP-specific security/performance rules as new Rule implementations

**Architecture:**

```
pkg/abaplint/                        <- lexer, parser, rules (pure Go, no ADT deps)
pkg/adt/codeanalysis.go              <- Client method (fetches source, calls abaplint)
internal/mcp/handlers_codeanalysis.go <- MCP handler
```

**Known issue to fix:** `ColonMissingSpaceRule` in `rules.go` sets `Row: 0` instead of the actual line number. Must fix before using for MCP output.

**Aligns with upstream owner request:** "consider building on top of the existing pkg/abaplint lexer + statement parser"

## ADR-004: Refactoring & QuickFix v2 -- Correct ADT API URLs

**Status:** Proposed (deferred to post-merge phase)

**Decision:** Rewrite refactoring and quickfix tools from scratch using the correct ADT REST API patterns from the `abap-adt-api` reference implementation.

**Current (WRONG) API patterns:**

| Operation | Current URL | Correct URL |
|-----------|-------------|-------------|
| Rename | `/sap/bc/adt/refactoring/rename?method=evaluate` | `/sap/bc/adt/refactorings?rel=http://www.sap.com/refactoring/rename` with `?step=` |
| QuickFix | `/sap/bc/adt/quickfix/proposals` | `/sap/bc/adt/quickfixes/evaluation` |
| Body format | Plain text source | Complex XML with genericRefactoring wrapper |
| Routing | `?method=evaluate/preview/execute` | `?step=test_before_refactoring` / `?step=apply` with `?rel=` |

**Implementation notes:**
- Reference: `abap-adt-api/src/api/refactor.ts` (Marcel Goldammer / marcellourbani)
- Requires trace-based validation against real SAP systems (API varies by release)
- Must include feature detection -- not all systems support refactoring APIs
- Plan for graceful degradation with informative error messages

---

## Implementation Phases

### Phase 1.1: Upstream Merge (merge + accept deletions + build fix)

- [ ] T1.1: Fetch upstream: `git fetch upstream`
- [ ] T1.2: Create migration branch: `git checkout -b migration/upstream-merge-v0.47`
- [ ] T1.3: Run `git merge upstream/main` -- resolve conflicts:
  - `tools_register.go`: accept upstream version (loses our 5 extra registration calls -- intentional)
  - `go.mod` / `go.sum`: accept upstream (mcp-go v0.47, chromedp deps)
  - `.claude/`: keep ours (re-add after merge if deleted)
  - All rejected files: accept upstream deletion
  - `integration_test.go`: accept upstream version
- [ ] T1.4: Run `go build ./...` -- fix any compilation errors from merge
- [ ] T1.5: Run `go test ./...` -- fix any test failures from merge
- [ ] T1.6: Commit merge
- [ ] GATE: `go build` + `go test ./...` pass with zero errors

### Phase 1.2: Re-add Valuable Fork Code

- [ ] T2.1: Re-add `pkg/adt/impact.go` + `pkg/adt/impact_test.go` -- restore from pre-merge commit, fix Client API changes
- [ ] T2.2: Re-add `pkg/adt/regression.go` + `pkg/adt/regression_test.go`
- [ ] T2.3: Re-add `pkg/adt/sqlperf.go` + `pkg/adt/sqlperf_test.go`
- [ ] T2.4: Re-add `pkg/adt/ddic_test.go`
- [ ] T2.5: Re-add `internal/mcp/handlers_intelligence.go` -- handler for impact/regression/sqlperf
- [ ] T2.6: Create `internal/mcp/tools_register_fork.go` with `registerForkTools()` -- extension hook
- [ ] T2.7: Add ONE line to `tools_register.go`: `s.registerForkTools(shouldRegister)`
- [ ] T2.8: Re-add `internal/mcp/handlers_codeanalysis.go` with stub AnalyzeABAPCode + working CheckRegression
- [ ] T2.9: Re-add `.claude/` directory from our main branch
- [ ] T2.10: Run `go build ./...` + `go test ./...`
- [ ] GATE: Build + tests pass, all re-added tools register correctly

### Phase 1.3: Branch Cleanup

- [ ] T3.1: Delete local branches: feat/http-sse-transport, feat/atc-transport-timeout, feat/intelligence-layer, feat/adt-refactoring, feat/cds-tools, feat/testing-quality, feat/version-history, local-changes-backup-2026-02-18
- [ ] T3.2: Delete remote branches on myfork (GitHub)
- [ ] T3.3: Merge migration branch to main
- [ ] GATE: `git branch -a` shows clean state

### Phase 1.4: AnalyzeABAPCode v2 (abaplint-based)

- [ ] T4.1: Fix `ColonMissingSpaceRule` Row:0 bug in `pkg/abaplint/rules.go`
- [ ] T4.2: Add new abaplint rules for security patterns:
  - hardcoded_credentials -- detect password/secret assignments in string literals
  - select_star -- detect SELECT * FROM (using statement type awareness)
  - catch_cx_root -- detect overly broad exception handling
  - commit_in_loop -- detect COMMIT WORK inside LOOP/DO/WHILE
  - dynamic_call -- detect CALL METHOD (var) / CALL FUNCTION var
- [ ] T4.3: Rewrite `pkg/adt/codeanalysis.go` to use `abaplint.NewABAPFile()` + `abaplint.Linter`
- [ ] T4.4: Update `internal/mcp/handlers_codeanalysis.go` to use new AnalyzeABAPCode
- [ ] T4.5: Write unit tests for new rules (oracle-verified where possible)
- [ ] T4.6: Run `go test ./pkg/abaplint/...` + `go test ./pkg/adt/...`
- [ ] GATE: All tests pass, abaplint rules produce correct findings on test corpus

### Phase 1.5: Refactoring & QuickFix v2 (correct ADT APIs)

- [ ] T5.1: Research correct API URLs from `abap-adt-api/src/api/refactor.ts` -- document in report
- [ ] T5.2: Implement `pkg/adt/refactoring_v2.go` with correct endpoints
- [ ] T5.3: Implement `pkg/adt/quickfix_v2.go` with correct endpoints
- [ ] T5.4: Add feature detection for refactoring API availability
- [ ] T5.5: Update `internal/mcp/handlers_refactoring.go`
- [ ] T5.6: Write unit tests with mock HTTP responses
- [ ] T5.7: Integration test against real SAP system (manual, requires connection)
- [ ] GATE: Unit tests pass, API URLs verified against abap-adt-api reference

### Phase 1.6: Documentation & Final Verification

- [ ] T6.1: Update `README.md` tool tables with new upstream tools + our re-added tools
- [ ] T6.2: Update `CLAUDE.md` tool counts and project status
- [ ] T6.3: Write migration report: `reports/2026-04-05-001-upstream-merge-migration.md`
- [ ] T6.4: Run full test suite: `go test ./...`
- [ ] T6.5: Build all platforms: `go build -o vsp ./cmd/vsp`
- [ ] GATE: Build + all tests pass, documentation accurate

---

## Risk Register

| Risk | Severity | Mitigation |
|------|----------|------------|
| mcp-go v0.47 API breaking changes | HIGH | Build + test after merge before re-adding code |
| chromedp dependency breaks CI/containers | MEDIUM | Browser auth is opt-in; build tag if needed |
| Re-added impact/regression/sqlperf have Client API drift | MEDIUM | Fix Client method signatures after transport upgrade |
| Refactoring v2 API varies by SAP release | MEDIUM | Feature detection + graceful degradation |
| abaplint parser coverage gaps (modern ABAP) | LOW | Unknown statements degrade to Unknown type, not crash |
| Future upstream filename collisions on re-added files | LOW | Our files have unique names; upstream explicitly deleted them |

## Compatibility Strategy

**Principle:** Minimize divergence surface area.

1. **Fork-only code in separate files** -- never modify upstream-origin files beyond the ONE registration hook
2. **Extension hook pattern** -- `tools_register_fork.go` holds all fork tool registrations
3. **Separate handler files** -- `handlers_intelligence.go`, `handlers_codeanalysis.go`, `handlers_refactoring.go`
4. **pkg/abaplint stays independent** -- pure Go, no ADT client dependencies, upstream-portable
5. **Integration tests behind build tags** -- fork-only integration tests use `//go:build integration`

**Future merge workflow:**

```bash
git fetch upstream
git merge upstream/main
# Conflicts will be limited to:
#   - tools_register.go (our ONE added line)
#   - go.mod (dependency drift -- normal)
# Our files in pkg/adt/ and internal/mcp/handlers_*.go will NOT conflict
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

