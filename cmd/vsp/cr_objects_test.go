package main

import (
	"context"
	"testing"
)

// TestResolveLIMUParent covers the pure subset of LIMU→parent resolution
// that does not require a SAP client (every kind except FUNC). FUNC relies
// on a TFDIR round-trip and is integration-tested elsewhere.
//
// All identifiers are synthetic — real customer object names must never
// appear in committed tests.
func TestResolveLIMUParent(t *testing.T) {
	cases := []struct {
		name       string
		object     string
		objName    string
		wantType   string
		wantParent string
	}{
		{
			name:       "METH with padded class and method",
			object:     "METH",
			objName:    "ZCL_TEST_EXAMPLE              EXAMPLE_METHOD_NAME",
			wantType:   "CLAS",
			wantParent: "ZCL_TEST_EXAMPLE",
		},
		{
			name:       "METH with tab separator",
			object:     "METH",
			objName:    "ZCL_TEST_FOO\tMETHOD_NAME",
			wantType:   "CLAS",
			wantParent: "ZCL_TEST_FOO",
		},
		{
			name:       "METH no separator — whole thing is class",
			object:     "METH",
			objName:    "ZCL_TEST_BAR",
			wantType:   "CLAS",
			wantParent: "ZCL_TEST_BAR",
		},
		{
			name:       "CPRI is class header",
			object:     "CPRI",
			objName:    "ZCL_TEST_SETTINGS",
			wantType:   "CLAS",
			wantParent: "ZCL_TEST_SETTINGS",
		},
		{
			name:       "CPRO is class header",
			object:     "CPRO",
			objName:    "ZCL_TEST_X",
			wantType:   "CLAS",
			wantParent: "ZCL_TEST_X",
		},
		{
			name:       "CPUB is class header",
			object:     "CPUB",
			objName:    "ZCL_TEST_Y",
			wantType:   "CLAS",
			wantParent: "ZCL_TEST_Y",
		},
		{
			name:       "CLSD is class definition",
			object:     "CLSD",
			objName:    "ZCL_TEST_Z",
			wantType:   "CLAS",
			wantParent: "ZCL_TEST_Z",
		},
		{
			name:       "CINC is class include",
			object:     "CINC",
			objName:    "ZCL_TEST_A",
			wantType:   "CLAS",
			wantParent: "ZCL_TEST_A",
		},
		{
			name:       "INTD is interface definition",
			object:     "INTD",
			objName:    "ZIF_TEST_Q",
			wantType:   "INTF",
			wantParent: "ZIF_TEST_Q",
		},
		{
			name:       "REPS maps to PROG",
			object:     "REPS",
			objName:    "ZTEST_REPORT_SYNC",
			wantType:   "PROG",
			wantParent: "ZTEST_REPORT_SYNC",
		},
		{
			name:       "TABD maps to TABL",
			object:     "TABD",
			objName:    "ZTEST_SCREEN_RESULT",
			wantType:   "TABL",
			wantParent: "ZTEST_SCREEN_RESULT",
		},
		{
			name:       "DTED maps to DTEL",
			object:     "DTED",
			objName:    "ZTEST_PILE_ID",
			wantType:   "DTEL",
			wantParent: "ZTEST_PILE_ID",
		},
		{
			name:       "DOMD maps to DOMA",
			object:     "DOMD",
			objName:    "ZDOM_TEST",
			wantType:   "DOMA",
			wantParent: "ZDOM_TEST",
		},
		{
			name:       "Unknown LIMU kind returns empty",
			object:     "DOCU",
			objName:    "ZTEST_DOC",
			wantType:   "",
			wantParent: "",
		},
		{
			name:       "MESS is documentation-adjacent, ignored",
			object:     "MESS",
			objName:    "ZTEST_MSG",
			wantType:   "",
			wantParent: "",
		},
		{
			name:       "Case normalised to upper",
			object:     "CLSD",
			objName:    "zcl_test_lowercase",
			wantType:   "CLAS",
			wantParent: "ZCL_TEST_LOWERCASE",
		},
	}

	// FUNC lookups touch TFDIR → not pure, skipped in this unit test. When
	// fmToFugr already has an entry the helper returns it synchronously, so
	// that case IS testable:
	cases = append(cases, struct {
		name       string
		object     string
		objName    string
		wantType   string
		wantParent string
	}{
		name:       "FUNC with pre-populated cache",
		object:     "FUNC",
		objName:    "Z_TEST_FM_INSERT",
		wantType:   "FUGR",
		wantParent: "ZTEST_FG",
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cache := map[string]string{}
			if tc.object == "FUNC" {
				cache["Z_TEST_FM_INSERT"] = "ZTEST_FG"
			}
			gotType, gotParent := resolveLIMUParent(tc.object, tc.objName, nil, context.Background(), cache)
			if gotType != tc.wantType || gotParent != tc.wantParent {
				t.Errorf("resolveLIMUParent(%q, %q) = (%q, %q); want (%q, %q)",
					tc.object, tc.objName, gotType, gotParent, tc.wantType, tc.wantParent)
			}
		})
	}
}

// TestTadirRowMatches covers the permissive-match logic between TADIR-reported
// object type and the type we guessed from E071/LIMU resolution.
func TestTadirRowMatches(t *testing.T) {
	cases := []struct {
		r    tadirRow
		want string
		ok   bool
	}{
		{tadirRow{objType: "CLAS"}, "CLAS", true},
		{tadirRow{objType: "CLAS"}, "PROG", false},
		// Interface / class interchangeability on older systems.
		{tadirRow{objType: "CLAS"}, "INTF", true},
		{tadirRow{objType: "INTF"}, "CLAS", false},
		{tadirRow{objType: "PROG"}, "PROG", true},
		{tadirRow{objType: "TABL"}, "TABL", true},
	}
	for _, c := range cases {
		if got := c.r.matches(c.want); got != c.ok {
			t.Errorf("matches(%s→%s) = %v; want %v", c.r.objType, c.want, got, c.ok)
		}
	}
}
