package wasmcomp

import (
	"fmt"
	"strings"
)

// Compile takes a parsed WASM module and produces ABAP source code.
func Compile(mod *Module, className string) string {
	c := &compiler{
		mod:       mod,
		className: className,
	}
	return c.emit()
}

// blockKind tracks what ABAP construct a WASM block maps to.
type blockKind int

const (
	blockDO  blockKind = iota // block/loop → DO ... ENDDO
	blockIF                   // if → IF ... ENDIF
	blockTRY                  // try → TRY ... ENDTRY
)

// blockEntry tracks the kind and the stack depth when the block was entered.
type blockEntry struct {
	kind       blockKind
	savedDepth int // stack depth after popping condition (for if) or at block start
}

type compiler struct {
	mod        *Module
	className  string
	sb         strings.Builder
	indent     int
	blockStack []blockEntry // tracks what to close on OpEnd + stack depth

	// FUGR mode: emit PERFORM instead of method calls, gv_ instead of mv_
	useFUGR       bool
	fugrRedirects map[int]int

	// Line packing: multiple statements per line
	packLines bool
	packer    *linePacker
}

func (c *compiler) emit() string {
	c.emitDefinition()
	c.line("")
	c.emitImplementation()
	return c.sb.String()
}

// --- Class Definition ---

func (c *compiler) emitDefinition() {
	c.line("CLASS %s DEFINITION PUBLIC FINAL CREATE PUBLIC.", c.className)
	c.indent++
	c.line("PUBLIC SECTION.")
	c.indent++

	// Constructor
	c.line("METHODS constructor.")

	// Exported functions as public methods
	for _, f := range c.mod.Functions {
		if f.ExportName != "" && f.Type != nil {
			c.emitMethodSignature(f.ExportName, f.Type, true)
		}
	}

	c.indent--
	c.line("PRIVATE SECTION.")
	c.indent++

	// Linear memory
	c.line("DATA mv_mem TYPE xstring.")
	c.line("DATA mv_mem_pages TYPE i.")

	// Globals
	for i, g := range c.mod.Globals {
		c.line("DATA mv_g%d TYPE %s.", i, g.Type.ABAPType())
	}

	// Function table
	for i, elem := range c.mod.Elements {
		_ = elem
		c.line("DATA mt_tab%d TYPE STANDARD TABLE OF i WITH DEFAULT KEY.", i)
	}

	// Memory helper methods
	c.line("METHODS mem_ld_i32 IMPORTING iv_addr TYPE i RETURNING VALUE(rv) TYPE i.")
	c.line("METHODS mem_st_i32 IMPORTING iv_addr TYPE i iv_val TYPE i.")
	c.line("METHODS mem_ld_i32_8u IMPORTING iv_addr TYPE i RETURNING VALUE(rv) TYPE i.")
	c.line("METHODS mem_ld_i32_8s IMPORTING iv_addr TYPE i RETURNING VALUE(rv) TYPE i.")
	c.line("METHODS mem_ld_i32_16u IMPORTING iv_addr TYPE i RETURNING VALUE(rv) TYPE i.")
	c.line("METHODS mem_st_i32_8 IMPORTING iv_addr TYPE i iv_val TYPE i.")
	c.line("METHODS mem_st_i32_16 IMPORTING iv_addr TYPE i iv_val TYPE i.")
	c.line("METHODS mem_grow IMPORTING iv_pages TYPE i RETURNING VALUE(rv) TYPE i.")

	// Internal functions (non-exported only — exported are already in PUBLIC SECTION)
	for i, f := range c.mod.Functions {
		if f.Type != nil && f.ExportName == "" {
			name := fmt.Sprintf("f%d", i)
			c.emitMethodSignature(name, f.Type, false)
		}
	}

	c.indent--
	c.indent--
	c.line("ENDCLASS.")
}

func (c *compiler) emitMethodSignature(name string, ft *FuncType, isPublic bool) {
	parts := []string{"METHODS " + sanitizeABAP(name)}

	// Parameters
	if len(ft.Params) > 0 {
		var params []string
		for i, p := range ft.Params {
			params = append(params, fmt.Sprintf("p%d TYPE %s", i, p.ABAPType()))
		}
		parts = append(parts, "IMPORTING "+strings.Join(params, " "))
	}

	// Return value
	if len(ft.Results) == 1 {
		parts = append(parts, fmt.Sprintf("RETURNING VALUE(rv) TYPE %s", ft.Results[0].ABAPType()))
	}

	c.line("%s.", strings.Join(parts, " "))
}

// --- Class Implementation ---

func (c *compiler) emitImplementation() {
	c.line("CLASS %s IMPLEMENTATION.", c.className)
	c.indent++

	// Constructor
	c.emitConstructor()

	// Memory helpers
	c.emitMemoryHelpers()

	// Functions
	for i, f := range c.mod.Functions {
		if f.Type != nil {
			name := fmt.Sprintf("f%d", i)
			if f.ExportName != "" {
				name = sanitizeABAP(f.ExportName)
			}
			c.emitFunction(name, &f)
		}
	}

	c.indent--
	c.line("ENDCLASS.")
}

func (c *compiler) emitConstructor() {
	c.line("METHOD constructor.")
	c.indent++

	// Initialize memory
	if c.mod.Memory != nil {
		pages := c.mod.Memory.Min
		if pages == 0 {
			pages = 1
		}
		c.line("mv_mem_pages = %d.", pages)
		// Initialize memory to zeros
		c.line("DATA(lv_page) = CONV xstring( '00' ).")
		c.line("DO %d TIMES.", pages*65536-1)
		c.indent++
		c.line("CONCATENATE mv_mem lv_page INTO mv_mem IN BYTE MODE.")
		c.indent--
		c.line("ENDDO.")
	}

	// Initialize globals
	for i, g := range c.mod.Globals {
		if g.InitI32 != 0 {
			c.line("mv_g%d = %d.", i, g.InitI32)
		}
		if g.InitI64 != 0 {
			c.line("mv_g%d = %d.", i, g.InitI64)
		}
	}

	// Initialize data segments
	for _, seg := range c.mod.Data {
		if len(seg.Data) > 0 {
			hex := bytesToHex(seg.Data)
			c.line("mv_mem+%d(%d) = '%s'.", seg.Offset, len(seg.Data), hex)
		}
	}

	// Initialize element segments (function tables)
	for i, elem := range c.mod.Elements {
		for _, funcIdx := range elem.FuncIndices {
			c.line("APPEND %d TO mt_tab%d.", funcIdx, i)
		}
	}

	c.indent--
	c.line("ENDMETHOD.")
}

