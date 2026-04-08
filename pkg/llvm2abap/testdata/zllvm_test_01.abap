REPORT zllvm_test_01.

CLASS lcl DEFINITION FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    CLASS-METHODS add IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS sub IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS mul IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS div_s IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS rem_s IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS negate IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS identity IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS and_ IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS or_ IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS xor_ IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS shl IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS shr IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS quadratic IMPORTING a TYPE i b TYPE i c TYPE i d TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS fadd IMPORTING a TYPE f b TYPE f RETURNING VALUE(rv) TYPE f.
    CLASS-METHODS fmul IMPORTING a TYPE f b TYPE f RETURNING VALUE(rv) TYPE f.
    CLASS-METHODS add64 IMPORTING a TYPE int8 b TYPE int8 RETURNING VALUE(rv) TYPE int8.
    CLASS-METHODS abs_val IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS max IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS min IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS clamp IMPORTING a TYPE i b TYPE i c TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS sign IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS sum_to IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS factorial IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS fibonacci IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS gcd IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS is_prime IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS double_val IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS square IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS cube IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS factorial_rec IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS fib_rec IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS point_sum IMPORTING a TYPE i RETURNING VALUE(rv) TYPE i.
    CLASS-METHODS point_set IMPORTING a TYPE i b TYPE i c TYPE i.
    CLASS-METHODS array_sum IMPORTING a TYPE i b TYPE i RETURNING VALUE(rv) TYPE i.
ENDCLASS.

