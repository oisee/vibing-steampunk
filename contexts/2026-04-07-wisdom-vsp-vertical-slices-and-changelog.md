# 2026-04-07 Wisdom — vsp-vertical-slices-and-changelog

## What Was Done

### Product slices confirmed complete or effectively complete

- `vsp slim` usable slice was completed and SAP-validated by the parallel Claude worker.
  - Main evidence:
    - [cmd/vsp/cli_extra.go](/home/alice/dev/vibing-steampunk/cmd/vsp/cli_extra.go)
    - [cmd/vsp/acquire.go](/home/alice/dev/vibing-steampunk/cmd/vsp/acquire.go)
    - [pkg/graph/scope.go](/home/alice/dev/vibing-steampunk/pkg/graph/scope.go)
    - [pkg/graph/scope_test.go](/home/alice/dev/vibing-steampunk/pkg/graph/scope_test.go)
    - [pkg/graph/slim_integration_test.go](/home/alice/dev/vibing-steampunk/pkg/graph/slim_integration_test.go)

- `health` was discovered to already exist, so no second system was built.
  - Existing command:
    - [cmd/vsp/devops.go](/home/alice/dev/vibing-steampunk/cmd/vsp/devops.go)
  - Thin improvement added by Claude:
    - [pkg/graph/queries_health.go](/home/alice/dev/vibing-steampunk/pkg/graph/queries_health.go)
    - `E070` transport fallback for staleness, committed as `9ae10f3`

- `api-surface` was discovered to already exist as a complete vertical slice and was SAP-validated.
  - Main files:
    - [cmd/vsp/api_surface.go](/home/alice/dev/vibing-steampunk/cmd/vsp/api_surface.go)
    - [pkg/graph/queries_apisurface.go](/home/alice/dev/vibing-steampunk/pkg/graph/queries_apisurface.go)

- Shared package acquisition helper was extracted so `slim` and `api-surface` no longer duplicate the same package-scope + TADIR + reverse-ref plumbing.
  - New helper:
    - [cmd/vsp/acquire.go](/home/alice/dev/vibing-steampunk/cmd/vsp/acquire.go)
  - Claude commit:
    - `e549006`

- PR `#89` was reviewed, fixed, cherry-picked, and closed.
  - Merge-quality result, not blind acceptance.
  - Commit:
    - `8623acd`
  - Main files touched by Claude:
    - [pkg/abaplint/rules.go](/home/alice/dev/vibing-steampunk/pkg/abaplint/rules.go)
    - [pkg/abaplint/lint_test.go](/home/alice/dev/vibing-steampunk/pkg/abaplint/lint_test.go)
    - [pkg/adt/codeanalysis.go](/home/alice/dev/vibing-steampunk/pkg/adt/codeanalysis.go)
    - [pkg/adt/codeanalysis_test.go](/home/alice/dev/vibing-steampunk/pkg/adt/codeanalysis_test.go)
    - [internal/mcp/handlers_codeanalysis.go](/home/alice/dev/vibing-steampunk/internal/mcp/handlers_codeanalysis.go)

### New local work started in this session

- Began a new `vsp changelog` CLI slice locally.
  - New files created:
    - [cmd/vsp/changelog.go](/home/alice/dev/vibing-steampunk/cmd/vsp/changelog.go)
    - [cmd/vsp/changelog_test.go](/home/alice/dev/vibing-steampunk/cmd/vsp/changelog_test.go)
  - Intent:
    - package-level transport changelog
    - grouped by transport request
    - text + JSON output
    - uses package acquisition helper + `E071`/`E070`

## What Is Not Finished And Why

### 1. `cmd/vsp/changelog.go` is not yet build-clean

Reason:
- I removed the `adt` import while switching transport header handling to `graph.TransportHeader`, but the file still uses `*adt.Client` in function signatures.
- Current compile errors are in [cmd/vsp/changelog.go](/home/alice/dev/vibing-steampunk/cmd/vsp/changelog.go).

Observed failing validation:
- `go test ./cmd/vsp`
- `go build ./cmd/vsp`

Most likely next fix:
- restore the `adt` import in `cmd/vsp/changelog.go`
- rerun focused validation

### 2. `changelog` is CLI-only work in progress

Reason:
- no MCP wiring was attempted yet
- the first goal was the thinnest package-level CLI slice

### 3. `#88` and `#90` are still “shipped, reporter validation pending”

Reason:
- Claude fixed them in commit `27f4d7c`
- there is still no external confirmation from affected reporters/systems in this session

## Traps / Non-Obvious Decisions

### `ddll send` routing is unsafe right now

This is the biggest operational trap from the session.

