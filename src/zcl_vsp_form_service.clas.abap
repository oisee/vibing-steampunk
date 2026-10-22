"! <p class="shorttext synchronized">VSP Form Service: SAPscript, Smart Forms, Adobe forms</p>
"! Domain "form". Reads and writes print forms as documents:
"! <ul>
"! <li>SSFO - Smart Form, the XML of the SMARTFORMS download</li>
"! <li>FORM - SAPscript form, abapGit's FORM structure, all languages, the
"!     text lines of each language inline</li>
"! <li>SFPF - Adobe form without the layout of its original language
"!     (CL_FP_HELPER, as abapGit); with a language, the XDP layout of that
"!     language</li>
"! <li>SFPI - Adobe form interface (CL_FP_HELPER, as abapGit)</li>
"! </ul>
"! Content travels base64-encoded: the message parser of this package does
"! not survive quotes and line breaks inside a parameter.
"! <p>Nothing here activates. The workbench activation waits for asynchronous
"! tasks, which an APC session forbids. Adobe objects are saved inactive and
"! the client activates them over ADT; Smart Forms are stored active by their
"! own API; SAPscript has no inactive version.</p>
CLASS zcl_vsp_form_service DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    INTERFACES zif_vsp_service.

  PRIVATE SECTION.
    CONSTANTS:
      c_ssfo TYPE trobjtype VALUE 'SSFO',
      c_form TYPE trobjtype VALUE 'FORM',
      c_sfpf TYPE trobjtype VALUE 'SFPF',
      c_sfpi TYPE trobjtype VALUE 'SFPI'.

    " the FORM structure of abapGit's form serializer, plus the text lines
    " that abapGit keeps in a file of their own.
    TYPES:
      BEGIN OF ty_form_data,
        form_header   TYPE itcta,
        text_header   TYPE thead,
        orig_language TYPE sy-langu,
        pages         TYPE STANDARD TABLE OF itctg WITH DEFAULT KEY,
        page_windows  TYPE STANDARD TABLE OF itcth WITH DEFAULT KEY,
        paragraphs    TYPE STANDARD TABLE OF itcdp WITH DEFAULT KEY,
        strings       TYPE STANDARD TABLE OF itcds WITH DEFAULT KEY,
        tabs          TYPE STANDARD TABLE OF itcdq WITH DEFAULT KEY,
        windows       TYPE STANDARD TABLE OF itctw WITH DEFAULT KEY,
        tdlines       TYPE STANDARD TABLE OF tline WITH DEFAULT KEY,
      END OF ty_form_data,
      ty_form_data_tt TYPE STANDARD TABLE OF ty_form_data WITH DEFAULT KEY,
      ty_languages    TYPE STANDARD TABLE OF spras WITH DEFAULT KEY,
      ty_header_captions TYPE STANDARD TABLE OF stxfadmt WITH DEFAULT KEY.

    TYPES:
      BEGIN OF ty_request,
        type      TYPE trobjtype,
        name      TYPE sobj_name,
        language  TYPE spras,
        transport TYPE trkorr,
        package   TYPE devclass,
        test_run  TYPE abap_bool,
        content   TYPE xstring,
      END OF ty_request.

    TYPES:
      BEGIN OF ty_write_result,
        created  TYPE abap_bool,
        activate TYPE abap_bool,
        backup   TYPE xstring,
      END OF ty_write_result.

    METHODS handle_info
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response
      RAISING   zcx_vsp_form.

    METHODS handle_read
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response
      RAISING   zcx_vsp_form.

    METHODS handle_write
      IMPORTING is_message         TYPE zif_vsp_service=>ty_message
      RETURNING VALUE(rs_response) TYPE zif_vsp_service=>ty_response
      RAISING   zcx_vsp_form.

    METHODS parse_request
      IMPORTING is_message        TYPE zif_vsp_service=>ty_message
                iv_with_content   TYPE abap_bool DEFAULT abap_false
      RETURNING VALUE(rs_request) TYPE ty_request
      RAISING   zcx_vsp_form.

    " --- Smart Forms
    METHODS ssf_read
      IMPORTING iv_name       TYPE tdsfname
      RETURNING VALUE(rv_xml) TYPE xstring
      RAISING   zcx_vsp_form.

    METHODS ssf_write
      IMPORTING is_request       TYPE ty_request
      RETURNING VALUE(rs_result) TYPE ty_write_result
      RAISING   zcx_vsp_form.

    METHODS ssf_restore_captions
      IMPORTING iv_name    TYPE tdsfname
                io_form    TYPE REF TO if_ixml_element
                it_headers TYPE ty_header_captions.

    " --- SAPscript
    METHODS form_read
      IMPORTING iv_name       TYPE tdform
                iv_language   TYPE spras OPTIONAL
      RETURNING VALUE(rv_xml) TYPE xstring
      RAISING   zcx_vsp_form.

    METHODS form_write
      IMPORTING is_request       TYPE ty_request
      RETURNING VALUE(rs_result) TYPE ty_write_result
      RAISING   zcx_vsp_form.

    METHODS form_languages
      IMPORTING iv_name             TYPE tdform
      RETURNING VALUE(rt_languages) TYPE ty_languages.

    METHODS sort_lines_by_window
      CHANGING ct_windows TYPE ty_form_data-windows
               ct_lines   TYPE ty_form_data-tdlines.

    " --- Adobe forms
    METHODS sfp_load_form
      IMPORTING iv_name      TYPE fpname
                iv_language  TYPE spras
                iv_mode      TYPE string DEFAULT if_fp_wb_object=>c_mode_read
                iv_transport TYPE trkorr OPTIONAL
      RETURNING VALUE(ro_wb) TYPE REF TO if_fp_wb_form
      RAISING   zcx_vsp_form.

    METHODS sfp_read_form
      IMPORTING iv_name       TYPE fpname
      RETURNING VALUE(rv_xml) TYPE xstring
      RAISING   zcx_vsp_form.

    METHODS sfp_read_layout
      IMPORTING iv_name       TYPE fpname
                iv_language   TYPE spras
      RETURNING VALUE(rv_xdp) TYPE xstring
      RAISING   zcx_vsp_form.

    METHODS sfp_read_interface
      IMPORTING iv_name       TYPE fpname
      RETURNING VALUE(rv_xml) TYPE xstring
      RAISING   zcx_vsp_form.

    METHODS sfp_write_form
      IMPORTING is_request       TYPE ty_request
      RETURNING VALUE(rs_result) TYPE ty_write_result
      RAISING   zcx_vsp_form.

    METHODS sfp_write_layout
      IMPORTING is_request       TYPE ty_request
      RETURNING VALUE(rs_result) TYPE ty_write_result
      RAISING   zcx_vsp_form.

    METHODS sfp_write_interface
      IMPORTING is_request       TYPE ty_request
      RETURNING VALUE(rs_result) TYPE ty_write_result
      RAISING   zcx_vsp_form.

    METHODS sfp_layout_languages
      IMPORTING iv_name             TYPE fpname
      RETURNING VALUE(rt_languages) TYPE ty_languages.

    METHODS sfp_free
      IMPORTING io_wb TYPE REF TO if_fp_wb_object.

    " --- Shared
    METHODS tadir_entry
      IMPORTING iv_type     TYPE trobjtype
                iv_name     TYPE csequence
      EXPORTING ev_exists   TYPE abap_bool
                ev_package  TYPE devclass
                ev_language TYPE spras.

    METHODS tadir_insert
      IMPORTING iv_type     TYPE trobjtype
                iv_name     TYPE csequence
                iv_package  TYPE devclass
                iv_language TYPE spras
      RAISING   zcx_vsp_form.

    METHODS tadir_delete
      IMPORTING iv_type TYPE trobjtype
                iv_name TYPE csequence.

    METHODS check_transport
      IMPORTING iv_type      TYPE trobjtype
                iv_name      TYPE csequence
                iv_package   TYPE devclass
                iv_transport TYPE trkorr
      RAISING   zcx_vsp_form.

    METHODS register_in_transport
      IMPORTING iv_type      TYPE trobjtype
                iv_name      TYPE csequence
                iv_package   TYPE devclass
                iv_transport TYPE trkorr
      RAISING   zcx_vsp_form.

    METHODS check_writable
      IMPORTING iv_name TYPE csequence
      RAISING   zcx_vsp_form.

    METHODS check_xml
      IMPORTING iv_xml TYPE xstring
      RAISING   zcx_vsp_form.

    METHODS is_inactive
      IMPORTING iv_type          TYPE trobjtype
                iv_name          TYPE csequence
      RETURNING VALUE(rv_result) TYPE abap_bool.

    METHODS to_iso
      IMPORTING iv_language   TYPE spras
      RETURNING VALUE(rv_iso) TYPE string.

    METHODS to_spras
      IMPORTING iv_iso             TYPE string
      RETURNING VALUE(rv_language) TYPE spras
      RAISING   zcx_vsp_form.

    METHODS languages_json
      IMPORTING it_languages   TYPE ty_languages
      RETURNING VALUE(rv_json) TYPE string.

    METHODS fail
      IMPORTING iv_code TYPE string DEFAULT 'FORM_ERROR'
                iv_text TYPE string
                ix_prev TYPE REF TO cx_root OPTIONAL
      RAISING   zcx_vsp_form.

    METHODS message_text
      RETURNING VALUE(rv_text) TYPE string.

