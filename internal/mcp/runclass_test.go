package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestRunClassTool_RequiresClassName(t *testing.T) {
	s := NewServer(&Config{BaseURL: "https://example.invalid", Username: "tester", Password: "unused", Client: "000", Language: "EN", Mode: "expert"})
	req := mcp.CallToolRequest{}
	req.Params.Name = "RunClass"
	req.Params.Arguments = map[string]any{"class_name": "  "}

	res, err := s.handleRunClass(context.Background(), req)
	if err != nil {
		t.Fatalf("handleRunClass: %v", err)
	}
	if !res.IsError {
		t.Fatal("an empty class_name must be a tool error")
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "class_name is required") {
		t.Errorf("error text = %q", text)
	}
}

// classrunSAP stands in for SAP: the classrun POST answers with status and
// body, everything else succeeds without a body. Without --allowed-packages
// RunClass asks nothing else.
func classrunSAP(t *testing.T, status int, body string) *Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "AbCdEfGhIjKlMnOpQrStUv==")
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/sap/bc/adt/oo/classrun/") {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return NewServer(&Config{BaseURL: srv.URL, Username: "u", Password: "p", Client: "001", Language: "EN", Mode: "expert"})
}

func TestRunClassTool_EmptyOutputIsNotAnError(t *testing.T) {
	s := classrunSAP(t, http.StatusOK, "")

	res, err := s.handleRunClass(context.Background(), newRequest(map[string]any{"class_name": "ZCL_DEMO_RUN"}))
	if err != nil {
		t.Fatalf("handleRunClass: %v", err)
	}
	if res.IsError {
		t.Fatalf("a class without console output is not an error: %s", resultText(res))
	}
	if text := resultText(res); !strings.Contains(text, "wrote no console output") {
		t.Errorf("text = %q, want the note that the class wrote no console output", text)
	}
}

func TestRunClassTool_SAPErrorIsAToolError(t *testing.T) {
	s := classrunSAP(t, http.StatusNotFound, "Class ZCL_DEMO_RUN does not exist")

	res, err := s.handleRunClass(context.Background(), newRequest(map[string]any{"class_name": "ZCL_DEMO_RUN"}))
	if err != nil {
		t.Fatalf("handleRunClass: %v", err)
	}
	if !res.IsError {
		t.Fatalf("an SAP error must be a tool error: %s", resultText(res))
	}
	if text := resultText(res); !strings.HasPrefix(text, "RunClass failed:") {
		t.Errorf("text = %q, want it to start with \"RunClass failed:\"", text)
	}
}
