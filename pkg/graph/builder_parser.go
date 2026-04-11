package graph

import (
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/abaplint"
)

// ExtractDepsFromSource extracts dependency edges from ABAP source code
// using the native Go lexer+parser. This works completely offline — no SAP needed.
//
// The parser recognizes statement types and extracts referenced object names:
//   - CALL FUNCTION 'FM_NAME'       → CALLS edge to FUGR
//   - SUBMIT PROG_NAME              → CALLS edge to PROG
//   - PERFORM sub IN PROGRAM prog   → CALLS edge to PROG
//   - CREATE OBJECT TYPE zcl        → REFERENCES edge to CLAS
//   - DATA x TYPE REF TO zcl        → REFERENCES edge to CLAS
//   - INCLUDE zincl                  → CONTAINS_INCLUDE edge
//   - SELECT ... FROM ztable        → REFERENCES edge to TABL
//   - lo->method( ) / zcl=>method() → CALLS edge (from Call statements)
func ExtractDepsFromSource(source string, sourceNodeID string) []*Edge {
	// Lex + parse
	lexer := &abaplint.Lexer{}
	tokens := lexer.Run(source)

	parser := &abaplint.StatementParser{}
	stmts := parser.Parse(tokens)

	matcher := abaplint.NewStatementMatcher()
	matcher.ClassifyStatements(stmts)

	var edges []*Edge

	for _, stmt := range stmts {
		if stmt.Type == "Comment" || stmt.Type == "Empty" || stmt.Type == "Unknown" {
			continue
		}

		extracted := extractFromStatement(stmt, sourceNodeID)
		edges = append(edges, extracted...)
	}

	return edges
}

// extractFromStatement extracts dependency edges from a single classified statement.
func extractFromStatement(stmt abaplint.Statement, sourceNodeID string) []*Edge {
	toks := stmt.Tokens
	if len(toks) < 2 {
		return nil
	}

	switch stmt.Type {
	case "CallFunction":
		return extractCallFunction(toks, sourceNodeID)
	case "Submit":
		return extractSubmit(toks, sourceNodeID)
	case "Perform":
		return extractPerform(toks, sourceNodeID)
	case "CreateObject":
		return extractCreateObject(toks, sourceNodeID)
	case "Include":
		return extractInclude(toks, sourceNodeID)
	case "Select":
		return extractSelect(toks, sourceNodeID)
	case "Data":
		return extractTypeRef(toks, sourceNodeID)
	case "Type":
		return extractTypeRef(toks, sourceNodeID)
	case "Constant":
		return extractTypeRef(toks, sourceNodeID)
	case "InterfaceDef":
		return extractInterfaceDef(toks, sourceNodeID)
	case "ClassDefinition":
		return extractClassDef(toks, sourceNodeID)
	case "Call":
		return extractMethodCall(toks, sourceNodeID)
	case "Raise":
		return extractRaise(toks, sourceNodeID)
	case "CallTransaction":
		return extractCallTransaction(toks, sourceNodeID)
	case "LeaveToTransaction":
		return extractLeaveToTransaction(toks, sourceNodeID)
	case "CallTransformation":
		return extractCallTransformation(toks, sourceNodeID)
	}
	return nil
}

// CALL FUNCTION 'FM_NAME' ...
func extractCallFunction(toks []abaplint.Token, from string) []*Edge {
	for i, t := range toks {
		if strings.EqualFold(t.Str, "FUNCTION") && i+1 < len(toks) {
			name := unquote(toks[i+1].Str)
			if name != "" {
				return []*Edge{{
					From:      from,
					To:        NodeID("FUGR", fmToFugrName(name)),
					Kind:      EdgeCalls,
					Source:    SourceParser,
					RefDetail: "FM:" + name,
				}}
			}
		}
	}
	return nil
}

// SUBMIT prog_name ...
func extractSubmit(toks []abaplint.Token, from string) []*Edge {
	if len(toks) >= 2 {
		name := toks[1].Str
		if isIdentifier(name) && isCustomName(name) {
			return []*Edge{{
				From:      from,
				To:        NodeID("PROG", name),
				Kind:      EdgeCalls,
				Source:    SourceParser,
				RefDetail: "SUBMIT:" + name,
			}}
		}
	}
	return nil
}

// PERFORM sub IN PROGRAM prog
func extractPerform(toks []abaplint.Token, from string) []*Edge {
	for i, t := range toks {
		if strings.EqualFold(t.Str, "PROGRAM") && i+1 < len(toks) {
			name := toks[i+1].Str
			if isIdentifier(name) && isCustomName(name) {
				return []*Edge{{
					From:      from,
					To:        NodeID("PROG", name),
					Kind:      EdgeCalls,
					Source:    SourceParser,
					RefDetail: "PERFORM_IN:" + name,
				}}
			}
		}
	}
	return nil
}

