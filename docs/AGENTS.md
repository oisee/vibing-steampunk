# VSP Specialized Agents - Complete Guide

This guide documents the 6 specialized agents for AI-powered ABAP development using the **vsp** (Vibing Steampunk) MCP server.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Agent Reference](#agent-reference)
  - [1. Code Generator](#1-code-generator-code-gen)
  - [2. Debug Orchestrator](#2-debug-orchestrator-debug-orchestrator)
  - [3. Test Generator](#3-test-generator-test-gen)
  - [4. Code Quality Guardian](#4-code-quality-guardian-code-quality)
  - [5. Documentation Generator](#5-documentation-generator-doc-gen)
  - [6. Transport & Deployment Manager](#6-transport--deployment-manager-transport-deploy)
- [Agent Collaboration](#agent-collaboration)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

---

## Overview

VSP agents are **Claude Code skills** that orchestrate multiple MCP tools to accomplish complex ABAP development workflows. Each agent is specialized for a specific domain and follows a structured, step-by-step approach.

### What Are Agents?

Agents are high-level automation scripts that:
- Orchestrate multiple MCP tool calls
- Follow best practices automatically
- Track progress with TodoWrite
- Provide comprehensive reporting
- Handle errors gracefully
- Generate actionable recommendations

### When to Use Agents

| Scenario | Use Agent | Instead Of |
|----------|-----------|------------|
| Creating new ABAP classes | `/code-gen` | Manual WriteSource + Activate |
| Investigating production crashes | `/debug-orchestrator` | Manual GetDumps + GetSource |
| Generating unit tests | `/test-gen` | Manual test class writing |
| Code quality audit | `/code-quality` | Manual ATC checks + fixes |
| Generating documentation | `/doc-gen` | Manual README writing |
| Preparing deployment | `/transport-deploy` | Manual transport management |

---

## Quick Start

### Installation

Agents are installed automatically as part of vsp in the [`.claude/commands/`](/.claude/commands/) directory:

```
.claude/commands/
├── code-gen.md
├── debug-orchestrator.md
├── test-gen.md
├── code-quality.md
├── doc-gen.md
└── transport-deploy.md
```

### Invocation

Agents are invoked using Claude Code slash commands:

```
/code-gen
/debug-orchestrator
/test-gen
/code-quality
/doc-gen
/transport-deploy
```

### Basic Usage

**Step 1**: Invoke an agent
```
/code-gen
```

**Step 2**: Provide requirements when prompted
```
User: Create a class ZCL_ORDER_VALIDATOR with method CHECK_ORDER
```

**Step 3**: Review and approve actions
- Agent shows what it will do
- You approve or modify approach
- Agent executes steps automatically

**Step 4**: Review results
- Agent provides comprehensive summary
- All changes tracked in TodoWrite
- Next steps recommended

---

## Agent Reference

### 1. Code Generator (`/code-gen`)

**Purpose**: Generate ABAP objects from natural language descriptions

**Use Cases**:
- Create classes, programs, interfaces
- Generate CDS views and behavior definitions
- Create DDIC tables with proper structure
- Bootstrap new features quickly

**Example Invocations**:

```
/code-gen
"Create a class ZCL_ORDER_VALIDATOR with method CHECK_ORDER that validates order data"

/code-gen
"Create a CDS view ZORDER_VIEW based on ZTABLE_ORDERS with fields ORDER_ID, CUSTOMER, AMOUNT"

/code-gen
"Create a table ZTABLE_CUSTOMER_CONFIG with fields ID (key), CONFIG_TYPE, CONFIG_VALUE"
```

**What It Does**:
1. Gathers requirements from user
2. Searches for similar objects (pattern learning)
3. Generates complete, syntactically correct code
4. Validates syntax before creation
5. Creates object in SAP system
6. Activates and verifies compilation
7. Generates unit test skeleton (for classes)
8. Provides comprehensive summary

**Supported Object Types**:
- Classes (CLAS)
- Programs (PROG)
- Interfaces (INTF)
- CDS Views (DDLS)
- Behavior Definitions (BDEF)
- Service Definitions (SRVD)
- Tables (TABL)

**Best For**:
- Rapid prototyping
- Creating boilerplate code
- Learning ABAP patterns from existing code
- Generating test objects quickly

---

### 2. Debug Orchestrator (`/debug-orchestrator`)

**Purpose**: Autonomous debugging and root cause analysis

**Use Cases**:
- Investigate production crashes
- Analyze performance bottlenecks
- Root cause analysis for complex issues
- Live debugging with breakpoints

**Example Invocations**:

```
/debug-orchestrator
"Investigate the ZERODIVIDE crash in ZCL_PRICING that happened this morning"

/debug-orchestrator
"Analyze why ZREPORT_SALES is slow - it used to run in 5 seconds, now takes 2 minutes"

/debug-orchestrator
"Debug ZCL_ORDER_PROCESSOR->PROCESS_ORDER when order_id is 12345"
```

**What It Does**:
1. Retrieves dumps, traces, or sets up live debugging
2. Analyzes stack traces and variable values
3. Reads source code at failure points
4. Builds call graphs for context
5. Searches for similar patterns in codebase
6. Sets breakpoints and captures state (if live)
7. Generates comprehensive RCA report
8. Proposes fix with test case
9. Applies fix if approved

**Analysis Types**:
- **Short Dumps**: Exception analysis with variable inspection
- **Performance**: Profiler traces, SQL analysis, hot spots
- **Live Debugging**: Breakpoint-based state capture
- **Pattern Analysis**: Search for similar issues across codebase

**Best For**:
- Production issue investigation
- Performance tuning
- Understanding complex execution flows
- Debugging without SAP GUI

---

### 3. Test Generator (`/test-gen`)

**Purpose**: Create comprehensive unit test coverage

**Use Cases**:
- Generate test classes for existing code
- Achieve coverage targets (80%+)
- Create positive and negative test cases
- Generate mock objects for dependencies

**Example Invocations**:

```
/test-gen
"Generate unit tests for ZCL_ORDER_PROCESSOR with 80% coverage"

/test-gen
"Create tests for the VALIDATE_INPUT method only"

/test-gen
"Generate tests for all classes in $ZRAY package"
```

**What It Does**:
1. Reads target class/program source code
2. Extracts metadata (methods, parameters, exceptions)
3. Analyzes dependencies via call graphs
4. Generates test class skeleton
5. Creates positive test cases (happy path)
6. Creates negative test cases (error handling)
7. Generates edge case tests
8. Creates mock objects for dependencies
9. Executes tests and reports coverage

**Test Types Generated**:
- **Positive tests**: Valid inputs, expected outputs
- **Negative tests**: Invalid inputs, exception handling
- **Edge cases**: Empty, null, boundary values, special characters
- **Integration tests**: Real dependencies (risk level DANGEROUS)
- **Performance tests**: Large datasets, time limits

**Best For**:
- Increasing test coverage quickly
- Ensuring error handling is tested
- Creating test templates to customize
- TDD (create tests before implementation)

---

### 4. Code Quality Guardian (`/code-quality`)

**Purpose**: Ensure code quality and compliance

**Use Cases**:
- Security audits (SQL injection, auth checks)
- Performance optimization (nested SELECTs, N+1 queries)
- Code standard enforcement
- Technical debt cleanup

**Example Invocations**:

```
/code-quality
"Run a security audit on $ZPROD package"

/code-quality
"Check ZCL_DATA_ACCESS for security vulnerabilities and performance issues"

/code-quality
"Analyze $ZRAY* and fix all critical issues automatically"
```

**What It Does**:
1. Discovers objects in scope
2. Runs ATC quality checks
3. Searches for anti-patterns (SQL injection, hardcoded values)
4. Identifies security vulnerabilities
5. Checks for deprecated API usage
6. Categorizes findings by severity
7. Proposes specific fixes with code examples
8. Applies approved fixes automatically
9. Generates quality scorecard

**Analysis Categories**:
- **Security**: SQL injection, missing auth checks, data exposure
- **Performance**: Nested SELECTs, SELECT *, inefficient loops
- **Maintainability**: Code complexity, duplication, naming
- **Standards**: Deprecated APIs, old syntax, style violations

**Severity Levels**:
- 🔴 **Critical**: Security vulnerabilities, data corruption risks
- 🟡 **High**: Performance issues, missing error handling
- 🔵 **Medium**: Naming violations, minor optimizations
- ⚪ **Low**: Suggestions, best practice recommendations

**Best For**:
- Pre-deployment quality gates
- Security compliance audits
- Performance troubleshooting
- Code modernization initiatives

---

### 5. Documentation Generator (`/doc-gen`)

**Purpose**: Create comprehensive technical documentation

**Use Cases**:
- Package README files
- API reference documentation
- Architecture guides with UML diagrams
- Developer onboarding guides

**Example Invocations**:

```
/doc-gen
"Generate complete documentation for $ZAPI package"

/doc-gen
"Document ZCL_ORDER_PROCESSOR API with examples"

/doc-gen
"Create architecture guide for $ZRAY with call graphs and UML diagrams"
```

**What It Does**:
1. Discovers all objects in scope
2. Extracts metadata (class info, method signatures)
3. Analyzes dependencies and call graphs
4. Generates package README
5. Creates API reference for each class/interface
6. Generates UML class diagrams (Mermaid)
7. Creates call graph and architecture diagrams
8. Generates code examples
9. Creates navigation index

**Documentation Types**:
- **README**: Overview, getting started, quick examples
- **API Reference**: Complete method documentation with parameters
- **Architecture**: UML diagrams, call graphs, design patterns
- **Data Model**: Tables, CDS views, dependencies
- **Examples**: Practical, runnable code samples

**Output Formats**:
- Markdown (default)
- Mermaid diagrams for visualization
- Organized directory structure (`docs/`)

**Best For**:
- New developer onboarding
- API documentation for consumers
- Architecture decision records
- Knowledge sharing and preservation

---

### 6. Transport & Deployment Manager (`/transport-deploy`)

**Purpose**: Manage transports and coordinate deployments

**Use Cases**:
- Validate readiness for production
- Create properly ordered transports
- Pre-deployment validation
- Rollback planning

**Example Invocations**:

```
/transport-deploy
"Prepare $ZRAY package for production deployment"

/transport-deploy
"Validate transport A4HK900123 before releasing to QA"

/transport-deploy
"Create emergency hotfix transport for ZCL_PRICING_SERVICE"
```

**What It Does**:
1. Discovers objects in deployment scope
2. Validates syntax and activation status
3. Checks dependencies and determines order
4. Runs pre-deployment tests (unit tests, ATC)
5. Organizes objects by priority (tables → CDS → classes)
6. Creates or selects transport request
7. Generates comprehensive deployment report
8. Creates rollback backup (GitExport)
9. Provides step-by-step deployment instructions
10. Tracks post-deployment verification

**Deployment Phases**:
- **Validation**: Syntax, activation, tests, ATC checks
- **Organization**: Dependency ordering, priority levels
- **Documentation**: Deployment report, rollback plan
- **Backup**: GitExport for rollback safety
- **Verification**: Post-deployment smoke tests

**Risk Assessment**:
- Analyzes test coverage, ATC findings, scope
- Assigns risk level (Low/Medium/High)
- Recommends mitigation strategies
- Provides approval checklist

**Best For**:
- Production deployments with confidence
- Emergency hotfixes with validation
- Multi-system deployments (DEV → QA → PROD)
- Ensuring deployment safety with rollback plans

---

## Agent Collaboration

Agents can work together for end-to-end workflows:

### Workflow 1: Complete Feature Development

```
1. /code-gen       → Create new class with methods
2. /test-gen       → Generate comprehensive unit tests
3. /code-quality   → Validate code quality (ATC, security)
4. /doc-gen        → Document the new API
5. /transport-deploy → Deploy to production
```

### Workflow 2: Bug Fix Workflow

```
1. /debug-orchestrator → Investigate production crash
2. /code-gen          → Generate fix (or manual edit)
3. /test-gen          → Add test case for the bug
4. /transport-deploy  → Emergency hotfix deployment
```

### Workflow 3: Code Quality Sprint

```
1. /code-quality    → Identify all issues in package
2. /code-gen        → Generate missing classes (if needed)
3. /test-gen        → Increase test coverage to 90%
4. /doc-gen         → Update documentation
5. /transport-deploy → Deploy improvements
```

### Workflow 4: New Package Development

```
1. /code-gen        → Create package structure
2. /test-gen        → Generate test suite
3. /doc-gen         → Create README and API docs
4. /code-quality    → Validate quality before release
5. /transport-deploy → Deploy to dev/qa/prod
```

---

## Best Practices

### 1. Always Use Agents for Complex Tasks

❌ **Don't**: Manually call 10 MCP tools to create and test a class
✅ **Do**: Use `/code-gen` → creates, validates, tests automatically

### 2. Review Before Approval

Agents show their plan before executing:
- Read the steps
- Ask questions if unclear
- Approve when confident

### 3. Track Progress with TodoWrite

Agents automatically use TodoWrite:
- See what's in progress
- Know what's completed
- Understand what's next

### 4. Combine Agents for Workflows

Don't use agents in isolation:
- `/code-gen` + `/test-gen` + `/code-quality` = Complete feature
- `/debug-orchestrator` + `/transport-deploy` = Bug fix workflow

### 5. Let Agents Learn from Your Codebase

Agents search for similar patterns:
- They learn your coding style
- They follow your conventions
- They adapt to your patterns

### 6. Use Agent Reports for Documentation

Agents generate comprehensive reports:
- Save RCA reports for knowledge base
- Use quality reports for audits
- Share deployment reports with team

### 7. Trust the Validation

Agents validate before acting:
- Syntax checked before creating
- Tests run before deploying
- Quality checked before releasing

---

## Troubleshooting

### Agent Not Found

**Problem**: `/code-gen` command not recognized

**Solution**:
1. Check that skill files exist in `.claude/commands/`
2. Restart Claude Code to reload skills
3. Verify vsp MCP server is connected

### Agent Stuck or Slow

**Problem**: Agent seems frozen during execution

**Solution**:
1. Check if waiting for user input (questions)
2. Look for errors in MCP server logs
3. Check SAP system connectivity (`vsp systems`)

### Agent Produces Errors

**Problem**: Agent fails with "Tool execution failed"

**Solution**:
1. Check SAP system permissions
2. Verify vsp MCP server is running
3. Check object exists and is accessible
4. Review agent error message for specific issue

### Agent Results Not as Expected

**Problem**: Generated code/docs not meeting expectations

**Solution**:
1. Provide more detailed requirements upfront
2. Review and adjust agent's plan before approval
3. Use AskUserQuestion to clarify approach
4. Customize results manually after generation

### Permission Errors

**Problem**: "Authorization check failed"

**Solution**:
1. Verify SAP user has required authorizations
2. Check package permissions ($TMP usually open)
3. For transport operations, verify CTS authorization
4. Contact SAP basis team if needed

---

## Additional Resources

- [vsp README](../README.md) - Complete vsp documentation
- [vsp Tools Reference](../README_TOOLS.md) - All 99 MCP tools
- [CLAUDE.md](../CLAUDE.md) - AI development guidelines
- [Agent Source Files](../.claude/commands/) - Review agent implementations

---

## Feedback

Found an issue or have a suggestion?

- Report issues: [GitHub Issues](https://github.com/oisee/vibing-steampunk/issues)
- Contribute: See [CLAUDE.md](../CLAUDE.md) for development guidelines
- Contact: See project README for contact information

---

Generated by Claude Code on 2026-01-30
