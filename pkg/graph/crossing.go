package graph

import "strings"

// CrossingDirection classifies the direction of a cross-package dependency.
type CrossingDirection string

const (
	CrossUpward    CrossingDirection = "UPWARD"      // child → parent/ancestor — OK
	CrossUpwardSkip CrossingDirection = "UPWARD_SKIP" // child → grandparent+ (skipping levels) — WARN
	CrossCommon    CrossingDirection = "COMMON"       // anything → common/shared package (_00) — OK
	CrossSibling   CrossingDirection = "SIBLING"      // between siblings at same level — BAD
	CrossDownward  CrossingDirection = "DOWNWARD"     // parent → child — BAD
	CrossCommonDown CrossingDirection = "COMMON_DOWN" // common package → specific module — BAD
	CrossExternal  CrossingDirection = "EXTERNAL"     // different hierarchy entirely — INFO
	CrossSame      CrossingDirection = "SAME"         // same package — OK (not a crossing)
	CrossStandard  CrossingDirection = "STANDARD"     // standard SAP — ignore
)

// CrossingEntry represents a single directional crossing.
type CrossingEntry struct {
	SourceObject  string            `json:"sourceObject"`
	SourceType    string            `json:"sourceType,omitempty"`
	SourcePackage string            `json:"sourcePackage"`
	TargetObject  string            `json:"targetObject"`
	TargetType    string            `json:"targetType,omitempty"`
	TargetPackage string            `json:"targetPackage"`
	Direction     CrossingDirection `json:"direction"`
	EdgeKind      string            `json:"edgeKind"`
	RefDetail     string            `json:"refDetail,omitempty"`
}

// CrossingReport is the result of a directional boundary crossing analysis.
type CrossingReport struct {
	RootPackage     string           `json:"rootPackage"`
	PackagesScanned int              `json:"packagesScanned"`
	ObjectsScanned  int              `json:"objectsScanned"`
	Entries         []CrossingEntry  `json:"entries,omitempty"`

	// Counts by direction
	Upward     int `json:"upward"`
	UpwardSkip int `json:"upwardSkip"`
	Common     int `json:"common"`
	Sibling    int `json:"sibling"`
	Downward   int `json:"downward"`
	CommonDown int `json:"commonDown"`
	External   int `json:"external"`
	Dynamic    int `json:"dynamic"`

	// Circular: pairs of sibling packages with deps in both directions
	Circular []string `json:"circular,omitempty"`
}

// CrossingOptions configures the directional crossing analysis.
type CrossingOptions struct {
	// CommonPatterns identifies "common" packages by suffix (default: ["_00"])
	CommonPatterns []string
	// TestPatterns identifies test packages — sibling crossings from test packages are OK
	TestPatterns []string
	// UpwardSkipThreshold: how many levels of skip before flagging (default: 2)
	UpwardSkipThreshold int
}

func defaultCrossingOptions() *CrossingOptions {
	return &CrossingOptions{
		CommonPatterns:      []string{"_00"},
		TestPatterns:        []string{"_TEST"},
		UpwardSkipThreshold: 2,
	}
}

// ClassifyCrossing determines the direction of a dependency from sourcePackage to targetPackage
// within a package hierarchy defined by scope.
func ClassifyCrossing(sourcePackage, targetPackage string, scope *PackageScope, opts *CrossingOptions) CrossingDirection {
	if opts == nil {
		opts = defaultCrossingOptions()
	}

	src := strings.ToUpper(strings.TrimSpace(sourcePackage))
	tgt := strings.ToUpper(strings.TrimSpace(targetPackage))

	if src == tgt {
		return CrossSame
	}

	// External: target not in scope
	if !scope.InScope(tgt) {
		return CrossExternal
	}

	// Is target a "common" package?
	if isCommonPackage(tgt, opts.CommonPatterns) {
		// But if source is also common and target is not → COMMON_DOWN
		if isCommonPackage(src, opts.CommonPatterns) && !isCommonPackage(tgt, opts.CommonPatterns) {
			return CrossCommonDown
		}
		return CrossCommon
	}

	// Is source a "common" package pointing to a non-common sibling?
	if isCommonPackage(src, opts.CommonPatterns) {
		return CrossCommonDown
	}

	// Check ancestry using the hierarchy map
	// Is target an ancestor of source? → UPWARD
	if isAncestor(tgt, src, scope.Hierarchy) {
		depth := ancestorDepth(tgt, src, scope.Hierarchy)
		if depth > opts.UpwardSkipThreshold {
			return CrossUpwardSkip
		}
		return CrossUpward
	}

	// Is target a descendant of source? → DOWNWARD
	if isAncestor(src, tgt, scope.Hierarchy) {
		return CrossDownward
	}

	// Fallback: try prefix-based hierarchy when TDEVC hierarchy is incomplete
	if len(scope.Hierarchy) == 0 || !hasHierarchyEntry(src, scope.Hierarchy) {
		return classifyByPrefix(src, tgt, opts)
	}

	// Same level, different branch → SIBLING
	return CrossSibling
}

