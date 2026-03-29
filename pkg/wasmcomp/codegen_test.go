package wasmcomp

import (
	"strings"
	"testing"
)

func buildTestWasm() []byte {
	w := newWasmBuilder()

	// Type section: (i32,i32)->i32 and (i32)->i32
	w.addSection(1, buildTypeSection([]FuncType{
		{Params: []ValType{ValI32, ValI32}, Results: []ValType{ValI32}},
		{Params: []ValType{ValI32}, Results: []ValType{ValI32}},
	}))

	// Function section: func0=type0, func1=type1
	w.addSection(3, buildFuncSection([]int{0, 1}))

	// Export section
	w.addSection(7, buildExportSection([]Export{
		{Name: "add", Kind: 0, Index: 0},
		{Name: "factorial", Kind: 0, Index: 1},
	}))

	// Code section
	// add(a, b) = a + b
	addBody := buildFuncBody(nil, []byte{
		OpLocalGet, 0x00, // local.get 0
		OpLocalGet, 0x01, // local.get 1
		OpI32Add,         // i32.add
	})

	// factorial(n) = if n<=1 then 1 else n*factorial(n-1)
	factBody := buildFuncBody(nil, []byte{
		OpLocalGet, 0x00, // local.get 0
		OpI32Const, 0x01, // i32.const 1
		OpI32LeS,         // i32.le_s
		OpIf, 0x7F,       // if (result i32)
		OpI32Const, 0x01, // i32.const 1
		OpElse,           // else
		OpLocalGet, 0x00, // local.get 0
		OpLocalGet, 0x00, // local.get 0
		OpI32Const, 0x01, // i32.const 1
		OpI32Sub,         // i32.sub
		OpCall, 0x01,     // call 1 (factorial)
		OpI32Mul,         // i32.mul
		OpEnd,            // end if
	})

	w.addSection(10, buildCodeSection([][]byte{addBody, factBody}))

	return w.bytes()
}

func TestParseAndCompile(t *testing.T) {
	mod, err := Parse(buildTestWasm())
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(mod.Types) != 2 {
		t.Errorf("Expected 2 types, got %d", len(mod.Types))
	}
	if len(mod.Functions) != 2 {
		t.Errorf("Expected 2 functions, got %d", len(mod.Functions))
	}
	if len(mod.Exports) != 2 {
		t.Errorf("Expected 2 exports, got %d", len(mod.Exports))
	}

	if mod.Functions[0].ExportName != "add" {
		t.Errorf("Expected export 'add', got '%s'", mod.Functions[0].ExportName)
	}
	if mod.Functions[1].ExportName != "factorial" {
		t.Errorf("Expected export 'factorial', got '%s'", mod.Functions[1].ExportName)
	}

	// Compile to ABAP
	abap := Compile(mod, "zcl_wasm_test")
	t.Logf("Generated ABAP (%d bytes):\n%s", len(abap), abap)

	// Verify structure
	checks := []string{
		"CLASS zcl_wasm_test DEFINITION",
		"CLASS zcl_wasm_test IMPLEMENTATION",
		"METHOD add.",
		"METHOD factorial.",
		"ENDCLASS.",
		"s0 + s1", // add: a + b
		"factorial(",    // recursive call
	}
	for _, check := range checks {
		if !strings.Contains(abap, check) {
			t.Errorf("Missing in output: %q", check)
		}
	}
}
