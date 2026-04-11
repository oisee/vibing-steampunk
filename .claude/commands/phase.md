---
name: phase
description: "Phase planning: critical analysis, double audit, phase breakdown with task decomposition, plan persistence, documentation update, commit"
---

# Phase Planning Workflow

You are executing the `/phase` command — a shortcut for structured phase planning.

**FIRST OUTPUT:** Before any tool calls, print: `▶ /phase {$ARGUMENTS}`

**Step 0 — Resolve session context:**
Call `resolve_session` MCP tool with: `project_root` = current working directory, `env_session` = CLAUDE_SESSION env var (empty if unset), `branch` = current git branch, `skill_args` = ARGUMENTS, `skill_name` = "phase".

Use returned `plan_file`, `tasks_file`, `review_file`, `label`, `project_suffix`, `parsed_args` throughout. For `start_pipeline`: use `project=<basename_of_cwd>{project_suffix}`. For `list_active_pipelines`: ALWAYS pass `project=<basename_of_cwd>`.
Use `parsed_args.auto_label` for the auto-extracted session label (when `resolve_session` derives a label from the description). Use `parsed_args.description` as the planning description.
Print: "Session: **{label}** → {plan_file}" only when label is set (print "Auto-session: **{label}** → {plan_file}" when label was auto-extracted from description). Otherwise proceed silently.

**Anti-hallucination rule (phase exception):** `/phase` is the ONLY skill that may derive a session label from description text (`parsed_args.auto_label`) because it CREATES new plan files. All other skills (`/run`, `/save`, `/check`, `/finish`, `/summary`) MUST NOT — they only reference EXISTING plans. Even in `/phase`, the derived label must pass sanitization (`^[A-Za-z][A-Za-z0-9_-]*$`, min 3 chars) and must not be a conversation topic or generic word. The `resolve_session` tool enforces this — any other derivation is a hallucination.

**Immediately invoke the `orchestrate` skill** using the Skill tool with:
- skill: `orchestrate`
- args: `custom "__RESOLVED__ PLAN_FILE={PLAN_FILE} TASKS_FILE={TASKS_FILE} REVIEW_FILE={REVIEW_FILE} PROJECT_SUFFIX={PROJECT_SUFFIX}. Critical analysis + double audit, recursive until zero MEDIUM+ findings: (1) critical analysis of current state — use mcp__context7__resolve-library-id + mcp__context7__query-docs to verify documentation for any external libraries involved in the planned work; (2) double audit — lead-auditor with CV-GATE mcp__pal__consensus, then specialist-auditor with CV-GATE mcp__pal__thinkdeep (direct PAL MCP tool calls; if PAL MCP unavailable: Agent tool with different model tier (opus if current is sonnet; sonnet if current is opus) — document fallback used); (3) fix ALL MEDIUM+ findings found by either auditor; (4) repeat steps 2-3 until zero CRITICAL, HIGH, and MEDIUM findings remain; (5) phase decomposition: break work into phases with concrete tasks per P41-P44 planning rules — each phase that modifies code/data/infrastructure must include a Rollback subsection: ordered steps to undo the phase; for irreversible phases write 'Rollback: N/A — [mitigation plan]'; (6) persist plan to PLAN_FILE and TASKS_FILE — each phase MUST end with a mandatory GATE step: '- [ ] GATE: run tests (verify new code paths have dedicated tests) + mcp__pal__codereview + mcp__pal__thinkdeep (if PAL unavailable: Agent tool with different model tier) — zero MEDIUM+ before next phase'; (6b) add '## Next Plans' section at the end of PLAN_FILE — read docs/ROADMAP.md to identify the next 1–4 phases after the current plan, list each with Phase ID, title, status emoji (✅/🚧/⏸/📋), and one-line goal; if next phases are unknown, write 'TBD — run /phase after this plan completes'; (7) save all artifacts; (8) update all documentation (ROADMAP.md, ANALYSIS.md, AGENTS.md, MEMORY.md); (9) commit with mcp__pal__precommit gate."`

Do not describe what you are about to do — invoke the skill immediately.

## Final Output (MANDATORY after commit)

After the commit succeeds, output a **Plan Summary** directly to the user:

```
## Phase N Plan — <Title>

**Audit:** APPROVE [C+O] — <findings summary, e.g. "2 CRITICAL + 3 HIGH fixed">
**Commit:** <hash>

### Phases & Steps

| Phase | Goal | Key Tasks | Can start |
|-------|------|-----------|-----------|
| Phase 1 — <Name> | <one-line goal> | T1.1, T1.2, ... | Immediately / After X |
| Phase 2 — <Name> | <one-line goal> | T2.1, T2.2, ... | After Phase 1 |
| ...   |      |           |           |

### Findings Fixed

| ID | Severity | Description | Resolution |
|----|----------|-------------|------------|
| C-1 | CRITICAL | ... | ... |
| H-1 | HIGH | ... | ... |
| M-1 | MEDIUM | ... | ... |
```

This output must be in the main response, not inside a tool call or file.

After the Plan Summary, **invoke the `/summary` skill** (no args — session mode: quick summary of commits, plan status, next work, test count). For full project deep analysis + doc actualization, the user can run `/summary project` separately.
