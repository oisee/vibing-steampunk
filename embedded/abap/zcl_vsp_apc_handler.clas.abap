"! <p class="shorttext synchronized">VSP APC WebSocket Handler</p>
"! Unified WebSocket handler for vsp MCP server.
"! Provides stateful operations not available via standard ADT REST.
CLASS zcl_vsp_apc_handler DEFINITION
  PUBLIC
  INHERITING FROM cl_apc_wsp_ext_stateful_base
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    METHODS if_apc_wsp_extension~on_start REDEFINITION.
    METHODS if_apc_wsp_extension~on_message REDEFINITION.
    METHODS if_apc_wsp_extension~on_close REDEFINITION.
    METHODS if_apc_wsp_extension~on_error REDEFINITION.

    CLASS-METHODS class_constructor.

  PRIVATE SECTION.
    DATA mo_context TYPE REF TO if_apc_wsp_server_context.
    DATA mo_message_manager TYPE REF TO if_apc_wsp_message_manager.
    DATA mv_session_id TYPE string.

    CLASS-DATA gt_services TYPE STANDARD TABLE OF REF TO zif_vsp_service WITH KEY table_line.

    METHODS parse_message
      IMPORTING iv_text           TYPE string
      RETURNING VALUE(rs_message) TYPE zif_vsp_service=>ty_message.

    METHODS send_response
      IMPORTING is_response TYPE zif_vsp_service=>ty_response.

    METHODS send_error
      IMPORTING iv_id      TYPE string
                iv_code    TYPE string
                iv_message TYPE string.

    METHODS route_message
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_ping
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_abap_help
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

ENDCLASS.


