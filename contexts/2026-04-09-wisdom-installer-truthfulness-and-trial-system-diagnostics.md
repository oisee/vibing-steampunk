# 2026-04-09 Wisdom — installer truthfulness and trial-system diagnostics

## What Was Decided

This session clarified that the immediate priority is not a broad lock/session refactor, but making the `zadt-vsp` installer truthful, gentle, and diagnosable on first run.

The key position is:

1. bootstrap install must never claim success unless objects are verifiably present
2. installer diagnostics must reflect SAP/workflow failure, not only transport/runtime errors
3. lock/session investigation should continue only after install state becomes trustworthy

## Core Findings

### 1. The installer bug is stronger than “silent failure”

The main install path currently calls `WriteSource(..., Mode: upsert)` without passing `Description`.

That matters because the create path in `pkg/adt/workflows_source.go` requires:

- `Package`
- `Description`

If `Description` is missing, create returns:

- `result.Success = false`
- `result.Message = "Description is required for creating new objects"`
- `err = nil`

So on a clean package:

- package creation may succeed
- every object create may fail by design
- installer may still print success if it only checks `err`

This explains the user symptom “it says it worked but only creates the package”.

### 2. Installer success must use `result.Success`, not only `err`

`WriteSource` intentionally reports many SAP/workflow failures as structured result failures rather than Go errors.

For installer code, the real success contract is:

- `err != nil` means transport/runtime failure
- `result.Success == false` means SAP/workflow failure
- only `err == nil && result.Success == true` is actual success

### 3. Verification is part of the install contract

For a bootstrap installer using only standard ADT:

- package existence must be verified
- object existence must be verified
- final summary must distinguish deployed, skipped, failed, and missing-after-verify

“No error returned” is not enough.

### 4. The MCP and CLI installers should converge on reliability behavior

Observed difference:

- CLI install path is too optimistic about object writes
- MCP install path is somewhat more explanatory, but still not strict enough

Desired shared behavior:

1. pass `Description`
2. honor `result.Success`
3. print `result.Message`
4. verify package existence after package-create failure
5. verify object existence after deployment

### 5. Lock/session issue is still secondary

The stateful CRUD fix is present in `pkg/adt/crud.go`:

- `LockObject` uses `Stateful: true`
- `UpdateSource` uses `Stateful: true`
- `UnlockObject` uses `Stateful: true`

That likely resolved the original lock-handle/session bug for many systems.

One plausible remaining edge case is CSRF/session alignment in `pkg/adt/http.go`:

- CSRF fetch uses global session mode
- lock/write/unlock can force per-request stateful mode

This is worth investigating later, but not before the installer is made truthful.

## Project-State Notes Worth Carrying Forward

### Systems access

This session verified working SAP connector access to both:

- `105`
- `110`

Both responded to `system INFO` as:

- `systemId`: `A4H`
- `client`: `001`
- `sapRelease`: `758`
- `kernelRelease`: `75I`
- `abapRelease`: `758`

### Issue report created

Canonical report for this session:

- [reports/2026-04-09-001-install-and-lock-bug-report.md](/home/alice/dev/vibing-steampunk/reports/2026-04-09-001-install-and-lock-bug-report.md)

That report is the steering reference for the installer fix.

### Claude governance

Claude worker `7ln759eq:main` was explicitly steered to fix the installer first with narrow scope:

- `cmd/vsp/devops.go`
- `internal/mcp/handlers_install.go`

Required direction sent:

1. pass `Description: obj.Description`
2. treat `!result.Success` as failure
3. print `result.Message`
4. verify package existence after create failure
5. verify object existence before final success

And explicitly:

- do not redesign `WriteSource`
- do not start with CSRF/session refactors

## What To Avoid

- do not start broad HTTP/session surgery before installer truthfulness is fixed
- do not treat bootstrap install as best-effort “fire and forget”
- do not collapse SAP/workflow failures into generic success output
- do not claim install success unless deployed objects are actually present
- do not expand scope into unrelated tooling while reporter-facing reliability bug is still open

## Best Next Slice

The best immediate slice remains:

1. fix installer inputs (`Description`)
2. fix installer success criteria (`result.Success`)
3. add post-create package verification
4. add post-deploy object verification
5. only then re-test on `110`

After that, if lock issues remain reproducible on trial systems, inspect CSRF/session alignment and per-request stateful retry behavior.
