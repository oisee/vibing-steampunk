# Marian Zeis — how far his line has gone, and what is worth harvesting

Survey date: 2026-08-21. Read-only: no issues, comments, PRs, stars, forks or pushes were
made; nothing was added as a remote to `vibing-steampunk` or `open-rfc-go`. Code was read
from throwaway clones under the session scratchpad.

Companion to [`fork-survey.md`](fork-survey.md) (2026-08-20). That survey covered his
**vsp fork**; this one deliberately does not repeat it and goes where it did not — the
standalone projects, the trajectory, and everything newer.

---

## Headline

His vsp fork is not the story, and has not been since March. **`marianfoo/vibing-steampunk`
is frozen** — 38 ahead / 265 behind, last pushed **2026-03-25**. On *that same day* he
created `arc-mcp/arc-1` and started over in TypeScript. Everything he has built since is on
the ARC-1 line.

| | |
|---|---|
| `oisee/arc-1` | Our own **fork of his repo** — 0 ahead, 0 behind, identical. A tracking mirror |
| His flagship | **`arc-mcp/arc-1`** — MIT, TypeScript, 166★, 46 forks, 582 commits since 2026-03-25, v1.1.0 |
| Its origin | *"Initial release. Ported from oisee/vibing-steampunk"* (`CHANGELOG.md:895`) |
| Its licence | MIT, **dual copyright**: `Copyright (c) 2025-2026 Alice Vinogradova and contributors` + `Copyright (c) 2026 Marian Zeis` |
| Scale | 59 162 LOC `src/`, 97 153 LOC `tests/`, 221 test files |
| It is not | a vsp fork in git terms — a clean-room-adjacent **reimplementation** with attribution, sharing no history |
| Around it | a whole GitHub org, `arc-mcp`, with 14 repos |
| Newest line | **`marianfoo/open-rfc`** (2026-08-07, Apache-2.0) — the SDK-free RFC client our `open-rfc-go` ports |

---

## 1. What `oisee/arc-1` actually is

`gh api repos/oisee/arc-1` returns `"fork": true` with `parent.full_name = "arc-mcp/arc-1"`.
`gh api repos/arc-mcp/arc-1/compare/main...oisee:arc-1:main` returns
`{"ahead_by": 0, "behind_by": 0, "status": "identical"}`. Its only branches are `main` and an
inherited `release-please--branches--main--components--arc-1`. Created 2026-04-29, last
synced 2026-08-20.

So: **it is our own read-only mirror of Marian's product, kept current, with zero local
commits.** It is not an archive of vsp, not a rewrite of ours, and not a repo anyone has
worked in. Its value is exactly the value of watching ARC-1 — which, as the next sections
show, is considerable.

The parent, `arc-mcp/arc-1`, is the real object of study.

---

## 2. What ARC-1 is

**ARC-1 ("ABAP Relay Connector")** is vsp's premise — an ADT REST → MCP bridge for SAP
ABAP — rebuilt in TypeScript and pointed at a different customer. vsp is a developer's
single Go binary run on a laptop. ARC-1 is, in its own words
(`docs_page/roadmap.md`, "Vision"):

> a **centralized, admin-controlled MCP gateway** deployed on BTP Cloud Foundry or a
> company server (Docker). […] The admin controls which tools are exposed, which packages
> can be touched, and whether writes are allowed — before any LLM request reaches SAP.

Concretely:

- **Distribution**: npm (`arc-1`, with provenance attestations + CycloneDX SBOM) and a GHCR
  Docker image; a documentation site at `docs.arc-1-mcp.com` (mkdocs, 46 pages in
  `docs_page/`); a landing page; a blog series on `blog.zeis.de`.
- **Tool surface**: **12 verb-shaped tools** — `SAPRead`, `SAPWrite`, `SAPSearch`,
  `SAPNavigate`, `SAPQuery`, `SAPActivate`, `SAPTransport`, `SAPGit`, `SAPLint`,
  `SAPContext`, `SAPDiagnose`, `SAPManage` — each dispatching on `action`/`type`
  (`src/handlers/schemas.ts`). Not a mode switch like our 99/54/1: one permanent
  intent-shaped surface. He took our hyperfocused idea and made it the *only* design.
