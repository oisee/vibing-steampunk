// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_context.go contains handlers for dependency context compression,
// ABAP parsing, and unified code analysis.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/ctxcomp"
)

// routeContextAction routes "analyze" with type=context, parse_abap, analyze_deps.
func (s *Server) routeContextAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "analyze" {
		return nil, false, nil
	}
	switch getStringParam(params, "type") {
	case "context":
		return s.callHandler(ctx, s.handleGetContext, params)
	case "parse_abap":
		return s.callHandler(ctx, s.handleParseABAP, params)
	case "analyze_deps":
		return s.callHandler(ctx, s.handleAnalyzeDeps, params)
	}
	return nil, false, nil
}

// adtSourceAdapter adapts adt.Client to the ctxcomp.ADTSourceFetcher interface.
type adtSourceAdapter struct {
	server *Server
}

func (a *adtSourceAdapter) GetSource(ctx context.Context, objectType, name string, opts interface{}) (string, error) {
	return a.server.adtClient.GetSource(ctx, objectType, name, nil)
}

// handleParseABAP tokenizes and parses ABAP source into structured statements.
// Uses Go-side parser (equivalent to the transpiled abaplint lexer on SAP).
func (s *Server) handleParseABAP(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	source, _ := request.Params.Arguments["source"].(string)
	objectType, _ := request.Params.Arguments["object_type"].(string)
	name, _ := request.Params.Arguments["name"].(string)

	// Fetch source if not provided
	if source == "" {
		if objectType == "" || name == "" {
			return newToolResultError("Either 'source' or 'object_type'+'name' is required"), nil
		}
		var err error
		source, err = s.adtClient.GetSource(ctx, objectType, name, nil)
		if err != nil {
			return newToolResultError(fmt.Sprintf("Failed to fetch source: %v", err)), nil
		}
	}

	// Parse using Go-side tokenizer (same logic as zcl_lexer on SAP)
	lines := strings.Split(source, "\n")

	type Statement struct {
		Type   string  `json:"type"` // UNKNOWN, COMMENT, EMPTY, DATA, IF, LOOP, SELECT, METHOD, CLASS, etc.
		Tokens []abapToken `json:"tokens"`
		First  string  `json:"first"` // first keyword
	}

	var statements []Statement
	var currentTokens []abapToken
	inComment := false

	for lineIdx, line := range lines {
		row := lineIdx + 1
		trimmed := strings.TrimSpace(line)

		// Full-line comment
		if strings.HasPrefix(trimmed, "*") {
			if len(currentTokens) > 0 {
				statements = append(statements, Statement{Type: "UNKNOWN", Tokens: currentTokens, First: currentTokens[0].Str})
				currentTokens = nil
			}
			statements = append(statements, Statement{Type: "COMMENT", Tokens: []abapToken{{Str: trimmed, Type: "comment", Row: row}}, First: "*"})
			continue
		}

		// Inline comment
		if strings.HasPrefix(trimmed, "\"") {
			if len(currentTokens) > 0 {
				statements = append(statements, Statement{Type: "UNKNOWN", Tokens: currentTokens, First: currentTokens[0].Str})
				currentTokens = nil
			}
			statements = append(statements, Statement{Type: "COMMENT", Tokens: []abapToken{{Str: trimmed, Type: "comment", Row: row}}, First: "\""})
			continue
		}
		_ = inComment

		// Tokenize the line (simple word splitter — respects strings)
		words := tokenizeLine(trimmed)
		for _, w := range words {
			tokType := "identifier"
			if w == "." || w == "," || w == ":" {
				tokType = "punctuation"
			} else if strings.HasPrefix(w, "'") || strings.HasPrefix(w, "`") || strings.HasPrefix(w, "|") {
				tokType = "string"
			} else if w == "->" || w == "=>" {
				tokType = "arrow"
			}

			currentTokens = append(currentTokens, abapToken{Str: w, Type: tokType, Row: row})

			// Statement end
			if w == "." {
				stmtType := classifyStatement(currentTokens)
				first := ""
				if len(currentTokens) > 0 {
					first = strings.ToUpper(currentTokens[0].Str)
				}
				statements = append(statements, Statement{Type: stmtType, Tokens: currentTokens, First: first})
				currentTokens = nil
			}
		}
	}

	// Remaining tokens
	if len(currentTokens) > 0 {
		first := ""
		if len(currentTokens) > 0 {
			first = strings.ToUpper(currentTokens[0].Str)
		}
		statements = append(statements, Statement{Type: "UNKNOWN", Tokens: currentTokens, First: first})
	}

	// Build summary
	type ParseResult struct {
		Lines      int         `json:"lines"`
		Statements int         `json:"statements"`
		Stmts      []Statement `json:"stmts"`
	}

	result := ParseResult{
		Lines:      len(lines),
		Statements: len(statements),
		Stmts:      statements,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// handleAnalyzeDeps runs the unified 5-layer analyzer.
func (s *Server) handleAnalyzeDeps(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	source, _ := request.Params.Arguments["source"].(string)
	objectType, _ := request.Params.Arguments["object_type"].(string)
	name, _ := request.Params.Arguments["name"].(string)

	// Fetch source if not provided
	if source == "" {
		if objectType == "" || name == "" {
			return newToolResultError("Either 'source' or 'object_type'+'name' is required"), nil
		}
		var err error
		source, err = s.adtClient.GetSource(ctx, objectType, name, nil)
		if err != nil {
			return newToolResultError(fmt.Sprintf("Failed to fetch source: %v", err)), nil
		}
	}

	if name == "" {
		name = "UNKNOWN"
	}

	// Run unified analyzer (offline layers: regex + parser validation)
	analyzer := ctxcomp.NewAnalyzer(nil)
	result := analyzer.Analyze(ctx, source, name)

	// Build output
	type DepOut struct {
		Name       string   `json:"name"`
		Kind       string   `json:"kind"`
		Confidence float64  `json:"confidence"`
		FoundBy    []string `json:"found_by"`
		Line       int      `json:"line,omitempty"`
		Suspect    bool     `json:"suspect,omitempty"`
	}

	type AnalysisOut struct {
		Object         string   `json:"object"`
		Lines          int      `json:"lines"`
		TotalDeps      int      `json:"total_deps"`
		ConfirmedDeps  int      `json:"confirmed_deps"`
		FalsePositives int      `json:"false_positives"`
		Layers         []string `json:"layers"`
		DurationMs     float64  `json:"duration_ms"`
		Dependencies   []DepOut `json:"dependencies"`
	}

	out := AnalysisOut{
		Object:         name,
		Lines:          result.TotalLines,
		TotalDeps:      len(result.Dependencies),
		ConfirmedDeps:  result.TrueDeps,
		FalsePositives: result.FalsePositives,
		DurationMs:     float64(result.Duration.Microseconds()) / 1000.0,
	}

	for _, l := range result.Layers {
		out.Layers = append(out.Layers, l.String())
	}

	for _, d := range result.Dependencies {
		dep := DepOut{
			Name:       d.Name,
			Kind:       string(d.Kind),
			Confidence: d.Confidence,
			Line:       d.Line,
			Suspect:    d.InString || d.InComment,
		}
		for _, l := range d.FoundBy {
			dep.FoundBy = append(dep.FoundBy, l.String())
		}
		out.Dependencies = append(out.Dependencies, dep)
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// tokenizeLine splits an ABAP line into tokens, respecting string literals.
func tokenizeLine(line string) []string {
	var tokens []string
	var current strings.Builder
	inString := false
	stringChar := byte(0)

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if inString {
			current.WriteByte(ch)
			if ch == stringChar {
				// Check for doubled quote (escape)
				if i+1 < len(line) && line[i+1] == stringChar {
					current.WriteByte(line[i+1])
					i++
					continue
				}
				inString = false
				flush()
			}
			continue
		}

		switch ch {
		case '\'', '`':
			flush()
			inString = true
			stringChar = ch
			current.WriteByte(ch)
		case '|':
			flush()
			// String template — find matching |
			current.WriteByte(ch)
			for i++; i < len(line); i++ {
				current.WriteByte(line[i])
				if line[i] == '|' && (i == 0 || line[i-1] != '\\') {
					break
				}
			}
			flush()
		case ' ', '\t':
			flush()
		case '.', ',', ':', '(', ')', '[', ']':
			flush()
			tokens = append(tokens, string(ch))
		case '-':
			if i+1 < len(line) && line[i+1] == '>' {
				flush()
				tokens = append(tokens, "->")
				i++
			} else {
				current.WriteByte(ch)
			}
		case '=':
			if i+1 < len(line) && line[i+1] == '>' {
				flush()
				tokens = append(tokens, "=>")
				i++
			} else {
				flush()
				tokens = append(tokens, "=")
			}
		default:
			current.WriteByte(ch)
		}
	}
	flush()
	return tokens
}

// abapToken is used by the parser output.
type abapToken struct {
	Str  string `json:"str"`
	Type string `json:"type"`
	Row  int    `json:"row"`
}

// classifyStatement determines the statement type from its tokens.
func classifyStatement(tokens []abapToken) string {
	if len(tokens) == 0 {
		return "EMPTY"
	}
	if len(tokens) == 1 && tokens[0].Str == "." {
		return "EMPTY"
	}

	first := strings.ToUpper(tokens[0].Str)
	switch first {
	case "DATA", "TYPES", "CONSTANTS", "STATICS", "FIELD-SYMBOLS", "TABLES":
		return "DATA"
	case "CLASS":
		if len(tokens) > 2 {
			third := strings.ToUpper(tokens[2].Str)
			if third == "DEFINITION" {
				return "CLASS_DEFINITION"
			} else if third == "IMPLEMENTATION" {
				return "CLASS_IMPLEMENTATION"
			}
		}
		return "CLASS"
	case "ENDCLASS":
		return "ENDCLASS"
	case "METHOD":
		return "METHOD"
	case "ENDMETHOD":
		return "ENDMETHOD"
	case "INTERFACE":
		return "INTERFACE"
	case "ENDINTERFACE":
		return "ENDINTERFACE"
	case "IF", "ELSEIF":
		return "IF"
	case "ELSE":
		return "ELSE"
	case "ENDIF":
		return "ENDIF"
	case "DO", "WHILE":
		return "LOOP"
	case "ENDDO", "ENDWHILE":
		return "ENDLOOP"
	case "LOOP":
		return "LOOP"
	case "ENDLOOP":
		return "ENDLOOP"
	case "SELECT", "UPDATE", "INSERT", "DELETE", "MODIFY":
		return "SQL"
	case "WRITE", "MESSAGE":
		return "OUTPUT"
	case "CALL":
		if len(tokens) > 1 {
			second := strings.ToUpper(tokens[1].Str)
			if second == "FUNCTION" {
				return "CALL_FUNCTION"
			} else if second == "METHOD" {
				return "CALL_METHOD"
			}
		}
		return "CALL"
	case "FORM":
		return "FORM"
	case "ENDFORM":
		return "ENDFORM"
	case "PERFORM":
		return "PERFORM"
	case "REPORT", "PROGRAM":
		return "REPORT"
	case "FUNCTION-POOL":
		return "FUNCTION_POOL"
	case "FUNCTION":
		return "FUNCTION"
	case "ENDFUNCTION":
		return "ENDFUNCTION"
	case "TRY":
		return "TRY"
	case "CATCH":
		return "CATCH"
	case "ENDTRY":
		return "ENDTRY"
	case "RAISE":
		return "RAISE"
	case "RETURN":
		return "RETURN"
	case "APPEND", "READ", "SORT", "CLEAR", "FREE", "REFRESH":
		return "ITAB"
	case "MOVE", "COMPUTE", "ADD", "SUBTRACT", "MULTIPLY", "DIVIDE":
		return "COMPUTE"
	case "INCLUDE":
		return "INCLUDE"
	case "PUBLIC", "PRIVATE", "PROTECTED":
		return "SECTION"
	case "METHODS", "CLASS-METHODS", "EVENTS", "CLASS-EVENTS", "ALIASES":
		return "DECLARATION"
	case "INHERITING", "INTERFACES", "CREATE":
		return "CLASS_OPTION"
	case "CHECK", "ASSERT":
		return "CHECK"
	case "EXPORT", "IMPORT":
		return "MEMORY"
	}

	return "UNKNOWN"
}

func (s *Server) handleGetContext(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectType, ok := request.Params.Arguments["object_type"].(string)
	if !ok || objectType == "" {
		return newToolResultError("object_type is required"), nil
	}

	name, ok := request.Params.Arguments["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}

	source, _ := request.Params.Arguments["source"].(string)
	maxDeps := 20
	if md, ok := request.Params.Arguments["max_deps"].(float64); ok && md > 0 {
		maxDeps = int(md)
	}
	depth := 1
	if d, ok := request.Params.Arguments["depth"].(float64); ok && d > 0 {
		depth = int(d)
	}

	// Fetch source from SAP if not provided
	if source == "" {
		var err error
		source, err = s.adtClient.GetSource(ctx, objectType, name, nil)
		if err != nil {
			return newToolResultError(fmt.Sprintf("GetContext: failed to fetch source for %s %s: %v", objectType, name, err)), nil
		}
	}

	provider := ctxcomp.NewMultiSourceProvider("", &adtSourceAdapter{server: s})
	compressor := ctxcomp.NewCompressor(provider, maxDeps).WithDepth(depth)
	result, err := compressor.Compress(ctx, source, name, objectType)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetContext failed: %v", err)), nil
	}

	if result.Prologue == "" {
		return mcp.NewToolResultText(fmt.Sprintf("No resolvable dependencies found for %s %s", objectType, name)), nil
	}

	// Append stats
	output := fmt.Sprintf("%s\n* Stats: %d deps found, %d resolved, %d failed, %d lines",
		result.Prologue, result.Stats.DepsFound, result.Stats.DepsResolved, result.Stats.DepsFailed, result.Stats.TotalLines)

	return mcp.NewToolResultText(output), nil
}
