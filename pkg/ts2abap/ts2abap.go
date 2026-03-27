// Package ts2abap transpiles TypeScript AST (JSON) to ABAP source code.
// The TS→JSON AST conversion is done by a small Node.js script using
// the TypeScript compiler API. This package handles JSON AST → ABAP.
package ts2abap

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Node represents a TypeScript AST node (simplified from TS compiler API).
type Node struct {
	Kind       string  `json:"kind"`
	Name       string  `json:"name,omitempty"`
	Text       string  `json:"text,omitempty"`
	Type       string  `json:"type,omitempty"`
	Value      string  `json:"value,omitempty"`
	Visibility string  `json:"visibility,omitempty"` // public, private, protected
	Static     bool    `json:"static,omitempty"`
	Children   []*Node `json:"children,omitempty"`

	// For specific node types
	Params     []*Node `json:"params,omitempty"`
	ReturnType string  `json:"returnType,omitempty"`
	Body       []*Node `json:"body,omitempty"`
	Init       *Node   `json:"init,omitempty"`
	Condition  *Node   `json:"condition,omitempty"`
	Then       *Node   `json:"then,omitempty"`
	Else       *Node   `json:"else,omitempty"`
	Left       *Node   `json:"left,omitempty"`
	Right      *Node   `json:"right,omitempty"`
	Operator   string  `json:"operator,omitempty"`
	Object     *Node   `json:"object,omitempty"`
	Property   string  `json:"property,omitempty"`
	Arguments  []*Node `json:"arguments,omitempty"`
	Expression *Node   `json:"expression,omitempty"`
	Increment  *Node   `json:"increment,omitempty"`
	Statements []*Node `json:"statements,omitempty"`
	ElementType string `json:"elementType,omitempty"`
}

// TranspileResult holds the generated ABAP code.
type TranspileResult struct {
	Classes map[string]string // class name → ABAP source
	Prefix  string            // naming prefix (e.g., "zcl_")
}

// Transpile converts a TS AST (JSON) to ABAP source.
func Transpile(astJSON []byte, prefix string) (*TranspileResult, error) {
	var root Node
	if err := json.Unmarshal(astJSON, &root); err != nil {
		return nil, fmt.Errorf("parse AST JSON: %w", err)
	}

	t := &transpiler{
		prefix:  prefix,
		classes: make(map[string]string),
		typeMap: defaultTypeMap(),
	}

	for _, child := range root.Children {
		switch child.Kind {
		case "ClassDeclaration":
			t.transpileClass(child)
		}
	}

	return &TranspileResult{
		Classes: t.classes,
		Prefix:  prefix,
	}, nil
}

type transpiler struct {
	prefix  string
	classes map[string]string
	typeMap map[string]string // TS type → ABAP type
	sb      strings.Builder
	indent  int
}

func defaultTypeMap() map[string]string {
	return map[string]string{
		"number":  "i",
		"string":  "string",
		"boolean": "abap_bool",
		"void":    "",
		"any":     "REF TO data",
		"undefined": "REF TO data",
	}
}

func (t *transpiler) mapType(tsType string) string {
	// Array types: Token[] → TABLE OF REF TO zcl_token
	if strings.HasSuffix(tsType, "[]") {
		elemType := tsType[:len(tsType)-2]
		abapElem := t.mapType(elemType)
		if strings.HasPrefix(abapElem, "REF TO") {
			return "TABLE OF " + abapElem
		}
		return "TABLE OF " + abapElem
	}

	// Built-in types
	if abap, ok := t.typeMap[tsType]; ok {
		return abap
	}

	// Class reference
	return "REF TO " + t.abapClassName(tsType)
}

func (t *transpiler) abapClassName(tsName string) string {
	return t.prefix + strings.ToLower(tsName)
}

func (t *transpiler) line(format string, args ...any) {
	prefix := strings.Repeat("  ", t.indent)
	t.sb.WriteString(prefix)
	fmt.Fprintf(&t.sb, format, args...)
	t.sb.WriteByte('\n')
}

// --- Class Transpilation ---

