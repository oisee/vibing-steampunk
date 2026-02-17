# Debug Orchestrator & Root Cause Analysis

Autonomous debugging agent that investigates production issues, analyzes crashes, traces execution, and provides comprehensive root cause analysis with fix recommendations.

## 1. Gather Incident Details

Ask the user for incident information (or search automatically if details are vague):

- **Issue type**:
  - Short dump / crash (exception type, program name)
  - Performance issue (slow program, transaction)
  - Incorrect behavior (wrong results, logic error)
  - General investigation (explore recent issues)

- **Context** (if known):
  - Dump ID or exception type (e.g., ZERODIVIDE, CX_SY_ARITHMETIC_OVERFLOW)
  - Program name or class (e.g., ZCL_PRICING, ZREPORT_SALES)
  - User who encountered the issue
  - Date/time range
  - Transaction code

If user provides vague information like "something crashed in production", help narrow down:
- When did it happen? (last hour, today, this week)
- Which user reported it?
- What were they trying to do?

## 2. Initialize Progress Tracking

Use TodoWrite to create investigation task list:

- Retrieve incident data (dumps/traces)
- Analyze stack trace and error details
- Read source code at failure points
- Build call graph for context
- Search for similar patterns
- Set breakpoints (if live debugging needed)
- Generate root cause analysis
- Propose fix with test case

Mark first task as in_progress.

## 3. Retrieve Incident Data

### For Short Dumps (Runtime Errors)

Use GetDumps to search for recent crashes:

```
Parameters:
- exception_type: (if known, e.g., "ZERODIVIDE")
- program: (if known)
- user: (if known)
- date_from: YYYYMMDD (default: last 7 days)
- max_results: 20
```

Review the list and identify the most relevant dump(s):
- Sort by date (most recent first)
- Filter by program/exception if specified
- Look for patterns (same error recurring)

Use GetDump with the dump ID to get full details:
- Stack trace (call hierarchy)
- Variable values at crash time
- Exception message
- System information

### For Performance Issues

Use ListTraces to find profiler data:

```
Parameters:
- user: (if known)
- object_type: (if known)
- max_results: 20
```

Use GetTrace to get detailed hitlist:
- Identify hot spots (most time-consuming operations)
- Database access analysis
- Method call frequencies

### For SQL Performance

Use GetSQLTraceState to check if tracing is available.

Use ListSQLTraces to find SQL trace files:
- Filter by user/date
- Look for expensive SQL statements

## 4. Analyze Stack Trace and Error Context

From the dump or trace data, identify:

1. **Failure point**: Exact program, line number, method
2. **Call path**: How execution reached the failure point
3. **Variable values**: What data caused the issue
4. **Error message**: Exception text and parameters

Parse the stack trace to create a narrative:

```
Example analysis:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
CRASH ANALYSIS
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Exception:    CX_SY_ZERODIVIDE
Program:      ZCL_PRICING=>CALCULATE_DISCOUNT
Line:         42
Date/Time:    2026-01-30 14:23:15

Call Stack:
  1. ZREPORT_SALES (line 156)
  2. ZCL_ORDER_PROCESSOR=>PROCESS_ORDER (line 89)
  3. ZCL_PRICING=>CALCULATE_DISCOUNT (line 42) ← CRASH HERE

Variables at crash:
  LV_TOTAL_AMOUNT = 0    ← Problem: Zero value
  LV_DISCOUNT_PCT = 10
  LV_RESULT = ?

Root Cause Hypothesis:
  Division by zero when calculating discount ratio.
  LV_TOTAL_AMOUNT is zero for orders with no line items.
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

## 5. Read Source Code at Failure Points

Use GetSource to read the code where the failure occurred:

```
For classes:
- object_type: CLAS
- name: <class_name>
- method: <method_name> (optional, for method-level reading)

For programs:
- object_type: PROG
- name: <program_name>
```

Focus on:
- The failing line and surrounding context (±10 lines)
- Variable declarations
- Logic flow leading to the error
- Error handling (or lack thereof)

Identify the specific code pattern causing the issue:

```abap
" Example problematic code:
METHOD calculate_discount.
  DATA(lv_ratio) = lv_discount_amount / lv_total_amount.  " ← Line 42: No zero check!
  rv_result = lv_ratio * 100.
