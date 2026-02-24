# Custom Code Analysis Prompts for Claude + VSP

**Date:** 2026-02-24
**Report ID:** 004
**Subject:** Copy-pasteable Claude prompts for /CBA/ namespace custom code inventory and detailed analysis
**Related Documents:** [Execution Guide](./2026-02-24-003-s4hana-migration-vsp-execution-guide.md)

---

## Overview

Two prompts designed to be used with Claude connected to VSP (MCP server) against the managed SAP system:

1. **Prompt 1** - Inventory compilation → Excel output
2. **Prompt 2** - Detailed analysis using the Excel baseline → Updated Excel with findings

Both prompts assume VSP is running in `--read-only --mode expert --allowed-packages "/CBA/*"` configuration.

---

## Prompt 1: Custom Code Inventory Compilation

Copy and paste this prompt into Claude when VSP is connected to the managed SAP system.

---

```
You are performing a custom code inventory analysis for an SAP S/4HANA migration project. The target namespace is /CBA/ (a registered SAP namespace). You have access to VSP (an MCP server connected to the SAP system) with 122 tools.

Your task is to compile a comprehensive inventory of ALL custom code objects under the /CBA/ namespace and output the results as a detailed Excel (.xlsx) file.

## Step 1: System Landscape Profile

Run these tools and record the results:
- GetSystemInfo → Record: System ID, Release, Kernel, Database, Instance
- GetInstalledComponents → Record: SAP_BASIS version, S4CORE (if present), other key components
- GetFeatures → Record: Which features are available (abapGit, RAP, AMDP, etc.)

## Step 2: Discover All /CBA/ Objects

Search for all objects in the /CBA/ namespace. Run multiple searches to ensure complete coverage:

1. SearchObject with query="/CBA/*" and maxResults=5000
2. If results hit the limit, run additional targeted searches:
   - SearchObject with query="/CBA/CL_*" (classes)
   - SearchObject with query="/CBA/IF_*" (interfaces)
   - SearchObject with query="/CBA/*" filtering by type if needed
3. Deduplicate results by object URI

For EACH object discovered, record:
- Object URI (ADT path)
- Object Type (CLAS, PROG, FUGR, FUNC, INTF, DDLS, TABL, VIEW, DTEL, DOMA, MSAG, TTYP, etc.)
- Object Name
- Package
- Description (from search result)

## Step 3: Enrich with Structure Data

For each CLASS (CLAS type) discovered:
- Run GetClassInfo to get: method count, attribute count, interface implementations
- Record: methodCount, attributeCount, interfaceCount

For each CDS VIEW (DDLS type) discovered:
- Run GetCDSDependencies to get: dependency count, any cycles
- Record: dependencyCount, hasCycles

## Step 4: Quick ATC Scan (Sample)

Run RunATCCheck on a representative sample of objects (up to 200 objects, prioritizing classes and programs) using the S4HANA readiness check variant. For the variant name:
- First run GetATCCustomizing to discover available variants
- Use whichever variant contains "S4HANA" or "READINESS" in its name
- If no S4HANA variant exists, use the default variant

For each sampled object, record:
- ATC Error count (Priority 1)
- ATC Warning count (Priority 2)
- ATC Info count (Priority 3)
- Top finding category (most frequent check ID)

For objects NOT in the ATC sample, leave ATC columns as "Not Yet Scanned".

## Step 5: Generate Excel Output

Create a Python script using openpyxl to generate a properly formatted Excel file. The Excel file should contain these worksheets:

### Sheet 1: "System Profile"
| Field | Value |
|-------|-------|
| System ID | (from GetSystemInfo) |
| Release | (from GetSystemInfo) |
| Database | (from GetSystemInfo) |
| SAP_BASIS Version | (from GetInstalledComponents) |
| Analysis Date | (today's date) |
| Total Custom Objects | (count) |
| Namespace | /CBA/ |

### Sheet 2: "Object Inventory" (Main Sheet)
Columns:
| Column | Header | Description | Width |
|--------|--------|-------------|-------|
| A | Object Name | Full object name (e.g., /CBA/CL_EXAMPLE) | 40 |
| B | Object Type | ABAP object type (CLAS, PROG, FUGR, etc.) | 10 |
| C | Package | Development package | 30 |
| D | Description | Object description | 50 |
| E | Object URI | ADT URI for reference | 60 |
| F | Method Count | Number of methods (classes only) | 15 |
| G | Attribute Count | Number of attributes (classes only) | 15 |
| H | Interface Count | Interfaces implemented (classes only) | 15 |
| I | CDS Dep Count | CDS dependency count (DDLS only) | 15 |
| J | CDS Has Cycles | Circular dependencies? (DDLS only) | 15 |
| K | ATC Errors (P1) | Priority 1 finding count | 15 |
| L | ATC Warnings (P2) | Priority 2 finding count | 15 |
| M | ATC Info (P3) | Priority 3 finding count | 15 |
| N | ATC Top Finding | Most frequent check category | 30 |
| O | ATC Scan Status | "Scanned" or "Not Yet Scanned" | 18 |
| P | Usage Status | Blank (to be filled in Phase 2 from CCLM) | 15 |
| Q | Migration Action | Blank (to be filled in Phase 2) | 15 |
| R | Estimated Effort | Blank (to be filled in Phase 2) | 15 |
| S | Priority Score | Blank (to be filled in Phase 2) | 15 |
| T | Notes | Blank (for analyst notes) | 50 |

Formatting requirements:
- Header row: Bold, light blue background (#D6EAF8), freeze panes
- Auto-filter on all columns
- Conditional formatting on ATC Errors: Red if > 0
- Conditional formatting on ATC Warnings: Orange if > 0
- Column widths as specified above
- Sort by: Object Type (A-Z), then Object Name (A-Z)

### Sheet 3: "Summary by Type"
Pivot summary:
| Object Type | Count | % of Total | ATC Scanned | Avg P1 Findings | Avg P2 Findings |
|-------------|-------|-----------|-------------|-----------------|-----------------|
| CLAS | ... | ... | ... | ... | ... |
| PROG | ... | ... | ... | ... | ... |
| (etc.) | | | | | |

### Sheet 4: "Summary by Package"
Pivot summary:
| Package | Object Count | Types Present | ATC Errors Total | ATC Warnings Total |
|---------|-------------|---------------|------------------|-------------------|
| /CBA/PKG_001 | ... | CLAS, PROG, ... | ... | ... |
| (etc.) | | | | |

### Sheet 5: "ATC Findings Detail"
For each ATC finding from the sampled objects:
| Object Name | Object Type | Check ID | Check Title | Priority | Message | Line | Column |
|-------------|-----------|----------|-------------|----------|---------|------|--------|
| ... | ... | ... | ... | ... | ... | ... | ... |

Save the file as: /Users/VincentSegami/Documents/GitHub/vibing-steampunk/output/cba-custom-code-inventory.xlsx

## Important Notes

- Work methodically. Process objects in batches to avoid timeouts.
- If SearchObject pagination is needed, be thorough - do not miss objects.
- For the ATC sample: prioritize classes (CLAS) and programs (PROG) as they typically have the most findings.
- Log any errors or objects that couldn't be processed in a "Processing Log" sheet.
- Print a summary at the end: total objects found, objects by type, ATC scan coverage percentage.
- Do NOT modify any SAP objects - this is a read-only analysis.
```

