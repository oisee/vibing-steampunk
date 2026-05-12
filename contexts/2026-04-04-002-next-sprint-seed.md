# Next Sprint Seed — Quality Sprint Phase 2

**Date:** 2026-04-04
**Seed for:** Next session(s) after quality sprint phase 1
**Context:** reports/2026-04-04-001-quality-sprint-plan.md

---

## Status at End of Phase 1

### Released: v2.33.1
- Stateless session default (fixes #81)
- Installer resilience for 7.40 (fixes #87, #75)
- Graph namespace fix (fixes #76)
- --verbose for all CLI subcommands
- Table pagination (offset, columns_only)
- Version history tools (3 tools from PR #83)
- CDS impact/element tools (2 tools from PR #85)
- CodeCoverage + CheckRun (2 tools from PR #84)
- All 13 test packages PASS (was 11/13)

### Open PRs — Remaining Work

| PR | Status | Next Action |
|---:|--------|-------------|
| **#38** | Open, conflicts | mcp-go v0.43.2 upgrade — do as separate track, 37 files |
| **#77** | Open, conflicts | Browser SSO — split into keepalive + browser-auth |
| **#53** | Open, conflicts | Clean Core — reimplement (PathEscape bug) |
| **#41** | Open, conflicts | gCTS — merge after adding tests, migrate to GetArguments() |
| **#42** | Open, conflicts | i18n — merge after adding tests, review http.go buildURL change |
| **#82** | Open, conflicts | Refactoring tools — verify endpoints on real SAP first |
| **#86** | Open, conflicts | Intelligence Layer — does NOT compile, needs full rewrite on pkg/abaplint |

### Open Issues — Remaining

| Issue | Priority | Action |
|------:|----------|--------|
| **#26** | P1 | GetTransport — verify after session fix |
| **#43** | Low | Missing commands — documentation/support gap |
| **#55** | Backlog | RunReport APC spool — needs architecture change |
| **#56** | Low | Unable to create program — retry with v2.33.1 |
| **#27** | Backlog | Object types — ongoing (158 types in abapGit export) |
| **#74** | Backlog | CDS DDLX — partially addressed by PR #85 |
| **#45/#46** | Low | Sync script improvements |
| **#2** | Parked | GUI debugger |

---

## Concrete TODO for Next Session

### Track A: Continue Stability (2-3h)

1. **ADT Capability Probe** (`vsp probe`) — test all endpoints, report availability
   - Triggered by user feedback (Birzhan, Александр)
   - Critical for systems with incomplete ADT support
   - Design: probe key endpoints on first connect, cache results, add `vsp probe` CLI command

2. **Verify #26** (GetTransport) — may be fixed by stateless session change

3. **Close #56** — ask reporter to retry with v2.33.1

### Track B: PR Intake (4-8h)

1. **PR #53 (Clean Core)** — reimplement: fix PathEscape, parse XML, fix typo. 1-2h
2. **PR #41 (gCTS)** — add unit tests, cherry-pick. 2-3h
3. **PR #42 (i18n)** — review buildURL change carefully, add tests. 2-3h

### Track C: Test Coverage (4-6h)

Priority order from coverage analysis:
1. Transport safety tests (mock configs, blacklist/whitelist)
2. CRUD operation tests (mock HTTP, error paths)
3. MCP handler routing tests
4. Lua smoke test script (`examples/tests/smoke.lua`)
5. DSL CI workflow (`examples/workflows/ci-smoke.yaml`)

### Track D: Strategic (separate sprint)

1. **mcp-go upgrade** (#38) — blocks HTTP Streamable. 4-8h.
2. **Browser SSO** (#77) — after mcp-go upgrade. 4-6h.
3. **GitHub Actions CI** — automated `go test ./...` on PRs. 1-2h.

---

## Key Reminders for Next Session

- `go test ./...` should be GREEN (13/13) — run at START to verify baseline
- v2.33.1 tag is on commit 8429e28 (but more commits after — retag or bump to v2.34.0)
- blicksten has 2 remaining PRs (#82, #86) — don't merge without SAP verification
- PR #86 needs "do not merge" label — it doesn't compile
- Community is active: 8 contributors, check for new PRs/issues
- `.vsp.json` has 3 configured systems: devsys2, devsys, x15-abap

## SAP Test Commands (quick smoke test)

```bash
# Verify connection
./build/vsp -s devsys-adt search "ZCL_VSP*" --type CLAS --max 5

# Verify query (stateless session)
./build/vsp -s devsys-adt query T000 --top 3

# Verify new tools
./build/vsp -s devsys-adt install zadt-vsp --dry-run

# Verbose for debugging
./build/vsp -s devsys-adt -v query T100 --top 1
```
