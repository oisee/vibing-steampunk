---
name: save
description: "Context cleanup: verify all state is persisted to files, then prompt user to run the built-in /clear command"
---

# Save Workflow

You are executing the `/save` skill — a context cleanup checkpoint between phases.

**Note on terminology:**
- `/save` — this skill (defined here, invokable via slash command)
- `/clear` — a **built-in Claude Code command** (Clear conversation) that actually clears the context. It cannot be called programmatically; the user must run it manually.

**FIRST OUTPUT:** Before any tool calls, print: `▶ /save`

**Step 0 — Resolve session context:**
Call `resolve_session` MCP tool with: `project_root` = current working directory, `env_session` = CLAUDE_SESSION env var (empty if unset), `branch` = current git branch, `skill_args` = ARGUMENTS, `skill_name` = "save".

Use returned `plan_file`, `tasks_file`, `review_file`, `label`, `project_suffix` throughout. Set SESSION_LABEL=`label`, PLAN_FILE=`plan_file`, TASKS_FILE=`tasks_file`.
Print: "Session: **{label}** → {plan_file}" only when label is set. Otherwise proceed silently.

**Step 0b — Validate session label against plan file (MANDATORY anti-hallucination gate):**
After Step 0 resolves SESSION_LABEL and PLAN_FILE:
1. **If SESSION_LABEL is set:** save ORIGINAL_LABEL=SESSION_LABEL. Validate format `^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$` — if invalid: print "Warning: invalid session label '{ORIGINAL_LABEL}'", force SESSION_LABEL=(none), PLAN_FILE=`docs/PLAN.md`, TASKS_FILE=`docs/TASKS.md`. If valid: glob `docs/PLAN-{SESSION_LABEL}.md`. If file NOT found: force SESSION_LABEL=(none), PLAN_FILE=`docs/PLAN.md`, TASKS_FILE=`docs/TASKS.md`. Print: "Warning: PLAN-{ORIGINAL_LABEL}.md not found — falling back to docs/PLAN.md".
2. **If SESSION_LABEL is not set:** confirm `docs/PLAN.md` exists. If not: print "Warning: no plan file found."
3. **NEVER derive SESSION_LABEL from the conversation topic, task description, or user request content.** SESSION_LABEL comes ONLY from: (a) CLAUDE_SESSION env var, (b) git branch name, (c) `.claude/.session` file (read by hook), (d) single PLAN-*.md auto-detect (hook). Any other source is a hallucination.

**Step 0c — Persist session label (ONLY after step 0b validation passes):** If SESSION_LABEL is still set (not overridden by 0b) AND was set from Priority 3 (Bash fallback) — NOT from Priority 1 (Reuse) or Priority 2 (Hook tag): run `bash -c 'tmp=".claude/.session.tmp.$$"; printf "%s" "{SESSION_LABEL}" > "$tmp" && mv -f "$tmp" .claude/.session 2>/dev/null || rm -f "$tmp" 2>/dev/null'` (atomic write to persist session across `/clear`).

## Steps

1. Verify that all state is persisted to files:
   - PLAN_FILE (from Step 0) — previous phase GATE checkpoint marked `[x]`, current phase tasks marked as implemented
   - TASKS_FILE (from Step 0) — task breakdown current
   - `docs/ROADMAP.md` — phase progress updated
   - `MEMORY.md` — project state current
   - All changes committed (`git status` must be clean)

2. If anything is not persisted: save it now before clearing context.

3. **Session Resume Breadcrumb (MANDATORY — do this BEFORE step 4, never skip):**
   - Path: project memory directory (the same directory where MEMORY.md lives — Claude Code resolves this automatically via the Write tool), file `session_resume.md`
   - **MUST be written.** If the Write tool fails: print the full breadcrumb content to the user as a code block with the message "Could not write session_resume.md — save this manually."
   - Content (update if exists, create if not):
     ```markdown
     ---
     name: session_resume
     description: What to do next session — written by /save, read by /run work discovery
     type: project
     ---

     # Session Resume — {date}

     ## Resume Command
     {The exact /run command to continue — e.g., `/run` or `/run LABEL`. MUST match the PLAN_FILE actually used in this session. If PLAN_FILE=docs/PLAN.md → `/run`. If PLAN_FILE=docs/PLAN-X.md → `/run X`.}

     ## Last Action
     {1-line summary of what was just done}

     ## Next Action
     {What should be done next — extracted from PLAN_FILE TODO items, ROADMAP next milestone, or git log hints}

     ## Unresolved Findings
     {List any BLOCKER/CRITICAL/HIGH/MEDIUM findings from audits/spikes that are not yet fixed, or "None"}

     ## Active Plans
     {List all PLAN-*.md and PLAN.md files with their status: COMPLETE / has TODO phases / does not exist}
     ```
   - To populate: read PLAN_FILE status, glob `docs/PLAN-*.md` for other plans, check `docs/ROADMAP.md` for next milestone, scan last 3 git commits for "next session"/"TODO" hints.
   - Keep it under 30 lines — this is a quick-reference breadcrumb, not a full summary.

4. **Build the resume command (cross-validation):**
   - If SESSION_LABEL is set AND `docs/PLAN-{SESSION_LABEL}.md` exists on disk: RESUME_CMD = `/run {SESSION_LABEL}`
   - If SESSION_LABEL is NOT set OR `docs/PLAN-{SESSION_LABEL}.md` does NOT exist: RESUME_CMD = `/run`
   - **NEVER put a topic name, feature name, or conversation keyword into RESUME_CMD.** Only validated SESSION_LABELs that passed Step 0b.

5. Output this message to the user (same meaning; match the user's language; keep command tokens verbatim):

---

**Context ready for cleanup.**

All state is persisted to PLAN_FILE, TASKS_FILE, `docs/ROADMAP.md`, `MEMORY.md`, and `session_resume.md`.

> Run **`/clear`**, then **`{RESUME_CMD}`** to continue from where this session left off.

---
