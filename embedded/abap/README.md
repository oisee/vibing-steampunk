# Embedded ABAP Objects

This directory contains optional ABAP objects that can be deployed to SAP systems for enhanced vsp functionality.

---

## WebSocket Handler (ZADT_VSP) v2.0.0

> **CRITICAL FOR AI-POWERED ROOT CAUSE ANALYSIS (RCA)**
>
> This WebSocket handler is the **cornerstone of programmatic ABAP debugging**. Without it, AI agents
> cannot step through code, inspect variables, or perform automated RCA. The standard ADT REST APIs
> do not provide stateful debug sessions - only this WebSocket/TPDAPI integration enables true
> AI-driven debugging capabilities.

The WebSocket handler enables **stateful operations** not available through standard ADT REST APIs:

- **RFC/BAPI execution** with full parameter support
- **Real-time debugging** with step-into, step-over, step-return, continue
- **Call stack inspection** during debug sessions
- **Variable inspection** at breakpoints

### Why WebSocket?

| HTTP REST | WebSocket |
|-----------|-----------|
| Stateless | Stateful session |
| Cannot maintain debug context | Persistent debug session |
| Timeout issues with blocking ops | Native long-polling |
| No TPDAPI access | Full debugger integration |

### Objects (9 total - self-contained package)

| File | Object | Description |
|------|--------|-------------|
| `zif_vsp_service.intf.abap` | Interface | Service contract for domain handlers |
| `zcl_vsp_apc_handler.clas.abap` | Class | Main APC WebSocket handler (router) |
| `zcl_vsp_rfc_service.clas.abap` | Class | RFC domain - function module calls, package moves |
| `zcl_vsp_debug_service.clas.abap` | Class | Debug domain - TPDAPI integration |
| `zcl_vsp_amdp_service.clas.abap` | Class | AMDP domain - HANA/SQLScript debugging |
| `zcl_vsp_git_service.clas.abap` | Class | Git domain - abapGit integration (158 object types) |
| `zadt_cl_tadir_move.clas.abap` | Class | TADIR package reassignment helper |
| `zcl_adt_00_amdp_test.clas.abap` | Class | AMDP test class for debugging demos |

**Note:** The Git service requires abapGit to be installed (optional dependency, handled gracefully).

### Deployment

#### Option 1: Using vsp WriteSource

```bash
# Create package first
vsp CreatePackage --name '$ZADT_VSP' --description 'VSP WebSocket Handler'

# Deploy interface
vsp WriteSource --object_type INTF --name ZIF_VSP_SERVICE \
    --package '$ZADT_VSP' --source "$(cat embedded/abap/zif_vsp_service.intf.abap)"

# Deploy RFC service
vsp WriteSource --object_type CLAS --name ZCL_VSP_RFC_SERVICE \
    --package '$ZADT_VSP' --source "$(cat embedded/abap/zcl_vsp_rfc_service.clas.abap)"

# Deploy debug service
vsp WriteSource --object_type CLAS --name ZCL_VSP_DEBUG_SERVICE \
    --package '$ZADT_VSP' --source "$(cat embedded/abap/zcl_vsp_debug_service.clas.abap)"

# Deploy handler
vsp WriteSource --object_type CLAS --name ZCL_VSP_APC_HANDLER \
    --package '$ZADT_VSP' --source "$(cat embedded/abap/zcl_vsp_apc_handler.clas.abap)"

# Deploy AMDP service
vsp WriteSource --object_type CLAS --name ZCL_VSP_AMDP_SERVICE \
    --package '$ZADT_VSP' --source "$(cat embedded/abap/zcl_vsp_amdp_service.clas.abap)"

# Deploy Git service
vsp WriteSource --object_type CLAS --name ZCL_VSP_GIT_SERVICE \
    --package '$ZADT_VSP' --source "$(cat embedded/abap/zcl_vsp_git_service.clas.abap)"
```

#### Option 2: Using ImportFromFile

