---
name: planning-rules
description: Full planning, audit, and plan-persistence rules. Load this skill when designing implementation plans, entering plan mode, running audits, or before committing phased work.
---

# Planning, Audit & Plan Persistence Rules

This skill is automatically injected by `start-pipeline-gate.sh` and by `/plan-feature`. Load it manually with `/planning-rules` when doing any phased implementation.

---

## Independent Audit (MANDATORY)

After creating any implementation plan: conduct a structured audit before approving for execution. No implementation begins without audit approval.

### When to Run the Audit

- After plan design (before user approval / ExitPlanMode).
- After implementing changes touching >3 files (before commit).
- After major refactoring.

### Audit Workflow

Every APPROVE verdict (specialist or Chief Architect) must include Verification Evidence (see format below). An APPROVE without evidence is invalid.

1. **Launch Lead Auditor** -- start a `lead-auditor` agent (fallback: `general-purpose` only if `lead-auditor` is unavailable).
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

5. **On REJECT** -- fix all CRITICAL, HIGH, and MEDIUM issues, re-submit to the same auditor. **Audit is recursive**: repeat the fix + re-audit cycle until APPROVE (zero CRITICAL/HIGH/MEDIUM findings) or ESCALATE. Do not proceed while any MEDIUM+ finding is open. After specialist fixes: Chief Architect re-reviews the whole plan.
   - When re-audit finds CRITICAL, HIGH, or MEDIUM issues in a previously APPROVED plan: trigger the Audit Failure Protocol (see "Zero MEDIUM+ on Re-audit").

6. **Final outcome:**
   - All auditors + Chief Architect APPROVE: implementation begins.
   - Any level ESCALATE: notify user with the unresolved question.
   - Record the audit summary in the plan file.

7. **Session Summary (MANDATORY output after audit APPROVE or final ESCALATE):**
   After the audit completes (one final summary — not after each recursive pass), output to the user:

   **a) What was done** — one-paragraph summary: what changed, how many audit cycles ran, what findings were resolved.

   **b) Findings table** — every finding across all cycles:
   ```
   | ID | Severity | Description | Status | Action taken |
   |----|----------|-------------|--------|--------------|
   | M-01 | MEDIUM | ... | Fixed | Updated lead-auditor.md:288 |
   | L-02 | LOW | ... | Deferred | Tracked in docs/AUDIT.md |
   ```
   Status values: `Fixed` / `Deferred` / `Open (escalated)`.

   **c) Manual review table** — items user must verify by hand (separate from findings table):
   ```
   | Item | Why manual verification needed | Risk if skipped |
   |------|-------------------------------|-----------------|
   | Deferred M-06: UNC NTLM leak | Requires opt-in setting design | Security |
   ```
   Include: all Deferred and Open (escalated) findings (any severity), external system integrations not covered by automated tests, security controls requiring human sign-off. Exclude: Fixed findings (already auto-verified).

### Execution Plan Requirement

After audit approval (all levels APPROVE): structure the plan as a detailed execution roadmap before implementing.

- Format as **Phase -> Steps**: each phase contains numbered, atomic steps.
- Each step has a **checkpoint**: what was done, what file changed, what to verify.
- The plan must be **resumable**: readable by any developer or agent to continue from last completed step.
- Mark completed steps with `[x]`; pending steps remain `[ ]`.
- Record commit hashes, test counts, and deviations inline after each phase.
- Save to `docs/ROADMAP.md` or a plan file -- never only in conversation memory.
- Each phase that modifies code, data, or infrastructure must include a **Rollback** subsection: ordered steps to undo the phase changes if they cause breakage. For genuinely irreversible phases: write `Rollback: N/A — [mitigation plan: backup / forward-fix / feature-flag]` instead.

### Per-Phase PAL Verification Gate (MANDATORY)

Before starting the next phase of any phased implementation plan: complete the verification gate for the current phase.

1. Run automated checks (`npm test`, `pytest`, etc.) -- must pass with zero failures. For new code paths introduced in this phase: verify by reviewing the diff and new test files that corresponding tests exist -- not just that existing tests pass.
2. Call `mcp__pal__codereview` on all files changed in this phase. On any CRITICAL, HIGH, or MEDIUM finding: HALT, fix, re-review.
3. Call `mcp__pal__thinkdeep` for deep analysis of the phase's changes. On any CRITICAL, HIGH, or MEDIUM: HALT.
4. If PAL MCP is unavailable: perform steps 2-3 using internal cross-model review (Agent tool, different model tier — opus if current is sonnet; sonnet if current is opus). Document which fallback was used.
5. Only after all automated checks pass AND both PAL tools (or fallback) return zero MEDIUM+ findings: mark phase complete and proceed.
   If a finding is believed to be a false positive: use `mcp__pal__challenge` to contest it, or escalate to the user. Never silently skip or downgrade findings.

### End-of-Plan Double Audit (MANDATORY)

After all phases are complete and before committing:

1. Call `mcp__pal__precommit` -- full diff review, security scan, change impact assessment.
2. Call `mcp__pal__consensus` (multi-model, >=2 models) -- holistic architecture review.
3. When any finding >= MEDIUM: create a fix task, re-run the relevant phase gate, then re-run the double audit. Repeat until zero MEDIUM+ findings remain.

> **Note:** Invoking the `/finish` skill satisfies all requirements of this section. When `/finish` is run (manually or automatically via Per-Phase Gate step 6 / `/run` LOOP CONTROL), the End-of-Plan Double Audit is fulfilled — do not run it again separately.

### Audit Scope Checklist

