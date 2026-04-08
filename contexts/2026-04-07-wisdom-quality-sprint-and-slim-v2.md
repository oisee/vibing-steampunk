# Session Wisdom: Quality Sprint + Slim V2 + Shared Acquisition

**Date:** 2026-04-07
**Session scope:** Community PR backlog zero, Slim V2 usable slice, health/api-surface validation, shared package acquisition refactor, AnalyzeABAPCode v2 integration

---

## What Was Done

### Community PR Backlog → Zero
All 14 community PRs processed (was 14 open → 0 open):
- 3 merged as-is: #80 (copilot docs), #44 (Windows quickstart), #83 (version history)
- 4 cherry-picked with fixes: #85 (CDS tools), #84 (testing), #41 (gCTS), #42 (i18n)
- 2 reimplemented: #53 (Clean Core — PathEscape bug), #37 (pagination)
- 2 reimplemented better: #79 (session fix), #38 (mcp-go v0.47)
- 1 cherry-picked: #77 (Browser SSO)
- 2 closed with analysis: #82 (wrong endpoints), #86 (doesn't compile)
- 1 cherry-picked with 3 fixes: #89 (AnalyzeABAPCode v2)

### Slim V2 — Phases 1-3 + 5 Done
- `pkg/graph/scope.go` — TDEVC hierarchy resolution + prefix fallback
- `pkg/graph/queries_slim.go` — ComputeSlim with DEAD/INTERNAL_ONLY/LIVE verdicts
- `cmd/vsp/cli_extra.go` — `vsp slim` with `--level objects|methods|full`
- `pkg/graph/slim_integration_test.go` — 4 integration tests
- `pkg/graph/scope_test.go` — 6 scope tests including prefix fallback

### Shared Package Acquisition Helper
- `cmd/vsp/acquire.go` — AcquirePackageScope, AcquirePackageObjects, AcquireReverseRefs, ScopeToWhere, IsSourceBearing
- Consumed by: slim (cli_extra.go), api-surface (api_surface.go)

### mcp-go v0.17 → v0.47
- 400+ `request.Params.Arguments` → `request.GetArguments()` migrations across 33 handlers
- `--transport http --http-addr :8080` for Streamable HTTP
- `cmd/vsp/main.go` — PersistentPreRunE for verbose, transport selection

### Other Fixes
- Stateless session default (`pkg/adt/config.go:186`)
- InstallZADTVSP resilience (`internal/mcp/handlers_install.go`)
- Graph namespace fix (`cmd/vsp/cli_extra.go` — url.PathEscape)
- `--verbose` as PersistentFlag (`cmd/vsp/main.go`)
- All test failures fixed: pkg/dsl (fmt.Errorf), pkg/jseval (quote escaping)

---

## What's Not Done and Why

### Slim V2 Phase 4 — Attribute-level dead code
Skipped per Codex guidance: "not nearly free." WBCROSSGT attribute tracking (OTYPE='DA'/'OA') is unreliable. Deferred to V2.1.

### Slim V2.1 — Graph reachability (unreachable clusters)
Design exists (report 2026-04-07-003). Needs iterative forward-traverse from entry points. Current V2 flags INTERNAL_ONLY as warning, not deletion recommendation.

### Health staleness for local packages
E070 fallback added but returns UNKNOWN for $-prefixed packages (correct — no transports). Could use TADIR CREATED_ON but Codex explicitly said "do not anchor on dubious TADIR last-changed."

### Forward ref sharing (deps ↔ api-surface)
Both use `queryObjectRefs` from cli_deps.go but direction differs (slim = reverse WHERE NAME LIKE, deps/api-surface = forward WHERE INCLUDE LIKE). Different enough to not force-share.

---

## Gotchas and Non-Obvious Decisions

### ADT Freestyle SQL does NOT support OR + LIKE
`SELECT ... WHERE (NAME LIKE 'A%' OR NAME LIKE 'B%')` → 400 error. Must use per-object queries. This was the root cause of slim returning 0 refs initially. Commit f17cf04.

### TDEVC PARENTCL may be empty
Test packages created via CreatePackage API don't get PARENTCL set. `ResolvePackageScope` falls back to prefix matching when root package is not found in TDEVC hierarchy (scope.Method = "prefix").

