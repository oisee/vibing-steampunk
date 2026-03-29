# VSP Central Consolidated Plan v2 (Standalone-First)

**Date:** 2026-02-07  
**Supersedes:** `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/reports/2026-02-07-009-vsp-central-consolidated-document.md`  
**Basis:** Consolidation and correction of 2026-02-07 reports `001`-`008` plus follow-up review.

## Executive Summary
vsp should be run as an independent SAP execution runtime, with optional orchestrator integrations. The critical path is operational trust: security hygiene, deterministic contracts, auditability, policy-complete enforcement, safety defaults, runtime controls, and test maturity. Feature breadth is already strong; reliability and governance maturity are the blockers.

## What Changed From v1
1. Secret cleanup now requires history remediation, not just file deletion.
2. Test gates split into `unit`, `integration`, and `live-sap`.
3. `/CBA/` package policy gap is promoted to a tracked P0 enforcement item.
4. Unsafe default posture is promoted to Week-1 gate.
5. Focused-mode WS dependency is now a concrete gating item.
6. AMDP mutex issue is treated as unconfirmed until reproduced.
7. Coral language is demoted to optional integration context only.

## Canonical Positioning
**vsp is a standalone, single-binary SAP ADT execution runtime for AI-assisted and agentic ABAP workflows.**

Positioning rules:
1. Standalone-first product identity.
2. Orchestrator-compatible interfaces.
3. No orchestrator dependency in roadmap or narrative.

Messaging to use:
- "Independent SAP ADT execution runtime"
- "Works with any MCP-capable orchestrator or direct agent"
- "Hardening roadmap focused on auditability, reliability, and policy"

Messaging to avoid:
- "Execution layer for everything"
- "Production-ready" before gates are met
- Tool-count headline positioning

## P0 Readiness Gates (Must Pass Before Any Enterprise Pitch)

### Gate A: Security Hygiene + History Remediation
- Remove committed sensitive artifacts.
- Rotate exposed credentials.
- Remediate sensitive content from Git history.
- Add CI secret scanning and block merges on findings.

Current evidence:
- `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/reports/2025-12-21-007-phase5-live-experiment.md:26`
- `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/amdp-breakpoint-test-results-20251206-084536.log`

Acceptance criteria:
1. No active secrets in working tree.
2. No active secrets in commit history after remediation.
3. Secret scan clean on PR pipeline.

### Gate B: Deterministic Response Contracts
- Implement one versioned response envelope across all tools.
- Remove mixed plain-text/prose/JSON-as-text ambiguity.

Current evidence of inconsistency:
- `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:2446`
- `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/handlers_system.go:99`
- `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/handlers_search.go:31`

Acceptance criteria:
1. All tool handlers return schema-conformant envelopes.
2. Error taxonomy includes retryability and category.
3. Contract tests cover all focused-mode tools.

### Gate C: Auditability + Correlation
- Emit structured audit events for every mutating operation.
- Include `operation_id`, `correlation_id`, actor context, policy decision, target, outcome.

Acceptance criteria:
1. 100% mutating operation audit coverage.
2. Replayable timeline from audit logs.

### Gate D: Policy-Complete Write Enforcement
- Enforce package policy checks on all write/update/edit flows.

Current partial coverage evidence:
- Enforced: `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/crud.go:313`, `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/workflows.go:220`, `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/workflows.go:313`
- Gaps: `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/workflows.go:1291`, `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/workflows.go:2084`, `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/crud.go:113`

Acceptance criteria:
1. No mutating path bypasses package policy.
2. Automated tests prove enforcement across create/update/edit/delete.

### Gate E: Safe Defaults
- Replace unrestricted startup baseline with explicit deployment profiles.

Current default evidence:
- `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:114`
- `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/config.go:188`

Acceptance criteria:
1. Default profile is safe for enterprise environments.
2. Unsafe mode requires explicit override and startup warning.

