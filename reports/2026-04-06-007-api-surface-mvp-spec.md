# API Surface MVP Spec

Date: 2026-04-06
Status: Proposed implementation spec
Priority: After Health

## Goal

Build a focused inventory tool that answers:

> What SAP standard APIs does our custom package or namespace actually depend on?

This is a dependency inventory tool, not a full upgrade checker and not a generic cross-scope query language.

## User Jobs

Primary jobs:

- see which standard contracts a custom package really uses
- rank the most central SAP APIs in a custom landscape
- identify upgrade-sensitive standard dependencies before migration work
- choose which standard APIs deserve examples, docs, and onboarding attention first

Secondary jobs:

- feed `usage examples` with top-used standard targets
- feed future `upgrade-check` with real dependency inputs
- give AI assistants one compact standard-dependency snapshot instead of manual SQL work

## Why Dedicated `api-surface` First

Long-term, the deeper model is a generic dependency surface between scopes.

For MVP, that abstraction is premature.

Dedicated `api-surface` is better because:

- the user job is obvious
- output format is stable
- discoverability is better than a generic `surface --from ... --to ...`
- implementation can reuse old `deps` logic without inventing a scope DSL

Internals should still be written so later `surface` generalization is possible.

## MVP Scope

Supported scopes in v1:

- package
- package with subpackages

Optional v1.1 scope:

- prefix / namespace family

Recommended CLI:

```bash
vsp api-surface '$ZDEV'
vsp api-surface '$ZDEV' --include-subpackages
vsp api-surface '$ZDEV' --top 50
vsp api-surface '$ZDEV' --with-release-state
vsp api-surface '$ZDEV' --format json
```

Recommended MCP:

```json
SAP(action="analyze", params={"type":"api_surface","package":"$ZDEV"})
SAP(action="analyze", params={"type":"api_surface","package":"$ZDEV","top_n":50})
SAP(action="analyze", params={"type":"api_surface","package":"$ZDEV","with_release_state":true})
```

## Data Sources

### 1. TADIR

Use `TADIR` to enumerate custom repository objects in the package.

Needed fields:

- `OBJECT`
- `OBJ_NAME`
- `DEVCLASS`

### 2. WBCROSSGT

Use as the primary dependency inventory for OO/type-level references.

Needed fields:

- `INCLUDE`
- `OTYPE`
- `NAME`

### 3. CROSS

Use as the procedural supplement for function modules, submits, and other procedural refs.

Needed fields:

- `INCLUDE`
- `TYPE`
- `NAME`

### 4. Optional Release-State Enrichment

Use ADT `GetAPIReleaseState` only for the ranked top N entries when explicitly requested.

This must stay optional in v1 because per-object enrichment is comparatively expensive.

## Classification Rules

### Caller Side

Caller scope is package-bound custom code.

Only source-bearing code objects should participate in v1:

- `CLAS`
- `PROG`
- `INTF`
- `FUGR`

### Callee Side

Keep only standard targets in the final inventory.

Important:

- do not assume every slash namespace is SAP standard
- `Z*` and `Y*` are customer objects
- `/Z.../` and `/Y.../` are also customer namespaces
- other `/.../` names cannot be assumed standard without configuration

So v1 policy should be:

- `Z*`, `Y*` => custom
- `/Z.../`, `/Y.../` => custom
- configured custom prefixes / namespaces => custom
- everything else => standard by default

This policy is good enough for MVP, but it must be documented as configurable and not universal truth.

## Result Shape

```json
{
  "scope": {
    "kind": "package",
    "name": "$ZDEV",
    "include_subpackages": false
  },
  "summary": {
    "custom_objects": 87,
    "unique_standard_apis": 41,
    "total_standard_references": 312
  },
  "entries": [
    {
      "node_id": "CLAS:CL_HTTP_CLIENT",
      "name": "CL_HTTP_CLIENT",
      "type": "CLAS",
      "usage_count": 19,
      "caller_count": 12,
      "caller_packages": ["$ZDEV"],
      "release_state": "RELEASED"
    }
  ]
}
```

Human CLI output should be simpler:

- one short summary line
- ranked top list
- optional release-state column

## Ranking

Primary sort:

- `caller_count` descending

Secondary sort:

- `usage_count` descending

Tertiary sort:

- `node_id` ascending for stability

Why:

- unique caller spread is more meaningful than repeated references from one object
- raw usage count still helps distinguish heavy use from incidental use

## Implementation Approach

Thin slice first.

Reuse the existing `deps` command logic where it is already correct:

1. enumerate package objects via `TADIR`
2. query `WBCROSSGT` and `CROSS`
3. normalize includes to object-level callers
4. filter to standard targets
5. aggregate counts
6. optionally enrich top N with release state
7. format text/json

Suggested implementation order:

1. reusable internal helpers for:
   - package inventory
   - target classification
   - standard-target aggregation
2. CLI command `api-surface`
3. MCP `analyze type=api_surface`

Do not start with a new graph builder unless current query shaping proves insufficient.

## Non-Goals For v1

Do not include yet:

- generic `surface --from --to`
- domain/module clustering
- historical trend tracking
- replacement suggestions
- auto-generated corpora
- package-to-package crossing reports
- boundary verdicts

## Relationship To Other Features

- `health` answers: is this package in good shape?
- `api-surface` answers: what standard contracts does it rely on?
- `upgrade-check` will later answer: which of those contracts are risky for Cloud/S/4?
- `usage examples` can later mine examples for the top-ranked standard APIs