// CREATE OBJECT lo TYPE zcl_foo
func extractCreateObject(toks []abaplint.Token, from string) []*Edge {
	for i, t := range toks {
		if strings.EqualFold(t.Str, "TYPE") && i+1 < len(toks) {
			name := toks[i+1].Str
			if isIdentifier(name) && isCustomName(name) {
				return []*Edge{{
					From:      from,
					To:        NodeID("CLAS", name),
					Kind:      EdgeReferences,
					Source:    SourceParser,
					RefDetail: "CREATE_OBJECT:" + name,
				}}
			}
		}
	}
	return nil
}

// INCLUDE zinclude_name
func extractInclude(toks []abaplint.Token, from string) []*Edge {
	if len(toks) >= 2 {
		name := toks[1].Str
		if isIdentifier(name) {
			return []*Edge{{
				From:      from,
				To:        NodeID("PROG", name),
				Kind:      EdgeContainsInclude,
				Source:    SourceParser,
				RefDetail: "INCLUDE:" + name,
			}}
		}
	}
	return nil
}

// SELECT ... FROM ztable ...
func extractSelect(toks []abaplint.Token, from string) []*Edge {
	for i, t := range toks {
		if strings.EqualFold(t.Str, "FROM") && i+1 < len(toks) {
			name := toks[i+1].Str
			if isIdentifier(name) && isCustomName(name) {
				return []*Edge{{
					From:      from,
					To:        NodeID("TABL", name),
					Kind:      EdgeReferences,
					Source:    SourceParser,
					RefDetail: "SELECT_FROM:" + name,
				}}
			}
		}
	}
	return nil
}

// DATA x TYPE REF TO zcl_foo  /  DATA x TYPE zcl_foo-component
func extractTypeRef(toks []abaplint.Token, from string) []*Edge {
	var edges []*Edge
	for i, t := range toks {
		if strings.EqualFold(t.Str, "TYPE") {
			// TYPE REF TO name
			if i+3 < len(toks) &&
				strings.EqualFold(toks[i+1].Str, "REF") &&
				strings.EqualFold(toks[i+2].Str, "TO") {
				name := toks[i+3].Str
				if isIdentifier(name) && isCustomName(name) {
					edges = append(edges, &Edge{
						From:      from,
						To:        NodeID("CLAS", name), // could be INTF too
						Kind:      EdgeReferences,
						Source:    SourceParser,
						RefDetail: "TYPE_REF_TO:" + name,
					})
				}
			} else if i+1 < len(toks) {
				// TYPE name or TYPE TABLE OF name
				idx := i + 1
				if idx < len(toks) && strings.EqualFold(toks[idx].Str, "TABLE") {
					idx++ // skip TABLE
					if idx < len(toks) && strings.EqualFold(toks[idx].Str, "OF") {
						idx++ // skip OF
					}
				}
				if idx < len(toks) {
					name := strings.Split(toks[idx].Str, "-")[0] // strip component
					if isIdentifier(name) && isCustomName(name) {
						edges = append(edges, &Edge{
							From:      from,
							To:        NodeID("TYPE", name),
							Kind:      EdgeReferences,
							Source:    SourceParser,
							RefDetail: "TYPE:" + name,
						})
					}
				}
			}
		}
	}
	return edges
}

// INTERFACES zif_foo
func extractInterfaceDef(toks []abaplint.Token, from string) []*Edge {
	if len(toks) >= 2 {
		name := toks[1].Str
		if isIdentifier(name) && isCustomName(name) {
			return []*Edge{{
				From:      from,
				To:        NodeID("INTF", name),
				Kind:      EdgeReferences,
				Source:    SourceParser,
				RefDetail: "IMPLEMENTS:" + name,
			}}
		}
	}
	return nil
}

// CLASS zcl_child DEFINITION ... INHERITING FROM zcl_parent
func extractClassDef(toks []abaplint.Token, from string) []*Edge {
	for i, t := range toks {
		if strings.EqualFold(t.Str, "INHERITING") && i+2 < len(toks) &&
			strings.EqualFold(toks[i+1].Str, "FROM") {
			name := toks[i+2].Str
			if isIdentifier(name) && isCustomName(name) {
				return []*Edge{{
					From:      from,
					To:        NodeID("CLAS", name),
					Kind:      EdgeReferences,
					Source:    SourceParser,
					RefDetail: "INHERITS:" + name,
				}}
			}
		}
	}
	return nil
}

