#!/usr/bin/env bash

cat << 'EOF'
{
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": "VSP MCP tools are available for SAP ABAP development. Key rules: (1) Always use method-level GetSource/WriteSource for classes — specify the method name or include_type. (2) After writing code, ALWAYS re-read it with GetSource to verify what you wrote. Do not trust your memory. (3) Always SyntaxCheck before Activate. Never claim code is correct — run the checks and report actual results. (4) Run GetFeatures to check system capabilities before using advanced features. (5) For complex multi-object changes, define a sprint contract (list of objects, tests, success criteria) before writing code. (6) For CBA environments: enforce /CBA/ namespace, follow transport discipline."
  }
}
EOF

exit 0
