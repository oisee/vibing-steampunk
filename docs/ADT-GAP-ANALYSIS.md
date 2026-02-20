# ADT API Gap Analysis — vibing-steampunk (vsp)

**Date:** 2026-02-21
**Current tools:** 103 (58 focused, 103 expert)
**ADT capabilities identified:** ~141
**Estimated coverage:** ~73% (expert mode)

---

## Part 1: What's Already Implemented (103 tools)

### System & Connection (4 tools)
| Tool | ADT Endpoint | Mode |
|------|-------------|------|
| GetConnectionInfo | Internal | Both |
| GetFeatures | Probes multiple endpoints | Both |
| GetSystemInfo | `/sap/bc/adt/system` | Both |
| GetInstalledComponents | `/sap/bc/adt/system/components` | Expert |

### Search (4 tools)
| Tool | ADT Endpoint | Mode |
|------|-------------|------|
| SearchObject | `/sap/bc/adt/repository/informationsystem/search` | Focused |
| SourceSearch | `/sap/bc/adt/repository/informationsystem/textsearch` (SRIS/HANA) | Expert |
| GrepObjects | Client-side source pattern matching | Focused |
| GrepPackages | Client-side source pattern matching | Expert |

### Source Read (11 tools)
| Tool | ADT Endpoint | Mode |
|------|-------------|------|
| GetSource | Unified: PROG, CLAS, INTF, FUNC, INCL, DDLS, BDEF, SRVD | Focused |
| GetSourceAndRevisions | Source + revision feed | Expert |
| GetProgram | `/sap/bc/adt/programs/programs/{name}` | Focused |
| GetClass | `/sap/bc/adt/oo/classes/{name}` | Focused |
| GetInterface | `/sap/bc/adt/oo/interfaces/{name}` | Focused |
| GetFunction | `/sap/bc/adt/functions/groups/{grp}/fmodules/{name}` | Focused |
| GetFunctionGroup | `/sap/bc/adt/functions/groups/{name}` | Focused |
| GetInclude | `/sap/bc/adt/programs/includes/{name}` | Focused |
| GetClassInclude | testclasses, locals_def, locals_imp, macros | Expert |
| GetDDLS | `/sap/bc/adt/ddic/ddl/sources/{name}` | Expert |
| GetBDEF | `/sap/bc/adt/bo/behaviordefinitions/{name}` | Expert |
| GetSRVD | `/sap/bc/adt/ddic/srvd/sources/{name}` | Expert |

### Metadata Read (10 tools)
| Tool | ADT Endpoint | Mode |
|------|-------------|------|
| GetPackage | `/sap/bc/adt/packages/{name}` | Focused |
| GetTable | `/sap/bc/adt/ddic/tables/{name}` | Focused |
| GetTableContents | `/sap/bc/adt/datapreview/ddic` | Focused |
| GetStructure | `/sap/bc/adt/ddic/structures/{name}` | Expert |
| GetTypeInfo | `/sap/bc/adt/ddic/types/{name}` | Expert |
| GetTransaction | `/sap/bc/adt/transactions/{name}` | Expert |
| GetTextElements | `/sap/bc/adt/programs/programs/{name}/textElements` | Expert |
| GetMessages | `/sap/bc/adt/ddic/messageclasses/{name}` | Expert |
| GetRevisions | `.../source/main/versions` (Atom feed) | Expert |
| GetRevisionSource | Version URI from revision feed | Expert |

### SQL Query (1 tool)
| Tool | ADT Endpoint | Mode |
|------|-------------|------|
| RunQuery | `/sap/bc/adt/datapreview/ddic` (POST, free SQL) | Expert |

