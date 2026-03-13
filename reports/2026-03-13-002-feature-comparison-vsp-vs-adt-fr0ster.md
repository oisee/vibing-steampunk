# Feature Comparison: vsp vs mcp-abap-adt

**Date:** 2026-03-13
**Report ID:** 002
**Subject:** Detailed feature gap analysis for merging vsp capabilities into mcp-abap-adt
**Goal:** Identify everything in vsp that mcp-abap-adt lacks — to drive a merge/port effort

---

## Table of Contents

1. [Quick Summary](#1-quick-summary)
2. [Technology Stack Comparison](#2-technology-stack-comparison)
3. [Features ONLY in vsp (missing from mcp-abap-adt)](#3-features-only-in-vsp-missing-from-mcp-abap-adt)
4. [Features ONLY in mcp-abap-adt (missing from vsp)](#4-features-only-in-mcp-abap-adt-missing-from-vsp)
5. [Features in Both Repos](#5-features-in-both-repos)
6. [Tool-by-Tool Matrix](#6-tool-by-tool-matrix)
7. [Merge Strategy Recommendations](#7-merge-strategy-recommendations)

---

## 1. Quick Summary

| Metric | vsp (vibing-steampunk) | mcp-abap-adt |
|--------|----------------------|----------------------|
| **Language** | Go | TypeScript / Node.js |
| **MCP Tools** | 122 (81 focused / 122 expert) | 272 (read-only + high + low + compact) |
| **ADT Object Types (CRUD)** | ~12 types | ~20 types |
| **Distribution** | Single binary, 9 platforms | npm package / Docker |
| **Transports** | stdio only | stdio, HTTP, SSE |
| **Auth Methods** | Basic, Cookie | Basic, JWT/XSUAA, Service Keys, RFC |
| **Safety System** | ✅ Full (5 mechanisms) | ❌ None |
| **Debugger (ABAP)** | ✅ Full WebSocket debugger | ❌ Not implemented |
| **Debugger (AMDP/HANA)** | ✅ Experimental | ❌ Not implemented |
| **Lua Scripting** | ✅ Full REPL + 40 bindings | ❌ Not implemented |
| **YAML Workflows** | ✅ Full engine | ❌ Not implemented |
| **Fluent DSL API** | ✅ Full | ❌ Not implemented |
| **Caching Layer** | ✅ Memory + SQLite | ⚠️ Object list cache only |
| **abapGit Export** | ✅ 158 object types | ❌ Not implemented |
| **ExecuteABAP** | ✅ Arbitrary code execution | ❌ Not implemented |
| **Call Graph Analysis** | ✅ 6 tools | ❌ Not implemented |
| **Where-Used Analysis** | ❌ Not implemented | ✅ GetWhereUsed |
| **ABAP AST** | ❌ Not implemented | ✅ GetAbapAST |
| **Semantic Analysis** | ❌ Not implemented | ✅ GetAbapSemanticAnalysis |
| **Enhancement Framework** | ❌ Not implemented | ✅ Read + Full CRUD stubs |
| **Domain / DataElement CRUD** | ❌ Not implemented | ✅ Full CRUD |
| **Metadata Extension (DDLX)** | ❌ Not implemented | ✅ Full CRUD |
| **Low-level lock/check/validate** | ⚠️ Only generic | ✅ Per-object-type |
| **RFC Connection (legacy)** | ❌ Not supported | ✅ v4.0 feature |
| **ABAP Cloud / BTP Auth** | ❌ No JWT/XSUAA | ✅ Full XSUAA support |
| **Unit Tests** | 244 | 244+ |
| **Integration Tests** | 34 | 34+ |

---

## 2. Technology Stack Comparison

| Aspect | vsp | mcp-abap-adt |
|--------|-----|--------------|
| **Language** | Go 1.23+ | TypeScript 5.9+ / Node.js 20+ |
| **Binary** | Single self-contained binary | npm package (requires Node.js) |
| **Platforms** | 9 (Windows, macOS, Linux, BSD, AIX) | Any Node.js platform + Docker |
| **MCP SDK** | mark3labs/mcp-go v0.17.0 | @modelcontextprotocol/sdk v1.27.1 |
| **HTTP Client** | net/http (stdlib) | axios v1.13.6 |
| **XML Parsing** | encoding/xml (stdlib) | fast-xml-parser, xml-js |
| **Config** | Viper + Cobra + .env | YAML + Commander + dotenv |
| **Validation** | Manual | Zod v4.3.6 |
| **Logging** | stderr + structured | Pino v10 |
| **Testing** | Go test | Jest + ts-jest |
| **WebSocket** | gorilla/websocket | N/A (no WebSocket) |
| **Cache** | SQLite + in-memory | In-memory only |
| **Scripting** | Lua (gopher-lua) | N/A |
| **Workflows** | YAML engine | N/A |

**Key trade-offs:**
- vsp is a single Go binary with no runtime requirements — easier deployment
- mcp-abap-adt has a richer ecosystem (npm, Docker, multiple transports, enterprise auth)
- The merge direction is: **port vsp's unique features INTO mcp-abap-adt** (TypeScript)

---

## 3. Features ONLY in vsp (Missing from mcp-abap-adt)

These are features that exist in vsp but are **entirely absent** from mcp-abap-adt. These are the primary candidates for porting.

---

### 3.1 🔴 Safety & Protection System

**vsp has a complete safety framework. mcp-abap-adt has zero protection mechanisms.**

| Safety Mechanism | vsp Implementation | Impact |
|-----------------|-------------------|--------|
| **Read-Only Mode** | `--read-only` / `SAP_READ_ONLY` — blocks ALL write operations | Prevents accidental production changes |
| **Block Free SQL** | `--block-free-sql` / `SAP_BLOCK_FREE_SQL` — disables RunQuery | Prevents arbitrary SQL data extraction |
| **Operation Whitelist** | `--allowed-ops` / `SAP_ALLOWED_OPS` — e.g. `"RSQ"` only allows Read/Search/Query | Fine-grained access control |
| **Operation Blacklist** | `--disallowed-ops` / `SAP_DISALLOWED_OPS` — e.g. `"CDUA"` blocks Create/Delete/Update/Activate | Granular denial |
| **Package Whitelist** | `--allowed-packages` / `SAP_ALLOWED_PACKAGES` — e.g. `"Z*"` only allows customer namespaces | Namespace safety |
| **Transportable Edits Control** | `--allow-transportable-edits` — blocks editing objects in transport-bound packages | Prevents accidental transport pollution |

**Files:** `pkg/adt/safety.go`, `pkg/adt/safety_test.go` (25 tests)

**Port effort:** Medium — requires a middleware/interceptor layer in the handler groups.

---

### 3.2 🔴 Feature Detection (Safety Network / Auto-Probe)

**vsp auto-detects whether SAP features exist before enabling tools.**

| Feature | What's Probed | Modes |
|---------|--------------|-------|
| `abapGit` | Checks if ABAP git endpoint exists | auto / on / off |
| `RAP` | Checks if RAP/OData service exists | auto / on / off |
| `AMDP` | Checks if HANA debugger is available | auto / on / off |
| `UI5` | Checks if BSP management exists | auto / on / off |
| `CTS Transport` | Checks if transport management is enabled | auto / on / off |
| `HANA` | Checks if HANA-specific features exist | auto / on / off |

`GetFeatures` MCP tool exposes this to the LLM so it knows what's safe to try.

**Config:** `SAP_FEATURE_ABAPGIT`, `SAP_FEATURE_RAP`, etc. with `auto`/`on`/`off`

**Files:** `pkg/adt/features.go`

**Port effort:** Low — lightweight HTTP probes, easy to port.

---

### 3.3 🔴 External ABAP Debugger (WebSocket-Based)

**mcp-abap-adt has no debugging capabilities at all.**

vsp implements a full external ABAP debugger via WebSocket (ZADT_VSP APC handler):

| Tool | Description |
|------|-------------|
| `SetBreakpoint` | Set line/statement/exception breakpoint (supports method-relative lines for class includes) |
| `GetBreakpoints` | List all active external breakpoints |
| `DeleteBreakpoint` | Delete a specific breakpoint |
| `DebuggerListen` | Long-poll listener — waits for debuggee to hit a breakpoint |
| `DebuggerAttach` | Attach to the stopped debuggee |
| `DebuggerDetach` | Clean detach from session |
| `DebuggerStep` | stepInto / stepOver / stepReturn / Continue / RunToLine / JumpToLine |
| `DebuggerGetStack` | Get current call stack |
| `DebuggerGetVariables` | Read variable values (supports `@ROOT` for top-level) |
| `CallRFC` | Call a function module via WebSocket to trigger ABAP code |
| `MoveObject` | Move object to a different package via WebSocket |

**Additional:** SAP GUI Terminal ID integration (`--terminal-id` / `SAP_TERMINAL_ID`) — allows breakpoints set by vsp to be hit by SAP GUI sessions.

**Files:** `pkg/adt/debugger.go`, `pkg/adt/websocket*.go`, `internal/mcp/handlers_debugger.go`

**Port effort:** High — requires WebSocket infrastructure + goroutine session management + ZADT_VSP ABAP plugin.

---

### 3.4 🔴 AMDP / HANA Debugger

**mcp-abap-adt has no HANA/SQLScript debugging at all.**

| Tool | Description |
|------|-------------|
| `AMDPDebuggerStart` | Start goroutine-based AMDP SQLScript debug session |
| `AMDPDebuggerResume` | Get AMDP session status |
| `AMDPDebuggerStop` | Stop session |
| `AMDPDebuggerStep` | Step through HANA SQLScript |
| `AMDPGetVariables` | Get HANA variable values during debugging |
| `AMDPSetBreakpoint` | Set AMDP breakpoint |
| `AMDPGetBreakpoints` | List AMDP breakpoints |

**Port effort:** Very High — experimental, HANA-specific, uses ZADT_VSP WebSocket.

---

### 3.5 🔴 ExecuteABAP — Arbitrary ABAP Code Execution

**mcp-abap-adt has no way to execute arbitrary ABAP code.**

vsp's `ExecuteABAP` wraps arbitrary ABAP source in a unit test class, deploys it to `$TMP`, runs it, captures output, and cleans up. This enables:
- On-demand ABAP scripting
- Verifying ABAP logic without creating permanent objects
- LLM-driven ABAP experiments

**Files:** `pkg/adt/devtools.go` (`ExecuteABAP`, `ExecuteABAPMultiple`)

**Port effort:** Medium — requires class create + unit test run + cleanup lifecycle.

---

### 3.6 🔴 Call Graph / Code Analysis

**mcp-abap-adt has no call graph or impact analysis beyond WhereUsed.**

| Tool | Description |
|------|-------------|
| `GetCallGraph` | Get call hierarchy with depth control and direction (up/down/both) |
| `GetCallersOf` | Simplified: who calls this object? |
| `GetCalleesOf` | Simplified: what does this object call? |
| `GetObjectStructure` | Object explorer tree structure |
| `AnalyzeCallGraph` | Graph statistics (nodes, edges, max depth) |
| `CompareCallGraphs` | Static graph vs actual execution trace comparison |
| `TraceExecution` | Composite RCA: static graph + unit tests + profiler + comparison |

**Files:** `pkg/adt/client.go` (GetCallGraph*), `internal/mcp/handlers_analysis.go`

**Port effort:** Medium — uses standard ADT endpoints, no custom ABAP needed.

---

### 3.7 🔴 Grep / Pattern Search Across Packages

**mcp-abap-adt's SearchObject only does name-pattern matching. vsp adds regex content search.**

| Tool | Description |
|------|-------------|
| `GrepObject` | Regex search in a single object's source |
| `GrepObjects` | Multi-object regex search (unified result) |
| `GrepPackage` | Regex search across an entire package |
| `GrepPackages` | Multi-package recursive regex search |

**Port effort:** Low — ADT has a search content endpoint; already used in vsp.

---

### 3.8 🔴 abapGit Export Integration

**mcp-abap-adt has no abapGit integration.**

| Tool | Description |
|------|-------------|
| `GitTypes` | List all 158 supported ABAP object types for export |
| `GitExport` | Export packages/objects to abapGit-compatible ZIP format |

Also: `InstallAbapGit` deploys the abapGit standalone version to a system.

**Files:** `pkg/adt/git.go`, `internal/mcp/handlers_git.go`

**Port effort:** Medium — requires WebSocket for reliable operation.

---

### 3.9 🔴 Install / Bootstrap Tools

**mcp-abap-adt has no tooling to install dependencies on the SAP system.**

| Tool | Description |
|------|-------------|
| `InstallZADTVSP` | Deploy ZADT_VSP WebSocket APC handler to SAP system |
| `InstallAbapGit` | Deploy abapGit (standalone or dev edition) |
| `ListDependencies` | List available server-side dependencies |
| `InstallDummyTest` | Test the install workflow |

**Port effort:** Medium — depends on how mcp-abap-adt handles ABAP source deployment.

---

### 3.10 🔴 Report Execution (SE38 / SUBMIT)

**mcp-abap-adt has no way to run ABAP programs/reports.**

| Tool | Description |
|------|-------------|
| `RunReport` | Execute program/report with parameters or variant |
| `RunReportAsync` | Background execution with polling |
| `GetVariants` | List report variants |
| `GetAsyncResult` | Retrieve async execution result |
| `GetTextElements` | Get program text elements (I18N) |
| `SetTextElements` | Set program text elements |

**Port effort:** Medium — ADT endpoints exist but require ZADT_VSP for async polling.

---

### 3.11 🔴 SQL Trace (ST05)

**mcp-abap-adt has no SQL trace access.**

| Tool | Description |
|------|-------------|
| `GetSQLTraceState` | Check if ST05 SQL trace is active |
| `ListSQLTraces` | List SQL trace files |

**Port effort:** Low — standard ADT endpoints.

---

### 3.12 🔴 Code Formatting (Pretty Printer)

**mcp-abap-adt has no code formatting support.**

| Tool | Description |
|------|-------------|
| `PrettyPrint` | Format ABAP source using the built-in formatter |
| `GetPrettyPrinterSettings` | Read current formatter config (indent width, etc.) |
| `SetPrettyPrinterSettings` | Update formatter configuration |

**Port effort:** Low — standard ADT endpoint.

---

### 3.13 🔴 Surgical Source Editing (EditSource)

**mcp-abap-adt requires full source replacement for any edit. vsp adds surgical string replacement.**

`EditSource` does:
- Regex or literal string search in current source
- Replace matched occurrence
- Optional syntax check before save
- Optional method-scope constraint
- Case-insensitive matching option

**Port effort:** Low — pure string manipulation + existing UpdateSource.

---

### 3.14 🔴 CDS FORWARD Dependency Analysis

**mcp-abap-adt has no CDS dependency graph traversal.**

`GetCDSDependencies` returns the forward dependency graph of a CDS view — what tables, views, and other CDS entities it reads from, recursively.

**Port effort:** Low — uses standard ADT CDS endpoint.

---

### 3.15 🔴 Unified Source Tools (GetSource / WriteSource)

**mcp-abap-adt requires knowing the object type first. vsp adds type-agnostic tools.**

| Tool | Description |
|------|-------------|
| `GetSource` | Get source for ANY object type by URI — no need to know the type |
| `WriteSource` | Write source for ANY object type — auto-detects, locks, writes, activates |

**Port effort:** Low — thin wrapper over existing per-type handlers.

---

### 3.16 🔴 File-Based Deployment (Bidirectional Sync)

**mcp-abap-adt has no local file ↔ SAP sync capability.**

| Tool | Description |
|------|-------------|
| `DeployFromFile` | Smart deploy — reads local `.abap` file, auto-detects create vs update |
| `SaveToFile` | Save SAP object source to local file |
| `ImportFromFile` | Alias for DeployFromFile |
| `ExportToFile` | Alias for SaveToFile |

**Port effort:** Low — filesystem I/O + existing read/write tools.

---

### 3.17 🔴 Object Clone, Rename & Move

**mcp-abap-adt has no object refactoring operations.**

| Tool | Description |
|------|-------------|
| `CloneObject` | Copy object to a new name (in same or different package) |
| `RenameObject` | Rename object (copy + delete workflow) |
| `MoveObject` | Move object to a different package (via WebSocket ZADT_VSP) |

**Port effort:** Medium — CloneObject/RenameObject are ADT-native; MoveObject needs ZADT_VSP.

---

### 3.18 🔴 Source Diff (CompareSource)

**mcp-abap-adt has no source comparison capability.**

`CompareSource` returns a unified diff between the sources of two ABAP objects — useful for reviewing changes before deployment or comparing implementations.

**Port effort:** Very low — pure string diff after reading two sources.

---

### 3.19 🔴 System Information Tools

| Tool | Description | mcp-abap-adt? |
|------|-------------|--------------|
| `GetSystemInfo` | SAP system ID, release, kernel, database type | ❌ Missing |
| `GetInstalledComponents` | Full list of installed software components + versions | ❌ Missing |
| `GetConnectionInfo` | Current MCP connection (user, URL, client) | ❌ Missing |
| `GetAbapHelp` | ABAP keyword documentation (links + optional system docs) | ❌ Missing |

**Port effort:** Low — standard ADT endpoints.

---

### 3.20 🔴 ATC Customizing

**mcp-abap-adt runs ATC checks but cannot inspect the ATC configuration.**

`GetATCCustomizing` retrieves:
- Available check variants
- Exemption reasons
- System-level ATC configuration

**Port effort:** Low.

---

### 3.21 🔴 Package Batch Activation

**mcp-abap-adt activates single objects. vsp adds package-level batch activation.**

`ActivatePackage` collects all inactive objects in a package, sorts by dependency, and activates them in the correct order. Essential for deploying entire packages.

**Port effort:** Medium — requires dependency resolution logic.

---

### 3.22 🔴 Cookie Authentication

**mcp-abap-adt only supports Basic, JWT, and Service Keys. vsp adds cookie-based auth.**

| Config | Description |
|--------|-------------|
| `--cookie-file` / `SAP_COOKIE_FILE` | Load cookies from Netscape-format `.txt` file |
| `--cookie-string` / `SAP_COOKIE_STRING` | Pass cookies as inline string |

Useful for SSO environments where login is browser-based.

**Files:** `pkg/adt/cookies.go`

**Port effort:** Low — cookie parsing + HTTP header injection.

---

### 3.23 🔴 Fluent DSL / API

**mcp-abap-adt exposes no programmatic API beyond MCP tools.**

vsp's `pkg/dsl` provides:
- **Search builder** — chainable `.Query().Classes().InPackage().Execute()`
- **Test orchestration** — parallel execution, risk levels, duration filtering
- **Batch import** — abapGit-compatible format with dependency-aware ordering
- **Batch export** — all class includes, dependency graph
- **Pipeline builders** — `DeployPipeline`, `RAPPipeline`, `ExportPipeline`

**Port effort:** Very High — internal SDK-level feature, not MCP-facing. Could be adapted as TypeScript library.

---

### 3.24 🔴 YAML Workflow Engine

**mcp-abap-adt has no workflow automation beyond individual tool calls.**

vsp's workflow engine allows declarative YAML definitions for complex multi-step ABAP operations (CI pipelines, deploy sequences, test runs).

**Port effort:** High — architectural feature, would need TypeScript equivalent.

---

### 3.25 🔴 Lua Scripting Engine

**mcp-abap-adt has no scripting capability.**

vsp's `pkg/scripting` provides:
- Interactive Lua REPL
- 40+ ADT operation bindings
- Scriptable automation for complex ADT workflows

**Port effort:** Very High — Go-specific (gopher-lua). TypeScript equivalent would use Node.js native scripting.

---

### 3.26 🔴 Multi-Layer Caching (SQLite)

**mcp-abap-adt has only in-memory object list cache.**

vsp's `pkg/cache` provides:
- Memory cache (hot data, TTL-based)
- SQLite persistent cache (survives restarts)
- Pluggable interface (Redis-ready)
- 16 unit tests

**Port effort:** Medium — TypeScript has good SQLite options (better-sqlite3).

---

### 3.27 🔴 Multi-System Profiles

**mcp-abap-adt requires separate process/config per system. vsp supports named profiles.**

`.vsp.json` allows storing multiple SAP system profiles:
```json
{
  "systems": {
    "dev": { "url": "http://dev:50000", "user": "developer" },
    "qas": { "url": "http://qas:50000", "user": "developer" }
  }
}
```

**Port effort:** Low — mcp-abap-adt's `--mcp=<destination>` partially covers this but differently.

---

### 3.28 🔴 Tool Group / Mode System

**mcp-abap-adt exposes all 272 tools always. vsp has a focused/expert mode + disable groups.**

| vsp Feature | Description |
|-------------|-------------|
| `--mode focused` | Expose 81 curated tools (default) |
| `--mode expert` | Expose all 122 tools |
| `--disabled-groups U,T,H,D,C` | Disable specific tool categories |

This keeps the LLM's tool window small and focused, reducing token overhead and confusion.

**Port effort:** Low — filter at tool registration time.

---

## 4. Features ONLY in mcp-abap-adt (Missing from vsp)

These are features in mcp-abap-adt that vsp does not have and should eventually gain.

---

### 4.1 🟦 Object Types with Full CRUD (not in vsp)

| Object Type | mcp-abap-adt | vsp |
|-------------|-------------|-----|
| **Domain (DOMA)** | ✅ Create/Read/Update/Delete + Low-level | ❌ Read only stub |
| **Data Element (DTEL)** | ✅ Create/Read/Update/Delete + Low-level | ❌ Read only stub |
| **Metadata Extension (DDLX)** | ✅ Create/Read/Update/Delete + Low-level | ❌ Not implemented |
| **Enhancement (ENHO/ENHS/ENHSPOT)** | ✅ Read + partial | ❌ Not implemented |
| **View (VIEW)** | ✅ Full CRUD | ⚠️ Read only |
| **Table (TABL)** | ✅ Full CRUD + Low-level | ⚠️ Create only (no update/delete) |

---

### 4.2 🟦 ABAP Language Analysis Tools

| Tool | Description | vsp |
|------|-------------|-----|
| `GetAbapAST` | Full Abstract Syntax Tree for source code | ❌ Missing |
| `GetAbapSemanticAnalysis` | Semantic analysis (type resolution, symbol binding) | ❌ Missing |
| `GetAbapSystemSymbols` | Resolve system symbols (SY-* fields) | ❌ Missing |
| `GetWhereUsed` | Impact analysis — where is this object used? | ❌ Missing |
| `GetAllTypes` | Discover all types in the system | ❌ Missing (GetTypeInfo is different) |

**These are extremely high-value language analysis tools missing from vsp.**

---

### 4.3 🟦 Object Discovery & Navigation

| Tool | Description | vsp |
|------|-------------|-----|
| `GetObjectsByType` | List all objects of a given type | ❌ Missing |
| `GetObjectsList` | General object listing with filters | ❌ Missing |
| `GetObjectInfo` | Object metadata (type, package, description, transport) | ❌ Missing |
| `GetObjectNodeFromCache` | Cached tree node access (fast re-query) | ❌ Missing |
| `GetVirtualFoldersLow` | Virtual folder tree navigation | ❌ Missing |
| `GetNodeStructureLow` | Low-level node structure query | ❌ Missing |
| `DescribeByList` | Batch metadata retrieval for multiple objects | ❌ Missing |
| `GetPackageContents` | Package contents listing | ⚠️ GetPackage is different |
| `GetPackageTree` | Full recursive package tree navigation | ❌ Missing |
| `GetIncludesList` | Recursive include dependency list | ❌ Missing |
| `GetProgFullCode` | Get program with all includes expanded inline | ❌ Missing |

---

### 4.4 🟦 Runtime Profiling (More Advanced)

mcp-abap-adt has a more complete profiling API than vsp:

| Tool | mcp-abap-adt | vsp |
|------|-------------|-----|
| `RuntimeRunClassWithProfiling` | Execute class with profiler attached | ❌ Missing |
| `RuntimeRunProgramWithProfiling` | Execute program with profiler attached | ❌ Missing |
| `RuntimeCreateProfilerTraceParameters` | Configure profiler before run | ❌ Missing |
| `RuntimeGetProfilerTraceData` | Get profiler trace raw data | ⚠️ GetTrace is similar |
| `RuntimeAnalyzeProfilerTrace` | Analyze profiler output | ❌ Missing |
| `RuntimeListProfilerTraceFiles` | List trace files | ⚠️ ListTraces is similar |
| `RuntimeAnalyzeDump` | Analyze dump (structured) | ⚠️ GetDump is similar |

---

### 4.5 🟦 Multiple MCP Transports

**mcp-abap-adt supports HTTP and SSE in addition to stdio. vsp is stdio-only.**

| Transport | mcp-abap-adt | vsp |
|-----------|-------------|-----|
| stdio | ✅ | ✅ |
| Streamable HTTP | ✅ (`--http`) | ❌ |
| SSE (Server-Sent Events) | ✅ (`--sse`) | ❌ |
| Docker / containerized | ✅ | ❌ (binary only) |

HTTP/SSE transports enable:
- Web-based MCP clients
- Central shared MCP server deployment
- Multi-user server scenarios

---

### 4.6 🟦 Enterprise Authentication

**vsp only supports Basic auth and cookies. mcp-abap-adt adds enterprise auth.**

| Auth Method | mcp-abap-adt | vsp |
|-------------|-------------|-----|
| Basic (user/pass) | ✅ | ✅ |
| JWT / XSUAA tokens | ✅ | ❌ |
| Service Keys (auth-broker) | ✅ | ❌ |
| Per-request headers (HTTP/SSE) | ✅ | ❌ |
| RFC for legacy systems | ✅ (v4.0) | ❌ |
| Cookie file | ❌ | ✅ |

JWT/XSUAA is essential for ABAP Cloud (BTP) systems.

---

### 4.7 🟦 Legacy System Support (RFC, BASIS < 7.50)

**vsp targets modern ABAP only. mcp-abap-adt v4.0 adds RFC for old systems.**

- RFC connection type via `node-rfc` SDK
- `available_in: ["legacy"]` tool filtering
- 102 legacy-compatible handlers
- Supports BASIS versions before 7.50

---

### 4.8 🟦 Low-Level Per-Object Operations

mcp-abap-adt exposes 122 low-level tools with granular per-object-type control:

- **Per-type Lock/Unlock** — e.g., `LockClassLow`, `LockProgramLow`, `LockDomainLow`
- **Per-type Check** — e.g., `CheckClassLow`, `CheckDomainLow`
- **Per-type Validate** — e.g., `ValidateBehaviorDefinitionLow`
- **Per-type Activate** — e.g., `ActivateClassLow`, `ActivateFunctionGroupLow`

vsp only has generic `LockObject`/`UnlockObject`/`Activate`.

---

### 4.9 🟦 CDS / Unit Test Lifecycle Management

mcp-abap-adt has full unit test and CDS test lifecycle:

| Tool Group | mcp-abap-adt | vsp |
|------------|-------------|-----|
| Create/Update/Delete ABAP Unit test include | ✅ | ⚠️ CreateTestInclude only |
| CDS Unit Tests (Create/Get/Run/Status/Result) | ✅ | ❌ Missing |

---

### 4.10 🟦 System Type Awareness

**mcp-abap-adt differentiates on-prem vs ABAP Cloud vs legacy at tool-registration time.**

- `available_in: ["onprem", "cloud", "legacy"]` in tool definitions
- Automatic filtering — cloud-only tools hidden on on-prem systems
- On-prem: `masterSystem` + `responsible` resolution for correct transport binding

vsp has no concept of ABAP Cloud vs on-prem differentiation.

---

### 4.11 🟦 Session Information

`GetSession` returns:
- Current session details (user, client, language)
- System info (system ID, release)
- Connected status

vsp's `GetConnectionInfo` is similar but lighter.

---

### 4.12 🟦 Service Binding: Validation + Type Listing

mcp-abap-adt extends service binding operations:

| Tool | mcp-abap-adt | vsp |
|------|-------------|-----|
| `ValidateServiceBinding` | Validate before publish | ❌ Missing |
| `ListServiceBindingTypes` | List available binding types | ❌ Missing |

vsp only has `PublishServiceBinding` / `UnpublishServiceBinding`.

---

### 4.13 🟦 Function Group / Module: Separate Update

mcp-abap-adt has dedicated `UpdateFunctionGroup` and `UpdateFunctionModule` tools.
vsp bundles all updates into `UpdateSource`.

---

### 4.14 🟦 Individual Include Deletion

mcp-abap-adt allows deleting individual class includes:
- `DeleteLocalDefinitions`
- `DeleteLocalTypes`
- `DeleteLocalMacros`
- `DeleteLocalTestClass`

vsp only supports updating includes, not deleting them individually.

---

## 5. Features in Both Repos

Both repos implement these features, though with different APIs and quality levels:

| Feature | vsp | mcp-abap-adt | Notes |
|---------|-----|-------------|-------|
| Program CRUD | ✅ | ✅ | vsp uses unified tools; fr0ster has per-operation tools |
| Class CRUD (main) | ✅ | ✅ | Similar |
| Interface CRUD | ✅ | ✅ | Similar |
| Function Module (Read) | ✅ | ✅ | Both read FM source |
| Function Group (Read) | ✅ | ✅ | Similar |
| CDS / DDLS | ✅ | ✅ | vsp adds dependency analysis |
| BDEF (Read+Write) | ✅ | ✅ | Similar |
| Service Definition (SRVD) | ✅ | ✅ | Similar |
| Service Binding (SRVB) | ✅ | ✅ | fr0ster adds validate + list types |
| Package CRUD | ✅ | ✅ | Similar |
| Table (Read+Create) | ✅ | ✅ | fr0ster adds Update/Delete |
| Structure (Read) | ✅ | ✅ | Similar |
| View (Read) | ✅ | ✅ | fr0ster adds CRUD |
| Transport (Read) | ✅ | ✅ | Similar |
| Transport Create | ✅ | ✅ | Similar |
| Search Objects | ✅ | ✅ | vsp adds regex grep |
| Run Unit Tests (ABAP) | ✅ | ✅ | vsp adds parallel + risk level |
| ATC Checks | ✅ | ✅ | Similar |
| Runtime Errors (Dumps) | ✅ | ✅ | Different API depth |
| Profiler Traces | ✅ | ✅ | fr0ster has more profiling options |
| Table Contents | ✅ | ✅ | Similar |
| SQL Query | ✅ | ✅ | Both support ad-hoc SQL |
| Code Completion | ✅ | ⚠️ | vsp has it; fr0ster via GetAbapSemanticAnalysis |
| Type Hierarchy | ✅ | ⚠️ | vsp dedicated tool; fr0ster via GetAbapAST |
| Find Definition | ✅ | ⚠️ | vsp dedicated tool; fr0ster via semantic analysis |
| Find References | ✅ | ⚠️ | vsp dedicated tool; fr0ster has GetWhereUsed |
| Class Includes (Get/Update) | ✅ | ✅ | Both; fr0ster also has Delete |
| Syntax Check | ✅ | ✅ | Both; fr0ster per-type (CheckProgramLow etc.) |
| Activation (single) | ✅ | ✅ | fr0ster per-type; vsp generic |
| Lock / Unlock | ✅ | ✅ | fr0ster per-type; vsp generic |
| Publish/Unpublish OData | ✅ | ✅ | fr0ster adds validate |
| GetTransaction | ✅ | ✅ | Similar |
| GetTypeInfo | ✅ | ✅ | fr0ster has more type analysis depth |
| Include (Read) | ✅ | ✅ | fr0ster adds GetIncludesList |
| Documentation | ✅ | ✅ | Different style |
| Unit Tests | 244 | 244+ | Both well-tested |
| Integration Tests | 34 | 34+ | Both |

---

## 6. Tool-by-Tool Matrix

### 6.1 vsp-Unique Tools (not in mcp-abap-adt)

| # | Tool | Category | Port Priority |
|---|------|----------|--------------|
| 1 | Safety System (ReadOnly, BlockSQL, etc.) | Safety | 🔴 HIGH |
| 2 | GetFeatures | Feature Detection | 🔴 HIGH |
| 3 | GetConnectionInfo | System Info | 🟡 MEDIUM |
| 4 | GetSystemInfo | System Info | 🟡 MEDIUM |
| 5 | GetInstalledComponents | System Info | 🟡 MEDIUM |
| 6 | GetAbapHelp | Documentation | 🟢 LOW |
| 7 | GetSource (unified) | Read | 🟡 MEDIUM |
| 8 | WriteSource (unified) | Write | 🟡 MEDIUM |
| 9 | GetMessages | Read | 🟢 LOW |
| 10 | GetCDSDependencies | CDS Analysis | 🟡 MEDIUM |
| 11 | GetCallGraph | Code Analysis | 🔴 HIGH |
| 12 | GetCallersOf | Code Analysis | 🔴 HIGH |
| 13 | GetCalleesOf | Code Analysis | 🔴 HIGH |
| 14 | AnalyzeCallGraph | Code Analysis | 🟡 MEDIUM |
| 15 | CompareCallGraphs | Code Analysis | 🟡 MEDIUM |
| 16 | TraceExecution | Code Analysis | 🟡 MEDIUM |
| 17 | GetSQLTraceState | SQL Trace | 🟡 MEDIUM |
| 18 | ListSQLTraces | SQL Trace | 🟡 MEDIUM |
| 19 | SetBreakpoint | ABAP Debugger | 🔴 HIGH |
| 20 | GetBreakpoints | ABAP Debugger | 🔴 HIGH |
| 21 | DeleteBreakpoint | ABAP Debugger | 🔴 HIGH |
| 22 | DebuggerListen | ABAP Debugger | 🔴 HIGH |
| 23 | DebuggerAttach | ABAP Debugger | 🔴 HIGH |
| 24 | DebuggerDetach | ABAP Debugger | 🔴 HIGH |
| 25 | DebuggerStep | ABAP Debugger | 🔴 HIGH |
| 26 | DebuggerGetStack | ABAP Debugger | 🔴 HIGH |
| 27 | DebuggerGetVariables | ABAP Debugger | 🔴 HIGH |
| 28 | AMDPDebuggerStart | AMDP Debugger | 🟢 LOW |
| 29 | AMDPDebuggerResume | AMDP Debugger | 🟢 LOW |
| 30 | AMDPDebuggerStop | AMDP Debugger | 🟢 LOW |
| 31 | AMDPDebuggerStep | AMDP Debugger | 🟢 LOW |
| 32 | AMDPGetVariables | AMDP Debugger | 🟢 LOW |
| 33 | AMDPSetBreakpoint | AMDP Debugger | 🟢 LOW |
| 34 | AMDPGetBreakpoints | AMDP Debugger | 🟢 LOW |
| 35 | CallRFC | WebSocket RFC | 🟡 MEDIUM |
| 36 | MoveObject | Object Ops | 🟡 MEDIUM |
| 37 | GrepObject | Search | 🔴 HIGH |
| 38 | GrepObjects | Search | 🔴 HIGH |
| 39 | GrepPackage | Search | 🔴 HIGH |
| 40 | GrepPackages | Search | 🔴 HIGH |
| 41 | EditSource | Editing | 🔴 HIGH |
| 42 | CompareSource | Editing | 🟡 MEDIUM |
| 43 | CloneObject | Refactoring | 🟡 MEDIUM |
| 44 | RenameObject | Refactoring | 🟡 MEDIUM |
| 45 | GetClassInfo | Code Intel | 🟡 MEDIUM |
| 46 | PrettyPrint | Formatting | 🟡 MEDIUM |
| 47 | GetPrettyPrinterSettings | Formatting | 🟢 LOW |
| 48 | SetPrettyPrinterSettings | Formatting | 🟢 LOW |
| 49 | GetATCCustomizing | ATC | 🟢 LOW |
| 50 | ActivatePackage | Activation | 🔴 HIGH |
| 51 | DeployFromFile | File Deploy | 🟡 MEDIUM |
| 52 | SaveToFile | File Deploy | 🟡 MEDIUM |
| 53 | ImportFromFile | File Deploy | 🟡 MEDIUM |
| 54 | ExportToFile | File Deploy | 🟡 MEDIUM |
| 55 | ExecuteABAP | Code Execution | 🔴 HIGH |
| 56 | WriteProgram | Workflow | 🟡 MEDIUM |
| 57 | WriteClass | Workflow | 🟡 MEDIUM |
| 58 | CreateAndActivateProgram | Workflow | 🟡 MEDIUM |
| 59 | CreateClassWithTests | Workflow | 🟡 MEDIUM |
| 60 | GitTypes | abapGit | 🟡 MEDIUM |
| 61 | GitExport | abapGit | 🟡 MEDIUM |
| 62 | RunReport | Reports | 🟡 MEDIUM |
| 63 | RunReportAsync | Reports | 🟡 MEDIUM |
| 64 | GetVariants | Reports | 🟡 MEDIUM |
| 65 | GetAsyncResult | Reports | 🟡 MEDIUM |
| 66 | GetTextElements | Reports | 🟢 LOW |
| 67 | SetTextElements | Reports | 🟢 LOW |
| 68 | InstallZADTVSP | Install | 🔴 HIGH |
| 69 | InstallAbapGit | Install | 🟡 MEDIUM |
| 70 | ListDependencies | Install | 🟡 MEDIUM |
| 71 | UI5ListApps | UI5 | 🟡 MEDIUM |
| 72 | UI5GetApp | UI5 | 🟡 MEDIUM |
| 73 | UI5GetFileContent | UI5 | 🟡 MEDIUM |
| 74 | UI5UploadFile | UI5 | 🟡 MEDIUM |
| 75 | UI5DeleteFile | UI5 | 🟡 MEDIUM |
| 76 | UI5CreateApp | UI5 | 🟡 MEDIUM |
| 77 | UI5DeleteApp | UI5 | 🟡 MEDIUM |
| 78 | Cookie Auth (--cookie-file/string) | Auth | 🟡 MEDIUM |
| 79 | Mode System (focused/expert) | Config | 🔴 HIGH |
| 80 | Tool Groups (--disabled-groups) | Config | 🔴 HIGH |
| 81 | Safety flags (--read-only etc.) | Safety | 🔴 HIGH |
| 82 | SAP GUI Terminal ID integration | Debugger | 🟢 LOW |

### 6.2 mcp-abap-adt-Unique Tools (not in vsp)

| # | Tool | Category | Add to vsp? |
|---|------|----------|------------|
| 1 | GetAbapAST | Language Analysis | 🔴 HIGH |
| 2 | GetAbapSemanticAnalysis | Language Analysis | 🔴 HIGH |
| 3 | GetAbapSystemSymbols | Language Analysis | 🟡 MEDIUM |
| 4 | GetWhereUsed | Impact Analysis | 🔴 HIGH |
| 5 | GetAllTypes | Type Discovery | 🟡 MEDIUM |
| 6 | GetObjectsByType | Object Discovery | 🔴 HIGH |
| 7 | GetObjectsList | Object Discovery | 🔴 HIGH |
| 8 | GetObjectInfo | Object Metadata | 🔴 HIGH |
| 9 | GetObjectNodeFromCache | Caching | 🟡 MEDIUM |
| 10 | GetVirtualFolders | Tree Nav | 🟢 LOW |
| 11 | GetNodeStructure | Tree Nav | 🟢 LOW |
| 12 | DescribeByList | Batch Metadata | 🟡 MEDIUM |
| 13 | GetPackageContents | Package Nav | 🟡 MEDIUM |
| 14 | GetPackageTree | Package Nav | 🟡 MEDIUM |
| 15 | GetIncludesList | Include Analysis | 🟡 MEDIUM |
| 16 | GetProgFullCode | Program Analysis | 🟡 MEDIUM |
| 17 | GetEnhancementImpl | Enhancements | 🟡 MEDIUM |
| 18 | GetEnhancements | Enhancements | 🟡 MEDIUM |
| 19 | GetEnhancementSpot | Enhancements | 🟡 MEDIUM |
| 20 | CreateDomain / UpdateDomain / DeleteDomain | DDIC | 🟡 MEDIUM |
| 21 | CreateDataElement / UpdateDataElement / DeleteDataElement | DDIC | 🟡 MEDIUM |
| 22 | CreateMetadataExtension / Update / Delete | DDLX | 🟡 MEDIUM |
| 23 | UpdateView + CreateView + DeleteView | DDIC | 🟡 MEDIUM |
| 24 | UpdateTable + DeleteTable | DDIC | 🟡 MEDIUM |
| 25 | ValidateServiceBinding | SRVB | 🟢 LOW |
| 26 | ListServiceBindingTypes | SRVB | 🟢 LOW |
| 27 | DeleteLocalDefinitions/Types/Macros/TestClass | Class Includes | 🟢 LOW |
| 28 | CreateUnitTest / UpdateUnitTest / DeleteUnitTest | Unit Tests | 🟡 MEDIUM |
| 29 | CDS Unit Test full CRUD + Run + Result + Status | CDS Tests | 🔴 HIGH |
| 30 | RuntimeRunClassWithProfiling | Profiling | 🟡 MEDIUM |
| 31 | RuntimeRunProgramWithProfiling | Profiling | 🟡 MEDIUM |
| 32 | RuntimeCreateProfilerTraceParameters | Profiling | 🟡 MEDIUM |
| 33 | RuntimeAnalyzeProfilerTrace | Profiling | 🟡 MEDIUM |
| 34 | GetSession | System | 🟢 LOW |
| 35 | HTTP/SSE transport modes | Infrastructure | 🔴 HIGH |
| 36 | JWT/XSUAA authentication | Auth | 🔴 HIGH |
| 37 | RFC connection for legacy systems | Legacy | 🟡 MEDIUM |
| 38 | System type awareness (onprem/cloud/legacy) | Config | 🔴 HIGH |
| 39 | Per-object-type Low-level tools (122 tools) | Low-level | 🟡 MEDIUM |
| 40 | YAML server configuration file | Config | 🟡 MEDIUM |
| 41 | UpdateFunctionGroup / UpdateFunctionModule | Functions | 🟡 MEDIUM |
| 42 | GetAdtTypes | Metadata | 🟢 LOW |

---

## 7. Merge Strategy Recommendations

### 7.1 Recommended Approach

The goal is to merge vsp's capabilities into mcp-abap-adt (TypeScript). The reverse (porting mcp-abap-adt's features to Go) would waste the large TypeScript investment already made.

**Strategy:** Port vsp's unique features to TypeScript as new handler groups in mcp-abap-adt.

---

### 7.2 Priority Tiers for Porting

#### Tier 1 — High Value, Low Risk (port first)

1. **Safety System** — Middleware interceptor in `BaseHandlerGroup`; blocks writes, enforces package restrictions
2. **Feature Detection** — HTTP probes at startup; `GetFeatures` tool
3. **Grep Operations** — 4 new tools; uses existing ADT search endpoint
4. **EditSource** — Surgical string replacement; uses existing read/write tools
5. **Call Graph** — 6 tools; standard ADT endpoints
6. **GetSystemInfo + GetInstalledComponents** — Standard ADT endpoints
7. **ActivatePackage** — Batch activate with dependency sort
8. **ExecuteABAP** — Unit test wrapper lifecycle
9. **Mode / Tool Group System** — Filter at registration; configuration-driven
10. **Cookie Authentication** — Add to existing auth providers

#### Tier 2 — High Value, Moderate Effort

11. **ABAP External Debugger** — Needs WebSocket infrastructure + ZADT_VSP
12. **CDS Dependency Analysis** — Standard ADT endpoint
13. **SQL Trace Tools** — Standard ADT endpoints
14. **Pretty Printer** — Standard ADT endpoints
15. **CompareSource** — String diff, no new ADT calls
16. **Clone/Rename/Move** — Uses existing create/delete
17. **File-Based Deployment** — Filesystem I/O + existing tools
18. **Report Execution** — Needs ZADT_VSP for async
19. **UI5/BSP Management** — vsp read ops + vsp write ops (need ZADT_VSP)
20. **abapGit Export** — Needs WebSocket for reliability
21. **Install Tools** — Bootstrap infrastructure

#### Tier 3 — Experimental / Complex

22. **AMDP/HANA Debugger** — Very experimental; needs ZADT_VSP
23. **Lua Scripting** — TypeScript alternative: JS scripting
24. **YAML Workflow Engine** — TypeScript equivalent possible
25. **Multi-layer Cache** — SQLite in TypeScript (better-sqlite3)
26. **DSL/Fluent API** — TypeScript library-level feature

---

### 7.3 What mcp-abap-adt Adds to the Merged Server

When merging, these mcp-abap-adt capabilities should be preserved (they don't exist in vsp):

- JWT/XSUAA auth (ABAP Cloud systems)
- RFC connection type (legacy BASIS < 7.50)
- System type awareness (onprem / cloud / legacy)
- HTTP + SSE transports
- `GetAbapAST` + `GetAbapSemanticAnalysis` + `GetWhereUsed`
- Domain / DataElement / DDLX full CRUD
- Enhancement framework reading
- Low-level per-object-type operations
- CDS Unit Tests
- `GetObjectsByType`, `GetObjectsList`, `GetObjectInfo`
- Package tree navigation

---

### 7.4 Architecture for the Merge

**Recommended new handler groups to add to mcp-abap-adt:**

```
src/handlers/
├── safety/              # Safety middleware (NEW — port from vsp)
│   └── safetyMiddleware.ts
├── features/            # Feature detection (NEW — port from vsp)
│   └── handleGetFeatures.ts
├── grep/                # Grep search (NEW — port from vsp)
│   ├── handleGrepObject.ts
│   ├── handleGrepPackage.ts
│   └── handleGrepPackages.ts
├── debugger/            # ABAP External Debugger (NEW — port from vsp)
│   ├── handleSetBreakpoint.ts
│   ├── handleDebuggerListen.ts
│   ├── handleDebuggerStep.ts
│   └── ...
├── code_analysis/       # Call graph + CDS deps (NEW — port from vsp)
│   ├── handleGetCallGraph.ts
│   └── handleGetCDSDependencies.ts
├── execution/           # ExecuteABAP + reports (NEW — port from vsp)
│   ├── handleExecuteAbap.ts
│   └── handleRunReport.ts
├── editing/             # EditSource, Compare, Clone (NEW — port from vsp)
│   ├── handleEditSource.ts
│   ├── handleCompareSource.ts
│   └── handleCloneObject.ts
├── formatting/          # Pretty printer (NEW — port from vsp)
│   └── handlePrettyPrint.ts
├── ui5/                 # UI5/Fiori BSP (NEW — port from vsp)
│   └── ...
└── abapgit/             # abapGit export (NEW — port from vsp)
    └── handleGitExport.ts
```

---

*Report created: 2026-03-13*
*Based on deep analysis of:*
- `/Users/marianzeis/DEV/vibing-steampunk` (vsp — Go, 122 tools)
- `/Users/marianzeis/DEV/mcp-abap-adt-fr0ster` (mcp-abap-adt — TypeScript, 272 tools)*
