package graph

import (
	"fmt"
	"sort"
	"strings"
)

// ToDOT renders a Graph in Graphviz DOT format.
func ToDOT(g *Graph, title string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("digraph %q {\n", title))
	sb.WriteString("  rankdir=LR;\n")
	sb.WriteString("  node [shape=box, style=filled, fontname=\"Helvetica\"];\n")
	sb.WriteString("  edge [fontname=\"Helvetica\", fontsize=10];\n\n")

	g.mu.RLock()
	defer g.mu.RUnlock()

	mid := func(id string) string {
		r := strings.NewReplacer(":", "_", "/", "_", "$", "_", " ", "_", "-", "_", ".", "_")
		return r.Replace(id)
	}

	// Group by package using subgraph clusters
	byPackage := make(map[string][]*Node)
	for _, n := range g.nodes {
		if strings.HasPrefix(n.ID, "DYNAMIC:") {
			continue
		}
		pkg := n.Package
		if pkg == "" {
			pkg = "_unknown"
		}
		byPackage[pkg] = append(byPackage[pkg], n)
	}

	pkgOrder := make([]string, 0, len(byPackage))
	for pkg := range byPackage {
		pkgOrder = append(pkgOrder, pkg)
	}
	sort.Strings(pkgOrder)

	for _, pkg := range pkgOrder {
		nodes := byPackage[pkg]
		sb.WriteString(fmt.Sprintf("  subgraph cluster_%s {\n", mid(pkg)))
		sb.WriteString(fmt.Sprintf("    label=%q;\n", pkg))
		sb.WriteString("    style=dashed;\n")
		for _, n := range nodes {
			color := nodeColor(n.Type)
			shape := dotShape(n.Type)
			sb.WriteString(fmt.Sprintf("    %s [label=%q, fillcolor=%q, shape=%s];\n",
				mid(n.ID), n.Name, color, shape))
		}
		sb.WriteString("  }\n\n")
	}

	// Edges
	for _, n := range g.nodes {
		edges := g.outEdges[n.ID]
		for _, e := range edges {
			toNode := g.nodes[e.To]
			if toNode == nil {
				continue
			}
			style := "solid"
			color := "gray40"
			switch e.Kind {
			case EdgeCalls:
				color = "blue"
			case EdgeReferences:
				color = "gray60"
				style = "dashed"
			case EdgeDynamic:
				color = "red"
				style = "dotted"
			case EdgeContainsInclude:
				color = "darkgreen"
			}
			label := ""
			if e.RefDetail != "" {
				label = e.RefDetail
			}
			sb.WriteString(fmt.Sprintf("  %s -> %s [color=%s, style=%s, label=%q];\n",
				mid(n.ID), mid(e.To), color, style, label))
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

// ToPlantUML renders a Graph in PlantUML format.
func ToPlantUML(g *Graph, title string) string {
	var sb strings.Builder
	sb.WriteString("@startuml\n")
	sb.WriteString(fmt.Sprintf("title %s\n", title))
	sb.WriteString("left to right direction\n")
	sb.WriteString("skinparam packageStyle rectangle\n")
	sb.WriteString("skinparam linetype ortho\n\n")

	g.mu.RLock()
	defer g.mu.RUnlock()

	mid := func(id string) string {
		r := strings.NewReplacer(":", "_", "/", "_", "$", "P", " ", "_", "-", "_", ".", "_")
		return r.Replace(id)
	}

	// Group by package
	byPackage := make(map[string][]*Node)
	for _, n := range g.nodes {
		if strings.HasPrefix(n.ID, "DYNAMIC:") || IsStandardObject(n.Name) {
			continue
		}
		pkg := n.Package
		if pkg == "" {
			pkg = "_unknown"
		}
		byPackage[pkg] = append(byPackage[pkg], n)
	}

	pkgOrder := make([]string, 0, len(byPackage))
	for pkg := range byPackage {
		pkgOrder = append(pkgOrder, pkg)
	}
	sort.Strings(pkgOrder)

	for _, pkg := range pkgOrder {
		nodes := byPackage[pkg]
		sb.WriteString(fmt.Sprintf("package \"%s\" {\n", pkg))
		for _, n := range nodes {
			pumlType := plantUMLType(n.Type)
			sb.WriteString(fmt.Sprintf("  %s %s as \"%s\"\n", pumlType, mid(n.ID), n.Name))
		}
		sb.WriteString("}\n\n")
	}

	// Edges
	for _, n := range g.nodes {
		if IsStandardObject(n.Name) {
			continue
		}
		edges := g.outEdges[n.ID]
		for _, e := range edges {
			toNode := g.nodes[e.To]
			if toNode == nil || IsStandardObject(toNode.Name) {
				continue
			}
			arrow := "-->"
			color := ""
			switch e.Kind {
			case EdgeCalls:
				color = "[#0066CC]"
			case EdgeReferences:
				arrow = "..>"
				color = "[#999999]"
			case EdgeDynamic:
				arrow = "..>"
				color = "[#CC0000]"
			}
			label := ""
			if e.RefDetail != "" {
				label = " : " + e.RefDetail
			}
			sb.WriteString(fmt.Sprintf("%s %s%s %s%s\n",
				mid(n.ID), arrow, color, mid(e.To), label))
		}
	}

	sb.WriteString("\n@enduml\n")
	return sb.String()
}

// ToGraphML renders a Graph in GraphML format (for Gephi / yEd).
func ToGraphML(g *Graph) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<graphml xmlns="http://graphml.graphstruct.org/graphml"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://graphml.graphstruct.org/graphml">
  <key id="d0" for="node" attr.name="label" attr.type="string"/>
  <key id="d1" for="node" attr.name="type" attr.type="string"/>
  <key id="d2" for="node" attr.name="package" attr.type="string"/>
  <key id="d3" for="edge" attr.name="kind" attr.type="string"/>
  <key id="d4" for="edge" attr.name="detail" attr.type="string"/>
  <graph id="G" edgedefault="directed">
`)

	g.mu.RLock()
	defer g.mu.RUnlock()

	xmlEsc := func(s string) string {
		s = strings.ReplaceAll(s, "&", "&amp;")
		s = strings.ReplaceAll(s, "<", "&lt;")
		s = strings.ReplaceAll(s, ">", "&gt;")
		s = strings.ReplaceAll(s, "\"", "&quot;")
		return s
	}

	mid := func(id string) string {
		return xmlEsc(id)
	}

	for _, n := range g.nodes {
		if strings.HasPrefix(n.ID, "DYNAMIC:") {
			continue
		}
		sb.WriteString(fmt.Sprintf("    <node id=%q>\n", mid(n.ID)))
		sb.WriteString(fmt.Sprintf("      <data key=\"d0\">%s</data>\n", xmlEsc(n.Name)))
		sb.WriteString(fmt.Sprintf("      <data key=\"d1\">%s</data>\n", xmlEsc(n.Type)))
		sb.WriteString(fmt.Sprintf("      <data key=\"d2\">%s</data>\n", xmlEsc(n.Package)))
		sb.WriteString("    </node>\n")
	}

	edgeID := 0
	for _, n := range g.nodes {
		edges := g.outEdges[n.ID]
		for _, e := range edges {
			if g.nodes[e.To] == nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("    <edge id=\"e%d\" source=%q target=%q>\n",
				edgeID, mid(n.ID), mid(e.To)))
			sb.WriteString(fmt.Sprintf("      <data key=\"d3\">%s</data>\n", xmlEsc(string(e.Kind))))
			if e.RefDetail != "" {
				sb.WriteString(fmt.Sprintf("      <data key=\"d4\">%s</data>\n", xmlEsc(e.RefDetail)))
			}
			sb.WriteString("    </edge>\n")
			edgeID++
		}
	}

	sb.WriteString("  </graph>\n</graphml>\n")
	return sb.String()
}

func nodeColor(objType string) string {
	switch strings.ToUpper(objType) {
	case "CLAS":
		return "#4A90D9"
	case "INTF":
		return "#81C784"
	case "FUGR":
		return "#FFB74D"
	case "PROG":
		return "#BA68C8"
	case "TABL", "TTYP":
		return "#90A4AE"
	case "TRAN":
		return "#EF5350"
	case "XSLT":
		return "#26A69A"
	default:
		return "#E0E0E0"
	}
}

func dotShape(objType string) string {
	switch strings.ToUpper(objType) {
	case "INTF":
		return "hexagon"
	case "FUGR":
		return "component"
	case "PROG":
		return "trapezium"
	case "TABL", "TTYP":
		return "cylinder"
	case "TRAN":
		return "house"
	case "XSLT":
		return "diamond"
	default:
		return "box"
	}
}

func plantUMLType(objType string) string {
	switch strings.ToUpper(objType) {
	case "CLAS":
		return "class"
	case "INTF":
		return "interface"
	case "FUGR":
		return "component"
	case "PROG":
		return "entity"
	case "TABL", "TTYP":
		return "storage"
	default:
		return "object"
	}
}
