# abapGit Integration

vsp can export ABAP packages using abapGit's native serialization format via a WebSocket connection to the ZADT_VSP handler. This produces ZIP files compatible with abapGit repositories.

## Prerequisites

- **ZADT_VSP** installed on your SAP system (see `InstallZADTVSP`)
- **abapGit** installed on the SAP system

Install both with:
```
InstallZADTVSP(package="$ZADT_VSP")
InstallAbapGit(edition="standalone", package="$ABAPGIT")
```

---

## Tools

### GitTypes — List supported object types

```
GitTypes()
```

Returns all 158 ABAP object types supported by abapGit's serialization (CLAS, PROG, INTF, FUGR, TABL, VIEW, DDLS, BDEF, SRVD, SRVB, and 148 more).

---

### GitExport — Export packages as ZIP

```
GitExport(
  packages="$ZRAY,$TMP",
  include_subpackages=true
)
```

Returns a base64-encoded ZIP with abapGit-compatible file structure:

```
src/
├── zcl_example.clas.abap      # Class source
├── zcl_example.clas.xml       # Class metadata
├── zif_example.intf.abap      # Interface source
├── zif_example.intf.xml       # Interface metadata
├── ztravel.ddls.asddls        # CDS view source
├── ztravel.ddls.xml           # CDS metadata
└── .abapgit.xml               # abapGit manifest
```

### Export specific objects

```
GitExport(
  objects=[
    {"type": "CLAS", "name": "ZCL_MY_CLASS"},
    {"type": "INTF", "name": "ZIF_MY_INTERFACE"},
    {"type": "DDLS", "name": "ZTRAVEL"}
  ]
)
```

### Save to disk

```
GitExport(
  packages="$ZRAY",
  output_path="./exports/zray-backup.zip"
)
```

---

## CLI Export

```bash
# Export packages to ZIP
vsp -s dev export '$ZORK' '$ZLLM' -o packages.zip

# Export with subpackages
vsp -s dev export '$TMP' --subpackages -o tmp-all.zip
```

---

## Use cases

**Backup before changes:**
```
GitExport(packages="ZPROD", output_path="./backup/pre-change.zip")
```

**CI/CD pipeline export:**
```yaml
# .github/workflows/export.yml
- name: Export ABAP package
  run: vsp -s dev export '$ZPROD' -o release.zip
```

**Review AI changes before committing:**
After an AI session modifies ABAP objects, export to compare with git:
```
GitExport(packages="$TMP", output_path="./review.zip")
```

---

## abapGit Import (not yet supported)

GitImport (deploying a ZIP back to SAP) is not yet implemented. It requires `ZCL_ABAPGIT_OBJECTS=>deserialize` with a virtual repository interface — tracked as a roadmap item.

As a workaround, use `ImportFromFile` for individual files or `dsl.Import()` for batch operations from a directory.

---

## Disable abapGit tools

If abapGit is not installed, disable the Git tool group to avoid tool registration noise:

```bash
vsp --disabled-groups G
# or
SAP_DISABLED_GROUPS=G vsp
```

Or use feature detection — vsp auto-detects abapGit and adjusts tool availability:

```bash
SAP_FEATURE_ABAPGIT=off vsp
```
