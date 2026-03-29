package wasmcomp

import (
	"fmt"
	"strings"
)

// CompileResult holds the output of multi-class compilation.
type CompileResult struct {
	MainClass   string            // main class source (memory, globals, exports, WASI)
	ChunkClasses map[string]string // chunk class name → source
	RuntimeClass string           // zcl_wasm_rt source
	Stats       CompileStats
}

// CompileStats contains compilation statistics.
type CompileStats struct {
	TotalFunctions    int
	DuplicateFunctions int
	SavedInstructions int
	ChunkCount        int
	TotalLines        int
	FuncsPerChunk     int
}

// CompileMultiClass compiles a WASM module into multiple ABAP classes.
// Functions are distributed across chunk classes (max funcsPerChunk each).
// A main class holds memory, globals, exports, WASI shim, and delegates.
func CompileMultiClass(mod *Module, baseName string, funcsPerChunk int) *CompileResult {
	if funcsPerChunk <= 0 {
		funcsPerChunk = 80
	}

	// Deduplicate functions
	redirects := DeduplicateFunctions(mod)
	dupes, _, savedInstrs := DedupStats(mod, redirects)

	// Build function name map and chunk assignments
	type funcInfo struct {
		localIdx   int
		name       string
		chunkIdx   int
		isDupe     bool
		canonName  string
		canonChunk int
	}

	funcMap := make([]funcInfo, len(mod.Functions))
	activeFuncCount := 0

	for i, f := range mod.Functions {
		fi := funcInfo{localIdx: i}
		if f.ExportName != "" {
			fi.name = sanitizeABAP(f.ExportName)
		} else {
			fi.name = fmt.Sprintf("f%d", i)
		}

		if canonIdx, ok := redirects[i]; ok {
			fi.isDupe = true
			cf := mod.Functions[canonIdx]
			if cf.ExportName != "" {
				fi.canonName = sanitizeABAP(cf.ExportName)
			} else {
				fi.canonName = fmt.Sprintf("f%d", canonIdx)
			}
			fi.canonChunk = canonIdx / funcsPerChunk
			fi.chunkIdx = canonIdx / funcsPerChunk // map to canonical's chunk
		} else {
			fi.chunkIdx = activeFuncCount / funcsPerChunk
			activeFuncCount++
		}

		funcMap[i] = fi
	}

	// Determine chunk assignments for non-duplicate functions
	chunkAssign := make([]int, len(mod.Functions))
	slot := 0
	for i := range mod.Functions {
		if _, ok := redirects[i]; ok {
			// Duplicate — assigned to canonical's chunk
			canonIdx := redirects[i]
			chunkAssign[i] = chunkAssign[canonIdx]
		} else {
			chunkAssign[i] = slot / funcsPerChunk
			slot++
		}
	}

	numChunks := 0
	if slot > 0 {
		numChunks = (slot-1)/funcsPerChunk + 1
	}

	result := &CompileResult{
		ChunkClasses: make(map[string]string),
		Stats: CompileStats{
			TotalFunctions:    len(mod.Functions),
			DuplicateFunctions: dupes,
			SavedInstructions: savedInstrs,
			ChunkCount:        numChunks,
			FuncsPerChunk:     funcsPerChunk,
		},
	}

	// Generate chunk classes
	for chunkIdx := 0; chunkIdx < numChunks; chunkIdx++ {
		chunkName := fmt.Sprintf("%s_c%02d", baseName, chunkIdx)
		src := emitChunkClass(mod, chunkName, baseName, chunkIdx, funcsPerChunk, chunkAssign, redirects)
		result.ChunkClasses[chunkName] = src
		result.Stats.TotalLines += strings.Count(src, "\n")
	}

	// Generate main class
	result.MainClass = emitMainClass(mod, baseName, numChunks, funcsPerChunk, chunkAssign, redirects)
	result.Stats.TotalLines += strings.Count(result.MainClass, "\n")

	// Generate runtime class
	result.RuntimeClass = emitRuntimeClass()
	result.Stats.TotalLines += strings.Count(result.RuntimeClass, "\n")

	return result
}

// --- Chunk Class ---