func (t *transpiler) transpileClass(node *Node) {
	className := t.abapClassName(node.Name)
	t.sb.Reset()

	// Collect members by visibility
	var publicMethods, privateMethods []*Node
	var publicAttrs, privateAttrs []*Node
	var constructor *Node

	for _, child := range node.Children {
		switch child.Kind {
		case "PropertyDeclaration":
			if child.Visibility == "private" || child.Visibility == "" {
				privateAttrs = append(privateAttrs, child)
			} else {
				publicAttrs = append(publicAttrs, child)
			}
		case "MethodDeclaration":
			if child.Visibility == "private" {
				privateMethods = append(privateMethods, child)
			} else {
				publicMethods = append(publicMethods, child)
			}
		case "Constructor":
			constructor = child
		}
	}

	// CLASS DEFINITION
	t.line("CLASS %s DEFINITION PUBLIC FINAL CREATE PUBLIC.", className)
	t.indent++

	// PUBLIC SECTION
	t.line("PUBLIC SECTION.")
	t.indent++
	if constructor != nil {
		t.emitMethodSignature("constructor", constructor.Params, "")
	}
	for _, m := range publicMethods {
		t.emitMethodSignature(t.abapMethodName(m.Name), m.Params, m.ReturnType)
	}
	for _, a := range publicAttrs {
		t.emitAttribute(a)
	}
	t.indent--

	// PRIVATE SECTION
	if len(privateAttrs) > 0 || len(privateMethods) > 0 {
		t.line("PRIVATE SECTION.")
		t.indent++
		for _, a := range privateAttrs {
			t.emitAttribute(a)
		}
		for _, m := range privateMethods {
			t.emitMethodSignature(t.abapMethodName(m.Name), m.Params, m.ReturnType)
		}
		t.indent--
	}

	t.indent--
	t.line("ENDCLASS.")
	t.line("")

	// CLASS IMPLEMENTATION
	t.line("CLASS %s IMPLEMENTATION.", className)
	t.indent++

	if constructor != nil {
		t.emitMethod("constructor", constructor.Params, "", constructor.Body)
	}
	for _, m := range publicMethods {
		t.emitMethod(t.abapMethodName(m.Name), m.Params, m.ReturnType, m.Body)
	}
	for _, m := range privateMethods {
		t.emitMethod(t.abapMethodName(m.Name), m.Params, m.ReturnType, m.Body)
	}

	t.indent--
	t.line("ENDCLASS.")

	t.classes[className] = t.sb.String()
}

func (t *transpiler) emitMethodSignature(name string, params []*Node, returnType string) {
	parts := []string{"METHODS " + name}

	if len(params) > 0 {
		var ps []string
		for _, p := range params {
			abapType := t.mapType(p.Type)
			ps = append(ps, fmt.Sprintf("iv_%s TYPE %s", strings.ToLower(p.Name), abapType))
		}
		parts = append(parts, "IMPORTING "+strings.Join(ps, " "))
	}

	if returnType != "" && returnType != "void" {
		abapType := t.mapType(returnType)
		if abapType != "" {
			parts = append(parts, fmt.Sprintf("RETURNING VALUE(rv_result) TYPE %s", abapType))
		}
	}

	t.line("%s.", strings.Join(parts, " "))
}

func (t *transpiler) emitAttribute(node *Node) {
	abapType := t.mapType(node.Type)
	name := "mv_" + strings.ToLower(node.Name)
	if strings.HasPrefix(abapType, "TABLE OF") {
		name = "mt_" + strings.ToLower(node.Name)
		t.line("DATA %s TYPE %s.", name, abapType)
	} else if strings.HasPrefix(abapType, "REF TO") {
		name = "mo_" + strings.ToLower(node.Name)
		t.line("DATA %s TYPE %s.", name, abapType)
	} else {
		t.line("DATA %s TYPE %s.", name, abapType)
	}
}

// --- Method Body Transpilation ---

func (t *transpiler) emitMethod(name string, params []*Node, returnType string, body []*Node) {
	t.line("METHOD %s.", name)
	t.indent++

	for _, stmt := range body {
		t.emitStatement(stmt)
	}

	t.indent--
	t.line("ENDMETHOD.")
}

