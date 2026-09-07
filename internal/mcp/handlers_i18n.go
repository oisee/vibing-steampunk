// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_i18n.go contains handlers for translation/internationalization operations.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- i18n Handlers ---

func (s *Server) handleGetObjectTextsInLanguage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.GetArguments()["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	lang, ok := request.GetArguments()["language"].(string)
	if !ok || lang == "" {
		return newToolResultError("language is required"), nil
	}

	content, err := s.adtClient.GetObjectTextsInLanguage(ctx, objectURL, lang)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetObjectTextsInLanguage failed: %v", err)), nil
	}

	return mcp.NewToolResultText(content), nil
}

func (s *Server) handleGetDataElementLabels(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := request.GetArguments()["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}

	lang, ok := request.GetArguments()["language"].(string)
	if !ok || lang == "" {
		return newToolResultError("language is required"), nil
	}

	labels, err := s.adtClient.GetDataElementLabels(ctx, name, lang)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetDataElementLabels failed: %v", err)), nil
	}

	jsonBytes, err := json.MarshalIndent(labels, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to format result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

func (s *Server) handleGetMessageClassTexts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := request.GetArguments()["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}

	lang, ok := request.GetArguments()["language"].(string)
	if !ok || lang == "" {
		return newToolResultError("language is required"), nil
	}

	texts, err := s.adtClient.GetMessageClassTexts(ctx, name, lang)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetMessageClassTexts failed: %v", err)), nil
	}

	if len(texts) == 0 {
		return mcp.NewToolResultText("No messages found."), nil
	}

	jsonBytes, err := json.MarshalIndent(texts, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to format result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

func (s *Server) handleWriteMessageClassTexts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := request.GetArguments()["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}

	lang, ok := request.GetArguments()["language"].(string)
	if !ok || lang == "" {
		return newToolResultError("language is required"), nil
	}

	lockHandle, ok := request.GetArguments()["lock_handle"].(string)
	if !ok || lockHandle == "" {
		return newToolResultError("lock_handle is required"), nil
	}

	transport, _ := request.GetArguments()["transport"].(string)

	// Parse texts from arguments
	textsRaw, ok := request.GetArguments()["texts"]
	if !ok || textsRaw == nil {
		return newToolResultError("texts is required"), nil
	}

	textsJSON, err := json.Marshal(textsRaw)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to parse texts: %v", err)), nil
	}

	var texts []adt.MessageClassMessage
	if err := json.Unmarshal(textsJSON, &texts); err != nil {
		return newToolResultError(fmt.Sprintf("Failed to parse texts: %v", err)), nil
	}

	err = s.adtClient.WriteMessageClassTexts(ctx, name, lang, texts, lockHandle, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("WriteMessageClassTexts failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Message class %s texts updated successfully in language %s.", name, lang)), nil
}

func (s *Server) handleWriteDataElementLabels(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := request.GetArguments()["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}

	lang, ok := request.GetArguments()["language"].(string)
	if !ok || lang == "" {
		return newToolResultError("language is required"), nil
	}

	lockHandle, ok := request.GetArguments()["lock_handle"].(string)
	if !ok || lockHandle == "" {
		return newToolResultError("lock_handle is required"), nil
	}

	transport, _ := request.GetArguments()["transport"].(string)

	labels := &adt.DataElementLabels{}
	if short, ok := request.GetArguments()["short"].(string); ok {
		labels.Short = short
	}
	if medium, ok := request.GetArguments()["medium"].(string); ok {
		labels.Medium = medium
	}
	if long, ok := request.GetArguments()["long"].(string); ok {
		labels.Long = long
	}
	if heading, ok := request.GetArguments()["heading"].(string); ok {
		labels.Heading = heading
	}

	err := s.adtClient.WriteDataElementLabels(ctx, name, lang, labels, lockHandle, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("WriteDataElementLabels failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Data element %s labels updated successfully in language %s.", name, lang)), nil
}

func (s *Server) handleGetTextPoolInLanguage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	programName, ok := request.GetArguments()["program_name"].(string)
	if !ok || programName == "" {
		return newToolResultError("program_name is required"), nil
	}

	lang, ok := request.GetArguments()["language"].(string)
	if !ok || lang == "" {
		return newToolResultError("language is required"), nil
	}

	entries, err := s.adtClient.GetTextPoolInLanguage(ctx, programName, lang)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetTextPoolInLanguage failed: %v", err)), nil
	}

	if len(entries) == 0 {
		return mcp.NewToolResultText("No text pool entries found."), nil
	}

	jsonBytes, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to format result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

func (s *Server) handleCompareObjectLanguages(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.GetArguments()["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	sourceLang, ok := request.GetArguments()["source_language"].(string)
	if !ok || sourceLang == "" {
		return newToolResultError("source_language is required"), nil
	}

	targetLang, ok := request.GetArguments()["target_language"].(string)
	if !ok || targetLang == "" {
		return newToolResultError("target_language is required"), nil
	}

	comparison, err := s.adtClient.CompareObjectLanguages(ctx, objectURL, sourceLang, targetLang)
	if err != nil {
		return newToolResultError(fmt.Sprintf("CompareObjectLanguages failed: %v", err)), nil
	}

	jsonBytes, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to format result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// handleWriteTextPool writes a program's texts of one kind: params
// program_name, language (logon language by default), kind (S selection
// texts by default, I symbols, H headings), texts as {KEY: text} or as
// "KEY=text" lines, transport for a transportable program. The lock is
// taken and released here; nothing to carry across calls.
func (s *Server) handleWriteTextPool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	program := getStringParam(args, "program_name")
	if program == "" {
		program = getStringParam(args, "program")
	}
	if program == "" {
		return newToolResultError("program_name is required"), nil
	}
	lang := getStringParam(args, "language")
	if lang == "" {
		lang = s.adtClient.Language()
	}
	kind := getStringParam(args, "kind")
	if kind == "" {
		kind = "S"
	}
	entries := map[string]string{}
	switch t := args["texts"].(type) {
	case map[string]any:
		for k, v := range t {
			entries[strings.ToUpper(k)] = fmt.Sprint(v)
		}
	case string:
		for _, line := range strings.Split(t, "\n") {
			if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) != "" {
				entries[strings.ToUpper(strings.TrimSpace(k))] = strings.TrimRight(v, "\r")
			}
		}
	}
	if len(entries) == 0 {
		return newToolResultError("texts is required: {\"P_DEVC\": \"Package to scan\"} or \"P_DEVC=Package to scan\" lines"), nil
	}
	if err := s.adtClient.WriteTextPool(ctx, program, lang, kind, entries, getStringParam(args, "transport")); err != nil {
		return newToolResultError(fmt.Sprintf("WriteTextPool failed: %v", err)), nil
	}
	return newToolResultJSON(map[string]any{"program": strings.ToUpper(program), "language": strings.ToUpper(lang), "kind": strings.ToUpper(kind), "written": len(entries)}), nil
}

// handleSyncTextPool takes the "~t: comments of a program's source as its
// selection texts and writes them; dry_run lists them instead.
func (s *Server) handleSyncTextPool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	program := getStringParam(args, "program_name")
	if program == "" {
		program = getStringParam(args, "program")
	}
	if program == "" {
		return newToolResultError("program_name is required"), nil
	}
	source, err := s.adtClient.GetSource(ctx, "PROG", strings.ToUpper(program), nil)
	if err != nil {
		return newToolResultError(fmt.Sprintf("reading the source: %v", err)), nil
	}
	entries := adt.SelectionTextsFromSource(source)
	if len(entries) == 0 {
		return newToolResultJSON(map[string]any{"program": strings.ToUpper(program), "found": 0, "notes": []string{"No \"~t: comments in the source. Put one after a PARAMETERS, SELECT-OPTIONS or named SELECTION-SCREEN COMMENT line: PARAMETERS p_devc TYPE tadir-devclass. \"~t: Package to scan"}}), nil
	}
	if dry, _ := getBoolParam(args, "dry_run"); dry {
		return newToolResultJSON(map[string]any{"program": strings.ToUpper(program), "found": len(entries), "texts": entries, "written": 0}), nil
	}
	lang := getStringParam(args, "language")
	if lang == "" {
		lang = s.adtClient.Language()
	}
	if err := s.adtClient.WriteTextPool(ctx, program, lang, "S", entries, getStringParam(args, "transport")); err != nil {
		return newToolResultError(fmt.Sprintf("WriteTextPool failed: %v", err)), nil
	}
	return newToolResultJSON(map[string]any{"program": strings.ToUpper(program), "language": strings.ToUpper(lang), "texts": entries, "written": len(entries)}), nil
}
