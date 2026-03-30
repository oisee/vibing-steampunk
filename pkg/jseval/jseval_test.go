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
