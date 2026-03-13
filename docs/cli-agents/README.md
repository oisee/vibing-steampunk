# CLI Coding Agents + VSP (SAP ADT MCP Server)

Guide for setting up CLI coding assistants to work with SAP via [VSP (vibing-steampunk)](https://github.com/oisee/vibing-steampunk).

**VSP** is an MCP server that gives AI assistants access to SAP ADT API: read/write code, debug, test, transports, and more.

**Translations:** [Русский](README_RU.md) | [Українська](README_UA.md) | [Español](README_ES.md)

---

## Summary Table

| Tool | LLM | Free? | MCP | Install | VSP Config |
|---|---|---|---|---|---|
| **Gemini CLI** | Gemini 2.5 Pro/Flash, 3 Pro | **Yes** (1000 req/day) | Yes | `npm i -g @google/gemini-cli` | `.gemini/settings.json` |
| **Claude Code** | Claude Opus/Sonnet 4.6 | No ($20+/mo) | Yes | `curl -fsSL https://claude.ai/install.sh \| bash` | `.mcp.json` |
| **GitHub Copilot** | Claude, GPT-5, Gemini | No ($10+/mo) | Yes | `npm i -g @github/copilot` | `.copilot/mcp-config.json` |
| **OpenAI Codex** | GPT-5-Codex, GPT-4.1 | No ($20+/mo) | Yes | `npm i -g @openai/codex` | `.mcp.json` |
| **Qwen Code** | Qwen3-Coder | **Yes** (1000 req/day) | Yes | `npm i -g @qwen-code/qwen-code` | `.qwen/settings.json` |
| **OpenCode** | 75+ models (BYOK) | **Yes** (own key) | Yes | `brew install anomalyco/tap/opencode` | `opencode.json` |
| **Goose** | 75+ providers (BYOK) | **Yes** (own key) | Yes | `brew install block-goose-cli` | `~/.config/goose/config.yaml` |
| **Mistral Vibe** | Devstral 2, local models | No (API) / **Yes** (Ollama) | Yes | `pip install mistral-vibe` | `.vibe/config.toml` |

> **BYOK** = Bring Your Own Key

---

## 1. Gemini CLI (Google)

**Best free option.** 1000 requests/day free with a Google account.

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

Create `.mcp.json` in the project root (same format as Claude Code):

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

### Links
- GitHub: https://github.com/openai/codex

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
| Codex | JSON | `.mcp.json` | `mcpServers` | `env` |
| Qwen Code | JSON | `.qwen/settings.json` | `mcpServers` | `env` |
| OpenCode | JSON | `opencode.json` | `mcp` | `environment` |
| Goose | YAML | `~/.config/goose/config.yaml` | `extensions` | `envs` |
| Mistral Vibe | TOML | `.vibe/config.toml` | `[[mcp_servers]]` | `.vibe/.env` |

---

## Recommendations

### Free Options for Working with VSP

1. **Gemini CLI** — best free option. 1000 requests/day, Gemini 2.5 Pro with 1M token context
2. **Qwen Code** — 1000 requests/day free via Qwen OAuth
3. **Mistral Vibe + Ollama** — completely free with local models (needs powerful GPU/Mac)
4. **OpenCode / Goose** — free CLIs, but need an API key from some provider

### Best Quality

1. **Claude Code** (Opus 4.6) — MCP creator, best integration
2. **GitHub Copilot** (multi-model) — switch between Claude/GPT/Gemini
3. **Gemini CLI** (Gemini 2.5 Pro) — strong model + free

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
export SAP_MODE=focused        # focused (48 tools) or expert (96)
```

More info: [VSP README](https://github.com/oisee/vibing-steampunk) | [MCP Usage Guide](https://github.com/oisee/vibing-steampunk/blob/main/MCP_USAGE.md)
