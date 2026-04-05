package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestRenameEvaluate(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="UTF-8"?>
<renameRefactoring xmlns:generic="http://www.sap.com/adt/refactoring/genericrefactoring">
  <oldName>lv_count</oldName>
  <newName>lv_counter</newName>
  <generic:genericRefactoring>
    <generic:title>Rename Variable</generic:title>
    <generic:adtObjectUri>/sap/bc/adt/oo/classes/zcl_test#start=10,5;end=10,13</generic:adtObjectUri>
    <generic:affectedObjects>
      <generic:affectedObject name="ZCL_TEST" parentUri="/sap/bc/adt/packages/%24tmp" type="CLAS" uri="/sap/bc/adt/oo/classes/zcl_test"/>
    </generic:affectedObjects>
    <generic:transport/>
  </generic:genericRefactoring>
</renameRefactoring>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"refactorings": newTestResponse(xmlResp),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	transport.csrfToken = "test-token"
	client := NewClientWithTransport(cfg, transport)

	result, err := client.RenameEvaluate(context.Background(), "/sap/bc/adt/oo/classes/zcl_test#start=10,5;end=10,13")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.OldName != "lv_count" {
		t.Errorf("OldName = %q, want %q", result.OldName, "lv_count")
	}
	if result.Step != "evaluate" {
		t.Errorf("Step = %q, want %q", result.Step, "evaluate")
	}
	if len(result.AffectedObjects) != 1 {
		t.Fatalf("AffectedObjects count = %d, want 1", len(result.AffectedObjects))
	}
	if result.AffectedObjects[0].Name != "ZCL_TEST" {
		t.Errorf("AffectedObject name = %q, want %q", result.AffectedObjects[0].Name, "ZCL_TEST")
	}
}

func TestRenameExecute_Safety(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithReadOnly())
	mock := &mockTransportClient{responses: map[string]*http.Response{}}
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	_, err := client.RenameExecute(context.Background(), "/sap/bc/adt/oo/classes/zcl_test#start=1,1;end=1,5", "new_name", "")
	if err == nil {
		t.Fatal("expected error for read-only mode")
	}
	if !strings.Contains(err.Error(), "read-only") && !strings.Contains(err.Error(), "blocked") {
		t.Errorf("expected safety error, got: %v", err)
	}
}

func TestGetQuickFixProposals(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="UTF-8"?>
<evaluationResults xmlns:qf="http://www.sap.com/adt/quickfixes">
  <evaluationResult>
    <qf:id>fix001</qf:id>
    <qf:description>Add missing method implementation</qf:description>
  </evaluationResult>
  <evaluationResult>
    <qf:id>fix002</qf:id>
    <qf:description>Remove unused variable</qf:description>
  </evaluationResult>
</evaluationResults>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"quickfixes": newTestResponse(xmlResp),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	transport.csrfToken = "test-token"
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GetQuickFixProposals(context.Background(),
		"/sap/bc/adt/oo/classes/zcl_test#start=10,5", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Count != 2 {
		t.Errorf("Count = %d, want 2", result.Count)
	}
}
