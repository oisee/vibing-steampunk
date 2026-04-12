# Issue Overview: `install zadt-vsp` False Success + Trial-System Lock Failures

**Date:** 2026-04-09  
**Reporter:** Marcello Urbani  
**Context:** SAP 2023 trial / public repro system, plus local code review of current `main`

## Summary

There are two separate problems:

1. `vsp install zadt-vsp` can report success while deploying no objects.
2. Edit/lock operations can still fail on some systems even after the stateful-session fix.

The installer problem is the more concrete and higher-confidence issue. It is not just a diagnostics problem. On a clean system, the current installer path is structurally unable to create the embedded objects, and then it also hides that failure.

## Findings

### 1. Clean install path is missing required create metadata

The installer deploy loop in [cmd/vsp/devops.go](/home/alice/dev/vibing-steampunk/cmd/vsp/devops.go) and the MCP install loop in [internal/mcp/handlers_install.go](/home/alice/dev/vibing-steampunk/internal/mcp/handlers_install.go) call `WriteSource(..., Mode: upsert)` with only `Package` and `Mode`.

They do **not** pass `Description`.

That matters because the create branch of `WriteSource` in [pkg/adt/workflows_source.go](/home/alice/dev/vibing-steampunk/pkg/adt/workflows_source.go) explicitly requires both:

- `Package`
- `Description`

If `Description` is empty, `writeSourceCreate()` returns a result with:

- `Success = false`
- `Message = "Description is required for creating new objects"`
- `error = nil`

So on a fresh target package:

- package creation can succeed
- object creation can fail for every embedded object
- the installer sees no Go error
- the installer prints `OK` / `Deployed`

This exactly matches the user symptom: “it says it worked but only creates the package”.

This is the primary root cause.

### 2. Installer loops ignore `result.Success`

Both install loops currently only check `err != nil` after `WriteSource`.

That is insufficient because `WriteSource` is intentionally designed to return many SAP/workflow failures as:

- `result.Success = false`
- `result.Message = ...`
- `error = nil`

Examples include:

- missing description/package for create
- syntax-check failures
- activation failures
- object existence mode mismatch
- lock/update/unlock workflow failures surfaced as workflow result messages

For installer/reporting purposes, `result.Success` is the real success bit. Ignoring it makes the installer optimistic when SAP/workflow-level execution already failed.

### 3. The MCP install handler also lacks post-write verification

The main ZADT_VSP install handler in [internal/mcp/handlers_install.go](/home/alice/dev/vibing-steampunk/internal/mcp/handlers_install.go) does not verify that deployed objects actually exist afterward.

Given that the tool is a bootstrap installer and uses only standard ADT capabilities, it should be conservative:

- detect per-object failure
- verify existence after deploy
- summarize missing objects clearly

At the moment it can end with a success-looking summary while the package is empty or partially populated.

### 4. Package creation handling should be gentler on older ADT systems

The MCP handler already documents that `/sap/bc/adt/packages` may be missing on older systems and continues after package creation failure. That is reasonable in principle, but incomplete in execution.

If package creation fails and the package truly does not exist, the installer should stop early with a direct diagnostic instead of cascading into a long stream of object failures.

The gentle behavior should be:

- try create
- if create fails, probe package existence
- continue only if existence is confirmed
- otherwise stop with a message that the package must be pre-created manually

The CLI path is currently stricter: it aborts immediately on package create failure. The MCP path is more tolerant, but not yet diagnostic enough.

### 5. Lock/edit path is improved, but still has one plausible session-risk area

The lock fix from issue `#88` is present in [pkg/adt/crud.go](/home/alice/dev/vibing-steampunk/pkg/adt/crud.go):

- `LockObject` forces `Stateful: true`
- `UpdateSource` forces `Stateful: true`
- `UnlockObject` forces `Stateful: true`

That is the correct direction and likely fixes the original same-session lock-handle problem for most systems.

However, one plausible remaining edge case exists in [pkg/adt/http.go](/home/alice/dev/vibing-steampunk/pkg/adt/http.go):

- modifying requests fetch CSRF tokens through `fetchCSRFToken()`
- `fetchCSRFToken()` uses global `SessionType`, not per-request `Stateful`
- the later lock/write/unlock requests can be forced stateful per request

If a stricter SAP system binds the CSRF token to the same stateful session context that will later be used for locking, this mismatch could still cause trial-specific failures even though the CRUD requests themselves are marked stateful.

I have not yet reproduced that on system `110`, so this remains a plausible secondary cause, not a confirmed one.

## Why This Matters

The initial installer is a bootstrap path using standard ADT only. That means it has to be:

- gentle
- explicit
- truthful
- self-diagnosing

A bootstrap installer must never claim success unless it has confirmed object existence. “Package created” is not “tool installed”.

## Recommended Fixes

### Immediate

1. Pass object descriptions into installer `WriteSource` calls.

Use the embedded object metadata already available in the install loop and set:

- `Description: obj.Description`

in both:

- [cmd/vsp/devops.go](/home/alice/dev/vibing-steampunk/cmd/vsp/devops.go)
- [internal/mcp/handlers_install.go](/home/alice/dev/vibing-steampunk/internal/mcp/handlers_install.go)

2. Treat `!result.Success` as deployment failure.

Installer logic should be:

- if `err != nil`: transport/runtime failure
- else if `!result.Success`: SAP/workflow failure
- else: success

3. Print the actual workflow message.

Per object, report at least:

- create/update decision
- `result.Message`
- activation or syntax details when available

### Short-term reliability

4. Verify object existence after deployment.

For each expected object, run a read/search/existence probe and include a final “installed / missing” summary.

5. Harden package handling.

If package creation fails:

- re-check package existence
- only continue if it exists
- otherwise abort with a direct manual-remediation message

6. Add a `check_only` or “preflight” mode to CLI install if not already exposed equivalently.

Preflight should validate:

- package create capability
- required ADT endpoints
- write capability on one scratch object or dry validation path
- whether abapGit-dependent optional pieces will be skipped

### Lock/edit follow-up

7. Audit CSRF/session alignment for per-request stateful writes.

The safe design is:

- when a write request will be stateful, the CSRF acquisition used for that write path should also be stateful

8. Add an integration test specifically for:

- stateless default client
- per-request stateful lock/update/unlock sequence
- token refresh/retry path

## Suggested acceptance criteria

The installer should only report success when all of these are true:

1. target package exists
2. each required object write returns `result.Success == true`
3. each required object is readable/searchable after deployment
4. final summary distinguishes deployed, skipped, failed, and missing-after-verify

## Candidate code touch points

- [cmd/vsp/devops.go](/home/alice/dev/vibing-steampunk/cmd/vsp/devops.go)
- [internal/mcp/handlers_install.go](/home/alice/dev/vibing-steampunk/internal/mcp/handlers_install.go)
- [pkg/adt/workflows_source.go](/home/alice/dev/vibing-steampunk/pkg/adt/workflows_source.go)
- [pkg/adt/http.go](/home/alice/dev/vibing-steampunk/pkg/adt/http.go)
- [pkg/adt/crud.go](/home/alice/dev/vibing-steampunk/pkg/adt/crud.go)

## Current confidence

- Installer false-success diagnosis: high confidence
- Missing `Description` as primary clean-install blocker: high confidence
- Trial-system lock failure exact remaining cause: medium confidence

## Next practical step

Implement the installer fixes first:

- pass `Description`
- check `result.Success`
- verify deployed object existence

That addresses the concrete user-visible failure immediately and makes subsequent lock-session debugging much easier because install state becomes trustworthy.
