// Package jseval implements a minimal JavaScript evaluator in pure Go.
// Supports: numbers, strings, variables, arithmetic, comparisons,
// if/else, while, for, functions, closures, objects, arrays, classes,
// switch, typeof, console.log.
// Designed to be simple enough to compile to ABAP via llvm2abap.
package jseval

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// Value represents a JS value.
type Value struct {
	Type   int // 0=undefined, 1=number, 2=string, 3=bool, 4=function, 5=null, 6=object, 7=array
	Num    float64
	Str    string
	Fn     *Function
	Obj    map[string]Value
	Arr    *[]Value // pointer so mutations are shared
}

var Undefined = Value{Type: 0}
var Null = Value{Type: 5}

func NumberVal(n float64) Value  { return Value{Type: 1, Num: n} }
func StringVal(s string) Value   { return Value{Type: 2, Str: s} }
func BoolVal(b bool) Value       { if b { return Value{Type: 3, Num: 1} }; return Value{Type: 3, Num: 0} }
func ObjectVal() Value           { return Value{Type: 6, Obj: make(map[string]Value)} }
func ArrayVal(elems []Value) Value {
	a := make([]Value, len(elems))
	copy(a, elems)
	return Value{Type: 7, Arr: &a}
}

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
	case 6: return "[object Object]"
	case 7: return fmt.Sprintf("[array %d]", len(*v.Arr))
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
	NodeObject       // {key: val, ...}
	NodeArray        // [expr, ...]
	NodeMemberAccess // obj.prop or obj[expr]
	NodeMemberAssign // obj.prop = val or obj[expr] = val
	NodeMethodCall   // obj.method(args)
	NodeFor          // for (init; cond; update) body
	NodeSwitch       // switch(expr) { case ... }
	NodeTypeof       // typeof expr
	NodeNew          // new Ctor(args)
	NodeClass        // class Name { ... }
	NodeBreak        // break
	NodeContinue     // continue
	NodeBool         // true/false literal
	NodeTernary      // cond ? then : else
	NodeThrow        // throw expr
	NodeTryCatch     // try { ... } catch(e) { ... }
	NodeForOf        // for (let x of arr) { ... }
	NodeTemplateLit  // `text ${expr} text`
	NodeSpread       // ...expr
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
	// For loop
	Init     *Node
	Update   *Node
	// Member access
	Object   *Node   // the object expression
	Property string  // static property name
	PropExpr *Node   // dynamic property expression (bracket access)
	// Switch
	Cases    []SwitchCase
	// Class
	Methods  []ClassMethod
	// Try/catch
	Catch    []*Node  // catch body
	CatchVar string   // catch variable name
}

type SwitchCase struct {
	Expr *Node   // nil = default
	Body []*Node
}

type ClassMethod struct {
	Name     string
	Params   []string
	Body     []*Node
	IsCtor   bool
	IsStatic bool
}

// Function represents a JS function.
type Function struct {
	Name    string
	Params  []string
	Body    []*Node
	Closure *Env // captured environment for closures
}

// Env is a variable environment (scope).
type Env struct {
	vars      map[string]Value
	parent    *Env
	output    *strings.Builder // for console.log
	returning  bool
	retVal     Value
	breaking   bool
	continuing bool
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
	// Built-in: Error class
	env.Define("undefined", Undefined)
	env.Define("null", Null)
	env.Define("Infinity", NumberVal(math.Inf(1)))
	env.Define("NaN", NumberVal(math.NaN()))

	for _, stmt := range stmts {
		result := evalNode(stmt, env)
		_ = result
	}

	return env.output.String(), nil
}

func callFunction(fn *Function, args []Value, env *Env, thisVal *Value) Value {
	// Use closure environment if available, otherwise caller env
	parentEnv := env
	if fn.Closure != nil {
		parentEnv = fn.Closure
	}
	callEnv := NewEnv(parentEnv)
	// Propagate output from caller
	callEnv.output = env.output
	if thisVal != nil {
		callEnv.Define("this", *thisVal)
	}
	for i, param := range fn.Params {
		if strings.HasPrefix(param, "...") {
			// Rest parameter: collect remaining args into array
			restName := param[3:]
			rest := make([]Value, 0)
			for j := i; j < len(args); j++ {
				rest = append(rest, args[j])
			}
			callEnv.Define(restName, ArrayVal(rest))
			break
		}
		if i < len(args) {
			callEnv.Define(param, args[i])
		}
	}
	var result Value
	for _, s := range fn.Body {
		result = evalNode(s, callEnv)
		if callEnv.returning {
			result = callEnv.retVal
			break
		}
	}
	// Write back this if it was an object (for mutation)
	if thisVal != nil && thisVal.Type == 6 {
		updated := callEnv.Get("this")
		if updated.Type == 6 {
			// Copy properties back
			for k, v := range updated.Obj {
				thisVal.Obj[k] = v
			}
		}
	}
	return result
}

