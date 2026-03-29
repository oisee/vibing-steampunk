package wasmcomp

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
)

// buildSingleFuncWasm builds a minimal WASM binary with one exported function.
func buildSingleFuncWasm(name string, ft FuncType, locals []ValType, code []byte) []byte {
	w := newWasmBuilder()
	w.addSection(1, buildTypeSection([]FuncType{ft}))
	w.addSection(3, buildFuncSection([]int{0}))
	w.addSection(7, buildExportSection([]Export{{Name: name, Kind: 0, Index: 0}}))
	body := buildFuncBody(locals, code)
	w.addSection(10, buildCodeSection([][]byte{body}))
	return w.bytes()
}

// i32_i32 is shorthand for (i32,i32)->i32 function type.
var i32_i32 = FuncType{Params: []ValType{ValI32, ValI32}, Results: []ValType{ValI32}}

// i32_ret is shorthand for (i32)->i32 function type.
var i32_ret = FuncType{Params: []ValType{ValI32}, Results: []ValType{ValI32}}

func buildAddWasm() []byte {
	return buildSingleFuncWasm("add", i32_i32, nil, []byte{
		OpLocalGet, 0x00, OpLocalGet, 0x01, OpI32Add,
	})
}

func buildFactorialWasm() []byte {
	return buildSingleFuncWasm("factorial", i32_ret, nil, []byte{
		OpLocalGet, 0x00, OpI32Const, 0x01, OpI32LeS,
		OpIf, 0x7F, // result i32
		OpI32Const, 0x01,
		OpElse,
		OpLocalGet, 0x00,
		OpLocalGet, 0x00, OpI32Const, 0x01, OpI32Sub,
		OpCall, 0x00, // call self
		OpI32Mul,
		OpEnd,
	})
}

func buildFibonacciWasm() []byte {
	return buildSingleFuncWasm("fibonacci", i32_ret, nil, []byte{
		OpLocalGet, 0x00, OpI32Const, 0x01, OpI32LeS,
		OpIf, 0x7F,
		OpLocalGet, 0x00,
		OpElse,
		OpLocalGet, 0x00, OpI32Const, 0x01, OpI32Sub, OpCall, 0x00,
		OpLocalGet, 0x00, OpI32Const, 0x02, OpI32Sub, OpCall, 0x00,
		OpI32Add,
		OpEnd,
	})
}

func buildAbsWasm() []byte {
	return buildSingleFuncWasm("abs", i32_ret, nil, []byte{
		OpLocalGet, 0x00, OpI32Const, 0x00, OpI32LtS,
		OpIf, 0x7F,
		OpI32Const, 0x00, OpLocalGet, 0x00, OpI32Sub,
		OpElse,
		OpLocalGet, 0x00,
		OpEnd,
	})
}

func buildMaxWasm() []byte {
	return buildSingleFuncWasm("max", i32_i32, nil, []byte{
		OpLocalGet, 0x00, OpLocalGet, 0x01, OpI32GtS,
		OpIf, 0x7F,
		OpLocalGet, 0x00,
		OpElse,
		OpLocalGet, 0x01,
		OpEnd,
	})
}

func buildMinWasm() []byte {
	return buildSingleFuncWasm("min", i32_i32, nil, []byte{
		OpLocalGet, 0x00, OpLocalGet, 0x01, OpI32LtS,
		OpIf, 0x7F,
		OpLocalGet, 0x00,
		OpElse,
		OpLocalGet, 0x01,
		OpEnd,
	})
}

func buildNegateWasm() []byte {
	return buildSingleFuncWasm("negate", i32_ret, nil, []byte{
		OpI32Const, 0x00, OpLocalGet, 0x00, OpI32Sub,
	})
}