func (t *transpiler) emitStatement(node *Node) {
	if node == nil {
		return
	}

	switch node.Kind {
	case "VariableDeclaration":
		t.emitVarDecl(node)
	case "Assignment":
		t.line("%s = %s.", t.emitExpr(node.Left), t.emitExpr(node.Right))
	case "ExpressionStatement":
		expr := t.emitExpr(node.Expression)
		if expr != "" {
			t.line("%s.", expr)
		}
	case "ReturnStatement":
		if node.Expression != nil {
			t.line("rv_result = %s.", t.emitExpr(node.Expression))
			t.line("RETURN.")
		} else {
			t.line("RETURN.")
		}
	case "IfStatement":
		t.line("IF %s.", t.emitExpr(node.Condition))
		t.indent++
		if node.Then != nil {
			for _, s := range node.Then.Statements {
				t.emitStatement(s)
			}
		}
		if node.Else != nil {
			t.indent--
			t.line("ELSE.")
			t.indent++
			if node.Else.Kind == "IfStatement" {
				t.indent--
				t.line("ELSEIF %s.", t.emitExpr(node.Else.Condition))
				t.indent++
				if node.Else.Then != nil {
					for _, s := range node.Else.Then.Statements {
						t.emitStatement(s)
					}
				}
			} else {
				for _, s := range node.Else.Statements {
					t.emitStatement(s)
				}
			}
		}
		t.indent--
		t.line("ENDIF.")

	case "WhileStatement":
		t.line("WHILE %s.", t.emitExpr(node.Condition))
		t.indent++
		for _, s := range node.Body {
			t.emitStatement(s)
		}
		t.indent--
		t.line("ENDWHILE.")

	case "ForStatement":
		// for (init; cond; incr) { body }
		if node.Init != nil {
			t.emitStatement(node.Init)
		}
		t.line("WHILE %s.", t.emitExpr(node.Condition))
		t.indent++
		for _, s := range node.Body {
			t.emitStatement(s)
		}
		if node.Increment != nil {
			t.emitStatement(node.Increment)
		}
		t.indent--
		t.line("ENDWHILE.")

	case "ContinueStatement":
		t.line("CONTINUE.")

	case "BreakStatement":
		t.line("EXIT.")

	case "Block":
		for _, s := range node.Statements {
			t.emitStatement(s)
		}
	}
}

func (t *transpiler) emitVarDecl(node *Node) {
	name := "lv_" + strings.ToLower(node.Name)
	abapType := "i" // default
	if node.Type != "" {
		abapType = t.mapType(node.Type)
	}

	if strings.HasPrefix(abapType, "TABLE OF") {
		name = "lt_" + strings.ToLower(node.Name)
	} else if strings.HasPrefix(abapType, "REF TO") {
		name = "lo_" + strings.ToLower(node.Name)
	}

	if node.Init != nil {
		initExpr := t.emitExpr(node.Init)
		t.line("DATA(%s) = %s.", name, initExpr)
	} else {
		t.line("DATA %s TYPE %s.", name, abapType)
	}
}

// --- Expression Transpilation ---

func (t *transpiler) emitExpr(node *Node) string {
	if node == nil {
		return ""
	}

	switch node.Kind {
	case "NumericLiteral":
		return node.Value

	case "StringLiteral":
		// TS: "hello" or 'hello' → ABAP: 'hello'
		val := node.Value
		val = strings.ReplaceAll(val, "'", "''") // escape single quotes
		return "'" + val + "'"

	case "BooleanLiteral":
		if node.Value == "true" {
			return "abap_true"
		}
		return "abap_false"

	case "Identifier":
		return t.mapIdentifier(node.Name)

	case "BinaryExpression":
		left := t.emitExpr(node.Left)
		right := t.emitExpr(node.Right)
		op := t.mapOperator(node.Operator)
		return fmt.Sprintf("%s %s %s", left, op, right)

	case "UnaryExpression":
		operand := t.emitExpr(node.Expression)
		switch node.Operator {
		case "!":
			return fmt.Sprintf("xsdbool( %s = abap_false )", operand)
		case "-":
			return "- " + operand
		case "++":
			return operand + " + 1"
		}
		return operand

	case "PropertyAccess":
		obj := t.emitExpr(node.Object)
		prop := node.Property
		return t.mapPropertyAccess(obj, prop)

	case "MethodCall":
		obj := t.emitExpr(node.Object)
		method := node.Property
		var args []string
		for _, a := range node.Arguments {
			args = append(args, t.emitExpr(a))
		}
		return t.mapMethodCall(obj, method, args)

	case "NewExpression":
		className := t.abapClassName(node.Name)
		var args []string
		for _, a := range node.Arguments {
			args = append(args, t.emitExpr(a))
		}
		if len(args) > 0 {
			var ps []string
			for i, a := range args {
				ps = append(ps, fmt.Sprintf("iv_p%d = %s", i+1, a))
			}
			return fmt.Sprintf("NEW %s( %s )", className, strings.Join(ps, " "))
		}
		return fmt.Sprintf("NEW %s( )", className)

	case "ThisExpression":
		return "me"

	case "ArrayLiteral":
		return "VALUE #( )" // empty array

	case "ConditionalExpression":
		cond := t.emitExpr(node.Condition)
		then := t.emitExpr(node.Then)
		els := t.emitExpr(node.Else)
		return fmt.Sprintf("COND #( WHEN %s THEN %s ELSE %s )", cond, then, els)

	case "NullLiteral":
		return "VALUE #( )" // or REF #( ) depending on context
	}

	return "\" TODO: " + node.Kind
}

