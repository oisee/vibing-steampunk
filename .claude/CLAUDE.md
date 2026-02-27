<!-- DO NOT EDIT -- managed by sync.ps1 from claude-team-control -->
<!-- Synced: 2026-02-27 15:50:46 -->
<!-- Base: base/CLAUDE.md | Overlay: overlays/vibing-steampunk.md -->


## Requirements

- When uncertain about any fact, API, or behavior: state "I don't know" explicitly. Never guess, hallucinate, or fabricate information.

## Language & Terminology

- When writing any code artifact (code, comments, docstrings, variable names, README, commit messages, diagrams): write in English.
- When encountering an English technical term with no established Russian equivalent: use the original Latin-script term (git stash, merge, rebase, commit, pull request). Never transliterate into Cyrillic.
- When responding to the user: match the language the user writes in.

## Research & Verification

### Tool-First Analysis (MANDATORY)

Before forming any conclusion about code, architecture, or technical decisions: make at least one tool call (Read, Grep, Glob, context7, WebSearch, WebFetch, MCP, or Task agent). Never reason from memory alone.

- Before implementing a solution or suggesting an approach: query official documentation via context7, WebSearch, or WebFetch to verify assumptions.
- Before choosing an API, library, or pattern: look up its actual behavior. Never assume.
- When in plan mode: actively explore the codebase (read files, search patterns, check dependencies). Plans without tool-grounded analysis are invalid.
- When analysis requires multi-file exploration or heavy research: delegate to a Task agent (Explore, Plan, general-purpose) to offload token cost from the main context.

### PAL MCP Tools (MANDATORY)

Before concluding on architecture, bugs, or security: call the appropriate PAL tool. Never keep complex reasoning purely internal.

| Trigger | Call |
|---------|------|
| Before concluding on a non-trivial problem (architecture, complex bug, performance, security) | `mcp__pal__thinkdeep` |
| Before presenting an implementation plan to the user | `mcp__pal__planner` |
| Before making a decision with significant long-term impact (technology choice, architecture trade-off) | `mcp__pal__consensus` |
| After writing or modifying non-trivial code | `mcp__pal__codereview` |
| Before committing changes (enforced by hook) | `mcp__pal__precommit` |
| When questioning a previous conclusion or disagreeing with a finding | `mcp__pal__challenge` |
| When brainstorming or seeking a second opinion | `mcp__pal__chat` |
| When debugging a complex bug or investigating a multi-component issue | `mcp__pal__debug` |

## Project Structure

### General Rules (All Projects)

- When creating a new file: place it in the correct standard directory (see table below). Never place scratch files, experiments, temp outputs, or logs in the project root.
- When creating a new top-level directory: document its purpose in README.md or docs/ before committing.

| Directory | Purpose | Rules |
|-----------|---------|-------|
| `docs/` | Documentation | ROADMAP.md, ANALYSIS.md, ARCHITECTURE.md, AGENTS.md |
| `tests/` | Test files | Mirror source structure, prefix with `test_` |
| `logs/` | Log outputs | Gitignored, created at runtime, never committed |
| `_archive/` | Backups and temp files | Database backups, old versions, scratch -- gitignored |
| `.claude/` | Claude Code config | **Managed by sync.ps1 -- NEVER edit directly** |

**Naming conventions:**
- Directories and non-Python files: `kebab-case` (e.g., `sync-check.py`, `plan-feature.md`)
- Python modules: `snake_case` (e.g., `file_util.py`, `test_router.py`)
- Uppercase exceptions: `CLAUDE.md`, `README.md`, `ROADMAP.md`, `ANALYSIS.md`

### Config Repo Structure (claude-team-control)

This is the **source of truth** for all shared rules, agents, and skills. Edits happen here; `sync.ps1` distributes to target projects.