```bash
vsp ImportFromFile --file_path embedded/abap/zif_vsp_service.intf.abap --package_name '$ZADT_VSP'
vsp ImportFromFile --file_path embedded/abap/zcl_vsp_rfc_service.clas.abap --package_name '$ZADT_VSP'
vsp ImportFromFile --file_path embedded/abap/zcl_vsp_debug_service.clas.abap --package_name '$ZADT_VSP'
vsp ImportFromFile --file_path embedded/abap/zcl_vsp_apc_handler.clas.abap --package_name '$ZADT_VSP'
vsp ImportFromFile --file_path embedded/abap/zcl_vsp_amdp_service.clas.abap --package_name '$ZADT_VSP'
vsp ImportFromFile --file_path embedded/abap/zcl_vsp_git_service.clas.abap --package_name '$ZADT_VSP'
```

#### Option 3: Using InstallZADTVSP (Recommended)

The easiest deployment method - single command:

```bash
vsp InstallZADTVSP --package '$ZADT_VSP'
```

This tool reads embedded sources and deploys all objects automatically.

### Post-Deployment: Create APC Application

After deploying the ABAP objects, create the APC application manually:

1. **Transaction SAPC** - Create APC Application:
   - Application ID: `ZADT_VSP`
   - Description: `VSP WebSocket Handler`
   - Handler Class: `ZCL_VSP_APC_HANDLER`
   - State: Stateful

2. **Transaction SICF** - Activate ICF Service:
   - Path: `/sap/bc/apc/sap/zadt_vsp`
   - Activate the node

### Testing

```bash
# Test connection
wscat -c "ws://host:port/sap/bc/apc/sap/zadt_vsp?sap-client=001" \
      -H "Authorization: Basic $(echo -n user:pass | base64)"

# Or use Go test
go run test/websocket_test.go
```

### Verified Working (2025-12-21)

Full end-to-end debugging tested and verified:

| Feature | Status | Notes |
|---------|--------|-------|
| WebSocket Connection | **Working** | Session ID and version returned |
| Statement Breakpoints | **Working** | e.g., `CONDENSE`, `CALL FUNCTION` |
| Exception Breakpoints | **Working** | e.g., `CX_SY_ZERODIVIDE` |
| Listener (TPDAPI) | **Working** | Catches debuggees from external sessions |
| Attach to Debuggee | **Working** | Returns program, include, line |
| Get Variables | **Working** | SY-SUBRC, SY-TABIX, SY-UNAME, etc. |
| Step Over | **Working** | Advances to next line |
| Continue | **Working** | Runs until next breakpoint |
| Detach | **Working** | Ends debug session |
| RFC Domain | **Working** | Function module execution |

**Note:** Line breakpoints require correct program name (e.g., `SAPLZFUGR` for function groups,
`ZCL_CLASS================CP` for classes). Statement and exception breakpoints work globally.

**Important:** The breakpoints trigger code executed in a *different* SAP session (e.g., HTTP request,
unit tests), not code within the same WebSocket handler. This is standard SAP external debugger behavior.

---

## RFC Domain (`domain: "rfc"`)

Execute any RFC/BAPI function module with parameters, plus object management.

### Actions

| Action | Description |
|--------|-------------|
| `call` | Execute RFC with parameters |
| `search` | Search function modules |
| `getMetadata` | Get function signature |
| `moveToPackage` | Move object to different package (TADIR update) |

### Examples

```json
// Call RFC_SYSTEM_INFO
{
  "id": "1",
  "domain": "rfc",
  "action": "call",
  "params": {
    "function": "RFC_SYSTEM_INFO"
  }
}

// Call BAPI with parameters
{
  "id": "2",
  "domain": "rfc",
  "action": "call",
  "params": {
    "function": "BAPI_USER_GET_DETAIL",
    "USERNAME": "DEVELOPER"
  }
}

// Move object to different package
{
  "id": "3",
  "domain": "rfc",
  "action": "moveToPackage",
  "params": {
    "pgmid": "R3TR",
    "object": "CLAS",
    "obj_name": "ZCL_MY_CLASS",
    "new_package": "$ZADT_VSP"
  }
}
```

