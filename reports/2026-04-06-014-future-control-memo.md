# Future Control Memo

**Date:** 2026-04-06
**Subject:** Near-term control memo after the new analysis/refactoring wave

## Executive View

The strongest conclusion is simple:

- stop adding narrow reader commands
- start consolidating internals
- deepen the strongest analysis surfaces
- begin passive data collection for the next quarter's differentiated bets

The current product already has enough new surface area.
The next leverage now comes from stronger shared models and stronger preview/risk tools.

## Strongest Next Product Moves

1. **CodeUnitContract**
   - Generalize the current method-signature work into a shared internal contract model.
   - First targets: class methods, interface methods, FMs, FORMs, SUBMIT/report params.
   - Why: this prevents `method-signature`, `fm-signature`, `form-signature`, and similar narrow readers from multiplying.

2. **Refactor Preview Family**
   - Build `move-method-preview`, `move-attribute-preview`, and signature-change preview on top of existing `rename-preview` foundations.
   - Why: VSP becomes much more differentiated when it helps developers change code safely, not only inspect it.

3. **API Surface -> Upgrade Tightening**
   - Keep `api-surface` as a dedicated command, then deepen it with release-state and upgrade exposure.
   - Why: this creates a clean path from inventory to assessment.

4. **Config-Impact Polish**
   - Strengthen config-aware impact without collapsing `where-used-config` too early.
   - Why: there is still a real gap between config-reader discovery and a trustworthy config-impact story.

## Strongest Internal Consolidations

1. **Shared package acquisition helper**
   - `deps`, `api-surface`, `slim`, and package-mode `health` all repeat the same package inventory plus `WBCROSSGT`/`CROSS` plumbing.
   - Extract that first.

2. **Shared reference-evidence base type**
   - `rename-preview`, `slim`, and `api-surface` all produce variants of caller/target/source/confidence/count evidence.
   - Converge these on a small common base model.

3. **Reduce `cli_extra.go` sprawl**
   - Split analysis/refactor/read command wiring into smaller files.
   - Do not change user-facing command grouping yet.

## Highest-Upside Longer Bets

1. **Historical Impact / Time Machine**
   - Combine transport history, impact, health, and API surface into time-aware change risk.

2. **Safe Refactoring Cockpit**
   - Unify preview flows into a stronger refactoring control surface.

3. **Standard API Drift Radar**
   - Extend `api-surface` into change-aware standard-contract exposure.

4. **Usage Corpus Builder**
   - Turn `examples` into a reusable corpus for grounded AI coding assistance.

5. **Package Clone/Fork Planner**
   - Start as a planner, not an executor.
   - Use export/rewrite/validate/deploy rather than risky live mutation first.

## Passive Data Collection To Start Now

- co-change snapshots from transport history
- package and object inventory snapshots
- API surface snapshots with release-state enrichment
- health snapshots over time
- usage-example snippet and caller-frequency caches
- rename-preview and impact-preview telemetry

This is worth doing now because several of the best future features get materially better once history exists.

## What Not To Do Yet

- do not introduce a generic `surface --from --to` CLI yet
- do not collapse the CLI into one mega `analyze` or `read` command
- do not push execution refactors before preview tools are strong
- do not invest in graph DB/persistence as a primary product move yet
- do not keep adding standalone readers around every ABAP unit type

## Practical Order

1. Shared package acquisition helper
2. `CodeUnitContract`
3. Refactor preview family
4. `api-surface` -> upgrade tightening
5. config-impact polish
6. passive snapshotting groundwork for historical features

