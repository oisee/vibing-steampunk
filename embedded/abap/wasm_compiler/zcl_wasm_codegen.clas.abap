CLASS zcl_wasm_codegen DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    METHODS compile
      IMPORTING io_module  TYPE REF TO zcl_wasm_module
                iv_name    TYPE string DEFAULT 'ZWASM_OUT'
      RETURNING VALUE(rv)  TYPE string.
  PRIVATE SECTION.
    DATA mo_mod TYPE REF TO zcl_wasm_module.
    DATA mv_out TYPE string.
    DATA mv_indent TYPE i.
    DATA mv_stack_depth TYPE i.
    DATA mt_block_kinds TYPE STANDARD TABLE OF i WITH DEFAULT KEY.
    METHODS line IMPORTING iv TYPE string.
    METHODS push RETURNING VALUE(rv) TYPE string.
    METHODS pop RETURNING VALUE(rv) TYPE string.
    METHODS peek RETURNING VALUE(rv) TYPE string.
    METHODS emit_function IMPORTING is_func TYPE zcl_wasm_module=>ty_function.
    METHODS emit_instructions IMPORTING it_code TYPE zcl_wasm_module=>ty_instructions.
    METHODS emit_call IMPORTING iv_func_idx TYPE i.
    METHODS func_name IMPORTING iv_idx TYPE i RETURNING VALUE(rv) TYPE string.
    METHODS valtype_abap IMPORTING iv_type TYPE i RETURNING VALUE(rv) TYPE string.
ENDCLASS.
