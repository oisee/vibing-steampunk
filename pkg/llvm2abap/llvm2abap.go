// Package llvm2abap compiles LLVM IR text format to idiomatic typed ABAP.
package llvm2abap

import (
	"fmt"
	"regexp"
	"strings"
)

// Module represents a parsed LLVM IR module.
type Module struct {
	SourceFile string
	Types      map[string]*StructType // named struct types
	Functions  []*Function
	Globals    []*Global
}

// StructType represents an LLVM named struct type.
type StructType struct {
	Name   string
	Fields []Type
}

// Type represents an LLVM IR type.
type Type struct {
	Kind     TypeKind
	BitWidth int    // for IntType
	Name     string // for StructType (named)
	Elem     *Type  // for PtrType, ArrayType
	Len      int    // for ArrayType
}

type TypeKind int

const (
	VoidType TypeKind = iota
	IntType
	FloatType
	DoubleType
	PtrType
	StructTypeKind
	ArrayTypeKind
)

func (t Type) ABAPType() string {
	switch t.Kind {
	case IntType:
		switch {
		case t.BitWidth <= 32:
			return "i"
		case t.BitWidth <= 64:
			return "int8"
		default:
			return "i"
		}
	case FloatType, DoubleType:
		return "f"
	case PtrType:
		return "i" // pointer = offset
	case VoidType:
		return ""
	default:
		return "i"
	}
}

// Function represents an LLVM IR function.
type Function struct {
	Name       string
	ReturnType Type
	Params     []Param
	Blocks     []*BasicBlock
	IsExternal bool // declare (no body)
}

// Param represents a function parameter.
type Param struct {
	Name string // %0, %1, etc.
	Type Type
}

// BasicBlock represents an LLVM basic block.
type BasicBlock struct {
	Label string
	Insts []*Instruction
}

// Instruction represents a single LLVM IR instruction.
type Instruction struct {
	Result string // destination register (%3, etc.)
	Op     string // add, sub, mul, icmp, br, ret, ...
	Type   Type
	Args   []string
	// For icmp
	Predicate string // eq, ne, slt, sgt, sle, sge, ult, ugt
	// For br
	Cond      string
	TrueLabel string
	FalseLabel string
	// For phi
	PhiPairs []PhiPair
	// For call
	CallTarget string
	// For select
	SelectCond  string
	SelectTrue  string
	SelectFalse string
	// For getelementptr
	GEPBase     string   // base pointer (also: indirect call function pointer)
	GEPType     string   // struct type name
	GEPIndices  []string // index values
	// For indirect call type matching
	CallParamTypes []Type
	// For load/store
	LoadAddr    string
	StoreVal    string
	StoreAddr   string
}

type PhiPair struct {
	Value string
	Label string
}

// Global represents an LLVM global variable.
type Global struct {
	Name string
	Type Type
	Init string
}

// Parse parses LLVM IR text format into a Module.
func Parse(source string) (*Module, error) {
	m := &Module{
		Types: make(map[string]*StructType),
	}

	lines := strings.Split(source, "\n")
	i := 0

	for i < len(lines) {
		line := strings.TrimSpace(lines[i])

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "!") ||
			strings.HasPrefix(line, "source_filename") || strings.HasPrefix(line, "target") ||
			strings.HasPrefix(line, "attributes") {
			i++
			continue
		}

		// Named struct type: %struct.Point = type { i32, i32 }
		if strings.HasPrefix(line, "%") && strings.Contains(line, "= type {") {
			m.parseStructType(line)
			i++
			continue
		}

		// Function definition
		if strings.HasPrefix(line, "define ") {
			fn, end := m.parseFunction(lines, i)
			if fn != nil {
				m.Functions = append(m.Functions, fn)
			}
			i = end + 1
			continue
		}

		// Function declaration (external)
		if strings.HasPrefix(line, "declare ") {
			fn := m.parseDeclare(line)
			if fn != nil {
				m.Functions = append(m.Functions, fn)
			}
			i++
			continue
		}

		i++
	}

	return m, nil
}

func (m *Module) parseStructType(line string) {
	// %struct.Point = type { i32, i32 }
	re := regexp.MustCompile(`%(\S+)\s*=\s*type\s*\{([^}]*)\}`)
	match := re.FindStringSubmatch(line)
	if match == nil {
		return
	}
	name := match[1]
	fieldsStr := strings.TrimSpace(match[2])
	var fields []Type
	if fieldsStr != "" {
		for _, f := range strings.Split(fieldsStr, ",") {
			fields = append(fields, parseType(strings.TrimSpace(f)))
		}
	}
	m.Types[name] = &StructType{Name: name, Fields: fields}
}

var funcDefRe = regexp.MustCompile(`define\s+(?:\w+\s+)*(\S+)\s+@(\w+)\(([^)]*)\)`)

func (m *Module) parseFunction(lines []string, start int) (*Function, int) {
	line := lines[start]

	match := funcDefRe.FindStringSubmatch(line)
	if match == nil {
		return nil, start
	}

	retType := parseType(match[1])
	name := match[2]
	paramsStr := match[3]

	fn := &Function{
		Name:       name,
		ReturnType: retType,
	}

	// Parse parameters
	if paramsStr != "" {
		for idx, p := range strings.Split(paramsStr, ",") {
			p = strings.TrimSpace(p)
			parts := strings.Fields(p)
			if len(parts) >= 1 {
				typ := parseType(parts[0])
				pname := fmt.Sprintf("%%%d", idx)
				if len(parts) >= 2 {
					last := parts[len(parts)-1]
					if strings.HasPrefix(last, "%") {
						pname = last
					}
				}
				fn.Params = append(fn.Params, Param{Name: pname, Type: typ})
			}
		}
	}

	// Find opening { and parse basic blocks until closing }
	i := start
	// The { might be on the same line as define or the next line
	if !strings.HasSuffix(strings.TrimSpace(line), "{") {
		i++
	}

	var currentBlock *BasicBlock
	for i++; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		if line == "}" {
			if currentBlock != nil {
				fn.Blocks = append(fn.Blocks, currentBlock)
			}
			return fn, i
		}

		if line == "" || strings.HasPrefix(line, ";") || line == "]" {
			continue
		}

		// Join multi-line instructions (switch with [ ... ])
		if strings.Contains(line, "[") && !strings.Contains(line, "]") {
			for i+1 < len(lines) {
				i++
				next := strings.TrimSpace(lines[i])
				line += " " + next
				if strings.Contains(next, "]") {
					break
				}
			}
		}

		// Basic block label: "3:" or "entry:" or "3:    ; preds = ..."
		if !strings.HasPrefix(line, "%") && !strings.HasPrefix(line, "ret") &&
			!strings.HasPrefix(line, "br") && !strings.HasPrefix(line, "store") &&
			!strings.HasPrefix(line, "call") && !strings.HasPrefix(line, "tail") &&
			!strings.HasPrefix(line, "switch") &&
			strings.Contains(line, ":") {
			if currentBlock != nil {
				fn.Blocks = append(fn.Blocks, currentBlock)
			}
			label := strings.Split(line, ":")[0]
			label = strings.TrimSpace(label)
			currentBlock = &BasicBlock{Label: label}
			continue
		}

		// First instruction without a label → implicit entry block
		// LLVM names the entry block as %N where N = number of params
		if currentBlock == nil {
			entryLabel := fmt.Sprintf("%d", len(fn.Params))
			if entryLabel == "0" {
				entryLabel = "entry"
			}
			currentBlock = &BasicBlock{Label: entryLabel}
		}

		inst := parseInstruction(line)
		if inst != nil {
			currentBlock.Insts = append(currentBlock.Insts, inst)
		}
	}

	return fn, i
}

func (m *Module) parseDeclare(line string) *Function {
	re := regexp.MustCompile(`declare\s+(?:\w+\s+)*(\S+)\s+@(\w+)\(`)
	match := re.FindStringSubmatch(line)
	if match == nil {
		return nil
	}
	return &Function{
		Name:       match[2],
		ReturnType: parseType(match[1]),
		IsExternal: true,
	}
}

func parseType(s string) Type {
	s = strings.TrimSpace(s)
	// Remove qualifiers
	for _, q := range []string{"noundef", "nonnull", "returned", "dso_local", "nsw", "nuw", "local_unnamed_addr"} {
		s = strings.TrimSpace(strings.ReplaceAll(s, q, ""))
	}
	s = strings.TrimSpace(s)

	switch s {
	case "void":
		return Type{Kind: VoidType}
	case "i1":
		return Type{Kind: IntType, BitWidth: 1}
	case "i8":
		return Type{Kind: IntType, BitWidth: 8}
	case "i16":
		return Type{Kind: IntType, BitWidth: 16}
	case "i32":
		return Type{Kind: IntType, BitWidth: 32}
	case "i33":
		return Type{Kind: IntType, BitWidth: 33}
	case "i64":
		return Type{Kind: IntType, BitWidth: 64}
	case "float":
		return Type{Kind: FloatType}
	case "double":
		return Type{Kind: DoubleType}
	case "ptr":
		return Type{Kind: PtrType}
	}
	if strings.HasPrefix(s, "%struct.") || strings.HasPrefix(s, "%") {
		return Type{Kind: StructTypeKind, Name: strings.TrimPrefix(s, "%")}
	}
	return Type{Kind: IntType, BitWidth: 32} // fallback
}