func (c *compiler) emitMemoryHelpers() {
	// i32 load (little-endian)
	c.line("METHOD mem_ld_i32.")
	c.indent++
	c.line("DATA lv_b TYPE x LENGTH 4.")
	c.line("lv_b = mv_mem+iv_addr(4).")
	c.line("\" Little-endian to big-endian")
	c.line("DATA(lv_r) = lv_b+3(1) && lv_b+2(1) && lv_b+1(1) && lv_b+0(1).")
	c.line("rv = lv_r.")
	c.indent--
	c.line("ENDMETHOD.")

	// i32 store (little-endian)
	c.line("METHOD mem_st_i32.")
	c.indent++
	c.line("DATA lv_b TYPE x LENGTH 4.")
	c.line("lv_b = iv_val.")
	c.line("\" Big-endian to little-endian")
	c.line("DATA(lv_r) = lv_b+3(1) && lv_b+2(1) && lv_b+1(1) && lv_b+0(1).")
	c.line("mv_mem+iv_addr(4) = lv_r.")
	c.indent--
	c.line("ENDMETHOD.")

	// i32 load 8-bit unsigned
	c.line("METHOD mem_ld_i32_8u.")
	c.indent++
	c.line("DATA lv_b TYPE x LENGTH 1.")
	c.line("lv_b = mv_mem+iv_addr(1).")
	c.line("rv = lv_b.")
	c.indent--
	c.line("ENDMETHOD.")

	// i32 load 8-bit signed
	c.line("METHOD mem_ld_i32_8s.")
	c.indent++
	c.line("DATA lv_b TYPE x LENGTH 1.")
	c.line("lv_b = mv_mem+iv_addr(1).")
	c.line("rv = lv_b.")
	c.line("IF rv > 127. rv = rv - 256. ENDIF.")
	c.indent--
	c.line("ENDMETHOD.")

	// i32 load 16-bit unsigned
	c.line("METHOD mem_ld_i32_16u.")
	c.indent++
	c.line("DATA lv_b TYPE x LENGTH 2.")
	c.line("lv_b = mv_mem+iv_addr(2).")
	c.line("DATA(lv_r) = lv_b+1(1) && lv_b+0(1).")
	c.line("rv = lv_r.")
	c.indent--
	c.line("ENDMETHOD.")

	// i32 store 8-bit
	c.line("METHOD mem_st_i32_8.")
	c.indent++
	c.line("DATA lv_b TYPE x LENGTH 1.")
	c.line("lv_b = iv_val.")
	c.line("mv_mem+iv_addr(1) = lv_b.")
	c.indent--
	c.line("ENDMETHOD.")

	// i32 store 16-bit
	c.line("METHOD mem_st_i32_16.")
	c.indent++
	c.line("DATA lv_b TYPE x LENGTH 2.")
	c.line("lv_b = iv_val.")
	c.line("DATA(lv_r) = lv_b+1(1) && lv_b+0(1).")
	c.line("mv_mem+iv_addr(2) = lv_r.")
	c.indent--
	c.line("ENDMETHOD.")

	// memory.grow
	c.line("METHOD mem_grow.")
	c.indent++
	c.line("rv = mv_mem_pages.")
	c.line("DATA(lv_new_bytes) = iv_pages * 65536.")
	c.line("DATA lv_zeros TYPE xstring.")
	c.line("DO lv_new_bytes TIMES.")
	c.indent++
	c.line("CONCATENATE lv_zeros '00' INTO lv_zeros IN BYTE MODE.")
	c.indent--
	c.line("ENDDO.")
	c.line("CONCATENATE mv_mem lv_zeros INTO mv_mem IN BYTE MODE.")
	c.line("mv_mem_pages = mv_mem_pages + iv_pages.")
	c.indent--
	c.line("ENDMETHOD.")
}

// --- Function Code Generation ---

func (c *compiler) emitFunction(name string, f *Function) {
	c.line("METHOD %s.", sanitizeABAP(name))
	c.indent++

	// Emit chained DATA declaration
	c.line("%s", emitChainedDATA(f))

	// Enable line packing for code
	c.packLines = true
	c.packer = newLinePacker(&c.sb, c.indent)

	// Emit instructions
	stack := &virtualStack{}
	c.blockStack = nil // reset block stack for each function
	c.emitInstructions(f, f.Code, stack, 0)

	// Assign return value from top of stack
	if len(f.Type.Results) > 0 && stack.depth > 0 {
		c.line("rv = %s.", stack.peek())
	}

	c.flushPacker()
	c.packLines = false
	c.packer = nil

	c.indent--
	c.line("ENDMETHOD.")
}

