package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleWriteSourceUsesExistingObjectPackage exercises the registered MCP
// boundary rather than calling the ADT helper directly. An omitted package is
// valid for an update, but the object's actual forbidden package still wins.
func TestHandleWriteSourceUsesExistingObjectPackage(t *testing.T) {
	locks := 0
	sap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "test-token")
		if strings.Contains(r.URL.Path, "informationsystem/search") {
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/zdemo_mcp_denied" adtcore:type="PROG/P" adtcore:name="ZDEMO_MCP_DENIED" adtcore:packageName="ZDENIED"/>
</adtcore:objectReferences>`))
			return
		}
		if r.URL.Query().Get("_action") == "LOCK" {
			locks++
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer sap.Close()

	s := NewServer(&Config{
		BaseURL:         sap.URL,
		Username:        "TESTUSER",
		Password:        "secret",
		Mode:            "focused",
		AllowedPackages: []string{"$TMP"},
	})
	result, err := s.handleWriteSource(context.Background(), newRequest(map[string]any{
		"object_type": "PROG",
		"name":        "ZDEMO_MCP_DENIED",
		"source":      "REPORT zdemo_mcp_denied.",
		"mode":        "update",
	}))
	if err != nil {
		t.Fatalf("handleWriteSource: %v", err)
	}
	if !result.IsError || !strings.Contains(resultText(result), "blocked by safety configuration") {
		t.Fatalf("MCP WriteSource result = %#v, want actual-package refusal", result)
	}
	if locks != 0 {
		t.Fatalf("forbidden MCP update acquired %d locks", locks)
	}
}