func evalNode(n *Node, env *Env) Value {
	if n == nil { return Undefined }
	if env.returning || env.breaking { return Undefined }

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

	case NodeBool:
		return BoolVal(n.Num != 0)

	case NodeTernary:
		if evalNode(n.Cond, env).IsTrue() {
			return evalNode(n.Left, env)
		}
		return evalNode(n.Right, env)

	case NodeThrow:
		val := evalNode(n.Left, env)
		panic(val)

	case NodeTryCatch:
		func() {
			defer func() {
				if r := recover(); r != nil {
					if n.Catch != nil {
						catchEnv := NewEnv(env)
						if n.CatchVar != "" {
							if v, ok := r.(Value); ok {
								catchEnv.Define(n.CatchVar, v)
							} else {
								catchEnv.Define(n.CatchVar, StringVal(fmt.Sprintf("%v", r)))
							}
						}
						for _, s := range n.Catch {
							evalNode(s, catchEnv)
							if catchEnv.returning || catchEnv.breaking { break }
						}
						if catchEnv.returning {
							env.returning = true
							env.retVal = catchEnv.retVal
						}
					}
				}
			}()
			for _, s := range n.Body {
				evalNode(s, env)
				if env.returning || env.breaking { break }
			}
		}()

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

	case NodeMemberAssign:
		obj := evalNode(n.Object, env)
		val := evalNode(n.Right, env)
		prop := n.Property
		if n.PropExpr != nil {
			prop = evalNode(n.PropExpr, env).ToString()
		}
		if obj.Type == 6 {
			obj.Obj[prop] = val
		}
		return val

	case NodeVar:
		val := Undefined
		if n.Right != nil { val = evalNode(n.Right, env) }
		env.Define(n.Str, val)
		return val

	case NodeIf:
		cond := evalNode(n.Cond, env)
		if cond.IsTrue() {
			for _, s := range n.Body {
				evalNode(s, env)
				if env.returning || env.breaking { break }
			}
		} else if n.Else != nil {
			for _, s := range n.Else {
				evalNode(s, env)
				if env.returning || env.breaking { break }
			}
		}

	case NodeWhile:
		for {
			cond := evalNode(n.Cond, env)
			if !cond.IsTrue() || env.returning || env.breaking { break }
			for _, s := range n.Body {
				evalNode(s, env)
				if env.returning || env.breaking || env.continuing { break }
			}
			if env.continuing { env.continuing = false; continue }
		}
		if env.breaking { env.breaking = false }

	case NodeFor:
		// Init
		forEnv := NewEnv(env)
		if n.Init != nil { evalNode(n.Init, forEnv) }
		for {
			if forEnv.returning || forEnv.breaking { break }
			cond := evalNode(n.Cond, forEnv)
			if !cond.IsTrue() { break }
			for _, s := range n.Body {
				evalNode(s, forEnv)
				if forEnv.returning || forEnv.breaking || forEnv.continuing { break }
			}
			if forEnv.continuing { forEnv.continuing = false }
			if forEnv.returning || forEnv.breaking { break }
			if n.Update != nil { evalNode(n.Update, forEnv) }
		}
		if forEnv.returning {
			env.returning = true
			env.retVal = forEnv.retVal
		}
		if forEnv.breaking { /* consumed */ }

	case NodeForOf:
		iter := evalNode(n.Right, env)
		forEnv := NewEnv(env)
		forEnv.output = env.output
		if iter.Type == 7 && iter.Arr != nil { // array
			for _, elem := range *iter.Arr {
				forEnv.Define(n.Str, elem)
				for _, s := range n.Body {
					evalNode(s, forEnv)
					if forEnv.returning || forEnv.breaking || forEnv.continuing { break }
				}
				if forEnv.continuing { forEnv.continuing = false; continue }
				if forEnv.returning || forEnv.breaking { break }
			}
		} else if iter.Type == 6 && iter.Obj != nil { // object — for...in
			for k := range iter.Obj {
				forEnv.Define(n.Str, StringVal(k))
				for _, s := range n.Body {
					evalNode(s, forEnv)
					if forEnv.returning || forEnv.breaking || forEnv.continuing { break }
				}
				if forEnv.continuing { forEnv.continuing = false; continue }
				if forEnv.returning || forEnv.breaking { break }
			}
		}
		if forEnv.returning {
			env.returning = true
			env.retVal = forEnv.retVal
		}

	case NodeBlock:
		var last Value
		for _, s := range n.Body {
			last = evalNode(s, env)
			if env.returning || env.breaking { break }
		}
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
			var args []Value
			for _, a := range n.Args {
				args = append(args, evalNode(a, env))
			}
			return callFunction(fn.Fn, args, env, nil)
		}

	case NodeMethodCall:
		obj := evalNode(n.Object, env)
		method := n.Property
		var args []Value
		for _, a := range n.Args {
			args = append(args, evalNode(a, env))
		}
		// Direct function call: expr(args) — for arrow/closure results
		if method == "__call__" && obj.Type == 4 && obj.Fn != nil {
			return callFunction(obj.Fn, args, env, nil)
		}
		return evalMethodCall(obj, method, args, env, n.Object)

	case NodeFuncDecl:
		fn := &Function{Name: n.Str, Params: n.Params, Body: n.Body, Closure: env}
		fnVal := Value{Type: 4, Fn: fn}
		if n.Str != "" {
			env.Define(n.Str, fnVal)
		}
		return fnVal

	case NodeReturn:
		val := evalNode(n.Left, env)
		env.returning = true
		env.retVal = val
		return val

	case NodeBreak:
		env.breaking = true
		return Undefined

	case NodeContinue:
		env.continuing = true
		return Undefined

	case NodeObject:
		obj := ObjectVal()
		for i := 0; i < len(n.Args); i += 2 {
			key := n.Args[i].Str
			val := evalNode(n.Args[i+1], env)
			obj.Obj[key] = val
		}
		return obj

	case NodeArray:
		var elems []Value
		for _, a := range n.Args {
			if a.Kind == NodeSpread {
				spread := evalNode(a.Left, env)
				if spread.Type == 7 && spread.Arr != nil {
					elems = append(elems, *spread.Arr...)
				}
			} else {
				elems = append(elems, evalNode(a, env))
			}
		}
		return ArrayVal(elems)

	case NodeMemberAccess:
		obj := evalNode(n.Object, env)
		if obj.Type == 0 || obj.Type == 5 { return Undefined } // null/undefined?.prop
		prop := n.Property
		if n.PropExpr != nil {
			prop = evalNode(n.PropExpr, env).ToString()
		}
		return evalPropertyAccess(obj, prop)

	case NodeTypeof:
		val := evalNode(n.Left, env)
		switch val.Type {
		case 0: return StringVal("undefined")
		case 1: return StringVal("number")
		case 2: return StringVal("string")
		case 3: return StringVal("boolean")
		case 4: return StringVal("function")
		case 5: return StringVal("object")
		case 6: return StringVal("object")
		case 7: return StringVal("object")
		}
		return StringVal("undefined")

	case NodeNew:
		// Built-in: new Error(msg)
		if n.Str == "Error" || n.Str == "TypeError" || n.Str == "RangeError" {
			var args []Value
			for _, a := range n.Args { args = append(args, evalNode(a, env)) }
			errObj := ObjectVal()
			msg := ""
			if len(args) > 0 { msg = args[0].ToString() }
			errObj.Obj["message"] = StringVal(msg)
			errObj.Obj["name"] = StringVal(n.Str)
			return errObj
		}
		// Resolve constructor: evaluate the expression (handles a.b.c)
		cls := evalNode(n.Left, env)
		// Function-as-constructor: new Func(args)
		if cls.Type == 4 && cls.Fn != nil {
			instance := ObjectVal()
			var args []Value
			for _, a := range n.Args { args = append(args, evalNode(a, env)) }
			result := callFunction(cls.Fn, args, env, &instance)
			// JS: if constructor returns an object, use it instead of this
			if result.Type == 6 {
				return result
			}
			return instance
		}
		if cls.Type == 6 && cls.Obj != nil {
			// Class prototype - create instance
			instance := ObjectVal()
			var args []Value
			for _, a := range n.Args {
				args = append(args, evalNode(a, env))
			}
			// Call constructor if exists
			if ctor, ok := cls.Obj["constructor"]; ok && ctor.Type == 4 && ctor.Fn != nil {
				callFunction(ctor.Fn, args, env, &instance)
			}
			// Copy methods to instance
			for k, v := range cls.Obj {
				if k != "constructor" {
					instance.Obj[k] = v
				}
			}
			return instance
		}

	case NodeClass:
		cls := ObjectVal()
		// Inherit from parent class
		if n.Property != "" {
			parent := env.Get(n.Property)
			if parent.Type == 6 {
				for k, v := range parent.Obj {
					cls.Obj[k] = v
				}
			}
		}
		for _, m := range n.Methods {
			fn := &Function{Name: m.Name, Params: m.Params, Body: m.Body, Closure: env}
			cls.Obj[m.Name] = Value{Type: 4, Fn: fn}
		}
		env.Define(n.Str, cls)

	case NodeSwitch:
		val := evalNode(n.Cond, env)
		matched := false
		for _, c := range n.Cases {
			if !matched && c.Expr != nil {
				caseVal := evalNode(c.Expr, env)
				if val.ToNumber() == caseVal.ToNumber() && val.Type == caseVal.Type {
					matched = true
				}
			}
			if !matched && c.Expr == nil {
				matched = true // default
			}
			if matched {
				for _, s := range c.Body {
					evalNode(s, env)
					if env.breaking || env.returning { break }
				}
				if env.breaking {
					env.breaking = false
					break
				}
				if env.returning { break }
			}
		}
	}

	return Undefined
}

