# CLI Coding Agents + VSP (SAP ADT MCP Server)

Guide for setting up CLI coding assistants to work with SAP via [VSP (vibing-steampunk)](https://github.com/oisee/vibing-steampunk).

**VSP** is an MCP server that gives AI assistants access to SAP ADT API: read/write code, debug, test, transports, and more.

**Translations:** [Русский](README_RU.md) | [Українська](README_UA.md) | [Español](README_ES.md)

---

## Summary Table

| Tool | Model Access | Availability | MCP | Install | VSP Config |
|---|---|---|---|---|---|
| **Gemini CLI** | Gemini models | Free tier available; paid/API-backed usage also available | Yes | `npm i -g @google/gemini-cli` | `.gemini/settings.json` |
| **Claude Code** | Claude models | Paid usage or subscription-backed access | Yes | `curl -fsSL https://claude.ai/install.sh \| bash` | `.mcp.json` |
| **GitHub Copilot** | Multi-model (plan-dependent) | Free tier available; paid plans unlock more limits/models | Yes | `npm i -g @github/copilot` | `.copilot/mcp-config.json` |
| **OpenAI Codex** | OpenAI coding models / ChatGPT-linked access | Limited or plan-dependent access; API usage also available | Yes | `npm i -g @openai/codex` | `codex.toml` |
| **Qwen Code** | Qwen models | Free tier available; BYOK/API-backed usage also available | Yes | `npm i -g @qwen-code/qwen-code` | `.qwen/settings.json` |
| **OpenCode** | Multi-provider BYOK | Depends on your provider/account | Yes | `brew install anomalyco/tap/opencode` | `opencode.json` |
| **Goose** | Multi-provider BYOK | Depends on your provider/account | Yes | `brew install block-goose-cli` | `~/.config/goose/config.yaml` |
| **Mistral Vibe** | Mistral API or local models | Local/Ollama path can be free; API usage is provider-billed | Yes | `pip install mistral-vibe` | `.vibe/config.toml` |

> **BYOK** = Bring Your Own Key
>
> Availability, pricing, request limits, and model lineups change often. Treat this table as orientation, not a permanent price sheet.

---

## 1. Gemini CLI (Google)

**One strong free-tier option.** Exact limits and model access depend on Google's current Gemini CLI and account policy.

### Install

```bash
npm install -g @google/gemini-cli
# or without installing:
npx @google/gemini-cli
```

### First Run

```bash
cd /path/to/your/project
gemini
# First run — sign in with Google account
```

### VSP Setup

Create `.gemini/settings.json` in the project folder:

```json
{
  "mcpServers": {
    "sap-adt": {
      "command": "/path/to/vsp-darwin-arm64",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<password>"
      }
    }
  }
}
```

### Test MCP

```
> Use the SearchObject tool to find classes starting with ZCL_VDB
```

### Links
- GitHub: https://github.com/google-gemini/gemini-cli
- Docs: https://ai.google.dev/gemini-api/docs

---

## 2. Claude Code (Anthropic)

Creator of the MCP standard. Deepest MCP integration.

### Install

```bash
curl -fsSL https://claude.ai/install.sh | bash
# or:
brew install claude-code
```

### First Run

```bash
cd /path/to/your/project
claude
# Requires Claude Pro ($20/mo) or Anthropic API key
```

### VSP Setup

Create `.mcp.json` in the project root:

```json
{
  "mcpServers": {
    "sap-adt": {
      "command": "/path/to/vsp-darwin-arm64",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<password>"
      }
    }
  }
}
```

### Test MCP

```
> Use the SearchObject tool to find classes starting with ZCL_VDB
```

### Links
- GitHub: https://github.com/anthropics/claude-code
- Docs: https://docs.anthropic.com/en/docs/claude-code

---

## 3. GitHub Copilot CLI

Multi-model: Claude, GPT-5, Gemini — switch between models on the fly.

### Install

```bash
npm install -g @github/copilot
# or via GitHub CLI:
gh extension install github/gh-copilot
```

### First Run

```bash
cd /path/to/your/project
github-copilot
# Requires GitHub Copilot subscription ($10+/mo)
```

### VSP Setup

Create `.copilot/mcp-config.json` in the project folder:

```json
{
  "mcpServers": {
    "sap-adt": {
      "command": "/path/to/vsp-darwin-arm64",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<password>"
      }
    }
  }
}
```

### Test MCP

```
> Use the sap-adt tools to search for objects starting with ZCL_VDB
```

### Links
- GitHub: https://github.com/github/copilot-cli
- Docs: https://docs.github.com/en/copilot

---

## 4. OpenAI Codex CLI

> **Detailed guide:** [codex.md](codex.md) -- full Codex agent configuration with SAP Q/A testing scenarios, safety setup, and example prompts.

### Install

```bash
npm install -g @openai/codex
# or:
brew install --cask codex
```

### First Run

```bash
cd /path/to/your/project
codex
# Requires ChatGPT Plus ($20/mo) or OpenAI API key
```

### VSP Setup

Create `codex.toml` in the project root (TOML format, not JSON):

```toml
[mcp_servers.sap-adt]
command = "/path/to/vsp"
enabled = true

[mcp_servers.sap-adt.env]
SAP_URL = "https://your-sap-host:44300"
SAP_USER = "YOUR_USER"
SAP_PASSWORD = "<password>"
SAP_CLIENT = "001"
SAP_READ_ONLY = "true"
SAP_MODE = "focused"
```

> **Note:** `codex.toml` contains credentials — add it to `.gitignore`. Project-local vs `~/.codex/config.toml` scope depends on your Codex version.

> **Safety tip:** Use `SAP_READ_ONLY=true` for Q/A testing, or `SAP_ALLOWED_OPS=RSQ` to allow read + search + query only. See [codex.md](codex.md) for full safety options.

### Links
- GitHub: https://github.com/openai/codex
- Detailed VSP config: [codex.md](codex.md)

---

## 5. Qwen Code CLI (Alibaba)

**Free.** 1000 requests/day via Qwen OAuth.

### Install

```bash
npm install -g @qwen-code/qwen-code@latest
# or:
brew install qwen-code
```

### First Run

```bash
cd /path/to/your/project
qwen-code
# First run — sign in via Qwen OAuth (free)
```

### VSP Setup

Create `.qwen/settings.json` in the project folder:

```json
{
  "mcpServers": {
    "sap-adt": {
      "command": "/path/to/vsp-darwin-arm64",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<password>"
      },
      "timeout": 60000,
      "trust": false
    }
  }
}
```

### Links
- GitHub: https://github.com/QwenLM/qwen-code
- MCP Docs: https://qwenlm.github.io/qwen-code-docs/en/developers/tools/mcp-server/

---

## 6. OpenCode CLI

**Free.** 75+ models, works with any provider (Anthropic, OpenAI, Google, Ollama...).

### Install

```bash
brew install anomalyco/tap/opencode
# or:
npm i -g opencode-ai@latest
# or:
curl -fsSL https://opencode.ai/install | bash
```

### First Run

```bash
cd /path/to/your/project
opencode
# Enter your provider's API key (or connect GitHub Copilot)
```

### VSP Setup

Create `opencode.json` in the project root:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "azure-openai": {
      "options": {
        "apiKey": "{env:AZURE_OPENAI_API_KEY}",
        "resourceName": "your-resource",
        "apiVersion": "{env:AZURE_OPENAI_API_VERSION}"
      }
    }
  },
  "mcp": {
    "sap-adt": {
      "type": "local",
      "command": ["/path/to/vsp-darwin-arm64"],
      "enabled": true,
      "environment": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<password>"
      },
      "timeout": 60000
    }
  }
}
```

> **Note:** The provider can be replaced with any other (Anthropic, OpenAI, Google, Ollama, etc.).

### Links
- GitHub: https://github.com/opencode-ai/opencode
- MCP Docs: https://opencode.ai/docs/mcp-servers/

---

## 7. Goose (Block / Linux Foundation)

**Free.** 75+ providers, written in Rust. MCP is a core architectural principle.

### Install

```bash
brew install block-goose-cli
# or:
curl -fsSL https://github.com/block/goose/releases/download/stable/download_cli.sh | bash
```

### First Run

```bash
goose configure
# Choose provider (Azure, Anthropic, OpenAI, Google, Ollama...)
# Enter API key
goose
```

### VSP Setup

Copy config to `~/.config/goose/config.yaml`:

```yaml
extensions:
  sap-adt:
    enabled: true
    name: sap-adt
    type: stdio
    cmd: "/path/to/vsp-darwin-arm64"
    args: []
    description: "SAP ABAP Development Tools via MCP"
    timeout: 300
    envs:
      SAP_URL: "https://your-sap-host:44300"
      SAP_USER: "YOUR_USER"
      SAP_PASSWORD: "<password>"