- **Safety**: he inherited our 13 operation-type codes wholesale (`src/adt/safety.ts`,
  and his own `docs/compare/01-vibing-steampunk.md` says "ARC-1 inherited this design"),
  then put a scope layer *on top* — see §5.1.
- **Auth**: API key, OIDC/JWT, OAuth 2.0 (BTP service key), XSUAA, BTP Destination Service,
  Cloud Connector principal propagation, `OAuth2UserTokenExchange` for BTP ABAP.
- **No RFC at all.** `grep -rn '\brfc\b' src/*.ts` matches only RFC *specification* numbers
  (7636, 8252, 9728, 7231) and ST05 process-type strings. ARC-1 is pure ADT HTTP. His RFC
  work lives in the separate `open-rfc` line (§4).

Cadence: 179 commits in April, 96 in May, 195 in June, 63 in July, 49 in August. Sustained
and still going — last commit 2026-08-20 23:10.

---

## 3. The `arc-mcp` org — the shape of the thing

He did not build a product; he built an **ecosystem**, and split it into publishable
modules. All read from `gh api orgs/arc-mcp/repos` plus each repo's README.

| Repo | Lang | Licence | What it is |
|---|---|---|---|
| `arc-1` | TS | MIT | The gateway itself (166★) |
| **`adt-ls`** | TS | Apache-2.0 | **SDK over SAP's headless `adt-lsc` language server** — a *second transport to ADT*, not REST. See §5.5 |
| **`arc1-adt-abap-mcp-ext`** | Java | MIT | **Eclipse plugin contributing 18 tools to SAP's own in-Eclipse MCP server.** See §5.6 |
| `xsuaa-auth` | TS | MIT | The BTP auth stack **extracted as a reusable npm package** (`@arc-mcp/xsuaa-auth`) — XSUAA proxy, RFC 7591 dynamic client registration, chained XSUAA→OIDC→API-key verifier, principal propagation |
| `mcp-hub` | TS | — | Deterministic multi-system MCP hub for BTP: one login, `/dev/mcp` `/qa/mcp` `/prod/mcp` routed to per-system ARC-1 instances, per-user identity preserved via `OAuth2JWTBearer` destination exchange. **No LLM in the middle** — explicitly |
| `arc-1-lsp` | TS | — | Sibling MCP server that delegates *all* ADT work to `adt-ls` |
| `arc-1-extension-sample` | TS | — | Worked sample for the ARC-1 plugin API, live-verified against S/4HANA |
| `arc1-abap-bridge` | JS | MIT | VS Code bridge between SAP's ABAP extension and ARC-1 |
| `arc-1-segw-to-rap`, `arc-1-abap-cicd-review`, `arc1-transport-review-poc` | ABAP/JS | mixed | Demo/PoC repos: SEGW→RAP migration, ABAP review in a GitHub PR workflow, CTS transport review as pull requests |
| `arc-1-mcp.com`, `live-arc-1` | HTML/TS | — | Landing page; interactive replay demo |

Outside the org, on his personal account, the relevant ones:

| Repo | Lang | Licence | Relevance |
|---|---|---|---|
| **`open-rfc`** | TS | Apache-2.0 | SDK-free classic-sync RFC client. Our `open-rfc-go` is the port. §4 |
| `vibing-steampunk` | Go | MIT | His fork of ours. **Frozen 2026-03-25.** Covered in `fork-survey.md` §5 |
| `ztoad` | ABAP | GPL-3.0 | Maintained clone of S. Hermann's Open SQL editor. An **ABAP-side** artifact, but unrelated to MCP — not a companion to ARC-1 |
| `sap-mcp-servers` | TS | Apache-2.0 | Monorepo: API Hub / Road Map Explorer / SAP Notes MCP servers + shared SAP auth |
| `mcp-sap-notes` (56★), `btp-cf-mcp`, `btp-drawio-skill` | TS/Py | Apache-2.0 / MIT | Adjacent SAP MCP servers — no ADT/RFC overlap |
| `sap-ai-mcp-servers` | — | MIT | The 435★ **directory** of SAP MCP servers. He curates the map of the space he competes in |

