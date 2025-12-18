INTERFACE zif_vsp_service
  PUBLIC.

  TYPES:
    BEGIN OF ty_message,
      id      TYPE string,
      domain  TYPE string,
      action  TYPE string,
      params  TYPE string,
      timeout TYPE i,
    END OF ty_message,

    BEGIN OF ty_response,
      id      TYPE string,
      success TYPE abap_bool,
      data    TYPE string,
      error   TYPE string,
    END OF ty_response.

  METHODS get_domain
    RETURNING VALUE(rv_domain) TYPE string.

  METHODS handle_message
    IMPORTING iv_session_id      TYPE string
              is_message         TYPE ty_message
    RETURNING VALUE(rs_response) TYPE ty_response.

  METHODS on_disconnect
    IMPORTING iv_session_id TYPE string.

ENDINTERFACE.
