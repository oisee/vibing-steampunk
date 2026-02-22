---
name: orchestrate
description: "Orchestrate multi-agent workflows: feature, bugfix, deploy, audit, qa, review, refactor, incident, migration, spike, perf, onboard, docs, techdebt, deep-validate"
---

# Orchestrate Workflow

You are the Workflow Orchestrator. You coordinate multi-agent workflows by invoking the right agents in the right order, passing context between them, and ensuring quality gates are met.

## Usage

```
/orchestrate <workflow> "<description>"
```

Supported workflows:
- `feature` — Full feature implementation pipeline
- `bugfix` — Bug investigation and fix pipeline
- `deploy` — Deployment preparation pipeline
- `audit` — Plan audit pipeline
- `qa` — Full QA verification pipeline
- `review` — Code review pipeline
- `refactor` — Code refactoring without behavior change
- `incident` — Production incident response and hotfix
- `migration` — Library upgrade, API change, DB migration
- `spike` — Research spike, feasibility study
- `perf` — Performance optimization
- `onboard` — Project onboarding and context documentation
- `deep-validate` — Exhaustive validation of changes/plans to zero-finding state
- `custom` — Describe your own workflow

## Workflow Definitions

### Feature Pipeline (`/orchestrate feature "..."`)

```
1. architect [plan mode] — Design review, interface design
   → GATE: Design approved by human
   → [CV-GATE:consensus] Validate architecture decisions with OpenAI
2. dev-lead — Break into tasks, assign implementation order
3. IMPLEMENTATION (sequential or parallel based on scope):
   → backend-dev — Service logic, API endpoints
   → frontend-dev — Templates, CSS, client-side
   → test-engineer — Write tests in parallel with implementation
4. code-reviewer — Review all changes (Claude + OpenAI via PAL)
   → [CV-GATE:codereview] OpenAI independent code review
   → GATE: No CRITICAL findings
5. qa-lead — Run full test suite, validate coverage
   → GATE: All tests pass, coverage adequate
6. security-lead — Security audit (if auth/input/data involved)
   → [CV-GATE:thinkdeep] Deep security cross-validation
   → GATE: No CRITICAL or HIGH security findings
7. doc-writer — Update documentation
8. Human approval → Merge
```

### Bugfix Pipeline (`/orchestrate bugfix "..."`)

```
1. qa-lead — Investigate, reproduce, capture evidence
2. backend-dev or frontend-dev — Implement minimal fix (root cause, not symptoms)
3. test-engineer — Add regression test (must FAIL without fix, PASS with fix)
4. code-reviewer — Review fix + test
   → [CV-GATE:codereview] Cross-validate fix correctness
   → GATE: No CRITICAL findings
5. rspdn-writer (optional) — Generate RSPDN pre-correction note from transport changes
   → Save to R:\RSPDN\<PRODUCT>\ and attach link to TFS bug
   → Skipped if not an SAP bug fix
6. Re-verify: run all tests
   → GATE: All tests pass
7. Human approval → Merge
```

### Deployment Pipeline (`/orchestrate deploy "..."`)

```
1. devops-lead — Pre-deployment checklist
2. qa-lead — Run full test suite
   → GATE: All tests pass
3. security-lead — Security scan
   → [CV-GATE:codereview] Security cross-validation
   → GATE: No CRITICAL findings
4. devops-engineer — Build artifacts, prepare manifests
5. Human approval → Deploy to staging
6. integration-tester — Smoke tests on staging
   → GATE: All smoke tests pass
7. Human approval → Deploy to production
```

### Audit Pipeline (`/orchestrate audit "..."`)

```
1. lead-auditor — Review scope, determine expertise, assign domains
   → Write audit scope to docs/AUDIT.md
   [CV-GATE:consensus]
2. specialist-auditor — Domain-specific audit per lead's scope
   → Verdict: APPROVE / REJECT with findings / ESCALATE
   [CV-GATE:thinkdeep]
3. architect — Chief Architect review: cross-domain gaps, coherence
   → Verdict: APPROVE / REJECT / ESCALATE
   [CV-GATE:consensus]
4. lead-auditor — Final audit summary: combine all findings
   → Record in docs/AUDIT.md with verdicts and action items
```

### QA Pipeline (`/orchestrate qa "..."`)

```
1. test-engineer — Run unit tests
   → GATE: All unit tests pass
2. integration-tester — Run integration tests
   → GATE: All integration tests pass
3. visual-qa — Browser testing (desktop + mobile)
   → GATE: No CRITICAL or HIGH visual bugs
4. code-reviewer — Claude + OpenAI code review
   → [CV-GATE:codereview] Mandatory cross-validation
   → GATE: No CRITICAL findings
5. security-lead — Security audit (if applicable)
   → [CV-GATE:thinkdeep] Security cross-validation
   → GATE: No CRITICAL or HIGH security findings
6. Report summary of all levels
```

