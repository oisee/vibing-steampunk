"! <p class="shorttext synchronized">VSP Transport Upload Service</p>
"! Domain "transport": writes a released request's cofile K&lt;nr&gt;.&lt;SID&gt;
"! and data file R&lt;nr&gt;.&lt;SID&gt; into DIR_TRANS of this system and adds
"! the request to this system's import buffer. It never imports: the import
"! stays a human step in STMS.
"!
"! Rules, each pinned by a Go test over this source
"! (embedded/abap/transport_service_test.go):
"! - files are written only through EPS_OPEN_OUTPUT_FILE, EPS_WRITE_BLOCK and
"!   EPS_CLOSE_FILE, into the logical directories $TR_COFI and $TR_DATA; the
"!   caller names a file, never a directory or a path;
"! - a file that exists is never overwritten, a zero-byte one included, and a
"!   failure deletes only what this call wrote;
"! - tp is given one command, the literal ADDTOBUFFER, for sy-sysid, through
"!   TMS_TP_MAINTAIN_BUFFER, and only for the request this session uploaded;
"!   the job reads the buffer through TMS_TP_SHOW_BUFFER before it adds;
"! - show_buffer reads the buffer file DIR_TRANS/buffer/&lt;SID&gt; and nothing more;
"! - there is no dynamic CALL FUNCTION.
"! tp is started over synchronous RFC, which an ABAP Push Channel may not do
"! (APC_ILLEGAL_STATEMENT), so the add runs as a background job,
"! ZVSP_TRANSPORT_BUFFER, which calls run_job. The request and the SHA-256 of
"! the two files are bound to the job step as parameters when it is
"! scheduled (SUBMIT ... VIA JOB); the job checks the files still have them
"! before it adds, and writes its outcome to its job log. The upload answers
"! "pending" with the job's number, and add_status reads the outcome: job
"! status (TBTCO), job log, and -- the only proof of "queued" -- the buffer
"! file.
"! Authority is SAP's own: S_CTS_ADMI EPS1 for the files (EPS layer), TADD
"! for the buffer (tp interface). A refusal is reported, not worked around.
CLASS zcl_vsp_transport_service DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    INTERFACES zif_vsp_service.

    "! Both files together, in bytes (50 MB).
    CONSTANTS c_max_total TYPE i VALUE 52428800.
    "! The cofile alone, in bytes.
    CONSTANTS c_max_cofile TYPE i VALUE 1048576.
    "! One decoded chunk, in bytes.
    CONSTANTS c_max_chunk TYPE i VALUE 1048576.
    "! Buffer lines returned by show_buffer.
    CONSTANTS c_max_buffer_entries TYPE i VALUE 500.
    "! The background job (and its program) that runs the tp step.
    CONSTANTS c_job_name TYPE tbtcjob-jobname VALUE 'ZVSP_TRANSPORT_BUFFER'.
    "! AMC application and channel the job publishes its outcome on; the
    "! extension is the uploading WebSocket session's id.
    CONSTANTS c_amc_app TYPE amc_application_id VALUE 'ZVSP_TRANSPORT'.
    CONSTANTS c_amc_channel TYPE string VALUE `/buffer`.

    "! The background step: checks the request and that both files still have
    "! the SHA-256 the upload wrote, reads the buffer and adds the request;
    "! writes the outcome to the job log. Its parameters are bound to the job
    "! when it is scheduled. Outside a ZVSP_TRANSPORT_BUFFER job it does nothing.
    CLASS-METHODS run_job
      IMPORTING iv_request    TYPE csequence
                iv_cofile_sha TYPE csequence
                iv_data_sha   TYPE csequence
                iv_push_id    TYPE csequence OPTIONAL.

    "! The request two file names belong to: ev_request is &lt;SID&gt;K&lt;nr&gt;.
    "! ev_error is set, and the rest initial, when they are not a pair.
    CLASS-METHODS request_from_names
      IMPORTING iv_cofile_name TYPE string
                iv_data_name   TYPE string
      EXPORTING ev_request     TYPE trkorr
                ev_sid         TYPE string
                ev_error       TYPE string.

    "! Initial when iv_content has the shape of a cofile of a request exported
    "! from iv_sid (the way STRF_READ_COFILE reads one); otherwise why not.
    CLASS-METHODS validate_cofile
      IMPORTING iv_content      TYPE xstring
                iv_sid          TYPE string
      RETURNING VALUE(rv_error) TYPE string.

  PRIVATE SECTION.
    TYPES:
      BEGIN OF ty_assembly,
        id          TYPE string,
        request     TYPE trkorr,
        sid         TYPE string,
        cofile_name TYPE string,
        data_name   TYPE string,
        cofile_size TYPE i,
        data_size   TYPE i,
        cofile_sha  TYPE string,
        data_sha    TYPE string,
        cofile      TYPE xstring,
        data        TYPE xstring,
      END OF ty_assembly,
      BEGIN OF ty_written,
        request     TYPE trkorr,
        cofile_name TYPE string,
        data_name   TYPE string,
        cofile_sha  TYPE string,
        data_sha    TYPE string,
      END OF ty_written,
      BEGIN OF ty_job_result,
        request      TYPE string,
        system       TYPE string,
        job          TYPE string,
        "! queued (the buffer shows it), not_added (certain), or unknown.
        outcome      TYPE string,
        code         TYPE string,
        message      TYPE string,
        tp_cmd       TYPE string,
        tp_rc        TYPE string,
        tp_msg       TYPE string,
        buffer_known TYPE abap_bool,
        in_buffer    TYPE abap_bool,
        rolled_back  TYPE abap_bool,
      END OF ty_job_result,
      BEGIN OF ty_buffer_line,
        trkorr     TYPE string,
        trfunction TYPE string,
        owner      TYPE string,
        raw        TYPE string,
      END OF ty_buffer_line,
      tt_buffer_line TYPE STANDARD TABLE OF ty_buffer_line WITH DEFAULT KEY,
      tt_tpbuffer TYPE STANDARD TABLE OF tpbuffer WITH DEFAULT KEY,
      tt_tpstdout TYPE STANDARD TABLE OF tpstdout WITH DEFAULT KEY.

    "! The upload being assembled in this session; one at a time.
    DATA ms_assembly TYPE ty_assembly.
    "! The files the last commit of this session wrote, until they are added
    "! to the buffer. Only this request may be added, and only these files
    "! are deleted when the add fails.
    DATA ms_written TYPE ty_written.

    METHODS handle_upload_files
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS upload_begin
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS upload_chunk
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS upload_commit
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    "! Discards the upload in progress -- only the one the caller names.
    METHODS upload_abort
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_add_to_buffer
      IMPORTING iv_session_id      TYPE string
                is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_show_buffer
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_download_files
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    "! The outcome of an add: queued, pending, job_failed or unknown. Read-only.
    METHODS handle_add_status
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    "! Deletes the files of an upload that was committed but never handed to
    "! a buffer job (abandoned: the session ended, or a new upload began).
    METHODS rollback_written
      EXPORTING ev_rolled_back TYPE abap_bool
                ev_message     TYPE string.

    "! The buffer file DIR_TRANS/buffer/&lt;SID&gt;, read through EPS. ev_exists is
    "! false only when a directory listing shows it absent.
    CLASS-METHODS read_buffer_file
      EXPORTING ev_exists TYPE abap_bool
                ev_text   TYPE string
                ev_error  TYPE string.

    "! The requests in the buffer file's text, '#' lines skipped; with
    "! iv_request, that request's lines only.
    CLASS-METHODS buffer_lines
      IMPORTING iv_text         TYPE string
                iv_request      TYPE string OPTIONAL
      RETURNING VALUE(rt_lines) TYPE tt_buffer_line.

    "! Writes the job's outcome to its job log, where add_status reads it.
    CLASS-METHODS job_log
      IMPORTING is_result TYPE ty_job_result.

    "! Publishes the job's outcome on AMC ZVSP_TRANSPORT /buffer, extension
    "! iv_push_id: the uploading WebSocket gets it as a push message.
    CLASS-METHODS publish
      IMPORTING is_result  TYPE ty_job_result
                iv_push_id TYPE csequence.

    "! Schedules ZVSP_TRANSPORT_BUFFER for one request, its step parameters
    "! being the request and the SHA-256 of its two files.
    CLASS-METHODS start_job
      IMPORTING iv_request    TYPE string
                iv_cofile_sha TYPE string
                iv_data_sha   TYPE string
                iv_push_id    TYPE string OPTIONAL
      EXPORTING ev_jobcount   TYPE string
                ev_error      TYPE string.

    "! Reads a whole cofile or data file through EPS's read checks.
    CLASS-METHODS read_dir_file
      IMPORTING iv_kind    TYPE string
                iv_name    TYPE string
      EXPORTING ev_content TYPE xstring
                ev_error   TYPE string.

    "! The logical directory of a kind of file: $TR_COFI or $TR_DATA.
    CLASS-METHODS dir_of
      IMPORTING iv_kind       TYPE string
      RETURNING VALUE(rv_dir) TYPE epsf-epsdirnam.

    "! Whether a file of that kind and name exists in DIR_TRANS, at any size.
    CLASS-METHODS file_exists
      IMPORTING iv_kind          TYPE string
                iv_name          TYPE string
      EXPORTING ev_error         TYPE string
      RETURNING VALUE(rv_exists) TYPE abap_bool.

    "! Whether a file exists in a DIR_TRANS subdirectory. A failure to read
    "! the directory or the file's attributes is an error, never absence:
    "! absence is concluded only from a directory listing without it.
    CLASS-METHODS probe_file
      IMPORTING iv_subdir        TYPE epsf-epssubdir
                iv_name          TYPE string
      EXPORTING ev_error         TYPE string
                ev_long_dir      TYPE eps2path
      RETURNING VALUE(rv_exists) TYPE abap_bool.

    "! Writes a new file through the EPS layer. ev_opened says a file was
    "! created, so that a failure after it knows what to delete.
    CLASS-METHODS write_file
      IMPORTING iv_kind    TYPE string
                iv_name    TYPE string
                iv_content TYPE xstring
      EXPORTING ev_opened  TYPE abap_bool
                ev_path    TYPE string
                ev_error   TYPE string.

    "! Deletes a file this service wrote. With iv_sha, only when the file
    "! still has that SHA-256 (it is the one written). Returns why not, or
    "! initial when the file is gone.
    CLASS-METHODS delete_file
      IMPORTING iv_kind         TYPE string
                iv_name         TYPE string
                iv_sha          TYPE string OPTIONAL
      RETURNING VALUE(rv_error) TYPE string.

    "! Locks the request (lock object E_TRKORR) for this session or job, so
    "! that two uploads or adds of one request cannot interleave.
    CLASS-METHODS lock_request
      IMPORTING iv_request      TYPE csequence
                iv_wait         TYPE abap_bool DEFAULT abap_false
      RETURNING VALUE(rv_error) TYPE string.

    CLASS-METHODS unlock_request
      IMPORTING iv_request TYPE csequence.

    "! The no-overwrite check and the two writes of a commit, under the
    "! request's lock. ev_code initial: both files are written.
    CLASS-METHODS write_pair
      IMPORTING is_up          TYPE ty_assembly
      EXPORTING ev_code        TYPE string
                ev_message     TYPE string
                ev_cofile_path TYPE string
                ev_data_path   TYPE string.

    "! This system's import buffer, read-only.
    CLASS-METHODS read_buffer
      EXPORTING et_buffer TYPE tt_tpbuffer
                et_stdout TYPE tt_tpstdout
                ev_cmd    TYPE string
                ev_rc     TYPE string
                ev_msg    TYPE string
                ev_error  TYPE string.

    CLASS-METHODS request_parts
      IMPORTING iv_request TYPE string
      EXPORTING ev_sid     TYPE string
                ev_number  TYPE string
      RETURNING VALUE(rv_ok) TYPE abap_bool.

    CLASS-METHODS sha256
      IMPORTING iv_data        TYPE xstring
      RETURNING VALUE(rv_hash) TYPE string.

    "! A SHA-256 in hex, as base64 (44 characters).
    CLASS-METHODS sha_b64
      IMPORTING iv_hex        TYPE csequence
      RETURNING VALUE(rv_b64) TYPE string.

    CLASS-METHODS last_message
      RETURNING VALUE(rv_text) TYPE string.

    CLASS-METHODS stdout_json
      IMPORTING it_stdout      TYPE tt_tpstdout
      RETURNING VALUE(rv_json) TYPE string.

    CLASS-METHODS err
      IMPORTING iv_id              TYPE string
                iv_code            TYPE string
                iv_message         TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

