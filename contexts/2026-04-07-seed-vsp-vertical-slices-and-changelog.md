# 2026-04-07 Seed — vsp-vertical-slices-and-changelog

Read this first:
- [context/2026-04-07-wisdom-vsp-vertical-slices-and-changelog.md](/home/alice/dev/vibing-steampunk/context/2026-04-07-wisdom-vsp-vertical-slices-and-changelog.md)

## Self-Contained Prompt

You are resuming work in `~/dev/vibing-steampunk`.

Context:
- The repo’s operational analysis slices are now in a stronger state than the docs initially suggested. `vsp slim`, `vsp health`, and `vsp api-surface` are all usable and SAP-validated. A shared package acquisition helper was extracted into `cmd/vsp/acquire.go`.
- PR `#89` was already reviewed, fixed, cherry-picked, and closed. The current open implementation thread is a new package-level `vsp changelog` CLI slice.

What to read first:
1. [context/2026-04-07-wisdom-vsp-vertical-slices-and-changelog.md](/home/alice/dev/vibing-steampunk/context/2026-04-07-wisdom-vsp-vertical-slices-and-changelog.md)
2. [cmd/vsp/changelog.go](/home/alice/dev/vibing-steampunk/cmd/vsp/changelog.go)
3. [cmd/vsp/changelog_test.go](/home/alice/dev/vibing-steampunk/cmd/vsp/changelog_test.go)
4. [cmd/vsp/acquire.go](/home/alice/dev/vibing-steampunk/cmd/vsp/acquire.go)
5. [cmd/vsp/api_surface.go](/home/alice/dev/vibing-steampunk/cmd/vsp/api_surface.go)
6. [cmd/vsp/devops.go](/home/alice/dev/vibing-steampunk/cmd/vsp/devops.go)
7. [pkg/adt/revisions.go](/home/alice/dev/vibing-steampunk/pkg/adt/revisions.go)
8. [pkg/graph/builder_transport.go](/home/alice/dev/vibing-steampunk/pkg/graph/builder_transport.go)

Primary task:
- Finish the package-level `vsp changelog` CLI slice.

Known current state:
- `cmd/vsp/changelog.go` and `cmd/vsp/changelog_test.go` were added.
- The first focused validation failed because `cmd/vsp/changelog.go` still references `*adt.Client` after the `adt` import was removed during a type switch to `graph.TransportHeader`.
- Re-add the correct import and finish compile/test cleanup first.

Required next steps:
1. Make `go test ./cmd/vsp` pass.
2. Make `go build ./cmd/vsp` pass.
3. Verify the changelog command shape is still thin:
   - package-level only
   - transport-grouped output
   - text + JSON
   - no MCP work yet unless it becomes nearly free
4. If compile/test passes, assess whether a cheap local polish is needed:
   - sorting
   - deduping object rows
   - `since` handling
   - top-N truncation semantics
5. If feasible, run a focused SAP validation on a real package.

What to avoid:
- Do not start a new `pkg/health` or `pkg/changelog` platform.
- Do not route work through `ddll send` unless exact-target routing is proven safe; it leaked messages into unrelated workers/sessions in this session.
- Do not rebuild `health` or `api-surface`; they already exist.
- Do not broaden into `upgrade-check`, `sketch`, or other new surfaces before closing `changelog`.

Success condition:
- `vsp changelog <package>` is build-clean, minimally tested, and consistent with the existing thin-slice style of the CLI.
