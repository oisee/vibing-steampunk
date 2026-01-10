package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// mockTransportClient is a mock for testing the ADT client.
type mockTransportClient struct {
	responses map[string]*http.Response
	requests  []*http.Request
}

func (m *mockTransportClient) Do(req *http.Request) (*http.Response, error) {
	m.requests = append(m.requests, req)

	// Match by path
	path := req.URL.Path
	if resp, ok := m.responses[path]; ok {
		return resp, nil
	}

	// Check for partial matches (for CSRF fetch)
	for key, resp := range m.responses {
		if strings.Contains(path, key) {
			return resp, nil
		}
	}

	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("Not found")),
		Header:     http.Header{},
	}, nil
}

func newTestResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"X-CSRF-Token": []string{"test-token"}},
	}
}

func TestClient_SearchObject(t *testing.T) {
	searchResponse := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/ztest" adtcore:type="PROG/P" adtcore:name="ZTEST" adtcore:packageName="$TMP"/>
</adtcore:objectReferences>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"search":    newTestResponse(searchResponse),
			"discovery": newTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	results, err := client.SearchObject(context.Background(), "ZTEST*", 10)
	if err != nil {
		t.Fatalf("SearchObject failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	if results[0].Name != "ZTEST" {
		t.Errorf("Name = %v, want ZTEST", results[0].Name)
	}
}

func TestClient_GetProgram(t *testing.T) {
	sourceCode := `REPORT ztest.
WRITE 'Hello World'.`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/programs/programs/ZTEST/source/main": newTestResponse(sourceCode),
			"discovery": newTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	source, err := client.GetSource(context.Background(), "PROG", "ztest", nil)
	if err != nil {
		t.Fatalf("GetSource(PROG) failed: %v", err)
	}

	if !strings.Contains(source, "REPORT ztest") {
		t.Errorf("Source should contain REPORT statement")
	}
	if !strings.Contains(source, "Hello World") {
		t.Errorf("Source should contain Hello World")
	}
}

func TestClient_GetClass(t *testing.T) {
	sourceCode := `CLASS zcl_test DEFINITION PUBLIC.
ENDCLASS.
CLASS zcl_test IMPLEMENTATION.
ENDCLASS.`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/oo/classes/ZCL_TEST/source/main": newTestResponse(sourceCode),
			"discovery": newTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	sources, err := client.GetClass(context.Background(), "zcl_test")
	if err != nil {
		t.Fatalf("GetClass failed: %v", err)
	}

	mainSource, ok := sources["main"]
	if !ok {
		t.Fatal("Expected 'main' source in result")
	}

	if !strings.Contains(mainSource, "CLASS zcl_test") {
		t.Errorf("Source should contain CLASS statement")
	}
}

func TestClient_NewClient(t *testing.T) {
	client := NewClient("https://sap.example.com:44300", "user", "pass",
		WithClient("100"),
		WithLanguage("DE"),
	)

	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.config.Client != "100" {
		t.Errorf("Client = %v, want 100", client.config.Client)
	}
	if client.config.Language != "DE" {
		t.Errorf("Language = %v, want DE", client.config.Language)
	}
}

func TestClient_NameNormalization(t *testing.T) {
	// Test that names are converted to uppercase
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"discovery": newTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	// Call with lowercase - should make request with uppercase
	_, _ = client.GetSource(context.Background(), "PROG", "lowercase_program", nil)

	// Check that the request used uppercase
	found := false
	for _, req := range mock.requests {
		if strings.Contains(req.URL.Path, "LOWERCASE_PROGRAM") {
			found = true
			break
		}
	}

	if !found {
		t.Error("Request should use uppercase program name")
	}
}

func TestParseSRVBMetadata(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="utf-8"?>
<srvb:serviceBinding srvb:releaseSupported="false" srvb:published="true" srvb:repair="false"
    adtcore:name="Z_RAP_TRAVEL_O2" adtcore:type="SRVB/SVB"
    adtcore:description="Travel Booking Service"
    xmlns:srvb="http://www.sap.com/adt/ddic/ServiceBindings"
    xmlns:adtcore="http://www.sap.com/adt/core">
  <srvb:binding srvb:type="ODATA" srvb:version="V2" srvb:category="0">
    <srvb:implementation adtcore:name="Z_RAP_TRAVEL_O2"/>
  </srvb:binding>
  <srvb:services srvb:name="Z_RAP_TRAVEL_O2">
    <srvb:content srvb:version="0001" srvb:releaseState="">
      <srvb:serviceDefinition adtcore:uri="/sap/bc/adt/ddic/srvd/sources/z_rap_travel"
          adtcore:type="SRVD/SRV" adtcore:name="Z_RAP_TRAVEL"/>
    </srvb:content>
  </srvb:services>
</srvb:serviceBinding>`

	result, err := parseSRVBMetadata([]byte(xmlData))
	if err != nil {
		t.Fatalf("parseSRVBMetadata failed: %v", err)
	}

	if result.Name != "Z_RAP_TRAVEL_O2" {
		t.Errorf("expected name 'Z_RAP_TRAVEL_O2', got '%s'", result.Name)
	}
	if result.Type != "SRVB/SVB" {
		t.Errorf("expected type 'SRVB/SVB', got '%s'", result.Type)
	}
	if result.Description != "Travel Booking Service" {
		t.Errorf("expected description 'Travel Booking Service', got '%s'", result.Description)
	}
	if !result.Published {
		t.Error("expected published to be true")
	}
	if result.BindingType != "ODATA" {
		t.Errorf("expected binding type 'ODATA', got '%s'", result.BindingType)
	}
	if result.BindingVersion != "V2" {
		t.Errorf("expected binding version 'V2', got '%s'", result.BindingVersion)
	}
	if result.ServiceDefName != "Z_RAP_TRAVEL" {
		t.Errorf("expected service def name 'Z_RAP_TRAVEL', got '%s'", result.ServiceDefName)
	}
}
