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

    " Service registry
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

    METHODS escape_json
      IMPORTING iv_string         TYPE string
      RETURNING VALUE(rv_escaped) TYPE string.

ENDCLASS.


CLASS zcl_vsp_apc_handler IMPLEMENTATION.

  METHOD class_constructor.
    " Register available services
    APPEND NEW zcl_vsp_rfc_service( ) TO gt_services.
  ENDMETHOD.


  METHOD if_apc_wsp_extension~on_start.
    mo_context = i_context.
    mo_message_manager = i_message_manager.

    " Generate session ID
    DATA lv_uuid TYPE sysuuid_c32.
    TRY.
        lv_uuid = cl_system_uuid=>create_uuid_c32_static( ).
      CATCH cx_uuid_error.
        lv_uuid = |VSP{ sy-uzeit }{ sy-datum }|.
    ENDTRY.
    mv_session_id = lv_uuid.

    " Send welcome message
    DATA(lv_brace_open) = '{'.
    DATA(lv_brace_close) = '}'.
    DATA(lv_data) = |{ lv_brace_open }"session":"{ mv_session_id }","version":"1.0.0","domains":["rfc"]{ lv_brace_close }|.

    send_response( VALUE #(
      id      = 'welcome'
      success = abap_true
      data    = lv_data
    ) ).
  ENDMETHOD.


  METHOD if_apc_wsp_extension~on_message.
    DATA(lv_text) = i_message->get_text( ).

    " Parse incoming message
    DATA(ls_message) = parse_message( lv_text ).

    IF ls_message-id IS INITIAL.
      send_error( iv_id = 'unknown' iv_code = 'PARSE_ERROR' iv_message = 'Invalid message format' ).
      RETURN.
    ENDIF.

    " Route and handle
    DATA(ls_response) = route_message( ls_message ).
    send_response( ls_response ).
  ENDMETHOD.


  METHOD if_apc_wsp_extension~on_close.
    " Cleanup - notify services of disconnect
    LOOP AT gt_services INTO DATA(lo_service).
      lo_service->on_disconnect( mv_session_id ).
    ENDLOOP.
  ENDMETHOD.


  METHOD if_apc_wsp_extension~on_error.
    " Log error - could extend with more sophisticated handling
  ENDMETHOD.


  METHOD parse_message.
    TRY.
        " Extract fields using regex
        FIND REGEX '"id"\s*:\s*"([^"]*)"' IN iv_text SUBMATCHES rs_message-id.
        FIND REGEX '"domain"\s*:\s*"([^"]*)"' IN iv_text SUBMATCHES rs_message-domain.
        FIND REGEX '"action"\s*:\s*"([^"]*)"' IN iv_text SUBMATCHES rs_message-action.

        " Extract params as raw JSON substring
        FIND REGEX '"params"\s*:\s*(\{[^}]*\})' IN iv_text SUBMATCHES rs_message-params.

        " Extract timeout if present
        DATA lv_timeout TYPE string.
        FIND REGEX '"timeout"\s*:\s*(\d+)' IN iv_text SUBMATCHES lv_timeout.
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

        DATA(lv_brace_open) = '{'.
        DATA(lv_brace_close) = '}'.
        DATA(lv_success) = COND string( WHEN is_response-success = abap_true THEN 'true' ELSE 'false' ).

        DATA(lv_json) = |{ lv_brace_open }"id":"{ is_response-id }","success":{ lv_success }|.

        IF is_response-data IS NOT INITIAL.
          lv_json = |{ lv_json },"data":{ is_response-data }|.
        ENDIF.

        IF is_response-error IS NOT INITIAL.
          lv_json = |{ lv_json },"error":{ is_response-error }|.
        ENDIF.

        lv_json = |{ lv_json }{ lv_brace_close }|.

        lo_message->set_text( lv_json ).
        mo_message_manager->send( lo_message ).

      CATCH cx_apc_error.
        " Connection may be closed - ignore
    ENDTRY.
  ENDMETHOD.


  METHOD send_error.
    DATA(lv_brace_open) = '{'.
    DATA(lv_brace_close) = '}'.
    DATA(lv_escaped_msg) = escape_json( iv_message ).

    send_response( VALUE #(
      id      = iv_id
      success = abap_false
      error   = |{ lv_brace_open }"code":"{ iv_code }","message":"{ lv_escaped_msg }"{ lv_brace_close }|
    ) ).
  ENDMETHOD.


  METHOD route_message.
    " Handle built-in actions
    IF is_message-domain = 'system'.
      CASE is_message-action.
        WHEN 'ping'.
          rs_response = handle_ping( is_message ).
          RETURN.
      ENDCASE.
    ENDIF.

    " Route to domain service
    LOOP AT gt_services INTO DATA(lo_service).
      IF lo_service->get_domain( ) = is_message-domain.
        rs_response = lo_service->handle_message(
          iv_session_id = mv_session_id
          is_message    = is_message
        ).
        RETURN.
      ENDIF.
    ENDLOOP.

    " Unknown domain
    DATA(lv_brace_open) = '{'.
    DATA(lv_brace_close) = '}'.
    rs_response = VALUE #(
      id      = is_message-id
      success = abap_false
      error   = |{ lv_brace_open }"code":"UNKNOWN_DOMAIN","message":"Domain '{ is_message-domain }' not found"{ lv_brace_close }|
    ).
  ENDMETHOD.


  METHOD handle_ping.
    DATA(lv_brace_open) = '{'.
    DATA(lv_brace_close) = '}'.
    rs_response = VALUE #(
      id      = is_message-id
      success = abap_true
      data    = |{ lv_brace_open }"pong":true,"timestamp":"{ sy-datum }T{ sy-uzeit }"{ lv_brace_close }|
    ).
  ENDMETHOD.


  METHOD escape_json.
    rv_escaped = iv_string.
    REPLACE ALL OCCURRENCES OF '"' IN rv_escaped WITH '\"'.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>cr_lf IN rv_escaped WITH '\n'.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>newline IN rv_escaped WITH '\n'.
  ENDMETHOD.

ENDCLASS.
