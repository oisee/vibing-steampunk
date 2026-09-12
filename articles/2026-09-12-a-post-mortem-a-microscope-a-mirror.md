# A Post-Mortem, a Microscope, and a Mirror

**Or: the 35 instruments `vsp` grew since April — and why every one of them answers a *why*, not a *what***

---

In April, `vsp` was a very good remote control for SAP's ADT. Since then it grew **35 new features**, and they all point at the same thing: not *what* the system is, but **why** it broke, what it was carrying when it did, and whether the tool that told you can even be believed.

**The scoreboard for the tools alone:** 35 features across **19 releases**, from **v2.38 (Apr 7)** to **v2.57 (Sep 10)** — ~55 CLI commands, 151 MCP tools, 1,362 tests.

## One release in April. Then a week that shipped fifteen.

The history isn't a steady drip. It's a four-month silence followed by a burst:

- **v2.38** — Apr 7
- *— quiet, four months —*
- **v2.40 → v2.54** — **Aug 20–27** (fifteen releases in eight days)
- **v2.55 → v2.57** — early September

Eighteen releases in twenty-two days after a four-month gap. Nearly every feature below lands in that band.

## Thirty-five features, seven families

| Family | Features |
|---|:---:|
| 🔬 RCA / dumps | 8 |
| 🪞 Platform & self-audit | 8 |
| 🕸️ Graphs | 7 |
| 🐞 Debuggers | 4 |
| 🧭 Context | 3 |
| 🗄️ Offline reads | 3 |
| 📞 Classic RFC | 2 |

---

## The headliner: dump correlations

RCA stopped handing you a stack and started handing you the **reason**.

A dump is never alone. It sat inside a running program that wrote to logs on its way down; it shares a signature with the dumps before it; it has a blast radius. Since **v2.43** the post-mortem learned to *correlate* — to find the log entry that belongs to *this* failure and the frame that wrote it — and by **v2.57** to open with the reason itself. From a single `MESSAGE_TYPE_X` in `SAPLOLEA`, the correlations fan out:

- **The log around it** — rank the application-log entries written near the failure. (v2.43)
- **The frame that logged** — which stack frame wrote a given log entry. (v2.43)
- **The graph rung** — a log written by something a stack frame *calls*. (v2.43)
- **The similar-dump ladder** — other dumps that share this one's signature. (v2.44)
- **Blast radius** — up the call graph, what a recurring one touches. (v2.44)
- **The whole "why"** — message coordinates, `SY-*` fields, the failing source line, per-frame variables. (v2.57)

The null pointer shows up *as its value* — `RESULT_VARS_WA … POINTER = 0x0` — all from the one document ST22 already serves. **No extra round trips.**

```
message SY 373 (type X)  with -1
SUBRC=4  FDPOS=40  PFKEY=SESSION_ADMIN  TITLE=SAP Easy Access
source:
    530  else.
  > 531  MESSAGE X373 WITH '-2'.     "===============> Error
    532  endif.
```

---

## All thirty-five, by family

### 🔬 RCA / dumps — 8

1. **Read & group runtime errors** by signature — `v2.43`
2. **Correlate dump ↔ application log** written around it — `v2.43`
3. Rank a log **by the stack frame** that wrote it — `v2.43`
4. **Graph-rung correlation** — a log written by a frame's callee — `v2.43`
5. **Similar-dump ladder** — `v2.44`
6. **Blast-radius graph-up** — `v2.44`
7. The full **"why" detail** — message coords, `SY-*` fields, failing line, per-frame variables — `v2.57`
8. Post-mortem reachable from MCP; a 7.50 fallback for releases with no detail resource — `v2.44`

### 🕸️ Graphs — 7

9. **Directional boundary crossings** + standalone `vsp boundaries` — `v2.40`
10. **Graph export** — DOT / PlantUML / GraphML / Mermaid subgraphs — `v2.40`
11. **Side-effect + LUW classification**; extract CALL TRANSACTION / TRANSFORMATION / LEAVE — `v2.40`, reachable `v2.47`
12. **D010INC load graph** — `vsp loads`, the one source that is *not* a cross-reference — `v2.47`
13. **Method-include decode** → upward tracing unblocked — `v2.48`
14. `graph_stats` over an object or package (not just pasted source); inactive index isolated — `v2.47`
15. `references` resolves the **section** a cross-reference row points at — `v2.49`

