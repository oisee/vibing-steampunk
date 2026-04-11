package graph

import (
	"sort"
	"strings"
)

// SlimEntry represents a candidate dead object or dead method.
type SlimEntry struct {
	NodeID        string `json:"node_id"`                   // CLAS:ZCL_OLD_HELPER
	Name          string `json:"name"`                      // ZCL_OLD_HELPER
	Type          string `json:"type"`                      // CLAS, PROG, INTF, FUGR, TABL, ...
	Package       string `json:"package,omitempty"`         // $ZDEV
	Kind          string `json:"kind"`                      // "dead_object", "internal_only", "dead_method"
	Method        string `json:"method,omitempty"`          // method name (for dead_method only)
	IncomingRefs  int    `json:"incoming_refs"`             // total incoming references found
	InternalRefs  int    `json:"internal_refs,omitempty"`   // refs from within scope
	ExternalRefs  int    `json:"external_refs,omitempty"`   // refs from outside scope
	Confidence    string `json:"confidence"`                // HIGH, MEDIUM
	LastTransport string `json:"last_transport,omitempty"`  // YYYYMMDD from meta if available
}

// SlimResult is the result of ComputeSlim.
type SlimResult struct {
	Scope             string      `json:"scope"`
	TotalObjects      int         `json:"total_objects"`
	DeadObjects       []SlimEntry `json:"dead_objects"`
	InternalOnly      []SlimEntry `json:"internal_only,omitempty"`
	DeadMethods       []SlimEntry `json:"dead_methods,omitempty"`
	DeadObjectCount   int         `json:"dead_object_count"`
	InternalOnlyCount int         `json:"internal_only_count"`
	DeadMethodCount   int         `json:"dead_method_count"`
	LiveObjectCount   int         `json:"live_object_count"`
	ScopePackages     []string    `json:"scope_packages,omitempty"`
}

// SlimObjectInfo describes a custom object in the target package.
type SlimObjectInfo struct {
	Name    string
	Type    string
	Package string
	Methods []string // for classes: method names (empty for non-class types)
}

// SlimRefRow represents a reverse cross-reference row.
// CallerInclude = who references this, TargetName = what is referenced.
type SlimRefRow struct {
	CallerInclude string // include that holds the reference
	TargetName    string // object being referenced
	Source        string // "WBCROSSGT" or "CROSS"
}

