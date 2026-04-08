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
cmd/vsp/                             # CLI entry point (6 files)
├── main.go                          # Entry point, Cobra root command
├── cli.go                           # CLI mode (interactive terminal)
├── config_cmd.go                    # System profile management commands
├── debug.go                         # Debug/diagnostic commands
├── lua.go                           # Lua REPL command
└── workflow.go                      # Workflow CLI commands

internal/mcp/                        # MCP server (27 files)
├── server.go                        # Server core, tool registration, mode/group logic
├── server_test.go                   # Server tests
└── handlers_*.go                    # 25 domain-specific handler files:
    handlers_amdp.go                 #   AMDP/HANA debugger
    handlers_analysis.go             #   Code analysis (call graph, structure)
    handlers_atc.go                  #   ATC checks
    handlers_classinclude.go         #   Class include operations
    handlers_codeintel.go            #   Code intelligence (find def/refs)
    handlers_crud.go                 #   CRUD operations (create, update, delete)
    handlers_debugger.go             #   External debugger (WebSocket)
    handlers_debugger_legacy.go      #   Legacy HTTP debugger
    handlers_deploy.go               #   abapGit ZIP deployment (3-phase bulk deploy)
    handlers_devtools.go             #   Dev tools (syntax, activate, pretty print)
    handlers_dumps.go                #   Runtime errors / short dumps
    handlers_fileio.go               #   File import/export
    handlers_git.go                  #   abapGit integration
    handlers_grep.go                 #   Grep/search operations
    handlers_install.go              #   Install tools (ZADT_VSP, abapGit)
    handlers_read.go                 #   Read operations (source, metadata)
    handlers_report.go               #   Report execution
    handlers_search.go               #   Object search
    handlers_servicebinding.go       #   RAP service binding publish
    handlers_sqltrace.go             #   SQL trace (ST05)
    handlers_system.go               #   System info
    handlers_traces.go               #   ABAP profiler traces
    handlers_transport.go            #   CTS transport management
    handlers_ui5.go                  #   UI5/Fiori BSP management
    handlers_workflow.go             #   Workflow operations

pkg/
├── adt/                             # ADT client library (28 source files)
│   ├── client.go                    # ADT client + read operations
│   ├── crud.go                      # CRUD operations (lock, create, update, delete)
│   ├── devtools.go                  # Dev tools (syntax check, activate, unit tests)
│   ├── codeintel.go                 # Code intelligence (find def, refs, completion)
│   ├── debugger.go                  # External debugger (breakpoints, listener)
│   ├── amdp_debugger.go            # HANA/AMDP debugger (SQLScript debugging)
│   ├── amdp_websocket.go           # AMDP WebSocket client
│   ├── websocket_base.go           # WebSocket base client (shared)
│   ├── websocket.go                # WebSocket connection management
│   ├── websocket_debug.go          # WebSocket debug service
│   ├── websocket_rfc.go            # WebSocket RFC service
│   ├── websocket_types.go          # WebSocket type definitions
│   ├── git.go                       # abapGit integration (GitTypes, GitExport)
│   ├── help.go                      # ABAP keyword help (GetAbapHelp)
│   ├── history.go                   # Object history / versions
│   ├── reports.go                   # Report execution (RunReport, variants)
│   ├── transport.go                 # CTS transport management
│   ├── fileparser.go                # File parser utilities
│   ├── recorder.go                  # HTTP request recorder
│   ├── ui5.go                       # UI5/Fiori BSP management
│   ├── workflows.go                 # High-level workflow operations
│   ├── cds.go                       # CDS view dependency analysis
│   ├── safety.go                    # Safety & protection configuration
│   ├── features.go                  # Feature detection (safety network)
│   ├── http.go                      # HTTP transport (CSRF, sessions)
│   ├── config.go                    # Configuration
│   ├── cookies.go                   # Cookie file parsing (Netscape format)
│   └── xml.go                       # XML types
│
├── config/                          # System profile management
│   ├── systems.go                   # Multi-system config (add, list, switch)
│   └── systems_test.go              # Config tests
│
├── dsl/                             # Fluent API & Workflow Engine
│   ├── types.go                     # Core types (ObjectRef, TestConfig, etc.)
│   ├── search.go                    # Fluent search builder
│   ├── test_runner.go               # Unit test orchestration
│   ├── workflow.go                  # YAML workflow engine
│   ├── batch.go                     # Batch operations & pipeline builder
│   └── import.go                    # Import operations
│
├── scripting/                       # Lua Scripting Engine
│   ├── lua.go                       # Lua VM wrapper, REPL
│   ├── bindings.go                  # ADT tool bindings for Lua
│   └── helpers.go                   # Lua<->Go value conversion
│
├── abaplint/                        # Native Go port of abaplint lexer
│   ├── lexer.go                     # Lexer (mechanical port from TS), 48 token types
│   ├── lexer_test.go                # Unit tests + oracle differential (29 files, 22K tokens)
│   └── testdata/
│       ├── oracle.js                # Node.js oracle using @abaplint/core
│       └── oracle_fixtures.json     # Oracle reference data
│
└── cache/                           # Caching infrastructure
    ├── cache.go                     # Core interfaces and types
    ├── memory.go                    # In-memory cache (default)
    ├── sqlite.go                    # SQLite cache (optional)
    ├── cache_test.go                # Unit tests (16 tests)
    ├── example_test.go              # Usage examples
    └── README.md                    # Documentation

