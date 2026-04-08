package jseval

import (
	"strings"
	"testing"
)

func TestBasic(t *testing.T) {
	tests := []struct {
		name   string
		code   string
		expect string
	}{
		{"2+2", `console.log(2+2)`, "4\n"},
		{"arithmetic", `console.log(6 * 7)`, "42\n"},
		{"string", `console.log("hello")`, "hello\n"},
		{"concat", `console.log("a" + "b")`, "ab\n"},
		{"variable", `let x = 10; console.log(x)`, "10\n"},
		{"assign", `let x = 1; x = 42; console.log(x)`, "42\n"},
		{"if-true", `if (1) console.log("yes")`, "yes\n"},
		{"if-false", `if (0) console.log("yes")`, ""},
		{"if-else", `if (0) console.log("a"); else console.log("b")`, "b\n"},
		{"comparison", `console.log(3 < 7)`, "true\n"},
		{"multi", `console.log(2+3); console.log(4*5)`, "5\n20\n"},

		// Functions
		{"function", `
			function add(a, b) { return a + b; }
			console.log(add(3, 4))
		`, "7\n"},
		{"factorial", `
			function factorial(n) {
				if (n <= 1) return 1;
				return n * factorial(n - 1);
			}
			console.log(factorial(10))
		`, "3628800\n"},
		{"fibonacci", `
			function fib(n) {
				if (n <= 1) return n;
				let a = 0;
				let b = 1;
				let i = 2;
				while (i <= n) {
					let t = a + b;
					a = b;
					b = t;
					i = i + 1;
				}
				return b;
			}
			console.log(fib(10))
		`, "55\n"},
		// Objects
		{"object-literal", `let o = {x: 1, y: 2}; console.log(o.x + o.y)`, "3\n"},
		{"object-assign", `let o = {}; o.name = "hello"; console.log(o.name)`, "hello\n"},
		{"object-bracket", `let o = {a: 42}; console.log(o["a"])`, "42\n"},

		// Arrays
		{"array-literal", `let a = [10, 20, 30]; console.log(a[1])`, "20\n"},
		{"array-push", `let a = []; a.push(1); a.push(2); console.log(a.length)`, "2\n"},
		{"array-length", `let a = [1,2,3]; console.log(a.length)`, "3\n"},

		// String methods
		{"string-length", `console.log("hello".length)`, "5\n"},
		{"string-charAt", `console.log("hello".charAt(1))`, "e\n"},
		{"string-indexOf", `console.log("hello".indexOf("ll"))`, "2\n"},
		{"string-substring", `console.log("hello".substring(1, 3))`, "el\n"},
		{"string-charCodeAt", `console.log("A".charCodeAt(0))`, "65\n"},

		// For loop
		{"for-loop", `let s = 0; for (let i = 1; i <= 10; i = i + 1) { s = s + i; } console.log(s)`, "55\n"},

		// Switch
		{"switch", `let x = 2; let r = ""; switch(x) { case 1: r = "one"; break; case 2: r = "two"; break; } console.log(r)`, "two\n"},

		// typeof
		{"typeof-num", `console.log(typeof 42)`, "number\n"},
		{"typeof-str", `console.log(typeof "hi")`, "string\n"},

		// Closures
		{"closure", `
			function make() { let x = 10; function get() { return x; } return get; }
			let f = make();
			console.log(f())
		`, "10\n"},

		// Class (minimal)
		{"class", `
			class Counter {
				constructor(n) { this.n = n; }
				inc() { this.n = this.n + 1; }
				get() { return this.n; }
			}
			let c = new Counter(0);
			c.inc();
			c.inc();
			c.inc();
			console.log(c.get())
		`, "3\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Eval(tc.code)
			if err != nil {
				t.Fatalf("Eval error: %v", err)
			}
			if strings.TrimRight(out, "\n") != strings.TrimRight(tc.expect, "\n") {
				t.Errorf("got %q, want %q", out, tc.expect)
			}
		})
	}
}