ENDMETHOD.
```

## 6. Build Call Graph for Context

Use GetCallGraph or TraceExecution to understand the execution flow:

```
GetCallGraph parameters:
- object_uri: /sap/bc/adt/oo/classes/<NAME>/source/main#start=<line>,1
- direction: "callers" (who calls this method)
- max_depth: 5
```

This helps understand:
- How the failing method is called
- What preconditions should exist
- Where input validation should happen
- Impact of the failure on calling code

For complex scenarios, use TraceExecution:
```
Parameters:
- object_uri: <starting_point>
- run_tests: true (if unit tests available)
- max_depth: 5
```

This provides:
- Static call graph
- Actual execution trace
- Comparison of expected vs actual paths

## 7. Search for Similar Patterns

Use GrepPackages to find similar code patterns across the codebase:

```
Search for:
- Same variable names
- Similar calculations
- Division operations without zero checks
- Exception patterns
```

Example searches:
```
Pattern: "/ lv_total" → Find all divisions using lv_total
Pattern: "CATCH cx_sy_zerodivide" → See how others handle this
Pattern: "lv_amount = 0" → Find zero-checking patterns
```

This identifies:
- Whether this is an isolated issue or systemic
- How other developers handled similar cases
- Existing patterns to follow for the fix

## 8. Live Debugging (If Needed)

If the issue cannot be fully understood from dumps/traces, set up live debugging:

### Set Strategic Breakpoints

Use SetBreakpoint to set breakpoints at key locations:

```
For statement breakpoints (catch all divisions):
- kind: "statement"
- statement: "DIVIDE" or "/"

For line breakpoints:
- program: <program_name>
- line: <line_number>
- method: <method_name> (for classes)

For exception breakpoints:
- kind: "exception"
- exception: "CX_SY_ZERODIVIDE"
```

### Wait for Debuggee

Use DebuggerListen to wait for the breakpoint to be hit:

```
Parameters:
- user: (filter by user, or current user)
- timeout: 60 (seconds)
```

This will block until:
- A breakpoint is hit
- Timeout occurs
- User cancels

### Attach and Inspect

When a debuggee is caught:

Use DebuggerAttach to attach to the session:
```
Parameters:
- debuggee_id: <from DebuggerListen result>
```

Use DebuggerGetStack to view the call stack:
- Understand the execution path
- See which methods called the failing code

Use DebuggerGetVariables to inspect values:
```
Parameters:
- variable_ids: ["@ROOT"] (for top-level variables)
```

Examine:
- Input parameters
- Local variables
- Object attributes

### Step Through Execution

Use DebuggerStep to control execution:
```
Parameters:
- step_type: "stepOver" | "stepInto" | "stepReturn" | "stepContinue"
```

Observe:
- How variables change
- Which branches are taken
- Where the error occurs

Use DebuggerDetach when investigation complete.

## 9. Generate Root Cause Analysis Report

Create a comprehensive RCA document:

```markdown
═══════════════════════════════════════════════════════
🔍 ROOT CAUSE ANALYSIS REPORT
═══════════════════════════════════════════════════════

## Incident Summary

**Issue ID**: <dump_id or trace_id>
**Reported**: <date_time>
**Severity**: Critical / High / Medium / Low
**Status**: Analysis Complete

**Description**: <brief description of the problem>

## Technical Details

**Exception**: <exception_type>
**Program**: <program_name>
**Method**: <method_name>
**Line**: <line_number>

**Call Stack**:
```
1. <caller_1> (line X)
2. <caller_2> (line Y)
3. <failing_method> (line Z) ← FAILURE POINT
```

## Root Cause

**Immediate Cause**:
<What specifically triggered the error>

**Variable State at Failure**:
- `lv_var1` = <value> ← <significance>
- `lv_var2` = <value>

**Code Analysis**:
```abap
" Line 42: Problematic code
DATA(lv_result) = lv_amount / lv_total.  " No zero check
```

**Why It Happened**:
<Explanation of the underlying issue>

**Contributing Factors**:
- Missing input validation
- Incorrect assumption about data
- Edge case not handled

## Impact Analysis

**Affected Users**: <count or description>
**Frequency**: <how often it occurs>
**Business Impact**: <operational impact>
**Data Integrity**: <any data corruption>

## Similar Occurrences

**Pattern Search Results**:
- Found X similar patterns in the codebase
- Y of them have proper error handling
- Z are potential candidates for the same issue

**Recurring Issues**:
<If this same error has happened before>

## Recommended Fix

### Option 1: Add Input Validation (Recommended)

```abap
METHOD calculate_discount.
  " Validate input
  IF lv_total_amount <= 0.
    RAISE EXCEPTION TYPE cx_invalid_input
      EXPORTING
        textid = cx_invalid_input=>zero_or_negative_amount.
  ENDIF.

  DATA(lv_ratio) = lv_discount_amount / lv_total_amount.
  rv_result = lv_ratio * 100.
ENDMETHOD.
```

**Pros**: Prevents error, provides clear message
**Cons**: Requires exception handling in callers

### Option 2: Default Behavior

```abap
METHOD calculate_discount.
  " Use safe division with default
  DATA(lv_ratio) = COND #(
    WHEN lv_total_amount > 0
    THEN lv_discount_amount / lv_total_amount
    ELSE 0 ).
  rv_result = lv_ratio * 100.
ENDMETHOD.
```