func parseInstruction(line string) *Instruction {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, ";") {
		return nil
	}

	inst := &Instruction{}

	// ret instruction
	if strings.HasPrefix(line, "ret ") {
		inst.Op = "ret"
		rest := strings.TrimPrefix(line, "ret ")
		parts := strings.Fields(rest)
		if len(parts) >= 2 {
			inst.Type = parseType(parts[0])
			inst.Args = []string{parts[1]}
		} else if len(parts) == 1 && parts[0] == "void" {
			inst.Type = Type{Kind: VoidType}
		}
		return inst
	}

	// br instruction
	if strings.HasPrefix(line, "br ") {
		inst.Op = "br"
		rest := strings.TrimPrefix(line, "br ")
		if strings.HasPrefix(rest, "label %") {
			// Unconditional: br label %target
			inst.TrueLabel = extractLabel(rest)
		} else if strings.HasPrefix(rest, "i1 ") {
			// Conditional: br i1 %cond, label %true, label %false
			parts := strings.SplitN(rest, ",", 3)
			if len(parts) >= 3 {
				inst.Cond = strings.TrimSpace(strings.TrimPrefix(parts[0], "i1 "))
				inst.TrueLabel = extractLabel(parts[1])
				inst.FalseLabel = extractLabel(parts[2])
			}
		}
		return inst
	}

	// switch instruction (no result)
	if strings.HasPrefix(line, "switch ") {
		inst.Op = "switch"
		// switch i32 %val, label %default [ i32 0, label %bb0  i32 1, label %bb1 ... ]
		re := regexp.MustCompile(`switch\s+\w+\s+(%?\w+)\s*,\s*label\s+%(\w+)`)
		m := re.FindStringSubmatch(line)
		if m != nil {
			inst.Cond = m[1]
			inst.FalseLabel = m[2] // default label
		}
		// Parse cases: i32 N, label %bbX
		caseRe := regexp.MustCompile(`i\d+\s+(-?\d+)\s*,\s*label\s+%(\w+)`)
		cases := caseRe.FindAllStringSubmatch(line, -1)
		for _, c := range cases {
			inst.Args = append(inst.Args, c[1])       // value
			inst.PhiPairs = append(inst.PhiPairs, PhiPair{Value: c[1], Label: c[2]}) // reuse PhiPair for case→label
		}
		return inst
	}

	// store instruction (no result)
	if strings.HasPrefix(line, "store ") {
		inst.Op = "store"
		// store i32 %1, ptr %0, align 4
		parts := strings.SplitN(line, ",", 3)
		if len(parts) >= 2 {
			valParts := strings.Fields(strings.TrimPrefix(parts[0], "store "))
			if len(valParts) >= 2 {
				inst.Type = parseType(valParts[0])
				inst.StoreVal = valParts[len(valParts)-1]
			}
			addrParts := strings.Fields(strings.TrimSpace(parts[1]))
			if len(addrParts) >= 2 {
				inst.StoreAddr = addrParts[len(addrParts)-1]
			}
		}
		return inst
	}

	// Assignment: %result = op ...
	if strings.HasPrefix(line, "%") && strings.Contains(line, " = ") {
		eqIdx := strings.Index(line, " = ")
		inst.Result = line[:eqIdx]
		rest := line[eqIdx+3:]

		// Strip optional keywords
		rest = stripKeywords(rest, []string{"nsw", "nuw", "tail", "nnan", "ninf"})

		// phi instruction
		if strings.HasPrefix(rest, "phi ") {
			inst.Op = "phi"
			re := regexp.MustCompile(`\[\s*(%?\w+)\s*,\s*%(\w+)\s*\]`)
			matches := re.FindAllStringSubmatch(rest, -1)
			for _, m := range matches {
				inst.PhiPairs = append(inst.PhiPairs, PhiPair{Value: m[1], Label: m[2]})
			}
			// Parse type
			parts := strings.Fields(rest)
			if len(parts) >= 2 {
				inst.Type = parseType(parts[1])
			}
			return inst
		}

		// icmp instruction
		if strings.HasPrefix(rest, "icmp ") {
			inst.Op = "icmp"
			parts := strings.Fields(rest)
			// icmp pred type %a, %b
			if len(parts) >= 5 {
				inst.Predicate = parts[1]
				inst.Type = parseType(parts[2])
				inst.Args = []string{
					strings.TrimSuffix(parts[3], ","),
					parts[4],
				}
			}
			return inst
		}

		// select instruction
		if strings.HasPrefix(rest, "select ") {
			inst.Op = "select"
			// select i1 %cond, TYPE %true_val, TYPE %false_val
			// Also handles: select i1 %cond, i1 true, i1 %val
			re := regexp.MustCompile(`select\s+i1\s+(%\w+)\s*,\s*(\w+)\s+(%?\w+)\s*,\s*(\w+)\s+(%?\w+)`)
			m := re.FindStringSubmatch(rest)
			if m != nil {
				inst.SelectCond = m[1]
				inst.Type = parseType(m[2])
				inst.SelectTrue = m[3]
				inst.SelectFalse = m[5]
			}
			return inst
		}

		// getelementptr instruction
		if strings.HasPrefix(rest, "getelementptr ") {
			inst.Op = "getelementptr"
			inst.Type = Type{Kind: PtrType}
			// getelementptr inbounds %struct.Point, ptr %0, i64 0, i32 1
			// getelementptr inbounds i32, ptr %0, i64 %9
			cleaned := strings.Replace(rest, "inbounds ", "", 1)
			cleaned = strings.Replace(cleaned, "nuw ", "", 1)
			parts := strings.SplitN(cleaned, ",", -1)
			if len(parts) >= 2 {
				// First part: "getelementptr TYPE"
				typePart := strings.TrimPrefix(parts[0], "getelementptr ")
				typePart = strings.TrimSpace(typePart)
				inst.GEPType = typePart
				// Second part: "ptr %base"
				basePart := strings.TrimSpace(parts[1])
				baseFields := strings.Fields(basePart)
				if len(baseFields) >= 2 {
					inst.GEPBase = baseFields[len(baseFields)-1]
				}
				// Remaining parts: indices
				for _, p := range parts[2:] {
					f := strings.Fields(strings.TrimSpace(p))
					if len(f) >= 2 {
						inst.GEPIndices = append(inst.GEPIndices, f[len(f)-1])
					}
				}
			}
			return inst
		}

		// load instruction
		if strings.HasPrefix(rest, "load ") {
			inst.Op = "load"
			// load i32, ptr %0, align 4
			parts := strings.SplitN(rest, ",", 3)
			if len(parts) >= 2 {
				typePart := strings.TrimPrefix(parts[0], "load ")
				inst.Type = parseType(strings.TrimSpace(typePart))
				addrParts := strings.Fields(strings.TrimSpace(parts[1]))
				if len(addrParts) >= 2 {
					inst.LoadAddr = addrParts[len(addrParts)-1]
				}
			}
			return inst
		}

		// call instruction (including tail call, indirect call)
		if strings.Contains(rest, "call ") {
			inst.Op = "call"
			// Try direct call: call TYPE @name(args)
			re := regexp.MustCompile(`call\s+\S+\s+@([\w.]+)\(([^)]*)\)`)
			m := re.FindStringSubmatch(rest)
			if m != nil {
				inst.CallTarget = m[1]
				inst.Type = parseType(strings.Fields(rest)[1])
				if m[2] != "" {
					for _, arg := range strings.Split(m[2], ",") {
						parts := strings.Fields(strings.TrimSpace(arg))
						if len(parts) >= 2 {
							inst.Args = append(inst.Args, parts[len(parts)-1])
						}
					}
				}
				return inst
			}
			// Try indirect call: call TYPE %ptr(args)
			re2 := regexp.MustCompile(`call\s+(\S+)\s+(%\w+)\(([^)]*)\)`)
			m2 := re2.FindStringSubmatch(rest)
			if m2 != nil {
				inst.CallTarget = "__indirect"
				inst.Type = parseType(m2[1])
				inst.GEPBase = m2[2]
				if m2[3] != "" {
					for _, arg := range strings.Split(m2[3], ",") {
						parts := strings.Fields(strings.TrimSpace(arg))
						if len(parts) >= 2 {
							// First word (stripped of attrs) is the type
							ptype := parts[0]
							inst.CallParamTypes = append(inst.CallParamTypes, parseType(ptype))
							inst.Args = append(inst.Args, parts[len(parts)-1])
						}
					}
				}
				return inst
			}
			// Fallback: mark as unknown call
			inst.CallTarget = "__unknown"
			inst.Type = Type{Kind: IntType, BitWidth: 32}
			return inst
		}

		// alloca instruction — result is a pointer (TYPE i)
		if strings.HasPrefix(rest, "alloca ") {
			inst.Op = "alloca"
			inst.Type = Type{Kind: PtrType} // alloca returns ptr
			return inst
		}

		// freeze instruction (LLVM poison/undef → just pass through)
		if strings.HasPrefix(rest, "freeze ") {
			inst.Op = "freeze"
			parts := strings.Fields(rest)
			if len(parts) >= 3 {
				inst.Type = parseType(parts[1])
				inst.Args = []string{parts[2]}
			}
			return inst
		}

		// extractvalue: %x = extractvalue {i32, i32} %val, 0
		if strings.HasPrefix(rest, "extractvalue ") || strings.HasPrefix(rest, "insertvalue ") {
			inst.Op = strings.Fields(rest)[0]
			// Find the last operand before any index
			re := regexp.MustCompile(`(%\w+)\s*,\s*\d+`)
			m := re.FindStringSubmatch(rest)
			if m != nil {
				inst.Args = []string{m[1]}
			}
			inst.Type = Type{Kind: IntType, BitWidth: 32} // default
			return inst
		}

		// fcmp instruction
		if strings.HasPrefix(rest, "fcmp ") {
			inst.Op = "fcmp"
			parts := strings.Fields(rest)
			if len(parts) >= 5 {
				inst.Predicate = parts[1]
				inst.Type = Type{Kind: IntType, BitWidth: 1}
				inst.Args = []string{
					strings.TrimSuffix(parts[3], ","),
					parts[4],
				}
			}
			return inst
		}

		// fneg instruction
		if strings.HasPrefix(rest, "fneg ") {
			inst.Op = "fneg"
			parts := strings.Fields(rest)
			if len(parts) >= 3 {
				inst.Type = parseType(parts[1])
				inst.Args = []string{parts[2]}
			}
			return inst
		}

		// Conversion ops: zext, sext, trunc, bitcast, ptrtoint, inttoptr, sitofp, etc.
		convOps := []string{"zext ", "sext ", "trunc ", "bitcast ", "ptrtoint ", "inttoptr ",
			"sitofp ", "uitofp ", "fptosi ", "fptoui ", "fptrunc ", "fpext "}
		isConv := false
		for _, op := range convOps {
			if strings.HasPrefix(rest, op) {
				isConv = true
				break
			}
		}
		if isConv {
			// Strip extra keywords like nneg
			rest = strings.ReplaceAll(rest, " nneg ", " ")
			fields := strings.Fields(rest)
			inst.Op = fields[0]
			// zext i32 %5 to i64 → just pass through value
			if len(fields) >= 3 {
				inst.Args = []string{fields[2]}
			}
			// Get target type (after "to")
			for j, f := range fields {
				if f == "to" && j+1 < len(fields) {
					inst.Type = parseType(fields[j+1])
					break
				}
			}
			return inst
		}

		// Binary ops: add, sub, mul, sdiv, srem, and, or, xor, shl, ashr, lshr
		// Unary: sub 0, %x (negate)
		// Float: fadd, fsub, fmul, fdiv
		parts := strings.Fields(rest)
		if len(parts) >= 4 {
			inst.Op = parts[0]
			inst.Type = parseType(parts[1])
			inst.Args = []string{
				strings.TrimSuffix(parts[2], ","),
				strings.TrimSuffix(parts[3], ","),
			}
			return inst
		}
		if len(parts) >= 3 {
			inst.Op = parts[0]
			inst.Type = parseType(parts[1])
			inst.Args = []string{strings.TrimSuffix(parts[2], ",")}
			return inst
		}
	}

	return nil
}