---

## Debug Domain (`domain: "debug"`) - TPDAPI Integration

Full debugging capabilities via SAP's Test and Performance Development API.

### Actions

| Action | Description | Params |
|--------|-------------|--------|
| `getStatus` | Get debug session status | - |
| `listen` | Wait for debuggee (blocking) | `timeout` (sec), `user` |
| `getDebuggees` | List waiting debuggees | `user` |
| `attach` | Attach to debuggee | `debuggeeId` |
| `step` | Step execution | `type`: into/over/return/continue |
| `getStack` | Get call stack | - |
| `getVariables` | Get variable values | `scope`: system/locals/all |
| `detach` | End debug session | - |
| `setBreakpoint` | Set session breakpoint | `kind`, `uri`, `line`, etc. |
| `getBreakpoints` | List session breakpoints | - |
| `deleteBreakpoint` | Remove breakpoint | `breakpointId` |

### Complete Debug Flow Example

```javascript
// 1. Check debugging is available
ws.send({"id":"1","domain":"debug","action":"getStatus"})
// Response: {"debuggingAvailable":true,"attached":false,...}

// 2. Start listener (blocking call - waits for debuggee)
ws.send({"id":"2","domain":"debug","action":"listen","params":{"timeout":120}})

// ... Meanwhile, trigger execution (e.g., RunUnitTests via MCP) ...

// Response when debuggee caught:
// {"status":"caught","debuggees":[{"id":"ABC123...","program":"ZCL_MY_CLASS","user":"DEVELOPER"}]}

// 3. Attach to the debuggee
ws.send({"id":"3","domain":"debug","action":"attach","params":{"debuggeeId":"ABC123..."}})
// Response: {"attached":true,"program":"ZCL_MY_CLASS","include":"...CCIMP","line":42}

// 4. Step through code
ws.send({"id":"4","domain":"debug","action":"step","params":{"type":"over"}})
// Response: {"stepped":"over","program":"ZCL_MY_CLASS","line":43,"procedure":"METHOD_NAME"}

// 5. Check call stack
ws.send({"id":"5","domain":"debug","action":"getStack"})
// Response: {"stack":[{"index":0,"program":"ZCL_MY_CLASS","line":43,"active":true},...]

// 6. Inspect variables
ws.send({"id":"6","domain":"debug","action":"getVariables","params":{"scope":"system"}})
// Response: {"variables":[{"name":"SY-SUBRC","value":"0"},{"name":"SY-TABIX","value":"5"},...]}

// 7. Continue execution
ws.send({"id":"7","domain":"debug","action":"step","params":{"type":"continue"}})
// Response: {"stepped":"continue","ended":true,"message":"Debuggee ended"}

// 8. Detach (cleanup)
ws.send({"id":"8","domain":"debug","action":"detach"})
// Response: {"detached":true}
```

### Root Cause Analysis (RCA) Workflow

Combine WebSocket debugging with MCP tools for powerful RCA:

```
┌─────────────────────────────────────────────────────────────┐
│                    RCA Workflow                              │
│                                                              │
│  1. GetDumps ──────► Find runtime error in ST22              │
│         │                                                    │
│  2. GetDump ───────► Analyze stack trace, find failure point │
│         │                                                    │
│  3. GetSource ─────► Read code at failure location           │
│         │                                                    │
│  4. SetExternalBreakpoint ──► Set BP before failure          │
│         │                                                    │
│  5. RunUnitTests ──► Trigger reproduction                    │
│         │                                                    │
│  6. WebSocket ─────► listen → attach → step → inspect        │
│         │                                                    │
│  7. Identify ──────► Root cause found, fix code              │
└─────────────────────────────────────────────────────────────┘
```

### Example: Debugging a Division by Zero

