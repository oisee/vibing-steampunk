# vsp agenda

The living board. One file, kept current — not a dated series. Dated analyses
live beside it as `agenda/YYYY-MM-DD-NNN-topic.md`.

Written for whoever picks the work up next, including the other agents working
on this repo from other machines. Last updated 2026-08-24, merging the release
session's board with **vsp-amdp-c1**'s — two sessions wrote it the same day from
two worktrees, which is why it says so.

> Sanitize policy applies here like anywhere else in the tree: no live
> hostnames, usernames, transport IDs or customer packages. Operational detail
> with real identifiers belongs under `.local/` or `.private/`.

---

> **Start here for the graph:** [contexts/2026-08-24-graph-handoff.md](../contexts/2026-08-24-graph-handoff.md)
> — state, direction and where to resume. This board carries the items; that
> file carries the shape and the order.

> **Start here for the next release:**
> [agenda/2026-08-29-001-silence-as-an-answer.md](2026-08-29-001-silence-as-an-answer.md)
> — the v2.55.0 sprint. Four defects that are one defect: the tool could not
> answer, so it answered anyway. Ordered, with the test that proves each.

## Landed — 2026-09-04 — v2.55.0

Released through the workflow this time (`gh workflow run release.yml -f
version=v2.55.0`), so the "Commit CHANGELOG.md" step that had skipped twelve
releases ran and landed as `3bbec88`. The section is long — 26 merged PRs,
88 commits — because branches merged after v2.54.0 had never been in a tag;
every entry in it is first-released now, checked with `git merge-base
--is-ancestor` against v2.54.0. Release title and notes were set by hand
afterwards, the way the previous releases did theirs.

Left on A4H: two INDX rows under `RELID = ZV` from the cluster fixture
program, which itself was deleted after the fixtures were captured. Its
source is `pkg/datacluster/testdata/zvsp_cluster_fixture.prog.abap`.

## Landed — 2026-09-05 — v2.56.0

