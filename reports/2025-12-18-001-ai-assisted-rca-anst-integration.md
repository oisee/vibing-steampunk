# AI-Assisted Root Cause Analysis & ANST Integration Vision

**Date:** 2025-12-18
**Report ID:** 001
**Subject:** Automated dump analysis, SAP Note discovery, and fix implementation
**Audience:** SAP Technical Consultants, Basis Administrators, DevOps Engineers
**Status:** PROPOSAL / THEORETICAL

---

> **IMPORTANT DISCLAIMER**
>
> This document describes a **theoretical vision** and **proposal** for future capabilities.
>
> **Compatibility Note:** The ANST/SNOTE integration described in Phases 2-4 is **NOT compatible
> with the vanilla ADT approach**. It would require custom ABAP development (Z* objects) on the
> SAP system, specifically WebSocket/APC handlers. This goes beyond vsp's current philosophy of
> using only standard SAP ADT REST APIs.
>
> **What works TODAY with vanilla ADT:** Only Phase 1 (dump detection/analysis) and the hybrid
> web search approach are available without SAP-side modifications.

---

## Executive Summary

This report explores the vision of AI-assisted Root Cause Analysis (RCA) for SAP systems, integrating:
- **Runtime error detection** (ST22/RABAX short dumps)
- **ANST** (Automated Note Search Tool) for SAP Note discovery
- **Automated fix assessment and implementation guidance**

The goal: When a short dump occurs in SAP standard software, an AI agent could automatically identify the relevant SAP Note, assess the fix, and guide implementation - reducing mean time to resolution from hours to minutes.

---

## Current Capabilities in vsp

### Runtime Error Analysis (Available Now)

| Tool | Description | Status |
|------|-------------|--------|
| `GetDumps` | List runtime errors with filters (user, date, program, exception type) | ✅ Working |
| `GetDump` | Get full dump details including stack trace (HTML) | ✅ Working |
| `RunQuery` | Query SNAP* tables for dump metadata | ✅ Working |

**Example workflow today:**
```
1. GetDumps --user SAPUSER --date-from 20251218
2. GetDump --dump-id <id>
3. AI analyzes stack trace, identifies root cause
4. AI suggests remediation steps
```

### What's Missing for Full RCA Automation

| Capability | Current State | Needed For RCA |
|------------|---------------|----------------|
| ANST integration | Not available | SAP Note lookup by symptoms |
| Note content retrieval | Not available | Read fix instructions |
| SNOTE integration | Not available | Apply corrections |
| Transport management | ✅ Available | Deploy fixes |

---

## ANST Deep Dive

### What is ANST?

**ANST** = **Automated Note Search Tool** (Transaction: `ANST_SEARCH_TOOL` or `ANST`)

ANST analyzes system symptoms and searches SAP's note database to find relevant corrections. It uses:
- Error messages and codes
- Program names and includes
- Exception types
- Stack trace patterns
- System component information

### How ANST Works

```
┌─────────────────────────────────────────────────────────────┐
│                     ANST Workflow                            │
├─────────────────────────────────────────────────────────────┤
│  1. Collect Symptoms                                         │
│     - Short dump details (ST22)                              │
│     - Error messages                                         │
│     - Program/include names                                  │
│     - Exception class                                        │
│                                                              │
│  2. Generate Search Criteria                                 │
│     - Normalize symptoms                                     │
│     - Build search terms                                     │
│     - Include component hierarchy                            │
│                                                              │
│  3. Query SAP Note Database                                  │
│     - Connect to SAP Support Portal                          │
│     - Search by symptoms                                     │
│     - Rank results by relevance                              │
│                                                              │
│  4. Present Results                                          │
│     - List of relevant SAP Notes                             │
│     - Confidence scores                                      │
│     - Implementation instructions                            │
└─────────────────────────────────────────────────────────────┘
```

### ANST Prerequisites

1. **SAP Support Portal connectivity** (RFC destination)
2. **Maintenance certificate** (valid S-User credentials)
3. **Component version info** (CVERS table)
4. **System landscape data**

