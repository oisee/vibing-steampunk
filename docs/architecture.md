# vsp Architecture

## High-Level Architecture

```mermaid
flowchart TB
    subgraph Clients["MCP Clients"]
        CC[Claude Code]
        CD[Claude Desktop]
        Other[Other MCP Clients]
    end

    subgraph VSP["vsp - Go Binary"]
        direction TB

        subgraph Entry["Entry Points"]
            MCP[MCP Server<br/>JSON-RPC / stdio]
            CLI[CLI Mode<br/>search · source · export · debug]
            LUA[Lua Scripting<br/>REPL · Scripts]
        end

        subgraph Core["internal/mcp/server.go"]
            direction LR
            Focused[Focused Mode<br/>54 Tools]
            Expert[Expert Mode<br/>99 Tools]
        end

        subgraph Safety["Safety Layer"]
            RO[Read-Only Mode]
            PF[Package Filter]
            TF[Transport Filter]
            OF[Operation Filter]
            TE[Transportable<br/>Edit Guard]
        end

        subgraph ADTLib["pkg/adt/ — ADT Client Library"]
            direction TB
            subgraph Read["Read"]
                client[client.go<br/>Search · Get*]
                cds[cds.go<br/>CDS Dependencies]
            end
            subgraph Write["Write"]
                crud[crud.go<br/>Lock · Create · Update · Delete]
                workflows[workflows.go<br/>GetSource · WriteSource · Grep*]
            end
            subgraph DevTools["DevTools"]
                devtools[devtools.go<br/>Syntax · Activate · Tests · ATC]
                codeintel[codeintel.go<br/>FindDef · FindRefs · Completion]
            end
            subgraph Debug["Debugger"]
                dbg[debugger.go<br/>Breakpoints · Listen · Attach · Step]
                amdp[amdp_debugger.go<br/>HANA SQLScript Debug]
            end
            subgraph Extras["Extras"]
                ui5[ui5.go<br/>UI5/BSP Apps]
                features[features.go<br/>System Probing]
            end
        end

        subgraph Transport["Transport Layer"]
            HTTP[http.go<br/>CSRF · Sessions · Auth]
            WS[WebSocket Client<br/>ZADT_VSP APC]
        end

        subgraph Packages["Supporting Packages"]
            DSL[pkg/dsl/<br/>Fluent API · YAML Workflows]
            Cache[pkg/cache/<br/>Memory · SQLite]
            Script[pkg/scripting/<br/>Lua VM · Bindings]
        end

        subgraph Embedded["Embedded Assets"]
            ABAP[embedded/abap/<br/>ZADT_VSP Source]
            Deps[embedded/deps/<br/>abapGit ZIPs]
        end
    end

    subgraph SAP["SAP System"]
        ADT[ADT REST API<br/>/sap/bc/adt/*]
        APC[ZADT_VSP<br/>WebSocket APC]
        HANA[HANA DB<br/>AMDP Debug]
    end

    CC & CD & Other <-->|JSON-RPC / stdio| MCP
    CLI --> Core
    LUA --> Core
    MCP --> Core
    Core --> Safety
    Safety --> ADTLib
    ADTLib --> Transport
    HTTP <-->|HTTPS| ADT
    WS <-->|WebSocket| APC
    amdp <-->|WebSocket| HANA
    DSL --> ADTLib
    Script --> ADTLib
```

## Request Flow

```mermaid
sequenceDiagram
    participant Client as MCP Client
    participant Server as MCP Server
    participant Safety as Safety Layer
    participant ADT as ADT Client
    participant HTTP as HTTP Transport
    participant SAP as SAP System

    Client->>Server: Tool Call (JSON-RPC)
    Server->>Safety: Check permissions

    alt Blocked
        Safety-->>Server: Denied (read-only / package / operation)
        Server-->>Client: Error result
    else Allowed
        Safety->>ADT: Execute operation
        ADT->>HTTP: HTTP request
        HTTP->>HTTP: Add CSRF token + cookies
        HTTP->>SAP: HTTPS / WebSocket
        SAP-->>HTTP: Response
        HTTP-->>ADT: Parsed response
        ADT-->>Server: Result
        Server-->>Client: Tool result (JSON)
    end
```

