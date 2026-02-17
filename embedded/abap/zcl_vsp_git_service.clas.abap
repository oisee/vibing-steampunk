"! <p class="shorttext synchronized">VSP Git Service - abapGit Integration</p>
"! Provides import/export of ABAP objects using abapGit serialization.
"! Supports 150+ object types.
CLASS zcl_vsp_git_service DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    INTERFACES zif_vsp_service.

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

    METHODS handle_get_types
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_export
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_import
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response.

    METHODS handle_validate
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
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
      WHEN 'import'.
        rs_response = handle_import( is_message ).
      WHEN 'validate'.
        rs_response = handle_validate( is_message ).
      WHEN OTHERS.
        rs_response = build_json_response(
          iv_id      = is_message-id
          iv_success = abap_false
          iv_error   = |Unknown action: { is_message-action }|
        ).
    ENDCASE.
  ENDMETHOD.

  METHOD zif_vsp_service~on_disconnect.
    " No cleanup needed for git domain
  ENDMETHOD.

  METHOD handle_get_types.
    DATA: lt_types     TYPE zif_abapgit_objects=>ty_types_tt,
          lv_types_json TYPE string.

    TRY.
        lt_types = zcl_abapgit_objects=>supported_list( ).

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

  METHOD handle_import.
    " TODO: Implement using ZCL_ABAPGIT_OBJECTS=>deserialize
    " Requires creating a virtual repo implementation
    rs_response = build_json_response(
      iv_id      = is_message-id
      iv_success = abap_false
      iv_error   = 'Import not yet implemented - use standard ADT for now'
    ).
  ENDMETHOD.

  METHOD handle_validate.
    " TODO: Implement using ZCL_ABAPGIT_OBJECTS=>deserialize_checks
    rs_response = build_json_response(
      iv_id      = is_message-id
      iv_success = abap_false
      iv_error   = 'Validate not yet implemented'
    ).
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
