package ctxcomp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ADTSourceFetcher abstracts the ADT client methods needed by the provider.
type ADTSourceFetcher interface {
	GetSource(ctx context.Context, objectType, name string, opts interface{}) (string, error)
}

// MultiSourceProvider resolves source code from local files, cache, or SAP.
type MultiSourceProvider struct {
	workspaceRoot string
	adtFetcher    ADTSourceFetcher // nil = offline only

	contracts  map[string]string
	contractMu sync.RWMutex
}

// NewMultiSourceProvider creates a provider. Both workspaceRoot and adtFetcher may be empty/nil.
func NewMultiSourceProvider(workspaceRoot string, adtFetcher ADTSourceFetcher) *MultiSourceProvider {
	return &MultiSourceProvider{
		workspaceRoot: workspaceRoot,
		adtFetcher:    adtFetcher,
		contracts:     make(map[string]string),
	}
}

func (p *MultiSourceProvider) GetSource(ctx context.Context, kind DependencyKind, name string) (string, error) {
	key := string(kind) + ":" + name

	// 1. Check contract cache
	p.contractMu.RLock()
	if cached, ok := p.contracts[key]; ok {
		p.contractMu.RUnlock()
		return cached, nil
	}
	p.contractMu.RUnlock()

	// 2. Check local workspace files
	if p.workspaceRoot != "" {
		if src, err := p.findLocal(kind, name); err == nil {
			return src, nil
		}
	}

	// 3. Fetch from SAP
	if p.adtFetcher != nil {
		objType := kindToObjectType(kind)
		src, err := p.adtFetcher.GetSource(ctx, objType, name, nil)
		if err != nil {
			return "", fmt.Errorf("SAP fetch %s %s: %w", kind, name, err)
		}
		return src, nil
	}

	return "", fmt.Errorf("no source available for %s %s", kind, name)
}

// CacheContract stores a pre-extracted contract for future lookups.
func (p *MultiSourceProvider) CacheContract(kind DependencyKind, name, contract string) {
	key := string(kind) + ":" + name
	p.contractMu.Lock()
	p.contracts[key] = contract
	p.contractMu.Unlock()
}

func (p *MultiSourceProvider) findLocal(kind DependencyKind, name string) (string, error) {
	// Convert name to abapGit-style filename
	lower := strings.ToLower(name)
	lower = strings.ReplaceAll(lower, "/", "#")

	var patterns []string
	switch kind {
	case KindClass:
		patterns = []string{lower + ".clas.abap"}
	case KindInterface:
		patterns = []string{lower + ".intf.abap"}
	case KindFunction:
		patterns = []string{"*.fugr." + lower + ".abap"} // func inside fugr
	}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(p.workspaceRoot, "**", pattern))
		if err == nil && len(matches) > 0 {
			data, err := os.ReadFile(matches[0])
			if err == nil {
				return string(data), nil
			}
		}
		// Also try flat directory
		matches, err = filepath.Glob(filepath.Join(p.workspaceRoot, pattern))
		if err == nil && len(matches) > 0 {
			data, err := os.ReadFile(matches[0])
			if err == nil {
				return string(data), nil
			}
		}
	}

	return "", fmt.Errorf("not found locally: %s", name)
}

func kindToObjectType(kind DependencyKind) string {
	switch kind {
	case KindClass:
		return "CLAS"
	case KindInterface:
		return "INTF"
	case KindFunction:
		return "FUNC"
	default:
		return string(kind)
	}
}
