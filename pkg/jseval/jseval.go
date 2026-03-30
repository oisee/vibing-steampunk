// Package jseval implements a minimal JavaScript evaluator in pure Go.
// Supports: numbers, strings, variables, arithmetic, comparisons,
// if/else, while, functions, console.log.
// Designed to be simple enough to compile to ABAP via llvm2abap.
package jseval

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Value represents a JS value.
type Value struct {
	Type   int // 0=undefined, 1=number, 2=string, 3=bool, 4=function, 5=null
	Num    float64
	Str    string
	Fn     *Function
}

var Undefined = Value{Type: 0}
var Null = Value{Type: 5}

func NumberVal(n float64) Value  { return Value{Type: 1, Num: n} }
func StringVal(s string) Value   { return Value{Type: 2, Str: s} }
func BoolVal(b bool) Value       { if b { return Value{Type: 3, Num: 1} }; return Value{Type: 3, Num: 0} }

func (v Value) IsTrue() bool {
	switch v.Type {
	case 0, 5: return false
	case 1: return v.Num != 0
	case 2: return v.Str != ""
	case 3: return v.Num != 0
	}
	return true
}

func (v Value) ToNumber() float64 {
	switch v.Type {
	case 1: return v.Num
	case 2: n, err := strconv.ParseFloat(v.Str, 64); if err == nil { return n }; return 0
	case 3: return v.Num
	}
	return 0
}

func (v Value) ToString() string {
	switch v.Type {
	case 0: return "undefined"
	case 1:
		if v.Num == float64(int64(v.Num)) { return fmt.Sprintf("%d", int64(v.Num)) }
		return fmt.Sprintf("%g", v.Num)
	case 2: return v.Str
	case 3: if v.Num != 0 { return "true" }; return "false"
	case 5: return "null"
	}
	return "undefined"
}

// Node types
const (
	NodeNumber = iota
	NodeString
	NodeIdent
	NodeBinOp
	NodeUnaryOp
	NodeAssign
	NodeVar
	NodeIf
	NodeWhile
	NodeBlock
	NodeCall
	NodeFuncDecl
	NodeReturn
)

// Node is an AST node.
type Node struct {
	Kind     int
	Num      float64
	Str      string
	Op       string
	Left     *Node
	Right    *Node
	Args     []*Node
	Body     []*Node
	Params   []string
	Cond     *Node
	Else     []*Node
}

// Function represents a JS function.
type Function struct {
	Name   string
	Params []string
	Body   []*Node
}

// Env is a variable environment (scope).
type Env struct {
	vars      map[string]Value
	parent    *Env
	output    *strings.Builder // for console.log
	returning bool             // set by return statement
	retVal    Value
}

func NewEnv(parent *Env) *Env {
	e := &Env{vars: make(map[string]Value), parent: parent}
	if parent != nil {
		e.output = parent.output
	} else {
		e.output = &strings.Builder{}
	}
	return e
}

func (e *Env) Get(name string) Value {
	if v, ok := e.vars[name]; ok { return v }
	if e.parent != nil { return e.parent.Get(name) }
	return Undefined
}

func (e *Env) Set(name string, v Value) {
	// Walk up to find existing var
	for env := e; env != nil; env = env.parent {
		if _, ok := env.vars[name]; ok {
			env.vars[name] = v
			return
		}
	}
	e.vars[name] = v
}

func (e *Env) Define(name string, v Value) {
	e.vars[name] = v
}

// Eval evaluates a JS source string and returns the output.
func Eval(source string) (string, error) {
	tokens := tokenize(source)
	parser := &Parser{tokens: tokens, pos: 0}
	stmts := parser.parseProgram()

	env := NewEnv(nil)
	// Built-in: console.log
	env.Define("console", Value{Type: 1}) // placeholder

	for _, stmt := range stmts {
		result := evalNode(stmt, env)
		_ = result
	}

	return env.output.String(), nil
}

