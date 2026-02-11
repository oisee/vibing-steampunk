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

func TestModifyMessageClassXML_AddMessage(t *testing.T) {
	// Test with namespace-prefixed XML (typical SAP format)
	xmlInput := `<?xml version="1.0" encoding="utf-8"?>
<mc:messageClass xmlns:mc="http://www.sap.com/adt/MessageClass" mc:name="ZTEST_MC" mc:description="Test Message Class">
<mc:messages mc:msgno="001" mc:msgtext="Hello &amp;1"/>
<mc:messages mc:msgno="002" mc:msgtext="World &amp;1 &amp;2"/>
</mc:messageClass>`

	result, updated, deleted, err := modifyMessageClassXML([]byte(xmlInput), map[string]string{
		"003": "New message",
	}, map[string]string{"003": "LOCK123"})
	if err != nil {
		t.Fatalf("modifyMessageClassXML failed: %v", err)
	}

	xmlStr := string(result)
	if !strings.Contains(xmlStr, `mc:msgno="003"`) {
		t.Error("Output missing new message 003")
	}
	if !strings.Contains(xmlStr, `mc:msgtext="New message"`) {
		t.Error("Output missing new message text")
	}
	if !strings.Contains(xmlStr, `mc:lockhandle="LOCK123"`) {
		t.Error("Output missing lockhandle on new message")
	}
	if !strings.Contains(xmlStr, `mc:msgno="001"`) {
		t.Error("Output missing existing message 001")
	}
	if len(updated) != 1 || updated["003"] != "New message" {
		t.Errorf("updated = %v, want {003: New message}", updated)
	}
	if len(deleted) != 0 {
		t.Errorf("deleted = %v, want empty", deleted)
	}
}

func TestModifyMessageClassXML_UpdateMessage(t *testing.T) {
	xmlInput := `<?xml version="1.0" encoding="utf-8"?>
<mc:messageClass xmlns:mc="http://www.sap.com/adt/MessageClass" mc:name="ZMC" mc:description="Test">
<mc:messages mc:msgno="001" mc:msgtext="Old text"/>
</mc:messageClass>`

	result, updated, _, err := modifyMessageClassXML([]byte(xmlInput), map[string]string{
		"001": "New text",
	}, nil)
	if err != nil {
		t.Fatalf("modifyMessageClassXML failed: %v", err)
	}

	xmlStr := string(result)
	if strings.Contains(xmlStr, "Old text") {
		t.Error("Output still contains old text")
	}
	if !strings.Contains(xmlStr, `mc:msgtext="New text"`) {
		t.Errorf("Output missing updated text. Got:\n%s", xmlStr)
	}
	if len(updated) != 1 || updated["001"] != "New text" {
		t.Errorf("updated = %v, want {001: New text}", updated)
	}
}

func TestModifyMessageClassXML_DeleteMessage(t *testing.T) {
	xmlInput := `<?xml version="1.0" encoding="utf-8"?>
<mc:messageClass xmlns:mc="http://www.sap.com/adt/MessageClass" mc:name="ZMC" mc:description="Test">
<mc:messages mc:msgno="001" mc:msgtext="Keep"/>
<mc:messages mc:msgno="002" mc:msgtext="Delete me"/>
</mc:messageClass>`

	result, _, deleted, err := modifyMessageClassXML([]byte(xmlInput), map[string]string{
		"002": "",
	}, nil)
	if err != nil {
		t.Fatalf("modifyMessageClassXML failed: %v", err)
	}

	xmlStr := string(result)
	if strings.Contains(xmlStr, "Delete me") {
		t.Error("Output still contains deleted message")
	}
	if !strings.Contains(xmlStr, `mc:msgno="001"`) {
		t.Error("Output missing kept message 001")
	}
	if len(deleted) != 1 || deleted[0] != "002" {
		t.Errorf("deleted = %v, want [002]", deleted)
	}
}

