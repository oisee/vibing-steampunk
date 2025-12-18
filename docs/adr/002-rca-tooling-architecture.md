# ADR-002: Root Cause Analysis Tooling Architecture

**Date:** 2025-12-18
**Status:** PROPOSAL / THEORETICAL
**Context:** AI-assisted RCA for SAP runtime errors and ANST integration

---

> **IMPORTANT: NOT VANILLA ADT COMPATIBLE**
>
> This ADR describes a **theoretical architecture** that would require **custom ABAP
> development** (Z* objects, WebSocket/APC handlers) on the SAP system.
>
> **Phases 2-3 break the vanilla ADT philosophy** - vsp currently requires NO custom
> SAP objects and works purely via standard ADT REST APIs.
>
> Only **Phase 1** (enhanced dump parsing) is compatible with the vanilla approach.

---

## Summary

This ADR defines the architecture for Root Cause Analysis (RCA) tooling in vsp, enabling AI agents to automatically analyze runtime errors, search for relevant SAP Notes via ANST, and guide fix implementation.

## Context

### Current State

vsp provides basic runtime error access:
- `GetDumps` - List short dumps with filtering
- `GetDump` - Retrieve full dump details (HTML format)

### Problem

1. Dump details are returned as HTML - difficult to parse programmatically
2. No integration with ANST for SAP Note discovery
3. No way to assess fix applicability
4. Manual correlation between dumps and solutions

### Opportunity

SAP Basis consultants spend significant time on:
1. Analyzing short dumps
2. Searching for relevant SAP Notes
3. Assessing fix applicability
4. Planning implementation

AI can automate much of this workflow.

## Decision

Implement RCA tooling in three phases:

### Phase 1: Enhanced Dump Analysis (ADT REST)

**New/Improved Tools:**

| Tool | Description |
|------|-------------|
| `GetDumpStructured` | Parse dump HTML into structured JSON |
| `AnalyzeDumpSymptoms` | Extract search-ready symptoms |
| `GetDumpHistory` | Track recurring dumps |

**Structured Dump Format:**
```json
{
  "id": "20251218...",
  "timestamp": "2025-12-18T14:46:24Z",
  "exception": {
    "class": "CX_SY_OPEN_SQL_DB",
    "message": "SQL error occurred"
  },
  "location": {
    "program": "SAPLKKBL",
    "include": "LKKBLF01",
    "line": 1234,
    "statement": "SELECT * FROM BKPF..."
  },
  "stack": [
    {"program": "SAPLKKBL", "event": "FORM_POST_DOCUMENT", "line": 1234},
    {"program": "SAPMF05A", "event": "MODULE_USER_COMMAND", "line": 567}
  ],
  "variables": {
    "SY-SUBRC": "4",
    "SY-DBCNT": "0"
  },
  "symptoms": [
    "CX_SY_OPEN_SQL_DB",
    "SAPLKKBL",
    "SELECT BKPF",
    "FI posting"
  ]
}
```

### Phase 2: ANST Integration (WebSocket/APC)

**Requires:** ZADT_RCA_APC handler on SAP side

**New Tools:**

| Tool | Description |
|------|-------------|
| `SearchSAPNotes` | Query ANST with symptoms |
| `GetNoteDetails` | Retrieve full note content |
| `CheckNoteApplicability` | Verify note applies to system |
| `GetNotePrerequisites` | List required notes/support packs |

**ANST Search Request:**
```json
{
  "action": "searchNotes",
  "params": {
    "symptoms": ["CX_SY_OPEN_SQL_DB", "SAPLKKBL"],
    "component": "FI-GL",
    "releaseFrom": "7.50",
    "maxResults": 10
  }
}
```

### Phase 3: Fix Assessment & Guidance (WebSocket/APC)

**New Tools:**

| Tool | Description |
|------|-------------|
| `AssessFixImpact` | Analyze what note changes |
| `SimulateNoteImpl` | Preview without applying |
| `GenerateTestPlan` | Create test scenarios |
| `GetImplementationGuide` | Step-by-step instructions |

## Architecture

