# SCMON + SUSG for Code-Level Change Impact in VSP

**Date:** 2026-02-07  
**Purpose:** Targeted research on using SCMON/SUSG to map production runtime behavior to specific changed ABAP code and define how to incorporate that into vsp.

## Executive Summary
SCMON is the right primary signal for production-reality impact because it captures dynamic procedure usage, request entry points, caller/callee relationships, and time evolution slices. SUSG is the long-horizon aggregation and governance layer, not the detailed call-chain layer. For vsp, the best design is a new runtime-usage toolset that correlates git/ADT changes to procedure-level SCMON usage and entry points, then augments with SAT/SQLM where full stack or statement-level precision is required.

## Direct Answers to the 6 Runtime Questions

1. **Which specific code paths through the changed object are actually executed in production?**  
   SCMON can answer this at **procedure/path level** (dynamic caller/callee and used procedures), not perfect statement-level execution.

2. **How frequently each path is called?**  
   SCMON provides usage frequency counters and time evolution slices for observed paths.

3. **Which entry points (transactions, URLs, RFCs) trigger the changed code?**  
   SCMON records request entry points; this is one of its core differentiators for custom code impact.

4. **What the actual call stack looks like at runtime (not static where-used)?**  
   SCMON gives dynamic relations but not always full frame-by-frame stacks for each execution. For full runtime stack reconstruction, pair SCMON-selected entry points with SAT/ST12 traces.

5. **Time-of-day patterns for changed code?**  
   SCMON provides time-evolution slices (daily granularity). For finer intra-day analysis, combine with SQLM snapshots and/or targeted SAT windows.

6. **Whether changed code is in hot paths vs cold/error paths?**  
   Use SCMON call counts + entry-point diversity + recurrence over time as base signal. Add SQLM source-position hits and SAT hitlist timing to classify hot/cold with higher confidence.

## What SCMON/SUSG Are Good At (and Not)

### SCMON strengths
1. Dynamic usage (what actually runs, not static where-used).
2. Request entry point linkage.
3. Caller/callee usage relationships and call-position navigation.
4. Time evolution slices.

### SCMON limits
1. Primary granularity is procedure/path, not exact changed line block.
2. Time evolution is not a full high-resolution telemetry system.
3. Large volumes require operational controls (aggregation, export strategy).

### SUSG strengths
1. Aggregation/normalization of usage for governance and custom code migration.
2. Snapshot handling and long-range usage view.
3. Strong complement to SCMON, especially for “used vs unused” over long periods.

### SUSG limits
1. Not a replacement for detailed dynamic chain analysis.
2. Best viewed as summary/aggregation layer, not per-change forensic trace layer.

## Correlating SCMON to Specific Changed Code Blocks

## Correlation model (recommended)
1. Extract changed blocks from git/ADT diff:
   - `object_uri`, include/program, `start_line`, `end_line`, patch hash.
2. Map each block to owning ABAP procedure:
   - class method / function module / form / method include section.
3. Query SCMON by procedure + time window:
   - execution counts, first/last seen, day slices.
4. Query SCMON dynamic relations:
   - callers -> changed procedure -> callees.
5. Query SCMON request entry points for those relations:
   - transaction, URL/service, RFC trigger contexts.
6. Build a change impact graph:
   - `entry_point -> caller_path -> changed_procedure -> downstream_calls`.
7. Compute risk indicators:
   - hotness, entry-point breadth, business-hour concentration, cold-path likelihood.

### Precision caveat
If only part of a procedure changed, SCMON tells you the procedure ran, not necessarily that every changed statement ran.  
Mitigation: run SAT/ST12 traces for top SCMON entry points and map stack/source positions back to changed ranges.

## Proposed VSP Design (Concrete)

