# RAP / OData E2E

vsp supports full end-to-end creation of RAP (RESTful Application Programming) OData services directly through AI conversation. Create complete OData V2/V4 services from CDS view to published endpoint.

## The full workflow

```
DDLS (CDS view)  →  BDEF (Behavior)  →  SRVD (Service Def)  →  SRVB (Service Binding)  →  Publish
```

All steps can be done with `WriteSource` using the appropriate `type` parameter.

---

## Step 1: Create the CDS View (DDLS)

```
WriteSource(
  type="DDLS",
  name="ZTRAVEL",
  package="$TMP",
  description="Travel Entity",
  source=`
@EndUserText.label: 'Travel'
@AccessControl.authorizationCheck: #NOT_REQUIRED
define root view entity ZTRAVEL as select from ztravel_tab {
  key travel_id   as TravelId,
  description     as Description,
  start_date      as StartDate,
  end_date        as EndDate,
  status          as Status
}
`
)
```

---

## Step 2: Create the Behavior Definition (BDEF)

```
WriteSource(
  type="BDEF",
  name="ZTRAVEL",
  package="$TMP",
  description="Travel Behavior",
  source=`
managed implementation in class ZBP_TRAVEL unique;
strict ( 2 );

define behavior for ZTRAVEL alias Travel
persistent table ztravel_tab
lock master
authorization master ( instance )
{
  field ( readonly ) TravelId;
  field ( mandatory ) Description;

  create;
  update;
  delete;

  mapping for ztravel_tab {
    TravelId    = travel_id;
    Description = description;
    StartDate   = start_date;
    EndDate     = end_date;
    Status      = status;
  }
}
`
)
```

---

## Step 3: Create the Service Definition (SRVD)

```
WriteSource(
  type="SRVD",
  name="ZTRAVEL_SD",
  package="$TMP",
  description="Travel Service Definition",
  source=`
@EndUserText.label: 'Travel Service'
define service ZTRAVEL_SD {
  expose ZTRAVEL;
}
`
)
```

---

## Step 4: Create the Service Binding (SRVB)

```
WriteSource(
  type="SRVB",
  name="ZTRAVEL_SB",
  package="$TMP",
  description="Travel OData V4 Binding",
  service_definition="ZTRAVEL_SD",
  binding_version="V4"
)
```

### Binding options

| Parameter | Values | Description |
|-----------|--------|-------------|
| `binding_version` | `V2`, `V4` | OData protocol version |
| `binding_category` | `0`, `1` | `0` = Web API, `1` = UI (Fiori) |

---

## Step 5: Publish the Service

```
PublishServiceBinding(
  service_name="ZTRAVEL_SB",
  service_version="0001"
)
```

After publishing, the OData service is available at:
```
/sap/opu/odata4/sap/ztravel_sb/srvd/sap/ztravel_sd/0001/
```

---

## Transportable Packages

Add the `transport` parameter to all `WriteSource` calls when working in transportable packages:

```
WriteSource(
  type="DDLS",
  name="ZTRAVEL",
  package="ZPROD",
  transport="DEVK900123",
  source=`...`
)
```

---

## Using the DSL Pipeline

For automated RAP deployments, use the Go DSL:

```go
pipeline := dsl.RAPPipeline(client, "./src/", "$ZRAY", "ZTRAVEL_SB")
```

This handles the correct deployment order: DDLS → BDEF → Classes → SRVD → SRVB.

Or with YAML workflows:

```yaml
steps:
  - action: import
    directory: ./src/
    package: "$ZRAY"
    order: rap   # DDLS → BDEF → Classes → SRVD
```

See [DSL & Workflows](../DSL.md) for complete DSL documentation.

---

## File-based deployment

For large CDS views and behavior definitions that exceed token limits:

```
ImportFromFile(
  file_path="./src/ztravel.ddls.asddls",
  package="$TMP"
)
```

Supported extensions: `.ddls.asddls`, `.bdef.asbdef`, `.srvd.srvdsrv`

---

## Tips

- Always check syntax with `SyntaxCheck` before activating
- Use `GetCDSDependencies` to understand view dependency trees
- Use `RunATCCheck` to verify code quality before transport release
- If activation fails, use `GetInactiveObjects` to see what needs attention
