# VSP IS ONLY 5% EXPLORED!

**Or: 48 Days, 258 Commits, 147 Tools, and a JavaScript Interpreter That Runs Inside ABAP**

---

On February 18, 2026, VSP hit 100 GitHub stars.

At that point, it was already real software. Useful. Weird in a good way. Clearly alive.

But looking at where the project stands on April 7, 2026, that milestone now feels less like "we made it" and more like the point where we had barely opened the first door.

In the 48 days since then, VSP went from a strong ADT-MCP bridge into something much broader:

- a serious ABAP tool surface for AI agents
- a compiler playground with LLVM IR, WASM, and JavaScript execution experiments
- a graph and change-analysis engine
- a CLI that keeps getting closer to "ABAP DevOps from the terminal"
- a project that closed out its open PR backlog and started shipping fast, coherent release slices

If the 100-stars moment was the trailer, this period was the first real season.

---

## The Scoreboard

| Metric | 100 Stars (Feb 18) | Today (Apr 7) | Delta |
|--------|:------------------:|:-------------:|:-----:|
| **GitHub Stars** | 103 | **257** (0x101 — we overflowed into two bytes!) | +154 |
| **Forks** | 26 | **58** | +32 |
| **Commits** | 197 | **455** | +258 |
| **Releases** | v2.26.0 (44 total) | **v2.38.1** (55 total) | +11 releases |
| **MCP Tools** | 99 (54 focused) | **147** (100 focused) | +48 / +46 |
| **Unit Tests** | 270 | **821+** (15/15 packages) | +551 |
| **Contributors** | 5 | **15** | +10 |
| **Pull Requests** | 10 | **37** | +27 |
| **mcp-go** | v0.17 | **v0.47** | Streamable HTTP |

That is not cosmetic velocity. That is stabilization. And the part that matters most: not just feature count, but throughput with consolidation.

---

## The Big Shape Change

The project now has three clear fronts.

### 1. Compiler Experiments Stopped Being Toy Demos

This was one of the most surprising expansions. Over late February and March, VSP pushed hard into compilers and execution. These are not random side quests. They all push on the same question:

*How far can ABAP tooling go when you stop treating SAP as a closed IDE-only environment and start treating it like a programmable runtime?*

**LLVM IR → ABAP.** We built a transpiler that takes C code, compiles it to LLVM IR with clang, and generates ABAP. Then we pointed it at QuickJS — a complete JavaScript engine written in C. Result: **537 functions, 121,000 lines of ABAP**. It compiles on SAP.

The function dispatch alone required auto-splitting CASE statements into IF/ELSEIF chunks of 12 (ABAP has limits). Function pointers became CASE trampolines. Memory management became a flat internal table with byte-level addressing. Does it fully run? Almost. The WASI stubs need work. But the compiler is self-hosting — it can compile itself.

**pkg/jseval — JavaScript in pure Go.** A JS interpreter in about 2,200 lines. Objects, arrays, closures, classes, for...of, template literals, optional chaining, nullish coalescing, spread/rest. Why? Because we wanted to run the abapLint lexer *inside vsp* without depending on Node.js. And it works.

Then we transpiled the evaluator to ABAP. A JavaScript interpreter running inside SAP. `ZCL_JSEVAL`. It evaluates `fib(20)` in 378ms.

**WASM → ABAP.** On the `feat/wasm-abap` branch: a WebAssembly-to-ABAP compiler. 785 lines. It compiles *itself* on SAP.

This part of the project still sounds slightly absurd when you say it out loud, which is usually a good sign.

### 2. Graph Analysis Became a Product Surface

This is where the project got much more practical. The `pkg/graph` work turned into real user-facing analysis slices — commands that actually change how you inspect a package before editing it.

#### Slim V2 — Dead Code Detection

```bash
vsp slim '$ZDEV' --level methods --include-subpackages
```

Slim answers: "What code in this package is dead?" Not by guessing — by tracing reverse references through WBCROSSGT and CROSS, resolving package hierarchies through TDEVC, and classifying every method as LIVE, INTERNAL_ONLY, or DEAD.

Three levels: objects, methods, full. Hierarchical package scope with prefix fallback. Validated on real SAP with a 4-level test hierarchy.

The non-obvious gotcha that took a full day to find: **ADT freestyle SQL doesn't support `OR` with `LIKE`**. `WHERE (NAME LIKE 'A%' OR NAME LIKE 'B%')` returns HTTP 400. We had to restructure every query to use per-object lookups. The kind of thing you only learn by hitting a real system.

#### Health — Package Vital Signs

```bash
vsp health --package '$ZDEV'
vsp health --package '$ZDEV' --fast
```

Four signals: **tests** (unit test pass/fail), **ATC** (static analysis findings), **boundaries** (cross-package violations), **staleness** (last transport date from E070). Fast mode skips the expensive ones.

Health doesn't tell you what to fix. It tells you where to look. "This package has 0 test classes, 3 ATC errors, and hasn't been transported in 14 months" is a different conversation than "this package is fine."

#### API Surface — Clean Core Inventory

```bash
vsp api-surface '$ZDEV' --include-subpackages
```

