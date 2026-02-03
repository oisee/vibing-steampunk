"! <p class="shorttext synchronized">VSP AMDP Debug Domain Service</p>
"! Provides AMDP (SQLScript/HANA) debugging capabilities via WebSocket.
"! Wraps IF_AMDP_DBG_MAIN and IF_AMDP_DBG_CONTROL APIs.
CLASS zcl_vsp_amdp_service DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    INTERFACES zif_vsp_service.

  PRIVATE SECTION.
    " Session state per WebSocket connection
    TYPES:
      BEGIN OF ty_session,
        session_id  TYPE string,
        dbg_main    TYPE REF TO if_amdp_dbg_main,
        dbg_control TYPE REF TO if_amdp_dbg_control,
        context_id  TYPE string,
        is_active   TYPE abap_bool,
      END OF ty_session.

    CLASS-DATA gt_sessions TYPE HASHED TABLE OF ty_session WITH UNIQUE KEY session_id.

    METHODS handle_start
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
                iv_session_id      TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_stop
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
                iv_session_id      TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_resume
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
                iv_session_id      TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_step
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
                iv_session_id      TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_set_breakpoint
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
                iv_session_id      TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_get_variables
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
                iv_session_id      TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_get_status
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
                iv_session_id      TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_execute_and_debug
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
                iv_session_id      TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS get_session
      IMPORTING iv_session_id     TYPE string
      RETURNING VALUE(rs_session) TYPE REF TO ty_session.

    METHODS escape_json
      IMPORTING iv_string         TYPE string
      RETURNING VALUE(rv_escaped) TYPE string.

    METHODS error_response
      IMPORTING iv_id              TYPE string
                iv_code            TYPE string
                iv_message         TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS extract_param
      IMPORTING iv_params       TYPE string
                iv_name         TYPE string
      RETURNING VALUE(rv_value) TYPE string.

ENDCLASS.


