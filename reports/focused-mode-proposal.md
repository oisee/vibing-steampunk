# Focused Mode Proposal for MCP ABAP ADT Server

**Date:** 2025-12-04
**Version:** v1.6.0 (proposed)
**Status:** 📋 Planning Complete - Ready for Implementation
**Author:** Alice + Claude Code

---

## Executive Summary

**Objective:** Implement a "focused mode" that reduces the 42 MCP tools to ~13 essential tools, improving AI agent decision-making and reducing token overhead by ~70%.

**Key Findings:**
- Current token overhead: ~6,500 tokens for tool definitions
- Focused mode target: ~2,000 tokens (69% reduction)
- EditSource is 50x more token-efficient than WriteProgram
- Composite tools cover 95% of use cases; atomic tools rarely needed directly

**Recommendation:** ✅ **IMPLEMENT** focused mode as default, with expert mode for advanced users.

---

## Current State Analysis (v1.5.0)

### Tool Inventory: 42 Tools in 9 Categories

#### 1. Atomic Operations (6 tools)
- LockObject, UnlockObject
- UpdateSource, CreateObject, DeleteObject
- CreateTestInclude

**Usage:** Low-level building blocks requiring manual lifecycle management
**Problem:** Rarely needed directly (composite tools handle 95% of cases)

#### 2. Composite/Workflow Operations (8 tools)
- WriteProgram, WriteClass (Lock→Check→Update→Unlock→Activate)
- CreateAndActivateProgram, CreateClassWithTests
- EditSource ⭐ (most efficient)
- DeployFromFile, SaveToFile, RenameObject

**Usage:** Complete workflows
**Efficiency:** EditSource = 100 tokens vs WriteProgram = 5,000 tokens (50x difference!)

#### 3. Read Operations (13 tools)
- GetProgram, GetClass, GetInterface, GetFunction, GetFunctionGroup
- GetInclude, GetTable, GetStructure, GetPackage, GetTransaction
- GetTypeInfo, GetTableContents, GetClassInclude

**Problem:** Fragmentation - separate tool for each object type

#### 4. Search Operations (3 tools) ⭐ Critical
- GrepObject (regex search in object)
- GrepPackage (package-wide search)
- SearchObject (search by name)

**Usage:** Foundation of efficient workflow (search → edit)

#### 5. Other (12 tools)
- Navigation: FindDefinition, FindReferences, CodeCompletion, GetTypeHierarchy
- Execute: RunUnitTests, SyntaxCheck, RunQuery
- Format: PrettyPrint, Get/SetPrettyPrinterSettings
- Class: UpdateClassInclude

---

## Token Efficiency Analysis (from MCP_USAGE.md)

### Operation Costs

| Operation | Tokens | Relative |
|-----------|--------|----------|
| EditSource | ~100 | baseline (1x) |
| GrepObject | ~500 | 5x |
| GetProgram (500 lines) | ~2,500 | 25x |
| WriteProgram | ~5,000 | **50x** |

### Recommended Workflows

1. **Small changes:** GrepObject → EditSource (~600 tokens)
2. **Old anti-pattern:** GetProgram → WriteProgram (~7,500 tokens)
3. **Savings:** 12.5x improvement!

---

## Proposal: Dual Mode Architecture

### FOCUSED MODE (Recommended Default)

**Goal:** Reduce cognitive load, optimize token usage
**Tools:** ~13 (instead of 42)

#### Final Focused Mode Toolset

