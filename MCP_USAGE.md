# MCP Usage Guide for AI Agents

**Target Audience:** AI assistants (Claude, GPT, etc.) using this MCP server for ABAP development.

**Purpose:** Machine-friendly reference for optimal tool usage patterns, workflows, and best practices.

---

## Critical Limitations (Read First!)

### SQL Query Limitations (RunQuery / GetTableContents)

The SAP ADT Data Preview API uses **ABAP SQL syntax**, NOT standard SQL:

| Feature | Status | Syntax |
|---------|--------|--------|
| `ORDER BY col` | **Works** | `ORDER BY field_name` |
| `ORDER BY col ASCENDING` | **Works** | ABAP keyword |
| `ORDER BY col DESCENDING` | **Works** | ABAP keyword |
| `ORDER BY col ASC` | **FAILS** | SQL standard - not supported |
| `ORDER BY col DESC` | **FAILS** | SQL standard - not supported |
| `LIMIT n` | **FAILS** | Use `max_rows` parameter instead |
| `GROUP BY` | **Works** | `GROUP BY field_name` |
| `COUNT(*)` | **Works** | Aggregate functions work |
| `WHERE` | **Works** | Standard conditions |

**Correct Example:**
```sql
SELECT carrid, COUNT(*) as cnt FROM sflight GROUP BY carrid ORDER BY cnt DESCENDING
```

**Wrong Example (will fail):**
```sql
SELECT carrid, COUNT(*) as cnt FROM sflight GROUP BY carrid ORDER BY cnt DESC
```

### Object Type Coverage

| Object Type | GetSource | WriteSource | Notes |
|-------------|:---------:|:-----------:|-------|
| PROG (Program) | **Y** | **Y** | Full support |
| CLAS (Class) | **Y** | **Y** | Includes: definitions, implementations, testclasses |
| INTF (Interface) | **Y** | **Y** | Full support |
| FUNC (Function Module) | **Y** | N | Requires `parent` (function group) |
| FUGR (Function Group) | **Y** | N | Returns JSON metadata |
| INCL (Include) | **Y** | N | Read-only |
| DDLS (CDS DDL Source) | **Y** | N | CDS view definitions |
| MSAG (Message Class) | **Y** | N | Returns JSON with all messages |

### CDS Dependencies

`GetCDSDependencies` returns the **base table/view dependencies** of a CDS view:
- Tables the view reads from
- Other CDS views used in FROM clause
- Does NOT return reverse dependencies (where-used)

---

## Server Modes

```mermaid
flowchart TD
    subgraph Focused["Focused Mode (Default) - 19 Tools"]
        U[Unified Tools]
        U --> GS[GetSource]
        U --> WS[WriteSource]

        S[Search]
        S --> GO[GrepObjects]
        S --> GP[GrepPackages]
        S --> SO[SearchObject]

        E[Edit]
        E --> ES[EditSource]

        D[Data]
        D --> GT[GetTable/Contents]
        D --> RQ[RunQuery]
        D --> CD[GetCDSDependencies]

        N[Navigate]
        N --> FD[FindDefinition]
        N --> FR[FindReferences]

        T[Test/Check]
        T --> SC[SyntaxCheck]
        T --> UT[RunUnitTests]
    end

    subgraph Expert["Expert Mode - 45 Tools"]
        direction TB
        F[All Focused Tools]
        A[+ Atomic Operations]
        A --> LO[LockObject]
        A --> UO[UnlockObject]
        A --> CO[CreateObject]
        A --> US[UpdateSource]
        A --> DO[DeleteObject]

        G[+ Granular Reads]
        G --> GP2[GetProgram]
        G --> GC[GetClass]
        G --> GI[GetInterface]

        W[+ Legacy Workflows]
        W --> WP[WriteProgram]
        W --> WC[WriteClass]
        W --> CC[CreateClassWithTests]
    end

    Focused -.->|--mode=expert| Expert
```

### Mode Selection Guide

| Use Case | Mode | Reason |
|----------|------|--------|
| Standard development | Focused | Simpler, fewer choices |
| Existing workflow scripts | Expert | Backward compatibility |
| Debugging lock issues | Expert | Direct LockObject access |
| Learning the API | Expert | See all atomic operations |

---

## Tool Selection Decision Tree

