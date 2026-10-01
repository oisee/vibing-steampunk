package mcp

import (
	"context"
	"strings"
	"testing"
	"time"
)

func importStatusServer(t *testing.T, cfg func(*Config)) *Server {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	c := &Config{
		BaseURL: "http://127.0.0.1:1", Username: "u", Password: "p", Client: "001", Language: "EN",
		ReadOnly: true, EnableTransports: true,
	}
	if cfg != nil {
		cfg(c)
	}
	return NewServer(c)
}

func callImportStatus(t *testing.T, s *Server, port int, extra map[string]any) string {
	t.Helper()
	p := rfcParams(t, port, extra)
	p["type"] = "import_status"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := s.handleUniversalTool(ctx, newRequest(map[string]any{"action": "system", "params": p}))
	if err != nil {
		t.Fatalf("import_status: %v", err)
	}
	if !res.IsError {
		t.Fatalf("import_status succeeded against a gateway that hangs up: %s", uploadResultText(res))
	}
	return uploadResultText(res)
}

// A read: under --read-only it goes on to the gateway.
func TestImportStatus_ReadsUnderReadOnly(t *testing.T) {
	s := importStatusServer(t, nil)
	port, dials := fakeGateway(t)
	msg := callImportStatus(t, s, port, map[string]any{"transport": "XYZK900001, XYZK900002", "since": "20260101"})
	if strings.Contains(msg, "blocked") {
		t.Fatalf("refused: %s", msg)
	}
	if dials() == 0 {
		t.Errorf("never reached the gateway: %s", msg)
	}
}

// Every refusal comes before the gateway is dialled.
func TestImportStatus_RefusesBeforeLogon(t *testing.T) {
	cases := map[string]struct {
		cfg   func(*Config)
		extra map[string]any
		want  string
	}{
		"no request":          {nil, map[string]any{}, "is required"},
		"not a request":       {nil, map[string]any{"transport": "X' OR '1'='1"}, "is not"},
		"transports disabled": {func(c *Config) { c.EnableTransports = false }, map[string]any{"transport": "XYZK900001"}, "transports not enabled"},
		"not allowed":         {func(c *Config) { c.AllowedTransports = []string{"ABCK*"} }, map[string]any{"transport": "XYZK900001"}, "blocked by safety configuration"},
		"one of a list not allowed": {func(c *Config) { c.AllowedTransports = []string{"XYZK900001"} },
			map[string]any{"transport": []any{"XYZK900001", "XYZK900002"}}, "XYZK900002"},
		"other host":   {nil, map[string]any{"transport": "XYZK900001", "host": "192.0.2.1"}, "not accepted"},
		"other client": {nil, map[string]any{"transport": "XYZK900001", "client": "100"}, "not accepted"},
		"bad since":    {nil, map[string]any{"transport": "XYZK900001", "since": "yesterday"}, "since"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := importStatusServer(t, tc.cfg)
			port, dials := fakeGateway(t)
			msg := callImportStatus(t, s, port, tc.extra)
			if !strings.Contains(msg, tc.want) {
				t.Errorf("got %q, want it to mention %q", msg, tc.want)
			}
			if dials() != 0 {
				t.Errorf("refused import_status still dialled the gateway %d time(s)", dials())
			}
		})
	}
}
