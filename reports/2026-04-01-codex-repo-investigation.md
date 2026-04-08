# Repository Investigation Report: vibing-steampunk

**Date:** 2026-04-01  
**Author:** Codex  
**Scope:** full-repo investigation of implementation, architecture, research direction, novelty, and current verification status

## Overview

`vibing-steampunk` is not a single-purpose CLI. It is a layered Go monorepo whose center of gravity is `vsp`: a CLI + MCP server + LSP + scripting runtime for SAP ABAP Development Tools (ADT). Around that product core, the repo also contains:

- an ADT client library in Go
- an MCP server with multiple tool-surface modes
- a terminal-first ABAP DevOps CLI
- a Lua scripting engine for automation and debugger orchestration
- a YAML workflow / DSL layer
- an LSP server for ABAP
- an offline ABAP lexer/linter/parser effort
- several compilation pipelines targeting ABAP
- ABAP-side helper code, especially the `ZADT_VSP` WebSocket handler
- a large design-history corpus in `docs/`, `reports/`, `articles/`, and `contexts/`

At a high level, the repo is pursuing one thesis:

> make SAP ABAP systems operable by AI agents and terminal tooling, not just by SAP GUI / Eclipse workflows.

That thesis is already implemented in a substantial way. The repo is not just speculative. It has a real binary, documented usage, embedded ABAP deployment assets, test coverage across many Go packages, and a broad surface of shipped commands and MCP tools. At the same time, it also contains active research strands that are more ambitious than the current stable product: self-hosted compilation on SAP, context compression for LLMs, TAS-style debugging, and JavaScript execution on ABAP.

## Executive Summary

### What the repo currently is

The most grounded reading of the repo is:

1. `vsp` is a Go-native bridge between AI tooling / terminal workflows and SAP ADT.
2. The bridge exposes SAP operations through three main operator interfaces:
   - MCP server for agentic tooling
   - CLI for human and CI use
   - Lua / YAML automation for repeatable workflows
3. The repo extends beyond plain REST ADT by using an ABAP-side WebSocket handler (`ZADT_VSP`) for stateful operations that ADT alone does not handle well, especially debugging, RFC-style execution, report execution, and Git-style export.
4. The repo also serves as a research vehicle for reducing token cost, making code context LLM-friendly, and compiling non-ABAP languages or IRs into executable ABAP.

### What is mature vs exploratory

**Mature / productized surfaces**

- Go ADT client library in `pkg/adt`
- MCP server in `internal/mcp`
- CLI in `cmd/vsp`
- safety model for operations/packages/transports
- config model for multi-system use
- embedded installer/dependency story for ABAP-side assets
- major parts of offline ABAP lexer/linter work
- Lua scripting and YAML workflow execution

**Exploratory / high-ambition surfaces**

- hyperfocused one-tool MCP mode as a token-economy design experiment
- deep context compression and multi-layer dependency analysis
- WASM-to-ABAP, TS-to-ABAP, LLVM-to-ABAP compilation work
- self-hosted compilation on SAP
- TAS-style debugging / recording / replay ambitions
- JS interpreter work on ABAP, documented in `contexts/2026-04-01-001-zmjs-interpreter-session.md`

### Current verification status from this checkout

I ran `go test ./...` in the repository.

**Passing packages included**

- `internal/lsp`
- `internal/mcp`
- `pkg/abaplint`
- `pkg/adt`
- `pkg/cache`
- `pkg/config`
- `pkg/ctxcomp`
- `pkg/llvm2abap`
- `pkg/scripting`
- `pkg/ts2abap`
- `pkg/wasmcomp`

**Failing areas in this checkout**

- `pkg/dsl` fails to build under test because of several `fmt.Errorf` calls with non-constant format strings
- `pkg/jseval` fails an oracle comparison test due to generated Node.js script quoting / syntax breakage in the test harness

So the repo is broadly alive, but not fully green.

## Novelty Summary

This repo has both conventional engineering and genuinely unusual ideas. The most novel approaches I found are below.

### 1. AI-first SAP surface design, not just “CLI for ADT”

