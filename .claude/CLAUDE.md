<!-- DO NOT EDIT -- managed by sync.ps1 from claude-team-config -->
<!-- Synced: 2026-02-20 00:20:42 -->
<!-- Base: base/CLAUDE.md | Overlay: overlays/vibing-steampunk.md -->


## Requirements

Never hallucinate or fabricate information. If you're unsure about anything, you MUST explicitly state your uncertainty. Say "I don't know" rather than guessing or making assumptions. Honesty about limitations is required.

## Language & Terminology

- **All code artifacts in English.** Code, comments, docstrings, variable/function names, README files, commit messages, and diagrams — always in English.
- **No transliteration of English technical terms into Cyrillic.** If there is no established Russian equivalent, use the original term in Latin script (git stash, merge, rebase, commit, pull request), not Cyrillic transliterations like "стэшить", "мержить", "ребейзить", "коммит".
- **Conversation with the user** — in the language the user writes in.

## Research & Verification

Before implementing solutions or suggesting approaches:
- **Check documentation first** - Use WebSearch, WebFetch, or context7 MCP tools to verify your assumptions against official documentation and reliable sources
- **Validate technical decisions** - Don't assume APIs, libraries, or patterns work in certain ways - look it up
- **Research before building** - If you're unsure about the best approach, research available options before coding

### Thorough Analysis with Tools (MANDATORY)

For **every request** — including plan mode — conduct analysis using actual tools, not reasoning from memory alone:

- **Always use tools for analysis** — Read files (Read, Glob, Grep), search code, examine actual project state. Never plan or analyze based solely on assumptions or cached knowledge.
- **Leverage all relevant MCP tools** — Use configured MCP servers (context7 for docs, pal for deep analysis, etc.) to verify information, look up documentation, and validate approaches.
- **Use specialized agents when needed** — For complex analysis, delegate to appropriate agents (Explore for codebase research, architect for design decisions, security-lead for security review).
- **Plan mode is not passive** — In plan mode, actively explore the codebase: read relevant files, search for patterns, check dependencies, run research queries. Plans must be grounded in actual code analysis, not theoretical reasoning.
- **No "thinking only" analysis** — If the task involves code, architecture, or technical decisions, at least one tool call (Read, Grep, Glob, WebSearch, MCP, or agent) must be made before forming conclusions.

### MCP PAL for Thinking and Plan Validation (MANDATORY)

Use the **PAL MCP server** tools to externalize reasoning, validate plans, and audit decisions — do not keep complex reasoning purely internal:

- **`thinkdeep`** — Use for any non-trivial problem analysis: architecture decisions, complex bugs, performance issues, security analysis. Provides systematic hypothesis testing with expert validation. Use when reasoning requires more than surface-level analysis.
- **`planner`** — Use when designing implementation plans, migration strategies, or multi-step workflows. Builds plans incrementally with deep reflection. Every plan created in plan mode should be validated through `planner` before presenting to the user.
- **`consensus`** — Use for critical decisions where multiple perspectives matter: technology choices, architectural trade-offs, feature design. Consults multiple models to synthesize a balanced recommendation. Use when the decision has significant long-term impact.
- **`codereview`** — Use after writing or modifying code. Performs systematic review covering quality, security, performance, and architecture. Do not skip this step for non-trivial changes.
- **`precommit`** — Use before committing changes. Validates git changes, checks for security issues, assesses change impact. Provides structured pre-commit validation.
- **`challenge`** — Use when you or the user question a previous conclusion. Forces critical re-evaluation instead of reflexive agreement.
- **`chat`** — Use for brainstorming, getting a second opinion, or exploring ideas collaboratively with an external model.

**When to use PAL (minimum requirements):**
- Plan mode → at least `planner` or `thinkdeep` for plan validation
- Code changes → `codereview` after implementation, `precommit` before commit
- Architecture/design decisions → `consensus` or `thinkdeep`
- Debugging complex issues → `thinkdeep` or `debug`
- Disagreement or uncertainty → `challenge` or `consensus`

## Project Structure

### General Rules (All Projects)

- **Root directory** — only core application files, config files, and README.md. No scratch files, experiments, temp outputs, or logs.
- **Standard directories** — every project should use these as needed:

| Directory | Purpose | Rules |
|-----------|---------|-------|
| `docs/` | Documentation | ROADMAP.md, ANALYSIS.md, ARCHITECTURE.md, AGENTS.md |
| `tests/` | Test files | Mirror source structure, prefix with `test_` |
| `logs/` | Log outputs | Gitignored, created at runtime, never committed |
| `_archive/` | Backups and temp files | Database backups, old versions, scratch — gitignored |
| `.claude/` | Claude Code config | **Managed by sync.ps1 — NEVER edit directly** |