```
┌────────────────────────────────────────────────────────────────────┐
│                         MCP Client (Claude)                         │
└────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌────────────────────────────────────────────────────────────────────┐
│                            vsp Server                               │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐             │
│  │  RCA Tools   │  │  Dump Tools  │  │  Note Tools  │             │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘             │
│         │                 │                 │                       │
│         ▼                 ▼                 ▼                       │
│  ┌─────────────────────────────────────────────────────────┐       │
│  │              Transport Abstraction Layer                 │       │
│  │   ┌─────────────────┐    ┌─────────────────────────┐    │       │
│  │   │  HTTP (Phase 1) │    │  WebSocket (Phase 2-3)  │    │       │
│  │   └─────────────────┘    └─────────────────────────┘    │       │
│  └─────────────────────────────────────────────────────────┘       │
└────────────────────────────────────────────────────────────────────┘
                     │                           │
                     ▼                           ▼
          ┌──────────────────┐       ┌──────────────────────┐
          │   ADT REST API   │       │   ZADT_RCA_APC       │
          │   /sap/bc/adt/   │       │   (WebSocket)        │
          │   runtime/dumps  │       │                      │
          └──────────────────┘       └──────────┬───────────┘
                                                │
                    ┌───────────────────────────┼───────────────────────────┐
                    │                           │                           │
                    ▼                           ▼                           ▼
          ┌──────────────────┐       ┌──────────────────┐       ┌──────────────────┐
          │    ST22/RABAX    │       │       ANST       │       │      SNOTE       │
          │   (Dump Access)  │       │   (Note Search)  │       │   (Note Apply)   │
          └──────────────────┘       └──────────────────┘       └──────────────────┘
```

## SAP Objects Required (Phase 2-3)

| Object | Type | Description |
|--------|------|-------------|
| `$ZADT_RCA` | Package | RCA functionality container |
| `ZADT_RCA_APC` | SICF Service | WebSocket endpoint |
| `ZCL_APC_RCA_HANDLER` | Class | IF_APC_WS_EXTENSION impl |
| `ZCL_RCA_ANST_WRAPPER` | Class | ANST function wrapper |
| `ZCL_RCA_SNOTE_WRAPPER` | Class | SNOTE function wrapper |
| `ZCL_RCA_DUMP_ANALYZER` | Class | Structured dump parsing |

## Security Considerations

1. **Authorization**
   - ANST access requires S_TCODE for ANST_SEARCH_TOOL
   - SNOTE access requires note implementation auth
   - All RCA operations logged

2. **Data Sensitivity**
   - Dump data may contain business data
   - SAP Notes may be confidential
   - Implement data masking options

3. **Change Control**
   - No automated note implementation without approval
   - Simulation mode by default
   - Audit trail for all actions

## Alternatives Considered

### 1. Direct ANST RFC Calls
- **Pro:** No APC handler needed
- **Con:** Requires RFC destination in vsp, complex auth

### 2. Scraping SAP Support Portal
- **Pro:** No SAP-side development
- **Con:** Fragile, against ToS, limited access

### 3. SAP AI Core Integration
- **Pro:** SAP-supported AI for analysis
- **Con:** Requires BTP, additional licensing

## Consequences

### Positive
- Dramatically reduced MTTR for known issues
- Consistent RCA process
- Knowledge capture in AI interactions
- 24/7 availability of "expert" analysis

### Negative
- Requires SAP-side development (Phase 2-3)
- ANST requires SAP maintenance agreement
- Risk of over-automation without human review

### Risks
- SAP may restrict ANST programmatic access
- Note implementation without proper testing
- False positive note recommendations

## Implementation Priority

| Phase | Effort | Value | Priority |
|-------|--------|-------|----------|
| Phase 1: Dump Parsing | Low | Medium | High |
| Phase 2: ANST | High | High | Medium |
| Phase 3: Fix Assessment | High | High | Low |

## Status

**Phase 1:** Ready to implement with current vsp architecture
**Phase 2-3:** Blocked on ZADT_RCA_APC development

## Related Documents

- ADR-001: WebSocket/APC for Stateful Operations
- Report: 2025-12-18-001-ai-assisted-rca-anst-integration.md
