CLASS zcl_abstract_token DEFINITION PUBLIC ABSTRACT CREATE PROTECTED.
  PUBLIC SECTION.
    METHODS constructor IMPORTING io_start TYPE REF TO zcl_position iv_str TYPE string.
    METHODS get_str RETURNING VALUE(rv_result) TYPE string.
    METHODS get_row RETURNING VALUE(rv_result) TYPE i.
    METHODS get_col RETURNING VALUE(rv_result) TYPE i.
    METHODS get_start RETURNING VALUE(rv_result) TYPE REF TO zcl_position.
    METHODS get_end RETURNING VALUE(rv_result) TYPE REF TO zcl_position.
  PRIVATE SECTION.
    DATA mo_start TYPE REF TO zcl_position.
    DATA mv_str TYPE string.
ENDCLASS.

CLASS zcl_abstract_token IMPLEMENTATION.
  METHOD constructor. mo_start = io_start. mv_str = iv_str. ENDMETHOD.
  METHOD get_str. rv_result = mv_str. ENDMETHOD.
  METHOD get_row. rv_result = mo_start->get_row( ). ENDMETHOD.
  METHOD get_col. rv_result = mo_start->get_col( ). ENDMETHOD.
  METHOD get_start. rv_result = mo_start. ENDMETHOD.
  METHOD get_end. rv_result = NEW zcl_position( iv_row = mo_start->get_row( ) iv_col = mo_start->get_col( ) + strlen( mv_str ) ). ENDMETHOD.
ENDCLASS.
