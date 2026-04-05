---
name: check
description: "Checkpoint: critical analysis, double audit, documentation update, save and commit"
---

# Check Workflow

You are executing the `/check` command — a shortcut for a quality checkpoint.

**FIRST OUTPUT:** Before any tool calls, print: `▶ /check`

**Step 0 — Detect session context (silently — NO visible bash calls):**
**Priority 1 — Reuse:** if session context is already known from this conversation (Session Start Protocol ran earlier, or a previous skill resolved it): reuse existing SESSION_LABEL, PLAN_FILE, TASKS_FILE, REVIEW_FILE. Skip to step 7.
**Priority 2 — Hook tag:** check conversation context for `[SESSION]` tag (injected by sync-check.py SessionStart hook). Parse it:
  - `[SESSION] label=X source=env|branch ...` → SESSION_LABEL=`X`. Skip to step 5.
  - `[SESSION] default escape=true` → force no-session mode. Skip to step 6.
  - `[SESSION] default branch=...` → no session. Check args (step 4b), then step 6.
**Priority 3 — Args:** if SESSION_LABEL still not set AND ARGUMENTS (`$ARGUMENTS`) non-empty: extract FIRST_ARG = first whitespace-delimited word. If FIRST_ARG matches `^[A-Za-z][A-Za-z0-9_-]{1,62}$`: glob `docs/PLAN-*.md` — if match found → SESSION_LABEL=FIRST_ARG (from args). Skip to step 5.
**Priority 4 — Bash fallback (ONLY if no `[SESSION]` tag in context AND no reuse):** run `bash -c 'printf "%s\n%s" "${CLAUDE_SESSION:-(no session)}" "$(git branch --show-current 2>/dev/null || true)"'`. Parse and resolve as before.
5. SESSION_LABEL set: PLAN_FILE=`docs/PLAN-{SESSION_LABEL}.md`, TASKS_FILE=`docs/TASKS-{SESSION_LABEL}.md`, REVIEW_FILE=`docs/REVIEW-{SESSION_LABEL}.md`, PROJECT_SUFFIX=`__{SESSION_LABEL}`. Verify header if PLAN_FILE exists: read first 3 lines — must contain `Session: {SESSION_LABEL}`; if mismatch: ABORT.
6. SESSION_LABEL not set: PLAN_FILE=`docs/PLAN.md`, TASKS_FILE=`docs/TASKS.md`, REVIEW_FILE=`docs/REVIEW.md`, PROJECT_SUFFIX=(none).
7. Use PLAN_FILE/TASKS_FILE/REVIEW_FILE throughout. For `start_pipeline`: use `project=<basename_of_cwd><PROJECT_SUFFIX>`. For `list_active_pipelines`: if SESSION_LABEL set, pass `project=<basename_of_cwd><PROJECT_SUFFIX>`.
**Output:** Print session result in ONE line ONLY when SESSION_LABEL is set: "Session: {SESSION_LABEL} → {PLAN_FILE}". When no session: print NOTHING — proceed silently.

**Anti-hallucination rule:** NEVER derive SESSION_LABEL from conversation topic, task description, or user request content. SESSION_LABEL comes ONLY from: (a) CLAUDE_SESSION env var, (b) git branch name, (c) args matching an existing `PLAN-*.md` file. Any other source is a hallucination.

**CONTEXT GATHERING (MANDATORY — run BEFORE invoking orchestrate):**

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
