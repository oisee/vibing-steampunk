# Quality Sprint Plan

**Date:** 2026-04-04
**Report ID:** 001
**Subject:** Quality & Stability Sprint — PR Triage, Issue Triage, Sprint Plan
**Scope:** 2 weeks, ~24h effort

---

## Executive Summary

14 open PRs, 18 open issues. Community growing (8 external contributors). Main risks: session handling (breaks POST operations), installer fragility on older SAP versions, ADT endpoint version differences. Sprint focuses on stability first, easy community intake second.

---

## PR Triage

### Merge Immediately (2-4h total)

| PR | Author | Title | Size | Risk | Action |
|---:|--------|-------|------|------|--------|
| **#80** | oklausen | docs: copilot-instructions.md | +117, 1 file | None | Merge as-is |
| **#44** | mv0101 | Windows Quick Start | +61, 1 file | Low | Merge with minor fixes (tab format in cookie template, HTML->MD links) |

### Merge After Short Adaptation (8-14h total)

| PR | Author | Title | Size | Risk | Technical Notes | Action |
|---:|--------|-------|------|------|-----------------|--------|
| **#37** | kts982 | Table pagination + schema | +92, 4 files | Low | Clean impl. Client-side offset correct. Remove unrelated `embed.go` diff | Merge with changes |
| **#53** | andreasmuenster | Clean Core API check | +44, 4 files | Medium | **Bug:** `url.PathEscape` breaks ADT URI paths (encodes `/`). Typo "neccessary" | Reimplement (fix PathEscape, parse XML response) |
| **#83** | blicksten | Version history (3 tools) | +744, 6 files | Low | **Endpoints verified real.** Atom feed API standard SAP. Check `generateUnifiedDiff` exists. Adds `adtcore:name` to `Link` type | Merge with minor check |
| **#85** | blicksten | CDS impact + element info | +576, 5 files | Medium | Real endpoints. **Safety bug in tests:** ReadOnly tests expect error for `OpRead` operations — fix tests | Merge after test fix |
| **#84** | blicksten | Testing & quality (3 tools) | +931, 5 files | Medium | Coverage + CheckRun good. **SQLExplainPlan dubious** — `EXPLAIN PLAN FOR` via datapreview not standard. Duplicate CSRF helper | Merge CodeCoverage+CheckRun, drop SQLExplainPlan |
| **#41** | Prolls | gCTS tools (10 tools) | +1026, 4 files | Medium | Clean structure, generics, safety checks. **No unit tests.** Legacy `Params.Arguments` API | Merge after: add tests, migrate to GetArguments() |
| **#42** | Prolls | i18n tools (7 tools) | +874, 6 files | Medium | Good `OverrideLanguage` in RequestOptions. **No unit tests.** Modifies core `buildURL()` signature | Merge after: add tests, careful review of http.go change |

### Reimplement Cleanly, Close PR with Thanks

| PR | Author | Title | Size | Why Reimplement | Est. |
|---:|--------|-------|------|-----------------|------|
| **#79** | snymanpaul | Stateless session default | +33, 3 files | **Correct diagnosis**, wrong fix location. Appends `WithSessionType(Stateless)` in `server.go` AFTER user opts — overrides user's explicit stateful choice. Fix: change default in `config.go:186` instead. Also: cookie jar reset has potential race condition | 1-2h |
| **#38** | danielheringers | mcp-go v0.43.2 + HTTP Streamable | +1149, 37 files | Strategic value high. But: all handlers need migration, massive rebase needed. Better as separate upgrade track | 4-8h |
| **#77** | cwbr | Browser SSO auth | +637, 10 files | Good impl (chromedp, multi-browser). But: mixes SSO + keepalive, heavy dependency. Split into keepalive + browser-auth. Consider build tags for chromedp | 4-6h |

### Defer to Post-Sprint

| PR | Author | Title | Size | Reason |
|---:|--------|-------|------|--------|
| **#82** | blicksten | ADT refactoring + quick fix | +1767, 7 files | **Endpoints likely fabricated** — ADT discovery marks Refactoring as "NOT Exposed". Need SAP verification first |
| **#86** | blicksten | Intelligence Layer | +4093, 15 files | **Does not compile** — calls `getSourceForAnalysis`, `parseObjectURIComponents` which don't exist. `AnalyzeABAPCode` duplicates existing `pkg/abaplint` lexer. Impact/SQL analysis — LLM does this natively. Needs full rewrite on top of existing lexer |

---

## Issue Triage

### Close After Docs/Confirmation

| Issue | Title | Resolution |
|------:|-------|------------|
| **#30** | Cookie authentication | Already implemented. Docs in README.md + CLAUDE.md. Close with pointer |
| **#43** | Missing commands | Partially fixed in PR #67. Rest is documentation gap. Add FAQ section |
| **#56** | Unable to create program | Likely auth/session issue (#79 cluster). Ask reporter to retry with v2.33.0+ verbose |

### Link to PR, Close After Merge

| Issue | Title | Resolved By |
|------:|-------|-------------|
| **#21** | Streaming HTTP support | PR #38 |
| **#34** | GetTableContents pagination | PR #37 |
| **#39** | gCTS tools | PR #41 |
| **#40** | i18n tools | PR #42 |
| **#74** | CDS metadata extensions (DDLX) | Partially by PR #85 |

