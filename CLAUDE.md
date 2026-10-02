# CLAUDE.md

**vsp** — Go-native MCP server and CLI for SAP ABAP Development Tools (ADT).

> **Doc intent:** CLAUDE.md = what an agent needs in every session. README.md = user onboarding. [`docs/dev-notes.md`](docs/dev-notes.md) = background and history by area. reports/ = research. contexts/ = session handoff. agenda/ = what is open and what was decided (`AGENDA.md` is the living board).
>
> **Shared knowledge base:** [`../sap-kb/`](../sap-kb/) maps vsp against its SAP-protocol siblings (open-rfc-go, open-diag-go-pro, sap-sso-trace). vsp owns `pkg/sapcompress` (decode), `pkg/datacluster`, the ADT transport and the `ZADT_VSP` bridge.

Current work and priorities are on the project's Trail Map, in `agenda/AGENDA.md`, and in the GitHub issues. They are not in this file, because a status line here goes stale.

---

## Build & Test

```bash
go build -o vsp ./cmd/vsp               # Build
go test ./...                            # Unit tests
go test -tags=integration -v ./pkg/adt/  # Integration (needs SAP or the OSD emulator)
make build-all-all                       # Release build, all 9 platforms (build-all builds only 3)
```

Key flags: `--mode focused|expert|hyperfocused`, `--read-only`, `--allowed-packages "Z*"`, `--block-free-sql`, `--disabled-groups 5THD`

**CI on every PR:**
- **Blocking:** build, vet, tests (`-race`, shuffled), the correctness lint on new code (`.github/ci/lint.sh gate`), and the leak scan.
- **Advisory:** full lint, complexity and token drift (`make metrics`), fuzz, and an integration run against the OSD SAP emulator (`.github/workflows/osd-integration.yml`, versions pinned with committed sha256).
- **Hooks:** opt-in, enabled with `git config core.hooksPath .githooks`. The pre-push hook runs the lint gate and the leak scan.

---

## Agents and budget modes

