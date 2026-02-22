---
name: generate-rspdn
description: "Generate RSPDN pre-correction note from a TFS bug number. Finds transports, reads code changes, and produces formatted instructions."
---

# Generate RSPDN

You are the RSPDN generation orchestrator. Generate a pre-correction note for the given bug.

## Usage

```
/generate-rspdn <BUG_NUMBER>
```

## Workflow

1. **Parse input**: Extract the bug number from the argument. If no number provided, ask the user.

2. **Launch rspdn-writer agent**: Use the Task tool to launch the `rspdn-writer` agent with the following prompt:

```
Generate an RSPDN pre-correction note for bug <BUG_NUMBER>.

Follow the full automated workflow:
1. Get bug context from TFS via pdap-docs: get_workitem(<BUG_NUMBER>)
2. Find transports by searching for the bug number in transport descriptions
3. Get objects from each transport
4. Compare versions to detect code changes
5. Search reference RSPDNs for style
6. Generate the RSPDN document
7. Save to R:\RSPDN\<PRODUCT>\RSPDN<BUG_NUMBER>.txt
8. Attach hyperlink to TFS bug

Report the result: file path, web URL, and TFS link status.
```

3. **Report result**: Show the user the generated RSPDN path, web URL, and whether the TFS link was attached.

## Notes

- The rspdn-writer agent has access to vsp-sc3 (SAP) and pdap-docs (TFS/RSPDN) MCP servers
- The agent must run in **foreground** (MCP-dependent)
- If the agent reports errors (no transports found, etc.), relay them to the user
