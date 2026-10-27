"! <p class="shorttext synchronized">VSP Git Service - abapGit Integration</p>
"! Domain "git": abapGit serialization through the abapGit installed on this
"! system (export), and the import of an abapGit offline zip into a package.
"!
"! import_zip: the zip arrives in chunks (begin with its size, SHA-256 and
"! the import's parameters; chunk; commit; abort). commit checks size and
"! SHA-256 and hands the zip to background job ZVSP_GIT_IMPORT -- abapGit's
"! deserialize activates, may run for minutes, and must not run inside an
"! ABAP Push Channel. The job's parameters are a protected variant of
"! ZVSP_GIT_IMPORT per job (an APC session may not SUBMIT), holding the
"! package and the SHA-256 of the zip and of the import's parameters; zip and
"! parameters wait in INDX(ZV) under the job's number. The job checks both
"! before it does anything, and stores its result in INDX(ZV); import_status
"! reads it, and AMC ZVSP_GIT /import pushes the outcome to the WebSocket
"! that started it.
"!
"! The import honours abapGit's deserialize checks: an object that exists is
"! never overwritten without overwrite = true; requirements or dependencies
"! not met, an object of another package, a package move, potential data
"! loss and an unsupported object type refuse the import; a local object not
"! in the zip is never deleted; every package the zip maps to must be one of
"! the packages the caller listed (the ones it checked against its own
"! package whitelist). Rules pinned by embedded/abap/git_service_test.go.
CLASS zcl_vsp_git_service DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    INTERFACES zif_vsp_service.

    "! The zip, in bytes (20 MB).
    CONSTANTS c_max_zip TYPE i VALUE 20971520.
    "! One decoded chunk, in bytes.
    CONSTANTS c_max_chunk TYPE i VALUE 1048576.
    "! The zip's entries, and their declared uncompressed size in bytes
    "! (200 MB): checked from the zip's directory before anything is
    "! decompressed. vsp checks the same limits before it sends the zip.
    CONSTANTS c_max_entries TYPE i VALUE 50000.
    CONSTANTS c_max_unzipped TYPE int8 VALUE 209715200.
    "! The background job (and its program) that runs the import.
    CONSTANTS c_job_name TYPE tbtcjob-jobname VALUE 'ZVSP_GIT_IMPORT'.
    "! AMC application and channel the job publishes its outcome on; the
    "! extension is the WebSocket session that started the import.
    CONSTANTS c_amc_app TYPE amc_application_id VALUE 'ZVSP_GIT'.
    CONSTANTS c_amc_channel TYPE string VALUE `/import`.
    "! The objects one object_versions call may name.
    CONSTANTS c_max_versions TYPE i VALUE 500.

    "! The background step. Checks that it runs as its own job with its own
    "! protected variant, that the zip and the parameters waiting in INDX
    "! have the SHA-256 the variant names, then imports and stores the
    "! result. Outside a ZVSP_GIT_IMPORT job it does nothing.
    CLASS-METHODS run_job
      IMPORTING iv_package  TYPE csequence
                iv_zip_sha  TYPE csequence
                iv_meta_sha TYPE csequence
                iv_push_id  TYPE csequence OPTIONAL.

  PRIVATE SECTION.
    TYPES:
      BEGIN OF ty_object_ref,
        type TYPE trobjtype,
        name TYPE sobj_name,
      END OF ty_object_ref,
      ty_object_refs TYPE STANDARD TABLE OF ty_object_ref WITH DEFAULT KEY.

    TYPES:
      BEGIN OF ty_file_info,
        path TYPE string,
        size TYPE i,
      END OF ty_file_info,
      ty_files_info TYPE STANDARD TABLE OF ty_file_info WITH DEFAULT KEY.

    TYPES:
      "! What an import is asked to do. Bound to the job by its SHA-256.
      BEGIN OF ty_import_params,
        package   TYPE devclass,
        repo_name TYPE string,
        overwrite TYPE abap_bool,
        transport TYPE trkorr,
        packages  TYPE string,
      END OF ty_import_params,
      BEGIN OF ty_upload,
        id     TYPE string,
        size   TYPE i,
        sha    TYPE string,
        params TYPE ty_import_params,
        data   TYPE xstring,
      END OF ty_upload,
      BEGIN OF ty_log_line,
        type     TYPE string,
        text     TYPE string,
        obj_type TYPE string,
        obj_name TYPE string,
      END OF ty_log_line,
      tt_log_line TYPE STANDARD TABLE OF ty_log_line WITH DEFAULT KEY,
      BEGIN OF ty_tadir_row,
        object   TYPE trobjtype,
        obj_name TYPE sobj_name,
        devclass TYPE devclass,
        created  TYPE abap_bool,
      END OF ty_tadir_row,
      tt_tadir_row TYPE STANDARD TABLE OF ty_tadir_row WITH DEFAULT KEY,
      BEGIN OF ty_decision,
        obj_type TYPE string,
        obj_name TYPE string,
        devclass TYPE string,
        action   TYPE string,
        decision TYPE string,
      END OF ty_decision,
      tt_decision TYPE STANDARD TABLE OF ty_decision WITH DEFAULT KEY,
      "! outcome: imported, imported_with_errors, refused or failed.
      BEGIN OF ty_result,
        outcome      TYPE string,
        code         TYPE string,
        message      TYPE string,
        package      TYPE string,
        repo_key     TYPE string,
        repo_name    TYPE string,
        repo_created TYPE abap_bool,
        "! The target package did not exist and the import created it.
        package_created TYPE abap_bool,
        transport    TYPE string,
        log          TYPE tt_log_line,
        info_count   TYPE i,
        tadir        TYPE tt_tadir_row,
        decisions    TYPE tt_decision,
      END OF ty_result,
      tt_devclass TYPE STANDARD TABLE OF devclass WITH DEFAULT KEY.

    "! The zip being assembled in this session; one at a time.
    DATA ms_upload TYPE ty_upload.

    METHODS handle_get_types
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_export
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_import_zip
      IMPORTING iv_session_id      TYPE string
                is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS import_begin
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS import_chunk
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    "! Checks size and SHA-256 and starts the import job.
    METHODS import_commit
      IMPORTING iv_session_id      TYPE string
                is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    "! Discards the upload in progress -- only the one the caller names.
    METHODS import_abort
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    "! The outcome of an import job: pending, done (with its result), failed
    "! or unknown. Read-only; only the caller's own jobs.
    METHODS handle_import_status
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    "! Deletes the abapGit repository registered for exactly this package:
    "! its row, not its objects -- and only an offline repository of a
    "! package that is empty (no object but its own entry, no subpackage).
    "! An online repository (URL, branch, settings) is never unregistered.
    METHODS handle_delete_repo
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    "! A package's TADIR objects, its subpackages and its abapGit repository.
    "! Read-only.
    METHODS handle_package_objects
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    "! The version of objects of a package. Read-only. For each "TYPE NAME"
    "! of objects (comma-separated, 1 to c_max_versions): its TADIR package,
    "! and -- only when that is this package -- its stamp (object_stamp)
    "! and, with sha256 = "true", the SHA-256 of its abapGit serialisation
    "! (object_sha256). vsp reads them before it decides, and again under
    "! the ADT lock it deletes with.
    METHODS handle_object_versions
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    "! v2:<TABLES>:<YYYYMMDDHHMMSS>:<ROWS>:<DIGEST> -- the newest change date
    "! and time over every dated version row of the object (active and
    "! inactive) and the number of those rows, and the first 16 hex digits of
    "! a SHA-256 over the rows of its tables that carry no date. Per type:
    "! CLAS/INTF REPOSRC (every include of the pool but CS, which is
    "! regenerated without the source changing) and REPOTEXT of the pool;
    "! digest SEOCLASSDF, SEOCLASSTX, SEOCOMPOTX. PROG REPOSRC, REPOTEXT,
    "! D020S (DGEN, TGEN). TABL DD02L, DD09L, DD12L; digest DD02T, DD35L,
    "! TDDAT. DTEL DD04L; digest DD04T. DOMA DD01L; digest DD01T, DD07L,
    "! DD07T. TTYP DD40L; digest DD40T. DDLS DDDDLSRC; digest DDDDLSRCT.
    "! Another type: no stamp, and ev_error says so. No dated row: no stamp.
    CLASS-METHODS object_stamp
      IMPORTING iv_type  TYPE trobjtype
                iv_name  TYPE sobj_name
      EXPORTING ev_stamp TYPE string
                ev_error TYPE string.

    "! SHA-256 (lower-case hex) of the object's abapGit serialisation in its
    "! original language (TADIR-MASTERLANG, E when empty) only: of the
    "! UTF-8 text of the lines "<file name>=<lower-case hex SHA-256 of the
    "! file>", one per file, sorted, joined by LF, without a final LF.
    "! Whether the object, or a part of it, has an inactive version: a row
    "! of the inactive worklist (DWINACTIV) for it or one of its parts.
    CLASS-METHODS object_inactive
      IMPORTING iv_type            TYPE trobjtype
                iv_name            TYPE sobj_name
      RETURNING VALUE(rv_inactive) TYPE abap_bool.

    CLASS-METHODS object_sha256
      IMPORTING iv_type     TYPE trobjtype
                iv_name     TYPE sobj_name
                iv_devclass TYPE devclass
                iv_language TYPE spras
      EXPORTING ev_sha256   TYPE string
                ev_files    TYPE i
                ev_error    TYPE string.

    "! The import itself: offline repository, deserialize checks, decisions,
    "! deserialize. Runs in the background job.
    CLASS-METHODS do_import
      IMPORTING iv_zip           TYPE xstring
                is_params        TYPE ty_import_params
      RETURNING VALUE(rs_result) TYPE ty_result.

    "! Applies the import's policy to abapGit's checks: sets the decisions
    "! and the transport, or says why the import is refused (ev_code).
    "! iv_new_repo: the repository was created by this import (the package
    "! had none).
    CLASS-METHODS evaluate_checks
      IMPORTING is_params    TYPE ty_import_params
                iv_new_repo  TYPE abap_bool
      EXPORTING ev_code      TYPE string
                ev_message   TYPE string
                et_decisions TYPE tt_decision
      CHANGING  cs_checks    TYPE zif_abapgit_definitions=>ty_deserialize_checks.

    "! Every file of the zip must map to a package the caller listed.
    CLASS-METHODS check_packages
      IMPORTING ii_repo           TYPE REF TO zif_abapgit_repo
                it_files          TYPE zif_abapgit_git_definitions=>ty_files_tt
                is_params         TYPE ty_import_params
      RETURNING VALUE(rv_message) TYPE string
      RAISING   zcx_abapgit_exception.

    "! zif_abapgit_repo_srv~new_offline, whose name parameter was iv_url in
    "! older abapGit releases and is iv_name in newer ones.
    CLASS-METHODS new_offline_repo
      IMPORTING iv_name        TYPE string
                iv_package     TYPE devclass
      RETURNING VALUE(ri_repo) TYPE REF TO zif_abapgit_repo
      RAISING   zcx_abapgit_exception
                cx_sy_dyn_call_error.

    CLASS-METHODS package_tadir
      IMPORTING it_packages    TYPE tt_devclass
      RETURNING VALUE(rt_rows) TYPE tt_tadir_row.

    CLASS-METHODS split_packages
      IMPORTING iv_packages        TYPE string
      RETURNING VALUE(rt_packages) TYPE tt_devclass.

    CLASS-METHODS valid_package
      IMPORTING iv_package   TYPE string
      RETURNING VALUE(rv_ok) TYPE abap_bool.

    CLASS-METHODS result_json
      IMPORTING is_result      TYPE ty_result
      RETURNING VALUE(rv_json) TYPE string.

    "! Schedules ZVSP_GIT_IMPORT for one import: zip and parameters into
    "! INDX(ZV) under the job's number, a protected variant naming the
    "! package and their SHA-256.
    CLASS-METHODS start_job
      IMPORTING iv_zip      TYPE xstring
                is_params   TYPE ty_import_params
                iv_push_id  TYPE string OPTIONAL
      EXPORTING ev_jobcount TYPE string
                ev_error    TYPE string.

    "! Deletes what ended jobs left: their variants and zips, and results
    "! older than a week; and this user's jobs that were scheduled but never
    "! released, a day on.
    CLASS-METHODS housekeeping.

    "! A job that will not run: the job, its variant and its zip go.
    CLASS-METHODS drop_job
      IMPORTING iv_jobcount TYPE csequence.

    "! The zip's directory within c_max_entries and c_max_unzipped, with
    "! exactly one .abapgit.xml at its root; else a refusal (ev_code).
    CLASS-METHODS zip_limits
      IMPORTING iv_zip     TYPE xstring
      EXPORTING ev_code    TYPE string
                ev_message TYPE string.

    CLASS-METHODS store_result
      IMPORTING iv_jobcount TYPE csequence
                is_result   TYPE ty_result.

    CLASS-METHODS job_log
      IMPORTING is_result TYPE ty_result.

    CLASS-METHODS publish
      IMPORTING is_result   TYPE ty_result
                iv_jobcount TYPE csequence
                iv_push_id  TYPE csequence.

    CLASS-METHODS sha256
      IMPORTING iv_data        TYPE xstring
      RETURNING VALUE(rv_hash) TYPE string.

    "! A SHA-256 in hex, as base64 (44 characters).
    CLASS-METHODS sha_b64
      IMPORTING iv_hex        TYPE csequence
      RETURNING VALUE(rv_b64) TYPE string.

    "! SHA-256 (base64) of the import's parameters.
    CLASS-METHODS meta_sha
      IMPORTING is_params     TYPE ty_import_params
      RETURNING VALUE(rv_b64) TYPE string.

    CLASS-METHODS clean
      IMPORTING iv_text        TYPE csequence
      RETURNING VALUE(rv_text) TYPE string.

    CLASS-METHODS last_message
      RETURNING VALUE(rv_text) TYPE string.

    CLASS-METHODS err
      IMPORTING iv_id              TYPE string
                iv_code            TYPE string
                iv_message         TYPE string
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS get_package_objects
      IMPORTING iv_package             TYPE devclass
                iv_include_subpackages TYPE abap_bool DEFAULT abap_true
      RETURNING VALUE(rt_tadir)        TYPE zif_abapgit_definitions=>ty_tadir_tt.

    METHODS serialize_objects
      IMPORTING it_tadir             TYPE zif_abapgit_definitions=>ty_tadir_tt
      EXPORTING ev_zip_base64        TYPE string
                et_files             TYPE ty_files_info
      RAISING   zcx_abapgit_exception.

    METHODS base64_to_xstring
      IMPORTING iv_base64          TYPE string
      RETURNING VALUE(rv_xstring)  TYPE xstring.

    METHODS xstring_to_base64
      IMPORTING iv_xstring        TYPE xstring
      RETURNING VALUE(rv_base64)  TYPE string.

    METHODS json_escape
      IMPORTING iv_string         TYPE string
      RETURNING VALUE(rv_escaped) TYPE string.

    METHODS build_json_response
      IMPORTING iv_id            TYPE string
                iv_success       TYPE abap_bool
                iv_data          TYPE string OPTIONAL
                iv_error         TYPE string OPTIONAL
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