### Code Intelligence (6 tools)
| Tool | ADT Endpoint | Mode |
|------|-------------|------|
| FindDefinition | `/sap/bc/adt/navigation/target` | Expert |
| FindReferences | `/sap/bc/adt/repository/informationsystem/usageReferences` | Expert |
| CodeCompletion | `/sap/bc/adt/abapsource/codecompletion/proposal` | Expert |
| GetClassComponents | Class metadata + structure | Expert |
| GetClassInfo | `/sap/bc/adt/oo/classes/{name}` | Expert |
| GetObjectStructure | `/sap/bc/adt/{type}/{name}/objectstructure` | Expert |

### Development Tools (6 tools)
| Tool | ADT Endpoint | Mode |
|------|-------------|------|
| SyntaxCheck | `/sap/bc/adt/checkruns` | Focused |
| Activate | `/sap/bc/adt/activation` | Focused |
| ActivatePackage | `/sap/bc/adt/activation` (batch) | Focused |
| RunUnitTests | `/sap/bc/adt/abapunit/testruns` | Focused |
| GetInactiveObjects | `/sap/bc/adt/activation/inactiveobjects` | Expert |
| PrettyPrint | `/sap/bc/adt/abapsource/prettyprinter` | Expert |

### ATC / Code Quality (3 tools)
| Tool | ADT Endpoint | Mode |
|------|-------------|------|
| RunATCCheck | `/sap/bc/adt/atc/runs` | Expert |
| GetATCCustomizing | `/sap/bc/adt/atc/customizing` | Expert |
| GetPrettyPrinterSettings | `/sap/bc/adt/abapsource/prettyprinterSettings` | Expert |

### CRUD Operations (12 tools)
| Tool | ADT Endpoint | Mode |
|------|-------------|------|
| LockObject | `?_action=LOCK` | Focused |
| UnlockObject | `?_action=UNLOCK` | Focused |
| CreateObject | POST to object type URL | Focused |
| UpdateSource | PUT to source URL | Focused |
| DeleteObject | `?_action=DELETE` | Expert |
| WriteSource | Composite: lock → update → unlock → activate | Focused |
| WriteProgram | Composite workflow | Focused |
| WriteClass | Composite workflow | Focused |
| CreateAndActivateProgram | Composite: create + activate | Expert |
| CreateClassWithTests | Composite: class + test include | Expert |
| CreateTable | `/sap/bc/adt/ddic/tables` | Expert |
| CreatePackage | `/sap/bc/adt/packages` | Expert |
| CreateTestInclude | Class includes POST | Expert |

### Analysis & Call Graph (7 tools)
| Tool | ADT Endpoint | Mode |
|------|-------------|------|
| GetCallGraph | `/sap/bc/adt/codeflow/call_hierarchy` | Expert |
| GetCalleesOf | Call hierarchy (down) | Expert |
| GetCallersOf | Call hierarchy (up) | Expert |
| AnalyzeCallGraph | Call hierarchy + statistics | Expert |
| CompareCallGraphs | Static vs actual trace | Expert |
| TraceExecution | Static + execution trace | Expert |
| CompareSource | Client-side LCS diff | Focused |
| CompareVersions | Version diff via revision URIs | Expert |
| GetCDSDependencies | `/sap/bc/adt/ddic/ddl/sources/{name}/dependency` | Expert |

### Debugging — ABAP External (9+ tools, group D)
| Tool | Mechanism | Mode |
|------|-----------|------|
| SetBreakpoint | ZADT_VSP WebSocket | Expert |
| GetBreakpoints | ZADT_VSP WebSocket | Expert |
| DeleteBreakpoint | ZADT_VSP WebSocket | Expert |
| DebuggerListen | HTTP long-poll → WebSocket | Expert |
| DebuggerAttach | WebSocket session | Expert |
| DebuggerDetach | WebSocket close | Expert |
| DebuggerStep | WebSocket (into/over/return/continue) | Expert |
| DebuggerGetStack | WebSocket | Expert |
| DebuggerGetVariables | WebSocket | Expert |

