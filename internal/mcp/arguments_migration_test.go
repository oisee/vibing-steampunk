package mcp

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestHandlersUseGetArgumentsAPI(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("failed to read internal/mcp directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "handlers_") || !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}

		content, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("failed to read %s: %v", name, err)
		}
		if bytes.Contains(content, []byte("Params.Arguments")) {
			t.Fatalf("%s still uses legacy Params.Arguments API; migrate to request.GetArguments()", name)
		}
	}
}
