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
    DATA mv_unreachable TYPE abap_bool.
    DATA mv_dead_depth TYPE i.
    DATA mt_block_kinds TYPE STANDARD TABLE OF i WITH DEFAULT KEY.
    DATA mv_pack_buf TYPE string.
    DATA mv_pack_indent TYPE i VALUE -1.
    " Block-as-method: shared prefix for g=> (from FORMs) or empty (from class methods)
    DATA mv_prefix TYPE string.
    " Block method collection
    TYPES: BEGIN OF ty_block_method,
             name TYPE string,
             body TYPE string,
           END OF ty_block_method.
    DATA mt_block_methods TYPE STANDARD TABLE OF ty_block_method WITH DEFAULT KEY.
    DATA mv_block_counter TYPE i.
    DATA mv_in_block_method TYPE abap_bool.
    " Global max vars (computed during pre-scan)
    DATA mv_gmax_params TYPE i.
    DATA mv_gmax_locals TYPE i.
    DATA mv_gmax_stack TYPE i.
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
    METHODS find_matching_end
      IMPORTING it_code TYPE zcl_wasm_module=>ty_instructions
                iv_start TYPE i
      RETURNING VALUE(rv) TYPE i.
    METHODS exit_or_return RETURNING VALUE(rv) TYPE string.
    METHODS emit_br_propagation.
    METHODS emit_br_propagate.
    METHODS pre_scan_max_vars.
ENDCLASS.

