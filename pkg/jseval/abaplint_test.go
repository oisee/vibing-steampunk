package jseval

import (
	"strings"
	"testing"
)

func TestAbaplintLexer(t *testing.T) {
	code := `
class Token {
  constructor(type, str, row, col) {
    this.type = type;
    this.str = str;
    this.row = row;
    this.col = col;
  }
}

function isLetter(ch) {
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

function lexer(source) {
  let tokens = [];
  let i = 0;
  let row = 1;
  let col = 1;

  while (i < source.length) {
    let ch = source.charAt(i);

    if (ch === " " || ch === "\t") {
      i = i + 1; col = col + 1; continue;
    }
    if (ch === "\n") {
      i = i + 1; row = row + 1; col = 1; continue;
    }
    if (ch === ".") {
      tokens.push(new Token("punct", ".", row, col));
      i = i + 1; col = col + 1; continue;
    }
    if (ch === ":") {
      tokens.push(new Token("punct", ":", row, col));
      i = i + 1; col = col + 1; continue;
    }
    if (ch === ",") {
      tokens.push(new Token("punct", ",", row, col));
      i = i + 1; col = col + 1; continue;
    }
    if (ch === "=") {
      tokens.push(new Token("op", "=", row, col));
      i = i + 1; col = col + 1; continue;
    }
    if (isLetter(ch)) {
      let start = i;
      let startCol = col;
      while (i < source.length && (isLetter(source.charAt(i)) || isDigit(source.charAt(i)))) {
        i = i + 1; col = col + 1;
      }
      let word = source.substring(start, i);
      tokens.push(new Token("ident", word, row, startCol));
      continue;
    }
    if (isDigit(ch)) {
      let start = i;
      let startCol = col;
      while (i < source.length && isDigit(source.charAt(i))) {
        i = i + 1; col = col + 1;
      }
      let num = source.substring(start, i);
      tokens.push(new Token("number", num, row, startCol));
      continue;
    }
    if (ch === "'") {
      let start = i + 1;
      let startCol = col;
      i = i + 1; col = col + 1;
      while (i < source.length && source.charAt(i) !== "'") {
        i = i + 1; col = col + 1;
      }
      let str = source.substring(start, i);
      tokens.push(new Token("string", str, row, startCol));
      i = i + 1; col = col + 1;
      continue;
    }
    i = i + 1; col = col + 1;
  }
  return tokens;
}

let abap = "DATA lv_x TYPE i.\nlv_x = 42.\nWRITE lv_x.";
let result = lexer(abap);
console.log("Tokens: " + result.length);
for (let i = 0; i < result.length; i = i + 1) {
  let t = result[i];
  console.log("  " + t.type + ": " + t.str);
}
`
	out, err := Eval(code)
	if err != nil {
		t.Fatalf("Eval error: %v", err)
	}
	t.Logf("Output:\n%s", out)

	// Verify key results
	if !strings.Contains(out, "Tokens: 12") {
		t.Errorf("Expected 12 tokens, got: %s", out)
	}
	if !strings.Contains(out, "ident: DATA") {
		t.Error("Missing 'ident: DATA'")
	}
	if !strings.Contains(out, "number: 42") {
		t.Error("Missing 'number: 42'")
	}
	if !strings.Contains(out, "ident: WRITE") {
		t.Error("Missing 'ident: WRITE'")
	}
}