| Directory/File | Purpose | File format |
|----------------|---------|-------------|
| `agents/` | Agent definitions -- one `*.md` per agent | YAML frontmatter + markdown prompt |
| `skills/` | Slash-command definitions -- one `*.md` per skill | YAML frontmatter + markdown prompt |
| `overlays/` | Per-project CLAUDE.md additions -- one `*.md` per project | Markdown (appended after base) |
| `base/CLAUDE.md` | **Single source of truth** for shared rules | Markdown -- the ONLY file in `base/` |
| `orchestrator/` | MCP server Python package (flat layout) | Python modules, `pyproject.toml` |
| `orchestrator/tests/` | pytest tests for the orchestrator | `test_*.py`, `conftest.py` |
| `docs/` | Project documentation | Markdown |
| `scripts/` | Utility scripts (sync-check, templates) | Python, PowerShell |
| `hooks/` | Claude Code hook scripts | PowerShell |
| `templates/` | Templates for new agents/projects | Markdown |
| `projects.json` | Project registry (paths, overlays, exclusions) | JSON -- committed |
| `projects.local.json` | User-specific path overrides | JSON -- **gitignored, never committed** |
| `providers.json` | Multi-model provider config | JSON -- committed, **NOT synced** to projects |
| `sync.ps1` | PowerShell sync distribution script | PowerShell with UTF-8 BOM |

**When creating a new file, place it by type:**
- New agent: `agents/<name>.md` (kebab-case, YAML frontmatter required)
- New skill: `skills/<name>.md` (kebab-case, YAML frontmatter required)
- New project overlay: `overlays/<project-key>.md` (name must match key in `projects.json`)
- New orchestrator module: `orchestrator/<name>.py` (flat layout, direct imports only -- no relative imports)
- New orchestrator test: `orchestrator/tests/test_<module>.py`
- New utility script: `scripts/<name>.py` or `scripts/<name>.ps1`
- New hook: `hooks/<name>.ps1`
- New documentation: `docs/<NAME>.md`

**Prohibited (never do these):**
- Do NOT create files in `base/` other than `CLAUDE.md`
- Do NOT put agent/skill files outside their designated directories
- Do NOT add Python packages to orchestrator without updating `pyproject.toml`
- Do NOT edit `projects.local.json` in commits -- it is user-specific and gitignored
- Do NOT store secrets, credentials, or API keys anywhere in this repo

### Target Project `.claude/` Directory

`sync.ps1` creates and manages this structure in each target project:

| File/Directory | Contents | Editable? |
|----------------|----------|-----------|
| `.claude/CLAUDE.md` | Composed from `base/CLAUDE.md` + project overlay | **NO** -- overwritten by sync |
| `.claude/agents/*.md` | Synced agent definitions | **NO** -- overwritten by sync |
| `.claude/commands/*.md` | Synced skill definitions | **NO** -- overwritten by sync |
| `.claude/.sync-manifest.json` | File hashes for desync detection | **NO** -- auto-generated, gitignored |

- When modifying rules: edit `base/CLAUDE.md` or `overlays/<project>.md` in the config repo, then run `/sync`. Never edit `.claude/CLAUDE.md` directly.
- When adding/excluding agents per project: edit `exclude_agents` in `projects.json`, then run `/sync`.
- When adding/removing skills per project: edit `include_skills` in `projects.json`, then run `/sync`.

## Agent & Tool Usage

- When a task requires information from an MCP server: call it. Never skip available MCP tools when they are relevant.
- When a task is complex (multi-file, multi-domain, deep analysis): delegate to a specialized agent via Task tool (Explore, Plan, Bash, general-purpose).
- When a repetitive task pattern emerges: create a new agent definition, document it in `docs/AGENTS.md`, and update these instructions.
- When multiple independent tool calls are needed: batch them in a single message. Never make sequential calls where parallel is possible.

## Automatic Task Routing (MANDATORY)

Before starting ANY implementation: assess the task scope and route it. Never ask the user "should I use an agent?" -- decide and proceed.

### Assessment Criteria

| Signal | Threshold | Route to |
|--------|-----------|----------|
| Files affected | >3 files | Pipeline or agents |
| Architecture change | Any (new component, API, data model) | `architect` agent, then pipeline |
| Security surface | Auth, input validation, crypto, secrets | `security-lead` agent |
| Bug complexity | Multi-component, race condition, data corruption | `/orchestrate bugfix` pipeline |
| New feature | Any user-facing feature | `/orchestrate feature` pipeline |
| Code review request | Any PR or diff review | `code-reviewer` agent (triggers L1 CV) |
| Audit request | Plan review, risk assessment | `lead-auditor` agent (triggers L1 CV) |
| Deployment | Any release, deploy, migration | `/orchestrate deploy` pipeline |

### Routing Decision

