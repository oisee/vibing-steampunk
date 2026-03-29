// Package ctxcomp provides ABAP context compression.
// It extracts dependencies from ABAP source code and produces compressed
// "prologue" text containing only public API contracts of referenced objects.
package ctxcomp

import "context"

// DependencyKind classifies a dependency.
type DependencyKind string

const (
	KindClass     DependencyKind = "CLAS"
	KindInterface DependencyKind = "INTF"
	KindFunction  DependencyKind = "FUNC"
)

// Dependency represents a reference to an external ABAP object.
type Dependency struct {
	Name string
	Kind DependencyKind
	Line int // 1-based line where first reference was found
}

// Contract is the compressed public API of a dependency.
type Contract struct {
	Name      string
	Kind      DependencyKind
	Source    string // compressed public API text
	FromCache bool
	Error     string // non-empty if resolution failed
}

// ContextResult is the output of context compression.
type ContextResult struct {
	SourceName   string
	Dependencies []Dependency
	Contracts    []Contract
	Prologue     string
	Stats        ContextStats
}

// ContextStats holds compression statistics.
type ContextStats struct {
	DepsFound    int
	DepsResolved int
	DepsFailed   int
	TotalLines   int // lines in prologue
}

// SourceProvider fetches full source code for a given object.
type SourceProvider interface {
	GetSource(ctx context.Context, kind DependencyKind, name string) (string, error)
}