CLASS zcl_vsp_apc_handler IMPLEMENTATION.

  METHOD class_constructor.
    APPEND NEW zcl_vsp_rfc_service( ) TO gt_services.
    APPEND NEW zcl_vsp_debug_service( ) TO gt_services.
    APPEND NEW zcl_vsp_amdp_service( ) TO gt_services.
    APPEND NEW zcl_vsp_git_service( ) TO gt_services.
    APPEND NEW zcl_vsp_report_service( ) TO gt_services.
  ENDMETHOD.

  METHOD if_apc_wsp_extension~on_start.
    mo_context = i_context.
    mo_message_manager = i_message_manager.

    DATA lv_uuid TYPE sysuuid_c32.
    TRY.
        lv_uuid = cl_system_uuid=>create_uuid_c32_static( ).
      CATCH cx_uuid_error.
        lv_uuid = |VSP{ sy-uzeit }{ sy-datum }|.
    ENDTRY.
    mv_session_id = lv_uuid.

    DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'session' iv_value = mv_session_id ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'version' iv_value = '2.3.0' ) )
      ( |"domains":["rfc","debug","amdp","git","report"]| )
    ) ) ).

    send_response( VALUE #(
      id      = 'welcome'
      success = abap_true
      data    = lv_data
    ) ).
  ENDMETHOD.

  METHOD if_apc_wsp_extension~on_message.
    TRY.
        DATA(lv_text) = i_message->get_text( ).

        DATA(ls_message) = parse_message( lv_text ).

        IF ls_message-id IS INITIAL.
          send_error( iv_id = 'unknown' iv_code = 'PARSE_ERROR' iv_message = 'Invalid message format' ).
          RETURN.
        ENDIF.

        DATA(ls_response) = route_message( ls_message ).
        send_response( ls_response ).

      CATCH cx_apc_error INTO DATA(lx_error) ##NO_HANDLER.
    ENDTRY.
  ENDMETHOD.

  METHOD if_apc_wsp_extension~on_close.
    LOOP AT gt_services INTO DATA(lo_service).
      lo_service->on_disconnect( mv_session_id ).
    ENDLOOP.
  ENDMETHOD.

  METHOD if_apc_wsp_extension~on_error.
    " Log error - could extend with more sophisticated handling
  ENDMETHOD.

  METHOD parse_message.
    TRY.
        FIND PCRE '"id"\s*:\s*"([^"]*)"' IN iv_text SUBMATCHES rs_message-id.
        FIND PCRE '"domain"\s*:\s*"([^"]*)"' IN iv_text SUBMATCHES rs_message-domain.
        FIND PCRE '"action"\s*:\s*"([^"]*)"' IN iv_text SUBMATCHES rs_message-action.

        " Handle nested JSON in params by finding the balanced braces
        DATA(lv_params_start) = find( val = iv_text sub = '"params"' ).
        IF lv_params_start >= 0.
          DATA(lv_brace_start) = find( val = iv_text off = lv_params_start sub = '{' ).
          IF lv_brace_start >= 0.
            " Count braces to find the matching closing brace
            DATA(lv_depth) = 0.
            DATA(lv_pos) = lv_brace_start.
            DATA(lv_len) = strlen( iv_text ).
            WHILE lv_pos < lv_len.
              DATA(lv_char) = iv_text+lv_pos(1).
              IF lv_char = '{'.
                lv_depth = lv_depth + 1.
              ELSEIF lv_char = '}'.
                lv_depth = lv_depth - 1.
                IF lv_depth = 0.
                  DATA(lv_params_len) = lv_pos - lv_brace_start + 1.
                  rs_message-params = iv_text+lv_brace_start(lv_params_len).
                  EXIT.
                ENDIF.
              ENDIF.
              lv_pos = lv_pos + 1.
            ENDWHILE.
          ENDIF.
        ENDIF.

        DATA lv_timeout TYPE string.
        FIND PCRE '"timeout"\s*:\s*(\d+)' IN iv_text SUBMATCHES lv_timeout.
        IF sy-subrc = 0.
          rs_message-timeout = lv_timeout.
        ELSE.
          rs_message-timeout = 30000.
        ENDIF.

      CATCH cx_root.
        CLEAR rs_message.
    ENDTRY.
  ENDMETHOD.

  METHOD send_response.
    TRY.
        DATA(lo_message) = mo_message_manager->create_message( ).

        DATA lt_items TYPE string_table.
        APPEND zcl_vsp_utils=>json_str( iv_key = 'id' iv_value = is_response-id ) TO lt_items.
        APPEND zcl_vsp_utils=>json_bool( iv_key = 'success' iv_value = is_response-success ) TO lt_items.

        IF is_response-data IS NOT INITIAL.
          APPEND |"data":{ is_response-data }| TO lt_items.
        ENDIF.

        IF is_response-error IS NOT INITIAL.
          APPEND |"error":{ is_response-error }| TO lt_items.
        ENDIF.

        DATA(lv_json) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( lt_items ) ).

        lo_message->set_text( lv_json ).
        mo_message_manager->send( lo_message ).

      CATCH cx_apc_error ##NO_HANDLER.
    ENDTRY.
  ENDMETHOD.

  METHOD send_error.
    send_response( zcl_vsp_utils=>build_error(
      iv_id      = iv_id
      iv_code    = iv_code
      iv_message = iv_message
    ) ).
  ENDMETHOD.

  METHOD route_message.
    IF is_message-domain = 'system'.
      CASE is_message-action.
        WHEN 'ping'.
          rs_response = handle_ping( is_message ).
          RETURN.
        WHEN 'get_abap_help'.
          rs_response = handle_abap_help( is_message ).
          RETURN.
      ENDCASE.
    ENDIF.

    LOOP AT gt_services INTO DATA(lo_service).
      IF lo_service->get_domain( ) = is_message-domain.
        TRY.
            rs_response = lo_service->handle_message(
              iv_session_id = mv_session_id
              is_message    = is_message
            ).
          CATCH cx_root INTO DATA(lx_service_error).
            DATA(lv_err_msg) = zcl_vsp_utils=>escape_json( lx_service_error->get_text( ) ).
            rs_response = VALUE #(
              id      = is_message-id
              success = abap_false
              error   = `{"code":"SERVICE_EXCEPTION","message":"` && lv_err_msg && `"}`
            ).
        ENDTRY.
        RETURN.
      ENDIF.
    ENDLOOP.

    rs_response = zcl_vsp_utils=>build_error(
      iv_id      = is_message-id
      iv_code    = 'UNKNOWN_DOMAIN'
      iv_message = |Domain '{ is_message-domain }' not found|
    ).
  ENDMETHOD.

  METHOD handle_ping.
    DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_bool( iv_key = 'pong' iv_value = abap_true ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'timestamp' iv_value = |{ sy-datum }T{ sy-uzeit }| ) )
    ) ) ).
    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).
  ENDMETHOD.


  METHOD handle_abap_help.
    " Get ABAP keyword documentation via CL_ABAP_DOCU
    DATA(lv_keyword) = zcl_vsp_utils=>extract_param(
      iv_params = is_message-params
      iv_name   = 'keyword'
    ).

    IF lv_keyword IS INITIAL.
      rs_response = zcl_vsp_utils=>build_error(
        iv_id      = is_message-id
        iv_code    = 'MISSING_PARAM'
        iv_message = 'keyword is required'
      ).
      RETURN.
    ENDIF.

    " Convert to uppercase for CL_ABAP_DOCU lookup
    TRANSLATE lv_keyword TO UPPER CASE.

    " Retrieve ABAP documentation
    DATA lt_html TYPE abapdocu_html_tab.
    DATA lv_html TYPE string.

    TRY.
        cl_abap_docu=>convert_itf_to_html(
          EXPORTING
            area   = 'ABEN'                    " ABAP English documentation
            name   = CONV #( lv_keyword )
            langu  = sy-langu
          IMPORTING
            html   = lt_html ).

        " Concatenate HTML lines
        LOOP AT lt_html INTO DATA(ls_html_line).
          lv_html = lv_html && ls_html_line.
        ENDLOOP.

      CATCH cx_abap_docu_conversion INTO DATA(lx_docu_error).
        " Try generic error message area if keyword not found in ABEN
        TRY.
            cl_abap_docu=>convert_itf_to_html(
              EXPORTING
                area   = 'AB'                  " Alternative area
                name   = CONV #( lv_keyword )
                langu  = sy-langu
              IMPORTING
                html   = lt_html ).

            LOOP AT lt_html INTO ls_html_line.
              lv_html = lv_html && ls_html_line.
            ENDLOOP.

          CATCH cx_abap_docu_conversion INTO DATA(lx_error2).
            " Documentation not found - return empty
            lv_html = ''.
        ENDTRY.
    ENDTRY.

    " Build response
    DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'keyword' iv_value = lv_keyword ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'html' iv_value = lv_html ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'found' iv_value = xsdbool( lv_html IS NOT INITIAL ) ) )
    ) ) ).

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).
  ENDMETHOD.

ENDCLASS.
