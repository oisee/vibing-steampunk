package main

import (
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/graph"
)

// TestIncludeToParent covers the reverse mapping from SAP include
// names to their owning objects. This is the fragile heuristic at the
// heart of transitive reach — everything downstream assumes that a
// given INCLUDE routes to the right parent object.
func TestIncludeToParent(t *testing.T) {
	cases := []struct {
		name     string
		include  string
		wantType string
		wantName string
	}{
		{
			name:     "class program include CP",
			include:  "ZCL_TEST========CP",
			wantType: "CLAS",
			wantName: "ZCL_TEST",
		},
		{
			name:     "class impl include CI",
			include:  "ZCL_TEST========CI",
			wantType: "CLAS",
			wantName: "ZCL_TEST",
		},
		{
			name:     "class method include CM001",
			include:  "ZCL_TEST========CM001",
			wantType: "CLAS",
			wantName: "ZCL_TEST",
		},
		{
			name:     "class testclass include CCAU",
			include:  "ZCL_TEST========CCAU",
			wantType: "CLAS",
			wantName: "ZCL_TEST",
		},
		{
			name:     "interface include IP",
			include:  "ZIF_TEST========IP",
			wantType: "INTF",
			wantName: "ZIF_TEST",
		},
		{
			name:     "interface intf include IF",
			include:  "ZIF_TEST========IF",
			wantType: "INTF",
			wantName: "ZIF_TEST",
		},
		{
			name:     "function group main SAPL",
			include:  "SAPLZTEST_FG",
			wantType: "FUGR",
			wantName: "ZTEST_FG",
		},
		{
			name:     "function group TOP",
			include:  "LZTEST_FGTOP",
			wantType: "FUGR",
			wantName: "ZTEST_FG",
		},
		{
			name:     "function group UXX",
			include:  "LZTEST_FGUXX",
			wantType: "FUGR",
			wantName: "ZTEST_FG",
		},
		{
			name:     "function group U01",
			include:  "LZTEST_FGU01",
			wantType: "FUGR",
			wantName: "ZTEST_FG",
		},
		{
			name:     "function group U99",
			include:  "LZTEST_FGU99",
			wantType: "FUGR",
			wantName: "ZTEST_FG",
		},
		{
			name:     "function group F01",
			include:  "LZTEST_FGF01",
			wantType: "FUGR",
			wantName: "ZTEST_FG",
		},
		{
			name:     "function group I01",
			include:  "LZTEST_FGI01",
			wantType: "FUGR",
			wantName: "ZTEST_FG",
		},
		{
			name:     "plain program name",
			include:  "ZREPORT_TEST",
			wantType: "PROG",
			wantName: "ZREPORT_TEST",
		},
		{
			name:     "lowercase normalised to upper",
			include:  "zcl_test========cp",
			wantType: "CLAS",
			wantName: "ZCL_TEST",
		},
		// Example: a fugr sub-include whose group name itself contains
		// underscores. The suffix U01 is the standard section letter
		// followed by a two-digit sequence, which the walker must
		// strip to recover the bare group name.
		{
			name:     "fugr sub-include with underscore in group name",
			include:  "LZTEST_GROUPU01",
			wantType: "FUGR",
			wantName: "ZTEST_GROUP",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := includeToParent(tc.include)
			if !ok {
				t.Fatalf("includeToParent(%q) returned ok=false", tc.include)
			}
			if got.objType != tc.wantType {
				t.Errorf("objType = %q, want %q", got.objType, tc.wantType)
			}
			if got.objName != tc.wantName {
				t.Errorf("objName = %q, want %q", got.objName, tc.wantName)
			}
		})
	}

	// Degenerate inputs that should return ok=false.
	t.Run("empty string", func(t *testing.T) {
		if _, ok := includeToParent(""); ok {
			t.Error("empty include should return ok=false")
		}
	})
}

// TestFilterHopsByScope confirms the cache reuse guard: when a cached
// hop references a FromScope that is no longer in the current run's
// scope, it should be dropped.
func TestFilterHopsByScope(t *testing.T) {
	hops := []graph.TransitiveReachHop{
		{FromScope: "CLAS:ZCL_TEST_A", Via: "FUGR:ZTEST_HELPER"},
		{FromScope: "CLAS:ZCL_TEST_B", Via: "FUGR:ZTEST_HELPER"},
		{FromScope: "FUGR:ZTEST_MIN", Via: "FUGR:ZTEST_AUTH"},
	}
	scope := map[string]bool{
		"CLAS:ZCL_TEST_A": true,
		"FUGR:ZTEST_MIN":  true,
		// CLAS:ZCL_TEST_B deliberately omitted
	}
	got := filterHopsByScope(hops, scope)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	for _, h := range got {
		if h.FromScope == "CLAS:ZCL_B" {
			t.Error("CLAS:ZCL_B should have been filtered out")
		}
	}
}
