# fun/ — Things You Can Try Right Now

Hands-on experiments with vsp's compilers, tools and SAP integration.

## Quick Setup

```bash
# Build everything
go build -o vsp ./cmd/vsp
go build -o abapgit-pack ./cmd/abapgit-pack

# LLVM tools (need clang 18+)
clang --version
```

---

## 1. Compile C to ABAP (LLVM path)

Write any C function, compile to LLVM IR, then to typed ABAP:

```bash
# Write a function
cat > /tmp/myfunc.c << 'EOF'
int fibonacci(int n) {
    if (n <= 1) return n;
    int a = 0, b = 1;
    for (int i = 2; i <= n; i++) {
        int t = a + b; a = b; b = t;
    }
    return b;
}
int square(int x) { return x * x; }
double lerp(double a, double b, double t) { return a + (b - a) * t; }
EOF

# C → LLVM IR
clang -S -emit-llvm -O1 /tmp/myfunc.c -o /tmp/myfunc.ll

# LLVM IR → ABAP (use Go directly)
go run fun/llvm2abap_demo.go /tmp/myfunc.ll
```

Output: typed CLASS-METHODS with `IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i`.

### Try these:

| Source | What you get |
|--------|-------------|
| `int add(int a, int b) { return a+b; }` | `rv = a + b.` |
| `int factorial(int n) { ... }` | Loop with phi nodes, CASE dispatcher |
| `double lerp(double a, double b, double t)` | `TYPE f` params and return |
| `long long add64(long long a, long long b)` | `TYPE int8` (64-bit) |
| FatFS `ff.c` (7249 lines) | 28 CLASS-METHODS, 8K lines ABAP |
| QuickJS `quickjs.c` (55K lines) | 537 CLASS-METHODS, 124K lines ABAP |

## 2. Compile TypeScript to ABAP (Porffor → WASM path)

```bash
# Install Porffor
npm install -g porffor

# Write TypeScript
cat > /tmp/demo.ts << 'EOF'
const x: number = 2 + 3;
EOF

# TS → WASM
npx porffor wasm /tmp/demo.ts /tmp/demo.wasm -t

# WASM → ABAP
go run fun/wasm2abap_demo.go /tmp/demo.wasm
```

## 3. Package for SAP (abapGit ZIP)

```bash
# Any ABAP file → abapGit-compatible ZIP
go build -o abapgit-pack ./cmd/abapgit-pack

./abapgit-pack \
  -file myprogram.abap \
  -name ZMYPROG \
  -desc "My compiled program" \
  -package '$TMP' \
  -o myprogram.zip

# Import in SAP: abapGit → Offline → Import ZIP
```

Pre-built ZIPs ready to import:
- `pkg/llvm2abap/output/zllvm_test_01.zip` — 34 functions + 17 unit tests
- `pkg/llvm2abap/output/quickjs_llvm.zip` — QuickJS JS engine (537 functions!)

## 4. WASM → ABAP (the proven path)

```bash
# Compile any .wasm to ABAP function pool
go run fun/wasm_fugr_demo.go testdata/factorial.wasm

# Pre-built test WASMs:
ls pkg/wasmcomp/testdata/*.wasm
#   add.wasm, factorial.wasm, fibonacci.wasm, collatz.wasm,
#   sum_to.wasm, pow2.wasm, suite.wasm, extended.wasm,
#   quickjs_eval.wasm (1.2MB QuickJS!)
```

Output: CLASS g with CLASS-METHODS (block-as-method architecture).

## 5. Run on SAP (if you have access)

```bash
# Configure
export SAP_URL=http://your-sap:50000
export SAP_USER=developer
export SAP_PASSWORD=secret

# Execute ABAP directly
./vsp execute "DATA(x) = 2 + 2. WRITE x."

# Run unit tests
./vsp test CLAS ZCL_MY_CLASS

# Search code
./vsp grep "FACTORIAL" --package '$TMP'

# Deploy via abapGit ZIP
# Upload pkg/llvm2abap/output/zllvm_test_01.zip via abapGit UI
```

## 6. Offline ABAP Tools (no SAP needed)

```bash
# Lint ABAP source
./vsp lint --file myprogram.abap

# Parse ABAP to AST
./vsp parse --file myprogram.abap --format json

# ABAP lexer (abaplint-compatible)
echo "DATA lv TYPE i. lv = 42." | ./vsp parse --stdin
```

## 7. Inspect Generated ABAP

Compare the two compilation paths:

```bash
# Same C function, two paths:

# Path 1: C → LLVM IR → ABAP (typed)
clang -S -emit-llvm -O1 fun/examples/fibonacci.c -o /tmp/fib.ll
go run fun/llvm2abap_demo.go /tmp/fib.ll
# Output: CLASS-METHODS fibonacci IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i

# Path 2: C → WASM → ABAP (untyped)
clang --target=wasm32 -O1 fun/examples/fibonacci.c -o /tmp/fib.wasm
go run fun/wasm2abap_demo.go /tmp/fib.wasm
# Output: FORM f0 USING p0 TYPE i CHANGING rv TYPE i
```

---

## What's Inside

| Package | What | Try it |
|---------|------|--------|
| `pkg/llvm2abap` | LLVM IR → typed ABAP | `go test ./pkg/llvm2abap/ -v` |
| `pkg/wasmcomp` | WASM → ABAP (CLASS g) | `go test ./pkg/wasmcomp/ -v` |
| `pkg/abaplint` | ABAP lexer (Go port) | `go test ./pkg/abaplint/ -v` |
| `pkg/ts2abap` | TS → ABAP transpiler | `go test ./pkg/ts2abap/ -v` |
| `cmd/vsp` | MCP server (122 tools) | `./vsp --help` |
| `cmd/abapgit-pack` | abapGit ZIP creator | `./abapgit-pack --help` |

## Stats (this session)

- **19 commits**, 2 compilers built
- **LLVM → ABAP**: 537 functions (QuickJS), FatFS, SAP verified 5/5
- **WASM → ABAP**: 12K CLASS-METHODS, QuickJS GENERATE rc=0
- **TS → ABAP**: Porffor chain proven
- **abapgit-pack**: CLI tool for SAP deployment