- **Naming conventions:**
  - Directories and non-Python files: `kebab-case` (e.g., `sync-check.py`, `plan-feature.md`)
  - Python modules: `snake_case` (e.g., `file_util.py`, `test_router.py`)
  - Uppercase exceptions: `CLAUDE.md`, `README.md`, `ROADMAP.md`, `ANALYSIS.md`
- **Never** put temporary files, test outputs, logs, or experimental scripts in the project root
- **Never** create top-level directories without documenting their purpose

### Config Repo Structure (claude-team-config)

This is the **source of truth** for all shared rules, agents, and skills. Edits happen here; `sync.ps1` distributes to target projects.

| Directory/File | Purpose | File format |
|----------------|---------|-------------|
| `agents/` | Agent definitions — one `*.md` per agent | YAML frontmatter + markdown prompt |
| `skills/` | Slash-command definitions — one `*.md` per skill | YAML frontmatter + markdown prompt |
| `overlays/` | Per-project CLAUDE.md additions — one `*.md` per project | Markdown (appended after base) |
| `base/CLAUDE.md` | **Single source of truth** for shared rules | Markdown — the ONLY file in `base/` |
| `orchestrator/` | MCP server Python package (flat layout) | Python modules, `pyproject.toml` |
| `orchestrator/tests/` | pytest tests for the orchestrator | `test_*.py`, `conftest.py` |
| `docs/` | Project documentation | Markdown |
| `scripts/` | Utility scripts (sync-check, templates) | Python, PowerShell |
| `hooks/` | Claude Code hook scripts | PowerShell |
| `templates/` | Templates for new agents/projects | Markdown |
| `projects.json` | Project registry (paths, overlays, exclusions) | JSON — committed |
| `projects.local.json` | User-specific path overrides | JSON — **gitignored, never committed** |
| `providers.json` | Multi-model provider config | JSON — committed, **NOT synced** to projects |
| `sync.ps1` | PowerShell sync distribution script | PowerShell with UTF-8 BOM |

**Placement rules — where to put new files:**
- New agent → `agents/<name>.md` (kebab-case, YAML frontmatter required)
- New skill → `skills/<name>.md` (kebab-case, YAML frontmatter required)
- New project overlay → `overlays/<project-key>.md` (name must match key in `projects.json`)
- New orchestrator module → `orchestrator/<name>.py` (flat layout, direct imports only — no relative imports)
- New orchestrator test → `orchestrator/tests/test_<module>.py`
- New utility script → `scripts/<name>.py` or `scripts/<name>.ps1`
- New hook → `hooks/<name>.ps1`
- New documentation → `docs/<NAME>.md`

**Prohibited:**
- Do NOT create files in `base/` other than `CLAUDE.md`
- Do NOT put agent/skill files outside their designated directories
- Do NOT add Python packages to orchestrator without updating `pyproject.toml`
- Do NOT edit `projects.local.json` in commits — it is user-specific and gitignored
- Do NOT store secrets, credentials, or API keys anywhere in this repo

### Target Project `.claude/` Directory

`sync.ps1` creates and manages this structure in each target project:

| File/Directory | Contents | Editable? |
|----------------|----------|-----------|
| `.claude/CLAUDE.md` | Composed from `base/CLAUDE.md` + project overlay | **NO** — overwritten by sync |
| `.claude/agents/*.md` | Synced agent definitions | **NO** — overwritten by sync |
| `.claude/commands/*.md` | Synced skill definitions | **NO** — overwritten by sync |
| `.claude/.sync-manifest.json` | File hashes for desync detection | **NO** — auto-generated, gitignored |

- **All files in `.claude/` are managed by sync.ps1** — local edits will be overwritten on next sync
- To modify rules → edit `base/CLAUDE.md` or `overlays/<project>.md` in the config repo, then run `/sync`
- To add/exclude agents per project → edit `exclude_agents` in `projects.json`, then run `/sync`
- To add/remove skills per project → edit `include_skills` in `projects.json`, then run `/sync`

## Agent & Tool Usage

- **Use all available MCP tools** - Leverage all configured MCP servers when relevant to the task
- **Use specialized agents** - Utilize Task tool with appropriate agents (Explore, Plan, Bash, etc.) for complex tasks
- **Create agents when needed** - If a repetitive task pattern emerges that would benefit from a specialized agent:
  1. Create the agent with clear responsibilities
  2. Document it in project docs (README.md or docs/AGENTS.md)
  3. Update these instructions to reference the new agent
