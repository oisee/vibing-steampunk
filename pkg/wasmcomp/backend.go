package wasmcomp

import (
	"fmt"
	"strings"
)

// BackendKind selects the ABAP output strategy.
type BackendKind int

const (
	// BackendFUGR emits a function group with FORM-based functions.
	// Best for large modules: no size limits, shared globals, zero overhead.
	BackendFUGR BackendKind = iota

	// BackendClass emits a single class (for small modules only).
	// Simple but hits SAP limits around 74K lines.
	BackendClass

	// BackendHybrid emits a function group for internals + a class as public API.
	// Best of both: clean OO API, unlimited internal size.
	BackendHybrid
)

func (b BackendKind) String() string {
	switch b {
	case BackendFUGR:
		return "fugr"
	case BackendClass:
		return "class"
	case BackendHybrid:
		return "hybrid"
	}
	return "unknown"
}

// BackendResult holds generated ABAP sources for any backend.
type BackendResult struct {
	Backend  BackendKind
	Files    map[string]string // filename → source
	Stats    BackendStats
}

type BackendStats struct {
	TotalFunctions     int
	DuplicateFunctions int
	SavedInstructions  int
	FileCount          int
	TotalLines         int
	LargestFile        int
	LargestFileName    string
}

// CompileWith compiles a WASM module using the specified backend.
func CompileWith(mod *Module, name string, backend BackendKind, funcsPerInclude int) *BackendResult {
	if funcsPerInclude <= 0 {
		funcsPerInclude = 80
	}

	redirects := DeduplicateFunctions(mod)
	dupes, _, savedInstrs := DedupStats(mod, redirects)

	result := &BackendResult{
		Backend: backend,
		Files:   make(map[string]string),
		Stats: BackendStats{
			TotalFunctions:     len(mod.Functions),
			DuplicateFunctions: dupes,
			SavedInstructions:  savedInstrs,
		},
	}

	switch backend {
	case BackendFUGR:
		emitFUGR(mod, name, funcsPerInclude, redirects, result)
	case BackendClass:
		// Use the original single-class Compile for small modules
		src := Compile(mod, name)
		result.Files[name+".clas.abap"] = src
	case BackendHybrid:
		emitHybrid(mod, name, funcsPerInclude, redirects, result)
	}

	// Compute stats
	for fname, src := range result.Files {
		lines := strings.Count(src, "\n")
		result.Stats.TotalLines += lines
		result.Stats.FileCount++
		if lines > result.Stats.LargestFile {
			result.Stats.LargestFile = lines
			result.Stats.LargestFileName = fname
		}
	}

	return result
}

// --- FUGR Backend ---
// Emits: TOP include (globals), Fnn includes (functions), FM wrappers

func emitFUGR(mod *Module, fugrName string, funcsPerInclude int, redirects map[int]int, result *BackendResult) {
	upper := strings.ToUpper(fugrName)
	prefix := "L" + upper

	// TOP include — globals
	result.Files[prefix+"TOP.abap"] = emitFUGRTop(mod, upper)

	// Runtime include
	result.Files[prefix+"RT.abap"] = emitFUGRRuntime()

	// Function includes — group non-duplicate functions
	slot := 0
	includeIdx := 0
	var currentFuncs []int

	flushInclude := func() {
		if len(currentFuncs) == 0 {
			return
		}
		fname := fmt.Sprintf("%sF%02d.abap", prefix, includeIdx)
		result.Files[fname] = emitFUGRInclude(mod, currentFuncs, redirects, upper)
		includeIdx++
		currentFuncs = nil
	}

	for i := range mod.Functions {
		if _, ok := redirects[i]; ok {
			continue // skip duplicates
		}
		if mod.Functions[i].Type == nil {
			continue
		}
		currentFuncs = append(currentFuncs, i)
		slot++
		if slot%funcsPerInclude == 0 {
			flushInclude()
		}
	}
	flushInclude()

	// Init include — memory/data/element initialization
	result.Files[prefix+"INIT.abap"] = emitFUGRInit(mod, upper)

	// Function module wrappers for exports
	for _, f := range mod.Functions {
		if f.ExportName != "" && f.Type != nil {
			fmName := upper + "_" + strings.ToUpper(sanitizeABAP(f.ExportName))
			result.Files[fmName+".func.abap"] = emitFMWrapper(mod, &f, fmName, redirects)
		}
	}
}

