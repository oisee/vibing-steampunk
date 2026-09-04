REPORT zvsp_cluster_fixture.

* Writes the INDX fixtures in this directory: one EXPORT data cluster with
* every elementary type vsp's cluster parser has to name, so the parser is
* tested against a genuine kernel encoding rather than a reverse-engineered
* guess. Run it once (vsp rfc run ZVSP_CLUSTER_FIXTURE --wait 60), then:
*
*   vsp query indx --where "relid = 'ZV'" --fields "srtfd, srtf2, clustr, clustd"
*
* and save each CLUSTD, trimmed to CLUSTR bytes, as indx_compressed.hex
* (VSPFIX) and indx_plain.hex (VSPFIXPLAIN).

TYPES: BEGIN OF ty_all,
         f_char TYPE c LENGTH 10,
         f_numc TYPE n LENGTH 6,
         f_dats TYPE d,
         f_tims TYPE t,
         f_int  TYPE i,
         f_int1 TYPE int1,
         f_int2 TYPE int2,
         f_int8 TYPE int8,
         f_p    TYPE p LENGTH 8 DECIMALS 2,
         f_fltp TYPE f,
         f_raw  TYPE x LENGTH 4,
         f_str  TYPE string,
         f_xstr TYPE xstring,
         f_dec  TYPE decfloat34,
         f_ts   TYPE timestampl,
       END OF ty_all,
       ty_all_t TYPE STANDARD TABLE OF ty_all WITH EMPTY KEY,
       BEGIN OF ty_nested,
         head  TYPE c LENGTH 4,
         inner TYPE ty_all,
         tail  TYPE n LENGTH 2,
       END OF ty_nested.

DATA(ls_all) = VALUE ty_all(
  f_char = 'ABC' f_numc = '42' f_dats = '20260904' f_tims = '123456'
  f_int = -7 f_int1 = 200 f_int2 = -300 f_int8 = 1234567890123
  f_p = '-12345.67' f_fltp = '2.5' f_raw = 'DEADBEEF'
  f_str = 'a string value' f_xstr = 'CAFE' f_dec = '3.14159'
  f_ts = '20260904123456.1234567' ).
DATA(lt_all) = VALUE ty_all_t( ( ls_all ) ( ls_all ) ).
lt_all[ 2 ]-f_char = 'row2'.
lt_all[ 2 ]-f_int = 2.
DATA(ls_nested) = VALUE ty_nested( head = 'HEAD' inner = ls_all tail = '99' ).
DATA(lv_scalar) = 'a bare elementary field'.
DATA(lv_int) = 4711.

EXPORT struct = ls_all
       table = lt_all
       nested = ls_nested
       scalar = lv_scalar
       number = lv_int
       TO DATABASE indx(zv) ID 'VSPFIX'.

EXPORT struct = ls_all
       table = lt_all
       TO DATABASE indx(zv) ID 'VSPFIXPLAIN' COMPRESSION OFF.

COMMIT WORK.
WRITE: / 'exported VSPFIX and VSPFIXPLAIN to INDX(ZV)'.
