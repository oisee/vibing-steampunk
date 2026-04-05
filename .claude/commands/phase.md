---
name: phase
description: "Phase planning: critical analysis, double audit, phase breakdown with task decomposition, plan persistence, documentation update, commit"
---

# Phase Planning Workflow

You are executing the `/phase` command — a shortcut for structured phase planning.

**FIRST OUTPUT:** Before any tool calls, print: `▶ /phase {$ARGUMENTS}`

**Step 0 — Detect session context (silently — NO visible bash calls):**
**Priority 1 — Reuse:** if session context already known from this conversation: reuse. Skip to step 7.
**Priority 2 — Hook tag:** check conversation context for `[SESSION]` tag (from sync-check.py hook):
  - `[SESSION] label=X ...` → SESSION_LABEL=`X`. Skip to step 5.
  - `[SESSION] default escape=true` → no session. Check args (step 4b), then step 6.
  - `[SESSION] default branch=...` → no session. Check args (step 4b), then step 6.
**Priority 3 — Bash fallback (ONLY if no `[SESSION]` tag and no reuse):** run `bash -c 'printf "%s\n%s" "${CLAUDE_SESSION:-(no session)}" "$(git branch --show-current 2>/dev/null || true)"'`. Parse as before.
4b. If SESSION_LABEL still not set AND ARGUMENTS non-empty: auto-extract a session label from the description. Algorithm: split ARGUMENTS on whitespace; skip stop words (a, an, the, for, with, in, of, to, from, by, and, or, but) and generic action words (fix, add, update, implement, create, build, run, use, write, change, refactor, configure, setup, set, get, make, new); take the first remaining word; sanitize (replace `[^A-Za-z0-9_-]` with `-`, strip leading/trailing `-`). If result is ≥ 3 chars and matches `^[A-Za-z][A-Za-z0-9_-]*$`: SESSION_LABEL=result. Print: "Auto-session: {SESSION_LABEL} → docs/PLAN-{SESSION_LABEL}.md". Skip to step 5. (PLAN file may not exist yet — will be created during planning.)
5. SESSION_LABEL set: PLAN_FILE=`docs/PLAN-{SESSION_LABEL}.md`, TASKS_FILE=`docs/TASKS-{SESSION_LABEL}.md`, REVIEW_FILE=`docs/REVIEW-{SESSION_LABEL}.md`, PROJECT_SUFFIX=`__{SESSION_LABEL}`. Verify header if PLAN_FILE exists: read first 3 lines — must contain `Session: {SESSION_LABEL}`; if mismatch: ABORT.
6. SESSION_LABEL not set: PLAN_FILE=`docs/PLAN.md`, TASKS_FILE=`docs/TASKS.md`, REVIEW_FILE=`docs/REVIEW.md`, PROJECT_SUFFIX=(none).
7. Use PLAN_FILE/TASKS_FILE/REVIEW_FILE throughout. For `start_pipeline`: use `project=<basename_of_cwd><PROJECT_SUFFIX>`. For `list_active_pipelines`: if SESSION_LABEL set, pass `project=<basename_of_cwd><PROJECT_SUFFIX>`.
**Output:** Print session result ONLY when SESSION_LABEL is set: "Session: {SESSION_LABEL} → {PLAN_FILE}". When no session: print NOTHING — proceed silently.

**Anti-hallucination rule (phase.md exception):** `/phase` is the ONLY skill that may derive SESSION_LABEL from description text (Step 4b above) because it CREATES new plan files. All other skills (`/run`, `/save`, `/check`, `/finish`, `/summary`) MUST NOT — they only reference EXISTING plans. Even in `/phase`, the derived label must pass sanitization (`^[A-Za-z][A-Za-z0-9_-]*$`, min 3 chars) and must not be a conversation topic or generic word.

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
