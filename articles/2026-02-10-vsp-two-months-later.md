# Article Plan: VSP - Two Months Later

**Date:** 2026-02-10
**Type:** LinkedIn Article (Follow-up to Dec 17, 2025 article)
**Working Title:** "Agentic ABAP: Two Months Later - What Works, What Doesn't, and What I Learned"

---

## 1. Article Objectives

- Honest progress update for the community
- Acknowledge what works vs. what's still challenging
- Celebrate community contributions
- Provide real-world usage insights
- Set realistic expectations

---

## 2. Factual Ground (Verified Data)

### Repository Statistics
| Metric | Dec 17, 2025 | Feb 10, 2026 | Source |
|--------|--------------|--------------|--------|
| Commits | ~50 | **155+** (105 new) | `git log --oneline \| wc -l` |
| Releases | v2.12.x | **v2.26.0** (30 total) | `gh release list` |
| Tools | ~45 | **99** (52 focused, 99 expert) | README.md |
| Lines of Code | ~20K | **36,341** | `wc -l pkg/adt/*.go ...` |
| Unit Tests | ~150 | **244** | CLAUDE.md |
| Integration Tests | ~20 | **34** | CLAUDE.md |
| Contributors | 1 | **4** | GitHub |

### Community Contributors (PRs Merged)
| Contributor | PRs | Contributions |
|-------------|-----|---------------|
| @vitalratel (Filipp Gnilyak) | #3, #4, #6 | CLI mode, RunReport background jobs, MoveObject, WebSocket refactor |
| @kts982 (Kostas T.) | #14 | Transport API fix, EditSource transport support |
| @ingenium-it-engineering | #20 | Package validation fix for `$` in local packages |

### Release Timeline (Dec 17 → Feb 10)
```
v2.12.6  (Dec 10) - EditSource Class Include Support
v2.13.0  (Dec 21) - Call Graph & RCA Tools
v2.14.0  (Dec 21) - Lua Scripting Integration
v2.15.0  (Dec 21) - TAS-Style Debugging Vision
v2.16.0  (Dec 23) - abapGit WebSocket Integration
v2.17.1  (Dec 24) - Install Tools (InstallZADTVSP, InstallAbapGit)
v2.18.0  (Jan 02) - Report Execution Tools
v2.19.0  (Jan 05) - Async & Developer Productivity
v2.20.0  (Jan 06) - CLI Mode & Multi-System Management
v2.21.0  (Jan 06) - Method-Level Source Operations
v2.22.0  (Feb 01) - Transport Fixes & Proxy Support
v2.23.0  (Feb 02) - GitExport to disk, GetAbapHelp
v2.24.0  (Feb 03) - Transportable Edits Safety Feature
v2.25.0  (Feb 03) - CreatePackage Software Component
v2.26.0  (Feb 04) - Package Validation Fix
```

---

## 3. What Actually Works (Proven Features)

### A. Core CRUD Operations (Rock Solid)
- **GetSource** - Read any ABAP object (PROG, CLAS, INTF, FUNC, DDLS, BDEF, SRVD)
- **WriteSource** - Create/update with automatic syntax check + activation
- **EditSource** - Surgical string replacement (the "Composite Tool" from original article)
- **Method-level operations** - 95% token reduction for class method work
- **Namespace handling** - `/DMO/CL_*` → `#dmo#cl_*.clas.abap` (abapGit compatible)

### B. Analysis & RCA Tools (Working)
- **GetCallersOf / GetCalleesOf** - Call graph traversal
- **ListDumps / GetDump** - ST22 short dump analysis
- **ListTraces / GetTrace** - ATRA profiler integration
- **RunATCCheck** - Code quality analysis
- **CompareSource** - Diff between objects

### C. abapGit Integration (Working)
- **GitExport** - Export packages to ZIP (158 object types supported)
- **InstallZADTVSP** - Deploy WebSocket handler to SAP
- **InstallAbapGit** - One-command abapGit installation

### D. Developer Productivity (Working)
- **CLI Mode** - Multi-system profiles (`vsp config init`)
- **RunReport / RunReportAsync** - Execute reports with variants
- **CreateTable** - DDIC table creation from JSON
- **CloneObject** - Copy with name replacement
- **MoveObject** - Move between packages

### E. Safety & Governance (Working)
- **Transport integration** - Assign changes to transport requests
- **Package restrictions** - `--allowed-packages "Z*,$TMP"`
- **Operation filtering** - `--read-only`, `--disallowed-ops`
- **Transportable package creation** - With software component support

---

## 4. What's Harder Than Expected (Honest Assessment)

### A. Interactive Debugging (Experimental)
**The Vision (Dec 17):**
> "Claude sets a breakpoint, runs the code, inspects variables, fixes the bug"

**The Reality:**
- SAP's debugging architecture is **session-stateful**
- HTTP-based debugging is unreliable (session drops)
- WebSocket-based (ZADT_VSP) works better but still has edge cases
- AMDP/HANA debugging is even more complex (separate session manager)

**Current Status:**
- Breakpoint tools exist but are in **expert mode only**
- Recommended workflow: Use VSP for **analysis**, SAP GUI for **interactive debugging**
- Terminal ID feature added for **breakpoint sharing** between VSP and SAP GUI

