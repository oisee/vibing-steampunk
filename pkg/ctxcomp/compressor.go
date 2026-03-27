package ctxcomp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Compressor orchestrates dependency extraction, source fetching, and contract generation.
type Compressor struct {
	provider SourceProvider
	maxDeps  int
	maxDepth int // 1 = direct deps only, 2 = deps of deps, etc.
}

// NewCompressor creates a new Compressor. maxDeps limits how many dependencies are resolved (default 20).
func NewCompressor(provider SourceProvider, maxDeps int) *Compressor {
	if maxDeps <= 0 {
		maxDeps = 20
	}
	return &Compressor{provider: provider, maxDeps: maxDeps, maxDepth: 1}
}

// WithDepth sets the dependency expansion depth (1=direct only, 2=deps of deps, max 3).
func (c *Compressor) WithDepth(depth int) *Compressor {
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}
	c.maxDepth = depth
	return c
}

// Compress extracts dependencies from source, fetches their contracts, and builds a prologue.
func (c *Compressor) Compress(ctx context.Context, source, objectName, objectType string) (*ContextResult, error) {
	seen := map[string]bool{strings.ToUpper(objectName): true}
	var allDeps []Dependency
	var allContracts []Contract

	// Level 1: extract deps from main source
	pendingSources := []string{source}
	pendingNames := []string{objectName}

	for level := 1; level <= c.maxDepth; level++ {
		var levelDeps []Dependency
		for i, src := range pendingSources {
			deps := ExtractDependencies(src)
			deps = filterSelf(deps, pendingNames[i])
			// Filter already-seen
			for _, d := range deps {
				if !seen[d.Name] {
					levelDeps = append(levelDeps, d)
					seen[d.Name] = true
				}
			}
		}

		if len(levelDeps) == 0 {
			break
		}

		// Prioritize: custom (Z*/Y*) first, then SAP standard
		sort.Slice(levelDeps, func(i, j int) bool {
			iCustom := isCustom(levelDeps[i].Name)
			jCustom := isCustom(levelDeps[j].Name)
			if iCustom != jCustom {
				return iCustom
			}
			return levelDeps[i].Line < levelDeps[j].Line
		})

		// Limit total deps across all levels
		remaining := c.maxDeps - len(allDeps)
		if remaining <= 0 {
			break
		}
		if len(levelDeps) > remaining {
			levelDeps = levelDeps[:remaining]
		}

		allDeps = append(allDeps, levelDeps...)

		// Fetch full sources + contracts for this level
		contracts, fullSources := c.fetchContractsWithSources(ctx, levelDeps)
		allContracts = append(allContracts, contracts...)

		// Prepare next level: extract deps from fetched full sources
		if level < c.maxDepth {
			pendingSources = nil
			pendingNames = nil
			for i, src := range fullSources {
				if src != "" {
					pendingSources = append(pendingSources, src)
					pendingNames = append(pendingNames, levelDeps[i].Name)
				}
			}
		}
	}

	prologue := formatPrologue(objectName, allContracts)
	lines := strings.Count(prologue, "\n") + 1

	stats := ContextStats{
		DepsFound:  len(allDeps),
		TotalLines: lines,
	}
	for _, ct := range allContracts {
		if ct.Error != "" {
			stats.DepsFailed++
		} else {
			stats.DepsResolved++
		}
	}

	return &ContextResult{
		SourceName:   objectName,
		Dependencies: allDeps,
		Contracts:    allContracts,
		Prologue:     prologue,
		Stats:        stats,
	}, nil
}

// fetchContractsWithSources fetches contracts and returns full sources for deeper expansion.
func (c *Compressor) fetchContractsWithSources(ctx context.Context, deps []Dependency) ([]Contract, []string) {
	contracts := make([]Contract, len(deps))
	fullSources := make([]string, len(deps))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // bounded parallelism

	for i, dep := range deps {
		wg.Add(1)
		go func(idx int, d Dependency) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fullSource, err := c.provider.GetSource(ctx, d.Kind, d.Name)
			if err != nil {
				contracts[idx] = Contract{
					Name:  d.Name,
					Kind:  d.Kind,
					Error: err.Error(),
				}
				return
			}

			fullSources[idx] = fullSource
			compressed := ExtractContract(fullSource, d.Kind)
			contracts[idx] = Contract{
				Name:   d.Name,
				Kind:   d.Kind,
				Source: compressed,
			}
		}(i, dep)
	}

	wg.Wait()
	return contracts, fullSources
}

func filterSelf(deps []Dependency, objectName string) []Dependency {
	upper := strings.ToUpper(objectName)
	filtered := make([]Dependency, 0, len(deps))
	for _, d := range deps {
		if d.Name != upper {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

func isCustom(name string) bool {
	return strings.HasPrefix(name, "Z") || strings.HasPrefix(name, "Y") ||
		strings.HasPrefix(name, "/Z") || strings.HasPrefix(name, "/Y")
}

func formatPrologue(objectName string, contracts []Contract) string {
	if len(contracts) == 0 {
		return ""
	}

	var resolved []Contract
	for _, c := range contracts {
		if c.Error == "" && c.Source != "" {
			resolved = append(resolved, c)
		}
	}

	if len(resolved) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("* === Dependency context for %s (%d deps) ===\n", objectName, len(resolved)))

	for _, c := range resolved {
		kindLabel := "class"
		switch c.Kind {
		case KindInterface:
			kindLabel = "interface"
		case KindFunction:
			kindLabel = "function module"
		}

		// Count methods in contract for info
		methodCount := strings.Count(strings.ToUpper(c.Source), "METHODS ")
		info := kindLabel
		if methodCount > 0 {
			info = fmt.Sprintf("%s, %d methods", kindLabel, methodCount)
		}

		sb.WriteString(fmt.Sprintf("\n* --- %s (%s) ---\n", c.Name, info))
		sb.WriteString(c.Source)
		sb.WriteString("\n")
	}

	return sb.String()
}