func evalNode(n *Node, env *Env) Value {
	if n == nil { return Undefined }
	if env.returning { return env.retVal }

	switch n.Kind {
	case NodeNumber:
		return NumberVal(n.Num)
	case NodeString:
		return StringVal(n.Str)
	case NodeIdent:
		return env.Get(n.Str)

	case NodeBinOp:
		left := evalNode(n.Left, env)
		right := evalNode(n.Right, env)
		return evalBinOp(n.Op, left, right)

	case NodeUnaryOp:
		val := evalNode(n.Left, env)
		switch n.Op {
		case "-": return NumberVal(-val.ToNumber())
		case "!": return BoolVal(!val.IsTrue())
		}

	case NodeAssign:
		val := evalNode(n.Right, env)
		env.Set(n.Str, val)
		return val

	case NodeVar:
		val := Undefined
		if n.Right != nil { val = evalNode(n.Right, env) }
		env.Define(n.Str, val)
		return val

	case NodeIf:
		cond := evalNode(n.Cond, env)
		if cond.IsTrue() {
			for _, s := range n.Body { evalNode(s, env) }
		} else if n.Else != nil {
			for _, s := range n.Else { evalNode(s, env) }
		}

	case NodeWhile:
		for {
			cond := evalNode(n.Cond, env)
			if !cond.IsTrue() || env.returning { break }
			for _, s := range n.Body {
				evalNode(s, env)
				if env.returning { break }
			}
		}

	case NodeBlock:
		var last Value
		for _, s := range n.Body { last = evalNode(s, env) }
		return last

	case NodeCall:
		// console.log special case
		if n.Str == "console.log" {
			var parts []string
			for _, arg := range n.Args {
				parts = append(parts, evalNode(arg, env).ToString())
			}
			env.output.WriteString(strings.Join(parts, " ") + "\n")
			return Undefined
		}
		// User function
		fn := env.Get(n.Str)
		if fn.Type == 4 && fn.Fn != nil {
			callEnv := NewEnv(env)
			for i, param := range fn.Fn.Params {
				if i < len(n.Args) {
					callEnv.Define(param, evalNode(n.Args[i], env))
				}
			}
			var result Value
			for _, s := range fn.Fn.Body {
				result = evalNode(s, callEnv)
				if callEnv.returning {
					result = callEnv.retVal
					break
				}
			}
			return result
		}

	case NodeFuncDecl:
		fn := &Function{Name: n.Str, Params: n.Params, Body: n.Body}
		env.Define(n.Str, Value{Type: 4, Fn: fn})

	case NodeReturn:
		val := evalNode(n.Left, env)
		env.returning = true
		env.retVal = val
		return val
	}

	return Undefined
}

func evalBinOp(op string, l, r Value) Value {
	// String concatenation
	if op == "+" && (l.Type == 2 || r.Type == 2) {
		return StringVal(l.ToString() + r.ToString())
	}
	a, b := l.ToNumber(), r.ToNumber()
	switch op {
	case "+": return NumberVal(a + b)
	case "-": return NumberVal(a - b)
	case "*": return NumberVal(a * b)
	case "/": if b != 0 { return NumberVal(a / b) }; return NumberVal(0)
	case "%": if b != 0 { return NumberVal(float64(int64(a) % int64(b))) }; return NumberVal(0)
	case "==", "===": return BoolVal(a == b)
	case "!=", "!==": return BoolVal(a != b)
	case "<": return BoolVal(a < b)
	case ">": return BoolVal(a > b)
	case "<=": return BoolVal(a <= b)
	case ">=": return BoolVal(a >= b)
	case "&&": return BoolVal(l.IsTrue() && r.IsTrue())
	case "||": return BoolVal(l.IsTrue() || r.IsTrue())
	}
	return Undefined
}

// --- Tokenizer ---

type Token struct {
	Kind int // 0=number, 1=string, 2=ident, 3=op, 4=punc, 5=eof
	Val  string
}