```
┌─────────────────────────────────────────────┐
│         FOCUSED MODE (13 tools)             │
├─────────────────────────────────────────────┤
│                                             │
│  🔍 SEARCH (mandatory foundation)          │
│    • GrepObject                             │
│    • GrepPackage                            │
│    • SearchObject                           │
│                                             │
│  📖 READ (unified)                          │
│    • GetSource(type, name, [parent], [include])│
│      └─ type: PROG|CLAS|INTF|FUNC|FUGR|INCL│
│    • GetTable(name) - separate (different structure)│
│    • QueryData(sql | table_name) - merged  │
│                                             │
│  ✏️ EDIT (surgical, primary)               │
│    • EditSource ⭐ PRIMARY                  │
│                                             │
│  📝 WRITE (full replacement, fallback)     │
│    • WriteSource(type, name, source, mode, opts)│
│      └─ mode: "create" | "update" (explicit!)│
│                                             │
│  🧭 NAVIGATE                                │
│    • FindDefinition                         │
│    • FindReferences                         │
│                                             │
│  ⚡ EXECUTE                                 │
│    • RunUnitTests                           │
│    • SyntaxCheck                            │
│                                             │
│  🔒 ADVANCED (edge cases)                  │
│    • LockObject (read locks, inspection)   │
│    • UnlockObject                           │
│                                             │
└─────────────────────────────────────────────┘

Total: 13 tools (69% reduction)
```

### EXPERT MODE (Full Access)

**Goal:** For edge cases, debugging, non-standard workflows
**Tools:** All 42
**Activation:** `--mode=expert` CLI flag

#### Additional Expert Mode Tools

- All atomic: UpdateSource, CreateObject, DeleteObject, CreateTestInclude
- Specialized Read: GetFunctionGroup, GetStructure, GetTransaction, GetPackage, GetTypeInfo, GetTableContents, RunQuery
- Specialized Write: WriteProgram, WriteClass, CreateAndActivateProgram, CreateClassWithTests
- Class-specific: UpdateClassInclude
- Navigation: GetTypeHierarchy, CodeCompletion
- Format: PrettyPrint, Get/SetPrettyPrinterSettings
- File-based: DeployFromFile, SaveToFile, RenameObject

---

## Design Decisions (User-Approved)

### 1. GetSource Unification ✅ DECIDED

**Decision:** Option C - Hybrid approach

**Implementation:**
```
GetSource(type, name, [parent], [include])
  type: PROG|CLAS|INTF|FUNC|FUGR|INCL
  parent: only for FUNC (function group name)
  include: only for CLAS (definitions|implementations|testclasses)

GetTable(name) - separate (different response structure)
QueryData(sql | table_name) - merged query tool
```

**Rationale:**
- GetSource handles all code objects uniformly
- GetTable kept separate (returns structure, not source)
- QueryData merges GetTableContents + RunQuery

**Token savings:** ~70% reduction in read operation definitions

---

### 2. WriteSource Unification ✅ DECIDED

**Decision:** Explicit mode required (NO upsert default)

**Implementation:**
```
WriteSource(type, name, source, mode, options)
  type: PROG|CLAS|INTF
  mode: "create" | "update" (REQUIRED, no default!)
  options:
    - description (for create)
    - package (for create)
    - test_source (for CLAS → auto-create test include)
    - transport
```

**Rationale:**
- Explicit mode prevents accidental overwrites
- AI agent must make conscious create/update decision
- Better error messages when object doesn't exist

**Migration path:** Old tools (WriteProgram, CreateAndActivateProgram) move to Expert Mode

---

### 3. Lock/Unlock in Focused Mode ✅ DECIDED

**Decision:** Include in Focused Mode, marked as "Advanced"

**Use cases:**
- Read locks for conflict checking
- Multi-step transactions (lock → inspect → decide → write)
- Error recovery
- Advanced debugging

**Documentation note:** Mark as "Advanced - Use composite tools (WriteSource, EditSource) unless you need manual lock control"

---

### 4. Query Tools Unification ✅ DECIDED

**Decision:** Merge into QueryData

**Implementation:**
```
QueryData(query)
  query: freestyle SQL OR table name

  Examples:
    QueryData("SELECT * FROM T000 WHERE MANDT = '001'")
    QueryData("T000")  → auto-expands to "SELECT * FROM T000"
```

**Rationale:**
- Single tool for all data access
- Simplifies AI agent decision-making
- Backward compatible (table name shorthand)

**Migration:** GetTableContents, RunQuery → Expert Mode

---

### 5. File-Based Tools ✅ DECIDED