```bash
# Step 1: Find the dump
vsp GetDumps --exception_type CX_SY_ZERODIVIDE --user DEVELOPER

# Step 2: Get details
vsp GetDump --dump_id "20251219/143052/DEVELOPER"
# Shows: ZCL_INVOICE_CALC, line 127, method CALCULATE_DISCOUNT

# Step 3: Set breakpoint
vsp SetExternalBreakpoint --kind line \
    --object_uri "/sap/bc/adt/oo/classes/zcl_invoice_calc/source/main" \
    --line 126 --user DEVELOPER

# Step 4: Run unit tests to trigger (in parallel with WebSocket listener)
vsp RunUnitTests --object_url "/sap/bc/adt/oo/classes/zcl_invoice_calc"

# Step 5: Debug via WebSocket (see flow above)
# Discover: lv_discount_rate = 0 causing division by zero
```

### OData/RAP Debugging

Debug RAP behavior implementations by:

1. Set breakpoint in behavior pool class (e.g., `ZCL_BP_I_SALESORDER`)
2. Start WebSocket listener
3. Trigger OData call (via test or curl)
4. Attach and step through the read/create/update/delete implementation

```json
// Set breakpoint in RAP handler
vsp SetExternalBreakpoint --kind line \
    --object_uri "/sap/bc/adt/oo/classes/zcl_bp_i_salesorder/source/main" \
    --line 45 --user DEVELOPER

// Trigger via curl
// curl -X GET "http://host:50000/sap/opu/odata4/sap/zapi_salesorder/SalesOrder('1000')"

// WebSocket: listen → attach → step → analyze
```

---

## AMDP Domain (`domain: "amdp"`) - HANA SQLScript Debugging

Debug AMDP (ABAP Managed Database Procedures) and SQLScript code.

### Actions

| Action | Description |
|--------|-------------|
| `start` | Start AMDP debug session |
| `stop` | Stop AMDP debug session |
| `setBreakpoint` | Set breakpoint in SQLScript procedure |
| `getBreakpoints` | List current breakpoints |
| `step` | Step through SQLScript code |
| `getVariables` | Get SQLScript variable values |
| `getStatus` | Get current debug session status |

### Example

```json
// Start AMDP debug session
{
  "id": "1",
  "domain": "amdp",
  "action": "start",
  "params": { "user": "DEVELOPER" }
}

// Set breakpoint in AMDP procedure
{
  "id": "2",
  "domain": "amdp",
  "action": "setBreakpoint",
  "params": {
    "proc_name": "ZCL_MY_AMDP=>GET_DATA",
    "line": 15
  }
}

// Step through code
{
  "id": "3",
  "domain": "amdp",
  "action": "step",
  "params": { "step_type": "stepOver" }
}
```

---

## Git Domain (`domain: "git"`) - abapGit Integration

Export and deploy ABAP objects using abapGit-compatible format (158 object types).

### Actions

| Action | Description |
|--------|-------------|
| `getTypes` | List supported object types |
| `export` | Export packages/objects to ZIP |
| `status` | Check abapGit availability |

### Example

```json
// Get supported object types
{
  "id": "1",
  "domain": "git",
  "action": "getTypes"
}

// Export package to ZIP
{
  "id": "2",
  "domain": "git",
  "action": "export",
  "params": {
    "packages": "$ZADT_VSP",
    "include_subpackages": true
  }
}
```

### Supported Object Types (158)

CLAS, INTF, PROG, FUGR, FUNC, TABL, TTYP, DTEL, DOMA, SHLP, VIEW, DDLS, DCLS, DDLX,
BDEF, SRVD, SRVB, SMBC, STYL, TRAN, MSAG, ENQU, AUTH, SUSO, SUSC, and 130+ more.

---

## Architecture

