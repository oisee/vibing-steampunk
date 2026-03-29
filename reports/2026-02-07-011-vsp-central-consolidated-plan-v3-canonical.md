# VSP Central Consolidated Plan v3 (Canonical, Standalone-First)

**Date:** 2026-02-07  
**Supersedes:** `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/reports/2026-02-07-010-vsp-central-consolidated-plan-v2-standalone.md`  
**Purpose:** Final coherent synthesis of today’s Codex + Claude strategic reports, with corrections applied and ambiguity removed.

## Executive Summary
vsp should be positioned as an independent SAP ADT execution runtime that can integrate with orchestrators but does not depend on any one of them. The primary adoption blockers are operational and governance maturity, not feature breadth: no audit trail, inconsistent response contracts, incomplete policy enforcement across mutating paths, weak default safety posture, and limited orchestration-layer tests. The immediate strategy is a 60-day hardening program focused on deterministic contracts, auditability, safe defaults, runtime controls, and test reliability.

## Source Set (Today)
1. `001` Codex deep dive.
2. `002` Codex CBA alignment review.
3. `003` Claude CBA-corrected strategic analysis.
4. `004` Claude re-evaluation gap analysis.
5. `004a` Claude open-ended re-evaluation prompt.
6. `005` Codex Coral-context addendum.
7. `006` Codex independent reassessment.
8. `007` Codex standalone-first positioning.
9. `008` Codex operational reliance/relevance plan.
10. `010` prior consolidated v2 draft.

## Canonical Positioning (Final)
**vsp is a standalone, single-binary SAP ADT execution runtime for AI-assisted and agentic ABAP workflows.**

Positioning rules:
1. Standalone-first identity.
2. Orchestrator-compatible interfaces.
3. No orchestrator-coupled roadmap dependencies.
4. Do not claim enterprise production readiness until P0 gates pass.
5. Do not headline with hardcoded tool counts.

Messaging to use:
1. "Independent SAP ADT execution runtime."
2. "Works with any MCP-capable orchestrator or direct agent."
3. "Enterprise hardening roadmap focused on auditability, reliability, and policy."

Messaging to avoid:
1. "Execution layer for everything."
2. "Production-ready" (current state does not support this claim).
3. Any fixed tool-count headline pulled from stale docs.

## Architecture Snapshot (Code-Verified)
Entry points and runtime shape:
1. `cmd/vsp/main.go` initializes CLI, MCP server mode, config and execution.
2. `internal/mcp/server.go` central registry + tool dispatch + mode gating.
3. REST path: MCP tool handler -> `pkg/adt` client/workflow -> ADT REST endpoints.
4. WebSocket path: MCP tool handler -> `pkg/adt` ws clients -> ZADT_VSP domains.

Key coupling findings:
1. `internal/mcp/server.go` remains a large god-file risk.
2. `pkg/adt` remains a large god-package risk.
3. Debugger path split across legacy REST and newer WS handlers.
4. Runtime registry and config/tool listings can drift.

No import cycles were identified in the package graph, but coupling is high.

## Non-Negotiable Corrections (Applied in v3)

