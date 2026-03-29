CLASS zcl_wasm_module DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    " Function type (signature)
    TYPES: BEGIN OF ty_valtype,
             type TYPE i,  " x'7F'=i32, x'7E'=i64, x'7D'=f32, x'7C'=f64
           END OF ty_valtype,
           ty_valtypes TYPE STANDARD TABLE OF ty_valtype WITH DEFAULT KEY.
    TYPES: BEGIN OF ty_functype,
             params  TYPE ty_valtypes,
             results TYPE ty_valtypes,
           END OF ty_functype.

    " Instruction
    TYPES: BEGIN OF ty_instruction,
             op          TYPE i,
             i32_value   TYPE i,
             i64_value   TYPE int8,
             local_idx   TYPE i,
             global_idx  TYPE i,
             func_idx    TYPE i,
             type_idx    TYPE i,
             label_idx   TYPE i,
             offset      TYPE i,
             align       TYPE i,
             block_type  TYPE i,
             misc_op     TYPE i,
           END OF ty_instruction,
           ty_instructions TYPE STANDARD TABLE OF ty_instruction WITH DEFAULT KEY.

    " Function
    TYPES: BEGIN OF ty_function,
             index       TYPE i,
             type_index  TYPE i,
             export_name TYPE string,
             locals      TYPE ty_valtypes,
             code        TYPE ty_instructions,
           END OF ty_function.

    " Import
    TYPES: BEGIN OF ty_import,
             module     TYPE string,
             name       TYPE string,
             kind       TYPE i,  " 0=func
             type_index TYPE i,
             func_index TYPE i,
           END OF ty_import.

    " Export
    TYPES: BEGIN OF ty_export,
             name  TYPE string,
             kind  TYPE i,  " 0=func, 1=table, 2=memory, 3=global
             index TYPE i,
           END OF ty_export.

    " Global
    TYPES: BEGIN OF ty_global,
             type    TYPE i,
             mutable TYPE abap_bool,
             init_i32 TYPE i,
             init_i64 TYPE int8,
           END OF ty_global.

    " Data segment
    TYPES: BEGIN OF ty_data_segment,
             offset TYPE i,
             data   TYPE xstring,
           END OF ty_data_segment.

    " Element segment
    TYPES: BEGIN OF ty_element,
             offset       TYPE i,
             func_indices TYPE STANDARD TABLE OF i WITH DEFAULT KEY,
           END OF ty_element.

    " Memory
    TYPES: BEGIN OF ty_memory,
             min_pages TYPE i,
             max_pages TYPE i,
           END OF ty_memory.

    " Module data
    DATA mt_types     TYPE STANDARD TABLE OF ty_functype WITH DEFAULT KEY.
    DATA mt_functions TYPE STANDARD TABLE OF ty_function WITH DEFAULT KEY.
    DATA mt_imports   TYPE STANDARD TABLE OF ty_import WITH DEFAULT KEY.
    DATA mt_exports   TYPE STANDARD TABLE OF ty_export WITH DEFAULT KEY.
    DATA mt_globals   TYPE STANDARD TABLE OF ty_global WITH DEFAULT KEY.
    DATA mt_data      TYPE STANDARD TABLE OF ty_data_segment WITH DEFAULT KEY.
    DATA mt_elements  TYPE STANDARD TABLE OF ty_element WITH DEFAULT KEY.
    DATA ms_memory    TYPE ty_memory.
    DATA mv_num_imported_funcs TYPE i.
    DATA mv_start_func TYPE i VALUE -1.
    DATA mv_parse_error TYPE string.

    " Parse WASM binary
    METHODS parse IMPORTING iv_wasm TYPE xstring.

    " Get human-readable summary
    METHODS summary RETURNING VALUE(rv) TYPE string.

  PRIVATE SECTION.
    DATA mo_reader TYPE REF TO zcl_wasm_reader.
    METHODS parse_type_section IMPORTING iv_end TYPE i.
    METHODS parse_import_section IMPORTING iv_end TYPE i.
    METHODS parse_function_section IMPORTING iv_end TYPE i.
    METHODS parse_memory_section IMPORTING iv_end TYPE i.
    METHODS parse_global_section IMPORTING iv_end TYPE i.
    METHODS parse_export_section IMPORTING iv_end TYPE i.
    METHODS parse_element_section IMPORTING iv_end TYPE i.
    METHODS parse_code_section IMPORTING iv_end TYPE i.
    METHODS parse_data_section IMPORTING iv_end TYPE i.
    METHODS parse_instructions IMPORTING iv_end TYPE i RETURNING VALUE(rt) TYPE ty_instructions.
