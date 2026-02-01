package mcp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestNewToolResultError(t *testing.T) {
	result := newToolResultError("test error message")

	if result == nil {
		t.Fatal("newToolResultError returned nil")
	}

	if !result.IsError {
		t.Error("IsError should be true")
	}

	if len(result.Content) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(result.Content))
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("Content should be TextContent, got %T", result.Content[0])
	}

	if textContent.Text != "test error message" {
		t.Errorf("Text = %v, want 'test error message'", textContent.Text)
	}
}

func TestConfig(t *testing.T) {
	cfg := &Config{
		BaseURL:            "https://sap.example.com:44300",
		Username:           "testuser",
		Password:           "testpass",
		Client:             "100",
		Language:           "DE",
		InsecureSkipVerify: true,
	}

	if cfg.BaseURL != "https://sap.example.com:44300" {
		t.Errorf("BaseURL = %v, want https://sap.example.com:44300", cfg.BaseURL)
	}
	if cfg.Username != "testuser" {
		t.Errorf("Username = %v, want testuser", cfg.Username)
	}
	if cfg.Password != "testpass" {
		t.Errorf("Password = %v, want testpass", cfg.Password)
	}
	if cfg.Client != "100" {
		t.Errorf("Client = %v, want 100", cfg.Client)
	}
	if cfg.Language != "DE" {
		t.Errorf("Language = %v, want DE", cfg.Language)
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

func TestNewServer(t *testing.T) {
	cfg := &Config{
		BaseURL:  "https://sap.example.com:44300",
		Username: "testuser",
		Password: "testpass",
		Client:   "001",
		Language: "EN",
	}

	server := NewServer(cfg)

	if server == nil {
		t.Fatal("NewServer returned nil")
	}
	if server.mcpServer == nil {
		t.Error("MCP server should not be nil")
	}
	if server.adtClient == nil {
		t.Error("ADT client should not be nil")
	}
}

// TestAllHandlersAreRouted verifies that all handler functions defined in handlers_*.go
// files are actually called from their respective sub-routers. This prevents dead code
// where handlers are written but never wired up to any router.
func TestAllHandlersAreRouted(t *testing.T) {
	// Find the package directory
	pkgDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Collect all handler definitions from handlers_*.go files
	definedHandlers := make(map[string]string) // handler name -> source file
	handlerFiles, _ := filepath.Glob(filepath.Join(pkgDir, "handlers_*.go"))

	fset := token.NewFileSet()
	for _, file := range handlerFiles {
		// Skip the universal tool file - it's the router, not handler definitions
		if strings.HasSuffix(file, "handlers_universal.go") {
			continue
		}

		node, parseErr := parser.ParseFile(fset, file, nil, 0)
		if parseErr != nil {
			t.Fatalf("Failed to parse %s: %v", file, parseErr)
		}

		for _, decl := range node.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}
			// Check if it's a method on *Server
			if len(fn.Recv.List) != 1 {
				continue
			}
			starExpr, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			ident, ok := starExpr.X.(*ast.Ident)
			if !ok || ident.Name != "Server" {
				continue
			}
			// Check if method name starts with "handle"
			if strings.HasPrefix(fn.Name.Name, "handle") {
				definedHandlers[fn.Name.Name] = filepath.Base(file)
			}
		}
	}

	// Collect all handler calls from handlers_universal.go (main router)
	calledHandlers := make(map[string]bool)
	universalFile := filepath.Join(pkgDir, "handlers_universal.go")

	universalNode, parseErr := parser.ParseFile(fset, universalFile, nil, 0)
	if parseErr != nil {
		t.Fatalf("Failed to parse handlers_universal.go: %v", parseErr)
	}

	// Walk the AST to find all s.handle* calls and references
	ast.Inspect(universalNode, func(n ast.Node) bool {
		// Check for direct calls: s.handleXxx(...)
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if strings.HasPrefix(sel.Sel.Name, "handle") {
					calledHandlers[sel.Sel.Name] = true
				}
			}
		}
		// Check for function references: s.handleXxx passed as argument
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if strings.HasPrefix(sel.Sel.Name, "handle") {
				calledHandlers[sel.Sel.Name] = true
			}
		}
		return true
	})

	// Also check all other .go files in the package for handler references
	// This catches handlers called from sub-routers (routeXxxAction functions)
	allGoFiles, _ := filepath.Glob(filepath.Join(pkgDir, "*.go"))
	for _, file := range allGoFiles {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		if strings.HasSuffix(file, "handlers_universal.go") {
			continue // already processed
		}
		node, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			continue
		}
		ast.Inspect(node, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if strings.HasPrefix(sel.Sel.Name, "handle") {
					calledHandlers[sel.Sel.Name] = true
				}
			}
			return true
		})
	}

	// Find handlers that are defined but never called
	var unroutedHandlers []string
	for handler, sourceFile := range definedHandlers {
		if !calledHandlers[handler] {
			unroutedHandlers = append(unroutedHandlers, handler+" ("+sourceFile+")")
		}
	}

	if len(unroutedHandlers) > 0 {
		t.Errorf("Found %d handler(s) that are defined but never routed:\n  - %s\n\nThese handlers need to be called from their respective sub-router (routeXxxAction)",
			len(unroutedHandlers), strings.Join(unroutedHandlers, "\n  - "))
	}
}
