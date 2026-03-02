# vsp Reviewer Guide

> A hands-on checklist for anyone who wants to kick the tires.
> No SAP system required for most tasks.

## Build It (30 seconds)

```bash
git clone https://github.com/oisee/vibing-steampunk.git
cd vibing-steampunk
go build -o vsp ./cmd/vsp
./vsp --version
```

That's it. Single binary, zero dependencies beyond Go 1.23+.

---

## Task 1: Read the --help

```bash
./vsp --help
```

**What to spotlight:**
- Two modes of operation (MCP Server + CLI)
- 81 focused / 122 expert tools
- Enterprise safety flags (`--read-only`, `--allowed-packages`, `--disallowed-ops`)
- Configuration files section (`.env`, `.vsp.json`, `.mcp.json`)
- Subcommand list (search, source, export, debug, lua, workflow, config, systems)

**Questions to ask yourself:**
- Does the help make it clear this is both an MCP server AND a CLI tool?
- Are the safety examples convincing for enterprise use?
- Would you know how to get started after reading this?

---

## Task 2: Run the Tests (no SAP needed)

```bash
go test ./...
```

244+ unit tests, all pass without any SAP connection.

**Dig deeper:**

```bash
# Safety system (25 tests) - read-only, package restrictions, operation filtering
go test -v -run TestSafety ./pkg/adt/

# Cookie auth parsing (6 scenarios)
go test -v -run TestCookie ./pkg/adt/

# MCP server tool registration
go test -v ./internal/mcp/

# Cache subsystem (in-memory + SQLite)
go test -v ./pkg/cache/

# DSL & workflow engine
go test -v ./pkg/dsl/

# Code coverage
go test -cover ./...
```

**What to spotlight:** test coverage, mock patterns, safety edge cases.

**Try to break:**
- Run `go vet ./...` and `go test -race ./...` - any warnings?
- Check if tests are deterministic (run twice, same results?)

---

## Task 3: Play With Config (no SAP needed)

```bash
# Generate example config files
./vsp config init
ls -la .env.example .vsp.json.example .mcp.json.example

# See what config vsp would use right now
./vsp config show

# Test environment variable parsing
SAP_MODE=expert SAP_READ_ONLY=true ./vsp config show
SAP_ALLOWED_PACKAGES='Z*,$TMP' ./vsp config show
```

**Multi-system profiles:**

```bash
# Copy and edit the example
cp .vsp.json.example .vsp.json
# Edit .vsp.json to add your own system names (no real credentials needed)
./vsp systems
```

**Config conversion (bidirectional):**

```bash
# If you have a .mcp.json from Claude Desktop:
./vsp config mcp-to-vsp

# Generate .mcp.json from .vsp.json:
./vsp config vsp-to-mcp
```

**What to spotlight:**
- Config priority: CLI flags > env vars > .env > defaults
- `.vsp.json` multi-system profiles (dev, test, prod with different safety settings)
- Conversion between MCP and vsp config formats

**Try to break:**
- What happens with conflicting config? (`SAP_MODE=expert --mode focused`)
- Empty values? (`SAP_URL= ./vsp config show`)
- Weird package patterns? (`SAP_ALLOWED_PACKAGES='***'`)

---

## Task 4: Tool Visibility (no SAP needed)

vsp lets you enable/disable individual tools per system profile:

```bash
# Create a tool visibility map for focused mode
./vsp config tools init --mode focused
./vsp config tools list

# Disable a tool
./vsp config tools disable AMDPDebuggerStart
./vsp config tools list | grep AMDP

# Re-enable it
./vsp config tools enable AMDPDebuggerStart
```

**What to spotlight:** granular tool control beyond just focused/expert modes.

**Try to break:**
- Disable a tool that doesn't exist
- Enable a tool in focused mode that's normally expert-only
- What happens if `.vsp.json` has invalid JSON?

---

## Task 5: Safety System (code review)

The safety system is in `pkg/adt/safety.go` with 25 tests in `safety_test.go`.

**Key scenarios to review:**