When auditing, check each of these:
- Logic gaps, race conditions, missing error handling
- Security holes (injection, XSS, auth bypass)
- Coupling issues, backward compatibility breaks — before modifying any exported function or shared module: Grep for all call sites first
- Untested paths, wrong assumptions about APIs/libraries — use mcp__context7__resolve-library-id + mcp__context7__query-docs (or WebSearch when context7 lacks the library) to verify actual API behavior before flagging as a finding
- Performance regressions, deployment blind spots
- Blast radius -- which other components are affected

### Zero MEDIUM+ on Re-audit (ABSOLUTE RULE)

When a re-audit or implementation review discovers CRITICAL, HIGH, or MEDIUM issues in a previously APPROVED plan: this is an Audit Failure. The initial audit was deficient.

**On Audit Failure:**
1. HALT -- stop all implementation immediately.
2. Root cause analysis -- document WHY the initial audit missed it in `docs/AUDIT.md` under "Audit Failures".
3. Full re-audit -- re-audit the entire plan from scratch, not just the failed area.
4. Process update -- add the gap to the Audit Depth Checklist to prevent recurrence.
5. Run `/orchestrate deep-validate` to achieve zero-finding state.

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

---

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

---

## Task Granularity (Advisory)

Each task in a plan should be: **(a)** scoped to one logical concern, **(b)** independently verifiable with a specific test or command, **(c)** worthy of its own commit. Red flags for oversized tasks: description longer than 2 lines, task touching more than 5 files without justification, description containing "and" joining unrelated changes. When in doubt, split.

---

## Plan Persistence After Thinking (MANDATORY)

Before starting implementation: verify that the plan is persisted to a file. Plans existing only in conversation context are invalid.

### Persistence Rules

| Trigger | Save to | Format |
|---------|---------|--------|
| After producing a plan in plan mode | PLAN_FILE (`docs/PLAN.md` by default, or `docs/PLAN-{label}.md` if CLAUDE_SESSION is set) | Problem statement, options, decision + rationale, numbered steps; **must include `## Next Plans` section** listing the next 1–4 phases from `docs/ROADMAP.md` with status and one-line goals |
| After PAL tools produce strategic findings | PLAN_FILE, REVIEW_FILE, or `docs/AUDIT.md` | Key conclusions summary |
| After making an architecture decision (e.g., introduces a new library/framework, changes a public API contract, or removes a previously available option) | `docs/adr/NNNN-<title>.md` | Context, Decision, Consequences, Status |
| After completing a spike/research | `docs/spikes/YYYY-MM-DD-<topic>.md` | Question, options, recommendation, evidence |
| After a postmortem | `docs/postmortems/YYYY-MM-DD-<title>.md` | Timeline, root cause, impact, action items |

### Clean Context Gate

Before starting implementation, verify all six:
- [ ] Plan saved to `docs/` with clear execution steps.
- [ ] Each step has a checkpoint (what to verify).
- [ ] Steps are numbered and atomic (resumable from any point).
- [ ] No plan details exist ONLY in conversation -- all persisted to files.
- [ ] `## Next Plans` section present in PLAN_FILE — lists next 1–4 phases with status and goals.
- [ ] Every phase in this plan includes a **Rollback** subsection.

### Artifact Index

After creating any decision artifact (ADR, spike, postmortem, plan): update `docs/INDEX.md` with a link to the new artifact.

---

## Plan & Phase Numbering Convention

Consistent numbering prevents confusion between roadmap phases and sub-phases within implementation plans.

**Roadmap phases** (`docs/ROADMAP.md`): `Phase N` (N = integer, e.g. 1-9).
These are the canonical top-level identifiers. Never reuse them as sub-phase names inside plan files.

> **Legacy exception -- claude-team-control Phases 5A-5P:** These phases predate the `Phase N` integer rule (introduced 2026-03) and use a letter-suffix system (5A, 5B, 5B.2, 5C.1...5P). They are **frozen and immutable**. The next roadmap phase in that project is **Phase 6** (integer only, no letters). Never introduce new letter-suffix phases.

**Sub-phases within a plan file**: `Phase N.M`
- N = parent roadmap phase number (matches ROADMAP.md)
- M = sequential sub-phase index within that plan (1, 2, 3...)
- Example: Phase 9.1, Phase 9.2, Phase 9.3 are the first three sub-phases of the Phase 9 plan

**Off-roadmap plans** (tooling, infra, optimization -- no roadmap phase number):
- Format: `LABEL.M` where LABEL is a 2-5 char uppercase acronym from the plan name
- Example: `GPU.0`, `GPU.1`, `GPU.2` for a GPU optimization plan

**Tasks within a sub-phase**: `T[M].[K]`
- M = local sub-phase number (same digit as Phase N.M suffix)
- K = task sequence within that sub-phase (1, 2, 3...)
- Example: T1.1, T1.2 within Phase 9.1; T2.1, T2.2 within Phase 9.2
- Within the plan file, short form T[M].[K] is unambiguous (phase heading provides N)
- Cross-file references must use full form: `Phase 9.2 T2.3`

**Tasks within off-roadmap phases** (LABEL.M): use the same `T[M].[K]` format where M is the
numeric index of that phase. Example: T0.1, T1.2 for tasks within GPU.0 and GPU.1.

**GATE steps**: not numbered -- always the last item in a phase, written as `- [ ] GATE: ...`

**IDs are immutable**: never renumber existing phase or task assignments once created.
To insert a new phase between existing ones: add it at the end and document the logical ordering,
or leave a gap. Do NOT shift existing numbers.

**Completed / archived plans**: do NOT renumber historical plan files. Leave as written.

**Why this matters**: using Phase 1-6 inside a Phase 9 sub-plan collides with roadmap Phase 1-6,
causing ambiguity in cross-references, audit trails, and ROADMAP.md log entries.
