CLASS zcl_position DEFINITION PUBLIC CREATE PUBLIC.
  PUBLIC SECTION.
    METHODS constructor IMPORTING iv_row TYPE i iv_col TYPE i.
    METHODS get_col RETURNING VALUE(rv_result) TYPE i.
    METHODS get_row RETURNING VALUE(rv_result) TYPE i.
    METHODS is_after IMPORTING io_p TYPE REF TO zcl_position RETURNING VALUE(rv_result) TYPE abap_bool.
    METHODS equals IMPORTING io_p TYPE REF TO zcl_position RETURNING VALUE(rv_result) TYPE abap_bool.
    METHODS is_before IMPORTING io_p TYPE REF TO zcl_position RETURNING VALUE(rv_result) TYPE abap_bool.
  PRIVATE SECTION.
    DATA mv_row TYPE i.
    DATA mv_col TYPE i.
ENDCLASS.

CLASS zcl_position IMPLEMENTATION.
  METHOD constructor.
    mv_row = iv_row. mv_col = iv_col.
  ENDMETHOD.
  METHOD get_col. rv_result = mv_col. ENDMETHOD.
  METHOD get_row. rv_result = mv_row. ENDMETHOD.
  METHOD is_after. rv_result = xsdbool( mv_row > io_p->get_row( ) OR ( mv_row = io_p->get_row( ) AND mv_col >= io_p->get_col( ) ) ). ENDMETHOD.
  METHOD equals. rv_result = xsdbool( mv_row = io_p->get_row( ) AND mv_col = io_p->get_col( ) ). ENDMETHOD.
  METHOD is_before. rv_result = xsdbool( mv_row < io_p->get_row( ) OR ( mv_row = io_p->get_row( ) AND mv_col < io_p->get_col( ) ) ). ENDMETHOD.
ENDCLASS.