func tokenize(src string) []Token {
	var tokens []Token
	i := 0
	for i < len(src) {
		ch := src[i]
		// Skip whitespace
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			i++; continue
		}
		// Skip comments
		if i+1 < len(src) && ch == '/' && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' { i++ }
			continue
		}
		// Number
		if ch >= '0' && ch <= '9' {
			j := i
			for j < len(src) && (src[j] >= '0' && src[j] <= '9' || src[j] == '.') { j++ }
			tokens = append(tokens, Token{0, src[i:j]})
			i = j; continue
		}
		// String
		if ch == '\'' || ch == '"' {
			j := i + 1
			for j < len(src) && src[j] != ch { j++ }
			tokens = append(tokens, Token{1, src[i+1 : j]})
			i = j + 1; continue
		}
		// Identifier / keyword
		if ch == '_' || unicode.IsLetter(rune(ch)) {
			j := i
			for j < len(src) && (src[j] == '_' || src[j] == '.' || unicode.IsLetter(rune(src[j])) || unicode.IsDigit(rune(src[j]))) { j++ }
			tokens = append(tokens, Token{2, src[i:j]})
			i = j; continue
		}
		// Operators (multi-char)
		if i+1 < len(src) {
			two := src[i : i+2]
			if two == "==" || two == "!=" || two == "<=" || two == ">=" || two == "&&" || two == "||" {
				if i+2 < len(src) && src[i+2] == '=' {
					tokens = append(tokens, Token{3, src[i : i+3]})
					i += 3; continue
				}
				tokens = append(tokens, Token{3, two})
				i += 2; continue
			}
		}
		// Single char op/punc
		if strings.ContainsRune("+-*/%=<>!(),{};", rune(ch)) {
			tokens = append(tokens, Token{3, string(ch)})
			i++; continue
		}
		i++ // skip unknown
	}
	tokens = append(tokens, Token{5, ""})
	return tokens
}

// --- Parser ---

type Parser struct {
	tokens []Token
	pos    int
}

func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) { return Token{5, ""} }
	return p.tokens[p.pos]
}

func (p *Parser) next() Token {
	t := p.peek()
	p.pos++
	return t
}

func (p *Parser) expect(val string) {
	t := p.next()
	_ = t
	// Simple: just consume, don't error
}

func (p *Parser) parseProgram() []*Node {
	var stmts []*Node
	for p.peek().Kind != 5 {
		s := p.parseStatement()
		if s != nil { stmts = append(stmts, s) }
	}
	return stmts
}

func (p *Parser) parseStatement() *Node {
	t := p.peek()

	switch t.Val {
	case "let", "var", "const":
		return p.parseVar()
	case "if":
		return p.parseIf()
	case "while":
		return p.parseWhile()
	case "function":
		return p.parseFunc()
	case "return":
		return p.parseReturn()
	case "{":
		stmts := p.parseBlock()
		return &Node{Kind: NodeBlock, Body: stmts}
	case ";":
		p.next(); return nil
	}

	// Expression statement
	expr := p.parseExpr()
	if p.peek().Val == ";" { p.next() }
	// Check for assignment
	if expr != nil && expr.Kind == NodeIdent && p.peek().Val == "=" {
		// Handled in parseExpr
	}
	return expr
}

func (p *Parser) parseVar() *Node {
	p.next() // skip let/var/const
	name := p.next().Val
	var init *Node
	if p.peek().Val == "=" {
		p.next()
		init = p.parseExpr()
	}
	if p.peek().Val == ";" { p.next() }
	return &Node{Kind: NodeVar, Str: name, Right: init}
}

func (p *Parser) parseIf() *Node {
	p.next() // skip if
	p.expect("(")
	cond := p.parseExpr()
	p.expect(")")
	body := p.parseBody()
	var elsePart []*Node
	if p.peek().Val == "else" {
		p.next()
		if p.peek().Val == "if" {
			elsePart = []*Node{p.parseIf()}
		} else {
			elsePart = p.parseBody()
		}
	}
	return &Node{Kind: NodeIf, Cond: cond, Body: body, Else: elsePart}
}

func (p *Parser) parseWhile() *Node {
	p.next() // skip while
	p.expect("(")
	cond := p.parseExpr()
	p.expect(")")
	body := p.parseBody()
	return &Node{Kind: NodeWhile, Cond: cond, Body: body}
}

func (p *Parser) parseFunc() *Node {
	p.next() // skip function
	name := p.next().Val
	p.expect("(")
	var params []string
	for p.peek().Val != ")" && p.peek().Kind != 5 {
		params = append(params, p.next().Val)
		if p.peek().Val == "," { p.next() }
	}
	p.expect(")")
	body := p.parseBody()
	return &Node{Kind: NodeFuncDecl, Str: name, Params: params, Body: body}
}

