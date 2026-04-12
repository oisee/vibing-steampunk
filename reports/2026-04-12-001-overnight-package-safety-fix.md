# Overnight Work Report: SAP_ALLOWED_PACKAGES Enforcement Fix

**Date:** 2026-04-12
**Authors:** Claude (implementation), Codex (audit + review), Alice (direction)
**PR:** https://github.com/oisee/vibing-steampunk/pull/101
**Commit:** 08e3d78
**Branch:** pr-93-fix

---

## Summary

User Philip Dolker reported that `SAP_ALLOWED_PACKAGES` did not restrict object
modifications to the configured package whitelist — vsp was letting him edit
objects in any package regardless of the setting.

Investigation confirmed this was a real enforcement bug, not a configuration
issue. The fix is committed and pushed in PR #101, with Codex's review approval.

## Problem

`SAP_ALLOWED_PACKAGES` was correctly parsed from the environment and wired into
`SafetyConfig.AllowedPackages`, but the check was only enforced on a small
subset of operations:

- `CreateObject` (crud.go)
- `CreateAndActivateProgram`, `CreateClassWithTests` (workflows.go)
- `UI5CreateApp` (ui5.go)

The main mutation paths that AI agents actually use to modify existing objects
bypassed the package check entirely. An agent could call `EditSource`,
`WriteSource`, `WriteProgram`, or `WriteClass` on any object in any package and
the `AllowedPackages` restriction would have no effect.

## Root Cause

