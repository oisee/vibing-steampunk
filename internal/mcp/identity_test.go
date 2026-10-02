package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// identityFake answers the system information resource as sid/client/user and
// everything else with a CSRF token, recording what it saw.
func identityFake(t *testing.T, sid, client, user string) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("x-csrf-token", "AbCdEfGhIjKlMnOpQrStUv==")
		if r.URL.Path == "/sap/bc/adt/core/http/systeminformation" {
			fmt.Fprintf(w, `{"systemID":%q,"client":%q,"userName":%q}`, sid, client, user)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

func pinnedServer(t *testing.T, url, pin string) *Server {
	t.Helper()
	p, err := adt.ParseIdentityPin(pin)
	if err != nil {
		t.Fatal(err)
	}
	return NewServer(&Config{BaseURL: url, Username: "DEVELOPER", Password: "x", Client: "001",
		Mode: "hyperfocused", Build: "v9.9.9", Expect: p, ExpectSource: "--expect"})
}

// SAP(action="info") shows the pin and its verdict.
func TestInfoShowsThePin(t *testing.T) {
	sap, _ := identityFake(t, "A4H", "001", "DEVELOPER")
	text := resultText(pinnedServer(t, sap.URL, "A4H.001/DEVELOPER").handleInfo(t.Context()))
	if !strings.Contains(text, "pinned       A4H.001/DEVELOPER ✓") {
		t.Errorf("a confirmed pin is not shown:\n%s", text)
	}

	other, _ := identityFake(t, "B4H", "001", "DEVELOPER")
	text = resultText(pinnedServer(t, other.URL, "A4H.001/DEVELOPER").handleInfo(t.Context()))
	if !strings.Contains(text, "pinned       A4H.001/DEVELOPER ✗") || !strings.Contains(text, "connected to B4H.001 as DEVELOPER") {
		t.Errorf("a refused pin is not shown with its reason:\n%s", text)
	}
	if !strings.Contains(text, "NOT usable") {
		t.Errorf("a refused pin still reports a usable connection:\n%s", text)
	}

	unpinned := NewServer(&Config{BaseURL: sap.URL, Username: "DEVELOPER", Password: "x", Client: "001", Mode: "hyperfocused"})
	if text := resultText(unpinned.handleInfo(t.Context())); strings.Contains(text, "pinned") {
		t.Errorf("no pin, yet the card mentions one:\n%s", text)
	}
}

// A tool that reaches SAP by another road than the ADT client — here classic
// RFC — is held to the pin before it runs.
func TestToolCallsAreRefusedWhenThePinDoesNotMatch(t *testing.T) {
	sap, seen := identityFake(t, "B4H", "001", "DEVELOPER")
	s := pinnedServer(t, sap.URL, "A4H.001/DEVELOPER")
	raw := s.mcpServer.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call",
		"params":{"name":"SAP","arguments":{"action":"rfc","params":{"op":"info"}}}}`))
	resp, ok := raw.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("got %T", raw)
	}
	result, ok := resp.Result.(*mcp.CallToolResult)
	if !ok {
		t.Fatalf("got result %T", resp.Result)
	}
	if !result.IsError || !strings.Contains(resultText(result), "identity pin refused") {
		t.Errorf("the call was not refused by the pin:\n%s", resultText(result))
	}
	if got := seen(); len(got) != 1 {
		t.Errorf("only the preflight may reach the system; it saw %v", got)
	}
}

// The debug session opens its own ADT connection; it carries the server's pin.
func TestDebugSessionTransportCarriesThePin(t *testing.T) {
	sap, seen := identityFake(t, "B4H", "001", "DEVELOPER")
	s := pinnedServer(t, sap.URL, "A4H.001/DEVELOPER")
	transport, err := s.statefulADTTransport()
	if err != nil {
		t.Fatal(err)
	}
	_, err = transport.Do(context.Background(), saprfc.ADTRequest{Method: "GET", URI: "/sap/bc/adt/debugger/listeners"})
	if !adt.IsIdentityMismatch(err) {
		t.Fatalf("want an identity refusal, got %v", err)
	}
	if got := seen(); len(got) != 1 {
		t.Errorf("only the preflight may reach another system; it saw %v", got)
	}
}

// An RFC call that names another logon user than the pin's is refused before
// the gateway is dialled, even when the server's own ADT logon matches.
func TestRFCUserOverrideContradictingThePinIsRefused(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", dir)
	sap, seen := identityFake(t, "A4H", "001", "DEVELOPER")
	s := pinnedServer(t, sap.URL, "A4H.001/DEVELOPER")
	raw := s.mcpServer.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call",
		"params":{"name":"SAP","arguments":{"action":"rfc","params":{"op":"ping","user":"OTHERUSER"}}}}`))
	resp, ok := raw.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("got %T", raw)
	}
	result, _ := resp.Result.(*mcp.CallToolResult)
	if result == nil || !result.IsError || !strings.Contains(resultText(result), "configured to log on as OTHERUSER") {
		t.Errorf("the RFC override was not refused by the pin:\n%s", resultText(result))
	}
	if got := seen(); len(got) != 1 {
		t.Errorf("only the ADT preflight may be sent; saw %v", got)
	}
}
