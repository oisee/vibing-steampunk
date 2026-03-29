# Codex Addendum — Standalone-First Positioning for vsp

**Date:** 2026-02-07  
**References:** `2026-02-07-001`, `2026-02-07-002`, `2026-02-07-006`  
**Directive applied:** vsp must stand on its own feet; Coral is not the center.

## Decision
vsp should be positioned as an **independent SAP engineering runtime**, not as an extension of Coral. Coral remains a possible integration target, but not the product anchor.

## Revised Core Positioning
**vsp is a standalone, single-binary SAP ADT runtime for AI-assisted and agentic ABAP engineering across read/write/debug/deploy workflows.**

### Product center (independent value)
1. SAP-native execution surface in one runtime (`cmd/vsp/main.go:26`, `internal/mcp/server.go:96`).
2. Broad tooling surface for ABAP lifecycle operations (`internal/mcp/server.go:224`).
3. Built-in safety controls (ops, packages, transport gates) (`pkg/adt/safety.go:8`, `pkg/adt/safety.go:170`, `pkg/adt/safety.go:251`).
4. REST-first value with optional WebSocket capability unlocks (`internal/mcp/server.go:551`, `internal/mcp/server.go:905`).

### Messaging to use
- "Independent SAP ADT execution runtime for AI and automation"
- "Works with any MCP-capable orchestrator or direct agent"
- "Enterprise hardening roadmap focused on auditability, reliability, and policy"

### Messaging to avoid
- "Coral adapter" as primary identity
- "Execution layer for everything"
- "Production-ready" (until hardening items are complete)

## Standalone-First Strategy

### Priority A: Product hardening (no external dependency)
1. Uniform machine-readable response contracts.
2. Audit/event trail with operation lineage.
3. Policy-complete enforcement across all write paths.
4. Rate controls and graceful shutdown behavior.
5. Tool registry consistency and contract tests.

### Priority B: Platform maturity
1. Deployment profiles (`safe-default`, `dev`, `unrestricted`).
2. Health/readiness API for operations teams.
3. Deterministic error taxonomy and retry semantics.
4. Scalability validation under concurrent agent load.

### Priority C: Ecosystem integrations (optional)
1. Coral integration.
2. Other orchestrators/CI systems.
3. Enterprise evidence export consumers.

## Risk Reframe (Standalone)
1. Biggest risk is not "missing Coral"; it is failing enterprise runtime quality gates.
2. Biggest opportunity is becoming the default SAP execution layer for any orchestrator.
3. Biggest strategic mistake would be coupling roadmap identity to one enterprise program.

## Practical Rule
Coral-compatible: **yes**.  
Coral-dependent: **no**.

