# CLAUDE.md

**vsp** — Go-native MCP server and CLI for SAP ABAP Development Tools (ADT).

> **Doc intent:** CLAUDE.md = dev context. README.md = user onboarding. reports/ = research/history. contexts/ = session handoff. agenda/ = what is open and what was decided (`AGENDA.md` is the living board; `YYYY-MM-DD-NNN-topic.md` are dated analyses).

---

## Current Priorities

### 1. Graph Engine (`pkg/graph/`) — Feature-complete
This section understated the package for four months; corrected 2026-08-24. It
then went stale again the other way — it listed D010INC and `ExtractEffects` as
pending for a week after both shipped on 2026-08-25. Corrected 2026-09-02.
- Done: core types, parser dep extraction, boundary analyzer, **SQL adapters
  (`builder_sql.go` — CROSS + WBCROSSGT + WBCROSSGTX long names)**,
  `builder_transport.go`, `builder_config.go`, and the `queries_*.go` surface
  behind slim / health / impact / api-surface / rename / examples.
  51 files, 218 test functions.
- Done 2026-08-25: **D010INC**, the compile-time *load* graph — the one novel
  source in the original design — is `builder_loads.go`, reachable as
  `vsp loads`. And `graph.ExtractEffects` (side effects / LUW) has callers at
  last: `cmd/vsp/effects.go:82` and `internal/mcp/handlers_effects.go:100`.
  Both were described here as unwired for the four months they sat unused.
- Pending: unify `cli_deps.go` + `cli_extra.go` + `ctxcomp/analyzer.go`. Two of
  the three now import `pkg/graph` (`cli_extra.go:16`, `analyzer.go:9`); only
  `cli_deps.go` still carries its own extraction.
- Design: [002](reports/2026-04-05-002-graph-engine-design.md), [003](reports/2026-04-05-003-graph-engine-alignment-for-claude.md)

### 2. Debugger — Phase 1 shipped, #2 closed 2026-09-02
This line advertised "MCP debug sessions → DAP → Web UI" and pointed at #2 after
that issue was closed, so it promised two phases nobody was tracking.
- Built: `pkg/adt/debugger.go` (1833 lines), a session that survives across MCP
  tool calls (`internal/mcp/handlers_debug_session.go`), six registered tools,
  and ADT-native AMDP routing whose breakpoints actually fire.
- Not built: the DAP shim and the Web UI. No `DebugAdapter` code, no `web/`.
  Now tracked as #184, together with the question of whether vsp should ship a
  UI at all or stop at DAP and let editors be the front end.
- Design: [001](reports/2026-04-05-001-gui-debugger-design.md)

