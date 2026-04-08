# PM6 WebSocket APC Investigation — ECC Compatibility

**Date:** 2026-02-06
**Report ID:** 001
**Subject:** ZADT_VSP APC handler on PM6 (ECC) — 500 on WebSocket handshake
**Systems:** PM6 (ECC, `pm6.microsoftintegrationdemo.com:8001`, client 800), PM4 (S/4HANA, resolved)
**Related:** MEMORY.md (WebSocket Auth notes), 2025-12-05-024 (AMDP Goroutine Architecture)

> **Note:** PM4 (`pm4.microsoftintegrationdemo.com:8001`, S/4HANA, client 400) WebSocket issue was **already resolved** in this session — the fix was the Basic Auth → session cookie fallback (commit `4dcce5c`). PM4 rejected standalone Basic Auth on WebSocket upgrade but accepted session cookies. PM6 (ECC) is a different problem — it returns 500 even with the auth fallback.

---

## Executive Summary

The ZADT_VSP APC WebSocket handler was deployed to PM6 (an ECC system). After fixing `FIND PCRE` → `FIND REGEX` in the service classes, the SICF node returns **501** on plain HTTP (correct), but the actual WebSocket upgrade handshake still fails with **500**. The root cause is likely the APC class instantiation phase — when `CL_APC_MANAGER` actually creates `ZCL_VSP_APC_HANDLER` and its dependencies during the WebSocket upgrade, something still fails silently (no ST22 dump).

---

## What We Tested

### 1. Basic ADT Connectivity — PASS
```
GET /sap/bc/adt/discovery?sap-client=800 → 200 OK
```
PM6 is reachable, credentials work, ADT is functional.

### 2. APC SICF Node (Plain HTTP) — PASS (501)
```
GET /sap/bc/apc/sap/zadt_vsp?sap-client=800 → 501 Not Implemented
```
**501** is the correct response. It means:
- The SICF node `/sap/bc/apc/sap/zadt_vsp` is active
- `CL_APC_MANAGER` can process the request
- It rejects plain HTTP because APC expects a WebSocket `Upgrade` header
- Previously this returned **500** due to PCRE syntax errors in handler classes

### 3. WebSocket Upgrade Handshake — FAIL (500)
```go
ws := adt.NewDebugWebSocketClient("http://pm6...", "800", ...)
err := ws.Connect(ctx)
// → "WebSocket connection failed (HTTP 500): websocket: bad handshake"
```
When doing an actual WebSocket upgrade (`Connection: Upgrade`, `Upgrade: websocket`), CL_APC_MANAGER returns **500**. This happens at a different code path than the plain HTTP 501 — the APC manager actually tries to instantiate the handler and its dependency tree.

### 4. Source Code Review — PCRE Fix Confirmed

All four classes were inspected on PM6 via `GetSource`:

| Class | PCRE Usage | Status |
|-------|-----------|--------|
| `ZCL_VSP_UTILS` | `FIND REGEX` everywhere, but `\d+` used in `extract_param_int` | Fixed? See below |
| `ZCL_VSP_APC_HANDLER` | `FIND regex` with `\s*`, `\d+` patterns in `parse_message` | Fixed? See below |
| `ZCL_VSP_RFC_SERVICE` | `FIND REGEX` everywhere, no PCRE shortcuts | Clean |
| `ZCL_VSP_REPORT_SERVICE` | `FIND REGEX` everywhere, `\s*` used in `extract_param` | Fixed? See below |

**Key concern:** While `FIND PCRE` → `FIND REGEX` was done, several patterns still use PCRE-style shortcuts:
- `\d+` (PCRE digit class — may not work in POSIX ERE)
- `\s*` (PCRE whitespace class — may not work in POSIX ERE)

On ECC systems, `FIND REGEX` uses the **POSIX Extended Regular Expression** engine, which may not support `\d` and `\s`. These should be replaced with:
- `\d` → `[0-9]`
- `\s` → `[ ]` or `[[:space:]]`

However, this may NOT be the cause of the 500 — these would cause runtime failures, not load-time failures. The 500 during WebSocket upgrade suggests a class loading issue.

### 5. Dependency Analysis

The `class_constructor` of `ZCL_VSP_APC_HANDLER` creates:
```abap
APPEND NEW zcl_vsp_rfc_service( ) TO gt_services.
APPEND NEW zcl_vsp_report_service( ) TO gt_services.
```

Note: `zcl_vsp_debug_service`, `zcl_vsp_amdp_service`, and `zcl_vsp_git_service` are commented out (good — avoids abapGit dependency crash).

**Transitive dependencies of the instantiated classes:**

| Class | Dependencies |
|-------|-------------|
| `ZCL_VSP_RFC_SERVICE` | `ZIF_VSP_SERVICE`, `ZCL_VSP_TADIR_MOVE` (in `handle_move_to_package`) |
| `ZCL_VSP_REPORT_SERVICE` | `ZIF_VSP_SERVICE` |
| `ZCL_VSP_UTILS` | None (static methods only, called at runtime) |

