# Safety Controls

vsp includes multiple layers of protection to prevent accidental or unauthorized changes to SAP systems — especially important when AI assistants have write access.

---

## Read-Only Mode

Blocks all write, create, and delete operations. The AI can read, search, and analyze but cannot make any changes.

```bash
# CLI flag
vsp --read-only

# Environment variable
SAP_READ_ONLY=true vsp

# .mcp.json
"env": {
  "SAP_READ_ONLY": "true"
}
```

Ideal for:
- Production system access (analysis only)
- Onboarding / exploratory sessions
- Code review without risk

---

## Package Restrictions

Restrict all write operations to specific packages only. Supports wildcards.

```bash
SAP_ALLOWED_PACKAGES="Z*,$TMP,$*"
```

Examples:
| Pattern | Matches |
|---------|---------|
| `$TMP` | Only the $TMP package |
| `$*` | All local packages |
| `Z*` | All Z-namespace packages |
| `ZPROD,$TMP` | ZPROD and $TMP only |
| `ZDEV*,$*` | ZDEV and all subpackages + all local |

Any write to a package not matching the whitelist is blocked with a clear error.

---

## Transportable Package Edits

By default, editing objects in transportable packages (non-`$` prefixed) is blocked. Enable explicitly when needed:

```bash
SAP_ALLOW_TRANSPORTABLE_EDITS=true
SAP_ALLOWED_TRANSPORTS="DEVK*,A4HK*"
```

This requires explicitly specifying which transport patterns are allowed as an additional safeguard.

---

## Operation Filtering

Fine-grained control using operation type codes.

### Whitelist (allowed ops only)

```bash
SAP_ALLOWED_OPS="RSQ"   # Only Read, Search, Query
```

### Blacklist (block specific ops)

```bash
SAP_DISALLOWED_OPS="CDUA"  # Block Create, Delete, Update, Activate
```

### Operation codes

| Code | Operation |
|------|-----------|
| `R` | Read (GetSource, GetTable, etc.) |
| `S` | Search (SearchObject, GrepPackages, etc.) |
| `Q` | Query (RunQuery, GetTableContents) |
| `C` | Create (CreateObject, CreatePackage, etc.) |
| `U` | Update (WriteSource, EditSource, etc.) |
| `D` | Delete (DeleteObject) |
| `A` | Activate (Activate, ActivatePackage) |
| `T` | Test (RunUnitTests) |
| `X` | Execute (ExecuteABAP) |

---

## Block Free SQL

Block the `RunQuery` tool to prevent arbitrary SQL queries:

```bash
SAP_BLOCK_FREE_SQL=true
```

Useful when you want to allow other tools but prevent uncontrolled data access.

---

## Feature-Based Safety Network

vsp auto-detects system capabilities and adjusts tool availability. If a feature isn't installed, related tools are hidden automatically.

| Feature | Auto-detection | Force override |
|---------|---------------|----------------|
| abapGit | Checks for `ZCL_ABAPGIT_OBJECTS` | `SAP_FEATURE_ABAPGIT=off` |
| RAP/OData | Checks ADT RAP endpoints | `SAP_FEATURE_RAP=off` |
| AMDP Debugger | Checks HANA kernel | `SAP_FEATURE_AMDP=off` |
| UI5/BSP | Checks BSP endpoint | `SAP_FEATURE_UI5=off` |
| CTS Transport | Checks transport API | `SAP_FEATURE_TRANSPORT=off` |

Check detected features:
```
GetFeatures()
```

---

## Recommended configurations

### Production read-only

```json
{
  "env": {
    "SAP_URL": "https://prod:44300",
    "SAP_USER": "MONITOR",
    "SAP_PASSWORD": "...",
    "SAP_READ_ONLY": "true",
    "SAP_BLOCK_FREE_SQL": "true"
  }
}
```

### Development with guardrails

```json
{
  "env": {
    "SAP_URL": "https://dev:44300",
    "SAP_USER": "DEVELOPER",
    "SAP_PASSWORD": "...",
    "SAP_ALLOWED_PACKAGES": "Z*,$TMP,$*",
    "SAP_ALLOW_TRANSPORTABLE_EDITS": "true",
    "SAP_ALLOWED_TRANSPORTS": "DEVK*"
  }
}
```

### Full expert access (trusted environment)

```json
{
  "env": {
    "SAP_URL": "https://sandbox:50000",
    "SAP_USER": "ADMIN",
    "SAP_PASSWORD": "...",
    "SAP_MODE": "expert",
    "SAP_ALLOW_TRANSPORTABLE_EDITS": "true"
  }
}
```
