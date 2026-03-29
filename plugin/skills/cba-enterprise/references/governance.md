# CBA Governance & Compliance

## AI Decision Framework

CBA uses a confidence-based governance model for AI-driven development:

| Confidence Level | Action | Gate |
|-----------------|--------|------|
| >95% | Auto-deploy | No human review required |
| 80-95% | Human review | Senior developer approval |
| <80% | Human takeover | AI assists, human drives |

## Mandatory Quality Gates

Before any code enters the transport pipeline:

1. **Syntax Check** — Zero errors required
2. **ABAP Unit Tests** — All tests must pass
3. **ATC Check** — No Priority 1 findings; Priority 2 findings reviewed
4. **Code Review** — Human review for transportable changes
5. **Transport Assignment** — Proper CTS request with description

## Audit Trail Requirements

All VSP operations in CBA environments should produce traceable evidence:
- What was changed (object name, type)
- Who initiated the change (user, agent ID)
- When the change was made (timestamp)
- Why the change was made (linked to feature/incident)
- Test evidence (unit test results, ATC findings)

## Transport Discipline

- All changes flow through CTS (Change and Transport System)
- No direct database modifications
- Transport requests must reference a feature or incident ID
- Release only after all quality gates pass

### Transport Workflow

```
Create Transport Request → Assign Objects → Quality Gates → Release → Import
```

VSP tools involved:
- `CreateTransport` — Create the request
- `ListTransports` — View existing requests
- `GetTransport` — Check request details
- `ReleaseTransport` — Release after quality gates

## Cloud ALM Integration

CBA is migrating from Active Control to SAP Cloud ALM for deployment orchestration:
- Feature tracking in Cloud ALM
- Evidence publication from VSP test results
- Quality approval workflows in Cloud ALM
- Transport scheduling and monitoring

VSP produces the evidence; Cloud ALM manages the pipeline.

## Compliance Patterns

### Authorization
- Always use `AUTHORITY-CHECK OBJECT` before sensitive data access
- Check object `S_TABU_DIS` for table authorization
- Never hardcode user names for authorization bypass

### Error Handling
- Use CBA exception hierarchy (`/CBA/CX_*` exception classes)
- Always provide meaningful error messages
- Log exceptions for audit purposes

### Performance
- Avoid SELECT in LOOP (use SELECT FOR ALL ENTRIES)
- Use secondary indexes for frequently queried fields
- Limit result sets with `max_rows` parameter

## Compliance Examples (Evaluator Calibration)

Use these examples to calibrate quality judgment. When reviewing CBA code, compare against these patterns.

### COMPLIANT Example (Score: 5/5)

```abap
METHOD /cba/if_invoice_proc~validate.
  " Authorization check before data access
  AUTHORITY-CHECK OBJECT 'S_TABU_DIS'
    ID 'DICBERCLS' FIELD lv_auth_group.
  IF sy-subrc <> 0.
    RAISE EXCEPTION TYPE /cba/cx_not_authorized
      EXPORTING textid = /cba/cx_not_authorized=>no_display_auth.
  ENDIF.

  " Bulk read with FOR ALL ENTRIES
  IF lt_invoice_ids IS NOT INITIAL.
    SELECT bukrs, belnr, gjahr, bldat
      FROM bkpf
      FOR ALL ENTRIES IN @lt_invoice_ids
      WHERE belnr = @lt_invoice_ids-belnr
      INTO TABLE @DATA(lt_documents).
  ENDIF.
ENDMETHOD.
```

Why compliant:
- `/CBA/` namespace on class and exception
- AUTHORITY-CHECK before data access
- CBA exception hierarchy (`/CBA/CX_*`)
- Bulk read pattern (FOR ALL ENTRIES, not SELECT in LOOP)
- Meaningful variable names

### NON-COMPLIANT Example (Score: 1/5)

```abap
METHOD validate.
  " Just read everything
  SELECT * FROM bkpf INTO TABLE lt_docs.

  LOOP AT lt_ids INTO ls_id.
    SELECT SINGLE * FROM bseg
      WHERE belnr = ls_id-belnr
      INTO ls_detail.
    APPEND ls_detail TO lt_results.
  ENDLOOP.
ENDMETHOD.
```

Why non-compliant:
- No `/CBA/` namespace
- No authorization check (security violation)
- SELECT * (performance: reads all fields)
- SELECT in LOOP (N+1 query anti-pattern)
- No exception handling
- No meaningful error feedback

### PARTIALLY COMPLIANT Example (Score: 3/5)

```abap
METHOD /cba/cl_invoice_proc=>validate.
  SELECT bukrs, belnr FROM bkpf
    WHERE bukrs = @iv_company_code
    INTO TABLE @DATA(lt_documents).

  IF sy-subrc <> 0.
    RAISE EXCEPTION TYPE cx_sy_no_data.
  ENDIF.
ENDMETHOD.
```

Why partial:
- Correct `/CBA/` namespace (good)
- Targeted field list, not SELECT * (good)
- WHERE clause limits scope (good)
- Missing AUTHORITY-CHECK (security gap)
- Uses generic `cx_sy_no_data` instead of `/CBA/CX_*` exception (governance gap)
- No error message context (audit gap)
