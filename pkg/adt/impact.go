package adt

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Pre-compiled regexes (compiled once at package init).
var (
	reSQLSanitize    = regexp.MustCompile(`[^a-zA-Z0-9/_]`)
	reUserExitCall   = regexp.MustCompile(`(?i)CALL\s+FUNCTION\s+['"]EXIT_`)
	reCrossProgramFM = regexp.MustCompile(`(?i)PERFORM\s+\w+.*IN\s+PROGRAM`)
	reBAdIInterface  = regexp.MustCompile(`(?i)IF_EX_\w+|IF_BADI_\w+`)
)

// ImpactAnalysisOptions configures which layers to execute.
type ImpactAnalysisOptions struct {
	StaticRefs      bool     // Layer 1 (default: true)
	Transitive      bool     // Layer 2 (default: false)
	MaxDepth        int      // Layer 2 depth (default: 3)
	DynamicPatterns bool     // Layer 3 (default: false)
	ExtensionPoints bool     // Layer 4 (default: false)
	MaxResults      int      // Cap (default: 200)
	ScopePackages   []string // Scope for Layer 3-4 searches
}

// ImpactAnalysisResult is the full impact analysis output.
type ImpactAnalysisResult struct {
	ObjectURI         string            `json:"objectUri"`
	ObjectName        string            `json:"objectName,omitempty"`
	DirectConsumers   []ImpactedObject  `json:"directConsumers,omitempty"`
	TransitiveCallers []ImpactedObject  `json:"transitiveCallers,omitempty"`
	DynamicCallRisks  []DynamicCallRisk `json:"dynamicCallRisks,omitempty"`
	ConfigCallRisks   []ConfigCallRisk  `json:"configCallRisks,omitempty"`
	Summary           ImpactSummary     `json:"summary"`
	Layers            []string          `json:"layersExecuted"`
	Warnings          []string          `json:"warnings,omitempty"`
}

// ImpactedObject represents an object affected by a change.
type ImpactedObject struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Package     string `json:"package,omitempty"`
	Description string `json:"description,omitempty"`
	Depth       int    `json:"depth,omitempty"` // hops from changed object (Layer 2)
}

// DynamicCallRisk represents a potential dynamic call reference to the target.
type DynamicCallRisk struct {
	ObjectURI  string `json:"objectUri"`
	ObjectName string `json:"objectName"`
	ObjectType string `json:"objectType,omitempty"`
	Package    string `json:"package,omitempty"`
	Pattern    string `json:"pattern"`
	MatchLine  string `json:"matchLine,omitempty"`
	LineNumber int    `json:"lineNumber,omitempty"`
	RiskLevel  string `json:"riskLevel"` // "high", "medium"
}

// ConfigCallRisk represents a configuration-driven call pattern.
type ConfigCallRisk struct {
	Type        string `json:"type"`        // "badi", "enhancement", "user_exit", "nace", etc.
	Name        string `json:"name"`        // BAdI name, enhancement spot, etc.
	ObjectURI   string `json:"objectUri,omitempty"`
	ObjectName  string `json:"objectName,omitempty"`
	Description string `json:"description"`
	Source      string `json:"source"` // "source_analysis" or "table_query"
}

// ImpactSummary aggregates the analysis results.
type ImpactSummary struct {
	DirectConsumerCount  int    `json:"directConsumerCount"`
	TransitiveCallerCount int   `json:"transitiveCallerCount"`
	DynamicRiskCount     int    `json:"dynamicRiskCount"`
	ConfigRiskCount      int    `json:"configRiskCount"`
	TotalAffected        int    `json:"totalAffected"`
	RiskLevel            string `json:"riskLevel"` // "low", "medium", "high", "critical"
	Truncated            bool   `json:"truncated,omitempty"`
}

// CalculateRiskLevel determines the overall risk level from counts.
func CalculateRiskLevel(dynamicRisks, configRisks, totalAffected int) string {
	if dynamicRisks > 0 || configRisks > 0 {
		return "critical" // unknown blast radius
	}
	if totalAffected > 50 {
		return "high"
	}
	if totalAffected > 10 {
		return "medium"
	}
	return "low"
}