embedded/
├── abap/                            # ABAP source files (13 files)
│   ├── zcl_vsp_apc_handler.clas.abap        # APC WebSocket handler
│   ├── zcl_vsp_amdp_service.clas.abap       # AMDP debug service
│   ├── zcl_vsp_debug_service.clas.abap      # Debug service
│   ├── zcl_vsp_git_service.clas.abap        # Git/abapGit service
│   ├── zcl_vsp_report_service.clas.abap     # Report execution service
│   ├── zcl_vsp_rfc_service.clas.abap        # RFC service
│   ├── zcl_vsp_utils.clas.abap              # Utility functions
│   ├── zif_vsp_service.intf.abap            # Service interface
│   ├── zadt_cl_tadir_move.clas.abap         # TADIR object mover
│   ├── zcl_adt_00_amdp_test.clas.abap       # AMDP test class
│   ├── zcl_adt_00_amdp_test.clas.testclasses.abap  # AMDP test methods
│   ├── zadt_test_alv_report.prog.abap       # ALV test report
│   └── zadt_test_simple_report.prog.abap    # Simple test report
└── deps/                            # Dependency embeddings
    └── embed.go

docs/                                # Project documentation
├── adr/                             # Architecture Decision Records (3 ADRs)
├── architecture.md                  # Architecture diagrams (Mermaid)
├── cli-agents/                      # CLI coding agents guide (4 languages)
├── reviewer-guide.md                # Reviewer guide (8 hands-on tasks)
└── plans/                           # Phase planning docs

articles/                            # Published articles

abap/src/zadt_vsp/                   # ABAP source (abapGit-format mirror)