### Debugging — AMDP/HANA (7 tools, group H)
| Tool | Mechanism | Mode |
|------|-----------|------|
| AMDPDebuggerStart | ZADT_VSP WebSocket | Expert |
| AMDPDebuggerStop | WebSocket close | Expert |
| AMDPDebuggerStep | WebSocket | Expert |
| AMDPDebuggerResume | WebSocket | Expert |
| AMDPGetVariables | WebSocket | Expert |
| AMDPSetBreakpoint | WebSocket | Expert |
| AMDPGetBreakpoints | WebSocket | Expert |

### Runtime Analysis (6 tools)
| Tool | ADT Endpoint | Mode |
|------|-------------|------|
| ListDumps | `/sap/bc/adt/runtime/dumps` | Expert |
| GetDump | `/sap/bc/adt/runtime/dumps/{id}` | Expert |
| ListTraces | `/sap/bc/adt/profiler/traces` | Expert |
| GetTrace | `/sap/bc/adt/profiler/traces/{id}/analysis` | Expert |
| GetSQLTraceState | `/sap/bc/adt/sqltrace/state` | Expert |
| ListSQLTraces | `/sap/bc/adt/sqltrace/traces` | Expert |

### Transport Management (7 tools, group C)
| Tool | ADT Endpoint | Mode |
|------|-------------|------|
| ListTransports | `/sap/bc/adt/cts/transportrequests` + SQL fallback | Expert |
| GetUserTransports | `/sap/bc/adt/cts/transportrequests?user=...` | Expert |
| GetTransport | `/sap/bc/adt/cts/transportorganizer/...` | Expert |
| GetTransportInfo | `/sap/bc/adt/cts/transportchecks` | Expert |
| CreateTransport | `/sap/bc/adt/cts/transportorganizer` | Expert |
| DeleteTransport | `/sap/bc/adt/cts/transportorganizer/{id}` | Expert |
| ReleaseTransport | `...?_action=release` | Expert |

### UI5/Fiori BSP (7 tools, group 5/U)
| Tool | ADT Endpoint | Mode |
|------|-------------|------|
| UI5ListApps | `/sap/bc/adt/filestore/ui5-bsp/objects` | Expert |
| UI5GetApp | `.../objects/{name}/content` | Expert |
| UI5GetFileContent | `.../objects/{name}/{path}/content` | Expert |
| UI5UploadFile | PUT to file path | Expert |
| UI5DeleteFile | DELETE file | Expert |
| UI5CreateApp | POST create app | Expert |
| UI5DeleteApp | DELETE app | Expert |

### RAP / Service Binding (2 tools)
| Tool | ADT Endpoint | Mode |
|------|-------------|------|
| PublishServiceBinding | `?_action=publish` | Expert |
| UnpublishServiceBinding | `?_action=unpublish` | Expert |

### abapGit Integration (3 tools, group G)
| Tool | Mechanism | Mode |
|------|-----------|------|
| GitExport | ZADT_VSP WebSocket → ZIP to disk | Expert |
| GitTypes | ZADT_VSP WebSocket (158 types) | Expert |
| InstallAbapGit | Embedded ZIP deployment | Expert |

### Report Execution (5 tools, group R)
| Tool | Mechanism | Mode |
|------|-----------|------|
| RunReport | ZADT_VSP WebSocket | Expert |
| RunReportAsync | `/sap/bc/adt/batch/reports` | Expert |
| GetAsyncResult | `/sap/bc/adt/batch/tasks/{id}` | Expert |
| GetVariants | `/sap/bc/adt/abapunit/variants` | Expert |
| SetTextElements | PUT text elements | Expert |

### Installation (3 tools, group I)
| Tool | Mechanism | Mode |
|------|-----------|------|
| InstallZADTVSP | Embedded ZIP | Expert |
| InstallAbapGit | Embedded ZIP | Expert |
| ListDependencies | Internal | Expert |