Lists every standard SAP API your custom code calls and checks its release state. The Clean Core question isn't "are we clean?" — it's "how unclean are we, exactly, and where?"

#### Changelog & Changes — Transport as Change Data

This pair matters a lot.

```bash
# "What changed in this package recently?"
vsp changelog '$ZDEV' --since 20260101

# "Which transports belong to the same change request?"
vsp changes '$ZDEV' --attribute SAPNOTE
```

`changelog` aggregates E071 (object-transport mapping) with E070 (transport headers) and E07T (descriptions). Yes, AS4TEXT lives in E07T, not E070. We learned this the hard way when the command worked perfectly on local packages but crashed with HTTP 400 on the first transportable one.

`changes` goes one layer higher. It reads E070A transport attributes and groups transports that share the same attribute value. Configure `transport_attribute: "ZCR"` in `.vsp.json`, assign your custom attribute to transports via SE01, and suddenly you have CR-level correlation across your entire package landscape.

That is the moment where VSP stops being just "show me objects" or "run ADT remotely" and starts answering:

- What changed here?
- What usually changes together?
- Which requests were part of the same logical change?

That is a real analysis product.

#### AnalyzeABAPCode — Static Analysis Without Infrastructure

```bash
vsp analyze ZCL_MY_CLASS
```

13 lint rules running in pure Go. No external dependencies. No abapLint server. Empty catch blocks, broad exception catches, hardcoded credentials, magic numbers, unreachable code.

Born from community PR #89, shipped with 3 corrections to the original rules. The credential detector flagged `lv_next_token` as a hardcoded secret. The broad-exception rule flagged `CX_SY_ZERODIVIDE` as too generic. Both wrong. Both fixed before shipping. The kind of edge cases that only surface when you run rules against real codebases.

### 3. The Platform Got More Boring in the Good Way

Some of the most important work in this period is the least flashy:

**mcp-go v0.17 → v0.47.** 400+ call sites migrated. The `Arguments` field changed from `map[string]any` to `any`. Every MCP handler had to be updated. What we got: Streamable HTTP transport. `vsp --transport http --http-addr :8080` unlocks web-based clients, load balancers, and multi-user setups.

**Browser SSO authentication.** Not everyone can use basic auth. Now vsp can authenticate through the browser, which matters for enterprise environments with SSO mandates.

**Shared plumbing.** `acquire.go` extracts package scope resolution, TADIR object listing, and reverse reference collection into shared helpers. Used by slim, api-surface, changelog, and changes. The key insight: forward refs and reverse refs use genuinely different query shapes. Don't force-share what's different.

**Release pipeline.** GoReleaser v2.15 silently dropped `changelog.use: git-cliff`. Then it turned out `changelog.skip: true` isn't valid either — you need `changelog.disable: true`. Then the generated `RELEASE_NOTES.md` made git "dirty" and GoReleaser refused to release. Three bugs, one afternoon, all fixed. 9 platform binaries in under 60 seconds now.

A project only compounds if the boring parts improve too.

---

## 14 PRs, 0 Open

This deserves its own section.

There is a big difference between a repo that accumulates community work and a repo that absorbs it.

Over this stretch, VSP worked through 14 community PRs and brought the open PR count to zero.

| PR | Author | What | Outcome |
|----|--------|------|---------|
| #89 | @blicksten | AnalyzeABAPCode v2 (13 rules) | Cherry-picked with 3 fixes |
| #85 | @blicksten | CDS impact analysis | Cherry-picked with fixes |
| #84 | @blicksten | Code coverage + check results | Cherry-picked with fixes |
| #83 | @blicksten | Version history tools | Merged as-is |
| #82 | @blicksten | Refactoring tools | Closed — endpoints real, implementation wrong |
| #86 | @blicksten | Intelligence Layer | Closed — doesn't compile |
| #80 | @oklausen | Copilot instructions | Merged as-is |
| #79 | @snymanpaul | Session fix | Reimplemented (stateless default) |
| #77 | @cwbr | Browser SSO auth | Cherry-picked |
| #44 | @mv0101 | Windows quickstart | Merged as-is |
| #42 | @Prolls | i18n tools (7 tools) | Cherry-picked with fixes |
| #41 | @Prolls | gCTS tools (10 tools) | Cherry-picked with fixes |
| #38 | @danielheringers | mcp-go upgrade | Reimplemented (v0.47) |
| #37 | @kts982 | Pagination | Reimplemented |

The honest breakdown: 3 merged clean, 5 cherry-picked with fixes, 4 reimplemented from scratch, 2 closed. That's not a criticism of the contributors — it's the reality of a fast-moving codebase where the internal APIs shift between PR submission and review.

PR #82 deserves a special note. The ADT refactoring endpoints (`/sap/bc/adt/refactorings`, `/sap/bc/adt/quickfixes/evaluation`) are **real**. They exist on any modern SAP system. But the PR used wrong URLs, wrong parameters, and fabricated XML formats. The reference implementation in `abap-adt-api` tells the true story. These endpoints are on our roadmap — they're just harder than they look.

This matters for two reasons:

1. It increases throughput without fragmenting the codebase.
2. It signals that the project is not just producing experiments; it is maintaining a tool surface.

A fast-moving repo with an unmanaged PR pile is noisy. A fast-moving repo that actually lands, fixes, and integrates work is a platform.

### New Contributors Since 100 Stars

| Who | What |
|-----|------|
| **@blicksten** (5 PRs) | AnalyzeABAPCode, CDS tools, version history, code coverage |
| **@Prolls** (2 PRs) | i18n translation tools, gCTS integration |
| **@cwbr** (1 PR) | Browser SSO authentication |
| **@snymanpaul** (1 PR) | Session handling fix |
| **@oklausen** (1 PR) | Copilot documentation |
| **@danielheringers** (1 PR) | mcp-go upgrade initiative |
| **@thm-ma** | CLI source command flags |
| **@dominik-kropp** | Function module export fix |
| **@AndreaBorgia-Abo** | TADIR move reference fix |

15 contributors total. From 5 to 15 in 48 days. Germany, South Africa, Brazil, and more.

---

## Why "Only 5% Explored"?

Because that is honestly what it feels like.

By April 2026, VSP already spans ABAP source operations, MCP integration, CLI DevOps workflows, transport analysis, graph reasoning, compiler experiments, browser auth, package health, API surface discovery, and CTS-level change correlation.

Most users touch search, source read/write, and maybe ATC checks. That's about 8 tools out of 147.

The SQL freestyle query alone is a Swiss army knife that most users don't know exists:

```bash
vsp query USR02 --where "BNAME LIKE 'Z%'" --fields "BNAME, TRDAT" --top 10
```

Transport attribute correlation (`vsp changes`) landed today. Change request grouping across package boundaries — from terminal. No SE03, no SE10, no SAP GUI.

And yet most of the interesting second-order questions are still ahead:

- richer CR-level co-change graphs
- upgrade-check slices on top of API surface
- stronger impact analysis from transport metadata
- more object types
- debugger maturity (MCP → DAP → Web UI)
- deeper static + dynamic analysis fusion

The map got bigger faster than the explored territory.

That is a good problem.

---

## The Honest Assessment, Updated

### What Works Brilliantly
- **Everything from the last article**, plus it's faster and more stable
- **Analysis engines** — Slim, health, API surface. Real insights, not toy demos
- **Transport correlation** — Changelog and CR grouping. The thing SE03 should have been
- **Community absorption** — 15 people building real features, all integrated
- **Test coverage** — 821 tests across 15 packages. Things that break get caught

### What's Still Hard
- **Interactive debugging** — WebSocket works, REST returns 403 on newer SAP. ZADT_VSP helps but adds a dependency
- **UI5/BSP writes** — Still read-only. `/UI5/CL_REPOSITORY_LOAD` is the path but it's undocumented
- **AMDP debugging** — Session works, breakpoints are unreliable
- **ADT freestyle SQL** — No JOINs, no OR+LIKE, no subqueries. Every query is a creative workaround

### What Surprised Me This Time
- **The compiler experiments** — I didn't expect QuickJS to compile to ABAP. I definitely didn't expect the ABAP to almost run
- **Community PR volume** — 27 PRs in 48 days, more than one every two days
- **The analysis engines** — They were supposed to be building blocks. They turned out to be the most immediately useful features we've shipped

---

## The Real Takeaway

The headline is not "VSP added a lot of features."

The headline is:

**VSP is converging on a new shape of ABAP tooling.**

Not just an API wrapper. Not just a CLI. Not just an MCP server. Not just a compiler lab.

A stack where:

- AI agents can operate on SAP meaningfully
- humans can inspect and automate real ABAP workflows from the terminal
- transport history becomes analyzable change data
- package-level reasoning gets first-class tooling
- experimental compiler work feeds back into practical tool design

That is why "100 stars" now feels like ancient history. It was a milestone. But it was not the reveal.

---

## Closing

From v2.26.0 to v2.38.1 in 48 days, VSP did not just grow. It widened, deepened, and got sharper.

Twelve releases. Hundreds of commits. Hundreds more tests. Zero open PRs. New graph slices. New change-analysis slices. New compiler layers. A sturdier platform.

101 features. 53 fixes. 48 days. That ratio tells the story. This was not one big rewrite. It was continuous shipping — small surfaces, shipped often, validated, then reused.

So yes:

**VSP IS ONLY 5% EXPLORED.**

And that is exactly why this part is fun.

---

*P.S. While this article was being written, the star count crossed 257 — that's 0x101. We no longer fit in a single byte. Time to upgrade to `int2`.*

---

**GitHub**: [oisee/vibing-steampunk](https://github.com/oisee/vibing-steampunk)
**Stars**: 257
**Latest**: v2.38.1

*Previous articles:*
- *"Agentic ABAP: Why I Built a Bridge for Claude Code" (Dec 17, 2025)*
- *"Agentic ABAP at 100 Stars" (Feb 18, 2026)*

#ABAP #ClaudeCode #MCP #SAP #OpenSource #AI #GoLang #ECC #S4HANA #Community #DeadCode #StaticAnalysis #TransportManagement #VSP
