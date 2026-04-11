# Slim V2 — Hierarchical Scope + Multi-Level Dead Code Analysis

**Date:** 2026-04-07
**Report ID:** 2026-04-07-003
**Subject:** Design for proper hierarchical slim with mask support and multi-level analysis
**Status:** DRAFT — awaiting peer review before implementation

---

## Current State (Slim V1)

- Scope: single package or LIKE prefix (`DEVCLASS LIKE '$ZLLM%'`)
- Analysis: object-level only (zero incoming refs = dead)
- Problem: LIKE prefix is not true hierarchy (matches `$ZLLM2` alongside `$ZLLM_UTILS`)
- Problem: "dead" means zero refs total, not zero refs from OUTSIDE scope

---

## Proposed: Slim V2

### Phase 1: Hierarchical Scope Resolution

**Input patterns:**

```
vsp slim '$ZLLM'              # exact package + all children (hierarchy walk)
vsp slim '$ZLLM*'             # mask: find matching packages, then expand hierarchy
vsp slim '$ZLLM' --exact-package  # exact package only (no hierarchy)
```

**Data source: TDEVC table**

```sql
-- Find package hierarchy
SELECT DEVCLASS, PARENTCL FROM TDEVC WHERE DEVCLASS LIKE '$ZLLM%'
-- or for exact:
SELECT DEVCLASS, PARENTCL FROM TDEVC WHERE PARENTCL = '$ZLLM'
```

**Algorithm:**

```
1. If input has '*': query TDEVC WHERE DEVCLASS LIKE mask → candidate packages
2. For each candidate: walk PARENTCL tree downward to collect all children
3. Deduplicate → full scope set of package names
4. Query TADIR WHERE DEVCLASS IN (scope_packages) → objects in scope
```

**Why TDEVC, not LIKE prefix:**
- `$ZLLM` has children `$ZLLM_CORE`, `$ZLLM_UI`, `$ZLLM_TEST`
- LIKE `$ZLLM%` also matches `$ZLLM2` (unrelated package)
- TDEVC PARENTCL gives real parent-child relationships

### Phase 2: Object-Level Dead Code Analysis

**AMENDMENT (2026-04-07): Corrected after Codex review.**

**Current V1 logic:** zero incoming refs = dead.

**V2 logic — three verdicts:**

```
For each object in scope:
  1. Get ALL incoming refs (WBCROSSGT + CROSS reverse)
  2. NormalizeInclude each caller → object name
  3. Classify each caller:
     - caller is IN scope (same package set) → internal ref
     - caller is OUTSIDE scope → external ref
  4. Assign verdict:
     - external_refs > 0           → LIVE (directly used from outside)
     - external_refs == 0 AND
       total_refs == 0             → DEAD (zero refs anywhere, HIGH confidence)
     - external_refs == 0 AND
       total_refs > 0              → INTERNAL_ONLY (warning, not verdict)
```

**Verdicts:**
- `DEAD` — zero refs total (HIGH confidence). Definitely unused.
- `INTERNAL_ONLY` — has refs, but all from within scope (WARNING, not assertion of dead).
  This is **informational**, not a deletion recommendation. The object may be:
  - A legitimate internal helper called from a live public entrypoint → LIVE in reality
  - Part of a dead cluster where nothing in the cluster is called externally → truly dead
  - **We cannot distinguish these cases without graph reachability analysis.**
- `LIVE` — has at least one external ref. Definitely in use.

**Key correction (from Codex review):** "no external refs" ≠ "not needed." A helper class
called only by a live public class IS needed. INTERNAL_ONLY is a signal for human review,
not an automated verdict. Asserting UNREACHABLE at per-object level would be semantically wrong.

**Full graph reachability (V2.1, deferred):**
To truly identify unreachable clusters, need:
1. Find entry points: objects with at least one external ref
2. Forward-traverse internal edges from entry points → mark reachable
3. Objects not reached → truly unreachable as a cluster
This is a graph-level property, not per-object. Deferred to V2.1.

### Phase 3: Member-Level Dead Code (within REACHABLE classes)