func evalPropertyAccess(obj Value, prop string) Value {
	switch obj.Type {
	case 6: // object
		if v, ok := obj.Obj[prop]; ok { return v }
		return Undefined
	case 7: // array
		if prop == "length" {
			return NumberVal(float64(len(*obj.Arr)))
		}
		// Numeric index
		idx, err := strconv.Atoi(prop)
		if err == nil && idx >= 0 && idx < len(*obj.Arr) {
			return (*obj.Arr)[idx]
		}
		return Undefined
	case 2: // string
		if prop == "length" {
			return NumberVal(float64(len(obj.Str)))
		}
		return Undefined
	}
	return Undefined
}

func evalMethodCall(obj Value, method string, args []Value, env *Env, objNode *Node) Value {
	switch obj.Type {
	case 6: // object - look up method
		if fn, ok := obj.Obj[method]; ok && fn.Type == 4 && fn.Fn != nil {
			return callFunction(fn.Fn, args, env, &obj)
		}
	case 7: // array
		switch method {
		case "push":
			if len(args) > 0 {
				*obj.Arr = append(*obj.Arr, args[0])
				return NumberVal(float64(len(*obj.Arr)))
			}
		}
	case 2: // string
		switch method {
		case "charAt":
			if len(args) > 0 {
				idx := int(args[0].ToNumber())
				if idx >= 0 && idx < len(obj.Str) {
					return StringVal(string(obj.Str[idx]))
				}
			}
			return StringVal("")
		case "indexOf":
			if len(args) > 0 {
				idx := strings.Index(obj.Str, args[0].ToString())
				return NumberVal(float64(idx))
			}
			return NumberVal(-1)
		case "substring":
			if len(args) >= 2 {
				start := int(args[0].ToNumber())
				end := int(args[1].ToNumber())
				if start < 0 { start = 0 }
				if start > len(obj.Str) { start = len(obj.Str) }
				if end < 0 { end = 0 }
				if end > len(obj.Str) { end = len(obj.Str) }
				if start > end { start, end = end, start }
				return StringVal(obj.Str[start:end])
			}
		case "charCodeAt":
			if len(args) > 0 {
				idx := int(args[0].ToNumber())
				if idx >= 0 && idx < len(obj.Str) {
					return NumberVal(float64(obj.Str[idx]))
				}
			}
			return NumberVal(0)
		}
	}
	return Undefined
}

