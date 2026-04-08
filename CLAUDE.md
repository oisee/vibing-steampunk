# CLAUDE.md

**vsp** — Go-native MCP server and CLI for SAP ABAP Development Tools (ADT).

> **Doc intent:** CLAUDE.md = dev context. README.md = user onboarding. reports/ = research/history. contexts/ = session handoff.

---

## Current Priorities

### 1. Graph Engine (`pkg/graph/`) — In Progress
Sequence: unify existing dep logic → SQL/ADT adapters → impact/path queries.
- Done: core types, parser dep extraction, boundary analyzer (11 tests)
- Pending: SQL adapters (CROSS/WBCROSSGT/D010INC), ADT adapters, unify `cli_deps.go` + `cli_extra.go` + `ctxcomp/analyzer.go`
- Design: [002](reports/2026-04-05-002-graph-engine-design.md), [003](reports/2026-04-05-003-graph-engine-alignment-for-claude.md)

### 2. GUI Debugger (Issue #2) — Strategic
Plan: MCP debug sessions → DAP → Web UI. ADT REST API mapped from `CL_TPDA_ADT_RES_APP`. Design: [001](reports/2026-04-05-001-gui-debugger-design.md)

### 3. Open Issues
- **#88** Lock handle bug (EditSource/WriteSource) — real user report
- **#55** RunReport in APC — architectural limit
- **#46, #45** Sync script — low effort

