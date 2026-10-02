# Developer notes

Background that used to live in CLAUDE.md. It is useful when you work in the
area it describes, but an agent does not need it in every conversation. The
status of current work lives on the project's Trail Map, in
[`agenda/AGENDA.md`](../agenda/AGENDA.md) and in the GitHub issues. This file
records how things got the way they are.

## Graph engine (`pkg/graph/`)

- The package is feature-complete for its original design:
  - core types and parser dependency extraction;
  - the boundary analyzer;
  - SQL adapters (`builder_sql.go`: CROSS, WBCROSSGT and WBCROSSGTX long names);
  - `builder_transport.go` and `builder_config.go`;
  - the D010INC compile-time load graph (`builder_loads.go`, `vsp loads`);
  - `graph.ExtractEffects`, which has callers in `cmd/vsp/effects.go` and `internal/mcp/handlers_effects.go`;
  - the `queries_*.go` surface behind slim, health, impact, api-surface, rename and examples.
- `pkg/graph/adtsource` (#322) is the one ADT/SQL source that both front ends
  call for TADIR/TFDIR package resolution, the TVARVC readers, D010INC rows and
  the shared health verdicts. `graph.AddSourceDeps` replaced eleven hand-rolled
  parse loops.
- #325 made package scanning honest:
  - `adtsource.SourceKind` maps listing codes exactly (FUGR/F is read through the group's sources; FUGR/FF goes with its group, or alone);
  - unknown listing types become gaps;
  - crossing pairs are keyed by node ID and keep the worst direction.
- Still twinned between `cmd/vsp` and `internal/mcp`:
  - the package-level health collectors' summaries;
  - the CROSS/WBCROSSGT reverse readers;
  - transport data fetching.
- History: CLAUDE.md described this package as "in progress, only a parser
  adapter" for four months after the SQL adapters shipped, then listed D010INC
  and `ExtractEffects` as pending for a week after both shipped (2026-08-25).
  Check the code before trusting a status line.
- Design: [002](../reports/2026-04-05-002-graph-engine-design.md),
  [003](../reports/2026-04-05-003-graph-engine-alignment-for-claude.md)

## Debugger

- Built:
  - `pkg/adt/debugger.go`;
  - a session that survives across MCP tool calls (`internal/mcp/handlers_debug_session.go`);
  - six registered tools;
  - ADT-native AMDP routing;
  - a local web UI, `vsp debug ui` (2026-09-02).
- Breakpoints and the debug loop go through `/sap/bc/adt/debugger*` on a held
  ADT session. No ZADT_VSP is needed. The old note "REST breakpoints return 403
  on newer SAP" described the stateless client, not the SAP release. The
  breakpoint kinds (line, statement, exception, message) were verified against
  A4H on 2026-08-21.
- Not built: a DAP adapter. It is tracked in #184; a design for `vsp dap` plus
  a VS Code companion is being drafted together with open-steamgate.
- Design: [001](../reports/2026-04-05-001-gui-debugger-design.md)

## Lock handles and sessions (#91 family)

- **Symptom:** HTTP 423 `ExceptionResourceInvalidLockHandle` on a write after
  LOCK.
- **Cause:**
  - `SessionType` defaults to stateless, and unflagged requests are stamped `stateless`.
  - So any request between LOCK and the write can retire the ICM context and kill the handle. Examples: a package lookup, a CSRF probe, a stateless mutation.
- **What closed it:**
  - session affinity for the lock window;
  - the keep-alive ticker defaulting to off and skipping ticks inside a lock window (#168);
  - the lock handle being optional in MCP, so the window can't span a model turn (#183; it closed #169 and #181);
  - #251's isolation of concurrent callers on one client.
- **History:** #88, #92, #98, #110 and #132 are closed as duplicates of #91.
  Commit `22517d4` was once claimed to fix it; a third-party bisect named it as
  the start of a regression instead.
- **Still open:**
  - **#91:** reports from other releases. An S/4 816 user sees a 423 that A4H 758 does not reproduce (see the contributor PR #292).
  - **#166:** a failed mutation can strand the SAP-side ENQUEUE, which users clear in SM12. Deleting with a caller-supplied lock handle can also leave the lock with the session.

## RunReport in APC (#55)

This is not an architectural limit, though it was described that way for
months. Commit `34eb727` (2026-02-06) replaced a working XBP background-job and
spool implementation in `src/zcl_vsp_report_service.clas.abap` with a bare
`SUBMIT ... AND RETURN`, and deleted actions the Go client still speaks.

#261 added running a report as a background job from the SAP tool. Contributor
PR #293 restores the ZADT_VSP side with ALV capture.

## Closed and not ours

#45 and #46 asked for a sync script (`scripts/sync-upstream.sh`) that never
existed in this repository. They were filed from a downstream fork's workflow
and closed on 2026-09-01.
