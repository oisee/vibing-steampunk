---
name: cba-enterprise
description: CBA (Commonwealth Bank of Australia) enterprise deployment context for VSP. Use when working in CBA SAP environments, with /CBA/ namespace objects, when governance or compliance is relevant, or when discussing Project Coral, Cloud ALM, or enterprise AI deployment patterns.
---

# CBA Enterprise Context

## Namespace Enforcement

CBA uses the `/CBA/` registered namespace — NOT `Z*` customer namespace.

**Rules:**
- All custom objects MUST use `/CBA/` prefix (e.g., `/CBA/CL_INVOICE_PROC`)
- Default package: `/CBA/VSP` or configured CBA package
- Configure VSP: `--namespace /CBA/ --allowed-packages "/CBA/*"`
- Never create Z* objects in CBA systems unless explicitly in `$TMP` for testing

See `references/namespace-rules.md` for enforcement details.

## Safety-First Defaults

CBA environments require strict safety configuration:

```bash
vsp --namespace /CBA/ \
    --allowed-packages "/CBA/*" \
    --disallowed-ops "D" \
    --block-free-sql \
    --allow-transportable-edits  # Only with transport workflow
```

- No deletes without explicit approval
- No free SQL execution (use parameterized queries)
- Package-restricted to `/CBA/` namespace
- Transportable edits only through proper CTS workflow

## Just-In-Time SKILLS Pattern

CBA follows a just-in-time context retrieval pattern — query information only when needed, never load everything upfront. This reduces token usage by 99.2% (1,600 vs 200,000 tokens).

**Pattern:** Instead of injecting all CBA documentation into context, query specific topics on demand:
- Need authorization patterns? → Query CBA guidelines for "authorization"
- Need code examples? → Query CBA examples for the specific pattern
- Need compliance validation? → Validate against CBA standards

See `references/skills-pattern.md` for the full SKILLS catalog.

## Project Coral Integration

CBA's Project Coral is an enterprise autonomous AI initiative:
- 7,800+ engineers using AI-assisted development
- 55 million AI decisions daily
- Strategic partnerships with Anthropic and OpenAI
- VSP positions as the SAP execution adapter for Coral agents

**VSP's role in Coral:** Execution layer for SAP operations. Coral agents orchestrate intent; VSP executes ABAP development tasks (create, test, deploy, debug).

## Deployment Pipeline

CBA is migrating from Active Control to SAP Cloud ALM:

```
Code Generation (VSP) → Quality Checks (VSP) → Unit Tests (VSP)
    → Evidence Bundle → Cloud ALM Ingestion → Transport (CTS/CTS+)
```

VSP is the execution layer, NOT the orchestration layer. It produces evidence bundles (test results, ATC findings) that feed into the deployment pipeline.

## Governance Requirements

- **Audit logging**: All operations should be traceable
- **Confidence scoring**: Auto-deploy at >95%, human review at 80-95%, human takeover at <80%
- **Transport discipline**: All changes via CTS — no direct database modifications
- **Code review gates**: ATC checks mandatory before transport release

See `references/governance.md` for compliance details.
