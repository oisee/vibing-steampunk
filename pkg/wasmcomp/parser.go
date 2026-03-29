package wasmcomp

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

// ParseFile parses a .wasm file from disk.
func ParseFile(path string) (*Module, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return Parse(data)
}

// Parse parses a WASM binary.
func Parse(data []byte) (*Module, error) {
	r := &reader{data: data}

	// Magic number + version
	magic := r.readBytes(4)
	if string(magic) != "\x00asm" {
		return nil, fmt.Errorf("invalid WASM magic number")
	}
	version := r.readU32Fixed()
	if version != 1 {
		return nil, fmt.Errorf("unsupported WASM version: %d", version)
	}

	mod := &Module{StartFunc: -1}

	// Parse sections
	for r.pos < len(r.data) {
		sectionID := r.readByte()
		sectionLen := r.readU32()
		sectionEnd := r.pos + int(sectionLen)

		switch sectionID {
		case 1: // Type section
			mod.Types = parseTypeSection(r, sectionEnd)
		case 2: // Import section
			mod.Imports = parseImportSection(r, sectionEnd, mod.Types)
			for i := range mod.Imports {
				if mod.Imports[i].Kind == 0 { // func import
					mod.Imports[i].FuncIndex = mod.NumImportedFuncs
					mod.NumImportedFuncs++
				}
			}
		case 3: // Function section
			parseFunctionSection(r, sectionEnd, mod)
		case 4: // Table section
			r.pos = sectionEnd // skip for now
		case 5: // Memory section
			mod.Memory = parseMemorySection(r, sectionEnd)
		case 6: // Global section
			mod.Globals = parseGlobalSection(r, sectionEnd)
		case 7: // Export section
			mod.Exports = parseExportSection(r, sectionEnd)
		case 8: // Start section
			mod.StartFunc = int(r.readU32())
		case 9: // Element section
			mod.Elements = parseElementSection(r, sectionEnd)
		case 10: // Code section
			parseCodeSection(r, sectionEnd, mod)
		case 11: // Data section
			mod.Data = parseDataSection(r, sectionEnd)
		default:
			r.pos = sectionEnd // skip unknown sections
		}

		if r.pos != sectionEnd {
			r.pos = sectionEnd // align to section boundary
		}
	}

	// Assign export names to functions
	for _, exp := range mod.Exports {
		if exp.Kind == 0 { // function export
			funcIdx := exp.Index - mod.NumImportedFuncs
			if funcIdx >= 0 && funcIdx < len(mod.Functions) {
				mod.Functions[funcIdx].ExportName = exp.Name
			}
		}
	}

	return mod, nil
}

// --- Section Parsers ---

func parseTypeSection(r *reader, end int) []FuncType {
	count := r.readU32()
	types := make([]FuncType, count)
	for i := range types {
		_ = r.readByte() // 0x60 = functype
		paramCount := r.readU32()
		types[i].Params = make([]ValType, paramCount)
		for j := range types[i].Params {
			types[i].Params[j] = ValType(r.readByte())
		}
		resultCount := r.readU32()
		types[i].Results = make([]ValType, resultCount)
		for j := range types[i].Results {
			types[i].Results[j] = ValType(r.readByte())
		}
	}
	return types
}

func parseImportSection(r *reader, end int, types []FuncType) []Import {
	count := r.readU32()
	imports := make([]Import, count)
	for i := range imports {
		imports[i].Module = r.readString()
		imports[i].Name = r.readString()
		imports[i].Kind = r.readByte()
		switch imports[i].Kind {
		case 0: // func
			imports[i].TypeIndex = int(r.readU32())
			if imports[i].TypeIndex < len(types) {
				imports[i].Type = &types[imports[i].TypeIndex]
			}
		case 1: // table
			_ = r.readByte() // elemtype
			_ = r.readU32()  // min
			if r.peekByte() != 0 {
				// has max — but we already read the limits flag above, need to handle properly
			}
			// simplified: skip limits
			r.pos = findNextImportOrEnd(r, end, int(count)-i-1)
			return imports[:i+1]
		case 2: // memory
			parseMemoryLimits(r)
		case 3: // global
			_ = r.readByte() // valtype
			_ = r.readByte() // mutability
		}
	}
	return imports
}

