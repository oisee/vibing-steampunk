---
name: test
description: Run ABAP unit tests and ATC checks for a package or class
allowed-tools: Bash, Read, Grep
argument-hint: <package_or_class_name>
---

Run unit tests and code quality checks for the specified ABAP package or class.

## Workflow

1. If no argument provided, ask the user what to test
2. Use **SearchObject** to verify the target exists
3. Run **RunUnitTests** on the target
4. Run **RunATCCheck** on the target
5. Report results clearly:
   - Total tests: passed / failed / skipped
   - ATC findings by priority (P1 = critical, P2 = important, P3 = info)
   - If failures: show the failing test names and error messages
   - If ATC P1/P2 findings: show the finding details with object and line

## Example Usage

```
/vsp:test $TMP
/vsp:test ZCL_INVOICE_PROCESSOR
/vsp:test $ZADT_VSP
```