```mermaid
flowchart TD
    START[Need to work with ABAP?]

    START --> READ{Read or Write?}

    READ -->|Read| RTYPE{What type?}
    RTYPE -->|Source code| GS[GetSource type,name]
    RTYPE -->|Table data| TD{Need SQL?}
    TD -->|Simple| GTC[GetTableContents]
    TD -->|Complex| RQ[RunQuery]
    RTYPE -->|Find pattern| GREP{Scope?}
    GREP -->|Single object| GO[GrepObjects]
    GREP -->|Package/namespace| GP[GrepPackages]
    RTYPE -->|CDS deps| CDS[GetCDSDependencies]
    RTYPE -->|Symbol location| FD[FindDefinition]
    RTYPE -->|All usages| FR[FindReferences]

    READ -->|Write| WSIZE{Change size?}
    WSIZE -->|Small <50 lines| ES[EditSource]
    WSIZE -->|Large rewrite| WS[WriteSource]
    WSIZE -->|Huge >2000 lines| IF[ImportFromFile]

    ES -->|Know location?| ESLOC{Location known?}
    ESLOC -->|No| GO2[GrepObjects first] --> ES
    ESLOC -->|Yes| ES2[EditSource directly]
```

---

## Quick Reference

### Reading Objects

```mermaid
flowchart LR
    subgraph "GetSource(type, name)"
        PROG[PROG] --> SRC1[ABAP Source]
        CLAS[CLAS] --> SRC2[Class Source]
        INTF[INTF] --> SRC3[Interface Source]
        FUNC[FUNC + parent] --> SRC4[FM Source]
        FUGR[FUGR] --> JSON1[JSON Metadata]
        DDLS[DDLS] --> SRC5[CDS Source]
        MSAG[MSAG] --> JSON2[JSON Messages]
    end
```

| Task | Tool | Parameters | Returns |
|------|------|------------|---------|
| Read program | `GetSource` | `type=PROG, name=ZTEST` | ABAP source |
| Read class | `GetSource` | `type=CLAS, name=ZCL_TEST` | Class source |
| Read class definitions | `GetSource` | `type=CLAS, name=ZCL_TEST, include=definitions` | Definitions include |
| Read class tests | `GetSource` | `type=CLAS, name=ZCL_TEST, include=testclasses` | Test classes |
| Read interface | `GetSource` | `type=INTF, name=ZIF_TEST` | Interface source |
| Read function module | `GetSource` | `type=FUNC, name=Z_FM, parent=ZFUGR` | FM source |
| Read function group structure | `GetSource` | `type=FUGR, name=ZFUGR` | JSON (FM list) |
| Read CDS view | `GetSource` | `type=DDLS, name=ZDDL_VIEW` | CDS source |
| Read message class | `GetSource` | `type=MSAG, name=ZMSAG` | JSON (all messages) |

### Writing Objects

| Task | Tool | Notes |
|------|------|-------|
| Small edit (<50 lines) | `EditSource` | Surgical replacement, syntax checked |
| Full rewrite | `WriteSource` | Auto-detects create vs update |
| Create new | `WriteSource(mode=create)` | Explicit create |
| Update existing | `WriteSource(mode=update)` | Explicit update |
| Deploy large file | `ImportFromFile` | Bypasses token limits |

### Creating DDIC domains and data elements

`CreateObject` supports `DOMA/DD` and `DTEL/DE`. Both calls create the SAP
repository shell, write the typed DDIC property document while holding a
MODIFY lock, unlock, activate, and verify the resulting object. A data element
may reference an existing domain through `domain`.

```json
{
  "object_type": "DOMA/DD",
  "name": "ZORDER_STATUS",
  "description": "Order status",
  "package_name": "$TMP",
  "properties_json": "{\"dataType\":\"CHAR\",\"length\":1,\"decimals\":0,\"outputLength\":1}"
}
```

For `DTEL/DE`, use `domain`, `dataType`, `length`, `decimals`, and
`labels` (`short`, `medium`, `long`, `heading`) in `properties_json`. The
universal tool accepts the shorter form:
`SAP(action="create", target="DOMA ZORDER_STATUS", params={package:"$TMP", properties:{...}})`.

### Searching

| Task | Tool | Parameters |
|------|------|------------|
| Find in single object | `GrepObjects` | `object_urls=[url], pattern=regex` |
| Find in multiple objects | `GrepObjects` | `object_urls=[url1, url2, ...], pattern=regex` |
| Find in package | `GrepPackages` | `packages=[PKG], pattern=regex` |
| Find in namespace | `GrepPackages` | `packages=[Z*], include_subpackages=true` |
| Find object by name | `SearchObject` | `query=Z*TEST*` |

---

## Tool Catalog (Focused Mode)

### GetSource - Unified Read Tool

**Purpose:** Read any ABAP object source code with a single tool.

**Parameters:**
| Parameter | Required | Values | Description |
|-----------|----------|--------|-------------|
| `object_type` | Yes | PROG, CLAS, INTF, FUNC, FUGR, INCL, DDLS, MSAG | Object type |
| `name` | Yes | string | Object name (uppercase) |
| `parent` | FUNC only | string | Function group name |
| `include` | CLAS only | definitions, implementations, macros, testclasses | Class include type |

