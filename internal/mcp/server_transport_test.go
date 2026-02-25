package mcp

import (
	"errors"
	"strings"
	"testing"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

type mockStreamableServer struct {
	startCalls int
	startAddr  string
	startErr   error
}

func (m *mockStreamableServer) Start(addr string) error {
	m.startCalls++
	m.startAddr = addr
	return m.startErr
}

func newTestConfig() *Config {
	return &Config{
		BaseURL:  "https://sap.example.com:44300",
		Username: "testuser",
		Password: "testpass",
		Client:   "001",
		Language: "EN",
		Mode:     "focused",
	}
}

func installTransportHooks(
	t *testing.T,
	stdio func(*mcpserver.MCPServer) error,
	factory func(*mcpserver.MCPServer, ...mcpserver.StreamableHTTPOption) streamableHTTPStarter,
) {
	t.Helper()
	oldStdio := serveStdioFunc
	oldFactory := newStreamableHTTPServerFunc
	serveStdioFunc = stdio
	newStreamableHTTPServerFunc = factory
	t.Cleanup(func() {
		serveStdioFunc = oldStdio
		newStreamableHTTPServerFunc = oldFactory
	})
}

func TestServe_UsesStdioTransport(t *testing.T) {
	s := NewServer(newTestConfig())
	expectedErr := errors.New("stdio failure")
	stdioCalls := 0

	installTransportHooks(
		t,
		func(_ *mcpserver.MCPServer) error {
			stdioCalls++
			return expectedErr
		},
		func(_ *mcpserver.MCPServer, _ ...mcpserver.StreamableHTTPOption) streamableHTTPStarter {
			t.Fatal("streamable HTTP transport should not be created for stdio")
			return nil
		},
	)

	err := s.Serve("stdio")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected stdio error %v, got %v", expectedErr, err)
	}
	if stdioCalls != 1 {
		t.Fatalf("expected stdio to be called once, got %d", stdioCalls)
	}
}

func TestServe_EmptyTransportDefaultsToStdio(t *testing.T) {
	s := NewServer(newTestConfig())
	stdioCalls := 0

	installTransportHooks(
		t,
		func(_ *mcpserver.MCPServer) error {
			stdioCalls++
			return nil
		},
		func(_ *mcpserver.MCPServer, _ ...mcpserver.StreamableHTTPOption) streamableHTTPStarter {
			t.Fatal("streamable HTTP transport should not be created for empty transport")
			return nil
		},
	)

	if err := s.Serve("   "); err != nil {
		t.Fatalf("expected no error for default stdio transport, got %v", err)
	}
	if stdioCalls != 1 {
		t.Fatalf("expected stdio to be called once, got %d", stdioCalls)
	}
}

func TestServe_UsesHTTPStreamableTransport(t *testing.T) {
	s := NewServer(newTestConfig())
	expectedErr := errors.New("http streamable failure")
	mock := &mockStreamableServer{startErr: expectedErr}
	stdioCalls := 0
	factoryCalls := 0
	optionCount := 0

	installTransportHooks(
		t,
		func(_ *mcpserver.MCPServer) error {
			stdioCalls++
			return nil
		},
		func(_ *mcpserver.MCPServer, opts ...mcpserver.StreamableHTTPOption) streamableHTTPStarter {
			factoryCalls++
			optionCount = len(opts)
			return mock
		},
	)

	err := s.Serve("http-streamable")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected streamable HTTP error %v, got %v", expectedErr, err)
	}
	if stdioCalls != 0 {
		t.Fatalf("expected stdio not to be called, got %d calls", stdioCalls)
	}
	if factoryCalls != 1 {
		t.Fatalf("expected streamable HTTP factory to be called once, got %d", factoryCalls)
	}
	if optionCount != 1 {
		t.Fatalf("expected exactly one streamable HTTP option, got %d", optionCount)
	}
	if mock.startCalls != 1 {
		t.Fatalf("expected streamable HTTP server to start once, got %d", mock.startCalls)
	}
	if mock.startAddr != DefaultStreamableHTTPAddr {
		t.Fatalf("expected default addr %s, got %s", DefaultStreamableHTTPAddr, mock.startAddr)
	}
}

func TestServe_InvalidTransport(t *testing.T) {
	s := NewServer(newTestConfig())

	err := s.Serve("sse")
	if err == nil {
		t.Fatal("expected error for unsupported transport")
	}
	if !strings.Contains(err.Error(), "unsupported transport") {
		t.Fatalf("expected unsupported transport error, got %v", err)
	}
}

func TestServeStreamableHTTP_UsesProvidedAddr(t *testing.T) {
	s := NewServer(newTestConfig())
	mock := &mockStreamableServer{}

	installTransportHooks(
		t,
		func(_ *mcpserver.MCPServer) error { return nil },
		func(_ *mcpserver.MCPServer, _ ...mcpserver.StreamableHTTPOption) streamableHTTPStarter {
			return mock
		},
	)

	addr := "127.0.0.1:9090"
	if err := s.ServeStreamableHTTP(addr); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if mock.startAddr != addr {
		t.Fatalf("expected streamable HTTP addr %s, got %s", addr, mock.startAddr)
	}
}

func TestServeStreamableHTTP_BlankAddrUsesDefault(t *testing.T) {
	s := NewServer(newTestConfig())
	mock := &mockStreamableServer{}

	installTransportHooks(
		t,
		func(_ *mcpserver.MCPServer) error { return nil },
		func(_ *mcpserver.MCPServer, _ ...mcpserver.StreamableHTTPOption) streamableHTTPStarter {
			return mock
		},
	)

	if err := s.ServeStreamableHTTP("   "); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if mock.startAddr != DefaultStreamableHTTPAddr {
		t.Fatalf("expected default addr %s, got %s", DefaultStreamableHTTPAddr, mock.startAddr)
	}
}
