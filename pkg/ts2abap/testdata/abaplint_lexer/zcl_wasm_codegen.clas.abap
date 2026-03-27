CLASS zcl_wasm_codegen DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    " Compile a parsed WASM module to ABAP source (single REPORT with FORMs)
    METHODS compile
      IMPORTING io_module  TYPE REF TO zcl_wasm_module
                iv_name    TYPE string DEFAULT 'ZWASM_OUT'
      RETURNING VALUE(rv)  TYPE string.
  PRIVATE SECTION.
    DATA mo_mod TYPE REF TO zcl_wasm_module.
    DATA mv_out TYPE string.
    DATA mv_indent TYPE i.
    DATA mv_stack_depth TYPE i.
    DATA mt_block_kinds TYPE STANDARD TABLE OF i WITH DEFAULT KEY. " 0=DO, 1=IF
    METHODS line IMPORTING iv TYPE string.
    METHODS push RETURNING VALUE(rv) TYPE string.
    METHODS pop RETURNING VALUE(rv) TYPE string.
    METHODS peek RETURNING VALUE(rv) TYPE string.
    METHODS emit_function IMPORTING is_func TYPE zcl_wasm_module=>ty_function.
    METHODS emit_instructions IMPORTING it_code TYPE zcl_wasm_module=>ty_instructions.
    METHODS emit_call IMPORTING iv_func_idx TYPE i.
    METHODS func_name IMPORTING iv_idx TYPE i RETURNING VALUE(rv) TYPE string.
    METHODS valtype_abap IMPORTING iv_type TYPE i RETURNING VALUE(rv) TYPE string.
ENDCLASS.