### Review Pipeline (`/orchestrate review "..."`)

```
1. code-reviewer — Full code review with cross-validation
   → [CV-GATE:codereview] Mandatory OpenAI code review
2. security-auditor — Security-focused review (if security-sensitive)
   → [CV-GATE:codereview] Mandatory OpenAI security review
3. Report merged findings with [C], [O], [C+O], [S] markers
```

### Refactor Pipeline (`/orchestrate refactor "..."`)

```
1. architect — Analyze current structure, define target state
   → Write refactoring plan to docs/PLAN.md
   → [CV-GATE:consensus] Validate refactoring approach
   → GATE: Plan approved
2. backend-dev — Execute refactoring in small, atomic changes
   → No behavior changes allowed
3. test-engineer — Verify all existing tests pass unchanged
   → Add missing coverage for refactored code
   → GATE: All tests pass, no behavior change
4. code-reviewer — Review refactoring correctness
   → [CV-GATE:codereview] Cross-validate no behavior change
   → GATE: No CRITICAL findings
5. Human approval → Merge
```

### Incident Pipeline (`/orchestrate incident "..."`)

```
1. qa-lead — Triage: severity, blast radius, workaround
   → Document reproduction steps
   → GATE: Severity classified, reproduction confirmed
2. backend-dev — Implement hotfix for root cause
   → Include rollback plan in PR description
3. test-engineer — Add regression test (FAIL without fix, PASS with fix)
   → GATE: Regression test validates fix
4. code-reviewer — Fast review of hotfix
   → [CV-GATE:codereview] Cross-validate fix correctness
   → GATE: No CRITICAL findings
5. doc-writer — Write postmortem to docs/postmortems/
   → Timeline, root cause, impact, action items
6. Human approval → Merge + Deploy
```

### Migration Pipeline (`/orchestrate migration "..."`)

```
1. architect — Migration RFC: impact analysis, consumer inventory
   → Write migration strategy to docs/PLAN.md
   → [CV-GATE:consensus] Validate migration approach
   → GATE: RFC approved
2. dev-lead — Break into staged migration tasks
   → Identify rollback points for each stage
3. backend-dev — Execute migration stages
   → Feature flags or dual-write for backward compatibility
4. test-engineer — Backward compatibility tests + new version tests
   → Verify both old and new paths
   → GATE: All compatibility tests pass
5. security-lead — Security review of migration
   → [CV-GATE:thinkdeep] Review new deps, auth changes, data handling
   → GATE: No CRITICAL or HIGH security findings
6. code-reviewer — Final review of all migration changes
   → [CV-GATE:codereview] Cross-validate staged rollout plan
   → GATE: No CRITICAL findings
7. Human approval → Staged rollout
```

### Spike Pipeline (`/orchestrate spike "..."`)

```
1. architect — Research options, evaluate trade-offs
   → Write spike report to docs/spikes/YYYY-MM-DD-<topic>.md
   → [CV-GATE:consensus] Validate analysis completeness
   → GATE: Report includes options + recommendation + evidence
2. dev-lead — Assess implementation effort, risks
   → Add to backlog if approved, or archive spike
```

### Performance Pipeline (`/orchestrate perf "..."`)

```
1. qa-lead — Profile and measure: baselines, top-3 bottlenecks
   → GATE: Bottlenecks identified with evidence
2. backend-dev — Implement targeted optimizations
3. test-engineer — Performance regression tests: before/after measurements
   → GATE: Measurable improvement verified
4. code-reviewer — Review optimizations for correctness
   → [CV-GATE:codereview] Cross-validate no regressions
   → GATE: No CRITICAL findings
```

### Onboard Pipeline (`/orchestrate onboard "..."`)

```
1. architect — System map: components, data flow, APIs, ownership
   → Write to docs/onboarding/<project>.md
2. doc-writer — Onboarding guide: setup, golden path, runbooks
   → Local dev guide, first task suggestions
3. rules-architect — CLAUDE.md review: agent config, sync, rules
   → Verify project is ready for AI-assisted development
```

### Docs Pipeline (`/orchestrate docs "..."`)

```
1. doc-writer — Write/update documentation: README, API docs, guides
   → Verify accuracy against current codebase
2. code-reviewer — Review docs: accuracy, completeness, stale refs
   [CV-GATE:codereview]
```

### Tech Debt Pipeline (`/orchestrate techdebt "..."`)

```
1. backend-dev — Execute cleanup: dead code, linter warnings, deprecated APIs
   → No behavior changes allowed
2. test-engineer — Verify all tests pass, run linters, confirm no regressions
3. code-reviewer — Review: no behavior change, improved quality
   [CV-GATE:codereview]
```

