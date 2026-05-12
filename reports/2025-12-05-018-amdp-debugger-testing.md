# AMDP Debugger Testing Report

**Date:** 2025-12-05
**Report ID:** 018
**Subject:** AMDP Debugger API testing with ZCL_ADT_AMDP_TEST

---

## Summary

| Item | Status |
|------|--------|
| AMDP Class Created | ✅ ZCL_ADT_AMDP_TEST in $TMP |
| Unit Tests | ✅ Created (3 test methods) |
| AMDP Debugger API | ✅ Working (endpoint responds) |
| Debug Session Start | ❌ Blocked by existing lock |

---

## AMDP Test Class

Created `ZCL_ADT_AMDP_TEST` with:

```abap
CLASS zcl_adt_amdp_test DEFINITION
  PUBLIC FINAL CREATE PUBLIC.

  PUBLIC SECTION.
    INTERFACES if_amdp_marker_hdb.

    CLASS-METHODS calc_sum
      IMPORTING VALUE(iv_n) TYPE i
      EXPORTING VALUE(ev_sum) TYPE i.

    CLASS-METHODS get_sample_data
      IMPORTING VALUE(iv_count) TYPE i
      EXPORTING VALUE(et_result) TYPE tt_result.
ENDCLASS.
```

Methods use `BY DATABASE PROCEDURE FOR HDB LANGUAGE SQLSCRIPT`.

### Unit Tests

- `test_calc_sum` - Verify sum of 1..10 = 55
- `test_calc_sum_zero` - Edge case: sum of 1..0 = 0
- `test_get_sample_data` - Verify 3 rows returned

---

## AMDP Debugger API Findings

### Required Accept Header

```
application/vnd.sap.adt.amdp.dbg.startmain.v1+xml
```

Standard `application/xml` returns 406 Not Acceptable.

### API Endpoints

| Method | Endpoint | Purpose |
|--------|----------|---------|
| POST | `/sap/bc/adt/amdp/debugger/main?user={user}&cascadeMode={mode}` | Start session |
| GET | `/sap/bc/adt/amdp/debugger/main/{mainId}?timeout={sec}` | Resume/wait |
| DELETE | `/sap/bc/adt/amdp/debugger/main/{mainId}?hardStop=true` | Stop session |
| POST | `/sap/bc/adt/amdp/debugger/main/{mainId}?action={step}` | Step |

### Session Lock Issue

When starting a new AMDP debug session:

```xml
<exc:exception>
  <type id="AMDB_DBG_Failure"/>
  <properties>
    <entry key="TEXT">Debugging for user "TESTUSER" already in use</entry>
    <entry key="com.sap.adt.communicationFramework.subType">DEBUGGEE_CONTEXT_LOCKED_BY_ME</entry>
  </properties>
</exc:exception>
```

This indicates an existing AMDP debug lock at the HANA level. Resolution:
1. Use SAP GUI/Eclipse ADT to release the debug session
2. Wait for debug session timeout
3. Restart HANA debugger service (admin action)

---

## Code Fixes Applied

Updated `pkg/adt/amdp_debugger.go`:

```go
// Changed from:
Accept: "application/xml"

// To:
Accept: "application/vnd.sap.adt.amdp.dbg.startmain.v1+xml"
```

---

## Critical Finding: Session Binding

**AMDP debugging requires persistent HTTP session context.**

The debug session is bound to:
1. HTTP session (cookies: `SAP_SESSIONID_*`, `sap-usercontext`)
2. HANA session ID (`vhcala4hci:30203:300139`)

### Problem

Each MCP tool call creates a **new HTTP session**, so:
- `AMDPDebuggerStart` creates a session but loses the HTTP context
- Subsequent calls (`Resume`, `Stop`) can't access the session
- Result: "Debugging already in use" lock error

### Solutions

| Approach | Complexity | Notes |
|----------|------------|-------|
| **Cookie persistence** | Medium | Store cookies in vsp state across calls |
| **Single session mode** | Low | Keep one HTTP client alive for debug workflow |
| **Eclipse ADT** | None | Use Eclipse for AMDP debugging (recommended) |

### Workaround

For now, AMDP debugging via MCP is **read-only exploration** of the API.
Full debugging requires Eclipse ADT which maintains persistent sessions.

---

## Recommendations

1. **Document Session Limitation**: AMDP debugging needs persistent HTTP session

2. **Implement Cookie Persistence**: Store/restore session cookies for debug workflows

3. **Add Lock Detection**: Check for `DEBUGGEE_CONTEXT_LOCKED_BY_ME` error

4. **Consider Stateful Mode**: Add `--stateful` flag to maintain single HTTP session

---

## Files Created/Modified

- `ZCL_ADT_AMDP_TEST` - AMDP test class in $TMP package
- `pkg/adt/amdp_debugger.go` - Fixed Accept header