func parseFunctionSection(r *reader, end int, mod *Module) {
	count := r.readU32()
	mod.Functions = make([]Function, count)
	for i := range mod.Functions {
		typeIdx := int(r.readU32())
		mod.Functions[i].Index = mod.NumImportedFuncs + i
		mod.Functions[i].TypeIndex = typeIdx
		if typeIdx < len(mod.Types) {
			mod.Functions[i].Type = &mod.Types[typeIdx]
		}
	}
}

func parseMemorySection(r *reader, end int) *Memory {
	count := r.readU32()
	if count == 0 {
		return nil
	}
	mem := &Memory{}
	hasMax := r.readByte()
	mem.Min = int(r.readU32())
	if hasMax == 1 {
		mem.Max = int(r.readU32())
	}
	// skip additional memories
	r.pos = end
	return mem
}

func parseGlobalSection(r *reader, end int) []Global {
	count := r.readU32()
	globals := make([]Global, count)
	for i := range globals {
		globals[i].Type = ValType(r.readByte())
		globals[i].Mutable = r.readByte() == 1
		// Parse init expression (simplified: expect i32.const or i64.const + end)
		initOp := r.readByte()
		switch initOp {
		case OpI32Const:
			globals[i].InitI32 = r.readI32()
		case OpI64Const:
			globals[i].InitI64 = r.readI64()
		case OpGlobalGet:
			_ = r.readU32() // global index
		}
		_ = r.readByte() // end (0x0B)
	}
	return globals
}

func parseExportSection(r *reader, end int) []Export {
	count := r.readU32()
	exports := make([]Export, count)
	for i := range exports {
		exports[i].Name = r.readString()
		exports[i].Kind = r.readByte()
		exports[i].Index = int(r.readU32())
	}
	return exports
}

func parseElementSection(r *reader, end int) []ElementSegment {
	count := r.readU32()
	elements := make([]ElementSegment, 0, count)
	for i := 0; i < int(count); i++ {
		flags := r.readU32()
		elem := ElementSegment{}

		if flags == 0 {
			// Active, table 0, i32.const offset
			_ = r.readByte() // i32.const
			elem.Offset = int(r.readI32())
			_ = r.readByte() // end
			funcCount := r.readU32()
			elem.FuncIndices = make([]int, funcCount)
			for j := range elem.FuncIndices {
				elem.FuncIndices[j] = int(r.readU32())
			}
			elements = append(elements, elem)
		} else {
			// Skip complex element segments for now
			r.pos = end
			return elements
		}
	}
	return elements
}

func parseCodeSection(r *reader, end int, mod *Module) {
	count := r.readU32()
	for i := 0; i < int(count) && i < len(mod.Functions); i++ {
		bodySize := r.readU32()
		bodyEnd := r.pos + int(bodySize)

		// Parse locals
		localDeclCount := r.readU32()
		var locals []ValType
		for j := 0; j < int(localDeclCount); j++ {
			localCount := r.readU32()
			localType := ValType(r.readByte())
			for k := 0; k < int(localCount); k++ {
				locals = append(locals, localType)
			}
		}
		mod.Functions[i].Locals = locals

		// Parse instructions
		mod.Functions[i].Code = parseInstructions(r, bodyEnd)

		r.pos = bodyEnd
	}
}

func parseDataSection(r *reader, end int) []DataSegment {
	count := r.readU32()
	segments := make([]DataSegment, 0, count)
	for i := 0; i < int(count); i++ {
		flags := r.readU32()
		seg := DataSegment{}

		if flags == 0 {
			// Active, memory 0
			_ = r.readByte() // i32.const
			seg.Offset = int(r.readI32())
			_ = r.readByte() // end
			dataLen := r.readU32()
			seg.Data = r.readBytes(int(dataLen))
			segments = append(segments, seg)
		} else if flags == 1 {
			// Passive
			dataLen := r.readU32()
			seg.Data = r.readBytes(int(dataLen))
			segments = append(segments, seg)
		} else {
			// Skip
			r.pos = end
			return segments
		}
	}
	return segments
}