- **Parallel execution** - Run independent tasks in parallel using multiple tool calls in a single message
- **Agent collaboration** - Agents may request help from other specialists via the NEEDS ASSISTANCE protocol. The orchestrator (or main context) handles chaining.

## Automatic Task Routing (MANDATORY)

Before starting ANY implementation, assess the task and route it automatically. Users should NOT need to type `/orchestrate` or agent names — the system must select the right workflow on its own.

### Assessment criteria

Evaluate every incoming task against these signals:

| Signal | Threshold | Route to |
|--------|-----------|----------|
| Files affected | >3 files | Pipeline or agents |
| Architecture change | Any (new component, API, data model) | `architect` agent → pipeline |
| Security surface | Auth, input validation, crypto, secrets | `security-lead` agent |
| Bug complexity | Multi-component, race condition, data corruption | `/orchestrate bugfix` pipeline |
| New feature | Any user-facing feature | `/orchestrate feature` pipeline |
| Code review request | Any PR or diff review | `code-reviewer` agent (triggers L1 CV) |
| Audit request | Plan review, risk assessment | `lead-auditor` agent (triggers L1 CV) |
| Deployment | Any release, deploy, migration | `/orchestrate deploy` pipeline |

### Routing decision tree

```
User request arrives
       │
  Is it a question / exploration / reading only?
       │
  ┌────┴────┐
 YES        NO (implementation needed)
  │         │
  ▼         ▼
Answer    Assess scope:
directly  │
          ├─ Single file, cosmetic/trivial fix? → Implement directly
          │
          ├─ Single file, logic/security change? → Use relevant agent
          │    (code-reviewer, security-lead, architect)
          │    Agent's L1 CV Protocol activates automatically
          │
          ├─ Multiple files, one concern? → Use relevant agent(s)
          │    Launch agents in sequence, pass context between them
          │    L1 CV activates in each CV-enabled agent
          │
          └─ Multiple files, multiple concerns? → Use /orchestrate pipeline
               Select pipeline type: feature / bugfix / deploy / qa / review
               L1 + L2 (CV-Gates) + L3 (if disputes) all activate automatically
```

### Rules

- **When in doubt, use agents.** Over-checking wastes some tokens. Under-checking risks shipping bad code. Err toward agents.
- **Never ask the user "should I use an agent?"** — decide based on the criteria above and proceed.
- **Announce routing briefly** — tell the user which route was chosen and why, in one line. Example: "This touches auth + 4 files → using feature pipeline with security-lead."
- **Single-file trivial changes** are the ONLY case where direct implementation without agents is acceptable. Examples: typo fix, comment update, adding a log line, formatting.
- **If during implementation you discover the task is more complex than initially assessed** — stop, re-route to a heavier workflow. Do not continue with a light workflow for a heavy task.

### MCP Orchestrator Integration

Before starting any implementation task, call `orchestrator.route_task(task_description)` and follow the returned routing decision. The orchestrator provides:

- **`route_task(description)`** — Analyzes task complexity. Returns: pipeline type, recommended agents, CV requirements, complexity estimate, detected signals with weights.
- **`get_agent_info(name)`** — Returns agent metadata: tier, model, tools, CV requirement, MCP servers.
- **`list_agents(tier?)`** — Lists available agents, optionally filtered by model tier (strategic/execution/routine).

**Usage rules:**
- Call `route_task` for every non-trivial task before implementation.
- Follow the returned `pipeline` and `agents` list.
- If `cv_required` is true, cross-validation agents must participate.
- If `llm_refinement_suggested` is true, the rule-based routing had low confidence — apply extra judgment.
- If the orchestrator MCP server is unavailable, fall back to the manual routing rules in the "Automatic Task Routing" section above.

### Cross-Validation via Orchestrator (Level 2)

After completing each pipeline stage involving a CV-enabled agent:

1. Call `orchestrator.cv_gate(stage_output, gate_type)` before proceeding to the next step
2. If cv_gate returns `PASS` — continue to next step
3. If cv_gate returns `HALT` — fix the identified CRITICAL issue, then re-submit the step output
4. If cv_gate returns `DISPUTE` — call `orchestrator.cross_validate(topic, claude_analysis)` for multi-round debate
5. If cv_gate returns `SKIP` — continue (CV temporarily unavailable, log warning)
6. If cv_gate returns `FAIL` — report configuration error to user

