"! <p class="shorttext synchronized">VSP Debug Domain Service</p>
"! Provides full debugging capabilities via WebSocket.
"! Integrates with SAP TPDAPI for real debugger operations.
CLASS zcl_vsp_debug_service DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    INTERFACES zif_vsp_service.

    TYPES:
      BEGIN OF ty_breakpoint_state,
        id         TYPE string,
        kind       TYPE string,
        uri        TYPE string,
        line       TYPE i,
        enabled    TYPE abap_bool,
        condition  TYPE string,
        exception  TYPE string,
        statement  TYPE string,
      END OF ty_breakpoint_state,
      tt_breakpoints TYPE STANDARD TABLE OF ty_breakpoint_state WITH KEY id.

  PRIVATE SECTION.
    DATA mv_session_id TYPE string.
    DATA mv_debug_user TYPE sy-uname.
    DATA mt_breakpoints TYPE tt_breakpoints.
    DATA mv_attached_debuggee TYPE string.
    DATA mo_dbg_service TYPE REF TO if_tpdapi_service.
    DATA mo_dbg_session TYPE REF TO if_tpdapi_session.
    DATA mv_listener_active TYPE abap_bool.
    DATA mo_static_bp_services TYPE REF TO if_tpdapi_static_bp_services.
    DATA mv_bp_context_set TYPE abap_bool.

    " Maps our breakpoint IDs to TPDAPI breakpoint references
    TYPES:
      BEGIN OF ty_bp_mapping,
        id     TYPE string,
        ref_bp TYPE REF TO if_tpdapi_bp,
      END OF ty_bp_mapping,
      tt_bp_mappings TYPE STANDARD TABLE OF ty_bp_mapping WITH KEY id.
    DATA mt_bp_mappings TYPE tt_bp_mappings.

    METHODS handle_set_breakpoint
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_get_breakpoints
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_delete_breakpoint
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_listen
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_get_debuggees
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_attach
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_step
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_get_stack
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_get_variables
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_detach
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_get_status
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    " HTTP breakpoint handlers (CL_ABAP_DEBUGGER - writes to ABDBG_BPS)
    METHODS handle_set_http_breakpoint
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_get_http_breakpoints
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_delete_http_breakpoints
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS get_debugger_service
      RETURNING VALUE(ro_service) TYPE REF TO if_tpdapi_service.

    METHODS get_static_bp_services
      RETURNING VALUE(ro_services) TYPE REF TO if_tpdapi_static_bp_services
      RAISING   cx_tpdapi_failure.

    METHODS ensure_bp_context
      RAISING cx_tpdapi_invalid_user
              cx_tpdapi_invalid_param
              cx_tpdapi_not_authorized
              cx_tpdapi_failure.

    METHODS extract_program_from_uri
      IMPORTING iv_uri            TYPE string
      RETURNING VALUE(rv_program) TYPE string.

ENDCLASS.