func (c *compiler) emitInstructions(f *Function, code []Instruction, stack *virtualStack, blockDepth int) {
	for i := 0; i < len(code); i++ {
		inst := code[i]
		switch inst.Op {

		case OpNop:
			// nothing

		case OpUnreachable:
			c.line("RAISE EXCEPTION TYPE cx_sy_program_error. \" unreachable")

		// Constants
		case OpI32Const:
			v := stack.push()
			c.line("%s = %d.", v, inst.I32Value)
		case OpI64Const:
			v := stack.push()
			c.line("%s = %d.", v, inst.I64Value)
		case OpF32Const:
			v := stack.push()
			c.line("%s = '%f'.", v, inst.F32Value)
		case OpF64Const:
			v := stack.push()
			c.line("%s = '%f'.", v, inst.F64Value)

		// Local/Global access
		case OpLocalGet:
			v := stack.push()
			c.line("%s = %s.", v, c.localName(f, inst.LocalIndex))
		case OpLocalSet:
			v := stack.pop()
			c.line("%s = %s.", c.localName(f, inst.LocalIndex), v)
		case OpLocalTee:
			v := stack.peek()
			c.line("%s = %s.", c.localName(f, inst.LocalIndex), v)
		case OpGlobalGet:
			v := stack.push()
			c.line("%s = %s%d.", v, c.globalPrefix(), inst.GlobalIndex)
		case OpGlobalSet:
			v := stack.pop()
			c.line("%s%d = %s.", c.globalPrefix(), inst.GlobalIndex, v)

		// i32 arithmetic
		case OpI32Add:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s + %s.", r, a, b)
		case OpI32Sub:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s - %s.", r, a, b)
		case OpI32Mul:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s * %s.", r, a, b)
		case OpI32DivS:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s / %s.", r, a, b)
		case OpI32DivU:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>div_u32( iv_a = %s iv_b = %s ).", r, a, b)
		case OpI32RemS:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s MOD %s.", r, a, b)
		case OpI32RemU:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>rem_u32( iv_a = %s iv_b = %s ).", r, a, b)

		// Bitwise
		case OpI32And:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>and32( iv_a = %s iv_b = %s ).", r, a, b)
		case OpI32Or:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>or32( iv_a = %s iv_b = %s ).", r, a, b)
		case OpI32Xor:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>xor32( iv_a = %s iv_b = %s ).", r, a, b)
		case OpI32Shl:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>shl32( iv_val = %s iv_shift = %s ).", r, a, b)
		case OpI32ShrS:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>shr_s32( iv_val = %s iv_shift = %s ).", r, a, b)
		case OpI32ShrU:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>shr_u32( iv_val = %s iv_shift = %s ).", r, a, b)

		// Comparisons
		case OpI32Eqz:
			a := stack.pop()
			r := stack.push()
			c.line("IF %s = 0. %s = 1. ELSE. %s = 0. ENDIF.", a, r, r)
		case OpI32Eq:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s = %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpI32Ne:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s <> %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpI32LtS:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s < %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpI32GtS:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s > %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpI32LeS:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s <= %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpI32GeS:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s >= %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpI32LtU, OpI32GtU, OpI32LeU, OpI32GeU:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			op := "lt"
			switch inst.Op {
			case OpI32GtU:
				op = "gt"
			case OpI32LeU:
				op = "le"
			case OpI32GeU:
				op = "ge"
			}
			c.line("IF zcl_wasm_rt=>%s_u32( iv_a = %s iv_b = %s ) = abap_true. %s = 1. ELSE. %s = 0. ENDIF.", op, a, b, r, r)

		// Memory
		case OpI32Load:
			addr := stack.pop()
			r := stack.push()
			if inst.Offset > 0 {
				c.line("%s = mem_ld_i32( %s + %d ).", r, addr, inst.Offset)
			} else {
				c.line("%s = mem_ld_i32( %s ).", r, addr)
			}
		case OpI32Store:
			val, addr := stack.pop(), stack.pop()
			if inst.Offset > 0 {
				c.line("mem_st_i32( iv_addr = %s + %d iv_val = %s ).", addr, inst.Offset, val)
			} else {
				c.line("mem_st_i32( iv_addr = %s iv_val = %s ).", addr, val)
			}
		case OpI32Load8U:
			addr := stack.pop()
			r := stack.push()
			if inst.Offset > 0 {
				c.line("%s = mem_ld_i32_8u( %s + %d ).", r, addr, inst.Offset)
			} else {
				c.line("%s = mem_ld_i32_8u( %s ).", r, addr)
			}
		case OpI32Load8S:
			addr := stack.pop()
			r := stack.push()
			if inst.Offset > 0 {
				c.line("%s = mem_ld_i32_8s( %s + %d ).", r, addr, inst.Offset)
			} else {
				c.line("%s = mem_ld_i32_8s( %s ).", r, addr)
			}
		case OpI32Store8:
			val, addr := stack.pop(), stack.pop()
			if inst.Offset > 0 {
				c.line("mem_st_i32_8( iv_addr = %s + %d iv_val = %s ).", addr, inst.Offset, val)
			} else {
				c.line("mem_st_i32_8( iv_addr = %s iv_val = %s ).", addr, val)
			}

		case OpMemorySize:
			r := stack.push()
			c.line("%s = %s.", r, c.memPagesVar())
		case OpMemoryGrow:
			pages := stack.pop()
			r := stack.push()
			c.line("%s = mem_grow( %s ).", r, pages)

		// Call
		case OpCall:
			c.emitCall(f, inst.FuncIndex, stack)

		// Control flow
		case OpBlock:
			c.line("DO 1 TIMES. \" block")
			c.indent++
			c.blockStack = append(c.blockStack, blockEntry{kind: blockDO, savedDepth: stack.depth})
		case OpLoop:
			c.line("DO. \" loop")
			c.indent++
			c.blockStack = append(c.blockStack, blockEntry{kind: blockDO, savedDepth: stack.depth})
		case OpIf:
			cond := stack.pop()
			c.line("IF %s <> 0.", cond)
			c.indent++
			c.blockStack = append(c.blockStack, blockEntry{kind: blockIF, savedDepth: stack.depth})
		case OpElse:
			c.indent--
			c.line("ELSE.")
			c.indent++
			// Reset stack depth to what it was at the start of the if block
			if len(c.blockStack) > 0 {
				stack.depth = c.blockStack[len(c.blockStack)-1].savedDepth
			}
		case OpEnd:
			if len(c.blockStack) > 0 {
				entry := c.blockStack[len(c.blockStack)-1]
				c.blockStack = c.blockStack[:len(c.blockStack)-1]
				c.indent--
				switch entry.kind {
				case blockIF:
					c.line("ENDIF.")
				case blockDO:
					c.line("ENDDO.")
				case blockTRY:
					c.line("ENDTRY.")
				}
			}
		case OpBr:
			if inst.LabelIndex == 0 {
				c.line("EXIT. \" br 0")
			} else {
				c.line("lv_br = %d. EXIT. \" br %d", inst.LabelIndex, inst.LabelIndex)
			}
		case OpBrIf:
			cond := stack.pop()
			if inst.LabelIndex == 0 {
				c.line("IF %s <> 0. EXIT. ENDIF. \" br_if 0", cond)
			} else {
				c.line("IF %s <> 0. lv_br = %d. EXIT. ENDIF. \" br_if %d", cond, inst.LabelIndex, inst.LabelIndex)
			}
		case OpReturn:
			if len(f.Type.Results) > 0 {
				ret := stack.pop()
				c.line("rv = %s. RETURN.", ret)
			} else {
				c.line("RETURN.")
			}

		// Stack
		case OpDrop:
			stack.pop()
		case OpSelect, OpSelectT:
			cond, b, a := stack.pop(), stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s <> 0. %s = %s. ELSE. %s = %s. ENDIF.", cond, r, a, r, b)

		// Tail calls — treat as regular call + return
		case OpReturnCall:
			c.emitCall(f, inst.FuncIndex, stack)
			if len(f.Type.Results) > 0 {
				c.line("rv = %s. RETURN.", stack.peek())
			} else {
				c.line("RETURN.")
			}
		case OpReturnCallIndirect:
			c.emitCallIndirect(f, inst.TypeIndex, inst.TableIndex, stack)
			if len(f.Type.Results) > 0 {
				c.line("rv = %s. RETURN.", stack.peek())
			} else {
				c.line("RETURN.")
			}

		// Try/catch — map to TRY/CATCH in ABAP
		case OpTry:
			c.line("TRY. \" wasm try")
			c.indent++
			c.blockStack = append(c.blockStack, blockEntry{kind: blockTRY, savedDepth: stack.depth})
		case OpCatch, OpCatchAll:
			c.indent--
			c.line("CATCH cx_root. \" wasm catch")
			c.indent++
		case OpThrow:
			c.line("RAISE EXCEPTION TYPE cx_sy_program_error. \" wasm throw")
		case OpRethrow:
			c.line("RAISE EXCEPTION TYPE cx_sy_program_error. \" wasm rethrow")
		case OpDelegate:
			c.indent--
			c.line("ENDTRY. \" delegate")

		// SIMD — stub as trap (QuickJS shouldn't hit these in normal execution)
		case OpSIMDPrefix:
			c.line("RAISE EXCEPTION TYPE cx_sy_program_error. \" SIMD not supported")

		// i64 arithmetic (same patterns as i32 — ABAP INT8 handles it)
		case OpI64Add:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s + %s.", r, a, b)
		case OpI64Sub:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s - %s.", r, a, b)
		case OpI64Mul:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s * %s.", r, a, b)
		case OpI64DivS:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s / %s.", r, a, b)
		case OpI64DivU:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>div_u64( iv_a = %s iv_b = %s ).", r, a, b)
		case OpI64RemS:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s MOD %s.", r, a, b)
		case OpI64RemU:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>rem_u64( iv_a = %s iv_b = %s ).", r, a, b)
		case OpI64And:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>and64( iv_a = %s iv_b = %s ).", r, a, b)
		case OpI64Or:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>or64( iv_a = %s iv_b = %s ).", r, a, b)
		case OpI64Xor:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>xor64( iv_a = %s iv_b = %s ).", r, a, b)
		case OpI64Shl:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>shl64( iv_val = %s iv_shift = %s ).", r, a, b)
		case OpI64ShrS:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>shr_s64( iv_val = %s iv_shift = %s ).", r, a, b)
		case OpI64ShrU:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>shr_u64( iv_val = %s iv_shift = %s ).", r, a, b)

		// i64 comparisons
		case OpI64Eqz:
			a := stack.pop()
			r := stack.push()
			c.line("IF %s = 0. %s = 1. ELSE. %s = 0. ENDIF.", a, r, r)
		case OpI64Eq:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s = %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpI64Ne:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s <> %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpI64LtS:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s < %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpI64GtS:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s > %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpI64LeS:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s <= %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpI64GeS:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s >= %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpI64LtU, OpI64GtU, OpI64LeU, OpI64GeU:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			op := "lt"
			switch inst.Op {
			case OpI64GtU:
				op = "gt"
			case OpI64LeU:
				op = "le"
			case OpI64GeU:
				op = "ge"
			}
			c.line("IF zcl_wasm_rt=>%s_u64( iv_a = %s iv_b = %s ) = abap_true. %s = 1. ELSE. %s = 0. ENDIF.", op, a, b, r, r)

		// i32 rotl/rotr/clz/ctz/popcnt
		case OpI32Rotl:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>rotl32( iv_val = %s iv_shift = %s ).", r, a, b)
		case OpI32Rotr:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>rotr32( iv_val = %s iv_shift = %s ).", r, a, b)
		case OpI32Clz:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>clz32( %s ).", r, a)
		case OpI32Ctz:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>ctz32( %s ).", r, a)
		case OpI32Popcnt:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>popcnt32( %s ).", r, a)

		// i64 rotl/rotr/clz/ctz/popcnt
		case OpI64Rotl:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>rotl64( iv_val = %s iv_shift = %s ).", r, a, b)
		case OpI64Rotr:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>rotr64( iv_val = %s iv_shift = %s ).", r, a, b)
		case OpI64Clz:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>clz64( %s ).", r, a)
		case OpI64Ctz:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>ctz64( %s ).", r, a)
		case OpI64Popcnt:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>popcnt64( %s ).", r, a)

		// f64 arithmetic
		case OpF64Add:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s + %s.", r, a, b)
		case OpF64Sub:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s - %s.", r, a, b)
		case OpF64Mul:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s * %s.", r, a, b)
		case OpF64Div:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s / %s.", r, a, b)

		// f64 comparisons
		case OpF64Eq:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s = %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpF64Ne:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s <> %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpF64Lt:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s < %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpF64Gt:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s > %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpF64Le:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s <= %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpF64Ge:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s >= %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)

		// f32 comparisons
		case OpF32Eq:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s = %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpF32Ne:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s <> %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpF32Lt:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s < %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpF32Gt:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s > %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpF32Le:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s <= %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)
		case OpF32Ge:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s >= %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, r, r)

		// f32 arithmetic
		case OpF32Add:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s + %s.", r, a, b)
		case OpF32Sub:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s - %s.", r, a, b)
		case OpF32Mul:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s * %s.", r, a, b)
		case OpF32Div:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = %s / %s.", r, a, b)
		case OpF32Abs:
			a := stack.pop()
			r := stack.push()
			c.line("%s = abs( %s ).", r, a)
		case OpF32Neg:
			a := stack.pop()
			r := stack.push()
			c.line("%s = - %s.", r, a)
		case OpF32Sqrt:
			a := stack.pop()
			r := stack.push()
			c.line("%s = sqrt( %s ).", r, a)
		case OpF32Min:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s < %s. %s = %s. ELSE. %s = %s. ENDIF.", a, b, r, a, r, b)
		case OpF32Max:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s > %s. %s = %s. ELSE. %s = %s. ENDIF.", a, b, r, a, r, b)
		case OpF32Ceil:
			a := stack.pop()
			r := stack.push()
			c.line("%s = ceil( %s ).", r, a)
		case OpF32Floor:
			a := stack.pop()
			r := stack.push()
			c.line("%s = floor( %s ).", r, a)
		case OpF32Trunc:
			a := stack.pop()
			r := stack.push()
			c.line("%s = trunc( %s ).", r, a)
		case OpF32Nearest:
			a := stack.pop()
			r := stack.push()
			c.line("%s = round( val = %s dec = 0 ).", r, a)
		case OpF32Copysign:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>copysign( iv_mag = %s iv_sign = %s ).", r, a, b)

		// f64.copysign, f64.nearest
		case OpF64Copysign:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>copysign( iv_mag = %s iv_sign = %s ).", r, a, b)
		case OpF64Nearest:
			a := stack.pop()
			r := stack.push()
			c.line("%s = round( val = %s dec = 0 ).", r, a)

		// Additional conversions
		case OpF64ConvertI64U:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>extend_u64_f( %s ).", r, a)
		case OpF32ConvertI32U:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>extend_u32( %s ). \" f32.convert_i32_u", r, a)
		case OpF32ConvertI64U:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>extend_u64_f( %s ).", r, a)
		case OpI64TruncF64U, OpI64TruncF32U:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>trunc_f_u64( %s ).", r, a)

		// i64 memory
		case OpI64Load:
			addr := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>mem_ld_i64( iv_mem = mv_mem iv_addr = %s + %d ).", r, addr, inst.Offset)
		case OpI64Store:
			val, addr := stack.pop(), stack.pop()
			c.line("zcl_wasm_rt=>mem_st_i64( EXPORTING iv_val = %s iv_addr = %s + %d CHANGING cv_mem = mv_mem ).", val, addr, inst.Offset)
		case OpI64Load8S, OpI64Load8U, OpI64Load16S, OpI64Load16U, OpI64Load32S, OpI64Load32U:
			addr := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>mem_ld_i64_ext( iv_mem = mv_mem iv_addr = %s + %d iv_op = %d ).", r, addr, inst.Offset, inst.Op)
		case OpI64Store8, OpI64Store16, OpI64Store32:
			val, addr := stack.pop(), stack.pop()
			c.line("zcl_wasm_rt=>mem_st_i64_trunc( EXPORTING iv_val = %s iv_addr = %s + %d iv_op = %d CHANGING cv_mem = mv_mem ).", val, addr, inst.Offset, inst.Op)

		// i32 load16s, store16
		case OpI32Load16S:
			addr := stack.pop()
			r := stack.push()
			if inst.Offset > 0 {
				c.line("%s = zcl_wasm_rt=>mem_ld_i32_16s( iv_mem = mv_mem iv_addr = %s + %d ).", r, addr, inst.Offset)
			} else {
				c.line("%s = zcl_wasm_rt=>mem_ld_i32_16s( iv_mem = mv_mem iv_addr = %s ).", r, addr)
			}
		case OpI32Load16U:
			addr := stack.pop()
			r := stack.push()
			if inst.Offset > 0 {
				c.line("%s = mem_ld_i32_16u( %s + %d ).", r, addr, inst.Offset)
			} else {
				c.line("%s = mem_ld_i32_16u( %s ).", r, addr)
			}
		case OpI32Store16:
			val, addr := stack.pop(), stack.pop()
			if inst.Offset > 0 {
				c.line("mem_st_i32_16( iv_addr = %s + %d iv_val = %s ).", addr, inst.Offset, val)
			} else {
				c.line("mem_st_i32_16( iv_addr = %s iv_val = %s ).", addr, val)
			}

		// f32/f64 load/store
		case OpF32Load:
			addr := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>mem_ld_f32( iv_mem = mv_mem iv_addr = %s + %d ).", r, addr, inst.Offset)
		case OpF64Load:
			addr := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>mem_ld_f64( iv_mem = mv_mem iv_addr = %s + %d ).", r, addr, inst.Offset)
		case OpF32Store:
			val, addr := stack.pop(), stack.pop()
			c.line("zcl_wasm_rt=>mem_st_f32( EXPORTING iv_val = %s iv_addr = %s + %d CHANGING cv_mem = mv_mem ).", val, addr, inst.Offset)
		case OpF64Store:
			val, addr := stack.pop(), stack.pop()
			c.line("zcl_wasm_rt=>mem_st_f64( EXPORTING iv_val = %s iv_addr = %s + %d CHANGING cv_mem = mv_mem ).", val, addr, inst.Offset)

		// Conversions
		case OpI32WrapI64:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>wrap_i64( %s ).", r, a)
		case OpI64ExtendI32S:
			a := stack.pop()
			r := stack.push()
			c.line("%s = %s. \" i64.extend_i32_s (noop in ABAP - sign preserved)", r, a)
		case OpI64ExtendI32U:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>extend_u32( %s ).", r, a)
		case OpI32TruncF64S, OpI32TruncF32S:
			a := stack.pop()
			r := stack.push()
			c.line("%s = trunc( %s ).", r, a)
		case OpI32TruncF64U, OpI32TruncF32U:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>trunc_f_u32( %s ).", r, a)
		case OpI64TruncF64S, OpI64TruncF32S:
			a := stack.pop()
			r := stack.push()
			c.line("%s = trunc( %s ).", r, a)
		case OpF64ConvertI32S, OpF64ConvertI64S:
			a := stack.pop()
			r := stack.push()
			c.line("%s = %s. \" convert to f64", r, a)
		case OpF64ConvertI32U:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>extend_u32( %s ). \" f64.convert_i32_u", r, a)
		case OpF64PromoteF32:
			a := stack.pop()
			r := stack.push()
			c.line("%s = %s. \" f64.promote_f32 (noop in ABAP)", r, a)
		case OpF32DemoteF64:
			a := stack.pop()
			r := stack.push()
			c.line("%s = %s. \" f32.demote_f64 (precision loss ok)", r, a)
		case OpF32ConvertI32S, OpF32ConvertI64S:
			a := stack.pop()
			r := stack.push()
			c.line("%s = %s. \" convert to f32", r, a)
		case OpI32ReinterpretF32:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>reinterpret_f32_i32( %s ).", r, a)
		case OpF32ReinterpretI32:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>reinterpret_i32_f32( %s ).", r, a)
		case OpI64ReinterpretF64:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>reinterpret_f64_i64( %s ).", r, a)
		case OpF64ReinterpretI64:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>reinterpret_i64_f64( %s ).", r, a)

		// Sign extension
		case OpI32Extend8S:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>extend8s_i32( %s ).", r, a)
		case OpI32Extend16S:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>extend16s_i32( %s ).", r, a)
		case OpI64Extend8S:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>extend8s_i64( %s ).", r, a)
		case OpI64Extend16S:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>extend16s_i64( %s ).", r, a)
		case OpI64Extend32S:
			a := stack.pop()
			r := stack.push()
			c.line("%s = zcl_wasm_rt=>extend32s_i64( %s ).", r, a)

		// call_indirect
		case OpCallIndirect:
			c.emitCallIndirect(f, inst.TypeIndex, inst.TableIndex, stack)

		// Misc prefix (0xFC)
		case OpMiscPrefix:
			switch inst.MiscOp {
			case MiscMemoryCopy:
				n, src, dst := stack.pop(), stack.pop(), stack.pop()
				c.line("zcl_wasm_rt=>mem_copy( EXPORTING iv_dst = %s iv_src = %s iv_n = %s CHANGING cv_mem = mv_mem ).", dst, src, n)
			case MiscMemoryFill:
				n, val, dst := stack.pop(), stack.pop(), stack.pop()
				c.line("zcl_wasm_rt=>mem_fill( EXPORTING iv_dst = %s iv_val = %s iv_n = %s CHANGING cv_mem = mv_mem ).", dst, val, n)
			default:
				c.line("\" TODO: misc opcode 0xFC 0x%02X", inst.MiscOp)
			}

		// f64 math functions
		case OpF64Abs:
			a := stack.pop()
			r := stack.push()
			c.line("%s = abs( %s ).", r, a)
		case OpF64Neg:
			a := stack.pop()
			r := stack.push()
			c.line("%s = - %s.", r, a)
		case OpF64Ceil:
			a := stack.pop()
			r := stack.push()
			c.line("%s = ceil( %s ).", r, a)
		case OpF64Floor:
			a := stack.pop()
			r := stack.push()
			c.line("%s = floor( %s ).", r, a)
		case OpF64Sqrt:
			a := stack.pop()
			r := stack.push()
			c.line("%s = sqrt( %s ).", r, a)
		case OpF64Trunc:
			a := stack.pop()
			r := stack.push()
			c.line("%s = trunc( %s ).", r, a)
		case OpF64Min:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s < %s. %s = %s. ELSE. %s = %s. ENDIF.", a, b, r, a, r, b)
		case OpF64Max:
			b, a := stack.pop(), stack.pop()
			r := stack.push()
			c.line("IF %s > %s. %s = %s. ELSE. %s = %s. ENDIF.", a, b, r, a, r, b)

		// br_table
		case OpBrTable:
			idx := stack.pop()
			c.line("CASE %s.", idx)
			c.indent++
			for i, label := range inst.Labels {
				if label == 0 {
					c.line("WHEN %d. EXIT.", i)
				} else {
					c.line("WHEN %d. lv_br = %d. EXIT.", i, label)
				}
			}
			if inst.DefaultLabel == 0 {
				c.line("WHEN OTHERS. EXIT.")
			} else {
				c.line("WHEN OTHERS. lv_br = %d. EXIT.", inst.DefaultLabel)
			}
			c.indent--
			c.line("ENDCASE.")

		default:
			c.line("\" TODO: opcode 0x%02X", inst.Op)
		}
	}
}

