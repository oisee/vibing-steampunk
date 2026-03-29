# CBA Namespace Rules

## The /CBA/ Namespace

CBA uses a registered SAP namespace `/CBA/` for all custom development. This is enforced at the organizational level.

### Configuration

```bash
# VSP configuration for CBA
SAP_NAMESPACE=/CBA/
SAP_PACKAGE=/CBA/VSP

# CLI equivalent
vsp --namespace /CBA/ --package /CBA/VSP --allowed-packages "/CBA/*"
```

### Auto-Prefixing Behavior

When namespace is configured, VSP auto-prefixes object names:
- User provides: `INVOICE_PROCESSOR`
- VSP creates: `/CBA/INVOICE_PROCESSOR`

This applies to all CreateObject operations.

### Validation Rules

1. **Object names** must start with `/CBA/` in CBA systems
2. **Package names** must start with `/CBA/` (except `$TMP` for local testing)
3. **Function groups** follow same convention: `/CBA/FG_NAME`
4. **Message classes** follow same convention: `/CBA/MSG_CLASS`

### Exceptions

- `$TMP` package is allowed for temporary/testing objects
- `$TEST` packages may be used for integration testing
- Z* objects are NOT allowed in CBA production landscapes

### Current VSP Objects (Need Migration)

The ZADT_VSP handler objects currently use Z* namespace:
- `ZCL_VSP_APC_HANDLER` → would become `/CBA/CL_VSP_APC_HANDLER`
- `ZIF_VSP_SERVICE` → would become `/CBA/IF_VSP_SERVICE`

This migration is a future task — current Z* objects work but don't follow CBA convention.
