package wasmcomp

import (
	"os"
	"testing"
)

func TestParseAbaplint(t *testing.T) {
	data, err := os.ReadFile("testdata/abaplint.wasm")
	if err != nil {
		t.Skipf("abaplint WASM not found: %v", err)
	}

	t.Logf("WASM binary size: %d bytes (%.1f MB)", len(data), float64(len(data))/1024/1024)

	mod, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	t.Logf("Types:     %d", len(mod.Types))
	t.Logf("Imports:   %d (funcs: %d)", len(mod.Imports), mod.NumImportedFuncs)
	t.Logf("Functions: %d", len(mod.Functions))
	t.Logf("Exports:   %d", len(mod.Exports))
	t.Logf("Globals:   %d", len(mod.Globals))
	if mod.Memory != nil {
		t.Logf("Memory:    %d pages min (%d MB)", mod.Memory.Min, mod.Memory.Min*64/1024)
	}
	t.Logf("Data segs: %d", len(mod.Data))
	t.Logf("Elements:  %d", len(mod.Elements))

	// Count instructions
	totalInstructions := 0
	for _, f := range mod.Functions {
		totalInstructions += len(f.Code)
	}
	t.Logf("Total instructions: %d", totalInstructions)

	// Try FUGR compilation
	t.Log("\n--- Compiling to ABAP (FUGR backend) ---")
	result := CompileWith(mod, "zabaplint", BackendFUGR, 100)

	t.Logf("Files:          %d", result.Stats.FileCount)
	t.Logf("Total lines:    %d", result.Stats.TotalLines)
	t.Logf("Largest file:   %s (%d lines)", result.Stats.LargestFileName, result.Stats.LargestFile)
	t.Logf("Duplicates:     %d / %d (%.1f%%)", result.Stats.DuplicateFunctions, result.Stats.TotalFunctions,
		100*float64(result.Stats.DuplicateFunctions)/float64(result.Stats.TotalFunctions))
	t.Logf("Saved instrs:   %d", result.Stats.SavedInstructions)

	// Write output
	outDir := "testdata/abaplint_fugr"
	os.MkdirAll(outDir, 0755)
	for fname, src := range result.Files {
		os.WriteFile(outDir+"/"+fname, []byte(src), 0644)
	}
	t.Logf("Written to %s/", outDir)
}