func (c *compiler) emitCall(f *Function, funcIndex int, stack *virtualStack) {
	// Determine if it's an import or local function
	if funcIndex < c.mod.NumImportedFuncs {
		imp := c.findImport(funcIndex)
		if imp != nil {
			// WASI imports — emit stub based on function name
			c.emitWASICall(imp, stack)
		}
		return
	}

	localIdx := funcIndex - c.mod.NumImportedFuncs
	if localIdx >= len(c.mod.Functions) {
		c.line("\" ERROR: invalid function index %d", funcIndex)
		return
	}

	// Check dedup redirect
	if c.useFUGR && c.fugrRedirects != nil {
		if canonIdx, ok := c.fugrRedirects[localIdx]; ok {
			localIdx = canonIdx
		}
	}

	target := &c.mod.Functions[localIdx]
	if target.Type == nil {
		return
	}

	// Pop arguments from stack (in reverse)
	args := make([]string, len(target.Type.Params))
	for i := len(args) - 1; i >= 0; i-- {
		args[i] = stack.pop()
	}

	name := fmt.Sprintf("f%d", localIdx)
	if target.ExportName != "" {
		name = sanitizeABAP(target.ExportName)
	}

	if c.useFUGR {
		// PERFORM-based call
		if len(target.Type.Results) > 0 {
			result := stack.push()
			if len(args) > 0 {
				c.line("PERFORM %s USING %s CHANGING %s.", name, strings.Join(args, " "), result)
			} else {
				c.line("PERFORM %s CHANGING %s.", name, result)
			}
		} else {
			if len(args) > 0 {
				c.line("PERFORM %s USING %s.", name, strings.Join(args, " "))
			} else {
				c.line("PERFORM %s.", name)
			}
		}
	} else {
		// Method-based call
		if len(target.Type.Results) > 0 {
			result := stack.push()
			if len(args) > 0 {
				var paramParts []string
				for i, a := range args {
					paramParts = append(paramParts, fmt.Sprintf("p%d = %s", i, a))
				}
				c.line("%s = %s( %s ).", result, name, strings.Join(paramParts, " "))
			} else {
				c.line("%s = %s( ).", result, name)
			}
		} else {
			if len(args) > 0 {
				var paramParts []string
				for i, a := range args {
					paramParts = append(paramParts, fmt.Sprintf("p%d = %s", i, a))
				}
				c.line("%s( %s ).", name, strings.Join(paramParts, " "))
			} else {
				c.line("%s( ).", name)
			}
		}
	}
}

