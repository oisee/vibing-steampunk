# Multiple Systems & Clients

This page covers how to connect vsp to more than one SAP system, and how to use vsp with multiple AI clients at the same time.

---

## Multiple SAP systems in MCP mode

Each vsp process connects to exactly **one** SAP system. To expose multiple systems to an AI agent, register multiple `mcpServers` entries — one per system. Each entry starts its own vsp process with different environment variables.

### Example: dev + production

```json
{
  "mcpServers": {
    "sap-dev": {
      "command": "/usr/local/bin/vsp",
      "env": {
        "SAP_URL": "http://dev.example.com:50000",
        "SAP_USER": "DEVELOPER",
        "SAP_PASSWORD": "devpass",
        "SAP_CLIENT": "001"
      }
    },
    "sap-prod": {
      "command": "/usr/local/bin/vsp",
      "env": {
        "SAP_URL": "https://prod.example.com:44300",
        "SAP_USER": "MONITOR",
        "SAP_PASSWORD": "monpass",
        "SAP_CLIENT": "100",
        "SAP_READ_ONLY": "true"
      }
    }
  }
}
```

The AI agent sees both systems as separate tool namespaces: `sap-dev__GetSource` and `sap-prod__GetSource`. You can tell Claude:

> "Compare the implementation of ZCL_ORDER in dev vs production."

Claude will call `GetSource` on both servers automatically.

---

## Naming conventions

Use descriptive names so the AI can distinguish systems clearly:

| Name pattern | Example | When to use |
|---|---|---|
| `sap-<role>` | `sap-dev`, `sap-prod` | Single landscape |
| `sap-<sid>` | `sap-d01`, `sap-p10` | Multiple landscapes |
| `sap-<client>-<role>` | `sap-200-qa`, `sap-100-prod` | Multi-client systems |
| `<project>-<role>` | `hrp-dev`, `hrp-prod` | Project-specific workspaces |

Avoid generic names like `abap-adt` when you have more than one system — the AI will struggle to pick the right one.

---

## Recommended safety settings per environment

```json
{
  "mcpServers": {
    "sap-sandbox": {
      "command": "/usr/local/bin/vsp",
      "env": {
        "SAP_URL": "http://sandbox:50000",
        "SAP_USER": "ADMIN",
        "SAP_PASSWORD": "...",
        "SAP_MODE": "expert",
        "SAP_ALLOW_TRANSPORTABLE_EDITS": "true"
      }
    },
    "sap-dev": {
      "command": "/usr/local/bin/vsp",
      "env": {
        "SAP_URL": "https://dev:44300",
        "SAP_USER": "DEVELOPER",
        "SAP_PASSWORD": "...",
        "SAP_ALLOWED_PACKAGES": "Z*,$TMP,$*",
        "SAP_ALLOW_TRANSPORTABLE_EDITS": "true",
        "SAP_ALLOWED_TRANSPORTS": "DEVK*"
      }
    },
    "sap-qa": {
      "command": "/usr/local/bin/vsp",
      "env": {
        "SAP_URL": "https://qa:44300",
        "SAP_USER": "TESTER",
        "SAP_PASSWORD": "...",
        "SAP_READ_ONLY": "true"
      }
    },
    "sap-prod": {
      "command": "/usr/local/bin/vsp",
      "env": {
        "SAP_URL": "https://prod:44300",
        "SAP_USER": "MONITOR",
        "SAP_PASSWORD": "...",
        "SAP_READ_ONLY": "true",
        "SAP_BLOCK_FREE_SQL": "true"
      }
    }
  }
}
```

---

## Where to put the config

### Project-level (`.mcp.json`)

Checked into the repo. Anyone who clones the project gets the same server list. Passwords are provided via environment variables at runtime, not stored in the file.

```json
{
  "mcpServers": {
    "sap-dev": {
      "command": "/usr/local/bin/vsp",
      "env": {
        "SAP_URL": "https://dev.example.com:44300",
        "SAP_USER": "DEVELOPER",
        "SAP_PASSWORD": "${VSP_DEV_PASSWORD}"
      }
    }
  }
}
```

