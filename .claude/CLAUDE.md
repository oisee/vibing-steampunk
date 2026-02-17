<!-- DO NOT EDIT -- managed by sync.ps1 from claude-team-config -->
<!-- Synced: 2026-02-17 19:47:09 -->
<!-- Base: base/CLAUDE.md | Overlay: overlays/vibing-steampunk.md -->


## Requirements

Never hallucinate or fabricate information. If you're unsure about anything, you MUST explicitly state your uncertainty. Say "I don't know" rather than guessing or making assumptions. Honesty about limitations is required.

## Research & Verification

Before implementing solutions or suggesting approaches:
- **Check documentation first** - Use WebSearch, WebFetch, or context7 MCP tools to verify your assumptions against official documentation and reliable sources
- **Validate technical decisions** - Don't assume APIs, libraries, or patterns work in certain ways - look it up
- **Research before building** - If you're unsure about the best approach, research available options before coding

## Project Organization

- **No test/temp files in root** - All temporary files, test outputs, logs, and experimental scripts MUST go into appropriate subdirectories:
  - `_archive/` - for temporary and backup files
  - `tests/` - for test files
  - `logs/` - for log outputs
  - Project root should only contain core application files and documentation
- Keep the project structure clean and professional

## Agent & Tool Usage

- **Use all available MCP tools** - Leverage all configured MCP servers when relevant to the task
- **Use specialized agents** - Utilize Task tool with appropriate agents (Explore, Plan, Bash, etc.) for complex tasks
- **Create agents when needed** - If a repetitive task pattern emerges that would benefit from a specialized agent:
  1. Create the agent with clear responsibilities
  2. Document it in project docs (README.md or docs/AGENTS.md)
  3. Update these instructions to reference the new agent
- **Parallel execution** - Run independent tasks in parallel using multiple tool calls in a single message
- **Agent collaboration** - Agents may request help from other specialists via the NEEDS ASSISTANCE protocol. The orchestrator (or main context) handles chaining.

## Permissions

- **Always allow reading log/output files** - Reading temporary output files (task logs, server output, background process output) should NEVER require user confirmation
- **Always allow reading project source files** - Reading any file within the project directory and related project directories should not require confirmation
- **Always allow reading configuration files** - Reading `.env`, `*.json`, `*.toml`, `*.yaml`, `*.cfg` files in any project directory should not require confirmation

## Git & GitLab

- **Push after commits** - After creating git commits, ALWAYS remind the user to push to GitLab (or offer to push). Don't let commits accumulate locally without pushing.
- **Check unpushed commits** - At the start of a session, check `git status` and `git log origin/main..HEAD` for unpushed commits. If there are any, remind the user.
- **Push command** - Use `git push origin main` (or the current branch name). Never force-push without explicit user approval.

## Independent Audit (MANDATORY)

After creating any implementation plan, a structured audit **must** be conducted before the plan is approved for execution. No implementation begins without audit approval.

### Audit Workflow

1. **Audit Leader Agent** â€” launch a `general-purpose` agent acting as **Lead Auditor / Team Lead**.
   - The Lead Auditor reads the plan and determines which domain expertise is required (backend, database, security, API design, performance, etc.).
   - The Lead Auditor delegates the review to one or more **Specialist Auditor** agents â€” each with clear domain scope.

2. **Specialist Auditor Agent(s)** â€” launched by the Lead Auditor (or in parallel by the orchestrator).
   - Each Specialist Auditor is given a focused scope (e.g., "audit database query patterns", "audit search algorithm correctness", "audit backward compatibility").
   - The Specialist Auditor **must verify technical assumptions** using all available means: external sources (WebSearch, context7, official docs), own knowledge/memory, and any relevant MCP tools. No single source is sufficient â€” cross-check when possible.
   - The Specialist Auditor produces a verdict:
     - **APPROVE** â€” no issues found, plan is sound
     - **REJECT with findings** â€” list of issues ranked by severity (CRITICAL / HIGH / MEDIUM / LOW) with specific fix recommendations
     - **ESCALATE to user** â€” unresolvable ambiguity or risk that requires human decision

3. **Chief Architect Review** â€” after all Specialist Auditors finish, the **Lead Auditor** performs a final holistic review as **Chief Architect**:
   - Has access to the full picture: all specialist findings + the original plan + codebase context
   - Focuses on **cross-domain gaps** that no single specialist could see
   - Validates that specialist findings don't contradict each other
   - Checks that the plan as a whole is coherent, not just that individual parts are correct
   - Produces the same verdict: APPROVE / REJECT with findings / ESCALATE

4. **No inventing, no guessing** â€” auditors at all levels must not fabricate concerns or imagine problems. Only concrete, verifiable findings based on actual code analysis and documentation. If unsure â€” escalate to user, do not assume.

5. **Iteration** â€” if any audit level returns REJECT:
   - Fix all CRITICAL and HIGH issues in the plan
   - Re-submit to the same auditor for re-review
   - Repeat until APPROVE or ESCALATE
   - After specialist fixes, Chief Architect re-reviews the whole plan again

