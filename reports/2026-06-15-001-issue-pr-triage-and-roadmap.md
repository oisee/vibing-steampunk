# 2026-06-15 — Issue / PR Triage & "What to do next"

Full sweep of GitHub issues, pull requests, the offline bug report in
`../feedback-vsp/MCP_BUG_REPORT/`, and a user support chat. Identifiers
from the chat and the offline report are sanitized per the CLAUDE.md
policy (real username → `TESTUSER`, live CR IDs → `TR-EXAMPLE`).

## Snapshot

| Bucket | Count |
|--------|-------|
| Open issues | 31 |
| Closed issues | 30 |
| Open PRs | 11 |
| Merged PRs | 18 |
| Closed-unmerged PRs | ~25 |

The community has become the primary source of bug reports and fixes.
Many open PRs and several issues come with working fork patches and
tests. The bottleneck is now **review/merge throughput and dedup**, not
diagnosis.

---

## The 4 things that actually matter next

1. **Finish the write-path / lock bug class** — it is 80% done on `main`
   but the last 20% (#132, #135, transport-owned objects) is what
   real users still hit daily. (Cluster A)
2. **Land INCL (program-include) write support** — 3 competing PRs,
   pick one. (Cluster B)
3. **Land activation-error detection** — the server currently reports
   `success:true` on objects that failed to activate. This is a
   correctness/trust bug. (Cluster C)
4. **Decide the anonymization story** — a real enterprise user cannot
   use vsp at all until table column names can be masked on the way to
   the LLM and unmasked on return. Strategic, not yet filed. (Cluster M)

Everything else is important but slots behind these.

---

## Cluster A — Write-path / lock-handle bug class  ★ TOP PRIORITY

The single largest theme across the whole tracker. Same symptom family:
`LockObject` succeeds, the following `PUT/POST` is rejected with `423
ExceptionResourceInvalidLockHandle` or `400 Parameter corrNr could not
be found`, leaving orphan objects and hanging method-locks.

**Issues:** #88, #91, #92, #98, #110, #132, #135, #105 (open);
#78, #81, #17, #56 (closed/resolved).

**Already landed on `main`:**
- `22517d4` — `Stateful:true` on all lock-consuming `crud.go` funcs +
  `MODIFICATION_SUPPORT=NoModification` guard at lock time (closes the
  #88/#91/#92/#98 mechanism).
- `3d1353e` — reconcile partial-create on 5xx.
- `f00356a` / `1b05441` — `RecoverFailedCreate` primitive + CLI wrapper
  (the cleanup tool the offline report explicitly asked for).
- The offline `PR_DESCRIPTION_issue_88.md` fix (atomic rename + SyntaxCheck-
  before-lock + corrNr fallback) overlaps heavily with what landed —
  confirm parity, then thank the reporter.

**Still open / next steps:**
- **#132** "LockObject rejects editable objects already in open
  transport (regression from #91)" and **#135** "Parameter corrNr could
  not be found" are the *current* live pain. Root cause (per txape10's
  fork analysis): the `corrNr` from `LockResult` is not forwarded into
  the subsequent PUT, so on transport-owned objects SAP can't find the
  request. Fix = adopt `lockResult.CorrNr` when the caller supplies no
  transport (the `fallbackTransport` helper from the offline PR).
- **PR #125** (dme007) "skip redundant mutation gate after lock to keep
  stateful session alive" — directly relevant; the `mutationGateSkipKey`
  flag. **Review & merge** — txape10 says merging it unblocks the
  community.
- **PR #108** (dme007) "deploy session ordering + correct
  MODIFICATION_SUPPORT handling" — overlaps with what landed in
  `22517d4`; **reconcile and merge/close**.
- **#110** is the v2.32→v2.36 regression report for the same 423 — verify
  it's covered by `22517d4` and close on confirmation.

**Action:** (a) post on #88/#91/#92/#98/#110 asking reporters to confirm
on a `main` build, close on confirmation; (b) implement corrNr-forward
for #132/#135; (c) review/merge #125, reconcile #108.

---

## Cluster B — INCL (program include) write support  ★ HIGH

WriteSource/EditSource don't support program includes (PROG/I).

**Issues:** #116, #133 (closed). **PRs (3, overlapping — must dedup):**
- **#121** frd1201 — "add INCL (PROG/I) write support for WriteSource,
  EditSource, CLI" (broadest scope)
- **#139** enricoandreoli — "treat program includes as source-bearing
  objects, not class includes"
- **#134** txape10 — closed, "INCL fail before reaching SAP"

**Action:** evaluate #121 vs #139, pick the more complete one as the
base, cherry-pick anything unique from the other, merge. Closes #116,
#133, and unblocks part of #135.

---

## Cluster C — Activation / syntax-error silently ignored  ★ HIGH (correctness)

The server returns `success:true` even when SAP rejected activation —
a trust-breaking correctness bug. txape10 has fully-diagnosed fork
fixes **with tests and captured live XML**.

**Issues:** #136 (parseActivationResult/parseSyntaxCheckResults ignore
errors due to XML namespace mismatch + S/4HANA `<chkl:messages>` format),
#137 (ActivateMultiple — dependency-aware batch activation like Eclipse).

**Action:** these are the highest-quality external contributions in the
tracker (root cause + fix commits + tests + on-prem verification). Port
`stripXMLNamespaces` + the dual-format `parseActivationResult` (#136)
first — it's a pure correctness fix — then `ActivateMultiple` (#137).
Ask txape10 to open PRs against `main` or port directly with attribution.

---

## Cluster D — Authentication & connectivity  ★ HIGH

Broadest surface; blocks first-run for many users.

- **Client / mandant selection (NOT YET FILED — from support chat):** a
  user reported the client always connects to `001` regardless of config
  when they need `500`. No issue exists. **File one and reproduce** —
  this is a silent first-run blocker that cost a real user their whole
  token budget. Likely a missing `sap-client` query param / `?sap-client=`
  on the ADT URL or env precedence bug.
- **#104** CSRF 403 (HEAD not supported by `CL_ADT_WB_RES_APP`) +
  **PR #120** (frd1201) "CSRF HEAD→GET fallback + secure-cookie +
  `SAP_SESSION_TYPE` env" — **review/merge**, closes #104, helps cloud.
- **#90** BTP Basic Auth 401 on abap-web domain, **#99** OAuth2 for BTP
  ABAP Environment — the next access-control thread after SAML (#97
  merged). #99 needs design.
- **#112** MCP doesn't reload cookies from `--cookie-file` after external
  refresh; **PR #128** (andreasmuenster) browser-auth client fix.
- **#103** SAProuter access; **PR #107** (dme007) proxy env for WebSocket.

**Action:** merge #120 (CSRF) and #107 (proxy) — both low-risk; file +
fix the mandant bug; queue #99/#90 (BTP OAuth2) as a design item.

---

## Cluster E — System / HANA detection

- **#114** HANA detection still fails on S/4HANA 758 — COMPONENTS
  endpoint returns 406, probe never reaches the S4CORE fallback added in
  merged PR #100. **Follow-up to #100; fix the 406 Accept header path.**

---

## Cluster F — Transport API gaps

- **#140** ListTransports wildcard user `*` doesn't return other users'
  transports (newest issue, 2026-06-15).
- **#111** `get_user_transports` returns empty while `list_transports`
  returns the same data (follow-up to closed #9).
- Offline report: `get_user_transports` is **blocked by safety config**
  (`type X` operation) — users can't see their own locked requests
  programmatically, forcing manual SE09. Consider allowing it read-only.

**Action:** treat #111 + #140 + the safety-block as one transport-API
cleanup pass.

---

## Cluster G — RunReport / APC (architectural)

- **#113** (open) / **#115** (closed dup) runReport times out in
  WebSocket/APC — REPORT_SERVICE uses sync SUBMIT. **#55** all spool-
  reading blocked in APC. **#7** (closed) re-add ALV capture.

**Action:** architectural limit (noted in CLAUDE.md priorities). Decide:
document the limitation on #113/#55 and keep as tracking, or invest in
async SUBMIT. Recommend **docs-close #55/#113** with the ZADT_VSP
requirement called out, unless this becomes a frequent ask.

---

## Cluster H — "Warnings block edits" gate

- **#131** allow opting out globally (flag/env/config), not only
  per-call. Builds on merged #36 (`ignore_warnings` param) / closed #33.

**Action:** **low effort, high satisfaction.** Add `--allow-warnings` /
`SAP_IGNORE_WARNINGS` global default. Pairs with #117.

---

## Cluster I — New object-type / metadata support

- **#109** create Domains & Data Elements (feature req)
- **#74** CDS metadata extensions (DDLX/EX)
- **#27** broader object-type support; **#123** GetRevisions 404 for
  INTF/DDLS despite schema claiming support; **#94** enable features via
  zadt custom API.
- **PR #130** (barkow15) ENHO (Enhancement Framework) read support.

**Action:** merge #130 (additive read). Batch #109/#74/#27 into a
"DDIC/metadata write" roadmap item. Fix #123 (schema lies about support).

---

## Cluster J — Install / bootstrap robustness

- **PR #138** (blicksten) InstallZADTVSP deploys real source, not empty
  shells; **PR #106** (dme007) propagate Description, detect pre-existing
  package, stop silent success. Closed history: #87, #75, #54, #51.

**Action:** **#138 is important** — empty-shell deploys mean ZADT_VSP
features silently don't work. Review/merge #138 and #106 together.

---

## Cluster K — CLI / search / flags

- **#119** bug in `vsp search`; **PR #126** (frd1201) server-side type
  filter so `--max` applies after `--type` — **merge, closes #119**.
- **#117** `--allow-transportable-edits` advertised but rejected as
  unknown in 2.38.1 — flag/doc drift. **#118** usage issues with Kiro +
  steampunk (UX/docs).

---

## Cluster L — Sync script (low effort)

- **#45** auto-resolve CLAUDE.md/README.md conflicts; **#46** fix oisee
  references in markdown. Batch together when convenient. Note closed
  PR #129 was a sync attempt.

---

## Cluster M — Data anonymization / column masking  ★ STRATEGIC (NEW)

From the support chat: an enterprise security team mandated that **table
column names be masked before data is sent to the LLM and unmasked on
the way back**, on the fly — without it they are forbidden from using
vsp at all. The user (an active, enthusiastic adopter) is blocked.

This is not yet a GitHub issue. It is a **gating requirement for
enterprise adoption** and a genuine differentiator.

**Action:** file an issue and design a masking layer:
- deterministic, reversible field/column-name mapping per session
- applied at the GetTableContents / read boundary and reversed on
  responses referencing those names
- opt-in via config (`--mask-schema` / mapping file)
- consider extending to data values, not just identifiers, later.

Recommend a short design report before coding — this touches every
read path.

---

## Cluster N — Strategic / in-flight (already on the roadmap)

- **#2** GUI debugger (design report 001), graph engine (`pkg/graph/`).
  No change to priority; continue as planned.

---

## Abandoned / closed-unmerged PRs worth a decision

A large batch of feature PRs were opened and closed unmerged. Decide
explicitly whether to revive or formally decline so contributors aren't
left guessing:

- **blicksten** #82 (refactoring/quick-fix), #84 (testing/quality), #85
  (CDS impact), #86 (intelligence layer), #89 (AnalyzeABAPCode v2,
  abaplint) — significant feature work, all closed unmerged. #89 overlaps
  the existing `pkg/abaplint/`; worth reconsidering.
- **marianfoo** #62 (readonly mode), #63 (MkDocs site), #65 (Docker/GHCR),
  #66 (integration-test infra) — infra/docs; #62 readonly mode partly
  superseded by `--read-only`.
- **Prolls** #41/#42 (gCTS + i18n) → issues #39/#40 still open in spirit.
- **berndeplo** #48/#49 (JCo RFC sidecar) — architectural; ties to #103.

**Action:** triage this list once; either reopen with a rebase request
or close the parent issues with a rationale.

---

## Recommended execution order

**Sprint 1 (correctness & merges — mostly review throughput):**
1. Confirm + close #88/#91/#92/#98/#110 (verify `main` build).
2. Implement corrNr-forward → close #132/#135.
3. Merge #125 (mutation-gate skip); reconcile #108.
4. Port #136 (activation error detection) — pure correctness.
5. Merge #126 (search), #120 (CSRF), #107 (proxy) — low-risk.

**Sprint 2 (capability):**
6. Land INCL write (pick #121 vs #139) → #116/#133.
7. Port #137 (ActivateMultiple).
8. Merge #138 + #106 (install real source).
9. Fix #114 (HANA 758 detect), #123 (GetRevisions), #111/#140 (transport).
10. Add global warnings-opt-out (#131) + fix #117 flag drift.

**Sprint 3 (strategic):**
11. File + reproduce the mandant/client bug (chat).
12. Design + prototype the anonymization layer (#M).
13. BTP OAuth2 design (#99/#90).
14. Decide GUI debugger / graph engine continuation.

**Whenever convenient:** #45/#46 sync script; triage the abandoned-PR
list.

---

## Cross-cutting note

Several of the best fixes (lock/corrNr, INCL, activation) exist as
**fork patches with tests** but aren't merged. The highest-leverage
single action is a **review-and-merge pass** on the open PRs — most of
the diagnosis work is already done by the community.
