# S/4HANA Custom Code Migration: VSP Technical Execution Guide

**Date:** 2026-02-24
**Report ID:** 003
**Subject:** Step-by-step Claude+VSP playbook for /CBA/ namespace custom code analysis
**Related Documents:** [Stakeholder Report](./2026-02-24-002-s4hana-custom-code-migration-analysis.md)

---

## 1. Prerequisites & Setup

### 1.1 VSP Configuration

Connect VSP to the managed SAP system in **read-only expert mode** for maximum tool coverage with safety:

```bash
# Recommended: Read-only analysis mode with /CBA/ package restriction
./vsp \
  --url http://managed-system-host:port \
  --user <SAP_USER> \
  --password <SAP_PASSWORD> \
  --client <CLIENT> \
  --read-only \
  --mode expert \
  --allowed-packages "/CBA/*"
```

Or via environment variables:
```bash
export SAP_URL=http://managed-system-host:port
export SAP_USER=<user>
export SAP_PASSWORD=<pass>
export SAP_CLIENT=<client>
export SAP_READ_ONLY=true
export SAP_MODE=expert
export SAP_ALLOWED_PACKAGES="/CBA/*"
./vsp
```

**Key flags:**
| Flag | Value | Purpose |
|------|-------|---------|
| `--read-only` | true | Block all write operations during analysis |
| `--mode` | expert | Enable all 122 tools (including call graph, CDS deps, etc.) |
| `--allowed-packages` | "/CBA/*" | Restrict scope to /CBA/ namespace |
| `--verbose` | (optional) | Enable stderr logging for troubleshooting |

### 1.2 odata_mcp_go Configuration (Approach 2)

For CCLM/BW data access from Solution Manager:

```bash
# Step 1: Discover available CCLM OData services
curl -u <SOLMAN_USER>:<SOLMAN_PASS> \
  "https://solman-host:port/sap/opu/odata/sap/?$format=json" | \
  jq '.d.EntitySets[] | select(contains("CCLM") or contains("CCM") or contains("CALM"))'
```

```bash
# Step 2: Run odata_mcp_go pointed at discovered CCLM service
./odata-mcp \
  --service "https://solman-host:port/sap/opu/odata/sap/AI_CCLM_CCLM_SRV/" \
  --user <SOLMAN_USER> \
  --password <SOLMAN_PASS> \
  --read-only \
  --claude-code-friendly \
  --lazy-metadata
```

**For BW OData services (alternative/supplement):**
```bash
./odata-mcp \
  --service "https://solman-host:port/sap/opu/odata/sap/CCLM_ANALYTICS_SRV/" \
  --user <SOLMAN_USER> \
  --password <SOLMAN_PASS> \
  --read-only \
  --claude-code-friendly
```

### 1.3 Claude MCP Configuration

For Claude Desktop or Claude Code, configure both servers:

```json
{
  "mcpServers": {
    "vsp": {
      "command": "/path/to/vsp",
      "args": [
        "--url", "http://managed-system:port",
        "--read-only",
        "--mode", "expert",
        "--allowed-packages", "/CBA/*"
      ],
      "env": {
        "SAP_USER": "your-user",
        "SAP_PASSWORD": "your-password",
        "SAP_CLIENT": "001"
      }
    },
    "solman-cclm": {
      "command": "/path/to/odata-mcp",
      "args": [
        "--service", "https://solman-host:port/sap/opu/odata/sap/AI_CCLM_CCLM_SRV/",
        "--read-only",
        "--claude-code-friendly",
        "--lazy-metadata"
      ],
      "env": {
        "ODATA_USERNAME": "solman-user",
        "ODATA_PASSWORD": "solman-password"
      }
    }
  }
}
```

### 1.4 Verification Steps

Run these checks before starting the analysis:

```
Step 1: Verify VSP connectivity
→ Tool: GetSystemInfo
→ Expected: System ID, release version, kernel version, database type
→ Note the SAP_BASIS release - needed for ATC variant selection

Step 2: Verify /CBA/ namespace is accessible
→ Tool: SearchObject
→ Parameters: query="/CBA/*", maxResults=10
→ Expected: List of /CBA/ objects with URI, type, name, package

Step 3: Verify ATC check variant availability
→ Tool: GetATCCustomizing (expert mode)
→ Expected: List of configured check variants
→ Look for: S4HANA_READINESS, S4HANA_READINESS_REMOTE, or version-specific variants

Step 4: Verify ZADT_VSP is operational
→ Tool: GetFeatures
→ Expected: Feature availability flags

Step 5: (If using Approach 2) Verify CCLM OData connectivity
→ Tool: filter_CustomCodeObjects (via odata_mcp_go)
→ Parameters: $top=5
→ Expected: Sample CCLM data entries
```

---

## 2. Phase 1: Inventory Compilation

### 2.1 Object Discovery

**Goal:** Build a complete list of all /CBA/ namespace objects.

```
Step 1: Search for all /CBA/ objects
→ Tool: SearchObject
→ Parameters:
    query: "/CBA/*"
    maxResults: 5000  (run multiple times if >5000)
→ Output: Array of {uri, type, name, package, description}
→ Action: Record total count, save full list

Step 2: If count exceeds single search limit, paginate:
→ Search by type: "/CBA/*" with objectType filter
    → Classes: "/CBA/CL_*"
    → Interfaces: "/CBA/IF_*"
    → Programs: "/CBA/*" (type=PROG)
    → Function groups: "/CBA/*" (type=FUGR)
    → CDS views: "/CBA/*" (type=DDLS)
→ Merge results, deduplicate by URI

Step 3: Map package structure
→ Tool: GrepPackages
→ Parameters:
    packageName: "/CBA/*"
    pattern: "" (empty - lists all objects)
→ Output: Package tree with object counts

Step 4: For complex objects, get structure detail
→ Tool: GetObjectStructure
→ Parameters: objectName (for top 50 largest classes)
→ Output: Methods, attributes, includes, interfaces
→ Purpose: Complexity assessment
```

### 2.2 Inventory Categorization

After collecting all objects, categorize:

```
Object Type Summary Template:
─────────────────────────────────────
Type        | Count | % of Total
─────────────────────────────────────
CLAS        | ____  | ____%
PROG        | ____  | ____%
FUGR/FUNC   | ____  | ____%
INTF        | ____  | ____%
DDLS (CDS)  | ____  | ____%
TABL        | ____  | ____%
VIEW        | ____  | ____%
BDEF        | ____  | ____%
SRVD        | ____  | ____%
SRVB        | ____  | ____%
Other       | ____  | ____%
─────────────────────────────────────
TOTAL       | ____  | 100%
```

---

## 3. Phase 2: ATC Analysis

### 3.1 Check Variant Selection

Determine which ATC check variant to use:

```
Decision Tree:
─────────────────────────────────────
1. Is a Central Check System (NW 7.52+) configured with remote stubs?
   ├── YES → Use S4HANA_READINESS_REMOTE (or version-specific variant)
   └── NO  → Is the managed system NW 7.50+ with simplification DB installed?
             ├── YES → Use S4HANA_READINESS (local variant)
             └── NO  → Use default ATC variant + manual deprecated API scan (Phase 4)

Version-specific variants (use one matching your target S/4HANA release):
  • S4HANA_READINESS_2020
  • S4HANA_READINESS_2021
  • S4HANA_READINESS_2022
  • S4HANA_READINESS_2023
  • S4HANA_READINESS_2025

Verify availability:
→ Tool: GetATCCustomizing
→ Look for variant names containing "S4HANA" or "READINESS"
```

### 3.2 Batch ATC Execution Strategy

**For 5,000-20,000 objects, use a tiered approach:**

```
Tier 1: Critical Sample (500-1,000 objects)
─────────────────────────────────────
Selection criteria:
  • Top packages by object count
  • All CDS views (highest migration impact)
  • All classes implementing key interfaces
  • All programs/reports called in production (if usage data available)

Execution:
→ Tool: RunATCCheck
→ Parameters:
    objectURL: <object ADT URI>
    variant: "S4HANA_READINESS" (or version-specific)
    maxResults: 100
→ Repeat for each object in Tier 1

Estimated time: ~10-15 sec per object × 1,000 = ~3-4 hours

Tier 2: Namespace-Wide ATC (All objects)
─────────────────────────────────────
Option A: Package-level ATC via report (if supported)
→ Tool: RunReportAsync
→ Parameters:
    report: "RSATCCHECK" (or "RS_ATC_CHECK")
    variant: (create variant with /CBA/* scope)
→ This runs ATC across all objects in scope as a background job

Option B: Iterate remaining objects (if report approach unavailable)
→ Loop RunATCCheck for remaining objects
→ Use parallel Claude sessions if needed
→ Estimated: ~14-83 hours depending on object count

Tier 3: Focused Deep Checks
─────────────────────────────────────
For objects with Tier 1/2 findings:
→ Run additional checks (syntax, code inspector)
→ Get call graph for blast radius assessment
```