// Method calls: lo->method( ), zcl_foo=>method( )
func extractMethodCall(toks []abaplint.Token, from string) []*Edge {
	var edges []*Edge
	for i, t := range toks {
		// Static call: ZCL_FOO=>METHOD
		if isStaticArrow(t) && i > 0 {
			name := toks[i-1].Str
			if isIdentifier(name) && isCustomName(name) {
				method := ""
				if i+1 < len(toks) {
					method = toks[i+1].Str
				}
				edges = append(edges, &Edge{
					From:      from,
					To:        NodeID("CLAS", name),
					Kind:      EdgeCalls,
					Source:    SourceParser,
					RefDetail: "STATIC_CALL:" + name + "=>" + method,
				})
			}
		}
	}
	return edges
}

// RAISE EXCEPTION TYPE zcx_error
func extractRaise(toks []abaplint.Token, from string) []*Edge {
	for i, t := range toks {
		if strings.EqualFold(t.Str, "TYPE") && i+1 < len(toks) {
			name := toks[i+1].Str
			if isIdentifier(name) && isCustomName(name) {
				return []*Edge{{
					From:      from,
					To:        NodeID("CLAS", name),
					Kind:      EdgeReferences,
					Source:    SourceParser,
					RefDetail: "RAISES:" + name,
				}}
			}
		}
	}
	return nil
}

// CALL TRANSACTION 'VA01' ...
func extractCallTransaction(toks []abaplint.Token, from string) []*Edge {
	for i, t := range toks {
		if strings.EqualFold(t.Str, "TRANSACTION") && i+1 < len(toks) {
			name := unquote(toks[i+1].Str)
			if name != "" {
				return []*Edge{{
					From:      from,
					To:        NodeID(NodeTRAN, name),
					Kind:      EdgeCalls,
					Source:    SourceParser,
					RefDetail: "CALL_TRANSACTION:" + name,
				}}
			}
		}
	}
	return nil
}

// LEAVE TO TRANSACTION 'SM30' ...
func extractLeaveToTransaction(toks []abaplint.Token, from string) []*Edge {
	for i, t := range toks {
		if strings.EqualFold(t.Str, "TRANSACTION") && i+1 < len(toks) {
			name := unquote(toks[i+1].Str)
			if name != "" {
				return []*Edge{{
					From:      from,
					To:        NodeID(NodeTRAN, name),
					Kind:      EdgeCalls,
					Source:    SourceParser,
					RefDetail: "LEAVE_TO_TRANSACTION:" + name,
				}}
			}
		}
	}
	return nil
}

// CALL TRANSFORMATION id SOURCE ... RESULT ...
func extractCallTransformation(toks []abaplint.Token, from string) []*Edge {
	for i, t := range toks {
		if strings.EqualFold(t.Str, "TRANSFORMATION") && i+1 < len(toks) {
			name := toks[i+1].Str
			// Can be a literal ('ID') or identifier
			cleaned := unquote(name)
			if cleaned != "" {
				name = cleaned
			}
			if isIdentifier(name) && !strings.EqualFold(name, "SOURCE") && !strings.EqualFold(name, "RESULT") {
				return []*Edge{{
					From:      from,
					To:        NodeID(NodeXSLT, name),
					Kind:      EdgeCalls,
					Source:    SourceParser,
					RefDetail: "CALL_TRANSFORMATION:" + name,
				}}
			}
		}
	}
	return nil
}

// --- Dynamic call detection ---
// Dynamic calls are invisible to CROSS/WBCROSSGT tables because the target
// is only known at runtime. The parser can detect the PATTERN and flag it.

// EdgeDynamic is a special edge kind for dynamic calls that cannot be
// statically resolved. These should be highlighted in boundary reports
// as potential hidden cross-package dependencies.
const EdgeDynamic EdgeKind = "DYNAMIC_CALL"

