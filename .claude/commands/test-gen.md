# ABAP Unit Test Generator

Generate comprehensive unit test coverage for ABAP classes and programs. Creates test classes with positive/negative test cases, edge case handling, and mock objects for dependencies.

## 1. Identify Target Code

Ask the user to specify what needs testing:

- **Target type**:
  - Single class (most common)
  - Multiple classes in a package
  - Program with forms/functions
  - Method-level (specific method only)

- **Target name**: e.g., ZCL_ORDER_PROCESSOR
- **Coverage goal**: 80% (default) or specific percentage
- **Test approach**:
  - Full coverage (all public methods)
  - Specific methods only
  - Critical paths only
  - Edge cases and error handling only

If user provides just a class name, generate complete test coverage for all public methods.

## 2. Initialize Progress Tracking

Use TodoWrite to track test generation:

- Read and analyze target code
- Extract class metadata
- Generate test class skeleton
- For each public method: generate test cases
- Create mock objects for dependencies
- Write test class to SAP
- Execute tests and verify
- Generate coverage report

Mark first task as in_progress.

## 3. Read and Analyze Target Code

Use GetSource to read the class or program:

```
For classes:
- object_type: CLAS
- name: <class_name>

For programs:
- object_type: PROG
- name: <program_name>
```

Analyze the source code to understand:
- Public method signatures (parameters, return types, exceptions)
- Business logic complexity
- Dependencies (other classes, function modules, database tables)
- Existing error handling

## 4. Extract Class Metadata

Use GetClassInfo to get structured class information:

```
Parameters:
- class_name: <target_class>
```

This provides:
- List of all methods (public, protected, private)
- Method signatures with parameters
- Attributes and their types
- Interfaces implemented
- Superclass
- Abstract/final flags

Focus on **public methods** for test generation (they define the API contract).

## 5. Analyze Dependencies

Use GetCallGraph to identify dependencies:

```
Parameters:
- object_uri: /sap/bc/adt/oo/classes/<NAME>/source/main
- direction: "callees" (what this class calls)
- max_depth: 2
```

Identify:
- External dependencies (other classes, FMs)
- Database access (SELECT statements)
- System calls (sy-*, GET TIME, etc.)
- Web service calls

These will need mocking in the tests.

## 6. Generate Test Class Skeleton

Create the basic test class structure:

```abap
"! @testing ZCL_<TARGET_CLASS>
CLASS ltc_<target_class> DEFINITION FINAL FOR TESTING
  DURATION SHORT
  RISK LEVEL HARMLESS.

  PRIVATE SECTION.
    "! Class under test
    DATA mo_cut TYPE REF TO zcl_<target_class>.

    "! Mocks for dependencies
    DATA mo_<dependency>_mock TYPE REF TO <dependency_type>.

    "! Setup - executed before each test
    METHODS setup.

    "! Teardown - executed after each test
    METHODS teardown.

    "! Test methods (one per public method + edge cases)
    METHODS test_<method>_positive FOR TESTING.
    METHODS test_<method>_negative FOR TESTING.
    METHODS test_<method>_edge_case_1 FOR TESTING.
ENDCLASS.
```

**Test Class Naming Convention**:
- `ltc_<classname>` for local test class
- Add `_integration` suffix for integration tests
- Add `_performance` suffix for performance tests

## 7. Generate Test Methods

For each public method in the target class, generate test methods:

### Positive Test Case (Happy Path)

```abap
METHOD test_<method>_positive.
  "! Given: Valid input data
  DATA(lv_input) = 'valid_value'.

  "! When: Method is called with valid input
  DATA(lv_result) = mo_cut-><method>(
    iv_param = lv_input ).

  "! Then: Result should be as expected
  cl_abap_unit_assert=>assert_equals(
    act = lv_result
    exp = 'expected_result'
    msg = 'Method should return correct result for valid input' ).

  cl_abap_unit_assert=>assert_not_initial(
    act = lv_result
    msg = 'Result should not be initial' ).
ENDMETHOD.
```

### Negative Test Case (Error Handling)

