# Review of Claude SCMON/SUSG Plan Against VSP Code

**Date:** 2026-02-07  
**Reviewed Input:** "SCMON & SUSG Deep Dive — vsp Integration Research" (dated 2026-02-08 in source text)  
**Goal:** Decide what is immediately actionable in vsp, what needs correction, and what depends on live SAP validation.

## Executive Verdict
Claude's plan is strategically strong and directionally correct, but not implementation-ready as written for current vsp. The biggest gaps are transport mechanics: current `CallRFC` payload shape and ABAP RFC handler parsing are too limited for robust `RFC_READ_TABLE`-style table-parameter workflows, and `RunQuery`-based access to SCMON/SUSG artifacts is environment-dependent. The right path is to keep the architecture, but implement through a dedicated ABAP usage service (`usage` domain) rather than generic RFC table scraping.

## Findings (Priority Ordered)

### [P0] `RFC_READ_TABLE` path is not reliable with current vsp RFC interface
Evidence:
1. Go client `CallRFC` only accepts `map[string]string` parameters:  
   `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/websocket_rfc.go:20`
2. ABAP parameter extraction is string-only regex extraction (`extract_param`):  
   `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/embedded/abap/zcl_vsp_rfc_service.clas.abap:559`
3. TABLES parameters are created but not populated from structured JSON input in handler:  
   `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/embedded/abap/zcl_vsp_rfc_service.clas.abap:220`

Impact:
1. Complex RFC calls requiring structured table inputs (fields/options rows, iterators) are fragile or impossible in current shape.
2. Plan items depending on generic RFC table extraction must be downgraded until protocol expansion.

Correction:
1. Do not center initial implementation on `RFC_READ_TABLE`.
2. Add dedicated ABAP usage service API returning normalized JSON.

### [P0] `RunQuery` access to SCMON/SUSG is possible but not guaranteed
Evidence:
1. `RunQuery` exists and uses ADT freestyle SQL endpoint:  
   `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/client.go:647`
2. Tool is safety-gated and can be blocked by config/safety policy:  
   `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/client.go:649`  
   `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/cmd/vsp/main.go:75`
3. Tool description already warns ABAP SQL syntax constraints:  
   `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:554`

Impact:
1. "Phase 1 zero ABAP change via RunQuery" is valid only if target systems expose required objects and policy allows free SQL.

Correction:
1. Treat RunQuery path as opportunistic bootstrap, not canonical integration path.
2. Include readiness probe before enabling SCMON/SUSG tools.

### [P1] Plan assumes external SAP object/API details that must be validated on target systems
Impact:
1. Table names/views/function modules may vary by release, add-on, or authorization.
2. Hardcoding undocumented FM paths increases break risk.

Correction:
1. Mark as environment assumptions and validate via live-system capability checks.
2. Use feature detection tool before runtime usage queries.

### [P1] Good integration target identified: `TraceExecution` composite flow
Evidence:
1. Composite runtime+static pipeline already exists:  
   `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/client.go:1278`
2. Tool exposed in MCP layer:  
   `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:780`  
   `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/handlers_analysis.go:216`

Impact:
1. SCMON/SUSG integration can reuse existing output model and reduce design churn.

### [P2] Date mismatch in source plan
Observation:
1. Input plan is dated `2026-02-08`, while current repo work context is `2026-02-07`.

Impact:
1. Low technical impact, but documentation chronology should stay consistent.

## What to Keep from Claude Plan (Adopt As-Is)
1. Procedure-level impact focus (`METH/FUNC/FORM`) instead of object-only counts.
2. Entry-point correlation as first-class risk signal.
3. Hot/cold/dead classification model.
4. SCMON recent-window + SUSG long-window complement model.
5. Pre-write impact advisory workflow.

## What to Modify Before Execution
1. Replace "RFC_READ_TABLE as fallback baseline" with "dedicated usage service baseline."
2. Keep RunQuery fallback only for systems that pass capability checks.
3. Add explicit "statement-level certainty requires SAT/ST12" caveat in all user-facing outputs.
4. Gate tools behind read-only usage policy by default.

## Revised VSP Implementation Plan (Code-Fit)

### Phase 1 (1-2 weeks): Capability + Schema + Service Stub
1. Add capability probe tool:
   - `GetRuntimeUsageCapabilities`
   - checks query access, SCMON/SUSG availability, auth posture, service availability.
2. Add ABAP usage service stub in embedded ABAP:
   - new class `ZCL_VSP_USAGE_SERVICE`
   - register domain in APC handler.
3. Define stable JSON schema for usage outputs and uncertainty fields.

### Phase 2 (1-2 weeks): Core Tools
1. `ListSCMONProcedureUsage`
2. `ListSCMONEntryPoints`
3. `GetSUSGAggregates`
4. `GetRuntimeUsageForChange`

Wiring:
1. `pkg/adt/websocket_usage.go`
2. `internal/mcp/handlers_usage.go`
3. registration in `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go`

### Phase 3 (1 week): Integration + Risk Scoring
1. Integrate usage impact summary into `TraceExecution`.
2. Add `ClassifyChangeHotness`.
3. Add pre-write advisory hooks for `WriteSource` and `Activate` (advisory first, enforcement later).

## Minimal Technical Changes Required
1. Expand WebSocket protocol for structured params (if retaining generic RFC route), or skip and use dedicated service.
2. Add new handler file + registration entries.
3. Add unit tests for parser/normalizer and contract tests for new tools.

## Unknowns That Require Live SAP Validation
1. Presence and accessibility of SCMON/SUSG artifacts in target landscape.
2. Authorization boundaries for usage data reads.
3. Performance characteristics on production-like volume.
4. Data completeness when root/request correlation settings are off.

## Final Recommendation
Adopt Claude's strategic model, but execute with a dedicated usage-service integration path. Keep RunQuery and generic RFC access as optional fallback mechanisms, not the core design.
