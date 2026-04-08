package llvm2abap

import (
	"os"
	"strings"
	"testing"
)

func TestParseAndCompileCorpus(t *testing.T) {
	src, err := os.ReadFile("testdata/corpus.ll")
	if err != nil {
		t.Fatalf("Failed to read corpus.ll: %v", err)
	}

	mod, err := Parse(string(src))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	t.Logf("Parsed: %d functions, %d struct types", len(mod.Functions), len(mod.Types))

	// Check expected functions
	expected := []string{"add", "sub", "mul", "div_s", "rem_s", "negate", "identity",
		"and_", "or_", "xor_", "shl", "shr", "quadratic",
		"fadd", "fmul", "add64",
		"abs_val", "max", "min", "clamp", "sign",
		"sum_to", "factorial", "fibonacci", "gcd", "is_prime",
		"double_val", "square", "cube", "factorial_rec", "fib_rec",
		"point_sum", "point_set", "array_sum"}

	funcNames := make(map[string]bool)
	for _, fn := range mod.Functions {
		funcNames[fn.Name] = true
	}

	for _, name := range expected {
		if !funcNames[name] {
			t.Errorf("Missing function: %s", name)
		}
	}

	// Check struct type
	if _, ok := mod.Types["struct.Point"]; !ok {
		t.Error("Missing struct type: struct.Point")
	}

	// Compile to ABAP
	abap := Compile(mod, "zcl_corpus")
	t.Logf("Generated ABAP: %d bytes", len(abap))

	// Verify structure
	checks := []string{
		"CLASS zcl_corpus DEFINITION",
		"CLASS zcl_corpus IMPLEMENTATION",
		"CLASS-METHODS add IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i",
		"CLASS-METHODS factorial IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i",
		"CLASS-METHODS fibonacci IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i",
		"CLASS-METHODS fadd IMPORTING a TYPE f b TYPE f RETURNING VALUE(rv) TYPE f",
		"CLASS-METHODS add64 IMPORTING a TYPE int8 b TYPE int8 RETURNING VALUE(rv) TYPE int8",
		"METHOD add.",
		"METHOD factorial.",
		"ENDCLASS.",
	}
	for _, check := range checks {
		if !strings.Contains(abap, check) {
			t.Errorf("Missing in output: %q", check)
		}
	}

	// Verify leaf function quality (LLVM may swap operand order)
	if !strings.Contains(abap, "b + a") && !strings.Contains(abap, "a + b") {
		t.Error("add() should produce addition of a and b")
	}

	t.Log("\n" + abap)
}

func TestLeafFunctions(t *testing.T) {
	ir := `
define i32 @add(i32 %0, i32 %1) {
  %3 = add i32 %1, %0
  ret i32 %3
}

define i32 @negate(i32 %0) {
  %2 = sub i32 0, %0
  ret i32 %2
}

define double @fmul(double %0, double %1) {
  %3 = fmul double %0, %1
  ret double %3
}
`
	mod, err := Parse(ir)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(mod.Functions) != 3 {
		t.Fatalf("Expected 3 functions, got %d", len(mod.Functions))
	}

	abap := Compile(mod, "zcl_leaf")
	t.Log(abap)

	// Check typed signatures
	if !strings.Contains(abap, "IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i") {
		t.Error("add should have typed i32 signature")
	}
	if !strings.Contains(abap, "IMPORTING a TYPE f b TYPE f RETURNING VALUE(rv) TYPE f") {
		t.Error("fmul should have typed double signature")
	}
	// Check clean body (may use temp var: lv_3 = b + a. rv = lv_3.)
	if !strings.Contains(abap, "b + a") && !strings.Contains(abap, "a + b") {
		t.Error("add body should contain addition")
	}
}

func TestControlFlow(t *testing.T) {
	ir := `
define i32 @sign(i32 %0) {
  %2 = ashr i32 %0, 31
  %3 = icmp slt i32 %0, 1
  %4 = select i1 %3, i32 %2, i32 1
  ret i32 %4
}
`
	mod, _ := Parse(ir)
	abap := Compile(mod, "zcl_cf")
	t.Log(abap)

	if !strings.Contains(abap, "IF") && !strings.Contains(abap, "select") {
		t.Error("sign should use IF or select")
	}
}
