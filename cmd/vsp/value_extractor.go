package main

import (
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/abaplint"
)

// CodeLiteralCall captures one point in the source where the code supplies
// literal values that, taken together, identify a single customizing row.
// A CALL FUNCTION 'APPL_LOG_INIT' EXPORTING object = 'ZTEST_LOG' sub_object =
// 'EVENT' yields one CodeLiteralCall whose Fields map is
// {OBJECT:'ZTEST_LOG', SUBOBJECT:'EVENT'}. The value-level audit then checks
// whether that exact tuple is present among the transported rows of the
// Target table.
//
// v2a scope: two extractor paths, both narrow.
//
//   - CALL FUNCTION to a registered customizing FM (v2a-min path) —
//     handled by extractFromCallFunction.
//   - SELECT / UPDATE / MODIFY / DELETE FROM <static table> WHERE
//     <field> = 'LITERAL' [AND <field> = 'LITERAL'] (v2a.1) — handled
//     by extractFromSQL. Constraints follow the second-reviewer feedback:
//     only string literals, only `=`, no IN, no LIKE, no host variables,
//     no concatenation, no constants. The matcher additionally filters
//     extracted fields against DD03L key fields and drops findings whose
//     literals do not touch a key, so non-key WHERE conditions cannot
//     produce false positives.
//
// Interprocedural dataflow ("variable assigned a SELECT result, passed
// through method parameters") is deliberately out of scope for v2a.1 —
// it is the v2a.2 / v3 phase and needs its own design.
type CodeLiteralCall struct {
	SourceObject   string            `json:"source_object"` // e.g. "CLAS:ZCL_FOO"
	Target         string            `json:"target"`        // table name
	Fields         map[string]string `json:"fields"`        // key field → literal (both upper-cased)
	Row            int               `json:"row"`
	Via            string            `json:"via"`             // "CALL_FUNCTION:APPL_LOG_INIT"
	Kind           string            `json:"kind"`            // "known_call" for v2a-min; "direct_select" later
	IncompleteKey  bool              `json:"incomplete_key"`  // true if code supplied fewer fields than keyFields
}

// extractCodeLiterals runs the ABAP parser on the given source and returns
// every CodeLiteralCall it can extract. Two statement shapes are
// recognised; everything else is silently skipped.
//
//	CALL FUNCTION '<name>' EXPORTING p1 = 'LIT' p2 = 'LIT' ...
//	SELECT/UPDATE/MODIFY/DELETE ... FROM <static table> WHERE k1 = 'LIT' ...
//
// Dynamic FM calls (CALL FUNCTION lv_name) and dynamic table names
// (FROM (lv_table)) are skipped because we only want concrete literal
// keys, not runtime-computed ones. No regex: everything routes through
// pkg/abaplint's tokeniser and statement parser.
//
// Constant and host-var resolution: the extractor walks statements in
// source order. Before each CALL FUNCTION / SELECT is parsed, it has
// access to a running "literals seen so far" map built from every
// CONSTANTS / DATA / plain assignment that appeared above it in the
// same source unit. Each recognised assignment updates the map AFTER
// the current statement's extractor runs, so a variable never resolves
// to a value it only takes on later in the file.
//
// The scope is still coarse — file-wide, no method-boundary awareness,
// no branch awareness — so `lv_x = 'A'. IF cond. lv_x = 'B'. ENDIF.
// SELECT … WHERE k = lv_x.` will observe 'B' as the last value seen.
// Method-scoped resolution is a later refinement (v2a.2). What this
// pass DOES fix is the order-violation case: a variable used before
// it is assigned cannot be resolved to a future literal.
//
// The function is pure: it takes source plus the source object id, returns
// findings, and never talks to SAP. That keeps it unit-testable with inline
// ABAP fixtures.
func extractCodeLiterals(sourceObjectID, source string) []CodeLiteralCall {
	if strings.TrimSpace(source) == "" {
		return nil
	}
	file := abaplint.NewABAPFile(sourceObjectID, source)
	stmts := file.GetStatements()

	// Statement-order resolution: localLiterals grows as we walk
	// statements top-to-bottom. Each SELECT / CALL FUNCTION is parsed
	// against the map as it looked *before* this statement, so a
	// variable used before its defining assignment stays unresolved.
	// CONSTANTS / DATA / plain assignment statements update the map
	// AFTER they have been considered for extraction, so the update
	// only reaches subsequent statements.
	localLiterals := map[string]string{}

	var out []CodeLiteralCall
	for _, stmt := range stmts {
		if len(stmt.Tokens) == 0 {
			continue
		}
		first := strings.ToUpper(stmt.Tokens[0].Str)
		switch first {
		case "CALL":
			if c := extractFromCallFunction(stmt, sourceObjectID, localLiterals); c != nil {
				out = append(out, *c)
			}
		case "SELECT", "UPDATE", "MODIFY", "DELETE":
			if c := extractFromSQL(stmt, sourceObjectID, localLiterals); c != nil {
				out = append(out, *c)
			}
		}
		// Update the running literal map from this statement's own
		// declarations/assignments. Extraction of this statement has
		// already happened above, so the update only affects what
		// comes AFTER this line — matching real ABAP semantics.
		absorbLocalLiteral(stmt, localLiterals)
	}
	return out
}