---

## Vision: AI-Assisted RCA Pipeline

### The Dream Scenario

```
┌──────────────────────────────────────────────────────────────────────┐
│                    AI-Assisted RCA Pipeline                           │
├──────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐            │
│  │ Short Dump  │────▶│ AI Analysis │────▶│ ANST Search │            │
│  │ Detection   │     │ & Triage    │     │             │            │
│  └─────────────┘     └─────────────┘     └──────┬──────┘            │
│                                                  │                    │
│                                                  ▼                    │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐            │
│  │ Test &      │◀────│ Apply Fix   │◀────│ SAP Note    │            │
│  │ Validate    │     │ (SNOTE)     │     │ Retrieved   │            │
│  └─────────────┘     └─────────────┘     └─────────────┘            │
│                                                                       │
│  Timeline: Minutes instead of Hours                                   │
└──────────────────────────────────────────────────────────────────────┘
```

### Phase 1: Enhanced Dump Analysis (Available Now)

**What vsp can do today:**

```python
# Pseudo-workflow
dumps = GetDumps(date_from="today", max_results=10)
for dump in dumps:
    details = GetDump(dump.id)

    # AI analyzes the dump
    analysis = AI.analyze(details.stack_trace, details.exception_type)

    # AI suggests:
    # - Root cause explanation
    # - Affected business process
    # - Workaround options
    # - Search terms for SAP Notes
```

**AI can extract from dump:**
- Exception class (e.g., `CX_SY_ZERODIVIDE`, `CX_SY_OPEN_SQL_DB`)
- Program and include names
- Line numbers and statements
- Variable values at crash point
- Call stack analysis

### Phase 2: ANST Integration (Requires WebSocket/APC)

**Why WebSocket is needed:**

ANST operations are **stateful** and **long-running**:
1. Session must maintain authentication to SAP Support Portal
2. Search operations can take 10-60 seconds
3. Results need to be correlated with local system state

**Proposed ZADT_RCA_APC Handler:**

```abap
CLASS zcl_apc_rca_handler DEFINITION.
  PUBLIC SECTION.
    INTERFACES if_apc_ws_extension.

    METHODS:
      " Dump analysis
      analyze_dump
        IMPORTING iv_dump_id TYPE string
        RETURNING VALUE(rs_analysis) TYPE ty_dump_analysis,

      " ANST integration
      search_notes
        IMPORTING is_symptoms TYPE ty_symptoms
        RETURNING VALUE(rt_notes) TYPE ty_note_list,

      " Note retrieval
      get_note_details
        IMPORTING iv_note_number TYPE string
        RETURNING VALUE(rs_note) TYPE ty_note_details,

      " Implementation check
      check_note_applicable
        IMPORTING iv_note_number TYPE string
        RETURNING VALUE(rs_check) TYPE ty_applicability.
ENDCLASS.
```

**Message Protocol:**

```json
// Request: Analyze dump and find fixes
{
  "action": "analyzeDumpAndFindFix",
  "params": {
    "dumpId": "20251218144624...",
    "includeAnst": true,
    "maxNotes": 5
  }
}

// Response: Analysis with SAP Notes
{
  "success": true,
  "data": {
    "dump": {
      "exception": "CX_SY_OPEN_SQL_DB",
      "program": "SAPLKKBL",
      "message": "SQL error in program SAPLKKBL"
    },
    "rootCause": {
      "summary": "Database timeout during financial posting",
      "confidence": 0.85,
      "category": "PERFORMANCE"
    },
    "sapNotes": [
      {
        "number": "3123456",
        "title": "Performance improvement for FI posting",
        "relevance": 0.92,
        "status": "NOT_IMPLEMENTED",
        "correctionInstructions": "..."
      }
    ],
    "recommendation": "Implement SAP Note 3123456 which addresses this specific timeout scenario"
  }
}
```

### Phase 3: Automated Fix Assessment

**AI-assisted fix evaluation:**