// isAncestor returns true if 'ancestor' is an ancestor of 'child' in the hierarchy.
func isAncestor(ancestor, child string, hierarchy map[string]string) bool {
	current := child
	for i := 0; i < 20; i++ { // safety limit
		parent, ok := hierarchy[current]
		if !ok || parent == "" {
			return false
		}
		if parent == ancestor {
			return true
		}
		current = parent
	}
	return false
}

// ancestorDepth returns how many levels up 'ancestor' is from 'child'.
func ancestorDepth(ancestor, child string, hierarchy map[string]string) int {
	current := child
	depth := 0
	for i := 0; i < 20; i++ {
		parent, ok := hierarchy[current]
		if !ok || parent == "" {
			return depth
		}
		depth++
		if parent == ancestor {
			return depth
		}
		current = parent
	}
	return depth
}

func hasHierarchyEntry(pkg string, hierarchy map[string]string) bool {
	if _, ok := hierarchy[pkg]; ok {
		return true
	}
	// Also check if pkg is a parent of anything
	for _, parent := range hierarchy {
		if parent == pkg {
			return true
		}
	}
	return false
}

// classifyByPrefix uses package name prefix matching when TDEVC hierarchy is unavailable.
func classifyByPrefix(src, tgt string, opts *CrossingOptions) CrossingDirection {
	// Target is a prefix of source → UPWARD (tgt is parent of src by name)
	if strings.HasPrefix(src, tgt+"_") || strings.HasPrefix(src, tgt) && len(src) > len(tgt) {
		return CrossUpward
	}

	// Source is a prefix of target → DOWNWARD
	if strings.HasPrefix(tgt, src+"_") || strings.HasPrefix(tgt, src) && len(tgt) > len(src) {
		return CrossDownward
	}

	// Find common prefix to determine if siblings
	commonRoot := longestCommonPrefix(src, tgt)
	if commonRoot != "" && (strings.HasSuffix(commonRoot, "_") || commonRoot == src[:strings.LastIndex(src, "_")+1]) {
		return CrossSibling
	}

	return CrossSibling // default for same-hierarchy non-ancestor/descendant
}

func longestCommonPrefix(a, b string) string {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			return a[:i]
		}
	}
	return a[:minLen]
}

func isCommonPackage(pkg string, patterns []string) bool {
	upper := strings.ToUpper(pkg)
	for _, p := range patterns {
		if strings.HasSuffix(upper, strings.ToUpper(p)) {
			return true
		}
	}
	return false
}