func (c *compiler) emitCallIndirect(f *Function, typeIndex, tableIndex int, stack *virtualStack) {
	if typeIndex >= len(c.mod.Types) {
		c.line("\" ERROR: invalid type index %d for call_indirect", typeIndex)
		return
	}
	ft := &c.mod.Types[typeIndex]

	// Pop the table index from stack
	tableIdx := stack.pop()

	// Pop arguments
	args := make([]string, len(ft.Params))
	for i := len(args) - 1; i >= 0; i-- {
		args[i] = stack.pop()
	}

	// Read function index from table
	c.line("DATA(lv_ci_func) = mt_tab%d[ %s + 1 ]. \" call_indirect", tableIndex, tableIdx)

	// Generate dispatch
	var paramParts []string
	for i, a := range args {
		paramParts = append(paramParts, fmt.Sprintf("p%d = %s", i, a))
	}
	paramStr := strings.Join(paramParts, " ")

	if len(ft.Results) > 0 {
		result := stack.push()
		c.line("%s = dispatch_t%d( iv_idx = lv_ci_func %s ).", result, typeIndex, paramStr)
	} else {
		c.line("dispatch_t%d( iv_idx = lv_ci_func %s ).", typeIndex, paramStr)
	}
}

func (c *compiler) emitWASICall(imp *Import, stack *virtualStack) {
	if imp.Type == nil {
		c.line("\" IMPORT: %s.%s (no type info)", imp.Module, imp.Name)
		return
	}

	// Pop arguments
	args := make([]string, len(imp.Type.Params))
	for i := len(args) - 1; i >= 0; i-- {
		args[i] = stack.pop()
	}

	mem := c.memVar()

	switch imp.Name {
	case "fd_write":
		// fd_write(fd, iovs_ptr, iovs_len, nwritten_ptr) -> errno
		result := stack.push()
		c.line("\" WASI fd_write: fd=%s iovs=%s iovs_len=%s nwritten=%s", args[0], args[1], args[2], args[3])
		c.line("DATA lv_wasi_written TYPE i.")
		c.line("DATA lv_wasi_iov_ptr TYPE i.")
		c.line("DATA lv_wasi_iov_len TYPE i.")
		c.line("DATA lv_wasi_str_ptr TYPE i.")
		c.line("DATA lv_wasi_str_len TYPE i.")
		c.line("lv_wasi_written = 0.")
		c.line("DO %s TIMES.", args[2])
		c.indent++
		c.line("lv_wasi_iov_ptr = %s + ( sy-index - 1 ) * 8.", args[1])
		c.line("PERFORM mem_ld_i32 USING lv_wasi_iov_ptr CHANGING lv_wasi_str_ptr.")
		c.line("PERFORM mem_ld_i32 USING lv_wasi_iov_ptr + 4 CHANGING lv_wasi_str_len.")
		c.line("IF lv_wasi_str_len > 0.")
		c.indent++
		c.line("DATA(lv_wasi_bytes) = %s+lv_wasi_str_ptr(lv_wasi_str_len).", mem)
		c.line("\" Output bytes (could be WRITE or collect in buffer)")
		c.indent--
		c.line("ENDIF.")
		c.line("lv_wasi_written = lv_wasi_written + lv_wasi_str_len.")
		c.indent--
		c.line("ENDDO.")
		c.line("PERFORM mem_st_i32 USING %s lv_wasi_written.", args[3])
		c.line("%s = 0. \" errno = success", result)

	case "fd_read":
		result := stack.push()
		c.line("\" WASI fd_read: stub (return 0 bytes read)")
		c.line("PERFORM mem_st_i32 USING %s 0.", args[3])
		c.line("%s = 0.", result)

	case "fd_close":
		result := stack.push()
		c.line("%s = 0. \" WASI fd_close: stub", result)

	case "fd_seek":
		result := stack.push()
		c.line("%s = 8. \" WASI fd_seek: EBADF", result)

	case "fd_fdstat_get":
		result := stack.push()
		c.line("\" WASI fd_fdstat_get: return filetype=regular")
		c.line("PERFORM mem_st_i32_8 USING %s 4.", args[1]) // filetype = regular file
		c.line("%s = 0.", result)

	case "clock_time_get":
		result := stack.push()
		c.line("\" WASI clock_time_get: return current time in nanoseconds")
		c.line("GET TIME STAMP FIELD DATA(lv_wasi_ts).")
		c.line("DATA(lv_wasi_ns) = CONV int8( lv_wasi_ts * 1000000000 ).")
		c.line("zcl_wasm_rt=>mem_st_i64( EXPORTING iv_val = lv_wasi_ns iv_addr = %s CHANGING cv_mem = %s ).", args[2], mem)
		c.line("%s = 0.", result)

	case "environ_sizes_get":
		result := stack.push()
		c.line("\" WASI environ_sizes_get: 0 env vars")
		c.line("PERFORM mem_st_i32 USING %s 0.", args[0])
		c.line("PERFORM mem_st_i32 USING %s 0.", args[1])
		c.line("%s = 0.", result)

	case "environ_get":
		result := stack.push()
		c.line("%s = 0. \" WASI environ_get: stub", result)

	case "proc_exit":
		c.line("\" WASI proc_exit: %s", args[0])
		c.line("RETURN. \" exit")

	default:
		// Pop all args, push result if needed
		if len(imp.Type.Results) > 0 {
			result := stack.push()
			c.line("%s = 0. \" WASI %s.%s: unimplemented stub", result, imp.Module, imp.Name)
		} else {
			c.line("\" WASI %s.%s: unimplemented stub", imp.Module, imp.Name)
		}
	}
}

