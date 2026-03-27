package wasmcomp

import (
	"os"
	"strings"
	"testing"
)

func TestBackendFUGR_QuickJS(t *testing.T) {
	data, err := os.ReadFile("testdata/quickjs_eval.wasm")
	if err != nil {
		t.Skipf("QuickJS WASM not found: %v", err)
	}

	mod, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result := CompileWith(mod, "zqjs", BackendFUGR, 80)

	t.Logf("Backend: %s", result.Backend)
	t.Logf("Files: %d", result.Stats.FileCount)
	t.Logf("Total lines: %d", result.Stats.TotalLines)
	t.Logf("Largest file: %s (%d lines)", result.Stats.LargestFileName, result.Stats.LargestFile)
	t.Logf("Duplicates: %d / %d", result.Stats.DuplicateFunctions, result.Stats.TotalFunctions)

	// Write files
	outDir := "testdata/fugr"
	os.MkdirAll(outDir, 0755)
	for fname, src := range result.Files {
		os.WriteFile(outDir+"/"+fname, []byte(src), 0644)
	}

	t.Logf("\nFiles written to %s/:", outDir)
	for fname := range result.Files {
		t.Logf("  %s", fname)
	}

	// Verify FUGR TOP has globals
	top := result.Files["LZQJSTOP.abap"]
	if top == "" {
		t.Error("Missing TOP include")
	}
	if !containsStr(top, "gv_mem TYPE xstring") {
		t.Error("TOP missing gv_mem")
	}
	if !containsStr(top, "gv_g0") {
		t.Error("TOP missing gv_g0")
	}

	// Verify function includes use PERFORM
	for fname, src := range result.Files {
		if len(fname) > 6 && fname[:5] == "LZQJS" && fname[5] == 'F' {
			if containsStr(src, "FORM f") {
				t.Logf("  %s: has FORMs ✓", fname)
			}
			// Check it uses PERFORM for calls (not method calls)
			if containsStr(src, "PERFORM f") {
				t.Logf("  %s: uses PERFORM calls ✓", fname)
			}
			// Check global access
			if containsStr(src, "gv_g0") {
				t.Logf("  %s: accesses globals directly ✓", fname)
			}
			break // just check first include
		}
	}
}

func TestBackendHybrid_QuickJS(t *testing.T) {
	data, err := os.ReadFile("testdata/quickjs_eval.wasm")
	if err != nil {
		t.Skipf("QuickJS WASM not found: %v", err)
	}

	mod, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result := CompileWith(mod, "zcl_qjs", BackendHybrid, 80)

	t.Logf("Backend: %s", result.Backend)
	t.Logf("Files: %d", result.Stats.FileCount)
	t.Logf("Total lines: %d", result.Stats.TotalLines)

	// Verify wrapper class exists
	wrapperSrc := result.Files["zcl_qjs.clas.abap"]
	if wrapperSrc == "" {
		t.Error("Missing wrapper class")
	} else {
		t.Logf("Wrapper class: %d bytes", len(wrapperSrc))
		if containsStr(wrapperSrc, "CALL FUNCTION") {
			t.Log("  Wrapper delegates to FMs ✓")
		}
	}
}

func containsStr(s, sub string) bool {
	return strings.Contains(s, sub)
}
