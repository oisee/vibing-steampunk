# Copilot Instructions for vsp

## Project

**vsp** is a Go-native MCP (Model Context Protocol) server that bridges AI agents to SAP ABAP systems via the ADT REST API. Single binary, 9 platforms, zero dependencies. It exposes 81 tools (focused mode) or 122 tools (expert mode) over JSON-RPC/stdio.

## Build, Test, Lint

```bash
# Build
go build -o vsp ./cmd/vsp

# Run all unit tests
go test ./...

# Run a single test by name
go test -run TestSafetyReadOnly -v ./pkg/adt/

# Run a single package's tests
go test -v ./pkg/cache/

# Integration tests (require live SAP system via SAP_* env vars)
go test -tags=integration -v ./pkg/adt/

# Lint (golangci-lint with .golangci.yml config)
golangci-lint run ./...

# Format (gofumpt preferred, falls back to go fmt)
gofumpt -w .
```

## Architecture

```
cmd/vsp/main.go            → CLI entry point (cobra + viper)
internal/mcp/
  server.go                → MCP server struct, initialization
  tools_register.go        → Tool registration logic (mode + group filtering)
  tools_focused.go         → Focused-mode tool whitelist
  tools_groups.go          → Disableable tool groups (5/U, T, H, D, C, G, R, I, X)
  handlers_*.go            → One file per tool category (20+ handler files)
pkg/adt/
  config.go                → ADT client config (functional options pattern)
  client.go                → Main client facade + read operations
  http.go                  → HTTP transport (CSRF token refresh, session management)
  crud.go                  → Lock → Update → Unlock → Activate lifecycle
  workflows.go             → High-level composite operations (GetSource, WriteSource)
  workflows_edit.go        → EditSource (find-replace with syntax check)
  safety.go                → Operation filtering and package restrictions
  features.go              → Runtime feature probing (auto/on/off)
  debugger.go              → External ABAP debugger (WebSocket via ZADT_VSP)
  xml.go                   → XML type definitions for ADT responses
pkg/dsl/                   → Fluent API for search, test, batch, YAML workflows
pkg/cache/                 → In-memory + SQLite caching
pkg/scripting/             → Lua scripting engine (40+ ADT bindings)
```

The request flow is: **AI Agent → MCP Server (handlers) → ADT Client (pkg/adt) → HTTP Transport → SAP ADT REST API**.

Write operations follow a strict lifecycle: **GetSource → SyntaxCheck → Lock → UpdateSource → Unlock → Activate**. The `EditSource` and `WriteSource` workflows handle this automatically.

## Adding a New Tool

1. **Add ADT client method** in the appropriate `pkg/adt/` file (read ops in `client.go`, writes in `crud.go`, dev tools in `devtools.go`, etc.)
2. **Add handler** in `internal/mcp/handlers_<category>.go` — create a new handler file if no existing category fits
3. **Register tool** in `internal/mcp/tools_register.go` within `registerTools()` — add it to the `allTools` map with its MCP schema
4. **Add to focused mode** (if appropriate) in `internal/mcp/tools_focused.go`
5. **Add to a tool group** (if applicable) in `internal/mcp/tools_groups.go`

### ADT Client Method Pattern

```go
// Read operation — returns parsed data
func (c *Client) GetSomething(ctx context.Context, name string) (*Result, error) {
    url := fmt.Sprintf("/sap/bc/adt/path/%s", name)
    resp, err := c.http.Get(ctx, url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    // Parse XML/JSON response
}

// Write operation — uses stateful session
func (c *Client) UpdateSomething(ctx context.Context, name, content string) error {
    url := fmt.Sprintf("/sap/bc/adt/path/%s", name)
    return c.http.Put(ctx, url, "text/plain", strings.NewReader(content))
}
```

### MCP Handler Pattern

```go
func (s *Server) handleNewTool(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
    name, _ := getString(args, "name")
    result, err := s.adtClient.NewMethod(ctx, name)
    if err != nil {
        return mcp.NewToolResultError(err.Error()), nil
    }
    return mcp.NewToolResultText(formatResult(result)), nil
}
```

Handlers return `(result, nil)` — never `(nil, error)`. Errors go through `mcp.NewToolResultError()`.

## Key Conventions

- **Functional options** for client configuration: `adt.NewClient(url, user, pass, adt.WithClient("001"), adt.WithInsecureSkipVerify())`
- **Safety system**: All write operations check `SafetyConfig` before execution. Operation types are single-letter codes: R(ead), S(earch), Q(uery), F(ree SQL), C(reate), U(pdate), D(elete), A(ctivate), T(est), L(ock), I(ntelligence), W(orkflow), X(transport)
- **Tool modes**: `focused` (default, 81 tools), `expert` (122 tools), `hyperfocused` (1 universal tool). Mode determines which tools are registered at startup.
- **Handler files** are split by domain: `handlers_source.go`, `handlers_crud.go`, `handlers_debugger.go`, etc. Keep handlers in the file matching their category.
- **ADT XML types** live in `pkg/adt/xml.go`. ADT responses are XML; use Go struct tags for parsing.
- **Config priority**: CLI flags > Environment variables > `.env` file > Defaults. Managed by viper.
- **Integration tests** use build tag `integration` and require `SAP_URL`, `SAP_USER`, `SAP_PASSWORD`, `SAP_CLIENT` env vars. They create objects in the `$TMP` package and clean up after themselves.
- **Never commit** `.env`, `cookies.txt`, or `.mcp.json` — all are in `.gitignore`.
- **Reports** follow the naming convention `reports/YYYY-MM-DD-NNN-title.md` with sequential numbering per day.
- **Releases** use GoReleaser (`.goreleaser.yml`) with git-cliff changelogs (`cliff.toml`), triggered via the `Release` workflow dispatch.
