# Refactoring Tools Ranking

Date: 2026-04-06
Status: Codex baseline ranking, with Claude review requested
Scope: VSP refactoring direction, including bold package/prefix clone ideas

## Executive Verdict

VSP should absolutely have a refactoring line.

But the order matters.

The best first refactoring tools are not giant "clone a whole package" operations.
The best first tools are the ones that:

- reduce manual rename/move risk
- show dependency leakage before edits
- give preview and dry-run output
- compose safely with the ADT and abapGit capabilities we already have

The bold package/prefix clone idea is real and valuable, but it is a heavyweight product, not a casual command.

## Ranking

### 1. Refactor Preview / Rewrite Planner

This should come first.

It answers:

- what objects would be renamed
- what references would be rewritten
- what dependencies would stay external
- what objects are unsafe / unsupported

Recommended shapes:

```bash
vsp refactor plan rename-package '$ZSCR_103' --to '$ZNEO_433'
vsp refactor plan prefix ZSCR_103 --to ZNEO_433
vsp refactor plan object CLAS ZCL_SCR_103_FOO --to ZCL_NEO_433_FOO
```

Why first:

- very high value
- low destructive risk
- useful even if actual execution stays manual at first
- can reuse graph + parser + search + existing file/copy/export capabilities

This is the safest bridge between analysis and mutation.

### 2. Safe Rename / Move Tools

This means focused refactors such as:

- rename object with reference preview
- move object to another package with dependency audit
- rename prefix inside a constrained object set
- update direct static references where confidence is high

Why this is high priority:

- daily usefulness
- smaller blast radius than package clone
- forces us to build the right primitives:
  - object inventory
  - reference discovery
  - preview
  - dry-run
  - validation

This is where VSP can become genuinely useful for controlled refactors rather than only analysis.

### 3. Package / Prefix Clone-Fork

This is the bold idea:

> Take a disciplined custom package or prefix family and clone it into a new package/prefix family with minimal dependency leakage.

Example:

- source package: `$ZSCR_103`
- source names: `ZCL_SCR_103_*`, `ZIF_SCR_103_*`, `ZSCR_103_*`
- target package: `$ZNEO_433`
- target names: `ZCL_NEO_433_*`, `ZIF_NEO_433_*`, `ZNEO_433_*`
- explicit allowlist for dependencies that may remain external

This is powerful, but it is not a "small refactor tool".

It is closer to:

- package fork
- codebase template instantiation from a live SAP package
- controlled product-line duplication

Verdict:

- strategically strong
- operationally dangerous
- worth doing later, after preview + rename primitives exist

### 4. Dependency Internalization / Leak Cutter

This is a refactoring helper, not a clone tool.

It answers:

- which dependencies prevent this package from being self-contained
- which objects should be copied or rewritten first
- which external references are allowed vs must be eliminated

Example:

```bash
vsp refactor leaks '$ZSCR_103' --allowed '$ZCOMMON,/VENDOR/*'
```

This is an excellent precursor to package clone.

### 5. Prefix Discipline Auditor

This is a low-risk, medium-value tool.

It checks:

- do object names actually follow the package prefix convention
- do internal references respect expected naming boundaries
- are there strays that will break mechanical cloning

This is not glamorous, but it makes clone/fork and rename tools much safer.

### 6. Full Clone / Fork Executor

This is the actual write operation after plan/preview/diff/audit.

This should come only after:

- planner
- preview
- leak audit
- rename/move primitives
- dry-run validation

It is the most dangerous tool in the set.

## Deep Evaluation: Package / Prefix Clone

## User Jobs

This solves real jobs:

- fork a vertical/business solution into a new customer or rollout variant
- duplicate a proven package skeleton for a new domain without manual copy-paste hell
- reduce hidden dependencies when splitting one custom area into another
- stamp out new "bounded packages" from an existing reference implementation

This is very different from a normal rename.

It is codebase forking inside SAP.

## Why It Is Attractive

- SAP teams really do duplicate package families and rename prefixes
- discipline around `ZCL_FOO_*`, `ZIF_FOO_*`, `ZFOO_*` naming makes this tractable
- VSP already has ingredients:
  - ADT source access and writes
  - rename support
  - package creation
  - abapGit ZIP copy/deploy
  - graph and dependency queries
  - usage examples / impact / api-surface style inventories

