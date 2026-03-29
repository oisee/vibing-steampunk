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
	GEPBase     string   // base pointer
	GEPType     string   // struct type name
	GEPIndices  []string // index values
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

		// call instruction
		if strings.Contains(rest, "call ") {
			inst.Op = "call"
			re := regexp.MustCompile(`call\s+\S+\s+@(\w+)\(([^)]*)\)`)
			m := re.FindStringSubmatch(rest)
			if m != nil {
				inst.CallTarget = m[1]
				inst.Type = parseType(strings.Fields(rest)[1])
				// Parse call args
				if m[2] != "" {
					for _, arg := range strings.Split(m[2], ",") {
						parts := strings.Fields(strings.TrimSpace(arg))
						if len(parts) >= 2 {
							inst.Args = append(inst.Args, parts[len(parts)-1])
						}
					}
				}
			}
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

		// Conversion ops: zext, sext, trunc
		if strings.HasPrefix(rest, "zext ") || strings.HasPrefix(rest, "sext ") || strings.HasPrefix(rest, "trunc ") {
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
	}
	return strings.TrimSpace(s)
}

// Compile converts a parsed LLVM IR Module to ABAP source.
func Compile(mod *Module, className string) string {
	c := &abapCompiler{mod: mod, className: className}
	return c.emit()
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
}

type phiAssign struct {
	dst string // lv_4
	val string // lv_8 or "2"
}

func (c *abapCompiler) emit() string {
	c.line("CLASS %s DEFINITION PUBLIC FINAL CREATE PUBLIC.", c.className)
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

	return c.sb.String()
}

func (c *abapCompiler) emitMethodDecl(fn *Function) {
	parts := []string{"CLASS-METHODS " + sanitizeName(fn.Name)}

	if len(fn.Params) > 0 {
		var params []string
		for i, p := range fn.Params {
			name := paramName(i, fn.Name)
			params = append(params, fmt.Sprintf("%s TYPE %s", name, p.Type.ABAPType()))
		}
		parts = append(parts, "IMPORTING "+strings.Join(params, " "))
	}

	if fn.ReturnType.Kind != VoidType {
		parts = append(parts, fmt.Sprintf("RETURNING VALUE(rv) TYPE %s", fn.ReturnType.ABAPType()))
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
		c.line("DATA: %s.", strings.Join(decls, ", "))
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

func (c *abapCompiler) emitWithDispatcher(fn *Function) {
	c.buildPhiMap(fn)

	// Determine first block label
	firstLabel := "entry"
	if len(fn.Blocks) > 0 {
		firstLabel = fn.Blocks[0].Label
	}

	c.line("DATA lv_block TYPE string VALUE '%s'.", firstLabel)
	c.line("DO.")
	c.indent++
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
		c.line("DATA(lx_a_%s) = CONV x4( %s ). DATA(lx_b_%s) = CONV x4( %s ). %s = lx_a_%s BIT-AND lx_b_%s.",
			dst, c.val(inst.Args[0]), dst, c.val(inst.Args[1]), dst, dst, dst)
	case "or":
		c.line("DATA(lx_a_%s) = CONV x4( %s ). DATA(lx_b_%s) = CONV x4( %s ). %s = lx_a_%s BIT-OR lx_b_%s.",
			dst, c.val(inst.Args[0]), dst, c.val(inst.Args[1]), dst, dst, dst)
	case "xor":
		c.line("DATA(lx_a_%s) = CONV x4( %s ). DATA(lx_b_%s) = CONV x4( %s ). %s = lx_a_%s BIT-XOR lx_b_%s.",
			dst, c.val(inst.Args[0]), dst, c.val(inst.Args[1]), dst, dst, dst)
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
			// LLVM intrinsic
			c.emitIntrinsic(inst, fn)
			return
		}
		var argParts []string
		calledFn := c.findFunction(inst.CallTarget)
		for i, arg := range inst.Args {
			name := paramName(i, inst.CallTarget)
			if calledFn != nil && i < len(calledFn.Params) {
				name = paramName(i, inst.CallTarget)
			}
			argParts = append(argParts, fmt.Sprintf("%s = %s", name, c.val(arg)))
		}
		argStr := strings.Join(argParts, " ")
		if inst.Result != "" && inst.Type.Kind != VoidType {
			c.line("%s = %s( %s ).", dst, target, argStr)
		} else {
			c.line("%s( %s ).", target, argStr)
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
		switch size {
		case 1:
			c.line("PERFORM mem_ld_i32_8u USING %s CHANGING %s.", addr, dst)
		case 4:
			c.line("PERFORM mem_ld_i32 USING %s CHANGING %s.", addr, dst)
		default:
			c.line("PERFORM mem_ld_i32 USING %s CHANGING %s.", addr, dst)
		}

	// Store — memory write
	case "store":
		addr := c.val(inst.StoreAddr)
		val := c.val(inst.StoreVal)
		size := c.typeSizeOf(inst.Type)
		switch size {
		case 1:
			c.line("PERFORM mem_st_i32_8 USING %s %s.", addr, val)
		case 4:
			c.line("PERFORM mem_st_i32 USING %s %s.", addr, val)
		default:
			c.line("PERFORM mem_st_i32 USING %s %s.", addr, val)
		}

	// Extensions / conversions
	case "zext", "sext", "trunc":
		if len(inst.Args) > 0 {
			c.line("%s = %s. \" %s", dst, c.val(inst.Args[0]), inst.Op)
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
	default:
		c.line("\" TODO: intrinsic %s", inst.CallTarget)
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
		return v
	}
	// Boolean
	if v == "true" {
		return "1"
	}
	if v == "false" {
		return "0"
	}
	// SSA register
	if strings.HasPrefix(v, "%") {
		return c.ssaName(v)
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
