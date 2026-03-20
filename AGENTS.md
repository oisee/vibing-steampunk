# AGENTS.md — AI Agent Guidelines

This file guides AI coding agents (Claude Code, Gemini CLI, GitHub Copilot, etc.) working on the **vsp** project.

For full development guidelines, see **[CLAUDE.md](CLAUDE.md)**.

## Documentation Rule

> **New features, bug fixes, configuration changes, and usage information belong in the `docs/` folder — not in root-level markdown files.**

When you add or change something:

| Change | Where to document it |
|--------|----------------------|
| New MCP tool | `docs/tools/reference.md` |
| New configuration option | `docs/configuration.md` |
| New feature (debugger, RAP, etc.) | `docs/features/<feature>.md` |
| Architecture change | `docs/architecture.md` or `docs/adr/` |
| Breaking change or release note | `docs/changelog.md` (and `CHANGELOG.md`) |

Root-level files like `README.md` are entry points only — they link into `docs/` for details. Do not expand them with new content.

## Documentation Site

The docs are built with **MkDocs + Material theme** and deployed to GitHub Pages automatically on every push to `main`.

```bash
# Local preview
pip install mkdocs-material
mkdocs serve   # → http://127.0.0.1:8000
```

Site structure is defined in `mkdocs.yml` at the project root.

## Quick Reference

```bash
# Build
go build -o vsp ./cmd/vsp

# Unit tests
go test ./...

# Integration tests (requires SAP system)
SAP_URL=http://host:port SAP_USER=user SAP_PASSWORD=pass \
  go test -tags=integration -v ./pkg/adt/
```

See [CLAUDE.md](CLAUDE.md) for the full development reference including:
- Codebase structure and key files
- Code patterns for adding tools
- SAP object naming conventions
- Security notes
