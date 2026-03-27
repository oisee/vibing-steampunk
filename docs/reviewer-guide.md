# vsp Reviewer Guide

> A hands-on checklist for anyone who wants to kick the tires.
> No SAP system required for most tasks — 6 of 11 tasks are fully offline.

## Build It (30 seconds)

```bash
git clone https://github.com/oisee/vibing-steampunk.git
cd vibing-steampunk
go build -o vsp ./cmd/vsp
./vsp --version
```

Single binary, zero dependencies beyond Go 1.23+.

---

## Task 1: Read the --help

```bash
./vsp --help
```

**What to spotlight:**
- Two modes: MCP Server (AI agents) + CLI (terminal DevOps)
- 28 CLI commands, 81–122 MCP tools
- Enterprise safety flags (`--read-only`, `--allowed-packages`, `--disallowed-ops`)
- Subcommands: search, source, query, grep, graph, deps, lint, compile, parse, test, atc, deploy, export, system, install...

**Questions:**
- Does the help make clear this is both an MCP server AND a CLI tool?
- Would you know how to get started?

---

## Task 2: Run the Tests (no SAP needed)

```bash
go test ./...
```

250+ unit tests, all pass without any SAP connection.

**Dig deeper:**

```bash
# Safety system (25 tests)
go test -v -run TestSafety ./pkg/adt/

# ABAP lexer — oracle-verified against TypeScript abaplint
go test -v -run TestLexer_OracleDifferential ./pkg/abaplint/

# Statement parser — 100% match on 3,254 statements
go test -v -run TestStatementMatcher_OracleDifferential ./pkg/abaplint/

# ABAP linter — 100% match on 4 oracle-verified rules
go test -v -run TestLinter_OracleDifferential ./pkg/abaplint/

# WASM compiler
go test -v -run TestWASMSuite ./pkg/wasmcomp/

# Cache, DSL, scripting
go test -v ./pkg/cache/ ./pkg/dsl/ ./pkg/scripting/

# Race detection
go test -race ./...
```

---

## Task 3: ABAP Linter — Fully Offline

No SAP, no Node.js, no network. Just pipe ABAP:

```bash
echo 'REPORT ztest.
DATA bad_name TYPE i.
.
COMPUTE bad_name = 42.
IF bad_name EQ 10. WRITE bad_name. ENDIF.' | ./vsp lint --stdin
```

Expected: finds `empty_statement`, `obsolete_statement`, `preferred_compare_operator`, `max_one_statement`.

**Lint a real file:**
```bash
./vsp lint --file embedded/abap/zcl_vsp_utils.clas.abap
```

**What to spotlight:**
- gcc-style output (`file:row:col: severity [rule] message`)
- 8 rules: line_length, empty_statement, obsolete_statement, max_one_statement, preferred_compare_operator, colon_missing_space, double_space, local_variable_names
- Oracle-verified against TypeScript abaplint (100% match)

---

## Task 4: ABAP Parser — Fully Offline

```bash
echo 'CLASS zcl_demo DEFINITION PUBLIC.
  PUBLIC SECTION.
    METHODS run IMPORTING iv_name TYPE string.
ENDCLASS.
CLASS zcl_demo IMPLEMENTATION.
  METHOD run.
    DATA lv_result TYPE string.
    lv_result = iv_name.
    IF lv_result IS NOT INITIAL.
      WRITE lv_result.
    ENDIF.
  ENDMETHOD.
ENDCLASS.' | ./vsp parse --stdin --format summary
```

Expected: 13 statements, types: ClassDefinition, ClassImplementation, MethodDef, etc.

**JSON output for tooling:**
```bash
echo "DATA lv_x TYPE i. lv_x = 42." | ./vsp parse --stdin --format json
```

---

## Task 5: WASM→ABAP Compiler — Fully Offline

```bash
# Compile a WASM binary to ABAP (if you have one)
./vsp compile wasm pkg/wasmcomp/testdata/quickjs_eval.wasm --class ZCL_QUICKJS 2>/dev/null | head -20

# Or build the test suite WASM and compile it
go test -v -run TestWASMSuite_CompileGo ./pkg/wasmcomp/
cat /tmp/wasm_suite_go.abap | head -20
```

