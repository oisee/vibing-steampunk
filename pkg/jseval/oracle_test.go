package jseval

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The JS lexer source (same as abaplint_test.go but returns JSON)
const lexerJS = `
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
  if (c >= 48 && c <= 57) return true;
  return false;
}

function isDigit(ch) {
  let c = ch.charCodeAt(0);
  return c >= 48 && c <= 57;
}

function isAlpha(ch) {
  let c = ch.charCodeAt(0);
  if (c >= 65 && c <= 90) return true;
  if (c >= 97 && c <= 122) return true;
  if (ch === "_") return true;
  return false;
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
    if (ch === "\r") {
      i = i + 1; continue;
    }
    if (ch === ".") {
      tokens.push(new Token("punct", ".", row, col));
      i = i + 1; col = col + 1; continue;
    }
    if (ch === ":" || ch === "," || ch === "(" || ch === ")") {
      tokens.push(new Token("punct", ch, row, col));
      i = i + 1; col = col + 1; continue;
    }
    if (ch === "=" || ch === "+" || ch === "-" || ch === "*" || ch === "/") {
      tokens.push(new Token("op", ch, row, col));
      i = i + 1; col = col + 1; continue;
    }
    if (ch === "<" || ch === ">") {
      tokens.push(new Token("op", ch, row, col));
      i = i + 1; col = col + 1; continue;
    }
    if (isAlpha(ch)) {
      let start = i;
      let startCol = col;
      while (i < source.length && isLetter(source.charAt(i))) {
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
    if (ch === "*" && col === 1) {
      let start = i;
      let startCol = col;
      while (i < source.length && source.charAt(i) !== "\n") {
        i = i + 1; col = col + 1;
      }
      let comment = source.substring(start, i);
      tokens.push(new Token("comment", comment, row, startCol));
      continue;
    }
    if (ch === "\"") {
      let start = i;
      let startCol = col;
      while (i < source.length && source.charAt(i) !== "\n") {
        i = i + 1; col = col + 1;
      }
      let comment = source.substring(start, i);
      tokens.push(new Token("comment", comment, row, startCol));
      continue;
    }
    i = i + 1; col = col + 1;
  }
  return tokens;
}
`

type tokenResult struct {
	Type string `json:"type"`
	Str  string `json:"str"`
}

func TestOracleComparison(t *testing.T) {
	// ABAP test corpus
	abapSamples := []struct {
		name string
		code string
	}{
		{"simple", "DATA lv_x TYPE i.\nlv_x = 42.\nWRITE lv_x."},
		{"string", "DATA lv TYPE string.\nlv = 'hello world'."},
		{"method", "DATA(lo) = NEW zcl_test( ).\nlo->method( )."},
		{"loop", "DO 10 TIMES.\n  WRITE sy-index.\nENDDO."},
		{"if", "IF lv > 0.\n  WRITE 'positive'.\nELSE.\n  WRITE 'negative'.\nENDIF."},
		{"select", "SELECT * FROM mara INTO TABLE lt_mara."},
		{"chain", "DATA: lv1 TYPE i, lv2 TYPE string, lv3 TYPE c."},
		{"comment", "* This is a comment\nDATA lv TYPE i. \" inline comment"},
	}

	for _, s := range abapSamples {
		t.Run(s.name, func(t *testing.T) {
			// Run on our jseval
			jsCode := lexerJS + fmt.Sprintf(`
let source = "%s";
let result = lexer(source);
let out = "";
for (let i = 0; i < result.length; i = i + 1) {
  let t = result[i];
  if (i > 0) out = out + "|";
  out = out + t.type + ":" + t.str;
}
console.log(out);
`, strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s.code, "\\", "\\\\"), "\"", "\\\""), "\n", "\\n"))

			goOut, err := Eval(jsCode)
			if err != nil {
				t.Fatalf("jseval error: %v", err)
			}
			goTokens := strings.TrimSpace(goOut)

			// Run on Node.js for comparison
			nodeCode := lexerJS + fmt.Sprintf(`
let source = "%s";
let result = lexer(source);
let out = "";
for (let i = 0; i < result.length; i++) {
  let t = result[i];
  if (i > 0) out += "|";
  out += t.type + ":" + t.str;
}
console.log(out);
`, strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s.code, "\\", "\\\\"), "\"", "\\\""), "\n", "\\n"))

			tmpFile := fmt.Sprintf("/tmp/oracle_test_%s.js", s.name)
			os.WriteFile(tmpFile, []byte(nodeCode), 0644)
			nodeResult, err := exec.Command("node", tmpFile).CombinedOutput()
			if err != nil {
				t.Fatalf("node error: %v\n%s", err, nodeResult)
			}
			nodeTokens := strings.TrimSpace(string(nodeResult))

			if goTokens != nodeTokens {
				t.Errorf("MISMATCH!\n  Go jseval: %s\n  Node.js:   %s", goTokens, nodeTokens)
			} else {
				t.Logf("MATCH (%d tokens): %s", strings.Count(goTokens, "|")+1, goTokens)
			}
		})
	}
}