CLASS lcl IMPLEMENTATION.
  METHOD add.
    DATA: lv_3 TYPE i.
    lv_3 = b + a.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD sub.
    DATA: lv_3 TYPE i.
    lv_3 = a - b.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD mul.
    DATA: lv_3 TYPE i.
    lv_3 = b * a.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD div_s.
    DATA: lv_3 TYPE i.
    lv_3 = a / b.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD rem_s.
    DATA: lv_3 TYPE i.
    lv_3 = a MOD b.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD negate.
    DATA: lv_2 TYPE i.
    lv_2 = 0 - a.
    rv = lv_2. RETURN.
  ENDMETHOD.
  
  METHOD identity.
    rv = a. RETURN.
  ENDMETHOD.
  
  METHOD and_.
    DATA: lv_3 TYPE i.
    DATA(lx_a_lv_3) = CONV x4( b ). DATA(lx_b_lv_3) = CONV x4( a ). lv_3 = lx_a_lv_3 BIT-AND lx_b_lv_3.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD or_.
    DATA: lv_3 TYPE i.
    DATA(lx_a_lv_3) = CONV x4( b ). DATA(lx_b_lv_3) = CONV x4( a ). lv_3 = lx_a_lv_3 BIT-OR lx_b_lv_3.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD xor_.
    DATA: lv_3 TYPE i.
    DATA(lx_a_lv_3) = CONV x4( b ). DATA(lx_b_lv_3) = CONV x4( a ). lv_3 = lx_a_lv_3 BIT-XOR lx_b_lv_3.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD shl.
    DATA: lv_3 TYPE i.
    TRY. lv_3 = a * ipow( base = 2 exp = b ). CATCH cx_root. lv_3 = 0. ENDTRY.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD shr.
    DATA: lv_3 TYPE i.
    TRY. lv_3 = a / ipow( base = 2 exp = b ). CATCH cx_root. lv_3 = 0. ENDTRY.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD quadratic.
    DATA: lv_5 TYPE i, lv_6 TYPE i, lv_7 TYPE i, lv_8 TYPE i.
    lv_5 = d * a.
    lv_6 = lv_5 + b.
    lv_7 = lv_6 * d.
    lv_8 = lv_7 + c.
    rv = lv_8. RETURN.
  ENDMETHOD.
  
  METHOD fadd.
    DATA: lv_3 TYPE f.
    lv_3 = a + b.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD fmul.
    DATA: lv_3 TYPE f.
    lv_3 = a * b.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD add64.
    DATA: lv_3 TYPE int8.
    lv_3 = b + a.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD abs_val.
    DATA: lv_2 TYPE i.
    lv_2 = abs( a ).
    rv = lv_2. RETURN.
  ENDMETHOD.
  
  METHOD max.
    DATA: lv_3 TYPE i.
    IF a > b. lv_3 = a. ELSE. lv_3 = b. ENDIF.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD min.
    DATA: lv_3 TYPE i.
    IF a < b. lv_3 = a. ELSE. lv_3 = b. ENDIF.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD clamp.
    DATA: lv_4 TYPE i, lv_5 TYPE i, lv_6 TYPE i.
    IF a < b. lv_4 = 1. ELSE. lv_4 = 0. ENDIF.
    IF a < c. lv_5 = a. ELSE. lv_5 = c. ENDIF.
    IF lv_4 <> 0. lv_6 = b. ELSE. lv_6 = lv_5. ENDIF.
    rv = lv_6. RETURN.
  ENDMETHOD.
  
  METHOD sign.
    DATA: lv_2 TYPE i, lv_3 TYPE i, lv_4 TYPE i.
    TRY. lv_2 = a / ipow( base = 2 exp = 31 ). CATCH cx_root. lv_2 = 0. ENDTRY.
    IF a < 1. lv_3 = 1. ELSE. lv_3 = 0. ENDIF.
    IF lv_3 <> 0. lv_4 = lv_2. ELSE. lv_4 = 1. ENDIF.
    rv = lv_4. RETURN.
  ENDMETHOD.
  
  METHOD sum_to.
    DATA: lv_2 TYPE i, lv_4 TYPE i, lv_5 TYPE i, lv_6 TYPE int8, lv_7 TYPE i, lv_8 TYPE int8, lv_9 TYPE int8, lv_10 TYPE int8, lv_11 TYPE i, lv_12 TYPE i, lv_13 TYPE i, lv_15 TYPE i, lv_phi0 TYPE i.
    DATA lv_block TYPE string VALUE '1'.
    DO.
      CASE lv_block.
        WHEN '1'.
          IF a < 1. lv_2 = 1. ELSE. lv_2 = 0. ENDIF.
          IF lv_2 <> 0.
            lv_15 = 0.
            lv_block = '14'.
          ELSE.
            lv_block = '3'.
          ENDIF.
        WHEN '3'.
          TRY. lv_4 = a * ipow( base = 2 exp = 1 ). CATCH cx_root. lv_4 = 0. ENDTRY.
          lv_5 = a + -1.
          lv_6 = lv_5. " zext
          lv_7 = a + -2.
          lv_8 = lv_7. " zext
          lv_9 = lv_6 * lv_8.
          TRY. lv_10 = lv_9 / ipow( base = 2 exp = 1 ). CATCH cx_root. lv_10 = 0. ENDTRY.
          lv_11 = lv_10. " trunc
          lv_12 = lv_4 + lv_11.
          lv_13 = lv_12 + -1.
          lv_15 = lv_13.
          lv_block = '14'.
        WHEN '14'.
          rv = lv_15. RETURN.
      ENDCASE.
    ENDDO.
  ENDMETHOD.
  
  METHOD factorial.
    DATA: lv_2 TYPE i, lv_4 TYPE i, lv_6 TYPE i, lv_7 TYPE i, lv_8 TYPE i, lv_9 TYPE i, lv_10 TYPE i, lv_phi0 TYPE i, lv_phi1 TYPE i.
    DATA lv_block TYPE string VALUE '1'.
    DO.
      CASE lv_block.
        WHEN '1'.
          IF a < 2. lv_2 = 1. ELSE. lv_2 = 0. ENDIF.
          IF lv_2 <> 0.
            lv_4 = 1.
            lv_block = '3'.
          ELSE.
            lv_6 = 2.
            lv_7 = 1.
            lv_block = '5'.
          ENDIF.
        WHEN '3'.
          rv = lv_4. RETURN.
        WHEN '5'.
          lv_8 = lv_6 * lv_7.
          lv_9 = lv_6 + 1.
          IF lv_6 = a. lv_10 = 1. ELSE. lv_10 = 0. ENDIF.
          IF lv_10 <> 0.
            lv_4 = lv_8.
            lv_block = '3'.
          ELSE.
            lv_6 = lv_9.
            lv_7 = lv_8.
            lv_block = '5'.
          ENDIF.
      ENDCASE.
    ENDDO.
  ENDMETHOD.
  
  METHOD fibonacci.
    DATA: lv_2 TYPE i, lv_4 TYPE i, lv_5 TYPE i, lv_6 TYPE i, lv_7 TYPE i, lv_8 TYPE i, lv_9 TYPE i, lv_11 TYPE i, lv_phi0 TYPE i, lv_phi1 TYPE i, lv_phi2 TYPE i.
    DATA lv_block TYPE string VALUE '1'.
    DO.
      CASE lv_block.
        WHEN '1'.
          IF a < 2. lv_2 = 1. ELSE. lv_2 = 0. ENDIF.
          IF lv_2 <> 0.
            lv_11 = a.
            lv_block = '10'.
          ELSE.
            lv_4 = 2.
            lv_5 = 1.
            lv_6 = 0.
            lv_block = '3'.
          ENDIF.
        WHEN '3'.
          lv_7 = lv_5 + lv_6.
          lv_8 = lv_4 + 1.
          IF lv_4 = a. lv_9 = 1. ELSE. lv_9 = 0. ENDIF.
          IF lv_9 <> 0.
            lv_11 = lv_7.
            lv_block = '10'.
          ELSE.
            lv_phi0 = lv_8.
            lv_phi1 = lv_7.
            lv_phi2 = lv_5.
            lv_4 = lv_phi0.
            lv_5 = lv_phi1.
            lv_6 = lv_phi2.
            lv_block = '3'.
          ENDIF.
        WHEN '10'.
          rv = lv_11. RETURN.
      ENDCASE.
    ENDDO.
  ENDMETHOD.
  
  METHOD gcd.
    DATA: lv_3 TYPE i, lv_5 TYPE i, lv_6 TYPE i, lv_7 TYPE i, lv_8 TYPE i, lv_10 TYPE i, lv_phi0 TYPE i, lv_phi1 TYPE i.
    DATA lv_block TYPE string VALUE '2'.
    DO.
      CASE lv_block.
        WHEN '2'.
          IF b = 0. lv_3 = 1. ELSE. lv_3 = 0. ENDIF.
          IF lv_3 <> 0.
            lv_10 = a.
            lv_block = '9'.
          ELSE.
            lv_5 = a.
            lv_6 = b.
            lv_block = '4'.
          ENDIF.
        WHEN '4'.
          lv_7 = lv_5 MOD lv_6.
          IF lv_7 = 0. lv_8 = 1. ELSE. lv_8 = 0. ENDIF.
          IF lv_8 <> 0.
            lv_10 = lv_6.
            lv_block = '9'.
          ELSE.
            lv_phi0 = lv_6.
            lv_phi1 = lv_7.
            lv_5 = lv_phi0.
            lv_6 = lv_phi1.
            lv_block = '4'.
          ENDIF.
        WHEN '9'.
          rv = lv_10. RETURN.
      ENDCASE.
    ENDDO.
  ENDMETHOD.
  
  METHOD is_prime.
    DATA: lv_2 TYPE i, lv_4 TYPE i, lv_5 TYPE i, lv_6 TYPE i, lv_7 TYPE i, lv_9 TYPE i, lv_10 TYPE i, lv_11 TYPE i, lv_12 TYPE i, lv_14 TYPE i, lv_15 TYPE i, lv_17 TYPE i, lv_18 TYPE i, lv_20 TYPE i, lv_phi0 TYPE i.
    DATA lv_block TYPE string VALUE '1'.
    DO.
      CASE lv_block.
        WHEN '1'.
          IF a < 2. lv_2 = 1. ELSE. lv_2 = 0. ENDIF.
          IF lv_2 <> 0.
            lv_20 = 0.
            lv_block = '19'.
          ELSE.
            lv_block = '3'.
          ENDIF.
        WHEN '3'.
          IF a < 4. lv_4 = 1. ELSE. lv_4 = 0. ENDIF.
          DATA(lx_a_lv_5) = CONV x4( a ). DATA(lx_b_lv_5) = CONV x4( 1 ). lv_5 = lx_a_lv_5 BIT-AND lx_b_lv_5.
          IF lv_5 = 0. lv_6 = 1. ELSE. lv_6 = 0. ENDIF.
          DATA(lx_a_lv_7) = CONV x4( lv_4 ). DATA(lx_b_lv_7) = CONV x4( lv_6 ). lv_7 = lx_a_lv_7 BIT-OR lx_b_lv_7.
          IF lv_7 <> 0.
            lv_17 = lv_4.
            lv_block = '16'.
          ELSE.
            lv_9 = 2.
            lv_block = '8'.
          ENDIF.
        WHEN '8'.
          lv_10 = lv_9 + 1.
          lv_11 = lv_10 * lv_10.
          IF lv_11 > a. lv_12 = 1. ELSE. lv_12 = 0. ENDIF.
          IF lv_12 <> 0.
            lv_17 = lv_12.
            lv_block = '16'.
          ELSE.
            lv_block = '13'.
          ENDIF.
        WHEN '13'.
          lv_14 = a MOD lv_10.
          IF lv_14 = 0. lv_15 = 1. ELSE. lv_15 = 0. ENDIF.
          IF lv_15 <> 0.
            lv_17 = lv_12.
            lv_block = '16'.
          ELSE.
            lv_9 = lv_10.
            lv_block = '8'.
          ENDIF.
        WHEN '16'.
          lv_18 = lv_17. " zext
          lv_20 = lv_18.
          lv_block = '19'.
        WHEN '19'.
          rv = lv_20. RETURN.
      ENDCASE.
    ENDDO.
  ENDMETHOD.
  
  METHOD double_val.
    DATA: lv_2 TYPE i.
    TRY. lv_2 = a * ipow( base = 2 exp = 1 ). CATCH cx_root. lv_2 = 0. ENDTRY.
    rv = lv_2. RETURN.
  ENDMETHOD.
  
  METHOD square.
    DATA: lv_2 TYPE i.
    lv_2 = a * a.
    rv = lv_2. RETURN.
  ENDMETHOD.
  
  METHOD cube.
    DATA: lv_2 TYPE i, lv_3 TYPE i.
    lv_2 = a * a.
    lv_3 = lv_2 * a.
    rv = lv_3. RETURN.
  ENDMETHOD.
  
  METHOD factorial_rec.
    DATA: lv_3 TYPE i, lv_4 TYPE i, lv_5 TYPE i, lv_7 TYPE i, lv_8 TYPE i, lv_10 TYPE i, lv_phi0 TYPE i, lv_phi1 TYPE i.
    DATA lv_block TYPE string VALUE '1'.
    DO.
      CASE lv_block.
        WHEN '1'.
          lv_3 = 1.
          lv_4 = a.
          lv_block = '2'.
        WHEN '2'.
          IF lv_4 < 2. lv_5 = 1. ELSE. lv_5 = 0. ENDIF.
          IF lv_5 <> 0.
            lv_block = '9'.
          ELSE.
            lv_block = '6'.
          ENDIF.
        WHEN '6'.
          lv_7 = lv_4 + -1.
          lv_8 = lv_3 * lv_4.
          lv_3 = lv_8.
          lv_4 = lv_7.
          lv_block = '2'.
        WHEN '9'.
          lv_10 = lv_3 * 1.
          rv = lv_10. RETURN.
      ENDCASE.
    ENDDO.
  ENDMETHOD.
  
  METHOD fib_rec.
    DATA: lv_3 TYPE i, lv_4 TYPE i, lv_5 TYPE i, lv_7 TYPE i, lv_8 TYPE i, lv_9 TYPE i, lv_10 TYPE i, lv_12 TYPE i, lv_phi0 TYPE i, lv_phi1 TYPE i.
    DATA lv_block TYPE string VALUE '1'.
    DO.
      CASE lv_block.
        WHEN '1'.
          lv_3 = 0.
          lv_4 = a.
          lv_block = '2'.
        WHEN '2'.
          IF lv_4 < 2. lv_5 = 1. ELSE. lv_5 = 0. ENDIF.
          IF lv_5 <> 0.
            lv_block = '11'.
          ELSE.
            lv_block = '6'.
          ENDIF.
        WHEN '6'.
          lv_7 = lv_4 + -1.
          lv_8 = fib_rec( a = lv_7 ).
          lv_9 = lv_4 + -2.
          lv_10 = lv_3 + lv_8.
          lv_3 = lv_10.
          lv_4 = lv_9.
          lv_block = '2'.
        WHEN '11'.
          lv_12 = lv_3 + lv_4.
          rv = lv_12. RETURN.
      ENDCASE.
    ENDDO.
  ENDMETHOD.
  
  METHOD point_sum.
    DATA: lv_2 TYPE i, lv_3 TYPE i, lv_4 TYPE i, lv_5 TYPE i.
    PERFORM mem_ld_i32 USING a CHANGING lv_2.
    " GEP: lv_3 = &a->y (field 1)
    lv_3 = a + 4. " offset to field y
    PERFORM mem_ld_i32 USING lv_3 CHANGING lv_4.
    lv_5 = lv_4 + lv_2.
    rv = lv_5. RETURN.
  ENDMETHOD.
  
  METHOD point_set.
    DATA: lv_4 TYPE i.
    PERFORM mem_st_i32 USING a b.
    " GEP: lv_4 = &a->y (field 1)
    lv_4 = a + 4. " offset to field y
    PERFORM mem_st_i32 USING lv_4 c.
    RETURN.
  ENDMETHOD.
  
  METHOD array_sum.
    DATA: lv_3 TYPE i, lv_5 TYPE int8, lv_7 TYPE i, lv_9 TYPE int8, lv_10 TYPE i, lv_11 TYPE i, lv_12 TYPE i, lv_13 TYPE i, lv_14 TYPE int8, lv_15 TYPE int8, lv_phi0 TYPE i, lv_phi1 TYPE i.
    DATA lv_block TYPE string VALUE '2'.
    DO.
      CASE lv_block.
        WHEN '2'.
          IF b > 0. lv_3 = 1. ELSE. lv_3 = 0. ENDIF.
          IF lv_3 <> 0.
            lv_block = '4'.
          ELSE.
            lv_7 = 0.
            lv_block = '6'.
          ENDIF.
        WHEN '4'.
          lv_5 = i32. " zext
          lv_9 = 0.
          lv_10 = 0.
          lv_block = '8'.
        WHEN '6'.
          rv = lv_7. RETURN.
        WHEN '8'.
          lv_11 = a + lv_9 * 4.
          PERFORM mem_ld_i32 USING lv_11 CHANGING lv_12.
          lv_13 = lv_12 + lv_10.
          lv_14 = lv_9 + 1.
          IF lv_14 = lv_5. lv_15 = 1. ELSE. lv_15 = 0. ENDIF.
          IF lv_15 <> 0.
            lv_7 = lv_13.
            lv_block = '6'.
          ELSE.
            lv_9 = lv_14.
            lv_10 = lv_13.
            lv_block = '8'.
          ENDIF.
      ENDCASE.
    ENDDO.
  ENDMETHOD.
  
