# Vibing Steampunk (vsp)

**AI-Agentic Development Unlocked for ABAP** — ECC, S/4HANA, everywhere ADT is available.

> **ADT ↔ MCP Bridge**: Gives Claude (and other AI assistants) full access to SAP ADT APIs.
> Read code, write code, debug, deploy, run tests — all through natural language (or DSL for automation).
>
> See also: [OData ↔ MCP Bridge](https://github.com/oisee/odata_mcp_go) for SAP data access.

![Vibing ABAP Developer](./media/vibing-steampunk.png)

## What's New

**v2.25.0** - CreatePackage Software Component Support
- **`software_component` Parameter**: Create transportable packages with proper software component (e.g., `HOME`, `ZLOCAL`)
- **Viper Env Fix**: Comma-separated env vars (`SAP_ALLOWED_PACKAGES`, `SAP_ALLOWED_TRANSPORTS`) now parse correctly
- **Closes #18**: Namespace URL encoding verified working
- ⚠️ **Requires**: `--allow-transportable-edits` or `--enable-transports` flag for transportable packages

**v2.24.0** - Transportable Edits Safety Feature
- **Safety by Default**: Editing objects in transportable packages blocked unless explicitly enabled
- **`--allow-transportable-edits`**: Opt-in flag to enable editing non-local objects
- **Transport Whitelisting**: `--allowed-transports "A4HK*,DEVK*"` restricts allowed TRs
- **Package Whitelisting**: `--allowed-packages "Z*,$TMP"` restricts editable packages
- **Clear Error Messages**: Explains why edits are blocked and how to enable
- **Transport Tool Visibility**: `ListTransports`/`GetTransport` visible when flag enabled
- See [Transportable Edits Report](reports/2026-02-03-002-transportable-edits-safety-feature.md)

**v2.23.0** - GitExport to Disk & GetAbapHelp via WebSocket
- **GitExport saves ZIP to disk**: No more base64 - files written directly to `output_dir`
- **GetAbapHelp via WebSocket**: Real SAP documentation from system (uses ZADT_VSP if connected)
- **Lazy WebSocket Connection**: WebSocket connects on-demand for Git/Help operations
- **Embedded ABAP Sync**: Updated all ZADT_VSP classes from SAP system (2026-02-02)

**v2.22.0** - SAP GUI Debugger Integration
- **SAP_TERMINAL_ID**: Share breakpoints between vsp and SAP GUI sessions
- **MoveObject Tool**: Move ABAP objects between packages (via ZADT_VSP WebSocket)
- **RunReport Refactor**: Uses background jobs with spool output - more reliable execution
- **Cross-Tool Debugging**: Set breakpoint in vsp, hit it in SAP GUI (or vice versa)
- Configure: `--terminal-id <id>` or `SAP_TERMINAL_ID=<id>` (get ID from SAP GUI registry/file)

**v2.21.0** - Method-Level Source Operations
- **GetSource with `method`**: Returns only the `METHOD...ENDMETHOD` block (~50 lines vs 1000+)
- **EditSource with `method`**: Constrains find/replace to specific method - no accidental edits elsewhere
- **WriteSource with `method`**: Replace only one method implementation (method must exist)
- **95% Token Reduction**: Work with individual methods instead of entire classes
- **AI-Optimized**: Faster responses, more precise edits, safer refactoring
- See [Method-Level Operations Report](reports/2026-01-06-001-method-level-source-operations.md)

**v2.20.0** - CLI Mode & Multi-System Management
- **CLI Subcommands**: `vsp search`, `vsp source`, `vsp export`, `vsp systems` - direct ABAP operations without MCP
- **System Profiles**: `.vsp.json` config file for managing multiple SAP systems
- **Config Conversion**: `vsp config mcp-to-vsp` / `vsp-to-mcp` - convert between config formats
- **Cookie Auth in Profiles**: Support `cookie_file` / `cookie_string` for SSO environments
- **Password Sync**: Passwords imported from `.mcp.json` env blocks automatically
- **GitExport FULL Mode**: Multi-package exports with proper folder hierarchy (`src/$package/`)
- **abapGit Compatible**: Exports now include `.abapgit.xml` with FULL folder logic

**v2.19.0** - Async Execution & Developer Productivity
- **RunReportAsync**: Execute reports in background goroutine - no more timeouts!
- **GetAsyncResult**: Poll or wait for async task completion (up to 60s)
- **CompareSource**: Unified diff between any two ABAP objects (LCS algorithm)
- **CloneObject**: Copy PROG/CLAS/INTF to new name with auto-replace
- **GetClassInfo**: Quick class metadata via CAI (methods, attrs, interfaces)
- **CreateTable**: Create DDIC tables from simple JSON definition
- **GetSystemInfo**: Fixed to use SQL (CVERS/T000) - works across all SAP versions
- **54 Focused / 99 Expert Tools** - see [Release Notes](reports/2026-01-05-001-v2.19.0-release-notes.md)

**v2.18.1** - Interactive CLI Debugger
- **`vsp debug` Command**: Standalone interactive ABAP debugger
- **REPL Interface**: s=step, n=next, o=out, c=continue, r=stack, v=vars
- **WebSocket Breakpoints**: Set/delete/list via ZADT_VSP
- **Attach Mode**: Wait for any debuggee or filter by user
- **Graceful Handling**: Ctrl+C cleanup, session state management
- Phase 1 of DAP implementation - see [DAP Plan](reports/2026-01-03-001-dap-implementation-plan.md)

**v2.18.0** - Report Execution Tools
- **RunReport**: Execute selection-screen reports with params/variants, capture ALV output
- **GetVariants**: List available variants for a report
- **GetTextElements**: Read selection texts and text symbols
- **SetTextElements**: Update selection texts and text symbols programmatically
- **Tool Group "R"**: Disable report tools with `--disabled-groups R`
- **ZADT_VSP v2.3.0**: New "report" domain for WebSocket-based report execution
- See [Report Design](reports/2026-01-02-002-run-report-text-elements-design.md)

**v2.17.0** - Install Tools & One-Command Deployment
- **InstallZADTVSP**: Deploy WebSocket handler to SAP in one command
- **InstallAbapGit**: Deploy abapGit standalone or dev edition
- **ListDependencies**: Show available installable packages
- **Upsert Logic**: Proper CREATE vs UPDATE detection for all objects
- Embedded ABAP source in binary - no external files needed

**v2.16.0** - abapGit WebSocket Integration
- **GitTypes Tool**: Query 158 supported abapGit object types
- **GitExport Tool**: Export packages/objects as abapGit-compatible ZIP (base64)
- **WebSocket Transport**: Via ZADT_VSP handler (domain: "git")
- **abapGit Serialization**: Uses native `ZCL_ABAPGIT_OBJECTS=>serialize()` for full compatibility
- **Tool Group "G"**: Disable Git tools with `--disabled-groups G`
- **Requires**: abapGit installed on SAP system
- See [abapGit Integration Report](reports/2025-12-23-002-abapgit-websocket-integration-complete.md)

**v2.15.0** - Phase 5 TAS-Style Debugging Complete
- **Variable History Recording**: Track all variable changes during debug sessions
- **Extended Breakpoint Types**: Statement, exception, and watchpoint scripting
- **Force Replay / State Injection**: THE KILLER FEATURE - inject captured state into live debug sessions
- **WebSocket Debugging**: Full TPDAPI integration via ZADT_VSP WebSocket handler
- **AMDP Debugging**: Experimental support (session works, breakpoint triggering under investigation)
- See [Observations Since v2.12.5](reports/2025-12-22-observations-since-v2.12.5.md) for complete changelog

**v2.14.0** - Lua Scripting Integration (Phase 5)
- **`vsp lua` Command**: Interactive REPL and script execution
- **40+ Lua Bindings**: All MCP tools accessible from Lua (searchObject, getSource, setBreakpoint, etc.)
- **Debug Session Management**: listen, attach, stepOver, stepInto, stepReturn, continue, getStack
- **Checkpoint System**: saveCheckpoint, getCheckpoint, listCheckpoints, injectCheckpoint (Force Replay foundation)
- **Example Scripts**: search-and-grep, call-graph-analysis, debug-session, analyze-dumps
- See [TAS & RCA Vision](VISION.md) and [Roadmap](ROADMAP.md) for Phase 5-8 plans

**v2.13.0** - Call Graph & RCA Tools
- **GetCallersOf/GetCalleesOf**: Navigate call graphs up (who calls) and down (what's called)
- **TraceExecution**: Composite RCA tool - static graph + trace + comparison in one call
- **CompareCallGraphs**: Find untested paths (static only) and dynamic calls (actual only)
- **AnalyzeCallGraph**: Statistics on nodes, edges, depth, types
- **WebSocket Debugging**: Full TPDAPI integration for statement/exception breakpoints

**v2.12.5** - EditSource Line Ending Fix
- **CRLF→LF Normalization**: EditSource now works reliably across platforms
- SAP ADT returns `\r\n`, AI sends `\n` - now automatically normalized
- No more "old_string not found" errors due to line ending mismatches

**v2.12.4** - Feature Detection & Safety Network
- **GetFeatures Tool**: Probe SAP system for available optional capabilities
- **Feature Flags**: `--feature-abapgit`, `--feature-rap`, `--feature-amdp`, `--feature-ui5`, `--feature-transport`
- **Feature Modes**: `auto` (probe system), `on` (force enable), `off` (force disable)
- **SRVB WriteSource**: Create Service Bindings via unified WriteSource tool
- **BDEF Improvements**: Fixed behavior definition creation flow

**v2.12.0** - abapGit-Compatible Format & Batch Operations
- **Class Includes**: Import/export `.clas.testclasses.abap`, `.clas.locals_def.abap`, `.clas.locals_imp.abap`
- **Batch Import DSL**: `dsl.Import(client).FromDirectory("./src/").RAPOrder().Execute(ctx)`
- **Batch Export DSL**: `dsl.Export(client).Classes("ZCL_*").ToDirectory("./backup/").Execute(ctx)`
- **Pipeline Builder**: `dsl.RAPPipeline(client, "./src/", "$PKG", "ZSRV_BINDING")` - full RAP deployment
- **Priority Control**: `DDLSFirst()`, `RAPOrder()`, `CustomOrder()` for dependency handling

**v2.11.0** - Transport Management & Safety Controls
- **5 Transport Tools**: ListTransports, GetTransport, CreateTransport, ReleaseTransport, DeleteTransport
- **Safety Controls**: `--enable-transports`, `--allowed-transports "A4HK*"`, `--allowed-packages "Z*"`
- **Tool Group "C"**: Disable all CTS tools with `--disabled-groups C`
- Enterprise-grade transport governance for AI assistants
- See also v2.24.0 for `--allow-transportable-edits` safety feature

**v2.10.0** - UI5/BSP Management & Tool Groups
- **7 UI5/BSP Tools**: List apps, read files, search content, view manifests
- **AMDP Debugger**: 5 tools for HANA stored procedure debugging
- **Tool Groups**: Selectively disable features (`--disabled-groups 5THD`)
- **94 Total Tools**: 46 focused mode, 94 expert mode

**v2.8.0** - Full Debug Session Support
- **DebuggerAttach/Detach** - Attach to caught debuggees, release sessions
- **DebuggerGetStack** - View call stack with program/line info
- **DebuggerGetVariables** - Inspect variable values during debugging
- **DebuggerStep** - Step into/over/return/continue through code
- Complete AI-powered debugging: breakpoint → listen → attach → inspect → step

**v2.7.0** - External Debugger & Listener
- Set external breakpoints (line, exception, statement, message)
- Long-polling debug listener for autonomous debugging
- Foundation for AI-powered debugger scripting

**v2.6.0** - RAP OData E2E Support
- Create CDS views, Service Definitions, and Service Bindings
- Publish OData V2/V4 services directly from AI assistant
- Full RAP development lifecycle via MCP tools

---

**Single binary** with **52 focused tools** (default) or **99 expert tools** for AI-assisted ABAP development.

## Key Features

| Feature | Description |
|---------|-------------|
| **AI Debugger** | Breakpoints, listener, attach, step, inspect stack & variables |
| **RAP OData E2E** | Create CDS views, Service Definitions, Bindings → Publish OData services |
| **Focused Mode** | 37 curated tools optimized for AI assistants (50% token reduction) |
| **AI-Powered RCA** | Root cause analysis with dumps, traces, profiler + code intelligence |
| **DSL & Workflows** | Fluent Go API + YAML automation for CI/CD pipelines |
| **ExecuteABAP** | Run arbitrary ABAP code via unit test wrapper |
| **Code Analysis** | Call graphs, object structure, find definition/references |
| **System Introspection** | System info, installed components, CDS dependencies |
| **Diagnostics** | Short dumps (RABAX), ABAP profiler (ATRA), SQL traces (ST05) |
| **File Deployment** | Bypass token limits - deploy large files directly from filesystem |
| **Surgical Edits** | `EditSource` tool matches Claude's Edit pattern for precise changes |

## Specialized Agents

**6 high-level agents** that orchestrate multiple MCP tools for complex workflows:

| Agent | Purpose | Use Case |
|-------|---------|----------|
| **Code Generator** (`/code-gen`) | Generate ABAP objects from natural language | Create classes, programs, interfaces, CDS views, tables |
| **Debug Orchestrator** (`/debug-orchestrator`) | Autonomous debugging & RCA | Investigate crashes, analyze performance, live debugging |
| **Test Generator** (`/test-gen`) | Create comprehensive unit test coverage | Generate test classes with positive/negative/edge case tests |
| **Code Quality Guardian** (`/code-quality`) | Ensure code quality & compliance | Security audits, performance optimization, ATC checks |
| **Documentation Generator** (`/doc-gen`) | Create technical documentation | README files, API references, UML diagrams, architecture guides |
| **Transport & Deployment Manager** (`/transport-deploy`) | Manage deployments with validation | Pre-deployment checks, dependency ordering, rollback plans |

**Quick Examples**:
```
/code-gen "Create a class ZCL_ORDER_VALIDATOR with method CHECK_ORDER"
/debug-orchestrator "Investigate ZERODIVIDE crash in ZCL_PRICING"
/test-gen "Generate unit tests for ZCL_ORDER_PROCESSOR with 80% coverage"
/code-quality "Run security audit on $ZPROD package"
/doc-gen "Generate complete documentation for $ZAPI package"
/transport-deploy "Prepare $ZRAY package for production deployment"
```

See [docs/AGENTS.md](docs/AGENTS.md) for complete agent documentation and workflows.

## Quick Start

```bash
# Download from releases
curl -LO https://github.com/oisee/vibing-steampunk/releases/latest/download/vsp-linux-amd64
chmod +x vsp-linux-amd64

# Or build from source
git clone https://github.com/oisee/vibing-steampunk.git && cd vibing-steampunk
make build
```

## CLI Mode

vsp works in two modes:
1. **MCP Server Mode** (default) - Exposes tools via Model Context Protocol for Claude
2. **CLI Mode** - Direct command-line operations without MCP

### CLI Commands

```bash
# Search for ABAP objects
vsp -s a4h search "ZCL_*"
vsp -s dev search "Z*ORDER*" --type CLAS --max 50

# Get source code
vsp -s a4h source CLAS ZCL_MY_CLASS
vsp -s dev source PROG ZTEST_PROGRAM

# Export packages to ZIP (abapGit format)
vsp -s a4h export '$ZORK' '$ZLLM' -o packages.zip
vsp -s dev export '$TMP' --subpackages

# List configured systems
vsp systems

# Configuration management
vsp config init          # Create example configs
vsp config show          # Show effective configuration
vsp config mcp-to-vsp    # Import from .mcp.json to .vsp.json
vsp config vsp-to-mcp    # Export from .vsp.json to .mcp.json
```

### System Profiles (`.vsp.json`)

Configure multiple SAP systems in `.vsp.json`:

```json
{
  "default": "dev",
  "systems": {
    "dev": {
      "url": "http://dev.example.com:50000",
      "user": "DEVELOPER",
      "client": "001"
    },
    "a4h": {
      "url": "http://a4h.local:50000",
      "user": "ADMIN",
      "client": "001",
      "insecure": true
    },
    "prod": {
      "url": "https://prod.example.com:44300",
      "user": "READONLY",
      "client": "100",
      "read_only": true,
      "cookie_file": "/path/to/cookies.txt"
    }
  }
}
```

**Password Resolution:**
- Set via environment variable: `VSP_<SYSTEM>_PASSWORD` (e.g., `VSP_DEV_PASSWORD`)
- Or use cookie authentication: `cookie_file` or `cookie_string`

**Config Locations** (searched in order):
1. `.vsp.json` (current directory)
2. `.vsp/systems.json`
3. `~/.vsp.json`
4. `~/.vsp/systems.json`

<details>
<summary><strong>MCP Server Configuration</strong></summary>

### CLI Flags
```bash
vsp --url https://host:44300 --user admin --password secret
vsp --url https://host:44300 --cookie-file cookies.txt
vsp --mode expert  # Enable all 99 tools
```

### Environment Variables
```bash
export SAP_URL=https://host:44300
export SAP_USER=developer
export SAP_PASSWORD=secret
export SAP_CLIENT=001
```

### .env File
```bash
# .env (auto-loaded from current directory)
SAP_URL=https://host:44300
SAP_USER=developer
SAP_PASSWORD=secret
```

| Flag | Env Variable | Description |
|------|--------------|-------------|
| `--url` | `SAP_URL` | SAP system URL |
| `--user` | `SAP_USER` | Username |
| `--password` | `SAP_PASSWORD` | Password |
| `--client` | `SAP_CLIENT` | Client (default: 001) |
| `--mode` | `SAP_MODE` | `focused` (default) or `expert` |
| `--cookie-file` | `SAP_COOKIE_FILE` | Netscape cookie file |
| `--insecure` | `SAP_INSECURE` | Skip TLS verification |
| `--terminal-id` | `SAP_TERMINAL_ID` | SAP GUI terminal ID for cross-tool debugging |
| `--allow-transportable-edits` | `SAP_ALLOW_TRANSPORTABLE_EDITS` | Enable editing transportable objects |
| `--allowed-transports` | `SAP_ALLOWED_TRANSPORTS` | Whitelist transports (wildcards: `A4HK*`) |
| `--allowed-packages` | `SAP_ALLOWED_PACKAGES` | Whitelist packages (wildcards: `Z*,$TMP`) |

</details>

## Usage with Claude

### Claude Desktop

Add to `~/.config/claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "abap-adt": {
      "command": "/path/to/vsp",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "your-username",
        "SAP_PASSWORD": "your-password"
      }
    }
  }
}
```

### Claude Code

Add `.mcp.json` to your project:

```json
{
  "mcpServers": {
    "abap-adt": {
      "command": "/path/to/vsp",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "your-username",
        "SAP_PASSWORD": "your-password"
      }
    }
  }
}
```

### Transportable Packages Configuration

To work with transportable packages (non-`$` prefixed), you **must** explicitly enable transport support:

```json
{
  "mcpServers": {
    "abap-adt": {
      "command": "/path/to/vsp",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "your-username",
        "SAP_PASSWORD": "your-password",
        "SAP_CLIENT": "001",
        "SAP_ALLOW_TRANSPORTABLE_EDITS": "true",
        "SAP_ALLOWED_TRANSPORTS": "DEVK*,A4HK*",
        "SAP_ALLOWED_PACKAGES": "ZPROD,$TMP,$*,Z*"
      }
    }
  }
}
```

| Env Variable | Purpose |
|-------------|---------|
| `SAP_ALLOW_TRANSPORTABLE_EDITS` | Enable editing objects in transportable packages |
| `SAP_ENABLE_TRANSPORTS` | Enable full transport management (create, release) |
| `SAP_ALLOWED_TRANSPORTS` | Whitelist transport patterns (wildcards supported) |
| `SAP_ALLOWED_PACKAGES` | Whitelist package patterns (wildcards supported) |

**CreatePackage with software component:**
```
CreatePackage(
  name="ZPROD_005",
  description="Sub-package",
  parent="ZPROD",
  transport="DEVK900123",
  software_component="HOME"
)
```

Without these flags, operations on transportable packages will be blocked by the safety system.

## Focused vs Expert Mode

| Aspect | Focused (Default) | Expert |
|--------|-------------------|--------|
| **Tools** | 52 essential | 99 complete |
| **Token overhead** | ~2,800 | ~8,000 |
| **Use case** | Daily development | Edge cases, debugging |
| **Unified tools** | GetSource, WriteSource | + granular Get*/Write* |

**Focused mode** consolidates 11 read/write tools into 2 unified tools, reducing cognitive load and token usage by 58%.

Enable expert mode: `--mode=expert` or `SAP_MODE=expert`

## DSL & Automation

### YAML Workflows

```yaml
# ci-pipeline.yaml
name: CI Pipeline
vars:
  package: "$TMP"
steps:
  - action: search
    query: "ZCL_*"
    types: [class]
    package: "{{ .package }}"
    save_as: classes

  - action: test
    objects: "{{ .classes }}"
    parallel: 4

  - action: fail_if
    condition: tests_failed
    message: "Unit tests failed"
```

```bash
vsp workflow run ci-pipeline.yaml --var package='$ZRAY'
```

### Go Library

```go
// Fluent search
objects, _ := dsl.Search(client).
    Query("ZCL_*").Classes().InPackage("$TMP").Execute(ctx)

// Test orchestration
summary, _ := dsl.Test(client).
    Objects(objects...).Parallel(4).Run(ctx)

// Batch import from directory (abapGit-compatible)
result, _ := dsl.Import(client).
    FromDirectory("./src/").
    ToPackage("$ZRAY").
    RAPOrder().  // DDLS → BDEF → Classes → SRVD
    Execute(ctx)

// Export classes with all includes
result, _ := dsl.Export(client).
    Classes("ZCL_TRAVEL", "ZCL_BOOKING").
    ToDirectory("./backup/").
    Execute(ctx)

// RAP deployment pipeline
pipeline := dsl.RAPPipeline(client, "./src/", "$ZRAY", "ZTRAVEL_SB")
```

See [docs/DSL.md](docs/DSL.md) for complete documentation.

## RAP OData Service Creation

VSP supports full RAP OData E2E development since v2.6.0. Create complete OData services via AI assistant:

### Step-by-Step Workflow

**1. Create CDS View (DDLS)**
```
WriteSource(
  object_type="DDLS",
  name="ZTRAVEL",
  package="$TMP",
  description="Travel Entity",
  source=`
@EndUserText.label: 'Travel'
@AccessControl.authorizationCheck: #NOT_REQUIRED
define root view entity ZTRAVEL as select from ztravel_tab {
  key travel_id as TravelId,
  description as Description,
  start_date as StartDate,
  end_date as EndDate,
  status as Status
}
`
)
```

**2. Create Behavior Definition (BDEF)**
```
WriteSource(
  object_type="BDEF",
  name="ZTRAVEL",
  package="$TMP",
  description="Travel Behavior",
  source=`
managed implementation in class ZBP_TRAVEL unique;
strict ( 2 );

define behavior for ZTRAVEL alias Travel
persistent table ztravel_tab
lock master
authorization master ( instance )
{
  field ( readonly ) TravelId;
  field ( mandatory ) Description;

  create;
  update;
  delete;

  mapping for ztravel_tab {
    TravelId = travel_id;
    Description = description;
    StartDate = start_date;
    EndDate = end_date;
    Status = status;
  }
}
`
)
```

**3. Create Service Definition (SRVD)**
```
WriteSource(
  object_type="SRVD",
  name="ZTRAVEL_SD",
  package="$TMP",
  description="Travel Service Definition",
  source=`
@EndUserText.label: 'Travel Service'
define service ZTRAVEL_SD {
  expose ZTRAVEL;
}
`
)
```

**4. Create Service Binding (SRVB)**
```
WriteSource(
  object_type="SRVB",
  name="ZTRAVEL_SB",
  package="$TMP",
  description="Travel OData V4 Binding",
  service_definition="ZTRAVEL_SD",
  binding_version="V4"
)
```

### Binding Options

| Parameter | Values | Description |
|-----------|--------|-------------|
| `binding_version` | `V2`, `V4` | OData protocol version |
| `binding_category` | `0`, `1` | `0`=Web API, `1`=UI |

### For Transportable Packages

Add `transport` parameter to all WriteSource calls:
```
WriteSource(
  object_type="DDLS",
  name="ZTRAVEL",
  package="ZPROD",
  transport="DEVK900123",
  ...
)
```

### Related
- [RAP OData Lessons Report](reports/2025-12-08-003-rap-odata-service-lessons.md)
- DSL Pipeline: `dsl.RAPPipeline(client, "./src/", "$PKG", "ZSRV_SB")`

## ExecuteABAP

Run arbitrary ABAP code via unit test wrapper:

```
ExecuteABAP:
  code: |
    DATA(lv_msg) = |Hello from SAP at { sy-datum }|.
    lv_result = lv_msg.
```

**Risk levels:** `harmless` (read-only), `dangerous` (write), `critical` (full access)

See [ExecuteABAP Report](reports/2025-12-05-004-execute-abap-implementation.md) for details.

## AI-Powered Root Cause Analysis

vsp enables AI assistants to investigate production issues autonomously:

```
User: "Investigate the ZERODIVIDE crash in production"

AI Workflow:
  1. GetDumps      → Find recent crashes by exception type
  2. GetDump       → Analyze stack trace and variable values
  3. GetSource     → Read code at crash location
  4. GetCallGraph  → Trace call hierarchy
  5. GrepPackages  → Find similar patterns
  6. Analysis      → Identify root cause
  7. Propose Fix   → Generate solution + test case
```

**Example Output:**
> "The crash occurs in `ZCL_PRICING=>CALCULATE_RATIO` when `LV_TOTAL=0`.
> This happens for archived orders with no line items. Here's the fix..."

See [AI-Powered RCA Workflows](reports/2025-12-05-013-ai-powered-rca-workflows.md) for the complete vision.

## Tools Reference

**52 Focused Mode Tools:**
- **Search:** SearchObject, GrepObjects, GrepPackages
- **Read:** GetSource, GetTable, GetTableContents, RunQuery, GetPackage, GetFunctionGroup, GetCDSDependencies
- **Debugger:** DebuggerListen, DebuggerAttach, DebuggerDetach, DebuggerStep, DebuggerGetStack, DebuggerGetVariables
  - *Note: Breakpoints now managed via WebSocket (ZADT_VSP)*
- **Write:** WriteSource, EditSource, ImportFromFile, ExportToFile, MoveObject
- **Dev:** SyntaxCheck, RunUnitTests, RunATCCheck, LockObject, UnlockObject
- **Intelligence:** FindDefinition, FindReferences
- **System:** GetSystemInfo, GetInstalledComponents, GetCallGraph, GetObjectStructure, GetFeatures
- **Diagnostics:** GetDumps, GetDump, ListTraces, GetTrace, GetSQLTraceState, ListSQLTraces
- **Git:** GitTypes, GitExport (requires abapGit on SAP)
- **Reports:** RunReport, GetVariants, GetTextElements, SetTextElements
- **Install:** InstallZADTVSP, InstallAbapGit, ListDependencies

See [README_TOOLS.md](README_TOOLS.md) for complete tool documentation (99 tools).

<details>
<summary><strong>Capability Matrix</strong></summary>

| Capability | ADT (Eclipse) | abap-adt-api (TS) | **vsp** |
|------------|:-------------:|:-----------------:|:-------:|
| Programs, Classes, Interfaces | Y | Y | **Y** |
| Functions, Function Groups | Y | Y | **Y** |
| Tables, Structures | Y | Y | **Y** |
| CDS Views | Y | Y | **Y** |
| Syntax Check, Activation | Y | Y | **Y** |
| Unit Tests | Y | Y | **Y** |
| CRUD Operations | Y | Y | **Y** |
| Find Definition/References | Y | Y | **Y** |
| Code Completion | Y | Y | **Y** |
| ATC Checks | Y | Y | **Y** |
| Call Graph | Y | Y | **Y** |
| System Info | Y | Y | **Y** |
| Surgical Edit (Edit pattern) | - | - | **Y** |
| File-based Deploy | - | - | **Y** |
| ExecuteABAP | - | - | **Y** |
| RAP OData (DDLS/SRVD/SRVB) | Y | - | **Y** |
| OData Service Publish | Y | - | **Y** |
| abapGit Export | Y | - | **Y** (WebSocket) |
| Debugging | Y | Y | N |

</details>

## Credits

| Project | Author | Contribution |
|---------|--------|--------------|
| [abap-adt-api](https://github.com/marcellourbani/abap-adt-api) | Marcello Urbani | TypeScript ADT library, definitive API reference |
| [mcp-abap-adt](https://github.com/mario-andreschak/mcp-abap-adt) | Mario Andreschak | First MCP server for ABAP ADT |

**vsp** is a Go rewrite with:
- Single binary, zero dependencies
- 62 tools (vs 13 original)
- ~50x faster startup

## Optional: WebSocket Handler (ZADT_VSP)

vsp can optionally deploy a WebSocket handler to SAP for enhanced functionality like RFC calls:

```bash
# 1. Create package
vsp CreatePackage --name '$ZADT_VSP' --description 'VSP WebSocket Handler'

# 2. Deploy objects (embedded in binary)
vsp WriteSource --object_type INTF --name ZIF_VSP_SERVICE --package '$ZADT_VSP' \
    --source "$(cat embedded/abap/zif_vsp_service.intf.abap)"
vsp WriteSource --object_type CLAS --name ZCL_VSP_RFC_SERVICE --package '$ZADT_VSP' \
    --source "$(cat embedded/abap/zcl_vsp_rfc_service.clas.abap)"
vsp WriteSource --object_type CLAS --name ZCL_VSP_APC_HANDLER --package '$ZADT_VSP' \
    --source "$(cat embedded/abap/zcl_vsp_apc_handler.clas.abap)"

# 3. Manually create APC app in SAPC + activate in SICF
#    See embedded/abap/README.md for details
```

**After deployment**, connect via WebSocket to call RFCs:
```json
{"id":"1","domain":"rfc","action":"call","params":{"function":"BAPI_USER_GET_DETAIL","USERNAME":"TESTUSER"}}
```

See [WebSocket Handler Report](reports/2025-12-18-002-websocket-rfc-handler.md) for complete documentation.

## Documentation

| Document | Description |
|----------|-------------|
| [README_TOOLS.md](README_TOOLS.md) | Complete tool reference (94 tools) |
| [MCP_USAGE.md](MCP_USAGE.md) | AI agent usage guide |
| [docs/DSL.md](docs/DSL.md) | DSL & workflow documentation |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Technical architecture |
| [CLAUDE.md](CLAUDE.md) | AI development guidelines |
| [embedded/abap/README.md](embedded/abap/README.md) | WebSocket handler deployment |
| [Roadmap: Quick/Mid/Far Wins](reports/2026-01-02-005-roadmap-quick-mid-far-wins.md) | Prioritized feature backlog |
| [Observations Since v2.12.5](reports/2025-12-22-observations-since-v2.12.5.md) | Recent changes & research summary |

<details>
<summary><strong>SQL Query Notes</strong></summary>

Uses **ABAP SQL syntax**, not standard SQL:

| Feature | Status |
|---------|--------|
| `ORDER BY col ASCENDING` | Works |
| `ORDER BY col DESCENDING` | Works |
| `ORDER BY col ASC/DESC` | **FAILS** - use ASCENDING/DESCENDING |
| `LIMIT n` | **FAILS** - use `max_rows` parameter |

</details>

## Development

```bash
# Build
make build          # Current platform
make build-all      # All 9 platforms

# Test
go test ./...                              # Unit tests (249)
go test -tags=integration -v ./pkg/adt/    # Integration tests (21+)
```

<details>
<summary><strong>Architecture</strong></summary>

```
vibing-steampunk/
├── cmd/vsp/main.go           # CLI (cobra/viper)
├── pkg/adt/
│   ├── client.go             # ADT client + read ops
│   ├── crud.go               # CRUD operations
│   ├── devtools.go           # Syntax check, activate, tests
│   ├── codeintel.go          # Definition, refs, completion
│   ├── workflows.go          # High-level workflows
│   └── http.go               # HTTP transport (CSRF, auth)
├── internal/mcp/server.go    # MCP tool handlers (62 tools)
└── pkg/dsl/                  # DSL & workflow engine
```

</details>

## Project Status

| Metric | Value |
|--------|-------|
| **Tools** | 99 (52 focused, 99 expert) |
| **Unit Tests** | 270+ |
| **Platforms** | 9 (Linux, macOS, Windows × amd64/arm64/386) |

<details>
<summary><strong>Roadmap</strong></summary>

### Completed (v2.15.0)
- [x] DSL & Workflow Engine
- [x] CDS Dependency Analysis (`GetCDSDependencies`)
- [x] ATC Code Quality Checks (`RunATCCheck`)
- [x] ExecuteABAP (code injection via unit tests)
- [x] System Info & Components (`GetSystemInfo`, `GetInstalledComponents`)
- [x] Call Graph & Object Structure (`GetCallGraph`, `GetObjectStructure`)
- [x] Short Dumps / Runtime Errors - `GetDumps`, `GetDump` (RABAX)
- [x] ABAP Profiler / Traces - `ListTraces`, `GetTrace` (ATRA)
- [x] SQL Trace - `GetSQLTraceState`, `ListSQLTraces` (ST05)
- [x] **RAP OData E2E** - DDLS, SRVD, SRVB create + publish (v2.6.0)
- [x] **External Breakpoints** - Line, exception, statement, message (v2.7.0)
- [x] **Debug Session** - Listener, attach, detach, step, stack, variables (v2.8.0)
- [x] **Tool Group Disablement** - `--disabled-groups 5THD` (v2.10.0)
- [x] **UI5/BSP Read** - `UI5ListApps`, `UI5GetApp`, `UI5GetFileContent` (v2.10.1)
- [x] **Feature Detection** - `GetFeatures` tool + system capability probing (v2.12.4)
- [x] **WriteSource SRVB** - Create Service Bindings via unified API (v2.12.4)
- [x] **Call Graph & RCA** - GetCallersOf, GetCalleesOf, TraceExecution (v2.13.0)
- [x] **Lua Scripting** - REPL, 40+ bindings, debug session management (v2.14.0)
- [x] **WebSocket Debugging** - ZADT_VSP handler, TPDAPI integration (v2.15.0)
- [x] **Force Replay** - Variable history, state injection (v2.15.0)

### Parked (Needs Further Work)
- [ ] **AMDP Debugger** - Experimental: Session works, breakpoint triggering under investigation ([Report](reports/2025-12-22-001-amdp-debugging-investigation.md))
- [ ] **UI5/BSP Write** - ADT filestore is read-only, needs custom plugin via `/UI5/CL_REPOSITORY_LOAD`
- [x] **abapGit Export** - WebSocket integration complete (v2.16.0) - GitTypes, GitExport tools ([Report](reports/2025-12-23-002-abapgit-websocket-integration-complete.md))
- [ ] **abapGit Import** - Requires `ZCL_ABAPGIT_OBJECTS=>deserialize` with virtual repository

### Planned
- [ ] API Release State (ARS) - Contract stability checks
- [ ] Message Server Logs
- [ ] Background Job Management

### Future Considerations
- [ ] AMDP Session Persistence (enable full HANA debugging)
- [ ] Graph Traversal & Analysis (code dependency graphs)
- [ ] Test Intelligence (smart test execution based on changes)
- [ ] Standard API Surface Scraper

**Research Reports:**
- [AMDP Session Architecture](reports/2025-12-05-019-amdp-session-architecture.md) - Session binding analysis & solutions
- [Native ADT Features](reports/2025-12-05-005-native-adt-features-deep-dive.md) - Comprehensive ADT capability analysis
- [ADT Debugger API](reports/2025-12-05-012-adt-debugger-api-deep-dive.md) - External debugging REST API
- [AI-Powered RCA](reports/2025-12-05-013-ai-powered-rca-workflows.md) - Vision for AI-assisted debugging

</details>

## Lua Scripting (New in v2.14)

Automate debugging workflows with Lua scripts:

```bash
# Interactive REPL
vsp lua

# Run a script
vsp lua examples/scripts/debug-session.lua

# Execute inline
vsp lua -e 'print(json.encode(searchObject("ZCL_*", 10)))'
```

**Example: Set breakpoint and debug**
```lua
-- Set breakpoint
local bpId = setBreakpoint("ZTEST_PROGRAM", 42)
print("Breakpoint: " .. bpId)

-- Wait for debuggee
local event = listen(60)
if event then
    attach(event.id)
    print("Stack:")
    for i, frame in ipairs(getStack()) do
        print("  " .. frame.program .. ":" .. frame.line)
    end
    stepOver()
    detach()
end
```

**Available Functions:**
- **Search**: `searchObject`, `grepObjects`
- **Source**: `getSource`, `writeSource`, `editSource`
- **Debug**: `setBreakpoint`, `listen`, `attach`, `detach`, `stepOver`, `stepInto`, `stepReturn`, `continue_`, `getStack`, `getVariables`
- **Checkpoints**: `saveCheckpoint`, `getCheckpoint`, `listCheckpoints`, `injectCheckpoint`
- **Diagnostics**: `getDumps`, `getDump`, `runUnitTests`, `syntaxCheck`
- **Call Graph**: `getCallGraph`, `getCallersOf`, `getCalleesOf`
- **Utilities**: `print`, `sleep`, `json.encode`, `json.decode`

See `examples/scripts/` for more examples.

## RCA, Replay & Test Extraction

### The Vision: AI-Powered Debugging Pipeline

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  1. SET BREAKPOINT    →  2. RUN PROGRAM    →  3. CAPTURE CONTEXT           │
│     setBreakpoint()       (trigger via         saveCheckpoint()             │
│     on FM/method          unit test/RFC)       for each hit                 │
├─────────────────────────────────────────────────────────────────────────────┤
│  4. EXTRACT TEST CASES  →  5. AI NORMALIZE  →  6. GENERATE UNIT TESTS      │
│     inputs + outputs       deduplicate,         ABAP Unit classes           │
│     from checkpoints       explain patterns     with mocks                  │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Example: Capture FM Execution for Test Generation

```lua
-- Step 1: Set breakpoint on function module entry
local bpId = setBreakpoint("SAPL<FGROUP>", 10)  -- Entry point

-- Step 2: Prepare to capture multiple executions
local captures = {}

-- Step 3: Loop to capture test cases
for i = 1, 10 do
    local event = listen(120)  -- Wait for debuggee
    if not event then break end

    attach(event.id)

    -- Capture input parameters at entry
    local vars = getVariables()
    local testCase = {
        id = i,
        inputs = extractInputs(vars),  -- IV_*, IT_*, IS_*
        timestamp = os.time()
    }

    -- Step to end to capture outputs
    continue_()
    local event2 = listen(5)
    if event2 then
        attach(event2.id)
        testCase.outputs = extractOutputs(getVariables())  -- EV_*, ET_*, ES_*, RETURN
    end

    -- Save checkpoint for replay
    saveCheckpoint("testcase_" .. i, testCase)
    table.insert(captures, testCase)

    detach()
end

-- Step 4: Export for AI processing
print(json.encode(captures))
```

### AI Processing Pipeline

After capturing test cases, AI can:

1. **Normalize & Deduplicate** - Group similar inputs, identify unique scenarios
2. **Explain Patterns** - "TestCase 3 tests error path when IV_AMOUNT < 0"
3. **Generate Unit Tests** - Create ABAP Unit test class with proper mocks

```
User: "Analyze captured test cases and generate unit tests"

AI Workflow:
  1. Load checkpoints     → listCheckpoints("testcase_*")
  2. Analyze patterns     → Cluster by input signatures
  3. Identify edge cases  → Empty tables, zero values, error conditions
  4. Generate mock specs  → Which FMs/DB tables need mocking
  5. Create ABAP Unit     → ZCL_TEST_<FM> with test methods
  6. Deploy tests         → WriteSource to SAP system
```

### What Works Today (v2.14)

| Feature | Status | Command/Function |
|---------|--------|------------------|
| Set breakpoints | ✅ | `setBreakpoint(program, line)` |
| Listen for debuggee | ✅ | `listen(timeout)` |
| Attach/detach | ✅ | `attach(id)`, `detach()` |
| Step execution | ✅ | `stepOver()`, `stepInto()`, `continue_()` |
| Get variables | ✅ | `getVariables()` |
| Get stack trace | ✅ | `getStack()` |
| Save checkpoints | ✅ | `saveCheckpoint(name, data)` |
| Load checkpoints | ✅ | `getCheckpoint(name)` |
| Call graph analysis | ✅ | `getCallersOf()`, `getCalleesOf()` |
| Short dump analysis | ✅ | `getDumps()`, `getDump(id)` |

### Coming in Future Phases

| Feature | Phase | Description |
|---------|-------|-------------|
| Variable history recording | 5.2 | ✅ Track all variable changes during execution |
| Force Replay (state injection) | 5.5 | ✅ Inject saved state into live debug session |
| Test case extraction | 6.1 | Automated input/output extraction from recordings |
| ABAP test generator | 6.3 | Generate ABAP Unit classes from test cases |
| Mock framework | 6.4 | ZCL_VSP_MOCK for DB/RFC mocking |
| Isolated playground | 7.1 | Fast test execution with mocked dependencies |
| Time-travel debugging | 8.1 | Navigate backwards through execution |

### Related Documentation

| Document | Description |
|----------|-------------|
| [VISION.md](VISION.md) | The dream: AI as a senior developer |
| [ROADMAP.md](ROADMAP.md) | Detailed implementation timeline |
| [TAS & Scripting](reports/2025-12-21-001-tas-scripting-time-travel-vision.md) | Technical design for TAS-style debugging |
| [Test Extraction](reports/2025-12-21-002-test-extraction-isolated-replay.md) | Playground and mock architecture |
| [Force Replay](reports/2025-12-21-003-force-replay-state-injection.md) | State injection design |
| [**Implications Analysis**](reports/2025-12-21-004-test-extraction-implications.md) | Paradigm shift: archaeology → observation |
| [AI-Powered RCA](reports/2025-12-05-013-ai-powered-rca-workflows.md) | Root cause analysis workflows |

---

## Vision & Roadmap

**Where we're going:** TAS-style debugging, time-travel, AI-powered RCA

| Phase | Target | Features |
|-------|--------|----------|
| 5 | Q1 2026 | Lua scripting ✅, variable history, checkpoints, Force Replay |
| 6 | Q2 2026 | Test case extraction, ABAP test generator, mock framework |
| 7 | Q3 2026 | Isolated playground with mocks, patch & re-run |
| 8 | Q4 2026 | Time-travel debugging, temporal queries |
| 9+ | 2027 | AI-suggested breakpoints, multi-agent debugging, self-healing |

**Read more:**
- [VISION.md](VISION.md) - The dream: AI as a senior developer
- [ROADMAP.md](ROADMAP.md) - Detailed implementation plan
- [TAS & Scripting Report](reports/2025-12-21-001-tas-scripting-time-travel-vision.md) - Full technical design
- [Test Extraction Report](reports/2025-12-21-002-test-extraction-isolated-replay.md) - Playground architecture

## License

MIT

## Contributing

Contributions welcome! See [ARCHITECTURE.md](ARCHITECTURE.md) and [CLAUDE.md](CLAUDE.md) for guidelines.