func buildSumToWasm() []byte {
	// sum_to(n): sum = 0, i = 1; while i <= n: sum += i, i++; return sum
	return buildSingleFuncWasm("sum_to", i32_ret, []ValType{ValI32, ValI32}, []byte{
		OpI32Const, 0x00, OpLocalSet, 0x01, // sum = 0
		OpI32Const, 0x01, OpLocalSet, 0x02, // i = 1
		OpBlock, 0x40, // block (void)
		OpLoop, 0x40, // loop (void)
		OpLocalGet, 0x02, OpLocalGet, 0x00, OpI32GtS, // i > n?
		OpBrIf, 0x01, // br_if 1 → break block
		OpLocalGet, 0x01, OpLocalGet, 0x02, OpI32Add, OpLocalSet, 0x01, // sum += i
		OpLocalGet, 0x02, OpI32Const, 0x01, OpI32Add, OpLocalSet, 0x02, // i++
		OpBr, 0x00, // br 0 → continue loop
		OpEnd, // end loop
		OpEnd, // end block
		OpLocalGet, 0x01, // return sum
	})
}

func buildCollatzWasm() []byte {
	// collatz(n): count steps until n reaches 1
	return buildSingleFuncWasm("collatz", i32_ret, []ValType{ValI32}, []byte{
		OpI32Const, 0x00, OpLocalSet, 0x01, // count = 0
		OpBlock, 0x40,
		OpLoop, 0x40,
		OpLocalGet, 0x00, OpI32Const, 0x01, OpI32LeS, // n <= 1?
		OpBrIf, 0x01, // break
		OpLocalGet, 0x01, OpI32Const, 0x01, OpI32Add, OpLocalSet, 0x01, // count++
		OpLocalGet, 0x00, OpI32Const, 0x02, OpI32RemS, OpI32Eqz, // n%2 == 0?
		OpIf, 0x40, // void
		OpLocalGet, 0x00, OpI32Const, 0x02, OpI32DivS, OpLocalSet, 0x00, // n /= 2
		OpElse,
		OpI32Const, 0x03, OpLocalGet, 0x00, OpI32Mul, OpI32Const, 0x01, OpI32Add, OpLocalSet, 0x00, // n = 3n+1
		OpEnd,
		OpBr, 0x00, // continue
		OpEnd, OpEnd,
		OpLocalGet, 0x01, // return count
	})
}

func buildPow2Wasm() []byte {
	// pow2(n): compute 2^n via loop
	return buildSingleFuncWasm("pow2", i32_ret, []ValType{ValI32, ValI32}, []byte{
		OpI32Const, 0x01, OpLocalSet, 0x01, // result = 1
		OpI32Const, 0x00, OpLocalSet, 0x02, // i = 0
		OpBlock, 0x40,
		OpLoop, 0x40,
		OpLocalGet, 0x02, OpLocalGet, 0x00, OpI32GeS, // i >= n?
		OpBrIf, 0x01,
		OpLocalGet, 0x01, OpI32Const, 0x02, OpI32Mul, OpLocalSet, 0x01, // result *= 2
		OpLocalGet, 0x02, OpI32Const, 0x01, OpI32Add, OpLocalSet, 0x02, // i++
		OpBr, 0x00,
		OpEnd, OpEnd,
		OpLocalGet, 0x01, // return result
	})
}

