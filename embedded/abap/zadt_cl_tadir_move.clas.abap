CLASS zadt_cl_tadir_move DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    "! Move an ABAP object to a different package
    "! @parameter iv_pgmid | Program ID (default: R3TR)
    "! @parameter iv_object | Object type (e.g., CLAS, PROG, INTF)
    "! @parameter iv_obj_name | Object name
    "! @parameter iv_new_pkg | Target package name
    "! @parameter rv_msg | Result message (SUCCESS: or ERROR:)
    CLASS-METHODS move_object
      IMPORTING
        iv_pgmid      TYPE tadir-pgmid DEFAULT 'R3TR'
        iv_object     TYPE tadir-object
        iv_obj_name   TYPE tadir-obj_name
        iv_new_pkg    TYPE devclass
      RETURNING
        VALUE(rv_msg) TYPE string.

    "! Move object and commit the change
    "! Use this method when calling from external context
    CLASS-METHODS move_object_and_commit
      IMPORTING
        iv_pgmid      TYPE tadir-pgmid DEFAULT 'R3TR'
        iv_object     TYPE tadir-object
        iv_obj_name   TYPE tadir-obj_name
        iv_new_pkg    TYPE devclass
      RETURNING
        VALUE(rv_msg) TYPE string.

ENDCLASS.

CLASS zadt_cl_tadir_move IMPLEMENTATION.
  METHOD move_object.
    DATA: ls_tadir     TYPE tadir,
          ls_new_tadir TYPE tadir.

    " Read current TADIR entry
    SELECT SINGLE * FROM tadir INTO ls_tadir
      WHERE pgmid = iv_pgmid
        AND object = iv_object
        AND obj_name = iv_obj_name.

    IF sy-subrc <> 0.
      rv_msg = |ERROR: Object { iv_obj_name } not found in TADIR|.
      RETURN.
    ENDIF.

    " Check if already in target package
    IF ls_tadir-devclass = iv_new_pkg.
      rv_msg = |INFO: Object already in package { iv_new_pkg }|.
      RETURN.
    ENDIF.

    rv_msg = |Old: { ls_tadir-devclass } -> New: { iv_new_pkg }|.

    " Call TR_TADIR_INTERFACE to change package
    CALL FUNCTION 'TR_TADIR_INTERFACE'
      EXPORTING
        wi_test_modus       = space
        wi_tadir_pgmid      = iv_pgmid
        wi_tadir_object     = iv_object
        wi_tadir_obj_name   = iv_obj_name
        wi_tadir_devclass   = iv_new_pkg
        wi_tadir_author     = ls_tadir-author
        wi_tadir_masterlang = ls_tadir-masterlang
      IMPORTING
        new_tadir_entry     = ls_new_tadir
      EXCEPTIONS
        tadir_entry_not_existing       = 1
        tadir_entry_ill_type           = 2
        no_systemname                  = 3
        no_systemtype                  = 4
        original_system_conflict       = 5
        object_reserved_for_devclass   = 6
        object_exists_global           = 7
        object_exists_local            = 8
        object_is_distributed          = 9
        obj_specification_not_unique   = 10
        no_authorization_to_delete     = 11
        devclass_not_existing          = 12
        simultanious_set_remove_repair = 13
        order_missing                  = 14
        no_modification_of_head_syst   = 15
        pgmid_object_not_allowed       = 16
        masterlanguage_not_specified   = 17
        devclass_not_specified         = 18
        specify_owner_unique           = 19
        loc_priv_objs_no_repair        = 20
        gtadir_not_reached             = 21
        object_locked_for_order        = 22
        change_of_class_not_allowed    = 23
        no_change_from_sap_to_tmp      = 24
        OTHERS                         = 25.

    IF sy-subrc = 0.
      rv_msg = |SUCCESS: { rv_msg }|.
    ELSE.
      rv_msg = |ERROR sy-subrc={ sy-subrc } { sy-msgid }-{ sy-msgno }: { sy-msgv1 } { sy-msgv2 }|.
    ENDIF.
  ENDMETHOD.

  METHOD move_object_and_commit.
    rv_msg = move_object(
      iv_pgmid    = iv_pgmid
      iv_object   = iv_object
      iv_obj_name = iv_obj_name
      iv_new_pkg  = iv_new_pkg ).

    IF rv_msg CP 'SUCCESS*'.
      COMMIT WORK AND WAIT.
      rv_msg = rv_msg && ' [COMMITTED]'.
    ENDIF.
  ENDMETHOD.
ENDCLASS.
