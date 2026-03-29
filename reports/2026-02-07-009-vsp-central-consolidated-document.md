# VSP Central Consolidated Document (2026-02-07)

**Date:** 2026-02-07  
**Consolidates:** `2026-02-07-001`, `2026-02-07-002`, `2026-02-07-003`, `2026-02-07-004`, `2026-02-07-004a`, `2026-02-07-005`, `2026-02-07-006`, `2026-02-07-007`, `2026-02-07-008`  
**Intent:** Single coherent strategic and technical source of truth.

## Executive Summary
vsp is a high-capability SAP ADT runtime with real technical differentiation, but it is not yet enterprise-operational. The correct consolidated strategy is: **standalone-first product identity**, **Coral-informed operational discipline**, and **optional orchestrator integrations**. The core value is SAP-native execution depth; the core blockers are operational reliability and governance readiness (audit trail, machine contracts, policy completeness, runtime controls, and security hygiene). Success depends less on adding tools and more on making existing capabilities deterministic, governable, and measurable.

## 1) What All Reports Agree On (Consolidated Facts)

1. Core ADT execution capability is real and valuable.
Evidence appears repeatedly across `001`, `002`, `003` for CRUD/search/test/activation depth and dual REST+WS architecture.

2. Current enterprise-readiness claims are overstated.
Common blockers: no structured audit trail, weak operational controls, tool-contract inconsistency, and CI/security gaps.

3. Tool-count messaging is noisy and trust-eroding.
- CLI banner drift: `cmd/vsp/main.go:32`
- Runtime tool registration sprawl: `internal/mcp/server.go:224`
- Static config catalog mismatch: `cmd/vsp/config_cmd.go:753`

4. Security hygiene issues must be treated as immediate defects.
- Credential in report artifact: `reports/2025-12-21-007-phase5-live-experiment.md:26`
- Committed log artifact: `amdp-breakpoint-test-results-20251206-084536.log`

5. Architectural debt is concentrated in two god modules.
- `internal/mcp/server.go`
- `pkg/adt/workflows.go`

## 2) Resolved Contradictions From Today’s Reports

### 2.1 Coral-Centered vs Standalone-Centered
Resolved position:
- vsp is **not** Coral-dependent.
- vsp should still learn from Coral’s operating model (contract discipline, evidence, reliability loops).
- Coral is an integration path, not product identity.

Canonical rule:
- **Coral-compatible: yes.**
- **Coral-dependent: no.**

### 2.2 `/CBA/` Namespace Enforcement
Resolved position:
- Report 002’s original "missing" assessment is incorrect.
- Package wildcard policy exists (`pkg/adt/safety.go:170`, `pkg/adt/safety.go:200`) and is configurable (`cmd/vsp/main.go:78`, `cmd/vsp/main.go:310`).
- Coverage is partial across mutating paths.
Package-aware checks exist in create/workflow paths (`pkg/adt/crud.go:313`, `pkg/adt/workflows.go:220`, `pkg/adt/workflows.go:313`), while edit/update paths are not equivalently package-gated (`pkg/adt/workflows.go:1291`, `pkg/adt/workflows.go:2084`, `pkg/adt/crud.go:113`).

### 2.3 AMDP "lock-shadowing bug" confidence level
Resolved position:
- Treat as **high-risk hypothesis**, not fully proven defect from static analysis alone.
- Requires targeted concurrency test harness to confirm deadlock/race conditions.

### 2.4 Positioning breadth
Resolved position:
- Reject "execution layer for everything".
- Keep focus on SAP execution runtime and reliability moat.

## 3) Canonical Positioning (Final)

**vsp is an independent, single-binary SAP ADT execution runtime for AI-assisted and agentic ABAP engineering.**

What this includes:
1. SAP-native execution surface across read/write/debug/deploy.
2. Safety controls for constrained operation.
3. Optional integrations with external orchestrators and enterprise systems.

What this excludes:
1. Owning full enterprise orchestration platforms.
2. Positioning around raw tool count.
3. "Production-ready" claims before hardening gates are met.

## 4) Consolidated Strategic Direction

