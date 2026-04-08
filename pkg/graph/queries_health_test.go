package graph

import "testing"

func TestComputeHealthSummary_AllGood(t *testing.T) {
	signals := &HealthSignals{
		Tests:      &HealthSignal{Status: "PASS"},
		ATC:        &HealthSignal{Status: "CLEAN"},
		Boundaries: &HealthSignal{Status: "CLEAN"},
		Staleness:  &HealthSignal{Status: "ACTIVE"},
	}
	s := ComputeHealthSummary(signals)
	if s.Status != "GOOD" {
		t.Errorf("Status: got %q, want GOOD", s.Status)
	}
}

func TestComputeHealthSummary_TestsFail(t *testing.T) {
	signals := &HealthSignals{
		Tests:      &HealthSignal{Status: "FAIL"},
		ATC:        &HealthSignal{Status: "CLEAN"},
		Boundaries: &HealthSignal{Status: "CLEAN"},
	}
	s := ComputeHealthSummary(signals)
	if s.Status != "BAD" {
		t.Errorf("Status: got %q, want BAD", s.Status)
	}
}

func TestComputeHealthSummary_BoundaryViolations(t *testing.T) {
	signals := &HealthSignals{
		Tests:      &HealthSignal{Status: "PASS"},
		Boundaries: &HealthSignal{Status: "VIOLATIONS"},
	}
	s := ComputeHealthSummary(signals)
	if s.Status != "WARN" {
		t.Errorf("Status: got %q, want WARN", s.Status)
	}
}

func TestComputeHealthSummary_ATCFindings(t *testing.T) {
	signals := &HealthSignals{
		ATC: &HealthSignal{Status: "FINDINGS"},
	}
	s := ComputeHealthSummary(signals)
	if s.Status != "WARN" {
		t.Errorf("Status: got %q, want WARN", s.Status)
	}
}

func TestComputeHealthSummary_MultipleWarnings(t *testing.T) {
	signals := &HealthSignals{
		Tests:      &HealthSignal{Status: "PASS"},
		ATC:        &HealthSignal{Status: "FINDINGS"},
		Boundaries: &HealthSignal{Status: "VIOLATIONS"},
		Staleness:  &HealthSignal{Status: "STALE"},
	}
	s := ComputeHealthSummary(signals)
	if s.Status != "WARN" {
		t.Errorf("Status: got %q, want WARN", s.Status)
	}
	// Headline should mention all warnings
	if s.Headline == "" {
		t.Error("Headline should not be empty")
	}
}

func TestComputeHealthSummary_NoSignals(t *testing.T) {
	signals := &HealthSignals{}
	s := ComputeHealthSummary(signals)
	if s.Status != "GOOD" {
		t.Errorf("Status: got %q, want GOOD (missing data is not failure)", s.Status)
	}
}

func TestComputeHealthSummary_TestsNone(t *testing.T) {
	signals := &HealthSignals{
		Tests: &HealthSignal{Status: "NONE"},
		ATC:   &HealthSignal{Status: "CLEAN"},
	}
	s := ComputeHealthSummary(signals)
	if s.Status != "GOOD" {
		t.Errorf("Status: got %q, want GOOD (no tests = not failure)", s.Status)
	}
}

func TestComputeHealthSummary_TestFailOverridesWarn(t *testing.T) {
	signals := &HealthSignals{
		Tests:      &HealthSignal{Status: "FAIL"},
		ATC:        &HealthSignal{Status: "FINDINGS"},
		Boundaries: &HealthSignal{Status: "VIOLATIONS"},
	}
	s := ComputeHealthSummary(signals)
	if s.Status != "BAD" {
		t.Errorf("Status: got %q, want BAD (test fail overrides warn)", s.Status)
	}
}
