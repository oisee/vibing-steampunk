# Coral Lessons, vsp Outcome: Operational Reliance and Relevance

**Date:** 2026-02-07  
**Intent:** Learn from Coral operating patterns while keeping vsp standalone-first.

## Core Principle
Adopt Coral's **operational patterns** (governance, reliability loops, measurable outcomes), not Coral dependency.

## What to Learn and Apply to vsp

### 1) Contract-first runtime behavior
Coral-style systems depend on deterministic machine contracts. vsp currently mixes response styles:
- plain text error helper: `internal/mcp/server.go:2446`
- prose-heavy output in some handlers: `internal/mcp/handlers_system.go:99`
- JSON-as-text in others: `internal/mcp/handlers_search.go:31`

**Action:** enforce one versioned response envelope for every tool call.

### 2) Evidence-first operations
Operational trust comes from lineage and replayability.

**Action:** emit structured audit events for each operation:
- `operation_id`, `correlation_id`, actor, tool, target object, policy decision, outcome, timestamps.

### 3) Quality ingestion pattern for SAP
Coral ingests quality/security sources. vsp can do SAP-native equivalent.

**Action:** normalize ATC, dumps, traces into one issue/event schema and expose a meta-output for any orchestrator.

### 4) Reliability engineering as product feature
Coral-like systems survive retries, bursts, and flaky dependencies.

**Action:** add runtime controls:
- rate limits and concurrency budgets
- health/readiness status
- retry/backoff policy with clear retryability flags
- graceful shutdown/drain semantics

### 5) Policy-complete safety enforcement
Current package policy exists but is not uniformly guaranteed across all write paths.
- policy engine: `pkg/adt/safety.go:170`
- update/edit paths to harden: `pkg/adt/workflows.go:1291`, `pkg/adt/workflows.go:2084`, `pkg/adt/crud.go:113`

**Action:** make policy checks comprehensive for all mutating operations.

### 6) Outcome metrics over tool-count metrics
Relevance is earned by measurable delivery impact, not number of tools.

**Action:** track and publish:
- change success rate
- mean time to recover failed runs
- policy violation rate
- P95 tool latency by operation type
- percent of runs producing complete evidence bundles

## Standalone Positioning (Updated)
- vsp is the independent SAP execution runtime.
- It is compatible with Coral-like orchestrators, but not coupled to any one orchestrator.
- Operational maturity is the main differentiator.

## 60-Day Execution Plan

### Weeks 1-2
1. Define and implement unified response envelope.
2. Add operation and correlation IDs to all tool executions.
3. Add baseline audit event sink.

### Weeks 3-4
1. Add health/readiness tool.
2. Add rate limiting and concurrency caps.
3. Add graceful shutdown/drain behavior.

### Weeks 5-6
1. Normalize SAP quality outputs (ATC/dumps/traces) into one schema.
2. Add evidence bundle format.
3. Add contract tests for core mutating tools.

### Weeks 7-8
1. Publish reliability dashboard metrics.
2. Run load/failure drills and tune budgets.
3. Freeze a "production profile" config preset.

## Decision Rule
Learn from Coral's operating discipline.  
Build those capabilities natively in vsp.  
Keep vsp independently valuable with or without Coral.