### Other
| Tool | Purpose | Mode |
|------|---------|------|
| GetAbapHelp | ABAP keyword docs (discovery + WebSocket L2) | Both |
| SetPrettyPrinterSettings | PP config write | Expert |
| ExecuteABAP | Code exec via unit test wrapper | Expert |
| RenameObject | Composite rename (GetSource→Create→Delete) | Expert |
| MoveObject | ZADT_VSP WebSocket (TADIR) | Expert |
| CloneObject | Source copy to new name | Expert |
| SaveToFile / ExportToFile / ImportFromFile / DeployFromFile | Local file I/O | Expert |
| GetTypeHierarchy | `/sap/bc/adt/typeHierarchy` (supertypes/subtypes) | Expert |
| DebuggerSetVariableValue | Modify variable during debug (debugger.go + Lua) | Expert |
| GetView | `/sap/bc/adt/ddic/views/{name}` (DDIC view read) | Expert |

---

## Part 2: What's NOT Implemented (Gaps)

> **Note (2026-02-21 update):** Cross-checking revealed several false positives. Items marked ~~strikethrough~~ are already implemented. See [spike report](spikes/2026-02-21-adt-gaps-deep-research.md) for details.

### Corrections from code verification
- **ATC Worklist** → already in `RunATCCheck()` (devtools.go). Real gap: ATC QuickFix.
- **GetTypeHierarchy** → already in `codeintel.go:554`. Remove from gaps.
- **Debug Set Variable** → already in `debugger.go:1113`. Remove from gaps.
- **DDIC View read** → already in `client.go:617` (GetView). Remove from gaps.
- **RenameObject** → exists as composite workflow (`workflows.go`), but NOT ADT-native refactoring (evaluate→preview→execute). The ADT-native version is the real gap.
- **QuickFix metadata** → ATC findings parse `quickfixInfo` field, but no standalone QuickFix tool.
- **CodeCompletion** → already in `codeintel.go`. Remove from gaps.

### Tier 1 — High Value, Reasonable Effort

| # | Feature | ADT Endpoint | Value | Effort | Notes |
|---|---------|-------------|-------|--------|-------|
| 1 | **Refactoring: Rename** | `/sap/bc/adt/refactoring/rename` (evaluate → preview → execute) | HIGH | 5 | ADT-native rename across all references. Three-step flow. |
| 2 | **Refactoring: Extract Method** | `/sap/bc/adt/refactoring/extractmethod` | HIGH | 6 | Extract code block into new method. Three-step flow. |
| 3 | **Quick Fix Proposals** | `/sap/bc/adt/quickfix/proposals` | HIGH | 4 | Get suggested fixes for syntax/ATC issues. AI could auto-apply. |
| 4 | **Apply Quick Fix** | `/sap/bc/adt/quickfix/apply` | HIGH | 5 | Automatically apply proposed fixes. |
| ~~5~~ | ~~**ATC Worklist**~~ | ~~`/sap/bc/adt/atc/worklists/{id}`~~ | — | — | **ALREADY IMPLEMENTED** in `RunATCCheck()`. Real gap: ATC QuickFix details. |
| 5 | **ATC QuickFix Details** | `/sap/bc/adt/atc/quickfix/{findingId}` | HIGH | 3 | Fetch and apply auto-fixes for ATC findings. |
| 6 | **Coverage Analysis** | `/sap/bc/adt/abapunit/testruns` (with coverage flag) | HIGH | 4 | Unit test code coverage markers. Helps identify untested code. |
| 7 | **SQL Explain Plan** | `/sap/bc/adt/datapreview/sqlexplainplan` | MEDIUM | 5 | Query execution plan for performance optimization. HANA only. |
| 8 | **Object Change Feed** | `/sap/bc/adt/feed/objects` | MEDIUM | 3 | Track recent changes to objects — useful for AI context. Unconfirmed endpoint. |
| 9 | **User Activity Feed** | `/sap/bc/adt/feed/users/{user}` | MEDIUM | 3 | Who changed what recently. Unconfirmed endpoint. |

