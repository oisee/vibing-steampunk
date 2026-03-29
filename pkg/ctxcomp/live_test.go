package ctxcomp

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// TestCompressor_LiveAbapGitClass tests the compressor against a real abapGit class source.
// This uses inline source (captured from live SAP) to verify extraction without needing SAP.
func TestCompressor_LiveAbapGitClass(t *testing.T) {
	source := `CLASS zcl_abapgit_code_inspector DEFINITION
  PUBLIC
  CREATE PROTECTED.

  PUBLIC SECTION.

    INTERFACES zif_abapgit_code_inspector .

    METHODS constructor
      IMPORTING
        !iv_package TYPE devclass
      RAISING
        zcx_abapgit_exception .

    CLASS-METHODS get_code_inspector
      IMPORTING
        !iv_package              TYPE devclass
      RETURNING
        VALUE(ri_code_inspector) TYPE REF TO zif_abapgit_code_inspector
      RAISING
        zcx_abapgit_exception.

    CLASS-METHODS set_code_inspector
      IMPORTING
        !iv_package        TYPE devclass
        !ii_code_inspector TYPE REF TO zif_abapgit_code_inspector.

  PROTECTED SECTION.
    DATA mv_package TYPE devclass .

    METHODS create_variant
      IMPORTING
        !iv_variant       TYPE sci_chkv
      RETURNING
        VALUE(ro_variant) TYPE REF TO cl_ci_checkvariant
      RAISING
        zcx_abapgit_exception .
    METHODS cleanup
      IMPORTING
        !io_set TYPE REF TO cl_ci_objectset
      RAISING
        zcx_abapgit_exception .
  PRIVATE SECTION.

    TYPES:
      BEGIN OF ty_code_inspector_pack,
        package  TYPE devclass,
        instance TYPE REF TO zif_abapgit_code_inspector,
      END OF ty_code_inspector_pack.

    CLASS-DATA gt_code_inspector TYPE ty_code_inspector_packs.
    DATA mo_inspection TYPE REF TO cl_ci_inspection .
    DATA mv_name TYPE sci_objs .

    METHODS create_objectset
      RETURNING
        VALUE(ro_set) TYPE REF TO cl_ci_objectset .
    METHODS run_inspection
      IMPORTING
        !io_inspection TYPE REF TO cl_ci_inspection
      RETURNING
        VALUE(rt_list) TYPE scit_alvlist
      RAISING
        zcx_abapgit_exception .
ENDCLASS.

CLASS zcl_abapgit_code_inspector IMPLEMENTATION.
  METHOD constructor.
    mv_package = iv_package.
    mv_name = |{ sy-uname }_{ sy-datum }|.
  ENDMETHOD.
  METHOD create_objectset.
    DATA lt_packages TYPE zif_abapgit_sap_package=>ty_devclass_tt.
    lt_packages = zcl_abapgit_factory=>get_sap_package( mv_package )->list_subpackages( ).
    DATA ls_item TYPE zif_abapgit_definitions=>ty_item.
    IF zcl_abapgit_objects=>exists( ls_item ) = abap_false.
    ENDIF.
  ENDMETHOD.
  METHOD run_inspection.
    DATA lo_timer TYPE REF TO zcl_abapgit_timer.
    lo_timer = zcl_abapgit_timer=>create( iv_count = 1 )->start( ).
    DATA lo_settings TYPE REF TO zcl_abapgit_settings.
    lo_settings = zcl_abapgit_persist_factory=>get_settings( )->read( ).
  ENDMETHOD.
ENDCLASS.`

	// Check dependency extraction
	deps := ExtractDependencies(source)
	found := make(map[string]DependencyKind)
	for _, d := range deps {
		found[d.Name] = d.Kind
	}

	// Expected deps from this class
	expectedDeps := map[string]DependencyKind{
		"ZIF_ABAPGIT_CODE_INSPECTOR": KindInterface,
		"ZCX_ABAPGIT_EXCEPTION":     KindClass,
		"CL_CI_CHECKVARIANT":        KindClass,
		"CL_CI_OBJECTSET":           KindClass,
		"CL_CI_INSPECTION":          KindClass,
		"ZIF_ABAPGIT_SAP_PACKAGE":   KindInterface,
		"ZCL_ABAPGIT_FACTORY":       KindClass,
		"ZIF_ABAPGIT_DEFINITIONS":   KindInterface,
		"ZCL_ABAPGIT_OBJECTS":       KindClass,
		"ZCL_ABAPGIT_TIMER":         KindClass,
		"ZCL_ABAPGIT_SETTINGS":      KindClass,
		"ZCL_ABAPGIT_PERSIST_FACTORY": KindClass,
	}

	for name, kind := range expectedDeps {
		gotKind, ok := found[name]
		if !ok {
			t.Errorf("missing expected dep: %s", name)
			continue
		}
		if gotKind != kind {
			t.Errorf("dep %s: got %s, want %s", name, gotKind, kind)
		}
	}
	t.Logf("Found %d dependencies total", len(deps))

	// Check contract extraction
	contract := ExtractContract(source, KindClass)

	if !strings.Contains(contract, "constructor") {
		t.Error("contract missing constructor method")
	}
	if !strings.Contains(contract, "get_code_inspector") {
		t.Error("contract missing get_code_inspector method")
	}
	if strings.Contains(contract, "create_objectset") {
		t.Error("contract should not contain private method create_objectset")
	}
	if strings.Contains(strings.ToUpper(contract), "IMPLEMENTATION") {
		t.Error("contract should not contain IMPLEMENTATION")
	}

	// Compression ratio
	ratio := float64(len(contract)) / float64(len(source))
	t.Logf("Contract: %d bytes, Source: %d bytes, Ratio: %.1f%%", len(contract), len(source), ratio*100)

	// Test end-to-end with mock deps
	provider := &mockProvider{
		sources: map[string]string{
			"INTF:ZIF_ABAPGIT_CODE_INSPECTOR": `INTERFACE zif_abapgit_code_inspector PUBLIC.
  TYPES: BEGIN OF ty_result, kind TYPE c LENGTH 1, obj_type TYPE trobjtype, END OF ty_result.
  METHODS run IMPORTING iv_variant TYPE sci_chkv iv_save TYPE abap_bool RETURNING VALUE(rt_list) TYPE scit_alvlist.
  METHODS is_successful RETURNING VALUE(rv_success) TYPE abap_bool.
  METHODS get_summary RETURNING VALUE(rv_summary) TYPE string.
ENDINTERFACE.`,
			"CLAS:ZCX_ABAPGIT_EXCEPTION": `CLASS zcx_abapgit_exception DEFINITION PUBLIC INHERITING FROM cx_static_check.
  PUBLIC SECTION.
    CLASS-METHODS raise IMPORTING iv_text TYPE string RAISING zcx_abapgit_exception.
    CLASS-METHODS raise_with_text IMPORTING ix_previous TYPE REF TO cx_root RAISING zcx_abapgit_exception.
ENDCLASS.
CLASS zcx_abapgit_exception IMPLEMENTATION.
  METHOD raise. ENDMETHOD.
  METHOD raise_with_text. ENDMETHOD.
ENDCLASS.`,
			"CLAS:ZCL_ABAPGIT_FACTORY": `CLASS zcl_abapgit_factory DEFINITION PUBLIC.
  PUBLIC SECTION.
    CLASS-METHODS get_sap_package IMPORTING iv_package TYPE devclass RETURNING VALUE(ri_sap_package) TYPE REF TO zif_abapgit_sap_package.
ENDCLASS.
CLASS zcl_abapgit_factory IMPLEMENTATION.
  METHOD get_sap_package. ENDMETHOD.
ENDCLASS.`,
			"CLAS:ZCL_ABAPGIT_TIMER": `CLASS zcl_abapgit_timer DEFINITION PUBLIC.
  PUBLIC SECTION.
    CLASS-METHODS create IMPORTING iv_count TYPE i RETURNING VALUE(ro_timer) TYPE REF TO zcl_abapgit_timer.
    METHODS start RETURNING VALUE(ro_timer) TYPE REF TO zcl_abapgit_timer.
    METHODS end RETURNING VALUE(rv_text) TYPE string.
ENDCLASS.
CLASS zcl_abapgit_timer IMPLEMENTATION.
  METHOD create. ENDMETHOD.
  METHOD start. ENDMETHOD.
  METHOD end. ENDMETHOD.
ENDCLASS.`,
		},
	}

	comp := NewCompressor(provider, 20)
	result, err := comp.Compress(context.Background(), source, "ZCL_ABAPGIT_CODE_INSPECTOR", "CLAS")
	if err != nil {
		t.Fatalf("Compress error: %v", err)
	}

	t.Logf("Stats: %d found, %d resolved, %d failed", result.Stats.DepsFound, result.Stats.DepsResolved, result.Stats.DepsFailed)
	t.Logf("Prologue (%d lines):\n%s", result.Stats.TotalLines, result.Prologue)

	if result.Stats.DepsResolved < 3 {
		t.Errorf("expected at least 3 resolved deps, got %d", result.Stats.DepsResolved)
	}

	// Verify self-exclusion
	for _, d := range result.Dependencies {
		if d.Name == "ZCL_ABAPGIT_CODE_INSPECTOR" {
			t.Error("self should be excluded from dependencies")
		}
	}

	// Print prologue size vs original
	fmt.Printf("Original source: %d bytes\n", len(source))
	fmt.Printf("Prologue: %d bytes (%d lines)\n", len(result.Prologue), result.Stats.TotalLines)
}