```
User request arrives
       |
  Is it a question / exploration / reading only?
       |
  +----+----+
 YES        NO (implementation needed)
  |         |
  v         v
Answer    Assess scope:
directly  |
          +-- Single file, cosmetic/trivial fix? -> Implement directly
          |
          +-- Single file, logic/security change? -> Use relevant agent
          |    (code-reviewer, security-lead, architect)
          |
          +-- Multiple files, one concern? -> Use relevant agent(s)
          |    Launch agents in sequence, pass context between them
          |
          +-- Multiple files, multiple concerns? -> Use /orchestrate pipeline
               Select pipeline type: feature / bugfix / deploy / qa / review
```

### Routing Rules

- When in doubt between direct implementation and agents: use agents.
- After selecting a route: announce it in one line. Example: "This touches auth + 4 files -> using feature pipeline with security-lead."
- When discovering mid-implementation that the task is more complex than assessed: stop. Re-route to a heavier workflow.

### MCP Orchestrator Integration

Before starting any non-trivial implementation: call `orchestrator.route_task(task_description)` and follow the returned routing decision.

- **`route_task(description)`** -- returns pipeline type, recommended agents, CV requirements, complexity estimate.
- **`get_agent_info(name)`** -- returns agent metadata: tier, model, tools, CV requirement, MCP servers.
- **`list_agents(tier?)`** -- lists available agents, optionally filtered by model tier.

**After receiving a route_task response:**
- Follow the returned `pipeline` and `agents` list.
- When `cv_required` is true: ensure cross-validation agents participate.
- When `llm_refinement_suggested` is true: apply extra judgment (rule-based routing had low confidence).
- When the orchestrator MCP server is unavailable: fall back to the manual routing rules above.

### Cross-Validation via Orchestrator (Level 2)

After completing each pipeline stage involving a CV-enabled agent: call `orchestrator.cv_gate(stage_output, gate_type)` before proceeding.

| cv_gate result | Action |
|----------------|--------|
| `PASS` | Continue to next step |
| `HALT` | Fix the CRITICAL issue, re-submit the step output |
| `DISPUTE` | Call `orchestrator.cross_validate(topic, claude_analysis)` for multi-round debate |
| `SKIP` | Continue (CV temporarily unavailable, log warning) |
| `FAIL` | Report configuration error to user |

**Gate types:** `consensus` (architecture), `codereview` (code), `thinkdeep` (deep analysis), `precommit` (pre-commit).

PAL MCP tools provide **Level 1** agent-initiated CV. The orchestrator's `cv_gate` provides **Level 2** pipeline-enforced CV. Both coexist (defense-in-depth).

### Pipeline Execution via Orchestrator (Level 3)

When a task requires multi-step execution: use orchestrator pipeline tools instead of manually chaining agents.

- **`start_pipeline(pipeline_type, description)`** -- initialize pipeline, get pipeline_id and first step.
- **`complete_step(pipeline_id, step_output)`** -- report step completion. Server runs CV-gate if required. Returns next step or HALT/PIPELINE_COMPLETE.
- **`pipeline_status(pipeline_id)`** -- get current state. Use to resume after context window reset.

**Pipeline types:** `feature`, `bugfix`, `deploy`, `audit`, `qa`, `review`, `refactor`, `incident`, `migration`, `spike`, `perf`, `onboard`, `docs`, `techdebt`, `deep-validate`.

**Execution sequence:**
1. Call `route_task(description)` -- get recommended `pipeline` type.
2. Call `start_pipeline(pipeline_type, description)` -- get pipeline_id and first step.
3. Execute the step using the assigned agent.
4. Call `complete_step(pipeline_id, step_output)` -- server runs CV-gate if needed.
5. On `HALT`: fix the issue, re-submit the same step.
6. On `next_step`: execute next agent, repeat from step 4.
7. On `PIPELINE_COMPLETE`: pipeline finished.

**Pipeline rules:**
- Never skip `complete_step` -- the server tracks state and enforces CV gates.
- Pipeline state persists to disk and survives context resets.
- After a context window reset: call `pipeline_status` to resume with full context.
- Optional steps (e.g., frontend-dev, visual-qa) are auto-skipped when not applicable.

## Permissions

- When reading log/output files (`.output`, `*.log`, `*.txt` in temp dirs, server stdout/stderr, test runner output): read without asking for confirmation.
- When reading project source files (any file within the project directory or related project directories): read without asking for confirmation.
- When reading configuration files (`.env`, `*.json`, `*.toml`, `*.yaml`, `*.cfg` in project directories): read without asking for confirmation.