// absorbLocalLiteral inspects one statement and, if it pins an
// identifier to a string literal value, updates the running
// localLiterals map so subsequent statements can resolve that
// identifier. Called AFTER the statement has been considered for
// literal extraction, so a variable defined here does not retroactively
// apply to earlier uses.
//
// Recognised shapes:
//
//	CONSTANTS gc_obj TYPE c LENGTH 10 VALUE 'ZTEST'.
//	CONSTANTS: gc_a VALUE 'A', gc_b VALUE 'B'.     (chained — first member only for now)
//	DATA      lv_obj TYPE string VALUE 'ZTEST'.
//	lv_obj = 'ZTEST'.                              (plain assignment)
//
// Anything more complex — concatenation, conversion, function call on
// the right side — is silently skipped; the caller never resolves an
// identifier it has not explicitly recorded.
func absorbLocalLiteral(stmt abaplint.Statement, localLiterals map[string]string) {
	if len(stmt.Tokens) < 3 {
		return
	}
	first := strings.ToUpper(stmt.Tokens[0].Str)
	switch first {
	case "CONSTANTS", "DATA":
		// Variable name is always the token immediately after the
		// CONSTANTS/DATA keyword (skipping a chain colon if present).
		// For chained declarations `CONSTANTS: a VALUE 'x', b VALUE 'y'.`
		// only the leading member gets captured for v2a.1; full chain
		// support is a later refinement.
		nameIdx := 1
		if stmt.Tokens[1].Str == ":" {
			nameIdx = 2
		}
		if nameIdx >= len(stmt.Tokens) {
			return
		}
		nameTok := stmt.Tokens[nameIdx]
		if nameTok.Type != abaplint.TokenIdentifier {
			return
		}
		for i := nameIdx + 1; i+1 < len(stmt.Tokens); i++ {
			if !strings.EqualFold(stmt.Tokens[i].Str, "VALUE") {
				continue
			}
			val := stmt.Tokens[i+1]
			if val.Type != abaplint.TokenString {
				return
			}
			localLiterals[strings.ToUpper(nameTok.Str)] = strings.ToUpper(unquoteLiteral(val.Str))
			return
		}
	default:
		// Plain assignment shape: `IDENT = 'LIT' .` (3-4 tokens).
		// First token is an identifier, second is `=`, third is a
		// string literal. Reject if the first token is a keyword.
		if len(stmt.Tokens) < 3 || stmt.Tokens[1].Str != "=" {
			return
		}
		if stmt.Tokens[0].Type != abaplint.TokenIdentifier {
			return
		}
		if stmt.Tokens[2].Type != abaplint.TokenString {
			return
		}
		name := strings.ToUpper(stmt.Tokens[0].Str)
		localLiterals[name] = strings.ToUpper(unquoteLiteral(stmt.Tokens[2].Str))
	}
}

// resolveLiteral returns the literal value for a given token: either the
// token's own string content if it's a TokenString, or the looked-up value
// from localLiterals if it's an identifier matching a recorded constant or
// host variable. Returns ("", false) if neither applies.
func resolveLiteral(tok abaplint.Token, localLiterals map[string]string) (string, bool) {
	if tok.Type == abaplint.TokenString {
		val := strings.ToUpper(unquoteLiteral(tok.Str))
		return val, val != ""
	}
	if tok.Type == abaplint.TokenIdentifier && localLiterals != nil {
		// Inline host-var prefix `@var` in modern ABAP SQL gets the `@`
		// as part of the identifier on some lexers; strip it for lookup.
		name := strings.ToUpper(strings.TrimPrefix(tok.Str, "@"))
		if val, ok := localLiterals[name]; ok && val != "" {
			return val, true
		}
	}
	return "", false
}

