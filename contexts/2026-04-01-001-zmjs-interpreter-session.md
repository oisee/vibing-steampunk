# ZMJS — JS Interpreter on SAP ABAP: Full Session Context

**Date:** 2026-04-01
**Sessions covered:** 2026-03-29 → 2026-04-01 (multiple)
**Subject:** ZMJS JavaScript interpreter on SAP ABAP — architecture, test262, VM design, interpreter consolidation

---

## 1. What Is ZMJS

ZMJS is a JavaScript interpreter running natively on SAP ABAP. The goal: evaluate JS source
directly inside an ABAP runtime via `ZCL_MJS=>EVAL( iv_source )`, returning captured
`console.log` output as a string.

**Primary use cases:**
- Run JS business logic without leaving ABAP (no RFC, no HTTP)
- Run TC39 test262 conformance tests directly on SAP to measure JS compliance
- Foundation for a future self-hosted bytecode VM (ZJSVM)

**Go reference implementation:** `~/dev/zmjs/pkg/jseval/jseval.go`
This mirrors `ZCL_MJS` 1:1 and is used for rapid development + test262 CI.

---

## 2. Canonical ABAP Objects (SAP System a4h-105-adt)

All objects live in package `$ZMJS` (or similar local package).

### The Interpreter Stack

```
ZCL_MJS          — canonical JS interpreter (USE THIS)
  ├── ZIF_MJS         — all types: ty_value, ty_node, ty_token, ty_function, constants
  ├── ZCL_MJS_ENV     — scope/environment (vars, parent, output buffer, returning/breaking flags)
  ├── ZCL_MJS_PARSER  — tokenizer + recursive descent parser → AST
  ├── ZCL_MJS_OBJ     — JS object (props table, get/set/copy_from)
  └── ZCL_MJS_ARR     — JS array (push, length, get_item)

ZCL_JSEVAL       — DEPRECATED stub, delegates to ZCL_MJS=>EVAL
                   (was a parallel test implementation, now gutted)
ZCL_JSEVAL_ENV   — still exists but orphaned (ZCL_JSEVAL no longer uses it)
ZCL_JSEVAL_PARSER — still exists but orphaned
ZIF_JSEVAL       — still exists but orphaned
```

### Test / Runner Programs

```
ZMJS_TEST262     — loads ZMJS_TEST262.JS from SMW0, runs via ZCL_MJS=>EVAL, prints results
ZTEST_JSEVAL     — unit test class: bench_fib25, bench_loop_10k, bench_string_concat,
                   lexer_big_abap — all now call ZCL_MJS=>EVAL (was ZCL_JSEVAL)
```

### SMW0 Objects

```
ZMJS_TEST262.JS  — flattened TC39 test262 bundle (190KB, ~267 tests)
ZABAPLINT_LEXER.JS — abaplint lexer (complex JS, used by ZTEST_JSEVAL)
```

---

## 3. ZCL_MJS Architecture

### Value Types (`ty_value.type`)
```
0 = undefined
1 = number   (f)
2 = string   (str)
3 = bool     (num: 1=true, 0=false)
4 = function (fn: REF TO ty_function)
5 = null
6 = object   (obj: REF TO zcl_mjs_obj)
7 = array    (arr: REF TO zcl_mjs_arr)
```

### Node Kinds (`zif_mjs` constants)
```
0  = number literal      11 = function declaration
1  = string literal      12 = return
2  = identifier          13 = object literal {}
3  = binary op           14 = array literal []
4  = unary op            15 = member access (obj.prop / obj[idx])
5  = assignment          16 = member assign
6  = var declaration     17 = method call
7  = if                  18 = for
8  = while               19 = switch
9  = block               20 = typeof
10 = call                21 = new
                         22 = class
                         23 = break
                         24 = continue
                         25 = bool literal  ← ZCL_MJS has this, ZCL_JSEVAL did NOT
```

### Key eval_node behaviors
- `c_node_func_decl`: stores function in env, returns **undefined** (not the function value!)
  → function *expressions* as arguments don't work — returns undefined as the arg value
- `c_node_call` with `str = "console.log"`: special-cased, appends to output buffer
- `c_node_new`: only works if ctor is type=6 (object/class), NOT type=4 (plain function)
- `c_node_method_call` on type=4 (function): falls to WHEN OTHERS → undefined (assert.sameValue etc. are no-ops)

### Parser `parse_primary` special case
`console.log(...)` is special-cased: parsed as `c_node_call` with `str = "console.log"`,
NOT as `c_node_method_call`. This is how output is captured.

---

## 4. ZCL_MJS vs ZCL_JSEVAL — Why We Merged

| Aspect               | ZCL_JSEVAL (old)            | ZCL_MJS (canonical)         |
|----------------------|-----------------------------|-----------------------------|
| `true`/`false`       | → `c_node_number` (1/0)     | → `c_node_bool` (25)        |
| `typeof true`        | returns `"number"` ❌        | returns `"boolean"` ✅       |
| Everything else      | identical                   | identical                   |
| Status               | **deprecated, delegates**   | **use this**                |