### Tier 2 — CDS/RAP Ecosystem Gaps

| # | Feature | ADT Endpoint | Value | Effort | Notes |
|---|---------|-------------|-------|--------|-------|
| 10 | **Metadata Extension** read/write | `/sap/bc/adt/ddic/cds/metadataextensions/{name}` | MEDIUM | 3 | CDS annotation extensions (UI5 annotations). |
| 11 | **Access Control (DCL)** read/write | `/sap/bc/adt/ddic/cds/accesscontrols/{name}` | MEDIUM | 3 | CDS row-level security definitions. |
| 12 | **CDS Element Info** | `/sap/bc/adt/ddic/cds/elementinfo` | MEDIUM | 3 | Field-level metadata for CDS views. |
| 13 | **CDS Impact Analysis** | `/sap/bc/adt/ddic/cds/impactanalysis` | MEDIUM | 4 | What breaks if a CDS view changes. |
| 14 | **CDS Annotation Value Help** | `/sap/bc/adt/ddic/cds/annotationvaluehelp` | LOW | 4 | Suggest valid annotation values. |
| 15 | **Behavior Definition** direct write | `/sap/bc/adt/bo/behaviordefinitions/{name}` | MEDIUM | 3 | Already can read (GetBDEF), write via WriteSource; dedicated write would be cleaner. |

### Tier 3 — Transport & Lifecycle Gaps

| # | Feature | ADT Endpoint | Value | Effort | Notes |
|---|---------|-------------|-------|--------|-------|
| 16 | **Add to Transport** | `?_action=checkintotr` or `?_action=assigntotr` | MEDIUM | 3 | Explicitly add object to transport without editing it. |
| 17 | **Set Transport Owner** | CTS API | LOW | 3 | Change request owner. |
| 18 | **Add User to Transport** | CTS API | LOW | 3 | Add task user. |
| 19 | **Transport Configs** | CTS search configs | LOW | 3 | Query transport configurations. |

### Tier 4 — DDIC Object Types

| # | Feature | ADT Endpoint | Value | Effort | Notes |
|---|---------|-------------|-------|--------|-------|
| ~~20~~ | ~~**DDIC View** read~~ | — | — | — | **ALREADY IMPLEMENTED** as `GetView()` in `client.go:617`. |
| 21 | **Search Help** read | `/sap/bc/adt/ddic/searchhelps/{name}` | LOW | 2 | F4 help definitions. |
| 22 | **Lock Object** read | `/sap/bc/adt/ddic/lockobjects/{name}` | LOW | 2 | Enqueue objects. |
| 23 | **Type Group** read | `/sap/bc/adt/ddic/typegroups/{name}` | LOW | 2 | Type-pools (legacy). |

### Tier 5 — Nice to Have

| # | Feature | ADT Endpoint | Value | Effort | Notes |
|---|---------|-------------|-------|--------|-------|
| 24 | **SQL Trace Details** | `/sap/bc/adt/sqltrace/traces/{id}` | LOW | 4 | Detailed ST05 trace data. |
| 25 | **Memory Analysis** | `/sap/bc/adt/runtime/memoryanalysis` | LOW | 5 | Heap/memory profiling. |
| 26 | **ADT Discovery** full | `/sap/bc/adt/discovery` | LOW | 3 | Full API discovery document parse. |
| 27 | **Reentrance Ticket** | SSO ticket endpoint | LOW | 4 | SSO integration. |
| 28 | **abapGit Pull/Push** (via ADT) | abapGit ADT plugin endpoints | MEDIUM | 6 | Native abapGit operations. Currently via ZADT_VSP WebSocket. |
| ~~29~~ | ~~**Set Variable in Debug**~~ | — | — | — | **ALREADY IMPLEMENTED** as `DebuggerSetVariableValue()` in `debugger.go:1113`. |
| 30 | **Transport Change Feed** | `/sap/bc/adt/feed/transports` | LOW | 3 | Track transport changes. |