## Write Operation Flow (EditSource)

```mermaid
sequenceDiagram
    participant AI as AI Assistant
    participant VSP as vsp
    participant SAP as SAP System

    AI->>VSP: EditSource(url, old_string, new_string)

    VSP->>SAP: GET source
    SAP-->>VSP: Current source code

    VSP->>VSP: Find & replace (uniqueness check)

    VSP->>SAP: POST syntax check
    SAP-->>VSP: OK / Errors

    alt Syntax Errors
        VSP-->>AI: Error (no changes saved)
    else Syntax OK
        VSP->>SAP: POST lock
        VSP->>SAP: PUT source
        VSP->>SAP: POST unlock
        VSP->>SAP: POST activate
        VSP-->>AI: Success
    end
```

## Tool Categories

```mermaid
flowchart LR
    subgraph Search["Search (3)"]
        SO[SearchObject]
        GO[GrepObjects]
        GP[GrepPackages]
    end

    subgraph Read["Read (10)"]
        GS[GetSource]
        GT[GetTable]
        GTC[GetTableContents]
        RQ[RunQuery]
        GPk[GetPackage]
        GFG[GetFunctionGroup]
        GCD[GetCDSDependencies]
        GCI[GetClassInfo]
        GMs[GetMessages]
        CS[CompareSource]
    end

    subgraph Write["Write (5)"]
        WS[WriteSource]
        ES[EditSource]
        IF[ImportFromFile]
        EF[ExportToFile]
        MO[MoveObject]
    end

    subgraph Dev["Dev (5)"]
        SC[SyntaxCheck]
        UT[RunUnitTests]
        ATC[RunATCCheck]
        LO[LockObject]
        UO[UnlockObject]
    end

    subgraph Intel["Intelligence (2)"]
        FD[FindDefinition]
        FR[FindReferences]
    end

    subgraph Debug["Debugger (6)"]
        DL[Listen]
        DA[Attach]
        DD[Detach]
        DS[Step]
        DGS[GetStack]
        DGV[GetVariables]
    end

    subgraph System["System (5)"]
        SI[GetSystemInfo]
        IC[GetInstalledComponents]
        CG[GetCallGraph]
        OS[GetObjectStructure]
        GF[GetFeatures]
    end

    subgraph Diag["Diagnostics (6)"]
        LD[ListDumps]
        GD[GetDump]
        LT[ListTraces]
        GTr[GetTrace]
        STS[GetSQLTraceState]
        LST[ListSQLTraces]
    end

    subgraph Git["Git (2)"]
        GiT[GitTypes]
        GiE[GitExport]
    end

    subgraph Reports["Reports (4)"]
        RR[RunReport]
        GV[GetVariants]
        GTE[GetTextElements]
        STE[SetTextElements]
    end

    subgraph Install["Install (3)"]
        IV[InstallZADTVSP]
        IA[InstallAbapGit]
        LDp[ListDependencies]
    end
```

## Dual Transport: HTTP + WebSocket

```mermaid
flowchart LR
    subgraph VSP["vsp"]
        HTTP[HTTP Client<br/>pkg/adt/http.go]
        WS[WebSocket Client<br/>pkg/adt/websocket.go]
    end

    subgraph SAP["SAP System"]
        ADT[ADT REST API<br/>/sap/bc/adt/*]
        APC[ZADT_VSP APC Handler<br/>/sap/bc/apc/ws/zadt_vsp]
    end

    HTTP -->|"CRUD · Search · Read<br/>Syntax · Activate · Debug"| ADT
    WS -->|"RFC Calls · Breakpoints<br/>Git Export · Reports<br/>AMDP Debug"| APC

    subgraph WSServices["WebSocket Domains"]
        direction TB
        RFC[rfc — Function Calls]
        BRK[debug — Breakpoints]
        GIT[git — abapGit Export]
        RPT[report — Report Execution]
        HLP[help — ABAP Documentation]
    end

    APC --- WSServices
```

