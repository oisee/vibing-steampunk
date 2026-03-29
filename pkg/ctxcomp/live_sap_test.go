package ctxcomp

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// TestCompressor_RealSAPDeps tests with real source fetched from SAP A4H system.
// This validates the full pipeline: extract deps → resolve contracts → format prologue.
func TestCompressor_RealSAPDeps(t *testing.T) {
	// Full source of ZCL_ABAPGIT_AJSON fetched from SAP (700+ lines, truncated to definition + partial impl)
	// Using a representative subset that exercises all patterns
	ajsonSource := `CLASS zcl_abapgit_ajson DEFINITION
  PUBLIC
  CREATE PUBLIC.

  PUBLIC SECTION.

    INTERFACES zif_abapgit_ajson.

    ALIASES:
      is_empty FOR zif_abapgit_ajson~is_empty,
      exists FOR zif_abapgit_ajson~exists,
      get FOR zif_abapgit_ajson~get,
      set FOR zif_abapgit_ajson~set,
      stringify FOR zif_abapgit_ajson~stringify,
      clone FOR zif_abapgit_ajson~clone,
      filter FOR zif_abapgit_ajson~filter,
      map FOR zif_abapgit_ajson~map,
      mt_json_tree FOR zif_abapgit_ajson~mt_json_tree,
      freeze FOR zif_abapgit_ajson~freeze.

    CLASS-METHODS parse
      IMPORTING
        !iv_json            TYPE any
        !ii_custom_mapping  TYPE REF TO zif_abapgit_ajson_mapping OPTIONAL
      RETURNING
        VALUE(ro_instance)  TYPE REF TO zcl_abapgit_ajson
      RAISING
        zcx_abapgit_ajson_error.

    CLASS-METHODS create_from
      IMPORTING
        !ii_source_json    TYPE REF TO zif_abapgit_ajson
        !ii_filter         TYPE REF TO zif_abapgit_ajson_filter OPTIONAL
        !ii_mapper         TYPE REF TO zif_abapgit_ajson_mapping OPTIONAL
      RETURNING
        VALUE(ro_instance) TYPE REF TO zcl_abapgit_ajson
      RAISING
        zcx_abapgit_ajson_error.

    CLASS-METHODS new
      RETURNING
        VALUE(ro_instance) TYPE REF TO zcl_abapgit_ajson.

    CLASS-METHODS normalize_path
      IMPORTING iv_path TYPE string
      RETURNING VALUE(rv_path) TYPE string.

  PROTECTED SECTION.
  PRIVATE SECTION.
    CLASS-DATA go_float_regex TYPE REF TO cl_abap_regex.
    DATA ms_opts TYPE zif_abapgit_ajson=>ty_opts.
    DATA mi_custom_mapping TYPE REF TO zif_abapgit_ajson_mapping.
    METHODS get_item IMPORTING iv_path TYPE string RETURNING VALUE(rv_item) TYPE REF TO zif_abapgit_ajson_types=>ty_node.
    METHODS prove_path_exists IMPORTING iv_path TYPE string RETURNING VALUE(rr_end_node) TYPE REF TO zif_abapgit_ajson_types=>ty_node RAISING zcx_abapgit_ajson_error.
    METHODS delete_subtree IMPORTING iv_path TYPE string iv_name TYPE string RETURNING VALUE(rs_top_node) TYPE zif_abapgit_ajson_types=>ty_node.
    METHODS read_only_watchdog RAISING zcx_abapgit_ajson_error.
ENDCLASS.

CLASS zcl_abapgit_ajson IMPLEMENTATION.
  METHOD zif_abapgit_ajson~set.
    DATA ls_split_path TYPE zif_abapgit_ajson_types=>ty_path_name.
    read_only_watchdog( ).
    ri_json = me.
    ls_split_path = lcl_utils=>split_path( iv_path ).
  ENDMETHOD.
  METHOD zif_abapgit_ajson~stringify.
    rv_json = lcl_json_serializer=>stringify( it_json_tree = mt_json_tree ).
  ENDMETHOD.
  METHOD zif_abapgit_ajson~clone.
    ri_json = create_from( me ).
  ENDMETHOD.
  METHOD zif_abapgit_ajson~filter.
    ri_json = create_from( ii_source_json = me ii_filter = ii_filter ).
  ENDMETHOD.
ENDCLASS.`

	// Real SAP interfaces as dependency sources
	depSources := map[string]string{
		"INTF:ZIF_ABAPGIT_AJSON": `INTERFACE zif_abapgit_ajson
  PUBLIC.
  CONSTANTS version TYPE string VALUE 'v1.1.13'.
  TYPES:
    BEGIN OF ty_opts,
      read_only                  TYPE abap_bool,
      keep_item_order            TYPE abap_bool,
      format_datetime            TYPE abap_bool,
      to_abap_corresponding_only TYPE abap_bool,
    END OF ty_opts.
  DATA mt_json_tree TYPE zif_abapgit_ajson_types=>ty_nodes_ts READ-ONLY.
  METHODS clone RETURNING VALUE(ri_json) TYPE REF TO zif_abapgit_ajson RAISING zcx_abapgit_ajson_error.
  METHODS filter IMPORTING ii_filter TYPE REF TO zif_abapgit_ajson_filter RETURNING VALUE(ri_json) TYPE REF TO zif_abapgit_ajson RAISING zcx_abapgit_ajson_error.
  METHODS map IMPORTING ii_mapper TYPE REF TO zif_abapgit_ajson_mapping RETURNING VALUE(ri_json) TYPE REF TO zif_abapgit_ajson RAISING zcx_abapgit_ajson_error.
  METHODS freeze.
  METHODS is_empty RETURNING VALUE(rv_yes) TYPE abap_bool.
  METHODS exists IMPORTING iv_path TYPE string RETURNING VALUE(rv_exists) TYPE abap_bool.
  METHODS get IMPORTING iv_path TYPE string RETURNING VALUE(rv_value) TYPE string.
  METHODS set IMPORTING iv_path TYPE string iv_val TYPE any RETURNING VALUE(ri_json) TYPE REF TO zif_abapgit_ajson RAISING zcx_abapgit_ajson_error.
  METHODS stringify IMPORTING iv_indent TYPE i DEFAULT 0 RETURNING VALUE(rv_json) TYPE string RAISING zcx_abapgit_ajson_error.
ENDINTERFACE.`,
		"INTF:ZIF_ABAPGIT_AJSON_TYPES": `INTERFACE zif_abapgit_ajson_types
  PUBLIC.
  TYPES: ty_node_type TYPE string.
  CONSTANTS:
    BEGIN OF node_type,
      boolean TYPE ty_node_type VALUE 'bool',
      string  TYPE ty_node_type VALUE 'str',
      number  TYPE ty_node_type VALUE 'num',
      null    TYPE ty_node_type VALUE 'null',
      array   TYPE ty_node_type VALUE 'array',
      object  TYPE ty_node_type VALUE 'object',
    END OF node_type.
  TYPES:
    BEGIN OF ty_node,
      path TYPE string, name TYPE string, type TYPE ty_node_type,
      value TYPE string, index TYPE i, order TYPE i, children TYPE i,
    END OF ty_node.
  TYPES: ty_nodes_ts TYPE SORTED TABLE OF ty_node WITH UNIQUE KEY path name.
  TYPES: BEGIN OF ty_path_name, path TYPE string, name TYPE string, END OF ty_path_name.
ENDINTERFACE.`,
		"INTF:ZIF_ABAPGIT_AJSON_MAPPING": `INTERFACE zif_abapgit_ajson_mapping
  PUBLIC.
  TYPES: BEGIN OF ty_rename, from TYPE string, to TYPE string, END OF ty_rename.
  METHODS to_abap IMPORTING iv_path TYPE string iv_name TYPE string RETURNING VALUE(rv_result) TYPE string.
  METHODS to_json IMPORTING iv_path TYPE string iv_name TYPE string RETURNING VALUE(rv_result) TYPE string.
  METHODS rename_node IMPORTING is_node TYPE zif_abapgit_ajson_types=>ty_node CHANGING cv_name TYPE zif_abapgit_ajson_types=>ty_node-name.
ENDINTERFACE.`,
		"INTF:ZIF_ABAPGIT_AJSON_FILTER": `INTERFACE zif_abapgit_ajson_filter
  PUBLIC.
  METHODS keep_node IMPORTING is_node TYPE zif_abapgit_ajson_types=>ty_node RETURNING VALUE(rv_keep) TYPE abap_bool RAISING zcx_abapgit_ajson_error.
ENDINTERFACE.`,
		"CLAS:ZCX_ABAPGIT_AJSON_ERROR": `CLASS zcx_abapgit_ajson_error DEFINITION PUBLIC INHERITING FROM cx_static_check.
  PUBLIC SECTION.
    CLASS-METHODS raise IMPORTING iv_msg TYPE string RAISING zcx_abapgit_ajson_error.
    DATA message TYPE string READ-ONLY.
ENDCLASS.
CLASS zcx_abapgit_ajson_error IMPLEMENTATION.
  METHOD raise. ENDMETHOD.
ENDCLASS.`,
	}

	provider := &mockProvider{sources: depSources}
	comp := NewCompressor(provider, 20)
	result, err := comp.Compress(context.Background(), ajsonSource, "ZCL_ABAPGIT_AJSON", "CLAS")
	if err != nil {
		t.Fatalf("Compress error: %v", err)
	}

	// Verify stats
	t.Logf("Dependencies found: %d", result.Stats.DepsFound)
	t.Logf("Resolved: %d, Failed: %d", result.Stats.DepsResolved, result.Stats.DepsFailed)
	t.Logf("Prologue lines: %d", result.Stats.TotalLines)

	if result.Stats.DepsResolved < 4 {
		t.Errorf("expected at least 4 resolved, got %d", result.Stats.DepsResolved)
	}

	// Verify prologue contains key deps
	for _, name := range []string{"ZIF_ABAPGIT_AJSON", "ZIF_ABAPGIT_AJSON_TYPES", "ZIF_ABAPGIT_AJSON_MAPPING"} {
		if !strings.Contains(result.Prologue, name) {
			t.Errorf("prologue missing %s", name)
		}
	}

	// Verify compression ratio
	prologueBytes := len(result.Prologue)
	sourceBytes := len(ajsonSource)
	t.Logf("Source: %d bytes → Prologue: %d bytes (context adds %.0f%% of source size)",
		sourceBytes, prologueBytes, float64(prologueBytes)/float64(sourceBytes)*100)

	// Print the full prologue
	fmt.Println("=== PROLOGUE OUTPUT ===")
	fmt.Println(result.Prologue)
	fmt.Println("=== END ===")

	// Verify no IMPLEMENTATION in prologue
	if strings.Contains(strings.ToUpper(result.Prologue), "IMPLEMENTATION") {
		t.Error("prologue must not contain IMPLEMENTATION sections")
	}
}
