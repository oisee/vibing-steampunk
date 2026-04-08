package graph

// HealthResult is the aggregated health snapshot for a package or object.
type HealthResult struct {
	Scope   HealthScope   `json:"scope"`
	Summary HealthSummary `json:"summary"`
	Signals HealthSignals `json:"signals"`
}

// HealthScope identifies what was analyzed.
type HealthScope struct {
	Kind string `json:"kind"` // "package" or "object"
	Name string `json:"name"`
}

// HealthSummary is the top-level verdict.
type HealthSummary struct {
	Status   string `json:"status"`   // GOOD, WARN, BAD
	Headline string `json:"headline"`
}

// HealthSignals contains individual signal results.
type HealthSignals struct {
	Tests      *HealthSignal `json:"tests,omitempty"`
	ATC        *HealthSignal `json:"atc,omitempty"`
	Boundaries *HealthSignal `json:"boundaries,omitempty"`
	Staleness  *HealthSignal `json:"staleness,omitempty"`
}

// HealthSignal is one signal's result.
type HealthSignal struct {
	Status  string         `json:"status"` // signal-specific: PASS/FAIL/NONE/CLEAN/FINDINGS/VIOLATIONS/ACTIVE/AGING/STALE/ERROR/UNKNOWN
	Details map[string]any `json:"details,omitempty"`
}

// ComputeHealthSummary derives the top-level verdict from individual signals.
func ComputeHealthSummary(signals *HealthSignals) HealthSummary {
	// BAD: tests fail
	if signals.Tests != nil && signals.Tests.Status == "FAIL" {
		return HealthSummary{Status: "BAD", Headline: "Unit tests failing"}
	}

	// WARN: boundary violations or ATC findings
	var warnings []string
	if signals.Boundaries != nil && signals.Boundaries.Status == "VIOLATIONS" {
		warnings = append(warnings, "boundary violations")
	}
	if signals.ATC != nil && signals.ATC.Status == "FINDINGS" {
		warnings = append(warnings, "ATC findings")
	}
	if signals.Staleness != nil && signals.Staleness.Status == "STALE" {
		warnings = append(warnings, "stale code")
	}
	if len(warnings) > 0 {
		headline := warnings[0]
		for i := 1; i < len(warnings); i++ {
			headline += " + " + warnings[i]
		}
		return HealthSummary{Status: "WARN", Headline: headline}
	}

	return HealthSummary{Status: "GOOD", Headline: "All signals clean"}
}