// --- Instruction Parser ---

func parseInstructions(r *reader, end int) []Instruction {
	var instructions []Instruction

	for r.pos < end {
		op := r.readByte()
		inst := Instruction{Op: op}

		switch op {
		// Control with block type
		case OpBlock, OpLoop, OpIf:
			bt := r.readI32() // block type: 0x40=void, valtype, or type index
			inst.BlockType = int(bt)

		case OpBr, OpBrIf:
			inst.LabelIndex = int(r.readU32())

		case OpBrTable:
			labelCount := r.readU32()
			inst.Labels = make([]int, labelCount)
			for i := range inst.Labels {
				inst.Labels[i] = int(r.readU32())
			}
			inst.DefaultLabel = int(r.readU32())

		case OpCall:
			inst.FuncIndex = int(r.readU32())

		case OpCallIndirect:
			inst.TypeIndex = int(r.readU32())
			inst.TableIndex = int(r.readU32())

		// Variables
		case OpLocalGet, OpLocalSet, OpLocalTee:
			inst.LocalIndex = int(r.readU32())
		case OpGlobalGet, OpGlobalSet:
			inst.GlobalIndex = int(r.readU32())

		// Memory ops (all have align + offset)
		case OpI32Load, OpI64Load, OpF32Load, OpF64Load,
			OpI32Load8S, OpI32Load8U, OpI32Load16S, OpI32Load16U,
			OpI64Load8S, OpI64Load8U, OpI64Load16S, OpI64Load16U,
			OpI64Load32S, OpI64Load32U,
			OpI32Store, OpI64Store, OpF32Store, OpF64Store,
			OpI32Store8, OpI32Store16,
			OpI64Store8, OpI64Store16, OpI64Store32:
			inst.Align = int(r.readU32())
			inst.Offset = int(r.readU32())

		case OpMemorySize, OpMemoryGrow:
			_ = r.readByte() // memory index (always 0)

		// Constants
		case OpI32Const:
			inst.I32Value = r.readI32()
		case OpI64Const:
			inst.I64Value = r.readI64()
		case OpF32Const:
			bits := r.readU32Fixed()
			inst.F32Value = math.Float32frombits(bits)
		case OpF64Const:
			bits := r.readU64Fixed()
			inst.F64Value = math.Float64frombits(bits)

		// Typed select
		case OpSelectT:
			count := r.readU32() // number of types (always 1)
			for j := 0; j < int(count); j++ {
				_ = r.readByte() // valtype
			}

		// Try/catch (exception handling proposal)
		case OpTry:
			inst.BlockType = int(r.readI32())
		case OpCatch:
			inst.LabelIndex = int(r.readU32()) // tag index
		case OpThrow:
			inst.LabelIndex = int(r.readU32()) // tag index
		case OpRethrow:
			inst.LabelIndex = int(r.readU32()) // depth
		case OpDelegate:
			inst.LabelIndex = int(r.readU32()) // depth
		case OpCatchAll:
			// no immediates

		// Tail calls
		case OpReturnCall:
			inst.FuncIndex = int(r.readU32())
		case OpReturnCallIndirect:
			inst.TypeIndex = int(r.readU32())
			inst.TableIndex = int(r.readU32())

		// Multi-byte opcodes
		case OpMiscPrefix:
			miscOp := r.readU32()
			inst.MiscOp = byte(miscOp)
			switch inst.MiscOp {
			case MiscMemoryCopy:
				_ = r.readByte() // src memory
				_ = r.readByte() // dst memory
			case MiscMemoryFill:
				_ = r.readByte() // memory index
			default:
				// trunc_sat opcodes (0x00-0x07) have no additional immediates
				// memory.init (0x08), data.drop (0x09), table.* (0x0C-0x11) have immediates
				if inst.MiscOp == 0x08 { // memory.init
					_ = r.readU32() // data index
					_ = r.readByte() // memory index
				} else if inst.MiscOp == 0x09 { // data.drop
					_ = r.readU32() // data index
				} else if inst.MiscOp >= 0x0C && inst.MiscOp <= 0x11 { // table ops
					_ = r.readU32() // table/elem index
					if inst.MiscOp == 0x0E { // table.copy
						_ = r.readU32() // second table index
					}
				}
			}

		// SIMD prefix (0xFD) — skip the opcode + any immediates
		case OpSIMDPrefix:
			simdOp := r.readU32()
			inst.MiscOp = byte(simdOp)
			// SIMD loads/stores (opcodes 0-11) have memarg (align + offset)
			if simdOp <= 11 || (simdOp >= 84 && simdOp <= 91) || (simdOp >= 92 && simdOp <= 95) {
				_ = r.readU32() // align
				_ = r.readU32() // offset
			}
			// v128.const (opcode 12) has 16 bytes immediate
			if simdOp == 12 {
				_ = r.readBytes(16)
			}
			// i8x16.shuffle (opcode 13) has 16 byte lane indices
			if simdOp == 13 {
				_ = r.readBytes(16)
			}
			// extract_lane / replace_lane have 1 byte lane index
			if (simdOp >= 21 && simdOp <= 34) {
				_ = r.readByte()
			}

		// Everything else: no immediates
		// (end, else, nop, unreachable, drop, select, all arithmetic/comparison/conversion)
		}

		instructions = append(instructions, inst)

		if op == OpEnd && len(instructions) > 1 {
			// Check if this might be the function-level end
			// (we rely on bodyEnd in the caller to stop)
		}
	}

	return instructions
}