// AnalyzeCrossings performs directional boundary analysis on a graph within a scope.
func AnalyzeCrossings(g *Graph, scope *PackageScope, opts *CrossingOptions) *CrossingReport {
	if opts == nil {
		opts = defaultCrossingOptions()
	}

	report := &CrossingReport{
		RootPackage:     scope.RootPackage,
		PackagesScanned: len(scope.Packages),
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	// Find all nodes in scope
	scopeNodes := make(map[string]bool)
	for _, n := range g.nodes {
		if scope.InScope(n.Package) {
			scopeNodes[n.ID] = true
		}
	}
	report.ObjectsScanned = len(scopeNodes)

	// Track sibling direction pairs for circular detection
	siblingPairs := make(map[string]map[string]bool) // srcPkg → set of tgtPkgs
	// Deduplicate: same source→target pair only counted once
	seenPairs := make(map[string]bool)

	for nodeID := range scopeNodes {
		edges := g.outEdges[nodeID]
		for _, e := range edges {
			fromNode := g.nodes[nodeID]
			toNode := g.nodes[e.To]

			// Dynamic calls
			if e.Kind == EdgeDynamic {
				report.Dynamic++
				continue
			}

			if toNode == nil || fromNode == nil {
				continue
			}

			// Standard SAP
			if IsStandardObject(toNode.Name) {
				continue
			}

			srcPkg := strings.ToUpper(fromNode.Package)
			tgtPkg := strings.ToUpper(toNode.Package)

			if srcPkg == "" || srcPkg == tgtPkg {
				continue
			}

			// Unresolved package on a custom object → guess from name, classify
			if tgtPkg == "" {
				pairKey := fromNode.Name + "→" + toNode.Name
				if !IsStandardObject(toNode.Name) && !seenPairs[pairKey] {
					seenPairs[pairKey] = true
					guessed := GuessPackageFromName(toNode.Name)
					direction := CrossExternal
					if guessed != "" {
						toNode.Package = guessed
						tgtPkg = guessed
						direction = ClassifyCrossing(srcPkg, tgtPkg, scope, opts)
					}
					if direction == CrossSame {
						continue
					}
					report.Entries = append(report.Entries, CrossingEntry{
						SourceObject:  fromNode.Name,
						SourceType:    fromNode.Type,
						SourcePackage: srcPkg,
						TargetObject:  toNode.Name,
						TargetType:    toNode.Type,
						TargetPackage: guessed,
						Direction:     direction,
						EdgeKind:      string(e.Kind),
						RefDetail:     e.RefDetail,
					})
					switch direction {
					case CrossUpward:
						report.Upward++
					case CrossCommon:
						report.Common++
					case CrossSibling:
						report.Sibling++
					case CrossDownward:
						report.Downward++
					case CrossCommonDown:
						report.CommonDown++
					default:
						report.External++
					}
				}
				continue
			}

			direction := ClassifyCrossing(srcPkg, tgtPkg, scope, opts)

			// Skip test package sibling crossings — tests are expected to reach into siblings
			if direction == CrossSibling && isTestPackage(srcPkg, opts.TestPatterns) {
				continue
			}

			pairKey := fromNode.Name + "→" + toNode.Name
			if seenPairs[pairKey] {
				continue
			}
			seenPairs[pairKey] = true

			entry := CrossingEntry{
				SourceObject:  fromNode.Name,
				SourceType:    fromNode.Type,
				SourcePackage: srcPkg,
				TargetObject:  toNode.Name,
				TargetType:    toNode.Type,
				TargetPackage: tgtPkg,
				Direction:     direction,
				EdgeKind:      string(e.Kind),
				RefDetail:     e.RefDetail,
			}
			report.Entries = append(report.Entries, entry)

			switch direction {
			case CrossUpward:
				report.Upward++
			case CrossUpwardSkip:
				report.UpwardSkip++
			case CrossCommon:
				report.Common++
			case CrossSibling:
				report.Sibling++
				// Track for circular detection
				if siblingPairs[srcPkg] == nil {
					siblingPairs[srcPkg] = make(map[string]bool)
				}
				siblingPairs[srcPkg][tgtPkg] = true
			case CrossDownward:
				report.Downward++
			case CrossCommonDown:
				report.CommonDown++
			case CrossExternal:
				report.External++
			}
		}
	}

	// Detect circular sibling dependencies
	seen := make(map[string]bool)
	for srcPkg, targets := range siblingPairs {
		for tgtPkg := range targets {
			if siblingPairs[tgtPkg] != nil && siblingPairs[tgtPkg][srcPkg] {
				pair := srcPkg + " <-> " + tgtPkg
				pairRev := tgtPkg + " <-> " + srcPkg
				if !seen[pair] && !seen[pairRev] {
					report.Circular = append(report.Circular, pair)
					seen[pair] = true
				}
			}
		}
	}

	return report
}

// GuessPackageFromName infers a likely package name from an object name.
// ZCL_LLM_00_CACHE → $ZLLM_00, ZIF_RAY_00_NODE → $ZRAY_00, ZRAY_00_CCLM → $ZRAY_00
func GuessPackageFromName(objName string) string {
	upper := strings.ToUpper(strings.TrimSpace(objName))
	if upper == "" || !strings.HasPrefix(upper, "Z") {
		return ""
	}
	// Strip type prefixes: ZCL_, ZIF_, ZCX_, ZTT_
	core := upper
	prefixes := []string{"ZCL_", "ZIF_", "ZCX_", "ZTT_"}
	for _, p := range prefixes {
		if strings.HasPrefix(core, p) {
			core = core[len(p):]
			break
		}
	}
	// core is now e.g. "LLM_00_CACHE" or "RAY_00_CCLM"
	// For names that didn't have a type prefix (ZRAY_00_CCLM), strip leading Z
	if strings.HasPrefix(core, "Z") {
		core = core[1:]
	}
	// Find project prefix + module: first two _-separated segments
	parts := strings.SplitN(core, "_", 3)
	if len(parts) < 2 {
		return ""
	}
	return "$Z" + parts[0] + "_" + parts[1]
}

func isTestPackage(pkg string, patterns []string) bool {
	upper := strings.ToUpper(pkg)
	for _, p := range patterns {
		if strings.Contains(upper, strings.ToUpper(p)) {
			return true
		}
	}
	return false
}