CLASS zcl_wasm_codegen IMPLEMENTATION.
  METHOD compile.
    mo_mod = io_module. CLEAR mv_out. mv_indent = 0.

    line( |REPORT { to_lower( iv_name ) }.| ).
    line( || ).

    " Globals
    line( |DATA gv_mem TYPE xstring.| ).
    line( |DATA gv_mem_pages TYPE i.| ).
    LOOP AT mo_mod->mt_globals INTO DATA(ls_g).
      line( |DATA gv_g{ sy-tabix - 1 } TYPE { valtype_abap( ls_g-type ) }.| ).
    ENDLOOP.
    LOOP AT mo_mod->mt_elements INTO DATA(ls_el).
      line( |DATA gt_tab{ sy-tabix - 1 } TYPE STANDARD TABLE OF i WITH DEFAULT KEY.| ).
    ENDLOOP.
    line( || ).

    " Init FORM
    line( |FORM wasm_init.| ). mv_indent = mv_indent + 1.
    IF mo_mod->ms_memory-min_pages > 0.
      DATA(lv_bytes) = mo_mod->ms_memory-min_pages * 65536.
      line( |gv_mem_pages = { mo_mod->ms_memory-min_pages }.| ).
      line( |DATA lv_chunk TYPE x LENGTH 256. DO { lv_bytes / 256 } TIMES. CONCATENATE gv_mem lv_chunk INTO gv_mem IN BYTE MODE. ENDDO.| ).
    ENDIF.
    LOOP AT mo_mod->mt_globals INTO ls_g.
      IF ls_g-init_i32 <> 0. line( |gv_g{ sy-tabix - 1 } = { ls_g-init_i32 }.| ). ENDIF.
    ENDLOOP.
    LOOP AT mo_mod->mt_data INTO DATA(ls_d).
      DATA lv_hex TYPE string.
      DATA(lv_dlen) = xstrlen( ls_d-data ).
      IF lv_dlen > 0 AND lv_dlen <= 100.
        lv_hex = ls_d-data.
        line( |gv_mem+{ ls_d-offset }({ lv_dlen }) = '{ lv_hex }'.| ).
      ENDIF.
    ENDLOOP.
    LOOP AT mo_mod->mt_elements INTO ls_el.
      LOOP AT ls_el-func_indices INTO DATA(lv_fi).
        line( |APPEND { lv_fi } TO gt_tab{ sy-tabix - 1 }.| ).
      ENDLOOP.
    ENDLOOP.
    mv_indent = mv_indent - 1. line( |ENDFORM.| ). line( || ).

    " Memory helpers (inline FORMs)
    line( |FORM mem_ld_i32 USING iv_addr TYPE i CHANGING rv TYPE i.| ). mv_indent = mv_indent + 1.
    line( |DATA lv_b TYPE x LENGTH 4. lv_b = gv_mem+iv_addr(4). DATA(lv_r) = lv_b+3(1) && lv_b+2(1) && lv_b+1(1) && lv_b+0(1). rv = lv_r.| ).
    mv_indent = mv_indent - 1. line( |ENDFORM.| ).
    line( |FORM mem_st_i32 USING iv_addr TYPE i iv_val TYPE i.| ). mv_indent = mv_indent + 1.
    line( |DATA lv_b TYPE x LENGTH 4. lv_b = iv_val. DATA(lv_r) = lv_b+3(1) && lv_b+2(1) && lv_b+1(1) && lv_b+0(1). gv_mem+iv_addr(4) = lv_r.| ).
    mv_indent = mv_indent - 1. line( |ENDFORM.| ).
    line( |FORM mem_ld_i32_8u USING iv_addr TYPE i CHANGING rv TYPE i.| ). mv_indent = mv_indent + 1.
    line( |DATA lv_b TYPE x LENGTH 1. lv_b = gv_mem+iv_addr(1). rv = lv_b.| ).
    mv_indent = mv_indent - 1. line( |ENDFORM.| ).
    line( |FORM mem_st_i32_8 USING iv_addr TYPE i iv_val TYPE i.| ). mv_indent = mv_indent + 1.
    line( |DATA lv_b TYPE x LENGTH 1. lv_b = iv_val. gv_mem+iv_addr(1) = lv_b.| ).
    mv_indent = mv_indent - 1. line( |ENDFORM.| ).
    line( || ).

    " Function FORMs
    LOOP AT mo_mod->mt_functions INTO DATA(ls_func).
      emit_function( ls_func ).
    ENDLOOP.

    rv = mv_out.
  ENDMETHOD.

  METHOD emit_function.
    DATA(lv_name) = func_name( sy-tabix - 1 ).
    DATA(lv_ti) = is_func-type_index.
    IF lv_ti < 0 OR lv_ti >= lines( mo_mod->mt_types ). RETURN. ENDIF.
    DATA(ls_type) = mo_mod->mt_types[ lv_ti + 1 ].

    " Build FORM signature
    DATA lv_sig TYPE string.
    DATA(lv_has_params) = lines( ls_type-params ).
    DATA(lv_has_result) = lines( ls_type-results ).
    IF lv_has_params > 0.
      DATA lv_params TYPE string.
      LOOP AT ls_type-params INTO DATA(ls_p).
        IF lv_params IS NOT INITIAL. lv_params = lv_params && | |. ENDIF.
        lv_params = lv_params && |p{ sy-tabix - 1 } TYPE { valtype_abap( ls_p-type ) }|.
      ENDLOOP.
      lv_sig = | USING { lv_params }|.
    ENDIF.
    IF lv_has_result > 0.
      lv_sig = lv_sig && | CHANGING rv TYPE { valtype_abap( ls_type-results[ 1 ]-type ) }|.
    ENDIF.

    line( |FORM { lv_name }{ lv_sig }.| ).
    mv_indent = mv_indent + 1.

    " DATA declarations — chained
    DATA lv_decls TYPE string.
    DATA(lv_num_params) = lines( ls_type-params ).
    LOOP AT is_func-locals INTO DATA(ls_l).
      DATA(lv_li) = lv_num_params + sy-tabix - 1.
      IF lv_decls IS NOT INITIAL. lv_decls = lv_decls && |, |. ENDIF.
      lv_decls = lv_decls && |l{ lv_li } TYPE { valtype_abap( ls_l-type ) }|.
    ENDLOOP.
    " Stack vars — estimate max depth
    DATA(lv_max_stack) = 8.
    IF lines( is_func-code ) > lv_max_stack. lv_max_stack = lines( is_func-code ) / 3. ENDIF.
    IF lv_max_stack > 64. lv_max_stack = 64. ENDIF.
    DO lv_max_stack TIMES.
      IF lv_decls IS NOT INITIAL. lv_decls = lv_decls && |, |. ENDIF.
      lv_decls = lv_decls && |s{ sy-index - 1 } TYPE i|.
    ENDDO.
    lv_decls = lv_decls && |, lv_br TYPE i|.
    line( |DATA: { lv_decls }.| ).

    " Emit instructions
    mv_stack_depth = 0.
    CLEAR mt_block_kinds.
    emit_instructions( is_func-code ).

    " Return value
    IF lv_has_result > 0 AND mv_stack_depth > 0.
      line( |rv = { peek( ) }.| ).
    ENDIF.

    mv_indent = mv_indent - 1. line( |ENDFORM.| ). line( || ).
  ENDMETHOD.

  METHOD emit_instructions.
    LOOP AT it_code INTO DATA(ls_i).
      CASE ls_i-op.
        WHEN 0. " nop
        WHEN 1. line( |RAISE EXCEPTION TYPE cx_sy_program_error.| ). " unreachable

        " Constants
        WHEN 65. line( |{ push( ) } = { ls_i-i32_value }.| ). " i32.const
        WHEN 66. line( |{ push( ) } = { ls_i-i64_value }.| ). " i64.const

        " Local/Global access
        WHEN 32. DATA(lv_ln) = ls_i-local_idx. line( |{ push( ) } = p{ lv_ln }.| ). " local.get (simplified: assumes params)
        WHEN 33. line( |p{ ls_i-local_idx } = { pop( ) }.| ). " local.set (simplified)
        WHEN 34. line( |p{ ls_i-local_idx } = { peek( ) }.| ). " local.tee
        WHEN 35. line( |{ push( ) } = gv_g{ ls_i-global_idx }.| ). " global.get
        WHEN 36. line( |gv_g{ ls_i-global_idx } = { pop( ) }.| ). " global.set

        " i32 arithmetic
        WHEN 106. DATA(lv_b) = pop( ). DATA(lv_a) = pop( ). line( |{ push( ) } = { lv_a } + { lv_b }.| ).
        WHEN 107. lv_b = pop( ). lv_a = pop( ). line( |{ push( ) } = { lv_a } - { lv_b }.| ).
        WHEN 108. lv_b = pop( ). lv_a = pop( ). line( |{ push( ) } = { lv_a } * { lv_b }.| ).
        WHEN 109. lv_b = pop( ). lv_a = pop( ). line( |{ push( ) } = { lv_a } / { lv_b }.| ).
        WHEN 111. lv_b = pop( ). lv_a = pop( ). line( |{ push( ) } = { lv_a } MOD { lv_b }.| ).

        " i32 comparisons
        WHEN 69. lv_a = pop( ). DATA(lv_r) = push( ). line( |IF { lv_a } = 0. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 70. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |IF { lv_a } = { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 71. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |IF { lv_a } <> { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 72. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |IF { lv_a } < { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 74. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |IF { lv_a } > { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 76. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |IF { lv_a } <= { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 78. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |IF { lv_a } >= { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).

        " Bitwise (via zcl_wasm_rt or BIT- operators)
        WHEN 113. lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |DATA lv_xa TYPE x LENGTH 4. DATA lv_xb TYPE x LENGTH 4. lv_xa = { lv_a }. lv_xb = { lv_b }. DATA(lv_xr) = lv_xa BIT-AND lv_xb. { lv_r } = lv_xr.| ).
        WHEN 114. lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |DATA lv_oa TYPE x LENGTH 4. DATA lv_ob TYPE x LENGTH 4. lv_oa = { lv_a }. lv_ob = { lv_b }. DATA(lv_or) = lv_oa BIT-OR lv_ob. { lv_r } = lv_or.| ).
        WHEN 115. lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |DATA lv_ea TYPE x LENGTH 4. DATA lv_eb TYPE x LENGTH 4. lv_ea = { lv_a }. lv_eb = { lv_b }. DATA(lv_er) = lv_ea BIT-XOR lv_eb. { lv_r } = lv_er.| ).

        " Memory
        WHEN 40. " i32.load
          lv_a = pop( ). lv_r = push( ).
          DATA(lv_addr) = lv_a. IF ls_i-offset > 0. lv_addr = |{ lv_a } + { ls_i-offset }|. ENDIF.
          line( |PERFORM mem_ld_i32 USING { lv_addr } CHANGING { lv_r }.| ).
        WHEN 54. " i32.store
          DATA(lv_val) = pop( ). lv_a = pop( ).
          lv_addr = lv_a. IF ls_i-offset > 0. lv_addr = |{ lv_a } + { ls_i-offset }|. ENDIF.
          line( |PERFORM mem_st_i32 USING { lv_addr } { lv_val }.| ).
        WHEN 45. " i32.load8_u
          lv_a = pop( ). lv_r = push( ).
          lv_addr = lv_a. IF ls_i-offset > 0. lv_addr = |{ lv_a } + { ls_i-offset }|. ENDIF.
          line( |PERFORM mem_ld_i32_8u USING { lv_addr } CHANGING { lv_r }.| ).
        WHEN 58. " i32.store8
          lv_val = pop( ). lv_a = pop( ).
          lv_addr = lv_a. IF ls_i-offset > 0. lv_addr = |{ lv_a } + { ls_i-offset }|. ENDIF.
          line( |PERFORM mem_st_i32_8 USING { lv_addr } { lv_val }.| ).
        WHEN 63. lv_r = push( ). line( |{ lv_r } = gv_mem_pages.| ). " memory.size
        WHEN 64. " memory.grow
          lv_a = pop( ). lv_r = push( ).
          line( |{ lv_r } = gv_mem_pages. DATA lv_z TYPE x LENGTH 256. DO { lv_a } * 256 TIMES. CONCATENATE gv_mem lv_z INTO gv_mem IN BYTE MODE. ENDDO. gv_mem_pages = gv_mem_pages + { lv_a }.| ).

        " Control flow
        WHEN 2. line( |DO 1 TIMES.| ). mv_indent = mv_indent + 1. APPEND 0 TO mt_block_kinds. " block
        WHEN 3. line( |DO.| ). mv_indent = mv_indent + 1. APPEND 0 TO mt_block_kinds. " loop
        WHEN 4. DATA(lv_cond) = pop( ). line( |IF { lv_cond } <> 0.| ). mv_indent = mv_indent + 1. APPEND 1 TO mt_block_kinds. " if
        WHEN 5. mv_indent = mv_indent - 1. line( |ELSE.| ). mv_indent = mv_indent + 1. " else
        WHEN 11. " end
          IF lines( mt_block_kinds ) > 0.
            DATA(lv_kind) = mt_block_kinds[ lines( mt_block_kinds ) ].
            DELETE mt_block_kinds INDEX lines( mt_block_kinds ).
            mv_indent = mv_indent - 1.
            IF lv_kind = 1. line( |ENDIF.| ). ELSE. line( |ENDDO.| ). ENDIF.
          ENDIF.
        WHEN 12. " br
          IF ls_i-label_idx = 0. line( |EXIT.| ). ELSE. line( |lv_br = { ls_i-label_idx }. EXIT.| ). ENDIF.
        WHEN 13. " br_if
          lv_cond = pop( ).
          IF ls_i-label_idx = 0. line( |IF { lv_cond } <> 0. EXIT. ENDIF.| ). ELSE. line( |IF { lv_cond } <> 0. lv_br = { ls_i-label_idx }. EXIT. ENDIF.| ). ENDIF.
        WHEN 15. " return
          IF lines( mo_mod->mt_types ) > 0 AND mv_stack_depth > 0.
            line( |rv = { pop( ) }. RETURN.| ).
          ELSE.
            line( |RETURN.| ).
          ENDIF.

        " Call
        WHEN 16. emit_call( ls_i-func_idx ). " call

        " Stack
        WHEN 26. pop( ). " drop
        WHEN 27. " select
          DATA(lv_c) = pop( ). lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |IF { lv_c } <> 0. { lv_r } = { lv_a }. ELSE. { lv_r } = { lv_b }. ENDIF.| ).

        " Conversions (simplified)
        WHEN 167. lv_a = pop( ). line( |{ push( ) } = { lv_a }.| ). " i32.wrap_i64
        WHEN 172. lv_a = pop( ). line( |{ push( ) } = { lv_a }.| ). " i64.extend_i32_s

        WHEN OTHERS. line( |" TODO: opcode { ls_i-op }.| ).
      ENDCASE.
    ENDLOOP.
  ENDMETHOD.

  METHOD emit_call.
    IF iv_func_idx < mo_mod->mv_num_imported_funcs.
      " Import call — find import info
      LOOP AT mo_mod->mt_imports INTO DATA(ls_imp) WHERE kind = 0.
        IF ls_imp-func_index = iv_func_idx.
          line( |" IMPORT: { ls_imp-module }.{ ls_imp-name } (stub).| ).
          " Pop params, push result if needed
          IF ls_imp-type_index < lines( mo_mod->mt_types ).
            DATA(ls_it) = mo_mod->mt_types[ ls_imp-type_index + 1 ].
            DO lines( ls_it-params ) TIMES. pop( ). ENDDO.
            IF lines( ls_it-results ) > 0. line( |{ push( ) } = 0.| ). ENDIF.
          ENDIF.
          RETURN.
        ENDIF.
      ENDLOOP.
      RETURN.
    ENDIF.

    DATA(lv_local_idx) = iv_func_idx - mo_mod->mv_num_imported_funcs.
    IF lv_local_idx >= lines( mo_mod->mt_functions ). RETURN. ENDIF.
    DATA(ls_target) = mo_mod->mt_functions[ lv_local_idx + 1 ].
    IF ls_target-type_index >= lines( mo_mod->mt_types ). RETURN. ENDIF.
    DATA(ls_type) = mo_mod->mt_types[ ls_target-type_index + 1 ].

    " Pop arguments (reverse order)
    DATA lt_args TYPE string_table.
    DO lines( ls_type-params ) TIMES. INSERT pop( ) INTO lt_args INDEX 1. ENDDO.

    DATA(lv_name) = func_name( lv_local_idx ).
    DATA(lv_has_result) = lines( ls_type-results ).

    DATA lv_args TYPE string.
    LOOP AT lt_args INTO DATA(lv_arg).
      IF lv_args IS NOT INITIAL. lv_args = lv_args && | |. ENDIF.
      lv_args = lv_args && lv_arg.
    ENDLOOP.

    IF lv_has_result > 0.
      DATA(lv_result) = push( ).
      IF lv_args IS NOT INITIAL.
        line( |PERFORM { lv_name } USING { lv_args } CHANGING { lv_result }.| ).
      ELSE.
        line( |PERFORM { lv_name } CHANGING { lv_result }.| ).
      ENDIF.
    ELSE.
      IF lv_args IS NOT INITIAL.
        line( |PERFORM { lv_name } USING { lv_args }.| ).
      ELSE.
        line( |PERFORM { lv_name }.| ).
      ENDIF.
    ENDIF.
  ENDMETHOD.

  METHOD func_name.
    " Check if function has an export name
    IF iv_idx >= 0 AND iv_idx < lines( mo_mod->mt_functions ).
      DATA(ls_f) = mo_mod->mt_functions[ iv_idx + 1 ].
      IF ls_f-export_name IS NOT INITIAL.
        rv = to_lower( ls_f-export_name ).
        REPLACE ALL OCCURRENCES OF '-' IN rv WITH '_'.
        IF strlen( rv ) > 30. rv = rv(30). ENDIF.
        RETURN.
      ENDIF.
    ENDIF.
    rv = |f{ iv_idx }|.
  ENDMETHOD.

  METHOD valtype_abap.
    CASE iv_type.
      WHEN 127. rv = 'i'. " 0x7F = i32
      WHEN 126. rv = 'int8'. " 0x7E = i64
      WHEN 125 OR 124. rv = 'f'. " f32/f64
      WHEN OTHERS. rv = 'i'.
    ENDCASE.
  ENDMETHOD.

  METHOD line.
    DO mv_indent TIMES. mv_out = mv_out && |  |. ENDDO.
    mv_out = mv_out && iv && cl_abap_char_utilities=>newline.
  ENDMETHOD.

  METHOD push.
    rv = |s{ mv_stack_depth }|. mv_stack_depth = mv_stack_depth + 1.
  ENDMETHOD.

  METHOD pop.
    IF mv_stack_depth > 0. mv_stack_depth = mv_stack_depth - 1. ENDIF.
    rv = |s{ mv_stack_depth }|.
  ENDMETHOD.

  METHOD peek.
    IF mv_stack_depth > 0.
      rv = |s{ mv_stack_depth - 1 }|.
    ELSE.
      rv = |s0|.
    ENDIF.
  ENDMETHOD.
ENDCLASS.