Many SAP tools expose APIs. This repo goes further and designs the surface specifically for AI-agent consumption:

- multiple MCP tool modes
- a focused mode that curates the tool set
- a hyperfocused mode that collapses the entire surface into one universal `SAP(action, target, params)` tool
- safety controls that can be applied to the same AI-facing tool surface

This is not a generic transport wrapper. It is an intentional attempt to compress operational capability into an LLM-usable interface.

### 2. Context compression as a first-class compiler/problem-shaping layer

`pkg/ctxcomp` is one of the strongest ideas in the repo. The goal is not just “fetch dependencies”, but:

- detect referenced ABAP artifacts
- resolve their source/contracts
- compress them into public-facing summaries
- feed that compressed context back into read operations and LSP notifications

This is a direct answer to the LLM token-budget problem and is more interesting than ordinary code indexing.

### 3. Method-level surgery for ABAP class editing

Instead of treating a class as the atomic edit unit, the repo adds method-aware reads and writes:

- fetch method boundaries
- splice only the relevant method block
- validate and reconstruct full class source internally

This is a practical token-minimization technique with real implementation value for agentic editing.

### 4. Dual transport model: REST ADT + ABAP WebSocket handler

The repo treats standard ADT as necessary but insufficient. The answer is an ABAP-side APC/WebSocket service (`ZADT_VSP`) that provides stateful capabilities not cleanly available via REST alone:

- debugger session handling
- breakpoints
- RFC-style calls
- report execution
- Git export

This is a strong architectural move because it recognizes the transport mismatch rather than forcing everything through stateless ADT endpoints.

### 5. Compilation-to-ABAP as a strategic capability

The repo is unusually serious about ABAP as a compilation target:

- `pkg/wasmcomp`: WASM binary to ABAP
- `pkg/ts2abap`: TypeScript AST to ABAP
- `pkg/llvm2abap`: LLVM IR to typed ABAP
- `embedded/abap/wasm_compiler`: ABAP code that compiles WASM to ABAP on SAP itself

This is not common tooling. It treats ABAP as a runtime target for other ecosystems, not just a handwritten enterprise language.

### 6. TAS-style debugging / replay framing

The debugging model is framed less like “basic debug automation” and more like tool-assisted superplay:

- record frames and state
- inspect variable history
- save checkpoints
- inject state back into live sessions
- move toward replay and time-travel style workflows

The implementation is unevenly mature, but the conceptual framing is distinctive and consistent across docs and code.

## Repository Scale

Approximate counts from this checkout:

- total files excluding obvious vendor/git internals: **841**
- Go files: **184**
- ABAP files: **200**
- Markdown files: **185**
- YAML files: **6**
- Go test files: **60**

This is a substantial monorepo, not a thin wrapper.

## Repository Structure

### Core executable surfaces

- `cmd/vsp/`
  Main binary entry points and CLI subcommands.
- `cmd/abapgit-pack/`
  Secondary utility binary.

### Runtime internals

- `internal/mcp/`
  MCP server, tool registration, and tool handlers.
- `internal/lsp/`
  Minimal ABAP LSP server implementation.

### Main library packages

- `pkg/adt/`
  SAP ADT client library and workflow operations.
- `pkg/config/`
  multi-system config and tool visibility config.
- `pkg/cache/`
  in-memory and SQLite cache backends.
- `pkg/ctxcomp/`
  dependency extraction and context compression.
- `pkg/dsl/`
  fluent library + YAML workflow engine.
- `pkg/scripting/`
  Lua runtime and bindings.
- `pkg/abaplint/`
  native Go lexer/linter work inspired by abaplint.
- `pkg/jseval/`
  Go-side JS evaluation / interpreter experiments.
- `pkg/wasmcomp/`
  WASM-to-ABAP compiler.
- `pkg/ts2abap/`
  TS AST to ABAP transpiler.
- `pkg/ts2go/`
  TS to Go transpilation experiments.
- `pkg/llvm2abap/`
  LLVM IR to typed ABAP compiler.

### ABAP-side assets

- `src/`
  abapGit-style ABAP object sources for `ZADT_VSP` and helpers.