func (c *compiler) findImport(funcIndex int) *Import {
	for i := range c.mod.Imports {
		if c.mod.Imports[i].Kind == 0 && c.mod.Imports[i].FuncIndex == funcIndex {
			return &c.mod.Imports[i]
		}
	}
	return nil
}

// emitChainedDATA produces a single chained DATA: statement for all locals + stack vars.
func emitChainedDATA(f *Function) string {
	var parts []string

	// Declared locals (after params)
	for i := 0; i < len(f.Locals); i++ {
		localIdx := len(f.Type.Params) + i
		parts = append(parts, fmt.Sprintf("l%d TYPE %s", localIdx, f.Locals[i].ABAPType()))
	}

	// Stack variables
	maxStack := estimateMaxStack(f.Code)
	for i := 0; i < maxStack; i++ {
		parts = append(parts, fmt.Sprintf("s%d TYPE i", i))
	}

	// Branch depth flag
	parts = append(parts, "lv_br TYPE i")

	if len(parts) == 0 {
		return "DATA lv_br TYPE i."
	}

	// Build chained declaration, splitting across lines if needed
	var lines []string
	current := "DATA: "
	for i, p := range parts {
		suffix := ","
		if i == len(parts)-1 {
			suffix = "."
		}
		entry := p + suffix
		// If adding this entry would exceed line limit, start new line
		if len(current)+len(entry) > 235 {
			// End current line with trailing comma already there
			lines = append(lines, current)
			current = "      " // continuation indent
		}
		current += " " + entry
	}
	lines = append(lines, current)

	return strings.Join(lines, "\n")
}

