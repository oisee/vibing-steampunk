# WebSocket RFC Handler (ZADT_VSP)

**Date:** 2025-12-18
**Report ID:** 002
**Subject:** APC WebSocket Handler for RFC/BAPI Calls
**Status:** Complete

---

## Overview

ZADT_VSP is an optional SAP-side WebSocket handler that enables stateful operations not available through standard ADT REST APIs. The primary use case is RFC/BAPI function module calls with full parameter support.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     vsp CLI / MCP Client                     │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ WebSocket (ws://)
                              ▼
┌─────────────────────────────────────────────────────────────┐
│              SAP APC (ABAP Push Channel)                     │
│                    /sap/bc/apc/sap/zadt_vsp                  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│               ZCL_VSP_APC_HANDLER                            │
│  - Session management                                        │
│  - Message routing                                           │
│  - Service registry                                          │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│ ZCL_VSP_RFC_    │ │ ZCL_VSP_DEBUG_  │ │ Future services │
│ SERVICE         │ │ SERVICE (TODO)  │ │                 │
│ domain: "rfc"   │ │ domain: "debug" │ │                 │
└─────────────────┘ └─────────────────┘ └─────────────────┘
```

## SAP Objects

| Object | Type | Description |
|--------|------|-------------|
| `$ZADT_VSP` | Package | Container package |
| `ZIF_VSP_SERVICE` | Interface | Service contract |
| `ZCL_VSP_APC_HANDLER` | Class | Main WebSocket handler |
| `ZCL_VSP_RFC_SERVICE` | Class | RFC domain service |
| `ZADT_VSP` | APC Application | WebSocket endpoint config |

## Message Protocol

### Request Format
```json
{
  "id": "unique-request-id",
  "domain": "rfc",
  "action": "call",
  "params": {
    "function": "BAPI_USER_GET_DETAIL",
    "USERNAME": "TESTUSER"
  },
  "timeout": 30000
}
```

### Response Format
```json
{
  "id": "unique-request-id",
  "success": true,
  "data": { ... }
}
```

### Error Response
```json
{
  "id": "unique-request-id",
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human readable message"
  }
}
```

## RFC Domain Actions

### `call` - Execute RFC/BAPI

Execute any RFC-enabled function module with parameters.

**Request:**
```json
{
  "id": "1",
  "domain": "rfc",
  "action": "call",
  "params": {
    "function": "BAPI_USER_GET_DETAIL",
    "USERNAME": "AVINOGRADOVA"
  }
}
```

**Response:**
```json
{
  "id": "1",
  "success": true,
  "data": {
    "subrc": 0,
    "exports": {
      "ADDRESS": {
        "FIRSTNAME": "Alice",
        "LASTNAME": "V",
        ...
      },
      "LOGONDATA": { ... }
    },
    "tables": {
      "PROFILES": [
        {"BAPIPROF": "SAP_ALL", "BAPIPTEXT": "All SAP System authorizations", ...},
        {"BAPIPROF": "S_A.SYSTEM", ...}
      ],
      "RETURN": []
    }
  }
}
```

**Supported parameter types:**
- Scalar types (CHAR, STRING, INT, etc.) ✅
- Structure EXPORTING params → JSON objects ✅
- TABLE params → arrays of JSON objects ✅
- Nested structures → `[complex]` placeholder ✅

### `search` - Find Function Modules

Search for function modules by pattern.

**Request:**
```json
{
  "id": "2",
  "domain": "rfc",
  "action": "search",
  "params": {"pattern": "BAPI_USER*"}
}
```

### `getMetadata` - Get Function Signature

Get parameter information for a function module.

**Request:**
```json
{
  "id": "3",
  "domain": "rfc",
  "action": "getMetadata",
  "params": {"function": "BAPI_USER_GET_DETAIL"}
}
```

### `ping` - Heartbeat

Simple ping/pong for connection health.

**Request:**
```json
{
  "id": "4",
  "domain": "rfc",
  "action": "ping"
}
```

## System Domain Actions

### `ping` - System Heartbeat

```json
{
  "id": "5",
  "domain": "system",
  "action": "ping"
}
```

## Installation

### Option 1: Manual Deployment via ADT

1. Create package `$ZADT_VSP` (local, no transport)
2. Create interface `ZIF_VSP_SERVICE` from `embedded/abap/zif_vsp_service.intf.abap`
3. Create class `ZCL_VSP_RFC_SERVICE` from `embedded/abap/zcl_vsp_rfc_service.clas.abap`
4. Create class `ZCL_VSP_APC_HANDLER` from `embedded/abap/zcl_vsp_apc_handler.clas.abap`
5. Create APC application `ZADT_VSP`:
   - Transaction: `SAPC`
   - Application ID: `ZADT_VSP`
   - Handler Class: `ZCL_VSP_APC_HANDLER`
   - Connection Type: WebSocket
   - URL Path: `/sap/bc/apc/sap/zadt_vsp`

### Option 2: Deploy via vsp CLI

```bash
# Deploy all WebSocket handler objects
vsp deploy-handler

