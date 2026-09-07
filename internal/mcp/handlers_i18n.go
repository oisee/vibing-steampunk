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

// textTargetFrom reads the object a text-pool op is about: program_name,
// or object_type + object_name (which the router fills from a target such
// as "PROG ZDEMO" or "CLAS ZCL_DEMO").
func textTargetFrom(args map[string]any) (adt.TextPoolTarget, error) {
	if p := getStringParam(args, "program_name"); p != "" {
		return adt.TextPoolTarget{Type: "PROG", Name: p}, nil
	}
	if p := getStringParam(args, "program"); p != "" {
		return adt.TextPoolTarget{Type: "PROG", Name: p}, nil
	}
	if c := getStringParam(args, "class_name"); c != "" {
		return adt.TextPoolTarget{Type: "CLAS", Name: c}, nil
	}
	name := getStringParam(args, "object_name")
	if name == "" {
		name = getStringParam(args, "name")
	}
	if name == "" {
		return adt.TextPoolTarget{}, fmt.Errorf("program_name (or class_name, or a target such as \"PROG ZDEMO\") is required")
	}
	return adt.TextPoolTarget{Type: getStringParam(args, "object_type"), Name: name}, nil
}

// handleTextsGet reads a text pool: op=texts_get (text_pool is an alias).
func (s *Server) handleTextsGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	target, err := textTargetFrom(args)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	lang := getStringParam(args, "language")
	if lang == "" {
		lang = s.adtClient.Language()
	}
	entries, err := s.adtClient.TextPool(ctx, target, lang)
	if err != nil {
		return newToolResultError(fmt.Sprintf("reading the text pool: %v", err)), nil
	}
	if entries == nil {
		entries = []adt.TextPoolEntry{}
	}
	return newToolResultJSON(map[string]any{"target": target.Type + " " + strings.ToUpper(target.Name), "language": strings.ToUpper(lang), "entries": entries, "count": len(entries)}), nil
}

// handleTextsSet writes texts: op=texts_set (write_text_pool is an alias).
// texts is {KEY: text} for one kind (kind: S by default, I, H), or
// {"selections": {...}, "symbols": {...}, "headings": {...}}; a null text
// removes the key. dry_run
// returns the plan; language names a translation and allows it;
// allow_unknown writes selection texts for keys the screen does not have.
func (s *Server) handleTextsSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	target, err := textTargetFrom(args)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	kinds, err := textKindsFrom(args)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	lang := getStringParam(args, "language")
	opts := adt.TextPoolOptions{AnyLanguage: lang != ""}
	if lang == "" {
		lang = s.adtClient.Language()
	}
	opts.DryRun, _ = getBoolParam(args, "dry_run")
	opts.AllowUnknown, _ = getBoolParam(args, "allow_unknown")
	plan, err := s.adtClient.WriteTextPool(ctx, target, lang, kinds, getStringParam(args, "transport"), opts)
	if err != nil {
		if plan != nil {
			return newToolResultJSON(map[string]any{"error": err.Error(), "plan": plan}), nil
		}
		return newToolResultError(fmt.Sprintf("WriteTextPool failed: %v", err)), nil
	}
	return newToolResultJSON(plan), nil
}

// textKindsFrom reads the texts parameter in either shape.
func textKindsFrom(args map[string]any) (map[string]map[string]string, error) {
	raw, ok := args["texts"]
	if !ok {
		return nil, fmt.Errorf("texts is required: {\"P_DEVC\": \"Package to scan\"} for one kind, or {\"selections\": {...}, \"symbols\": {...}}")
	}
	byName := map[string]string{"SELECTIONS": "S", "SYMBOLS": "I", "HEADINGS": "H", "S": "S", "I": "I", "H": "H"}
	out := map[string]map[string]string{}
	toMap := func(v any) (map[string]string, bool) {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		flat := map[string]string{}
		for k, x := range m {
			if x == nil {
				// null removes the key.
				flat[strings.ToUpper(k)] = adt.TextDelete
				continue
			}
			flat[strings.ToUpper(k)] = fmt.Sprint(x)
		}
		return flat, true
	}
	switch t := raw.(type) {
	case map[string]any:
		nested := false
		for k, v := range t {
			if kind, known := byName[strings.ToUpper(k)]; known {
				if m, ok := toMap(v); ok {
					out[kind] = m
					nested = true
				}
			}
		}
		if !nested {
			kind := strings.ToUpper(getStringParam(args, "kind"))
			if kind == "" {
				kind = "S"
			}
			m, _ := toMap(t)
			out[kind] = m
		}
	case string:
		kind := strings.ToUpper(getStringParam(args, "kind"))
		if kind == "" {
			kind = "S"
		}
		m := map[string]string{}
		for _, line := range strings.Split(t, "\n") {
			if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) != "" {
				m[strings.ToUpper(strings.TrimSpace(k))] = strings.TrimRight(v, "\r")
			}
		}
		out[kind] = m
	default:
		return nil, fmt.Errorf("texts must be an object")
	}
	for k, m := range out {
		if len(m) == 0 {
			delete(out, k)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("texts holds no entries")
	}
	return out, nil
}

// textPoolHint is the line appended to a create or edit of a program: which
// screen fields have no selection text, which TEXT-xxx the source uses but
// the pool does not define, and how to set them. A hint, never a write;
// empty when nothing is missing or the check itself failed.
func (s *Server) textPoolHint(ctx context.Context, target adt.TextPoolTarget, source string) string {
	gaps, err := s.adtClient.TextPoolGaps(ctx, target, s.adtClient.Language(), source)
	if err != nil || gaps.Empty() {
		return ""
	}
	var b strings.Builder
	b.WriteString("Texts: ")
	if len(gaps.Selections) > 0 {
		fmt.Fprintf(&b, "%d screen field(s) without a selection text (%s)", len(gaps.Selections), strings.Join(gaps.Selections, ", "))
	}
	if len(gaps.Symbols) > 0 {
		if len(gaps.Selections) > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%d text symbol(s) used but not defined (%s)", len(gaps.Symbols), strings.Join(gaps.Symbols, ", "))
	}
	b.WriteString(". Set them with SAP(action=\"i18n\", params={\"op\": \"texts_set\", \"program_name\": \"" + strings.ToUpper(target.Name) + "\", \"texts\": {")
	var parts []string
	for _, k := range gaps.Selections {
		parts = append(parts, fmt.Sprintf("\"selections\": {\"%s\": \"...\"}", k))
		break
	}
	for _, k := range gaps.Symbols {
		parts = append(parts, fmt.Sprintf("\"symbols\": {\"%s\": \"...\"}", k))
		break
	}
	b.WriteString(strings.Join(parts, ", ") + "}}); dry_run: true shows the plan first.")
	return b.String()
}

// withHint appends a hint to a JSON text result as a "hints" field, or to a
// plain text result as a trailing line.
func withHint(result *mcp.CallToolResult, hint string) *mcp.CallToolResult {
	if hint == "" || result == nil || len(result.Content) == 0 {
		return result
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return result
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &m); err == nil && m != nil {
		m["hints"] = []string{hint}
		if out, err := json.MarshalIndent(m, "", "  "); err == nil {
			return mcp.NewToolResultText(string(out))
		}
	}
	return mcp.NewToolResultText(tc.Text + "\n\n" + hint)
}
