# vsp DSL & Workflow Guide

This guide covers two ways to automate SAP ADT operations:

1. **YAML Workflows** - Declarative automation via CLI
2. **Go Library** - Programmatic access with fluent API

## Table of Contents

- [YAML Workflows](#yaml-workflows)
  - [Quick Start](#quick-start)
  - [Workflow Structure](#workflow-structure)
  - [Built-in Actions](#built-in-actions)
  - [Variables & Conditions](#variables--conditions)
  - [Example Workflows](#example-workflows)
- [Go Library](#go-library)
  - [Installation](#installation)
  - [Search Builder](#search-builder)
  - [Test Runner](#test-runner)
  - [Batch Operations](#batch-operations)
  - [Pipeline Builder](#pipeline-builder)
  - [Workflow Engine](#workflow-engine)
- [CLI Reference](#cli-reference)

---

## YAML Workflows

vsp can execute automation workflows defined in YAML files. This is ideal for CI/CD pipelines, batch operations, and repeatable tasks.

### Quick Start

```bash
# Run unit tests for all classes in $TMP
vsp workflow test '$TMP'

# Run a YAML workflow file
vsp workflow run my-workflow.yaml

# Dry run (preview without executing)
vsp workflow run my-workflow.yaml --dry-run

# Pass variables
vsp workflow run my-workflow.yaml --var PACKAGE='$ZRAY*' --var TRANSPORT=DEVK900123
```

### Workflow Structure

```yaml
# workflow.yaml
name: my-workflow
description: Optional description

# Variables (can use ${VAR} or environment variables)
variables:
  PACKAGE: "$TMP"
  MAX_RESULTS: "100"

# Steps execute sequentially
steps:
  - name: step-name        # Optional, auto-generated if omitted
    action: search         # Required: action type
    parameters:            # Action-specific parameters
      query: "${PACKAGE}/*"
      maxResults: 100
    saveAs: results        # Save output to variable
    condition: "exists:X"  # Skip if condition false
    onFailure: continue    # continue | fail | skip
```

### Built-in Actions

#### `search` - Find ABAP Objects

```yaml
- action: search
  parameters:
    query: "ZCL_*"              # Search pattern (supports *)
    types: [CLAS, PROG, FUNC]   # Filter by object type
    package: "$TMP"             # Filter by package
    packages: ["$TMP", "$ZRAY*"] # Multiple packages
    maxResults: 100             # Limit results
  saveAs: objects
```

#### `test` - Run Unit Tests

```yaml
- action: test
  parameters:
    objects: myObjects      # Variable containing objects
    # OR
    class: ZCL_MY_CLASS     # Single class
    # OR
    package: "$TMP"         # All testable objects in package

    dangerous: false        # Include dangerous risk level
    long: false            # Include long duration tests
    stopOnFirstFailure: false
  saveAs: testResults
```

#### `syntax_check` - Validate Syntax

```yaml
- action: syntax_check
  parameters:
    objects: myObjects     # Variable containing objects
  saveAs: syntaxResults
```

#### `activate` - Activate Objects

```yaml
- action: activate
  parameters:
    objects: myObjects     # Variable containing objects
```

#### `fail_if` - Conditional Failure

```yaml
# Fail if tests failed
- action: fail_if
  parameters:
    condition: "tests_failed:testResults"
    message: "Unit tests failed!"

# Fail if syntax errors
- action: fail_if
  parameters:
    condition: "syntax_errors:syntaxResults"
    message: "Syntax errors found"
```

#### `print` - Log Message

```yaml
- action: print
  parameters:
    message: "Processing complete!"
```

### Variables & Conditions

#### Variable Expansion

```yaml
variables:
  PACKAGE: "$TMP"

steps:
  - action: search
    parameters:
      query: "${PACKAGE}/*"    # Expands to "$TMP/*"
```

Environment variables are also expanded:
```yaml
parameters:
  query: "${SAP_PACKAGE:-$TMP}"  # Uses SAP_PACKAGE or defaults to $TMP
```

#### Conditions

Skip steps based on conditions:

```yaml
# Skip if variable exists
- action: test
  condition: "exists:objects"

# Skip if list is empty
- action: test
  condition: "not_empty:objects"

# Always skip (for debugging)
- action: test
  condition: "false"
```

#### Failure Handling

```yaml
# Stop workflow on failure (default)
- action: test
  onFailure: fail

# Continue despite failure
- action: syntax_check
  onFailure: continue

# Skip remaining steps in stage
- action: test
  onFailure: skip
```

### Example Workflows

#### CI/CD Pipeline

```yaml
# ci-pipeline.yaml
name: ci-pipeline
description: Continuous Integration - syntax check and test

variables:
  PACKAGE: "$ZRAY*"

steps:
  - name: discover
    action: search
    parameters:
      query: "${PACKAGE}"
      types: [CLAS]
      maxResults: 200
    saveAs: classes

  - name: syntax-check
    action: syntax_check
    parameters:
      objects: classes
    saveAs: syntaxResults

  - name: fail-on-syntax-errors
    action: fail_if
    parameters:
      condition: "syntax_errors:syntaxResults"
      message: "Build failed: syntax errors detected"

  - name: unit-tests
    action: test
    parameters:
      objects: classes
    saveAs: testResults

  - name: fail-on-test-failures
    action: fail_if
    parameters:
      condition: "tests_failed:testResults"
      message: "Build failed: unit tests failed"

  - name: success
    action: print
    parameters:
      message: "CI pipeline completed successfully!"
```

#### Test Package

```yaml
# test-package.yaml
name: test-package
description: Run all tests in a package

variables:
  PACKAGE: "$TMP"

steps:
  - action: search
    parameters:
      query: "${PACKAGE}/*"
      types: [CLAS, PROG]
    saveAs: objects

  - action: test
    parameters:
      objects: objects
    saveAs: results

  - action: fail_if
    parameters:
      condition: "tests_failed:results"
```

---

## Go Library

The `pkg/dsl` package provides a fluent API for programmatic ADT automation.

### Installation

```go
import (
    "github.com/oisee/vibing-steampunk/pkg/adt"
    "github.com/oisee/vibing-steampunk/pkg/dsl"
)
```

### Search Builder

Find ABAP objects with a fluent interface:

```go
// Create ADT client
client := adt.NewClient(
    "http://host:50000",
    "user",
    "password",
    adt.WithClient("001"),
)

// Basic search
objects, err := dsl.Search(client).
    Query("ZCL_*").
    Execute(ctx)

// With filters
objects, err := dsl.Search(client).
    Query("Z*").
    Classes().                    // Only classes
    InPackage("$TMP").           // In specific package
    MaxResults(100).
    Execute(ctx)

// Multiple types
objects, err := dsl.Search(client).
    Query("Z*").
    Types("CLAS", "PROG", "INTF").
    InPackages("$TMP", "$ZRAY*").
    Execute(ctx)

// With pattern filters
objects, err := dsl.Search(client).
    Query("ZCL_*").
    NameMatches("ZCL_RAY_.*").    // Regex match
    NameContains("TEST").         // Contains substring
    NamePrefix("ZCL_RAY_").       // Starts with
    Exclude(".*_BACKUP$").        // Exclude pattern
    Execute(ctx)

// Custom filter function
objects, err := dsl.Search(client).
    Query("ZCL_*").
    Where(func(obj dsl.ObjectRef) bool {
        return len(obj.Name) > 10
    }).
    Execute(ctx)

// Get single result
obj, err := dsl.Search(client).
    Query("ZCL_MY_CLASS").
    ExecuteOne(ctx)

// Count results
count, err := dsl.Search(client).
    Query("ZCL_*").
    Count(ctx)
```

### Test Runner

Run ABAP Unit tests with detailed control:

```go
// Simple: test a class
summary, err := dsl.Test(client).
    Class("ZCL_MY_CLASS").
    Run(ctx)

// Test multiple objects
summary, err := dsl.Test(client).
    Objects(objects...).
    Run(ctx)

// Test a package
summary, err := dsl.Test(client).
    Package("$TMP").
    Run(ctx)

// With configuration
summary, err := dsl.Test(client).
    Objects(objects...).
    IncludeDangerous().           // Include dangerous tests
    IncludeLong().                // Include long tests
    StopOnFirstFailure().         // Stop on first failure
    Parallel(4).                  // Run 4 tests in parallel
    Timeout(5 * time.Minute).     // Timeout per test
    Run(ctx)

// With callbacks
summary, err := dsl.Test(client).
    Objects(objects...).
    OnStart(func(obj dsl.ObjectRef) {
        fmt.Printf("Testing: %s\n", obj.Name)
    }).
    OnComplete(func(obj dsl.ObjectRef, result dsl.TestResult) {
        fmt.Printf("  %s: %d/%d passed\n",
            obj.Name, result.PassedTests, result.TotalTests)
    }).
    OnError(func(obj dsl.ObjectRef, err error) {
        fmt.Printf("  ERROR: %s: %v\n", obj.Name, err)
    }).
    Run(ctx)

// Process results
fmt.Printf("Objects: %d tested, %d passed, %d failed\n",
    summary.TestedObjects, summary.PassedObjects, summary.FailedObjects)
fmt.Printf("Tests: %d total, %d passed, %d failed\n",
    summary.TotalTests, summary.PassedTests, summary.FailedTests)
fmt.Printf("Time: %v\n", summary.TotalTime)

// Check individual results
for _, result := range summary.Results {
    if !result.Success {
        fmt.Printf("FAILED: %s - %s\n", result.Object.Name, result.Error)
    }
}
```

### Batch Operations

Transform multiple objects:

```go
// Search then transform
search := dsl.Search(client).
    Query("ZCL_*").
    Classes().
    InPackage("$TMP")

batch, err := dsl.Batch(client).
    FromSearch(ctx, search)

// Apply transformation
result, err := batch.
    Transform(func(source string, obj dsl.ObjectRef) (string, error) {
        // Add copyright header if not present
        if strings.Contains(source, "(c) 2025") {
            return source, nil  // No change needed
        }
        header := "*----------------------------------------------------------------------*\n" +
                  "* (c) 2025 My Company\n" +
                  "*----------------------------------------------------------------------*\n"
        return header + source, nil
    }).
    Transport("DEVK900123").
    WithActivation().
    Execute(ctx)

// Convenience methods
result, err := dsl.Batch(client).
    Objects(objects...).
    PrependHeader("* (c) 2025 My Company").
    Transport("DEVK900123").
    Execute(ctx)

result, err := dsl.Batch(client).
    Objects(objects...).
    ReplaceAll("old_text", "new_text").
    Transport("DEVK900123").
    Execute(ctx)

// Dry run (preview changes)
result, err := dsl.Batch(client).
    Objects(objects...).
    Transform(myTransform).
    DryRun().  // Don't actually save
    Execute(ctx)

// With callbacks
result, err := dsl.Batch(client).
    Objects(objects...).
    Transform(myTransform).
    OnStart(func(obj dsl.ObjectRef) {
        fmt.Printf("Processing: %s\n", obj.Name)
    }).
    OnComplete(func(obj dsl.ObjectRef, result dsl.ObjectResult) {
        fmt.Printf("  %s: %s\n", result.Action, obj.Name)
    }).
    Execute(ctx)

// Process results
fmt.Printf("Processed: %d, Success: %d, Failed: %d, Skipped: %d\n",
    result.ProcessedObjects, result.SuccessCount,
    result.FailureCount, result.SkippedCount)
```

### Pipeline Builder

Build multi-stage pipelines programmatically:

```go
// Build a CI pipeline
pipeline := dsl.NewPipeline(client, "ci-pipeline").
    Stage("discover").
        Search("ZCL_*", "classes").
        Print("Found classes to test").
        Then().
    Stage("validate").
        DependsOn("discover").
        SyntaxCheck("classes", "syntaxResults").
        FailIfSyntaxErrors("syntaxResults").
        Then().
    Stage("test").
        DependsOn("validate").
        Test("classes", "testResults").
        FailIfTestsFailed("testResults").
        Then().
    Build()

// Use pre-built pipelines
pipeline := dsl.TestPipeline(client, "$TMP")
pipeline := dsl.CIPipeline(client, "$ZRAY*")
```

### Workflow Engine

Execute YAML workflows programmatically:

```go
// Create workflow engine
engine := dsl.NewWorkflowEngine(client)

// Load from file
workflow, err := engine.LoadWorkflow("workflow.yaml")

// Or parse from string
workflow, err := engine.ParseWorkflow([]byte(yamlContent))

// Execute
result, err := engine.Execute(ctx, workflow)

// With options
result, err := engine.Execute(ctx, workflow,
    dsl.WithDryRun(true),                    // Preview only
    dsl.WithVerbose(true),                   // Verbose output
    dsl.WithVariables(map[string]string{     // Override variables
        "PACKAGE": "$ZRAY*",
        "TRANSPORT": "DEVK900123",
    }),
)

// Check results
if result.Success {
    fmt.Println("Workflow completed successfully")
} else {
    fmt.Printf("Workflow failed: %s\n", result.Error)
}

// Inspect step results
for _, step := range result.StepResults {
    fmt.Printf("[%s] %s: %s\n",
        statusString(step.Success), step.Name, step.Action)
}

// Register custom action handlers
engine.RegisterHandler("my_action", func(ctx *dsl.ExecutionContext, params map[string]interface{}) (interface{}, error) {
    // Custom logic here
    return result, nil
})
```

### Complete Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/oisee/vibing-steampunk/pkg/adt"
    "github.com/oisee/vibing-steampunk/pkg/dsl"
)

func main() {
    // Create client
    client := adt.NewClient(
        "http://vhcala4hci:50000",
        "DEVELOPER",
        "password",
        adt.WithClient("001"),
        adt.WithLanguage("EN"),
    )

    ctx := context.Background()

    // 1. Discover classes
    fmt.Println("Discovering classes...")
    classes, err := dsl.Search(client).
        Query("ZCL_*").
        Classes().
        InPackage("$TMP").
        MaxResults(50).
        Execute(ctx)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Found %d classes\n\n", len(classes))

    // 2. Run tests
    fmt.Println("Running tests...")
    summary, err := dsl.Test(client).
        Objects(classes...).
        Parallel(2).
        OnComplete(func(obj dsl.ObjectRef, result dsl.TestResult) {
            status := "PASS"
            if !result.Success {
                status = "FAIL"
            }
            fmt.Printf("  [%s] %s (%d tests, %v)\n",
                status, obj.Name, result.TotalTests,
                result.ExecutionTime.Round(time.Millisecond))
        }).
        Run(ctx)
    if err != nil {
        log.Fatal(err)
    }

    // 3. Print summary
    fmt.Println("\n=== Summary ===")
    fmt.Printf("Objects: %d tested, %d passed, %d failed\n",
        summary.TestedObjects, summary.PassedObjects, summary.FailedObjects)
    fmt.Printf("Tests: %d total, %d passed, %d failed\n",
        summary.TotalTests, summary.PassedTests, summary.FailedTests)
    fmt.Printf("Time: %v\n", summary.TotalTime.Round(time.Millisecond))

    if summary.FailedTests > 0 {
        fmt.Println("\nFailed tests:")
        for _, r := range summary.Results {
            if !r.Success {
                fmt.Printf("  - %s\n", r.Object.Name)
            }
        }
    }
}
```

---

## CLI Reference

### `vsp workflow run`

Execute a YAML workflow file.

```bash
vsp workflow run <workflow.yaml> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--dry-run` | Preview changes without executing |
| `-v, --verbose` | Verbose output |
| `--var KEY=VALUE` | Set workflow variable (can repeat) |

**Examples:**
```bash
vsp workflow run ci.yaml
vsp workflow run ci.yaml --dry-run
vsp workflow run ci.yaml --var PACKAGE='$TMP' --var TRANSPORT=DEVK900123
```

### `vsp workflow test`

Run unit tests for a package pattern.

```bash
vsp workflow test <package-pattern> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--parallel N` | Number of parallel test executions (default: 1) |
| `--dangerous` | Include dangerous risk level tests |
| `--long` | Include long duration tests |
| `--stop-on-fail` | Stop on first failure |
| `--json` | Output results as JSON |

**Examples:**
```bash
vsp workflow test '$TMP'
vsp workflow test 'ZCL_*' --parallel 4
vsp workflow test '$ZRAY*' --dangerous --long
vsp workflow test '$TMP' --json > results.json
```

---

## Tips & Best Practices

### 1. Use Variables for Flexibility

```yaml
variables:
  PACKAGE: "${SAP_PACKAGE:-$TMP}"  # Environment variable with default
```

### 2. Dry Run First

```bash
vsp workflow run my-workflow.yaml --dry-run
```

### 3. Handle Failures Gracefully

```yaml
- action: syntax_check
  onFailure: continue  # Don't stop on syntax errors
  saveAs: results

- action: fail_if
  condition: "syntax_errors:results"  # Explicit failure check
```

### 4. Use Parallel Testing

```bash
vsp workflow test '$TMP' --parallel 4
```

### 5. JSON Output for CI/CD

```bash
vsp workflow test '$TMP' --json | jq '.failedTests'
```

---

## See Also

- [Example Workflows](https://github.com/oisee/vibing-steampunk/tree/main/examples/workflows)
- [Architecture](architecture.md) - Project architecture