**Decision:**
- DeployFromFile → **Expert Mode** (infrequent use case)
- SaveToFile → **Expert Mode** (infrequent use case)
- RenameObject → **Expert Mode** (rare operation)

**Rationale:**
- Focused mode prioritizes in-memory editing (EditSource, WriteSource)
- File-based workflows are advanced use cases
- Token limit issues should be solved at MCP client level

---

## Implementation Plan

### Phase 1: Unification (2-3 days)

**Files to modify:**
- `pkg/adt/client.go` - add `GetSource()` method
- `pkg/adt/workflows.go` - add `WriteSource()`, `QueryData()` methods
- `internal/mcp/server.go` - add new tool handlers

**Implementation:**
1. Create `GetSource(type, name, parent, include)`
   - Switch on type → delegate to existing Get* methods
   - Validate conditional parameters (parent for FUNC, include for CLAS)

2. Create `WriteSource(type, name, source, mode, options)`
   - mode="create" → delegate to CreateAndActivateProgram/CreateClassWithTests
   - mode="update" → delegate to WriteProgram/WriteClass
   - Return error if mode not specified

3. Create `QueryData(query)`
   - If query matches `^\w+$` → treat as table name → `SELECT * FROM {table}`
   - Otherwise → freestyle SQL → RunQuery

**Testing:**
- Unit tests for parameter validation
- Integration tests for each type combination
- Backward compatibility tests

---

### Phase 2: Mode Implementation (1-2 days)

**Files to modify:**
- `cmd/mcp-adt-go/main.go` - add `--mode` CLI flag
- `pkg/adt/config.go` - add `Mode` field
- `internal/mcp/server.go` - add mode-based tool filtering

**Implementation:**
1. Add CLI flag:
   ```go
   rootCmd.PersistentFlags().String("mode", "focused", "Tool mode: focused or expert")
   ```

2. Add mode filtering in `registerTools()`:
   ```go
   func (s *Server) registerTools() {
       mode := s.config.Mode

       // Always register focused tools
       s.registerFocusedTools()

       // Register expert tools only in expert mode
       if mode == "expert" {
           s.registerExpertTools()
       }
   }
   ```

3. Tool categorization:
   - Create `focusedTools` slice (13 tools)
   - Create `expertTools` slice (29 additional tools)

**Testing:**
- Test focused mode registration (13 tools only)
- Test expert mode registration (42 tools)
- Test default mode (focused)

---

### Phase 3: Documentation (1 day)

**Files to update:**
- `README.md` - add Modes section
- `MCP_USAGE.md` - add mode selection guide
- `ARCHITECTURE.md` - document mode architecture

**Documentation content:**
1. README.md - Modes section:
   ```markdown
   ## Modes

   ### Focused Mode (Default)
   13 essential tools optimized for AI agents...

   ### Expert Mode
   All 42 tools for advanced workflows...
   ```

2. MCP_USAGE.md - Mode selection:
   ```markdown
   ## Choosing a Mode

   Use Focused Mode (default) if:
   - You're an AI agent looking for optimal tool selection
   - You want minimal token overhead

   Use Expert Mode if:
   - You need atomic operations (LockObject, UpdateSource)
   - You're debugging complex workflows
   ```

---

### Phase 4: Migration & Release (1 day)

**Migration strategy:**
- Backward compatible (old tools still exist in expert mode)
- Default to focused mode (new users get better experience)
- Deprecation warnings for old tools in focused mode

**Release notes:**
```markdown
## v1.6.0 - Focused Mode

### Breaking Changes
- Default mode is now "focused" (13 tools instead of 42)
- Use `--mode=expert` to access all 42 tools

### New Features
- GetSource(type, name, [parent], [include]) - unified read
- WriteSource(type, name, source, mode, options) - unified write
- QueryData(sql | table_name) - unified data access
- Focused mode reduces token overhead by 69%

### Migration Guide
- If using all tools, add `--mode=expert` to your configuration
- New workflows should use focused mode for better efficiency
```