### 3. Open Issues
- **#91** The 423 lock-handle class — the live one, and this entry was wrong
  twice. `22517d4` did not close it: a third-party release bisect names that
  commit as the start of a regression, and its `ModificationSupport` guard was
  itself removed by `9b98997`. #88, #92, #98, #110 are closed as duplicates of
  #91 (2026-09-01), and #132 with them once PR #145 landed the transport reuse
  its second leg needed. Confirmed fixed on a live S/4HANA 758 on 2026-09-02
  with a control: `main` creates and activates in `$TMP` where v2.54.0 fails
  the identical operation on the same host minutes apart.
  Cause: `SessionType` defaults to stateless (`config.go:198`, `d84db03`) and
  `http.go:502` stamps every unflagged request `stateless`, so any hop between
  LOCK and the write retires the ICM context and kills the handle. The fix on
  `fix/91-session-affinity` closes the package-lookup hop, the CSRF probe, and
  two mutations that were themselves stateless. The keep-alive ticker is fixed
  too (#168: default 0, and a tick inside a lock window is skipped). What is
  left is the MCP cross-tool-call window — #169, and #181 is its deterministic
  face: with `--allowed-packages` set every write failed this way in every
  release since v2.42.0. PR #183 makes the lock handle optional so the window
  cannot span a model turn.
- **#166** A failed mutation strands the SAP-side ENQUEUE — split out of #92
  so it survives that closure. Users clear these by hand in SM12.
- **#55** RunReport in APC — *not* an architectural limit, which this line
  claimed for months. `34eb727` ("$ZADT_VSP sync", 2026-02-06) replaced a
  working XBP background-job + spool implementation in
  `src/zcl_vsp_report_service.clas.abap` with a bare `SUBMIT ... AND RETURN`,
  deleting the `getJobStatus`/`getSpoolOutput` actions the Go client still
  speaks. It is a regression with a known good parent commit. #113 was the
  same defect and is closed against this one.
- ~~**#46, #45** Sync script~~ — closed 2026-09-01. `scripts/sync-upstream.sh`
  has never existed here (`git log --all --diff-filter=A` finds it in none of
  the 913 commits); both issues were filed from a downstream fork's workflow.
  This line advertised work that was not ours to do.

---

## Build & Test

```bash
go build -o vsp ./cmd/vsp              # Build
go test ./...                           # Unit tests
go test -tags=integration -v ./pkg/adt/ # Integration (needs SAP)
make build-all                          # 9 platforms
```

Key flags: `--mode focused|expert|hyperfocused`, `--read-only`, `--allowed-packages "Z*"`, `--disabled-groups 5THD`

---

## Codebase

```
cmd/vsp/              CLI entry + 55 commands
internal/mcp/
  handlers_*.go       Domain handlers (read, edit, debug, graph, ...)
  tools_register.go   Registration + mode logic
  tools_focused.go    Focused mode whitelist
  handlers_universal.go  Hyperfocused single-tool (SAP)
pkg/
  adt/                ADT client (HTTP, CSRF, sessions, all SAP ops)
  graph/              Dependency graph engine (in progress)
  datacluster/        EXPORT data cluster parser (BALDAT, INDX, STXL): descriptors, rows, typed values
  sapcompress/        SAP LZH (= DEFLATE + prefix, via compress/flate) and LZC (compress(1)) decoders
  ctxcomp/            Context compression (dep resolution for read)
  abaplint/           ABAP lexer + parser (95 statement patterns; 13 lint rules, 8 on by default)
  dsl/                Fluent API, YAML workflows, batch ops
  cache/              In-memory + SQLite
  scripting/          Lua engine
  llvm2abap/          LLVM→ABAP (research)
  wasmcomp/           WASM→ABAP (research)
```

| Task | Files |
|------|-------|
| Add MCP tool | `tools_register.go` + `handlers_*.go` + `tools_focused.go` |
| Add ADT operation | `pkg/adt/client.go`, `crud.go`, `devtools.go`, `codeintel.go` |
| Touch SSO auth | `pkg/adt/sso*.go`, `cmd/vsp-sso/`, `cmd/vsp/sso.go` |
| Add graph feature | `pkg/graph/` |
| Add lint rule | `pkg/abaplint/rules.go` |
| Add integration test | `pkg/adt/integration_test.go` |
| Fix MCP/docs/config | `README.md`, `docs/cli-agents/*`, `handlers_universal.go` |

---

## Adding a New MCP Tool

1. Handler in `handlers_*.go`:
```go
func (s *Server) handleX(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    name, _ := req.GetArguments()["name"].(string)
    result, err := s.adtClient.Method(ctx, name)
    if err != nil { return newToolResultError(err.Error()), nil }
    return mcp.NewToolResultText(format(result)), nil
}
```
2. Register in `tools_register.go` with `shouldRegister("X")`
3. Route in `handlers_analysis.go` (or appropriate router)
4. Add to `tools_focused.go` if needed in focused mode

---

## Provoking a failure on a live system

Testing the unhappy path means making something fail, and the cheapest
way to make SAP say no is the one that costs the most. A wrong password
for a **real** user counts against `login/fails_to_user_lock`, and one
sweep is dozens of requests — `vsp compat` locked the developer account
for a day this way, after which the *correct* password also returns 401.

Reach for these in order. The first three touch no credential at all:

1. **A client-side refusal** — `SAP_BLOCK_FREE_SQL=1`,
   `adt.WithBlockFreeSQL()`, `--disallowed-ops`. The request is never
   sent, and the error is the one a safety-blocked user would see.
2. **An `httptest` server returning 403.** The authorisation case with
   no SAP anywhere near it, and it runs in CI.
3. **An object or package that does not exist** — a real 404 from a
   real session.
4. **An unresolvable hostname** — fails before any credential leaves the
   process.
5. **A user that does not exist**, if a genuine 401 is unavoidable.
   Nothing can be locked, because there is nothing to lock. It still
   writes to the security audit log, so keep it to a few requests rather
   than a sweep.

Never a real user with a wrong password. Not once, not "just to see":
the cost is not a failed request, it is the system for everyone until
the lock clears — midnight on a stock A4H, `SU01` otherwise.

## Common Issues

1. **CSRF errors** — auto-refreshed in `http.go`
2. **Lock conflicts** — edit handler does auto lock/unlock
3. **Session issues** — some CRUD/debugger flows are session-sensitive; verify stateful/stateless before changing transport or auth logic
4. **Auth** — use basic OR cookies, not both. `HasBasicAuth()` disables `ReauthFunc`, so a stray `SAP_USER`/`SAP_PASSWORD` alongside SSO silently kills auto-refresh
5. **Expired SSO sessions do not return 401** — ICF forwards to the IdP and a logon page arrives under a 200. Detection is by origin and by a missing CSRF token (`http.go`), not by status code
6. **ZADT_VSP** — WebSocket debug/RFC/RunReport require it installed on SAP

## Security

Never commit `.env`, `cookies.txt`, `.mcp.json`, or local agent/MCP config files (all in `.gitignore`).

### Sanitize policy for tracked docs, tests, and examples

The public repo must not contain concrete identifiers that tie code or
docs to a live SAP system, a real user, or a customer's ABAP namespace.
Anything that does belongs under `.local/` (gitignored) and never in
`contexts/`, `reports/`, `docs/`, or any tracked test fixture.

**Never in tracked files:**
- Real SAP usernames — use `TESTUSER`
- Real hostnames or IPs — use `dev.example.local`, `prodsys-a.example`, `trialsys.example`
- System aliases that name a live box — use `devsys`, `devsys-adt`, `prodsys-a`, `prodsys-b`
- Live transport numbers (`DEVK[0-9]+`, `R[0-9]{2}K[0-9]+`, `D[0-9]{2}K[0-9]+`) — use `TR-EXAMPLE`
- Live change request IDs — use `CR-EXAMPLE`
- Customer ABAP namespaces from real projects — use synthetic `ZDEMO_*`, `ZCL_DEMO_*`, `ZIF_DEMO_*`, `$ZDEMO`
- Customer transport attribute names — use `Z_CR_ATTR`
- Real passwords, API keys, bearer tokens (obvious, but stated)
- Real person names tied to private systems (OSS attribution for upstream libraries is fine — "user X on private host Y" is not)

**Always OK in tracked files:**
- `$ZHIRTEST*`, `ZCL_HIRT*`, `ZCUSTOM_DEVELOPMENT` — pre-agreed synthetic fixtures
- Public GitHub handles that are already in the Go module path
- Upstream OSS attribution for library authors

**Operational scratch goes under `.local/`** — session notes, live CR
dumps, bug repros with real identifiers, debugging transcripts. The
`.local/` dir is gitignored. If you need to reference it from a
tracked doc, redact first.

**Before every commit that touches `reports/`, `contexts/`, `docs/`,
or test fixtures:** scan the staged diff for the identifier families
above. The detection signature (concrete literal list of past-leaked
strings) lives at `.local/scripts/check-identifiers.sh` and is
gitignored on purpose — the signature itself would otherwise be the
leak it is trying to prevent. Structural patterns safe to commit:

```bash
git diff --cached | grep -nE \
  '\b[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\b|' \
  '\b[A-Z][0-9]{2}K[0-9]{6}\b|' \
  '\bDEVK[0-9]{6,}\b'
```

That catches IPv4 literals and SAP transport IDs without hardcoding
a specific customer's values. Pair it with the private signature
file for the names-based families (usernames, hostnames, ABAP object
prefixes). If either matches, move the content under `.local/` and
replace the tracked version with a synthetic placeholder. Rule of
thumb: "would a stranger reading this file be able to identify the
customer, the system, or a live account?" If yes, redact.

## Conventions

Reports: `reports/YYYY-MM-DD-NNN-title.md`.

**SAP object names.** After the kind prefix comes a *domain* token, then the
name. Ours is `VSP`. There is never an underscore straight after `Z`.

| Kind | Form | Ours |
|------|------|------|
| Class | `ZCL_<domain>_<name>` | `ZCL_VSP_GIT_SERVICE`, `ZCL_VSP_APC_HANDLER` |
| Interface | `ZIF_<domain>_<name>` | `ZIF_VSP_SERVICE` |
| Program | `Z<domain>_<name>` | `ZVSP_ENQUEUE_RESET` |
| Function group | `Z<domain>_<name>` | `ZVSP_GIT` |
| Function module | `Z<domain>_<name>` | `ZVSP_GIT_CALL` |
| Message class | `Z<domain>_<name>` | `ZVSP_GIT` |
| Package | `$ZADT_VSP` | |

A numeric bucket may sit between domain and name — `ZCL_VSP_00_AMDP_TEST` — and
we use it only for test fixtures. Landscapes that carry it everywhere use it to
mirror package structure; we do not.

Names predating this — `ZADT_CL_TADIR_MOVE`, `ZCL_ADT_00_AMDP_TEST` — are the
older `ADT` domain and put `CL` in the wrong place. Both have `ZCL_VSP_*`
successors; do not add more.

---

## Areas Requiring Care

| Area | Risk | Notes |
|------|------|-------|
| `pkg/graph/` | Large, moving | This row said "only parser adapter; SQL/ADT adapters pending" for four months while contradicting this file's own Current Priorities section. `builder_sql.go`, `builder_transport.go`, `builder_config.go` and `builder_loads.go` all exist with tests |
| `handlers_debugger.go` | ADT over a held session | Breakpoints and the debug loop both go through `/sap/bc/adt/debugger*` on the session in `handlers_debug_session.go`. The old "REST breakpoints 403 on newer SAP" was the stateless client, not the release |
| `handlers_amdp.go` | Experimental | Session works, breakpoints unreliable |
| `pkg/adt/ui5.go` | Writes, ungated by package | Not read-only, and has not been since v2.10.0 (2025-12-05): `UI5UploadFile:273`, `UI5DeleteFile:312`, `UI5CreateApp:345`, `UI5DeleteApp:387`, all MCP-reachable via `handlers_ui5.go`. They go through the ADT filestore, not `/UI5/CL_REPOSITORY_LOAD`. The real hazard is `mutation_gate.go:117` — with `--allowed-packages` set, every UI5 mutation is refused outright because app→package resolution is unimplemented |
| `pkg/llvm2abap/`, `pkg/wasmcomp/` | Research | Not production; don't treat as stable |
| `pkg/adt/debugger.go` (REST) | Types and parsers only | Its *client* methods still assume a stateless session; the request builders and parsers are shared and exported via `debugger_parse.go` |
| `docs/cli-agents/*` | Config drift | Codex TOML format may differ from Claude/Gemini JSON docs |
| `pkg/datacluster/` | Reverse-engineered format | Object header, descriptor markers and type codes were read off real clusters (fixtures in `testdata/`: a 7.58 INDX with every type, a BALDAT). A kernel that writes a marker or type code not seen there fails loudly rather than misreading; add the fixture, then the code |
| `pkg/adt/sso*.go` | Host-dependent | Browser step must be a Windows process under WSL (PRT/WAM); needs `vsp-sso.exe` from `make sso-helper` |
