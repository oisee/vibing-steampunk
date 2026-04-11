package main

import (
	"testing"
)

func TestBuildChangesResult_GroupsByReference(t *testing.T) {
	entries := []changelogEntry{
		{Transport: "A4HK900100", Date: "20260407", User: "DEV", Description: "Fix A",
			Objects: []changelogObject{{Type: "CLAS", Name: "ZCL_A"}}},
		{Transport: "A4HK900200", Date: "20260406", User: "DEV", Description: "Fix B",
			Objects: []changelogObject{{Type: "CLAS", Name: "ZCL_B"}}},
		{Transport: "A4HK900300", Date: "20260405", User: "ADMIN", Description: "Cleanup",
			Objects: []changelogObject{{Type: "PROG", Name: "ZCLEANUP"}}},
	}
	attrMap := map[string][]string{
		"A4HK900100": {"CR-001"},
		"A4HK900200": {"CR-001"},
		// A4HK900300 has no attribute → untagged
	}

	result := buildChangesResult("$ZDEV", "ZCR", "", 3, entries, attrMap, 0)

	if result.TotalGroups != 1 {
		t.Fatalf("TotalGroups = %d, want 1", result.TotalGroups)
	}
	if result.TotalUntagged != 1 {
		t.Fatalf("TotalUntagged = %d, want 1", result.TotalUntagged)
	}
	if len(result.Groups) != 1 {
		t.Fatalf("len(Groups) = %d, want 1", len(result.Groups))
	}
	if result.Groups[0].Reference != "CR-001" {
		t.Fatalf("Reference = %q, want CR-001", result.Groups[0].Reference)
	}
	if len(result.Groups[0].Transports) != 2 {
		t.Fatalf("Transports in CR-001 = %d, want 2", len(result.Groups[0].Transports))
	}
	if len(result.Untagged) != 1 {
		t.Fatalf("Untagged = %d, want 1", len(result.Untagged))
	}
	if result.Untagged[0].Transport != "A4HK900300" {
		t.Fatalf("Untagged transport = %q, want A4HK900300", result.Untagged[0].Transport)
	}
}

func TestBuildChangesResult_MultiValueAttribute(t *testing.T) {
	entries := []changelogEntry{
		{Transport: "A4HK900100", Date: "20260407", User: "DEV",
			Objects: []changelogObject{{Type: "CLAS", Name: "ZCL_A"}}},
	}
	attrMap := map[string][]string{
		"A4HK900100": {"CR-001", "CR-002"}, // transport tagged with two CRs
	}

	result := buildChangesResult("$ZDEV", "ZCR", "", 1, entries, attrMap, 0)

	if result.TotalGroups != 2 {
		t.Fatalf("TotalGroups = %d, want 2 (transport appears in both groups)", result.TotalGroups)
	}
}

func TestBuildChangesResult_TopNLimitsGroups(t *testing.T) {
	entries := []changelogEntry{
		{Transport: "A4HK900100", Date: "20260407", User: "DEV"},
		{Transport: "A4HK900200", Date: "20260406", User: "DEV"},
		{Transport: "A4HK900300", Date: "20260405", User: "DEV"},
	}
	attrMap := map[string][]string{
		"A4HK900100": {"CR-001"},
		"A4HK900200": {"CR-002"},
		"A4HK900300": {"CR-003"},
	}

	result := buildChangesResult("$ZDEV", "ZCR", "", 3, entries, attrMap, 2)

	if len(result.Groups) != 2 {
		t.Fatalf("len(Groups) = %d, want 2 (topN=2)", len(result.Groups))
	}
	// TotalGroups reflects all groups before truncation
	if result.TotalGroups != 3 {
		t.Fatalf("TotalGroups = %d, want 3", result.TotalGroups)
	}
	if result.TotalTransports != 3 {
		t.Fatalf("TotalTransports = %d, want 3", result.TotalTransports)
	}
}

func TestBuildChangesResult_SortsByLatestDate(t *testing.T) {
	entries := []changelogEntry{
		{Transport: "A4HK900100", Date: "20260401", User: "DEV"},
		{Transport: "A4HK900200", Date: "20260407", User: "DEV"},
	}
	attrMap := map[string][]string{
		"A4HK900100": {"CR-OLD"},
		"A4HK900200": {"CR-NEW"},
	}

	result := buildChangesResult("$ZDEV", "ZCR", "", 2, entries, attrMap, 0)

	if result.Groups[0].Reference != "CR-NEW" {
		t.Fatalf("First group = %q, want CR-NEW (most recent)", result.Groups[0].Reference)
	}
}

func TestBuildChangesResult_AllUntagged(t *testing.T) {
	entries := []changelogEntry{
		{Transport: "A4HK900100", Date: "20260407", User: "DEV"},
	}
	attrMap := map[string][]string{} // no attributes at all

	result := buildChangesResult("$ZDEV", "ZCR", "", 1, entries, attrMap, 0)

	if result.TotalGroups != 0 {
		t.Fatalf("TotalGroups = %d, want 0", result.TotalGroups)
	}
	if result.TotalUntagged != 1 {
		t.Fatalf("TotalUntagged = %d, want 1", result.TotalUntagged)
	}
}