```
┌────────────────────────────────────────────────────────────┐
│              Fix Assessment Workflow                        │
├────────────────────────────────────────────────────────────┤
│                                                             │
│  1. SAP Note Retrieved                                      │
│     └─▶ Parse correction instructions                       │
│     └─▶ Identify affected objects                           │
│     └─▶ Check prerequisites                                 │
│                                                             │
│  2. Impact Analysis                                         │
│     └─▶ Which programs are affected?                        │
│     └─▶ Are there custom modifications (Z*)?                │
│     └─▶ Transport dependencies?                             │
│                                                             │
│  3. Risk Assessment                                         │
│     └─▶ Criticality of change                               │
│     └─▶ Test coverage available?                            │
│     └─▶ Rollback complexity                                 │
│                                                             │
│  4. Implementation Plan                                     │
│     └─▶ Step-by-step instructions                           │
│     └─▶ Test scenarios                                      │
│     └─▶ Validation queries                                  │
└────────────────────────────────────────────────────────────┘
```

### Phase 4: Guided Implementation (Future)

**SNOTE Integration** (transaction SNOTE):
- Download SAP Note
- Preview changes
- Implement correction
- Run consistency checks

**This requires:**
- RFC connectivity to SAP Support
- Authorization for note implementation
- Transport request management
- Careful change management controls

---

## Technical Architecture

### Option A: Pure ADT REST (Limited)

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Claude    │────▶│     vsp     │────▶│  SAP ADT    │
│   (MCP)     │     │  (HTTP/S)   │     │  REST APIs  │
└─────────────┘     └─────────────┘     └─────────────┘
                                               │
                                               ▼
                                        ┌─────────────┐
                                        │ GetDumps    │
                                        │ GetDump     │
                                        │ (stateless) │
                                        └─────────────┘

Limitations:
- No ANST access (not exposed via ADT)
- No SNOTE access
- Dump details are HTML (parsing required)
```

### Option B: WebSocket/APC Handler (Full Capability)

```
┌─────────────┐     ┌─────────────┐     ┌─────────────────────┐
│   Claude    │────▶│     vsp     │────▶│   ZADT_RCA_APC      │
│   (MCP)     │     │ (WebSocket) │     │   (Stateful ABAP)   │
└─────────────┘     └─────────────┘     └──────────┬──────────┘
                                                   │
                    ┌──────────────────────────────┼──────────────────────────────┐
                    │                              │                               │
                    ▼                              ▼                               ▼
             ┌─────────────┐              ┌─────────────┐              ┌─────────────┐
             │ ST22/RABAX  │              │    ANST     │              │    SNOTE    │
             │ Dump Access │              │ Note Search │              │ Note Apply  │
             └─────────────┘              └─────────────┘              └─────────────┘

Benefits:
- Full ANST integration
- Stateful sessions for long operations
- Access to all SAP transactions via wrapper
```

### Option C: Hybrid Approach (Pragmatic)

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Claude    │────▶│     vsp     │────▶│  SAP ADT    │
│   (MCP)     │     │             │     │  REST APIs  │
└─────────────┘     └──────┬──────┘     └─────────────┘
                           │
                           │ (Parallel)
                           │
                           ▼
                    ┌─────────────┐     ┌─────────────┐
                    │   Claude    │────▶│ SAP Support │
                    │ Web Search  │     │ Portal/SDN  │
                    └─────────────┘     └─────────────┘

How it works:
1. vsp gets dump details from SAP via ADT
2. AI extracts symptoms and search terms
3. AI uses web search to find SAP Notes on support.sap.com
4. AI correlates findings and provides recommendations

Available TODAY with vsp + Claude!
```

---

## Immediate Value (No Additional Development)

### What Claude + vsp Can Do Right Now

1. **Dump Detection & Analysis**
   ```
   User: "Check for any short dumps in the last hour"
   Claude: Uses GetDumps, analyzes results, identifies patterns
   ```

