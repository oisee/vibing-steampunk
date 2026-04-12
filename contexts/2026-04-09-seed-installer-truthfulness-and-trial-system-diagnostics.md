# Seed: Next Agent Session

**Date:** 2026-04-09
**Wisdom file:** `contexts/2026-04-09-wisdom-installer-truthfulness-and-trial-system-diagnostics.md`

---

## Project Context

**vsp** is a Go MCP server + CLI for SAP ABAP Development Tools. Current urgent user-facing problem is installer reliability on trial/public systems: `vsp install zadt-vsp` can report success while deploying no objects. This session established that installer truthfulness is the top priority before any broader session/lock work.

## Read First

1. `CLAUDE.md` — project structure, build commands, conventions
2. `contexts/2026-04-09-wisdom-installer-truthfulness-and-trial-system-diagnostics.md` — latest conclusions and scope boundaries
3. `reports/2026-04-09-001-install-and-lock-bug-report.md` — concrete issue overview and fix direction

## Current State

```
Priority bug: installer false success on clean package
Secondary bug: edit/lock failures on some trial systems
Branch: main
SAP connector access verified: 105 and 110
Reporter-facing direction: fix installer first, keep lock work secondary
Claude worker governed: 7ln759eq:main
```

## What To Do Next

### Priority 1: Fix installer truthfulness

In:

- `cmd/vsp/devops.go`
- `internal/mcp/handlers_install.go`

Do exactly this:

1. pass `Description: obj.Description` into installer `WriteSource` calls
2. treat `!result.Success` as failure
3. print `result.Message` on failure
4. if package create fails, verify whether package exists before continuing
5. after deployment, verify required objects exist before reporting success

### Priority 2: Validate on SAP 110

After the fix:

1. run installer against a clean test package on `110`
2. verify expected objects exist
3. confirm summary is truthful for success and failure cases

### Priority 3: Revisit lock/session only if still reproducible

If edit failures remain after installer fix, inspect:

- `pkg/adt/http.go`
- per-request stateful writes vs CSRF fetch session mode
- retry behavior for stateful modifying requests

## Key Diagnosis To Preserve

The primary clean-install failure is not only “installer ignores `result.Success`”.

It is also that installer `WriteSource(..., Mode: upsert)` currently omits `Description`, while the create path requires it. That means a clean install can fail deterministically before any SAP-version nuance enters the picture.

## What To Avoid

- do not redesign `WriteSource` in this slice
- do not start with CSRF/session refactors
- do not broaden scope beyond installer and immediate verification
- do not report install success based only on lack of Go errors

## Useful Checks

```bash
./build/vsp -s a4h-110-adt install zadt-vsp --package '$ZVSP_TEST'
./build/vsp -s a4h-110-adt search "ZCL_VSP*" --type CLAS --max 20
./build/vsp -s a4h-110-adt search "ZIF_VSP*" --type INTF --max 20
./build/vsp -s a4h-110-adt search "ZADT_TEST*" --type PROG --max 20
```

If install partially fails, summary should say so explicitly and list missing objects.
