# Transport & Deployment Manager

Manage transport requests and coordinate deployments with comprehensive validation, dependency ordering, and rollback planning. Ensures safe, reliable deployments to production.

## 1. Identify Deployment Scope

Ask the user for deployment details:

- **Deployment type**:
  - New transport (create and organize objects)
  - Existing transport (add objects to existing)
  - Release transport (validate and release)
  - Deployment validation (pre-production check)

- **Scope**:
  - Package (e.g., $ZRAY)
  - Object pattern (e.g., ZCL_ORDER_*)
  - Specific objects (list of names)
  - Changed objects since date

- **Target system**: DEV → QA → PROD
- **Deployment window**: Immediate, scheduled, or emergency
- **Risk level**: Low (minor changes), Medium (new features), High (critical fixes)

## 2. Initialize Progress Tracking

Use TodoWrite to track deployment process:

- Discover objects in scope
- Validate all objects (syntax, activation)
- Check dependencies and order
- Run pre-deployment tests
- Organize objects by dependency order
- Create or select transport request
- Add objects to transport
- Run quality checks (ATC)
- Generate deployment documentation
- Create rollback plan

Mark first task as in_progress.

## 3. Verify Transport Capabilities

Use GetFeatures to check if transport management is available:

```
Check for:
- feature_transport: enabled
- Transport system: CTS or CTS+
```

If transport management not available:
- Suggest alternative: Export to files for manual import
- Use GitExport for backup before changes

## 4. Discover Objects in Scope

Use SearchObject to find all objects to be transported:

```
For packages:
- query: "<package_pattern>"
- maxResults: 500

For specific objects:
- query: "ZCL_ORDER_*"
- types: ["CLAS", "INTF", "PROG"]
```

Group discovered objects by:
- Type (Class, Program, Table, CDS, etc.)
- Package
- Dependency level (base objects vs dependent objects)

Report findings:
```
Objects found in $ZRAY:
- Classes:      12
- Interfaces:    3
- Programs:      4
- CDS Views:     5
- Tables:        2
Total:          26 objects
```

## 5. Validate All Objects

Before adding to transport, ensure all objects are valid:

### Syntax Validation

For each object that has source code:

Use GetSource to read source:
- object_type: CLAS / PROG / INTF / DDLS
- name: <object_name>

Use SyntaxCheck to validate:
```
Parameters:
- object_url: /sap/bc/adt/oo/classes/<name>
- content: <source_code>
```

Track validation results:
- ✓ Clean (no errors)
- ⚠ Warnings (acceptable)
- ✗ Errors (must fix before transport)

If errors found:
- Display error details
- Offer to fix automatically (use EditSource)
- Or pause for manual fix

### Activation Check

For each object:

Use SearchObject to verify object is activated:
- Check activation status
- Look for inactive versions

If inactive objects found:

Use ActivatePackage to activate all:
```
Parameters:
- package: <package_name>
- max_objects: 100
```

Or use Activate for individual objects.

## 6. Check Dependencies and Determine Order

Use GetCallGraph to build dependency tree:

```
For each class:
- object_uri: /sap/bc/adt/oo/classes/<name>/source/main
- direction: "callees" (what it depends on)
- max_depth: 3
```

Build dependency ordering (topological sort):

