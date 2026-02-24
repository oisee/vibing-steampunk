# S/4HANA Custom Code Migration Analysis: VSP-Powered Approach

**Date:** 2026-02-24
**Report ID:** 002
**Subject:** Custom code analysis and migration strategy for SolMan-to-Focused-Run 5.2 migration
**Namespace:** /CBA/ (registered SAP namespace)
**Scale:** ~5,000-20,000 custom objects
**Related Documents:** [Technical Execution Guide](./2026-02-24-003-s4hana-migration-vsp-execution-guide.md)

---

## 1. Executive Summary

### Context

As part of the SAP Solution Manager to Focused Run 5.2 migration, all custom ABAP code under the `/CBA/` registered namespace must be analyzed for S/4HANA compatibility. Focused Run 5.2 runs on S/4HANA, making custom code readiness a prerequisite for successful migration.

### Scale & Scope

- **~5,000-20,000 custom objects** across the `/CBA/` namespace
- Objects span classes, programs, function modules, interfaces, CDS views, and more
- Code resides in **transportable packages** (not $TMP), requiring transport-aware analysis
- ZADT_VSP already deployed on the managed system

### Key Findings

1. **The AI-assisted migration market is maturing rapidly** - 8+ vendors now offer AI-powered custom code migration, with SAP's own Joule CCM becoming the free baseline in 2026
2. **VSP is uniquely positioned** as an analyze-remediate-and-execute platform with 122 tools, compared to competitors that only analyze and recommend
3. **Three viable approaches** identified, from immediate demonstration (zero development) to full migration agent capability
4. **Industry data suggests ~40% of custom code may be unused** (smartShift/Forrester) - SCMON analysis should precede any remediation work
5. **Dual-server architecture** (VSP + odata_mcp_go for CCLM/BW) provides the most comprehensive analysis picture

### Recommended Approach

A phased strategy combining all three approaches:
- **Week 1:** Demonstrate VSP capabilities with existing tools (Approach 1)
- **Weeks 1-2:** Set up odata_mcp_go for CCLM/BW data access (Approach 2)
- **Weeks 2-3:** Execute full dual-server analysis
- **Weeks 3-5:** Build reusable migration tools if appetite exists (Approach 3)

---

## 2. Competitive Landscape

### Market Overview

The SAP S/4HANA custom code migration market is experiencing rapid transformation driven by AI and agentic automation. With SAP's mainstream ECC maintenance ending in 2027, urgency is rising - only ~45% of SAP customers are live on S/4HANA. The market is splitting into two camps:

**(a) Analyze & Recommend** - Platforms that assess code and provide reports (Panaya, KTern.AI)
**(b) Analyze, Remediate & Execute** - Platforms that assess, fix, AND deploy code changes (NOVA, Adri, smartShift, AWS ABAP Accelerator)

VSP sits firmly in category (b) with 122 tools covering read, analyze, AND write operations.

### Key Players

| Player | Type | Approach | Pricing | Maturity |
|--------|------|----------|---------|----------|
| **NOVA Intelligence** | AI Startup | 3 AI agents: Code Intelligence, Fit-to-Standard, AI Build | Enterprise (undisclosed) | Early (2024-2025) |
| **Adri AI** | AI Startup | Research Agent + Code Agent with self-healing loop | Free tier + Enterprise | Early (2022, seed) |
| **smartShift** | Automation | Rules-based ABAP transformation at scale | Fixed price/timeline | Mature (Forrester-validated) |
| **Panaya/Seemore** | Change Intel | Agentic AI for code corrections + test automation | SaaS (undisclosed) | Mature |
| **KTern.AI** | DXaaS | Process mining + code + testing platform | Freemium | Established |
| **AWS ABAP Accelerator** | MCP Server | Open-source MCP server for Amazon Q Developer | Free (open source) | New (2025) |
| **SAP Joule CCM** | Native | AI copilot in ADT/VS Code + CCM app | Free until Sep 2026 | Evolving (2026 flagship) |
| **Applexus CeleRITE** | Platform | Unified code + config + data migration | Undisclosed | Established |
| **IBM ICAMS** | Enterprise | AI-powered SAP application management | Enterprise | New (Dec 2025) |

