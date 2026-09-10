package adt

import "testing"

// Real ST22 chapter layout, synthetic names. The System fields grid, the Source
// Code Extract with SAP's "=====> Error" marker, and the Selected Variables
// chapter — including a value that wraps with a trailing "\" the way long hex
// dumps do — are all taken structurally from a live dump and rewritten with
// invented names. Chapters are separated by a blank line, as on the wire.
const formattedDumpEnrichSample = `
----------------------------------------------------------------------------------------------------
Category               ABAP programming error
Runtime Errors         MESSAGE_TYPE_X
ABAP: Program          ZCL_DEMO_FLUSH===============CP
Application Component   BC-FES-CTL
Date and Time          10.09.2026 21:41:27 (UTC)
----------------------------------------------------------------------------------------------------

----------------------------------------------------------------------------------------------------
|Short Text                                                                                        |
|    The current application has intentionally triggered a termination.                            |
----------------------------------------------------------------------------------------------------

----------------------------------------------------------------------------------------------------
|Source Code Extract                                                                               |
----------------------------------------------------------------------------------------------------
|Line |Code                                                                                        |
----------------------------------------------------------------------------------------------------
|  538|        IF SY-SUBRC <> 0.                                                                   |
|  539|          MESSAGE X373 WITH SY-SUBRC."=================> Error                              |
|  540|        ENDIF.                                                                              |
----------------------------------------------------------------------------------------------------

----------------------------------------------------------------------------------------------------
|Contents of system fields                                                                         |
----------------------------------------------------------------------------------------------------
|Name    |Val.                                                                                     |
----------------------------------------------------------------------------------------------------
|SY-SUBRC|4                                                                                        |
|SY-FDPOS|40                                                                                       |
|SY-MSGID|SY                                                                                       |
|SY-MSGNO|373                                                                                      |
|SY-MSGV1|-1                                                                                       |
|SY-PFKEY|SESSION_ADMIN                                                                            |
|SY-TITLE|SAP Easy Access                                                                          |
----------------------------------------------------------------------------------------------------

----------------------------------------------------------------------------------------------------
|Selected Variables                                                                                |
----------------------------------------------------------------------------------------------------
|Name                                                                                              |
|    Val.                                                                                          |
----------------------------------------------------------------------------------------------------
|No.       6 Ty.          FUNCTION                                                                 |
|Name  AC_FLUSH_CALL_INTERNAL                                                                      |
----------------------------------------------------------------------------------------------------
|SYSTEM_FLUSH                                                                                      |
|    X                                                                                             |
|    5800                                                                                          |
|MESSAGE_TEXT                                                                                      |
|    22222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222\|
|00002222222222                                                                                    |
|RESULT_VARS_WA                                                                                    |
|    POINTER = 0x0000000000000000                                                                  |
----------------------------------------------------------------------------------------------------

----------------------------------------------------------------------------------------------------
|Information About Memory Usage                                                                     |
----------------------------------------------------------------------------------------------------
`

func TestDumpDetailReadsSystemFields(t *testing.T) {
	d := parseDumpDetail(formattedDumpEnrichSample)
	if d.SystemFields["SY-MSGNO"] != "373" {
		t.Errorf("SY-MSGNO = %q", d.SystemFields["SY-MSGNO"])
	}
	if d.SystemFields["SY-MSGV1"] != "-1" {
		t.Errorf("SY-MSGV1 = %q", d.SystemFields["SY-MSGV1"])
	}
	if d.SystemFields["SY-TITLE"] != "SAP Easy Access" {
		t.Errorf("SY-TITLE = %q", d.SystemFields["SY-TITLE"])
	}
	if _, ok := d.SystemFields["Name"]; ok {
		t.Error("the column header leaked in as a field")
	}
}

func TestDumpDetailReadsSourceAndFlagsTheFailure(t *testing.T) {
	d := parseDumpDetail(formattedDumpEnrichSample)
	if len(d.Source) != 3 {
		t.Fatalf("source lines = %d, want 3", len(d.Source))
	}
	var failed []int
	for _, s := range d.Source {
		if s.Failed {
			failed = append(failed, s.Line)
		}
	}
	if len(failed) != 1 || failed[0] != 539 {
		t.Errorf("failed lines = %v, want [539]", failed)
	}
}

func TestDumpDetailReadsSelectedVariablesAndJoinsWraps(t *testing.T) {
	d := parseDumpDetail(formattedDumpEnrichSample)
	byName := map[string]DumpVariable{}
	for _, v := range d.Variables {
		byName[v.Name] = v
	}
	if v, ok := byName["SYSTEM_FLUSH"]; !ok || v.Frame != "AC_FLUSH_CALL_INTERNAL" || v.Value != "X 5800" {
		t.Errorf("SYSTEM_FLUSH = %+v", v)
	}
	// The wrapped hex continuation must fold into MESSAGE_TEXT, not appear as
	// its own variable named after the second half of the hex.
	if _, leaked := byName["00002222222222"]; leaked {
		t.Error("a wrapped hex continuation leaked in as a variable name")
	}
	mt := byName["MESSAGE_TEXT"].Value
	if len(mt) < 90 || mt[:6] != "222222" {
		t.Errorf("MESSAGE_TEXT did not fold its wrap: %q", mt)
	}
	if _, ok := byName["RESULT_VARS_WA"]; !ok {
		t.Error("RESULT_VARS_WA missing")
	}
}
