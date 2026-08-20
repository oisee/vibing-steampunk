package adt

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//nolint:bodyclose // Transport.Request owns and closes every synthetic response body.
func TestDeployFromFileStopsOnInconclusiveExistenceCheck(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "zsynthetic.prog.abap")
	if err := os.WriteFile(filePath, []byte("REPORT zsynthetic."), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	mock := &mockHTTPClient{responses: []*http.Response{
		newMockResponse(http.StatusInternalServerError, "synthetic server failure", nil),
	}}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	result, err := client.DeployFromFile(context.Background(), filePath, "$TMP", "")
	if err == nil || !strings.Contains(err.Error(), "checking whether") {
		t.Fatalf("DeployFromFile() result = %#v, error = %v, want inconclusive existence error", result, err)
	}
	if len(mock.requests) != 1 {
		t.Fatalf("requests = %d, want only the existence probe", len(mock.requests))
	}
}
