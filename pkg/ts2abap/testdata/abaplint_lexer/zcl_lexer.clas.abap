CLASS zcl_lexer DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    TYPES: BEGIN OF ty_result,
             tokens TYPE STANDARD TABLE OF REF TO zcl_abstract_token WITH DEFAULT KEY,
           END OF ty_result.
    METHODS run IMPORTING iv_raw TYPE string RETURNING VALUE(rs_result) TYPE ty_result.
  PRIVATE SECTION.
    CONSTANTS c_normal   TYPE i VALUE 1.
    CONSTANTS c_ping     TYPE i VALUE 2.
    CONSTANTS c_str      TYPE i VALUE 3.
    CONSTANTS c_template TYPE i VALUE 4.
    CONSTANTS c_comment  TYPE i VALUE 5.
    CONSTANTS c_pragma   TYPE i VALUE 6.
    DATA mt_tokens TYPE STANDARD TABLE OF REF TO zcl_abstract_token WITH DEFAULT KEY.
    DATA mv_m TYPE i.
    DATA mo_stream TYPE REF TO zcl_lexer_stream.
    DATA mo_buffer TYPE REF TO zcl_lexer_buffer.
    METHODS add.
    METHODS process IMPORTING iv_raw TYPE string.
ENDCLASS.

CLASS zcl_lexer IMPLEMENTATION.
  METHOD run.
    CLEAR mt_tokens. mv_m = c_normal.
    DATA(lv_clean) = replace( val = iv_raw sub = cl_abap_char_utilities=>cr_lf with = cl_abap_char_utilities=>newline occ = 0 ).
    process( lv_clean ).
    rs_result-tokens = mt_tokens.
  ENDMETHOD.

  METHOD add.
    DATA(lv_s) = condense( replace( val = replace( val = mo_buffer->get( ) sub = cl_abap_char_utilities=>newline with = `` occ = 0 ) sub = cl_abap_char_utilities=>horizontal_tab with = `` occ = 0 ) ).
    IF strlen( lv_s ) > 0.
      DATA(lv_col) = mo_stream->get_col( ).
      DATA(lv_row) = mo_stream->get_row( ).
      DATA lv_white_before TYPE abap_bool.
      IF mo_stream->get_offset( ) - strlen( lv_s ) >= 0.
        DATA(lv_off) = mo_stream->get_offset( ) - strlen( lv_s ).
        DATA(lv_prev) = substring( val = mo_stream->get_raw( ) off = lv_off len = 1 ).
        IF lv_prev = ` ` OR lv_prev = cl_abap_char_utilities=>newline OR lv_prev = cl_abap_char_utilities=>horizontal_tab OR lv_prev = ':'.
          lv_white_before = abap_true.
        ENDIF.
      ENDIF.
      DATA lv_white_after TYPE abap_bool.
      DATA(lv_next) = mo_stream->next_char( ).
      IF lv_next = ` ` OR lv_next = cl_abap_char_utilities=>newline OR lv_next = cl_abap_char_utilities=>horizontal_tab
          OR lv_next = ':' OR lv_next = ',' OR lv_next = '.' OR lv_next = '' OR lv_next = '"'.
        lv_white_after = abap_true.
      ENDIF.
      DATA(lo_pos) = NEW zcl_position( iv_row = lv_row iv_col = lv_col - strlen( lv_s ) ).
      DATA lo_tok TYPE REF TO zcl_abstract_token.
      IF mv_m = c_comment.
        lo_tok = NEW zcl_tok_comment( io_start = lo_pos iv_str = lv_s ).
      ELSEIF mv_m = c_ping OR mv_m = c_str.
        lo_tok = NEW zcl_tok_string_token( io_start = lo_pos iv_str = lv_s ).
      ELSEIF mv_m = c_template.
        DATA(lv_first) = lv_s(1). DATA(lv_slen) = strlen( lv_s ) - 1. DATA(lv_last) = lv_s+lv_slen(1).
        IF lv_first = '|' AND lv_last = '|'.
          lo_tok = NEW zcl_tok_string_template( io_start = lo_pos iv_str = lv_s ).
        ELSEIF lv_first = '|' AND lv_last = '{' AND lv_white_after = abap_true.
          lo_tok = NEW zcl_tok_string_template_begin( io_start = lo_pos iv_str = lv_s ).
        ELSEIF lv_first = '}' AND lv_last = '|' AND lv_white_before = abap_true.
          lo_tok = NEW zcl_tok_string_template_end( io_start = lo_pos iv_str = lv_s ).
        ELSEIF lv_first = '}' AND lv_last = '{' AND lv_white_after = abap_true AND lv_white_before = abap_true.
          lo_tok = NEW zcl_tok_string_template_middle( io_start = lo_pos iv_str = lv_s ).
        ELSE.
          lo_tok = NEW zcl_tok_identifier( io_start = lo_pos iv_str = lv_s ).
        ENDIF.
      ELSEIF strlen( lv_s ) > 2 AND lv_s(2) = '##'.
        lo_tok = NEW zcl_tok_pragma( io_start = lo_pos iv_str = lv_s ).
      ELSEIF strlen( lv_s ) = 1.
        CASE lv_s.
          WHEN '.' OR ','. lo_tok = NEW zcl_tok_punctuation( io_start = lo_pos iv_str = lv_s ).
          WHEN '['.
            IF lv_white_before = abap_true AND lv_white_after = abap_true. lo_tok = NEW zcl_tok_wbracket_leftw( io_start = lo_pos iv_str = lv_s ).
            ELSEIF lv_white_before = abap_true. lo_tok = NEW zcl_tok_wbracket_left( io_start = lo_pos iv_str = lv_s ).
            ELSEIF lv_white_after = abap_true. lo_tok = NEW zcl_tok_bracket_leftw( io_start = lo_pos iv_str = lv_s ).
            ELSE. lo_tok = NEW zcl_tok_bracket_left( io_start = lo_pos iv_str = lv_s ). ENDIF.
          WHEN '('.
            IF lv_white_before = abap_true AND lv_white_after = abap_true. lo_tok = NEW zcl_tok_wparen_leftw( io_start = lo_pos iv_str = lv_s ).
            ELSEIF lv_white_before = abap_true. lo_tok = NEW zcl_tok_wparen_left( io_start = lo_pos iv_str = lv_s ).
            ELSEIF lv_white_after = abap_true. lo_tok = NEW zcl_tok_paren_leftw( io_start = lo_pos iv_str = lv_s ).
            ELSE. lo_tok = NEW zcl_tok_paren_left( io_start = lo_pos iv_str = lv_s ). ENDIF.
          WHEN ']'.
            IF lv_white_before = abap_true AND lv_white_after = abap_true. lo_tok = NEW zcl_tok_wbracket_rightw( io_start = lo_pos iv_str = lv_s ).
            ELSEIF lv_white_before = abap_true. lo_tok = NEW zcl_tok_wbracket_right( io_start = lo_pos iv_str = lv_s ).
            ELSEIF lv_white_after = abap_true. lo_tok = NEW zcl_tok_bracket_rightw( io_start = lo_pos iv_str = lv_s ).
            ELSE. lo_tok = NEW zcl_tok_bracket_right( io_start = lo_pos iv_str = lv_s ). ENDIF.
          WHEN ')'.
            IF lv_white_before = abap_true AND lv_white_after = abap_true. lo_tok = NEW zcl_tok_wparen_rightw( io_start = lo_pos iv_str = lv_s ).
            ELSEIF lv_white_before = abap_true. lo_tok = NEW zcl_tok_wparen_right( io_start = lo_pos iv_str = lv_s ).
            ELSEIF lv_white_after = abap_true. lo_tok = NEW zcl_tok_paren_rightw( io_start = lo_pos iv_str = lv_s ).
            ELSE. lo_tok = NEW zcl_tok_paren_right( io_start = lo_pos iv_str = lv_s ). ENDIF.
          WHEN '-'.
            IF lv_white_before = abap_true AND lv_white_after = abap_true. lo_tok = NEW zcl_tok_wdashw( io_start = lo_pos iv_str = lv_s ).
            ELSEIF lv_white_before = abap_true. lo_tok = NEW zcl_tok_wdash( io_start = lo_pos iv_str = lv_s ).
            ELSEIF lv_white_after = abap_true. lo_tok = NEW zcl_tok_dashw( io_start = lo_pos iv_str = lv_s ).
            ELSE. lo_tok = NEW zcl_tok_dash( io_start = lo_pos iv_str = lv_s ). ENDIF.
          WHEN '+'.
            IF lv_white_before = abap_true AND lv_white_after = abap_true. lo_tok = NEW zcl_tok_wplusw( io_start = lo_pos iv_str = lv_s ).
            ELSEIF lv_white_before = abap_true. lo_tok = NEW zcl_tok_wplus( io_start = lo_pos iv_str = lv_s ).
            ELSEIF lv_white_after = abap_true. lo_tok = NEW zcl_tok_plusw( io_start = lo_pos iv_str = lv_s ).
            ELSE. lo_tok = NEW zcl_tok_plus( io_start = lo_pos iv_str = lv_s ). ENDIF.
          WHEN '@'.
            IF lv_white_before = abap_true AND lv_white_after = abap_true. lo_tok = NEW zcl_tok_watw( io_start = lo_pos iv_str = lv_s ).
            ELSEIF lv_white_before = abap_true. lo_tok = NEW zcl_tok_wat( io_start = lo_pos iv_str = lv_s ).
            ELSEIF lv_white_after = abap_true. lo_tok = NEW zcl_tok_atw( io_start = lo_pos iv_str = lv_s ).
            ELSE. lo_tok = NEW zcl_tok_at( io_start = lo_pos iv_str = lv_s ). ENDIF.
        ENDCASE.
      ELSEIF strlen( lv_s ) = 2.
        IF lv_s = '->'.
          IF lv_white_before = abap_true AND lv_white_after = abap_true. lo_tok = NEW zcl_tok_winstance_arroww( io_start = lo_pos iv_str = lv_s ).
          ELSEIF lv_white_before = abap_true. lo_tok = NEW zcl_tok_winstance_arrow( io_start = lo_pos iv_str = lv_s ).
          ELSEIF lv_white_after = abap_true. lo_tok = NEW zcl_tok_instance_arroww( io_start = lo_pos iv_str = lv_s ).
          ELSE. lo_tok = NEW zcl_tok_instance_arrow( io_start = lo_pos iv_str = lv_s ). ENDIF.
        ELSEIF lv_s = '=>'.
          IF lv_white_before = abap_true AND lv_white_after = abap_true. lo_tok = NEW zcl_tok_wstatic_arroww( io_start = lo_pos iv_str = lv_s ).
          ELSEIF lv_white_before = abap_true. lo_tok = NEW zcl_tok_wstatic_arrow( io_start = lo_pos iv_str = lv_s ).
          ELSEIF lv_white_after = abap_true. lo_tok = NEW zcl_tok_static_arroww( io_start = lo_pos iv_str = lv_s ).
          ELSE. lo_tok = NEW zcl_tok_static_arrow( io_start = lo_pos iv_str = lv_s ). ENDIF.
        ENDIF.
      ENDIF.
      IF lo_tok IS NOT BOUND. lo_tok = NEW zcl_tok_identifier( io_start = lo_pos iv_str = lv_s ). ENDIF.
      APPEND lo_tok TO mt_tokens.
    ENDIF.
    mo_buffer->clear( ).
  ENDMETHOD.

  METHOD process.
    mo_stream = NEW zcl_lexer_stream( iv_raw ).
    mo_buffer = NEW zcl_lexer_buffer( ).
    DO.
      DATA(lv_current) = mo_stream->current_char( ).
      DATA(lv_buf) = mo_buffer->add( lv_current ).
      DATA(lv_ahead) = mo_stream->next_char( ).
      DATA(lv_aahead) = mo_stream->next_next_char( ).
      IF mv_m = c_normal.
        IF lv_ahead = ` ` OR lv_ahead = ':' OR lv_ahead = '.' OR lv_ahead = ',' OR lv_ahead = '-' OR lv_ahead = '+'
            OR lv_ahead = '(' OR lv_ahead = ')' OR lv_ahead = '[' OR lv_ahead = ']'
            OR lv_ahead = cl_abap_char_utilities=>horizontal_tab OR lv_ahead = cl_abap_char_utilities=>newline.
          add( ).
        ELSEIF lv_ahead = ''''.
          add( ). mv_m = c_str.
        ELSEIF lv_ahead = '|' OR lv_ahead = '}'.
          add( ). mv_m = c_template.
        ELSEIF lv_ahead = '`'.
          add( ). mv_m = c_ping.
        ELSEIF lv_aahead = '##'.
          add( ). mv_m = c_pragma.
        ELSEIF lv_ahead = '"' OR ( lv_ahead = '*' AND lv_current = cl_abap_char_utilities=>newline ).
          add( ). mv_m = c_comment.
        ELSEIF lv_ahead = '@' AND strlen( condense( lv_buf ) ) = 0.
          add( ).
        ELSEIF lv_aahead = '->' OR lv_aahead = '=>'.
          add( ).
        ELSEIF lv_current = '>' AND lv_ahead <> ` ` AND ( mo_stream->prev_char( ) = '-' OR mo_stream->prev_char( ) = '=' ).
          add( ).
        ELSEIF strlen( lv_buf ) = 1 AND ( lv_buf = '.' OR lv_buf = ',' OR lv_buf = ':' OR lv_buf = '(' OR lv_buf = ')' OR lv_buf = '[' OR lv_buf = ']' OR lv_buf = '+' OR lv_buf = '@' OR ( lv_buf = '-' AND lv_ahead <> '>' ) ).
          add( ).
        ENDIF.
      ELSEIF mv_m = c_pragma AND ( lv_ahead = ',' OR lv_ahead = ':' OR lv_ahead = '.' OR lv_ahead = ` ` OR lv_ahead = cl_abap_char_utilities=>newline ).
        add( ). mv_m = c_normal.
      ELSEIF mv_m = c_ping AND strlen( lv_buf ) > 1 AND lv_current = '`' AND lv_aahead <> '``' AND lv_ahead <> '`' AND mo_buffer->count_is_even( '`' ).
        add( ). IF lv_ahead = '"'. mv_m = c_comment. ELSE. mv_m = c_normal. ENDIF.
      ELSEIF mv_m = c_template AND strlen( lv_buf ) > 1 AND ( lv_current = '|' OR lv_current = '{' ) AND ( mo_stream->prev_char( ) <> '\' OR mo_stream->prev_prev_char( ) = '\\' ).
        add( ). mv_m = c_normal.
      ELSEIF mv_m = c_template AND lv_ahead = '}' AND lv_current <> '\'.
        add( ).
      ELSEIF mv_m = c_str AND lv_current = '''' AND strlen( lv_buf ) > 1 AND lv_aahead <> '''''' AND lv_ahead <> '''' AND mo_buffer->count_is_even( '''' ).
        add( ). IF lv_ahead = '"'. mv_m = c_comment. ELSE. mv_m = c_normal. ENDIF.
      ELSEIF lv_ahead = cl_abap_char_utilities=>newline AND mv_m <> c_template.
        add( ). mv_m = c_normal.
      ELSEIF mv_m = c_template AND lv_current = cl_abap_char_utilities=>newline.
        add( ).
      ENDIF.
      IF mo_stream->advance( ) = abap_false. EXIT. ENDIF.
    ENDDO.
    add( ).
  ENDMETHOD.
ENDCLASS.