// globalPrefix returns "gv_g" for FUGR mode, "mv_g" for class mode.
func (c *compiler) globalPrefix() string {
	if c.useFUGR {
		return "gv_g"
	}
	return "mv_g"
}

// memVar returns the memory variable name.
func (c *compiler) memVar() string {
	if c.useFUGR {
		return "gv_mem"
	}
	return "mv_mem"
}

// memPagesVar returns the memory pages variable name.
func (c *compiler) memPagesVar() string {
	if c.useFUGR {
		return "gv_mem_pages"
	}
	return "mv_mem_pages"
}

func (c *compiler) localName(f *Function, index int) string {
	if index < len(f.Type.Params) {
		return fmt.Sprintf("p%d", index)
	}
	return fmt.Sprintf("l%d", index)
}

// --- Virtual Stack ---

type virtualStack struct {
	depth int
}

func (s *virtualStack) push() string {
	name := fmt.Sprintf("s%d", s.depth)
	s.depth++
	return name
}

func (s *virtualStack) pop() string {
	if s.depth <= 0 {
		return "s0"
	}
	s.depth--
	return fmt.Sprintf("s%d", s.depth)
}

func (s *virtualStack) peek() string {
	if s.depth <= 0 {
		return "s0"
	}
	return fmt.Sprintf("s%d", s.depth-1)
}

