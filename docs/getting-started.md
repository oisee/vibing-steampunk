# Getting Started

## 1. Install vsp

### Download binary (recommended)

Pre-built binaries are available for 9 platforms on the [releases page](https://github.com/oisee/vibing-steampunk/releases/latest).

```bash
# Linux (amd64)
curl -LO https://github.com/oisee/vibing-steampunk/releases/latest/download/vsp-linux-amd64
chmod +x vsp-linux-amd64 && mv vsp-linux-amd64 /usr/local/bin/vsp

# macOS (arm64 / Apple Silicon)
curl -LO https://github.com/oisee/vibing-steampunk/releases/latest/download/vsp-darwin-arm64
chmod +x vsp-darwin-arm64 && mv vsp-darwin-arm64 /usr/local/bin/vsp

# Windows (amd64) — download vsp-windows-amd64.exe from releases
```

### Build from source

```bash
git clone https://github.com/oisee/vibing-steampunk.git
cd vibing-steampunk
go build -o vsp ./cmd/vsp   # or: make build
```

Requires Go 1.23+.

---

## 2. Connect to your SAP system

vsp needs a SAP system with ADT enabled (standard in AS ABAP 7.40+).

### Option A — Environment variables / .env file

Create a `.env` file in your working directory:

```bash
SAP_URL=https://your-sap-host:44300
SAP_USER=your-username
SAP_PASSWORD=your-password
SAP_CLIENT=001
```

vsp reads `.env` automatically when starting as an MCP server.

### Option B — CLI flags

```bash
vsp --url https://your-sap-host:44300 --user DEVELOPER --password secret
```

### Option C — Cookie authentication (SSO / no password)

```bash
# From a cookie file (Netscape format, exported by browser extensions)
vsp --url https://host:44300 --cookie-file cookies.txt

# Or inline cookie string
vsp --url https://host:44300 --cookie-string "sap-usercontext=abc; SAP_SESSIONID=xyz"
```

---

## 3. Connect your AI agent

### Claude Code

Create `.mcp.json` in your project root:

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

### Claude Desktop

Add to `~/.config/claude/claude_desktop_config.json` (Linux/macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

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

### Other agents

See [CLI Agents](cli-agents/README.md) for setup guides for Gemini CLI, GitHub Copilot, OpenAI Codex, Qwen Code, OpenCode, Goose, and Mistral Vibe.

---

## 4. Verify the connection

Ask your AI assistant:

```
Call GetSystemInfo to verify the connection to SAP.
```

You should get back the system ID, release, database type, and host name.

---

## 5. First commands to try

```
# Read source code
GetSource(type="PROG", name="RSDEMO_BRF")

# Search for objects
SearchObject(query="ZCL_*", max_results=10)

# Run unit tests on a package
RunUnitTests(package="$TMP")

# Check syntax
SyntaxCheck(type="PROG", name="ZTEST")
```

---

## Working with transportable packages

By default, vsp only allows changes to local packages (`$*`). To work with transportable packages, add these settings:

```json
{
  "mcpServers": {
    "abap-adt": {
      "command": "/path/to/vsp",
      "env": {
        "SAP_URL": "...",
        "SAP_USER": "...",
        "SAP_PASSWORD": "...",
        "SAP_ALLOW_TRANSPORTABLE_EDITS": "true",
        "SAP_ALLOWED_TRANSPORTS": "DEVK*",
        "SAP_ALLOWED_PACKAGES": "ZPROD,$TMP,$*,Z*"
      }
    }
  }
}
```

See [Safety Controls](features/safety.md) for details on all protection options.

---

## Multiple SAP systems

To connect to more than one SAP system (dev, QA, prod), or to use vsp with multiple AI clients at the same time, see the dedicated guide:

→ **[Multiple Systems & Clients](mcp-multi-system.md)**

Covers: multiple `mcpServers` entries, naming conventions, per-environment safety settings, keeping secrets out of config, and CLI multi-system profiles.

---

## Next steps

- [Configuration](configuration.md) — full reference for all options
- [Tool Reference](tools/reference.md) — all 122 tools
- [Safety Controls](features/safety.md) — protect your production system
- [ABAP Debugger](features/debugging.md) — AI-driven debugging