---

## Prompt 2: Detailed Custom Code Analysis

Use this prompt AFTER Prompt 1 has completed and the Excel baseline exists. This prompt reads the Excel, performs deep analysis, and updates it with findings.

---

```
You are performing a detailed custom code analysis for an SAP S/4HANA migration project. The namespace is /CBA/. You have access to VSP (MCP server) connected to the SAP system.

A baseline inventory Excel file exists at:
/Users/VincentSegami/Documents/GitHub/vibing-steampunk/output/cba-custom-code-inventory.xlsx

Your task is to perform a comprehensive S/4HANA readiness analysis of EVERY object in this inventory and produce an updated Excel with full findings, classification, and migration recommendations.

## Step 0: Load the Baseline

Read the Excel file using Python (openpyxl). Load the "Object Inventory" sheet. Count total objects and report the breakdown by type.

## Step 1: Complete ATC Coverage

For every object that has ATC Scan Status = "Not Yet Scanned":
- Run RunATCCheck with the S4HANA readiness variant (check GetATCCustomizing first for the variant name)
- Record: P1 count, P2 count, P3 count, top finding category
- Update the ATC columns in the Excel
- Work in batches of 50 objects. After each batch, save progress to the Excel file.

After completing: Report ATC coverage (should be 100%).

## Step 2: Deprecated API Pattern Scan

Run GrepPackages across the /CBA/ namespace for each of these critical S/4HANA incompatibility patterns. For EACH pattern, record which objects are affected:

Pattern Group 1 - CRITICAL (Removed Tables):
1. GrepPackages(packageName="/CBA/*", pattern="SELECT.*FROM.*BSEG", caseSensitive=false)
   → Issue: BSEG cluster table removed in S/4HANA
   → Replacement: ACDOCA (Universal Journal)

2. GrepPackages(packageName="/CBA/*", pattern="SELECT.*FROM.*KONV", caseSensitive=false)
   → Issue: KONV cluster table removed in S/4HANA
   → Replacement: PRCD_ELEMENTS

3. GrepPackages(packageName="/CBA/*", pattern="SELECT.*FROM.*BSIK|BSID|BSAK|BSAD", caseSensitive=false)
   → Issue: Open/cleared item tables replaced
   → Replacement: ACDOCA

4. GrepPackages(packageName="/CBA/*", pattern="SELECT.*FROM.*COEP", caseSensitive=false)
   → Issue: CO line items changed
   → Replacement: ACDOCA

5. GrepPackages(packageName="/CBA/*", pattern="TYPE.*BSEG|KONV", caseSensitive=false)
   → Issue: Type references to removed structures
   → Replacement: New structure types

Pattern Group 2 - HIGH (Deprecated APIs):
6. GrepPackages(packageName="/CBA/*", pattern="CALL FUNCTION.*BAPI_MATERIAL_SAVEDATA", caseSensitive=false)
   → Replacement: CL_MM_MATERIAL_MANAGE_API

7. GrepPackages(packageName="/CBA/*", pattern="CALL FUNCTION.*BAPI_ACC_DOCUMENT_POST", caseSensitive=false)
   → Replacement: CL_FINS_ACDOC_API

8. GrepPackages(packageName="/CBA/*", pattern="CALL FUNCTION.*BAPI_COSTCENTER", caseSensitive=false)
   → Replacement: Check SAP Note for replacement

9. GrepPackages(packageName="/CBA/*", pattern="CALL FUNCTION.*CONVERSION_EXIT", caseSensitive=false)
   → Note: Check if conversion exits still exist in S/4

Pattern Group 3 - MEDIUM (Structural Issues):
10. GrepPackages(packageName="/CBA/*", pattern="TABLES\\s+\\w+", caseSensitive=false)
    → Issue: Obsolete TABLES parameter
    → Replacement: TYPE TABLE OF / CHANGING TYPE TABLE OF

11. GrepPackages(packageName="/CBA/*", pattern="FIELD-SYMBOLS.*TYPE.*BSEG|KONV", caseSensitive=false)
    → Issue: Field-symbol typed to removed structures

12. GrepPackages(packageName="/CBA/*", pattern="AUTHORITY-CHECK OBJECT", caseSensitive=false)
    → Note: Verify authority objects exist in S/4HANA

Pattern Group 4 - LOW/INFORMATIONAL (Review Items):
13. GrepPackages(packageName="/CBA/*", pattern="CALL TRANSACTION", caseSensitive=false)
    → Note: Check if target tcode has Fiori app replacement

14. GrepPackages(packageName="/CBA/*", pattern="MODIFY SCREEN", caseSensitive=false)
    → Note: Classic dynpro - Fiori migration candidate

15. GrepPackages(packageName="/CBA/*", pattern="^\\s*WRITE[:/]", caseSensitive=false)
    → Note: Classic list processing

16. GrepPackages(packageName="/CBA/*", pattern="SUBMIT.*VIA SELECTION-SCREEN", caseSensitive=false)
    → Note: Report submission pattern

17. GrepPackages(packageName="/CBA/*", pattern="SELECT.*FOR ALL ENTRIES", caseSensitive=false)
    → Note: Performance review for HANA optimization

18. GrepPackages(packageName="/CBA/*", pattern="SELECT.*INTO CORRESPONDING", caseSensitive=false)
    → Note: Performance anti-pattern for HANA

19. GrepPackages(packageName="/CBA/*", pattern="ENDSELECT", caseSensitive=false)
    → Note: SELECT/ENDSELECT loop - performance concern

20. GrepPackages(packageName="/CBA/*", pattern="EXIT\\.|CHECK\\.", caseSensitive=false)
    → Note: Obsolete flow control in loops

For each pattern match, record the object name, line number, and matched code in a new "Deprecated Patterns" sheet.

## Step 3: Dependency Analysis

For the TOP 100 objects by ATC finding count (highest P1+P2):
- Run GetCallGraph with direction="callers" and maxDepth=3
- Record: caller count (blast radius), max depth reached
- Run GetCallGraph with direction="callees" and maxDepth=2
- Record: callee count (dependency footprint)

For ALL CDS views (DDLS type):
- Run GetCDSDependencies
- Record: total dependencies, table dependencies, cycles detected

Add these columns to the Excel:
| Column | Header |
|--------|--------|
| U | Caller Count (Blast Radius) |
| V | Callee Count (Dependencies) |
| W | Max Call Depth |
| X | Deprecated Pattern Hits |
| Y | Critical Pattern Hits |
| Z | Pattern Details |

## Step 4: AI-Powered Source Code Analysis

For the TOP 50 objects by combined score (ATC findings + pattern hits + blast radius):

1. Run GetSource to read the full source code
2. Analyze the code for:
   - S/4HANA incompatibilities not caught by ATC or pattern scan
   - Code quality issues (error handling, hardcoded values, missing comments)
   - Complexity assessment (nested IFs, large methods, God classes)
   - Clean Core compliance (usage of released vs unreleased APIs)
   - Estimated remediation complexity (Simple/Medium/Complex/Rewrite)
3. Write a 2-3 sentence analysis summary per object

Add column:
| Column | Header |
|--------|--------|
| AA | AI Analysis Summary |

## Step 5: Classification and Scoring

For EVERY object in the inventory, calculate:

### Priority Score (Column S):
Score = (ATC_Severity × 3) + (Usage_Weight × 2) + (Dependency_Score × 1)

Where:
- ATC_Severity: 0=None, 1=Info only, 2=Warnings, 3=Errors
- Usage_Weight: Default to 2 (Active) unless CCLM data says otherwise. Leave as 2 if Usage Status is blank.
- Dependency_Score: 0=No callers, 1=1-5 callers, 2=6-20 callers, 3=20+ callers
  (Use 1 as default if caller data not available)

### Migration Action (Column Q):
Apply this decision tree:
- Score 0-2 AND no ATC findings AND no pattern hits → "Keep"
- Score 0-2 AND only Info findings → "Keep (Review)"
- Score 3-5 AND warnings only AND < 3 pattern hits → "Refactor (Minor)"
- Score 3-5 AND errors OR > 3 pattern hits → "Refactor (Major)"
- Score 6-8 AND critical patterns (BSEG/KONV) → "Rewrite (Data Layer)"
- Score 6-8 AND deprecated BAPIs → "Refactor (API Replacement)"
- Score 9+ → "Rewrite (Full)" or "Investigate"
- If Usage Status = "Dead" → "Retire" (override)

### Estimated Effort (Column R):
- Keep: 0 days
- Keep (Review): 0.5 days
- Retire: 0.5 days
- Refactor (Minor): 2 days
- Refactor (Major): 5 days
- Refactor (API Replacement): 3 days
- Rewrite (Data Layer): 10 days
- Rewrite (Full): 20 days
- Investigate: 3 days (investigation effort)

## Step 6: Generate Updated Excel

Update the Excel file with ALL new data. Add these additional sheets:

### Sheet 6: "Deprecated Patterns"
| Object Name | Pattern Group | Pattern # | Severity | Matched Line | Line Number | Issue Description | Recommended Fix |
|-------------|--------------|-----------|----------|-------------|-------------|-------------------|----------------|

### Sheet 7: "Dependency Map"
| Object Name | Direction | Connected Object | Depth | Relationship Type |
|-------------|-----------|-----------------|-------|-------------------|

### Sheet 8: "AI Analysis Detail"
| Object Name | Analysis Summary | Complexity | Clean Core Status | Key Issues Found | Remediation Notes |
|-------------|-----------------|-----------|-------------------|-----------------|-------------------|

### Sheet 9: "Migration Dashboard"
Summary statistics:
| Metric | Value |
|--------|-------|
| Total Objects | |
| Retire | count (%) |
| Keep | count (%) |
| Keep (Review) | count (%) |
| Refactor (Minor) | count (%) |
| Refactor (Major) | count (%) |
| Refactor (API Replacement) | count (%) |
| Rewrite (Data Layer) | count (%) |
| Rewrite (Full) | count (%) |
| Investigate | count (%) |
| Total Estimated Effort | person-days |
| Critical Pattern Objects | count |
| High Blast Radius Objects (>20 callers) | count |
| Objects with ATC Errors | count |

### Sheet 10: "Risk Matrix"
| Risk Level | Object Count | Top Objects | Action Required |
|-----------|-------------|-------------|----------------|
| Critical (Score 10+) | | Top 5 names | Immediate investigation |
| High (Score 7-9) | | Top 5 names | Plan remediation |
| Medium (Score 4-6) | | Top 5 names | Schedule refactor |
| Low (Score 0-3) | | (count only) | Monitor |

Formatting for updated Excel:
- Conditional formatting on Migration Action:
  - "Retire" → Grey background
  - "Keep" → Green background
  - "Refactor (*)" → Yellow/Orange background
  - "Rewrite (*)" → Red background
  - "Investigate" → Purple background
- Conditional formatting on Priority Score:
  - 0-3: Green
  - 4-6: Yellow
  - 7-9: Orange
  - 10+: Red
- "Migration Dashboard" sheet: Include a text-based summary chart

Save the updated file as:
/Users/VincentSegami/Documents/GitHub/vibing-steampunk/output/cba-custom-code-analysis-complete.xlsx

## Step 7: Print Final Summary

After completion, print:
1. Total objects analyzed
2. ATC scan coverage (should be 100%)
3. Pattern scan results (objects affected per pattern group)
4. Classification breakdown (Retire/Keep/Refactor/Rewrite counts)
5. Total estimated remediation effort in person-days
6. Top 10 highest-risk objects by priority score
7. Top 5 objects by blast radius (caller count)

## Important Rules

- SAVE PROGRESS FREQUENTLY. After every batch of 50 objects, save the Excel.
- If any VSP tool call fails, log the error and continue with the next object. Don't stop.
- Do NOT modify any SAP objects. This is read-only analysis.
- Work methodically through the object list. Don't skip objects.
- If the ATC check variant is not available, note this and still complete the pattern scan and dependency analysis.
- Record processing time for reporting (start time at beginning, end time at completion).
- The output Excel must be usable as a standalone deliverable for stakeholders.
```