Makefile                             # Cross-compilation (9 platforms)
ARCHITECTURE.md                      # Architecture overview
ROADMAP.md                           # Feature roadmap
VISION.md                            # Project vision
README_TOOLS.md                      # Tool reference (all 122 tools)
```

| Task | Files |
|------|-------|
| Register new MCP tool | `internal/mcp/server.go` (registerTools) |
| Add MCP tool handler | `internal/mcp/handlers_*.go` (domain-specific file) |
| Add ADT read operation | `pkg/adt/client.go` |
| Add CRUD operation | `pkg/adt/crud.go` |
| Add development tool | `pkg/adt/devtools.go` |
| Add code intelligence | `pkg/adt/codeintel.go` |
| Add ABAP debugger feature | `pkg/adt/debugger.go` |
| Add HANA/AMDP debugger | `pkg/adt/amdp_debugger.go` |
| Add WebSocket feature | `pkg/adt/websocket_base.go` |
| Add abapGit feature | `pkg/adt/git.go` |
| Add report feature | `pkg/adt/reports.go` |
| Add transport feature | `pkg/adt/transport.go` |
| Add UI5/BSP feature | `pkg/adt/ui5.go` |
| Add deployment feature | `internal/mcp/handlers_deploy.go` |
| Add workflow | `pkg/adt/workflows.go` |
| Add XML types | `pkg/adt/xml.go` |
| Add system config | `pkg/config/systems.go` |
| Add ABAP lint rule | `pkg/abaplint/lexer.go` / `pkg/abaplint/rules.go` |
| Add graph feature | `pkg/graph/` |
| Add MCP tool (modern) | `tools_register.go` + `handlers_*.go` + `tools_focused.go` |
| Add integration test | `pkg/adt/integration_test.go` |
| Fix MCP/docs/config | `README.md`, `docs/cli-agents/*`, `handlers_universal.go` |

---

## Adding a New MCP Tool

1. **Add ADT client method** in appropriate file (`client.go`, `crud.go`, etc.)
2. **Register tool** in `tools_register.go` with `shouldRegister("ToolName")` (or `internal/mcp/server.go` → `registerTools()` for legacy paths):
   - Add to `focusedTools` whitelist if it should appear in focused mode
3. **Add tool handler** in appropriate `internal/mcp/handlers_*.go` file:
   - Each domain has its own handler file (e.g., `handlers_crud.go`, `handlers_git.go`)
   - Handler functions are routed from `handleToolCall()` in `server.go`
4. **Add integration test** in `pkg/adt/integration_test.go`
5. **Update documentation**:
   - `README.md` tool tables

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

## Security

Never commit `.env`, `cookies.txt`, `.mcp.json`, or local agent/MCP config files (all in `.gitignore`).

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

## Areas Requiring Care

## Project Status

| Metric | Value |
|--------|-------|
| **Tools** | 122 (81 focused, 122 expert) |
| **Unit Tests** | 499 |
| **Integration Tests** | 35 |
| **Platforms** | 9 |
| **Phase** | 5 (TAS-Style Debugging) - Complete |
| **Reports** | 123 dated + 11 reference docs |
| **Lua Scripting** | ✅ Complete (v2.32 - REPL, 50+ bindings, 8 example scripts) |
| **Cache Package** | ✅ Complete (in-memory + SQLite) |
| **Safety System** | ✅ Complete (operation filtering, package restrictions) |
| **Feature Detection** | ✅ Complete (GetFeatures tool, auto/on/off for abapGit, RAP, AMDP, UI5, Transport) |
| **DSL Package** | ✅ Complete (fluent API, YAML workflows, test orchestration, batch import/export) |
| **Batch Import/Export** | ✅ Complete (v2.12 - abapGit-compatible format, priority ordering) |
| **Pipeline Builder** | ✅ Complete (v2.12 - DeployPipeline, RAPPipeline, ExportPipeline) |
| **ExecuteABAP** | ✅ Complete (code execution via Unit Test wrapper) |
| **System Info** | ✅ Complete (GetSystemInfo, GetInstalledComponents) |
| **Code Analysis** | ✅ Complete (GetCallGraph, GetObjectStructure, GetCallersOf, GetCalleesOf) |
| **Runtime Errors** | ✅ Complete (ListDumps, GetDump - RABAX) |
| **ABAP Profiler** | ✅ Complete (ListTraces, GetTrace - ATRA) |
| **SQL Trace** | ✅ Complete (GetSQLTraceState, ListSQLTraces - ST05) |
| **RAP OData E2E** | ✅ Complete (DDLS, SRVD, SRVB create + publish) |
| **Report Execution** | ✅ Complete (v2.18.0 - RunReport, GetVariants, text elements via ZADT_VSP) |
| **Async Execution** | ✅ Complete (v2.19.0 - RunReportAsync, GetAsyncResult for background tasks) |
| **Interactive Debugger** | ✅ Complete (v2.18.1 - WebSocket-based breakpoints, step, stack, variables) |
| **CLI Mode** | ✅ Complete (v2.20.0 - Interactive terminal mode with Cobra commands) |
| **System Profiles** | ✅ Complete (v2.20.0 - Multi-system config management via pkg/config) |
| **Method-Level Ops** | ✅ Complete (v2.21.0 - GetClassComponents, GetClassInclude, UpdateClassInclude) |
| **SAP GUI Integration** | ✅ Complete (v2.22.0 - GetTransaction, CallRFC via WebSocket) |
| **Transportable Edits** | ✅ Complete (v2.24.0 - --allow-transportable-edits safety control) |
| **External Debugger** | ✅ Complete via WebSocket ZADT_VSP (stateful APC, replaced HTTP) |
| **AMDP Debugger** | ⚠️ Experimental (Session works, breakpoints need investigation - expert mode only) |
| **Transport Mgmt** | ✅ Complete (5 tools with safety controls - v2.11.0) |
| **UI5/BSP Mgmt** | ✅ Partial (Read ops work; Create needs alternate API) |
| **Tool Groups** | ✅ Complete (--disabled-groups: 5/U, T, H, D, C, G, R, I, X) |
| **Class Includes** | ✅ Complete (v2.12 - testclasses, locals_def, locals_imp, macros) |
| **abapGit Integration** | ✅ Complete (v2.16.0 - WebSocket, GitTypes, GitExport - 158 object types) |
| **Install Tools** | ✅ Complete (v2.17.0 - InstallZADTVSP, InstallAbapGit, ListDependencies) |
| **GetAbapHelp** | ✅ Complete (v2.23.0 - ABAP keyword docs via WebSocket/ZADT_VSP) |
| **GitExport to Disk** | ✅ Complete (v2.23.0 - ZIP files written directly, no base64) |
| **Tool Visibility** | ✅ Complete (v2.22.0 - .vsp.json for granular tool control) |
| **HTTP Proxy** | ✅ Complete (v2.22.0 - HTTP_PROXY/HTTPS_PROXY support) |
| **DeployZip** | ✅ Complete (3-phase bulk deploy from abapGit ZIP: create → upload → activate) |
| **Iterative Activation** | ✅ Complete (ActivatePackageIterative with package filtering) |
| **Native ABAP Lexer** | ✅ Complete (v2.31 - abaplint lexer ported to Go, 100% oracle match, 22K tokens verified) |
| **ABAP Statement Parser** | ✅ Complete (v2.31 - 91 statement types, 100% oracle match, 3,254 statements) |
| **ABAP Linter** | ✅ Complete (v2.32 - 8 rules, 100% oracle match, 795μs/file) |
| **Context Depth** | ✅ Complete (v2.31 - multi-level dep expansion, depth 1-3, cycle detection) |
| **CLI Toolchain** | ✅ Complete (v2.32 - 28 commands: query, grep, graph, deps, lint, parse, compile, execute) |
| **WASM Self-Host** | ✅ Verified (v2.32 - 3-way proof: Native 51/51, Go OK, ABAP 11/11 on SAP) |
| **TS→Go Transpiler** | ✅ Complete (v2.32 - produces valid Go from abaplint TS, 3 files compile) |

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

### Upstream Roadmap (from last session 2026-03-20)

- [ ] **Phase 2: Statement parser** — port abaplint 2_statements to Go (318 types, 227 expressions)
- [ ] **Phase 3: Lint rules** — cherry-pick naming, obsolete, line_length rules
- [ ] **Wire `pkg/abaplint` lexer** into MCP parse_abap handler (replace self-written tokenizer)
- [ ] **Re-add ALV capture for RunReport**
