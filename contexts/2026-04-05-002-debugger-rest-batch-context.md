# Debugger REST Batch Context

**Date:** 2026-04-05
**Context for:** Issue #2, `reports/2026-04-05-001-gui-debugger-design.md`
**Purpose:** Preserve the key debugger findings from the latest design discussion so follow-up implementation can start without re-discovery

---

## What Changed

Reading `CL_TPDA_ADT_RES_APP` on A4H shifted the debugger plan materially.

Three findings matter:

1. **`/debugger/batch` exists and is strategic**
   - Multiple debugger operations can be combined into one HTTP round-trip
   - This removes the main latency disadvantage of REST fallback versus WebSocket for paused-session workflows
   - Practical implication: `step + stack + variables` can be fetched together after each stop

2. **Variable export is richer than expected**
   - Variable/table data can be exported as CSV
   - The export supports filtering, sorting, and WHERE-style narrowing
   - Live debugger data can be converted into ABAP `VALUE #( ... )` statements
   - Practical implication: debugger can become a test-data extraction tool, not just an inspection tool

3. **Watchpoints are available over REST**
   - Full watchpoint CRUD is exposed without requiring ZADT_VSP
   - We previously did not treat REST fallback as watchpoint-capable
   - Practical implication: fallback mode is more feature-complete than assumed

---

## Architectural Consequences

The old assumption was:
- WebSocket/ZADT_VSP = fast, complete debugger
- REST fallback = slower, reduced capability, mainly emergency compatibility mode

The revised assumption is:
- WebSocket/ZADT_VSP is still the best transport for push-style events and interactive UX
- REST fallback is now strong enough for a real Phase 1 debugger session layer if batch is used well
- DAP and Web GUI no longer need to wait for a perfect browser frontend to start delivering value

This lowers the risk of implementing debugger sessions in MCP first.

---

## Updated Phase Estimates

| Phase | Scope | Est. | Depends On |
|------:|-------|------|------------|
| **1** | MCP Debug Sessions + REST batch | 10-14h | Nothing |
| **2** | DAP Provider (VS Code) | 16-21h | Phase 1 |
| **3** | Web GUI Debugger | 22-32h | Phase 1 |
| **Total** | End-to-end debugger platform | 48-67h | - |

Why the estimate changed:
- REST batch reduces per-step latency concerns
- Watchpoints no longer need a custom ZADT_VSP-only path
- Variable export/value generation adds high-leverage functionality with no separate platform dependency

---

## Phase 1 Intent

Phase 1 should focus on a durable session/control layer, not on UI polish.

Recommended target:
- MCP debug session lifecycle
- Attach/listen/status primitives
- Batched step execution responses
- Stack + variables fetch in the same post-step cycle
- Watchpoint CRUD in fallback mode
- Variable export helpers

Recommended non-goals for Phase 1:
- Full browser debugger UI
- Rich editor embedding
- Advanced visualization beyond structured MCP outputs

---

## Dependency View

### Hard dependencies
- Existing ADT authentication/session handling
- REST debugger client support
- MCP tool surface for session lifecycle and step actions

### Likely implementation slices
- `pkg/adt/*` debugger REST client additions
- Batch request/response modeling
- Watchpoint REST wrappers
- Variable export/value-generation helpers
- MCP handler layer for debug sessions

### Soft dependencies for later phases
- DAP transport adapter for VS Code
- Web HTTP/UI serving layer for browser debugger
- Optional ZADT_VSP acceleration path where push events matter

---

## Strategic Position

The clean sequencing now looks like this:

1. Build Phase 1 on MCP Debug Sessions with REST batch as the baseline
2. Reuse that session model for a DAP provider
3. Build Web GUI only after the session/control contract is stable

This avoids over-investing in frontend before the debugger session API is shaped correctly.

---

## Open Questions

- What exact request/response shape does `/debugger/batch` expect across different SAP releases?
- Are watchpoint semantics stable enough between systems to expose directly in MCP without feature flags?
- Should `VALUE #( ... )` generation live in debugger code or as a reusable data-conversion helper?
- Which paused-state payload should be treated as the canonical Phase 1 response: stack-first, variables-first, or combined frame bundle?

---

## Next Practical Step

Do a focused dependency/code map for Phase 1:
- locate current REST debugger client coverage
- identify missing batch/watchpoint/export pieces
- map CLI/MCP entry points that should own debug sessions

That map should be saved as the next context/report artifact before implementation starts.