---

## 4. `open-rfc` — the newest line, and the one closest to us

`marianfoo/open-rfc`, Apache-2.0, created **2026-08-07**. It is the direct answer to his own
April research note (§8): an SDK-free TypeScript/JavaScript client for SAP classic
synchronous RFC on Node.js — no NetWeaver RFC SDK, no S-user, no native addon, **zero runtime
dependencies**. Our `/Users/alice/dev/open-rfc-go` is the Go port of it.

**Its shape.** 17 commits, all between 2026-08-07 and 2026-08-10; tags `v0.2.0`–`v0.2.3`;
published on npm as `open-rfc`. 45 168 LOC in `src/` (`protocol`, `transport`, `client`,
`compat`, `destination`, `lifecycle`, `metadata`, `pool`, `values`, `diagnostics`) against
44 907 LOC of tests, run by a hand-rolled conformance harness
(`tools/public_conformance.mjs` + `tools/public_test_suite.mjs`) rather than a framework.
Full contributor surface: `NOTICE`, `THIRD_PARTY_NOTICES.md`, **`DCO.md`**, `CONTRIBUTING.md`,
`CODE_OF_CONDUCT.md`, `SECURITY.md`, `SUPPORT.md`, and a mkdocs site. No CLI, no MCP surface.

**Divergence: none pending.** His `main` has had **zero commits since 2026-08-10**. Our
`docs/provenance.md` pins the baseline at `847036dce5e29015bbc266a4d19cc9c15295a831`
(open-rfc 0.2.3) — which is his current HEAD. There are unmerged feature branches on his repo
(`codex/connectivity-socks5-rfc-route`, `codex/v1-roadmap`, `chore/launch-readiness`, …) but
nothing landed. **There is nothing to backport.**

**And we have overtaken him.** Not by a little. Things `open-rfc-go` has that `open-rfc`
explicitly does not:

| Capability | `open-rfc` (TS) | `open-rfc-go` |
|---|---|---|
| RFC **server** — answering inbound calls, SM59 type-3 test green | Not present; README lists it as unsupported | `internal/rfcserver/`, live-verified against a real ABAP program, all three serialization modes |
| Server→client callbacks (`DESTINATION 'BACK'`) | Unsupported | `Session.CallWithCallbacks`, live-verified .105↔.103 |
| Fast serializer | Deliberately declined — his roadmap treats the grammar as an unsolved XL/high-risk problem | Partial decoder (character fields), reverse-engineered from live A4H captures |
| CLI | None | `rfc info/ping/describe/call/search/read-table/mcp` |
| MCP tool surface | None | `cmd/rfc/mcp.go`, with `--expose`/`--hide` per-FM autodiscovery and a `--safe` write-FM gate |
| Error taxonomy | `RFCError`/`ABAPError` classes | Typed `*rfc.ABAPException` plus an `errors.Is` sentinel tree |
| Fuzzing | Not a stated policy | House rule "fuzz every decoder" — 18 `Fuzz*` targets |
| SAProuter | Implemented but "not supported by this release" | Ported **and wired into the dial path** |

At parity: password scramble (his 21 592-vector conformance sweep reproduced), classic
serializer, flat and nested structures/tables, `RFC_METADATA_GET` + `RFC_GET_FUNCTION_INTERFACE`,
STRING/XSTRING xRFC, unicode/codepage handling (including his zero-padded XML character-reference
fix), simple row compression, BTP Connectivity SOCKS5. Neither side has SNC, tRFC/qRFC/bgRFC,
or a gateway-**registered** inbound server.

Deliberately not ported: `src/compat/**` (his node-rfc / `@sap/cds-rfc` shims — no Go consumer)
and his ~2100-line Promise-scheduler pool, which we redesigned as a generic `Pool[T]`.

**A correction to the brief.** `vsp rfc probe` and abapGit-ZIP-over-RFC are **vsp** features
(`pkg/saprfc/probe.go`, `pkg/saprfc/abapgit.go`, commits `6169305` and `60aacbf`), not
`open-rfc-go` features — `open-rfc-go` has neither. Worth keeping straight: the RFC *protocol*
work lives in `open-rfc-go`, the RFC *applications* live in vsp.