### 3.3 Results Aggregation

Aggregate ATC findings into this structure:

```
ATC Findings Summary Template:
─────────────────────────────────────
Priority 1 (Error):    ____ findings across ____ objects
Priority 2 (Warning):  ____ findings across ____ objects
Priority 3 (Info):     ____ findings across ____ objects
─────────────────────────────────────
Total:                 ____ findings across ____ objects

Top 10 Check Categories:
─────────────────────────────────────
Category                        | Count | Priority
─────────────────────────────────────
Removed/changed function module | ____  | 1
Cluster table access (BSEG)     | ____  | 1
Changed data model              | ____  | 1
Deprecated BAPI                 | ____  | 2
Performance recommendation      | ____  | 3
...                             |       |
─────────────────────────────────────

Top 20 Most Impacted Objects:
─────────────────────────────────────
Object Name          | Type | Findings | P1 | P2 | P3
─────────────────────────────────────
/CBA/CL_EXAMPLE_001  | CLAS | 15       | 5  | 8  | 2
...                   |      |          |    |    |
─────────────────────────────────────
```

---

## 4. Phase 3: Usage Analysis (via Solution Manager)

### 4.1 CCLM OData Queries

**Using odata_mcp_go pointed at Solution Manager:**

```
Step 1: Query usage statistics
→ Tool: filter_UsageStatistics (or generic OData query)
→ Parameters:
    $filter: Namespace eq '/CBA/'
    $select: ObjectName, ObjectType, UsageCount, LastUsedDate, SystemID
    $orderby: UsageCount desc
    $top: 10000
→ Output: Usage data per object

Step 2: Query quality results (historical ATC)
→ Tool: filter_QualityResults
→ Parameters:
    $filter: Namespace eq '/CBA/'
    $select: ObjectName, CheckVariant, FindingCount, LastCheckDate
→ Output: Historical quality metrics

Step 3: Query object ownership
→ Tool: filter_CustomCodeObjects
→ Parameters:
    $filter: Namespace eq '/CBA/'
    $select: ObjectName, Owner, Contract, CreationDate, Package
→ Output: Ownership metadata
```

### 4.2 BW Cube Queries (Alternative)

If CCLM OData services are not available or insufficient:

```
→ Tool: filter_{BW_CUBE_ENTITY}
→ Parameters:
    $filter: contains(ObjectName, '/CBA/')
→ Note: BW cube entity names depend on your SolMan configuration
→ Common: SCMON usage aggregates, ATC results aggregates
```

### 4.3 Usage Classification

Apply this classification to each object:

```
Classification Decision Tree:
─────────────────────────────────────
Is the object in SCMON usage data?
├── YES → When was it last used?
│   ├── Last 3 months → ACTIVE (must remediate)
│   ├── 3-12 months ago → DORMANT (review for retirement)
│   └── 12+ months ago → DEAD (candidate for retirement)
└── NO  → Is it called by an ACTIVE object? (check via call graph)
    ├── YES → INFRASTRUCTURE (must remediate - indirect usage)
    └── NO  → DEAD (strong candidate for retirement)
```

### 4.4 Usage Summary Template

```
Usage Classification Summary:
─────────────────────────────────────
Category        | Count | % of Total | Action
─────────────────────────────────────
Active          | ____  | ____%      | Must remediate
Infrastructure  | ____  | ____%      | Must remediate
Dormant         | ____  | ____%      | Review & decide
Dead            | ____  | ____%      | Retire
Unknown         | ____  | ____%      | Investigate
─────────────────────────────────────
TOTAL           | ____  | 100%       |

Expected reduction: ~30-40% of objects (Dead + partial Dormant)
Adjusted migration scope: ____ objects (down from ____)
```

