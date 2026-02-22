package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestDetectSignatureChanges_ParamAdded(t *testing.T) {
	old := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS do_something IMPORTING iv_name TYPE string.
ENDCLASS.`

	new := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS do_something IMPORTING iv_name TYPE string iv_age TYPE i.
ENDCLASS.`

	findings := DetectSignatureChanges(old, new)
	if len(findings) == 0 {
		t.Error("expected changed_signature finding when parameter added")
	}
	if findings[0].Rule != "changed_signature" {
		t.Errorf("expected rule changed_signature, got %s", findings[0].Rule)
	}
}

func TestDetectSignatureChanges_ParamTypeChanged(t *testing.T) {
	old := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS process IMPORTING iv_value TYPE string.
ENDCLASS.`

	new := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS process IMPORTING iv_value TYPE i.
ENDCLASS.`

	findings := DetectSignatureChanges(old, new)
	if len(findings) == 0 {
		t.Error("expected changed_signature finding when parameter type changed")
	}
}

func TestDetectRemovedPublicMethods_Removed(t *testing.T) {
	old := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS do_something.
    METHODS do_other.
ENDCLASS.`

	new := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS do_other.
ENDCLASS.`

	findings := DetectRemovedPublicMethods(old, new)
	if len(findings) == 0 {
		t.Error("expected removed_public_method finding")
	}
	found := false
	for _, f := range findings {
		if f.Rule == "removed_public_method" {
			found = true
		}
	}
	if !found {
		t.Error("expected removed_public_method rule")
	}
}

func TestDetectRemovedPublicMethods_Renamed(t *testing.T) {
	old := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS old_method.
ENDCLASS.`

	new := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS new_method.
ENDCLASS.`

	findings := DetectRemovedPublicMethods(old, new)
	if len(findings) == 0 {
		t.Error("expected removed_public_method when method is renamed (detected as remove)")
	}
}

func TestDetectInterfaceChanges_MethodAdded(t *testing.T) {
	old := `INTERFACE zif_test PUBLIC.
  METHODS do_something.
ENDINTERFACE.`

	new := `INTERFACE zif_test PUBLIC.
  METHODS do_something.
  METHODS do_other.
ENDINTERFACE.`

	findings := DetectInterfaceChanges(old, new)
	if len(findings) == 0 {
		t.Error("expected changed_interface_method finding when method added to interface")
	}
}

func TestDetectExceptionChanges_RaisingChanged(t *testing.T) {
	old := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS process RAISING cx_sy_open_sql_db.
ENDCLASS.`

	new := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS process RAISING cx_sy_open_sql_db cx_sy_dynamic_osql_error.
ENDCLASS.`

	findings := DetectExceptionChanges(old, new)
	if len(findings) == 0 {
		t.Error("expected changed_exception_handling finding when RAISING clause changes")
	}
}