So this is not fantasy.

## Why It Is Dangerous

Main risks:

- reference rewriting is not only source replacement
- DDIC/RAP/service artifacts often require metadata-level consistency
- transactions, message classes, text elements, variants, lock objects, and generated artifacts complicate cloning
- package-local assumptions may break when object names change
- partial rewrites can leave hidden external dependencies
- dynamic calls and string-based object names are hard to detect safely

The failure mode is nasty:

- clone seems successful
- compiles partially
- still depends on the old package in subtle places
- or breaks at runtime only

## Best Architecture Choice

Do not build this first as direct live ADT rename/edit operations.

That is the wrong default.

Best path:

1. Export / snapshot stage
- use abapGit-style export or structured file export as the mutation workspace

2. Rewrite stage
- perform planned renames in files and metadata in a controlled artifact workspace
- parser/graph/search help produce the rewrite plan

3. Validate stage
- dependency audit after rewrite
- syntax / activation checks
- boundary checks
- leak report against allowed external dependencies

4. Deploy stage
- import into new package
- ideally with dry-run and staged deployment

So the safest architecture is:

- abapGit/file-based roundtrip for heavyweight clone
- ADT-native edits for smaller targeted refactors

## Recommended Clone Product Shape

### Phase A: Planner Only

```bash
vsp clone plan '$ZSCR_103' --to-package '$ZNEO_433' \
  --rename-prefix ZSCR_103=ZNEO_433 \
  --allow '$ZCOMMON,/VENDOR/*'
```

Output:

- objects to clone
- objects to rename
- references to rewrite
- references allowed to remain external
- unresolved/dangerous references
- unsupported artifact types

### Phase B: Workspace Generation

```bash
vsp clone export '$ZSCR_103' --plan plan.json --out clone-work/
```

This produces a reviewable artifact workspace.

### Phase C: Deploy

```bash
vsp clone apply clone-work/ --to-package '$ZNEO_433'
```

Only after review.

## Recommendation: Build Now or Later?

Not now as an execution feature.

Yes now as a design and planner track.

So:

- build supporting refactor primitives now
- build planner/dry-run before executor
- treat full clone as a later heavy feature

## Smaller Refactoring Wins Before Clone

These are worth doing first:

### A. Rename Planner

Given a proposed object rename:

- show static references
- show parser-confirmed references
- show unresolved dynamic risks
- show impacted callers / tests / transports

### B. Package Leak Audit

Given a package:

- show what prevents isolation
- classify allowed vs forbidden external dependencies

### C. Prefix Rewrite Preview

Given a prefix mapping:

- show which objects qualify
- show which names collide
- show which references would change

### D. Package Move Safety Check

Before moving an object:

- does it cross package boundaries
- does it rely on sibling package-local assumptions
- will tests or includes break

### E. Activation / Syntax Batch Verifier

After planned rewrites:

- run syntax / activation / ATC in a focused batch

These are all useful even if full clone never ships.

## Architecture Ranking

### 1. Hybrid: File/abapGit Workspace + Graph/Parser Planning

Best for heavyweight refactors.

Why:

- safer review surface
- easier preview/diff
- easier rollback story
- better for package clone and mass prefix rewrite

This should be the preferred architecture for big refactors.

### 2. ADT-Native Direct Mutations

Best for smaller focused refactors.

Examples:

- rename one object
- move one object
- change package assignment
- surgical source rewrite

Good for small tools, bad as the default for package-wide clone/fork.

### 3. Pure Regex / Text Rewrite

Never sufficient alone.

Can assist, but must not be the trust anchor.

### 4. Pure Graph-Driven Rewrite

Also not enough alone.

Graph is excellent for planning, ranking, and coverage.
It is not enough to perform safe semantic rewrites by itself.

## Final Recommendation

Recommended next refactoring roadmap:

1. `refactor plan`
2. safe rename / move tools
3. leak auditor and prefix auditor
4. file/abapGit-based clone workspace generator
5. only then full clone executor

Package/prefix clone is worth pursuing.

But it should be treated as:

- a heavyweight product surface
- staged, preview-first
- file/workspace-driven
- validated by graph/parser/activation checks

not as a casual one-shot live mutation command.