func emitFUGRTop(mod *Module, upper string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("FUNCTION-POOL %s.\n\n", lower(upper)))

	// Memory
	sb.WriteString("\" Linear memory (WASM)\n")
	sb.WriteString("DATA gv_mem TYPE xstring.\n")
	sb.WriteString("DATA gv_mem_pages TYPE i.\n")
	sb.WriteString("DATA gv_initialized TYPE abap_bool.\n\n")

	// Globals
	for i, g := range mod.Globals {
		sb.WriteString(fmt.Sprintf("DATA gv_g%d TYPE %s.\n", i, g.Type.ABAPType()))
	}

	// Function table
	for i := range mod.Elements {
		sb.WriteString(fmt.Sprintf("DATA gt_tab%d TYPE STANDARD TABLE OF i WITH DEFAULT KEY.\n", i))
	}

	sb.WriteString("\n")
	return sb.String()
}

func emitFUGRRuntime() string {
	return `" WASM Runtime helpers — included in function group
" Memory load/store (little-endian)

FORM mem_ld_i32 USING iv_addr TYPE i CHANGING rv TYPE i.
  DATA lv_b TYPE x LENGTH 4.
  lv_b = gv_mem+iv_addr(4).
  DATA(lv_r) = lv_b+3(1) && lv_b+2(1) && lv_b+1(1) && lv_b+0(1).
  rv = lv_r.
ENDFORM.

FORM mem_st_i32 USING iv_addr TYPE i iv_val TYPE i.
  DATA lv_b TYPE x LENGTH 4.
  lv_b = iv_val.
  DATA(lv_r) = lv_b+3(1) && lv_b+2(1) && lv_b+1(1) && lv_b+0(1).
  gv_mem+iv_addr(4) = lv_r.
ENDFORM.

FORM mem_ld_i32_8u USING iv_addr TYPE i CHANGING rv TYPE i.
  DATA lv_b TYPE x LENGTH 1.
  lv_b = gv_mem+iv_addr(1).
  rv = lv_b.
ENDFORM.

FORM mem_ld_i32_8s USING iv_addr TYPE i CHANGING rv TYPE i.
  DATA lv_b TYPE x LENGTH 1.
  lv_b = gv_mem+iv_addr(1).
  rv = lv_b.
  IF rv > 127. rv = rv - 256. ENDIF.
ENDFORM.

FORM mem_st_i32_8 USING iv_addr TYPE i iv_val TYPE i.
  DATA lv_b TYPE x LENGTH 1.
  lv_b = iv_val.
  gv_mem+iv_addr(1) = lv_b.
ENDFORM.

FORM mem_ld_i32_16u USING iv_addr TYPE i CHANGING rv TYPE i.
  DATA lv_b TYPE x LENGTH 2.
  lv_b = gv_mem+iv_addr(2).
  DATA(lv_r) = lv_b+1(1) && lv_b+0(1).
  rv = lv_r.
ENDFORM.

FORM mem_st_i32_16 USING iv_addr TYPE i iv_val TYPE i.
  DATA lv_b TYPE x LENGTH 2.
  lv_b = iv_val.
  DATA(lv_r) = lv_b+1(1) && lv_b+0(1).
  gv_mem+iv_addr(2) = lv_r.
ENDFORM.

FORM mem_grow USING iv_pages TYPE i CHANGING rv TYPE i.
  rv = gv_mem_pages.
  DATA lv_zeros TYPE xstring.
  DATA(lv_new_bytes) = iv_pages * 65536.
  DATA lv_chunk TYPE x LENGTH 256.
  DATA(lv_chunks) = lv_new_bytes DIV 256.
  DO lv_chunks TIMES.
    CONCATENATE lv_zeros lv_chunk INTO lv_zeros IN BYTE MODE.
  ENDDO.
  CONCATENATE gv_mem lv_zeros INTO gv_mem IN BYTE MODE.
  gv_mem_pages = gv_mem_pages + iv_pages.
ENDFORM.
`
}

func emitFUGRInclude(mod *Module, funcIndices []int, redirects map[int]int, upper string) string {
	c := &compiler{mod: mod}

	for _, i := range funcIndices {
		f := &mod.Functions[i]
		if f.Type == nil {
			continue
		}
		emitFORM(c, f, i, mod, redirects)
	}

	return c.sb.String()
}

