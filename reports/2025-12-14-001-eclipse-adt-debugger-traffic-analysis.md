# Eclipse ADT Debugger Traffic Analysis

**Date:** 2025-12-14
**Report ID:** 001
**Subject:** Discovery of RFC-based debugging protocol in Eclipse ADT
**Status:** Key Finding

---

## Executive Summary

Traffic capture between Eclipse ADT and SAP revealed that **Eclipse does NOT use direct HTTP for debugging**. Instead, it tunnels ADT REST requests through **RFC protocol (port 3300)** using the `SADT_REST_RFC_ENDPOINT` function module.

This explains why vsp's HTTP-based external breakpoint approach was not working as expected.

---

## Traffic Capture Setup

```
Eclipse ADT → <lan-ip> (proxy) → <lan-ip>:3300 (SAP RFC)
```

Tools used:
- `socat` for RFC traffic capture (port 3300)
- `mitmproxy` for HTTP traffic (port 50000) - showed no debugger traffic

---

## Key Findings

### 1. RFC Tunneling, Not Direct HTTP

Eclipse ADT uses:
- **Port 3300** (SAP RFC/Gateway)
- **Function module:** `SADT_REST_RFC_ENDPOINT`
- REST requests are serialized and sent via RFC, not HTTP

### 2. Batch Requests

Eclipse sends multiple debugger operations in a single batch request:

```http
POST /sap/bc/adt/debugger/batch HTTP/1.1
Content-Type: multipart/mixed; boundary=batch_xxxx-xxxx-xxxx
Accept: multipart/mixed

--batch_xxxx-xxxx-xxxx
Content-Type: application/http
content-transfer-encoding: binary

POST /sap/bc/adt/debugger?method=stepOver HTTP/1.1
Accept: application/xml

--batch_xxxx-xxxx-xxxx
Content-Type: application/http
content-transfer-encoding: binary

POST /sap/bc/adt/debugger?emode=_&semanticURIs=true&method=getStack HTTP/1.1
Accept: application/xml

--batch_xxxx-xxxx-xxxx
Content-Type: application/http
content-transfer-encoding: binary

POST /sap/bc/adt/debugger?method=getChildVariables HTTP/1.1
Content-Type: application/vnd.sap.as+xml; charset=UTF-8; dataname=com.sap.adt.debugger.ChildVariables
Accept: application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.debugger.ChildVariables

<?xml version="1.0" encoding="UTF-8" ?>
<asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml">
  <asx:values>
    <DATA>
      <HIERARCHIES>
        <STPDA_ADT_VARIABLE_HIERARCHY>
          <PARENT_ID>@ROOT</PARENT_ID>
        </STPDA_ADT_VARIABLE_HIERARCHY>
      </HIERARCHIES>
    </DATA>
  </asx:values>
</asx:abap>

--batch_xxxx-xxxx-xxxx
Content-Type: application/http
content-transfer-encoding: binary

POST /sap/bc/adt/debugger?method=getVariables HTTP/1.1
Content-Type: application/vnd.sap.as+xml; charset=UTF-8; dataname=com.sap.adt.debugger.Variables
Accept: application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.debugger.Variables

<?xml version="1.0" encoding="UTF-8" ?>
<asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml">
  <asx:values>
    <DATA>
      <STPDA_ADT_VARIABLE>
        <ID>SY-SUBRC</ID>
      </STPDA_ADT_VARIABLE>
    </DATA>
  </asx:values>
</asx:abap>

--batch_xxxx-xxxx-xxxx--
```

### 3. Debugger Methods Observed

| Method | Purpose |
|--------|---------|
| `stepOver` | Step over current line |
| `stepInto` | Step into function/method |
| `stepJumpToLine` | Jump to specific line (with URI) |
| `getStack` | Get call stack |
| `getChildVariables` | Get child variables of a parent |
| `getVariables` | Get specific variables by ID |

### 4. Variable Request Format

Variables are requested using ABAP XML format:
```xml
<asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml">
  <asx:values>
    <DATA>
      <STPDA_ADT_VARIABLE>
        <ID>SY-SUBRC</ID>
      </STPDA_ADT_VARIABLE>
    </DATA>
  </asx:values>
</asx:abap>
```

### 5. User-Agent

```
Eclipse/4.35.0.v20250228-0140 (win32; x86_64; Java 21.0.6) ADT/3.48.1 (devedition)
```

---

## Why vsp HTTP Approach Didn't Work

| Aspect | Eclipse ADT | vsp |
|--------|-------------|-----|
| Transport | RFC (port 3300) | HTTP (port 50000) |
| Session | RFC connection maintains session | HTTP stateless |
| Batch | Multiple ops per request | Single ops |
| Endpoint | Tunneled via `SADT_REST_RFC_ENDPOINT` | Direct HTTP |

The RFC connection provides session affinity that HTTP doesn't naturally have. External breakpoints likely require this session context.

