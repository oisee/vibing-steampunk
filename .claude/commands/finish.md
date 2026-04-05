---
name: finish
description: "Finish: critical analysis, double audit, documentation update, save and commit"
---

# Finish Workflow

You are executing the `/finish` command — a shortcut for finalizing a work session or feature.

**FIRST OUTPUT:** Before any tool calls, print: `▶ /finish`

**Step 0 — Detect session context (silently — NO visible bash calls):**
**Priority 1 — Reuse:** if session context already known from this conversation: reuse. Skip to step 7.
**Priority 2 — Hook tag:** check conversation context for `[SESSION]` tag (from sync-check.py hook):
  - `[SESSION] label=X ...` → SESSION_LABEL=`X`. Skip to step 5.
  - `[SESSION] default escape=true` → no session. Skip to step 6.
  - `[SESSION] default branch=...` → no session. Check args (step 4b), then step 6.
**Priority 3 — Args:** if SESSION_LABEL still not set AND ARGUMENTS non-empty: extract FIRST_ARG. If matches `^[A-Za-z][A-Za-z0-9_-]{1,62}$`: glob `docs/PLAN-*.md` — if match → SESSION_LABEL=FIRST_ARG (from args). Skip to step 5.
**Priority 4 — Bash fallback (ONLY if no `[SESSION]` tag and no reuse):** run `bash -c 'printf "%s\n%s" "${CLAUDE_SESSION:-(no session)}" "$(git branch --show-current 2>/dev/null || true)"'`. Parse as before.
5. SESSION_LABEL set: PLAN_FILE=`docs/PLAN-{SESSION_LABEL}.md`, TASKS_FILE=`docs/TASKS-{SESSION_LABEL}.md`, REVIEW_FILE=`docs/REVIEW-{SESSION_LABEL}.md`, PROJECT_SUFFIX=`__{SESSION_LABEL}`. Verify header if PLAN_FILE exists: read first 3 lines — must contain `Session: {SESSION_LABEL}`; if mismatch: ABORT.
6. SESSION_LABEL not set: PLAN_FILE=`docs/PLAN.md`, TASKS_FILE=`docs/TASKS.md`, REVIEW_FILE=`docs/REVIEW.md`, PROJECT_SUFFIX=(none).
7. Use PLAN_FILE/TASKS_FILE/REVIEW_FILE throughout. For `start_pipeline`: use `project=<basename_of_cwd><PROJECT_SUFFIX>`. For `list_active_pipelines`: if SESSION_LABEL set, pass `project=<basename_of_cwd><PROJECT_SUFFIX>`.
**Output:** Print session result ONLY when SESSION_LABEL is set: "Session: {SESSION_LABEL} → {PLAN_FILE}". When no session: print NOTHING — proceed silently.

**Anti-hallucination rule:** NEVER derive SESSION_LABEL from conversation topic, task description, or user request content. SESSION_LABEL comes ONLY from: (a) CLAUDE_SESSION env var, (b) git branch name, (c) args matching an existing `PLAN-*.md` file. Any other source is a hallucination.

**PRE-FLIGHT — Orphan pipeline scan (run BEFORE invoking orchestrate):**
1. If SESSION_LABEL set: call `list_active_pipelines(project=<basename_of_cwd>__<SESSION_LABEL>)` — session-specific pipelines only
   If no SESSION_LABEL: call `list_active_pipelines()` — all pipelines (default behavior)
2. For each pipeline returned:
   - If stale (`stale: true`, >24h) OR all related work is committed in git → call `cancel_pipeline(id, "Closed by /finish — work committed")`
   - If real pending work remains (not yet committed) → warn the user before proceeding
3. If SESSION_LABEL set: also call `list_active_pipelines()` (unfiltered) and compare to step 1 results:
   - Pipelines not in step 1 results are **legacy or other-session pipelines** (project=<basename> without label, project=None, or different label suffix)
   - If any found: report to user: "[N legacy/other-session pipeline(s) found: <IDs + projects>]. These are pre-session-isolation or from another session. Review manually — NOT auto-cancelled."
   - This is informational only — do NOT auto-cancel these pipelines
4. Only after own-session orphans are resolved — continue to orchestrate below.

**Immediately invoke the `orchestrate` skill** using the Skill tool with:
- skill: `orchestrate`
- args: `custom "__RESOLVED__ PLAN_FILE={PLAN_FILE} TASKS_FILE={TASKS_FILE} REVIEW_FILE={REVIEW_FILE} PROJECT_SUFFIX={PROJECT_SUFFIX}. Critical analysis + double audit, recursive until zero MEDIUM+ findings: (1) critical analysis of all current changes — for code using external libraries or APIs, use mcp__context7__resolve-library-id + mcp__context7__query-docs to verify correct API usage before auditing; (2) double audit — run lead-auditor then specialist-auditor, each with CV-GATE using mcp__pal__thinkdeep and mcp__pal__consensus (direct PAL MCP tool calls — if PAL MCP unavailable, perform internal cross-model review using Agent tool with a different model tier (opus if current is sonnet; sonnet if current is opus) and document fallback model used); (3) fix ALL MEDIUM+ findings found by either auditor; (4) repeat steps 2-3 until zero CRITICAL, HIGH, and MEDIUM findings remain; (5) update all documentation (ROADMAP.md, ANALYSIS.md, AGENTS.md, MEMORY.md, STATS.md if applicable); (6) save all artifacts; (7) commit with mcp__pal__precommit gate."`

Do not describe what you are about to do — invoke the skill immediately.
