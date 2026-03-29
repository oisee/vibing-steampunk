package wasmcomp

// WASM opcodes (MVP + bulk memory).
const (
	// Control
	OpUnreachable  byte = 0x00
	OpNop          byte = 0x01
	OpBlock        byte = 0x02
	OpLoop         byte = 0x03
	OpIf           byte = 0x04
	OpElse         byte = 0x05
	OpEnd          byte = 0x0B
	OpBr           byte = 0x0C
	OpBrIf         byte = 0x0D
	OpBrTable      byte = 0x0E
	OpReturn       byte = 0x0F
	OpCall         byte = 0x10
	OpCallIndirect byte = 0x11

	// Parametric
	OpDrop   byte = 0x1A
	OpSelect byte = 0x1B

	// Variable
	OpLocalGet  byte = 0x20
	OpLocalSet  byte = 0x21
	OpLocalTee  byte = 0x22
	OpGlobalGet byte = 0x23
	OpGlobalSet byte = 0x24

	// Memory
	OpI32Load    byte = 0x28
	OpI64Load    byte = 0x29
	OpF32Load    byte = 0x2A
	OpF64Load    byte = 0x2B
	OpI32Load8S  byte = 0x2C
	OpI32Load8U  byte = 0x2D
	OpI32Load16S byte = 0x2E
	OpI32Load16U byte = 0x2F
	OpI64Load8S  byte = 0x30
	OpI64Load8U  byte = 0x31
	OpI64Load16S byte = 0x32
	OpI64Load16U byte = 0x33
	OpI64Load32S byte = 0x34
	OpI64Load32U byte = 0x35
	OpI32Store   byte = 0x36
	OpI64Store   byte = 0x37
	OpF32Store   byte = 0x38
	OpF64Store   byte = 0x39
	OpI32Store8  byte = 0x3A
	OpI32Store16 byte = 0x3B
	OpI64Store8  byte = 0x3C
	OpI64Store16 byte = 0x3D
	OpI64Store32 byte = 0x3E
	OpMemorySize byte = 0x3F
	OpMemoryGrow byte = 0x40

	// Constants
	OpI32Const byte = 0x41
	OpI64Const byte = 0x42
	OpF32Const byte = 0x43
	OpF64Const byte = 0x44

	// i32 comparison
	OpI32Eqz  byte = 0x45
	OpI32Eq   byte = 0x46
	OpI32Ne   byte = 0x47
	OpI32LtS  byte = 0x48
	OpI32LtU  byte = 0x49
	OpI32GtS  byte = 0x4A
	OpI32GtU  byte = 0x4B
	OpI32LeS  byte = 0x4C
	OpI32LeU  byte = 0x4D
	OpI32GeS  byte = 0x4E
	OpI32GeU  byte = 0x4F

	// i64 comparison
	OpI64Eqz  byte = 0x50
	OpI64Eq   byte = 0x51
	OpI64Ne   byte = 0x52
	OpI64LtS  byte = 0x53
	OpI64LtU  byte = 0x54
	OpI64GtS  byte = 0x55
	OpI64GtU  byte = 0x56
	OpI64LeS  byte = 0x57
	OpI64LeU  byte = 0x58
	OpI64GeS  byte = 0x59
	OpI64GeU  byte = 0x5A

	// f32 comparison
	OpF32Eq byte = 0x5B
	OpF32Ne byte = 0x5C
	OpF32Lt byte = 0x5D
	OpF32Gt byte = 0x5E
	OpF32Le byte = 0x5F
	OpF32Ge byte = 0x60

	// f64 comparison
	OpF64Eq byte = 0x61
	OpF64Ne byte = 0x62
	OpF64Lt byte = 0x63
	OpF64Gt byte = 0x64
	OpF64Le byte = 0x65
	OpF64Ge byte = 0x66

	// i32 arithmetic
	OpI32Clz    byte = 0x67
	OpI32Ctz    byte = 0x68
	OpI32Popcnt byte = 0x69
	OpI32Add    byte = 0x6A
	OpI32Sub    byte = 0x6B
	OpI32Mul    byte = 0x6C
	OpI32DivS   byte = 0x6D
	OpI32DivU   byte = 0x6E
	OpI32RemS   byte = 0x6F
	OpI32RemU   byte = 0x70
	OpI32And    byte = 0x71
	OpI32Or     byte = 0x72
	OpI32Xor    byte = 0x73
	OpI32Shl    byte = 0x74
	OpI32ShrS   byte = 0x75
	OpI32ShrU   byte = 0x76
	OpI32Rotl   byte = 0x77
	OpI32Rotr   byte = 0x78

	// i64 arithmetic
	OpI64Clz    byte = 0x79
	OpI64Ctz    byte = 0x7A
	OpI64Popcnt byte = 0x7B
	OpI64Add    byte = 0x7C
	OpI64Sub    byte = 0x7D
	OpI64Mul    byte = 0x7E
	OpI64DivS   byte = 0x7F
	OpI64DivU   byte = 0x80
	OpI64RemS   byte = 0x81
	OpI64RemU   byte = 0x82
	OpI64And    byte = 0x83
	OpI64Or     byte = 0x84
	OpI64Xor    byte = 0x85
	OpI64Shl    byte = 0x86
	OpI64ShrS   byte = 0x87
	OpI64ShrU   byte = 0x88
	OpI64Rotl   byte = 0x89
	OpI64Rotr   byte = 0x8A

	// f32 arithmetic
	OpF32Abs      byte = 0x8B
	OpF32Neg      byte = 0x8C
	OpF32Ceil     byte = 0x8D
	OpF32Floor    byte = 0x8E
	OpF32Trunc    byte = 0x8F
	OpF32Nearest  byte = 0x90
	OpF32Sqrt     byte = 0x91
	OpF32Add      byte = 0x92
	OpF32Sub      byte = 0x93
	OpF32Mul      byte = 0x94
	OpF32Div      byte = 0x95
	OpF32Min      byte = 0x96
	OpF32Max      byte = 0x97
	OpF32Copysign byte = 0x98

	// f64 arithmetic
	OpF64Abs      byte = 0x99
	OpF64Neg      byte = 0x9A
	OpF64Ceil     byte = 0x9B
	OpF64Floor    byte = 0x9C
	OpF64Trunc    byte = 0x9D
	OpF64Nearest  byte = 0x9E
	OpF64Sqrt     byte = 0x9F
	OpF64Add      byte = 0xA0
	OpF64Sub      byte = 0xA1
	OpF64Mul      byte = 0xA2
	OpF64Div      byte = 0xA3
	OpF64Min      byte = 0xA4
	OpF64Max      byte = 0xA5
	OpF64Copysign byte = 0xA6

	// Conversions
	OpI32WrapI64      byte = 0xA7
	OpI32TruncF32S    byte = 0xA8
	OpI32TruncF32U    byte = 0xA9
	OpI32TruncF64S    byte = 0xAA
	OpI32TruncF64U    byte = 0xAB
	OpI64ExtendI32S   byte = 0xAC
	OpI64ExtendI32U   byte = 0xAD
	OpI64TruncF32S    byte = 0xAE
	OpI64TruncF32U    byte = 0xAF
	OpI64TruncF64S    byte = 0xB0
	OpI64TruncF64U    byte = 0xB1
	OpF32ConvertI32S  byte = 0xB2
	OpF32ConvertI32U  byte = 0xB3
	OpF32ConvertI64S  byte = 0xB4
	OpF32ConvertI64U  byte = 0xB5
	OpF32DemoteF64    byte = 0xB6
	OpF64ConvertI32S  byte = 0xB7
	OpF64ConvertI32U  byte = 0xB8
	OpF64ConvertI64S  byte = 0xB9
	OpF64ConvertI64U  byte = 0xBA
	OpF64PromoteF32   byte = 0xBB
	OpI32ReinterpretF32 byte = 0xBC
	OpI64ReinterpretF64 byte = 0xBD
	OpF32ReinterpretI32 byte = 0xBE
	OpF64ReinterpretI64 byte = 0xBF

	// Sign extension (post-MVP but widely used)
	OpI32Extend8S  byte = 0xC0
	OpI32Extend16S byte = 0xC1
	OpI64Extend8S  byte = 0xC2
	OpI64Extend16S byte = 0xC3
	OpI64Extend32S byte = 0xC4

	// Typed select (post-MVP)
	OpSelectT byte = 0x1C

	// Reference types
	OpRefNull   byte = 0xD0
	OpRefIsNull byte = 0xD1
	OpRefFunc   byte = 0xD2

	// Try/catch (exception handling - stub as nop/trap)
	OpTry       byte = 0x06
	OpCatch     byte = 0x07
	OpThrow     byte = 0x08
	OpRethrow   byte = 0x09
	OpDelegate  byte = 0x18
	OpCatchAll  byte = 0x19

	// Tail calls
	OpReturnCall         byte = 0x12
	OpReturnCallIndirect byte = 0x13

	// Multi-byte prefix
	OpMiscPrefix byte = 0xFC
	OpSIMDPrefix byte = 0xFD
	OpAtomicPrefix byte = 0xFE
)

// Misc opcodes (after 0xFC prefix)
const (
	MiscMemoryCopy byte = 0x0A
	MiscMemoryFill byte = 0x0B
)

// Instruction represents a parsed WASM instruction with its immediates.
type Instruction struct {
	Op byte

	// Immediates (usage depends on opcode)
	I32Value   int32
	I64Value   int64
	F32Value   float32
	F64Value   float64
	LocalIndex int
	GlobalIndex int
	FuncIndex  int
	TypeIndex  int
	TableIndex int
	LabelIndex int     // for br, br_if
	Labels     []int   // for br_table
	DefaultLabel int   // for br_table
	Align      int     // for memory ops
	Offset     int     // for memory ops
	BlockType  int     // for block/loop/if: -1=void, >=0=type index, or valtype

	// Misc opcode (for 0xFC prefix)
	MiscOp byte
}