CLASS zcl_vsp_debug_service IMPLEMENTATION.

  METHOD zif_vsp_service~get_domain.
    rv_domain = 'debug'.
  ENDMETHOD.

  METHOD zif_vsp_service~handle_message.
    mv_session_id = iv_session_id.
    IF mv_debug_user IS INITIAL.
      mv_debug_user = sy-uname.
    ENDIF.

    CASE is_message-action.
      WHEN 'setBreakpoint'.
        rs_response = handle_set_breakpoint( is_message ).
      WHEN 'getBreakpoints'.
        rs_response = handle_get_breakpoints( is_message ).
      WHEN 'deleteBreakpoint'.
        rs_response = handle_delete_breakpoint( is_message ).
      WHEN 'listen'.
        rs_response = handle_listen( is_message ).
      WHEN 'getDebuggees'.
        rs_response = handle_get_debuggees( is_message ).
      WHEN 'attach'.
        rs_response = handle_attach( is_message ).
      WHEN 'step'.
        rs_response = handle_step( is_message ).
      WHEN 'getStack'.
        rs_response = handle_get_stack( is_message ).
      WHEN 'getVariables'.
        rs_response = handle_get_variables( is_message ).
      WHEN 'detach'.
        rs_response = handle_detach( is_message ).
      WHEN 'getStatus'.
        rs_response = handle_get_status( is_message ).
      " HTTP breakpoints (CL_ABAP_DEBUGGER) - triggers on HTTP execution
      WHEN 'setHttpBreakpoint'.
        rs_response = handle_set_http_breakpoint( is_message ).
      WHEN 'getHttpBreakpoints'.
        rs_response = handle_get_http_breakpoints( is_message ).
      WHEN 'deleteHttpBreakpoints'.
        rs_response = handle_delete_http_breakpoints( is_message ).
      WHEN OTHERS.
        rs_response = zcl_vsp_utils=>build_error(
          iv_id      = is_message-id
          iv_code    = 'UNKNOWN_ACTION'
          iv_message = |Action '{ is_message-action }' not supported|
        ).
    ENDCASE.
  ENDMETHOD.

  METHOD zif_vsp_service~on_disconnect.
    " Clean up debug session
    IF mo_dbg_session IS NOT INITIAL.
      TRY.
          mo_dbg_session->get_control_services( )->end_debugger( ).
        CATCH cx_tpdapi_failure cx_root ##NO_HANDLER.
      ENDTRY.
    ENDIF.

    " Stop listener
    IF mv_listener_active = abap_true AND mo_dbg_service IS NOT INITIAL.
      TRY.
          mo_dbg_service->stop_listener_for_user( i_request_user = mv_debug_user ).
        CATCH cx_tpdapi_failure cx_root ##NO_HANDLER.
      ENDTRY.
    ENDIF.

    " Delete all breakpoints via TPDAPI
    IF mo_static_bp_services IS NOT INITIAL.
      LOOP AT mt_bp_mappings INTO DATA(ls_mapping).
        TRY.
            mo_static_bp_services->delete_breakpoint( i_ref_bp = ls_mapping-ref_bp ).
          CATCH cx_tpdapi_failure cx_root ##NO_HANDLER.
        ENDTRY.
      ENDLOOP.
    ENDIF.

    CLEAR: mv_attached_debuggee, mt_breakpoints, mt_bp_mappings,
           mo_dbg_session, mv_listener_active, mv_bp_context_set.
  ENDMETHOD.

  METHOD get_debugger_service.
    IF mo_dbg_service IS INITIAL.
      mo_dbg_service = cl_tpdapi_service=>s_get_instance( ).
    ENDIF.
    ro_service = mo_dbg_service.
  ENDMETHOD.

  METHOD get_static_bp_services.
    IF mo_static_bp_services IS INITIAL.
      DATA(lo_service) = get_debugger_service( ).
      mo_static_bp_services = lo_service->get_static_bp_services( ).
    ENDIF.
    ro_services = mo_static_bp_services.
  ENDMETHOD.

  METHOD ensure_bp_context.
    IF mv_bp_context_set = abap_true.
      RETURN.
    ENDIF.

    DATA(lo_bp_services) = get_static_bp_services( ).
    lo_bp_services->set_external_bp_context_user(
      i_ide_user     = mv_debug_user
      i_request_user = mv_debug_user
    ).
    mv_bp_context_set = abap_true.
  ENDMETHOD.

  METHOD extract_program_from_uri.
    " Extract program name from ADT URI formats:
    " /sap/bc/adt/programs/programs/ZTEST/source/main -> ZTEST
    " /sap/bc/adt/oo/classes/zcl_test/source/main -> ZCL_TEST
    " /sap/bc/adt/functions/groups/zfg_test/fmodules/zfm_test/source/main -> ZFM_TEST
    DATA(lv_uri) = to_upper( iv_uri ).
    SPLIT lv_uri AT '/' INTO TABLE DATA(lt_parts).

    " Look for common patterns
    LOOP AT lt_parts INTO DATA(lv_part).
      DATA(lv_idx) = sy-tabix.
      IF lv_part = 'PROGRAMS' OR lv_part = 'CLASSES' OR lv_part = 'FMODULES'.
        " Next part is the object name
        READ TABLE lt_parts INTO rv_program INDEX lv_idx + 1.
        IF sy-subrc = 0.
          " For classes, append CP for class pool
          IF lv_part = 'CLASSES'.
            rv_program = |{ rv_program }================CP|.
          ENDIF.
          EXIT.
        ENDIF.
      ENDIF.
    ENDLOOP.
  ENDMETHOD.

  METHOD handle_listen.
    DATA lv_timeout TYPE i.
    DATA lv_user TYPE syuname.

    lv_timeout = zcl_vsp_utils=>extract_param_int( iv_params = is_message-params iv_name = 'timeout' ).
    IF lv_timeout <= 0.
      lv_timeout = 60.
    ENDIF.
    IF lv_timeout > 240.
      lv_timeout = 240.
    ENDIF.

    lv_user = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'user' ).
    IF lv_user IS INITIAL.
      lv_user = mv_debug_user.
    ENDIF.

    IF cl_tpdapi_service=>is_debugging_available( ) IS INITIAL.
      rs_response = zcl_vsp_utils=>build_error(
        iv_id      = is_message-id
        iv_code    = 'DEBUG_NOT_AVAILABLE'
        iv_message = 'Debugging is not available on this system'
      ).
      RETURN.
    ENDIF.

    TRY.
        DATA(lo_service) = get_debugger_service( ).
        lo_service->activate_session_for_ext_debug( i_ide_user = mv_debug_user ).

        mv_listener_active = abap_true.
        lo_service->start_listener_for_user(
          i_request_user = lv_user
          i_ide_user     = mv_debug_user
          i_timeout      = lv_timeout ).

        DATA(lt_debuggees) = lo_service->get_waiting_debuggees(
          i_request_user = lv_user
          i_ide_user     = mv_debug_user ).

        mv_listener_active = abap_false.

        IF lt_debuggees IS INITIAL.
          DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
            ( zcl_vsp_utils=>json_str( iv_key = 'status' iv_value = 'timeout' ) )
            ( |"debuggees":[]| )
          ) ) ).
          rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).
        ELSE.
          DATA lt_dbg_items TYPE string_table.
          LOOP AT lt_debuggees INTO DATA(ls_debuggee).
            APPEND zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
              ( zcl_vsp_utils=>json_str( iv_key = 'id' iv_value = CONV #( ls_debuggee-debuggee_id ) ) )
              ( zcl_vsp_utils=>json_str( iv_key = 'host' iv_value = CONV #( ls_debuggee-host ) ) )
              ( zcl_vsp_utils=>json_str( iv_key = 'user' iv_value = CONV #( ls_debuggee-debuggee_user ) ) )
              ( zcl_vsp_utils=>json_str( iv_key = 'program' iv_value = CONV #( ls_debuggee-program ) ) )
              ( zcl_vsp_utils=>json_bool( iv_key = 'sameServer' iv_value = ls_debuggee-is_same_server ) )
            ) ) ) TO lt_dbg_items.
          ENDLOOP.

          lv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
            ( zcl_vsp_utils=>json_str( iv_key = 'status' iv_value = 'caught' ) )
            ( |"debuggees":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_dbg_items ) ) }| )
          ) ) ).
          rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).
        ENDIF.

      CATCH cx_abdbg_actext_lis_timeout.
        mv_listener_active = abap_false.
        lv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
          ( zcl_vsp_utils=>json_str( iv_key = 'status' iv_value = 'timeout' ) )
          ( |"debuggees":[]| )
        ) ) ).
        rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).

      CATCH cx_abdbg_actext_conflict_lis INTO DATA(lx_conflict).
        mv_listener_active = abap_false.
        rs_response = zcl_vsp_utils=>build_error(
          iv_id      = is_message-id
          iv_code    = 'LISTENER_CONFLICT'
          iv_message = |Another listener is already active: { lx_conflict->get_text( ) }|
        ).

      CATCH cx_tpdapi_failure cx_root INTO DATA(lx_error).
        mv_listener_active = abap_false.
        rs_response = zcl_vsp_utils=>build_error(
          iv_id      = is_message-id
          iv_code    = 'LISTEN_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.

  METHOD handle_get_debuggees.
    DATA lv_user TYPE syuname.

    lv_user = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'user' ).
    IF lv_user IS INITIAL.
      lv_user = mv_debug_user.
    ENDIF.

    TRY.
        DATA(lo_service) = get_debugger_service( ).
        DATA(lt_debuggees) = lo_service->get_waiting_debuggees(
          i_request_user = lv_user
          i_ide_user     = mv_debug_user ).

        DATA lt_dbg_items TYPE string_table.
        LOOP AT lt_debuggees INTO DATA(ls_debuggee).
          APPEND zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
            ( zcl_vsp_utils=>json_str( iv_key = 'id' iv_value = CONV #( ls_debuggee-debuggee_id ) ) )
            ( zcl_vsp_utils=>json_str( iv_key = 'host' iv_value = CONV #( ls_debuggee-host ) ) )
            ( zcl_vsp_utils=>json_str( iv_key = 'user' iv_value = CONV #( ls_debuggee-debuggee_user ) ) )
            ( zcl_vsp_utils=>json_str( iv_key = 'program' iv_value = CONV #( ls_debuggee-program ) ) )
          ) ) ) TO lt_dbg_items.
        ENDLOOP.

        DATA(lv_data) = zcl_vsp_utils=>json_obj(
          |"debuggees":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_dbg_items ) ) }|
        ).
        rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).

      CATCH cx_tpdapi_failure cx_root INTO DATA(lx_error).
        rs_response = zcl_vsp_utils=>build_error(
          iv_id      = is_message-id
          iv_code    = 'GET_DEBUGGEES_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.

  METHOD handle_attach.
    DATA(lv_debuggee_id) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'debuggeeId' ).
    IF lv_debuggee_id IS INITIAL.
      rs_response = zcl_vsp_utils=>build_error(
        iv_id      = is_message-id
        iv_code    = 'INVALID_PARAMS'
        iv_message = 'debuggeeId parameter required'
      ).
      RETURN.
    ENDIF.

    IF mo_dbg_session IS NOT INITIAL.
      rs_response = zcl_vsp_utils=>build_error(
        iv_id      = is_message-id
        iv_code    = 'ALREADY_ATTACHED'
        iv_message = |Already attached to debuggee { mv_attached_debuggee }. Detach first.|
      ).
      RETURN.
    ENDIF.

    TRY.
        DATA(lo_service) = get_debugger_service( ).
        mo_dbg_session = lo_service->attach_debuggee( i_debuggee_id = CONV #( lv_debuggee_id ) ).
        mv_attached_debuggee = lv_debuggee_id.

        DATA(lo_stack_handler) = mo_dbg_session->get_stack_handler( ).
        DATA(lt_stack) = lo_stack_handler->get_stack( ).

        DATA lv_program TYPE string.
        DATA lv_include TYPE string.
        DATA lv_line TYPE i.

        READ TABLE lt_stack INTO DATA(ls_stack) WITH KEY flg_active = abap_true.
        IF sy-subrc = 0.
          lv_program = ls_stack-program.
          lv_include = ls_stack-include.
          lv_line = ls_stack-line.
        ENDIF.

        DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
          ( zcl_vsp_utils=>json_bool( iv_key = 'attached' iv_value = abap_true ) )
          ( zcl_vsp_utils=>json_str( iv_key = 'debuggeeId' iv_value = lv_debuggee_id ) )
          ( zcl_vsp_utils=>json_str( iv_key = 'program' iv_value = lv_program ) )
          ( zcl_vsp_utils=>json_str( iv_key = 'include' iv_value = lv_include ) )
          ( zcl_vsp_utils=>json_int( iv_key = 'line' iv_value = lv_line ) )
        ) ) ).
        rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).

      CATCH cx_tpdapi_failure cx_root INTO DATA(lx_error).
        CLEAR: mo_dbg_session, mv_attached_debuggee.
        rs_response = zcl_vsp_utils=>build_error(
          iv_id      = is_message-id
          iv_code    = 'ATTACH_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.

  METHOD handle_step.
    IF mo_dbg_session IS INITIAL.
      rs_response = zcl_vsp_utils=>build_error(
        iv_id      = is_message-id
        iv_code    = 'NOT_ATTACHED'
        iv_message = 'Not attached to any debuggee. Use attach first.'
      ).
      RETURN.
    ENDIF.

    DATA(lv_step_type) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'type' ).
    IF lv_step_type IS INITIAL.
      lv_step_type = 'into'.
    ENDIF.

    TRY.
        DATA(lo_control) = mo_dbg_session->get_control_services( ).
        DATA lo_steptype TYPE REF TO ie_tpdapi_steptype.

        CASE lv_step_type.
          WHEN 'into'.
            lo_steptype = ce_tpdapi_steptype=>into.
          WHEN 'over'.
            lo_steptype = ce_tpdapi_steptype=>over.
          WHEN 'return'.
            lo_steptype = ce_tpdapi_steptype=>out.
          WHEN 'continue'.
            lo_steptype = ce_tpdapi_steptype=>continue.
          WHEN OTHERS.
            rs_response = zcl_vsp_utils=>build_error(
              iv_id      = is_message-id
              iv_code    = 'INVALID_STEP_TYPE'
              iv_message = |Invalid step type '{ lv_step_type }'. Use: into, over, return, continue|
            ).
            RETURN.
        ENDCASE.

        lo_control->debug_step( i_ref_steptype = lo_steptype ).

        DATA(lo_stack_handler) = mo_dbg_session->get_stack_handler( ).
        DATA(lt_stack) = lo_stack_handler->get_stack( ).

        DATA lv_program TYPE string.
        DATA lv_include TYPE string.
        DATA lv_line TYPE i.
        DATA lv_procname TYPE string.

        READ TABLE lt_stack INTO DATA(ls_stack) WITH KEY flg_active = abap_true.
        IF sy-subrc = 0.
          lv_program = ls_stack-program.
          lv_include = ls_stack-include.
          lv_line = ls_stack-line.
          lv_procname = ls_stack-procname.
        ENDIF.

        DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
          ( zcl_vsp_utils=>json_str( iv_key = 'stepped' iv_value = lv_step_type ) )
          ( zcl_vsp_utils=>json_str( iv_key = 'program' iv_value = lv_program ) )
          ( zcl_vsp_utils=>json_str( iv_key = 'include' iv_value = lv_include ) )
          ( zcl_vsp_utils=>json_int( iv_key = 'line' iv_value = lv_line ) )
          ( zcl_vsp_utils=>json_str( iv_key = 'procedure' iv_value = lv_procname ) )
        ) ) ).
        rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).

      CATCH cx_tpdapi_debuggee_ended.
        CLEAR: mo_dbg_session, mv_attached_debuggee.
        lv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
          ( zcl_vsp_utils=>json_str( iv_key = 'stepped' iv_value = lv_step_type ) )
          ( zcl_vsp_utils=>json_bool( iv_key = 'ended' iv_value = abap_true ) )
          ( zcl_vsp_utils=>json_str( iv_key = 'message' iv_value = 'Debuggee ended' ) )
        ) ) ).
        rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).

      CATCH cx_tpdapi_failure cx_root INTO DATA(lx_error).
        rs_response = zcl_vsp_utils=>build_error(
          iv_id      = is_message-id
          iv_code    = 'STEP_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.

  METHOD handle_get_stack.
    IF mo_dbg_session IS INITIAL.
      rs_response = zcl_vsp_utils=>build_error(
        iv_id      = is_message-id
        iv_code    = 'NOT_ATTACHED'
        iv_message = 'Not attached to any debuggee'
      ).
      RETURN.
    ENDIF.

    TRY.
        DATA(lo_stack_handler) = mo_dbg_session->get_stack_handler( ).
        DATA(lt_stack) = lo_stack_handler->get_stack( ).

        DATA(lt_frames) = VALUE string_table( ).

        LOOP AT lt_stack INTO DATA(ls_frame).
          APPEND zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
            ( zcl_vsp_utils=>json_int( iv_key = 'index' iv_value = sy-tabix - 1 ) )
            ( zcl_vsp_utils=>json_str( iv_key = 'program' iv_value = CONV #( ls_frame-program ) ) )
            ( zcl_vsp_utils=>json_str( iv_key = 'include' iv_value = CONV #( ls_frame-include ) ) )
            ( zcl_vsp_utils=>json_int( iv_key = 'line' iv_value = ls_frame-line ) )
            ( zcl_vsp_utils=>json_str( iv_key = 'procedure' iv_value = CONV #( ls_frame-procname ) ) )
            ( zcl_vsp_utils=>json_bool( iv_key = 'active' iv_value = ls_frame-flg_active ) )
            ( zcl_vsp_utils=>json_bool( iv_key = 'system' iv_value = ls_frame-flg_sysprog ) )
          ) ) ) TO lt_frames.
        ENDLOOP.

        DATA(lv_data) = zcl_vsp_utils=>json_obj(
          |"stack":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_frames ) ) }|
        ).
        rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).

      CATCH cx_tpdapi_failure cx_root INTO DATA(lx_error).
        rs_response = zcl_vsp_utils=>build_error(
          iv_id      = is_message-id
          iv_code    = 'GET_STACK_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.

  METHOD handle_get_variables.
    IF mo_dbg_session IS INITIAL.
      rs_response = zcl_vsp_utils=>build_error(
        iv_id      = is_message-id
        iv_code    = 'NOT_ATTACHED'
        iv_message = 'Not attached to any debuggee'
      ).
      RETURN.
    ENDIF.

    DATA(lv_scope) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'scope' ).
    IF lv_scope IS INITIAL.
      lv_scope = 'system'.
    ENDIF.

    TRY.
        DATA(lo_data_services) = mo_dbg_session->get_data_services( ).

        DATA lt_var_items TYPE string_table.

        IF lv_scope = 'system' OR lv_scope = 'all'.
          DATA(lt_sy_vars) = VALUE string_table(
            ( `SY-SUBRC` ) ( `SY-TABIX` ) ( `SY-INDEX` ) ( `SY-DBCNT` )
            ( `SY-UNAME` ) ( `SY-DATUM` ) ( `SY-UZEIT` ) ( `SY-CPROG` )
          ).

          LOOP AT lt_sy_vars INTO DATA(lv_var_name).
            TRY.
                DATA(lo_data) = lo_data_services->get_data( i_name = lv_var_name ).
                DATA(lv_value) = lo_data->get_quickinfo( ).

                APPEND zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
                  ( zcl_vsp_utils=>json_str( iv_key = 'name' iv_value = lv_var_name ) )
                  ( zcl_vsp_utils=>json_str( iv_key = 'value' iv_value = lv_value ) )
                  ( zcl_vsp_utils=>json_str( iv_key = 'scope' iv_value = 'system' ) )
                ) ) ) TO lt_var_items.

              CATCH cx_tpdapi_failure cx_root ##NO_HANDLER.
            ENDTRY.
          ENDLOOP.
        ENDIF.

        DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
          ( |"variables":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_var_items ) ) }| )
          ( zcl_vsp_utils=>json_str( iv_key = 'scope' iv_value = lv_scope ) )
        ) ) ).
        rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).

      CATCH cx_tpdapi_failure cx_root INTO DATA(lx_error).
        rs_response = zcl_vsp_utils=>build_error(
          iv_id      = is_message-id
          iv_code    = 'GET_VARIABLES_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.

  METHOD handle_detach.
    IF mo_dbg_session IS INITIAL.
      DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
        ( zcl_vsp_utils=>json_bool( iv_key = 'detached' iv_value = abap_true ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'message' iv_value = 'No active debug session' ) )
      ) ) ).
      rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).
      RETURN.
    ENDIF.

    TRY.
        DATA(lo_control) = mo_dbg_session->get_control_services( ).
        lo_control->end_debugger( ).

        DATA(lv_prev_debuggee) = mv_attached_debuggee.
        CLEAR: mo_dbg_session, mv_attached_debuggee.

        lv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
          ( zcl_vsp_utils=>json_bool( iv_key = 'detached' iv_value = abap_true ) )
          ( zcl_vsp_utils=>json_str( iv_key = 'debuggeeId' iv_value = lv_prev_debuggee ) )
        ) ) ).
        rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).

      CATCH cx_tpdapi_failure cx_root INTO DATA(lx_error).
        CLEAR: mo_dbg_session, mv_attached_debuggee.
        lv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
          ( zcl_vsp_utils=>json_bool( iv_key = 'detached' iv_value = abap_true ) )
          ( zcl_vsp_utils=>json_str( iv_key = 'warning' iv_value = lx_error->get_text( ) ) )
        ) ) ).
        rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).
    ENDTRY.
  ENDMETHOD.

  METHOD handle_set_breakpoint.
    DATA(lv_kind) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'kind' ).
    DATA(lv_uri) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'uri' ).
    DATA lv_line TYPE i.
    lv_line = zcl_vsp_utils=>extract_param_int( iv_params = is_message-params iv_name = 'line' ).
    DATA(lv_exception) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'exception' ).
    DATA(lv_statement) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'statement' ).
    DATA(lv_condition) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'condition' ).
    DATA(lv_program) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'program' ).
    DATA(lv_method) = to_upper( zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'method' ) ).
    DATA(lv_include) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'include' ).

    IF lv_kind IS INITIAL.
      lv_kind = 'line'.
    ENDIF.

    DATA lv_class TYPE string.
    CASE lv_kind.
      WHEN 'line'.
        IF ( lv_uri IS INITIAL AND lv_program IS INITIAL ) OR lv_line <= 0.
          rs_response = zcl_vsp_utils=>build_error(
            iv_id = is_message-id
            iv_code = 'INVALID_PARAMS'
            iv_message = 'Line breakpoint requires (uri or program) and line parameters'
          ).
          RETURN.
        ENDIF.
        " Extract program from URI if not provided directly
        IF lv_program IS INITIAL.
          lv_program = extract_program_from_uri( lv_uri ).
        ENDIF.
        IF lv_program IS INITIAL.
          rs_response = zcl_vsp_utils=>build_error(
            iv_id = is_message-id
            iv_code = 'INVALID_URI'
            iv_message = |Cannot extract program name from URI: { lv_uri }|
          ).
          RETURN.
        ENDIF.

        " Resolve method name to include for class breakpoints
        " This enables include-relative line numbers (line within method, not pool)
        IF lv_method IS NOT INITIAL.
          " Extract class name from program (remove =*CP suffix)
          " Class pool format: ZCL_xxx===========CP (padded to 30 chars + CP)
          lv_class = lv_program.
          IF lv_class CP '*=*CP'.
            " Find the position of the first '=' and extract class name
            DATA lv_eq_pos TYPE i.
            FIND '=' IN lv_class MATCH OFFSET lv_eq_pos.
            IF lv_eq_pos > 0.
              lv_class = lv_class(lv_eq_pos).
            ENDIF.
          ENDIF.
          TRY.
              DATA lo_incl_naming TYPE REF TO if_oo_class_incl_naming.
              lo_incl_naming ?= cl_oo_include_naming=>get_instance_by_name( CONV seoclsname( lv_class ) ).
              lv_include = lo_incl_naming->get_include_by_mtdname( CONV seocpdname( lv_method ) ).
            CATCH cx_root INTO DATA(lx_naming).
              " Fall back to no include if resolution fails
              CLEAR lv_include.
          ENDTRY.
        ENDIF.
      WHEN 'exception'.
        IF lv_exception IS INITIAL.
          rs_response = zcl_vsp_utils=>build_error(
            iv_id = is_message-id
            iv_code = 'INVALID_PARAMS'
            iv_message = 'Exception breakpoint requires exception parameter'
          ).
          RETURN.
        ENDIF.
      WHEN 'statement'.
        IF lv_statement IS INITIAL.
          rs_response = zcl_vsp_utils=>build_error(
            iv_id = is_message-id
            iv_code = 'INVALID_PARAMS'
            iv_message = 'Statement breakpoint requires statement parameter'
          ).
          RETURN.
        ENDIF.
      WHEN OTHERS.
        rs_response = zcl_vsp_utils=>build_error(
          iv_id = is_message-id
          iv_code = 'INVALID_KIND'
          iv_message = |Invalid breakpoint kind '{ lv_kind }'|
        ).
        RETURN.
    ENDCASE.

    " Generate breakpoint ID
    DATA lv_bp_id TYPE string.
    TRY.
        lv_bp_id = cl_system_uuid=>create_uuid_c32_static( ).
      CATCH cx_uuid_error.
        lv_bp_id = |BP{ sy-uzeit }{ sy-datum }|.
    ENDTRY.

    DATA lo_bp TYPE REF TO if_tpdapi_bp.
    " Set breakpoint via TPDAPI
    TRY.
        ensure_bp_context( ).
        DATA(lo_bp_services) = get_static_bp_services( ).

        CASE lv_kind.
          WHEN 'line'.
            " Use include-aware breakpoint if include is resolved
            IF lv_include IS NOT INITIAL.
              lo_bp ?= lo_bp_services->create_line_breakpoint(
                i_main_program = lv_program
                i_include      = lv_include
                i_line_nr      = lv_line
              ).
            ELSE.
              lo_bp ?= lo_bp_services->create_line_breakpoint(
                i_main_program = lv_program
                i_line_nr      = lv_line
              ).
            ENDIF.

          WHEN 'exception'.
            lo_bp ?= lo_bp_services->create_exception_breakpoint(
              i_exception = lv_exception
            ).

          WHEN 'statement'.
            lo_bp ?= lo_bp_services->create_statement_breakpoint(
              i_statement = lv_statement
            ).
        ENDCASE.

        " Store mapping between our ID and TPDAPI breakpoint
        APPEND VALUE #( id = lv_bp_id ref_bp = lo_bp ) TO mt_bp_mappings.

        " Also store in our internal table for getBreakpoints
        APPEND VALUE #(
          id        = lv_bp_id
          kind      = lv_kind
          uri       = lv_uri
          line      = lv_line
          enabled   = abap_true
          condition = lv_condition
          exception = lv_exception
          statement = lv_statement
        ) TO mt_breakpoints.

        DATA(lt_items) = VALUE string_table( ).
        APPEND zcl_vsp_utils=>json_str( iv_key = 'breakpointId' iv_value = lv_bp_id ) TO lt_items.
        APPEND zcl_vsp_utils=>json_str( iv_key = 'kind' iv_value = lv_kind ) TO lt_items.
        APPEND zcl_vsp_utils=>json_bool( iv_key = 'registered' iv_value = abap_true ) TO lt_items.
        IF lv_program IS NOT INITIAL.
          APPEND zcl_vsp_utils=>json_str( iv_key = 'program' iv_value = lv_program ) TO lt_items.
        ENDIF.
        IF lv_include IS NOT INITIAL.
          APPEND zcl_vsp_utils=>json_str( iv_key = 'include' iv_value = lv_include ) TO lt_items.
        ENDIF.
        IF lv_method IS NOT INITIAL.
          APPEND zcl_vsp_utils=>json_str( iv_key = 'method' iv_value = lv_method ) TO lt_items.
        ENDIF.
        IF lv_uri IS NOT INITIAL.
          APPEND zcl_vsp_utils=>json_str( iv_key = 'uri' iv_value = lv_uri ) TO lt_items.
          APPEND zcl_vsp_utils=>json_int( iv_key = 'line' iv_value = lv_line ) TO lt_items.
        ENDIF.
        IF lv_exception IS NOT INITIAL.
          APPEND zcl_vsp_utils=>json_str( iv_key = 'exception' iv_value = lv_exception ) TO lt_items.
        ENDIF.
        IF lv_statement IS NOT INITIAL.
          APPEND zcl_vsp_utils=>json_str( iv_key = 'statement' iv_value = lv_statement ) TO lt_items.
        ENDIF.
        DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( lt_items ) ).
        rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).

      CATCH cx_tpdapi_existing INTO DATA(lx_existing).
        rs_response = zcl_vsp_utils=>build_error(
          iv_id      = is_message-id
          iv_code    = 'BREAKPOINT_EXISTS'
          iv_message = |Breakpoint already exists: { lx_existing->get_text( ) }|
        ).

      CATCH cx_tpdapi_invalid_param cx_tpdapi_invalid_user
            cx_tpdapi_not_authorized cx_tpdapi_insufficient_data
            cx_tpdapi_failure cx_root INTO DATA(lx_error).
        rs_response = zcl_vsp_utils=>build_error(
          iv_id      = is_message-id
          iv_code    = 'SET_BREAKPOINT_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.

  METHOD handle_get_breakpoints.
    DATA lt_bp_items TYPE string_table.

    LOOP AT mt_breakpoints INTO DATA(ls_bp).
      DATA(lt_bp_props) = VALUE string_table( ).
      APPEND zcl_vsp_utils=>json_str( iv_key = 'id' iv_value = ls_bp-id ) TO lt_bp_props.
      APPEND zcl_vsp_utils=>json_str( iv_key = 'kind' iv_value = ls_bp-kind ) TO lt_bp_props.
      IF ls_bp-uri IS NOT INITIAL.
        APPEND zcl_vsp_utils=>json_str( iv_key = 'uri' iv_value = ls_bp-uri ) TO lt_bp_props.
        APPEND zcl_vsp_utils=>json_int( iv_key = 'line' iv_value = ls_bp-line ) TO lt_bp_props.
      ENDIF.
      IF ls_bp-exception IS NOT INITIAL.
        APPEND zcl_vsp_utils=>json_str( iv_key = 'exception' iv_value = ls_bp-exception ) TO lt_bp_props.
      ENDIF.
      IF ls_bp-statement IS NOT INITIAL.
        APPEND zcl_vsp_utils=>json_str( iv_key = 'statement' iv_value = ls_bp-statement ) TO lt_bp_props.
      ENDIF.
      APPEND zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( lt_bp_props ) ) TO lt_bp_items.
    ENDLOOP.

    DATA(lv_data) = zcl_vsp_utils=>json_obj(
      |"breakpoints":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_bp_items ) ) }|
    ).
    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).
  ENDMETHOD.

  METHOD handle_delete_breakpoint.
    DATA(lv_bp_id) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'breakpointId' ).

    IF lv_bp_id IS INITIAL.
      rs_response = zcl_vsp_utils=>build_error(
        iv_id = is_message-id
        iv_code = 'INVALID_PARAMS'
        iv_message = 'breakpointId parameter required'
      ).
      RETURN.
    ENDIF.

    " Find the TPDAPI breakpoint reference
    READ TABLE mt_bp_mappings WITH KEY id = lv_bp_id INTO DATA(ls_mapping).
    IF sy-subrc <> 0.
      " Check if it exists in our internal table (might be old format)
      READ TABLE mt_breakpoints WITH KEY id = lv_bp_id TRANSPORTING NO FIELDS.
      IF sy-subrc <> 0.
        rs_response = zcl_vsp_utils=>build_error(
          iv_id = is_message-id
          iv_code = 'NOT_FOUND'
          iv_message = |Breakpoint { lv_bp_id } not found|
        ).
        RETURN.
      ENDIF.
    ENDIF.

    " Delete via TPDAPI if we have a reference
    IF ls_mapping-ref_bp IS NOT INITIAL.
      TRY.
          DATA(lo_bp_services) = get_static_bp_services( ).
          lo_bp_services->delete_breakpoint( i_ref_bp = ls_mapping-ref_bp ).
        CATCH cx_tpdapi_failure cx_root INTO DATA(lx_error).
          " Log warning but continue - we'll still clean up our tables
      ENDTRY.
    ENDIF.

    " Clean up internal tables
    DELETE mt_bp_mappings WHERE id = lv_bp_id.
    DELETE mt_breakpoints WHERE id = lv_bp_id.

    DATA(lv_data) = zcl_vsp_utils=>json_obj(
      zcl_vsp_utils=>json_str( iv_key = 'deleted' iv_value = lv_bp_id )
    ).
    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).
  ENDMETHOD.

  METHOD handle_get_status.
    DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'sessionId' iv_value = mv_session_id ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'user' iv_value = CONV #( mv_debug_user ) ) )
      ( zcl_vsp_utils=>json_int( iv_key = 'breakpointCount' iv_value = lines( mt_breakpoints ) ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'attached' iv_value = xsdbool( mo_dbg_session IS NOT INITIAL ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'attachedDebuggee' iv_value = mv_attached_debuggee ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'listenerActive' iv_value = mv_listener_active ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'debuggingAvailable' iv_value = xsdbool( cl_tpdapi_service=>is_debugging_available( ) IS NOT INITIAL ) ) )
    ) ) ).
    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).
  ENDMETHOD.

  METHOD handle_set_http_breakpoint.
    " Uses CL_ABAP_DEBUGGER which writes to ABDBG_BPS - checked by HTTP execution
    DATA(lv_program) = to_upper( zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'program' ) ).
    DATA(lv_method) = to_upper( zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'method' ) ).
    DATA lv_line TYPE i.
    lv_line = zcl_vsp_utils=>extract_param_int( iv_params = is_message-params iv_name = 'line' ).

    IF lv_program IS INITIAL OR lv_line <= 0.
      rs_response = zcl_vsp_utils=>build_error(
        iv_id = is_message-id
        iv_code = 'INVALID_PARAMS'
        iv_message = 'HTTP breakpoint requires program and line parameters'
      ).
      RETURN.
    ENDIF.

    " For classes, resolve method to include name
    DATA lv_include TYPE syrepid.
    DATA lv_class TYPE string.
    DATA lv_main_program TYPE syrepid.

    " Check if this looks like a class (contains = or starts with Z/Y and has CL in name)
    IF lv_program CP '*=*CP' OR lv_program CP '*=*'.
      lv_class = lv_program.
      DATA lv_eq_pos TYPE i.
      FIND '=' IN lv_class MATCH OFFSET lv_eq_pos.
      IF lv_eq_pos > 0.
        lv_class = lv_class(lv_eq_pos).
      ENDIF.
    ELSEIF lv_program CP 'ZCL_*' OR lv_program CP 'YCL_*' OR lv_program CP 'CL_*'.
      lv_class = lv_program.
    ENDIF.

    IF lv_class IS NOT INITIAL.
      IF lv_method IS INITIAL.
        rs_response = zcl_vsp_utils=>build_error(
          iv_id = is_message-id
          iv_code = 'INVALID_PARAMS'
          iv_message = 'HTTP breakpoint for class requires method parameter'
        ).
        RETURN.
      ENDIF.

      TRY.
          DATA lo_incl_naming TYPE REF TO if_oo_class_incl_naming.
          lo_incl_naming ?= cl_oo_include_naming=>get_instance_by_name( CONV seoclsname( lv_class ) ).
          lv_include = lo_incl_naming->get_include_by_mtdname( CONV seocpdname( lv_method ) ).
          DATA(lv_padded_name) = |{ lv_class WIDTH = 30 PAD = '=' ALIGN = LEFT }|.
          lv_main_program = |{ lv_padded_name }CP|.
        CATCH cx_root INTO DATA(lx_naming).
          rs_response = zcl_vsp_utils=>build_error(
            iv_id = is_message-id
            iv_code = 'RESOLVE_FAILED'
            iv_message = |Cannot resolve class/method: { lx_naming->get_text( ) }|
          ).
          RETURN.
      ENDTRY.
    ELSE.
      lv_include = lv_program.
      lv_main_program = lv_program.
    ENDIF.

    " Build breakpoint structure - program is the include, line is within include
    DATA: lt_breakpoints TYPE breakpoints,
          ls_breakpoint  TYPE breakpoint.

    ls_breakpoint-program = lv_include.
    ls_breakpoint-line    = lv_line.
    APPEND ls_breakpoint TO lt_breakpoints.

    " Save via CL_ABAP_DEBUGGER (writes to ABDBG_BPS table)
    TRY.
        cl_abap_debugger=>save_http_breakpoints(
          EXPORTING
            username     = sy-uname
            main_program = lv_main_program
            breakpoints  = lt_breakpoints
          EXCEPTIONS
            too_many_breakpoints  = 1
            generate              = 2
            bp_position_not_found = 3
            error                 = 4
            wrong_parameters      = 5
            OTHERS                = 6 ).

        IF sy-subrc <> 0.
          rs_response = zcl_vsp_utils=>build_error(
            iv_id = is_message-id
            iv_code = 'SAVE_FAILED'
            iv_message = |Failed to save HTTP breakpoint (sy-subrc={ sy-subrc }): main={ lv_main_program }, incl={ lv_include }, line={ lv_line }|
          ).
          RETURN.
        ENDIF.

        " Success
        DATA(lt_result) = VALUE string_table(
          ( zcl_vsp_utils=>json_str( iv_key = 'type' iv_value = 'http' ) )
          ( zcl_vsp_utils=>json_str( iv_key = 'table' iv_value = 'ABDBG_BPS' ) )
          ( zcl_vsp_utils=>json_str( iv_key = 'program' iv_value = CONV #( lv_main_program ) ) )
          ( zcl_vsp_utils=>json_str( iv_key = 'include' iv_value = CONV #( lv_include ) ) )
          ( zcl_vsp_utils=>json_int( iv_key = 'line' iv_value = lv_line ) )
          ( zcl_vsp_utils=>json_str( iv_key = 'user' iv_value = CONV #( sy-uname ) ) )
          ( zcl_vsp_utils=>json_bool( iv_key = 'saved' iv_value = abap_true ) )
        ).
        IF lv_method IS NOT INITIAL.
          APPEND zcl_vsp_utils=>json_str( iv_key = 'method' iv_value = lv_method ) TO lt_result.
        ENDIF.
        DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( lt_result ) ).
        rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).

      CATCH cx_root INTO DATA(lx_error).
        rs_response = zcl_vsp_utils=>build_error(
          iv_id = is_message-id
          iv_code = 'SET_HTTP_BP_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.

  METHOD handle_get_http_breakpoints.
    " Read HTTP breakpoints from ABDBG_BPS via CL_ABAP_DEBUGGER
    DATA: lt_breakpoints TYPE breakpoints,
          lv_count       TYPE i.

    TRY.
        cl_abap_debugger=>read_http_breakpoints(
          EXPORTING
            main_program           = '*'
            username               = sy-uname
          IMPORTING
            breakpoints            = lt_breakpoints
            number_all_breakpoints = lv_count ).

        " Build response
        DATA lt_bp_items TYPE string_table.
        LOOP AT lt_breakpoints INTO DATA(ls_bp).
          DATA(lt_bp_props) = VALUE string_table(
            ( zcl_vsp_utils=>json_str( iv_key = 'program' iv_value = CONV #( ls_bp-program ) ) )
            ( zcl_vsp_utils=>json_int( iv_key = 'line' iv_value = ls_bp-line ) )
          ).
          APPEND zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( lt_bp_props ) ) TO lt_bp_items.
        ENDLOOP.

        DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
          ( zcl_vsp_utils=>json_str( iv_key = 'type' iv_value = 'http' ) )
          ( zcl_vsp_utils=>json_str( iv_key = 'table' iv_value = 'ABDBG_BPS' ) )
          ( zcl_vsp_utils=>json_int( iv_key = 'count' iv_value = lv_count ) )
          ( |"breakpoints":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_bp_items ) ) }| )
        ) ) ).
        rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).

      CATCH cx_root INTO DATA(lx_error).
        rs_response = zcl_vsp_utils=>build_error(
          iv_id = is_message-id
          iv_code = 'GET_HTTP_BP_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.

  METHOD handle_delete_http_breakpoints.
    " Delete all HTTP breakpoints for current user
    TRY.
        cl_abap_debugger=>delete_http_breakpoints(
          client   = sy-mandt
          username = sy-uname ).

        DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
          ( zcl_vsp_utils=>json_str( iv_key = 'type' iv_value = 'http' ) )
          ( zcl_vsp_utils=>json_bool( iv_key = 'deleted' iv_value = abap_true ) )
          ( zcl_vsp_utils=>json_str( iv_key = 'user' iv_value = CONV #( sy-uname ) ) )
        ) ) ).
        rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).

      CATCH cx_root INTO DATA(lx_error).
        rs_response = zcl_vsp_utils=>build_error(
          iv_id = is_message-id
          iv_code = 'DELETE_HTTP_BP_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.

ENDCLASS.