```
WebSocket Client (vsp/AI Agent)
      │
      ▼
ZCL_VSP_APC_HANDLER (router)
      │
      ├── ZCL_VSP_RFC_SERVICE (domain: "rfc")
      │         │
      │         ├── CALL FUNCTION (dynamic)
      │         └── TR_TADIR_INTERFACE (package moves)
      │
      ├── ZCL_VSP_DEBUG_SERVICE (domain: "debug")
      │         │
      │         └── CL_TPDAPI_SERVICE / IF_TPDAPI_SESSION
      │                   │
      │                   └── SAP Debugger (same as Eclipse ADT)
      │
      ├── ZCL_VSP_AMDP_SERVICE (domain: "amdp")
      │         │
      │         └── CL_AMDP_DEBUGGER (SQLScript/HANA debugging)
      │
      └── ZCL_VSP_GIT_SERVICE (domain: "git")
                │
                └── ZCL_ABAPGIT* (abapGit serialization, 158 object types)
```

Each service implements `ZIF_VSP_SERVICE` interface:
- `get_domain()` - Returns domain name for routing
- `handle_message()` - Processes action requests
- `on_disconnect()` - Cleanup when WebSocket closes

---

## Step Types

| Type | Description | Use Case |
|------|-------------|----------|
| `into` | Step into function/method call | Investigate called code |
| `over` | Step over, execute call as single step | Skip known-good code |
| `return` | Run until current function returns | Exit deep call stack |
| `continue` | Run until next breakpoint or end | Fast forward |

---

## Variable Scopes

| Scope | Variables |
|-------|-----------|
| `system` | SY-SUBRC, SY-TABIX, SY-INDEX, SY-DBCNT, SY-UNAME, SY-DATUM, SY-UZEIT, SY-CPROG |
| `locals` | Local variables in current scope (planned) |
| `all` | All available variables |

---

## Error Responses

```json
{
  "id": "request-id",
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human readable description"
  }
}
```

Common error codes:
- `UNKNOWN_ACTION` - Invalid action name
- `INVALID_PARAMS` - Missing required parameters
- `DEBUG_NOT_AVAILABLE` - System doesn't support debugging
- `LISTENER_CONFLICT` - Another listener already active
- `NOT_ATTACHED` - Operation requires attached debuggee
- `ALREADY_ATTACHED` - Already debugging another process

---

## Security

- **Authentication**: Uses SAP credentials via HTTP Basic Auth
- **Authorization**: Respects S_DEBUG authorization object
- **Session Isolation**: Each WebSocket connection gets isolated debug session
- **Cleanup**: `on_disconnect` properly releases debugger resources
- **Timeout**: Max 240 seconds prevents hung sessions

---

## Why WebSocket Debugging is CRITICAL for RCA

### The Problem with Traditional Debugging

Traditional ABAP debugging requires:
- Manual SAP GUI interaction
- Developer sitting at keyboard
- Cannot be automated or scripted
- No AI assistance possible

### The WebSocket Solution

With ZADT_VSP + TPDAPI:
- **Fully programmatic** - No GUI required
- **AI-driven** - Claude/LLMs can debug autonomously
- **Scriptable** - Reproducible debug sessions
- **Remote** - Debug from anywhere via WebSocket

### RCA Workflow: From Dump to Fix

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    AI-POWERED ROOT CAUSE ANALYSIS                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  PHASE 1: DISCOVERY                                                          │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐                   │
│  │  GetDumps    │───►│   GetDump    │───►│  GetSource   │                   │
│  │  (ST22)      │    │  (Details)   │    │  (Code)      │                   │
│  └──────────────┘    └──────────────┘    └──────────────┘                   │
│         │                   │                   │                            │
│         ▼                   ▼                   ▼                            │
│    Find errors         Stack trace         Read failure                      │
│    by user/date        & variables         location code                     │
│                                                                              │
│  PHASE 2: REPRODUCTION                                                       │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐                   │
│  │ setBreakpoint│───►│ RunUnitTests │───►│   listen     │                   │
│  │  (TPDAPI)    │    │  or SUBMIT   │    │  (TPDAPI)    │                   │
│  └──────────────┘    └──────────────┘    └──────────────┘                   │
│         │                   │                   │                            │
│         ▼                   ▼                   ▼                            │
│    Set BP before       Trigger the         Catch the                         │
│    failure point       failing code        debuggee                          │
│                                                                              │
│  PHASE 3: INVESTIGATION                                                      │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐                   │
│  │    attach    │───►│    step      │───►│ getVariables │                   │
│  │              │    │  (over/into) │    │              │                   │
│  └──────────────┘    └──────────────┘    └──────────────┘                   │
│         │                   │                   │                            │
│         ▼                   ▼                   ▼                            │
│    Attach to           Walk through        Inspect state                     │
│    stopped proc        execution           at each step                      │
│                                                                              │
│  PHASE 4: RESOLUTION                                                         │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐                   │
│  │  Identify    │───►│  EditSource  │───►│ RunUnitTests │                   │
│  │  Root Cause  │    │  (Fix Code)  │    │  (Verify)    │                   │
│  └──────────────┘    └──────────────┘    └──────────────┘                   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Example: AI Debugging a Production Issue