func evalBinOp(op string, l, r Value) Value {
	// String concatenation
	if op == "+" && (l.Type == 2 || r.Type == 2) {
		return StringVal(l.ToString() + r.ToString())
	}
	// Equality — compare by type+value, not just numeric
	switch op {
	case "==", "===":
		if l.Type != r.Type { return BoolVal(false) }
		if l.Type == 2 { return BoolVal(l.Str == r.Str) }
		return BoolVal(l.ToNumber() == r.ToNumber())
	case "!=", "!==":
		if l.Type != r.Type { return BoolVal(true) }
		if l.Type == 2 { return BoolVal(l.Str != r.Str) }
		return BoolVal(l.ToNumber() != r.ToNumber())
	}
	a, b := l.ToNumber(), r.ToNumber()
	switch op {
	case "+": return NumberVal(a + b)
	case "-": return NumberVal(a - b)
	case "*": return NumberVal(a * b)
	case "/": if b != 0 { return NumberVal(a / b) }; return NumberVal(0)
	case "%": if b != 0 { return NumberVal(float64(int64(a) % int64(b))) }; return NumberVal(0)
	case "<": return BoolVal(a < b)
	case ">": return BoolVal(a > b)
	case "<=": return BoolVal(a <= b)
	case ">=": return BoolVal(a >= b)
	case "&&": return BoolVal(l.IsTrue() && r.IsTrue())
	case "||": if l.IsTrue() { return l }; return r
	case "??": if l.Type != 0 && l.Type != 5 { return l }; return r
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
			var sb strings.Builder
			j := i + 1
			for j < len(src) && src[j] != ch {
				if src[j] == '\\' && j+1 < len(src) {
					j++
					switch src[j] {
					case 'n': sb.WriteByte('\n')
					case 't': sb.WriteByte('\t')
					case '\\': sb.WriteByte('\\')
					case '\'': sb.WriteByte('\'')
					case '"': sb.WriteByte('"')
					default: sb.WriteByte(src[j])
					}
				} else {
					sb.WriteByte(src[j])
				}
				j++
			}
			tokens = append(tokens, Token{1, sb.String()})
			i = j + 1; continue
		}
		// Template literal: `text${expr}text`
		if ch == '`' {
			i++ // skip opening `
			// Desugar to: ("" + "text" + (expr) + "text")
			tokens = append(tokens, Token{3, "("})
			tokens = append(tokens, Token{1, ""}) // start with empty string for type coercion
			for i < len(src) && src[i] != '`' {
				if i+1 < len(src) && src[i] == '$' && src[i+1] == '{' {
					// End current text part, start expression
					i += 2 // skip ${
					tokens = append(tokens, Token{3, "+"})
					tokens = append(tokens, Token{3, "("})
					// Tokenize the expression inside ${}
					depth := 1
					exprStart := i
					for i < len(src) && depth > 0 {
						if src[i] == '{' { depth++ }
						if src[i] == '}' { depth-- }
						if depth > 0 { i++ }
					}
					// Recursively tokenize the inner expression
					innerTokens := tokenize(src[exprStart:i])
					// Remove EOF token from inner
					if len(innerTokens) > 0 && innerTokens[len(innerTokens)-1].Kind == 5 {
						innerTokens = innerTokens[:len(innerTokens)-1]
					}
					tokens = append(tokens, innerTokens...)
					tokens = append(tokens, Token{3, ")"})
					i++ // skip closing }
				} else {
					// Regular text — collect until ` or ${
					var tb strings.Builder
					for i < len(src) && src[i] != '`' && !(i+1 < len(src) && src[i] == '$' && src[i+1] == '{') {
						if src[i] == '\\' && i+1 < len(src) {
							i++
							switch src[i] {
							case 'n': tb.WriteByte('\n')
							case 't': tb.WriteByte('\t')
							case '\\': tb.WriteByte('\\')
							case '`': tb.WriteByte('`')
							default: tb.WriteByte(src[i])
							}
						} else {
							tb.WriteByte(src[i])
						}
						i++
					}
					tokens = append(tokens, Token{3, "+"})
					tokens = append(tokens, Token{1, tb.String()})
				}
			}
			if i < len(src) { i++ } // skip closing `
			tokens = append(tokens, Token{3, ")"})
			continue
		}
		// Identifier / keyword (no longer include '.' in identifiers)
		if ch == '_' || unicode.IsLetter(rune(ch)) {
			j := i
			for j < len(src) && (src[j] == '_' || unicode.IsLetter(rune(src[j])) || unicode.IsDigit(rune(src[j]))) { j++ }
			tokens = append(tokens, Token{2, src[i:j]})
			i = j; continue
		}
		// Spread/rest: ...
		if i+2 < len(src) && src[i] == '.' && src[i+1] == '.' && src[i+2] == '.' {
			tokens = append(tokens, Token{3, "..."})
			i += 3; continue
		}
		// Operators (multi-char)
		if i+1 < len(src) {
			two := src[i : i+2]
			if two == "==" || two == "!=" || two == "<=" || two == ">=" || two == "&&" || two == "||" || two == "=>" || two == "?." || two == "??" {
				if i+2 < len(src) && src[i+2] == '=' {
					tokens = append(tokens, Token{3, src[i : i+3]})
					i += 3; continue
				}
				tokens = append(tokens, Token{3, two})
				i += 2; continue
			}
		}
		// Single char op/punc
		if strings.ContainsRune("+-*/%=<>!(),{};:.[]?", rune(ch)) {
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
	p.next()
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
	case "for":
		return p.parseFor()
	case "function":
		return p.parseFunc()
	case "return":
		return p.parseReturn()
	case "break":
		p.next()
		if p.peek().Val == ";" { p.next() }
		return &Node{Kind: NodeBreak}
	case "continue":
		p.next()
		if p.peek().Val == ";" { p.next() }
		return &Node{Kind: NodeContinue}
	case "throw":
		p.next()
		expr := p.parseExpr()
		if p.peek().Val == ";" { p.next() }
		return &Node{Kind: NodeThrow, Left: expr}
	case "try":
		return p.parseTryCatch()
	case "switch":
		return p.parseSwitch()
	case "class":
		return p.parseClass()
	case "{":
		stmts := p.parseBlock()
		return &Node{Kind: NodeBlock, Body: stmts}
	case ";":
		p.next(); return nil
	}

	// Expression statement
	expr := p.parseExpr()
	if p.peek().Val == ";" { p.next() }
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

func (p *Parser) parseFor() *Node {
	p.next() // skip for
	p.expect("(")
	// Check for for...of / for...in: for (let/const/var x of/in expr)
	if p.peek().Val == "let" || p.peek().Val == "var" || p.peek().Val == "const" {
		saved := p.pos
		p.next() // skip let/var/const
		varName := p.next().Val
		if p.peek().Val == "of" || p.peek().Val == "in" {
			p.next() // skip of/in
			iter := p.parseExpr()
			p.expect(")")
			body := p.parseBody()
			return &Node{Kind: NodeForOf, Str: varName, Right: iter, Body: body}
		}
		// Not for-of, restore and parse as regular for
		p.pos = saved
	}
	// Regular for(init; cond; update)
	var init *Node
	if p.peek().Val == "let" || p.peek().Val == "var" || p.peek().Val == "const" {
		init = p.parseVar()
	} else if p.peek().Val != ";" {
		init = p.parseExpr()
		if p.peek().Val == ";" { p.next() }
	} else {
		p.next() // skip ;
	}
	var cond *Node
	if p.peek().Val != ";" {
		cond = p.parseExpr()
	}
	if p.peek().Val == ";" { p.next() }
	var update *Node
	if p.peek().Val != ")" {
		update = p.parseExpr()
	}
	p.expect(")")
	body := p.parseBody()
	return &Node{Kind: NodeFor, Init: init, Cond: cond, Update: update, Body: body}
}

func (p *Parser) parseTryCatch() *Node {
	p.next() // consume "try"
	body := p.parseBlock()
	var catchBody []*Node
	var catchVar string
	if p.peek().Val == "catch" {
		p.next() // consume "catch"
		if p.peek().Val == "(" {
			p.next() // consume "("
			catchVar = p.next().Val
			p.expect(")")
		}
		catchBody = p.parseBlock()
	}
	// skip optional "finally" block
	if p.peek().Val == "finally" {
		p.next()
		p.parseBlock() // parse but ignore for now
	}
	return &Node{Kind: NodeTryCatch, Body: body, Catch: catchBody, CatchVar: catchVar}
}

func (p *Parser) parseSwitch() *Node {
	p.next() // skip switch
	p.expect("(")
	expr := p.parseExpr()
	p.expect(")")
	p.expect("{")
	var cases []SwitchCase
	for p.peek().Val != "}" && p.peek().Kind != 5 {
		if p.peek().Val == "case" {
			p.next()
			caseExpr := p.parseExpr()
			p.expect(":") // colon
			var body []*Node
			for p.peek().Val != "case" && p.peek().Val != "default" && p.peek().Val != "}" && p.peek().Kind != 5 {
				s := p.parseStatement()
				if s != nil { body = append(body, s) }
			}
			cases = append(cases, SwitchCase{Expr: caseExpr, Body: body})
		} else if p.peek().Val == "default" {
			p.next()
			p.expect(":")
			var body []*Node
			for p.peek().Val != "case" && p.peek().Val != "}" && p.peek().Kind != 5 {
				s := p.parseStatement()
				if s != nil { body = append(body, s) }
			}
			cases = append(cases, SwitchCase{Expr: nil, Body: body})
		} else {
			p.next() // skip unknown
		}
	}
	p.expect("}")
	return &Node{Kind: NodeSwitch, Cond: expr, Cases: cases}
}

func (p *Parser) parseClass() *Node {
	p.next() // skip class
	name := p.next().Val
	parentClass := ""
	if p.peek().Val == "extends" {
		p.next() // skip extends
		parentClass = p.next().Val
	}
	p.expect("{")
	var methods []ClassMethod
	for p.peek().Val != "}" && p.peek().Kind != 5 {
		isStatic := false
		if p.peek().Val == "static" {
			isStatic = true
			p.next()
		}
		mName := p.next().Val
		p.expect("(")
		var params []string
		for p.peek().Val != ")" && p.peek().Kind != 5 {
			if p.peek().Val == "..." { p.next(); params = append(params, "..." + p.next().Val) } else { params = append(params, p.next().Val) }
			if p.peek().Val == "," { p.next() }
		}
		p.expect(")")
		body := p.parseBlock()
		methods = append(methods, ClassMethod{
			Name:   mName,
			Params: params,
			Body:   body,
			IsCtor: mName == "constructor",
			IsStatic: isStatic,
		})
	}
	p.expect("}")
	n := &Node{Kind: NodeClass, Str: name, Methods: methods}
	n.Property = parentClass // reuse Property field for extends
	return n
}

func (p *Parser) parseFunc() *Node {
	p.next() // skip function
	name := p.next().Val
	p.expect("(")
	var params []string
	for p.peek().Val != ")" && p.peek().Kind != 5 {
		if p.peek().Val == "..." { p.next(); params = append(params, "..." + p.next().Val) } else { params = append(params, p.next().Val) }
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
	if p.peek().Val == "=" {
		if left != nil && left.Kind == NodeIdent {
			p.next()
			right := p.parseExpr()
			return &Node{Kind: NodeAssign, Str: left.Str, Right: right}
		}
		if left != nil && left.Kind == NodeMemberAccess {
			p.next()
			right := p.parseExpr()
			return &Node{Kind: NodeMemberAssign, Object: left.Object, Property: left.Property, PropExpr: left.PropExpr, Right: right}
		}
	}
	// Arrow function: ident => ...
	if left != nil && left.Kind == NodeIdent && p.peek().Val == "=>" {
		p.next() // consume =>
		var body []*Node
		if p.peek().Val == "{" {
			body = p.parseBlock()
		} else {
			expr := p.parseAssign()
			body = []*Node{{Kind: NodeReturn, Left: expr}}
		}
		return &Node{Kind: NodeFuncDecl, Str: "", Params: []string{left.Str}, Body: body}
	}
	// Ternary: cond ? then : else
	if p.peek().Kind == 3 && p.peek().Val == "?" {
		p.next() // consume ?
		then := p.parseAssign()
		p.expect(":") // consume :
		els := p.parseAssign()
		return &Node{Kind: NodeTernary, Cond: left, Left: then, Right: els}
	}
	return left
}

func (p *Parser) parseOr() *Node {
	left := p.parseAnd()
	for p.peek().Kind == 3 && (p.peek().Val == "||" || p.peek().Val == "??") {
		op := p.next().Val
		right := p.parseAnd()
		left = &Node{Kind: NodeBinOp, Op: op, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseAnd() *Node {
	left := p.parseEquality()
	for p.peek().Kind == 3 && p.peek().Val == "&&" {
		op := p.next().Val
		right := p.parseEquality()
		left = &Node{Kind: NodeBinOp, Op: op, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseEquality() *Node {
	left := p.parseComparison()
	for p.peek().Kind == 3 && (p.peek().Val == "==" || p.peek().Val == "!=" || p.peek().Val == "===" || p.peek().Val == "!==") {
		op := p.next().Val
		right := p.parseComparison()
		left = &Node{Kind: NodeBinOp, Op: op, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseComparison() *Node {
	left := p.parseAddSub()
	for p.peek().Kind == 3 && (p.peek().Val == "<" || p.peek().Val == ">" || p.peek().Val == "<=" || p.peek().Val == ">=") {
		op := p.next().Val
		right := p.parseAddSub()
		left = &Node{Kind: NodeBinOp, Op: op, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseAddSub() *Node {
	left := p.parseMulDiv()
	for p.peek().Kind == 3 && (p.peek().Val == "+" || p.peek().Val == "-") {
		op := p.next().Val
		right := p.parseMulDiv()
		left = &Node{Kind: NodeBinOp, Op: op, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseMulDiv() *Node {
	left := p.parseUnary()
	for p.peek().Kind == 3 && (p.peek().Val == "*" || p.peek().Val == "/" || p.peek().Val == "%") {
		op := p.next().Val
		right := p.parseUnary()
		left = &Node{Kind: NodeBinOp, Op: op, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseUnary() *Node {
	if p.peek().Kind == 3 && (p.peek().Val == "-" || p.peek().Val == "!") {
		op := p.next().Val
		operand := p.parseUnary()
		return &Node{Kind: NodeUnaryOp, Op: op, Left: operand}
	}
	if p.peek().Val == "typeof" {
		p.next()
		operand := p.parseUnary()
		return &Node{Kind: NodeTypeof, Left: operand}
	}
	if p.peek().Val == "new" {
		p.next()
		// Parse constructor expression: could be Name, a.b.c, etc.
		// Collect dotted name chain
		name := p.next().Val
		var ctorExpr *Node
		ctorExpr = &Node{Kind: NodeIdent, Str: name}
		for p.peek().Val == "." {
			p.next() // consume .
			prop := p.next().Val
			ctorExpr = &Node{Kind: NodeMemberAccess, Object: ctorExpr, Property: prop}
		}
		p.expect("(")
		var args []*Node
		for p.peek().Val != ")" && p.peek().Kind != 5 {
			args = append(args, p.parseExpr())
			if p.peek().Val == "," { p.next() }
		}
		p.expect(")")
		// Store ctor expression in Left, keep Str for simple case
		return &Node{Kind: NodeNew, Str: name, Left: ctorExpr, Args: args}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() *Node {
	left := p.parsePrimary()
	for {
		if p.peek().Val == "." || p.peek().Val == "?." {
			p.next() // consume .
			prop := p.next().Val
			// Check if method call: obj.method(...)
			if p.peek().Val == "(" {
				p.next() // consume (
				var args []*Node
				for p.peek().Val != ")" && p.peek().Kind != 5 {
					args = append(args, p.parseExpr())
					if p.peek().Val == "," { p.next() }
				}
				p.expect(")")
				left = &Node{Kind: NodeMethodCall, Object: left, Property: prop, Args: args}
			} else {
				left = &Node{Kind: NodeMemberAccess, Object: left, Property: prop}
			}
		} else if p.peek().Val == "[" {
			p.next() // consume [
			idx := p.parseExpr()
			p.expect("]")
			left = &Node{Kind: NodeMemberAccess, Object: left, PropExpr: idx}
		} else if p.peek().Val == "(" && left != nil {
			if left.Kind == NodeIdent {
				// named function call: name(args)
				p.next()
				var args []*Node
				for p.peek().Val != ")" && p.peek().Kind != 5 {
					args = append(args, p.parseExpr())
					if p.peek().Val == "," { p.next() }
				}
				p.expect(")")
				left = &Node{Kind: NodeCall, Str: left.Str, Args: args}
			} else {
				// expression call: expr(args) — for arrow results, IIFE, etc.
				p.next()
				var args []*Node
				for p.peek().Val != ")" && p.peek().Kind != 5 {
					args = append(args, p.parseExpr())
					if p.peek().Val == "," { p.next() }
				}
				p.expect(")")
				left = &Node{Kind: NodeMethodCall, Object: left, Property: "__call__", Args: args}
			}
		} else {
			break
		}
	}
	return left
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

	case t.Val == "function":
		// Anonymous function expression: function(params) { body }
		p.next() // consume "function"
		name := ""
		if p.peek().Kind == 2 && p.peek().Val != "(" {
			name = p.next().Val // optional name
		}
		p.expect("(")
		var params []string
		for p.peek().Val != ")" && p.peek().Kind != 5 {
			if p.peek().Val == "..." { p.next(); params = append(params, "..." + p.next().Val) } else { params = append(params, p.next().Val) }
			if p.peek().Val == "," { p.next() }
		}
		p.expect(")")
		body := p.parseBlock()
		return &Node{Kind: NodeFuncDecl, Str: name, Params: params, Body: body}

	case t.Val == "(":
		// Try arrow function: (params) => ...
		saved := p.pos
		p.next() // consume (
		var arrowParams []string
		isArrow := true
		for p.peek().Val != ")" && p.peek().Kind != 5 {
			if p.peek().Kind != 2 { // not an identifier — not arrow params
				isArrow = false
				break
			}
			arrowParams = append(arrowParams, p.next().Val)
			if p.peek().Val == "," { p.next() }
		}
		if isArrow && p.peek().Val == ")" {
			p.next() // consume )
			if p.peek().Val == "=>" {
				p.next() // consume =>
				var body []*Node
				if p.peek().Val == "{" {
					body = p.parseBlock()
				} else {
					expr := p.parseAssign()
					body = []*Node{{Kind: NodeReturn, Left: expr}}
				}
				return &Node{Kind: NodeFuncDecl, Str: "", Params: arrowParams, Body: body}
			}
		}
		// Not arrow — restore and parse as grouping
		p.pos = saved
		p.next() // consume (
		expr := p.parseExpr()
		p.expect(")")
		return expr

	case t.Val == "[":
		return p.parseArrayLiteral()

	case t.Val == "{":
		return p.parseObjectLiteral()

	case t.Val == "true":
		p.next()
		return &Node{Kind: NodeBool, Num: 1}
	case t.Val == "false":
		p.next()
		return &Node{Kind: NodeBool, Num: 0}

	case t.Kind == 2: // identifier
		p.next()
		name := t.Val
		// console.log handled specially - check if "console" followed by ".log"
		if name == "console" && p.peek().Val == "." {
			p.next() // consume .
			sub := p.next().Val // "log"
			fullName := name + "." + sub
			if p.peek().Val == "(" {
				p.next()
				var args []*Node
				for p.peek().Val != ")" && p.peek().Kind != 5 {
					args = append(args, p.parseExpr())
					if p.peek().Val == "," { p.next() }
				}
				p.expect(")")
				return &Node{Kind: NodeCall, Str: fullName, Args: args}
			}
			return &Node{Kind: NodeIdent, Str: fullName}
		}
		return &Node{Kind: NodeIdent, Str: name}
	}

	p.next() // skip unknown
	return nil
}

func (p *Parser) parseArrayLiteral() *Node {
	p.expect("[")
	var elems []*Node
	for p.peek().Val != "]" && p.peek().Kind != 5 {
		if p.peek().Val == "..." {
			p.next() // consume ...
			elems = append(elems, &Node{Kind: NodeSpread, Left: p.parseAssign()})
		} else {
			elems = append(elems, p.parseExpr())
		}
		if p.peek().Val == "," { p.next() }
	}
	p.expect("]")
	return &Node{Kind: NodeArray, Args: elems}
}

func (p *Parser) parseObjectLiteral() *Node {
	p.expect("{")
	var pairs []*Node // alternating key, value nodes
	for p.peek().Val != "}" && p.peek().Kind != 5 {
		key := p.next().Val
		p.expect(":") // colon
		val := p.parseExpr()
		pairs = append(pairs, &Node{Kind: NodeString, Str: key}, val)
		if p.peek().Val == "," { p.next() }
	}
	p.expect("}")
	return &Node{Kind: NodeObject, Args: pairs}
}
