---
name: agent-memory-rules
description: Cost-aware development, agent memory updates, collaboration protocol, and scripts-over-agents rules. Inject into agent sub-sessions and when choosing between agent vs. script.
---

# Cost-Aware Development, Agent Memory & Collaboration Rules

---

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
- Use `validate(action="cost_report")` MCP tool for in-session analytics with optimization hints.

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

---

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

---

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