**What to spotlight:**
- 225 bytes WASM → 117 lines ABAP
- 3-way verified: Native WASM (51/51), Go compiler, ABAP self-host on SAP (11/11)
- Functions: add, factorial, fibonacci, gcd, is_prime, abs, max, min, pow, sum_to, collatz, select

---

## Task 6: Config & Safety (no SAP needed)

```bash
# Generate example configs
./vsp config init
cat .env.example
cat .vsp.json.example

# Test safety flags
SAP_MODE=expert SAP_READ_ONLY=true ./vsp config show
SAP_ALLOWED_PACKAGES='Z*,$TMP' ./vsp config show
```

**Multi-system profiles:**
```bash
cp .vsp.json.example .vsp.json
./vsp systems
```

**Safety review:** Read `pkg/adt/safety.go` + `safety_test.go` (25 tests).

| Flag | What It Does |
|------|-------------|
| `--read-only` | Blocks all write operations |
| `--block-free-sql` | Blocks `RunQuery` (arbitrary SQL) |
| `--allowed-packages 'Z*,$TMP'` | Restricts to matching packages |
| `--allowed-ops RSQ` | Whitelist: only Read, Search, Query |
| `--disallowed-ops CDUA` | Blacklist: block Create, Delete, Update, Activate |

---

## Task 7: With SAP Access — Quick Smoke Test

If you have an SAP system with ADT enabled:

```bash
export SAP_URL=https://host:44300 SAP_USER=dev SAP_PASSWORD=secret
# Or: ./vsp -s dev ...

# System info
./vsp system info

# Search
./vsp search "ZCL_*" --max 10

# Query a table
./vsp query T000 --top 3

# Grep source code
./vsp grep "SELECT" --package '$TMP' --max 5

# Read source with dependency context
./vsp context CLAS ZCL_SOMETHING --depth 2
```

**What to spotlight:** Everything works with standard ADT. No ZADT_VSP needed.

---

## Task 8: Graph & Dependency Analysis (with SAP)

```bash
# What does a class use?
./vsp graph CLAS ZCL_MY_CLASS

# Who uses an interface?
./vsp graph INTF ZIF_MY_INTERFACE --direction callers

# Transaction → resolve to program → graph
./vsp graph TRAN SE80

# Package transport readiness
./vsp deps '$MY_PACKAGE' --format summary
```

**What to spotlight:**
- `graph` falls back to WBCROSSGT/CROSS tables when ADT call graph API is unavailable
- `deps` classifies: internal (safe) / external custom (need transport) / SAP standard (always there)
- Transaction resolution via TSTC table

---

## Task 9: Lua Scripting & YAML Workflows (with SAP)

vsp includes a Lua scripting engine (50+ SAP bindings) and a YAML workflow engine for automation.

**Lua REPL:**
```bash
./vsp -s dev lua
# lua> objs = searchObject("ZCL_VSP*")
# lua> for _, o in ipairs(objs) do print(o.name) end
# lua> rows = query("SELECT MANDT, MTEXT FROM T000")
# lua> source = getSource("CLAS", "ZCL_VSP_UTILS")
# lua> issues = lint(source)
# lua> stmts = parse(source)
```

**Run example scripts:**
```bash
# Package audit — lint + parse all classes
./vsp -s dev lua examples/scripts/package-audit.lua

# Table explorer — interactive SQL queries
./vsp -s dev lua examples/scripts/table-explorer.lua

# Dependency check — transport readiness via WBCROSSGT
./vsp -s dev lua examples/scripts/dependency-check.lua

# Debug session recording
./vsp -s dev lua examples/scripts/record-debug-session.lua
```

**YAML workflows:**
```bash
# CI pipeline: search → syntax check → unit tests
./vsp -s dev workflow run examples/workflows/ci-pipeline.yaml

# Pre-transport quality gate
./vsp -s dev workflow run examples/workflows/quality-gate.yaml --var PACKAGE='$ZADT_VSP'
```

**What to spotlight:**
- Lua has full SAP access: search, query, grep, source, debug, lint, parse
- `query()` returns Lua tables — native data processing
- `lint()` and `parse()` work on any string — no SAP needed
- YAML workflows with variable substitution, step chaining, error handling
- Debugger scripting: set breakpoints, step, inspect, record, replay

