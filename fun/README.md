# fun/ — Try It Now

Build vsp once, then play:
```bash
go build -o vsp ./cmd/vsp
```

---

## 1. C → ABAP (one command)

```bash
# Your function → typed ABAP
echo 'int square(int x) { return x * x; }' > /tmp/sq.c
./vsp compile llvm /tmp/sq.c
# → CLASS-METHODS square IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.

# With ZIP for SAP import
./vsp compile llvm /tmp/sq.c --class zcl_math --zip -o math.zip

# Fibonacci, factorial, gcd, lerp
./vsp compile llvm fun/examples/fibonacci.c --class zcl_fib

# FatFS filesystem (7249 lines C → 8016 lines ABAP)
./vsp compile llvm ~/dev/minz/examples/c89/fatfs/ff.c --class zcl_fatfs --zip

# QuickJS JS engine (55K lines C → 124K lines ABAP, 537 functions)
./vsp compile llvm /tmp/quickjs-2024-01-13/quickjs.c --class zcl_qjs --zip
```

## 2. LLVM IR directly

```bash
# Compile C to LLVM IR yourself, then to ABAP
clang -S -emit-llvm -O1 mycode.c -o mycode.ll
./vsp compile llvm mycode.ll --class zcl_mycode

# Rust → LLVM IR → ABAP (needs rustc)
echo 'pub fn add(a: i32, b: i32) -> i32 { a + b } fn main(){}' > /tmp/r.rs
rustc --emit=llvm-ir /tmp/r.rs -o /tmp/r.ll
./vsp compile llvm /tmp/r.ll --class zcl_rust
```

## 3. WASM → ABAP

```bash
# Any .wasm file
./vsp compile wasm pkg/wasmcomp/testdata/factorial.wasm --class zcl_fact

# Available test WASMs:
ls pkg/wasmcomp/testdata/*.wasm
# add, factorial, fibonacci, collatz, sum_to, pow2, suite, extended
```

## 4. TypeScript → ABAP (via Porffor → WASM)

```bash
npm install -g porffor
echo 'const x: number = 2 + 3;' > /tmp/demo.ts
npx porffor wasm /tmp/demo.ts /tmp/demo.wasm -t
./vsp compile wasm /tmp/demo.wasm --class zcl_ts_demo
```

## 5. ABAP offline tools (no SAP needed)

```bash
# Lint
./vsp lint --file myprogram.abap

# Parse to AST
./vsp parse --file myprogram.abap --format json

# Tokenize
echo "DATA lv TYPE i. lv = 42." | ./vsp parse --stdin
```

## 6. SAP tools (needs SAP_URL/SAP_USER/SAP_PASSWORD)

```bash
# Execute ABAP
./vsp execute "DATA(x) = 2 + 2. WRITE x."

# Search code
./vsp grep "FACTORIAL" --package '$TMP'

# Run unit tests
./vsp test CLAS ZCL_MY_CLASS

# System info
./vsp system info
```

## 7. Pre-built ZIPs (import via abapGit)

```
pkg/llvm2abap/output/zllvm_test_01.zip   — 34 functions + 17 unit tests
pkg/llvm2abap/output/quickjs_llvm.zip    — QuickJS 537 functions, 124K lines
```

## 8. QuickJS on SAP — JavaScript engine compiled to ABAP

See **[QUICKJS_ON_SAP.md](QUICKJS_ON_SAP.md)** for the full guide.

```bash
# Compile QuickJS C → ABAP (one command!)
vsp compile llvm quickjs.c --class zcl_quickjs --zip -o quickjs.zip
# → 537 CLASS-METHODS, 124K lines, 0 TODOs, ready for abapGit import
```

## 9. The Full Chain: abaplint on QuickJS-from-LLVM

```bash
# Step 1: Build QuickJS from C via LLVM IR
curl -L bellard.org/quickjs/quickjs-2024-01-13.tar.xz | tar xJ -C /tmp/
cd /tmp/quickjs-2024-01-13 && make qjs  # builds repl.c, qjscalc.c

# Step 2: Compile all .c → .ll → .o → native binary
for f in quickjs cutils libbf libregexp libunicode quickjs-libc qjs repl qjscalc; do
  clang -S -emit-llvm -O1 -fPIC -DCONFIG_VERSION=\"test\" -D_GNU_SOURCE -DCONFIG_BIGNUM $f.c -o $f.ll 2>/dev/null
  llc -filetype=obj -relocation-model=pic $f.ll -o ${f}_ll.o
done
clang -O1 *_ll.o -lm -ldl -lpthread -o /tmp/qjs_from_llvm

# Step 3: Test — JavaScript on QuickJS-from-LLVM
echo 'console.log([1,2,3].map(x=>x*x))' | /tmp/qjs_from_llvm --std
# → 1,4,9

# Step 4: Bundle abaplint for QuickJS
npm install @abaplint/core
npx esbuild your_script.js --bundle --format=iife --platform=node --external:crypto -o flat.js

# Step 5: Run abaplint on QuickJS-from-LLVM
/tmp/qjs_from_llvm flat.js
# → Tokens: 15 (parses ABAP!)

# Step 6: Compile QuickJS to ABAP for SAP (next session!)
./vsp compile llvm /tmp/quickjs-2024-01-13/quickjs.c --class zcl_qjs --zip -o quickjs.zip
```

---

## What gets compiled

| Source | Command | Output |
|--------|---------|--------|
| `int add(int a, int b)` | `vsp compile llvm` | `CLASS-METHODS add IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i` |
| `double lerp(...)` | `vsp compile llvm` | `IMPORTING a TYPE f b TYPE f ... RETURNING VALUE(rv) TYPE f` |
| `long long add64(...)` | `vsp compile llvm` | `IMPORTING a TYPE int8 ... RETURNING VALUE(rv) TYPE int8` |
| `.wasm binary` | `vsp compile wasm` | `FORM f0 USING p0 TYPE i CHANGING rv TYPE i` |
| TypeScript `.ts` | porffor → `vsp compile wasm` | WASM-style ABAP |

## Quick comparison: same function, two paths

```bash
# Path 1: C → LLVM → ABAP (typed, clean)
./vsp compile llvm fun/examples/fibonacci.c --class zcl_fib

# Path 2: C → WASM → ABAP (untyped, assembly-like)
clang --target=wasm32 -O1 -nostdlib -Wl,--no-entry \
  -Wl,--export=fibonacci fun/examples/fibonacci.c -o /tmp/fib.wasm
./vsp compile wasm /tmp/fib.wasm --class zcl_fib_wasm
```
