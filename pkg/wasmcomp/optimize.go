package wasmcomp

import (
	"crypto/sha256"
	"fmt"
)

// DeduplicateFunctions finds functions with identical type signatures and bytecode,
// and redirects duplicates to a single canonical implementation.
// Returns a redirect map: funcIndex → canonicalFuncIndex.
func DeduplicateFunctions(mod *Module) map[int]int {
	type funcKey struct {
		typeIndex int
		codeHash  [32]byte
	}

	canonical := make(map[funcKey]int)  // key → first function index (local, not absolute)
	redirects := make(map[int]int)       // duplicate local index → canonical local index
	dupeCount := 0
	savedInstructions := 0

	for i := range mod.Functions {
		f := &mod.Functions[i]
		if f.Type == nil || len(f.Code) == 0 {
			continue
		}

		// Hash the function body: type + locals + instructions
		h := sha256.New()
		// Include type signature
		fmt.Fprintf(h, "type:%d:", f.TypeIndex)
		// Include locals
		for _, l := range f.Locals {
			fmt.Fprintf(h, "l:%d:", l)
		}
		// Include instruction opcodes and immediates
		for _, inst := range f.Code {
			fmt.Fprintf(h, "%02x:", inst.Op)
			// Include significant immediates
			switch inst.Op {
			case OpI32Const:
				fmt.Fprintf(h, "i32:%d:", inst.I32Value)
			case OpI64Const:
				fmt.Fprintf(h, "i64:%d:", inst.I64Value)
			case OpF32Const:
				fmt.Fprintf(h, "f32:%v:", inst.F32Value)
			case OpF64Const:
				fmt.Fprintf(h, "f64:%v:", inst.F64Value)
			case OpLocalGet, OpLocalSet, OpLocalTee:
				fmt.Fprintf(h, "loc:%d:", inst.LocalIndex)
			case OpGlobalGet, OpGlobalSet:
				fmt.Fprintf(h, "glob:%d:", inst.GlobalIndex)
			case OpCall:
				fmt.Fprintf(h, "call:%d:", inst.FuncIndex)
			case OpCallIndirect:
				fmt.Fprintf(h, "ci:%d:%d:", inst.TypeIndex, inst.TableIndex)
			case OpBr, OpBrIf:
				fmt.Fprintf(h, "br:%d:", inst.LabelIndex)
			case OpBlock, OpLoop, OpIf:
				fmt.Fprintf(h, "bt:%d:", inst.BlockType)
			case OpI32Load, OpI64Load, OpF32Load, OpF64Load,
				OpI32Store, OpI64Store, OpF32Store, OpF64Store,
				OpI32Load8S, OpI32Load8U, OpI32Load16S, OpI32Load16U,
				OpI32Store8, OpI32Store16,
				OpI64Load8S, OpI64Load8U, OpI64Load16S, OpI64Load16U,
				OpI64Load32S, OpI64Load32U,
				OpI64Store8, OpI64Store16, OpI64Store32:
				fmt.Fprintf(h, "mem:%d:%d:", inst.Align, inst.Offset)
			case OpBrTable:
				fmt.Fprintf(h, "brt:%d:", inst.DefaultLabel)
				for _, l := range inst.Labels {
					fmt.Fprintf(h, "%d:", l)
				}
			case OpMiscPrefix:
				fmt.Fprintf(h, "misc:%d:", inst.MiscOp)
			}
		}

		var hash [32]byte
		copy(hash[:], h.Sum(nil))
		key := funcKey{typeIndex: f.TypeIndex, codeHash: hash}

		if canonIdx, exists := canonical[key]; exists {
			// This function is a duplicate
			redirects[i] = canonIdx
			dupeCount++
			savedInstructions += len(f.Code)
		} else {
			canonical[key] = i
		}
	}

	return redirects
}

// DedupStats returns dedup statistics for logging.
func DedupStats(mod *Module, redirects map[int]int) (dupes, totalFuncs, savedInstrs int) {
	totalFuncs = len(mod.Functions)
	dupes = len(redirects)
	for i := range redirects {
		savedInstrs += len(mod.Functions[i].Code)
	}
	return
}
