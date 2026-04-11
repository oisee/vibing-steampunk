---
name: finish
description: "Finish: critical analysis, double audit, documentation update, save and commit"
---

# Finish Workflow

You are executing the `/finish` command — a shortcut for finalizing a work session or feature.

**FIRST OUTPUT:** Before any tool calls, print: `▶ /finish`

**Step 0 — Resolve session context:**
Call the `resolve_session` MCP tool with: `project_root` = current working directory, `env_session` = value of CLAUDE_SESSION env var (or `""` if unset), `branch` = current git branch name, `skill_args` = ARGUMENTS string (if any), `skill_name` = name of the invoking skill (e.g. `"run"`, `"check"`).

The tool returns `SessionResult` JSON with: `plan_file`, `tasks_file`, `review_file`, `label`, `source`, `project_suffix`, `warnings`, `parsed_args`.

**Use the returned values throughout:**
- `plan_file` / `tasks_file` / `review_file` — session-scoped file paths
- `label` — session label (null = no session)
- `project_suffix` — append to project name for `start_pipeline` / `list_active_pipelines`
- `parsed_args` — skill-specific extracted arguments (e.g. `count`, `mode`, `description`, `workflow`)

**Output:** Print session result ONLY when `label` is set: "Session: **{label}** → {plan_file}". When no label: proceed silently.

**Anti-hallucination rule:** NEVER derive session label from conversation topic, task description, or user request content. The `resolve_session` tool is the ONLY valid source. Any other derivation is a hallucination.

**PRE-FLIGHT — Orphan pipeline scan (run BEFORE invoking orchestrate):**
1. Call `list_active_pipelines(project=<basename_of_cwd>)` — server-side prefix matching returns all sessions for this project, excludes foreign pipelines.
2. For each pipeline returned:
   - If stale (`stale: true`, >24h) OR all related work is committed in git → call `pipeline_ops(action="cancel", pipeline_id=id, reason="Closed by /finish — work committed")`
   - If real pending work remains (not yet committed) → warn the user before proceeding
3. Only after orphans are resolved — continue to orchestrate below.

**Immediately invoke the `orchestrate` skill** using the Skill tool with:
- skill: `orchestrate`
- args: `custom "__RESOLVED__ PLAN_FILE={PLAN_FILE} TASKS_FILE={TASKS_FILE} REVIEW_FILE={REVIEW_FILE} PROJECT_SUFFIX={PROJECT_SUFFIX}. Critical analysis + double audit, recursive until zero MEDIUM+ findings: (1) critical analysis of all current changes — for code using external libraries or APIs, use mcp__context7__resolve-library-id + mcp__context7__query-docs to verify correct API usage before auditing; (2) double audit — run lead-auditor then specialist-auditor, each with CV-GATE using mcp__pal__thinkdeep and mcp__pal__consensus (direct PAL MCP tool calls — if PAL MCP unavailable, perform internal cross-model review using Agent tool with a different model tier (opus if current is sonnet; sonnet if current is opus) and document fallback model used); (3) fix ALL MEDIUM+ findings found by either auditor; (4) repeat steps 2-3 until zero CRITICAL, HIGH, and MEDIUM findings remain; (5) update all documentation (ROADMAP.md, ANALYSIS.md, AGENTS.md, MEMORY.md, STATS.md if applicable); (6) save all artifacts; (7) commit with mcp__pal__precommit gate."`

Do not describe what you are about to do — invoke the skill immediately.

**Post-completion cleanup:**
After the orchestrate skill completes successfully (commit done, all gates passed): delete the session file only if it matches the current session label: run `bash -c '[ -f .claude/.session ] && [ "$(cat .claude/.session 2>/dev/null)" = "{SESSION_LABEL}" ] && rm -f .claude/.session 2>/dev/null || true'`. This prevents deleting another session's `.session` file in concurrent multi-window setups.
