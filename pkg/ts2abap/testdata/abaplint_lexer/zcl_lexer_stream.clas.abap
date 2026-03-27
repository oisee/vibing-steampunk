CLASS zcl_lexer_stream DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    METHODS constructor IMPORTING iv_raw TYPE string.
    METHODS advance RETURNING VALUE(rv_result) TYPE abap_bool.
    METHODS get_col RETURNING VALUE(rv_result) TYPE i.
    METHODS get_row RETURNING VALUE(rv_result) TYPE i.
    METHODS prev_char RETURNING VALUE(rv_result) TYPE string.
    METHODS prev_prev_char RETURNING VALUE(rv_result) TYPE string.
    METHODS current_char RETURNING VALUE(rv_result) TYPE string.
    METHODS next_char RETURNING VALUE(rv_result) TYPE string.
    METHODS next_next_char RETURNING VALUE(rv_result) TYPE string.
    METHODS get_raw RETURNING VALUE(rv_result) TYPE string.
    METHODS get_offset RETURNING VALUE(rv_result) TYPE i.
  PRIVATE SECTION.
    DATA mv_raw TYPE string.
    DATA mv_offset TYPE i.
    DATA mv_row TYPE i.
    DATA mv_col TYPE i.
ENDCLASS.

CLASS zcl_lexer_stream IMPLEMENTATION.
  METHOD constructor.
    mv_raw = iv_raw. mv_offset = -1. mv_row = 0. mv_col = 0.
  ENDMETHOD.

  METHOD advance.
    IF current_char( ) = cl_abap_char_utilities=>newline.
      mv_col = 1. mv_row = mv_row + 1.
    ENDIF.
    IF mv_offset = strlen( mv_raw ).
      mv_col = mv_col - 1. rv_result = abap_false. RETURN.
    ENDIF.
    mv_col = mv_col + 1. mv_offset = mv_offset + 1. rv_result = abap_true.
  ENDMETHOD.

  METHOD get_col. rv_result = mv_col. ENDMETHOD.
  METHOD get_row. rv_result = mv_row. ENDMETHOD.

  METHOD prev_char.
    IF mv_offset - 1 < 0. rv_result = ''. RETURN. ENDIF.
    DATA(lv_o) = mv_offset - 1.
    rv_result = mv_raw+lv_o(1).
  ENDMETHOD.

  METHOD prev_prev_char.
    IF mv_offset - 2 < 0. rv_result = ''. RETURN. ENDIF.
    DATA(lv_o) = mv_offset - 2.
    rv_result = mv_raw+lv_o(2).
  ENDMETHOD.

  METHOD current_char.
    IF mv_offset < 0. rv_result = cl_abap_char_utilities=>newline. RETURN. ENDIF.
    IF mv_offset >= strlen( mv_raw ). rv_result = ''. RETURN. ENDIF.
    rv_result = mv_raw+mv_offset(1).
  ENDMETHOD.

  METHOD next_char.
    IF mv_offset + 2 > strlen( mv_raw ). rv_result = ''. RETURN. ENDIF.
    DATA(lv_o) = mv_offset + 1.
    rv_result = mv_raw+lv_o(1).
  ENDMETHOD.

  METHOD next_next_char.
    IF mv_offset + 3 > strlen( mv_raw ). rv_result = next_char( ). RETURN. ENDIF.
    DATA(lv_o) = mv_offset + 1.
    rv_result = mv_raw+lv_o(2).
  ENDMETHOD.

  METHOD get_raw. rv_result = mv_raw. ENDMETHOD.
  METHOD get_offset. rv_result = mv_offset. ENDMETHOD.
ENDCLASS.
