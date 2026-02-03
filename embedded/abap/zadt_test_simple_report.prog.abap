REPORT zadt_test_simple_report.

PARAMETERS: p_rows TYPE i DEFAULT 5,
            p_mult TYPE i DEFAULT 2.

TYPES: BEGIN OF ty_data,
         id    TYPE i,
         name  TYPE string,
         value TYPE p DECIMALS 2,
       END OF ty_data.

DATA: gt_data TYPE TABLE OF ty_data,
      gs_data TYPE ty_data.

START-OF-SELECTION.

  DO p_rows TIMES.
    gs_data-id = sy-index.
    gs_data-name = |Item { sy-index }|.
    gs_data-value = sy-index * p_mult.
    APPEND gs_data TO gt_data.
  ENDDO.

  TRY.
      cl_salv_table=>factory(
        IMPORTING r_salv_table = DATA(lo_alv)
        CHANGING  t_table      = gt_data ).
      lo_alv->display( ).
    CATCH cx_salv_msg INTO DATA(lx_error).
      WRITE: / 'Error:', lx_error->get_text( ).
  ENDTRY.
