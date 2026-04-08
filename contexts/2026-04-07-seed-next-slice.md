# Seed: Next Agent Session

**Date:** 2026-04-07
**Wisdom file:** `contexts/2026-04-07-wisdom-quality-sprint-and-slim-v2.md`

---

## Project Context

**vsp** is a Go MCP server + CLI for SAP ABAP Development Tools. 147 expert tools, 100 focused. mcp-go v0.47. Three analysis slices (slim, health, api-surface) are usable and SAP-validated. All community PRs processed (0 open). 8 open issues (all backlog). Test suite: 15/15 packages, 821+ tests.

## Read First

1. `CLAUDE.md` — project structure, build commands, conventions
2. `contexts/2026-04-07-wisdom-quality-sprint-and-slim-v2.md` — what was just done, gotchas, architecture
3. `reports/2026-04-07-003-slim-v2-hierarchical-design.md` — Slim V2 design (phases 1-3 done, 4 deferred)

## Current State

```
Open PRs:  0
Open Issues: 8 (all backlog: #74 CDS DDLX, #55 RunReport, #27 object types, #45/#46 sync, #2 debugger, #88/#90 awaiting reporter validation)
Branch: main
Latest commits: e549006 (acquire.go refactor), 8623acd (AnalyzeABAPCode), 9ae10f3 (health E070)
SAP test system: a4h-110-adt (192.168.8.110:50000, user AVINOGRADOVA)
Test hierarchy: $ZHIRTEST_00/001/010/101 with 7 objects for slim validation
```

## What To Do Next (per Codex directive)

Choose between these two (ask Codex if unclear):

### Option A: Changelog / release notes automation
Aggregate commits since last tag into structured release notes. Low risk, high value for release cadence.

### Option B: Upgrade-check MVP
Build on api-surface output: check which standard APIs have release state != RELEASED. Uses existing `GetAPIReleaseState` + `ComputeAPISurface`. Thin slice.

### Standing tasks
- Issues #88/#90: fixes shipped, need reporter validation (not our action)
- Slim V2.1 (graph reachability): design ready, not prioritized yet
- Refactoring tools: real endpoints exist but need reimplementation from `abap-adt-api/src/api/refactor.ts`

## What To Avoid

- Do not start new surfaces without Codex approval
- Do not expand health/slim/api-surface unless clear bug
- Do not merge PRs on enthusiasm — strict review first
- Do not use `OR` + `LIKE` in ADT freestyle SQL (use per-object queries)
- Do not use `request.Params.Arguments` — use `request.GetArguments()` (mcp-go v0.47)
- Do not put package acquisition logic inline — use `cmd/vsp/acquire.go` helpers

## SAP Smoke Test Commands

```bash
./build/vsp -s a4h-110-adt search "ZCL_HIRT*" --type CLAS --max 5
./build/vsp -s a4h-110-adt query T000 --top 3
./build/vsp -s a4h-110-adt slim '$ZHIRTEST' --level methods
./build/vsp -s a4h-110-adt health --package '$ZHIRTEST' --fast
./build/vsp -s a4h-110-adt api-surface '$ZHIRTEST' --include-subpackages
```