func extractLabel(s string) string {
	s = strings.TrimSpace(s)
	re := regexp.MustCompile(`label\s+%(\w+)`)
	m := re.FindStringSubmatch(s)
	if m != nil {
		return m[1]
	}
	return ""
}

func stripKeywords(s string, kws []string) string {
	for _, kw := range kws {
		s = strings.ReplaceAll(s, " "+kw+" ", " ")
		s = strings.ReplaceAll(s, " "+kw+",", ",")
		// Also strip at beginning of string
		if strings.HasPrefix(s, kw+" ") {
			s = s[len(kw)+1:]
		}
	}
	return strings.TrimSpace(s)
}

// Compile converts a parsed LLVM IR Module to a single ABAP source.
func Compile(mod *Module, className string) string {
	c := &abapCompiler{mod: mod, className: className}
	return c.emit()
}

// MultiClassFile represents one file in a multi-class compilation.
type MultiClassFile struct {
	FileName  string // e.g., "zcl_qjs_p01"
	ClassName string // e.g., "ZCL_QJS_P01"
	Source    string // ABAP source
	IsClass  bool   // true=.clas.abap, false=.prog.abap
}

// CompileMultiClass splits a large module into multiple classes,
// each under maxFuncsPerClass functions. Returns a list of files.
func CompileMultiClass(mod *Module, baseName string, maxFuncsPerClass int) []MultiClassFile {
	if maxFuncsPerClass <= 0 {
		maxFuncsPerClass = 30
	}

	// Collect non-external functions
	var funcs []*Function
	for _, fn := range mod.Functions {
		if !fn.IsExternal && !strings.HasPrefix(fn.Name, "llvm.") {
			funcs = append(funcs, fn)
		}
	}

	// Build function→class mapping
	numParts := (len(funcs) + maxFuncsPerClass - 1) / maxFuncsPerClass
	funcToClass := make(map[string]string) // function name → class name

	for p := 0; p < numParts; p++ {
		start := p * maxFuncsPerClass
		end := start + maxFuncsPerClass
		if end > len(funcs) {
			end = len(funcs)
		}
		partName := fmt.Sprintf("%s_p%02d", baseName, p+1)
		for _, fn := range funcs[start:end] {
			funcToClass[fn.Name] = partName
		}
	}

	var files []MultiClassFile

	// Generate each class part
	for p := 0; p < numParts; p++ {
		start := p * maxFuncsPerClass
		end := start + maxFuncsPerClass
		if end > len(funcs) {
			end = len(funcs)
		}

		partName := fmt.Sprintf("%s_p%02d", baseName, p+1)
		partFuncs := funcs[start:end]

		// Create a sub-module with only these functions
		subMod := &Module{
			Types:     mod.Types,
			Functions: mod.Functions, // keep all for dispatch/reference
			Globals:   mod.Globals,
		}

		c := &abapCompiler{
			mod:         subMod,
			className:   partName,
			funcToClass: funcToClass,
			partFuncs:   make(map[string]bool),
		}
		for _, fn := range partFuncs {
			c.partFuncs[fn.Name] = true
		}

		source := c.emitClassFile()
		files = append(files, MultiClassFile{
			FileName:  partName,
			ClassName: strings.ToUpper(partName),
			Source:    source,
			IsClass:  true,
		})
	}

	// Auto-detect external/missing functions and stub them in _mem class
	// "defined" = has blocks (real implementation). External = declared only.
	definedFuncs := make(map[string]bool)
	for _, fn := range mod.Functions {
		if !fn.IsExternal {
			definedFuncs[fn.Name] = true
		}
	}
	// Hardcoded stubs already in mem class
	hardcoded := map[string]bool{
		"strlen": true, "malloc": true, "realloc": true, "free": true,
		"malloc_usable_size": true, "memset": true, "memcpy": true, "memmove": true,
		"snprintf": true, "printf": true, "fprintf": true, "fflush": true,
		"abort": true, "exit": true, "gettimeofday": true, "strchr": true,
		"strncmp": true, "strcmp": true, "memcmp": true, "strtod": true,
		"strtol": true, "atoi": true, "qsort": true,
	}
	var missingStubDecls, missingStubImpls []string
	seenMissing := make(map[string]bool)
	for _, fn := range mod.Functions {
		for _, b := range fn.Blocks {
			for _, inst := range b.Insts {
				if inst.Op == "call" && inst.CallTarget != "" &&
					inst.CallTarget != "__indirect" && inst.CallTarget != "__unknown" &&
					!strings.HasPrefix(inst.CallTarget, "llvm.") &&
					!definedFuncs[inst.CallTarget] && !hardcoded[inst.CallTarget] &&
					!seenMissing[inst.CallTarget] {
					seenMissing[inst.CallTarget] = true
					name := sanitizeName(inst.CallTarget)
					if name == "abort" { name = "c_abort" }
					if name == "exit" { name = "c_exit" }
					nArgs := len(inst.Args)
					hasRet := inst.Type.Kind != VoidType
					var params []string
					for i := 0; i < nArgs; i++ {
						params = append(params, fmt.Sprintf("%s TYPE i", paramName(i, inst.CallTarget)))
					}
					sig := "CLASS-METHODS " + name
					if len(params) > 0 {
						sig += " IMPORTING " + strings.Join(params, " ")
					}
					if hasRet {
						sig += " RETURNING VALUE(rv) TYPE i"
					}
					missingStubDecls = append(missingStubDecls, "    "+sig+".")
					if hasRet {
						missingStubImpls = append(missingStubImpls, fmt.Sprintf("  METHOD %s. rv = 0. ENDMETHOD.", name))
					} else {
						missingStubImpls = append(missingStubImpls, fmt.Sprintf("  METHOD %s. ENDMETHOD.", name))
					}
				}
			}
		}
	}

	// Add memory runtime class
	memClass := fmt.Sprintf(`CLASS %s_mem DEFINITION PUBLIC.
  PUBLIC SECTION.
    TYPES x4 TYPE x LENGTH 4.
    CLASS-DATA gv_mem TYPE xstring.
    CLASS-METHODS mem_ld_i32 IMPORTING iv_addr TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS mem_st_i32 IMPORTING iv_addr TYPE i iv_val TYPE i.
    CLASS-METHODS mem_ld_i32_8u IMPORTING iv_addr TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS mem_st_i32_8 IMPORTING iv_addr TYPE i iv_val TYPE i.
    " C library stubs
    CLASS-METHODS strlen IMPORTING a TYPE i RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS malloc IMPORTING a TYPE int8 RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS realloc IMPORTING a TYPE i b TYPE int8 RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS free IMPORTING a TYPE i.
    CLASS-METHODS malloc_usable_size IMPORTING a TYPE i RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS memset IMPORTING a TYPE i b TYPE i c TYPE int8.
    CLASS-METHODS memcpy IMPORTING a TYPE i b TYPE i c TYPE int8.
    CLASS-METHODS memmove IMPORTING a TYPE i b TYPE i c TYPE int8.
    CLASS-METHODS snprintf IMPORTING a TYPE i b TYPE int8 c TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS printf IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS fprintf IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS fflush IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS c_abort.
    CLASS-METHODS c_exit IMPORTING a TYPE i.
    CLASS-METHODS gettimeofday IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS strchr IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS strncmp IMPORTING a TYPE i b TYPE i c TYPE int8 RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS strcmp IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS memcmp IMPORTING a TYPE i b TYPE i c TYPE int8 RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS strtod IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE f.
    CLASS-METHODS strtol IMPORTING a TYPE i b TYPE i c TYPE i RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS atoi IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS qsort IMPORTING a TYPE i b TYPE int8 c TYPE int8 d TYPE i.
%s
ENDCLASS.
CLASS %s_mem IMPLEMENTATION.
  METHOD mem_ld_i32.
    DATA lv_b TYPE x LENGTH 4. DATA lv_r TYPE xstring.
    IF iv_addr >= 0 AND iv_addr + 4 <= xstrlen( gv_mem ).
      lv_b = gv_mem+iv_addr(4).
      CONCATENATE lv_b+3(1) lv_b+2(1) lv_b+1(1) lv_b+0(1) INTO lv_r IN BYTE MODE. rv = lv_r.
    ENDIF.
  ENDMETHOD.
  METHOD mem_st_i32.
    DATA lv_x TYPE x LENGTH 4. DATA lv_r TYPE xstring. lv_x = iv_val.
    IF iv_addr >= 0 AND iv_addr + 4 <= xstrlen( gv_mem ).
      CONCATENATE lv_x+3(1) lv_x+2(1) lv_x+1(1) lv_x+0(1) INTO lv_r IN BYTE MODE.
      REPLACE SECTION OFFSET iv_addr LENGTH 4 OF gv_mem WITH lv_r IN BYTE MODE.
    ENDIF.
  ENDMETHOD.
  METHOD mem_ld_i32_8u.
    DATA lv_b TYPE x LENGTH 1.
    IF iv_addr >= 0 AND iv_addr + 1 <= xstrlen( gv_mem ).
      lv_b = gv_mem+iv_addr(1). rv = lv_b.
    ENDIF.
  ENDMETHOD.
  METHOD mem_st_i32_8.
    DATA lv_x TYPE x LENGTH 1. DATA lv_bx TYPE xstring. lv_x = iv_val.
    IF iv_addr >= 0 AND iv_addr + 1 <= xstrlen( gv_mem ).
      lv_bx = lv_x.
      REPLACE SECTION OFFSET iv_addr LENGTH 1 OF gv_mem WITH lv_bx IN BYTE MODE.
    ENDIF.
  ENDMETHOD.
  METHOD strlen. rv = 0. ENDMETHOD.
  METHOD malloc. rv = 0. ENDMETHOD.
  METHOD realloc. rv = 0. ENDMETHOD.
  METHOD free. ENDMETHOD.
  METHOD malloc_usable_size. rv = 0. ENDMETHOD.
  METHOD memset. ENDMETHOD.
  METHOD memcpy. ENDMETHOD.
  METHOD memmove. ENDMETHOD.
  METHOD snprintf. rv = 0. ENDMETHOD.
  METHOD printf. rv = 0. ENDMETHOD.
  METHOD fprintf. rv = 0. ENDMETHOD.
  METHOD fflush. rv = 0. ENDMETHOD.
  METHOD c_abort. ENDMETHOD.
  METHOD c_exit. ENDMETHOD.
  METHOD gettimeofday. rv = 0. ENDMETHOD.
  METHOD strchr. rv = 0. ENDMETHOD.
  METHOD strncmp. rv = 0. ENDMETHOD.
  METHOD strcmp. rv = 0. ENDMETHOD.
  METHOD memcmp. rv = 0. ENDMETHOD.
  METHOD strtod. rv = 0. ENDMETHOD.
  METHOD strtol. rv = 0. ENDMETHOD.
  METHOD atoi. rv = 0. ENDMETHOD.
  METHOD qsort. ENDMETHOD.
%s
ENDCLASS.
`, baseName, strings.Join(missingStubDecls, "\n"), baseName, strings.Join(missingStubImpls, "\n"))

	files = append(files, MultiClassFile{
		FileName:  baseName + "_mem",
		ClassName: strings.ToUpper(baseName + "_MEM"),
		Source:    memClass,
		IsClass:  true,
	})

	return files
}

