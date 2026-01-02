# DAP Integration Vision for ABAP Debugging

**Date:** 2026-01-02
**Report ID:** 004
**Subject:** Debug Adapter Protocol (DAP) integration architecture for AI-assisted ABAP debugging
**Related Documents:**
- 2025-12-05-014-external-debugger-scripting-vision.md
- 2025-12-05-024-amdp-goroutine-channel-architecture.md
- 2025-12-06-002-amdp-debugging-status-progress.md

---

## Executive Summary

This report outlines a vision for integrating Debug Adapter Protocol (DAP) with the existing MCP-based vsp server, enabling native VS Code debugging experience for ABAP while maintaining AI-assisted capabilities through Claude Code.

## Background: MCP vs DAP

### Model Context Protocol (MCP)

MCP is fundamentally **request-response** based:
- Each tool call is stateless from the protocol perspective
- Server maintains state internally (WebSocket connections, debug sessions)
- No native event push mechanism - client must poll
- Designed for AI tool use, not interactive debugging

**Current vsp Debug Architecture:**
```
Claude Code → MCP → vsp → ADT APIs
                    ↓
              WebSocket (ZADT_VSP)
                    ↓
              SAP Debug Session
```

### Debug Adapter Protocol (DAP)

DAP is **bidirectional and event-driven**:
- Session-oriented with explicit lifecycle (initialize → launch/attach → ... → disconnect)
- Server pushes events: `stopped`, `output`, `breakpoint`, `thread`
- Client sends requests: `continue`, `stepIn`, `stackTrace`, `variables`
- Designed specifically for interactive debugging in IDEs

**Standard DAP Flow:**
```
VS Code ←→ DAP Adapter ←→ Debug Runtime
         (bidirectional)
```

## Proposed Architecture

### Dual-Mode Server

Single vsp binary supporting both protocols:

```
┌─────────────────────────────────────────────────────────┐
│                      vsp binary                          │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌─────────────┐         ┌─────────────┐                │
│  │   MCP Mode  │         │  DAP Mode   │                │
│  │  (default)  │         │  (--dap)    │                │
│  └──────┬──────┘         └──────┬──────┘                │
│         │                       │                        │
│         └───────────┬───────────┘                        │
│                     ▼                                    │
│         ┌───────────────────────┐                        │
│         │    Shared State       │                        │
│         │  - Debug Sessions     │                        │
│         │  - Breakpoints        │                        │
│         │  - WebSocket Pool     │                        │
│         └───────────┬───────────┘                        │
│                     ▼                                    │
│         ┌───────────────────────┐                        │
│         │    ADT Client         │                        │
│         │  - HTTP/REST          │                        │
│         │  - WebSocket          │                        │
│         └───────────────────────┘                        │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Usage Scenarios

#### Scenario 1: Pure MCP (Current)
```bash
./vsp  # Default MCP mode for Claude Code
```

AI assistant uses tools like `SetBreakpoint`, `DebuggerListen`, `DebuggerStep` through MCP.

#### Scenario 2: Pure DAP
```bash
./vsp --dap  # VS Code debug adapter mode
```

VS Code connects directly for traditional debugging experience.

#### Scenario 3: Hybrid (AI + IDE)
```bash
./vsp &                    # MCP server in background
./vsp --dap --share-state  # DAP adapter sharing state
```

Both Claude Code and VS Code can interact with the same debug session:
- VS Code shows native debug UI (variables, call stack, breakpoints)
- Claude can analyze state, suggest fixes, explain code flow
- User switches between AI guidance and manual stepping

## DAP Implementation Details

### Required DAP Requests

| Request | ABAP Mapping |
|---------|--------------|
| `initialize` | Setup ADT client, negotiate capabilities |
| `launch` | Run program (via RunReport/RunUnitTests) |
| `attach` | Connect to running process (DebuggerListen + Attach) |
| `setBreakpoints` | SetBreakpoint (ADT external breakpoints) |
| `continue` | DebuggerStep(stepContinue) |
| `next` | DebuggerStep(stepOver) |
| `stepIn` | DebuggerStep(stepInto) |
| `stepOut` | DebuggerStep(stepReturn) |
| `stackTrace` | DebuggerGetStack |
| `scopes` | Map stack frames to variable scopes |
| `variables` | DebuggerGetVariables |
| `disconnect` | DebuggerDetach |

### Required DAP Events

| Event | When to Send |
|-------|--------------|
| `initialized` | After initialize request processed |
| `stopped` | Breakpoint hit, step completed |
| `output` | Debug console messages |
| `breakpoint` | Breakpoint verified/changed |
| `thread` | New work process attached |
| `terminated` | Debug session ended |

### AMDP Debugging

For HANA SQLScript debugging, map additional operations:

| DAP | AMDP Mapping |
|-----|--------------|
| `setBreakpoints` (SQLScript) | AMDPSetBreakpoint |
| `stepIn` (into HANA) | AMDPDebuggerStep(stepInto) |
| `variables` (HANA) | AMDPGetVariables |

## Implementation Phases

### Phase 1: Basic DAP Adapter
- Implement DAP protocol handler in Go
- Map core requests to existing ADT operations
- Support attach mode (external breakpoints)
- VS Code extension with launch.json configuration

### Phase 2: Launch Mode
- Implement program execution triggers
- Support reports, unit tests, function modules
- Handle selection screen parameters

### Phase 3: Shared State
- State synchronization between MCP and DAP modes
- Breakpoint registry shared across modes
- Event forwarding to MCP clients (polling-based)

### Phase 4: Enhanced Features
- Conditional breakpoints
- Watch expressions
- Hot code replace (if SAP supports)
- AMDP debugging integration

## VS Code Extension

Minimal extension to configure the debug adapter:

**package.json:**
```json
{
  "contributes": {
    "debuggers": [{
      "type": "abap",
      "label": "ABAP Debug",
      "program": "./vsp",
      "args": ["--dap"],
      "configurationAttributes": {
        "launch": {
          "properties": {
            "program": { "type": "string", "description": "Program/class to debug" },
            "breakpoints": { "type": "array", "items": { "type": "object" } }
          }
        },
        "attach": {
          "properties": {
            "user": { "type": "string", "description": "SAP user to attach to" }
          }
        }
      }
    }]
  }
}
```

**launch.json:**
```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "type": "abap",
      "request": "attach",
      "name": "Attach to ABAP",
      "user": "${env:SAP_USER}"
    },
    {
      "type": "abap",
      "request": "launch",
      "name": "Debug Unit Tests",
      "program": "ZCL_MY_CLASS",
      "testClass": "ltcl_test"
    }
  ]
}
```

## Alternative: Claude Code Skill

Instead of full DAP implementation, a simpler `/debug` skill could orchestrate existing MCP tools:

```
User: /debug ZCL_MY_CLASS->METHOD_X

