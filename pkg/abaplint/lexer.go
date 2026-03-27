// Package abaplint provides a native Go port of the abaplint lexer.
// This is a mechanical translation of the TypeScript abaplint lexer
// (https://github.com/abaplint/abaplint) by Lars Hvam Petersen (MIT license).
//
// The lexer tokenizes ABAP source code into typed tokens with position info.
// Token types encode whitespace context (W prefix = whitespace before, W suffix = whitespace after).
package abaplint

import "strings"

// TokenType identifies the kind of token.
type TokenType int

const (
	TokenIdentifier TokenType = iota
	TokenComment
	TokenString        // single-quoted 'abc'
	TokenStringTemplate // |complete template|
	TokenStringTemplateBegin // |begin{
	TokenStringTemplateEnd   // }end|
	TokenStringTemplateMiddle // }middle{
	TokenPunctuation   // . ,
	TokenPragma        // ##PRAGMA
	// Parens — 4 variants: no-ws, ws-before (W prefix), ws-after (W suffix), both
	TokenParenLeft
	TokenParenLeftW
	TokenWParenLeft
	TokenWParenLeftW
	TokenParenRight
	TokenParenRightW
	TokenWParenRight
	TokenWParenRightW
	// Brackets
	TokenBracketLeft
	TokenBracketLeftW
	TokenWBracketLeft
	TokenWBracketLeftW
	TokenBracketRight
	TokenBracketRightW
	TokenWBracketRight
	TokenWBracketRightW
	// Dash
	TokenDash
	TokenDashW
	TokenWDash
	TokenWDashW
	// Plus
	TokenPlus
	TokenPlusW
	TokenWPlus
	TokenWPlusW
	// At @
	TokenAt
	TokenAtW
	TokenWAt
	TokenWAtW
	// Instance arrow ->
	TokenInstanceArrow
	TokenInstanceArrowW
	TokenWInstanceArrow
	TokenWInstanceArrowW
	// Static arrow =>
	TokenStaticArrow
	TokenStaticArrowW
	TokenWStaticArrow
	TokenWStaticArrowW
)

// tokenTypeNames maps TokenType to the abaplint class name for oracle comparison.
var tokenTypeNames = map[TokenType]string{
	TokenIdentifier:          "Identifier",
	TokenComment:             "Comment",
	TokenString:              "StringToken",
	TokenStringTemplate:      "StringTemplate",
	TokenStringTemplateBegin: "StringTemplateBegin",
	TokenStringTemplateEnd:   "StringTemplateEnd",
	TokenStringTemplateMiddle: "StringTemplateMiddle",
	TokenPunctuation:         "Punctuation",
	TokenPragma:              "Pragma",
	TokenParenLeft:           "ParenLeft",
	TokenParenLeftW:          "ParenLeftW",
	TokenWParenLeft:          "WParenLeft",
	TokenWParenLeftW:         "WParenLeftW",
	TokenParenRight:          "ParenRight",
	TokenParenRightW:         "ParenRightW",
	TokenWParenRight:         "WParenRight",
	TokenWParenRightW:        "WParenRightW",
	TokenBracketLeft:         "BracketLeft",
	TokenBracketLeftW:        "BracketLeftW",
	TokenWBracketLeft:        "WBracketLeft",
	TokenWBracketLeftW:       "WBracketLeftW",
	TokenBracketRight:        "BracketRight",
	TokenBracketRightW:       "BracketRightW",
	TokenWBracketRight:       "WBracketRight",
	TokenWBracketRightW:      "WBracketRightW",
	TokenDash:                "Dash",
	TokenDashW:               "DashW",
	TokenWDash:               "WDash",
	TokenWDashW:              "WDashW",
	TokenPlus:                "Plus",
	TokenPlusW:               "PlusW",
	TokenWPlus:               "WPlus",
	TokenWPlusW:              "WPlusW",
	TokenAt:                  "At",
	TokenAtW:                 "AtW",
	TokenWAt:                 "WAt",
	TokenWAtW:                "WAtW",
	TokenInstanceArrow:       "InstanceArrow",
	TokenInstanceArrowW:      "InstanceArrowW",
	TokenWInstanceArrow:      "WInstanceArrow",
	TokenWInstanceArrowW:     "WInstanceArrowW",
	TokenStaticArrow:         "StaticArrow",
	TokenStaticArrowW:        "StaticArrowW",
	TokenWStaticArrow:        "WStaticArrow",
	TokenWStaticArrowW:       "WStaticArrowW",
}

func (t TokenType) String() string {
	if s, ok := tokenTypeNames[t]; ok {
		return s
	}
	return "Unknown"
}

// Token represents a lexed ABAP token with position and type.
type Token struct {
	Str   string
	Type  TokenType
	Row   int // 1-based
	Col   int // 1-based
}

// --- Lexer Stream (mechanical port of LexerStream) ---

type lexerStream struct {
	raw    string
	offset int
	row    int
	col    int
}

func newLexerStream(raw string) *lexerStream {
	return &lexerStream{raw: raw, offset: -1, row: 0, col: 0}
}

