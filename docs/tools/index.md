# Tools Overview

vsp exposes SAP ADT operations as MCP tools. Two modes control which tools are available.

## Focused vs Expert Mode

| Aspect | Focused (Default) | Expert |
|--------|-------------------|--------|
| **Tools** | 81 essential | 122 complete |
| **Token overhead** | ~2,800 | ~8,000 |
| **Use case** | Daily AI-assisted development | Edge cases, low-level ops |
| **Unified tools** | `GetSource`, `WriteSource` | + granular `GetProgram`, `GetClass`, etc. |

**Focused mode** consolidates 11 granular read/write tools into 2 unified tools, cutting token usage by ~58% and reducing decision complexity.

```bash
# Default: focused mode
vsp

# Enable all 122 tools
vsp --mode expert
# or
SAP_MODE=expert vsp
```

---

## Tool Categories

### Unified Source Access (2 tools — Focused)
`GetSource` / `WriteSource` — read or write any ABAP source with a single tool. Supports PROG, CLAS, INTF, FUNC, FUGR, INCL, DDLS, BDEF, SRVD, SRVB, MSAG, VIEW. Optional `method` parameter for method-level access (95% token reduction for large classes).

### Search & Grep (4 tools — Focused + Expert)
`SearchObject`, `GrepObjects`, `GrepPackages` (Focused) + `GrepObject`, `GrepPackage` (Expert)

### Read Operations (14 tools — Expert)
Granular read tools: `GetProgram`, `GetClass`, `GetInterface`, `GetFunction`, `GetFunctionGroup`, `GetInclude`, `GetTable`, `GetTableContents`, `GetStructure`, `GetPackage`, `GetTransaction`, `GetTypeInfo`, `GetCDSDependencies`, `RunQuery`

### System Information (3 tools — Focused)
`GetSystemInfo`, `GetInstalledComponents`, `GetConnectionInfo`

### Feature Detection (1 tool — Focused)
`GetFeatures` — probes SAP system for installed components (abapGit, RAP, AMDP, UI5, CTS)

### Code Analysis (7 tools — Focused + Expert)
`GetCallGraph`, `GetObjectStructure` (Focused) + `GetCallersOf`, `GetCalleesOf`, `AnalyzeCallGraph`, `CompareCallGraphs`, `TraceExecution` (Expert)

### Diagnostics (6 tools — Focused)
`GetDumps`, `GetDump` (short dumps / RABAX), `ListTraces`, `GetTrace` (ATRA profiler), `GetSQLTraceState`, `ListSQLTraces` (ST05)

### Development Tools (10 tools)
`SyntaxCheck`, `ActivatePackage`, `RunUnitTests`, `RunATCCheck`, `CompareSource`, `CloneObject`, `GetClassInfo`, `CreateTable`, `CreatePackage` (Focused) + `Activate` (Expert)

### CRUD Operations (5 tools — Expert)
`LockObject`, `UnlockObject` (Focused) + `CreateObject`, `UpdateSource`, `DeleteObject` (Expert)

### Workflow Tools (5 tools)
`EditSource` (Focused) + `WriteProgram`, `WriteClass`, `CreateAndActivateProgram`, `CreateClassWithTests` (Expert)

### Code Intelligence (7 tools)
`FindDefinition`, `FindReferences` (Focused) + `CodeCompletion`, `PrettyPrint`, `GetPrettyPrinterSettings`, `SetPrettyPrinterSettings`, `GetTypeHierarchy` (Expert)

### File Operations (4 tools)
`ImportFromFile`, `ExportToFile` (Focused) + `DeployFromFile` (alias), `SaveToFile` (alias), `RenameObject`, `MoveObject` (Expert)

### ABAP Debugger (6 tools — Focused)
`DebuggerListen`, `DebuggerAttach`, `DebuggerDetach`, `DebuggerStep`, `DebuggerGetStack`, `DebuggerGetVariables`

### abapGit (2 tools — Focused)
`GitTypes`, `GitExport` — requires abapGit installed on SAP system

### Report Execution (6 tools — Focused, via ZADT_VSP)
`RunReport`, `RunReportAsync`, `GetAsyncResult`, `GetVariants`, `GetTextElements`, `SetTextElements`

### Transport Management (5 tools — Expert, opt-in)
`ListTransports`, `GetTransport`, `CreateTransport`, `ReleaseTransport`, `DeleteTransport`

### UI5/BSP Management (7 tools — Focused read-only + Expert write)
`UI5ListApps`, `UI5GetApp`, `UI5GetFileContent` (Focused) + `UI5UploadFile`, `UI5DeleteFile`, `UI5CreateApp`, `UI5DeleteApp` (Expert)

### AMDP/HANA Debugger (7 tools — Expert, experimental)
`AMDPDebuggerStart`, `AMDPDebuggerResume`, `AMDPDebuggerStop`, `AMDPDebuggerStep`, `AMDPGetVariables`, `AMDPSetBreakpoint`, `AMDPGetBreakpoints`

### ExecuteABAP (1 tool — Expert)
`ExecuteABAP` — run arbitrary ABAP code via unit test wrapper

### Install/Setup (3 tools — Focused)
`InstallZADTVSP`, `InstallAbapGit`, `ListDependencies`

### Help (1 tool — Focused)
`GetAbapHelp` — ABAP keyword documentation from SAP system (requires ZADT_VSP)

---

## Disabling Tool Groups

Reduce token usage or restrict access by disabling groups:

```bash
# Disable UI5, Transport, and debugger tools
vsp --disabled-groups UCD
```

| Code | Group |
|------|-------|
| `U` or `5` | UI5/BSP |
| `T` | Unit tests |
| `H` | HANA/AMDP debugger |
| `D` | ABAP debugger |
| `C` | CTS/Transport |
| `G` | Git/abapGit |
| `R` | Report execution |
| `I` | Install tools |
| `X` | Experimental |

---

## Token Budget Reference

| Mode | Tool definitions | Typical workflow |
|------|-----------------|------------------|
| Focused | ~2,800 tokens | ~40% of Expert |
| Expert | ~8,000 tokens | Baseline |

→ See [Tool Reference](reference.md) for full descriptions of all 122 tools.
