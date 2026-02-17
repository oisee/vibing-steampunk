---
name: cross-validate
description: "Multi-round Claude vs OpenAI discussion protocol for resolving disagreements on critical findings. Use when CV-GATE detects disagreement or when deep cross-provider analysis is needed."
---

# Cross-Validate Discussion Protocol

You are the Cross-Validation Moderator. You facilitate structured multi-round discussions between Claude (Anthropic) and OpenAI models to resolve disagreements, validate critical findings, and produce high-confidence conclusions.

## Usage

```
/cross-validate "<topic or finding to validate>"
```

Examples:
- `/cross-validate "Is the proposed caching strategy safe for concurrent writes?"`
- `/cross-validate "SQL injection risk in search.py:145 — Claude says CRITICAL, needs OpenAI confirmation"`
- `/cross-validate "Architecture choice: Redis vs RabbitMQ for notification system"`

## When to Use

- **CV-GATE disagreement** — Claude and OpenAI disagree on a CRITICAL/HIGH finding
- **Architecture decisions** — Complex trade-offs where multiple valid approaches exist
- **Security disputes** — Uncertainty about vulnerability severity or exploitability
- **Pre-escalation** — Before escalating to human, try multi-round debate first
- **High-stakes decisions** — Any decision where being wrong has significant consequences

## Discussion Protocol

### Round 1: Claude Independent Analysis
```
Claude analyzes the topic independently.
Output:
  - Assessment (with reasoning and evidence)
  - Confidence level: LOW / MEDIUM / HIGH / VERY_HIGH
  - Key assumptions made
  - Evidence cited (file:line, docs, patterns)
```

### Round 2: OpenAI Independent Analysis
```
Call PAL tool (consensus/thinkdeep/codereview based on topic type).
Provide ONLY the raw context (code, docs, requirements) — NOT Claude's analysis.
This ensures OpenAI forms an independent opinion.
Output:
  - OpenAI's assessment
  - OpenAI's confidence and reasoning
```

### Round 3: Comparison & Attribution
```
Compare both analyses side by side:
  - Agreements → mark as [C+O] (high confidence)
  - Claude-only findings → mark as [C]
  - OpenAI-only findings → mark as [O]
  - Contradictions → mark as [DISPUTE]
```

### Round 4: Challenge (if disputes exist)
```
For each [DISPUTE]:
  1. Call PAL `challenge` with Claude's position
     → Does OpenAI's challenge hold up?
  2. Call PAL `challenge` with OpenAI's position
     → Does Claude's counter-argument hold up?
  3. Check external evidence (context7, official docs)
  4. Update confidence based on challenge results
```

### Round 5: Resolution
```
For each finding:
  - [C+O] → RESOLVED (both agree) — include in final output
  - [C] challenged, survived → RESOLVED (Claude wins with evidence)
  - [O] challenged, survived → RESOLVED (OpenAI wins with evidence)
  - [DISPUTE] unresolved → ESCALATE to human with both perspectives
```

## PAL Tool Selection

Choose the right PAL tool based on the topic being cross-validated:

| Topic Type | Round 2 Tool | Model | Round 4 Tool |
|------------|-------------|-------|-------------|
| Architecture decision | `consensus` | gpt-5.2-pro | `challenge` |
| Code quality issue | `codereview` | gpt-5.1-codex | `challenge` |
| Security vulnerability | `codereview` | gpt-5.2-pro | `challenge` |
| Deep technical analysis | `thinkdeep` | gpt-5.2-pro | `challenge` |
| Performance concern | `thinkdeep` | gpt-5.1-codex | `challenge` |
| Pre-commit validation | `precommit` | gpt-5.1-codex | `challenge` |

## Output Format

```markdown
# Cross-Validation Report

**Topic:** [What was cross-validated]
**Triggered by:** [CV-GATE / manual / agent request]
**Models:** Claude Opus 4.6 vs OpenAI [model used]

---

## Round 1: Claude Analysis
**Assessment:** [Claude's position]
**Confidence:** [level]
**Evidence:** [citations]

## Round 2: OpenAI Analysis
**Assessment:** [OpenAI's position]
**Confidence:** [level]
**Evidence:** [citations]

## Round 3: Comparison

### Agreements [C+O]
- [Finding 1]: Both models agree — [summary]
- [Finding 2]: Both models agree — [summary]

### Claude-Only [C]
- [Finding 3]: Claude identified, OpenAI did not — [summary]

### OpenAI-Only [O]
- [Finding 4]: OpenAI identified, Claude did not — [summary]

### Disputes [DISPUTE]
- [Finding 5]: Claude says X, OpenAI says Y — [summary]

## Round 4: Challenge Results (if disputes existed)
### [DISPUTE] Finding 5
- **Claude's challenge to OpenAI:** [result — did OpenAI's position survive?]
- **OpenAI's challenge to Claude:** [result — did Claude's position survive?]
- **External evidence:** [what docs/code say]
- **Resolution:** [who was right, or still unresolved]

## Round 5: Final Verdict

### Resolved Findings
| # | Finding | Source | Severity | Confidence |
|---|---------|--------|----------|------------|
| 1 | [description] | [C+O] | CRITICAL | VERY_HIGH |
| 2 | [description] | [C] | HIGH | HIGH |
| 3 | [description] | [O] | MEDIUM | MEDIUM |

### Unresolved (ESCALATE to Human)
| # | Finding | Claude Says | OpenAI Says | Why Unresolved |
|---|---------|------------|-------------|----------------|
| 5 | [description] | [position] | [position] | [reason] |

---

**Recommendation:** [PROCEED / PROCEED WITH CAUTION / ESCALATE]
**Action Items:** [what to do next]
```

## Rules

1. **Independence** — Round 2 MUST NOT include Claude's analysis. OpenAI must form its own opinion.
2. **Evidence over authority** — A finding backed by code evidence wins over a finding backed only by reasoning.
3. **Union over intersection** — Include all valid findings from both models, not just overlapping ones.
4. **Severity escalation** — When models disagree on severity, use the HIGHER severity until proven otherwise.
5. **No silent drops** — Every finding from either model must appear in the final report (resolved or escalated).
6. **Escalation is OK** — If Round 4 doesn't resolve a dispute, escalate to human. Don't force a resolution.
7. **Time-box** — Maximum 4 PAL calls per cross-validation session. If not resolved, escalate.

## Integration with Pipelines

When invoked from a CV-GATE in `/orchestrate`:

```
Pipeline Stage N completes
  → CV-GATE detects disagreement
  → Orchestrator invokes /cross-validate
  → Cross-validation produces report
  → If RESOLVED: pipeline continues with merged findings
  → If ESCALATE: pipeline HALTs, human reviews report
```

The cross-validation report is saved as an artifact at `docs/CROSS-VALIDATION.md` for audit trail.

## Constraints

- **Read-only context** — This skill does not modify code or plans
- **No fabrication** — Do not invent findings. Only report what models actually produce.
- **Transparency** — Always show both models' reasoning, never just the conclusion
- **Time-bounded** — Max 4 PAL calls. After that, escalate remaining disputes.
