package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

func TestTextKindsFrom(t *testing.T) {
	flat, err := textKindsFrom(map[string]any{"texts": map[string]any{"p_devc": "Package", "p_gone": nil}})
	if err != nil || flat["S"]["P_DEVC"] != "Package" || flat["S"]["P_GONE"] != adt.TextDelete || len(flat) != 1 {
		t.Errorf("flat: %v %v", flat, err)
	}
	kind, err := textKindsFrom(map[string]any{"kind": "i", "texts": map[string]any{"001": "Nothing"}})
	if err != nil || kind["I"]["001"] != "Nothing" {
		t.Errorf("kind: %v %v", kind, err)
	}
	nested, err := textKindsFrom(map[string]any{"texts": map[string]any{
		"selections": map[string]any{"P_DEVC": "Package"},
		"symbols":    map[string]any{"001": "Nothing"},
		"headings":   map[string]any{},
	}})
	if err != nil || nested["S"]["P_DEVC"] != "Package" || nested["I"]["001"] != "Nothing" || len(nested) != 2 {
		t.Errorf("nested: %v %v", nested, err)
	}
	lines, err := textKindsFrom(map[string]any{"texts": "P_DEVC=Package\nS_OBJ=Objects\n"})
	if err != nil || lines["S"]["S_OBJ"] != "Objects" {
		t.Errorf("lines: %v %v", lines, err)
	}
	if _, err := textKindsFrom(map[string]any{}); err == nil {
		t.Error("missing texts accepted")
	}
	if _, err := textKindsFrom(map[string]any{"texts": map[string]any{}}); err == nil {
		t.Error("empty texts accepted")
	}
}

func TestWithHint(t *testing.T) {
	res := withHint(mcp.NewToolResultText(`{"success": true}`), "Texts: 1 field")
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &m); err != nil || m["success"] != true {
		t.Fatalf("json result lost: %v %v", m, err)
	}
	if h, _ := m["hints"].([]any); len(h) != 1 || h[0] != "Texts: 1 field" {
		t.Errorf("hints: %v", m["hints"])
	}
	plain := withHint(mcp.NewToolResultText("done"), "Texts: 1 field")
	if txt := plain.Content[0].(mcp.TextContent).Text; !strings.HasSuffix(txt, "\n\nTexts: 1 field") {
		t.Errorf("plain: %q", txt)
	}
	if same := withHint(mcp.NewToolResultText("done"), ""); same.Content[0].(mcp.TextContent).Text != "done" {
		t.Error("empty hint changed the result")
	}
}
