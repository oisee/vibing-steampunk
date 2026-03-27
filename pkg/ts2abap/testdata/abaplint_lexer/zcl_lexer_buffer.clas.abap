CLASS zcl_lexer_buffer DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    METHODS add IMPORTING iv_s TYPE string RETURNING VALUE(rv_result) TYPE string.
    METHODS get RETURNING VALUE(rv_result) TYPE string.
    METHODS clear.
    METHODS count_is_even IMPORTING iv_char TYPE string RETURNING VALUE(rv_result) TYPE abap_bool.
  PRIVATE SECTION.
    DATA mv_buf TYPE string.
ENDCLASS.

CLASS zcl_lexer_buffer IMPLEMENTATION.
  METHOD add. mv_buf = mv_buf && iv_s. rv_result = mv_buf. ENDMETHOD.
  METHOD get. rv_result = mv_buf. ENDMETHOD.
  METHOD clear. CLEAR mv_buf. ENDMETHOD.
  METHOD count_is_even.
    DATA(lv_count) = 0.
    DO strlen( mv_buf ) TIMES.
      DATA(lv_i) = sy-index - 1.
      IF mv_buf+lv_i(1) = iv_char. lv_count = lv_count + 1. ENDIF.
    ENDDO.
    rv_result = xsdbool( lv_count MOD 2 = 0 ).
  ENDMETHOD.
ENDCLASS.
