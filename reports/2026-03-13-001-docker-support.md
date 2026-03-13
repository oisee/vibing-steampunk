# Docker Support & GHCR Publishing

**Date:** 2026-03-13
**Report ID:** 001
**Subject:** Dockerfile, .dockerignore, GHCR publishing workflow, and documentation
**Related Documents:** `docs/docker.md`, `Dockerfile`, `.dockerignore`,
`.github/workflows/docker.yml`

---

## Summary

Added first-class Docker support: a multi-stage `Dockerfile`, a `.dockerignore`,
an automated GitHub Actions workflow that publishes to **GitHub Container Registry
(GHCR)** on every release, and comprehensive documentation at `docs/docker.md`.
No Docker Hub account is required.

---

## Files Added / Changed

| File | Change |
|---|---|
| `Dockerfile` | New — multi-stage build, Alpine runtime |
| `.dockerignore` | New — excludes credentials, reports, IDE/git files |
| `.github/workflows/docker.yml` | New — builds and pushes to `ghcr.io/oisee/vsp` |
| `docs/docker.md` | New — full configuration and usage guide |

---

## `release.yml` — No Changes Needed

`docker.yml` triggers on `push: tags: v*`. That event fires automatically when
`release.yml` pushes the tag in its "Tag and push" step. Both workflows run in
parallel, independently, with their own `permissions` blocks. No modification to
`release.yml` is required.

```
release.yml (existing, unchanged)
  └─► git push origin v2.22.0
            │
            ├─► GoReleaser job continues in release.yml
            │     └─► 9 native binaries, GitHub release
            │
            └─► docker.yml triggered (separate workflow)
                  └─► linux/amd64 + linux/arm64 images → ghcr.io/oisee/vsp
```

---

## Dockerfile Design

### Build stages

```
golang:1.23-alpine  (builder)
  ├── apk: gcc, musl-dev          ← CGO required by go-sqlite3
  ├── go mod download             ← cached layer
  ├── COPY source
  └── go build -ldflags           ← VERSION / COMMIT / BUILD_DATE via --build-arg

alpine:3.21  (runtime)
  ├── apk: ca-certificates, libgcc, libstdc++
  ├── adduser vsp  (non-root)
  └── COPY /vsp from builder
```

### CGO note

GoReleaser builds native binaries with `CGO_ENABLED=0` for cross-platform
portability. The Docker image uses `CGO_ENABLED=1` because `go-sqlite3` (cache
package) requires it and Alpine's `musl-libc` is consistent inside the container.
Both are intentional — different targets, different constraints.

### Non-root user

A dedicated `vsp:vsp` user and group are created. `USER vsp` is set before
`ENTRYPOINT`. The binary runs without root privileges.

### No exposed ports

vsp is an MCP stdio server (JSON-RPC over stdin/stdout). There are no network
listeners and no `EXPOSE` directive. MCP clients invoke `docker run -i` as a
subprocess; `-i` is required to keep stdin open.

### Version injection

Three `--build-arg` values are wired into the binary via `-ldflags`:

```bash
docker build \
  --build-arg VERSION=2.22.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t vsp:2.22.0 .
```

---

## `.dockerignore`

Excludes from build context:

- **Credentials:** `.env*`, `cookies.txt`, `.mcp.json`, `.vsp*.json`
- **Build artifacts:** `/build/`, `*.exe`, `*.so`, `*.test`
- **Large non-source dirs:** `reports/`, `articles/`
- **VCS / IDE:** `.git/`, `.idea/`, `.vscode/`
- **Go workspace:** `go.work`, `go.work.sum`

---

## GitHub Actions Workflow (`.github/workflows/docker.yml`)

### Triggers

```yaml
on:
  push:
    tags: ["v*"]      # automatic — fires when release.yml pushes the tag
  workflow_dispatch:   # manual re-publish without a full release
```

### Permissions

```yaml
permissions:
  contents: read
  packages: write
```

Only `packages: write` is needed. The built-in `GITHUB_TOKEN` authenticates to
`ghcr.io` — no repository secrets, no Docker Hub account.

### Action chain

| Step | Action | Purpose |
|---|---|---|
| 1 | `actions/checkout@v4` | Checkout source |
| 2 | `docker/setup-qemu-action@v3` | arm64 emulation on amd64 runner |
| 3 | `docker/setup-buildx-action@v3` | Multi-platform builder |
| 4 | `docker/login-action@v3` | Authenticate to `ghcr.io` |
| 5 | `docker/metadata-action@v5` | Generate semver tags + OCI labels |
| 6 | `docker/build-push-action@v6` | Build and push, GHA layer cache |

### Tag strategy

For a `v2.22.0` git tag `docker/metadata-action` produces:

| Tag | Notes |
|---|---|
| `ghcr.io/oisee/vsp:2.22.0` | Exact version |
| `ghcr.io/oisee/vsp:2.22` | Latest patch of 2.22.x |
| `ghcr.io/oisee/vsp:2` | Latest minor/patch of major 2 |
| `ghcr.io/oisee/vsp:latest` | Stable only — withheld for `-rc`/`-beta`/`-alpha` |
| `ghcr.io/oisee/vsp:manual` | `workflow_dispatch` only |

### Multi-platform

`linux/amd64` and `linux/arm64` are built via QEMU on the standard GitHub runner.
Docker selects the correct variant automatically at pull/run time.

### Layer cache

```yaml
cache-from: type=gha
cache-to: type=gha,mode=max
```

Reuses cached layers between builds. When only metadata changes, only the final
layers are rebuilt.

---

## Documentation (`docs/docker.md`)

The guide covers every configurable aspect of vsp when run via Docker:

| Section | Key topics |
|---|---|
| Quick Start | Minimal `docker run -i` commands |
| Pre-Built Images (GHCR) | Image location, tags, pull commands, platforms, workflow diagram, visibility |
| Building the Image | Local build, `--build-arg` version injection, multi-platform buildx |
| How vsp Runs in Docker | stdio model, no ports, `-i` requirement |
| Configuration Reference | Full env var table, all `SAP_*` options |
| Authentication | Basic auth, cookie file (volume mount), cookie string, `--env-file` |
| Tool Mode | `focused` vs `expert` |
| Disabling Tool Groups | `T/H/D/U/C` codes |
| Safety / Read-Only | `READ_ONLY`, op codes `R/S/Q/C/D/U/A`, package allowlist |
| Transport Management | Opt-in CTS tools, read-only transport, allowed transport patterns |
| Feature Flags | `auto`/`on`/`off` per feature, token usage impact |
| Network / TLS | `SAP_INSECURE`, custom CA, proxy, host network, Docker networks |
| MCP Client Integration | Claude Desktop JSON examples, `--env-file` pattern |
| Common Configurations | 4 named recipes (read-only prod, sandboxed Z*, expert, cookie auth) |
| Updating the Image | Pull, rebuild from source, pin versions, Dockerfile-only fix via `workflow_dispatch` |
| Security Notes | Credential handling, non-root, TLS |
| Troubleshooting | Exit immediately, TLS errors, auth conflicts, verbose logging, missing tools |
