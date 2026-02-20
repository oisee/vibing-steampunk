# Version History & Comparison Tools Implementation

**Date:** 2026-02-20
**Report ID:** 001
**Subject:** ADT Object Version History — research, design, and implementation
**Related Documents:** abap-adt-api/revisions.ts (reference implementation)

---

## 1. Problem Statement

VSP had no way to retrieve or compare different versions of ABAP objects. When a transport is released, SAP creates a version snapshot — but our MCP server could only see the current active version. Developers need to:
- See what changed between transport releases
- Compare historical versions against the current code
- Audit who changed what and when

## 2. ADT Versioning API (Research Results)

### Discovery

ADT exposes version history through a standard Atom feed pattern:

1. Every ABAP object's XML metadata contains a link with `rel="http://www.sap.com/adt/relations/versions"`
2. The link's `href` points to a revision feed URL
3. The URL follows a predictable pattern: `{sourceURL}/versions`

### Atom Feed Structure

```
GET /sap/bc/adt/programs/programs/{name}/source/main/versions
Accept: application/atom+xml;type=feed
```

Response:
```xml
<atom:feed>
  <atom:entry>
    <atom:id>5</atom:id>
    <atom:title>Active Version</atom:title>
    <atom:updated>2025-06-15T14:30:00Z</atom:updated>
    <atom:author><atom:name>DEVELOPER1</atom:name></atom:author>
    <atom:content src="/sap/bc/adt/.../source/main?version=5" type="text/plain"/>
    <atom:link href="..." type="application/vnd.sap.adt.transportrequests.v1+xml"
               adtcore:name="K900123"/>
  </atom:entry>
  ...
</atom:feed>
```

### URL Patterns Per Object Type

| Type | Revision Feed URL |
|------|-------------------|
| PROG | `/sap/bc/adt/programs/programs/{name}/source/main/versions` |
| CLAS | `/sap/bc/adt/oo/classes/{name}/includes/main/versions` |
| CLAS (include) | `/sap/bc/adt/oo/classes/{name}/includes/{include}/versions` |
| INTF | `/sap/bc/adt/oo/interfaces/{name}/includes/main/versions` |
| FUNC | `/sap/bc/adt/functions/groups/{parent}/fmodules/{name}/source/main/versions` |
| INCL | `/sap/bc/adt/programs/includes/{name}/source/main/versions` |
| DDLS | `/sap/bc/adt/ddic/ddl/sources/{name}/source/main/versions` |
| BDEF | `/sap/bc/adt/bo/behaviordefinitions/{name}/source/main/versions` |
| SRVD | `/sap/bc/adt/ddic/srvd/sources/{name}/source/main/versions` |

### Reference: abap-adt-api (TypeScript)

Source: `github.com/marcellourbani/abap-adt-api/src/api/revisions.ts`

Key patterns borrowed:
- Extract revision link via `rel="http://www.sap.com/adt/relations/versions"`
- Accept header: `application/atom+xml;type=feed`
- Transport extraction from `adtcore:name` attribute on transport link
- Class includes: revisions are per-include, not per-class

## 3. Implementation

### New Files

| File | LOC | Purpose |
|------|-----|---------|
| `pkg/adt/revisions.go` | ~160 | Core client methods |
| `pkg/adt/revisions_test.go` | ~260 | 8 unit tests |
| `internal/mcp/handlers_revisions.go` | ~90 | MCP handler functions |

### Modified Files

| File | Changes |
|------|---------|
| `pkg/adt/xml.go` | +Revision struct, +RelVersions const, +ParseRevisionFeed(), +Name field in Link |
| `internal/mcp/server.go` | +3 tool registrations, +3 focused mode whitelist entries |
| `pkg/adt/integration_test.go` | +3 integration tests |
| `CLAUDE.md` | Updated tool counts, status, file structure |

### New MCP Tools

| Tool | Params | Returns |
|------|--------|---------|
| **GetRevisions** | type, name, include?, parent? | JSON array of {uri, version, versionTitle, date, author, transport} |
| **GetRevisionSource** | version_uri | Source code text |
| **CompareVersions** | type, name, version1_uri, version2_uri?, include?, parent? | Unified diff JSON |

### Design Decisions

1. **Predictable URL construction** vs fetching object structure first → chose predictable URLs (saves HTTP round-trip, ADT pattern is consistent)
2. **Reused existing infrastructure**: `generateUnifiedDiff()` from workflows.go, `SourceDiff` struct, `GetSourceOptions`, `checkSafety(OpRead, ...)`
3. **"current" sentinel** in CompareVersions — allows comparing any historical version against the live code without knowing the version URI
4. **Added `Name` field to Link struct** — maps `adtcore:name` attribute, needed for transport number extraction, backward-compatible

### Test Results

- 8 unit tests: all passing
- Build: clean (`go build ./...`)
- Vet: clean (`go vet ./...`)
- No regressions in existing test suite

### Integration Test Results (live SAP system SC3 — non-HANA)

| Test | Status | Details |
|------|--------|---------|
| `TestIntegration_GetRevisions` | **PASS** | /COCKPIT/CL_TOOLS: 3 revisions (IVANOV, SAPUSER, BAYAN) |
| `GetRevisionSource` (within above) | **PASS** | 83,077 chars of historical source |
| `TestIntegration_CompareVersions` | **PASS** | diff v00002→v00000: +473 lines; vs current: identical |
| `TestIntegration_GetRevisions_Class` | **PASS** | 3 main revisions; testclasses 404 (class has no tests) |

**Key findings:**
- **Bugfix:** Classes use `/includes/main/versions` (NOT `/source/main/versions`). Initial implementation used wrong URL pattern.
- PROG uses `/source/main/versions`, CLAS/INTF use `/includes/{type}/versions`
- Transport numbers extracted correctly (e.g., UPGRADE751, S75K900082)
- Version source URI format differs: PROG uses `?version=N`, CLAS uses `.../versions/{timestamp}/{id}/content`
- CompareVersions works: unified diff between historical versions and vs current

## 4. Known Limitations

- Revision URL pattern differs between PROG and CLAS/INTF — discovered empirically, now correct
- Objects in `$TMP` without transport history may return empty revision lists
- No pagination support (Atom feed returns all versions at once)
- testclasses include returns 404 if class has no test include (expected behavior)

## 5. Integration Test Design

Tests use fallback strategy — try multiple objects until one works:
1. `TestIntegration_GetRevisions` — tries /COCKPIT/CL_TOOLS, SAPMV45A, ZABAPGIT; also tests GetRevisionSource
2. `TestIntegration_CompareVersions` — tries /COCKPIT/CL_TOOLS, SAPMV45A, ZABAPGIT; needs 2+ revisions
3. `TestIntegration_GetRevisions_Class` — tries /COCKPIT/CL_TOOLS, /COCKPIT/CL_AP_CUSTOMIZING, CL_GUI_ALV_GRID