func emitChunkClass(mod *Module, chunkName, baseName string, chunkIdx, funcsPerChunk int, chunkAssign []int, redirects map[int]int) string {
	c := &compiler{mod: mod, className: chunkName}

	c.line("CLASS %s DEFINITION PUBLIC FINAL CREATE PUBLIC.", chunkName)
	c.indent++
	c.line("PUBLIC SECTION.")
	c.indent++

	// Reference to main class (for memory, globals)
	c.line("DATA mo_main TYPE REF TO %s.", baseName)

	// Declare methods for functions in this chunk
	for i, f := range mod.Functions {
		if chunkAssign[i] != chunkIdx {
			continue
		}
		if _, ok := redirects[i]; ok {
			continue // skip duplicates
		}
		if f.Type == nil {
			continue
		}
		name := fmt.Sprintf("f%d", i)
		if f.ExportName != "" {
			name = sanitizeABAP(f.ExportName)
		}
		c.emitMethodSignature(name, f.Type, false)
	}

	c.indent--
	c.indent--
	c.line("ENDCLASS.")
	c.line("")
	c.line("CLASS %s IMPLEMENTATION.", chunkName)
	c.indent++

	// Emit function implementations
	for i := range mod.Functions {
		f := &mod.Functions[i]
		if chunkAssign[i] != chunkIdx {
			continue
		}
		if _, ok := redirects[i]; ok {
			continue
		}
		if f.Type == nil {
			continue
		}
		name := fmt.Sprintf("f%d", i)
		if f.ExportName != "" {
			name = sanitizeABAP(f.ExportName)
		}
		c.emitFunctionWithMainRef(name, f, baseName, chunkAssign, redirects)
	}

	c.indent--
	c.line("ENDCLASS.")

	return c.sb.String()
}

// emitFunctionWithMainRef generates a method that accesses memory/globals via mo_main.
func (c *compiler) emitFunctionWithMainRef(name string, f *Function, baseName string, chunkAssign []int, redirects map[int]int) {
	c.line("METHOD %s.", sanitizeABAP(name))
	c.indent++

	// Locals
	for i := 0; i < len(f.Locals); i++ {
		localIdx := len(f.Type.Params) + i
		c.line("DATA l%d TYPE %s.", localIdx, f.Locals[i].ABAPType())
	}

	maxStack := estimateMaxStack(f.Code)
	for i := 0; i < maxStack; i++ {
		c.line("DATA s%d TYPE i.", i)
	}
	c.line("DATA lv_br TYPE i.")

	stack := &virtualStack{}
	c.blockStack = nil

	// Emit instructions — memory/global access goes through mo_main
	c.emitInstructions(f, f.Code, stack, 0)

	if len(f.Type.Results) > 0 && stack.depth > 0 {
		c.line("rv = %s.", stack.peek())
	}

	c.indent--
	c.line("ENDMETHOD.")
}

// --- Main Class ---

