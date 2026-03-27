*&---------------------------------------------------------------------*
*& Report ZWASM_COMPILER
*& Interactive WASM-to-ABAP compiler
*& Load .wasm binary → parse → compile → execute / save
*&---------------------------------------------------------------------*
REPORT zwasm_compiler.

*----------------------------------------------------------------------*
* Selection Screen
*----------------------------------------------------------------------*
SELECTION-SCREEN BEGIN OF BLOCK b1 WITH FRAME TITLE sc_src.
  PARAMETERS: p_file  RADIOBUTTON GROUP src DEFAULT 'X',
              p_smw0  RADIOBUTTON GROUP src,
              p_hex   RADIOBUTTON GROUP src.
  PARAMETERS: p_fname TYPE string LOWER CASE VISIBLE LENGTH 60.
  PARAMETERS: p_smw0n TYPE c LENGTH 40 LOWER CASE.
  PARAMETERS: p_hexin TYPE string LOWER CASE VISIBLE LENGTH 60.
SELECTION-SCREEN END OF BLOCK b1.

SELECTION-SCREEN BEGIN OF BLOCK b2 WITH FRAME TITLE sc_exe.
  PARAMETERS: p_temp  RADIOBUTTON GROUP gen DEFAULT 'X',
              p_perm  RADIOBUTTON GROUP gen.
  PARAMETERS: p_prog  TYPE c LENGTH 30 DEFAULT 'ZWASM_GEN'.
  PARAMETERS: p_split AS CHECKBOX DEFAULT ' '.
  PARAMETERS: p_maxln TYPE i DEFAULT 5000.
  SELECTION-SCREEN SKIP.
  PARAMETERS: p_exec  AS CHECKBOX DEFAULT 'X'.
  PARAMETERS: p_func  TYPE c LENGTH 30 LOWER CASE DEFAULT 'add'.
  PARAMETERS: p_arg0  TYPE i DEFAULT 2.
  PARAMETERS: p_arg1  TYPE i DEFAULT 3.
SELECTION-SCREEN END OF BLOCK b2.

SELECTION-SCREEN BEGIN OF BLOCK b3 WITH FRAME TITLE sc_out.
  PARAMETERS: p_show  AS CHECKBOX DEFAULT 'X'.
  PARAMETERS: p_class AS CHECKBOX DEFAULT ' '.
  PARAMETERS: p_clsnm TYPE c LENGTH 30 DEFAULT 'ZCL_WASM_OUT'.
SELECTION-SCREEN END OF BLOCK b3.

INITIALIZATION.
  sc_src = 'WASM Source'.
  sc_exe = 'Generation & Execution'.
  sc_out = 'Output'.
  %_p_temp_%_app_%-text = 'Temporary (GENERATE POOL)'.
  %_p_perm_%_app_%-text = 'Persistent program (INSERT REPORT)'.
  %_p_prog_%_app_%-text = 'Program name'.
  %_p_split_%_app_%-text = 'Split to INCLUDEs'.
  %_p_maxln_%_app_%-text = 'Max lines per include'.
  %_p_class_%_app_%-text = 'Generate CLASS wrapper'.
  %_p_clsnm_%_app_%-text = 'Class name'.

*----------------------------------------------------------------------*
* File dialog
*----------------------------------------------------------------------*
AT SELECTION-SCREEN ON VALUE-REQUEST FOR p_fname.
  DATA: lt_filetable TYPE filetable,
        lv_rc        TYPE i.
  cl_gui_frontend_services=>file_open_dialog(
    EXPORTING
      window_title  = 'Select WASM binary'
      file_filter   = 'WASM files (*.wasm)|*.wasm|All (*.*)|*.*'
    CHANGING
      file_table    = lt_filetable
      rc            = lv_rc
    EXCEPTIONS OTHERS = 1 ).
  IF lv_rc > 0.
    READ TABLE lt_filetable INDEX 1 INTO p_fname.
  ENDIF.