### Detailed Competitor Analysis

#### NOVA Intelligence
- **Backing:** SAP.io Fund, Accel, Conviction VC. Co-inventor of SAP HANA (Prof. Dr. Alexander Zeier) on founding team
- **Partnership:** Kyndryl strategic collaboration (Aug 2025) for SAP modernization
- **Capabilities:** Auto-generated documentation, definitive inventory, clean-core output in ABAP Cloud/BTP CAP
- **Claimed results:** 33,000+ lines transformed in 10 days, 75% productivity boost, 50% lower transformation costs
- **Security:** SOC 2 Type II, ISO 27001 certified

#### Adri AI
- **Backing:** Y Combinator, GitHub Accelerator. US patent for SAP-specific AI technology
- **Capabilities:** 50M+ object indexing, searches for standard replacements before writing code, self-healing code loop (write → activate → diagnose → fix → retry)
- **Differentiation:** ChromaSQL proprietary query language, fit-to-standard-first methodology
- **User base:** 3,000+ users, 500M+ tokens monthly

#### AWS ABAP Accelerator (Most Architecturally Similar)
- **Architecture:** MCP server connecting to SAP ADT - same approach as VSP
- **Capabilities:** System-aware code generation, ATC validation with custom variants, automated remediation loops
- **Differentiation from VSP:** Fewer tools (~20-30 vs 122), no OData integration, Amazon Q-specific
- **Pricing:** Free, open-source Docker image

#### SAP Joule CCM (2026 Baseline)
- **Direction:** AI-driven Custom Code Migration is the 2026 flagship feature
- **Capabilities:** Code adaptation proposals, clean-core recommendations, fit-to-standard analysis
- **Availability:** Free promotional period until September 2026
- **Implication:** Will become the baseline every SAP customer has. Third-party tools must differentiate beyond what Joule offers

### VSP Competitive Position

| Capability | VSP | NOVA | Adri | AWS Accel. | Joule CCM |
|-----------|-----|------|------|------------|-----------|
| Tool count | 122 | ~10 agents | ~5 agents | ~25 | Embedded |
| Code reading | All ABAP types | Yes | Yes | Yes | Yes |
| Code writing | Full CRUD | AI Build | Self-healing | Yes | Code gen |
| ATC integration | Direct | Via SAP | Via SAP | Direct | Native |
| Call graph analysis | Yes | Yes | Partial | No | No |
| CDS dependency analysis | Yes | No | No | No | Partial |
| Batch operations | DSL/YAML | Agentic | Agentic | Partial | No |
| Usage data (SCMON) | Via OData | Via SAP | Via SAP | No | Partial |
| Transport management | 5 tools | Via partners | Yes | Yes | Partial |
| OData integration | odata_mcp_go | No | No | No | No |
| Report execution | RunReport/Async | No | No | No | No |
| Safety controls | Comprehensive | Enterprise | Outbound-only | Basic | Sandboxed |
| Pricing | Open source | Enterprise | Free tier + Enterprise | Free | Free (2026) |

**VSP Unique Differentiators:**
1. Deepest ADT tool coverage (122 tools vs next-best ~25)
2. OData bridge via odata_mcp_go (CCLM/BW data access)
3. Comprehensive safety system (read-only, package restrictions, operation filtering)
4. DSL/YAML workflow automation
5. Report execution capability (ZADT_VSP)
6. LLM-agnostic (works with Claude, any MCP client)

---

## 3. Technical Architecture

### Current Capabilities Relevant to Migration

VSP provides a comprehensive toolset across 15 functional domains:

| Domain | Key Tools | Migration Use |
|--------|-----------|---------------|
| **Discovery** | SearchObject, GrepPackages, GrepObjects | Inventory compilation |
| **Quality** | RunATCCheck (with variant support) | S/4HANA readiness checks |
| **Dependencies** | GetCallGraph, GetCallersOf, GetCalleesOf | Impact analysis |
| **CDS** | GetCDSDependencies (cycle detection) | CDS migration analysis |
| **Source** | GetSource (all types), GetClass, GetProgram | Code review by AI |
| **Intelligence** | FindDefinition, FindReferences | Deprecated API scoping |
| **Reports** | RunReport, RunReportAsync | Batch analysis execution |
| **System** | GetSystemInfo, GetInstalledComponents | Landscape profiling |
| **Runtime** | GetDumps, ListTraces, GetSQLTraceState | Error/perf analysis |
| **Export** | GitExport (158 object types) | Code backup |
| **Modify** | WriteSource, EditSource, SyntaxCheck, Activate | Apply fixes |
| **Test** | RunUnitTests, DSL test orchestration | Validation |
| **Transport** | 5 transport tools with safety controls | Change management |
| **Safety** | Read-only, package restrictions, op filtering | Governance |
| **Workflow** | YAML engine, fluent API, batch ops | Automation |