6. **Final outcome:**
   - **All auditors + Chief Architect APPROVE** â†’ plan is approved, implementation begins
   - **Any level ESCALATE** â†’ user is notified with the specific unresolved question, user makes the decision
   - The audit summary (specialist findings + architect review + final verdict) is recorded in the plan file

### Execution Plan Requirement

After the audit is fully approved (all levels APPROVE), the final plan **must** be structured as a detailed execution roadmap before implementation begins:

- **Phase â†’ Steps** format: each phase contains numbered, atomic steps that can be executed independently
- Each step has a **clear checkpoint**: what was done, what file was changed, what to verify
- The plan must be **resumable**: if the context window is cleared or a new session starts, any developer (or agent) can read the plan file and continue from the last completed step without re-gathering context
- Mark completed steps with `[x]` as work progresses; pending steps remain `[ ]`
- Record commit hashes, test counts, and deviations inline after each phase completion
- Save the plan to a persistent location (plan file or `docs/ROADMAP.md`) â€” not just in conversation memory

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
- Blast radius â€” which other components are affected by the change

### Rules Architect Agent

Rules and CLAUDE.md instructions must NOT be written ad-hoc by the implementation agent. A dedicated **Rules Architect Agent** is responsible for crafting, structuring, and maintaining CLAUDE.md rules across all projects.

**Rule quality principles:**
- **Atomic** â€” one rule = one concern, no compound sentences mixing unrelated requirements
- **Actionable** â€” each rule describes a concrete action, not an abstract goal
- **Verifiable** â€” it must be possible to check whether the rule was followed
- **Non-contradictory** â€” no conflicts with existing rules; if replacing a rule, explicitly state what it replaces
- **Scoped** â€” clearly state when the rule applies and when it doesn't

## Database Protection (CRITICAL â€” NEVER VIOLATE)

- **NEVER delete databases** (ChromaDB `chroma_db/` directories, Docker volumes, SQLite files). This is an absolute rule with ZERO exceptions.
- **Before full re-index or destructive operation**: ALWAYS create a backup first:
  1. Copy database directory to `_archive/<db>_backup_YYYY-MM-DD/`
  2. Verify the backup exists and has correct size
  3. Only then proceed
- **Allowed operations**: backup, copy, archive, read. **Forbidden**: delete, drop, rm -rf, `shutil.rmtree()` on DB
- **Double verification**: Any code that touches a database path destructively must be reviewed twice â€” once by the implementer, once by the audit agent

## Plan Continuity & Documentation (MANDATORY)

When working on phased implementation plans:

- **Save detailed plans to project documentation** â€” After completing planning or any phase, save the full plan to `docs/ROADMAP.md` with enough detail to resume from any point without additional context gathering
- **Document all analysis** â€” Save comprehensive codebase analysis to `docs/ANALYSIS.md` including: architecture, all components, patterns, configuration, known issues
- **Track deviations** â€” When a phase produces critical changes (bug fixes, architectural decisions, pattern changes), immediately update the roadmap to reflect impact on future phases
- **Mark completed phases** â€” Update `docs/ROADMAP.md` with completion status, actual test counts, and commit hashes after each phase
- **Record learned patterns** â€” When discovering gotchas, document them in the roadmap's "Known Gotchas" section for future phases
- **Update MEMORY.md** â€” Keep the auto-memory file current with project state
- **Update all documentation before commit** â€” After all changes are implemented and tests pass, but BEFORE committing and pushing, update all relevant documentation. This is a gate: no commit without documentation being current.

## Testing & Mock Data (CRITICAL)

- **Mock fixtures must match real formats** â€” NEVER fabricate or guess response formats for fixture/mock files. Before creating or updating a fixture, query the real external service to capture the actual response format.
- **If real format is broken** â€” file a bug against the upstream service. Do not invent a workaround format for the fixture.
- **Tests must verify real patterns** â€” Unit tests must include test cases using the real response format (not just mock format). A test that passes against fake data but fails on real data is worse than no test at all.
- **Test what matters** â€” Tests that only verify fabricated formats provide zero value.

## Collaboration Protocol (ALL AGENTS)

If for quality execution of a task you need help from another specialist:
1. Do NOT try to do work that another agent is better suited for
2. Complete your current work phase
3. Return results with a recommendation: which agent is needed, what context to pass, what to do with the result
4. Format:
   **NEEDS ASSISTANCE:**
   - **Agent**: [agent name, e.g. security-auditor]
   - **Why**: [why this specialist is needed]
   - **Context**: [what to pass â€” files, lines, findings]
   - **After**: [continue my work / hand to human / chain to third agent]

## Mindset

- **Think broadly** - Consider multiple approaches and their trade-offs
- **Be practical, not formal** - Focus on what works, not on ceremony
- **Don't overcomplicate** - Simple, working solutions beat complex, "perfect" ones
- **Be proactive** - Suggest improvements when you see opportunities, but don't force them


<!-- === Project-specific overlay: vibing-steampunk.md === -->


## Go Development Patterns

- **Error handling**: Always check `err != nil` immediately after function calls
- **Naming**: Use Go conventions â€” `camelCase` for unexported, `PascalCase` for exported
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
- Do NOT hardcode passwords in `settings.local.json` â€” use environment variables

