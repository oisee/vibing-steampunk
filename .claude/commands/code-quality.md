# Code Quality Guardian

Ensure code quality and compliance by running ATC checks, identifying anti-patterns, finding security vulnerabilities, and automatically applying fixes. Prevents technical debt and enforces coding standards.

## 1. Identify Scope

Ask the user what to analyze:

- **Scope type**:
  - Single object (class, program, interface)
  - Package (all objects in package)
  - Package pattern (e.g., $ZRAY*)
  - Changed objects only (recent modifications)

- **Target**: e.g., ZCL_ORDER_PROCESSOR, $ZRAY package
- **Analysis depth**:
  - Quick (ATC only, ~1 minute)
  - Standard (ATC + pattern search, ~5 minutes)
  - Deep (ATC + patterns + dependencies + references, ~15 minutes)
  - Security audit (focus on vulnerabilities)

- **Auto-fix**: Should fixes be applied automatically or require approval?

## 2. Initialize Progress Tracking

Use TodoWrite to track quality analysis:

- Discover objects in scope
- Run ATC quality checks
- Search for anti-patterns
- Identify security vulnerabilities
- Check for deprecated API usage
- Categorize findings by severity
- Propose fixes
- Apply approved fixes
- Generate quality report

Mark first task as in_progress.

## 3. Discover Objects in Scope

### For Single Object

User provided specific object name - proceed directly to analysis.

### For Package

Use SearchObject to find all objects in the package:

```
Parameters:
- query: "<package_pattern>" (e.g., "$ZRAY*")
- maxResults: 500
```

Filter by object type if needed:
- Classes (CLAS/OC)
- Programs (PROG/P)
- Interfaces (INTF/OI)
- Function groups (FUGR/F)

Group objects by type for organized reporting.

### For Changed Objects

Use SearchObject combined with date filters (if available), or:
- Ask user for specific object list
- Focus on recently activated objects
- Check objects in user's recent transport

## 4. Run ATC Quality Checks

For each object in scope, run ATC checks:

Use RunATCCheck to analyze code quality:

```
Parameters:
- object_url: /sap/bc/adt/oo/classes/<name> (or programs/programs/<name>)
- variant: "" (use system default) or specify custom ATC variant
- max_results: 100
```

ATC returns findings with:
- **Priority**: 1=Error (critical), 2=Warning (high), 3=Info (low)
- **Check title**: Name of the rule violated
- **Message**: Description of the issue
- **Location**: File path, line number, column
- **QuickFix**: Whether automatic fix is available

Collect all findings and categorize:
- **Critical** (Priority 1): Must fix before deployment
- **High** (Priority 2): Should fix soon
- **Medium** (Priority 3): Nice to fix
- **Low**: Informational only

## 5. Search for Anti-Patterns

Beyond ATC, search for common anti-patterns using GrepPackages or GrepObjects:

### SQL Injection Risks

Search for dynamic SQL without proper validation:

```
Patterns to search:
- "CONCATENATE.*INTO.*WHERE" → Dynamic WHERE clause
- "&&.*INTO.*SELECT" → String concatenation in SELECT
- "iv_.*INTO.*WHERE" → Input parameter in WHERE without validation
```

Example vulnerable code:
```abap
" DANGEROUS: SQL injection risk
CONCATENATE 'SELECT * FROM users WHERE name = ''' iv_user_input '''' INTO lv_sql.
EXEC SQL.
  EXECUTE IMMEDIATE :lv_sql.
ENDEXEC.
```

### Hardcoded Values

```
Patterns:
- "'[0-9]{3}'" → Hardcoded client numbers
- "'http://" → Hardcoded URLs
- "PASSWORD.*=" → Hardcoded passwords (security issue!)
```

### Missing Error Handling

```
Patterns:
- "CALL FUNCTION.*EXCEPTIONS\\s*\\." → Function call without exception handling
- "SELECT.*WHERE.*iv_" → Database query without TRY-CATCH
```

### Performance Issues

```
Patterns:
- "SELECT.*LOOP.*SELECT" → Nested SELECT (bad performance)
- "SELECT \*.*FOR ALL ENTRIES" → SELECT * in FAE (inefficient)
- "LOOP.*MODIFY.*LOOP" → Nested modification loops
```

### Deprecated API Usage

```
Patterns:
- "CALL METHOD" → Old syntax, use mo_obj->method( ) instead
- "PERFORM.*USING" → Old FORM routines, use methods
- "DATA.*LIKE" → Old TYPE, use TYPE instead
```

## 6. Identify Security Vulnerabilities

Focus on security-specific issues:

### 1. Sensitive Data Exposure

Search for:
- Password handling without encryption
- Personal data (email, phone) in logs
- API keys or tokens in code

```
Patterns:
- "PASSWORD.*=" (especially if not using secure storage)
- "API_KEY.*=" (hardcoded keys)
- "WRITE.*sy-pass" (exposing passwords in logs)
```

### 2. Authorization Checks

```
Pattern violations:
- Missing AUTHORITY-CHECK before sensitive operations
- Authority checks with hard-coded values
- Authority checks in wrong location (after operation)
```

Example vulnerable code:
```abap
" DANGEROUS: No authority check
METHOD delete_customer.
  DELETE FROM customers WHERE id = iv_customer_id.
  " Should have AUTHORITY-CHECK first!
ENDMETHOD.
```

### 3. Cross-Site Scripting (XSS) in UI5/BSP

```
Patterns:
- Direct output of user input without encoding
- JavaScript eval() with user input
- innerHTML with untrusted data
```

## 7. Check for Deprecated API Usage

Use FindReferences to analyze API usage:

Search for common deprecated patterns:
```
- CL_GUI_* classes → Use CL_SALV_* instead
- CALL TRANSACTION → Use BAPIs or OData
- RFC_* function modules → Use released APIs
```

For each deprecated API found:
- Identify modern replacement
- Check if migration is feasible
- Add to recommendations list

## 8. Categorize Findings

Organize all findings by severity and type:

```
Severity Levels:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🔴 CRITICAL (Priority 1)
  - Security vulnerabilities
  - SQL injection risks
  - Data corruption risks
  - Syntax errors preventing activation

🟡 HIGH (Priority 2)
  - Performance issues
  - Missing error handling
  - Deprecated API usage
  - Code smells (complexity, duplication)

🔵 MEDIUM (Priority 3)
  - Naming convention violations
  - Missing documentation
  - Code formatting issues
  - Minor optimizations

⚪ LOW (Informational)
  - Suggestions for improvement
  - Best practice recommendations
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

Create structured finding records:
```
Finding {
  severity: "critical" | "high" | "medium" | "low"
  category: "security" | "performance" | "maintainability" | "style"
  rule: "SQL_INJECTION" | "HARDCODED_VALUE" | "MISSING_ERROR_HANDLING"
  object: "ZCL_ORDER_PROCESSOR"
  location: "method VALIDATE_INPUT, line 42"
  message: "Potential SQL injection: dynamic WHERE clause with user input"
  example: "... WHERE name = " && iv_user_input"
  recommendation: "Use parameterized queries or validate input"
  auto_fix_available: true | false
}
```

## 9. Propose Fixes

For each finding, generate a specific fix recommendation:

### Critical: SQL Injection Fix

**Before (Vulnerable)**:
```abap
DATA(lv_sql) = |SELECT * FROM users WHERE name = '{ iv_name }'|.
EXEC SQL.
  EXECUTE IMMEDIATE :lv_sql.
ENDEXEC.
```

**After (Fixed)**:
```abap
" Use parameterized query
SELECT * FROM users
  INTO TABLE @DATA(lt_users)
  WHERE name = @iv_name.