### 4. Recent Additions (post-merge, v2.32 → v2.38.1)
Discoverable but not yet load-bearing in workflows:
- **`AnalyzeABAPCode`** — abaplint static analysis MCP tool (PR #89)
- **Slim V2** — method-level dead-code analysis (`vsp slim --level method`)
- **Package health MVP** — `vsp health <package>` with E070 transport fallback
- **`internal/lsp/`** — ABAP LSP server (online diagnostics, go-to-definition)
- **`cmd/abapgit-pack/`** — standalone abapGit ZIP packer
- **Browser auth** — `pkg/adt/browser_auth.go` (chromedp-based interactive login)
- **New MCP handler domains** — `cds`, `codeanalysis`, `gcts`, `graph`, `health`, `i18n`, `revisions`, `testing`

---

## Build & Test

```bash
# Build
go build -o vsp ./cmd/vsp

# Run unit tests
go test ./...

# Run integration tests (requires SAP system)
SAP_URL=http://host:port SAP_USER=user SAP_PASSWORD=pass SAP_CLIENT=001 \
  go test -tags=integration -v ./pkg/adt/

# Cross-compile (via Makefile)
make build              # Current platform → build/vsp
make build-all          # 3 common platforms (linux-amd64, darwin-arm64, windows-amd64)
make build-all-all      # All 9 platforms
```

Key flags: `--mode focused|expert|hyperfocused`, `--read-only`, `--allowed-packages "Z*"`, `--disabled-groups 5THD`

---

# Using environment variables
SAP_URL=http://host:50000 SAP_USER=user SAP_PASSWORD=pass ./vsp

# Using cookie authentication
./vsp --url http://host:50000 --cookie-string "sap-usercontext=abc; SAP_SESSIONID=xyz"
./vsp --url http://host:50000 --cookie-file cookies.txt
```

| Variable / Flag | Description |
|-----------------|-------------|
| `SAP_URL` / `--url` | SAP system URL (e.g., `http://host:50000`) |
| `SAP_USER` / `--user` | SAP username |
| `SAP_PASSWORD` / `--password` | SAP password |
| `SAP_CLIENT` / `--client` | SAP client number (default: 001) |
| `SAP_LANGUAGE` / `--language` | SAP language (default: EN) |
| `SAP_INSECURE` / `--insecure` | Skip TLS verification (default: false) |
| `SAP_COOKIE_FILE` / `--cookie-file` | Path to Netscape-format cookie file |
| `SAP_COOKIE_STRING` / `--cookie-string` | Cookie string (key1=val1; key2=val2) |
| `SAP_MODE` / `--mode` | Tool mode: `focused` (81 tools, default) or `expert` (122 tools) |
| `SAP_DISABLED_GROUPS` / `--disabled-groups` | Disable tool groups: `5`/`U`=UI5, `T`=Tests, `H`=HANA, `D`=Debug, `C`=CTS, `G`=Git, `R`=Reports, `I`=Install, `X`=Experimental |
| `SAP_VERBOSE` / `--verbose` | Enable verbose logging to stderr |
| **Safety Configuration** | |
| `SAP_READ_ONLY` / `--read-only` | Block all write operations (default: false) |
| `SAP_BLOCK_FREE_SQL` / `--block-free-sql` | Block RunQuery execution (default: false) |
| `SAP_ALLOWED_OPS` / `--allowed-ops` | Whitelist operation types (e.g., "RSQ") |
| `SAP_DISALLOWED_OPS` / `--disallowed-ops` | Blacklist operation types (e.g., "CDUA") |
| `SAP_ALLOWED_PACKAGES` / `--allowed-packages` | Restrict to packages (supports wildcards: "Z*") |
| `SAP_ALLOW_TRANSPORTABLE_EDITS` / `--allow-transportable-edits` | Allow editing objects in transportable packages (default: false) |
| **Feature Configuration (Safety Network)** | |
| `SAP_FEATURE_ABAPGIT` / `--feature-abapgit` | abapGit integration: auto, on, off (default: auto) |
| `SAP_FEATURE_RAP` / `--feature-rap` | RAP/OData development: auto, on, off (default: auto) |
| `SAP_FEATURE_AMDP` / `--feature-amdp` | AMDP/HANA debugger: auto, on, off (default: auto) |
| `SAP_FEATURE_UI5` / `--feature-ui5` | UI5/Fiori BSP management: auto, on, off (default: auto) |
| `SAP_FEATURE_TRANSPORT` / `--feature-transport` | CTS transport management: auto, on, off (default: auto) |

## Codebase Structure

```
cmd/
├── vsp/                  # Main CLI + MCP server (18 files: cli, devops, compile, deps, lsp, lua, workflow, ...)
└── abapgit-pack/         # Standalone abapGit ZIP packer

internal/
├── mcp/                  # MCP server core + 37 handlers_*.go (one per domain: crud, git, graph, health, ...)
└── lsp/                  # ABAP LSP server (online diagnostics, go-to-definition)

pkg/
├── adt/                  # ADT REST client (HTTP, CSRF, sessions, all SAP ops; 28+ files)
├── abaplint/             # Native Go port of abaplint: lexer, statement parser, 8 lint rules (oracle-verified)
├── graph/                # Dependency graph engine: queries (slim/health/rename/impact/api-surface), builders, scopes
├── ctxcomp/              # Context compression: dep resolution + contract injection for GetSource
├── dsl/                  # Fluent API + YAML workflow engine, batch import/export, pipelines
├── cache/                # In-memory + SQLite cache
├── config/               # Multi-system profile management (add/list/switch)
├── scripting/            # Lua VM + 50+ ADT tool bindings, REPL
├── jseval/               # JavaScript evaluator
├── llvm2abap/            # LLVM IR → ABAP compiler (research)
├── ts2abap/              # TypeScript → ABAP transpiler (research)
├── ts2go/                # TypeScript → Go transpiler (used to port abaplint)
└── wasmcomp/             # WASM → ABAP AOT compiler (QuickJS proven on SAP)

embedded/
├── abap/                 # ABAP sources installed on SAP (ZADT_VSP service, debug, git, AMDP, reports, RFC)
└── deps/                 # Embedded abapGit dependency ZIPs

docs/                     # Architecture, ADRs, cli-agents guides (4 langs), reviewer guide
contexts/                 # Session handoff notes (chronological)
reports/                  # Research / design / status reports (YYYY-MM-DD-NNN-title.md)
articles/                 # Published articles (upstream author content — do not rewrite oisee URLs)
abap/src/zadt_vsp/        # ABAP source mirror in abapGit format
scripts/                  # Sync upstream, release helpers

Makefile                  # Cross-compilation (9 platforms)
ARCHITECTURE.md  ROADMAP.md  VISION.md  README_TOOLS.md
```

> File-level detail rots fast. To list current files in a package: `ls pkg/<name>/`. To find a handler: `ls internal/mcp/handlers_*.go`.

### Where things live

| Task | Where |
|------|-------|
| New MCP tool | `tools_register.go` + `handlers_<domain>.go` + (optional) `tools_focused.go` — see "Adding a New MCP Tool" below |
| New ADT operation | `pkg/adt/` — `client.go` (read), `crud.go` (write), `devtools.go` (syntax/activate), `codeintel.go` (def/refs), `debugger.go` / `amdp_debugger.go`, `git.go`, `reports.go`, `transport.go`, `ui5.go`, `workflows.go` |
| New graph query | `pkg/graph/queries_*.go` |
| New ABAP lint rule | `pkg/abaplint/rules.go` |
| New CLI command | `cmd/vsp/cli_*.go` (see existing: `cli_extra.go`, `cli_compile.go`, `cli_deps.go`, `devops.go`) |
| Integration test | `pkg/adt/integration_test.go` (build tag `integration`) |
| One-tool / hyperfocused mode | `internal/mcp/handlers_universal.go` |

---

## Adding a New MCP Tool

1. **Add ADT client method** in `pkg/adt/` (e.g. `client.go`, `crud.go`, `devtools.go`).
2. **Register tool** in `internal/mcp/tools_register.go` with `shouldRegister("ToolName")` and the tool definition.
3. **Whitelist for focused mode** (optional): add to `internal/mcp/tools_focused.go`. For tool grouping (`--disabled-groups`), update `tools_groups.go`.
4. **Add handler** in the appropriate `internal/mcp/handlers_<domain>.go` (37 domain files exist; pick the closest fit or create a new one). Handlers are routed from `handleToolCall()` in `server.go`.
5. **Add integration test** in `pkg/adt/integration_test.go` (build tag `integration`).
6. **Update `README_TOOLS.md`** tool reference table.

> Legacy path: `internal/mcp/server.go` → `registerTools()` still exists for some early tools, but new work goes through `tools_register.go`.

## Code Patterns

### ADT Client Methods

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

### Tool Handler Pattern

Handlers are organized in domain-specific files (`internal/mcp/handlers_*.go`). Each file contains handler functions for related tools:

```go
// In handlers_read.go (or appropriate domain file)
func (s *Server) handleNewTool(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
    name, _ := getString(args, "name")
    result, err := s.client.NewMethod(ctx, name)
    if err != nil {
        return mcp.NewToolResultError(err.Error()), nil
    }
    return mcp.NewToolResultText(formatResult(result)), nil
}
```

### AMDP WebSocket Client Pattern (via ZADT_VSP)

For AMDP/HANA debugging, we use WebSocket connection to the ZADT_VSP APC handler:

```go
// WebSocket client connects to ZADT_VSP for stateful debugging
type AMDPWebSocketClient struct {
    conn      *websocket.Conn
    sessionID string
    isActive  bool
    Events    chan *AMDPEvent
    // ...
}

// Handler uses WebSocket client directly
func (s *Server) handleAMDPDebuggerStep(...) {
    if err := s.amdpWSClient.Step(ctx, stepType); err != nil {
        return newToolResultError(fmt.Sprintf("AMDPDebuggerStep failed: %v", err)), nil
    }
    // ...
}
```

See `pkg/adt/amdp_websocket.go` for Go client implementation.
See `embedded/abap/zcl_vsp_amdp_service.clas.abap` for ABAP service implementation.

## Testing

### Unit Tests (499 tests across 6 packages)
- `internal/mcp` - Server, tool registration, handler tests
- `pkg/adt` - ADT client, HTTP, safety, transport, codeintel, debugger, help, history, recorder, XML tests
- `pkg/cache` - In-memory and SQLite cache tests
- `pkg/config` - System profile management tests
- `pkg/dsl` - DSL, workflow, search tests
- `pkg/scripting` - Lua VM, bindings, helpers tests
- Run: `go test ./...`

### Integration Tests (35 tests)
- Build tag: `integration`
- Create objects in `$TMP` package, clean up after
- Run: `go test -tags=integration -v ./pkg/adt/`
- Test program for manual testing: `ZTEST_MCP_CRUD` in `$TMP`

## ADT API Reference

The SAP ADT REST API documentation can be found at:
- `/sap/bc/adt/discovery` - API discovery document
- See `reports/adt-abap-internals-documentation.md` for detailed endpoint analysis

---

## Common Issues

1. **CSRF errors** — auto-refreshed in `http.go`
2. **Lock conflicts** — edit handler does auto lock/unlock
3. **Session issues** — some CRUD/debugger flows are session-sensitive; verify stateful/stateless before changing transport or auth logic
4. **Auth** — use basic OR cookies, not both
5. **ZADT_VSP** — WebSocket debug/RFC/RunReport require it installed on SAP

## Conventions

### Object Naming
| Object Type | Pattern | Example |
|-------------|---------|---------|
| Programs | `ZADT_<nn>_<name>` | `ZADT_00_DEBUG_TEST` |
| Classes | `ZCL_ADT_<name>` | `ZCL_ADT_DEBUG_HELPER` |
| Interfaces | `ZIF_ADT_<name>` | `ZIF_ADT_DEBUGGABLE` |
| Function Groups | `ZADT_<nn>_<name>` | `ZADT_00_UTILS` |

### Debugging via Unit Tests
To trigger breakpoints programmatically (without SAP GUI):
1. Create a class with test methods (`lcl_test` pattern)
2. Set external breakpoint on the test code
3. Run `RunUnitTests` to trigger the breakpoint
4. Use `DebuggerListen` → `DebuggerAttach` to catch and debug

This allows AI-driven debugging without manual SAP GUI interaction.

## Security Notes

- Never commit `.env`, `cookies.txt`, or `.mcp.json` (all in `.gitignore`)
- Session summaries (`*SESSION-SUMMARY*`) are also gitignored
- Always verify no credentials in `git log --all -p` before pushing

## Reports and Documentation

### Report Naming Convention

All research reports, analysis documents, and design specifications follow this naming pattern:

**Format:** `./reports/{YYYY-MM-DD-<number>-<title>}.md`

**Examples:**
- `2025-12-02-001-auto-pilot-cross-wbcrossgt-analysis.md`
- `2025-12-02-005-improved-graph-architecture-design.md`

**Numbering:**
- Sequential numbers starting from 001 each day
- Preserves chronological order
- Easy to reference in documentation

### Current Reports (134 total: 123 dated + 11 reference)

**Date range:** 2025-12-02 through 2026-02-07

**Categories:**
- Analysis & research (graph architecture, CROSS/WBCROSSGT, ADT capabilities)
- Design documents (cache, safety, DSL, graph traversal, test intelligence)
- Implementation reports (cache, safety, debugger, AMDP, abapGit, transport)
- Strategic reports (future vision, SAP positioning, CBA alignment, Codex evaluation)
- Feature designs (tool visibility, report execution, async, transportable edits)
- Status reports (project status snapshots at various milestones)

Browse `reports/` directory for full listing. Files follow `YYYY-MM-DD-NNN-title.md` naming.

#### Reference Documentation (Non-numbered)
- `abap-adt-discovery-guide.md` - ADT API discovery process
- `abap-adt-tools-overview.md` - ADT tools overview
- `adt-abap-internals-documentation.md` - Detailed ADT endpoint analysis
- `adt-capability-matrix.md` - ADT feature comparison
- `adt-toolset-analysis.md` - ADT toolset analysis
- `adt-tracing-and-z-implementations.md` - ADT tracing and Z implementations
- `cookie-auth-implementation-guide.md` - Cookie authentication research
- `focused-mode-proposal.md` - Focused mode design proposal
- `golang-port-assessment.md` - Go port assessment
- `mcp-adt-go-status.md` - MCP ADT Go status
- `project-rename-analysis.md` - Project rename analysis

### Creating New Reports

When creating a new report:

1. **Determine the date:** Use ISO format `YYYY-MM-DD`
2. **Assign next number:** Continue sequence from last report that day
3. **Choose descriptive title:** Lowercase, hyphen-separated
4. **Use the format:** `reports/{YYYY-MM-DD-<number>-<title>}.md`
5. **Include metadata:** Date, Report ID, Subject at top of document

**Template:**
```markdown
# Report Title

**Date:** 2025-12-02
**Report ID:** 009
**Subject:** Brief description
**Related Documents:** Links to related reports

---

## Project Status

| Metric | Value |
|--------|-------|
| **Latest version** | v2.38.1 |
| **Modes** | `focused` (~81 tools, default) · `expert` (122 tools) · `hyperfocused` (1 universal tool) |
| **Tests** | ~1,000 unit test cases across 16 packages + 35 integration tests |
| **Platforms** | 9 (cross-compiled via Makefile) |
| **Reports** | 179 in `reports/` (`YYYY-MM-DD-NNN-title.md`) |
| **Sync** | 0 commits behind upstream `oisee/vibing-steampunk` (auto-merged via `scripts/sync-upstream.sh`) |

### Recent / in-flight (post v2.32)

| Area | Status |
|------|--------|
| `pkg/graph/` engine | 🚧 In progress — queries (slim, health, impact, rename, api-surface), SQL/transport builders, scope analysis |
| `AnalyzeABAPCode` tool | ✅ abaplint-based static analysis (PR #89) |
| Slim V2 dead-code | ✅ Method-level with `--level` flag, TDEVC hierarchy resolution |
| Package health MVP | ✅ E070 transport fallback for staleness signal, package-level CLI |
| Browser auth | ✅ `pkg/adt/browser_auth.go` (chromedp-based) |
| ABAP LSP server | ✅ `internal/lsp/` — online diagnostics, go-to-definition |
| New MCP handlers | ✅ `cds`, `codeanalysis`, `gcts`, `graph`, `health`, `i18n`, `revisions`, `testing` |
| AMDP Debugger | ⚠️ Experimental — session works, breakpoints need investigation (expert mode only) |
| UI5/BSP Mgmt | ⚠️ Partial — Read ops work; Create needs alternate API |

### DSL & Workflow Usage

```bash
# Run unit tests for a package
vsp workflow test "$TMP"
vsp workflow test "$ZRAY*" --parallel 4 --json

# Run YAML workflow
vsp workflow run examples/workflows/ci-pipeline.yaml --var PACKAGE=\$TMP
```

```go
// Go fluent API - Search & Test
objects, _ := dsl.Search(client).
    Query("ZCL_*").
    Classes().
    InPackage("$TMP").
    Execute(ctx)

summary, _ := dsl.Test(client).
    Objects(objects...).
    IncludeDangerous().
    Parallel(4).
    Run(ctx)

// Batch Import (abapGit-compatible)
result, _ := dsl.Import(client).
    FromDirectory("./src/").
    ToPackage("$ZRAY").
    RAPOrder().  // DDLS → BDEF → Classes → SRVD
    Execute(ctx)

// Batch Export (with all class includes)
result, _ := dsl.Export(client).
    Classes("ZCL_TRAVEL").
    ToDirectory("./backup/").
    Execute(ctx)

// RAP Deployment Pipeline
pipeline := dsl.RAPPipeline(client, "./src/", "$ZRAY", "ZTRAVEL_SB")
```

### Roadmap
- Graph Traversal & Analysis (Design: Reports 005-007)
- Standard API Surface Scraper (Design: Report 006)
- Test Intelligence - smart test execution based on code changes (Design: Report 008)
- One Tool Mode - ultra-minimal tool consolidation (Design: 2026-02-01-001)
- abapGit dependency management & submodules (Design: 2026-02-03-001)

---

## Upstream Sync Automation

This fork automatically syncs with upstream `oisee/vibing-steampunk`. See [scripts/README.md](scripts/README.md) for details.

### Quick Sync

```bash
# Manual sync (recommended first time)
./scripts/sync-upstream.sh

# Auto-merge and push
./scripts/sync-upstream.sh --auto-merge --push

# Or trigger GitHub Action
gh workflow run sync-upstream.yml
```

### What's Automated

- ✅ Daily checks for upstream changes (2 AM UTC)
- ✅ Auto-merge when no conflicts
- ✅ Fix import paths (`oisee` → `vinchacho`)
- ✅ Update dependencies (`go mod tidy`)
- ✅ Build & test verification
- ✅ Create PR for review
- ⚠️ CLAUDE.md updates (template provided, manual review needed)
- ⚠️ Markdown URL fixes (`oisee` → `vinchacho` in `docs/` only, NOT `articles/`)
- ⚠️ CLAUDE.md/README.md conflict resolution (script only handles `cmd/vsp/main.go`)

### Conflict Resolution Strategy

When resolving fork-vs-upstream conflicts:
- **CLAUDE.md data sections** (test counts, feature lists, codebase structure): keep fork (HEAD) — it has richer, more accurate content
- **CLAUDE.md new content** (new sections from upstream): merge in
- **README.md URLs**: keep `vinchacho` URLs, incorporate new upstream content (links, badges)
- **`docs/` markdown**: fix `oisee` → `vinchacho` in all repo URLs
- **`articles/`**: do NOT change `oisee` references — these are published upstream author content referencing their own repos (`oisee/zork-abap`, `oisee/vivid-vibes`)

