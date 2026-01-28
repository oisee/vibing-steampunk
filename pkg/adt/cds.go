package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
)

// CDSDependencyNode represents a node in the CDS dependency tree
type CDSDependencyNode struct {
	Name                  string              `xml:"name,attr" json:"name"`
	Type                  string              `xml:"type,attr" json:"type"`
	ObjectType            string              `xml:"object_type,attr,omitempty" json:"objectType,omitempty"`
	HasParams             bool                `xml:"has_params,attr,omitempty" json:"hasParams,omitempty"`
	Relation              string              `xml:"relation,attr,omitempty" json:"relation,omitempty"`
	EntityName            string              `xml:"entity_name,attr,omitempty" json:"entityName,omitempty"`
	UserDefinedEntityName string              `xml:"user_defined_entity_name,attr,omitempty" json:"userDefinedEntityName,omitempty"`
	ActivationState       string              `xml:"activation_state,attr,omitempty" json:"activationState,omitempty"`
	DDLSName              string              `xml:"ddls_name,attr,omitempty" json:"ddlsName,omitempty"`
	Children              []CDSDependencyNode `xml:"node" json:"children,omitempty"`
}

// CDSDependencyOptions configures dependency retrieval
type CDSDependencyOptions struct {
	DependencyLevel  string // "unit" or "hierarchy" (default: hierarchy)
	WithAssociations bool   // Include modeled associations (default: false)
	ContextPackage   string // Filter to specific package context (optional)
}

// GetCDSDependencies retrieves CDS view dependency tree
func (c *Client) GetCDSDependencies(ctx context.Context, ddlsName string, opts CDSDependencyOptions) (*CDSDependencyNode, error) {
	if ddlsName == "" {
		return nil, fmt.Errorf("ddlsName is required")
	}

	// Default to hierarchy if not specified
	if opts.DependencyLevel == "" {
		opts.DependencyLevel = "hierarchy"
	}

	// Use /sap/bc/adt/testcodegen/dependencies/doubledata endpoint
	// This returns the table/view dependencies for a CDS view (designed for test doubles)
	// Alternative endpoints like /sap/bc/adt/cds/dependencies don't exist on all systems
	endpoint := fmt.Sprintf("/sap/bc/adt/testcodegen/dependencies/doubledata?ddlsourceName=%s",
		url.QueryEscape(ddlsName))

	resp, err := c.transport.Request(ctx, endpoint, &RequestOptions{
		Method: "GET",
		Accept: "application/vnd.sap.adt.codegen.data.v1+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get CDS dependencies: %w", err)
	}

	// Parse the test double data response
	type cdsDouble struct {
		Name string `xml:"double_name,attr"`
		Type string `xml:"double_type,attr"`
	}
	type cdsUnderTest struct {
		Name    string      `xml:"cds_name,attr"`
		Doubles []cdsDouble `xml:"doublelist>double"`
	}
	type cdsToBeTested struct {
		CDS cdsUnderTest `xml:"cdsundertest"`
	}

	var parsed cdsToBeTested
	if err := xml.Unmarshal(resp.Body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse XML response: %w", err)
	}

	// Convert to CDSDependencyNode structure
	root := &CDSDependencyNode{
		Name:     parsed.CDS.Name,
		Type:     "CDS_VIEW",
		Children: make([]CDSDependencyNode, 0, len(parsed.CDS.Doubles)),
	}

	for _, d := range parsed.CDS.Doubles {
		root.Children = append(root.Children, CDSDependencyNode{
			Name:     d.Name,
			Type:     d.Type,
			Relation: "FROM",
		})
	}

	return root, nil
}

// FlattenDependencies returns a flat list of all dependencies (BFS traversal)
func (node *CDSDependencyNode) FlattenDependencies() []CDSDependencyNode {
	var result []CDSDependencyNode
	queue := []*CDSDependencyNode{node}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		result = append(result, *current)

		for i := range current.Children {
			queue = append(queue, &current.Children[i])
		}
	}

	return result
}

// CountDependenciesByType returns count of dependencies grouped by type
func (node *CDSDependencyNode) CountDependenciesByType() map[string]int {
	counts := make(map[string]int)
	flat := node.FlattenDependencies()

	for _, dep := range flat {
		counts[dep.Type]++
	}

	return counts
}

// FindCycles detects circular dependencies in the tree
func (node *CDSDependencyNode) FindCycles() []string {
	var cycles []string
	visited := make(map[string]bool)
	stack := make(map[string]bool)

	var dfs func(*CDSDependencyNode)
	dfs = func(n *CDSDependencyNode) {
		visited[n.Name] = true
		stack[n.Name] = true

		for i := range n.Children {
			child := &n.Children[i]
			if !visited[child.Name] {
				dfs(child)
			} else if stack[child.Name] {
				cycles = append(cycles, fmt.Sprintf("%s -> %s (circular)", n.Name, child.Name))
			}
		}

		stack[n.Name] = false
	}

	dfs(node)
	return cycles
}

// GetDependencyDepth returns the maximum depth of the dependency tree
func (node *CDSDependencyNode) GetDependencyDepth() int {
	if len(node.Children) == 0 {
		return 1
	}

	maxDepth := 0
	for i := range node.Children {
		depth := node.Children[i].GetDependencyDepth()
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	return maxDepth + 1
}

// FindNodeByName searches for a node by name in the tree
func (node *CDSDependencyNode) FindNodeByName(name string) *CDSDependencyNode {
	if node.Name == name {
		return node
	}

	for i := range node.Children {
		if found := node.Children[i].FindNodeByName(name); found != nil {
			return found
		}
	}

	return nil
}

// GetTableDependencies returns only table dependencies from the tree
func (node *CDSDependencyNode) GetTableDependencies() []CDSDependencyNode {
	var tables []CDSDependencyNode
	flat := node.FlattenDependencies()

	for _, dep := range flat {
		if dep.Type == "TABLE" {
			tables = append(tables, dep)
		}
	}

	return tables
}

// GetInactiveDependencies returns dependencies with INACTIVE or INCONSISTENT state
func (node *CDSDependencyNode) GetInactiveDependencies() []CDSDependencyNode {
	var inactive []CDSDependencyNode
	flat := node.FlattenDependencies()

	for _, dep := range flat {
		if dep.ActivationState == "INACTIVE" || dep.ActivationState == "INCONSISTENT" {
			inactive = append(inactive, dep)
		}
	}

	return inactive
}