# Or deploy individual objects
vsp import --file embedded/abap/zif_vsp_service.intf.abap --package $ZADT_VSP
vsp import --file embedded/abap/zcl_vsp_rfc_service.clas.abap --package $ZADT_VSP
vsp import --file embedded/abap/zcl_vsp_apc_handler.clas.abap --package $ZADT_VSP
```

### Option 3: Use WriteSource Tool

```bash
# Via MCP tools
vsp WriteSource --object_type INTF --name ZIF_VSP_SERVICE --package $ZADT_VSP --source "$(cat embedded/abap/zif_vsp_service.intf.abap)"
```

## Activation

After deploying objects, activate the APC application:

1. **Create APC Application (SAPC transaction):**
   - Application ID: `ZADT_VSP`
   - Description: `VSP WebSocket Handler`
   - Handler Class: `ZCL_VSP_APC_HANDLER`
   - Subprotocol: (leave empty)
   - State: Stateful

2. **Activate ICF Service:**
   - Transaction: `SICF`
   - Path: `/sap/bc/apc/sap/zadt_vsp`
   - Activate the service node

3. **Test Connection:**
   ```bash
   # Simple WebSocket test
   wscat -c "ws://host:port/sap/bc/apc/sap/zadt_vsp?sap-client=001" \
         -H "Authorization: Basic $(echo -n user:pass | base64)"
   ```

## Usage from Go

```go
import (
    "github.com/gorilla/websocket"
)

// Connect
wsURL := "ws://host:port/sap/bc/apc/sap/zadt_vsp?sap-client=001"
header := http.Header{}
header.Set("Authorization", "Basic "+base64Auth)

conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)

// Read welcome message
_, msg, _ := conn.ReadMessage()
fmt.Printf("Welcome: %s\n", msg)

// Call RFC
callMsg := `{"id":"1","domain":"rfc","action":"call","params":{"function":"RFC_SYSTEM_INFO"}}`
conn.WriteMessage(websocket.TextMessage, []byte(callMsg))

_, response, _ := conn.ReadMessage()
fmt.Printf("Response: %s\n", response)
```

## Technical Implementation Notes

### CALL FUNCTION with PARAMETER-TABLE

The RFC service uses dynamic function calls with `PARAMETER-TABLE`:

```abap
" Key insight: kind values are from CALLER perspective
" Function IMPORTS → Caller EXPORTS (abap_func_exporting)
" Function EXPORTS → Caller IMPORTS (abap_func_importing)
" Function TABLES → abap_func_tables

DATA lt_ptab TYPE abap_func_parmbind_tab.

" For function's IMPORT params
ls_ptab-kind = abap_func_exporting.  " We export TO the function

" For function's EXPORT params
ls_ptab-kind = abap_func_importing.  " We import FROM the function

CALL FUNCTION lv_function
  PARAMETER-TABLE lt_ptab
  EXCEPTION-TABLE lt_etab.
```

### Type Detection with RTTI

To avoid dumps when serializing complex types:

```abap
DATA(lo_type) = cl_abap_typedescr=>describe_by_data( <fs_data> ).
IF lo_type->kind = cl_abap_typedescr=>kind_elem.
  " Safe to convert to string
  lv_str = <fs_data>.
ELSE.
  " Structure/table - need component-level serialization
  ...
ENDIF.
```

### Empty String Check

For STRING type, empty string "" is NOT INITIAL:

```abap
" Wrong - returns TRUE for empty string
IF lv_string IS NOT INITIAL.

" Correct
IF strlen( lv_string ) > 0.
```

## Future: Debug Domain

The architecture supports adding a `debug` domain for stateful debugging:

```json
{
  "id": "1",
  "domain": "debug",
  "action": "setBreakpoint",
  "params": {
    "program": "ZTEST",
    "line": 42
  }
}
```

Planned actions:
- `setBreakpoint` - Set breakpoint
- `listen` - Wait for debuggee
- `attach` - Attach to debuggee
- `step` - Step into/over/out
- `getVariables` - Get variable values
- `continue` - Continue execution

## Security Considerations

- WebSocket endpoint requires SAP authentication
- Uses same authorization as user's SAP session
- RFC calls execute with user's authorizations
- No special privileges required beyond standard RFC auth

## Limitations

- Nested structures beyond 2 levels show `[complex]`
- CHANGING parameters not fully supported yet
- Binary data (XSTRING) serialized as hex string
- Large responses may impact performance

## Related Documents

- `adt-abap-internals-documentation.md` - ADT API details
- `2025-12-05-014-external-debugger-scripting-vision.md` - Debug domain design
- `cookie-auth-implementation-guide.md` - Authentication options
