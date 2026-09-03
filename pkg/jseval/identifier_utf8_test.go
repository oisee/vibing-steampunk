package jseval

import (
	"testing"
	"unicode/utf8"
)

// TestTokenizeKeepsNonASCIIIdentifiersWhole pins a fix whose absence was
// invisible in every ASCII test.
//
// The identifier scanner asked unicode.IsLetter about one BYTE. Outside ASCII
// that is a question about Latin-1: "á" is 0xC3 0xA1, and 0xC3 as a rune is 'Ã'
// — a letter, so the scan began — while 0xA1 is '¡', which is not, so it ended
// one byte in. `var ábc = 1` tokenised as two identifiers, "\xc3" and "bc", the
// first of them not valid UTF-8.
func TestTokenizeKeepsNonASCIIIdentifiersWhole(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"latin-1 accent", "var ábc = 1", "ábc"},
		{"cyrillic", "var имя = 1", "имя"},
		{"cjk", "var 変数 = 1", "変数"},
		// U+3164 HANGUL FILLER is category Lo, invisible, and a valid JavaScript
		// identifier — V8 accepts `var ㅤ;`. So accepting it here is agreement
		// with the language, not an oversight, and this case is here to stop a
		// future "harden the tokeniser to ASCII" from silently diverging from JS.
		{"invisible but valid in JS", "var ㅤ = 1", "ㅤ"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ident string
			for _, tk := range tokenize(tc.src) {
				if tk.Kind == 2 && tk.Val != "var" {
					ident = tk.Val
					break
				}
			}
			if ident != tc.want {
				t.Errorf("identifier = %q, want %q", ident, tc.want)
			}
			if !utf8.ValidString(ident) {
				t.Errorf("identifier %q is not valid UTF-8 — the scanner split a rune", ident)
			}
		})
	}
}

// TestTokenizeStopsAtNonIdentifierRunes is the other half: widening the scanner
// to runes must not make it swallow separators.
func TestTokenizeStopsAtNonIdentifierRunes(t *testing.T) {
	// U+00A0 NBSP is a space separator, not a letter, and V8 rejects `var  ;`.
	toks := tokenize("var a b = 1")
	var idents []string
	for _, tk := range toks {
		if tk.Kind == 2 {
			idents = append(idents, tk.Val)
		}
	}
	if len(idents) != 3 || idents[1] != "a" || idents[2] != "b" {
		t.Errorf("NBSP must separate identifiers; got %q", idents)
	}
}
