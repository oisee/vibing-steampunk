# Tool Reference

All 122 MCP tools available in vsp. **Mode** column: `F` = Focused (default), `E` = Expert only.

---

## Unified Source Access

| Tool | Mode | Description |
|------|------|-------------|
| `GetSource` | F | Read any ABAP source. `type`: PROG/CLAS/INTF/FUNC/FUGR/INCL/DDLS/BDEF/SRVD/SRVB/MSAG/VIEW. Optional: `parent` (for FUNC), `include` (for CLAS), `method` (for method-level access). |
| `WriteSource` | F | Write/create any ABAP source. Same types as GetSource. Auto-detects create vs update. |

---

## Search & Grep

| Tool | Mode | Description |
|------|------|-------------|
| `SearchObject` | F | Quick search for ABAP objects by name pattern |
| `GrepObjects` | F | Regex search across multiple objects (array of URLs). Supports context lines, case flags, type filter. |
| `GrepPackages` | F | Regex search across packages with recursive subpackage support |
| `GrepObject` | E | Regex search in a single object |
| `GrepPackage` | E | Regex search in a single package |

---

## Read Operations (Granular)

These are superseded by `GetSource` in Focused mode but available individually in Expert mode.

| Tool | Mode | Description |
|------|------|-------------|
| `GetProgram` | E | Get ABAP program source |
| `GetClass` | E | Get ABAP class source |
| `GetInterface` | E | Get ABAP interface source |
| `GetFunction` | E | Get function module source |
| `GetFunctionGroup` | F | Get function group structure (list of function modules) |
| `GetInclude` | E | Get ABAP include source |
| `GetTable` | F | Get table/structure definition |
| `GetTableContents` | F | Query table data (supports WHERE, ORDER BY using ABAP SQL syntax) |
| `GetStructure` | E | Get structure definition |
| `GetPackage` | F | List package contents |
| `GetTransaction` | E | Get transaction details |
| `GetTypeInfo` | E | Get data type information |
| `GetCDSDependencies` | F | Get CDS view dependency tree |
| `RunQuery` | F | Execute freestyle ABAP SQL query (can be blocked with `--block-free-sql`) |

!!! note "ABAP SQL Syntax"
    `RunQuery` uses ABAP SQL, not standard SQL: use `ASCENDING`/`DESCENDING` (not `ASC`/`DESC`), and pass `max_rows` parameter instead of `LIMIT`.

---

## System Information

| Tool | Mode | Description |
|------|------|-------------|
| `GetSystemInfo` | F | SAP system info: SID, release, kernel version, database type, host |
| `GetInstalledComponents` | F | List installed software components with version numbers |
| `GetConnectionInfo` | F | Current connection details (user, client, language) |
| `GetFeatures` | F | Probe system for installed capabilities (abapGit, RAP, AMDP, UI5, CTS) |
| `GetAbapHelp` | F | ABAP keyword documentation from the connected SAP system (requires ZADT_VSP) |

---

## Code Analysis

| Tool | Mode | Description |
|------|------|-------------|
| `GetCallGraph` | F | Call hierarchy (callers and callees) for a method or function |
| `GetObjectStructure` | F | Object explorer tree (methods, attributes, includes) |
| `GetCallersOf` | E | Static call graph — who calls this object (upward traversal) |
| `GetCalleesOf` | E | Static call graph — what this object calls (downward traversal) |
| `AnalyzeCallGraph` | E | Call graph statistics: nodes, edges, depth, type distribution |
| `CompareCallGraphs` | E | Compare static vs actual execution (test coverage analysis) |
| `TraceExecution` | E | **Composite RCA tool**: static graph + runtime trace + comparison |

---

## Diagnostics

| Tool | Mode | Description |
|------|------|-------------|
| `GetDumps` | F | List runtime errors (short dumps / RABAX). Filter by user, exception type, program, date range. |
| `GetDump` | F | Full details of a specific dump: stack trace, variable values, context |
| `ListTraces` | F | List ABAP runtime profiler traces (ATRA) |
| `GetTrace` | F | Profiler trace analysis: `hitlist` (hot spots), `statements`, `dbAccesses` |
| `GetSQLTraceState` | F | Check if SQL trace (ST05) is active |
| `ListSQLTraces` | F | List SQL trace files |

