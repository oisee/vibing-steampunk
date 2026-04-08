package jseval

import (
	"strings"
	"testing"
)

func TestNewFeatures(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		// Ternary
		{"ternary-true", `console.log(true ? "yes" : "no")`, "yes"},
		{"ternary-false", `console.log(false ? "yes" : "no")`, "no"},
		{"ternary-expr", `let x = 5; console.log(x > 3 ? "big" : "small")`, "big"},
		{"ternary-nested", `console.log(true ? false ? "a" : "b" : "c")`, "b"},

		// Arrow functions
		{"arrow-expr", `let f = (a, b) => a + b; console.log(f(3, 4))`, "7"},
		{"arrow-body", `let f = (x) => { return x * 2; }; console.log(f(21))`, "42"},
		{"arrow-single", `let f = x => x + 1; console.log(f(9))`, "10"},
		{"arrow-no-params", `let f = () => 42; console.log(f())`, "42"},
		{"arrow-closure", `function make(n) { return () => n; } console.log(make(7)())`, "7"},

		// Throw / try / catch
		{"try-no-throw", `try { console.log("ok"); } catch(e) { console.log("err"); }`, "ok"},
		{"try-catch", `try { throw "boom"; } catch(e) { console.log("caught:" + e); }`, "caught:boom"},
		{"try-catch-value", `let r = ""; try { throw "x"; } catch(e) { r = e; } console.log(r)`, "x"},
		{"throw-number", `try { throw 42; } catch(e) { console.log(e); }`, "42"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Eval(tc.code)
			if err != nil { t.Fatalf("error: %v", err) }
			got := strings.TrimSpace(out)
			if got != tc.want { t.Errorf("got %q, want %q", got, tc.want) }
		})
	}
}

func TestForOfAndTemplates(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		// for...of
		{"for-of-array", `let a = [1,2,3]; let s = 0; for (const x of a) { s = s + x; } console.log(s)`, "6"},
		{"for-of-string-arr", `let a = ["a","b","c"]; let s = ""; for (let x of a) { s = s + x; } console.log(s)`, "abc"},
		{"for-of-break", `let a = [1,2,3,4,5]; let s = 0; for (const x of a) { if (x === 4) break; s = s + x; } console.log(s)`, "6"},
		// for...in
		{"for-in-obj", `let o = {a: 1, b: 2}; let keys = ""; for (const k in o) { keys = keys + k; } console.log(keys.length)`, "2"},
		// Template literals
		{"template-simple", "console.log(`hello`)", "hello"},
		{"template-expr", "let x = 42; console.log(`val=${x}`)", "val=42"},
		{"template-multi", "let a = 1; let b = 2; console.log(`${a}+${b}=${a+b}`)", "1+2=3"},
		{"template-nested", "console.log(`x=${true ? `yes` : `no`}`)", "x=yes"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Eval(tc.code)
			if err != nil { t.Fatalf("error: %v", err) }
			got := strings.TrimSpace(out)
			if got != tc.want { t.Errorf("got %q, want %q", got, tc.want) }
		})
	}
}

func TestAdvancedFeatures(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		// Function expression
		{"func-expr", `let f = function(x) { return x * 2; }; console.log(f(21))`, "42"},
		{"named-func-expr", `let f = function double(x) { return x * 2; }; console.log(f(21))`, "42"},
		// Static method on class
		{"static-assign", `class Foo {} Foo.bar = function(x) { return x * 2; }; console.log(Foo.bar(21))`, "42"},
		// Mini runtime pattern
		{"mini-runtime", `
class Integer {
  constructor(opts) { this.value = 0; }
  set(v) { if (v !== undefined && v.value !== undefined) { this.value = v.value; } }
  get() { return this.value; }
}
class Factory {}
Factory.get = function(n) { return {value: n}; };
let x = new Integer({});
x.set(Factory.get(42));
console.log(x.get());`, "42"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Eval(tc.code)
			if err != nil { t.Fatalf("error: %v", err) }
			got := strings.TrimSpace(out)
			if got != tc.want { t.Errorf("got %q, want %q", got, tc.want) }
		})
	}
}

