# OpenAI Codex CLI + VSP: SAP Testing Agent

Guide for configuring [OpenAI Codex CLI](https://github.com/openai/codex) as an SAP Q/A testing agent using [VSP](https://github.com/oisee/vibing-steampunk).

---

## Overview

Codex CLI can use VSP as an MCP server to interact with SAP systems. This guide focuses on a **read-only testing** configuration -- ideal for Q/A, code review, Clean Core audits, and exploratory analysis without risk of modifying the SAP system.

---

## 1. Install

### Codex CLI

```bash
npm install -g @openai/codex
```

### VSP Binary

```bash
# Download prebuilt binary (pick your platform)
curl -LO https://github.com/oisee/vibing-steampunk/releases/latest/download/vsp-linux-amd64
chmod +x vsp-linux-amd64

# Or build from source
git clone https://github.com/oisee/vibing-steampunk.git
cd vibing-steampunk && go build -o vsp ./cmd/vsp
```

---

## 2. MCP Configuration

Codex supports **two config formats**. Use whichever your Codex version expects.

### Option A: TOML format (`codex.toml`) — Codex native

Create `codex.toml` in the project root:

```toml
# Read-Only (recommended for Q/A)
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

```toml
# Restricted Write (Search + Read + Query)
[mcp_servers.sap-adt]
command = "/path/to/vsp"
enabled = true

[mcp_servers.sap-adt.env]
SAP_URL = "https://your-sap-host:44300"
SAP_USER = "YOUR_USER"
SAP_PASSWORD = "<password>"
SAP_CLIENT = "001"
SAP_ALLOWED_OPS = "RSQ"
SAP_MODE = "focused"
```

```toml
# Windows example
[mcp_servers.sap-adt]
command = "C:\\SOFT\\vsp.exe"
enabled = true

[mcp_servers.sap-adt.env]
SAP_URL = "http://127.0.0.1:50000"
SAP_USER = "DEVELOPER"
SAP_PASSWORD = "<password>"
```

> **`SAP_ALLOWED_OPS` codes:** `R` = Read, `S` = Search, `Q` = Query (SQL), `C` = Create, `D` = Delete, `U` = Update, `A` = Activate, `T` = Transport

> **Security:** `codex.toml` contains credentials — it is in `.gitignore` by default. Never commit it.

### Option B: JSON format (`.mcp.json`) — also works

```json
{
  "mcpServers": {
    "sap-adt": {
      "command": "/path/to/vsp",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<password>",
        "SAP_CLIENT": "001",
        "SAP_READ_ONLY": "true",
        "SAP_MODE": "focused"
      }
    }
  }
}
```

### Cookie Authentication (Alternative)

For SSO or cookie-based auth (both formats):

```toml
# TOML
[mcp_servers.sap-adt]
command = "/path/to/vsp"
args = ["--cookie-file", "/path/to/cookies.txt"]
enabled = true

[mcp_servers.sap-adt.env]
SAP_URL = "https://your-sap-host:44300"
SAP_READ_ONLY = "true"
SAP_MODE = "focused"
```

---

## 3. Codex Instructions File

Create `codex.md` (or add to your existing instructions) in the project root so Codex understands how to use the SAP tools:

```markdown
# Codex Instructions: SAP Testing via VSP

You have access to an MCP server called `sap-adt` that provides tools
for interacting with an SAP ABAP system via ADT (ABAP Development Tools).

## Available Tool Categories

- **Search:** SearchObject, GrepSource, GrepPackages
- **Read:** GetSource, GetObjectStructure, GetClassSource
- **Query:** RunQuery (SQL via ABAP CDS / OpenSQL)
- **Test:** RunUnitTests, RunATCCheck, SyntaxCheck
- **Analyze:** GetCallGraph, GetAPIReleaseState, GetContext
- **System:** GetSystemInfo, GetInstalledComponents, GetFeatures

## Safety

This server runs in read-only mode. You cannot create, modify, or
delete any SAP objects. Use it for investigation, testing, and analysis.

## Conventions