// extractDynamicCalls scans all tokens for dynamic call patterns.
// These are calls where the target is a variable, not a literal:
//   - CALL FUNCTION lv_variable       (vs CALL FUNCTION 'LITERAL')
//   - SUBMIT (lv_variable)            (vs SUBMIT ZPROG)
//   - PERFORM (lv_form) IN PROGRAM (lv_prog)
//   - CREATE OBJECT lo TYPE (lv_class)
//   - CALL METHOD (lv_variable)=>method
func ExtractDynamicCalls(source string, sourceNodeID string) []*Edge {
	lexer := &abaplint.Lexer{}
	tokens := lexer.Run(source)

	parser := &abaplint.StatementParser{}
	stmts := parser.Parse(tokens)

	matcher := abaplint.NewStatementMatcher()
	matcher.ClassifyStatements(stmts)

	var edges []*Edge

	for _, stmt := range stmts {
		toks := stmt.Tokens
		if len(toks) < 2 {
			continue
		}

		switch stmt.Type {
		case "CallFunction":
			// CALL FUNCTION <not-a-string-literal> → dynamic
			for i, t := range toks {
				if strings.EqualFold(t.Str, "FUNCTION") && i+1 < len(toks) {
					next := toks[i+1]
					if next.Str != "" && next.Str[0] != '\'' {
						edges = append(edges, &Edge{
							From:      sourceNodeID,
							To:        "DYNAMIC:" + next.Str,
							Kind:      EdgeDynamic,
							Source:    SourceParser,
							RefDetail: "DYNAMIC_FM:" + next.Str,
						})
					}
				}
			}
		case "Submit":
			// SUBMIT (variable) → dynamic
			if len(toks) >= 2 && toks[1].Str == "(" {
				varName := ""
				if len(toks) >= 3 {
					varName = toks[2].Str
				}
				edges = append(edges, &Edge{
					From:      sourceNodeID,
					To:        "DYNAMIC:" + varName,
					Kind:      EdgeDynamic,
					Source:    SourceParser,
					RefDetail: "DYNAMIC_SUBMIT:" + varName,
				})
			}
		case "Perform":
			// PERFORM sub IN PROGRAM (variable) → dynamic
			for i, t := range toks {
				if strings.EqualFold(t.Str, "PROGRAM") && i+1 < len(toks) {
					if toks[i+1].Str == "(" {
						varName := ""
						if i+2 < len(toks) {
							varName = toks[i+2].Str
						}
						edges = append(edges, &Edge{
							From:      sourceNodeID,
							To:        "DYNAMIC:" + varName,
							Kind:      EdgeDynamic,
							Source:    SourceParser,
							RefDetail: "DYNAMIC_PERFORM:" + varName,
						})
					}
				}
			}
		case "CreateObject":
			// CREATE OBJECT lo TYPE (variable)
			for i, t := range toks {
				if strings.EqualFold(t.Str, "TYPE") && i+1 < len(toks) {
					if toks[i+1].Str == "(" {
						varName := ""
						if i+2 < len(toks) {
							varName = toks[i+2].Str
						}
						edges = append(edges, &Edge{
							From:      sourceNodeID,
							To:        "DYNAMIC:" + varName,
							Kind:      EdgeDynamic,
							Source:    SourceParser,
							RefDetail: "DYNAMIC_CREATE:" + varName,
						})
					}
				}
			}
		case "CallTransaction":
			// CALL TRANSACTION lv_variable (not a literal)
			for i, t := range toks {
				if strings.EqualFold(t.Str, "TRANSACTION") && i+1 < len(toks) {
					next := toks[i+1]
					if next.Str != "" && next.Str[0] != '\'' {
						edges = append(edges, &Edge{
							From:      sourceNodeID,
							To:        "DYNAMIC:" + next.Str,
							Kind:      EdgeDynamic,
							Source:    SourceParser,
							RefDetail: "DYNAMIC_TRANSACTION:" + next.Str,
						})
					}
				}
			}
		case "CallTransformation":
			// CALL TRANSFORMATION (lv_variable) or CALL TRANSFORMATION lv_var
			for i, t := range toks {
				if strings.EqualFold(t.Str, "TRANSFORMATION") && i+1 < len(toks) {
					next := toks[i+1]
					if next.Str == "(" {
						varName := ""
						if i+2 < len(toks) {
							varName = toks[i+2].Str
						}
						edges = append(edges, &Edge{
							From:      sourceNodeID,
							To:        "DYNAMIC:" + varName,
							Kind:      EdgeDynamic,
							Source:    SourceParser,
							RefDetail: "DYNAMIC_TRANSFORMATION:" + varName,
						})
					}
				}
			}
		}
	}

	return edges
}

// --- helpers ---

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '/') {
			return false
		}
	}
	return true
}

func isCustomName(name string) bool {
	upper := strings.ToUpper(name)
	return len(upper) > 0 && (upper[0] == 'Z' || upper[0] == 'Y')
}

func isStaticArrow(t abaplint.Token) bool {
	return t.Str == "=>" || strings.Contains(strings.ToLower(t.Type.String()), "static")
}

// fmToFugrName extracts the function group name from a function module name.
// Convention: Z_FUGR_FM_NAME → hard to reverse reliably, so we keep FM name.
func fmToFugrName(fmName string) string {
	return fmName // Keep as-is; TADIR resolution will fix it
}