---

## Part 3: Priority Recommendations

### Immediate Impact (implement next)

1. **ATC Worklist** (gap #5) — we already run ATC checks but can't read the detailed findings. This is a missing "second half" of an existing feature. Effort: 3.

2. **Refactoring: Rename** (gap #1) — AI-powered rename across all references. Three ADT calls (evaluate → preview → execute). Effort: 5.

3. **Quick Fix Proposals + Apply** (gaps #3-4) — AI reads issues, ADT suggests fixes, AI applies them. Huge automation potential. Effort: 4+5.

4. **Coverage Analysis** (gap #6) — extend RunUnitTests to request coverage data. Shows which lines are tested. Effort: 4.

### Medium-Term (CDS/RAP completeness)

5. **Metadata Extension** (gap #10) — needed for full RAP development pipeline.
6. **Access Control / DCL** (gap #11) — row-level security is critical for RAP.
7. **CDS Impact Analysis** (gap #13) — "what breaks if I change this CDS view?"

### Low Priority (fill the gaps)

8. DDIC views, search helps, lock objects, type groups (gaps #20-23) — 2 effort each, mostly legacy objects.
9. Feeds (gaps #8-9, #30) — nice for monitoring, not critical.
10. SQL Explain Plan (gap #7) — useful for optimization, HANA-only.

---

## Part 4: Coverage Summary

| Category | Implemented | Total ADT | Coverage |
|----------|------------|-----------|----------|
| System & Connection | 4 | 5 | 80% |
| Search | 4 | 4 | 100% |
| Source Read | 12 | 16 | 75% |
| Metadata Read | 10 | 14 | 71% |
| SQL / Data Query | 2 | 3 | 67% |
| Code Intelligence | 6 | 8 | 75% |
| Development Tools | 6 | 6 | 100% |
| ATC / Quality | 3 | 6 | 50% |
| CRUD Operations | 12 | 12 | 100% |
| Refactoring | 2 (RenameObject composite + TypeHierarchy) | 8 | 25% |
| Analysis & Call Graph | 9 | 10 | 90% |
| Debugging (ABAP) | 9 | 9 | 100% |
| Debugging (AMDP) | 7 | 7 | 100% |
| Runtime Analysis | 6 | 8 | 75% |
| Transport Management | 7 | 11 | 64% |
| UI5/Fiori BSP | 7 | 7 | 100% |
| RAP / OData | 2 | 5 | 40% |
| abapGit | 3 | 9 | 33% |
| Report Execution | 5 | 5 | 100% |
| Installation | 3 | 3 | 100% |
| Version History | 3 | 3 | 100% |
| Documentation | 2 | 3 | 67% |
| Formatting | 3 | 3 | 100% |
| **TOTAL** | **~103** | **~141** | **~73%** |

### Biggest Gaps by Category

1. **Refactoring** — 25% (RenameObject composite + TypeHierarchy, no ADT-native refactoring or Extract Method)
2. **abapGit** — 33% (export works, no pull/push/stage/branch via ADT plugin — but WebSocket approach is preferred)
3. **RAP / OData** — 40% (publish/unpublish, missing CDS extensions — BDEF write actually works via WriteSource)
4. **ATC / Quality** — 67% (run checks + read worklist works; missing ATC QuickFix and Coverage)
5. **Transport Management** — 64% (core ops, missing assign-to-transport and owner mgmt)

---

## Notes

- The capability matrix in `reports/adt-capability-matrix.md` was written Dec 2025 when vsp had ~16 tools. **It's outdated** — coverage has grown from 11% to 73%.
- Some "missing" features may be partially available via ZADT_VSP WebSocket (custom ABAP handler) even if the native ADT REST endpoint isn't implemented.
- UI5 write operations have known issues — ADT filestore returns 405 on some systems. vsp implements them but they may not work everywhere.
- Refactoring is the biggest gap with highest potential value for AI-assisted development.
