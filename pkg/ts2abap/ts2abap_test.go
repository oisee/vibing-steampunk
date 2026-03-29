package ts2abap

import (
	"os"
	"os/exec"
	"testing"
)

func TestTranspileLexer(t *testing.T) {
	// Step 1: Parse TS to JSON AST via node
	cmd := exec.Command("node", "ts_ast.js", "testdata/lexer.ts")
	astJSON, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to parse TS: %v (install: npm install typescript)", err)
	}
	t.Logf("AST JSON: %d bytes", len(astJSON))

	// Step 2: Transpile to ABAP
	result, err := Transpile(astJSON, "zcl_")
	if err != nil {
		t.Fatalf("Transpile failed: %v", err)
	}

	t.Logf("Generated %d ABAP classes:", len(result.Classes))
	for name, src := range result.Classes {
		lines := 0
		for _, c := range src {
			if c == '\n' {
				lines++
			}
		}
		t.Logf("  %s: %d lines", name, lines)
		t.Logf("\n%s", src)

		// Write to file
		os.MkdirAll("testdata/abap_out", 0755)
		os.WriteFile("testdata/abap_out/"+name+".clas.abap", []byte(src), 0644)
	}
}
