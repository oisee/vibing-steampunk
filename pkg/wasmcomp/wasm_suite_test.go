package wasmcomp

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// WASMTestCase defines a test function with expected input→output pairs.
type WASMTestCase struct {
	Name     string
	FuncName string
	Args     []int32
	Expected int32
}

// buildSuiteWasm builds a WASM module with 5 test functions:
//   add(a,b)       = a+b
//   factorial(n)   = n!
//   fibonacci(n)   = fib(n)
//   gcd(a,b)       = GCD(a,b)
//   isPrime(n)     = 1 if prime, 0 otherwise
func buildSuiteWasm() []byte {
	w := newWasmBuilder()

	// Types: (i32,i32)->i32 and (i32)->i32
	w.addSection(1, buildTypeSection([]FuncType{
		{Params: []ValType{ValI32, ValI32}, Results: []ValType{ValI32}}, // type 0
		{Params: []ValType{ValI32}, Results: []ValType{ValI32}},        // type 1
	}))

	// Functions: 5 funcs
	w.addSection(3, buildFuncSection([]int{0, 1, 1, 0, 1}))

	// Exports
	w.addSection(7, buildExportSection([]Export{
		{Name: "add", Kind: 0, Index: 0},
		{Name: "factorial", Kind: 0, Index: 1},
		{Name: "fibonacci", Kind: 0, Index: 2},
		{Name: "gcd", Kind: 0, Index: 3},
		{Name: "is_prime", Kind: 0, Index: 4},
	}))

	// Code section

	// 0: add(a, b) = a + b
	addBody := buildFuncBody(nil, []byte{
		OpLocalGet, 0x00,
		OpLocalGet, 0x01,
		OpI32Add,
	})

	// 1: factorial(n) = if n<=1 then 1 else n*factorial(n-1)
	factBody := buildFuncBody(nil, []byte{
		OpLocalGet, 0x00,
		OpI32Const, 0x01,
		OpI32LeS,
		OpIf, 0x7F, // result i32
		OpI32Const, 0x01,
		OpElse,
		OpLocalGet, 0x00,
		OpLocalGet, 0x00,
		OpI32Const, 0x01,
		OpI32Sub,
		OpCall, 0x01, // call factorial
		OpI32Mul,
		OpEnd,
	})

	// 2: fibonacci(n) = if n<=1 then n else fib(n-1)+fib(n-2)
	fibBody := buildFuncBody(nil, []byte{
		OpLocalGet, 0x00,
		OpI32Const, 0x01,
		OpI32LeS,
		OpIf, 0x7F,
		OpLocalGet, 0x00, // return n
		OpElse,
		OpLocalGet, 0x00,
		OpI32Const, 0x01,
		OpI32Sub,
		OpCall, 0x02, // fib(n-1)
		OpLocalGet, 0x00,
		OpI32Const, 0x02,
		OpI32Sub,
		OpCall, 0x02, // fib(n-2)
		OpI32Add,
		OpEnd,
	})

	// 3: gcd(a, b) = if b==0 then a else gcd(b, a%b)
	gcdBody := buildFuncBody(nil, []byte{
		OpLocalGet, 0x01,
		OpI32Eqz,
		OpIf, 0x7F,
		OpLocalGet, 0x00, // return a
		OpElse,
		OpLocalGet, 0x01,         // b
		OpLocalGet, 0x00,         // a
		OpLocalGet, 0x01,         // b
		OpI32RemS,                // a % b
		OpCall, 0x03,             // gcd(b, a%b)
		OpEnd,
	})

	// 4: is_prime(n) — from wat2wasm verified bytecode
	// if n<2 return 0; i=2; block { loop { if i*i>n br_if 1; if n%i==0 return 0; i++; br 0 } } return 1
	primeBody := buildFuncBody([]ValType{ValI32}, []byte{
		0x20, 0x00, // local.get 0 (n)
		0x41, 0x02, // i32.const 2
		0x48,       // i32.lt_s
		0x04, 0x40, // if (void)
		0x41, 0x00, // i32.const 0
		0x0f,       // return
		0x0b,       // end if
		0x41, 0x02, // i32.const 2
		0x21, 0x01, // local.set 1 (i=2)
		0x02, 0x40, // block (void)
		0x03, 0x40, // loop (void)
		0x20, 0x01, // local.get 1 (i)
		0x20, 0x01, // local.get 1 (i)
		0x6c,       // i32.mul (i*i)
		0x20, 0x00, // local.get 0 (n)
		0x4a,       // i32.gt_s
		0x0d, 0x01, // br_if 1 (break block → prime)
		0x20, 0x00, // local.get 0 (n)
		0x20, 0x01, // local.get 1 (i)
		0x6f,       // i32.rem_s (n%i)
		0x45,       // i32.eqz
		0x04, 0x40, // if (void)
		0x41, 0x00, // i32.const 0
		0x0f,       // return
		0x0b,       // end if
		0x20, 0x01, // local.get 1 (i)
		0x41, 0x01, // i32.const 1
		0x6a,       // i32.add (i+1)
		0x21, 0x01, // local.set 1 (i=i+1)
		0x0c, 0x00, // br 0 (continue loop)
		0x0b,       // end loop
		0x0b,       // end block
		0x41, 0x01, // i32.const 1 (prime!)
	})

	w.addSection(10, buildCodeSection([][]byte{addBody, factBody, fibBody, gcdBody, primeBody}))

	return w.bytes()
}