**Gate types:** `consensus` (architecture decisions), `codereview` (code changes), `thinkdeep` (deep analysis), `precommit` (pre-commit validation).

PAL MCP tools (consensus, codereview, thinkdeep) remain available for **Level 1** agent-initiated cross-validation. The orchestrator's `cv_gate` provides **Level 2** pipeline-enforced cross-validation. Both coexist (defense-in-depth).

### Pipeline Execution via Orchestrator (Level 3)

For multi-step tasks, use the orchestrator's pipeline tools instead of manually chaining agents:

- **`start_pipeline(pipeline_type, description)`** — Initialize a pipeline. Returns pipeline_id and first step instructions.
- **`complete_step(pipeline_id, step_output)`** — Report step completion. Server runs CV-gate automatically if the step requires it. Returns next step instructions or HALT/PIPELINE_COMPLETE.
- **`pipeline_status(pipeline_id)`** — Get current pipeline state. Use to check progress or resume after context window reset.

**Pipeline types:** `feature` (9 steps), `bugfix` (4 steps), `deploy` (5 steps), `audit` (2 steps), `qa` (5 steps), `review` (2 steps).

**Workflow:**
1. Call `route_task(description)` — it returns the recommended `pipeline` type
2. Call `start_pipeline(pipeline_type, description)` — get pipeline_id and first step
3. Execute the step using the assigned agent
4. Call `complete_step(pipeline_id, step_output)` — server runs CV-gate if needed
5. If result is `HALT` — fix the issue and re-submit the same step
6. If result has `next_step` — execute next agent and repeat from step 4
7. If result is `PIPELINE_COMPLETE` — pipeline finished successfully

**Rules:**
- Never skip `complete_step` — the server tracks state and enforces CV gates
- Pipeline state is persisted to disk — survives context window resets
- Use `pipeline_status` to resume an interrupted pipeline with full context
- Optional steps (e.g., frontend-dev, visual-qa) are auto-skipped when not applicable
- Halted pipelines can be resumed by re-submitting the failed step with corrected output

## Permissions

- **Always allow reading log/output files** - Reading temporary output files (task logs, server output, background process output) should NEVER require user confirmation. This includes but is not limited to:
  - Claude Code background task output files (`.output` in temp directories)
  - Any `*.log`, `*.output`, `*.txt` files in system temp directories
  - Server stdout/stderr output files
  - Test runner output files
- **Always allow reading project source files** - Reading any file within the project directory and related project directories should not require confirmation. This includes all subdirectories, config files, fixture files, test files, and documentation.
- **Always allow reading configuration files** - Reading `.env`, `*.json`, `*.toml`, `*.yaml`, `*.cfg` files in any project directory should not require confirmation

## Git & GitLab

- **Push after commits** - After creating git commits, ALWAYS remind the user to push to GitLab (or offer to push). Don't let commits accumulate locally without pushing.
- **Check unpushed commits** - At the start of a session, check `git status` and `git log origin/main..HEAD` for unpushed commits. If there are any, remind the user.
- **Push command** - Use `git push origin main` (or the current branch name). Never force-push without explicit user approval.

## Independent Audit (MANDATORY)

After creating any implementation plan, a structured audit **must** be conducted before the plan is approved for execution. No implementation begins without audit approval.

### Audit Workflow

1. **Audit Leader Agent** — launch a `general-purpose` agent acting as **Lead Auditor / Team Lead**.
   - The Lead Auditor reads the plan and determines which domain expertise is required (backend, database, security, API design, performance, etc.).
   - The Lead Auditor delegates the review to one or more **Specialist Auditor** agents — each with clear domain scope.

2. **Specialist Auditor Agent(s)** — launched by the Lead Auditor (or in parallel by the orchestrator).
   - Each Specialist Auditor is given a focused scope (e.g., "audit database query patterns", "audit search algorithm correctness", "audit backward compatibility").
   - The Specialist Auditor **must verify technical assumptions** using all available means: external sources (WebSearch, context7, official docs), own knowledge/memory, and any relevant MCP tools. No single source is sufficient — cross-check when possible.
   - The Specialist Auditor produces a verdict:
     - **APPROVE** — no issues found, plan is sound
     - **REJECT with findings** — list of issues ranked by severity (CRITICAL / HIGH / MEDIUM / LOW) with specific fix recommendations
     - **ESCALATE to user** — unresolvable ambiguity or risk that requires human decision