```abap
METHOD test_<method>_negative.
  "! Given: Invalid input data
  DATA(lv_invalid_input) = ''.

  "! When/Then: Method should raise exception for invalid input
  TRY.
      mo_cut-><method>( iv_param = lv_invalid_input ).
      cl_abap_unit_assert=>fail(
        msg = 'Expected CX_<EXCEPTION> not raised' ).
    CATCH cx_<exception> INTO DATA(lx_error).
      "! Exception raised as expected
      cl_abap_unit_assert=>assert_bound(
        act = lx_error
        msg = 'Exception should be raised for invalid input' ).
  ENDTRY.
ENDMETHOD.
```

### Edge Case Tests

Generate tests for common edge cases:

```abap
METHOD test_<method>_empty_input.
  "! Test with empty string
ENDMETHOD.

METHOD test_<method>_null_input.
  "! Test with initial/null reference
ENDMETHOD.

METHOD test_<method>_boundary_values.
  "! Test with min/max values
ENDMETHOD.

METHOD test_<method>_special_characters.
  "! Test with special characters in strings
ENDMETHOD.

METHOD test_<method>_large_dataset.
  "! Test with large table (performance check)
ENDMETHOD.
```

## 8. Create Mock Objects

For dependencies identified in step 5, create mock implementations:

### Option 1: Test Double Classes

```abap
"! Mock implementation of dependency
CLASS ltd_<dependency> DEFINITION FOR TESTING.
  PUBLIC SECTION.
    INTERFACES <interface_name>.

    "! Configurable return values
    DATA mv_return_value TYPE <type>.
    DATA mv_should_fail TYPE abap_bool.

    "! Track method calls
    DATA mt_method_calls TYPE <call_log_table>.
ENDCLASS.

CLASS ltd_<dependency> IMPLEMENTATION.
  METHOD <interface_name>~method.
    " Log the call
    APPEND VALUE #( method = '<method>' params = iv_param )
      TO mt_method_calls.

    " Return configured value or raise exception
    IF mv_should_fail = abap_true.
      RAISE EXCEPTION TYPE cx_<exception>.
    ENDIF.

    rv_result = mv_return_value.
  ENDMETHOD.
ENDCLASS.
```

### Option 2: Dependency Injection

Modify setup to inject mocks:

```abap
METHOD setup.
  " Create mock objects
  mo_<dependency>_mock = NEW ltd_<dependency>( ).

  " Configure mock behavior
  mo_<dependency>_mock->mv_return_value = 'test_value'.

  " Inject mock into class under test
  mo_cut = NEW zcl_<target_class>(
    io_<dependency> = mo_<dependency>_mock ).

  " Alternative: Use setter injection if constructor injection not available
  " mo_cut->set_<dependency>( mo_<dependency>_mock ).
ENDMETHOD.
```

### Database Mocking

For database operations, create test data in setup:

```abap
METHOD setup.
  " Create test data
  INSERT <table> FROM TABLE @( VALUE #(
    ( mandt = sy-mandt id = '001' value = 'test1' )
    ( mandt = sy-mandt id = '002' value = 'test2' )
  ) ).
ENDMETHOD.

METHOD teardown.
  " Clean up test data
  DELETE FROM <table> WHERE id LIKE '00%'.
ENDMETHOD.
```

## 9. Implement Test Class

Complete implementation with all methods:

```abap
CLASS ltc_<target_class> IMPLEMENTATION.

  METHOD setup.
    "! Create fresh instance before each test
    mo_cut = NEW zcl_<target_class>( ).

    "! Setup mocks if needed
    " mo_<dependency>_mock = NEW ltd_<dependency>( ).

    "! Setup test data if needed
    " INSERT <table> FROM ...
  ENDMETHOD.

  METHOD teardown.
    "! Cleanup
    CLEAR: mo_cut, mo_<dependency>_mock.

    "! Clean test data from database
    " DELETE FROM <table> WHERE ...
  ENDMETHOD.

  METHOD test_<method>_positive.
    " ... implementation from step 7
  ENDMETHOD.

  METHOD test_<method>_negative.
    " ... implementation from step 7
  ENDMETHOD.

  METHOD test_<method>_edge_cases.
    " ... edge case tests
  ENDMETHOD.

ENDCLASS.
```

## 10. Write Test Class to SAP

Use WriteSource to create the test include:

```
Parameters:
- object_type: CLAS
- name: <target_class_name>
- source: <complete_test_class_code>
- method: "test_methods" (creates testclasses include)
- mode: "upsert" (create or update)
```

This creates the file `zcl_<name>.clas.testclasses.abap` in the class.

If WriteSource fails:
- Check if class exists
- Verify package permissions
- Ensure syntax is correct (use SyntaxCheck first)

## 11. Validate Test Syntax

Before activating, validate syntax:

Use SyntaxCheck on the test class:
- Check for type mismatches
- Verify mock classes are complete
- Ensure all methods are implemented

Fix any syntax errors and re-write.

## 12. Activate Test Class

Use Activate to activate the test class:

```
Parameters:
- object_url: /sap/bc/adt/oo/classes/<name>
- object_name: <class_name>
```

Check activation log for warnings or errors.

## 13. Execute Tests

Use RunUnitTests to run all generated tests:

```
Parameters:
- object_url: /sap/bc/adt/oo/classes/<name>
- include_dangerous: false (default)
- include_long: false (default)
```

Analyze test results:
- Count passed vs failed tests
- Review failure messages
- Identify missing assertions
- Check for false positives

If tests fail:
- Examine failure details
- Fix test code (assertions, test data, mocks)
- Or fix production code if bug found
- Re-run tests until all pass

## 14. Calculate Coverage

Analyze test coverage:

```
Coverage metrics:
- Methods tested: X / Y (aim for 100% of public methods)
- Lines covered: X / Y (aim for 80%+)
- Branches covered: X / Y (conditional logic)
- Edge cases: X identified and tested

Coverage breakdown by method:
  ✓ method1:           95% (19/20 lines)
  ✓ method2:          100% (12/12 lines)
  ⚠ method3:           60% (6/10 lines) - needs more tests
  ✗ private_helper:     0% (not tested - OK for private)
```

If coverage goal not met, ask user:
- Generate additional tests for uncovered code?
- Focus on specific methods?
- Accept current coverage?

## 15. Generate Test Report

Create comprehensive test generation report:

```markdown
═══════════════════════════════════════════════════════
✅ UNIT TEST GENERATION COMPLETE
═══════════════════════════════════════════════════════

Target Class: ZCL_<NAME>
Package:      <package>
Generated:    <timestamp>

## Test Class Generated

File:         ZCL_<NAME>.clas.testclasses.abap
Lines:        <count>
Test Methods: <count>

## Coverage Analysis

Public Methods Tested:     X / Y (XX%)
Total Lines Covered:       X / Y (XX%)
Edge Cases Identified:     X
Mock Objects Created:      X

### Method Coverage

| Method              | Status | Coverage | Tests |
|---------------------|--------|----------|-------|
| validate_input      | ✓      | 95%      | 3     |
| process_order       | ✓      | 88%      | 4     |
| calculate_total     | ⚠      | 65%      | 2     |

⚠ = Below 80% target

## Test Execution Results

Total Tests:    X
✓ Passed:       X
✗ Failed:       0
⊘ Skipped:      0

Execution Time: X.XX seconds

## Test Cases Generated

### validate_input (3 tests)
- ✓ test_validate_input_positive - Valid input accepted
- ✓ test_validate_input_negative - Invalid input rejected
- ✓ test_validate_input_empty - Empty input handled

### process_order (4 tests)
- ✓ test_process_order_positive - Order processed successfully
- ✓ test_process_order_negative - Invalid order rejected
- ✓ test_process_order_no_items - Empty order handled
- ✓ test_process_order_large - Large order handled

### calculate_total (2 tests)
- ✓ test_calculate_total_positive - Calculation correct
- ✓ test_calculate_total_zero - Zero amount handled

## Mocks Created

- **ltd_pricing_service**: Mock for ZCL_PRICING_SERVICE
- **ltd_db_accessor**: Mock for database operations

## Edge Cases Covered

- ✓ Empty/null inputs
- ✓ Boundary values (min/max)
- ✓ Special characters in strings
- ✓ Large datasets (performance)
- ✓ Exception scenarios

## Recommendations

**Increase Coverage**:
- `calculate_total` needs additional branch coverage
- Add test for discount calculation with zero price

**Code Improvements**:
- Consider adding input validation in `process_order`
- Error messages could be more specific

**Maintenance**:
- Update tests when method signatures change
- Keep mocks synchronized with real dependencies
- Review test data setup for realism

## Next Steps

1. [✓] Review generated test code
2. [ ] Add custom test scenarios if needed
3. [ ] Run tests regularly in CI/CD
4. [ ] Update tests when code changes
5. [ ] Aim for 90%+ coverage for critical classes

═══════════════════════════════════════════════════════
```

