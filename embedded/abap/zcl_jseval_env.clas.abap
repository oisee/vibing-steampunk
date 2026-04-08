CLASS zcl_jseval_env DEFINITION PUBLIC.
  PUBLIC SECTION.
    TYPES:
      BEGIN OF ty_var_entry,
        name TYPE string,
        val  TYPE zif_jseval=>ty_value,
      END OF ty_var_entry,
      tt_vars TYPE HASHED TABLE OF ty_var_entry WITH UNIQUE KEY name.

    DATA vars       TYPE tt_vars.
    DATA parent     TYPE REF TO zcl_jseval_env.
    DATA output     TYPE REF TO data.
    DATA returning  TYPE abap_bool.
    DATA ret_val    TYPE zif_jseval=>ty_value.
    DATA breaking   TYPE abap_bool.
    DATA continuing TYPE abap_bool.

    METHODS constructor
      IMPORTING io_parent TYPE REF TO zcl_jseval_env OPTIONAL.
    METHODS get
      IMPORTING iv_name       TYPE string
      RETURNING VALUE(rs_val) TYPE zif_jseval=>ty_value.
    METHODS set
      IMPORTING iv_name TYPE string
                is_val  TYPE zif_jseval=>ty_value.
    METHODS define
      IMPORTING iv_name TYPE string
                is_val  TYPE zif_jseval=>ty_value.
    METHODS append_output
      IMPORTING iv_text TYPE string.
    METHODS get_output
      RETURNING VALUE(rv_out) TYPE string.
ENDCLASS.

CLASS zcl_jseval_env IMPLEMENTATION.
  METHOD constructor.
    parent = io_parent.
    IF io_parent IS BOUND.
      output = io_parent->output.
    ELSE.
      DATA lv_str TYPE string.
      CREATE DATA output LIKE lv_str.
    ENDIF.
  ENDMETHOD.

  METHOD get.
    READ TABLE vars WITH TABLE KEY name = iv_name ASSIGNING FIELD-SYMBOL(<v>).
    IF sy-subrc = 0.
      rs_val = <v>-val.
      RETURN.
    ENDIF.
    IF parent IS BOUND.
      rs_val = parent->get( iv_name ).
      RETURN.
    ENDIF.
    rs_val-type = 0.
  ENDMETHOD.

  METHOD set.
    DATA lo_cur TYPE REF TO zcl_jseval_env.
    lo_cur = me.
    WHILE lo_cur IS BOUND.
      READ TABLE lo_cur->vars WITH TABLE KEY name = iv_name ASSIGNING FIELD-SYMBOL(<v>).
      IF sy-subrc = 0.
        <v>-val = is_val.
        RETURN.
      ENDIF.
      lo_cur = lo_cur->parent.
    ENDWHILE.
    DATA ls_entry TYPE ty_var_entry.
    ls_entry-name = iv_name.
    ls_entry-val  = is_val.
    INSERT ls_entry INTO TABLE vars.
  ENDMETHOD.

  METHOD define.
    DATA ls_entry TYPE ty_var_entry.
    ls_entry-name = iv_name.
    ls_entry-val  = is_val.
    READ TABLE vars WITH TABLE KEY name = iv_name ASSIGNING FIELD-SYMBOL(<v>).
    IF sy-subrc = 0.
      <v>-val = is_val.
    ELSE.
      INSERT ls_entry INTO TABLE vars.
    ENDIF.
  ENDMETHOD.

  METHOD append_output.
    FIELD-SYMBOLS <str> TYPE string.
    ASSIGN output->* TO <str>.
    <str> = <str> && iv_text.
  ENDMETHOD.

  METHOD get_output.
    FIELD-SYMBOLS <str> TYPE string.
    ASSIGN output->* TO <str>.
    rv_out = <str>.
  ENDMETHOD.
ENDCLASS.
