# VSP Strategic Deep Dive — CBA Alignment, Assumption Challenge & Codebase Review

**Date:** 2026-02-07
**Report ID:** 003
**Subject:** Critical strategic and technical review of vsp positioning for CBA
**Author:** Claude Opus 4.6 (independent analysis)
**Created By:** Claude Opus 4.6 via Claude Code
**Related Documents:** 2026-01-18-002, OPUS-STRATEGIC-RECOMMENDATIONS, 2026-02-07-001 (Codex), 2026-02-07-002 (Codex)
**Plan File:** `/Users/VincentSegami/.claude/plans/piped-bouncing-rossum.md`

---

## Relationship to Codex Reports (001, 002)

This report builds on and corrects the framing of two Codex-generated reports from the same date:

- **Report 001** (Codex): Excellent technical deep dive with precise file:line references and architecture maps. Findings on god files, debugger split, AMDP concurrency, and sensitive data are confirmed and incorporated here.
- **Report 002** (Codex): Comprehensive CBA alignment analysis with full 121-report inventory and code-verified critical paths. Strong technical work.

**Critical correction**: Neither Codex report researched CBA's actual AI posture. Both frame CBA as a generically cautious regulated bank. In reality, CBA is #4 globally in AI (Evident AI Index), runs Project Coral (autonomous agentic AI across 7,800 engineers), and has strategic partnerships with Anthropic and OpenAI. This reframes:

| Codex Assessment | This Report's Revision | Why |
|-----------------|----------------------|-----|
| Multi-agent: "WEAK" / "Ignore" | More viable at CBA specifically | Project Coral already does multi-agent |
| Confidence routing: "FAILS" | Viable with APRA constraints | CBA makes 55M AI decisions daily |
| Cultural shift: "Underestimated" | Largely achieved (non-SAP) | 7,800 engineers already use AI agents |
| /CBA/ namespace: "Missing" | Configurable via `--allowed-packages` | `safety.go:CheckPackage()` supports wildcards |
| Positioning: "Best ADT bridge" | "SAP extension for Project Coral" | CBA already has orchestration |

**Codex findings incorporated as-is** (confirmed by independent analysis):
- server.go is 2,489 LOC god file (Report 001)
- Debugger architecture split REST/WS (Report 001)
- AMDP lock shadowing concurrency bug (Report 001)
- Sensitive data committed (Reports 001, 002)
- 3 tests for MCP orchestration layer (Report 001)
- Tool count contradictions across docs (Reports 001, 002)
- Cache package unused (Reports 001, 002)
- ~35-45% end-to-end workflow coverage (Report 002)

---

## Executive Summary

**vsp is a technically impressive, feature-rich MCP server for ABAP development with genuine strategic value as the only MCP-native full-SDLC execution bridge for SAP ADT today.** Its 99 tools, safety-first design, dual-protocol architecture, and abapGit integration represent real engineering depth that competitors (ABAPilot, Joule, mcp-abap-adt) cannot match today.

**CBA is not a cautious bank waiting to adopt AI — they are #4 globally in the Evident AI Index, running Project Coral (autonomous agentic AI for code quality) across 7,800 engineers, with strategic partnerships with both Anthropic and OpenAI.** This means vsp's opportunity is not "convince CBA to adopt AI agents" but rather **"extend CBA's existing agentic infrastructure (Project Coral) to the SAP frontier."** CBA already has the governance, culture, and engineering capability. What they lack is the SAP ADT bridge.

**However, vsp is not enterprise-production-ready.** Zero audit logging (CBA needs it because they DO deploy AI changes at scale), no graceful shutdown, untested MCP orchestration layer (3 tests for 2,489 LOC), no structured logging, no rate limiting. The "production-ready" claim fails under the scrutiny of a team that already runs autonomous agents at scale.

**The biggest opportunity**: Position vsp as the SAP extension for Project Coral — CBA's existing agentic framework can orchestrate, vsp provides the SAP hands.

**The biggest risk**: CBA's AI engineering team evaluates vsp and finds it immature compared to Project Coral's standards.

---

## CBA AI Context (Critical — Reframes Everything)

Before any technical analysis, understand who CBA actually is in the AI space:

| Fact | Source |
|------|--------|
| **#4 globally** in 2025 Evident AI Index | [CommBank AI](https://www.commbank.com.au/about-us/opportunity-initiatives/policies-and-practices/artificial-intelligence.html) |
| **Chief AI Officer** appointed (Ranil Boteju, ex-Lloyds, 2,000+ person AI team) | [Newsroom Nov 2025](https://www.commbank.com.au/articles/newsroom/2025/11/ranil-boteju-chief-ai-officer.html) |
| **55 million AI decisions daily** across 2,000+ models | [CIO.inc](https://www.cio.inc/commonwealth-bank-australia-builds-ai-native-banking-a-30513) |
| **Project Coral** — autonomous agentic AI scanning codebases, creating PRs, fixing tech debt | [AI-Powered Engineering](https://www.commbank.com.au/articles/newsroom/2025/08/ai-powered-engineering.html) |
| **7,800+ engineers** already using AI agents for software delivery | [CommBank Newsroom](https://www.commbank.com.au/articles/newsroom/2025/08/ai-powered-engineering.html) |
| **OpenAI strategic partnership** — ChatGPT Enterprise, co-developed fraud models | [OpenAI Partnership](https://www.commbank.com.au/articles/newsroom/2025/08/tech-ai-partnership.html) |
| **Anthropic partnership** + Seattle Tech Hub near Anthropic/AWS | Various sources |
| **GenAI Council** with CEO involvement | [CIO.inc](https://www.cio.inc/commonwealth-bank-australia-builds-ai-native-banking-a-30513) |
| **First Australian bank** to publish AI adoption transparency report (Feb 2026) | [AI Report](https://www.commbank.com.au/articles/newsroom/2026/02/cba-approach-to-adopting-ai-report-announcement.html) |
| AI across **entire software delivery lifecycle** — planning, coding, testing, maintenance | [iTnews](https://www.itnews.com.au/news/cba-plans-to-use-ai-across-entire-software-delivery-614346) |
| **AI Factory with AWS** for accelerating GenAI innovation | [Cloud migration](https://www.commbank.com.au/articles/newsroom/2025/06/cba-ai-migration-cloud.html) |

**What this means for vsp**: CBA doesn't need to be convinced that AI agents can write code. They're already doing it. The question is: **can vsp meet CBA's existing standards for AI-powered engineering?** Project Coral already scans codebases, creates PRs, runs CI/CD, and deploys with human review. vsp needs to enable the same workflow for SAP ABAP.

---

## 1. Reports Directory Assessment

| Report | Date | Key Finding | Currency | CBA Relevance |
|--------|------|-------------|----------|---------------|
| 001 Auto Pilot Deep Dive | 2025-12-02 | ZRAY execution flow analysis | Stale | Low |
| 002 CROSS/WBCROSSGT Reference | 2025-12-02 | SAP cross-reference stats | Stale | Low |
| 003-007 Graph Architecture Reports | 2025-12-02 | Graph proposals (not implemented) | Stale | Low |
| 008 Test Intelligence Plan | 2025-12-02 | Smart test execution | Stale (not built) | Medium |
| 009 Library Architecture | 2025-12-02 | Caching strategy | Partially stale | Low |
| **010 Cache Implementation** | 2025-12-02 | Cache built but never integrated | **Contradicted** — dead code | Low |
| **011 Safety Implementation** | 2025-12-02 | Safety system complete | **Current** | **High** |
| 004 ExecuteABAP | 2025-12-05 | Code execution via unit test wrapper | Current | Medium |
| 014/017-019 Debugger Reports | 2025-12-05 | AMDP investigation, issues remain | Partially current | Low |
| 021 Project Status v2.11 | 2025-12-05 | Snapshot metrics | Superseded | None |
| 022 Future Vision | 2025-12-05 | Strategic roadmap | Aspirational | Medium |
| 024 AMDP Architecture | 2025-12-05 | Go concurrency for sessions | Current | Low |
| 001-003 abapGit Integration | 2025-12-08 | RAP OData service | Partially current | Medium |
| 001 v2.12.0 Release | 2025-12-07 | Class includes, batch DSL | Current | Medium |
| 001 v2.19.0 Release | 2026-01-05 | Async execution, 8 new tools | Current | Medium |
| **002 SAP Future Engineering** | 2026-01-18 | CBA positioning, Phase 1.5 | **Needs revision** — CBA framing too conservative | **Critical** |
| **MASTER-RESEARCH-SUMMARY** | 2026-01-18 | CBA context compilation | **Needs CBA AI context update** | **Critical** |
| **OPUS-STRATEGIC-RECOMMENDATIONS** | 2026-01-19 | Critical review of pitch | **Current and underappreciated** | **Critical** |
| 001 Tool Search Integration | 2026-02-03 | Claude Tool Search beta | Current | Medium |
| 002-004 Transport Safety | 2026-02-03 | v2.24.0 features | Current | High |
| **001 Deep Dive Review** | 2026-02-07 | 30+ architecture issues | **Current** | **Critical** |
| **002 Codex Analysis** | 2026-02-07 | Codex-generated deep dive | **Current** | **Critical** |

**Key finding**: 60%+ of reports are stale research. The 5 critical docs are: SAP Future Engineering (002), MASTER-RESEARCH-SUMMARY, OPUS-STRATEGIC-RECOMMENDATIONS, Deep Dive (001), and Codex Analysis (002). The rest should be archived to `reports/archive/`.

**Critical contradiction**: Report 010 claims cache is "complete" and CLAUDE.md marks it complete, but `pkg/cache/` has zero runtime integration — it's dead code with an unused SQLite dependency.

---

## 2. CBA Integration Readiness Matrix

| Architecture Component | Code Evidence | Honest Status | Gap |
|----------------------|---------------|---------------|-----|
| **SAP MCP (core ADT CRUD)** | `pkg/adt/client.go`, `crud.go`, 120 tools | **Operational** | None for core |
| **SAP Test Sub-Agent MCP** | `devtools.go:RunUnitTests`, `RunATCCheck` | **Partial** — triggers tests but no orchestration | No test result parsing for AI, no multi-agent coordination |
| **CBA Test Artefacts** | None | **Not started** | CBA's test libraries not integrated |
| **MCP Documentation servers** | None (CBA has own ABAP Docs MCP + DB3 System MCP) | **CBA-owned** | vsp should consume, not build |
| **abapGit -> GitHub sync** | `pkg/adt/git.go`, `GitExport`, WebSocket | **Operational** (158 object types, ZIP export) | No GitHub push, no Actions trigger |
| **SAP Cloud ALM integration** | None | **Not started** | No API calls, no evidence publication |
| **FTR evidence publication** | None | **Not started** | No evidence bundle generation |
| **Jira work item ingestion** | None | **Not started** | No Jira API |
| **GitHub Actions CI** | `.github/workflows/sync-upstream.yml` (sync only) | **Minimal** | No ABAP CI pipeline |
| **CTS/Transport orchestration** | `pkg/adt/transport.go`, 5 tools | **Operational** with safety | No Cloud ALM linkage |
| **Multi-agent coordination** | None | **Not started** | **CBA has Project Coral** — vsp needs to be Coral-compatible |
| **/CBA/ namespace enforcement** | `pkg/adt/safety.go:CheckPackage()` | **Configurable** | Works via `--allowed-packages "/CBA/*"` |
| **Audit logging** | **None** (confirmed: zero in codebase) | **Not started** | **BLOCKER** — CBA's AI governance requires audit trail |
| **Credential management** | Basic auth or cookies, CLI flags/env vars | **Basic** | No vault, no SSO, password in `ps` output |

**Verdict**: 3/14 operational, 2/14 partial, 9/14 not started (~25% coverage). But the critical reframe: **CBA already has the orchestration layer (Project Coral). vsp needs to be a tool that Coral can invoke, not a competing orchestrator.**

---

## 3. Assumption Challenge Results

### 3.1 Challenging Michael's Vision

#### a) "VS Code + SAP ADT will solve the IDE problem"
**Verdict: HOLDS but timing uncertain.** SAP announced VS Code ADT at TechEd 2025 but timeline is unclear. vsp provides the headless execution layer neither Eclipse nor VS Code gives an AI agent. vsp has 12-24 months of runway.

#### b) "AI coding assistants as peer programmers" is the right Phase 1 model
**Verdict: CORRECT for initial SAP adoption — but CBA is already PAST this for non-SAP code.**

Project Coral already does autonomous code scanning, fix generation, PR creation with human review. CBA's non-SAP engineering is already in "Phase 2" — AI executes, humans review. The gap is that SAP ABAP hasn't been brought into this workflow yet. **vsp should position as "bringing SAP into CBA's existing AI-powered engineering workflow" not "introducing AI to CBA's SAP team."**

#### c) "SAP MCP servers providing enterprise context" will work
**Verdict: PARTIALLY HOLDS.** CBA has 2 MCP servers already (ABAP Documentation, DB3 System). The pattern is proven. But the 10+ context types Michael lists (regulatory mappings, tribal knowledge) are likely aspirational. vsp should demonstrate integration with existing CBA MCPs, not build new ones.

#### d) "abapGit -> GitHub -> GitHub Actions" is the right CI pipeline
**Verdict: CORRECT DIRECTION — aligned with how CBA already works.** Project Coral already uses GitHub Issues, PRs, and CI/CD. The question is whether abapGit can serialize ABAP reliably enough for this workflow. Known failure modes exist but CBA's engineering team can handle them — they already manage complex CI/CD at scale.

#### e) "Multi-agent specialisation" (Phase 2) is realistic
**Verdict: MORE REALISTIC THAN INITIALLY ASSESSED — specifically at CBA.**

Project Coral already demonstrates multi-agent orchestration at CBA. The pattern works for non-SAP code. For SAP ABAP, the specific challenge is SAP's single-writer lock mechanism — but this is a constraint to design around, not a fundamental blocker. Solution: agents work on separate objects/packages, coordinator prevents lock conflicts. CBA's engineering team has the expertise to build this coordination — vsp just needs to be a reliable execution tool they can orchestrate.

**The multi-agent vision is realistic for CBA specifically because they've already solved the orchestration problem for other platforms. SAP is the new frontier, not the first experiment.**

#### f) "Confidence-based routing" (>95% auto, 80-95% review, <80% takeover)
**Verdict: MORE VIABLE AT CBA THAN GENERICALLY.**

CBA already runs confidence-based decision-making at massive scale (55 million AI decisions daily). They have governance models for this. The specific thresholds for ABAP changes need to be empirically determined, but the organizational machinery to operate confidence-based routing already exists at CBA.

That said, APRA compliance still requires human review for code changes affecting regulated systems. The confidence score determines review priority and depth, not whether review happens.

#### g) "Cultural shift recognising AI agents as first-class contributors"
**Verdict: LARGELY ALREADY ACHIEVED AT CBA (for non-SAP).**

CBA has 7,800 engineers working with AI agents. Project Coral creates autonomous PRs. GenAI Council has CEO involvement. The cultural shift has happened — **except possibly for the SAP team specifically**, which may operate with more traditional workflows. The SAP-specific cultural shift is narrower and more tractable than an organization-wide transformation.

#### h) "High-quality documentation of the existing system landscape"
**Verdict: LIKELY FALSE for SAP legacy, but vsp can help.**
Legacy SAP systems are under-documented. vsp's `GetCallGraph`, `GetObjectStructure`, `GrepObjects` can generate documentation from the living system. This is vsp's strongest Phase 1 use case: **use AI to document the SAP landscape that isn't documented.**

### 3.2 Challenging vsp's Positioning

#### a) "Production-ready" claim
**Verdict: DOES NOT HOLD — especially against CBA's standards.**

CBA's AI engineering team has built Project Coral with proper governance, CI/CD integration, monitoring, and audit trails. They will evaluate vsp against these standards:

- **Zero audit logging** — CBA's AI governance requires knowing who ran what when
- **No graceful shutdown** — no SIGTERM/SIGINT handlers, leaves SAP sessions locked
- **No structured logging** — only `--verbose` to stderr
- **3 tests for 2,489 LOC** in `internal/mcp/server.go`
- **WebSocket clients untested** — `amdp_websocket.go`, `websocket_debug.go` have 0 unit tests
- **No rate limiting** — AI agent can spam SAP with unlimited requests
- **Sensitive data committed** — password in `reports/2025-12-21-007:26`, metadata in `.log` file
- **AMDP concurrency bug** — lock shadowing between `AMDPWebSocketClient.mu` and base `c.mu`
- **No retry with backoff** — single CSRF/session retry

**vsp is "developer-ready." It is not CBA-production-ready.** CBA's team will immediately identify these gaps.

#### b) "99 tools" as a selling point
**Verdict: NUMBER IS REAL, VALUE IS INFLATED.**
Tool count contradictions: main.go says "19/45", README says "52/99", CLAUDE.md says "54/99" and "20/47". Duplicate tools coexist. Schema inconsistencies. An AI agent realistically uses 20-30 tools regularly.
**Lead with quality ("54 essential tools in focused mode") not quantity.**

#### c) "Execution layer" — is vsp trying to be too much?
**Verdict: YES — and the CBA context makes this clearer.**

CBA already has Project Coral for orchestration, GitHub for CI/CD, SonarQube/Snyk for quality, Jira for work items, Cloud ALM for SAP lifecycle. vsp should be **one tool in this ecosystem**, not a competing platform. **Position vsp as the SAP ADT bridge that Coral can invoke.**

#### d) Lua scripting
**Verdict: NOBODY AT CBA IS ASKING FOR LUA.** Deprioritize. CBA orchestrates via Coral/GitHub Actions.

#### e) Fork governance risk
**Verdict: SIGNIFICANT.** Third-party risk assessment for forked OSS with solo maintainer is a flag. **Best option**: CBA forks internally with 1-2 dedicated engineers, original maintainer advises.

#### f) ZADT_VSP deployment
**Verdict: PRACTICAL BLOCKER but manageable at CBA.** Lead with REST-only (52 tools), position ZADT_VSP as Phase 2 unlock after trust established.

### 3.3 Challenging Prior Strategic Analysis

#### a) "ABAPilot: absorb features, don't partner"
**Verdict: WRONG STRATEGY.** Don't compete on AI features. Compete on execution reliability. Let the AI layer be pluggable. vsp's moat is SAP ADT execution, not AI intelligence.

#### b) "3 CBA SKILLS" design
**Verdict: CBA's responsibility.** vsp provides the example pattern. CBA builds/maintains knowledge bases.

#### c) Cloud ALM integration scope
**Verdict: NOT VSP'S JOB.** Export structured JSON evidence bundles. CBA's team handles Cloud ALM.

#### d) "Phase 1.5 — operational today"
**Verdict: MISLEADING.** Individual tools work. End-to-end Jira -> fix -> PR does not exist. ~30% of claimed workflows work end-to-end. **But at CBA, Coral IS the glue code.** vsp doesn't need to orchestrate — it needs to be reliably invocable.

#### e) Resource estimates
**Verdict: CODING ESTIMATES FAIR; TOTAL EFFORT 2-3x.** But at CBA, organizational readiness (culture, governance, AI infrastructure) already exists. The delta is smaller. Phase 0 is 40 hours — table stakes.

---

## 4. Technical Findings

### 4.1 Architecture Issues

| # | Finding | File:Line | Severity | CBA Relevance |
|---|---------|-----------|----------|---------------|
| 1 | **server.go is 2,489 LOC god file** — 120+ tool registrations | `internal/mcp/server.go:224-2393` | Medium | Maintenance |
| 2 | **pkg/adt is a god package** — 25,607 LOC, 46 files | `pkg/adt/` | Medium | Testability |
| 3 | **MCP layer has 3 tests** for 2,489 LOC | `internal/mcp/server_test.go` | High | Reliability |
| 4 | **WebSocket clients have 0 unit tests** | `amdp_websocket.go`, `websocket_debug.go` | High | Reliability |
| 5 | **AMDP lock shadowing** concurrency bug | `pkg/adt/amdp_websocket.go:17` | High | Deadlock risk |
| 6 | **Debugger split** — REST (deprecated) vs WebSocket | `debugger.go`, `handlers_debugger_legacy.go` | Medium | Dual maintenance |
| 7 | **Method-level GetSource** fetches full class then slices | `pkg/adt/client.go:184` | Low | SAP load |
| 8 | **Tool schema inconsistencies** | Various handlers | Low | AI confusion |
| 9 | **Duplicate legacy/unified tools** | `internal/mcp/server.go` | Low | Bloat |
| 10 | **Cache package unused** — 2,180 LOC, SQLite dep | `pkg/cache/` | Low | Dead code |

### 4.2 Operational Issues (Critical for CBA)

| # | Finding | Severity | CBA Relevance |
|---|---------|----------|---------------|
| 1 | **ZERO audit logging** | **BLOCKER** | CBA AI governance requires it |
| 2 | **No graceful shutdown** | **BLOCKER** | Locked SAP sessions at scale |
| 3 | **No structured logging** | High | CBA observability standards |
| 4 | **No rate limiting** | High | Multi-agent SAP load |
| 5 | **No connection health checks** | High | Silent failures |
| 6 | **Sensitive data committed** | High | Compliance violation |
| 7 | **No WebSocket reconnection** | Medium | Fragile advanced features |
| 8 | **Minimal retry** | Medium | Transient failures |
| 9 | **Tool count contradictions** | Medium | Credibility |
| 10 | **Password in `ps` output** via CLI flags | Medium | Security |

### 4.3 What's Working Well

| # | Strength | Evidence |
|---|----------|----------|
| 1 | **CRLF normalization** — robust, tested | `workflows.go:1181-1185`, `workflows_test.go:500-536` |
| 2 | **Safety system** — comprehensive | `safety.go`, 25+ tests |
| 3 | **Error handling** — consistent Go patterns | Throughout `pkg/adt/` |
| 4 | **Feature detection** — auto/on/off | `features.go` |
| 5 | **abapGit integration** — 158 object types | `git.go` |
| 6 | **Async execution** — `RunReportAsync` | Innovative pattern |
| 7 | **Transport safety** — disabled by default | `transport.go` |
| 8 | **Minimal dependencies** — 7 direct | `go.mod` |

---

## 5. Positioning Recommendation

### What vsp should claim to be:

> **The SAP ADT MCP bridge that brings ABAP into AI-powered engineering workflows — including CBA's Project Coral.**

### Why this works for CBA:

1. **Aligns with existing infrastructure** — Coral orchestrates, vsp provides SAP execution
2. **Not competitive** — doesn't replace Coral, GitHub, Jira, Cloud ALM
3. **Fills a real gap** — SAP ABAP is the platform Coral can't reach today
4. **Defensible** — nobody else provides MCP-native full-CRUD SAP access
5. **Operational today** — core tools work, advanced features follow

### What to STOP claiming:

- "Execution layer for everything" — CBA has Coral
- "Multi-agent orchestrator" — Coral handles this
- "Phase 1.5" — say "operational today for Phase 1"
- "Production-ready" — say "developer-ready, enterprise hardening underway"

---

## 6. Build / Integrate / Ignore Matrix

| Capability | Action | Justification |
|-----------|--------|---------------|
| **Audit logging** | **BUILD** (blocker) | CBA AI governance requires audit trail |
| **Graceful shutdown** | **BUILD** (blocker) | 2 hours; prevents locked sessions |
| **Structured logging** | **BUILD** (high) | CBA observability; Go `log/slog` |
| **Rate limiting** | **BUILD** (high) | Protect SAP from multi-agent load |
| **MCP handler tests** | **BUILD** (high) | 20+ tests for untested layer |
| **Connection health** | **BUILD** (medium) | Periodic SAP ping |
| **WebSocket reconnect** | **BUILD** (medium) | Auto-reconnect with backoff |
| **Project Coral integration** | **INTEGRATE** (high) | vsp as Coral-invocable MCP tool |
| **GitHub PR creation** | **INTEGRATE** (medium) | Export; Coral/Actions create PR |
| **Evidence bundles** | **INTEGRATE** (medium) | JSON export; Cloud ALM consumes |
| **CBA SKILLS pattern** | **INTEGRATE** (medium) | Example; CBA builds knowledge bases |
| **Credential vault** | **INTEGRATE** (medium) | Support HashiCorp/AWS |
| **Jira** | **IGNORE** | CBA owns; Coral handles |
| **Cloud ALM deep integration** | **IGNORE** | CBA integration team's job |
| **Multi-agent orchestration** | **IGNORE** | Coral handles this |
| **NL query / AI review** | **IGNORE** | AI layer is pluggable |
| **Lua scripting** | **DEPRIORITIZE** | Keep as-is |
| **Cache package** | **DECIDE** | Integrate or remove |

---

## 7. What Blocks CBA Adoption (Ranked)

### Blocks a Pilot

1. **No audit logging** — CBA's AI governance requires full audit trail. Coral has this. vsp does not.
2. **Credential management** — No vault, no SSO, password in `ps` output.
3. **Security review** — Sensitive data in repo, dynamic RFC, test gaps.
4. **Mac SOE compatibility** — CBA disabled MCP for Copilot. vsp needs pre-approval.

### Blocks Daily Use

5. **No graceful shutdown** — Locked SAP sessions at scale.
6. **No structured logging** — CBA observability requires structured data.
7. **ZADT_VSP deployment** — SAP Basis approval needed.
8. **Tool count contradictions** — Credibility with Coral-quality engineering team.
9. **No rate limiting** — SAP performance risk with multiple agents.

### Blocks Production

10. **Single-maintainer risk** — Bus factor of 1 for bank with 7,800 engineers.
11. **Fork governance** — Third-party risk assessment.
12. **No observability** — No metrics, tracing, health endpoints.
13. **Coral integration gap** — No defined interface.
14. **APRA/SOX evidence chain** — No audit trail = no compliance sign-off.

---

## 8. Roadmap (Honest Effort Estimates)

### Phase 0: "Make It CBA-Ready" (2-3 weeks, 1 developer)
**Coding: 40h | Non-coding: 16h**

Must-do before any CBA conversation:
- [ ] Audit logging (structured JSON of every tool invocation) — 8h
- [ ] Graceful shutdown (SIGTERM/SIGINT, WebSocket cleanup) — 2h
- [ ] Structured logging via `log/slog` — 6h
- [ ] Remove sensitive data (password in report, .log file) — 1h
- [ ] Fix .gitignore (*.log, /output/) — 30min
- [ ] MCP handler tests (20+ tests) — 8h
- [ ] Fix tool count contradictions — 2h
- [ ] Fix AMDP lock shadowing bug — 2h
- [ ] Rate limiting (configurable per-tool) — 4h
- [ ] Security documentation for CBA review — 8h
- [ ] Remove or integrate cache package — 4h

**Kill criteria**: CBA AI engineering team rejects architecture after Phase 0.
**CBA provides**: Security standards, SOE requirements, credential policy.

### Phase 1: "Coral Integration" (2-3 months, 1-2 developers)
**Coding: 200h | Non-coding: 80h**

Focus: Make vsp a tool that Project Coral can orchestrate for SAP ABAP.
- [ ] Define vsp MCP interface for Coral — 16h
- [ ] WebSocket reconnection with backoff — 16h
- [ ] Connection health monitoring — 8h
- [ ] Credential vault integration — 24h
- [ ] Evidence bundle export (JSON) — 16h
- [ ] Unify debugger (WebSocket-first) — 16h
- [ ] Decompose server.go — 24h
- [ ] Integration test harness (offline mock) — 40h
- [ ] SAP Basis deployment runbook — 24h (non-coding)
- [ ] CBA configuration guide — 16h (non-coding)
- [ ] SAP Basis approval — 4-8 weeks (CBA-dependent)

**Kill criteria**: SAP ships VS Code ADT + MCP + full CRUD.
**CBA provides**: SAP access, Basis team, Coral API docs, pilot team.

### Phase 2: "Scale" (3-6 months, 2-3 developers)
**Coding: 400h | Non-coding: 160h**

- [ ] Observability (metrics, tracing, health) — 40h
- [ ] Multi-instance (Coral spawning multiple vsp agents) — 60h
- [ ] /CBA/ namespace enforcement — 8h
- [ ] abapGit -> GitHub push — 40h
- [ ] GitHub Actions trigger — 24h
- [ ] Test result parsing for AI — 32h
- [ ] Performance testing at CBA scale — 40h
- [ ] Training materials — 40h (non-coding)
- [ ] Governance model — 16h (non-coding)
- [ ] Second contributor — 24h

**Kill criteria**: <5 daily users by end of Phase 2.

---

## 9. Kill List

| Item | Action | Justification |
|------|--------|---------------|
| **"Phase 1.5" terminology** | Kill | CBA is past Phase 1 for non-SAP. Say "operational today." |
| **"Production-ready" claim** | Kill | Say "developer-ready, enterprise hardening underway" |
| **"99.2% token reduction" claim** | Kill | Strawman comparison |
| **"Execution layer for everything"** | Kill | CBA has Coral. vsp is the SAP bridge. |
| **Multi-agent orchestrator positioning** | Kill | Coral handles orchestration |
| **Cache package** (if not integrated) | Remove | Dead code + SQLite dependency |
| **Lua investment** | Freeze | Keep, no further development |
| **Legacy REST debugger** | Deprecate | WebSocket forward |
| **Graph roadmap** (Reports 005-007) | Archive | Not implemented after 3 months |
| **60+ stale reports** | Archive | False confidence, overwhelming |
| **Response letter (387 lines)** | Rewrite | One page + demo |
| **Duplicate ABAP trees** | Consolidate | Keep `embedded/abap/` |
| **Sensitive data** | Delete immediately | Compliance violation |

---

## 10. Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| SAP ships VS Code ADT + AI/MCP | Medium (12-18mo) | High | vsp operational today. Become embedded before SAP catches up. |
| CBA security blocks ZADT_VSP | Medium | Medium | Lead REST-only (52 tools). ZADT_VSP is Phase 2. |
| **CBA AI team finds vsp immature** | **High** | **High** | **Complete Phase 0 before evaluation. Match Coral standards.** |
| Upstream fork adversarial | Low | Medium | MIT license. Formalize relationship. |
| SAP Joule good for on-prem | Low (24+mo) | High | Joule cloud-first, read-only. 2+ years for on-prem CRUD. |
| CBA Mac SOE blocks vsp | Medium | High | Go binary easier to whitelist. Get pre-approval. |
| Michael's vision unfunded | Low-Medium | Critical | CBA already investing in AI engineering. SAP is the gap. |
| Solo maintainer unavailable | Medium | High | Phase 1 includes second contributor. |
| Coral subsumes vsp scope | Low | Medium | Coral orchestrates, doesn't provide SAP tools. Complementary. |
| **CBA builds own SAP MCP** | Low-Medium | Critical | **Move fast. Be obvious choice before they build.** |

---

## 11. The Uncomfortable Questions — Answered Honestly

### If SAP ships native VS Code + ABAP + AI in 2026, does vsp exist?
**Yes, for 12-24 months.** SAP VS Code ADT will be thin. Joule is cloud-first, read-only. On-prem full-SDLC is 2+ years. But the window is finite. Become embedded via Coral before SAP catches up.

### Is a solo-maintainer fork credible for CBA?
**Not as-is.** CBA has 7,800 engineers at Coral quality. Best path: CBA forks internally with 1-2 dedicated engineers. Original maintainer advises.

### Is "execution layer" operational?
**~30% operational.** Individual tools work. Full workflows need Coral as glue. **This is actually a strength**: vsp doesn't need to orchestrate — just be reliably invocable.

### Is multi-agent realistic for CBA?
**Yes — uniquely so.** Coral already does multi-agent. SAP locks need coordination (agents on separate objects), which CBA can build.

### What would Michael's team find trying vsp tomorrow?
1. Manual env vars (no SSO/vault)
2. No audit trail (Coral logs everything; vsp nothing)
3. No Coral integration path
4. ZADT_VSP not deployed (REST-only)
5. Quality gaps (untested handlers, tool count contradictions)
6. **But genuine value**: GetSource, WriteSource, SyntaxCheck, Activate, RunUnitTests — the core loop works. No other MCP tool does this.

**The pitch: "We have what you need for SAP. We know we need to harden it. Here's the 3-week plan to match your standards."**

---

## Sources

- [CBA AI Practices](https://www.commbank.com.au/about-us/opportunity-initiatives/policies-and-practices/artificial-intelligence.html)
- [CBA Chief AI Officer](https://www.commbank.com.au/articles/newsroom/2025/11/ranil-boteju-chief-ai-officer.html)
- [CBA AI-Powered Engineering / Project Coral](https://www.commbank.com.au/articles/newsroom/2025/08/ai-powered-engineering.html)
- [CBA OpenAI Partnership](https://www.commbank.com.au/articles/newsroom/2025/08/tech-ai-partnership.html)
- [CBA AI Report Feb 2026](https://www.commbank.com.au/articles/newsroom/2026/02/cba-approach-to-adopting-ai-report-announcement.html)
- [CBA Cloud Migration](https://www.commbank.com.au/articles/newsroom/2025/06/cba-ai-migration-cloud.html)
- [CBA AI-Native Banking](https://www.cio.inc/commonwealth-bank-australia-builds-ai-native-banking-a-30513)
- [CBA AI Software Delivery](https://www.itnews.com.au/news/cba-plans-to-use-ai-across-entire-software-delivery-614346)
- [Project Coral Blog](https://medium.com/commbank-technology/project-coral-how-were-orchestrating-ai-agents-for-development-at-scale-e0e11b9f0e2a)
- [CBA AI Engineering Evolution](https://medium.com/commbank-technology/the-evolution-of-ai-software-engineering-75a8a5a02c14)
