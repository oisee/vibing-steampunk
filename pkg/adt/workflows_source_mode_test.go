package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestClientWriteSourceRejectsUnknownMode(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	mock := &mockWorkflowTransport{responses: map[string]*http.Response{}}
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	result, err := client.WriteSource(context.Background(), "PROG", "Z_SYNTHETIC", "REPORT z_synthetic.", &WriteSourceOptions{
		Mode: WriteSourceMode("replace"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Success || !strings.Contains(result.Message, "Invalid mode") {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(mock.requests) != 0 {
		t.Fatalf("invalid mode made network requests: %d", len(mock.requests))
	}
}