// GetImpactAnalysis performs multi-layer impact analysis for a given object.
func (c *Client) GetImpactAnalysis(ctx context.Context, objectURI string, objectName string, opts ImpactAnalysisOptions) (*ImpactAnalysisResult, error) {
	if err := c.checkSafety(OpRead, "GetImpactAnalysis"); err != nil {
		return nil, err
	}

	if objectURI == "" {
		return nil, fmt.Errorf("object_uri is required")
	}

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if opts.MaxResults <= 0 {
		opts.MaxResults = 200
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 3
	}

	result := &ImpactAnalysisResult{
		ObjectURI:  objectURI,
		ObjectName: objectName,
	}

	// Track visited URIs for deduplication across layers
	visited := make(map[string]bool)

	// Layer 1: Static References (always on unless explicitly disabled)
	if opts.StaticRefs {
		result.Layers = append(result.Layers, "static_references")
		refs, err := c.FindReferences(ctx, objectURI, 0, 0)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Layer 1 (static refs) failed: %v", err))
		} else {
			for _, ref := range refs {
				if ref.URI == "" || visited[ref.URI] {
					continue
				}
				visited[ref.URI] = true
				result.DirectConsumers = append(result.DirectConsumers, ImpactedObject{
					URI:         ref.URI,
					Name:        ref.Name,
					Type:        ref.Type,
					Package:     ref.PackageName,
					Description: ref.Description,
					Depth:       1,
				})
				if len(result.DirectConsumers) >= opts.MaxResults {
					break
				}
			}
		}
	}

	// Layer 2: Transitive Callers (opt-in)
	if opts.Transitive {
		result.Layers = append(result.Layers, "transitive_callers")
		callGraph, err := c.GetCallersOf(ctx, objectURI, opts.MaxDepth)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Layer 2 (transitive callers) failed: %v", err))
		} else if callGraph != nil {
			edges := FlattenCallGraph(callGraph)
			for _, edge := range edges {
				if edge.CallerURI == "" || visited[edge.CallerURI] {
					continue
				}
				visited[edge.CallerURI] = true
				result.TransitiveCallers = append(result.TransitiveCallers, ImpactedObject{
					URI:   edge.CallerURI,
					Name:  edge.CallerName,
					Depth: 2, // simplified depth — actual depth requires BFS from root
				})
				if len(result.DirectConsumers)+len(result.TransitiveCallers) >= opts.MaxResults {
					break
				}
			}
			// Stable sort by URI for deterministic output
			sort.Slice(result.TransitiveCallers, func(i, j int) bool {
				return result.TransitiveCallers[i].URI < result.TransitiveCallers[j].URI
			})
		}
	}

	// Layer 3: Dynamic Call Patterns (opt-in, needs scope)
	if opts.DynamicPatterns {
		result.Layers = append(result.Layers, "dynamic_patterns")
		if len(opts.ScopePackages) == 0 {
			result.Warnings = append(result.Warnings, "Layer 3 skipped: specify scope_packages for dynamic call detection")
		} else if objectName != "" {
			risks, err := c.searchDynamicPatterns(ctx, objectName, opts.ScopePackages)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Layer 3 partial: %v", err))
			}
			result.DynamicCallRisks = risks
		}
	}

	// Layer 4: Config-Driven Calls (opt-in)
	if opts.ExtensionPoints {
		result.Layers = append(result.Layers, "extension_points")

		// 4a: Source analysis of the target object
		if objectURI != "" {
			source, err := c.getSourceForAnalysis(ctx, objectURI)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Layer 4a (source analysis) failed: %v", err))
			} else if source != "" {
				configRisks := DetectConfigPatterns(source, objectName)
				result.ConfigCallRisks = append(result.ConfigCallRisks, configRisks...)
			}
		}

		// 4b: Table queries (optional, graceful degradation)
		if len(opts.ScopePackages) > 0 && objectName != "" {
			tableRisks := c.queryConfigTables(ctx, objectName)
			result.ConfigCallRisks = append(result.ConfigCallRisks, tableRisks...)
		}
	}

	// Build summary
	total := len(result.DirectConsumers) + len(result.TransitiveCallers)
	truncated := total >= opts.MaxResults
	result.Summary = ImpactSummary{
		DirectConsumerCount:   len(result.DirectConsumers),
		TransitiveCallerCount: len(result.TransitiveCallers),
		DynamicRiskCount:      len(result.DynamicCallRisks),
		ConfigRiskCount:       len(result.ConfigCallRisks),
		TotalAffected:         total,
		RiskLevel:             CalculateRiskLevel(len(result.DynamicCallRisks), len(result.ConfigCallRisks), total),
		Truncated:             truncated,
	}

	return result, nil
}