ENDCLASS.


CLASS zcl_vsp_transport_service IMPLEMENTATION.

  METHOD zif_vsp_service~get_domain.
    rv_domain = 'transport'.
  ENDMETHOD.


  METHOD zif_vsp_service~handle_message.
    CASE is_message-action.
      WHEN 'upload_files'.
        rs_response = handle_upload_files( is_message ).
      WHEN 'add_to_buffer'.
        rs_response = handle_add_to_buffer( iv_session_id = iv_session_id is_message = is_message ).
      WHEN 'show_buffer'.
        rs_response = handle_show_buffer( is_message ).
      WHEN 'download_files'.
        rs_response = handle_download_files( is_message ).
      WHEN 'add_status'.
        rs_response = handle_add_status( is_message ).
      WHEN OTHERS.
        rs_response = err( iv_id = is_message-id iv_code = 'UNKNOWN_ACTION'
                           iv_message = |Action '{ is_message-action }' not supported| ).
    ENDCASE.
  ENDMETHOD.


  METHOD zif_vsp_service~on_disconnect.
    " An upload committed but never handed to a buffer job is abandoned.
    rollback_written( ).
    CLEAR ms_assembly.
  ENDMETHOD.


  METHOD handle_upload_files.
    DATA(lv_step) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'step' ).
    CASE lv_step.
      WHEN 'begin'.
        rs_response = upload_begin( is_message ).
      WHEN 'chunk'.
        rs_response = upload_chunk( is_message ).
      WHEN 'commit'.
        rs_response = upload_commit( is_message ).
      WHEN 'abort'.
        rs_response = upload_abort( is_message ).
      WHEN OTHERS.
        rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                           iv_message = |step must be begin, chunk, commit or abort, not '{ lv_step }'| ).
    ENDCASE.
  ENDMETHOD.


  METHOD upload_begin.
    DATA: lv_request TYPE trkorr,
          lv_sid     TYPE string,
          lv_error   TYPE string,
          lv_uuid    TYPE sysuuid_c32.

    CLEAR ms_assembly.
    " A new upload abandons one committed and not added.
    rollback_written( ).
    DATA(lv_params) = is_message-params.
    DATA(lv_cofile_name) = zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'cofile_name' ).
    DATA(lv_data_name) = zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'data_name' ).
    DATA(lv_cofile_size) = zcl_vsp_utils=>extract_param_int( iv_params = lv_params iv_name = 'cofile_size' ).
    DATA(lv_data_size) = zcl_vsp_utils=>extract_param_int( iv_params = lv_params iv_name = 'data_size' ).
    DATA(lv_cofile_sha) = to_upper( zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'cofile_sha256' ) ).
    DATA(lv_data_sha) = to_upper( zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'data_sha256' ) ).

    request_from_names( EXPORTING iv_cofile_name = lv_cofile_name iv_data_name = lv_data_name
                        IMPORTING ev_request = lv_request ev_sid = lv_sid ev_error = lv_error ).
    IF lv_error IS NOT INITIAL.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_NAMES' iv_message = lv_error ).
      RETURN.
    ENDIF.

    IF lv_cofile_size <= 0 OR lv_data_size <= 0.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                         iv_message = `Both files are required and neither may be empty` ).
      RETURN.
    ENDIF.
    IF lv_cofile_size > c_max_cofile.
      rs_response = err( iv_id = is_message-id iv_code = 'TOO_LARGE'
                         iv_message = |The cofile is { lv_cofile_size } bytes, over the { c_max_cofile }-byte limit| ).
      RETURN.
    ENDIF.
    IF lv_data_size > c_max_total - lv_cofile_size.
      rs_response = err( iv_id = is_message-id iv_code = 'TOO_LARGE'
                         iv_message = |The two files are over the { c_max_total }-byte (50 MB) limit| ).
      RETURN.
    ENDIF.
    FIND PCRE '^[0-9A-F]{64}\z' IN lv_cofile_sha.
    DATA(lv_ok1) = xsdbool( sy-subrc = 0 ).
    FIND PCRE '^[0-9A-F]{64}\z' IN lv_data_sha.
    IF lv_ok1 = abap_false OR sy-subrc <> 0.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                         iv_message = `cofile_sha256 and data_sha256 must be SHA-256 digests in hex` ).
      RETURN.
    ENDIF.

    " Neither file may exist, not even empty: an upload never replaces one.
    DATA(lt_kinds) = VALUE string_table( ( `data` ) ( `cofile` ) ).
    LOOP AT lt_kinds INTO DATA(lv_kind).
      DATA(lv_name) = COND string( WHEN lv_kind = `cofile` THEN lv_cofile_name ELSE lv_data_name ).
      DATA(lv_exists) = file_exists( EXPORTING iv_kind = lv_kind iv_name = lv_name IMPORTING ev_error = lv_error ).
      IF lv_error IS NOT INITIAL.
        rs_response = err( iv_id = is_message-id iv_code = 'DIR_TRANS_ERROR' iv_message = lv_error ).
        RETURN.
      ENDIF.
      IF lv_exists = abap_true.
        rs_response = err( iv_id = is_message-id iv_code = 'FILE_EXISTS'
                           iv_message = |{ lv_name } already exists in DIR_TRANS ({ dir_of( lv_kind ) }); it is never overwritten. Nothing was written.| ).
        RETURN.
      ENDIF.
    ENDLOOP.

    TRY.
        lv_uuid = cl_system_uuid=>create_uuid_c32_static( ).
      CATCH cx_uuid_error.
        lv_uuid = |TR{ sy-datum }{ sy-uzeit }|.
    ENDTRY.

    ms_assembly = VALUE #(
      id          = lv_uuid
      request     = lv_request
      sid         = lv_sid
      cofile_name = lv_cofile_name
      data_name   = lv_data_name
      cofile_size = lv_cofile_size
      data_size   = lv_data_size
      cofile_sha  = lv_cofile_sha
      data_sha    = lv_data_sha ).

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'assembly_id' iv_value = ms_assembly-id ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'request' iv_value = CONV #( lv_request ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'system' iv_value = CONV #( sy-sysid ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'client' iv_value = CONV #( sy-mandt ) ) )
      ( zcl_vsp_utils=>json_int( iv_key = 'max_chunk' iv_value = c_max_chunk ) )
    ) ) ) ).
  ENDMETHOD.


  METHOD upload_chunk.
    DATA lv_chunk TYPE xstring.

    DATA(lv_params) = is_message-params.
    DATA(lv_id) = zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'assembly_id' ).
    IF ms_assembly-id IS INITIAL OR lv_id <> ms_assembly-id.
      rs_response = err( iv_id = is_message-id iv_code = 'NO_UPLOAD'
                         iv_message = `No upload with this assembly_id is in progress in this session` ).
      RETURN.
    ENDIF.
    DATA(lv_kind) = zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'file' ).
    DATA(lv_offset) = zcl_vsp_utils=>extract_param_int( iv_params = lv_params iv_name = 'offset' ).
    DATA(lv_b64) = zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'chunk_b64' ).
    TRY.
        lv_chunk = cl_http_utility=>decode_x_base64( lv_b64 ).
      CATCH cx_root.
        CLEAR lv_chunk.
    ENDTRY.
    IF xstrlen( lv_chunk ) = 0 OR xstrlen( lv_chunk ) > c_max_chunk.
      CLEAR ms_assembly.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_CHUNK'
                         iv_message = |A chunk is 1 to { c_max_chunk } bytes of base64; the upload is discarded| ).
      RETURN.
    ENDIF.

    CASE lv_kind.
      WHEN 'data'.
        IF lv_offset <> xstrlen( ms_assembly-data ) OR lv_offset + xstrlen( lv_chunk ) > ms_assembly-data_size.
          CLEAR ms_assembly.
          rs_response = err( iv_id = is_message-id iv_code = 'INVALID_CHUNK'
                             iv_message = `Data chunk out of order or past the declared size; the upload is discarded` ).
          RETURN.
        ENDIF.
        CONCATENATE ms_assembly-data lv_chunk INTO ms_assembly-data IN BYTE MODE.
      WHEN 'cofile'.
        IF lv_offset <> xstrlen( ms_assembly-cofile ) OR lv_offset + xstrlen( lv_chunk ) > ms_assembly-cofile_size.
          CLEAR ms_assembly.
          rs_response = err( iv_id = is_message-id iv_code = 'INVALID_CHUNK'
                             iv_message = `Cofile chunk out of order or past the declared size; the upload is discarded` ).
          RETURN.
        ENDIF.
        CONCATENATE ms_assembly-cofile lv_chunk INTO ms_assembly-cofile IN BYTE MODE.
      WHEN OTHERS.
        CLEAR ms_assembly.
        rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                           iv_message = `file must be cofile or data; the upload is discarded` ).
        RETURN.
    ENDCASE.

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_int( iv_key = 'cofile_received' iv_value = xstrlen( ms_assembly-cofile ) ) )
      ( zcl_vsp_utils=>json_int( iv_key = 'data_received' iv_value = xstrlen( ms_assembly-data ) ) )
    ) ) ) ).
  ENDMETHOD.


  METHOD upload_abort.
    DATA(lv_id) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'assembly_id' ).
    IF ms_assembly-id IS INITIAL OR lv_id <> ms_assembly-id.
      rs_response = err( iv_id = is_message-id iv_code = 'NO_UPLOAD'
                         iv_message = `No upload with this assembly_id is in progress in this session; nothing was discarded` ).
      RETURN.
    ENDIF.
    CLEAR ms_assembly.
    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id
      iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_bool( iv_key = 'aborted' iv_value = abap_true ) ) ).
  ENDMETHOD.


  METHOD upload_commit.
    DATA: lv_error       TYPE string,
          lv_data_path   TYPE string,
          lv_cofile_path TYPE string.

    DATA(lv_id) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'assembly_id' ).
    IF ms_assembly-id IS INITIAL OR lv_id <> ms_assembly-id.
      rs_response = err( iv_id = is_message-id iv_code = 'NO_UPLOAD'
                         iv_message = `No upload with this assembly_id is in progress in this session` ).
      RETURN.
    ENDIF.
    DATA(ls_up) = ms_assembly.
    CLEAR ms_assembly.

    IF xstrlen( ls_up-data ) <> ls_up-data_size OR xstrlen( ls_up-cofile ) <> ls_up-cofile_size.
      rs_response = err( iv_id = is_message-id iv_code = 'INCOMPLETE'
                         iv_message = |Received { xstrlen( ls_up-cofile ) } of { ls_up-cofile_size } cofile bytes and | &&
                                      |{ xstrlen( ls_up-data ) } of { ls_up-data_size } data bytes. Nothing was written.| ).
      RETURN.
    ENDIF.
    IF sha256( ls_up-data ) <> ls_up-data_sha OR sha256( ls_up-cofile ) <> ls_up-cofile_sha.
      rs_response = err( iv_id = is_message-id iv_code = 'CHECKSUM_MISMATCH'
                         iv_message = `The received bytes do not match the SHA-256 declared at begin. Nothing was written.` ).
      RETURN.
    ENDIF.
    lv_error = validate_cofile( iv_content = ls_up-cofile iv_sid = ls_up-sid ).
    IF lv_error IS NOT INITIAL.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_COFILE'
                         iv_message = |{ ls_up-cofile_name }: { lv_error }. Nothing was written.| ).
      RETURN.
    ENDIF.

    " The no-overwrite check and the writes hold the request's lock, so no
    " other upload of it can create a file in between.
    lv_error = lock_request( ls_up-request ).
    IF lv_error IS NOT INITIAL.
      rs_response = err( iv_id = is_message-id iv_code = 'LOCKED' iv_message = |{ lv_error }. Nothing was written.| ).
      RETURN.
    ENDIF.
    write_pair( EXPORTING is_up = ls_up
                IMPORTING ev_code = DATA(lv_code) ev_message = DATA(lv_msg)
                          ev_cofile_path = lv_cofile_path ev_data_path = lv_data_path ).
    unlock_request( ls_up-request ).
    IF lv_code IS NOT INITIAL.
      rs_response = err( iv_id = is_message-id iv_code = lv_code iv_message = lv_msg ).
      RETURN.
    ENDIF.

    ms_written = VALUE #( request = ls_up-request cofile_name = ls_up-cofile_name data_name = ls_up-data_name
                          cofile_sha = ls_up-cofile_sha data_sha = ls_up-data_sha ).

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'request' iv_value = CONV #( ls_up-request ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'cofile_path' iv_value = lv_cofile_path ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'data_path' iv_value = lv_data_path ) )
      ( zcl_vsp_utils=>json_int( iv_key = 'cofile_size' iv_value = ls_up-cofile_size ) )
      ( zcl_vsp_utils=>json_int( iv_key = 'data_size' iv_value = ls_up-data_size ) )
    ) ) ) ).
  ENDMETHOD.


  METHOD write_pair.
    DATA: lv_error       TYPE string,
          lv_data_open   TYPE abap_bool,
          lv_cofile_open TYPE abap_bool.

    CLEAR: ev_code, ev_message, ev_cofile_path, ev_data_path.
    " Checked again, under the lock: begin may have been a while ago.
    DATA(lt_kinds) = VALUE string_table( ( `data` ) ( `cofile` ) ).
    LOOP AT lt_kinds INTO DATA(lv_kind).
      DATA(lv_name) = COND string( WHEN lv_kind = `cofile` THEN is_up-cofile_name ELSE is_up-data_name ).
      DATA(lv_exists) = file_exists( EXPORTING iv_kind = lv_kind iv_name = lv_name IMPORTING ev_error = lv_error ).
      IF lv_error IS NOT INITIAL.
        ev_code = `DIR_TRANS_ERROR`.
        ev_message = |{ lv_error }. Nothing was written.|.
        RETURN.
      ENDIF.
      IF lv_exists = abap_true.
        ev_code = `FILE_EXISTS`.
        ev_message = |{ lv_name } already exists in DIR_TRANS; it is never overwritten. Nothing was written.|.
        RETURN.
      ENDIF.
    ENDLOOP.

    " The data file first, so that tp never finds a cofile without its data.
    " A file opened here did not exist a moment ago under the same lock, so
    " it is this call's own and may be removed again.
    write_file( EXPORTING iv_kind = `data` iv_name = is_up-data_name iv_content = is_up-data
                IMPORTING ev_opened = lv_data_open ev_path = ev_data_path ev_error = lv_error ).
    IF lv_error IS NOT INITIAL.
      DATA(lv_cleanup) = COND string( WHEN lv_data_open = abap_true THEN delete_file( iv_kind = `data` iv_name = is_up-data_name ) ).
      " A code of its own when something of this call may be left behind.
      ev_code = COND #( WHEN lv_cleanup IS INITIAL THEN `WRITE_FAILED` ELSE `WRITE_FAILED_FILES_LEFT` ).
      ev_message = |{ is_up-data_name }: { lv_error }. | &&
                   COND string( WHEN lv_data_open = abap_false THEN `Nothing was written.`
                                WHEN lv_cleanup IS INITIAL THEN `What this call wrote was deleted again.`
                                ELSE |What this call wrote could not be deleted: { lv_cleanup }| ).
      RETURN.
    ENDIF.

    write_file( EXPORTING iv_kind = `cofile` iv_name = is_up-cofile_name iv_content = is_up-cofile
                IMPORTING ev_opened = lv_cofile_open ev_path = ev_cofile_path ev_error = lv_error ).
    IF lv_error IS NOT INITIAL.
      lv_cleanup = COND string( WHEN lv_cofile_open = abap_true THEN delete_file( iv_kind = `cofile` iv_name = is_up-cofile_name ) ).
      " The data file only once no cofile of ours is left.
      DATA lv_cleanup2 TYPE string.
      IF lv_cleanup IS INITIAL.
        lv_cleanup2 = delete_file( iv_kind = `data` iv_name = is_up-data_name iv_sha = is_up-data_sha ).
      ELSE.
        lv_cleanup2 = |{ is_up-data_name } kept: the cofile could not be deleted|.
      ENDIF.
      ev_code = COND #( WHEN lv_cleanup IS INITIAL AND lv_cleanup2 IS INITIAL THEN `WRITE_FAILED` ELSE `WRITE_FAILED_FILES_LEFT` ).
      ev_message = |{ is_up-cofile_name }: { lv_error }. | &&
                   COND string( WHEN lv_cleanup IS INITIAL AND lv_cleanup2 IS INITIAL THEN `What this call wrote was deleted again.`
                                ELSE |Cleanup incomplete: { lv_cleanup } { lv_cleanup2 }| ).
      RETURN.
    ENDIF.
  ENDMETHOD.


  METHOD handle_add_to_buffer.
    DATA: lv_sid      TYPE string,
          lv_number   TYPE string,
          lv_error    TYPE string,
          lv_trkorr   TYPE trkorr,
          lv_jobcount TYPE string.

    DATA(lv_request) = to_upper( zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'request' ) ).
    IF request_parts( EXPORTING iv_request = lv_request IMPORTING ev_sid = lv_sid ev_number = lv_number ) = abap_false.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_REQUEST'
                         iv_message = |Request '{ lv_request }' is not <SID>K<6 alphanum>| ).
      RETURN.
    ENDIF.
    lv_trkorr = lv_request.

    " Only the request whose files this session's upload wrote.
    IF ms_written-request IS INITIAL OR ms_written-request <> lv_trkorr.
      rs_response = err( iv_id = is_message-id iv_code = 'NOT_UPLOADED'
                         iv_message = |{ lv_request } was not uploaded in this session; only an uploaded request is added to the buffer| ).
      RETURN.
    ENDIF.

    DATA(lt_kinds) = VALUE string_table( ( `data` ) ( `cofile` ) ).
    LOOP AT lt_kinds INTO DATA(lv_kind).
      DATA(lv_name) = COND string( WHEN lv_kind = `cofile` THEN ms_written-cofile_name ELSE ms_written-data_name ).
      IF file_exists( EXPORTING iv_kind = lv_kind iv_name = lv_name IMPORTING ev_error = lv_error ) = abap_false.
        CLEAR ms_written.
        rs_response = err( iv_id = is_message-id iv_code = 'FILES_MISSING'
                           iv_message = |{ lv_name } is not in DIR_TRANS { lv_error }; tp needs both files. Nothing was added.| ).
        RETURN.
      ENDIF.
    ENDLOOP.

    start_job( EXPORTING iv_request = lv_request iv_cofile_sha = ms_written-cofile_sha iv_data_sha = ms_written-data_sha
                         iv_push_id = iv_session_id
               IMPORTING ev_jobcount = lv_jobcount ev_error = lv_error ).
    IF lv_error IS NOT INITIAL.
      " Nothing reached tp: the request is not in the buffer, so this
      " upload's files go again -- and the answer says whether they did.
      rollback_written( IMPORTING ev_rolled_back = DATA(lv_rolled_back) ev_message = DATA(lv_rollback_msg) ).
      rs_response = err( iv_id = is_message-id
                         iv_code = COND #( WHEN lv_rolled_back = abap_true THEN `ADD_FAILED_ROLLED_BACK` ELSE `ADD_FAILED_FILES_KEPT` )
                         iv_message = |The buffer job could not be started: { lv_error }. Nothing was added. { lv_rollback_msg }| ).
      RETURN.
    ENDIF.
    " Handed over: from here the job owns the outcome, and the files.
    CLEAR ms_written.

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'status' iv_value = `pending` ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'ticket' iv_value = lv_jobcount ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'job' iv_value = CONV #( c_job_name ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'job_count' iv_value = lv_jobcount ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'request' iv_value = lv_request ) )
    ) ) ) ).
  ENDMETHOD.


  METHOD handle_show_buffer.
    " The import buffer is a file, DIR_TRANS/buffer/<SID>. It is read, and
    " nothing else is done: no tp, no job, no database write.
    DATA: lv_sid    TYPE string,
          lv_number TYPE string,
          lv_exists TYPE abap_bool,
          lv_text   TYPE string,
          lv_error  TYPE string,
          lt_items  TYPE string_table.

    DATA(lv_request) = to_upper( zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'request' ) ).
    IF lv_request IS NOT INITIAL AND
       request_parts( EXPORTING iv_request = lv_request IMPORTING ev_sid = lv_sid ev_number = lv_number ) = abap_false.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_REQUEST'
                         iv_message = |Request '{ lv_request }' is not <SID>K<6 alphanum>| ).
      RETURN.
    ENDIF.

    read_buffer_file( IMPORTING ev_exists = lv_exists ev_text = lv_text ev_error = lv_error ).
    IF lv_error IS NOT INITIAL.
      rs_response = err( iv_id = is_message-id iv_code = 'BUFFER_READ_FAILED' iv_message = lv_error ).
      RETURN.
    ENDIF.

    DATA(lt_lines) = buffer_lines( iv_text = lv_text iv_request = lv_request ).
    DATA(lv_total) = lines( lt_lines ).
    LOOP AT lt_lines INTO DATA(ls_line) TO c_max_buffer_entries.
      APPEND zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
        ( zcl_vsp_utils=>json_str( iv_key = 'trkorr' iv_value = ls_line-trkorr ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'trfunction' iv_value = ls_line-trfunction ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'owner' iv_value = ls_line-owner ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'raw' iv_value = ls_line-raw ) )
      ) ) ) TO lt_items.
    ENDLOOP.

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'status' iv_value = `done` ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'system' iv_value = CONV #( sy-sysid ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'client' iv_value = CONV #( sy-mandt ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'source' iv_value = |DIR_TRANS/buffer/{ sy-sysid }| ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'file_exists' iv_value = lv_exists ) )
      ( zcl_vsp_utils=>json_int( iv_key = 'total' iv_value = lv_total ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'truncated' iv_value = xsdbool( lv_total > c_max_buffer_entries ) ) )
      ( |"entries":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_items ) ) }| )
    ) ) ) ).
  ENDMETHOD.


  METHOD start_job.
    DATA: lv_jobname  TYPE tbtcjob-jobname,
          lv_jobcount TYPE tbtcjob-jobcount,
          lv_released TYPE btch0000-char1,
          lv_variant  TYPE rsvar-variant,
          ls_varid    TYPE varid,
          lt_contents TYPE STANDARD TABLE OF rsparams WITH DEFAULT KEY,
          lt_text     TYPE STANDARD TABLE OF varit WITH DEFAULT KEY.

    CLEAR: ev_jobcount, ev_error.
    lv_jobname = c_job_name.
    CALL FUNCTION 'JOB_OPEN'
      EXPORTING
        jobname          = lv_jobname
      IMPORTING
        jobcount         = lv_jobcount
      EXCEPTIONS
        cant_create_job  = 1
        invalid_job_data = 2
        jobname_missing  = 3
        OTHERS           = 4.
    IF sy-subrc <> 0.
      ev_error = |JOB_OPEN failed (exception { sy-subrc }) { last_message( ) }|.
      RETURN.
    ENDIF.

    " What the job is to do is bound to its step: a protected variant (only
    " its creator may change it) of ZVSP_TRANSPORT_BUFFER, named after the
    " job, holding the request and the SHA-256 of the two files as written
    " (base64: a variant value is at most 45 characters). The next upload
    " deletes it once the job has ended.
    " SUBMIT ... VIA JOB would bind them directly, but an APC session may not
    " execute SUBMIT.
    " Housekeeping first: the variants of earlier jobs that have ended (a
    " running job cannot delete its own).
    SELECT variant FROM varid INTO TABLE @DATA(lt_old)
      WHERE report = 'ZVSP_TRANSPORT_BUFFER' AND variant LIKE 'VSP%'.
    LOOP AT lt_old INTO DATA(ls_old).
      DATA(lv_oldcount) = CONV tbtcjob-jobcount( ls_old-variant+3 ).
      SELECT SINGLE status FROM tbtco INTO @DATA(lv_oldstatus)
        WHERE jobname = @lv_jobname AND jobcount = @lv_oldcount.
      IF sy-subrc <> 0 OR lv_oldstatus = 'F' OR lv_oldstatus = 'A'.
        CALL FUNCTION 'RS_VARIANT_DELETE'
          EXPORTING
            report                = 'ZVSP_TRANSPORT_BUFFER'
            variant               = ls_old-variant
            flag_confirmscreen    = 'X'
            suppress_message      = 'X'
            suppress_input_dialog = 'X'
          EXCEPTIONS
            OTHERS                = 1.
      ENDIF.
    ENDLOOP.

    lv_variant = |VSP{ lv_jobcount }|.
    ls_varid = VALUE #( report = 'ZVSP_TRANSPORT_BUFFER' variant = lv_variant environmnt = 'B' protected = 'X' ).
    lt_contents = VALUE #(
      ( selname = 'P_REQ'  kind = 'P' sign = 'I' option = 'EQ' low = iv_request )
      ( selname = 'P_SHAC' kind = 'P' sign = 'I' option = 'EQ' low = sha_b64( iv_cofile_sha ) )
      ( selname = 'P_SHAD' kind = 'P' sign = 'I' option = 'EQ' low = sha_b64( iv_data_sha ) )
      ( selname = 'P_PUSH' kind = 'P' sign = 'I' option = 'EQ' low = iv_push_id ) ).
    lt_text = VALUE #( ( langu = sy-langu report = 'ZVSP_TRANSPORT_BUFFER' variant = lv_variant vtext = |vsp upload { iv_request }| ) ).
    CALL FUNCTION 'RS_CREATE_VARIANT'
      EXPORTING
        curr_report               = 'ZVSP_TRANSPORT_BUFFER'
        curr_variant              = lv_variant
        vari_desc                 = ls_varid
      TABLES
        vari_contents             = lt_contents
        vari_text                 = lt_text
      EXCEPTIONS
        illegal_report_or_variant = 1
        illegal_variantname       = 2
        not_authorized            = 3
        not_executed              = 4
        report_not_existent       = 5
        report_not_supplied       = 6
        variant_exists            = 7
        variant_locked            = 8
        OTHERS                    = 9.
    IF sy-subrc <> 0.
      ev_error = |RS_CREATE_VARIANT { lv_variant } failed (exception { sy-subrc }) { last_message( ) }|.
      RETURN.
    ENDIF.

    CALL FUNCTION 'JOB_SUBMIT'
      EXPORTING
        authcknam         = sy-uname
        jobcount          = lv_jobcount
        jobname           = lv_jobname
        report            = 'ZVSP_TRANSPORT_BUFFER'
        variant           = lv_variant
      EXCEPTIONS
        bad_priparams     = 1
        bad_xpgflags      = 2
        invalid_jobdata   = 3
        jobname_missing   = 4
        job_notex         = 5
        job_submit_failed = 6
        lock_failed       = 7
        OTHERS            = 8.
    IF sy-subrc <> 0.
      ev_error = |JOB_SUBMIT failed (exception { sy-subrc }) { last_message( ) }|.
      RETURN.
    ENDIF.

    " A start time within ten seconds is an immediate start (JOB_CLOSE).
    CALL FUNCTION 'JOB_CLOSE'
      EXPORTING
        jobcount             = lv_jobcount
        jobname              = lv_jobname
        sdlstrtdt            = sy-datum
        sdlstrttm            = sy-uzeit
      IMPORTING
        job_was_released     = lv_released
      EXCEPTIONS
        cant_start_immediate = 1
        invalid_startdate    = 2
        jobname_missing      = 3
        job_close_failed     = 4
        job_nosteps          = 5
        job_notex            = 6
        lock_failed          = 7
        invalid_target       = 8
        invalid_time_zone    = 9
        OTHERS               = 10.
    IF sy-subrc <> 0.
      ev_error = |JOB_CLOSE failed (exception { sy-subrc }) { last_message( ) }|.
      RETURN.
    ENDIF.
    IF lv_released IS INITIAL.
      ev_error = |job { lv_jobname } { lv_jobcount } was scheduled but not released: releasing it needs S_BTCH_JOB with JOBACTION RELE; delete it in SM37|.
      RETURN.
    ENDIF.
    ev_jobcount = lv_jobcount.
  ENDMETHOD.


  METHOD read_dir_file.
    DATA: lv_name     TYPE eps2filnam,
          lv_dir      TYPE epsf-epsdirnam,
          lv_long_dir TYPE eps2path,
          lv_path     TYPE eps2path,
          lv_size     TYPE eps2filsiz,
          lv_pos      TYPE epsfilsiz,
          lv_dataset  TYPE string.

    CLEAR: ev_content, ev_error.
    lv_name = iv_name.
    lv_dir = dir_of( iv_kind ).
    CALL FUNCTION 'EPS_OPEN_INPUT_FILE'
      EXPORTING
        iv_long_file_name      = lv_name
        dir_name               = lv_dir
        pos                    = lv_pos
      IMPORTING
        ev_long_dir_name       = lv_long_dir
        ev_long_file_path      = lv_path
        ev_file_size_long      = lv_size
      EXCEPTIONS
        invalid_eps_subdir     = 1
        sapgparam_failed       = 2
        build_directory_failed = 3
        no_authorization       = 4
        build_path_failed      = 5
        open_failed            = 6
        read_directory_failed  = 7
        read_attributes_failed = 8
        OTHERS                 = 9.
    IF sy-subrc <> 0.
      ev_error = |{ iv_name } in DIR_TRANS ({ lv_dir }) cannot be read (exception { sy-subrc }) { last_message( ) }|.
      RETURN.
    ENDIF.
    CALL FUNCTION 'EPS_CLOSE_FILE'
      EXPORTING
        iv_long_file_name = lv_name
        iv_long_dir_name  = lv_long_dir
      EXCEPTIONS
        OTHERS            = 1.
    IF lv_size > c_max_total.
      ev_error = |{ iv_name } is { lv_size } bytes, over the { c_max_total }-byte limit|.
      RETURN.
    ENDIF.
    lv_dataset = lv_path.
    TRY.
        OPEN DATASET lv_dataset FOR INPUT IN BINARY MODE.
        IF sy-subrc <> 0.
          ev_error = |{ iv_name } could not be opened|.
          RETURN.
        ENDIF.
        READ DATASET lv_dataset INTO ev_content.
        CLOSE DATASET lv_dataset.
      CATCH cx_root INTO DATA(lx_read).
        CLOSE DATASET lv_dataset.
        ev_error = |{ iv_name }: { lx_read->get_text( ) }|.
    ENDTRY.
  ENDMETHOD.


  METHOD run_job.
    DATA: lv_jobcount TYPE tbtcm-jobcount,
          lv_jobname  TYPE tbtcm-jobname,
          ls_res      TYPE ty_job_result,
          lt_buffer   TYPE tt_tpbuffer,
          lt_addout   TYPE tt_tpstdout,
          lv_content  TYPE xstring,
          lv_error    TYPE string,
          lv_sid      TYPE string,
          lv_number   TYPE string,
          lv_trkorr   TYPE trkorr,
          lv_cmd      TYPE stpa-cmdstring,
          lv_rc       TYPE stpa-retcode,
          lv_msg      TYPE stpa-message,
          lv_system   TYPE stpa-sysname.

    " This system, and only ever this system (STPA-SYSNAME is CHAR10).
    lv_system = sy-sysid.

    CALL FUNCTION 'GET_JOB_RUNTIME_INFO'
      IMPORTING
        jobcount        = lv_jobcount
        jobname         = lv_jobname
      EXCEPTIONS
        no_runtime_info = 1
        OTHERS          = 2.
    IF sy-subrc <> 0 OR lv_jobname <> c_job_name.
      RETURN.
    ENDIF.

    ls_res = VALUE #( request = to_upper( iv_request ) system = CONV #( sy-sysid ) job = CONV #( lv_jobcount ) ).

    " The step must run with this job's own variant as start_job made it:
    " VSP<job number>, protected, created by this user and changed by no one
    " else. The program run by hand, with another variant, or with a variant
    " someone edited is refused here, before anything else is done.
    DATA(lv_own_variant) = CONV rsvar-variant( |VSP{ lv_jobcount }| ).
    SELECT SINGLE protected, ename, aename FROM varid INTO @DATA(ls_varid)
      WHERE report = 'ZVSP_TRANSPORT_BUFFER' AND variant = @lv_own_variant.
    DATA(lv_variant_found) = xsdbool( sy-subrc = 0 ).
    " Its text names the request too (start_job wrote both): what ties the
    " job to the request for add_status, so it must agree with P_REQ.
    SELECT SINGLE vtext FROM varit INTO @DATA(lv_vtext)
      WHERE report = 'ZVSP_TRANSPORT_BUFFER' AND variant = @lv_own_variant.
    IF sy-subrc <> 0 OR lv_vtext <> |vsp upload { to_upper( iv_request ) }|.
      lv_variant_found = abap_false.
    ENDIF.
    IF sy-slset <> lv_own_variant OR lv_variant_found = abap_false OR ls_varid-protected <> 'X'
       OR ls_varid-ename <> sy-uname OR ( ls_varid-aename IS NOT INITIAL AND ls_varid-aename <> sy-uname ).
      ls_res-outcome = `not_added`.
      ls_res-code = `VARIANT_MISMATCH`.
      ls_res-message = |The step does not run with its own protected variant { lv_own_variant } of { sy-uname } (runs with '{ sy-slset }'); nothing was done.|.
      job_log( ls_res ).
      publish( is_result = ls_res iv_push_id = iv_push_id ).
      RETURN.
    ENDIF.

    " The SHA-256 of the two files as the upload wrote them, in base64.
    DATA(lv_shac) = condense( CONV string( iv_cofile_sha ) ).
    DATA(lv_shad) = condense( CONV string( iv_data_sha ) ).
    FIND PCRE '^[A-Za-z0-9+/]{43}=\z' IN lv_shac.
    DATA(lv_sha_ok) = xsdbool( sy-subrc = 0 ).
    FIND PCRE '^[A-Za-z0-9+/]{43}=\z' IN lv_shad.
    lv_sha_ok = xsdbool( lv_sha_ok = abap_true AND sy-subrc = 0 ).
    IF request_parts( EXPORTING iv_request = ls_res-request IMPORTING ev_sid = lv_sid ev_number = lv_number ) = abap_false
       OR lv_sha_ok = abap_false.
      ls_res-outcome = `not_added`.
      ls_res-code = `INVALID_PARAM`.
      ls_res-message = |'{ iv_request }' is not a request, or the SHA-256 values are not digests; nothing was done.|.
      job_log( ls_res ).
    publish( is_result = ls_res iv_push_id = iv_push_id ).
      RETURN.
    ENDIF.
    lv_trkorr = ls_res-request.

    " The files must be the ones the upload wrote: same SHA-256 as recorded
    " when the job was scheduled.
    DATA(lt_files) = VALUE string_table( ( `cofile` ) ( `data` ) ).
    LOOP AT lt_files INTO DATA(lv_kind).
      DATA(lv_name) = COND string( WHEN lv_kind = `cofile` THEN |K{ lv_number }.{ lv_sid }| ELSE |R{ lv_number }.{ lv_sid }| ).
      read_dir_file( EXPORTING iv_kind = lv_kind iv_name = lv_name IMPORTING ev_content = lv_content ev_error = lv_error ).
      IF lv_error IS INITIAL AND sha_b64( sha256( lv_content ) ) <> COND string( WHEN lv_kind = `cofile` THEN lv_shac ELSE lv_shad ).
        lv_error = |{ lv_name } is not the file the upload wrote (SHA-256 differs)|.
      ENDIF.
      IF lv_error IS NOT INITIAL.
        ls_res-outcome = `not_added`.
        ls_res-code = `FILES_CHANGED`.
        ls_res-message = |{ lv_error }; ADDTOBUFFER was not called, and no file was touched.|.
        job_log( ls_res ).
    publish( is_result = ls_res iv_push_id = iv_push_id ).
        RETURN.
      ENDIF.
    ENDLOOP.

    " The buffer check and the add hold the request's lock: no second add of
    " it can come in between (tp would re-initialise an existing entry).
    lv_error = lock_request( iv_request = lv_trkorr iv_wait = abap_true ).
    IF lv_error IS NOT INITIAL.
      ls_res-outcome = `not_added`.
      ls_res-code = `LOCKED`.
      ls_res-message = |{ lv_error }; ADDTOBUFFER was not called.|.
      job_log( ls_res ).
    publish( is_result = ls_res iv_push_id = iv_push_id ).
      RETURN.
    ENDIF.

    read_buffer( IMPORTING et_buffer = lt_buffer ev_error = lv_error ).
    IF lv_error IS NOT INITIAL.
      ls_res-outcome = `unknown`.
      ls_res-code = `BUFFER_READ_FAILED`.
      ls_res-message = |The import buffer of { sy-sysid } could not be read, so ADDTOBUFFER was not called: { lv_error }|.
    ELSEIF line_exists( lt_buffer[ trkorr = lv_trkorr ] ).
      " Never twice: a request already in the queue is left alone.
      ls_res-outcome = `queued`.
      ls_res-in_buffer = abap_true.
      ls_res-buffer_known = abap_true.
      ls_res-code = `ALREADY_IN_BUFFER`.
      ls_res-message = |{ lv_trkorr } was already in the import buffer of { sy-sysid }; it was not added again.|.
    ELSE.
      " The one tp command this class sends. The command is a literal,
      " the system is this one, and no option, client or flag is passed.
      CALL FUNCTION 'TMS_TP_MAINTAIN_BUFFER'
        EXPORTING
          iv_tp_command      = 'ADDTOBUFFER'
          iv_system_name     = lv_system
          iv_request         = lv_trkorr
        IMPORTING
          ev_tp_cmd_strg     = lv_cmd
          ev_tp_ret_code     = lv_rc
          ev_tp_message      = lv_msg
        TABLES
          tt_stdout          = lt_addout
        EXCEPTIONS
          invalid_command    = 1
          permission_denied  = 2
          tp_call_failed     = 3
          tp_interface_error = 4
          tp_reported_error  = 5
          OTHERS             = 6.
      DATA(lv_subrc) = sy-subrc.
      DATA(lv_sysmsg) = COND string( WHEN lv_subrc <> 0 THEN last_message( ) ).
      ls_res-tp_cmd = lv_cmd.
      ls_res-tp_rc = lv_rc.
      ls_res-tp_msg = lv_msg.
      IF lv_subrc <> 0.
        ls_res-code = SWITCH string( lv_subrc
          WHEN 2 THEN `PERMISSION_DENIED`
          WHEN 3 THEN `TP_CALL_FAILED`
          WHEN 5 THEN `TP_REPORTED_ERROR`
          ELSE `ADD_FAILED` ).
        ls_res-message = |ADDTOBUFFER { lv_trkorr } { sy-sysid } failed ({ ls_res-code }, tp rc { lv_rc }): { lv_sysmsg } { lv_msg }|.
        IF lv_subrc = 2.
          ls_res-message = ls_res-message && ` -- adding to the buffer needs S_CTS_ADMI with CTS_ADMFCT TADD (or IMPT).`.
        ENDIF.
      ENDIF.
      " Read back: only the buffer says whether the request is queued.
      CLEAR lt_buffer.
      read_buffer( IMPORTING et_buffer = lt_buffer ev_error = lv_error ).
      ls_res-buffer_known = xsdbool( lv_error IS INITIAL ).
      ls_res-in_buffer = xsdbool( line_exists( lt_buffer[ trkorr = lv_trkorr ] ) ).
      IF ls_res-in_buffer = abap_true.
        ls_res-outcome = `queued`.
      ELSEIF ls_res-buffer_known = abap_true.
        " Certainly not in the buffer: the upload's files go again, so that
        " the same pair can be uploaded once more.
        ls_res-outcome = `not_added`.
        " The cofile first; the data file only once the cofile is gone, so a
        " cofile is never left without its data file.
        DATA lv_del2 TYPE string.
        DATA(lv_del1) = delete_file( iv_kind = `cofile` iv_name = |K{ lv_number }.{ lv_sid }|
                                     iv_sha = CONV string( cl_http_utility=>decode_x_base64( lv_shac ) ) ).
        IF lv_del1 IS INITIAL.
          lv_del2 = delete_file( iv_kind = `data` iv_name = |R{ lv_number }.{ lv_sid }|
                                 iv_sha = CONV string( cl_http_utility=>decode_x_base64( lv_shad ) ) ).
        ELSE.
          lv_del2 = |R{ lv_number }.{ lv_sid } kept: the cofile could not be taken back|.
        ENDIF.
        ls_res-rolled_back = xsdbool( lv_del1 IS INITIAL AND lv_del2 IS INITIAL ).
        IF ls_res-rolled_back = abap_false.
          ls_res-message = |{ ls_res-message } Files not taken back: { lv_del1 } { lv_del2 }|.
        ENDIF.
      ELSE.
        ls_res-outcome = `unknown`.
      ENDIF.
    ENDIF.
    unlock_request( lv_trkorr ).
    job_log( ls_res ).
    publish( is_result = ls_res iv_push_id = iv_push_id ).
  ENDMETHOD.


  METHOD handle_add_status.
    " Read-only: the job's status (TBTCO), its log, and the buffer file. Only
    " the buffer file proves "queued".
    DATA: lv_sid       TYPE string,
          lv_number    TYPE string,
          lv_exists    TYPE abap_bool,
          lv_text      TYPE string,
          lv_error     TYPE string,
          lv_status    TYPE tbtco-status,
          lv_jobname   TYPE tbtcjob-jobname,
          lv_jobcount  TYPE tbtcjob-jobcount,
          lt_log       TYPE STANDARD TABLE OF tbtc5 WITH DEFAULT KEY,
          lt_log_json  TYPE string_table,
          lv_outcome   TYPE string,
          lv_job_found TYPE abap_bool,
          lv_cof_err   TYPE string,
          lv_dat_err   TYPE string,
          lv_tied_to   TYPE string.

    DATA(lv_request) = to_upper( zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'request' ) ).
    IF request_parts( EXPORTING iv_request = lv_request IMPORTING ev_sid = lv_sid ev_number = lv_number ) = abap_false.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_REQUEST'
                         iv_message = |Request '{ lv_request }' is not <SID>K<6 alphanum>| ).
      RETURN.
    ENDIF.
    DATA(lv_job) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'job' ).
    IF lv_job IS NOT INITIAL.
      FIND PCRE '^[0-9]{8}\z' IN lv_job.
      IF sy-subrc <> 0.
        rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM' iv_message = |job '{ lv_job }' is not a job number| ).
        RETURN.
      ENDIF.
      lv_jobname = c_job_name.
      lv_jobcount = lv_job.
      SELECT SINGLE status FROM tbtco INTO @lv_status
        WHERE jobname = @lv_jobname AND jobcount = @lv_jobcount.
      lv_job_found = xsdbool( sy-subrc = 0 ).
      IF lv_job_found = abap_true.
        CALL FUNCTION 'BP_JOBLOG_READ'
          EXPORTING
            jobcount              = lv_jobcount
            jobname               = lv_jobname
          TABLES
            joblogtbl             = lt_log
          EXCEPTIONS
            cant_read_joblog      = 1
            jobcount_missing      = 2
            joblog_does_not_exist = 3
            joblog_is_empty       = 4
            joblog_name_missing   = 5
            jobname_missing       = 6
            job_does_not_exist    = 7
            OTHERS                = 8.
        LOOP AT lt_log INTO DATA(ls_log) WHERE text CP 'VSP *'.
          APPEND |"{ zcl_vsp_utils=>escape_json( CONV #( ls_log-text ) ) }"| TO lt_log_json.
          " The job names its request in its log.
          IF ls_log-text CP 'VSP request=*'.
            " "VSP request=<REQ> outcome=...": the second word.
            SPLIT condense( CONV string( ls_log-text ) ) AT space INTO DATA(lv_vsp) DATA(lv_req_word) DATA(lv_tail).
            lv_tied_to = to_upper( substring_after( val = lv_req_word sub = '=' ) ).
          ENDIF.
        ENDLOOP.
        " Before it has logged anything: its variant's text names it.
        IF lv_tied_to IS INITIAL.
          SELECT SINGLE vtext FROM varit
            WHERE report = 'ZVSP_TRANSPORT_BUFFER' AND variant = @( CONV rsvar-variant( |VSP{ lv_job }| ) )
            INTO @DATA(lv_vtext).
          IF sy-subrc = 0.
            IF lv_vtext CP 'vsp upload *'.
              SPLIT condense( CONV string( lv_vtext ) ) AT space INTO DATA(lv_w1) DATA(lv_w2) DATA(lv_w3).
              lv_tied_to = to_upper( lv_w3 ).
            ENDIF.
          ENDIF.
        ENDIF.
        IF lv_tied_to IS NOT INITIAL AND lv_tied_to <> lv_request.
          rs_response = err( iv_id = is_message-id iv_code = 'JOB_NOT_FOR_REQUEST'
                             iv_message = |Job { lv_job } does not belong to { lv_request }: it is for { lv_tied_to }| ).
          RETURN.
        ENDIF.
      ENDIF.
    ENDIF.

    read_buffer_file( IMPORTING ev_exists = lv_exists ev_text = lv_text ev_error = lv_error ).
    DATA(lv_in_buffer) = xsdbool( lv_error IS INITIAL AND lines( buffer_lines( iv_text = lv_text iv_request = lv_request ) ) > 0 ).
    DATA(lv_cof_present) = file_exists( EXPORTING iv_kind = `cofile` iv_name = |K{ lv_number }.{ lv_sid }| IMPORTING ev_error = lv_cof_err ).
    DATA(lv_dat_present) = file_exists( EXPORTING iv_kind = `data` iv_name = |R{ lv_number }.{ lv_sid }| IMPORTING ev_error = lv_dat_err ).

    " queued: the buffer file has the request, and the job (if named) is done.
    " pending: the job has not finished. job_failed: the job finished or was
    " cancelled and the buffer file does not have the request. Anything else,
    " including a buffer that cannot be read, is unknown.
    IF lv_error IS NOT INITIAL.
      lv_outcome = `unknown`.
    ELSEIF lv_job IS INITIAL.
      lv_outcome = COND #( WHEN lv_in_buffer = abap_true THEN `queued` ELSE `not_in_buffer` ).
    ELSEIF lv_job_found = abap_false OR lv_tied_to IS INITIAL.
      " A job that cannot be tied to the request says nothing about it.
      lv_outcome = `unknown`.
    ELSEIF lv_status = 'P' OR lv_status = 'S' OR lv_status = 'Y' OR lv_status = 'Z' OR lv_status = 'R'.
      lv_outcome = `pending`.
    ELSEIF lv_status = 'F'.
      lv_outcome = COND #( WHEN lv_in_buffer = abap_true THEN `queued` ELSE `job_failed` ).
    ELSE.
      lv_outcome = COND #( WHEN lv_in_buffer = abap_true THEN `unknown` ELSE `job_failed` ).
    ENDIF.

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'request' iv_value = lv_request ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'system' iv_value = CONV #( sy-sysid ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'outcome' iv_value = lv_outcome ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'job' iv_value = CONV #( c_job_name ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'job_count' iv_value = lv_job ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'job_found' iv_value = lv_job_found ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'job_tied' iv_value = xsdbool( lv_tied_to = lv_request ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'job_request' iv_value = lv_tied_to ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'job_status' iv_value = CONV #( lv_status ) ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'in_buffer' iv_value = lv_in_buffer ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'buffer_error' iv_value = lv_error ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'cofile_present' iv_value = lv_cof_present ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'data_present' iv_value = lv_dat_present ) )
      ( |"job_log":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_log_json ) ) }| )
    ) ) ) ).
  ENDMETHOD.


  METHOD rollback_written.
    DATA lv_del2 TYPE string.

    CLEAR: ev_rolled_back, ev_message.
    IF ms_written-request IS INITIAL.
      RETURN.
    ENDIF.
    " Only the files as this upload wrote them; one changed since is kept.
    " The cofile first, the data file only once the cofile is gone.
    DATA(lv_lock) = lock_request( ms_written-request ).
    IF lv_lock IS NOT INITIAL.
      ev_message = |The files of { ms_written-request } were not taken back: { lv_lock }|.
    ELSE.
      DATA(lv_del1) = delete_file( iv_kind = `cofile` iv_name = ms_written-cofile_name iv_sha = ms_written-cofile_sha ).
      IF lv_del1 IS INITIAL.
        lv_del2 = delete_file( iv_kind = `data` iv_name = ms_written-data_name iv_sha = ms_written-data_sha ).
      ELSE.
        lv_del2 = |{ ms_written-data_name } kept: the cofile could not be taken back|.
      ENDIF.
      unlock_request( ms_written-request ).
      ev_rolled_back = xsdbool( lv_del1 IS INITIAL AND lv_del2 IS INITIAL ).
      ev_message = COND #( WHEN ev_rolled_back = abap_true
                           THEN |{ ms_written-cofile_name } and { ms_written-data_name } were deleted again.|
                           ELSE |Not taken back: { lv_del1 } { lv_del2 }| ).
    ENDIF.
    CLEAR ms_written.
  ENDMETHOD.


  METHOD read_buffer_file.
    DATA: lv_name       TYPE eps2filnam,
          lv_buffer_dir TYPE epsf-epsdirnam,
          lv_long_dir   TYPE eps2path,
          lv_path       TYPE eps2path,
          lv_size       TYPE eps2filsiz,
          lv_pos        TYPE epsfilsiz,
          lv_content    TYPE xstring,
          lv_dataset    TYPE string,
          lv_probe_err  TYPE string.

    CLEAR: ev_exists, ev_text, ev_error.
    lv_name = sy-sysid.
    lv_buffer_dir = '$TR_BUFF'.
    ev_exists = probe_file( EXPORTING iv_subdir = CONV #( lv_buffer_dir ) iv_name = CONV #( lv_name )
                            IMPORTING ev_error = lv_probe_err ).
    IF lv_probe_err IS NOT INITIAL.
      ev_error = |The import buffer file { lv_name } in DIR_TRANS ($TR_BUFF) could not be read: { lv_probe_err }|.
      RETURN.
    ENDIF.
    IF ev_exists = abap_false.
      " A listing without it: nothing was ever queued for this system.
      RETURN.
    ENDIF.

    CALL FUNCTION 'EPS_OPEN_INPUT_FILE'
      EXPORTING
        iv_long_file_name      = lv_name
        dir_name               = lv_buffer_dir
        pos                    = lv_pos
      IMPORTING
        ev_long_dir_name       = lv_long_dir
        ev_long_file_path      = lv_path
        ev_file_size_long      = lv_size
      EXCEPTIONS
        invalid_eps_subdir     = 1
        sapgparam_failed       = 2
        build_directory_failed = 3
        no_authorization       = 4
        build_path_failed      = 5
        open_failed            = 6
        read_directory_failed  = 7
        read_attributes_failed = 8
        OTHERS                 = 9.
    DATA(lv_subrc) = sy-subrc.
    IF lv_subrc <> 0.
      " It exists; failing to open it is an error, not an empty buffer.
      ev_error = |The import buffer file { lv_name } in DIR_TRANS ($TR_BUFF) could not be read (exception { lv_subrc }) { last_message( ) }|.
      RETURN.
    ENDIF.
    CALL FUNCTION 'EPS_CLOSE_FILE'
      EXPORTING
        iv_long_file_name = lv_name
        iv_long_dir_name  = lv_long_dir
      EXCEPTIONS
        OTHERS            = 1.
    IF lv_size > c_max_total.
      ev_error = |The import buffer file is { lv_size } bytes, over the { c_max_total }-byte limit|.
      RETURN.
    ENDIF.
    lv_dataset = lv_path.
    TRY.
        OPEN DATASET lv_dataset FOR INPUT IN BINARY MODE.
        IF sy-subrc <> 0.
          ev_error = |The import buffer file { lv_name } could not be opened|.
          RETURN.
        ENDIF.
        READ DATASET lv_dataset INTO lv_content.
        CLOSE DATASET lv_dataset.
        ev_text = cl_abap_codepage=>convert_from( source = lv_content codepage = `UTF-8` ).
      CATCH cx_root INTO DATA(lx_read).
        CLOSE DATASET lv_dataset.
        ev_error = |The import buffer file { lv_name }: { lx_read->get_text( ) }|.
    ENDTRY.
  ENDMETHOD.


  METHOD buffer_lines.
    " One request per line: [/<n>/]<TRKORR> <type><release> <owner> ... ;
    " lines starting with '#' are comments (tp comments out what
    " delfrombuffer removes).
    DATA: lt_lines TYPE string_table,
          lt_tok   TYPE string_table.

    SPLIT iv_text AT cl_abap_char_utilities=>newline INTO TABLE lt_lines.
    LOOP AT lt_lines INTO DATA(lv_line).
      REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>cr_lf(1) IN lv_line WITH ``.
      DATA(lv_raw) = condense( lv_line ).
      IF lv_raw IS INITIAL OR lv_raw(1) = '#'.
        CONTINUE.
      ENDIF.
      SPLIT lv_raw AT space INTO TABLE lt_tok.
      DATA(lv_req) = lt_tok[ 1 ].
      REPLACE PCRE '^/[^/]*/' IN lv_req WITH ``.
      IF iv_request IS NOT INITIAL AND lv_req <> iv_request.
        CONTINUE.
      ENDIF.
      APPEND VALUE #(
        trkorr     = lv_req
        trfunction = COND #( WHEN lines( lt_tok ) >= 2 AND strlen( lt_tok[ 2 ] ) >= 1 THEN substring( val = lt_tok[ 2 ] len = 1 ) )
        owner      = COND #( WHEN lines( lt_tok ) >= 3 THEN lt_tok[ 3 ] )
        raw        = lv_raw ) TO rt_lines.
    ENDLOOP.
  ENDMETHOD.


  METHOD publish.
    IF iv_push_id IS INITIAL.
      RETURN.
    ENDIF.
    DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'event' iv_value = `add_result` ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'request' iv_value = is_result-request ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'system' iv_value = is_result-system ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'job' iv_value = CONV #( c_job_name ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'job_count' iv_value = is_result-job ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'outcome' iv_value = is_result-outcome ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'code' iv_value = is_result-code ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'message' iv_value = is_result-message ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'tp_command' iv_value = is_result-tp_cmd ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'tp_rc' iv_value = is_result-tp_rc ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'tp_message' iv_value = is_result-tp_msg ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'in_buffer' iv_value = is_result-in_buffer ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'rolled_back' iv_value = is_result-rolled_back ) )
    ) ) ).
    " The frame the WebSocket client gets: a response-shaped message whose
    " id names the job.
    DATA(lv_message) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'id' iv_value = |push:transport:{ is_result-job }| ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'success' iv_value = abap_true ) )
      ( |"data":{ lv_data }| )
    ) ) ).
    TRY.
        DATA(lo_producer) = CAST if_amc_message_producer_text(
          cl_amc_channel_manager=>create_message_producer(
            i_application_id       = c_amc_app
            i_channel_id           = c_amc_channel
            i_channel_extension_id = CONV #( iv_push_id ) ) ).
        lo_producer->send( lv_message ).
        COMMIT WORK.
      CATCH cx_amc_error INTO DATA(lx_amc).
        DATA(lv_line) = CONV char120( |VSP push not sent: { lx_amc->get_text( ) }| ).
        MESSAGE lv_line TYPE 'S'.
    ENDTRY.
  ENDMETHOD.


  METHOD job_log.
    " Background job: MESSAGE TYPE 'S' goes to the job log.
    DATA lv_line TYPE c LENGTH 120.
    " The request first: add_status ties a job to its request by this line.
    lv_line = |VSP request={ is_result-request } outcome={ is_result-outcome } code={ is_result-code } in_buffer={ is_result-in_buffer } rolled_back={ is_result-rolled_back } tp_rc={ is_result-tp_rc }|.
    MESSAGE lv_line TYPE 'S'.
    IF is_result-message IS NOT INITIAL.
      lv_line = |VSP message { is_result-message }|.
      MESSAGE lv_line TYPE 'S'.
    ENDIF.
    IF is_result-tp_cmd IS NOT INITIAL.
      lv_line = |VSP tp { is_result-tp_cmd }|.
      MESSAGE lv_line TYPE 'S'.
    ENDIF.
  ENDMETHOD.


  METHOD handle_download_files.
    DATA: lv_sid      TYPE string,
          lv_number   TYPE string,
          lv_name     TYPE eps2filnam,
          lv_dir      TYPE epsf-epsdirnam,
          lv_long_dir TYPE eps2path,
          lv_path     TYPE eps2path,
          lv_size     TYPE eps2filsiz,
          lv_pos      TYPE epsfilsiz,
          lv_chunk    TYPE xstring,
          lv_dataset  TYPE string.

    DATA(lv_params) = is_message-params.
    DATA(lv_request) = to_upper( zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'request' ) ).
    IF request_parts( EXPORTING iv_request = lv_request IMPORTING ev_sid = lv_sid ev_number = lv_number ) = abap_false.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_REQUEST'
                         iv_message = |Request '{ lv_request }' is not <SID>K<6 alphanum>| ).
      RETURN.
    ENDIF.
    DATA(lv_kind) = zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'file' ).
    IF lv_kind <> `cofile` AND lv_kind <> `data`.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM' iv_message = `file must be cofile or data` ).
      RETURN.
    ENDIF.
    DATA(lv_offset) = zcl_vsp_utils=>extract_param_int( iv_params = lv_params iv_name = 'offset' ).
    DATA(lv_length) = zcl_vsp_utils=>extract_param_int( iv_params = lv_params iv_name = 'length' ).
    IF lv_offset < 0.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM' iv_message = `offset must be a non-negative number` ).
      RETURN.
    ENDIF.
    IF lv_length <= 0 OR lv_length > c_max_chunk.
      lv_length = c_max_chunk.
    ENDIF.
    DATA(lv_limit) = COND i( WHEN lv_kind = `cofile` THEN c_max_cofile ELSE c_max_total ).

    lv_name = COND string( WHEN lv_kind = `cofile` THEN |K{ lv_number }.{ lv_sid }| ELSE |R{ lv_number }.{ lv_sid }| ).
    lv_dir = dir_of( lv_kind ).
    lv_pos = lv_offset.

    " SAP's own read checks (transport read authority, the transdir sub-path)
    " and the size; the file is then read here and closed again.
    CALL FUNCTION 'EPS_OPEN_INPUT_FILE'
      EXPORTING
        iv_long_file_name      = lv_name
        dir_name               = lv_dir
        pos                    = lv_pos
      IMPORTING
        ev_long_dir_name       = lv_long_dir
        ev_long_file_path      = lv_path
        ev_file_size_long      = lv_size
      EXCEPTIONS
        invalid_eps_subdir     = 1
        sapgparam_failed       = 2
        build_directory_failed = 3
        no_authorization       = 4
        build_path_failed      = 5
        open_failed            = 6
        read_directory_failed  = 7
        read_attributes_failed = 8
        OTHERS                 = 9.
    DATA(lv_subrc) = sy-subrc.
    IF lv_subrc <> 0.
      DATA(lv_code) = SWITCH string( lv_subrc WHEN 4 THEN `NO_AUTHORIZATION` WHEN 7 OR 8 THEN `FILE_NOT_FOUND` ELSE `READ_FAILED` ).
      rs_response = err( iv_id = is_message-id iv_code = lv_code
                         iv_message = |{ lv_name } in DIR_TRANS ({ lv_dir }): { lv_code } { last_message( ) }| ).
      RETURN.
    ENDIF.
    CALL FUNCTION 'EPS_CLOSE_FILE'
      EXPORTING
        iv_long_file_name = lv_name
        iv_long_dir_name  = lv_long_dir
      EXCEPTIONS
        OTHERS            = 1.

    IF lv_size > lv_limit.
      rs_response = err( iv_id = is_message-id iv_code = 'TOO_LARGE'
                         iv_message = |{ lv_name } is { lv_size } bytes, over the { lv_limit }-byte limit| ).
      RETURN.
    ENDIF.

    lv_dataset = lv_path.
    IF lv_offset < lv_size.
      TRY.
          OPEN DATASET lv_dataset FOR INPUT IN BINARY MODE AT POSITION lv_offset.
          IF sy-subrc = 0.
            READ DATASET lv_dataset INTO lv_chunk MAXIMUM LENGTH lv_length.
            CLOSE DATASET lv_dataset.
          ELSE.
            rs_response = err( iv_id = is_message-id iv_code = 'READ_FAILED' iv_message = |{ lv_name } could not be opened| ).
            RETURN.
          ENDIF.
        CATCH cx_root INTO DATA(lx_read).
          CLOSE DATASET lv_dataset.
          rs_response = err( iv_id = is_message-id iv_code = 'READ_FAILED' iv_message = |{ lv_name }: { lx_read->get_text( ) }| ).
          RETURN.
      ENDTRY.
    ENDIF.

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'name' iv_value = CONV #( lv_name ) ) )
      ( |"size":{ lv_size }| )
      ( zcl_vsp_utils=>json_int( iv_key = 'offset' iv_value = lv_offset ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'chunk_b64' iv_value = cl_http_utility=>encode_x_base64( lv_chunk ) ) )
    ) ) ) ).
  ENDMETHOD.


  METHOD request_from_names.
    DATA: lv_cnr  TYPE string,
          lv_csid TYPE string,
          lv_dnr  TYPE string,
          lv_dsid TYPE string.

    CLEAR: ev_request, ev_sid, ev_error.
    IF iv_cofile_name IS INITIAL OR iv_data_name IS INITIAL.
      ev_error = `Both files are required: the cofile K<nr>.<SID> and the data file R<nr>.<SID>`.
      RETURN.
    ENDIF.
    FIND PCRE '^K([A-Z0-9]{6})\.([A-Z0-9]{3})\z' IN iv_cofile_name SUBMATCHES lv_cnr lv_csid.
    IF sy-subrc <> 0.
      ev_error = |Cofile name '{ iv_cofile_name }' is not K<6 alphanum>.<SID>|.
      RETURN.
    ENDIF.
    FIND PCRE '^R([A-Z0-9]{6})\.([A-Z0-9]{3})\z' IN iv_data_name SUBMATCHES lv_dnr lv_dsid.
    IF sy-subrc <> 0.
      ev_error = |Data file name '{ iv_data_name }' is not R<6 alphanum>.<SID>|.
      RETURN.
    ENDIF.
    IF lv_cnr <> lv_dnr OR lv_csid <> lv_dsid.
      ev_error = |{ iv_cofile_name } and { iv_data_name } are not one request's files: number and SID must match|.
      RETURN.
    ENDIF.
    ev_sid = lv_csid.
    ev_request = |{ lv_csid }K{ lv_cnr }|.
  ENDMETHOD.


  METHOD validate_cofile.
    DATA: lv_text   TYPE string,
          lt_lines  TYPE string_table,
          lt_tok    TYPE string_table,
          lv_header TYPE abap_bool,
          lv_steps  TYPE i,
          lv_export TYPE abap_bool,
          lv_lineno TYPE i,
          lv_sys    TYPE string,
          lv_cli    TYPE string,
          lv_nul    TYPE x LENGTH 1 VALUE '00'.

    IF xstrlen( iv_content ) = 0.
      rv_error = `the cofile is empty`.
      RETURN.
    ENDIF.
    IF xstrlen( iv_content ) > c_max_cofile.
      rv_error = |the cofile is over the { c_max_cofile }-byte limit for a cofile|.
      RETURN.
    ENDIF.
    FIND lv_nul IN iv_content IN BYTE MODE.
    IF sy-subrc = 0.
      rv_error = `the cofile contains NUL bytes: it is not a cofile (is it the data file?)`.
      RETURN.
    ENDIF.
    TRY.
        lv_text = cl_abap_codepage=>convert_from( source = iv_content codepage = `UTF-8` ).
      CATCH cx_root.
        rv_error = `the cofile is not text`.
        RETURN.
    ENDTRY.
    FIND PCRE '[\x01-\x08\x0B\x0C\x0E-\x1F]' IN lv_text.
    IF sy-subrc = 0.
      rv_error = `the cofile contains control characters: it is not a cofile`.
      RETURN.
    ENDIF.

    SPLIT lv_text AT cl_abap_char_utilities=>newline INTO TABLE lt_lines.
    LOOP AT lt_lines INTO DATA(lv_line).
      lv_lineno = sy-tabix.
      REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>cr_lf(1) IN lv_line WITH ``.
      REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>horizontal_tab IN lv_line WITH ` `.
      CONDENSE lv_line.
      IF lv_line IS INITIAL.
        CONTINUE.
      ENDIF.
      IF lv_line(1) = '#'.
        CONTINUE.
      ENDIF.
      SPLIT lv_line AT space INTO TABLE lt_tok.

      IF lv_header = abap_false.
        " Owner, request type, target, step and the nine object counts
        " STRF_READ_COFILE reads: thirteen fields at least.
        IF lines( lt_tok ) < 13.
          rv_error = |line { lv_lineno } (the header) has { lines( lt_tok ) } fields; a header has at least 13 (owner, request type, target, step, nine object counts)|.
          RETURN.
        ENDIF.
        FIND PCRE '^[A-Z]\z' IN lt_tok[ 2 ].
        IF sy-subrc <> 0.
          rv_error = |line { lv_lineno } (the header): request type '{ lt_tok[ 2 ] }' is not a single letter|.
          RETURN.
        ENDIF.
        FIND PCRE '^[A-Z0-9/_]{1,20}(\.[0-9]{3})?\z' IN lt_tok[ 3 ].
        IF sy-subrc <> 0.
          rv_error = |line { lv_lineno } (the header): target '{ lt_tok[ 3 ] }' is not a system|.
          RETURN.
        ENDIF.
        FIND PCRE '^[0-3]\z' IN lt_tok[ 4 ].
        IF sy-subrc <> 0.
          rv_error = |line { lv_lineno } (the header): step '{ lt_tok[ 4 ] }' is not one of 0, 1, 2, 3|.
          RETURN.
        ENDIF.
        " Nine object counts follow the step (what STRF_READ_COFILE reads);
        " what comes after them (release, flags, client) is not checked.
        LOOP AT lt_tok INTO DATA(lv_count) FROM 5 TO 13.
          FIND PCRE '^[0-9]+\z' IN lv_count.
          IF sy-subrc <> 0.
            rv_error = |line { lv_lineno } (the header): object count '{ lv_count }' is not a number|.
            RETURN.
          ENDIF.
        ENDLOOP.
        lv_header = abap_true.
        CONTINUE.
      ENDIF.

      IF lines( lt_tok ) < 4.
        rv_error = |line { lv_lineno }: a step line has at least system, function, return code and time|.
        RETURN.
      ENDIF.
      FIND PCRE '^[A-Z0-9/_]{1,20}(\.[0-9]{3})?\z' IN lt_tok[ 1 ].
      DATA(lv_ok) = xsdbool( sy-subrc = 0 ).
      FIND PCRE '^[0-9]+\z' IN lt_tok[ 3 ].
      lv_ok = xsdbool( lv_ok = abap_true AND sy-subrc = 0 ).
      FIND PCRE '^[0-9]{14}\z' IN lt_tok[ 4 ].
      lv_ok = xsdbool( lv_ok = abap_true AND sy-subrc = 0 AND strlen( lt_tok[ 2 ] ) = 1 ).
      IF lv_ok = abap_false.
        rv_error = |line { lv_lineno } is not a step line (<system>[.<client>] <function> <retcode> <YYYYMMDDhhmmss> ...)|.
        RETURN.
      ENDIF.
      lv_steps = lv_steps + 1.
      SPLIT lt_tok[ 1 ] AT '.' INTO lv_sys lv_cli.
      IF lv_sys = iv_sid AND lt_tok[ 2 ] = 'E'.
        lv_export = abap_true.
      ENDIF.
    ENDLOOP.

    IF lv_header = abap_false.
      rv_error = `the cofile has no header line`.
    ELSEIF lv_steps = 0.
      rv_error = `the cofile records no steps: the request was never exported`.
    ELSEIF lv_export = abap_false.
      rv_error = |the cofile records no export (step E) from { iv_sid }, the system its name says it comes from|.
    ENDIF.
  ENDMETHOD.


  METHOD dir_of.
    " The only two directories this class touches, both inside DIR_TRANS.
    rv_dir = COND #( WHEN iv_kind = `cofile` THEN '$TR_COFI' ELSE '$TR_DATA' ).
  ENDMETHOD.


  METHOD file_exists.
    rv_exists = probe_file( EXPORTING iv_subdir = CONV #( dir_of( iv_kind ) ) iv_name = iv_name
                            IMPORTING ev_error = ev_error ).
  ENDMETHOD.


  METHOD probe_file.
    DATA: lv_name    TYPE eps2filnam,
          lv_dirname TYPE eps2filnam,
          lv_mask    TYPE epsf-epsfilnam,
          lt_list    TYPE STANDARD TABLE OF eps2fili WITH DEFAULT KEY.

    CLEAR: ev_error, ev_long_dir.
    lv_name = iv_name.
    CALL FUNCTION 'EPS_GET_DIRECTORY_PATH'
      EXPORTING
        eps_subdir       = iv_subdir
      IMPORTING
        ev_long_dir_name = ev_long_dir
      EXCEPTIONS
        OTHERS           = 1.
    IF sy-subrc <> 0 OR ev_long_dir IS INITIAL.
      ev_error = |DIR_TRANS ({ iv_subdir }) could not be resolved: { last_message( ) }|.
      RETURN.
    ENDIF.
    CALL FUNCTION 'EPS_GET_FILE_ATTRIBUTES'
      EXPORTING
        iv_long_file_name      = lv_name
        iv_long_dir_name       = ev_long_dir
      EXCEPTIONS
        read_directory_failed  = 1
        read_attributes_failed = 2
        OTHERS                 = 3.
    IF sy-subrc = 0.
      rv_exists = abap_true.
      RETURN.
    ENDIF.

    " No attributes: missing, or unreadable? Only a listing of the
    " directory without the file says missing.
    lv_dirname = ev_long_dir.
    lv_mask = iv_name.
    CALL FUNCTION 'EPS2_GET_DIRECTORY_LISTING'
      EXPORTING
        iv_dir_name            = lv_dirname
        file_mask              = lv_mask
      TABLES
        dir_list               = lt_list
      EXCEPTIONS
        invalid_eps_subdir     = 1
        sapgparam_failed       = 2
        build_directory_failed = 3
        no_authorization       = 4
        read_directory_failed  = 5
        too_many_read_errors   = 6
        empty_directory_list   = 7
        OTHERS                 = 8.
    DATA(lv_subrc) = sy-subrc.
    IF lv_subrc = 7 OR ( lv_subrc = 0 AND NOT line_exists( lt_list[ name = lv_name ] ) ).
      rv_exists = abap_false.
    ELSEIF lv_subrc = 0.
      ev_error = |{ iv_name } is listed in { ev_long_dir } but its attributes cannot be read|.
    ELSE.
      ev_error = |{ ev_long_dir } cannot be listed (exception { lv_subrc }) { last_message( ) }|.
    ENDIF.
  ENDMETHOD.


  METHOD write_file.
    DATA: lv_dir      TYPE epsf-epsdirnam,
          lv_name     TYPE eps2filnam,
          lv_long_dir TYPE eps2path,
          lv_path     TYPE eps2path,
          lv_size     TYPE epsf-epsfilsiz,
          lt_buf      TYPE STANDARD TABLE OF tbl8000 WITH DEFAULT KEY,
          ls_buf      TYPE tbl8000,
          lv_off      TYPE i,
          lv_n        TYPE i,
          lv_last     TYPE i,
          lv_records  TYPE i.

    CLEAR: ev_opened, ev_path, ev_error.
    lv_dir = dir_of( iv_kind ).
    lv_name = iv_name.

    " overwrite_mode space: SAP refuses a non-empty existing file; the caller
    " has already refused any existing file, empty or not.
    TRY.
        CALL FUNCTION 'EPS_OPEN_OUTPUT_FILE'
          EXPORTING
            iv_long_file_name      = lv_name
            dir_name               = lv_dir
            overwrite_mode         = space
          IMPORTING
            ev_long_dir_name       = lv_long_dir
            ev_long_file_path      = lv_path
          EXCEPTIONS
            invalid_eps_subdir     = 1
            sapgparam_failed       = 2
            build_directory_failed = 3
            no_authorization       = 4
            build_path_failed      = 5
            open_failed            = 6
            file_already_exists    = 7
            OTHERS                 = 8.
        DATA(lv_subrc) = sy-subrc.
      CATCH cx_root INTO DATA(lx_open).
        ev_error = |open failed: { lx_open->get_text( ) }|.
        RETURN.
    ENDTRY.
    IF lv_subrc <> 0.
      ev_error = SWITCH #( lv_subrc
        WHEN 4 THEN |no authorization (S_CTS_ADMI CTS_ADMFCT EPS1, or S_DATASET for the transport directory) { last_message( ) }|
        WHEN 7 THEN `the file already exists`
        WHEN 6 THEN |the file could not be opened for writing in DIR_TRANS ({ lv_dir }): does the directory exist, writable by the SAP system's OS user? { last_message( ) }|
        ELSE |EPS_OPEN_OUTPUT_FILE failed (exception { lv_subrc }) { last_message( ) }| ).
      RETURN.
    ENDIF.
    ev_opened = abap_true.
    ev_path = lv_path.

    DATA(lv_len) = xstrlen( iv_content ).
    WHILE lv_off < lv_len.
      lv_n = nmin( val1 = 8000 val2 = lv_len - lv_off ).
      CLEAR ls_buf.
      ls_buf-line = iv_content+lv_off(lv_n).
      APPEND ls_buf TO lt_buf.
      lv_last = lv_n.
      lv_off = lv_off + lv_n.
    ENDWHILE.
    lv_records = lines( lt_buf ).

    TRY.
        CALL FUNCTION 'EPS_WRITE_BLOCK'
          EXPORTING
            iv_long_file_path  = lv_path
            number_of_records  = lv_records
            last_record_length = lv_last
          TABLES
            eps_buffer         = lt_buf
          EXCEPTIONS
            write_failure      = 1
            OTHERS             = 2.
        lv_subrc = sy-subrc.
      CATCH cx_root INTO DATA(lx_write).
        lv_subrc = 9.
        ev_error = |write failed: { lx_write->get_text( ) }|.
    ENDTRY.

    CALL FUNCTION 'EPS_CLOSE_FILE'
      EXPORTING
        iv_long_file_name      = lv_name
        iv_long_dir_name       = lv_long_dir
      IMPORTING
        file_size              = lv_size
      EXCEPTIONS
        build_path_failed      = 1
        read_directory_failed  = 2
        read_attributes_failed = 3
        OTHERS                 = 4.
    DATA(lv_close_subrc) = sy-subrc.

    IF lv_subrc <> 0.
      IF ev_error IS INITIAL.
        ev_error = |EPS_WRITE_BLOCK failed (exception { lv_subrc })|.
      ENDIF.
      RETURN.
    ENDIF.
    IF lv_close_subrc <> 0.
      ev_error = |EPS_CLOSE_FILE failed (exception { lv_close_subrc })|.
      RETURN.
    ENDIF.
    IF lv_size <> lv_len.
      ev_error = |{ lv_size } bytes on disk after writing { lv_len }|.
      RETURN.
    ENDIF.
  ENDMETHOD.


  METHOD delete_file.
    DATA: lv_dir     TYPE epsf-epsdirnam,
          lv_name    TYPE eps2filnam,
          lv_content TYPE xstring.

    IF iv_sha IS NOT INITIAL.
      read_dir_file( EXPORTING iv_kind = iv_kind iv_name = iv_name IMPORTING ev_content = lv_content ev_error = rv_error ).
      IF rv_error IS NOT INITIAL.
        rv_error = |{ iv_name } kept: { rv_error }|.
        RETURN.
      ENDIF.
      IF sha256( lv_content ) <> to_upper( iv_sha ).
        rv_error = |{ iv_name } kept: it is not the file this upload wrote (SHA-256 differs)|.
        RETURN.
      ENDIF.
    ENDIF.
    lv_dir = dir_of( iv_kind ).
    lv_name = iv_name.
    CALL FUNCTION 'EPS_DELETE_FILE'
      EXPORTING
        iv_long_file_name  = lv_name
        dir_name           = lv_dir
      EXCEPTIONS
        invalid_eps_subdir = 1
        sapgparam_failed   = 2
        build_directory_failed = 3
        no_authorization   = 4
        build_path_failed  = 5
        delete_failed      = 6
        OTHERS             = 7.
    IF sy-subrc <> 0.
      rv_error = |{ iv_name } could not be deleted (exception { sy-subrc }) { last_message( ) }|.
    ENDIF.
  ENDMETHOD.


  METHOD lock_request.
    DATA lv_trkorr TYPE e070-trkorr.
    lv_trkorr = iv_request.
    CALL FUNCTION 'ENQUEUE_E_TRKORR'
      EXPORTING
        trkorr         = lv_trkorr
        _scope         = '1'
        _wait          = iv_wait
      EXCEPTIONS
        foreign_lock   = 1
        system_failure = 2
        OTHERS         = 3.
    IF sy-subrc = 1.
      rv_error = |{ lv_trkorr } is locked by { sy-msgv1 } (lock object E_TRKORR); nothing was done|.
    ELSEIF sy-subrc <> 0.
      rv_error = |{ lv_trkorr } could not be locked (exception { sy-subrc }) { last_message( ) }; nothing was done|.
    ENDIF.
  ENDMETHOD.


  METHOD unlock_request.
    DATA lv_trkorr TYPE e070-trkorr.
    lv_trkorr = iv_request.
    CALL FUNCTION 'DEQUEUE_E_TRKORR'
      EXPORTING
        trkorr = lv_trkorr
        _scope = '1'.
  ENDMETHOD.


  METHOD read_buffer.
    DATA: lv_cmd    TYPE stpa-cmdstring,
          lv_rc     TYPE stpa-retcode,
          lv_msg    TYPE stpa-message,
          lv_system TYPE stpa-sysname.

    CLEAR: et_buffer, et_stdout, ev_cmd, ev_rc, ev_msg, ev_error.
    lv_system = sy-sysid.
    " Read-only: no lock clearing, no lock reading, nothing but the list.
    CALL FUNCTION 'TMS_TP_SHOW_BUFFER'
      EXPORTING
        iv_system_name     = lv_system
      IMPORTING
        ev_tp_cmd_strg     = lv_cmd
        ev_tp_ret_code     = lv_rc
        ev_tp_message      = lv_msg
      TABLES
        tt_stdout          = et_stdout
        tt_buffer          = et_buffer
      EXCEPTIONS
        permission_denied  = 1
        tp_call_failed     = 2
        tp_interface_error = 3
        tp_reported_error  = 4
        OTHERS             = 5.
    DATA(lv_subrc) = sy-subrc.
    ev_cmd = lv_cmd.
    ev_rc = lv_rc.
    ev_msg = lv_msg.
    IF lv_subrc <> 0.
      ev_error = |TMS_TP_SHOW_BUFFER failed (exception { lv_subrc }, tp rc { lv_rc }): { last_message( ) } { lv_msg }|.
    ENDIF.
  ENDMETHOD.


  METHOD request_parts.
    CLEAR: ev_sid, ev_number.
    FIND PCRE '^([A-Z0-9]{3})K([A-Z0-9]{6})\z' IN iv_request SUBMATCHES ev_sid ev_number.
    rv_ok = xsdbool( sy-subrc = 0 ).
  ENDMETHOD.


  METHOD sha256.
    TRY.
        cl_abap_message_digest=>calculate_hash_for_raw(
          EXPORTING if_algorithm  = 'SHA256'
                    if_data       = iv_data
          IMPORTING ef_hashstring = DATA(lv_hash) ).
        rv_hash = to_upper( lv_hash ).
      CATCH cx_abap_message_digest.
        CLEAR rv_hash.
    ENDTRY.
  ENDMETHOD.


  METHOD sha_b64.
    DATA lv_raw TYPE xstring.
    TRY.
        lv_raw = to_upper( iv_hex ).
        rv_b64 = cl_http_utility=>encode_x_base64( lv_raw ).
      CATCH cx_root.
        CLEAR rv_b64.
    ENDTRY.
  ENDMETHOD.


  METHOD last_message.
    IF sy-msgid IS INITIAL.
      RETURN.
    ENDIF.
    MESSAGE ID sy-msgid TYPE 'S' NUMBER sy-msgno
      WITH sy-msgv1 sy-msgv2 sy-msgv3 sy-msgv4 INTO rv_text.
  ENDMETHOD.


  METHOD stdout_json.
    DATA lt_items TYPE string_table.
    LOOP AT it_stdout INTO DATA(ls_line).
      APPEND |"{ zcl_vsp_utils=>escape_json( CONV #( ls_line-line ) ) }"| TO lt_items.
    ENDLOOP.
    rv_json = zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_items ) ).
  ENDMETHOD.


  METHOD err.
    rs_response = zcl_vsp_utils=>build_error( iv_id = iv_id iv_code = iv_code iv_message = iv_message ).
  ENDMETHOD.

ENDCLASS.
