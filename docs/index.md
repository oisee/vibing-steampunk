# vsp — AI-Agentic ABAP Development

**ADT ↔ MCP Bridge**: Gives Claude (and 7 other AI coding agents) full access to SAP ABAP Development Tools.
Read code, write code, debug, deploy, run tests — all through natural language.

> Works with ECC, S/4HANA, BTP ABAP — everywhere ADT is available.

---

## What vsp does

vsp is a single Go binary that runs as an MCP server. It exposes SAP ADT APIs as tools that AI assistants can call directly:

```
You → Claude → vsp → SAP ADT → Your ABAP system
```

No Eclipse. No browser. No copy-paste. The AI reads your code, makes changes, runs tests, and debugs — all in one conversation.

## Key Features

| Feature | Description |
|---------|-------------|
| **81 / 122 Tools** | Focused mode (81 tools, default) or Expert mode (122 tools) |
| **AI Debugger** | Breakpoints, step, inspect stack & variables via WebSocket |
| **RAP OData E2E** | Create CDS views, BDEFs, Service Definitions & Bindings |
| **ExecuteABAP** | Run arbitrary ABAP code via unit test wrapper |
| **AI-Powered RCA** | Autonomously investigate short dumps, traces, call graphs |
| **DSL & Workflows** | Fluent Go API + YAML automation for CI/CD pipelines |
| **abapGit Export** | Export packages in abapGit-compatible format (158 object types) |
| **Safety Controls** | Read-only mode, operation filtering, package restrictions |
| **8 AI Agents** | Claude, Gemini, Copilot, Codex, Qwen, OpenCode, Goose, Mistral |

## Quick Start

```bash
# Download binary (Linux/macOS/Windows — 9 platforms)
curl -LO https://github.com/oisee/vibing-steampunk/releases/latest/download/vsp-linux-amd64
chmod +x vsp-linux-amd64

# Or build from source
git clone https://github.com/oisee/vibing-steampunk && cd vibing-steampunk
go build -o vsp ./cmd/vsp
```

Connect to Claude Code by creating `.mcp.json` in your project:

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

→ See [Getting Started](getting-started.md) for detailed setup including Claude Desktop, other agents, and cookie authentication.

## Supported AI Agents

| Agent | LLM | Free? |
|-------|-----|-------|
| **Claude Code** | Claude Opus/Sonnet 4.6 | No ($20+/mo) |
| **Gemini CLI** | Gemini 2.5 Pro/Flash | Yes (1000 req/day) |
| **GitHub Copilot** | Claude, GPT-5, Gemini | No ($10+/mo) |
| **OpenAI Codex** | GPT-4.1 / GPT-5-Codex | No ($20+/mo) |
| **Qwen Code** | Qwen3-Coder | Yes (1000 req/day) |
| **OpenCode** | 75+ models (BYOK) | Yes (own key) |
| **Goose** | 75+ providers (BYOK) | Yes (own key) |
| **Mistral Vibe** | Devstral 2, local Ollama | Yes (free) |

→ See [CLI Agents](cli-agents/README.md) for full setup guides with config examples.

## Navigation

<div class="grid cards" markdown>

- :material-rocket-launch: **[Getting Started](getting-started.md)**
  Install, connect, and run your first command

- :material-cog: **[Configuration](configuration.md)**
  All flags, env vars, and `.env` / `.vsp.json` options

- :material-tools: **[Tool Reference](tools/reference.md)**
  All 122 tools with descriptions and mode availability

- :material-bug: **[ABAP Debugger](features/debugging.md)**
  Breakpoints, step execution, SAP GUI integration

- :material-database: **[RAP / OData E2E](features/rap-odata.md)**
  Create complete OData services with AI

- :material-shield-check: **[Safety Controls](features/safety.md)**
  Protect production systems from unintended changes

</div>

## Project Status

| Metric | Value |
|--------|-------|
| Tools | 122 (81 focused / 122 expert) |
| Unit Tests | 270+ |
| Platforms | 9 (Linux, macOS, Windows × amd64/arm64/386) |
| Latest | v2.27.0 |
