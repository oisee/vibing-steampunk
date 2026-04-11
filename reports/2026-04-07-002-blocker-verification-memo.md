# Blocker Verification Memo — Issues #88 and #90

**Date:** 2026-04-07
**Report ID:** 2026-04-07-002
**Subject:** Root-cause analysis of two adoption blockers

---

## Issue #90: BTP Authentication Failure

**Status:** Reproducible from current main. Locally actionable.

### Root Cause

Go's `http.Client` strips `Authorization` header on ALL redirects (RFC 7235 §4.2). BTP authentication flow uses redirects. No custom `CheckRedirect` exists in the codebase.

**File:** `pkg/adt/config.go:216-232` — `NewHTTPClient()` creates client without `CheckRedirect`.

### Why curl Works

curl preserves Authorization headers across redirects by default (or with `--location-trusted`). Go does not.

### Fix

Add `CheckRedirect` to `NewHTTPClient()`:

```go
client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
    if len(via) >= 10 {
        return http.ErrUseLastResponse
    }
    if len(via) > 0 {
        if auth := via[0].Header.Get("Authorization"); auth != "" {
            req.Header.Set("Authorization", auth)
        }
    }
    return nil
}
```

**Effort:** ~10 lines. Low risk — preserves auth only from the original request.

**Verification needed:** BTP instance access to confirm redirect flow.

---

## Issue #88: Lock Handle Invalid

**Status:** Likely reproducible on SAP_ABA 7.51 with stateless default. Root cause identified.

### Root Cause

The default `SessionStateless` (changed in commit d84db03 on 2026-04-04) sends `X-sap-adt-sessiontype: stateless` header. Lock handles are session-specific server-side references. The sequence:

1. LockObject (POST) → SAP creates lock bound to session A → returns lock handle
2. UpdateSource (PUT with lockHandle) → header says "stateless" → SAP treats as session B
3. Lock handle from session A not found in session B → `ExceptionResourceInvalidLockHandle`

**The paradox:** Go's CookieJar sends `sap-contextid` cookies automatically, but the `stateless` header overrides and forces SAP to treat each request as independent.

**File:** `pkg/adt/http.go:385-391` sets the header. `pkg/adt/config.go:186` defaults to stateless.

### Fix Options

**Option A (Recommended): Selective stateful for edit operations**

In `workflows_edit.go`, switch to stateful mode for the lock→write→unlock sequence:

```go
// Before lock
c.transport.SetSessionType(SessionStateful)
defer c.transport.SetSessionType(originalType)

// Lock → Write → Unlock (all in same stateful session)
```

This preserves the stateless default for read operations (fixing #81) while ensuring write operations maintain session affinity.

**Option B: Detect and retry**

Add `ExceptionResourceInvalidLockHandle` to `IsSessionExpired()` in `http.go:456-467`, then retry with stateful session.

**Option C: Per-request session type**

Add `SessionType` field to `RequestOptions` so individual requests can override the default without mutating transport config.

**Effort:** Option A is ~20 lines. Option C is cleaner but requires touching more code.

---

## Decision

**Issue #90 is the narrower, safer fix.** ~10 lines, clear root cause, low regression risk. Should fix first.

**Issue #88 requires Option A or C.** More nuanced — needs to not regress #81 (the reason stateless was made default). Option C (per-request session type) is architecturally cleanest.

### Recommendation

1. Fix #90 now (CheckRedirect, 30 minutes)
2. Fix #88 next (selective stateful for edit ops, 2-3 hours)
3. Both fixes are small enough to merge same day

---

## Verification Without SAP Access

- #90: Can verify redirect handling with a test HTTP server that redirects. Cannot verify BTP-specific flow without BTP instance.
- #88: Can verify stateful header is sent during edit operations via unit test. Cannot reproduce the exact `ExceptionResourceInvalidLockHandle` without affected SAP system.
