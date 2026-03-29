# Codex Re-Evaluation Addendum — CBA AI Maturity + Coral Alignment

**Date:** 2026-02-07  
**Report ID:** 005  
**Type:** Addendum to `2026-02-07-001` and `2026-02-07-002` (not a full rewrite)  
**Trigger:** Execution of `2026-02-07-004-codex-reeval-prompt-and-gap-analysis.md`

## Executive Summary
The CBA context materially changes the strategic read: the primary uncertainty is no longer "will CBA adopt agentic engineering," but "can vsp meet Coral-grade operational standards fast enough to be adopted before CBA builds its own SAP adapter." The core bridge value remains strong, but the differentiator shifts from "standalone MCP server" to "SAP domain adapter invocable by Project Coral." Prior critical findings on operational maturity still hold (audit trail, structured outputs, shutdown behavior, contract consistency), and they now become urgent because CBA already operates at production AI scale.

## Scope And Assumptions
- This addendum recalibrates prior findings using the CBA AI context provided in Report 004.
- CBA external claims are treated as input assumptions for this addendum.
- Code evidence is validated directly in this repo and referenced with file/line pointers.

---

## 1) Revised Assumption Challenge Scores

| Item | Prior Position (001/002) | Revised Score | Why This Changed | Code Evidence Impact |
|---|---|---|---|---|
| Multi-agent viability | Weak / deprioritize | **Viable at CBA specifically** | Coral already solves orchestration for non-SAP; SAP lock coordination is a bounded adapter problem | vsp should expose machine-safe contracts and lock metadata, not build an orchestrator |
| Confidence-based routing | Fails as governance model | **Viable with ABAP-specific calibration** | CBA already has confidence governance machinery; ABAP thresholds need empirical tuning | vsp must emit evidence artifacts and deterministic outcomes |
| Cultural shift | Major org challenge | **Mostly solved org-wide; SAP-team alignment remains** | CBA already runs hybrid teams at scale; remaining issue is SAP enclave adoption | vsp needs SAP-native governance affordances, not culture programs |
| `/CBA/` namespace enforcement | Missing | **Implemented (config-driven), but generic** | `AllowedPackages` wildcard gate already supports `/CBA/*` | `pkg/adt/safety.go:170`, `pkg/adt/safety.go:200`, `cmd/vsp/main.go:78`, `cmd/vsp/main.go:310`, `internal/mcp/server.go:127` |
| Positioning | Best standalone bridge | **SAP adapter for Coral/CBA AI engineering stack** | CBA already has orchestration layer; vsp should be a composable SAP execution component | Product messaging and roadmap should shift toward integration primitives |
| Kill-list focus | Ignore orchestration and stay standalone | **Do not build orchestrator; build Coral-compatibility primitives** | In CBA context, compatibility features are core (structured output, traceability, retry safety) | Current output/error contract is not machine-strong enough |
| Risk register completeness | Missing CBA build/quality-bar risks | **Add medium-high strategic risks immediately** | CBA has team scale to build internal adapter and quality standards from Coral | Requires acceleration on reliability posture and governance evidence |

### Net Re-Score
- **What moved from weak to viable:** multi-agent execution and confidence routing (for CBA specifically, not universally).
- **What did not change:** vsp is not yet enterprise-operational by Coral standards.
- **What became more urgent:** contract hardening, observability, and auditability.

---

## 2) Revised Positioning Recommendation (Coral-Aware)

### Recommended Positioning Statement
**"vsp is the SAP ADT execution adapter for Project Coral-style agentic workflows: it brings ABAP into CBA's existing AI engineering system."**

### What To Stop Claiming
- "Execution layer for everything"
- "Standalone orchestrator"
- "Production-ready" (without qualifiers)

### What To Claim Instead (Evidence-Backed)
- "Single-binary SAP ADT MCP bridge with broad ABAP operational surface"
- "Safety controls already present (ops, packages, transport gating)"
- "Designed to be orchestrated by external systems (Coral), not to replace them"

### Supporting Code References
- Safety model and package gates: `pkg/adt/safety.go:8`, `pkg/adt/safety.go:170`, `pkg/adt/safety.go:200`
- CLI policy controls: `cmd/vsp/main.go:74`, `cmd/vsp/main.go:78`, `cmd/vsp/main.go:79`, `cmd/vsp/main.go:82`
- External orchestration boundary: MCP stdio server startup `cmd/vsp/main.go:223`, `internal/mcp/server.go:201`

