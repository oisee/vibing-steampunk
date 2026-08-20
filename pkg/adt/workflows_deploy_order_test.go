package adt

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateFromFile_SyntaxFailureDoesNotAcquireLock(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "zsynthetic.prog.abap")
	if err := os.WriteFile(filePath, []byte("REPORT zsynthetic.\n"), 0o600); err != nil {
		t.Fatalf("writing synthetic source fixture: %v", err)
	}

	const syntaxErrorXML = `<?xml version="1.0" encoding="UTF-8"?>
<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun">
  <chkrun:checkReport>
    <chkrun:checkMessageList>
      <chkrun:checkMessage chkrun:uri="/synthetic#start=1,1" chkrun:type="E" chkrun:shortText="synthetic syntax error"/>
    </chkrun:checkMessageList>
  </chkrun:checkReport>
</chkrun:checkRunReports>`
	mock := &methodPathMock{routes: []routedResponse{
		resp("", "discovery", http.StatusOK, ""),
		resp(http.MethodPost, "/sap/bc/adt/checkruns", http.StatusOK, syntaxErrorXML),
		resp(http.MethodPost, "/sap/bc/adt/programs/programs/ZSYNTHETIC", http.StatusOK, syntheticLocalLockXML),
	}}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	result, err := client.UpdateFromFile(context.Background(), filePath, "")
	if err != nil {
		t.Fatalf("UpdateFromFile returned error: %v", err)
	}
	if result == nil || result.Success || len(result.SyntaxErrors) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	for _, call := range mock.calls {
		if call.query.Get("_action") == "LOCK" || strings.Contains(call.path, "/source/main") {
			t.Fatalf("syntax failure should stop before lock/write, calls: %#v", mock.calls)
		}
	}
}
