package wasmcomp

import (
	"fmt"
	"os"
	"testing"
)

func TestParseQuickJS(t *testing.T) {
	data, err := os.ReadFile("testdata/quickjs_eval.wasm")
	if err != nil {
		t.Skipf("QuickJS WASM not found: %v (run: npx javy-cli compile test.js -o testdata/quickjs_eval.wasm)", err)
	}

	t.Logf("WASM binary size: %d bytes (%.1f KB)", len(data), float64(len(data))/1024)

	mod, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	t.Logf("Types:     %d", len(mod.Types))
	t.Logf("Imports:   %d", len(mod.Imports))
	t.Logf("Functions: %d", len(mod.Functions))
	t.Logf("Exports:   %d", len(mod.Exports))
	t.Logf("Globals:   %d", len(mod.Globals))
	if mod.Memory != nil {
		t.Logf("Memory:    %d pages min (%d KB)", mod.Memory.Min, mod.Memory.Min*64)
	}
	t.Logf("Data segs: %d", len(mod.Data))
	t.Logf("Elements:  %d", len(mod.Elements))
	t.Logf("Imported funcs: %d", mod.NumImportedFuncs)

	// List imports
	t.Log("\n--- Imports ---")
	for _, imp := range mod.Imports {
		kindStr := "func"
		switch imp.Kind {
		case 1:
			kindStr = "table"
		case 2:
			kindStr = "memory"
		case 3:
			kindStr = "global"
		}
		t.Logf("  %s.%s (%s)", imp.Module, imp.Name, kindStr)
	}

	// List exports
	t.Log("\n--- Exports ---")
	for _, exp := range mod.Exports {
		kindStr := "func"
		switch exp.Kind {
		case 1:
			kindStr = "table"
		case 2:
			kindStr = "memory"
		case 3:
			kindStr = "global"
		}
		t.Logf("  %s (%s, index=%d)", exp.Name, kindStr, exp.Index)
	}

	// Count opcodes used across all functions
	opcodeCounts := make(map[byte]int)
	totalInstructions := 0
	functionsWithCode := 0
	for _, f := range mod.Functions {
		if len(f.Code) > 0 {
			functionsWithCode++
			for _, inst := range f.Code {
				opcodeCounts[inst.Op]++
				totalInstructions++
			}
		}
	}

	t.Logf("\nFunctions with code: %d", functionsWithCode)
	t.Logf("Total instructions: %d", totalInstructions)
	t.Logf("Unique opcodes used: %d", len(opcodeCounts))

	// Check how many opcodes we handle
	handled := 0
	unhandled := 0
	unhandledList := make(map[byte]int)
	for op, count := range opcodeCounts {
		if isHandled(op) {
			handled += count
		} else {
			unhandled += count
			unhandledList[op] = count
		}
	}

	t.Logf("\nHandled instructions:   %d (%.1f%%)", handled, 100*float64(handled)/float64(totalInstructions))
	t.Logf("Unhandled instructions: %d (%.1f%%)", unhandled, 100*float64(unhandled)/float64(totalInstructions))

	if len(unhandledList) > 0 {
		t.Log("\n--- Unhandled opcodes ---")
		for op, count := range unhandledList {
			t.Logf("  0x%02X: %d occurrences", op, count)
		}
	}

	// Try to compile (may panic on unhandled opcodes — recover)
	t.Log("\n--- Attempting compilation ---")
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Compilation panicked (expected for incomplete compiler): %v", r)
			}
		}()

		abap := Compile(mod, "zcl_quickjs")
		t.Logf("Generated ABAP size: %d bytes (%.1f KB)", len(abap), float64(len(abap))/1024)

		// Write to file for inspection
		outPath := "testdata/quickjs_eval.clas.abap"
		if err := os.WriteFile(outPath, []byte(abap), 0644); err != nil {
			t.Logf("Failed to write output: %v", err)
		} else {
			t.Logf("Written to %s", outPath)
		}
	}()
}

