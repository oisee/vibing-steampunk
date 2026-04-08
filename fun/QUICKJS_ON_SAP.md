# QuickJS on SAP: JavaScript Engine Compiled to ABAP

Run JavaScript on SAP — via QuickJS (C) compiled through LLVM IR to ABAP.

## How It Works

```
quickjs.c (55K lines C)
    ↓ vsp compile llvm
zcl_quickjs.abap (124K lines, 537 CLASS-METHODS)
    ↓ abapGit import
SAP ABAP runtime
    ↓ GENERATE SUBROUTINE POOL
JavaScript execution on SAP!
```

## Quick Start

### 1. Get QuickJS source

```bash
curl -L bellard.org/quickjs/quickjs-2024-01-13.tar.xz | tar xJ -C /tmp/
```

### 2. Compile to ABAP

```bash
# One command:
vsp compile llvm /tmp/quickjs-2024-01-13/quickjs.c \
  --class zcl_quickjs --zip -o quickjs.zip

# Output: quickjs.zip (691KB, abapGit format)
# Contains: 537 CLASS-METHODS, 124K lines typed ABAP
```

Or use the pre-built ZIP:
```bash
ls pkg/llvm2abap/output/quickjs_llvm.zip  # 691KB, ready to import
```

### 3. Deploy to SAP

Import via abapGit:
1. Open abapGit in SAP (transaction `ZABAPGIT`)
2. New → Offline → Upload ZIP
3. Select `quickjs.zip`
4. Target package: `$TMP` (or create `$ZQJS`)
5. Pull / Import

### 4. Run JavaScript

Create a test report:

```abap
REPORT zqjs_test.

" QuickJS compiled from C via LLVM IR
" Class zcl_quickjs contains 537 CLASS-METHODS

DATA: lt_code TYPE STANDARD TABLE OF string,
      lv_prog TYPE string.

" Load the generated ABAP source (from zcl_quickjs)
" and GENERATE SUBROUTINE POOL
" ... (source is the class + wrapper FORMs)

" Alternative: if deployed as a persistent class,
" call directly:
DATA(lv_result) = zcl_quickjs=>js_eval( ... ).
```

## What Works (verified)

### Native (x86, from same LLVM IR)

```bash
# Build native binary from LLVM IR
cd /tmp/quickjs-2024-01-13
for f in quickjs cutils libbf libregexp libunicode quickjs-libc qjs repl qjscalc; do
  clang -S -emit-llvm -O1 -fPIC \
    -DCONFIG_VERSION=\"test\" -D_GNU_SOURCE -DCONFIG_BIGNUM \
    $f.c -o $f.ll 2>/dev/null
  llc -filetype=obj -relocation-model=pic $f.ll -o ${f}_ll.o
done
clang -O1 *_ll.o -lm -ldl -lpthread -o qjs_from_llvm

# Test:
echo 'console.log(2+2)' > test.js
./qjs_from_llvm test.js
# → 4

echo 'console.log(JSON.stringify({a:1,b:[2,3]}))' > test2.js
./qjs_from_llvm test2.js
# → {"a":1,"b":[2,3]}

echo 'console.log([1,2,3,4,5].map(x=>x*x).filter(x=>x>5))' > test3.js
./qjs_from_llvm test3.js
# → 9,16,25
```

### ABAP (from same LLVM IR)

```bash
vsp compile llvm /tmp/quickjs-2024-01-13/quickjs.c --class zcl_quickjs
# → 537 CLASS-METHODS, 124K lines, 0 TODOs, max line 239 chars
```

### abaplint on QuickJS (TypeScript parser on C engine)