ENDCLASS.

CLASS zcl_wasm_module IMPLEMENTATION.
  METHOD parse.
    mo_reader = NEW zcl_wasm_reader( iv_wasm ).
    " Magic number
    DATA(lv_magic) = mo_reader->read_bytes( 4 ).
    " Version
    DATA(lv_version) = mo_reader->read_u32_fixed( ).
    " Sections
    DATA lv_parse_err TYPE string.
    WHILE mo_reader->eof( ) = abap_false.
      TRY.
          DATA(lv_section_id) = mo_reader->read_byte( ).
          DATA(lv_section_len) = mo_reader->read_u32( ).
          DATA(lv_section_end) = mo_reader->get_pos( ) + lv_section_len.
          CASE lv_section_id.
            WHEN 1. parse_type_section( lv_section_end ).
            WHEN 2. parse_import_section( lv_section_end ).
            WHEN 3. parse_function_section( lv_section_end ).
            WHEN 5. parse_memory_section( lv_section_end ).
            WHEN 6. parse_global_section( lv_section_end ).
            WHEN 7. parse_export_section( lv_section_end ).
            WHEN 8. mv_start_func = mo_reader->read_u32( ).
            WHEN 9. parse_element_section( lv_section_end ).
            WHEN 10. parse_code_section( lv_section_end ).
            WHEN 11. parse_data_section( lv_section_end ).
            WHEN OTHERS. mo_reader->set_pos( lv_section_end ).
          ENDCASE.
          IF mo_reader->get_pos( ) <> lv_section_end. mo_reader->set_pos( lv_section_end ). ENDIF.
        CATCH cx_root INTO DATA(lx_sec).
          lv_parse_err = |SECTION { lv_section_id } FAILED pos={ mo_reader->get_pos( ) }/{ mo_reader->remaining( ) }: { lx_sec->get_text( ) }|.
          EXIT. " break WHILE, continue with what we have
      ENDTRY.
    ENDWHILE.
    IF lv_parse_err IS NOT INITIAL.
      mv_parse_error = lv_parse_err.
    ENDIF.
    " Assign export names
    LOOP AT mt_exports INTO DATA(ls_exp) WHERE kind = 0.
      DATA(lv_fi) = ls_exp-index - mv_num_imported_funcs.
      IF lv_fi >= 0 AND lv_fi < lines( mt_functions ).
        FIELD-SYMBOLS <func> TYPE ty_function.
        READ TABLE mt_functions INDEX lv_fi + 1 ASSIGNING <func>.
        IF sy-subrc = 0. <func>-export_name = ls_exp-name. ENDIF.
      ENDIF.
    ENDLOOP.
  ENDMETHOD.

  METHOD parse_type_section.
    DATA(lv_count) = mo_reader->read_u32( ).
    DO lv_count TIMES.
      mo_reader->read_byte( ). " 0x60 = functype
      DATA ls_ft TYPE ty_functype.
      CLEAR ls_ft.
      DATA(lv_pc) = mo_reader->read_u32( ).
      DO lv_pc TIMES. APPEND VALUE ty_valtype( type = mo_reader->read_byte( ) ) TO ls_ft-params. ENDDO.
      DATA(lv_rc) = mo_reader->read_u32( ).
      DO lv_rc TIMES. APPEND VALUE ty_valtype( type = mo_reader->read_byte( ) ) TO ls_ft-results. ENDDO.
      APPEND ls_ft TO mt_types.
    ENDDO.
  ENDMETHOD.

  METHOD parse_import_section.
    DATA(lv_count) = mo_reader->read_u32( ).
    DO lv_count TIMES.
      DATA ls_imp TYPE ty_import. CLEAR ls_imp.
      ls_imp-module = mo_reader->read_string( ). ls_imp-name = mo_reader->read_string( ).
      ls_imp-kind = mo_reader->read_byte( ).
      CASE ls_imp-kind.
        WHEN 0. ls_imp-type_index = mo_reader->read_u32( ). ls_imp-func_index = mv_num_imported_funcs. mv_num_imported_funcs = mv_num_imported_funcs + 1.
        WHEN 1. mo_reader->read_byte( ). mo_reader->read_u32( ). DATA(lv_has_max) = mo_reader->read_byte( ). IF lv_has_max = 1. mo_reader->read_u32( ). ENDIF.
        WHEN 2. DATA(lv_hm2) = mo_reader->read_byte( ). ms_memory-min_pages = mo_reader->read_u32( ). IF lv_hm2 = 1. ms_memory-max_pages = mo_reader->read_u32( ). ENDIF.
        WHEN 3. mo_reader->read_byte( ). mo_reader->read_byte( ).
      ENDCASE.
      APPEND ls_imp TO mt_imports.
    ENDDO.
  ENDMETHOD.

  METHOD parse_function_section.
    DATA(lv_count) = mo_reader->read_u32( ).
    DO lv_count TIMES.
      DATA(lv_type_idx) = mo_reader->read_u32( ).
      APPEND VALUE ty_function( index = mv_num_imported_funcs + sy-index - 1 type_index = lv_type_idx ) TO mt_functions.
    ENDDO.
  ENDMETHOD.

  METHOD parse_memory_section.
    DATA(lv_count) = mo_reader->read_u32( ).
    IF lv_count > 0.
      DATA(lv_hm) = mo_reader->read_byte( ). ms_memory-min_pages = mo_reader->read_u32( ).
      IF lv_hm = 1. ms_memory-max_pages = mo_reader->read_u32( ). ENDIF.
    ENDIF.
    mo_reader->set_pos( iv_end ).
  ENDMETHOD.

  METHOD parse_global_section.
    DATA(lv_count) = mo_reader->read_u32( ).
    DO lv_count TIMES.
      DATA ls_g TYPE ty_global. CLEAR ls_g.
      ls_g-type = mo_reader->read_byte( ). ls_g-mutable = xsdbool( mo_reader->read_byte( ) = 1 ).
      DATA(lv_init_op) = mo_reader->read_byte( ).
      CASE lv_init_op.
        WHEN 65. ls_g-init_i32 = mo_reader->read_i32( ). " i32.const
        WHEN 66. ls_g-init_i64 = mo_reader->read_i64( ). " i64.const
        WHEN 35. mo_reader->read_u32( ). " global.get
      ENDCASE.
      mo_reader->read_byte( ). " end
      APPEND ls_g TO mt_globals.
    ENDDO.
  ENDMETHOD.

  METHOD parse_export_section.
    DATA(lv_count) = mo_reader->read_u32( ).
    DO lv_count TIMES.
      DATA ls_e TYPE ty_export. CLEAR ls_e.
      ls_e-name = mo_reader->read_string( ). ls_e-kind = mo_reader->read_byte( ). ls_e-index = mo_reader->read_u32( ).
      APPEND ls_e TO mt_exports.
    ENDDO.
  ENDMETHOD.

  METHOD parse_element_section.
    DATA(lv_count) = mo_reader->read_u32( ).
    DO lv_count TIMES.
      DATA(lv_flags) = mo_reader->read_u32( ).
      IF lv_flags = 0.
        DATA ls_el TYPE ty_element. CLEAR ls_el.
        mo_reader->read_byte( ). ls_el-offset = mo_reader->read_i32( ). mo_reader->read_byte( ).
        DATA(lv_fc) = mo_reader->read_u32( ).
        DO lv_fc TIMES. APPEND mo_reader->read_u32( ) TO ls_el-func_indices. ENDDO.
        APPEND ls_el TO mt_elements.
      ELSE.
        mo_reader->set_pos( iv_end ). RETURN.
      ENDIF.
    ENDDO.
  ENDMETHOD.

  METHOD parse_code_section.
    DATA(lv_count) = mo_reader->read_u32( ).
    DO lv_count TIMES.
      DATA(lv_idx) = sy-index.
      IF lv_idx > lines( mt_functions ). EXIT. ENDIF.
      DATA(lv_body_size) = mo_reader->read_u32( ).
      DATA(lv_body_end) = mo_reader->get_pos( ) + lv_body_size.
      TRY.
          " Locals
          DATA(lv_local_decl_count) = mo_reader->read_u32( ).
          FIELD-SYMBOLS <func> TYPE ty_function.
          READ TABLE mt_functions INDEX lv_idx ASSIGNING <func>.
          DO lv_local_decl_count TIMES.
            DATA(lv_lc) = mo_reader->read_u32( ). DATA(lv_lt) = mo_reader->read_byte( ).
            DO lv_lc TIMES. APPEND VALUE ty_valtype( type = lv_lt ) TO <func>-locals. ENDDO.
          ENDDO.
          " Instructions
          <func>-code = parse_instructions( lv_body_end ).
        CATCH cx_root INTO DATA(lx_parse).
          IF mv_parse_error IS INITIAL.
            mv_parse_error = |func { lv_idx }: { lx_parse->get_text( ) } at pos { mo_reader->get_pos( ) }|.
          ENDIF.
      ENDTRY.
      mo_reader->set_pos( lv_body_end ).
    ENDDO.
  ENDMETHOD.

  METHOD parse_data_section.
    DATA(lv_count) = mo_reader->read_u32( ).
    DO lv_count TIMES.
      DATA(lv_flags) = mo_reader->read_u32( ).
      DATA ls_d TYPE ty_data_segment. CLEAR ls_d.
      IF lv_flags = 0.
        mo_reader->read_byte( ). ls_d-offset = mo_reader->read_i32( ). mo_reader->read_byte( ).
        DATA(lv_dl) = mo_reader->read_u32( ). ls_d-data = mo_reader->read_bytes( lv_dl ).
        APPEND ls_d TO mt_data.
      ELSEIF lv_flags = 1.
        DATA(lv_dl2) = mo_reader->read_u32( ). ls_d-data = mo_reader->read_bytes( lv_dl2 ).
        APPEND ls_d TO mt_data.
      ELSE.
        mo_reader->set_pos( iv_end ). RETURN.
      ENDIF.
    ENDDO.
  ENDMETHOD.

  METHOD parse_instructions.
    DATA ls_i TYPE ty_instruction.
    WHILE mo_reader->get_pos( ) < iv_end.
      CLEAR ls_i.
      ls_i-op = mo_reader->read_byte( ).
      CASE ls_i-op.
        WHEN 2 OR 3 OR 4. ls_i-block_type = mo_reader->read_i32( ). " block/loop/if
        WHEN 12 OR 13. ls_i-label_idx = mo_reader->read_u32( ). " br/br_if
        WHEN 14. " br_table
          DATA(lv_bt_cnt) = mo_reader->read_u32( ).
          DO lv_bt_cnt TIMES. mo_reader->read_u32( ). ENDDO.
          ls_i-label_idx = mo_reader->read_u32( ). " default
        WHEN 16 OR 18. ls_i-func_idx = mo_reader->read_u32( ). " call / return_call
        WHEN 17 OR 19. ls_i-type_idx = mo_reader->read_u32( ). mo_reader->read_u32( ). " call_indirect / return_call_indirect
        WHEN 28. " select_t
          DATA(lv_st_cnt) = mo_reader->read_u32( ).
          DO lv_st_cnt TIMES. mo_reader->read_byte( ). ENDDO.
        WHEN 32 OR 33 OR 34. ls_i-local_idx = mo_reader->read_u32( ). " local.get/set/tee
        WHEN 35 OR 36. ls_i-global_idx = mo_reader->read_u32( ). " global.get/set
        WHEN 40 OR 41 OR 42 OR 43 OR 44 OR 45 OR 46 OR 47 OR 48 OR 49 OR 50 OR 51 OR 52 OR 53 OR 54 OR 55 OR 56 OR 57 OR 58 OR 59 OR 60 OR 61 OR 62.
          ls_i-align = mo_reader->read_u32( ). ls_i-offset = mo_reader->read_u32( ). " memory ops
        WHEN 63 OR 64. mo_reader->read_byte( ). " memory.size/grow
        WHEN 65. ls_i-i32_value = mo_reader->read_i32( ). " i32.const
        WHEN 66. ls_i-i64_value = mo_reader->read_i64( ). " i64.const
        WHEN 67. mo_reader->read_bytes( 4 ). " f32.const
        WHEN 68. mo_reader->read_bytes( 8 ). " f64.const
        WHEN 252. " 0xFC misc prefix
          ls_i-misc_op = mo_reader->read_u32( ).
          CASE ls_i-misc_op.
            WHEN 0 OR 1 OR 2 OR 3 OR 4 OR 5 OR 6 OR 7. " trunc_sat — no extra operands
            WHEN 8. mo_reader->read_u32( ). mo_reader->read_byte( ). " memory.init
            WHEN 9. mo_reader->read_u32( ). " data.drop
            WHEN 10. mo_reader->read_byte( ). mo_reader->read_byte( ). " memory.copy
            WHEN 11. mo_reader->read_byte( ). " memory.fill
            WHEN 12. mo_reader->read_u32( ). mo_reader->read_byte( ). " table.init
            WHEN 13. mo_reader->read_u32( ). " elem.drop
            WHEN 14. mo_reader->read_u32( ). mo_reader->read_u32( ). " table.copy
            WHEN 15 OR 16 OR 17. mo_reader->read_u32( ). " table.grow/size/fill
          ENDCASE.
        WHEN 208. mo_reader->read_byte( ). " ref.null (type)
        WHEN 210. mo_reader->read_u32( ). " ref.func (func_idx)
        WHEN 253. " 0xFD SIMD prefix — skip
          DATA(lv_simd_op) = mo_reader->read_u32( ).
          IF lv_simd_op <= 11. mo_reader->read_u32( ). mo_reader->read_u32( ). ENDIF.
          IF lv_simd_op = 12. mo_reader->read_bytes( 16 ). ENDIF. " v128.const
          IF lv_simd_op = 13. mo_reader->read_bytes( 16 ). ENDIF. " shuffle
        WHEN OTHERS.
          " Opcodes 0-1, 5, 11, 15, 26-27, 69-192, 209: no operands — safe to skip
          " If an unknown opcode WITH operands appears, parsing will desync
          IF ls_i-op > 192 AND ls_i-op < 208.
            " Unknown prefix range — abort this function
            IF mv_parse_error IS INITIAL.
              mv_parse_error = |unknown opcode { ls_i-op } at pos { mo_reader->get_pos( ) }|.
            ENDIF.
            RETURN.
          ENDIF.
      ENDCASE.
      APPEND ls_i TO rt.
    ENDWHILE.
  ENDMETHOD.

  METHOD summary.
    rv = |Types: { lines( mt_types ) }, | &&
         |Imports: { lines( mt_imports ) } ({ mv_num_imported_funcs } funcs), | &&
         |Functions: { lines( mt_functions ) }, | &&
         |Exports: { lines( mt_exports ) }, | &&
         |Globals: { lines( mt_globals ) }, | &&
         |Data: { lines( mt_data ) }, | &&
         |Elements: { lines( mt_elements ) }, | &&
         |Memory: { ms_memory-min_pages } pages|.
    DATA(lv_total_instrs) = 0.
    LOOP AT mt_functions INTO DATA(ls_f). lv_total_instrs = lv_total_instrs + lines( ls_f-code ). ENDLOOP.
    rv = rv && |, Instructions: { lv_total_instrs }|.
    IF mv_parse_error IS NOT INITIAL.
      rv = rv && |, ERROR: { mv_parse_error }|.
    ENDIF.
  ENDMETHOD.
ENDCLASS.
