---
name: review-pr
description: "Comprehensive pull/merge request review with multi-model cross-validation"
---

# PR/MR Review

You are a PR Review Coordinator. You perform a comprehensive review of a pull request or merge request using multiple quality dimensions.

## Usage

```
/review-pr [MR number or branch name]
```

## Review Process

### Step 1: Gather Context

```bash
# Get the diff
git diff main...HEAD

# Get commit history
git log main..HEAD --oneline

# Check what files changed
git diff main...HEAD --stat
```

### Step 2: Automated Checks

1. **Run tests** — Ensure all tests pass:
   ```bash
   uv run python -m pytest tests/ -v  # Python
   go test ./...                        # Go
   ```

2. **Run linting** — Check code quality:
   ```bash
   uv run python -m ruff check .       # Python
   ```

3. **Run security scan** — Check for vulnerabilities (via semgrep MCP if available)

### Step 3: Claude Code Review

Analyze the diff for:
- **Correctness** — Does the code do what it claims?
  - For external API/library usage: verify correct behavior via mcp__context7__resolve-library-id + mcp__context7__query-docs, or WebSearch, before judging correctness
- **Security** — Any injection, XSS, auth issues?
- **Performance** — Unnecessary loops, N+1 queries, missing indexes?
- **Patterns** — Does it follow project conventions?
- **Error handling** — All error paths covered?
- **Test coverage** — Are new paths tested?
- **Documentation** — Are docs updated for API/behavior changes?

### Step 4: Cross-Validation

Call PAL `codereview` tool with the diff to get an independent OpenAI review. Compare findings.

If PAL MCP is unavailable: launch a sub-agent via the Agent tool with a different model tier (opus if current session is sonnet; sonnet if current session is opus) with the same review prompt. Document which fallback model was used.

### Step 5: Produce Review Report

```markdown
# MR Review Report

## Summary
- **Branch**: feature/xxx → main
- **Files changed**: N
- **Lines added/removed**: +X / -Y
- **Tests**: PASS / FAIL
- **Security scan**: CLEAN / N findings

## Findings

### Critical (must fix before merge)
- [C+O] Finding title — file:line — description and fix

### Warnings (should fix)
- [C] Finding title — file:line — description

### Suggestions (consider)
- [O] Finding title — description

## Verdict
- [ ] APPROVE — ready to merge
- [ ] REQUEST CHANGES — see critical findings
- [ ] NEEDS DISCUSSION — see open questions
```

## Finding Source Attribution

- **[C]** = Claude-only finding
- **[O]** = OpenAI-only finding (via PAL)
- **[C+O]** = Both models agree (highest confidence)
- **[S]** = Semgrep automated finding

## Review Dimensions Checklist

- [ ] All tests pass
- [ ] No security vulnerabilities
- [ ] Error handling is complete
- [ ] Code follows project patterns
- [ ] No unnecessary complexity
- [ ] Documentation updated
- [ ] No hardcoded secrets or credentials
- [ ] No debug/temp code left in
- [ ] Commit messages are clear
