# VSP Safety Configuration Patterns

## Safety Presets

### Default (Recommended for exploration)
```bash
vsp --read-only --block-free-sql
# Allowed operations: Read, Search, Query, Test, Intelligence
# Operation codes: RSQTI
```

### Development (For active coding in sandbox)
```bash
vsp --allowed-packages '$TMP,$TEST'
# Full operations within sandbox packages only
```

### Enterprise (For CBA/production-adjacent systems)
```bash
vsp --allowed-packages '/CBA/*' \
    --disallowed-ops 'D' \
    --block-free-sql
# Create/update in /CBA/ namespace, no deletes, no free SQL
```

### Unrestricted (Use with extreme caution)
```bash
vsp --allow-transportable-edits
# WARNING: Can modify objects in transportable packages
```

## Operation Type Codes

| Code | Operation | Examples |
|------|-----------|---------|
| R | Read | GetSource, GetObjectStructure |
| S | Search | SearchObject, GrepObjects, GrepPackages |
| Q | Query | RunQuery (ABAP SQL) |
| F | Free SQL | Unrestricted SQL execution |
| C | Create | CreateObject, WriteSource (new) |
| U | Update | WriteSource (existing), EditSource |
| D | Delete | DeleteObject |
| A | Activate | Activate, ActivatePackage |
| T | Test | RunUnitTests, RunATCCheck |
| L | Lock/Unlock | LockObject, UnlockObject |
| I | Intelligence | FindDefinition, FindReferences, CodeCompletion |
| W | Workflow | Higher-level workflow operations |
| X | Transport | CreateTransport, ReleaseTransport |

## Combining Safety Controls

Controls stack — the most restrictive combination wins:

```bash
# Package restriction + operation filtering
vsp --allowed-packages 'Z*' --allowed-ops 'RSCUAT'

# This means: Read, Search, Create, Update, Activate, Test
# ONLY in packages starting with Z
# No deletes, no free SQL, no transport ops
```

## Transportable Edit Protection

By default, VSP blocks edits to objects in transportable packages (anything not `$TMP`). This prevents accidental changes to code that feeds into transport pipelines.

To enable:
```bash
vsp --allow-transportable-edits
```

Always confirm with the user before enabling this.

## When Safety Blocks an Operation

If a write operation fails with a safety message:
1. Explain which safety control blocked it
2. Show the current configuration
3. Suggest the specific flag needed to allow it
4. Let the user decide — never bypass safety silently
