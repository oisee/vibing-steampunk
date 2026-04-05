package adt

import (
	"context"
	"net/http"
	"testing"
)

// --- DDIC Read Tests ---

func TestClient_GetSearchHelp(t *testing.T) {
	source := `@AbapCatalog.searchHelp.deliveryClass : #A
define search help ZSHLP_TEST
  as elementary search help for SFLIGHT
  with parameters
    CARRID : S_CARR_ID,
    CONNID : S_CONN_ID
end`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/ddic/searchhelps/ZSHLP_TEST/source/main": newTestResponse(source),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GetSearchHelp(context.Background(), "zshlp_test")
	if err != nil {
		t.Fatalf("GetSearchHelp failed: %v", err)
	}

	if result != source {
		t.Errorf("Expected source to match, got %q", result)
	}
}

func TestClient_GetLockObject(t *testing.T) {
	source := `@AbapCatalog.lockObject.deliveryClass : #A
define lock object EZLOCK_TEST
  for table SFLIGHT
  with lock mode default write
end`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/ddic/lockobjects/EZLOCK_TEST/source/main": newTestResponse(source),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GetLockObject(context.Background(), "ezlock_test")
	if err != nil {
		t.Fatalf("GetLockObject failed: %v", err)
	}

	if result != source {
		t.Errorf("Expected source to match, got %q", result)
	}
}

func TestClient_GetTypeGroup(t *testing.T) {
	source := `TYPE-POOL: ZTYP1.
TYPES: BEGIN OF ztyp1_struct,
         field1 TYPE c LENGTH 10,
         field2 TYPE i,
       END OF ztyp1_struct.`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/ddic/typegroups/ZTYP1/source/main": newTestResponse(source),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GetTypeGroup(context.Background(), "ztyp1")
	if err != nil {
		t.Fatalf("GetTypeGroup failed: %v", err)
	}

	if result != source {
		t.Errorf("Expected source to match, got %q", result)
	}
}

func TestClient_GetSearchHelp_UpperCase(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/ddic/searchhelps/ZSHLP_LOWER/source/main": newTestResponse("found"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	// Lowercase input should be uppercased
	result, err := client.GetSearchHelp(context.Background(), "zshlp_lower")
	if err != nil {
		t.Fatalf("GetSearchHelp uppercase conversion failed: %v", err)
	}

	if result != "found" {
		t.Errorf("Expected 'found', got %q", result)
	}
}

func TestClient_AddObjectToTransport(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/core/discovery":                    newCSRFResponse(),
			"/sap/bc/adt/cts/transportrequests/S23K900123": newTestResponse(""),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	cfg.Safety.EnableTransports = true
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	err := client.AddObjectToTransport(context.Background(), "S23K900123", "R3TR", "PROG", "ZTEST_PROGRAM")
	if err != nil {
		t.Fatalf("AddObjectToTransport failed: %v", err)
	}
}

func TestClient_AddObjectToTransport_DefaultPGMID(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/core/discovery":                    newCSRFResponse(),
			"/sap/bc/adt/cts/transportrequests/S23K900123": newTestResponse(""),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	cfg.Safety.EnableTransports = true
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	// Empty pgmid should default to R3TR
	err := client.AddObjectToTransport(context.Background(), "S23K900123", "", "CLAS", "ZCL_TEST")
	if err != nil {
		t.Fatalf("AddObjectToTransport with default pgmid failed: %v", err)
	}
}

func TestClient_AddObjectToTransport_MissingParams(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, &mockTransportClient{responses: map[string]*http.Response{}})
	client := NewClientWithTransport(cfg, transport)

	// Missing transport number
	err := client.AddObjectToTransport(context.Background(), "", "R3TR", "PROG", "ZTEST")
	if err == nil {
		t.Error("Expected error for empty transport number")
	}

	// Missing object type
	err = client.AddObjectToTransport(context.Background(), "S23K900123", "R3TR", "", "ZTEST")
	if err == nil {
		t.Error("Expected error for empty object type")
	}

	// Missing object name
	err = client.AddObjectToTransport(context.Background(), "S23K900123", "R3TR", "PROG", "")
	if err == nil {
		t.Error("Expected error for empty object name")
	}
}

func TestClient_AddObjectToTransport_ReadOnly(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	cfg.Safety.ReadOnly = true
	transport := NewTransportWithClient(cfg, &mockTransportClient{responses: map[string]*http.Response{}})
	client := NewClientWithTransport(cfg, transport)

	err := client.AddObjectToTransport(context.Background(), "S23K900123", "R3TR", "PROG", "ZTEST")
	if err == nil {
		t.Error("Expected error for read-only mode")
	}
}
