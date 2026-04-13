package main

import (
	"testing"
)

// TestExtractCodeLiterals_KnownCallFullKey covers the canonical v2a-min
// case: a CALL FUNCTION to a registered customizing FM with every
// business-key parameter supplied as a string literal.
func TestExtractCodeLiterals_KnownCallFullKey(t *testing.T) {
	// Synthetic ABAP fixture modelled on the R15 post-import miss: code
	// asks APPL_LOG_INIT for (OBJECT='ZTEST_LOG', SUBOBJECT='EVENT'). All
	// identifiers are test-only — no customer names anywhere.
	source := `
REPORT ztest_value_extractor.
DATA lv_handle TYPE balloghndl.
CALL FUNCTION 'APPL_LOG_INIT'
  EXPORTING
    object     = 'ZTEST_LOG'
    sub_object = 'EVENT'
  IMPORTING
    log_handle = lv_handle.
`
	got := extractCodeLiterals("PROG:ZTEST_VALUE_EXTRACTOR", source)
	if len(got) != 1 {
		t.Fatalf("expected 1 call site, got %d: %+v", len(got), got)
	}
	c := got[0]
	if c.Target != "BALSUB" {
		t.Errorf("Target = %q, want BALSUB", c.Target)
	}
	if c.Fields["OBJECT"] != "ZTEST_LOG" {
		t.Errorf("Fields[OBJECT] = %q, want ZTEST_LOG", c.Fields["OBJECT"])
	}
	if c.Fields["SUBOBJECT"] != "EVENT" {
		t.Errorf("Fields[SUBOBJECT] = %q, want EVENT", c.Fields["SUBOBJECT"])
	}
	if c.IncompleteKey {
		t.Error("IncompleteKey = true, want false for full-key call")
	}
	if c.Kind != "known_call" {
		t.Errorf("Kind = %q, want known_call", c.Kind)
	}
	if c.Via != "CALL_FUNCTION:APPL_LOG_INIT" {
		t.Errorf("Via = %q, want CALL_FUNCTION:APPL_LOG_INIT", c.Via)
	}
}

// TestExtractCodeLiterals_PartialKey covers the case where the code
// supplies only a subset of the registered business key fields. The
// extractor must still produce a finding but flag IncompleteKey=true so
// the matcher and the report can treat it as "less confident".
func TestExtractCodeLiterals_PartialKey(t *testing.T) {
	source := `
CALL FUNCTION 'APPL_LOG_INIT'
  EXPORTING
    object = 'ZTEST_LOG'.
`
	got := extractCodeLiterals("PROG:ZTEST", source)
	if len(got) != 1 {
		t.Fatalf("expected 1 call site, got %d", len(got))
	}
	if !got[0].IncompleteKey {
		t.Error("IncompleteKey = false, want true (missing sub_object)")
	}
	if got[0].Fields["OBJECT"] != "ZTEST_LOG" {
		t.Errorf("OBJECT = %q, want ZTEST_LOG", got[0].Fields["OBJECT"])
	}
}

// TestExtractCodeLiterals_DynamicFMSkipped covers the "runtime-computed
// FM name" shape. We intentionally drop these — no literal, no lookup.
func TestExtractCodeLiterals_DynamicFMSkipped(t *testing.T) {
	source := `
DATA lv_fm TYPE string VALUE 'APPL_LOG_INIT'.
CALL FUNCTION lv_fm
  EXPORTING
    object = 'ZTEST'
    sub_object = 'EVENT'.
`
	got := extractCodeLiterals("PROG:ZTEST", source)
	if len(got) != 0 {
		t.Errorf("expected 0 findings for dynamic FM, got %d: %+v", len(got), got)
	}
}

// TestExtractCodeLiterals_UnregisteredFMIgnored makes sure the extractor
// stays silent on CALL FUNCTION to an FM we do not track.
func TestExtractCodeLiterals_UnregisteredFMIgnored(t *testing.T) {
	source := `
CALL FUNCTION 'ZZZ_NOT_IN_REGISTRY'
  EXPORTING
    object = 'ZTEST_LOG'
    sub_object = 'EVENT'.
`
	got := extractCodeLiterals("PROG:ZTEST", source)
	if len(got) != 0 {
		t.Errorf("expected 0 findings for unregistered FM, got %d", len(got))
	}
}