var suiteTestCases = []WASMTestCase{
	// add
	{"add(2,3)", "add", []int32{2, 3}, 5},
	{"add(0,0)", "add", []int32{0, 0}, 0},
	{"add(-1,1)", "add", []int32{-1, 1}, 0},
	{"add(100,200)", "add", []int32{100, 200}, 300},

	// factorial
	{"factorial(0)", "factorial", []int32{0}, 1},
	{"factorial(1)", "factorial", []int32{1}, 1},
	{"factorial(5)", "factorial", []int32{5}, 120},
	{"factorial(10)", "factorial", []int32{10}, 3628800},

	// fibonacci
	{"fibonacci(0)", "fibonacci", []int32{0}, 0},
	{"fibonacci(1)", "fibonacci", []int32{1}, 1},
	{"fibonacci(10)", "fibonacci", []int32{10}, 55},
	{"fibonacci(15)", "fibonacci", []int32{15}, 610},

	// gcd
	{"gcd(12,8)", "gcd", []int32{12, 8}, 4},
	{"gcd(100,75)", "gcd", []int32{100, 75}, 25},
	{"gcd(17,13)", "gcd", []int32{17, 13}, 1},
	{"gcd(0,5)", "gcd", []int32{0, 5}, 5},

	// is_prime
	{"is_prime(2)", "is_prime", []int32{2}, 1},
	{"is_prime(7)", "is_prime", []int32{7}, 1},
	{"is_prime(10)", "is_prime", []int32{10}, 0},
	{"is_prime(97)", "is_prime", []int32{97}, 1},
	{"is_prime(100)", "is_prime", []int32{100}, 0},
	{"is_prime(1)", "is_prime", []int32{1}, 0},
}

// TestWASMSuite_Parse verifies the suite WASM binary parses correctly.
func TestWASMSuite_Parse(t *testing.T) {
	wasm := buildSuiteWasm()
	t.Logf("Suite WASM: %d bytes", len(wasm))

	mod, err := Parse(wasm)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(mod.Functions) != 5 {
		t.Fatalf("Expected 5 functions, got %d", len(mod.Functions))
	}

	for _, f := range mod.Functions {
		t.Logf("  %s: %d instructions", f.ExportName, len(f.Code))
	}
}

// TestWASMSuite_CompileGo verifies Go compiler produces valid ABAP.
func TestWASMSuite_CompileGo(t *testing.T) {
	wasm := buildSuiteWasm()
	mod, err := Parse(wasm)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	abap := Compile(mod, "zcl_wasm_suite")
	t.Logf("Go compiler → %d lines ABAP", strings.Count(abap, "\n"))

	// Structural checks
	for _, fn := range []string{"add", "factorial", "fibonacci", "gcd", "is_prime"} {
		if !strings.Contains(abap, "METHOD "+fn+".") {
			t.Errorf("Missing METHOD %s", fn)
		}
	}

	// Write for inspection
	os.WriteFile("/tmp/wasm_suite_go.abap", []byte(abap), 0644)
	t.Log("Written to /tmp/wasm_suite_go.abap")
}

