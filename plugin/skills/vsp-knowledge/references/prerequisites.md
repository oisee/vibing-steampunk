# ZADT_VSP Prerequisites

## What Is ZADT_VSP?

ZADT_VSP is a set of ABAP objects that enable advanced VSP features via WebSocket:

- **Stateful debugging** (breakpoints, step, variables)
- **RFC execution** (function module calls)
- **Report execution** (RunReport, RunReportAsync)
- **AMDP debugging** (SQLScript/HANA debugging)
- **abapGit integration** (ZIP export with 158 object types)
- **ABAP keyword help** (GetAbapHelp)

## Components (9 objects)

| Object | Type | Purpose |
|--------|------|---------|
| ZIF_VSP_SERVICE | Interface | Service contract for domain handlers |
| ZCL_VSP_APC_HANDLER | Class | Main APC WebSocket handler (router) |
| ZCL_VSP_RFC_SERVICE | Class | RFC domain — function module calls, package moves |
| ZCL_VSP_DEBUG_SERVICE | Class | Debug domain — TPDAPI integration |
| ZCL_VSP_AMDP_SERVICE | Class | AMDP domain — HANA/SQLScript debugging |
| ZCL_VSP_GIT_SERVICE | Class | Git domain — abapGit integration |
| ZCL_VSP_REPORT_SERVICE | Class | Report execution service |
| ZCL_VSP_UTILS | Class | Utility functions |
| ZADT_CL_TADIR_MOVE | Class | TADIR package reassignment helper |

## Installation

### Option 1: Single command (recommended)
```
InstallZADTVSP(package="$ZADT_VSP")
```

### Option 2: Manual deployment
1. Create package `$ZADT_VSP`
2. Deploy each object via WriteSource or ImportFromFile
3. Create APC application in transaction SAPC:
   - Application ID: `ZADT_VSP`
   - Handler Class: `ZCL_VSP_APC_HANDLER`
   - State: Stateful
4. Activate ICF service at `/sap/bc/apc/sap/zadt_vsp`

## Checking Installation Status

Run `GetFeatures` or `ListDependencies` to check if ZADT_VSP is deployed.

## Optional Dependencies

- **abapGit**: Required for Git service (ZIP export). Detected automatically. Install via `InstallAbapGit` if needed.
- **HANA database**: Required for AMDP debugging. Detected via `GetFeatures`.

## What Works Without ZADT_VSP

All ADT REST API tools work without ZADT_VSP:
- Source read/write/edit
- Search, grep
- Syntax check, activate
- Unit tests, ATC checks
- Object creation/deletion
- Code intelligence (definitions, references, completion)
- Transport management

Only WebSocket-based features require ZADT_VSP.
