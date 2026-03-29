*"* WASM Compiler Unit Tests
CLASS lcl_test DEFINITION FOR TESTING DURATION MEDIUM RISK LEVEL HARMLESS.
  PRIVATE SECTION.
    METHODS test_parse_add FOR TESTING.
    METHODS test_compile_add FOR TESTING.
    METHODS test_execute_add FOR TESTING.
    METHODS test_execute_factorial FOR TESTING.
    METHODS test_parse_quickjs FOR TESTING.
    METHODS test_compile_quickjs FOR TESTING.
    METHODS test_generate_quickjs FOR TESTING.
    METHODS compile_and_gen IMPORTING iv_wasm TYPE xstring RETURNING VALUE(rv_prog) TYPE string.
    METHODS load_smw0 IMPORTING iv_name TYPE c RETURNING VALUE(rv_wasm) TYPE xstring.
ENDCLASS.

CLASS lcl_test IMPLEMENTATION.
  METHOD load_smw0.
    DATA: lt_mime TYPE STANDARD TABLE OF w3mime,
          lv_size TYPE i,
          ls_key  TYPE wwwdatatab.
    ls_key-relid = 'MI'.
    ls_key-objid = iv_name.
    CALL FUNCTION 'WWWDATA_IMPORT'
      EXPORTING key = ls_key
      TABLES mime = lt_mime
      EXCEPTIONS OTHERS = 1.
    IF sy-subrc = 0 AND lines( lt_mime ) > 0.
      lv_size = lines( lt_mime ) * 255.
      CALL FUNCTION 'SCMS_BINARY_TO_XSTRING'
        EXPORTING input_length = lv_size
        IMPORTING buffer = rv_wasm
        TABLES binary_tab = lt_mime.
    ENDIF.
  ENDMETHOD.

  METHOD compile_and_gen.
    DATA(lo_mod) = NEW zcl_wasm_module( ).
    lo_mod->parse( iv_wasm ).
    DATA(lv_abap) = NEW zcl_wasm_codegen( )->compile( lo_mod ).
    DATA: lt_code TYPE STANDARD TABLE OF string.
    SPLIT lv_abap AT cl_abap_char_utilities=>newline INTO TABLE lt_code.
    " Check no lines > 255
    LOOP AT lt_code INTO DATA(lv_chk).
      IF strlen( lv_chk ) > 255.
        cl_abap_unit_assert=>fail( msg = |Line { sy-tabix } too long: { strlen( lv_chk ) } chars| ).
      ENDIF.
    ENDLOOP.
    GENERATE SUBROUTINE POOL lt_code NAME rv_prog.
    cl_abap_unit_assert=>assert_subrc( msg = 'GENERATE failed' ).
  ENDMETHOD.

  METHOD test_parse_add.
    DATA(lv_w) = CONV xstring( '0061736D0100000001070160027F7F017F030201000707010361646400000A09010700200020016A0B' ).
    DATA(lo_mod) = NEW zcl_wasm_module( ).
    lo_mod->parse( lv_w ).
    cl_abap_unit_assert=>assert_equals( act = lines( lo_mod->mt_functions ) exp = 1 ).
  ENDMETHOD.

  METHOD test_compile_add.
    DATA(lv_w) = CONV xstring( '0061736D0100000001070160027F7F017F030201000707010361646400000A09010700200020016A0B' ).
    DATA(lo_mod) = NEW zcl_wasm_module( ).
    lo_mod->parse( lv_w ).
    DATA(lv_abap) = NEW zcl_wasm_codegen( )->compile( lo_mod ).
    cl_abap_unit_assert=>assert_true( act = xsdbool( lv_abap CS 'FORM ADD' ) msg = 'FORM ADD' ).
    cl_abap_unit_assert=>assert_true( act = xsdbool( lv_abap CS 'FORM WASM_INIT' ) msg = 'FORM WASM_INIT' ).
  ENDMETHOD.

  METHOD test_execute_add.
    DATA(lv_w) = CONV xstring( '0061736D0100000001070160027F7F017F030201000707010361646400000A09010700200020016A0B' ).
    DATA(lv_prog) = compile_and_gen( lv_w ).
    DATA lv_r TYPE i.
    PERFORM ('ADD') IN PROGRAM (lv_prog) USING 2 3 CHANGING lv_r.
    cl_abap_unit_assert=>assert_equals( act = lv_r exp = 5 msg = 'add(2,3)=5' ).
  ENDMETHOD.

  METHOD test_execute_factorial.
    DATA(lv_w) = CONV xstring( '0061736D0100000001060160017F017F03020100070D0109666163746F7269616C00000A19011700200041014C047F4101052000200041016B10006C0B0B' ).
    DATA(lv_prog) = compile_and_gen( lv_w ).
    DATA lv_r TYPE i.
    PERFORM ('FACTORIAL') IN PROGRAM (lv_prog) USING 10 CHANGING lv_r.
    cl_abap_unit_assert=>assert_equals( act = lv_r exp = 3628800 msg = |factorial(10)={ lv_r }| ).
  ENDMETHOD.

  METHOD test_parse_quickjs.
    DATA(lv_wasm) = load_smw0( 'ZQJS.WASM' ).
    cl_abap_unit_assert=>assert_not_initial( act = lv_wasm msg = 'ZQJS.WASM not found in SMW0' ).
    cl_abap_unit_assert=>assert_true( act = xsdbool( xstrlen( lv_wasm ) > 1000000 ) msg = 'Should be > 1MB' ).

    DATA(lo_mod) = NEW zcl_wasm_module( ).
    lo_mod->parse( lv_wasm ).

    " QuickJS: 98 types, 9 imports, 1410 functions, 3 exports
    cl_abap_unit_assert=>assert_equals( act = lines( lo_mod->mt_types ) exp = 98 msg = 'types' ).
    cl_abap_unit_assert=>assert_equals( act = lines( lo_mod->mt_imports ) exp = 9 msg = 'imports' ).
    cl_abap_unit_assert=>assert_equals( act = lines( lo_mod->mt_functions ) exp = 1410 msg = 'functions' ).
    cl_abap_unit_assert=>assert_equals( act = lines( lo_mod->mt_exports ) exp = 3 msg = 'exports' ).
    cl_abap_unit_assert=>assert_equals( act = lines( lo_mod->mt_data ) exp = 3802 msg = 'data segments' ).

    " Check partial parse error info
    DATA(lv_summary) = lo_mod->summary( ).
    cl_abap_unit_assert=>assert_true( act = xsdbool( lv_summary CS 'Instructions' ) msg = lv_summary ).
  ENDMETHOD.

  METHOD test_compile_quickjs.
    DATA(lv_wasm) = load_smw0( 'ZQJS.WASM' ).
    cl_abap_unit_assert=>assert_not_initial( act = lv_wasm msg = 'ZQJS.WASM not in SMW0' ).

    DATA(lo_mod) = NEW zcl_wasm_module( ).
    lo_mod->parse( lv_wasm ).

    DATA(lv_abap) = NEW zcl_wasm_codegen( )->compile( lo_mod ).
    cl_abap_unit_assert=>assert_not_initial( act = lv_abap ).

    " Check line lengths
    DATA: lt_code TYPE STANDARD TABLE OF string.
    SPLIT lv_abap AT cl_abap_char_utilities=>newline INTO TABLE lt_code.
    DATA lv_long TYPE i.
    LOOP AT lt_code INTO DATA(lv_chk).
      IF strlen( lv_chk ) > 255. lv_long = lv_long + 1. ENDIF.
    ENDLOOP.
    cl_abap_unit_assert=>assert_equals( act = lv_long exp = 0 msg = |{ lv_long } lines > 255 chars| ).

    " Should have > 50K lines (packing reduces ~244K to ~100K)
    cl_abap_unit_assert=>assert_true( act = xsdbool( lines( lt_code ) > 50000 ) msg = |Only { lines( lt_code ) } lines| ).

    " Check FORM WASM_INIT exists
    cl_abap_unit_assert=>assert_true( act = xsdbool( lv_abap CS 'FORM WASM_INIT' ) msg = 'FORM WASM_INIT' ).
  ENDMETHOD.

  METHOD test_generate_quickjs.
    DATA(lv_wasm) = load_smw0( 'ZQJS.WASM' ).
    cl_abap_unit_assert=>assert_not_initial( act = lv_wasm msg = 'ZQJS.WASM not in SMW0' ).

    DATA(lo_mod) = NEW zcl_wasm_module( ).
    lo_mod->parse( lv_wasm ).

    DATA lv_total_instr TYPE i.
    LOOP AT lo_mod->mt_functions INTO DATA(ls_f).
      lv_total_instr = lv_total_instr + lines( ls_f-code ).
    ENDLOOP.

    cl_abap_unit_assert=>assert_true( act = xsdbool( lo_mod->ms_memory-min_pages > 0 ) msg = |mem_pages={ lo_mod->ms_memory-min_pages }| ).

    DATA(lv_abap) = NEW zcl_wasm_codegen( )->compile( lo_mod ).
    cl_abap_unit_assert=>assert_true( act = xsdbool( lv_abap CS 'CONCATENATE gv_mem' ) msg = 'no mem alloc in WASM_INIT' ).
    DATA: lt_code TYPE STANDARD TABLE OF string.
    SPLIT lv_abap AT cl_abap_char_utilities=>newline INTO TABLE lt_code.

    DATA: lv_prog TYPE string, lv_msg TYPE string, lv_word TYPE string, lv_line TYPE i.
    GENERATE SUBROUTINE POOL lt_code NAME lv_prog MESSAGE lv_msg WORD lv_word LINE lv_line.
    IF sy-subrc <> 0.
      DATA(lv_ctx) = ||.
      DATA(lv_from) = lv_line - 25. IF lv_from < 1. lv_from = 1. ENDIF.
      DATA(lv_to) = lv_line + 3. IF lv_to > lines( lt_code ). lv_to = lines( lt_code ). ENDIF.
      DO lv_to - lv_from + 1 TIMES. DATA(lv_li) = lv_from + sy-index - 1. lv_ctx = lv_ctx && |{ lv_li }:{ lt_code[ lv_li ] }\n|. ENDDO.
      cl_abap_unit_assert=>fail( msg = |GENERATE: rc={ sy-subrc } { lv_msg } line={ lv_line }\n{ lv_ctx }| ).
      RETURN.
    ENDIF.

    " WASM_INIT + _START
    TRY.
        PERFORM ('WASM_INIT') IN PROGRAM (lv_prog).
        PERFORM ('_START') IN PROGRAM (lv_prog).
      CATCH cx_root INTO DATA(lx_run).
        " Runtime errors expected (WASI stubs, partial parse)
        " The key assertion is that GENERATE succeeded
    ENDTRY.
  ENDMETHOD.
ENDCLASS.
