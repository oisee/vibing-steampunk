package ctxcomp

import (
	"strings"
	"testing"
)

func TestExtractContract_ClassUtils(t *testing.T) {
	src := readEmbedded(t, "zcl_vsp_utils.clas.abap")
	contract := ExtractContract(src, KindClass)

	if contract == "" {
		t.Fatal("empty contract")
	}

	// Should contain CLASS DEFINITION
	if !strings.Contains(strings.ToUpper(contract), "CLASS ZCL_VSP_UTILS DEFINITION") {
		t.Error("missing CLASS DEFINITION line")
	}

	// Should contain PUBLIC SECTION
	if !strings.Contains(strings.ToUpper(contract), "PUBLIC SECTION") {
		t.Error("missing PUBLIC SECTION")
	}

	// Should contain method signatures
	if !strings.Contains(contract, "escape_json") && !strings.Contains(contract, "ESCAPE_JSON") {
		t.Error("missing escape_json method")
	}
	if !strings.Contains(contract, "extract_param") && !strings.Contains(contract, "EXTRACT_PARAM") {
		t.Error("missing extract_param method")
	}
	if !strings.Contains(contract, "build_error") && !strings.Contains(contract, "BUILD_ERROR") {
		t.Error("missing build_error method")
	}

	// Should NOT contain IMPLEMENTATION
	if strings.Contains(strings.ToUpper(contract), "IMPLEMENTATION") {
		t.Error("contract should not contain IMPLEMENTATION section")
	}

	// Should NOT contain PRIVATE SECTION content
	if strings.Contains(strings.ToUpper(contract), "PRIVATE SECTION") {
		t.Error("contract should not contain PRIVATE SECTION")
	}

	// Contract should be much shorter than source
	ratio := float64(len(contract)) / float64(len(src))
	if ratio > 0.5 {
		t.Errorf("compression ratio too low: %.2f (contract %d bytes, source %d bytes)", ratio, len(contract), len(src))
	}
}

func TestExtractContract_ClassAPCHandler(t *testing.T) {
	src := readEmbedded(t, "zcl_vsp_apc_handler.clas.abap")
	contract := ExtractContract(src, KindClass)

	if contract == "" {
		t.Fatal("empty contract")
	}

	// Should contain INHERITING FROM
	if !strings.Contains(strings.ToUpper(contract), "INHERITING FROM") {
		t.Error("missing INHERITING FROM in definition")
	}

	// Should contain public method redefinitions
	if !strings.Contains(contract, "on_start") && !strings.Contains(contract, "ON_START") {
		t.Error("missing on_start method")
	}

	// Should NOT contain PRIVATE SECTION methods
	if strings.Contains(contract, "parse_message") || strings.Contains(contract, "PARSE_MESSAGE") {
		t.Error("contract should not contain private method parse_message")
	}
	if strings.Contains(contract, "send_response") || strings.Contains(contract, "SEND_RESPONSE") {
		t.Error("contract should not contain private method send_response")
	}

	// Should NOT contain IMPLEMENTATION
	if strings.Contains(strings.ToUpper(contract), "IMPLEMENTATION") {
		t.Error("contract should not contain IMPLEMENTATION section")
	}
}

func TestExtractContract_Interface(t *testing.T) {
	src := readEmbedded(t, "zif_vsp_service.intf.abap")
	contract := ExtractContract(src, KindInterface)

	if contract == "" {
		t.Fatal("empty contract")
	}

	// Interface contract should be (nearly) the full source
	if !strings.Contains(contract, "INTERFACE") {
		t.Error("missing INTERFACE keyword")
	}
	if !strings.Contains(contract, "ENDINTERFACE") {
		t.Error("missing ENDINTERFACE keyword")
	}
	if !strings.Contains(contract, "get_domain") && !strings.Contains(contract, "GET_DOMAIN") {
		t.Error("missing get_domain method")
	}
	if !strings.Contains(contract, "handle_message") && !strings.Contains(contract, "HANDLE_MESSAGE") {
		t.Error("missing handle_message method")
	}
}