| Flag | What It Does |
|------|-------------|
| `--read-only` | Blocks all write operations (create, update, delete, activate) |
| `--block-free-sql` | Blocks `RunQuery` (arbitrary SQL execution) |
| `--allowed-packages 'Z*,$TMP'` | Only allows operations on matching packages |
| `--allowed-ops RSQ` | Whitelist: only Read, Search, Query |
| `--disallowed-ops CDUA` | Blacklist: block Create, Delete, Update, Activate |
| `--allow-transportable-edits` | Must opt-in to edit objects in transportable packages |

**What to spotlight:**
- Can an AI agent escape the sandbox?
- Are wildcard patterns (`Z*`) handled correctly?
- What happens when `--allowed-ops` and `--disallowed-ops` conflict?

**Files to read:**
- `pkg/adt/safety.go` - the implementation
- `pkg/adt/safety_test.go` - the test matrix
- `internal/mcp/server.go` - where safety checks are called

---

## Task 6: MCP Integration (with any MCP client)

If you have Claude Desktop, Gemini CLI, Copilot, or any MCP client:

```bash
# Generate Claude Desktop config
./vsp config init
cat .mcp.json.example
```

Ready-to-use configs for 8 AI agents live in `docs/cli-agents/`:
- Claude Code, Gemini CLI, GitHub Copilot, OpenAI Codex
- Qwen Code, OpenCode, Goose, Mistral Vibe

**What to spotlight:**
- Does the MCP config format look right for your agent?
- Is the tool count (81/122) reasonable for an LLM context window?
- Do the tool descriptions make sense to an AI?

---

## Task 7: With SAP Access (optional)

If you have an SAP system with ADT enabled:

```bash
# Quick smoke test
./vsp --url https://host:44300 --user dev --password secret --verbose

# CLI mode
./vsp -s dev search "ZCL_*"
./vsp -s dev source CLAS ZCL_SOMETHING

# Safety sandbox
./vsp --url ... --user ... --password ... --read-only --verbose
# Then try to write something through MCP - should be blocked

# Export a package
./vsp -s dev export '$TMP' -o tmp-backup.zip
```

**What to spotlight:**
- Does it connect on first try?
- Are error messages clear when auth fails?
- Does `--verbose` give enough debug info?
- Does `--read-only` actually block writes?

---

## Task 8: Code Quality (for Go developers)

```bash
# Static analysis
go vet ./...

# Race condition detection
go test -race ./...

# Binary size
ls -lh vsp

# Dependency tree
go mod graph | head -20

# Check for outdated deps
go list -m -u all 2>/dev/null | grep '\[' | head -10
```

**Architecture overview:** `docs/architecture.md` has Mermaid diagrams.

**Key files by size/complexity:**

| File | What | Why It Matters |
|------|------|---------------|
| `internal/mcp/server.go` | All 122 tool handlers | The core of vsp |
| `pkg/adt/client.go` | ADT HTTP client | Every SAP call goes through here |
| `pkg/adt/safety.go` | Safety checks | Enterprise trust boundary |
| `cmd/vsp/config_cmd.go` | Config management | Multi-system + tool visibility |
| `cmd/vsp/debug.go` | Interactive debugger | Most complex CLI subcommand |

---

## Quick Reference

| What | Command | SAP Needed? |
|------|---------|------------|
| Build | `go build -o vsp ./cmd/vsp` | No |
| Unit tests | `go test ./...` | No |
| Help text | `./vsp --help` | No |
| Generate configs | `./vsp config init` | No |
| Show config | `./vsp config show` | No |
| List systems | `./vsp systems` | No |
| Tool visibility | `./vsp config tools list` | No |
| Shell completion | `./vsp completion zsh` | No |
| Search objects | `./vsp -s dev search "Z*"` | **Yes** |
| Read source | `./vsp -s dev source CLAS ZCL_X` | **Yes** |
| Export package | `./vsp -s dev export '$PKG'` | **Yes** |
| Debug session | `./vsp -s dev debug` | **Yes** |
| Lua REPL | `./vsp -s dev lua` | **Yes** |

---

## Found Something?

- Open an issue: https://github.com/oisee/vibing-steampunk/issues
- PRs welcome - especially for: test coverage, error messages, documentation, new MCP agent configs