// buildExtendedSuiteWasm builds a WASM module with 12 test functions.
func buildExtendedSuiteWasm() []byte {
	w := newWasmBuilder()

	// Types
	w.addSection(1, buildTypeSection([]FuncType{
		{Params: []ValType{ValI32, ValI32}, Results: []ValType{ValI32}}, // type 0: (i32,i32)->i32
		{Params: []ValType{ValI32}, Results: []ValType{ValI32}},        // type 1: (i32)->i32
	}))

	// Functions: 12 funcs
	// 0:add 1:factorial 2:fibonacci 3:gcd 4:is_prime 5:abs 6:max 7:min 8:negate 9:sum_to 10:collatz 11:pow2
	w.addSection(3, buildFuncSection([]int{0, 1, 1, 0, 1, 1, 0, 0, 1, 1, 1, 1}))

	// Exports
	w.addSection(7, buildExportSection([]Export{
		{Name: "add", Kind: 0, Index: 0},
		{Name: "factorial", Kind: 0, Index: 1},
		{Name: "fibonacci", Kind: 0, Index: 2},
		{Name: "gcd", Kind: 0, Index: 3},
		{Name: "is_prime", Kind: 0, Index: 4},
		{Name: "abs", Kind: 0, Index: 5},
		{Name: "max", Kind: 0, Index: 6},
		{Name: "min", Kind: 0, Index: 7},
		{Name: "negate", Kind: 0, Index: 8},
		{Name: "sum_to", Kind: 0, Index: 9},
		{Name: "collatz", Kind: 0, Index: 10},
		{Name: "pow2", Kind: 0, Index: 11},
	}))

	// Code section - all 12 function bodies
	bodies := [][]byte{
		// 0: add
		buildFuncBody(nil, []byte{OpLocalGet, 0x00, OpLocalGet, 0x01, OpI32Add}),
		// 1: factorial (calls self = index 1)
		buildFuncBody(nil, []byte{
			OpLocalGet, 0x00, OpI32Const, 0x01, OpI32LeS,
			OpIf, 0x7F, OpI32Const, 0x01, OpElse,
			OpLocalGet, 0x00, OpLocalGet, 0x00, OpI32Const, 0x01, OpI32Sub,
			OpCall, 0x01, OpI32Mul, OpEnd,
		}),
		// 2: fibonacci (calls self = index 2)
		buildFuncBody(nil, []byte{
			OpLocalGet, 0x00, OpI32Const, 0x01, OpI32LeS,
			OpIf, 0x7F, OpLocalGet, 0x00, OpElse,
			OpLocalGet, 0x00, OpI32Const, 0x01, OpI32Sub, OpCall, 0x02,
			OpLocalGet, 0x00, OpI32Const, 0x02, OpI32Sub, OpCall, 0x02,
			OpI32Add, OpEnd,
		}),
		// 3: gcd (calls self = index 3)
		buildFuncBody(nil, []byte{
			OpLocalGet, 0x01, OpI32Eqz,
			OpIf, 0x7F, OpLocalGet, 0x00, OpElse,
			OpLocalGet, 0x01, OpLocalGet, 0x00, OpLocalGet, 0x01, OpI32RemS,
			OpCall, 0x03, OpEnd,
		}),
		// 4: is_prime (same bytecode as in wasm_suite_test.go)
		buildFuncBody([]ValType{ValI32}, []byte{
			0x20, 0x00, 0x41, 0x02, 0x48, 0x04, 0x40, 0x41, 0x00, 0x0f, 0x0b,
			0x41, 0x02, 0x21, 0x01,
			0x02, 0x40, 0x03, 0x40,
			0x20, 0x01, 0x20, 0x01, 0x6c, 0x20, 0x00, 0x4a, 0x0d, 0x01,
			0x20, 0x00, 0x20, 0x01, 0x6f, 0x45, 0x04, 0x40, 0x41, 0x00, 0x0f, 0x0b,
			0x20, 0x01, 0x41, 0x01, 0x6a, 0x21, 0x01, 0x0c, 0x00,
			0x0b, 0x0b, 0x41, 0x01,
		}),
		// 5: abs
		buildFuncBody(nil, []byte{
			OpLocalGet, 0x00, OpI32Const, 0x00, OpI32LtS,
			OpIf, 0x7F, OpI32Const, 0x00, OpLocalGet, 0x00, OpI32Sub,
			OpElse, OpLocalGet, 0x00, OpEnd,
		}),
		// 6: max
		buildFuncBody(nil, []byte{
			OpLocalGet, 0x00, OpLocalGet, 0x01, OpI32GtS,
			OpIf, 0x7F, OpLocalGet, 0x00, OpElse, OpLocalGet, 0x01, OpEnd,
		}),
		// 7: min
		buildFuncBody(nil, []byte{
			OpLocalGet, 0x00, OpLocalGet, 0x01, OpI32LtS,
			OpIf, 0x7F, OpLocalGet, 0x00, OpElse, OpLocalGet, 0x01, OpEnd,
		}),
		// 8: negate
		buildFuncBody(nil, []byte{OpI32Const, 0x00, OpLocalGet, 0x00, OpI32Sub}),
		// 9: sum_to (locals: sum, i)
		buildFuncBody([]ValType{ValI32, ValI32}, []byte{
			OpI32Const, 0x00, OpLocalSet, 0x01,
			OpI32Const, 0x01, OpLocalSet, 0x02,
			OpBlock, 0x40, OpLoop, 0x40,
			OpLocalGet, 0x02, OpLocalGet, 0x00, OpI32GtS, OpBrIf, 0x01,
			OpLocalGet, 0x01, OpLocalGet, 0x02, OpI32Add, OpLocalSet, 0x01,
			OpLocalGet, 0x02, OpI32Const, 0x01, OpI32Add, OpLocalSet, 0x02,
			OpBr, 0x00, OpEnd, OpEnd,
			OpLocalGet, 0x01,
		}),
		// 10: collatz (locals: count)
		buildFuncBody([]ValType{ValI32}, []byte{
			OpI32Const, 0x00, OpLocalSet, 0x01,
			OpBlock, 0x40, OpLoop, 0x40,
			OpLocalGet, 0x00, OpI32Const, 0x01, OpI32LeS, OpBrIf, 0x01,
			OpLocalGet, 0x01, OpI32Const, 0x01, OpI32Add, OpLocalSet, 0x01,
			OpLocalGet, 0x00, OpI32Const, 0x02, OpI32RemS, OpI32Eqz,
			OpIf, 0x40,
			OpLocalGet, 0x00, OpI32Const, 0x02, OpI32DivS, OpLocalSet, 0x00,
			OpElse,
			OpI32Const, 0x03, OpLocalGet, 0x00, OpI32Mul, OpI32Const, 0x01, OpI32Add, OpLocalSet, 0x00,
			OpEnd,
			OpBr, 0x00, OpEnd, OpEnd,
			OpLocalGet, 0x01,
		}),
		// 11: pow2 (locals: result, i)
		buildFuncBody([]ValType{ValI32, ValI32}, []byte{
			OpI32Const, 0x01, OpLocalSet, 0x01,
			OpI32Const, 0x00, OpLocalSet, 0x02,
			OpBlock, 0x40, OpLoop, 0x40,
			OpLocalGet, 0x02, OpLocalGet, 0x00, OpI32GeS, OpBrIf, 0x01,
			OpLocalGet, 0x01, OpI32Const, 0x02, OpI32Mul, OpLocalSet, 0x01,
			OpLocalGet, 0x02, OpI32Const, 0x01, OpI32Add, OpLocalSet, 0x02,
			OpBr, 0x00, OpEnd, OpEnd,
			OpLocalGet, 0x01,
		}),
	}

	w.addSection(10, buildCodeSection(bodies))
	return w.bytes()
}

