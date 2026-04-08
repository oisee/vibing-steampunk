package jseval

import (
	"strings"
	"testing"
)

// Test all features the abaplint lexer needs
func TestLexerFeatures(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		// Class with constructor and this
		{"class-this", `
			class Token {
				constructor(type, str) {
					this.type = type;
					this.str = str;
				}
			}
			let t = new Token("ident", "DATA");
			console.log(t.type + ":" + t.str);
		`, "ident:DATA"},
		// Array push + length + iteration
		{"array-iter", `
			let a = [];
			a.push("x"); a.push("y"); a.push("z");
			let out = "";
			for (let i = 0; i < a.length; i = i + 1) {
				if (i > 0) out = out + ",";
				out = out + a[i];
			}
			console.log(out);
		`, "x,y,z"},
		// String charAt + charCodeAt + comparison
		{"string-ops", `
			let s = "Hello";
			console.log(s.charAt(0));
			console.log(s.charCodeAt(0));
			console.log(s.length);
			console.log(s.substring(1, 3));
		`, "H\n72\n5\nel"},
		// While + continue
		{"while-continue", `
			let i = 0; let out = "";
			while (i < 5) {
				i = i + 1;
				if (i === 3) continue;
				out = out + i;
			}
			console.log(out);
		`, "1245"},
		// === with string operators (the bug we just fixed)
		{"string-op-compare", `
			let ch = "+";
			if (ch === "=" || ch === "+" || ch === "-" || ch === "*" || ch === "/") {
				console.log("op");
			} else {
				console.log("not");
			}
		`, "op"},
		// Nested function + closure
		{"nested-fn", `
			function isAlpha(ch) {
				let c = ch.charCodeAt(0);
				if (c >= 65 && c <= 90) return true;
				if (c >= 97 && c <= 122) return true;
				if (ch === "_") return true;
				return false;
			}
			console.log(isAlpha("D"));
			console.log(isAlpha("4"));
			console.log(isAlpha("_"));
		`, "true\nfalse\ntrue"},
		// Full mini-lexer test
		{"mini-lexer", `
			class Token {
				constructor(type, str) { this.type = type; this.str = str; }
			}
			function isAlpha(ch) {
				let c = ch.charCodeAt(0);
				if (c >= 65 && c <= 90) return true;
				if (c >= 97 && c <= 122) return true;
				if (ch === "_") return true;
				return false;
			}
			function isDigit(ch) {
				let c = ch.charCodeAt(0);
				return c >= 48 && c <= 57;
			}
			function isLetter(ch) {
				let c = ch.charCodeAt(0);
				if (c >= 65 && c <= 90) return true;
				if (c >= 97 && c <= 122) return true;
				if (ch === "_") return true;
				if (c >= 48 && c <= 57) return true;
				return false;
			}
			let source = "DATA x.";
			let tokens = [];
			let i = 0;
			while (i < source.length) {
				let ch = source.charAt(i);
				if (ch === " ") { i = i + 1; continue; }
				if (ch === ".") {
					tokens.push(new Token("punct", "."));
					i = i + 1; continue;
				}
				if (isAlpha(ch)) {
					let start = i;
					while (i < source.length && isLetter(source.charAt(i))) {
						i = i + 1;
					}
					tokens.push(new Token("ident", source.substring(start, i)));
					continue;
				}
				i = i + 1;
			}
			let out = "";
			for (let j = 0; j < tokens.length; j = j + 1) {
				if (j > 0) out = out + "~";
				out = out + tokens[j].type + ":" + tokens[j].str;
			}
			console.log(out);
		`, "ident:DATA~ident:x~punct:."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Eval(tc.code)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			got := strings.TrimRight(out, "\n")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
