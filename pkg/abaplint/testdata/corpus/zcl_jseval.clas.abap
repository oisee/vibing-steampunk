CLASS zcl_jseval DEFINITION PUBLIC.
  PUBLIC SECTION.
    CLASS-METHODS eval
      IMPORTING iv_source        TYPE string
      RETURNING VALUE(rv_output) TYPE string.

  PRIVATE SECTION.
    CLASS-METHODS tokenize
      IMPORTING iv_src           TYPE string
      RETURNING VALUE(rt_tokens) TYPE zif_jseval=>tt_tokens.

    CLASS-METHODS number_val
      IMPORTING iv_num        TYPE f
      RETURNING VALUE(rs_val) TYPE zif_jseval=>ty_value.
    CLASS-METHODS string_val
      IMPORTING iv_str        TYPE string
      RETURNING VALUE(rs_val) TYPE zif_jseval=>ty_value.
    CLASS-METHODS bool_val
      IMPORTING iv_bool       TYPE abap_bool
      RETURNING VALUE(rs_val) TYPE zif_jseval=>ty_value.
    CLASS-METHODS object_val
      RETURNING VALUE(rs_val) TYPE zif_jseval=>ty_value.
    CLASS-METHODS array_val
      IMPORTING it_elems      TYPE zif_jseval=>tt_nodes
      RETURNING VALUE(rs_val) TYPE zif_jseval=>ty_value.
    CLASS-METHODS undefined_val
      RETURNING VALUE(rs_val) TYPE zif_jseval=>ty_value.

    CLASS-METHODS is_true
      IMPORTING is_val        TYPE zif_jseval=>ty_value
      RETURNING VALUE(rv_yes) TYPE abap_bool.
    CLASS-METHODS to_number
      IMPORTING is_val        TYPE zif_jseval=>ty_value
      RETURNING VALUE(rv_num) TYPE f.
    CLASS-METHODS to_string
      IMPORTING is_val        TYPE zif_jseval=>ty_value
      RETURNING VALUE(rv_str) TYPE string.

    CLASS-METHODS eval_node
      IMPORTING ir_node       TYPE REF TO data
                io_env        TYPE REF TO zcl_jseval_env
      RETURNING VALUE(rs_val) TYPE zif_jseval=>ty_value.
    CLASS-METHODS eval_bin_op
      IMPORTING iv_op         TYPE string
                is_left       TYPE zif_jseval=>ty_value
                is_right      TYPE zif_jseval=>ty_value
      RETURNING VALUE(rs_val) TYPE zif_jseval=>ty_value.
    CLASS-METHODS eval_property_access
      IMPORTING is_obj        TYPE zif_jseval=>ty_value
                iv_prop       TYPE string
      RETURNING VALUE(rs_val) TYPE zif_jseval=>ty_value.
    CLASS-METHODS eval_method_call
      IMPORTING is_obj        TYPE zif_jseval=>ty_value
                iv_method     TYPE string
                it_args       TYPE zif_jseval=>tt_nodes
                io_env        TYPE REF TO zcl_jseval_env
                ir_obj_node   TYPE REF TO data
      RETURNING VALUE(rs_val) TYPE zif_jseval=>ty_value.
    CLASS-METHODS call_function
      IMPORTING is_fn         TYPE zif_jseval=>ty_function
                it_args       TYPE zif_jseval=>tt_nodes
                io_env        TYPE REF TO zcl_jseval_env
                ir_this       TYPE REF TO data OPTIONAL
      RETURNING VALUE(rs_val) TYPE zif_jseval=>ty_value.
    CLASS-METHODS box_value
      IMPORTING is_val        TYPE zif_jseval=>ty_value
      RETURNING VALUE(rr_ref) TYPE REF TO data.
    CLASS-METHODS unbox_value
      IMPORTING ir_ref        TYPE REF TO data
      RETURNING VALUE(rs_val) TYPE zif_jseval=>ty_value.
ENDCLASS.


