# Spike: vsp integration tests in CI against open-steamgate (OSD / OSGo)

**Date:** 2026-10-01
**Branch:** `spike/osgo-ci`
**Question:** can CI download the latest open-steamgate release, start it, and
run vsp's read / write / lock / version / dump / impact / callgraph tests
against it, so that the same scenarios can later be diffed against A4H?

**Short answer:** yes for the OSD release binary, and the job is drafted
(`.github/workflows/osd-integration.yml`, advisory). Against
OSD `vscode-v0.4.1444`, 18 of 57 integration tests pass and 1 more passes
vacuously. Most of the rest fail for two reasons. Either OSD does not serve
the route (16), or the test reads an SAP-standard object that OSD does not
ship (7). Five are real behaviour differences worth triage, and the lock
session model is the first of them. OSGo, the Go build, has no ADT yet. The
job carries it as a disabled smoke row that turns on by itself once it does.

## Update 2026-10-02: both pins move to `vscode-v0.5.1486`

dell released `vscode-v0.5.1486`. It is the first tag with OSGo binaries
(`osgo-linux-x64`, `-linux-arm64`, `-darwin-arm64`, `-windows-x64.exe`). Its
`osd` carries the ADT fixes this spike asked for. Both `.github/ci/osd.version`
and `.github/ci/osgo.version` now pin it. All 8 `osd-*` and `osgo-*` assets
were downloaded and pass `sha256sum -c`. The branch was merged with
`origin/main` (`1b917ee`) first. `VSP_TEST_PACKAGE` and `VSP_TEST_TIMEOUT`
still work; no new hard-coded `$TMP` arrived.

**OSD suite (`vscode-v0.5.1486`):**

| | Count |
|---|---|
| Tests | 57 |
| pass | 22 |
| vacuous-pass | 1 |
| missing-endpoint | 14 |
| missing-object | 7 |
| different-answer | 3 |
| environment | 2 |
| skipped | 8 |

On `vscode-v0.4.1444` the same suite had 18 pass, 16 missing-endpoint and
5 different-answer. Same env as before (`$ZOSD_TEST_SRC`, `120s`), fresh
`XDG_DATA_HOME`, `HOME` and `STG_DB_PATH`. The log's `system home` line
pointed into scratch.

Four tests flipped to **pass**:

| Test | Was | Fix |
|---|---|---|
| StatelessRequestInsideAnotherCallersLockWindow | different-answer (409) | stateless requests keep the session's locks |
| ConcurrentLockChainsAndReaders | different-answer (409) | same |
| ClassWithUnitTests | missing-endpoint | class includes can be created |
| CreateClassWithTests | missing-endpoint | same |

No integration test covers the other fixes, so they were checked with the
hand probe:

- **Package visibility** is fixed. `$ZOSD_TEST_VSPCI` under `$ZOSD_TEST` is
  created (201). `GetPackage` and search then find it, and a program can be
  created in it.
  - Cleanup works dell's way: delete the program, then
    `DELETE /packages/<name>` without a lock. After that, search no longer
    finds the package.
- **Enqueue** is fixed. A second session locking a held object gets **403**
  "User DEVELOPER is currently editing ZVSP_PROBE_…", as A4H does.
- **Database error text** is fixed. The callees refusal now reads
  `no such column: DIRECT` / `no such column: PROG`. The columns themselves
  are still 0.6.

**Still failing on `vscode-v0.5.1486`:**

- **different-answer (3):** each is a vsp assumption or a known fidelity
  limit:
  - CreatePackage hits the `<parent>_<FOLDER>` naming rule;
  - RAP_E2E_OData's CDS view is refused by the abaplint check;
  - Namespace_SearchObject finds no `/DMO/` content.
- **missing-endpoint (14):** code completion, navigation, type hierarchy,
  pretty-printer, BDEF/SRVB, function groups and modules, debugger.
- **missing-object (7):** unchanged.

Activation still costs 25-30 s per call (FindDefinition 28 s,
GetTypeHierarchy 28 s) without warm mode. From the script's start to ready
took 50 s, including the 111 MB download. RSS across both processes was
about 2.7 GB. Warm mode is measured in the next update.

**OSGo smoke (`vscode-v0.5.1486`), run through `OSD_BINARY=osgo .github/ci/osd-up.sh`:**

- **Start:** download, sha256 check, then `-home <fresh> -port`. The whole
  script took 12.6 s, mostly the 39 MB download.
- **Ready:** `GET /health` → `{"status":"ready","version":"vscode-v0.5.1486","commit":"ad753f1d…"}`.
  `-version` prints `osgo vscode-v0.5.1486 (ad753f1d…)`.
- **Cold start to ready:** **453-454 ms** over three fresh homes.
- **RSS:** **59-64 MB**, against about 2.7 GB for OSD.
- **HEAD `/sap/bc/adt/core/discovery`:** **404**, as expected (OData only),
  so the row runs smoke only. `GET /` answers 200, and the ICF services are
  listed in the log.
- **Isolation:** the database is `<home>/osgo.sqlite`, "new: seeded". The
  real `~/.local/share/open-steamgate` was untouched.

The CI OSGo row is now live. It turns into the full suite by itself once
discovery answers 200. The nightly "latest" row now follows any `vscode-v0.*`
tag, not only `v0.4.*`.

## Update 2026-10-02 (2): warm mode, `OSD_WARM=1 STG_DEV=1`

dell's advice: with `OSD_WARM=1 STG_DEV=1`, a content edit of an existing
class or interface activates in about 0.5 s. Creates, PROG edits and
`INTERFACES` changes stay cold, and the `X-OSD-Generation` / `X-OSD-Build`
response headers say which path an activation took. `osd-up.sh` now exports
both for the `osd` binary (default 1, `OSD_WARM=0 STG_DEV=0` restores the
old behaviour), and the workflow sets them explicitly. `osgo` ignores them.

Same suite, same pin (`vscode-v0.5.1486`), same env (`$ZOSD_TEST_SRC`,
`120s`), isolated as before: fresh `XDG_DATA_HOME`, `HOME` and
`STG_DB_PATH` under the scratch dir, and the `osd: system home` line pointed
there. The real `~/.local/share/open-steamgate` was untouched (same listing
and mtimes before and after).