## Package Structure

```
vibing-steampunk/
├── cmd/vsp/                    # CLI entry point (cobra/viper)
│   └── main.go                 #   Flags, env vars, auth, server startup
│
├── internal/mcp/               # MCP protocol layer
│   └── server.go               #   99 tool handlers, mode-aware registration
│
├── pkg/adt/                    # ADT client library (core)
│   ├── client.go               #   Read operations + search
│   ├── crud.go                 #   Lock / create / update / delete
│   ├── devtools.go             #   Syntax check, activate, unit tests, ATC
│   ├── codeintel.go            #   Find definition, references, completion
│   ├── workflows.go            #   High-level: GetSource, WriteSource, Grep*
│   ├── debugger.go             #   External ABAP debugger (HTTP + WebSocket)
│   ├── amdp_debugger.go        #   HANA/AMDP SQLScript debugger
│   ├── ui5.go                  #   UI5/Fiori BSP management
│   ├── cds.go                  #   CDS view dependency analysis
│   ├── safety.go               #   Read-only, package/op filtering
│   ├── features.go             #   System capability detection
│   ├── http.go                 #   HTTP transport (CSRF, sessions, auth)
│   └── xml.go                  #   ADT XML type definitions
│
├── pkg/dsl/                    # Fluent API & workflow engine
│   ├── search.go               #   Search builder
│   ├── test_runner.go          #   Unit test orchestration
│   ├── workflow.go             #   YAML workflow engine
│   └── batch.go                #   Batch import/export, pipelines
│
├── pkg/scripting/              # Lua scripting engine
│   ├── lua.go                  #   Lua VM, REPL
│   └── bindings.go             #   40+ ADT tool bindings
│
├── pkg/cache/                  # Caching infrastructure
│   ├── memory.go               #   In-memory cache
│   └── sqlite.go               #   SQLite persistent cache
│
├── embedded/                   # Assets embedded in binary
│   ├── abap/                   #   ZADT_VSP ABAP source files
│   └── deps/                   #   abapGit ZIP packages
│
└── docs/                       # Documentation
    ├── architecture.md         #   This file
    ├── DSL.md                  #   DSL & workflow guide
    └── adr/                    #   Architecture Decision Records
```

## Authentication

```mermaid
flowchart TD
    Start[Request] --> Auth{Auth Method?}

    Auth -->|Basic| Basic[Username + Password<br/>--user / --password]
    Auth -->|Cookie File| CFile[Netscape Format<br/>--cookie-file]
    Auth -->|Cookie String| CStr[Key=Value pairs<br/>--cookie-string]

    Basic --> CSRF[Fetch CSRF Token]
    CFile --> CSRF
    CStr --> CSRF

    CSRF --> Session[Stateful Session<br/>Cookie Jar]
    Session --> SAP[SAP ADT API]
```

## Safety System

```mermaid
flowchart TD
    Request[Tool Call] --> RO{Read-Only?}

    RO -->|Yes, Write Op| Block1[BLOCKED]
    RO -->|No / Read Op| SQL{Free SQL<br/>Blocked?}

    SQL -->|Yes, RunQuery| Block2[BLOCKED]
    SQL -->|No| Ops{Operation<br/>Allowed?}

    Ops -->|Disallowed| Block3[BLOCKED]
    Ops -->|Allowed| Pkg{Package<br/>Allowed?}

    Pkg -->|Outside whitelist| Block4[BLOCKED]
    Pkg -->|In whitelist| TE{Transportable<br/>Package?}

    TE -->|Yes, not enabled| Block5[BLOCKED]
    TE -->|No / Enabled| TR{Transport<br/>Allowed?}

    TR -->|Outside whitelist| Block6[BLOCKED]
    TR -->|In whitelist| OK[EXECUTE]
```