// --- Reader helpers ---

type reader struct {
	data []byte
	pos  int
}

func (r *reader) readByte() byte {
	if r.pos >= len(r.data) {
		return 0
	}
	b := r.data[r.pos]
	r.pos++
	return b
}

func (r *reader) peekByte() byte {
	if r.pos >= len(r.data) {
		return 0
	}
	return r.data[r.pos]
}

func (r *reader) readBytes(n int) []byte {
	if r.pos+n > len(r.data) {
		n = len(r.data) - r.pos
	}
	b := make([]byte, n)
	copy(b, r.data[r.pos:r.pos+n])
	r.pos += n
	return b
}

// readU32 reads an unsigned LEB128-encoded uint32.
func (r *reader) readU32() uint32 {
	var result uint32
	var shift uint
	for {
		b := r.readByte()
		result |= uint32(b&0x7F) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
	}
	return result
}

// readI32 reads a signed LEB128-encoded int32.
func (r *reader) readI32() int32 {
	var result int32
	var shift uint
	for {
		b := r.readByte()
		result |= int32(b&0x7F) << shift
		shift += 7
		if b&0x80 == 0 {
			if shift < 32 && b&0x40 != 0 {
				result |= -(1 << shift)
			}
			break
		}
	}
	return result
}

// readI64 reads a signed LEB128-encoded int64.
func (r *reader) readI64() int64 {
	var result int64
	var shift uint
	for {
		b := r.readByte()
		result |= int64(b&0x7F) << shift
		shift += 7
		if b&0x80 == 0 {
			if shift < 64 && b&0x40 != 0 {
				result |= -(1 << shift)
			}
			break
		}
	}
	return result
}

// readU32Fixed reads a fixed-size little-endian uint32.
func (r *reader) readU32Fixed() uint32 {
	b := r.readBytes(4)
	return binary.LittleEndian.Uint32(b)
}

// readU64Fixed reads a fixed-size little-endian uint64.
func (r *reader) readU64Fixed() uint64 {
	b := r.readBytes(8)
	return binary.LittleEndian.Uint64(b)
}

func (r *reader) readString() string {
	length := r.readU32()
	return string(r.readBytes(int(length)))
}

// findNextImportOrEnd is a fallback for complex import parsing.
func findNextImportOrEnd(r *reader, end int, remaining int) int {
	_ = remaining
	return end
}

func parseMemoryLimits(r *reader) {
	hasMax := r.readByte()
	_ = r.readU32() // min
	if hasMax == 1 {
		_ = r.readU32() // max
	}
}

// Ensure io and os are used.
var _ = io.EOF
var _ = os.Stdout
