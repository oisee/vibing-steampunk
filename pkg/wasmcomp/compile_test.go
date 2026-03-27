package wasmcomp

import (
	"os"
	"testing"
)

func TestCompileMultiClassQuickJS(t *testing.T) {
	data, err := os.ReadFile("testdata/quickjs_eval.wasm")
	if err != nil {
		t.Skipf("QuickJS WASM not found: %v", err)
	}

	mod, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result := CompileMultiClass(mod, "zcl_qjs", 80)

	t.Logf("Stats:")
	t.Logf("  Total functions:    %d", result.Stats.TotalFunctions)
	t.Logf("  Duplicate functions: %d (%.1f%%)", result.Stats.DuplicateFunctions,
		100*float64(result.Stats.DuplicateFunctions)/float64(result.Stats.TotalFunctions))
	t.Logf("  Saved instructions: %d", result.Stats.SavedInstructions)
	t.Logf("  Chunk count:        %d", result.Stats.ChunkCount)
	t.Logf("  Funcs per chunk:    %d", result.Stats.FuncsPerChunk)
	t.Logf("  Total lines:        %d", result.Stats.TotalLines)

	t.Logf("\nMain class: %d bytes", len(result.MainClass))
	t.Logf("Runtime class: %d bytes", len(result.RuntimeClass))

	for name, src := range result.ChunkClasses {
		t.Logf("Chunk %s: %d bytes", name, len(src))
	}

	// Write all files for inspection
	outDir := "testdata/multi"
	os.MkdirAll(outDir, 0755)

	os.WriteFile(outDir+"/zcl_qjs.clas.abap", []byte(result.MainClass), 0644)
	os.WriteFile(outDir+"/zcl_wasm_rt.clas.abap", []byte(result.RuntimeClass), 0644)
	for name, src := range result.ChunkClasses {
		os.WriteFile(outDir+"/"+name+".clas.abap", []byte(src), 0644)
	}

	t.Logf("\nWritten to %s/", outDir)
}