---

## 5. Phase 4: Deep Analysis

### 5.1 Deprecated API Pattern Scan

Run GrepPackages for each known S/4HANA incompatible pattern:

```
Pattern 1: Cluster Table Access (BSEG)
→ Tool: GrepPackages
→ Parameters:
    packageName: "/CBA/*"
    pattern: "SELECT.*FROM.*BSEG"
    caseSensitive: false
→ Replacement: Use ACDOCA (Universal Journal)
→ Severity: High (table no longer exists in S/4HANA)

Pattern 2: Cluster Table Access (KONV)
→ Tool: GrepPackages
→ Parameters:
    packageName: "/CBA/*"
    pattern: "SELECT.*FROM.*KONV"
    caseSensitive: false
→ Replacement: Use PRCD_ELEMENTS
→ Severity: High

Pattern 3: Deprecated BAPIs
→ Tool: GrepPackages
→ Parameters:
    packageName: "/CBA/*"
    pattern: "CALL FUNCTION.*BAPI_MATERIAL_SAVEDATA"
→ Note: Run for each known deprecated BAPI. Key ones:
    • BAPI_MATERIAL_SAVEDATA → Use MM_MATERIAL_API
    • BAPI_COSTCENTER_GETLIST → Use Cost Center API
    • BAPI_ACC_DOCUMENT_POST → Use Journal Entry API
→ Severity: Medium-High

Pattern 4: Obsolete TABLES Parameter
→ Tool: GrepPackages
→ Parameters:
    packageName: "/CBA/*"
    pattern: "TABLES\\s+\\w+\\s+STRUCTURE"
→ Replacement: TYPE TABLE OF
→ Severity: Medium

Pattern 5: Classic List Processing
→ Tool: GrepPackages
→ Parameters:
    packageName: "/CBA/*"
    pattern: "^\\s*WRITE[:/]"
    caseSensitive: false
→ Note: Not an error but indicates classic dynpro. Review for Fiori migration
→ Severity: Low (informational)

Pattern 6: CALL TRANSACTION
→ Tool: GrepPackages
→ Parameters:
    packageName: "/CBA/*"
    pattern: "CALL TRANSACTION"
    caseSensitive: false
→ Note: Check if target transaction has Fiori equivalent
→ Severity: Low-Medium

Pattern 7: Direct DB Modifications to Standard Tables
→ Tool: GrepPackages
→ Parameters:
    packageName: "/CBA/*"
    pattern: "(INSERT|UPDATE|MODIFY|DELETE)\\s+\\w+\\s+FROM"
→ Note: Check if modifying SAP standard tables directly
→ Severity: Varies

Pattern 8: Currency/Quantity Field Handling
→ Tool: GrepPackages
→ Parameters:
    packageName: "/CBA/*"
    pattern: "CURR(ENCY)?.*AMOUNT|QUANTITY.*UNIT"
→ Note: S/4HANA has changed currency/quantity handling
→ Severity: Medium

Pattern 9: Sequential Number Ranges
→ Tool: GrepPackages
→ Parameters:
    packageName: "/CBA/*"
    pattern: "NUMBER_GET_NEXT"
→ Note: Some number range objects changed in S/4HANA
→ Severity: Low-Medium

Pattern 10: ABAP Obsolete Statements
→ Tool: GrepPackages
→ Parameters:
    packageName: "/CBA/*"
    pattern: "(COMPUTE|MOVE-CORRESPONDING|MULTIPLY|DIVIDE|ADD|SUBTRACT)\\s"
→ Note: Not strictly S/4HANA but indicates old coding style
→ Severity: Low (informational)
```

### 5.2 Dependency Analysis for Critical Objects

For each object with Priority 1 ATC findings:

```
Step 1: Get upward dependencies (who calls this?)
→ Tool: GetCallersOf
→ Parameters:
    objectURI: <object URI>
    maxDepth: 3
→ Output: Caller chain (blast radius)

Step 2: Get downward dependencies (what does this call?)
→ Tool: GetCalleesOf
→ Parameters:
    objectURI: <object URI>
    maxDepth: 3
→ Output: Dependency chain

Step 3: For CDS views, get CDS-specific dependencies
→ Tool: GetCDSDependencies
→ Parameters:
    ddlsName: <CDS view name>
→ Output: Dependency tree with cycle detection

Step 4: Analyze blast radius
→ Tool: AnalyzeCallGraph
→ Parameters:
    objectURI: <object URI>
    direction: "callers"
    maxDepth: 5
→ Output: Node count, edge count, max depth = blast radius score
```

