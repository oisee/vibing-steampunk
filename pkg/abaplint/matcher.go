package abaplint

import "strings"

// StatementMatcher classifies statements by matching token patterns.
// This is a simplified port of abaplint's statement matching logic.
// It uses the combinator DSL to match against known patterns.
type StatementMatcher struct {
	byKeyword map[string][]namedMatcher // first-keyword → candidate matchers
	fallback  []namedMatcher            // matchers for statements starting with identifiers
}

type namedMatcher struct {
	name    string
	matcher Matcher
}

// NewStatementMatcher creates a matcher with all known statement patterns.
func NewStatementMatcher() *StatementMatcher {
	m := &StatementMatcher{
		byKeyword: make(map[string][]namedMatcher),
	}
	m.register()
	return m
}

// Classify determines the statement type for a given statement.
// Returns the type name (e.g., "Data", "If", "Move") or "Unknown".
func (sm *StatementMatcher) Classify(stmt *Statement) string {
	if stmt.Type == "Comment" || stmt.Type == "Empty" {
		return stmt.Type
	}

	tokens := stmt.Tokens
	if len(tokens) == 0 {
		return "Empty"
	}

	// Remove trailing punctuation for matching
	matchTokens := tokens
	if len(matchTokens) > 0 && matchTokens[len(matchTokens)-1].Type == TokenPunctuation {
		matchTokens = matchTokens[:len(matchTokens)-1]
	}

	if len(matchTokens) == 0 {
		return "Empty"
	}

	first := strings.ToUpper(matchTokens[0].Str)

	// Try keyword-specific matchers first
	if candidates, ok := sm.byKeyword[first]; ok {
		for _, c := range candidates {
			if Match(c.matcher, matchTokens) {
				return c.name
			}
		}
	}

	// Try fallback matchers (for Move, Call, etc.)
	for _, c := range sm.fallback {
		if Match(c.matcher, matchTokens) {
			return c.name
		}
	}

	return "Unknown"
}

func (sm *StatementMatcher) addKeyword(name, keyword string, m Matcher) {
	upper := strings.ToUpper(keyword)
	sm.byKeyword[upper] = append(sm.byKeyword[upper], namedMatcher{name, m})
}

func (sm *StatementMatcher) addFallback(name string, m Matcher) {
	sm.fallback = append(sm.fallback, namedMatcher{name, m})
}

