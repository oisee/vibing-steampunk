---
name: handoff
description: Generate a structured handoff document for context reset or session transfer
allowed-tools: Bash, Read, Grep
argument-hint: [<task_description>]
---

Generate a structured handoff document that captures the current session state. This enables clean context resets — a fresh agent session can pick up exactly where this one left off.

## Workflow

1. Review the current conversation context — what objects were discussed, what changes were made
2. Use **GetSource** to verify the current state of any modified objects
3. Use **RunUnitTests** on recently modified objects to capture test status
4. Produce a handoff document in this format:

```
## Handoff: [Task Name]
Date: [today]

### Current State
[Objects involved, their relationships, what exists in the system now]

### Changes Made (This Session)
[Every object created, modified, or deleted — with names and what changed]

### Remaining Work
1. [Numbered, actionable steps in dependency order]
2. [Each step should be self-contained enough for a fresh agent]

### Key Decisions
[Architecture choices and WHY — prevent the next agent from revisiting them]

### Test Status
[Pass/fail counts, specific failing tests if any]

### Critical Context
[Gotchas, prerequisites, safety config, anything a fresh agent must know]
```

5. Present the handoff document to the user
6. Suggest: "You can start a fresh session and paste this handoff to continue."

## When to Use

- Long debugging sessions where context is getting noisy
- Before switching from architecture analysis to implementation
- When handing off between team members
- When you notice the AI is losing track of earlier context

## Example Usage

```
/vsp:handoff
/vsp:handoff "Refactoring invoice processing module"
```
