// Package ts2go transpiles TypeScript AST (JSON) to Go source code.
// The TS→JSON AST conversion is done by ts_ast.js (TypeScript Compiler API).
// This package handles JSON AST → Go.
package ts2go

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Node represents a TypeScript AST node.
type Node struct {
	Kind       string  `json:"kind"`
	Name       string  `json:"name,omitempty"`
	Text       string  `json:"text,omitempty"`
	Type       string  `json:"type,omitempty"`
	Value      string  `json:"value,omitempty"`
	Visibility string  `json:"visibility,omitempty"`
	Static     bool    `json:"static,omitempty"`
	Readonly   bool    `json:"readonly,omitempty"`
	Const      bool    `json:"const,omitempty"`
	Optional   bool    `json:"optional,omitempty"`
	Extends    string  `json:"extends,omitempty"`
	Operator   string  `json:"operator,omitempty"`
	Property   string  `json:"property,omitempty"`
	Head       string  `json:"head,omitempty"`
	Children   []*Node `json:"children,omitempty"`
	Params     []*Node `json:"params,omitempty"`
	ReturnType string  `json:"returnType,omitempty"`
	Body       []*Node `json:"body,omitempty"`
	Init       *Node   `json:"init,omitempty"`
	Condition  *Node   `json:"condition,omitempty"`
	Then       *Node   `json:"then,omitempty"`
	Else       *Node   `json:"else,omitempty"`
	Left       *Node   `json:"left,omitempty"`
	Right      *Node   `json:"right,omitempty"`
	Object     *Node   `json:"object,omitempty"`
	Index      *Node   `json:"index,omitempty"`
	Expression *Node   `json:"expression,omitempty"`
	Increment  *Node   `json:"increment,omitempty"`
	Arguments  []*Node `json:"arguments,omitempty"`
	Statements []*Node `json:"statements,omitempty"`
	Properties []*struct {
		Key   string `json:"key"`
		Value *Node  `json:"value"`
	} `json:"properties,omitempty"`
	Cases []*struct {
		Kind       string  `json:"kind"`
		Expression *Node   `json:"expression"`
		Statements []*Node `json:"statements"`
	} `json:"cases,omitempty"`
	Spans []*struct {
		Expression *Node  `json:"expression"`
		Text       string `json:"text"`
	} `json:"spans,omitempty"`
}

// TranspileResult holds generated Go files.
type TranspileResult struct {
	Files  map[string]string // filename → Go source
	Pkg    string
}

// Transpile converts a TS AST (JSON) to Go source.
func Transpile(astJSON []byte, pkg string) (*TranspileResult, error) {
	var root Node
	if err := json.Unmarshal(astJSON, &root); err != nil {
		return nil, fmt.Errorf("parse AST JSON: %w", err)
	}

	t := &transpiler{
		pkg:     pkg,
		files:   make(map[string]string),
		typeMap: defaultTypeMap(),
	}

	for _, child := range root.Children {
		switch child.Kind {
		case "ClassDeclaration":
			t.transpileClass(child)
		}
	}

	return &TranspileResult{Files: t.files, Pkg: pkg}, nil
}

type transpiler struct {
	pkg     string
	files   map[string]string
	typeMap map[string]string
	sb      strings.Builder
	indent  int
}

func defaultTypeMap() map[string]string {
	return map[string]string{
		"number":    "int",
		"string":    "string",
		"boolean":   "bool",
		"void":      "",
		"any":       "interface{}",
		"undefined": "interface{}",
	}
}

func (t *transpiler) mapType(tsType string) string {
	if tsType == "" {
		return ""
	}
	if strings.HasSuffix(tsType, "[]") {
		elem := tsType[:len(tsType)-2]
		return "[]" + t.mapType(elem)
	}
	if strings.Contains(tsType, " | ") {
		return "interface{}" // union types
	}
	if g, ok := t.typeMap[tsType]; ok {
		return g
	}
	// Class/interface reference → pointer
	return "*" + tsType
}

func (t *transpiler) inferType(init *Node) string {
	if init == nil {
		return "interface{}"
	}
	switch init.Kind {
	case "NumericLiteral":
		if strings.Contains(init.Value, ".") {
			return "float64"
		}
		return "int"
	case "StringLiteral":
		return "string"
	case "BooleanLiteral":
		return "bool"
	case "ArrayLiteral":
		return "[]interface{}"
	case "NullLiteral":
		return "interface{}"
	case "NewExpression":
		return "*" + init.Name
	case "PrefixUnaryExpression":
		// e.g., -1
		return t.inferType(init.Expression)
	}
	return "interface{}"
}

