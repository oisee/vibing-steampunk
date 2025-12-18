"! <p class="shorttext synchronized">VSP RFC Service</p>
"! Enables dynamic RFC/BAPI calls via WebSocket.
"! Actions: call, search, getMetadata, ping
CLASS zcl_vsp_rfc_service DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    INTERFACES zif_vsp_service.

  PRIVATE SECTION.
    TYPES:
      BEGIN OF ty_param_info,
        parameter  TYPE rs38l_par_,
        kind       TYPE rs38l_kind,
        typ        TYPE string,
        dbfield    TYPE string,
        optional   TYPE rs38l_opti,
      END OF ty_param_info,
      tt_param_info TYPE STANDARD TABLE OF ty_param_info WITH KEY parameter.

    METHODS handle_call
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_search
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_get_metadata
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_ping
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS get_func_interface
      IMPORTING iv_function       TYPE rs38l_fnam
      EXPORTING et_import         TYPE tt_param_info
                et_export         TYPE tt_param_info
                et_changing       TYPE tt_param_info
                et_tables         TYPE tt_param_info
      RETURNING VALUE(rv_success) TYPE abap_bool.

    METHODS create_param_data
      IMPORTING is_param       TYPE ty_param_info
      RETURNING VALUE(ro_data) TYPE REF TO data.

    METHODS create_table_data
      IMPORTING is_param       TYPE ty_param_info
      RETURNING VALUE(ro_data) TYPE REF TO data.

    METHODS extract_param
      IMPORTING iv_params       TYPE string
                iv_name         TYPE string
      RETURNING VALUE(rv_value) TYPE string.

    METHODS escape_json
      IMPORTING iv_string         TYPE string
      RETURNING VALUE(rv_escaped) TYPE string.

    METHODS build_error
      IMPORTING iv_id            TYPE string
                iv_code          TYPE string
                iv_message       TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

ENDCLASS.