ENDCLASS.


CLASS zcl_vsp_form_service IMPLEMENTATION.

  METHOD zif_vsp_service~get_domain.
    rv_domain = 'form'.
  ENDMETHOD.


  METHOD zif_vsp_service~handle_message.
    TRY.
        CASE is_message-action.
          WHEN 'info'.
            rs_response = handle_info( is_message ).
          WHEN 'read'.
            rs_response = handle_read( is_message ).
          WHEN 'write'.
            rs_response = handle_write( is_message ).
          WHEN OTHERS.
            rs_response = zcl_vsp_utils=>build_error(
              iv_id      = is_message-id
              iv_code    = 'UNKNOWN_ACTION'
              iv_message = |Action '{ is_message-action }' not supported| ).
        ENDCASE.
      CATCH zcx_vsp_form INTO DATA(lx_form).
        rs_response = zcl_vsp_utils=>build_error(
          iv_id      = is_message-id
          iv_code    = lx_form->mv_code
          iv_message = lx_form->mv_text ).
    ENDTRY.
  ENDMETHOD.


  METHOD zif_vsp_service~on_disconnect.
  ENDMETHOD.


  METHOD handle_info.
    DATA lt_languages TYPE ty_languages.

    DATA(ls_request) = parse_request( is_message ).
    tadir_entry( EXPORTING iv_type     = ls_request-type
                           iv_name     = ls_request-name
                 IMPORTING ev_exists   = DATA(lv_exists)
                           ev_package  = DATA(lv_package)
                           ev_language = DATA(lv_language) ).

    IF lv_exists = abap_true.
      CASE ls_request-type.
        WHEN c_form.
          lt_languages = form_languages( CONV #( ls_request-name ) ).
        WHEN c_sfpf.
          lt_languages = sfp_layout_languages( CONV #( ls_request-name ) ).
        WHEN OTHERS.
          APPEND lv_language TO lt_languages.
      ENDCASE.
    ENDIF.

    rs_response = zcl_vsp_utils=>build_success(
      iv_id   = is_message-id
      iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
        ( zcl_vsp_utils=>json_str( iv_key = 'type' iv_value = CONV #( ls_request-type ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'name' iv_value = CONV #( ls_request-name ) ) )
        ( zcl_vsp_utils=>json_bool( iv_key = 'exists' iv_value = lv_exists ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'package' iv_value = CONV #( lv_package ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'masterLanguage' iv_value = to_iso( lv_language ) ) )
        ( |"languages":{ languages_json( lt_languages ) }| )
        ( zcl_vsp_utils=>json_bool( iv_key = 'inactive'
                                    iv_value = is_inactive( iv_type = ls_request-type iv_name = ls_request-name ) ) ) ) ) ) ).
  ENDMETHOD.


  METHOD handle_read.
    DATA lv_content TYPE xstring.
    DATA lv_mime    TYPE string VALUE 'application/xml'.

    DATA(ls_request) = parse_request( is_message ).
    tadir_entry( EXPORTING iv_type     = ls_request-type
                           iv_name     = ls_request-name
                 IMPORTING ev_exists   = DATA(lv_exists)
                           ev_language = DATA(lv_language) ).
    " SAP's own SAPscript forms often live in client 000 only, without a
    " TADIR entry in the customer system; READ_FORM finds them anyway.
    IF lv_exists = abap_false AND ls_request-type <> c_form.
      fail( iv_code = 'NOT_FOUND' iv_text = |{ ls_request-type } { ls_request-name } does not exist| ).
    ENDIF.

    CASE ls_request-type.
      WHEN c_ssfo.
        lv_content = ssf_read( CONV #( ls_request-name ) ).
      WHEN c_form.
        lv_content = form_read( iv_name = CONV #( ls_request-name ) iv_language = ls_request-language ).
      WHEN c_sfpf.
        IF ls_request-language IS INITIAL.
          lv_content = sfp_read_form( CONV #( ls_request-name ) ).
        ELSE.
          lv_content = sfp_read_layout( iv_name = CONV #( ls_request-name ) iv_language = ls_request-language ).
          lv_mime = 'application/vnd.adobe.xdp+xml'.
        ENDIF.
      WHEN c_sfpi.
        lv_content = sfp_read_interface( CONV #( ls_request-name ) ).
    ENDCASE.

    rs_response = zcl_vsp_utils=>build_success(
      iv_id   = is_message-id
      iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
        ( zcl_vsp_utils=>json_str( iv_key = 'type' iv_value = CONV #( ls_request-type ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'name' iv_value = CONV #( ls_request-name ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'masterLanguage' iv_value = to_iso( lv_language ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'mimeType' iv_value = lv_mime ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'contentBase64'
                                   iv_value = cl_http_utility=>encode_x_base64( lv_content ) ) ) ) ) ) ).
  ENDMETHOD.


  METHOD handle_write.
    DATA ls_result TYPE ty_write_result.

    DATA(ls_request) = parse_request( is_message = is_message iv_with_content = abap_true ).
    check_writable( ls_request-name ).
    check_xml( ls_request-content ).

    CASE ls_request-type.
      WHEN c_ssfo.
        ls_result = ssf_write( ls_request ).
      WHEN c_form.
        ls_result = form_write( ls_request ).
      WHEN c_sfpf.
        IF ls_request-language IS INITIAL.
          ls_result = sfp_write_form( ls_request ).
        ELSE.
          ls_result = sfp_write_layout( ls_request ).
        ENDIF.
      WHEN c_sfpi.
        ls_result = sfp_write_interface( ls_request ).
    ENDCASE.

    rs_response = zcl_vsp_utils=>build_success(
      iv_id   = is_message-id
      iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(
        ( zcl_vsp_utils=>json_str( iv_key = 'type' iv_value = CONV #( ls_request-type ) ) )
        ( zcl_vsp_utils=>json_str( iv_key = 'name' iv_value = CONV #( ls_request-name ) ) )
        ( zcl_vsp_utils=>json_bool( iv_key = 'testRun' iv_value = ls_request-test_run ) )
        ( zcl_vsp_utils=>json_bool( iv_key = 'saved' iv_value = xsdbool( ls_request-test_run = abap_false ) ) )
        ( zcl_vsp_utils=>json_bool( iv_key = 'created' iv_value = ls_result-created ) )
        ( zcl_vsp_utils=>json_bool( iv_key = 'activationRequired' iv_value = ls_result-activate ) )
        ( COND #( WHEN ls_result-backup IS NOT INITIAL
                  THEN zcl_vsp_utils=>json_str( iv_key = 'backupBase64'
                                                iv_value = cl_http_utility=>encode_x_base64( ls_result-backup ) ) ) ) ) ) ) ).
  ENDMETHOD.


  METHOD parse_request.
    DATA(lv_params) = is_message-params.

    rs_request-type = to_upper( zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'type' ) ).
    rs_request-name = to_upper( zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'name' ) ).
    IF rs_request-type <> c_ssfo AND rs_request-type <> c_form
       AND rs_request-type <> c_sfpf AND rs_request-type <> c_sfpi.
      fail( iv_code = 'INVALID_TYPE' iv_text = |Type '{ rs_request-type }' is not one of SSFO, FORM, SFPF, SFPI| ).
    ENDIF.
    IF rs_request-name IS INITIAL.
      fail( iv_code = 'MISSING_PARAM' iv_text = 'Parameter name is required' ).
    ENDIF.

    DATA(lv_language) = zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'language' ).
    IF lv_language IS NOT INITIAL.
      rs_request-language = to_spras( lv_language ).
    ENDIF.
    rs_request-transport = to_upper( zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'transport' ) ).
    rs_request-package   = to_upper( zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'package' ) ).
    rs_request-test_run  = xsdbool( zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'testRun' ) = 'X' ).

    IF iv_with_content = abap_true.
      DATA(lv_base64) = zcl_vsp_utils=>extract_param( iv_params = lv_params iv_name = 'contentBase64' ).
      IF lv_base64 IS INITIAL.
        fail( iv_code = 'MISSING_PARAM' iv_text = 'Parameter contentBase64 is required' ).
      ENDIF.
      rs_request-content = cl_http_utility=>decode_x_base64( lv_base64 ).
      IF rs_request-content IS INITIAL.
        fail( iv_code = 'INVALID_PARAM' iv_text = 'contentBase64 is not valid base64' ).
      ENDIF.
    ENDIF.
  ENDMETHOD.



  METHOD ssf_read.
    " The layout SMARTFORMS writes on download: <sf:SMARTFORM> as the root,
    " both namespaces and sf:language. Without them XML_UPLOAD skips the
    " whole content on the way back, without an error.
    CONSTANTS:
      lc_ns_uri_sf  TYPE string VALUE 'urn:sap-com:SmartForms:2000:internal-structure',
      lc_ns_uri_ifr TYPE string VALUE 'urn:sap-com:sdixml-ifr:2000'.
    DATA lo_form    TYPE REF TO cl_ssf_fb_smart_form.
    DATA lv_dummy   TYPE tdfmnumb.
    DATA lv_master  TYPE sylangu.

    " Load in the original language: the download carries the texts of the
    " language it was loaded in, and an upload writes them as originals.
    SELECT SINGLE masterlang FROM stxfadm WHERE formname = @iv_name INTO @lv_master.
    IF lv_master IS INITIAL.
      lv_master = sy-langu.
    ENDIF.

    CREATE OBJECT lo_form.
    TRY.
        lo_form->load( EXPORTING im_formname    = iv_name
                                 im_active      = abap_true
                                 im_language    = lv_master
                       IMPORTING ex_fmnumb      = lv_dummy
                                 ex_fmnumb_test = lv_dummy ).
      CATCH cx_ssf_fb INTO DATA(lx_load).
        fail( iv_code = 'NOT_FOUND' iv_text = |Smart Form { iv_name } could not be loaded| ix_prev = lx_load ).
    ENDTRY.

    DATA(lo_ixml) = cl_ixml=>create( ).
    DATA(lo_doc)  = lo_ixml->create_document( ).
    lo_doc->set_encoding( lo_ixml->create_encoding( byte_order    = if_ixml_encoding=>co_none
                                                    character_set = 'utf-8' ) ).
    DATA(lo_root) = lo_doc->create_element_ns( name = 'SMARTFORM' prefix = 'sf' ).
    lo_root->set_attribute_ns( name = 'xmlns' value = lc_ns_uri_ifr ).
    lo_root->set_attribute_ns( name = 'xmlns:sf' value = lc_ns_uri_sf ).
    lo_root->set_attribute_ns( name = 'sf:language' value = to_iso( lv_master ) ).
    lo_doc->append_child( lo_root ).

    lo_form->xml_download( EXPORTING parent   = lo_root
                           CHANGING  document = lo_doc ).

    " XML_DOWNLOAD adds its own <sf:SMARTFORM> under PARENT; lift its
    " children to the root and drop the duplicate wrapper.
    DATA(lo_inner) = lo_root->get_first_child( ).
    IF lo_inner IS BOUND AND lo_inner->get_name( ) = 'SMARTFORM'.
      DATA(lo_child) = lo_inner->get_first_child( ).
      WHILE lo_child IS BOUND.
        DATA(lo_next) = lo_child->get_next( ).
        lo_inner->remove_child( lo_child ).
        lo_root->append_child( lo_child ).
        lo_child = lo_next.
      ENDWHILE.
      lo_root->remove_child( lo_inner ).
    ENDIF.

    DATA(lo_stream) = lo_ixml->create_stream_factory( )->create_ostream_xstring( string = rv_xml ).
    lo_ixml->create_renderer( document = lo_doc ostream = lo_stream )->render( ).
  ENDMETHOD.


  METHOD ssf_write.
    DATA lo_form      TYPE REF TO zcl_vsp_ssf_silent.
    DATA lo_form_base TYPE REF TO cl_ssf_fb_smart_form.
    DATA lo_sf_el     TYPE REF TO if_ixml_element.
    DATA lv_name      TYPE tdsfname.
    DATA lv_master    TYPE sylangu.

    " Parse with whitespace kept: leading blanks in text lines indent the
    " text, and iXML's default normalisation drops them on the way back.
    DATA(lv_content) = is_request-content.
    DATA(lo_ixml)   = cl_ixml=>create( ).
    DATA(lo_doc)    = lo_ixml->create_document( ).
    DATA(lo_sf)     = lo_ixml->create_stream_factory( ).
    DATA(lo_parser) = lo_ixml->create_parser( stream_factory = lo_sf
                                              istream        = lo_sf->create_istream_xstring( string = lv_content )
                                              document       = lo_doc ).
    " SPIKE parser mode
    lo_parser->set_normalizing( abap_false ).
    lo_parser->add_preserve_space_element( ).
    IF lo_parser->parse( ) <> 0.
      fail( iv_code = 'INVALID_CONTENT' iv_text = 'The Smart Form XML could not be parsed' ).
    ENDIF.

    DATA(lo_root) = lo_doc->get_root_element( ).
    IF lo_root IS BOUND AND ( lo_root->get_name( ) = 'SMARTFORM' OR lo_root->get_name( ) = 'sf:SMARTFORM' ).
      lo_sf_el = lo_root.
    ELSE.
      lo_sf_el ?= lo_doc->find_from_name( name = 'SMARTFORM' ).
    ENDIF.
    IF lo_sf_el IS NOT BOUND.
      fail( iv_code = 'INVALID_CONTENT' iv_text = 'No <SMARTFORM> element in the content' ).
    ENDIF.
    DATA(lo_header) = lo_sf_el->find_from_name( name = 'HEADER' ).
    IF lo_header IS BOUND.
      DATA(lo_formname) = lo_header->find_from_name( name = 'FORMNAME' ).
      DATA(lo_masterlang) = lo_header->find_from_name( name = 'MASTERLANG' ).
    ENDIF.
    IF lo_formname IS NOT BOUND OR lo_masterlang IS NOT BOUND.
      fail( iv_code = 'INVALID_CONTENT' iv_text = 'HEADER/FORMNAME and HEADER/MASTERLANG are required' ).
    ENDIF.
    lv_name   = lo_formname->get_value( ).
    lv_master = lo_masterlang->get_value( ).
    IF lv_name <> is_request-name.
      fail( iv_code = 'NAME_MISMATCH' iv_text = |The content is Smart Form { lv_name }, not { is_request-name }| ).
    ENDIF.

    tadir_entry( EXPORTING iv_type     = c_ssfo
                           iv_name     = lv_name
                 IMPORTING ev_exists   = DATA(lv_exists)
                           ev_package  = DATA(lv_package) ).
    IF lv_exists = abap_true.
      " Writing in another language than the original drops every text
      " that has no translation in it - silently.
      SELECT SINGLE masterlang FROM stxfadm WHERE formname = @lv_name INTO @DATA(lv_db_master).
      IF lv_db_master IS NOT INITIAL AND lv_db_master <> lv_master.
        fail( iv_code = 'LANGUAGE_MISMATCH'
              iv_text = |The content is in { to_iso( lv_master ) }, Smart Form { lv_name } in { to_iso( lv_db_master ) }| ).
      ENDIF.
    ELSE.
      IF is_request-package IS INITIAL.
        fail( iv_code = 'MISSING_PARAM' iv_text = |Smart Form { lv_name } does not exist; a package is required to create it| ).
      ENDIF.
      lv_package = is_request-package.
    ENDIF.

    check_transport( iv_type = c_ssfo iv_name = lv_name iv_package = lv_package iv_transport = is_request-transport ).
    IF is_request-test_run = abap_true.
      RETURN.
    ENDIF.

    rs_result-created = xsdbool( lv_exists = abap_false ).
    IF rs_result-created = abap_true.
      tadir_insert( iv_type = c_ssfo iv_name = lv_name iv_package = lv_package iv_language = lv_master ).
    ENDIF.
    DATA lt_header_captions TYPE ty_header_captions.
    SELECT * FROM stxfadmt WHERE formname = @lv_name INTO TABLE @lt_header_captions.

    TRY.
        register_in_transport( iv_type = c_ssfo iv_name = lv_name iv_package = lv_package
                               iv_transport = is_request-transport ).

        CREATE OBJECT lo_form.
        lo_form_base = lo_form.
        TRY.
            lo_form->enqueue_silent( im_formname   = lv_name
                                     im_korrnum    = is_request-transport
                                     im_masterlang = lv_master
                                     im_mode       = COND #( WHEN lv_exists = abap_true THEN 'MODIFY' ELSE 'INSERT' ) ).
          CATCH cx_ssf_fb INTO DATA(lx_lock).
            fail( iv_code = 'LOCKED' iv_text = |Smart Form { lv_name } could not be locked: { message_text( ) }| ix_prev = lx_lock ).
        ENDTRY.

        TRY.
            lo_form->xml_upload( EXPORTING dom      = lo_sf_el
                                           formname = lv_name
                                           language = lv_master
                                 CHANGING  sform    = lo_form_base ).
            " The redefined SET_CORRECTION_REQUEST keeps STORE from asking.
            lo_form->store( im_active   = abap_true
                            im_formname = lv_name
                            im_language = lv_master ).
          CATCH cx_root INTO DATA(lx_store).
            lo_form->dequeue( formname = lv_name ).
            fail( iv_code = 'SAVE_FAILED' iv_text = |Smart Form { lv_name }: { lx_store->get_text( ) }| ix_prev = lx_store ).
        ENDTRY.
        lo_form->dequeue( formname = lv_name ).

        ssf_restore_captions( iv_name = lv_name io_form = lo_sf_el it_headers = lt_header_captions ).
        " STORE( active ) can leave an inactive-object entry behind.
        DATA(lv_obj_name) = CONV trobj_name( lv_name ).
        DELETE FROM dwinactiv WHERE object = @c_ssfo AND obj_name = @lv_obj_name.
        " A plain COMMIT WORK: COMMIT WORK AND WAIT is forbidden in APC.
        COMMIT WORK.
      CATCH zcx_vsp_form INTO DATA(lx_failed).
        IF rs_result-created = abap_true.
          tadir_delete( iv_type = c_ssfo iv_name = lv_name ).
        ENDIF.
        RAISE EXCEPTION lx_failed.
    ENDTRY.
  ENDMETHOD.



  METHOD ssf_restore_captions.
    " XML_UPLOAD and STORE lose the descriptions of the nodes (STXFOBJT):
    " STORE writes each node's CAPTION field over the language rows of
    " T_CAPTION, and after the upload that field is empty - also through
    " the SMARTFORMS API in an HTTP session, so not an APC matter. The
    " document carries every description in T_CAPTION, all languages; they
    " are written back here. The description of the form itself (STXFADMT)
    " is not in the download at all: the one it had is kept.
    DATA lt_objt TYPE STANDARD TABLE OF stxfobjt WITH DEFAULT KEY.

    DATA(lo_lists) = io_form->get_elements_by_tag_name( name = 'T_CAPTION' ).
    DO lo_lists->get_length( ) TIMES.
      DATA(lo_item) = lo_lists->get_item( sy-index - 1 )->get_first_child( ).
      WHILE lo_item IS BOUND.
        DATA(ls_objt) = VALUE stxfobjt( formname = iv_name ).
        DATA(lo_field) = lo_item->get_first_child( ).
        WHILE lo_field IS BOUND.
          CASE lo_field->get_name( ).
            WHEN 'LANGU'.   ls_objt-langu   = lo_field->get_value( ).
            WHEN 'OBJTYPE'. ls_objt-objtype = lo_field->get_value( ).
            WHEN 'INAME'.   ls_objt-iname   = lo_field->get_value( ).
            WHEN 'VARI'.    ls_objt-vari    = lo_field->get_value( ).
            WHEN 'CAPTION'. ls_objt-caption = lo_field->get_value( ).
          ENDCASE.
          lo_field = lo_field->get_next( ).
        ENDWHILE.
        IF ls_objt-langu IS NOT INITIAL AND ls_objt-iname IS NOT INITIAL AND ls_objt-caption IS NOT INITIAL.
          APPEND ls_objt TO lt_objt.
        ENDIF.
        lo_item = lo_item->get_next( ).
      ENDWHILE.
    ENDDO.
    IF lt_objt IS NOT INITIAL.
      MODIFY stxfobjt FROM TABLE @lt_objt.
    ENDIF.

    LOOP AT it_headers INTO DATA(ls_header) WHERE caption IS NOT INITIAL.
      UPDATE stxfadmt SET caption = @ls_header-caption
        WHERE formname = @ls_header-formname
          AND langu    = @ls_header-langu
          AND caption  = @space.
    ENDLOOP.
  ENDMETHOD.


  METHOD form_languages.
    SELECT DISTINCT tdspras FROM stxh
      WHERE tdobject = 'FORM'
        AND tdname   = @iv_name
        AND tdid     = 'TXT'
      ORDER BY tdspras
      INTO TABLE @rt_languages.
  ENDMETHOD.


  METHOD form_read.
    DATA lt_languages TYPE ty_languages.
    DATA lt_forms     TYPE ty_form_data_tt.
    DATA lv_found     TYPE c LENGTH 1.

    IF iv_language IS NOT INITIAL.
      APPEND iv_language TO lt_languages.
    ELSE.
      lt_languages = form_languages( iv_name ).
      IF lt_languages IS INITIAL.
        tadir_entry( EXPORTING iv_type = c_form iv_name = iv_name IMPORTING ev_language = DATA(lv_master) ).
        APPEND COND spras( WHEN lv_master IS NOT INITIAL THEN lv_master ELSE sy-langu ) TO lt_languages.
      ENDIF.
    ENDIF.

    LOOP AT lt_languages INTO DATA(lv_language).
      DATA(ls_form) = VALUE ty_form_data( ).
      " No STATUS = 'A': SAP's own forms have no active-status row and would
      " not be found. THROUGHCLIENT reads client 000 where the customer
      " client has nothing.
      CALL FUNCTION 'READ_FORM'
        EXPORTING
          form            = iv_name
          language        = lv_language
          throughclient   = abap_true
          throughlanguage = abap_true
        IMPORTING
          form_header     = ls_form-form_header
          found           = lv_found
          header          = ls_form-text_header
          olanguage       = ls_form-orig_language
        TABLES
          form_lines      = ls_form-tdlines
          pages           = ls_form-pages
          page_windows    = ls_form-page_windows
          paragraphs      = ls_form-paragraphs
          strings         = ls_form-strings
          tabs            = ls_form-tabs
          windows         = ls_form-windows.
      IF lv_found <> abap_true.
        IF iv_language IS NOT INITIAL.
          fail( iv_code = 'NOT_FOUND' iv_text = |SAPscript form { iv_name } has no language { to_iso( iv_language ) }| ).
        ENDIF.
        CONTINUE.
      ENDIF.

      sort_lines_by_window( CHANGING ct_windows = ls_form-windows ct_lines = ls_form-tdlines ).
      " As abapGit: who changed it and when is noise in a comparison.
      CLEAR: ls_form-form_header-tdfuser, ls_form-form_header-tdfdate, ls_form-form_header-tdftime,
             ls_form-form_header-tdfreles, ls_form-form_header-tdluser, ls_form-form_header-tdldate,
             ls_form-form_header-tdltime, ls_form-form_header-tdlreles,
             ls_form-text_header-tdfuser, ls_form-text_header-tdfdate, ls_form-text_header-tdftime,
             ls_form-text_header-tdfreles, ls_form-text_header-tdluser, ls_form-text_header-tdldate,
             ls_form-text_header-tdltime, ls_form-text_header-tdlreles.
      ls_form-form_header-tdversion = '00001'.
      ls_form-text_header-tdversion = '00001'.
      APPEND ls_form TO lt_forms.
    ENDLOOP.

    IF lt_forms IS INITIAL.
      fail( iv_code = 'NOT_FOUND' iv_text = |SAPscript form { iv_name } was not found| ).
    ENDIF.

    CALL TRANSFORMATION id SOURCE form = lt_forms RESULT XML rv_xml.
  ENDMETHOD.


  METHOD sort_lines_by_window.
    " Window blocks in window order, as abapGit does, so that two reads
    " compare line by line. Unlike abapGit nothing is dropped: lines before
    " the first /W line, and blocks of windows not in WINDOWS, keep their
    " place behind the sorted blocks.
    TYPES: BEGIN OF ty_block,
             window TYPE tdwindow,
             lines  TYPE ty_form_data-tdlines,
           END OF ty_block.
    DATA lt_blocks TYPE STANDARD TABLE OF ty_block WITH DEFAULT KEY.
    DATA lt_lead   TYPE ty_form_data-tdlines.
    DATA lt_sorted TYPE ty_form_data-tdlines.

    LOOP AT ct_lines INTO DATA(ls_line).
      IF ls_line-tdformat = '/W'.
        APPEND VALUE #( window = CONV #( ls_line-tdline ) ) TO lt_blocks.
      ENDIF.
      IF lt_blocks IS INITIAL.
        APPEND ls_line TO lt_lead.
      ELSE.
        ASSIGN lt_blocks[ lines( lt_blocks ) ] TO FIELD-SYMBOL(<ls_current>).
        APPEND ls_line TO <ls_current>-lines.
      ENDIF.
    ENDLOOP.

    SORT ct_windows BY tdwindow.
    APPEND LINES OF lt_lead TO lt_sorted.
    LOOP AT ct_windows INTO DATA(ls_window).
      LOOP AT lt_blocks ASSIGNING FIELD-SYMBOL(<ls_block>) WHERE window = ls_window-tdwindow.
        APPEND LINES OF <ls_block>-lines TO lt_sorted.
        CLEAR <ls_block>-window.
        FREE <ls_block>-lines.
      ENDLOOP.
    ENDLOOP.
    LOOP AT lt_blocks INTO DATA(ls_rest) WHERE lines IS NOT INITIAL.
      APPEND LINES OF ls_rest-lines TO lt_sorted.
    ENDLOOP.
    ct_lines = lt_sorted.
  ENDMETHOD.


  METHOD form_write.
    DATA lt_forms TYPE ty_form_data_tt.
    DATA lv_name  TYPE tdform.

    TRY.
        CALL TRANSFORMATION id SOURCE XML is_request-content RESULT form = lt_forms.
      CATCH cx_root INTO DATA(lx_xml).
        fail( iv_code = 'INVALID_CONTENT' iv_text = |SAPscript content: { lx_xml->get_text( ) }| ix_prev = lx_xml ).
    ENDTRY.
    IF lt_forms IS INITIAL.
      fail( iv_code = 'INVALID_CONTENT' iv_text = 'The content holds no language of the form' ).
    ENDIF.

    lv_name = is_request-name.
    LOOP AT lt_forms ASSIGNING FIELD-SYMBOL(<ls_form>).
      IF <ls_form>-form_header-tdform IS NOT INITIAL AND <ls_form>-form_header-tdform <> lv_name.
        fail( iv_code = 'NAME_MISMATCH' iv_text = |The content is form { <ls_form>-form_header-tdform }, not { lv_name }| ).
      ENDIF.
      IF <ls_form>-form_header-tdspras IS INITIAL.
        fail( iv_code = 'INVALID_CONTENT' iv_text = 'FORM_HEADER-TDSPRAS is required for every language' ).
      ENDIF.
      " Never let a stray header write under another key.
      <ls_form>-form_header-tdform = lv_name.
    ENDLOOP.

    tadir_entry( EXPORTING iv_type     = c_form
                           iv_name     = lv_name
                 IMPORTING ev_exists   = DATA(lv_exists)
                           ev_package  = DATA(lv_package) ).
    IF lv_exists = abap_false.
      IF is_request-package IS INITIAL.
        fail( iv_code = 'MISSING_PARAM' iv_text = |SAPscript form { lv_name } does not exist; a package is required to create it| ).
      ENDIF.
      lv_package = is_request-package.
    ENDIF.

    check_transport( iv_type = c_form iv_name = lv_name iv_package = lv_package iv_transport = is_request-transport ).
    IF is_request-test_run = abap_true.
      RETURN.
    ENDIF.

    DATA(lv_orig) = lt_forms[ 1 ]-orig_language.
    IF lv_orig IS INITIAL.
      lv_orig = lt_forms[ 1 ]-form_header-tdspras.
    ENDIF.

    rs_result-created = xsdbool( lv_exists = abap_false ).
    IF rs_result-created = abap_true.
      tadir_insert( iv_type = c_form iv_name = lv_name iv_package = lv_package iv_language = lv_orig ).
    ENDIF.

    TRY.
        " SAPSCRIPT_ORDER_INSERT is no use here: it opens the request dialog.
        register_in_transport( iv_type = c_form iv_name = lv_name iv_package = lv_package
                               iv_transport = is_request-transport ).

        " Only the languages in the content are written; the others stay.
        LOOP AT lt_forms ASSIGNING <ls_form>.
          CALL FUNCTION 'SAVE_FORM'
            EXPORTING
              form_header  = <ls_form>-form_header
            TABLES
              form_lines   = <ls_form>-tdlines
              pages        = <ls_form>-pages
              page_windows = <ls_form>-page_windows
              paragraphs   = <ls_form>-paragraphs
              strings      = <ls_form>-strings
              tabs         = <ls_form>-tabs
              windows      = <ls_form>-windows.
        ENDLOOP.

        IF rs_result-created = abap_true.
          CALL FUNCTION 'SAPSCRIPT_CHANGE_OLANGUAGE'
            EXPORTING
              forced    = abap_true
              name      = CONV tdobname( lv_name )
              object    = 'FORM'
              olanguage = lv_orig
            EXCEPTIONS
              OTHERS    = 1 ##FM_SUBRC_OK.
        ENDIF.

        CALL FUNCTION 'SAPSCRIPT_DELETE_LOAD'
          EXPORTING
            delete = abap_true
            form   = lv_name
            write  = space.
        COMMIT WORK.
      CATCH zcx_vsp_form INTO DATA(lx_failed).
        IF rs_result-created = abap_true.
          tadir_delete( iv_type = c_form iv_name = lv_name ).
        ENDIF.
        RAISE EXCEPTION lx_failed.
    ENDTRY.
  ENDMETHOD.



  METHOD sfp_load_form.
    " Without I_SUPPRESS_LANGUAGE_CHECK the API ignores I_LANGUAGE and
    " loads in the logon language - the translation instead of the original.
    TRY.
        ro_wb = cl_fp_wb_form=>load( i_name                    = iv_name
                                     i_mode                    = iv_mode
                                     i_language                = iv_language
                                     i_suppress_language_check = abap_true
                                     i_ordernum                = iv_transport
                                     i_dark                    = xsdbool( iv_mode = if_fp_wb_object=>c_mode_write ) ).
      CATCH cx_fp_api INTO DATA(lx_fp).
        fail( iv_code = 'NOT_FOUND' iv_text = |Adobe form { iv_name } could not be loaded: { lx_fp->get_text( ) }| ix_prev = lx_fp ).
    ENDTRY.
  ENDMETHOD.


  METHOD sfp_read_form.
    " As abapGit: the layout of the original language is emptied for the
    " conversion and put back; it travels as a document of its own. Layouts
    " of other languages stay inside, in LAYOUTT.
    DATA lx_convert TYPE REF TO cx_fp_api.

    tadir_entry( EXPORTING iv_type = c_sfpf iv_name = iv_name IMPORTING ev_language = DATA(lv_master) ).
    DATA(lo_wb) = sfp_load_form( iv_name = iv_name iv_language = lv_master ).
    TRY.
        DATA(lo_form)   = CAST if_fp_form( lo_wb->get_object( ) ).
        DATA(lo_layout) = lo_form->get_layout( ).
        DATA(lv_layout) = lo_layout->get_layout_data( ).
        lo_layout->set_layout_data( i_layout_data = VALUE xstring( ) i_set_xliff_ids = abap_false ).
        TRY.
            rv_xml = cl_fp_helper=>convert_form_to_xstring( lo_form ).
          CATCH cx_fp_api INTO lx_convert ##NO_HANDLER.
        ENDTRY.
        lo_layout->set_layout_data( i_layout_data = lv_layout i_set_xliff_ids = abap_false ).
      CATCH cx_fp_api INTO DATA(lx_fp).
        sfp_free( lo_wb ).
        fail( iv_text = |Adobe form { iv_name }: { lx_fp->get_text( ) }| ix_prev = lx_fp ).
    ENDTRY.
    sfp_free( lo_wb ).
    IF lx_convert IS BOUND.
      fail( iv_text = |Adobe form { iv_name }: { lx_convert->get_text( ) }| ix_prev = lx_convert ).
    ENDIF.
  ENDMETHOD.


  METHOD sfp_read_layout.
    DATA(lt_languages) = sfp_layout_languages( iv_name ).
    IF NOT line_exists( lt_languages[ table_line = iv_language ] ).
      fail( iv_code = 'NOT_FOUND' iv_text = |Adobe form { iv_name } has no layout in { to_iso( iv_language ) }| ).
    ENDIF.
    DATA(lo_wb) = sfp_load_form( iv_name = iv_name iv_language = iv_language ).
    TRY.
        rv_xdp = CAST if_fp_form( lo_wb->get_object( ) )->get_layout( )->get_layout_data( ).
      CATCH cx_fp_api INTO DATA(lx_fp).
        sfp_free( lo_wb ).
        fail( iv_text = |Adobe form { iv_name }: { lx_fp->get_text( ) }| ix_prev = lx_fp ).
    ENDTRY.
    sfp_free( lo_wb ).
  ENDMETHOD.


  METHOD sfp_read_interface.
    DATA lo_wb TYPE REF TO if_fp_wb_interface.

    tadir_entry( EXPORTING iv_type = c_sfpi iv_name = iv_name IMPORTING ev_language = DATA(lv_master) ).
    TRY.
        lo_wb = cl_fp_wb_interface=>load( i_name                    = iv_name
                                          i_mode                    = if_fp_wb_object=>c_mode_read
                                          i_language                = lv_master
                                          i_suppress_language_check = abap_true ).
        rv_xml = cl_fp_helper=>convert_interface_to_xstring( CAST if_fp_interface( lo_wb->get_object( ) ) ).
      CATCH cx_fp_api INTO DATA(lx_fp).
        sfp_free( lo_wb ).
        fail( iv_text = |Adobe interface { iv_name }: { lx_fp->get_text( ) }| ix_prev = lx_fp ).
    ENDTRY.
    sfp_free( lo_wb ).
  ENDMETHOD.


  METHOD sfp_write_form.
    " The API has no setter for the context, so the form is replaced as
    " abapGit does it: DELETE and CREATE. CREATE leaves it inactive, and
    " until the client has activated it the form has no active version.
    " The old form comes back as BACKUP: when the activation fails, the
    " client writes the backup and activates that.
    DATA lv_name TYPE fpname.
    DATA lo_new  TYPE REF TO if_fp_form.
    DATA lo_wb   TYPE REF TO if_fp_wb_form.

    lv_name = is_request-name.
    tadir_entry( EXPORTING iv_type     = c_sfpf
                           iv_name     = lv_name
                 IMPORTING ev_exists   = DATA(lv_exists)
                           ev_package  = DATA(lv_package)
                           ev_language = DATA(lv_master) ).
    IF lv_exists = abap_false.
      fail( iv_code = 'NOT_FOUND' iv_text = |Adobe form { lv_name } does not exist; creating one is not supported| ).
    ENDIF.

    TRY.
        lo_new = cl_fp_helper=>convert_xstring_to_form( i_xstring = is_request-content i_language = lv_master ).
      CATCH cx_fp_api INTO DATA(lx_parse).
        fail( iv_code = 'INVALID_CONTENT' iv_text = |Adobe form content: { lx_parse->get_text( ) }| ix_prev = lx_parse ).
    ENDTRY.

    check_transport( iv_type = c_sfpf iv_name = lv_name iv_package = lv_package iv_transport = is_request-transport ).
    IF is_request-test_run = abap_true.
      RETURN.
    ENDIF.

    " The whole old form, layout included, as the backup. A content without
    " the original-language layout keeps the current one.
    DATA(lo_old_wb) = sfp_load_form( iv_name = lv_name iv_language = lv_master ).
    TRY.
        DATA(lo_old) = CAST if_fp_form( lo_old_wb->get_object( ) ).
        rs_result-backup = cl_fp_helper=>convert_form_to_xstring( lo_old ).
        IF lo_new->get_layout( )->get_layout_data( ) IS INITIAL.
          lo_new->get_layout( )->set_layout_data( lo_old->get_layout( )->get_layout_data( ) ).
        ENDIF.
      CATCH cx_fp_api INTO DATA(lx_backup).
        sfp_free( lo_old_wb ).
        fail( iv_text = |Adobe form { lv_name }: backup failed: { lx_backup->get_text( ) }| ix_prev = lx_backup ).
    ENDTRY.
    sfp_free( lo_old_wb ).

    register_in_transport( iv_type = c_sfpf iv_name = lv_name iv_package = lv_package
                           iv_transport = is_request-transport ).

    TRY.
        cl_fp_wb_form=>delete( i_name = lv_name i_ordernum = is_request-transport i_dark = abap_true ).
      CATCH cx_fp_api INTO DATA(lx_delete).
        fail( iv_code = 'SAVE_FAILED' iv_text = |Adobe form { lv_name }: delete failed: { lx_delete->get_text( ) }| ix_prev = lx_delete ).
    ENDTRY.

    TRY.
        lo_wb = cl_fp_wb_form=>create( i_name     = lv_name
                                       i_form     = lo_new
                                       i_devclass = lv_package
                                       i_ordernum = is_request-transport
                                       i_dark     = abap_true ).
        lo_wb->save( ).
        sfp_free( lo_wb ).
      CATCH cx_fp_api INTO DATA(lx_create).
        sfp_free( lo_wb ).
        " Put the old form back right away; the client never sees it gone.
        TRY.
            lo_wb = cl_fp_wb_form=>create( i_name     = lv_name
                                           i_form     = cl_fp_helper=>convert_xstring_to_form(
                                                          i_xstring  = rs_result-backup
                                                          i_language = lv_master )
                                           i_devclass = lv_package
                                           i_ordernum = is_request-transport
                                           i_dark     = abap_true ).
            lo_wb->save( ).
            sfp_free( lo_wb ).
            COMMIT WORK.
          CATCH cx_fp_api INTO DATA(lx_restore).
            fail( iv_code = 'SAVE_FAILED'
                  iv_text = |Adobe form { lv_name }: create failed ({ lx_create->get_text( ) }), and so did the restore: { lx_restore->get_text( ) }|
                  ix_prev = lx_restore ).
        ENDTRY.
        fail( iv_code = 'SAVE_FAILED'
              iv_text = |Adobe form { lv_name }: create failed, the old form was put back inactive: { lx_create->get_text( ) }|
              ix_prev = lx_create ).
    ENDTRY.

    COMMIT WORK.
    rs_result-activate = abap_true.
  ENDMETHOD.


  METHOD sfp_write_layout.
    " Original language: load for writing, set the layout, save - inactive.
    " A translation: the save of the form API writes the original language
    " only; translations go straight into the active version, as SE63 does
    " (FP_WRITE_XDP). The load for writing is then only for the lock and
    " the change check.
    DATA lv_name TYPE fpname.

    lv_name = is_request-name.
    tadir_entry( EXPORTING iv_type     = c_sfpf
                           iv_name     = lv_name
                 IMPORTING ev_exists   = DATA(lv_exists)
                           ev_package  = DATA(lv_package)
                           ev_language = DATA(lv_master) ).
    IF lv_exists = abap_false.
      fail( iv_code = 'NOT_FOUND' iv_text = |Adobe form { lv_name } does not exist| ).
    ENDIF.
    " Only existing language versions: a new translation should not come
    " about by accident.
    DATA(lt_languages) = sfp_layout_languages( lv_name ).
    IF NOT line_exists( lt_languages[ table_line = is_request-language ] ).
      fail( iv_code = 'NOT_FOUND' iv_text = |Adobe form { lv_name } has no layout in { to_iso( is_request-language ) }| ).
    ENDIF.

    check_transport( iv_type = c_sfpf iv_name = lv_name iv_package = lv_package iv_transport = is_request-transport ).
    IF is_request-test_run = abap_true.
      RETURN.
    ENDIF.

    register_in_transport( iv_type = c_sfpf iv_name = lv_name iv_package = lv_package
                           iv_transport = is_request-transport ).

    DATA(lo_wb) = sfp_load_form( iv_name      = lv_name
                                 iv_language  = lv_master
                                 iv_mode      = if_fp_wb_object=>c_mode_write
                                 iv_transport = is_request-transport ).

    IF is_request-language <> lv_master.
      CALL FUNCTION 'FP_WRITE_XDP'
        EXPORTING
          i_name       = lv_name
          i_language   = is_request-language
          i_xdp        = is_request-content
        EXCEPTIONS
          update_error = 1
          OTHERS       = 2.
      DATA(lv_subrc) = sy-subrc.
      DATA(lv_message) = message_text( ).
      sfp_free( lo_wb ).
      IF lv_subrc <> 0.
        fail( iv_code = 'SAVE_FAILED' iv_text = |Adobe form { lv_name } layout { to_iso( is_request-language ) }: { lv_message }| ).
      ENDIF.
      COMMIT WORK.
      RETURN.
    ENDIF.

    TRY.
        CAST if_fp_form( lo_wb->get_object( ) )->get_layout( )->set_layout_data( is_request-content ).
        lo_wb->save( ).
      CATCH cx_fp_api INTO DATA(lx_fp).
        sfp_free( lo_wb ).
        fail( iv_code = 'SAVE_FAILED' iv_text = |Adobe form { lv_name } layout: { lx_fp->get_text( ) }| ix_prev = lx_fp ).
    ENDTRY.
    sfp_free( lo_wb ).
    COMMIT WORK.
    rs_result-activate = abap_true.
  ENDMETHOD.


  METHOD sfp_write_interface.
    " CL_FP_WB_INTERFACE has no I_DARK, so no DELETE and CREATE: the
    " interface is loaded for writing and changed in place - initialization
    " (code, its input and output parameters, FORM routines) and global
    " definitions (types, data, field symbols). The form parameters
    " (IMPORTING/EXPORTING) are not taken over.
    DATA lv_name TYPE fpname.
    DATA lo_src  TYPE REF TO if_fp_interface.
    DATA lo_wb   TYPE REF TO if_fp_wb_interface.

    lv_name = is_request-name.
    tadir_entry( EXPORTING iv_type     = c_sfpi
                           iv_name     = lv_name
                 IMPORTING ev_exists   = DATA(lv_exists)
                           ev_package  = DATA(lv_package)
                           ev_language = DATA(lv_master) ).
    IF lv_exists = abap_false.
      fail( iv_code = 'NOT_FOUND' iv_text = |Adobe interface { lv_name } does not exist; creating one is not supported| ).
    ENDIF.

    TRY.
        lo_src = cl_fp_helper=>convert_xstring_to_interface( i_xstring = is_request-content i_language = lv_master ).
      CATCH cx_fp_api INTO DATA(lx_parse).
        fail( iv_code = 'INVALID_CONTENT' iv_text = |Adobe interface content: { lx_parse->get_text( ) }| ix_prev = lx_parse ).
    ENDTRY.

    check_transport( iv_type = c_sfpi iv_name = lv_name iv_package = lv_package iv_transport = is_request-transport ).
    IF is_request-test_run = abap_true.
      RETURN.
    ENDIF.

    register_in_transport( iv_type = c_sfpi iv_name = lv_name iv_package = lv_package
                           iv_transport = is_request-transport ).

    TRY.
        lo_wb = cl_fp_wb_interface=>load( i_name                    = lv_name
                                          i_mode                    = if_fp_wb_object=>c_mode_write
                                          i_language                = lv_master
                                          i_suppress_language_check = abap_true
                                          i_ordernum                = is_request-transport ).
        DATA(lo_tgt_data) = CAST if_fp_interface( lo_wb->get_object( ) )->get_interface_data( ).
        DATA(lo_src_data) = lo_src->get_interface_data( ).

        DATA(lo_tgt_code) = lo_tgt_data->get_coding( ).
        DATA(lo_src_code) = lo_src_data->get_coding( ).
        lo_tgt_code->set_init_input_params( lo_src_code->get_init_input_params( ) ).
        lo_tgt_code->set_init_output_params( lo_src_code->get_init_output_params( ) ).
        lo_tgt_code->set_init_coding( lo_src_code->get_init_coding( ) ).
        lo_tgt_code->set_forms_coding( lo_src_code->get_forms_coding( ) ).

        DATA(lo_tgt_glob) = lo_tgt_data->get_global_definitions( ).
        DATA(lo_src_glob) = lo_src_data->get_global_definitions( ).
        lo_tgt_glob->set_types( lo_src_glob->get_types( ) ).
        lo_tgt_glob->set_global_data( lo_src_glob->get_global_data( ) ).
        lo_tgt_glob->set_fieldsymbols( lo_src_glob->get_fieldsymbols( ) ).

        lo_wb->save( ).
      CATCH cx_fp_api INTO DATA(lx_fp).
        sfp_free( lo_wb ).
        fail( iv_code = 'SAVE_FAILED' iv_text = |Adobe interface { lv_name }: { lx_fp->get_text( ) }| ix_prev = lx_fp ).
    ENDTRY.
    sfp_free( lo_wb ).
    COMMIT WORK.
    rs_result-activate = abap_true.
  ENDMETHOD.


  METHOD sfp_layout_languages.
    SELECT language FROM fplayoutt
      WHERE name  = @iv_name
        AND state = 'A'
      ORDER BY language
      INTO TABLE @rt_languages.
  ENDMETHOD.


  METHOD sfp_free.
    IF io_wb IS NOT BOUND.
      RETURN.
    ENDIF.
    TRY.
        io_wb->free( ).
      CATCH cx_fp_api ##NO_HANDLER.
    ENDTRY.
  ENDMETHOD.



  METHOD tadir_entry.
    DATA(lv_obj_name) = CONV sobj_name( iv_name ).
    CLEAR: ev_exists, ev_package, ev_language.
    SELECT SINGLE devclass, masterlang FROM tadir
      WHERE pgmid    = 'R3TR'
        AND object   = @iv_type
        AND obj_name = @lv_obj_name
        AND delflag  = @abap_false
      INTO (@ev_package, @ev_language).
    ev_exists = xsdbool( sy-subrc = 0 ).
    IF ev_language IS INITIAL.
      ev_language = sy-langu.
    ENDIF.
  ENDMETHOD.


  METHOD tadir_insert.
    " The form APIs write the object but not its catalog entry; the
    " workbench does that in the dialog this service has to avoid.
    CALL FUNCTION 'TR_TADIR_INTERFACE'
      EXPORTING
        wi_test_modus       = space
        wi_tadir_pgmid      = 'R3TR'
        wi_tadir_object     = iv_type
        wi_tadir_obj_name   = CONV sobj_name( iv_name )
        wi_tadir_devclass   = iv_package
        wi_tadir_masterlang = iv_language
        wi_set_genflag      = space
      EXCEPTIONS
        OTHERS              = 1.
    IF sy-subrc <> 0.
      fail( iv_code = 'SAVE_FAILED' iv_text = |{ iv_type } { iv_name } in package { iv_package }: { message_text( ) }| ).
    ENDIF.
  ENDMETHOD.


  METHOD tadir_delete.
    CALL FUNCTION 'TR_TADIR_INTERFACE'
      EXPORTING
        wi_test_modus         = space
        wi_delete_tadir_entry = abap_true
        wi_tadir_pgmid        = 'R3TR'
        wi_tadir_object       = iv_type
        wi_tadir_obj_name     = CONV sobj_name( iv_name )
      EXCEPTIONS
        OTHERS                = 1 ##FM_SUBRC_OK.
  ENDMETHOD.


  METHOD register_in_transport.
    " The standard registration opens the request dialog (SAPLSTRD 0300),
    " a runtime error in this session. TRINT_OBJECTS_CHECK_AND_INSERT in API
    " mode checks and inserts without asking, and runs before the save so
    " that a refused request leaves the object as it was.
    IF iv_package IS INITIAL OR iv_package(1) = '$'.
      RETURN.
    ENDIF.
    check_transport( iv_type = iv_type iv_name = iv_name iv_package = iv_package iv_transport = iv_transport ).

    DATA lt_entries TYPE cts_obj_entries.
    DATA lt_tadir   TYPE STANDARD TABLE OF tadir.
    DATA lt_ko200   TYPE STANDARD TABLE OF ko200.
    DATA ls_api     TYPE sctsa_s_api_call.

    APPEND VALUE #( pgmid = 'R3TR' object = iv_type obj_name = iv_name ) TO lt_entries.
    ls_api-is_api    = abap_true.
    ls_api-is_insert = abap_true.
    ls_api-request   = iv_transport.

    CALL FUNCTION 'TRINT_OBJECTS_CHECK_AND_INSERT'
      EXPORTING
        iv_with_dialog         = 'D'
        iv_send_message        = space
        iv_no_show_option      = space
        iv_no_ps               = abap_true
        iv_append_to_order     = space
        iv_insert_into_sbcsets = space
        iv_old_call            = space
        is_api_call            = ls_api
        it_obj_entries         = lt_entries
      IMPORTING
        et_tadir               = lt_tadir
      CHANGING
        ct_ko200               = lt_ko200
      EXCEPTIONS
        show_only_user_after_error   = 1
        cancel_edit_user_after_error = 2
        OTHERS                       = 3.
    IF sy-subrc <> 0.
      fail( iv_code = 'TRANSPORT_FAILED'
            iv_text = |{ iv_type } { iv_name } could not be recorded in { iv_transport }: { message_text( ) }| ).
    ENDIF.
  ENDMETHOD.


  METHOD check_transport.
    " Part of every write, the test run included: a transportable package
    " needs a request.
    IF iv_package IS INITIAL OR iv_package(1) = '$'.
      RETURN.
    ENDIF.
    IF iv_transport IS INITIAL.
      fail( iv_code = 'TRANSPORT_REQUIRED'
            iv_text = |{ iv_type } { iv_name } is in package { iv_package }; a transport request is required| ).
    ENDIF.
  ENDMETHOD.


  METHOD check_writable.
    " Customer objects only: Z*, Y* and namespaces. SAP's own forms are
    " copied, not changed.
    DATA(lv_first) = substring( val = iv_name len = 1 ).
    IF lv_first <> 'Z' AND lv_first <> 'Y' AND lv_first <> '/'.
      fail( iv_code = 'NOT_CUSTOMER_OBJECT' iv_text = |{ iv_name } is not a customer object (Z*, Y* or a namespace)| ).
    ENDIF.
  ENDMETHOD.


  METHOD check_xml.
    TRY.
        DATA(lo_reader) = cl_sxml_string_reader=>create( iv_xml ).
        WHILE lo_reader->read_next_node( ) IS BOUND.
        ENDWHILE.
      CATCH cx_sxml_error INTO DATA(lx_xml).
        fail( iv_code = 'INVALID_CONTENT' iv_text = |The content is not well-formed XML: { lx_xml->get_text( ) }| ix_prev = lx_xml ).
    ENDTRY.
  ENDMETHOD.


  METHOD is_inactive.
    DATA(lv_obj_name) = CONV trobj_name( iv_name ).
    SELECT SINGLE @abap_true FROM dwinactiv
      WHERE object   = @iv_type
        AND obj_name = @lv_obj_name
      INTO @rv_result.
  ENDMETHOD.


  METHOD to_iso.
    CALL FUNCTION 'CONVERSION_EXIT_ISOLA_OUTPUT'
      EXPORTING
        input  = iv_language
      IMPORTING
        output = rv_iso.
  ENDMETHOD.


  METHOD to_spras.
    DATA lv_iso TYPE laiso.
    lv_iso = to_upper( iv_iso ).
    CALL FUNCTION 'CONVERSION_EXIT_ISOLA_INPUT'
      EXPORTING
        input            = lv_iso
      IMPORTING
        output           = rv_language
      EXCEPTIONS
        unknown_language = 1
        OTHERS           = 2.
    IF sy-subrc <> 0 OR rv_language IS INITIAL.
      fail( iv_code = 'INVALID_PARAM' iv_text = |Unknown language '{ iv_iso }' (ISO code expected, e.g. DE)| ).
    ENDIF.
  ENDMETHOD.


  METHOD languages_json.
    DATA lt_parts TYPE string_table.
    LOOP AT it_languages INTO DATA(lv_language).
      APPEND |"{ to_iso( lv_language ) }"| TO lt_parts.
    ENDLOOP.
    rv_json = zcl_vsp_utils=>json_arr( zcl_vsp_utils=>json_join( lt_parts ) ).
  ENDMETHOD.


  METHOD fail.
    RAISE EXCEPTION TYPE zcx_vsp_form
      EXPORTING
        iv_code  = iv_code
        iv_text  = iv_text
        previous = ix_prev.
  ENDMETHOD.


  METHOD message_text.
    IF sy-msgid IS INITIAL.
      rv_text = |subrc { sy-subrc }|.
      RETURN.
    ENDIF.
    MESSAGE ID sy-msgid TYPE 'S' NUMBER sy-msgno
      WITH sy-msgv1 sy-msgv2 sy-msgv3 sy-msgv4 INTO rv_text.
  ENDMETHOD.

ENDCLASS.
