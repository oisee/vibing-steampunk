# CLAUDE.md - AI Assistant Guidelines

This file provides context for AI assistants (Claude, etc.) working on this project.

## Project Overview

**vsp** is a Go-native MCP (Model Context Protocol) server for SAP ABAP Development Tools (ADT). It provides a single-binary distribution with 41 essential tools (focused mode, default) or 68 complete tools (expert mode) for use with Claude and other MCP-compatible LLMs.

## Quick Reference

### Build & Test

```bash
# Build
go build -o vsp ./cmd/vsp

# Run unit tests
go test ./...

# Run integration tests (requires SAP system)
SAP_URL=http://host:port SAP_USER=user SAP_PASSWORD=pass SAP_CLIENT=001 \
  go test -tags=integration -v ./pkg/adt/
```

### Configuration (Priority: CLI > Env > .env > Defaults)

```bash
# Using CLI flags
./vsp --url http://host:50000 --user admin --password secret

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
| `SAP_MODE` / `--mode` | Tool mode: `focused` (20 tools, default) or `expert` (47 tools) |
| `SAP_DISABLED_GROUPS` / `--disabled-groups` | Disable tool groups: `5`/`U`=UI5, `T`=Tests, `H`=HANA, `D`=Debug |
| `SAP_VERBOSE` / `--verbose` | Enable verbose logging to stderr |
| **Safety Configuration** | |
| `SAP_READ_ONLY` / `--read-only` | Block all write operations (default: false) |
| `SAP_BLOCK_FREE_SQL` / `--block-free-sql` | Block RunQuery execution (default: false) |
| `SAP_ALLOWED_OPS` / `--allowed-ops` | Whitelist operation types (e.g., "RSQ") |
| `SAP_DISALLOWED_OPS` / `--disallowed-ops` | Blacklist operation types (e.g., "CDUA") |
| `SAP_ALLOWED_PACKAGES` / `--allowed-packages` | Restrict to packages (supports wildcards: "Z*") |

## Codebase Structure

```
cmd/vsp/main.go       # Entry point
internal/mcp/server.go       # MCP server (45 tool handlers, mode-aware)
pkg/
├── adt/
│   ├── client.go             # ADT client + read operations
│   ├── crud.go               # CRUD operations (lock, create, update, delete)
│   ├── devtools.go           # Dev tools (syntax check, activate, unit tests)
│   ├── codeintel.go          # Code intelligence (find def, refs, completion)
│   ├── debugger.go           # External debugger (breakpoints, listener)
│   ├── amdp_debugger.go      # HANA/AMDP debugger (SQLScript debugging)
│   ├── ui5.go                # UI5/Fiori BSP management
│   ├── workflows.go          # High-level workflow operations
│   ├── cds.go                # CDS view dependency analysis
│   ├── safety.go             # Safety & protection configuration
│   ├── safety_test.go        # Safety unit tests (25 tests)
│   ├── http.go               # HTTP transport (CSRF, sessions)
│   ├── config.go             # Configuration
│   ├── cookies.go            # Cookie file parsing (Netscape format)
│   └── xml.go                # XML types
│
├── dsl/                      # Fluent API & Workflow Engine (Report 012)
│   ├── types.go              # Core types (ObjectRef, TestConfig, etc.)
│   ├── search.go             # Fluent search builder
│   ├── test_runner.go        # Unit test orchestration
│   ├── workflow.go           # YAML workflow engine
│   ├── batch.go              # Batch operations & pipeline builder
│   └── dsl_test.go           # Unit tests (13 tests)
│
└── cache/                    # Caching infrastructure (Report 010)
    ├── cache.go              # Core interfaces and types
    ├── memory.go             # In-memory cache (default)
    ├── sqlite.go             # SQLite cache (optional)
    ├── cache_test.go         # Unit tests (16 tests)
    ├── example_test.go       # Usage examples
    └── README.md             # Documentation
