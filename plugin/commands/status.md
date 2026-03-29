---
name: status
description: Show VSP system info, available features, and dependencies
allowed-tools: Bash, Read
---

Display a comprehensive status report of the connected SAP system.

## Workflow

1. Run **GetSystemInfo** — show system ID, release, kernel version, database
2. Run **GetFeatures** — show which features are available:
   - HANA, abapGit, RAP, AMDP, UI5, Transport
   - For each: Available (yes/no), Mode (auto/on/off)
3. Run **ListDependencies** — show ZADT_VSP installation status
4. Report the current VSP mode (focused/expert) and any safety restrictions

## Output Format

```
System: <SID> (<release>) on <database>
Mode: focused (81 tools) | expert (122 tools)
Safety: read-only | restricted to <packages> | unrestricted

Features:
  HANA:      available | not available
  abapGit:   available | not available
  RAP:       available | not available
  Transport: available | not available
  UI5:       available | not available
  AMDP:      available | not available

ZADT_VSP: installed | not installed
  WebSocket debugging: available | requires ZADT_VSP
  Report execution:    available | requires ZADT_VSP
  RFC calls:           available | requires ZADT_VSP
```

## Example Usage

```
/vsp:status
```
