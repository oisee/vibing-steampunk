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

## Focused vs Expert Mode

VSP operates in two modes:
- **Focused mode** (default): 81 essential tools — covers 95% of development tasks
- **Expert mode**: 122 tools — adds atomic CRUD operations, AMDP debugger, advanced features

Use focused mode unless you specifically need: LockObject/UnlockObject (atomic ops), AMDP debugging, or tools in disabled groups.

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
