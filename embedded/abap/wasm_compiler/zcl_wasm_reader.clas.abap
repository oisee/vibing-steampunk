CLASS zcl_wasm_reader DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    METHODS constructor IMPORTING iv_wasm TYPE xstring.
    METHODS read_byte RETURNING VALUE(rv) TYPE i.
    METHODS read_u32 RETURNING VALUE(rv) TYPE i.
    METHODS read_i32 RETURNING VALUE(rv) TYPE i.
    METHODS read_i64 RETURNING VALUE(rv) TYPE int8.
    METHODS read_u32_fixed RETURNING VALUE(rv) TYPE i.
    METHODS read_bytes IMPORTING iv_n TYPE i RETURNING VALUE(rv) TYPE xstring.
    METHODS read_string RETURNING VALUE(rv) TYPE string.
    METHODS get_pos RETURNING VALUE(rv) TYPE i.
    METHODS set_pos IMPORTING iv_pos TYPE i.
    METHODS remaining RETURNING VALUE(rv) TYPE i.
    METHODS eof RETURNING VALUE(rv) TYPE abap_bool.
  PRIVATE SECTION.
    DATA mv_data TYPE xstring.
    DATA mv_pos TYPE i.
    DATA mv_len TYPE i.
ENDCLASS.

CLASS zcl_wasm_reader IMPLEMENTATION.
  METHOD constructor.
    mv_data = iv_wasm. mv_pos = 0. mv_len = xstrlen( iv_wasm ).
  ENDMETHOD.

  METHOD read_byte.
    IF mv_pos >= mv_len. rv = 0. RETURN. ENDIF.
    DATA lv_b TYPE x LENGTH 1. lv_b = mv_data+mv_pos(1). rv = lv_b. mv_pos = mv_pos + 1.
  ENDMETHOD.

  METHOD read_u32.
    " Unsigned LEB128 (int8 arithmetic, manual doubling — no ipow overflow)
    DATA: lv_b TYPE i, lv_shift TYPE i, lv_result TYPE int8, lv_pw TYPE int8.
    lv_result = 0. lv_shift = 0.
    DO.
      lv_b = read_byte( ).
      DATA(lv_masked) = CONV int8( lv_b MOD 128 ).
      lv_pw = 1. DO lv_shift TIMES. lv_pw = lv_pw * 2. ENDDO.
      lv_result = lv_result + lv_masked * lv_pw.
      IF lv_b < 128. EXIT. ENDIF.
      lv_shift = lv_shift + 7.
      IF lv_shift > 35. EXIT. ENDIF. " safety: max 5 bytes for u32
    ENDDO.
    " Truncate to signed 32-bit (u32 values > 2^31 wrap to negative)
    DATA(lv_mod) = CONV int8( 4294967296 ).
    lv_result = lv_result MOD lv_mod.
    IF lv_result > 2147483647. lv_result = lv_result - lv_mod. ENDIF.
    rv = lv_result.
  ENDMETHOD.

  METHOD read_i32.
    " Signed LEB128 (int8 arithmetic, manual doubling — no ipow overflow)
    DATA: lv_b TYPE i, lv_shift TYPE i, lv_result TYPE int8, lv_pw TYPE int8.
    lv_result = 0. lv_shift = 0.
    DO.
      lv_b = read_byte( ).
      DATA(lv_masked2) = CONV int8( lv_b MOD 128 ).
      lv_pw = 1. DO lv_shift TIMES. lv_pw = lv_pw * 2. ENDDO.
      lv_result = lv_result + lv_masked2 * lv_pw.
      lv_shift = lv_shift + 7.
      IF lv_b < 128.
        IF lv_shift < 32 AND lv_b >= 64.
          lv_pw = 1. DO lv_shift TIMES. lv_pw = lv_pw * 2. ENDDO.
          lv_result = lv_result - lv_pw.
        ENDIF.
        EXIT.
      ENDIF.
      IF lv_shift > 35. EXIT. ENDIF.
    ENDDO.
    " Truncate to signed 32-bit
    DATA(lv_mod2) = CONV int8( 4294967296 ).
    lv_result = lv_result MOD lv_mod2.
    IF lv_result < 0. lv_result = lv_result + lv_mod2. ENDIF.
    IF lv_result > 2147483647. lv_result = lv_result - lv_mod2. ENDIF.
    rv = lv_result.
  ENDMETHOD.

  METHOD read_i64.
    " Signed LEB128 for i64 (int8 arithmetic, overflow-safe)
    DATA: lv_b TYPE i, lv_shift TYPE i, lv_pw TYPE int8.
    rv = 0. lv_shift = 0.
    DO.
      lv_b = read_byte( ).
      DATA(lv_masked3) = CONV int8( lv_b MOD 128 ).
      IF lv_shift < 56.
        lv_pw = 1. DO lv_shift TIMES. lv_pw = lv_pw * 2. ENDDO.
        rv = rv + lv_masked3 * lv_pw.
      ENDIF. " shifts >= 56: skip high bits (would overflow int8)
      lv_shift = lv_shift + 7.
      IF lv_b < 128.
        IF lv_shift < 64 AND lv_b >= 64.
          lv_pw = 1. DO lv_shift TIMES. lv_pw = lv_pw * 2. ENDDO.
          TRY. rv = rv - lv_pw. CATCH cx_root. ENDTRY.
        ENDIF.
        EXIT.
      ENDIF.
      IF lv_shift > 70. EXIT. ENDIF.
    ENDDO.
  ENDMETHOD.

  METHOD read_u32_fixed.
    " Little-endian 4-byte unsigned integer
    DATA lv_b TYPE x LENGTH 4. lv_b = mv_data+mv_pos(4).
    " Reverse little-endian to big-endian
    DATA(lv_r) = lv_b+3(1) && lv_b+2(1) && lv_b+1(1) && lv_b+0(1).
    rv = lv_r. mv_pos = mv_pos + 4.
  ENDMETHOD.

  METHOD read_bytes.
    IF iv_n <= 0. RETURN. ENDIF.
    IF mv_pos + iv_n > mv_len.
      DATA(lv_rem) = mv_len - mv_pos.
      IF lv_rem > 0. rv = mv_data+mv_pos(lv_rem). ENDIF.
      mv_pos = mv_len. RETURN.
    ENDIF.
    rv = mv_data+mv_pos(iv_n). mv_pos = mv_pos + iv_n.
  ENDMETHOD.

  METHOD read_string.
    DATA(lv_len) = read_u32( ).
    DATA(lv_bytes) = read_bytes( lv_len ).
    DATA(lo_conv) = cl_abap_conv_in_ce=>create( input = lv_bytes encoding = 'UTF-8' ).
    lo_conv->read( IMPORTING data = rv ).
  ENDMETHOD.

  METHOD get_pos. rv = mv_pos. ENDMETHOD.
  METHOD set_pos. mv_pos = iv_pos. ENDMETHOD.
  METHOD remaining. rv = mv_len - mv_pos. ENDMETHOD.
  METHOD eof. rv = xsdbool( mv_pos >= mv_len ). ENDMETHOD.
ENDCLASS.