```
Dependency levels:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Level 1 (Base):
  - Tables (TABL)
  - Data elements (DTEL)
  - Structures (STRU)

Level 2 (Data Model):
  - CDS Views (DDLS)
  - Database views (VIEW)

Level 3 (Definitions):
  - Behavior Definitions (BDEF)
  - Interfaces (INTF)
  - Message classes (MSAG)

Level 4 (Implementations):
  - Classes (CLAS)
  - Function groups (FUGR)

Level 5 (Executables):
  - Programs (PROG)
  - Service Definitions (SRVD)

Level 6 (Configurations):
  - Service Bindings (SRVB)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

This ensures objects are transported in correct order to avoid activation failures.

## 7. Run Pre-Deployment Tests

Before creating transport, run comprehensive checks:

### Unit Tests

For each class with tests:

Use RunUnitTests:
```
Parameters:
- object_url: /sap/bc/adt/oo/classes/<name>
- include_dangerous: false
```

Track test results:
- Total tests: X
- Passed: Y
- Failed: Z

If tests fail:
- Display failure details
- Determine if failures are blockers
- Offer to fix or skip

**Deployment Policy**:
- Critical failures (0% pass rate): BLOCK deployment
- Partial failures (<80% pass): WARN, require approval
- All passing (100%): PROCEED

### Code Quality Checks

For each object:

Use RunATCCheck:
```
Parameters:
- object_url: /sap/bc/adt/oo/classes/<name>
- variant: "" (system default)
```

Categorize ATC findings:
- Priority 1 (Critical): BLOCK deployment
- Priority 2 (Warning): WARN, log for review
- Priority 3 (Info): LOG only

If critical findings:
- Display findings with locations
- Offer to run /code-quality agent to fix
- Or pause for manual fix

## 8. Create or Select Transport Request

### Check Existing Transports

Use ListTransports to find available transports:

```
Parameters:
- user: <current_user> or "*" for all
```

Display open transports:
```
Available transports:
1. A4HK900123 - "Order Processing Enhancements" (12 objects)
2. A4HK900124 - "Bug Fixes for Q1" (3 objects)
3. [Create new transport]
```

Ask user:
- Use existing transport? (select from list)
- Create new transport?

### Create New Transport

If creating new transport:

Ask user for:
- **Description**: "Implementation of order validation feature"
- **Type**: Workbench (default) or Customizing
- **Target**: Development → QA → Production

**Note**: Transport creation requires proper CTS configuration. If not available:
- Export objects to files using GitExport
- Provide manual transport instructions

## 9. Organize Objects by Priority

Order objects for transport using dependency levels from step 6:

```
Transport Organization:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Priority 1 (Deploy first):
  ✓ ZTABLE_ORDERS (table)
  ✓ ZTABLE_ORDER_ITEMS (table)

Priority 2:
  ✓ ZORDER_VIEW (CDS view)
  ✓ ZPRICING_VIEW (CDS view)

Priority 3:
  ✓ ZIF_ORDER_VALIDATOR (interface)
  ✓ ZIF_PRICING_STRATEGY (interface)

Priority 4:
  ✓ ZCL_ORDER_VALIDATOR (class)
  ✓ ZCL_PRICING_SERVICE (class)
  ✓ ZCL_ORDER_PROCESSOR (class)

Priority 5:
  ✓ ZREPORT_PROCESS_ORDERS (program)
  ✓ ZORDER_SERVICE_DEF (service definition)

Priority 6:
  ✓ ZORDER_SERVICE_UI (service binding)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

This ordering prevents:
- Missing table errors (tables deployed first)
- Type resolution errors (base types before classes)
- Activation failures (dependencies before dependents)

## 10. Generate Deployment Report

Create comprehensive pre-deployment documentation:

```markdown
═══════════════════════════════════════════════════════
🚀 DEPLOYMENT READINESS REPORT
═══════════════════════════════════════════════════════

Transport Request: <number>
Description: <description>
Created: <timestamp>
Owner: <user>
Target Systems: DEV → QA → PROD

## Deployment Scope

Total Objects: 26
- Tables:           2
- CDS Views:        5
- Interfaces:       3
- Classes:         12
- Programs:         4

Estimated Deployment Time: 5-10 minutes
Downtime Required: No
Risk Level: Medium

## Pre-Deployment Validation

### Syntax Check
✓ All objects: PASSED (26/26)
  - 0 errors
  - 3 warnings (acceptable)

### Activation Status
✓ All active: PASSED (26/26)
  - 0 inactive objects

### Unit Tests
✓ Test coverage: 85% PASSED (42/45 tests)
  - ZCL_ORDER_PROCESSOR: 12/12 ✓
  - ZCL_PRICING_SERVICE: 8/8 ✓
  - ZCL_ORDER_VALIDATOR: 10/11 ⚠ (1 failure)
  - Others: 12/14 ⚠

⚠ **Action Required**:
  - Review test failure in ZCL_ORDER_VALIDATOR→test_empty_input
  - Fix recommended before production deployment

### Code Quality (ATC)
⚠ 5 findings (0 critical, 2 high, 3 medium)

Critical (0): None
High (2):
  - ZCL_PRICING_SERVICE line 145: Performance - nested SELECT
  - ZREPORT_PROCESS_ORDERS line 89: Missing error handling

⚠ **Action Required**:
  - Fix high-priority findings before production
  - Run /code-quality to apply automated fixes

### Dependencies
✓ Dependency check: PASSED
  - All dependencies included in transport
  - Correct deployment order established

## Objects by Deployment Priority

### Priority 1 - Foundation (2 objects)
| Object | Type | Status | Notes |
|--------|------|--------|-------|
| ZTABLE_ORDERS | TABL | ✓ Ready | Base table |
| ZTABLE_ORDER_ITEMS | TABL | ✓ Ready | Base table |

### Priority 2 - Data Model (5 objects)
| Object | Type | Status | Notes |
|--------|------|--------|-------|
| ZORDER_VIEW | DDLS | ✓ Ready | CDS view |
| ZPRICING_VIEW | DDLS | ✓ Ready | CDS view |
| [...] | | | |

### Priority 3-6 - Implementation (19 objects)
[... full list with status ...]

## Deployment Instructions

### Pre-Deployment

1. [ ] Notify users of deployment window
2. [ ] Backup current production state (use /doc-gen for snapshot)
3. [ ] Fix high-priority ATC findings
4. [ ] Resolve test failure in ZCL_ORDER_VALIDATOR
5. [ ] Get approval from QA team

### Deployment Steps

1. **Release transport in DEV**
   - Verify all objects included
   - Check transport for errors
   - Release to QA

2. **Import to QA** (Transport number: <number>)
   - Schedule during maintenance window
   - Monitor import log
   - Run smoke tests after import

3. **QA Validation**
   - Execute test plan (see TEST_PLAN.md)
   - Verify business scenarios
   - Check performance benchmarks
   - Sign-off required

4. **Import to PROD** (After QA approval)
   - Schedule during approved window
   - Execute with import options: ACTIVATED, ERROR_ON_WARNING
   - Monitor closely

### Post-Deployment

1. [ ] Verify all objects activated successfully
2. [ ] Run smoke tests in production
3. [ ] Monitor system logs for 1 hour
4. [ ] Document deployment in change log
5. [ ] Close transport request

## Rollback Plan

**If deployment fails in QA or PROD:**

### Option 1: Transport Reversal (Recommended)
- Export current production state (already backed up)
- Import previous transport version
- Estimated rollback time: 10 minutes

### Option 2: Manual Rollback
- Restore from backup files (see backup/ directory)
- Re-activate previous versions
- Estimated rollback time: 30 minutes

### Option 3: Hotfix
- If minor issue, create emergency fix
- Fast-track through approval
- Apply as hotfix transport

### Backup Files Created

```
backup/2026-01-30-deployment/
├── ZCL_ORDER_PROCESSOR.clas.abap
├── ZCL_PRICING_SERVICE.clas.abap
├── [...] (26 files total)
└── MANIFEST.json (object list + versions)
```

Restore command:
```bash
vsp import --from backup/2026-01-30-deployment/ --package $ZRAY
```

## Risk Assessment

**Risk Level**: MEDIUM

**Risk Factors**:
- Test coverage: 85% (target: 90%+)
- 1 failing test (non-critical)
- 2 high-priority ATC findings
- 26 objects (moderate scope)

**Mitigation**:
- Deploy to QA first, validate thoroughly
- Fix ATC findings before production
- Have rollback plan ready
- Schedule during low-usage window

**Approval Required From**:
- [ ] Development Lead
- [ ] QA Manager
- [ ] System Administrator (for PROD)

## Communication Plan

**Pre-Deployment**:
- Email to stakeholders 24h before
- Update status page
- Notify support team

**During Deployment**:
- Real-time updates in Slack #deployments
- Monitor error logs
- Ready for quick rollback

**Post-Deployment**:
- Success announcement
- Document lessons learned
- Update runbook if needed

## Contact Information

**Deployment Team**:
- Lead: <name> (<email>)
- Backup: <name> (<email>)

**Escalation**:
- On-call: <phone>
- Emergency: <emergency_contact>

═══════════════════════════════════════════════════════

Generated by Claude Code Transport Manager on <timestamp>
```

