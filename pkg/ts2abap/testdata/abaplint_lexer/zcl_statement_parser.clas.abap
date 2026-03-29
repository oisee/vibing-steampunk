CLASS zcl_statement_parser DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    TYPES: BEGIN OF ty_statement,
             type   TYPE string,  " UNKNOWN, COMMENT, EMPTY
             tokens TYPE STANDARD TABLE OF REF TO zcl_abstract_token WITH DEFAULT KEY,
             colon  TYPE REF TO zcl_abstract_token,
           END OF ty_statement,
           ty_statements TYPE STANDARD TABLE OF ty_statement WITH DEFAULT KEY.

    " Split lexer tokens into statements (by . and , for chaining)
    METHODS parse
      IMPORTING it_tokens TYPE zcl_lexer=>ty_result-tokens
      RETURNING VALUE(rt) TYPE ty_statements.

  PRIVATE SECTION.
    DATA mt_result TYPE ty_statements.
    METHODS add_statement
      IMPORTING it_pre   TYPE zcl_lexer=>ty_result-tokens
                it_add   TYPE zcl_lexer=>ty_result-tokens
                io_colon TYPE REF TO zcl_abstract_token OPTIONAL.
ENDCLASS.

CLASS zcl_statement_parser IMPLEMENTATION.
  METHOD parse.
    DATA lt_add TYPE zcl_lexer=>ty_result-tokens.
    DATA lt_pre TYPE zcl_lexer=>ty_result-tokens.
    DATA lo_colon TYPE REF TO zcl_abstract_token.
    CLEAR mt_result.

    LOOP AT it_tokens INTO DATA(lo_token).
      " Comments become their own statement
      IF lo_token IS INSTANCE OF zcl_tok_comment.
        DATA ls_comment TYPE ty_statement.
        ls_comment-type = 'COMMENT'.
        APPEND lo_token TO ls_comment-tokens.
        APPEND ls_comment TO mt_result.
        CLEAR ls_comment.
        CONTINUE.
      ENDIF.

      APPEND lo_token TO lt_add.

      DATA(lv_str) = lo_token->get_str( ).
      IF strlen( lv_str ) = 1.
        IF lv_str = '.'.
          " End of statement
          add_statement( it_pre = lt_pre it_add = lt_add io_colon = lo_colon ).
          CLEAR: lt_add, lt_pre, lo_colon.
        ELSEIF lv_str = ',' AND lines( lt_pre ) > 0.
          " Chained statement separator
          add_statement( it_pre = lt_pre it_add = lt_add io_colon = lo_colon ).
          CLEAR lt_add.
        ELSEIF lv_str = ':' AND lo_colon IS NOT BOUND.
          " First colon — start of chain
          lo_colon = lo_token.
          " Remove colon from add, move to pre
          DELETE lt_add INDEX lines( lt_add ).
          APPEND LINES OF lt_add TO lt_pre.
          CLEAR lt_add.
        ELSEIF lv_str = ':'.
          " Additional colons — ignore
          DELETE lt_add INDEX lines( lt_add ).
        ENDIF.
      ENDIF.
    ENDLOOP.

    " Remaining tokens (unterminated statement)
    IF lines( lt_add ) > 0.
      add_statement( it_pre = lt_pre it_add = lt_add io_colon = lo_colon ).
    ENDIF.

    rt = mt_result.
  ENDMETHOD.

  METHOD add_statement.
    DATA ls_stmt TYPE ty_statement.

    " Combine pre + add tokens
    APPEND LINES OF it_pre TO ls_stmt-tokens.
    APPEND LINES OF it_add TO ls_stmt-tokens.
    ls_stmt-colon = io_colon.

    " Determine type
    IF lines( ls_stmt-tokens ) = 1.
      DATA(lo_last) = ls_stmt-tokens[ lines( ls_stmt-tokens ) ].
      IF lo_last IS INSTANCE OF zcl_tok_punctuation.
        ls_stmt-type = 'EMPTY'.
      ELSE.
        ls_stmt-type = 'UNKNOWN'.
      ENDIF.
    ELSE.
      ls_stmt-type = 'UNKNOWN'.
    ENDIF.

    APPEND ls_stmt TO mt_result.
  ENDMETHOD.
ENDCLASS.
