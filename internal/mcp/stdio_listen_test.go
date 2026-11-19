package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

// A 2026-07-28 client (Copilot CLI) opens subscriptions/listen right after
// server/discover and keeps it open for the session, then asks for the tools.
// The stdio server must keep reading while the listen request is held:
// with mcp-go v1.1.0 it read the next line only after a request returned, so
// tools/list never got an answer and the client gave up after its timeout.
func TestServeStdio_ToolsListAnsweredWhileListenIsOpen(t *testing.T) {
	server := NewServer(&Config{BaseURL: "http://127.0.0.1:1", Username: "u", Password: "p", Client: "001", Language: "EN"})

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() { _ = inW.Close(); _ = outW.Close() }()
	go func() { _ = server.serveStdio(ctx, inR, outW) }()

	answers := make(chan map[string]any, 8)
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var m map[string]any
			if json.Unmarshal(sc.Bytes(), &m) == nil && m["id"] != nil {
				answers <- m
			}
		}
	}()

	meta := map[string]any{
		"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
		"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "test", "version": "0"},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}
	line := func(id int, method string, params map[string]any) []byte {
		params["_meta"] = meta
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		return append(b, '\n')
	}
	// Written from a goroutine: a server that stopped reading would block the
	// pipe, and the test must still end at its own deadline.
	go func() {
		for _, b := range [][]byte{
			line(0, "server/discover", map[string]any{}),
			line(1, "subscriptions/listen", map[string]any{"notifications": map[string]any{"toolsListChanged": true}}),
			line(2, "tools/list", map[string]any{}),
		} {
			if _, err := inW.Write(b); err != nil {
				return
			}
		}
	}()

	deadline := time.After(10 * time.Second)
	for {
		select {
		case m := <-answers:
			if id, _ := m["id"].(float64); id == 2 {
				if m["error"] != nil {
					t.Fatalf("tools/list answered with an error: %v", m["error"])
				}
				return
			}
		case <-deadline:
			t.Fatal("tools/list got no answer while subscriptions/listen was open")
		}
	}
}