How to run codex and Claude subagents (commands, sandboxes, prompt shapes, the review loop) is in the shared playbook [oisee/agent-playbook](https://github.com/oisee/agent-playbook) (private; `gh repo clone oisee/agent-playbook`), especially [critics-and-executors](https://github.com/oisee/agent-playbook/blob/main/critics-and-executors.md) and [review-loop](https://github.com/oisee/agent-playbook/blob/main/review-loop.md). Alice sets the mode in chat; without a word from her it is **normal**.

| Mode | Who writes code | Critics | Claude reads |
|------|-----------------|---------|--------------|
| **Economy** ("экономим") | codex executors (`-s workspace-write`, own worktree, `gpt-6-sol` medium); Claude only writes TASK.md/CRITIC.md and orchestrates | one per PR: codex for Claude's few lines, a Sonnet subagent on the diff only for codex's code | the diff and the critic's final answer, not whole files or logs |
| **Normal** | Claude, or Claude subagents in worktrees | codex read-only critic on every PR; a fresh Claude critic for codex-written code; block only on P1/P2 | as needed |
| **Lavish** ("не экономим") | as normal; for safety gates, release, delete | both families every round, a running critic that applies mutants, a third-model tie-breaker, high effort | everything it needs |

Every mode keeps the gates: CI green, cross-family review before merge, and the hard rules in the spec (commit locally, never push or comment from an executor).

---

## Codebase

```
cmd/vsp/              CLI entry + commands (devops_*.go per command family)
internal/mcp/
  handlers_*.go       Domain handlers (read, edit, debug, graph, ...)
  tools_register.go   Mode logic (shouldRegister) + registration order
  tools_<domain>.go   register*Tools per domain
  tools_focused.go    Focused mode whitelist
  handlers_universal.go  Hyperfocused single tool (SAP)
  help/*.txt          Help topics (embedded)
  readonly_classes_test.go  READ/MUTATE/EXECUTE classification of every tool and action
pkg/
  adt/                ADT client (HTTP, CSRF, sessions, all SAP ops), one file per domain:
    client.go           Client, NewClient*, keep-alive, cookies, Language, Safety
    package_guard.go    package allowlist (safety gate, used by checkMutation)
    search.go  objects_read.go  package_read.go  ddic_read.go  query_sql.go  system_info.go
    lock.go  create.go  delete.go  object_urls.go  table_create.go  ...
    debugger.go         ADT debugger requests and parsers (no ZADT_VSP needed). The held stateful session lives in internal/mcp/handlers_debug_session.go; the standalone client methods use the ordinary transport
    git_import.go       abapGit zip import / conditional delete (via ZADT_VSP)
  graph/              Dependency graph engine
    adtsource/          What the graph is read from on SAP; shared by cmd/vsp and internal/mcp
  datacluster/        EXPORT data cluster parser (BALDAT, INDX, STXL)
  sapcompress/        SAP LZH and LZC decoders
  temse/  itf/        TemSe spool → lines; SAPscript ITF → Markdown
  ctxcomp/            Context compression (dep resolution for read)
  abaplint/           ABAP lexer + parser + lint rules
  dsl/  cache/  scripting/   Fluent API and YAML workflows; caches; Lua
  (the ABAP transpilers, ex-`vsp compile`, moved to github.com/oisee/abapiti)
src/ + embedded/abap/ ZADT_VSP ABAP sources (embedded/ is generated from src/; CI fails on drift)
```

| Task | Files |
|------|-------|
| Add MCP tool | `tools_<domain>.go` + `handlers_*.go` + `tools_focused.go` (see below) |
| Add ADT operation | the domain file in `pkg/adt/`; `package_guard.go` when mutating |
| Change help text | `internal/mcp/help/<topic>.txt`, then regenerate the help golden |
| Touch SSO auth | `pkg/adt/sso*.go`, `cmd/vsp-sso/`, `cmd/vsp/sso.go` |
| Add graph feature | `pkg/graph/`, `pkg/graph/adtsource/` |
| Add lint rule | `pkg/abaplint/rules.go` |
| Add integration test | `pkg/adt/integration_test.go` (honour `VSP_TEST_PACKAGE` / `VSP_TEST_TIMEOUT`) |
| Change ZADT_VSP | `src/`, then regenerate `embedded/abap/`; keep the guard tests in `embedded/abap/*_test.go` pinning every refusal |

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
2. Register it in the matching `tools_<domain>.go` with `shouldRegister("X")`. A new domain also needs a call in `tools_register.go`.
3. Route it in `handlers_analysis.go` (or the appropriate router).
4. Add it to `tools_focused.go` if it is needed in focused mode.
5. Classify it in `internal/mcp/readonly_classes_test.go` as READ, MUTATE or EXECUTE. The read-only invariant test fails with "classify me" otherwise.
6. Run `go test ./internal/mcp -run 'TestToolRegistryGolden|TestHelpGolden' -update-tools-golden -update-help-golden` and commit the golden diff.

**Safety gates come before any write.** Read-only, operation filters and free SQL are refused before anything is sent. With `--allowed-packages`, `checkMutation` may first send a read to resolve an existing object's package. It never writes before the gate has passed.

---

## Working against a live SAP system

- **Use only the dedicated test user, and print it first.** A `.vsp.json` in the working directory beats environment variables. Run live CLI checks from an empty scratch directory and confirm the user with `vsp config show` before the first request.
- **Stop at the first 401. Don't retry.** A wrong password for a real user counts against `login/fails_to_user_lock`, and one sweep locks the account for a day. After that, even the correct password returns 401.
- **To provoke a failure, use these in order:**
  1. a client-side refusal (`--block-free-sql`, `--disallowed-ops`);
  2. an `httptest` server that returns 403;
  3. an object that doesn't exist;
  4. an unresolvable hostname;
  5. a user that doesn't exist, for a few requests only.

  Never a real user with a wrong password.
- **Throwaway objects go in `$TMP` or a `$ZVSP_*` scratch package.** End every live run with proof that nothing is left: search by prefix, read each object back (404), and confirm no ENQUEUE locks remain.
- **After testing a branch's ZADT_VSP, reinstall main's.**
- **vsp never runs a transport import.** Adding a request to the import queue is allowed. The import stays a human step in STMS.

## Common Issues

1. **CSRF errors** are auto-refreshed in `http.go`.
2. **Locks and sessions:** a lock handle lives in a stateful session, and any stateless request between LOCK and the write can kill it (423). See [`docs/dev-notes.md`](docs/dev-notes.md#lock-handles-and-sessions-91-family). Check stateful vs stateless before changing transport or auth logic.
3. **Auth:** use basic auth OR cookies, never both. `HasBasicAuth()` disables `ReauthFunc`, so a stray `SAP_USER`/`SAP_PASSWORD` alongside SSO silently kills auto-refresh.
4. **Expired SSO sessions don't return 401.** ICF forwards to the IdP, and a logon page arrives under a 200. Detection is by origin and by a missing CSRF token (`http.go`).
5. **ZADT_VSP** (the APC/WebSocket bridge) is required for the WebSocket-based MCP features: RFC and RunReport over the bridge, abapGit zip import and delete, and transport upload. Classic RFC (`vsp rfc call`, `vsp rfc run`) and ADT debugging don't need it.
6. **Response cache** (`VSP_CACHE`, `pkg/adt/response_cache.go`):
   - It keeps GET answers and data-preview queries on the tables in `stableTables`.
   - It is emptied on any write through the client.
   - A change made by someone else inside the TTL is invisible to it.

## Security

Never commit `.env`, `cookies.txt`, `.mcp.json`, or local agent/MCP config files (all in `.gitignore`).

**Sanitize policy.** The public repo must not contain identifiers that tie code or docs to a live SAP system, a real user or a customer's namespace. Anything that does goes under `.local/` (gitignored).

| Never in tracked files | Use instead |
|---|---|
| Real SAP usernames | `TESTUSER` |
| Real hostnames or IPs | `dev.example.local`, `prodsys-a.example` |
| Aliases of live boxes | `devsys`, `prodsys-a` |
| Live transport numbers | `TR-EXAMPLE` |
| Live change request IDs | `CR-EXAMPLE` |
| Customer namespaces | `ZDEMO_*`, `ZCL_DEMO_*`, `$ZDEMO` |
| Customer transport attributes | `Z_CR_ATTR` |
| Passwords, keys, tokens; real people tied to private systems | — |

Always OK: `$ZHIRTEST*`, `ZCL_HIRT*`, `ZCUSTOM_DEVELOPMENT`, public GitHub handles in the module path, and upstream OSS attribution.

**The leak scan enforces this** in CI and at pre-push, in full only where the identifier list is available. Fork PRs (no secret) and a pre-push run without `.local/leak-identifiers.txt` check generic patterns only, so treat those as partial. It reads text, hex, base64 and UTF-16, checks every added line and commit message, and takes its identifier list from the `VSP_LEAK_IDENTIFIERS` secret or `.local/leak-identifiers.txt`. Exceptions go in `.github/ci/leakscan-allow.txt`, each with a reason, in a PR of their own. The rule of thumb: could a stranger reading this file identify the customer, the system or a live account? If yes, redact.

## Conventions

Reports are named `reports/YYYY-MM-DD-NNN-title.md`.

**SAP object names:** the kind prefix, then the domain token `VSP`, then the name. There is never an underscore straight after `Z`.

| Kind | Form | Ours |
|------|------|------|
| Class | `ZCL_<domain>_<name>` | `ZCL_VSP_GIT_SERVICE` |
| Interface | `ZIF_<domain>_<name>` | `ZIF_VSP_SERVICE` |
| Program / function group / FM / message class | `Z<domain>_<name>` | `ZVSP_GIT_IMPORT` |
| Package | `$ZADT_VSP` | |

A numeric bucket (`ZCL_VSP_00_AMDP_TEST`) is used only for test fixtures. Older `ZADT_CL_*` / `ZCL_ADT_*` names have `ZCL_VSP_*` successors; don't add more.

---

## Areas Requiring Care

| Area | Notes |
|------|-------|
| `pkg/adt/package_guard.go`, `checkMutation` | Safety gate. Move it in its own commit; never weaken it in a refactor |
| `internal/mcp/readonly_*_test.go` | Judges what goes on the wire, not the label: a write labelled READ that goes over WebSocket or RFC isn't caught |
| `pkg/adt/git_import.go` + `src/zcl_vsp_git_service.clas.abap` | Conditional delete: version read under the lock; sha256 decides over the stamp; dependent objects first |
| `handlers_amdp.go` | Experimental: session works, breakpoints unreliable |
| `pkg/adt/ui5.go` | Writes through the ADT filestore (upload, delete, create app) and is MCP-reachable. With `--allowed-packages` set, every UI5 mutation is refused, because app→package resolution is unimplemented |
| `pkg/datacluster/` | Reverse-engineered format. A marker or type code not seen in `testdata/` fails loudly: add the fixture first, then the code |
| `pkg/adt/sso*.go` | Under WSL the browser step must be a Windows process (PRT/WAM); needs `vsp-sso.exe` from `make sso-helper` |
| ABAP transpilers (ex-`vsp compile`) | Moved to [ABAPiti](https://github.com/oisee/abapiti); fix them there, not here |
| `docs/cli-agents/*` | Config drift: Codex TOML differs from the Claude/Gemini JSON docs |
