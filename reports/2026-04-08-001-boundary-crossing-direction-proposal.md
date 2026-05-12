# ADR: Directional Package Boundary Crossing Analysis

**Date:** 2026-04-08
**Status:** Proposal
**Context:** health --package boundaries signal, graph engine

---

## Problem

The current boundary check is binary: "crossed" or "not crossed." This is almost useless because *some* crossings are correct architecture and others are bugs. Without direction awareness, the signal is noisy and teams learn to ignore it.

## Proposed Model

Classify every cross-package reference by the **direction** of the dependency relative to the package hierarchy.

### Hierarchy Assumptions

SAP package hierarchies typically follow this shape:

```
$ZLLM                  ← root (orchestration, API surface)
├── $ZLLM_00           ← common/shared (types, interfaces, base classes)
├── $ZLLM_01           ← domain module A
├── $ZLLM_02           ← domain module B
│   └── $ZLLM_02_SUB   ← sub-module of B
├── $ZLLM_04           ← tests
└── $ZLLM_99           ← utilities/tools
```

### Crossing Categories

| Direction | Example | Verdict | Rationale |
|-----------|---------|---------|-----------|
| **UPWARD** | ZLLM_02 → ZLLM | OK | Child depends on parent. Natural. Dependency flows toward the root. |
| **UPWARD_SKIP** | ZLLM_02_SUB → ZLLM | WARN | Skipping a level. Might indicate missing abstraction in ZLLM_02. Not wrong, but worth flagging. |
| **COMMON** | ZLLM_02 → ZLLM_00 | OK | Referencing the shared/common package. This is the whole point of having _00. |
| **SIBLING** | ZLLM_02 → ZLLM_01 | BAD | Coupling between siblings. Usually means shared code should be extracted to _00 or a new common package. |
| **DOWNWARD** | ZLLM → ZLLM_01 | BAD | Parent depends on child. Inverts the hierarchy. Makes the parent impossible to understand without reading the child. |
| **COMMON_DOWN** | ZLLM_00 → ZLLM_01 | BAD | Common/shared depends on a specific module. Defeats the purpose of common. Circular risk. |
| **EXTERNAL** | ZLLM_02 → ZOTHER_01 | INFO | Cross-hierarchy reference. Not necessarily wrong but important to track. Outside the package's control. |
| **STANDARD** | ZLLM_02 → CL_ABAP_TYPEDESCR | IGNORE | Standard SAP. Not a boundary issue. |

### How "Common" is Identified

A package is considered "common" if it matches any of these patterns (configurable):

1. Suffix `_00` (SAP convention for base/shared)
2. Suffix `_COMMON`, `_BASE`, `_CORE`, `_SHARED`
3. Explicitly marked in config

Default: `_00` only. Keep it simple until someone needs more.

### How Direction is Determined

Given source package S and target package T within the same hierarchy:

```
1. Is T an ancestor of S?           → UPWARD (check skip distance)
2. Is T a "common" sibling?         → COMMON
3. Is T a sibling of S?             → SIBLING
4. Is T a descendant of S?          → DOWNWARD
5. Is S "common" and T a sibling?   → COMMON_DOWN
6. Is T in a different root?        → EXTERNAL
7. Is T a standard SAP object?      → STANDARD (ignore)
```

Ancestor/descendant is determined by package name prefix matching (same as AcquirePackageScope fallback), validated against TDEVC when available.

---

## Critique of This Model

### What's Good

- **Direction is the right axis.** Binary crossed/not-crossed produces noise. Direction produces actionable categories.
- **_00 as common is practical.** Matches real SAP development patterns. Not invented, observed.
- **SIBLING is the most actionable finding.** "ZLLM_02 depends on ZLLM_01" directly tells you what to extract to _00.
- **DOWNWARD is the most dangerous.** Parent→child inversion is a real architectural bug that makes packages unmaintainable.

### What Needs Care

