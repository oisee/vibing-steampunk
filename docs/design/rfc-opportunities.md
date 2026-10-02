# Classic RFC: what a second protocol unlocks for vsp

Design / ideation report, 2026-08-20. Grounded in the code of both repositories
and, where marked ✅, **verified live** against the A4H test system
(`SAP_BASIS 758`, kernel `793`, HDB, code page `4103`, `RFCPROTO 011`) over the
SDK-free [open-rfc-go](https://github.com/oisee/open-rfc-go) client on the same
day. Nothing here has been implemented; this is the ranked case for what to
build next, including the ideas that should **not** be built.

> **Reading key.** ✅ = observed live on A4H today. ⚠️ = plausible but unverified;
> every ⚠️ carries the exact command that settles it. ❌ = checked and false.

---

## 0. Where the RFC leg stands today

vsp shipped classic RFC on 2026-08-20 (commits `d4e51ea`, `2f79046`, `c2f0fb9`).
Three files, no CHANGELOG entry yet:

| File | Role |
|---|---|
| `pkg/saprfc/saprfc.go` | destination resolution — ADT URL → gateway host, port `3300 + sysnr`, credential precedence (`rfc_user`/`rfc_password` → `SAP_USER`/`SAP_PASSWORD` → the ADT logon) |
| `cmd/vsp/rfc.go` | CLI `vsp rfc info\|ping\|describe\|call\|search\|read-table` |
| `internal/mcp/handlers_rfc.go` | `routeRFCAction`, wired into the universal tool at `internal/mcp/handlers_universal.go:97` — `SAP(action="rfc", params={"op":…})` |

That is a good v1 and it already calls *any* RFC-enabled function module. But it
was built as "a second way to read things", and it inherits four structural
limits that every idea below runs into. **Fix these first; they are hours, not
days.**

| # | Limit | Where | Consequence |
|---|---|---|---|
| L1 | The MCP handler opens **and closes** a client per call (`handlers_rfc.go:63` `defer c.Close(ctx)`) | `Server.rfcClient` | Logon + metadata round-trips on every tool call; and no stateful conversation is expressible at all (see §1) |
| L2 | `--where` is a single `OPTIONS` row | `cmd/vsp/rfc.go:186`, `handlers_rfc.go:170` | WHERE clauses silently truncate at **72 characters**. ✅ Multi-row `OPTIONS` works and concatenates — I read `TADIR` with a two-row clause today |
| L3 | Only the 512-byte `DATA` table is read | both `rfcReadTable` copies | ✅ `read-table USR02` fails `DATA_BUFFER_EXCEEDED (AD559)`. ✅ The same call with `USE_ET_DATA_4_RETURN='X'` succeeds and returns `ET_DATA` rows of type `SDTI_RESULT-LINE` (**`STRING`**, no 512 cap) |
| L4 | `rfcReadTable` is **duplicated verbatim** in `cmd/vsp/rfc.go:207` and `internal/mcp/handlers_rfc.go:166` | — | Every fix has to be made twice. Promote it to `pkg/saprfc` |

Also worth knowing: `rfc.Destination.Callbacks` exists in open-rfc-go and is
live-proven, and vsp wires **none** of it (`pkg/saprfc/saprfc.go:113` builds a
`Destination` with no `Callbacks` and no `Pool`). The push direction is
available and unused.

---

## 1. The property that makes this interesting

vsp's debugger has been disabled by default since it shipped
(`pkg/config/systems.go:271` `DefaultDisabledTools` — 16 tools: the ABAP
debugger, the AMDP debugger, breakpoints, UI5 writes). The reason is written
down in `docs/adr/001-websocket-stateful-debugging.md`:

> "cannot be reliably maintained via standard HTTP/HTTPS transport due to
> **session affinity** requirements and long-polling timeout constraints … Each
> MCP tool call may spawn a separate process … The debugger listener catches the
> debuggee, but attach/step operations fail due to session mismatch."

and, in the underlying investigation
(`reports/2025-12-14-002-…-breakpoint-storage-investigation.md:399`):

> "**Eclipse works because RFC maintains server affinity**; HTTP requests may hit
> different servers."

That is the whole thesis of this document. **A classic RFC connection is a
pinned, stateful ABAP session on one application server.** It is not a request
protocol with a session cookie bolted on; the ABAP session — with its function-
group globals, its locks, its debugger context, its update task — lives for the
lifetime of the TCP conversation and dies with it. HTTP/ADT cannot express that;
that is precisely why ZADT_VSP's APC WebSocket was invented, and the WebSocket
turned out to be the least reliable component in the stack (500 on upgrade,
`websocket: bad handshake`, 5-second welcome timeout, no reconnect, no re-attach,
and the `APC_ILLEGAL_STATEMENT` restriction that forbids `SUBMIT`,
`COMMIT WORK AND WAIT` and `CALL FUNCTION … DESTINATION` inside the handler —
issue #55).

RFC gives us the stickiness for free and has none of the APC statement
restrictions. **But vsp cannot use that property today** — see L1 above, plus a
gap on the open-rfc-go side: `rfc.Client.Call` acquires a pooled lease per call
(`rfc/call.go:64`) from a pool that defaults to `MaxSize = 8`
(`internal/pool/pool.go:59`). Sequential calls on one `Client` do de-facto reuse
one session, but nothing *guarantees* or documents it, and there is no exported
way to say "these N calls are one conversation". That is the single most
valuable thing open-rfc-go could add for vsp (§13).

---

## 2. abapGit over RFC — the "хитрый инжект" · ✅ **proven live today** · effort **S**

**This is the standout finding.** abapGit's developer version ships a function
group `ZABAPGIT_PARALLEL` (package `$ZGIT_DEV_OBJECTS_CORE`) whose two function
modules are **remote-enabled out of the box** — abapGit needs them RFC-enabled
because it calls them with `STARTING NEW TASK … DESTINATION IN GROUP` for
parallel serialization. We can just call them ourselves:

```
✅ TFDIR: FUNCNAME=Z_ABAPGIT_SERIALIZE_PACKAGE   FMODE=R  PNAME=SAPLZABAPGIT_PARALLEL
✅ TFDIR: FUNCNAME=Z_ABAPGIT_SERIALIZE_PARALLEL  FMODE=R  PNAME=SAPLZABAPGIT_PARALLEL
```

`Z_ABAPGIT_SERIALIZE_PACKAGE` interface (✅ from `vsp rfc describe`):

| Direction | Parameter | Notes |
|---|---|---|
| IN | `IV_PACKAGE` (required, 15) | development class |
| IN | `IV_FOLDER_LOGIC` | `PREFIX` / `FULL` / `MIXED` |
| IN | `IV_MAIN_LANG_ONLY`, `IV_SHOW_LOG` | flags |
| OUT | `EV_XSTRING` | **the abapGit ZIP** |
| EXC | `ERROR` | |

Live run against A4H, one call, no ADT, no WebSocket, no ZADT_VSP:

```bash
vsp rfc call Z_ABAPGIT_SERIALIZE_PACKAGE \
  '{"IV_PACKAGE":"Z_BADI_CHECK","IV_FOLDER_LOGIC":"PREFIX","IV_MAIN_LANG_ONLY":"X"}'
# ✅ EV_XSTRING → 10 263 bytes, magic PK\x03\x04, entries:
#    .abapgit.xml, src/package.devc.xml, src/zbadicheck.msag.xml,
#    src/zbadicheck_inc.prog.abap, src/zbadicheck_inc.prog.xml,
#    src/zbadicheck_inc_f01.prog.abap, src/zbadicheck_inc_f01.prog.xml,
#    src/z_badi_check.prog.abap, src/z_badi_check.prog.xml
```

### What it replaces

`vsp export` today (`cmd/vsp/cli.go:205` → `pkg/adt/git.go:92 GitExportToBytes`)
goes: Go → APC WebSocket → `ZCL_VSP_GIT_SERVICE` (`embedded/abap/
zcl_vsp_git_service.clas.abap`) → `zcl_abapgit_objects=>serialize` → hand-rolled
`cl_abap_zip` + `SSFC_BASE64_ENCODE` → back over the socket, with a **120-second
hard timeout** (`git.go:163`). Every one of those hops can and does fail. The
RFC path is: Go → gateway → abapGit's own serializer → `XSTRING`. It needs

- no ZADT_VSP (which the export path currently requires — `vsp config` groups it
  under "requires ZADT_VSP"),
- no `ZCL_VSP_GIT_SERVICE` (whose `handle_import`/`handle_validate` are stubs
  returning "not yet implemented" anyway),
- no ICF/APC node, no WebSocket upgrade, no base64 hop, and it produces a *real*
  abapGit zip with real `.abapgit.xml` folder logic instead of vsp's
  reimplementation.

And note the state of the alternative: **`vsp install abapgit` is a no-op today**
— `embedded/deps/abapgit-standalone.zip` and `abapgit-full.zip` are both **0
bytes**, `GetAvailableDependencies()` reports them unavailable, the CLI path
errors out with instructions, and the MCP path (`handlers_install.go:581`) is an
unimplemented TODO.

### Where it lands

| Surface | Shape |
|---|---|
| CLI | `vsp export <PKG> -o pkg.zip --via rfc` (new flag on the existing `exportCmd`), or a new `vsp rfc export` |
| MCP | `SAP(action="rfc", params={"op":"export","package":"Z_FOO","folder_logic":"PREFIX"})`, or better: make `SAP(action="system", params={"type":"git_export"})` (`internal/mcp/handlers_git.go:62`) pick RFC when available and fall back to the WebSocket |
| Code | new `pkg/saprfc/abapgit.go`; `handlers_git.go` gains a transport switch |

### Honest limits

- **Serialize only.** There is no RFC-enabled deserialize. ✅ Only two
  `Z_ABAPGIT*` FMs exist on A4H, both serializers. Pull/push/staging/commit would
  need a thin Z wrapper FM over `zcl_abapgit_objects=>deserialize` /
  `zcl_abapgit_repo_online`. That is a real but small ABAP artefact (~80 lines,
  RFC-enabled, one `IV_XSTRING` in, a `BAPIRET2` table out) and it belongs in
  ZADT_VSP as `Z_VSP_ABAPGIT_DESERIALIZE` — see §12.
- **Requires the abapGit developer version**, not the standalone report. The
  standalone `ZABAPGIT` program cannot contain a function group. ✅ A4H has the
  dev version (`$ZGIT_DEV*` packages, `FUGR ZABAPGIT_PARALLEL`); a system with
  only `ZABAPGIT_STANDALONE` will not have these FMs. Probe with
  `vsp rfc search 'Z_ABAPGIT_*'` before choosing the transport.
- Serialization runs synchronously in the RFC work process. A large package will
  hit `rdisp/max_wprun_time`. Use `Z_ABAPGIT_SERIALIZE_PARALLEL` (object-level:
  `IV_OBJ_TYPE`/`IV_OBJ_NAME`/`IV_DEVCLASS`) and fan out from Go — which is
  exactly what abapGit itself does, except Go can do it across *our* goroutines
  with our own concurrency budget.
- Auth: needs `S_RFC` on function group `ZABAPGIT_PARALLEL` plus whatever
  `S_DEVELOP` the serializer's readers check.

**Effort S.** One new file, one flag, one MCP branch. The hard part is already
proven.

---

## 3. An RFC system probe — fingerprint a system in one round trip · effort **S**

`pkg/adt/features.go` has a `FeatureProber` with exactly six features
(`hana`, `abapgit`, `rap`, `amdp`, `ui5`, `transport`), each an HTTP probe
against an ADT endpoint, cached per process. It answers "can I use this vsp
tool", and it cannot see anything the ADT surface does not expose.

RFC sees a different, and frankly better, layer of the system. Everything in this
table is ✅ verified callable on A4H today:

| Fact | How | Notes |
|---|---|---|
| SID, release, kernel, DB, host, IP, code page, RFC proto | `RFC_SYSTEM_INFO` | ✅ already in `vsp rfc info`. Code page `4103` = UTF-16LE ⇒ Unicode system; `RFCPROTO 011`; `RFCKERNRL 793`; `RFCSAPRL 758` |
| Installed components + patch levels | `RFC_READ_TABLE CVERS` **or** `DELIVERY_GET_INSTALLED_COMPS` (`FMODE=R`) | ✅ A4H: `SAP_BASIS 758 SP2`, `S4FND 108`, `SAP_GWFND`, `SAP_UI`, `ST-PI 740 SP28`, `DMIS 2020`… |
| Component release / state | `DELIVERY_GET_COMPONENT_RELEASE`, `DELIVERY_GET_COMPONENT_STATE` (`R`) | ✅ |
| Application servers in the system | `TH_SERVER_LIST` (`R`), `TH_SERVER_STATE` (`R`) | ✅ tells us whether affinity is even a risk here |
| **Does the caller have `S_RFC` for FM X** | `RFC_SIMULATE_AUTH_CHECK(IV_FM, IV_USER) → EV_AUTHORIZED` (`R`) | ✅ ran it: `SXPG_COMMAND_EXECUTE` → `X`, `RFC_READ_TABLE` → `X`. **This is the killer probe** — it answers "will this tool work for this user" *without executing anything* |
| Is ZADT_VSP installed, and which version | `RFC_READ_TABLE TADIR WHERE OBJ_NAME LIKE 'ZCL_VSP%'` | ✅ A4H: 9 classes in `$ZADT_VSP` |
| Is abapGit installed, dev vs standalone | `TFDIR FUNCNAME LIKE 'Z_ABAPGIT%'` + `TADIR OBJ_NAME LIKE 'ZABAPGIT%'` | ✅ A4H: dev version, `$ZGIT_DEV*` |
| Which RFC destinations exist (and their types) | `RFC_READ_TABLE RFCDES` | ✅ A4H: 8 `A4H@*` — seven type `3`, one type `T` |
| Whether a given FM exists and is remote-enabled | `TFDIR … FMODE='R'` | ✅ already `vsp rfc search` |
| Gateway reachability, before any logon | TCP connect to `3300+NN` | ✅ trivially, and it distinguishes "system down" from "ICF down" |
| Locks held right now | `ENQUEUE_READ` (`R`) | ✅ RFC-enabled. `DEQUEUE_*` are **not** ❌ — you cannot break locks over plain RFC, which is a *feature* |
| Field/DDIC metadata | `DDIF_FIELDINFO_GET`, `DDIF_FIELDLABEL_GET`, `DDIF_STATE_GET` (`R`) | ✅ |
| Class metadata | `SEO_CLASS_TYPEINFO_GET_RFC` (`R`) | ✅ |

**Not** available: SNC status and gateway ACL configuration are not exposed as an
RFC-enabled FM I could find; infer SNC from `RFCDES`/profile parameters via
`SXPG_PROFILE_PARAMETER_GET` (`R`, ✅) — ⚠️ unverified whether it returns
`snc/enable` for the calling instance.

### Where it lands

- `pkg/adt/features.go` grows a sibling `pkg/saprfc/probe.go` with an
  `RFCProbe` producing a single struct; `FeatureProber` gains an optional RFC
  arm so `SAP(action="system", params={"type":"features"})`
  (`internal/mcp/handlers_system.go:76`) reports both.
- CLI: `vsp rfc probe` — one command, one screen, the whole fingerprint.
- The most useful new output is a **capability matrix keyed by tool**: for each
  vsp tool that has an RFC implementation, run `RFC_SIMULATE_AUTH_CHECK` on the
  FMs it needs and print ✓/✗ per tool per user. No other transport can do that.

**Risks.** `RFC_SIMULATE_AUTH_CHECK` itself needs authority (it takes an
arbitrary `IV_USER`, so treat it as sensitive); reading `RFCDES` exposes
destination names and, on some releases, credentials-adjacent columns — project
only `RFCDEST`/`RFCTYPE`. Everything else here is read-only.

**Effort S.**

---

## 4. Mass data extraction, done properly · effort **S**

vsp reads table data over ADT `datapreview`:

- `POST /sap/bc/adt/datapreview/ddic?rowNumber=N&ddicEntityName=T` (`pkg/adt/client.go:986`)
- `POST /sap/bc/adt/datapreview/freestyle?rowNumber=N` with SQL as `text/plain` (`client.go:1012`)

Known pain, in vsp's own comments: **"SAP freestyle query has a ~255 char literal
limit for IN clauses"** (`internal/mcp/handlers_graph.go:249`, again at `:289`),
**"SAP freestyle doesn't support complex OR clauses"**
(`handlers_transport_analysis.go:49`), the `ASCENDING` not `ASC` dialect trap,
and a default of 100 rows. `RunQuery` is also gated behind the `OpFreeSQL`
safety op, so a locked-down profile loses table access entirely.

RFC's `RFC_READ_TABLE` trades a different set of constraints, and the trade is
favourable for the graph/analysis workloads that hurt most:

| | ADT freestyle | RFC_READ_TABLE |
|---|---|---|
| WHERE length | ~255 char literal ceiling on `IN` | ✅ unbounded — `OPTIONS` is a *table* of 72-char rows that concatenate (verified with a two-row clause) |
| Row width | fine | 512-byte `DATA`… ✅ unless `USE_ET_DATA_4_RETURN='X'`, which returns `ET_DATA-LINE` as a **`STRING`** |
| Paging | client-side `--skip` slicing (`cmd/vsp/cli_extra.go`) | `ROWSKIPS` + `ROWCOUNT`, server-side |
| Schema-only | — | `NO_DATA='X'` returns just `FIELDS` |
| Joins / aggregates | yes | **no** — one table or view, no joins. This is RFC's real weakness |
| Blocked by | `BlockFreeSQL` safety op | `S_RFC` on `SDIFRUNTIME` + `S_TABU_*` |
| Blocked tables | — | ✅ some are simply not exposed: `SNAP` → `TABLE_NOT_AVAILABLE (DA131)` |

So: **keep ADT freestyle for joins and ad-hoc SQL; route wide/large/long-WHERE
reads over RFC.** The concrete wins are the batched `IN` lists in
`handlers_graph.go` (which today are chopped into batches of 5 to dodge the
255-char limit — over RFC they need no chopping at all) and any `SELECT *` on a
wide table.

Also available and unused: `RFC_GET_TABLE_ENTRIES` (`R` ✅) as a second reader,
and `GET_SORTED`.

**Where it lands.** Fix L2/L3/L4 in a promoted `pkg/saprfc/readtable.go`; add
`--et-data`, `--skip`, `--no-data` to `vsp rfc read-table`; teach
`handlers_read.go` `action="query"` to prefer RFC when the WHERE clause is long
or the projection is wide.

**Effort S.**

---

## 5. Reports, background jobs and spool over RFC — the fix for issue #55 · effort **M**

Today `vsp` runs reports over the WebSocket `report` domain
(`pkg/adt/reports.go:247`, `internal/mcp/handlers_report.go`), landing in
`ZCL_VSP_REPORT_SERVICE`, which does `SUBMIT (lv_report) … AND RETURN` and
scrapes ALV via `cl_salv_bs_runtime_info`. It is documented as an *architectural*
failure: inside an APC handler, `SUBMIT`, `COMMIT WORK AND WAIT`,
`CALL FUNCTION … DESTINATION` and `RSTS_OPEN` are all blocked
(`APC_ILLEGAL_STATEMENT`); the ABAP service even carries a comment "Run a report
via async RFC — workaround for APC_ILLEGAL_STATEMENT with SUBMIT". Worse, the Go
and ABAP sides have drifted: `handleRunReport` demands `JobName`/`JobCount` that
the embedded ABAP never returns, and errors with "ABAP service may need
updating". The report tools sit in disable-group `"X"` (experimental).

Over RFC none of that applies. The standard, RFC-enabled machinery exists:

| FM | `FMODE` | Role |
|---|---|---|
| `SUBST_START_REPORT_IN_BATCH` | ✅ `R` | schedule a report as a background job. IN: `IV_REPNAME` (req), `IV_JOBNAME`, `IV_VARNAME`, `IV_VARIANTTEXT`, `TT_REPORTPARAM`, `IS_PRIPARAMS`, `IV_AUTHCKNAM`, `IV_BATCHHOST`, `IV_LANGUAGE`. OUT: `EV_JOBCOUNT`, `EV_STARTRC`, `EV_VARIWRC` |
| `SUBST_START_REPORT`, `SUBST_START_BATCHJOB` | ✅ `R` | synchronous / job variants |
| `BAPI_XMI_LOGON` / `BAPI_XMI_LOGOFF` | ✅ `R` | **mandatory prologue** for every XBP call — the XBP interface refuses to work without an XMI session |
| `BAPI_XBP_JOB_START_ASAP`, `_JOB_ADD_ABAP_STEP`, `_JOB_CLOSE`, `_JOB_ABORT`, `_JOBLIST_STATUS_GET`, `_JOB_COUNT`, `_JOB_CHILDREN_GET` | ✅ `R` | full job lifecycle |
| `BAPI_XBP_JOB_SPOOLLIST_READ` | ✅ `R` | IN `JOBNAME`, `JOBCOUNT`, `STEP_NUMBER`, `EXTERNAL_USER_NAME`; OUT `SPOOL_LIST`, `RETURN` — **the report output** |
| `BAPI_XBP_VARIANT_CREATE/CHANGE/COPY/DELETE`, `BAPI_XBP_VARINFO` | ✅ `R` | variant management, which vsp's `GET_VARIANTS` currently does over the WebSocket |
| `BAPI_XBP_EVENT_RAISE`, `_BTC_EVTHISTORY_GET` | ✅ `R` | event-driven orchestration |

Note the classic `JOB_OPEN`/`JOB_SUBMIT`/`JOB_CLOSE` trio is ❌ **not**
RFC-enabled (✅ checked: no `JOB_%` FM has `FMODE='R'`). Use XBP, or
`SUBST_START_REPORT_IN_BATCH`, which wraps them.

**The loop.** `BAPI_XMI_LOGON` → `SUBST_START_REPORT_IN_BATCH` → poll
`BAPI_XBP_JOBLIST_STATUS_GET` → `BAPI_XBP_JOB_SPOOLLIST_READ` →
`BAPI_XMI_LOGOFF`. All read/execute; the only write is the job itself.

**Where it lands.** `pkg/saprfc/jobs.go`; `internal/mcp/handlers_report.go`
gains an RFC branch so `SAP(action="debug", target="RUN_REPORT")` and
`RUN_REPORT_ASYNC` work without ZADT_VSP; CLI `vsp rfc run-report ZFOO --variant
V1 --wait`. This *un-disables* an entire experimental tool group.

**Risks / limits.** Background execution is genuinely asynchronous — no ALV
runtime capture, so the ALV-scraping behaviour of `ZCL_VSP_REPORT_SERVICE` is not
reproduced; you get the spool list instead (usually better, occasionally worse).
`BAPI_XMI_LOGON` writes an XMI audit log entry per session — noisy on a shared
system; set the audit level low. Scheduling a job is a **write**: it must sit
behind the same confirmation gate as any other mutation, and `BAPI_XBP_JOB_ABORT`
is destructive. `S_BTCH_JOB`/`S_BTCH_ADM` and `S_RFC` on `SXBP`/`SXMI` are
required.

**Effort M.** Two days of plumbing plus a careful safety gate.

---

## 6. The debugger over RFC — bringing back 16 disabled tools · effort **L**

This is the idea the user floated first, it is the most valuable, and it is also
the one that needs the most new work. The honest answer has three parts.

### 6.1 Can ZADT_VSP's existing services be reached as RFC FMs? Not as they stand.

`ZCL_VSP_DEBUG_SERVICE` (1 037 lines,
`src/zcl_vsp_debug_service.clas.abap`) is a **class implementing
`ZIF_VSP_SERVICE`**, dispatching a `{id, domain, action, params, timeout}`
envelope. Classes are not remote-enabled; only function modules are. So "just
RFC-enable the existing thing" is not available. What *is* available is a **thin
Z function-group facade**: one function group `ZVSP_DBG`, whose global data holds
the service instance, and one remote-enabled FM per action that forwards to it.

```abap
FUNCTION-POOL zvsp_dbg.
DATA go_dbg TYPE REF TO zcl_vsp_debug_service.   " survives across calls
                                                  " on the same RFC connection

FUNCTION z_vsp_dbg_call.        " RFC-enabled
*"  IMPORTING IV_ACTION TYPE STRING
*"            IV_PARAMS TYPE STRING        " the same JSON as today
*"  EXPORTING EV_DATA   TYPE STRING
*"            EV_ERROR  TYPE STRING
  IF go_dbg IS INITIAL. go_dbg = NEW #( ). ENDIF.
  DATA(ls_resp) = go_dbg->zif_vsp_service~handle_message(
      iv_session_id = iv_session_id
      is_message    = VALUE #( action = iv_action params = iv_params ) ).
  ev_data = ls_resp-data. ev_error = ls_resp-error.
ENDFUNCTION.
```

That is deliberately **one** FM with the existing envelope, so the ABAP payload
is a ~40-line wrapper and every existing `handle_*` method, every TPDAPI call,
every JSON shape is reused unchanged. It is the smallest possible diff to the
ABAP side and it deletes the entire WebSocket/APC/ICF dependency chain.

Why this works where the WebSocket didn't: `go_dbg`, `mo_dbg_session`,
`mt_bp_mappings` (which holds `REF TO if_tpdapi_bp` — object references that
*cannot* survive a stateless hop) are function-group globals, and function-group
globals live as long as the **RFC connection**, on **one** application server.
That is the affinity the ADR says is missing.

⚠️ **Verify this before building anything.** The claim "FUGR globals persist
across calls on one RFC connection" is standard classic-sRFC behaviour, but it
must be proven on this stack, because open-rfc-go pools sessions:

```abap
FUNCTION z_vsp_probe_state.   " RFC-enabled, ~6 lines
*"  EXPORTING EV_N TYPE I
  gv_n = gv_n + 1. ev_n = gv_n.
ENDFUNCTION.
```

```bash
# expect 1 then 2 on one client; 1 then 1 means no stickiness
vsp rfc call Z_VSP_PROBE_STATE '{}' && vsp rfc call Z_VSP_PROBE_STATE '{}'   # two clients → 1,1
# and from Go, two Call()s on ONE rfc.Client with Pool.MaxSize=1
```

Standard TPDA function modules are **not** an alternative: ✅ all 40 `TPDA*` FMs
on A4H have `FMODE` blank. `RFC_EXT_DEBUGGING_IP(IP)` is ✅ `R` but only registers
an external-debugging IP; it is a hint, not a debugger API.

### 6.2 Which disabled tools come back

| Disabled tool (`pkg/config/systems.go:274-282`) | Over RFC | Why |
|---|---|---|
| `SetBreakpoint`, `GetBreakpoints`, `DeleteBreakpoint` | ✅ yes | Pure TPDAPI static-BP calls; they fail on ADT only because of the 403/CSRF issue (`pkg/adt/debugger.go:145` "DEPRECATED … return 403 CSRF errors") and on WebSocket because the socket is flaky. Neither problem exists on RFC. Breakpoints do not even need a *stateful* session, only a working one — this is the cheapest subset to bring back |
| `DebuggerAttach`, `DebuggerStep`, `DebuggerGetStack`, `DebuggerGetVariables`, `DebuggerDetach` | ✅ yes, **conditional on 6.1** | These are exactly the "session mismatch" victims. They need one pinned conversation for the whole debug session |
| `DebuggerListen` | ⚠️ partly — see 6.3 | It is a *blocking wait*, not a request |
| `AMDPDebugger*` (7 tools) | ❌ don't bother | Their own comment is "session management issues"; the AMDP/HANA debugger is a second, HANA-side protocol whose problems are not transport problems. Fix the ABAP debugger first and revisit |
| `UI5CreateApp/DeleteApp/DeleteFile/UploadFile` | ❌ unrelated | "need alternate API" — a BSP/ICF problem, not a transport one |

### 6.3 The push direction: can callbacks model listen/step?

`DebuggerListen` blocks until a debuggee hits a breakpoint. Two RFC-native
options, and they are genuinely different:

**(a) Just block.** An RFC call may run as long as the work process allows.
`Z_VSP_DBG_CALL(action='listen', timeout=…)` blocks in the RFC work process and
returns the debuggee when one arrives. `rfc.Destination.OperationTimeout` bounds
the Go side; `rdisp/max_wprun_time` bounds the ABAP side. This is exactly what
the ADT long-poll does (`POST /sap/bc/adt/debugger/listeners`, default 240 s,
`pkg/adt/debugger.go:562`) minus the affinity problem. **Start here** — it is
strictly simpler and needs no new protocol.

**(b) RFC callbacks — `DESTINATION 'BACK'`.** open-rfc-go implements
server→client callbacks and they are live-proven
(`rfc/callback.go`, `rfc.Destination.Callbacks`, verified against A4H with
`STFC_CONNECTION_BACK`/`ZSTFC_CONNECTION_BACK` — ✅ `ZSTFC_CONNECTION_BACK`
exists on A4H with `FMODE=R`). While an FM is running on the server, it can call
*back* into our process, and we answer. That is a real push channel over the same
socket, and it is the correct model for **debug events**: the ABAP side, sitting
in its `listen`/`step` call, pushes `Z_VSP_DBG_EVENT` (breakpoint hit, stack
frame, variable batch) into Go as it happens, and the outer call returns only
when the session ends.

Verdict: **(b) is the elegant answer and the one that finally makes streaming
debug honest, but it is not the first move.** It requires the callback handler to
decode raw classic-encoded bytes today (`CallbackRequest.Imports` is
`map[string][]byte`) — open-rfc-go should first give callbacks the same typed
decode that `Client.Call` has (§13). Build (a), then upgrade to (b).

### Where it lands

`pkg/saprfc/debug.go` implementing the same surface as
`pkg/adt/websocket_debug.go`, so `internal/mcp/handlers_debugger.go` and
`handlers_debugger_legacy.go` can switch transport behind one interface;
`ensureDebugWSClient` (`handlers_debugger.go:38`) becomes
`ensureDebugSession(transport)`. The MCP shape does not change at all:
`SAP(action="debug", target="SET_BREAKPOINT"|"ATTACH"|"STEP"|…)`. A single
`Server`-level session cache keyed by system keeps the conversation alive across
MCP tool calls — **this is the piece L1 currently forbids.**

**Risks.** A pinned RFC session holds a work process for the entire debug
session; on a small system that is a real resource (`rdisp/wp_no_dia`).
Debugging still only triggers for code executed in a *different* SAP session
(the existing warning at `handlers_debugger.go:160` stays true). Needs
`S_DEVELOP` with `ACTVT=03` on debugging plus `S_RFC` on `ZVSP_DBG`. And the
whole thing needs new ABAP shipped in ZADT_VSP — so `vsp install zadt-vsp`
(`cmd/vsp/devops.go:3032`, which writes 9 objects via ADT `WriteSource`) grows a
function group, which ADT can create but the current installer only knows how to
write CLAS/INTF/PROG.

**Effort L.** The ABAP facade is S; the Go session lifecycle, the transport
abstraction, and re-enabling + re-testing 9 tools are the L.

---

## 7. Remote ABAP Unit and ATC over RFC · effort **M**

✅ `SATC_AC_AUNIT_REMOTE` is RFC-enabled on A4H:
`IN: I_OBJECTS, I_OBJECT_ATTRIBUTES, I_PARAMETERS` → `OUT: E_RESULT,
E_PROCESSING_INFO`, exception `NOT_AUTHORIZED`. Its siblings
`SATC_AC_REMOTE_EXEMPTION`, `SATC_ACM_VALIDATE_CHECK_MODULE` are also `R`. These
are the FMs SAP's own central ATC uses to drive checks on a satellite system.

vsp runs unit tests and ATC over ADT today (`handlers_testing.go`,
`handlers_atc.go`). The RFC path adds: no CSRF dance, no ICF dependency, works
when ADT is off, and — because it is the *central-ATC* interface — it is designed
for machine consumption and batch object lists rather than for an IDE.

⚠️ The `I_PARAMETERS`/`I_OBJECTS` structures are internal and undocumented; the
first job is `vsp rfc describe SATC_AC_AUNIT_REMOTE` (which already renders the
full JSON Schema, including nested DDIC structures) and then one careful live
call on a package with tests. Verdict: **worth prototyping, not urgent** — the
ADT path here actually works.

**Effort M**, most of it in reverse-engineering the parameter structures.

---

## 8. Transport / CTS over RFC · effort **M**

✅ RFC-enabled on A4H:

| FM | Use |
|---|---|
| `CTS_API_CREATE_CHANGE_REQUEST` | create a request |
| `CTS_API_READ_CHANGE_REQUEST` | read one |
| `CTS_API_IMPORT_CHANGE_REQUEST` | **import** — destructive |
| `CTS_WBO_API_READ_REQUESTS_RFC`, `_READ_PACKAGES_RFC`, `_CHECK_OBJECTS_RFC`, `_INSERT_OBJECTS_RFC` | workbench-object level: which requests exist, what is in them, insert objects |
| `TR_EXT_INSERT_IN_REQUEST`, `TRPF_INSERT_CHANGELIST` | insertion helpers |
| `SVRS_GET_VERSION_DATA`, `SVRS_GET_OBJECTS_PER_COMPONENT`, `SVRS_CHECK_SAME_SYSTEM` | version database — **`SVRS_GET_VERSION_DATA` is interesting**: object version history over RFC, which vsp currently gets from ADT revisions (`handlers_revisions.go`) |

vsp has 5 transport tools over ADT (`pkg/adt/…`, `handlers_transport.go`) plus
10 gCTS tools over `/sap/bc/cts_abapvcs` (`pkg/adt/gcts.go:98`) that are *not
even reachable from the universal `SAP()` tool* — there is no `routeGctsAction`
in the route chain at `handlers_universal.go:77-104`. So the transport surface is
already fragmented. Adding a third transport is only justified for what ADT
cannot do: **cross-system import** (`CTS_API_IMPORT_CHANGE_REQUEST` against a
*target* system's RFC destination) and **version history**.

**Risks are the highest in this document.** Importing a transport is irreversible
and system-wide. Any RFC transport tool must be write-gated, must never be
exposed in `--safe` mode, and should require an explicit per-system opt-in in
`.vsp.json`. `S_TRANSPRT`, `S_CTS_ADMI`.

**Effort M.** Verdict: **worth prototyping the read half** (`_READ_REQUESTS_RFC`,
`SVRS_GET_VERSION_DATA`); **hold** the import half until the write-safety gate
exists.

---

## 9. RFC-only mode — a system with no ZADT_VSP and no ADT · effort **M**

Today most interesting vsp tools have a prerequisite: either ADT/ICF is up, or
ZADT_VSP is installed (`vsp config` literally groups tools as "requires
ZADT_VSP"). RFC lets vsp be useful on a system where **neither** holds. What
survives, all ✅ `FMODE='R'` on A4H:

| Capability | FMs |
|---|---|
| Read repository source | `RPY_PROGRAM_READ`, `RPY_INCLUDE_READ`, `RPY_TABLE_READ`, `RPY_TABLE_READ_SHORT`, `RPY_DOMAIN_READ`, `RPY_DATAELEMENT_READ`, `RPY_MESSAGE_READ`, `RPY_DOCU_READ`, `RPY_DIALOG_READ` |
| Enumerate the repository | `RPY_PROGRAM_SELECT`, `RPY_INCLUDE_SELECT`, `RPY_DOMAIN_SELECT`, `RPY_DATAELEMENT_SELECT` + `RFC_READ_TABLE TADIR/TRDIR/REPOSRC` |
| **Write** repository objects | `RPY_PROGRAM_INSERT`, `RPY_INCLUDE_INSERT`, `RPY_INCLUDE_UPDATE`, `RPY_TABLE_INSERT`, `RPY_DOMAIN_INSERT/UPDATE`, `RPY_DATAELEMENT_INSERT/UPDATE`, `RPY_DOCU_WRITE`, `RPY_TEXTELEMENTS_INSERT` |
| Regenerate a program | `RFC_GENERATE_REPORT(I_PROGRAM) → O_GEN_SUBRC` |
| Class metadata | `SEO_CLASS_TYPEINFO_GET_RFC` |
| Short dumps | `RS_ST22_RFC` — ✅ note `RFC_READ_TABLE SNAP` fails `TABLE_NOT_AVAILABLE`, so this is the *only* RFC route to ST22 |
| Users | `BAPI_USER_GET_DETAIL`, `BAPI_USER_UNLOCK` (already used as the account-lock safety net) |
| Run anything | §5 |
| Export a package | §2 |
| OS-level commands | `SXPG_COMMAND_EXECUTE`, `SXPG_COMMAND_EXECUTE_LONG`, `SXPG_COMMAND_LIST_GET`, `SXPG_CALL_SYSTEM` |

Two honest gaps: **there is no RFC-enabled `RFC_ABAP_INSTALL_AND_RUN` on this
system** (✅ checked — only `/BODS/RFC_ABAP_INSTALL_AND_RUN` from a Data Services
add-on exists), so "compile and run arbitrary ABAP in one call" is not available;
and `RFC_GENERATE_REPORT` only *regenerates an existing* program. The
`RPY_PROGRAM_INSERT` + `RFC_GENERATE_REPORT` pair gets you there in two steps, but
it writes a real repository object with all that implies (TADIR, transport,
locks).

**`SXPG_COMMAND_EXECUTE` deserves its own warning.** It is ✅ RFC-enabled and
✅ my test user is authorized for it. It executes OS commands as `<sid>adm`. It
is one of the classic SAP attack primitives. **vsp should never expose it as a
tool** — not in expert mode, not behind a flag. If it appears at all, it appears
in the probe output as a *risk finding* ("this user can run OS commands over
RFC"), never as a capability.

**Where it lands.** A `--transport rfc` mode on the existing read/search commands
and a documented "RFC-only" tool subset in `tools_groups.go`, so
`vsp -s legacy search 'Z*'` works against a system that has never heard of ADT.

**Effort M.** Verdict: **worth prototyping the read half.** The write half
(`RPY_*_INSERT`) is a trap — see §12.

---

## 10. The server side: ABAP calls *into* vsp · effort **L**, and be honest about it

open-rfc-go has a server leg (`internal/rfcserver`, `cmd/rfc-lab`) that answers
as an SM59 **type-3** destination — every SM59 test button green, and a real ABAP
program (`ZLOCAL_RFC_TEST`) running six parametrized calls at `rc=0`. The
`docs/polyglot-rfc-server.md` vision is: expose any Go/Python/Rust function as an
ABAP-callable function module, and let vsp generate the ABAP proxy.

Applied to vsp, the idea is genuinely new: **an ABAP program calls out to vsp**,
and vsp answers with something ABAP cannot compute — an LLM completion, an
abaplint run (vsp embeds `abaplint-lexer.zip`), a graph query, a Lua script.
`Z_VSP_LINT(source) → findings`, `Z_VSP_ASK(prompt) → answer`, evaluated in Go.
It inverts the whole tool: instead of an AI driving SAP, SAP consults the AI.

**Now the honest part.** Three things stand between here and that:

1. **Registered-server (type T) is not implemented.** The roadmap lists
   "Registered/inbound RFC server" as **P2, not done**. What works today is type
   3: SM59 points at *our* host:port and we answer as if we were an application
   server. That means an inbound firewall hole and a static endpoint — the
   opposite of the type-T model where we dial out and register a Program ID.
   ✅ A4H already has both shapes configured (`RFCDES`: seven type `3` `A4H@*`
   destinations plus `A4H@PROXY-TYPE-NON-ABAP` of type `T`), so the target is
   real and the comparison is testable.
2. **The generating server is WIP.** Two research pieces remain: generating the
   logon-accept from the client's `init` (it is init-dependent; template
   *selection* does not scale past the fixed SM59 tests to a real calling
   program) and a fast-serialization Delta Manager encoder for table results.
   Until #1 lands, an arbitrary ABAP program cannot log on to us.
3. **No SNC, no authentication of the caller.** A type-3 endpoint we operate has
   no way to verify who is calling, and the conversation is plaintext.

**Verdict: strategically the most exciting item, tactically the least ready.**
It belongs in open-rfc-go's roadmap, not vsp's. What vsp *can* do cheaply today
is the mirror image: `Z_CALL_RFC` (✅ exists on A4H: `DESTINATION`, `N`, `NAME` →
`GREETING`, `RESULT`, `RC01`, `RC02`) is already the dual-destination driver used
to make ABAP call our Go endpoint. Keep it as an integration test, not a feature.

---

## 11. RFC as the fallback transport · effort **S**

Small, cheap, and quietly valuable. ADT lives in ICF; ICF nodes get deactivated,
`/sap/bc/adt` gets blocked by a reverse proxy, CSRF tokens expire, the WebSocket
node 500s. The gateway is a different port, a different daemon, and a different
failure domain. ✅ On A4H, `3300` is open on all three hosts including through
the public port-forward, and `44300` is closed everywhere — i.e. **there is
already a topology here where RFC reaches further than HTTPS does.**

**Where it lands.** `pkg/adt/client.go`'s error path and `handlers_health.go`:
when an ADT call fails with a connection error or 404 on the discovery document,
emit a diagnostic that says *"ADT unreachable; RFC to `host:3300` is up — try
`vsp rfc info`"* instead of a bare transport error. Then, for the handful of
operations that have both implementations (§2, §4, §5, §9), fall back
automatically with a loud note in the result.

**Effort S.** Do it as part of §3 — the probe already knows both answers.

---

## 12. Ideas to reject, and why

| Idea | Verdict |
|---|---|
| **Replace ADT source read/write/activate with `RPY_*`** | ❌ **Don't.** `RPY_PROGRAM_INSERT` and friends bypass the ADT lock protocol, the syntax check, the activation queue and the inactive-object handling that `pkg/adt/workflows_deploy.go` gets right. vsp's whole write story is built on ADT locking (there is even a known affinity bug for lock handles, `pkg/adt/http.go:404`). Adding a second write path that does not participate in it is how you corrupt a repository. Read over RFC, write over ADT |
| **AMDP debugger over RFC** | ❌ **Don't.** The 7 AMDP tools are disabled for "session management issues" and the AMDP debugger is a HANA-side protocol. Changing the ABAP transport does not touch the actual defect |
| **abapGit push/pull/staging/commit over RFC without new ABAP** | ❌ **Not possible.** ✅ Only two `Z_ABAPGIT*` FMs exist and both serialize. A deserialize wrapper is easy to write but it *is* new ABAP, so it does not qualify as "no install needed". Ship it inside ZADT_VSP and be explicit about the dependency |
| **Expose `SXPG_COMMAND_EXECUTE` as a tool** | ❌ **Never.** See §9 |
| **`DEQUEUE_*` / lock-breaking over RFC** | ❌ Not available anyway (✅ no `DEQUEUE_*` FM is RFC-enabled), and that is the right default |
| **Use `RFC_READ_TABLE` as a general SQL engine** | ❌ No joins, no aggregates, no subqueries. It is a table reader. Keep ADT freestyle for SQL |
| **Anything against production over plaintext RFC** | ❌ open-rfc-go has **no SNC** — password scrambling is a published table, not encryption (`SECURITY.md`, `docs/live-test-plan.md`). Trusted segment or SAProuter/SOCKS5 tunnel only. vsp should refuse to dial RFC to a system flagged `production` in `.vsp.json` unless a tunnel is configured |
| **A per-FM MCP tool explosion in vsp** | ❌ vsp deliberately went the other way — `hyperfocused` (1 tool) is the *default* (`cmd/vsp/main.go:137`), `focused` is 102, `expert` is 150. open-rfc-go's own `rfc mcp --expose` autodiscovery is the right home for per-FM tools; inside vsp, RFC stays one action |

---

## 13. What open-rfc-go should add for vsp

These are the library gaps this document keeps hitting. All belong upstream.

| # | Gap | Why vsp needs it |
|---|---|---|
| G1 | **An exported sticky session** — `Client.Session(ctx) (*Session, release)` that pins one pooled connection for N calls | §6 is impossible without it; §5 and §2 get faster with it. Today `Call` takes a fresh lease each time (`rfc/call.go:64`) from a `MaxSize: 8` pool |
| G2 | **Typed callbacks** — `CallbackRequest.Imports` is `map[string][]byte`; give it the same interface-driven decode `Client.Call` has | §6.3 push-mode debug events |
| G3 | **A public `ReadTable`** with `OPTIONS` as a row list, `ROWSKIPS`, `NO_DATA`, and `USE_ET_DATA_4_RETURN` | Already on open-rfc-go's roadmap as P1 "typed `ReadTable`"; vsp has two copies of a worse version |
| G4 | **`XSTRING` ergonomics** — `Result.Bytes(name)` | `EV_XSTRING` from §2 comes back as a hex/base64 string today |
| G5 | Observability hooks (`log/slog`) | vsp has `--verbose`; RFC calls are currently invisible |

---

## 14. Ranked shortlist

### Do next

| Rank | Item | Effort | Why now |
|---|---|---|---|
| 1 | **Fix L1–L4** — promote `rfcReadTable` to `pkg/saprfc`, multi-row `OPTIONS`, `USE_ET_DATA_4_RETURN`, `ROWSKIPS`, and a cached per-system `*rfc.Client` instead of open/close per MCP call | S | Every other item depends on it, and L2 is a *silent correctness bug* today (WHERE clauses truncate at 72 chars) |
| 2 | **abapGit export over RFC** (§2) | S | Live-proven; replaces the most fragile chain in the product with one call; removes the ZADT_VSP dependency from `vsp export` |
| 3 | **`vsp rfc probe`** (§3) + the ADT-down fallback hint (§11) | S | Cheap, read-only, and `RFC_SIMULATE_AUTH_CHECK` answers a question no other transport can |
| 4 | **Reports and jobs over RFC** (§5) | M | Closes issue #55, un-disables an experimental tool group, and removes the Go↔ABAP contract drift in `handlers_report.go` |

### Worth prototyping

| Rank | Item | Effort | Gate |
|---|---|---|---|
| 5 | **Breakpoints over RFC** (§6.2, first row only) | M | Needs `Z_VSP_DBG_CALL`, but *not* session stickiness. The cheapest way to bring disabled tools back |
| 6 | **Full stateful debug session** (§6.1/6.3, option (a)) | L | Blocked on G1 and on the `Z_VSP_PROBE_STATE` verification |
| 7 | **RFC-only read mode** (§9, read half) | M | Useful the first time someone points vsp at a system without ADT |
| 8 | **Transport read half + `SVRS_GET_VERSION_DATA`** (§8) | M | Import half stays behind a write gate |
| 9 | **Remote ABAP Unit** (§7) | M | ADT path works; do it when the parameter structures are cheap to decode |
| 10 | **Debug events over callbacks** (§6.3, option (b)) | L | Blocked on G2. The elegant end-state |

### Don't bother

§12, in full. In one line each: **`RPY_*` writes** (bypasses the lock/activate
protocol), **AMDP over RFC** (wrong defect), **`SXPG_COMMAND_EXECUTE`** (attack
primitive), **`RFC_READ_TABLE` as SQL** (no joins), **per-FM MCP tools in vsp**
(against the hyperfocused design), **plaintext RFC to production** (no SNC).

---

## 15. The first prototype — exact commands

Everything below except the two marked ⚠️ was **run today** against A4H
(`<lan-ip>`, gateway `3300`, client `001`, sysnr `00`). Credentials come
from `.vsp.json` / `SAP_USER`+`SAP_PASSWORD`; none appear here.

```bash
# 0 — reachability and identity (baseline; 2 s)
nc -z <lan-ip> 3300
vsp rfc info                     # ✅ A4H / 758 / kernel 793 / HDB / cp 4103

# 1 — is abapGit's RFC serializer here, and is it the dev version?
vsp rfc search 'Z_ABAPGIT_*'     # ✅ Z_ABAPGIT_SERIALIZE_PACKAGE, _PARALLEL (FMODE=R)
vsp rfc describe Z_ABAPGIT_SERIALIZE_PACKAGE

# 2 — THE PROTOTYPE: export a package as an abapGit zip in one RFC call
vsp rfc call Z_ABAPGIT_SERIALIZE_PACKAGE \
  '{"IV_PACKAGE":"Z_BADI_CHECK","IV_FOLDER_LOGIC":"PREFIX","IV_MAIN_LANG_ONLY":"X"}' \
  > pkg.json
#   ✅ EV_XSTRING → 10 263 bytes, valid ZIP, 9 entries incl. .abapgit.xml
#   compare against the existing path:  vsp export Z_BADI_CHECK -o ws.zip
#   (which needs ZADT_VSP + a working APC WebSocket)

# 3 — the probe primitives
vsp rfc read-table CVERS --fields COMPONENT,RELEASE,EXTRELEASE --top 20   # ✅
vsp rfc call RFC_SIMULATE_AUTH_CHECK '{"IV_FM":"RFC_READ_TABLE","IV_USER":"<USER>"}'
#   ✅ {"EV_AUTHORIZED":"X"}
vsp rfc read-table TADIR --fields OBJECT,OBJ_NAME,DEVCLASS \
  --where "OBJ_NAME LIKE 'ZCL_VSP%'"                                      # ✅ 9 classes → ZADT_VSP present

# 4 — the read-table limits, and the fix (this is L2/L3, live)
vsp rfc read-table USR02 --top 1
#   ✅ FAILS: ABAP exception DATA_BUFFER_EXCEEDED (AD559)
vsp rfc call RFC_READ_TABLE \
  '{"QUERY_TABLE":"USR02","DELIMITER":"|","ROWCOUNT":1,"USE_ET_DATA_4_RETURN":"X"}'
#   ✅ SUCCEEDS: ET_DATA rows (SDTI_RESULT-LINE, type STRING), 44 FIELDS
vsp rfc call RFC_READ_TABLE '{"QUERY_TABLE":"TADIR","DELIMITER":"|","ROWCOUNT":3,
  "FIELDS":[{"FIELDNAME":"OBJECT"},{"FIELDNAME":"OBJ_NAME"}],
  "OPTIONS":[{"TEXT":"DEVCLASS = '\''Z_BADI_CHECK'\'' AND"},{"TEXT":"OBJECT = '\''PROG'\''"}]}'
#   ✅ multi-row OPTIONS concatenate — the 72-char ceiling is per row, not per clause

# 5 — the report/job loop (read-only describes first)
vsp rfc describe SUBST_START_REPORT_IN_BATCH      # ✅ EV_JOBCOUNT, EV_STARTRC, TT_REPORTPARAM
vsp rfc describe BAPI_XBP_JOB_SPOOLLIST_READ      # ✅ SPOOL_LIST, RETURN
# ⚠️ then, on a scratch report only:
#   BAPI_XMI_LOGON → SUBST_START_REPORT_IN_BATCH → BAPI_XBP_JOBLIST_STATUS_GET
#   → BAPI_XBP_JOB_SPOOLLIST_READ → BAPI_XMI_LOGOFF

# 6 — ⚠️ the one experiment that gates the debugger (needs ~6 lines of new ABAP)
#   deploy FUGR ZVSP_PROBE with a global gv_n and RFC-enabled Z_VSP_PROBE_STATE,
#   then two Call()s on ONE rfc.Client with Pool.MaxSize=1:
#     1,2 → function-group state survives; §6 is buildable
#     1,1 → it does not; §6 needs a different design
```

Negative results worth recording, all ✅ checked today: no `RFC_ABAP_INSTALL_AND_RUN`
(only a `/BODS/` add-on variant); all 40 `TPDA*` FMs are non-remote-enabled; no
`JOB_*` FM is remote-enabled; no `DEQUEUE_*` FM is remote-enabled;
`RFC_READ_TABLE SNAP` returns `TABLE_NOT_AVAILABLE`.

---

## 16. Cross-cutting risks

- **No SNC.** Classic RFC as implemented has exactly one auth mechanism: user +
  password, scrambled with a published table. Trusted segment, tunnel, or
  nothing. vsp should mark RFC-capable systems in `.vsp.json` and refuse
  production without `rfc_router`/SOCKS5.
- **`S_RFC` is coarse.** Authorization is per *function group*, not per function
  module — granting `SDIFRUNTIME` for `RFC_READ_TABLE` grants the whole group.
  `RFC_SIMULATE_AUTH_CHECK` lets vsp tell the user exactly what they are about to
  need.
- **Write safety.** vsp's `pkg/adt/safety.go` op gates (`OpFreeSQL`,
  `OpWorkflow`, …) cover ADT only. Every RFC write — job scheduling, transport,
  `RPY_*` — must join that system before it ships. open-rfc-go's `--safe`
  heuristic (mutating name verbs, `BAPI_TRANSACTION_COMMIT`) is a starting point,
  not a policy.
- **No implicit commit.** RFC does not commit. A BAPI flow that ends without
  `BAPI_TRANSACTION_COMMIT` silently rolls back at connection close — and with
  L1's open/close-per-call, *every* MCP call closes the connection. Any future
  BAPI-flow support must own its session (G1) and its commit explicitly.
- **Work-process pressure.** Pinned sessions (§6) and long polls (§5, §6.3) each
  hold a dialog work process. Bound the pool.
- **Audit noise.** `BAPI_XMI_LOGON` writes an XMI log entry per session;
  `SM59`/`SMGW` and the security audit log will show a new external program
  (`Destination.ProgramName`, default `open-rfc`). Set it to something
  identifiable — `vsp` — so an administrator can tell what is calling.
