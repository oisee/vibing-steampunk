CLASS zcl_jseval_arr DEFINITION PUBLIC.
  PUBLIC SECTION.
    DATA items TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
    METHODS push IMPORTING ir_val TYPE REF TO data.
    METHODS get_item
      IMPORTING iv_idx        TYPE i
      RETURNING VALUE(rr_val) TYPE REF TO data.
    METHODS length RETURNING VALUE(rv_len) TYPE i.
ENDCLASS.

CLASS zcl_jseval_arr IMPLEMENTATION.
  METHOD push.
    APPEND ir_val TO items.
  ENDMETHOD.

  METHOD get_item.
    DATA lv_idx TYPE i.
    lv_idx = iv_idx + 1.
    IF lv_idx >= 1 AND lv_idx <= lines( items ).
      READ TABLE items INDEX lv_idx INTO rr_val.
    ENDIF.
  ENDMETHOD.

  METHOD length.
    rv_len = lines( items ).
  ENDMETHOD.
ENDCLASS.