- **Prefix matching is fragile.** `$ZLLM_01` and `$ZLLM_010` — is _010 a child of _01 or a sibling? TDEVC PARENTCL is authoritative but not always populated (as we learned with test packages). Heuristic: if TDEVC says parent, trust it. Otherwise, longest prefix match with `_` as separator.
- **"Common" is a convention, not a rule.** Some teams use _00 for something else. Making it configurable is right, but the default matters. _00 is the safest default based on SAP ecosystem norms.
- **Test packages blur the lines.** ZLLM_04 (tests) crossing into ZLLM_01 is *expected* — tests should test things. Test packages should be excluded from SIBLING violations or given their own category (TEST_CROSS = OK).
- **UPWARD_SKIP might be too noisy.** ZLLM_02_SUB using something from ZLLM root is often fine. Consider making this INFO not WARN unless the skip is > 2 levels.
- **Circular sibling deps are the worst case.** ZLLM_01 → ZLLM_02 AND ZLLM_02 → ZLLM_01. This should be detected and called out specifically as CIRCULAR, not just two SIBLING findings.

### What This Model Cannot Do

- **It doesn't know about interfaces.** If ZLLM_00 defines an interface and ZLLM_01 implements it, ZLLM_02 → ZLLM_01 via the interface is architecturally fine. But the parser sees a concrete class reference. Fixing this requires type resolution, which is expensive.
- **It doesn't distinguish compile-time from runtime deps.** A `CREATE OBJECT TYPE (lv_variable)` dynamic call is detected but the target package is unknown. These should be flagged but not categorized.
- **It doesn't cover data model coupling.** ZLLM_02 using a table type defined in ZLLM_01 is a SIBLING crossing at the DDIC level, but we only parse ABAP source, not DDIC metadata graphs.

---

## Implementation Plan

### Phase 1: Fix the Expander (Now)

- Extract common package hierarchy expansion used by health tests, boundaries, ATC, changelog, changes into a shared pattern
- Add `--exact` flag (or `--no-subpackages`) to health to prevent expansion
- Fix `collectPackageBoundariesCLI` to use `AcquirePackageScope`

### Phase 2: Directional Classification (Next)

- Add `ClassifyCrossing(sourcePackage, targetPackage, scope PackageScope) CrossingDirection` to `pkg/graph/`
- Scope carries the hierarchy tree + common package patterns
- Health boundary signal reports counts per direction instead of flat "violations"
- Text/MD/HTML reports show crossings grouped by direction

### Phase 3: Report Actionability (Later)

- Each SIBLING crossing suggests: "consider moving X to common package _00"
- Each DOWNWARD crossing suggests: "parent should not depend on child — invert or extract interface"
- CIRCULAR detection across the full sibling set
- Integration with `vsp slim` — dead code + boundary violations in one view

### Config Surface

```json
{
  "boundaries": {
    "common_patterns": ["_00"],
    "test_patterns": ["_TEST", "_04"],
    "ignore_external": false,
    "upward_skip_threshold": 2
  }
}
```

Default: zero config needed. `_00` is common. Everything else is derived from the hierarchy.

---

## Output Format

### Text (--details)

```
Boundaries: $ZLLM (11 packages, 47 objects scanned)

  OK       12  upward crossings
  OK        8  common crossings (_00)
  SIBLING   3  sibling crossings
    ZLLM_02 → ZLLM_01: ZCL_LLM_00_CG references ZCL_LLM_00_CACHE (extract to _00?)
    ZLLM_02 → ZLLM_01: ZCL_LLM_00_EDGE references ZCL_LLM_00_ARRAY
    ZLLM_01 → ZLLM_02: ZCL_LLM_00_CACHE references ZCL_LLM_00_CG  ← CIRCULAR with line 1
  DOWNWARD  1  parent→child crossing
    ZLLM → ZLLM_01: ZCL_LLM references ZCL_LLM_00_CACHE (invert dependency?)
  EXTERNAL  2  cross-hierarchy references
    ZLLM_02 → $ZRAY: ZCL_LLM_00_CODE_UNIT references ZCL_RAY_PARSER
```

### Summary in Health Signal

```json
{
  "status": "WARN",
  "details": {
    "packages_scanned": 11,
    "objects_scanned": 47,
    "upward": 12,
    "common": 8,
    "sibling": 3,
    "downward": 1,
    "external": 2,
    "circular": 1
  }
}
```

---

## Decision

Pending review. Core question: is the direction model worth the complexity, or should we ship a simpler "sibling + downward = bad, everything else = ok" binary first?

Recommendation: **ship the binary version first** (Phase 1 + simplified Phase 2), then refine categories based on real output from actual packages.