Symptom:
- sending to `147aszyg:main` leaked messages to unrelated workers:
  - `tmhas69k:main` in `~/dev/vibing-steampunk` (`codex`)
  - `gv_wsvdk:main` in `~/dev/minz-vir`

Meaning:
- `ddll send` is not treating `session:worker` as a strict exact address in practice, or there is a bug in its match logic.

Operational consequence:
- do **not** keep sending coordination messages until routing is made safe
- otherwise project-control messages bleed into other repos/sessions

### `health` already existed

Before adding any new command, search the tree aggressively.

This session’s important discovery:
- `health` was already implemented in [cmd/vsp/devops.go](/home/alice/dev/vibing-steampunk/cmd/vsp/devops.go)
- the right move was canonicalization and a small fallback improvement, not rebuilding the feature

### `api-surface` already existed too

Same lesson:
- the engine already lived in [pkg/graph/queries_apisurface.go](/home/alice/dev/vibing-steampunk/pkg/graph/queries_apisurface.go)
- the CLI already lived in [cmd/vsp/api_surface.go](/home/alice/dev/vibing-steampunk/cmd/vsp/api_surface.go)

The repo has many “planned in docs, already half-or-fully implemented in code” cases. Search before coding.

### Freestyle ADT SQL has annoying edge cases

Documented by Claude during Slim V2:
- freestyle ADT query path did not support `OR` with `LIKE` reliably in the reverse-ref acquisition context
- the working fix was per-object queries

This should influence future data-acquisition code:
- prefer simple queries
- avoid getting too clever with wide `OR` chains

## Architectural Insights Not Well Captured Elsewhere

### The next best work was not a new surface, but leverage-debt reduction

After `slim`, `health`, and `api-surface` were all usable and SAP-validated, the right next move was extracting shared package acquisition rather than immediately starting `changelog` or `upgrade-check`.

That refactor was justified only after:
- multiple slices had proven the duplication was real
- the duplicated logic had stabilized

This is a good pattern for future work:
- thin vertical slice first
- shared helper second

### `health`, `api-surface`, and `slim` are now the stable “operational analysis trio”

That is a meaningful product milestone even though it was reached through a mix of discovery and refactor instead of pure greenfield work.

### `changelog` should probably use transport tables, not per-object `GetRevisions`, for MVP

Why:
- transport-table path covers package/object history in the shape requested by the spec
- `GetRevisions` is object-centric and source-type limited
- transport aggregation is the actual user value

That is the reason the local in-progress implementation was based on:
- package scope/TADIR object list
- `E071` object-to-transport
- `E070` transport headers

## Key Artifacts

- [cmd/vsp/acquire.go](/home/alice/dev/vibing-steampunk/cmd/vsp/acquire.go)
  Shared package acquisition helper extracted by Claude. This is the internal leverage cleanup that now supports `slim` and `api-surface`.

- [cmd/vsp/api_surface.go](/home/alice/dev/vibing-steampunk/cmd/vsp/api_surface.go)
  Existing API-surface CLI slice. Useful reference for thin CLI wiring style.

- [cmd/vsp/devops.go](/home/alice/dev/vibing-steampunk/cmd/vsp/devops.go)
  Existing `health` command. Important because it prevents accidentally rebuilding the feature.

- [cmd/vsp/changelog.go](/home/alice/dev/vibing-steampunk/cmd/vsp/changelog.go)
  New local, unfinished changelog CLI implementation. Build currently broken due to missing `adt` import after a type refactor.

- [cmd/vsp/changelog_test.go](/home/alice/dev/vibing-steampunk/cmd/vsp/changelog_test.go)
  New local tests for changelog transport aggregation semantics.

- [pkg/adt/revisions.go](/home/alice/dev/vibing-steampunk/pkg/adt/revisions.go)
  Existing revision primitives. Read this before deciding whether to use revision feeds or transport tables for changelog work.

- [pkg/graph/builder_transport.go](/home/alice/dev/vibing-steampunk/pkg/graph/builder_transport.go)
  Existing transport-domain data model and request/task collapse semantics.

- PR `#89`
  AnalyzeABAPCode v2. Reviewed, corrected, cherry-picked, and closed with credit.

- Issue `#88`
  Lock handle invalid. Fix shipped in `27f4d7c`; still needs reporter/system validation.

- Issue `#90`
  BTP auth redirect. Fix shipped in `27f4d7c`; still needs reporter/system validation.

## Bottom Line

The session materially improved VSP product shape:
- `slim`, `health`, and `api-surface` are now treated as real usable slices
- shared acquisition debt was cleaned up
- PR `#89` was integrated correctly

The main unfinished local work is `vsp changelog`, and the main operational blocker is unsafe `ddll send` routing.