**Examples:**

```json
// Read program
{ "object_type": "PROG", "name": "ZTEST" }

// Read class with test include
{ "object_type": "CLAS", "name": "ZCL_TEST", "include": "testclasses" }

// Read function module
{ "object_type": "FUNC", "name": "Z_MY_FM", "parent": "Z_MY_FG" }

// Read CDS view source
{ "object_type": "DDLS", "name": "ZRAY_00_I_DOC_NODE_00" }

// Read all messages in message class
{ "object_type": "MSAG", "name": "ZRAY_00" }
```

### EditSource - Surgical Edit

**Purpose:** Make small, precise changes with automatic syntax checking.

**Workflow:**
```mermaid
flowchart LR
    A[Find old_string] --> B{Unique?}
    B -->|No| ERR[Error: not unique]
    B -->|Yes| C[Replace with new_string]
    C --> D[Syntax Check]
    D -->|Errors| ERR2[Abort, no changes]
    D -->|OK| E[Lock → Update → Unlock → Activate]
```

**Parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `object_url` | Yes | ADT URL (e.g., `/sap/bc/adt/programs/programs/ZTEST`) |
| `old_string` | Yes | Exact string to find and replace |
| `new_string` | Yes | Replacement string |
| `replace_all` | No | Replace all occurrences (default: false) |
| `syntax_check` | No | Validate before saving (default: true) |
| `case_insensitive` | No | Case-insensitive matching (default: false) |

**Best Practice:** Include context for uniqueness:
```json
{
  "old_string": "METHOD calculate.\n    rv_result = 0.\n  ENDMETHOD.",
  "new_string": "METHOD calculate.\n    rv_result = iv_a + iv_b.\n  ENDMETHOD."
}
```

### GrepObjects / GrepPackages - Pattern Search

**Purpose:** Find patterns before editing, audit code, prepare refactoring.

**Regex Syntax:** Go regexp (NOT PCRE)
- `\w+` - word characters
- `\s+` - whitespace
- `(?i)` - case insensitive flag
- NO lookahead/lookbehind

**Common ABAP Patterns:**
```regex
TODO|FIXME                    # Find TODO comments
lv_\w+                        # Local variables
gv_\w+                        # Global variables
SELECT.*FROM\s+(\w+)          # SELECT statements
CALL FUNCTION\s+'(\w+)'       # Function calls
AUTHORITY-CHECK               # Auth checks
(?i)password|pwd|secret       # Credentials (case-insensitive)
```

### RunQuery - SQL Execution

**Purpose:** Execute freestyle SQL against SAP database.

**IMPORTANT:** Use ABAP SQL syntax!

**Working Examples:**
```sql
-- Simple query
SELECT * FROM t000

-- With filter
SELECT * FROM sflight WHERE carrid = 'LH'

-- With aggregation (ASCENDING/DESCENDING, not ASC/DESC!)
SELECT carrid, COUNT(*) as cnt FROM sflight GROUP BY carrid ORDER BY cnt DESCENDING

-- Use max_rows parameter for limiting, NOT LIMIT keyword
```

**Parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `sql_query` | Yes | ABAP SQL query |
| `max_rows` | No | Row limit (default: 100) |

### GetCDSDependencies - CDS Analysis

**Purpose:** Get the dependency tree of a CDS view.

**Returns:** Tables and views that the CDS view depends on (forward dependencies).

```json
{
  "ddls_name": "ZRAY_00_I_DOC_NODE_00"
}
// Returns:
{
  "name": "ZRAY_00_I_DOC_NODE_00",
  "type": "CDS_VIEW",
  "children": [
    { "name": "ZLLM_00_NODE", "type": "TABLE", "relation": "FROM" },
    { "name": "ZRAY_00_DOC", "type": "TABLE", "relation": "FROM" }
  ]
}
```

---

## Common Workflows

### 1. Find and Replace Pattern

```mermaid
sequenceDiagram
    participant AI as AI Agent
    participant MCP as MCP Server
    participant SAP as SAP System

    AI->>MCP: GrepObjects(url, "lv_count = 10")
    MCP->>SAP: Search object
    SAP-->>MCP: 1 match at line 42
    MCP-->>AI: Match found

    AI->>MCP: EditSource(old="lv_count = 10", new="lv_count = 42")
    MCP->>SAP: Lock object
    MCP->>SAP: Syntax check
    MCP->>SAP: Update source
    MCP->>SAP: Unlock & activate
    SAP-->>MCP: Success
    MCP-->>AI: Edited and activated
```

### 2. Read CDS View and Dependencies