### Dual-Server Architecture (Recommended)

```
                    ┌─────────────────────────┐
                    │     Claude (LLM)        │
                    │  Migration Orchestrator  │
                    └──────┬──────────┬───────┘
                           │          │
              MCP Protocol │          │ MCP Protocol
                           │          │
                    ┌──────▼──┐    ┌──▼────────────┐
                    │   VSP   │    │ odata_mcp_go   │
                    │  (ADT)  │    │   (OData)      │
                    │ 122 tools│    │  Generic V2/V4 │
                    └──────┬──┘    └──┬─────────────┘
                           │          │
                    ┌──────▼──┐    ┌──▼───────────────┐
                    │   SAP   │    │ SAP Solution Mgr  │
                    │ Managed │    │ CCLM / BW Cubes   │
                    │ System  │    │ (Usage + Quality)  │
                    └─────────┘    └───────────────────┘
```

**VSP** handles:
- Code-level analysis (source reading, ATC checks, dependency graphs)
- Pattern matching (deprecated API searches)
- Code modification (remediation phase)

**odata_mcp_go** handles:
- CCLM usage statistics (SCMON consolidated data)
- BW cube queries (historical trends)
- Quality metrics aggregation
- Object ownership/contract metadata

---

## 4. Three Approaches

### Approach 1: VSP-Powered Analysis Pipeline

**Investment:** Zero development | **Timeline:** Immediate | **Risk:** Low

Use VSP's existing 122 tools orchestrated by Claude to perform the analysis. Pure tool orchestration with no code changes.

**Workflow:**
1. **Inventory:** SearchObject("/CBA/*") to discover all objects, GrepPackages for package mapping
2. **ATC:** RunATCCheck per object with S4HANA_READINESS variant (sampled for large footprint)
3. **Deep Analysis:** GetSource for flagged objects, Claude analyzes for S/4HANA issues
4. **Dependencies:** GetCallGraph for high-priority findings to assess blast radius
5. **Report:** Claude generates structured migration recommendations

**Scaling for 5K-20K objects:**
- Inventory phase: Paginated search in batches of 1,000
- ATC phase: Sample top 500-1,000 critical objects first, expand later
- Deep analysis: AI review of top 50-100 highest-priority findings
- Namespace-wide ATC via RunReportAsync("RSATCCHECK") may be possible

**Best for:** Immediate demonstration, proof of concept, initial assessment

### Approach 2: Dual-Server Architecture (VSP + OData MCP for CCLM)

**Investment:** Configuration only | **Timeline:** 1-2 weeks | **Risk:** Medium

Combine VSP (code-level analysis) with odata_mcp_go (CCLM/BW data from Solution Manager). Claude orchestrates both MCP servers.

**Additional data from CCLM/BW:**
- SCMON usage statistics (which code is actually used in production)
- Historical ATC quality trends
- Object ownership and contract metadata
- Decommissioning candidates

**Configuration:**
```json
{
  "mcpServers": {
    "vsp": {
      "command": "./vsp",
      "args": ["--url", "http://managed-system:8000", "--read-only", "--mode", "expert"]
    },
    "solman-cclm": {
      "command": "./odata-mcp",
      "args": [
        "--service", "https://solman-host:port/sap/opu/odata/sap/AI_CCLM_CCLM_SRV/",
        "--read-only", "--claude-code-friendly", "--lazy-metadata"
      ]
    }
  }
}
```

**Prerequisites:**
- Discover CCLM OData service URLs on Solution Manager (`/sap/opu/odata/sap/` discovery)
- Verify SCMON data collection has been active for 3+ months
- Ensure network connectivity from Claude host to both systems