### Existing building blocks in vsp
1. Static call graph already exists:
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/client.go:965`
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/handlers_analysis.go:16`
2. Runtime trace comparison already exists (non-SCMON):
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/client.go:1278`
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/handlers_analysis.go:216`
3. SQL trace hooks exist:
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/client.go:1997`
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go:881`
4. WebSocket RFC execution exists (usable integration rail):
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/pkg/adt/websocket_rfc.go:20`
   - `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/embedded/abap/zcl_vsp_rfc_service.clas.abap:169`

### Add new runtime-usage toolset
Create a dedicated tool family (focused-expert split as needed):
1. `GetRuntimeUsageForChange`
2. `ListSCMONProcedureUsage`
3. `ListSCMONEntryPoints`
4. `ListSCMONCallRelations`
5. `GetSCMONTimeEvolution`
6. `GetSUSGAggregates`
7. `ClassifyChangeHotness`

### Add ABAP-side usage service (recommended)
1. New class: `ZCL_VSP_USAGE_SERVICE` (new domain: `usage`) or new actions in RFC domain.
2. Purpose: expose SCMON/SUSG data in stable JSON for MCP consumption.
3. Prefer released APIs where available; fallback with strict compatibility checks.
4. Return schema should include:
   - `changed_procedure`
   - `execution_count`
   - `entry_points[]`
   - `call_relations[]`
   - `time_series[]`
   - `confidence` + `limitations`.

### MCP/Go wiring
1. Add `pkg/adt/websocket_usage.go` client calls.
2. Add `internal/mcp/handlers_usage.go`.
3. Register tools in `/Users/VincentSegami/Documents/GitHub/vibing-steampunk/internal/mcp/server.go`.
4. Add tests:
   - parser/normalization unit tests
   - contract tests for response envelope
   - policy tests for restricted package/object scopes.

## Operational Prerequisites and Pitfalls

1. **Request entry point capture must be enabled correctly**  
   SAP KBA preview indicates missing request-entry-point type can occur unless profile parameter configuration is correct.

2. **SCMON retention behavior matters**  
   Community guidance indicates in-memory retention and need for regular persistence/aggregation cycles.

3. **Volume/export limits exist**  
   KBA preview highlights ALV export issues around very large result sets; avoid ad hoc manual export as primary integration path.

4. **Do not rely on SCMON alone for full stack forensics**  
   Use SAT/ST12 for full stack/source-level trace on top SCMON entry points.

5. **Governance**  
   Restrict usage data queries by package/namespace and enforce auditability of who queried what.

## Implementation Plan (Phased)

### Phase 1 (1-2 weeks): Read-only runtime usage evidence
1. Add ABAP usage service returning:
   - procedure usage counts
   - entry points
   - call relations
   - daily time evolution.
2. Add MCP tools:
   - `ListSCMONProcedureUsage`
   - `ListSCMONEntryPoints`
   - `ListSCMONCallRelations`
3. Add integration tests with sample JSON fixtures.

### Phase 2 (1-2 weeks): Change-level impact correlation
1. Add `GetRuntimeUsageForChange` composite tool:
   - input: changed object/procedure ranges.
   - output: dynamic impact graph + hotness classification.
2. Integrate with existing static/dynamic comparison path in `TraceExecution`.

### Phase 3 (1 week): SUSG horizon + confidence model
1. Add `GetSUSGAggregates` for long-window validation.
2. Compute confidence flags:
   - `coverage_complete`
   - `entry_point_confidence`
   - `needs_sat_trace`.

## Suggested MCP Output Schema (for AI agents)
```json
{
  "change_id": "abc123",
  "object": "ZCL_FOO",
  "changed_blocks": [
    { "procedure": "ZCL_FOO=>BAR", "start_line": 120, "end_line": 158 }
  ],
  "runtime_impact": {
    "executed_in_window": true,
    "total_calls": 42107,
    "entry_points": [
      { "type": "TCode", "name": "VA01", "calls": 19811 },
      { "type": "URL", "name": "/sap/bc/ui2/flp", "calls": 10102 },
      { "type": "RFC", "name": "Z_API_ORDER_CREATE", "calls": 12194 }
    ],
    "call_relations": [
      { "caller": "CL_SOMETHING=>RUN", "callee": "ZCL_FOO=>BAR", "calls": 42050 }
    ],
    "time_series_daily": [
      { "date": "2026-02-01", "calls": 5011 }
    ],
    "hotness": "HOT",
    "confidence": 0.82,
    "limitations": [
      "Procedure-level evidence; statement-level execution not guaranteed",
      "Use SAT for full stack confirmation"
    ]
  }
}
```

## Key Sources

### SAP Help / Official docs
1. Usage Data Collection for ABAP Custom Code Migration (SCMON + SUSG):  
   https://help.sap.com/docs/ABAP_PLATFORM_NEW/fc4c71aa50014fd1b43721701471913d/e0a1b1c3f79643d6ba4df719576de431.html
2. ABAP Test Cockpit Custom Code Migration Guide (PDF; request entry points, aggregation context):  
   https://help.sap.com/doc/34796706f38646f68d51a0fa0d4636e4/100/en-US/abap_testcockpit_custom_code_migration_guide.pdf
3. SQL Monitor usage and options (request entry points, source positions, snapshots):  
   https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abenusing_sql_monitor.htm  
   https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abenusing_sql_monitor_options.htm
4. ABAP Runtime Analysis (call hierarchy, source assignment, measurement scheduling):  
   https://help.sap.com/saphelp_snc700_ehp01/helpdata/en/18/0f1f48935711d194b200a0c94260a5/content.htm  
   https://help.sap.com/saphelp_snc700_ehp01/helpdata/en/18/0f1f4b935711d194b200a0c94260a5/content.htm  
   https://help.sap.com/saphelp_snc700_ehp01/helpdata/en/8f/a8432f98ea11d295980000e8353423/content.htm

### SAP KBAs / SAP Community (operational behavior)
1. KBA preview: request entry point type issues in SCMON:  
   https://userapps.support.sap.com/sap/support/knowledge/en/3425828
2. KBA preview: SCMON display/export large-volume limits:  
   https://userapps.support.sap.com/sap/support/knowledge/en/3538741
3. KBA preview: SUSG/SCMON no data scenarios:  
   https://userapps.support.sap.com/sap/support/knowledge/en/3423696
4. SAP Community: SCMON operational walkthrough and usage patterns:  
   https://community.sap.com/t5/application-development-and-automation-blog-posts/scmon-step-by-step/ba-p/13443043  
   https://community.sap.com/t5/enterprise-resource-planning-blogs-by-sap/abap-test-cockpit-and-scmon-in-sap-s-4hana/bc-p/13306974

## Bottom Line
To understand code-level production impact of a specific change, vsp should treat SCMON as the runtime truth source, SUSG as aggregation/governance, and SAT/SQLM as precision amplifiers. Implement this as a first-class runtime-usage toolset with change correlation output optimized for AI decisioning.
