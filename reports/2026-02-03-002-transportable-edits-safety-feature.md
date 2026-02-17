# Transportable Edits Safety Feature

**Date:** 2026-02-03
**Report ID:** 002
**Subject:** Safety feature to control editing of transportable objects
**Tags:** `safety`, `transports`, `implementation`

---

## Executive Summary

Implement a safety feature that by default blocks editing objects in transportable packages. When enabled via `--allow-transportable-edits`, expose transport-related tools to support the workflow.

## Problem Statement

Currently, vsp allows passing a `transport` parameter to `EditSource`/`WriteSource`. This can lead to:
1. Accidental modifications to production-transportable code
2. Users not understanding why edits fail for non-local objects
3. No clear guidance on how to work with transportable packages

## Proposed Solution

### Behavior Matrix

| Flag State | Transport Param | Result |
|------------|-----------------|--------|
| OFF (default) | Not provided | Works (local objects only) |
| OFF (default) | Provided | **BLOCKED** with helpful error |
| ON | Not provided | Works (local objects only) |
| ON | Provided | **ALLOWED** + transport tools visible |

### Error Message (when blocked)

```
operation 'EditSource' with transport 'A4HK900114' is blocked: editing transportable objects is disabled.
Objects in transportable packages require explicit opt-in.
Use --allow-transportable-edits or SAP_ALLOW_TRANSPORTABLE_EDITS=true to enable.
WARNING: This allows modifications to non-local objects that may affect production systems.
```

### Tool Visibility

When `--allow-transportable-edits` is TRUE:
- Expose `ListTransports`, `GetTransport` tools (read-only transport info)
- Allow `transport` parameter in `EditSource`, `WriteSource`

When `--allow-transportable-edits` is FALSE:
- Hide transport-related tools
- Block any operation with `transport` parameter

### Interaction with Other Flags

| Flag | Purpose | Interaction |
|------|---------|-------------|
| `--enable-transports` | Transport MANAGEMENT (create/release/delete) | Independent |
| `--allow-transportable-edits` | Allow USING transports in edits | New flag |
| `--allowed-transports` | Whitelist specific transports | Works with both |
| `--allowed-packages` | Whitelist packages for operations | Independent |

## Implementation Status

### Completed

- [x] Added `AllowTransportableEdits` to `SafetyConfig` struct
- [x] Added `CheckTransportableEdit()` method with clear error message
- [x] Added `isTransportInWhitelist()` helper
- [x] Added `WithAllowTransportableEdits()` config option
- [x] Added `checkTransportableEdit()` client helper
- [x] Added check in `EditSourceWithOptions()`
- [x] Added check in `WriteSource()`
- [x] Added `--allow-transportable-edits` CLI flag
- [x] Added `SAP_ALLOW_TRANSPORTABLE_EDITS` env var support
- [x] Added verbose output when flag is enabled
- [x] Added unit tests (7 test cases)
- [x] Updated CLAUDE.md documentation

### TODO

- [x] **Implement tool visibility logic:**
  - `ListTransports`/`GetTransport` work if: `--enable-transports` OR `--allow-transportable-edits`
  - `CreateTransport`/`ReleaseTransport`/`DeleteTransport` work only if: `--enable-transports`
- [ ] Test with vsp: flag OFF + transport param (should block)
- [ ] Test with vsp: flag ON + transport param (should work)
- [ ] Test with vsp: flag ON + whitelist (should filter)
- [ ] Test with vsp: ListTransports with --allow-transportable-edits
- [ ] Update README.md with new flag
- [ ] Update GitHub issues #17, #18 with final status
- [ ] Clean up test objects in SAP

## Files Changed

| File | Changes |
|------|---------|
| `pkg/adt/safety.go` | +50 lines: new field, methods |
| `pkg/adt/safety_test.go` | +60 lines: 7 test cases |
| `pkg/adt/config.go` | +10 lines: config option |
| `pkg/adt/client.go` | +5 lines: helper method |
| `pkg/adt/workflows.go` | +8 lines: checks in EditSource, WriteSource |
| `cmd/vsp/main.go` | +10 lines: flag, env var, verbose |
| `internal/mcp/server.go` | +4 lines: config field |
| `CLAUDE.md` | +1 line: documentation |

## Test Objects Created in SAP

| Object | Type | Package | Transport | Status |
|--------|------|---------|-----------|--------|
| ZPROD | Package | - | - | Created |
| ZPROD_001_TEST | Program | ZPROD | A4HK900114 | Modified |
| ZPROD_002_TEST | Program | ZPROD | A4HK900114 | Created |
| ZCL_PROD_TEST | Class | ZPROD | A4HK900114 | Created |

## Test Plan

See: `reports/2026-02-03-002-test-checklist.md`

## Configuration Examples

```bash
# Default: Only local packages ($TMP, $*)
./vsp --url ... --user ... --password ...

# Allow transportable edits
./vsp --url ... --allow-transportable-edits

# Allow transportable edits with transport whitelist
./vsp --url ... --allow-transportable-edits --allowed-transports "A4HK*,DEVK*"

# Full transport management (create, release, delete)
./vsp --url ... --allow-transportable-edits --enable-transports

# Read-only transport info
./vsp --url ... --allow-transportable-edits --enable-transports --transport-read-only
```

## References

- GitHub Issue #17: EditSource transport parameter
- GitHub Issue #18: Namespaced objects (also fixed in this session)
- Report 2026-02-03-001: abapGit dependencies (related roadmap item)