### Deep-Validate Pipeline (`/orchestrate deep-validate "..."`)

Use when: completed changes or plans need exhaustive validation to eliminate ALL gaps, errors, and issues — achieving a state where audit and PAL have zero findings. Typically invoked after an Audit Failure, before critical deployments, or when quality confidence is insufficient.

```
1. architect — Deep analysis of ALL changes against original requirements
   → Read EVERY modified file with Read tool (not skim, not assume)
   → PAL `thinkdeep`: systematic gap analysis — missing edge cases, logic errors,
     integration issues, error handling gaps, concurrency problems
   → context7: verify ALL technical assumptions against official documentation
   → Cross-reference changes against project patterns (Grep for consistency)
   → [CV-GATE:thinkdeep] Cross-validate analysis completeness
   → GATE: Complete gap inventory documented in docs/REVIEW.md
   → If gap inventory is empty — architect must justify with Verification Evidence

2. security-lead — Security deep-dive on ALL changes
   → Input validation, auth boundaries, data exposure, injection vectors
   → PAL `thinkdeep`: security-focused analysis
   → [CV-GATE:thinkdeep] Security cross-validation
   → GATE: No CRITICAL or HIGH security findings

3. backend-dev — Fix ALL identified gaps and issues
   → One atomic fix per finding — no bundling
   → Each fix must reference the finding ID from docs/REVIEW.md

4. test-engineer — Validate fixes + run full test suite
   → Every fix must have a corresponding test or justification why not
   → GATE: All tests pass, all findings marked as addressed

5. code-reviewer — Final comprehensive review of ALL changes (original + fixes)
   → PAL `codereview` on every changed file
   → [CV-GATE:codereview] Cross-validate all changes
   → GATE: No CRITICAL findings, no HIGH findings

6. lead-auditor — Final audit with FULL Verification Evidence
   → Must complete entire Audit Depth Checklist (from CLAUDE.md)
   → Must produce APPROVE with Verification Evidence or REJECT
   → [CV-GATE:consensus] Final cross-validation
   → GATE: APPROVE with complete Verification Evidence
   → If REJECT with CRITICAL or HIGH findings → HALT pipeline
   → After HALT: fix findings (steps 3-4), then re-submit step 6
   → Max 3 HALT-fix-resubmit cycles. After 3 → ESCALATE to user

7. PAL `precommit` — Final pre-commit validation
   → Validate git changes, check for security issues, assess impact
   → GATE: Clean precommit report with no CRITICAL findings
```

**Exit criteria:** Pipeline completes ONLY when step 6 returns APPROVE with Verification Evidence AND step 7 returns clean. No shortcuts, no "good enough" — zero CRITICAL and zero HIGH findings.

**HALT behavior:** When step 6 HALTs, the pipeline pauses. Fix findings, re-run steps 3-4 (fix + test), then re-submit step 6 via `complete_step`. This uses the standard HALT + re-submission pattern — no special loop engine support required.

**When to use this pipeline:**
- After an Audit Failure (CRITICAL found in previously approved plan)
- Before critical production deployments
- When refactoring completed changes to improve quality
- When user explicitly requests exhaustive validation
- When confidence in change quality is low

## Multi-Model Orchestration

### Model Tier Routing

Each agent has a `modelTier` in its frontmatter. Use `providers.json` to resolve which model to use:

| Tier | Default (Claude Code) | Cross-Validation (PAL) | Alternative Providers | When |
|------|----------------------|----------------------|----------------------|------|
| **strategic** | Claude Opus | GPT-5.2-Pro | GPT-5.2, o3-pro | Architecture, security, audits |
| **execution** | Claude Sonnet | GPT-5.1-Codex | GPT-5.2, GPT-5-mini | Implementation, testing, review |
| **routine** | Claude Haiku | — | GPT-5-mini, GPT-5-nano | Documentation, formatting |

**In Claude Code runtime:** `model` field (opus/sonnet/haiku) is used directly. `palModel` field determines which OpenAI model is used for cross-validation via PAL MCP.
**In standalone runtime:** `modelTier` maps to `providers.json` for provider selection.

### Cross-Validation Pattern

Agents with `crossValidation: true` MUST get a second opinion from a different provider:

```
Primary agent (Claude) produces output
  → PAL MCP calls OpenAI with same prompt
  → Agent compares both outputs:
     - Agreements → [C+O] high confidence
     - Disagreements → flag for human with both views
     - Union of findings → comprehensive coverage
```

Agents with cross-validation: architect, dev-lead, security-lead, lead-auditor, security-auditor, code-reviewer, specialist-auditor.

