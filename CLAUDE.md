# CLAUDE.md - AI Assistant Guidelines

This file provides context for AI assistants (Claude, etc.) working on this project.

## Project Overview

**mcp-adt-go** is a Go-native MCP (Model Context Protocol) server for SAP ABAP Development Tools (ADT). It provides a single-binary distribution of 43 ADT tools for use with Claude and other MCP-compatible LLMs.

## Quick Reference

### Build & Test

```bash
# Build
go build -o mcp-adt-go ./cmd/mcp-adt-go

# Run unit tests
go test ./...

# Run integration tests (requires SAP system)
SAP_URL=http://host:port SAP_USER=user SAP_PASSWORD=pass SAP_CLIENT=001 \
  go test -tags=integration -v ./pkg/adt/
```

### Configuration (Priority: CLI > Env > .env > Defaults)

```bash
# Using CLI flags
./mcp-adt-go --url http://host:50000 --user admin --password secret

# Using environment variables
SAP_URL=http://host:50000 SAP_USER=user SAP_PASSWORD=pass ./mcp-adt-go

# Using cookie authentication
./mcp-adt-go --url http://host:50000 --cookie-string "sap-usercontext=abc; SAP_SESSIONID=xyz"
./mcp-adt-go --url http://host:50000 --cookie-file cookies.txt
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
| `SAP_VERBOSE` / `--verbose` | Enable verbose logging to stderr |
| **Safety Configuration** | |
| `SAP_READ_ONLY` / `--read-only` | Block all write operations (default: false) |
| `SAP_BLOCK_FREE_SQL` / `--block-free-sql` | Block RunQuery execution (default: false) |
| `SAP_ALLOWED_OPS` / `--allowed-ops` | Whitelist operation types (e.g., "RSQ") |
| `SAP_DISALLOWED_OPS` / `--disallowed-ops` | Blacklist operation types (e.g., "CDUA") |
| `SAP_ALLOWED_PACKAGES` / `--allowed-packages` | Restrict to packages (supports wildcards: "Z*") |

## Codebase Structure

```
cmd/mcp-adt-go/main.go       # Entry point
internal/mcp/server.go       # MCP server (43 tool handlers)
pkg/
├── adt/
│   ├── client.go             # ADT client + read operations
│   ├── crud.go               # CRUD operations (lock, create, update, delete)
│   ├── devtools.go           # Dev tools (syntax check, activate, unit tests)
│   ├── codeintel.go          # Code intelligence (find def, refs, completion)
│   ├── workflows.go          # High-level workflow operations
│   ├── cds.go                # CDS view dependency analysis
│   ├── safety.go             # Safety & protection configuration
│   ├── safety_test.go        # Safety unit tests (25 tests)
│   ├── http.go               # HTTP transport (CSRF, sessions)
│   ├── config.go             # Configuration
│   ├── cookies.go            # Cookie file parsing (Netscape format)
│   └── xml.go                # XML types
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
   - `reports/mcp-adt-go-status.md`

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

### Unit Tests (84 tests)
- Mock HTTP client (see `client_test.go`, `http_test.go`)
- Cookie parsing tests (`cookies_test.go`)
- Run: `go test ./...`

### Integration Tests (20+ tests)
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
- **007:** Graph Traversal Implementation Plan - Step-by-step implementation for mcp-adt-go
- **008:** Test Intelligence Plan - Smart test execution based on code changes
- **009:** Library Architecture & Caching Strategy - Multi-layer architecture and SQLite caching

#### Implementation Reports (Reports 010+)
- **010:** Cache Implementation Complete - Phase 1 done: in-memory + SQLite caching (2,180 LOC, 16 tests passing)
- **011:** Safety & Protection Implementation - CRUD protection with operation filtering and package restrictions (530 LOC, 25 tests passing)

#### Reference Documentation (Non-numbered)
- `abap-adt-discovery-guide.md` - ADT API discovery process
- `adt-abap-internals-documentation.md` - Detailed ADT endpoint analysis
- `adt-capability-matrix.md` - ADT feature comparison
- `cookie-auth-implementation-guide.md` - Cookie authentication research
- `mcp-adt-go-status.md` - Current project status

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
| **Tools** | 43 |
| **Unit Tests** | 109 (84 ADT + 25 safety) |
| **Integration Tests** | 21+ |
| **Platforms** | 9 |
| **Phase** | 5 (CDS Dependency Analysis) - Complete |
| **Reports** | 11 numbered + 6 reference docs |
| **Cache Package** | ✅ Complete (in-memory + SQLite) |
| **Safety System** | ✅ Complete (operation filtering, package restrictions) |

### Roadmap
- **Phase 5:** Graph Traversal & Analysis (Design: Reports 005-007)
- **Phase 6:** Standard API Surface Scraper (Design: Report 006)
- **Phase 7:** Test Intelligence (Design: Report 008)
- Transport Management
- ATC Integration
- CDS View Support
- RAP/BDEF Support