### Cobra PersistentFlags vs Flags
`rootCmd.Flags()` = invisible to subcommands. `rootCmd.PersistentFlags()` = inherited. Also need `PersistentPreRunE` to read env vars via viper for CLI subcommands since `resolveConfig()` only runs in MCP server mode.

### mcp-go v0.47 — Arguments is now `any`
`request.Params.Arguments` changed from `map[string]any` to `any`. Must use `request.GetArguments()` which returns `map[string]any`. Also `CallToolParams` struct changed — `Meta` is now `*mcp.Meta` not inline struct.

### PR #82 endpoints are REAL but implementation is WRONG
ADT refactoring endpoints exist (`/sap/bc/adt/refactorings`, `/sap/bc/adt/quickfixes/evaluation`). But PR used wrong URLs (`/refactoring/rename`), wrong params (`?method=` vs `?step=`+`?rel=`), and fabricated XML formats. Reference: `abap-adt-api/src/api/refactor.ts`.

### PR #89 CatchCxRootRule — CX_SY_* are NOT broad exceptions
CX_SY_CONVERSION_ERROR, CX_SY_ZERODIVIDE etc. are specific, valid exception classes. Only CX_ROOT, CX_STATIC_CHECK, CX_DYNAMIC_CHECK, CX_NO_CHECK are genuinely broad.

### PR #89 HardcodedCredentialsRule — "token" matches lexer variables
Bare "token" in credential name list flags lv_next_token, ls_token_data. Narrowed to auth_token, access_token, bearer_token, refresh_token, api_token.

---

## Architectural Insights

### acquire.go pattern
The smallest useful shared helper is scope + TADIR + reverse refs. Forward refs (deps/api-surface) use a different query shape and should stay separate. Health uses GetPackage() API (not TADIR SQL) — genuinely different path.

### Result struct discipline
All graph queries (slim, health, api-surface) follow canonical result pattern:
- Scope + Summary + domain-specific entries
- JSON marshallable with `json` tags
- Text formatter separate from computation
- ComputeX functions are pure (no I/O) — callers provide data

### Session handling
Stateless default is correct for 95% of operations. Stateful needed only for debugger and lock→write→unlock sequences. Per-request Stateful flag added in commit 27f4d7c for issue #88.

---

## Key Artifacts

| Path | Description |
|------|-------------|
| `cmd/vsp/acquire.go` | Shared package scope/object/ref acquisition |
| `cmd/vsp/cli_extra.go` | slim, graph, query, examples CLI commands |
| `cmd/vsp/api_surface.go` | api-surface CLI command |
| `cmd/vsp/devops.go` | health, test, atc, deploy, transport, install CLI |
| `cmd/vsp/main.go` | PersistentFlags, PersistentPreRunE, transport selection |
| `pkg/graph/scope.go` | TDEVC hierarchy + prefix fallback |
| `pkg/graph/queries_slim.go` | ComputeSlim engine |
| `pkg/graph/queries_health.go` | HealthResult types + ComputeHealthSummary |
| `pkg/graph/queries_apisurface.go` | ComputeAPISurface engine |
| `pkg/graph/slim_integration_test.go` | $ZHIRTEST hierarchy test scenarios |
| `pkg/adt/config.go:186` | SessionStateless default |
| `pkg/adt/codeanalysis.go` | AnalyzeABAPSource orchestrator (13 rules) |
| `pkg/abaplint/rules.go` | 13 lint rules (8 original + 5 new) |
| `internal/mcp/handlers_codeanalysis.go` | AnalyzeABAPCode MCP tool |
| `reports/2026-04-07-003-slim-v2-hierarchical-design.md` | Slim V2 design doc |
| `reports/2026-04-06-006-health-mvp-spec.md` | Health MVP spec |
| `reports/2026-04-06-007-api-surface-mvp-spec.md` | API Surface MVP spec |

### External Links
- Release v2.36.0: https://github.com/oisee/vibing-steampunk/releases/tag/v2.36.0
- Issues #88, #90: fixes shipped (27f4d7c), awaiting reporter validation
- PR #89: cherry-picked with 3 fixes (commit 8623acd)