ENDCLASS.


CLASS ltc_test DEFINITION FOR TESTING DURATION SHORT RISK LEVEL HARMLESS.
  PRIVATE SECTION.
    METHODS test_add FOR TESTING.
    METHODS test_sub FOR TESTING.
    METHODS test_mul FOR TESTING.
    METHODS test_div FOR TESTING.
    METHODS test_rem FOR TESTING.
    METHODS test_negate FOR TESTING.
    METHODS test_identity FOR TESTING.
    METHODS test_quadratic FOR TESTING.
    METHODS test_sign FOR TESTING.
    METHODS test_factorial FOR TESTING.
    METHODS test_fibonacci FOR TESTING.
    METHODS test_double FOR TESTING.
    METHODS test_square FOR TESTING.
    METHODS test_cube FOR TESTING.
    METHODS test_factorial_rec FOR TESTING.
    METHODS test_fib_rec FOR TESTING.
    METHODS test_gcd FOR TESTING.
ENDCLASS.

CLASS ltc_test IMPLEMENTATION.
  METHOD test_add.
    cl_abap_unit_assert=>assert_equals( act = lcl=>add( a = 3 b = 4 ) exp = 7 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>add( a = -1 b = 1 ) exp = 0 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>add( a = 0 b = 0 ) exp = 0 ).
  ENDMETHOD.
  METHOD test_sub.
    cl_abap_unit_assert=>assert_equals( act = lcl=>sub( a = 10 b = 3 ) exp = 7 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>sub( a = 5 b = 5 ) exp = 0 ).
  ENDMETHOD.
  METHOD test_mul.
    cl_abap_unit_assert=>assert_equals( act = lcl=>mul( a = 6 b = 7 ) exp = 42 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>mul( a = 0 b = 99 ) exp = 0 ).
  ENDMETHOD.
  METHOD test_div.
    cl_abap_unit_assert=>assert_equals( act = lcl=>div_s( a = 15 b = 3 ) exp = 5 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>div_s( a = 7 b = 2 ) exp = 3 ).
  ENDMETHOD.
  METHOD test_rem.
    cl_abap_unit_assert=>assert_equals( act = lcl=>rem_s( a = 17 b = 5 ) exp = 2 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>rem_s( a = 10 b = 3 ) exp = 1 ).
  ENDMETHOD.
  METHOD test_negate.
    cl_abap_unit_assert=>assert_equals( act = lcl=>negate( a = 42 ) exp = -42 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>negate( a = -1 ) exp = 1 ).
  ENDMETHOD.
  METHOD test_identity.
    cl_abap_unit_assert=>assert_equals( act = lcl=>identity( a = 99 ) exp = 99 ).
  ENDMETHOD.
  METHOD test_quadratic.
    " a*x*x + b*x + c where a=1,b=2,c=3,x=5 → 1*25+2*5+3=38
    cl_abap_unit_assert=>assert_equals( act = lcl=>quadratic( a = 1 b = 2 c = 3 d = 5 ) exp = 38 ).
  ENDMETHOD.
  METHOD test_sign.
    cl_abap_unit_assert=>assert_equals( act = lcl=>sign( a = 5 ) exp = 1 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>sign( a = -5 ) exp = -1 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>sign( a = 0 ) exp = 0 ).
  ENDMETHOD.
  METHOD test_factorial.
    cl_abap_unit_assert=>assert_equals( act = lcl=>factorial( a = 0 ) exp = 1 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>factorial( a = 1 ) exp = 1 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>factorial( a = 5 ) exp = 120 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>factorial( a = 10 ) exp = 3628800 ).
  ENDMETHOD.
  METHOD test_fibonacci.
    cl_abap_unit_assert=>assert_equals( act = lcl=>fibonacci( a = 0 ) exp = 0 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>fibonacci( a = 1 ) exp = 1 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>fibonacci( a = 10 ) exp = 55 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>fibonacci( a = 20 ) exp = 6765 ).
  ENDMETHOD.
  METHOD test_double.
    cl_abap_unit_assert=>assert_equals( act = lcl=>double_val( a = 21 ) exp = 42 ).
  ENDMETHOD.
  METHOD test_square.
    cl_abap_unit_assert=>assert_equals( act = lcl=>square( a = 7 ) exp = 49 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>square( a = 0 ) exp = 0 ).
  ENDMETHOD.
  METHOD test_cube.
    cl_abap_unit_assert=>assert_equals( act = lcl=>cube( a = 3 ) exp = 27 ).
  ENDMETHOD.
  METHOD test_factorial_rec.
    cl_abap_unit_assert=>assert_equals( act = lcl=>factorial_rec( a = 10 ) exp = 3628800 ).
  ENDMETHOD.
  METHOD test_fib_rec.
    cl_abap_unit_assert=>assert_equals( act = lcl=>fib_rec( a = 10 ) exp = 55 ).
  ENDMETHOD.
  METHOD test_gcd.
    cl_abap_unit_assert=>assert_equals( act = lcl=>gcd( a = 12 b = 8 ) exp = 4 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>gcd( a = 100 b = 75 ) exp = 25 ).
    cl_abap_unit_assert=>assert_equals( act = lcl=>gcd( a = 7 b = 13 ) exp = 1 ).
  ENDMETHOD.
ENDCLASS.
