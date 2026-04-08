package graph

import (
	"sort"
	"strings"
)

// APIUsageEntry represents a standard API object used by custom code.
type APIUsageEntry struct {
	NodeID       string   `json:"node_id"`                 // CLAS:CL_SALV_TABLE
	Name         string   `json:"name"`                    // CL_SALV_TABLE
	Type         string   `json:"type"`                    // CLAS, FUGR, INTF, TABL, ...
	CallerCount  int      `json:"caller_count"`            // How many distinct custom objects reference it
	UsageCount   int      `json:"usage_count"`             // Total reference count (may be > caller_count)
	Callers      []string `json:"callers"`                 // Custom object names that use it (up to 10)
	ReleaseState string   `json:"release_state,omitempty"` // RELEASED, NOT_RELEASED, DEPRECATED (if enriched)
}

// APISurfaceResult is the result of ComputeAPISurface.
type APISurfaceResult struct {
	Scope              string          `json:"scope"`                // Package or prefix
	TotalCustomObjects int             `json:"total_custom_objects"` // How many custom objects were scanned
	UniqueStandardAPIs int             `json:"unique_standard_apis"` // Distinct standard objects found
	TotalCrossings     int             `json:"total_crossings"`      // Total custom→standard references
	TopAPIs            []APIUsageEntry `json:"top_apis"`
	ByReleaseState     map[string]int  `json:"by_release_state,omitempty"` // If enriched
}

// APISurfaceRow represents a single cross-reference row (from WBCROSSGT or CROSS).
// Include = referencing custom include, RefName = referenced object name.
type APISurfaceRow struct {
	Include string // Custom include (e.g., ZCL_FOO========CP)
	RefName string // Referenced object name (e.g., CL_SALV_TABLE)
	RefType string // Reference type code (OTYPE from WBCROSSGT or TYPE from CROSS)
	Source  string // "WBCROSSGT" or "CROSS"
}

// ComputeAPISurface aggregates custom→standard dependency crossings.
// Takes pre-fetched cross-reference rows and classifies each reference.
// No I/O — callers provide rows and custom object set.
func ComputeAPISurface(rows []APISurfaceRow, customObjects map[string]bool, topN int) *APISurfaceResult {
	if topN <= 0 {
		topN = 50
	}

	result := &APISurfaceResult{
		TotalCustomObjects: len(customObjects),
	}

	// Aggregate: standard object → {callers set, total count}
	type apiAgg struct {
		callers map[string]bool
		count   int
	}
	agg := make(map[string]*apiAgg)

	for _, row := range rows {
		refName := strings.ToUpper(strings.TrimSpace(row.RefName))
		include := strings.TrimSpace(row.Include)

		if refName == "" || include == "" {
			continue
		}

		// Skip if referenced object is custom (we want custom→standard only)
		if IsCustomObject(refName) {
			continue
		}
		if isAPISurfaceNoise(refName) {
			continue
		}

		// Skip component-internal refs
		if strings.Contains(refName, "\\") {
			continue
		}

		// Normalize the include to object-level to identify the caller
		_, _, callerName := NormalizeInclude(include)
		callerUpper := strings.ToUpper(callerName)

		// Only count if caller is in our custom scope
		if !customObjects[callerUpper] {
			continue
		}

		// Skip self-references (shouldn't happen with custom→standard, but be safe)
		if strings.EqualFold(callerName, refName) {
			continue
		}

		a, ok := agg[refName]
		if !ok {
			a = &apiAgg{callers: make(map[string]bool)}
			agg[refName] = a
		}
		a.callers[callerUpper] = true
		a.count++
		result.TotalCrossings++
	}

	// Build entries
	entries := make([]APIUsageEntry, 0, len(agg))
	for name, a := range agg {
		entry := APIUsageEntry{
			NodeID:      NodeID(guessTypeFromName(name), name),
			Name:        name,
			Type:        guessTypeFromName(name),
			CallerCount: len(a.callers),
			UsageCount:  a.count,
		}

		// Collect up to 10 caller names
		callerList := make([]string, 0, len(a.callers))
		for c := range a.callers {
			callerList = append(callerList, c)
		}
		sort.Strings(callerList)
		if len(callerList) > 10 {
			callerList = callerList[:10]
		}
		entry.Callers = callerList

		entries = append(entries, entry)
	}

	// Sort: caller_count desc, usage_count desc, node_id asc
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CallerCount != entries[j].CallerCount {
			return entries[i].CallerCount > entries[j].CallerCount
		}
		if entries[i].UsageCount != entries[j].UsageCount {
			return entries[i].UsageCount > entries[j].UsageCount
		}
		return entries[i].NodeID < entries[j].NodeID
	})

	result.UniqueStandardAPIs = len(entries)

	// Cap to topN
	if len(entries) > topN {
		entries = entries[:topN]
	}
	result.TopAPIs = entries

	return result
}

// IsCustomObject returns true if the object name indicates customer code.
// Classification policy:
//   - Z*, Y* prefix → custom
//   - /Z.../, /Y.../ namespace → custom
//   - Everything else → standard (including /SAP/, /UI5/, etc.)
//
// Note: not all slash-namespaces are SAP. Partner namespaces like /VENDOR/
// are treated as standard by default. Use configuration to override if needed.
func IsCustomObject(name string) bool {
	upper := strings.ToUpper(strings.TrimSpace(name))
	if upper == "" {
		return false
	}

	// Z* or Y* prefix (no namespace)
	if upper[0] == 'Z' || upper[0] == 'Y' {
		return true
	}

	// /Z.../ or /Y.../ namespace
	if len(upper) > 2 && upper[0] == '/' {
		// Find the closing slash of the namespace
		closeSlash := strings.Index(upper[1:], "/")
		if closeSlash > 0 {
			nsChar := upper[1] // first char of namespace
			if nsChar == 'Z' || nsChar == 'Y' {
				return true
			}
		}
	}

	return false
}

func isAPISurfaceNoise(name string) bool {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "SY", "SYST", "SYST_SUBRC", "ABAP_BOOL", "ABAP_TRUE", "ABAP_FALSE":
		return true
	default:
		return false
	}
}