// TestExtractCodeLiterals_HostVarResolved covers the case where the
// value side of EXPORTING is a host variable that was declared with a
// VALUE clause. The pre-pass should record the assignment and the
// extractor should resolve the variable to its literal value.
func TestExtractCodeLiterals_HostVarResolved(t *testing.T) {
	source := `
DATA lv_obj TYPE string VALUE 'ZTEST'.
CALL FUNCTION 'APPL_LOG_INIT'
  EXPORTING
    object = lv_obj
    sub_object = 'EVENT'.
`
	got := extractCodeLiterals("PROG:ZTEST", source)
	if len(got) != 1 {
		t.Fatalf("expected 1 call site, got %d", len(got))
	}
	c := got[0]
	if c.Fields["OBJECT"] != "ZTEST" {
		t.Errorf("OBJECT = %q, want ZTEST (resolved from DATA … VALUE)", c.Fields["OBJECT"])
	}
	if c.Fields["SUBOBJECT"] != "EVENT" {
		t.Errorf("SUBOBJECT = %q, want EVENT", c.Fields["SUBOBJECT"])
	}
	if c.IncompleteKey {
		t.Error("IncompleteKey = true, want false — both fields resolved")
	}
}

// TestExtractCodeLiterals_UnresolvedVarStillIncomplete covers the
// degenerate case: a variable that was *not* declared with a VALUE
// clause and is therefore truly dynamic. The extractor should skip
// that parameter and flag IncompleteKey on the resulting finding.
func TestExtractCodeLiterals_UnresolvedVarStillIncomplete(t *testing.T) {
	source := `
DATA lv_obj TYPE string.
lv_obj = compute_runtime_value( ).
CALL FUNCTION 'APPL_LOG_INIT'
  EXPORTING
    object = lv_obj
    sub_object = 'EVENT'.
`
	got := extractCodeLiterals("PROG:ZTEST", source)
	if len(got) != 1 {
		t.Fatalf("expected 1 call site, got %d", len(got))
	}
	c := got[0]
	if _, ok := c.Fields["OBJECT"]; ok {
		t.Errorf("OBJECT should not be set — variable has no static literal, got %q", c.Fields["OBJECT"])
	}
	if c.Fields["SUBOBJECT"] != "EVENT" {
		t.Errorf("SUBOBJECT = %q, want EVENT", c.Fields["SUBOBJECT"])
	}
	if !c.IncompleteKey {
		t.Error("IncompleteKey = false, want true — OBJECT could not be resolved")
	}
}

// TestExtractCodeLiterals_ConstantResolved covers the most common real
// idiom: module-level CONSTANTS declarations referenced from a SELECT
// or CALL FUNCTION elsewhere in the same source unit.
func TestExtractCodeLiterals_ConstantResolved(t *testing.T) {
	source := `
CONSTANTS gc_obj TYPE c LENGTH 30 VALUE 'ZTEST_LOG'.
SELECT * FROM ztest_t WHERE object = gc_obj.
`
	got := extractCodeLiterals("PROG:ZTEST", source)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(got), got)
	}
	if got[0].Fields["OBJECT"] != "ZTEST_LOG" {
		t.Errorf("OBJECT = %q, want ZTEST_LOG (resolved from CONSTANTS)", got[0].Fields["OBJECT"])
	}
}

// TestExtractCodeLiterals_AtHostVarResolved covers the modern
// `WHERE field = @lv_var` shape where the lexer splits `@` into a
// separate token. The extractor should step past the @ and resolve
// the host variable from the local literal map.
func TestExtractCodeLiterals_AtHostVarResolved(t *testing.T) {
	source := `
DATA lv_obj TYPE string VALUE 'ZTEST_LOG'.
SELECT * FROM ztest_t WHERE object = @lv_obj.
`
	got := extractCodeLiterals("PROG:ZTEST", source)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0].Fields["OBJECT"] != "ZTEST_LOG" {
		t.Errorf("OBJECT = %q, want ZTEST_LOG (resolved from DATA via @host-var)", got[0].Fields["OBJECT"])
	}
}

// TestExtractCodeLiterals_PlainAssignmentResolved covers the case
// where a local variable is set by a plain assignment (not a VALUE
// clause) but the right-hand side is a string literal.
func TestExtractCodeLiterals_PlainAssignmentResolved(t *testing.T) {
	source := `
DATA lv_obj TYPE string.
lv_obj = 'ZTEST_LOG'.
SELECT * FROM ztest_t WHERE object = lv_obj.
`
	got := extractCodeLiterals("PROG:ZTEST", source)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0].Fields["OBJECT"] != "ZTEST_LOG" {
		t.Errorf("OBJECT = %q, want ZTEST_LOG (resolved from plain assignment)", got[0].Fields["OBJECT"])
	}
}

