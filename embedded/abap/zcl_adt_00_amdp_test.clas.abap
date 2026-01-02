CLASS zcl_adt_00_amdp_test DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    INTERFACES if_amdp_marker_hdb.

    TYPES: BEGIN OF ty_result,
             id     TYPE i,
             value  TYPE string,
             square TYPE i,
           END OF ty_result,
           tt_result TYPE STANDARD TABLE OF ty_result WITH EMPTY KEY.

    CLASS-METHODS:
      "! Calculate squares using AMDP
      calculate_squares
        IMPORTING VALUE(iv_count) TYPE i
        EXPORTING VALUE(et_result) TYPE tt_result.

  PROTECTED SECTION.
  PRIVATE SECTION.
ENDCLASS.

CLASS zcl_adt_00_amdp_test IMPLEMENTATION.

  METHOD calculate_squares BY DATABASE PROCEDURE FOR HDB
                           LANGUAGE SQLSCRIPT
                           OPTIONS READ-ONLY.
    DECLARE lv_i INTEGER;
    DECLARE lv_square INTEGER;
    DECLARE lv_value NVARCHAR(100);

    -- Initialize result table
    et_result = SELECT 0 AS id, '' AS value, 0 AS square FROM DUMMY WHERE 1 = 0;

    -- Loop and calculate squares (debuggable lines)
    lv_i = 1;
    WHILE lv_i <= :iv_count DO
      lv_square = :lv_i * :lv_i;
      lv_value = 'Square of ' || :lv_i;

      -- Insert into result (breakpoint line)
      et_result = SELECT * FROM :et_result
                  UNION ALL
                  SELECT :lv_i AS id, :lv_value AS value, :lv_square AS square FROM DUMMY;

      lv_i = :lv_i + 1;
    END WHILE;
  ENDMETHOD.

ENDCLASS.