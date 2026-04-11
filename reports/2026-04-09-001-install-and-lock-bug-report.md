# Bug Report: Install Silent Failure + Lock Errors on Trial Systems

**Date:** 2026-04-09
**Reporter:** Marcello Urbani (SAP 2023 Trial, muabap.ydns.eu)
**Severity:** HIGH — installer lies about success

---

## Reported Symptoms

1. **`vsp install zadt-vsp` says it works but only creates the package** — no objects inside
2. **Editing objects reliably fails** — "not locked" / lock instance errors

## Bug 1: Install Silent Failure

### Root Cause

The install handler in `cmd/vsp/devops.go:2119-2131` only checks Go errors (`err != nil`), NOT the SAP operation result (`result.Success`):

```go
_, err := client.WriteSource(ctx, obj.Type, obj.Name, obj.Source, opts)
if err != nil {
    fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
    failed++
} else {
    fmt.Fprintf(os.Stderr, "OK\n")  // ← LOGS OK EVEN IF result.Success = false!
    deployed++
}
```

`WriteSource` returns `(result, nil)` — nil error — in these failure scenarios:
- **Syntax errors**: source has errors → `result.Success = false`, `result.Message = "Source has syntax errors"`, but `err == nil`
- **Activation failure**: object won't activate → `result.Success = false`, `result.Activation.Success = false`, but `err == nil`
- **Object exists conflict**: mode mismatch → `result.Message = "Object already exists"`, but `err == nil`
- **Lock failure inside WriteSource**: lock fails → returns result with error message, but `err == nil` in some paths

**On a 2023 trial system**, the embedded ABAP source may have syntax issues due to missing dependencies or SAP version differences. Each `WriteSource` call returns a result with `Success=false` but the handler prints "OK" and increments `deployed`.

### Fix Required

```go
result, err := client.WriteSource(ctx, obj.Type, obj.Name, obj.Source, opts)
if err != nil {
    // Go-level error (network, auth, etc.)
    fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
    failed++
} else if !result.Success {
    // SAP-level error (syntax, activation, lock, etc.)
    fmt.Fprintf(os.Stderr, "FAILED: %s\n", result.Message)
    failed++
} else {
    fmt.Fprintf(os.Stderr, "OK\n")
    deployed++
}
```

Same fix needed in MCP handler (`handlers_install.go:418-441`).

### Additional Issue: Package Creation Continues on Error

`handlers_install.go:398-403` logs a warning when package creation fails but continues. On a trial system where `/sap/bc/adt/packages` API may not exist, the package is never created, and all subsequent `WriteSource` calls fail silently because they try to assign objects to a non-existent package.

**Fix:** After package creation failure, verify the package exists via `GetPackage()` before continuing. If it doesn't exist, abort with a clear message: "Package creation failed and package doesn't exist. Create it manually via SE80/SE21."

---

## Bug 2: Lock/Edit Failures on Trial Systems

### Root Cause

The stateful session fix (commit 27f4d7c) added `Stateful: true` to `LockObject`, `UnlockObject`, and `UpdateSource` in `pkg/adt/crud.go`. This fix is correct and should work on trial systems.

### Why Marcello May Still See Lock Errors

Possible causes on SAP 2023 trial:

1. **Running old vsp binary** — the fix is in v2.38.x, Marcello may have an older version
2. **Cookie authentication issues** — trial systems may require specific cookie handling
3. **CSRF token bound to wrong session** — CSRF fetch uses stateless, but lock uses stateful. If the CSRF token is session-bound on strict systems, it may be rejected in the stateful session
4. **SAP_ABA 7.51 stricter session enforcement** — even with stateful header, the trial system may enforce additional session requirements

### Verification Steps

1. Check which vsp version Marcello is running
2. Try `vsp -s trial source CLAS ZCL_SOMETHING` (read) — does basic auth work?
3. Try `vsp -s trial source edit CLAS ZCL_TEST --old "X" --new "Y"` — does edit work?
4. Check verbose output: `vsp -s trial -v source edit ...` to see session headers

### Potential Fix: CSRF Token in Stateful Mode

In `pkg/adt/http.go:295-298`, CSRF token fetch only uses stateful if the GLOBAL config is stateful:

```go
if t.config.SessionType == SessionStateful {
    req.Header.Set("X-sap-adt-sessiontype", "stateful")
}
```

But the global default is stateless. If a trial system requires the CSRF token to be in the same session as the lock, we need to fetch CSRF in stateful mode when the subsequent operation will be stateful.

---

## Recommendations

### Immediate (fix for next release)

1. **Fix install handler** — check `result.Success`, not just `err != nil`
2. **Fix package creation** — verify package exists after creation attempt
3. **Print result.Message on failure** — users need to see what SAP actually said
4. **Add `--verbose` to install** — show SAP responses for each step

### Short-term

5. **Add install smoke test** — after deployment, verify each object exists via `SearchObject`
6. **Recommend abapGit path** — for trial systems, `vsp install abapgit` + import via abapGit may be more reliable than direct object creation
7. **Test on a4h-110-adt** — reproduce the install flow and check result.Success values

### Medium-term

8. **CSRF+stateful alignment** — ensure CSRF token is fetched in stateful mode when lock operations follow
9. **Session keep-alive for install** — install creates 9 objects sequentially; session may timeout between objects on slow systems

---

## Test Plan

```bash
# 1. Test install on a4h-110-adt
vsp -s a4h-110-adt install zadt-vsp --package '$ZTEST_INSTALL'

# 2. Verify objects exist
vsp -s a4h-110-adt search "ZCL_VSP*" --type CLAS --max 10
vsp -s a4h-110-adt search "ZIF_VSP*" --type INTF --max 10

# 3. Test edit flow
vsp -s a4h-110-adt source CLAS ZCL_HIRT_API  # read (should work)
vsp -s a4h-110-adt source edit CLAS ZCL_HIRT_API --old "PUBLIC" --new "PUBLIC"  # no-op edit

# 4. Clean up test package
# (manual via SE80 or leave for next test)
```

---

## Files to Modify

| File | Change |
|------|--------|
| `cmd/vsp/devops.go:2119-2131` | Check `result.Success` in install loop |
| `internal/mcp/handlers_install.go:418-441` | Same fix for MCP handler |
| `cmd/vsp/devops.go:2088-2102` | Verify package exists after creation |
| `internal/mcp/handlers_install.go:396-403` | Same package verification |
| `pkg/adt/http.go:295-298` | Consider stateful CSRF for lock sequences |