### 5.3 AI-Powered Code Review

For the top 50-100 highest-priority objects, have Claude analyze the source:

```
Step 1: Read source code
→ Tool: GetSource
→ Parameters:
    type: <CLAS/PROG/FUNC/etc>
    name: <object name>
→ Output: Full ABAP source code

Step 2: Prompt Claude for analysis
"Analyze this ABAP code for S/4HANA readiness. Identify:
1. Any direct access to removed/changed tables (BSEG, KONV, BKPF changes)
2. Calls to deprecated function modules or BAPIs
3. Usage of obsolete ABAP statements
4. Classic dynpro patterns that need Fiori migration
5. Performance anti-patterns for HANA
6. Data model dependencies on changed structures
7. Authority check objects that may not exist in S/4HANA

For each finding, provide:
- Line number(s)
- Severity (Critical/High/Medium/Low)
- Current code pattern
- Recommended replacement
- Estimated remediation effort"

Step 3: Validate AI-proposed fixes
→ Tool: SyntaxCheck (if remediation mode is enabled later)
→ Parameters:
    objectURL: <object URI>
    content: <proposed fixed code>
→ Output: Syntax validation result
```

---

## 6. Phase 5: Migration Plan Generation

### 6.1 Scoring Methodology

Assign each object a migration priority score:

```
Score = (ATC_Severity × 3) + (Usage_Weight × 2) + (Dependency_Score × 1)

Where:
  ATC_Severity:
    0 = No findings
    1 = Info findings only
    2 = Warning findings
    3 = Error findings

  Usage_Weight:
    0 = Dead (no usage)
    1 = Dormant (3-12 months)
    2 = Active (used in last 3 months)
    3 = Critical infrastructure (called by many active objects)

  Dependency_Score:
    0 = Leaf node (no callers)
    1 = 1-5 callers
    2 = 6-20 callers
    3 = 20+ callers (high blast radius)

Priority Bands:
  0-3  = Low priority (keep or minor refactor)
  4-6  = Medium priority (scheduled refactor)
  7-9  = High priority (immediate attention)
  10+  = Critical (migration blocker)
```

### 6.2 Classification Decision Tree

```
For each /CBA/ object, classify:
─────────────────────────────────────

Is the object DEAD (no usage)?
├── YES → RETIRE
│   Effort: Low (delete object + transport)
│   Risk: Low (verify no indirect calls first)
│
└── NO → Does it have ATC findings?
    ├── NO → KEEP AS-IS
    │   Effort: None
    │   Risk: None
    │
    └── YES → How severe?
        ├── Info only → KEEP (note for future cleanup)
        │
        ├── Warnings → REFACTOR
        │   Sub-classify:
        │   ├── API replacement available → Simple refactor (1-3 days)
        │   ├── Data model change → Medium refactor (3-10 days)
        │   └── Behavioral change → Needs investigation
        │
        └── Errors → REWRITE or REFACTOR
            Sub-classify:
            ├── Removed table access → REWRITE data layer (5-20 days)
            ├── Removed function module → REFACTOR to replacement (3-10 days)
            ├── Structural incompatibility → REWRITE (10-30 days)
            └── Can be replaced by standard → RETIRE + configure standard
```

### 6.3 Remediation Ordering Rules

```
Order remediation work by:

1. DEPENDENCIES FIRST
   → Fix leaf nodes before their callers
   → Use GetCalleesOf to determine order
   → Example: Fix utility class before programs that use it

2. PACKAGE GROUPING
   → Group remediation by package for transport efficiency
   → One transport per package per sprint

3. PRIORITY SCORING
   → Within same dependency level, fix highest-priority first
   → Critical infrastructure objects before low-usage objects

4. EFFORT BALANCING
   → Mix quick wins (API replacements) with complex rewrites
   → Target 70% quick wins / 30% complex per sprint

Suggested Sprint Structure (2-week sprints):
─────────────────────────────────────
Sprint 1: Quick wins - API replacements in top packages
Sprint 2: Data model changes (BSEG/KONV replacements)
Sprint 3: Complex rewrites - structural changes
Sprint 4: CDS view migrations
Sprint 5: Testing & validation
Sprint 6: Transport & deployment
```