// TestExtractCodeLiterals_SimpleSelect covers the v2a.1 SELECT path:
// SELECT FROM <static table> WHERE <field> = '<literal>' [AND ...] is
// captured as a finding with Kind="direct_select". Field name
// qualifiers like `t~field` and `tab-field` are stripped.
func TestExtractCodeLiterals_SimpleSelect(t *testing.T) {
	source := `
SELECT SINGLE * FROM ztest_config
  WHERE object = 'ZTEST_LOG' AND subobject = 'EVENT'
  INTO @DATA(ls_row).
`
	got := extractCodeLiterals("CLAS:ZCL_TEST", source)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(got), got)
	}
	c := got[0]
	if c.Kind != "direct_select" {
		t.Errorf("Kind = %q, want direct_select", c.Kind)
	}
	if c.Target != "ZTEST_CONFIG" {
		t.Errorf("Target = %q, want ZTEST_CONFIG", c.Target)
	}
	if c.Fields["OBJECT"] != "ZTEST_LOG" || c.Fields["SUBOBJECT"] != "EVENT" {
		t.Errorf("Fields = %+v, want {OBJECT:ZTEST_LOG, SUBOBJECT:EVENT}", c.Fields)
	}
	if c.Via != "SELECT:ZTEST_CONFIG" {
		t.Errorf("Via = %q, want SELECT:ZTEST_CONFIG", c.Via)
	}
}

// TestExtractCodeLiterals_SelectWithQualifiers covers `table~field`
// shape used in JOINs and `table-field` used in older syntax. Both
// must reduce to the bare field name.
func TestExtractCodeLiterals_SelectWithQualifiers(t *testing.T) {
	source := `
SELECT * FROM ztest_t WHERE ztest_t~object = 'ZTEST'.
SELECT * FROM ztest_t WHERE ztest_t-object = 'ZTEST'.
`
	got := extractCodeLiterals("PROG:ZTEST", source)
	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(got))
	}
	for i, c := range got {
		if c.Fields["OBJECT"] != "ZTEST" {
			t.Errorf("[%d] Fields[OBJECT] = %q, want ZTEST (qualifier strip failed)", i, c.Fields["OBJECT"])
		}
	}
}

// TestExtractCodeLiterals_SelectSkipsUnresolvedHostVar covers the
// case where the WHERE value is a host variable that has NO static
// VALUE clause and therefore cannot be resolved to a literal — those
// predicates must drop, producing no finding.
func TestExtractCodeLiterals_SelectSkipsUnresolvedHostVar(t *testing.T) {
	source := `
DATA lv_obj TYPE string.
lv_obj = compute_runtime_value( ).
SELECT * FROM ztest_t WHERE object = @lv_obj.
`
	got := extractCodeLiterals("PROG:ZTEST", source)
	if len(got) != 0 {
		t.Errorf("expected 0 findings (host var has no static value), got %d: %+v", len(got), got)
	}
}

// TestExtractCodeLiterals_SelectIgnoresInLikeNeq covers the operators
// the extractor must skip: IN, LIKE, !=, <>, <=, >=. Even with literal
// right-hand sides these are not single-value predicates.
func TestExtractCodeLiterals_SelectIgnoresInLikeNeq(t *testing.T) {
	cases := []string{
		`SELECT * FROM ztest_t WHERE object IN ( 'A' , 'B' ).`,
		`SELECT * FROM ztest_t WHERE object LIKE 'Z%'.`,
		`SELECT * FROM ztest_t WHERE object <> 'OLD'.`,
	}
	for _, src := range cases {
		got := extractCodeLiterals("PROG:ZTEST", src)
		if len(got) != 0 {
			t.Errorf("expected 0 findings for %q, got %d: %+v", src, len(got), got)
		}
	}
}

// TestExtractCodeLiterals_SelectIgnoresDynamicTable covers FROM (var)
// — a dynamic table name we cannot resolve at extract time.
func TestExtractCodeLiterals_SelectIgnoresDynamicTable(t *testing.T) {
	source := `
DATA lv_tab TYPE tabname VALUE 'ZTEST_T'.
SELECT * FROM (lv_tab) WHERE object = 'ZTEST'.
`
	got := extractCodeLiterals("PROG:ZTEST", source)
	if len(got) != 0 {
		t.Errorf("expected 0 findings for dynamic table, got %d", len(got))
	}
}

// TestExtractCodeLiterals_UpdateAndDelete covers UPDATE and DELETE
// FROM. Both should produce findings with the right Via tag.
func TestExtractCodeLiterals_UpdateAndDelete(t *testing.T) {
	source := `
UPDATE ztest_t SET status = 'DONE' WHERE object = 'ZTEST'.
DELETE FROM ztest_t WHERE object = 'OLD'.
`
	got := extractCodeLiterals("PROG:ZTEST", source)
	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d: %+v", len(got), got)
	}
	if got[0].Via != "UPDATE:ZTEST_T" {
		t.Errorf("first Via = %q, want UPDATE:ZTEST_T", got[0].Via)
	}
	if got[1].Via != "DELETE:ZTEST_T" {
		t.Errorf("second Via = %q, want DELETE:ZTEST_T", got[1].Via)
	}
}

