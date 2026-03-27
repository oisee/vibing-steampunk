// Package wasmcomp implements a WASM-to-ABAP ahead-of-time compiler.
// It reads .wasm binaries, converts stack operations to SSA form,
// and emits ABAP class source code.
package wasmcomp

// ValType represents a WASM value type.
type ValType byte

const (
	ValI32 ValType = 0x7F
	ValI64 ValType = 0x7E
	ValF32 ValType = 0x7D
	ValF64 ValType = 0x7C
)

func (v ValType) String() string {
	switch v {
	case ValI32:
		return "i32"
	case ValI64:
		return "i64"
	case ValF32:
		return "f32"
	case ValF64:
		return "f64"
	default:
		return "unknown"
	}
}

// ABAPType returns the ABAP type for this value type.
func (v ValType) ABAPType() string {
	switch v {
	case ValI32:
		return "i" // TYPE i (signed 32-bit)
	case ValI64:
		return "int8" // TYPE int8 (signed 64-bit)
	case ValF32:
		return "f" // TYPE f (IEEE 754 double — ABAP has no 32-bit float)
	case ValF64:
		return "f" // TYPE f (IEEE 754 double)
	default:
		return "i"
	}
}

// FuncType represents a WASM function signature.
type FuncType struct {
	Params  []ValType
	Results []ValType
}

// Function represents a parsed WASM function.
type Function struct {
	Index      int
	TypeIndex  int
	Type       *FuncType
	Locals     []ValType // declared locals (not including params)
	Code       []Instruction
	ExportName string // empty if not exported
}

// Import represents a WASM import.
type Import struct {
	Module string
	Name   string
	Kind   byte // 0=func, 1=table, 2=memory, 3=global
	// For func imports:
	TypeIndex int
	Type      *FuncType
	// Assigned function index
	FuncIndex int
}

// Global represents a WASM global variable.
type Global struct {
	Type    ValType
	Mutable bool
	InitI32 int32 // initial value (simplified — only i32.const init for now)
	InitI64 int64
}

// Export represents a WASM export.
type Export struct {
	Name  string
	Kind  byte // 0=func, 1=table, 2=memory, 3=global
	Index int
}

// DataSegment represents a WASM data segment for memory initialization.
type DataSegment struct {
	MemIndex int
	Offset   int // simplified — only i32.const offset for now
	Data     []byte
}

// ElementSegment represents a WASM element segment for table initialization.
type ElementSegment struct {
	TableIndex int
	Offset     int // simplified — only i32.const offset for now
	FuncIndices []int
}

// Memory represents a WASM linear memory declaration.
type Memory struct {
	Min int // minimum pages
	Max int // maximum pages (0 = no limit)
}

// Module represents a parsed WASM module.
type Module struct {
	Types    []FuncType
	Imports  []Import
	Functions []Function
	Globals  []Global
	Exports  []Export
	Memory   *Memory
	Data     []DataSegment
	Elements []ElementSegment
	StartFunc int // -1 if no start function
	NumImportedFuncs int
}
