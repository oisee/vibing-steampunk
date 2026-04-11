---
name: routing-rules
description: Full MCP orchestrator integration, CV gates, and pipeline execution rules. Load when starting non-trivial implementations or working inside pipelines.
---

# Full Task Routing, Orchestrator Integration & Pipeline Rules

This skill contains the detailed routing rules referenced by the `Automatic Task Routing` section in CLAUDE.md.

---

## MCP Orchestrator Integration

Before starting any non-trivial implementation: call `orchestrator.route_task(task_description)` and follow the returned routing decision.

- **`route_task(description)`** -- returns pipeline type, recommended agents, CV requirements, complexity estimate.
- **`registry(action="get_agent", name=...)`** -- returns agent metadata: tier, model, tools, CV requirement, MCP servers.
- **`registry(action="list_agents", tier=...)`** -- lists available agents, optionally filtered by model tier.

**After receiving a route_task response:**
- Follow the returned `pipeline` and `agents` list.
- When `cv_required` is true: ensure cross-validation agents participate.
- When `llm_refinement_suggested` is true: call `mcp__orchestrator__registry(action="refine_routing", routing_json=..., task_description=...)` to get LLM-classified pipeline before proceeding.
- When the orchestrator MCP server is unavailable: fall back to the manual routing rules in CLAUDE.md.

---

## Cross-Validation via Orchestrator (Level 2)

After completing each pipeline stage involving a CV-enabled agent: call `orchestrator.validate(action="cv_gate", stage_output=..., gate_type=...)` before proceeding.

| cv_gate result | Action |
|----------------|--------|
| `PASS` | Continue to next step |
| `HALT` | Fix the CRITICAL issue, re-submit the step output |
| `DISPUTE` | Call `orchestrator.validate(action="cross_validate", topic=..., claude_analysis=...)` for multi-round debate |
| `SKIP` | Continue (CV temporarily unavailable, log warning) |
| `FAIL` | Report configuration error to user |

**Gate types:** `consensus` (architecture), `codereview` (code), `thinkdeep` (deep analysis), `precommit` (pre-commit).

PAL MCP tools provide **Level 1** agent-initiated CV. The orchestrator's `cv_gate` provides **Level 2** pipeline-enforced CV. Both coexist (defense-in-depth).

---

## Pipeline Execution via Orchestrator (Level 3)

When a task requires multi-step execution: use orchestrator pipeline tools instead of manually chaining agents.

- **`start_pipeline(pipeline_type, description, project?)`** -- initialize pipeline, get pipeline_id and first step. Pass `project=<basename of cwd>` so the dashboard can group pipelines by project.
- **`complete_step(pipeline_id, step_output)`** -- report step completion. Server runs CV-gate if required. Returns next step or HALT/PIPELINE_COMPLETE.
- **`pipeline_ops(action="status", pipeline_id=...)`** -- get current state. Use to resume after context window reset.

**Pipeline types:** `feature`, `bugfix`, `deploy`, `audit`, `qa`, `review`, `refactor`, `incident`, `migration`, `spike`, `perf`, `onboard`, `docs`, `techdebt`, `deep-validate`.

**Execution sequence:**
1. Call `route_task(description)` -- get recommended `pipeline` type.
2. Call `start_pipeline(pipeline_type, description, project=<basename of cwd>)` -- get pipeline_id and first step.
3. Execute the step using the assigned agent.
4. Call `complete_step(pipeline_id, step_output)` -- server runs CV-gate if needed.
5. On `HALT`: fix the issue, re-submit the same step.
6. On `next_step`: execute next agent, repeat from step 4.
7. On `PIPELINE_COMPLETE`: pipeline finished.

**Pipeline rules:**
- Never skip `complete_step` -- the server tracks state and enforces CV gates.
- Pipeline state persists to disk and survives context resets.
- After a context window reset: call `pipeline_ops(action="status")` to resume with full context.
- Optional steps (e.g., frontend-dev, visual-qa) are auto-skipped when not applicable.

---

## Task Agent Launch Protocol (MANDATORY)

> **Critical:** Hooks run ONLY in the main session. Task sub-agents bypass all hooks. Enforce quality and pipeline discipline through prompt injection, not hooks.

### Authority Tool Rule

**Only the main session may call:**
- `mcp__orchestrator__complete_step(pipeline_id, step_output)` -- after validating STEP RESULT
- `mcp__orchestrator__start_pipeline(pipeline_type, description, project=<basename of cwd>)` -- after route_task + plan saved

**Sub-agents MUST NOT call these tools directly.** Sub-agents produce STEP RESULT blocks; the main session validates and calls the authority tools.

### Pipeline Context Injection

Every Task agent launched for a pipeline step MUST receive this in its prompt:

```
PIPELINE CONTEXT (do not ignore):
  pipeline_id: {id from start_pipeline result}
  step: {n} of {total} -- {step_name}
  pipeline_type: {type}

REQUIRED: Produce a ## STEP RESULT block as the very last thing in your response.
DO NOT call mcp__orchestrator__complete_step -- the main session does this after reviewing your output.
```

### STEP RESULT Format

Every pipeline agent MUST end its response with this block:

```
## STEP RESULT
- step: {step-name}
- pipeline_id: {pipeline-id}
- status: COMPLETE | INCOMPLETE | FAILED | SKIPPED | NEEDS_ASSISTANCE
- artifacts: [list of files created or modified]
- notes: <1-3 lines summary>
- next: {next-agent-name or PIPELINE_COMPLETE}
```

### Main Session Validation Rules

After receiving a Task agent response:

1. **Check for STEP RESULT block** -- if absent, ask the agent to resubmit with the required format
2. **Check status** -- do NOT advance if status is INCOMPLETE, FAILED, or NEEDS_ASSISTANCE
3. **Verify artifacts** -- confirm listed artifact files exist before calling complete_step
4. **Call complete_step yourself** -- `mcp__orchestrator__complete_step(pipeline_id, step_output)` after validation
5. **If complete_step returns HALT** -- fix the issue (run PAL/audit if needed), then resubmit the step

---

## Sub-Agent Enforcement Invariants (inject into EVERY Task agent prompt)

```
ENFORCEMENT INVARIANTS (non-negotiable):
- Never call mcp__orchestrator__complete_step or start_pipeline directly
- Produce ## STEP RESULT block as the last thing in your response
- No secrets, credentials, or API keys in output
- DB paths (*.db, *.sqlite, chroma_db/) are READ-ONLY -- no deletion
- No git push --force or git reset --hard without explicit user instruction
```
