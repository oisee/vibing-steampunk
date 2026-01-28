"! <p class="shorttext synchronized">VSP ADT Classrun Runner</p>
"! Executes code via ADT classrun endpoint in dialog context.
"! This runs in ICF/dialog work process and WILL hit external breakpoints.
CLASS zcl_vsp_classrun DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    INTERFACES if_oo_adt_classrun.

ENDCLASS.


CLASS zcl_vsp_classrun IMPLEMENTATION.

  METHOD if_oo_adt_classrun~main.
    " This method runs in dialog (ICF) context
    " External breakpoints SHOULD be checked here

    out->write( |=== VSP Classrun Test ===| ).
    out->write( |Timestamp: { sy-datum } { sy-uzeit }| ).
    out->write( |User: { sy-uname }| ).
    out->write( '' ).

    " Simple loop - set breakpoint on line below to test
    DATA lv_counter TYPE i.
    DO 5 TIMES.
      lv_counter = lv_counter + 1.  " <-- SET BREAKPOINT HERE (line ~21)
      out->write( |Loop iteration: { lv_counter }| ).
    ENDDO.

    out->write( '' ).
    out->write( |Execution complete. Counter = { lv_counter }| ).
  ENDMETHOD.

ENDCLASS.
