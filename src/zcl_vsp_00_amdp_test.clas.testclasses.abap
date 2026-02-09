CLASS lcl_test DEFINITION FOR TESTING RISK LEVEL HARMLESS DURATION SHORT.
  PRIVATE SECTION.
    METHODS test_calculate FOR TESTING.
ENDCLASS.

CLASS lcl_test IMPLEMENTATION.
  METHOD test_calculate.
    DATA lt_result TYPE zcl_vsp_00_amdp_test=>tt_result.
    zcl_vsp_00_amdp_test=>calculate_squares(
      EXPORTING iv_count = 5
      IMPORTING et_result = lt_result
    ).
    cl_abap_unit_assert=>assert_equals( exp = 5 act = lines( lt_result ) ).
  ENDMETHOD.
ENDCLASS.