```
Step 1: GetSource(type=DDLS, name=ZRAY_00_I_DOC_NODE_00)
        → Returns CDS source code

Step 2: GetCDSDependencies(ddls_name=ZRAY_00_I_DOC_NODE_00)
        → Returns: ZLLM_00_NODE (TABLE), ZRAY_00_DOC (TABLE)

Step 3: GetTable(table_name=ZLLM_00_NODE)
        → Returns table structure
```

### 3. Package-Wide Refactoring

```
Step 1: GrepPackages(packages=["ZPACKAGE"], pattern="CALL FUNCTION 'OLD_FM'")
        → Returns: 3 objects with matches

Step 2: For each object:
        EditSource(
          object_url=obj.objectUrl,
          old_string="CALL FUNCTION 'OLD_FM'",
          new_string="CALL FUNCTION 'NEW_FM'",
          replace_all=true
        )
```

### 4. Understand Error Messages

```
Step 1: GetSource(type=MSAG, name=ZRAY_00)
        → Returns JSON with all messages:
        {
          "messages": [
            {"number": "001", "text": "Include & is empty"},
            {"number": "002", "text": "Object &1 : &2 not found"}
          ]
        }
```

---

## Error Handling

### EditSource Errors

| Error | Cause | Solution |
|-------|-------|----------|
| "matches 3 locations (not unique)" | old_string found multiple times | Add more context or use `replace_all=true` |
| "syntax errors" | New code has errors | Fix syntax, check is atomic (no changes made) |
| "old_string not found" | Exact match not found | Check whitespace, case, use GrepObjects first |
| "object locked" | Someone else has lock | Wait or contact lock owner |

### RunQuery Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `"DESC" is not allowed` | Used SQL `DESC` | Use `DESCENDING` instead |
| `"ASC" is not allowed` | Used SQL `ASC` | Use `ASCENDING` instead |
| `LIMIT not recognized` | Used SQL `LIMIT` | Use `max_rows` parameter |

### GetCDSDependencies Errors

| Error | Cause | Solution |
|-------|-------|----------|
| 404 Not Found | CDS view doesn't exist | Check name with SearchObject first |
| Empty children | View has no dependencies | View selects from parameter or is empty |

---

## Performance Tips

### Token Optimization

| Operation | Tokens | Better Alternative |
|-----------|--------|-------------------|
| GetSource (500 lines) | ~2,500 | GrepObjects (targeted) ~100 |
| WriteSource (full rewrite) | ~5,000 | EditSource (surgical) ~100 |
| Multiple GetProgram calls | ~10,000 | Single GrepPackages ~500 |

### Search Strategy

1. **Start narrow:** GrepObjects on known object first
2. **Expand if needed:** GrepPackages for package-wide
3. **Use filters:** Object type filters reduce noise
4. **Limit results:** max_results prevents overwhelming responses

---

## Summary: When to Use What

```mermaid
flowchart TD
    Q1{What do you need?}

    Q1 -->|Read source| GS[GetSource]
    Q1 -->|Find pattern| Q2{Scope?}
    Q1 -->|Make change| Q3{Size?}
    Q1 -->|Query data| Q4{Type?}
    Q1 -->|CDS info| CDS[GetCDSDependencies]

    Q2 -->|1-5 objects| GO[GrepObjects]
    Q2 -->|Package+| GP[GrepPackages]

    Q3 -->|Small <50 lines| ES[EditSource]
    Q3 -->|Large rewrite| WS[WriteSource]
    Q3 -->|Huge file| IF[ImportFromFile]

    Q4 -->|Table structure| GT[GetTable]
    Q4 -->|Table data| GTC[GetTableContents]
    Q4 -->|Complex SQL| RQ[RunQuery]
```

---

**Last Updated:** 2025-12-04
**MCP Server Version:** v2.2.0+
**Maintained by:** vibing-steampunk project


## Classic RFC (`action="rfc"`)

RFC is one more action on the universal `SAP` tool, so the tool space stays a single
tool. It talks to the same system over the gateway instead of ADT.

```
SAP(action="rfc", params={"op":"info"})                    RFC_SYSTEM_INFO
SAP(action="rfc", params={"op":"ping"})                    RFC_PING
SAP(action="rfc", target="BAPI_USER_*", params={"op":"search"})
SAP(action="rfc", target="STFC_CONNECTION")                describe (default with a target)
SAP(action="rfc", target="Z_DOUBLE", params={"op":"call","args":{"N":21}})
SAP(action="rfc", target="T000", params={"op":"read_table","fields":["MANDT"],"top":5})
```

Ops: `info`, `ping`, `describe`, `call`, `search`, `read_table`. Destination overrides
in `params`: `host`, `sysnr`, `port`, `user` — otherwise the host and system number
come from the configured ADT URL and the gateway is `3300 + sysnr`.
