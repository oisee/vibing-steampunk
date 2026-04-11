package graph

import (
	"sort"
	"strings"
)

// RenameRefEntry represents a single reference that would need updating in a rename.
type RenameRefEntry struct {
	CallerID   string `json:"caller_id"`   // CLAS:ZCL_CONSUMER
	CallerName string `json:"caller_name"` // ZCL_CONSUMER
	CallerType string `json:"caller_type"` // CLAS
	Package    string `json:"package,omitempty"`
	RefCount   int    `json:"ref_count"`   // number of references from this caller
	Source     string `json:"source"`      // WBCROSSGT, CROSS, or combined
	Confidence string `json:"confidence"`  // HIGH (exact name in CROSS/WBCROSSGT), MEDIUM (LIKE match)
}

// RenameRisk describes an unresolvable risk in the rename.
type RenameRisk struct {
	Kind        string `json:"kind"`        // DYNAMIC_CALL, STRING_LITERAL, CONFIG_REF, NAME_OVERFLOW
	Description string `json:"description"`
}

// RenamePreviewResult is the full rename impact preview.
type RenamePreviewResult struct {
	OldName       string           `json:"old_name"`
	NewName       string           `json:"new_name"`
	ObjectType    string           `json:"object_type"`
	TotalRefs     int              `json:"total_refs"`      // total reference count
	AffectedCount int              `json:"affected_count"`  // distinct callers affected
	Refs          []RenameRefEntry `json:"refs"`
	Risks         []RenameRisk     `json:"risks"`
}

// RenameRefRow represents a reverse cross-reference row for rename analysis.
type RenameRefRow struct {
	CallerInclude string // who references the target
	TargetName    string // what is referenced (should match old name)
	RefType       string // OTYPE/TYPE code
	Source        string // "WBCROSSGT" or "CROSS"
}

// ComputeRenamePreview analyzes what references would need updating
// if an object is renamed from oldName to newName.
// Read-only analysis — no changes are made.
func ComputeRenamePreview(objType, oldName, newName string, refs []RenameRefRow) *RenamePreviewResult {
	oldUpper := strings.ToUpper(strings.TrimSpace(oldName))
	newUpper := strings.ToUpper(strings.TrimSpace(newName))

	result := &RenamePreviewResult{
		OldName:    oldUpper,
		NewName:    newUpper,
		ObjectType: strings.ToUpper(strings.TrimSpace(objType)),
	}

	// Aggregate references by caller object
	type callerAgg struct {
		callerType string
		count      int
		sources    map[string]bool
		exact      bool // at least one exact name match (not just LIKE prefix)
	}
	callers := make(map[string]*callerAgg)

	for _, ref := range refs {
		targetUpper := strings.ToUpper(strings.TrimSpace(ref.TargetName))
		include := strings.TrimSpace(ref.CallerInclude)

		if include == "" || targetUpper == "" {
			continue
		}

		// Skip component-internal refs
		if strings.Contains(targetUpper, "\\") {
			continue
		}

		// Normalize caller
		_, callerType, callerName := NormalizeInclude(include)
		callerUpper := strings.ToUpper(callerName)

		// Skip self-references
		if strings.EqualFold(callerUpper, oldUpper) {
			continue
		}

		callerID := NodeID(callerType, callerUpper)
		agg, ok := callers[callerID]
		if !ok {
			agg = &callerAgg{
				callerType: callerType,
				sources:    make(map[string]bool),
			}
			callers[callerID] = agg
		}
		agg.count++
		agg.sources[ref.Source] = true
		result.TotalRefs++

		// Check if this is an exact match vs LIKE prefix match
		if targetUpper == oldUpper {
			agg.exact = true
		}
	}

	// Build entries
	entries := make([]RenameRefEntry, 0, len(callers))
	for callerID, agg := range callers {
		parts := strings.SplitN(callerID, ":", 2)
		name := callerID
		if len(parts) == 2 {
			name = parts[1]
		}

		// Combine sources
		var sourceList []string
		for s := range agg.sources {
			sourceList = append(sourceList, s)
		}
		sort.Strings(sourceList)
		source := strings.Join(sourceList, "+")

		confidence := "HIGH"
		if !agg.exact {
			confidence = "MEDIUM" // only LIKE prefix match, may be false positive
		}

		entries = append(entries, RenameRefEntry{
			CallerID:   callerID,
			CallerName: name,
			CallerType: agg.callerType,
			RefCount:   agg.count,
			Source:     source,
			Confidence: confidence,
		})
	}

	// Sort: HIGH confidence first, then by ref count desc, then by caller ID
	sort.Slice(entries, func(i, j int) bool {
		ci := confidenceRank(entries[i].Confidence)
		cj := confidenceRank(entries[j].Confidence)
		if ci != cj {
			return ci < cj
		}
		if entries[i].RefCount != entries[j].RefCount {
			return entries[i].RefCount > entries[j].RefCount
		}
		return entries[i].CallerID < entries[j].CallerID
	})

	result.Refs = entries
	result.AffectedCount = len(entries)

	// Compute risks
	result.Risks = computeRenameRisks(objType, oldUpper, newUpper)

	return result
}

// computeRenameRisks generates known risk warnings for a rename operation.
func computeRenameRisks(objType, oldName, newName string) []RenameRisk {
	var risks []RenameRisk

	// Name length overflow
	if len(newName) > 30 {
		risks = append(risks, RenameRisk{
			Kind:        "NAME_OVERFLOW",
			Description: "New name exceeds 30 characters (SAP maximum for most object types)",
		})
	}

	// Dynamic call risk (always present for FMs and classes)
	switch strings.ToUpper(objType) {
	case "FUNC", "FUGR":
		risks = append(risks, RenameRisk{
			Kind:        "DYNAMIC_CALL",
			Description: "CALL FUNCTION with variable name (e.g., CALL FUNCTION lv_fm) will not be detected. Grep source for '" + oldName + "' in string literals after rename.",
		})
	case "CLAS":
		risks = append(risks, RenameRisk{
			Kind:        "DYNAMIC_CALL",
			Description: "CREATE OBJECT TYPE (variable) and dynamic method calls will not be detected. Check for '" + oldName + "' in string literals and TVARVC/config tables.",
		})
	case "PROG":
		risks = append(risks, RenameRisk{
			Kind:        "DYNAMIC_CALL",
			Description: "SUBMIT (variable) will not be detected. Check for '" + oldName + "' in variants, selection texts, and job definitions.",
		})
	}

	// String literal risk (always present)
	risks = append(risks, RenameRisk{
		Kind:        "STRING_LITERAL",
		Description: "Object name may appear in string literals, message texts, or documentation that CROSS/WBCROSSGT do not track.",
	})

	// Config reference risk
	risks = append(risks, RenameRisk{
		Kind:        "CONFIG_REF",
		Description: "Object name may appear in customizing tables (TVARVC, config entries), variant values, or external system references.",
	})

	return risks
}
