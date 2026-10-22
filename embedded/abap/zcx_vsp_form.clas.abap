"! <p class="shorttext synchronized">VSP Form Service: error with a code for the client</p>
CLASS zcx_vsp_form DEFINITION
  PUBLIC
  INHERITING FROM cx_static_check
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    DATA mv_code TYPE string READ-ONLY.
    DATA mv_text TYPE string READ-ONLY.

    METHODS constructor
      IMPORTING iv_code   TYPE string DEFAULT 'FORM_ERROR'
                iv_text   TYPE string OPTIONAL
                previous  TYPE REF TO cx_root OPTIONAL.

    METHODS get_text REDEFINITION.

ENDCLASS.


CLASS zcx_vsp_form IMPLEMENTATION.

  METHOD constructor ##ADT_SUPPRESS_GENERATION.
    super->constructor( previous = previous ).
    mv_code = iv_code.
    mv_text = iv_text.
  ENDMETHOD.

  METHOD get_text.
    result = mv_text.
  ENDMETHOD.

ENDCLASS.
