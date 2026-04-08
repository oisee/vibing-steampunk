# Docs Cleanup Report for Claude

**Date:** 2026-04-05
**Report ID:** 004
**Subject:** README / CLAUDE / CLI-agent docs contain overstatements and at least one likely incorrect Codex MCP setup path

---

## Executive Summary

There are two distinct cleanup problems:

1. **Status inflation**
   - `README.md` and `CLAUDE.md` describe several experimental or partially proven compiler tracks as if they are product-complete.
   - `Graph Engine` is described as shipped/slice-complete even though the current report trail still frames it as a design/unification effort and the worktree shows new/uncommitted graph files.

2. **Codex MCP configuration drift**
   - The docs currently tell users to configure Codex via project-local `.mcp.json`.
   - User validation in this session indicates Codex works with TOML using `mcp_servers.*`.
   - Repo docs should stop asserting the Claude-style JSON format for Codex until re-verified.

This is not a wording nit. These claims affect trust, onboarding success, and agent setup.

---

## 1. Overstatements in `CLAUDE.md`

Relevant section:
- [`CLAUDE.md:408`](/home/alice/dev/vibing-steampunk/CLAUDE.md#L408)
- [`CLAUDE.md:410`](/home/alice/dev/vibing-steampunk/CLAUDE.md#L410)
- [`CLAUDE.md:411`](/home/alice/dev/vibing-steampunk/CLAUDE.md#L411)
- [`CLAUDE.md:412`](/home/alice/dev/vibing-steampunk/CLAUDE.md#L412)
- [`CLAUDE.md:413`](/home/alice/dev/vibing-steampunk/CLAUDE.md#L413)
- [`CLAUDE.md:488`](/home/alice/dev/vibing-steampunk/CLAUDE.md#L488)

### A. `LLVM IR→ABAP` is overstated

Current claim:
- `✅ Complete (v2.33 - typed CLASS-METHODS, 34+28 functions, FatFS, SAP verified 5/5)`

Why this is too strong:
- Internal research already frames LLVM as **research / advanced prototype**, not product-complete.
- See [`reports/2026-04-01-gemini-vsp-analysis.md`](/home/alice/dev/vibing-steampunk/reports/2026-04-01-gemini-vsp-analysis.md), which labels LLVM-to-ABAP as `Advanced prototype / Research`.
- Even the stronger LLVM report still says "architecture, results, and the road ahead", not "production-ready compiler".
- "SAP verified 5/5" is evidence of partial validation, not completion.

Recommended downgrade:
- `⚠️ Advanced prototype`
- or `✅ Proven on selected corpora, not product-complete`

### B. `WASM Block-as-METHOD` and `TS→ABAP Pipeline` are framed too broadly

Current claims:
- `WASM Block-as-METHOD | ✅ Complete`
- `TS→ABAP Pipeline | ✅ Proven`

Why this needs tightening:
- These look like generally available product capabilities.
- The evidence in repo is much closer to "pipeline experiments demonstrated on selected artifacts".
- README also presents these tracks as broad cross-language capability, which compounds the overstatement.

Recommended wording:
- `WASM Block-as-METHOD | ✅ Proven on large generated outputs`
- `TS→ABAP Pipeline | ⚠️ Experimental / demonstrated path`

### C. `Graph Engine` is almost certainly premature

Current claims:
- [`CLAUDE.md:413`](/home/alice/dev/vibing-steampunk/CLAUDE.md#L413): `✅ Slice 1+2 (v2.37 - boundary analysis, dynamic call detection, offline parser, 11 tests)`
- [`CLAUDE.md:488`](/home/alice/dev/vibing-steampunk/CLAUDE.md#L488): `Phase 5 ... ✅ Slice 1+2 done`

Why this is too strong:
- The same day’s report sequence still describes graph work as design/alignment/MVP shaping:
  - [`reports/2026-04-05-002-graph-engine-design.md`](/home/alice/dev/vibing-steampunk/reports/2026-04-05-002-graph-engine-design.md)
  - [`reports/2026-04-05-003-graph-engine-alignment-for-claude.md`](/home/alice/dev/vibing-steampunk/reports/2026-04-05-003-graph-engine-alignment-for-claude.md)
- Current worktree also shows new/untracked graph files, which strongly suggests this is not yet a clean released fact.
- Repo docs elsewhere still treat graph as emerging/planned.

Recommended downgrade:
- `⚠️ In progress — initial boundary-analysis implementation underway`
- or `✅ Existing graph/deps/context pieces exist; pkg/graph unification in progress`

---

## 2. Overstatements in `README.md`

Relevant section:
- [`README.md:140`](/home/alice/dev/vibing-steampunk/README.md#L140)
- [`README.md:150`](/home/alice/dev/vibing-steampunk/README.md#L150)
- [`README.md:201`](/home/alice/dev/vibing-steampunk/README.md#L201)
- [`README.md:984`](/home/alice/dev/vibing-steampunk/README.md#L984)

### A. WASM section reads as broader than the repo can safely promise

Current pattern:
- "Run Any Language on SAP"
- "Three paths, one goal"
- "Proven on SAP A4H"

Why this needs tightening:
- The examples are impressive, but they are still selective demonstrations.
- The wording implies generic language/runtime support as a stable product feature.
- Better to separate:
  - proven large-scale experiments
  - experimental compiler tracks
  - generally supported product workflows

Recommended rewrite direction:
- Keep the achievements.
- Reduce universal claims like "Run Any Language on SAP".
- Frame it as:
  - "research/prototype compiler toolchain"
  - "verified on selected corpora and SAP experiments"

### B. `Graph Engine` is presented as a shipped feature too early

Current claims:
- [`README.md:201`](/home/alice/dev/vibing-steampunk/README.md#L201): `Graph Engine | Package boundary analysis, dynamic call detection, offline dep extraction`
- [`README.md:984`](/home/alice/dev/vibing-steampunk/README.md#L984): checked as done in future/considerations list

Why this needs tightening:
- Existing product definitely has `graph`, `deps`, and `ctxcomp` capabilities.
- But that is not the same as a finished unified `pkg/graph` engine.
- README should describe the shipped surface honestly:
  - current dependency analysis capabilities
  - graph-engine unification in progress

Recommended rewrite direction:
- Replace `Graph Engine` with something like:
  - `Dependency Analysis` — call graph, package deps, offline dep extraction
- If keeping the graph wording, append `initial` or `in progress`

---

## 3. Codex MCP Setup Docs Are Likely Wrong

Affected files:
- [`README.md:285`](/home/alice/dev/vibing-steampunk/README.md#L285)
- [`README.md:447`](/home/alice/dev/vibing-steampunk/README.md#L447)
- [`docs/cli-agents/README.md:18`](/home/alice/dev/vibing-steampunk/docs/cli-agents/README.md#L18)
- [`docs/cli-agents/README.md:181`](/home/alice/dev/vibing-steampunk/docs/cli-agents/README.md#L181)
- [`docs/cli-agents/codex.md:25`](/home/alice/dev/vibing-steampunk/docs/cli-agents/codex.md#L25)
- translated `README_RU/UA/ES` sections for Codex

Current claim:
- Codex uses project-local `.mcp.json`, same format as Claude Code

Problem:
- User validation from this session says Codex worked with TOML:

```toml
[mcp_servers.sap-adt]
command = "C:\\SOFT\\vsp.exe"
enabled = true

[mcp_servers.sap-adt.env]
SAP_URL = "http://127.0.0.1:50000"
SAP_USER = "DEVELOPER"
SAP_PASSWORD = ""
```

- That directly conflicts with current repo docs.
- OpenAI product docs also point to Codex config living in `~/.codex/config.toml` for configuration generally, which is consistent with TOML-based setup rather than project `.mcp.json`.

Practical conclusion:
- Until re-verified, repo docs should **not** claim `.mcp.json` as the Codex configuration format.

Recommended fix:
1. Change Codex rows in agent tables from `.mcp.json` to `~/.codex/config.toml` or `config.toml (verify scope)`.
2. Replace JSON examples with TOML examples using `mcp_servers.<name>`.
3. Add an explicit note:
   - `Claude Code` uses project-local `.mcp.json`
   - `Codex` uses TOML config
   - project-local vs global scope for Codex still needs explicit verification if not yet documented

### Important uncertainty to preserve honestly

The remaining question is:
- does Codex support project-local MCP config, or only global/home config?

Do **not** guess in docs.

Recommended wording if still unverified:
- `Verified: Codex works with TOML mcp_servers config.`
- `Unverified in this repo docs: whether project-local MCP config is supported or only ~/.codex/config.toml.`

---

## 4. Suggested Cleanup Edits

### In `CLAUDE.md`
- downgrade LLVM from `Complete` to `Advanced prototype` or `Proven on selected corpora`
- downgrade graph from `Slice 1+2 done` to `in progress`
- tighten TS/WASM wording to avoid implying general availability

### In `README.md`
- change `Graph Engine` feature line to `Dependency Analysis` or mark it as initial/in progress
- uncheck or reword the `Graph Engine & Boundary Analysis` completion bullet
- tone down universal compiler language

### In Codex docs
- replace `.mcp.json` with TOML-based configuration
- add a Windows example using `vsp.exe`
- clarify what is verified versus assumed
- propagate the fix to:
  - [`docs/cli-agents/README.md`](/home/alice/dev/vibing-steampunk/docs/cli-agents/README.md)
  - [`docs/cli-agents/README_RU.md`](/home/alice/dev/vibing-steampunk/docs/cli-agents/README_RU.md)
  - [`docs/cli-agents/README_UA.md`](/home/alice/dev/vibing-steampunk/docs/cli-agents/README_UA.md)
  - [`docs/cli-agents/README_ES.md`](/home/alice/dev/vibing-steampunk/docs/cli-agents/README_ES.md)
  - [`docs/cli-agents/codex.md`](/home/alice/dev/vibing-steampunk/docs/cli-agents/codex.md)
  - main [`README.md`](/home/alice/dev/vibing-steampunk/README.md)

---

## 5. Recommended Message to Claude

The docs currently overclaim maturity in a few places and likely contain an incorrect Codex MCP setup path.

Please clean up the docs with the following principles:
- distinguish `research/prototype/proven experiment` from `product-complete`
- do not mark Graph Engine as shipped unless the unified `pkg/graph` work is actually merged and released
- update Codex MCP docs from `.mcp.json` to TOML-based `mcp_servers.*` config
- if project-local Codex MCP config is not verified, say so explicitly instead of guessing

The goal is not to make the project look smaller.
The goal is to make the docs trustworthy.
