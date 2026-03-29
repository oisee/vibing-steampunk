// Package mcp provides the MCP server implementation for ABAP ADT tools.
// tools_aliases.go provides short alias names for frequently used tools.
package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// registerToolAliases registers short alias names for frequently used tools.
// Aliases provide quick access: gs→GetSource, ws→WriteSource, etc.
func (s *Server) registerToolAliases(shouldRegister func(string) bool) {
	// Define aliases: alias -> canonical tool name
	// Only register alias if the canonical tool is registered
	type aliasInfo struct {
		canonical string
		desc      string
		handler   func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
	}

	aliases := map[string]aliasInfo{
		// Core read/write
		"gs": {"GetSource", "Alias for GetSource - read ABAP source code", s.handleGetSource},
		"ws": {"WriteSource", "Alias for WriteSource - write ABAP source code", s.handleWriteSource},
		"es": {"EditSource", "Alias for EditSource - surgical string replacement", s.handleEditSource},

		// Search
		"so": {"SearchObject", "Alias for SearchObject - find ABAP objects", s.handleSearchObject},
		"gro": {"GrepObjects", "Alias for GrepObjects - regex search in objects", s.handleGrepObjects},
		"grp": {"GrepPackages", "Alias for GrepPackages - regex search in packages", s.handleGrepPackages},

		// Common operations
		"gt": {"GetTable", "Alias for GetTable - get table structure", s.handleGetTable},
		"gtc": {"GetTableContents", "Alias for GetTableContents - read table data", s.handleGetTableContents},
		"rq": {"RunQuery", "Alias for RunQuery - execute SQL query", s.handleRunQuery},
		"sc": {"SyntaxCheck", "Alias for SyntaxCheck - check ABAP syntax", s.handleSyntaxCheck},
		"act": {"Activate", "Alias for Activate - activate ABAP object", s.handleActivate},

		// Testing
		"rut": {"RunUnitTests", "Alias for RunUnitTests - run ABAP unit tests", s.handleRunUnitTests},
		"atc": {"RunATCCheck", "Alias for RunATCCheck - run ATC code check", s.handleRunATCCheck},
	}

	// Aliases disabled by default - they bloat the tool list without adding value
	// Uncomment if you want short names like gs, ws, es, etc.
	_ = aliases // suppress unused variable warning
	/*
	for alias, info := range aliases {
		if shouldRegister(info.canonical) {
			s.mcpServer.AddTool(mcp.NewTool(alias,
				mcp.WithDescription(info.desc),
				// Aliases inherit all parameters from the canonical tool
				// The handler is the same, so parameters work identically
			), info.handler)
		}
	}
	*/
}