func TestOpenAbapFeatures(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		// Nullish coalescing
		{"nullish-value", `let x = 42; console.log(x ?? 0)`, "42"},
		{"nullish-undef", `let x; console.log(x ?? "default")`, "default"},
		// Optional chaining
		{"optchain-ok", `let o = {a: {b: 1}}; console.log(o.a.b)`, "1"},
		{"optchain-null", `let o = {}; let r = o.missing; console.log(r)`, "undefined"},
		// new Error
		{"new-error", `let e = new Error("boom"); console.log(e.message)`, "boom"},
		{"new-typeerror", `let e = new TypeError("bad"); console.log(e.name + ":" + e.message)`, "TypeError:bad"},
		{"throw-error", `try { throw new Error("x"); } catch(e) { console.log(e.message); }`, "x"},
		// class extends
		{"extends", `
class Animal { constructor(n) { this.name = n; } speak() { return this.name; } }
class Dog extends Animal { constructor(n) { this.name = n; this.type = "dog"; } }
let d = new Dog("Rex");
console.log(d.speak() + " " + d.type);`, "Rex dog"},
		// static methods
		{"static-method", `
class Util {
  static double(x) { return x * 2; }
}
console.log(Util.double(21));`, "42"},
		// || returns value (not just bool)
		{"or-value", `console.log(0 || "fallback")`, "fallback"},
		{"or-truthy", `console.log(42 || "no")`, "42"},
		// function expression in object
		{"func-in-obj", `let o = {f: function(x) { return x + 1; }}; console.log(o.f(9))`, "10"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Eval(tc.code)
			if err != nil { t.Fatalf("error: %v", err) }
			got := strings.TrimSpace(out)
			if got != tc.want { t.Errorf("got %q, want %q", got, tc.want) }
		})
	}
}

func TestSpread(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{"rest-params", `function f(...args) { console.log(args.length); } f(1,2,3)`, "3"},
		{"rest-sum", `function sum(...args) { let s = 0; for (const x of args) { s = s + x; } return s; } console.log(sum(1,2,3,4))`, "10"},
		{"rest-mixed", `function f(a, b, ...rest) { console.log(a + "," + b + "," + rest.length); } f(1,2,3,4,5)`, "1,2,3"},
		{"spread-array", `let a = [1,2]; let b = [0, ...a, 3]; console.log(b.length)`, "4"},
		{"spread-concat", `let a = [1,2]; let b = [3,4]; let c = [...a, ...b]; console.log(c.length)`, "4"},
		{"spread-copy", `let a = [1,2,3]; let b = [...a]; console.log(b.length)`, "3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Eval(tc.code)
			if err != nil { t.Fatalf("error: %v", err) }
			got := strings.TrimSpace(out)
			if got != tc.want { t.Errorf("got %q, want %q", got, tc.want) }
		})
	}
}

func TestOpenAbapCoreShim(t *testing.T) {
	code := `
var abap = {};
abap.types = {};
abap.statements = {};
abap.IntegerFactory = { get: function(n) { return {value: n}; } };
abap.types.Integer = function(opts) {
  this.qualifiedName = opts ? opts.qualifiedName : "I";
  this.value = 0;
  this.set = function(v) {
    if (typeof v === "number") { this.value = v; }
    else if (v !== undefined && v !== null && v.value !== undefined) { this.value = v.value; }
  };
  this.get = function() { return this.value; };
};
abap.Console = function() { this.buffer = ""; this.add = function(s) { this.buffer = this.buffer + s; }; this.get = function() { return this.buffer; }; };
abap.statements.write = function(source) {
  if (abap.console === undefined) { abap.console = new abap.Console(); }
  let val = typeof source === "object" && source.get ? "" + source.get() : "" + source;
  abap.console.add(val);
};
let lv_x = new abap.types.Integer({qualifiedName: "I"});
lv_x.set(abap.IntegerFactory.get(42));
abap.statements.write(lv_x);
console.log(abap.console.get());
`
	out, err := Eval(code)
	if err != nil { t.Fatalf("error: %v", err) }
	got := strings.TrimSpace(out)
	if got != "42" { t.Errorf("got %q, want 42", got) }
}
