package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// Variants, function test data, documentation and the IMG: what a system is
// set up to do, for an agent asked "what does this job's variant select",
// "how was this module tested", "where is that setting".

const noteNoLabels = "Fields carry no label: the program has no selection texts in this language, so the field names are what the screen shows too."

func (s *Server) handleVariants(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	report := firstString(args, "report", "program", "object_name")
	if report == "" {
		return newToolResultError("variants needs report: the program whose variants to read"), nil
	}
	lang := firstString(args, "language", "lang")
	if lang == "" {
		lang = s.adtClient.Language()
	}
	name := firstString(args, "variant", "name")
	if name == "" {
		list, err := s.adtClient.Variants(ctx, report, lang)
		if err != nil {
			return newToolResultError(fmt.Sprintf("Failed to list variants: %v", err)), nil
		}
		if list == nil {
			list = []adt.Variant{}
		}
		return newToolResultJSON(map[string]any{"report": strings.ToUpper(report), "variants": list, "count": len(list)}), nil
	}
	v, err := s.adtClient.Variant(ctx, report, name, lang)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to read variant: %v", err)), nil
	}
	out := map[string]any{"variant": v, "markdown": adt.VariantMarkdown(v)}
	labelled := false
	for _, f := range v.Fields {
		if f.Label != "" {
			labelled = true
		}
	}
	if !labelled && len(v.Fields) > 0 {
		out["notes"] = []string{noteNoLabels}
	}
	return newToolResultJSON(out), nil
}

func (s *Server) handleFunctionTestData(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	fm := firstString(args, "function", "function_module", "name", "object_name")
	if fm == "" {
		return newToolResultError("fm_test_data needs function: the function module"), nil
	}
	data, err := s.adtClient.FunctionTestData(ctx, fm)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to read test data: %v", err)), nil
	}
	return newToolResultJSON(map[string]any{"data": data, "markdown": adt.FunctionTestMarkdown(data)}), nil
}

func (s *Server) handleDocumentation(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id := firstString(args, "class", "id", "doc_class")
	object := firstString(args, "object", "name", "object_name")
	if object == "" {
		return newToolResultError("documentation needs object (and class: DE, RE, FU, CL, TB, NA, TX, HY ...; without it the index of what exists is returned)"), nil
	}
	lang := firstString(args, "language", "lang")
	if lang == "" {
		lang = s.adtClient.Language()
	}
	if id == "" {
		list, err := s.adtClient.DocumentationIndex(ctx, object)
		if err != nil {
			return newToolResultError(fmt.Sprintf("Failed to read the documentation index: %v", err)), nil
		}
		if list == nil {
			list = []adt.Documentation{}
		}
		return newToolResultJSON(map[string]any{"object": strings.ToUpper(object), "texts": list, "classes": adt.DocumentationClasses}), nil
	}
	doc, err := s.adtClient.Documentation(ctx, id, object, lang)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to read documentation: %v", err)), nil
	}
	doc.Lines = nil
	return newToolResultJSON(doc), nil
}

func (s *Server) handleIMGSearch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	text := firstString(args, "text", "query", "pattern")
	if text == "" {
		return newToolResultError("img_search needs text: a word or pattern (* wildcard) from the node's title"), nil
	}
	lang := firstString(args, "language", "lang")
	if lang == "" {
		lang = s.adtClient.Language()
	}
	limit := 0
	if n, ok := firstNumber(args, "max_results", "top", "limit"); ok && n > 0 {
		limit = int(n)
	}
	nodes, err := s.adtClient.IMGSearch(ctx, text, lang, limit)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to search the IMG: %v", err)), nil
	}
	if nodes == nil {
		nodes = []adt.IMGNode{}
	}
	return newToolResultJSON(map[string]any{"nodes": nodes, "count": len(nodes)}), nil
}

func (s *Server) handleIMGActivity(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	activity := firstString(args, "activity", "name", "object_name")
	if activity == "" {
		return newToolResultError("img_activity needs activity: the technical name from img_search"), nil
	}
	lang := firstString(args, "language", "lang")
	if lang == "" {
		lang = s.adtClient.Language()
	}
	a, err := s.adtClient.IMGActivity(ctx, activity, lang)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to read IMG activity: %v", err)), nil
	}
	return newToolResultJSON(a), nil
}