ENDCLASS.


CLASS zcl_vsp_git_service IMPLEMENTATION.

  METHOD zif_vsp_service~get_domain.
    rv_domain = 'git'.
  ENDMETHOD.

  METHOD zif_vsp_service~handle_message.
    CASE is_message-action.
      WHEN 'getTypes' OR 'get_types'.
        rs_response = handle_get_types( is_message ).
      WHEN 'export'.
        rs_response = handle_export( is_message ).
      WHEN 'import_zip'.
        rs_response = handle_import_zip( iv_session_id = iv_session_id is_message = is_message ).
      WHEN 'import_status'.
        rs_response = handle_import_status( is_message ).
      WHEN 'delete_repo'.
        rs_response = handle_delete_repo( is_message ).
      WHEN 'package_objects'.
        rs_response = handle_package_objects( is_message ).
      WHEN 'object_versions'.
        rs_response = handle_object_versions( is_message ).
      WHEN 'import' OR 'validate'.
        rs_response = err( iv_id = is_message-id iv_code = 'UNKNOWN_ACTION'
                           iv_message = |Action '{ is_message-action }' is gone: an abapGit zip is imported with import_zip (vsp git import-zip)| ).
      WHEN OTHERS.
        rs_response = build_json_response(
          iv_id      = is_message-id
          iv_success = abap_false
          iv_error   = |Unknown action: { is_message-action }|
        ).
    ENDCASE.
  ENDMETHOD.

  METHOD zif_vsp_service~on_disconnect.
    " A zip not yet handed to a job is dropped with the session.
    CLEAR ms_upload.
  ENDMETHOD.

  METHOD handle_get_types.
    DATA lv_types_json TYPE string.

    TRY.
        " The table type moved between abapGit releases: older ones declare it
        " inside ZCL_ABAPGIT_OBJECTS, newer ones in ZIF_ABAPGIT_OBJECTS. Naming
        " either spelling makes this class refuse to compile on half the
        " systems it is installed on — and because a syntax error in one class
        " takes the whole APC application down, that failure is not local to
        " this method: it costs the debugger, RFC over the tunnel and
        " RunReport. Inferring the type asks the method what it returns and
        " works on both.
        DATA(lt_types) = zcl_abapgit_objects=>supported_list( ).

        " Build JSON array of types
        LOOP AT lt_types INTO DATA(lv_type).
          IF lv_types_json IS NOT INITIAL.
            lv_types_json = |{ lv_types_json },|.
          ENDIF.
          lv_types_json = |{ lv_types_json }"{ lv_type }"|.
        ENDLOOP.

        DATA(lv_data) = |\{"count":{ lines( lt_types ) },"types":[{ lv_types_json }]\}|.

        rs_response = build_json_response(
          iv_id      = is_message-id
          iv_success = abap_true
          iv_data    = lv_data
        ).

      CATCH cx_root INTO DATA(lx_error).
        rs_response = build_json_response(
          iv_id      = is_message-id
          iv_success = abap_false
          iv_error   = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.

  METHOD handle_export.
    DATA: lt_packages    TYPE STANDARD TABLE OF devclass,
          lt_objects     TYPE ty_object_refs,
          lt_tadir       TYPE zif_abapgit_definitions=>ty_tadir_tt,
          lv_zip_base64  TYPE string,
          lt_files       TYPE ty_files_info,
          lv_include_sub TYPE abap_bool.

    TRY.
        " Parse packages from params
        DATA(lv_params) = is_message-params.
        IF lv_params IS INITIAL.
          lv_params = '{}'. " Default empty
        ENDIF.

        " Extract package names from JSON params
        " Format: {"packages":["$PKG1","$PKG2"],"includeSubpackages":true}
        DATA lv_pkg TYPE devclass.

        " Check for packages array
        FIND PCRE '"packages"\s*:\s*\[([^\]]*)\]' IN lv_params SUBMATCHES DATA(lv_pkgs_str).
        IF sy-subrc = 0.
          " Parse comma-separated quoted package names
          FIND ALL OCCURRENCES OF PCRE '"([^"]+)"' IN lv_pkgs_str
            RESULTS DATA(lt_matches).

          LOOP AT lt_matches INTO DATA(ls_match).
            DATA(lv_off) = ls_match-submatches[ 1 ]-offset.
            DATA(lv_len) = ls_match-submatches[ 1 ]-length.
            lv_pkg = lv_pkgs_str+lv_off(lv_len).
            TRANSLATE lv_pkg TO UPPER CASE.
            APPEND lv_pkg TO lt_packages.
          ENDLOOP.
        ENDIF.

        " Check for includeSubpackages flag
        FIND PCRE '"includeSubpackages"\s*:\s*(true|false)' IN lv_params SUBMATCHES DATA(lv_sub_flag).
        lv_include_sub = xsdbool( lv_sub_flag = 'true' OR lv_sub_flag IS INITIAL ).

        " Check for objects array (alternative to packages)
        FIND PCRE '"objects"\s*:\s*\[([^\]]*)\]' IN lv_params SUBMATCHES DATA(lv_objs_str).
        IF sy-subrc = 0 AND lt_packages IS INITIAL.
          " Parse objects like: {"type":"CLAS","name":"ZCL_TEST"}
          FIND ALL OCCURRENCES OF PCRE '\{"type":"([^"]+)","name":"([^"]+)"\}' IN lv_objs_str
            RESULTS DATA(lt_obj_matches).

          LOOP AT lt_obj_matches INTO DATA(ls_obj_match).
            DATA(lv_type_off) = ls_obj_match-submatches[ 1 ]-offset.
            DATA(lv_type_len) = ls_obj_match-submatches[ 1 ]-length.
            DATA(lv_name_off) = ls_obj_match-submatches[ 2 ]-offset.
            DATA(lv_name_len) = ls_obj_match-submatches[ 2 ]-length.
            DATA(ls_obj) = VALUE ty_object_ref(
              type = lv_objs_str+lv_type_off(lv_type_len)
              name = lv_objs_str+lv_name_off(lv_name_len)
            ).
            APPEND ls_obj TO lt_objects.
          ENDLOOP.
        ENDIF.

        " Collect objects from packages
        LOOP AT lt_packages INTO lv_pkg.
          APPEND LINES OF get_package_objects(
            iv_package             = lv_pkg
            iv_include_subpackages = lv_include_sub
          ) TO lt_tadir.
        ENDLOOP.

        " Or collect from object list
        LOOP AT lt_objects INTO DATA(ls_object).
          DATA(ls_tadir_entry) = zcl_abapgit_factory=>get_tadir( )->read_single(
            iv_object   = ls_object-type
            iv_obj_name = ls_object-name
          ).
          IF ls_tadir_entry IS NOT INITIAL.
            APPEND ls_tadir_entry TO lt_tadir.
          ENDIF.
        ENDLOOP.

        IF lt_tadir IS INITIAL.
          rs_response = build_json_response(
            iv_id      = is_message-id
            iv_success = abap_false
            iv_error   = 'No objects found to export'
          ).
          RETURN.
        ENDIF.

        " Serialize to ZIP
        serialize_objects(
          EXPORTING it_tadir      = lt_tadir
          IMPORTING ev_zip_base64 = lv_zip_base64
                    et_files      = lt_files
        ).

        " Build files JSON
        DATA lv_files_json TYPE string.
        LOOP AT lt_files INTO DATA(ls_file).
          IF lv_files_json IS NOT INITIAL.
            lv_files_json = |{ lv_files_json },|.
          ENDIF.
          lv_files_json = |{ lv_files_json }\{"path":"{ ls_file-path }","size":{ ls_file-size }\}|.
        ENDLOOP.

        DATA(lv_data) = |\{"objectCount":{ lines( lt_tadir ) },"fileCount":{ lines( lt_files ) },"zipBase64":"{ lv_zip_base64 }","files":[{ lv_files_json }]\}|.

        rs_response = build_json_response(
          iv_id      = is_message-id
          iv_success = abap_true
          iv_data    = lv_data
        ).

      CATCH zcx_abapgit_exception cx_root INTO DATA(lx_error).
        rs_response = build_json_response(
          iv_id      = is_message-id
          iv_success = abap_false
          iv_error   = lx_error->get_text( )
        ).
    ENDTRY.
  ENDMETHOD.


  METHOD handle_import_zip.
    DATA(lv_step) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'step' ).
    CASE lv_step.
      WHEN 'begin'.
        rs_response = import_begin( is_message ).
      WHEN 'chunk'.
        rs_response = import_chunk( is_message ).
      WHEN 'commit'.
        rs_response = import_commit( iv_session_id = iv_session_id is_message = is_message ).
      WHEN 'abort'.
        rs_response = import_abort( is_message ).
      WHEN OTHERS.
        rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                           iv_message = |step must be begin, chunk, commit or abort, not '{ lv_step }'| ).
    ENDCASE.
  ENDMETHOD.


  METHOD import_begin.
    DATA: lv_uuid   TYPE sysuuid_c32,
          ls_params TYPE ty_import_params.

    " One upload per session at a time: a second begin must not throw away
    " an upload that is still being sent (an MCP server shares its
    " WebSocket between calls). It ends with commit, abort or disconnect.
    IF ms_upload-id IS NOT INITIAL.
      rs_response = err( iv_id = is_message-id iv_code = 'UPLOAD_IN_PROGRESS'
                         iv_message = |Upload { ms_upload-id } is in progress in this session; commit or abort it first. Nothing was imported.| ).
      RETURN.
    ENDIF.
    DATA(lv_params) = is_message-params.
    DATA(lv_size) = zcl_vsp_utils=>extract_param_int( iv_params = lv_params iv_name = 'size' ).
    DATA(lv_sha) = to_upper( zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'sha256' ) ).
    DATA(lv_package) = to_upper( condense( zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'package' ) ) ).
    DATA(lv_repo_name) = condense( zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'repo_name' ) ).
    DATA(lv_overwrite) = zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'overwrite' ).
    DATA(lv_transport) = to_upper( condense( zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'transport' ) ) ).
    DATA(lv_packages) = to_upper( zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'packages' ) ).

    IF valid_package( lv_package ) = abap_false.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                         iv_message = |package '{ lv_package }' is not a package name| ).
      RETURN.
    ENDIF.
    IF lv_repo_name IS INITIAL OR strlen( lv_repo_name ) > 60.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                         iv_message = `repo_name is required, at most 60 characters` ).
      RETURN.
    ENDIF.
    FIND PCRE '[\x00-\x1F]' IN lv_repo_name.
    IF sy-subrc = 0.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                         iv_message = `repo_name may not contain control characters` ).
      RETURN.
    ENDIF.
    IF lv_overwrite <> 'true' AND lv_overwrite <> 'false'.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                         iv_message = `overwrite must be "true" or "false"` ).
      RETURN.
    ENDIF.
    IF lv_transport IS NOT INITIAL.
      FIND PCRE '^[A-Z0-9]{3}K[A-Z0-9]{6}\z' IN lv_transport.
      IF sy-subrc <> 0.
        rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                           iv_message = |transport '{ lv_transport }' is not <SID>K<6 alphanum>| ).
        RETURN.
      ENDIF.
    ENDIF.
    " The packages the caller checked against its whitelist: the target
    " among them, every one a package name.
    DATA(lt_packages) = split_packages( lv_packages ).
    IF NOT line_exists( lt_packages[ table_line = CONV devclass( lv_package ) ] ).
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                         iv_message = |packages must list every package the zip may touch, { lv_package } among them| ).
      RETURN.
    ENDIF.
    LOOP AT lt_packages INTO DATA(lv_listed).
      IF valid_package( CONV #( lv_listed ) ) = abap_false.
        rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                           iv_message = |packages: '{ lv_listed }' is not a package name| ).
        RETURN.
      ENDIF.
    ENDLOOP.
    IF lv_size <= 0 OR lv_size > c_max_zip.
      rs_response = err( iv_id = is_message-id iv_code = 'TOO_LARGE'
                         iv_message = |size must be 1 to { c_max_zip } bytes| ).
      RETURN.
    ENDIF.
    FIND PCRE '^[0-9A-F]{64}\z' IN lv_sha.
    IF sy-subrc <> 0.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                         iv_message = `sha256 must be a SHA-256 digest in hex` ).
      RETURN.
    ENDIF.
    " The job program must be there before a byte is sent.
    SELECT SINGLE name FROM trdir INTO @DATA(lv_prog) WHERE name = @c_job_name.
    IF sy-subrc <> 0.
      rs_response = err( iv_id = is_message-id iv_code = 'JOB_PROGRAM_MISSING'
                         iv_message = |Program { c_job_name } is not on this system; deploy the current ZADT_VSP (vsp install zadt-vsp). Nothing was imported.| ).
      RETURN.
    ENDIF.

    TRY.
        lv_uuid = cl_system_uuid=>create_uuid_c32_static( ).
      CATCH cx_uuid_error.
        lv_uuid = |GIT{ sy-datum }{ sy-uzeit }|.
    ENDTRY.

    ls_params = VALUE #(
      package   = lv_package
      repo_name = lv_repo_name
      overwrite = xsdbool( lv_overwrite = 'true' )
      transport = lv_transport
      packages  = lv_packages ).
    ms_upload = VALUE #( id = lv_uuid size = lv_size sha = lv_sha params = ls_params ).

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'assembly_id' iv_value = ms_upload-id ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'package' iv_value = lv_package ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'system' iv_value = CONV #( sy-sysid ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'client' iv_value = CONV #( sy-mandt ) ) )
      ( zcl_vsp_utils=>json_int( iv_key = 'max_chunk' iv_value = c_max_chunk ) )
    ) ) ) ).
  ENDMETHOD.


  METHOD import_chunk.
    DATA lv_chunk TYPE xstring.

    DATA(lv_params) = is_message-params.
    DATA(lv_id) = zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'assembly_id' ).
    IF ms_upload-id IS INITIAL OR lv_id <> ms_upload-id.
      rs_response = err( iv_id = is_message-id iv_code = 'NO_UPLOAD'
                         iv_message = `No upload with this assembly_id is in progress in this session` ).
      RETURN.
    ENDIF.
    DATA(lv_offset) = zcl_vsp_utils=>extract_param_int( iv_params = lv_params iv_name = 'offset' ).
    DATA(lv_b64) = zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'chunk_b64' ).
    TRY.
        lv_chunk = cl_http_utility=>decode_x_base64( lv_b64 ).
      CATCH cx_root.
        CLEAR lv_chunk.
    ENDTRY.
    IF xstrlen( lv_chunk ) = 0 OR xstrlen( lv_chunk ) > c_max_chunk.
      CLEAR ms_upload.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_CHUNK'
                         iv_message = |A chunk is 1 to { c_max_chunk } bytes of base64; the upload is discarded| ).
      RETURN.
    ENDIF.
    IF lv_offset <> xstrlen( ms_upload-data ) OR lv_offset + xstrlen( lv_chunk ) > ms_upload-size.
      CLEAR ms_upload.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_CHUNK'
                         iv_message = `Chunk out of order or past the declared size; the upload is discarded` ).
      RETURN.
    ENDIF.
    CONCATENATE ms_upload-data lv_chunk INTO ms_upload-data IN BYTE MODE.

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj(
      zcl_vsp_utils=>json_int( iv_key = 'received' iv_value = xstrlen( ms_upload-data ) ) ) ).
  ENDMETHOD.


  METHOD import_abort.
    DATA(lv_id) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'assembly_id' ).
    IF ms_upload-id IS INITIAL OR lv_id <> ms_upload-id.
      rs_response = err( iv_id = is_message-id iv_code = 'NO_UPLOAD'
                         iv_message = `No upload with this assembly_id is in progress in this session; nothing was discarded` ).
      RETURN.
    ENDIF.
    CLEAR ms_upload.
    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id
      iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_bool( iv_key = 'aborted' iv_value = abap_true ) ) ).
  ENDMETHOD.


  METHOD import_commit.
    DATA: lv_jobcount TYPE string,
          lv_error    TYPE string,
          lv_code     TYPE string,
          lv_message  TYPE string.

    DATA(lv_id) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'assembly_id' ).
    IF ms_upload-id IS INITIAL OR lv_id <> ms_upload-id.
      rs_response = err( iv_id = is_message-id iv_code = 'NO_UPLOAD'
                         iv_message = `No upload with this assembly_id is in progress in this session` ).
      RETURN.
    ENDIF.
    DATA(ls_up) = ms_upload.
    CLEAR ms_upload.

    IF xstrlen( ls_up-data ) <> ls_up-size.
      rs_response = err( iv_id = is_message-id iv_code = 'INCOMPLETE'
                         iv_message = |Received { xstrlen( ls_up-data ) } of { ls_up-size } bytes. Nothing was imported.| ).
      RETURN.
    ENDIF.
    IF sha256( ls_up-data ) <> ls_up-sha.
      rs_response = err( iv_id = is_message-id iv_code = 'CHECKSUM_MISMATCH'
                         iv_message = `The received bytes do not match the SHA-256 declared at begin. Nothing was imported.` ).
      RETURN.
    ENDIF.
    " A zip within the limits, with one .abapgit.xml at its root.
    zip_limits( EXPORTING iv_zip = ls_up-data IMPORTING ev_code = lv_code ev_message = lv_message ).
    IF lv_code IS NOT INITIAL.
      rs_response = err( iv_id = is_message-id iv_code = lv_code iv_message = |{ lv_message } Nothing was imported.| ).
      RETURN.
    ENDIF.

    start_job( EXPORTING iv_zip = ls_up-data is_params = ls_up-params iv_push_id = iv_session_id
               IMPORTING ev_jobcount = lv_jobcount ev_error = lv_error ).
    IF lv_error IS NOT INITIAL.
      rs_response = err( iv_id = is_message-id iv_code = 'JOB_NOT_STARTED'
                         iv_message = |The import job could not be started: { lv_error }. Nothing was imported.| ).
      RETURN.
    ENDIF.

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'status' iv_value = `pending` ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'job' iv_value = CONV #( c_job_name ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'job_count' iv_value = lv_jobcount ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'package' iv_value = CONV #( ls_up-params-package ) ) )
    ) ) ) ).
  ENDMETHOD.


  METHOD start_job.
    DATA: lv_jobname  TYPE tbtcjob-jobname,
          lv_jobcount TYPE tbtcjob-jobcount,
          lv_released TYPE btch0000-char1,
          lv_variant  TYPE rsvar-variant,
          ls_varid    TYPE varid,
          lt_contents TYPE STANDARD TABLE OF rsparams WITH DEFAULT KEY,
          lt_text     TYPE STANDARD TABLE OF varit WITH DEFAULT KEY,
          ls_indx     TYPE indx,
          lv_key      TYPE indx-srtfd,
          lv_report   TYPE syrepid.

    CLEAR: ev_jobcount, ev_error.
    housekeeping( ).
    lv_report = c_job_name.

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

    " The zip and the parameters wait for the job under its number.
    lv_key = |VSPGITZ{ lv_jobcount }|.
    ls_indx-aedat = sy-datum.
    ls_indx-usera = sy-uname.
    ls_indx-pgmid = c_job_name.
    DATA(lv_zip) = iv_zip.
    DATA(ls_params) = is_params.
    EXPORT zip = lv_zip params = ls_params TO DATABASE indx(zv) FROM ls_indx ID lv_key.

    " What the job is to do is bound to its step: a protected variant (only
    " its creator may change it) named after the job, holding the package
    " and the SHA-256 of the zip and of the parameters. An APC session may
    " not SUBMIT, so SUBMIT ... VIA JOB is not an option.
    lv_variant = |VSP{ lv_jobcount }|.
    ls_varid = VALUE #( report = c_job_name variant = lv_variant environmnt = 'B' protected = 'X' ).
    lt_contents = VALUE #(
      ( selname = 'P_PKG'  kind = 'P' sign = 'I' option = 'EQ' low = is_params-package )
      ( selname = 'P_SHZ'  kind = 'P' sign = 'I' option = 'EQ' low = sha_b64( sha256( iv_zip ) ) )
      ( selname = 'P_SHM'  kind = 'P' sign = 'I' option = 'EQ' low = meta_sha( is_params ) )
      ( selname = 'P_PUSH' kind = 'P' sign = 'I' option = 'EQ' low = iv_push_id ) ).
    lt_text = VALUE #( ( langu = sy-langu report = c_job_name variant = lv_variant vtext = 'vsp git import' ) ).
    CALL FUNCTION 'RS_CREATE_VARIANT'
      EXPORTING
        curr_report               = lv_report
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
      drop_job( lv_jobcount ).
      RETURN.
    ENDIF.

    CALL FUNCTION 'JOB_SUBMIT'
      EXPORTING
        authcknam         = sy-uname
        jobcount          = lv_jobcount
        jobname           = lv_jobname
        report            = lv_report
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
      drop_job( lv_jobcount ).
      RETURN.
    ENDIF.

    " The ticket is on the database before the job can start.
    COMMIT WORK.

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
      ev_error = |JOB_CLOSE failed (exception { sy-subrc }) { last_message( ) }; the job, its variant and its zip were deleted again|.
      drop_job( lv_jobcount ).
      RETURN.
    ENDIF.
    IF lv_released IS INITIAL.
      ev_error = |job { lv_jobname } { lv_jobcount } was scheduled but not released (releasing it needs S_BTCH_JOB with JOBACTION RELE); the job, its variant and its zip were deleted again|.
      drop_job( lv_jobcount ).
      RETURN.
    ENDIF.
    ev_jobcount = lv_jobcount.
  ENDMETHOD.


  METHOD housekeeping.
    DATA: lv_jobname  TYPE tbtcjob-jobname,
          lv_jobcount TYPE tbtcjob-jobcount,
          lv_key      TYPE indx-srtfd,
          lv_report   TYPE syrepid.

    lv_jobname = c_job_name.
    lv_report = c_job_name.
    " Variants and zips of jobs that have ended, or are gone (a running job
    " cannot delete its own variant).
    SELECT variant FROM varid INTO TABLE @DATA(lt_old)
      WHERE report = @c_job_name AND variant LIKE 'VSP%'.
    LOOP AT lt_old INTO DATA(ls_old).
      lv_jobcount = ls_old-variant+3.
      SELECT SINGLE status FROM tbtco INTO @DATA(lv_status)
        WHERE jobname = @lv_jobname AND jobcount = @lv_jobcount.
      IF sy-subrc <> 0 OR lv_status = 'F' OR lv_status = 'A'.
        CALL FUNCTION 'RS_VARIANT_DELETE'
          EXPORTING
            report                = lv_report
            variant               = ls_old-variant
            flag_confirmscreen    = 'X'
            suppress_message      = 'X'
            suppress_input_dialog = 'X'
          EXCEPTIONS
            OTHERS                = 1.
        lv_key = |VSPGITZ{ lv_jobcount }|.
        DELETE FROM DATABASE indx(zv) ID lv_key.
      ENDIF.
    ENDLOOP.
    " Results are kept a week for import_status.
    DATA(lv_cutoff) = CONV d( sy-datum - 7 ).
    DELETE FROM indx WHERE relid = 'ZV' AND srtfd LIKE 'VSPGITR%' AND aedat < @lv_cutoff.
    " This user's jobs that were scheduled and never released (JOB_CLOSE
    " failed, or no release authorization) and that drop_job could not
    " remove at the time.
    DATA(lv_stale) = CONV d( sy-datum - 1 ).
    SELECT jobcount FROM tbtco INTO TABLE @DATA(lt_stale)
      WHERE jobname = @lv_jobname AND status = 'P' AND sdluname = @sy-uname AND sdldate < @lv_stale.
    LOOP AT lt_stale INTO DATA(ls_stale).
      drop_job( ls_stale-jobcount ).
    ENDLOOP.
  ENDMETHOD.


  METHOD drop_job.
    DATA: lv_jobname  TYPE tbtcjob-jobname,
          lv_jobcount TYPE tbtcjob-jobcount,
          lv_variant  TYPE rsvar-variant,
          lv_key      TYPE indx-srtfd,
          lv_report   TYPE syrepid.

    lv_jobname = c_job_name.
    lv_report = c_job_name.
    lv_jobcount = iv_jobcount.
    " Only a job of this user: another user's job, its variant and its zip
    " are not this session's to remove.
    SELECT SINGLE sdluname FROM tbtco INTO @DATA(lv_owner)
      WHERE jobname = @lv_jobname AND jobcount = @lv_jobcount.
    IF sy-subrc = 0 AND lv_owner <> sy-uname.
      RETURN.
    ENDIF.
    CALL FUNCTION 'BP_JOB_DELETE'
      EXPORTING
        jobcount   = lv_jobcount
        jobname    = lv_jobname
        forcedmode = 'X'
      EXCEPTIONS
        OTHERS     = 1.
    lv_variant = |VSP{ lv_jobcount }|.
    CALL FUNCTION 'RS_VARIANT_DELETE'
      EXPORTING
        report                = lv_report
        variant               = lv_variant
        flag_confirmscreen    = 'X'
        suppress_message      = 'X'
        suppress_input_dialog = 'X'
      EXCEPTIONS
        OTHERS                = 1.
    lv_key = |VSPGITZ{ lv_jobcount }|.
    DELETE FROM DATABASE indx(zv) ID lv_key.
    COMMIT WORK.
  ENDMETHOD.


  METHOD zip_limits.
    DATA: lo_zip   TYPE REF TO cl_abap_zip,
          lv_total TYPE int8,
          lv_dots  TYPE i.

    CLEAR: ev_code, ev_message.
    " The directory only: nothing is decompressed here.
    CREATE OBJECT lo_zip.
    lo_zip->load( EXPORTING zip = iv_zip EXCEPTIONS zip_parse_error = 1 OTHERS = 2 ).
    IF sy-subrc <> 0.
      ev_code = `INVALID_ZIP`.
      ev_message = `The upload is not a zip.`.
      RETURN.
    ENDIF.
    IF lines( lo_zip->files ) > c_max_entries.
      ev_code = `TOO_LARGE`.
      ev_message = |The zip has { lines( lo_zip->files ) } entries, over the limit of { c_max_entries }.|.
      RETURN.
    ENDIF.
    LOOP AT lo_zip->files INTO DATA(ls_file).
      " The size is a 4-byte integer read from the zip: past 2 GB it comes
      " back negative.
      IF ls_file-size < 0.
        ev_code = `TOO_LARGE`.
        ev_message = |{ ls_file-name } declares a size over 2 GB.|.
        RETURN.
      ENDIF.
      lv_total = lv_total + ls_file-size.
      IF ls_file-name = '.abapgit.xml'.
        lv_dots = lv_dots + 1.
      ENDIF.
    ENDLOOP.
    IF lv_total > c_max_unzipped.
      ev_code = `TOO_LARGE`.
      ev_message = |The zip unpacks to { lv_total } bytes, over the limit of { c_max_unzipped }.|.
      RETURN.
    ENDIF.
    IF lv_dots <> 1.
      ev_code = `INVALID_ZIP`.
      ev_message = |The zip has { lv_dots } .abapgit.xml files at its root; an abapGit offline zip has exactly one.|.
      RETURN.
    ENDIF.
  ENDMETHOD.


  METHOD run_job.
    DATA: lv_jobcount TYPE tbtcm-jobcount,
          lv_jobname  TYPE tbtcm-jobname,
          lv_zip      TYPE xstring,
          ls_params   TYPE ty_import_params,
          ls_result   TYPE ty_result,
          lv_key      TYPE indx-srtfd.

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
    ls_result-package = to_upper( condense( CONV string( iv_package ) ) ).

    " The step must run with this job's own variant as start_job made it:
    " VSP<job number>, protected, created by this user and changed by no
    " one else. Run by hand, with another variant, or with one someone
    " edited, it does nothing.
    DATA(lv_own_variant) = CONV rsvar-variant( |VSP{ lv_jobcount }| ).
    SELECT SINGLE protected, ename, aename FROM varid INTO @DATA(ls_varid)
      WHERE report = @c_job_name AND variant = @lv_own_variant.
    DATA(lv_variant_found) = xsdbool( sy-subrc = 0 ).
    SELECT SINGLE vtext FROM varit INTO @DATA(lv_vtext)
      WHERE report = @c_job_name AND variant = @lv_own_variant.
    IF sy-subrc <> 0 OR lv_vtext <> 'vsp git import'.
      lv_variant_found = abap_false.
    ENDIF.
    IF sy-slset <> lv_own_variant OR lv_variant_found = abap_false OR ls_varid-protected <> 'X'
       OR ls_varid-ename <> sy-uname OR ( ls_varid-aename IS NOT INITIAL AND ls_varid-aename <> sy-uname ).
      ls_result-outcome = `refused`.
      ls_result-code = `VARIANT_MISMATCH`.
      ls_result-message = |The step does not run with its own protected variant { lv_own_variant } of { sy-uname } (runs with '{ sy-slset }'); nothing was imported.|.
      store_result( iv_jobcount = lv_jobcount is_result = ls_result ).
      job_log( ls_result ).
      publish( is_result = ls_result iv_jobcount = lv_jobcount iv_push_id = iv_push_id ).
      RETURN.
    ENDIF.

    " The zip and the parameters, taken once.
    lv_key = |VSPGITZ{ lv_jobcount }|.
    IMPORT zip = lv_zip params = ls_params FROM DATABASE indx(zv) ID lv_key.
    DATA(lv_found) = xsdbool( sy-subrc = 0 ).
    DELETE FROM DATABASE indx(zv) ID lv_key.
    COMMIT WORK.
    IF lv_found = abap_false.
      ls_result-outcome = `refused`.
      ls_result-code = `TICKET_MISSING`.
      ls_result-message = `The zip of this job is not in INDX(ZV); nothing was imported.`.
    ELSEIF sha_b64( sha256( lv_zip ) ) <> condense( CONV string( iv_zip_sha ) )
        OR meta_sha( ls_params ) <> condense( CONV string( iv_meta_sha ) )
        OR ls_params-package <> ls_result-package.
      ls_result-outcome = `refused`.
      ls_result-code = `TICKET_MISMATCH`.
      ls_result-message = `The zip or the parameters waiting for this job are not the ones it was scheduled with (SHA-256 differs); nothing was imported.`.
    ELSE.
      ls_result = do_import( iv_zip = lv_zip is_params = ls_params ).
    ENDIF.

    store_result( iv_jobcount = lv_jobcount is_result = ls_result ).
    job_log( ls_result ).
    publish( is_result = ls_result iv_jobcount = lv_jobcount iv_push_id = iv_push_id ).
  ENDMETHOD.


  METHOD do_import.
    DATA: li_repo    TYPE REF TO zif_abapgit_repo,
          lv_reason  TYPE string,
          lv_created TYPE abap_bool,
          lv_code    TYPE string,
          lv_message TYPE string,
          lv_phase   TYPE string,
          lx_error   TYPE REF TO cx_root.

    rs_result-package = is_params-package.
    rs_result-transport = is_params-transport.
    DATA(lt_packages) = split_packages( is_params-packages ).

    " The limits again, before abapGit decompresses anything.
    zip_limits( EXPORTING iv_zip = iv_zip IMPORTING ev_code = lv_code ev_message = lv_message ).
    IF lv_code IS NOT INITIAL.
      rs_result-outcome = `refused`.
      rs_result-code = lv_code.
      rs_result-message = |{ lv_message } Nothing was imported.|.
      RETURN.
    ENDIF.

    TRY.
        lv_phase = `load`.
        DATA(lt_files) = zcl_abapgit_zip=>load( iv_zip ).
        " Read by hand: the table's keys differ between abapGit releases.
        DATA(lv_has_dot) = abap_false.
        LOOP AT lt_files INTO DATA(ls_zip_file).
          IF ls_zip_file-path = '/' AND ls_zip_file-filename = '.abapgit.xml'.
            lv_has_dot = abap_true.
          ENDIF.
        ENDLOOP.
        IF lv_has_dot = abap_false.
          rs_result-outcome = `refused`.
          rs_result-code = `INVALID_ZIP`.
          rs_result-message = `The zip has no .abapgit.xml at its root; nothing was imported.`.
          RETURN.
        ENDIF.

        " A transportable package is never created here: it must exist.
        IF is_params-package(1) <> '$' AND zcl_abapgit_factory=>get_sap_package( is_params-package )->exists( ) = abap_false.
          rs_result-outcome = `refused`.
          rs_result-code = `PACKAGE_MISSING`.
          rs_result-message = |Package { is_params-package } does not exist; only a local ($) package is created by the import. Nothing was imported.|.
          RETURN.
        ENDIF.

        lv_phase = `repository`.
        zcl_abapgit_repo_srv=>get_instance( )->get_repo_from_package(
          EXPORTING iv_package = is_params-package
          IMPORTING ei_repo    = li_repo
                    ev_reason  = lv_reason ).
        IF li_repo IS BOUND.
          rs_result-repo_key = li_repo->get_key( ).
          rs_result-repo_name = li_repo->get_name( ).
          IF li_repo->get_package( ) <> is_params-package.
            rs_result-outcome = `refused`.
            rs_result-code = `REPO_OTHER_PACKAGE`.
            rs_result-message = |{ lv_reason }: the repository of { li_repo->get_package( ) } covers { is_params-package }. Nothing was imported.|.
            RETURN.
          ENDIF.
          IF is_params-overwrite = abap_false.
            rs_result-outcome = `refused`.
            rs_result-code = `REPO_EXISTS`.
            rs_result-message = |{ lv_reason } (repository { rs_result-repo_key }); it is not overwritten without overwrite = true. Nothing was imported.|.
            RETURN.
          ENDIF.
          IF li_repo->is_offline( ) = abap_false.
            rs_result-outcome = `refused`.
            rs_result-code = `REPO_ONLINE`.
            rs_result-message = |Repository { rs_result-repo_key } of { is_params-package } is an online repository; a zip is imported into an offline one only. Nothing was imported.|.
            RETURN.
          ENDIF.
        ELSE.
          li_repo = new_offline_repo( iv_name = is_params-repo_name iv_package = is_params-package ).
          lv_created = abap_true.
          rs_result-repo_created = abap_true.
          rs_result-repo_key = li_repo->get_key( ).
          rs_result-repo_name = li_repo->get_name( ).
        ENDIF.

        lv_phase = `checks`.
        li_repo->set_files_remote( lt_files ).
        DATA(ls_checks) = li_repo->deserialize_checks( ).

        lv_message = check_packages( ii_repo = li_repo it_files = lt_files is_params = is_params ).
        IF lv_message IS NOT INITIAL.
          lv_code = `PACKAGE_NOT_LISTED`.
        ELSE.
          evaluate_checks( EXPORTING is_params    = is_params
                                     iv_new_repo  = lv_created
                           IMPORTING ev_code      = lv_code
                                     ev_message   = lv_message
                                     et_decisions = rs_result-decisions
                           CHANGING  cs_checks    = ls_checks ).
        ENDIF.
        IF lv_code IS NOT INITIAL.
          rs_result-outcome = `refused`.
          rs_result-code = lv_code.
          rs_result-message = |{ lv_message } Nothing was imported.|.
          " A repository made for this import goes again with it.
          IF lv_created = abap_true.
            zcl_abapgit_repo_srv=>get_instance( )->delete( li_repo ).
            COMMIT WORK.
            rs_result-repo_created = abap_false.
            rs_result-message = |{ rs_result-message } The repository created for it was removed again.|.
            CLEAR rs_result-repo_key.
          ENDIF.
          RETURN.
        ENDIF.
        rs_result-transport = ls_checks-transport-transport.

        DATA(lt_before) = package_tadir( lt_packages ).
        SELECT SINGLE devclass FROM tdevc INTO @DATA(lv_pkg_before) WHERE devclass = @is_params-package.
        DATA(lv_pkg_existed) = xsdbool( sy-subrc = 0 ).

        lv_phase = `deserialize`.
        DATA(li_log) = li_repo->create_new_log( 'vsp git_import_zip' ).
        TRY.
            li_repo->deserialize( is_checks = ls_checks ii_log = li_log ).
            rs_result-outcome = `imported`.
          CATCH zcx_abapgit_exception INTO DATA(lx_deser).
            rs_result-outcome = `failed`.
            rs_result-code = `DESERIALIZE_FAILED`.
            rs_result-message = clean( lx_deser->get_text( ) ).
        ENDTRY.

        LOOP AT li_log->get_messages( ) INTO DATA(ls_msg).
          IF ls_msg-type = 'E' OR ls_msg-type = 'W' OR ls_msg-type = 'A'.
            APPEND VALUE #( type = ls_msg-type text = clean( ls_msg-text )
                            obj_type = ls_msg-obj_type obj_name = ls_msg-obj_name ) TO rs_result-log.
            IF ( ls_msg-type = 'E' OR ls_msg-type = 'A' ) AND rs_result-outcome = `imported`.
              rs_result-outcome = `imported_with_errors`.
            ENDIF.
          ELSE.
            rs_result-info_count = rs_result-info_count + 1.
          ENDIF.
        ENDLOOP.

        IF lv_pkg_existed = abap_false.
          SELECT SINGLE devclass FROM tdevc INTO @DATA(lv_pkg_after) WHERE devclass = @is_params-package.
          rs_result-package_created = xsdbool( sy-subrc = 0 ).
        ENDIF.

        " The TADIR rows the import created, and those of the objects it
        " was allowed to change. (A local package has no TADIR row.)
        DATA(lt_after) = package_tadir( lt_packages ).
        LOOP AT lt_after INTO DATA(ls_after).
          IF NOT line_exists( lt_before[ object = ls_after-object obj_name = ls_after-obj_name ] ).
            ls_after-created = abap_true.
            APPEND ls_after TO rs_result-tadir.
          ELSEIF line_exists( rs_result-decisions[ obj_type = CONV string( ls_after-object )
                                                   obj_name = CONV string( ls_after-obj_name ) decision = `Y` ] ).
            APPEND ls_after TO rs_result-tadir.
          ENDIF.
        ENDLOOP.

      CATCH zcx_abapgit_exception cx_sy_dyn_call_error INTO lx_error.
        rs_result-outcome = COND #( WHEN lv_phase = `deserialize` THEN `failed` ELSE `refused` ).
        rs_result-code = |{ to_upper( lv_phase ) }_FAILED|.
        rs_result-message = clean( lx_error->get_text( ) ).
        IF lv_created = abap_true AND lv_phase <> `deserialize` AND li_repo IS BOUND.
          TRY.
              zcl_abapgit_repo_srv=>get_instance( )->delete( li_repo ).
              COMMIT WORK.
              rs_result-repo_created = abap_false.
              CLEAR rs_result-repo_key.
              rs_result-message = |{ rs_result-message } The repository created for it was removed again.|.
            CATCH zcx_abapgit_exception ##NO_HANDLER.
          ENDTRY.
        ENDIF.
    ENDTRY.
  ENDMETHOD.


  METHOD evaluate_checks.
    " abapGit's deserialize actions (ZIF_ABAPGIT_OBJECTS=>C_DESERIALIZE_ACTION),
    " as numbers: the constant moved between releases.
    CONSTANTS: lc_no_support TYPE i VALUE -1,
               lc_add        TYPE i VALUE 1,
               lc_update     TYPE i VALUE 2,
               lc_overwrite  TYPE i VALUE 3,
               lc_delete     TYPE i VALUE 4,
               lc_delete_add TYPE i VALUE 5,
               lc_packmove   TYPE i VALUE 6,
               lc_data_loss  TYPE i VALUE 7.
    DATA: lt_refused TYPE string_table,
          lt_kept    TYPE string_table.

    CLEAR: ev_code, ev_message, et_decisions.

    IF cs_checks-requirements-met = 'N'.
      ev_code = `REQUIREMENTS_NOT_MET`.
      ev_message = `The repository's requirements (.abapgit.xml) are not met on this system.`.
      RETURN.
    ENDIF.
    IF cs_checks-dependencies-met = 'N'.
      ev_code = `DEPENDENCIES_NOT_MET`.
      ev_message = `The repository's APACK dependencies are not met on this system.`.
      RETURN.
    ENDIF.
    IF cs_checks-warning_package IS NOT INITIAL.
      LOOP AT cs_checks-warning_package INTO DATA(ls_pkg).
        APPEND |{ ls_pkg-obj_type } { ls_pkg-obj_name } (in { ls_pkg-devclass })| TO lt_refused.
      ENDLOOP.
      ev_code = `PACKAGE_CONFLICT`.
      ev_message = |Objects of the zip exist in another package and are never taken over: { concat_lines_of( table = lt_refused sep = `, ` ) }.|.
      RETURN.
    ENDIF.
    IF cs_checks-data_loss IS NOT INITIAL.
      LOOP AT cs_checks-data_loss INTO DATA(ls_loss).
        APPEND |{ ls_loss-obj_type } { ls_loss-obj_name }| TO lt_refused.
      ENDLOOP.
      ev_code = `DATA_LOSS`.
      ev_message = |The import would lose table data: { concat_lines_of( table = lt_refused sep = `, ` ) }.|.
      RETURN.
    ENDIF.
    IF cs_checks-customizing-required = abap_true.
      ev_code = `CUSTOMIZING_NOT_SUPPORTED`.
      ev_message = `The zip carries table content (customizing); that is not imported here.`.
      RETURN.
    ENDIF.

    LOOP AT cs_checks-overwrite ASSIGNING FIELD-SYMBOL(<ls_over>).
      DATA(ls_decision) = VALUE ty_decision( obj_type = <ls_over>-obj_type obj_name = <ls_over>-obj_name
                                             devclass = <ls_over>-devclass ).
      " The target package existed without a repository: abapGit lists its
      " own package entry as changed. That one entry is kept as it is (never
      " written) and needs no overwrite; nothing else is exempt.
      IF iv_new_repo = abap_true AND <ls_over>-obj_type = 'DEVC' AND <ls_over>-obj_name = is_params-package
         AND ( <ls_over>-action = lc_update OR <ls_over>-action = lc_overwrite ).
        ls_decision-action = `package_kept`.
        <ls_over>-decision = 'N'.
        ls_decision-decision = <ls_over>-decision.
        APPEND ls_decision TO et_decisions.
        CONTINUE.
      ENDIF.
      CASE <ls_over>-action.
        WHEN lc_add.
          ls_decision-action = `add`.
          <ls_over>-decision = 'Y'.
        WHEN lc_delete.
          " A local object the zip does not have is kept: deleting is
          " git_delete_objects' job, item by item.
          ls_decision-action = `delete`.
          <ls_over>-decision = 'N'.
          APPEND |{ <ls_over>-obj_type } { <ls_over>-obj_name }| TO lt_kept.
        WHEN lc_update OR lc_overwrite OR lc_delete_add.
          ls_decision-action = SWITCH #( <ls_over>-action WHEN lc_update THEN `update`
                                                          WHEN lc_overwrite THEN `overwrite`
                                                          ELSE `delete_add` ).
          IF is_params-overwrite = abap_true.
            <ls_over>-decision = 'Y'.
          ELSE.
            <ls_over>-decision = 'N'.
            APPEND |{ <ls_over>-obj_type } { <ls_over>-obj_name } ({ ls_decision-action })| TO lt_refused.
          ENDIF.
        WHEN lc_no_support.
          ev_code = `UNSUPPORTED_OBJECT`.
          ev_message = |{ <ls_over>-obj_type } { <ls_over>-obj_name }: the object type is not supported by the abapGit on this system.|.
          RETURN.
        WHEN lc_packmove.
          ev_code = `PACKAGE_CONFLICT`.
          ev_message = |{ <ls_over>-obj_type } { <ls_over>-obj_name } would be moved to another package; that is never done here.|.
          RETURN.
        WHEN lc_data_loss.
          ev_code = `DATA_LOSS`.
          ev_message = |{ <ls_over>-obj_type } { <ls_over>-obj_name }: the import would lose table data.|.
          RETURN.
        WHEN OTHERS.
          ev_code = `UNKNOWN_ACTION`.
          ev_message = |{ <ls_over>-obj_type } { <ls_over>-obj_name }: abapGit action { <ls_over>-action } is not one this import knows.|.
          RETURN.
      ENDCASE.
      ls_decision-decision = <ls_over>-decision.
      APPEND ls_decision TO et_decisions.
    ENDLOOP.
    IF lt_refused IS NOT INITIAL.
      ev_code = `OVERWRITE_REFUSED`.
      ev_message = |Objects exist and would be changed: { concat_lines_of( table = lt_refused sep = `, ` ) }; pass overwrite = true to overwrite them.|.
      RETURN.
    ENDIF.

    IF cs_checks-transport-required = abap_true.
      IF is_params-transport IS INITIAL.
        ev_code = `TRANSPORT_REQUIRED`.
        ev_message = |Package { is_params-package } records changes: a transport request is required.|.
        RETURN.
      ENDIF.
      cs_checks-transport-transport = is_params-transport.
    ELSE.
      CLEAR cs_checks-transport-transport.
    ENDIF.
  ENDMETHOD.


  METHOD check_packages.
    DATA(lt_listed) = split_packages( is_params-packages ).
    DATA(lo_dot) = ii_repo->get_dot_abapgit( ).
    DATA(lv_start) = lo_dot->get_starting_folder( ).
    DATA(lv_start_len) = strlen( lv_start ).
    DATA(lo_logic) = zcl_abapgit_folder_logic=>get_instance( ).
    LOOP AT it_files INTO DATA(ls_file).
      " Only files below the starting folder are objects.
      IF strlen( ls_file-path ) < lv_start_len.
        CONTINUE.
      ENDIF.
      IF substring( val = ls_file-path len = lv_start_len ) <> lv_start.
        CONTINUE.
      ENDIF.
      DATA(lv_package) = lo_logic->path_to_package(
        iv_top                  = is_params-package
        io_dot                  = lo_dot
        iv_path                 = ls_file-path
        iv_create_if_not_exists = abap_false ).
      IF lv_package IS NOT INITIAL AND NOT line_exists( lt_listed[ table_line = lv_package ] ).
        rv_message = |{ ls_file-path }{ ls_file-filename } maps to package { lv_package }, which is not among the packages checked ({ is_params-packages }).|.
        RETURN.
      ENDIF.
    ENDLOOP.
  ENDMETHOD.


  METHOD new_offline_repo.
    DATA: lt_params  TYPE abap_parmbind_tab,
          lv_name    TYPE string,
          lv_package TYPE devclass.

    lv_name = iv_name.
    lv_package = iv_package.
    DATA(li_srv) = zcl_abapgit_repo_srv=>get_instance( ).
    DATA(lo_type) = CAST cl_abap_objectdescr( cl_abap_typedescr=>describe_by_name( 'ZIF_ABAPGIT_REPO_SRV' ) ).
    READ TABLE lo_type->methods INTO DATA(ls_method) WITH KEY name = 'NEW_OFFLINE'.
    DATA(lv_name_param) = COND abap_parmname( WHEN line_exists( ls_method-parameters[ name = 'IV_NAME' ] )
                                              THEN 'IV_NAME' ELSE 'IV_URL' ).
    lt_params = VALUE #(
      ( name = lv_name_param kind = cl_abap_objectdescr=>exporting value = REF #( lv_name ) )
      ( name = 'IV_PACKAGE'  kind = cl_abap_objectdescr=>exporting value = REF #( lv_package ) )
      ( name = 'RI_REPO'     kind = cl_abap_objectdescr=>receiving value = REF #( ri_repo ) ) ).
    CALL METHOD li_srv->('NEW_OFFLINE') PARAMETER-TABLE lt_params.
  ENDMETHOD.


  METHOD package_tadir.
    DATA lt_range TYPE RANGE OF devclass.
    IF it_packages IS INITIAL.
      RETURN.
    ENDIF.
    lt_range = VALUE #( FOR lv_pkg IN it_packages ( sign = 'I' option = 'EQ' low = lv_pkg ) ).
    SELECT object, obj_name, devclass FROM tadir
      WHERE pgmid = 'R3TR' AND devclass IN @lt_range AND delflag = @space
      INTO CORRESPONDING FIELDS OF TABLE @rt_rows.
    SORT rt_rows BY object obj_name.
  ENDMETHOD.


  METHOD split_packages.
    SPLIT iv_packages AT ',' INTO TABLE DATA(lt_parts).
    LOOP AT lt_parts INTO DATA(lv_part).
      lv_part = to_upper( condense( lv_part ) ).
      IF lv_part IS NOT INITIAL AND strlen( lv_part ) <= 30.
        APPEND CONV devclass( lv_part ) TO rt_packages.
      ENDIF.
    ENDLOOP.
    SORT rt_packages.
    DELETE ADJACENT DUPLICATES FROM rt_packages.
  ENDMETHOD.


  METHOD valid_package.
    FIND PCRE '^[A-Z0-9_$/]{1,30}\z' IN iv_package.
    rv_ok = xsdbool( sy-subrc = 0 ).
  ENDMETHOD.


  METHOD result_json.
    DATA: lt_log   TYPE string_table,
          lt_tadir TYPE string_table,
          lt_dec   TYPE string_table.

    LOOP AT is_result-log INTO DATA(ls_log).
      APPEND zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
        ( zcl_vsp_utils=>json_str( iv_key = 'type' iv_value = ls_log-type ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'text' iv_value = ls_log-text ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'obj_type' iv_value = ls_log-obj_type ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'obj_name' iv_value = ls_log-obj_name ) )
      ) ) ) TO lt_log.
    ENDLOOP.
    LOOP AT is_result-tadir INTO DATA(ls_row).
      APPEND zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
        ( zcl_vsp_utils=>json_str( iv_key = 'pgmid' iv_value = `R3TR` ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'object' iv_value = CONV #( ls_row-object ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'obj_name' iv_value = CONV #( ls_row-obj_name ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'devclass' iv_value = CONV #( ls_row-devclass ) ) )
        ( zcl_vsp_utils=>json_bool( iv_key = 'created' iv_value = ls_row-created ) )
      ) ) ) TO lt_tadir.
    ENDLOOP.
    LOOP AT is_result-decisions INTO DATA(ls_dec).
      APPEND zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
        ( zcl_vsp_utils=>json_str( iv_key = 'obj_type' iv_value = ls_dec-obj_type ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'obj_name' iv_value = ls_dec-obj_name ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'devclass' iv_value = ls_dec-devclass ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'action' iv_value = ls_dec-action ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'decision' iv_value = ls_dec-decision ) )
      ) ) ) TO lt_dec.
    ENDLOOP.
    rv_json = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'outcome' iv_value = is_result-outcome ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'code' iv_value = is_result-code ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'message' iv_value = is_result-message ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'system' iv_value = CONV #( sy-sysid ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'client' iv_value = CONV #( sy-mandt ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'package' iv_value = is_result-package ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'repo_key' iv_value = is_result-repo_key ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'repo_name' iv_value = is_result-repo_name ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'repo_created' iv_value = is_result-repo_created ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'package_created' iv_value = is_result-package_created ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'transport' iv_value = is_result-transport ) )
      ( zcl_vsp_utils=>json_int( iv_key = 'info_count' iv_value = is_result-info_count ) )
      ( |"log":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_log ) ) }| )
      ( |"tadir":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_tadir ) ) }| )
      ( |"decisions":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_dec ) ) }| )
    ) ) ).
  ENDMETHOD.


  METHOD store_result.
    DATA: ls_indx TYPE indx,
          lv_key  TYPE indx-srtfd.
    lv_key = |VSPGITR{ iv_jobcount }|.
    ls_indx-aedat = sy-datum.
    ls_indx-usera = sy-uname.
    ls_indx-pgmid = c_job_name.
    DATA(lv_json) = result_json( is_result ).
    EXPORT result = lv_json TO DATABASE indx(zv) FROM ls_indx ID lv_key.
    COMMIT WORK.
  ENDMETHOD.


  METHOD job_log.
    " Background job: MESSAGE TYPE 'S' goes to the job log.
    DATA lv_line TYPE c LENGTH 120.
    lv_line = |VSP package={ is_result-package } outcome={ is_result-outcome } code={ is_result-code } repo={ is_result-repo_key }|.
    MESSAGE lv_line TYPE 'S'.
    IF is_result-message IS NOT INITIAL.
      lv_line = |VSP message { is_result-message }|.
      MESSAGE lv_line TYPE 'S'.
    ENDIF.
  ENDMETHOD.


  METHOD publish.
    IF iv_push_id IS INITIAL.
      RETURN.
    ENDIF.
    DATA(lv_data) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'event' iv_value = `import_result` ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'job' iv_value = CONV #( c_job_name ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'job_count' iv_value = CONV #( iv_jobcount ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'package' iv_value = is_result-package ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'outcome' iv_value = is_result-outcome ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'code' iv_value = is_result-code ) )
    ) ) ).
    " A response-shaped frame whose id names the job.
    DATA(lv_message) = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'id' iv_value = |push:git:{ iv_jobcount }| ) )
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


  METHOD handle_import_status.
    DATA: lv_jobname  TYPE tbtcjob-jobname,
          lv_jobcount TYPE tbtcjob-jobcount,
          lv_json     TYPE string,
          lv_outcome  TYPE string,
          lv_key      TYPE indx-srtfd,
          ls_indx     TYPE indx,
          lt_log      TYPE STANDARD TABLE OF tbtc5 WITH DEFAULT KEY,
          lt_log_json TYPE string_table.

    DATA(lv_job) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'job' ).
    FIND PCRE '^[0-9]{8}\z' IN lv_job.
    IF sy-subrc <> 0.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM' iv_message = |job '{ lv_job }' is not a job number| ).
      RETURN.
    ENDIF.
    lv_jobname = c_job_name.
    lv_jobcount = lv_job.
    SELECT SINGLE status, sdluname FROM tbtco INTO @DATA(ls_job)
      WHERE jobname = @lv_jobname AND jobcount = @lv_jobcount.
    DATA(lv_found) = xsdbool( sy-subrc = 0 ).
    IF lv_found = abap_true AND ls_job-sdluname <> sy-uname.
      rs_response = err( iv_id = is_message-id iv_code = 'NOT_YOUR_JOB'
                         iv_message = |Job { lv_job } was scheduled by another user| ).
      RETURN.
    ENDIF.

    " The result records whose import it was (INDX-USERA, the job's user,
    " which is the user who started it): once SM37's history of the job is
    " gone, that is the only proof, and it is checked either way.
    lv_key = |VSPGITR{ lv_job }|.
    SELECT SINGLE usera FROM indx INTO @DATA(lv_result_owner)
      WHERE relid = 'ZV' AND srtfd = @lv_key AND srtf2 = 0.
    IF sy-subrc = 0 AND lv_result_owner <> sy-uname.
      rs_response = err( iv_id = is_message-id iv_code = 'NOT_YOUR_JOB'
                         iv_message = |The result of job { lv_job } belongs to another user| ).
      RETURN.
    ENDIF.
    IMPORT result = lv_json FROM DATABASE indx(zv) TO ls_indx ID lv_key.
    DATA(lv_has_result) = xsdbool( sy-subrc = 0 AND lv_json IS NOT INITIAL ).
    IF lv_has_result = abap_true AND ls_indx-usera <> sy-uname.
      rs_response = err( iv_id = is_message-id iv_code = 'NOT_YOUR_JOB'
                         iv_message = |The result of job { lv_job } belongs to another user| ).
      RETURN.
    ENDIF.

    IF lv_has_result = abap_true.
      lv_outcome = `done`.
    ELSEIF lv_found = abap_false.
      lv_outcome = `unknown`.
    ELSEIF ls_job-status = 'P' OR ls_job-status = 'S' OR ls_job-status = 'Y' OR ls_job-status = 'Z' OR ls_job-status = 'R'.
      lv_outcome = `pending`.
    ELSE.
      " Ended without a result: cancelled, or a dump.
      lv_outcome = `failed`.
    ENDIF.

    IF lv_found = abap_true AND lv_outcome <> `pending`.
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
      LOOP AT lt_log INTO DATA(ls_log).
        " A failed job's whole log says why; otherwise this service's lines.
        IF lv_outcome = `failed` OR ls_log-text CP 'VSP *'.
          APPEND |"{ zcl_vsp_utils=>escape_json( clean( ls_log-text ) ) }"| TO lt_log_json.
        ENDIF.
      ENDLOOP.
    ENDIF.

    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'job' iv_value = CONV #( c_job_name ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'job_count' iv_value = lv_job ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'job_found' iv_value = lv_found ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'job_status' iv_value = CONV #( ls_job-status ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'outcome' iv_value = lv_outcome ) )
      ( |"job_log":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_log_json ) ) }| )
      ( COND #( WHEN lv_has_result = abap_true THEN |"result":{ lv_json }| ) )
    ) ) ) ).
  ENDMETHOD.


  METHOD handle_delete_repo.
    DATA: li_repo     TYPE REF TO zif_abapgit_repo,
          lv_devclass TYPE devclass,
          lv_obj_name TYPE sobj_name.

    DATA(lv_package) = to_upper( condense( zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'package' ) ) ).
    DATA(lv_repo_key) = condense( zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'key' ) ).
    DATA(lv_expect_name) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'name' ).
    IF valid_package( lv_package ) = abap_false.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                         iv_message = |package '{ lv_package }' is not a package name| ).
      RETURN.
    ENDIF.
    lv_devclass = lv_package.
    lv_obj_name = lv_package.
    AUTHORITY-CHECK OBJECT 'S_DEVELOP'
      ID 'DEVCLASS' FIELD lv_devclass
      ID 'OBJTYPE'  FIELD 'DEVC'
      ID 'OBJNAME'  FIELD lv_obj_name
      ID 'P_GROUP'  DUMMY
      ID 'ACTVT'    FIELD '06'.
    IF sy-subrc <> 0.
      rs_response = err( iv_id = is_message-id iv_code = 'NOT_AUTHORIZED'
                         iv_message = |No authorization to delete in package { lv_package } (S_DEVELOP, activity 06); nothing was deleted| ).
      RETURN.
    ENDIF.
    TRY.
        " Only the repository registered for exactly this package; one of
        " a super package that merely covers it is not this package's.
        DATA(lt_repos) = zcl_abapgit_persist_factory=>get_repo( )->list( ).
        READ TABLE lt_repos INTO DATA(ls_repo) WITH KEY package = lv_package.
        IF sy-subrc <> 0.
          rs_response = err( iv_id = is_message-id iv_code = 'REPO_NOT_FOUND'
                             iv_message = |No abapGit repository is registered for package { lv_package }| ).
          RETURN.
        ENDIF.
        IF lv_repo_key IS NOT INITIAL AND lv_repo_key <> ls_repo-key.
          rs_response = err( iv_id = is_message-id iv_code = 'REPO_KEY_MISMATCH'
                             iv_message = |The repository of { lv_package } is { ls_repo-key }, not { lv_repo_key }; nothing was deleted| ).
          RETURN.
        ENDIF.
        li_repo = zcl_abapgit_repo_srv=>get_instance( )->get( ls_repo-key ).
        " Only the row the caller saw: with a name, exactly that name too.
        IF lv_expect_name IS NOT INITIAL AND li_repo->get_name( ) <> lv_expect_name.
          rs_response = err( iv_id = is_message-id iv_code = 'REPO_NAME_MISMATCH'
                             iv_message = |The repository of { lv_package } is { ls_repo-key } { li_repo->get_name( ) }, not { lv_expect_name }; nothing was deleted| ).
          RETURN.
        ENDIF.
        " An online repository (its URL, branch and settings) is never
        " unregistered here: that is for a person to decide, in abapGit.
        IF li_repo->is_offline( ) = abap_false.
          rs_response = err( iv_id = is_message-id iv_code = 'REPO_ONLINE'
                             iv_message = |Repository { ls_repo-key } of { lv_package } is an online repository; vsp never unregisters one (do it in abapGit). Nothing was deleted| ).
          RETURN.
        ENDIF.
        " Only the registration of an emptied package: no object but the
        " package's own entry, and no subpackage.
        SELECT SINGLE obj_name FROM tadir INTO @DATA(lv_left)
          WHERE devclass = @lv_devclass AND delflag = @space
            AND NOT ( pgmid = 'R3TR' AND object = 'DEVC' AND obj_name = @lv_obj_name ).
        IF sy-subrc = 0.
          rs_response = err( iv_id = is_message-id iv_code = 'PACKAGE_NOT_EMPTY'
                             iv_message = |Package { lv_package } still has objects ({ lv_left } and maybe more); its repository is kept. Nothing was deleted| ).
          RETURN.
        ENDIF.
        SELECT SINGLE devclass FROM tdevc INTO @DATA(lv_child) WHERE parentcl = @lv_devclass.
        IF sy-subrc = 0.
          rs_response = err( iv_id = is_message-id iv_code = 'PACKAGE_NOT_EMPTY'
                             iv_message = |Package { lv_package } has subpackage { lv_child }; its repository is kept. Nothing was deleted| ).
          RETURN.
        ENDIF.
        DATA(lv_name) = li_repo->get_name( ).
        zcl_abapgit_repo_srv=>get_instance( )->delete( li_repo ).
        COMMIT WORK.
      CATCH zcx_abapgit_exception INTO DATA(lx_error).
        rs_response = err( iv_id = is_message-id iv_code = 'DELETE_FAILED' iv_message = clean( lx_error->get_text( ) ) ).
        RETURN.
    ENDTRY.
    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_bool( iv_key = 'deleted' iv_value = abap_true ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'package' iv_value = lv_package ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'key' iv_value = CONV #( ls_repo-key ) ) )
      ( zcl_vsp_utils=>json_str( iv_key = 'name' iv_value = lv_name ) )
    ) ) ) ).
  ENDMETHOD.


  METHOD handle_package_objects.
    DATA: lt_items      TYPE string_table,
          lt_subs       TYPE string_table,
          lv_repo       TYPE string,
          lv_repo_state TYPE string,
          lv_repo_name  TYPE string,
          lv_devclass   TYPE devclass.

    DATA(lv_package) = to_upper( condense( zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'package' ) ) ).
    IF valid_package( lv_package ) = abap_false.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                         iv_message = |package '{ lv_package }' is not a package name| ).
      RETURN.
    ENDIF.
    lv_devclass = lv_package.
    AUTHORITY-CHECK OBJECT 'S_DEVELOP'
      ID 'DEVCLASS' FIELD lv_devclass
      ID 'OBJTYPE'  DUMMY
      ID 'OBJNAME'  DUMMY
      ID 'P_GROUP'  DUMMY
      ID 'ACTVT'    FIELD '03'.
    IF sy-subrc <> 0.
      rs_response = err( iv_id = is_message-id iv_code = 'NOT_AUTHORIZED'
                         iv_message = |No authorization to display package { lv_package } (S_DEVELOP, activity 03)| ).
      RETURN.
    ENDIF.
    SELECT SINGLE devclass FROM tdevc INTO @DATA(lv_exists) WHERE devclass = @lv_package.
    DATA(lv_found) = xsdbool( sy-subrc = 0 ).
    SELECT pgmid, object, obj_name, devclass FROM tadir
      WHERE devclass = @lv_package AND delflag = @space
      ORDER BY pgmid, object, obj_name
      INTO TABLE @DATA(lt_tadir)
      UP TO 5000 ROWS.
    LOOP AT lt_tadir INTO DATA(ls_tadir).
      APPEND zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
        ( zcl_vsp_utils=>json_str( iv_key = 'pgmid' iv_value = CONV #( ls_tadir-pgmid ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'object' iv_value = CONV #( ls_tadir-object ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'obj_name' iv_value = CONV #( ls_tadir-obj_name ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'devclass' iv_value = CONV #( ls_tadir-devclass ) ) )
      ) ) ) TO lt_items.
    ENDLOOP.
    SELECT devclass FROM tdevc WHERE parentcl = @lv_package INTO TABLE @DATA(lt_children).
    LOOP AT lt_children INTO DATA(lv_child).
      APPEND |"{ zcl_vsp_utils=>escape_json( CONV #( lv_child ) ) }"| TO lt_subs.
    ENDLOOP.
    " The repository registered for the package. Fail closed: when the list
    " cannot be read the answer is an error, and a row whose repository
    " abapGit cannot open is still reported, its state unknown (vsp treats
    " that as online: never unregistered, the package never deleted).
    TRY.
        DATA(lt_repos) = zcl_abapgit_persist_factory=>get_repo( )->list( ).
      CATCH zcx_abapgit_exception INTO DATA(lx_list).
        rs_response = err( iv_id = is_message-id iv_code = 'REPO_LIST_FAILED'
                           iv_message = |abapGit's repository list cannot be read: { clean( lx_list->get_text( ) ) }| ).
        RETURN.
    ENDTRY.
    READ TABLE lt_repos INTO DATA(ls_repo) WITH KEY package = lv_package.
    IF sy-subrc = 0.
      lv_repo_state = `unknown`.
      lv_repo_name = ls_repo-key.
      TRY.
          DATA(li_repo) = zcl_abapgit_repo_srv=>get_instance( )->get( ls_repo-key ).
          lv_repo_name = li_repo->get_name( ).
          lv_repo_state = COND #( WHEN li_repo->is_offline( ) = abap_true THEN `offline` ELSE `online` ).
        CATCH zcx_abapgit_exception ##NO_HANDLER.
      ENDTRY.
      lv_repo = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
        ( zcl_vsp_utils=>json_str( iv_key = 'key' iv_value = CONV #( ls_repo-key ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'name' iv_value = lv_repo_name ) )
        ( zcl_vsp_utils=>json_bool( iv_key = 'offline' iv_value = xsdbool( lv_repo_state = `offline` ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'repo_state' iv_value = lv_repo_state ) )
      ) ) ).
    ENDIF.
    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'package' iv_value = lv_package ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'exists' iv_value = lv_found ) )
      ( zcl_vsp_utils=>json_bool( iv_key = 'truncated' iv_value = xsdbool( lines( lt_tadir ) >= 5000 ) ) )
      ( |"objects":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_items ) ) }| )
      ( |"subpackages":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_subs ) ) }| )
      ( COND #( WHEN lv_repo IS NOT INITIAL THEN |"repo":{ lv_repo }| ) )
    ) ) ) ).
  ENDMETHOD.


  METHOD handle_object_versions.
    DATA: lt_items     TYPE string_table,
          ls_tadir     TYPE tadir,
          lv_devclass  TYPE devclass,
          lv_type      TYPE trobjtype,
          lv_name      TYPE sobj_name,
          lv_stamp     TYPE string,
          lv_stamp_err TYPE string,
          lv_sha       TYPE string,
          lv_sha_err   TYPE string,
          lv_inactive  TYPE abap_bool,
          lv_files     TYPE i.

    DATA(lv_package) = to_upper( condense( zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'package' ) ) ).
    DATA(lv_objects) = to_upper( zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'objects' ) ).
    DATA(lv_want_sha) = xsdbool( zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'sha256' ) = `true` ).
    IF valid_package( lv_package ) = abap_false.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                         iv_message = |package '{ lv_package }' is not a package name| ).
      RETURN.
    ENDIF.
    lv_devclass = lv_package.
    AUTHORITY-CHECK OBJECT 'S_DEVELOP'
      ID 'DEVCLASS' FIELD lv_devclass
      ID 'OBJTYPE'  DUMMY
      ID 'OBJNAME'  DUMMY
      ID 'P_GROUP'  DUMMY
      ID 'ACTVT'    FIELD '03'.
    IF sy-subrc <> 0.
      rs_response = err( iv_id = is_message-id iv_code = 'NOT_AUTHORIZED'
                         iv_message = |No authorization to display package { lv_package } (S_DEVELOP, activity 03)| ).
      RETURN.
    ENDIF.
    SPLIT lv_objects AT ',' INTO TABLE DATA(lt_parts).
    IF lines( lt_parts ) = 0 OR lines( lt_parts ) > c_max_versions.
      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                         iv_message = |objects must name 1 to { c_max_versions } objects, as "TYPE NAME,TYPE NAME"| ).
      RETURN.
    ENDIF.
    LOOP AT lt_parts INTO DATA(lv_part).
      SPLIT condense( lv_part ) AT space INTO DATA(lv_t) DATA(lv_n) DATA(lv_rest).
      IF lv_t IS INITIAL OR lv_n IS INITIAL OR lv_rest IS NOT INITIAL OR strlen( lv_t ) > 4 OR strlen( lv_n ) > 40.
        rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'
                           iv_message = |object '{ condense( lv_part ) }' is not "TYPE NAME"| ).
        RETURN.
      ENDIF.
      lv_type = lv_t.
      lv_name = lv_n.
      CLEAR: lv_stamp, lv_stamp_err, lv_sha, lv_sha_err, lv_files, lv_inactive.
      " Only an object of this package gets a version: one that moved, or
      " is gone, has none to compare.
      CLEAR ls_tadir.
      SELECT SINGLE devclass, masterlang FROM tadir
        WHERE pgmid = 'R3TR' AND object = @lv_type AND obj_name = @lv_name AND delflag = @space
        INTO CORRESPONDING FIELDS OF @ls_tadir.
      DATA(lv_in) = xsdbool( sy-subrc = 0 AND ls_tadir-devclass = lv_devclass ).
      IF lv_in = abap_true.
        object_stamp( EXPORTING iv_type = lv_type iv_name = lv_name
                      IMPORTING ev_stamp = lv_stamp ev_error = lv_stamp_err ).
        lv_inactive = object_inactive( iv_type = lv_type iv_name = lv_name ).
        IF lv_want_sha = abap_true.
          object_sha256( EXPORTING iv_type = lv_type iv_name = lv_name iv_devclass = lv_devclass
                                   iv_language = COND #( WHEN ls_tadir-masterlang IS INITIAL THEN 'E' ELSE ls_tadir-masterlang )
                         IMPORTING ev_sha256 = lv_sha ev_files = lv_files ev_error = lv_sha_err ).
        ENDIF.
      ENDIF.
      APPEND zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
        ( zcl_vsp_utils=>json_str( iv_key = 'type' iv_value = CONV #( lv_type ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'name' iv_value = CONV #( lv_name ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'devclass' iv_value = CONV #( ls_tadir-devclass ) ) )
        ( zcl_vsp_utils=>json_bool( iv_key = 'in_package' iv_value = lv_in ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'stamp' iv_value = lv_stamp ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'stamp_error' iv_value = clean( lv_stamp_err ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'sha256' iv_value = lv_sha ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'sha256_error' iv_value = clean( lv_sha_err ) ) )
        ( zcl_vsp_utils=>json_int( iv_key = 'files' iv_value = lv_files ) )
        ( zcl_vsp_utils=>json_bool( iv_key = 'inactive' iv_value = lv_inactive ) )
      ) ) ) TO lt_items.
    ENDLOOP.
    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
      ( zcl_vsp_utils=>json_str( iv_key = 'package' iv_value = lv_package ) )
      ( |"objects":{ zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_items ) ) }| )
    ) ) ) ).
  ENDMETHOD.


  METHOD object_stamp.
    TYPES: BEGIN OF ty_row,
             d TYPE d,
             t TYPE t,
           END OF ty_row.
    DATA: lt_rows   TYPE STANDARD TABLE OF ty_row WITH DEFAULT KEY,
          lt_digest TYPE string_table,
          lv_xml    TYPE xstring,
          lv_tables TYPE string,
          lv_pool   TYPE string,
          lv_like   TYPE string,
          lv_text   TYPE progname,
          lv_digest TYPE string,
          lv_newest TYPE string.

    CLEAR: ev_stamp, ev_error.
    CASE iv_type.
      WHEN 'CLAS' OR 'INTF'.
        lv_tables = `REPOSRC.REPOTEXT.SEOCLASSDF.SEOCLASSTX.SEOCOMPOTX`.
        lv_pool = |{ iv_name WIDTH = 30 PAD = '=' }|.
        lv_like = |{ lv_pool }%|.
        SELECT progname, udat, utime FROM reposrc WHERE progname LIKE @lv_like INTO TABLE @DATA(lt_pool).
        LOOP AT lt_pool INTO DATA(ls_pool).
          " LIKE reads _ as any character: only the pool's own includes. Not
          " CS: it is regenerated, and moves without the source changing.
          IF strlen( lv_pool ) = 30 AND ls_pool-progname(30) = lv_pool AND ls_pool-progname+30 <> 'CS'.
            APPEND VALUE #( d = ls_pool-udat t = ls_pool-utime ) TO lt_rows.
          ENDIF.
        ENDLOOP.
        lv_text = COND #( WHEN iv_type = 'CLAS' THEN |{ lv_pool }CP| ELSE |{ lv_pool }IP| ).
        SELECT udat AS d, utime AS t FROM repotext WHERE progname = @lv_text APPENDING CORRESPONDING FIELDS OF TABLE @lt_rows.
        SELECT * FROM seoclassdf WHERE clsname = @iv_name ORDER BY PRIMARY KEY INTO TABLE @DATA(lt_seodf).
        CALL TRANSFORMATION id SOURCE rows = lt_seodf RESULT XML lv_xml.
        APPEND sha256( lv_xml ) TO lt_digest.
        SELECT * FROM seoclasstx WHERE clsname = @iv_name ORDER BY PRIMARY KEY INTO TABLE @DATA(lt_seotx).
        CALL TRANSFORMATION id SOURCE rows = lt_seotx RESULT XML lv_xml.
        APPEND sha256( lv_xml ) TO lt_digest.
        SELECT * FROM seocompotx WHERE clsname = @iv_name ORDER BY PRIMARY KEY INTO TABLE @DATA(lt_seoctx).
        CALL TRANSFORMATION id SOURCE rows = lt_seoctx RESULT XML lv_xml.
        APPEND sha256( lv_xml ) TO lt_digest.
      WHEN 'PROG'.
        lv_tables = `REPOSRC.REPOTEXT.D020S`.
        SELECT udat AS d, utime AS t FROM reposrc WHERE progname = @iv_name INTO CORRESPONDING FIELDS OF TABLE @lt_rows.
        SELECT udat AS d, utime AS t FROM repotext WHERE progname = @iv_name APPENDING CORRESPONDING FIELDS OF TABLE @lt_rows.
        SELECT dgen AS d, tgen AS t FROM d020s WHERE prog = @iv_name APPENDING CORRESPONDING FIELDS OF TABLE @lt_rows.
      WHEN 'TABL'.
        lv_tables = `DD02L.DD09L.DD12L.DD02T.DD35L.TDDAT`.
        SELECT as4date AS d, as4time AS t FROM dd02l WHERE tabname = @iv_name INTO CORRESPONDING FIELDS OF TABLE @lt_rows.
        SELECT as4date AS d, as4time AS t FROM dd09l WHERE tabname = @iv_name APPENDING CORRESPONDING FIELDS OF TABLE @lt_rows.
        SELECT as4date AS d, as4time AS t FROM dd12l WHERE sqltab = @iv_name APPENDING CORRESPONDING FIELDS OF TABLE @lt_rows.
        SELECT * FROM dd02t WHERE tabname = @iv_name ORDER BY PRIMARY KEY INTO TABLE @DATA(lt_dd02t).
        CALL TRANSFORMATION id SOURCE rows = lt_dd02t RESULT XML lv_xml.
        APPEND sha256( lv_xml ) TO lt_digest.
        SELECT * FROM dd35l WHERE tabname = @iv_name ORDER BY PRIMARY KEY INTO TABLE @DATA(lt_dd35l).
        CALL TRANSFORMATION id SOURCE rows = lt_dd35l RESULT XML lv_xml.
        APPEND sha256( lv_xml ) TO lt_digest.
        SELECT * FROM tddat WHERE tabname = @iv_name ORDER BY PRIMARY KEY INTO TABLE @DATA(lt_tddat).
        CALL TRANSFORMATION id SOURCE rows = lt_tddat RESULT XML lv_xml.
        APPEND sha256( lv_xml ) TO lt_digest.
      WHEN 'DTEL'.
        lv_tables = `DD04L.DD04T`.
        SELECT as4date AS d, as4time AS t FROM dd04l WHERE rollname = @iv_name INTO CORRESPONDING FIELDS OF TABLE @lt_rows.
        SELECT * FROM dd04t WHERE rollname = @iv_name ORDER BY PRIMARY KEY INTO TABLE @DATA(lt_dd04t).
        CALL TRANSFORMATION id SOURCE rows = lt_dd04t RESULT XML lv_xml.
        APPEND sha256( lv_xml ) TO lt_digest.
      WHEN 'DOMA'.
        lv_tables = `DD01L.DD01T.DD07L.DD07T`.
        SELECT as4date AS d, as4time AS t FROM dd01l WHERE domname = @iv_name INTO CORRESPONDING FIELDS OF TABLE @lt_rows.
        SELECT * FROM dd01t WHERE domname = @iv_name ORDER BY PRIMARY KEY INTO TABLE @DATA(lt_dd01t).
        CALL TRANSFORMATION id SOURCE rows = lt_dd01t RESULT XML lv_xml.
        APPEND sha256( lv_xml ) TO lt_digest.
        SELECT * FROM dd07l WHERE domname = @iv_name ORDER BY PRIMARY KEY INTO TABLE @DATA(lt_dd07l).
        CALL TRANSFORMATION id SOURCE rows = lt_dd07l RESULT XML lv_xml.
        APPEND sha256( lv_xml ) TO lt_digest.
        SELECT * FROM dd07t WHERE domname = @iv_name ORDER BY PRIMARY KEY INTO TABLE @DATA(lt_dd07t).
        CALL TRANSFORMATION id SOURCE rows = lt_dd07t RESULT XML lv_xml.
        APPEND sha256( lv_xml ) TO lt_digest.
      WHEN 'TTYP'.
        lv_tables = `DD40L.DD40T`.
        SELECT as4date AS d, as4time AS t FROM dd40l WHERE typename = @iv_name INTO CORRESPONDING FIELDS OF TABLE @lt_rows.
        SELECT * FROM dd40t WHERE typename = @iv_name ORDER BY PRIMARY KEY INTO TABLE @DATA(lt_dd40t).
        CALL TRANSFORMATION id SOURCE rows = lt_dd40t RESULT XML lv_xml.
        APPEND sha256( lv_xml ) TO lt_digest.
      WHEN 'DDLS'.
        lv_tables = `DDDDLSRC.DDDDLSRCT`.
        SELECT as4date AS d, as4time AS t FROM ddddlsrc WHERE ddlname = @iv_name INTO CORRESPONDING FIELDS OF TABLE @lt_rows.
        SELECT * FROM ddddlsrct WHERE ddlname = @iv_name ORDER BY PRIMARY KEY INTO TABLE @DATA(lt_ddlst).
        CALL TRANSFORMATION id SOURCE rows = lt_ddlst RESULT XML lv_xml.
        APPEND sha256( lv_xml ) TO lt_digest.
      WHEN OTHERS.
        ev_error = |no stamp for type { iv_type }: only CLAS, INTF, PROG, TABL, DTEL, DOMA, TTYP and DDLS have one|.
        RETURN.
    ENDCASE.
    IF lt_rows IS INITIAL.
      RETURN.
    ENDIF.
    LOOP AT lt_rows INTO DATA(ls_row).
      IF |{ ls_row-d }{ ls_row-t }| > lv_newest.
        lv_newest = |{ ls_row-d }{ ls_row-t }|.
      ENDIF.
    ENDLOOP.
    lv_digest = to_lower( sha256( cl_abap_codepage=>convert_to( concat_lines_of( table = lt_digest sep = `,` ) ) ) ).
    IF strlen( lv_digest ) <> 64.
      ev_error = `the stamp's digest could not be computed`.
      RETURN.
    ENDIF.
    ev_stamp = |v2:{ lv_tables }:{ lv_newest }:{ lines( lt_rows ) }:{ lv_digest(16) }|.
  ENDMETHOD.


  METHOD object_inactive.
    DATA: lv_like TYPE string,
          lv_pool TYPE string.

    rv_inactive = abap_false.
    lv_pool = |{ iv_name WIDTH = 30 PAD = '=' }|.
    lv_like = |{ iv_name }%|.
    " The object (R3TR), and its parts (LIMU): a method is the class name
    " padded to 30 then the method, a class include the pool's include.
    SELECT object, obj_name FROM dwinactiv WHERE obj_name LIKE @lv_like INTO TABLE @DATA(lt_inactive).
    LOOP AT lt_inactive INTO DATA(ls_inactive).
      IF ls_inactive-obj_name = iv_name OR ls_inactive-obj_name(30) = iv_name OR ( ( iv_type = 'CLAS' OR iv_type = 'INTF' ) AND ls_inactive-obj_name(30) = lv_pool ).
        rv_inactive = abap_true.
        RETURN.
      ENDIF.
    ENDLOOP.
  ENDMETHOD.


  METHOD object_sha256.
    DATA: lt_lines TYPE string_table,
          ls_files TYPE zif_abapgit_objects=>ty_serialization.

    CLEAR: ev_sha256, ev_files, ev_error.
    TRY.
        ls_files = zcl_abapgit_objects=>serialize(
          is_item        = VALUE #( obj_type = iv_type obj_name = iv_name devclass = iv_devclass )
          io_i18n_params = zcl_abapgit_i18n_params=>new( iv_main_language      = iv_language
                                                         iv_main_language_only = abap_true ) ).
      CATCH cx_root INTO DATA(lx_error).
        ev_error = |abapGit could not serialise it: { lx_error->get_text( ) }|.
        RETURN.
    ENDTRY.
    LOOP AT ls_files-files INTO DATA(ls_file).
      APPEND |{ ls_file-filename }={ to_lower( sha256( ls_file-data ) ) }| TO lt_lines.
    ENDLOOP.
    IF lt_lines IS INITIAL.
      ev_error = `abapGit serialised no file of it`.
      RETURN.
    ENDIF.
    SORT lt_lines.
    DATA(lv_text) = concat_lines_of( table = lt_lines sep = cl_abap_char_utilities=>newline ).
    ev_sha256 = to_lower( sha256( cl_abap_codepage=>convert_to( lv_text ) ) ).
    IF strlen( ev_sha256 ) <> 64.
      CLEAR ev_sha256.
      ev_error = `the SHA-256 could not be computed`.
      RETURN.
    ENDIF.
    ev_files = lines( lt_lines ).
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


  METHOD meta_sha.
    DATA(lv_text) = |{ is_params-package }\|{ is_params-repo_name }\|{ is_params-overwrite }\|{ is_params-transport }\|{ is_params-packages }|.
    rv_b64 = sha_b64( sha256( cl_abap_codepage=>convert_to( lv_text ) ) ).
  ENDMETHOD.


  METHOD clean.
    rv_text = iv_text.
    REPLACE ALL OCCURRENCES OF PCRE '[\x00-\x1F]' IN rv_text WITH ` `.
  ENDMETHOD.


  METHOD last_message.
    IF sy-msgid IS INITIAL.
      RETURN.
    ENDIF.
    MESSAGE ID sy-msgid TYPE 'S' NUMBER sy-msgno
      WITH sy-msgv1 sy-msgv2 sy-msgv3 sy-msgv4 INTO rv_text.
  ENDMETHOD.


  METHOD err.
    rs_response = zcl_vsp_utils=>build_error( iv_id = iv_id iv_code = iv_code iv_message = iv_message ).
  ENDMETHOD.


  METHOD get_package_objects.
    DATA lt_tadir TYPE STANDARD TABLE OF tadir.
    DATA lt_result TYPE zif_abapgit_definitions=>ty_tadir_tt.

    " Get all objects from TADIR for a package
    SELECT * FROM tadir
      INTO TABLE @lt_tadir
      WHERE devclass = @iv_package
        AND delflag  = @abap_false
        AND pgmid    = 'R3TR'.

    " Convert to abapGit format
    lt_result = CORRESPONDING zif_abapgit_definitions=>ty_tadir_tt( lt_tadir ).
    APPEND LINES OF lt_result TO rt_tadir.

    IF iv_include_subpackages = abap_true.
      " Get subpackages recursively
      TRY.
          DATA(lt_subpackages) = zcl_abapgit_factory=>get_sap_package( iv_package )->list_subpackages( ).
          LOOP AT lt_subpackages INTO DATA(lv_subpkg).
            CLEAR lt_tadir.
            SELECT * FROM tadir
              INTO TABLE @lt_tadir
              WHERE devclass = @lv_subpkg
                AND delflag  = @abap_false
                AND pgmid    = 'R3TR'.
            lt_result = CORRESPONDING zif_abapgit_definitions=>ty_tadir_tt( lt_tadir ).
            APPEND LINES OF lt_result TO rt_tadir.
          ENDLOOP.
        CATCH zcx_abapgit_exception.
          " Ignore errors for subpackages
      ENDTRY.
    ENDIF.
  ENDMETHOD.

  METHOD serialize_objects.
    DATA: lo_zip   TYPE REF TO cl_abap_zip,
          lo_i18n  TYPE REF TO zcl_abapgit_i18n_params.

    CREATE OBJECT lo_zip.
    lo_i18n = zcl_abapgit_i18n_params=>new( ).

    " Add .abapgit.xml repository metadata file (required by abapGit)
    " Using FULL folder logic for multi-package exports (files organized by package)
    DATA(lv_abapgit_xml) =
      |<?xml version="1.0" encoding="utf-8"?>\n| &&
      |<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">\n| &&
      | <asx:values>\n| &&
      |  <DATA>\n| &&
      |   <MASTER_LANGUAGE>E</MASTER_LANGUAGE>\n| &&
      |   <STARTING_FOLDER>/src/</STARTING_FOLDER>\n| &&
      |   <FOLDER_LOGIC>FULL</FOLDER_LOGIC>\n| &&
      |  </DATA>\n| &&
      | </asx:values>\n| &&
      |</asx:abap>\n|.

    lo_zip->add(
      name    = '.abapgit.xml'
      content = cl_abap_codepage=>convert_to( lv_abapgit_xml )
    ).

    LOOP AT it_tadir INTO DATA(ls_tadir).
      DATA(ls_item) = VALUE zif_abapgit_definitions=>ty_item(
        obj_type = ls_tadir-object
        obj_name = ls_tadir-obj_name
        devclass = ls_tadir-devclass
      ).

      TRY.
          DATA(ls_serialized) = zcl_abapgit_objects=>serialize(
            is_item        = ls_item
            io_i18n_params = lo_i18n
          ).

          LOOP AT ls_serialized-files INTO DATA(ls_file).
            " FULL folder logic: src/{package}/{filename}
            DATA(lv_package) = to_lower( ls_tadir-devclass ).
            DATA(lv_path) = |src/{ lv_package }/{ ls_file-filename }|.

            lo_zip->add(
              name    = lv_path
              content = ls_file-data
            ).

            APPEND VALUE #(
              path = lv_path
              size = xstrlen( ls_file-data )
            ) TO et_files.
          ENDLOOP.

        CATCH zcx_abapgit_exception INTO DATA(lx_error).
          " Log error but continue with other objects
          CONTINUE.
      ENDTRY.
    ENDLOOP.

    DATA(lv_zip_xstring) = lo_zip->save( ).
    ev_zip_base64 = xstring_to_base64( lv_zip_xstring ).
  ENDMETHOD.

  METHOD base64_to_xstring.
    CALL FUNCTION 'SSFC_BASE64_DECODE'
      EXPORTING
        b64data = iv_base64
      IMPORTING
        bindata = rv_xstring
      EXCEPTIONS
        OTHERS  = 1.
  ENDMETHOD.

  METHOD xstring_to_base64.
    CALL FUNCTION 'SSFC_BASE64_ENCODE'
      EXPORTING
        bindata = iv_xstring
      IMPORTING
        b64data = rv_base64
      EXCEPTIONS
        OTHERS  = 1.
  ENDMETHOD.

  METHOD json_escape.
    rv_escaped = iv_string.
    REPLACE ALL OCCURRENCES OF '\' IN rv_escaped WITH '\\'.
    REPLACE ALL OCCURRENCES OF '"' IN rv_escaped WITH '\"'.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>cr_lf IN rv_escaped WITH '\n'.
    REPLACE ALL OCCURRENCES OF cl_abap_char_utilities=>newline IN rv_escaped WITH '\n'.
  ENDMETHOD.

  METHOD build_json_response.
    rs_response-id = iv_id.
    rs_response-success = iv_success.

    IF iv_data IS NOT INITIAL.
      rs_response-data = iv_data.
    ENDIF.

    IF iv_error IS NOT INITIAL.
      rs_response-error = |\{"code":"GIT_ERROR","message":"{ json_escape( iv_error ) }"\}|.
    ENDIF.
  ENDMETHOD.

ENDCLASS.