### 4.1 Product Strategy
1. Compete on execution reliability, policy enforcement, and operational trust.
2. Keep AI/orchestration layer pluggable.
3. Treat integrations as adapters at boundaries, not core runtime entanglement.

### 4.2 Technical Strategy
1. Contract-first architecture.
2. Evidence-first operations.
3. Policy-complete safety.
4. Runtime control plane for health, rate, retry, and shutdown.

### 4.3 Market Strategy
1. Lead with standalone value.
2. Present Coral as one of several ecosystem fits.
3. Emphasize SAP frontier coverage and governance-grade execution.

## 5) Consolidated Roadmap

### Phase 0: Trust and Safety Baseline (1-2 weeks)
1. Remove sensitive artifacts and add secret scanning gate.
2. Unified response envelope for all tools.
3. Structured audit event model with operation IDs.
4. Graceful shutdown behavior and session cleanup.
5. Tool catalog single source of truth.

Gate to exit Phase 0:
- No known secret leaks.
- Deterministic response schema in core tools.
- Audit records for all mutating operations.

### Phase 1: Operational Runtime Hardening (2-4 weeks)
1. Health/readiness contract.
2. Rate limiting and concurrency budgets.
3. Retry taxonomy and idempotency guidance.
4. Policy-complete enforcement across all mutating flows.
5. Expand MCP orchestration test coverage significantly.

Gate to exit Phase 1:
- Stable behavior under synthetic concurrency/load tests.
- Policy bypasses closed for update/edit paths.
- Measurable MTTR and failure-rate improvements.

### Phase 2: Integration-Ready Platform (4-8 weeks)
1. Evidence bundle schema for tests, activation, transport lineage.
2. SAP quality ingestion normalization (ATC, dumps, traces).
3. Adapter contracts for orchestrators/CI (Coral optional).
4. Operations metrics dashboard and reliability SLOs.

Gate to exit Phase 2:
- At least one real integration consuming evidence bundles successfully.
- SLOs defined and met in pilot runs.

## 6) Consolidated Risk Register (Top)

1. Enterprise quality bar failure.
Likelihood: High. Impact: High.
Mitigation: Phase 0/1 hardening before expansion.

2. Security review rejection.
Likelihood: Medium. Impact: High.
Mitigation: immediate cleanup, scanning, documented security posture.

3. Policy coverage gaps causing unsafe writes.
Likelihood: Medium. Impact: High.
Mitigation: enforce package/policy checks uniformly across all mutating paths.

4. Runtime instability under concurrent agent load.
Likelihood: Medium. Impact: High.
Mitigation: limiter budgets, retry semantics, load and chaos tests.

5. Strategic drift into non-core features.
Likelihood: High. Impact: Medium.
Mitigation: roadmap gate: no net-new tool families before hardening gates pass.

## 7) Consolidated Kill List

1. Stop leading with "99 tools" in strategic messaging.
2. Stop "execution layer for everything" positioning.
3. Stop "production-ready" phrasing until readiness gates are objectively met.
4. Stop adding broad new capability areas before core runtime hardening is complete.
5. Remove stale or contradictory strategy artifacts from active narrative (archive where needed).

## 8) Success Metrics (Unified)

1. Change success rate.
2. Policy violation/prevention rate.
3. P95 latency by critical tool family.
4. Mean time to recover failed runs.
5. Evidence bundle completeness rate.
6. Test coverage growth in `internal/mcp` and WS-critical code.

## 9) Immediate 14-Day Execution Checklist

1. Delete/rotate sensitive artifacts and credentials.
2. Implement and enforce versioned tool response envelope.
3. Add audit event emission for all mutating operations.
4. Add graceful shutdown and readiness/health primitive.
5. Fix tool-catalog drift with generated registry/config/docs.
6. Publish a concise "standalone-first, integration-ready" positioning page.

## 10) Final Consolidated Stance

vsp should stand on its own feet as an independent SAP execution runtime.  
vsp should absorb Coral’s operational lessons (not Coral dependency).  
Operational trust is now the critical path to relevance.