!!! note
    Most MCP clients do NOT expand `${VAR}` in JSON — pass the password directly or use a wrapper script. See [Keeping secrets out of config](#keeping-secrets-out-of-config) below.

### Global / user-level

For Claude Code, the global MCP config lives at:

| Platform | Path |
|----------|------|
| Linux/macOS | `~/.config/claude/mcp.json` or `~/.claude/mcp.json` |
| Windows | `%APPDATA%\Claude\mcp.json` |

Systems defined globally are available in every project without needing a `.mcp.json`.

### Combining both

Project-level `.mcp.json` and global config merge — both sets of servers are available. If the same name appears in both, the project-level entry wins.

---

## Keeping secrets out of config

**Option 1 — Wrapper script**

Create a shell script that reads the password from a secrets manager or environment:

```bash
#!/bin/bash
# /usr/local/bin/vsp-dev
export SAP_PASSWORD="$(pass show sap/dev)"  # or: op read "op://vault/sap-dev/password"
exec /usr/local/bin/vsp "$@"
```

Then reference the script in `.mcp.json`:

```json
{
  "mcpServers": {
    "sap-dev": {
      "command": "/usr/local/bin/vsp-dev"
    }
  }
}
```

**Option 2 — `.env` file (not committed)**

Keep a `.env` in your working directory (gitignored):

```bash
SAP_URL=https://dev:44300
SAP_USER=DEVELOPER
SAP_PASSWORD=secret
```

vsp auto-loads `.env` from the directory where the MCP client launches it. Then in `.mcp.json` omit the password:

```json
{
  "mcpServers": {
    "sap-dev": {
      "command": "/usr/local/bin/vsp"
    }
  }
}
```

**Option 3 — Cookie authentication**

For SSO environments, export browser cookies and use a cookie file — no password needed:

```json
{
  "mcpServers": {
    "sap-prod": {
      "command": "/usr/local/bin/vsp",
      "env": {
        "SAP_URL": "https://prod:44300",
        "SAP_COOKIE_FILE": "/home/user/.sap/prod-cookies.txt"
      }
    }
  }
}
```

---

## Multiple AI clients

Multiple AI clients can connect to the same SAP system simultaneously — each client starts its own vsp process, and vsp is stateless between calls (sessions are maintained per-process).

### Claude Code + Claude Desktop simultaneously

Both can use the same vsp binary against the same SAP system without conflict:

```
Claude Code   →  vsp process A  →  SAP system
Claude Desktop →  vsp process B  →  SAP system
```

Each process manages its own CSRF token and session independently. The only contention point is **object locks** — if both processes try to edit the same object at the same time, the second lock attempt will fail. This is expected SAP behaviour and not specific to vsp.

### Per-client `.mcp.json` vs global config

| Scenario | Approach |
|----------|----------|
| All clients use the same systems | Global MCP config |
| Per-project system access | Project `.mcp.json` |
| Claude Code uses all systems, Desktop only prod | Use global for prod, project `.mcp.json` for others |

---

## CLI multi-system with `.vsp.json`

For CLI usage (not MCP), configure all systems in a single `.vsp.json` profile file:

```json
{
  "default": "dev",
  "systems": {
    "dev": {
      "url": "http://dev.example.com:50000",
      "user": "DEVELOPER",
      "client": "001"
    },
    "qa": {
      "url": "https://qa.example.com:44300",
      "user": "TESTER",
      "client": "001",
      "read_only": true
    },
    "prod": {
      "url": "https://prod.example.com:44300",
      "user": "MONITOR",
      "client": "100",
      "read_only": true,
      "cookie_file": "/home/user/.sap/prod-cookies.txt"
    }
  }
}
```

**Set passwords** via environment variables — never put them in the file:

```bash
export VSP_DEV_PASSWORD=devpass
export VSP_QA_PASSWORD=qapass
export VSP_PROD_PASSWORD=monpass
```

Pattern: `VSP_<SYSTEM_NAME_UPPERCASE>_PASSWORD`

**Use a specific system:**

```bash
vsp -s dev source CLAS ZCL_ORDER
vsp -s prod source CLAS ZCL_ORDER
vsp -s dev export '$ZPKG' -o backup.zip
```

**Config file locations** (searched in order):

1. `.vsp.json` — current directory
2. `.vsp/systems.json` — current directory
3. `~/.vsp.json` — home directory
4. `~/.vsp/systems.json` — home directory

**Import/export between MCP and CLI config:**

```bash
vsp config mcp-to-vsp   # import servers from .mcp.json → .vsp.json
vsp config vsp-to-mcp   # export systems from .vsp.json → .mcp.json
```