## Git & GitLab

- After creating a git commit: remind the user to push to GitLab (or offer to push). Never let commits accumulate locally.
- At the start of a session: run `git status` and `git log origin/main..HEAD`. When unpushed commits exist: notify the user immediately.
- When pushing: use `git push origin main` (or the current branch name). Never force-push without explicit user approval.

## Independent Audit (MANDATORY)

After creating any implementation plan: conduct a structured audit before approving for execution. No implementation begins without audit approval.

### When to Run the Audit

- After plan design (before user approval / ExitPlanMode).
- After implementing changes touching >3 files (before commit).
- After major refactoring.

### Audit Workflow

Every APPROVE verdict (specialist or Chief Architect) must include Verification Evidence (see format below). An APPROVE without evidence is invalid.

1. **Launch Lead Auditor** -- start a `general-purpose` agent as Lead Auditor / Team Lead.
   - The Lead Auditor reads the plan and identifies required domain expertise.
   - The Lead Auditor delegates review to one or more Specialist Auditor agents, each with clear domain scope.

2. **Specialist Auditors execute** -- launched by Lead Auditor or in parallel by orchestrator.
   - Each Specialist receives a focused scope (e.g., "audit database query patterns", "audit backward compatibility").
   - Before issuing any verdict: complete all applicable items in the Audit Depth Checklist (below).
   - When auditing code or architecture changes: call `mcp__pal__thinkdeep`. Surface-level reasoning is insufficient.
   - When auditing docs-only, config-only, or single-file trivial changes: PAL usage is recommended but not mandatory.
   - Produce one verdict: **APPROVE** / **REJECT with findings** (CRITICAL/HIGH/MEDIUM/LOW + fix recommendations) / **ESCALATE to user**.

3. **Chief Architect Review** -- after all Specialist Auditors finish, the Lead Auditor performs a holistic review:
   - Focus on cross-domain gaps no single specialist could see. Validate that specialist findings do not contradict each other.
   - Before issuing verdict: call `mcp__pal__consensus` for cross-domain validation and read source code at integration points.
   - Produce verdict: APPROVE / REJECT with findings / ESCALATE.

4. **No inventing, no guessing** -- auditors must not fabricate concerns. Only concrete, verifiable findings from actual code analysis and documentation. When unsure: ESCALATE, never assume.

5. **On REJECT** -- fix all CRITICAL and HIGH issues, re-submit to the same auditor. Repeat until APPROVE or ESCALATE. After specialist fixes: Chief Architect re-reviews the whole plan.
   - When re-audit finds CRITICAL issues in a previously APPROVED plan: trigger the Audit Failure Protocol (see "Zero CRITICAL on Re-audit").

6. **Final outcome:**
   - All auditors + Chief Architect APPROVE: implementation begins.
   - Any level ESCALATE: notify user with the unresolved question.
   - Record the audit summary in the plan file.

### Execution Plan Requirement

After audit approval (all levels APPROVE): structure the plan as a detailed execution roadmap before implementing.

- Format as **Phase -> Steps**: each phase contains numbered, atomic steps.
- Each step has a **checkpoint**: what was done, what file changed, what to verify.
- The plan must be **resumable**: readable by any developer or agent to continue from last completed step.
- Mark completed steps with `[x]`; pending steps remain `[ ]`.
- Record commit hashes, test counts, and deviations inline after each phase.
- Save to `docs/ROADMAP.md` or a plan file -- never only in conversation memory.

### Per-Phase PAL Verification Gate (MANDATORY)

Before starting the next phase of any phased implementation plan: complete the verification gate for the current phase.

1. Run automated checks (`npm test`, `pytest`, etc.) -- must pass with zero failures.
2. Call `mcp__pal__codereview` on all files changed in this phase. On any CRITICAL finding: HALT, fix, re-review.
3. Call `mcp__pal__thinkdeep` for deep analysis of the phase's changes. On any CRITICAL: HALT.
4. Only after all automated checks pass AND both PAL tools return no CRITICAL findings: mark phase complete and proceed.

### End-of-Plan Double Audit (MANDATORY)

After all phases are complete and before committing:

1. Call `mcp__pal__precommit` -- full diff review, security scan, change impact assessment.
2. Call `mcp__pal__consensus` (multi-model, >=2 models) -- holistic architecture review.
3. When any finding >= HIGH: create a fix task, re-run the relevant phase gate, then re-run the double audit.

### Audit Scope Checklist

When auditing, check each of these:
- Logic gaps, race conditions, missing error handling
- Security holes (injection, XSS, auth bypass)
- Coupling issues, backward compatibility breaks
- Untested paths, wrong assumptions about APIs/libraries
- Performance regressions, deployment blind spots
- Blast radius -- which other components are affected

### Zero CRITICAL on Re-audit (ABSOLUTE RULE)

When a re-audit or implementation review discovers CRITICAL issues in a previously APPROVED plan: this is an Audit Failure. The initial audit was deficient.

**On Audit Failure:**
1. HALT -- stop all implementation immediately.
2. Root cause analysis -- document WHY the initial audit missed it in `docs/AUDIT.md` under "Audit Failures".
3. Full re-audit -- re-audit the entire plan from scratch, not just the failed area.
4. Process update -- add the gap to the Audit Depth Checklist to prevent recurrence.
5. Run `/orchestrate deep-validate` to achieve zero-finding state. Note: deep-validate is currently skill-orchestrated only; backend pipeline registration is a follow-up task.

### Audit Verification Evidence (MANDATORY)

Every APPROVE verdict must include this section:

```
## Verification Evidence
- **Files read**: [files with line ranges actually examined]
- **Documentation verified**: [context7 queries or WebSearch URLs consulted]
- **PAL tools used**: [tool name -> key conclusion]
- **Code patterns checked**: [Grep/Glob queries run, what was verified]
- **Edge cases analyzed**: [boundary conditions, error paths, concurrency scenarios]
- **Cross-domain risks**: [integration points checked]
```

- When a section is not applicable: explain why. Never leave sections empty.
- Evidence must be specific: "read `router.py:45-120`, verified route registration pattern" -- not "read the code".
- Record evidence in `docs/AUDIT.md` alongside the verdict.

### Audit Depth Checklist

Before issuing APPROVE, confirm each applicable item:

- [ ] **Source code read** -- all affected files read with `Read` tool (not just referenced)
- [ ] **Technical assumptions verified** -- every claim confirmed via context7 or WebSearch
- [ ] **PAL analysis performed** -- `thinkdeep` (specialist) or `consensus` (Chief Architect) called
- [ ] **Edge cases considered** -- boundary values, empty inputs, concurrent access analyzed
- [ ] **Security surface noted** -- security implications flagged for security specialist if beyond scope
- [ ] **Backward compatibility verified** -- existing consumers and dependents checked for breakage
- [ ] **Test coverage assessed** -- existing tests reviewed; gaps flagged
- [ ] **Cross-domain integration verified** -- interaction points with other modules checked

Report which items were completed and which were not applicable (with justification).

### Rules Architect Agent

When creating or modifying CLAUDE.md instructions: delegate to the Rules Architect agent. Never write rules ad-hoc from an implementation agent.

**Agent profile:**
- Type: `general-purpose` agent with role **Rules Architect**
- Expertise: technical writing, process design, CLAUDE.md conventions

**Before writing any rule, the Rules Architect must:**
- Consult Claude Code documentation via context7 or WebSearch for best practices.
- Study existing CLAUDE.md patterns in the project.

**Rule quality requirements (every rule must satisfy all five):**
- **Atomic** -- one rule = one concern.
- **Actionable** -- describes a concrete action, not an abstract goal.
- **Verifiable** -- possible to check whether followed.
- **Non-contradictory** -- no conflicts with existing rules; replacement rules state what they replace.
- **Scoped** -- clear when it applies and when it does not.

**Workflow:** Rules Architect produces a draft. Chief Architect reviews before applying to any CLAUDE.md.

## Database Protection (CRITICAL -- NEVER VIOLATE)

Enforced automatically by `protect-db.sh` hook -- blocks destructive commands on DB paths.

- When encountering any database file or directory (`*.db`, `*.sqlite`, `*.sqlite3`, `*chroma*`, `chroma_db/`, `pgdata`, `*redis*data`, `*mongo*data`, `*elastic*data`, `*mysql*data`, `*_db/`): NEVER delete it. Zero exceptions.
- Before any destructive operation on a DB path: create a backup first:
  1. `cp -r <db_dir> _archive/<db>_backup_$(date +%Y-%m-%d)/`
  2. Verify: `ls -la _archive/<db>_backup_*/`
  3. Only then proceed.
