# Claude Tool Search Tool Integration Analysis

**Date:** 2026-02-03
**Report ID:** 001
**Subject:** Leveraging Claude's Tool Search Tool for Dynamic Tool Discovery in vsp
**Status:** Research Complete - Future Implementation
**Related Documents:** [Claude Platform Docs](https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-search-tool)

---

## Executive Summary

Claude's **Tool Search Tool** (beta) enables dynamic tool discovery via `defer_loading`, allowing Claude to search and load tools on-demand rather than having all definitions in context upfront. This feature directly addresses vsp's scaling challenge: 122 tools in expert mode far exceeds Claude's optimal threshold of 30-50 tools, where selection accuracy begins to degrade.

**Key Benefits:**
- **Context reduction:** ~20K tokens → ~3K tokens (85% savings)
- **Improved accuracy:** Claude searches for tools rather than selecting from 122 options
- **Simplified modes:** Expert mode becomes safe when tools load on-demand

---

## The Problem

### Current vsp Tool Landscape

| Mode | Tools | Context Tokens | Accuracy Impact |
|------|-------|----------------|-----------------|
| Focused | 54 | ~10K | Above threshold |
| Expert | 122 | ~20K | Significantly degraded |

Claude's tool selection accuracy degrades significantly beyond 30-50 tools. Even focused mode (54 tools) exceeds this threshold. Expert mode (122 tools) consumes massive context and risks poor tool selection.

### Current Mitigations

vsp already implements several strategies:
1. **Mode system:** Focused (54) vs Expert (122) tool visibility
2. **Tool groups:** `--disabled-groups` for category-based filtering (UI5, Debug, HANA, etc.)
3. **Per-tool config:** `.vsp.json` for granular enable/disable

These help but don't solve the fundamental problem: even 54 tools is too many.

---

## The Solution: Tool Search Tool

### How It Works

1. Client includes a tool search tool (`regex` or `bm25` variant) in the tools array
2. Tools marked with `defer_loading: true` are hidden from Claude initially
3. Claude sees only the search tool + non-deferred core tools
4. When Claude needs additional tools, it searches (regex patterns or natural language)
5. API returns 3-5 matching `tool_reference` blocks
6. References auto-expand to full tool definitions
7. Claude invokes the discovered tool

### Two Search Variants

| Variant | Query Type | Example |
|---------|------------|---------|
| `tool_search_tool_regex_20251119` | Python regex | `"debug.*step"`, `"(?i)transport"` |
| `tool_search_tool_bm25_20251119` | Natural language | "step through debugger code" |

### Beta Requirements

```
Header: anthropic-beta: advanced-tool-use-2025-11-20
Models: Claude Opus 4.5, Claude Sonnet 4.5 only
```

---

## Integration Architecture

### Data Flow

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────┐
│  Claude API     │────▶│  MCP Connector   │────▶│  vsp Server │
│  + Tool Search  │     │  (mcp_toolset)   │     │  (122 tools)│
└─────────────────┘     └──────────────────┘     └─────────────┘
        │                        │
        │  defer_loading: true   │
        │  for ~114 tools        │
        ▼                        ▼
   Only 8 core tools       Full definitions
   loaded initially        fetched on-demand