```javascript
// 1. AI finds recent dumps
const dumps = await mcp.GetDumps({ user: "BATCH_USER", max_results: 5 });
// Found: CX_SY_ZERODIVIDE in ZCL_INVOICE_CALC

// 2. AI analyzes dump details
const dump = await mcp.GetDump({ dump_id: dumps[0].id });
// Stack shows: CALCULATE_DISCOUNT method, line 127

// 3. AI sets breakpoint before failure
ws.send({ domain: "debug", action: "setBreakpoint", params: {
  kind: "line", program: "ZCL_INVOICE_CALC================CP", line: 125
}});

// 4. AI triggers reproduction via unit tests
mcp.RunUnitTests({ object_url: "/sap/bc/adt/oo/classes/zcl_invoice_calc" });

// 5. AI catches and attaches to debuggee
ws.send({ domain: "debug", action: "listen", params: { timeout: 60 }});
// Caught! Attach...
ws.send({ domain: "debug", action: "attach", params: { debuggeeId: "..." }});

// 6. AI steps and inspects
ws.send({ domain: "debug", action: "step", params: { type: "over" }});
ws.send({ domain: "debug", action: "getVariables", params: { scope: "all" }});
// Found: lv_discount_pct = 0, causing division by zero!

// 7. AI fixes the code
mcp.EditSource({ ... add IF lv_discount_pct > 0 check ... });

// 8. AI verifies fix
mcp.RunUnitTests({ ... }); // All tests pass!
```

---

## Future Roadmap: Smart Debugging & Tracing

### TODO: Extended Execution Triggers

Currently only RFC calls work. Future:

| Trigger Type | ABAP Statement | Use Case |
|--------------|----------------|----------|
| `submit` | `SUBMIT program` | Report execution |
| `callTransaction` | `CALL TRANSACTION` | Transaction debugging |
| `callScreen` | `CALL SCREEN` | Dynpro flow debugging |
| `callMethod` | Dynamic method calls | OO debugging |
| `backgroundJob` | `JOB_SUBMIT` | Batch job debugging |

