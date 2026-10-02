INTERFACE zif_jseval PUBLIC.

  " Value types: 0=undefined, 1=number, 2=string, 3=bool,
  "              4=function, 5=null, 6=object, 7=array
  TYPES:
    BEGIN OF ty_value,
      type TYPE i,
      num  TYPE f,
      str  TYPE string,
      obj  TYPE REF TO zcl_jseval_obj,
      arr  TYPE REF TO zcl_jseval_arr,
      fn   TYPE REF TO data,
    END OF ty_value.

  " Token: 0=number, 1=string, 2=ident, 3=op, 4=punc, 5=eof
  TYPES:
    BEGIN OF ty_token,
      kind TYPE i,
      val  TYPE string,
    END OF ty_token,
    tt_tokens TYPE STANDARD TABLE OF ty_token WITH DEFAULT KEY.

  " Node references table (for RETURNING/IMPORTING parameters)
  TYPES tt_nodes TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY.

  " Switch case
  TYPES:
    BEGIN OF ty_switch_case,
      expr TYPE REF TO data,
      body TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY,
    END OF ty_switch_case,
    tt_switch_cases TYPE STANDARD TABLE OF ty_switch_case WITH DEFAULT KEY.

  " Class method
  TYPES:
    BEGIN OF ty_class_method,
      name    TYPE string,
      params  TYPE STANDARD TABLE OF string WITH DEFAULT KEY,
      body    TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY,
      is_ctor TYPE abap_bool,
    END OF ty_class_method,
    tt_class_methods TYPE STANDARD TABLE OF ty_class_method WITH DEFAULT KEY.

  " Node kinds
  CONSTANTS:
    c_node_number        TYPE i VALUE 0,
    c_node_string        TYPE i VALUE 1,
    c_node_ident         TYPE i VALUE 2,
    c_node_binop         TYPE i VALUE 3,
    c_node_unaryop       TYPE i VALUE 4,
    c_node_assign        TYPE i VALUE 5,
    c_node_var           TYPE i VALUE 6,
    c_node_if            TYPE i VALUE 7,
    c_node_while         TYPE i VALUE 8,
    c_node_block         TYPE i VALUE 9,
    c_node_call          TYPE i VALUE 10,
    c_node_func_decl     TYPE i VALUE 11,
    c_node_return        TYPE i VALUE 12,
    c_node_object        TYPE i VALUE 13,
    c_node_array         TYPE i VALUE 14,
    c_node_member_access TYPE i VALUE 15,
    c_node_member_assign TYPE i VALUE 16,
    c_node_method_call   TYPE i VALUE 17,
    c_node_for           TYPE i VALUE 18,
    c_node_switch        TYPE i VALUE 19,
    c_node_typeof        TYPE i VALUE 20,
    c_node_new           TYPE i VALUE 21,
    c_node_class         TYPE i VALUE 22,
    c_node_break         TYPE i VALUE 23,
    c_node_continue      TYPE i VALUE 24.

  " AST Node
  TYPES:
    BEGIN OF ty_node,
      kind      TYPE i,
      num       TYPE f,
      str       TYPE string,
      op        TYPE string,
      left      TYPE REF TO data,
      right     TYPE REF TO data,
      args      TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY,
      body      TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY,
      params    TYPE STANDARD TABLE OF string WITH DEFAULT KEY,
      cond      TYPE REF TO data,
      els       TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY,
      init      TYPE REF TO data,
      update    TYPE REF TO data,
      object    TYPE REF TO data,
      property  TYPE string,
      prop_expr TYPE REF TO data,
      cases     TYPE tt_switch_cases,
      methods   TYPE tt_class_methods,
    END OF ty_node.

  " Function (closure is REF TO object to break circular dep with zcl_jseval_env)
  TYPES:
    BEGIN OF ty_function,
      name    TYPE string,
      params  TYPE STANDARD TABLE OF string WITH DEFAULT KEY,
      body    TYPE STANDARD TABLE OF REF TO data WITH DEFAULT KEY,
      closure TYPE REF TO object,
    END OF ty_function.

ENDINTERFACE.
