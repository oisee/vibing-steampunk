# Remaining Backlog Closure Plan

**Date:** 2026-04-04
**Status after:** v2.34.0 Quality Sprint Phase 1

---

## Remaining: 7 PRs, 12 issues

## Sprint 2A: Quick Wins (3-4h)

- [ ] **PR #53** (Clean Core) — reimplement: fix PathEscape, parse XML, close PR. 1-2h
- [ ] **PR #41** (gCTS) — cherry-pick + add 5-8 unit tests, close PR. 2-3h
- [ ] Close **#39** (gCTS feature request) after #41 merge
- [ ] Close **#43** (missing commands) with docs pointer
- [ ] Close **#56** (create program) — ask retry with v2.34.0

## Sprint 2B: Medium (4-6h)

- [ ] **PR #42** (i18n) — cherry-pick, review buildURL change, add tests. 2-3h
- [ ] **PR #77** (Browser SSO) — split into keepalive + browser-auth. 3-4h
- [ ] Close **#40** (i18n feature request) after #42 merge
- [ ] Verify **#26** (GetTransport) on SAP — may be fixed by session change. 30min

## Sprint 3: Strategic (8-16h)

- [ ] **PR #38** (mcp-go upgrade) — reimplement, migrate all handlers. 6-10h
- [ ] **PR #82** (refactoring tools) — verify endpoints on SAP first. 2-4h + 2h
- [ ] **PR #86** (intelligence layer) — close, doesn't compile. 0h
- [ ] Close **#21** (streaming HTTP) after #38
- [ ] **#74** (CDS DDLX) — add endpoint if exists. 1-2h

## Permanent Backlog

- #55 — RunReport APC spool (architecture)
- #27 — Object types (ongoing)
- #45, #46 — Sync script (low priority tooling)
- #2 — GUI debugger (parked)

## Total estimate: 15-26h across 3 sprints