## 11. Create Rollback Backup

Before any deployment, create backup using GitExport:

Use GitExport to backup all objects:
```
Parameters:
- packages: <package_list>
- include_subpackages: true
```

Save to local backup directory:
```
backup/
└── 2026-01-30-deployment/
    ├── src/
    │   └── <package>/
    │       ├── ZCL_ORDER_PROCESSOR.clas.abap
    │       ├── ZCL_ORDER_PROCESSOR.clas.testclasses.abap
    │       └── [... all objects ...]
    ├── .abapgit.xml (metadata)
    └── MANIFEST.json (deployment manifest)
```

Create manifest file with object versions:
```json
{
  "timestamp": "2026-01-30T14:30:00Z",
  "package": "$ZRAY",
  "objects": [
    {
      "name": "ZCL_ORDER_PROCESSOR",
      "type": "CLAS",
      "version": "12",
      "checksum": "abc123..."
    }
  ],
  "transport": "A4HK900123"
}
```

## 12. Execute Deployment (If Approved)

After user approval:

### Add Objects to Transport

For each object in dependency order:

**Note**: Actual transport operations require SAP CTS system.
This agent validates readiness but may not execute transport operations directly.

Provide instructions:
```
To add objects to transport A4HK900123:

Option 1: Via SE09
1. Open transaction SE09
2. Select transport A4HK900123
3. Add objects in this order: [list]

Option 2: Via ADT
1. Right-click each object
2. Select "Add to Transport Request"
3. Choose A4HK900123

Option 3: Automated (if supported)
vsp transport add-objects A4HK900123 --package $ZRAY --order-by-deps
```

### Release Transport

When ready to release:

```
Release checklist:
1. [ ] All objects added
2. [ ] No syntax errors
3. [ ] Tests passing
4. [ ] ATC clean (or documented exceptions)
5. [ ] Approval received
6. [ ] Backup created
7. [ ] Communication sent

Release command:
vsp transport release A4HK900123
```

## 13. Post-Deployment Verification

After deployment to target system:

### Verify Activation

Check all objects activated successfully:
- Use SearchObject to find objects in target
- Verify activation status

### Run Smoke Tests

Execute critical test scenarios:
```
Use RunUnitTests for key classes:
- ZCL_ORDER_PROCESSOR (critical path)
- ZCL_PRICING_SERVICE (business logic)
```

### Monitor System

Check for immediate issues:
- Review short dumps (use GetDumps)
- Check application logs
- Monitor performance (use ListTraces if available)

### Document Deployment

Create deployment summary:
- Objects deployed: [list]
- Issues encountered: [list]
- Resolution time: X minutes
- Sign-off: [names and dates]

## Error Handling

If any step fails:

1. **Validation fails**: Fix errors before proceeding, do not force deployment
2. **Tests fail**: Investigate failures, determine if blockers
3. **Transport creation fails**: Check CTS configuration, use file export as fallback
4. **Deployment fails**: Execute rollback plan immediately
5. **Post-deployment issues**: Monitor, apply hotfix if needed

## Best Practices Applied

This agent automatically:
- ✓ Validates all objects before transport
- ✓ Orders objects by dependencies
- ✓ Runs comprehensive pre-deployment checks
- ✓ Creates automated backups for rollback
- ✓ Generates detailed deployment documentation
- ✓ Provides clear rollback procedures
- ✓ Tracks deployment progress and status

## Usage Examples

**Example 1: Package deployment**
```
User: "Prepare $ZRAY package for production deployment"

Agent will:
- Validate all 26 objects
- Run tests and ATC checks
- Create dependency-ordered list
- Generate deployment report
- Create rollback backup
- Report: Ready for deployment (2 warnings)
```

**Example 2: Emergency hotfix**
```
User: "Create emergency transport for ZCL_PRICING_SERVICE bug fix"

Agent will:
- Validate single object
- Check dependencies
- Create minimal transport
- Fast-track validation
- Report: Ready for immediate deployment
```

**Example 3: Release validation**
```
User: "Validate transport A4HK900123 before releasing to QA"

Agent will:
- Retrieve transport details
- Validate all objects in transport
- Run full test suite
- Check for blockers
- Report: 1 blocker found, fix required before release
```