**Pros**: No exception handling needed
**Cons**: Silent failure (returns 0)

### Recommended: Option 1
Explicit error handling is better for maintainability

## Test Case

Create this unit test to verify the fix:

```abap
METHOD test_calculate_discount_zero_amount.
  " Test edge case: zero total amount
  TRY.
      DATA(result) = mo_cut->calculate_discount(
        iv_discount_amount = 10
        iv_total_amount = 0 ).
      cl_abap_unit_assert=>fail( 'Expected exception not raised' ).
    CATCH cx_invalid_input.
      " Expected - test passes
  ENDTRY.
ENDMETHOD.
```

## Prevention Measures

**Short Term**:
1. Apply fix to ZCL_PRICING
2. Run unit tests to verify
3. Search for similar patterns and fix
4. Deploy via transport request

**Long Term**:
1. Add code review checklist item: "Check for division without zero validation"
2. Create ATC custom check for division operations
3. Add to coding standards documentation
4. Training: Safe arithmetic operations

## Next Steps

1. [ ] Review and approve recommended fix
2. [ ] Apply fix using EditSource or WriteSource
3. [ ] Create unit test
4. [ ] Run full test suite
5. [ ] Search for similar issues (38 potential locations found)
6. [ ] Create transport request
7. [ ] Schedule deployment
8. [ ] Monitor for recurrence

═══════════════════════════════════════════════════════
```

## 10. Propose and Apply Fix

After user approves the recommended fix:

### Apply the Fix

Use EditSource to make surgical changes:

```
Parameters:
- object_url: /sap/bc/adt/oo/classes/<NAME>
- method: <method_name> (for method-level editing)
- old_string: <exact code to replace>
- new_string: <fixed code>
```

Or use WriteSource to replace the entire method:

```
Parameters:
- object_type: CLAS
- name: <class_name>
- method: <method_name>
- source: <complete fixed method code>
```

### Validate the Fix

Use SyntaxCheck to ensure no syntax errors:
- Review any warnings
- Ensure all variables are declared

Use Activate to activate the changes:
- Check activation log
- Verify successful activation

### Create Test Case

Use WriteSource to add a unit test:

```
Parameters:
- object_type: CLAS
- name: <class_name>
- source: <test method code>
- method: "test_methods" (adds to test include)
```

### Run Tests

Use RunUnitTests to execute all tests:
- Verify the new test passes
- Ensure no regression (existing tests still pass)
- Report test results

## 11. Update Task Progress

Mark all investigation todos as completed.

## Error Handling

If any step fails:

1. **No dumps found**: Expand search criteria (date range, user filter)
2. **Cannot read source**: Check object permissions
3. **Breakpoint not hit**: Verify program is being executed, check terminal ID settings
4. **Debug session conflicts**: Another debugger may be attached
5. **Fix causes new errors**: Roll back and revise approach

## Advanced Scenarios

### Memory Leaks

Use GetTrace with tool_type "hitlist" to identify:
- Methods called repeatedly
- Growing data structures
- Resource allocations without cleanup

### Performance Bottlenecks

Use GetTrace with tool_type "statements":
- Find expensive SQL queries
- Identify loops with high iteration counts
- Locate unnecessary data conversions

Use GetSQLTraceState and ListSQLTraces for database analysis.

### Concurrency Issues

Use TraceExecution to compare multiple runs:
- Look for race conditions
- Identify timing-dependent behavior
- Check for missing locks

## Best Practices

This agent automatically:
- ✓ Provides comprehensive analysis, not just error messages
- ✓ Considers business context and impact
- ✓ Searches for patterns to prevent future issues
- ✓ Recommends testable, maintainable fixes
- ✓ Creates reproducible test cases
- ✓ Tracks the entire investigation process
- ✓ Generates documentation for knowledge sharing

## Usage Examples

**Example 1: Investigate crash**
```
User: "Investigate the ZERODIVIDE crash in production"

Agent will:
- Search dumps for ZERODIVIDE exceptions
- Analyze the most recent occurrence
- Read source code at failure point
- Identify root cause (missing validation)
- Propose fix with test case
- Apply fix if approved
```

**Example 2: Performance investigation**
```
User: "ZREPORT_SALES is very slow, investigate why"

Agent will:
- Search for profiler traces
- Analyze hitlist for bottlenecks
- Identify expensive operations (likely SQL)
- Check SQL traces for database issues
- Recommend optimization (indexes, buffering)
- Propose code changes
```

**Example 3: Live debugging**
```
User: "Debug ZCL_ORDER_PROCESSOR when it processes order 12345"

Agent will:
- Set breakpoint at method entry
- Wait for order 12345 to be processed
- Attach and inspect variables
- Step through logic
- Identify issue
- Recommend fix
```