### 🪞 Platform & self-audit — 8

16. **`vsp sweep` with an oracle** — tells `dead` from `no results` — `v2.46`
17. Found **three capabilities that had never worked** (green tests asserting shapes nobody received) — `v2.52`
18. **Three-system record** — swept clean on 7.58 / 7.57 / 7.50 — `v2.51`
19. **One declaration per capability**, everything else derived — `v2.52`
20. The universal tool reaches **every** advertised capability — `v2.51`
21. **Hyperfocused mode by default** — `v2.40`
22. Defaults tuned for a terminal and paid for in a context window — `v2.54`
23. An empty `SAP()` call answers **"what am I talking to?"** — `v2.53`

### 🐞 Debuggers — 4

24. **ABAP debugger over plain HTTPS** — breakpoints, read/write variables, stack frames, no Z code — `v2.42`
25. **AMDP debugging over ADT** — a breakpoint fires inside HANA; statement-level SQLScript trace + variable read — `v2.44`, `v2.45`
26. **`vsp trace`** — a measured call tree, and a unit recorded statement-by-statement with values — `v2.42`
27. A **local debug UI** — `v2.55`

### 🧭 Context that knows who calls you — 3

28. Contract **narrowed to the methods the code actually calls** — smaller contexts — `v2.50`
29. Reverse **"who calls this"** — which the source itself cannot say — `v2.50`
30. Rank candidates by what a reader needs; sees `CREATE OBJECT` dependencies — `v2.49`, `v2.50`

### 🗄️ Offline reads — 3

31. **Cluster decode** — BALDAT / INDX / STXL over plain ADT; LZH = DEFLATE, deep data, version 5, offline — `v2.55`, `v2.56`
32. **Application log without RFC** — richer than `BAPI_APPLICATIONLOG_GETDETAIL`, no gateway — `v2.43`, `v2.55`
33. **SM37 / SP01 as tables** + TemSe spool; `vsp inspect`: variants, SE37 test data, SE61 docs, the IMG — `v2.56`

### 📞 Classic RFC, pure Go — 2

34. **`vsp rfc`** — call any function module, no NetWeaver SDK, no cgo — `v2.40`
35. `rfc probe` / `rfc export` (abapGit ZIP) / run reports as jobs / **ADT REST over the RFC tunnel** — `v2.41`

**Plus the plumbing that made those land:** `texts` / `description` / DDIC-table-as-source (v2.55–56); CR-config-audit and tr/cr-boundaries and co-change (v2.40); transport merge/move through `ZADT_VSP` (v2.57); `landscape` / `detect` / `compat` discovery (v2.42); SAML SSO for S/4 Public Cloud (v2.40); `recover-failed-create` (v2.40).

---

## The thread through all of it

> April's `vsp` could tell you the **state** of the system. This period it learned to tell you the **cause**.

The dump says why it stopped; the cluster says what it was carrying when it did; the variant says what it was told to do; the graph says who else it touches; and `sweep` says whether the tool that answered can be believed. Every one of the thirty-five answers a *why*, not a *what* — and none of it is worth much if the instrument itself lies, which is exactly why the mirror shipped alongside the microscope.

---

**GitHub**: [oisee/vibing-steampunk](https://github.com/oisee/vibing-steampunk) · **v2.57.0** · 466 ★ · 72 releases · ~55 CLI commands · 151 MCP tools · 1,362 tests

*Part of a series — "Agentic ABAP" (Dec 2025) · "…at 100 Stars" (Feb 2026) · "VSP Is Only 5% Explored" (Apr 2026) · "…Still Only 5%" (Aug 2026) · "The Frontier Went Down a Layer" (Sep 2026).*

#ABAP #SAP #MCP #ClaudeCode #GoLang #OpenSource #RootCauseAnalysis #ST22 #ClusterTables #DataCluster #RFC #Debugging #VSP