### 1) Tool count discipline
Current state:
1. Count references drift across code/docs:
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/cmd/vsp/main.go:32`
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:224`
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/cmd/vsp/config_cmd.go:753`

Correction:
1. Treat generated catalog count as a target remediation, not a current fact.
2. Until generation lands, avoid hardcoded count claims in strategic docs.

### 2) Secret remediation scope
Current evidence:
1. `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/reports/2025-12-21-007-phase5-live-experiment.md:26`
2. `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/amdp-breakpoint-test-results-20251206-084536.log`

Correction:
1. Deleting files in working tree is insufficient.
2. Remediation must explicitly choose one of:
   - targeted redaction/removal with history rewrite, or
   - full affected-path history removal.
3. Any rewrite must be treated as destructive and coordinated.
4. Credential rotation is mandatory regardless of path chosen.

### 3) Test gate precision
Code signals:
1. Integration tests are build-tagged: `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/integration_test.go:1`
2. Debugger tests can open listeners: `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/debugger_test.go:333`

Correction:
1. Use a tiered testing policy.
2. Do not use `go test ./...` as sole enterprise readiness evidence.

### 4) AMDP concurrency status discipline
Evidence basis:
1. `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/amdp_websocket.go:17`
2. `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/websocket_base.go:28`

Correction:
1. Keep status as **unconfirmed high-risk hypothesis** until deterministic repro exists.
2. Prioritize targeted race/deadlock tests before declaring a bug.

## P0 Readiness Gates (Enterprise Pitch Blockers)

### Gate A: Security Hygiene + History Hygiene
Requirements:
1. No sensitive data in working tree.
2. No unresolved sensitive data in reachable history according to chosen remediation scope.
3. CI secret scanning on PRs.
4. Rotated credentials for any exposed material.

### Gate B: Deterministic Response Envelope
Current inconsistency evidence:
1. `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:2446`
2. `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/handlers_system.go:99`
3. `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/handlers_search.go:31`

Requirements:
1. One response envelope schema.
2. Typed errors with retryability metadata.
3. Contract tests for focused-stable profile first.

### Gate C: Auditability + Correlation
Requirements:
1. Structured audit events for 100% mutating operations.
2. `operation_id` + `correlation_id` propagation.
3. Replayable timeline of what vsp changed and why.

### Gate D: Policy-Complete Write Path Enforcement
Partial enforcement evidence:
1. Enforced create paths:
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/crud.go:313`
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/workflows.go:220`
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/workflows.go:313`
2. Missing on update/edit paths:
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/workflows.go:1291`
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/workflows.go:2084`
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/crud.go:113`

Requirements:
1. No mutating path bypasses package policy checks.
2. Automated enforcement tests cover create/update/edit/delete.

### Gate E: Safe Defaults
Current default posture evidence:
1. `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:114`
2. `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/config.go:188`

Requirements:
1. Enterprise-safe default profile.
2. Unsafe mode must require explicit opt-in and startup warning.

### Gate F: Runtime Controls
Gap evidence:
1. Missing full process shutdown/drain behavior around serve path:
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/cmd/vsp/main.go:223`
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:201`

Requirements:
1. Graceful shutdown and in-flight drain.
2. Health/readiness contract for orchestrators.
3. Rate limiting and concurrency controls to protect SAP backends.

### Gate G: Tool Inventory Integrity
Requirements:
1. Single source for registry/config/docs naming.
2. CI drift detection for catalog mismatches.

## Test Strategy (Canonical)

### Tier 1: Unit (required every PR)
1. Deterministic, fast, no live SAP dependency.
2. Includes response contract tests and policy enforcement tests.

### Tier 2: Integration (tagged)
1. Runs when environment is provided.
2. Separate from baseline PR gate.

### Tier 3: Live-SAP / Soak
1. Scheduled/manual.
2. Includes concurrency, shutdown, WS lifecycle, and longer-run stability.

## Focused Mode Reliability Policy
Current issue:
1. Focused mode includes WS-dependent tools that fail if ZADT_VSP is unavailable:
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:283`
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:357`
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:399`
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:403`

Policy:
1. `focused-stable`: REST-safe baseline.
2. `focused-extended`: WS-dependent subset with explicit dependency signaling.

## 60-Day Plan (Standalone, No Orchestrator Coupling)

### Weeks 1-2
1. Complete Gates A, B, C, E.
2. Patch package policy bypasses (Gate D start).
3. Start registry/catalog single-source work (Gate G start).

### Weeks 3-4
1. Complete Gates D, F, G.
2. Add focused-stable/focused-extended profiles.
3. Expand MCP layer tests and contract coverage.

### Weeks 5-6
1. Normalize SAP quality outputs (ATC/dumps/traces) into one schema.
2. Add evidence bundle format for change outcomes.
3. Add idempotency guidance/keys for mutating flows.

### Weeks 7-8
1. Run load/failure/shutdown drills.
2. Publish baseline SLOs and operational metrics.
3. Freeze a production profile and release-readiness checklist.

## Build / Integrate / Ignore (Final)

### Build (core to vsp)
1. Deterministic response contracts.
2. Audit logging and correlation propagation.
3. Policy-complete enforcement across all write paths.
4. Safe defaults and runtime controls.
5. Strong MCP handler test coverage.

### Integrate (vsp should connect, not own)
1. External orchestrators.
2. CI/CD systems and evidence consumers.
3. Secret/vault providers.

### Ignore or Deprioritize (for now)
1. Native multi-agent orchestration.
2. Deep Jira/Cloud ALM ownership.
3. Further Lua expansion before hardening gates.

## Kill List (Canonical)
1. "Production-ready" claim until P0 gates are complete.
2. "Execution layer for everything" positioning.
3. Hardcoded tool-count marketing.
4. Any narrative that makes Coral central to vsp identity.
5. New feature breadth initiatives that preempt operational hardening.

## Scenario Assumptions Register (Explicitly Non-Facts)
1. SAP native VS Code ADT + AI may compress runway; current 12-24 month window is a scenario assumption, not certainty.
2. CBA architecture-readiness percentages are estimate-quality and depend on operational definition granularity.
3. Multi-agent concurrency on same SAP objects is constrained by lock semantics; near-term practical model remains single-agent-per-object scope.

## Immediate 48-Hour Actions
1. Create issue set for Gates A-G with one acceptance test per gate.
2. Patch policy gaps on update/edit paths and add regression tests.
3. Add secret-scanning gate in CI.
4. Draft envelope schema and migrate focused-stable tool responses first.
5. Publish one-page external positioning statement based on this v3 plan.

## Final Directive
Stand on independent product value. Learn from enterprise operating patterns, but do not couple roadmap identity to any single enterprise program.