// --- Helpers ---

func (c *compiler) line(format string, args ...any) {
	stmt := fmt.Sprintf(format, args...)
	if c.packLines && c.packer != nil {
		c.packer.setIndent(c.indent)
		c.packer.add(stmt)
		return
	}
	prefix := strings.Repeat("  ", c.indent)
	c.sb.WriteString(prefix)
	c.sb.WriteString(stmt)
	c.sb.WriteByte('\n')
}

// flushPacker writes any pending packed statements.
func (c *compiler) flushPacker() {
	if c.packer != nil {
		c.packer.flush()
	}
}

func sanitizeABAP(name string) string {
	// ABAP identifiers: max 30 chars, alphanumeric + underscore
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "$", "_")
	if len(name) > 30 {
		name = name[:30]
	}
	return name
}

func bytesToHex(data []byte) string {
	var sb strings.Builder
	for _, b := range data {
		fmt.Fprintf(&sb, "%02X", b)
	}
	return sb.String()
}

func estimateMaxStack(code []Instruction) int {
	max := 0
	depth := 0
	for _, inst := range code {
		switch inst.Op {
		case OpI32Const, OpI64Const, OpF32Const, OpF64Const,
			OpLocalGet, OpGlobalGet, OpMemorySize:
			depth++
		case OpLocalSet, OpGlobalSet, OpDrop,
			OpI32Store, OpI64Store, OpF32Store, OpF64Store,
			OpI32Store8, OpI32Store16, OpI64Store8, OpI64Store16, OpI64Store32,
			OpBrIf:
			depth--
		case OpI32Add, OpI32Sub, OpI32Mul, OpI32DivS, OpI32DivU,
			OpI32RemS, OpI32RemU, OpI32And, OpI32Or, OpI32Xor,
			OpI32Shl, OpI32ShrS, OpI32ShrU,
			OpI32Eq, OpI32Ne, OpI32LtS, OpI32LtU, OpI32GtS, OpI32GtU,
			OpI32LeS, OpI32LeU, OpI32GeS, OpI32GeU:
			depth-- // pop 2, push 1
		case OpSelect:
			depth -= 2 // pop 3, push 1
		}
		if depth < 0 {
			depth = 0
		}
		if depth > max {
			max = depth
		}
	}
	if max < 8 {
		max = 8
	}
	return max
}