// TestExtractCodeLiterals_ModifyWithoutWhereSkipped covers the very
// common `MODIFY itab` pattern that has no WHERE — it must not become
// a finding (not interesting for value-level audit).
func TestExtractCodeLiterals_ModifyWithoutWhereSkipped(t *testing.T) {
	source := `
MODIFY ztest_t FROM ls_row.
`
	got := extractCodeLiterals("PROG:ZTEST", source)
	if len(got) != 0 {
		t.Errorf("expected 0 findings for MODIFY without WHERE, got %d: %+v", len(got), got)
	}
}

// TestUnpackTabkey_BALSUBShape unpacks a synthetic TABKEY with the
// (OBJECT, SUBOBJECT) key layout used by BALSUB: client(3) + object(20)
// + subobject(20). Matches how SAP pads CHAR fields with trailing spaces.
// Built programmatically so space-counting mistakes cannot creep in.
func TestUnpackTabkey_BALSUBShape(t *testing.T) {
	fields := []ddKeyField{
		{Name: "OBJECT", Length: 20},
		{Name: "SUBOBJECT", Length: 20},
	}
	tabkey := "122" + padRight("ZTEST_LOG", 20) + padRight("EVENT", 20)
	got := unpackTabkey(tabkey, fields)
	if got["OBJECT"] != "ZTEST_LOG" {
		t.Errorf("OBJECT = %q, want ZTEST_LOG", got["OBJECT"])
	}
	if got["SUBOBJECT"] != "EVENT" {
		t.Errorf("SUBOBJECT = %q, want EVENT", got["SUBOBJECT"])
	}
}

// padRight is a tiny test helper that right-pads `s` with spaces to
// exactly `width` bytes — matches SAP CHAR column storage layout.
func padRight(s string, width int) string {
	for len(s) < width {
		s += " "
	}
	return s
}

// TestSubsetMatch covers the three cases v2a-min relies on.
func TestSubsetMatch(t *testing.T) {
	have := map[string]string{"OBJECT": "ZTEST_LOG", "SUBOBJECT": "EVENT"}
	// Full key match.
	if !subsetMatch(map[string]string{"OBJECT": "ZTEST_LOG", "SUBOBJECT": "EVENT"}, have) {
		t.Error("full-key subset should match")
	}
	// Partial key match (subset).
	if !subsetMatch(map[string]string{"OBJECT": "ZTEST_LOG"}, have) {
		t.Error("partial-key subset should match")
	}
	// Mismatch on one field.
	if subsetMatch(map[string]string{"OBJECT": "ZTEST_LOG", "SUBOBJECT": "SYNC"}, have) {
		t.Error("mismatch on SUBOBJECT should not match")
	}
	// Empty `want` never matches — guards against zero-finding slip-through.
	if subsetMatch(map[string]string{}, have) {
		t.Error("empty want should not match")
	}
}

// TestPostImportMissedRowsFixture is the end-to-end fixture proving
// v2a-min catches the production-incident class where a CR transports
// a customizing table but misses some of the specific row keys the
// code depends on. We stage:
//
//   - one caller source doing APPL_LOG_INIT with (ZTEST, EVENT)
//   - a synthetic transported-row table containing only (ZTEST, META)
//
// and assert the matcher lands ValueMissing for the (ZTEST, EVENT) call.
func TestPostImportMissedRowsFixture(t *testing.T) {
	source := `
CALL FUNCTION 'APPL_LOG_INIT'
  EXPORTING
    object     = 'ZTEST'
    sub_object = 'EVENT'.
CALL FUNCTION 'APPL_LOG_INIT'
  EXPORTING
    object     = 'ZTEST'
    sub_object = 'META'.
`
	literals := extractCodeLiterals("PROG:ZTEST", source)
	if len(literals) != 2 {
		t.Fatalf("expected 2 literal call sites, got %d", len(literals))
	}

	// Pretend only one transported row exists: (ZTEST, META).
	fields := []ddKeyField{
		{Name: "OBJECT", Length: 20},
		{Name: "SUBOBJECT", Length: 20},
	}
	transported := []map[string]string{
		unpackTabkey("122"+padRight("ZTEST", 20)+padRight("META", 20), fields),
	}

	missing, covered := 0, 0
	for _, c := range literals {
		matched := false
		for _, row := range transported {
			if subsetMatch(c.Fields, row) {
				matched = true
				break
			}
		}
		if matched {
			covered++
		} else {
			missing++
		}
	}
	if missing != 1 {
		t.Errorf("missing = %d, want 1 — (ZTEST, EVENT) should be the missing call", missing)
	}
	if covered != 1 {
		t.Errorf("covered = %d, want 1 — (ZTEST, META) should match the transported row", covered)
	}
}