type abapCompiler struct {
	mod       *Module
	className string
	sb        strings.Builder
	indent    int
	// Track which SSA values are function params (for naming)
	paramMap map[string]string // %0 → "a", %1 → "b"
	// Phi resolution: phiMap[targetBlock][sourceBlock] = [{dst, val}, ...]
	phiMap map[string]map[string][]phiAssign
	// Current block label (for phi resolution at branches)
	currentBlock string
	// Multi-class support
	funcToClass map[string]string // function name → class name (for cross-class calls)
	partFuncs   map[string]bool   // functions to emit in THIS class (nil = emit all)
}

type phiAssign struct {
	dst string // lv_4
	val string // lv_8 or "2"
}

func (c *abapCompiler) memPrefix() string {
	if c.funcToClass != nil {
		// Multi-class mode — use shared memory class
		base := c.className
		// Strip _pNN suffix to get base name
		if idx := strings.LastIndex(base, "_p"); idx > 0 {
			base = base[:idx]
		}
		return base + "_mem=>"
	}
	return "" // single-class mode — use PERFORM (handled in emit())
}

func (c *abapCompiler) shouldEmitFunc(fn *Function) bool {
	if c.partFuncs == nil {
		return true // emit all
	}
	return c.partFuncs[fn.Name]
}

func (c *abapCompiler) emitClassFile() string {
	c.line("CLASS %s DEFINITION PUBLIC.", c.className)
	c.indent++
	c.line("PUBLIC SECTION.")
	c.indent++
	c.line("TYPES x4 TYPE x LENGTH 4.")

	for _, fn := range c.mod.Functions {
		if fn.IsExternal || strings.HasPrefix(fn.Name, "llvm.") {
			continue
		}
		if !c.shouldEmitFunc(fn) {
			continue
		}
		c.emitMethodDecl(fn)
	}

	c.indent--
	c.indent--
	c.line("ENDCLASS.")
	c.line("")

	c.line("CLASS %s IMPLEMENTATION.", c.className)
	c.indent++

	for _, fn := range c.mod.Functions {
		if fn.IsExternal || strings.HasPrefix(fn.Name, "llvm.") {
			continue
		}
		if !c.shouldEmitFunc(fn) {
			continue
		}
		c.emitMethod(fn)
	}

	c.indent--
	c.line("ENDCLASS.")

	return c.sb.String()
}