// searchDynamicPatterns searches for the target object name as a string literal in scope packages.
func (c *Client) searchDynamicPatterns(ctx context.Context, objectName string, packages []string) ([]DynamicCallRisk, error) {
	var risks []DynamicCallRisk

	// Search for object name as string literal in scope packages
	grepResult, err := c.GrepPackages(ctx, packages, true, objectName, true, nil, 50)
	if err != nil {
		return risks, fmt.Errorf("GrepPackages failed: %w", err)
	}

	for _, obj := range grepResult.Objects {
		for _, match := range obj.Matches {
			risks = append(risks, DynamicCallRisk{
				ObjectURI:  obj.ObjectURL,
				ObjectName: obj.ObjectName,
				ObjectType: obj.ObjectType,
				Pattern:    objectName,
				MatchLine:  match.MatchedLine,
				LineNumber: match.LineNumber,
				RiskLevel:  "medium",
			})
		}
	}
	return risks, nil
}

// getSourceForAnalysis fetches object source for config pattern analysis.
func (c *Client) getSourceForAnalysis(ctx context.Context, objectURI string) (string, error) {
	resp, err := c.transport.Request(ctx, objectURI+"/source/main", &RequestOptions{
		Accept: "text/plain",
	})
	if err != nil {
		return "", err
	}
	return string(resp.Body), nil
}

// DetectConfigPatterns analyzes source code for configuration-driven call patterns.
// Pure Go, exported for testing.
func DetectConfigPatterns(source, objectName string) []ConfigCallRisk {
	var risks []ConfigCallRisk
	upper := strings.ToUpper(source)

	// ENHANCEMENT-SECTION / ENHANCEMENT-POINT
	if strings.Contains(upper, "ENHANCEMENT-SECTION") || strings.Contains(upper, "ENHANCEMENT-POINT") {
		risks = append(risks, ConfigCallRisk{
			Type:        "enhancement",
			Name:        objectName,
			Description: "Object contains enhancement points — external code can inject logic",
			Source:      "source_analysis",
		})
	}

	// GET BADI / CALL BADI
	if strings.Contains(upper, "GET BADI") || strings.Contains(upper, "CALL BADI") {
		risks = append(risks, ConfigCallRisk{
			Type:        "badi",
			Name:        objectName,
			Description: "Object uses BAdI — implementations may change behavior",
			Source:      "source_analysis",
		})
	}

	// EXIT_ prefix in CALL FUNCTION
	if reUserExitCall.MatchString(source) {
		risks = append(risks, ConfigCallRisk{
			Type:        "user_exit",
			Name:        objectName,
			Description: "Object calls user exit function module",
			Source:      "source_analysis",
		})
	}

	// PERFORM...IN PROGRAM
	if reCrossProgramFM.MatchString(source) {
		risks = append(risks, ConfigCallRisk{
			Type:        "cross_program",
			Name:        objectName,
			Description: "Object calls FORM in external program (often config-driven)",
			Source:      "source_analysis",
		})
	}

	// IF_EX_ or IF_BADI_ interface implementation
	if reBAdIInterface.MatchString(source) {
		risks = append(risks, ConfigCallRisk{
			Type:        "badi",
			Name:        objectName,
			Description: "Object implements BAdI interface (IF_EX_/IF_BADI_)",
			Source:      "source_analysis",
		})
	}

	return risks
}

// queryConfigTables queries customizing tables for registrations referencing the object.
// Graceful degradation: if RunQuery is blocked, skip silently.
func (c *Client) queryConfigTables(ctx context.Context, objectName string) []ConfigCallRisk {
	var risks []ConfigCallRisk

	// Sanitize object name for SQL (strip quotes, allow only alphanumeric + / for namespaces)
	sanitized := reSQLSanitize.ReplaceAllString(objectName, "")
	if sanitized == "" {
		return risks
	}

	queries := []struct {
		table       string
		field       string
		riskType    string
		description string
	}{
		{"SXS_INTER", "IMP_CLASS", "badi", "BAdI implementation registered for this class"},
		{"MODSAP", "MEMBER", "user_exit", "User exit assignment referencing this object"},
		{"TNAPR", "ROUTINE", "nace", "NACE output determination referencing this object"},
	}

	for _, q := range queries {
		sql := fmt.Sprintf("SELECT * FROM %s WHERE %s LIKE '%%%s%%'", q.table, q.field, sanitized)
		result, err := c.RunQuery(ctx, sql, 10)
		if err != nil {
			// RunQuery blocked by safety (SAP_BLOCK_FREE_SQL) or table doesn't exist — skip
			continue
		}
		if result != nil && len(result.Rows) > 0 {
			risks = append(risks, ConfigCallRisk{
				Type:        q.riskType,
				Name:        sanitized,
				Description: fmt.Sprintf("%s: %s.%s has %d entries referencing '%s'", q.description, q.table, q.field, len(result.Rows), sanitized),
				Source:      "table_query",
			})
		}
	}

	return risks
}
