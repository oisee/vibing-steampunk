# ADT Refactoring & QuickFix API Reference

**Date:** 2026-04-05
**Report ID:** 001
**Subject:** Correct ADT API patterns for refactoring and quickfix tools
**Source:** abap-adt-api by marcellourbani (GitHub)

---

## Rename Refactoring

**Endpoint:** `POST /sap/bc/adt/refactorings`

### Step 1: Evaluate

```
?step=evaluate&rel=http://www.sap.com/adt/relations/refactoring/rename&uri={uri}#start={line},{startCol};end={line},{endCol}
```

Headers: `Content-Type: application/*`, `Accept: application/*`
Body: Empty

### Step 2: Preview

```
?step=preview&rel=http://www.sap.com/adt/relations/refactoring/rename
```

Body: XML with `rename:renameRefactoring` wrapper containing `generic:genericRefactoring`.

### Step 3: Execute

```
?step=execute
```

Body: Unwrapped genericRefactoring XML from preview response.

### XML Namespaces

```xml
xmlns:generic="http://www.sap.com/adt/refactoring/genericrefactoring"
xmlns:rename="http://www.sap.com/adt/refactoring/renamerefactoring"
xmlns:adtcore="http://www.sap.com/adt/core"
```

---

## Extract Method

**Endpoint:** `POST /sap/bc/adt/refactorings`

Same 3-step flow with `?rel=http://www.sap.com/adt/relations/refactoring/extractmethod`.

---

## QuickFix

### Evaluate Proposals

```
POST /sap/bc/adt/quickfixes/evaluation?uri={uri}#start={line},{column}
```

Body: source code. Response: XML array of FixProposal objects.

### Apply Fix

```
POST {dynamic_uri_from_proposal}
```

URI must match `/sap/bc/adt/quickfixes/` pattern. Body: XML proposalRequest with source + objectReference.

---

## Key Differences from PR #82 (WRONG → CORRECT)

| Aspect | PR #82 (Wrong) | Correct |
|--------|---------------|---------|
| URL | `/refactoring/rename` | `/refactorings?rel=...rename` |
| Routing | `?method=evaluate` | `?step=evaluate` + `?rel=` |
| Body | Plain text | XML with genericRefactoring |
| QuickFix URL | `/quickfix/proposals` | `/quickfixes/evaluation` |
| Flow | Single call | 3-step: evaluate → preview → execute |
