---
name: deploy
description: Deploy ABAP objects with full quality gates (syntax, activate, test, transport)
allowed-tools: Bash, Read, Grep
argument-hint: <object_name> [--transport <transport_id>]
---

Deploy an ABAP object through the full quality gate pipeline.

## Workflow

1. If no argument provided, ask the user what to deploy
2. Use **GetSource** to verify the object exists and read current state
3. Run **SyntaxCheck** — STOP if errors found, report them
4. Run **Activate** — activate the object
5. Run **RunUnitTests** — STOP if tests fail, report failures
6. Run **RunATCCheck** — report P1/P2 findings as warnings
7. If `--transport` specified or object is in a transportable package:
   - Use **ListTransports** to show available transport requests
   - Confirm with user before adding to transport
   - **gCTS alternative:** if the system uses gCTS instead of classic CTS, the `Gcts*` tool family handles git-based change management — `GctsListRepositories`, `GctsCommit`, `GctsPull`, `GctsSwitchBranch`, etc. Check `GetFeatures` and ask the user which transport flow applies.
8. Report final status: deployed, tested, transport status

## Safety Checks

- If object is in a transportable package and `--allow-transportable-edits` is not set, warn the user
- If `--read-only` mode is active, explain that deployment is blocked
- Never skip syntax check or unit tests

## Example Usage

```
/vsp:deploy ZCL_INVOICE_PROCESSOR
/vsp:deploy /CBA/CL_PAYMENT_HANDLER --transport DEVK900123
```