// extractFromCallFunction inspects a CALL FUNCTION statement. Returns nil
// for anything that does not match a known customizing FM, for dynamic
// (variable-named) FM calls, and for calls that supplied zero recognised
// literal EXPORTING parameters.
//
// Partial keys (code supplied some but not all keyFields from the registry)
// still produce a CodeLiteralCall with IncompleteKey=true — the match logic
// downstream uses subset semantics, and the flag lets the report call out
// that the finding is based on less than the full business key.
func extractFromCallFunction(stmt abaplint.Statement, sourceObjectID string, localLiterals map[string]string) *CodeLiteralCall {
	if len(stmt.Tokens) < 4 {
		return nil
	}
	if !strings.EqualFold(stmt.Tokens[1].Str, "FUNCTION") {
		return nil
	}
	nameTok := stmt.Tokens[2]
	if nameTok.Type != abaplint.TokenString {
		return nil // dynamic — no concrete FM name
	}
	fmName := strings.ToUpper(unquoteLiteral(nameTok.Str))
	kc, ok := knownCustCalls[fmName]
	if !ok {
		return nil
	}

	exportingIdx := -1
	for i, tok := range stmt.Tokens {
		if strings.EqualFold(tok.Str, "EXPORTING") {
			exportingIdx = i
			break
		}
	}
	if exportingIdx < 0 || exportingIdx+2 >= len(stmt.Tokens) {
		return nil
	}

	sectionEnd := map[string]bool{
		"IMPORTING":  true,
		"TABLES":     true,
		"CHANGING":   true,
		"EXCEPTIONS": true,
		"RECEIVING":  true,
	}

	fields := map[string]string{}
	i := exportingIdx + 1
	for i+2 < len(stmt.Tokens) {
		paramUp := strings.ToUpper(stmt.Tokens[i].Str)
		if sectionEnd[paramUp] || paramUp == "." {
			break
		}
		if stmt.Tokens[i+1].Str != "=" {
			i++
			continue
		}
		valTok := stmt.Tokens[i+2]
		// resolveLiteral covers both TokenString and identifier
		// substitution from the file-scoped localLiterals map (constants
		// and DATA … VALUE 'lit'). Anything we still can't resolve to a
		// literal — variables assigned at runtime, expressions, function
		// calls — is silently skipped at the per-parameter level. The
		// IncompleteKey flag below catches the resulting partial fill.
		literal, ok := resolveLiteral(valTok, localLiterals)
		if !ok {
			i += 3
			continue
		}
		paramLower := strings.ToLower(stmt.Tokens[i].Str)
		if tableField, mapped := kc.argMap[paramLower]; mapped {
			fields[tableField] = literal
		}
		i += 3
	}

	if len(fields) == 0 {
		return nil
	}

	// IncompleteKey: the registry declares which business-key fields matter
	// for the lookup. If the code filled fewer of them than declared, the
	// downstream match runs in subset mode and the finding is flagged so
	// the report can explain why one code call produced several candidate
	// transport rows.
	incomplete := false
	if len(kc.keyFields) > 0 {
		for _, kf := range kc.keyFields {
			if _, ok := fields[kf]; !ok {
				incomplete = true
				break
			}
		}
	}

	return &CodeLiteralCall{
		SourceObject:  sourceObjectID,
		Target:        kc.table,
		Fields:        fields,
		Row:           nameTok.Row,
		Via:           "CALL_FUNCTION:" + fmName,
		Kind:          "known_call",
		IncompleteKey: incomplete,
	}
}