// Extended test cases for all 12 functions.
var extendedTestCases = []WASMTestCase{
	// add
	{"add(2,3)", "add", []int32{2, 3}, 5},
	{"add(-1,1)", "add", []int32{-1, 1}, 0},
	// factorial
	{"factorial(0)", "factorial", []int32{0}, 1},
	{"factorial(5)", "factorial", []int32{5}, 120},
	{"factorial(10)", "factorial", []int32{10}, 3628800},
	// fibonacci
	{"fibonacci(0)", "fibonacci", []int32{0}, 0},
	{"fibonacci(10)", "fibonacci", []int32{10}, 55},
	// gcd
	{"gcd(12,8)", "gcd", []int32{12, 8}, 4},
	{"gcd(100,75)", "gcd", []int32{100, 75}, 25},
	// is_prime
	{"is_prime(7)", "is_prime", []int32{7}, 1},
	{"is_prime(10)", "is_prime", []int32{10}, 0},
	{"is_prime(97)", "is_prime", []int32{97}, 1},
	// abs
	{"abs(5)", "abs", []int32{5}, 5},
	{"abs(-5)", "abs", []int32{-5}, 5},
	{"abs(0)", "abs", []int32{0}, 0},
	// max
	{"max(3,7)", "max", []int32{3, 7}, 7},
	{"max(10,2)", "max", []int32{10, 2}, 10},
	// min
	{"min(3,7)", "min", []int32{3, 7}, 3},
	{"min(10,2)", "min", []int32{10, 2}, 2},
	// negate
	{"negate(5)", "negate", []int32{5}, -5},
	{"negate(-3)", "negate", []int32{-3}, 3},
	// sum_to
	{"sum_to(10)", "sum_to", []int32{10}, 55},
	{"sum_to(100)", "sum_to", []int32{100}, 5050},
	// collatz
	{"collatz(1)", "collatz", []int32{1}, 0},
	{"collatz(6)", "collatz", []int32{6}, 8},
	{"collatz(27)", "collatz", []int32{27}, 111},
	// pow2
	{"pow2(0)", "pow2", []int32{0}, 1},
	{"pow2(1)", "pow2", []int32{1}, 2},
	{"pow2(10)", "pow2", []int32{10}, 1024},
	{"pow2(16)", "pow2", []int32{16}, 65536},
}