**Possible failure point:** `ZCL_VSP_TADIR_MOVE` is referenced in `ZCL_VSP_RFC_SERVICE`. If this class doesn't exist or has syntax errors on PM6, it could cause the class_constructor to fail during instantiation.

### 6. SAP Standard APC Reference — Ping Pong Game

We investigated the standard SAP APC ping/pong demo on A4H:

**Architecture:**
- `/sap/bc/apc_test/ping_pong/game` → `CL_HTTP_EXT_PING_PONG_PAGE` (HTTP handler, serves HTML/JS game canvas)
- `/sap/bc/apc_test/ping_pong/player` → `CL_HTTP_EXT_PING_PONG_PLAYER` (HTTP handler, serves player controller HTML)
- `/sap/bc/apc/sap/ping_pong` → APC WebSocket endpoint (application ID: `PING_PONG`, type `SAPC`)

**Key observations from the standard implementation:**
1. The player page generates the WebSocket URL using `cl_apc_ws_utility=>get_access_url()` — this is the proper SAP API for APC URL generation
2. The player page **includes an XSRF token** in the WebSocket URL: `&sap-xsrf={token}` — this may be required for some systems
3. The protocol uses simple text messages (`PLAYER1.UP:USERNAME`, `PLAYER2.REGISTER:USERNAME`) — not JSON
4. The APC application is registered as type `SAPC` in the system

**This raises an important question:** Does PM6's APC manager require an XSRF token in the WebSocket URL? Our vsp client doesn't send one during the upgrade handshake.

### 7. Standard APC on PM6 — PASS (Confirmed by User)

The standard ping/pong APC **works on PM6 (ECC)**. This is a critical finding:
- The APC framework itself is functional on this ECC system
- WebSocket upgrade works for standard SAP handler classes
- The 500 is **specific to our ZADT_VSP handler**, not a platform issue

**APC handler patterns compared:**

| | Standard Ping/Pong | Our ZADT_VSP |
|---|---|---|
| **Base class** | `CL_APC_WS_EXT_STATELESS_BASE` | `CL_APC_WSP_EXT_STATEFUL_BASE` |
| **Interface** | `IF_APC_WS_EXTENSION` | `IF_APC_WSP_EXTENSION` |
| **class_constructor** | None | Creates service instances |
| **Dependencies** | AMC channel manager only | `ZCL_VSP_UTILS`, `ZCL_VSP_RFC_SERVICE`, `ZCL_VSP_REPORT_SERVICE`, `ZIF_VSP_SERVICE`, `ZCL_VSP_TADIR_MOVE` |
| **Complexity** | ~20 lines | ~500+ lines across classes |

The standard handler is dead simple — receive text, broadcast via AMC. No class_constructor, no dependencies. Our handler has a class_constructor that instantiates service objects, pulling in a transitive dependency tree that can fail.

---

## Root Cause Hypotheses (Updated)

### Hypothesis A: Missing/Broken Dependency Class (Most Likely)
`ZCL_VSP_TADIR_MOVE` is referenced by `ZCL_VSP_RFC_SERVICE`. If it doesn't exist or has syntax errors on PM6, the `class_constructor` would fail when creating `NEW zcl_vsp_rfc_service()`. Since standard APC works fine, this is a handler-specific loading issue.

**Test:** Check if `ZCL_VSP_TADIR_MOVE` exists and compiles on PM6.

### Hypothesis B: POSIX Regex Incompatibility
`\d` and `\s` in `FIND REGEX` patterns may not be valid on ECC's POSIX ERE engine. While this would normally be a runtime error, the ABAP compiler might flag it during class loading on some ECC kernels.

**Test:** Run `SyntaxCheck` on each class on PM6.

### Hypothesis C: APC Application Registration Issue
The ZADT_VSP APC application may not be properly registered (SAPC object type). The standard ping/pong has a proper SAPC registration. If ZADT_VSP was set up manually in SICF without a proper SAPC application definition, CL_APC_MANAGER might fail during handler resolution.

**Test:** Search for `ZADT_VSP` as SAPC type on PM6.

### Hypothesis D: Missing XSRF Token (Less Likely Now)
The standard ping/pong player sends `sap-xsrf={token}` in the WebSocket URL. However, since the standard APC works on PM6 and our issue is a 500 (server error, not 403 forbidden), this is likely not the cause. XSRF would cause an auth rejection, not a handler crash.

**Test:** Pre-fetch an XSRF token via `GET /sap/bc/adt/discovery` with `X-CSRF-Token: Fetch` header, then include it in the WebSocket URL.

---

## Plan: ECC WebSocket Support

### Phase 1: Diagnose the 500 (Immediate)

1. **Check ZCL_VSP_TADIR_MOVE existence** on PM6
   ```
   SearchObject("ZCL_VSP_TADIR_MOVE*")
   ```