### Gate F: Runtime Controls
- Add graceful shutdown.
- Add health/readiness contract.
- Add rate limits/concurrency budgets.

Current evidence:
- No process-level graceful shutdown wiring around stdio serve path: `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/cmd/vsp/main.go:223`, `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:201`

Acceptance criteria:
1. SIGTERM/SIGINT drains in-flight work and closes sessions cleanly.
2. Health endpoint/tool reports structured state.
3. Rate limiter prevents uncontrolled SAP load.

### Gate G: Tool Inventory Integrity
- Generate runtime registry, config list, and docs from one source.

Current drift evidence:
- `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:224`
- `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/cmd/vsp/config_cmd.go:753`
- `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/cmd/vsp/main.go:32`

Acceptance criteria:
1. No count/name mismatches across runtime/config/docs.
2. CI fails on catalog drift.

## Test Strategy v2 (Corrected)

### Tier 1: Unit (required on every PR)
- Fast, deterministic, no external SAP.
- Includes contract tests and policy enforcement tests.

### Tier 2: Integration (tagged, optional in default PR)
- Build-tagged integration suite remains separate.
- Evidence: `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/integration_test.go:1`

### Tier 3: Live-SAP / Soak (scheduled/manual)
- Real environment, concurrency/load/drain scenarios.
- Includes WS lifecycle and long-running stability.

Note on debugger tests:
- Some tests create listeners (`httptest.NewServer`), so execution policy must reflect environment constraints.
- Evidence: `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/debugger_test.go:333`

## Focused Mode Reliability Correction

Issue:
- Focused mode currently includes several WS-dependent capabilities that can fail in environments without ZADT_VSP.
- Evidence: `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:283`, `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:357`, `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:399`, `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:403`

Action:
1. Define `focused-stable` profile (REST-safe baseline).
2. Define `focused-extended` profile (WS-enabled where available).
3. Make WS dependency explicit in schema and health output.

## AMDP Concurrency Finding Status

Status: **Unconfirmed high-risk hypothesis**.
- Structural basis exists (`/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/amdp_websocket.go:17`, `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/websocket_base.go:28`).
- Not yet proven with a deterministic repro.

Action:
1. Build targeted race/deadlock repro tests.
2. Confirm or downgrade with evidence.

## 60-Day Delivery Plan (Standalone)

### Weeks 1-2
1. Complete Gates A, B, C, E.
2. Patch package-policy gaps (Gate D start).
3. Introduce generated tool catalog pipeline (Gate G start).

### Weeks 3-4
1. Complete Gates D, F, G.
2. Add `focused-stable` and `focused-extended` mode profiles.
3. Expand `internal/mcp` tests and contract coverage.

### Weeks 5-6
1. Normalize SAP quality outputs (ATC/dumps/traces) into one schema.
2. Add evidence bundle format and compatibility tests.
3. Introduce idempotency semantics for mutating operations.

### Weeks 7-8
1. Reliability drills (load/failure/shutdown).
2. Publish baseline SLOs and operational metrics.
3. Freeze production profile and release readiness checklist.

## Success Metrics
1. 100% mutating operations emit auditable structured events.
2. 0 policy bypasses across mutating paths in automated tests.
3. P95 latency and error-rate budgets defined and met for focused-stable tools.
4. Tool-catalog drift reduced to zero by generation + CI checks.
5. Security scans clean on every PR.

## Build / Integrate / Ignore (Final)

Build:
1. Contracts, audit, policy-complete enforcement, safe defaults, runtime controls, test coverage.

Integrate:
1. Orchestrators (including Coral), CI systems, evidence consumers.

Ignore / Deprioritize:
1. Native multi-agent orchestration.
2. Deep ownership of Jira/Cloud ALM workflows.
3. Feature-count expansion before reliability gates are done.

## Final Directive
- Standalone-first remains the product strategy.
- External orchestrators are optional adapters.
- Operational trust is the gating objective for relevance.
