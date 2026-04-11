package graph

import (
	"strings"
	"testing"
)

const testClassDef = `CLASS zcl_travel DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    TYPES ty_travel TYPE ztravel_tab.
    CLASS-METHODS factory
      IMPORTING iv_id TYPE string
      RETURNING VALUE(ro_instance) TYPE REF TO zcl_travel.
    METHODS get_data
      IMPORTING iv_id TYPE string
      EXPORTING es_result TYPE ty_travel
      RAISING zcx_not_found zcx_auth_error.
    METHODS create
      IMPORTING is_travel TYPE ty_travel
      RETURNING VALUE(rv_id) TYPE string.
  PROTECTED SECTION.
    METHODS validate
      IMPORTING is_travel TYPE ty_travel
      RETURNING VALUE(rv_valid) TYPE abap_bool.
  PRIVATE SECTION.
    DATA mv_data TYPE ty_travel.
    METHODS helper
      IMPORTING iv_input TYPE string OPTIONAL
                iv_mode TYPE char1 DEFAULT 'A'
      CHANGING ct_log TYPE string_table.
ENDCLASS.`

func TestExtractMethodSignature_PublicMethod(t *testing.T) {
	sig := ExtractMethodSignature("ZCL_TRAVEL", "GET_DATA", testClassDef)

	if sig.MethodName != "GET_DATA" {
		t.Errorf("MethodName: got %q", sig.MethodName)
	}
	if sig.Visibility != "PUBLIC" {
		t.Errorf("Visibility: got %q, want PUBLIC", sig.Visibility)
	}
	if sig.Level != "instance" {
		t.Errorf("Level: got %q, want instance", sig.Level)
	}

	// Should have IMPORTING and EXPORTING params
	impCount := 0
	expCount := 0
	for _, p := range sig.Params {
		switch p.Direction {
		case "IMPORTING":
			impCount++
		case "EXPORTING":
			expCount++
		}
	}
	if impCount != 1 {
		t.Errorf("IMPORTING params: got %d, want 1", impCount)
	}
	if expCount != 1 {
		t.Errorf("EXPORTING params: got %d, want 1", expCount)
	}

	// RAISING
	if len(sig.Raising) != 2 {
		t.Errorf("RAISING: got %d, want 2", len(sig.Raising))
	}
}

func TestExtractMethodSignature_StaticMethod(t *testing.T) {
	sig := ExtractMethodSignature("ZCL_TRAVEL", "FACTORY", testClassDef)

	if sig.Level != "static" {
		t.Errorf("Level: got %q, want static", sig.Level)
	}

	// RETURNING VALUE(ro_instance) TYPE REF TO zcl_travel
	retCount := 0
	for _, p := range sig.Params {
		if p.Direction == "RETURNING" {
			retCount++
			if p.Name != "RO_INSTANCE" {
				t.Errorf("RETURNING name: got %q", p.Name)
			}
			if !strings.Contains(p.Type, "REF TO") {
				t.Errorf("RETURNING type should contain REF TO: got %q", p.Type)
			}
		}
	}
	if retCount != 1 {
		t.Errorf("RETURNING params: got %d, want 1", retCount)
	}
}

func TestExtractMethodSignature_ProtectedMethod(t *testing.T) {
	sig := ExtractMethodSignature("ZCL_TRAVEL", "VALIDATE", testClassDef)

	if sig.Visibility != "PROTECTED" {
		t.Errorf("Visibility: got %q, want PROTECTED", sig.Visibility)
	}
}

func TestExtractMethodSignature_PrivateMethod(t *testing.T) {
	sig := ExtractMethodSignature("ZCL_TRAVEL", "HELPER", testClassDef)

	if sig.Visibility != "PRIVATE" {
		t.Errorf("Visibility: got %q, want PRIVATE", sig.Visibility)
	}

	// Check OPTIONAL and DEFAULT
	var optParam, defParam *MethodParam
	for i, p := range sig.Params {
		if p.Name == "IV_INPUT" {
			optParam = &sig.Params[i]
		}
		if p.Name == "IV_MODE" {
			defParam = &sig.Params[i]
		}
	}
	if optParam == nil {
		t.Fatal("IV_INPUT param not found")
	}
	if !optParam.Optional {
		t.Error("IV_INPUT should be OPTIONAL")
	}
	if defParam == nil {
		t.Fatal("IV_MODE param not found")
	}
	if defParam.Default == "" {
		t.Error("IV_MODE should have DEFAULT value")
	}

	// Check CHANGING param
	changingCount := 0
	for _, p := range sig.Params {
		if p.Direction == "CHANGING" {
			changingCount++
		}
	}
	if changingCount != 1 {
		t.Errorf("CHANGING params: got %d, want 1", changingCount)
	}
}

func TestExtractMethodSignature_NotFound(t *testing.T) {
	sig := ExtractMethodSignature("ZCL_TRAVEL", "NONEXISTENT", testClassDef)

	if sig.RawDef != "" {
		t.Error("Non-existent method should have empty RawDef")
	}
	if len(sig.Params) != 0 {
		t.Errorf("Non-existent method should have 0 params, got %d", len(sig.Params))
	}
}

func TestExtractMethodSignature_Redefinition(t *testing.T) {
	source := `CLASS zcl_child DEFINITION INHERITING FROM zcl_parent.
  PUBLIC SECTION.
    METHODS get_data REDEFINITION.
ENDCLASS.`

	sig := ExtractMethodSignature("ZCL_CHILD", "GET_DATA", source)

	if !sig.IsRedefined {
		t.Error("Should detect REDEFINITION")
	}
	if len(sig.Params) != 0 {
		t.Errorf("Redefined method should have 0 params in definition, got %d", len(sig.Params))
	}
}

func TestExtractMethodSignature_CreateMethod(t *testing.T) {
	sig := ExtractMethodSignature("ZCL_TRAVEL", "CREATE", testClassDef)

	if sig.MethodName != "CREATE" {
		t.Errorf("MethodName: got %q", sig.MethodName)
	}
	// Should have IMPORTING + RETURNING
	dirs := map[string]int{}
	for _, p := range sig.Params {
		dirs[p.Direction]++
	}
	if dirs["IMPORTING"] != 1 {
		t.Errorf("IMPORTING: got %d, want 1", dirs["IMPORTING"])
	}
	if dirs["RETURNING"] != 1 {
		t.Errorf("RETURNING: got %d, want 1", dirs["RETURNING"])
	}
}

func TestFormatMethodSignature(t *testing.T) {
	sig := ExtractMethodSignature("ZCL_TRAVEL", "GET_DATA", testClassDef)
	text := FormatMethodSignature(sig)

	if !strings.Contains(text, "GET_DATA") {
		t.Error("Should contain method name")
	}
	if !strings.Contains(text, "IMPORTING") {
		t.Error("Should contain IMPORTING section")
	}
	if !strings.Contains(text, "EXPORTING") {
		t.Error("Should contain EXPORTING section")
	}
	if !strings.Contains(text, "RAISING") {
		t.Error("Should contain RAISING section")
	}
}

func TestExtractMethodSignature_EmptySource(t *testing.T) {
	sig := ExtractMethodSignature("ZCL_FOO", "BAR", "")
	if sig.RawDef != "" {
		t.Error("Empty source should produce empty sig")
	}
}
