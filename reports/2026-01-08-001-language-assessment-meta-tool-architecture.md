# Language Assessment & Meta-Tool Architecture

**Date:** 2026-01-08
**Report ID:** 001
**Subject:** Multi-language strategy and token-efficient tool architecture
**Related Documents:** CLAUDE.md, focused-mode-proposal.md

---

## Executive Summary

This report analyzes programming language options for extending VSP infrastructure and proposes a meta-tool architecture to reduce MCP token consumption by ~85%. The recommendation is to keep Go as the core, add TypeScript for Claude Code skills integration, and implement meta-tools that bundle atomic operations.

---

## Part 1: Language Deep Dive

### Current State: Go

**What's Working Well:**
- Single binary distribution (critical for SAP environments with restricted package managers)
- 244 unit tests, 34 integration tests passing
- Excellent HTTP/WebSocket handling
- Goroutine-based concurrency (AMDP debugger uses this)
- Low memory footprint (~20MB runtime)
- Cross-compilation to 9 platforms

**Pain Points:**
- Verbose error handling (`if err != nil` everywhere)
- Limited metaprogramming (can't generate tools dynamically at compile time)
- Added Lua for scripting/REPL (additional complexity)
- No native REPL for interactive development

---

### TypeScript/Bun - Detailed Analysis

#### Pros

**1. Native MCP Ecosystem**
```typescript
// Anthropic's MCP SDK is TypeScript-first
import { Server } from "@modelcontextprotocol/sdk/server";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio";

const server = new Server({
  name: "vsp-ts",
  version: "1.0.0"
}, {
  capabilities: { tools: {} }
});

// Tool registration is natural
server.setRequestHandler(ListToolsRequestSchema, async () => ({
  tools: toolRegistry.getActiveTools()
}));
```

**2. Dynamic Tool Registration**
```typescript
// Load tools on-demand based on context
class DynamicToolRegistry {
  private loadedGroups: Set<string> = new Set();

  async loadGroup(group: ToolGroup): Promise<void> {
    if (this.loadedGroups.has(group)) return;

    // Lazy import - only loads when needed
    const module = await import(`./tools/${group}.js`);
    module.tools.forEach(t => this.register(t));
    this.loadedGroups.add(group);
  }

  // Context-aware loading
  async prepareForTask(taskDescription: string): Promise<void> {
    const groups = this.inferGroups(taskDescription);
    await Promise.all(groups.map(g => this.loadGroup(g)));
  }
}
```

**3. Bun Single-File Executables**
```bash
# Compile to native binary (like Go)
bun build --compile --minify ./src/index.ts --outfile vsp-ts

# Result: ~50MB standalone executable (no Node.js required)
```

**4. JSON Schema Generation**
```typescript
// Valibot: functional pipe-based API, composable validation
import * as v from "valibot";
import { toJsonSchema } from "@valibot/to-json-schema";

const GetSourceSchema = v.object({
  object_type: v.picklist(["PROG", "CLAS", "INTF", "FUNC"]),
  name: v.pipe(v.string(), v.minLength(1), v.maxLength(30)),
  method: v.optional(v.string())
});

// Auto-generate MCP tool schema
const toolSchema = toJsonSchema(GetSourceSchema);
```

**5. Claude Code Skills Integration**
```typescript
// skills/develop.ts - Native skill definition
export const developSkill: Skill = {
  name: "develop",
  description: "ABAP development workflow",
  triggers: ["/develop", "/fix", "/implement"],

  async execute(context: SkillContext): Promise<void> {
    // Orchestrates multiple VSP tools internally
    const object = await context.call("SearchObject", { query: context.target });
    const source = await context.call("GetSource", { ...object });
    // ... workflow continues
  }
};
```

#### Cons

**1. Runtime Size**
- Bun executable: ~50MB vs Go's ~15MB
- Not critical, but notable for constrained environments

**2. Ecosystem Maturity**
- Bun is newer (v1.0 in Sept 2023)
- Some npm packages have Node.js-specific code
- WebSocket libraries may need testing

**3. SAP Integration Libraries**
- No native RFC library (Go doesn't have one either)
- HTTP/ADT works fine, but PyRFC is Python-only

#### Implementation Strategy

```
┌─────────────────────────────────────────────────────────────┐
│                    Claude Code                               │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │  /develop    │  │   /debug     │  │   /deploy    │      │
│  │   skill      │  │    skill     │  │    skill     │      │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘      │
│         │                 │                 │               │
│         └─────────────────┼─────────────────┘               │
│                           │                                  │
│                           ▼                                  │
│              ┌────────────────────────┐                     │
│              │   TypeScript Skill     │                     │
│              │      Orchestrator      │                     │
│              │  (Bun single binary)   │                     │
│              └────────────┬───────────┘                     │
│                           │                                  │
└───────────────────────────┼──────────────────────────────────┘
                            │ MCP calls
                            ▼
              ┌────────────────────────┐
              │      Go VSP Server     │
              │   (existing 99 tools)  │
              └────────────────────────┘
```

---

### Python - Detailed Analysis

#### Pros

**1. PyRFC - Native SAP Connectivity**
```python
from pyrfc import Connection

# Direct RFC calls (not available in Go/TS)
conn = Connection(ashost='sap.server.com', sysnr='00', client='100',
                  user='USER', passwd='PASSWORD')

# Call any function module directly
result = conn.call('BAPI_USER_GET_DETAIL', USERNAME='TESTUSER')
```

**2. Anthropic SDK First-Class Support**
```python
from anthropic import Anthropic
from mcp import Server, Tool

# MCP server with type hints
class VSPServer(Server):
    @tool(
        name="GetSource",
        description="Get ABAP source code",
        input_schema=GetSourceSchema
    )
    async def get_source(self, object_type: str, name: str) -> str:
        # Implementation
        pass
```

**3. Data Analysis Capabilities**
```python
import pandas as pd

# Analyze call graphs, test results, traces
def analyze_test_coverage(trace_data: dict, call_graph: dict) -> pd.DataFrame:
    static_edges = set(call_graph['edges'])
    actual_edges = set(trace_data['edges'])

    return pd.DataFrame({
        'path': list(static_edges | actual_edges),
        'static': [p in static_edges for p in (static_edges | actual_edges)],
        'executed': [p in actual_edges for p in (static_edges | actual_edges)]
    })
```

**4. Rapid Prototyping**
```python
# Quick experiments without compilation
async def experiment_with_debugger():
    async with VSPClient() as vsp:
        await vsp.set_breakpoint("ZTEST", 10)

        # Start listener in background
        listener = asyncio.create_task(vsp.debugger_listen(timeout=60))

        # Trigger execution
        await vsp.run_unit_tests("/sap/bc/adt/oo/classes/ZCL_TEST")

        # Attach when breakpoint hit
        debuggee = await listener
        await vsp.debugger_attach(debuggee.id)
```

**5. Jupyter Integration**
```python
# Interactive debugging notebooks
# In[1]:
%load_ext vsp_magic

# In[2]:
%%abap ZCL_TEST
METHOD test_calculation.
  DATA(result) = calculate( 10 ).
  cl_abap_unit_assert=>assert_equals( act = result exp = 100 ).
ENDMETHOD.

# In[3]:
%vsp run_tests ZCL_TEST
# Output: 1 test passed, 0 failed
```

#### Cons

**1. Distribution Complexity**
```bash
# Option A: PyInstaller (large, slow startup)
pyinstaller --onefile vsp.py  # ~100MB, 2-3s startup

# Option B: Docker (requires Docker)
docker run -it vsp-python --url http://sap:50000

# Option C: uv/pipx (requires Python)
uv tool install vsp
```

**2. Async Complexity**
```python
# Mixing sync SAP libraries with async MCP
async def get_source(name: str) -> str:
    # PyRFC is synchronous - need thread pool
    loop = asyncio.get_event_loop()
    result = await loop.run_in_executor(
        None,  # Default executor
        lambda: rfc_conn.call('RFC_READ_TABLE', ...)
    )
    return result
```

**3. Memory Usage**
```
Go VSP:      ~20MB resident
Python VSP:  ~80-150MB resident (interpreter + libraries)
```

#### Implementation Strategy

```python
# vsp_python/client.py - Wrapper around Go binary or direct HTTP

class VSPClient:
    """Hybrid client - calls Go binary or direct HTTP"""

    def __init__(self, mode: Literal["binary", "http"] = "http"):
        self.mode = mode
        if mode == "binary":
            self._proc = subprocess.Popen(["vsp", "--stdio"], ...)
        else:
            self._session = aiohttp.ClientSession()

    async def get_source(self, object_type: str, name: str) -> str:
        if self.mode == "binary":
            return await self._call_binary("GetSource", {...})
        else:
            return await self._call_http(f"/adt/{object_type}/{name}")


# vsp_python/notebooks/debug_session.ipynb
# Interactive debugging in Jupyter
```

---

### Other Languages - Limitations Analysis

#### Rust

**Strengths:**
- Memory safety, excellent performance
- WebAssembly target (browser-based VSP possible)
- Strong type system with exhaustive matching

**MCP Ecosystem (Updated Jan 2026):**
- `rmcp` v0.12.0 - **Official SDK** from `modelcontextprotocol/rust-sdk`
- `rust-mcp-sdk` v0.8.1 - Async SDK, 11K SLoC
- `mcp-protocol-sdk` v0.5.1 - Production-ready

**Limitations:**
- Compile times hurt iteration (2-5 min full rebuild vs Go's 5-10s)
- Learning curve for team adoption

**Verdict:** Now viable with official SDK. Worth considering for new projects, but rewrite cost may not justify benefits for existing Go codebase.

---

#### Java/Kotlin

**Strengths:**
- JCo (SAP Java Connector) - mature RFC library
- GraalVM native images for single binaries
- Excellent SAP CAP integration

**Limitations:**
- JVM startup time without GraalVM native (2-5s cold start)
- GraalVM native compilation slow (5-10 minutes)
- No official MCP SDK

**Verdict:** Good if you need JCo/CAP integration. Otherwise, overhead isn't justified.

---

#### Zig

**Strengths:**
- C interop (could wrap SAP NW RFC SDK)
- Tiny binaries (~2MB)

**Limitations:**
- No HTTP/JSON/WebSocket in stdlib—would build from scratch
- No async/await, manual state machines
- No MCP ecosystem

**Verdict:** Wrong tool. Zig is for systems programming, not HTTP API clients.

---

### Language Comparison Matrix

| Aspect | Go (current) | TypeScript | Python | Rust | Java |
|--------|--------------|------------|--------|------|------|
| **Single Binary** | ✅ 15MB | ✅ 50MB (Bun) | ⚠️ 100MB+ | ✅ 10MB | ⚠️ GraalVM |
| **MCP SDK** | ⚠️ Custom | ✅ Official | ✅ Good | ❌ | ❌ |
| **SAP RFC** | ❌ | ❌ | ✅ PyRFC | ⚠️ C FFI | ✅ JCo |
| **Async HTTP** | ✅ | ✅ | ✅ | ✅ | ✅ |
| **WebSocket** | ✅ | ✅ | ✅ | ⚠️ | ✅ |
| **Learning Curve** | Low | Low | Low | High | Medium |
| **Startup Time** | 10ms | 50ms | 500ms | 10ms | 2000ms* |
| **Memory** | 20MB | 50MB | 100MB | 15MB | 200MB |

*Without GraalVM native image

---

## Part 2: Token-Efficient Architecture

### Problem Statement

Current VSP exposes 95 tools to MCP (82 main + 13 aliases). Measured from actual context:

```
MCP tools total:     15.7k tokens (7.8% of context)
├─ VSP tools (95):   14.2k tokens (~150 tokens/tool average)
└─ Other MCP (4):     1.5k tokens (context7, exa)
```

MCP requires tool schemas in context. Reducing tools = reducing tokens.

### Solution Comparison

| Approach | Schema Tokens | Structured Output | Model Guidance |
|----------|---------------|-------------------|----------------|
| 95 atomic MCP tools | ~14,200 | ✅ Strong | ✅ Full |
| 6 meta-tools MCP | ~1,000 | ✅ Strong | ✅ Good |
| **1 universal tool** | **~150** | ✅ Strong | ⚠️ Action-based |
| Skills + Bash CLI | 0 | ❌ Text parsing | ⚠️ Prompt-based |

---

### Recommended: Single Universal Tool

One tool that routes to all 99 internal operations:

```json
{
  "name": "SAP",
  "description": "SAP ABAP development. Actions: read, edit, create, delete, search, grep, debug, query, analyze, test, deploy, system",
  "inputSchema": {
    "type": "object",
    "properties": {
      "action": {
        "type": "string",
        "description": "Operation: read|edit|create|delete|search|grep|debug|query|analyze|test|deploy|system"
      },
      "target": {
        "type": "string",
        "description": "Target object (e.g., 'CLAS ZCL_TEST', 'PROG ZREPORT', 'TABLE SFLIGHT')"
      },
      "params": {
        "type": "object",
        "description": "Action-specific parameters"
      }
    },
    "required": ["action"]
  }
}
```

**Token cost: ~150 tokens** (99.3% reduction)

#### Action Routing

| Action | Target Format | Params | Internal Tools |
|--------|---------------|--------|----------------|
| `read` | `TYPE NAME` | `method`, `include` | GetSource |
| `edit` | `TYPE NAME` | `old`, `new`, `method`, `activate` | EditSource, SyntaxCheck, Activate |
| `create` | `TYPE NAME` | `source`, `package`, `description` | WriteSource |
| `delete` | `TYPE NAME` | | DeleteObject |
| `search` | query string | `max_results` | SearchObject |
| `grep` | pattern | `packages`, `types` | GrepPackages, GrepObjects |
| `debug` | `setup\|run\|step\|inspect\|cleanup` | breakpoints, step_type, etc. | Debugger* tools |
| `query` | `table\|sql` | `table`, `sql`, `max_rows` | GetTable*, RunQuery |
| `analyze` | `TYPE NAME` | `direction`, `depth` | GetCallGraph, FindReferences |
| `test` | `TYPE NAME` or package | `include_dangerous`, `atc` | RunUnitTests, RunATCCheck |
| `deploy` | `export\|import\|activate` | paths, packages | GitExport, Import*, Activate* |
| `system` | `info\|features\|connection` | | GetSystemInfo, GetFeatures |

#### Example Calls

```javascript
// Read a class
{ action: "read", target: "CLAS ZCL_ORDER" }

// Read specific method
{ action: "read", target: "CLAS ZCL_ORDER", params: { method: "CALCULATE" } }

// Edit with auto-activate
{ action: "edit", target: "CLAS ZCL_ORDER", params: {
    method: "CALCULATE",
    old: "result = a + b.",
    new: "result = a * b.",
    activate: true
}}

// Search
{ action: "search", target: "ZCL_*ORDER*" }

// Debug setup
{ action: "debug", target: "setup", params: {
    breakpoints: [{ program: "ZCL_ORDER", method: "CALCULATE", line: 10 }]
}}

// Run query
{ action: "query", target: "sql", params: {
    sql: "SELECT * FROM sflight WHERE carrid = 'LH'"
}}
```

---

### Alternative: Multiple Meta-Tools

If more structured guidance is needed, use 6 domain-specific tools instead of 1:

```
┌─────────────────────────────────────────────────────────────────┐
│                     MCP Tool Interface (~1,800 tokens)          │
│                                                                 │
│  DevWorkflow │ DebugWorkflow │ QueryWorkflow                   │
│  AnalyzeWorkflow │ TestWorkflow │ DeployWorkflow               │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ Internal calls
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│            Internal Tool Registry (99 tools, not exposed)       │
└─────────────────────────────────────────────────────────────────┘
```

#### 1. DevWorkflow

**Combines:** SearchObject, GetSource, EditSource, WriteSource, SyntaxCheck, Activate, RunUnitTests

| Action | Internal Flow |
|--------|---------------|
| `read` | SearchObject (if query) → GetSource |
| `edit` | EditSource → SyntaxCheck → Activate → RunUnitTests |
| `create` | WriteSource → Activate |
| `delete` | DeleteObject |

```json
{
  "name": "DevWorkflow",
  "description": "Unified ABAP development: read, edit, create objects.",
  "inputSchema": {
    "properties": {
      "action": {"enum": ["read", "edit", "create", "delete"]},
      "object_type": {"enum": ["PROG", "CLAS", "INTF", "FUNC", "DDLS"]},
      "object_name": {"type": "string"},
      "search_query": {"type": "string"},
      "old_string": {"type": "string"},
      "new_string": {"type": "string"},
      "method": {"type": "string"},
      "source": {"type": "string"},
      "package": {"type": "string"},
      "activate": {"type": "boolean", "default": true},
      "run_tests": {"type": "boolean", "default": false}
    },
    "required": ["action"]
  }
}
```

---

#### 2. DebugWorkflow

**Combines:** SetBreakpoint, DeleteBreakpoint, GetBreakpoints, DebuggerListen, DebuggerAttach, DebuggerStep, DebuggerGetStack, DebuggerGetVariables, DebuggerDetach, RunUnitTests, CallRFC

| Action | Internal Flow |
|--------|---------------|
| `setup` | SetBreakpoint (multiple) |
| `run` | DebuggerListen + trigger (RunUnitTests or CallRFC) → DebuggerAttach |
| `step` | DebuggerStep → DebuggerGetStack |
| `inspect` | DebuggerGetVariables |
| `cleanup` | DebuggerDetach → DeleteBreakpoint (all) |

```json
{
  "name": "DebugWorkflow",
  "description": "ABAP debugging: setup breakpoints, run to hit, step, inspect, cleanup.",
  "inputSchema": {
    "properties": {
      "action": {"enum": ["setup", "run", "step", "inspect", "cleanup"]},
      "program": {"type": "string"},
      "line": {"type": "integer"},
      "method": {"type": "string"},
      "breakpoints": {"type": "array", "items": {"type": "object"}},
      "trigger_test": {"type": "boolean"},
      "trigger_rfc": {"type": "string"},
      "timeout": {"type": "integer", "default": 60},
      "step_type": {"enum": ["into", "over", "return", "continue"]},
      "variables": {"type": "array", "items": {"type": "string"}}
    },
    "required": ["action"]
  }
}
```

---

#### 3. QueryWorkflow

**Combines:** GetTable, GetTableContents, RunQuery, SearchObject, GrepPackages

| Action | Internal Flow |
|--------|---------------|
| `table_info` | GetTable |
| `select` | GetTableContents |
| `sql` | RunQuery |
| `search` | SearchObject or GrepPackages |

---

#### 4. AnalyzeWorkflow

**Combines:** GetCallGraph, GetCallersOf, GetCalleesOf, GetCDSDependencies, GetObjectStructure, FindReferences

| Action | Internal Flow |
|--------|---------------|
| `call_graph` | GetCallGraph (with direction) |
| `dependencies` | GetCDSDependencies |
| `where_used` | FindReferences |
| `structure` | GetObjectStructure |

---

#### 5. TestWorkflow

**Combines:** RunUnitTests, RunATCCheck, TraceExecution, CompareCallGraphs

| Action | Internal Flow |
|--------|---------------|
| `run` | RunUnitTests (single or batch) |
| `coverage` | TraceExecution → CompareCallGraphs |
| `atc` | RunATCCheck |

---

#### 6. DeployWorkflow

**Combines:** GitExport, ImportFromFile, ExportToFile, ActivatePackage, GetTransport, ListTransports

| Action | Internal Flow |
|--------|---------------|
| `export` | GitExport or ExportToFile |
| `import` | ImportFromFile |
| `transport` | GetTransport / ListTransports |
| `activate_all` | ActivatePackage |

---

### Quick Win: Remove Aliases (-700 tokens)

Current aliases provide no value:

| Alias | Full Tool | Tokens |
|-------|-----------|--------|
| `act` | Activate | 53 |
| `atc` | RunATCCheck | 56 |
| `es` | EditSource | 51 |
| `gro` | GrepObjects | 54 |
| `grp` | GrepPackages | 55 |
| `gs` | GetSource | 54 |
| `gt` | GetTable | 51 |
| `gtc` | GetTableContents | 54 |
| `rq` | RunQuery | 52 |
| `rut` | RunUnitTests | 57 |
| `sc` | SyntaxCheck | 55 |
| `so` | SearchObject | 53 |
| `ws` | WriteSource | 54 |

**Why remove:**
- Model doesn't need short names - `GetSource` is as easy as `gs`
- 700 tokens for zero benefit (~5% of VSP tool budget)
- Duplicates functionality, adds confusion
- Universal tool makes this moot anyway

**Action:** Delete alias registrations from `internal/mcp/server.go`

---

### Token Budget Comparison (Measured)

| Mode | Tools Exposed | Tokens | Reduction |
|------|---------------|--------|-----------|
| **Current (95 tools)** | 82 main + 13 aliases | 14,200 | baseline |
| **Remove aliases** | 82 main | ~13,500 | -5% |
| **6 Meta-Tools** | 6 meta | ~1,000 | -93% |
| **1 Universal Tool** | 1 tool | ~150 | **-99%** |
| **Skills + Bash** | 0 (uses Bash) | 0 | -100% |

### Alternative: Skills + Bash CLI (Zero MCP Overhead)

For maximum token efficiency, bypass MCP entirely:

```yaml
# Claude Code skill: /sap
name: sap
prompt: |
  You have access to SAP via the `vsp` CLI tool.
  Use Bash to execute commands:
    vsp read CLAS ZCL_TEST
    vsp edit CLAS ZCL_TEST --old "a + b" --new "a * b"
    vsp search "ZCL_*"
    vsp query "SELECT * FROM sflight"
```

**Trade-off:** Output is text, not structured JSON. Requires parsing.

---

## Part 3: Implementation Recommendation

### Phase 0: Remove Aliases (5 minutes, -700 tokens)

1. Delete 13 alias tool registrations from `internal/mcp/server.go`
2. Immediate 5% token reduction with zero functionality loss

### Phase 1: Single Universal Tool in Go

1. Add `SAP` universal tool handler to `internal/mcp/server.go`
2. Implement action routing to existing internal functions
3. Create `--mode unified` that exposes only the universal tool
4. Test with real workflows

### Phase 2: CLI Enhancement

1. Add `vsp read`, `vsp edit`, `vsp query` subcommands
2. Enable Skills + Bash approach as zero-overhead alternative
3. Structured JSON output with `--json` flag

### Phase 3: Additional Interfaces (Optional, Future)

Only needed if building interfaces OUTSIDE of Claude Code:

#### Option A: VS Code Extension (TypeScript)

For developers who want VSP without AI:

```
VS Code Extension
├── Object browser (tree view)
├── Source editing (virtual documents)
├── Syntax checking (real-time squiggles)
├── Debugger integration (VS Code debug protocol)
├── Test explorer (unit test runner)
└── Problems panel (ATC findings)
```

**When:** Large team adoption, traditional developers who don't use Claude
**Effort:** Significant (~5-10K LOC)
**Alternative:** SAP's official ADT tools already exist

#### Option B: CI/CD Automation (Bash/TypeScript)

For pipelines without human interaction:

```bash
# Simple: Bash scripts calling vsp CLI
vsp read CLAS ZCL_ORDER > source.abap
vsp test CLAS ZCL_ORDER --json | jq '.failed'
vsp deploy --package '$ZPROD' --transport 'A4HK900123'

# Complex: TypeScript for conditional logic
import { VSPClient } from 'vsp-client';
const vsp = new VSPClient();

const tests = await vsp.test('$ZPACKAGE');
if (tests.failed > 0) process.exit(1);

await vsp.deploy({ transport: 'A4HK900123' });
```

**Use cases:**
- Automated deployments on git push
- Scheduled test runs
- Code quality gates (ATC must pass)
- Migration scripts (bulk operations)

**When:** CI/CD integration, scheduled jobs, batch operations
**Effort:** Moderate (CLI already exists, just scripting)

#### Option C: Python + PyRFC

For operations requiring RFC (not ADT):

```python
from pyrfc import Connection
from vsp import VSPClient

# ADT operations via VSP
vsp = VSPClient()
source = vsp.read('CLAS', 'ZCL_ORDER')

# RFC operations via PyRFC (not available in ADT)
rfc = Connection(ashost='sap', sysnr='00', client='100', ...)
result = rfc.call('BAPI_USER_GET_DETAIL', USERNAME='TESTUSER')
```

**When:** Need RFC function modules that ADT doesn't expose
**Effort:** Low (wrapper around existing tools)

---

**For Claude Code users: None of Phase 3 is needed.** Claude is the orchestrator.

---

## Appendix: Example Conversations

### Before (95 tools, 14.2K tokens for definitions)

```
User: Fix the bug in ZCL_ORDER->CALCULATE

Claude: [searches through 95 tool schemas]
        Let me search for the class...
        [calls SearchObject]
        Now let me read the source...
        [calls GetSource]
        I see the issue. Let me edit...
        [calls EditSource]
        Let me check syntax...
        [calls SyntaxCheck]
        Now activate...
        [calls Activate]
        And run tests...
        [calls RunUnitTests]

6 tool calls, 14.2K tokens schema overhead
```

### After (1 universal tool, ~150 tokens)

```
User: Fix the bug in ZCL_ORDER->CALCULATE

Claude: [calls SAP { action: "read", target: "CLAS ZCL_ORDER", params: { method: "CALCULATE" }}]
        I see the issue. Fixing...
        [calls SAP { action: "edit", target: "CLAS ZCL_ORDER", params: {
            method: "CALCULATE", old: "...", new: "...", activate: true
        }}]
        Done. Object activated.

2 tool calls, 99% less schema overhead
```

### Alternative: Skills + Bash (0 tokens)

```
User: /sap Fix the bug in ZCL_ORDER->CALCULATE

Claude: [skill prompt injected, uses existing Bash tool]
        $ vsp read CLAS ZCL_ORDER --method CALCULATE
        I see the issue. Fixing...
        $ vsp edit CLAS ZCL_ORDER --method CALCULATE --old "..." --new "..." --activate
        Done. Object activated.

0 MCP schema tokens (Bash tool already in context)
```

---

## Appendix: Go vs Rust Code Structure Analysis

### Current Go Architecture (`internal/mcp/`)

```
internal/mcp/
├── server.go              # Main server, tool registration (30K tokens)
├── handlers_read.go       # GetProgram, GetClass, GetTable, etc.
├── handlers_crud.go       # EditSource, WriteSource, Activate
├── handlers_debugger.go   # Breakpoints, stepping
├── handlers_codeintel.go  # FindDefinition, FindReferences
├── handlers_*.go          # 20+ handler files by domain
└── server_test.go
```

**Tool Registration Pattern (Go):**
```go
// Schema defined here...
s.mcpServer.AddTool(mcp.NewTool("GetProgram",
    mcp.WithDescription("Retrieve ABAP program source code"),
    mcp.WithString("program_name",
        mcp.Required(),
        mcp.Description("Name of the ABAP program"),
    ),
), s.handleGetProgram)  // ...handler reference here

// Handler defined separately
func (s *Server) handleGetProgram(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // Manual extraction - no compile-time link to schema above
    programName, ok := request.Params.Arguments["program_name"].(string)
    if !ok || programName == "" {
        return newToolResultError("program_name is required"), nil
    }
    source, err := s.adtClient.GetProgram(ctx, programName)
    // ...
}
```

**Pain Points:**
1. Schema and handler are disconnected - misspell `"program_name"` in handler, no compile error
2. Runtime type assertions `.(string)` - can panic if wrong type
3. No exhaustive check - add tool to registration, forget handler = runtime error
4. `shouldRegister()` checks are stringly-typed

### What Rust Would Improve

**With official `rmcp` SDK:**
```rust
use rmcp::{tool, Tool, ServerHandler};
use serde::Deserialize;

// Schema AND handler bound together - compile-time checked
#[derive(Deserialize, Tool)]
#[tool(name = "GetProgram", description = "Retrieve ABAP program source code")]
struct GetProgram {
    #[tool(required, description = "Name of the ABAP program")]
    program_name: String,  // Misspell this = compile error
}

#[async_trait]
impl ServerHandler for GetProgram {
    async fn handle(&self, client: &AdtClient) -> Result<ToolResult> {
        // self.program_name already validated, typed, extracted
        client.get_program(&self.program_name).await
    }
}

// Tool groups as enums - exhaustive matching
enum ToolGroup {
    Read(ReadTools),
    Crud(CrudTools),
    Debug(DebugTools),
    // ... add variant = compiler forces handling
}

enum ReadTools {
    GetProgram(GetProgram),
    GetClass(GetClass),
    GetTable(GetTable),
    // Missing variant = compile error in match
}
```

**Compile-Time Guarantees:**
| Issue | Go | Rust |
|-------|-----|------|
| Typo in param name | Runtime error | Compile error |
| Wrong param type | Runtime panic | Compile error |
| Missing handler | Runtime error | Compile error |
| Unhandled tool group | Silent bug | Compile error |

### Verdict

**Rust would eliminate entire bug categories**, but:
- Go code works (244 tests passing)
- Official `rmcp` crate is new (would be early adopter)
- Rewrite = months of work
- SAP HTTP latency (100-500ms) makes dispatch optimization irrelevant

**Recommendation:** For a NEW MCP server project, Rust + `rmcp` is compelling. For VSP, keep Go unless bugs in tool dispatch become a recurring problem.

---

## Conclusion

1. **Keep Go** as the core - it's working well
2. **Implement single universal tool** for 99% token reduction (~150 tokens vs 14,200)
3. **Add CLI subcommands** for Skills + Bash zero-overhead approach
4. **TypeScript/Python optional** only if specific integration needed

The universal tool approach provides maximum token savings while maintaining structured output. It's purely additive - existing 95 tools remain available via `--mode expert` for backward compatibility.