### Fix in This Sprint (P0/P1)

| Issue | Title | Priority | Est. | Notes |
|------:|-------|----------|------|-------|
| **#81** | WRITE operation failed / CSRF | P0 | 2-3h | Reporter: CSRF token validation fails on writes. Likely same root cause as #79 (stateful session binds CSRF to wrong handler). Fix: stateless default + better CSRF retry |
| **#87** | InstallZADTVSP fails on 7.40 | P0 | 2-3h | Package creation 404 on old systems aborts entire install. Fix: fallback when `/sap/bc/adt/packages` returns 404, continue if package pre-exists |
| **#75** | InstallZADTVSP not idempotent | P1 | 1-2h | Same cluster as #87. Existing objects should update, not fail |
| **#76** | Graph fallback namespaced objects | P1 | 1-2h | Uses filesystem paths instead of ABAP names. Real correctness bug |
| **#26** | GetTransport failed | P1 | 1h | Recheck after transport fixes; quick diagnostic |

### Backlog (Not This Sprint)

| Issue | Title | Notes |
|------:|-------|-------|
| **#55** | RunReport APC spool blocked | Known limitation, needs architecture change |
| **#27** | Supporting of object types | Enhancement, ongoing. 158 types already in abapGit export |
| **#45** | Sync script: CLAUDE.md conflicts | Tooling, low priority |
| **#46** | Sync script: oisee references | Tooling, low priority |
| **#2** | GUI debugger | Original issue, parked |

---

## New Feature: ADT Capability Discovery

**Triggered by:** User feedback from Birzhan + Александр Гаращенко (2026-04-04 chat)

**Problem:** Different SAP versions expose different ADT endpoints. `vsp query` fails silently on systems without `/sap/bc/adt/datapreview/freestyle`. Users see cryptic errors.

**Proposed solution:**

1. **`vsp probe`** CLI command — test all ADT endpoints, report what works
2. **Runtime capability cache** — on first connection, probe key endpoints, cache results
3. **Graceful fallback** — if `freestyle` not available, suggest `GetTableContents` (uses `/datapreview/ddic` which may work on older systems)
4. **Clear error messages** — "This ADT endpoint is not available on your SAP system (requires SAP_BASIS >= 7.50)"

**Estimate:** 4-6h for probe command + capability cache. Stretch goal for this sprint.

---

## Sprint Structure (2 weeks, ~24h)

### Track A: Stability (8-12h) — P0

| # | Task | Issues | Est. |
|--:|------|--------|------|
| 1 | Fix session default (stateless) + CSRF retry | #81, #79-rewrite | 2-3h |
| 2 | Fix InstallZADTVSP for old systems + idempotency | #87, #75 | 3-4h |
| 3 | Fix graph fallback for namespaced objects | #76 | 1-2h |
| 4 | Verify GetTransport after fixes | #26 | 1h |
| 5 | ADT capability probe (stretch) | user feedback | 4-6h |

### Track B: Easy Intake (3-5h) — Quick Wins

| # | Task | PRs | Est. |
|--:|------|-----|------|
| 1 | Merge docs PRs | #80, #44 | 30min |
| 2 | Merge table pagination | #37 | 1h |
| 3 | Reimplement Clean Core check | #53 | 1-2h |
| 4 | Close resolved issues | #30, #43, #21, #34, #39, #40 | 30min |

### Track C: Medium Feature Intake (8-14h) — By Capacity

| # | Task | PRs | Est. |
|--:|------|-----|------|
| 1 | Review + merge Version History | #83 | 2h |
| 2 | Review + merge CDS tools (fix safety tests) | #85 | 2h |
| 3 | Review + merge CodeCoverage (drop SQLExplain) | #84 | 2h |
| 4 | Review + merge gCTS (add tests) | #41 | 3h |
| 5 | Review + merge i18n (add tests) | #42 | 3h |

### Stretch (Not Committed)

- PR #38 — mcp-go upgrade (separate track)
- PR #77 — Browser SSO (split into 2 PRs)
- PR #82 — Refactoring tools (verify endpoints on SAP first)
- PR #86 — Intelligence Layer (rewrite on pkg/abaplint)
- Issue #55 — RunReport spool

---

## Top 5 Quality Risks

1. **Session handling breaks POST operations** — #79/#81 cluster. Affects every user doing writes. Fix first.
2. **Installer not resilient to SAP version differences** — #87/#75. First-time users bounce. Need graceful fallback.
3. **No ADT endpoint discovery** — Users get cryptic errors when endpoint doesn't exist on their system version.
4. **mcp-go pinned at v0.17** — 26 versions behind. Growing tech debt, blocks HTTP Streamable.
5. **blicksten PR #86 doesn't compile** — If merged accidentally, breaks build. Needs clear "do not merge" label.

---

## PR Closure Protocol

When closing a community PR in favor of reimplementation:

```
Thank you for this contribution! The diagnosis and approach are solid.

We're reimplementing this slightly differently in [commit/PR link] to [reason].
Your work was instrumental in identifying and solving this — credited in the commit.

Closing in favor of the new implementation. 🙏
```