CLASS zcl_vsp_rfc_service IMPLEMENTATION.

  METHOD zif_vsp_service~get_domain.
    rv_domain = 'rfc'.
  ENDMETHOD.

  METHOD zif_vsp_service~handle_message.
    CASE is_message-action.
      WHEN 'call'.
        rs_response = handle_call( is_message ).
      WHEN 'search'.
        rs_response = handle_search( is_message ).
      WHEN 'getMetadata'.
        rs_response = handle_get_metadata( is_message ).
      WHEN 'ping'.
        rs_response = handle_ping( is_message ).
      WHEN OTHERS.
        rs_response = build_error(
          iv_id      = is_message-id
          iv_code    = 'UNKNOWN_ACTION'
          iv_message = 'Action not supported'
        ).
    ENDCASE.
  ENDMETHOD.

  METHOD zif_vsp_service~on_disconnect.
  ENDMETHOD.

  METHOD handle_call.
    DATA(lv_function) = extract_param( iv_params = is_message-params iv_name = 'function' ).
    IF lv_function IS INITIAL.
      rs_response = build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter function is required' ).
      RETURN.
    ENDIF.
    TRANSLATE lv_function TO UPPER CASE.

    DATA lt_import TYPE tt_param_info.
    DATA lt_export TYPE tt_param_info.
    DATA lt_changing TYPE tt_param_info.
    DATA lt_tables TYPE tt_param_info.

    IF get_func_interface( EXPORTING iv_function = CONV #( lv_function )
                           IMPORTING et_import = lt_import et_export = lt_export
                                     et_changing = lt_changing et_tables = lt_tables ) = abap_false.
      rs_response = build_error( iv_id = is_message-id iv_code = 'FUNC_NOT_FOUND' iv_message = |Function { lv_function } not found| ).
      RETURN.
    ENDIF.

    DATA lt_ptab TYPE abap_func_parmbind_tab.
    DATA lt_etab TYPE abap_func_excpbind_tab.
    DATA ls_ptab TYPE abap_func_parmbind.
    DATA lo_data TYPE REF TO data.
    DATA lv_val TYPE string.
    FIELD-SYMBOLS <fs_val> TYPE any.

    " Function's IMPORT params: we EXPORT values TO the function
    LOOP AT lt_import INTO DATA(ls_imp).
      CLEAR: ls_ptab, lo_data, lv_val.
      lo_data = create_param_data( ls_imp ).
      IF lo_data IS BOUND.
        lv_val = extract_param( iv_params = is_message-params iv_name = CONV #( ls_imp-parameter ) ).
        IF lv_val IS NOT INITIAL.
          ASSIGN lo_data->* TO <fs_val>.
          IF sy-subrc = 0.
            TRY.
                <fs_val> = lv_val.
              CATCH cx_root.
            ENDTRY.
          ENDIF.
        ENDIF.
        ls_ptab-name = ls_imp-parameter.
        ls_ptab-kind = abap_func_exporting.  " We export TO the function
        ls_ptab-value = lo_data.
        INSERT ls_ptab INTO TABLE lt_ptab.
      ENDIF.
    ENDLOOP.

    " Function's EXPORT params: we IMPORT values FROM the function
    LOOP AT lt_export INTO DATA(ls_exp).
      CLEAR: ls_ptab, lo_data.
      lo_data = create_param_data( ls_exp ).
      IF lo_data IS BOUND.
        ls_ptab-name = ls_exp-parameter.
        ls_ptab-kind = abap_func_importing.  " We import FROM the function
        ls_ptab-value = lo_data.
        INSERT ls_ptab INTO TABLE lt_ptab.
      ENDIF.
    ENDLOOP.

    " TABLES params: create internal tables
    LOOP AT lt_tables INTO DATA(ls_tbl).
      CLEAR: ls_ptab, lo_data.
      lo_data = create_table_data( ls_tbl ).
      IF lo_data IS BOUND.
        ls_ptab-name = ls_tbl-parameter.
        ls_ptab-kind = abap_func_tables.
        ls_ptab-value = lo_data.
        INSERT ls_ptab INTO TABLE lt_ptab.
      ENDIF.
    ENDLOOP.

    INSERT VALUE #( name = 'OTHERS' value = 99 ) INTO TABLE lt_etab.

    TRY.
        CALL FUNCTION lv_function
          PARAMETER-TABLE lt_ptab
          EXCEPTION-TABLE lt_etab.
      CATCH cx_root INTO DATA(lx_call).
        rs_response = build_error( iv_id = is_message-id iv_code = 'CALL_ERROR' iv_message = lx_call->get_text( ) ).
        RETURN.
    ENDTRY.

    DATA(lv_subrc) = sy-subrc.
    DATA(lv_o) = '{'.
    DATA(lv_c) = '}'.
    DATA lv_json TYPE string.
    lv_json = |{ lv_o }"subrc":{ lv_subrc },"exports":{ lv_o }|.

    DATA lv_first TYPE abap_bool VALUE abap_true.
    DATA lv_str TYPE string.
    " Get results from function's export params (our importing)
    LOOP AT lt_ptab INTO DATA(ls_out) WHERE kind = abap_func_importing.
      ASSIGN ls_out-value->* TO FIELD-SYMBOL(<fs_out>).
      IF sy-subrc = 0.
        IF lv_first = abap_false.
          lv_json = |{ lv_json },|.
        ENDIF.
        DATA(lv_pname) = CONV string( ls_out-name ).
        CONDENSE lv_pname.
        " Check if export is elementary or complex
        DATA(lo_exp_type) = cl_abap_typedescr=>describe_by_data( <fs_out> ).
        IF lo_exp_type->kind = cl_abap_typedescr=>kind_elem.
          TRY.
              lv_str = <fs_out>.
              lv_str = escape_json( lv_str ).
            CATCH cx_root.
              lv_str = ''.
          ENDTRY.
          lv_json = |{ lv_json }"{ lv_pname }":"{ lv_str }"|.
        ELSE.
          " Serialize structure as JSON object
          lv_json = |{ lv_json }"{ lv_pname }":{ lv_o }|.
          DATA(lo_exp_struc) = CAST cl_abap_structdescr( lo_exp_type ).
          DATA lv_exp_first TYPE abap_bool.
          lv_exp_first = abap_true.
          LOOP AT lo_exp_struc->components INTO DATA(ls_exp_comp).
            IF lv_exp_first = abap_false.
              lv_json = |{ lv_json },|.
            ENDIF.
            ASSIGN COMPONENT ls_exp_comp-name OF STRUCTURE <fs_out> TO FIELD-SYMBOL(<fs_exp_comp>).
            IF sy-subrc = 0.
              DATA(lo_comp_type) = cl_abap_typedescr=>describe_by_data( <fs_exp_comp> ).
              IF lo_comp_type->kind = cl_abap_typedescr=>kind_elem.
                TRY.
                    lv_str = <fs_exp_comp>.
                    lv_str = escape_json( lv_str ).
                  CATCH cx_root.
                    lv_str = ''.
                ENDTRY.
              ELSE.
                lv_str = '[complex]'.
              ENDIF.
              lv_json = |{ lv_json }"{ ls_exp_comp-name }":"{ lv_str }"|.
            ENDIF.
            lv_exp_first = abap_false.
          ENDLOOP.
          lv_json = |{ lv_json }{ lv_c }|.
        ENDIF.
        lv_first = abap_false.
      ENDIF.
    ENDLOOP.

    lv_json = |{ lv_json }{ lv_c },"tables":{ lv_o }|.

    " Get results from tables
    lv_first = abap_true.
    LOOP AT lt_ptab INTO ls_out WHERE kind = abap_func_tables.
      ASSIGN ls_out-value->* TO FIELD-SYMBOL(<fs_tab>).
      IF sy-subrc = 0.
        IF lv_first = abap_false.
          lv_json = |{ lv_json },|.
        ENDIF.
        lv_pname = CONV string( ls_out-name ).
        CONDENSE lv_pname.
        lv_json = |{ lv_json }"{ lv_pname }":[|.
        DATA lv_row_first TYPE abap_bool.
        lv_row_first = abap_true.
        TRY.
            LOOP AT <fs_tab> ASSIGNING FIELD-SYMBOL(<fs_row>).
              IF lv_row_first = abap_false.
                lv_json = |{ lv_json },|.
              ENDIF.
              " Serialize structure as JSON object
              lv_json = |{ lv_json }{ lv_o }|.
              DATA(lo_struc) = CAST cl_abap_structdescr( cl_abap_typedescr=>describe_by_data( <fs_row> ) ).
              DATA lv_comp_first TYPE abap_bool.
              lv_comp_first = abap_true.
              LOOP AT lo_struc->components INTO DATA(ls_comp).
                IF lv_comp_first = abap_false.
                  lv_json = |{ lv_json },|.
                ENDIF.
                ASSIGN COMPONENT ls_comp-name OF STRUCTURE <fs_row> TO FIELD-SYMBOL(<fs_comp>).
                IF sy-subrc = 0.
                  " Check if component is elementary or complex
                  DATA(lo_type) = cl_abap_typedescr=>describe_by_data( <fs_comp> ).
                  IF lo_type->kind = cl_abap_typedescr=>kind_elem.
                    TRY.
                        lv_str = <fs_comp>.
                        lv_str = escape_json( lv_str ).
                      CATCH cx_root.
                        lv_str = ''.
                    ENDTRY.
                  ELSE.
                    lv_str = '[complex]'.
                  ENDIF.
                  lv_json = |{ lv_json }"{ ls_comp-name }":"{ lv_str }"|.
                ENDIF.
                lv_comp_first = abap_false.
              ENDLOOP.
              lv_json = |{ lv_json }{ lv_c }|.
              lv_row_first = abap_false.
            ENDLOOP.
          CATCH cx_root.
            lv_json = |{ lv_json }{ lv_o }"error":"serialization failed"{ lv_c }|.
        ENDTRY.
        lv_json = |{ lv_json }]|.
        lv_first = abap_false.
      ENDIF.
    ENDLOOP.

    lv_json = |{ lv_json }{ lv_c }{ lv_c }|.
    rs_response = VALUE #( id = is_message-id success = abap_true data = lv_json ).
  ENDMETHOD.

  METHOD create_param_data.
    DATA lv_type TYPE string.

    " Check for non-empty typ first, then dbfield
    IF strlen( is_param-typ ) > 0.
      lv_type = is_param-typ.
    ELSEIF strlen( is_param-dbfield ) > 0.
      lv_type = is_param-dbfield.
    ELSE.
      CREATE DATA ro_data TYPE string.
      RETURN.
    ENDIF.

    TRY.
        CREATE DATA ro_data TYPE (lv_type).
      CATCH cx_sy_create_data_error.
        TRY.
            CREATE DATA ro_data TYPE string.
          CATCH cx_root.
            CLEAR ro_data.
        ENDTRY.
    ENDTRY.
  ENDMETHOD.

  METHOD create_table_data.
    DATA lv_type TYPE string.

    " For tables, typ contains the line type
    IF strlen( is_param-typ ) > 0.
      lv_type = is_param-typ.
    ELSEIF strlen( is_param-dbfield ) > 0.
      lv_type = is_param-dbfield.
    ELSE.
      RETURN.
    ENDIF.

    TRY.
        CREATE DATA ro_data TYPE STANDARD TABLE OF (lv_type).
      CATCH cx_sy_create_data_error.
        CLEAR ro_data.
    ENDTRY.
  ENDMETHOD.

  METHOD handle_get_metadata.
    DATA(lv_function) = extract_param( iv_params = is_message-params iv_name = 'function' ).
    IF lv_function IS INITIAL.
      rs_response = build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter function is required' ).
      RETURN.
    ENDIF.
    TRANSLATE lv_function TO UPPER CASE.

    DATA lt_import TYPE tt_param_info.
    DATA lt_export TYPE tt_param_info.
    DATA lt_changing TYPE tt_param_info.
    DATA lt_tables TYPE tt_param_info.

    IF get_func_interface( EXPORTING iv_function = CONV #( lv_function )
                           IMPORTING et_import = lt_import et_export = lt_export
                                     et_changing = lt_changing et_tables = lt_tables ) = abap_false.
      rs_response = build_error( iv_id = is_message-id iv_code = 'FUNC_NOT_FOUND' iv_message = |Function { lv_function } not found| ).
      RETURN.
    ENDIF.

    DATA(lv_o) = '{'.
    DATA(lv_c) = '}'.
    DATA lv_json TYPE string.
    lv_json = |{ lv_o }"function":"{ lv_function }","parameters":[|.

    DATA lv_first TYPE abap_bool VALUE abap_true.

    LOOP AT lt_import INTO DATA(ls_i).
      IF lv_first = abap_false. lv_json = |{ lv_json },|. ENDIF.
      DATA(lv_typ) = COND string( WHEN strlen( ls_i-typ ) > 0 THEN ls_i-typ ELSE ls_i-dbfield ).
      DATA(lv_opt) = COND string( WHEN ls_i-optional = abap_true THEN 'true' ELSE 'false' ).
      DATA(lv_pname) = CONV string( ls_i-parameter ).
      CONDENSE lv_pname.
      lv_json = |{ lv_json }{ lv_o }"name":"{ lv_pname }","kind":"importing","type":"{ lv_typ }","optional":{ lv_opt }{ lv_c }|.
      lv_first = abap_false.
    ENDLOOP.

    LOOP AT lt_export INTO DATA(ls_e).
      IF lv_first = abap_false. lv_json = |{ lv_json },|. ENDIF.
      lv_typ = COND string( WHEN strlen( ls_e-typ ) > 0 THEN ls_e-typ ELSE ls_e-dbfield ).
      lv_pname = CONV string( ls_e-parameter ).
      CONDENSE lv_pname.
      lv_json = |{ lv_json }{ lv_o }"name":"{ lv_pname }","kind":"exporting","type":"{ lv_typ }","optional":true{ lv_c }|.
      lv_first = abap_false.
    ENDLOOP.

    LOOP AT lt_changing INTO DATA(ls_ch).
      IF lv_first = abap_false. lv_json = |{ lv_json },|. ENDIF.
      lv_typ = COND string( WHEN strlen( ls_ch-typ ) > 0 THEN ls_ch-typ ELSE ls_ch-dbfield ).
      lv_pname = CONV string( ls_ch-parameter ).
      CONDENSE lv_pname.
      lv_json = |{ lv_json }{ lv_o }"name":"{ lv_pname }","kind":"changing","type":"{ lv_typ }","optional":true{ lv_c }|.
      lv_first = abap_false.
    ENDLOOP.

    LOOP AT lt_tables INTO DATA(ls_t).
      IF lv_first = abap_false. lv_json = |{ lv_json },|. ENDIF.
      lv_typ = COND string( WHEN strlen( ls_t-typ ) > 0 THEN ls_t-typ ELSE ls_t-dbfield ).
      lv_pname = CONV string( ls_t-parameter ).
      CONDENSE lv_pname.
      lv_json = |{ lv_json }{ lv_o }"name":"{ lv_pname }","kind":"tables","type":"{ lv_typ }","optional":true{ lv_c }|.
      lv_first = abap_false.
    ENDLOOP.

    lv_json = |{ lv_json }]{ lv_c }|.
    rs_response = VALUE #( id = is_message-id success = abap_true data = lv_json ).
  ENDMETHOD.

  METHOD get_func_interface.
    rv_success = abap_false.
    CLEAR: et_import, et_export, et_changing, et_tables.

    DATA lt_import TYPE STANDARD TABLE OF rsimp.
    DATA lt_export TYPE STANDARD TABLE OF rsexp.
    DATA lt_changing TYPE STANDARD TABLE OF rscha.
    DATA lt_tables TYPE STANDARD TABLE OF rstbl.
    DATA lt_except TYPE STANDARD TABLE OF rsexc.

    CALL FUNCTION 'FUNCTION_IMPORT_INTERFACE'
      EXPORTING
        funcname           = iv_function
      TABLES
        import_parameter   = lt_import
        export_parameter   = lt_export
        changing_parameter = lt_changing
        tables_parameter   = lt_tables
        exception_list     = lt_except
      EXCEPTIONS
        error_message      = 1
        function_not_found = 2
        invalid_name       = 3
        OTHERS             = 4.

    IF sy-subrc <> 0.
      RETURN.
    ENDIF.

    LOOP AT lt_import INTO DATA(ls_i).
      INSERT VALUE #( parameter = ls_i-parameter kind = 'I' typ = ls_i-typ dbfield = ls_i-dbfield optional = ls_i-optional ) INTO TABLE et_import.
    ENDLOOP.
    LOOP AT lt_export INTO DATA(ls_e).
      INSERT VALUE #( parameter = ls_e-parameter kind = 'E' typ = ls_e-typ dbfield = ls_e-dbfield ) INTO TABLE et_export.
    ENDLOOP.
    LOOP AT lt_changing INTO DATA(ls_ch).
      INSERT VALUE #( parameter = ls_ch-parameter kind = 'C' typ = ls_ch-typ dbfield = ls_ch-dbfield optional = ls_ch-optional ) INTO TABLE et_changing.
    ENDLOOP.
    LOOP AT lt_tables INTO DATA(ls_t).
      INSERT VALUE #( parameter = ls_t-parameter kind = 'T' typ = ls_t-typ dbfield = ls_t-dbstruct optional = ls_t-optional ) INTO TABLE et_tables.
    ENDLOOP.

    rv_success = abap_true.
  ENDMETHOD.

  METHOD handle_search.
    DATA lv_pattern TYPE string.
    lv_pattern = extract_param( iv_params = is_message-params iv_name = 'pattern' ).
    IF lv_pattern IS INITIAL.
      lv_pattern = '%'.
    ENDIF.
    TRANSLATE lv_pattern TO UPPER CASE.
    REPLACE ALL OCCURRENCES OF '*' IN lv_pattern WITH '%'.

    DATA lt_funcs TYPE STANDARD TABLE OF rs38l_fnam.
    SELECT funcname FROM tfdir INTO TABLE lt_funcs
      UP TO 100 ROWS
      WHERE funcname LIKE lv_pattern
      ORDER BY funcname.

    DATA lv_json TYPE string.
    DATA lv_first TYPE abap_bool VALUE abap_true.
    lv_json = '['.
    LOOP AT lt_funcs INTO DATA(lv_func).
      IF lv_first = abap_false.
        CONCATENATE lv_json ',' INTO lv_json.
      ENDIF.
      CONCATENATE lv_json '{"name":"' lv_func '"}' INTO lv_json.
      lv_first = abap_false.
    ENDLOOP.
    CONCATENATE lv_json ']' INTO lv_json.

    rs_response = VALUE #( id = is_message-id success = abap_true data = lv_json ).
  ENDMETHOD.

  METHOD handle_ping.
    DATA lv_ts TYPE string.
    lv_ts = sy-datum && sy-uzeit.
    rs_response = VALUE #( id = is_message-id success = abap_true data = '{"pong":true,"timestamp":"' && lv_ts && '"}' ).
  ENDMETHOD.

  METHOD extract_param.
    DATA lv_name TYPE string.
    lv_name = iv_name.
    CONDENSE lv_name.

    DATA lv_regex TYPE string.
    CONCATENATE '"' lv_name '":' INTO lv_regex.
    DATA lv_pos TYPE i.
    FIND lv_regex IN iv_params MATCH OFFSET lv_pos.
    IF sy-subrc = 0.
      DATA lv_rest TYPE string.
      lv_rest = iv_params+lv_pos.
      FIND REGEX ':\s*"([^"]*)"' IN lv_rest SUBMATCHES rv_value.
    ENDIF.
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
