package abaplint

// Statement represents a parsed ABAP statement (one or more tokens ending at period).
// The statement splitter handles colon chaining: "DATA: a TYPE i, b TYPE string."
// becomes two statements: [DATA a TYPE i ,] and [DATA b TYPE string .]
type Statement struct {
	Tokens  []Token
	Pragmas []Token // pragma tokens (##...) stripped from main token list
	Type    string  // "Unknown", "Comment", "Empty", or matched statement type
	Colon   *Token  // the colon token if this was part of a chained statement
}

// ConcatTokens returns all token strings joined by space.
func (s *Statement) ConcatTokens() string {
	if len(s.Tokens) == 0 {
		return ""
	}
	result := s.Tokens[0].Str
	for i := 1; i < len(s.Tokens); i++ {
		result += " " + s.Tokens[i].Str
	}
	return result
}

// FirstTokenStr returns the uppercase string of the first token, or empty.
func (s *Statement) FirstTokenStr() string {
	if len(s.Tokens) == 0 {
		return ""
	}
	return toUpper(s.Tokens[0].Str)
}

// StatementParser splits lexer tokens into statements, handling colon chaining.
// This is a mechanical port of abaplint's StatementParser.process().
type StatementParser struct{}

// Parse splits tokens into statements. Handles:
// - Period (.) terminates a statement
// - Colon (:) starts chaining — prefix is shared across comma-separated parts
// - Comma (,) within chaining starts next chained statement (with same prefix)
// - Comment tokens become their own Comment statements
func (p *StatementParser) Parse(tokens []Token) []Statement {
	var statements []Statement
	var add []Token   // current tokens being accumulated
	var pre []Token   // prefix tokens (before colon)
	var colon *Token  // the colon token, if in chaining mode

	for _, token := range tokens {
		if token.Type == TokenComment {
			statements = append(statements, Statement{
				Tokens: []Token{token},
				Type:   "Comment",
			})
			continue
		}

		add = append(add, token)

		if len(token.Str) == 1 {
			switch token.Str {
			case ".":
				// End of statement
				stmt := buildStatement(pre, add, colon)
				statements = append(statements, stmt)
				add = nil
				pre = nil
				colon = nil

			case ",":
				if len(pre) > 0 {
					// Chained statement separator
					stmt := buildStatement(pre, add, colon)
					statements = append(statements, stmt)
					add = nil
				}

			case ":":
				if colon == nil {
					// First colon — start chaining
					colonTok := add[len(add)-1]
					colon = &colonTok
					add = add[:len(add)-1] // remove colon from tokens
					pre = append(pre, add...)
					add = nil
				} else {
					// Additional colons — just remove them
					add = add[:len(add)-1]
				}
			}
		}
	}

	// Remaining tokens
	if len(add) > 0 {
		stmt := buildStatement(pre, add, colon)
		statements = append(statements, stmt)
	}

	// Post-process: handle NativeSQL blocks (AMDP, EXEC SQL)
	statements = nativeSQL(statements)

	return statements
}

func buildStatement(pre, add []Token, colon *Token) Statement {
	var tokens []Token
	tokens = append(tokens, pre...)
	tokens = append(tokens, add...)

	// Separate pragmas from statement tokens (matching abaplint behavior).
	// Pragmas are removed from the token list but kept in the Pragmas field.
	var filtered []Token
	var pragmas []Token
	for i, tok := range tokens {
		if tok.Type == TokenPragma && i < len(tokens)-1 {
			// Skip pragmas (but not the last token which is punctuation)
			pragmas = append(pragmas, tok)
		} else {
			filtered = append(filtered, tok)
		}
	}

	stmtType := "Unknown"
	if len(filtered) == 1 && filtered[0].Type == TokenPunctuation {
		stmtType = "Empty"
	}

	return Statement{
		Tokens:  filtered,
		Pragmas: pragmas,
		Type:    stmtType,
		Colon:   colon,
	}
}

// nativeSQL post-processes statements to handle embedded SQL blocks.
// After METHOD ... BY DATABASE, all statements until ENDMETHOD become NativeSQL.
// If ENDMETHOD tokens are glued onto the last SQL statement, they get split out.
// This matches abaplint's nativeSQL() method in statement_parser.ts.
func nativeSQL(stmts []Statement) []Statement {
	var result []Statement
	sql := false

	for i := 0; i < len(stmts); i++ {
		stmt := stmts[i]

		if !sql {
			// Check if this is METHOD ... BY DATABASE
			if isMethodByDatabase(stmt) {
				sql = true
				result = append(result, stmt)
				continue
			}
			result = append(result, stmt)
			continue
		}

		// In SQL mode
		first := stmt.FirstTokenStr()
		if first == "ENDMETHOD" {
			sql = false
			result = append(result, stmt)
			continue
		}

		// Check if this statement ends with ENDMETHOD.
		// If so, split ENDMETHOD out.
		tokens := stmt.Tokens
		if len(tokens) >= 2 {
			lastIdx := len(tokens) - 1
			// Last token should be "." (Punctuation)
			// Second-to-last should be "ENDMETHOD"
			if tokens[lastIdx].Type == TokenPunctuation && tokens[lastIdx].Str == "." &&
				toUpper(tokens[lastIdx-1].Str) == "ENDMETHOD" {
				// Split: NativeSQL = tokens[0..lastIdx-2], EndMethod = tokens[lastIdx-1..lastIdx]
				sqlTokens := tokens[:lastIdx-1]
				endTokens := tokens[lastIdx-1:]

				if len(sqlTokens) > 0 {
					result = append(result, Statement{Tokens: sqlTokens, Type: "NativeSQL"})
				}
				result = append(result, Statement{Tokens: endTokens, Type: "Unknown"})
				sql = false
				continue
			}
		}

		// Regular SQL line
		stmt.Type = "NativeSQL"
		result = append(result, stmt)
	}

	return result
}

func isMethodByDatabase(stmt Statement) bool {
	// Look for "METHOD <name> BY DATABASE" pattern
	for i, tok := range stmt.Tokens {
		if toUpper(tok.Str) == "DATABASE" && i >= 2 && toUpper(stmt.Tokens[i-1].Str) == "BY" {
			return true
		}
	}
	return false
}

// toUpper is a simple ASCII upper for ABAP keywords.
func toUpper(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		b[i] = c
	}
	return string(b)
}
