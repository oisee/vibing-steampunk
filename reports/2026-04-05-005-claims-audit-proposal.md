# Claims Audit — What's Real, What's Inflated, What's Unnecessary

**Date:** 2026-04-05
**Report ID:** 005
**Subject:** Honest audit of all status claims in CLAUDE.md, with proposals to keep/downgrade/remove

---

## Methodology

For each claim in CLAUDE.md status table:
1. **Is it real?** — does the code exist, do tests pass, does it work on SAP?
2. **Is the label honest?** — does "Complete" mean complete, or "works in lab"?
3. **Do we need it in the table?** — does it help Claude/users, or is it resume padding?

---

## Tier 1: Core Product — Keep, labels are honest

These are the tools users actually use daily. Claims are accurate.

| Claim | Status | Verdict | Reason |
|-------|--------|---------|--------|
| **Tools** (147/100) | ✅ | KEEP | Verifiable: count tools in register |
| **Unit Tests** (816) | ✅ | KEEP | `go test ./...` confirms |
| **Safety System** | ✅ Complete | KEEP | 25 tests, used in production configs |
| **Feature Detection** | ✅ Complete | KEEP | GetFeatures works on live SAP |
| **System Info** | ✅ Complete | KEEP | Trivial, works |
| **Code Analysis** | ✅ Complete | KEEP | GetCallGraph tested on SAP |
| **Runtime Errors** | ✅ Complete | KEEP | RABAX dumps work |
| **ABAP Profiler** | ✅ Complete | KEEP | ATRA traces work |
| **SQL Trace** | ✅ Complete | KEEP | ST05 works |
| **Transport Mgmt** | ✅ Complete | KEEP | 5 tools with safety |
| **Tool Groups** | ✅ Complete | KEEP | Config feature, trivial |
| **Class Includes** | ✅ Complete | KEEP | testclasses etc, tested |
| **Install Tools** | ✅ Complete | KEEP | InstallAbapGit verified |
| **Cache Package** | ✅ Complete | KEEP | memory + SQLite, 16 tests |

---

## Tier 2: Solid but niche — Keep, maybe consolidate

These work but are specialized. Each takes a table row for something most users won't touch.

| Claim | Status | Verdict | Proposal |
|-------|--------|---------|----------|
| **Lua Scripting** | ✅ Complete | KEEP or CONSOLIDATE | Works, but niche. Could merge into "CLI Toolchain" row |
| **DSL Package** | ✅ Complete | KEEP or CONSOLIDATE | Fluent API works. Could merge with "Batch Import/Export" |
| **Batch Import/Export** | ✅ Complete | CONSOLIDATE → merge with DSL | Same feature area |
| **Pipeline Builder** | ✅ Complete | CONSOLIDATE → merge with DSL | DeployPipeline/RAPPipeline are DSL features |
| **ExecuteABAP** | ✅ Complete | KEEP | Unique capability, users ask about it |
| **RAP OData E2E** | ✅ Complete | KEEP | Key workflow for S/4 |
| **abapGit Integration** | ✅ Complete | KEEP | Major feature |
| **Context Depth** | ✅ Complete | CONSOLIDATE → part of "Code Analysis" | It's just a param on GetContext |

**Consolidation proposal:** Merge DSL + Batch + Pipeline into one row: "DSL & Workflows — fluent API, YAML workflows, batch import/export, deploy pipelines"

---

## Tier 3: Parser/Compiler — Honest but verbose

The lexer/parser/linter chain is real and tested. But 3 separate rows for what is one `pkg/abaplint` package is excessive.

| Claim | Status | Verdict | Proposal |
|-------|--------|---------|----------|
| **Native ABAP Lexer** | ✅ Complete | CONSOLIDATE | |
| **ABAP Statement Parser** | ✅ Complete | CONSOLIDATE | |
| **ABAP Linter** | ✅ Complete | CONSOLIDATE | |

**Proposal:** One row: "Native ABAP Parser — lexer + 91 statement types + 8 lint rules, 100% oracle match (v2.32)"

---

## Tier 4: Compiler experiments — Downgrade, possibly remove from status table

These are research/prototype work. Impressive demos, not product features.

| Claim | Old Status | New Status | Verdict |
|-------|-----------|------------|---------|
| **LLVM IR→ABAP** | ~~✅ Complete~~ → ⚠️ Advanced prototype | HONEST NOW | KEEP but consider moving to "Research" section |
| **WASM Block-as-METHOD** | ~~✅ Complete~~ → ⚠️ Proven on large outputs | HONEST NOW | KEEP but consider moving to "Research" section |
| **TS→ABAP Pipeline** | ~~✅ Proven~~ → ⚠️ Experimental | HONEST NOW | KEEP but consider moving to "Research" section |
| **WASM Self-Host** | ✅ Verified | OK | 3-way proof is real, but niche |
| **TS→Go Transpiler** | ✅ Complete | QUESTIONABLE | "Produces valid Go from 3 files" — is this really a product feature? |
| **CLI Toolchain** | ✅ Complete (28 commands) | OK | Real CLI, but count includes compile/execute which are experimental |