**Best for:** Most comprehensive analysis, leverages existing CCLM investment

### Approach 3: Custom Code Migration Agent (New VSP Tools)

**Investment:** ~15 dev days | **Timeline:** 3-5 weeks | **Risk:** Medium-High

Extend VSP with 6 purpose-built migration analysis tools, creating reusable IP.

**New tools:**
| Tool | Purpose |
|------|---------|
| `AnalyzeCustomCode` | Package-level batch ATC runner |
| `GetPackageInventory` | Full object inventory for package tree |
| `FindDeprecatedAPIs` | Curated pattern search across packages |
| `GetUsageData` | CCLM/BW usage statistics via OData |
| `GenerateMigrationReport` | Composite analysis → structured output |
| `ClassifyObject` | AI-assisted retire/refactor/rewrite/keep classification |

**Best for:** Reusable migration capability, product differentiation, multiple engagements

### Approach Comparison

| Criterion | Approach 1 | Approach 2 | Approach 3 |
|-----------|-----------|-----------|-----------|
| **Time to value** | Immediate | 1-2 weeks | 3-5 weeks |
| **Development effort** | None | Config only | ~15 days |
| **Data completeness** | Code-level only | Code + lifecycle | Code + lifecycle + automated |
| **Usage analysis** | Limited | Full CCLM/BW | Full CCLM/BW |
| **Scalability** | Manual sampling | Better | Best (batch tools) |
| **Reusability** | Session-specific | Config reusable | Fully productized |
| **Risk** | Low | Medium | Medium-High |

---

## 5. Migration Methodology

### Phase 1: Custom Code Inventory Compilation

**Objective:** Build a complete catalog of all `/CBA/` objects with metadata.

**Data collected per object:**
- Object type (CLAS, PROG, FUNC, INTF, DDLS, etc.)
- Object name and description
- Package assignment
- Transport status
- Creation/modification dates (from object history)
- Lines of code (approximation from source)
- Complexity indicators (from call graph depth)

**Expected output:** Structured inventory with 5,000-20,000 entries, categorized by type and package.

### Phase 2: S/4HANA Readiness Analysis

**Objective:** Identify all S/4HANA incompatibilities using ATC checks.

**Check variant:** `S4HANA_READINESS` (or version-specific variant like `S4HANA_READINESS_2023`)

**Finding categories:**
| Priority | Category | Example |
|----------|----------|---------|
| 1 (Error) | Syntax-breaking changes | Removed function modules, changed signatures |
| 2 (Warning) | Semantic changes | Changed behavior of existing APIs |
| 3 (Info) | Recommendations | Performance improvements, clean core alignment |

**Key S/4HANA incompatibilities to scan for:**
| Pattern | Issue | Replacement |
|---------|-------|-------------|
| `SELECT...FROM BSEG` | Cluster table removed | Use ACDOCA (Universal Journal) |
| `SELECT...FROM KONV` | Cluster table removed | Use PRCD_ELEMENTS |
| `CALL FUNCTION 'BAPI_...'` | Deprecated BAPIs | Check SAP Note for replacement |
| `SELECT...FOR ALL ENTRIES` | Performance concern | Review for HANA optimization |
| `MODIFY SCREEN` | Classic dynpro | Fiori compatibility review |
| `CALL TRANSACTION` | Transaction-based flow | Check for Fiori app equivalent |
| `TABLES` parameter | Obsolete typing | Use TYPE TABLE OF |
| `AUTHORITY-CHECK` | Removed auth objects | Map to new auth model |

### Phase 3: Usage Analysis

**Objective:** Determine which code is actively used, dormant, or dead.

**Data source:** SCMON usage statistics consolidated in Solution Manager BW cubes/CCLM (accessed via odata_mcp_go).

**Classification criteria:**
| Category | Definition | Action |
|----------|-----------|--------|
| **Active** | Executed in production within last 3 months | Must remediate |
| **Dormant** | Not executed in 3-12 months | Review for retirement |
| **Dead** | Not executed in 12+ months | Candidate for retirement |
| **Infrastructure** | Framework/utility code called by active objects | Must remediate (indirect usage) |