### TODO: Smart Tracing Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SMART TRACING VISION                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  STEP 1: BUILD STATIC CALL GRAPH                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  GetCallGraph → Analyze CROSS/WBCROSSGT → Build complete graph       │    │
│  │                                                                      │    │
│  │  ZCL_ORDER_PROCESSOR                                                 │    │
│  │    ├── CREATE_ORDER                                                  │    │
│  │    │     ├── VALIDATE_CUSTOMER ──► ZCL_CUSTOMER=>CHECK_CREDIT        │    │
│  │    │     ├── CALCULATE_PRICE ───► ZCL_PRICING=>GET_PRICE             │    │
│  │    │     │                         └── ZCL_DISCOUNT=>APPLY           │    │
│  │    │     └── SAVE_ORDER ────────► ZCL_PERSISTENCE=>SAVE              │    │
│  │    └── CANCEL_ORDER                                                  │    │
│  │          └── REVERSE_POSTING ──► BAPI_ACC_DOCUMENT_REV_POST          │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  STEP 2: TRACE ACTUAL EXECUTION (Multiple Variants)                          │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Run with different inputs, collect actual call paths:               │    │
│  │                                                                      │    │
│  │  Test 1: Normal Order (Customer A, Product X)                        │    │
│  │    ✓ CREATE_ORDER → VALIDATE_CUSTOMER → CALCULATE_PRICE → SAVE       │    │
│  │                                                                      │    │
│  │  Test 2: Discounted Order (Customer B, Promo Code)                   │    │
│  │    ✓ CREATE_ORDER → VALIDATE → CALCULATE_PRICE → APPLY_DISCOUNT → SAVE│   │
│  │                                                                      │    │
│  │  Test 3: Failed Order (Bad Customer)                                 │    │
│  │    ✗ CREATE_ORDER → VALIDATE_CUSTOMER → [EXCEPTION: Credit Check]    │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  STEP 3: COMPARE & IDENTIFY                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Overlay traces on static graph:                                     │    │
│  │                                                                      │    │
│  │  ▓▓▓ = Common path (all tests)                                       │    │
│  │  ░░░ = Variant path (some tests)                                     │    │
│  │  ▒▒▒ = Error path (failing tests only)                               │    │
│  │  ─── = Dead code (never executed)                                    │    │
│  │                                                                      │    │
│  │  INSIGHTS:                                                           │    │
│  │  • APPLY_DISCOUNT only called with promo codes → edge case!          │    │
│  │  • REVERSE_POSTING never tested → missing test coverage!             │    │
│  │  • Credit check failure → need negative test case!                   │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  STEP 4: GENERATE BETTER TESTS                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  AI generates test cases for uncovered paths:                        │    │
│  │                                                                      │    │
│  │  • Test CANCEL_ORDER → REVERSE_POSTING flow                          │    │
│  │  • Test discount = 0 edge case (division by zero?)                   │    │
│  │  • Test concurrent order creation                                    │    │
│  │  • Test boundary values for quantity/price                           │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### RCA with Multiple Test Variants

The key insight: **Run the same code with different inputs and compare execution paths**

```javascript
// Run 3 variants of the same test
const variants = [
  { customer: "GOOD_CUST", product: "NORMAL", expected: "success" },
  { customer: "BAD_CUST", product: "NORMAL", expected: "credit_fail" },
  { customer: "GOOD_CUST", product: "PROMO", expected: "discount_applied" },
];

for (const variant of variants) {
  // Set statement breakpoint on all method entries
  ws.send({ action: "setBreakpoint", params: { kind: "statement", statement: "METHOD" }});

  // Run test
  mcp.RunUnitTests({ ... variant ... });

  // Collect trace
  const trace = [];
  while (true) {
    const step = await ws.send({ action: "step", params: { type: "over" }});
    trace.push({ program: step.program, line: step.line, procedure: step.procedure });
    if (step.ended) break;
  }

  variant.trace = trace;
}

// Compare traces → find divergence points → identify root cause
const divergencePoint = findFirstDifference(variants[0].trace, variants[1].trace);
// "Traces diverge at ZCL_CUSTOMER=>CHECK_CREDIT line 45"
```

### Benefits for RCA

| Capability | Without Smart Tracing | With Smart Tracing |
|------------|----------------------|-------------------|
| Find failure point | Manual investigation | Automatic via trace comparison |
| Identify edge cases | Developer intuition | Data-driven analysis |
| Test coverage gaps | Code coverage tools | Execution path analysis |
| Regression detection | Hope for the best | Compare before/after traces |

---

## See Also

- `reports/2025-12-19-001-websocket-debugging-deep-dive.md` - Comprehensive documentation
- `reports/2025-12-18-002-websocket-rfc-handler.md` - RFC domain documentation
- `reports/2025-12-05-013-ai-powered-rca-workflows.md` - RCA workflow design
- `reports/2025-12-05-014-debugger-scripting-vision.md` - Debugger scripting architecture