// register defines all known statement patterns.
func (sm *StatementMatcher) register() {
	ident := Regex(`[\w~\/<>]+`) // matches identifiers, field-symbols, namespaced
	source := StarPrio(AnyToken()) // simplified: any remaining tokens
	anyRest := StarPrio(AnyToken())

	// Simple single-keyword statements
	for _, kw := range []struct{ name, keyword string }{
		{"EndIf", "ENDIF"},
		{"EndLoop", "ENDLOOP"},
		{"EndDo", "ENDDO"},
		{"EndWhile", "ENDWHILE"},
		{"EndCase", "ENDCASE"},
		{"EndTry", "ENDTRY"},
		{"EndMethod", "ENDMETHOD"},
		{"EndClass", "ENDCLASS"},
		{"EndForm", "ENDFORM"},
		{"EndFunction", "ENDFUNCTION"},
		{"EndInterface", "ENDINTERFACE"},
		{"EndModule", "ENDMODULE"},
		{"Else", "ELSE"},
		{"Try", "TRY"},
		{"Return", "RETURN"},
		{"Continue", "CONTINUE"},
		{"Exit", "EXIT"},
	} {
		kw := kw
		sm.addKeyword(kw.name, kw.keyword, Str(kw.keyword))
	}

	// Statements with fixed prefix + rest
	sm.addKeyword("Report", "REPORT", Seq(Str("REPORT"), anyRest))
	sm.addKeyword("Include", "INCLUDE", Seq(Str("INCLUDE"), anyRest))
	sm.addKeyword("If", "IF", Seq(Str("IF"), anyRest))
	sm.addKeyword("ElseIf", "ELSEIF", Seq(Str("ELSEIF"), anyRest))
	sm.addKeyword("While", "WHILE", Seq(Str("WHILE"), anyRest))
	sm.addKeyword("Do", "DO", Seq(Str("DO"), anyRest))
	sm.addKeyword("Case", "CASE", Seq(Str("CASE"), anyRest))
	sm.addKeyword("WhenOthers", "WHEN", Seq(Str("WHEN"), Str("OTHERS")))
	sm.addKeyword("When", "WHEN", Seq(Str("WHEN"), anyRest))
	sm.addKeyword("Loop", "LOOP", Seq(Str("LOOP"), anyRest))
	sm.addKeyword("Catch", "CATCH", Seq(Str("CATCH"), anyRest))
	sm.addKeyword("Raise", "RAISE", Seq(Str("RAISE"), anyRest))
	sm.addKeyword("Commit", "COMMIT", Seq(Str("COMMIT"), anyRest))
	sm.addKeyword("LeaveToTransaction", "LEAVE", Seq(Str("LEAVE"), Str("TO"), Str("TRANSACTION"), anyRest))
	sm.addKeyword("Leave", "LEAVE", Seq(Str("LEAVE"), anyRest))
	sm.addKeyword("Submit", "SUBMIT", Seq(Str("SUBMIT"), anyRest))
	sm.addKeyword("Sort", "SORT", Seq(Str("SORT"), anyRest))
	sm.addKeyword("Assign", "ASSIGN", Seq(Str("ASSIGN"), anyRest))
	sm.addKeyword("Unassign", "UNASSIGN", Seq(Str("UNASSIGN"), anyRest))
	sm.addKeyword("Clear", "CLEAR", Seq(Str("CLEAR"), anyRest))
	sm.addKeyword("Refresh", "REFRESH", Seq(Str("REFRESH"), anyRest))
	sm.addKeyword("Append", "APPEND", Seq(Str("APPEND"), anyRest))
	sm.addKeyword("Condense", "CONDENSE", Seq(Str("CONDENSE"), anyRest))
	sm.addKeyword("Translate", "TRANSLATE", Seq(Str("TRANSLATE"), anyRest))
	sm.addKeyword("Replace", "REPLACE", Seq(Str("REPLACE"), anyRest))
	sm.addKeyword("Find", "FIND", Seq(Str("FIND"), anyRest))
	sm.addKeyword("Split", "SPLIT", Seq(Str("SPLIT"), anyRest))
	sm.addKeyword("Concatenate", "CONCATENATE", Seq(Str("CONCATENATE"), anyRest))
	sm.addKeyword("Write", "WRITE", Seq(Str("WRITE"), anyRest))
	sm.addKeyword("Message", "MESSAGE", Seq(Str("MESSAGE"), anyRest))
	sm.addKeyword("Add", "ADD", Seq(Str("ADD"), anyRest))
	sm.addKeyword("Perform", "PERFORM", Seq(Str("PERFORM"), anyRest))
	sm.addKeyword("SelectOption", "SELECT", Seq(Str("SELECT"), Tok(TokenDash), Str("OPTIONS"), anyRest))
	sm.addKeyword("Select", "SELECT", Seq(Str("SELECT"), anyRest))

	// DATA — distinguish from Move (lv_x = ...)
	sm.addKeyword("Data", "DATA", Seq(Str("DATA"), ident, anyRest))

	// TYPES — distinguish TypeBegin, TypeEnd, Type
	sm.addKeyword("TypeBegin", "TYPES", Seq(Str("TYPES"), Str("BEGIN"), Str("OF"), anyRest))
	sm.addKeyword("TypeEnd", "TYPES", Seq(Str("TYPES"), Str("END"), Str("OF"), anyRest))
	sm.addKeyword("Type", "TYPES", Seq(Str("TYPES"), ident, anyRest))

	// CONSTANTS
	sm.addKeyword("Constant", "CONSTANTS", Seq(Str("CONSTANTS"), anyRest))

	// CLASS — ClassDefinition vs ClassImplementation vs ClassDeferred vs ClassData
	sm.addKeyword("ClassDeferred", "CLASS", Seq(Str("CLASS"), ident, Str("DEFINITION"), Str("DEFERRED"), anyRest))
	sm.addKeyword("ClassDefinition", "CLASS", Seq(Str("CLASS"), ident, Str("DEFINITION"), anyRest))
	sm.addKeyword("ClassImplementation", "CLASS", Seq(Str("CLASS"), ident, Str("IMPLEMENTATION"), anyRest))
	sm.addKeyword("ClassData", "CLASS", Seq(Str("CLASS"), Tok(TokenDash), Str("DATA"), anyRest))
	// CLASS-METHODS → MethodDef
	sm.addKeyword("MethodDef", "CLASS", Seq(Str("CLASS"), Tok(TokenDash), Str("METHODS"), anyRest))

	// METHOD — MethodImplementation
	sm.addKeyword("MethodImplementation", "METHOD", Seq(Str("METHOD"), anyRest))

	// METHODS — MethodDef
	sm.addKeyword("MethodDef", "METHODS", Seq(Str("METHODS"), anyRest))

	// INTERFACE / INTERFACES
	sm.addKeyword("Interface", "INTERFACE", Seq(Str("INTERFACE"), ident, Str("PUBLIC"), anyRest))
	sm.addKeyword("Interface", "INTERFACE", Seq(Str("INTERFACE"), ident))
	sm.addKeyword("InterfaceDef", "INTERFACES", Seq(Str("INTERFACES"), anyRest))

	// FORM
	sm.addKeyword("Form", "FORM", Seq(Str("FORM"), anyRest))

	// FUNCTION
	sm.addKeyword("FunctionModule", "FUNCTION", Seq(Str("FUNCTION"), ident))
	sm.addKeyword("FunctionPool", "FUNCTION", Seq(Str("FUNCTION"), Tok(TokenDash), Str("POOL"), anyRest))

	// PUBLIC / PRIVATE / PROTECTED SECTION
	sm.addKeyword("Public", "PUBLIC", Seq(Str("PUBLIC"), Str("SECTION")))
	sm.addKeyword("Private", "PRIVATE", Seq(Str("PRIVATE"), Str("SECTION")))
	sm.addKeyword("Protected", "PROTECTED", Seq(Str("PROTECTED"), Str("SECTION")))

	// CREATE
	sm.addKeyword("CreateObject", "CREATE", Seq(Str("CREATE"), Str("OBJECT"), anyRest))
	sm.addKeyword("CreateData", "CREATE", Seq(Str("CREATE"), Str("DATA"), anyRest))

	// CALL
	sm.addKeyword("CallFunction", "CALL", Seq(Str("CALL"), Str("FUNCTION"), anyRest))
	sm.addKeyword("CallTransaction", "CALL", Seq(Str("CALL"), Str("TRANSACTION"), anyRest))
	sm.addKeyword("CallTransformation", "CALL", Seq(Str("CALL"), Str("TRANSFORMATION"), anyRest))
	sm.addKeyword("CallScreen", "CALL", Seq(Str("CALL"), Str("SCREEN"), anyRest))
	sm.addKeyword("CallSelectionScreen", "CALL", Seq(Str("CALL"), Str("SELECTION"), Tok(TokenDash), Str("SCREEN"), anyRest))

	// READ
	sm.addKeyword("ReadTable", "READ", Seq(Str("READ"), Str("TABLE"), anyRest))
	sm.addKeyword("ReadTextpool", "READ", Seq(Str("READ"), Str("TEXTPOOL"), anyRest))

	// INSERT
	sm.addKeyword("InsertTextpool", "INSERT", Seq(Str("INSERT"), Str("TEXTPOOL"), anyRest))
	sm.addKeyword("InsertInternal", "INSERT", Seq(Str("INSERT"), anyRest))

	// DELETE
	sm.addKeyword("DeleteInternal", "DELETE", Seq(Str("DELETE"), anyRest))

	// FIELD-SYMBOLS
	sm.addKeyword("FieldSymbol", "FIELD", Seq(Str("FIELD"), Tok(TokenDash), Str("SYMBOLS"), anyRest))

	// PARAMETERS / SELECT-OPTIONS / SELECTION-SCREEN
	sm.addKeyword("Parameter", "PARAMETERS", Seq(Str("PARAMETERS"), anyRest))
	sm.addKeyword("SelectionScreen", "SELECTION", Seq(Str("SELECTION"), Tok(TokenDash), Str("SCREEN"), anyRest))

	// SET
	sm.addKeyword("SetPFStatus", "SET", Seq(Str("SET"), Str("PF"), Tok(TokenDash), Str("STATUS"), anyRest))
	sm.addKeyword("SetTitlebar", "SET", Seq(Str("SET"), Str("TITLEBAR"), anyRest))

	// GET TIME
	sm.addKeyword("GetTime", "GET", Seq(Str("GET"), Str("TIME"), anyRest))

	// MODULE / ENDMODULE
	sm.addKeyword("Module", "MODULE", Seq(Str("MODULE"), anyRest))

	// START-OF-SELECTION
	sm.addKeyword("StartOfSelection", "START", Seq(Str("START"), Tok(TokenDash), Str("OF"), Tok(TokenDash), Str("SELECTION")))

	// NativeSQL
	sm.addKeyword("NativeSQL", "DECLARE", Seq(Str("DECLARE"), anyRest))

	// --- Fallback matchers (for Move and Call) ---
	// Call and Move both start with identifiers. The key distinction:
	// - Call: method/function call without assignment (no standalone = token)
	// - Move: has an = assignment operator
	// We use a custom matcher that inspects tokens for = vs -> / =>
	sm.addFallback("Call", callMatcher())
	sm.addFallback("Move", Seq(AnyToken(), anyRest)) // ultimate fallback

	_ = source
}

