# ADR-001: WebSocket/APC for Stateful ABAP Debugging

**Date:** 2025-12-18
**Status:** PROPOSAL / PARKED
**Context:** External debugger limitations with HTTP transport

---

> **IMPORTANT: NOT VANILLA ADT COMPATIBLE**
>
> This ADR proposes a **theoretical architecture** that would require **custom ABAP
> development** (Z* objects, WebSocket/APC handlers) on the SAP system.
>
> This approach **breaks the vanilla ADT philosophy** - vsp currently requires NO custom
> SAP objects and works purely via standard ADT REST APIs.
>
> The current HTTP-based debugger tools work within vanilla ADT constraints (with known
> session limitations).

---

## Summary

The external ABAP debugger requires stateful HTTP sessions that cannot be reliably maintained via standard HTTP/HTTPS transport due to session affinity requirements and long-polling timeout constraints. This ADR proposes implementing a stateful WebSocket-based approach using SAP's APC (ABAP Push Channel) infrastructure.

## Context

### Current State

The vsp MCP server implements external debugger support via ADT REST APIs:
- `SetExternalBreakpoint` - Works correctly (breakpoints persist)
- `GetExternalBreakpoints` - Works correctly
- `DeleteExternalBreakpoint` - Works correctly
- `DebuggerListen` - Long-polling listener, works but unreliable
- `DebuggerAttach` / `DebuggerStep` / `DebuggerGetStack` / `DebuggerGetVariables` - Require same HTTP session

### Problem

External debugger operations require:
1. **Session Stickiness**: All debugging operations must use the same HTTP session (cookies, session ID)
2. **Long-polling Support**: Listener needs 60-240 second timeouts
3. **Terminal ID Consistency**: Same terminal ID across all operations

When using vsp via MCP:
- Each MCP tool call may spawn a separate process
- HTTP sessions are not shared between tool calls
- The debugger listener catches the debuggee, but attach/step operations fail due to session mismatch
- Go's HTTP client is stateless by design

### Attempted Solutions

1. **Daemon Mode** (branch `feature/debug-daemon-parked`)
   - Created persistent background process with Unix socket communication
   - Works for session persistence, but adds operational complexity
   - Parked: Requires service management, doesn't solve SAP-side session issues

2. **Batch API**
   - Implemented multipart/mixed batch requests (Eclipse-compatible)
   - Works for combining operations but doesn't solve listener → attach session gap

3. **Terminal ID Consistency**
   - Made terminal ID deterministic based on username
   - Helps with breakpoint association but doesn't fix session problem

## Decision

Implement a stateful WebSocket/APC handler on the SAP side to maintain debugging sessions. This approach:

1. Moves session state to the SAP server (where it belongs)
2. Uses WebSocket for bidirectional communication
3. Eliminates HTTP session stickiness requirements from vsp

### Proposed Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        SAP System                            │
│  ┌────────────────────────────────────────────────────────┐ │
│  │            ZADT_DEBUG_APC (APC Handler)                │ │
│  │  ┌──────────────┐   ┌───────────────┐                  │ │
│  │  │ WebSocket    │   │ Debug Session │                  │ │
│  │  │ Endpoint     │──▶│ Manager       │                  │ │
│  │  └──────────────┘   └───────────────┘                  │ │
│  │         │                   │                           │ │
│  │         │           ┌───────▼───────┐                   │ │
│  │         │           │ ADT Debugger  │                   │ │
│  │         │           │ APIs          │                   │ │
│  │         │           └───────────────┘                   │ │
│  └─────────│───────────────────────────────────────────────┘ │
└────────────│────────────────────────────────────────────────┘
             │ WebSocket
┌────────────│────────────────────────────────────────────────┐
│            │              vsp MCP Server                     │
│  ┌─────────▼─────────┐                                       │
│  │  WebSocket Client │    ┌──────────────────┐              │
│  │  (goroutine)      │◀──▶│ MCP Tool Handlers│              │
│  └───────────────────┘    └──────────────────┘              │
└──────────────────────────────────────────────────────────────┘
```

### SAP Objects Required

| Object | Type | Description |
|--------|------|-------------|
| `ZADT_DEBUG_APC` | SICF Service | WebSocket endpoint |
| `ZCL_APC_DEBUG_HANDLER` | Class | IF_APC_WS_EXTENSION implementation |
| `ZCL_APC_DEBUG_SESSION` | Class | Session manager (singleton per connection) |

### Message Protocol (JSON over WebSocket)

```typescript
// Request
{
  "id": "uuid-v4",
  "action": "listen" | "attach" | "step" | "getStack" | "getVariables" | "detach",
  "params": {
    // action-specific parameters
  }
}

// Response
{
  "id": "uuid-v4",
  "success": boolean,
  "data": { ... },
  "error": string | null
}

// Push Event (debuggee caught, breakpoint hit)
{
  "event": "debuggeeCaught" | "breakpointHit" | "sessionEnded",
  "data": { ... }
}
```

## Implementation Plan

### Phase 1: SAP Objects (ZADT package)
1. Create APC service configuration in SICF
2. Implement `ZCL_APC_DEBUG_HANDLER` with IF_APC_WS_EXTENSION
3. Implement `ZCL_APC_DEBUG_SESSION` for state management
4. Create message parsing/formatting utilities

### Phase 2: vsp WebSocket Client
1. Add WebSocket client to vsp (gorilla/websocket or nhooyr/websocket)
2. Implement connection manager with reconnection logic
3. Map MCP tool calls to WebSocket messages
4. Handle push events (debuggee notifications)

### Phase 3: Integration
1. Auto-detect APC availability (GetFeatures)
2. Fall back to HTTP if APC unavailable
3. Update debugger tools to use WebSocket transport
4. Add configuration options for WebSocket URL

## Alternatives Considered

### 1. HTTP/2 with Server Push
- **Pros**: Standard HTTP, no new SAP objects needed
- **Cons**: SAP ICF doesn't support HTTP/2 push, limited browser support

### 2. Persistent HTTP Session Daemon (Implemented, Parked)
- **Pros**: No SAP changes required
- **Cons**: Complex operational model, still suffers from session issues

### 3. Eclipse DAP (Debug Adapter Protocol)
- **Pros**: Standard protocol, editor integration
- **Cons**: Requires separate server process, more complexity

## Consequences

### Positive
- Clean separation of concerns (session state in SAP)
- Bidirectional communication enables real-time debug events
- Reusable infrastructure for other stateful operations
- Better reliability than HTTP long-polling

### Negative
- Requires SAP-side development and deployment
- WebSocket support varies by SAP version
- Additional firewall/proxy configuration may be needed
- Increased complexity vs pure REST

### Risks
- APC availability varies by SAP release (available from 7.40+)
- STRUST/SSL certificate management for secure WebSocket
- Network infrastructure may block WebSocket upgrades

## Status: PARKED

This ADR is parked pending:
1. Assessment of actual debugging use case priority
2. Availability of test system with APC support
3. Decision on whether to invest in SAP-side development

The existing HTTP-based debugger works for:
- Setting and managing breakpoints
- Listing breakpoints
- Basic listener functionality

The limitation is in the attach/step/variable inspection flow which requires session affinity.

## References

- SAP Documentation: [ABAP Push Channels](https://help.sap.com/doc/abapdocu_752_index_htm/7.52/en-US/abenabap_push_channel.htm)
- Branch: `feature/debug-daemon-parked` - HTTP daemon implementation
- Report: `reports/2025-12-14-002-external-debugger-investigation.md`
