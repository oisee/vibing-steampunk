# Seed: Next Agent Session

**Date:** 2026-04-09
**Previous wisdom files:**
- `contexts/2026-04-08-wisdom-analysis-sprint.md` — boundaries, effects, health reports, exports
- `contexts/2026-04-08-wisdom-execution-semantics-cache-and-context-compression.md` — architecture: topology vs contracts vs effects
- `contexts/2026-04-09-wisdom-install-bugs-and-session-wrap.md` — install handler bug, lock issues

---

## Project State

**vsp** — Go MCP server + CLI for SAP ABAP Development Tools.

```
Stars:      257 (0x101)
Release:    v2.38.1
Tools:      147 expert / 100 focused
Tests:      835+ across 15/15 packages
mcp-go:     v0.47 (Streamable HTTP)
Open PRs:   0
Open Issues: 8 (backlog)
Branch:     main
```

**Analysis surface:** slim (dead code), health (tests/ATC/boundaries/staleness), api-surface (Clean Core), boundaries (directional crossings), changelog (transport history), changes (CR correlation), effects (side effect/LUW detection).

**Graph exports:** mermaid, HTML, DOT (Graphviz), PlantUML, GraphML (Gephi/yEd), JSON, MD.

**Parser extracts:** CALL FUNCTION, SUBMIT, PERFORM, CREATE OBJECT, SELECT, TYPE REF TO, INTERFACES, INHERITING FROM, RAISE EXCEPTION, method calls, CALL TRANSACTION, LEAVE TO TRANSACTION, CALL TRANSFORMATION + dynamic variants.

**Cache config wired:** `"cache": true` in .vsp.json per system, `VSP_CACHE=true` env. Path: `.vsp-cache/<system>.db`. Not yet wired to actual cache reads/writes.

---

## Immediate Priorities

### P0: Install Handler Fix (Stashed)
**Bug:** `vsp install zadt-vsp` says success but doesn't create objects on trial systems.
**Root cause:** Handler checks `err != nil` but not `result.Success`.
**Fix:** In `git stash`. Needs Codex review before applying.
**Report:** `reports/2026-04-09-001-install-and-lock-bug-report.md`
**Files:** `cmd/vsp/devops.go:2088-2190`, `internal/mcp/handlers_install.go:397-441`

### P1: Cache Wiring (M2 — TADIR + Scope)
**Plan:** `reports/2026-04-08-004-sqlite-cache-wiring-plan.md`
**Status:** Config done (M1). Next: wire `resolvePackagesCLI` and `AcquirePackageScope` to read/write SQLite.
**Impact:** Eliminates 100+ SQL queries per analysis run.

### P2: Wire Effects Into CLI
`ExtractEffects()` works (14 tests) but no user-facing command yet.
Options: `--effects` flag on health, standalone `vsp effects CLAS ZCL_FOO`, or annotate boundaries.

### P3: Unit Test XML Parent Names
Health report shows `LCL_` without full name. `ParentName` from XML `program` element may not populate on all SAP versions. Need `VSP_DEBUG_XML=1` capture to verify.

---

## Architecture Decisions in Effect

### Three Layers (ADR-004)
1. **Topology** — nodes + edges in `pkg/graph/`
2. **Contracts** — `MethodSignature`, `CodeUnitContract` (partially implemented)
3. **Effects/Semantics** — `EffectInfo` with LUW classification (implemented, not wired to UI)

Transitive behavior = derived report, not first-class topology.

### Cache (ADR-005)
- SQLite opt-in, per-system file
- Source hash as primary cache key
- Analyzer version for invalidation
- Local facts first, transitive rollups later
- 6 milestones in steering plan

### Boundary Directions
- UPWARD/COMMON = OK, SIBLING/DOWNWARD/COMMON_DOWN = BAD, EXTERNAL = WARN
- `_00` suffix = common package convention
- Test packages exempt from sibling violations
- Circular detection via bidirectional pair tracking

---

## What To Avoid

- Do not commit install fix without Codex governance
- Do not cache transitive rollups yet — local facts first
- Do not assume `GetPackage` returns subpackages — use `AcquirePackageScope`
- Do not assume `RunUnitTests` covers subpackages — expand and run per-package
- Do not use `RunQuery(ctx, query, 0)` — defaults to 100 rows
- Do not use `OR` + `LIKE` in ADT freestyle SQL
- Do not use `request.Params.Arguments` — use `request.GetArguments()`
- Do not put acquisition logic inline — use `acquire.go` helpers

---

## Key Reports & ADRs

| Document | Purpose |
|----------|---------|
| `reports/2026-04-09-001-install-and-lock-bug-report.md` | Install silent failure + lock errors |
| `reports/2026-04-08-004-sqlite-cache-wiring-plan.md` | Cache 6-milestone steering plan |
| `reports/2026-04-08-002-code-unit-contracts-and-purity-proposal.md` | Contracts + effects 5-phase plan |
| `reports/2026-04-08-001-boundary-crossing-direction-proposal.md` | Crossing direction ADR |
| `docs/adr/004-execution-semantics-and-effect-profiles.md` | Topology vs contracts vs effects |
| `docs/adr/005-opt-in-sqlite-analysis-cache.md` | Cache architecture |

---

## SAP Systems

| System | URL | Notes |
|--------|-----|-------|
| a4h-110-adt | 192.168.8.110:50000 | Test system, user AVINOGRADOVA, `$ZHIRTEST` hierarchy |
| a4h (desude) | a4h.desude.su:50000 | Alice's dev system, `$ZLLM` package |
| muabap | muabap.ydns.eu | Marcello's trial (SAP 2023), user oisee — install bug reproduction target |

## Smoke Tests

```bash
vsp -s a4h-110-adt search "ZCL_HIRT*" --type CLAS --max 5
vsp -s a4h-110-adt health --package '$ZHIRTEST'
vsp -s a4h-110-adt boundaries '$ZHIRTEST'
vsp -s a4h-110-adt changelog '$ZHIRTEST' --include-subpackages --top 5
```