- `embedded/abap/`
  embedded ABAP sources deployed by installer flows.
- `abap/src/zadt_vsp/`
  focused ABAP-side package docs and sources.

### Documentation / design-history layers

- `README.md`, `ARCHITECTURE.md`, `VISION.md`, `ROADMAP.md`, `CHANGELOG.md`
- `docs/`
- `reports/`
- `articles/`
- `contexts/`

### Bundled / sibling material

- `abap-adt-api/`
- `abap-adt-api-lib/`

These appear to be sibling repos or copies retained in-tree for related experimentation/reference rather than the main active surface of `vsp`.

## Architectural Dissection

## 1. Entry Point Architecture

`cmd/vsp/main.go` defines `vsp` as a single binary with two primary personalities:

- default MCP server
- subcommand-driven CLI

It also loads:

- `.env`-style SAP credentials
- `.vsp.json` multi-system profiles
- `.mcp.json` integration config
- safety flags
- feature toggles
- mode selection (`focused`, `expert`, `hyperfocused`)

This is a good example of the repo’s design philosophy: one binary, multiple operator surfaces, shared underlying client.

## 2. ADT Client Layer

`pkg/adt` is the main implementation backbone. It contains:

- read operations
- CRUD operations
- syntax check / activation / test flows
- code intelligence
- transport handling
- debugger and WebSocket clients
- report execution helpers
- workflow helpers that compose multiple steps

This package is not just low-level HTTP plumbing. It contains:

- high-level workflow methods such as lock → syntax check → update → unlock → activate
- safety gating
- feature probing
- object-type routing
- file parsing/import/export helpers

This makes `pkg/adt` the real domain layer of the project.

## 3. MCP Layer

`internal/mcp` wraps `pkg/adt` into agent-facing tools.

Important design decisions:

- tools are grouped by domain
- focused mode is implemented as a whitelist
- expert mode exposes the larger surface
- hyperfocused mode replaces the entire schema with one universal router
- tool-level enable/disable config from `.vsp.json` can override the mode logic

This is one of the clearest places where the repo optimizes for AI interaction cost, not only raw functionality.

## 4. Safety Layer

The safety model in `pkg/adt/safety.go` is one of the stronger “proven” parts of the repo. It supports:

- read-only mode
- block-free-SQL mode
- operation whitelists/blacklists
- allowed package restrictions
- transport enablement and transport read-only mode
- transport whitelists
- transportable-edit protection

This matters because the project’s thesis is “let agents operate SAP systems”. Without a serious safety layer, that thesis would not be credible.

## 5. Feature-Probing Layer

`pkg/adt/features.go` introduces a “safety network” for optional SAP capabilities:

- HANA
- abapGit
- RAP
- AMDP
- UI5
- Transport

This is pragmatic. SAP landscapes vary heavily, so tool exposure cannot rely on one fixed assumption set.

## 6. WebSocket Extension Layer

The ABAP-side `ZADT_VSP` package is strategically important.

Based on the embedded docs and sources, it provides domain handlers for:

- RFC/service calls
- debugger operations
- AMDP operations
- Git export/integration
- report execution

This makes the project more than an ADT client. It becomes a hybrid system:

- Go on the client/agent side
- ABAP helper services on the SAP side

That hybrid pattern is central to the repo’s most advanced capabilities.

## Major Subsystems

## 1. CLI Toolchain

The CLI is broad and meaningful, not just an MCP debug helper. Docs and code indicate commands for:

- source read/write/edit
- search and grep
- table queries
- call graph and dependency analysis
- tests and ATC
- lint/parse
- compile/transpile
- workflows
- Lua REPL/scripts
- LSP
- export/install/deploy tasks

This gives the repo a second life beyond MCP. It can be used directly by engineers and CI systems.

## 2. LSP

`internal/lsp` is intentionally minimal, but useful:

- text sync
- diagnostics
- go-to-definition
- best-effort context push on open

This is not a full IDE-grade language server, but it fits the repo’s philosophy of agent/editor support with modest complexity.

## 3. Lua Scripting

`pkg/scripting` wraps a Lua VM with many bindings into SAP and vsp operations.

