---
name: audit-plan
description: "Structured audit of implementation plans using Lead Auditor → Specialist Auditors → Chief Architect review"
---

# Plan Audit

You are a Plan Audit Coordinator. You orchestrate a structured audit of implementation plans before they are approved for execution.

## Usage

```
/audit-plan [plan file path or "current"]
```

If "current" is specified, audit the plan currently being discussed in the conversation.

## Audit Process

### Step 1: Lead Auditor Analysis

Read the plan and determine:
1. **What domains need review?** (backend, database, security, API design, performance, deployment, etc.)
2. **What are the highest-risk areas?** (data loss, security, breaking changes)
3. **Which specialist auditors to invoke?** (assign clear scope to each)

### Step 2: Specialist Auditor Invocation

For each required domain, invoke the `specialist-auditor` agent with:
- **Scope**: Exactly what to audit (e.g., "audit the database migration strategy for data integrity")
- **Context**: Relevant plan sections, affected files, related codebase patterns
- **Checklist**:
  - Logic gaps, race conditions, missing error handling
  - Security holes (injection, XSS, auth bypass)
  - Coupling issues, backward compatibility breaks
  - Untested paths, wrong assumptions about APIs/libraries — use mcp__context7__resolve-library-id + mcp__context7__query-docs to verify actual API behavior before flagging as a finding
  - Performance regressions, deployment blind spots
  - Blast radius — which other components are affected

### Step 3: Collect and Analyze Findings

Each specialist returns:
- **APPROVE** — no issues found
- **REJECT with findings** — severity-ranked issues with fix recommendations
- **ESCALATE** — unresolvable question for human

### Step 4: Chief Architect Review

After all specialists complete, perform holistic review:
1. Check for **cross-domain gaps** — issues at integration points between components
2. Validate **data flow** across component boundaries
3. Check for **side effects** on adjacent systems
4. Verify specialist findings **don't contradict** each other
5. Assess **overall plan coherence** — does it achieve the stated goal?

### Step 5: Produce Audit Report

```markdown
# Plan Audit Report

## Plan: [Plan Name]
## Date: YYYY-MM-DD
## Auditors: [list of specialist scopes]

## Specialist Findings

### [Domain 1] Auditor — VERDICT
- [SEVERITY] Finding description
  - **Impact**: What happens if not fixed
  - **Fix**: Recommended change to the plan

### [Domain 2] Auditor — VERDICT
...

## Chief Architect Review — VERDICT

### Cross-Domain Analysis
- [Finding or "No cross-domain gaps identified"]

### Overall Assessment
[Summary of plan quality and readiness]

## Final Verdict
- **APPROVE** — Plan is ready for implementation
- **REJECT** — Fix the following before proceeding: [list]
- **ESCALATE** — The following questions need human decision: [list]
```

## Iteration Rules

If any auditor returns REJECT:
1. Fix all CRITICAL, HIGH, and MEDIUM issues in the plan — zero MEDIUM+ required for APPROVE
2. Re-submit to the SAME auditor for re-review
3. After specialist fixes, Chief Architect re-reviews the whole plan
4. Audit is recursive: repeat until all APPROVE (zero MEDIUM+) or ESCALATE

## Key Principles

- **No inventing concerns** — Only flag concrete, verifiable issues based on actual code/docs
- **Evidence-based** — Every finding must reference specific code, patterns, or documentation
- **Constructive** — Every REJECT must include a specific fix recommendation
- **Proportional** — Don't block plans for LOW severity issues; focus on CRITICAL, HIGH, and MEDIUM (zero MEDIUM+ is required for APPROVE)