**Still worth taking from `open-rfc`:**

- **His "road-to-v1 gate ledger"** — `tools/v1_roadmap.mjs` + `conformance/v1-gates.v1.json`
  (a 2433-line machine-checked ledger) + `test/v1-roadmap.test.mjs`, from commit `89030dd`.
  Our `docs/roadmap.md` is prose and a table; his is executable. Skim the gate *categories*
  even if we do not adopt the mechanism.
- **`docs_page/troubleshooting.md`, `safety.md`, `operations.md`** — good operational prose
  (gateway port `33NN` vs dispatcher `32NN`; never retry a mutating RFM after an uncertain
  send; credential handling). Adapt, do not copy.
- **His release-artifact verification** (`docs_page/status.md`): download the GitHub release,
  `npm pack`, compare SHA-256 against `dist.integrity`. The *pattern* — prove the release
  bytes equal the registry bytes — transfers to Go binaries.

**Attribution status: already correct, and unusually thorough.** `open-rfc-go` carries
Apache-2.0, a `NOTICE` crediting "open-rfc … Copyright 2026 Marian Zeis", a verbatim copy of
his `DCO.md`, and a `docs/provenance.md` that maps every ported file to its upstream path and
commit with a note on what changed — which over-satisfies Apache-2.0 §4(b). It also correctly
reasons that no `THIRD_PARTY_NOTICES.md` is needed because the only upstream files carrying
third-party SAP/node-rfc code were not ported, and flags that this must be revisited if they
ever are. Nothing to fix here.

---

## 5. He is ahead of us here

This is the part worth acting on. Each item is something with **no counterpart in
`vibing-steampunk`** (verified by grep over our working tree), ordered by how much it would
change what vsp can do.

### 5.1 A scope/policy matrix as a single source of truth, validated in CI

`src/authz/policy.ts` (321 L) is one `ACTION_POLICY` table keyed by `Tool.action`, each row
carrying `{ scope, opType, featureGate }`. Seven scopes (`read`, `write`, `data`, `sql`,
`transports`, `git`, `admin`), plus an `OPTYPE_SCOPE` consistency map so a tool cannot claim
`read` while declaring a write op. Two things make it good rather than merely present:

1. `dispatch.ts` consults it before *every* call, and **`tools/list` prunes actions the
   caller's scopes cannot execute** — the surface shrinks per-principal.
2. `scripts/validate-action-policy.ts` runs in CI and asserts the policy table and
   `src/handlers/schemas.ts` cover each other exactly. Adding an action without a policy row
   fails the build.

We have `pkg/adt/safety.go` (the op-type codes he took from us) but **no scope layer, no
per-principal tool pruning, and no CI coverage assertion**. Our safety config is global to
the process, not per-caller. This is the single most transferable idea in his codebase.

### 5.2 A plugin/extension framework — custom tools without forking

`src/server/plugin-loader.ts` + `src/plugins/manifest-interpreter.ts` + a published
`arc-1/public` entrypoint. Two tiers: a **code tier** (`defineTool`, TypeScript, gets
`ctx.http` / `ctx.run.classRun`) and a **manifest tier** (`*.tool.json`, no code, wraps one
GET). Custom tools inherit the authenticated SAP client, the safety ceiling, the scope
policy, audit, and principal propagation. The gating is careful:
`ctx.http` always allows GET/HEAD; POST/PUT/DELETE only to **non-ADT** paths and only behind
default-off `SAP_ALLOW_PLUGIN_RAW_WRITES`; writes to `/sap/bc/adt/…` object endpoints are
**always refused** because a raw path cannot be package-checked.

We have no plugin system at all (`grep -c plugin` over our `.go` files: 1, and it is
unrelated). Every vsp extension today is a fork — and `fork-survey.md` counted 41 of them.
That is the problem this solves.

### 5.3 A discovery-driven generic object engine ("server-driven objects")

