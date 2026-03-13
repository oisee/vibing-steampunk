# Docker Guide for vsp

vsp ships as a single static-ish binary that speaks MCP over **stdio**. Docker is
useful when you want a reproducible, credential-isolated runtime without installing
Go locally, or when you want to lock a specific version for a team.

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [Pre-Built Images (GHCR)](#pre-built-images-ghcr)
3. [Building the Image](#building-the-image)
4. [How vsp Runs in Docker](#how-vsp-runs-in-docker)
5. [Configuration Reference](#configuration-reference)
   - [Connection](#connection)
   - [Authentication](#authentication)
   - [Tool Mode](#tool-mode)
   - [Disabling Tool Groups](#disabling-tool-groups)
   - [Safety / Read-Only](#safety--read-only)
   - [Transport Management](#transport-management)
   - [Feature Flags](#feature-flags)
   - [Debugger](#debugger)
   - [Network / TLS](#network--tls)
6. [MCP Client Integration](#mcp-client-integration)
   - [Claude Desktop](#claude-desktop)
   - [Gemini CLI / Other Agents](#gemini-cli--other-agents)
7. [Common Configurations](#common-configurations)
   - [Read-Only for Production Review](#read-only-for-production-review)
   - [Sandboxed AI (Z* packages only)](#sandboxed-ai-z-packages-only)
   - [Expert Mode with All Tools](#expert-mode-with-all-tools)
   - [Cookie Authentication](#cookie-authentication)
   - [Corporate Proxy](#corporate-proxy)
7. [Updating the Image](#updating-the-image)
8. [Security Notes](#security-notes)
9. [Troubleshooting](#troubleshooting)

---

## Quick Start

> **No Docker Hub needed.** Pre-built images are published automatically to
> [GitHub Container Registry (GHCR)](https://ghcr.io/oisee/vsp) on every
> release. Pull them with `docker pull ghcr.io/oisee/vsp:latest`.

```bash
# Minimal run (basic auth)
docker run -i --rm \
  -e SAP_URL=https://host:44300 \
  -e SAP_USER=developer \
  -e SAP_PASSWORD=secret \
  ghcr.io/oisee/vsp:latest

# Read-only mode — safe for production systems
docker run -i --rm \
  -e SAP_URL=https://prod:44300 \
  -e SAP_USER=viewer \
  -e SAP_PASSWORD=secret \
  -e SAP_READ_ONLY=true \
  ghcr.io/oisee/vsp:latest
```

> **`-i` is required.** MCP communicates over stdin/stdout. Without `-i` the
> container exits immediately because stdin is closed.

---

## Pre-Built Images (GHCR)

Official images are built automatically by GitHub Actions and pushed to
**GitHub Container Registry** — no Docker Hub account is required for either
pulling or publishing.

### Image location

```
ghcr.io/oisee/vsp
```

### Available tags

| Tag | Meaning |
|---|---|
| `latest` | Most recent stable release |
| `2.22.0` | Exact version |
| `2.22` | Latest patch of `2.22.x` |
| `2` | Latest minor/patch of major `2` |

Pre-release tags (e.g. `v2.22.0-rc1`) are published but do **not** move the
`latest` tag.

### Pulling

```bash
# Latest stable
docker pull ghcr.io/oisee/vsp:latest

# Pinned version (recommended for production/team use)
docker pull ghcr.io/oisee/vsp:2.22.0
```

### Supported platforms

Each image is a multi-platform manifest covering:

| Platform | Architecture |
|---|---|
| `linux/amd64` | x86-64 servers, most CI runners |
| `linux/arm64` | Apple Silicon (via Rosetta or native Linux VM), AWS Graviton |

Docker automatically selects the right variant for your host.

### GitHub Actions: automated publishing

The workflow `.github/workflows/docker.yml` triggers on every `v*` tag push
(the same event fired at the end of the `release.yml` workflow):

```
release.yml
  └─► git tag v2.22.0 + git push tag
            │
            └─► docker.yml triggered
                  ├── docker/setup-qemu-action     (arm64 emulation)
                  ├── docker/setup-buildx-action   (multi-platform builder)
                  ├── docker/login-action          (GITHUB_TOKEN → ghcr.io)
                  ├── docker/metadata-action       (semver tags + OCI labels)
                  └── docker/build-push-action     (build + push)
```

**No extra secrets are needed.** The workflow uses the built-in `GITHUB_TOKEN`
with `packages: write` permission. This token is automatically available in all
GitHub Actions runs.

```yaml
permissions:
  contents: read
  packages: write
```

### Manual re-publish (workflow_dispatch)

You can trigger a new Docker build for any existing tag without a full release:

1. Go to **Actions → Docker** in the GitHub UI.
2. Click **Run workflow**.
3. Optionally supply an image tag override.

This is useful for republishing after a `Dockerfile` fix without bumping the
Go version.

### Visibility

Packages on GHCR inherit the visibility of the repository. If the repository is
public, the images are public and can be pulled anonymously. If private,
authentication is required (`docker login ghcr.io`).

---

## Building the Image

### From source

```bash
# Simple build (version = dev)
docker build -t vsp .

# With version metadata (recommended for releases)
docker build \
  --build-arg VERSION=2.22.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t vsp:2.22.0 .
```

### Multi-platform build (for sharing)

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=2.22.0 \
  -t ghcr.io/yourorg/vsp:2.22.0 \
  --push .
```

> **CGO note:** The image uses `golang:1.23-alpine` as builder with `gcc` and
> `musl-dev` installed because `go-sqlite3` requires CGO. The runtime image is
> `alpine:3.21` with only `libgcc`, `libstdc++`, and `ca-certificates`. No Go
> toolchain is present in the final image.

---

## How vsp Runs in Docker

vsp is an **MCP stdio server** — it reads JSON-RPC from stdin and writes to
stdout. There are no exposed ports, no HTTP endpoints, no daemons.

An MCP client (Claude Desktop, Gemini CLI, etc.) launches `docker run -i` as a
subprocess and communicates through pipes. The container lifecycle is:

```
MCP Client
  │
  ├─► docker run -i --rm -e SAP_URL=... vsp   ← spawns container
  │         │
  │    JSON-RPC over stdio
  │         │
  └─────────┴─► container exits when client disconnects
```

Because of this model:
- **No `EXPOSE` directive is needed.**
- **No `-d` (detached) mode.** The container must be in the foreground.
- **`--rm` is recommended** to avoid stale containers accumulating.

---

## Configuration Reference

All configuration is done through **environment variables** with the `SAP_`
prefix. CLI flags map 1:1 to env vars:

| CLI flag | Env variable | Default |
|---|---|---|
| `--url` | `SAP_URL` | *(required)* |
| `--user` | `SAP_USER` | |
| `--password` | `SAP_PASSWORD` | |
| `--client` | `SAP_CLIENT` | `001` |
| `--language` | `SAP_LANGUAGE` | `EN` |
| `--insecure` | `SAP_INSECURE` | `false` |
| `--mode` | `SAP_MODE` | `focused` |
| `--disabled-groups` | `SAP_DISABLED_GROUPS` | |
| `--read-only` | `SAP_READ_ONLY` | `false` |
| `--block-free-sql` | `SAP_BLOCK_FREE_SQL` | `false` |
| `--allowed-ops` | `SAP_ALLOWED_OPS` | |
| `--disallowed-ops` | `SAP_DISALLOWED_OPS` | |
| `--allowed-packages` | `SAP_ALLOWED_PACKAGES` | |
| `--allow-transportable-edits` | `SAP_ALLOW_TRANSPORTABLE_EDITS` | `false` |
| `--enable-transports` | `SAP_ENABLE_TRANSPORTS` | `false` |
| `--transport-read-only` | `SAP_TRANSPORT_READ_ONLY` | `false` |
| `--allowed-transports` | `SAP_ALLOWED_TRANSPORTS` | |
| `--feature-abapgit` | `SAP_FEATURE_ABAPGIT` | `auto` |
| `--feature-rap` | `SAP_FEATURE_RAP` | `auto` |
| `--feature-amdp` | `SAP_FEATURE_AMDP` | `auto` |
| `--feature-ui5` | `SAP_FEATURE_UI5` | `auto` |
| `--feature-transport` | `SAP_FEATURE_TRANSPORT` | `auto` |
| `--feature-hana` | `SAP_FEATURE_HANA` | `auto` |
| `--terminal-id` | `SAP_TERMINAL_ID` | |
| `--verbose` | `SAP_VERBOSE` | `false` |

---

### Connection

```bash
SAP_URL=https://host:44300    # Required. Include scheme and port.
SAP_CLIENT=001                # SAP client number (3 digits)
SAP_LANGUAGE=EN               # ABAP session language (2-char ISO)
```

---

### Authentication

vsp supports three mutually exclusive auth methods. Use exactly one.

#### Basic auth (username + password)

```bash
-e SAP_USER=myuser -e SAP_PASSWORD=mypassword
```

#### Cookie file (Netscape format)

Mount a cookie file into the container and reference it:

```bash
docker run -i --rm \
  -e SAP_URL=https://host:44300 \
  -e SAP_COOKIE_FILE=/cookies/cookies.txt \
  -v /path/to/local/cookies.txt:/cookies/cookies.txt:ro \
  vsp
```

The cookie file must use the Netscape format exported by browser extensions like
*Edit This Cookie* or *Cookie-Editor*.

#### Cookie string

```bash
-e SAP_COOKIE_STRING="sap-usercontext=sap-client=001; SAP_SESSIONID_ABC=xyz"
```

> **Never bake credentials into the image** with `ENV` in a downstream
> Dockerfile. Always pass them at runtime via `-e` or `--env-file`.

#### Using `--env-file`

Keep credentials in a local `.env` file (never committed to git):

```bash
# .env
SAP_URL=https://host:44300
SAP_USER=developer
SAP_PASSWORD=s3cr3t
```

```bash
docker run -i --rm --env-file .env vsp
```

---

### Tool Mode

vsp has two modes that control which tools are registered with the MCP client:

| Mode | Tools | Use case |
|---|---|---|
| `focused` (default) | 81 tools | Daily development — essential tools only, cleaner context |
| `expert` | 122 tools | Full access including AMDP debugger, experimental tools |

```bash
-e SAP_MODE=expert
```

---

### Disabling Tool Groups

Within a mode you can further reduce noise by disabling entire groups:

| Code | Group | Tools disabled |
|---|---|---|
| `5` or `U` | UI5/Fiori BSP | All BSP/UI5 management tools |
| `T` | Tests | Unit test runner tools |
| `H` | HANA/AMDP | HANA SQL, AMDP debugger |
| `D` | Debugger | External ABAP debugger |
| `C` | Code Intelligence | Find references, code completion |

The value is a string of codes. For example, to disable Tests and HANA:

```bash
-e SAP_DISABLED_GROUPS=TH
```

To disable everything except core read/search/edit:

```bash
-e SAP_DISABLED_GROUPS=THDUC
```

---

### Safety / Read-Only

These flags protect SAP systems from unintended AI-driven modifications.

#### Full read-only (safest)

Blocks **all** write operations — create, update, delete, activate, execute:

```bash
-e SAP_READ_ONLY=true
```

Use this when pointing an AI at a production or QA system where you only want
read/search access.

#### Block free SQL

Prevents the AI from running arbitrary SELECT statements via `RunQuery`:

```bash
-e SAP_BLOCK_FREE_SQL=true
```

#### Operation allowlist

Only permit specific operation types. The operation codes are:

| Code | Meaning |
|---|---|
| `R` | Read (GetSource, GetObject, etc.) |
| `S` | Search (SearchObjects, GrepSource, etc.) |
| `Q` | Query (RunQuery — free SQL) |
| `C` | Create |
| `D` | Delete |
| `U` | Update (EditSource, WriteSource) |
| `A` | Activate |

Example — allow only read and search (equivalent to read-only but explicit):

```bash
-e SAP_ALLOWED_OPS=RS
```

Example — allow read, search, and query but no writes:

```bash
-e SAP_ALLOWED_OPS=RSQ
```

#### Operation blocklist

Block specific operation types while allowing everything else:

```bash
# Block create, delete, update, activate (but allow read/search/query)
-e SAP_DISALLOWED_OPS=CDUA
```

> `SAP_ALLOWED_OPS` and `SAP_DISALLOWED_OPS` are mutually exclusive. Use one or
> the other, not both.

#### Package restriction

Restrict all write operations to specific packages. Supports wildcards:

```bash
# Only allow writes to custom packages and $TMP
-e SAP_ALLOWED_PACKAGES="Z*,$TMP"

# Only one specific package
-e SAP_ALLOWED_PACKAGES="\$ZRAY_DEV"

# Multiple packages (comma-separated)
-e SAP_ALLOWED_PACKAGES="ZMYAPP,ZMYAPP_TEST,\$TMP"
```

Read operations (`R`, `S`) are not restricted by this filter — the AI can still
read objects from any package; it just cannot modify objects outside the
allowlist.

#### Transportable edits

By default, vsp only allows editing objects in local (non-transportable) packages
like `$TMP`. To allow edits to objects that require a transport request:

```bash
-e SAP_ALLOW_TRANSPORTABLE_EDITS=true
```

When enabled, write tools accept an optional `transport` parameter. Without a
transport number, the edit will still fail for transportable objects.

---

### Transport Management

CTS transport tools are **disabled by default** to prevent accidental releases.

```bash
# Enable transport tools
-e SAP_ENABLE_TRANSPORTS=true

# Enable but restrict to read-only (list, view — no create/release/delete)
-e SAP_ENABLE_TRANSPORTS=true
-e SAP_TRANSPORT_READ_ONLY=true

# Enable and restrict to specific transports (wildcards OK)
-e SAP_ENABLE_TRANSPORTS=true
-e SAP_ALLOWED_TRANSPORTS="A4HK*"
```

---

### Feature Flags

Feature flags control whether optional SAP features are enabled. The default
`auto` mode probes the SAP system on startup and enables a feature only if the
corresponding ABAP components are installed.

Use `on` or `off` to skip the probe and force the behavior:

| Variable | Feature | Notes |
|---|---|---|
| `SAP_FEATURE_ABAPGIT` | abapGit integration | Requires ZADT_VSP installed |
| `SAP_FEATURE_RAP` | RAP/OData development | BDEF, SRVD, SRVB tools |
| `SAP_FEATURE_AMDP` | AMDP/HANA debugger | Expert mode only |
| `SAP_FEATURE_UI5` | UI5/Fiori BSP | BSP app management |
| `SAP_FEATURE_TRANSPORT` | CTS transport | Superset of `--enable-transports` |
| `SAP_FEATURE_HANA` | HANA detection | AMDP, HANA SQL tools |

```bash
# Force all features off — fastest startup, smallest tool surface
-e SAP_FEATURE_ABAPGIT=off \
-e SAP_FEATURE_RAP=off \
-e SAP_FEATURE_AMDP=off \
-e SAP_FEATURE_UI5=off \
-e SAP_FEATURE_TRANSPORT=off \
-e SAP_FEATURE_HANA=off

# Force abapGit on without probing
-e SAP_FEATURE_ABAPGIT=on
```

Setting features to `off` reduces the number of registered MCP tools, which
keeps the AI's tool list shorter and lowers token usage.

---

### Debugger

To share breakpoints with SAP GUI (cross-tool debugging), configure the same
terminal ID that SAP GUI uses:

```bash
-e SAP_TERMINAL_ID=D0C586D015974B75BFB2A306A4A13AEA
```

SAP GUI stores its terminal ID at:
- **Windows:** Registry `HKCU\Software\SAP\ABAP Debugging\TerminalID`
- **Linux/Mac:** `~/.SAP/ABAPDebugging/terminalId`

---

### Network / TLS

#### Self-signed or internal CA certificates

For SAP systems using self-signed certificates, either skip verification
(development only) or add your CA to the image:

```bash
# Option 1: skip verification (NOT for production)
-e SAP_INSECURE=true

# Option 2: mount your CA certificate
docker run -i --rm \
  -e SAP_URL=https://internal-sap:44300 \
  -e SAP_USER=user -e SAP_PASSWORD=pass \
  -v /etc/ssl/certs/company-ca.crt:/usr/local/share/ca-certificates/company-ca.crt:ro \
  vsp
```

For a permanent fix, extend the image:

```dockerfile
FROM ghcr.io/oisee/vsp:latest
USER root
COPY company-ca.crt /usr/local/share/ca-certificates/
RUN update-ca-certificates
USER vsp
```

#### HTTP/HTTPS proxy

vsp respects standard Go proxy environment variables. Pass them via `-e`:

```bash
-e HTTPS_PROXY=http://proxy.corp.example:3128
-e NO_PROXY=localhost,127.0.0.1,internal-sap.corp
```

#### Connecting to a SAP system on the same Docker host

Use the host network or `host.docker.internal` (Docker Desktop):

```bash
# Linux — host network mode
docker run -i --rm --network host \
  -e SAP_URL=http://localhost:50000 \
  -e SAP_USER=user -e SAP_PASSWORD=pass \
  vsp

# Docker Desktop (Mac/Windows)
-e SAP_URL=http://host.docker.internal:50000
```

#### Connecting to a SAP system in another Docker network

```bash
docker network create sap-net

docker run -i --rm \
  --network sap-net \
  -e SAP_URL=http://sap-container:50000 \
  -e SAP_USER=user -e SAP_PASSWORD=pass \
  vsp
```

---

## MCP Client Integration

### Claude Desktop

Edit `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS)
or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "vsp": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-e", "SAP_URL=https://my-sap-system:44300",
        "-e", "SAP_USER=developer",
        "-e", "SAP_PASSWORD=secret",
        "ghcr.io/oisee/vsp:latest"
      ]
    }
  }
}
```

For a production system where you want read-only access:

```json
{
  "mcpServers": {
    "vsp-prod": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-e", "SAP_URL=https://prod-sap:44300",
        "-e", "SAP_USER=readonly_user",
        "-e", "SAP_PASSWORD=secret",
        "-e", "SAP_READ_ONLY=true",
        "-e", "SAP_MODE=focused",
        "ghcr.io/oisee/vsp:latest"
      ]
    }
  }
}
```

> **Tip:** Use `--env-file` instead of individual `-e` flags to keep credentials
> out of the config file. Reference the absolute path to the env file:
>
> ```json
> "args": ["run", "-i", "--rm", "--env-file", "/Users/me/.vsp-prod.env", "ghcr.io/oisee/vsp:latest"]
> ```

### Gemini CLI / Other Agents

Most MCP clients follow the same stdio pattern. Use the same `docker run -i`
command. See `docs/cli-agents/README.md` for agent-specific configuration
examples.

---

## Common Configurations

### Read-Only for Production Review

Safe for handing to an AI agent with access to a production system:

```bash
docker run -i --rm \
  -e SAP_URL=https://prod:44300 \
  -e SAP_USER=s_ai_viewer \
  -e SAP_PASSWORD=secret \
  -e SAP_READ_ONLY=true \
  -e SAP_BLOCK_FREE_SQL=true \
  -e SAP_MODE=focused \
  -e SAP_FEATURE_TRANSPORT=off \
  -e SAP_VERBOSE=true \
  ghcr.io/oisee/vsp:latest
```

### Sandboxed AI (Z* packages only)

AI can read anything but only write to custom development packages:

```bash
docker run -i --rm \
  -e SAP_URL=https://dev:44300 \
  -e SAP_USER=developer \
  -e SAP_PASSWORD=secret \
  -e SAP_ALLOWED_PACKAGES="Z*,\$TMP" \
  -e SAP_BLOCK_FREE_SQL=true \
  -e SAP_DISALLOWED_OPS=D \
  ghcr.io/oisee/vsp:latest
```

This setup lets the AI read system objects, write only to custom packages, and
prevents deletions and arbitrary SQL.

### Expert Mode with All Tools

For advanced debugging and full feature access:

```bash
docker run -i --rm \
  -e SAP_URL=https://dev:44300 \
  -e SAP_USER=developer \
  -e SAP_PASSWORD=secret \
  -e SAP_MODE=expert \
  -e SAP_ENABLE_TRANSPORTS=true \
  -e SAP_ALLOW_TRANSPORTABLE_EDITS=true \
  -e SAP_FEATURE_ABAPGIT=on \
  -e SAP_FEATURE_RAP=on \
  -e SAP_FEATURE_AMDP=on \
  ghcr.io/oisee/vsp:latest
```

### Cookie Authentication

```bash
docker run -i --rm \
  -e SAP_URL=https://host:44300 \
  -e SAP_COOKIE_FILE=/cookies/cookies.txt \
  -v "${HOME}/.sap-cookies/my-system.txt:/cookies/cookies.txt:ro" \
  ghcr.io/oisee/vsp:latest
```

### Corporate Proxy

```bash
docker run -i --rm \
  --env-file .env \
  -e HTTPS_PROXY=http://proxy.corp.example:3128 \
  -e NO_PROXY=localhost,127.0.0.1 \
  ghcr.io/oisee/vsp:latest
```

---

## Updating the Image

### Pulling a newer pre-built image (GHCR)

```bash
# Pull latest
docker pull ghcr.io/oisee/vsp:latest

# Pull a specific version (recommended)
docker pull ghcr.io/oisee/vsp:2.22.0
```

### Rebuilding from source

```bash
git pull
docker build \
  --build-arg VERSION=$(git describe --tags --abbrev=0) \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t vsp:latest .
```

### Pinning a version (recommended)

For production or shared team environments, always pin to a specific version
tag rather than `latest`:

```json
"ghcr.io/oisee/vsp:2.22.0"
```

This ensures every team member and every CI run uses the same binary regardless
of when the image was pulled. Check the
[GitHub releases page](https://github.com/oisee/vibing-steampunk/releases)
for the latest version.

### Verifying the version

```bash
docker run --rm ghcr.io/oisee/vsp:latest --version
```

### Updating after a Dockerfile fix (no Go change)

If only the `Dockerfile` changed (e.g. base image bump, added CA cert), trigger
the Docker workflow manually via **Actions → Docker → Run workflow** on the
desired tag. No new Go release is needed.

### Staying up to date automatically

Teams that want automatic image updates can use tools like
[Renovate](https://docs.renovatebot.com/) or
[Dependabot](https://docs.github.com/en/code-security/dependabot) to open PRs
when a new `ghcr.io/oisee/vsp` image tag is published.

---

## Security Notes

1. **Never bake credentials into images.** Always pass `SAP_USER`,
   `SAP_PASSWORD`, `SAP_COOKIE_FILE` at runtime via `-e` or `--env-file`.

2. **Protect your `.env` file.** If using `--env-file`, ensure the file has
   restricted permissions (`chmod 600`) and is in `.gitignore`.

3. **Use `SAP_READ_ONLY=true` for production.** Only enable write access on
   development systems.

4. **The container runs as a non-root user** (`vsp:vsp`) inside Alpine. There
   are no open ports — the attack surface is minimal.

5. **Cookie files contain session tokens.** Mount them read-only (`:ro`) and
   use short-lived sessions where possible.

6. **Use `SAP_INSECURE=false` (the default).** Only set it to `true` in isolated
   development environments with no sensitive data.

---

## Troubleshooting

### Container exits immediately

vsp exits if stdin is closed. Make sure you are using `-i`:

```bash
docker run -i --rm ...   # correct
docker run --rm ...      # wrong — exits immediately
docker run -d --rm ...   # wrong — detached mode breaks stdio
```

### `SAP URL is required` error

The `SAP_URL` environment variable is mandatory. Verify it is being passed:

```bash
docker run -i --rm -e SAP_URL=https://host:44300 ... vsp
```

### TLS certificate errors

```
x509: certificate signed by unknown authority
```

Either add your CA certificate (see [Network / TLS](#network--tls)) or use
`SAP_INSECURE=true` in non-production environments.

### `authentication required` error

Only one auth method is accepted. Check you are not accidentally setting both
`SAP_USER/SAP_PASSWORD` and `SAP_COOKIE_FILE`.

### Enable verbose logging

Add `-e SAP_VERBOSE=true` to see startup decisions including which features were
detected and which safety rules are active. Logs go to stderr; they will not
interfere with MCP over stdout.

```bash
docker run -i --rm \
  -e SAP_URL=https://host:44300 \
  -e SAP_USER=user -e SAP_PASSWORD=pass \
  -e SAP_VERBOSE=true \
  vsp 2>vsp-debug.log
```

### Tool not appearing in the AI client

1. Check `SAP_MODE` — some tools are only in `expert` mode.
2. Check `SAP_DISABLED_GROUPS` — the tool's group may be disabled.
3. Check feature flags — features in `auto` mode may have been turned off because
   the SAP component was not detected. Force them on with e.g.
   `SAP_FEATURE_RAP=on`.
4. Check `SAP_ALLOWED_OPS` / `SAP_DISALLOWED_OPS` — operation filters can block
   tools at the handler level even if they are registered.
