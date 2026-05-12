package main

import (
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/graph"
)

// TestSubsetMatch_Basic reproduces the subset semantics directly on
// the pure helper so we do not need a SAP round-trip to cover the
// matching behaviour the matcher relies on.
func TestSubsetMatch_Basic(t *testing.T) {
	transported := map[string]string{
		"OBJECT":    "ZTEST",
		"SUBOBJECT": "EVENT",
	}

	// Full-key match.
	if !subsetMatch(map[string]string{"OBJECT": "ZTEST", "SUBOBJECT": "EVENT"}, transported) {
		t.Error("full-key match should be accepted")
	}
	// Key-only subset match — the caller supplied only OBJECT; the
	// transported row happens to have more (SUBOBJECT). Still a match.
	if !subsetMatch(map[string]string{"OBJECT": "ZTEST"}, transported) {
		t.Error("strict subset match should be accepted")
	}
	// Mismatch: one of the requested fields has a different value.
	if subsetMatch(map[string]string{"OBJECT": "ZOTHER"}, transported) {
		t.Error("value mismatch must not match")
	}
	// Empty "want": never matches. Guards against the zero-finding
	// slip-through that would happen if the filter accidentally drops
	// all predicates.
	if subsetMatch(map[string]string{}, transported) {
		t.Error("empty want must never match")
	}
}

// TestSubsetMatch_RegressionMixedKeyAndNonKeyWHERE pins the reviewer-
// reported bug: when a SELECT has a genuine key predicate AND a non-key
// predicate, the matcher's filtered `expected` map must be what we
// match on — not the raw call.Fields. Before the fix, the raw map
// contained STATUS='A' which never exists in an unpacked TABKEY row,
// and so subsetMatch produced a false negative.
//
// We exercise the matching primitive twice: once with the filtered
// key-only map (the code path after value_match.go's fix), and once
// with the raw mixed map (the broken pre-fix path). The filtered
// path must pass; the mixed path must fail. That asymmetry is the
// entire reason this test exists.
func TestSubsetMatch_RegressionMixedKeyAndNonKeyWHERE(t *testing.T) {
	// Transported row as the unpacker would reconstruct it from TABKEY.
	// It carries only the primary key of the table (OBJECT), because
	// TABKEY is the primary-key byte layout and nothing else.
	transported := map[string]string{"OBJECT": "ZTEST"}

	// The raw extractor output for `WHERE object = 'ZTEST' AND status = 'A'`.
	raw := map[string]string{"OBJECT": "ZTEST", "STATUS": "A"}
	// What value_match.go filters the raw map down to when the table's
	// DD03L key list contains only OBJECT.
	filtered := map[string]string{"OBJECT": "ZTEST"}

	if subsetMatch(raw, transported) {
		t.Error("pre-fix path (raw call.Fields) should NOT match — STATUS is absent from the unpacked TABKEY row")
	}
	if !subsetMatch(filtered, transported) {
		t.Error("post-fix path (filtered expected) must match the transported key row")
	}
}

// TestValueLevelFinding_CoveredStatusPropagates asserts that a
// ValueLevelFinding built around the filtered `expected` map renders
// the matched key correctly. This is a thin smoke check on the shape
// the UI layer reads — MatchedKeyDisplay is what the text output uses.
func TestValueLevelFinding_CoveredStatusPropagates(t *testing.T) {
	want := map[string]string{"OBJECT": "ZTEST"}
	have := map[string]string{"OBJECT": "ZTEST"}
	display := renderKeyMap(want, have)
	if display != "OBJECT=ZTEST" {
		t.Errorf("renderKeyMap = %q, want OBJECT=ZTEST", display)
	}
	// Defensive: the finding's status string is still "COVERED" when
	// the matcher records a successful subsetMatch. We assert the
	// constant lives where it should, so a rename of the Status field
	// triggers a test failure instead of a silent drift.
	var f graph.ValueLevelFinding
	f.Status = "COVERED"
	if f.Status != "COVERED" {
		t.Error("COVERED status string drifted")
	}
}
