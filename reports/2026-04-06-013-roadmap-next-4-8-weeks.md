# VSP Roadmap: Next 4-8 Weeks

## What We Have Now

We already shipped the first useful static-analysis and refactoring layer:

- graph intelligence
- usage examples
- health
- api-surface
- slim
- rename-preview
- class-sections
- method-signature
- docs and guides

The next phase should not add more narrow readers.
It should consolidate, deepen, and turn the current tools into stronger workflows.

---

## Prioritized Roadmap

### 1. `CodeUnitContract` as the next shared model

This is the most important architecture move for the next 4-8 weeks.

What it covers:

- class methods
- interface methods
- function modules
- FORM routines
- SUBMIT/report entrypoints
- later, transaction entry shapes

Why this matters:

- we already have multiple readers for method-like surfaces
- the current surface is fragmenting into one-off parsers
- a unified contract model makes future tools cheaper and more consistent
- it becomes the shared input for usage examples, refactor previews, and generated docs

User job:

- "Show me the contract for this ABAP entrypoint without reading the whole object"

Effort/value:

- effort: medium
- value: very high
- risk: low if built as a shared model first, not a new user-facing command

### 2. Refactor preview family, not more standalone readers

The next user-facing win should be structured preview tools for edits, not more inspection commands.

Start with:

- method move preview
- attribute move preview
- method signature change preview
- package/prefix rewrite preview

Why this matters:

- these are the first tools that meaningfully reduce refactoring risk
- they build directly on rename-preview and class/method structure work
- they are a better fit for AI-assisted change planning than raw source reading

User jobs:

- "What breaks if I move this method to protected?"
- "What callers change if I alter this signature?"
- "What would a prefix-based fork look like before I touch the source?"

Effort/value:

- effort: medium-high
- value: very high
- risk: medium, but manageable if preview-only first

### 3. Make `api-surface` feed `upgrade-check`

`api-surface` is already useful, but it becomes much stronger when it becomes the inventory source for upgrade checks.

Next step:

- enrich top standard APIs with release state
- add release-risk grouping
- feed those results into a focused `upgrade-check`

Why this matters:

- `api-surface` answers what you depend on
- `upgrade-check` answers which dependencies are risky
- together, they become a real SAP upgrade planning workflow

User jobs:

- "Which standard APIs are our real upgrade risk?"
- "Which package dependencies should we fix first?"

Effort/value:

- effort: medium
- value: high
- risk: low if the first version stays inventory-driven

### 4. Package-level consolidation: shared acquisition helpers

This is not a feature, but it is the highest-ROI internal cleanup.

What to extract:

- package scope discovery
- object inventory
- reverse reference batching
- shared source-fetch/ranking helpers

Why this matters:

- `health`, `api-surface`, `slim`, and future `changelog` all repeat the same package acquisition pattern
- shared helpers reduce drift and bugs
- it keeps future features moving faster without changing UX

Effort/value:

- effort: low-medium
- value: high
- risk: low

---

## Quick Wins

These are worth doing early because they are practical and low-regret.

### A. `changelog`

What it should answer:

- "What changed recently in this package?"

Why now:

- transport history is already part of the graph story
- it is useful for daily team work and release context
- it complements `health` and `api-surface` without overlapping too much

Effort/value:

- effort: low-medium
- value: medium-high

### B. `where-used-config` refinement

What it should answer:

- "Who reads this config variable, and how confident are we?"

Why now:

- config impact is still only partially covered by `impact`
- this tool is already a clean entry point for config-driven behavior
- the next step is better ranking and optional integration into broader impact flows

Effort/value:

- effort: low
- value: medium

### C. `impact` configuration-aware polish

What it should answer:

- "If this config changes, what code paths become suspicious?"

Why now:

- current impact is good for code dependencies
- config-impact still feels separate and underpowered
- this is a good polish step once the shared acquisition layer is cleaner

Effort/value:

- effort: medium
- value: medium-high

---

## Mid Wins

These are important, but they should wait until the shared models above exist.

### A. `health` v2 with better package semantics

Why:

- current health is useful, but package behavior can still be heavy
- package/object latency and output shape can be improved after shared acquisition is in place

### B. `surface` as a generic internal model

Why:

- the mental model is correct
- the CLI should not expose a generic query DSL too early
- this should emerge from the real use of `api-surface` and future package-to-package use cases

### C. `move-method-preview` / `move-attribute-preview`

Why:

- these are the natural successors to class-sections and method-signature
- they should be built when refactoring workflows are clearly the next product emphasis

---

## High-Upside Bets

These are good ideas, but they should not pull the next sprint.

### A. Historical impact

This could become a strong differentiator:

- what changed together historically
- where changes actually caused pain
- which objects are repeatedly risky

But it needs data accumulation first.
We should keep passively collecting signals, not build the feature yet.

### B. Package clone/fork

This is powerful, but it is heavyweight and risky.

The right time is after the refactoring preview and contract models are trusted.

### C. Full `surface` CLI

The generic abstraction is valid, but the dedicated commands should keep proving usage first.

Do not ship a scope DSL before users ask for it repeatedly.

---

## What Not To Do Yet

1. Do not add more narrow readers.
2. Do not expose the generic `surface` CLI yet.
3. Do not build actual refactor execution before preview tools are strong.
4. Do not spend time on graph DB or persistence before the current workflows are clearly useful.
5. Do not turn `class-sections` into a flagship product.

---

## Recommended 4-8 Week Order

1. `CodeUnitContract`
2. shared package acquisition helpers
3. refactor preview family
4. `api-surface` -> `upgrade-check` tightening
5. `changelog`
6. config-impact polish

This is the best balance of product value, technical leverage, and implementation risk.