func emitMainClass(mod *Module, baseName string, numChunks, funcsPerChunk int, chunkAssign []int, redirects map[int]int) string {
	c := &compiler{mod: mod, className: baseName}

	c.line("CLASS %s DEFINITION PUBLIC FINAL CREATE PUBLIC.", baseName)
	c.indent++
	c.line("PUBLIC SECTION.")
	c.indent++

	c.line("METHODS constructor.")

	// Memory and globals are public so chunk classes can access them
	c.line("DATA mv_mem TYPE xstring.")
	c.line("DATA mv_mem_pages TYPE i.")
	for i, g := range mod.Globals {
		c.line("DATA mv_g%d TYPE %s.", i, g.Type.ABAPType())
	}

	// Exported functions
	for _, f := range mod.Functions {
		if f.ExportName != "" && f.Type != nil {
			c.emitMethodSignature(f.ExportName, f.Type, true)
		}
	}

	// Memory helpers
	c.line("METHODS mem_ld_i32 IMPORTING iv_addr TYPE i RETURNING VALUE(rv) TYPE i.")
	c.line("METHODS mem_st_i32 IMPORTING iv_addr TYPE i iv_val TYPE i.")
	c.line("METHODS mem_ld_i32_8u IMPORTING iv_addr TYPE i RETURNING VALUE(rv) TYPE i.")
	c.line("METHODS mem_ld_i32_8s IMPORTING iv_addr TYPE i RETURNING VALUE(rv) TYPE i.")
	c.line("METHODS mem_ld_i32_16u IMPORTING iv_addr TYPE i RETURNING VALUE(rv) TYPE i.")
	c.line("METHODS mem_st_i32_8 IMPORTING iv_addr TYPE i iv_val TYPE i.")
	c.line("METHODS mem_st_i32_16 IMPORTING iv_addr TYPE i iv_val TYPE i.")
	c.line("METHODS mem_grow IMPORTING iv_pages TYPE i RETURNING VALUE(rv) TYPE i.")

	c.indent--
	c.line("PRIVATE SECTION.")
	c.indent++

	// Chunk class references
	for i := 0; i < numChunks; i++ {
		c.line("DATA mo_c%02d TYPE REF TO %s_c%02d.", i, baseName, i)
	}

	// Function table
	for i := range mod.Elements {
		c.line("DATA mt_tab%d TYPE STANDARD TABLE OF i WITH DEFAULT KEY.", i)
	}

	// WASI
	c.line("DATA mo_wasi TYPE REF TO zcl_wasm_wasi.")

	c.indent--
	c.indent--
	c.line("ENDCLASS.")
	c.line("")
	c.line("CLASS %s IMPLEMENTATION.", baseName)
	c.indent++

	// Constructor
	c.line("METHOD constructor.")
	c.indent++

	// Create chunk classes
	for i := 0; i < numChunks; i++ {
		c.line("mo_c%02d = NEW #( ).", i)
		c.line("mo_c%02d->mo_main = me.", i)
	}

	// WASI
	c.line("mo_wasi = NEW #( ).")

	// Memory init
	if mod.Memory != nil {
		pages := mod.Memory.Min
		if pages == 0 {
			pages = 1
		}
		totalBytes := pages * 65536
		c.line("mv_mem_pages = %d.", pages)
		c.line("mv_mem = zcl_wasm_rt=>alloc_mem( %d ).", totalBytes)
	}

	// Globals
	for i, g := range mod.Globals {
		if g.InitI32 != 0 {
			c.line("mv_g%d = %d.", i, g.InitI32)
		} else if g.InitI64 != 0 {
			c.line("mv_g%d = %d.", i, g.InitI64)
		}
	}

	// Data segments — batch init for efficiency
	for _, seg := range mod.Data {
		if len(seg.Data) > 0 {
			hex := bytesToHex(seg.Data)
			// Split very long hex strings
			if len(hex) > 200 {
				c.line("zcl_wasm_rt=>mem_init( EXPORTING iv_off = %d iv_hex = '%s' CHANGING cv_mem = mv_mem ).", seg.Offset, hex)
			} else {
				c.line("mv_mem+%d(%d) = '%s'.", seg.Offset, len(seg.Data), hex)
			}
		}
	}

	// Element segments
	for i, elem := range mod.Elements {
		for _, funcIdx := range elem.FuncIndices {
			c.line("APPEND %d TO mt_tab%d.", funcIdx, i)
		}
	}

	c.indent--
	c.line("ENDMETHOD.")

	// Memory helpers
	c.emitMemoryHelpers()

	// Exported functions — delegate to chunk classes
	for i, f := range mod.Functions {
		if f.ExportName == "" || f.Type == nil {
			continue
		}
		c.line("METHOD %s.", sanitizeABAP(f.ExportName))
		c.indent++

		// Find the actual target (might be deduplicated)
		targetIdx := i
		if canonIdx, ok := redirects[i]; ok {
			targetIdx = canonIdx
		}
		chunkIdx := chunkAssign[targetIdx]
		targetName := fmt.Sprintf("f%d", targetIdx)
		if mod.Functions[targetIdx].ExportName != "" {
			targetName = sanitizeABAP(mod.Functions[targetIdx].ExportName)
		}

		// Build delegation call
		var params []string
		for j := range f.Type.Params {
			params = append(params, fmt.Sprintf("p%d = p%d", j, j))
		}
		paramStr := strings.Join(params, " ")

		if len(f.Type.Results) > 0 {
			if len(params) > 0 {
				c.line("rv = mo_c%02d->%s( %s ).", chunkIdx, targetName, paramStr)
			} else {
				c.line("rv = mo_c%02d->%s( ).", chunkIdx, targetName)
			}
		} else {
			if len(params) > 0 {
				c.line("mo_c%02d->%s( %s ).", chunkIdx, targetName, paramStr)
			} else {
				c.line("mo_c%02d->%s( ).", chunkIdx, targetName)
			}
		}

		c.indent--
		c.line("ENDMETHOD.")
	}

	c.indent--
	c.line("ENDCLASS.")

	return c.sb.String()
}

// --- Runtime Class ---