`src/adt/server-driven.ts` (433 L). Rather than per-type plumbing for each new ABAP Platform
2025 object type, one engine handles the whole AFF generic-object family (the `blues`
content-type family, plus DTDC/DSFD/DTSC) from a **registry** of
`{ href, createType, metadataContentType, discoveryMarker, sourceFormat }`, and **gates each
type on `/sap/bc/adt/discovery` rather than on a hardcoded release number**. Read, create,
source-PUT, delete and activate all flow through it. Each registry row is annotated with the
system it was live-verified on (758 / 816).

Our client hardcodes per-type URLs. His own roadmap item ARCH-01 is to push this further and
delete his remaining release gates. The idea — *ask the system what it supports, don't
guess* — is one we should adopt regardless of whether we take the code.

### 5.4 The ADT type-availability probe, and the research corpus behind it

`src/probe/` (951 L across 6 files) plus `scripts/probe-adt-types.ts`. For every ADT object
type it collects several *independent* signals — presence in the discovery map, a collection
GET, a known-object GET against SAP-shipped objects (`SAPMSSY0`, `CL_ABAP_TYPEDESCR`, …), and
a conservative release floor — and reports both a per-type verdict **and quality metrics for
the probe itself**, so a user can see "TABL is supported" *and* "this answer is weak because
no universally-shipped BDEF exists". The HTTP classification vocabulary is well thought out:
`ok-400-bad-params` and `ok-405-method` both mean *the endpoint exists*.

Behind it: `docs/research/abap-types/` with a `00-methodology.md` and **37 per-type dossiers**,
and `docs/research/` with **69 dated research documents** and `docs/plans/` with 56. Several
of the type dossiers explicitly record what `vibing-steampunk`, `sapcli`, `fr0ster` and
`abap-adt-api` each do or do not support. This corpus is arguably worth more than any single
feature.

We have `pkg/saprfc/probe.go` and `vsp rfc probe` — a *system fingerprint* over RFC. That is
a different and complementary thing; we have no ADT type-availability probe.

### 5.5 `adt-ls` — a second, non-REST transport into ADT

`arc-mcp/adt-ls`, Apache-2.0, published as `@arc-mcp/adt-ls`. SAP ships a **headless
language server**, `adt-lsc`, inside its official `sapse.adt-vscode` extension. Marian wrote
an SDK that discovers it, starts the JVM, does the named-pipe + LSP handshake, the
**reentrance-ticket logon**, TLS/truststore and session resilience, and exposes
`adt.repository.search()`, `adt.source.read()`, `adt.lifecycle.create/activate()` and so on.
Auth strategies: `basic`, `bearer`, `interactive`, `clientCert`. He reports the full
authoring lifecycle running live against S/4HANA on adt-ls `1.0.1.202606111342`, on Node ≥20
and Bun. `arc-1-lsp` is an entire MCP server built on it.

This is a genuinely different door into the same system: instead of reverse-engineering ADT
REST contracts, drive SAP's own supported client library. The catch is that adt-ls is under
the SAP Developer License and **not redistributable** — the user must bring the VSIX. That is
also, from our side, the reason this is interesting rather than threatening: it is not a path
a single self-contained Go binary can take without shipping a JVM dependency.

### 5.6 The SAP-ships-an-MCP-server discovery

`arc-mcp/arc1-adt-abap-mcp-ext` (Java, MIT). **As of ADT 3.60, SAP ships a supported MCP
server inside Eclipse-for-ABAP**, off by default, exposing only SAP's own tools. His plugin
is a JAR you drop into `dropins/` that contributes **18 read-only ABAP tools** through SAP's
documented extension point `com.sap.adt.mcp.core.adtMcpTools`, and optionally logs you into
your ABAP project. Earlier versions (≤ 0.3.x, for ADT 3.58/3.59) *reflectively woke the
then-dormant server* — he found it before SAP shipped the activation surface.

We have no visibility on this at all. Even ignoring the plugin, the **fact** matters
strategically: the platform vendor is now in our product category, and there is a documented
extension point for riding it instead of competing with it.

### 5.7 A `tools/list` byte budget enforced in CI

