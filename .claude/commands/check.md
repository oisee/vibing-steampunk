---
name: check
description: "Checkpoint: critical analysis, double audit, documentation update, save and commit"
---

# Check Workflow

You are executing the `/check` command — a shortcut for a quality checkpoint.

**FIRST OUTPUT:** Before any tool calls, print: `▶ /check`

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

**CONTEXT GATHERING (MANDATORY — run BEFORE invoking orchestrate):**

**CRITICAL RULE: ALWAYS RUN REAL VERIFICATION.** When the user invokes `/check`, they expect actual audit work — never shortcut with "already audited in this session", "no changes since last check", or "no further action needed." If the same code was checked 5 minutes ago, check it again. The user explicitly asked. If there are genuinely zero changes AND zero commits AND no user description, ask "What should I check?" rather than deciding on your own to skip.

**CRITICAL RULE: NEVER ask the user clarifying questions about scope.** Determine CHECK_SCOPE algorithmically from the rules below and proceed immediately. If the user wrote something after `/check`, that is the primary target — combine it with any uncommitted changes and act. Do not ask "what do you want to check?" — the user already told you.

Capture USER_CONTEXT and CHANGE_CONTEXT to give the audit something real to analyze.

1. **User description** — set USER_CONTEXT from ARGUMENTS (the text the user typed after `/check`, with session label already consumed by step 4b if applicable). If ARGUMENTS is empty or was fully consumed by session detection: USER_CONTEXT = "(no user description)".

2. **Git changes** — run these in parallel:
   - `git diff --stat` — unstaged changes summary
   - `git diff --cached --stat` — staged changes summary
   - `git log --oneline -10` — recent commits for context
   - If PLAN_FILE exists: read PLAN_FILE (first 60 lines) to understand current phase

3. **Build CHANGE_CONTEXT** — a compact summary string:
   - If there are staged or unstaged changes: list modified files from `--stat` output
   - If no uncommitted changes: use the last 3-5 commit messages as context (the user likely wants to check recently committed work)
   - If PLAN_FILE has an active (unchecked) phase: note the phase name

4. **Determine CHECK_SCOPE** — what exactly to audit:
   - If USER_CONTEXT is not "(no user description)": the user provided a specific description — audit THAT description plus ALL uncommitted code changes. The user's text is the PRIMARY analysis target. Do NOT split or disambiguate — combine everything into one scope.
   - If uncommitted changes exist but no user description: audit those changes (git diff)
   - If no uncommitted changes and no user description: audit the most recent commit(s) since last `/check` or `/finish`
   - Print a one-line summary: `Scope: {CHECK_SCOPE}` (e.g., "Scope: user description + 3 unstaged files", "Scope: last 2 commits (no uncommitted changes)", "Scope: staged changes in 5 files")

**Invoke the `orchestrate` skill** using the Skill tool with:
- skill: `orchestrate`
- args: `custom "__RESOLVED__ PLAN_FILE={PLAN_FILE} TASKS_FILE={TASKS_FILE} REVIEW_FILE={REVIEW_FILE} PROJECT_SUFFIX={PROJECT_SUFFIX}. CHECK SCOPE: {CHECK_SCOPE}. USER DESCRIPTION: {USER_CONTEXT}. CHANGE CONTEXT: {CHANGE_CONTEXT}. Critical analysis + double audit, recursive until zero MEDIUM+ findings: (1) critical analysis of the CHECK SCOPE — first verify spec compliance: read PLAN_FILE + TASKS_FILE and compare against the changes (does implementation match plan? missing features? scope creep?); then read every file mentioned in CHANGE_CONTEXT, analyze USER DESCRIPTION if provided, for code using external libraries or APIs use mcp__context7__resolve-library-id + mcp__context7__query-docs to verify correct API usage before auditing; (2) double audit — run lead-auditor then specialist-auditor, each with CV-GATE using mcp__pal__thinkdeep and mcp__pal__consensus (direct PAL MCP tool calls — if PAL MCP unavailable, perform internal cross-model review using Agent tool with a different model tier (opus if current is sonnet; sonnet if current is opus) and document fallback model used); (3) fix ALL MEDIUM+ findings found by either auditor; (4) repeat steps 2-3 until zero CRITICAL, HIGH, and MEDIUM findings remain; (5) update all documentation (ROADMAP.md, ANALYSIS.md, AGENTS.md, MEMORY.md, STATS.md if applicable); (6) save all artifacts; (7) commit with mcp__pal__precommit gate."`

Do not describe what you are about to do — invoke the skill immediately.