CLASS zcl_jseval IMPLEMENTATION.

  METHOD eval.
    DATA lt_tokens TYPE zif_jseval=>tt_tokens.
    lt_tokens = tokenize( iv_source ).

    DATA lo_parser TYPE REF TO zcl_jseval_parser.
    CREATE OBJECT lo_parser
      EXPORTING it_tokens = lt_tokens.

    DATA lt_stmts TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
    lt_stmts = lo_parser->parse_program( ).

    DATA lo_env TYPE REF TO zcl_jseval_env.
    CREATE OBJECT lo_env.
    lo_env->define( iv_name = `console` is_val = number_val( 0 ) ).

    LOOP AT lt_stmts INTO DATA(lr_stmt).
      eval_node( ir_node = lr_stmt io_env = lo_env ).
    ENDLOOP.

    rv_output = lo_env->get_output( ).
  ENDMETHOD.

  METHOD tokenize.
    DATA lv_i   TYPE i VALUE 0.
    DATA lv_len TYPE i.
    DATA lv_ch  TYPE c LENGTH 1.
    DATA lv_j   TYPE i.
    DATA ls_tok TYPE zif_jseval=>ty_token.

    lv_len = strlen( iv_src ).

    WHILE lv_i < lv_len.
      lv_ch = iv_src+lv_i(1).

      " Skip whitespace
      IF lv_ch = ` ` OR lv_ch = cl_abap_char_utilities=>horizontal_tab
         OR lv_ch = cl_abap_char_utilities=>newline
         OR lv_ch = cl_abap_char_utilities=>cr_lf(1).
        lv_i = lv_i + 1.
        CONTINUE.
      ENDIF.

      " Skip // comments
      IF lv_i + 1 < lv_len.
        IF lv_ch = `/` AND iv_src+lv_i(2) = `//`.
          WHILE lv_i < lv_len.
            IF iv_src+lv_i(1) = cl_abap_char_utilities=>newline.
              EXIT.
            ENDIF.
            lv_i = lv_i + 1.
          ENDWHILE.
          CONTINUE.
        ENDIF.
      ENDIF.

      " Number
      IF lv_ch >= `0` AND lv_ch <= `9`.
        lv_j = lv_i.
        WHILE lv_j < lv_len.
          DATA lv_d TYPE c LENGTH 1.
          lv_d = iv_src+lv_j(1).
          IF ( lv_d >= `0` AND lv_d <= `9` ) OR lv_d = `.`.
            lv_j = lv_j + 1.
          ELSE.
            EXIT.
          ENDIF.
        ENDWHILE.
        DATA lv_numlen TYPE i.
        lv_numlen = lv_j - lv_i.
        CLEAR ls_tok.
        ls_tok-kind = 0.
        ls_tok-val  = iv_src+lv_i(lv_numlen).
        APPEND ls_tok TO rt_tokens.
        lv_i = lv_j.
        CONTINUE.
      ENDIF.

      " String
      IF lv_ch = `'` OR lv_ch = `"`.
        DATA lv_quote TYPE c LENGTH 1.
        lv_quote = lv_ch.
        lv_j = lv_i + 1.
        DATA lv_sbuf TYPE string.
        CLEAR lv_sbuf.
        WHILE lv_j < lv_len.
          DATA lv_sc TYPE c LENGTH 1.
          lv_sc = iv_src+lv_j(1).
          IF lv_sc = lv_quote.
            EXIT.
          ENDIF.
          IF lv_sc = `\` AND lv_j + 1 < lv_len.
            lv_j = lv_j + 1.
            DATA lv_esc TYPE c LENGTH 1.
            lv_esc = iv_src+lv_j(1).
            CASE lv_esc.
              WHEN `n`.
                lv_sbuf = lv_sbuf && cl_abap_char_utilities=>newline.
              WHEN `t`.
                lv_sbuf = lv_sbuf && cl_abap_char_utilities=>horizontal_tab.
              WHEN `\`.
                lv_sbuf = lv_sbuf && `\`.
              WHEN `'`.
                lv_sbuf = lv_sbuf && `'`.
              WHEN `"`.
                lv_sbuf = lv_sbuf && `"`.
              WHEN OTHERS.
                lv_sbuf = lv_sbuf && substring( val = iv_src off = lv_j len = 1 ).
            ENDCASE.
          ELSE.
            lv_sbuf = lv_sbuf && substring( val = iv_src off = lv_j len = 1 ).
          ENDIF.
          lv_j = lv_j + 1.
        ENDWHILE.
        CLEAR ls_tok.
        ls_tok-kind = 1.
        ls_tok-val  = lv_sbuf.
        APPEND ls_tok TO rt_tokens.
        lv_i = lv_j + 1.
        CONTINUE.
      ENDIF.

      " Identifier / keyword
      IF lv_ch = `_` OR ( lv_ch >= `a` AND lv_ch <= `z` )
                      OR ( lv_ch >= `A` AND lv_ch <= `Z` ).
        lv_j = lv_i.
        WHILE lv_j < lv_len.
          DATA lv_ic TYPE c LENGTH 1.
          lv_ic = iv_src+lv_j(1).
          IF lv_ic = `_` OR ( lv_ic >= `a` AND lv_ic <= `z` )
                         OR ( lv_ic >= `A` AND lv_ic <= `Z` )
                         OR ( lv_ic >= `0` AND lv_ic <= `9` ).
            lv_j = lv_j + 1.
          ELSE.
            EXIT.
          ENDIF.
        ENDWHILE.
        DATA lv_idlen TYPE i.
        lv_idlen = lv_j - lv_i.
        CLEAR ls_tok.
        ls_tok-kind = 2.
        ls_tok-val  = iv_src+lv_i(lv_idlen).
        APPEND ls_tok TO rt_tokens.
        lv_i = lv_j.
        CONTINUE.
      ENDIF.

      " Multi-char operators
      IF lv_i + 1 < lv_len.
        DATA lv_two TYPE string.
        lv_two = iv_src+lv_i(2).
        IF lv_two = `==` OR lv_two = `!=` OR lv_two = `<=`
           OR lv_two = `>=` OR lv_two = `&&` OR lv_two = `||`.
          IF lv_i + 2 < lv_len AND
             ( lv_two = `==` OR lv_two = `!=` ).
            DATA lv_three TYPE string.
            lv_three = iv_src+lv_i(3).
            IF lv_three = `===` OR lv_three = `!==`.
              CLEAR ls_tok.
              ls_tok-kind = 3.
              ls_tok-val  = lv_three.
              APPEND ls_tok TO rt_tokens.
              lv_i = lv_i + 3.
              CONTINUE.
            ENDIF.
          ENDIF.
          CLEAR ls_tok.
          ls_tok-kind = 3.
          ls_tok-val  = lv_two.
          APPEND ls_tok TO rt_tokens.
          lv_i = lv_i + 2.
          CONTINUE.
        ENDIF.
      ENDIF.

      " Single char op/punc
      IF lv_ch = `+` OR lv_ch = `-` OR lv_ch = `*` OR lv_ch = `/`
         OR lv_ch = `%` OR lv_ch = `=` OR lv_ch = `<` OR lv_ch = `>`
         OR lv_ch = `!` OR lv_ch = `(` OR lv_ch = `)` OR lv_ch = `,`
         OR lv_ch = `{` OR lv_ch = `}` OR lv_ch = `;` OR lv_ch = `:`
         OR lv_ch = `.` OR lv_ch = `[` OR lv_ch = `]`.
        CLEAR ls_tok.
        ls_tok-kind = 3.
        ls_tok-val  = lv_ch.
        APPEND ls_tok TO rt_tokens.
        lv_i = lv_i + 1.
        CONTINUE.
      ENDIF.

      lv_i = lv_i + 1.
    ENDWHILE.

    CLEAR ls_tok.
    ls_tok-kind = 5.
    ls_tok-val  = ``.
    APPEND ls_tok TO rt_tokens.
  ENDMETHOD.

  METHOD number_val.
    rs_val-type = 1.
    rs_val-num  = iv_num.
  ENDMETHOD.

  METHOD string_val.
    rs_val-type = 2.
    rs_val-str  = iv_str.
  ENDMETHOD.

  METHOD bool_val.
    rs_val-type = 3.
    IF iv_bool = abap_true.
      rs_val-num = 1.
    ELSE.
      rs_val-num = 0.
    ENDIF.
  ENDMETHOD.

  METHOD object_val.
    rs_val-type = 6.
    CREATE OBJECT rs_val-obj.
  ENDMETHOD.

  METHOD array_val.
    rs_val-type = 7.
    CREATE OBJECT rs_val-arr.
    FIELD-SYMBOLS <ref> TYPE REF TO data.
    LOOP AT it_elems ASSIGNING <ref>.
      rs_val-arr->push( <ref> ).
    ENDLOOP.
  ENDMETHOD.

  METHOD undefined_val.
    rs_val-type = 0.
  ENDMETHOD.

  METHOD is_true.
    CASE is_val-type.
      WHEN 0 OR 5.
        rv_yes = abap_false.
      WHEN 1.
        rv_yes = boolc( is_val-num <> 0 ).
      WHEN 2.
        rv_yes = boolc( is_val-str IS NOT INITIAL ).
      WHEN 3.
        rv_yes = boolc( is_val-num <> 0 ).
      WHEN OTHERS.
        rv_yes = abap_true.
    ENDCASE.
  ENDMETHOD.

  METHOD to_number.
    CASE is_val-type.
      WHEN 1.
        rv_num = is_val-num.
      WHEN 2.
        TRY.
            rv_num = is_val-str.
          CATCH cx_root.
            rv_num = 0.
        ENDTRY.
      WHEN 3.
        rv_num = is_val-num.
      WHEN OTHERS.
        rv_num = 0.
    ENDCASE.
  ENDMETHOD.

  METHOD to_string.
    CASE is_val-type.
      WHEN 0.
        rv_str = `undefined`.
      WHEN 1.
        DATA lv_int TYPE i.
        DATA lv_fcheck TYPE f.
        lv_int = is_val-num.
        lv_fcheck = lv_int.
        IF lv_fcheck = is_val-num.
          rv_str = |{ lv_int }|.
        ELSE.
          rv_str = |{ is_val-num }|.
          CONDENSE rv_str.
        ENDIF.
      WHEN 2.
        rv_str = is_val-str.
      WHEN 3.
        IF is_val-num <> 0.
          rv_str = `true`.
        ELSE.
          rv_str = `false`.
        ENDIF.
      WHEN 5.
        rv_str = `null`.
      WHEN 6.
        rv_str = `[object Object]`.
      WHEN 7.
        rv_str = |[array { is_val-arr->length( ) }]|.
      WHEN OTHERS.
        rv_str = `undefined`.
    ENDCASE.
  ENDMETHOD.

  METHOD box_value.
    CREATE DATA rr_ref TYPE zif_jseval=>ty_value.
    FIELD-SYMBOLS <val> TYPE zif_jseval=>ty_value.
    ASSIGN rr_ref->* TO <val>.
    <val> = is_val.
  ENDMETHOD.

  METHOD unbox_value.
    IF ir_ref IS NOT BOUND.
      rs_val-type = 0.
      RETURN.
    ENDIF.
    FIELD-SYMBOLS <val> TYPE zif_jseval=>ty_value.
    ASSIGN ir_ref->* TO <val>.
    rs_val = <val>.
  ENDMETHOD.

  METHOD call_function.
    DATA lo_parent TYPE REF TO zcl_jseval_env.
    IF is_fn-closure IS BOUND.
      lo_parent ?= is_fn-closure.
    ELSE.
      lo_parent = io_env.
    ENDIF.
    DATA lo_call_env TYPE REF TO zcl_jseval_env.
    CREATE OBJECT lo_call_env
      EXPORTING io_parent = lo_parent.
    lo_call_env->output = io_env->output.
    IF ir_this IS BOUND.
      FIELD-SYMBOLS <this> TYPE zif_jseval=>ty_value.
      ASSIGN ir_this->* TO <this>.
      lo_call_env->define( iv_name = `this` is_val = <this> ).
    ENDIF.
    DATA lv_idx TYPE i VALUE 0.
    LOOP AT is_fn-params INTO DATA(lv_param).
      DATA lr_arg TYPE REF TO data.
      READ TABLE it_args INDEX lv_idx + 1 INTO lr_arg.
      IF sy-subrc = 0.
        lo_call_env->define( iv_name = lv_param is_val = unbox_value( lr_arg ) ).
      ENDIF.
      lv_idx = lv_idx + 1.
    ENDLOOP.
    LOOP AT is_fn-body INTO DATA(lr_stmt).
      rs_val = eval_node( ir_node = lr_stmt io_env = lo_call_env ).
      IF lo_call_env->returning = abap_true.
        rs_val = lo_call_env->ret_val.
        EXIT.
      ENDIF.
    ENDLOOP.
    IF ir_this IS BOUND.
      ASSIGN ir_this->* TO <this>.
      IF <this>-type = 6.
        DATA(ls_updated) = lo_call_env->get( `this` ).
        IF ls_updated-type = 6.
          <this>-obj->copy_from( ls_updated-obj ).
        ENDIF.
      ENDIF.
    ENDIF.
  ENDMETHOD.

  METHOD eval_node.
    IF ir_node IS NOT BOUND.
      rs_val = undefined_val( ).
      RETURN.
    ENDIF.
    IF io_env->returning = abap_true OR io_env->breaking = abap_true.
      rs_val = undefined_val( ).
      RETURN.
    ENDIF.

    FIELD-SYMBOLS <n> TYPE zif_jseval=>ty_node.
    ASSIGN ir_node->* TO <n>.

    CASE <n>-kind.

      WHEN zif_jseval=>c_node_number.
        rs_val = number_val( <n>-num ).

      WHEN zif_jseval=>c_node_string.
        rs_val = string_val( <n>-str ).

      WHEN zif_jseval=>c_node_ident.
        rs_val = io_env->get( <n>-str ).

      WHEN zif_jseval=>c_node_binop.
        DATA(ls_bl) = eval_node( ir_node = <n>-left io_env = io_env ).
        DATA(ls_br) = eval_node( ir_node = <n>-right io_env = io_env ).
        rs_val = eval_bin_op( iv_op = <n>-op is_left = ls_bl is_right = ls_br ).

      WHEN zif_jseval=>c_node_unaryop.
        DATA(ls_uval) = eval_node( ir_node = <n>-left io_env = io_env ).
        CASE <n>-op.
          WHEN `-`.
            rs_val = number_val( - to_number( ls_uval ) ).
          WHEN `!`.
            rs_val = bool_val( boolc( is_true( ls_uval ) = abap_false ) ).
        ENDCASE.

      WHEN zif_jseval=>c_node_assign.
        DATA(ls_aval) = eval_node( ir_node = <n>-right io_env = io_env ).
        io_env->set( iv_name = <n>-str is_val = ls_aval ).
        rs_val = ls_aval.

      WHEN zif_jseval=>c_node_member_assign.
        DATA(ls_maobj) = eval_node( ir_node = <n>-object io_env = io_env ).
        DATA(ls_maval) = eval_node( ir_node = <n>-right io_env = io_env ).
        DATA lv_maprop TYPE string.
        lv_maprop = <n>-property.
        IF <n>-prop_expr IS BOUND.
          lv_maprop = to_string( eval_node( ir_node = <n>-prop_expr io_env = io_env ) ).
        ENDIF.
        IF ls_maobj-type = 6.
          ls_maobj-obj->set( iv_key = lv_maprop ir_val = box_value( ls_maval ) ).
        ENDIF.
        rs_val = ls_maval.

      WHEN zif_jseval=>c_node_var.
        DATA ls_vval TYPE zif_jseval=>ty_value.
        ls_vval = undefined_val( ).
        IF <n>-right IS BOUND.
          ls_vval = eval_node( ir_node = <n>-right io_env = io_env ).
        ENDIF.
        io_env->define( iv_name = <n>-str is_val = ls_vval ).
        rs_val = ls_vval.

      WHEN zif_jseval=>c_node_if.
        DATA(ls_icond) = eval_node( ir_node = <n>-cond io_env = io_env ).
        IF is_true( ls_icond ) = abap_true.
          LOOP AT <n>-body INTO DATA(lr_ib).
            eval_node( ir_node = lr_ib io_env = io_env ).
            IF io_env->returning = abap_true OR io_env->breaking = abap_true.
              EXIT.
            ENDIF.
          ENDLOOP.
        ELSEIF lines( <n>-els ) > 0.
          LOOP AT <n>-els INTO DATA(lr_ie).
            eval_node( ir_node = lr_ie io_env = io_env ).
            IF io_env->returning = abap_true OR io_env->breaking = abap_true.
              EXIT.
            ENDIF.
          ENDLOOP.
        ENDIF.

      WHEN zif_jseval=>c_node_while.
        DO.
          DATA(ls_wcond) = eval_node( ir_node = <n>-cond io_env = io_env ).
          IF is_true( ls_wcond ) = abap_false OR io_env->returning = abap_true
             OR io_env->breaking = abap_true.
            EXIT.
          ENDIF.
          LOOP AT <n>-body INTO DATA(lr_wb).
            eval_node( ir_node = lr_wb io_env = io_env ).
            IF io_env->returning = abap_true OR io_env->breaking = abap_true
               OR io_env->continuing = abap_true.
              EXIT.
            ENDIF.
          ENDLOOP.
          IF io_env->continuing = abap_true.
            io_env->continuing = abap_false.
            CONTINUE.
          ENDIF.
        ENDDO.
        IF io_env->breaking = abap_true.
          io_env->breaking = abap_false.
        ENDIF.

      WHEN zif_jseval=>c_node_for.
        DATA lo_for_env TYPE REF TO zcl_jseval_env.
        CREATE OBJECT lo_for_env
          EXPORTING io_parent = io_env.
        lo_for_env->output = io_env->output.
        IF <n>-init IS BOUND.
          eval_node( ir_node = <n>-init io_env = lo_for_env ).
        ENDIF.
        DO.
          IF lo_for_env->returning = abap_true OR lo_for_env->breaking = abap_true.
            EXIT.
          ENDIF.
          DATA(ls_fcond) = eval_node( ir_node = <n>-cond io_env = lo_for_env ).
          IF is_true( ls_fcond ) = abap_false.
            EXIT.
          ENDIF.
          LOOP AT <n>-body INTO DATA(lr_fb).
            eval_node( ir_node = lr_fb io_env = lo_for_env ).
            IF lo_for_env->returning = abap_true OR lo_for_env->breaking = abap_true
               OR lo_for_env->continuing = abap_true.
              EXIT.
            ENDIF.
          ENDLOOP.
          IF lo_for_env->continuing = abap_true.
            lo_for_env->continuing = abap_false.
          ENDIF.
          IF lo_for_env->returning = abap_true OR lo_for_env->breaking = abap_true.
            EXIT.
          ENDIF.
          IF <n>-update IS BOUND.
            eval_node( ir_node = <n>-update io_env = lo_for_env ).
          ENDIF.
        ENDDO.
        IF lo_for_env->returning = abap_true.
          io_env->returning = abap_true.
          io_env->ret_val   = lo_for_env->ret_val.
        ENDIF.

      WHEN zif_jseval=>c_node_block.
        LOOP AT <n>-body INTO DATA(lr_bs).
          rs_val = eval_node( ir_node = lr_bs io_env = io_env ).
          IF io_env->returning = abap_true OR io_env->breaking = abap_true.
            EXIT.
          ENDIF.
        ENDLOOP.

      WHEN zif_jseval=>c_node_call.
        IF <n>-str = `console.log`.
          DATA lv_parts TYPE string.
          CLEAR lv_parts.
          DATA lv_first TYPE abap_bool VALUE abap_true.
          LOOP AT <n>-args INTO DATA(lr_ca).
            DATA(ls_cav) = eval_node( ir_node = lr_ca io_env = io_env ).
            IF lv_first = abap_false.
              lv_parts = lv_parts && ` `.
            ENDIF.
            lv_parts = lv_parts && to_string( ls_cav ).
            lv_first = abap_false.
          ENDLOOP.
          io_env->append_output( lv_parts && cl_abap_char_utilities=>newline ).
          rs_val = undefined_val( ).
          RETURN.
        ENDIF.
        DATA(ls_fn) = io_env->get( <n>-str ).
        IF ls_fn-type = 4 AND ls_fn-fn IS BOUND.
          FIELD-SYMBOLS <fn> TYPE zif_jseval=>ty_function.
          ASSIGN ls_fn-fn->* TO <fn>.
          DATA lt_call_args TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
          CLEAR lt_call_args.
          LOOP AT <n>-args INTO DATA(lr_fa).
            APPEND box_value( eval_node( ir_node = lr_fa io_env = io_env ) ) TO lt_call_args.
          ENDLOOP.
          rs_val = call_function( is_fn = <fn> it_args = lt_call_args io_env = io_env ).
        ENDIF.

      WHEN zif_jseval=>c_node_method_call.
        DATA(ls_mcobj) = eval_node( ir_node = <n>-object io_env = io_env ).
        DATA lv_method TYPE string.
        lv_method = <n>-property.
        DATA lt_mc_args TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
        CLEAR lt_mc_args.
        LOOP AT <n>-args INTO DATA(lr_ma2).
          APPEND box_value( eval_node( ir_node = lr_ma2 io_env = io_env ) ) TO lt_mc_args.
        ENDLOOP.
        rs_val = eval_method_call(
          is_obj      = ls_mcobj
          iv_method   = lv_method
          it_args     = lt_mc_args
          io_env      = io_env
          ir_obj_node = <n>-object ).

      WHEN zif_jseval=>c_node_func_decl.
        DATA lr_fn_data TYPE REF TO data.
        CREATE DATA lr_fn_data TYPE zif_jseval=>ty_function.
        FIELD-SYMBOLS <fn_data> TYPE zif_jseval=>ty_function.
        ASSIGN lr_fn_data->* TO <fn_data>.
        <fn_data>-name    = <n>-str.
        <fn_data>-params  = <n>-params.
        <fn_data>-body    = <n>-body.
        <fn_data>-closure = io_env.
        DATA ls_fnval TYPE zif_jseval=>ty_value.
        ls_fnval-type = 4.
        ls_fnval-fn   = lr_fn_data.
        io_env->define( iv_name = <n>-str is_val = ls_fnval ).

      WHEN zif_jseval=>c_node_return.
        DATA(ls_retv) = eval_node( ir_node = <n>-left io_env = io_env ).
        io_env->returning = abap_true.
        io_env->ret_val   = ls_retv.
        rs_val = ls_retv.

      WHEN zif_jseval=>c_node_break.
        io_env->breaking = abap_true.
        rs_val = undefined_val( ).

      WHEN zif_jseval=>c_node_continue.
        io_env->continuing = abap_true.
        rs_val = undefined_val( ).

      WHEN zif_jseval=>c_node_object.
        DATA ls_obj TYPE zif_jseval=>ty_value.
        ls_obj = object_val( ).
        DATA lv_oi TYPE i VALUE 1.
        WHILE lv_oi <= lines( <n>-args ).
          DATA lr_okey TYPE REF TO data.
          READ TABLE <n>-args INDEX lv_oi INTO lr_okey.
          FIELD-SYMBOLS <okey> TYPE zif_jseval=>ty_node.
          ASSIGN lr_okey->* TO <okey>.
          DATA(lv_okey_str) = <okey>-str.
          DATA lr_oval TYPE REF TO data.
          READ TABLE <n>-args INDEX lv_oi + 1 INTO lr_oval.
          DATA(ls_oval) = eval_node( ir_node = lr_oval io_env = io_env ).
          ls_obj-obj->set( iv_key = lv_okey_str ir_val = box_value( ls_oval ) ).
          lv_oi = lv_oi + 2.
        ENDWHILE.
        rs_val = ls_obj.

      WHEN zif_jseval=>c_node_array.
        DATA lt_arr_refs TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
        CLEAR lt_arr_refs.
        LOOP AT <n>-args INTO DATA(lr_ae).
          APPEND box_value( eval_node( ir_node = lr_ae io_env = io_env ) ) TO lt_arr_refs.
        ENDLOOP.
        rs_val = array_val( lt_arr_refs ).

      WHEN zif_jseval=>c_node_member_access.
        DATA(ls_paobj) = eval_node( ir_node = <n>-object io_env = io_env ).
        DATA lv_paprop TYPE string.
        lv_paprop = <n>-property.
        IF <n>-prop_expr IS BOUND.
          lv_paprop = to_string( eval_node( ir_node = <n>-prop_expr io_env = io_env ) ).
        ENDIF.
        rs_val = eval_property_access( is_obj = ls_paobj iv_prop = lv_paprop ).

      WHEN zif_jseval=>c_node_typeof.
        DATA(ls_toval) = eval_node( ir_node = <n>-left io_env = io_env ).
        CASE ls_toval-type.
          WHEN 0.
            rs_val = string_val( `undefined` ).
          WHEN 1.
            rs_val = string_val( `number` ).
          WHEN 2.
            rs_val = string_val( `string` ).
          WHEN 3.
            rs_val = string_val( `boolean` ).
          WHEN 4.
            rs_val = string_val( `function` ).
          WHEN OTHERS.
            rs_val = string_val( `object` ).
        ENDCASE.

      WHEN zif_jseval=>c_node_new.
        DATA(ls_cls) = io_env->get( <n>-str ).
        IF ls_cls-type = 6 AND ls_cls-obj IS BOUND.
          DATA ls_instance TYPE zif_jseval=>ty_value.
          ls_instance = object_val( ).
          DATA lt_new_args TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
          CLEAR lt_new_args.
          LOOP AT <n>-args INTO DATA(lr_na).
            APPEND box_value( eval_node( ir_node = lr_na io_env = io_env ) ) TO lt_new_args.
          ENDLOOP.
          DATA lr_ctor TYPE REF TO data.
          lr_ctor = ls_cls-obj->get( `constructor` ).
          IF lr_ctor IS BOUND.
            DATA(ls_ctor_val) = unbox_value( lr_ctor ).
            IF ls_ctor_val-type = 4 AND ls_ctor_val-fn IS BOUND.
              FIELD-SYMBOLS <ctor_fn> TYPE zif_jseval=>ty_function.
              ASSIGN ls_ctor_val-fn->* TO <ctor_fn>.
              DATA lr_inst_ref TYPE REF TO data.
              lr_inst_ref = box_value( ls_instance ).
              call_function( is_fn = <ctor_fn> it_args = lt_new_args io_env = io_env
                             ir_this = lr_inst_ref ).
              ls_instance = unbox_value( lr_inst_ref ).
            ENDIF.
          ENDIF.
          LOOP AT ls_cls-obj->props ASSIGNING FIELD-SYMBOL(<cp>).
            IF <cp>-key <> `constructor`.
              ls_instance-obj->set( iv_key = <cp>-key ir_val = <cp>-val ).
            ENDIF.
          ENDLOOP.
          rs_val = ls_instance.
        ENDIF.

      WHEN zif_jseval=>c_node_class.
        DATA ls_clsobj TYPE zif_jseval=>ty_value.
        ls_clsobj = object_val( ).
        LOOP AT <n>-methods INTO DATA(ls_cm).
          DATA lr_mfn TYPE REF TO data.
          CREATE DATA lr_mfn TYPE zif_jseval=>ty_function.
          FIELD-SYMBOLS <mfn> TYPE zif_jseval=>ty_function.
          ASSIGN lr_mfn->* TO <mfn>.
          <mfn>-name    = ls_cm-name.
          <mfn>-params  = ls_cm-params.
          <mfn>-body    = ls_cm-body.
          <mfn>-closure = io_env.
          DATA ls_mfnval TYPE zif_jseval=>ty_value.
          ls_mfnval-type = 4.
          ls_mfnval-fn   = lr_mfn.
          ls_clsobj-obj->set( iv_key = ls_cm-name ir_val = box_value( ls_mfnval ) ).
        ENDLOOP.
        io_env->define( iv_name = <n>-str is_val = ls_clsobj ).

      WHEN zif_jseval=>c_node_switch.
        DATA(ls_swval) = eval_node( ir_node = <n>-cond io_env = io_env ).
        DATA lv_matched TYPE abap_bool VALUE abap_false.
        LOOP AT <n>-cases INTO DATA(ls_sc).
          IF lv_matched = abap_false AND ls_sc-expr IS BOUND.
            DATA(ls_caseval) = eval_node( ir_node = ls_sc-expr io_env = io_env ).
            IF to_number( ls_swval ) = to_number( ls_caseval )
               AND ls_swval-type = ls_caseval-type.
              lv_matched = abap_true.
            ENDIF.
          ENDIF.
          IF lv_matched = abap_false AND ls_sc-expr IS NOT BOUND.
            lv_matched = abap_true.
          ENDIF.
          IF lv_matched = abap_true.
            LOOP AT ls_sc-body INTO DATA(lr_sb).
              eval_node( ir_node = lr_sb io_env = io_env ).
              IF io_env->breaking = abap_true OR io_env->returning = abap_true.
                EXIT.
              ENDIF.
            ENDLOOP.
            IF io_env->breaking = abap_true.
              io_env->breaking = abap_false.
              EXIT.
            ENDIF.
            IF io_env->returning = abap_true.
              EXIT.
            ENDIF.
          ENDIF.
        ENDLOOP.

      WHEN OTHERS.
        rs_val = undefined_val( ).
    ENDCASE.
  ENDMETHOD.

  METHOD eval_property_access.
    CASE is_obj-type.
      WHEN 6.
        DATA lr_pv TYPE REF TO data.
        lr_pv = is_obj-obj->get( iv_prop ).
        IF lr_pv IS BOUND.
          rs_val = unbox_value( lr_pv ).
        ELSE.
          rs_val = undefined_val( ).
        ENDIF.
      WHEN 7.
        IF iv_prop = `length`.
          rs_val = number_val( CONV f( is_obj-arr->length( ) ) ).
        ELSE.
          TRY.
              DATA lv_aidx TYPE i.
              lv_aidx = iv_prop.
              DATA lr_aelem TYPE REF TO data.
              lr_aelem = is_obj-arr->get_item( lv_aidx ).
              IF lr_aelem IS BOUND.
                rs_val = unbox_value( lr_aelem ).
              ELSE.
                rs_val = undefined_val( ).
              ENDIF.
            CATCH cx_root.
              rs_val = undefined_val( ).
          ENDTRY.
        ENDIF.
      WHEN 2.
        IF iv_prop = `length`.
          rs_val = number_val( CONV f( strlen( is_obj-str ) ) ).
        ELSE.
          rs_val = undefined_val( ).
        ENDIF.
      WHEN OTHERS.
        rs_val = undefined_val( ).
    ENDCASE.
  ENDMETHOD.

  METHOD eval_method_call.
    CASE is_obj-type.
      WHEN 6.
        DATA lr_meth TYPE REF TO data.
        lr_meth = is_obj-obj->get( iv_method ).
        IF lr_meth IS BOUND.
          DATA(ls_mval) = unbox_value( lr_meth ).
          IF ls_mval-type = 4 AND ls_mval-fn IS BOUND.
            FIELD-SYMBOLS <fn> TYPE zif_jseval=>ty_function.
            ASSIGN ls_mval-fn->* TO <fn>.
            DATA lr_this TYPE REF TO data.
            lr_this = box_value( is_obj ).
            rs_val = call_function( is_fn = <fn> it_args = it_args io_env = io_env
                                    ir_this = lr_this ).
            IF ir_obj_node IS BOUND.
              FIELD-SYMBOLS <on> TYPE zif_jseval=>ty_node.
              ASSIGN ir_obj_node->* TO <on>.
              IF <on>-kind = zif_jseval=>c_node_ident.
                DATA(ls_updated) = unbox_value( lr_this ).
                io_env->set( iv_name = <on>-str is_val = ls_updated ).
              ENDIF.
            ENDIF.
          ENDIF.
        ENDIF.
      WHEN 7.
        CASE iv_method.
          WHEN `push`.
            IF lines( it_args ) > 0.
              DATA lr_push_arg TYPE REF TO data.
              READ TABLE it_args INDEX 1 INTO lr_push_arg.
              is_obj-arr->push( box_value( unbox_value( lr_push_arg ) ) ).
              rs_val = number_val( CONV f( is_obj-arr->length( ) ) ).
            ENDIF.
        ENDCASE.
      WHEN 2.
        CASE iv_method.
          WHEN `charAt`.
            IF lines( it_args ) > 0.
              DATA lr_cha TYPE REF TO data.
              READ TABLE it_args INDEX 1 INTO lr_cha.
              DATA lv_cidx TYPE i.
              lv_cidx = to_number( unbox_value( lr_cha ) ).
              IF lv_cidx >= 0 AND lv_cidx < strlen( is_obj-str ).
                DATA lv_cha_tmp TYPE string.
                lv_cha_tmp = substring( val = is_obj-str off = lv_cidx len = 1 ).
                rs_val = string_val( lv_cha_tmp ).
              ELSE.
                rs_val = string_val( `` ).
              ENDIF.
            ELSE.
              rs_val = string_val( `` ).
            ENDIF.
          WHEN `indexOf`.
            IF lines( it_args ) > 0.
              DATA lr_ioa TYPE REF TO data.
              READ TABLE it_args INDEX 1 INTO lr_ioa.
              DATA(lv_search) = to_string( unbox_value( lr_ioa ) ).
              FIND lv_search IN is_obj-str MATCH OFFSET DATA(lv_offset).
              IF sy-subrc = 0.
                rs_val = number_val( CONV f( lv_offset ) ).
              ELSE.
                rs_val = number_val( -1 ).
              ENDIF.
            ELSE.
              rs_val = number_val( -1 ).
            ENDIF.
          WHEN `substring`.
            IF lines( it_args ) >= 2.
              DATA lr_ss1 TYPE REF TO data.
              DATA lr_ss2 TYPE REF TO data.
              READ TABLE it_args INDEX 1 INTO lr_ss1.
              READ TABLE it_args INDEX 2 INTO lr_ss2.
              DATA lv_start TYPE i.
              DATA lv_end   TYPE i.
              lv_start = to_number( unbox_value( lr_ss1 ) ).
              lv_end   = to_number( unbox_value( lr_ss2 ) ).
              DATA lv_slen TYPE i.
              lv_slen = strlen( is_obj-str ).
              IF lv_start < 0. lv_start = 0. ENDIF.
              IF lv_start > lv_slen. lv_start = lv_slen. ENDIF.
              IF lv_end < 0. lv_end = 0. ENDIF.
              IF lv_end > lv_slen. lv_end = lv_slen. ENDIF.
              IF lv_start > lv_end.
                DATA lv_tmp TYPE i.
                lv_tmp = lv_start.
                lv_start = lv_end.
                lv_end = lv_tmp.
              ENDIF.
              DATA lv_sublen TYPE i.
              lv_sublen = lv_end - lv_start.
              IF lv_sublen > 0.
                DATA lv_sub_tmp TYPE string.
                lv_sub_tmp = substring( val = is_obj-str off = lv_start len = lv_sublen ).
                rs_val = string_val( lv_sub_tmp ).
              ELSE.
                rs_val = string_val( `` ).
              ENDIF.
            ENDIF.
          WHEN `charCodeAt`.
            IF lines( it_args ) > 0.
              DATA lr_cca TYPE REF TO data.
              READ TABLE it_args INDEX 1 INTO lr_cca.
              DATA lv_ccidx TYPE i.
              lv_ccidx = to_number( unbox_value( lr_cca ) ).
              IF lv_ccidx >= 0 AND lv_ccidx < strlen( is_obj-str ).
                DATA lv_char TYPE c LENGTH 1.
                DATA lv_cc_tmp TYPE string.
                lv_cc_tmp = substring( val = is_obj-str off = lv_ccidx len = 1 ).
                lv_char = lv_cc_tmp.
                DATA lv_hex TYPE x LENGTH 2.
                lv_hex = cl_abap_conv_out_ce=>uccp( lv_char ).
                DATA lv_code TYPE i.
                lv_code = lv_hex.
                rs_val = number_val( CONV f( lv_code ) ).
              ELSE.
                rs_val = number_val( 0 ).
              ENDIF.
            ELSE.
              rs_val = number_val( 0 ).
            ENDIF.
        ENDCASE.
    ENDCASE.
  ENDMETHOD.

  METHOD eval_bin_op.
    IF iv_op = `+` AND ( is_left-type = 2 OR is_right-type = 2 ).
      rs_val = string_val( to_string( is_left ) && to_string( is_right ) ).
      RETURN.
    ENDIF.
    CASE iv_op.
      WHEN `==` OR `===`.
        IF is_left-type <> is_right-type.
          rs_val = bool_val( abap_false ).
          RETURN.
        ENDIF.
        IF is_left-type = 2.
          rs_val = bool_val( boolc( is_left-str = is_right-str ) ).
          RETURN.
        ENDIF.
        rs_val = bool_val( boolc( to_number( is_left ) = to_number( is_right ) ) ).
        RETURN.
      WHEN `!=` OR `!==`.
        IF is_left-type <> is_right-type.
          rs_val = bool_val( abap_true ).
          RETURN.
        ENDIF.
        IF is_left-type = 2.
          rs_val = bool_val( boolc( is_left-str <> is_right-str ) ).
          RETURN.
        ENDIF.
        rs_val = bool_val( boolc( to_number( is_left ) <> to_number( is_right ) ) ).
        RETURN.
    ENDCASE.
    DATA lv_a TYPE f.
    DATA lv_b TYPE f.
    lv_a = to_number( is_left ).
    lv_b = to_number( is_right ).
    CASE iv_op.
      WHEN `+`.
        rs_val = number_val( lv_a + lv_b ).
      WHEN `-`.
        rs_val = number_val( lv_a - lv_b ).
      WHEN `*`.
        rs_val = number_val( lv_a * lv_b ).
      WHEN `/`.
        IF lv_b <> 0.
          rs_val = number_val( lv_a / lv_b ).
        ELSE.
          rs_val = number_val( 0 ).
        ENDIF.
      WHEN `%`.
        IF lv_b <> 0.
          DATA lv_ia TYPE i.
          DATA lv_ib TYPE i.
          lv_ia = lv_a.
          lv_ib = lv_b.
          rs_val = number_val( CONV f( lv_ia MOD lv_ib ) ).
        ELSE.
          rs_val = number_val( 0 ).
        ENDIF.
      WHEN `<`.
        rs_val = bool_val( boolc( lv_a < lv_b ) ).
      WHEN `>`.
        rs_val = bool_val( boolc( lv_a > lv_b ) ).
      WHEN `<=`.
        rs_val = bool_val( boolc( lv_a <= lv_b ) ).
      WHEN `>=`.
        rs_val = bool_val( boolc( lv_a >= lv_b ) ).
      WHEN `&&`.
        rs_val = bool_val( boolc( is_true( is_left ) = abap_true
                                  AND is_true( is_right ) = abap_true ) ).
      WHEN `||`.
        rs_val = bool_val( boolc( is_true( is_left ) = abap_true
                                  OR is_true( is_right ) = abap_true ) ).
      WHEN OTHERS.
        rs_val = undefined_val( ).
    ENDCASE.
  ENDMETHOD.

ENDCLASS.
