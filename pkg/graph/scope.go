package graph

import (
	"sort"
	"strings"
)

// PackageScope represents a resolved set of packages for analysis.
type PackageScope struct {
	RootPackage string            `json:"root_package"`
	Packages    []string          `json:"packages"`     // All packages in scope (sorted)
	PackageSet  map[string]bool   `json:"-"`            // For fast membership check
	Hierarchy   map[string]string `json:"hierarchy"`    // child → parent mapping
	Method      string            `json:"method"`       // "exact", "hierarchy", "mask"
}

// TDEVCRow represents a row from the TDEVC table (package hierarchy).
type TDEVCRow struct {
	DevClass string // Package name
	ParentCL string // Parent package name (empty = root)
}

// TADIRRow represents a row from the TADIR table (object inventory).
type TADIRRow struct {
	Object   string // Object type: CLAS, PROG, FUGR, ...
	ObjName  string // Object name
	DevClass string // Package
}

// InScope returns true if the given package is within this scope.
func (s *PackageScope) InScope(pkg string) bool {
	return s.PackageSet[strings.ToUpper(strings.TrimSpace(pkg))]
}

// ObjectInScope returns true if an object (by name) belongs to the scope.
// Uses the object set built from TADIR, not package membership.
func (s *PackageScope) ObjectInScope(objectName string, scopeObjects map[string]bool) bool {
	return scopeObjects[strings.ToUpper(strings.TrimSpace(objectName))]
}

// ResolvePackageScope builds a PackageScope from TDEVC rows.
//
// Input patterns:
//   - "$ZLLM" (no wildcard): exact package + all hierarchical children
//   - "$ZLLM*" (wildcard): find packages matching mask, then expand children
//   - "$ZLLM" + exact=true: exact package only, no children
//
// If tdevcRows is nil/empty (TDEVC not queryable), falls back to mask-based
// scope using the input pattern directly.
func ResolvePackageScope(input string, exact bool, tdevcRows []TDEVCRow) *PackageScope {
	input = strings.ToUpper(strings.TrimSpace(input))

	scope := &PackageScope{
		RootPackage: input,
		PackageSet:  make(map[string]bool),
		Hierarchy:   make(map[string]string),
	}

	// Exact mode: single package only
	if exact {
		scope.Method = "exact"
		scope.PackageSet[input] = true
		scope.Packages = []string{input}
		return scope
	}

	// No TDEVC data: fallback to pattern-based scope
	if len(tdevcRows) == 0 {
		scope.Method = "mask"
		// Caller should use LIKE prefix query instead
		cleanInput := strings.TrimRight(input, "*")
		scope.PackageSet[cleanInput] = true
		scope.Packages = []string{cleanInput}
		return scope
	}

	// Build parent→children index from TDEVC
	children := make(map[string][]string) // parent → list of children
	allPackages := make(map[string]bool)

	for _, row := range tdevcRows {
		pkg := strings.ToUpper(strings.TrimSpace(row.DevClass))
		parent := strings.ToUpper(strings.TrimSpace(row.ParentCL))
		if pkg == "" {
			continue
		}
		allPackages[pkg] = true
		if parent != "" {
			children[parent] = append(children[parent], pkg)
			scope.Hierarchy[pkg] = parent
		}
	}

	// Determine root packages to expand
	var roots []string
	hasMask := strings.Contains(input, "*")

	if hasMask {
		scope.Method = "mask"
		pattern := strings.TrimRight(input, "*")
		for pkg := range allPackages {
			if strings.HasPrefix(pkg, pattern) {
				roots = append(roots, pkg)
			}
		}
	} else {
		scope.Method = "hierarchy"
		roots = []string{input}
	}

	// BFS: expand each root through children hierarchy
	for _, root := range roots {
		scope.PackageSet[root] = true
		queue := []string{root}
		for i := 0; i < len(queue); i++ {
			for _, child := range children[queue[i]] {
				if !scope.PackageSet[child] {
					scope.PackageSet[child] = true
					queue = append(queue, child)
				}
			}
		}
	}

	// Build sorted list
	scope.Packages = make([]string, 0, len(scope.PackageSet))
	for pkg := range scope.PackageSet {
		scope.Packages = append(scope.Packages, pkg)
	}
	sort.Strings(scope.Packages)

	return scope
}

// ClassifyRefs classifies reverse references as internal or external relative to scope.
func ClassifyRefs(refs []SlimRefRow, scopeObjects map[string]bool) (internal, external []SlimRefRow) {
	for _, ref := range refs {
		callerInclude := strings.TrimSpace(ref.CallerInclude)
		if callerInclude == "" {
			continue
		}
		_, _, callerName := NormalizeInclude(callerInclude)
		callerUpper := strings.ToUpper(callerName)

		// Self-ref: skip
		targetUpper := strings.ToUpper(strings.TrimSpace(ref.TargetName))
		if callerUpper == targetUpper {
			continue
		}

		if scopeObjects[callerUpper] {
			internal = append(internal, ref)
		} else {
			external = append(external, ref)
		}
	}
	return
}
