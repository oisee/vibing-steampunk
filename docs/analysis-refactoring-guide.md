# Analysis and Refactoring Guide

This guide explains the newer VSP analysis and refactoring-oriented commands:

- what question each command answers
- when it is the right tool
- when it is not
- why it is useful for both humans and AI coding assistants

This is a practical guide.
For graph internals and data-source details, see [Graph Guide](graph-guide.md).

## Mental Model

The recent tools fall into three families:

### 1. Graph Intelligence

These answer questions about relationships:

- what changes together
- what depends on what
- who reads config

### 2. Code-Unit Understanding

These answer questions about how code is shaped:

- what a method accepts and returns
- how a class is structured
- how a thing is actually used in real code

### 3. Refactoring and Cleanup Intelligence

These answer questions about change safety:

- what breaks if I rename this
- what looks unused
- what can be slimmed or hidden

## Command Guide

## `vsp graph co-change`

Example:

```bash
vsp graph co-change CLAS ZCL_FOO
vsp graph co-change CLAS ZCL_FOO --top 10
vsp graph co-change CLAS ZCL_FOO --format json
```

What it answers:

- what usually moves together with this object in transports

Why it is useful:

- finds hidden change bundles
- helps upgrade planning and release sequencing
- gives AI assistants real transport history instead of guesses

Use it when:

- you are changing an object and want to know what tends to travel with it
- you want to discover tightly coupled work items in SAP transport history

Do not use it when:

- you need runtime truth
- you need call-site examples

## `vsp graph where-used-config`

Example:

```bash
vsp graph where-used-config ZMY_FLAG
vsp graph where-used-config ZMY_FLAG --no-grep
vsp graph where-used-config ZMY_FLAG --format json
```

What it answers:

- who likely reads this `TVARVC` variable

Why it is useful:

- much faster than manual grep across packages
- good first-pass config impact finder
- gives confidence ranking (`HIGH` vs `MEDIUM`)

Use it when:

- a variable changes and you need likely readers
- you are tracing config-driven behavior

Do not use it when:

- you need exact semantic truth
- the variable name is built dynamically

## `impact` (currently MCP-first)

MCP example:

```json
SAP(action="analyze", params={
  "type": "impact",
  "object_type": "CLAS",
  "object_name": "ZCL_FOO",
  "max_depth": 3
})
```

What it answers:

- who statically depends on this object

Why it is useful:

- reverse dependency blast-radius estimate
- strong input for refactoring and regression planning
- useful for AI assistants before edits

Use it when:

- you want to know who may be affected by a change
- you need reverse dependency traversal

Current limit:

- static reverse dependency, not runtime truth
- parser overlay helps, but this is not full dynamic impact yet

## `vsp examples`

Example:

```bash
vsp examples FUNC Z_CALCULATE_TAX
vsp examples CLAS ZCL_TRAVEL --method GET_DATA
vsp examples PROG ZREPORT --submit
vsp examples PROG ZPRICING --form CALC_TAX
```

What it answers:

- show me real usage examples, not just references

Why it is useful:

- one of the best tools for AI assistants
- gives concrete calling patterns and snippets
- much better than line-number-only where-used

Use it when:

- you want to understand how something is actually called
- you want realistic few-shot examples from your own system
- you are learning an unfamiliar API or FM

Do not use it when:

- you only need dependency counts
- you want transport history rather than code examples

## `vsp health`

Example:

```bash
vsp health --package '$ZDEV'
vsp health --package '$ZDEV' --fast
vsp health CLAS ZCL_ORDER_SERVICE
vsp health CLAS ZCL_ORDER_SERVICE --format json
```

What it answers:

- what shape is this package or object in right now

Why it is useful:

- combines tests, ATC, boundaries, and staleness into one snapshot
- good for pre-change checks and package reviews
- useful operational summary for AI agents

Use it when:

- you want a quick quality snapshot
- you want to inspect a package before refactoring

Current note:

- package health can be heavier/slower than object health

## `vsp api-surface`

Example:

```bash
vsp api-surface '$ZDEV'
vsp api-surface '$ZDEV' --include-subpackages
vsp api-surface '$ZDEV' --with-release-state
```

What it answers:

- which SAP standard APIs this custom package depends on most