**Industry benchmark:** ~40% of custom code is typically unused (smartShift/Forrester data).

### Phase 4: Deep Analysis & Classification

**Objective:** AI-assisted classification of each object into migration categories.

**Categories:**
| Action | Criteria | Effort |
|--------|----------|--------|
| **Retire** | Dead code, no production usage | Low (delete + transport) |
| **Keep** | No ATC findings, S/4HANA compatible | None |
| **Refactor** | Minor ATC findings, API replacements available | Medium (1-5 days per object) |
| **Rewrite** | Major incompatibilities, architectural changes needed | High (5-20 days per object) |

**Scoring formula:**
```
Migration Priority = (ATC Severity × 3) + (Usage Frequency × 2) + (Dependency Count × 1)
```

### Phase 5: Migration Plan Generation

**Objective:** Produce an actionable, prioritized remediation sequence.

**Remediation ordering rules:**
1. Dependencies first (leaf nodes before callers)
2. High-usage objects before low-usage
3. High-severity findings before low-severity
4. Package-by-package for transport efficiency

---

## 6. VSP Capability Demonstration

### Value Proposition

| Activity | Manual Approach | VSP-Powered Approach | Savings |
|----------|-----------------|----------------------|---------|
| Object inventory | SE16N/TADIR query + Excel | SearchObject + automated cataloging | Hours → minutes |
| ATC analysis | ATC transaction per object | RunATCCheck in batch | Days → hours |
| Code review | Manual reading in SE80 | GetSource + AI analysis | Weeks → days |
| Dependency mapping | SE80 where-used + manual tracing | GetCallGraph + AnalyzeCallGraph | Days → minutes |
| Pattern search | SE38/ABAP editor search | GrepPackages regex across all code | Hours → seconds |
| Usage analysis | SCMON + manual extraction | odata_mcp_go + CCLM OData | Days → minutes |

### Tools Used in This Analysis

| Phase | VSP Tools | odata_mcp_go Tools |
|-------|-----------|-------------------|
| Inventory | SearchObject, GrepPackages, GetObjectStructure | - |
| ATC | RunATCCheck, GetATCCustomizing | - |
| Dependencies | GetCallGraph, GetCallersOf, GetCalleesOf, GetCDSDependencies | - |
| Code Review | GetSource, FindReferences, GrepPackages | - |
| Usage | - | filter_UsageStatistics, filter_QualityResults |
| System | GetSystemInfo, GetInstalledComponents, GetFeatures | - |
| Runtime | GetDumps, ListTraces | - |
| Safety | Read-only mode, AllowedOps: "RSQTI" | --read-only |

---

## 7. Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|-----------|
| ATC check variant not available | Cannot run S/4HANA readiness checks | Verify variant availability; may need Central Check System setup |
| CCLM OData services not exposed | No usage data via odata_mcp_go | Fall back to BW OData or manual SCMON extraction |
| SCMON not running long enough | Incomplete usage data | Verify 3+ months of data; supplement with call graph analysis |
| Large object count causes timeout | ATC analysis incomplete | Sampling strategy; namespace-wide ATC via report execution |
| Registered namespace transport complexity | Remediation requires transport requests | Use VSP `--allow-transportable-edits` with transport safety controls |

---

## 8. Things You May Not Be Thinking About

1. **SAP Joule CCM is free until Sep 2026** - Consider complementing VSP with Joule, not competing. VSP handles batch automation, custom workflows, and OData integration that Joule cannot.

2. **AWS ABAP Accelerator exists** - Direct MCP competitor, free and AWS-backed. VSP differentiates with 122 tools vs ~25, plus odata_mcp_go.

3. **~40% of code may be unused** - Run SCMON/CCLM analysis BEFORE remediation to potentially halve the migration scope.

4. **Remote ATC requires remote stubs** - S4HANA_READINESS variant needs a Central Check System (NW 7.52+) with remote stubs installed.

5. **Simplification Items are release-specific** - Match the ATC check variant to your target S/4HANA version.

6. **Clean Core is the direction** - Focused Run 5.2 on S/4HANA means aiming for released APIs only.

7. **`/CBA/` namespace means transport-aware analysis** - All modifications need transport requests; VSP safety controls should whitelist `/CBA/*` packages.