func emitFORM(c *compiler, f *Function, funcIdx int, mod *Module, redirects map[int]int) {
	name := fmt.Sprintf("f%d", funcIdx)
	if f.ExportName != "" {
		name = sanitizeABAP(f.ExportName)
	}

	// FORM signature
	var parts []string
	for i, p := range f.Type.Params {
		parts = append(parts, fmt.Sprintf("p%d TYPE %s", i, p.ABAPType()))
	}

	if len(f.Type.Results) > 0 {
		// USING params CHANGING rv
		if len(parts) > 0 {
			c.line("FORM %s USING %s CHANGING rv TYPE %s.",
				name, strings.Join(parts, " "), f.Type.Results[0].ABAPType())
		} else {
			c.line("FORM %s CHANGING rv TYPE %s.", name, f.Type.Results[0].ABAPType())
		}
	} else {
		if len(parts) > 0 {
			c.line("FORM %s USING %s.", name, strings.Join(parts, " "))
		} else {
			c.line("FORM %s.", name)
		}
	}
	c.indent++

	// Chained DATA declaration
	c.line("%s", emitChainedDATA(f))

	// Enable packing for code
	c.packLines = true
	c.packer = newLinePacker(&c.sb, c.indent)

	// Emit instructions with FUGR-style
	stack := &virtualStack{}
	c.blockStack = nil
	c.useFUGR = true
	c.fugrRedirects = redirects
	c.emitInstructions(f, f.Code, stack, 0)

	if len(f.Type.Results) > 0 && stack.depth > 0 {
		c.line("rv = %s.", stack.peek())
	}

	c.indent--
	c.flushPacker()
	c.packLines = false
	c.packer = nil
	c.line("ENDFORM.")
	c.line("")
}

func emitFUGRInit(mod *Module, upper string) string {
	var sb strings.Builder

	sb.WriteString("FORM wasm_init.\n")
	sb.WriteString("  IF gv_initialized = abap_true. RETURN. ENDIF.\n")
	sb.WriteString("  gv_initialized = abap_true.\n\n")

	// Memory
	if mod.Memory != nil {
		pages := mod.Memory.Min
		if pages == 0 {
			pages = 1
		}
		totalBytes := pages * 65536
		sb.WriteString(fmt.Sprintf("  gv_mem_pages = %d.\n", pages))
		sb.WriteString(fmt.Sprintf("  gv_mem = zcl_wasm_rt=>alloc_mem( %d ).\n\n", totalBytes))
	}

	// Globals
	for i, g := range mod.Globals {
		if g.InitI32 != 0 {
			sb.WriteString(fmt.Sprintf("  gv_g%d = %d.\n", i, g.InitI32))
		} else if g.InitI64 != 0 {
			sb.WriteString(fmt.Sprintf("  gv_g%d = %d.\n", i, g.InitI64))
		}
	}
	sb.WriteString("\n")

	// Data segments
	for _, seg := range mod.Data {
		if len(seg.Data) > 0 {
			hex := bytesToHex(seg.Data)
			if len(hex) <= 200 {
				sb.WriteString(fmt.Sprintf("  gv_mem+%d(%d) = '%s'.\n", seg.Offset, len(seg.Data), hex))
			} else {
				// Split long hex into chunks
				for off := 0; off < len(hex); off += 200 {
					end := off + 200
					if end > len(hex) {
						end = len(hex)
					}
					chunk := hex[off:end]
					byteOff := seg.Offset + off/2
					byteLen := (end - off) / 2
					sb.WriteString(fmt.Sprintf("  gv_mem+%d(%d) = '%s'.\n", byteOff, byteLen, chunk))
				}
			}
		}
	}
	sb.WriteString("\n")

	// Element segments
	for i, elem := range mod.Elements {
		for _, funcIdx := range elem.FuncIndices {
			sb.WriteString(fmt.Sprintf("  APPEND %d TO gt_tab%d.\n", funcIdx, i))
		}
	}

	sb.WriteString("ENDFORM.\n")
	return sb.String()
}

