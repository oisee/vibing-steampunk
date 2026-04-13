# cr-config-audit v2a follow-ups — parked

**Status:** parked 2026-04-13. v2a.1 (SELECT path + constant resolution) is
landed; the four items below are deferred until a real-world CR exercises
them. Picking these up later does not require any new design work — the
plan is fully sketched here.

## Current state (v2a.1 landed)

`vsp cr-config-audit <cr> --value-level` already does:

- CALL FUNCTION literal extraction against a hardcoded SAP customizing FM
  registry (`known_calls.go`) — APPL_LOG_INIT, APPL_LOG_SET_OBJECT_AND_SUBOBJECT,
  NUMBER_GET_NEXT.
- Direct SELECT/UPDATE/MODIFY/DELETE WHERE-literal extraction with constant
  and host-var (`@var`) resolution from a file-scoped local literal map
  (`CONSTANTS x VALUE 'lit'`, `DATA x VALUE 'lit'`, `x = 'lit'`).
- TABKEY unpack via DD03L key-field metadata, MANDT-aware.
- Subset matching with IncompleteKey flag.
- Path A (CROSS TYPE='F') for known-FM callers, Path B (codeTables ∩
  custTables) for SELECT callers — derived from already-collected
  WBCROSSGT data, no extra SAP queries.
- Per-object L2 cache for CROSS/WBCROSSGT scans (1h TTL) plus L2 cache
  for DD02L (24h) and DDIC walk results (24h).
- Drop-counter logging with `VSP_VALUE_DEBUG=1` for non-key WHERE,
  unknown target, no-DD03L cases.

A live run on a real S/4HANA CR confirmed the mechanics work: 6 SELECT
findings extracted, all targeting SAP repository tables (SEOCLASS) on
non-key WHERE predicates, all correctly dropped by the matcher.

## v2a.1.1 — SAP-standard table filter (quick, ~30 LOC)

**Problem:** the SELECT extractor currently runs against any DDIC table
the code touches, including SAP repository (SEOCLASS, T000, MARA). These
are never customizing and any matching attempt is structurally
meaningless. The drop-counter eats the noise but the wasted source-fetch
and SAP round-trips are real.

**Fix:** in `extractFromSQL`, after `plausibleTableName(tableUp)`, also
require `!graph.IsStandardObject(tableUp)`. This drops every reference
to SAP-namespace tables before we even build the finding. The same
guard belongs in the matcher as a defensive second layer.

**Cost:** ~30 LOC + one unit test that asserts SEOCLASS-style SELECTs
disappear from extractor output.

## v2a.2 — full row content fetch (~150 LOC)

**Problem:** TABKEY only carries primary-key bytes. When code does
`WHERE module = 'AUTH' AND key = 'TIMEOUT'` against a Z-table whose
business meaning lives in non-key fields, our matcher has nothing to
compare against — the code-side constraint can never be verified
(unless `module` and `key` happen to be primary key fields).

**Fix:** for each Z-table in `custTables`, fetch the actual row content
from SAP via `RunQuery("SELECT * FROM <ztable> WHERE <reconstructed>")`
keyed by the transported TABKEYs. Reconstruction comes from DD03L key
field widths (we already have `unpackTabkey`). Cache the rows in L2
sqlite under `tablecontents:<system>:<table>:<tabkey-hash>`.

The matcher then runs subset-match on the **full row** field map, not
just the unpacked TABKEY map. Non-key WHERE predicates become
verifiable. The "only key fields" filter in `extractFromSQL` is then
relaxed to "all fields, but require the table to be in custTables and
to be custom-namespace".

**Cost:**
- New `fetchTransportedRowContents(table, tabkeys, cache)` helper
- ~80 LOC fetch + cache + reconstruction
- ~30 LOC matcher refactor to consume full rows
- ~40 LOC tests (mocked client returns synthetic rows, matcher
  classifies a known fixture)
- Total: ~150 LOC

**Why this matters:** the user's recent question was "tracking on all
fields, not just key" — this is the right way to honour that. Without
this fetch we either drop too much (current state) or generate noise
matched against TABKEY-only data (wrong).

