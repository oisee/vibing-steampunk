# Context Dump: Hyperfocused Mode Refactor

**Date:** 2026-03-18
**Session ID:** 002
**Subject:** Unified SAP_MODE with hyperfocused (one-tool) mode

---

## What Was Done

### Problem
- `.mcp.json` had all 3 systems running in `focused` mode (81 tools) — no `SAP_MODE` was set
- The one-tool "universal" mode existed but was controlled by a separate axis: `SAP_TOOL_MODE=universal` / `--tool-mode`
- Only `a4h-110-adt` had `SAP_TOOL_MODE=universal`, others didn't
- Two separate config axes (`SAP_MODE` + `SAP_TOOL_MODE`) was confusing

### Solution: Unified `SAP_MODE` with 3 values

Merged `SAP_TOOL_MODE` into `SAP_MODE` as a third value `hyperfocused`:

| Mode | Tools | Description |
|------|-------|-------------|
| `focused` | 81 | Essential tools (default) |
| `expert` | 122 | All tools individually |
| `hyperfocused` | 1 | Single `SAP(action, target, params)` universal tool |

### Files Changed

1. **`internal/mcp/server.go`**
   - Removed `ToolMode` field from `ServerConfig` struct
   - Updated `registerTools()` call — removed `cfg.ToolMode` param

2. **`internal/mcp/tools_register.go`**
   - `registerTools()` signature: removed `toolMode` parameter
   - Changed check from `toolMode == "universal"` to `mode == "hyperfocused"`

3. **`cmd/vsp/main.go`**
   - Removed `--tool-mode` flag and its viper binding
   - Removed `SAP_TOOL_MODE` env var handling
   - Updated `--mode` description to include `hyperfocused`
   - Updated validation to accept 3 modes
   - Updated root command description
   - Removed verbose logging for tool mode

4. **`cmd/vsp/config_cmd.go`**
   - Updated `config tools init` usage and descriptions to mention `hyperfocused`

5. **`.mcp.json`**
   - All 3 systems set to `SAP_MODE=hyperfocused`
   - Removed all `SAP_TOOL_MODE` references
   - Removed stale `SAP_MODE=expert` that was incorrectly added earlier in session

### Build Status
- Compiles clean: `go build ./cmd/vsp`
- Binary rebuilt: `build/vsp`
- All tests pass: `go test ./...`

### What Was NOT Changed
- `handlers_universal.go` — untouched, still implements the universal SAP tool
- `handlers_help.go` — untouched, help docs for universal tool
- Universal tool routing logic — unchanged, just triggered by different config path

## TODO / Next Steps
- Restart MCP servers to pick up new `.mcp.json`
- Verify single SAP tool appears in MCP tool list
- Consider updating CLAUDE.md to reflect the 3-mode setup
- Consider updating README with hyperfocused mode docs
