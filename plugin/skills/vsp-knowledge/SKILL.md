---
name: vsp-knowledge
description: Core SAP/VSP domain knowledge for ABAP development. Use when working with VSP MCP tools — provides best practices, workflow patterns, gotchas, and safety guidance. Triggers on ABAP development, SAP system interaction, or VSP tool usage.
---

# VSP Development Knowledge

## Method-Level Operations (Critical)

For ABAP classes, ALWAYS use method-level source operations. Reading/writing an entire class wastes 95% of tokens.

- `GetSource` with `method` parameter — reads only that method
- `GetSource` with `include_type` — reads specific include (definitions, implementations, testclasses, locals_def, locals_imp, macros)
- `WriteSource` and `EditSource` support the same parameters

Only read the full class when you need a structural overview. For edits, always target the specific method or include.

## Tool Modes

VSP operates in three modes (`--mode` flag or `SAP_MODE` env var):

- **Focused mode** (default, `--mode focused`): ~81 essential tools — covers 95% of development tasks.
- **Expert mode** (`--mode expert`): full ~150-tool surface — adds atomic CRUD operations, AMDP debugger, gCTS suite, refactoring tools, advanced experimental features.
- **Hyperfocused mode** (`--mode hyperfocused`): a single universal `SAP(action, target, params)` tool that routes internally to all handlers. ~150 tokens of tool definitions instead of ~30k. Built for token-budget-constrained sessions. The LLM discovers capabilities at runtime via `SAP(action="help")` or `SAP(action="help", target="<action>")`.

Choose focused unless you specifically need expert-only tools (LockObject/UnlockObject atomic ops, AMDP debugging, gCTS, RenameObject, MoveObject, …) or token budget is very tight (then hyperfocused).

## Object Lifecycle

Every ABAP development follows this lifecycle:

```
Search → Read → [Create/Lock] → Edit → Syntax Check → Activate → Test → [Unlock]
```

In focused mode, `WriteSource` and `EditSource` handle lock/unlock automatically. In expert mode, you must manage locks manually.

## Search Strategy

| Need | Tool | Best For |
|------|------|----------|
| Find by name | SearchObject | Quick lookup: `ZCL_*`, `*INVOICE*` |
| Find by content | GrepObjects | Regex in specific objects |
| Find across packages | GrepPackages | Recursive regex search |
| Find definition | FindDefinition | Navigate to symbol source |
| Find usages | FindReferences | Impact analysis before changes |

## Safety Configuration

VSP has enterprise safety controls. Before attempting write operations, be aware of:

- **Read-only mode** (`--read-only`): All writes blocked
- **Package restrictions** (`--allowed-packages`): Can only modify objects in whitelisted packages
- **Operation filtering** (`--allowed-ops` / `--disallowed-ops`): Specific operation types blocked
- **Transportable edit protection**: Objects in transportable packages require `--allow-transportable-edits`

If a write operation fails with a safety error, explain the restriction to the user rather than trying to work around it.

## Authentication

VSP supports three auth methods (use only one):

- **Basic auth** — `--user X --password Y` (or `SAP_USER`/`SAP_PASSWORD` env)
- **Cookie auth** — `--cookie-file <netscape.txt>` or `--cookie-string "key=val; key=val"` (for systems with custom SSO)
- **Browser-based SSO** — `--browser-auth` opens a Chromium-based browser for interactive Kerberos / SAML / Keycloak login. Use `--cookie-save <path>` to persist the session for reuse with `--cookie-file`.

If a user mentions Kerberos, SAML, ADFS, Keycloak, or "single sign-on," recommend `--browser-auth`.

## ABAP SQL Differences

When using `RunQuery`, use ABAP SQL syntax — NOT standard SQL:

| Standard SQL | ABAP SQL | Notes |
|-------------|----------|-------|
| `DESC` | `DESCENDING` | Full keyword required |
| `ASC` | `ASCENDING` | Full keyword required |
| `LIMIT n` | Use `max_rows` parameter | No LIMIT clause |
| `SELECT *` | Works | But prefer named fields |

See `references/sql-gotchas.md` for complete reference.

## Feature Detection

Run `GetFeatures` to check what's available on the connected system:

| Feature | What It Enables |
|---------|----------------|
| hana | HANA-specific features, AMDP debugging |
| abapgit | Git integration, ZIP export (158 object types) |
| rap | RAP/OData development (DDLS, BDEF, SRVD, SRVB) |
| transport | CTS transport management |
| ui5 | UI5/Fiori BSP management |
| amdp | AMDP/SQLScript debugging |

Features auto-detect by default. Missing features are not errors — they indicate the system doesn't support that capability.

## ZADT_VSP Prerequisites

Advanced features (WebSocket debugging, RFC calls, report execution, AMDP debugging) require the ZADT_VSP handler deployed on the SAP system. Check with `GetFeatures` or `ListDependencies`. Install with `InstallZADTVSP` if missing.

See `references/prerequisites.md` for deployment details.