---

## 3) Revised Risk Register (CBA-Specific Additions)

| Risk | Likelihood | Impact | Why It Matters At CBA | Mitigation |
|---|---|---|---|---|
| CBA builds its own SAP MCP adapter | Medium | High | CBA has engineering scale and existing Coral architecture | Beat internal build timeline with Coral-ready contract and pilot readiness |
| Coral team evaluates vsp as immature | High | High | Coral quality bar is already operational at scale | Ship P0 hardening (audit, health, output contracts, shutdown) before outreach |
| Coral subsumes vsp scope | Medium | Medium | If Coral adds SAP adapter, vsp loses differentiation | Focus on deep SAP expertise and fastest reliable adapter path |
| SAP Basis blocks ZADT_VSP deployment | Medium | High | WebSocket path may face stricter controls than REST | Lead with REST-first pilot; phase WS after security review |
| Repo trust failure from sensitive artifacts | Medium | High | Banking security review will fail quickly on hygiene gaps | Remove leaked credentials/artifacts, add secret scanning gate |
| Contract instability breaks orchestration | High | High | Coral needs deterministic, parseable responses | Versioned JSON envelope and stable error taxonomy |

### Evidence for security/credibility concerns
- Sensitive credential in report artifact: `reports/2025-12-21-007-phase5-live-experiment.md:26`
- Committed log artifact in repo root: `amdp-breakpoint-test-results-20251206-084536.log`
- `.gitignore` does not ignore `*.log`: `.gitignore:64`

---

## 4) "Coral-Ready" Technical Requirements

These are the minimum technical conditions for reliable invocation by an external orchestrator.

### R1. Deterministic Machine-Readable Envelope (P0)
- Current gap:
  - Errors are plain text: `internal/mcp/server.go:2446`
  - Some tools return prose blocks: `internal/mcp/handlers_system.go:99`
  - Others return JSON-as-text blobs: `internal/mcp/handlers_search.go:31`
- Requirement:
  - Standard envelope for every tool response (`ok/error`, typed `data`, typed `error`, `retryable`, `operation_id`).

### R2. Correlation/Audit Context Propagation (P0)
- Current gap:
  - No explicit correlation/request lineage fields across handlers.
  - No operation-level audit subsystem in runtime paths.
- Requirement:
  - Accept `correlation_id`/`work_item_id` and persist for every mutating operation + critical reads.
  - Append immutable audit records with actor, tool, target object, outcome, timestamp, and hash of key params.

### R3. Health/Readiness Contract (P0)
- Current gap:
  - System info tools exist (`GetSystemInfo`, `GetConnectionInfo`) but no explicit orchestrator health contract: `internal/mcp/server.go:648`, `internal/mcp/server.go:664`
- Requirement:
  - `GetHealth` with structured status (`sap_reachable`, `session_valid`, `ws_available`, `latency_ms`, `degraded_reasons[]`).

### R4. Idempotency Semantics (P0/P1)
- Current gap:
  - Create/update tools do not expose idempotency keys or replay handling semantics.
- Requirement:
  - Document idempotent vs non-idempotent operations.
  - Add optional idempotency token for create/release paths.

### R5. Admission Control / Rate Limiting (P1)
- Current gap:
  - No request budget controls to protect SAP backend under concurrent agents.
- Requirement:
  - Per-tool and global concurrency caps, queue/backpressure signals, retry-after hints.

### R6. Graceful Shutdown + In-Flight Safety (P0)
- Current gap:
  - process starts stdio server directly without shutdown orchestration: `cmd/vsp/main.go:223`, `internal/mcp/server.go:201`
- Requirement:
  - SIGTERM/SIGINT drain policy, in-flight cancellation/flush, final audit flush.

### R7. Contract Consistency + Tool Inventory Integrity (P0)
- Current gap:
  - Config catalog and registered tool set diverge:
    - Config static list omits tools like `GetAbapHelp` and `RunQuery`: `cmd/vsp/config_cmd.go:753`
    - Tools are registered in server: `internal/mcp/server.go:551`, `internal/mcp/server.go:676`
