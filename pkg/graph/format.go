package graph

import (
	"fmt"
	"sort"
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

// GraphToMermaid renders a Graph as a Mermaid flowchart with:
// - Subgraph grouping by package
// - Node shapes by object type (CLAS=rect, INTF=hexagon, FUGR=stadium, PROG=trapezoid)
// - Edge colors by kind (CALLS=blue, REFERENCES=gray, DYNAMIC=red dashed)
// If scope is provided, only nodes in scope are rendered; external targets shown without subgraph.
func GraphToMermaid(g *Graph, scope *PackageScope) string {
	var sb strings.Builder
	sb.WriteString("graph LR\n")

	g.mu.RLock()
	defer g.mu.RUnlock()

	// Group nodes by package
	byPackage := make(map[string][]*Node)
	var externalNodes []*Node
	for _, n := range g.nodes {
		if strings.HasPrefix(n.ID, "DYNAMIC:") {
			continue
		}
		pkg := n.Package
		if pkg == "" {
			pkg = "(unknown)"
		}
		if scope != nil && !scope.InScope(pkg) && !IsStandardObject(n.Name) {
			externalNodes = append(externalNodes, n)
			continue
		}
		if IsStandardObject(n.Name) {
			continue // skip standard SAP in diagram
		}
		byPackage[pkg] = append(byPackage[pkg], n)
	}

	// Sanitize ID for mermaid (replace special chars)
	mid := func(id string) string {
		r := strings.NewReplacer(":", "_", "/", "_", "$", "_", " ", "_", "-", "_")
		return r.Replace(id)
	}

	// Node shape by type
	nodeDecl := func(n *Node) string {
		label := n.Name
		id := mid(n.ID)
		switch strings.ToUpper(n.Type) {
		case "INTF":
			return fmt.Sprintf("    %s{{\"%s\"}}", id, label) // hexagon
		case "FUGR":
			return fmt.Sprintf("    %s([\"%s\"])", id, label) // stadium
		case "PROG":
			return fmt.Sprintf("    %s[\\\"%s\"/]", id, label) // trapezoid
		case "TABL", "TTYP", "DTEL", "DOMA":
			return fmt.Sprintf("    %s[(\"%s\")]", id, label) // cylinder
		case "TRAN":
			return fmt.Sprintf("    %s>\"%s\"]", id, label) // flag/asymmetric
		case "XSLT":
			return fmt.Sprintf("    %s{{\"%s\"}}", id, label) // hexagon (like INTF)
		default: // CLAS and others
			return fmt.Sprintf("    %s[\"%s\"]", id, label) // rectangle
		}
	}

	// Render subgraphs by package
	pkgOrder := make([]string, 0, len(byPackage))
	for pkg := range byPackage {
		pkgOrder = append(pkgOrder, pkg)
	}
	sort.Strings(pkgOrder)

	for _, pkg := range pkgOrder {
		nodes := byPackage[pkg]
		sb.WriteString(fmt.Sprintf("    subgraph %s[\"%s\"]\n", mid(pkg), pkg))
		for _, n := range nodes {
			sb.WriteString(nodeDecl(n) + "\n")
		}
		sb.WriteString("    end\n")
	}

	// External nodes (no subgraph)
	if len(externalNodes) > 0 {
		sb.WriteString("    subgraph _EXT[\"External\"]\n")
		for _, n := range externalNodes {
			sb.WriteString(nodeDecl(n) + "\n")
		}
		sb.WriteString("    end\n")
		sb.WriteString("    style _EXT fill:#3a0a0a,color:#fca5a5,stroke:#ef4444\n")
	}

	// Render edges with style by kind
	edgeIdx := 0
	type edgeStyle struct {
		stroke string
		dash   bool
	}
	edgeStyles := make(map[int]edgeStyle)

	for _, n := range g.nodes {
		if IsStandardObject(n.Name) || strings.HasPrefix(n.ID, "DYNAMIC:") {
			continue
		}
		edges := g.outEdges[n.ID]
		for _, e := range edges {
			toNode := g.nodes[e.To]
			if toNode == nil || IsStandardObject(toNode.Name) {
				continue
			}
			fromID := mid(n.ID)
			toID := mid(e.To)

			switch e.Kind {
			case EdgeCalls:
				label := ""
				if e.RefDetail != "" {
					label = e.RefDetail
				}
				if label != "" {
					sb.WriteString(fmt.Sprintf("    %s -->|\"%s\"| %s\n", fromID, label, toID))
				} else {
					sb.WriteString(fmt.Sprintf("    %s --> %s\n", fromID, toID))
				}
				edgeStyles[edgeIdx] = edgeStyle{stroke: "#3b82f6"} // blue
			case EdgeDynamic:
				sb.WriteString(fmt.Sprintf("    %s -.->|\"dynamic\"| %s\n", fromID, toID))
				edgeStyles[edgeIdx] = edgeStyle{stroke: "#ef4444", dash: true} // red dashed
			default: // REFERENCES
				label := ""
				if e.RefDetail != "" {
					label = e.RefDetail
				}
				if label != "" {
					sb.WriteString(fmt.Sprintf("    %s -.->|\"%s\"| %s\n", fromID, label, toID))
				} else {
					sb.WriteString(fmt.Sprintf("    %s -.-> %s\n", fromID, toID))
				}
				edgeStyles[edgeIdx] = edgeStyle{stroke: "#9ca3af"} // gray
			}
			edgeIdx++
		}
	}

	// Apply edge styles
	for idx, s := range edgeStyles {
		if s.dash {
			sb.WriteString(fmt.Sprintf("    linkStyle %d stroke:%s,stroke-dasharray:5 5\n", idx, s.stroke))
		} else {
			sb.WriteString(fmt.Sprintf("    linkStyle %d stroke:%s,stroke-width:2px\n", idx, s.stroke))
		}
	}

	return sb.String()
}

// CrossingToMermaid renders a CrossingReport as a Mermaid flowchart.
// Groups nodes by package, colors edges by crossing direction.
func CrossingToMermaid(report *CrossingReport, scope *PackageScope) string {
	var sb strings.Builder
	sb.WriteString("graph LR\n")

	mid := func(s string) string {
		r := strings.NewReplacer(":", "_", "/", "_", "$", "_", " ", "_", "-", "_")
		return r.Replace(s)
	}

	// Collect all unique nodes grouped by package
	type nodeInfo struct{ objType, name string }
	byPackage := make(map[string]map[string]nodeInfo) // pkg → id → info

	for _, e := range report.Entries {
		srcID := e.SourceType + "_" + e.SourceObject
		tgtID := e.TargetType + "_" + e.TargetObject
		srcPkg := e.SourcePackage
		tgtPkg := e.TargetPackage
		if srcPkg == "" {
			srcPkg = "(unknown)"
		}
		if tgtPkg == "" {
			tgtPkg = "(unknown)"
		}
		if byPackage[srcPkg] == nil {
			byPackage[srcPkg] = make(map[string]nodeInfo)
		}
		if byPackage[tgtPkg] == nil {
			byPackage[tgtPkg] = make(map[string]nodeInfo)
		}
		byPackage[srcPkg][srcID] = nodeInfo{e.SourceType, e.SourceObject}
		byPackage[tgtPkg][tgtID] = nodeInfo{e.TargetType, e.TargetObject}
	}

	// Render subgraphs
	pkgOrder := make([]string, 0, len(byPackage))
	for pkg := range byPackage {
		pkgOrder = append(pkgOrder, pkg)
	}
	sort.Strings(pkgOrder)

	// Determine which packages are in scope
	for _, pkg := range pkgOrder {
		nodes := byPackage[pkg]
		inScope := scope != nil && scope.InScope(pkg)
		subID := mid(pkg)
		sb.WriteString(fmt.Sprintf("    subgraph %s[\"%s\"]\n", subID, pkg))
		for id, info := range nodes {
			sb.WriteString(fmt.Sprintf("        %s[\"%s\"]\n", mid(id), info.name))
		}
		sb.WriteString("    end\n")
		if !inScope {
			sb.WriteString(fmt.Sprintf("    style %s fill:#3a0a0a,color:#fca5a5,stroke:#ef4444\n", subID))
		}
	}

	// Render edges colored by direction
	edgeIdx := 0
	type edgeStyle struct{ stroke string }
	styles := make(map[int]edgeStyle)

	for _, e := range report.Entries {
		fromID := mid(e.SourceType + "_" + e.SourceObject)
		toID := mid(e.TargetType + "_" + e.TargetObject)
		label := string(e.Direction)
		if e.RefDetail != "" {
			label = e.RefDetail
		}
		sb.WriteString(fmt.Sprintf("    %s -->|\"%s\"| %s\n", fromID, label, toID))

		switch e.Direction {
		case CrossSibling:
			styles[edgeIdx] = edgeStyle{"#f59e0b"} // amber
		case CrossDownward, CrossCommonDown:
			styles[edgeIdx] = edgeStyle{"#ef4444"} // red
		case CrossExternal:
			styles[edgeIdx] = edgeStyle{"#f97316"} // orange
		case CrossUpward:
			styles[edgeIdx] = edgeStyle{"#22c55e"} // green
		case CrossCommon:
			styles[edgeIdx] = edgeStyle{"#3b82f6"} // blue
		default:
			styles[edgeIdx] = edgeStyle{"#9ca3af"} // gray
		}
		edgeIdx++
	}

	for idx, s := range styles {
		sb.WriteString(fmt.Sprintf("    linkStyle %d stroke:%s,stroke-width:2px\n", idx, s.stroke))
	}

	return sb.String()
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
