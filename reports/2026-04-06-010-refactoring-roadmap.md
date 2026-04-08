# Refactoring Roadmap

Date: 2026-04-06
Status: Proposed execution roadmap
Scope: VSP refactoring line after analysis-heavy graph/features work

## Executive Summary

VSP should grow a refactoring line, but in a preview-first way.

The right sequence is:

1. read-only refactoring intelligence
2. structured previews
3. constrained execution tools
4. heavyweight workspace-driven refactors

This avoids jumping straight from analysis to dangerous mass mutation.

## Product Shape

Refactoring in VSP should be split into four layers:

### 1. Insight

Read-only tools that answer:

- what is dead
- what is unused
- what would break
- what leaks outside package boundaries

### 2. Preview

Tools that show proposed changes before writing:

- rename preview
- class section move preview
- method signature change preview
- prefix rewrite preview

### 3. Structured Surgery

Small write tools with strong scope:

- move object to package
- move method/attribute between sections
- update method signature
- execute single-object rename

### 4. Heavyweight Refactoring

Workspace-driven flows:

- package clone planner
- exported rewrite workspace
- validation and leak checks
- clone apply/deploy

## Priority Order

### Phase 1: First Features

#### 1. `rename-preview`

Examples:

```bash
vsp rename-preview CLAS ZCL_OLD ZCL_NEW
vsp rename-preview --prefix ZSCR_103 --to-prefix ZNEO_433 --package '$ZSCR_103'
```

Reason:

- highest daily value
- directly useful
- builds the right reference/impact primitives

#### 2. `slim`

This should be the user-facing umbrella for dead-code trimming.

Examples:

```bash
vsp slim '$ZDEV'
vsp slim '$ZDEV' --report html
vsp slim CLAS ZCL_FOO
```

Scope:

- dead objects
- dead methods
- dead includes later

Reason:

- safe read-only cleanup intelligence
- very demo-friendly
- highly practical for teams afraid to touch dead code

#### 3. `class sections`

Examples:

```bash
vsp class sections ZCL_FOO
vsp class move-method-preview ZCL_FOO GET_DATA --to protected
vsp class move-attribute-preview ZCL_FOO GV_CACHE --to private
```

Reason:

- useful structured refactoring surface
- smaller blast radius than generic rewrites
- good bridge from reading to safe mutation

#### 4. `method signature`

Examples:

```bash
vsp method signature ZCL_FOO GET_DATA
vsp method signature-preview ZCL_FOO GET_DATA --add-importing iv_id type string
vsp method signature-preview ZCL_FOO GET_DATA --rename-param iv_old iv_new
```

Reason:

- very common real-world change
- lets users and AI operate at method level without rewriting whole classes
- pairs naturally with caller-impact analysis

### Phase 2: Controlled Execution

#### 5. `move-object`

Examples:

```bash
vsp move-object CLAS ZCL_FOO --to-package '$ZNEW'
```

Reason:

- already close to existing capability
- useful package-reorg primitive

#### 6. `class section move`

Execution only after preview and impact checks exist.

Examples:

```bash
vsp class move-method ZCL_FOO GET_DATA --to private
vsp class move-attribute ZCL_FOO GV_CACHE --to protected
```

#### 7. `method signature apply`

Execution only after signature preview is trustworthy.

Examples:

```bash
vsp method signature-apply ZCL_FOO GET_DATA --add-importing iv_id type string
vsp method signature-apply ZCL_FOO GET_DATA --rename-param iv_old iv_new
```

#### 8. `dependency leak audit`

Examples:

```bash
vsp refactor leaks '$ZSCR_103' --allowed '$ZCOMMON,/VENDOR/*'
```

Reason:

- key preparation for safe package split or clone

### Phase 3: Heavyweight Refactoring

#### 9. `clone plan`

Examples:

```bash
vsp clone plan '$ZSCR_103' --to-package '$ZNEO_433' \
  --rename-prefix ZSCR_103=ZNEO_433 \
  --allow '$ZCOMMON,/VENDOR/*'
```

Reason:

- very high upside
- but dangerous without planner-first workflow

#### 10. `clone workspace`

Examples:

```bash
vsp clone export '$ZSCR_103' --plan plan.json --out clone-work/
```

Use abapGit/file-style workspace, not direct mass mutation.

#### 11. `clone apply`

Examples:

```bash
vsp clone apply clone-work/ --to-package '$ZNEO_433'
```

Only after:

- review
- leak validation
- syntax/activation validation

## Architecture Principles

### Preview Before Mutation

Any refactoring tool with meaningful blast radius should support:

- preview
- impact summary
- confidence / unresolved risk notes

### Structured, Not Generic

Prefer:

- class section tool
- method signature tool
- rename preview

over:

- arbitrary source rewrite command

### Heavy Refactors Use Workspace Flow

For package clone and mass prefix rewrites:

- export
- rewrite
- validate
- deploy

Do not start with direct live mutation in SAP.

### Graph / Parser Are Validation Layers

Use:

- graph for dependency coverage
- parser for statement-level precision
- ADT for source/object mutation and validation

No single layer is sufficient alone.

## Recommended Immediate Backlog

### Next 4 Refactoring Slices

1. `rename-preview`
2. `slim`
3. `class sections`
4. `method signature`

This is the best balance of:

- usefulness
- safety
- implementation leverage from existing VSP capabilities

## Final Position

The most important idea is:

- do not jump straight to clone/fork executor

The best way to make refactoring strong in VSP is to first become excellent at:

- preview
- structure-aware edits
- caller-aware impact
- cleanup intelligence

Then the heavyweight refactors become much safer and much more credible.