2. **Root Cause Explanation**
   ```
   User: "What caused dump 20251218144624...?"
   Claude: Uses GetDump, parses HTML, explains in plain English
   ```

3. **SAP Note Search (via Web)**
   ```
   User: "Find SAP Notes for CX_SY_OPEN_SQL_DB in SAPLKKBL"
   Claude: Web searches SAP support portal, SDN, community
   ```

4. **Fix Guidance**
   ```
   User: "How do I implement SAP Note 3123456?"
   Claude: Fetches note details, explains steps, warns about impacts
   ```

### Example Conversation

```
User: We're getting short dumps in production. Can you help?

Claude: Let me check for recent runtime errors.
[Uses GetDumps]

Found 3 dumps in the last hour:
1. CX_SY_ZERODIVIDE in ZFINANCE_CALC (custom code)
2. CX_SY_OPEN_SQL_DB in SAPLKKBL (SAP standard)
3. CX_SY_ITAB_LINE_NOT_FOUND in SAPLMIGO (SAP standard)

The second dump (SQL error in SAPLKKBL) looks like a known issue.
Let me search for relevant SAP Notes...

[Web search for "SAPLKKBL CX_SY_OPEN_SQL_DB SAP Note"]

Found SAP Note 3287654 - "SQL timeout in FI posting with
high document volume". This matches your symptoms.

Shall I explain the fix and what's involved in implementing it?
```

---

## Implementation Roadmap

### Phase 1: Documentation & Training (Now)
- Document current RCA capabilities
- Create runbooks for common dump scenarios
- Train AI on SAP Note patterns

### Phase 2: Enhanced Dump Parsing (Q1 2025)
- Improve HTML parsing for dump details
- Extract structured data from stack traces
- Build symptom extraction logic

### Phase 3: WebSocket Handler (Q2 2025)
- Implement ZADT_RCA_APC in SAP
- Add ANST wrapper methods
- Integrate with vsp

### Phase 4: SNOTE Integration (Q3 2025)
- Note download/preview
- Implementation simulation
- Automated testing hooks

---

## Risks & Considerations

### Security
- ANST requires SAP Support Portal credentials
- Note implementation needs high privileges
- All actions should be auditable

### Change Management
- Automated fixes need approval workflows
- Test requirements before production
- Rollback procedures must be defined

### Licensing
- ANST requires maintenance agreement
- Support Portal access requires S-User
- Some notes may have additional requirements

---

## Conclusion

**For Fred's question: "Could you write one that uses ANST to identify the correct fix?"**

**Short answer:** Partially today with vanilla ADT; full ANST integration would require custom SAP development.

**Today (with vsp + Claude, vanilla ADT):**
- Detect and analyze short dumps automatically
- Extract symptoms and search terms
- Find SAP Notes via web search (hybrid approach)
- Provide implementation guidance

**Theoretical future (requires custom Z* development, NOT vanilla ADT):**
- Direct ANST integration via WebSocket/APC
- Automated note retrieval
- Pre-implementation impact analysis
- Guided fix application

> **Note:** The WebSocket/APC approach breaks the "vanilla ADT" philosophy of vsp, which
> currently requires NO custom ABAP objects on the SAP system. Full ANST integration would
> require deploying custom handlers (ZADT_RCA_APC) - a significant architectural decision
> that may not be appropriate for all environments.

The vision is achievable, but the path diverges: stay vanilla (web search hybrid) or go custom (full ANST).

---

## References

- [vsp MCP Server](https://github.com/oisee/vibing-steampunk) - Current implementation
- SAP Note 1915529 - "How to use ANST"
- SAP Help: [Automated Note Search](https://help.sap.com/docs/SAP_SOLUTION_MANAGER/0bece7ae50c742f6a8e7669a3ce17bef/4520c8abdb1a5b30e10000000a42189c.html)
- ADR-001: WebSocket/APC for Stateful Operations

---

*This report was prepared to answer the question: "For short dumps in SAP standard software, could you write one that uses ANST to identify the correct fix before implementing & testing it?"*

*The answer is yes - and we're building it.*