func (t *transpiler) line(format string, args ...any) {
	t.sb.WriteString(strings.Repeat("\t", t.indent))
	fmt.Fprintf(&t.sb, format, args...)
	t.sb.WriteByte('\n')
}

func (t *transpiler) raw(s string) {
	t.sb.WriteString(s)
}

// --- Class ---

func (t *transpiler) transpileClass(node *Node) {
	name := node.Name
	t.sb.Reset()

	t.line("package %s", t.pkg)
	t.line("")

	// Collect members
	var fields, methods, constructor []*Node
	for _, child := range node.Children {
		switch child.Kind {
		case "PropertyDeclaration":
			fields = append(fields, child)
		case "MethodDeclaration":
			methods = append(methods, child)
		case "Constructor":
			constructor = append(constructor, child)
		}
	}

	// Struct
	t.line("// %s is transpiled from TypeScript.", name)
	t.line("type %s struct {", name)
	t.indent++
	for _, f := range fields {
		goType := t.mapType(f.Type)
		if goType == "" {
			// Infer type from initializer
			goType = t.inferType(f.Init)
		}
		t.line("%s %s", goFieldName(f.Name), goType)
	}
	t.indent--
	t.line("}")
	t.line("")

	// Constructor → New function
	if len(constructor) > 0 {
		t.emitConstructor(name, constructor[0])
	}

	// Methods
	for _, m := range methods {
		t.emitMethod(name, m)
	}

	t.files[toSnakeCase(name)+".go"] = t.sb.String()
}

func (t *transpiler) emitConstructor(className string, node *Node) {
	params := t.goParams(node.Params)
	t.line("func New%s(%s) *%s {", className, params, className)
	t.indent++
	t.line("l := &%s{}", className)
	for _, stmt := range node.Body {
		t.emitStatement(stmt)
	}
	t.line("return l")
	t.indent--
	t.line("}")
	t.line("")
}

func (t *transpiler) emitMethod(className string, node *Node) {
	methodName := goPublicName(node.Name)
	params := t.goParams(node.Params)
	ret := ""
	if node.ReturnType != "" && node.ReturnType != "void" {
		ret = " " + t.mapType(node.ReturnType)
	} else if node.ReturnType == "" && hasReturnValue(node.Body) {
		ret = " interface{}" // infer: method returns something but type unknown
	}
	t.line("func (l *%s) %s(%s)%s {", className, methodName, params, ret)
	t.indent++
	for _, stmt := range node.Body {
		t.emitStatement(stmt)
	}
	t.indent--
	t.line("}")
	t.line("")
}

func hasReturnValue(body []*Node) bool {
	for _, s := range body {
		if s.Kind == "ReturnStatement" && s.Expression != nil {
			return true
		}
	}
	return false
}

func (t *transpiler) goParams(params []*Node) string {
	var parts []string
	for _, p := range params {
		goType := t.mapType(p.Type)
		if goType == "" {
			goType = "interface{}"
		}
		parts = append(parts, fmt.Sprintf("%s %s", goVarName(p.Name), goType))
	}
	return strings.Join(parts, ", ")
}

// --- Statements ---

func (t *transpiler) emitStatement(node *Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case "VariableDeclaration":
		t.emitVarDecl(node)
	case "Assignment":
		t.emitAssignment(node)
	case "ReturnStatement":
		if node.Expression != nil {
			t.line("return %s", t.expr(node.Expression))
		} else {
			t.line("return")
		}
	case "IfStatement":
		t.emitIf(node)
	case "WhileStatement":
		t.line("for %s {", t.expr(node.Condition))
		t.indent++
		for _, s := range node.Body {
			t.emitStatement(s)
		}
		t.indent--
		t.line("}")
	case "ForStatement":
		t.emitFor(node)
	case "SwitchStatement":
		t.emitSwitch(node)
	case "ContinueStatement":
		t.line("continue")
	case "BreakStatement":
		t.line("break")
	case "ThrowStatement":
		t.line("panic(%s)", t.expr(node.Expression))
	case "ExpressionStatement":
		if node.Expression != nil {
			t.line("%s", t.expr(node.Expression))
		}
	case "Block":
		for _, s := range node.Statements {
			t.emitStatement(s)
		}
	default:
		t.line("// TODO: %s", node.Kind)
	}
}

func (t *transpiler) emitVarDecl(node *Node) {
	if node.Init != nil {
		t.line("%s := %s", goVarName(node.Name), t.expr(node.Init))
	} else {
		goType := t.mapType(node.Type)
		if goType == "" {
			goType = "interface{}"
		}
		t.line("var %s %s", goVarName(node.Name), goType)
	}
}

