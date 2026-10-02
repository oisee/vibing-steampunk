package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// identityRefusal holds a tool call to the identity pin before it runs.
//
// The ADT client already refuses every request on its own once the pin does
// not match; this is here for the tools that reach SAP by another road
// (WebSocket, classic RFC), which would otherwise log on before anything asked
// who is on the other end. Without a pin it returns nil at once and sends
// nothing. SAP() with no action, "info" and "help" are let through: the card
// reports the pin itself, and help needs no system.
func (s *Server) identityRefusal(ctx context.Context, req mcp.CallToolRequest) *mcp.CallToolResult {
	if s == nil || s.adtClient == nil {
		return nil
	}
	if pin, _, _ := s.adtClient.IdentityStatus(); pin == nil {
		return nil
	}
	if req.Params.Name == "SAP" {
		action, _ := req.GetArguments()["action"].(string)
		switch strings.ToLower(strings.TrimSpace(action)) {
		case "", "info", "help":
			return nil
		}
	}
	if err := s.adtClient.VerifyIdentity(ctx); err != nil {
		return newToolResultError(err.Error())
	}
	return nil
}

// identityLine is the info card's line about the pin, or "" without one.
func (s *Server) identityLine() string {
	pin, verified, refused := s.adtClient.IdentityStatus()
	if pin == nil {
		return ""
	}
	source := ""
	if s.config != nil && s.config.ExpectSource != "" {
		source = " (" + s.config.ExpectSource + ")"
	}
	switch {
	case refused != nil:
		return fmt.Sprintf("  pinned       %s ✗ %v%s\n", pin, refused, source)
	case verified != nil:
		return fmt.Sprintf("  pinned       %s ✓%s\n", pin, source)
	default:
		return fmt.Sprintf("  pinned       %s, not checked yet — the first request to SAP checks it%s\n", pin, source)
	}
}