- ABAP object names are uppercase: `ZCL_MY_CLASS`, `ZTEST_PROGRAM`
- Package names start with `$` for local or `Z`/`Y` for custom
- Standard SAP objects do not have Z/Y prefix
- Use `RunQuery` for SQL: `SELECT * FROM mara UP TO 10 ROWS`
```

---

## 4. Example Test Scenarios

These prompts demonstrate what Codex can do with VSP in Q/A testing mode.

### 4.1 Search for Objects

```
Find all custom classes in the system whose names start with ZCL_TRAVEL.
Show their package assignment and description.
```

```
Search for function modules matching *BAPI_MATERIAL* and list them.
```

```
Find all programs in package $TMP.
```

### 4.2 Query Tables

```
Run this query and show the results:
SELECT matnr, maktx FROM makt WHERE spras = 'E' AND matnr LIKE 'FG%' UP TO 20 ROWS
```

```
How many entries are in table USR02? Show the count by user type (ustyp).
SELECT ustyp, COUNT(*) AS cnt FROM usr02 GROUP BY ustyp
```

```
Show the 10 most recently changed custom programs:
SELECT obj_name, udat, utime FROM tadir WHERE pgmid = 'R3TR' AND object = 'PROG' AND devclass LIKE 'Z%' ORDER BY udat DESCENDING, utime DESCENDING UP TO 10 ROWS
```

### 4.3 Check Source Code

```
Get the source code of class ZCL_TRAVEL_SERVICE and explain its public methods.
```

```
Read the source of program ZTEST_MCP_CRUD and check if it follows
ABAP naming conventions.
```

```
Get the source of function module BAPI_MATERIAL_GET_DETAIL and
summarize its parameters and logic.
```

### 4.4 Run Unit Tests

```
Run unit tests for class ZCL_TRAVEL_SERVICE and report which tests
pass and which fail, with failure details.
```

```
Run unit tests for all classes in package $ZADT and give me a summary
of the overall test health.
```

```
Run ATC (ABAP Test Cockpit) checks on class ZCL_MY_CLASS and report
any findings by priority.
```

### 4.5 Check API Release States (Clean Core)

```
Check the API release state of class CL_ABAP_TYPEDESCR.
Is it released for Cloud development (tier 1)?
```

```
Analyze class ZCL_TRAVEL_SERVICE for Clean Core compliance:
1. Get its source code
2. Identify all referenced SAP standard objects (classes, function modules, tables)
3. Check the API release state of each referenced object
4. Report which references are released, deprecated, or not released
```

```
Check whether function module POPUP_TO_CONFIRM is released for
use in ABAP Cloud (S/4HANA Cloud tier 1).
```

### 4.6 System Analysis

```
Get the system info and installed components. What SAP_BASIS
release is this system running?
```

```
Show the call graph for class ZCL_TRAVEL_SERVICE. What are its
upstream and downstream dependencies?
```

```
Check what features are available on this system (abapGit, RAP,
AMDP, transport management).
```

---

## 5. Safety Recommendations

| Scenario | Configuration |
|----------|--------------|
| **Q/A / Code Review** | `SAP_READ_ONLY=true` -- blocks all write operations |
| **SQL Analysis** | `SAP_ALLOWED_OPS=RSQ` -- read + search + query only |
| **Development (restricted)** | `SAP_ALLOWED_PACKAGES=Z*,$TMP` -- restrict to custom packages |
| **Full testing** | `SAP_BLOCK_FREE_SQL=true` -- allow writes but block arbitrary SQL |
| **Production system** | `SAP_READ_ONLY=true` always -- never allow writes to production |

### Additional Safety Flags

```toml
# Add to [mcp_servers.sap-adt.env] section
SAP_READ_ONLY = "true"
SAP_BLOCK_FREE_SQL = "true"
SAP_ALLOWED_PACKAGES = "Z*,$TMP"
SAP_ALLOW_TRANSPORTABLE_EDITS = "false"
```

---

## 6. Modes

VSP exposes different tool sets depending on the mode:

| Mode | Tools | Use Case |
|------|-------|----------|
| `focused` | 100 | Default. Core ABAP development tools. Best for most testing. |
| `expert` | 147 | All tools including AMDP debugger, UI5, gCTS, i18n, advanced features. |

Set via `SAP_MODE` environment variable in the MCP config.

### Disable Tool Groups

To further reduce the tool surface (faster startup, less noise):

```toml
# Add to [mcp_servers.sap-adt.env]
SAP_DISABLED_GROUPS = "5THD"
```

| Code | Group | Description |
|------|-------|-------------|
| `5` or `U` | UI5 | UI5/Fiori BSP management |
| `T` | Tests | Unit test and ATC tools |
| `H` | HANA | HANA/AMDP tools |
| `D` | Debug | Debugger tools |
| `C` | CDS | CDS-specific tools |

---

## 7. Troubleshooting

### Verify MCP Connectivity

Start Codex and ask:

```
List all available tools from the sap-adt MCP server.
```

If no tools appear, check:

1. The path to the `vsp` binary is correct and executable
2. SAP credentials are valid
3. The SAP system is reachable from your machine
4. Try running `vsp` directly: `/path/to/vsp --url http://host:port --user USER --password PASS`

### Enable Verbose Logging

Add `"SAP_VERBOSE": "true"` to the env block. VSP will log detailed request/response info to stderr.

### Test Without SAP

Use the echo MCP server to verify Codex MCP integration works:

```json
{
  "mcpServers": {
    "echo": {
      "command": "python3",
      "args": ["/path/to/docs/cli-agents/mcp-echo-server.py"]
    }
  }
}
```

---

## Links

- VSP Repository: https://github.com/oisee/vibing-steampunk
- Codex CLI: https://github.com/openai/codex
- MCP Specification: https://modelcontextprotocol.io
- CLI Agents Overview: [README.md](README.md)