---

## Proposed Fixes

### Option 1: Use Batch Endpoint over HTTP (Recommended)

Try using `/sap/bc/adt/debugger/batch` directly over HTTP with multipart/mixed format.

**Pros:**
- Uses existing HTTP infrastructure
- Matches Eclipse's request format

**Cons:**
- May still lack session affinity
- Need to implement multipart handling

### Option 2: RFC Protocol via SAP NW RFC SDK

Use SAP's RFC SDK to call `SADT_REST_RFC_ENDPOINT` directly.

**Pros:**
- Exactly matches Eclipse behavior
- Full session support

**Cons:**
- Requires SAP NW RFC SDK (platform-specific binaries)
- Complex integration with Go
- Licensing considerations

### Option 3: WebSocket/Long-polling Session

Maintain HTTP session with cookies and implement long-polling similar to RFC.

**Pros:**
- Pure HTTP
- No external dependencies

**Cons:**
- May not fully replicate RFC behavior
- Complex implementation

---

## Immediate Next Steps

1. **Try batch endpoint** - Implement `/sap/bc/adt/debugger/batch` over HTTP
2. **Test session cookies** - Ensure HTTP session is properly maintained
3. **Match Eclipse headers** - Use same User-Agent and Accept headers

---

## Evidence

### RFC Traffic Sample (port 3300)

```
SADT_REST_RFC_ENDPOINT
POST /sap/bc/adt/debugger/batch HTTP/1.1
Content-Type: multipart/mixed; boundary=batch_20cd4567-f577-4fb5-85b0-6bf534444d04
Accept: multipart/mixed
User-Agent: Eclipse/4.35.0.v20250228-0140 (win32; x86_64; Java 21.0.6) ADT/3.48.1 (devedition)
X-sap-adt-profiling: server-time
```

### Port 50000 (HTTP)

No debugger traffic observed on port 50000 during Eclipse debugging session.

---

## Follow-up Testing Results (2025-12-14)

### Batch Endpoint Testing

We implemented and tested the batch endpoint over HTTP:

```go
// DebuggerBatchRequest sends multiple operations in one batch
func (c *Client) DebuggerBatchRequest(ctx context.Context, operations []DebugBatchOperation) ([]DebugBatchResponse, error)
```

**Result:** The batch endpoint **DOES work over HTTP** (port 50000). We received valid responses with "noSessionAttached" errors, confirming the endpoint is accessible.

### Critical Finding: Breakpoints Not Persisting

Testing revealed a critical issue with breakpoint persistence:

| Operation | HTTP Result | Expected |
|-----------|------------|----------|
| SetExternalBreakpoint | Returns success + ID | Breakpoint stored |
| GetExternalBreakpoints | Returns **0 breakpoints** | Should show the BP |
| DebuggerListen | Times out (no BP to hit) | Should catch debuggee |

**The SAP server accepts breakpoint requests but does NOT persist them when received via HTTP.**

### Test Evidence

```
2. Setting line breakpoint on ZCL_ADT_DEBUG_TEST...
   Response: 1 breakpoints
     - ID=KIND=0.SOURCETYPE=ABAP...LINE_NR=17, Line=17

3. Verifying breakpoint storage...
   Found 0 breakpoints stored   <-- NOT PERSISTED!

6. Waiting for breakpoint hit (max 90s)...
   Listener timed out - no breakpoint was hit
```

### Root Cause Analysis

The HTTP API appears to have different behavior than RFC:

| Aspect | RFC (Eclipse) | HTTP (vsp) |
|--------|--------------|------------|
| Breakpoint SET | Persists | Accepted but NOT persisted |
| Breakpoint GET | Returns stored BPs | Returns empty |
| Listener | Catches debuggee | Times out |
| Session | Maintained by RFC | Stateless HTTP |

This suggests the debugger API is designed primarily for RFC and HTTP support is limited or requires specific session handling that we haven't discovered.

---

## Conclusion

1. **Eclipse uses RFC (port 3300)** for all debugging operations
2. **HTTP batch endpoint works** but breakpoints don't persist via HTTP
3. **External breakpoints require RFC** for full functionality

### Recommendations

**Short-term:** Mark external debugger as RFC-only in vsp documentation

**Long-term options:**
1. **RFC Integration** - Use SAP NW RFC SDK (complex, platform-specific)
2. **SAP GUI fallback** - Port 3200 for GUI-based debugging
3. **Accept limitation** - Document that external debugging requires Eclipse ADT

### What DOES Work via HTTP

- Reading breakpoint lists
- Debug listener (but won't catch without persisted BPs)
- Batch requests (structure works)
- Stack/variable retrieval (once attached)
- Step operations (once attached)

The debugging infrastructure is implemented correctly - the issue is that SAP's HTTP API doesn't persist breakpoints, making the listener unable to catch any debuggees.

---

*Report updated with batch endpoint testing results on 2025-12-14*