// callMatcher returns a Matcher that detects method call statements.
// A Call statement is one that contains method invocations (-> => or function calls)
// but does NOT have a top-level assignment (= that isn't part of => or keyword params).
func callMatcher() Matcher {
	return func(tokens []Token, positions []int) []int {
		var result []int
		for _, pos := range positions {
			if pos < len(tokens) && isCallStatement(tokens[pos:]) {
				result = append(result, len(tokens)) // consume all
			}
		}
		return result
	}
}

func isCallStatement(tokens []Token) bool {
	if len(tokens) == 0 {
		return false
	}

	hasArrow := false
	hasParenCall := false
	parenDepth := 0

	isParenToken := func(t Token) bool {
		switch t.Type {
		case TokenParenLeft, TokenParenLeftW, TokenWParenLeft, TokenWParenLeftW:
			return true
		}
		return t.Str == "("
	}
	isParenRightToken := func(t Token) bool {
		switch t.Type {
		case TokenParenRight, TokenParenRightW, TokenWParenRight, TokenWParenRightW:
			return true
		}
		return t.Str == ")"
	}
	isArrowToken := func(t Token) bool {
		switch t.Type {
		case TokenInstanceArrow, TokenInstanceArrowW, TokenWInstanceArrow, TokenWInstanceArrowW,
			TokenStaticArrow, TokenStaticArrowW, TokenWStaticArrow, TokenWStaticArrowW:
			return true
		}
		return false
	}

	for i, tok := range tokens {
		switch {
		case isArrowToken(tok):
			hasArrow = true
		case isParenToken(tok):
			parenDepth++
			hasParenCall = true
		case isParenRightToken(tok):
			if parenDepth > 0 {
				parenDepth--
			}
		case parenDepth == 0 && (tok.Str == "=" || tok.Str == "?="):
			// Check = is not part of =>
			if tok.Str == "=" && i+1 < len(tokens) && tokens[i+1].Str == ">" {
				continue
			}
			// Top-level assignment → Move, not Call
			return false
		}
	}

	return hasArrow || hasParenCall
}

// ClassifyStatements applies the matcher to all statements in a slice.
func (sm *StatementMatcher) ClassifyStatements(stmts []Statement) {
	for i := range stmts {
		if stmts[i].Type == "Unknown" {
			stmts[i].Type = sm.Classify(&stmts[i])
		}
	}
}