`scripts/ci/check-tool-schema-budget.ts`. Two kinds of limit per scenario: hard **wire-byte
ceilings** measured on the exact `JSON.stringify({ tools })` the client receives, and
**token ratchets** (a deterministic bytes/4 estimate) seeded at current-plus-headroom, meant
to be lowered when the surface shrinks and raised only consciously, in the diff. The header
comment is honest about provenance: it was added under a theory about Copilot-for-Eclipse
that was *later disproven*, and it says so, while arguing the guard is still sound hygiene.

His roadmap shows it biting: FEAT-73 (three new object types) is **blocked** with
`check:sizes` at 67 986 / 68 000 bytes. A budget that actually blocks features is a budget
that works. We talk about token efficiency constantly and measure it nowhere.

### 5.8 Governance and ops plumbing we simply do not have

Grep over our `.go` files returns zero for each of these:

- **Three-layer rate limiting** — per-IP edge, per-user MCP quota, and a server-wide
  SAP-bound semaphore (`src/adt/semaphore.ts`, `src/server/mcp-rate-limit.ts`,
  `auth-rate-limit.ts`), honouring `Retry-After` on 429/503 from BTP gateways.
- **Structured audit logging with pluggable sinks** — `src/server/sinks/{stderr,file,btp-auditlog}.ts`.
- **HTTP security headers** (helmet: HSTS, CSP, X-Frame-Options, CORP), on by default, no
  disable flag; opt-in exact-match CORS via `ARC1_ALLOWED_ORIGINS`; COOP deliberately *not*
  set so popup OAuth keeps working — a documented, reasoned exception.
- **RFC 9728 protected-resource metadata** (`feat: serve RFC 9728 protected-resource metadata
  in OIDC mode`, #632). We have the API key + `WWW-Authenticate` (`internal/mcp/server.go:293`,
  using `subtle.ConstantTimeCompare`) but not the discovery document.