8. **Namespace-wide ATC may be possible** - Running ATC at package level via `RunReportAsync("RSATCCHECK")` could dramatically reduce analysis time for 5K-20K objects.

9. **Unit test coverage is likely minimal** - Legacy ABAP typically has near-zero unit tests. VSP's RunUnitTests can assess this gap.

10. **Focused Run vs SolMan CCLM** - Focused Run is monitoring-focused; verify which CCLM functions are available vs needing Solution Manager.

11. **BTP ABAP CCM app coming** - S/4HANA 2025 FPS01 will have a standalone Custom Code Migration app on BTP ABAP.

12. **SAP-samples/abap-platform-ccm-workshops** - Official SAP CCM workshop materials to align methodology with.

---

## 9. Appendices

### A. Sources

- [NOVA Intelligence](https://www.novaintelligence.com) - AI agents for SAP code transformation
- [Adri AI](https://www.getadri.ai/) - Research + Code agents for SAP
- [smartShift Forrester TEI Study](https://smartshift.com/forresters-total-economic-impact-of-smartshift/) - 253% ROI, 40% unused code elimination
- [Kyndryl + Nova Intelligence Partnership](https://www.kyndryl.com/us/en/about-us/news/2025/08/next-gen-sap-innovation-nova-intelligence)
- [AWS ABAP Accelerator](https://github.com/aws-solutions-library-samples/guidance-for-deploying-sap-abap-accelerator-for-amazon-q-developer)
- [SAP Joule for Developers](https://www.sap.com/products/artificial-intelligence/joule-for-developers.html)
- [ABAP AI with Joule, VS Code, and CCM (2025-2026)](https://community.sap.com/t5/technology-blog-posts-by-sap/2025-set-the-pace-2026-wins-the-race-abap-ai-with-joule-vs-code-and-ccm/ba-p/14302433)
- [Custom Code Migration Guide for S/4HANA](https://help.sap.com/doc/9dcbc5e47ba54a5cbb509afaa49dd5a1/2025.000/en-US/CustomCodeMigration_EndToEnd.pdf)
- [S/4HANA Custom Code Impact Analysis Using ATC](https://community.sap.com/t5/enterprise-resource-planning-blog-posts-by-members/s-4hana-custom-code-impact-analysis-using-atc/ba-p/13566164)
- [CCLM Configuration in Solution Manager 7.2](https://community.sap.com/t5/technology-blog-posts-by-members/cclm-custom-code-life-cycle-management-configuration-in-solution-manager-7/ba-p/13362118)
- [Managing Custom Code - Cloud ALM or Solution Manager?](https://community.sap.com/t5/enterprise-resource-planning-blog-posts-by-sap/managing-custom-code-sap-cloud-alm-or-sap-solution-manager/ba-p/13524454)
- [SAP CCM Workshops](https://github.com/SAP-samples/abap-platform-ccm-workshops)
- [Panaya Seemore AI](https://www.panaya.com/agentic-layer/)
- [KTern.AI Custom Code Migration](https://ktern.com/article/sap-custom-code-migration-guide-2024/)
- [Applexus CeleRITE](https://www.applexus.com/celerite)
- [IBM ICAMS](https://www.ibm.com/new/announcements/ibm-consulting-application-management-suite-driving-intelligent-sap-transformation)

### B. Glossary

| Term | Definition |
|------|-----------|
| **ADT** | ABAP Development Tools (Eclipse-based IDE) |
| **ATC** | ABAP Test Cockpit (quality check framework) |
| **CCLM** | Custom Code Lifecycle Management (Solution Manager feature) |
| **CCM** | Custom Code Migration (S/4HANA-specific process) |
| **MCP** | Model Context Protocol (AI tool integration standard) |
| **SCMON** | System Code Monitor (usage data collector) |
| **VSP** | Vibing Steampunk (MCP server for SAP ADT) |
| **ZADT_VSP** | ABAP component deployed on managed system for advanced VSP features |
| **Clean Core** | SAP's architecture principle: custom code uses only released APIs |
| **Simplification Items** | SAP's catalog of API/object changes per S/4HANA release |
| **Remote Stubs** | S/4HANA code stubs installed on Central Check System for remote ATC |