func (t *transpiler) emitAssignment(node *Node) {
	lhs := t.expr(node.Left)
	rhs := t.expr(node.Right)
	if node.Operator != "" && node.Operator != "=" {
		t.line("%s %s %s", lhs, node.Operator, rhs)
	} else {
		t.line("%s = %s", lhs, rhs)
	}
}

func (t *transpiler) emitIf(node *Node) {
	t.line("if %s {", t.expr(node.Condition))
	t.indent++
	if node.Then != nil {
		for _, s := range node.Then.Statements {
			t.emitStatement(s)
		}
	}
	t.indent--
	if node.Else != nil {
		if node.Else.Kind == "IfStatement" {
			// else if — emit on same line
			t.sb.WriteString(strings.Repeat("\t", t.indent))
			t.sb.WriteString("} else ")
			t.emitIf(node.Else)
			return
		}
		t.line("} else {")
		t.indent++
		for _, s := range node.Else.Statements {
			t.emitStatement(s)
		}
		t.indent--
	}
	t.line("}")
}

func (t *transpiler) emitFor(node *Node) {
	init := ""
	if node.Init != nil {
		if node.Init.Kind == "VariableDeclaration" {
			if node.Init.Init != nil {
				init = fmt.Sprintf("%s := %s", goVarName(node.Init.Name), t.expr(node.Init.Init))
			}
		}
	}
	cond := ""
	if node.Condition != nil {
		cond = t.expr(node.Condition)
	}
	inc := ""
	if node.Increment != nil {
		switch node.Increment.Kind {
		case "Assignment":
			if node.Increment.Operator != "" && node.Increment.Operator != "=" {
				inc = fmt.Sprintf("%s %s %s", t.expr(node.Increment.Left), node.Increment.Operator, t.expr(node.Increment.Right))
			} else {
				inc = fmt.Sprintf("%s = %s", t.expr(node.Increment.Left), t.expr(node.Increment.Right))
			}
		case "ExpressionStatement":
			inc = t.expr(node.Increment.Expression)
		}
	}
	t.line("for %s; %s; %s {", init, cond, inc)
	t.indent++
	for _, s := range node.Body {
		t.emitStatement(s)
	}
	t.indent--
	t.line("}")
}

func (t *transpiler) emitSwitch(node *Node) {
	t.line("switch %s {", t.expr(node.Expression))
	for _, c := range node.Cases {
		if c.Kind == "CaseClause" {
			t.line("case %s:", t.expr(c.Expression))
		} else {
			t.line("default:")
		}
		t.indent++
		for _, s := range c.Statements {
			if s.Kind == "BreakStatement" {
				continue // Go switches don't fallthrough by default
			}
			t.emitStatement(s)
		}
		t.indent--
	}
	t.line("}")
}

// --- Expressions ---