- Requirement:
  - Single source of truth for tool registry generated into both runtime and config UX.

### R8. Baseline Safety Defaults Review (P1)
- Current gap:
  - Unrestricted safety default remains for backward compatibility: `internal/mcp/server.go:114`, `pkg/adt/config.go:188`
- Requirement:
  - Introduce deployment profiles (`safe-default`, `dev`, `unrestricted`) with explicit startup warning and policy export.

---

## 5) Revised Roadmap Priorities (Given CBA AI Maturity)

### What Moves Up
1. **Coral compatibility primitives (contracts, audit, health, correlation, shutdown).**
2. **Security hygiene and trust readiness (secret/artifact cleanup + scanning).**
3. **Tool inventory consistency and compatibility tests.**

### What Moves Down
1. Native multi-agent orchestration inside vsp.
2. Lua-led orchestration narrative as core differentiator.
3. Deep platform integrations (Jira/Cloud ALM APIs) owned better by CBA systems.

### Phased Plan

#### Phase 0 (1-2 weeks): "Credibility Gate"
- Remove sensitive artifacts and add secret scan checks.
- Ship unified response envelope + error taxonomy.
- Add correlation IDs + append-only audit log.
- Add `GetHealth` and readiness semantics.
- Add graceful shutdown behavior.

#### Phase 1 (2-4 weeks): "Coral Invocation Readiness"
- Add idempotency guidance/keys for mutating operations.
- Add concurrency/rate controls.
- Publish Coral integration contract (tool contract + sample runbook + failure modes).
- Fix registry drift (single source tool catalog generation).

#### Phase 2 (4-8 weeks): "Scale + Governance"
- Build evidence bundle exporter for ABAP changes (ATC/tests/activation/transport lineage).
- Add structured telemetry for usage and failure patterns.
- Validate REST-first pilot path; WS path after Basis security review.

---

## 6) Additional Opportunities And Gaps (Fresh)

### Opportunities
1. **ATC-as-Sonar bridge artifact**: treat `RunATCCheck` output as Coral-ingestable quality findings stream.
2. **SAP debt feed meta-tool**: aggregate dumps/traces/ATC into one normalized issue payload.
3. **REST-first pilot package**: immediate value without ZADT_VSP deployment friction.
4. **Responsible-AI fit narrative**: existing safety controls align naturally with constrained-agent operation.

### Gaps Still Underestimated
1. **Operational contract quality**, not feature breadth, is the adoption bottleneck.
2. **Security review package** for Basis/AppSec is not yet productized.
3. **Concurrency behavior under fleet usage** is unproven.
4. **Test distribution remains thin in orchestration layer** (`internal/mcp/server_test.go` has only 3 tests: `internal/mcp/server_test.go:9`, `internal/mcp/server_test.go:34`, `internal/mcp/server_test.go:64`).

### Trust-Erosion Signals To Fix Immediately
- Tool-count messaging is stale/conflicting:
  - CLI banner says "19 focused / 45 expert": `cmd/vsp/main.go:32`
  - Server focused list comment says "41 essential tools" while map includes substantially more entries: `internal/mcp/server.go:283`

---

## 7) SAP As The Missing Frontier (Why This Still Matters)

CBA's agentic maturity does not remove vsp's value; it **clarifies** it.

### Why SAP Is Different
- ABAP changes are not pure file diffs; they require ADT semantics, object activation, transport constraints, lock handling, and SAP-specific runtime workflows.
- Generic code agents can orchestrate work items, but they cannot safely execute SAP-native operations without a domain adapter.

### Strategic Implication
- If Coral is the orchestration brain, vsp can be the SAP execution hand.
- This is a defensible role only if vsp behaves like production infrastructure: deterministic contracts, traceability, safety controls, and predictable failure modes.

### Hard Truth
- The strategic window exists because SAP is under-covered in mainstream agentic pipelines.
- The window closes quickly if CBA internal teams build a comparable adapter first.

---

## Final Recommendation (Addendum)
- **Reposition now**: "SAP execution adapter for Coral-style workflows."  
- **Prioritize now**: contract/audit/health/shutdown/security hygiene over net-new tool count.  
- **Measure now**: pilot success by operational reliability and governance evidence, not demo breadth.