func emitFMWrapper(mod *Module, f *Function, fmName string, redirects map[int]int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("FUNCTION %s.\n", lower(fmName)))

	// Build signature comments
	for i, p := range f.Type.Params {
		sb.WriteString(fmt.Sprintf("*\"  IMPORTING\n*\"    IV_P%d TYPE %s\n", i, strings.ToUpper(p.ABAPType())))
	}
	if len(f.Type.Results) > 0 {
		sb.WriteString(fmt.Sprintf("*\"  EXPORTING\n*\"    EV_RESULT TYPE %s\n", strings.ToUpper(f.Type.Results[0].ABAPType())))
	}

	sb.WriteString("\n")
	sb.WriteString("  PERFORM wasm_init.\n\n")

	// Find target (might be redirected)
	targetIdx := f.Index - mod.NumImportedFuncs
	if canonIdx, ok := redirects[targetIdx]; ok {
		targetIdx = canonIdx
	}
	targetName := fmt.Sprintf("f%d", targetIdx)
	if mod.Functions[targetIdx].ExportName != "" {
		targetName = sanitizeABAP(mod.Functions[targetIdx].ExportName)
	}

	// PERFORM call
	var params []string
	for i := range f.Type.Params {
		params = append(params, fmt.Sprintf("iv_p%d", i))
	}

	if len(f.Type.Results) > 0 {
		if len(params) > 0 {
			sb.WriteString(fmt.Sprintf("  PERFORM %s USING %s CHANGING ev_result.\n",
				targetName, strings.Join(params, " ")))
		} else {
			sb.WriteString(fmt.Sprintf("  PERFORM %s CHANGING ev_result.\n", targetName))
		}
	} else {
		if len(params) > 0 {
			sb.WriteString(fmt.Sprintf("  PERFORM %s USING %s.\n", targetName, strings.Join(params, " ")))
		} else {
			sb.WriteString(fmt.Sprintf("  PERFORM %s.\n", targetName))
		}
	}

	sb.WriteString(fmt.Sprintf("ENDFUNCTION.\n"))
	return sb.String()
}

// --- Hybrid Backend ---
// FUGR for internals + class as public API wrapper

func emitHybrid(mod *Module, name string, funcsPerInclude int, redirects map[int]int, result *BackendResult) {
	// Generate FUGR internals
	fugrName := name + "_int"
	emitFUGR(mod, fugrName, funcsPerInclude, redirects, result)

	// Generate wrapper class
	result.Files[name+".clas.abap"] = emitHybridClass(mod, name, fugrName)
}

func emitHybridClass(mod *Module, className, fugrName string) string {
	c := &compiler{mod: mod, className: className}

	c.line("CLASS %s DEFINITION PUBLIC FINAL CREATE PUBLIC.", className)
	c.indent++
	c.line("PUBLIC SECTION.")
	c.indent++

	c.line("METHODS constructor.")

	// Public methods for exports
	for _, f := range mod.Functions {
		if f.ExportName != "" && f.Type != nil {
			c.emitMethodSignature(f.ExportName, f.Type, true)
		}
	}

	c.indent--
	c.indent--
	c.line("ENDCLASS.")
	c.line("")
	c.line("CLASS %s IMPLEMENTATION.", className)
	c.indent++

	// Constructor — call FUGR init
	c.line("METHOD constructor.")
	c.indent++
	c.line("PERFORM wasm_init IN PROGRAM sapl%s.", lower(fugrName))
	c.indent--
	c.line("ENDMETHOD.")

	// Export methods delegate to FMs
	upper := strings.ToUpper(fugrName)
	for _, f := range mod.Functions {
		if f.ExportName == "" || f.Type == nil {
			continue
		}
		fmName := upper + "_" + strings.ToUpper(sanitizeABAP(f.ExportName))

		c.line("METHOD %s.", sanitizeABAP(f.ExportName))
		c.indent++

		var importParams []string
		for i := range f.Type.Params {
			importParams = append(importParams, fmt.Sprintf("iv_p%d = p%d", i, i))
		}

		if len(f.Type.Results) > 0 {
			c.line("CALL FUNCTION '%s'", lower(fmName))
			c.indent++
			if len(importParams) > 0 {
				c.line("EXPORTING %s", strings.Join(importParams, " "))
			}
			c.line("IMPORTING ev_result = rv.")
			c.indent--
		} else {
			if len(importParams) > 0 {
				c.line("CALL FUNCTION '%s' EXPORTING %s.", lower(fmName), strings.Join(importParams, " "))
			} else {
				c.line("CALL FUNCTION '%s'.", lower(fmName))
			}
		}

		c.indent--
		c.line("ENDMETHOD.")
	}

	c.indent--
	c.line("ENDCLASS.")

	return c.sb.String()
}

func lower(s string) string {
	return strings.ToLower(s)
}
