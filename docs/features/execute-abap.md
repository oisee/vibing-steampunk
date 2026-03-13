# ExecuteABAP

`ExecuteABAP` lets you run arbitrary ABAP code snippets directly on a connected SAP system — without creating permanent objects. It works by wrapping the code in a dynamically created unit test.

!!! warning "Expert mode only"
    `ExecuteABAP` is only available in expert mode (`--mode expert` or `SAP_MODE=expert`).

---

## Basic Usage

```
ExecuteABAP:
  code: |
    DATA(lv_date) = sy-datum.
    DATA(lv_msg) = |Today is { lv_date }|.
    lv_result = lv_msg.
```

The value assigned to `lv_result` is returned as output.

---

## Risk Levels

| Level | Description | Use case |
|-------|-------------|----------|
| `harmless` | Read-only, no external calls, no DB writes | Reading data, formatting |
| `dangerous` | Can write to DB, call external services | Data manipulation |
| `critical` | Full system access | RFC calls, transport operations |

```
ExecuteABAP:
  code: |
    SELECT SINGLE matnr, maktx FROM mara
      INNER JOIN makt ON mara~matnr = makt~matnr
      WHERE mara~matnr = 'MAT001'
        AND makt~spras = sy-langu
      INTO @DATA(ls_material).
    lv_result = ls_material-maktx.
  risk_level: harmless
```

---

## How it works

1. vsp creates a temporary ABAP program in `$TMP` (e.g. `ZADT_EXEC_TMP`)
2. Your code is wrapped in a test class method
3. `RunUnitTests` is called to execute it
4. The result captured from `lv_result` is returned
5. The temporary program is cleaned up

---

## Examples

### Read a configuration value

```
ExecuteABAP:
  code: |
    DATA lv_val TYPE string.
    CALL FUNCTION 'SUSR_USER_AUTH_FOR_OBJ_GET'
      EXPORTING
        bname  = sy-uname
        objname = 'S_TCODE'
      TABLES
        auth_fieldname_tab = lt_tab
      EXCEPTIONS
        OTHERS = 1.
    lv_result = |sy-subrc: { sy-subrc }|.
  risk_level: harmless
```

### Check system parameters

```
ExecuteABAP:
  code: |
    DATA lv_param TYPE string.
    CALL 'C_SAPGPARAM'
      ID 'NAME'  FIELD 'zcsa/installed_languages'
      ID 'VALUE' FIELD lv_param.
    lv_result = lv_param.
```

### Quick data query with formatting

```
ExecuteABAP:
  code: |
    SELECT COUNT(*) FROM usr02 WHERE bname LIKE 'Z%'
      INTO @DATA(lv_count).
    lv_result = |Z-users: { lv_count }|.
  risk_level: harmless
```

---

## Safety considerations

- `ExecuteABAP` with `critical` risk can perform any operation the logged-in user has authorization for
- Use `--read-only` mode to block `ExecuteABAP` entirely on sensitive systems
- Always specify the lowest adequate risk level
- The tool is disabled in focused mode by default — enable Expert mode explicitly

---

## Limitations

- Code must assign the return value to `lv_result` (type `string`)
- Requires the SAP user to have authorization for `S_DEVELOP` and unit test execution
- Long-running code may time out
- Side effects are real — writes to DB, RFC calls actually execute
