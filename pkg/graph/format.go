package graph

import (
	"fmt"
	"strings"
)

// CoChangeToMermaid renders a CoChangeResult as a Mermaid flowchart.
// Target node in center, co-changing objects around it with edge labels showing count.
func CoChangeToMermaid(result *CoChangeResult) string {
	var sb strings.Builder
	sb.WriteString("graph LR\n")

	// Target node (highlighted)
	targetLabel := nodeLabel(result.Target)
	sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", mermaidID("T"), targetLabel))
	sb.WriteString(fmt.Sprintf("    style %s fill:#2d6a4f,color:#fff,stroke:#4ade80,stroke-width:2px\n", mermaidID("T")))

	for i, e := range result.CoChanges {
		nodeID := fmt.Sprintf("N%d", i)
		label := fmt.Sprintf("%s %s", e.Type, e.Name)
		sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", nodeID, label))
		sb.WriteString(fmt.Sprintf("    %s -- \"%dx\" --> %s\n", nodeID, e.Count, mermaidID("T")))
	}

	return sb.String()
}

// ConfigUsageToMermaid renders a ConfigUsageResult as a Mermaid flowchart.
// TVARVC variable in center, reading programs pointing to it.
func ConfigUsageToMermaid(result *ConfigUsageResult) string {
	var sb strings.Builder
	sb.WriteString("graph LR\n")

	// Variable node (highlighted)
	sb.WriteString(fmt.Sprintf("    %s[\"%s\\n(TVARVC)\"]\n", mermaidID("V"), result.VariableName))
	sb.WriteString(fmt.Sprintf("    style %s fill:#7c3aed,color:#fff,stroke:#a78bfa,stroke-width:2px\n", mermaidID("V")))

	for i, r := range result.Readers {
		nodeID := fmt.Sprintf("R%d", i)
		label := fmt.Sprintf("%s %s", r.Type, r.Name)
		// Style based on confidence
		sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", nodeID, label))
		edgeLabel := r.Confidence
		if r.Package != "" {
			edgeLabel += " | " + r.Package
		}
		sb.WriteString(fmt.Sprintf("    %s -- \"%s\" --> %s\n", nodeID, edgeLabel, mermaidID("V")))

		if r.Confidence == "HIGH" {
			sb.WriteString(fmt.Sprintf("    style %s fill:#065f46,color:#fff\n", nodeID))
		} else {
			sb.WriteString(fmt.Sprintf("    style %s fill:#92400e,color:#fff\n", nodeID))
		}
	}

	return sb.String()
}

// WrapMermaidHTML wraps Mermaid diagram text in a minimal self-contained HTML page.
func WrapMermaidHTML(title, mermaidText string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html><head>
<meta charset="utf-8">
<title>%s</title>
<style>body{font-family:sans-serif;margin:2em;background:#1a1a2e;color:#e0e0e0}.mermaid{background:#16213e;padding:1em;border-radius:8px}h1{color:#4ade80;font-size:1.2em}</style>
</head><body>
<h1>%s</h1>
<div class="mermaid">
%s
</div>
<script src="https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.min.js"></script>
<script>mermaid.initialize({startOnLoad:true,theme:'dark'})</script>
</body></html>`, title, title, mermaidText)
}

func mermaidID(prefix string) string {
	return prefix
}

func nodeLabel(nodeID string) string {
	parts := strings.SplitN(nodeID, ":", 2)
	if len(parts) == 2 {
		return parts[0] + " " + parts[1]
	}
	return nodeID
}