Mark all todos as completed.

## Advanced Scenarios

### Testing Private Methods

Private methods are tested indirectly through public methods. If direct testing needed:

```abap
"! Use test friend concept
CLASS zcl_<target> DEFINITION LOCAL FRIENDS ltc_<target>.
```

Then access private methods in tests.

### Integration Tests

For integration tests (risk level DANGEROUS):

```abap
CLASS ltc_<target>_integration DEFINITION FINAL FOR TESTING
  DURATION MEDIUM
  RISK LEVEL DANGEROUS.  "! Can modify database

  PRIVATE SECTION.
    METHODS test_end_to_end FOR TESTING.
ENDCLASS.

CLASS ltc_<target>_integration IMPLEMENTATION.
  METHOD test_end_to_end.
    "! Test with real dependencies (no mocks)
    "! Use real database, real function modules
    "! Clean up thoroughly in teardown
  ENDMETHOD.
ENDCLASS.
```

### Performance Tests

For performance-sensitive code:

```abap
CLASS ltc_<target>_performance DEFINITION FINAL FOR TESTING
  DURATION LONG
  RISK LEVEL HARMLESS.

  PRIVATE SECTION.
    METHODS test_<method>_performance FOR TESTING.
ENDCLASS.

CLASS ltc_<target>_performance IMPLEMENTATION.
  METHOD test_<method>_performance.
    "! Given: Large dataset
    DATA(lt_large_data) = VALUE <table_type>(
      FOR i = 1 UNTIL i > 10000 ( value = |Item { i }| ) ).

    "! When: Method processes large dataset
    GET RUN TIME FIELD DATA(lv_start).
    mo_cut-><method>( lt_large_data ).
    GET RUN TIME FIELD DATA(lv_end).

    "! Then: Execution should complete within time limit
    DATA(lv_duration_ms) = lv_end - lv_start.
    cl_abap_unit_assert=>assert_true(
      act = xsdbool( lv_duration_ms < 1000 )  "! Max 1 second
      msg = |Method too slow: { lv_duration_ms }ms| ).
  ENDMETHOD.
ENDCLASS.
```

## Error Handling

If any step fails:

1. **Cannot read target class**: Verify class name and permissions
2. **Syntax errors in generated tests**: Fix assertions, variable types, or method calls
3. **Tests fail unexpectedly**: Review test logic, check test data, verify mocks
4. **Low coverage**: Generate additional tests, focus on uncovered branches
5. **Activation fails**: Check for naming conflicts, verify package

## Best Practices Applied

This agent automatically:
- ✓ Creates complete test classes with setup/teardown
- ✓ Generates positive, negative, and edge case tests
- ✓ Creates mock objects for external dependencies
- ✓ Follows ABAP Unit best practices
- ✓ Uses descriptive test method names
- ✓ Includes assertions with clear messages
- ✓ Provides comprehensive coverage analysis
- ✓ Generates executable, maintainable tests

## Usage Examples

**Example 1: Full class coverage**
```
User: "Generate unit tests for ZCL_ORDER_PROCESSOR with 80% coverage"

Agent will:
- Analyze all public methods
- Generate positive/negative tests for each
- Create mock objects for dependencies
- Achieve 80%+ coverage
- Report: 12 tests generated, 95% coverage
```

**Example 2: Specific method**
```
User: "Generate tests for the VALIDATE_INPUT method"

Agent will:
- Focus on one method only
- Generate comprehensive test cases
- Cover edge cases (empty, null, special chars)
- Report: 5 tests generated
```

**Example 3: Package-wide testing**
```
User: "Generate tests for all classes in $ZRAY package"

Agent will:
- Find all classes in package
- Generate test class for each
- Run all tests
- Report aggregate coverage
```
