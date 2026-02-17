# ABAP Code Generator

Generate ABAP objects (classes, programs, interfaces, CDS views, tables) from natural language descriptions. This agent creates production-ready code following SAP naming conventions and best practices.

## 1. Gather Requirements

Ask the user to specify:

- **Object type** (Class / Program / Interface / CDS View / Table / Function Group)
- **Object name** (must start with Z or Y, e.g., ZCL_ORDER_VALIDATOR)
- **Package name** (e.g., $TMP, $ZRAY)
- **Description** (one-line purpose statement)
- **Functionality** (detailed requirements):
  - Methods/functions needed
  - Input/output parameters
  - Business logic description
  - Dependencies or integrations

If the user provides vague requirements, ask clarifying questions about:
- Expected inputs and outputs
- Error handling requirements
- Performance considerations
- Integration points

## 2. Initialize Progress Tracking

Use the TodoWrite tool to create a task list:

- Validate object doesn't exist
- Search for similar objects for patterns
- Generate code following best practices
- Validate syntax
- Create object in SAP system
- Activate object
- Create unit tests (for classes)
- Verify compilation and execution

Mark the first task as in_progress.

## 3. Check if Object Already Exists

Use SearchObject to check if the object name is already taken:

- If object exists, ask user:
  - Overwrite existing object?
  - Choose different name?
  - Modify existing object instead?
- If name is available, proceed

## 4. Find Similar Objects (Pattern Learning)

Search for similar objects in the package or system to learn coding patterns:

```
Use SearchObject with wildcards to find:
- Similar classes (if creating a class)
- Classes with similar functionality
- Standard SAP objects for reference
```

Use GetSource to read 1-2 similar objects and understand:
- Coding style (indentation, naming)
- Common patterns (error handling, logging)
- Method signatures and documentation
- ABAP syntax version (newer vs older)

## 5. Generate Code

Based on requirements and learned patterns, generate complete ABAP code:

### For Classes (CLAS)

```abap
CLASS zcl_<name> DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    " Public methods
    METHODS <method_name>
      IMPORTING
        iv_param TYPE string
      RETURNING
        VALUE(rv_result) TYPE string
      RAISING
        cx_<exception>.

  PROTECTED SECTION.
  PRIVATE SECTION.
    " Private helper methods
ENDCLASS.

CLASS zcl_<name> IMPLEMENTATION.
  METHOD <method_name>.
    " Implementation with:
    " - Input validation
    " - Business logic
    " - Error handling
    " - Proper return values
  ENDMETHOD.
ENDCLASS.
```

### For Programs (PROG)

```abap
*&---------------------------------------------------------------------*
*& Report  Z<NAME>
*&---------------------------------------------------------------------*
*& Description: <purpose>
*&---------------------------------------------------------------------*
REPORT z<name>.

" Selection screen (if needed)
PARAMETERS: p_param TYPE string.

" Main logic
START-OF-SELECTION.
  " Implementation
  PERFORM main_process.

FORM main_process.
  " Processing logic
ENDFORM.
```

### For CDS Views (DDLS)

```abap
@AbapCatalog.sqlViewName: 'Z<VIEWNAME>'
@AbapCatalog.compiler.compareFilter: true
@AccessControl.authorizationCheck: #CHECK
@EndUserText.label: '<Description>'
define view Z<Name>
  as select from <source_table>
{
  key field1,
      field2,
      field3
}
```

### For Tables (TABL)

Use CreateTable tool with JSON field definitions:

```json
{
  "name": "ZTABLE_NAME",
  "description": "Table description",
  "fields": [
    {"name": "MANDT", "type": "CLNT", "key": true},
    {"name": "ID", "type": "CHAR32", "key": true},
    {"name": "VALUE", "type": "STRING"}
  ]
}
```

**Code Quality Checklist:**
- ✓ Follows SAP naming conventions (Z/Y prefix)
- ✓ Includes proper documentation (header comments, method docs)
- ✓ Input validation for all parameters
- ✓ Error handling with exceptions (CX_* classes)
- ✓ No hardcoded values (use parameters/constants)
- ✓ Proper typing (avoid ANY, use specific types)
- ✓ Message handling (use MESSAGE class or CX_* exceptions)

## 6. Validate Syntax

Before creating the object, validate the generated code:

Use SyntaxCheck to validate:
- Check for syntax errors
- Review warnings
- Ensure all types are defined

If syntax errors found:
- Display errors with line numbers
- Fix the code
- Re-validate until clean

## 7. Create Object in SAP System

Use WriteSource to create the object:

```
Parameters:
- object_type: PROG / CLAS / INTF / DDLS / BDEF / SRVD
- name: <object_name>
- source: <generated_code>
- package: <package_name>
- description: <description>
- mode: "create" (fail if exists) or "upsert" (create or update)
```

For tables, use CreateTable instead of WriteSource.