Claude: Setting up debug session...
1. Set breakpoint at ZCL_MY_CLASS line 42 (METHOD_X entry)
2. Running unit tests to trigger...
3. Breakpoint hit! Analyzing context...

Current state:
- Line 42: DATA(lv_result) = calculate( iv_input ).
- Variables: iv_input = 'TEST', lv_result = <unassigned>
- Call stack: 3 frames deep

Recommendation: The calculate() method expects numeric input
but received string 'TEST'. Step into to see the conversion error.

[Continue] [Step In] [Step Over] [Variables] [Explain]
```

This approach:
- Uses existing MCP tools (no new protocol)
- AI provides context and recommendations
- Interactive via Claude Code UI
- Simpler implementation, faster delivery

## Comparison: DAP vs Skill

| Aspect | DAP Adapter | `/debug` Skill |
|--------|-------------|----------------|
| VS Code integration | Native debug UI | None |
| Implementation effort | High (new protocol) | Low (orchestration) |
| User experience | Standard debugging | AI-guided |
| Breakpoint visualization | IDE gutter icons | Text description |
| Variable inspection | Tree view | AI-summarized |
| Stepping | Click buttons | Chat commands |
| AI assistance | Limited | Full context |

## Recommendation

**Short-term:** Implement `/debug` skill for AI-assisted debugging through Claude Code. This leverages existing infrastructure and provides immediate value.

**Medium-term:** Implement basic DAP adapter for attach mode. This enables VS Code's native debugging UI for users who prefer traditional tooling.

**Long-term:** Full DAP implementation with shared state, allowing seamless switching between AI-guided debugging and manual IDE debugging.

## Conclusion

The combination of MCP (AI tools) and DAP (IDE debugging) creates a powerful ABAP development experience. MCP handles the "what to do" through AI reasoning, while DAP handles the "how to interact" through familiar IDE paradigms.

The `/debug` skill provides immediate AI-assisted debugging value, while DAP integration builds toward a future where Claude Code and VS Code work together - AI understanding code flow and suggesting fixes, IDE providing familiar debugging controls.

---

## Appendix: Go DAP Libraries

- **go-dap**: Microsoft's official Go implementation
  - `github.com/google/go-dap`
  - Used by Delve (Go debugger)

- **vscode-go**: Reference implementation
  - Shows DAP adapter patterns

## References

- [DAP Specification](https://microsoft.github.io/debug-adapter-protocol/specification)
- [MCP Specification](https://modelcontextprotocol.io/)
- [ADT Debug APIs](./adt-abap-internals-documentation.md)