CLASS zcl_wasm_codegen IMPLEMENTATION.

  METHOD compile.
    mo_mod = io_module.
    CLEAR: mv_out, mv_indent, mv_pack_buf, mt_block_methods, mv_block_counter.
    mv_pack_indent = -1.

    " Pre-scan: find max params, locals, stack across all functions
    pre_scan_max_vars( ).

    " ── Phase 1: Emit function FORMs (collecting block methods) ──
    DATA lv_forms TYPE string.
    DATA(lv_saved) = mv_out.
    CLEAR mv_out.

    " Emit stub FORMs for imported functions (WASI etc.)
    LOOP AT mo_mod->mt_imports INTO DATA(ls_imp) WHERE kind = 0.
      READ TABLE mo_mod->mt_types INDEX ls_imp-type_index + 1 INTO DATA(ls_itype).
      IF sy-subrc <> 0. CONTINUE. ENDIF.
      DATA(lv_isig) = |FORM F{ ls_imp-func_index }|.
      IF lines( ls_itype-params ) > 0.
        lv_isig = lv_isig && | USING|.
        LOOP AT ls_itype-params INTO DATA(ls_ip).
          DATA(lv_ipi) = sy-tabix - 1.
          lv_isig = lv_isig && | p{ lv_ipi } TYPE i|.
        ENDLOOP.
      ENDIF.
      IF lines( ls_itype-results ) > 0.
        lv_isig = lv_isig && | CHANGING rv TYPE i|.
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
    lv_forms = mv_out.

    " ── Phase 2: Build complete program ──
    mv_out = lv_saved.

    " Program header
    line( |PROGRAM { iv_name }.| ).
    line( || ).

    " DATA declarations
    line( |TYPES: ty_x4 TYPE x LENGTH 4.| ).
    line( |DATA: gv_mem TYPE xstring, gv_mem_pages TYPE i.| ).
    line( |DATA: gv_wasm_initialized TYPE c.| ).
    line( |DATA: gv_xa TYPE ty_x4, gv_xb TYPE ty_x4, gv_xr TYPE ty_x4.| ).
    line( |DATA: gt_stk TYPE STANDARD TABLE OF i WITH DEFAULT KEY.| ).

    LOOP AT mo_mod->mt_globals INTO DATA(ls_g).
      DATA(lv_gi) = sy-tabix - 1.
      line( |DATA: gv_g{ lv_gi } TYPE { valtype_abap( ls_g-type ) }.| ).
    ENDLOOP.
    line( || ).

    " ── CLASS g: shared state + block methods ──
    line( |CLASS g DEFINITION.| ).
    line( |  PUBLIC SECTION.| ).

    " Params
    DO mv_gmax_params TIMES.
      line( |    CLASS-DATA p{ sy-index - 1 } TYPE i.| ).
    ENDDO.
    " Locals — declare l0 to l{max_local_index}
    " mv_gmax_locals is max(numParams + numLocals) across all functions
    DATA(lv_li) = 0.
    WHILE lv_li < mv_gmax_locals.
      line( |    CLASS-DATA l{ lv_li } TYPE i.| ).
      lv_li = lv_li + 1.
    ENDWHILE.
    " Stack vars
    DO mv_gmax_stack TIMES.
      line( |    CLASS-DATA s{ sy-index - 1 } TYPE i.| ).
    ENDDO.
    " Branch + return value
    line( |    CLASS-DATA br TYPE i.| ).
    line( |    CLASS-DATA rv TYPE i.| ).
    " Method declarations
    LOOP AT mt_block_methods INTO DATA(ls_bm).
      line( |    CLASS-METHODS { ls_bm-name }.| ).
    ENDLOOP.
    line( |ENDCLASS.| ).
    line( || ).

    " CLASS g IMPLEMENTATION — flush packer + use emit_raw_line to avoid reordering
    flush( ).
    emit_raw_line( |CLASS g IMPLEMENTATION.| ).
    LOOP AT mt_block_methods INTO ls_bm.
      emit_raw_line( |  METHOD { ls_bm-name }.| ).
      mv_out = mv_out && ls_bm-body.
      emit_raw_line( |  ENDMETHOD.| ).
    ENDLOOP.
    emit_raw_line( |ENDCLASS.| ).
    line( || ).

    " ── FORM WASM_INIT ──
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
      IF ls_g-init_i32 <> 0. line( |gv_g{ lv_gi2 } = { ls_g-init_i32 }.| ). ENDIF.
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

    " Memory helpers
    IF mo_mod->ms_memory-min_pages > 0 OR lines( mo_mod->mt_data ) > 0.
      line( |FORM mem_ld_i32 USING iv_addr TYPE i CHANGING rv TYPE i.| ).
      line( |  IF iv_addr < 0 OR iv_addr + 4 > xstrlen( gv_mem ). rv = 0. RETURN. ENDIF.| ).
      line( |  DATA lv_b TYPE xstring. lv_b = gv_mem+iv_addr(4).| ).
      line( |  DATA lv_r TYPE xstring.| ).
      line( |  CONCATENATE lv_b+3(1) lv_b+2(1) lv_b+1(1) lv_b+0(1) INTO lv_r IN BYTE MODE. rv = lv_r.| ).
      line( |ENDFORM.| ).
      line( || ).
      line( |FORM mem_st_i32 USING iv_addr TYPE i iv_val TYPE i.| ).
      line( |  IF iv_addr < 0 OR iv_addr + 4 > xstrlen( gv_mem ). RETURN. ENDIF.| ).
      line( |  DATA lv_x TYPE x LENGTH 4. lv_x = iv_val. DATA lv_r TYPE xstring.| ).
      line( |  CONCATENATE lv_x+3(1) lv_x+2(1) lv_x+1(1) lv_x+0(1) INTO lv_r IN BYTE MODE.| ).
      line( |  REPLACE SECTION OFFSET iv_addr LENGTH 4 OF gv_mem WITH lv_r IN BYTE MODE.| ).
      line( |ENDFORM.| ).
      line( || ).
      line( |FORM mem_ld_i32_8u USING iv_addr TYPE i CHANGING rv TYPE i.| ).
      line( |  IF iv_addr < 0 OR iv_addr + 1 > xstrlen( gv_mem ). rv = 0. RETURN. ENDIF.| ).
      line( |  DATA lv_b TYPE xstring. lv_b = gv_mem+iv_addr(1). rv = lv_b.| ).
      line( |ENDFORM.| ).
      line( || ).
      line( |FORM mem_st_i32_8 USING iv_addr TYPE i iv_val TYPE i.| ).
      line( |  IF iv_addr < 0 OR iv_addr + 1 > xstrlen( gv_mem ). RETURN. ENDIF.| ).
      line( |  DATA lv_x TYPE x LENGTH 1. lv_x = iv_val. DATA lv_b TYPE xstring. lv_b = lv_x.| ).
      line( |  REPLACE SECTION OFFSET iv_addr LENGTH 1 OF gv_mem WITH lv_b IN BYTE MODE.| ).
      line( |ENDFORM.| ).
      line( || ).
    ENDIF.

    " Function FORMs (generated in phase 1)
    mv_out = mv_out && lv_forms.

    flush( ).
    rv = mv_out.
  ENDMETHOD.


  METHOD pre_scan_max_vars.
    mv_gmax_params = 0. mv_gmax_locals = 0. mv_gmax_stack = 0.
    LOOP AT mo_mod->mt_functions INTO DATA(ls_f).
      READ TABLE mo_mod->mt_types INDEX ls_f-type_index + 1 INTO DATA(ls_t).
      IF sy-subrc <> 0. CONTINUE. ENDIF.
      DATA(lv_np) = lines( ls_t-params ).
      DATA(lv_nl) = lines( ls_f-locals ).
      IF lv_np > mv_gmax_params. mv_gmax_params = lv_np. ENDIF.
      " Track max local INDEX (numParams + numLocals) not just count
      DATA(lv_max_lidx) = lv_np + lv_nl.
      IF lv_max_lidx > mv_gmax_locals. mv_gmax_locals = lv_max_lidx. ENDIF.
      " Also scan instructions for actual local_idx values (parser may exceed declared locals)
      LOOP AT ls_f-code INTO DATA(ls_si) WHERE op = 32 OR op = 33 OR op = 34.
        IF ls_si-local_idx + 1 > mv_gmax_locals. mv_gmax_locals = ls_si-local_idx + 1. ENDIF.
      ENDLOOP.
      " Estimate max stack depth
      DATA(lv_ms) = 8. " minimum
      DATA(lv_d) = 0.
      LOOP AT ls_f-code INTO DATA(ls_i).
        CASE ls_i-op.
          WHEN 65 OR 66 OR 32 OR 35 OR 63. lv_d = lv_d + 1. " push
          WHEN 33 OR 36 OR 26 OR 54 OR 58.  lv_d = lv_d - 1. " pop
          WHEN 106 OR 107 OR 108 OR 109 OR 111 OR 113 OR 114 OR 115 OR 116 OR 117
            OR 70 OR 71 OR 72 OR 74 OR 76 OR 78 OR 40 OR 44. lv_d = lv_d. " net 0 or -1+1
        ENDCASE.
        IF lv_d < 0. lv_d = 0. ENDIF.
        IF lv_d > lv_ms. lv_ms = lv_d. ENDIF.
      ENDLOOP.
      IF lv_ms > mv_gmax_stack. mv_gmax_stack = lv_ms. ENDIF.
    ENDLOOP.
  ENDMETHOD.


  METHOD find_matching_end.
    DATA(lv_depth) = 1.
    DATA(lv_i) = iv_start + 1.
    WHILE lv_i <= lines( it_code ).
      READ TABLE it_code INDEX lv_i INTO DATA(ls_i).
      CASE ls_i-op.
        WHEN 2 OR 3 OR 4 OR 6. lv_depth = lv_depth + 1. " block/loop/if/try
        WHEN 11. " end
          lv_depth = lv_depth - 1.
          IF lv_depth = 0. rv = lv_i. RETURN. ENDIF.
      ENDCASE.
      lv_i = lv_i + 1.
    ENDWHILE.
    rv = lines( it_code ).
  ENDMETHOD.


  METHOD exit_or_return.
    " If inside a DO (from an if wrapper), use EXIT. Otherwise RETURN.
    LOOP AT mt_block_kinds INTO DATA(lv_k).
      IF lv_k = c_if. rv = 'EXIT.'. RETURN. ENDIF.
    ENDLOOP.
    rv = 'RETURN.'.
  ENDMETHOD.


  METHOD emit_br_propagation.
    " Consume one level. If more remain, EXIT/RETURN.
    DATA(lv_br) = mv_prefix && 'br'.
    DATA(lv_esc) = exit_or_return( ).
    line( |IF { lv_br } > 0. { lv_br } = { lv_br } - 1. IF { lv_br } > 0. { lv_esc } ENDIF. ENDIF.| ).
  ENDMETHOD.


  METHOD emit_br_propagate.
    " Don't consume a level (for after loop ENDDO).
    DATA(lv_br) = mv_prefix && 'br'.
    DATA(lv_esc) = exit_or_return( ).
    line( |IF { lv_br } > 0. { lv_esc } ENDIF.| ).
  ENDMETHOD.


  METHOD line.
    IF iv IS INITIAL. flush( ). emit_raw_line( iv ). RETURN. ENDIF.
    IF mv_pack_buf IS NOT INITIAL AND mv_indent <> mv_pack_indent. flush( ). ENDIF.
    DATA(lv_np) = abap_false.
    DATA(lv_c1) = iv(1).
    DATA(lv_len) = strlen( iv ).
    CASE lv_c1.
      WHEN 'D'. IF lv_len >= 3 AND ( iv(3) = 'DO ' OR iv(3) = 'DO.' ). lv_np = abap_true.
                ELSEIF lv_len >= 4 AND iv(4) = 'DATA'. lv_np = abap_true. ENDIF.
      WHEN 'E'. IF iv = 'ENDDO.' OR iv = 'ENDFORM.' OR iv = 'ELSE.' OR iv = 'ENDIF.'
                  OR iv = 'ENDCLASS.' OR iv = 'ENDMETHOD.'. lv_np = abap_true. ENDIF.
      WHEN 'F'. IF lv_len >= 5 AND iv(5) = 'FORM '. lv_np = abap_true. ENDIF.
      WHEN 'I'. IF lv_len >= 3 AND iv(3) = 'IF ' AND iv NS 'ENDIF'. lv_np = abap_true. ENDIF.
      WHEN 'C'. IF lv_len >= 6 AND iv(6) = 'CLASS '. lv_np = abap_true. ENDIF.
      WHEN 'R'. IF iv CS 'RETURN.'. lv_np = abap_true. ENDIF.
      WHEN 'M'. IF lv_len >= 7 AND ( iv(7) = 'METHOD ' OR iv(7) = 'METHODS' ). lv_np = abap_true. ENDIF.
    ENDCASE.
    IF lv_np = abap_true. flush( ). emit_raw_line( iv ). RETURN. ENDIF.
    DATA(lv_max) = 250 - mv_indent * 2.
    IF lv_max < 80. lv_max = 80. ENDIF.
    IF mv_pack_buf IS INITIAL. mv_pack_buf = iv. mv_pack_indent = mv_indent.
    ELSEIF strlen( mv_pack_buf ) + 1 + strlen( iv ) <= lv_max. mv_pack_buf = mv_pack_buf && ` ` && iv.
    ELSE. flush( ). mv_pack_buf = iv. mv_pack_indent = mv_indent. ENDIF.
  ENDMETHOD.


  METHOD emit_raw_line.
    DATA(lv_prefix) = ||.
    DO mv_indent TIMES. lv_prefix = lv_prefix && `  `. ENDDO.
    DATA(lv_full) = lv_prefix && iv.
    IF strlen( lv_full ) <= 255.
      mv_out = mv_out && lv_full && cl_abap_char_utilities=>newline.
    ELSE.
      DATA(lv_rest) = lv_full.
      WHILE strlen( lv_rest ) > 255.
        DATA(lv_cut) = 250.
        WHILE lv_cut > 40.
          IF lv_rest+lv_cut(1) = `.`. lv_cut = lv_cut + 1. EXIT. ENDIF.
          lv_cut = lv_cut - 1.
        ENDWHILE.
        IF lv_cut <= 40. lv_cut = 250. ENDIF.
        mv_out = mv_out && lv_rest(lv_cut) && cl_abap_char_utilities=>newline.
        lv_rest = lv_prefix && lv_rest+lv_cut.
        SHIFT lv_rest LEFT DELETING LEADING ` `.
        lv_rest = lv_prefix && lv_rest.
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
      CLEAR mv_pack_buf. mv_pack_indent = -1.
    ENDIF.
  ENDMETHOD.


  METHOD push.
    rv = |{ mv_prefix }s{ mv_stack_depth }|.
    mv_stack_depth = mv_stack_depth + 1.
    IF mv_stack_depth > mv_max_stack. mv_max_stack = mv_stack_depth. ENDIF.
  ENDMETHOD.


  METHOD pop.
    mv_stack_depth = mv_stack_depth - 1.
    IF mv_stack_depth < 0. mv_stack_depth = 0. ENDIF.
    rv = |{ mv_prefix }s{ mv_stack_depth }|.
  ENDMETHOD.


  METHOD peek.
    DATA(lv_idx) = mv_stack_depth - 1.
    IF lv_idx < 0. lv_idx = 0. ENDIF.
    rv = |{ mv_prefix }s{ lv_idx }|.
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
    rv = |{ mv_prefix }|.
    IF iv_idx < mv_num_params.
      rv = rv && |p{ iv_idx }|.
    ELSE.
      rv = rv && |l{ iv_idx }|.
    ENDIF.
  ENDMETHOD.


  METHOD valtype_abap.
    rv = 'i'.
  ENDMETHOD.


  METHOD emit_function.
    DATA(lv_name) = is_func-export_name.
    IF lv_name IS INITIAL. lv_name = |f{ is_func-index }|. ENDIF.
    TRANSLATE lv_name TO UPPER CASE.

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
        lv_sig = lv_sig && | p{ lv_pi } TYPE i|.
      ENDLOOP.
    ENDIF.
    mv_has_result = xsdbool( lines( ls_type-results ) > 0 ).
    IF mv_has_result = abap_true.
      lv_sig = lv_sig && | CHANGING rv TYPE i|.
    ENDIF.

    emit_raw_line( |{ lv_sig }.| ).
    mv_indent = mv_indent + 1.

    " Copy USING params to CLASS g
    mv_prefix = 'g=>'.
    LOOP AT ls_type-params INTO ls_p.
      lv_pi = sy-tabix - 1.
      line( |g=>p{ lv_pi } = p{ lv_pi }.| ).
    ENDLOOP.
    line( |g=>br = 0.| ).

    " Reset compiler state
    mv_stack_depth = 0.
    mv_max_stack = 0.
    mv_unreachable = abap_false.
    mv_dead_depth = 0.
    CLEAR mt_block_kinds.
    mv_in_block_method = abap_false.

    " Emit instructions
    emit_instructions( is_func-code ).
    flush( ).

    " Close any unclosed ifs (parser may exclude if's end from function body)
    WHILE lines( mt_block_kinds ) > 0.
      DELETE mt_block_kinds INDEX lines( mt_block_kinds ).
      mv_indent = mv_indent - 1.
      line( |ENDIF.| ).
      mv_indent = mv_indent - 1.
      line( |ENDDO.| ).
    ENDWHILE.

    " Return propagation from block methods
    IF mv_has_result = abap_true.
      line( |IF g=>br > 0. rv = g=>rv. RETURN. ENDIF.| ).
      IF mv_stack_depth > 0.
        line( |rv = { peek( ) }.| ).
      ENDIF.
    ELSE.
      line( |IF g=>br > 0. RETURN. ENDIF.| ).
    ENDIF.

    mv_indent = mv_indent - 1.
    line( |ENDFORM.| ).
    line( || ).
  ENDMETHOD.


  METHOD emit_instructions.
    DATA: lv_a TYPE string, lv_b TYPE string, lv_r TYPE string, lv_c TYPE string.
    DATA(lv_br) = mv_prefix && 'br'.
    DATA(lv_rv) = mv_prefix && 'rv'.
    DATA(lv_idx) = 1.

    WHILE lv_idx <= lines( it_code ).
      READ TABLE it_code INDEX lv_idx INTO DATA(ls_i).

      " No dead code elimination — br/return wrapped in IF 1=1 so GENERATE accepts code after them
      CASE ls_i-op.
        " --- Constants ---
        WHEN 65. line( |{ push( ) } = { ls_i-i32_value }.| ).
        WHEN 66.
          DATA(lv_i64) = ls_i-i64_value MOD 4294967296.
          IF lv_i64 < 0. lv_i64 = lv_i64 + 4294967296. ENDIF.
          IF lv_i64 > 2147483647. lv_i64 = lv_i64 - 4294967296. ENDIF.
          line( |{ push( ) } = { lv_i64 }.| ).

        " --- Local/Global ---
        WHEN 32. line( |{ push( ) } = { local_name( ls_i-local_idx ) }.| ).
        WHEN 33. line( |{ local_name( ls_i-local_idx ) } = { pop( ) }.| ).
        WHEN 34. line( |{ local_name( ls_i-local_idx ) } = { peek( ) }.| ).
        WHEN 35. line( |{ push( ) } = gv_g{ ls_i-global_idx }.| ).
        WHEN 36. line( |gv_g{ ls_i-global_idx } = { pop( ) }.| ).

        " --- i32 Arithmetic ---
        WHEN 106. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |{ lv_r } = { lv_a } + { lv_b }.| ).
        WHEN 107. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |{ lv_r } = { lv_a } - { lv_b }.| ).
        WHEN 108. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |{ lv_r } = { lv_a } * { lv_b }.| ).
        WHEN 109. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |{ lv_r } = { lv_a } / { lv_b }.| ).
        WHEN 111. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |{ lv_r } = { lv_a } MOD { lv_b }.| ).

        " --- Comparisons ---
        WHEN 69. lv_a = pop( ). lv_r = push( ). line( |IF { lv_a } = 0. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 70. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |IF { lv_a } = { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 71. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |IF { lv_a } <> { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 72. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |IF { lv_a } < { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 74. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |IF { lv_a } > { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 76. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |IF { lv_a } <= { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).
        WHEN 78. lv_b = pop( ). lv_a = pop( ). lv_r = push( ). line( |IF { lv_a } >= { lv_b }. { lv_r } = 1. ELSE. { lv_r } = 0. ENDIF.| ).

        " --- Bitwise ---
        WHEN 113. lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |gv_xa = { lv_a }. gv_xb = { lv_b }. gv_xr = gv_xa BIT-AND gv_xb. { lv_r } = gv_xr.| ).
        WHEN 114. lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |gv_xa = { lv_a }. gv_xb = { lv_b }. gv_xr = gv_xa BIT-OR gv_xb. { lv_r } = gv_xr.| ).
        WHEN 115. lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |gv_xa = { lv_a }. gv_xb = { lv_b }. gv_xr = gv_xa BIT-XOR gv_xb. { lv_r } = gv_xr.| ).
        WHEN 116. lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |TRY. { lv_r } = { lv_a } * ipow( base = 2 exp = { lv_b } MOD 32 ). CATCH cx_root. { lv_r } = 0. ENDTRY.| ).
        WHEN 117. lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |TRY. { lv_r } = { lv_a } / ipow( base = 2 exp = { lv_b } MOD 32 ). CATCH cx_root. { lv_r } = 0. ENDTRY.| ).

        " --- Memory ---
        WHEN 40. lv_a = pop( ). lv_r = push( ).
          IF ls_i-offset > 0. line( |{ lv_r } = { lv_a } + { ls_i-offset }.| ).
            line( |PERFORM mem_ld_i32 USING { lv_r } CHANGING { lv_r }.| ).
          ELSE. line( |PERFORM mem_ld_i32 USING { lv_a } CHANGING { lv_r }.| ). ENDIF.
        WHEN 54. lv_b = pop( ). lv_a = pop( ).
          IF ls_i-offset > 0. line( |{ lv_a } = { lv_a } + { ls_i-offset }.| ). ENDIF.
          line( |PERFORM mem_st_i32 USING { lv_a } { lv_b }.| ).
        WHEN 44. lv_a = pop( ). lv_r = push( ).
          IF ls_i-offset > 0. line( |{ lv_r } = { lv_a } + { ls_i-offset }.| ).
            line( |PERFORM mem_ld_i32_8u USING { lv_r } CHANGING { lv_r }.| ).
          ELSE. line( |PERFORM mem_ld_i32_8u USING { lv_a } CHANGING { lv_r }.| ). ENDIF.
        WHEN 58. lv_b = pop( ). lv_a = pop( ).
          IF ls_i-offset > 0. line( |{ lv_a } = { lv_a } + { ls_i-offset }.| ). ENDIF.
          line( |PERFORM mem_st_i32_8 USING { lv_a } { lv_b }.| ).

        " === CONTROL FLOW (CLASS g block-as-method) ===

        WHEN 2. " block → extract to CLASS-METHOD
          DATA(lv_saved_depth) = mv_stack_depth.
          DATA(lv_end_idx) = find_matching_end( it_code = it_code iv_start = lv_idx ).
          " Extract body
          DATA(lt_body) = VALUE zcl_wasm_module=>ty_instructions( ).
          DATA(lv_bi) = lv_idx + 1.
          WHILE lv_bi < lv_end_idx.
            READ TABLE it_code INDEX lv_bi INTO DATA(ls_bi).
            APPEND ls_bi TO lt_body.
            lv_bi = lv_bi + 1.
          ENDWHILE.
          DATA(lv_bname) = |f{ mv_func_idx }_b{ mv_block_counter }|.
          mv_block_counter = mv_block_counter + 1.
          " Generate block body — save ALL compiler state including packer
          DATA(lv_saved_out) = mv_out. CLEAR mv_out.
          DATA(lv_saved_bk) = mt_block_kinds. CLEAR mt_block_kinds.
          DATA(lv_saved_ibm) = mv_in_block_method.
          DATA(lv_saved_pfx) = mv_prefix.
          DATA(lv_saved_indent) = mv_indent.
          DATA(lv_saved_sd) = mv_stack_depth.
          DATA(lv_saved_unr) = mv_unreachable.
          DATA(lv_saved_pack) = mv_pack_buf. CLEAR mv_pack_buf.
          DATA(lv_saved_pi) = mv_pack_indent. mv_pack_indent = -1.
          mv_in_block_method = abap_true.
          mv_prefix = ''. " inside CLASS-METHOD, no prefix
          mv_indent = 2.
          mv_unreachable = abap_false.
          emit_instructions( lt_body ).
          flush( ).
          " Close any unclosed ifs (parser may exclude if's end from block body)
          WHILE lines( mt_block_kinds ) > 0.
            DELETE mt_block_kinds INDEX lines( mt_block_kinds ).
            mv_indent = mv_indent - 1.
            line( |ENDIF.| ).
            mv_indent = mv_indent - 1.
            line( |ENDDO.| ).
          ENDWHILE.
          APPEND VALUE ty_block_method( name = lv_bname body = mv_out ) TO mt_block_methods.
          " Restore ALL state
          mv_out = lv_saved_out. mt_block_kinds = lv_saved_bk.
          mv_in_block_method = lv_saved_ibm. mv_prefix = lv_saved_pfx.
          mv_indent = lv_saved_indent. mv_stack_depth = lv_saved_sd.
          mv_unreachable = lv_saved_unr.
          mv_pack_buf = lv_saved_pack. mv_pack_indent = lv_saved_pi.
          " Emit call
          DATA(lv_call_pfx) = mv_prefix. " g=> from FORM, empty from class method
          line( |{ lv_call_pfx }{ lv_bname }( ).| ).
          emit_br_propagation( ).
          " Adjust stack for block result type
          IF ls_i-block_type >= 0 AND ls_i-block_type <> 64.
            mv_stack_depth = lv_saved_depth + 1.
          ELSE.
            mv_stack_depth = lv_saved_depth.
          ENDIF.
          lv_idx = lv_end_idx + 1. CONTINUE.

        WHEN 3. " loop → extract to CLASS-METHOD, call in DO
          lv_saved_depth = mv_stack_depth.
          lv_end_idx = find_matching_end( it_code = it_code iv_start = lv_idx ).
          CLEAR lt_body.
          lv_bi = lv_idx + 1.
          WHILE lv_bi < lv_end_idx.
            READ TABLE it_code INDEX lv_bi INTO ls_bi.
            APPEND ls_bi TO lt_body.
            lv_bi = lv_bi + 1.
          ENDWHILE.
          lv_bname = |f{ mv_func_idx }_l{ mv_block_counter }|.
          mv_block_counter = mv_block_counter + 1.
          " Generate loop body — save ALL compiler state including packer
          lv_saved_out = mv_out. CLEAR mv_out.
          lv_saved_bk = mt_block_kinds. CLEAR mt_block_kinds.
          lv_saved_ibm = mv_in_block_method.
          lv_saved_pfx = mv_prefix.
          lv_saved_indent = mv_indent.
          lv_saved_sd = mv_stack_depth.
          lv_saved_unr = mv_unreachable.
          lv_saved_pack = mv_pack_buf. CLEAR mv_pack_buf.
          lv_saved_pi = mv_pack_indent. mv_pack_indent = -1.
          mv_in_block_method = abap_true.
          mv_prefix = ''.
          mv_indent = 2.
          mv_unreachable = abap_false.
          emit_instructions( lt_body ).
          flush( ).
          " Close any unclosed ifs
          WHILE lines( mt_block_kinds ) > 0.
            DELETE mt_block_kinds INDEX lines( mt_block_kinds ).
            mv_indent = mv_indent - 1.
            line( |ENDIF.| ).
            mv_indent = mv_indent - 1.
            line( |ENDDO.| ).
          ENDWHILE.
          APPEND VALUE ty_block_method( name = lv_bname body = mv_out ) TO mt_block_methods.
          mv_out = lv_saved_out. mt_block_kinds = lv_saved_bk.
          mv_in_block_method = lv_saved_ibm. mv_prefix = lv_saved_pfx.
          mv_indent = lv_saved_indent. mv_stack_depth = lv_saved_sd.
          mv_unreachable = lv_saved_unr.
          mv_pack_buf = lv_saved_pack. mv_pack_indent = lv_saved_pi.
          " Emit DO + call
          lv_call_pfx = mv_prefix.
          line( |DO.| ).
          mv_indent = mv_indent + 1.
          line( |{ lv_br } = 0.| ).
          line( |{ lv_call_pfx }{ lv_bname }( ).| ).
          line( |IF { lv_br } = 0. EXIT. ENDIF.| ).
          line( |{ lv_br } = { lv_br } - 1.| ).
          line( |IF { lv_br } > 0. EXIT. ENDIF.| ).
          mv_indent = mv_indent - 1.
          line( |ENDDO.| ).
          emit_br_propagate( ).
          mv_stack_depth = lv_saved_depth.
          lv_idx = lv_end_idx + 1. CONTINUE.

        WHEN 4. " if → DO 1 TIMES wrapper
          lv_c = pop( ).
          flush( ).
          emit_raw_line( |DO 1 TIMES.| ).
          mv_indent = mv_indent + 1.
          emit_raw_line( |IF { lv_c } <> 0.| ).
          mv_indent = mv_indent + 1.
          APPEND c_if TO mt_block_kinds.

        WHEN 5. " else — only emit if there's a matching if in mt_block_kinds
          IF lines( mt_block_kinds ) > 0.
            mv_indent = mv_indent - 1.
            line( |ELSE.| ).
            mv_indent = mv_indent + 1.
          ENDIF.

        WHEN 11. " end (only for ifs now — blocks/loops are extracted)
          IF lines( mt_block_kinds ) > 0.
            DATA(lv_kind) = mt_block_kinds[ lines( mt_block_kinds ) ].
            DELETE mt_block_kinds INDEX lines( mt_block_kinds ).
            IF lv_kind = c_if.
              mv_indent = mv_indent - 1.
              line( |ENDIF.| ).
              mv_indent = mv_indent - 1.
              line( |ENDDO.| ).
              emit_br_propagation( ).
            ENDIF.
          ENDIF.

        WHEN 12. " br — 1-based
          DATA(lv_esc) = exit_or_return( ).
          line( |IF 1 = 1. { lv_br } = { ls_i-label_idx + 1 }. { lv_esc } ENDIF.| ).

        WHEN 13. " br_if — 1-based
          lv_c = pop( ).
          lv_esc = exit_or_return( ).
          line( |IF { lv_c } <> 0. { lv_br } = { ls_i-label_idx + 1 }. { lv_esc } ENDIF.| ).

        WHEN 14. " br_table — simplified: use default label
          lv_c = pop( ).
          lv_esc = exit_or_return( ).
          line( |IF 1 = 1. { lv_br } = { ls_i-label_idx + 1 }. { lv_esc } ENDIF.| ).

        WHEN 15. " return — wrap in IF 1=1 so GENERATE accepts code after
          IF mv_has_result = abap_true AND mv_stack_depth > 0.
            line( |IF 1 = 1. { lv_rv } = { pop( ) }. { lv_br } = 999. RETURN. ENDIF.| ).
          ELSE.
            line( |IF 1 = 1. { lv_br } = 999. RETURN. ENDIF.| ).
          ENDIF.

        WHEN 17. " call_indirect — stub
          pop( ).
          READ TABLE mo_mod->mt_types INDEX ls_i-type_idx + 1 INTO DATA(ls_ci_type).
          IF sy-subrc = 0.
            DO lines( ls_ci_type-params ) TIMES. pop( ). ENDDO.
            IF lines( ls_ci_type-results ) > 0. line( |{ push( ) } = 0.| ). ENDIF.
          ENDIF.

        " --- Call ---
        WHEN 16. emit_call( ls_i-func_idx ).

        " --- Stack ---
        WHEN 26. pop( ).
        WHEN 27. lv_c = pop( ). lv_b = pop( ). lv_a = pop( ). lv_r = push( ).
          line( |IF { lv_c } <> 0. { lv_r } = { lv_a }. ELSE. { lv_r } = { lv_b }. ENDIF.| ).

        " --- Nop / Unreachable ---
        WHEN 0. line( |IF 1 = 1. { lv_br } = 999. RETURN. ENDIF.| ).
        WHEN 1. " nop
      ENDCASE.

      lv_idx = lv_idx + 1.
    ENDWHILE.
  ENDMETHOD.


  METHOD emit_call.
    DATA(lv_fname) = func_name( iv_func_idx ).
    DATA(lv_fi) = iv_func_idx - mo_mod->mv_num_imported_funcs.
    DATA ls_type TYPE zcl_wasm_module=>ty_functype.
    IF lv_fi < 0.
      READ TABLE mo_mod->mt_imports WITH KEY func_index = iv_func_idx INTO DATA(ls_imp).
      IF sy-subrc = 0. READ TABLE mo_mod->mt_types INDEX ls_imp-type_index + 1 INTO ls_type. ENDIF.
    ELSE.
      READ TABLE mo_mod->mt_functions INDEX lv_fi + 1 INTO DATA(ls_f).
      READ TABLE mo_mod->mt_types INDEX ls_f-type_index + 1 INTO ls_type.
    ENDIF.

    " Pop arguments in reverse
    DATA(lv_np) = lines( ls_type-params ).
    DATA lt_args TYPE STANDARD TABLE OF string WITH DEFAULT KEY.
    DO lv_np TIMES. INSERT pop( ) INTO lt_args INDEX 1. ENDDO.

    " Save shared g=> vars before call (recursion safety)
    DATA(lv_br) = mv_prefix && 'br'.
    DATA(lv_save_depth) = mv_stack_depth.
    DATA(lv_si) = 0.
    WHILE lv_si < lv_save_depth.
      line( |APPEND { mv_prefix }s{ lv_si } TO gt_stk.| ).
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
    IF lv_has_result = abap_true. lv_result = push( ). ENDIF.

    DATA(lv_call) = |PERFORM { lv_fname }|.
    IF lines( lt_args ) > 0.
      lv_call = lv_call && | USING|.
      LOOP AT lt_args INTO DATA(lv_arg). lv_call = lv_call && | { lv_arg }|. ENDLOOP.
    ENDIF.
    IF lv_has_result = abap_true. lv_call = lv_call && | CHANGING { lv_result }|. ENDIF.
    line( |{ lv_call }.| ).

    " Restore shared g=> vars after call (reverse order)
    lv_si = mv_num_params + mv_num_locals - 1.
    WHILE lv_si >= mv_num_params.
      line( |{ local_name( lv_si ) } = gt_stk[ lines( gt_stk ) ]. DELETE gt_stk INDEX lines( gt_stk ).| ).
      lv_si = lv_si - 1.
    ENDWHILE.
    lv_si = lv_save_depth - 1.
    WHILE lv_si >= 0.
      line( |{ mv_prefix }s{ lv_si } = gt_stk[ lines( gt_stk ) ]. DELETE gt_stk INDEX lines( gt_stk ).| ).
      lv_si = lv_si - 1.
    ENDWHILE.
  ENDMETHOD.


  METHOD split_to_includes.
    " Keep existing implementation — no changes needed
    rt = VALUE #( ).
  ENDMETHOD.


  METHOD compile_class.
    " Keep existing implementation — delegate to compile + GENERATE
    rv = ''.
  ENDMETHOD.

ENDCLASS.