```

### Or add via CLI

```bash
goose configure
# → Add extension → stdio → enter path to vsp and env variables
```

### Verify

```bash
goose info -v
```

### Links
- GitHub: https://github.com/block/goose
- Docs: https://block.github.io/goose/docs/guides/config-files

---

## 8. Mistral Vibe CLI

Supports **local models** via Ollama (completely free).

### Install

```bash
pip install mistral-vibe
# or:
brew install mistral-vibe
```

### First Run

```bash
cd /path/to/your/project
vibe
# Requires Mistral API key or configured Ollama
```

### VSP Setup

Create `.vibe/config.toml` in the project folder:

```toml
# Provider (Ollama for free local models)
[[providers]]
name = "ollama"
api_base = "http://localhost:11434/v1"
api_key_env_var = "OLLAMA_API_KEY"
api_style = "openai"
backend = "generic"

# Models
[[models]]
name = "devstral-small-2:latest"
provider = "ollama"
alias = "devstral"
temperature = 0.2

[[models]]
name = "qwen2.5-coder:32b"
provider = "ollama"
alias = "qwen-coder"
temperature = 0.2

# VSP MCP server
[[mcp_servers]]
name = "sap-adt"
transport = "stdio"
command = "/path/to/vsp-darwin-arm64"
```

Create `.vibe/.env`:
```bash
OLLAMA_API_KEY=not-required
SAP_URL=https://your-sap-host:44300
SAP_USER=YOUR_USER
SAP_PASSWORD=<password>
```

### Links
- GitHub: https://github.com/mistralai/mistral-vibe

---

## MCP Config Formats — Cheat Sheet

| Tool | Format | Config File | MCP Key | Env Key |
|---|---|---|---|---|
| Claude Code | JSON | `.mcp.json` | `mcpServers` | `env` |
| Gemini CLI | JSON | `.gemini/settings.json` | `mcpServers` | `env` |
| Copilot | JSON | `.copilot/mcp-config.json` | `mcpServers` | `env` |
| Codex | TOML | `codex.toml` | `mcp_servers.*` | `[env]` section |
| Qwen Code | JSON | `.qwen/settings.json` | `mcpServers` | `env` |
| OpenCode | JSON | `opencode.json` | `mcp` | `environment` |
| Goose | YAML | `~/.config/goose/config.yaml` | `extensions` | `envs` |
| Mistral Vibe | TOML | `.vibe/config.toml` | `[[mcp_servers]]` | `.vibe/.env` |

---

## Recommendations

### Free Options for Working with VSP

1. **Gemini CLI** — strong free-tier option if its current quota and auth model work for you
2. **Qwen Code** — another free-tier option, but check current OAuth and quota terms
3. **Mistral Vibe + Ollama** — effectively free if you already have local hardware for models
4. **OpenCode / Goose** — the CLI itself is easy to use, but cost depends on the provider you connect

### Best Quality

1. **Claude Code** — usually the most native MCP experience
2. **GitHub Copilot** — good if you want plan-dependent multi-model switching
3. **Gemini CLI** — strong option when the current free tier is enough

---

## Test MCP Server

To test MCP connectivity without SAP, use the echo server:

```bash
python3 /path/to/mcp-echo-server.py
```

Example config (Claude Code / Codex / Gemini):
```json
{
  "mcpServers": {
    "echo": {
      "command": "python3",
      "args": ["/path/to/mcp-echo-server.py"]
    }
  }
}
```

---

## VSP — Quick Start

```bash
# Download binary
curl -LO https://github.com/oisee/vibing-steampunk/releases/latest/download/vsp-darwin-arm64
chmod +x vsp-darwin-arm64

# Or build from source
git clone https://github.com/oisee/vibing-steampunk.git
cd vibing-steampunk && make build
```

Environment variables:
```bash
export SAP_URL=https://your-sap-host:44300
export SAP_USER=your-username
export SAP_PASSWORD=your-password
export SAP_CLIENT=001          # default
export SAP_MODE=focused        # focused (100 tools) or expert (147)
```

More info: [VSP README](https://github.com/oisee/vibing-steampunk) | [MCP Usage Guide](https://github.com/oisee/vibing-steampunk/blob/main/MCP_USAGE.md)