## v2a.3 — single-hop Z-FM transitive propagation (~200 LOC)

**Problem:** `known_calls.go` has 3 SAP FMs hardcoded. Any custom
function module — `Z_GET_CONFIG`, `Z_LOG_WRITE`, etc. — is invisible to
the value-level audit even though the customer-specific bug class lives
exactly there.

**Fix:** when the extractor sees a CALL FUNCTION to a Z-namespace FM
(not in the registry), fetch the FM source via `client.GetFunction(fm,
group)`, synthesise a `localLiterals` map from the caller's literal
EXPORTING parameters (`{module: 'AUTH', key: 'TIMEOUT'}`), and run the
extractor recursively on the FM source. Findings emitted by the inner
pass are tagged with `Via: "CALL_FUNCTION:Z_FOO → SELECT:ZCONFIG"` and
their `SourceObject` stays set to the original caller for traceability.

**Hop limit:** 1. If `Z_FOO` itself calls another `Z_BAR`, we stop
there for v2a.3. Full N-hop propagation is v3 territory and needs a
recursion guard, cycle detection, and a depth budget.

**Cost:**
- `extractFromCallFunctionTransitive` entry point (~60 LOC)
- Per-FM source cache in L2 sqlite (`fmsource:<system>:<group>:<fm>`,
  short TTL because customer code edits frequently)
- Recursion guard so an FM that calls itself does not loop
- Modify the existing `extractCodeLiterals` driver to recognise Z-FMs
  and dispatch to the transitive path
- Tests covering: synthetic `Z_GET_CONFIG` reads `WHERE module = im_module`,
  caller passes `im_module = 'AUTH'`, finding lands on caller with the
  right `Via` chain
- Total: ~200 LOC

**Why this matters:** the registry approach scales O(N) in customer
maintenance burden. Transitive walking scales O(1) — every Z-FM that
uses its parameters as table keys gets caught automatically.

## v3 — full transitive value-flow (large, separate design)

The v2a.3 hop limit of 1 is deliberate. Full data-flow — `lv = SELECT
mode FROM ztab WHERE k = 'LIT'`, `lv` flows through method calls and
assignments, eventually feeds an `IF lv = 'STORAGE'` — needs:

- Symbol tracking inside method bodies (provenance per local var)
- Parameter binding across method call boundaries (call graph)
- Field access on structures and class attributes
- Comparison-as-implicit-expectation pattern recognition
- Cycle detection, recursion budget, scope rules

This is a separate effort, ~500-800 LOC, with its own design report.
Not started here. The v2a.3 single-hop case is the 80% of the value at
20% of the cost.

## Suggested order when picking back up

1. **v2a.1.1 SAP-standard filter** (30 LOC, 30 minutes) — eliminates
   the SEOCLASS-class noise immediately. Unblocks honest signal-to-
   noise measurement on real CRs.
2. **v2a.3 single-hop Z-FM propagation** (200 LOC, half a day) —
   biggest signal expansion, removes the "registry maintenance"
   ceiling. Test on the same prod CR before/after to measure the
   findings delta.
3. **v2a.2 full row content fetch** (150 LOC, half a day) — closes the
   "non-key field" gap on Z-tables. Combine with v2a.3 and you get
   real prod-incident-class coverage without per-customer hardcoding.
4. **v3 full data-flow** — only after the above three have been
   exercised on multiple real CRs and the cost/benefit ratio is clear.

## Related artefacts

- `cmd/vsp/value_extractor.go` — CALL FUNCTION + SELECT extractor
- `cmd/vsp/known_calls.go` — current 3-entry FM registry
- `cmd/vsp/value_match.go` — matcher with key-field filter and drop counters
- `cmd/vsp/tabkey.go` — DD03L-driven TABKEY unpacker
- `cmd/vsp/audit_cache.go` — two-tier L2 sqlite cache
- `cmd/vsp/value_extractor_test.go` — 16 unit tests covering both paths

## Non-goals at every stage

- No regex-based ABAP analysis (hard rule, see project memory).
- No caching of E070/E071/E071K transport content — must always be fresh.
- No SAP-standard table customizing analysis — out of scope by definition.