### 6.4 Migration Summary Template

```
Migration Plan Summary:
═════════════════════════════════════

SCOPE
  Total /CBA/ objects:        ____
  After usage reduction:      ____ (-___%)

CLASSIFICATION
  Retire (dead code):         ____ (___%)
  Keep as-is:                 ____ (___%)
  Refactor (minor):           ____ (___%)
  Rewrite (major):            ____ (___%)

EFFORT ESTIMATE
  Retire:        ____ objects × 0.5 days = ____ person-days
  Refactor:      ____ objects × 3 days   = ____ person-days
  Rewrite:       ____ objects × 15 days  = ____ person-days
  Testing:       30% of remediation      = ____ person-days
  ──────────────────────────────────────────────────────
  TOTAL:                                   ____ person-days

TIMELINE (with 2 ABAP developers)
  Phase 1 - Retire dead code:     ____ weeks
  Phase 2 - Quick refactors:      ____ weeks
  Phase 3 - Complex rewrites:     ____ weeks
  Phase 4 - Testing/validation:   ____ weeks
  Phase 5 - Transport/deploy:     ____ weeks
  ──────────────────────────────────────────
  TOTAL:                          ____ weeks

RISK ASSESSMENT
  High-risk objects (rewrite):    ____
  Medium-risk (refactor):         ____
  Low-risk (retire/keep):         ____
```

---

## 7. Appendix: Tool Reference

### VSP Tools Used in This Analysis

| Tool | Mode | Phase | Purpose | Key Parameters |
|------|------|-------|---------|---------------|
| `GetSystemInfo` | Focused | Setup | System landscape profiling | (none) |
| `GetInstalledComponents` | Focused | Setup | Component inventory | (none) |
| `GetFeatures` | System | Setup | Feature availability check | (none) |
| `GetATCCustomizing` | Expert | Setup | ATC variant discovery | (none) |
| `SearchObject` | Focused | Inventory | Object discovery | query, maxResults |
| `GrepPackages` | Focused | Inventory, Patterns | Package-wide search | packageName, pattern, caseSensitive |
| `GetObjectStructure` | Focused | Inventory | Object complexity | objectName |
| `RunATCCheck` | Focused | ATC | S/4HANA readiness check | objectURL, variant, maxResults |
| `RunReportAsync` | Focused | ATC (batch) | Namespace-wide ATC | report, variant, params |
| `GetCallGraph` | Focused | Dependencies | Bidirectional call hierarchy | objectURI, direction, maxDepth |
| `GetCallersOf` | Expert | Dependencies | Upward traversal | objectURI, maxDepth |
| `GetCalleesOf` | Expert | Dependencies | Downward traversal | objectURI, maxDepth |
| `AnalyzeCallGraph` | Expert | Dependencies | Graph statistics | objectURI, direction, maxDepth |
| `GetCDSDependencies` | Focused | Dependencies | CDS view chains | ddlsName |
| `GetSource` | Focused | Deep Analysis | Source code reading | type, name |
| `FindReferences` | Focused | Deep Analysis | API usage scoping | objectURL, identifier |
| `GrepObjects` | Focused | Patterns | Multi-object search | objectURLs, pattern |
| `GetDumps` | Focused | Runtime | Runtime error discovery | (filters) |
| `ListTraces` | Focused | Runtime | Performance traces | (none) |
| `GitExport` | Focused | Backup | Code export to abapGit format | packages, objects |

### odata_mcp_go Tools (for CCLM/BW Access)

| Tool | Purpose | Key Parameters |
|------|---------|---------------|
| `filter_{Entity}` | Query CCLM entities | $filter, $select, $orderby, $top |
| `count_{Entity}` | Count entities | $filter |
| `get_{Entity}` | Get single entity | Key fields |

### VSP Safety Configuration for Remediation Phase

When transitioning from analysis to remediation:

```bash
# Remediation mode (after analysis is complete)
./vsp \
  --url http://managed-system-host:port \
  --mode expert \
  --allowed-packages "/CBA/*" \
  --allow-transportable-edits \
  --allowed-ops "RSQTCUA"  # Read, Search, Query, Test, Create, Update, Activate
  # Note: No D (delete) to prevent accidental deletion
```

---

## 8. Deprecated API Pattern Database

### S/4HANA Incompatible Patterns - Quick Reference

| # | Pattern (Regex) | Description | Replacement | Severity |
|---|----------------|-------------|-------------|----------|
| 1 | `SELECT.*FROM.*BSEG` | BSEG cluster table access | ACDOCA (Universal Journal) | Critical |
| 2 | `SELECT.*FROM.*KONV` | KONV cluster table access | PRCD_ELEMENTS | Critical |
| 3 | `SELECT.*FROM.*BKPF` | BKPF changed in S/4 | ACDOCA or compatibility view | High |
| 4 | `SELECT.*FROM.*BSIK\|BSID\|BSAK\|BSAD` | Open/cleared item tables | ACDOCA | High |
| 5 | `SELECT.*FROM.*COEP` | CO line items | ACDOCA | High |
| 6 | `CALL FUNCTION.*BAPI_MATERIAL_SAVEDATA` | Deprecated material BAPI | CL_MM_MATERIAL_MANAGE_API | High |
| 7 | `CALL FUNCTION.*BAPI_COSTCENTER_GETLIST` | Deprecated CC BAPI | CL_FINS_ACDOC_API | Medium |
| 8 | `CALL FUNCTION.*BAPI_ACC_DOCUMENT_POST` | Deprecated accounting BAPI | CL_FINS_ACDOC_API | High |
| 9 | `TABLES\s+\w+\s+STRUCTURE` | Obsolete TABLES parameter | TYPE TABLE OF | Medium |
| 10 | `CALL TRANSACTION` | Transaction call | Check Fiori app catalog | Low-Med |
| 11 | `MODIFY SCREEN` | Classic dynpro | Review for Fiori | Low |
| 12 | `TYPE.*BSEG\|KONV` | Type references to removed structures | Use replacement structures | High |
| 13 | `FIELD-SYMBOLS.*BSEG\|KONV` | Field-symbol typing to removed structures | Use replacement structures | High |
| 14 | `NUMBER_GET_NEXT` | Number range generation | Verify number object exists in S/4 | Low-Med |
| 15 | `AUTHORITY-CHECK.*OBJECT` | Authority check | Verify auth object exists in S/4 | Medium |
| 16 | `SELECT.*FOR ALL ENTRIES` | Performance pattern | Review for HANA optimization | Low |
| 17 | `SELECT.*INTO CORRESPONDING` | Performance anti-pattern | Use field list for HANA | Low |
| 18 | `COMMUNICATION.*RECEIVE\|SEND` | RFC communication | Review connectivity approach | Medium |
| 19 | `CLASS.*DEFINITION.*INHERITING FROM.*CL_GUI` | SAP GUI dependency | Fiori migration candidate | Low |
| 20 | `SUBMIT.*VIA SELECTION-SCREEN` | Report submission | Review for async execution | Low |

### Running All Patterns

To scan for all patterns, execute GrepPackages for each row above with `packageName: "/CBA/*"`.

Expected output per pattern: list of matching objects with line numbers and context.

Aggregate all matches into the ATC findings for a complete picture.

---

## 9. Troubleshooting

| Issue | Symptom | Resolution |
|-------|---------|-----------|
| ATC variant not found | RunATCCheck returns "variant not found" | Check GetATCCustomizing output; may need to create variant or use Central Check System |
| Search returns no results | SearchObject("/CBA/*") returns empty | Verify namespace registration; try SearchObject("/CBA/") without wildcard |
| CCLM OData 404 | odata_mcp_go cannot connect | Service may not be activated; check SICF in Solution Manager |
| CSRF token failure | odata_mcp_go 403 errors | Token auto-refresh should handle; verify user authorizations |
| ATC timeout | RunATCCheck hangs for large objects | Increase timeout; use RunReportAsync for namespace-wide |
| Package restriction blocks | VSP rejects operations | Verify --allowed-packages matches /CBA/ format (note the slashes) |