CLASS zcl_vsp_amdp_service IMPLEMENTATION.

  METHOD zif_vsp_service~get_domain.
    rv_domain = 'amdp'.
  ENDMETHOD.


  METHOD zif_vsp_service~handle_message.
    CASE is_message-action.
      WHEN 'start'.
        rs_response = handle_start( is_message = is_message iv_session_id = iv_session_id ).
      WHEN 'stop'.
        rs_response = handle_stop( is_message = is_message iv_session_id = iv_session_id ).
      WHEN 'resume'.
        rs_response = handle_resume( is_message = is_message iv_session_id = iv_session_id ).
      WHEN 'step'.
        rs_response = handle_step( is_message = is_message iv_session_id = iv_session_id ).
      WHEN 'setBreakpoint' OR 'set_breakpoint'.
        rs_response = handle_set_breakpoint( is_message = is_message iv_session_id = iv_session_id ).
      WHEN 'getVariables' OR 'get_variables'.
        rs_response = handle_get_variables( is_message = is_message iv_session_id = iv_session_id ).
      WHEN 'getStatus' OR 'get_status'.
        rs_response = handle_get_status( is_message = is_message iv_session_id = iv_session_id ).
      WHEN 'executeAndDebug' OR 'execute_and_debug'.
        rs_response = handle_execute_and_debug( is_message = is_message iv_session_id = iv_session_id ).
      WHEN OTHERS.
        rs_response = error_response(
          iv_id      = is_message-id
          iv_code    = 'UNKNOWN_ACTION'
          iv_message = |Unknown AMDP action: { is_message-action }|
        ).
    ENDCASE.
  ENDMETHOD.


  METHOD zif_vsp_service~on_disconnect.
    " Clean up AMDP session when WebSocket disconnects
    DATA(lr_session) = get_session( iv_session_id ).
    IF lr_session IS BOUND AND lr_session->is_active = abap_true.
      TRY.
          lr_session->dbg_main->stop( ).
        CATCH cx_root.
          " Ignore cleanup errors
      ENDTRY.
      lr_session->is_active = abap_false.
    ENDIF.
    DELETE gt_sessions WHERE session_id = iv_session_id.
  ENDMETHOD.


  METHOD handle_start.
    DATA: lt_response TYPE if_amdp_dbg_main=>tt_dbg_response.

    " Check if session already active
    DATA(lr_session) = get_session( iv_session_id ).
    IF lr_session IS BOUND AND lr_session->is_active = abap_true.
      rs_response = error_response(
        iv_id      = is_message-id
        iv_code    = 'SESSION_ACTIVE'
        iv_message = 'AMDP debug session already active. Stop first.'
      ).
      RETURN.
    ENDIF.

    " Extract parameters
    DATA(lv_user) = extract_param( iv_params = is_message-params iv_name = 'user' ).
    DATA(lv_cascade) = extract_param( iv_params = is_message-params iv_name = 'cascadeMode' ).

    IF lv_user IS INITIAL.
      lv_user = sy-uname.
    ENDIF.
    IF lv_cascade IS INITIAL.
      lv_cascade = 'FULL'.
    ENDIF.

    TRY.
        " Create AMDP debugger instance
        DATA(lo_dbg_main) = cl_amdp_dbg_main=>create( ).

        " Start debugger
        lo_dbg_main->start(
          EXPORTING
            im_for_user          = CONV #( lv_user )
            im_dbg_cascade_mode  = lv_cascade
            im_kill_active_dbg   = abap_true
          IMPORTING
            ex_response          = lt_response
        ).

        " Extract debugger_id from response first (needed for control interface)
        DATA lv_debugger_id TYPE string.
        LOOP AT lt_response INTO DATA(ls_resp) WHERE kind = 'START'.
          lv_debugger_id = ls_resp-values-start-debugger_id.
          EXIT.
        ENDLOOP.

        " Get control interface using debugger_id
        DATA lo_dbg_control TYPE REF TO if_amdp_dbg_control.
        IF lv_debugger_id IS NOT INITIAL.
          TRY.
              lo_dbg_control = cl_amdp_dbg_control=>create( lv_debugger_id ).
            CATCH cx_root.
              " Control interface not available - step/breakpoint features limited
              CLEAR lo_dbg_control.
          ENDTRY.
        ENDIF.

        " Store session
        DATA ls_session TYPE ty_session.
        ls_session-session_id  = iv_session_id.
        ls_session-dbg_main    = lo_dbg_main.
        ls_session-dbg_control = lo_dbg_control.
        ls_session-is_active   = abap_true.

        IF lr_session IS BOUND.
          lr_session->* = ls_session.
        ELSE.
          INSERT ls_session INTO TABLE gt_sessions.
        ENDIF.

        " Build response
        DATA(lv_brace_open) = '{'.
        DATA(lv_brace_close) = '}'.

        rs_response = VALUE #(
          id      = is_message-id
          success = abap_true
          data    = |{ lv_brace_open }"status":"started","debugger_id":"{ lv_debugger_id }","user":"{ lv_user }","cascadeMode":"{ lv_cascade }"{ lv_brace_close }|
        ).

      CATCH cx_root INTO DATA(lx_error).
        rs_response = error_response(
          iv_id      = is_message-id
          iv_code    = 'START_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.


  METHOD handle_stop.
    DATA(lr_session) = get_session( iv_session_id ).

    IF lr_session IS NOT BOUND OR lr_session->is_active = abap_false.
      rs_response = error_response(
        iv_id      = is_message-id
        iv_code    = 'NO_SESSION'
        iv_message = 'No active AMDP session'
      ).
      RETURN.
    ENDIF.

    TRY.
        lr_session->dbg_main->stop( ).
        lr_session->is_active = abap_false.

        DATA(lv_brace_open) = '{'.
        DATA(lv_brace_close) = '}'.
        rs_response = VALUE #(
          id      = is_message-id
          success = abap_true
          data    = |{ lv_brace_open }"status":"stopped"{ lv_brace_close }|
        ).

      CATCH cx_root INTO DATA(lx_error).
        rs_response = error_response(
          iv_id      = is_message-id
          iv_code    = 'STOP_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.


  METHOD handle_resume.
    DATA: lt_response TYPE if_amdp_dbg_main=>tt_dbg_response.

    DATA(lr_session) = get_session( iv_session_id ).

    IF lr_session IS NOT BOUND OR lr_session->is_active = abap_false.
      rs_response = error_response(
        iv_id      = is_message-id
        iv_code    = 'NO_SESSION'
        iv_message = 'No active AMDP session'
      ).
      RETURN.
    ENDIF.

    TRY.
        " Resume blocks until event occurs (breakpoint hit, execution end, etc.)
        lr_session->dbg_main->resume(
          IMPORTING
            ex_response = lt_response
        ).

        " Process responses and build JSON
        DATA(lv_brace_open) = '{'.
        DATA(lv_brace_close) = '}'.
        DATA lv_json TYPE string.
        lv_json = |{ lv_brace_open }"events":[|.

        DATA lv_first TYPE abap_bool VALUE abap_true.
        LOOP AT lt_response INTO DATA(ls_resp).
          IF lv_first = abap_false.
            lv_json = |{ lv_json },|.
          ENDIF.
          lv_first = abap_false.

          CASE ls_resp-kind.
            WHEN 'ON_BREAK'.
              " Store context_id for subsequent operations
              lr_session->context_id = ls_resp-values-on_break-context_id.

              lv_json = |{ lv_json }{ lv_brace_open }"kind":"on_break"|.
              lv_json = |{ lv_json },"context_id":"{ ls_resp-values-on_break-context_id }"|.
              lv_json = |{ lv_json },"bp_client_id":"{ ls_resp-values-on_break-bp_client_id }"|.

              " Position
              IF ls_resp-values-on_break-abap_position IS NOT INITIAL.
                lv_json = |{ lv_json },"abap_position":{ lv_brace_open }|.
                lv_json = |{ lv_json }"program":"{ ls_resp-values-on_break-abap_position-program_name }"|.
                lv_json = |{ lv_json },"include":"{ ls_resp-values-on_break-abap_position-include_name }"|.
                lv_json = |{ lv_json },"line":{ ls_resp-values-on_break-abap_position-line }{ lv_brace_close }|.
              ENDIF.

              IF ls_resp-values-on_break-native_position IS NOT INITIAL.
                lv_json = |{ lv_json },"native_position":{ lv_brace_open }|.
                lv_json = |{ lv_json }"schema":"{ ls_resp-values-on_break-native_position-schema_name }"|.
                lv_json = |{ lv_json },"name":"{ ls_resp-values-on_break-native_position-name }"|.
                lv_json = |{ lv_json },"line":{ ls_resp-values-on_break-native_position-line }{ lv_brace_close }|.
              ENDIF.

              " Variables summary
              lv_json = |{ lv_json },"variable_count":{ lines( ls_resp-values-on_break-local_variables ) }|.
              lv_json = |{ lv_json },"stack_depth":{ lines( ls_resp-values-on_break-callstack ) }|.
              lv_json = |{ lv_json }{ lv_brace_close }|.

            WHEN 'ON_EXECUTION_END'.
              lr_session->context_id = ''.
              lv_json = |{ lv_json }{ lv_brace_open }"kind":"on_execution_end"|.
              lv_json = |{ lv_json },"context_id":"{ ls_resp-values-on_execution_end-context_id }"|.
              lv_json = |{ lv_json },"aborted":{ COND #( WHEN ls_resp-values-on_execution_end-execution_aborted = abap_true THEN 'true' ELSE 'false' ) }{ lv_brace_close }|.

            WHEN 'ON_TOGGLE_BREAKPOINTS'.
              lv_json = |{ lv_json }{ lv_brace_open }"kind":"on_toggle_breakpoints"|.
              lv_json = |{ lv_json },"breakpoint_count":{ lines( ls_resp-values-on_toggle_breakpoints-breakpoints ) }{ lv_brace_close }|.

            WHEN OTHERS.
              lv_json = |{ lv_json }{ lv_brace_open }"kind":"{ ls_resp-kind }"{ lv_brace_close }|.
          ENDCASE.
        ENDLOOP.

        lv_json = |{ lv_json }]{ lv_brace_close }|.

        rs_response = VALUE #(
          id      = is_message-id
          success = abap_true
          data    = lv_json
        ).

      CATCH cx_root INTO DATA(lx_error).
        rs_response = error_response(
          iv_id      = is_message-id
          iv_code    = 'RESUME_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.


  METHOD handle_step.
    DATA(lr_session) = get_session( iv_session_id ).

    IF lr_session IS NOT BOUND OR lr_session->is_active = abap_false.
      rs_response = error_response(
        iv_id      = is_message-id
        iv_code    = 'NO_SESSION'
        iv_message = 'No active AMDP session'
      ).
      RETURN.
    ENDIF.

    IF lr_session->context_id IS INITIAL.
      rs_response = error_response(
        iv_id      = is_message-id
        iv_code    = 'NO_CONTEXT'
        iv_message = 'No debuggee context. Wait for breakpoint hit first.'
      ).
      RETURN.
    ENDIF.

    " Extract step type
    DATA(lv_step_type) = extract_param( iv_params = is_message-params iv_name = 'type' ).
    DATA lv_step_action TYPE if_amdp_dbg_control=>ty_dbg_step_action.

    CASE lv_step_type.
      WHEN 'into' OR 'stepInto'.
        lv_step_action = if_amdp_dbg_control=>co_dbg_step_action-step_into.
      WHEN 'over' OR 'stepOver' OR ''.
        lv_step_action = if_amdp_dbg_control=>co_dbg_step_action-step_over.
      WHEN 'out' OR 'stepOut'.
        lv_step_action = if_amdp_dbg_control=>co_dbg_step_action-step_out.
      WHEN 'continue'.
        lv_step_action = if_amdp_dbg_control=>co_dbg_step_action-continue.
      WHEN OTHERS.
        lv_step_action = if_amdp_dbg_control=>co_dbg_step_action-step_over.
    ENDCASE.

    IF lr_session->dbg_control IS NOT BOUND.
      rs_response = error_response(
        iv_id      = is_message-id
        iv_code    = 'NO_CONTROL'
        iv_message = 'Control interface not available. Step operations not supported.'
      ).
      RETURN.
    ENDIF.

    TRY.
        DATA(lv_req_id) = lr_session->dbg_control->continue(
          im_dbg_context_id = lr_session->context_id
          im_step_action    = lv_step_action
        ).

        DATA(lv_brace_open) = '{'.
        DATA(lv_brace_close) = '}'.
        rs_response = VALUE #(
          id      = is_message-id
          success = abap_true
          data    = |{ lv_brace_open }"status":"stepped","request_id":"{ lv_req_id }","step_type":"{ lv_step_type }"{ lv_brace_close }|
        ).

      CATCH cx_root INTO DATA(lx_error).
        rs_response = error_response(
          iv_id      = is_message-id
          iv_code    = 'STEP_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.


  METHOD handle_set_breakpoint.
    DATA(lr_session) = get_session( iv_session_id ).

    IF lr_session IS NOT BOUND OR lr_session->is_active = abap_false.
      rs_response = error_response(
        iv_id      = is_message-id
        iv_code    = 'NO_SESSION'
        iv_message = 'No active AMDP session. Start session first.'
      ).
      RETURN.
    ENDIF.

    " Extract parameters
    DATA(lv_program) = extract_param( iv_params = is_message-params iv_name = 'program' ).
    DATA(lv_include) = extract_param( iv_params = is_message-params iv_name = 'include' ).
    DATA(lv_line_str) = extract_param( iv_params = is_message-params iv_name = 'line' ).
    DATA(lv_client_id) = extract_param( iv_params = is_message-params iv_name = 'clientId' ).

    IF lv_program IS INITIAL OR lv_line_str IS INITIAL.
      rs_response = error_response(
        iv_id      = is_message-id
        iv_code    = 'INVALID_PARAMS'
        iv_message = 'Required: program, line'
      ).
      RETURN.
    ENDIF.

    DATA lv_line TYPE i.
    lv_line = lv_line_str.

    IF lv_include IS INITIAL.
      lv_include = lv_program.
    ENDIF.

    IF lv_client_id IS INITIAL.
      lv_client_id = |bp_{ lv_program }_{ lv_line }|.
    ENDIF.

    IF lr_session->dbg_control IS NOT BOUND.
      rs_response = error_response(
        iv_id      = is_message-id
        iv_code    = 'NO_CONTROL'
        iv_message = 'Control interface not available. Breakpoint operations not supported.'
      ).
      RETURN.
    ENDIF.

    TRY.
        " Build breakpoint request
        DATA lt_breakpoints TYPE if_amdp_dbg_main=>tt_dbg_breakpoint_req.
        DATA ls_bp TYPE if_amdp_dbg_main=>ty_dbg_breakpoint_req.

        ls_bp-client_id = lv_client_id.
        ls_bp-bp_kind   = if_amdp_dbg_main=>co_dbg_bp_kind-line.
        ls_bp-enabled   = abap_true.
        ls_bp-abap_position-program_name = to_upper( lv_program ).
        ls_bp-abap_position-include_name = to_upper( lv_include ).
        ls_bp-abap_position-line = lv_line.

        INSERT ls_bp INTO TABLE lt_breakpoints.

        " Sync breakpoints
        DATA(lv_req_id) = lr_session->dbg_control->sync_breakpoints(
          im_breakpoints = lt_breakpoints
        ).

        DATA(lv_brace_open) = '{'.
        DATA(lv_brace_close) = '}'.
        rs_response = VALUE #(
          id      = is_message-id
          success = abap_true
          data    = |{ lv_brace_open }"status":"set","request_id":"{ lv_req_id }","client_id":"{ lv_client_id }","program":"{ lv_program }","line":{ lv_line }{ lv_brace_close }|
        ).

      CATCH cx_root INTO DATA(lx_error).
        rs_response = error_response(
          iv_id      = is_message-id
          iv_code    = 'SET_BP_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.


  METHOD handle_get_variables.
    DATA(lr_session) = get_session( iv_session_id ).

    IF lr_session IS NOT BOUND OR lr_session->is_active = abap_false.
      rs_response = error_response(
        iv_id      = is_message-id
        iv_code    = 'NO_SESSION'
        iv_message = 'No active AMDP session'
      ).
      RETURN.
    ENDIF.

    IF lr_session->context_id IS INITIAL.
      rs_response = error_response(
        iv_id      = is_message-id
        iv_code    = 'NO_CONTEXT'
        iv_message = 'No debuggee context. Wait for breakpoint hit first.'
      ).
      RETURN.
    ENDIF.

    IF lr_session->dbg_control IS NOT BOUND.
      rs_response = error_response(
        iv_id      = is_message-id
        iv_code    = 'NO_CONTROL'
        iv_message = 'Control interface not available. Variable inspection not supported.'
      ).
      RETURN.
    ENDIF.

    " Extract variable names (comma-separated)
    DATA(lv_names_str) = extract_param( iv_params = is_message-params iv_name = 'names' ).

    TRY.
        DATA lt_var_req TYPE if_amdp_dbg_main=>tt_dbg_scalar_value_req.

        IF lv_names_str IS NOT INITIAL.
          SPLIT lv_names_str AT ',' INTO TABLE DATA(lt_names).
          LOOP AT lt_names INTO DATA(lv_name).
            INSERT VALUE #( name = lv_name ) INTO TABLE lt_var_req.
          ENDLOOP.
        ENDIF.

        DATA(lv_req_id) = lr_session->dbg_control->get_scalar_values(
          im_dbg_context_id = lr_session->context_id
          im_scalar_values  = lt_var_req
        ).

        DATA(lv_brace_open) = '{'.
        DATA(lv_brace_close) = '}'.
        rs_response = VALUE #(
          id      = is_message-id
          success = abap_true
          data    = |{ lv_brace_open }"status":"requested","request_id":"{ lv_req_id }","context_id":"{ lr_session->context_id }"{ lv_brace_close }|
        ).

      CATCH cx_root INTO DATA(lx_error).
        rs_response = error_response(
          iv_id      = is_message-id
          iv_code    = 'GET_VARS_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.


  METHOD handle_get_status.
    DATA(lr_session) = get_session( iv_session_id ).

    DATA(lv_brace_open) = '{'.
    DATA(lv_brace_close) = '}'.

    IF lr_session IS NOT BOUND.
      rs_response = VALUE #(
        id      = is_message-id
        success = abap_true
        data    = |{ lv_brace_open }"active":false,"context_id":""{ lv_brace_close }|
      ).
      RETURN.
    ENDIF.

    DATA(lv_active) = COND string( WHEN lr_session->is_active = abap_true THEN 'true' ELSE 'false' ).

    rs_response = VALUE #(
      id      = is_message-id
      success = abap_true
      data    = |{ lv_brace_open }"active":{ lv_active },"context_id":"{ lr_session->context_id }"{ lv_brace_close }|
    ).
  ENDMETHOD.


  METHOD handle_execute_and_debug.
    " Combined action: Start debugger -> Set breakpoint -> Execute AMDP -> Get debug event
    " This runs everything in a single request, avoiding the WebSocket blocking issue.
    " When AMDP hits breakpoint, execution pauses. resume() returns immediately with ON_BREAK.
    DATA: lt_response TYPE if_amdp_dbg_main=>tt_dbg_response.

    DATA(lv_brace_open) = '{'.
    DATA(lv_brace_close) = '}'.

    " Extract parameters
    DATA(lv_class) = extract_param( iv_params = is_message-params iv_name = 'class' ).
    DATA(lv_method) = extract_param( iv_params = is_message-params iv_name = 'method' ).
    DATA(lv_line_str) = extract_param( iv_params = is_message-params iv_name = 'line' ).
    DATA(lv_count_str) = extract_param( iv_params = is_message-params iv_name = 'count' ).
    DATA(lv_cascade) = extract_param( iv_params = is_message-params iv_name = 'cascadeMode' ).

    " Defaults
    IF lv_class IS INITIAL.
      lv_class = 'ZCL_ADT_00_AMDP_TEST'.
    ENDIF.
    IF lv_method IS INITIAL.
      lv_method = 'CALCULATE_SQUARES'.
    ENDIF.
    IF lv_cascade IS INITIAL.
      lv_cascade = 'FULL'.
    ENDIF.

    DATA lv_line TYPE i VALUE 40.
    IF lv_line_str IS NOT INITIAL.
      lv_line = lv_line_str.
    ENDIF.

    DATA lv_count TYPE i VALUE 5.
    IF lv_count_str IS NOT INITIAL.
      lv_count = lv_count_str.
    ENDIF.

    TRY.
        " Step 1: Start debug session (or reuse existing)
        DATA(lr_session) = get_session( iv_session_id ).
        DATA lo_dbg_main TYPE REF TO if_amdp_dbg_main.
        DATA lo_dbg_control TYPE REF TO if_amdp_dbg_control.
        DATA lv_debugger_id TYPE string.

        IF lr_session IS BOUND AND lr_session->is_active = abap_true.
          " Reuse existing session
          lo_dbg_main = lr_session->dbg_main.
          lo_dbg_control = lr_session->dbg_control.
        ELSE.
          " Create new session
          lo_dbg_main = cl_amdp_dbg_main=>create( ).

          lo_dbg_main->start(
            EXPORTING
              im_for_user          = CONV #( sy-uname )
              im_dbg_cascade_mode  = lv_cascade
              im_kill_active_dbg   = abap_true
            IMPORTING
              ex_response          = lt_response
          ).

          " Get debugger_id
          LOOP AT lt_response INTO DATA(ls_start_resp) WHERE kind = 'START'.
            lv_debugger_id = ls_start_resp-values-start-debugger_id.
            EXIT.
          ENDLOOP.

          " Get control interface
          IF lv_debugger_id IS NOT INITIAL.
            TRY.
                lo_dbg_control = cl_amdp_dbg_control=>create( lv_debugger_id ).
              CATCH cx_root.
                CLEAR lo_dbg_control.
            ENDTRY.
          ENDIF.

          " Store session
          DATA ls_session TYPE ty_session.
          ls_session-session_id  = iv_session_id.
          ls_session-dbg_main    = lo_dbg_main.
          ls_session-dbg_control = lo_dbg_control.
          ls_session-is_active   = abap_true.

          IF lr_session IS BOUND.
            lr_session->* = ls_session.
          ELSE.
            INSERT ls_session INTO TABLE gt_sessions.
          ENDIF.
          lr_session = get_session( iv_session_id ).
        ENDIF.

        " Step 2: Set breakpoint
        IF lo_dbg_control IS BOUND.
          DATA lt_breakpoints TYPE if_amdp_dbg_main=>tt_dbg_breakpoint_req.
          DATA ls_bp TYPE if_amdp_dbg_main=>ty_dbg_breakpoint_req.

          ls_bp-client_id = |bp_{ lv_class }_{ lv_line }|.
          ls_bp-bp_kind   = if_amdp_dbg_main=>co_dbg_bp_kind-line.
          ls_bp-enabled   = abap_true.
          ls_bp-abap_position-program_name = to_upper( lv_class ).
          ls_bp-abap_position-include_name = to_upper( lv_class ).
          ls_bp-abap_position-line = lv_line.

          INSERT ls_bp INTO TABLE lt_breakpoints.

          lo_dbg_control->sync_breakpoints( im_breakpoints = lt_breakpoints ).
        ENDIF.

        " Step 3: Execute AMDP (this will pause at breakpoint)
        DATA lt_result TYPE zcl_adt_00_amdp_test=>tt_result.
        DATA lv_execution_error TYPE string.

        IF to_upper( lv_class ) = 'ZCL_ADT_00_AMDP_TEST' AND to_upper( lv_method ) = 'CALCULATE_SQUARES'.
          TRY.
              zcl_adt_00_amdp_test=>calculate_squares(
                EXPORTING iv_count  = lv_count
                IMPORTING et_result = lt_result
              ).
            CATCH cx_root INTO DATA(lx_exec).
              lv_execution_error = lx_exec->get_text( ).
          ENDTRY.
        ELSE.
          rs_response = error_response(
            iv_id      = is_message-id
            iv_code    = 'UNSUPPORTED'
            iv_message = |Only ZCL_ADT_00_AMDP_TEST=>CALCULATE_SQUARES supported|
          ).
          RETURN.
        ENDIF.

        " Step 4: Call resume() to get debug events (should return immediately if breakpoint hit)
        CLEAR lt_response.
        lo_dbg_main->resume(
          IMPORTING
            ex_response = lt_response
        ).

        " Build result JSON
        DATA lv_json TYPE string.
        lv_json = |{ lv_brace_open }"status":"completed"|.
        lv_json = |{ lv_json },"class":"{ lv_class }","method":"{ lv_method }","line":{ lv_line }|.
        lv_json = |{ lv_json },"execution_rows":{ lines( lt_result ) }|.

        IF lv_execution_error IS NOT INITIAL.
          lv_json = |{ lv_json },"execution_error":"{ escape_json( lv_execution_error ) }"|.
        ENDIF.

        " Include debug events
        lv_json = |{ lv_json },"events":[|.

        DATA lv_first TYPE abap_bool VALUE abap_true.
        LOOP AT lt_response INTO DATA(ls_resp).
          IF lv_first = abap_false.
            lv_json = |{ lv_json },|.
          ENDIF.
          lv_first = abap_false.

          CASE ls_resp-kind.
            WHEN 'ON_BREAK'.
              lr_session->context_id = ls_resp-values-on_break-context_id.

              lv_json = |{ lv_json }{ lv_brace_open }"kind":"on_break"|.
              lv_json = |{ lv_json },"context_id":"{ ls_resp-values-on_break-context_id }"|.
              lv_json = |{ lv_json },"bp_client_id":"{ ls_resp-values-on_break-bp_client_id }"|.

              IF ls_resp-values-on_break-abap_position IS NOT INITIAL.
                lv_json = |{ lv_json },"abap_position":{ lv_brace_open }|.
                lv_json = |{ lv_json }"program":"{ ls_resp-values-on_break-abap_position-program_name }"|.
                lv_json = |{ lv_json },"include":"{ ls_resp-values-on_break-abap_position-include_name }"|.
                lv_json = |{ lv_json },"line":{ ls_resp-values-on_break-abap_position-line }{ lv_brace_close }|.
              ENDIF.

              IF ls_resp-values-on_break-native_position IS NOT INITIAL.
                lv_json = |{ lv_json },"native_position":{ lv_brace_open }|.
                lv_json = |{ lv_json }"schema":"{ ls_resp-values-on_break-native_position-schema_name }"|.
                lv_json = |{ lv_json },"name":"{ ls_resp-values-on_break-native_position-name }"|.
                lv_json = |{ lv_json },"line":{ ls_resp-values-on_break-native_position-line }{ lv_brace_close }|.
              ENDIF.

              lv_json = |{ lv_json },"variable_count":{ lines( ls_resp-values-on_break-local_variables ) }|.
              lv_json = |{ lv_json },"stack_depth":{ lines( ls_resp-values-on_break-callstack ) }|.
              lv_json = |{ lv_json }{ lv_brace_close }|.

            WHEN 'ON_EXECUTION_END'.
              lr_session->context_id = ''.
              lv_json = |{ lv_json }{ lv_brace_open }"kind":"on_execution_end"|.
              lv_json = |{ lv_json },"context_id":"{ ls_resp-values-on_execution_end-context_id }"|.
              lv_json = |{ lv_json },"aborted":{ COND #( WHEN ls_resp-values-on_execution_end-execution_aborted = abap_true THEN 'true' ELSE 'false' ) }{ lv_brace_close }|.

            WHEN OTHERS.
              lv_json = |{ lv_json }{ lv_brace_open }"kind":"{ ls_resp-kind }"{ lv_brace_close }|.
          ENDCASE.
        ENDLOOP.

        lv_json = |{ lv_json }]{ lv_brace_close }|.

        rs_response = VALUE #(
          id      = is_message-id
          success = abap_true
          data    = lv_json
        ).

      CATCH cx_root INTO DATA(lx_error).
        rs_response = error_response(
          iv_id      = is_message-id
          iv_code    = 'EXECUTE_DEBUG_FAILED'
          iv_message = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.


  METHOD get_session.
    DATA(lr_session) = REF #( gt_sessions[ session_id = iv_session_id ] OPTIONAL ).
    rs_session = lr_session.
  ENDMETHOD.


  METHOD escape_json.
    rv_escaped = iv_string.
    REPLACE ALL OCCURRENCES OF '\' IN rv_escaped WITH '\\'.
    REPLACE ALL OCCURRENCES OF '"' IN rv_escaped WITH '\"'.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>cr_lf IN rv_escaped WITH '\n'.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>newline IN rv_escaped WITH '\n'.
  ENDMETHOD.


  METHOD error_response.
    DATA(lv_brace_open) = '{'.
    DATA(lv_brace_close) = '}'.
    DATA(lv_escaped_msg) = escape_json( iv_message ).

    rs_response = VALUE #(
      id      = iv_id
      success = abap_false
      error   = |{ lv_brace_open }"code":"{ iv_code }","message":"{ lv_escaped_msg }"{ lv_brace_close }|
    ).
  ENDMETHOD.


  METHOD extract_param.
    " Extract parameter from JSON params string
    " Simple regex-based extraction
    DATA lv_pattern TYPE string.
    lv_pattern = |"{ iv_name }"\\s*:\\s*"([^"]*)"|.

    FIND REGEX lv_pattern IN iv_params SUBMATCHES rv_value.
    IF sy-subrc <> 0.
      " Try numeric value
      lv_pattern = |"{ iv_name }"\\s*:\\s*(\\d+)|.
      FIND REGEX lv_pattern IN iv_params SUBMATCHES rv_value.
    ENDIF.
  ENDMETHOD.

ENDCLASS.