func (c *abapCompiler) emit() string {
	c.line("REPORT %s.", c.className)
	c.line("")
	c.line("TYPES x4 TYPE x LENGTH 4.")
	c.line("DATA gv_mem TYPE xstring.")
	c.line("")
	c.line("CLASS %s DEFINITION FINAL CREATE PUBLIC.", c.className)
	c.indent++
	c.line("PUBLIC SECTION.")
	c.indent++

	// Emit typed method signatures for each non-external function
	for _, fn := range c.mod.Functions {
		if fn.IsExternal || strings.HasPrefix(fn.Name, "llvm.") {
			continue
		}
		c.emitMethodDecl(fn)
	}

	c.indent--
	c.indent--
	c.line("ENDCLASS.")
	c.line("")

	c.line("CLASS %s IMPLEMENTATION.", c.className)
	c.indent++

	for _, fn := range c.mod.Functions {
		if fn.IsExternal || strings.HasPrefix(fn.Name, "llvm.") {
			continue
		}
		c.emitMethod(fn)
	}

	c.indent--
	c.line("ENDCLASS.")
	c.line("")

	// Memory runtime + C library stubs (after class, as FORMs)
	c.line("\" Memory runtime")
	c.line("FORM mem_ld_i32 USING iv_addr TYPE i CHANGING rv TYPE i.")
	c.line("  DATA lv_b TYPE x LENGTH 4. DATA lv_r TYPE xstring.")
	c.line("  IF iv_addr >= 0 AND iv_addr + 4 <= xstrlen( gv_mem ).")
	c.line("    lv_b = gv_mem+iv_addr(4).")
	c.line("    CONCATENATE lv_b+3(1) lv_b+2(1) lv_b+1(1) lv_b+0(1) INTO lv_r IN BYTE MODE. rv = lv_r.")
	c.line("  ELSE. rv = 0. ENDIF.")
	c.line("ENDFORM.")
	c.line("FORM mem_st_i32 USING iv_addr TYPE i iv_val TYPE i.")
	c.line("  DATA lv_x TYPE x LENGTH 4. DATA lv_r TYPE xstring. lv_x = iv_val.")
	c.line("  IF iv_addr >= 0 AND iv_addr + 4 <= xstrlen( gv_mem ).")
	c.line("    CONCATENATE lv_x+3(1) lv_x+2(1) lv_x+1(1) lv_x+0(1) INTO lv_r IN BYTE MODE.")
	c.line("    REPLACE SECTION OFFSET iv_addr LENGTH 4 OF gv_mem WITH lv_r IN BYTE MODE.")
	c.line("  ENDIF.")
	c.line("ENDFORM.")
	c.line("FORM mem_ld_i32_8u USING iv_addr TYPE i CHANGING rv TYPE i.")
	c.line("  DATA lv_b TYPE x LENGTH 1.")
	c.line("  IF iv_addr >= 0 AND iv_addr + 1 <= xstrlen( gv_mem ).")
	c.line("    lv_b = gv_mem+iv_addr(1). rv = lv_b.")
	c.line("  ELSE. rv = 0. ENDIF.")
	c.line("ENDFORM.")
	c.line("FORM mem_st_i32_8 USING iv_addr TYPE i iv_val TYPE i.")
	c.line("  DATA lv_x TYPE x LENGTH 1. DATA lv_bx TYPE xstring. lv_x = iv_val.")
	c.line("  IF iv_addr >= 0 AND iv_addr + 1 <= xstrlen( gv_mem ).")
	c.line("    lv_bx = lv_x.")
	c.line("    REPLACE SECTION OFFSET iv_addr LENGTH 1 OF gv_mem WITH lv_bx IN BYTE MODE.")
	c.line("  ENDIF.")
	c.line("ENDFORM.")
	c.line("FORM mem_st_i32_16 USING iv_addr TYPE i iv_val TYPE i.")
	c.line("  DATA lv_x TYPE x LENGTH 2. DATA lv_r16 TYPE xstring. lv_x = iv_val.")
	c.line("  IF iv_addr >= 0 AND iv_addr + 2 <= xstrlen( gv_mem ).")
	c.line("    CONCATENATE lv_x+1(1) lv_x+0(1) INTO lv_r16 IN BYTE MODE.")
	c.line("    REPLACE SECTION OFFSET iv_addr LENGTH 2 OF gv_mem WITH lv_r16 IN BYTE MODE.")
	c.line("  ENDIF.")
	c.line("ENDFORM.")
	c.line("\" C library stubs")
	c.line("FORM strlen USING iv_ptr TYPE i CHANGING rv TYPE int8. rv = 0. ENDFORM.")
	c.line("FORM memset USING iv_dst TYPE i iv_val TYPE i iv_n TYPE int8. ENDFORM.")
	c.line("FORM memcpy USING iv_dst TYPE i iv_src TYPE i iv_n TYPE int8. ENDFORM.")
	c.line("FORM snprintf USING p0 TYPE i p1 TYPE int8 p2 TYPE i CHANGING rv TYPE i. rv = 0. ENDFORM.")
	c.line("FORM printf USING p0 TYPE i CHANGING rv TYPE i. rv = 0. ENDFORM.")
	c.line("FORM abort. ENDFORM.")

	return c.sb.String()
}

func (c *abapCompiler) emitMethodDecl(fn *Function) {
	parts := []string{"CLASS-METHODS " + sanitizeName(fn.Name)}

	if len(fn.Params) > 0 {
		var params []string
		for i, p := range fn.Params {
			name := paramName(i, fn.Name)
			typ := p.Type.ABAPType()
			if typ == "" {
				typ = "i"
			}
			params = append(params, fmt.Sprintf("%s TYPE %s", name, typ))
		}
		parts = append(parts, "IMPORTING "+strings.Join(params, " "))
	}

	if fn.ReturnType.Kind != VoidType {
		retTyp := fn.ReturnType.ABAPType()
		if retTyp == "" {
			retTyp = "i"
		}
		parts = append(parts, fmt.Sprintf("RETURNING VALUE(rv) TYPE %s", retTyp))
	}

	c.line("%s.", strings.Join(parts, " "))
}

func (c *abapCompiler) emitMethod(fn *Function) {
	c.line("METHOD %s.", sanitizeName(fn.Name))
	c.indent++

	// Build param map
	c.paramMap = make(map[string]string)
	for i, p := range fn.Params {
		c.paramMap[p.Name] = paramName(i, fn.Name)
	}

	// Collect all SSA values used (for DATA declarations)
	vars := c.collectVars(fn)
	if len(vars) > 0 {
		var decls []string
		for _, v := range vars {
			decls = append(decls, fmt.Sprintf("%s TYPE %s", v.name, v.typ))
		}
		// Emit DATA declarations — max 5 per line for transpiler compatibility
		for i := 0; i < len(decls); i += 5 {
			end := i + 5
			if end > len(decls) {
				end = len(decls)
			}
			chunk := decls[i:end]
			c.line("DATA: %s.", strings.Join(chunk, ", "))
		}
	}

	// Simple functions (single block, no phi, no complex CFG)
	if len(fn.Blocks) == 1 {
		c.emitBlock(fn.Blocks[0], fn)
	} else {
		// Multi-block: use dispatcher
		c.emitWithDispatcher(fn)
	}

	c.indent--
	c.line("ENDMETHOD.")
	c.line("")
}

type varDecl struct {
	name string
	typ  string
}

func (c *abapCompiler) collectVars(fn *Function) []varDecl {
	seen := make(map[string]bool)
	var result []varDecl

	// Count max parallel phi assignments for temp vars
	maxPhi := 0
	for _, b := range fn.Blocks {
		phiCount := 0
		for _, inst := range b.Insts {
			if inst.Op == "phi" {
				phiCount++
			}
			if inst.Result != "" {
				name := c.ssaName(inst.Result)
				if !seen[name] && !c.isParam(inst.Result) {
					seen[name] = true
					typ := inst.Type.ABAPType()
					if typ == "" {
						typ = "i" // default for unrecognized types
					}
					result = append(result, varDecl{name: name, typ: typ})
				}
			}
		}
		if phiCount > maxPhi {
			maxPhi = phiCount
		}
	}

	// Add phi temp vars if needed
	for i := 0; i < maxPhi; i++ {
		result = append(result, varDecl{name: fmt.Sprintf("lv_phi%d", i), typ: "i"})
	}

	// Add bitwise temp vars if any and/or/xor ops present
	hasBitwise := false
	for _, b := range fn.Blocks {
		for _, inst := range b.Insts {
			if inst.Op == "and" || inst.Op == "or" || inst.Op == "xor" {
				hasBitwise = true
				break
			}
		}
		if hasBitwise { break }
	}
	if hasBitwise {
		result = append(result, varDecl{name: "lv_xa", typ: "x4"})
		result = append(result, varDecl{name: "lv_xb", typ: "x4"})
	}

	return result
}