**Results: unchanged.** All 57 tests land in the same class as the cold run:
22 pass, 1 vacuous-pass, 14 missing-endpoint, 7 missing-object,
3 different-answer, 2 environment, 8 skipped. Warm mode is kept.

**Time:**

| | cold | warm | saved |
|---|---|---|---|
| Script start to ready (incl. 111 MB download) | 50 s | 39 s | (network noise) |
| `pkg/adt` integration package | 374.3 s | 330.4 s | **43.9 s (12 %)** |
| EditSource | 111.8 s | 85.0 s | 26.9 s |
| FindDefinition | 28.3 s | 20.5 s | 7.8 s |
| GetTypeHierarchy | 27.6 s | 20.0 s | 7.6 s |
| CreateAndActivateProgram | 27.8 s | 20.4 s | 7.4 s |
| WriteClass | 31.8 s | 25.2 s | 6.6 s |
| CreateClassWithTests | 39.6 s | 35.8 s | 3.8 s |

**Why only 12 %:** no activation in the suite took the warm path. Every
write test creates a fresh object and activates it, which is cold by design.
`osd.log` says so per build: `warm: a cold build: <file> is new` (or
`is gone` after a delete), and once `warm: builds stay cold: the tree is not
the live generation`. The saving comes from the dev loop around the cold
builds (`dev: reused <generation> …, recycled in 1.7-20 s`; one rebuild
skipped the 5.5 s cross-reference seed). To get the 0.5 s path, a test has to
edit a class or interface that already exists in the live generation, such
as the shipped `ZCL_ZOSD_TEST_DEMO`.

**The warm path itself works.** A hand probe on the same instance: lock,
PUT, unlock, activate `ZCL_ZOSD_TEST_DEMO` with a one-line comment change.

| Probe | Activate | Headers |
|---|---|---|
| 1st edit | 0.49 s | `X-OSD-Build: warm`, `X-OSD-Swap-Ms: 2`, `X-OSD-Closure: 2`, `X-OSD-Closure-Tests: ZCL_ZOSD_TEST_DEMO` |
| 2nd edit | 0.50 s | `X-OSD-Build: warm`, no `X-OSD-Swap-Ms`; `osd.log`: "the swap was refused, recycling instead: the runtime carries `<gen A>`, and the swap is from `<gen B>`" |

Both returned `activationExecuted="true"`. One point for dell: on the 2nd
edit the header still says `warm` while the log says the swap was refused
and the runtime recycled in the background.

**Two dev-mode log lines to watch** (no test result changed): "source changed
during build; leaving the new edit inactive for the next pass" (7 times, mostly during
the WriteProgram, WriteClass and EditSource sequences), and a harmless
`fatal: not a git repository` at boot.

**Memory and disk with warm mode:** at rest after the suite, `osd up`
2.6 GB plus `osd serve` 0.8 GB RSS. Sampled every 5 s during the suite, the sum
over all `osd` processes (builds included) peaked at 6.5 GB. The data dir
grew to about 1 GB (the 0.4 home was 147 MB) and the database to 11 MB. Both
fit a standard `ubuntu-latest` runner (16 GB RAM, 14 GB free disk).

## Update 2026-10-02 (3): first run on a GitHub runner (PR #321)

`ubuntu-latest`, `pull_request` event, run 36967594544. All three rows
finished green.

| Row | Download + sha256 | Ready | Suite | Job total |
|---|---|---|---|---|
| osd pinned (`vscode-v0.5.1486`, warm) | `osd-linux-x64: OK` 4 s after the step started | 49 s after the step started; HEAD discovery 200 | 8 m 42 s step (compile included; the top-level tests sum to 508 s) | 10 m 10 s |
| osgo pinned | `osgo-linux-x64: OK` after 1 s | 4 s after the step started; HEAD discovery 404, so smoke only | none | 34 s |
| osd latest | not run (nightly only); the plan step skipped it | | | 4 s |

