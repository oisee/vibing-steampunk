# Session Context: Strategic Decomposition, One-Tool Mode, CLI DevOps

**Date:** 2026-03-18
**Context ID:** 2026-03-18-001
**Branch:** main (all merged)
**Duration:** ~4 hours
**Topics:** Phase 1 decomposition, Phase 2 one-tool mode, Phase 4 CLI DevOps, community PR merges, bug fixes, README update

---

## What Was Done

### Phase 1: Decompose Monolith Files (merged via PR #72)

**`internal/mcp/server.go`** (2,539 → 256 lines):
- `server.go` — Server struct, Config, NewServer, ServeStdio, helpers (newToolResultError, ensureWSConnected, requireActiveAMDPSession)
- `tools_register.go` (1,944) — `registerTools()` + 21 domain-specific `register*Tools()` methods. Accepts `toolMode` param; if `"universal"`, calls `registerUniversalTool()` and returns early.
- `tools_focused.go` (134) — `focusedToolSet()` returns focused mode whitelist map
- `tools_groups.go` (55) — `toolGroups()` returns group definitions for `--disabled-groups`
- `tools_aliases.go` (59) — `registerToolAliases()` (disabled by default, bloats tool list)

**`pkg/adt/workflows.go`** (3,564 → 402 lines):
- `workflows.go` (402) — WriteProgram, WriteClass, CreateAndActivateProgram, CreateClassWithTests
- `workflows_source.go` (1,402) — GetSource, WriteSource, writeSourceCreate, writeSourceUpdate, writeClassMethodUpdate, CompareSource, CloneObject, GetClassInfo
- `workflows_deploy.go` (491) — DeployResult, CreateFromFile, UpdateFromFile, DeployFromFile, buildObjectURL, buildObjectURLWithParent, buildSourceURL
- `workflows_edit.go` (421) — EditSourceResult, EditSourceOptions, normalizeLineEndings, countMatches, replaceMatches, EditSource, EditSourceWithOptions (includes PR #36 ignore_warnings)
- `workflows_grep.go` (378) — GrepObject, GrepObjects, GrepPackage, GrepPackages, collectSubpackages, isSourceObject
- `workflows_fileio.go` (277) — RenameObject, SaveToFile (updated with parentName from PR #68), SaveClassIncludeToFile
- `workflows_execute.go` (272) — ExecuteABAP, ExecuteABAPMultiple

**`internal/mcp/handlers_codeintel.go`** (708 → 354 lines):
- `handlers_codeintel.go` (354) — FindDefinition, FindReferences, CodeCompletion, PrettyPrint, GetPrettyPrinterSettings, SetPrettyPrinterSettings, GetTypeHierarchy, GetClassComponents, formatClassComponents, componentFlags, GetInactiveObjects
- `handlers_source.go` (367) — registerGetSource, registerWriteSource, handleGetSource, handleWriteSource, registerGrepObjects, registerGrepPackages, registerImportFromFile, registerExportToFile, handleGrepObjects, handleGrepPackages

### Phase 2: One-Tool Mode (`--tool-mode universal`)

**New files:**
- `handlers_universal.go` (197) — Registers single `SAP(action, target, params)` tool. `handleUniversalTool` chains through 26 `route*Action` functions. Helper functions: parseTarget, getObject, getStringParam, getFloatParam, getBoolParam, newRequest, wrapErr, newToolResultJSON, callHandler.
- `handlers_help.go` (317) — `handleHelp(topic)` with docs for: read, edit, create, delete, search, query, test, grep, debug, analyze, system. Also `getUnhandledErrorMessage()`.

**26 handler files got `route*Action` functions:**
- Pattern: `func (s *Server) routeXxxAction(ctx, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error)`
- Returns `(result, true, err)` if handled, `(nil, false, nil)` if not
- Uses `callHandler(ctx, handler, args)` to delegate to existing handlers via `newRequest()`

**Config changes:**
- `Config.ToolMode` field added to `server.go`
- `--tool-mode` CLI flag added to `cmd/vsp/main.go` (default: "granular")
- `SAP_TOOL_MODE` env var via viper
- Validation: must be "granular" or "universal"

### Phase 4: CLI DevOps Surface

**New file: `cmd/vsp/devops.go` (1,134 lines)**

Commands added:
- `vsp source read <type> <name>` — GetSource
- `vsp source write <type> <name>` — WriteSource from stdin (--transport flag)
- `vsp source edit <type> <name>` — EditSource (--old, --new, --replace-all, --transport)
- `vsp source context <type> <name>` — GetSource + ctxcomp dependency contracts (--max-deps)
- `vsp context <type> <name>` — top-level shortcut for source context
- `vsp test [type] [name]` — RunUnitTests (--package for package-level)
- `vsp atc <type> <name>` — RunATCCheck (--variant, --max-findings)
- `vsp deploy <file> <package>` — DeployFromFile (--transport)
- `vsp transport list` — ListTransports (--user)
- `vsp transport get <number>` — GetTransport
- `vsp install zadt-vsp` — deploys 9 ABAP objects (--package, --dry-run, --skip-git-service)
- `vsp install abapgit` — deploys from embedded ZIP (--edition standalone|full, --package, --dry-run)
- `vsp install list` — shows installable components (offline)

**Modified: `cmd/vsp/cli.go`**
- `sourceCmd` changed from `Args: cobra.ExactArgs(2)` to `Args: cobra.ArbitraryArgs` — backwards compatible (2 args = direct source read) + subcommands
- Added --parent, --include, --method flags (from PR #59)

Helper: `buildObjectURL()` maps CLAS/PROG/INTF/FUGR/DDLS to ADT URL paths.
Adapter: `cliSourceAdapter` bridges `adt.Client` to `ctxcomp.ADTSourceFetcher`.

### Community PRs Merged (4)

| PR | Author | What |
|----|--------|------|
| #67 | AndreaBorgia-Abo | Fix class name reference (zadt_cl_tadir_move → zcl_vsp_tadir_mov) |
| #68 | dominik-kropp | Fix ExportToFile for function modules (added parentName param to SaveToFile) |
| #36 | kts982 | Add ignore_warnings to EditSource (closes #33) |
| #59 | thm-ma | Add --parent/--include/--method flags to CLI source command |

### Bugs Fixed (5)

| Issue | Fix | File |
|-------|-----|------|
| #71 | CreatePackage safety check uses `opts.Name` not `opts.PackageName` for package type | `pkg/adt/crud.go` |
| #54 | Install tools call `AllowPackageTemporarily()` to bypass SAP_ALLOWED_PACKAGES | `pkg/adt/client.go`, `internal/mcp/handlers_install.go` |
| #52 | SyntaxCheck uses bare object URL (no `/source/main`) for checkObject URI | `pkg/adt/devtools.go` |
| #70 | CreateTransport uses `/cts/transportrequests` + `transportorganizer.v1` for S/4HANA 757 | `pkg/adt/transport.go` |
| #69 | Added MIT LICENSE file | `LICENSE` |

### Issues Closed (6)

#69, #71, #54, #52, #70, #33

### Documentation

- README updated with one-tool mode, CLI DevOps commands, install workflow, tool modes section
- Report: `reports/2026-03-18-001-project-status-comprehensive.md` + `.epub` (bilingual EN/RU)

---

## Current State of Repository

**Branch:** main at commit `3e1e989`

**Open issues:** 13 (4 bugs: #55, #56, #43, #26; 6 features: #40, #39, #34, #30, #27, #21; 3 other)

**Open PRs ready to merge:** #37 (table pagination), #44 (Windows docs), #53 (clean core API)

**Open PRs need adaptation:** #62 (readonly mode — needs rebase to tools_focused.go pattern)

**Open PRs need review:** #38 (mcp-go v0.43.2 — large, 37 files), #42 (i18n tools), #41 (gCTS tools)

**Open PRs draft/blocked:** #66 (integration tests), #65 (Docker, blocked on #38), #64 (future plans), #63 (MkDocs docs)

---

## Key Architecture Decisions

1. **Decomposition is structural only** — no behavior changes, all tests pass, same 122 tools registered
2. **One-tool mode is opt-in** — default remains `granular` for backwards compat
3. **Route functions use try-handle pattern** — `(result, handled bool, error)` inspired by Filipp's `one-tool-mode` branch but rewritten fresh
4. **Did NOT cherry-pick from Filipp's branch** — his branch deleted cache/dsl/scripting and changed 226 files. We kept all packages and wrote routing fresh
5. **CLI commands reuse ADT client directly** — no MCP layer, same `pkg/adt` methods
6. **Install commands embed ABAP source** — read from `embedded/abap/*.abap` files, deploy via WriteSource

---

## What's Next (Prioritized)

1. Merge easy PRs: #37, #44, #53
2. Adapt and merge #62 (readonly mode — add `readonlyToolSet()` to `tools_focused.go`)
3. Review #38 (mcp-go upgrade — critical for Docker)
4. Review #42, #41 (i18n, gCTS from community)
5. Phase 3: WASM ABAP parser (abaplint/Lars Hvam) — `pkg/abap/`, offline AST
6. Fix #55 (RunReport APC — wrapper report + cache table)
7. Cross-system `vsp copy --from source -s target`

---

## LSP Plugin for Claude Code

Validated plugin structure at `/tmp/vsp-abap-lsp/`:

```
/tmp/vsp-abap-lsp/
├── .claude-plugin/
│   └── plugin.json    # name, version, description, author
└── .lsp.json          # abap: {command: "vsp", args: ["lsp", "--stdio"], fileExtensions: [".abap"]}
```

To use: `claude --plugin-dir /tmp/vsp-abap-lsp`

Requires `vsp` binary in PATH with SAP connection configured (env vars or .env).

---

## File Inventory (Changed/Created This Session)

### New files (13)
- `internal/mcp/tools_register.go` (1,944)
- `internal/mcp/tools_focused.go` (134)
- `internal/mcp/tools_groups.go` (55)
- `internal/mcp/tools_aliases.go` (59)
- `internal/mcp/handlers_universal.go` (197)
- `internal/mcp/handlers_help.go` (317)
- `internal/mcp/handlers_source.go` (367)
- `pkg/adt/workflows_source.go` (1,402)
- `pkg/adt/workflows_deploy.go` (491)
- `pkg/adt/workflows_edit.go` (421)
- `pkg/adt/workflows_grep.go` (378)
- `pkg/adt/workflows_fileio.go` (277)
- `pkg/adt/workflows_execute.go` (272)
- `cmd/vsp/devops.go` (1,134)

### Modified files (30)
- `internal/mcp/server.go` (2,539 → 256)
- `pkg/adt/workflows.go` (3,564 → 402)
- `internal/mcp/handlers_codeintel.go` (708 → 354)
- `cmd/vsp/cli.go` (minor — ArbitraryArgs + flags)
- `cmd/vsp/main.go` (ToolMode config)
- 26 handler files (added route*Action functions)
- `pkg/adt/crud.go` (bug #71 fix)
- `pkg/adt/client.go` (bug #54 fix — AllowPackageTemporarily)
- `pkg/adt/devtools.go` (bug #52 fix)
- `pkg/adt/transport.go` (bug #70 fix)
- `README.md` (documentation)
- `LICENSE` (new)
