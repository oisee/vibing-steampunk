CLASS zcl_jseval_parser DEFINITION PUBLIC.
  PUBLIC SECTION.
    DATA tokens TYPE zif_jseval=>tt_tokens.
    DATA pos    TYPE i.

    METHODS constructor
      IMPORTING it_tokens TYPE zif_jseval=>tt_tokens.
    METHODS peek
      RETURNING VALUE(rs_tok) TYPE zif_jseval=>ty_token.
    METHODS next
      RETURNING VALUE(rs_tok) TYPE zif_jseval=>ty_token.
    METHODS expect
      IMPORTING iv_val TYPE string.
    METHODS parse_program
      RETURNING VALUE(rt_stmts) TYPE zif_jseval=>tt_nodes.
    METHODS parse_statement
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_var
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_if
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_while
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_for
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_switch
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_class
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_func
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_return
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_block
      RETURNING VALUE(rt_stmts) TYPE zif_jseval=>tt_nodes.
    METHODS parse_body
      RETURNING VALUE(rt_stmts) TYPE zif_jseval=>tt_nodes.
    METHODS parse_expr
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_assign
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_or
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_and
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_equality
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_comparison
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_add_sub
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_mul_div
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_unary
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_postfix
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_primary
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_array_literal
      RETURNING VALUE(rr_node) TYPE REF TO data.
    METHODS parse_object_literal
      RETURNING VALUE(rr_node) TYPE REF TO data.
ENDCLASS.