func (c *abapCompiler) emitBlock(block *BasicBlock, fn *Function) {
	for _, inst := range block.Insts {
		c.emitInst(inst, fn)
	}
}

func (c *abapCompiler) buildPhiMap(fn *Function) {
	c.phiMap = make(map[string]map[string][]phiAssign)
	for _, block := range fn.Blocks {
		for _, inst := range block.Insts {
			if inst.Op != "phi" {
				continue
			}
			dst := c.ssaName(inst.Result)
			for _, pair := range inst.PhiPairs {
				target := block.Label
				source := pair.Label
				if c.phiMap[target] == nil {
					c.phiMap[target] = make(map[string][]phiAssign)
				}
				c.phiMap[target][source] = append(c.phiMap[target][source], phiAssign{
					dst: dst,
					val: c.val(pair.Value),
				})
			}
		}
	}
}

func (c *abapCompiler) emitPhiAssigns(targetBlock string) {
	if c.phiMap == nil {
		return
	}
	assigns, ok := c.phiMap[targetBlock][c.currentBlock]
	if !ok {
		return
	}
	// Emit phi assignments before the block transition
	// For parallel phi (multiple assignments), use temp-then-assign pattern
	// to avoid read-after-write conflicts (e.g., swap a,b)
	if len(assigns) > 1 {
		// Check if any value references another phi destination (conflict)
		dstSet := make(map[string]bool)
		for _, a := range assigns {
			dstSet[a.dst] = true
		}
		hasConflict := false
		for _, a := range assigns {
			if dstSet[a.val] {
				hasConflict = true
				break
			}
		}
		if hasConflict {
			// Save all values first, then assign
			for i, a := range assigns {
				c.line("lv_phi%d = %s.", i, a.val)
			}
			for i, a := range assigns {
				c.line("%s = lv_phi%d.", a.dst, i)
			}
		} else {
			// No conflict — direct assignment
			for _, a := range assigns {
				c.line("%s = %s.", a.dst, a.val)
			}
		}
	} else if len(assigns) == 1 {
		c.line("%s = %s.", assigns[0].dst, assigns[0].val)
	}
}

const maxBlocksPerCase = 12

func (c *abapCompiler) emitWithDispatcher(fn *Function) {
	c.buildPhiMap(fn)

	firstLabel := "entry"
	if len(fn.Blocks) > 0 {
		firstLabel = fn.Blocks[0].Label
	}

	c.line("DATA lv_block TYPE string VALUE '%s'.", firstLabel)
	c.line("DO.")
	c.indent++

	if len(fn.Blocks) <= maxBlocksPerCase {
		// Small enough — single CASE
		c.line("CASE lv_block.")
		c.indent++
		for _, block := range fn.Blocks {
			c.line("WHEN '%s'.", block.Label)
			c.indent++
			c.currentBlock = block.Label
			c.emitBlock(block, fn)
			c.indent--
		}
		c.indent--
		c.line("ENDCASE.")
	} else {
		// Large method — split into chained IF/ELSEIF blocks
		// Each chunk handles maxBlocksPerCase WHEN clauses
		for i := 0; i < len(fn.Blocks); i += maxBlocksPerCase {
			end := i + maxBlocksPerCase
			if end > len(fn.Blocks) {
				end = len(fn.Blocks)
			}
			chunk := fn.Blocks[i:end]

			// Build condition: IF lv_block = 'x' OR lv_block = 'y' OR ...
			var conds []string
			for _, b := range chunk {
				conds = append(conds, fmt.Sprintf("lv_block = '%s'", b.Label))
			}
			if i == 0 {
				c.line("IF %s.", strings.Join(conds, " OR "))
			} else {
				c.line("ELSEIF %s.", strings.Join(conds, " OR "))
			}
			c.indent++
			c.line("CASE lv_block.")
			c.indent++
			for _, block := range chunk {
				c.line("WHEN '%s'.", block.Label)
				c.indent++
				c.currentBlock = block.Label
				c.emitBlock(block, fn)
				c.indent--
			}
			c.indent--
			c.line("ENDCASE.")
			c.indent--
		}
		c.line("ENDIF.")
	}

	c.indent--
	c.line("ENDDO.")
}

