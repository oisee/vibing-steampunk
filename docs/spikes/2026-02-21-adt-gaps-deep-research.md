# ADT API Gaps — Deep Research Spike

**Date:** 2026-02-21
**Subject:** Detailed technical research of all identified ADT REST API gaps
**Related:** [docs/ADT-GAP-ANALYSIS.md](../ADT-GAP-ANALYSIS.md)

---

## Key Discoveries

### Corrections to Gap Analysis

1. **ATC Worklist is ALREADY IMPLEMENTED.** `RunATCCheck()` in `pkg/adt/devtools.go` already calls `GetATCWorklist()` internally. The worklist XML parsing with priorities, locations, check IDs is complete. What's actually missing is **ATC QuickFix details** (fetching/applying fixes for individual findings).

2. **BDEF, SRVD, SRVB write is ALREADY IMPLEMENTED** via `WriteSource()` composite workflow. The gap analysis incorrectly listed "Behavior Definition direct write" — it works through the standard lock→update→unlock→activate pattern.

3. **CodeCompletion is ALREADY IMPLEMENTED** in `pkg/adt/codeintel.go`.

### Revised Gap Count

- Original gap analysis: ~30 missing endpoints
- After correction: ~23 genuine gaps (7 were false positives)

---

## Research Results by Category

---

## 1. Refactoring (HIGH PRIORITY)

### 1.1 Rename Refactoring

**Three-step flow:** evaluate → preview → execute

```
POST /sap/bc/adt/refactoring/rename?method=evaluate&uri={object-uri}&position={line},{col}&newName={name}
POST /sap/bc/adt/refactoring/rename?method=preview&uri={object-uri}&newName={name}
POST /sap/bc/adt/refactoring/rename?method=execute&uri={object-uri}&newName={name}
```

**Step 1 — Evaluate:** Validates rename feasibility. Returns problems list (severity, location, description) and change count.

**Step 2 — Preview:** Returns all affected locations as `<refactoring:edits>` with per-change `<refactoring:change line="" column="" length="">` elements containing `oldText` / `newText`.

**Step 3 — Execute:** Applies changes. Request body includes the edits XML from preview. Returns status + list of affected objects.

**Key details:**
- Position format: `#start={line},{col}` fragment on URI (1-based)
- Content-Type: `text/plain` (source in body) for evaluate/preview; `application/xml` for execute
- Namespace: `xmlns:refactoring="http://www.sap.com/adt/refactoring"`
- The flow is **stateless** — evaluate/preview don't modify anything
- Errors: 400 (bad params), 409 (name conflict)

**Effort:** 5 | **Value:** HIGH | **Files:** new `pkg/adt/refactoring.go`, new `internal/mcp/handlers_refactoring.go`

### 1.2 Extract Method

Same three-step flow as Rename:

```
POST /sap/bc/adt/refactoring/extractmethod?method=evaluate&uri={uri}&newMethodName={name}&range={startLine},{startCol};{endLine},{endCol}
POST /sap/bc/adt/refactoring/extractmethod?method=preview&uri={uri}&newMethodName={name}&range={range}
POST /sap/bc/adt/refactoring/extractmethod?method=execute&uri={uri}&newMethodName={name}
```

**Additional parameter:** `range=10,5;15,8` — selection range (lines 10-15, cols 5-8)

**Evaluate returns:** inferred parameters, return type, conflict warnings.
**Preview returns:** new method signature, modified source, call site.

**Effort:** 6 | **Value:** HIGH

### 1.3 Quick Fix Proposals

```
POST /sap/bc/adt/quickfix/proposals?uri={object-uri}&line={line}&column={col}
  Body: source code (text/plain)
  Response: <quickfix:proposals> with proposal id, title, description, change preview

POST /sap/bc/adt/quickfix/apply?uri={object-uri}&proposalId={id}
  Body: source code (text/plain)
  Response: <quickfix:result> with status + newSource
```

**Namespace:** `xmlns:quickfix="http://www.sap.com/adt/quickfix"`

**Also available via ATC findings:** ATC worklist findings include `quickfixInfo` attribute and `<atcfinding:link>` to `/sap/bc/adt/atc/quickfix/{findingId}`.

**Effort:** 4 (proposals) + 5 (apply) | **Value:** HIGH (AI auto-fix pipeline)

---

## 2. Testing & Quality

### 2.1 Code Coverage (extension to RunUnitTests)

Add `requestCoverage=true&coverageFormat=full` to existing testruns endpoint.

Response includes additional `<abapunit:coverageData>` section:
```xml
<abapunit:sourceCoverage uri="...">
  <abapunit:coveredLine line="10" statement="true" decision="true"/>
  <abapunit:uncoveredLine line="12" statement="true" decision="true"/>
</abapunit:sourceCoverage>
```

**Effort:** 4 | **Value:** HIGH | **Files:** modify `pkg/adt/devtools.go`, extend UnitTestResult struct

### 2.2 SQL Explain Plan

```
POST /sap/bc/adt/datapreview/sqlexplainplan?rowNumber=100
  Content-Type: text/plain
  Body: SQL query
  Response: execution plan nodes (operator, table, cost, cardinality, index)
```

**HANA only.** Non-HANA returns simplified plan or error.

**Effort:** 5 | **Value:** MEDIUM (optimization tool)

### 2.3 ATC QuickFix (the REAL gap)

ATC findings include `quickfixInfo` and link to:
```
GET /sap/bc/adt/atc/quickfix/{findingId}
POST /sap/bc/adt/atc/quickfix/{findingId}/apply
```

