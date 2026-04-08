CLASS zcl_jseval_obj DEFINITION PUBLIC.
  PUBLIC SECTION.
    TYPES:
      BEGIN OF ty_prop,
        key TYPE string,
        val TYPE REF TO data,
      END OF ty_prop,
      tt_props TYPE HASHED TABLE OF ty_prop WITH UNIQUE KEY key.

    DATA props TYPE tt_props.

    METHODS get
      IMPORTING iv_key         TYPE string
      RETURNING VALUE(rr_val)  TYPE REF TO data.
    METHODS set
      IMPORTING iv_key TYPE string
                ir_val TYPE REF TO data.
    METHODS has
      IMPORTING iv_key        TYPE string
      RETURNING VALUE(rv_yes) TYPE abap_bool.
    METHODS copy_from
      IMPORTING io_other TYPE REF TO zcl_jseval_obj.
ENDCLASS.

CLASS zcl_jseval_obj IMPLEMENTATION.
  METHOD get.
    READ TABLE props WITH TABLE KEY key = iv_key ASSIGNING FIELD-SYMBOL(<p>).
    IF sy-subrc = 0.
      rr_val = <p>-val.
    ENDIF.
  ENDMETHOD.

  METHOD set.
    DATA ls TYPE ty_prop.
    ls-key = iv_key.
    ls-val = ir_val.
    READ TABLE props WITH TABLE KEY key = iv_key ASSIGNING FIELD-SYMBOL(<p>).
    IF sy-subrc = 0.
      <p>-val = ir_val.
    ELSE.
      INSERT ls INTO TABLE props.
    ENDIF.
  ENDMETHOD.

  METHOD has.
    READ TABLE props WITH TABLE KEY key = iv_key TRANSPORTING NO FIELDS.
    rv_yes = boolc( sy-subrc = 0 ).
  ENDMETHOD.

  METHOD copy_from.
    LOOP AT io_other->props ASSIGNING FIELD-SYMBOL(<p>).
      set( iv_key = <p>-key ir_val = <p>-val ).
    ENDLOOP.
  ENDMETHOD.
ENDCLASS.