ZCL_MJS was already the superset. ZCL_JSEVAL was a parallel copy that fell slightly behind.
No code needed porting — just deprecate ZCL_JSEVAL.

---

## 5. Known Parser Limitations

### ❌ Function expressions (critical — breaks test262)
```js
__test("name", function() { assert.sameValue(1,1); });
```
`function()` in expression position is parsed as ident `function` → call `function()` → args=[].
Then `{ ... }` is consumed as an object literal arg to the OUTER call.
This cascades and corrupts the entire parse of the file.

**Impact:** The flattened test262 bundle produces no output when run through ZCL_MJS.
**Fix needed:** Handle `function` keyword in `parse_primary` to return a `c_node_func_decl`
node whose *value* is the function (rs_val = ls_fnval).

### ❌ try/catch/throw
Not implemented — no node kinds for these.
`try` is parsed as an ident (no-op), `catch(e)` as a call to undefined function,
`throw` as ident + discarded expression.
**Impact:** assertions (`assert.sameValue` → `$ERROR` → `throw`) are all no-ops.

### ❌ Function call on non-ident callee
`(getFactory())(args)` — parse_postfix only creates c_node_call for direct ident calls.
Calling a returned function doesn't work.

### ✅ What DOES work
- All arithmetic, comparison, logical operators
- Variables (let/var/const), assignment, scoping
- if/else, while, for, switch/case/break/continue
- return, recursion, closures
- Objects `{}`, arrays `[]`, member access `obj.prop`, `arr[i]`
- Method calls on objects (type=6) — invokes the stored function
- String methods: charAt, charCodeAt, indexOf, substring
- Array: push, length
- typeof
- class syntax (parses methods, creates object with method props)
- new ClassName(args) when ClassName is type=6 object

---

## 6. test262 Setup

### Go test runner (`~/dev/zmjs/pkg/jseval/test262_test.go`)
- Set `TEST262_PATH=/path/to/tc39/test262`
- Runs each test in a goroutine with 2s timeout (prevents infinite loops)
- Skips: modules, onlyStrict, raw, negative tests, unsupported features
- `TEST262_MIN_PASS=50` enforces regression floor
- CI: `.github/workflows/ci.yml` clones test262, runs with `TEST262_MIN_PASS=50`

### Flattened bundle (`~/dev/zmjs/cmd/gen262/`)
```bash
go run ./cmd/gen262/ -test262 /path/to/test262 -o zmjs_test262.js
```
Generates `zmjs_test262.js` (~190KB, ~267 tests) for upload to SMW0.
The harness wraps each test as `__test("name", function() { ... })`.

**⚠️ Known issue:** The `function() { ... }` callback syntax doesn't parse correctly
in ZCL_MJS. The bundle runs but produces empty output. See §5.

### SMW0 Loading Pattern (CORRECT)
```abap
DATA(lo_file) = zcl_llm_00_file_smw0=>new(
  iv_      = 'ZMJS_TEST262.JS'
  io_codec = zcl_llm_00_codec_mock=>new( )  " NO XOR transformation
).
DATA(lv_js) = lo_file->get_string( ).  " cl_abap_conv_in_ce, system default encoding
```
**Do NOT use:** `zcl_llm_00_codec=>new()` (applies XOR cipher, mangles content)
**Do NOT use:** `cl_abap_conv_codepage=>create_in('UTF-8')->convert()` (CONVT_CODEPAGE on SMW0 binary)

---

## 7. Bytecode VM Roadmap

### Phase 1: GoJSVM (Go compiler + ABAP VM)
Architecture defined in `~/dev/zmjs/pkg/jseval/bytecode.go`:

```
Opcodes defined:
  Stack:    PUSH_NUM, PUSH_STR, PUSH_BOOL, PUSH_NULL, PUSH_UNDEF, POP
  Vars:     LOAD, STORE, DEFVAR
  Arith:    ADD, SUB, MUL, DIV, MOD, NEG
  Compare:  EQ, NEQ, LT, LE, GT, GE
  Logical:  AND, OR, NOT, NULL_COALESCE
  Flow:     JUMP, JUMP_IF, JUMP_IF_NOT
  Funcs:    MAKE_FUNC, CALL, RETURN, RETURN_UNDEF
  Objects:  MAKE_OBJ, MAKE_ARR, GET_PROP, SET_PROP, GET_IDX, SET_IDX
  Classes:  NEW
  Native:   NATIVE (готовченки — direct ABAP ops for str/arr built-ins)
  Misc:     TYPEOF, DUP, PUSH_THIS
```

Design principles:
- **"Шитый код"** (threaded code): integer opcode dispatch via ABAP CASE
- **"Готовченки"** (NATIVE ops): str.charAt, arr.push → direct ABAP, no JS function call overhead
- Flat instruction array, sequential IP
- Typed value stack (not byte array — ABAP native types)
- Explicit call frames with captured env (closures)
- Designed to port 1:1 to ABAP WHILE+CASE (ZCL_MJS_BC_VM)