Through the workflow again; CHANGELOG committed by it. Five PRs since
v2.55.0, all one arc: `--layout` from DDIC (#194), deep clusters (#195),
version 5 clusters (#196), jobs and spool (#197), variants, test data,
documentation and the IMG (#198). Titled "the other half of a root cause".

## Done — 2026-09-05 — variants, test data, documentation, the IMG

`vsp variants`, `vsp fmtest`, `vsp docs read|index|img|activity`; MCP
`variants`, `fm_test_data`, `documentation`, `img_search`, `img_activity`.
Facts that took finding: a variant is VARI under two RELIDs — VA the values
(one object per field, a table for a select-option), VB the screen's shape
(`%_VARI40C`); SE37 test data is EUFUNC with `%_I`/`%_V` objects and a 999
directory record; an IMG activity's text is DOKIL `HY` with object
`SIMG`+activity, its title is CUS_IMGACT (not TNODEIMGT, which has the
folders), and TNODEIMGR ties a node to it as `COBJ`/`ACTI`. `pkg/itf`
renders ITF — chapter markers, paragraph continuation, `<ex>/<zh>/<ds>`,
INCLUDE commands — as Markdown; formats seen but rendered only as plain
paragraphs: T1..T6, M1..M5, K1..K6, E1..E3, `>0`/`>1` tables.

Open: select-option storage in VARI was never observed on A4H (no variant
there has one) and is read by shape — a table object with four columns;
verify on a system that has them. `?...` in a text pool means "no selection
text", and `D…` means "DDIC label", read from DD04T.

## Done — 2026-09-05 — jobs and spool as tables

`vsp jobs list|log`, `vsp spool list|read|export`, MCP `job_list`,
`job_log`, `spool_list`, `spool_read`. TBTCO/TBTCP/TSP01/TST01/TST03 over
free SQL; the TemSe list format (`pkg/temse`) read off a 7.58 spool and
checked against XBP's rendering. Job logs are file-stored TemSe on this
system (and most), so they come over RFC through `BAPI_XBP_JOB_JOBLOG_READ`;
`BAPI_XBP_GET_SPOOL_AS_DAT` covers file-stored spool. `BP_JOBLOG_READ` and
`RSPO_RETURN_ABAP_SPOOLJOB` are not remote-enabled — checked, not assumed.

Two data-preview facts learned the hard way and now handled in `RunQuery`
for every caller: the statement is wrapped into 255-character ABAP lines, so
`wrapSQL` breaks it at blanks outside literals first; and a closing paren
needs a blank before it.

Open: OTF/PDF spool (none on A4H to test against; `--raw` gives the bytes,
`BAPI_XBP_GET_SPOOL_AS_PDF` would render), and non-Unicode list spools.

## Done — 2026-09-04 — `vsp applog --messages`, and cluster tables in general

Filed in the morning as "call `BAPI_APPLICATIONLOG_GETDETAIL` over RFC";
shipped in the evening as something better, on `feat/cluster-tables`. The
same day's finding: `BALDAT.CLUSTD` reads fine over free SQL, and the
"SAP LZH" it is compressed with is raw DEFLATE behind an eight-byte header and
a two-bit prefix — `compress/flate` inflates it after a bit shift. The data
cluster format was read off real clusters (a 7.58 INDX fixture with every
elementary type, plus BALDAT) and is `pkg/datacluster`. So:

- `vsp applog --messages` and MCP `application_log` with `messages: true`
  read the messages over ADT alone: class, number, variables, T100 text,
  context, `DETLEVEL`, `PROBCLASS`, time stamp — more than the BAPI returns,
  and no gateway needed.
- `vsp cluster read TABLE --where ...` and MCP `cluster_read` decode any
  INDX-like table; `vsp cluster decode FILE` does it offline from an SE16H
  export, which is the path for a system where SE16H is all there is.
- Field *names*, closed the same evening: `--layout STRUCTNAME` reads DD03L
  (DEPTH nests structured components; an include's rows follow its `.INCLUDE`
  row at the same depth, as many as the include has itself) and lays the
  structure over the descriptor, checking type family, byte length and
  decimals at every leaf. `OBJECT=STRUCTURE` pairs for clusters with several
  objects. `stxl` renders SAPscript text and is STXL's default. Note for the
  next reader: DD03L's `INTLEN` is not to be trusted — it is characters on
  some rows and bytes on others of the same system — so lengths come from
  `LENG` and `DATATYPE`.
- Deep data, same night: a table-typed component is an eight-byte slot in
  the row with its line type as a nested `AD…AE` descriptor and its rows as
  a `BE…BF` block inline in the row stream; a bare string is object kind 07;
  an elementary packed carries its decimals in header byte 2; sorted and
  hashed tables are written like standard ones (the kind is not stored).
  `indx_deep.hex` is the fixture. Table components take their line type
  from DD40L (and DD04L for an elementary line) in `--layout`.
- Version 5, the morning after: EUFUNC (SE37 test data) and AQLDB on A4H
  are clusters a pre-Unicode kernel wrote — `FF 05`, code page 1100,
  two-byte lengths, four-byte descriptor entries with no decimals, rows
  each introduced by `BB` with no count and no framing. `legacy.go` reads
  them; `eufunc_v5*.hex` are the fixtures. Any system that went through a
  Unicode conversion has such rows in every INDX-like table it did not
  rewrite. Not seen yet: a version 5 cluster with strings or nested tables
  (it refuses rather than guesses), and code pages other than 1100 (read
  as Latin-1).
- Found on the way: the data preview clips XSTRING columns to 128 bytes
  and refuses SUBSTRING on them, so REPOSRC (CS-compressed source, one
  0xFF byte before the 1F 9D header) cannot be read whole over free SQL.
  `vsp cluster decompress --skip 1 --text` takes a dump of it made by
  ABAP. Cluster tables worth a layout next: VARI (variant contents — a
  job's selection without RS_VARIANT_CONTENTS), SOC3 (SAPoffice
  documents), COVRES/SCI results, PCL1/PCL2 on HR systems.
- The BAPI path is still worth a `vsp rfc call` when a system blocks free
  SQL; it needs no code.

## Raised — 2026-08-29 — from outside this repo

Two bug reports and one feature request arrived from a session working in
another repo (`docs/bugs/` there, both filed against v2.54.0 / `9b8789d`).
Their identifiers are live, so they stay out of this file; the findings below
are checked against our own code and named with synthetic objects.

### Confirmed, reproducible without SAP

**`ParseABAPFile` and `parseFromContent` recurse into each other forever.**
`pkg/adt/fileparser.go` — the dispatcher falls to `ext == ".abap"` and calls
the content detector, which on `CLASS … DEFINITION`, `REPORT`, `PROGRAM`,
`INTERFACE`, `FUNCTION-POOL` or `FUNCTION` calls the dispatcher back with the
same path. Nothing changes between iterations. Reproduced in an isolated test:
`fatal error: stack overflow`, frames alternating `:131 ↔ :269`. Every bare
`.abap` file the content branch exists to serve takes the binary down, so
`vsp deploy` is gone for that whole class of input. The detector needs to
carry the type it found instead of re-entering the dispatcher.

### Reported as "the MCP tool lists an action it cannot dispatch" — it is worse than that

`test` is dispatched (`handlers_devtools.go`), but only from
`params.object_url`. It is the one action that ignores its `target`
altogether, so `SAP(action="test", target="CLAS ZCL_DEMO_THING")` — the shape
every other action takes — falls through the whole router chain and comes back
`No handler found for action="test"` with `test` printed in the valid-actions
line right below. So neither of the two remedies the report proposes is right:
it is wired up, and it must not be dropped. It should build the URL from the
target the way `buildObjectURL` does for the CLI, and join
`actionsNeedingTarget` so the message names what is missing.

### Unknown params are dropped in silence

`params={"function_group": …}` where the documented key is `parent` reads back
as empty, and the failure then says *give its group explicitly* to a caller who
did. Rejecting unknown keys turns a misleading error into an obvious one.

### `vsp test` on a function group: the client half, answered

`buildObjectURL` (`cmd/vsp/devops.go`) sends `FUGR` to
`/sap/bc/adt/functions/groups/{name}` — correct. But a function group's main
program and its includes both go to `/sap/bc/adt/programs/programs/{name}`,
which is the wrong resource kind for either: an include lives under
`/sap/bc/adt/programs/includes/{name}`, and `INCL` is not a case in that switch
at all. Two of the three addressing attempts in the report were never going to
work regardless of what the backend does; only the `FUGR` one is evidence.

Two things to try before blaming the server, both client-side:
`<testDeterminationStrategy sameProgram="true" assignedTests="false"/>` is
hardcoded in the run payload (`pkg/adt/devtools.go`), and
`parseUnitTestResult` unmarshals into a struct keyed on `<program>` elements —
any document without them yields zero classes and the CLI prints
`No test classes found.` A rejected URI, an error body under 200, and an object
with genuinely no tests are one sentence. That is our own instance of the thing
the report is complaining about: a test that never ran reports what a passing
test reports.

Whether the backend finds a `FOR TESTING` class in a function group's own
include is still open and needs a system.

## Backlog — 2026-08-29 — everything tier 1 does not cover

Tier 1 is the sprint linked at the top of this file and is not repeated here.
What follows is ordered by return on the work, not by age. Two of these need a
decision from Alice before anyone starts; both say so.

### `who-touches TABL <name>` — the only item with an outside caller

Asked for by a session about to add fields to a posting table: before changing
it, the full perimeter of what touches it, split by access — read versus
`INSERT`/`UPDATE`/`MODIFY`/`DELETE` — including AMDP bodies and CDS views
layered over it. Grep gets the easy half and misses dynamic SQL, AMDP and views
that reach the table through layers.

Nothing answers this today, and `vsp graph --direction callers` answers it
*wrongly* — see tier 1 item 2, which is a prerequisite rather than a separate
concern. Fix that first or this feature inherits the same lie.

The parts are already in the tree, which is what makes this a feature and not a
subsystem:

- `client.WhereUsed` posts to
  `/sap/bc/adt/repository/informationsystem/usageReferences` and accepts any
  URI, so a table URI needs no new plumbing;
- `pkg/adt/crud.go:952` already forms `/sap/bc/adt/ddic/tables/{name}`;
- `graph.ExtractEffects` returns `ReadsDB` / `WritesDB` per unit.

Perimeter from where-used, access kind from parsing each hit. Two things to
settle before starting: `WritesDB` lumps all four write statements together, so
the requested R/I/U/D split means widening `EffectInfo`. ~~And `ExtractEffects`
has had no caller since it was written, so this would be its first.~~ It has had
two since 2026-08-25 — `cmd/vsp/effects.go:82` and
`internal/mcp/handlers_effects.go:100` — so this would be its third, and the
"Library, not feature" line in CLAUDE.md was already retired by `a714cbf`.
Corrected 2026-09-02.

Worth asking the requester whether R/W is enough before building R/I/U/D.

### Reject unknown keys in `params` — needs a decision, not just work

`params={"function_group": …}` where the documented key is `parent` reads back
empty, and the failure then tells the caller to *give its group explicitly* when
they did exactly that under a name the tool does not know. Every handler using
`getStringParam` has this shape.

Technically small. But it changes the contract: today an unknown key is free,
after this it is an error, and calls that work now would start failing. That is
Alice's call, not a maintainer's.

### The root MCP-server command ignores `-s <system>`

`vsp -s a4h` works for every CLI subcommand and not for the server itself,
which reads only `SAP_URL` / `SAP_USER` / … from the environment. Found while
releasing v2.54.0, flagged, not changed. Small, and unpleasant to discover by
watching a server connect to the wrong system.

### `analyze type=call_graph` defaults to `callers`

`handleGetCallGraph` is a deliberate `direction`-parameterised entry, so
`callers` and `call_graph` returning byte-identical output is by design and not
a defect. The default is still arguably wrong — `both` is what someone asking
for a *graph* means. One-line change, needs agreement that it is an improvement
and not a break.

### ~~D010INC — the load graph~~ — landed 2026-08-25

~~Still two constants, `EdgeLoads` and `SourceD010INC`.~~ It was
`pkg/graph/builder_loads.go`, `cmd/vsp/loads.go` and `analyze type=loads` four
days before this entry was written, and this board says so itself two hundred
lines further down ("**`D010INC` is built**", under the three graph decisions).
Left struck rather than deleted, because the interesting fact is that a backlog
written on 2026-08-29 called a thing pending that the same file recorded as
done on 2026-08-25. Corrected 2026-09-02.

### `i18n write_message_texts` is unverified

The sweep reaches it; nothing proves it writes. Needs a scratch message class,
and the client has no MSAG creation path, so the verification costs more than
the capability did. Named rather than quietly counted as working.

### `read CLAS` returns 23,653 bytes

Measured during the v2.54.0 output-size work and deliberately left alone. The
answer here is not truncation — it is `pkg/ctxcomp` spending its budget better.
Anything else trades a large honest answer for a small misleading one.

### Three copies of object-type → URI

`runGraph` (`cmd/vsp/cli_extra.go`), `buildObjectURL` (`cmd/vsp/devops.go`) and
the MCP test router each map object types to ADT URIs, and they disagree about
which types exist. Tier 1 touches two of the three and deliberately does not
unify them, because a refactor inside a bug-fix release hides which change
caused what. Do it after v2.55.0 ships, as its own commit.

## Landed — 2026-09-02 — the board's own drift

A triage read CLAUDE.md, README.md and this file against the code line by line
and found fourteen claims that were false at HEAD. Every one was re-measured
before it was changed; three of the fourteen were themselves wrong, and are
recorded below with the rest, because a correction sweep that does not audit
its own inputs is the thing it is trying to fix.

**Counts.** `pkg/graph` is **51 files, 218 test functions**; CLAUDE.md said 45
and 195. `cmd/vsp` registers **55 commands**; CLAUDE.md said 28. The tool
counts are **102 focused / 147 expert**, pinned in
`internal/mcp/tools_parity_test.go:18-20` and printed by `--help`; README said
101 / 146 in two places, **100** in a third, and 102 / 147 in five others — the
same file disagreeing with itself three ways. Unit tests: **1203**
(`go test ./... -list`, integration excluded by build tag), not 821.

**`pkg/adt/ui5.go` is not read-only and has not been since v2.10.0
(2025-12-05).** `UI5UploadFile:273`, `UI5DeleteFile:312`, `UI5CreateApp:345`,
`UI5DeleteApp:387`, all reachable from MCP through `handlers_ui5.go`. The
README had the write leg parked behind "needs custom plugin via
`/UI5/CL_REPOSITORY_LOAD`"; the ADT filestore took PUT and DELETE and no plugin
was ever needed. Nine months of a shipped feature documented as impossible.

**The graph engine's own status was stale in both directions.** CLAUDE.md was
corrected on 2026-08-24 for *understating* the package, then went stale the
other way within a day: it listed D010INC and `graph.ExtractEffects` as pending
for a week after both shipped on 2026-08-25 (`builder_loads.go` + `vsp loads`;
`effects.go:82` + `handlers_effects.go:100`). Its "Areas Requiring Care" row
was worse — "only parser adapter; SQL/ADT adapters pending" contradicted the
Current Priorities section fourteen lines above it, and had done for four
months.

**Four items on this board were done before they were written down.** D010INC,
`ExtractEffects` having no caller, the mode gap, and the `graph_stats` scope
question are all struck in place above rather than deleted, so the pattern
stays visible: this board records a decision as taken in one section and as
open in another, and nothing reconciles the two.

**Undocumented commands.** `vsp loads`, `vsp examples` and `vsp sweep` shipped
and appear in no README; they are now in the CLI Commands block. The triage
also named `vsp applog` — that one was already documented, at README lines 197
and 864-865, and was the first of the three claims that did not survive
re-measurement.

**CHANGELOG.md was twelve releases stale** — newest entry 2.42.0 against tags
running to v2.54.0. Regenerating it is reproducible: git-cliff 2.12.0, the
version pinned in `.github/workflows/release.yml:97`, run against the repo's
own `cliff.toml`, reproduced all 693 existing lines byte for byte and appended
214 covering v2.43.0 through v2.54.0. Zero lines removed. The staleness was
never a formatting drift — the release workflow's "Commit CHANGELOG.md" step
simply did not land for twelve consecutive releases, which is worth a look
before v2.55.0 goes out, because regenerating by hand is not a fix for a
release pipeline that skips a step.

Note for whoever regenerates next: the workflow runs `git-cliff --tag
$VERSION`, which is right when the tag does not exist yet — it dates the new
section today. Run bare `git-cliff` for a catch-up over tags that already
exist, or every backfilled release is stamped with the date you ran it.

## Landed — 2026-08-27 — v2.54.0

**Defaults chosen for a terminal were being paid for in a context window.**
Measured against a live 7.58: thirteen ordinary MCP calls returned 207,138
bytes, and five of them were 92% of it — `callers` and `call_graph` at 52,305
each (200 rows by default), `list_dumps` at 39,954 (no MCP default at all, so
`pkg/adt`'s 100), `search` at 24,809.

None of those was a bug. Each number was picked for a terminal, where a
screenful costs nothing. In an MCP session the result stays in context for the
rest of the conversation, and nobody had converted the price. Forty rows for
lists, twenty for dumps; `max_results` still overrides every one.

The same thirteen calls now return 79,253 bytes — **61.7% less**.

Every cap says so. `callers` used to slice its list and leave the reader to
notice that `total` disagreed with the array length. `search` and `list_dumps`
cannot know their total without a second request, so they say "showing 40, and
there are more" and name the way to ask a smaller question rather than
inventing a figure — both ask for one row more than they show, which costs
nothing and tells a full page from a page that is exactly the size of the
limit.

### Named for whoever measures next

The figures above are **bytes**, because bytes are what the harness returned.
The first version of this note reported tokens — 52,000 before, 19,800 after —
which were bytes÷4, rounded, and written down as though counted. Four bytes a
token probably undercounts this material: JSON repeats its keys and
`/BOBF/CL_CONF_ACT_GEN_HTML` does not split the way prose does. The ratio
holds; the absolutes were never measured.

**`read CLAS` is now the largest single answer at 23,653 bytes** and was left
alone: it is source plus dependency contracts, which is what the call is for.
Making it smaller means making `pkg/ctxcomp` spend its budget better, not
truncating it.

## Landed — 2026-08-26 — v2.53.0

**`SAP()` with no arguments answers instead of refusing.** It used to say
`action is required` — correct, and the least useful correct answer available,
because the caller sending nothing is the one who does not yet know what this is
connected to. The card names the build, whether the session is authenticated,
which system and client, and what to call next. When the session is dead it
gives the diagnosis rather than five calls that will all fail, including that a
401 must not be retried.

The connection check is the CSRF token fetch — an expired SSO session answers
200 with a logon page and only the missing token gives it away — reached through
the new `Client.CheckSession` rather than reimplemented. Not `GetSystemInfo`,
which runs free SQL and would report a usable connection dead under
`--block-free-sql`.

The instance number is derived from the port and says so. `pkg/saprfc` exports
the derivation so there is one place to get the ICM ranges wrong.

`info` is the 52nd advertised capability and has a probe. **50 of 52 on a4h,
no dead capabilities.**

## Landed — 2026-08-25 — v2.52.0

**The v3 registry prototype is off `main` and on `feat/v3`.** Its central claim
was false: six of the eleven declared examples did not work, because tag names
were not handler keys. The revert put `main`'s Go code byte-identical to
`v2.51.0`, and the decision recorded for v3 stands — **static code generation**,
not reflection over struct tags. Do not extend the registry onto more
capabilities.

**The sweep was clean while looking at 39 of 51.** The twelve it skipped were
exactly the routed ones — i18n, revisions, lint — so the release gate was not
looking at the code that changed. Probes first, then the fixes they forced:

| Capability | What was wrong |
|---|---|
| `analyze type=lint` | the analysis router claims every `action="analyze"`; the lint router sat after it and never saw the call |
| `i18n data_element_labels` | `Accept: application/xml` → 406 on every name, *and* a parser reading attributes that are child elements |
| `i18n text_pool` | wrong address entirely — the text pool is its own resource with three plain-text sub-documents |

None had ever worked on any release. The unit tests for two of them asserted
against responses nobody had ever received, so they were green for exactly as
long as the capability was broken.

**`WriteDataElementLabels` refuses instead of guessing.** It PUT a four-field
document at a resource that takes the element's whole representation. Reading
was fixable because reading is verifiable without changing anything; this is
not, and a guess that turns out to be a whole-object replacement costs the
object. The error says what a correct read-modify-write has to do.

**`upsert` no longer reads a failure as an absence.** `objectExists = (err ==
nil)` meant a timeout during an edit became an attempt to create. 404 is the
only error that means absent.

### Open, and named on purpose

- **`i18n write_message_texts` is unverified.** Built against the shape ADT
  serves, never exercised. Verifying it needs a scratch message class to write
  to, and there is no MSAG creation path in the client.
- **The sweep measures one surface.** 51 capabilities is what the universal
  `SAP()` tool reaches. Expert mode's 147 tools and the ~60 CLI commands are
  reach-checked only — registered and routed, never asked whether they answer.
  That is the next real coverage project, and it is much larger than this one.

## Landed — 2026-08-24

**`feat/graph-forward` is on `main`.** Twelve commits, merged by the release
session. Two of them unblock the ABAP-IRC project, which was working around both
by hand: `e58097d` (a function module could be created from a file but never
updated) and `438c3d8` (`edit` accepts TABL). **ABAP-IRC should be told to drop
its workarounds.**

Merged with two conflicts, both in files two sessions edited the same day: this
board, and `handlers_analysis.go`, where the routing switch became a map so
`vsp sweep` can enumerate the analyze surface without calling SAP. The graph
session's `countExecutable` sits above it, untouched.

## Where things stand — 2026-08-24

Three releases in two days: **v2.43.0** (debugger cassettes, post-mortem),
**v2.44.0** (AMDP fires, MCP parity), **v2.45.0** (AMDP with values, and a graph
that was inventing object names).

**The AMDP debugger works.** Over plain ADT, nothing installed on the server:
breakpoints fire, stepping, statement-level traces, variable values, the whole
scope at a stop, and the call stack with both the ABAP and the native line. This
project spent months concluding it was impossible, through a Z service and a
WebSocket protocol built to reach what the system was already offering. The one
thing missing is table *contents*; the address is right and HANA's own `INIT`
refuses it — state and next step are in `AMDPTableRows`.

**The debugger is tested without a system.** `vsp adt debug --record` takes a
cassette from a live run and the tests replay it, so `go test ./...` drives the
real debugger with no SAP. Cassettes are 7.58 only, by the naming rule.

**Ten features were found dead** — advertised and never working — plus one that
was worse. Two classes, and they are not the same:

- **Silence**: an error swallowed as an empty result. A dozen sites across CLI,
  MCP and graph. Three were wrong *numbers*, not missing caveats — a health
  report saying GOOD over a sweep that could not run, `SELF-CONSISTENT` over a
  transport holding nothing, `trace unit` exiting zero while saying nobody ran.
- **Invention**: `vsp graph callees` returned SHA-1 hashes **as the names of
  referenced objects**, because a name too long for `CHAR(120)` is stored hashed
  with the real one in `WBCROSSGTX`. Silence is a loss; invention is a
  corruption. Only this one produced answers that were confidently false.

Not one of them was visible by reading code. Each needed a live system.

### The method that found them

Worth more than the findings, and currently living only in reports:

1. **Ask the system, do not read the catalogue.** Discovery lies both ways — a
   resource absent from it answers 200, one present in it answers 400, and the
   dump resources are listed nowhere at all.
2. **Read the handler.** Five times out of five it answered in one request what
   inference did not answer in several. Sharper form: *when SAP does something
   in the kernel, look at what the same class reads from a table* — that is how
   `TMDIR` was found after `GET_METHOD_BY_INCLUDE` turned out to be a
   `SYSTEM-CALL`.
3. **Read what the system already sent.** The AMDP stop event carried the
   position, then the variables, then the call stack — three finds in one
   document that had been in hand since the first trace.
4. **Measure, do not reason.** Every time a rule was inferred from the examples
   to hand it was wrong about the first case nobody tried: `'FU'` in a `C(1)`
   column, a section-prefix list covering `U01` and missing `U27`.

## Landed — 2026-08-25

**`boundaries` is 11× faster, and the cache is deliberately not built.**
18.8 s → 1.6 s on a 222-object package by fetching sources concurrently,
byte-identical output. The cache stays unwired, but **the signal it needs is now
established**: a controlled experiment on a Z class created and changed for the
purpose showed `ETag == max(REPOSRC includes, excluding the regenerated CS)`,
exactly, on four changes. The first conclusion here — that no sound signal
existed — was wrong, because the probe behind it read 40 rows of a class with 63
includes and the answer was past the cut. Written up in
[2026-08-25-001](2026-08-25-001-cache-invalidation.md) with the numbers, so the
next person does not re-derive it. The parse was never the cost.

## Found, not fixed — 2026-08-25

**`vsp examples` answers nothing for the most-called method on the system.**
`examples CLAS CL_ABAP_UNIT_ASSERT --method ASSERT_EQUALS` finds 186 callers,
reads 15, and produces **0 examples** — printed as `(0 of 15 callers)` and "No
usage examples found", with no caveat, because all 15 sources read cleanly.

Not a regression: identical on the binary from before the concurrency change.
Not the extractor's matcher either, as far as reading goes — it handles
`=>`, `->` and `~`, and falls back to a literal grep for the name, which should
match something in a real caller.

So the suspicion is the **caller list**: 186 where-used hits become 15 names,
and whatever those 15 are, their source does not contain the target. Which is
the shape of the week — a name from one catalogue used as an address into
another. Needs its own measurement, starting with: print the 15 and grep one by
hand.

## Decided — the public surface is generated, not reflected

**v3 is on hold.** Not paused for lack of a plan: the plan changed shape, and
building the old one further would be work to undo.

The shape it changed into: **one package that is the public surface**, whose
functions forward to the real handlers with the same parameters, carrying
annotations from which the MCP routing, the CLI routing, the help and the
parameter documentation are all produced.

**By static code generation, not by runtime reflection on struct tags.** The
distinction decides the failure mode, and that is what makes it the right
question:

- Struct tags have **no moment at which they can fail**. `vsp:"language"`
  either matches the `getStringParam(args, "language")` in the handler or it
  does not, and the only way to find out is to call it. That is the defect
  class this whole month was spent removing, reintroduced as its own fix.
- Generation reads the parameter names **from the source, where they live**.
  A name that does not exist is a compilation error rather than a silent
  disagreement.

Code generation has exactly one failure — a stale generated file, because
somebody did not run `go:generate` — and it is known in advance and cheap to
close: regenerate in CI and fail on a non-empty diff. A forgotten generate
becomes a red build instead of documentation that has quietly stopped matching
the code.

**What is already built and what it is worth.** The registry in
`internal/mcp/registry.go` declares the eleven and derives routing, help,
parameter documentation and the advertised set from one place. It proved the
shape works on real capabilities and it deleted three hand-kept lists. It is
also the reflected version of the idea, and its params structs are documentation
about a handler that still reads a map — so it is a prototype, and whether it
survives the change to generation is an open question rather than a given.

**Not to be extended before that is settled.** Migrating the remaining surface
onto struct tags would be forty more capabilities to move twice.

## Feature freeze — opened 2026-08-25

**The surface stops moving until it is verified on three systems.** Fixes,
tests and corrections are allowed; new capabilities are not, including small
ones. The plan, what "clean" has to mean, and the one exception worth arguing
about are in [2026-08-25-002](2026-08-25-002-feature-freeze.md).

The hard rule it turns on: **nothing from d15 or ms1 enters the repository** —
the tracked artefact is shaped counts and verdicts with the systems anonymised,
and the raw reports stay under `.local/`. A three-way diff is exactly the thing
that tempts an exception.

## Next sprint — analyzer, compressor, abaplint, LSP, graph

**The finding that starts it.** The context appended to a source read — the
dependency contracts a reader gets "around" the code — is built by
`ctxcomp.ExtractDependencies`, a set of nine regular expressions. The *analyzer*
in the same package runs that regex layer **and** a real abaplint parse on top,
merging them and marking confidence and false positives. `analyze_deps` uses the
analyzer. `Compress` does not: it calls `ExtractDependencies` directly.

So **two dependency readers in one package, and the better one does not feed the
user-facing path.** This is the shape the whole week was spent removing, one
level down.

Measured, so the sprint starts from evidence:

- The regex layer sees eight of nine forms probed — `NEW`, `CAST`, `TYPE REF TO`
  in a signature and in a body, the functional static call `zcl_x=>m( )`, the
  interface call, `CATCH`, `RAISING`, `CALL FUNCTION`. It missed `CREATE OBJECT
  lo_x TYPE zcl_y`, now fixed and measured: `CL_RFC_SYSTEM_INFO1` goes from two
  dependencies to three.
- This week's parser fix — the layer named for the parser that was running the
  regex a second time — improved the **analyzer** and left `vsp context`
  untouched. Verified: four classes, identical dependency counts before and
  after, 5/20/13/17.

**A caution for whoever measures next.** An attempt to compare the two readers on
standard classes returned zero dependencies from both, which is not a defect:
`standardSkipPrefixes` drops `CL_ABAP_*`, `IF_ABAP_*` and the `CX_*` families, so
a standard class that references only standard code has nothing left. Compare on
custom code, or the measurement will say the two readers agree because both
found nothing.

### Decided — 2026-08-25: locals are named, not filtered

The context lists nine names for `CL_ATO_CHANGELIST` that have no contract —
`BOM`, `ITEM`, `ITEMS`, `MERGED`, `PREVIEW` and so on. They are **local types**,
not repository objects, and a regex cannot tell the difference because telling
it requires a symbol table.

**They stay.** The one-line note already says what is true of them — referenced,
and no contract here — and that is the whole of what a reader needs. Filtering
them would mean an LSP-grade scope resolution, which is a large machine under a
small question.

Measured before deciding, because the objection to keeping them is cost: a
failed fetch is one bounded, concurrent request, and `vsp context` on those
classes runs in about two tenths of a second either way. There is nothing to
buy.

The interesting part is what the question answered on its way past. **This is
the concrete thing an LSP layer would add**: not "a parser is more reliable" —
the regex sees eight of nine dependency forms — but *scope*. It is the one
question neither the regex nor the graph's edge extractor can answer, because
neither knows what is local. Worth knowing, and not worth building for this.

### The questions, in order

1. **Should `Compress` use the analyzer?** The analyzer is a superset — it runs
   the regex first, then merges. So the gain is the parser's extra forms plus
   the ability to *drop* false positives instead of fetching a contract for
   something that is not a dependency. The cost is a parse per source and an
   API change. Measure both before deciding.
2. **What does the LSP layer add that neither has?** Inside the unit being read,
   a parse gives the local structure — the graph can only ever bring signatures
   of *adjacent* code units. These are different questions and the answer should
   say which one it is answering.
3. **Then the graph.** Contracts come from the dependency's own source today,
   not from the graph at all. Whether the graph should serve them is a real
   design question and not obviously yes: the source is authoritative and the
   graph is derived.

## Backlog — added 2026-08-24

**`vsp document`** — generate documentation for an object or package and push it
into SAP's own store, where an ABAP developer already looks. ZXRAY's idea, minus
its limitation: it could not do packages. Routing is settled and written up in
[2026-08-24-002](2026-08-24-002-vsp-document.md): **reading `DOKHL`/`DOKTL` is
plain ADT and works today; writing needs ABAP on the server**, because ADT
exposes no documentation resource (seven paths, all 404) and no `DOCU_*` module
is remote-enabled — the `BAL_DB_SEARCH` shape, where the blocker is not the
transport. Generate and convert can ship without a receiver; push cannot, and
would make Class D three items instead of two.

## Needs a decision — new

**Turn the method into a command.** Ten dead features found by hand; the
eleventh will ship the same way unless the sweep is automated. `vsp compat`
already has the shape — checks, report, JSON, two-system diff. Extending it to
walk the whole advertised surface is days, not weeks, and it closes the class
permanently, on customer systems too. **This is the recommendation.**

It composes with the other session's work: they are documenting the ten
undocumented capabilities in a form a machine can check — not "shows callers"
but "answers non-empty for an object that has callers, and says the query ran
when it does not". Description without verification rots silently; verification
without description does not know what a right answer is. Together: documentation
that can fail.

**~~The mode gap.~~ Closed 2026-08-25.** ~~The universal `SAP()` tool exists
only in hyperfocused mode, so agents in `focused` and `expert` cannot reach the
seven post-mortem types or the four AMDP targets at all.~~ `SAP` is in
`tools_focused.go:12` and registered through `shouldRegister("SAP")` in every
mode; the pinned counts moved with it and are **1 / 102 / 147**, asserted in
`internal/mcp/tools_parity_test.go:18-20`. The old 1 / 101 / 146 written here
was the count this gap was going to produce, not one that ever shipped.

**Breaking changes in minor versions.** v2.45.0 changed `(*adt.Client).Callees`
and said so in its first paragraph rather than hiding it behind a compatible
wrapper — the wrapper would have kept the defect reachable under the old name.
There is no stated API stability promise; if one is wanted, now is the time.

## Needs a decision

**Address mismatches: measured, mostly closed** —
[002-address-mismatches](2026-08-25-002-address-mismatches.md). `PROG` that is
really an include was the only pair among what package scanners read, confirmed
against a build with the retry removed so the check could have failed. One
object across twelve packages is still unreadable and it is a 500 at both
addresses, not a wrong address.

**What that measurement cannot see, and somebody should:** local classes.
`CCIMP`, `CCDEF` and the rest are in no catalogue, so nothing lists them, no
scan can miss them, and no dependency reader asks for them. A missing object
leaves a gap; an object nothing ever asks for leaves nothing. That is the shape
of defect this week has been worst at finding.


**The three graph decisions are taken** — branch `feat/graph-stats-and-loads`.

- **`graph_stats` widened, not renamed.** The case was in the same file:
  `check_boundaries` already accepts source, an object or a package, so the
  restriction here was the order things were written in. Its package route now
  goes through a scanner extracted from `check_boundaries` rather than a copy of
  it, so both get the object codes, the read type, the unreadable objects and
  the cap right once.
- **`WBCROSSGTI` is reachable and never merged.** Callees answers what an object
  references, and the running code is what that means; the inactive index
  describes a version that does not run. One list holding both would describe
  behaviour nothing has. So: a separate call, `include_inactive` on the surface,
  rows in their own field, every row carrying `Inactive` and its source. The one
  place it is consulted without being asked for is an *empty* answer, where the
  alternative is "references nothing" over an object with 29 of them filed
  against an unactivated version.
- **`D010INC` is built.** The load graph, `analyze type=loads`, both directions.
  It is the only source here that answers what must be *loaded* rather than what
  is *named*, and that is not a nuance: nothing references an include, it is
  included, so an include nothing loads is dead in a way no where-used list
  shows. Three kinds of row in that table and only one is a dependency between
  objects; the other two are containment and kernel machinery.

**Still open, and now the oldest thing on this board**, though smaller than it
reads: unify `cli_deps.go`, `cli_extra.go` and `ctxcomp/analyzer.go`, ~~which
still do not import `pkg/graph`~~. Two of the three now do — `cli_extra.go:16`
and `analyzer.go:9`, the latter calling `graph.ExtractDepsFromSource` directly
at line 315. Only `cli_deps.go` still carries its own extraction. Three
implementations of dependency resolution became one and a half; the sentence
saying otherwise stood until 2026-09-02.


**Four gaps reported by a neighbouring project** (an IRC server on ABAP Push
Channel, built against `$ZADT_VSP` as reference). Reproducible on a4h, reported
2026-08-24, none blocking — workarounds exist for all but the last:

1. `create TABL` adds the client field itself but does not reject a `MANDT` the
   caller also passed, producing a table with two client fields that activates
   and looks correct. An error is wanted, not silent precedence — a silent wrong
   result is indistinguishable from a right one.
2. ~~`edit` does not accept TABL~~ — **fixed**. It was exactly what the reporter
   said: every piece of the route already existed and driving it by hand worked;
   the type was simply not named in three switches and a URL helper. Creating a
   table from source still is not supported, and now says so along with what
   does work, instead of reading as "tables are unsupported".
3. `create` does not know ABAP Channels (`SAPC`, `SAMC`, `DMON`). The recipe,
   read out of abapGit's own object handlers: `CL_APC_APPLICATION_OBJ_DATA` /
   `_OBJ_PERS` with structure `APC_APPLICATION_COMPLETE` and lock `E_APC_APPL`;
   the AMC pair is the same shape. Sequence is `lock → corr_insert(package) →
   set_data → save → unlock` through `IF_WB_OBJECT_PERSIST`. One implementation
   closes the whole family.
4. ~~`query` and `grep` unhandled on the MCP surface~~ — **withdrawn by the
   reporter**: their server process started 2026-08-23 19:31 against a binary
   rebuilt 2026-08-24 00:52, so they were calling yesterday's image. Not a bug.
   Items 1–3 were observed on that same image and are worth re-observing on a
   fresh one before anyone works them.

**Item 2 above is fixed** (`edit`/deploy for function modules): a module is
addressable only under its group, the abapGit filename carries both, and
ParseABAPFile reads both correctly — but the deploy path dropped the group twice,
in the object URL and again in the source URL. It failed on *update* and not on
create, which is why it looked intermittent: DeployFromFile delegates to
UpdateFromFile whenever the object already exists, so the first deploy of a
module could succeed and every one after it could not.

**A twin of the deploy defect, in rename** — reported and **fixed** the same
night. `RenameObject` built both URLs through the parentless wrapper, so
renaming a function module refused with the same sentence. Unlike deploy there
is no filename to read the group from, but it never needed asking for: the ADT
object search maps a module to its group and the client already reads it.

Worth keeping from how it was fixed: the first test written for it **passed with
the defect reintroduced**. Its fake answered a table query while the resolver
uses the object search, so the resolution failed before the code under test was
reached. A test that cannot fail is worse than no test, because it reports a
guarantee nobody has. Every fix on this branch is mutation-checked for that
reason, and this is the one that justified the habit.

**A suggestion worth taking up:** the same reporter found a SAMC object silently
filed in `$TMP` — the design-time handler ignored the package it was given —
only because they exported the package and compared it against sources. The
export found that *and* a wrong AMC activity left on the system in one pass.
**A package export is not a backup, it is a test.** Somewhere in checks there
should be an "export and diff against source" step; it catches a class nothing
else here looks at.

**Two findings from the same reporter, about SAP rather than about vsp:**

- **An RFC session holds a loaded function group.** Edit a module, activate,
  call it again in the same session, and the *old* code runs — activation
  succeeds, the answer is well-formed, and it is simply the previous version.
  The symptom is indistinguishable from "my edit was wrong", which is what makes
  it expensive. Passing a destination override opens a fresh connection and
  reloads. **Open for us:** should `edit` reopen the connection after activating
  a FUGR, or at least say in its answer that a call on this session may return
  the old code?
- **AMC channels need program authorisations, and fail silently without them.**
  Both send and bind simply do nothing. With a trap: binding an AMC channel to a
  WebSocket connection is *not* covered by activity `R` even though the
  connection only consumes — the APC bind manager checks `C`. Recorded here
  because the next person into ABAP Channels loses an evening to it otherwise.

**The graph surface, swept 2026-08-24** —
[001-graph-surface-sweep](2026-08-24-001-graph-surface-sweep.md). All fifteen
graph capabilities called against a live 7.58 system: **ten answer, five do
not**. Worst is `check_boundaries`, which reports CLEAN with zero dependencies
for a package the CLI finds three boundary crossings in — wrong in the
reassuring direction. `analyze_call_graph` returns two nodes for twenty-seven
edges; `references` returns 56,000 characters and cannot be used by the agent it
is for. Two more (`object_structure`, `where_used_config`) failed against a
server process older than the release and **must be rechecked before anything is
spent on them**.

Two findings outrank the defects. The previous inventory listed thirteen
capabilities; the router dispatches fifteen, so a sweep of the list could not
reach `trace_execution` or `compare_call_graphs` — and did not. And a sweep must
name the build it exercised, because a long-lived server keeps the image it
started with and the answer looks identical either way.

**Status:** all five defects fixed the same night (`b3a3bbc`, `e678848`,
`b1b4f29`, `84487ae`), plus two the sweep was not looking for (`62c4c8e`). The
two failures marked for recheck were stale-process artefacts and needed no fix.
~~`graph_stats` remains open as a scope question, not a bug: it analyses source
handed to it and cannot be asked about a repository object, which its name does
not suggest.~~ Answered on 2026-08-25 by the widening recorded below under the
three graph decisions: it now takes source, an object, or a package, and the
sweep probes all three (`internal/mcp/sweep_probes.go:293-312`).

**Ordered next steps** are in the handoff linked at the top; in short: land the
branch, then one sentence in `edit` about function-group activation, then the
sweep as a command, then describe the fifteen against it. The traversal layer is
deliberately not next.

**Open question:** build the sweep as a command (`vsp compat` already has the
shape — checks, report, JSON, two-system comparison), so "does this work" is
answered by a transcript instead of a belief?

**The audit of 2026-08-22** — [002-truthfulness-sprint](2026-08-22-002-truthfulness-sprint.md).
181 promises inventoried, 134 verified, 68 overstated or unverifiable. Tool
counts are corrected and pinned by a test; the rest is queued there, along with
three open decisions: connect or delete gCTS (884 orphaned lines), what to do
with `pkg/jseval`, `pkg/cache` and `pkg/ts2go` (no consumers), and
`vsp install abapgit` — half corrected: `abapgit-standalone.zip` is 836 KB and
real, `abapgit-full.zip` is still 0 bytes. And the question underneath has
changed shape (see below). Strategy recorded: debugger plus dynamic analysis, with a time-boxed
truthfulness pass first; open-abap-go parked with its reasoning kept.


**PR #152 — `fix/lock-nomodification-with-handle`.** The same fix we merged as
`583f042`, opened three weeks earlier by an outside contributor. It now
conflicts with `pkg/adt/crud.go` and `pkg/adt/crud_reconcile_test.go`. The
guard condition differs slightly: #152 fails only on `NoModification && no
handle`, ours fails on `no handle` alone (stricter). Ours additionally parses
the ADT exception body so an EU510 lock conflict reaches the caller with SAP's
own message. **Close as superseded with thanks, or adopt their narrower
condition?** Either way the contributor deserves an answer.

**Tool modes** — see [2026-08-22-001-tool-modes.md](2026-08-22-001-tool-modes.md).
Recommendation: parity test and typed per-action params first, deprecate
`focused`/`expert`, delete in the next major. Not yet decided.

**Eight unmerged branches on origin** — `one-tool-mode` (+16), `worktree-
integration-test-infra` (+8), `pr-93-fix` (+5), plus five small ones from
December–March. Needs triage: what is still alive after the rewrites since.

---

## Handover to wsl-claude — 2026-08-22, from claude-mac-m2

You are closer to the real work systems, so two things are yours:

**1. Take the `GetFunctionGroup` module-list fix** (details under Issue #154
below — read that first; the reported 406 is *not* the bug worth fixing).

Where: `pkg/adt/client.go`, `GetFunctionGroup`. It returns metadata and leaves
`Functions` nil, always. The module list lives behind the `objectstructure`
link on the group; `GetFunctionGroupAllSources` in the same file already walks
it and parses `abapsource:objectStructureElement` children, picking the ones
with `adtcore:type="FUGR/FF"` — reuse that rather than writing a second parser.
Each child carries `adtcore:name` and a `definitionIdentifier` link to its
`source/main`, which is enough to fill `FunctionModule` (`pkg/adt/xml.go:143`).

Watch for: the group endpoint answers 406 to `application/xml`, so keep the
vendor content types and their q-ordering — `...groups.v2+xml` also 406s on the
backend I tested. And a namespaced group works with `%2F`, `%2f`, or a
lowercased name; only raw slashes 404. Do not "fix" the encoding.

**2. Confirm on a real ERP 6.0 non-HANA system.** Everything above was measured
against an S/4-generation backend. The reporter's system is ERP 6.0, which is
exactly where the content types may differ, and it is the one thing I could not
check from here. If the vendor types behave differently there, that changes the
fix, not just the test.

Then #154 can be answered: the 406 is already fixed on main by `edd94bc`, which
landed five days after their build — ask them to retest — and the module list is
the real gap.

Nothing is held back on this machine: everything is merged and pushed, working
tree clean, no unpushed branches. `git pull` is enough.

Also waiting on you, unchanged: rebase `feat/function-module-edit` onto current
main before pushing it — see the next section.

---

## Cross-machine coordination

**`feat/function-module-edit` is on `origin` now** — the claim below that it
exists only on one machine is stale as of 2026-08-24. The ADT contract notes
under it are still worth keeping. Meanwhile `583f042` landed here and covers the *create*
path for function modules end to end. Before pushing:

1. `git fetch origin && git rebase origin/main` — main has moved by four merges
   (lock fix, function modules, host sanitization, browser SSO).
2. Read `pkg/adt/workflows_function.go` — the create path is done. What is
   **not** done is **editing an existing** function module as its own API,
   which is likely where that branch's value actually is.

What we learned about the ADT contract, so it does not get rediscovered:

- The remote-enabled flag is **ignored at creation**. `POST` with
  `fmodule:processingType="rfc"` returns `201` and the module reads back as
  `"normal"`. The flag only takes on a **metadata PUT under a lock**.
- A function module's **signature lives in its source**, not its metadata.
- **Activate after UNLOCK.** Activating while holding the lock returns 403
  EU510. This is true for classes too.

---

## Queued work

**Fast-RFC serializer captures** — the original goal everything else was
clearing the way for. The `ZCL_RFC_TEST` battery is written and active, no
locks outstanding. Next: bring the RFC relay back up, drive the battery through
the sniffing destination once per SM59 serializer mode (Classic / basXML /
Force basXML / Fast), capture one file per mode. The fast-path oracle is
already captured. Operational detail is in the private session notes.

**ZADT_VSP surface** — unblocked now that vsp can create RFC-enabled function
modules without SE37:

- move the enqueue-reset report out of `$Z` into `$ZADT_VSP`
- add the remote-enabled wrapper FM (logic in a class — `ENQUE_DELETE` is not
  remote-enabled, so the reset must live ABAP-side)
- delete the stray `Z_*` report that predates the naming convention
- wire the WS command through the APC handler

**Issue #154 — namespaced function groups return HTTP 406.** Investigated
2026-08-22 on a live system (claude-mac-m2). Two separate things:

*The reported 406 is already fixed on main.* It was the `Accept` header, and
the fix (`edd94bc`, 2026-04-12) landed five days after the build the reporter
is running (`a75fbfd`, 2026-04-07). Reproduced the exact error live by sending
the old header: `Accept: application/xml` → **406**, the vendor type
`...functions.groups.v3+xml` → **200**, on the same namespaced group.
Worth noting `...groups.v2+xml` also answers 406 there, so the q-ordering in
the current header is load-bearing, not decoration.

*URL encoding is not involved.* All three forms answer 200 —
`%2FUI5%2FCACHE_BUSTER`, lowercase `%2f...`, and a lowercased name. Only raw
slashes give 404. Our new `GetFunctionModule` / `CreateFunctionModule` were
checked against a namespaced group and are fine.

*But a different, real bug turned up.* `GetFunctionGroup` **never** returns the
function module list — `Functions` is `null` for a namespaced group and for a
plain one that certainly has modules. The metadata document simply does not
carry them; they hang off the `objectstructure` link, which
`GetFunctionGroupAllSources` already knows how to walk. So the reporter's
actual need — "list the modules in this group" — is still unmet even with the
406 gone, and the tool is described in the focused whitelist as "Metadata:
function module list".

**Next:** populate `Functions` from `objectstructure` in `GetFunctionGroup`,
then answer #154 saying the 406 is fixed, asking the reporter to retest on
current main, and pointing at the module-list fix. **A real ERP 6.0 non-HANA
system would be the better place to confirm** — the checks above ran against
an S/4-generation backend.

**`delete` through MCP needs a lock handle.** It is a two-step call that leaks
internal mechanics to the caller, unlike `create`, which now does the whole
flow. Same one-call treatment would suit it.

**`gofmt -l` reports 5 files** — `cmd/abapgit-pack/main.go`,
`fun/llvm2abap_demo.go`, `internal/lsp/server_test.go`,
`internal/mcp/handlers_context.go`, `internal/mcp/handlers_graph.go`.
Long-standing, unrelated to recent work.

---

## The installer question, reshaped

The board asked whether `vsp install abapgit` can be made to work. The shape of
the question changed once the routing was checked on a live system.

Only **two** things still need ZADT_VSP: **stateful RFC** (SOAP-RFC cannot hold
a session) and **git**. Read, edit, debug, AMDP, table reads, module lists and
transports are all plain ADT. AMDP left that list this week.

And "git" is not really our package. `vsp copy` handles **six** object types
natively — PROG, CLAS, INTF, DDLS, BDEF, SRVD — while the Z path handles
whatever abapGit does, by delegating: `ZCL_VSP_GIT_SERVICE` calls
`zcl_abapgit_objects=>supported_list( )`. So the dependency is **abapGit**, and
our service is a bridge to it.

That gives the bootstrap problem a hard edge the earlier note missed. The six
native types include **PROG and CLAS**, and a program deploys to a bare system
over plain ADT — verified twice this week, on two releases. So:

- abapGit *standalone* can already be installed with zero Z code. It is one
  report. But it exposes no global classes, so nothing can call it.
- Therefore a lean receiver **must itself be a report or a class**. Not a
  function group, not a package of objects — those cannot be installed by the
  six, and a receiver that needs a receiver is the circle it was meant to break.

**Step one, unstarted:** confirm a *class* deploys to a bare system the way a
program does. Everything above rests on it and it is fifteen minutes. Nothing
should be built before that is checked — building a bootstrap for a dependency
whose shape we had wrong is how this note started.

## Still open, smaller

- **AMDP table contents.** Address right, HANA's `INIT` refuses. Untried: the
  `tableHandle` the stop reports, which appears in none of that resource's
  parameters and so may belong to another route.
- **`WBCROSSGTI`** is wired only to *explain* an empty answer, not merged into
  the data — deliberate, until someone decides what a merged list should mean.
- **`WBCROSSGTX` decodes downward, not upward.** `graphFromCross` stays
  object-level; `LONG_NAME` takes `LIKE` despite being `STRG`, recorded at the
  function.
- **7.50 coverage** for the newest work: the ST22 check in `execute`, `--impact`
  where the dump detail resource is absent.
- **gCTS** — parked, never checked at all.
- **Three orphan packages** — `ts2go` archive, `jseval` extract, `cache`
  connect. Decided, not executed.
- **The article.** Material is now large and unusually concrete: self-repairing
  SSO, RFC with no gateway, a debugger working on a release with no stack
  resource, an AMDP breakpoint after months of "impossible", ten dead features,
  and a tool that returned a hash and called it an object.

## Recently landed

- `61a0375` — `NoModification` no longer fails a MODIFY lock that came with a
  handle, and the guard no longer leaks the ENQUEUE it just took. That leak was
  the cycle where every retry hit its own orphan lock and reported it as
  "NoModification".
- `583f042` — create RFC-enabled function modules in one call; SE37 is out of
  the loop. Verified end to end: the created module answers a classic RFC call.
- `99f6896` — a live host name removed from a context table, a design note and
  three reports.
- `3bb165f` — browser SSO merged from the Windows side.

---

## Other tracks (not this repo)

Two threads run alongside and have their own private notes, deliberately not
tracked here:

- **Inter-agent IRC bus** — channel autodiscovery from the git remote is
  written and waiting on a decision to push; an April security review has six
  of seven findings still open, three of them Critical. The write-up names
  unpatched issues in a public repo and must stay private until they are fixed.
- **Storage reorganization** on the shared network volume — done, reversible
  from a manifest.
