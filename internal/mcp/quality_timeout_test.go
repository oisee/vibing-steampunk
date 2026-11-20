package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// Only the run is slow; variant lookup, CSRF and the worklist answer at once.
func qualitySAP(t *testing.T, delay time.Duration, onRun func()) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	runs := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("X-CSRF-Token", "test-token")
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Path == "/sap/bc/adt/atc/runs" || r.URL.Path == "/sap/bc/adt/abapunit/testruns" {
			runs.Add(1)
			if onRun != nil {
				onRun()
			}
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		switch r.URL.Path {
		case "/sap/bc/adt/core/discovery":
			_, _ = io.WriteString(w, `<service/>`)
		case "/sap/bc/adt/atc/worklists":
			_, _ = io.WriteString(w, "DEMO-WORKLIST")
		case "/sap/bc/adt/atc/runs":
			_, _ = io.WriteString(w, `<worklistRun><worklistId>DEMO-WORKLIST</worklistId></worklistRun>`)
		case "/sap/bc/adt/atc/worklists/DEMO-WORKLIST":
			_, _ = io.WriteString(w, `<worklist id="DEMO-WORKLIST"><objects><object name="ZCL_DEMO"/></objects></worklist>`)
		case "/sap/bc/adt/abapunit/testruns":
			_, _ = io.WriteString(w, `<runResult><coverage><statement><node total="10" covered="8"/></statement></coverage></runResult>`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, runs
}

func TestQualityCallsHonourCallTimeout(t *testing.T) {
	for _, tool := range []struct {
		name string
		op   string
		call func(*Server, context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
	}{
		{"RunATCCheck", "ATC run", (*Server).handleRunATCCheck},
		{"GetCodeCoverage", "ABAP Unit coverage run", (*Server).handleGetCodeCoverage},
	} {
		t.Run(tool.name, func(t *testing.T) {
			for _, tc := range []struct {
				name         string
				serverBudget time.Duration
				timeout      any
				callerBudget time.Duration
				cancelOnRun  bool
				wantError    string
			}{
				{name: "server budget", serverBudget: 3 * time.Second},
				{name: "parameter overrides server budget", serverBudget: 50 * time.Millisecond, timeout: 3.0},
				{name: "no budget keeps request limit", wantError: "per-request limit"},
				{name: "parameter budget expires", serverBudget: 3 * time.Second, timeout: 0.05, wantError: "timed out after 50ms"},
				{name: "caller deadline wins", timeout: 3.0, callerBudget: 50 * time.Millisecond, wantError: "at the caller's own deadline"},
				{name: "caller cancels", timeout: 3.0, cancelOnRun: true, wantError: "was cancelled by the client"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					var onRun func()
					if tc.cancelOnRun {
						onRun = cancel
					}
					srv, runs := qualitySAP(t, 300*time.Millisecond, onRun)
					s := &Server{
						config:    &Config{CallTimeout: tc.serverBudget},
						adtClient: adt.NewClient(srv.URL, "TESTUSER", "unused", adt.WithTimeout(100*time.Millisecond)),
					}
					args := map[string]any{"object_url": "/sap/bc/adt/oo/classes/zcl_demo", "variant": "STANDARD"}
					if tc.timeout != nil {
						args["timeout"] = tc.timeout
					}
					if tc.callerBudget > 0 {
						var deadlineCancel context.CancelFunc
						ctx, deadlineCancel = context.WithTimeout(ctx, tc.callerBudget)
						defer deadlineCancel()
					}
					res, err := tool.call(s, ctx, newRequest(args))
					if err != nil {
						t.Fatal(err)
					}
					text := resultText(res)
					if tc.wantError != "" {
						if !res.IsError || !strings.HasPrefix(text, tool.op+" ") || !strings.Contains(text, tc.wantError) || !strings.Contains(text, "may still be running on SAP") {
							t.Fatalf("want %q and operation-specific timeout/cancellation detail, got %s", tc.wantError, text)
						}
					} else {
						if res.IsError {
							t.Fatalf("run with a budget was cut at the shorter request limit: %s", text)
						}
						var got struct {
							Summary struct {
								TotalObjects int `json:"totalObjects"`
							} `json:"summary"`
							Statements adt.CoverageStats `json:"statements"`
						}
						if err := json.Unmarshal([]byte(text), &got); err != nil {
							t.Fatal(err)
						}
						if tool.name == "RunATCCheck" && got.Summary.TotalObjects != 1 {
							t.Fatalf("worklist was not returned: %s", text)
						}
						if tool.name == "GetCodeCoverage" && (got.Statements.Total != 10 || got.Statements.Covered != 8) {
							t.Fatalf("coverage was not returned: %s", text)
						}
					}
					if runs.Load() != 1 {
						t.Fatalf("run requests = %d, want exactly one", runs.Load())
					}
				})
			}
		})
	}
}

func TestQualityTimeoutRegistrationAndRouting(t *testing.T) {
	s := NewServer(&Config{BaseURL: "http://127.0.0.1:1", Username: "TESTUSER", Password: "unused", Mode: "expert"})
	for _, name := range []string{"RunATCCheck", "GetCodeCoverage"} {
		t.Run(name, func(t *testing.T) {
			tool := s.mcpServer.ListTools()[name]
			property, ok := tool.Tool.InputSchema.Properties["timeout"].(map[string]any)
			if !ok || property["type"] != "number" || property["description"] != callTimeoutDescription {
				t.Fatalf("missing shared numeric timeout schema: %v", property)
			}
		})
	}
	// An invalid budget must be rejected before network access, including
	// both universal routes. In particular, read COVERAGE must forward it.
	for _, tc := range []struct {
		action, target string
		params         map[string]any
	}{
		{"test", "ATC", map[string]any{"object_uri": "/sap/bc/adt/oo/classes/zcl_demo", "timeout": "invalid"}},
		{"read", "COVERAGE /sap/bc/adt/oo/classes/zcl_demo", map[string]any{"timeout": "invalid"}},
	} {
		t.Run(tc.action, func(t *testing.T) {
			res, err := s.handleUniversalTool(context.Background(), newRequest(map[string]any{
				"action": tc.action, "target": tc.target, "params": tc.params,
			}))
			if err != nil || res == nil || !res.IsError || !strings.Contains(resultText(res), "timeout must be a number") {
				t.Fatalf("route ignored invalid timeout: result=%v error=%v", res, err)
			}
		})
	}
}