func (t *transpiler) expr(node *Node) string {
	if node == nil {
		return "nil"
	}
	switch node.Kind {
	case "NumericLiteral":
		return node.Value
	case "StringLiteral":
		return fmt.Sprintf("%q", node.Value)
	case "BooleanLiteral":
		return node.Value
	case "NullLiteral":
		return "nil"
	case "RegexLiteral":
		return fmt.Sprintf("regexp.MustCompile(`%s`)", strings.Trim(node.Value, "/"))
	case "Identifier":
		return goVarName(node.Name)
	case "ThisExpression":
		return "l"
	case "PropertyAccess":
		obj := t.expr(node.Object)
		// JS/TS built-in property mappings
		switch node.Property {
		case "length":
			return fmt.Sprintf("len(%s)", obj)
		}
		return fmt.Sprintf("%s.%s", obj, goFieldName(node.Property))
	case "IndexAccess":
		return fmt.Sprintf("%s[%s]", t.expr(node.Object), t.expr(node.Index))
	case "BinaryExpression":
		op := node.Operator
		if op == "===" {
			op = "=="
		} else if op == "!==" {
			op = "!="
		}
		return fmt.Sprintf("%s %s %s", t.expr(node.Left), op, t.expr(node.Right))
	case "PrefixUnaryExpression":
		return fmt.Sprintf("%s%s", node.Operator, t.expr(node.Expression))
	case "PostfixUnaryExpression":
		return fmt.Sprintf("%s%s", t.expr(node.Expression), node.Operator)
	case "ConditionalExpression":
		// Go doesn't have ternary — caller should handle. For now, inline func.
		return fmt.Sprintf("func() interface{} { if %s { return %s }; return %s }()",
			t.expr(node.Condition), t.expr(node.Then), t.expr(node.Else))
	case "MethodCall":
		obj := t.expr(node.Object)
		args := t.exprList(node.Arguments)
		method := goPublicName(node.Property)
		// String methods mapping
		switch node.Property {
		case "length":
			return fmt.Sprintf("len(%s)", obj)
		case "push":
			return fmt.Sprintf("%s = append(%s, %s)", obj, obj, args)
		case "trim":
			return fmt.Sprintf("strings.TrimSpace(%s)", obj)
		case "charAt":
			if len(node.Arguments) == 1 {
				idx := t.expr(node.Arguments[0])
				return fmt.Sprintf("string(%s[%s])", obj, idx)
			}
			return fmt.Sprintf("string(%s[%s])", obj, args)
		case "indexOf":
			return fmt.Sprintf("strings.Index(%s, %s)", obj, args)
		case "substring":
			argList := t.exprListSlice(node.Arguments)
			if len(argList) == 2 {
				return fmt.Sprintf("%s[%s:%s]", obj, argList[0], argList[1])
			}
			return fmt.Sprintf("%s[%s:]", obj, args)
		case "substr":
			argList := t.exprListSlice(node.Arguments)
			if len(argList) == 2 {
				return fmt.Sprintf("%s[%s:%s+%s]", obj, argList[0], argList[0], argList[1])
			}
			return fmt.Sprintf("%s[%s:]", obj, args)
		case "replace":
			argList := t.exprListSlice(node.Arguments)
			if len(argList) == 2 {
				return fmt.Sprintf("strings.ReplaceAll(%s, %s, %s)", obj, argList[0], argList[1])
			}
		case "toUpperCase":
			return fmt.Sprintf("strings.ToUpper(%s)", obj)
		case "toLowerCase":
			return fmt.Sprintf("strings.ToLower(%s)", obj)
		case "test":
			return fmt.Sprintf("%s.MatchString(%s)", obj, args)
		}
		return fmt.Sprintf("%s.%s(%s)", obj, method, args)
	case "FunctionCall":
		args := t.exprList(node.Arguments)
		return fmt.Sprintf("%s(%s)", goVarName(node.Name), args)
	case "NewExpression":
		args := t.exprList(node.Arguments)
		return fmt.Sprintf("New%s(%s)", node.Name, args)
	case "ArrayLiteral":
		if len(node.Children) == 0 {
			return "nil"
		}
		var elems []string
		for _, c := range node.Children {
			elems = append(elems, t.expr(c))
		}
		return fmt.Sprintf("[]interface{}{%s}", strings.Join(elems, ", "))
	case "ObjectLiteral":
		if len(node.Properties) == 0 {
			return "map[string]interface{}{}"
		}
		var pairs []string
		for _, p := range node.Properties {
			pairs = append(pairs, fmt.Sprintf("%q: %s", p.Key, t.expr(p.Value)))
		}
		return fmt.Sprintf("map[string]interface{}{%s}", strings.Join(pairs, ", "))
	case "TemplateLiteral":
		// Convert to fmt.Sprintf
		format := node.Head
		var args []string
		for _, span := range node.Spans {
			format += "%v" + span.Text
			args = append(args, t.expr(span.Expression))
		}
		if len(args) == 0 {
			return fmt.Sprintf("%q", format)
		}
		return fmt.Sprintf("fmt.Sprintf(%q, %s)", format, strings.Join(args, ", "))
	default:
		return fmt.Sprintf("/* TODO: %s */nil", node.Kind)
	}
}

func (t *transpiler) exprList(nodes []*Node) string {
	var parts []string
	for _, n := range nodes {
		parts = append(parts, t.expr(n))
	}
	return strings.Join(parts, ", ")
}

func (t *transpiler) exprListSlice(nodes []*Node) []string {
	var parts []string
	for _, n := range nodes {
		parts = append(parts, t.expr(n))
	}
	return parts
}

// --- Naming helpers ---

func goFieldName(tsName string) string {
	if tsName == "" {
		return "_"
	}
	// Capitalize first letter for exported field
	return strings.ToUpper(tsName[:1]) + tsName[1:]
}

func goPublicName(tsName string) string {
	if tsName == "" {
		return "_"
	}
	return strings.ToUpper(tsName[:1]) + tsName[1:]
}

func goVarName(tsName string) string {
	if tsName == "" {
		return "_"
	}
	// Keep lowercase for local variables
	return tsName
}

func toSnakeCase(name string) string {
	var result strings.Builder
	for i, c := range name {
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				result.WriteByte('_')
			}
			result.WriteRune(c + 32)
		} else {
			result.WriteRune(c)
		}
	}
	return result.String()
}
