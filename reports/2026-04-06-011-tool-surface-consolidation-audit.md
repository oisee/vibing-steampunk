# Tool Surface Consolidation Audit

Date: 2026-04-06
Status: Codex audit, Claude review requested
Scope: recent analysis and refactoring command surface

## Executive Verdict

The current surface is already productive, but it is starting to drift into a pile of one-off commands.

The right next move is not to delete working tools.
The right next move is to recognize the deeper command families they belong to and stop adding new top-level commands without checking whether they fit one of those families.

Today we already have three families hiding in plain sight:

1. graph intelligence
2. code-unit understanding
3. refactor preview / cleanup intelligence

The real opportunity is to consolidate around these families.

## Current Commands

Relevant recent CLI tools:

- `graph co-change`
- `graph where-used-config`
- `health`
- `api-surface`
- `examples`
- `slim`
- `rename-preview`
- `class-sections`
- `method-signature`

## Commands That Are Genuinely Distinct

These solve clearly different jobs and should stay conceptually separate:

### `graph co-change`

Distinct job:

- what tends to move together in transports

This is not call graph, not impact, not dead code.
It belongs in the graph family and should stay.

### `graph where-used-config`

Distinct job:

- who reads this TVARVC variable

This is config usage, not code contract or refactor preview.
It stays.

### `health`

Distinct job:

- is this package/object healthy right now

This is orchestration of multiple signals, not graph traversal and not code-unit reading.
It stays.

### `api-surface`

Distinct job:

- what standard SAP contracts does this custom scope depend on

This is a strategic inventory tool.
It stays, though internally it points toward a future generalized `surface` model.

### `examples`

Distinct job:

- show real usage snippets

This is not just where-used and not just contract reading.
It absolutely stays.

## Commands That Are Mostly Thin Views Over a Deeper Model

These are useful, but they should be treated as members of a stronger family rather than standalone conceptual pillars.

### `method-signature`

This is valuable.
But it is really one slice of a broader abstraction:

- **code-unit contract**

The deeper model is:

- method contract
- interface method contract
- FM contract
- FORM contract
- SUBMIT/report contract
- maybe transaction entry contract later

So `method-signature` should probably survive as a convenience wrapper,
but the underlying model should become `contract`.

### `class-sections`

This is the weakest standalone tool in the set.

It is mostly:

- a structural lens over class metadata

Useful as:

- foundation for visibility refactors
- optional enrichment in context/analyze

Weak as:

- a flagship standalone command

Recommendation:

- keep the underlying logic
- keep the CLI for now if already shipped
- do not expand it as its own product line
- treat it as a building block for refactoring previews and context enrichment

### `slim`

`slim` is actually the right user-facing product shape,
but under the hood it should be seen as:

- cleanup intelligence family

Meaning:

- dead objects
- dead methods
- dead includes
- maybe stale/abandoned code later

So `slim` stays, but it should become the umbrella rather than spawning separate public commands like `dead-code`, `unused-methods`, etc.

### `rename-preview`

This is valuable as a command,
but conceptually it is the first member of:

- **refactor preview framework**

Future siblings:

- move-method-preview
- move-attribute-preview
- method-signature-preview
- prefix-rewrite-preview

So `rename-preview` should stay,
but we should stop thinking of it as a one-off tool.

## Stronger Shared Result Models

These are the key generalizations worth doing.

## 1. `CodeUnitContract`

This is the strongest unification opportunity.

Should cover:

- class method
- interface method
- function module
- FORM
- SUBMIT/report selection screen
- transaction entry later

Core fields:

- owner type / name
- unit kind
- unit name
- visibility / level if relevant
- inputs by direction
- outputs
- exceptions / raising
- modifiers
- raw signature/source snippet

This would subsume:

- `method-signature`
- future FM/form/report signature readers

Potential CLI:

```bash
vsp contract CLAS ZCL_FOO --method GET_DATA
vsp contract FUNC Z_MY_FM
vsp contract PROG ZREPORT --submit
vsp contract PROG ZREPORT --form BUILD_OUTPUT
```

## 2. `Structure Lens`

This is the weaker but still useful generalization.

Should cover:

- class sections
- maybe interface sections
- maybe package/module structure later

But this should probably remain secondary.
It is less important than `CodeUnitContract`.

## 3. `RefactorPreview`

This is the second strong generalization.

Current and future members:

- rename preview
- method move preview
- attribute move preview
- method signature change preview
- prefix rewrite preview

Shared fields:

- target
- proposed change
- affected objects
- confidence per affected object
- unresolved risks
- maybe follow-up validation hints

This is where the current refactoring line should consolidate.

## 4. `Cleanup Intelligence`

Umbrella for:

- dead candidates
- unused methods
- maybe stale candidates later

User-facing brand:

- `slim`

This is already the right product name.

## What Should Fold Into `context` / `analyze`

### Fold or enrich

- `class-sections` summary
- `method-signature` summary in class/method context flows

These are often better as enrichments than as primary commands.

### Keep standalone

- `examples`
- `health`
- `api-surface`
- `slim`
- `rename-preview`
- graph queries

These have sufficiently distinct jobs.

## Concrete Recommendations

### Keep as standalone pillars

- `examples`
- `health`
- `api-surface`
- `slim`
- `rename-preview`
- graph tools

### Keep but demote to building-block status

- `class-sections`
- `method-signature`

### Generalize next

1. Build `CodeUnitContract`
2. Build `RefactorPreview` family deliberately
3. Keep `slim` as cleanup umbrella

## Next-Step Recommendation

The best next move is not another isolated reader.

The best next move is:

### Option A

Define the shared `CodeUnitContract` model and migrate `method-signature` toward it.

### Option B

Define `RefactorPreview` result shape and build `method-signature-preview` or `move-method-preview` on top.

If choosing one:

- choose `CodeUnitContract` first

Why:

- stronger abstraction
- reduces future reader fragmentation
- useful to both humans and AI
- supports examples, refactors, and context enrichment

## Final Position

We should stop asking:

- "what new one-off command should we add?"

and start asking:

- "is this another graph question, another contract view, another refactor preview, or another cleanup signal?"

That framing will keep the surface coherent while still letting us ship useful small tools.