**Lua API categories (50+ functions):**

| Category | Functions |
|----------|-----------|
| Search & Source | `searchObject`, `grepObjects`, `getSource`, `writeSource`, `editSource` |
| Query & Analysis | `query`, `lint`, `parse`, `context`, `systemInfo` |
| Debugging | `setBreakpoint`, `listen`, `attach`, `stepOver/Into/Return`, `getStack`, `getVariables` |
| Breakpoints | line, statement, exception, message, BAdi, enhancement, watchpoint, method |
| Recording | `startRecording`, `stopRecording`, `saveRecording`, `loadRecording` |
| Time Travel | `getStateAtStep`, `findWhenChanged`, `findChanges`, `forceReplay` |
| Diagnostics | `listDumps`, `getDump`, `runUnitTests`, `syntaxCheck` |
| Utilities | `print`, `sleep`, `json.encode/decode` |

---

## Task 10: MCP Integration

If you have Claude Desktop, Gemini CLI, Copilot, or any MCP client:

```bash
./vsp config init
cat .mcp.json.example
```

Ready-to-use configs for 8 AI agents in `docs/cli-agents/`.

**What to spotlight:**
- Hyperfocused mode: 1 universal `SAP()` tool, ~200 tokens schema (vs ~40K for 122 tools)
- Context compression: dependencies auto-appended to GetSource
- Method-level surgery: 95% token reduction

---

## Task 11: Code Quality (for Go developers)

```bash
go vet ./...
go test -race ./...
ls -lh vsp                    # binary size
go mod graph | wc -l          # dependency count
```

**Key files:**

| File | What | Lines |
|------|------|------:|
| `internal/mcp/server.go` | 122 MCP tool handlers | ~250 |
| `pkg/adt/client.go` | ADT HTTP client | ~1800 |
| `pkg/adt/safety.go` | Enterprise safety | ~200 |
| `pkg/abaplint/lexer.go` | ABAP lexer (abaplint port) | ~340 |
| `pkg/abaplint/rules.go` | 8 lint rules | ~320 |
| `pkg/scripting/bindings.go` | 50+ Lua→SAP bindings | ~1600 |
| `pkg/dsl/workflow.go` | YAML workflow engine | ~600 |
| `pkg/wasmcomp/compile.go` | WASM→ABAP compiler | ~500 |
| `pkg/ts2go/ts2go.go` | TypeScript→Go transpiler | ~500 |
| `cmd/vsp/devops.go` | CLI command handlers | ~1100 |

---

## Quick Reference

| What | Command | SAP? |
|------|---------|:----:|
| Build | `go build -o vsp ./cmd/vsp` | — |
| Unit tests | `go test ./...` | — |
| Lint ABAP | `./vsp lint --file x.abap` | — |
| Parse ABAP | `./vsp parse --stdin` | — |
| Compile WASM | `./vsp compile wasm x.wasm` | — |
| Config | `./vsp config init/show` | — |
| System info | `./vsp system info` | ✅ |
| Search | `./vsp search "Z*"` | ✅ |
| Query table | `./vsp query T000 --top 5` | ✅ |
| Grep source | `./vsp grep "pattern" --package PKG` | ✅ |
| Call graph | `./vsp graph CLAS ZCL_X` | ✅ |
| Package deps | `./vsp deps '$PKG' --format summary` | ✅ |
| Read source | `./vsp source read CLAS ZCL_X` | ✅ |
| Context | `./vsp context CLAS ZCL_X --depth 2` | ✅ |
| Lua REPL | `./vsp lua` | ✅ |
| Lua script | `./vsp lua script.lua` | ✅ |
| YAML workflow | `./vsp workflow run pipeline.yaml` | ✅ |
| Unit tests | `./vsp test CLAS ZCL_X` | ✅ |
| Deploy | `./vsp deploy x.clas.abap '$TMP'` | ✅ |
| Export | `./vsp export '$PKG' -o backup.zip` | ✅+ |

✅+ = needs ZADT_VSP WebSocket

---

## Found Something?

- Open an issue: https://github.com/oisee/vibing-steampunk/issues
- PRs welcome — especially for: test coverage, error messages, documentation, new lint rules, MCP agent configs
