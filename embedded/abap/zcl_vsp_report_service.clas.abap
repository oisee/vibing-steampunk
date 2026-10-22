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

    METHODS handle_get_job_status
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_get_spool_output
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

ENDCLASS.


CLASS zcl_vsp_report_service IMPLEMENTATION.

  METHOD zif_vsp_service~get_domain.
    rv_domain = 'report'.
  ENDMETHOD.

  METHOD zif_vsp_service~handle_message.
    CASE is_message-action.
      WHEN 'runReport'.
        rs_response = handle_run_report( is_message ).
      WHEN 'getJobStatus'.
        rs_response = handle_get_job_status( is_message ).
      WHEN 'getSpoolOutput'.
        rs_response = handle_get_spool_output( is_message ).
      WHEN 'getTextElements'.
        rs_response = handle_get_text_elements( is_message ).
      WHEN 'setTextElements'.
        rs_response = handle_set_text_elements( is_message ).
      WHEN 'getVariants'.
        rs_response = handle_get_variants( is_message ).
      WHEN OTHERS.
        rs_response = zcl_vsp_utils=>build_error(
          iv_id      = is_message-id
          iv_code    = 'UNKNOWN_ACTION'
          iv_message = |Action '{ is_message-action }' not supported|
        ).
    ENDCASE.
  ENDMETHOD.

  METHOD zif_vsp_service~on_disconnect.
  ENDMETHOD.

  METHOD handle_run_report.
    " Background Job approach - works in APC/ICF context
    " Uses XBP BAPIs to schedule immediate job execution
    DATA: lv_report    TYPE progname,
          lv_variant   TYPE variant,
          lv_jobname   TYPE btcjob,
          lv_jobcount  TYPE btcjobcnt,
          lv_extuser   TYPE bapixmlogr-extuser,
          lt_rsparams  TYPE TABLE OF rsparams,
          lv_sessionid TYPE xmisessnid.

    DATA(lv_report_str) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'report' ).
    DATA(lv_variant_str) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'variant' ).
    DATA(lv_params_json) = zcl_vsp_utils=>extract_param_object( iv_params = is_message-params iv_name = 'params' ).
    DATA(lv_max_rows) = zcl_vsp_utils=>extract_param_int( iv_params = is_message-params iv_name = 'max_rows' ).

    IF lv_report_str IS INITIAL.
      rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter report is required' ).
      RETURN.
    ENDIF.

    TRANSLATE lv_report_str TO UPPER CASE.
    lv_report = lv_report_str.

    IF lv_variant_str IS NOT INITIAL.
      TRANSLATE lv_variant_str TO UPPER CASE.
      lv_variant = lv_variant_str.
    ENDIF.

    " Step 0: Logon to XMI interface (required for XBP BAPIs)
    DATA ls_xmi_return TYPE bapiret2.
    TRY.
        CALL FUNCTION 'BAPI_XMI_LOGON'
          EXPORTING
            extcompany = 'VSP'
            extproduct = 'VSP'
            interface  = 'XBP'
            version    = '2.0'
          IMPORTING
            sessionid  = lv_sessionid
            return     = ls_xmi_return.
      CATCH cx_sy_dyn_call_illegal_type INTO DATA(lx_xmi).
        rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'XMI_TYPE_ERROR' iv_message = |XMI_LOGON: { lx_xmi->get_text( ) }| ).
        RETURN.
    ENDTRY.

    " Continue even if already logged on - XMI logon errors are non-fatal
    " The "already logged on" error is normal when reusing APC session

    " Verify report exists
    SELECT SINGLE name FROM trdir INTO @DATA(lv_exists) WHERE name = @lv_report.
    IF sy-subrc <> 0.
      rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'REPORT_NOT_FOUND' iv_message = |Report { lv_report } not found| ).
      RETURN.
    ENDIF.

    " Parse params JSON if provided
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

    " Generate unique job name
    DATA(lv_timestamp) = |{ sy-datum }{ sy-uzeit }|.
    lv_jobname = |VSP_{ lv_timestamp(12) }|.
    lv_extuser = sy-uname.

    " Step 1: Open job
    DATA ls_ret TYPE bapiret2.
    TRY.
        CALL FUNCTION 'BAPI_XBP_JOB_OPEN'
          EXPORTING
            jobname            = lv_jobname
            external_user_name = lv_extuser
          IMPORTING
            jobcount           = lv_jobcount
            return             = ls_ret.
      CATCH cx_sy_dyn_call_illegal_type INTO DATA(lx_open).
        rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'JOB_OPEN_TYPE_ERROR' iv_message = |JOB_OPEN: { lx_open->get_text( ) }| ).
        RETURN.
    ENDTRY.

    IF ls_ret-type = 'E'.
      rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'JOB_OPEN_ERROR' iv_message = |{ ls_ret-message }| ).
      RETURN.
    ENDIF.

    " Step 2: Add ABAP step. SUBMIT is forbidden in APC, so the step is
    " ZVSP_REPORT_RUNNER, which SUBMITs the report in the batch work
    " process with ALV capture and reads its request from INDX.
    DATA lv_key TYPE indx_srtfd.
    lv_key = |VSPR{ lv_jobcount }|.
    EXPORT report = lv_report variant = lv_variant params = lt_rsparams max_rows = lv_max_rows
      TO DATABASE indx(zv) ID lv_key.

    CLEAR ls_ret.
    CALL FUNCTION 'BAPI_XBP_JOB_ADD_ABAP_STEP'
      EXPORTING
        jobname            = lv_jobname
        jobcount           = lv_jobcount
        external_user_name = lv_extuser
        abap_program_name  = 'ZVSP_REPORT_RUNNER'
      IMPORTING
        return             = ls_ret.

    IF ls_ret-type = 'E'.
      rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'JOB_STEP_ERROR' iv_message = |{ ls_ret-message }| ).
      RETURN.
    ENDIF.

    " Step 3: Close job
    CLEAR ls_ret.
    CALL FUNCTION 'BAPI_XBP_JOB_CLOSE'
      EXPORTING
        jobname            = lv_jobname
        jobcount           = lv_jobcount
        external_user_name = lv_extuser
      IMPORTING
        return             = ls_ret.

    IF ls_ret-type = 'E'.
      rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'JOB_CLOSE_ERROR' iv_message = |{ ls_ret-message }| ).
      RETURN.
    ENDIF.

    " Step 4: Start job immediately (target_server is required but can be blank for default)
    CLEAR ls_ret.
    CALL FUNCTION 'BAPI_XBP_JOB_START_IMMEDIATELY'
      EXPORTING
        jobname            = lv_jobname
        jobcount           = lv_jobcount
        external_user_name = lv_extuser
        target_server      = space
      IMPORTING
        return             = ls_ret.

    IF ls_ret-type = 'E'.
      rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'JOB_START_ERROR' iv_message = |{ ls_ret-message }| ).
      RETURN.
    ENDIF.

    " Success - return job info for polling
    DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'status' iv_value = 'scheduled' ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'report' iv_value = CONV #( lv_report ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'jobname' iv_value = CONV #( lv_jobname ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'jobcount' iv_value = CONV #( lv_jobcount ) ) )
    ) ) ).

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).
  ENDMETHOD.

  METHOD handle_get_job_status.
    " Check job status using XBP BAPI
    DATA: lv_jobname   TYPE btcjob,
          lv_jobcount  TYPE btcjobcnt,
          lv_extuser   TYPE bapixmlogr-extuser,
          lv_status    TYPE btcstatus,
          ls_return    TYPE bapiret2,
          lv_sessionid TYPE xmisessnid.

    DATA(lv_jobname_str) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'jobname' ).
    DATA(lv_jobcount_str) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'jobcount' ).

    IF lv_jobname_str IS INITIAL OR lv_jobcount_str IS INITIAL.
      rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameters jobname and jobcount are required' ).
      RETURN.
    ENDIF.

    " Logon to XMI interface (required for XBP BAPIs)
    DATA ls_xmi_return TYPE bapiret2.
    CALL FUNCTION 'BAPI_XMI_LOGON'
      EXPORTING
        extcompany = 'VSP'
        extproduct = 'VSP'
        interface  = 'XBP'
        version    = '2.0'
      IMPORTING
        sessionid  = lv_sessionid
        return     = ls_xmi_return.

    " Continue even if already logged on - XMI logon errors are non-fatal
    " The "already logged on" error is normal when reusing APC session

    lv_jobname = lv_jobname_str.
    lv_jobcount = lv_jobcount_str.
    lv_extuser = sy-uname.

    " Get job status
    CALL FUNCTION 'BAPI_XBP_JOB_STATUS_GET'
      EXPORTING
        jobname            = lv_jobname
        jobcount           = lv_jobcount
        external_user_name = lv_extuser
      IMPORTING
        status             = lv_status
        return             = ls_return.

    IF ls_return-type = 'E'.
      rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'JOB_STATUS_ERROR' iv_message = |{ ls_return-message }| ).
      RETURN.
    ENDIF.

    " Map status codes to readable status
    DATA lv_status_text TYPE string.
    CASE lv_status.
      WHEN 'S'. lv_status_text = 'scheduled'.
      WHEN 'R'. lv_status_text = 'running'.
      WHEN 'F'. lv_status_text = 'finished'.
      WHEN 'A'. lv_status_text = 'aborted'.
      WHEN 'Y'. lv_status_text = 'ready'.
      WHEN 'P'. lv_status_text = 'released'.
      WHEN OTHERS. lv_status_text = lv_status.
    ENDCASE.

    " Get spool list info if job finished
    DATA lt_spool_ids TYPE string_table.

    IF lv_status = 'F' OR lv_status = 'A'.
      " Get spool list from job steps table (TBTCP has LISTIDENT field)
      DATA lt_spool TYPE TABLE OF tbtcp.
      SELECT * FROM tbtcp INTO TABLE lt_spool
        WHERE jobname = lv_jobname
          AND jobcount = lv_jobcount
          AND listident <> ''.

      LOOP AT lt_spool INTO DATA(ls_spool).
        APPEND |"{ ls_spool-listident }"| TO lt_spool_ids.
      ENDLOOP.

      " ALV captured by ZVSP_REPORT_RUNNER, if the report had one
      DATA lv_alv TYPE string.
      DATA lv_alv_key TYPE indx_srtfd.
      lv_alv_key = |VSPA{ lv_jobcount }|.
      IMPORT alv = lv_alv FROM DATABASE indx(zv) ID lv_alv_key.
      IF sy-subrc = 0.
        DELETE FROM DATABASE indx(zv) ID lv_alv_key.
      ENDIF.
    ENDIF.

    DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'jobname' iv_value = CONV #( lv_jobname ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'jobcount' iv_value = CONV #( lv_jobcount ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'status' iv_value = lv_status_text ) )
      ( |"spool_ids":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_spool_ids ) ) }| )
      ( COND #( WHEN lv_alv IS NOT INITIAL THEN |"alv":{ lv_alv }| ) )
    ) ) ).

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).
  ENDMETHOD.

  METHOD handle_get_spool_output.
    " Retrieve spool output by ID using TemSe API (APC-safe)
    " RSPO_RETURN_SPOOLJOB uses SUBMIT which is forbidden in APC context
    TYPES: BEGIN OF ty_line,
             tdline TYPE c LENGTH 255,
           END OF ty_line.
    DATA: lv_spool_id TYPE rspoid,
          lt_data     TYPE STANDARD TABLE OF ty_line,
          lv_alldata  TYPE c.

    DATA(lv_spool_str) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'spool_id' ).

    IF lv_spool_str IS INITIAL.
      rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter spool_id is required' ).
      RETURN.
    ENDIF.

    lv_spool_id = lv_spool_str.

    " Get spool request info from TSP01
    SELECT SINGLE rqo1name, rqo1clie FROM tsp01 INTO @DATA(ls_spool)
      WHERE rqident = @lv_spool_id.
    IF sy-subrc <> 0.
      rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'SPOOL_NOT_FOUND'
        iv_message = |Spool { lv_spool_id } not found| ).
      RETURN.
    ENDIF.

    " Open TemSe object for reading line-by-line
    " RSTS_OPEN_RL stores handle internally when own_fbhandle=' ' (default)
    TRY.
        CALL FUNCTION 'RSTS_OPEN_RL'
          EXPORTING
            name      = ls_spool-rqo1name
            client    = ls_spool-rqo1clie
          EXCEPTIONS
            fb_error  = 1
            fb_rsts_other = 2
            no_object = 3
            no_permission = 4
            OTHERS    = 5.
      CATCH cx_sy_dyn_call_param_missing INTO DATA(lx_open).
        rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'OPEN_PARAM_MISSING'
          iv_message = |RSTS_OPEN_RL param missing: { lx_open->get_text( ) }| ).
        RETURN.
    ENDTRY.

    IF sy-subrc <> 0.
      rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'TEMSE_OPEN_ERROR'
        iv_message = |Failed to open TemSe for spool { lv_spool_id }: RC={ sy-subrc }| ).
      RETURN.
    ENDIF.

    " Read all TemSe content into table (uses internal handle)
    DATA(lv_read_rc) = 0.
    TRY.
        CALL FUNCTION 'RSTS_READ'
          IMPORTING
            alldata   = lv_alldata
          TABLES
            datatab   = lt_data
          EXCEPTIONS
            fb_error  = 1
            fb_rsts_other = 2
            OTHERS    = 3.
        lv_read_rc = sy-subrc.
      CATCH cx_sy_dyn_call_param_missing INTO DATA(lx_read).
        CALL FUNCTION 'RSTS_CLOSE' EXCEPTIONS OTHERS = 0.
        rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'READ_PARAM_MISSING'
          iv_message = |RSTS_READ param missing: { lx_read->get_text( ) }| ).
        RETURN.
    ENDTRY.

    " Close TemSe object (uses internal handle)
    CALL FUNCTION 'RSTS_CLOSE'
      EXCEPTIONS
        OTHERS = 0.

    IF lv_read_rc <> 0.
      rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'TEMSE_READ_ERROR'
        iv_message = |Failed to read TemSe for spool { lv_spool_id }: RC={ lv_read_rc }| ).
      RETURN.
    ENDIF.

    " Build output string from lines
    DATA lv_output TYPE string.
    DATA lv_line_count TYPE i.
    LOOP AT lt_data INTO DATA(ls_line).
      IF lv_output IS NOT INITIAL.
        lv_output = |{ lv_output }\n|.
      ENDIF.
      lv_output = |{ lv_output }{ ls_line-tdline }|.
      lv_line_count = lv_line_count + 1.
    ENDLOOP.

    IF lv_line_count = 0.
      lv_output = '[Spool exists but contains no readable data]'.
      lv_line_count = 1.
    ENDIF.

    DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'spool_id' iv_value = CONV #( lv_spool_id ) ) )
      ( zcl_vsp_utils=>json_int( iv_key = 'lines' iv_value = lv_line_count ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'output' iv_value = lv_output ) )
    ) ) ).

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).
  ENDMETHOD.

  METHOD handle_get_text_elements.
    DATA: lt_textpool TYPE TABLE OF textpool,
          lv_program  TYPE progname.

    DATA(lv_prog_str) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'program' ).
    DATA(lv_language) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'language' ).

    IF lv_prog_str IS INITIAL.
      rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter program is required' ).
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

    " Build selection_texts object
    DATA lt_sel_texts TYPE string_table.
    DATA lv_entry_str TYPE string.
    DATA lv_key TYPE string.
    LOOP AT lt_textpool INTO DATA(ls_text) WHERE id = 'S'.
      lv_key = ls_text-key.
      CONDENSE lv_key.
      " Selection text entry has 8-char key prefix - strip it
      lv_entry_str = ls_text-entry.
      IF strlen( lv_entry_str ) > 8.
        lv_entry_str = lv_entry_str+8.
      ENDIF.
      APPEND zcl_vsp_utils=>json_str( iv_key = lv_key iv_value = lv_entry_str ) TO lt_sel_texts.
    ENDLOOP.

    " Build text_symbols object
    DATA lt_sym_texts TYPE string_table.
    LOOP AT lt_textpool INTO ls_text WHERE id = 'I'.
      lv_key = ls_text-key.
      CONDENSE lv_key.
      APPEND zcl_vsp_utils=>json_str( iv_key = lv_key iv_value = CONV #( ls_text-entry ) ) TO lt_sym_texts.
    ENDLOOP.

    DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'program' iv_value = CONV #( lv_program ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'language' iv_value = CONV #( lv_lang ) ) )
      ( |"selection_texts":{ zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( lt_sel_texts ) ) }| )
      ( |"text_symbols":{ zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( lt_sym_texts ) ) }| )
      ( |"heading_texts":{ zcl_vsp_utils=>json_obj( '' ) }| )
    ) ) ).

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).
  ENDMETHOD.

  METHOD handle_set_text_elements.
    DATA: lt_textpool TYPE TABLE OF textpool,
          lv_program  TYPE progname.

    DATA(lv_prog_str) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'program' ).
    DATA(lv_language) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'language' ).
    DATA(lv_sel_json) = zcl_vsp_utils=>extract_param_object( iv_params = is_message-params iv_name = 'selection_texts' ).
    DATA(lv_sym_json) = zcl_vsp_utils=>extract_param_object( iv_params = is_message-params iv_name = 'text_symbols' ).
    DATA(lv_head_json) = zcl_vsp_utils=>extract_param_object( iv_params = is_message-params iv_name = 'heading_texts' ).

    IF lv_prog_str IS INITIAL.
      rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter program is required' ).
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
    DATA lv_head_count TYPE i.

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

    IF lv_head_json IS NOT INITIAL.
      lv_work = lv_head_json.
      WHILE lv_work CS '"'.
        CLEAR: lv_key, lv_val.
        FIND PCRE '"([^"]+)"\s*:\s*"([^"]*)"' IN lv_work SUBMATCHES lv_key lv_val.
        IF sy-subrc = 0.
          REPLACE ALL OCCURRENCES OF '\"' IN lv_val WITH '"'.
          REPLACE ALL OCCURRENCES OF '\\' IN lv_val WITH '\'.

          lv_textkey = lv_key.
          READ TABLE lt_textpool ASSIGNING <fs> WITH KEY id = 'H' key = lv_textkey.
          IF sy-subrc = 0.
            <fs>-entry = lv_val.
          ELSE.
            APPEND VALUE textpool( id = 'H' key = lv_textkey entry = lv_val ) TO lt_textpool.
          ENDIF.
          lv_head_count = lv_head_count + 1.

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

    DATA(lv_status) = COND string( WHEN sy-subrc = 0 THEN 'success' ELSE 'error' ).
    DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'status' iv_value = lv_status ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'program' iv_value = CONV #( lv_program ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'language' iv_value = CONV #( lv_lang ) ) )
      ( zcl_vsp_utils=>json_int( iv_key = 'selection_texts_set' iv_value = lv_sel_count ) )
      ( zcl_vsp_utils=>json_int( iv_key = 'text_symbols_set' iv_value = lv_sym_count ) )
      ( zcl_vsp_utils=>json_int( iv_key = 'heading_texts_set' iv_value = lv_head_count ) )
    ) ) ).

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).
  ENDMETHOD.

  METHOD handle_get_variants.
    DATA: lt_varid   TYPE TABLE OF varid,
          lv_report  TYPE progname.

    DATA(lv_report_str) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'report' ).

    IF lv_report_str IS INITIAL.
      rs_response = zcl_vsp_utils=>build_error( iv_id = is_message-id iv_code = 'MISSING_PARAM' iv_message = 'Parameter report is required' ).
      RETURN.
    ENDIF.

    TRANSLATE lv_report_str TO UPPER CASE.
    lv_report = lv_report_str.

    SELECT * FROM varid INTO TABLE lt_varid
      WHERE report = lv_report
      ORDER BY variant.

    " Build variants array
    DATA lt_variants TYPE string_table.
    LOOP AT lt_varid INTO DATA(ls_var).
      APPEND zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
        ( zcl_vsp_utils=>json_str( iv_key = 'name' iv_value = CONV #( ls_var-variant ) ) )
        ( zcl_vsp_utils=>json_bool( iv_key = 'protected' iv_value = ls_var-protected ) )
      ) ) ) TO lt_variants.
    ENDLOOP.

    DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'report' iv_value = CONV #( lv_report ) ) )
      ( |"variants":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_variants ) ) }| )
    ) ) ).

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = lv_data ).
  ENDMETHOD.

ENDCLASS.