// isHandled returns true if our codegen handles this opcode.
func isHandled(op byte) bool {
	switch op {
	case OpNop, OpUnreachable,
		OpI32Const, OpI64Const, OpF32Const, OpF64Const,
		OpLocalGet, OpLocalSet, OpLocalTee,
		OpGlobalGet, OpGlobalSet,
		OpI32Add, OpI32Sub, OpI32Mul, OpI32DivS, OpI32DivU,
		OpI32RemS, OpI32RemU,
		OpI32And, OpI32Or, OpI32Xor, OpI32Shl, OpI32ShrS, OpI32ShrU,
		OpI32Rotl, OpI32Rotr, OpI32Clz, OpI32Ctz, OpI32Popcnt,
		OpI32Eqz, OpI32Eq, OpI32Ne,
		OpI32LtS, OpI32LtU, OpI32GtS, OpI32GtU,
		OpI32LeS, OpI32LeU, OpI32GeS, OpI32GeU,
		OpI64Add, OpI64Sub, OpI64Mul, OpI64DivS, OpI64DivU,
		OpI64RemS, OpI64RemU,
		OpI64And, OpI64Or, OpI64Xor, OpI64Shl, OpI64ShrS, OpI64ShrU,
		OpI64Rotl, OpI64Rotr, OpI64Clz, OpI64Ctz, OpI64Popcnt,
		OpI64Eqz, OpI64Eq, OpI64Ne,
		OpI64LtS, OpI64LtU, OpI64GtS, OpI64GtU,
		OpI64LeS, OpI64LeU, OpI64GeS, OpI64GeU,
		OpF64Add, OpF64Sub, OpF64Mul, OpF64Div,
		OpF64Eq, OpF64Ne, OpF64Lt, OpF64Gt, OpF64Le, OpF64Ge,
		OpF64Abs, OpF64Neg, OpF64Ceil, OpF64Floor, OpF64Sqrt, OpF64Trunc,
		OpF64Min, OpF64Max,
		OpI32Load, OpI32Store,
		OpI32Load8S, OpI32Load8U, OpI32Load16S, OpI32Load16U,
		OpI32Store8, OpI32Store16,
		OpI64Load, OpI64Store,
		OpI64Load8S, OpI64Load8U, OpI64Load16S, OpI64Load16U,
		OpI64Load32S, OpI64Load32U,
		OpI64Store8, OpI64Store16, OpI64Store32,
		OpF32Load, OpF64Load, OpF32Store, OpF64Store,
		OpMemorySize, OpMemoryGrow,
		OpBlock, OpLoop, OpIf, OpElse, OpEnd,
		OpBr, OpBrIf, OpBrTable, OpReturn,
		OpCall, OpCallIndirect,
		OpDrop, OpSelect,
		OpI32WrapI64, OpI64ExtendI32S, OpI64ExtendI32U,
		OpI32TruncF64S, OpI32TruncF64U, OpI32TruncF32S, OpI32TruncF32U,
		OpI64TruncF64S, OpI64TruncF32S,
		OpF64ConvertI32S, OpF64ConvertI64S, OpF64ConvertI32U,
		OpF64PromoteF32, OpF32DemoteF64,
		OpF32ConvertI32S, OpF32ConvertI64S, OpF32ConvertI32U, OpF32ConvertI64U,
		OpI32ReinterpretF32, OpF32ReinterpretI32,
		OpI64ReinterpretF64, OpF64ReinterpretI64,
		OpI32Extend8S, OpI32Extend16S,
		OpI64Extend8S, OpI64Extend16S, OpI64Extend32S,
		OpI64TruncF64U, OpI64TruncF32U,
		OpF64ConvertI64U,
		OpF32Eq, OpF32Ne, OpF32Lt, OpF32Gt, OpF32Le, OpF32Ge,
		OpF32Add, OpF32Sub, OpF32Mul, OpF32Div,
		OpF32Abs, OpF32Neg, OpF32Sqrt, OpF32Min, OpF32Max,
		OpF32Ceil, OpF32Floor, OpF32Trunc, OpF32Nearest, OpF32Copysign,
		OpF64Copysign, OpF64Nearest,
		OpSelectT,
		OpTry, OpCatch, OpCatchAll, OpThrow, OpRethrow, OpDelegate,
		OpReturnCall, OpReturnCallIndirect,
		OpMiscPrefix, OpSIMDPrefix:
		return true
	default:
		return false
	}
}

// Ensure fmt is used
var _ = fmt.Sprintf
