# Configuration

vsp supports three configuration sources with this priority order:

```
CLI flags  >  Environment variables  >  .env file  >  Defaults
```

---

## Connection

| CLI Flag | Env Variable | Default | Description |
|----------|--------------|---------|-------------|
| `--url` | `SAP_URL` | — | SAP system URL (e.g. `https://host:44300`) |
| `--user` | `SAP_USER` | — | SAP username |
| `--password` | `SAP_PASSWORD` | — | SAP password |
| `--client` | `SAP_CLIENT` | `001` | SAP client number |
| `--language` | `SAP_LANGUAGE` | `EN` | Logon language |
| `--insecure` | `SAP_INSECURE` | `false` | Skip TLS certificate verification |

---

## Authentication

### Basic (username + password)

```bash
SAP_URL=https://host:44300
SAP_USER=developer
SAP_PASSWORD=secret
```

### Cookie (SSO / no password)

```bash
# Netscape-format cookie file (exportable from browsers via extensions)
SAP_COOKIE_FILE=/path/to/cookies.txt

# Or inline cookie string
SAP_COOKIE_STRING="sap-usercontext=sap-client=001; SAP_SESSIONID_A4H_100=abc"
```

| CLI Flag | Env Variable | Description |
|----------|--------------|-------------|
| `--cookie-file` | `SAP_COOKIE_FILE` | Path to Netscape-format cookie file |
| `--cookie-string` | `SAP_COOKIE_STRING` | Inline cookie string (`key=val; key2=val2`) |

!!! note
    Use either basic auth **or** cookie auth — not both. If `SAP_COOKIE_FILE` or `SAP_COOKIE_STRING` is set, basic auth credentials are ignored.

---

## Mode & Tool Visibility

| CLI Flag | Env Variable | Default | Description |
|----------|--------------|---------|-------------|
| `--mode` | `SAP_MODE` | `focused` | `focused` (81 tools) or `expert` (122 tools) |
| `--disabled-groups` | `SAP_DISABLED_GROUPS` | — | Disable tool groups (see below) |

### Tool Groups

Disable specific tool groups to reduce token usage or restrict access:

| Code | Group | Tools |
|------|-------|-------|
| `5` or `U` | UI5/BSP management | `UI5ListApps`, `UI5GetApp`, etc. |
| `T` | Unit test tools | `RunUnitTests`, `CreateTestInclude`, etc. |
| `H` | HANA/AMDP debugger | `AMDPDebuggerStart`, etc. |
| `D` | ABAP debugger | `DebuggerListen`, `DebuggerAttach`, etc. |
| `C` | CTS/Transport | `ListTransports`, `CreateTransport`, etc. |
| `G` | Git/abapGit | `GitTypes`, `GitExport` |
| `R` | Report execution | `RunReport`, `GetVariants`, etc. |
| `I` | Install/setup | `InstallZADTVSP`, `InstallAbapGit`, etc. |
| `X` | Experimental | Miscellaneous experimental tools |

```bash
# Disable UI5 and Transport tools
vsp --disabled-groups UC
```

### Per-system tool visibility (`.vsp.json`)

Fine-grained control in CLI mode system profiles:

```json
{
  "systems": {
    "prod": {
      "url": "...",
      "disabled_groups": "CGHT",
      "read_only": true
    }
  }
}
```

---

## Safety Controls

See [Safety Controls](features/safety.md) for full documentation.

| CLI Flag | Env Variable | Default | Description |
|----------|--------------|---------|-------------|
| `--read-only` | `SAP_READ_ONLY` | `false` | Block all write operations |
| `--block-free-sql` | `SAP_BLOCK_FREE_SQL` | `false` | Block `RunQuery` execution |
| `--allowed-ops` | `SAP_ALLOWED_OPS` | — | Whitelist operation types (e.g. `RSQ`) |
| `--disallowed-ops` | `SAP_DISALLOWED_OPS` | — | Blacklist operation types (e.g. `CDUA`) |
| `--allowed-packages` | `SAP_ALLOWED_PACKAGES` | — | Restrict to packages (wildcards: `Z*,$TMP`) |
| `--allow-transportable-edits` | `SAP_ALLOW_TRANSPORTABLE_EDITS` | `false` | Allow editing objects in transportable packages |

