package mcp

import (
	"errors"
	"net/http"
	"net/http/httptest"
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

func (m *mockStreamableServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
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

func installListenHook(t *testing.T, fn func(addr string, handler http.Handler) error) {
	t.Helper()
	old := listenAndServeFunc
	listenAndServeFunc = fn
	t.Cleanup(func() { listenAndServeFunc = old })
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
	mock := &mockStreamableServer{}
	stdioCalls := 0
	factoryCalls := 0
	optionCount := 0
	var listenAddr string

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
	installListenHook(t, func(addr string, _ http.Handler) error {
		listenAddr = addr
		return expectedErr
	})

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
	if listenAddr != DefaultStreamableHTTPAddr {
		t.Fatalf("expected default addr %s, got %s", DefaultStreamableHTTPAddr, listenAddr)
	}
}

func TestServe_UsesConfigHTTPAddr(t *testing.T) {
	cfg := newTestConfig()
	cfg.HTTPAddr = "0.0.0.0:9999"
	s := NewServer(cfg)
	var listenAddr string

	installTransportHooks(
		t,
		func(_ *mcpserver.MCPServer) error { return nil },
		func(_ *mcpserver.MCPServer, _ ...mcpserver.StreamableHTTPOption) streamableHTTPStarter {
			return &mockStreamableServer{}
		},
	)
	installListenHook(t, func(addr string, _ http.Handler) error {
		listenAddr = addr
		return nil
	})

	if err := s.Serve("http-streamable"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if listenAddr != "0.0.0.0:9999" {
		t.Fatalf("expected Config.HTTPAddr %q to be used, got %q", "0.0.0.0:9999", listenAddr)
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
	var listenAddr string

	installTransportHooks(
		t,
		func(_ *mcpserver.MCPServer) error { return nil },
		func(_ *mcpserver.MCPServer, _ ...mcpserver.StreamableHTTPOption) streamableHTTPStarter {
			return mock
		},
	)
	installListenHook(t, func(addr string, _ http.Handler) error {
		listenAddr = addr
		return nil
	})

	customAddr := "127.0.0.1:9090"
	if err := s.ServeStreamableHTTP(customAddr); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if listenAddr != customAddr {
		t.Fatalf("expected listen addr %s, got %s", customAddr, listenAddr)
	}
}

func TestServeStreamableHTTP_BlankAddrUsesDefault(t *testing.T) {
	s := NewServer(newTestConfig())
	mock := &mockStreamableServer{}
	var listenAddr string

	installTransportHooks(
		t,
		func(_ *mcpserver.MCPServer) error { return nil },
		func(_ *mcpserver.MCPServer, _ ...mcpserver.StreamableHTTPOption) streamableHTTPStarter {
			return mock
		},
	)
	installListenHook(t, func(addr string, _ http.Handler) error {
		listenAddr = addr
		return nil
	})

	if err := s.ServeStreamableHTTP("   "); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if listenAddr != DefaultStreamableHTTPAddr {
		t.Fatalf("expected default addr %s, got %s", DefaultStreamableHTTPAddr, listenAddr)
	}
}

func TestOriginValidationMiddleware_AllowsNoOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := originValidationMiddleware("127.0.0.1:8080", next)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200 for request with no Origin, got %d", rw.Code)
	}
}

func TestOriginValidationMiddleware_AllowsMatchingOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := originValidationMiddleware("127.0.0.1:8080", next)

	for _, origin := range []string{
		"http://127.0.0.1:8080",
		"http://localhost:8080",
	} {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Origin", origin)
		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)

		if rw.Code != http.StatusOK {
			t.Fatalf("expected 200 for origin %q, got %d", origin, rw.Code)
		}
	}
}

func TestOriginValidationMiddleware_BlocksForeignOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := originValidationMiddleware("127.0.0.1:8080", next)

	for _, origin := range []string{
		"http://evil.example.com",
		"http://attacker.com:8080",
	} {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Origin", origin)
		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)

		if rw.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for origin %q, got %d", origin, rw.Code)
		}
	}
}