func (s *lexerStream) advance() bool {
	if s.currentChar() == "\n" {
		s.col = 1
		s.row++
	}
	if s.offset == len(s.raw) {
		s.col--
		return false
	}
	s.col++
	s.offset++
	return true
}

func (s *lexerStream) getCol() int    { return s.col }
func (s *lexerStream) getRow() int    { return s.row }
func (s *lexerStream) getOffset() int { return s.offset }
func (s *lexerStream) getRaw() string { return s.raw }

func (s *lexerStream) prevChar() string {
	if s.offset-1 < 0 {
		return ""
	}
	return s.raw[s.offset-1 : s.offset]
}

func (s *lexerStream) prevPrevChar() string {
	if s.offset-2 < 0 {
		return ""
	}
	return s.raw[s.offset-2 : s.offset]
}

func (s *lexerStream) currentChar() string {
	if s.offset < 0 {
		return "\n" // simulate newline at start of file to handle star(*) comments
	} else if s.offset >= len(s.raw) {
		return ""
	}
	return s.raw[s.offset : s.offset+1]
}

func (s *lexerStream) nextChar() string {
	if s.offset+2 > len(s.raw) {
		return ""
	}
	return s.raw[s.offset+1 : s.offset+2]
}

func (s *lexerStream) nextNextChar() string {
	if s.offset+3 > len(s.raw) {
		return s.nextChar()
	}
	return s.raw[s.offset+1 : s.offset+3]
}

// --- Lexer Buffer (mechanical port of LexerBuffer) ---

type lexerBuffer struct {
	buf strings.Builder
}

func (b *lexerBuffer) add(s string) string {
	b.buf.WriteString(s)
	return b.buf.String()
}

func (b *lexerBuffer) get() string {
	return b.buf.String()
}

func (b *lexerBuffer) clear() {
	b.buf.Reset()
}

func (b *lexerBuffer) countIsEven(ch byte) bool {
	count := 0
	s := b.buf.String()
	for i := 0; i < len(s); i++ {
		if s[i] == ch {
			count++
		}
	}
	return count%2 == 0
}

// --- Lexer modes (mechanical port of Lexer) ---

const (
	modeNormal   = 1
	modePing     = 2
	modeStr      = 3
	modeTemplate = 4
	modeComment  = 5
	modePragma   = 6
)

// Lexer tokenizes ABAP source code.
type Lexer struct {
	tokens []Token
	m      int
	stream *lexerStream
	buffer *lexerBuffer
}

// Run tokenizes the given ABAP source and returns tokens.
func (l *Lexer) Run(source string) []Token {
	l.tokens = nil
	l.m = modeNormal
	l.process(source)
	return l.tokens
}

func (l *Lexer) add() {
	s := strings.TrimSpace(l.buffer.get())
	if len(s) > 0 {
		col := l.stream.getCol()
		row := l.stream.getRow()

		whiteBefore := false
		if l.stream.getOffset()-len(s) >= 0 {
			prev := l.stream.getRaw()[l.stream.getOffset()-len(s) : l.stream.getOffset()-len(s)+1]
			if prev == " " || prev == "\n" || prev == "\t" || prev == ":" {
				whiteBefore = true
			}
		}

		whiteAfter := false
		next := l.stream.nextChar()
		if next == " " || next == "\n" || next == "\t" || next == ":" || next == "," || next == "." || next == "" || next == "\"" {
			whiteAfter = true
		}

		pos := Token{Str: s, Row: row, Col: col - len(s)}

		if l.m == modeComment {
			pos.Type = TokenComment
		} else if l.m == modePing || l.m == modeStr {
			pos.Type = TokenString
		} else if l.m == modeTemplate {
			first := s[0]
			last := s[len(s)-1]
			if first == '|' && last == '|' {
				pos.Type = TokenStringTemplate
			} else if first == '|' && last == '{' && whiteAfter {
				pos.Type = TokenStringTemplateBegin
			} else if first == '}' && last == '|' && whiteBefore {
				pos.Type = TokenStringTemplateEnd
			} else if first == '}' && last == '{' && whiteAfter && whiteBefore {
				pos.Type = TokenStringTemplateMiddle
			} else {
				pos.Type = TokenIdentifier
			}
		} else if len(s) > 2 && s[:2] == "##" {
			pos.Type = TokenPragma
		} else if len(s) == 1 {
			switch s {
			case ".", ",":
				pos.Type = TokenPunctuation
			case "[":
				pos.Type = wsVariant4(whiteBefore, whiteAfter, TokenBracketLeft, TokenBracketLeftW, TokenWBracketLeft, TokenWBracketLeftW)
			case "(":
				pos.Type = wsVariant4(whiteBefore, whiteAfter, TokenParenLeft, TokenParenLeftW, TokenWParenLeft, TokenWParenLeftW)
			case "]":
				pos.Type = wsVariant4(whiteBefore, whiteAfter, TokenBracketRight, TokenBracketRightW, TokenWBracketRight, TokenWBracketRightW)
			case ")":
				pos.Type = wsVariant4(whiteBefore, whiteAfter, TokenParenRight, TokenParenRightW, TokenWParenRight, TokenWParenRightW)
			case "-":
				pos.Type = wsVariant4(whiteBefore, whiteAfter, TokenDash, TokenDashW, TokenWDash, TokenWDashW)
			case "+":
				pos.Type = wsVariant4(whiteBefore, whiteAfter, TokenPlus, TokenPlusW, TokenWPlus, TokenWPlusW)
			case "@":
				pos.Type = wsVariant4(whiteBefore, whiteAfter, TokenAt, TokenAtW, TokenWAt, TokenWAtW)
			default:
				pos.Type = TokenIdentifier
			}
		} else if len(s) == 2 {
			if s == "->" {
				pos.Type = wsVariant4(whiteBefore, whiteAfter, TokenInstanceArrow, TokenInstanceArrowW, TokenWInstanceArrow, TokenWInstanceArrowW)
			} else if s == "=>" {
				pos.Type = wsVariant4(whiteBefore, whiteAfter, TokenStaticArrow, TokenStaticArrowW, TokenWStaticArrow, TokenWStaticArrowW)
			} else {
				pos.Type = TokenIdentifier
			}
		} else {
			pos.Type = TokenIdentifier
		}

		l.tokens = append(l.tokens, pos)
	}
	l.buffer.clear()
}