*----------------------------------------------------------------------*
* Main
*----------------------------------------------------------------------*
START-OF-SELECTION.

  DATA: lv_wasm TYPE xstring.

  " ── Load WASM ──
  IF p_file = abap_true.
    IF p_fname IS INITIAL. WRITE: / 'ERROR: No file.'. RETURN. ENDIF.
    DATA: lt_data TYPE STANDARD TABLE OF x255, lv_flen TYPE i.
    cl_gui_frontend_services=>gui_upload(
      EXPORTING filename = p_fname filetype = 'BIN'
      IMPORTING filelength = lv_flen
      CHANGING  data_tab = lt_data
      EXCEPTIONS OTHERS = 1 ).
    IF sy-subrc <> 0. WRITE: / 'Upload failed:', sy-subrc. RETURN. ENDIF.
    CALL FUNCTION 'SCMS_BINARY_TO_XSTRING'
      EXPORTING input_length = lv_flen
      IMPORTING buffer = lv_wasm
      TABLES binary_tab = lt_data.

  ELSEIF p_smw0 = abap_true.
    IF p_smw0n IS INITIAL. WRITE: / 'ERROR: No SMW0 name.'. RETURN. ENDIF.
    DATA: lt_mime TYPE STANDARD TABLE OF w3mime, lv_size TYPE i.
    CALL FUNCTION 'SAP_READ_BINARY_RESOURCE'
      EXPORTING key = p_smw0n
      IMPORTING size = lv_size
      TABLES data = lt_mime
      EXCEPTIONS OTHERS = 1.
    IF sy-subrc <> 0. WRITE: / 'SMW0 read failed:', p_smw0n. RETURN. ENDIF.
    CALL FUNCTION 'SCMS_BINARY_TO_XSTRING'
      EXPORTING input_length = lv_size
      IMPORTING buffer = lv_wasm
      TABLES binary_tab = lt_mime.

  ELSEIF p_hex = abap_true.
    IF p_hexin IS INITIAL. WRITE: / 'ERROR: No hex.'. RETURN. ENDIF.
    lv_wasm = p_hexin.
  ENDIF.

  IF xstrlen( lv_wasm ) < 8. WRITE: / 'ERROR: Too small:', xstrlen( lv_wasm ). RETURN. ENDIF.

  WRITE: / '── WASM Binary ──'.
  WRITE: / 'Size:', xstrlen( lv_wasm ), 'bytes'.
  ULINE.

  " ── Parse ──
  DATA(lo_mod) = NEW zcl_wasm_module( ).
  TRY.
      lo_mod->parse( lv_wasm ).
    CATCH cx_root INTO DATA(lx_parse).
      WRITE: / 'PARSE ERROR:', lx_parse->get_text( ). RETURN.
  ENDTRY.

  WRITE: / '── Module Summary ──'.
  WRITE: / lo_mod->summary( ).
  ULINE.

  WRITE: / '── Exports ──'.
  LOOP AT lo_mod->mt_exports INTO DATA(ls_exp).
    DATA(lv_ekind) = ||.
    CASE ls_exp-kind.
      WHEN 0. lv_ekind = 'func'.
      WHEN 1. lv_ekind = 'table'.
      WHEN 2. lv_ekind = 'memory'.
      WHEN 3. lv_ekind = 'global'.
    ENDCASE.
    WRITE: / '  ', lv_ekind, ls_exp-name, 'index:', ls_exp-index.
  ENDLOOP.
  ULINE.

  " ── Compile ──
  DATA(lv_progname) = CONV string( p_prog ).
  CONDENSE lv_progname.
  TRANSLATE lv_progname TO UPPER CASE.

  DATA(lv_abap) = NEW zcl_wasm_codegen( )->compile(
    io_module = lo_mod
    iv_name   = lv_progname ).

  DATA: lt_code TYPE STANDARD TABLE OF string WITH DEFAULT KEY.
  SPLIT lv_abap AT cl_abap_char_utilities=>newline INTO TABLE lt_code.

  WRITE: / '── Compiled ABAP ──'.
  WRITE: / 'Lines:', lines( lt_code ).

  IF p_show = abap_true.
    ULINE.
    WRITE: / '── Generated Source ──'.
    DATA(lv_lineno) = 0.
    LOOP AT lt_code INTO DATA(lv_line).
      lv_lineno = lv_lineno + 1.
      WRITE: / lv_lineno, lv_line.
    ENDLOOP.
    ULINE.
  ENDIF.

  " ── Class wrapper ──
  IF p_class = abap_true.
    DATA(lv_wasm_hex) = CONV string( lv_wasm ).
    DATA(lv_clsname) = CONV string( p_clsnm ).
    CONDENSE lv_clsname.
    TRANSLATE lv_clsname TO UPPER CASE.

    DATA(lv_cls_src) = ||.
    IF p_perm = abap_true.
      " Static: class points to persistent program
      lv_cls_src = NEW zcl_wasm_codegen( )->compile_class(
        io_module    = lo_mod
        iv_classname = lv_clsname
        iv_program   = lv_progname ).
    ELSE.
      " Dynamic: self-contained with embedded WASM
      lv_cls_src = NEW zcl_wasm_codegen( )->compile_class(
        io_module    = lo_mod
        iv_classname = lv_clsname
        iv_wasm_hex  = lv_wasm_hex ).
    ENDIF.

    ULINE.
    WRITE: / '── CLASS Wrapper ──'.
    WRITE: / 'Class:', lv_clsname.
    DATA: lt_cls_lines TYPE STANDARD TABLE OF string.
    SPLIT lv_cls_src AT cl_abap_char_utilities=>newline INTO TABLE lt_cls_lines.
    WRITE: / 'Lines:', lines( lt_cls_lines ).
    ULINE.
    DATA(lv_cln) = 0.
    LOOP AT lt_cls_lines INTO DATA(lv_cl).
      lv_cln = lv_cln + 1.
      WRITE: / lv_cln, lv_cl.
    ENDLOOP.
    ULINE.
    WRITE: / 'Usage:'.
    WRITE: / '  DATA(lo) = NEW', lv_clsname, '( ).'.
    LOOP AT lo_mod->mt_exports INTO DATA(ls_ex2) WHERE kind = 0.
      DATA(lv_ename) = ls_ex2-name.
      TRANSLATE lv_ename TO UPPER CASE.
      WRITE: / '  lo->', lv_ename, '( ... )'.
    ENDLOOP.
    ULINE.
  ENDIF.

  " ── Generate / Save ──
  DATA lv_target_prog TYPE string.

  IF p_temp = abap_true.
    " Temporary subroutine pool
    WRITE: / '── Temporary Generation ──'.
    DATA lv_msg TYPE string.
    GENERATE SUBROUTINE POOL lt_code NAME lv_target_prog MESSAGE lv_msg.
    IF sy-subrc <> 0.
      WRITE: / 'GENERATE failed:', sy-subrc, lv_msg. RETURN.
    ENDIF.
    WRITE: / 'Generated temporary program:', lv_target_prog.

  ELSE.
    " Persistent program via INSERT REPORT
    WRITE: / '── Persistent Program ──'.
    DATA(lv_repname) = CONV syrepid( lv_progname ).

    DATA lv_gen_msg TYPE string.

    IF p_split = abap_true.
      " Split into INCLUDEs
      DATA(lt_includes) = NEW zcl_wasm_codegen( )->split_to_includes(
        iv_source    = lv_abap
        iv_name      = lv_progname
        iv_max_lines = p_maxln ).

      " Pass 1: INSERT all includes (includes first, main last)
      DATA(lv_main_idx) = 0.
      DATA(lv_inc_idx) = 0.
      LOOP AT lt_includes INTO DATA(ls_inc).
        lv_inc_idx = sy-tabix.
        DATA(lv_iname) = CONV syrepid( ls_inc-name ).
        DATA: lt_isrc TYPE STANDARD TABLE OF string.
        SPLIT ls_inc-source AT cl_abap_char_utilities=>newline INTO TABLE lt_isrc.
        " Check for long lines
        DATA(lv_ok) = abap_true.
        LOOP AT lt_isrc INTO DATA(lv_ichk).
          IF strlen( lv_ichk ) > 255.
            WRITE: / 'LONG LINE in', lv_iname, 'line', sy-tabix, 'len=', strlen( lv_ichk ).
            lv_ok = abap_false.
          ENDIF.
        ENDLOOP.
        IF lv_ok = abap_false. CONTINUE. ENDIF.
        " Main program = type 'S' (subroutine pool), includes = type 'I'
        IF lv_inc_idx = 1.
          INSERT REPORT lv_iname FROM lt_isrc PROGRAM TYPE 'S'.
        ELSE.
          INSERT REPORT lv_iname FROM lt_isrc PROGRAM TYPE 'I'.
        ENDIF.
        IF sy-subrc = 0.
          WRITE: / 'INSERT OK:', lv_iname, '(', lines( lt_isrc ), 'lines )'.
        ELSE.
          WRITE: / 'INSERT failed:', lv_iname.
        ENDIF.
        IF lv_inc_idx = 1. lv_main_idx = 1. ENDIF.
      ENDLOOP.
      " Pass 2: GENERATE only the main program (compiles all includes)
      IF lv_main_idx > 0.
        READ TABLE lt_includes INDEX 1 INTO ls_inc.
        DATA(lv_mname) = CONV syrepid( ls_inc-name ).
        GENERATE REPORT lv_mname MESSAGE lv_gen_msg.
        IF sy-subrc = 0.
          WRITE: / 'Activated:', lv_mname.
        ELSE.
          WRITE: / 'Activation failed:', lv_mname, lv_gen_msg.
        ENDIF.
      ENDIF.
    ELSE.
      " Single program — check for long lines first
      DATA: lv_long_cnt TYPE i.
      LOOP AT lt_code INTO DATA(lv_chk).
        IF strlen( lv_chk ) > 255.
          lv_long_cnt = lv_long_cnt + 1.
          IF lv_long_cnt <= 10.
            WRITE: / 'LINE', sy-tabix, 'len=', strlen( lv_chk ), lv_chk(80), '...'.
          ENDIF.
        ENDIF.
      ENDLOOP.
      IF lv_long_cnt > 0.
        WRITE: / 'Total lines > 255:', lv_long_cnt, '— fix codegen or use Split'.
        RETURN.
      ENDIF.
      INSERT REPORT lv_repname FROM lt_code.
      GENERATE REPORT lv_repname MESSAGE lv_gen_msg.
      IF sy-subrc <> 0.
        WRITE: / 'Failed:', sy-subrc, lv_gen_msg. RETURN.
      ENDIF.
      WRITE: / 'Saved + activated:', lv_repname.
    ENDIF.

    WRITE: / 'Call via: PERFORM <func> IN PROGRAM', lv_repname.
    lv_target_prog = lv_repname.

    " Also save wrapper program if class checkbox is set
    IF p_class = abap_true AND lv_cls_src IS NOT INITIAL.
      DATA(lv_wrap_name) = CONV syrepid( lv_progname && '_WRAP' ).
      DATA: lt_wrap_code TYPE STANDARD TABLE OF string.
      SPLIT lv_cls_src AT cl_abap_char_utilities=>newline INTO TABLE lt_wrap_code.

      INSERT REPORT lv_wrap_name FROM lt_wrap_code.
      IF sy-subrc = 0.
        GENERATE REPORT lv_wrap_name MESSAGE lv_gen_msg.
        IF sy-subrc = 0.
          WRITE: / 'Wrapper program created:', lv_wrap_name.
        ELSE.
          WRITE: / 'Wrapper activation failed:', sy-subrc, lv_gen_msg.
        ENDIF.
      ELSE.
        WRITE: / 'INSERT REPORT wrapper failed:', sy-subrc.
      ENDIF.
    ENDIF.
  ENDIF.

  " ── Execute ──
  IF p_exec = abap_true AND lv_target_prog IS NOT INITIAL.
    WRITE: / '── Execution ──'.

    DATA: lv_result TYPE i.
    DATA(lv_func) = CONV string( p_func ).
    CONDENSE lv_func.
    TRANSLATE lv_func TO UPPER CASE.
    WRITE: / 'Calling:', lv_func, 'with args:', p_arg0, p_arg1.

    TRY.
        PERFORM (lv_func) IN PROGRAM (lv_target_prog)
          USING p_arg0 p_arg1
          CHANGING lv_result.
      CATCH cx_root INTO DATA(lx_exec).
        WRITE: / 'EXECUTION ERROR:', lx_exec->get_text( ). RETURN.
    ENDTRY.

    WRITE: / '════════════════════'.
    WRITE: / 'RESULT:', lv_result.
    WRITE: / '════════════════════'.
  ENDIF.