func TestModifyMessageClassXML_DeleteMessageWithChildren(t *testing.T) {
	// Test deleting a message that has child elements (atom:link) - paired closing tag
	xmlInput := `<?xml version="1.0" encoding="utf-8"?>
<mc:messageClass xmlns:mc="http://www.sap.com/adt/MessageClass" mc:name="ZMC" mc:description="Test">
<mc:messages mc:msgno="001" mc:msgtext="Keep">
  <atom:link href="/sap/bc/adt/messageclass/zmc/messages/001" rel="http://www.sap.com/adt/relations/source" type="text/plain"/>
</mc:messages>
<mc:messages mc:msgno="002" mc:msgtext="Delete me">
  <atom:link href="/sap/bc/adt/messageclass/zmc/messages/002" rel="http://www.sap.com/adt/relations/source" type="text/plain"/>
</mc:messages>
</mc:messageClass>`

	result, _, deleted, err := modifyMessageClassXML([]byte(xmlInput), map[string]string{
		"002": "",
	}, nil)
	if err != nil {
		t.Fatalf("modifyMessageClassXML failed: %v", err)
	}

	xmlStr := string(result)
	if strings.Contains(xmlStr, "Delete me") {
		t.Errorf("Output still contains deleted message. Got:\n%s", xmlStr)
	}
	if strings.Contains(xmlStr, "messages/002") {
		t.Errorf("Output still contains deleted message's atom:link. Got:\n%s", xmlStr)
	}
	if !strings.Contains(xmlStr, `mc:msgno="001"`) {
		t.Error("Output missing kept message 001")
	}
	if !strings.Contains(xmlStr, "messages/001") {
		t.Error("Output missing kept message's atom:link")
	}
	if len(deleted) != 1 || deleted[0] != "002" {
		t.Errorf("deleted = %v, want [002]", deleted)
	}
}

func TestModifyMessageClassXML_NoNamespace(t *testing.T) {
	// Test with non-namespaced XML
	xmlInput := `<?xml version="1.0" encoding="utf-8"?>
<messageClass name="ZMC_SIMPLE" description="Simple">
<messages msgno="010" msgtext="Test"/>
</messageClass>`

	result, updated, _, err := modifyMessageClassXML([]byte(xmlInput), map[string]string{
		"010": "Updated",
		"020": "New",
	}, map[string]string{"020": "LOCK456"})
	if err != nil {
		t.Fatalf("modifyMessageClassXML failed: %v", err)
	}

	xmlStr := string(result)
	if !strings.Contains(xmlStr, `msgtext="Updated"`) {
		t.Errorf("Output missing updated text. Got:\n%s", xmlStr)
	}
	if !strings.Contains(xmlStr, `msgno="020"`) {
		t.Errorf("Output missing new message 020. Got:\n%s", xmlStr)
	}
	if len(updated) != 2 {
		t.Errorf("updated count = %d, want 2", len(updated))
	}
}

func TestModifyMessageClassXML_EscapeSpecialChars(t *testing.T) {
	xmlInput := `<?xml version="1.0" encoding="utf-8"?>
<messageClass name="ZMC" description="Test">
</messageClass>`

	result, _, _, err := modifyMessageClassXML([]byte(xmlInput), map[string]string{
		"001": `Value with "quotes" & <angles>`,
	}, map[string]string{"001": "LOCKESC"})
	if err != nil {
		t.Fatalf("modifyMessageClassXML failed: %v", err)
	}

	xmlStr := string(result)
	if !strings.Contains(xmlStr, "&amp;") {
		t.Error("& not escaped")
	}
	if !strings.Contains(xmlStr, "&lt;") {
		t.Error("< not escaped")
	}
	if !strings.Contains(xmlStr, "&gt;") {
		t.Error("> not escaped")
	}
	if !strings.Contains(xmlStr, "&quot;") {
		t.Error("\" not escaped")
	}
}