If creation fails:
- Report the error
- Check if name conflicts exist
- Verify package permissions
- Suggest fixes

Mark todo as completed and move to next task.

## 8. Activate Object

Use Activate to activate the created object:

- Activation triggers compilation
- Check activation messages
- Report any activation warnings/errors

If activation fails:
- Display activation log
- Identify problematic code sections
- Fix and re-activate

## 9. Create Unit Tests (For Classes Only)

If the object is a class, generate a test class:

```abap
CLASS ltc_<classname> DEFINITION FINAL FOR TESTING
  DURATION SHORT
  RISK LEVEL HARMLESS.

  PRIVATE SECTION.
    DATA: mo_cut TYPE REF TO zcl_<name>.

    METHODS:
      setup,
      teardown,
      test_<method>_positive FOR TESTING,
      test_<method>_negative FOR TESTING.
ENDCLASS.

CLASS ltc_<classname> IMPLEMENTATION.
  METHOD setup.
    mo_cut = NEW #( ).
  ENDMETHOD.

  METHOD teardown.
    CLEAR mo_cut.
  ENDMETHOD.

  METHOD test_<method>_positive.
    " Test successful execution
    DATA(lv_result) = mo_cut-><method>( iv_param = 'test' ).
    cl_abap_unit_assert=>assert_not_initial( lv_result ).
  ENDMETHOD.

  METHOD test_<method>_negative.
    " Test error handling
    TRY.
        mo_cut-><method>( iv_param = '' ). " Invalid input
        cl_abap_unit_assert=>fail( 'Expected exception not raised' ).
      CATCH cx_root.
        " Expected
    ENDTRY.
  ENDMETHOD.
ENDCLASS.
```

Use WriteSource with method parameter to add tests:
```
Parameters:
- object_type: CLAS
- name: <class_name>
- source: <test_code>
- method: "test_methods" (creates test include)
```

## 10. Execute Unit Tests

Use RunUnitTests to verify the tests execute successfully:

- Run all test methods
- Check pass/fail status
- Report test coverage

If tests fail:
- Display failure details
- Fix implementation or tests
- Re-run until passing

## 11. Generate Summary Report

Create a comprehensive summary:

```
═══════════════════════════════════════════════════════
📦 CODE GENERATION COMPLETE
═══════════════════════════════════════════════════════

Object Created:
  Type:        <object_type>
  Name:        <object_name>
  Package:     <package_name>
  Description: <description>

Validation Results:
  ✓ Syntax check:    Clean
  ✓ Activation:      Successful
  ✓ Unit tests:      X/Y passed (if applicable)

Generated Components:
  - Main source:     <lines> lines
  - Test class:      <lines> lines (if applicable)
  - Documentation:   Inline comments

Code Quality Metrics:
  - Naming convention: ✓ Follows Z/Y pattern
  - Error handling:    ✓ Uses exceptions
  - Documentation:     ✓ Complete
  - Test coverage:     <percentage>% (if applicable)

Next Steps:
  1. Review the generated code in SAP GUI or ADT
  2. Add additional methods if needed
  3. Integrate with other components
  4. Add to transport request for deployment

Object URL: /sap/bc/adt/oo/classes/<name>/source/main
═══════════════════════════════════════════════════════
```

Mark all todos as completed.

## Error Handling Guidelines

If any step fails:

1. **Object name conflict**: Suggest alternative names or offer to modify existing
2. **Syntax errors**: Display errors, fix code, retry
3. **Activation failure**: Review activation log, identify issues, fix and retry
4. **Permission errors**: Verify package access, suggest using $TMP for testing
5. **Test failures**: Analyze failure messages, fix implementation or tests

## Best Practices Applied

This agent automatically:
- ✓ Follows SAP naming conventions
- ✓ Generates complete, production-ready code
- ✓ Includes comprehensive error handling
- ✓ Adds inline documentation
- ✓ Creates unit tests for testability
- ✓ Validates syntax before creation
- ✓ Activates and verifies compilation
- ✓ Provides clear feedback at each step

## Usage Examples

**Example 1: Create a validator class**
```
User: "Create a class ZCL_ORDER_VALIDATOR with a method CHECK_ORDER that validates order data"

Agent will:
- Generate class with CHECK_ORDER method
- Add input validation
- Include error handling with CX_* exceptions
- Create unit tests
- Activate and verify
```

**Example 2: Create a report**
```
User: "Create a report ZLIST_CUSTOMERS that displays customer data with selection screen"

Agent will:
- Generate report with selection screen
- Add data retrieval logic
- Include ALV display
- Activate and verify
```

**Example 3: Create a table**
```
User: "Create a table ZCUSTOMER_CONFIG to store customer configuration data"

Agent will:
- Design table structure with proper keys
- Add MANDT field automatically
- Create delivery class A (Application)
- Activate table
```