---

## Transport Management

| CLI Flag | Env Variable | Default | Description |
|----------|--------------|---------|-------------|
| `--enable-transports` | `SAP_ENABLE_TRANSPORTS` | `false` | Enable transport management tools |
| `--transport-read-only` | `SAP_TRANSPORT_READ_ONLY` | `false` | Read-only transport access |
| `--allowed-transports` | `SAP_ALLOWED_TRANSPORTS` | — | Whitelist transport patterns (e.g. `DEVK*`) |

---

## Feature Detection (Safety Network)

These flags control automatic feature detection. `auto` (default) probes the SAP system; `on`/`off` force-enable or disable.

| CLI Flag | Env Variable | Default | Description |
|----------|--------------|---------|-------------|
| `--feature-abapgit` | `SAP_FEATURE_ABAPGIT` | `auto` | abapGit integration |
| `--feature-rap` | `SAP_FEATURE_RAP` | `auto` | RAP/OData development |
| `--feature-amdp` | `SAP_FEATURE_AMDP` | `auto` | AMDP/HANA debugger |
| `--feature-ui5` | `SAP_FEATURE_UI5` | `auto` | UI5/Fiori BSP management |
| `--feature-transport` | `SAP_FEATURE_TRANSPORT` | `auto` | CTS transport management |

---

## Debugger

| CLI Flag | Env Variable | Default | Description |
|----------|--------------|---------|-------------|
| `--terminal-id` | `SAP_TERMINAL_ID` | — | SAP GUI terminal ID for cross-tool breakpoint sharing |

See [ABAP Debugger](features/debugging.md) for how to find and use your terminal ID.

---

## Logging

| CLI Flag | Env Variable | Default | Description |
|----------|--------------|---------|-------------|
| `--verbose` | `SAP_VERBOSE` | `false` | Enable verbose logging to stderr |

---

## .env File

vsp auto-loads `.env` from the current directory when running as an MCP server:

```bash
# .env
SAP_URL=https://host:44300
SAP_USER=developer
SAP_PASSWORD=secret
SAP_CLIENT=001
SAP_MODE=focused
```

---

## .vsp.json (Multi-system CLI profiles)

For CLI mode with multiple SAP systems:

```json
{
  "default": "dev",
  "systems": {
    "dev": {
      "url": "http://dev.example.com:50000",
      "user": "DEVELOPER",
      "client": "001"
    },
    "a4h": {
      "url": "http://a4h.local:50000",
      "user": "ADMIN",
      "client": "001",
      "insecure": true
    },
    "prod": {
      "url": "https://prod.example.com:44300",
      "user": "READONLY",
      "client": "100",
      "read_only": true,
      "cookie_file": "/path/to/cookies.txt"
    }
  }
}
```

**Password resolution:** Set `VSP_<SYSTEM>_PASSWORD` (e.g. `VSP_DEV_PASSWORD=secret`).

**Config locations** (searched in order):
1. `.vsp.json` (current directory)
2. `.vsp/systems.json`
3. `~/.vsp.json`
4. `~/.vsp/systems.json`

**Config management commands:**
```bash
vsp config init          # Create example .vsp.json
vsp config show          # Print effective config
vsp config mcp-to-vsp    # Import from .mcp.json
vsp config vsp-to-mcp    # Export to .mcp.json
```

---

## Proxy

vsp honors standard HTTP proxy environment variables:

```bash
export HTTP_PROXY=http://proxy.example.com:3128
export HTTPS_PROXY=http://proxy.example.com:3128
export NO_PROXY=localhost,127.0.0.1
```
