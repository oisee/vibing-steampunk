# Seed: Next Agent Session

**Date:** 2026-04-08
**Wisdom file:** `contexts/2026-04-08-wisdom-execution-semantics-cache-and-context-compression.md`

---

## Project Context

You are resuming work in `~/dev/vibing-steampunk`.

This session did not implement runtime semantics yet. It clarified the architecture and saved the decisions as ADRs and steering docs.

The main theme is:

- stronger context compression for agents
- a shared code-unit contract model
- explicit execution semantics and effect profiles
- opt-in SQLite as analysis cache, not source of truth

## Read First

1. [contexts/2026-04-08-wisdom-execution-semantics-cache-and-context-compression.md](/home/alice/dev/vibing-steampunk/contexts/2026-04-08-wisdom-execution-semantics-cache-and-context-compression.md)
2. [docs/adr/004-execution-semantics-and-effect-profiles.md](/home/alice/dev/vibing-steampunk/docs/adr/004-execution-semantics-and-effect-profiles.md)
3. [docs/adr/005-opt-in-sqlite-analysis-cache.md](/home/alice/dev/vibing-steampunk/docs/adr/005-opt-in-sqlite-analysis-cache.md)
4. [reports/2026-04-08-002-context-contract-and-effects-sprint-guidance.md](/home/alice/dev/vibing-steampunk/reports/2026-04-08-002-context-contract-and-effects-sprint-guidance.md)
5. [reports/2026-04-08-003-execution-semantics-steering-plan.md](/home/alice/dev/vibing-steampunk/reports/2026-04-08-003-execution-semantics-steering-plan.md)
6. [pkg/graph/graph.go](/home/alice/dev/vibing-steampunk/pkg/graph/graph.go)
7. [pkg/graph/queries_signature.go](/home/alice/dev/vibing-steampunk/pkg/graph/queries_signature.go)
8. [pkg/graph/queries_examples.go](/home/alice/dev/vibing-steampunk/pkg/graph/queries_examples.go)
9. [pkg/cache/sqlite.go](/home/alice/dev/vibing-steampunk/pkg/cache/sqlite.go)

## Current State

- No code wiring was done yet for `CodeUnitContract`, `ExecutionSemantics`, `EffectProfile`, or `MethodContext`.
- The repo already has optional SQLite cache infrastructure, but product-level wiring and semantic-table scope are still open.
- The worktree contains unrelated untracked files. Do not assume all untracked files belong to this thread.

## Recommended Next Slice

Do the smallest real foundation step:

1. add shared structs in `pkg/graph`
2. map current method-signature output into `CodeUnitContract`
3. add placeholder `ExecutionSemantics` and `EffectProfile` structs
4. add a `MethodContext` result shape
5. avoid broad CLI/MCP UX changes unless nearly free

## If Continuing Into Implementation

Preferred order:

1. contract model first
2. local effect model second
3. one method-first consumer third
4. SQLite semantic caching only after source-hash provenance is explicit

## Questions To Keep Straight

- topology vs semantics vs derived propagation must stay separate
- local facts are safer than transitive claims
- LUW safety must be confidence-marked, not overclaimed
- APC must be treated as explicit execution context, not just “async”
- SQLite is cache only, not truth

## What To Avoid

- no graph DB pivot
- no many-new-edge-type explosion
- no default raw request archive
- no unrestricted transitive effect closure
- no attempt to solve all code-unit kinds equally in the first implementation slice