- **W3C trace-context propagation to SAP plus calling-agent identity** recorded (#641).
- **A supply-chain posture**: Dependabot across npm/Actions/Docker, `npm audit` PR gate,
  Dependency Review, CodeQL, Trivy, all third-party Actions pinned to SHA, `SECURITY.md` with
  severity-tiered SLAs, npm + image provenance attestations, CycloneDX SBOM per release.
- **A local web UI** — `src/server/ui.ts` + `public/ui/` — for inspecting server state and logs.
- **AFF (ABAP File Formats) JSON-schema validation** of written objects — `src/aff/validator.ts`
  with SAP's own bundled schemas for CLAS/INTF/PROG/DDLS/BDEF/SRVD/SRVB. We have zero AFF.
- **22 packaged SAP skills** (`skills/`) — `migrate-segw-to-rap`, `generate-rap-service`,
  `sap-clean-core-atc`, `debug-slow-sql`, `sap-migration-dossier`, … Product surface built
  *on* the tools rather than more tools.

### 5.9 He is tracking us, in detail, and we should read it

`docs/compare/` holds nine competitor dossiers (`abap-adt-api`, `mcp-abap-adt`, `fr0ster`,
`dassian-adt`, `sapcli`, `aws-abap-accelerator`, `btp-odata-mcp`, and **`01-vibing-steampunk.md`**),
a `00-feature-matrix.md`, per-repo `commits.json` / `issues.json`, and — for us —
`docs/compare/vibing-steampunk/evaluations/`, **24 per-commit and per-issue evaluation
documents**, each deciding whether one of our changes applies to him. He has
`.claude/commands/update-competitor-tracker.md` automating the refresh.

This is a free, adversarial, competent review of our own work. Two examples from
`01-vibing-steampunk.md` that are actionable *for us*:

- On our commit `0713d75`: *"**CRITICAL: Package safety bypass on mutations (#101)** —
  `SAP_ALLOWED_PACKAGES` not enforced on update/delete. **ARC-1 has same bug.** Fix
  immediately."* He audited our fix and found the same hole in his own code.
- On our `22517d4`: he identifies our `modificationSupport` guard as *"the root cause of all
  423 errors"* and rates it High — a cleaner articulation of that bug class than our own
  commit message.

His tracker is dated *Last updated: 2026-04-27*, so it does not know about our RFC line,
`vsp rfc probe`, or the abapGit-ZIP-over-RFC work. That is our current lead.

---

## 6. Ranked harvest list

Licensing throughout: **ARC-1, `xsuaa-auth`, `arc1-adt-abap-mcp-ext` are MIT** — same as vsp,
so a file-level take is legally clean with attribution retained. **`adt-ls` and `open-rfc` are
Apache-2.0** — compatible with MIT distribution, but Apache-2.0 carries NOTICE and
patent-grant obligations, so a wholesale copy needs a `NOTICE` entry (we already keep one at
`/Users/alice/dev/vibing-steampunk/NOTICE`). None of it is Go, so **every "take" below is a
reimplementation in practice, and the licence question is really an attribution question.**

And the standing point: **our DCO makes inviting him better than copying.** He has already
contributed to vsp before (`fork-survey.md` §"upstream already merges community PRs
routinely" lists marianfoo among the authors upstream has taken work from), and his LICENSE
credits Alice by name. For anything substantial — 6.1, 6.2, 6.5 — an invitation to author it
himself is both cheaper and more correct than a reimplementation.

| # | Item | Source | Verdict |
|---|---|---|---|
| **6.1** | **Scope matrix + per-principal `tools/list` pruning + CI coverage assertion** | `src/authz/policy.ts`, `scripts/validate-action-policy.ts` | **Reimplement.** The design is ~320 lines of table and is the highest-leverage thing here. The CI validator is the part not to skip |
| **6.2** | **Plugin/extension framework** with the GET-open / write-gated / ADT-paths-always-refused rule | `src/server/plugin-loader.ts`, `src/plugins/`, `docs_page/extensions.md` | **Reimplement**, and take the *gating rule* verbatim — the reasoning ("a raw path can't be package-checked") is correct and is the whole security argument. Go plugin story is harder than Node's; a subprocess/JSON-RPC tier may suit us better than in-process |
| **6.3** | **`tools/list` wire-byte budget + token ratchet in CI** | `scripts/ci/check-tool-schema-budget.ts` | **Reimplement.** Half a day. Immediately useful given our 99-tool expert mode |
| **6.4** | **Discovery-gated generic object engine** | `src/adt/server-driven.ts` | **Reimplement the pattern**, not the registry. Adopt "gate on `/discovery`, not on release number" as a rule across `pkg/adt` |
| **6.5** | **ADT type-availability probe** with independent signals and probe-quality metrics | `src/probe/` | **Reimplement.** Pairs naturally with our existing `vsp rfc probe` as an `adt` sibling. His `docs/research/abap-types/` methodology is the valuable half |
| **6.6** | **Read his `docs/compare/vibing-steampunk/evaluations/`** — 24 evaluations of our commits and issues | `docs/compare/` | **Take as intelligence, today.** Zero cost, and it contains at least one confirmed critical finding about our own code |
| **6.7** | **Rate limiting (three layers) + `Retry-After` honouring** | `src/adt/semaphore.ts`, `src/server/*rate-limit.ts` | **Reimplement**, and start with the SAP-bound semaphore — that is the layer that protects the *SAP system*, not us, and it is ~75 lines |
| **6.8** | **RFC 9728 protected-resource metadata** | ARC-1 #632; also present in his vsp fork per `fork-survey.md` | **Take as a patch** — small, and `fork-survey.md` already judged his vsp-fork implementation correct |
| **6.9** | **AFF JSON-schema validation of written objects** | `src/aff/` + SAP's schemas | **Reimplement**, low priority. Catches malformed writes before SAP does. The schemas are SAP's, not his |
| **6.10** | **Audit sinks + structured audit events** | `src/server/audit.ts`, `sinks/` | **Reimplement** if and when we take vsp multi-user seriously. Not before |
| **6.11** | Skills as product surface (22 SAP skills) | `skills/` | **Ignore as code, copy as strategy.** Skills are client-side and trivially re-authored; the insight is that they are a better growth surface than tool #100 |
| **6.12** | `adt-ls` transport, Eclipse MCP plugin, BTP/XSUAA/multi-target stack | `arc-mcp/adt-ls`, `arc1-adt-abap-mcp-ext`, `xsuaa-auth`, `mcp-hub` | **Ignore for vsp.** See §7 — right for his product, wrong for ours |
| **6.13** | **Machine-checked "road-to-v1" gate ledger** | `open-rfc` `tools/v1_roadmap.mjs` + `conformance/v1-gates.v1.json` | **Reimplement for `open-rfc-go`**, where the release-readiness question is live. Not for vsp |
| **6.14** | Backport anything from `open-rfc` into `open-rfc-go` | — | **Nothing to do.** His `main` has not moved since 2026-08-10 and our provenance baseline is his HEAD (§4) |

---

## 7. Where he went a direction we should not follow

**He optimised for the enterprise buyer, and paid for it in reach.** BTP Cloud Foundry
manifests, MTA descriptors, XSUAA, destination services, Cloud Connector, an approuter,
multi-target destination discovery, DCR signing-key lifecycle — `src/server/` has **eleven
`multi-target-*.ts` files** and a roadmap item (SEC-15, P1, effort L) about where to durably
store an OAuth signing key on Cloud Foundry. That is a lot of engineering that only pays off
if the deployment is BTP. vsp's single-binary, `go install`, laptop-first story is why it has
443 stars to his 166, and dissolving it into a deployment topology would be a bad trade.

**Node + npm, and the supply chain that comes with it.** His commit log carries entries like
`fix(deps): resolve npm audit advisories in transitive dependencies`,
`fix(security): patch and monitor AppRouter dependencies`, and a standing Dependabot/audit/
Trivy/CodeQL apparatus — much of which exists to manage a risk a static Go binary does not
have. Copy his *posture* (SBOM, pinned Actions, a real `SECURITY.md`); do not copy the
premise that made it necessary.

**Depending on a non-redistributable SAP binary.** `adt-ls` is elegant and it is a real
second door, but it requires the user to bring `adt-lsc` under the SAP Developer License,
plus a JVM. For a Go binary whose entire pitch is "one file, six platforms, no SAP SDK",
that is the same trap the NetWeaver RFC SDK is — and avoiding exactly that trap is the reason
`open-rfc-go` exists.

**A permanent 12-tool surface with no expert escape hatch.** His `docs_page/roadmap.md`
records FEAT-73 blocked on the tool-budget ceiling: three drop-in object types he has already
verified live cannot ship because the surface has no room. Our mode system (hyperfocused /
focused / expert) is the better answer — it gets the token economy *and* keeps the long tail
reachable. Keep it.

**Deleting capability to look enterprise-ready.** `fork-survey.md` §5 already noted that his
vsp-fork `main` removed the debugger, git, LSP, WASM compiler, workflow, DSL, reports, help
and install layers. ARC-1's own gap table repeats the judgement — ABAP debugger, AMDP
debugger, Lua scripting, WASM-to-ABAP, call graph, ExecuteABAP all marked "Low". Those are
the parts of vsp nobody else has, and several of them (the debugger over RFC, the RFC server)
are the frontier we are actually on. His verdict is right *for his product* and wrong for
ours.

---

## 8. Trajectory, in one paragraph

Feb 2026: forks vsp. Mar 25: last commit to that fork; same day, starts ARC-1. Apr: 179
commits, BTP OAuth, competitor trackers, and a **research note on RFC** that concludes the
only open-source Node RFC option needs SAP's proprietary C SDK and defers the whole idea. May–
Jun: the org forms — Eclipse plugin, `adt-ls`, `arc-1-lsp`, `mcp-hub`, extension framework,
extracted `xsuaa-auth`. Jul: MCP spec forward-compat, ATC, transport review, DCR key
lifecycle. **Aug 7: creates `open-rfc`** — and solves, in TypeScript and without the SDK, the
exact problem his April note said was blocked. Aug 18: ARC-1 v1.1.0.

The pattern is consistent: **research first, write it down, build the general thing, then
extract it as a package.** 69 dated research documents and 56 plans for 582 commits is close
to one written artifact per four commits. Whatever else is worth taking from him, that ratio
is.