---

## Development Tools

| Tool | Mode | Description |
|------|------|-------------|
| `SyntaxCheck` | F | Check source code syntax |
| `Activate` | E | Activate a single object |
| `ActivatePackage` | F | Batch activate all inactive objects in a package (with optional filtering) |
| `RunUnitTests` | F | Execute ABAP Unit tests for an object or package |
| `RunATCCheck` | F | ATC code quality check. Returns findings with priority (1=Error, 2=Warning, 3=Info). |
| `GetATCCustomizing` | E | Get ATC system configuration |
| `CompareSource` | F | Unified diff between any two ABAP objects |
| `CloneObject` | F | Copy PROG/CLAS/INTF to a new name |
| `GetClassInfo` | F | Quick class metadata: methods, attributes, interfaces |
| `CreateTable` | F | Create a DDIC table from JSON definition |
| `CreatePackage` | F | Create a local or transportable package |
| `PrettyPrint` | E | Format ABAP source code |
| `GetPrettyPrinterSettings` | E | Get formatter settings |
| `SetPrettyPrinterSettings` | E | Update formatter settings |
| `GetTypeHierarchy` | E | Get type hierarchy (supertypes and subtypes) |
| `GetInactiveObjects` | E | List inactive objects in the system |

---

## CRUD Operations

| Tool | Mode | Description |
|------|------|-------------|
| `LockObject` | F | Acquire edit lock on an object |
| `UnlockObject` | F | Release edit lock |
| `CreateObject` | E | Create a new object (supports PROG, CLAS, INTF, INCL, FUGR, FUNC, DDLS, BDEF, SRVD, SRVB) |
| `UpdateSource` | E | Write source code (low-level, requires manual lock/unlock) |
| `DeleteObject` | E | Delete an object |

---

## Workflow Tools

| Tool | Mode | Description |
|------|------|-------------|
| `EditSource` | F | **Surgical string replacement** matching Claude's Edit pattern. Auto-handles lock/unlock/activate. |
| `WriteProgram` | E | Update program: lock → syntax check → update → unlock → activate |
| `WriteClass` | E | Update class: lock → syntax check → update → unlock → activate |
| `CreateAndActivateProgram` | E | Create new program and activate |
| `CreateClassWithTests` | E | Create class with test include, write tests, activate, run tests |

---

## Code Intelligence

| Tool | Mode | Description |
|------|------|-------------|
| `FindDefinition` | F | Navigate to symbol definition |
| `FindReferences` | F | Find all references to a symbol |
| `CodeCompletion` | E | Get code completion suggestions at cursor position |

---

## File Operations

| Tool | Mode | Description |
|------|------|-------------|
| `ImportFromFile` | F | **File → SAP**: Deploy source from local file. Auto-detects create vs update. Supports `.clas.abap`, `.prog.abap`, `.intf.abap`, `.fugr.abap`, `.func.abap`, `.ddls.asddls`, `.bdef.asbdef`, `.srvd.srvdsrv`. |
| `ExportToFile` | F | **SAP → File**: Save object source to local filesystem |
| `MoveObject` | E | Move object to a different package |
| `RenameObject` | E | Rename object (creates copy, deletes original) |
| `DeployFromFile` | E | Alias for `ImportFromFile` |
| `SaveToFile` | E | Alias for `ExportToFile` |

---

## Class Include Operations

| Tool | Mode | Description |
|------|------|-------------|
| `GetClassInclude` | E | Get class include: `definitions`, `implementations`, `macros`, `testclasses` |
| `CreateTestInclude` | E | Create test classes include for a class |
| `UpdateClassInclude` | E | Update class include source |

---

## Service Binding

| Tool | Mode | Description |
|------|------|-------------|
| `PublishServiceBinding` | E | Publish service binding to make it available as OData service |
| `UnpublishServiceBinding` | E | Unpublish a service binding |

---

## ABAP Debugger

See [ABAP Debugger](../features/debugging.md) for full documentation.