This subsystem is important because it converts one-shot tools into programmable workflows:

- search
- source access
- query
- parsing/linting
- breakpoints
- stepping
- recording
- checkpoints
- replay-style operations

This is one of the more unusual and promising parts of the repo because it bridges declarative automation, interactive scripting, and debugger orchestration.

## 4. YAML Workflows / DSL

`pkg/dsl` provides both:

- a Go builder surface
- YAML workflow execution

The actions are intentionally CI/CD-shaped:

- search
- syntax_check
- test
- activate
- print
- fail_if
- foreach

This makes sense as a pipeline layer over the ADT client. It is conceptually solid, though this checkout currently has test/build breakage in `pkg/dsl`.

## 5. Context Compression

`pkg/ctxcomp` deserves special emphasis.

It combines multiple dependency-discovery layers:

- regex
- parser-style validation
- SAP scan/token layers
- cross-reference/index layers
- where-used style layers

Its confidence model explicitly acknowledges that some SAP indexes can be stale and that parser-grounded evidence is closer to real source truth. That is a sophisticated design choice, especially for LLM-facing tooling.

## 6. Offline ABAP Understanding

`pkg/abaplint` is a native Go lexer/linter effort that aims to port core capabilities from abaplint-style logic into Go.

Why this matters:

- reduces dependence on SAP round-trips for some tasks
- enables local linting/parsing
- supports CLI/LSP workflows
- underpins the repo’s broader ambition to make ABAP toolable outside SAP IDEs

This is a strategically strong subsystem because it supports both human and AI workflows.

## 7. Compilation Pipelines

This repo has three separate compilation/transpilation strands with different tradeoffs.

### WASM → ABAP

`pkg/wasmcomp` compiles WASM modules into ABAP code. This is the most ambitious compiler path and is backed by substantial docs, tests, and ABAP-side assets.

### TS → ABAP

`pkg/ts2abap` is smaller and more direct. It uses TS AST JSON as input and emits OO-style ABAP. Compared with the WASM path, it aims for much more readable output.

### LLVM IR → ABAP

`pkg/llvm2abap` compiles textual LLVM IR to typed ABAP. This is interesting because it sits between low-level universality and higher-level readable output.

Together, these packages show that the repo is exploring ABAP not only as a development target but as a compilation backend.

## 8. JS on ABAP / Interpreter Research

The repo also contains a separate research thread around JS interpretation and ABAP-hosted execution, surfaced most clearly in:

- `pkg/jseval`
- `embedded/abap/zcl_jseval*.abap`
- `contexts/2026-04-01-001-zmjs-interpreter-session.md`

This is not just “nice to have” experimentation. It connects to the repo’s broader theme: portable execution, self-hosting, and making SAP a runtime for ecosystems not originally native to it.

## Novel vs Proven Approaches

## Proven / grounded approaches

These are backed by code, docs, and practical architecture:

- Go ADT client with real workflows
- MCP server with large tool surface
- safety gating and feature probing
- terminal-first CLI
- WebSocket extension via ABAP helper package
- Lua and YAML automation
- offline lexer/linter/parser direction

These are the repo’s strongest proven contributions.

## Novel but partially proven approaches

These are real and implemented, but still more specialized and risk-bearing:

- hyperfocused one-tool MCP mode
- context compression as LLM context payload engineering
- method-level source surgery
- stateful debugger scripting and replay ideas
- multiple compilation targets into ABAP

These are the repo’s most distinctive contributions.

## Mostly research / frontier approaches

- full self-hosting compilation on SAP as a general practice
- broad JS runtime/conformance execution on ABAP
- future time-travel / replay / extraction workflows
- swarm-style debugging concepts in `VISION.md`

These are intellectually coherent, but should be read as frontier exploration rather than the repo’s stable product contract.

## Documentation and Design History

One striking characteristic of this repo is that it preserves its thinking process in-tree.

### Strengths of that approach

- architecture decisions are inspectable
- experimental reasoning is not lost
- major changes are contextualized by reports and contexts
- the project narrative is unusually legible

### Weaknesses of that approach