**Proposal:** Create a separate "Research & Experiments" section in the table. Move LLVM, WASM, TS→ABAP, TS→Go there. Stop mixing production tools with compiler experiments.

---

## Tier 5: In-progress / Partial — Be explicit about gaps

| Claim | Status | Verdict | Proposal |
|-------|--------|---------|----------|
| **External Debugger** | ⚠️ HTTP unreliable | HONEST | KEEP — clearly states limitation |
| **AMDP Debugger** | ⚠️ Experimental | HONEST | KEEP — clearly states limitation |
| **UI5/BSP Mgmt** | ✅ Partial | HONEST | KEEP |
| **Graph Engine** | ⚠️ In progress | HONEST NOW | KEEP |

---

## Tier 6: Meta claims — Questionable

| Claim | Current | Question | Proposal |
|-------|---------|----------|----------|
| **Phase** = 5 (TAS-Style Debugging) - Complete | Phases are confusing | REMOVE or simplify | Phases 1-5 are not meaningful to new readers |
| **Reports** = 29 numbered + 6 reference | Nice for us, useless for users | MOVE to bottom or remove | Claude doesn't need to know report count |
| **Platforms** = 9 | Accurate | KEEP | Useful info |
| **Integration Tests** = 34 | Accurate | KEEP | Shows real SAP testing |

---

## Summary Proposal

### Status table structure

**Before (39 rows):**
Everything in one giant table, mixing production tools with compiler experiments.

**After (proposed ~25 rows, 3 sections):**

**Core Platform:**
| Feature | Status |
|---------|--------|
| Tools | 147 expert / 100 focused |
| Tests | 816 unit + 34 integration |
| Safety System | ✅ |
| Feature Detection | ✅ |
| Code Analysis | ✅ (call graph, structure, refs, context depth) |
| System Info & Diagnostics | ✅ (dumps, profiler, SQL trace) |
| Transport Management | ✅ |
| RAP OData E2E | ✅ |
| abapGit Integration | ✅ |
| Install Tools | ✅ |
| ExecuteABAP | ✅ |
| DSL & Workflows | ✅ (fluent API, YAML, batch import/export, pipelines) |
| CLI Toolchain | ✅ (28 commands) |
| Native ABAP Parser | ✅ (lexer + 91 statements + 8 lint rules) |
| Cache (memory + SQLite) | ✅ |

**In Progress:**
| Feature | Status |
|---------|--------|
| Graph Engine | ⚠️ Initial impl (boundary analysis, dynamic call detection) |
| External Debugger | ⚠️ WebSocket only (HTTP unreliable) |
| AMDP Debugger | ⚠️ Experimental |
| UI5/BSP | ⚠️ Read-only |

**Research & Experiments:**
| Feature | Status |
|---------|--------|
| LLVM IR→ABAP | ⚠️ Advanced prototype (34+28 functions, SAP verified) |
| WASM-to-ABAP | ⚠️ Proven (12K methods, QuickJS compiles) |
| TS→ABAP Pipeline | ⚠️ Demonstrated (Porffor chain) |
| TS→Go Transpiler | ⚠️ Experimental (3 files compile) |
| WASM Self-Host | ✅ Verified (3-way proof) |

### What this achieves

1. **Honest** — no "Complete" on experimental stuff
2. **Scannable** — user finds what they need in 3 sections
3. **Fewer rows** — 25 vs 39, consolidated duplicates
4. **Research separated** — compiler work is impressive but shouldn't be mixed with "tools"
5. **Claude-friendly** — AI reads this for tool selection, not for compiler history

---

## Do we need all these claims at all?

### Claims that help Claude (AI agent):
- Tool counts, safety, feature detection, code analysis, testing, DSL — YES
- These inform tool selection and capability awareness

### Claims that help users (onboarding):
- RAP, abapGit, install tools, debugger status — YES
- These answer "can I do X?"

### Claims that help nobody in CLAUDE.md:
- Report count, phase number, specific compiler version numbers
- WASM/LLVM details (Claude won't use these tools)
- Platform count (already in README)

### Recommendation:
- CLAUDE.md should focus on what helps Claude work better
- README should focus on what helps users evaluate/adopt
- Compiler experiments should live in their own section, not inflate the status table
