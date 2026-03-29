# ABAP Object Type Reference

## Full Coverage Matrix

| Type Code | Object Type | GetSource | WriteSource | EditSource | Notes |
|-----------|------------|:---------:|:-----------:|:----------:|-------|
| CLAS | Class | YES | YES | YES | Use method-level ops (95% token savings) |
| PROG | Program/Report | YES | YES | YES | Full source support |
| INTF | Interface | YES | YES | YES | Full source support |
| FUNC | Function Module | YES | YES* | YES* | Requires `parent` (function group name) |
| FUGR | Function Group | YES | NO | NO | Returns JSON metadata only |
| INCL | Include | YES | NO | NO | Read-only |
| DDLS | CDS DDL Source | YES | YES | YES | Data definition language |
| VIEW | Database View | YES | NO | NO | Read-only |
| BDEF | Behavior Definition | YES | YES | YES | RAP behavior |
| SRVD | Service Definition | YES | YES | YES | RAP service |
| SRVB | Service Binding | YES | NO | NO | Metadata only; use PublishServiceBinding |
| MSAG | Message Class | YES | NO | NO | Returns JSON with all messages |

## Class Include Types

When working with classes, specify the include:

| Include Type | Content | When to Use |
|-------------|---------|------------|
| `definitions` | Public/protected/private sections | Reading/editing class interface |
| `implementations` | Method implementations | Reading/editing method code |
| `testclasses` | Local test classes | Reading/editing unit tests |
| `locals_def` | Local type definitions | Helper types for the class |
| `locals_imp` | Local class implementations | Helper class code |
| `macros` | ABAP macros | Rarely used |

## Object Creation Requirements

| Object | Required Parameters | Notes |
|--------|-------------------|-------|
| Class | name, package | Optional: description, transport |
| Program | name, package | Optional: description, transport |
| Interface | name, package | Optional: description, transport |
| Function Module | name, parent (FUGR), package | Parent function group must exist |
| CDS View (DDLS) | name, package | Optional: transport |
| Behavior Def (BDEF) | name, package | Must match CDS view name |
| Service Def (SRVD) | name, package | References BDEF |
| Service Binding (SRVB) | name, package | References SRVD |

## Object Naming Conventions

- **Z*** or **Y***: Customer namespace (development/sandbox)
- **/namespace/**: Registered namespace (e.g., `/CBA/`, `/UI2/`)
- **$TMP**: Local/temporary package (not transportable)
- Test objects: `ZADT_*` or `ZCL_ADT_*` (project convention)