// ComputeSlim identifies dead objects, internal-only objects, and dead methods.
//
// Verdicts:
//   - DEAD: zero refs total (HIGH confidence). Definitely unused.
//   - INTERNAL_ONLY: refs exist but all from within scope (WARNING).
//     May be legitimate internal helper or part of dead cluster.
//   - LIVE: has at least one external ref.
//
// scopeObjects: set of object names in the analysis scope (for internal/external classification).
// If nil, falls back to V1 behavior (zero refs = dead, any refs = live).
func ComputeSlim(objects []SlimObjectInfo, refs []SlimRefRow, interfaceMethods map[string]bool, scopeObjects map[string]bool) *SlimResult {
	result := &SlimResult{
		TotalObjects: len(objects),
	}

	// Build object set if not provided (V1 compat)
	if scopeObjects == nil {
		scopeObjects = make(map[string]bool)
		for _, obj := range objects {
			scopeObjects[strings.ToUpper(obj.Name)] = true
		}
	}

	// Count incoming refs per target, classified as internal/external
	type refCounts struct {
		internal int
		external int
	}
	refMap := make(map[string]*refCounts)

	for _, ref := range refs {
		targetUpper := strings.ToUpper(strings.TrimSpace(ref.TargetName))
		if targetUpper == "" {
			continue
		}

		callerInclude := strings.TrimSpace(ref.CallerInclude)
		if callerInclude == "" {
			continue
		}

		_, _, callerName := NormalizeInclude(callerInclude)
		callerUpper := strings.ToUpper(callerName)

		// Skip self-references
		if callerUpper == targetUpper {
			continue
		}

		rc, ok := refMap[targetUpper]
		if !ok {
			rc = &refCounts{}
			refMap[targetUpper] = rc
		}

		if scopeObjects[callerUpper] {
			rc.internal++
		} else {
			rc.external++
		}
	}

	// Classify objects
	for _, obj := range objects {
		nameUpper := strings.ToUpper(obj.Name)
		rc := refMap[nameUpper]

		totalRefs := 0
		internalRefs := 0
		externalRefs := 0
		if rc != nil {
			internalRefs = rc.internal
			externalRefs = rc.external
			totalRefs = internalRefs + externalRefs
		}

		if totalRefs == 0 {
			// DEAD: zero refs anywhere
			result.DeadObjects = append(result.DeadObjects, SlimEntry{
				NodeID:       NodeID(obj.Type, obj.Name),
				Name:         obj.Name,
				Type:         obj.Type,
				Package:      obj.Package,
				Kind:         "dead_object",
				IncomingRefs: 0,
				Confidence:   "HIGH",
			})
		} else if externalRefs == 0 {
			// INTERNAL_ONLY: refs exist but all from within scope
			result.InternalOnly = append(result.InternalOnly, SlimEntry{
				NodeID:       NodeID(obj.Type, obj.Name),
				Name:         obj.Name,
				Type:         obj.Type,
				Package:      obj.Package,
				Kind:         "internal_only",
				IncomingRefs: totalRefs,
				InternalRefs: internalRefs,
				Confidence:   "MEDIUM",
			})
		}
		// else: LIVE (has external refs) — not reported
	}

	// Dead methods: only for non-DEAD classes
	if interfaceMethods == nil {
		interfaceMethods = make(map[string]bool)
	}

	deadObjectSet := make(map[string]bool)
	for _, d := range result.DeadObjects {
		deadObjectSet[strings.ToUpper(d.Name)] = true
	}

	for _, obj := range objects {
		if obj.Type != "CLAS" || len(obj.Methods) == 0 {
			continue
		}
		nameUpper := strings.ToUpper(obj.Name)

		// Skip dead objects — all their methods are dead by definition
		if deadObjectSet[nameUpper] {
			continue
		}

		for _, method := range obj.Methods {
			methodUpper := strings.ToUpper(method)
			methodKey := nameUpper + "=>" + methodUpper

			// Skip interface methods (may be called polymorphically)
			if interfaceMethods[methodKey] {
				continue
			}

			// Method-level ref tracking is approximate in V2
			// (WBCROSSGT doesn't reliably track per-method refs)
			// For now: flag all non-interface methods as candidates
			// This is MEDIUM confidence — needs source-level validation in V2.1
			result.DeadMethods = append(result.DeadMethods, SlimEntry{
				NodeID:       NodeID(obj.Type, obj.Name),
				Name:         obj.Name,
				Type:         obj.Type,
				Package:      obj.Package,
				Kind:         "dead_method",
				Method:       method,
				IncomingRefs: 0,
				Confidence:   "MEDIUM",
			})
		}
	}

	// Sort
	sort.Slice(result.DeadObjects, func(i, j int) bool {
		return result.DeadObjects[i].NodeID < result.DeadObjects[j].NodeID
	})
	sort.Slice(result.InternalOnly, func(i, j int) bool {
		return result.InternalOnly[i].NodeID < result.InternalOnly[j].NodeID
	})
	sort.Slice(result.DeadMethods, func(i, j int) bool {
		if result.DeadMethods[i].NodeID != result.DeadMethods[j].NodeID {
			return result.DeadMethods[i].NodeID < result.DeadMethods[j].NodeID
		}
		return result.DeadMethods[i].Method < result.DeadMethods[j].Method
	})

	result.DeadObjectCount = len(result.DeadObjects)
	result.InternalOnlyCount = len(result.InternalOnly)
	result.DeadMethodCount = len(result.DeadMethods)
	result.LiveObjectCount = result.TotalObjects - result.DeadObjectCount - result.InternalOnlyCount

	return result
}
