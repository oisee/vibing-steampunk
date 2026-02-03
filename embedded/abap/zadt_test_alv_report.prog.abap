*&---------------------------------------------------------------------*
*& Report ZADT_TEST_ALV_REPORT
*&---------------------------------------------------------------------*
REPORT zadt_test_alv_report.

PARAMETERS: p_rows TYPE i DEFAULT 10,
            p_mult TYPE i DEFAULT 2.

SELECT-OPTIONS: s_id FOR sy-tabix.

TYPES: BEGIN OF ty_data,
         id          TYPE i,
         name        TYPE string,
         value       TYPE p DECIMALS 2,
         created_at  TYPE timestamp,
       END OF ty_data.

DATA: gt_data TYPE TABLE OF ty_data,
      gs_data TYPE ty_data.

START-OF-SELECTION.

  DO p_rows TIMES.
    IF s_id IS INITIAL OR sy-index IN s_id.
      gs_data-id = sy-index.
      gs_data-name = |Test Item { sy-index }|.
      gs_data-value = sy-index * p_mult.
      GET TIME STAMP FIELD gs_data-created_at.
      APPEND gs_data TO gt_data.
    ENDIF.
  ENDDO.

  TRY.
      cl_salv_table=>factory(
        IMPORTING r_salv_table = DATA(lo_alv)
        CHANGING  t_table      = gt_data ).

      DATA(lo_cols) = lo_alv->get_columns( ).
      lo_cols->set_optimize( ).

      lo_alv->display( ).

    CATCH cx_salv_msg INTO DATA(lx_error).
      MESSAGE lx_error TYPE 'E'.
  ENDTRY.
