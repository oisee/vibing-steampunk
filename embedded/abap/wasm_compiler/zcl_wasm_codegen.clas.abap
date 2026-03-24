CLASS zcl_wasm_codegen DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    TYPES: BEGIN OF ty_include,
             name   TYPE string,
             source TYPE string,
           END OF ty_include,
           ty_includes TYPE STANDARD TABLE OF ty_include WITH DEFAULT KEY.

    METHODS compile
      IMPORTING io_module  TYPE REF TO zcl_wasm_module
                iv_name    TYPE string DEFAULT 'ZWASM_OUT'
      RETURNING VALUE(rv)  TYPE string.
    METHODS split_to_includes
      IMPORTING iv_source    TYPE string
                iv_name      TYPE string
                iv_max_lines TYPE i DEFAULT 5000
      RETURNING VALUE(rt)    TYPE ty_includes.
    METHODS compile_class
      IMPORTING io_module    TYPE REF TO zcl_wasm_module
                iv_classname TYPE string DEFAULT 'ZCL_WASM_OUT'
                iv_wasm_hex  TYPE string OPTIONAL
                iv_program   TYPE string OPTIONAL
      RETURNING VALUE(rv)    TYPE string.
  PRIVATE SECTION.
    CONSTANTS: c_block TYPE i VALUE 1,
               c_loop  TYPE i VALUE 2,
               c_if    TYPE i VALUE 3.
    DATA mo_mod TYPE REF TO zcl_wasm_module.
    DATA mv_out TYPE string.
    DATA mv_indent TYPE i.
    DATA mv_stack_depth TYPE i.
    DATA mv_max_stack TYPE i.
    DATA mv_num_params TYPE i.
    DATA mv_num_locals TYPE i.
    DATA mv_func_idx TYPE i.
    DATA mv_has_result TYPE abap_bool.
    DATA mt_block_kinds TYPE STANDARD TABLE OF i WITH DEFAULT KEY.
    DATA mv_pack_buf TYPE string.
    DATA mv_pack_indent TYPE i VALUE -1.
    METHODS line IMPORTING iv TYPE string.
    METHODS push RETURNING VALUE(rv) TYPE string.
    METHODS pop RETURNING VALUE(rv) TYPE string.
    METHODS peek RETURNING VALUE(rv) TYPE string.
    METHODS emit_function IMPORTING is_func TYPE zcl_wasm_module=>ty_function.
    METHODS emit_instructions IMPORTING it_code TYPE zcl_wasm_module=>ty_instructions.
    METHODS emit_call IMPORTING iv_func_idx TYPE i.
    METHODS emit_raw_line IMPORTING iv TYPE string.
    METHODS flush.
    METHODS func_name IMPORTING iv_idx TYPE i RETURNING VALUE(rv) TYPE string.
    METHODS local_name IMPORTING iv_idx TYPE i RETURNING VALUE(rv) TYPE string.
    METHODS valtype_abap IMPORTING iv_type TYPE i RETURNING VALUE(rv) TYPE string.
ENDCLASS.

