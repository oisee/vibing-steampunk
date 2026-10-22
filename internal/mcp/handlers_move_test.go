package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// A transport the move could not be recorded in refuses the move before the
// WebSocket is opened: nothing moves that cannot be recorded.
func TestHandleMoveObject_TransportCheckedBeforeTheMove(t *testing.T) {
	server := NewServer(&Config{BaseURL: "http://127.0.0.1:1", Username: "u", Password: "p", Client: "001", Language: "EN"})
	res, err := server.handleMoveObject(context.Background(), newRequest(map[string]any{
		"object_type": "CLAS", "object_name": "ZCL_DEMO", "new_package": "$TMP", "transport": "A4HK900001",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !res.IsError || !strings.Contains(text, "transports not enabled") || strings.Contains(text, "WebSocket") {
		t.Errorf("result = %q, want the transport refusal before any WebSocket", text)
	}
}

func TestMoveTimeoutHint(t *testing.T) {
	if got := moveTimeoutHint(errors.New("request timeout")); !strings.Contains(got, "list_dumps") || !strings.Contains(got, "rolled the move back") {
		t.Errorf("timeout hint = %q", got)
	}
	if got := moveTimeoutHint(errors.New("not connected")); got != "" {
		t.Errorf("hint for another error: %q", got)
	}
}