3. **Chief Architect Review** — after all Specialist Auditors finish, the **Lead Auditor** performs a final holistic review as **Chief Architect**:
   - Has access to the full picture: all specialist findings + the original plan + codebase context
   - Focuses on **cross-domain gaps** that no single specialist could see
   - Validates that specialist findings don't contradict each other
   - Checks that the plan as a whole is coherent, not just that individual parts are correct
   - Produces the same verdict: APPROVE / REJECT with findings / ESCALATE

4. **No inventing, no guessing** — auditors at all levels must not fabricate concerns or imagine problems. Only concrete, verifiable findings based on actual code analysis and documentation. If unsure — escalate to user, do not assume.

5. **Iteration** — if any audit level returns REJECT:
   - Fix all CRITICAL and HIGH issues in the plan
   - Re-submit to the same auditor for re-review
   - Repeat until APPROVE or ESCALATE
   - After specialist fixes, Chief Architect re-reviews the whole plan again

6. **Final outcome:**
   - **All auditors + Chief Architect APPROVE** → plan is approved, implementation begins
   - **Any level ESCALATE** → user is notified with the specific unresolved question, user makes the decision
   - The audit summary (specialist findings + architect review + final verdict) is recorded in the plan file

### Execution Plan Requirement

After the audit is fully approved (all levels APPROVE), the final plan **must** be structured as a detailed execution roadmap before implementation begins:

- **Phase → Steps** format: each phase contains numbered, atomic steps that can be executed independently
- Each step has a **clear checkpoint**: what was done, what file was changed, what to verify
- The plan must be **resumable**: if the context window is cleared or a new session starts, any developer (or agent) can read the plan file and continue from the last completed step without re-gathering context
- Mark completed steps with `[x]` as work progresses; pending steps remain `[ ]`
- Record commit hashes, test counts, and deviations inline after each phase completion
- Save the plan to a persistent location (plan file or `docs/ROADMAP.md`) — not just in conversation memory

### When to run the audit

- After plan design (before user approval / ExitPlanMode)
- After implementation of changes touching >3 files (before commit)
- After major refactoring

### Audit scope checklist

- Logic gaps, race conditions, missing error handling
- Security holes (injection, XSS, auth bypass)
- Coupling issues, backward compatibility breaks
- Untested paths, wrong assumptions about APIs/libraries
- Performance regressions, deployment blind spots
- Blast radius — which other components are affected by the change

### Rules Architect Agent

Rules and CLAUDE.md instructions must NOT be written ad-hoc by the implementation agent. A dedicated **Rules Architect Agent** is responsible for crafting, structuring, and maintaining CLAUDE.md rules across all projects.

**Agent profile:**
- Type: `general-purpose` agent with role **Rules Architect**
- Expertise: technical writing, process design, CLAUDE.md conventions, task decomposition
- Before writing rules, the agent **must research best practices**: consult Claude Code documentation (via context7 or WebSearch), study existing CLAUDE.md patterns in the project, and review industry standards for AI agent instructions (clarity, atomicity, testability)

**Rule quality principles:**
- **Atomic** — one rule = one concern, no compound sentences mixing unrelated requirements
- **Actionable** — each rule describes a concrete action, not an abstract goal
- **Verifiable** — it must be possible to check whether the rule was followed
- **Non-contradictory** — no conflicts with existing rules; if replacing a rule, explicitly state what it replaces
- **Scoped** — clearly state when the rule applies and when it doesn't

**Workflow:**
- The Rules Architect produces a draft of new/updated rules
- The draft is reviewed by the Chief Architect (audit step 3) before being applied to any CLAUDE.md file
- If the Rules Architect agent doesn't exist yet — create it first: define the agent prompt template and document it in `docs/AGENTS.md`

## Database Protection (CRITICAL — NEVER VIOLATE)

- **NEVER delete databases** (ChromaDB `chroma_db/` directories, Docker volumes, SQLite files). This is an absolute rule with ZERO exceptions.
- **Before full re-index or destructive operation**: ALWAYS create a backup first:
  1. Copy database directory to `_archive/<db>_backup_YYYY-MM-DD/`
  2. Verify the backup exists and has correct size
  3. Only then proceed
- **Allowed operations**: backup, copy, archive, read. **Forbidden**: delete, drop, rm -rf, `shutil.rmtree()` on DB
- **Double verification**: Any code that touches a database path destructively must be reviewed twice — once by the implementer, once by the audit agent

## Plan Continuity & Documentation (MANDATORY)

When working on phased implementation plans:

- **Save detailed plans to project documentation** — After completing planning or any phase, save the full plan to `docs/ROADMAP.md` with enough detail to resume from any point without additional context gathering
- **Document all analysis** — Save comprehensive codebase analysis to `docs/ANALYSIS.md` including: architecture, all components, patterns, regex catalogs, configuration, known issues
- **Track deviations** — When a phase produces critical changes (bug fixes, architectural decisions, pattern changes), immediately update the roadmap to reflect impact on future phases
- **Mark completed phases** — Update `docs/ROADMAP.md` with completion status, actual test counts, and commit hashes after each phase
- **Record learned patterns** — When discovering gotchas, document them in the roadmap's "Known Gotchas" section for future phases
- **Update MEMORY.md** — Keep the auto-memory file current with project state
- **Update all documentation before commit** — After all changes are implemented and tests pass, but BEFORE committing and pushing, update all relevant documentation:
  - `docs/ROADMAP.md` — mark completed phases, record commit context, update status tables
  - `docs/ANALYSIS.md` — reflect any architectural changes, new patterns, updated regex catalogs
  - `docs/AGENTS.md` — if new agents were created or existing ones modified
  - `MEMORY.md` — update project state (current phase, test counts, key lessons learned)
  - Code comments — ensure new/changed functions have accurate docstrings
  - This is a gate: no commit without documentation being current

## Plan Persistence After Thinking (MANDATORY)

Every plan, analysis, or strategic decision produced during a session MUST be persisted before execution begins. Plans that exist only in conversation context are considered incomplete.

### Persistence Rules

1. **Plan Mode output** — Save to `docs/PLAN.md` before exiting plan mode. Format: problem statement, options considered, decision + rationale, numbered execution steps.
2. **ThinkDeep / PAL analysis** — If PAL tools produce strategic findings, summarize key conclusions in the relevant doc file (`docs/PLAN.md`, `docs/REVIEW.md`, or `docs/AUDIT.md`).
3. **Architecture decisions** — Save as ADR in `docs/adr/NNNN-<title>.md`. Template: Context, Decision, Consequences, Status (proposed/accepted/deprecated).
4. **Spike/Research results** — Save to `docs/spikes/YYYY-MM-DD-<topic>.md`. Must include: question, options explored, recommendation, evidence.
5. **Postmortems** — Save to `docs/postmortems/YYYY-MM-DD-<title>.md`. Must include: timeline, root cause, impact, action items.

### Clean Context Gate

Before starting implementation:
- Plan saved to `docs/` with clear execution steps
- Each step has a checkpoint (what to verify)
- Steps are numbered and atomic (resumable from any point)
- No plan details exist ONLY in conversation — all persisted to files

### Artifact Index

Maintain `docs/INDEX.md` as a table of contents for all decision artifacts (ADRs, spikes, postmortems, plans). Update after each new artifact is created.

## Session Start Protocol

At the start of each session:
1. Read `docs/PLAN.md` — check for in-progress plans
2. Read `docs/ROADMAP.md` — check current phase status
3. Check for uncompleted pipeline states (`pipeline_status` if orchestrator available)
4. Report any pending work to user before accepting new tasks

## Testing & Mock Data (CRITICAL)

- **Mock fixtures must match real formats** — NEVER fabricate or guess response formats for fixture/mock files. Before creating or updating a fixture, query the real external service to capture the actual response format.
- **If real format is broken** — file a bug against the upstream service. Do not invent a workaround format for the fixture.
- **Tests must verify real patterns** — Unit tests must include test cases using the real response format (not just mock format). A test that passes against fake data but fails on real data is worse than no test at all.
- **Test what matters** — Tests that only verify fabricated formats provide zero value.

## Collaboration Protocol (ALL AGENTS)

If for quality execution of a task you need help from another specialist:
1. Do NOT try to do work that another agent is better suited for
2. Complete your current work phase
3. Return results with a recommendation: which agent is needed, what context to pass, what to do with the result
4. Format:
   **NEEDS ASSISTANCE:**
   - **Agent**: [agent name, e.g. security-auditor]
   - **Why**: [why this specialist is needed]
   - **Context**: [what to pass — files, lines, findings]
   - **After**: [continue my work / hand to human / chain to third agent]

## Mindset

- **Think broadly** - Consider multiple approaches and their trade-offs
- **Be practical, not formal** - Focus on what works, not on ceremony
- **Don't overcomplicate** - Simple, working solutions beat complex, "perfect" ones
- **Be proactive** - Suggest improvements when you see opportunities, but don't force them


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