- Allowed operations: backup, copy, archive, read. Forbidden: `rm -rf`, `rmdir`, `shutil.rmtree()`, `DROP TABLE/DATABASE`, `docker volume rm`.
- When adding a new database to a project: add its path pattern to `hooks/protect-db.sh` `DB_PATTERN` and run `/sync`.

## Plan Continuity & Documentation (MANDATORY)

- After completing planning or any implementation phase: save the full plan to `docs/ROADMAP.md` with enough detail to resume from any point.
- After analyzing the codebase: save findings to `docs/ANALYSIS.md` (architecture, components, patterns, regex catalogs, configuration, known issues).
- When a phase produces critical changes: immediately update `docs/ROADMAP.md` to reflect impact on future phases.
- After completing a phase: update `docs/ROADMAP.md` with completion status, actual test counts, and commit hashes.
- When discovering a gotcha: add it to the roadmap's "Known Gotchas" section.
- Before committing (gate -- do not commit without this): update all documentation:
  - `docs/ROADMAP.md` -- mark completed phases, record commit context, update status tables.
  - `docs/ANALYSIS.md` -- reflect architectural changes, new patterns, updated regex catalogs.
  - `docs/AGENTS.md` -- if agents were created or modified.
  - `MEMORY.md` -- update project state (current phase, test counts, key lessons).
  - Code comments -- ensure new/changed functions have accurate docstrings.

## Documentation Quality (MANDATORY)

### When writing a document >100 lines, include:

- **Table of Contents** -- anchor-linked TOC at the top.
- **Mermaid diagrams** -- for architecture, flows, timelines, state machines, decision trees. Use `mermaid` code blocks.
- **Collapsible sections** -- `<details><summary>...</summary>...</details>` for verbose content.
- **Unicode emoji markers** -- use actual characters (checkmark, warning, construction), NOT GitHub shortcodes (`:white_check_mark:` etc.) -- shortcodes don't render in VSCode.
- **Bold blockquote callouts** -- `> **Note:**`, `> **Warning:**`, `> **Important:**` with emoji prefix -- NOT `> [!NOTE]` syntax (doesn't render in VSCode).
- **Aligned tables** -- use `:---|:---:|---:` for comparisons.

### When writing any document, always:

- Specify language tags on code blocks (` ```python `, ` ```typescript `, ` ```json `).
- Use **bold emphasis** for key terms and decisions.
- Place horizontal rules (`---`) between major sections.

### Compatibility rules

- Never use GitHub-only syntax (`> [!NOTE]`, emoji shortcodes) -- use universal alternatives.
- All formatting must render in VSCode Markdown Preview, GitLab, and GitHub.
- Mermaid requires `bierner.markdown-mermaid` VSCode extension.

## Plan Persistence After Thinking (MANDATORY)

Before starting implementation: verify that the plan is persisted to a file. Plans existing only in conversation context are invalid.

### Persistence Rules

| Trigger | Save to | Format |
|---------|---------|--------|
| After producing a plan in plan mode | `docs/PLAN.md` | Problem statement, options, decision + rationale, numbered steps |
| After PAL tools produce strategic findings | `docs/PLAN.md`, `docs/REVIEW.md`, or `docs/AUDIT.md` | Key conclusions summary |
| After making an architecture decision | `docs/adr/NNNN-<title>.md` | Context, Decision, Consequences, Status |
| After completing a spike/research | `docs/spikes/YYYY-MM-DD-<topic>.md` | Question, options, recommendation, evidence |
| After a postmortem | `docs/postmortems/YYYY-MM-DD-<title>.md` | Timeline, root cause, impact, action items |

### Clean Context Gate

Before starting implementation, verify all four:
- [ ] Plan saved to `docs/` with clear execution steps.
- [ ] Each step has a checkpoint (what to verify).
- [ ] Steps are numbered and atomic (resumable from any point).
- [ ] No plan details exist ONLY in conversation -- all persisted to files.

### Artifact Index

After creating any decision artifact (ADR, spike, postmortem, plan): update `docs/INDEX.md` with a link to the new artifact.

## Session Start Protocol

At the start of each session, execute these steps in order:
1. Read `docs/PLAN.md` -- check for in-progress plans.
2. Read `docs/ROADMAP.md` -- check current phase status.
3. Call `list_active_pipelines()` -- check for interrupted pipelines.
4. When active pipelines exist: report them to the user with resume instructions before accepting new tasks.
5. When other pending work exists: report it before accepting new tasks.

## Context & Token Optimization (MANDATORY)

### Before Switching Tasks or Phases

- Before moving to a different feature, phase, or task domain: commit all current work and update `docs/`. Never carry stale context.
- When research or exploration exceeds 3 file reads: delegate to a Task agent. Never run heavy scanning in the main context.
- When a subagent returns results: extract only relevant findings. Never paste full tool outputs verbatim.

### During a Session

- Before reading a file: check if it was already read in this conversation and not modified since. Never re-read unchanged files.
- When multiple independent tool calls are needed: batch them in one message.
- When responding: use minimum words needed. No filler phrases, no restating the question.
- When tracking multi-step progress: use TodoWrite. Never write status paragraphs in chat.
- When using Glob: NEVER use `**/*.md` or any `**/*` pattern on project roots. Use targeted patterns:
  - `Glob("*.md", path="~/project")` -- root only
  - `Glob("docs/*.md", path="~/project")` -- specific subdir
  - `Bash("find ~/project -maxdepth 2 -name '*.md' -not -path '*/node_modules/*' -not -path '*/.venv/*'")` -- safe find
  - Delegate to a Task agent -- handles scanning internally

### Before a Session Ends (Context Reset)

- Before context compresses or session ends: persist all state to files (`docs/PLAN.md`, `docs/ROADMAP.md`, pipeline state via `complete_step`, MEMORY.md).
- Plan files must be resumable: any developer or agent reading `docs/PLAN.md` must be able to continue from the last checkpoint.

## Cost-Aware Development (MANDATORY)

### Scripts Over Agents

When performing these tasks, use CLI tools, not LLM agents:

| Task | Use Script | NOT Agent |
|------|-----------|-----------|
| Linting | `ruff check .` | code-reviewer |
| Formatting | `ruff format .` | precommit gate |
| Type checking | `pyright` / `mypy` | codereview gate |
| Dead code detection | `vulture .` | techdebt pipeline |
| Dependency audit | `pip-audit` / `safety` | security-lead |
| Secret scanning | `trufflehog` / `detect-secrets` | security-lead |
| Import sorting | `isort --check .` | precommit gate |
| Doc link checking | `markdown-link-check` | doc-writer |
| Git diff stats | `git diff --stat` | code-reviewer |
| Test runner | `pytest -q` | test-engineer |

### CV Gate Applicability

**Skip CV gates for:** docs-only changes, config/formatting changes, single-file trivial fixes, tech debt cleanup (dead code, import sorting, lint fixes).

**Require CV gates for:** architecture decisions, security-sensitive changes, multi-file refactoring that changes behavior, production deployments and migrations.

### Cost Monitoring

- When daily cost exceeds $1.00: review the cost report and reduce CV gate usage.
- Use `cost_report` MCP tool for in-session analytics with optimization hints.

### Zero-Token CLI Tools (config repo only)

These scripts live in `claude-team-control/scripts/` and run without LLM tokens:

**Cost analytics** (`scripts/cost-report.py`):
- `python scripts/cost-report.py` -- full cost report
- `python scripts/cost-report.py --days 7` -- last 7 days
- `python scripts/cost-report.py --save` -- persist to `docs/COST-REPORT.md`
- `python scripts/cost-report.py --json` / `--csv` -- machine-readable
- `python scripts/cost-report.py --budget 1.00` -- warn if daily >$1.00 (exit code 1)
- `python scripts/cost-report.py --budget-total 5.00` -- warn if total >$5.00

**Pipeline statistics** (`scripts/pipeline-stats.py`):
- `python scripts/pipeline-stats.py` -- completion rates, avg time, failure points
- `python scripts/pipeline-stats.py --type feature` -- filter by pipeline type
- `python scripts/pipeline-stats.py --active` -- active/halted pipelines only
- `python scripts/pipeline-stats.py --json` -- JSON output

**Sync validation** (`scripts/sync-validate.py`):
- `python scripts/sync-validate.py` -- check all 5 projects + global
- `python scripts/sync-validate.py --project pdap-hub` -- single project
- `python scripts/sync-validate.py --fix` -- show fix commands
- Exit code 1 if any project desynced

**Orchestrator management** (`scripts/orchestrate-cli.py`):
- `python scripts/orchestrate-cli.py health` -- check config, venv, costs, API key
- `python scripts/orchestrate-cli.py pipelines` -- list all pipelines
- `python scripts/orchestrate-cli.py version` -- show orchestrator version
- Import-based commands (require `cd orchestrator && uv run`):
  - `uv run python ../scripts/orchestrate-cli.py agents [--tier strategic]` -- list agents
  - `uv run python ../scripts/orchestrate-cli.py info <agent>` -- agent details
  - `uv run python ../scripts/orchestrate-cli.py route "<task>"` -- route task locally

Note: These scripts are NOT deployed to target projects. Config-repo utilities only.

## Testing & Mock Data (CRITICAL)

- Before creating or updating a fixture/mock file: query the real external service to capture the actual response format. Never fabricate or guess formats.
- When the real format is broken: file a bug against the upstream service. Never invent a workaround format.
- When writing unit tests: include test cases using the real response format, not just mock format.
- When reviewing tests: reject tests that only verify fabricated formats.

## Agent Memory (ALL AGENTS)

### After These Events, Update MEMORY.md:

- After completing a significant task (new feature, bug fix, sprint).
- After discovering a new gotcha or pattern.
- Before session ends (before context is lost).
- After receiving a correction from the user.

### What to Save

- **Project patterns** -- coding conventions, mock structures, patching patterns.
- **Key file locations** -- important files, their purpose, line ranges of interest.
- **Gotchas discovered** -- pitfalls, workarounds, framework quirks.
- **Sprint/phase state** -- current progress, test counts, remaining work.
- **Decisions made** -- technical choices and rationale.

### What NOT to Save

- Session-specific context (current task details, in-progress debugging).
- Information that duplicates project docs (README, ROADMAP).
- Unverified assumptions -- only save confirmed patterns.
- Large code snippets -- reference file paths and line numbers instead.

## Collaboration Protocol (ALL AGENTS)

When a task requires expertise from another specialist:
1. Do NOT attempt work another agent is better suited for.
2. Complete the current work phase.
3. Return results with a handoff recommendation:

   **NEEDS ASSISTANCE:**
   - **Agent**: [agent name, e.g. security-auditor]
   - **Why**: [why this specialist is needed]
   - **Context**: [what to pass -- files, lines, findings]
   - **After**: [continue my work / hand to human / chain to third agent]

## Mindset

- When faced with multiple approaches: consider trade-offs before choosing. Prefer the simplest working solution.
- When process conflicts with pragmatism: focus on what works, not ceremony.
- When spotting improvement opportunities: suggest them proactively, but do not force them.


<!-- === Project-specific overlay: vibing-steampunk.md === -->


## Go Development Patterns

- **Error handling**: Always check `err != nil` immediately after function calls
- **Naming**: Use Go conventions — `camelCase` for unexported, `PascalCase` for exported
- **Testing**: `go test ./...` for all tests, `go test -v -run TestName` for specific
- **Dependencies**: Use `go mod tidy` after adding/removing imports

## SAP ABAP Conventions

- **Z/Y naming**: All custom objects MUST use Z_ or Y_ prefix (SAP namespace rules)
- **Transport management**: Every change requires a transport request. Use `/transport-deploy` skill for transport workflows
- **ABAP naming**: Class names uppercase, methods camelCase, variables with type prefix (lv_, lt_, lo_, etc.)
- **Unit tests**: Use ABAP Unit framework. Run via `RunUnitTests` MCP tool after every change
- **ATC checks**: Run `RunATCCheck` before transport release to catch quality issues

## VSP MCP Integration

- Use `vsp-sc3` MCP server for all SAP object operations
- Key tools: `SearchObject`, `GetSource`, `WriteSource`, `Activate`, `RunUnitTests`, `RunATCCheck`, `GetCallGraph`
- Use `pdap-docs` MCP for Process Director knowledge base (search_fixes, query_docs)

## Security Note

- SAP credentials MUST be stored in `.env` or credential manager, NEVER in committed files
- Do NOT hardcode passwords in `settings.local.json` — use environment variables