**Effort:** 3 | **Value:** HIGH

---

## 3. CDS/RAP Ecosystem

### 3.1 Metadata Extension (MDE)

```
GET/PUT /sap/bc/adt/ddic/cds/metadataextensions/{name}/source/main
POST    /sap/bc/adt/ddic/cds/metadataextensions  (create)
```

- Source format: DDL annotation syntax (text/plain)
- XML root for create: `mde:metadataExtension` + `xmlns:mde="http://www.sap.com/adt/ddic/cds/metadataextensions"`
- Required: standard `adtcore:name`, `adtcore:description`, `adtcore:type`, `adtcore:responsible` attributes
- Follow same CRUD pattern as DDLS

**Effort:** 3 | **Value:** MEDIUM (UI5 annotation development)

### 3.2 Access Control (DCL)

```
GET/PUT /sap/bc/adt/ddic/cds/accesscontrols/{name}/source/main
POST    /sap/bc/adt/ddic/cds/accesscontrols  (create)
```

- Source format: DCL syntax (text/plain)
- Object type: DCLS
- XML root: `dcl:accessControl` + `xmlns:dcl="http://www.sap.com/adt/ddic/cds/accesscontrols"`

**Effort:** 3 | **Value:** MEDIUM (row-level security for RAP)

### 3.3 CDS Element Info

```
GET /sap/bc/adt/ddic/cds/elementinfo?cdsViewName={name}&elementPath={path}
Accept: application/xml
```

Returns field-level metadata: type, annotations, semantic labels, cardinality.

**Effort:** 3 | **Value:** MEDIUM

### 3.4 CDS Impact Analysis

```
POST /sap/bc/adt/ddic/cds/impactanalysis
Content-Type: application/xml
Body: <impactRequest><cdsViewName>...</cdsViewName><analysisType>BACKWARD</analysisType></impactRequest>
```

Returns affected downstream objects with severity and reason. Different from `GetCDSDependencies()` which does FORWARD deps only.

**Effort:** 4 | **Value:** MEDIUM (refactoring safety)

### 3.5 CDS Annotation Value Help

```
GET /sap/bc/adt/ddic/cds/annotationvaluehelp?annotation={path}&context={entity}
```

Returns valid annotation values (literals with descriptions).

**Effort:** 4 | **Value:** LOW

---

## 4. Transport Operations

### 4.1 Add Object to Transport

```
POST /sap/bc/adt/cts/transports/{transport_number}?_action=checkintotr
Content-Type: application/vnd.sap.as+xml
Body:
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <PGMID>R3TR</PGMID>
      <OBJECT>PROG</OBJECT>
      <OBJECTNAME>ZTEST_PROG</OBJECTNAME>
      <OPERATION>I</OPERATION>
    </DATA>
  </asx:values>
</asx:abap>
```

**Alternative:** `/sap/bc/adt/cts/transportrequests/{id}/objects` (POST)

**Note:** Exact `_action` value needs validation on real system.

**Effort:** 3 | **Value:** MEDIUM

---

## 5. Activity Feeds

All feeds use Atom format (`Accept: application/atom+xml;type=feed`), same parser as `revisions.go`.

### 5.1 Object Change Feed
```
GET /sap/bc/adt/feed/objects?filter={pattern}&since={timestamp}&max_results={n}
```

### 5.2 User Activity Feed
```
GET /sap/bc/adt/feed/users/{username}?since={timestamp}&max_results={n}
```

### 5.3 Transport Feed
```
GET /sap/bc/adt/feed/transports?user={user}&status={status}&since={timestamp}
```

**Note:** These endpoints are **unconfirmed** on real systems. May not exist in all SAP versions.

**Effort:** 3 each | **Value:** LOW-MEDIUM

---

## 6. DDIC Object Types

All follow the standard read pattern (`GET` with `Accept: application/xml`):

| Type | Endpoint | Object Code | Effort |
|------|----------|-------------|--------|
| DDIC View | `/sap/bc/adt/ddic/views/{name}` | VIEW | 2 |
| Search Help | `/sap/bc/adt/ddic/searchhelps/{name}` | SHLP | 2 |
| Lock Object | `/sap/bc/adt/ddic/lockobjects/{name}` | ENQU | 2 |
| Type Group | `/sap/bc/adt/ddic/typegroups/{name}` | TYPE | 2 |

Response includes field definitions, joins (views), parameters (search helps), lock fields (enqueue).

---

## 7. Features Deprioritized

### 7.1 abapGit REST Endpoints — NOT RECOMMENDED
The WebSocket approach via ZADT_VSP is more reliable than the Eclipse plugin REST endpoints, which are unstable across versions. Current GitExport/GitTypes implementation is sufficient.

### 7.2 Debug Variable Modification — NOT FEASIBLE
SAP ADT does not reliably expose variable modification during debugging via REST or WebSocket. Low probability, high complexity.

### 7.3 SQL Trace Details — ALREADY COVERED
`ListSQLTraces()` + `GetTrace()` provide >90% of the value. Individual trace details (`/sap/bc/adt/sqltrace/traces/{id}`) would add minimal value.

---

## Validation Requirements

All endpoints marked "unconfirmed" should be validated via:
1. ADT discovery: `GET /sap/bc/adt/discovery` on target system
2. Eclipse network trace (Fiddler proxy)
3. ABAP debugging on handler classes (SADT_REST package)

Specifically unconfirmed:
- Activity feed endpoints (`/sap/bc/adt/feed/*`)
- Transport `_action=checkintotr` exact syntax
- CDS Impact Analysis exact request/response format
- CDS Element Info query parameters