**Matrix on the runner:** 22 pass, the same 22 as locally. The only difference
is the two `BrowserAuth` tests. The runner ships Chrome, so they run, and they
fail on navigation to an SSO page ("context deadline exceeded" and "websocket
url timeout reached"). They were therefore classed `timeout` and
`different-answer` instead of `environment`. `osd-summary.sh` now classes a
browser-auth navigation failure as `environment`, before the timeout rule.
The runner is about 1.5x slower than the local host (EditSource 123 s,
WriteProgram 48 s; the warm primes took 12-15 s instead of 10 s). No
activation took the warm path, as locally.

Two harmless warnings: `setup-go`'s cache restore hit a tar "File exists"
(exit 2), and GitHub notes that the v4/v5 actions are being forced onto
Node 24.

**Artifacts checked:** `summary.*`, `osd.log` and `build.json`. Their only
paths are OSD's own `/home/runner/work/_temp/osd/...`, and their only URLs
are `localhost`. There is no user, host or A4H identifier.

## Update 2026-10-02 (4): the checksum pins the binary; published output is redacted

Review found two problems (codex critic on PR #321). This update fixes both.

**The checksum did not pin the binary.** `osd-up.sh` used to check the
download only against the `.sha256` from the same release. An asset swapped
together with its checksum would have run.

- `.github/ci/osd.sha256` now commits the expected sha256 of all 8 pinned
  assets: `osd-*` and `osgo-*` for linux-x64, linux-arm64, darwin-arm64 and
  windows-x64. Each line reads `<sha256>  <tag>/<asset>`.
- The values are the release's `.sha256` files. They agree with binaries
  downloaded independently a day earlier.
- `osd-up.sh` requires the download to match the committed value **and** the
  release `.sha256`. It checks both before the binary is copied, chmod-ed or
  run. A mismatch exits 4.
- A tag with no committed line is refused, unless the caller sets
  `OSD_ALLOW_UNPINNED=1`. The workflow sets it only for the nightly "latest"
  row and for a dispatch with an explicit tag.
- An unpinned run is reported `OSD_PINNED=false`, and its summary says
  "(unpinned)". The latest row never runs on push or PR, and it holds no
  secret.
- `GH_TOKEN` and `GITHUB_TOKEN` are unset before the binary starts. This was
  checked locally: the started `osgo` process's environment had neither.

`.github/ci/osd-up-test.sh` is offline and runs in the workflow before every
start. If it fails, the start is skipped. It checks six cases:

| Case | Expected |
|---|---|
| control: a temp copy of the pin file whose line matches a fake asset | 0, `OSD_PINNED=true` |
| that copy with one hex digit of the committed hash edited | 4, binary not installed |
| the real committed pin against the fake asset | 4 |
| asset and release `.sha256` disagree | 4 |
| no committed line | 4 |
| no committed line, with `OSD_ALLOW_UNPINNED=1` | 0, `OSD_PINNED=false` |

A mutation check confirmed the test catches a removed pin: with the
committed-hash comparison disabled, both pin cases fail.

**Local paths and SAP details could leak.** `.github/ci/osd-redact.sh`
makes three replacements:

- the work dir becomes `<work>`;
- `$HOME` becomes `<home>`;
- the host of `$SAP_URL` becomes `<sap-host>`, unless it is a loopback host.

`osd-summary.sh` passes `summary.json` (and `summary.md`, which is built
from it) through the filter. The workflow does the same for the artifact
copies of `osd.log` and `build.json`, and for the log tail it prints when a
start fails.

**The summary is meant for OSD runs only.** Its reasons quote the target's
own messages. The redaction removes paths and the host, not object names or
message text. A summary of a run against A4H or another real system must not
be published.

## Update: 0.6.1504

dell released `vscode-v0.6.1504` (verified by content on their side). Both
`.github/ci/osd.version` and `.github/ci/osgo.version` now pin it, and
`.github/ci/osd.sha256` holds the new line for each of the 8 assets (`osd-*` and
`osgo-*` for linux-x64, linux-arm64, darwin-arm64 and windows-x64.exe).
Each asset was downloaded with `gh release download`, checked against its
release `.sha256`, and hashed again locally before its line was written.
`osd-up-test.sh` passes all six pin cases against the new pin.

All local runs went through `osd-up.sh`, with a fresh `XDG_DATA_HOME`, `HOME`
and `STG_DB_PATH` each time. The env was the same as before (`$ZOSD_TEST_SRC`,
`120s`). The real `~/.local/share/open-steamgate` was not touched (same
listing and mtimes before and after). The host was shared and under load
(load average 4-7 on 16 cores), so read the times as relative, not absolute.

**Matrix: unchanged.** 57 tests: 22 pass, 1 vacuous-pass, 14 missing-endpoint,
7 missing-object, 3 different-answer, 2 environment, 8 skipped. Every test
lands in the same class as on `vscode-v0.5.1486`. That holds for a local
0.5.1486 run on the same host today, and for the runner's 0.5.1486 matrix
(push to main, run 36974044881). That runner matrix reads 24/57 pass. The 2 extra
passes are the `BrowserAuth` tests: the runner ships Chrome, and this host
does not, so locally they are `environment`. On the runner, expect 24/57
again.

**Swap refused: 0.** None of the five 0.6 logs (two warm suites, one cold
suite, the probe below, the osgo smoke) has a "swap was refused" line. The
suite proves little here, though, because no activation in it takes the
warm path (0 `swapped in` lines), as on 0.5.1486. The hand probe from
update (2) is the real check. It ran on a fresh warm instance: three
back-to-back edits of `ZCL_ZOSD_TEST_DEMO` (lock, PUT, unlock, activate).

| Edit | Activate | `osd.log` |
|---|---|---|
| 1st | 200, 31 ms, `X-OSD-Build: warm` | `dev: warm, 2 objects … in 624 ms, swapped in 2 ms` |
| 2nd | 200, 29 ms, `X-OSD-Build: warm` | `… in 790 ms, swapped in 1 ms` (on 0.5.1486 this one was refused) |
| 3rd | 200, 27 ms, `X-OSD-Build: warm` | `… in 507 ms, swapped in 1 ms` |

All three returned `activationExecuted="true"`. dell's fix #440 holds. Two
things have changed:

- The activate response now returns in about 30 ms. The warm build and the
  swap land 0.5-0.8 s after the change, and `X-OSD-Swap-Ms` is no longer sent.
- One new line: "warm: `<gen>` not verified: inconclusive the tree changed
  while it was compared" (the edits were 3 s apart).

**Warm vs cold: on this release, warm is slower for the suite.** The timing
is per test, from `go test -json`:

| | 0.6 warm (run 1) | 0.6 warm (run 2) | 0.6 cold | 0.5.1486 warm (same host, today) |
|---|---|---|---|---|
| Script start to ready (local asset, no download) | 43.0 s | 43.4 s | 29.7 s | 40.3 s |
| `pkg/adt` integration package | 632.8 s | 652.7 s | **384.2 s** | 398.9 s |
| EditSource | 165.5 s | 178.0 s | 121.1 s | 87.3 s |
| WriteProgram | 68.5 s | 61.5 s | 35.6 s | 41.0 s |
| WriteClass | 50.6 s | 73.8 s | 33.0 s | 24.6 s |
| FindDefinition | 44.3 s | 48.0 s | 29.5 s | 20.3 s |
| GetTypeHierarchy | 37.4 s | 51.8 s | 29.5 s | 25.4 s |
| RAP_E2E_OData | 21.6 s | 22.2 s | 4.8 s | 4.9 s |
| RSS after the suite (`osd up` + children) | 3.6 GB | 3.7 GB | 2.6 GB | 4.8 GB |

On 0.6.1504, warm mode costs about 1.65x the cold time (two runs agree within
3 %). On 0.5.1486, warm and cold were about level. The difference shows in
the log. In warm mode every cold build is followed by an 11-17 s
`warm: primed 1133-1138 files` pass. The rebuilds then come out as
`dev: reused <gen> (? objects, 30 ms), nothing serving to recycle`: 9 and 12
times in the two 0.6 warm runs, against 2 on 0.5.1486 warm and 0 cold. The
test client waits through each of them. No test changed class, so this is
cost only. A question for dell: is "nothing serving to recycle" after a
reuse expected while a prime is running? The workflow keeps
`OSD_WARM=1 STG_DEV=1` for now, because the warm path itself (the probe) is
what dell asked to exercise. If the runner shows the same slowdown,
`OSD_WARM=0` is a one-line change.

**OSGo smoke (`vscode-v0.6.1504`):** `OSD_BINARY=osgo osd-up.sh`, 3.1 s
from start to ready (local asset). `GET /health` →
`{"status":"ready","version":"vscode-v0.6.1504","commit":"66c6dba3…"}`.
From a cold start, it is ready in 451-484 ms over three fresh homes, with an
RSS of 66 MB. **HEAD `/sap/bc/adt/core/discovery` is still 404** (GET too,
and `/sap/bc/adt/discovery` as well). The log lists ICF services, push
channels and OData, and no ADT. The osgo row therefore stays a smoke check.
It switches to the full suite on its own once the start step writes
`OSD_ADT=200` to `$GITHUB_ENV`, because the test step is gated on
`env.OSD_ADT == '200'`. Nothing in the workflow needed to change for that.

Sections 1-8 below are the original 2026-10-01 write-up against
`vscode-v0.4.1444`, kept as the baseline. Section 8's answers marked
"0.4.x" shipped in `vscode-v0.5.1486`.

## 1. What OSGO is, and which binary to run

"OSGO" covers two artefacts from one repository, `oisee/open-steamgate` (public,
MIT; local checkouts at `~/dev/osg-research` and `~/dev/open-steamgate`):

| | OSD | OSGo |
|---|---|---|
| What | The whole system as one Bun binary: ICF, Gateway, apps, SQLite, **ADT façade** (`tools/adt-facade.mjs`) | The same system compiled to Go by `gogen` (`tools/gogen/go/cmd/osgo`) |
| Release assets | Every `vscode-v0.4.*` tag: `osd-linux-x64`, `osd-linux-arm64`, `osd-darwin-arm64`, `osd-windows-x64.exe`, each with `.sha256` (checked for 1413, 1414, 1444) | None today. From the first `vscode-v0.5.N` tag (dell will name it): `osgo-linux-x64`, `osgo-linux-arm64`, `osgo-darwin-arm64`, `osgo-windows-x64.exe`, static (CGO off, pure-Go SQLite), each with `.sha256` |
| ADT | Yes, under `/sap/bc/adt/` | No, OData only. HEAD `/sap/bc/adt/core/discovery` is not 200. The ADT façade is moving to ABAP compiled to Go and JS (ADR 0007) |
| Start | `osd up` | `./osgo-linux-x64 -home "$RUNNER_TEMP/osgo-home" -port 3095`; a fresh `-home` is a full reset |
| Ready | `GET /sap/bc/adt/core/http/build`, `serving == live` | `GET /health` until `status` is `ready` (about 0.55 s). Not the build stamp |

**Decision (stoker, osg-research and dell, relayed):** the full suite runs on
the **OSD release binary, pinned to exactly `vscode-v0.4.1444`**, never
"latest". The nightly job also tries the newest `vscode-v0.4.*` tag, as
advisory only. OSGo is the preferred long-term target, because it is native
and faster, but it has no ADT to test until 0.6. Until then it is a smoke row
behind an empty pin (`.github/ci/osgo.version`). The spike did not build OSGo
from source, on the coordinator's instruction. The build needs node,
`npm ci`, `osd-libs` and `osgo.mjs` over all 822 classes, run under OSG's
shared lock, which is too heavy for a CI pre-step when 0.5 will ship a binary.

### Running OSD headless (verified locally)

```
gh release download vscode-v0.4.1444 -R oisee/open-steamgate -p 'osd-linux-x64*'
sha256sum -c osd-linux-x64.sha256 && mv osd-linux-x64 osd && chmod +x osd
XDG_DATA_HOME=$W/xdg HOME=$W/home STG_DB_PATH=$W/db/osd.sqlite STG_PORT=3030 ./osd up
```

The CI job needs no node, no `npm ci` and no `osd-libs`. The binary embeds its
runtime and every pinned dependency. `.github/ci/osd-up.sh <workdir>` does all
of the above and the readiness wait, and it runs the same locally.

| Setting | Value | Source |
|---|---|---|
| URL | `http://localhost:$STG_PORT` (default 3030), ADT under `/sap/bc/adt/` | binary, osg-research |
| User / password | Any Basic user and password are accepted, or none. The façade reports identity `DEVELOPER` | dell; verified |
| Client | **`001`**, or no `sap-client` at all. **Never `123`**: that is only the runtime's `sy-mandt`, where the seed rows live | dell |
| SID | `OS2` (override with `STG_ADT_SID`) | osg-research; `build.json` |
| Ready | `GET /sap/bc/adt/core/http/build` answers **and** `system.serving == system.live`, non-null. The stamp answers earlier, with `serving: null`, so "it answers" alone is too early | verified |
| Reset | A **fresh `XDG_DATA_HOME`**, a fresh `STG_DB_PATH` and a fresh process. A new DB alone is not enough: on first run the seed unpacks into `$XDG_DATA_HOME/open-steamgate/osd-home-<seed>/`, ADT writes land there as abapGit files, and builds go to `<home>/build/by-input/<hash>` | dell; verified |
| Writable package | **`$ZOSD_TEST_SRC`**. There is no `$TMP` ("DEVC $TMP does not exist"). A new package must be named `<parent>_<FOLDER>` | verified |
| Timing (cold, this host) | 121 s and 178 s from download to ready (110 MB download, seed unpack, full transpile). On the first run, with the binary already on disk, the stamp answered in 25 s and serving followed later | measured |
| Memory | ~1.85 GB RSS: `osd up` (the supervisor/builder) 1.48 GB plus `osd serve` 0.38 GB | measured |
| Disk | 147 MB home, 13 MB database | measured |

A note on isolation. My very first `osd doctor` run unpacked a 75 MB home into
the real `~/.local/share/open-steamgate`, although `XDG_DATA_HOME` was set on
the same command line. I removed it at once (it was 10 s old and nothing else
was in it). dell could not reproduce this either. Their reading is that it was
an `osd-home-<seedId>` from a start without `XDG_DATA_HOME`; `doctor` now
prints the resolved directory (§8, question 1). `osd-up.sh` also sets `HOME`
into the work dir, and the `osd: system home <path>` line at the top of
`osd.log` shows where a run lives.

## 2. How the run was made

- A worktree on `origin/main` (`9789f00`) and OSD `vscode-v0.4.1444`, started
  with `osd-up.sh` on fresh state (generation `1299cac0540dd93f`).
- Env: `SAP_URL=http://localhost:<port> SAP_USER=DEVELOPER SAP_PASSWORD=osd SAP_CLIENT=001
  VSP_TEST_PACKAGE=$ZOSD_TEST_SRC VSP_TEST_TIMEOUT=120s`.
- `go test -tags=integration -json ./pkg/adt/ ./pkg/ctxcomp/ ./pkg/saprfc/`.
  These are every package with integration-tagged files. The tag also builds
  the 1 400+ unit tests, and they all pass. `osd-summary.sh` counts only the
  57 top-level tests defined in integration-tagged files.
- No A4H credentials, no `.mcp.json`, no A4H contact.

### Two minimal changes to the tests

Both are opt-in; with neither variable set the tests behave exactly as before
against A4H.

1. **`VSP_TEST_PACKAGE`**: `integrationPackage()` replaces the 13 hard-coded
   `"$TMP"` in the write tests. The function-module tests already read this
   variable. Without it, every create test fails on OSD before it reaches the
   behaviour under test (first run: 35 fail, 14 pass).
2. **`VSP_TEST_TIMEOUT`**: overrides the integration client's fixed 30 s.
   Every OSD activation rebuilds and reboots the runtime, which takes 25-30 s
   (the log shows a fresh `boot: loading the generation` plus `cross-reference`
   per activation). `WriteProgram` and `CreateAndActivateProgram` timed out at
   36 s and 30 s. With 120 s both pass.

## 3. The capability matrix (OSD vscode-v0.4.1444)

**57 tests: 18 pass, 1 vacuous-pass, 16 missing-endpoint, 7 missing-object,
5 different-answer, 2 environment, 8 skipped.**

Classes (`osd-summary.sh`):

- `missing-endpoint`: OSD said "`<path>` is not served by OSD".
- `missing-object`: "`<TYPE> <NAME>` does not exist", for an SAP-standard
  fixture.
- `different-answer`: OSD answered, and the answer was not what the test
  expects.
- `vacuous-pass`: the test passed while tolerating a missing route.
- `timeout`, `environment`, `skipped`: as named.

Following dell's ask, a missing route is kept apart from a different answer.

**Who owns each failure:** "OSD gap" means a missing or different OSD
behaviour. "vsp assumption" means the test assumes SAP-standard content or
names. "env" means the runner lacks something.

| Test | Result | Class | Owner | Reason |
|---|---|---|---|---|
| **Read** | | | | |
| SearchObject | pass | | | |
| GetClass | pass | | | |
| GetTable, GetTableContents, …WithQuery, RunQuery | pass ×4 | | | T000 exists in OSD's DDIC |
| GetProgram | skip | missing-object | vsp assumption | SAPMSSY0 / RS_ABAP_SOURCE_SCAN not shipped |
| GetPackage | skip | missing-object | vsp assumption | DEVC BASIS |
| GetDDLS | fail | missing-object | vsp assumption | I_ABAPPACKAGE |
| GetCDSDependencies | skip | missing-endpoint | OSD gap | `testcodegen/dependencies/doubledata` |
| GetBDEF, GetSRVB, GetSource_RAP | fail ×3 | missing-endpoint | OSD gap | no BDEF / SRVB routes |
| Namespace_GetSource_{Class,Interface,Program,DDLS}, RoundTrip | fail ×5 | missing-object | vsp assumption | `/UI5/`, `/DMO/` objects not shipped |
| Namespace_GetSource_BDEF, ExportToFile, GetSource_Function | fail ×3 | missing-endpoint | OSD gap | BDEF; function modules under a group |
| Namespace_SearchObject | fail | different-answer | vsp assumption | `/DMO/*` finds nothing (no /DMO/ content) |
| Namespace_ParseFilename, URLEncoding | pass ×2 | | | offline |
| ctxcomp AnalyzerLive | fail | missing-object | vsp assumption | ZCL_ABAPGIT_AJSON (OSD ships ZCL_AJSON) |
| ctxcomp BenchmarkLive | pass | | | 18 abapGit classes found and analysed |
| **Write / CRUD / activate** | | | | |
| CRUD_FullWorkflow | pass | | | create → lock → write → unlock → activate → read → delete |
| EditSource | pass | | | 106 s: four activations |
| WriteProgram, WriteClass, CreateAndActivateProgram | pass ×3 | | | with `VSP_TEST_TIMEOUT=120s` |
| SyntaxCheck, SyntaxCheckWithErrors | pass ×2 | | | abaplint, not the SAP compiler |
| RunUnitTests | pass | | | |
| ClassWithUnitTests, CreateClassWithTests | fail ×2 | missing-endpoint | OSD gap | POST `oo/classes/{n}/includes` (creating the test-classes include) |
| CreatePackage | fail | different-answer | vsp assumption / OSD rule | 501: a package under P must be named `P_<FOLDER>` |
| CreateFunctionGroupThenRFCModule | fail | missing-endpoint | OSD gap | FUGR create/lock |
| CreateRFCFunctionModule | skip | environment | env | needs `VSP_TEST_FUGR` |
| RAP_E2E_OData | fail | different-answer | OSD fidelity + fixtures | vsp's pre-save syntax check (abaplint on OSD) rejects the CDS view: "Source has syntax errors - not saved" |
| **Lock** | | | | |
| LockUnlock | skip | missing-object | vsp assumption | locks SAPMSSY0 |
| StatelessRequestInsideAnotherCallersLockWindow | fail | **different-answer** | **OSD gap, triage first** | 409 "lock handle … does not hold this object in this session" after one stateless read between LOCK and PUT |
| ConcurrentLockChainsAndReaders | fail | **different-answer** | **OSD gap, same cause** | every write in both chains: same 409 |
| **Code intelligence** | | | | |
| CodeCompletion, FindDefinition, GetTypeHierarchy, PrettyPrint, GetPrettyPrinterSettings | fail ×5 | missing-endpoint | OSD gap | `codecompletion`, `navigation/target`, `typehierarchy`, `prettyprinter` |
| FindReferences | skip | missing-endpoint | OSD gap | `usageReferences`: the test skips on any error |
| **Debugger / RFC** | | | | |
| DebuggerListener | pass | (vacuous) | | logs the errors and passes |
| StatelessClientRefusesDebugCallsWithoutASession | vacuous-pass | | | `/sap/bc/adt/debugger` not served |
| ExternalBreakpoints | skip | missing-endpoint | OSD gap (by design) | |
| saprfc Conformance_Debugger, StepCost | fail ×2 | missing-endpoint | OSD gap (by design) | `debugger/breakpoints` |
| saprfc Conformance_Trace | skip | environment | env | no RFC listener on the ADT port |
| BrowserAuth ×2 | fail | environment | env | no `google-chrome` on the runner; out of scope for OSD |

OSD logs each miss itself (`ADT miss (resource|object): <METHOD> <path>` in
`osd.log`, uploaded with the matrix). That log is the authoritative list of
route gaps for one run.

### Scenarios the suite does not cover yet (probed by hand)

vsp has **no** integration tests for versions, dumps, impact or callgraph. A
throwaway probe (not committed) ran the client methods against OSD:

| Scenario | vsp call | OSD answer | Verdict |
|---|---|---|---|
| Versions, existing class | `GetRevisions("CLAS", "ZCL_AJSON")` | one entry `00000`, URI `…/includes/main/versions/19700101101123/00000/content`; `GetRevisionSource` gives 27 KB | works. The fixed `19700101101123` segment is stoker's design. Entry 00000 is the working tree; git commits are 00001..n |
| Versions after write and activate | `GetRevisions("PROG", new)` | still only `00000` | **different from A4H by design**: A4H adds a version on activation, OSD only on a git commit. The diff must say this, not flag it |
| Dumps | `Dumps(last 24 h)` | `[]`, no error | empty feed by design. `/osd/dumps` is a separate API |
| Callees / callgraph (callees) | `Callees`, `CallGraph(callees)` | error: "SAP refused the query" with an **empty message** | **OSD gap**: OSD's `CROSS` lacks `PROG` and `WBCROSSGT` lacks `DIRECT`, so vsp's freestyle `SELECT … PROG FROM CROSS` / `… DIRECT FROM WBCROSSGT` is refused. The refusal (`ExceptionResourceWrongData`) carries no text. The tables exist (CROSS 229, WBCROSSGT 4337 rows) |
| Callers / where-used / impact | `CallGraph(callers)`, `WhereUsed` | 404 `usageReferences is not served by OSD` | missing-endpoint. OSD has only its own `core/http/xref/readers` and `xref/closure` |
| Two sessions lock one object | `LockObject` from clients A and B | **both get a handle** | **OSD gap**: no enqueue (documented: "session-bound local locks"). A4H refuses B |
| Create a package | `$ZOSD_TEST_VSPCI` under `$ZOSD_TEST` | 201 and a `package.devc.xml` on disk, but `nodestructure` and search then say it does not exist | **OSD gap**: the created package is not visible |
| Lock or delete a package | LOCK `/packages/{n}` | 404 not served | missing-endpoint, so the package cannot be cleaned up over ADT |
| Lock handle | any LOCK | `ModificationSupport: NoModification` even on a writable object | worth checking against A4H |

## 4. The CI job (draft)

Files:

- `.github/workflows/osd-integration.yml`: a separate workflow, so `ci.yml`'s
  gate stays untouched. `continue-on-error: true` and a 45-minute timeout.
- `.github/ci/osd.version`: `vscode-v0.4.1444` at first; `vscode-v0.5.1486` since 2026-10-02.
- `.github/ci/osgo.version`: empty, which disables the OSGo row.
- `.github/ci/osd-up.sh`: download → verify the sha256 → start on throwaway
  `XDG_DATA_HOME`/`HOME`/`STG_DB_PATH` → wait for ready → HEAD discovery.
  Exit 3 means the asset is unavailable and the row is skipped with a notice.
  The target is chosen by binary name only (`OSD_BINARY=osd|osgo`), as dell
  asked; there are no OSGo-specific flags.
- `.github/ci/osd-summary.sh`: `go test -json` → `summary.json`
  (`schema: vsp-osd-matrix/1`) and `summary.md`, classified as above.

Rows:

- **The pinned OSD** runs on push, PR and nightly.
- **OSGo** skips while `osgo.version` is empty. Once a `vscode-v0.5.N` tag is
  pinned it starts the binary with `-home <fresh dir> -port 3095` and polls
  `GET /health` until `status` is `ready`. If HEAD
  `/sap/bc/adt/core/discovery` is 200 it runs the full suite; otherwise the
  start itself is the smoke check.
- **The latest `vscode-v0.*` OSD** runs nightly only.
- `workflow_dispatch` takes `target` and `tag`.

The matrix goes to the step summary and is uploaded as an artifact, together
with `osd.log` and `build.json`. Nothing is committed, and no issue is opened
automatically.

**Does it work in principle?** Yes. Everything the job does was run locally:
`osd-up.sh` against the real release, then the test command, then
`osd-summary.sh`. The workflow passes actionlint. It has not yet run on a
GitHub runner. Two expected differences there: the runner downloads the
110 MB asset each time (cacheable by tag), and the ~1.85 GB RSS fits the
standard 16 GB runner. The total is roughly 3 min to ready plus 5-6 min of
tests (each activation costs 25-30 s).

### A capability gate: proposed, not implemented

The classifier already keeps "OSD does not serve this" apart from "OSD got it
wrong", without hiding anything. If skipping becomes preferable later, the
minimal gate is one env var, `VSP_TEST_MISSING=debugger,prettyprinter,rap,fugr,usagerefs,enqueue`,
and a helper `requireCapability(t, "rap")`. The helper would `t.Skipf("target declares rap missing (VSP_TEST_MISSING)")`.
When the variable is unset, which is the case on A4H, nothing changes, and the
summary would count such skips separately. I left this out, because a 404
classified as `missing-endpoint` already says the same thing without
touching 20 tests.

**Planned next step (osg-research's suggestion):** the 7 `missing-object`
tests should skip by **capability**, never by host name. Each of them reads an
SAP-standard object (SAPMSSY0, RS_ABAP_SOURCE_SCAN, BASIS, I_ABAPPACKAGE,
`/DMO/`, `/UI5/`, ZCL_ABAPGIT_AJSON). Before using such a fixture, the test
asks the target whether it is there:

- discovery (`core/discovery`) for whether the collection exists at all;
- a search or a 404 "does not exist" for whether the object exists.

If either says no, the test skips with "fixture <name> absent on this target".
A test never checks whether it is talking to OSD. The same test then runs
unchanged on A4H, on OSD, and on any other target, and a skip names the
missing fixture rather than the system. Until this lands, the classifier
reports these tests as `missing-object` and does not skip them.

## 5. Scenario-suite plan

Each scenario is a named sequence of vsp client calls on throwaway objects,
replayable against any target (OSD, OSGo, A4H). The plan starts from what
`docs/adt-vsp-contracts.md` in open-steamgate already maps from vsp's
`pkg/adt`.

| # | Scenario | Steps | Must prove (not just "no error") |
|---|---|---|---|
| S1 | CRUD | create PROG / CLAS / INTF in the target package → read → delete | object listed in nodestructure; source round-trips; delete removes it from search |
| S2 | Write + activate | lock → PUT → unlock → activate → read | activation verdict from the **body** (HTTP 200 alone is not success); inactive list empty after |
| S3 | Syntax, negative | check a buffer with a known error | a non-empty message list with line/col |
| S4 | Unit, positive and negative | a class with one passing and one failing test | both outcomes reported. Two empty results prove nothing |
| S5 | Lock | lock A; lock B (expect refusal); stateless read between LOCK and PUT; unlock | the #91 sequence. On OSD today, B is granted and the PUT gets 409 |
| S6 | Versions | read the list → write+activate → read the list → read content of the newest entry | A4H: +1 entry per activation. OSD: entry 00000 = working tree, commits 00001..n. Feed root and entry 00000 byte-identical to A4H after normalisation (dell) |
| S7 | Dumps | list over a window; detail of one | OSD: an empty feed by design. A4H: shape only |
| S8 | Impact | `WhereUsed` / `usageReferences` on a used class | OSD: missing-endpoint until the façade grows it |
| S9 | Callgraph | `Callees` via CROSS/WBCROSSGT, `CallGraph` both directions | OSD: needs `CROSS.PROG` and `WBCROSSGT.DIRECT` (or vsp degrades) |

Fixture content comes from what OSD ships (`ZCL_AJSON`, `$ZOSD_TEST_SRC` with
`ZCL_ZOSD_TEST_DEMO`, `ZIF_ZOSD_TEST_GREETER`). Objects that exist only on SAP
(SAPMSSY0, `/DMO/`, `/UI5/`) stay in the A4H-only part of the suite.

## 6. The differential harness: A4H vs OSD

**Purpose.** It is a reusable contract, not a one-off diff. dell will use it as
the façade contract when porting ADT into OSGo (OSG 0.6).

**Shape.** One driver replays S1-S9 against a target and records each HTTP
exchange vsp makes, through a recording `http.RoundTripper` injected into
`adt.Client`. Two runs (A4H, OSD) are joined by `(scenario, step)`.

**Format: stable, versioned, machine-readable.**

- NDJSON, one object per exchange, file extension **`.ndjson`**, or one
  `.json` file per case. Not `.jsonl`: open-steamgate's `.gitignore` swallows
  `*.jsonl` on purpose, to keep captures out, and the redacted contract is
  meant to be importable there.
- Each record:

```json
{"schema":"vsp-adt-contract/1","scenario":"S5-lock","step":3,
 "method":"PUT","endpoint":"/sap/bc/adt/programs/programs/{name}/source/main",
 "request":{"headers":{"X-sap-adt-sessiontype":"stateful"},"query":{"lockHandle":"{lockHandle#1}"}},
 "a4h_normalized":{"status":200,"contentType":"text/plain","body":"…"},
 "osd_normalized":{"status":409,"contentType":"application/xml","body":"…"},
 "verdict":"different"}
```

- `verdict` is one of `same`, `different`, `missing-endpoint`. `missing-endpoint`
  is a 404 whose body says the route is not served (OSD's "is not served by
  OSD"; a 404 for a missing *object* is an answer). Keeping it separate stops
  one missing route from flooding triage.
- `schema` is bumped on any field change; consumers refuse an unknown major.

**Normalisation**, applied to both sides before comparing:

| What | Rule |
|---|---|
| Timestamps (headers, `adtcore:changedAt`, version feed dates) | → `{ts}`. Order is kept where the scenario asserts it |
| GUIDs / session IDs (`sap-contextid`, SAP session GUIDs, which embed the client IP) | → `{guid#n}`, numbered per run, so "same handle reused" stays visible |
| Lock handles | → `{lockHandle#n}`, the same way |
| CSRF tokens, cookies, `ETag`, `If-Match` | → `{csrf}`, `{cookie}`, `{etag#n}` |
| Host, SID, client, user | → `{host}`, `{sid}`, `{client}`, `{user}` |
| Transport numbers | → `{transport}` |
| Generated object names (`ZMCP_12345`) | → `{name}`, from the scenario's own name table |
| XML | canonicalised (attribute order, whitespace, namespace prefixes); the comparison is by element and attribute, not by bytes |

**Where each side runs.**

- **OSD** runs in CI. Its capture is an artifact, one file per run, never
  committed.
- **A4H** runs manually and locally only, never in CI; its credentials stay in
  `.mcp.json`. Throwaway objects go in `$TMP`, locked and then deleted one at a
  time.
- The A4H capture is redacted (host, SID, user, session GUIDs, CSRF tokens,
  cookies, transport numbers) **before** it is written to disk. It is then
  gated with `node tools/osd-leak-scan.mjs --paths <file>`, run from an
  open-steamgate checkout; a non-zero exit blocks any further use of it.
- vsp is a public repo, so nothing A4H-derived is ever uploaded from CI or
  committed here.

**Triage.** An issue in open-steamgate is opened only for a **triaged** OSD
conformance bug, one per behaviour. It is never opened automatically from the
diff.

## 7. Risks

- **Fidelity.** OSD is a model of SAP, not SAP:
  - the syntax check is abaplint, so RAP and CDS sources SAP accepts can be
    refused;
  - locks have no enqueue;
  - versions come from git, not from activation;
  - dumps are an empty feed;
  - the xref tables are a column subset.

  A green run on OSD certifies vsp-against-OSD only. The A4H diff is what
  keeps it honest.
- **Version drift.** Pinning `vscode-v0.4.1444` keeps the matrix comparable
  between runs. The nightly "latest" row shows a drift before the pin moves.
  Tags are frequent (three on 2026-10-01 alone), so moving the pin is a
  deliberate change that comes with the matrix delta.
- **State reset.** A new database alone is not a reset (see §1). Within one
  run, tests share one OSD. Tests that leave objects behind would leak into
  later tests, and the created-but-invisible package (§3) means a package
  cannot be cleaned up over ADT. One run per process, on a fresh home.
- **Cost.** ~1.85 GB RSS and 2-3 min to ready. Each activation reboots the
  runtime (25-30 s), so activation-heavy scenarios dominate wall time.
- **Vacuous passes.** Several vsp tests tolerate errors (`DebuggerListener`,
  `FindReferences` skips on any error, `StatelessClientRefusesDebugCalls…`).
  On OSD they pass or skip without testing anything. The classifier flags the
  ones it can see.
- **Isolation.** See the write to the real data home in §1. It was not reproduced; check the `system home` line in each run's log.

## 8. Open questions for the OSG team, and dell's answers

dell answered on 2026-10-01. The fixes marked **0.4.x** ship in a 0.4.x patch
release; when it is out, the pin moves and the matrix is rerun.

| # | Question | Answer (dell) | Effect on vsp's matrix |
|---|---|---|---|
| 1 | One `osd doctor` with `XDG_DATA_HOME` set materialised into `~/.local/share/open-steamgate` | **Not reproduced.** The 75 MB was probably an `osd-home-<seedId>` left by a start without `XDG_DATA_HOME`. `doctor` now prints the resolved directory. The rerun for the package capture logged `osd: system home <scratch>/xdg/open-steamgate/osd-home-…` and left the real home untouched | `osd-up.sh` keeps setting `HOME` as well; the `system home` line in the uploaded `osd.log` shows where each run lived |
| 2 | A stateless request between LOCK and PUT makes the PUT fail with 409 | **Bug, being fixed (0.4.x).** A stateless hop clears the session's locks | both lock-race tests should pass after the patch |
| 3 | Two sessions both get a lock | **Being fixed (0.4.x):** an enqueue across sessions | scenario S5's "B is refused" becomes testable |
| 4 | Created package invisible; no LOCK/DELETE on packages | There is **no LOCK route for packages, by design**. To clean up, delete the objects, then `DELETE /packages/<name>` **without a lock**. The invisible package is **not reproduced by dell**; vsp's exact exchange is below | vsp's cleanup must not lock a package on OSD (it does on A4H); still open |
| 5 | No `$TMP` | **A writable `$TMP` is planned** | `VSP_TEST_PACKAGE` becomes optional once it ships |
| 6 | `CROSS.PROG` / `WBCROSSGT.DIRECT` missing; empty refusal message | **The empty message is being fixed (0.4.x).** `PROG` and `DIRECT` come in **0.6** | callees/callgraph (S9) stay a known gap until 0.6 |
| 7 | Full runtime reboot per activation | `OSD_WARM=1 STG_DEV=1` gives saves of about 0.5 s for existing classes | worth trying in CI to cut the 25-30 s per activation; `VSP_TEST_TIMEOUT` stays for newly created objects |
| 8 | POST `oo/classes/{n}/includes` (test include) not served | **Being fixed (0.4.x)** | ClassWithUnitTests and CreateClassWithTests should move |
| 9 | OSGo 0.5 tag, readiness and start | Answered earlier: the first `vscode-v0.5.N` tag ships `osgo-*` with `.sha256`. Start with `-home <dir> -port 3095`; ready when `GET /health` says `ready` (about 0.55 s). OData only, no ADT yet (ADR 0007) | wired into `osd-up.sh` and the workflow |
| 10 | `ModificationSupport: NoModification` on a writable object's lock | **Deliberate; A4H does the same.** A read-only object shows as an **empty lock handle** | vsp must decide modifiability from the handle, not from this field |
| - | Versions do not grow on activation | **Deliberate.** Versions come from git commits, not from activation | the S6 diff records this as an expected difference, not a bug |

### Still open: the invisible package (vsp's exact exchange)

This was rerun on 2026-10-01 against `vscode-v0.4.1444`, on a fresh
`XDG_DATA_HOME`, `HOME` and `STG_DB_PATH`, through a logging proxy. Cookies,
CSRF tokens and authorization are dropped from the capture. It reproduces.

1. `POST /sap/bc/adt/packages?sap-client=001&sap-language=EN`
   - Headers: `Content-Type: application/*`, `X-Sap-Adt-Sessiontype: stateless`.
   - Body: a `pack:package` with `adtcore:name="$ZOSD_TEST_VSPCI"`,
     `adtcore:type="DEVC/K"`, `adtcore:responsible="DEVELOPER"`,
     `pack:packageType="development"`,
     `<pack:superPackage adtcore:name="$ZOSD_TEST"/>` and software component
     `LOCAL`.
   - Answer: **201**, with `Location: /sap/bc/adt/packages/%24zosd_test_vspci`.
     `src/zosd_test/vspci/package.devc.xml` is written to the home.
2. `POST /sap/bc/adt/repository/nodestructure?parent_name=%24ZOSD_TEST_VSPCI&parent_type=DEVC%2FK&withShortDescriptions=true`
   (stateless, empty body).
   - Answer: **404** `ExceptionResourceNotFound`, "DEVC $ZOSD_TEST_VSPCI does
     not exist".
3. `GET …/informationsystem/search?operation=quickSearch&query=%24ZOSD_TEST%2A&objectType=DEVC%2FK`.
   - Answer: 200 with the five seeded `$ZOSD_TEST*` packages, but **not**
     `$ZOSD_TEST_VSPCI`. It is still missing more than 10 s later.

One difference from Eclipse may matter: vsp sends the create **stateless**,
with no `adtcore:packageRef`, and nothing between the create and the lookup.

## 9. Reproduce locally

```
.github/ci/osd-up.sh "$(mktemp -d)"             # prints SAP_URL, OSD_PID, …; OSD_WARM=0 STG_DEV=0 for all-cold
SAP_URL=… SAP_USER=DEVELOPER SAP_PASSWORD=osd SAP_CLIENT=001 \
VSP_TEST_PACKAGE='$ZOSD_TEST_SRC' VSP_TEST_TIMEOUT=120s \
  go test -tags=integration -json ./pkg/adt/ ./pkg/ctxcomp/ ./pkg/saprfc/ > test.json
.github/ci/osd-summary.sh test.json summary.json summary.md
kill "$OSD_PID"
```
