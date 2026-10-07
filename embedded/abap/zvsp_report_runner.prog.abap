*&---------------------------------------------------------------------*
*& Report ZVSP_REPORT_RUNNER
*&---------------------------------------------------------------------*
*& Background job step for ZCL_VSP_REPORT_SERVICE=>HANDLE_RUN_REPORT.
*& SUBMIT is forbidden in APC, so the report runs here, in a batch work
*& process, with ALV capture. The request (report, variant or selection
*& table, max rows) is handed over in INDX(ZV) under VSPR<jobcount>. The
*& report's list goes to this step's spool; a captured ALV goes back as
*& JSON under VSPA<jobcount>, read by getJobStatus.
*&---------------------------------------------------------------------*
REPORT zvsp_report_runner.

DATA: gv_jobname  TYPE btcjob,
      gv_jobcount TYPE btcjobcnt,
      gv_report   TYPE progname,
      gv_variant  TYPE variant,
      gv_max_rows TYPE i,
      gt_params   TYPE TABLE OF rsparams,
      gt_list     TYPE TABLE OF abaplist,
      gv_key      TYPE indx_srtfd,
      gr_data     TYPE REF TO data,
      gv_json     TYPE string.

FIELD-SYMBOLS <gt_data> TYPE ANY TABLE.

START-OF-SELECTION.
  CALL FUNCTION 'GET_JOB_RUNTIME_INFO'
    IMPORTING
      jobcount        = gv_jobcount
      jobname         = gv_jobname
    EXCEPTIONS
      no_runtime_info = 1
      OTHERS          = 2.
  IF sy-subrc <> 0.
    MESSAGE 'ZVSP_REPORT_RUNNER only runs as a background job step' TYPE 'E'.
  ENDIF.

  gv_key = |VSPR{ gv_jobcount }|.
  IMPORT report = gv_report variant = gv_variant params = gt_params max_rows = gv_max_rows
    FROM DATABASE indx(zv) ID gv_key.
  IF sy-subrc <> 0.
    MESSAGE |No run request for job { gv_jobname }/{ gv_jobcount }| TYPE 'E'.
  ENDIF.
  DELETE FROM DATABASE indx(zv) ID gv_key.
  IF gv_max_rows <= 0.
    gv_max_rows = 1000.
  ENDIF.

  cl_salv_bs_runtime_info=>set( display = abap_false metadata = abap_true data = abap_true ).
  IF gv_variant IS NOT INITIAL.
    SUBMIT (gv_report) USING SELECTION-SET gv_variant EXPORTING LIST TO MEMORY AND RETURN.
  ELSEIF gt_params IS NOT INITIAL.
    SUBMIT (gv_report) WITH SELECTION-TABLE gt_params EXPORTING LIST TO MEMORY AND RETURN.
  ELSE.
    SUBMIT (gv_report) EXPORTING LIST TO MEMORY AND RETURN.
  ENDIF.

  " A list SUBMITted from a job step is not spooled on its own; writing it
  " here puts it in this step's spool, where getSpoolOutput reads it.
  CALL FUNCTION 'LIST_FROM_MEMORY'
    TABLES
      listobject = gt_list
    EXCEPTIONS
      not_found  = 1
      OTHERS     = 2.
  IF sy-subrc = 0.
    CALL FUNCTION 'WRITE_LIST'
      TABLES
        listobject = gt_list
      EXCEPTIONS
        OTHERS     = 1.
    CALL FUNCTION 'LIST_FREE_MEMORY'.
  ENDIF.

  TRY.
      cl_salv_bs_runtime_info=>get_data_ref( IMPORTING r_data = gr_data ).
    CATCH cx_salv_bs_sc_runtime_info.
      " Not an ALV report: its list output is in the job spool.
  ENDTRY.
  cl_salv_bs_runtime_info=>clear_all( ).

  IF gr_data IS BOUND.
    ASSIGN gr_data->* TO <gt_data>.
    DATA(go_table) = CAST cl_abap_tabledescr( cl_abap_typedescr=>describe_by_data( <gt_data> ) ).
    DATA(go_line) = CAST cl_abap_structdescr( go_table->get_table_line_type( ) ).

    DATA lt_columns TYPE string_table.
    LOOP AT go_line->components INTO DATA(gs_comp).
      APPEND zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
        ( zcl_vsp_utils=>json_str( iv_key = 'name' iv_value = CONV #( gs_comp-name ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'type' iv_value = CONV #( gs_comp-type_kind ) ) )
      ) ) ) TO lt_columns.
    ENDLOOP.

    DATA lt_rows TYPE string_table.
    DATA lt_fields TYPE string_table.
    DATA lv_value TYPE string.
    LOOP AT <gt_data> ASSIGNING FIELD-SYMBOL(<gs_row>).
      IF lines( lt_rows ) >= gv_max_rows.
        EXIT.
      ENDIF.
      CLEAR lt_fields.
      LOOP AT go_line->components INTO gs_comp.
        CASE gs_comp-type_kind.
          WHEN cl_abap_typedescr=>typekind_table   OR cl_abap_typedescr=>typekind_struct1
            OR cl_abap_typedescr=>typekind_struct2 OR cl_abap_typedescr=>typekind_oref
            OR cl_abap_typedescr=>typekind_dref.
            " ALV colour and style columns are tables. A string template on a
            " table, structure or reference ends the job with
            " STRG_ILLEGAL_DATA_TYPE, which CATCH cannot stop.
            APPEND |"{ gs_comp-name }":null| TO lt_fields.
            CONTINUE.
        ENDCASE.
        ASSIGN COMPONENT gs_comp-name OF STRUCTURE <gs_row> TO FIELD-SYMBOL(<gv_field>).
        CHECK sy-subrc = 0.
        TRY.
            lv_value = |{ <gv_field> }|.
          CATCH cx_root.
            CLEAR lv_value.
        ENDTRY.
        APPEND zcl_vsp_utils=>json_str( iv_key = CONV #( gs_comp-name ) iv_value = lv_value ) TO lt_fields.
      ENDLOOP.
      APPEND zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( lt_fields ) ) TO lt_rows.
    ENDLOOP.

    gv_json = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( |"columns":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_columns ) ) }| )
      ( |"rows":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_rows ) ) }| )
      ( zcl_vsp_utils=>json_int( iv_key = 'total_rows' iv_value = lines( <gt_data> ) ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'truncated' iv_value = xsdbool( lines( <gt_data> ) > gv_max_rows ) ) )
    ) ) ).

    gv_key = |VSPA{ gv_jobcount }|.
    EXPORT alv = gv_json TO DATABASE indx(zv) ID gv_key.
  ENDIF.