| Tool | Mode | Description |
|------|------|-------------|
| `DebuggerListen` | F | Start listening for a debug session (wait for debuggee to connect) |
| `DebuggerAttach` | F | Attach to a debug session |
| `DebuggerDetach` | F | Detach from current debug session |
| `DebuggerStep` | F | Step execution: `over`, `into`, `return`, `continue` |
| `DebuggerGetStack` | F | Get current call stack frames |
| `DebuggerGetVariables` | F | Get variable values at current stack frame |

Breakpoints are managed via WebSocket (ZADT_VSP):

| Tool | Mode | Description |
|------|------|-------------|
| `SetBreakpoint` | F | Set a breakpoint (line, exception, statement, message) |
| `GetBreakpoints` | F | List active breakpoints |
| `DeleteBreakpoint` | F | Remove a breakpoint |

---

## abapGit

See [abapGit Integration](../features/abapgit.md) for full documentation.

| Tool | Mode | Description |
|------|------|-------------|
| `GitTypes` | F | List all 158 supported abapGit object types |
| `GitExport` | F | Export packages/objects as abapGit-compatible ZIP (base64-encoded). Requires abapGit on SAP. |

---

## Report Execution

Requires ZADT_VSP WebSocket handler deployed to SAP.

| Tool | Mode | Description |
|------|------|-------------|
| `RunReport` | F | Execute ABAP report with parameters or variant, capture ALV output |
| `RunReportAsync` | F | Start report in background, returns `task_id` |
| `GetAsyncResult` | F | Poll or wait for async task completion (up to 60s) |
| `GetVariants` | F | List available variants for a report |
| `GetTextElements` | F | Get report selection texts and text symbols |
| `SetTextElements` | F | Update report selection texts and text symbols |

---

## Transport Management

See [Transport Management](../features/transport.md). Requires `--enable-transports`.

| Tool | Mode | Description |
|------|------|-------------|
| `ListTransports` | E | List transport requests (own, all, or by package) |
| `GetTransport` | E | Get transport details and object list |
| `CreateTransport` | E | Create a transport request |
| `ReleaseTransport` | E | Release a transport request |
| `DeleteTransport` | E | Delete a transport request |

---

## UI5 / BSP Management

| Tool | Mode | Description |
|------|------|-------------|
| `UI5ListApps` | F | List UI5/Fiori BSP applications |
| `UI5GetApp` | F | Get BSP application metadata |
| `UI5GetFileContent` | F | Read file content from BSP application |
| `UI5UploadFile` | E | Upload file to BSP application |
| `UI5DeleteFile` | E | Delete file from BSP application |
| `UI5CreateApp` | E | Create new BSP application |
| `UI5DeleteApp` | E | Delete BSP application |

---

## AMDP / HANA Debugger (Experimental)

Expert mode only. Requires ZADT_VSP. See project issue tracker for status.

| Tool | Mode | Description |
|------|------|-------------|
| `AMDPDebuggerStart` | E | Start AMDP debug session |
| `AMDPDebuggerResume` | E | Resume AMDP debug session |
| `AMDPDebuggerStop` | E | Stop AMDP debug session |
| `AMDPDebuggerStep` | E | Step execution in AMDP debugger |
| `AMDPGetVariables` | E | Get variable values in AMDP session |
| `AMDPSetBreakpoint` | E | Set AMDP breakpoint |
| `AMDPGetBreakpoints` | E | List AMDP breakpoints |

---

## ExecuteABAP

See [ExecuteABAP](../features/execute-abap.md) for full documentation.

| Tool | Mode | Description |
|------|------|-------------|
| `ExecuteABAP` | E | Run arbitrary ABAP code via unit test wrapper. Risk levels: `harmless`, `dangerous`, `critical`. |

---

## Install / Setup

| Tool | Mode | Description |
|------|------|-------------|
| `InstallZADTVSP` | F | Deploy ZADT_VSP WebSocket handler to SAP (6 ABAP objects). Parameters: `package`, `skip_git_service`, `check_only`. |
| `InstallAbapGit` | F | Deploy abapGit to SAP. Parameters: `edition` (`standalone`/`dev`), `package`, `check_only`. |
| `ListDependencies` | F | List available dependencies for installation |

---

## Tool Count Summary

| Mode | Tools | Token overhead |
|------|-------|----------------|
| Focused | 81 | ~2,800 |
| Expert | 122 | ~8,000 |
