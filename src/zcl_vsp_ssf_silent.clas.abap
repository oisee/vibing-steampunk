"! <p class="shorttext synchronized">VSP Form Service: Smart Form without dialogs</p>
"! CL_SSF_FB_SMART_FORM asks for a transport request in a dialog when it
"! locks and when it stores. Neither works in an APC (WebSocket) session:
"! a dialog there is a runtime error. This subclass locks without the
"! correction check and makes the store-time registration a no-op; the
"! form service registers the object in the request itself, before the
"! store, through TRINT_OBJECTS_CHECK_AND_INSERT in API mode.
CLASS zcl_vsp_ssf_silent DEFINITION
  PUBLIC
  INHERITING FROM cl_ssf_fb_smart_form
  CREATE PUBLIC.

  PUBLIC SECTION.
    "! Replaces ENQUEUE: RS_ACCESS_PERMISSION with the correction check
    "! suppressed, then the state STORE expects (enqueued, package, request).
    METHODS enqueue_silent
      IMPORTING im_formname   TYPE tdsfname
                im_korrnum    TYPE trkorr OPTIONAL
                im_masterlang TYPE sylangu DEFAULT sy-langu
                im_mode       TYPE string DEFAULT 'MODIFY'
      RAISING   cx_ssf_fb.

  PROTECTED SECTION.
    METHODS set_correction_request REDEFINITION.

ENDCLASS.


CLASS zcl_vsp_ssf_silent IMPLEMENTATION.

  METHOD enqueue_silent.
    DATA lv_devclass TYPE devclass.
    DATA lv_master   TYPE sy-langu.
    DATA lv_korrnum  TYPE trkorr.
    DATA lv_modlng   TYPE sy-langu.

    CLEAR me->enqueued.

    CALL FUNCTION 'RS_ACCESS_PERMISSION'
      EXPORTING
        authority_check                = 'X'
        global_lock                    = 'X'
        suppress_corr_check_altogether = 'X'
        suppress_language_check        = 'X'
        master_language                = im_masterlang
        mode                           = im_mode
        object                         = im_formname
        object_class                   = 'SSFO'
      IMPORTING
        devclass                       = lv_devclass
        new_master_language            = lv_master
        korrnum                        = lv_korrnum
        modification_language          = lv_modlng
      EXCEPTIONS
        canceled_in_corr               = 1
        enqueued_by_user               = 2
        enqueue_system_failure         = 3
        illegal_parameter_values       = 4
        no_modify_permission           = 5
        no_show_permission             = 6
        permission_failure             = 7
        request_language_denied        = 8
        OTHERS                         = 9.
    IF sy-subrc <> 0.
      RAISE EXCEPTION TYPE cx_ssf_fb
        EXPORTING textid = cx_ssf_fb=>canceled_in_corr
                  msgid  = sy-msgid msgty = sy-msgty
                  msgno  = sy-msgno msgv1 = sy-msgv1
                  msgv2  = sy-msgv2 msgv3 = sy-msgv3
                  msgv4  = sy-msgv4.
    ENDIF.

    me->header-formname   = im_formname.
    me->header-devclass   = lv_devclass.
    me->header-masterlang = COND #( WHEN lv_master IS NOT INITIAL THEN lv_master ELSE im_masterlang ).
    me->korrnum           = COND #( WHEN im_korrnum IS NOT INITIAL THEN im_korrnum ELSE lv_korrnum ).
    me->enqueued          = c_mode_edit.
  ENDMETHOD.

  METHOD set_correction_request.
    RETURN.
  ENDMETHOD.

ENDCLASS.
