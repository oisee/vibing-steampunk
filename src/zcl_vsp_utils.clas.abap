"! <p class="shorttext synchronized">VSP Utility Class</p>
"! Centralized utilities for JSON handling, parameter extraction, and response building.
"! Eliminates code duplication across VSP service classes.
CLASS zcl_vsp_utils DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    "! Escape a string for safe JSON embedding
    CLASS-METHODS escape_json
      IMPORTING iv_string         TYPE string
      RETURNING VALUE(rv_escaped) TYPE string.

    "! Extract a string parameter from JSON params
    CLASS-METHODS extract_param
      IMPORTING iv_params       TYPE string
                iv_name         TYPE string
      RETURNING VALUE(rv_value) TYPE string.

    "! Extract an integer parameter from JSON params
    CLASS-METHODS extract_param_int
      IMPORTING iv_params       TYPE string
                iv_name         TYPE string
      RETURNING VALUE(rv_value) TYPE i.

    "! Extract a nested JSON object from params
    CLASS-METHODS extract_param_object
      IMPORTING iv_params      TYPE string
                iv_name        TYPE string
      RETURNING VALUE(rv_json) TYPE string.

    "! Build an error response
    CLASS-METHODS build_error
      IMPORTING iv_id              TYPE string
                iv_code            TYPE string
                iv_message         TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    "! Build a success response
    CLASS-METHODS build_success
      IMPORTING iv_id              TYPE string
                iv_data            TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    "! Wrap content in JSON object braces
    CLASS-METHODS json_obj
      IMPORTING iv_content      TYPE string
      RETURNING VALUE(rv_json) TYPE string.

    "! Wrap content in JSON array brackets
    CLASS-METHODS json_arr
      IMPORTING iv_content      TYPE string
      RETURNING VALUE(rv_json) TYPE string.

    "! Build a JSON key-value pair with string value
    CLASS-METHODS json_str
      IMPORTING iv_key         TYPE string
                iv_value       TYPE string
      RETURNING VALUE(rv_json) TYPE string.

    "! Build a JSON key-value pair with integer value
    CLASS-METHODS json_int
      IMPORTING iv_key         TYPE string
                iv_value       TYPE i
      RETURNING VALUE(rv_json) TYPE string.

    "! Build a JSON key-value pair with boolean value
    CLASS-METHODS json_bool
      IMPORTING iv_key         TYPE string
                iv_value       TYPE abap_bool
      RETURNING VALUE(rv_json) TYPE string.

    "! Join multiple JSON fragments with commas
    CLASS-METHODS json_join
      IMPORTING it_parts       TYPE string_table
      RETURNING VALUE(rv_json) TYPE string.

ENDCLASS.


CLASS zcl_vsp_utils IMPLEMENTATION.

  METHOD escape_json.
    rv_escaped = iv_string.
    REPLACE ALL OCCURRENCES OF '\' IN rv_escaped WITH '\\'.
    REPLACE ALL OCCURRENCES OF '"' IN rv_escaped WITH '\"'.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>cr_lf IN rv_escaped WITH '\n'.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>newline IN rv_escaped WITH '\n'.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>horizontal_tab IN rv_escaped WITH '\t'.
  ENDMETHOD.


  METHOD extract_param.
    DATA lv_name TYPE string.
    lv_name = iv_name.
    CONDENSE lv_name.

    DATA(lv_search) = |"{ lv_name }":|.
    DATA lv_pos TYPE i.
    FIND lv_search IN iv_params MATCH OFFSET lv_pos.
    IF sy-subrc = 0.
      DATA(lv_rest) = iv_params+lv_pos.
      FIND PCRE ':\s*"([^"]*)"' IN lv_rest SUBMATCHES rv_value.
    ENDIF.
  ENDMETHOD.


  METHOD extract_param_int.
    DATA lv_str TYPE string.
    DATA(lv_pattern) = |"{ iv_name }"\\s*:\\s*(\\d+)|.
    FIND PCRE lv_pattern IN iv_params SUBMATCHES lv_str.
    IF sy-subrc = 0.
      rv_value = lv_str.
    ENDIF.
  ENDMETHOD.


  METHOD extract_param_object.
    DATA lv_name TYPE string.
    lv_name = iv_name.
    CONDENSE lv_name.

    DATA(lv_search) = |"{ lv_name }":|.
    DATA lv_pos TYPE i.
    FIND lv_search IN iv_params MATCH OFFSET lv_pos.
    IF sy-subrc <> 0.
      RETURN.
    ENDIF.

    DATA(lv_rest) = iv_params+lv_pos.
    DATA(lv_brace) = find( val = lv_rest sub = '{' ).
    IF lv_brace < 0.
      RETURN.
    ENDIF.

    DATA lv_depth TYPE i.
    DATA lv_i TYPE i.
    lv_i = lv_brace.
    DATA(lv_len) = strlen( lv_rest ).

    WHILE lv_i < lv_len.
      DATA(lv_char) = lv_rest+lv_i(1).
      IF lv_char = '{'.
        lv_depth = lv_depth + 1.
      ELSEIF lv_char = '}'.
        lv_depth = lv_depth - 1.
        IF lv_depth = 0.
          DATA(lv_obj_len) = lv_i - lv_brace + 1.
          rv_json = lv_rest+lv_brace(lv_obj_len).
          RETURN.
        ENDIF.
      ENDIF.
      lv_i = lv_i + 1.
    ENDWHILE.
  ENDMETHOD.


  METHOD build_error.
    rs_response = VALUE #(
      id      = iv_id
      success = abap_false
      error   = json_obj( |"code":"{ iv_code }","message":"{ escape_json( iv_message ) }"| )
    ).
  ENDMETHOD.


  METHOD build_success.
    rs_response = VALUE #(
      id      = iv_id
      success = abap_true
      data    = iv_data
    ).
  ENDMETHOD.


  METHOD json_obj.
    DATA(lv_open) = '{'.
    DATA(lv_close) = '}'.
    rv_json = |{ lv_open }{ iv_content }{ lv_close }|.
  ENDMETHOD.


  METHOD json_arr.
    rv_json = |[{ iv_content }]|.
  ENDMETHOD.


  METHOD json_str.
    rv_json = |"{ iv_key }":"{ escape_json( iv_value ) }"|.
  ENDMETHOD.


  METHOD json_int.
    rv_json = |"{ iv_key }":{ iv_value }|.
  ENDMETHOD.


  METHOD json_bool.
    DATA(lv_val) = COND string( WHEN iv_value = abap_true THEN 'true' ELSE 'false' ).
    rv_json = |"{ iv_key }":{ lv_val }|.
  ENDMETHOD.


  METHOD json_join.
    DATA lv_first TYPE abap_bool VALUE abap_true.
    LOOP AT it_parts INTO DATA(lv_part).
      IF lv_part IS INITIAL.
        CONTINUE.
      ENDIF.
      IF lv_first = abap_false.
        rv_json = |{ rv_json },|.
      ENDIF.
      rv_json = |{ rv_json }{ lv_part }|.
      lv_first = abap_false.
    ENDLOOP.
  ENDMETHOD.

ENDCLASS.
