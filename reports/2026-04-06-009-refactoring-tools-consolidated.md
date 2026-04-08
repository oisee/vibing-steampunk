# Refactoring Tools Consolidated Ranking

Date: 2026-04-06
Status: Consolidated Codex + Claude position
Scope: VSP refactoring direction, including package clone/fork and class section tools

## Executive Summary

Refactoring is a strong next direction for VSP, but the tools should be sequenced by safety.

The highest-value near-term tools are:

1. read-only refactoring intelligence
2. preview-first rename/move helpers
3. focused source-structure surgery

The bold package/prefix clone idea is real and strategically valuable, but it should not be the first execution feature.

It should begin as:

- planner
- preview
- workspace export/rewrite/import flow

not as a one-shot live mutation command.

## Final Ranking

### Tier 1: Build First

#### 1. Rename Preview

Show exactly what would change if an object or prefix were renamed.

Examples:

```bash
vsp rename-preview CLAS ZCL_OLD ZCL_NEW
vsp rename-preview --prefix ZSCR_103 --to-prefix ZNEO_433 --package '$ZSCR_103'
```

Why first:

- high daily value
- low write risk
- builds the right primitives for later execution features

This should include:

- static references
- parser-confirmed references
- unresolved/dynamic risk notes
- impacted callers / tests / transports if cheap

#### 2. Dead Code Finder

Examples:

```bash
vsp dead-code '$ZDEV'
vsp dead-code '$ZDEV' --top 100
```

Why early:

- safe read-only analysis
- popular cleanup use case
- leverages reverse graph work already done

#### 3. Class Section Tools

This is your new idea, and it is a good one.

Meaning tools for:

- read class sections
- rewrite class sections
- move methods or attributes between `PUBLIC`, `PROTECTED`, `PRIVATE`

Examples:

```bash
vsp class sections ZCL_FOO
vsp class move-method ZCL_FOO GET_DATA --to protected
vsp class move-attribute ZCL_FOO GV_CACHE --to private
```

Why useful:

- very common cleanup in legacy ABAP
- smaller blast radius than full rename
- directly helps encapsulation and API hardening

Why not trivial:

- must preserve declarations and section structure
- must avoid mangling comments, ordering, and formatting
- moving from public to protected/private can break callers

Best first shape:

- start with section reader + preview
- then allow execution when impact is clearly shown

So:

- `class sections` and `move-to-section preview` should come before auto-executing destructive moves

#### 4. Move Object to Package

Claude is right that this is high-value and feasible.

Examples:

```bash
vsp move-object CLAS ZCL_FOO --to-package '$ZNEW'
```

This is useful, but I would still put preview and read-only analysis slightly ahead of broad execution features.

### Tier 2: Build Next

#### 5. Unused Method Finder

Examples:

```bash
vsp unused-methods CLAS ZCL_FOO
```

Very useful for class cleanup, especially paired with class section tools.

#### 6. Dependency Leak / Isolation Audit

Examples:

```bash
vsp refactor leaks '$ZSCR_103' --allowed '$ZCOMMON,/VENDOR/*'
```

This is the best precursor to package clone.

It answers:

- what prevents package isolation
- what may remain external
- what must be rewritten or copied

#### 7. Dependency Inversion Check

Examples:

```bash
vsp check-di '$ZDEV'
```

Good modernization tool:

- concrete class refs where interface refs should exist
- public section overexposure
- direct instantiation hotspots

### Tier 3: Heavyweight Refactoring

#### 8. Full Rename with Execution

Only after preview becomes solid.

Examples:

```bash
vsp rename-object CLAS ZCL_OLD ZCL_NEW
vsp rename-prefix --package '$ZSCR_103' ZSCR_103 ZNEO_433
```

Needs:

- preview
- confidence reporting
- write ordering
- validation

#### 9. Package / Prefix Clone-Fork

Anchor scenario:

- source package: `$ZSCR_103`
- target package: `$ZNEO_433`
- source naming family:
  - `ZCL_SCR_103_*`
  - `ZIF_SCR_103_*`
  - `ZSCR_103_*`
- target naming family:
  - `ZCL_NEO_433_*`
  - `ZIF_NEO_433_*`
  - `ZNEO_433_*`

This is very compelling for:

- template-based development
- customer forks
- rollout variants
- sandbox/training duplication

But it is also risky:

- incomplete rewrites
- dynamic references
- hidden external dependencies
- DDIC/service metadata issues
- activation ordering
- collisions and lock conflicts

## Package Clone Verdict

### Is it a good idea?

Yes.

It is one of the most interesting high-upside refactoring products VSP could eventually ship.

### Should we build it now?

No, not as executor.

### Should we prepare for it now?

Yes.

The right order is:

1. dependency leak audit
2. rename preview
3. class/package structure tools
4. clone planner
5. exported workspace rewrite
6. clone execution

## Safest Architecture for Clone / Fork

### Best v1 Architecture

abapGit or file-based roundtrip.

Flow:

1. export source package to workspace
2. generate rewrite plan
3. apply rewrites locally
4. validate for leaks, syntax, activation order
5. deploy to target package

Why this is better than direct live ADT mutation:

- reviewable diff
- better rollback story
- easier preview
- lower fear factor

### ADT-Native Direct Mutation

Keep for:

- single object rename
- small focused section changes
- small source surgery

Not for first package clone executor.

### Graph / Parser Assisted Validation

This should assist every serious refactor tool:

- graph for reference coverage
- parser for precise source-local rewrites
- confidence levels for uncertain matches

But it should not be the only layer.

## Class Section Tools: Should We Do Them?

Yes, but as a controlled structured-surgery toolset.

Recommended subfeatures:

### A. Section Reader

Show:

- public methods
- protected methods
- private methods
- public/protected/private attributes

This is useful immediately for AI assistants and humans.

### B. Section Move Preview

Examples:

```bash
vsp class move-method-preview ZCL_FOO GET_DATA --to private
vsp class move-attribute-preview ZCL_FOO GV_BUFFER --to protected
```

Should show:

- declaration diff
- caller impact
- likely visibility breakage

### C. Section Move Execution

Only after preview works well.

This is a strong medium-size refactoring feature, much safer than package clone and more precise than whole-object rename.

## Practical Roadmap Recommendation

### Near-Term

1. Rename preview
2. Dead code finder
3. Class section reader / move preview
4. Unused method finder

### Mid-Term

5. Dependency leak audit
6. Safe rename execution
7. Package move helper

### Later

8. Clone planner
9. Clone workspace generator
10. Clone executor

## Final Position

The strongest conclusion is:

- yes, refactoring should become a major VSP line
- yes, package/prefix clone is worth pursuing
- no, it should not be the first execution feature
- class section tools are a very good intermediate refactoring layer

If we do this right, VSP stops being only:

- read / analyze / explain

and becomes:

- plan / preview / validate / refactor

which is a much bigger strategic step.