// TestWASMSuite_GenerateABAPTest generates an ABAP test program that calls all functions
// and verifies results. This can be run on SAP via ExecuteABAP.
func TestWASMSuite_GenerateABAPTest(t *testing.T) {
	wasm := buildSuiteWasm()
	mod, err := Parse(wasm)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	abap := Compile(mod, "zcl_wasm_suite")

	// Generate test harness
	var sb strings.Builder
	sb.WriteString("* WASM→ABAP Test Suite — auto-generated\n")
	sb.WriteString("* Compile the WASM suite class first, then run this test.\n")
	sb.WriteString("* Each test calls a compiled WASM function and checks the result.\n\n")
	sb.WriteString("REPORT zwasm_suite_test.\n\n")
	sb.WriteString("DATA: lo TYPE REF TO zcl_wasm_suite,\n")
	sb.WriteString("      lv_result TYPE i,\n")
	sb.WriteString("      lv_pass TYPE i,\n")
	sb.WriteString("      lv_fail TYPE i.\n\n")
	sb.WriteString("CREATE OBJECT lo.\n\n")

	for _, tc := range suiteTestCases {
		args := ""
		for i, a := range tc.Args {
			if i > 0 {
				args += " "
			}
			args += fmt.Sprintf("p%d = %d", i, a)
		}
		sb.WriteString(fmt.Sprintf("* Test: %s = %d\n", tc.Name, tc.Expected))
		sb.WriteString(fmt.Sprintf("lo->%s( EXPORTING %s CHANGING rv = lv_result ).\n", tc.FuncName, args))
		sb.WriteString(fmt.Sprintf("IF lv_result = %d.\n", tc.Expected))
		sb.WriteString(fmt.Sprintf("  WRITE: / 'PASS: %s'.\n", tc.Name))
		sb.WriteString("  lv_pass = lv_pass + 1.\n")
		sb.WriteString("ELSE.\n")
		sb.WriteString(fmt.Sprintf("  WRITE: / 'FAIL: %s expected %d got', lv_result.\n", tc.Name, tc.Expected))
		sb.WriteString("  lv_fail = lv_fail + 1.\n")
		sb.WriteString("ENDIF.\n\n")
	}

	sb.WriteString("WRITE: / '---'.\n")
	sb.WriteString("WRITE: / 'Passed:', lv_pass, '/', lv_pass + lv_fail.\n")
	sb.WriteString("IF lv_fail > 0.\n")
	sb.WriteString("  WRITE: / 'FAILURES:', lv_fail.\n")
	sb.WriteString("ENDIF.\n")

	testProg := sb.String()
	t.Logf("Test program: %d lines", strings.Count(testProg, "\n"))

	// Write both files
	os.WriteFile("/tmp/wasm_suite_class.abap", []byte(abap), 0644)
	os.WriteFile("/tmp/wasm_suite_test.abap", []byte(testProg), 0644)
	t.Log("Written: /tmp/wasm_suite_class.abap + /tmp/wasm_suite_test.abap")

	// Also write the raw WASM binary for the self-hosting compiler
	os.WriteFile("/tmp/wasm_suite.wasm", wasm, 0644)
	t.Logf("WASM binary: /tmp/wasm_suite.wasm (%d bytes)", len(wasm))
	t.Log("")
	t.Log("To test on SAP:")
	t.Log("  1. Deploy zcl_wasm_suite via: vsp deploy /tmp/wasm_suite_class.abap '$TMP'")
	t.Log("  2. Run test via: vsp execute /tmp/wasm_suite_test.abap")
	t.Log("  3. Or self-host: upload wasm_suite.wasm, compile with zcl_wasm_codegen, run")
}

// TestWASMSuite_WriteBinary writes the WASM binary for use with the self-hosting compiler.
func TestWASMSuite_WriteBinary(t *testing.T) {
	wasm := buildSuiteWasm()
	t.Logf("Suite WASM binary: %d bytes, %d functions", len(wasm), 5)
	t.Logf("Functions: add(i32,i32)->i32, factorial(i32)->i32, fibonacci(i32)->i32, gcd(i32,i32)->i32, is_prime(i32)->i32")
	t.Logf("")
	t.Logf("Test cases: %d", len(suiteTestCases))
	for _, tc := range suiteTestCases {
		t.Logf("  %-20s → %d", tc.Name, tc.Expected)
	}

	// Write hex dump for ABAP self-host (can be loaded via XSTRING)
	var hex strings.Builder
	for i, b := range wasm {
		if i > 0 && i%32 == 0 {
			hex.WriteByte('\n')
		}
		fmt.Fprintf(&hex, "%02X", b)
	}
	os.WriteFile("/tmp/wasm_suite.hex", []byte(hex.String()), 0644)
	t.Logf("Hex dump: /tmp/wasm_suite.hex (%d bytes hex)", hex.Len())
}