func (c *abapCompiler) emitInst(inst *Instruction, fn *Function) {
	dst := c.ssaName(inst.Result)

	switch inst.Op {
	// Arithmetic
	case "add", "sub", "mul", "fadd", "fsub", "fmul":
		op := map[string]string{"add": "+", "sub": "-", "mul": "*", "fadd": "+", "fsub": "-", "fmul": "*"}[inst.Op]
		c.line("%s = %s %s %s.", dst, c.val(inst.Args[0]), op, c.val(inst.Args[1]))

	case "sdiv", "udiv", "fdiv":
		c.line("%s = %s / %s.", dst, c.val(inst.Args[0]), c.val(inst.Args[1]))

	case "srem", "urem":
		c.line("%s = %s MOD %s.", dst, c.val(inst.Args[0]), c.val(inst.Args[1]))

	// Bitwise
	case "and":
		a, b := c.val(inst.Args[0]), c.val(inst.Args[1])
		c.line("lv_xa = %s. lv_xb = %s. lv_xa = lv_xa BIT-AND lv_xb. %s = lv_xa.", a, b, dst)
	case "or":
		a, b := c.val(inst.Args[0]), c.val(inst.Args[1])
		c.line("lv_xa = %s. lv_xb = %s. lv_xa = lv_xa BIT-OR lv_xb. %s = lv_xa.", a, b, dst)
	case "xor":
		a, b := c.val(inst.Args[0]), c.val(inst.Args[1])
		c.line("lv_xa = %s. lv_xb = %s. lv_xa = lv_xa BIT-XOR lv_xb. %s = lv_xa.", a, b, dst)
	case "shl":
		c.line("TRY. %s = %s * ipow( base = 2 exp = %s ). CATCH cx_root. %s = 0. ENDTRY.",
			dst, c.val(inst.Args[0]), c.val(inst.Args[1]), dst)
	case "ashr":
		c.line("TRY. %s = %s / ipow( base = 2 exp = %s ). CATCH cx_root. %s = 0. ENDTRY.",
			dst, c.val(inst.Args[0]), c.val(inst.Args[1]), dst)
	case "lshr":
		c.line("TRY. %s = %s / ipow( base = 2 exp = %s ). CATCH cx_root. %s = 0. ENDTRY.",
			dst, c.val(inst.Args[0]), c.val(inst.Args[1]), dst)

	// Comparisons
	case "icmp":
		a, b := c.val(inst.Args[0]), c.val(inst.Args[1])
		switch inst.Predicate {
		case "eq":
			c.line("IF %s = %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, dst, dst)
		case "ne":
			c.line("IF %s <> %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, dst, dst)
		case "slt", "ult":
			c.line("IF %s < %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, dst, dst)
		case "sgt", "ugt":
			c.line("IF %s > %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, dst, dst)
		case "sle", "ule":
			c.line("IF %s <= %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, dst, dst)
		case "sge", "uge":
			c.line("IF %s >= %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, dst, dst)
		default:
			c.line("\" TODO: icmp %s", inst.Predicate)
		}

	// Select
	case "select":
		c.line("IF %s <> 0. %s = %s. ELSE. %s = %s. ENDIF.",
			c.val(inst.SelectCond), dst, c.val(inst.SelectTrue), dst, c.val(inst.SelectFalse))

	// Phi — skip (resolved at branch source)
	case "phi":
		// handled by emitPhiAssigns at branch sites

	// Branch — emit phi assignments before target transition
	case "br":
		if inst.Cond != "" {
			// Conditional branch — phi assigns in each branch
			c.line("IF %s <> 0.", c.val(inst.Cond))
			c.indent++
			c.emitPhiAssigns(inst.TrueLabel)
			c.line("lv_block = '%s'.", inst.TrueLabel)
			c.indent--
			c.line("ELSE.")
			c.indent++
			c.emitPhiAssigns(inst.FalseLabel)
			c.line("lv_block = '%s'.", inst.FalseLabel)
			c.indent--
			c.line("ENDIF.")
		} else if inst.TrueLabel != "" {
			// Unconditional branch
			c.emitPhiAssigns(inst.TrueLabel)
			c.line("lv_block = '%s'.", inst.TrueLabel)
		}

	// Return
	case "ret":
		if len(inst.Args) > 0 && inst.Type.Kind != VoidType {
			c.line("rv = %s. RETURN.", c.val(inst.Args[0]))
		} else {
			c.line("RETURN.")
		}

	// Call
	case "call":
		target := sanitizeName(inst.CallTarget)
		if strings.HasPrefix(inst.CallTarget, "llvm.") {
			c.emitIntrinsic(inst, fn)
			return
		}
		if inst.CallTarget == "__indirect" {
			// Indirect call through function pointer
			// Function pointers are integer IDs → dispatch via CASE
			fptr := c.val(inst.GEPBase)
			nArgs := len(inst.Args)
			hasResult := inst.Result != "" && inst.Type.Kind != VoidType
			c.line("\" indirect call via %s (%d args)", fptr, nArgs)
			c.line("CASE %s.", fptr)
			c.indent++
			for idx, cfn := range c.mod.Functions {
				if cfn.IsExternal || strings.HasPrefix(cfn.Name, "llvm.") {
					continue
				}
				// Match by param count + return type compatibility
				if len(cfn.Params) != nArgs {
					continue
				}
				cfnHasResult := cfn.ReturnType.Kind != VoidType
				if hasResult && !cfnHasResult {
					continue
				}
				// Match param types from call signature
				if len(inst.CallParamTypes) > 0 {
					typeMatch := true
					for pi := 0; pi < nArgs && pi < len(cfn.Params) && pi < len(inst.CallParamTypes); pi++ {
						callT := inst.CallParamTypes[pi].ABAPType()
						paramT := cfn.Params[pi].Type.ABAPType()
						if callT == "" { callT = "i" }
						if paramT == "" { paramT = "i" }
						if callT != paramT {
							typeMatch = false
							break
						}
					}
					if !typeMatch {
						continue
					}
				}
				cname := sanitizeName(cfn.Name)
				// Add class prefix for cross-class calls
				if c.funcToClass != nil {
					if targetClass, ok := c.funcToClass[cfn.Name]; ok && targetClass != c.className {
						cname = targetClass + "=>" + cname
					}
				}
				var callArgs []string
				for i, arg := range inst.Args {
					callArgs = append(callArgs, fmt.Sprintf("%s = CONV #( %s )", paramName(i, cfn.Name), c.val(arg)))
				}
				argStr := strings.Join(callArgs, " ")
				if hasResult && cfnHasResult {
					c.line("WHEN %d. %s = %s( %s ).", idx, dst, cname, argStr)
				} else {
					c.line("WHEN %d. %s( %s ).", idx, cname, argStr)
				}
			}
			if hasResult {
				c.line("WHEN OTHERS. %s = 0.", dst)
			}
			c.indent--
			c.line("ENDCASE.")
			return
		}
		if inst.CallTarget == "__unknown" {
			if inst.Result != "" && inst.Type.Kind != VoidType {
				c.line("%s = 0. \" unknown call", dst)
			}
			return
		}
		calledFn := c.findFunction(inst.CallTarget)
		// External or unknown functions → mem class method call
		if calledFn == nil || calledFn.IsExternal {
			memPfx := c.memPrefix()
			// Map C names that conflict with ABAP keywords
			extName := target
			switch extName {
			case "abort":
				extName = "c_abort"
			case "exit":
				extName = "c_exit"
			}
			var argParts []string
			for i, arg := range inst.Args {
				argParts = append(argParts, fmt.Sprintf("%s = CONV #( %s )", paramName(i, inst.CallTarget), c.val(arg)))
			}
			argStr := strings.Join(argParts, " ")
			if inst.Result != "" && inst.Type.Kind != VoidType {
				if len(argParts) > 0 {
					c.line("%s = %s%s( %s ).", dst, memPfx, extName, argStr)
				} else {
					c.line("%s = %s%s( ).", dst, memPfx, extName)
				}
			} else {
				if len(argParts) > 0 {
					c.line("%s%s( %s ).", memPfx, extName, argStr)
				} else {
					c.line("%s%s( ).", memPfx, extName)
				}
			}
			return
		}
		var argParts []string
		for i, arg := range inst.Args {
			name := paramName(i, inst.CallTarget)
			if calledFn != nil && i < len(calledFn.Params) {
				name = paramName(i, inst.CallTarget)
			}
			argParts = append(argParts, fmt.Sprintf("%s = CONV #( %s )", name, c.val(arg)))
		}
		argStr := strings.Join(argParts, " ")
		// Cross-class call prefix
		callTarget := target
		if c.funcToClass != nil {
			if targetClass, ok := c.funcToClass[inst.CallTarget]; ok && targetClass != c.className {
				callTarget = targetClass + "=>" + target
			}
		}
		if inst.Result != "" && inst.Type.Kind != VoidType {
			c.line("%s = %s( %s ).", dst, callTarget, argStr)
		} else {
			c.line("%s( %s ).", callTarget, argStr)
		}

	// GEP — struct field access or array index
	case "getelementptr":
		if strings.HasPrefix(inst.GEPType, "%struct.") {
			// Struct field access: GEP %struct.Point, ptr %base, i64 0, i32 FIELD_IDX
			if len(inst.GEPIndices) >= 2 {
				structName := strings.TrimPrefix(inst.GEPType, "%")
				fieldIdx := inst.GEPIndices[1] // field index
				fieldName := c.structFieldName(structName, fieldIdx)
				// dst = base pointer offset to field
				c.line("\" GEP: %s = &%s->%s (field %s)", dst, c.val(inst.GEPBase), fieldName, fieldIdx)
				c.line("%s = %s + %s. \" offset to field %s", dst, c.val(inst.GEPBase), c.structFieldOffset(structName, fieldIdx), fieldName)
			}
		} else {
			// Array element: GEP i32, ptr %base, i64 %idx → base + idx * sizeof
			if len(inst.GEPIndices) >= 1 {
				elemSize := c.typeSize(inst.GEPType)
				idx := c.val(inst.GEPIndices[0])
				if elemSize == 1 {
					c.line("%s = %s + %s.", dst, c.val(inst.GEPBase), idx)
				} else {
					c.line("%s = %s + %s * %d.", dst, c.val(inst.GEPBase), idx, elemSize)
				}
			}
		}

	// Load — memory read
	case "load":
		addr := c.val(inst.LoadAddr)
		size := c.typeSizeOf(inst.Type)
		memClass := c.memPrefix()
		switch size {
		case 1:
			c.line("%s = %smem_ld_i32_8u( iv_addr = %s ).", dst, memClass, addr)
		default:
			c.line("%s = %smem_ld_i32( iv_addr = %s ).", dst, memClass, addr)
		}

	// Store — memory write
	case "store":
		addr := c.val(inst.StoreAddr)
		val := c.val(inst.StoreVal)
		size := c.typeSizeOf(inst.Type)
		memClass := c.memPrefix()
		switch size {
		case 1:
			c.line("%smem_st_i32_8( iv_addr = CONV i( %s ) iv_val = CONV i( %s ) ).", memClass, addr, val)
		default:
			c.line("%smem_st_i32( iv_addr = CONV i( %s ) iv_val = CONV i( %s ) ).", memClass, addr, val)
		}

	// Extensions / conversions / casts
	case "zext", "sext", "trunc", "bitcast", "ptrtoint", "inttoptr",
		"sitofp", "uitofp", "fptosi", "fptoui", "fptrunc", "fpext":
		if len(inst.Args) > 0 {
			c.line("%s = %s. \" %s", dst, c.val(inst.Args[0]), inst.Op)
		}

	// Float comparisons
	case "fcmp":
		if len(inst.Args) >= 2 {
			a, b := c.val(inst.Args[0]), c.val(inst.Args[1])
			switch inst.Predicate {
			case "oeq", "ueq":
				c.line("IF %s = %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, dst, dst)
			case "one", "une":
				c.line("IF %s <> %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, dst, dst)
			case "olt", "ult":
				c.line("IF %s < %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, dst, dst)
			case "ogt", "ugt":
				c.line("IF %s > %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, dst, dst)
			case "ole", "ule":
				c.line("IF %s <= %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, dst, dst)
			case "oge", "uge":
				c.line("IF %s >= %s. %s = 1. ELSE. %s = 0. ENDIF.", a, b, dst, dst)
			case "ord":
				c.line("%s = 1. \" fcmp ord (not NaN)", dst)
			case "uno":
				c.line("%s = 0. \" fcmp uno (is NaN)", dst)
			default:
				c.line("IF %s = %s. %s = 1. ELSE. %s = 0. ENDIF. \" fcmp %s", a, b, dst, dst, inst.Predicate)
			}
		}

	// Float negation
	case "fneg":
		if len(inst.Args) > 0 {
			c.line("%s = 0 - %s.", dst, c.val(inst.Args[0]))
		}

	// Aggregate operations
	case "extractvalue":
		// extractvalue {type, type} %val, INDEX → access aggregate element
		if len(inst.Args) > 0 {
			c.line("%s = %s. \" extractvalue", dst, c.val(inst.Args[0]))
		}
	case "insertvalue":
		if len(inst.Args) > 0 {
			c.line("%s = %s. \" insertvalue", dst, c.val(inst.Args[0]))
		}

	// Alloca — local stack allocation → DATA variable (already declared)
	case "alloca":
		c.line("\" alloca: %s (stack var, already in DATA)", dst)

	// Freeze — LLVM poison/undef passthrough
	case "freeze":
		if len(inst.Args) > 0 {
			c.line("%s = %s. \" freeze", dst, c.val(inst.Args[0]))
		}

	// Switch — multi-way branch
	case "switch":
		c.line("CASE %s.", c.val(inst.Cond))
		c.indent++
		for _, pair := range inst.PhiPairs {
			c.emitPhiAssigns(pair.Label)
			c.line("WHEN %s. lv_block = '%s'.", pair.Value, pair.Label)
		}
		c.emitPhiAssigns(inst.FalseLabel)
		c.line("WHEN OTHERS. lv_block = '%s'.", inst.FalseLabel)
		c.indent--
		c.line("ENDCASE.")

	default:
		if inst.Op != "" {
			c.line("\" TODO: %s", inst.Op)
		}
	}
}

func (c *abapCompiler) emitIntrinsic(inst *Instruction, fn *Function) {
	dst := c.ssaName(inst.Result)
	switch {
	case strings.Contains(inst.CallTarget, "abs"):
		c.line("%s = abs( %s ).", dst, c.val(inst.Args[0]))
	case strings.Contains(inst.CallTarget, "smax"):
		a, b := c.val(inst.Args[0]), c.val(inst.Args[1])
		c.line("IF %s > %s. %s = %s. ELSE. %s = %s. ENDIF.", a, b, dst, a, dst, b)
	case strings.Contains(inst.CallTarget, "smin"):
		a, b := c.val(inst.Args[0]), c.val(inst.Args[1])
		c.line("IF %s < %s. %s = %s. ELSE. %s = %s. ENDIF.", a, b, dst, a, dst, b)
	case strings.Contains(inst.CallTarget, "floor"):
		c.line("%s = floor( %s ).", dst, c.val(inst.Args[0]))
	case strings.Contains(inst.CallTarget, "round"):
		c.line("%s = round( val = %s dec = 0 ).", dst, c.val(inst.Args[0]))
	case strings.Contains(inst.CallTarget, "fmuladd"):
		// fmuladd(a, b, c) = a*b + c
		c.line("%s = %s * %s + %s.", dst, c.val(inst.Args[0]), c.val(inst.Args[1]), c.val(inst.Args[2]))
	case strings.Contains(inst.CallTarget, "usub.sat"):
		// saturating unsigned subtract: max(a-b, 0)
		a, b := c.val(inst.Args[0]), c.val(inst.Args[1])
		c.line("%s = %s - %s. IF %s < 0. %s = 0. ENDIF.", dst, a, b, dst, dst)
	case strings.Contains(inst.CallTarget, "frameaddress"):
		c.line("%s = 0. \" frameaddress stub", dst)
	default:
		c.line("%s = 0. \" intrinsic %s (stub)", dst, inst.CallTarget)
	}
}

func (c *abapCompiler) findFunction(name string) *Function {
	for _, fn := range c.mod.Functions {
		if fn.Name == name {
			return fn
		}
	}
	return nil
}

func (c *abapCompiler) isParam(name string) bool {
	_, ok := c.paramMap[name]
	return ok
}

func (c *abapCompiler) ssaName(reg string) string {
	if n, ok := c.paramMap[reg]; ok {
		return n
	}
	// %3 → lv_3, %result → lv_result
	name := strings.TrimPrefix(reg, "%")
	return "lv_" + name
}

func (c *abapCompiler) val(v string) string {
	v = strings.TrimSpace(v)
	// Numeric literal
	if len(v) > 0 && (v[0] >= '0' && v[0] <= '9' || v[0] == '-') {
		// Hex integer constants (0x7FF0000000000000 etc.)
		if strings.HasPrefix(v, "0x") || strings.HasPrefix(v, "0X") {
			var n uint64
			if _, err := fmt.Sscanf(v, "0x%x", &n); err == nil {
				return fmt.Sprintf("%d", int64(n))
			}
			return "0"
		}
		// Convert scientific notation to decimal
		if strings.ContainsAny(v, "eE") && strings.Contains(v, ".") {
			var f float64
			if _, err := fmt.Sscanf(v, "%e", &f); err == nil {
				if f == 0 {
					return "0"
				}
				return fmt.Sprintf("'%g'", f)
			}
		}
		return v
	}
	// Boolean
	if v == "true" {
		return "1"
	}
	if v == "false" {
		return "0"
	}
	// Null pointer
	if v == "null" || v == "zeroinitializer" || v == "undef" || v == "poison" {
		return "0"
	}
	// LLVM global reference (@name) → function index for dispatch
	if strings.HasPrefix(v, "@") {
		fname := strings.TrimPrefix(v, "@")
		for idx, fn := range c.mod.Functions {
			if fn.Name == fname {
				return fmt.Sprintf("%d", idx)
			}
		}
		return "0"
	}
	// Strip LLVM attributes with parentheses: byval(%struct.X), dereferenceable(N)
	if strings.HasPrefix(v, "byval(") || strings.HasPrefix(v, "sret(") ||
		strings.HasPrefix(v, "align(") || strings.HasPrefix(v, "dereferenceable(") {
		return "0"
	}
	if strings.Contains(v, "(") && !strings.HasPrefix(v, "%") && !strings.HasPrefix(v, "-") {
		// Other attr with parens — check if a %var follows
		parts := strings.Fields(v)
		for _, p := range parts {
			if strings.HasPrefix(p, "%") && !strings.Contains(p, "(") {
				return c.ssaName(p)
			}
		}
		return "0"
	}
	// Strip LLVM attributes from values
	for _, attr := range []string{"dereferenceable", "nonnull", "noundef", "signext", "zeroext"} {
		if strings.Contains(v, attr) {
			// Extract the actual value: "dereferenceable(1) %x" → "%x"
			parts := strings.Fields(v)
			for _, p := range parts {
				if strings.HasPrefix(p, "%") {
					return c.ssaName(p)
				}
			}
			return "0"
		}
	}
	// SSA register
	if strings.HasPrefix(v, "%") {
		return c.ssaName(v)
	}
	// LLVM type names leaked as values → 0
	for _, tn := range []string{"i1", "i8", "i16", "i32", "i33", "i64", "i128", "float", "double", "ptr", "void"} {
		if v == tn {
			return "0"
		}
	}
	// Filter LLVM syntax chars that leak into ABAP
	v = strings.TrimRight(v, "}")
	v = strings.TrimLeft(v, "{")
	if v == "" || v == "," {
		return "0"
	}
	return v
}

func (c *abapCompiler) line(format string, args ...any) {
	prefix := strings.Repeat("  ", c.indent)
	c.sb.WriteString(prefix)
	c.sb.WriteString(fmt.Sprintf(format, args...))
	c.sb.WriteByte('\n')
}

func (c *abapCompiler) structFieldName(structName, fieldIdx string) string {
	if _, ok := c.mod.Types[structName]; !ok {
		return "field_" + fieldIdx
	}
	idx := 0
	fmt.Sscanf(fieldIdx, "%d", &idx)
	names := []string{"x", "y", "z"}
	if idx < len(names) {
		return names[idx]
	}
	return fmt.Sprintf("f%d", idx)
}

func (c *abapCompiler) structFieldOffset(structName, fieldIdx string) string {
	st, ok := c.mod.Types[structName]
	if !ok {
		return fieldIdx + " * 4"
	}
	idx := 0
	fmt.Sscanf(fieldIdx, "%d", &idx)
	// Calculate byte offset based on field types
	offset := 0
	for i := 0; i < idx && i < len(st.Fields); i++ {
		offset += c.typeSizeOf(st.Fields[i])
	}
	return fmt.Sprintf("%d", offset)
}

func (c *abapCompiler) typeSize(typeName string) int {
	typeName = strings.TrimSpace(typeName)
	switch typeName {
	case "i8":
		return 1
	case "i16":
		return 2
	case "i32":
		return 4
	case "i64":
		return 8
	case "float":
		return 4
	case "double":
		return 8
	default:
		return 4
	}
}

func (c *abapCompiler) typeSizeOf(t Type) int {
	switch t.Kind {
	case IntType:
		return (t.BitWidth + 7) / 8
	case FloatType:
		return 4
	case DoubleType:
		return 8
	default:
		return 4
	}
}

func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, "$", "_")
	if len(name) > 30 {
		name = name[:30]
	}
	return name
}

func paramName(idx int, fnName string) string {
	// Use meaningful names for common patterns
	names := []string{"a", "b", "c", "d", "e", "f_", "g_", "h"}
	if idx < len(names) {
		return names[idx]
	}
	return fmt.Sprintf("p%d", idx)
}
