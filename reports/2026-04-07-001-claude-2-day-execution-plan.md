# Claude 2-Day Execution Plan

**Date:** 2026-04-07
**Scope:** Focused 2-day plan for Claude while the main thread is quiet
**Goal:** Maximize leverage without starting broad new product work

## Operating Principle

Do not start with new shiny features.
Use these 2 days to reduce leverage debt and unblock the next cycle.

Priority order:

1. blocker verification and scoping
2. shared internal consolidation
3. passive data collection groundwork
4. only then small product-facing follow-ups if time remains

## Hard Constraints

- Do not start generic `surface` CLI.
- Do not add new narrow reader commands.
- Do not start execution refactors.
- Do not start graph DB productization.
- Do not touch auth/role graph, E071K customizing graph, or cross-system diff.
- Prefer small merged slices over one large branch of speculative work.

## Day 1 — Architecture Leverage

### Track A. Verify the blocker picture first — ✅ DONE (2026-04-07)

**AMENDMENT:** Both blockers investigated, root-caused, and fixed in commit `27f4d7c`.

- Issue #90: Added `CheckRedirect` to preserve Authorization on redirects (`pkg/adt/config.go`)
- Issue #88: Added per-request `Stateful` flag for lock→write→unlock (`pkg/adt/http.go`, `crud.go`)
- Root cause memo: `reports/2026-04-07-002-blocker-verification-memo.md`
- Both fixes need user validation on affected SAP systems (no BTP/7.51 access for testing)

### Track B. Extract shared package acquisition helper

This is the highest-ROI internal consolidation.

Target:

- package/object inventory
- reverse refs from `WBCROSSGT` / `CROSS`
- exact-name post-filtering
- include normalization

Current duplicated consumers:

- `deps`
- `api-surface`
- `slim`
- package-mode `health`

Desired result:

- one shared internal helper layer
- no user-facing CLI changes
- no semantic regressions

Suggested shape:

- new internal package or `pkg/acquire`
- helpers like:
  - `FetchPackageContents(...)`
  - `FetchPackageReverseRefs(...)`
  - `NormalizeAndFilterRefs(...)`

Deliverable:

- merged internal refactor
- command behavior preserved
- tests/build green

Timebox:

- rest of Day 1

Success condition:

- at least `api-surface` and `slim` moved onto the shared helper
- `health` migrated if cheap and clean

## Day 2 — Foundation for Next Cycle

### Track C. Passive data collection groundwork

Implement cheap in-memory caching hooks only where nearly free.

Good candidates:

- cache transport metadata on `co-change` queries (in-memory by default)
- cache reverse-ref edge snapshots (in-memory by default)
- fingerprint `GetSource` payloads with SHA256 (in-memory hash map)

**AMENDMENT (2026-04-07): Security-critical constraint:**

- **Default: in-memory only.** No disk persistence unless explicitly opted in.
- SQLite/file persistence = data on disk = potential data breach (SAP credentials, source code, business object names).
- Disk caching only behind explicit opt-in flag: `--enable-cache` or `SAP_ENABLE_CACHE=true`.
- When opted in, cache file location must be documented and the user must be warned.
- Never cache: passwords, auth tokens, full source code, cookie values.
- Safe to cache: object names, edge relationships, transport numbers, SHA256 hashes.

Architecture:

- `pkg/cache/` already has `Cache` interface with `MemoryCache` and `SQLiteCache` implementations.
- Use `MemoryCache` as default (already built, session-scoped, GC'd on exit).
- `SQLiteCache` only when user explicitly enables persistent mode.

Important:

- this is groundwork, not a user-facing feature
- in-memory cache lives only for the duration of the process
- no risky persistence by default

Deliverable:

- in-memory caching hooks wired into co-change/impact handlers
- opt-in flag for SQLite persistence (documented with security warning)
- small design note documenting what is stored and why

Success condition:

- repeated queries within same session are faster (in-memory hit)
- future `historical impact` only works when user explicitly enables persistent cache

### Track D. `CodeUnitContract` design stub

Do not build the whole family in 2 days.
Do only the smallest foundation step.

Target:

- define a common internal contract/result model that can later cover:
  - class method
  - interface method
  - FM
  - FORM
  - SUBMIT/report parameters

Minimal execution:

- introduce the model
- map current `method-signature` output to it
- do not change CLI UX unless it is trivial

Deliverable:

- new internal model
- one existing consumer migrated
- short report on expansion path

Success condition:

- future contract readers can be added without inventing new result shapes

## If Time Remains

Only after A-D are done:

### Option 1. `api-surface` tightening

- improve classification cleanup
- centralize custom/standard detection
- reduce duplicated namespace logic

### Option 2. `health` polish

- improve package-path latency framing
- tighten `--fast` behavior or messaging

### Option 3. `impact`/config polish memo

- no broad rewrite
- just a concrete design note on how config-aware impact should evolve from `where-used-config`

## Explicit Non-Goals For These 2 Days

- no `transport check` implementation yet
- no AFF object type push yet
- no refactor execution
- no clone/fork tooling
- no full SQLite cache wiring unless the package-acquisition work naturally exposes a tiny safe hook

## Preferred Output By End Of 2 Days

Best case:

1. blocker verification memo
2. shared package acquisition helper merged
3. passive collection hooks merged
4. `CodeUnitContract` internal stub merged

Acceptable case:

1. blocker verification memo
2. shared package acquisition helper merged
3. one passive data collection hook or one `CodeUnitContract` stub merged

## Final Decision Rule

If there is tension between:

- "new visible feature"
- and "shared foundation that removes duplication"

choose the shared foundation.

The product already has enough new surface.
The next win is making the surface cheaper to extend safely.

