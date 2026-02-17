# One Tool Mode: Future Development Options

**Date:** 2026-02-01
**Report ID:** 001
**Subject:** Token Economy Mode - Architecture & Roadmap
**Branch:** `one-tool-mode`
**Credits:** Filipp Gnilyak (@vitalratel)

---

## Overview

VSP currently exposes 95+ individual MCP tools. For users with limited token budgets (non-Max subscriptions), this creates significant context overhead (~20-40K tokens for tool definitions alone).

The `one-tool-mode` branch preserves an alternative architecture: a single universal "SAP" tool that routes to all operations internally.

---

## Architecture Comparison

```
┌────────────────────────────────────────────────────────────────┐
│  Standard Mode (default)          │  Token Economy Mode        │
├───────────────────────────────────┼────────────────────────────┤
│  95 individual tools              │  1 universal "SAP" tool    │
│  ~20-40K tokens for definitions   │  ~150 tokens               │
│  LLM knows all capabilities       │  LLM queries help on-demand│
│  Direct tool calls                │  Action-based routing      │
└───────────────────────────────────┴────────────────────────────┘
```

### Example Calls

| Standard Mode | Token Economy Mode |
|---------------|-------------------|
| `GetSource(object_type="CLAS", name="ZCL_TEST")` | `SAP(action="read", target="CLAS ZCL_TEST")` |
| `WriteSource(object_type="PROG", name="ZTEST", source="...")` | `SAP(action="edit", target="PROG ZTEST", params={source: "..."})` |
| `SearchObject(query="ZCL_*")` | `SAP(action="search", target="ZCL_*")` |
| `RunUnitTests(object_url="...")` | `SAP(action="test", params={object_url: "..."})` |

---

## On-Demand Help System

Since tool definitions are minimal, the universal tool provides built-in documentation:

```
SAP(action="help")           → Overview of all actions
SAP(action="help", target="read")   → Detailed read documentation
SAP(action="help", target="debug")  → Debugging workflows
SAP(action="help", target="ABAP SELECT")  → ABAP keyword help
```

This shifts documentation cost from upfront (tool definitions) to on-demand (when needed).

---

## Trade-offs

| Aspect | Standard Mode | Token Economy Mode |
|--------|---------------|-------------------|
| Initial token cost | High (~20-40K) | Low (~150) |
| LLM learning curve | None (knows tools) | Must query help |
| Error potential | Low (typed params) | Higher (string parsing) |
| Debugging | Clear tool names | Generic "SAP" calls |
| Best for | Max subscribers, complex workflows | Limited budgets, simple tasks |

---

## Future: Unified Mode

Proposed CLI flag to let users choose:

```bash
vsp --tool-mode granular   # 95 tools (default)
vsp --tool-mode universal  # 1 tool (token economy)
```

### Implementation Plan

1. Keep both registration methods in server.go
2. Add `--tool-mode` / `SAP_TOOL_MODE` config
3. Conditional tool registration based on mode
4. Shared handlers (both modes use same backend)
5. Preserve pkg/cache (not deleted like in current one-tool-mode)

### Files to Cherry-pick from one-tool-mode

```
internal/mcp/handlers_universal.go  # Universal tool routing
internal/mcp/handlers_help.go       # On-demand documentation
internal/mcp/helpers.go             # Shared utilities
```

### Files to NOT merge

```
pkg/cache/*                         # Keep caching (deleted in one-tool-mode)
pkg/adt/workflows.go                # Keep workflows (deleted in one-tool-mode)
```

---

## Current Status

| Item | Status |
|------|--------|
| Branch `one-tool-mode` | ✅ Created, preserved |
| PR #16, #8 | ✅ Closed (work preserved in branch) |
| Unified mode implementation | 🔮 Future |
| Documentation | ✅ This report |

---

## Related

- PR #16: Universal SAP tool + refactoring (closed, preserved)
- PR #8: Refactor devtools.go (closed, included in #16)
- Branch: `one-tool-mode` on GitHub
