CLASS zcl_vsp_report_service DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    INTERFACES zif_vsp_service.

  PRIVATE SECTION.
    METHODS handle_run_report
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_get_text_elements
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_set_text_elements
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_get_variants
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS extract_param
      IMPORTING iv_params       TYPE string
                iv_name         TYPE string
      RETURNING VALUE(rv_value) TYPE string.

    METHODS extract_param_object
      IMPORTING iv_params       TYPE string
                iv_name         TYPE string
      RETURNING VALUE(rv_json)  TYPE string.

    METHODS escape_json
      IMPORTING iv_string         TYPE string
      RETURNING VALUE(rv_escaped) TYPE string.

    METHODS build_error
      IMPORTING iv_id              TYPE string
                iv_code            TYPE string
                iv_message         TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

ENDCLASS.


CLASS zcl_vsp_report_service IMPLEMENTATION.

  METHOD zif_vsp_service~get_domain.
    rv_domain = 'report'.
  ENDMETHOD.

  METHOD zif_vsp_service~handle_message.
    CASE is_message-action.
      WHEN 'runReport'.
        rs_response = handle_run_report( is_message ).
      WHEN 'getTextElements'.
        rs_response = handle_get_text_elements( is_message ).
      WHEN 'setTextElements'.
        rs_response = handle_set_text_elements( is_message ).
      WHEN 'getVariants'.
        rs_response = handle_get_variants( is_message ).
      WHEN OTHERS.
        rs_response = build_error(
          iv_id      = is_message-id
          iv_code    = 'UNKNOWN_ACTION'
          iv_message = |Action '{ is_message-action }' not supported|
        ).
    ENDCASE.
  ENDMETHOD.

  METHOD zif_vsp_service~on_disconnect.
  ENDMETHOD.

  METHOD handle_run_report.
    DATA: lt_rsparams  TYPE TABLE OF rsparams,
          lr_data      TYPE REF TO data,
          lv_start     TYPE timestampl,
          lv_end       TYPE timestampl,
          lv_runtime   TYPE i,
          lv_report    TYPE progname,
          lv_variant   TYPE variant.

    DATA(lv_report_str) = extract_param( iv_params = is_message-params iv_name = 'report' ).
    DATA(lv_variant_str) = extract_param( iv_params = is_message-params iv_name = 'variant' ).
    DATA(lv_capture) = extract_param( iv_params = is_message-params iv_name = 'capture_alv' ).
    DATA(lv_max_str) = extract_param( iv_params = is_message-params iv_name = 'max_rows' ).
    DATA(lv_params_json) = extract_param_object( iv_params = is_message-params iv_name = 'params' ).

    IF lv_report_str IS INITIAL.
      rs_response = build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter report is required' ).
      RETURN.
    ENDIF.

    TRANSLATE lv_report_str TO UPPER CASE.
    lv_report = lv_report_str.
    IF lv_variant_str IS NOT INITIAL.
      TRANSLATE lv_variant_str TO UPPER CASE.
      lv_variant = lv_variant_str.
    ENDIF.

    DATA(lv_capture_alv) = COND abap_bool( WHEN lv_capture = 'false' THEN abap_false ELSE abap_true ).
    DATA(lv_max_rows) = COND i( WHEN lv_max_str IS NOT INITIAL THEN CONV i( lv_max_str ) ELSE 1000 ).

    SELECT SINGLE name FROM trdir INTO @DATA(lv_exists) WHERE name = @lv_report.
    IF sy-subrc <> 0.
      rs_response = build_error( iv_id = is_message-id iv_code = 'REPORT_NOT_FOUND' iv_message = |Report { lv_report } not found| ).
      RETURN.
    ENDIF.

    IF lv_params_json IS NOT INITIAL.
      DATA(lv_work) = lv_params_json.
      WHILE lv_work CS '"'.
        DATA lv_pname TYPE string.
        DATA lv_pval TYPE string.
        FIND PCRE '"([^"]+)"\s*:\s*"([^"]*)"' IN lv_work SUBMATCHES lv_pname lv_pval.
        IF sy-subrc = 0.
          TRANSLATE lv_pname TO UPPER CASE.
          DATA lv_selname TYPE rsscr_name.
          lv_selname = lv_pname.
          APPEND VALUE rsparams(
            selname = lv_selname
            kind    = 'P'
            sign    = 'I'
            option  = 'EQ'
            low     = lv_pval
          ) TO lt_rsparams.
          FIND FIRST OCCURRENCE OF |"{ lv_pname }"| IN lv_work MATCH OFFSET DATA(lv_off) MATCH LENGTH DATA(lv_len) IGNORING CASE.
          IF sy-subrc = 0 AND strlen( lv_work ) > lv_off + lv_len.
            lv_work = lv_work+lv_off.
            lv_work = lv_work+lv_len.
          ELSE.
            EXIT.
          ENDIF.
        ELSE.
          EXIT.
        ENDIF.
      ENDWHILE.
    ENDIF.

    GET TIME STAMP FIELD lv_start.

    DATA(lv_o) = '{'.
    DATA(lv_c) = '}'.
    DATA lv_json TYPE string.
    DATA lv_alv_json TYPE string.
    DATA lv_columns_json TYPE string.
    DATA lv_total_rows TYPE i.
    DATA lv_truncated TYPE abap_bool.

    TRY.
        IF lv_capture_alv = abap_true.
          cl_salv_bs_runtime_info=>set(
            display  = abap_false
            metadata = abap_true
            data     = abap_true ).
        ENDIF.

        IF lv_variant IS NOT INITIAL.
          SUBMIT (lv_report)
            USING SELECTION-SET lv_variant
            AND RETURN.
        ELSEIF lt_rsparams IS NOT INITIAL.
          SUBMIT (lv_report)
            WITH SELECTION-TABLE lt_rsparams
            AND RETURN.
        ELSE.
          SUBMIT (lv_report) AND RETURN.
        ENDIF.

        IF lv_capture_alv = abap_true.
          TRY.
              cl_salv_bs_runtime_info=>get_data_ref( IMPORTING r_data = lr_data ).

              IF lr_data IS BOUND.
                FIELD-SYMBOLS <lt_data> TYPE ANY TABLE.
                ASSIGN lr_data->* TO <lt_data>.
                lv_total_rows = lines( <lt_data> ).

                DATA(lo_type) = cl_abap_typedescr=>describe_by_data_ref( lr_data ).
                IF lo_type->kind = cl_abap_typedescr=>kind_table.
                  DATA(lo_table) = CAST cl_abap_tabledescr( lo_type ).
                  DATA(lo_struct) = CAST cl_abap_structdescr( lo_table->get_table_line_type( ) ).
                  lv_columns_json = '['.
                  DATA lv_col_first TYPE abap_bool VALUE abap_true.
                  LOOP AT lo_struct->components INTO DATA(ls_comp).
                    IF lv_col_first = abap_false.
                      lv_columns_json = |{ lv_columns_json },|.
                    ENDIF.
                    lv_columns_json = |{ lv_columns_json }{ lv_o }"name":"{ ls_comp-name }","type":"{ ls_comp-type_kind }"{ lv_c }|.
                    lv_col_first = abap_false.
                  ENDLOOP.
                  lv_columns_json = |{ lv_columns_json }]|.

                  lv_alv_json = '['.
                  DATA lv_row_first TYPE abap_bool VALUE abap_true.
                  DATA lv_row_count TYPE i.
                  LOOP AT <lt_data> ASSIGNING FIELD-SYMBOL(<ls_row>).
                    lv_row_count = lv_row_count + 1.
                    IF lv_row_count > lv_max_rows.
                      lv_truncated = abap_true.
                      EXIT.
                    ENDIF.
                    IF lv_row_first = abap_false.
                      lv_alv_json = |{ lv_alv_json },|.
                    ENDIF.
                    lv_alv_json = |{ lv_alv_json }{ lv_o }|.
                    DATA lv_fld_first TYPE abap_bool VALUE abap_true.
                    LOOP AT lo_struct->components INTO ls_comp.
                      IF lv_fld_first = abap_false.
                        lv_alv_json = |{ lv_alv_json },|.
                      ENDIF.
                      ASSIGN COMPONENT ls_comp-name OF STRUCTURE <ls_row> TO FIELD-SYMBOL(<fv>).
                      IF sy-subrc = 0.
                        DATA lv_val TYPE string.
                        TRY.
                            lv_val = <fv>.
                          CATCH cx_root.
                            lv_val = ''.
                        ENDTRY.
                        lv_alv_json = |{ lv_alv_json }"{ ls_comp-name }":"{ escape_json( lv_val ) }"|.
                      ENDIF.
                      lv_fld_first = abap_false.
                    ENDLOOP.
                    lv_alv_json = |{ lv_alv_json }{ lv_c }|.
                    lv_row_first = abap_false.
                  ENDLOOP.
                  lv_alv_json = |{ lv_alv_json }]|.
                ENDIF.
              ENDIF.
            CATCH cx_salv_bs_sc_runtime_info.
          ENDTRY.
        ENDIF.

      CATCH cx_root INTO DATA(lx_error).
        IF lv_capture_alv = abap_true.
          cl_salv_bs_runtime_info=>clear_all( ).
        ENDIF.
        rs_response = build_error( iv_id = is_message-id iv_code = 'SUBMIT_ERROR' iv_message = lx_error->get_text( ) ).
        RETURN.
    ENDTRY.

    IF lv_capture_alv = abap_true.
      cl_salv_bs_runtime_info=>clear_all( ).
    ENDIF.

    GET TIME STAMP FIELD lv_end.
    lv_runtime = cl_abap_tstmp=>subtract( tstmp1 = lv_end tstmp2 = lv_start ) * 1000.

    DATA(lv_alv_captured) = COND string( WHEN lv_alv_json IS NOT INITIAL THEN 'true' ELSE 'false' ).
    DATA(lv_trunc) = COND string( WHEN lv_truncated = abap_true THEN 'true' ELSE 'false' ).

    lv_json = |{ lv_o }"status":"success","report":"{ lv_report }","runtime_ms":{ lv_runtime },"alv_captured":{ lv_alv_captured }|.
    IF lv_alv_json IS NOT INITIAL.
      lv_json = |{ lv_json },"columns":{ lv_columns_json },"rows":{ lv_alv_json },"total_rows":{ lv_total_rows },"truncated":{ lv_trunc }|.
    ENDIF.
    lv_json = |{ lv_json }{ lv_c }|.

    rs_response = VALUE #( id = is_message-id success = abap_true data = lv_json ).
  ENDMETHOD.

  METHOD handle_get_text_elements.
    DATA: lt_textpool TYPE TABLE OF textpool,
          lv_program  TYPE progname.

    DATA(lv_prog_str) = extract_param( iv_params = is_message-params iv_name = 'program' ).
    DATA(lv_language) = extract_param( iv_params = is_message-params iv_name = 'language' ).

    IF lv_prog_str IS INITIAL.
      rs_response = build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter program is required' ).
      RETURN.
    ENDIF.

    TRANSLATE lv_prog_str TO UPPER CASE.
    lv_program = lv_prog_str.

    DATA lv_lang TYPE sy-langu.
    IF lv_language IS NOT INITIAL.
      lv_lang = lv_language(1).
    ELSE.
      lv_lang = sy-langu.
    ENDIF.

    READ TEXTPOOL lv_program INTO lt_textpool LANGUAGE lv_lang.

    DATA(lv_o) = '{'.
    DATA(lv_c) = '}'.
    DATA lv_json TYPE string.
    lv_json = |{ lv_o }"program":"{ lv_program }","language":"{ lv_lang }","selection_texts":{ lv_o }|.

    DATA lv_first TYPE abap_bool VALUE abap_true.
    DATA lv_entry_str TYPE string.
    LOOP AT lt_textpool INTO DATA(ls_text) WHERE id = 'S'.
      IF lv_first = abap_false.
        lv_json = |{ lv_json },|.
      ENDIF.
      DATA(lv_key) = ls_text-key.
      CONDENSE lv_key.
      " Selection text entry has 8-char key prefix - strip it
      lv_entry_str = ls_text-entry.
      IF strlen( lv_entry_str ) > 8.
        lv_entry_str = lv_entry_str+8.
      ENDIF.
      lv_json = |{ lv_json }"{ lv_key }":"{ escape_json( lv_entry_str ) }"|.
      lv_first = abap_false.
    ENDLOOP.

    lv_json = |{ lv_json }{ lv_c },"text_symbols":{ lv_o }|.

    lv_first = abap_true.
    LOOP AT lt_textpool INTO ls_text WHERE id = 'I'.
      IF lv_first = abap_false.
        lv_json = |{ lv_json },|.
      ENDIF.
      lv_key = ls_text-key.
      CONDENSE lv_key.
      lv_entry_str = ls_text-entry.
      lv_json = |{ lv_json }"{ lv_key }":"{ escape_json( lv_entry_str ) }"|.
      lv_first = abap_false.
    ENDLOOP.

    lv_json = |{ lv_json }{ lv_c }{ lv_c }|.
    rs_response = VALUE #( id = is_message-id success = abap_true data = lv_json ).
  ENDMETHOD.

  METHOD handle_set_text_elements.
    DATA: lt_textpool TYPE TABLE OF textpool,
          lv_program  TYPE progname.

    DATA(lv_prog_str) = extract_param( iv_params = is_message-params iv_name = 'program' ).
    DATA(lv_language) = extract_param( iv_params = is_message-params iv_name = 'language' ).
    DATA(lv_sel_json) = extract_param_object( iv_params = is_message-params iv_name = 'selection_texts' ).
    DATA(lv_sym_json) = extract_param_object( iv_params = is_message-params iv_name = 'text_symbols' ).

    IF lv_prog_str IS INITIAL.
      rs_response = build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter program is required' ).
      RETURN.
    ENDIF.

    TRANSLATE lv_prog_str TO UPPER CASE.
    lv_program = lv_prog_str.

    DATA lv_lang TYPE sy-langu.
    IF lv_language IS NOT INITIAL.
      lv_lang = lv_language(1).
    ELSE.
      lv_lang = sy-langu.
    ENDIF.

    READ TEXTPOOL lv_program INTO lt_textpool LANGUAGE lv_lang.

    DATA lv_sel_count TYPE i.
    DATA lv_sym_count TYPE i.

    IF lv_sel_json IS NOT INITIAL.
      DATA(lv_work) = lv_sel_json.
      WHILE lv_work CS '"'.
        DATA lv_key TYPE string.
        DATA lv_val TYPE string.
        FIND PCRE '"([^"]+)"\s*:\s*"([^"]*)"' IN lv_work SUBMATCHES lv_key lv_val.
        IF sy-subrc = 0.
          TRANSLATE lv_key TO UPPER CASE.
          REPLACE ALL OCCURRENCES OF '\"' IN lv_val WITH '"'.
          REPLACE ALL OCCURRENCES OF '\\' IN lv_val WITH '\'.

          DATA lv_textkey TYPE textpoolky.
          lv_textkey = lv_key.
          " Selection text entry must be: 8-char key prefix + text value
          DATA(lv_entry) = |{ lv_textkey WIDTH = 8 }{ lv_val }|.
          READ TABLE lt_textpool ASSIGNING FIELD-SYMBOL(<fs>) WITH KEY id = 'S' key = lv_textkey.
          IF sy-subrc = 0.
            <fs>-entry = lv_entry.
          ELSE.
            APPEND VALUE textpool( id = 'S' key = lv_textkey entry = lv_entry ) TO lt_textpool.
          ENDIF.
          lv_sel_count = lv_sel_count + 1.

          FIND FIRST OCCURRENCE OF |"{ lv_key }"| IN lv_work MATCH OFFSET DATA(lv_off) MATCH LENGTH DATA(lv_len) IGNORING CASE.
          IF sy-subrc = 0 AND strlen( lv_work ) > lv_off + lv_len.
            lv_work = lv_work+lv_off.
            lv_work = lv_work+lv_len.
          ELSE.
            EXIT.
          ENDIF.
        ELSE.
          EXIT.
        ENDIF.
      ENDWHILE.
    ENDIF.

    IF lv_sym_json IS NOT INITIAL.
      lv_work = lv_sym_json.
      WHILE lv_work CS '"'.
        CLEAR: lv_key, lv_val.
        FIND PCRE '"([^"]+)"\s*:\s*"([^"]*)"' IN lv_work SUBMATCHES lv_key lv_val.
        IF sy-subrc = 0.
          REPLACE ALL OCCURRENCES OF '\"' IN lv_val WITH '"'.
          REPLACE ALL OCCURRENCES OF '\\' IN lv_val WITH '\'.

          lv_textkey = lv_key.
          READ TABLE lt_textpool ASSIGNING <fs> WITH KEY id = 'I' key = lv_textkey.
          IF sy-subrc = 0.
            <fs>-entry = lv_val.
          ELSE.
            APPEND VALUE textpool( id = 'I' key = lv_textkey entry = lv_val ) TO lt_textpool.
          ENDIF.
          lv_sym_count = lv_sym_count + 1.

          FIND FIRST OCCURRENCE OF |"{ lv_key }"| IN lv_work MATCH OFFSET lv_off MATCH LENGTH lv_len.
          IF sy-subrc = 0 AND strlen( lv_work ) > lv_off + lv_len.
            lv_work = lv_work+lv_off.
            lv_work = lv_work+lv_len.
          ELSE.
            EXIT.
          ENDIF.
        ELSE.
          EXIT.
        ENDIF.
      ENDWHILE.
    ENDIF.

    INSERT TEXTPOOL lv_program FROM lt_textpool LANGUAGE lv_lang.

    DATA(lv_o) = '{'.
    DATA(lv_c) = '}'.
    DATA(lv_status) = COND string( WHEN sy-subrc = 0 THEN 'success' ELSE 'error' ).
    DATA lv_json TYPE string.
    lv_json = |{ lv_o }"status":"{ lv_status }","program":"{ lv_program }","language":"{ lv_lang }","selection_texts_set":{ lv_sel_count },"text_symbols_set":{ lv_sym_count }{ lv_c }|.

    rs_response = VALUE #( id = is_message-id success = abap_true data = lv_json ).
  ENDMETHOD.

  METHOD handle_get_variants.
    DATA: lt_varid   TYPE TABLE OF varid,
          lv_report  TYPE progname.

    DATA(lv_report_str) = extract_param( iv_params = is_message-params iv_name = 'report' ).

    IF lv_report_str IS INITIAL.
      rs_response = build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter report is required' ).
      RETURN.
    ENDIF.

    TRANSLATE lv_report_str TO UPPER CASE.
    lv_report = lv_report_str.

    SELECT * FROM varid INTO TABLE lt_varid
      WHERE report = lv_report
      ORDER BY variant.

    DATA(lv_o) = '{'.
    DATA(lv_c) = '}'.
    DATA lv_json TYPE string.
    lv_json = |{ lv_o }"report":"{ lv_report }","variants":[|.

    DATA lv_first TYPE abap_bool VALUE abap_true.
    LOOP AT lt_varid INTO DATA(ls_var).
      IF lv_first = abap_false.
        lv_json = |{ lv_json },|.
      ENDIF.
      DATA(lv_protected) = COND string( WHEN ls_var-protected = abap_true THEN 'true' ELSE 'false' ).
      lv_json = |{ lv_json }{ lv_o }"name":"{ ls_var-variant }","protected":{ lv_protected }{ lv_c }|.
      lv_first = abap_false.
    ENDLOOP.

    lv_json = |{ lv_json }]{ lv_c }|.
    rs_response = VALUE #( id = is_message-id success = abap_true data = lv_json ).
  ENDMETHOD.

  METHOD extract_param.
    DATA lv_name TYPE string.
    lv_name = iv_name.
    CONDENSE lv_name.

    DATA lv_search TYPE string.
    lv_search = |"{ lv_name }":|.
    DATA lv_pos TYPE i.
    FIND lv_search IN iv_params MATCH OFFSET lv_pos.
    IF sy-subrc = 0.
      DATA lv_rest TYPE string.
      lv_rest = iv_params+lv_pos.
      FIND PCRE ':\s*"([^"]*)"' IN lv_rest SUBMATCHES rv_value.
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

  METHOD escape_json.
    rv_escaped = iv_string.
    REPLACE ALL OCCURRENCES OF '\' IN rv_escaped WITH '\\'.
    REPLACE ALL OCCURRENCES OF '"' IN rv_escaped WITH '\"'.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>cr_lf IN rv_escaped WITH '\n'.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>newline IN rv_escaped WITH '\n'.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>horizontal_tab IN rv_escaped WITH '\t'.
  ENDMETHOD.

  METHOD build_error.
    DATA(lv_o) = '{'.
    DATA(lv_c) = '}'.
    rs_response = VALUE #(
      id      = iv_id
      success = abap_false
      error   = |{ lv_o }"code":"{ iv_code }","message":"{ escape_json( iv_message ) }"{ lv_c }|
    ).
  ENDMETHOD.

ENDCLASS.
