---
name: run
description: "Execute: read current plan, run per-phase gate for completed phase, implement next phase(s), audit, update docs, commit. Usage: /run [N|all] — 1 phase (default), N phases, or all remaining."
---

# Run Workflow

You are executing the `/run` command — a shortcut for implementing one or more planned phases.

**FIRST OUTPUT:** Before any tool calls, print: `▶ /run {$ARGUMENTS or '1'}`

**Step 0 — Resolve session context:**
Call `resolve_session` MCP tool with: `project_root` = current working directory, `env_session` = CLAUDE_SESSION env var (empty if unset), `branch` = current git branch, `skill_args` = ARGUMENTS, `skill_name` = "run".

Use returned `plan_file`, `tasks_file`, `review_file`, `label`, `project_suffix`, `parsed_args` throughout. For `start_pipeline`: use `project=<basename_of_cwd>{project_suffix}`. For `list_active_pipelines`: ALWAYS pass `project=<basename_of_cwd>`.
Print: "Session: **{label}** → {plan_file}" only when label is set. Otherwise proceed silently.

Use `parsed_args.count` (integer or `"all"`) for the run scope instead of inline N/all parsing.

**Anti-hallucination rule:** NEVER derive session label from conversation topic, task description, or user request content. The `resolve_session` tool is the ONLY valid source. Any other derivation is a hallucination.

**Step 0.5 — Work Discovery (when PLAN_FILE has no incomplete phases):**

Before proceeding to scope/orchestrate, check whether PLAN_FILE actually has work:

1. Read PLAN_FILE. If it does not exist → mark `plan_empty=true`. If it exists: scan for any `- [ ]` checkbox lines (incomplete tasks/phases). If ALL phases are `[x]` (or file has no phases at all) → mark `plan_empty=true`.
2. If `plan_empty=false` → skip to Step 1 (normal flow, there is work to do).
3. If `plan_empty=true` → **Work Discovery scan:**
   a. Glob `docs/PLAN-*.md` — for each file found (excluding current PLAN_FILE), read the first 10 lines and any `- [ ]` lines. Collect files with incomplete phases.
   b. Read `docs/ROADMAP.md` — extract any lines containing `TODO`, `PENDING`, `IN_PROGRESS`, or "Next milestone".
   c. Read `MEMORY.md` — check for "Deferred Tasks" or "pending" entries.
   d. Read `session_resume.md` from project memory directory (the same directory where MEMORY.md lives — Claude Code resolves this automatically via the Read tool) (if exists) — this is the fast-path breadcrumb written by /save.
   e. Run `git log --oneline -5` — scan for "next session", "TODO", "PENDING", "v3 fix needed" hints.
4. **Present discovery results to user:**
   - If active work found → list it with numbers:
     ```
     PLAN_FILE is complete. Found active work:
       1. PLAN-Bug-to-QA.md: Phase 10.1 TODO
       2. ROADMAP: RSPDN re-embed pending
       3. Git log: "v3 fix needed next session" (abc1234)
     Which would you like to work on? (number, or 'none' to skip)
     ```
   - If no active work found → print "No active work found in this project. PLAN_FILE is complete." and **STOP** (do NOT invoke /finish — the plan was already finished in a prior session).
5. If user selects a PLAN-*.md → extract session label from filename, set SESSION_LABEL and PLAN_FILE accordingly, then continue to Step 1.
6. If user selects non-plan work (spike, roadmap item) → print recommendation (e.g., "Run `/phase <description>` to create a plan for this work") and **STOP**.

**Step 1 — Determine scope from ARGUMENTS (`$ARGUMENTS`):**
- Empty or `1` → run **1 phase** (default)
- Number `N` (e.g., `3`) → run **N consecutive phases**
- `all` → run **all remaining phases**

**Step 2 — Immediately invoke the `orchestrate` skill** using the Skill tool with:
- skill: `orchestrate`
- args: `custom "__RESOLVED__ PLAN_FILE={PLAN_FILE} TASKS_FILE={TASKS_FILE} REVIEW_FILE={REVIEW_FILE} PROJECT_SUFFIX={PROJECT_SUFFIX}. Execute phases from PLAN_FILE. SCOPE: $ARGUMENTS (empty=1, number=N phases, 'all'=all remaining). LOOP INSTRUCTIONS: repeat the following per-phase cycle until scope is exhausted or no incomplete phases remain — (1) read PLAN_FILE and TASKS_FILE — identify (a) the last implemented-but-not-yet-gated phase, if any, and (b) the next incomplete phase to implement; if no incomplete phase exists, stop the loop immediately; (2) PER-PHASE GATE — if a prior implemented phase exists: run automated tests (must pass zero failures), call mcp__pal__codereview on all files changed in that phase (any CRITICAL, HIGH, or MEDIUM finding → HALT ENTIRE LOOP), call mcp__pal__thinkdeep (any CRITICAL, HIGH, or MEDIUM finding → HALT ENTIRE LOOP); if PAL MCP unavailable, perform these reviews using Agent tool with a different model tier (opus if current is sonnet; sonnet if current is opus) and document fallback model used; if this is the first iteration after /phase (no prior implemented phase) — skip the gate; (3) if gate fails — HALT the entire loop immediately, report which phase caused the failure and the findings, do NOT proceed to next phase; (4) only after gate passes (or first-iteration skip) — mark the GATE checkpoint of the previous phase as [x] in PLAN_FILE; (5) route next phase via mcp__orchestrator__route_task and follow its decision; (6) implement all tasks in the next phase per the plan — when tasks involve external libraries or APIs, use mcp__context7__resolve-library-id + mcp__context7__query-docs to verify current documentation before writing code; (7) update PLAN_FILE (mark implemented tasks done), docs/ROADMAP.md, and MEMORY.md with phase progress; (8) commit with mcp__pal__precommit gate; (9) invoke the /summary skill with args=subtotal to output a per-phase checkpoint (read-only — no doc writes); (10) LOOP CONTROL: if scope was a number N, decrement counter — if counter > 0 AND incomplete phases remain, continue to next iteration WITHOUT invoking /save; if scope was 'all', continue to next iteration WITHOUT invoking /save; if scope is exhausted OR no incomplete phases remain, exit the loop; END OF LOOP — invoke the /summary skill with args=subtotal for the final run summary (read-only — user may run /summary project afterward for full project analysis + doc actualization); then branch: if ALL phases in PLAN_FILE are now complete — automatically invoke the /finish skill (via Skill tool) to perform final critical analysis, double audit, documentation update, and commit; do NOT invoke /save; if phases REMAIN — output 'Next step: run /run again to continue.' (or /run N / /run all for bulk); invoke the /save skill to verify all state is persisted and prompt the user to run the built-in /clear command before the next /run."`

Do not describe what you are about to do — invoke the skill immediately.