### Phase 2: ZJSVM (ABAP self-hosted)
- Port the bytecode compiler to ABAP (~500 lines)
- ABAP VM: `WHILE ip < lines. CASE instr-op. WHEN... ENDCASE. ENDWHILE.`
- This is the end goal: full JS bytecode VM in ABAP

### Why NOT WASM VM
Lars Hvam (abaplint) already tried WASM VM on ABAP → too slow (QuickJS needs millions
of init instructions). Custom 30-opcode bytecode is the right path.

### Why NOT TS→ABAP transpilation
TS types help AOT compilers generate typed ABAP (no CASE dispatch), but:
- An interpreted ZCL_MTS would be HARDER (more parsing, same runtime semantics)
- TS types belong in the vsp compilation pipeline (TS→LLVM→ABAP), not in an interpreter
- ZCL_MJS handles JS well; vsp handles the TS→JS step externally

---

## 8. SAP Utility Patterns

### Loading files from SMW0
```abap
" CORRECT: mock codec (no XOR), get_string uses cl_abap_conv_in_ce default encoding
DATA(lv_content) = zcl_llm_00_file_smw0=>new(
  iv_      = 'MYFILE.JS'
  io_codec = zcl_llm_00_codec_mock=>new( )
)->get_string( ).

" WRONG: default codec applies XOR cipher
" zcl_llm_00_file_smw0=>new( iv_ = 'MYFILE.JS' )  " no io_codec → XOR codec!
```

### Timing (use ZCL_VDB_002_STOPWATCH, not cl_abap_runtime)
```abap
DATA(lo_sw) = zcl_vdb_002_stopwatch=>new( ).
" ... work ...
lo_sw->next( 'label' ).
DATA(lv_ms) = lo_sw->get_stats( )-total / 1000.
```
**Do NOT use:** `cl_abap_runtime=>get_utc_long_timestamp()`

### ZCL_LLM utility methods
```abap
zcl_llm=>xstring_to_string( lv_xstr )  " uses cl_abap_conv_in_ce (system default)
zcl_llm=>string_to_xstring( lv_str )   " uses cl_abap_codepage=>convert_to UTF-8
zcl_llm=>tab_to_string( lt_tab )       " joins string table with newline
zcl_llm=>string_to_tab( lv_str )       " splits string to table by newline
```

### vsp CLI workflow for ABAP editing
```bash
# Export → edit locally → deploy back
./vsp source CLAS ZCL_MJS > /tmp/zcl_mjs.clas.abap
# ... edit with Write/Edit tools ...
./vsp source write CLAS ZCL_MJS < /tmp/zcl_mjs.clas.abap
```
Much better than inline SAP MCP edits for large refactoring.

---

## 9. Next Steps / Open Issues

### Immediate
1. **Fix function expressions** in `ZCL_MJS_PARSER.parse_primary`:
   When `function` keyword seen in expression context, create `c_node_func_decl` node
   AND return the function value (not undefined). This fixes the test262 bundle.

2. **Add try/catch/throw** to ZCL_MJS_PARSER + eval_node:
   - Add `c_node_try` (26), `c_node_throw` (27) to ZIF_MJS
   - `parse_statement`: handle `WHEN 'try'` → parse_try
   - `eval_node`: WHEN c_node_throw → raise ABAP exception; WHEN c_node_try → TRY/CATCH

3. **Run test262 on SAP** and get real PASS=N FAIL=M numbers
   (Currently produces empty output due to function expression issue)

### Medium term
4. **Bytecode compiler in Go** (AST → `[]Instr` in bytecode.go)
5. **Bytecode VM in ABAP** (`ZCL_MJS_BC_VM` — WHILE+CASE executor)
6. **Update gen262 harness** to avoid function expressions as a workaround:
   Rewrite `__test(name, fn)` callbacks as named functions or inline code

### Known regressions to watch
- `ZCL_JSEVAL_ENV`, `ZCL_JSEVAL_PARSER`, `ZIF_JSEVAL` are orphaned but still exist in SAP
  (safe to delete if nothing references them)
- `ZTEST_JSEVAL` now calls `ZCL_MJS` — all 4 bench methods should still pass

---

## 10. Conversation Wisdom / Lessons Learned

- **ZCL_MJS is the interpreter, ZCL_JSEVAL is deprecated** — never use ZCL_JSEVAL for new code
- **SMW0 binary data needs CODEC_MOCK** — default codec is XOR cipher
- **`cl_abap_conv_in_ce` without encoding** works for ASCII; `UTF-8` explicit often fails on SMW0 binary
- **function expressions break ZCL_MJS** — the single most important missing feature
- **ZCL_MJS c_node_func_decl** returns undefined as eval result (declaration, not expression)
- **assert.sameValue etc. are silent no-ops** in ZCL_MJS (method call on type=4 → WHEN OTHERS)
- **throw is a no-op** (no node kind) — tests can never "fail" through assertions currently
- **console.log special-cased** in parse_primary (not parse_postfix method call)
- **TS interpreter would be HARDER** than JS interpreter (superset, same runtime semantics)
- **TS types belong in vsp compile pipeline**, not in ABAP interpreter
- **vsp source write < file** is the right workflow for large ABAP edits
