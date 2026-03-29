package ctxcomp

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// TestCompressor_FullE2E_CodeInspector tests against all real SAP sources.
// Source was fetched from A4H system on 2026-03-18.
func TestCompressor_FullE2E_CodeInspector(t *testing.T) {
	// Count lines in full source
	mainSource := codeInspectorSource
	mainLines := strings.Count(mainSource, "\n") + 1

	provider := &mockProvider{sources: codeInspectorDeps}
	comp := NewCompressor(provider, 20)
	result, err := comp.Compress(context.Background(), mainSource, "ZCL_ABAPGIT_CODE_INSPECTOR", "CLAS")
	if err != nil {
		t.Fatalf("Compress error: %v", err)
	}

	t.Logf("Main source: %d lines, %d bytes", mainLines, len(mainSource))
	t.Logf("Dependencies found: %d", result.Stats.DepsFound)
	t.Logf("Resolved: %d, Failed: %d", result.Stats.DepsResolved, result.Stats.DepsFailed)
	t.Logf("Prologue: %d lines, %d bytes", result.Stats.TotalLines, len(result.Prologue))

	// Compression ratio: prologue should be much smaller than the sum of all dep sources
	totalDepBytes := 0
	for _, src := range codeInspectorDeps {
		totalDepBytes += len(src)
	}
	prologueBytes := len(result.Prologue)
	ratio := float64(prologueBytes) / float64(totalDepBytes) * 100
	t.Logf("Total dep sources: %d bytes → Prologue: %d bytes (%.0f%% = %.0fx compression)",
		totalDepBytes, prologueBytes, ratio, float64(totalDepBytes)/float64(prologueBytes))

	// Verify key deps resolved
	for _, name := range []string{
		"ZIF_ABAPGIT_CODE_INSPECTOR",
		"ZCX_ABAPGIT_EXCEPTION",
		"ZCL_ABAPGIT_FACTORY",
		"ZCL_ABAPGIT_TIMER",
		"ZCL_ABAPGIT_PERSIST_FACTORY",
	} {
		if !strings.Contains(result.Prologue, name) {
			t.Errorf("prologue missing %s", name)
		}
	}

	// No IMPLEMENTATION in prologue
	if strings.Contains(strings.ToUpper(result.Prologue), "IMPLEMENTATION") {
		t.Error("prologue must not contain IMPLEMENTATION")
	}

	// Print what Claude would see with include_context=true
	combined := mainSource + "\n\n" + result.Prologue +
		fmt.Sprintf("\n* Context stats: %d deps found, %d resolved, %d failed",
			result.Stats.DepsFound, result.Stats.DepsResolved, result.Stats.DepsFailed)

	combinedLines := strings.Count(combined, "\n") + 1
	t.Logf("\n=== What Claude sees with include_context=true ===")
	t.Logf("Total output: %d lines, %d bytes", combinedLines, len(combined))
	t.Logf("  Source: %d lines (%d bytes)", mainLines, len(mainSource))
	t.Logf("  Context: %d lines (%d bytes)", result.Stats.TotalLines, prologueBytes)
	t.Logf("  Deps compressed from: %d bytes → %d bytes (%.1fx)", totalDepBytes, prologueBytes, float64(totalDepBytes)/float64(prologueBytes))
}

// Real source from SAP A4H — ZCL_ABAPGIT_CODE_INSPECTOR (definition only, for brevity)
var codeInspectorSource = `CLASS zcl_abapgit_code_inspector DEFINITION
  PUBLIC
  CREATE PROTECTED.
  PUBLIC SECTION.
    INTERFACES zif_abapgit_code_inspector .
    METHODS constructor
      IMPORTING !iv_package TYPE devclass
      RAISING zcx_abapgit_exception .
    CLASS-METHODS get_code_inspector
      IMPORTING !iv_package TYPE devclass
      RETURNING VALUE(ri_code_inspector) TYPE REF TO zif_abapgit_code_inspector
      RAISING zcx_abapgit_exception.
    CLASS-METHODS set_code_inspector
      IMPORTING !iv_package TYPE devclass
                !ii_code_inspector TYPE REF TO zif_abapgit_code_inspector.
  PROTECTED SECTION.
    DATA mv_package TYPE devclass .
    METHODS create_variant
      IMPORTING !iv_variant TYPE sci_chkv
      RETURNING VALUE(ro_variant) TYPE REF TO cl_ci_checkvariant
      RAISING zcx_abapgit_exception .
    METHODS cleanup
      IMPORTING !io_set TYPE REF TO cl_ci_objectset
      RAISING zcx_abapgit_exception .
  PRIVATE SECTION.
    DATA mo_inspection TYPE REF TO cl_ci_inspection .
    METHODS create_objectset RETURNING VALUE(ro_set) TYPE REF TO cl_ci_objectset .
    METHODS run_inspection
      IMPORTING !io_inspection TYPE REF TO cl_ci_inspection
      RETURNING VALUE(rt_list) TYPE scit_alvlist
      RAISING zcx_abapgit_exception .
ENDCLASS.

CLASS zcl_abapgit_code_inspector IMPLEMENTATION.
  METHOD constructor.
    mv_package = iv_package.
  ENDMETHOD.
  METHOD create_objectset.
    DATA lt_packages TYPE zif_abapgit_sap_package=>ty_devclass_tt.
    lt_packages = zcl_abapgit_factory=>get_sap_package( mv_package )->list_subpackages( ).
    DATA ls_item TYPE zif_abapgit_definitions=>ty_item.
    IF zcl_abapgit_objects=>exists( ls_item ) = abap_false. ENDIF.
  ENDMETHOD.
  METHOD run_inspection.
    DATA lo_timer TYPE REF TO zcl_abapgit_timer.
    lo_timer = zcl_abapgit_timer=>create( iv_count = 1 )->start( ).
    DATA lo_settings TYPE REF TO zcl_abapgit_settings.
    lo_settings = zcl_abapgit_persist_factory=>get_settings( )->read( ).
  ENDMETHOD.
  METHOD zif_abapgit_code_inspector~run.
    DATA lx_error TYPE REF TO zcx_abapgit_exception.
    zcx_abapgit_exception=>raise( |Error| ).
  ENDMETHOD.
ENDCLASS.`