- claims can drift across README, architecture docs, roadmap, changelog, and reports
- counts and maturity statements do not always stay synchronized
- the repo contains both live product docs and speculative design material, which can blur the current truth if read carelessly

Examples of drift I observed:

- tool-count numbers vary across docs
- some docs describe older focused/expert counts than the current README/changelog
- roadmap/status documents reflect prior milestones while the changelog reflects newer features

The repo would benefit from more explicit labeling of:

- current supported product surface
- experimental but shipped features
- future vision / research only

## Current Health Assessment

## What looks strong

- architecture is coherent despite breadth
- core Go packages are reasonably modular
- documentation is abundant
- the repo has real test coverage in many subsystems
- the core thesis is reflected in actual implementation, not only prose

## What looks risky

- breadth is very high; maintenance burden is substantial
- several ambitious research strands coexist with product surfaces
- docs/version counts drift
- some advanced subsystems appear fragile or environment-dependent
- `pkg/dsl` and `pkg/jseval` are currently not green in this checkout

## What the repo appears to be optimizing for

The project is not optimizing for minimal scope. It is optimizing for leverage:

- one binary
- many operator surfaces
- AI-friendly interaction design
- reduced token cost
- fewer SAP GUI / Eclipse dependencies
- more end-to-end automation

That makes the repo unusually ambitious, but also more complex than a narrowly scoped SAP utility.

## Most Important Parts of the Repo

If I had to identify the truly central parts, they would be:

1. `pkg/adt/`
   The domain backbone.
2. `internal/mcp/`
   The AI-facing product surface.
3. `cmd/vsp/`
   The operational entry point for humans and CI.
4. `embedded/abap/` and `src/`
   The SAP-side extension mechanism.
5. `pkg/ctxcomp/`
   The most strategically interesting LLM-enablement subsystem.

Secondary but still important:

- `pkg/abaplint/`
- `pkg/scripting/`
- `pkg/dsl/`
- `internal/lsp/`

High-value research wings:

- `pkg/wasmcomp/`
- `pkg/ts2abap/`
- `pkg/llvm2abap/`
- `pkg/jseval/`

## Final Assessment

`vibing-steampunk` is best understood as an AI-native SAP tooling platform with three intertwined identities:

1. a serious ADT/MCP/CLI product
2. a terminal-first ABAP DevOps environment
3. a research lab for token-efficient, agent-oriented, and compilation-oriented ABAP tooling

Its most credible and durable achievements are the Go ADT client, the MCP/CLI surface, the safety model, and the WebSocket-assisted extension architecture. Its most original work lies in context compression, method-level editing, AI-oriented tool-surface design, and compilation-to-ABAP experiments.

The repo is already more than a prototype. But it is also clearly still in motion, with some subsystems at product maturity and others at active-research maturity. Any future reader should treat it as a living system with both proven foundations and aggressive experimental ambitions.

## Appendix: Key Evidence Used

Main top-level docs:

- `README.md`
- `ARCHITECTURE.md`
- `docs/architecture.md`
- `docs/cli-guide.md`
- `docs/DSL.md`
- `VISION.md`
- `ROADMAP.md`
- `CHANGELOG.md`

Core implementation surfaces:

- `cmd/vsp/`
- `internal/mcp/`
- `internal/lsp/`
- `pkg/adt/`
- `pkg/config/`
- `pkg/ctxcomp/`
- `pkg/scripting/`
- `pkg/dsl/`
- `pkg/abaplint/`
- `pkg/wasmcomp/`
- `pkg/ts2abap/`
- `pkg/llvm2abap/`
- `pkg/jseval/`

ABAP-side evidence:

- `embedded/abap/README.md`
- `embedded/abap/`
- `src/`
- `abap/src/zadt_vsp/`

Recent strategic context:

- `reports/2026-03-18-001-project-status-comprehensive.md`
- `reports/2026-03-20-001-wasm-abap-achievement.md`
- `reports/2026-03-29-001-llvm-abap-compilation-research.md`
- `contexts/2026-04-01-001-zmjs-interpreter-session.md`

Verification performed during this investigation:

- repository structure inventory
- package/file inventory
- `go test ./...` execution in this checkout