```

### API Usage Example

```python
response = client.beta.messages.create(
    model="claude-sonnet-4-5-20250929",
    betas=["advanced-tool-use-2025-11-20", "mcp-client-2025-11-20"],
    mcp_servers=[{
        "type": "url",
        "name": "vsp",
        "url": "http://localhost:8080"
    }],
    tools=[
        {"type": "tool_search_tool_bm25_20251119", "name": "tool_search"},
        {
            "type": "mcp_toolset",
            "mcp_server_name": "vsp",
            "default_config": {"defer_loading": True},
            "configs": {
                # Core tools always loaded
                "GetSource": {"defer_loading": False},
                "WriteSource": {"defer_loading": False},
                "GrepObjects": {"defer_loading": False},
                "SyntaxCheck": {"defer_loading": False},
                "Activate": {"defer_loading": False},
            }
        }
    ],
    messages=[...]
)
```

---

## Proposed Tool Tiers

### Tier 1: Always Load (8 tools)

Core workflow tools used in nearly every session:

| Tool | Purpose |
|------|---------|
| `GetSource` | Read any ABAP object source |
| `WriteSource` | Write any ABAP object source |
| `GrepObjects` | Search code across objects |
| `GrepPackages` | Search within packages |
| `SearchObject` | Find objects by name |
| `SyntaxCheck` | Validate ABAP syntax |
| `Activate` | Activate objects |
| `FindDefinition` | Navigate to definitions |

### Tier 2: Defer Load (114 tools)

Specialized tools loaded via search:

| Category | Tools | Example Triggers |
|----------|-------|------------------|
| Debugger | 12 | "debug", "breakpoint", "step" |
| Transport | 5 | "transport", "request", "release" |
| UI5/BSP | 7 | "ui5", "fiori", "bsp" |
| AMDP/HANA | 7 | "amdp", "hana", "sqlscript" |
| Git/abapGit | 2 | "git", "export", "abapgit" |
| Analysis | 10 | "call graph", "trace", "profile" |
| Reports | 6 | "run report", "variant" |
| Install | 4 | "install", "setup", "dependencies" |
| CRUD | 15+ | "create", "delete", "lock" |
| ... | ... | ... |

---

## Implementation Roadmap

### Phase 1: Documentation (Low Effort)
- Document how to use vsp with Tool Search via Claude API
- Provide example configurations for common workflows
- No code changes required

### Phase 2: Tool Metadata Enhancement (Medium Effort)
Add category/tier metadata to tool registration:

```go
s.mcpServer.AddTool(mcp.NewTool("DebuggerStep",
    mcp.WithDescription("Step through debugger..."),
    mcp.WithCategory("debugger"),      // NEW
    mcp.WithTier(2),                   // NEW
), s.handleDebuggerStep)
```

Benefits:
- Self-documenting tool hierarchy
- Easier client configuration
- Enables automated tier assignment

### Phase 3: .vsp.json Schema Update (Low Effort)
Add tier/defer configuration:

```json
{
  "tools": {
    "defaults": {
      "deferTier2": true
    },
    "GetSource": {"tier": 1},
    "DebuggerStep": {"tier": 2}
  }
}
```

### Phase 4: Claude Code Integration (External Dependency)
Wait for Claude Code to support `defer_loading` for MCP servers:

```json
{
  "mcpServers": {
    "vsp": {
      "command": "./vsp",
      "toolSearch": {
        "enabled": true,
        "variant": "bm25",
        "deferByDefault": true,
        "alwaysLoad": ["GetSource", "WriteSource"]
      }
    }
  }
}
```

---

## Impact Analysis

### Before Tool Search

| Metric | Focused Mode | Expert Mode |
|--------|--------------|-------------|
| Tools in context | 54 | 122 |
| Context tokens | ~10K | ~20K |
| Tool selection | Degraded | Poor |
| Mode switching | Required | N/A |

### After Tool Search

| Metric | Expert Mode + Tool Search |
|--------|---------------------------|
| Tools in context | 8 (core) + on-demand |
| Context tokens | ~3K base |
| Tool selection | High accuracy |
| Mode switching | Not needed |

**Context savings:** 85% reduction (20K → 3K base)

---

## Technical Constraints

### API Limitations
- **Max tools:** 10,000 in catalog (vsp: 122 - well within)
- **Search results:** Returns 3-5 tools per search
- **Regex max length:** 200 characters
- **Model support:** Opus 4.5, Sonnet 4.5 only (no Haiku)

### Current Blockers
- Claude Code doesn't yet support `defer_loading` for MCP servers
- Direct API usage requires manual configuration
- Beta feature - API may change

---

## Recommendations

1. **Immediate:** Document the integration pattern for API users
2. **Short-term:** Define official tier assignments for all 122 tools
3. **Medium-term:** Add tool metadata (category, tier) to registration code
4. **Long-term:** Advocate for Claude Code MCP + defer_loading support

---

## Conclusion

The Tool Search Tool is a natural fit for vsp's architecture. The existing mode/group system provides a foundation for tier assignments. Implementation can proceed incrementally from documentation to full metadata integration.

With Tool Search, vsp's "expert mode" transforms from a power-user risk to the sensible default - Claude discovers exactly what it needs, when it needs it.

---

## References

- [Claude Platform: Tool Search Tool](https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-search-tool)
- [Claude Platform: MCP Connector](https://platform.claude.com/docs/en/agents-and-tools/mcp-connector)
- vsp internal: `internal/mcp/server.go` (tool registration)
- vsp internal: `pkg/config/systems.go` (.vsp.json schema)