2. **Syntax check all handler classes** on PM6
   ```
   SyntaxCheck(ZCL_VSP_APC_HANDLER)
   SyntaxCheck(ZCL_VSP_RFC_SERVICE)
   SyntaxCheck(ZCL_VSP_REPORT_SERVICE)
   SyntaxCheck(ZCL_VSP_UTILS)
   ```

3. **Test standard APC on PM6** — Does `/sap/bc/apc/sap/ping_pong` return 501? If so, try a real WebSocket upgrade to it. If the standard one also fails, APC itself may not be properly configured on PM6.

4. **Try XSRF token in WebSocket URL** — Pre-authenticate, fetch token, add to URL

### Phase 2: Fix POSIX Regex Patterns (Quick Win)

Replace all PCRE shortcuts in the 4 classes for ECC compatibility:

| Pattern | Replace With |
|---------|-------------|
| `\d+` | `[0-9]+` |
| `\d` | `[0-9]` |
| `\s*` | `[ ]*` or remove (our JSON is compact) |
| `\s+` | `[ ]+` |

Files affected: `ZCL_VSP_UTILS`, `ZCL_VSP_APC_HANDLER`, `ZCL_VSP_REPORT_SERVICE`

### Phase 3: XSRF Token Support in Go Client (If Needed)

If Hypothesis A is confirmed, update `BaseWebSocketClient.Connect()`:

```go
// Before WebSocket dial, fetch XSRF token
tokenReq, _ := http.NewRequest("GET", discoveryURL, nil)
tokenReq.Header.Set("X-CSRF-Token", "Fetch")
tokenResp, _ := httpClient.Do(tokenReq)
xsrfToken := tokenResp.Header.Get("X-CSRF-Token")

// Include in WebSocket URL
wsURL = fmt.Sprintf("%s&sap-xsrf=%s", wsURL, xsrfToken)
```

### Phase 4: Minimal APC Handler for ECC (Fallback)

If the full ZADT_VSP handler proves too complex for ECC, create a minimal version:

1. **Strip dependencies:** Remove `ZCL_VSP_RFC_SERVICE` from class_constructor (avoid `ZCL_VSP_TADIR_MOVE` dependency)
2. **Inline JSON parsing:** Remove `ZCL_VSP_UTILS` dependency, use simple string operations
3. **Test with ping-only handler:** Create a bare-bones handler that just responds to ping/pong to validate the APC framework works

### Phase 5: ECC vs S/4HANA Feature Matrix

Document what works where:

| Feature | S/4HANA (A4H) | S/4HANA (PM4) | ECC (PM6) |
|---------|---------------|---------------|-----------|
| ADT REST API | Yes | Yes | Yes |
| APC WebSocket (standard) | Yes | ? | **Yes** (ping/pong confirmed working) |
| APC WebSocket (ZADT_VSP) | Yes | Yes (cookie auth fallback) | 500 (investigating) |
| FIND REGEX | Full PCRE | Full PCRE | POSIX ERE |
| FIND PCRE | Yes | Yes | No |
| abapGit | Yes | No | ? |
| Auth method for WS | Basic Auth | Session cookies (fallback) | ? (500 before auth matters) |

---

## Key Takeaway

APC works on PM6 (ECC) — the standard ping/pong game proves it. Our ZADT_VSP handler is the problem.

The 501 → 500 distinction is critical:
- **Plain HTTP → 501:** SICF node works, CL_APC_MANAGER loads, but no WebSocket upgrade → correct rejection
- **WebSocket Upgrade → 500:** CL_APC_MANAGER fails during actual handler instantiation

The most likely culprit is a **missing dependency class** (`ZCL_VSP_TADIR_MOVE`) causing `class_constructor` to fail when instantiating `ZCL_VSP_RFC_SERVICE`. This is easy to test and fix.

---

## TODO Checklist (Next Session)

- [ ] **Check `ZCL_VSP_TADIR_MOVE`** — does it exist on PM6? `SearchObject("ZCL_VSP_TADIR_MOVE*")`
- [ ] **Syntax check all 4 classes** — `SyntaxCheck` on `ZCL_VSP_APC_HANDLER`, `ZCL_VSP_RFC_SERVICE`, `ZCL_VSP_REPORT_SERVICE`, `ZCL_VSP_UTILS`
- [ ] **Check SAPC registration** — `SearchObject("ZADT_VSP")` — is it registered as SAPC type?
- [ ] **Fix `\d`/`\s` patterns** — replace with `[0-9]`/`[ ]` in all 4 classes for POSIX ERE compatibility
- [ ] **If dependency missing** — either install `ZCL_VSP_TADIR_MOVE` on PM6 or remove it from `class_constructor` service list
- [ ] **Test WebSocket again** after fixes — full round-trip connect → welcome → ping/pong
- [ ] **Try minimal handler** as fallback — bare-bones ping-only handler to isolate the issue
- [ ] **Test XSRF token** if above fixes don't resolve — add `sap-xsrf` to WebSocket URL in Go client