// TestGenerateWasmFiles generates individual .wasm test binaries.
func TestGenerateWasmFiles(t *testing.T) {
	outDir := "testdata"
	os.MkdirAll(outDir, 0755)

	type wasmFile struct {
		name    string
		builder func() []byte
	}

	files := []wasmFile{
		{"add", buildAddWasm},
		{"factorial", buildFactorialWasm},
		{"fibonacci", buildFibonacciWasm},
		{"abs", buildAbsWasm},
		{"max", buildMaxWasm},
		{"min", buildMinWasm},
		{"negate", buildNegateWasm},
		{"sum_to", buildSumToWasm},
		{"collatz", buildCollatzWasm},
		{"pow2", buildPow2Wasm},
		{"suite", buildSuiteWasm},
		{"extended", buildExtendedSuiteWasm},
	}

	for _, f := range files {
		data := f.builder()
		path := fmt.Sprintf("%s/%s.wasm", outDir, f.name)
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", path, err)
		}
		t.Logf("Generated %s (%d bytes)", path, len(data))

		// Also write hex dump for ABAP (paste into SAP)
		hexStr := hex.EncodeToString(data)
		hexPath := fmt.Sprintf("%s/%s.hex", outDir, f.name)
		os.WriteFile(hexPath, []byte(strings.ToUpper(hexStr)), 0644)
	}
}

// TestExtendedSuite_Parse verifies the extended suite parses and compiles.
func TestExtendedSuite_Parse(t *testing.T) {
	wasm := buildExtendedSuiteWasm()
	t.Logf("Extended suite: %d bytes", len(wasm))

	mod, err := Parse(wasm)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(mod.Functions) != 12 {
		t.Fatalf("Expected 12 functions, got %d", len(mod.Functions))
	}

	for _, f := range mod.Functions {
		t.Logf("  %s: %d instructions", f.ExportName, len(f.Code))
	}
}

// TestExtendedSuite_Compile verifies Go compiler produces valid ABAP for all 12 functions.
func TestExtendedSuite_Compile(t *testing.T) {
	wasm := buildExtendedSuiteWasm()
	mod, err := Parse(wasm)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	abap := Compile(mod, "zcl_wasm_extended")
	t.Logf("Go compiler → %d lines ABAP", strings.Count(abap, "\n"))

	for _, name := range []string{"add", "factorial", "fibonacci", "gcd", "is_prime",
		"abs", "max", "min", "negate", "sum_to", "collatz", "pow2"} {
		if !strings.Contains(abap, "METHOD "+name+".") {
			t.Errorf("Missing METHOD %s", name)
		}
	}

	os.WriteFile("/tmp/wasm_extended.abap", []byte(abap), 0644)
	t.Log("Written to /tmp/wasm_extended.abap")
}