func (p *Parser) parseReturn() *Node {
	p.next() // skip return
	var val *Node
	if p.peek().Val != ";" && p.peek().Val != "}" && p.peek().Kind != 5 {
		val = p.parseExpr()
	}
	if p.peek().Val == ";" { p.next() }
	return &Node{Kind: NodeReturn, Left: val}
}

func (p *Parser) parseBlock() []*Node {
	p.expect("{")
	var stmts []*Node
	for p.peek().Val != "}" && p.peek().Kind != 5 {
		s := p.parseStatement()
		if s != nil { stmts = append(stmts, s) }
	}
	p.expect("}")
	return stmts
}

func (p *Parser) parseBody() []*Node {
	if p.peek().Val == "{" {
		return p.parseBlock()
	}
	s := p.parseStatement()
	if s != nil { return []*Node{s} }
	return nil
}

func (p *Parser) parseExpr() *Node {
	return p.parseAssign()
}

func (p *Parser) parseAssign() *Node {
	left := p.parseOr()
	if left != nil && left.Kind == NodeIdent && p.peek().Val == "=" {
		p.next()
		right := p.parseExpr()
		return &Node{Kind: NodeAssign, Str: left.Str, Right: right}
	}
	return left
}

func (p *Parser) parseOr() *Node {
	left := p.parseAnd()
	for p.peek().Val == "||" {
		op := p.next().Val
		right := p.parseAnd()
		left = &Node{Kind: NodeBinOp, Op: op, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseAnd() *Node {
	left := p.parseEquality()
	for p.peek().Val == "&&" {
		op := p.next().Val
		right := p.parseEquality()
		left = &Node{Kind: NodeBinOp, Op: op, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseEquality() *Node {
	left := p.parseComparison()
	for p.peek().Val == "==" || p.peek().Val == "!=" || p.peek().Val == "===" || p.peek().Val == "!==" {
		op := p.next().Val
		right := p.parseComparison()
		left = &Node{Kind: NodeBinOp, Op: op, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseComparison() *Node {
	left := p.parseAddSub()
	for p.peek().Val == "<" || p.peek().Val == ">" || p.peek().Val == "<=" || p.peek().Val == ">=" {
		op := p.next().Val
		right := p.parseAddSub()
		left = &Node{Kind: NodeBinOp, Op: op, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseAddSub() *Node {
	left := p.parseMulDiv()
	for p.peek().Val == "+" || p.peek().Val == "-" {
		op := p.next().Val
		right := p.parseMulDiv()
		left = &Node{Kind: NodeBinOp, Op: op, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseMulDiv() *Node {
	left := p.parseUnary()
	for p.peek().Val == "*" || p.peek().Val == "/" || p.peek().Val == "%" {
		op := p.next().Val
		right := p.parseUnary()
		left = &Node{Kind: NodeBinOp, Op: op, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseUnary() *Node {
	if p.peek().Val == "-" || p.peek().Val == "!" {
		op := p.next().Val
		operand := p.parseUnary()
		return &Node{Kind: NodeUnaryOp, Op: op, Left: operand}
	}
	return p.parsePrimary()
}

func (p *Parser) parsePrimary() *Node {
	t := p.peek()

	switch {
	case t.Kind == 0: // number
		p.next()
		n, _ := strconv.ParseFloat(t.Val, 64)
		return &Node{Kind: NodeNumber, Num: n}

	case t.Kind == 1: // string
		p.next()
		return &Node{Kind: NodeString, Str: t.Val}

	case t.Kind == 2: // identifier
		p.next()
		name := t.Val
		// Check for function call
		if p.peek().Val == "(" {
			p.next()
			var args []*Node
			for p.peek().Val != ")" && p.peek().Kind != 5 {
				args = append(args, p.parseExpr())
				if p.peek().Val == "," { p.next() }
			}
			p.expect(")")
			return &Node{Kind: NodeCall, Str: name, Args: args}
		}
		return &Node{Kind: NodeIdent, Str: name}

	case t.Val == "(":
		p.next()
		expr := p.parseExpr()
		p.expect(")")
		return expr

	case t.Val == "true":
		p.next()
		return &Node{Kind: NodeNumber, Num: 1}
	case t.Val == "false":
		p.next()
		return &Node{Kind: NodeNumber, Num: 0}
	}

	p.next() // skip unknown
	return nil
}