func emitRuntimeClass() string {
	return `CLASS zcl_wasm_rt DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    " Memory allocation
    CLASS-METHODS alloc_mem IMPORTING iv_size TYPE i RETURNING VALUE(rv_mem) TYPE xstring.
    CLASS-METHODS mem_init IMPORTING iv_off TYPE i iv_hex TYPE string CHANGING cv_mem TYPE xstring.
    CLASS-METHODS mem_copy IMPORTING iv_dst TYPE i iv_src TYPE i iv_n TYPE i CHANGING cv_mem TYPE xstring.
    CLASS-METHODS mem_fill IMPORTING iv_dst TYPE i iv_val TYPE i iv_n TYPE i CHANGING cv_mem TYPE xstring.
    " Unsigned 32-bit ops
    CLASS-METHODS div_u32 IMPORTING iv_a TYPE i iv_b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS rem_u32 IMPORTING iv_a TYPE i iv_b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS lt_u32 IMPORTING iv_a TYPE i iv_b TYPE i RETURNING VALUE(rv) TYPE abap_bool.
    CLASS-METHODS gt_u32 IMPORTING iv_a TYPE i iv_b TYPE i RETURNING VALUE(rv) TYPE abap_bool.
    CLASS-METHODS le_u32 IMPORTING iv_a TYPE i iv_b TYPE i RETURNING VALUE(rv) TYPE abap_bool.
    CLASS-METHODS ge_u32 IMPORTING iv_a TYPE i iv_b TYPE i RETURNING VALUE(rv) TYPE abap_bool.
    " Bitwise 32
    CLASS-METHODS and32 IMPORTING iv_a TYPE i iv_b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS or32 IMPORTING iv_a TYPE i iv_b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS xor32 IMPORTING iv_a TYPE i iv_b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS shl32 IMPORTING iv_val TYPE i iv_shift TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS shr_s32 IMPORTING iv_val TYPE i iv_shift TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS shr_u32 IMPORTING iv_val TYPE i iv_shift TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS rotl32 IMPORTING iv_val TYPE i iv_shift TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS rotr32 IMPORTING iv_val TYPE i iv_shift TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS clz32 IMPORTING iv_val TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS ctz32 IMPORTING iv_val TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS popcnt32 IMPORTING iv_val TYPE i RETURNING VALUE(rv) TYPE i.
    " Unsigned 64-bit ops
    CLASS-METHODS div_u64 IMPORTING iv_a TYPE int8 iv_b TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS rem_u64 IMPORTING iv_a TYPE int8 iv_b TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS lt_u64 IMPORTING iv_a TYPE int8 iv_b TYPE int8 RETURNING VALUE(rv) TYPE abap_bool.
    CLASS-METHODS gt_u64 IMPORTING iv_a TYPE int8 iv_b TYPE int8 RETURNING VALUE(rv) TYPE abap_bool.
    CLASS-METHODS le_u64 IMPORTING iv_a TYPE int8 iv_b TYPE int8 RETURNING VALUE(rv) TYPE abap_bool.
    CLASS-METHODS ge_u64 IMPORTING iv_a TYPE int8 iv_b TYPE int8 RETURNING VALUE(rv) TYPE abap_bool.
    " Bitwise 64
    CLASS-METHODS and64 IMPORTING iv_a TYPE int8 iv_b TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS or64 IMPORTING iv_a TYPE int8 iv_b TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS xor64 IMPORTING iv_a TYPE int8 iv_b TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS shl64 IMPORTING iv_val TYPE int8 iv_shift TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS shr_s64 IMPORTING iv_val TYPE int8 iv_shift TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS shr_u64 IMPORTING iv_val TYPE int8 iv_shift TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS rotl64 IMPORTING iv_val TYPE int8 iv_shift TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS rotr64 IMPORTING iv_val TYPE int8 iv_shift TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS clz64 IMPORTING iv_val TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS ctz64 IMPORTING iv_val TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS popcnt64 IMPORTING iv_val TYPE int8 RETURNING VALUE(rv) TYPE int8.
    " Conversions
    CLASS-METHODS wrap_i64 IMPORTING iv_val TYPE int8 RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS extend_u32 IMPORTING iv_val TYPE i RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS extend_u64_f IMPORTING iv_val TYPE int8 RETURNING VALUE(rv) TYPE f.
    CLASS-METHODS trunc_f_u32 IMPORTING iv_val TYPE f RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS trunc_f_u64 IMPORTING iv_val TYPE f RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS extend8s_i32 IMPORTING iv_val TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS extend16s_i32 IMPORTING iv_val TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS extend8s_i64 IMPORTING iv_val TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS extend16s_i64 IMPORTING iv_val TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS extend32s_i64 IMPORTING iv_val TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS copysign IMPORTING iv_mag TYPE f iv_sign TYPE f RETURNING VALUE(rv) TYPE f.
    CLASS-METHODS reinterpret_f32_i32 IMPORTING iv_val TYPE f RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS reinterpret_i32_f32 IMPORTING iv_val TYPE i RETURNING VALUE(rv) TYPE f.
    CLASS-METHODS reinterpret_f64_i64 IMPORTING iv_val TYPE f RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS reinterpret_i64_f64 IMPORTING iv_val TYPE int8 RETURNING VALUE(rv) TYPE f.
    " Memory load/store for i64, f32, f64
    CLASS-METHODS mem_ld_i64 IMPORTING iv_mem TYPE xstring iv_addr TYPE i RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS mem_st_i64 IMPORTING iv_val TYPE int8 iv_addr TYPE i CHANGING cv_mem TYPE xstring.
    CLASS-METHODS mem_ld_i64_ext IMPORTING iv_mem TYPE xstring iv_addr TYPE i iv_op TYPE i RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS mem_st_i64_trunc IMPORTING iv_val TYPE int8 iv_addr TYPE i iv_op TYPE i CHANGING cv_mem TYPE xstring.
    CLASS-METHODS mem_ld_i32_16s IMPORTING iv_mem TYPE xstring iv_addr TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS mem_ld_f32 IMPORTING iv_mem TYPE xstring iv_addr TYPE i RETURNING VALUE(rv) TYPE f.
    CLASS-METHODS mem_ld_f64 IMPORTING iv_mem TYPE xstring iv_addr TYPE i RETURNING VALUE(rv) TYPE f.
    CLASS-METHODS mem_st_f32 IMPORTING iv_val TYPE f iv_addr TYPE i CHANGING cv_mem TYPE xstring.
    CLASS-METHODS mem_st_f64 IMPORTING iv_val TYPE f iv_addr TYPE i CHANGING cv_mem TYPE xstring.
ENDCLASS.

CLASS zcl_wasm_rt IMPLEMENTATION.
  METHOD alloc_mem.
    " Allocate iv_size bytes of zeroed memory
    DATA lv_hex TYPE string.
    DATA lv_chunk TYPE x LENGTH 256.
    DATA(lv_chunks) = iv_size DIV 256.
    DATA(lv_remainder) = iv_size MOD 256.
    DO lv_chunks TIMES.
      CONCATENATE rv_mem lv_chunk INTO rv_mem IN BYTE MODE.
    ENDDO.
    IF lv_remainder > 0.
      DATA lv_small TYPE x LENGTH 1.
      DO lv_remainder TIMES.
        CONCATENATE rv_mem lv_small INTO rv_mem IN BYTE MODE.
      ENDDO.
    ENDIF.
  ENDMETHOD.

  METHOD mem_init.
    DATA(lv_bytes) = xstrlen( CONV xstring( iv_hex ) ).
    cv_mem+iv_off(lv_bytes) = iv_hex.
  ENDMETHOD.

  METHOD mem_copy.
    IF iv_n <= 0. RETURN. ENDIF.
    DATA(lv_src_data) = cv_mem+iv_src(iv_n).
    cv_mem+iv_dst(iv_n) = lv_src_data.
  ENDMETHOD.

  METHOD mem_fill.
    IF iv_n <= 0. RETURN. ENDIF.
    DATA lv_byte TYPE x LENGTH 1.
    lv_byte = iv_val.
    DO iv_n TIMES.
      DATA(lv_off) = iv_dst + sy-index - 1.
      cv_mem+lv_off(1) = lv_byte.
    ENDDO.
  ENDMETHOD.

  " === Unsigned 32-bit via INT8 promotion ===
  METHOD div_u32.
    DATA(lv_a) = CONV int8( iv_a ).
    DATA(lv_b) = CONV int8( iv_b ).
    IF lv_a < 0. lv_a = lv_a + 4294967296. ENDIF.
    IF lv_b < 0. lv_b = lv_b + 4294967296. ENDIF.
    rv = CONV i( lv_a DIV lv_b ).
  ENDMETHOD.
  METHOD rem_u32.
    DATA(lv_a) = CONV int8( iv_a ).
    DATA(lv_b) = CONV int8( iv_b ).
    IF lv_a < 0. lv_a = lv_a + 4294967296. ENDIF.
    IF lv_b < 0. lv_b = lv_b + 4294967296. ENDIF.
    rv = CONV i( lv_a MOD lv_b ).
  ENDMETHOD.
  METHOD lt_u32.
    DATA(lv_a) = CONV int8( iv_a ).
    DATA(lv_b) = CONV int8( iv_b ).
    IF lv_a < 0. lv_a = lv_a + 4294967296. ENDIF.
    IF lv_b < 0. lv_b = lv_b + 4294967296. ENDIF.
    rv = xsdbool( lv_a < lv_b ).
  ENDMETHOD.
  METHOD gt_u32.
    DATA(lv_a) = CONV int8( iv_a ).
    DATA(lv_b) = CONV int8( iv_b ).
    IF lv_a < 0. lv_a = lv_a + 4294967296. ENDIF.
    IF lv_b < 0. lv_b = lv_b + 4294967296. ENDIF.
    rv = xsdbool( lv_a > lv_b ).
  ENDMETHOD.
  METHOD le_u32.
    DATA(lv_a) = CONV int8( iv_a ).
    DATA(lv_b) = CONV int8( iv_b ).
    IF lv_a < 0. lv_a = lv_a + 4294967296. ENDIF.
    IF lv_b < 0. lv_b = lv_b + 4294967296. ENDIF.
    rv = xsdbool( lv_a <= lv_b ).
  ENDMETHOD.
  METHOD ge_u32.
    DATA(lv_a) = CONV int8( iv_a ).
    DATA(lv_b) = CONV int8( iv_b ).
    IF lv_a < 0. lv_a = lv_a + 4294967296. ENDIF.
    IF lv_b < 0. lv_b = lv_b + 4294967296. ENDIF.
    rv = xsdbool( lv_a >= lv_b ).
  ENDMETHOD.

  " === Bitwise 32 (via XSTRING) ===
  METHOD and32.
    DATA lv_a TYPE x LENGTH 4. DATA lv_b TYPE x LENGTH 4. DATA lv_r TYPE x LENGTH 4.
    lv_a = iv_a. lv_b = iv_b.
    lv_r = lv_a BIT-AND lv_b.
    rv = lv_r.
  ENDMETHOD.
  METHOD or32.
    DATA lv_a TYPE x LENGTH 4. DATA lv_b TYPE x LENGTH 4. DATA lv_r TYPE x LENGTH 4.
    lv_a = iv_a. lv_b = iv_b.
    lv_r = lv_a BIT-OR lv_b.
    rv = lv_r.
  ENDMETHOD.
  METHOD xor32.
    DATA lv_a TYPE x LENGTH 4. DATA lv_b TYPE x LENGTH 4. DATA lv_r TYPE x LENGTH 4.
    lv_a = iv_a. lv_b = iv_b.
    lv_r = lv_a BIT-XOR lv_b.
    rv = lv_r.
  ENDMETHOD.
  METHOD shl32.
    DATA(lv_shift) = iv_shift MOD 32.
    DATA(lv_val) = CONV int8( iv_val ).
    IF lv_val < 0. lv_val = lv_val + 4294967296. ENDIF.
    DATA(lv_result) = lv_val * ipow( base = 2 exp = lv_shift ).
    rv = CONV i( lv_result MOD 4294967296 ).
  ENDMETHOD.
  METHOD shr_s32.
    DATA(lv_shift) = iv_shift MOD 32.
    rv = iv_val DIV ipow( base = 2 exp = lv_shift ).
  ENDMETHOD.
  METHOD shr_u32.
    DATA(lv_shift) = iv_shift MOD 32.
    DATA(lv_val) = CONV int8( iv_val ).
    IF lv_val < 0. lv_val = lv_val + 4294967296. ENDIF.
    rv = CONV i( lv_val DIV ipow( base = 2 exp = lv_shift ) ).
  ENDMETHOD.
  METHOD rotl32. rv = iv_val. " TODO: implement. ENDMETHOD.
  METHOD rotr32. rv = iv_val. " TODO: implement. ENDMETHOD.
  METHOD clz32.
    DATA(lv_val) = CONV int8( iv_val ).
    IF lv_val < 0. lv_val = lv_val + 4294967296. ENDIF.
    IF lv_val = 0. rv = 32. RETURN. ENDIF.
    rv = 0.
    DATA(lv_mask) = CONV int8( 2147483648 ). " 1 << 31
    WHILE lv_val BIT-AND lv_mask = 0.
      rv = rv + 1.
      lv_mask = lv_mask DIV 2.
    ENDWHILE.
  ENDMETHOD.
  METHOD ctz32.
    DATA(lv_val) = CONV int8( iv_val ).
    IF lv_val < 0. lv_val = lv_val + 4294967296. ENDIF.
    IF lv_val = 0. rv = 32. RETURN. ENDIF.
    rv = 0.
    WHILE lv_val MOD 2 = 0.
      rv = rv + 1.
      lv_val = lv_val DIV 2.
    ENDWHILE.
  ENDMETHOD.
  METHOD popcnt32.
    DATA(lv_val) = CONV int8( iv_val ).
    IF lv_val < 0. lv_val = lv_val + 4294967296. ENDIF.
    rv = 0.
    WHILE lv_val > 0.
      IF lv_val MOD 2 = 1. rv = rv + 1. ENDIF.
      lv_val = lv_val DIV 2.
    ENDWHILE.
  ENDMETHOD.

  " === 64-bit stubs (implement as needed) ===
  METHOD div_u64. rv = iv_a DIV iv_b. " simplified. ENDMETHOD.
  METHOD rem_u64. rv = iv_a MOD iv_b. ENDMETHOD.
  METHOD lt_u64. rv = xsdbool( iv_a < iv_b ). ENDMETHOD.
  METHOD gt_u64. rv = xsdbool( iv_a > iv_b ). ENDMETHOD.
  METHOD le_u64. rv = xsdbool( iv_a <= iv_b ). ENDMETHOD.
  METHOD ge_u64. rv = xsdbool( iv_a >= iv_b ). ENDMETHOD.
  METHOD and64. DATA lv_a TYPE x LENGTH 8. DATA lv_b TYPE x LENGTH 8. lv_a = iv_a. lv_b = iv_b. DATA(lv_r) = lv_a BIT-AND lv_b. rv = lv_r. ENDMETHOD.
  METHOD or64. DATA lv_a TYPE x LENGTH 8. DATA lv_b TYPE x LENGTH 8. lv_a = iv_a. lv_b = iv_b. DATA(lv_r) = lv_a BIT-OR lv_b. rv = lv_r. ENDMETHOD.
  METHOD xor64. DATA lv_a TYPE x LENGTH 8. DATA lv_b TYPE x LENGTH 8. lv_a = iv_a. lv_b = iv_b. DATA(lv_r) = lv_a BIT-XOR lv_b. rv = lv_r. ENDMETHOD.
  METHOD shl64. rv = iv_val * ipow( base = 2 exp = CONV i( iv_shift MOD 64 ) ). ENDMETHOD.
  METHOD shr_s64. rv = iv_val DIV ipow( base = 2 exp = CONV i( iv_shift MOD 64 ) ). ENDMETHOD.
  METHOD shr_u64. rv = iv_val DIV ipow( base = 2 exp = CONV i( iv_shift MOD 64 ) ). " simplified. ENDMETHOD.
  METHOD rotl64. rv = iv_val. ENDMETHOD.
  METHOD rotr64. rv = iv_val. ENDMETHOD.
  METHOD clz64. rv = 0. DATA(lv) = iv_val. IF lv = 0. rv = 64. RETURN. ENDIF. DATA(lv_m) = CONV int8( '4000000000000000' ). WHILE lv BIT-AND lv_m = 0. rv = rv + 1. lv_m = lv_m DIV 2. ENDWHILE. ENDMETHOD.
  METHOD ctz64. rv = 0. DATA(lv) = iv_val. IF lv = 0. rv = 64. RETURN. ENDIF. WHILE lv MOD 2 = 0. rv = rv + 1. lv = lv DIV 2. ENDWHILE. ENDMETHOD.
  METHOD popcnt64. rv = 0. DATA(lv) = iv_val. WHILE lv > 0. IF lv MOD 2 = 1. rv = rv + 1. ENDIF. lv = lv DIV 2. ENDWHILE. ENDMETHOD.

  " === Conversions ===
  METHOD wrap_i64. rv = CONV i( iv_val MOD 4294967296 ). ENDMETHOD.
  METHOD extend_u32. rv = iv_val. IF rv < 0. rv = rv + 4294967296. ENDIF. ENDMETHOD.
  METHOD extend_u64_f. rv = iv_val. IF rv < 0. rv = rv + CONV f( '18446744073709551616' ). ENDIF. ENDMETHOD.
  METHOD trunc_f_u32. rv = trunc( iv_val ). ENDMETHOD.
  METHOD trunc_f_u64. rv = trunc( iv_val ). ENDMETHOD.
  METHOD extend8s_i32.
    rv = iv_val MOD 256.
    IF rv > 127. rv = rv - 256. ENDIF.
  ENDMETHOD.
  METHOD extend16s_i32.
    rv = iv_val MOD 65536.
    IF rv > 32767. rv = rv - 65536. ENDIF.
  ENDMETHOD.
  METHOD extend8s_i64. rv = iv_val MOD 256. IF rv > 127. rv = rv - 256. ENDIF. ENDMETHOD.
  METHOD extend16s_i64. rv = iv_val MOD 65536. IF rv > 32767. rv = rv - 65536. ENDIF. ENDMETHOD.
  METHOD extend32s_i64. rv = iv_val MOD 4294967296. IF rv > 2147483647. rv = rv - 4294967296. ENDIF. ENDMETHOD.
  METHOD copysign.
    rv = abs( iv_mag ).
    IF iv_sign < 0. rv = - rv. ENDIF.
  ENDMETHOD.
  METHOD reinterpret_f32_i32. rv = iv_val. " simplified. ENDMETHOD.
  METHOD reinterpret_i32_f32. rv = iv_val. ENDMETHOD.
  METHOD reinterpret_f64_i64. rv = iv_val. ENDMETHOD.
  METHOD reinterpret_i64_f64. rv = iv_val. ENDMETHOD.

  " === Memory i64/f32/f64 ===
  METHOD mem_ld_i64.
    DATA lv_b TYPE x LENGTH 8.
    lv_b = iv_mem+iv_addr(8).
    " Reverse little-endian
    DATA(lv_r) = lv_b+7(1) && lv_b+6(1) && lv_b+5(1) && lv_b+4(1) && lv_b+3(1) && lv_b+2(1) && lv_b+1(1) && lv_b+0(1).
    rv = lv_r.
  ENDMETHOD.
  METHOD mem_st_i64.
    DATA lv_b TYPE x LENGTH 8. lv_b = iv_val.
    DATA(lv_r) = lv_b+7(1) && lv_b+6(1) && lv_b+5(1) && lv_b+4(1) && lv_b+3(1) && lv_b+2(1) && lv_b+1(1) && lv_b+0(1).
    cv_mem+iv_addr(8) = lv_r.
  ENDMETHOD.
  METHOD mem_ld_i64_ext. rv = 0. " TODO: sign/zero extend variants. ENDMETHOD.
  METHOD mem_st_i64_trunc. " TODO: truncate variants. ENDMETHOD.
  METHOD mem_ld_i32_16s.
    DATA lv_b TYPE x LENGTH 2.
    lv_b = iv_mem+iv_addr(2).
    DATA(lv_r) = lv_b+1(1) && lv_b+0(1).
    rv = lv_r.
    IF rv > 32767. rv = rv - 65536. ENDIF.
  ENDMETHOD.
  METHOD mem_ld_f32. rv = 0. " TODO: IEEE 754 decode. ENDMETHOD.
  METHOD mem_ld_f64.
    DATA lv_b TYPE x LENGTH 8.
    lv_b = iv_mem+iv_addr(8).
    DATA(lv_r) = lv_b+7(1) && lv_b+6(1) && lv_b+5(1) && lv_b+4(1) && lv_b+3(1) && lv_b+2(1) && lv_b+1(1) && lv_b+0(1).
    rv = lv_r.
  ENDMETHOD.
  METHOD mem_st_f32. " TODO. ENDMETHOD.
  METHOD mem_st_f64.
    DATA lv_b TYPE x LENGTH 8. lv_b = iv_val.
    DATA(lv_r) = lv_b+7(1) && lv_b+6(1) && lv_b+5(1) && lv_b+4(1) && lv_b+3(1) && lv_b+2(1) && lv_b+1(1) && lv_b+0(1).
    cv_mem+iv_addr(8) = lv_r.
  ENDMETHOD.
ENDCLASS.
`
}