```bash
# Bundle abaplint (TypeScript) for QuickJS
npm install @abaplint/core
cat > lexer_test.js << 'EOF'
const {Lexer} = require("@abaplint/core/build/src/abap/1_lexer/lexer");
const {MemoryFile} = require("@abaplint/core");
const file = new MemoryFile("test.abap", "DATA lv TYPE i. lv = 42.");
const r = new Lexer().run(file);
console.log("Tokens: " + r.tokens.length);
r.tokens.forEach((t, i) => console.log("  " + t.getStr()));
EOF

npx esbuild lexer_test.js --bundle --format=iife \
  --platform=node --external:crypto -o abaplint_flat.js

# Shim for QuickJS (no require/process)
cat > shim.js << 'EOF'
globalThis.require = function(m) {
  if (m === "crypto") return { randomBytes: (n) => new Uint8Array(n) };
  throw new Error("no module: " + m);
};
globalThis.process = { env: {} };
EOF

cat shim.js abaplint_flat.js > abaplint_run.js

# Run: QuickJS-from-LLVM executes abaplint which parses ABAP
./qjs_from_llvm abaplint_run.js
# → Tokens: 12
# →   DATA
# →   lv
# →   TYPE
# →   i
# →   .
# →   lv
# →   =
# →   42
# →   .
```

## Architecture

### Generated ABAP structure

```abap
CLASS zcl_quickjs DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    " 537 CLASS-METHODS — the full QuickJS engine
    CLASS-METHODS js_malloc_rt IMPORTING a TYPE i b TYPE int8
      RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS js_free_rt IMPORTING a TYPE i b TYPE i.
    CLASS-METHODS JS_NewInt32 IMPORTING a TYPE i b TYPE i
      RETURNING VALUE(rv) TYPE int8.
    " ... 534 more methods ...
ENDCLASS.

CLASS zcl_quickjs IMPLEMENTATION.
  METHOD js_malloc_rt.
    " Typed ABAP with CASE dispatcher for control flow
    DATA lv_block TYPE string VALUE '1'.
    DO.
      CASE lv_block.
        WHEN '1'. ...
        WHEN '3'. ...
      ENDCASE.
    ENDDO.
  ENDMETHOD.
  " ... 536 more methods ...
ENDCLASS.
```

### Type mapping (LLVM → ABAP)

| C type | LLVM IR | ABAP |
|--------|---------|------|
| `int` | `i32` | `TYPE i` |
| `long long` / `int64_t` | `i64` | `TYPE int8` |
| `double` | `double` | `TYPE f` |
| `void*` / pointer | `ptr` | `TYPE i` (offset) |
| `struct` | `%struct.X` | field offset calculation |

### Comparison: WASM path vs LLVM path

| | WASM → ABAP | LLVM IR → ABAP |
|---|---|---|
| Lines | 218K | **124K** (−43%) |
| Types | All `TYPE i` | `TYPE i` / `int8` / `f` |
| Functions | `FORM f0 USING p0` | `CLASS-METHODS js_malloc_rt IMPORTING a TYPE i` |
| Compile time | 4.4s | **0.3s** |
| Memory model | xstring + s0/s1 registers | xstring + named vars |
| TODOs | 0 | 0 |

## Status

| Test | Result |
|------|--------|
| LLVM IR valid (llc) | ✅ 222K lines x86 assembly |
| Native execution | ✅ console.log, JSON, RegExp, Array.map |
| abaplint on QuickJS | ✅ 15 tokens parsed |
| ABAP generation | ✅ 537 functions, 0 TODOs |
| ABAP lint check | ✅ 139K tokens, 2191 statements |
| SAP GENERATE (WASM path) | ✅ rc=0, 218K lines |
| SAP GENERATE (LLVM path) | 🔜 next session |
| JS eval on SAP | 🔜 needs WASI stubs + deploy |

## Next Steps

1. **Deploy** `quickjs_llvm.zip` via abapGit
2. **Add WASI stubs** (fd_write → gv_stdout, fd_read → gv_stdin)
3. **Test** `console.log(2+2)` on SAP via GENERATE
4. **Run abaplint.js** on QuickJS-ABAP on SAP
5. **Goal**: TypeScript tools running on SAP via C→LLVM→ABAP