func TestDetectRegressions_NoChanges(t *testing.T) {
	source := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS do_something IMPORTING iv_name TYPE string.
ENDCLASS.`

	findings := DetectSignatureChanges(source, source)
	findings = append(findings, DetectRemovedPublicMethods(source, source)...)
	findings = append(findings, DetectInterfaceChanges(source, source)...)
	findings = append(findings, DetectExceptionChanges(source, source)...)

	if len(findings) != 0 {
		t.Errorf("expected 0 findings for identical sources, got %d", len(findings))
	}
}

func TestDetectExceptionChanges_MultiLineRaising(t *testing.T) {
	// Multi-line METHODS definition where RAISING is on a different line
	old := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS process
      IMPORTING iv_value TYPE string
      RAISING cx_sy_open_sql_db.
ENDCLASS.`

	new := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS process
      IMPORTING iv_value TYPE string
      RAISING cx_sy_open_sql_db cx_sy_dynamic_osql_error.
ENDCLASS.`

	findings := DetectExceptionChanges(old, new)
	if len(findings) == 0 {
		t.Error("expected changed_exception_handling for multi-line METHODS with RAISING change")
	}
}

func TestDetectExceptionChanges_MultiMethodNoFalseAttribution(t *testing.T) {
	// Two methods: only second has RAISING. Ensure RAISING is attributed to correct method.
	old := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS do_simple
      IMPORTING iv_value TYPE string.
    METHODS do_complex
      IMPORTING iv_data TYPE ref to data
      RAISING cx_sy_open_sql_db.
ENDCLASS.`

	new := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS do_simple
      IMPORTING iv_value TYPE string.
    METHODS do_complex
      IMPORTING iv_data TYPE ref to data
      RAISING cx_sy_open_sql_db cx_sy_dynamic_osql_error.
ENDCLASS.`

	findings := DetectExceptionChanges(old, new)
	if len(findings) == 0 {
		t.Error("expected changed_exception_handling finding for do_complex")
	}
	for _, f := range findings {
		if strings.Contains(f.Description, "do_simple") || strings.Contains(strings.ToLower(f.Match), "do_simple") {
			t.Errorf("RAISING wrongly attributed to do_simple: %s", f.Description)
		}
	}
}

func TestParseObjectURIComponents(t *testing.T) {
	tests := []struct {
		uri      string
		wantType string
		wantName string
	}{
		{"/sap/bc/adt/oo/classes/zcl_test", "CLAS", "ZCL_TEST"},
		{"/sap/bc/adt/oo/interfaces/zif_test", "INTF", "ZIF_TEST"},
		{"/sap/bc/adt/programs/programs/ztest", "PROG", "ZTEST"},
		{"/sap/bc/adt/functions/groups/zfg_test", "FUGR", "ZFG_TEST"},
		{"/sap/bc/adt/ddic/ddl/sources/zddls_test", "DDLS", "ZDDLS_TEST"},
		{"/sap/bc/adt/unknown/path/zobject", "", "ZOBJECT"},
	}
	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			gotType, gotName := parseObjectURIComponents(tt.uri)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
			if gotName != tt.wantName {
				t.Errorf("name = %q, want %q", gotName, tt.wantName)
			}
		})
	}
}

func TestClient_CheckRegression(t *testing.T) {
	currentSource := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS process IMPORTING iv_value TYPE i.
ENDCLASS.`

	baseSource := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS process IMPORTING iv_value TYPE string.
    METHODS old_method.
ENDCLASS.`

	revisionFeed := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>2</id>
    <title>Version 2</title>
    <updated>2026-02-20T10:00:00Z</updated>
    <author><name>TESTUSER</name></author>
    <content type="text" src="/sap/bc/adt/oo/classes/zcl_test/includes/main/versions/2/content"/>
  </entry>
  <entry>
    <id>1</id>
    <title>Version 1</title>
    <updated>2026-02-19T10:00:00Z</updated>
    <author><name>TESTUSER</name></author>
    <content type="text" src="/sap/bc/adt/oo/classes/zcl_test/includes/main/versions/1/content"/>
  </entry>
</feed>`

	// Use full paths for exact matching to avoid ambiguous partial matches
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/oo/classes/zcl_test/source/main":                     newTestResponse(currentSource),
			"/sap/bc/adt/oo/classes/ZCL_TEST/includes/main/versions":          newTestResponse(revisionFeed),
			"/sap/bc/adt/oo/classes/zcl_test/includes/main/versions/1/content": newTestResponse(baseSource),
			"discovery": newTestResponse("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.CheckRegression(context.Background(), "/sap/bc/adt/oo/classes/zcl_test", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Summary.RiskLevel != "breaking" {
		t.Errorf("expected 'breaking' risk level, got %q", result.Summary.RiskLevel)
	}
	if result.Summary.SignatureChanges == 0 {
		t.Error("expected signature changes")
	}
	if result.Summary.RemovedMethods == 0 {
		t.Error("expected removed methods")
	}
}