// extractFromSQL inspects a SELECT / UPDATE / MODIFY / DELETE statement
// and returns a CodeLiteralCall when:
//
//   - The target table is statically named (no FROM (var) form).
//   - At least one WHERE predicate is exactly `<field> = '<literal>'`,
//     with both sides as concrete tokens (no host vars, no constants,
//     no concatenation).
//
// Anything more permissive — IN, LIKE, !=, host vars (@var or :var),
// constants, parameter binding — is silently ignored. The matcher
// downstream further filters extracted fields against DD03L key fields,
// so non-key WHERE conditions cannot produce a finding even if they
// happen to use a literal.
//
// Field names may be qualified as `table~field` or `table-field`; we
// strip the qualifier so the field name matches DD03L conventions.
//
// Multiple SELECT shapes are supported by entry keyword:
//
//	SELECT ... FROM <tab>      WHERE ...
//	UPDATE <tab> SET ...        WHERE ...
//	MODIFY <tab> FROM ...       WHERE ...
//	DELETE FROM <tab>           WHERE ...
//
// MODIFY and DELETE without WHERE are common in ABAP for itab operations
// (`MODIFY lt_foo`) — they get filtered out by the WHERE-required guard.
func extractFromSQL(stmt abaplint.Statement, sourceObjectID string, localLiterals map[string]string) *CodeLiteralCall {
	if len(stmt.Tokens) == 0 {
		return nil
	}
	keyword := strings.ToUpper(stmt.Tokens[0].Str)

	// Find FROM <table> or <verb> <table>, plus WHERE.
	fromIdx, whereIdx := -1, -1
	for i, tok := range stmt.Tokens {
		switch strings.ToUpper(tok.Str) {
		case "FROM":
			if fromIdx == -1 {
				fromIdx = i
			}
		case "WHERE":
			if whereIdx == -1 {
				whereIdx = i
			}
		}
	}

	if whereIdx < 0 {
		return nil // no WHERE → not value-level interesting (covers `MODIFY itab`)
	}

	// Determine the table name and its position. UPDATE/MODIFY without
	// FROM use the second token directly; DELETE FROM and SELECT use
	// the token after FROM.
	var table string
	switch keyword {
	case "SELECT":
		if fromIdx < 0 || fromIdx+1 >= len(stmt.Tokens) {
			return nil
		}
		table = stmt.Tokens[fromIdx+1].Str
	case "DELETE":
		if fromIdx < 0 || fromIdx+1 >= len(stmt.Tokens) {
			return nil
		}
		table = stmt.Tokens[fromIdx+1].Str
	case "UPDATE", "MODIFY":
		if len(stmt.Tokens) < 2 {
			return nil
		}
		table = stmt.Tokens[1].Str
	default:
		return nil
	}

	// Reject dynamic table names: `FROM (lv_table)` shows up as a `(`
	// token. Also reject host variables and structures used as a table.
	if table == "" || table == "(" {
		return nil
	}
	tableUp := strings.ToUpper(table)
	if !plausibleTableName(tableUp) {
		return nil
	}

	// Walk the WHERE body capturing `field = 'literal'` triples.
	// Stop at clause-ending keywords. AND/OR/NOT are skipped over so
	// chained predicates each get their own pass.
	clauseEnd := map[string]bool{
		"GROUP":     true,
		"ORDER":     true,
		"INTO":      true,
		"UP":        true,
		"APPENDING": true,
		"HAVING":    true,
		"BYPASSING": true,
	}
	fields := map[string]string{}
	i := whereIdx + 1
	for i+2 < len(stmt.Tokens) {
		fieldUp := strings.ToUpper(stmt.Tokens[i].Str)
		if clauseEnd[fieldUp] || fieldUp == "." {
			break
		}
		if fieldUp == "AND" || fieldUp == "OR" || fieldUp == "NOT" || fieldUp == "(" || fieldUp == ")" {
			i++
			continue
		}
		op := stmt.Tokens[i+1].Str
		if op != "=" {
			// Could be IN / LIKE / != / <> / <= / >= — none of these
			// produce single-value predicates. Advance past the next
			// token at minimum and resync.
			i += 2
			continue
		}
		// Modern ABAP SQL prefixes host variables with `@` which the
		// lexer reports as a separate WAt token. Step past it so the
		// resolver gets the actual identifier or literal token.
		valIdx := i + 2
		consumed := 3
		if stmt.Tokens[valIdx].Str == "@" {
			if valIdx+1 >= len(stmt.Tokens) {
				break
			}
			valIdx++
			consumed = 4
		}
		valTok := stmt.Tokens[valIdx]
		// Resolve via the file-scoped literal map: a TokenString gets
		// used directly; an identifier (constant, DATA … VALUE 'lit',
		// or a previously-assigned local variable) gets substituted.
		literal, ok := resolveLiteral(valTok, localLiterals)
		if !ok {
			i += consumed
			continue
		}
		fieldName := fieldUp
		// table~field or table-field — keep only the unqualified field name.
		if idx := strings.LastIndexAny(fieldName, "~-"); idx >= 0 {
			fieldName = fieldName[idx+1:]
		}
		// Reject anything that does not look like a DDIC field name —
		// the qualifier-strip above can leave punctuation behind on
		// pathological inputs.
		if !looksLikeFieldName(fieldName) {
			i += 3
			continue
		}
		if literal != "" {
			fields[fieldName] = literal
		}
		i += consumed
	}

	if len(fields) == 0 {
		return nil
	}

	return &CodeLiteralCall{
		SourceObject:  sourceObjectID,
		Target:        tableUp,
		Fields:        fields,
		Row:           stmt.Tokens[0].Row,
		Via:           keyword + ":" + tableUp,
		Kind:          "direct_select",
		IncompleteKey: false, // determined later by the matcher when DD03L keys are known
	}
}

// looksLikeFieldName cheaply guards against punctuation-only or empty
// field names that survived the qualifier-strip in extractFromSQL. Real
// DDIC field names are upper-case A-Z, 0-9 and underscore.
func looksLikeFieldName(s string) bool {
	if len(s) == 0 || len(s) > 30 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	return true
}

// unquoteLiteral strips the surrounding single quotes from an ABAP string
// literal token. The lexer preserves them verbatim in Token.Str for
// TokenString entries.
func unquoteLiteral(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}
