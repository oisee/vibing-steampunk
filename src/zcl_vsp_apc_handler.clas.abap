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
    "! Set when two services claim one domain: the handler then refuses
    "! every request, naming both classes, rather than pick one.
    CLASS-DATA gv_services_error TYPE string.

    CLASS-METHODS discover_services.

    CLASS-METHODS add_service
      IMPORTING io_service TYPE REF TO zif_vsp_service.

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
    add_service( NEW zcl_vsp_rfc_service( ) ).
    add_service( NEW zcl_vsp_debug_service( ) ).
    add_service( NEW zcl_vsp_amdp_service( ) ).
    add_service( NEW zcl_vsp_report_service( ) ).
    discover_services( ).
  ENDMETHOD.

  METHOD discover_services.
    " Every other service is found, not named: the git service exists only
    " where abapGit does (vsp install skips it otherwise), an administrator
    " may deploy ZADT_VSP without the transport service, and a service may
    " come from a package of its own (the print forms). The handler must
    " activate and run without any of them; a domain that is missing answers
    " UNKNOWN_DOMAIN.
    "
    " Only an active class named ZCL_VSP_*_SERVICE that implements
    " ZIF_VSP_SERVICE is taken, so an arbitrary class cannot put itself on
    " the WebSocket.
    DATA lo_service TYPE REF TO zif_vsp_service.

    SELECT clsname FROM seometarel
      WHERE refclsname = 'ZIF_VSP_SERVICE'
        AND reltype    = '1'
        AND version    = '1'
        AND clsname    LIKE 'ZCL\_VSP\_%\_SERVICE' ESCAPE '\'
      ORDER BY clsname
      INTO TABLE @DATA(lt_classes).

    LOOP AT lt_classes INTO DATA(ls_class).
      CASE ls_class-clsname.
        WHEN 'ZCL_VSP_RFC_SERVICE' OR 'ZCL_VSP_DEBUG_SERVICE'
          OR 'ZCL_VSP_AMDP_SERVICE' OR 'ZCL_VSP_REPORT_SERVICE'.
          CONTINUE.
      ENDCASE.
      TRY.
          CREATE OBJECT lo_service TYPE (ls_class-clsname).
          add_service( lo_service ).
        CATCH cx_sy_create_object_error ##NO_HANDLER.
      ENDTRY.
    ENDLOOP.
  ENDMETHOD.

  METHOD add_service.
    " Two services on one domain are an installation error. Neither wins:
    " which one did would depend on class names, and nobody would notice.
    DATA(lv_domain) = io_service->get_domain( ).
    LOOP AT gt_services INTO DATA(lo_known).
      IF lo_known->get_domain( ) = lv_domain.
        DATA(lv_entry) = |domain '{ lv_domain }' is served by both {
          replace( val = cl_abap_classdescr=>get_class_name( lo_known ) sub = '\CLASS=' with = `` ) } and {
          replace( val = cl_abap_classdescr=>get_class_name( io_service ) sub = '\CLASS=' with = `` ) }|.
        gv_services_error = COND #( WHEN gv_services_error IS INITIAL THEN lv_entry
                                    ELSE |{ gv_services_error }; { lv_entry }| ).
        RETURN.
      ENDIF.
    ENDLOOP.
    APPEND io_service TO gt_services.
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

    " Push: bind this WebSocket to its own extension of AMC channel
    " ZVSP_TRANSPORT /buffer, on which the transport service's background
    " job publishes the outcome of an add. Whatever goes wrong here -- the
    " AMC application missing (an abapGit deploy, a failed install step) or
    " anything else -- must not take the WebSocket down with it: the session
    " goes on without push, and outcomes are read with the status call.
    DATA(lv_push) = abap_false.
    TRY.
        i_context->get_binding_manager( )->bind_amc_message_consumer(
          i_application_id       = 'ZVSP_TRANSPORT'
          i_channel_id           = '/buffer'
          i_channel_extension_id = CONV #( mv_session_id ) ).
        lv_push = abap_true.
      CATCH cx_root ##CATCH_ALL.
        lv_push = abap_false.
    ENDTRY.
    " The same for the git service's import job: AMC ZVSP_GIT /import.
    " Without it, an import's outcome is read with import_status.
    DATA(lv_git_push) = abap_false.
    TRY.
        i_context->get_binding_manager( )->bind_amc_message_consumer(
          i_application_id       = 'ZVSP_GIT'
          i_channel_id           = '/import'
          i_channel_extension_id = CONV #( mv_session_id ) ).
        lv_git_push = abap_true.
      CATCH cx_root ##CATCH_ALL.
        lv_git_push = abap_false.
    ENDTRY.

    DATA lt_domains TYPE string_table.
    LOOP AT gt_services INTO DATA(lo_service).
      APPEND |"{ lo_service->get_domain( ) }"| TO lt_domains.
    ENDLOOP.

    DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'session' iv_value = mv_session_id ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'version' iv_value = '2.4.0' ) )
      ( |"domains":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_domains ) ) }| )
      ( zcl_vsp_utils=>json_bool( iv_key = 'push' iv_value = lv_push ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'git_push' iv_value = lv_git_push ) )
    ) ) ).

    IF gv_services_error IS NOT INITIAL.
      send_error( iv_id = 'welcome' iv_code = 'DUPLICATE_DOMAIN' iv_message = gv_services_error ).
      RETURN.
    ENDIF.

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
            " A brace inside a string value is not a brace: strings are
            " skipped, escapes honoured, or a value such as "{" cut the
            " params short and lost every key after it.
            DATA(lv_depth) = 0.
            DATA(lv_pos) = lv_brace_start.
            DATA(lv_len) = strlen( iv_text ).
            DATA(lv_in_str) = abap_false.
            WHILE lv_pos < lv_len.
              DATA(lv_char) = iv_text+lv_pos(1).
              IF lv_in_str = abap_true.
                IF lv_char = '\'.
                  lv_pos = lv_pos + 1.
                ELSEIF lv_char = '"'.
                  lv_in_str = abap_false.
                ENDIF.
              ELSEIF lv_char = '"'.
                lv_in_str = abap_true.
              ELSEIF lv_char = '{'.
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
    IF gv_services_error IS NOT INITIAL.
      rs_response = zcl_vsp_utils=>build_error(
        iv_id      = is_message-id
        iv_code    = 'DUPLICATE_DOMAIN'
        iv_message = |ZADT_VSP is misinstalled: { gv_services_error }|
      ).
      RETURN.
    ENDIF.

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