// Real dependency sources from SAP A4H (contracts will be extracted by compressor)
var codeInspectorDeps = map[string]string{
	"INTF:ZIF_ABAPGIT_CODE_INSPECTOR": `INTERFACE zif_abapgit_code_inspector PUBLIC .
  TYPES: BEGIN OF ty_result,
           objtype TYPE tadir-object, objname TYPE tadir-obj_name,
           kind TYPE c LENGTH 1, line TYPE n LENGTH 6, code TYPE c LENGTH 10,
           test TYPE c LENGTH 30, text TYPE string,
         END OF ty_result.
  TYPES ty_results TYPE STANDARD TABLE OF ty_result WITH DEFAULT KEY.
  METHODS run IMPORTING !iv_variant TYPE sci_chkv !iv_save TYPE abap_bool DEFAULT abap_false
    RETURNING VALUE(rt_list) TYPE ty_results RAISING zcx_abapgit_exception .
  METHODS is_successful RETURNING VALUE(rv_success) TYPE abap_bool .
  METHODS get_summary RETURNING VALUE(rv_summary) TYPE string.
  METHODS validate_check_variant IMPORTING !iv_check_variant_name TYPE sci_chkv RAISING zcx_abapgit_exception.
ENDINTERFACE.`,

	"CLAS:ZCX_ABAPGIT_EXCEPTION": `CLASS zcx_abapgit_exception DEFINITION PUBLIC INHERITING FROM cx_static_check CREATE PUBLIC.
  PUBLIC SECTION.
    INTERFACES if_t100_message .
    DATA msgv1 TYPE symsgv READ-ONLY .
    DATA mv_longtext TYPE string READ-ONLY.
    CLASS-METHODS raise IMPORTING !iv_text TYPE clike !ix_previous TYPE REF TO cx_root OPTIONAL RAISING zcx_abapgit_exception .
    CLASS-METHODS raise_t100 IMPORTING !iv_msgid TYPE symsgid DEFAULT sy-msgid !iv_msgno TYPE symsgno DEFAULT sy-msgno RAISING zcx_abapgit_exception .
    CLASS-METHODS raise_with_text IMPORTING !ix_previous TYPE REF TO cx_root RAISING zcx_abapgit_exception .
  PRIVATE SECTION.
    METHODS save_callstack .
ENDCLASS.
CLASS zcx_abapgit_exception IMPLEMENTATION.
  METHOD raise. cl_message_helper=>set_msg_vars_for_clike( iv_text ). ENDMETHOD.
  METHOD save_callstack. ENDMETHOD.
ENDCLASS.`,

	"CLAS:ZCL_ABAPGIT_FACTORY": `CLASS zcl_abapgit_factory DEFINITION PUBLIC CREATE PRIVATE.
  PUBLIC SECTION.
    CLASS-METHODS get_tadir RETURNING VALUE(ri_tadir) TYPE REF TO zif_abapgit_tadir .
    CLASS-METHODS get_sap_package IMPORTING !iv_package TYPE devclass RETURNING VALUE(ri_sap_package) TYPE REF TO zif_abapgit_sap_package .
    CLASS-METHODS get_cts_api RETURNING VALUE(ri_cts_api) TYPE REF TO zif_abapgit_cts_api .
    CLASS-METHODS get_environment RETURNING VALUE(ri_environment) TYPE REF TO zif_abapgit_environment .
  PRIVATE SECTION.
    CLASS-DATA gi_tadir TYPE REF TO zif_abapgit_tadir .
ENDCLASS.
CLASS zcl_abapgit_factory IMPLEMENTATION.
  METHOD get_tadir. ENDMETHOD.
  METHOD get_sap_package. ENDMETHOD.
  METHOD get_cts_api. ENDMETHOD.
  METHOD get_environment. ENDMETHOD.
ENDCLASS.`,

	"CLAS:ZCL_ABAPGIT_TIMER": `CLASS zcl_abapgit_timer DEFINITION PUBLIC FINAL CREATE PRIVATE.
  PUBLIC SECTION.
    CLASS-METHODS create IMPORTING !iv_text TYPE string OPTIONAL !iv_count TYPE i OPTIONAL RETURNING VALUE(ro_timer) TYPE REF TO zcl_abapgit_timer.
    METHODS start RETURNING VALUE(ro_timer) TYPE REF TO zcl_abapgit_timer.
    METHODS end IMPORTING !iv_output_as_status_message TYPE abap_bool DEFAULT abap_false RETURNING VALUE(rv_result) TYPE string.
  PRIVATE SECTION.
    DATA mv_timer TYPE timestampl.
ENDCLASS.
CLASS zcl_abapgit_timer IMPLEMENTATION.
  METHOD create. ENDMETHOD.
  METHOD start. ENDMETHOD.
  METHOD end. ENDMETHOD.
ENDCLASS.`,

	"CLAS:ZCL_ABAPGIT_OBJECTS": `CLASS zcl_abapgit_objects DEFINITION PUBLIC CREATE PUBLIC.
  PUBLIC SECTION.
    CLASS-METHODS serialize IMPORTING !is_item TYPE zif_abapgit_definitions=>ty_item RETURNING VALUE(rs_files_and_item) TYPE zif_abapgit_objects=>ty_serialization RAISING zcx_abapgit_exception .
    CLASS-METHODS exists IMPORTING !is_item TYPE zif_abapgit_definitions=>ty_item RETURNING VALUE(rv_bool) TYPE abap_bool .
    CLASS-METHODS is_supported IMPORTING !is_item TYPE zif_abapgit_definitions=>ty_item RETURNING VALUE(rv_bool) TYPE abap_bool .
  PRIVATE SECTION.
    CLASS-METHODS class_name IMPORTING !is_item TYPE zif_abapgit_definitions=>ty_item RETURNING VALUE(rv_class_name) TYPE string .
ENDCLASS.
CLASS zcl_abapgit_objects IMPLEMENTATION.
  METHOD serialize. ENDMETHOD.
  METHOD exists. ENDMETHOD.
  METHOD is_supported. ENDMETHOD.
  METHOD class_name. ENDMETHOD.
ENDCLASS.`,

	"CLAS:ZCL_ABAPGIT_PERSIST_FACTORY": `CLASS zcl_abapgit_persist_factory DEFINITION PUBLIC CREATE PRIVATE.
  PUBLIC SECTION.
    CLASS-METHODS get_repo RETURNING VALUE(ri_repo) TYPE REF TO zif_abapgit_persist_repo .
    CLASS-METHODS get_settings RETURNING VALUE(ri_settings) TYPE REF TO zif_abapgit_persist_settings .
    CLASS-METHODS get_background RETURNING VALUE(ri_background) TYPE REF TO zif_abapgit_persist_background.
    CLASS-METHODS get_user IMPORTING !iv_user TYPE sy-uname DEFAULT sy-uname RETURNING VALUE(ri_user) TYPE REF TO zif_abapgit_persist_user.
  PRIVATE SECTION.
    CLASS-DATA gi_settings TYPE REF TO zif_abapgit_persist_settings .
ENDCLASS.
CLASS zcl_abapgit_persist_factory IMPLEMENTATION.
  METHOD get_repo. ENDMETHOD.
  METHOD get_settings. ENDMETHOD.
  METHOD get_background. ENDMETHOD.
  METHOD get_user. ENDMETHOD.
ENDCLASS.`,

	"CLAS:ZCL_ABAPGIT_SETTINGS": `CLASS zcl_abapgit_settings DEFINITION PUBLIC CREATE PUBLIC.
  PUBLIC SECTION.
    METHODS get_parallel_proc_disabled RETURNING VALUE(rv_disabled) TYPE abap_bool.
  PRIVATE SECTION.
    DATA mv_parallel_proc_disabled TYPE abap_bool.
ENDCLASS.
CLASS zcl_abapgit_settings IMPLEMENTATION.
  METHOD get_parallel_proc_disabled. rv_disabled = mv_parallel_proc_disabled. ENDMETHOD.
ENDCLASS.`,
}