```

## Key Files for Common Tasks

| Task | Files |
|------|-------|
| Add new MCP tool | `internal/mcp/server.go` |
| Add ADT read operation | `pkg/adt/client.go` |
| Add CRUD operation | `pkg/adt/crud.go` |
| Add development tool | `pkg/adt/devtools.go` |
| Add code intelligence | `pkg/adt/codeintel.go` |
| Add ABAP debugger feature | `pkg/adt/debugger.go` |
| Add HANA/AMDP debugger | `pkg/adt/amdp_debugger.go` |
| Add UI5/BSP feature | `pkg/adt/ui5.go` |
| Add workflow | `pkg/adt/workflows.go` |
| Add XML types | `pkg/adt/xml.go` |
| Add integration test | `pkg/adt/integration_test.go` |

## Adding a New Tool

1. **Add ADT client method** in appropriate file (`client.go`, `crud.go`, etc.)
2. **Add tool handler** in `internal/mcp/server.go`:
   - Register tool in `registerTools()`
   - Add handler case in `handleToolCall()`
3. **Add integration test** in `pkg/adt/integration_test.go`
4. **Update documentation**:
   - `README.md` tool tables
   - `reports/vsp-status.md`

## Code Patterns

### ADT Client Methods

```go
// Read operation pattern
func (c *Client) GetSomething(ctx context.Context, name string) (*Result, error) {
    url := fmt.Sprintf("/sap/bc/adt/path/%s", name)
    resp, err := c.http.Get(ctx, url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    // Parse response
}

// Write operation pattern (requires stateful session)
func (c *Client) UpdateSomething(ctx context.Context, name, content string) error {
    url := fmt.Sprintf("/sap/bc/adt/path/%s", name)
    return c.http.Put(ctx, url, "text/plain", strings.NewReader(content))
}
```

### Tool Handler Pattern

```go
case "NewTool":
    name, _ := getString(args, "name")
    result, err := s.client.NewMethod(ctx, name)
    if err != nil {
        return mcp.NewToolResultError(err.Error()), nil
    }
    return mcp.NewToolResultText(formatResult(result)), nil
```

## Testing

### Unit Tests (172 tests)
- Mock HTTP client (see `client_test.go`, `http_test.go`, `workflows_test.go`)
- Cookie parsing tests (`cookies_test.go`)
- Unified tools tests (GetSource, WriteSource, GrepObjects, GrepPackages)
- Safety checks (`safety_test.go`)
- Run: `go test ./...`

### Integration Tests (21+ tests)
- Build tag: `integration`
- Create objects in `$TMP` package, clean up after
- Run: `go test -tags=integration -v ./pkg/adt/`
- Test program for manual testing: `ZTEST_MCP_CRUD` in `$TMP`

## ADT API Reference

The SAP ADT REST API documentation can be found at:
- `/sap/bc/adt/discovery` - API discovery document
- See `reports/adt-abap-internals-documentation.md` for detailed endpoint analysis

## Common Issues

1. **CSRF token errors**: The HTTP transport auto-refreshes tokens; check `http.go`
2. **Lock conflicts**: Objects must be unlocked before other operations
3. **Activation failures**: Check syntax errors first with `SyntaxCheck`
4. **Session issues**: CRUD operations require stateful sessions
5. **Auth conflicts**: Use only one auth method (basic OR cookies, not both)
6. **Cookie auth with .env**: Pass `--cookie-file` to override .env credentials

## SAP Object Naming Conventions

When creating ABAP objects for testing and experiments, follow these conventions:

### Package Structure
- **Root package**: `$ZADT` (ADT experiments and testing)
- **Subpackages**: `$ZADT_00`, `$ZADT_01`, etc. for different purposes/features
- Example: `$ZADT_00` for debugger experiments, `$ZADT_01` for CDS experiments

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

### Current Reports

#### Analysis & Research (Reports 001-002)
- **001:** Auto Pilot Deep Dive - Complete ZRAY_10_AUTO_PILOT execution flow to CROSS/WBCROSSGT
- **002:** CROSS & WBCROSSGT Reference Guide - Real system statistics, traversal patterns, handler architecture

#### Design Documents (Reports 003-009)
- **003:** Graph & API Surface Design Overview - Executive summary of both initiatives
- **004:** Graph Architecture Improvements (vs-punk) - Alternative design approach
- **005:** Improved Graph Architecture Design - Clean architecture redesign for ZRAY graph system
- **006:** Standard API Surface Scraper - Tool to discover and analyze SAP standard API usage
- **007:** Graph Traversal Implementation Plan - Step-by-step implementation for vsp
- **008:** Test Intelligence Plan - Smart test execution based on code changes
- **009:** Library Architecture & Caching Strategy - Multi-layer architecture and SQLite caching

#### Implementation Reports (Reports 010+)
- **010:** Cache Implementation Complete - Phase 1 done: in-memory + SQLite caching (2,180 LOC, 16 tests passing)
- **011:** Safety & Protection Implementation - CRUD protection with operation filtering and package restrictions (530 LOC, 25 tests passing)

#### 2025-12-05 Reports
- **001:** Code Injection & Bootstrap Strategies - Unit Test execution vehicle, data injection options
- **002:** Self-Replicating Deploy Agent Design - Rejected due to STRUST/SSL certificate concerns
- **003:** ADT-Assisted Universal Deployment - Factory Pattern strategy via vsp (ADT-native)
- **004:** ExecuteABAP Implementation - ABAP code execution via Unit Test wrapper (385 LOC, 2 tests)
- **014:** External Debugger Scripting Vision - Watchpoints API, AI-powered debugger scripting architecture

#### Reference Documentation (Non-numbered)
- `abap-adt-discovery-guide.md` - ADT API discovery process
- `adt-abap-internals-documentation.md` - Detailed ADT endpoint analysis
- `adt-capability-matrix.md` - ADT feature comparison
- `cookie-auth-implementation-guide.md` - Cookie authentication research
- `vsp-status.md` - Current project status

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

## Content here...
```

## Project Status

| Metric | Value |
|--------|-------|
| **Tools** | 76 (49 focused, 76 expert) |
| **Unit Tests** | 270 |
| **Integration Tests** | 34 |
| **Platforms** | 9 |
| **Phase** | 4 (Native ADT Features) - In Progress |
| **Reports** | 17 numbered + 6 reference docs |
| **Cache Package** | ✅ Complete (in-memory + SQLite) |
| **Safety System** | ✅ Complete (operation filtering, package restrictions) |
| **DSL Package** | ✅ Complete (fluent API, YAML workflows, test orchestration) |
| **ExecuteABAP** | ✅ Complete (code execution via Unit Test wrapper) |
| **System Info** | ✅ Complete (GetSystemInfo, GetInstalledComponents) |
| **Code Analysis** | ✅ Complete (GetCallGraph, GetObjectStructure) |
| **Runtime Errors** | ✅ Complete (GetDumps, GetDump - RABAX) |
| **ABAP Profiler** | ✅ Complete (ListTraces, GetTrace - ATRA) |
| **SQL Trace** | ✅ Complete (GetSQLTraceState, ListSQLTraces - ST05) |
| **RAP OData E2E** | ✅ Complete (DDLS, SRVD, SRVB create + publish) |
| **External Debugger** | ✅ Complete (Breakpoints, Debug Listener, Debuggee parsing) |
| **AMDP Debugger** | ✅ Complete (HANA SQLScript debugging via ADT) |
| **UI5/BSP Mgmt** | ✅ Partial (Read ops work; Create needs alternate API) |
| **Tool Groups** | ✅ Complete (--disabled-groups: 5/U, T, H, D) |

### DSL & Workflow Usage

```bash
# Run unit tests for a package
vsp workflow test "$TMP"
vsp workflow test "$ZRAY*" --parallel 4 --json

# Run YAML workflow
vsp workflow run examples/workflows/ci-pipeline.yaml --var PACKAGE=\$TMP
```

```go
// Go fluent API
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
```

### Roadmap
- **Phase 5:** Graph Traversal & Analysis (Design: Reports 005-007)
- **Phase 6:** Standard API Surface Scraper (Design: Report 006)
- **Phase 7:** Test Intelligence (Design: Report 008)
- Transport Management
- ATC Integration
- CDS View Support
- RAP/BDEF Support