CLASS zcl_wasm_codegen IMPLEMENTATION.

  METHOD compile.
    mo_mod = io_module.
    CLEAR: mv_out, mv_indent, mv_pack_buf.
    mv_pack_indent = -1.

    " Program header
    line( |PROGRAM { iv_name }.| ).
    line( || ).

    " ── DATA declarations (TOP include) ──
    line( |TYPES: ty_x4 TYPE x LENGTH 4.| ).
    line( |DATA: gv_mem TYPE xstring, gv_mem_pages TYPE i, gv_br TYPE i.| ).
    line( |DATA: gv_wasm_initialized TYPE c.| ).
    line( |DATA: gv_xa TYPE ty_x4, gv_xb TYPE ty_x4, gv_xr TYPE ty_x4.| ).
    line( |DATA: gt_stk TYPE STANDARD TABLE OF i WITH DEFAULT KEY.| ).

    LOOP AT mo_mod->mt_globals INTO DATA(ls_g).
      DATA(lv_gi) = sy-tabix - 1.
      line( |DATA: gv_g{ lv_gi } TYPE { valtype_abap( ls_g-type ) }.| ).
    ENDLOOP.

    line( || ).

    " ── FORM WASM_INIT — called once before first use ──
    line( |FORM WASM_INIT.| ).
    mv_indent = mv_indent + 1.
    line( |IF gv_wasm_initialized = 'X'. RETURN. ENDIF.| ).
    line( |gv_wasm_initialized = 'X'.| ).

    IF mo_mod->ms_memory-min_pages > 0.
      DATA(lv_pages) = mo_mod->ms_memory-min_pages.
      line( |gv_mem_pages = { lv_pages }.| ).
      line( |DATA lv_pg TYPE xstring.| ).
      line( |lv_pg = '00'.| ).
      line( |DO 16 TIMES. CONCATENATE lv_pg lv_pg INTO lv_pg IN BYTE MODE. ENDDO.| ).
      line( |DO { lv_pages } TIMES. CONCATENATE gv_mem lv_pg INTO gv_mem IN BYTE MODE. ENDDO.| ).
    ENDIF.

    LOOP AT mo_mod->mt_globals INTO ls_g.
      DATA(lv_gi2) = sy-tabix - 1.
      IF ls_g-init_i32 <> 0.
        line( |gv_g{ lv_gi2 } = { ls_g-init_i32 }.| ).
      ENDIF.
    ENDLOOP.
    IF lines( mo_mod->mt_data ) > 0.
      line( |DATA lv_seg TYPE xstring.| ).
    ENDIF.
    LOOP AT mo_mod->mt_data INTO DATA(ls_d).
      DATA(lv_dlen) = xstrlen( ls_d-data ).
      IF lv_dlen > 0 AND lv_dlen <= 80.
        line( |lv_seg = '{ ls_d-data }'.| ).
        line( |REPLACE SECTION OFFSET { ls_d-offset } LENGTH { lv_dlen } OF gv_mem WITH lv_seg IN BYTE MODE.| ).
      ELSEIF lv_dlen > 80.
        DATA(lv_doff) = 0.
        WHILE lv_doff < lv_dlen.
          DATA(lv_dchunk) = 80.
          IF lv_doff + lv_dchunk > lv_dlen. lv_dchunk = lv_dlen - lv_doff. ENDIF.
          DATA(lv_chunk_x) = ls_d-data+lv_doff(lv_dchunk).
          DATA(lv_moff) = ls_d-offset + lv_doff.
          line( |lv_seg = '{ lv_chunk_x }'.| ).
          line( |REPLACE SECTION OFFSET { lv_moff } LENGTH { lv_dchunk } OF gv_mem WITH lv_seg IN BYTE MODE.| ).
          lv_doff = lv_doff + lv_dchunk.
        ENDWHILE.
      ENDIF.
    ENDLOOP.

    mv_indent = mv_indent - 1.
    line( |ENDFORM.| ).
    line( || ).

    " Memory helper FORMs (only if module uses memory)
    IF mo_mod->ms_memory-min_pages > 0 OR lines( mo_mod->mt_data ) > 0.
      line( |FORM mem_ld_i32 USING iv_addr TYPE i CHANGING rv TYPE i.| ).
      line( |  DATA lv_b TYPE xstring.| ).
      line( |  lv_b = gv_mem+iv_addr(4).| ).
      line( |  DATA lv_r TYPE xstring.| ).
      line( |  CONCATENATE lv_b+3(1) lv_b+2(1) lv_b+1(1) lv_b+0(1) INTO lv_r IN BYTE MODE.| ).
      line( |  rv = lv_r.| ).
      line( |ENDFORM.| ).
      line( || ).
      line( |FORM mem_st_i32 USING iv_addr TYPE i iv_val TYPE i.| ).
      line( |  DATA lv_hex TYPE c LENGTH 8.| ).
      line( |  lv_hex = iv_val.| ).
      line( |  DATA lv_b TYPE xstring. lv_b = lv_hex.| ).
      line( |  DATA lv_r TYPE xstring.| ).
      line( |  CONCATENATE lv_b+3(1) lv_b+2(1) lv_b+1(1) lv_b+0(1) INTO lv_r IN BYTE MODE.| ).
      line( |  REPLACE SECTION OFFSET iv_addr LENGTH 4 OF gv_mem WITH lv_r IN BYTE MODE.| ).
      line( |ENDFORM.| ).
      line( || ).
      line( |FORM mem_ld_i32_8u USING iv_addr TYPE i CHANGING rv TYPE i.| ).
      line( |  DATA lv_b TYPE xstring.| ).
      line( |  lv_b = gv_mem+iv_addr(1). rv = lv_b.| ).
      line( |ENDFORM.| ).
      line( || ).
      line( |FORM mem_st_i32_8 USING iv_addr TYPE i iv_val TYPE i.| ).
      line( |  DATA lv_hex TYPE c LENGTH 2.| ).
      line( |  lv_hex = iv_val.| ).
      line( |  DATA lv_b TYPE xstring. lv_b = lv_hex.| ).
      line( |  REPLACE SECTION OFFSET iv_addr LENGTH 1 OF gv_mem WITH lv_b IN BYTE MODE.| ).
      line( |ENDFORM.| ).
      line( || ).
    ENDIF.

    " Emit stub FORMs for imported functions (WASI etc.)
    LOOP AT mo_mod->mt_imports INTO DATA(ls_imp) WHERE kind = 0.
      READ TABLE mo_mod->mt_types INDEX ls_imp-type_index + 1 INTO DATA(ls_itype).
      IF sy-subrc <> 0. CONTINUE. ENDIF.
      DATA(lv_isig) = |FORM F{ ls_imp-func_index }|.
      IF lines( ls_itype-params ) > 0.
        lv_isig = lv_isig && | USING|.
        LOOP AT ls_itype-params INTO DATA(ls_ip).
          DATA(lv_ipi) = sy-tabix - 1.
          lv_isig = lv_isig && | VALUE(p{ lv_ipi }) TYPE { valtype_abap( ls_ip-type ) }|.
        ENDLOOP.
      ENDIF.
      IF lines( ls_itype-results ) > 0.
        lv_isig = lv_isig && | CHANGING rv TYPE { valtype_abap( ls_itype-results[ 1 ]-type ) }|.
      ENDIF.
      line( |{ lv_isig }.| ).
      mv_indent = mv_indent + 1.
      IF lines( ls_itype-results ) > 0.
        line( |rv = 0.| ).
      ENDIF.
      mv_indent = mv_indent - 1.
      line( |ENDFORM.| ).
      line( || ).
    ENDLOOP.

    " Emit functions
    LOOP AT mo_mod->mt_functions INTO DATA(ls_func).
      emit_function( ls_func ).
    ENDLOOP.

    flush( ).
    rv = mv_out.
  ENDMETHOD.


  METHOD line.
    " Empty line → flush + blank
    IF iv IS INITIAL.
      flush( ). emit_raw_line( iv ). RETURN.
    ENDIF.
    " Indent mismatch → auto-flush
    IF mv_pack_buf IS NOT INITIAL AND mv_indent <> mv_pack_indent.
      flush( ).
    ENDIF.
    " Check if non-packable (block boundaries, declarations)
    DATA(lv_np) = abap_false.
    DATA(lv_c1) = iv(1).
    DATA(lv_len) = strlen( iv ).
    CASE lv_c1.
      WHEN 'D'. " DO, DATA, DELETE
        IF lv_len >= 3 AND ( iv(3) = 'DO ' OR iv(3) = 'DO.' ).
          lv_np = abap_true.
        ELSEIF lv_len >= 4 AND iv(4) = 'DATA'.
          lv_np = abap_true.
        ELSEIF lv_len >= 7 AND iv(7) = 'DELETE '.
          lv_np = abap_true.
        ENDIF.
      WHEN 'E'. " ENDDO, ENDFORM, ELSE, ENDIF, ENDCLASS, ENDMETHOD
        IF iv = 'ENDDO.' OR iv = 'ENDFORM.' OR iv = 'ELSE.' OR iv = 'ENDIF.'
          OR iv = 'ENDCLASS.' OR iv = 'ENDMETHOD.' OR iv = 'ENDWHILE.'.
          lv_np = abap_true.
        ENDIF.
      WHEN 'F'. " FORM
        IF lv_len >= 5 AND iv(5) = 'FORM '.
          lv_np = abap_true.
        ENDIF.
      WHEN 'I'. " IF (block), INCLUDE
        IF lv_len >= 3 AND iv(3) = 'IF ' AND iv NS 'ENDIF'.
          lv_np = abap_true.
        ELSEIF lv_len >= 8 AND iv(8) = 'INCLUDE '.
          lv_np = abap_true.
        ENDIF.
      WHEN 'P'. " PROGRAM, PUBLIC, PRIVATE
        IF lv_len >= 7 AND ( iv(7) = 'PROGRAM' OR iv(7) = 'PUBLIC ' OR iv(7) = 'PRIVATE' ).
          lv_np = abap_true.
        ENDIF.
      WHEN 'C'. " CLASS
        IF lv_len >= 6 AND iv(6) = 'CLASS '.
          lv_np = abap_true.
        ENDIF.
      WHEN 'M'. " METHOD, METHODS
        IF lv_len >= 7 AND ( iv(7) = 'METHOD ' OR iv(7) = 'METHODS' ).
          lv_np = abap_true.
        ENDIF.
      WHEN 'T'. " TYPES
        IF lv_len >= 5 AND iv(5) = 'TYPES'.
          lv_np = abap_true.
        ENDIF.
      WHEN 'W'. " WHILE
        IF lv_len >= 6 AND iv(6) = 'WHILE '.
          lv_np = abap_true.
        ENDIF.
    ENDCASE.
    IF lv_np = abap_true.
      flush( ). emit_raw_line( iv ). RETURN.
    ENDIF.
    " Pack: accumulate statements on one line (max 250 minus indent prefix)
    DATA(lv_max) = 250 - mv_indent * 2.
    IF lv_max < 80. lv_max = 80. ENDIF.
    IF mv_pack_buf IS INITIAL.
      mv_pack_buf = iv. mv_pack_indent = mv_indent.
    ELSEIF strlen( mv_pack_buf ) + 1 + strlen( iv ) <= lv_max.
      mv_pack_buf = mv_pack_buf && ` ` && iv.
    ELSE.
      flush( ).
      mv_pack_buf = iv. mv_pack_indent = mv_indent.
    ENDIF.
  ENDMETHOD.


  METHOD emit_raw_line.
    DATA(lv_prefix) = ||.
    DO mv_indent TIMES. lv_prefix = lv_prefix && `  `. ENDDO.
    DATA(lv_full) = lv_prefix && iv.
    IF strlen( lv_full ) <= 255.
      mv_out = mv_out && lv_full && cl_abap_char_utilities=>newline.
    ELSE.
      DATA(lv_cont_prefix) = lv_prefix && `  `.
      DATA(lv_rest) = lv_full.
      WHILE strlen( lv_rest ) > 255.
        DATA(lv_cut) = 250.
        WHILE lv_cut > 40 AND lv_rest+lv_cut(1) <> ` `.
          lv_cut = lv_cut - 1.
        ENDWHILE.
        IF lv_cut <= 40. lv_cut = 250. ENDIF.
        mv_out = mv_out && lv_rest(lv_cut) && cl_abap_char_utilities=>newline.
        lv_rest = lv_cont_prefix && lv_rest+lv_cut.
        SHIFT lv_rest LEFT DELETING LEADING ` `.
        lv_rest = lv_cont_prefix && lv_rest.
      ENDWHILE.
      mv_out = mv_out && lv_rest && cl_abap_char_utilities=>newline.
    ENDIF.
  ENDMETHOD.


  METHOD flush.
    IF mv_pack_buf IS NOT INITIAL.
      DATA(lv_saved_indent) = mv_indent.
      mv_indent = mv_pack_indent.
      emit_raw_line( mv_pack_buf ).
      mv_indent = lv_saved_indent.
      CLEAR mv_pack_buf.
      mv_pack_indent = -1.
    ENDIF.
  ENDMETHOD.


  METHOD push.
    rv = |lv_s{ mv_stack_depth }|.
    mv_stack_depth = mv_stack_depth + 1.
    IF mv_stack_depth > mv_max_stack.
      mv_max_stack = mv_stack_depth.
    ENDIF.
  ENDMETHOD.


  METHOD pop.
    mv_stack_depth = mv_stack_depth - 1.
    rv = |lv_s{ mv_stack_depth }|.
  ENDMETHOD.


  METHOD peek.
    rv = |lv_s{ mv_stack_depth - 1 }|.
  ENDMETHOD.


  METHOD func_name.
    DATA(lv_fi) = iv_idx - mo_mod->mv_num_imported_funcs.
    IF lv_fi >= 0 AND lv_fi < lines( mo_mod->mt_functions ).
      READ TABLE mo_mod->mt_functions INDEX lv_fi + 1 INTO DATA(ls_f).
      IF sy-subrc = 0 AND ls_f-export_name IS NOT INITIAL.
        rv = ls_f-export_name.
        TRANSLATE rv TO UPPER CASE.
        RETURN.
      ENDIF.
    ENDIF.
    rv = |F{ iv_idx }|.
  ENDMETHOD.


  METHOD local_name.
    IF iv_idx < mv_num_params.
      rv = |p{ iv_idx }|.
    ELSE.
      rv = |l{ mv_func_idx }_{ iv_idx }|.
    ENDIF.
  ENDMETHOD.


  METHOD valtype_abap.
    " i32 → TYPE i, i64 → TYPE int8
    CASE iv_type.
      WHEN 127. rv = 'i'. " 0x7F = i32
      WHEN 126. rv = 'int8'. " 0x7E = i64
      WHEN OTHERS. rv = 'i'.
    ENDCASE.
  ENDMETHOD.


  METHOD emit_function.
    " Determine function name
    DATA(lv_name) = is_func-export_name.
    IF lv_name IS INITIAL. lv_name = |f{ is_func-index }|. ENDIF.
    TRANSLATE lv_name TO UPPER CASE.

    " Get function type
    READ TABLE mo_mod->mt_types INDEX is_func-type_index + 1 INTO DATA(ls_type).

    " Build FORM signature
    DATA(lv_sig) = |FORM { lv_name }|.

    mv_num_params = lines( ls_type-params ).
    mv_num_locals = lines( is_func-locals ).
    mv_func_idx = is_func-index.
    IF mv_num_params > 0.
      lv_sig = lv_sig && | USING|.
      LOOP AT ls_type-params INTO DATA(ls_p).
        DATA(lv_pi) = sy-tabix - 1.
        lv_sig = lv_sig && | p{ lv_pi } TYPE { valtype_abap( ls_p-type ) }|.
      ENDLOOP.
    ENDIF.

    mv_has_result = xsdbool( lines( ls_type-results ) > 0 ).
    DATA(lv_has_result) = mv_has_result.
    IF lv_has_result = abap_true.
      lv_sig = lv_sig && | CHANGING rv TYPE { valtype_abap( ls_type-results[ 1 ]-type ) }|.
    ENDIF.

    line( |{ lv_sig }.| ).
    mv_indent = mv_indent + 1.

    " Reset compiler state
    mv_stack_depth = 0.
    mv_max_stack = 0.
    CLEAR mt_block_kinds.

    " Emit instructions into temp buffer
    DATA(lv_saved) = mv_out.
    CLEAR mv_out.
    emit_instructions( is_func-code ).
    " If blocks remain unclosed, this function was partially parsed — discard body
    IF lines( mt_block_kinds ) > 0.
      CLEAR: mv_out, mt_block_kinds.
      mv_indent = 1.
      IF lv_has_result = abap_true.
        line( |rv = 0.| ).
      ENDIF.
    ELSE.
      IF lv_has_result = abap_true AND mv_stack_depth > 0.
        line( |rv = { peek( ) }.| ).
      ENDIF.
    ENDIF.
    flush( ).
    DATA(lv_body) = mv_out.
    mv_out = lv_saved.

    " Now emit DATA declarations for only what's needed
    LOOP AT is_func-locals INTO DATA(ls_l).
      DATA(lv_li) = sy-tabix - 1 + mv_num_params.
      line( |DATA: { local_name( lv_li ) } TYPE { valtype_abap( ls_l-type ) }.| ).
    ENDLOOP.

    " Stack variables — only declare up to mv_max_stack
    IF mv_max_stack > 0.
      DATA(lv_decl) = |DATA: lv_s0 TYPE i|.
      DATA(lv_si) = 1.
      WHILE lv_si < mv_max_stack.
        lv_decl = lv_decl && |, lv_s{ lv_si } TYPE i|.
        " Split line if getting long
        IF strlen( lv_decl ) > 200.
          line( |{ lv_decl }.| ).
          lv_si = lv_si + 1.
          IF lv_si < mv_max_stack.
            lv_decl = |DATA: lv_s{ lv_si } TYPE i|.
          ELSE.
            CLEAR lv_decl.
          ENDIF.
        ENDIF.
        lv_si = lv_si + 1.
      ENDWHILE.
      IF lv_decl IS NOT INITIAL.
        line( |{ lv_decl }.| ).
      ENDIF.
    ENDIF.

    " Append instruction body
    mv_out = mv_out && lv_body.

    mv_indent = mv_indent - 1.
    line( |ENDFORM.| ).
    line( || ).
  ENDMETHOD.


  METHOD emit_instructions.
    DATA: lv_a TYPE string, lv_b TYPE string, lv_r TYPE string, lv_c TYPE string.

    LOOP AT it_code INTO DATA(ls_i).
      CASE ls_i-op.

        " --- Constants ---
        WHEN 65. " i32.const
          line( |{ push( ) } = { ls_i-i32_value }.| ).
        WHEN 66. " i64.const
          line( |{ push( ) } = { ls_i-i64_value }.| ).

        " --- Local/Global access ---
        WHEN 32. " local.get
          line( |{ push( ) } = { local_name( ls_i-local_idx ) }.| ).
        WHEN 33. " local.set
          line( |{ local_name( ls_i-local_idx ) } = { pop( ) }.| ).
        WHEN 34. " local.tee
          line( |{ local_name( ls_i-local_idx ) } = { peek( ) }.| ).
        WHEN 35. " global.get
          line( |{ push( ) } = gv_g{ ls_i-global_idx }.| ).
        WHEN 36. " global.set
          line( |gv_g{ ls_i-global_idx } = { pop( ) }.| ).

        " --- i32 Arithmetic ---
        WHEN 106. " i32.add
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |{ lv_r } = { lv_a } + { lv_b }.| ).
        WHEN 107. " i32.sub
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |{ lv_r } = { lv_a } - { lv_b }.| ).
        WHEN 108. " i32.mul
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |{ lv_r } = { lv_a } * { lv_b }.| ).
        WHEN 109. " i32.div_s
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |{ lv_r } = { lv_a } / { lv_b }.| ).
        WHEN 111. " i32.rem_s
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |{ lv_r } = { lv_a } MOD { lv_b }.| ).

        " --- i32 Comparisons ---
        WHEN 69. " i32.eqz
          lv_a = pop( ). lv_r = push( ).
          line( |IF { lv_a } = 0. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 70. " i32.eq
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |IF { lv_a } = { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 71. " i32.ne
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |IF { lv_a } <> { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 72. " i32.lt_s
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |IF { lv_a } < { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 74. " i32.gt_s
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |IF { lv_a } > { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 76. " i32.le_s
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |IF { lv_a } <= { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 78. " i32.ge_s
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |IF { lv_a } >= { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).

        " --- Bitwise (via ipow) ---
        WHEN 113. " i32.and
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |gv_xa = { lv_a }. gv_xb = { lv_b }. gv_xr = gv_xa BIT-AND gv_xb. { lv_r } = gv_xr.| ).
        WHEN 114. " i32.or
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |gv_xa = { lv_a }. gv_xb = { lv_b }. gv_xr = gv_xa BIT-OR gv_xb. { lv_r } = gv_xr.| ).
        WHEN 115. " i32.xor
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |gv_xa = { lv_a }. gv_xb = { lv_b }. gv_xr = gv_xa BIT-XOR gv_xb. { lv_r } = gv_xr.| ).
        WHEN 116. " i32.shl
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |{ lv_r } = { lv_a } * ipow( base = 2 exp = { lv_b } MOD 32 ).| ).
        WHEN 117. " i32.shr_s
          lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |{ lv_r } = { lv_a } / ipow( base = 2 exp = { lv_b } MOD 32 ).| ).

        " --- Memory (pre-calculate addr+offset for PERFORM) ---
        WHEN 40. " i32.load
          lv_a = pop( ). lv_r = push( ).
          IF ls_i-offset > 0.
            line( |{ lv_r } = { lv_a } + { ls_i-offset }.| ).
            line( |PERFORM mem_ld_i32 USING { lv_r } CHANGING { lv_r }.| ).
          ELSE.
            line( |PERFORM mem_ld_i32 USING { lv_a } CHANGING { lv_r }.| ).
          ENDIF.
        WHEN 54. " i32.store
          lv_b = pop( ). lv_a = pop( ).
          IF ls_i-offset > 0.
            line( |{ lv_a } = { lv_a } + { ls_i-offset }.| ).
            line( |PERFORM mem_st_i32 USING { lv_a } { lv_b }.| ).
          ELSE.
            line( |PERFORM mem_st_i32 USING { lv_a } { lv_b }.| ).
          ENDIF.
        WHEN 44. " i32.load8_u
          lv_a = pop( ). lv_r = push( ).
          IF ls_i-offset > 0.
            line( |{ lv_r } = { lv_a } + { ls_i-offset }.| ).
            line( |PERFORM mem_ld_i32_8u USING { lv_r } CHANGING { lv_r }.| ).
          ELSE.
            line( |PERFORM mem_ld_i32_8u USING { lv_a } CHANGING { lv_r }.| ).
          ENDIF.
        WHEN 58. " i32.store8
          lv_b = pop( ). lv_a = pop( ).
          IF ls_i-offset > 0.
            line( |{ lv_a } = { lv_a } + { ls_i-offset }.| ).
            line( |PERFORM mem_st_i32_8 USING { lv_a } { lv_b }.| ).
          ELSE.
            line( |PERFORM mem_st_i32_8 USING { lv_a } { lv_b }.| ).
          ENDIF.

        " --- Control flow ---
        WHEN 2. " block
          line( |DO 1 TIMES. " block| ).
          mv_indent = mv_indent + 1.
          APPEND c_block TO mt_block_kinds.
        WHEN 3. " loop
          line( |DO. " loop| ).
          mv_indent = mv_indent + 1.
          APPEND c_loop TO mt_block_kinds.
        WHEN 4. " if
          lv_c = pop( ).
          line( |IF { lv_c } <> 0.| ).
          mv_indent = mv_indent + 1.
          APPEND c_if TO mt_block_kinds.
        WHEN 5. " else
          mv_indent = mv_indent - 1.
          line( |ELSE.| ).
          mv_indent = mv_indent + 1.
        WHEN 11. " end
          IF lines( mt_block_kinds ) > 0.
            DATA(lv_kind) = mt_block_kinds[ lines( mt_block_kinds ) ].
            DELETE mt_block_kinds INDEX lines( mt_block_kinds ).
            mv_indent = mv_indent - 1.
            CASE lv_kind.
              WHEN c_block.
                line( |ENDDO.| ).
                line( |IF gv_br > 0. gv_br = gv_br - 1. EXIT. ENDIF.| ).
              WHEN c_loop.
                line( |ENDDO.| ).
                line( |IF gv_br > 0. gv_br = gv_br - 1. EXIT. ENDIF.| ).
              WHEN c_if.
                line( |ENDIF.| ).
            ENDCASE.
          ENDIF.

        WHEN 12. " br
          IF ls_i-label_idx = 0.
            " Check if targeting a loop → CONTINUE, else → EXIT
            IF lines( mt_block_kinds ) > 0 AND
               mt_block_kinds[ lines( mt_block_kinds ) ] = c_loop.
              line( |CONTINUE. " br 0 (loop)| ).
            ELSE.
              line( |EXIT. " br 0| ).
            ENDIF.
          ELSE.
            line( |gv_br = { ls_i-label_idx }. EXIT. " br { ls_i-label_idx }| ).
          ENDIF.

        WHEN 13. " br_if
          lv_c = pop( ).
          IF ls_i-label_idx = 0.
            IF lines( mt_block_kinds ) > 0 AND
               mt_block_kinds[ lines( mt_block_kinds ) ] = c_loop.
              line( |IF { lv_c } <> 0. CONTINUE. ENDIF. " br_if 0 (loop)| ).
            ELSE.
              line( |IF { lv_c } <> 0. EXIT. ENDIF. " br_if 0| ).
            ENDIF.
          ELSE.
            line( |IF { lv_c } <> 0. gv_br = { ls_i-label_idx }. EXIT. ENDIF. " br_if { ls_i-label_idx }| ).
          ENDIF.

        WHEN 15. " return
          IF mv_has_result = abap_true AND mv_stack_depth > 0.
            line( |rv = { pop( ) }. RETURN.| ).
          ELSE.
            line( |RETURN.| ).
          ENDIF.

        " --- Call ---
        WHEN 16. " call
          emit_call( ls_i-func_idx ).

        " --- Stack ---
        WHEN 26. " drop
          pop( ).
        WHEN 27. " select
          lv_c = pop( ). lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |IF { lv_c } <> 0. { lv_r } = { lv_a }. ELSE. { lv_r } = { lv_b }. ENDIF.| ).

        " --- Nop / Unreachable ---
        WHEN 0. " unreachable
          line( |MESSAGE 'WASM unreachable' TYPE 'X'. " trap| ).
        WHEN 1. " nop
          " nothing

      ENDCASE.
    ENDLOOP.
  ENDMETHOD.


  METHOD emit_call.
    DATA(lv_fname) = func_name( iv_func_idx ).

    " Get function type — handle both imported and local functions
    DATA(lv_fi) = iv_func_idx - mo_mod->mv_num_imported_funcs.
    DATA ls_type TYPE zcl_wasm_module=>ty_functype.
    IF lv_fi < 0.
      " Imported function — look up type from imports
      READ TABLE mo_mod->mt_imports WITH KEY func_index = iv_func_idx INTO DATA(ls_imp).
      IF sy-subrc = 0.
        READ TABLE mo_mod->mt_types INDEX ls_imp-type_index + 1 INTO ls_type.
      ENDIF.
    ELSE.
      READ TABLE mo_mod->mt_functions INDEX lv_fi + 1 INTO DATA(ls_f).
      READ TABLE mo_mod->mt_types INDEX ls_f-type_index + 1 INTO ls_type.
    ENDIF.

    " Pop arguments in reverse
    DATA(lv_np) = lines( ls_type-params ).
    DATA: lt_args TYPE STANDARD TABLE OF string WITH DEFAULT KEY.
    DO lv_np TIMES.
      INSERT pop( ) INTO lt_args INDEX 1.
    ENDDO.

    " Save stack vars and locals before call (GENERATE shares DATA across recursive calls)
    DATA(lv_save_depth) = mv_stack_depth.
    DATA(lv_si) = 0.
    WHILE lv_si < lv_save_depth.
      line( |APPEND lv_s{ lv_si } TO gt_stk.| ).
      lv_si = lv_si + 1.
    ENDWHILE.
    lv_si = mv_num_params.
    WHILE lv_si < mv_num_params + mv_num_locals.
      line( |APPEND { local_name( lv_si ) } TO gt_stk.| ).
      lv_si = lv_si + 1.
    ENDWHILE.

    " Build PERFORM call
    DATA(lv_has_result) = xsdbool( lines( ls_type-results ) > 0 ).
    DATA(lv_result) = ||.
    IF lv_has_result = abap_true.
      lv_result = push( ).
    ENDIF.

    DATA(lv_call) = |PERFORM { lv_fname }|.
    IF lines( lt_args ) > 0.
      lv_call = lv_call && | USING|.
      LOOP AT lt_args INTO DATA(lv_arg).
        lv_call = lv_call && | { lv_arg }|.
      ENDLOOP.
    ENDIF.
    IF lv_has_result = abap_true.
      lv_call = lv_call && | CHANGING { lv_result }|.
    ENDIF.
    line( |{ lv_call }.| ).

    " Restore locals and stack vars after call (reverse order)
    lv_si = mv_num_params + mv_num_locals - 1.
    WHILE lv_si >= mv_num_params.
      line( |{ local_name( lv_si ) } = gt_stk[ lines( gt_stk ) ].| ).
      line( |DELETE gt_stk INDEX lines( gt_stk ).| ).
      lv_si = lv_si - 1.
    ENDWHILE.
    lv_si = lv_save_depth - 1.
    WHILE lv_si >= 0.
      line( |lv_s{ lv_si } = gt_stk[ lines( gt_stk ) ].| ).
      line( |DELETE gt_stk INDEX lines( gt_stk ).| ).
      lv_si = lv_si - 1.
    ENDWHILE.
  ENDMETHOD.


  METHOD split_to_includes.
    DATA: lt_lines TYPE STANDARD TABLE OF string,
          lv_top   TYPE string,
          lv_cur   TYPE string,
          lv_cur_lines TYPE i,
          lv_inc_num   TYPE i VALUE 1.

    DATA(lv_name) = iv_name.
    TRANSLATE lv_name TO UPPER CASE.

    SPLIT iv_source AT cl_abap_char_utilities=>newline INTO TABLE lt_lines.

    " Phase 1: separate TOP (everything before first FORM) from FORMs
    DATA(lv_in_top) = abap_true.
    DATA: lv_form_buf TYPE string,
          lv_form_lines TYPE i.

    LOOP AT lt_lines INTO DATA(lv_line).
      IF lv_in_top = abap_true.
        " Skip PROGRAM statement (main wrapper will have its own)
        IF lv_line CP 'PROGRAM *'.
          CONTINUE.
        ENDIF.
        " First FORM = end of TOP
        IF lv_line CP 'FORM *'.
          lv_in_top = abap_false.
          lv_form_buf = lv_line && cl_abap_char_utilities=>newline.
          lv_form_lines = 1.
          CONTINUE.
        ENDIF.
        lv_top = lv_top && lv_line && cl_abap_char_utilities=>newline.
      ELSE.
        lv_form_buf = lv_form_buf && lv_line && cl_abap_char_utilities=>newline.
        lv_form_lines = lv_form_lines + 1.

        " At ENDFORM boundary, check if current include is full
        IF lv_line CP 'ENDFORM*'.
          IF lv_cur_lines + lv_form_lines > iv_max_lines AND lv_cur IS NOT INITIAL.
            " Save current include
            DATA(lv_inc_name) = |{ lv_name }_F{ lv_inc_num WIDTH = 3 PAD = '0' ALIGN = RIGHT }|.
            APPEND VALUE ty_include( name = lv_inc_name source = lv_cur ) TO rt.
            lv_inc_num = lv_inc_num + 1.
            CLEAR lv_cur.
            lv_cur_lines = 0.
          ENDIF.
          " Add form to current include
          lv_cur = lv_cur && lv_form_buf.
          lv_cur_lines = lv_cur_lines + lv_form_lines.
          CLEAR: lv_form_buf, lv_form_lines.
        ENDIF.
      ENDIF.
    ENDLOOP.

    " Flush last include
    IF lv_cur IS NOT INITIAL.
      DATA(lv_last_name) = |{ lv_name }_F{ lv_inc_num WIDTH = 3 PAD = '0' ALIGN = RIGHT }|.
      APPEND VALUE ty_include( name = lv_last_name source = lv_cur ) TO rt.
    ENDIF.

    " Build TOP include (globals go here)
    DATA(lv_top_name) = |{ lv_name }_TOP|.
    INSERT VALUE ty_include( name = lv_top_name source = lv_top ) INTO rt INDEX 1.

    " Build main program with INCLUDEs
    DATA(lv_main) = |PROGRAM { lv_name }.| && cl_abap_char_utilities=>newline.
    LOOP AT rt INTO DATA(ls_inc).
      lv_main = lv_main && |INCLUDE { ls_inc-name }.| && cl_abap_char_utilities=>newline.
    ENDLOOP.
    INSERT VALUE ty_include( name = lv_name source = lv_main ) INTO rt INDEX 1.
  ENDMETHOD.


  METHOD compile_class.
    mo_mod = io_module.
    CLEAR: mv_out, mv_indent, mv_pack_buf.
    mv_pack_indent = -1.

    DATA(lv_cls) = iv_classname.
    TRANSLATE lv_cls TO UPPER CASE.

    " Static mode: iv_program provided → PROGRAM with local class, PERFORM IN PROGRAM
    " Dynamic mode: iv_wasm_hex provided → constructor with GENERATE POOL
    DATA(lv_static) = xsdbool( iv_program IS NOT INITIAL ).
    DATA(lv_prog) = iv_program.
    TRANSLATE lv_prog TO UPPER CASE.

    " Emit PROGRAM header for static wrapper
    IF lv_static = abap_true.
      line( |PROGRAM { lv_prog }_WRAP.| ).
      line( || ).
    ENDIF.

    " ── CLASS DEFINITION ──
    DATA(lv_visibility) = ||.
    IF lv_static = abap_false.
      lv_visibility = | PUBLIC|.
    ENDIF.
    line( |CLASS { lv_cls } DEFINITION{ lv_visibility } FINAL CREATE PUBLIC.| ).
    mv_indent = mv_indent + 1.
    line( |PUBLIC SECTION.| ).
    mv_indent = mv_indent + 1.

    IF lv_static = abap_false.
      line( |METHODS constructor.| ).
    ENDIF.

    " One method per exported function
    LOOP AT mo_mod->mt_exports INTO DATA(ls_exp) WHERE kind = 0.
      DATA(lv_fi) = ls_exp-index - mo_mod->mv_num_imported_funcs.
      IF lv_fi < 0 OR lv_fi >= lines( mo_mod->mt_functions ). CONTINUE. ENDIF.
      READ TABLE mo_mod->mt_functions INDEX lv_fi + 1 INTO DATA(ls_f).
      READ TABLE mo_mod->mt_types INDEX ls_f-type_index + 1 INTO DATA(ls_t).
      DATA(lv_mname) = ls_exp-name.
      TRANSLATE lv_mname TO UPPER CASE.
      DATA(lv_msig) = |METHODS { lv_mname }|.
      IF lines( ls_t-params ) > 0.
        lv_msig = lv_msig && | IMPORTING|.
        LOOP AT ls_t-params INTO DATA(ls_p).
          DATA(lv_pi) = sy-tabix - 1.
          lv_msig = lv_msig && | p{ lv_pi } TYPE { valtype_abap( ls_p-type ) }|.
        ENDLOOP.
      ENDIF.
      IF lines( ls_t-results ) > 0.
        lv_msig = lv_msig && | RETURNING VALUE(rv) TYPE { valtype_abap( ls_t-results[ 1 ]-type ) }|.
      ENDIF.
      line( |{ lv_msig }.| ).
    ENDLOOP.

    IF lv_static = abap_true.
      line( |METHODS constructor.| ).
    ENDIF.

    IF lv_static = abap_false.
      mv_indent = mv_indent - 1.
      line( |PRIVATE SECTION.| ).
      mv_indent = mv_indent + 1.
      line( |DATA mv_prog TYPE string.| ).
    ENDIF.

    mv_indent = mv_indent - 1.
    mv_indent = mv_indent - 1.
    line( |ENDCLASS.| ).
    line( || ).

    " ── CLASS IMPLEMENTATION ──
    line( |CLASS { lv_cls } IMPLEMENTATION.| ).
    mv_indent = mv_indent + 1.

    " Constructor
    IF lv_static = abap_true.
      " Static: just call WASM_INIT
      line( |METHOD constructor.| ).
      mv_indent = mv_indent + 1.
      line( |PERFORM WASM_INIT IN PROGRAM { lv_prog }.| ).
      mv_indent = mv_indent - 1.
      line( |ENDMETHOD.| ).
      line( || ).
    ENDIF.
    IF lv_static = abap_false.
      line( |METHOD constructor.| ).
      mv_indent = mv_indent + 1.
      line( |DATA lv_wasm TYPE xstring.| ).

      " Split hex into chunks of 200 chars
      DATA(lv_hex) = iv_wasm_hex.
      TRANSLATE lv_hex TO UPPER CASE.
      DATA(lv_hexlen) = strlen( lv_hex ).
      IF lv_hexlen <= 200.
        line( |lv_wasm = '{ lv_hex }'.| ).
      ELSE.
        DATA(lv_off) = 0.
        DATA(lv_first) = abap_true.
        WHILE lv_off < lv_hexlen.
          DATA(lv_chunk) = 200.
          IF lv_off + lv_chunk > lv_hexlen.
            lv_chunk = lv_hexlen - lv_off.
          ENDIF.
          DATA(lv_part) = lv_hex+lv_off(lv_chunk).
          IF lv_first = abap_true.
            line( |lv_wasm = '{ lv_part }' &&| ).
            lv_first = abap_false.
          ELSEIF lv_off + lv_chunk >= lv_hexlen.
            line( |  '{ lv_part }'.| ).
          ELSE.
            line( |  '{ lv_part }' &&| ).
          ENDIF.
          lv_off = lv_off + lv_chunk.
        ENDWHILE.
      ENDIF.

      line( |DATA(lo_mod) = NEW zcl_wasm_module( ).| ).
      line( |lo_mod->parse( lv_wasm ).| ).
      line( |DATA(lv_src) = NEW zcl_wasm_codegen( )->compile( lo_mod ).| ).
      line( |DATA lt_code TYPE STANDARD TABLE OF string.| ).
      line( |SPLIT lv_src AT cl_abap_char_utilities=>newline INTO TABLE lt_code.| ).
      line( |GENERATE SUBROUTINE POOL lt_code NAME mv_prog.| ).
      mv_indent = mv_indent - 1.
      line( |ENDMETHOD.| ).
      line( || ).
    ENDIF.

    " One method per export: delegate to PERFORM
    LOOP AT mo_mod->mt_exports INTO ls_exp WHERE kind = 0.
      lv_fi = ls_exp-index - mo_mod->mv_num_imported_funcs.
      IF lv_fi < 0 OR lv_fi >= lines( mo_mod->mt_functions ). CONTINUE. ENDIF.
      READ TABLE mo_mod->mt_functions INDEX lv_fi + 1 INTO ls_f.
      READ TABLE mo_mod->mt_types INDEX ls_f-type_index + 1 INTO ls_t.
      lv_mname = ls_exp-name.
      TRANSLATE lv_mname TO UPPER CASE.

      line( |METHOD { lv_mname }.| ).
      mv_indent = mv_indent + 1.

      " Static: PERFORM IN PROGRAM ZWASM_GEN (hardcoded)
      " Dynamic: PERFORM IN PROGRAM (mv_prog) (variable)
      DATA(lv_perf) = ||.
      IF lv_static = abap_true.
        lv_perf = |PERFORM { lv_mname } IN PROGRAM { lv_prog }|.
      ELSE.
        lv_perf = |PERFORM { lv_mname } IN PROGRAM (mv_prog)|.
      ENDIF.
      IF lines( ls_t-params ) > 0.
        lv_perf = lv_perf && | USING|.
        LOOP AT ls_t-params INTO ls_p.
          lv_pi = sy-tabix - 1.
          lv_perf = lv_perf && | p{ lv_pi }|.
        ENDLOOP.
      ENDIF.
      IF lines( ls_t-results ) > 0.
        lv_perf = lv_perf && | CHANGING rv|.
      ENDIF.
      line( |{ lv_perf }.| ).

      mv_indent = mv_indent - 1.
      line( |ENDMETHOD.| ).
      line( || ).
    ENDLOOP.

    mv_indent = mv_indent - 1.
    line( |ENDCLASS.| ).

    flush( ).
    rv = mv_out.
  ENDMETHOD.

ENDCLASS.