" Or validate input:
IF iv_name CA ';''''" ".
  RAISE EXCEPTION TYPE cx_invalid_input.
ENDIF.
```

### High: Missing Error Handling

**Before**:
```abap
CALL FUNCTION 'BAPI_CUSTOMER_GETDETAIL'
  EXPORTING
    customerno = iv_customer
  IMPORTING
    customerdetail = ls_detail.
" No exception handling!
```

**After**:
```abap
TRY.
    CALL FUNCTION 'BAPI_CUSTOMER_GETDETAIL'
      EXPORTING
        customerno = iv_customer
      IMPORTING
        customerdetail = ls_detail
      EXCEPTIONS
        not_found = 1
        OTHERS = 2.

    IF sy-subrc <> 0.
      RAISE EXCEPTION TYPE cx_customer_not_found.
    ENDIF.
  CATCH cx_root INTO DATA(lx_error).
    " Handle or re-raise
    RAISE EXCEPTION TYPE cx_processing_error
      EXPORTING previous = lx_error.
ENDTRY.
```

### Medium: Deprecated Syntax

**Before**:
```abap
CALL METHOD lo_object->process_data( iv_input ).
```

**After**:
```abap
lo_object->process_data( iv_input ).
```

### Low: Naming Convention

**Before**:
```abap
DATA: l_customer TYPE string.  " Old naming
```

**After**:
```abap
DATA(lv_customer) TYPE string.  " Modern naming
```

## 10. Apply Fixes (If Approved)

For each fix to be applied:

### Step 1: Show Before/After Diff

Use CompareSource to show the impact (or display manually):
- Original code
- Proposed fix
- Explanation of change

### Step 2: Get Approval

If auto-fix enabled: Apply automatically for low-risk fixes
If manual approval: Ask user to confirm each fix

### Step 3: Apply Fix

Use EditSource for surgical edits:

```
Parameters:
- object_url: /sap/bc/adt/oo/classes/<name>
- method: <method_name> (for method-level edits)
- old_string: <exact_problematic_code>
- new_string: <fixed_code>
```

For multiple fixes in one file:
- Apply fixes one at a time
- Validate syntax after each fix
- If any fix fails, roll back and report

### Step 4: Validate

After applying fix:

Use SyntaxCheck to ensure no syntax errors:
- If errors introduced, roll back the change
- Report syntax validation failure

Use Activate to activate the fixed object:
- Check activation log
- Verify successful activation

### Step 5: Verify Fix

For critical fixes (security, data integrity):

Use RunUnitTests to ensure no regression:
- Run all unit tests for the fixed object
- If tests fail, investigate (fix broke functionality vs test needs update)
- Report test results

## 11. Generate Quality Report

Create comprehensive quality analysis report:

```markdown
═══════════════════════════════════════════════════════
📊 CODE QUALITY ANALYSIS REPORT
═══════════════════════════════════════════════════════

Scope: <package or object list>
Analysis Type: <Quick / Standard / Deep / Security>
Analyzed: <timestamp>
Duration: <X minutes>

## Executive Summary

Objects Analyzed:     X
Findings:             Y (Critical: C, High: H, Medium: M, Low: L)
Fixes Applied:        Z
Quality Score:        XX / 100

## Findings by Severity

### 🔴 CRITICAL (C findings)

#### 1. SQL Injection Risk
- **Object**: ZCL_DATA_ACCESS
- **Location**: method GET_USER_DATA, line 125
- **Issue**: Dynamic WHERE clause with unvalidated user input
- **Risk**: Attacker could extract sensitive data or modify database
- **Status**: ✓ FIXED

**Before**:
```abap
DATA(lv_where) = |name = '{ iv_name }'|.
SELECT * FROM users INTO TABLE @lt_users WHERE (lv_where).
```

**After**:
```abap
" Use parameterized query
SELECT * FROM users INTO TABLE @DATA(lt_users)
  WHERE name = @iv_name.
```

#### 2. Missing Authorization Check
- **Object**: ZCL_ORDER_PROCESSOR
- **Location**: method DELETE_ORDER, line 87
- **Issue**: Sensitive operation without authority check
- **Risk**: Unauthorized users could delete orders
- **Status**: ⚠ MANUAL FIX NEEDED

**Recommendation**:
```abap
METHOD delete_order.
  " Add authority check
  AUTHORITY-CHECK OBJECT 'Z_ORDER'
    ID 'ACTVT' FIELD '06'. " 06 = Delete

  IF sy-subrc <> 0.
    RAISE EXCEPTION TYPE cx_no_authority.
  ENDIF.

  DELETE FROM orders WHERE order_id = iv_order_id.
ENDMETHOD.
```

---

### 🟡 HIGH (H findings)

#### 3. Nested SELECT in Loop (Performance)
- **Object**: ZREPORT_CUSTOMER_ORDERS
- **Location**: line 245-250
- **Issue**: SELECT inside LOOP (N+1 problem)
- **Impact**: 100x slower for large datasets
- **Status**: ✓ FIXED

**Before**:
```abap
LOOP AT lt_customers INTO DATA(ls_customer).
  SELECT SINGLE * FROM orders
    WHERE customer_id = ls_customer-id
    INTO @DATA(ls_order).
ENDLOOP.
```

**After**:
```abap
SELECT * FROM orders
  FOR ALL ENTRIES IN @lt_customers
  WHERE customer_id = @lt_customers-id
  INTO TABLE @DATA(lt_orders).
```

---

### 🔵 MEDIUM (M findings)

[... similar format for medium findings ...]

---

### ⚪ LOW (L findings)

[... similar format for low findings ...]

---

## Fixes Applied

| # | Object | Issue | Severity | Status |
|---|--------|-------|----------|--------|
| 1 | ZCL_DATA_ACCESS | SQL Injection | Critical | ✓ Fixed |
| 2 | ZCL_ORDER_PROCESSOR | No auth check | Critical | ⚠ Manual |
| 3 | ZREPORT_CUSTOMER_ORDERS | Nested SELECT | High | ✓ Fixed |
| 4 | ZCL_PRICING | Hardcoded value | Medium | ✓ Fixed |
| 5 | ZIF_ORDER | Missing doc | Low | ⊘ Skipped |

**Legend**:
- ✓ Fixed automatically
- ⚠ Requires manual intervention
- ⊘ Skipped (low priority)
- ✗ Fix failed (needs investigation)

## Quality Metrics

### Before Analysis
- ATC Findings:        45
- Security Issues:      3
- Performance Issues:   8
- Code Smells:         15

### After Fixes
- ATC Findings:        12 (-73%)
- Security Issues:      0 (-100%) ✓
- Performance Issues:   2 (-75%) ✓
- Code Smells:         10 (-33%)

### Quality Score: 85 / 100

**Score Breakdown**:
- Security:        100 / 100 ✓ (all critical issues resolved)
- Performance:      80 / 100 ↑ (major issues fixed)
- Maintainability:  75 / 100 ↑ (reduced complexity)
- Style:            70 / 100 → (minor issues remain)

## Technical Debt Analysis

**Debt Removed**: ~24 hours of estimated remediation effort
**Debt Remaining**: ~8 hours

**Highest Debt Items**:
1. ZCL_LEGACY_PROCESSOR - 3h (complex refactoring needed)
2. ZREPORT_OLD_FORMAT - 2h (deprecated API usage)
3. ZIF_OLD_INTERFACE - 1.5h (naming convention updates)

## Recommendations

### Immediate Actions (Next Sprint)

1. **Manual Fix Required**: Add authorization check to ZCL_ORDER_PROCESSOR→DELETE_ORDER
2. **Code Review**: Review ZCL_LEGACY_PROCESSOR for refactoring opportunity
3. **Testing**: Add unit tests for fixed methods to prevent regression

### Short Term (Next Quarter)

1. **API Migration**: Replace deprecated CL_GUI_* with CL_SALV_*
2. **Performance Tuning**: Profile remaining database operations
3. **Documentation**: Add method documentation to public interfaces

### Long Term (Continuous Improvement)

1. **Code Standards**: Enforce naming conventions via custom ATC checks
2. **Security Training**: SQL injection prevention workshop
3. **Automated Checks**: Integrate quality checks into CI/CD pipeline
4. **Monitoring**: Set up quality dashboards for continuous tracking

## Prevention Measures

To prevent future issues:

**Development Guidelines**:
- ✓ Always use parameterized SQL queries
- ✓ Add AUTHORITY-CHECK before sensitive operations
- ✓ Validate all external input
- ✓ Use FOR ALL ENTRIES instead of nested SELECTs
- ✓ Follow naming conventions (lv_, lt_, ls_, etc.)
- ✓ Add inline documentation for public methods

**Process Improvements**:
- Run ATC checks before every commit
- Peer code review for all changes
- Automated quality gates in CI/CD
- Regular security audits (quarterly)

═══════════════════════════════════════════════════════
```

Mark all todos as completed.

## Error Handling

If any step fails:

1. **ATC check fails**: Verify object exists, check permissions
2. **Pattern search too slow**: Reduce scope, use more specific patterns
3. **Fix breaks tests**: Roll back fix, analyze test failure, revise fix
4. **Activation fails**: Check syntax, verify dependencies
5. **Authorization issues**: Some fixes may require elevated permissions

## Best Practices Applied

This agent automatically:
- ✓ Prioritizes findings by business impact
- ✓ Focuses on security and data integrity first
- ✓ Provides actionable, specific fix recommendations
- ✓ Validates fixes before and after application
- ✓ Tracks quality metrics and trends
- ✓ Generates comprehensive audit reports
- ✓ Prevents technical debt accumulation

## Usage Examples

**Example 1: Security audit**
```
User: "Run a security audit on $ZPROD package"

Agent will:
- Focus on security vulnerabilities
- Find SQL injection risks, missing auth checks
- Propose and apply fixes
- Report: 3 critical security issues resolved
```

**Example 2: Performance optimization**
```
User: "Check ZREPORT_SALES for performance issues"

Agent will:
- Run ATC checks
- Search for nested SELECTs, SELECT *
- Propose optimizations
- Report: 2 performance issues fixed, 50% faster
```

**Example 3: Package-wide cleanup**
```
User: "Analyze $ZRAY* and fix all critical issues"

Agent will:
- Analyze all objects in $ZRAY packages
- Fix critical and high-priority issues automatically
- Generate comprehensive quality report
- Report: 12 objects improved, quality score 85/100
```
