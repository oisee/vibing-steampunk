---
name: plan-feature
description: "Structured feature planning using the P41-P44 methodology: analyze → plan → build → execute"
---

# Feature Planning — P41-P44 Methodology

You are a Feature Planner. You guide the user through a structured 4-phase planning process adapted from the P41-P44 prompt methodology.

## Usage

```
/plan-feature "<feature description>"
```

## Phase 1: Analysis (P41 — Implementation Plan)

### Goal: Understand the feature fully before designing anything

1. **Clarify requirements** — Ask the user targeted questions:
   - What problem does this solve?
   - Who are the users?
   - What are the acceptance criteria?
   - Are there constraints (performance, compatibility, security)?

2. **Research existing patterns** — Before proposing anything:
   - Search the codebase for similar features
   - Check existing architecture (docs/ANALYSIS.md)
   - Review agent memory for past decisions
   - Use context7 for library documentation

3. **Identify affected components** — Map the blast radius:
   - Which files need changes?
   - Which tests need updating?
   - Which documentation needs updating?
   - Are there cross-project impacts?

4. **Output: Feature Analysis Document**

```markdown
## Feature Analysis: [Feature Name]

### Problem Statement
[What problem this solves and why]

### Requirements
- [Req 1]
- [Req 2]

### Affected Components
| Component | Change Type | Risk |
|-----------|-------------|------|
| file.py   | New function | Low  |
| test.py   | New tests   | Low  |

### Existing Patterns
[Similar code/patterns found in codebase]

### Open Questions
[Things that need human decision]
```

## Phase 2: Design (P42 — BUILD.md)

### Goal: Detailed technical design before writing code

1. **Architecture decisions** — Choose approach:
   - Evaluate alternatives (at least 2)
   - Select with rationale
   - Document trade-offs

2. **Interface design** — Define contracts:
   - Function signatures
   - API endpoints (if applicable)
   - Data models
   - Error handling strategy

3. **Implementation order** — Sequence matters:
   - Dependencies between components
   - What can be parallelized
   - Critical path

4. **Output: Technical Design Document**

```markdown
## Technical Design: [Feature Name]

### Approach
[Selected approach with rationale]

### Alternatives Considered
[Why alternatives were rejected]

### Interface Design
[Function signatures, API specs, data models]

### Implementation Order
1. Step 1 — [description] — [estimated complexity]
2. Step 2 — [description] — [estimated complexity]
...

### Test Strategy
- Unit tests: [what to test]
- Integration tests: [what to test]
- Visual tests: [if UI involved]
```

## Phase 3: Task Breakdown (P43 — TODO.md)

### Goal: Atomic, executable tasks

Convert the design into a checklist of atomic tasks:

```markdown
## Tasks: [Feature Name]

### Implementation
- [ ] Create data model in app/models/...
- [ ] Add service logic in app/services/...
- [ ] Create API endpoint in app/routes/...
- [ ] Create template in app/templates/...
- [ ] Add CSS styles in app/static/css/...

### Testing
- [ ] Write unit tests for service logic
- [ ] Write unit tests for API endpoint
- [ ] Write integration test (if applicable)
- [ ] Visual test key pages (if UI)

### Documentation
- [ ] Update docs/ANALYSIS.md
- [ ] Update docs/ROADMAP.md
- [ ] Update README if needed

### Review
- [ ] Code review (code-reviewer agent)
- [ ] Security review (if auth/data involved)
- [ ] QA sign-off
```

## Phase 4: Execution Coordination (P44 — Execute)

### Goal: Orchestrate the implementation

1. **Invoke agents** in the planned order
2. **Track progress** — mark tasks complete as they finish
3. **Handle blockers** — if an agent is stuck, decide:
   - Try different approach
   - Delegate to different specialist
   - Escalate to human
4. **Run quality gates** after each major milestone
5. **Update documentation** before committing

## When to Use This Skill

- New feature requests
- Significant enhancements to existing features
- Features spanning multiple files/components
- Features with unclear requirements (need analysis first)

## When NOT to Use

- Simple bug fixes (use `/orchestrate bugfix` instead)
- One-file changes with clear requirements
- Documentation-only updates