Why it is useful:

- inventory of real standard contracts
- input for upgrade-check and cloud-readiness thinking
- helps prioritize which standard APIs deserve examples and docs

Use it when:

- you want to understand your standard SAP coupling
- you want to prepare for upgrade or clean-core work

Do not use it when:

- you need snippet-level usage
- you want custom-to-custom dependency analysis

## `vsp slim`

Example:

```bash
vsp slim '$ZDEV'
vsp slim '$ZDEV' --include-subpackages
vsp slim '$ZDEV' --format json
```

What it answers:

- which objects look like dead-code candidates

Why it is useful:

- safe cleanup intelligence
- highlights candidates for trimming before broader refactors
- gives AI assistants a starting shortlist without deleting anything

Use it when:

- you want cleanup candidates
- you suspect a package carries abandoned code

Important limitation:

- v1 reports zero static incoming references
- dynamic/framework entrypoints may still exist
- this is candidate detection, not deletion proof

## `vsp rename-preview`

Example:

```bash
vsp rename-preview CLAS ZCL_OLD_HELPER ZCL_NEW_HELPER
vsp rename-preview FUNC Z_OLD_FM Z_NEW_FM
vsp rename-preview PROG ZOLD_REPORT ZNEW_REPORT
```

What it answers:

- what static references are likely affected by renaming this object

Why it is useful:

- preview-first refactoring
- explicit risk warnings for dynamic calls, string literals, config refs
- avoids blind rename attempts

Use it when:

- you are preparing an object rename
- you need to judge rename blast radius before making writes

Do not use it when:

- you need the rename to be executed
- you want exact runtime coverage of dynamic references

## `vsp class-sections`

Example:

```bash
vsp class-sections ZCL_FOO
vsp class-sections ZCL_FOO --format json
```

What it answers:

- how a class is split across `PUBLIC`, `PROTECTED`, `PRIVATE`

Why it is useful:

- structural view for visibility hygiene
- useful as a precursor to future move-method / move-attribute previews

Use it when:

- you want a quick visibility layout
- you are evaluating encapsulation before refactoring

Current note:

- this is more of a structural helper than a flagship command

## `vsp method-signature`

Example:

```bash
vsp method-signature ZCL_FOO GET_DATA
vsp method-signature ZCL_FOO FACTORY --format json
```

What it answers:

- what this method accepts, returns, and raises

Why it is useful:

- avoids reading a whole class just to inspect one contract
- very helpful for AI assistants doing targeted changes
- good foundation for later signature-preview/mutation tools

Use it when:

- you need method parameters quickly
- you want contract-level understanding before editing a caller

Current limit:

- focused on class methods today
- broader `code-unit contract` generalization is a natural next step

## Which Tool Should I Use?

If your question is:

- "What usually changes with this?" -> `graph co-change`
- "Who depends on this?" -> `impact`
- "Who reads this TVARVC variable?" -> `graph where-used-config`
- "How do people actually call this?" -> `examples`
- "What shape is this package/object in?" -> `health`
- "What SAP standard APIs do we rely on?" -> `api-surface`
- "What looks unused?" -> `slim`
- "What breaks if I rename this?" -> `rename-preview`
- "What is this class's visibility structure?" -> `class-sections`
- "What is this method's contract?" -> `method-signature`

## Why This Helps AI Assistants

These tools reduce hallucination pressure by giving agents:

- transport history instead of guesses
- real dependency sets instead of inferred ones
- real usage snippets instead of invented examples
- explicit risk warnings before refactoring
- compact contracts instead of whole-file overreading

That means:

- better edits
- fewer blind changes
- faster orientation in large SAP codebases

## Current Best Pairings

Useful workflows:

- `health` -> `impact` -> `rename-preview`
- `api-surface` -> `examples`
- `slim` -> `rename-preview`
- `class-sections` -> `method-signature`

## Roadmap Direction

The current tools are good, but the deeper abstractions are becoming visible:

- `CodeUnitContract`
  - unify method/FM/FORM/SUBMIT contracts
- `RefactorPreview`
  - unify rename/move/signature-change previews
- `CleanupIntelligence`
  - deepen `slim`

That is where the next round of consolidation should happen.
