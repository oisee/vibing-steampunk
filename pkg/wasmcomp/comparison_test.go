package wasmcomp

import (
	"os"
	"testing"
)

func TestCompareCompilers(t *testing.T) {
	files := map[string]string{
		"Javy/QuickJS":     "/tmp/cmp_javy.wasm",
		"porffor (JS)":     "/tmp/cmp_porffor.wasm",
		"porffor (TS)":     "/tmp/test_ts.wasm",
		"AssemblyScript":   "/tmp/as-test/test_as.wasm",
		"ABAP tokenizer":   "/tmp/abap_tokenizer.wasm",
	}

	t.Log("=== WASM Compiler Comparison ===")
	t.Logf("%-20s %8s %7s %8s %10s", "Compiler", "WASM", "Funcs", "Instrs", "ABAP Lines")
	t.Logf("%-20s %8s %7s %8s %10s", "--------", "----", "-----", "------", "----------")

	for name, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Logf("%-20s SKIP (file not found)", name)
			continue
		}

		mod, err := Parse(data)
		if err != nil {
			t.Logf("%-20s PARSE ERROR: %v", name, err)
			continue
		}

		totalInstrs := 0
		for _, f := range mod.Functions {
			totalInstrs += len(f.Code)
		}

		abap := Compile(mod, "zcl_test")
		lines := 0
		for _, c := range abap {
			if c == '\n' {
				lines++
			}
		}

		t.Logf("%-20s %7dB %5d %8d %10d", name, len(data), len(mod.Functions), totalInstrs, lines)
	}
}