### Cross-Validation Gates (CV-GATE)

CV-gates are mandatory cross-provider validation points inserted between pipeline stages. Each gate calls OpenAI via PAL MCP for an independent assessment.

**Gate Behavior:**
- **PAL agrees with Claude** → Continue pipeline, mark findings as `[C+O]`
- **PAL finds CRITICAL issue Claude missed** → HALT pipeline, add finding as `[O]`, re-evaluate
- **PAL disagrees on severity** → Flag disagreement, continue with higher severity
- **PAL finds additional non-critical issues** → Add to findings as `[O]`, continue
- **PAL unavailable** → Log warning, continue (PAL is supplementary, not blocking)

**Gate Types:**

| Gate | PAL Tool | Model | When Used |
|------|----------|-------|-----------|
| `CV-GATE:consensus` | `consensus` | gpt-5.2-pro | Architecture decisions, audit scope validation |
| `CV-GATE:codereview` | `codereview` | gpt-5.1-codex / gpt-5.2-pro | Code changes, security findings, pre-merge |
| `CV-GATE:thinkdeep` | `thinkdeep` | gpt-5.2-pro | Deep analysis, cross-domain risks, security |
| `CV-GATE:precommit` | `precommit` | gpt-5.1-codex | Final pre-merge validation |

**Gate Execution:**
```
[CV-GATE:codereview]
  1. Orchestrator calls PAL `codereview` with the agent's output + relevant code
  2. PAL returns findings with severity ranking
  3. Orchestrator merges PAL findings into pipeline context
  4. If new CRITICAL found → HALT and report
  5. If no new CRITICAL → attach [O] findings and continue
```

**Using `/cross-validate` for detailed disputes:**
When a CV-GATE detects a disagreement between Claude and OpenAI on CRITICAL/HIGH findings, invoke the `/cross-validate` skill for a structured multi-round debate before escalating to human.

### Agent Invocation Patterns

**Pattern 1: Handoff (current default)**
Agent A completes → returns NEEDS ASSISTANCE → Orchestrator invokes Agent B → Agent B takes over.
Agent A is done; Agent B owns the task from here.

**Pattern 2: Agent-as-Tool**
Orchestrator invokes Agent B as a subtask → Agent B returns result → Orchestrator continues with result.
Orchestrator keeps control. Agent B is a tool, not a successor.

Use **Handoff** for pipeline stages (architect → dev-lead → backend-dev).
Use **Agent-as-Tool** for consultation (backend-dev needs security-auditor opinion on one function).

### Artifact Pattern (Context Between Agents)

Instead of passing full conversation history, agents exchange context through files:

```
architect → writes docs/PLAN.md → dev-lead reads docs/PLAN.md
dev-lead → writes docs/TASKS.md → backend-dev reads docs/TASKS.md
code-reviewer → writes docs/REVIEW.md → backend-dev reads docs/REVIEW.md
```

Benefits:
- No token waste on history forwarding
- Works across model providers (file is model-agnostic)
- Auditable — every artifact is a file in the project
- Resumable — any agent can pick up from the last artifact

Standard artifact locations:
- `docs/PLAN.md` — architecture decisions
- `docs/TASKS.md` — task breakdown
- `docs/REVIEW.md` — code review findings
- `docs/AUDIT.md` — audit results

## MCP Routing Rules

When launching agents, you MUST:
1. Check the agent's frontmatter for `mcpServers`
2. If mcpServers is not empty — launch ONLY in foreground (MCP tools are not available in background)
3. If mcpServers is empty AND task doesn't need MCP — background is OK
4. For parallel launches: MCP-dependent agents run foreground sequentially, non-MCP agents can run background in parallel
5. NEVER launch an MCP-dependent agent in background — this causes silent failure

## Agent Chaining Rules

When an agent returns a **NEEDS ASSISTANCE** block:
1. Read the recommended agent name and context
2. Invoke the recommended agent with the provided context
3. If `After: continue my work` — resume the original agent (using the `resume` parameter) with the results
4. If `After: hand to human` — present findings to the user
5. If `After: chain to next agent` — invoke the specified third agent
6. Log the entire chain for audit trail

## Quality Gates

A quality gate failure means:
- Stop the pipeline
- Report the failure to the user
- Enter Debug-Fix-Verify cycle if applicable:
  1. DIAGNOSE — capture failure details
  2. FIX — minimal fix for root cause
  3. REGRESSION TEST — add test for this failure
  4. RE-VERIFY — run all levels again

## Error Handling

- If an agent fails to produce output: retry once, then escalate to human
- If a gate fails: do NOT proceed to next step. Report and wait for resolution.
- If multiple agents disagree: flag disagreements for human review
- Keep a log of all agent invocations and results for audit trail