CLASS zcl_jseval_parser IMPLEMENTATION.
  METHOD constructor.
    tokens = it_tokens.
    pos = 0.
  ENDMETHOD.

  METHOD peek.
    IF pos >= lines( tokens ).
      rs_tok-kind = 5.
      rs_tok-val = ``.
    ELSE.
      READ TABLE tokens INDEX pos + 1 INTO rs_tok.
    ENDIF.
  ENDMETHOD.

  METHOD next.
    rs_tok = peek( ).
    pos = pos + 1.
  ENDMETHOD.

  METHOD expect.
    next( ).
  ENDMETHOD.

  METHOD parse_program.
    WHILE peek( )-kind <> 5.
      DATA(lr_s) = parse_statement( ).
      IF lr_s IS BOUND.
        APPEND lr_s TO rt_stmts.
      ENDIF.
    ENDWHILE.
  ENDMETHOD.

  METHOD parse_statement.
    DATA(ls_t) = peek( ).

    CASE ls_t-val.
      WHEN `let` OR `var` OR `const`.
        rr_node = parse_var( ).
        RETURN.
      WHEN `if`.
        rr_node = parse_if( ).
        RETURN.
      WHEN `while`.
        rr_node = parse_while( ).
        RETURN.
      WHEN `for`.
        rr_node = parse_for( ).
        RETURN.
      WHEN `function`.
        rr_node = parse_func( ).
        RETURN.
      WHEN `return`.
        rr_node = parse_return( ).
        RETURN.
      WHEN `break`.
        next( ).
        IF peek( )-val = `;`.
          next( ).
        ENDIF.
        DATA lr_brk TYPE REF TO zif_jseval=>ty_node.
        CREATE DATA lr_brk.
        lr_brk->kind = zif_jseval=>c_node_break.
        rr_node = lr_brk.
        RETURN.
      WHEN `continue`.
        next( ).
        IF peek( )-val = `;`.
          next( ).
        ENDIF.
        DATA lr_cont TYPE REF TO zif_jseval=>ty_node.
        CREATE DATA lr_cont.
        lr_cont->kind = zif_jseval=>c_node_continue.
        rr_node = lr_cont.
        RETURN.
      WHEN `switch`.
        rr_node = parse_switch( ).
        RETURN.
      WHEN `class`.
        rr_node = parse_class( ).
        RETURN.
      WHEN `{`.
        DATA lt_blk TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
        lt_blk = parse_block( ).
        DATA lr_blk TYPE REF TO zif_jseval=>ty_node.
        CREATE DATA lr_blk.
        lr_blk->kind = zif_jseval=>c_node_block.
        lr_blk->body = lt_blk.
        rr_node = lr_blk.
        RETURN.
      WHEN `;`.
        next( ).
        RETURN.
    ENDCASE.

    rr_node = parse_expr( ).
    IF peek( )-val = `;`.
      next( ).
    ENDIF.
  ENDMETHOD.

  METHOD parse_var.
    next( ).
    DATA(lv_name) = next( )-val.
    DATA lr_init TYPE REF TO data.
    IF peek( )-val = `=`.
      next( ).
      lr_init = parse_expr( ).
    ENDIF.
    IF peek( )-val = `;`.
      next( ).
    ENDIF.
    DATA lr_n TYPE REF TO zif_jseval=>ty_node.
    CREATE DATA lr_n.
    lr_n->kind = zif_jseval=>c_node_var.
    lr_n->str  = lv_name.
    lr_n->right = lr_init.
    rr_node = lr_n.
  ENDMETHOD.

  METHOD parse_if.
    next( ).
    expect( `(` ).
    DATA(lr_cond) = parse_expr( ).
    expect( `)` ).
    DATA(lt_body) = parse_body( ).
    DATA lt_else TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
    IF peek( )-val = `else`.
      next( ).
      IF peek( )-val = `if`.
        DATA(lr_elif) = parse_if( ).
        APPEND lr_elif TO lt_else.
      ELSE.
        lt_else = parse_body( ).
      ENDIF.
    ENDIF.
    DATA lr_n TYPE REF TO zif_jseval=>ty_node.
    CREATE DATA lr_n.
    lr_n->kind = zif_jseval=>c_node_if.
    lr_n->cond = lr_cond.
    lr_n->body = lt_body.
    lr_n->els  = lt_else.
    rr_node = lr_n.
  ENDMETHOD.

  METHOD parse_while.
    next( ).
    expect( `(` ).
    DATA(lr_cond) = parse_expr( ).
    expect( `)` ).
    DATA(lt_body) = parse_body( ).
    DATA lr_n TYPE REF TO zif_jseval=>ty_node.
    CREATE DATA lr_n.
    lr_n->kind = zif_jseval=>c_node_while.
    lr_n->cond = lr_cond.
    lr_n->body = lt_body.
    rr_node = lr_n.
  ENDMETHOD.

  METHOD parse_for.
    next( ).
    expect( `(` ).
    DATA lr_init TYPE REF TO data.
    DATA(lv_pv) = peek( )-val.
    IF lv_pv = `let` OR lv_pv = `var` OR lv_pv = `const`.
      lr_init = parse_var( ).
    ELSEIF lv_pv <> `;`.
      lr_init = parse_expr( ).
      IF peek( )-val = `;`.
        next( ).
      ENDIF.
    ELSE.
      next( ).
    ENDIF.
    DATA lr_cond TYPE REF TO data.
    IF peek( )-val <> `;`.
      lr_cond = parse_expr( ).
    ENDIF.
    IF peek( )-val = `;`.
      next( ).
    ENDIF.
    DATA lr_update TYPE REF TO data.
    IF peek( )-val <> `)`.
      lr_update = parse_expr( ).
    ENDIF.
    expect( `)` ).
    DATA(lt_body) = parse_body( ).
    DATA lr_n TYPE REF TO zif_jseval=>ty_node.
    CREATE DATA lr_n.
    lr_n->kind   = zif_jseval=>c_node_for.
    lr_n->init   = lr_init.
    lr_n->cond   = lr_cond.
    lr_n->update = lr_update.
    lr_n->body   = lt_body.
    rr_node = lr_n.
  ENDMETHOD.

  METHOD parse_switch.
    next( ).
    expect( `(` ).
    DATA(lr_expr) = parse_expr( ).
    expect( `)` ).
    expect( `{` ).
    DATA lt_cases TYPE zif_jseval=>tt_switch_cases.
    WHILE peek( )-val <> `}` AND peek( )-kind <> 5.
      IF peek( )-val = `case`.
        next( ).
        DATA(lr_ce) = parse_expr( ).
        expect( `:` ).
        DATA lt_cb TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
        CLEAR lt_cb.
        WHILE peek( )-val <> `case` AND peek( )-val <> `default`
              AND peek( )-val <> `}` AND peek( )-kind <> 5.
          DATA(lr_cs) = parse_statement( ).
          IF lr_cs IS BOUND.
            APPEND lr_cs TO lt_cb.
          ENDIF.
        ENDWHILE.
        DATA ls_case TYPE zif_jseval=>ty_switch_case.
        ls_case-expr = lr_ce.
        ls_case-body = lt_cb.
        APPEND ls_case TO lt_cases.
      ELSEIF peek( )-val = `default`.
        next( ).
        expect( `:` ).
        DATA lt_db TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
        CLEAR lt_db.
        WHILE peek( )-val <> `case` AND peek( )-val <> `}`
              AND peek( )-kind <> 5.
          DATA(lr_ds) = parse_statement( ).
          IF lr_ds IS BOUND.
            APPEND lr_ds TO lt_db.
          ENDIF.
        ENDWHILE.
        DATA ls_def TYPE zif_jseval=>ty_switch_case.
        CLEAR ls_def.
        ls_def-body = lt_db.
        APPEND ls_def TO lt_cases.
      ELSE.
        next( ).
      ENDIF.
    ENDWHILE.
    expect( `}` ).
    DATA lr_n TYPE REF TO zif_jseval=>ty_node.
    CREATE DATA lr_n.
    lr_n->kind  = zif_jseval=>c_node_switch.
    lr_n->cond  = lr_expr.
    lr_n->cases = lt_cases.
    rr_node = lr_n.
  ENDMETHOD.

  METHOD parse_class.
    next( ).
    DATA(lv_name) = next( )-val.
    expect( `{` ).
    DATA lt_methods TYPE zif_jseval=>tt_class_methods.
    WHILE peek( )-val <> `}` AND peek( )-kind <> 5.
      DATA(lv_mname) = next( )-val.
      expect( `(` ).
      DATA lt_params TYPE STANDARD TABLE OF string WITH DEFAULT KEY.
      CLEAR lt_params.
      WHILE peek( )-val <> `)` AND peek( )-kind <> 5.
        APPEND next( )-val TO lt_params.
        IF peek( )-val = `,`.
          next( ).
        ENDIF.
      ENDWHILE.
      expect( `)` ).
      DATA(lt_mbody) = parse_block( ).
      DATA ls_m TYPE zif_jseval=>ty_class_method.
      ls_m-name    = lv_mname.
      ls_m-params  = lt_params.
      ls_m-body    = lt_mbody.
      ls_m-is_ctor = boolc( lv_mname = `constructor` ).
      APPEND ls_m TO lt_methods.
    ENDWHILE.
    expect( `}` ).
    DATA lr_n TYPE REF TO zif_jseval=>ty_node.
    CREATE DATA lr_n.
    lr_n->kind    = zif_jseval=>c_node_class.
    lr_n->str     = lv_name.
    lr_n->methods = lt_methods.
    rr_node = lr_n.
  ENDMETHOD.

  METHOD parse_func.
    next( ).
    DATA(lv_name) = next( )-val.
    expect( `(` ).
    DATA lt_params TYPE STANDARD TABLE OF string WITH DEFAULT KEY.
    WHILE peek( )-val <> `)` AND peek( )-kind <> 5.
      APPEND next( )-val TO lt_params.
      IF peek( )-val = `,`.
        next( ).
      ENDIF.
    ENDWHILE.
    expect( `)` ).
    DATA(lt_body) = parse_body( ).
    DATA lr_n TYPE REF TO zif_jseval=>ty_node.
    CREATE DATA lr_n.
    lr_n->kind   = zif_jseval=>c_node_func_decl.
    lr_n->str    = lv_name.
    lr_n->params = lt_params.
    lr_n->body   = lt_body.
    rr_node = lr_n.
  ENDMETHOD.

  METHOD parse_return.
    next( ).
    DATA lr_val TYPE REF TO data.
    IF peek( )-val <> `;` AND peek( )-val <> `}` AND peek( )-kind <> 5.
      lr_val = parse_expr( ).
    ENDIF.
    IF peek( )-val = `;`.
      next( ).
    ENDIF.
    DATA lr_n TYPE REF TO zif_jseval=>ty_node.
    CREATE DATA lr_n.
    lr_n->kind = zif_jseval=>c_node_return.
    lr_n->left = lr_val.
    rr_node = lr_n.
  ENDMETHOD.

  METHOD parse_block.
    expect( `{` ).
    WHILE peek( )-val <> `}` AND peek( )-kind <> 5.
      DATA(lr_s) = parse_statement( ).
      IF lr_s IS BOUND.
        APPEND lr_s TO rt_stmts.
      ENDIF.
    ENDWHILE.
    expect( `}` ).
  ENDMETHOD.

  METHOD parse_body.
    IF peek( )-val = `{`.
      rt_stmts = parse_block( ).
    ELSE.
      DATA(lr_s) = parse_statement( ).
      IF lr_s IS BOUND.
        APPEND lr_s TO rt_stmts.
      ENDIF.
    ENDIF.
  ENDMETHOD.

  METHOD parse_expr.
    rr_node = parse_assign( ).
  ENDMETHOD.

  METHOD parse_assign.
    DATA(lr_left) = parse_or( ).
    IF peek( )-val = `=`.
      IF lr_left IS BOUND.
        FIELD-SYMBOLS <left_node> TYPE zif_jseval=>ty_node.
        ASSIGN lr_left->* TO <left_node>.
        IF <left_node>-kind = zif_jseval=>c_node_ident.
          next( ).
          DATA(lr_right) = parse_expr( ).
          DATA lr_n TYPE REF TO zif_jseval=>ty_node.
          CREATE DATA lr_n.
          lr_n->kind  = zif_jseval=>c_node_assign.
          lr_n->str   = <left_node>-str.
          lr_n->right = lr_right.
          rr_node = lr_n.
          RETURN.
        ENDIF.
        IF <left_node>-kind = zif_jseval=>c_node_member_access.
          next( ).
          DATA(lr_right2) = parse_expr( ).
          DATA lr_ma TYPE REF TO zif_jseval=>ty_node.
          CREATE DATA lr_ma.
          lr_ma->kind      = zif_jseval=>c_node_member_assign.
          lr_ma->object    = <left_node>-object.
          lr_ma->property  = <left_node>-property.
          lr_ma->prop_expr = <left_node>-prop_expr.
          lr_ma->right     = lr_right2.
          rr_node = lr_ma.
          RETURN.
        ENDIF.
      ENDIF.
    ENDIF.
    rr_node = lr_left.
  ENDMETHOD.

  METHOD parse_or.
    DATA(lr_left) = parse_and( ).
    WHILE peek( )-kind = 3 AND peek( )-val = `||`.
      DATA(lv_op) = next( )-val.
      DATA(lr_right) = parse_and( ).
      DATA lr_n TYPE REF TO zif_jseval=>ty_node.
      CREATE DATA lr_n.
      lr_n->kind  = zif_jseval=>c_node_binop.
      lr_n->op    = lv_op.
      lr_n->left  = lr_left.
      lr_n->right = lr_right.
      lr_left = lr_n.
    ENDWHILE.
    rr_node = lr_left.
  ENDMETHOD.

  METHOD parse_and.
    DATA(lr_left) = parse_equality( ).
    WHILE peek( )-kind = 3 AND peek( )-val = `&&`.
      DATA(lv_op) = next( )-val.
      DATA(lr_right) = parse_equality( ).
      DATA lr_n TYPE REF TO zif_jseval=>ty_node.
      CREATE DATA lr_n.
      lr_n->kind  = zif_jseval=>c_node_binop.
      lr_n->op    = lv_op.
      lr_n->left  = lr_left.
      lr_n->right = lr_right.
      lr_left = lr_n.
    ENDWHILE.
    rr_node = lr_left.
  ENDMETHOD.

  METHOD parse_equality.
    DATA(lr_left) = parse_comparison( ).
    WHILE peek( )-kind = 3 AND ( peek( )-val = `==` OR peek( )-val = `!=`
       OR peek( )-val = `===` OR peek( )-val = `!==` ).
      DATA(lv_op) = next( )-val.
      DATA(lr_right) = parse_comparison( ).
      DATA lr_n TYPE REF TO zif_jseval=>ty_node.
      CREATE DATA lr_n.
      lr_n->kind  = zif_jseval=>c_node_binop.
      lr_n->op    = lv_op.
      lr_n->left  = lr_left.
      lr_n->right = lr_right.
      lr_left = lr_n.
    ENDWHILE.
    rr_node = lr_left.
  ENDMETHOD.

  METHOD parse_comparison.
    DATA(lr_left) = parse_add_sub( ).
    WHILE peek( )-kind = 3 AND ( peek( )-val = `<` OR peek( )-val = `>`
       OR peek( )-val = `<=` OR peek( )-val = `>=` ).
      DATA(lv_op) = next( )-val.
      DATA(lr_right) = parse_add_sub( ).
      DATA lr_n TYPE REF TO zif_jseval=>ty_node.
      CREATE DATA lr_n.
      lr_n->kind  = zif_jseval=>c_node_binop.
      lr_n->op    = lv_op.
      lr_n->left  = lr_left.
      lr_n->right = lr_right.
      lr_left = lr_n.
    ENDWHILE.
    rr_node = lr_left.
  ENDMETHOD.

  METHOD parse_add_sub.
    DATA(lr_left) = parse_mul_div( ).
    WHILE peek( )-kind = 3 AND ( peek( )-val = `+` OR peek( )-val = `-` ).
      DATA(lv_op) = next( )-val.
      DATA(lr_right) = parse_mul_div( ).
      DATA lr_n TYPE REF TO zif_jseval=>ty_node.
      CREATE DATA lr_n.
      lr_n->kind  = zif_jseval=>c_node_binop.
      lr_n->op    = lv_op.
      lr_n->left  = lr_left.
      lr_n->right = lr_right.
      lr_left = lr_n.
    ENDWHILE.
    rr_node = lr_left.
  ENDMETHOD.

  METHOD parse_mul_div.
    DATA(lr_left) = parse_unary( ).
    WHILE peek( )-kind = 3 AND ( peek( )-val = `*` OR peek( )-val = `/` OR peek( )-val = `%` ).
      DATA(lv_op) = next( )-val.
      DATA(lr_right) = parse_unary( ).
      DATA lr_n TYPE REF TO zif_jseval=>ty_node.
      CREATE DATA lr_n.
      lr_n->kind  = zif_jseval=>c_node_binop.
      lr_n->op    = lv_op.
      lr_n->left  = lr_left.
      lr_n->right = lr_right.
      lr_left = lr_n.
    ENDWHILE.
    rr_node = lr_left.
  ENDMETHOD.

  METHOD parse_unary.
    IF peek( )-kind = 3 AND ( peek( )-val = `-` OR peek( )-val = `!` ).
      DATA(lv_op) = next( )-val.
      DATA(lr_operand) = parse_unary( ).
      DATA lr_n TYPE REF TO zif_jseval=>ty_node.
      CREATE DATA lr_n.
      lr_n->kind = zif_jseval=>c_node_unaryop.
      lr_n->op   = lv_op.
      lr_n->left = lr_operand.
      rr_node = lr_n.
      RETURN.
    ENDIF.
    IF peek( )-val = `typeof`.
      next( ).
      DATA(lr_op2) = parse_unary( ).
      DATA lr_to TYPE REF TO zif_jseval=>ty_node.
      CREATE DATA lr_to.
      lr_to->kind = zif_jseval=>c_node_typeof.
      lr_to->left = lr_op2.
      rr_node = lr_to.
      RETURN.
    ENDIF.
    IF peek( )-val = `new`.
      next( ).
      DATA(lv_name) = next( )-val.
      expect( `(` ).
      DATA lt_args TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
      WHILE peek( )-val <> `)` AND peek( )-kind <> 5.
        APPEND parse_expr( ) TO lt_args.
        IF peek( )-val = `,`.
          next( ).
        ENDIF.
      ENDWHILE.
      expect( `)` ).
      DATA lr_new TYPE REF TO zif_jseval=>ty_node.
      CREATE DATA lr_new.
      lr_new->kind = zif_jseval=>c_node_new.
      lr_new->str  = lv_name.
      lr_new->args = lt_args.
      rr_node = lr_new.
      RETURN.
    ENDIF.
    rr_node = parse_postfix( ).
  ENDMETHOD.

  METHOD parse_postfix.
    DATA(lr_left) = parse_primary( ).
    DATA lv_continue TYPE abap_bool.
    lv_continue = abap_true.
    WHILE lv_continue = abap_true.
      IF peek( )-val = `.`.
        next( ).
        DATA(lv_prop) = next( )-val.
        IF peek( )-val = `(`.
          next( ).
          DATA lt_margs TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
          CLEAR lt_margs.
          WHILE peek( )-val <> `)` AND peek( )-kind <> 5.
            APPEND parse_expr( ) TO lt_margs.
            IF peek( )-val = `,`.
              next( ).
            ENDIF.
          ENDWHILE.
          expect( `)` ).
          DATA lr_mc TYPE REF TO zif_jseval=>ty_node.
          CREATE DATA lr_mc.
          lr_mc->kind     = zif_jseval=>c_node_method_call.
          lr_mc->object   = lr_left.
          lr_mc->property = lv_prop.
          lr_mc->args     = lt_margs.
          lr_left = lr_mc.
        ELSE.
          DATA lr_ma TYPE REF TO zif_jseval=>ty_node.
          CREATE DATA lr_ma.
          lr_ma->kind     = zif_jseval=>c_node_member_access.
          lr_ma->object   = lr_left.
          lr_ma->property = lv_prop.
          lr_left = lr_ma.
        ENDIF.
      ELSEIF peek( )-val = `[`.
        next( ).
        DATA(lr_idx) = parse_expr( ).
        expect( `]` ).
        DATA lr_ba TYPE REF TO zif_jseval=>ty_node.
        CREATE DATA lr_ba.
        lr_ba->kind      = zif_jseval=>c_node_member_access.
        lr_ba->object    = lr_left.
        lr_ba->prop_expr = lr_idx.
        lr_left = lr_ba.
      ELSEIF peek( )-val = `(` AND lr_left IS BOUND.
        FIELD-SYMBOLS <ln> TYPE zif_jseval=>ty_node.
        ASSIGN lr_left->* TO <ln>.
        IF <ln>-kind = zif_jseval=>c_node_ident.
          next( ).
          DATA lt_fargs TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
          CLEAR lt_fargs.
          WHILE peek( )-val <> `)` AND peek( )-kind <> 5.
            APPEND parse_expr( ) TO lt_fargs.
            IF peek( )-val = `,`.
              next( ).
            ENDIF.
          ENDWHILE.
          expect( `)` ).
          DATA lr_call TYPE REF TO zif_jseval=>ty_node.
          CREATE DATA lr_call.
          lr_call->kind = zif_jseval=>c_node_call.
          lr_call->str  = <ln>-str.
          lr_call->args = lt_fargs.
          lr_left = lr_call.
        ELSE.
          lv_continue = abap_false.
        ENDIF.
      ELSE.
        lv_continue = abap_false.
      ENDIF.
    ENDWHILE.
    rr_node = lr_left.
  ENDMETHOD.

  METHOD parse_primary.
    DATA ls_t TYPE zif_jseval=>ty_token.
    ls_t = peek( ).

    IF ls_t-kind = 0.
      next( ).
      DATA lr_num TYPE REF TO zif_jseval=>ty_node.
      CREATE DATA lr_num.
      lr_num->kind = zif_jseval=>c_node_number.
      DATA lv_f TYPE f.
      lv_f = ls_t-val.
      lr_num->num  = lv_f.
      rr_node = lr_num.
      RETURN.
    ENDIF.

    IF ls_t-kind = 1.
      next( ).
      DATA lr_str TYPE REF TO zif_jseval=>ty_node.
      CREATE DATA lr_str.
      lr_str->kind = zif_jseval=>c_node_string.
      lr_str->str  = ls_t-val.
      rr_node = lr_str.
      RETURN.
    ENDIF.

    IF ls_t-val = `(`.
      next( ).
      DATA(lr_expr) = parse_expr( ).
      expect( `)` ).
      rr_node = lr_expr.
      RETURN.
    ENDIF.

    IF ls_t-val = `[`.
      rr_node = parse_array_literal( ).
      RETURN.
    ENDIF.

    IF ls_t-val = `{`.
      rr_node = parse_object_literal( ).
      RETURN.
    ENDIF.

    IF ls_t-val = `true`.
      next( ).
      DATA lr_true TYPE REF TO zif_jseval=>ty_node.
      CREATE DATA lr_true.
      lr_true->kind = zif_jseval=>c_node_number.
      lr_true->num  = 1.
      rr_node = lr_true.
      RETURN.
    ENDIF.

    IF ls_t-val = `false`.
      next( ).
      DATA lr_false TYPE REF TO zif_jseval=>ty_node.
      CREATE DATA lr_false.
      lr_false->kind = zif_jseval=>c_node_number.
      lr_false->num  = 0.
      rr_node = lr_false.
      RETURN.
    ENDIF.

    IF ls_t-kind = 2.
      next( ).
      DATA lv_name TYPE string.
      lv_name = ls_t-val.
      IF lv_name = `console` AND peek( )-val = `.`.
        next( ).
        DATA(lv_sub) = next( )-val.
        DATA(lv_full) = |{ lv_name }.{ lv_sub }|.
        IF peek( )-val = `(`.
          next( ).
          DATA lt_args TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
          WHILE peek( )-val <> `)` AND peek( )-kind <> 5.
            APPEND parse_expr( ) TO lt_args.
            IF peek( )-val = `,`.
              next( ).
            ENDIF.
          ENDWHILE.
          expect( `)` ).
          DATA lr_cl TYPE REF TO zif_jseval=>ty_node.
          CREATE DATA lr_cl.
          lr_cl->kind = zif_jseval=>c_node_call.
          lr_cl->str  = lv_full.
          lr_cl->args = lt_args.
          rr_node = lr_cl.
          RETURN.
        ENDIF.
        DATA lr_cid TYPE REF TO zif_jseval=>ty_node.
        CREATE DATA lr_cid.
        lr_cid->kind = zif_jseval=>c_node_ident.
        lr_cid->str  = lv_full.
        rr_node = lr_cid.
        RETURN.
      ENDIF.
      DATA lr_id TYPE REF TO zif_jseval=>ty_node.
      CREATE DATA lr_id.
      lr_id->kind = zif_jseval=>c_node_ident.
      lr_id->str  = lv_name.
      rr_node = lr_id.
      RETURN.
    ENDIF.

    next( ).
  ENDMETHOD.

  METHOD parse_array_literal.
    expect( `[` ).
    DATA lt_elems TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
    WHILE peek( )-val <> `]` AND peek( )-kind <> 5.
      APPEND parse_expr( ) TO lt_elems.
      IF peek( )-val = `,`.
        next( ).
      ENDIF.
    ENDWHILE.
    expect( `]` ).
    DATA lr_n TYPE REF TO zif_jseval=>ty_node.
    CREATE DATA lr_n.
    lr_n->kind = zif_jseval=>c_node_array.
    lr_n->args = lt_elems.
    rr_node = lr_n.
  ENDMETHOD.

  METHOD parse_object_literal.
    expect( `{` ).
    DATA lt_pairs TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.
    WHILE peek( )-val <> `}` AND peek( )-kind <> 5.
      DATA(lv_key) = next( )-val.
      expect( `:` ).
      DATA(lr_val) = parse_expr( ).
      DATA lr_k TYPE REF TO zif_jseval=>ty_node.
      CREATE DATA lr_k.
      lr_k->kind = zif_jseval=>c_node_string.
      lr_k->str  = lv_key.
      APPEND lr_k TO lt_pairs.
      APPEND lr_val TO lt_pairs.
      IF peek( )-val = `,`.
        next( ).
      ENDIF.
    ENDWHILE.
    expect( `}` ).
    DATA lr_n TYPE REF TO zif_jseval=>ty_node.
    CREATE DATA lr_n.
    lr_n->kind = zif_jseval=>c_node_object.
    lr_n->args = lt_pairs.
    rr_node = lr_n.
  ENDMETHOD.
ENDCLASS.