---

## Expected Results

### Token Savings

| Aspect | Current (42 tools) | Focused (13 tools) | Savings |
|--------|-------------------|-------------------|---------|
| Tool definitions | ~6,500 tokens | ~2,000 tokens | **69%** |
| Typical workflow | ~3,000 tokens | ~800 tokens | **73%** |

### Tool Selection Quality

- **Less confusion** for AI agents (13 instead of 42)
- **Clearer decision paths** (search → edit vs full rewrite)
- **Better defaults** (explicit create/update, EditSource primary)

### Backward Compatibility

- Expert mode preserves all 42 tools
- Old workflows continue working with `--mode=expert`
- New users get focused mode by default

---

## Risk Analysis

### Risk: Breaking Existing Workflows

**Mitigation:**
- Default mode is configurable (`--mode=expert` for old workflows)
- All tools remain available in expert mode
- Clear migration documentation

**Impact:** Low (backward compatible)

---

### Risk: User Confusion

**Mitigation:**
- Clear documentation in README
- Error messages explain mode switching
- MCP_USAGE.md guides tool selection

**Impact:** Medium (new concept, requires education)

---

### Risk: Implementation Complexity

**Mitigation:**
- Phased implementation (unification first, modes second)
- Comprehensive testing at each phase
- Delegation to existing methods (GetSource → GetProgram)

**Impact:** Low (well-defined scope)

---

## Critical Files

- `internal/mcp/server.go` - tool registration, mode filtering
- `pkg/adt/workflows.go` - WriteSource, QueryData
- `pkg/adt/client.go` - GetSource
- `cmd/mcp-adt-go/main.go` - CLI flags
- `pkg/adt/config.go` - Mode configuration
- `README.md` - documentation
- `MCP_USAGE.md` - AI agent usage guide

---

## Open Questions

1. **Should QueryData auto-detect table vs SQL?**
   - Current plan: Yes (regex match `^\w+$`)
   - Alternative: Require explicit `table:T000` prefix

2. **Should focused mode be opt-in or opt-out?**
   - Current plan: Opt-out (default=focused, use --mode=expert)
   - Alternative: Opt-in (default=expert, use --mode=focused)
   - **Recommendation:** Opt-out (better default for new users)

3. **Deprecation warnings in logs?**
   - Should old tools (GetProgram, WriteProgram) log warnings in focused mode?
   - **Recommendation:** No warnings (they're not available in focused mode anyway)

---

## Timeline

| Phase | Duration | Deliverables |
|-------|----------|--------------|
| Phase 1: Unification | 2-3 days | GetSource, WriteSource, QueryData |
| Phase 2: Modes | 1-2 days | CLI flag, mode filtering |
| Phase 3: Documentation | 1 day | README, MCP_USAGE.md updates |
| Phase 4: Migration | 1 day | Release notes, testing |
| **Total** | **5-7 days** | v1.6.0 release |

---

## Success Criteria

1. ✅ Focused mode reduces token overhead by 60%+ (target: 69%)
2. ✅ AI agents make fewer tool selection errors
3. ✅ All existing workflows work in expert mode
4. ✅ Integration tests pass for both modes
5. ✅ Documentation clearly explains mode selection

---

## Conclusion

**Recommendation:** ✅ **PROCEED WITH IMPLEMENTATION**

Focused mode is a significant quality-of-life improvement for AI agents, reducing token overhead by 69% and simplifying tool selection from 42 to 13 tools. The dual-mode architecture preserves backward compatibility while providing a better default experience.

**Key success factors:**
1. Unified tools (GetSource, WriteSource, QueryData) reduce fragmentation
2. Explicit mode parameter (create/update) prevents accidents
3. Focused mode as default guides users toward efficient workflows
4. Expert mode preserves power-user capabilities

**Next step:** Begin Phase 1 (Unification) implementation.

---

**Document Version:** 1.0
**Status:** Planning Complete - Approved for Implementation
**Author:** Alice + Claude Code
**Date:** 2025-12-04