// TestGenerateABAPTestHarness generates a test program for SAP that exercises
// all 12 functions via GENERATE SUBROUTINE POOL + PERFORM.
func TestGenerateABAPTestHarness(t *testing.T) {
	wasm := buildExtendedSuiteWasm()

	var sb strings.Builder
	sb.WriteString("* WASM Extended Test Harness — auto-generated\n")
	sb.WriteString("* Upload extended.wasm to SMW0, run ZWASM_COMPILER to compile,\n")
	sb.WriteString("* or paste this hex into the report:\n")
	sb.WriteString(fmt.Sprintf("* HEX: %s\n\n", strings.ToUpper(hex.EncodeToString(wasm))))
	sb.WriteString("REPORT zwasm_extended_test.\n\n")
	sb.WriteString("DATA: lv_result TYPE i,\n")
	sb.WriteString("      lv_pass TYPE i,\n")
	sb.WriteString("      lv_fail TYPE i,\n")
	sb.WriteString("      lv_total TYPE i.\n\n")

	// Generate WASM hex constant
	sb.WriteString("* Load WASM binary\n")
	sb.WriteString("DATA: lv_wasm TYPE xstring.\n")
	hexStr := strings.ToUpper(hex.EncodeToString(wasm))
	sb.WriteString(fmt.Sprintf("lv_wasm = '%s'.\n\n", hexStr))

	// Parse + compile + generate
	sb.WriteString("* Parse and compile\n")
	sb.WriteString("DATA(lo_mod) = NEW zcl_wasm_module( ).\n")
	sb.WriteString("lo_mod->parse( lv_wasm ).\n")
	sb.WriteString("DATA(lv_abap) = NEW zcl_wasm_codegen( )->compile( lo_mod ).\n\n")
	sb.WriteString("* Generate subroutine pool\n")
	sb.WriteString("DATA lv_prog TYPE string.\n")
	sb.WriteString("GENERATE SUBROUTINE POOL lv_abap NAME lv_prog.\n")
	sb.WriteString("IF sy-subrc <> 0.\n")
	sb.WriteString("  WRITE: / 'GENERATE failed:', sy-subrc.\n")
	sb.WriteString("  RETURN.\n")
	sb.WriteString("ENDIF.\n\n")

	// Generate test calls
	for _, tc := range extendedTestCases {
		sb.WriteString(fmt.Sprintf("* Test: %s = %d\n", tc.Name, tc.Expected))
		args := ""
		for i, a := range tc.Args {
			if i > 0 {
				args += " "
			}
			args += fmt.Sprintf("p%d = %d", i, a)
		}
		sb.WriteString(fmt.Sprintf("PERFORM %s IN PROGRAM (lv_prog) USING %s CHANGING lv_result.\n", tc.FuncName, args))
		sb.WriteString(fmt.Sprintf("lv_total = lv_total + 1.\n"))
		sb.WriteString(fmt.Sprintf("IF lv_result = %d.\n", tc.Expected))
		sb.WriteString(fmt.Sprintf("  WRITE: / 'PASS:', '%s'.\n", tc.Name))
		sb.WriteString("  lv_pass = lv_pass + 1.\n")
		sb.WriteString("ELSE.\n")
		sb.WriteString(fmt.Sprintf("  WRITE: / 'FAIL:', '%s', 'expected %d got', lv_result.\n", tc.Name, tc.Expected))
		sb.WriteString("  lv_fail = lv_fail + 1.\n")
		sb.WriteString("ENDIF.\n\n")
	}

	sb.WriteString("WRITE: / '---'.\n")
	sb.WriteString("WRITE: / 'Result:', lv_pass, '/', lv_total.\n")
	sb.WriteString("IF lv_fail > 0. WRITE: / 'FAILURES:', lv_fail. ENDIF.\n")
	sb.WriteString("IF lv_fail = 0. WRITE: / 'ALL TESTS PASSED'. ENDIF.\n")

	testProg := sb.String()
	os.WriteFile("testdata/zwasm_extended_test.prog.abap", []byte(testProg), 0644)
	t.Logf("Generated test harness: %d lines", strings.Count(testProg, "\n"))
}