**AMENDMENT:** Analyze classes that are LIVE or INTERNAL_ONLY (not just externally-referenced).
A helper class with zero external refs but called from a live entrypoint still has meaningful
methods. The criterion is "class exists and has code," not "class has external refs."

For DEAD classes (zero refs total) → skip method analysis (whole object is dead).

**Methods:**

```
For each live class:
  1. GetObjectStructure → method list with visibility
  2. For each method:
     a. Is it from an interface? (check interface implementations) → skip (may be called polymorphically)
     b. Is it constructor/class_constructor? → skip (framework-called)
     c. Query WBCROSSGT for external refs to this method specifically
        (WBCROSSGT OTYPE='ME' and NAME contains method name)
     d. If zero external refs → dead method candidate (MEDIUM confidence)
```

**Attributes:**

```
For each live class:
  1. GetObjectStructure → attribute list (CLAS/OA elements)
  2. For each PUBLIC/PROTECTED attribute:
     a. Query WBCROSSGT for external refs (OTYPE='DA' or 'OA')
     b. If zero external refs → dead attribute candidate (MEDIUM confidence)
  3. PRIVATE attributes: skip (internal usage detection requires source parsing, deferred)
```

**Confidence levels:**
- Method: MEDIUM — WBCROSSGT may not track all method-level refs perfectly
- Attribute: MEDIUM — same reason
- Both: cannot detect dynamic access (`CALL METHOD (variable)` or `ASSIGN COMPONENT`)

---

## CLI UX

```bash
# Default: hierarchical, all levels
vsp slim '$ZLLM'

# Mask: find packages matching pattern, expand hierarchy
vsp slim '$ZLLM*'

# Exact package only
vsp slim '$ZLLM' --exact-package

# Control analysis depth
vsp slim '$ZLLM' --level objects         # object-level only (fastest)
vsp slim '$ZLLM' --level methods         # objects + methods
vsp slim '$ZLLM' --level full            # objects + methods + attributes (default)

# Output
vsp slim '$ZLLM' --format json
vsp slim '$ZLLM' --format text           # default
```

## Output Shape

```
Slim Report: $ZLLM (hierarchical, 5 packages, 274 objects)

Packages in scope:
  $ZLLM, $ZLLM_CORE, $ZLLM_UI, $ZLLM_TEST, $ZLLM_UTILS

=== DEAD OBJECTS (12) — zero references anywhere ===
  ❌ CLAS ZCL_OLD_HELPER [$ZLLM_UTILS] — 0 refs, last transport 2024-01-15
  ❌ PROG ZTEST_ABANDONED [$ZLLM_TEST] — 0 refs, last transport 2023-06-01
  ...

=== INTERNAL_ONLY OBJECTS (5) — referenced only within scope (review needed) ===
  ℹ️ CLAS ZCL_INTERNAL_CACHE [$ZLLM_CORE] — 3 refs, all internal (may be legitimate helper)
  ℹ️ CLAS ZCL_INTERNAL_LOGGER [$ZLLM_CORE] — 2 refs, all internal (may be legitimate helper)
  ...

=== DEAD METHODS (8) — in live classes, no external callers ===
  ⚠️ ZCL_TRAVEL=>OLD_CALC — 0 external callers [MEDIUM]
  ⚠️ ZCL_ORDER=>LEGACY_CHECK — 0 external callers [MEDIUM]
  ...

=== DEAD ATTRIBUTES (3) — public/protected, no external refs ===
  ⚠️ ZCL_TRAVEL=>MV_OBSOLETE_FLAG — 0 external refs [MEDIUM]
  ...

Summary:
  274 objects: 257 live, 12 dead, 5 unreachable
  8 dead methods in live classes
  3 dead attributes in live classes
```

---

## Data Sources

