# Transport Management

vsp can manage CTS (Change and Transport System) transport requests. Transport tools are disabled by default and require explicit opt-in.

!!! note "Expert mode only"
    Transport tools are only available in expert mode and require `--enable-transports`.

---

## Enabling Transport Tools

```bash
# CLI
vsp --mode expert --enable-transports

# Environment variables
SAP_MODE=expert
SAP_ENABLE_TRANSPORTS=true

# .mcp.json
"env": {
  "SAP_MODE": "expert",
  "SAP_ENABLE_TRANSPORTS": "true",
  "SAP_ALLOWED_TRANSPORTS": "DEVK*"
}
```

---

## Transport Tools

### ListTransports

List transport requests. Can filter by owner, status, or package.

```
ListTransports(owner="DEVELOPER", status="D")  # D=Modifiable, R=Released
```

### GetTransport

Get transport details: description, owner, objects list, status.

```
GetTransport(transport="DEVK900123")
```

### CreateTransport

Create a new transport request.

```
CreateTransport(
  description="Feature: Add ZCL_MY_CLASS",
  category="W"   # W=Workbench, C=Customizing
)
```

### ReleaseTransport

Release a transport request for export.

```
ReleaseTransport(transport="DEVK900123")
```

!!! warning
    Releasing a transport is irreversible. The AI will confirm before proceeding.

### DeleteTransport

Delete a transport request (only modifiable ones can be deleted).

```
DeleteTransport(transport="DEVK900123")
```

---

## Safety Controls for Transports

### Whitelist allowed transports

Only allow operations on specific transport patterns:

```bash
SAP_ALLOWED_TRANSPORTS="DEVK*,A4HK*"
```

### Read-only transport access

Allow reading transport info but block create/release/delete:

```bash
SAP_TRANSPORT_READ_ONLY=true
```

### Required for transportable edits

To create objects in transportable packages (non-`$` prefix), both settings are needed:

```bash
SAP_ALLOW_TRANSPORTABLE_EDITS=true
SAP_ALLOWED_TRANSPORTS="DEVK*"
```

---

## Workflow example

```
1. CreateTransport(description="Add travel entity")
   → DEVK900456

2. WriteSource(type="DDLS", name="ZTRAVEL", package="ZPROD",
               transport="DEVK900456", source=`...`)

3. WriteSource(type="BDEF", name="ZTRAVEL", package="ZPROD",
               transport="DEVK900456", source=`...`)

4. RunATCCheck(object_type="DDLS", object_name="ZTRAVEL")
   → No errors found

5. ReleaseTransport(transport="DEVK900456")
   → Transport released for export
```

---

## Feature detection

vsp auto-detects transport management availability:

```
GetFeatures()
→ { "transport": true, "transport_version": "740" }
```

To force-disable transport tools (e.g. on systems without proper CTS setup):

```bash
SAP_FEATURE_TRANSPORT=off
```