func TestExtractContract_ClassReportService(t *testing.T) {
	src := readEmbedded(t, "zcl_vsp_report_service.clas.abap")
	contract := ExtractContract(src, KindClass)

	if contract == "" {
		t.Fatal("empty contract")
	}

	// Should contain the INTERFACES line (in public section)
	if !strings.Contains(contract, "zif_vsp_service") && !strings.Contains(contract, "ZIF_VSP_SERVICE") {
		t.Error("missing INTERFACES zif_vsp_service in public section")
	}

	// Should NOT contain private methods
	if strings.Contains(contract, "handle_run_report") || strings.Contains(contract, "HANDLE_RUN_REPORT") {
		t.Error("contract should not contain private method handle_run_report")
	}

	// Should NOT contain IMPLEMENTATION
	if strings.Contains(strings.ToUpper(contract), "IMPLEMENTATION") {
		t.Error("contract should not contain IMPLEMENTATION")
	}
}

func TestExtractContract_FunctionModule(t *testing.T) {
	src := `FUNCTION ztest_my_func.
*"----------------------------------------------------------------------
*"*"Local Interface:
*"  IMPORTING
*"     VALUE(IV_NAME) TYPE  STRING
*"     VALUE(IV_COUNT) TYPE  I
*"  EXPORTING
*"     VALUE(EV_RESULT) TYPE  STRING
*"  EXCEPTIONS
*"      NOT_FOUND
*"----------------------------------------------------------------------
  DATA lv_temp TYPE string.
  lv_temp = iv_name.
  ev_result = lv_temp.
ENDFUNCTION.`

	contract := ExtractContract(src, KindFunction)

	if contract == "" {
		t.Fatal("empty contract")
	}

	// Should contain FUNCTION line
	if !strings.Contains(contract, "FUNCTION") {
		t.Error("missing FUNCTION line")
	}

	// Should contain parameter comments
	if !strings.Contains(contract, "IV_NAME") {
		t.Error("missing IV_NAME parameter")
	}
	if !strings.Contains(contract, "EV_RESULT") {
		t.Error("missing EV_RESULT parameter")
	}

	// Should NOT contain implementation code
	if strings.Contains(contract, "lv_temp") {
		t.Error("contract should not contain implementation code")
	}
}

func TestExtractContract_InlineClass(t *testing.T) {
	src := `CLASS zcl_example DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    METHODS do_work IMPORTING iv_input TYPE string RETURNING VALUE(rv_output) TYPE string.
    METHODS get_status RETURNING VALUE(rv_status) TYPE i.
  PROTECTED SECTION.
    DATA mv_internal TYPE string.
  PRIVATE SECTION.
    METHODS helper RETURNING VALUE(rv_val) TYPE string.
    DATA mv_secret TYPE string.
ENDCLASS.

CLASS zcl_example IMPLEMENTATION.
  METHOD do_work.
    rv_output = iv_input.
  ENDMETHOD.
  METHOD get_status.
    rv_status = 0.
  ENDMETHOD.
  METHOD helper.
    rv_val = mv_secret.
  ENDMETHOD.
ENDCLASS.`

	contract := ExtractContract(src, KindClass)

	// Should include definition + public section
	if !strings.Contains(contract, "do_work") {
		t.Error("missing public method do_work")
	}
	if !strings.Contains(contract, "get_status") {
		t.Error("missing public method get_status")
	}

	// Should NOT include protected/private
	if strings.Contains(contract, "mv_internal") {
		t.Error("should not contain protected data")
	}
	if strings.Contains(contract, "helper") {
		t.Error("should not contain private method")
	}
	if strings.Contains(contract, "mv_secret") {
		t.Error("should not contain private data")
	}

	// Should NOT include IMPLEMENTATION
	if strings.Contains(strings.ToUpper(contract), "IMPLEMENTATION") {
		t.Error("should not contain IMPLEMENTATION")
	}

	// Should end with ENDCLASS.
	if !strings.Contains(contract, "ENDCLASS.") {
		t.Error("should end with ENDCLASS.")
	}
}