| Phase | Source | Query |
|-------|--------|-------|
| Scope | TDEVC | `SELECT DEVCLASS, PARENTCL FROM TDEVC WHERE ...` |
| Objects | TADIR | `SELECT OBJECT, OBJ_NAME, DEVCLASS FROM TADIR WHERE DEVCLASS IN (...)` |
| Object refs | WBCROSSGT | `SELECT INCLUDE, NAME FROM WBCROSSGT WHERE NAME LIKE 'obj%'` (reverse) |
| Object refs | CROSS | `SELECT INCLUDE, NAME FROM CROSS WHERE NAME LIKE 'obj%'` (reverse) |
| Methods | ADT | `GetClassObjectStructure` → elements with CLAS/OM type |
| Method refs | WBCROSSGT | `WHERE INCLUDE LIKE 'classname%' AND OTYPE = 'ME' AND NAME = 'method'` (owner-scoped to avoid name collision) |
| Attributes | ADT | `GetClassObjectStructure` → elements with CLAS/OA type |
| Attribute refs | WBCROSSGT | `WHERE INCLUDE LIKE 'classname%' AND OTYPE IN ('DA','OA') AND NAME = 'attr'` (owner-scoped) |

**AMENDMENT:** Method/attribute queries MUST be scoped by owning class (via INCLUDE pattern)
to avoid false matches from identically-named methods in different classes (e.g., GET_DATA
exists in 100+ classes). Query `NAME = 'GET_DATA'` without class scope is useless.

---

## Risks and Limitations

1. **TDEVC may not be queryable on all systems** — freestyle SQL may be blocked for system tables. Fallback: LIKE prefix (current behavior).

2. **Unreachable analysis can be slow for large scopes** — 274 objects × 2 SQL queries each = 548 queries. Mitigate: batch queries (already doing this in V1).

3. **Method-level analysis requires ADT object structure calls** — one per live class. For 100 live classes = 100 ADT round-trips. Mitigate: only analyze classes with `--level methods|full`, skip with `--level objects`.

4. **WBCROSSGT method-level tracking is imprecise** — OTYPE='ME' rows may not exist for all method call patterns. False negatives possible.

5. **Dynamic calls invisible** — `CALL METHOD (variable)`, `ASSIGN COMPONENT`, BAdI calls via framework. Flagged as risk, not detectable.

6. **Scope boundary edge case** — if an object in scope is called by another object also in scope, but that caller is itself unreachable, both should be unreachable. This requires iterative/recursive analysis (mark unreachable → recheck callers → mark more unreachable). V2 should do one pass first, iterative refinement in V2.1.

---

## Implementation Phases

| Phase | What | Effort | Dependencies |
|-------|------|--------|-------------|
| 1 | TDEVC hierarchy resolution | 3h | New helper in pkg/acquire or pkg/graph |
| 2 | Unreachable-from-outside analysis | 4h | Requires scope set from Phase 1 |
| 3 | Method-level dead code | 4h | Requires GetClassObjectStructure |
| 4 | Attribute-level dead code | 2h | Same ADT call as Phase 3 |
| 5 | CLI UX (--level, mask support) | 2h | Phases 1-4 |
| **Total** | | **~15h** | |

---

## Open Questions — Status After Codex Review

1. ~~Should UNREACHABLE be separate from DEAD?~~ **RESOLVED:** Renamed to INTERNAL_ONLY (warning, not verdict). Separate section in output with ℹ️ not ❌.

2. ~~Should iterative unreachable analysis be in V2 or V2.1?~~ **RESOLVED:** Deferred to V2.1. Codex correctly identified that the cascade example was logically broken — it's impact simulation, not current-state analysis.

3. **Is TDEVC always queryable via freestyle SQL?** OPEN. Fallback to LIKE prefix if TDEVC query fails.

4. **For method-level: should we analyze ALL non-dead classes or only large ones?** OPEN. Codex noted Phase 3 should use "reachable" not "externally-referenced" as criterion. For V2: analyze all non-DEAD classes (both LIVE and INTERNAL_ONLY).

5. **Attribute analysis: V2 or V2.1?** OPEN. WBCROSSGT attribute tracking unreliable. Leaning V2.1.

6. **(NEW from Codex)** Method/attribute name collisions must be resolved by scoping queries to owning class. `NAME = 'GET_DATA'` alone is useless. Query must include `INCLUDE LIKE 'classname%'`.
