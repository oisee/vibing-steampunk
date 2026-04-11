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

func TestClient_GetAPIReleaseState(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<apiRelease>
  <releasableObject uri="/sap/bc/adt/oo/classes/cl_abap_typedescr" type="CLAS" name="CL_ABAP_TYPEDESCR"/>
  <c1Release contract="C1" useInKeyUserApps="false" useInSAPCloudPlatform="true" name="CL_ABAP_TYPEDESCR">
    <status state="RELEASED" stateDescription="Released"/>
  </c1Release>
  <apiCatalogData isAnyAssignmentPossible="true" isAnyContractReleased="true"/>
</apiRelease>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"apireleases": newTestResponse(xmlResponse),
			"discovery":   newTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	state, err := client.GetAPIReleaseState(context.Background(), "/sap/bc/adt/oo/classes/cl_abap_typedescr")
	if err != nil {
		t.Fatalf("GetAPIReleaseState failed: %v", err)
	}

	if state.C1 == nil {
		t.Fatal("C1 release should not be nil")
	}
	if state.C1.Status.State != "RELEASED" {
		t.Errorf("C1 Status.State = %q, want RELEASED", state.C1.Status.State)
	}
	if state.C1.UseInSAPCloudPlatform != true {
		t.Error("C1 UseInSAPCloudPlatform should be true")
	}
	if state.C1.UseInKeyUserApps {
		t.Error("C1 UseInKeyUserApps should be false")
	}
	if !state.Catalog.IsAnyContractReleased {
		t.Error("Catalog.IsAnyContractReleased should be true")
	}

	// Verify the request URL uses the apireleases endpoint
	if len(mock.requests) == 0 {
		t.Fatal("No requests recorded")
	}
	lastReq := mock.requests[len(mock.requests)-1]
	if !strings.Contains(lastReq.URL.Path, "apireleases") {
		t.Errorf("Request path %q should contain 'apireleases'", lastReq.URL.Path)
	}
}

func TestClient_GetAPIReleaseState_WithDeprecation(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<apiRelease>
  <releasableObject uri="/sap/bc/adt/oo/classes/cl_old_api" type="CLAS" name="CL_OLD_API"/>
  <c1Release contract="C1" useInKeyUserApps="false" useInSAPCloudPlatform="true" name="CL_OLD_API">
    <status state="DEPRECATED" stateDescription="Deprecated"/>
    <successors>
      <successor uri="/sap/bc/adt/oo/classes/cl_abap_typedescr_v2" type="CLAS" name="CL_ABAP_TYPEDESCR_V2"/>
    </successors>
  </c1Release>
  <apiCatalogData isAnyAssignmentPossible="true" isAnyContractReleased="true"/>
</apiRelease>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"apireleases": newTestResponse(xmlResponse),
			"discovery":   newTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	state, err := client.GetAPIReleaseState(context.Background(), "/sap/bc/adt/oo/classes/cl_old_api")
	if err != nil {
		t.Fatalf("GetAPIReleaseState failed: %v", err)
	}

	if state.C1 == nil {
		t.Fatal("C1 release should not be nil")
	}
	if state.C1.Status.State != "DEPRECATED" {
		t.Errorf("C1 Status.State = %q, want DEPRECATED", state.C1.Status.State)
	}
	if len(state.C1.Successors) == 0 {
		t.Fatal("C1 Successors should not be empty")
	}
	if state.C1.Successors[0].URI != "/sap/bc/adt/oo/classes/cl_abap_typedescr_v2" {
		t.Errorf("Successor URI = %q, want /sap/bc/adt/oo/classes/cl_abap_typedescr_v2", state.C1.Successors[0].URI)
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

	source, err := client.GetProgram(context.Background(), "ztest")
	if err != nil {
		t.Fatalf("GetProgram failed: %v", err)
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
	_, _ = client.GetProgram(context.Background(), "lowercase_program")

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
