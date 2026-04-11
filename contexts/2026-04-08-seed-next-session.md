# Seed: Next Agent Session

**Date:** 2026-04-08
**Wisdom file:** `contexts/2026-04-08-wisdom-analysis-sprint.md`

---

## Project Context

**vsp** is a Go MCP server + CLI for SAP ABAP Development Tools. 147 expert tools, 100 focused. mcp-go v0.47. Analysis suite: slim (dead code), health (tests/ATC/boundaries/staleness), api-surface (Clean Core), boundaries (directional crossings), changelog (transport history), changes (CR correlation), effects (side effect/LUW detection). 7 graph export formats. Test suite: 15/15 packages, 835+ tests.

## Read First

1. `CLAUDE.md` — project structure, build commands, conventions
2. `contexts/2026-04-08-wisdom-analysis-sprint.md` — what was done, gotchas, architecture decisions
3. `reports/2026-04-08-004-sqlite-cache-wiring-plan.md` — cache steering plan (M1 config done, M2-M6 pending)

## Current State

```
Open PRs:  0
Open Issues: 8 (all backlog)
Branch: main
Latest release: v2.38.1
Stars: 257 (0x101)
SAP test system: a4h-110-adt (192.168.8.110:50000, user AVINOGRADOVA)
Test hierarchy: $ZHIRTEST_00/001/010/101 with 7 objects
```

## What To Do Next

### Priority 1: Cache Wiring (M2 — TADIR + Scope)
Config is done (M1). Next: wire `resolvePackagesCLI` and `AcquirePackageScope` to read/write SQLite when `params.Cache` is true. Use existing `pkg/cache/sqlite.go` infrastructure. Schema additions in steering plan.

### Priority 2: Wire Effects Into CLI
`ExtractEffects()` works (14 tests) but has no user-facing command. Options:
- Add `--effects` flag to `vsp health` that shows LUW/purity per method
- Add standalone `vsp effects CLAS ZCL_FOO` command
- Annotate `vsp boundaries` entries with effect badges

### Priority 3: Unit Test XML Debug
Health report shows `LCL_` with no full name for local test classes. `ParentName` from XML `program` element may not be populated on all SAP versions. Need `VSP_DEBUG_XML=1` capture on a4h to check actual XML structure and fix parser if needed.

### Priority 4: Context Frame Builder (Phase 5 from contracts ADR)
Build `BuildContextFrame(graph, nodeID, depth)` that composes:
- Method contract + body
- Sibling contracts (no bodies)
- Caller examples
- Callee subgraph with depth + effects summary
Integration with `vsp context` command.

### Standing Tasks
- Issues #88/#90: fixes shipped, awaiting reporter validation
- Slim V2.1 graph reachability: design ready, not prioritized
- Refactoring tools: real ADT endpoints exist, need implementation from `abap-adt-api`

## What To Avoid

- Do not start new analysis surfaces without reviewing existing ADRs (004, 005)
- Do not cache transitive rollup results yet — local facts first
- Do not use `OR` + `LIKE` in ADT freestyle SQL (use per-object queries)
- Do not use `request.Params.Arguments` — use `request.GetArguments()` (mcp-go v0.47)
- Do not put package acquisition logic inline — use `cmd/vsp/acquire.go` helpers
- Do not assume `GetPackage` returns subpackages — use `AcquirePackageScope`
- Do not assume `RunUnitTests` covers subpackages — expand hierarchy and run per-package
- Do not use `RunQuery(ctx, query, 0)` — maxRows 0 defaults to 100, specify explicitly

## SAP Smoke Test Commands

```bash
./build/vsp -s a4h-110-adt search "ZCL_HIRT*" --type CLAS --max 5
./build/vsp -s a4h-110-adt query T000 --top 3
./build/vsp -s a4h-110-adt slim '$ZHIRTEST' --level methods
./build/vsp -s a4h-110-adt health --package '$ZHIRTEST'
./build/vsp -s a4h-110-adt boundaries '$ZHIRTEST'
./build/vsp -s a4h-110-adt changelog '$ZHIRTEST' --include-subpackages --top 5
./build/vsp -s a4h-110-adt api-surface '$ZHIRTEST' --include-subpackages
```