func (t *transpiler) mapIdentifier(name string) string {
	switch name {
	case "this":
		return "me"
	case "true":
		return "abap_true"
	case "false":
		return "abap_false"
	case "undefined", "null":
		return "VALUE #( )"
	}

	// Check if it's a local variable or parameter
	return "lv_" + strings.ToLower(name)
}

func (t *transpiler) mapOperator(op string) string {
	switch op {
	case "===", "==":
		return "="
	case "!==", "!=":
		return "<>"
	case "&&":
		return "AND"
	case "||":
		return "OR"
	case "+":
		return "+" // works for both numbers and strings (with &&)
	case ">=":
		return ">="
	case "<=":
		return "<="
	default:
		return op
	}
}

func (t *transpiler) mapPropertyAccess(obj, prop string) string {
	// this.x → me->mv_x
	if obj == "me" {
		return "me->mv_" + strings.ToLower(prop)
	}
	// obj.length → strlen( obj ) or lines( obj )
	if prop == "length" {
		return fmt.Sprintf("strlen( %s )", obj)
	}
	return fmt.Sprintf("%s->mv_%s", obj, strings.ToLower(prop))
}

func (t *transpiler) mapMethodCall(obj, method string, args []string) string {
	argStr := strings.Join(args, " ")

	// String methods
	switch method {
	case "charAt":
		if len(args) == 1 {
			return fmt.Sprintf("%s+%s(1)", obj, args[0])
		}
	case "indexOf":
		if len(args) == 1 {
			return fmt.Sprintf("find( val = %s sub = %s )", obj, args[0])
		}
		if len(args) == 2 {
			return fmt.Sprintf("find( val = %s sub = %s off = %s )", obj, args[0], args[1])
		}
	case "substring", "substr":
		if len(args) == 2 {
			return fmt.Sprintf("substring( val = %s off = %s len = %s - %s )", obj, args[0], args[1], args[0])
		}
		if len(args) == 1 {
			return fmt.Sprintf("substring( val = %s off = %s )", obj, args[0])
		}
	case "push":
		return fmt.Sprintf("APPEND %s TO %s", argStr, obj)
	case "trim":
		return fmt.Sprintf("condense( %s )", obj)
	case "replace":
		if len(args) == 2 {
			return fmt.Sprintf("replace( val = %s sub = %s with = %s )", obj, args[0], args[1])
		}
	case "charCodeAt":
		if len(args) == 1 {
			return fmt.Sprintf("cl_abap_conv_out_ce=>uccpi( %s+%s(1) )", obj, args[0])
		}
	}

	// this.method() → me->method()
	if obj == "me" {
		method = t.abapMethodName(method)
		if len(args) > 0 {
			var ps []string
			for i, a := range args {
				ps = append(ps, fmt.Sprintf("iv_p%d = %s", i+1, a))
			}
			return fmt.Sprintf("me->%s( %s )", method, strings.Join(ps, " "))
		}
		return fmt.Sprintf("me->%s( )", method)
	}

	// Generic method call
	if len(args) > 0 {
		var ps []string
		for i, a := range args {
			ps = append(ps, fmt.Sprintf("iv_p%d = %s", i+1, a))
		}
		return fmt.Sprintf("%s->%s( %s )", obj, strings.ToLower(method), strings.Join(ps, " "))
	}
	return fmt.Sprintf("%s->%s( )", obj, strings.ToLower(method))
}

func (t *transpiler) abapMethodName(name string) string {
	// Convert camelCase to snake_case
	var result []byte
	for i, c := range name {
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				result = append(result, '_')
			}
			result = append(result, byte(c+32))
		} else {
			result = append(result, byte(c))
		}
	}
	return string(result)
}