---

## Usage Instructions

### Prerequisites
1. VSP running and connected to the managed SAP system
2. Python 3 with `openpyxl` installed (`pip install openpyxl`)
3. `/output/` directory exists in the project root

### Execution Order
1. **Run Prompt 1 first** - This creates the baseline inventory Excel
2. **Verify the Excel** - Open it, check object counts match expectations
3. **Run Prompt 2** - This performs the detailed analysis and updates the Excel
4. **Review the output** - Check the Migration Dashboard sheet for the summary

### Expected Duration
- **Prompt 1:** 30-90 minutes (depending on object count and ATC sample size)
- **Prompt 2:** 2-6 hours (full ATC scan of all objects + pattern scan + dependency analysis + AI review)

### Tips
- Run Prompt 2 in stages if the context window fills up. You can re-paste the prompt and tell Claude "Continue from where you left off - the Excel has progress saved"
- The ATC scan is the most time-consuming part. Consider running it overnight
- If the managed system is slow, reduce the ATC batch size from 50 to 20
- The pattern scan (Step 2 of Prompt 2) is fast since GrepPackages searches the entire namespace at once

---

## Extending the Analysis

### Adding CCLM Usage Data (Approach 2)

If odata_mcp_go is configured for Solution Manager CCLM, add this to Prompt 2 before Step 5:

```
## Step 4b: Usage Data from CCLM

Using the odata_mcp_go MCP server connected to Solution Manager, query usage statistics:

1. Query CCLM for /CBA/ usage data:
   - filter_UsageStatistics with $filter=contains(ObjectName, '/CBA/')
   - Record: ObjectName, LastUsedDate, UsageCount, SystemID

2. Classify each object's usage:
   - Last used within 3 months → "Active"
   - Last used 3-12 months ago → "Dormant"
   - Last used 12+ months ago → "Dead"
   - No SCMON data → "Unknown"

3. Update column P (Usage Status) in the Excel with the classification.

4. For objects classified as "Dead":
   - Override Migration Action to "Retire" (unless they have active callers)
   - Run GetCallersOf to verify no active objects call this dead code
   - If active callers exist, change to "Infrastructure (Review)"
```

### Adding Transport History

```
## Step 4c: Transport Context

For each object:
1. Run GetTransportInfo to determine current transport status
2. Record: Transport number, transport status, target system
3. Add columns to Excel:
   - AB: Transport Number
   - AC: Transport Status
   - AD: Target System
```