The safety check was applied at create-time (where the package is an explicit
parameter) but never at edit-time (where the package must be resolved from the
existing object's metadata).

## Fix

Added `checkObjectPackageSafety(ctx, objectURL)` calls to all existing-object
mutation paths. This helper already existed in `client.go` — it resolves the
object's package via `SearchObject` and validates it against the whitelist,
with URL normalization for `/source/main` and `/includes/...` suffixes.

### Functions hardened

| Function | File | Check added |
|---|---|---|
| `EditSourceWithOptions` | `workflows_edit.go` | `checkObjectPackageSafety` |
| `WriteProgram` | `workflows.go` | `checkObjectPackageSafety` |
| `WriteClass` | `workflows.go` | `checkObjectPackageSafety` |
| `RenameObject` | `workflows_fileio.go` | `checkObjectPackageSafety` (old) + `checkPackageSafety` (target) |
| `UpdateSource` | `crud.go` | `checkObjectPackageSafety` + `checkTransportableEdit` |
| `DeleteObject` | `crud.go` | `checkObjectPackageSafety` + `checkTransportableEdit` |
| `CreateTestInclude` | `crud.go` | `checkObjectPackageSafety` + `checkTransportableEdit` |
| `UpdateClassInclude` | `crud.go` | `checkObjectPackageSafety` + `checkTransportableEdit` |
| `WriteMessageClassTexts` | `i18n.go` | `checkObjectPackageSafety` + `checkTransportableEdit` |
| `WriteDataElementLabels` | `i18n.go` | `checkObjectPackageSafety` + `checkTransportableEdit` |

Note: `WriteSource` didn't need a direct check — its update path delegates to
`WriteProgram`/`WriteClass` which are now guarded, and its create path
delegates to `CreateAndActivateProgram`/`CreateClassWithTests` which already
had checks.

### Tests

New tests in `client_test.go`:
- `TestClient_CheckObjectPackageSafety_NormalizesObjectURLs` — verifies
  `/source/main` and `/includes/...` URL normalization
- `TestClient_UpdateSource_EnforcesAllowedPackages`
- `TestClient_DeleteObject_EnforcesAllowedPackages`
- `TestClient_CreateTestInclude_EnforcesAllowedPackages`
- `TestClient_WriteMessageClassTexts_EnforcesAllowedPackages`

Full test suite passes: `go test ./...` clean, zero regressions.

## Defense in Depth

The fix applies checks at both the workflow layer (early rejection with clear
errors) and the low-level CRUD layer (backstop). This is intentional
redundancy — it makes intent obvious at the call sites and prevents future
workflow additions from silently bypassing the guard.

## Phase 2 — Unified Mutation Gate (commit `c3a5341`)

After Phase 1 landed, Alice asked for a single unified gate instead of
scattered per-function checks:

> "давайте сделайте всё через один auth/filter gate - потому что у нас
> гейты есть по CRUD, по TR (по CR!) по пакетам"

Phase 2 consolidates all three policy dimensions behind one entry point.

### Design

New file `pkg/adt/mutation_gate.go`:

```go
type MutationContext struct {
    Op        OperationType     // safety op (R/U/C/D/A/W/...)
    OpName    string            // human name for error messages
    ObjectURL string            // for existing-object path resolution
    Package   string            // for create path (explicit package)
    Transport string            // transport request number
    Surface   MutationSurface   // ADT or UI5 (different resolution)
}

func (c *Client) checkMutation(ctx, m) error {
    // 1. Operation type check
    // 2. Package ownership check
    //    - explicit Package > resolve via ObjectURL > fail closed
    // 3. Transportable-edit check
}
```

Precedence: explicit `Package` wins for create ops; otherwise the gate
resolves the package from `ObjectURL` via `SearchObject`; if neither is
available under an active whitelist, **fail closed** with a clear error
naming the op.

### Migrated mutators (15 functions)

All mutation paths in `pkg/adt/` now use the unified gate:

- `workflows_edit.go`: `EditSourceWithOptions`
- `workflows.go`: `WriteProgram`, `WriteClass`, `CreateAndActivateProgram`,
  `CreateClassWithTests`
- `workflows_source.go`: `WriteSource` (top-level gate; delegates still gate)
- `workflows_fileio.go`: `RenameObject` (dual gate: delete old + create target)
- `crud.go`: `UpdateSource`, `DeleteObject`, `CreateObject`,
  `CreateTestInclude`, `UpdateClassInclude`
- `i18n.go`: `WriteMessageClassTexts`, `WriteDataElementLabels`
- `ui5.go`: `UI5CreateApp`, `UI5UploadFile`, `UI5DeleteFile`, `UI5DeleteApp`

### Behavior change — UI5 fail-closed

The UI5 surface previously bypassed package policy silently. In Phase 2
this is closed:

- `UI5CreateApp` works as before (explicit `Package` parameter)
- `UI5UploadFile`, `UI5DeleteFile`, `UI5DeleteApp` now **fail closed**
  when `SAP_ALLOWED_PACKAGES` is set — they return a clear error
  pointing to the follow-up (UI5 app→package resolution)
- When no package policy is configured, UI5 mutations work as before

Users combining `AllowedPackages` with UI5 writes will see:

> operation 'UI5UploadFile' on UI5 surface is blocked: UI5 app→package
> resolution not yet implemented, cannot verify package against
> SAP_ALLOWED_PACKAGES (tracked as follow-up)

### Tests

New file `pkg/adt/mutation_gate_test.go` with 10 tests:

- `TestCheckMutation_NoPolicy_Passes`
- `TestCheckMutation_OpType_Blocked`
- `TestCheckMutation_ExplicitPackage_NotInWhitelist`
- `TestCheckMutation_ObjectURL_ResolvesADTPackage`
- `TestCheckMutation_UI5Surface_BlockedWhenPolicyActive`
- `TestCheckMutation_UI5Surface_AllowedWhenNoPolicy`
- `TestCheckMutation_MissingObjectURLAndPackage_FailsClosed`
- `TestClient_UI5UploadFile_BlockedUnderAllowedPackages`
- `TestClient_UI5DeleteFile_BlockedUnderAllowedPackages`
- `TestClient_UI5DeleteApp_BlockedUnderAllowedPackages`

Full suite still green.

## Remaining Follow-up

UI5 app→package resolution remains the one outstanding item, but the gap
is no longer a silent bypass — it's a clear fail-closed error until the
resolver is implemented. Recommended approach (from Codex's audit):

1. Add `UI5ResolveAppPackage(appName)` using BSP metadata
2. Let `SurfaceUI5` use that resolver instead of fail-closed
3. Ship as a follow-up PR

## Timeline

- **23:00** — User report received from Philip Dolker via Alice
- **23:05** — Independent audits by Claude and Codex; both identified the
  same gap in the workflow layer
- **23:15** — Alice approved fix implementation
- **23:35** — Phase 1 fix implemented across 7 files, tests pass
- **23:45** — Codex review approved Phase 1, no blockers
- **23:50** — Phase 1 committed (`08e3d78`), pushed, PR #101 created
- **23:55** — Graceful handover
- **next day** — Alice requested unified gate ("один auth/filter gate")
- **next day** — Phase 2 implemented: `checkMutation`, 15 mutators
  migrated, UI5 fail-closed, 10 new tests; Codex reviewed and approved
- **next day** — Phase 2 committed (`c3a5341`), pushed

## Philip Reply Draft (not yet sent)

> Hi Philip,
>
> Thank you for reporting this — and congrats-thanks for the 200+ stars!
>
> You found a real bug. Your configuration was correct, but
> `SAP_ALLOWED_PACKAGES` was only being enforced on object creation paths
> (like `CreateObject`). The main mutation paths for existing objects —
> `EditSource`, `WriteSource`, `WriteProgram`, `WriteClass`, and others —
> were not checking the package restriction at all. So vsp would let an
> agent modify any object regardless of the configured package whitelist.
>
> This is now fixed. The package ownership check is enforced on all
> standard ADT object mutation paths (edit, write, update, delete, rename).
>
> One additional note: if `$CONDUCT_RAPTEST` is a transportable package
> (not `$TMP`), you may also need to set `SAP_ALLOW_TRANSPORTABLE_EDITS=true`
> in your env block to allow writes to objects in that package. Without it,
> vsp blocks edits to objects in non-local packages as an extra safety layer.
>
> The fix will be in the next release. A small remaining area (UI5/BSP app
> mutations) will be completed separately, but that shouldn't affect typical
> ABAP development scenarios.
>
> Thanks again for the detailed report — it helped us find and close a real
> enforcement gap.

## Pending Actions

- [ ] Merge PR #101 to main
- [ ] Rebuild release binaries (if hotfix desired)
- [ ] Send reply to Philip
- [ ] Open follow-up issue for UI5 BSP app-to-package resolution