**Lesson Learned:**
> "Some things are better done with the right tool. VSP excels at read-write-analyze. Interactive debugging needs more research."

### B. UI5/BSP Management (Partial)
- Read operations work (UI5ListApps, UI5GetApp, UI5GetFileContent)
- Write operations blocked - ADT Filestore is read-only
- Would need alternate API (e.g., /UI5/UI5_REPOSITORY_LOAD FM)

---

## 5. Unexpected Wins

### A. Community Adoption
- 4 external contributors in 2 months
- PRs fixing real production issues (transport API, package validation)
- Used for actual development projects (Zork, Z80 emulator, demos)

### B. Method-Level Operations
- Not planned originally, emerged from real usage
- `GetSource(class="ZCL_BIG", method="SMALL_METHOD")` returns just that method
- `EditSource(class="ZCL_BIG", method="SMALL_METHOD", old="...", new="...")`
- Dramatic reduction in tokens and context window usage

### C. abapGit WebSocket Integration
- 158 object types exportable
- Direct integration with ZADT_VSP (no separate abapGit UI needed)
- One-command deployment of the WebSocket handler

---

## 6. Real-World Usage Examples

### A. Retro Computing Projects (Public)
- **Zork ABAP** - Z-machine emulator (Dec 9)
- **6502 Emulator** - MS-BASIC on ABAP (Dec 10)
- **Z80/CP/M Emulator** - Russian Doll emulation (Dec 11)
- **Vivid Vibes Demo** - Demoscene on SAP (Dec 27)

All built using VSP for vibe coding in ABAP.

### B. Typical Agent Workflow (Observed)
```
User: "Fix the dump in ZCL_PRICING"

Claude (using VSP):
1. ListDumps(program="ZCL_PRICING") → finds RABAX
2. GetDump(id="...") → sees "CX_SY_ZERODIVIDE at line 42"
3. GetSource(class="ZCL_PRICING", method="CALCULATE") → reads code
4. EditSource(..., old="lv_result = x / y", new="lv_result = COND #( WHEN y <> 0 THEN x / y ELSE 0 )")
5. RunUnitTests(class="ZCL_PRICING") → confirms fix
```

This works. What doesn't work (yet) is live stepping through code.

---

## 7. Article Structure (Proposed)

### Title Options
1. "Agentic ABAP: Two Months Later - Wins, Losses, and Lessons"
2. "VSP at v2.26: What 105 Commits Taught Me About AI + SAP"
3. "The Honest Update: Building an AI Bridge to SAP"

### Outline
1. **Hook** - "Two months ago I announced VSP. Here's the honest update."
2. **By the Numbers** - Stats table (commits, tools, contributors)
3. **What Works Brilliantly** - CRUD, RCA, abapGit, CLI Mode
4. **What's Harder Than Expected** - Debugging reality check
5. **Community Wins** - Contributors, PRs, real usage
6. **Unexpected Discoveries** - Method-level ops, token efficiency
7. **What's Next** - Roadmap, call for contributors
8. **Conclusion** - "Perfect is the enemy of shipped"

---

## 8. Key Quotes to Include

### On Debugging Reality
> "I promised 'RCA on autopilot.' The RCA part works. The 'set breakpoint and step through' part? SAP's session architecture had other plans."

### On Community
> "The best validation isn't stars on GitHub. It's when someone submits a PR that fixes a bug they hit in production."

### On Scope
> "VSP started as a bridge. It's becoming a toolkit. 99 tools later, the question isn't 'can Claude do X in SAP?' but 'which tool should it use?'"

### On Iteration
> "v2.12 to v2.26 in two months. 14 releases. Some features landed perfectly. Some got parked. That's how you ship."

---

## 9. Sources & Links

### GitHub
- Repository: https://github.com/oisee/vibing-steampunk
- Releases: https://github.com/oisee/vibing-steampunk/releases
- Contributors: @vitalratel, @kts982, @ingenium-it-engineering

### Previous Articles
- Dec 17, 2025: "Agentic ABAP: Why I Built a Bridge for Claude Code"
- Dec 21, 2025: "RCA Autopilot Follow-up" (articles/2025-12-21-rca-autopilot-followup.md)
- Jan 15, 2026: "Deep Research inside your SAP system" (ZLLM v2)

### Demo Projects
- Zork ABAP: https://github.com/oisee/zork-abap
- Vivid Vibes: https://github.com/oisee/vivid-vibes

### Technical Reports
- reports/2025-12-05-019-amdp-session-architecture.md (debugging challenges)
- reports/2025-12-06-002-amdp-debugging-status.md (current state)

---

## 10. Call to Action

1. **Try it** - Download binary, point at your sandbox
2. **Contribute** - PRs welcome (debugging help especially!)
3. **Share** - What's your AI+SAP workflow?

---

## 11. Hashtags

#ABAP #ClaudeCode #MCP #SAP #OpenSource #AI #GoLang #ECC #S4HANA #Joule #VibeCode

---

## Notes for Writing

- **Tone**: Honest, technical, slightly self-deprecating about debugging
- **Length**: ~1500 words (similar to original)
- **Visuals**: Stats table, maybe architecture diagram update
- **Timing**: Post mid-week for visibility