// wsVariant4 picks the right whitespace-annotated token type.
func wsVariant4(wBefore, wAfter bool, none, afterOnly, beforeOnly, both TokenType) TokenType {
	if wBefore && wAfter {
		return both
	} else if wBefore {
		return beforeOnly
	} else if wAfter {
		return afterOnly
	}
	return none
}

func (l *Lexer) process(raw string) {
	raw = strings.ReplaceAll(raw, "\r", "")
	l.stream = newLexerStream(raw)
	l.buffer = &lexerBuffer{}

	splits := map[byte]bool{
		' ': true, ':': true, '.': true, ',': true,
		'-': true, '+': true, '(': true, ')': true,
		'[': true, ']': true, '\t': true, '\n': true,
	}

	bufs := map[byte]bool{
		'.': true, ',': true, ':': true,
		'(': true, ')': true, '[': true, ']': true,
		'+': true, '@': true,
	}

	for {
		current := l.stream.currentChar()
		buf := l.buffer.add(current)
		ahead := l.stream.nextChar()
		aahead := l.stream.nextNextChar()

		if l.m == modeNormal {
			if len(ahead) == 1 && splits[ahead[0]] {
				l.add()
			} else if ahead == "'" {
				// start string
				l.add()
				l.m = modeStr
			} else if ahead == "|" || ahead == "}" {
				// start template
				l.add()
				l.m = modeTemplate
			} else if ahead == "`" {
				// start ping
				l.add()
				l.m = modePing
			} else if aahead == "##" {
				// start pragma
				l.add()
				l.m = modePragma
			} else if ahead == "\"" || (ahead == "*" && current == "\n") {
				// start comment
				l.add()
				l.m = modeComment
			} else if ahead == "@" && len(strings.TrimSpace(buf)) == 0 {
				l.add()
			} else if aahead == "->" || aahead == "=>" {
				l.add()
			} else if current == ">" && ahead != " " &&
				(l.stream.prevChar() == "-" || l.stream.prevChar() == "=") {
				// arrows
				l.add()
			} else if len(buf) == 1 &&
				(bufs[buf[0]] || (buf == "-" && ahead != ">")) {
				l.add()
			}
		} else if l.m == modePragma &&
			(ahead == "," || ahead == ":" || ahead == "." || ahead == " " || ahead == "\n") {
			// end of pragma
			l.add()
			l.m = modeNormal
		} else if l.m == modePing &&
			len(buf) > 1 &&
			current == "`" &&
			aahead != "``" &&
			ahead != "`" &&
			l.buffer.countIsEven('`') {
			// end of ping
			l.add()
			if ahead == "\"" {
				l.m = modeComment
			} else {
				l.m = modeNormal
			}
		} else if l.m == modeTemplate &&
			len(buf) > 1 &&
			(current == "|" || current == "{") &&
			(l.stream.prevChar() != "\\" || l.stream.prevPrevChar() == "\\\\") {
			// end of template
			l.add()
			l.m = modeNormal
		} else if l.m == modeTemplate && ahead == "}" && current != "\\" {
			l.add()
		} else if l.m == modeStr &&
			current == "'" &&
			len(buf) > 1 &&
			aahead != "''" &&
			ahead != "'" &&
			l.buffer.countIsEven('\'') {
			// end of string
			l.add()
			if ahead == "\"" {
				l.m = modeComment
			} else {
				l.m = modeNormal
			}
		} else if ahead == "\n" && l.m != modeTemplate {
			l.add()
			l.m = modeNormal
		} else if l.m == modeTemplate && current == "\n" {
			l.add()
		}

		if !l.stream.advance() {
			break
		}
	}

	l.add()
}
