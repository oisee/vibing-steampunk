package vsp

import (
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/graph"
)

// The first version of this trimmed padding and section letters from the right,
// which ate real characters and handed the sweep a generated include as an
// object name. The handler refused it and the sweep filed the refusal as a
// defect — a false finding, which is the one failure this tool must not have.
func TestRepositoryNameFromInclude(t *testing.T) {
	cases := map[string]string{
		"CL_ABAP_TYPEDESCR=============CP": "CL_ABAP_TYPEDESCR",
		"ZCL_VSP_GIT_SERVICE===========CU": "ZCL_VSP_GIT_SERVICE",
		"/IWBEP/CL_MGW_REQUEST=========CP": "/IWBEP/CL_MGW_REQUEST",
		"ZVSP_ENQUEUE_RESET":               "ZVSP_ENQUEUE_RESET",

		// Not repository objects, and each one previously produced a name.
		"%_CCRMB":  "",
		"%_T00001": "",
		"":         "",
		"CP":       "",
	}
	for include, want := range cases {
		if got := repositoryNameFromInclude(include); got != want {
			t.Errorf("repositoryNameFromInclude(%q) = %q, want %q", include, got, want)
		}
	}
}

// A name ending in one of the section letters must survive intact. This is the
// case the right-trimming version got wrong.
func TestSectionLettersAreNotTrimmedFromTheName(t *testing.T) {
	for _, name := range []string{"ZCL_DEMO_ACT", "ZCL_DEMO_TOP", "ZCL_DEMO_CU"} {
		include := name + "=============CP"
		if got := repositoryNameFromInclude(include); got != name {
			t.Errorf("repositoryNameFromInclude(%q) = %q, want %q", include, got, name)
		}
	}
}

// A target resolved without its type made every probe assert CLAS. That is
// right for a class pool and wrong for an ordinary include — and on one system
// the first WBCROSSGT row happened to be a class so the sweep passed, while on
// another it was a program include, the probe asked for it under /oo/classes/,
// got a 404, and reported the capability **absent on that release**.
//
// A verdict about the release, produced by a mistake of ours, in the tool whose
// whole job is telling those two apart.
func TestAnIncludeIsNotAssumedToBeAClass(t *testing.T) {
	cases := map[string]string{
		"CL_ABAP_TYPEDESCR=============CP": "CLAS",
		"ZIF_VSP_SERVICE==============IU":  "INTF",
		"HFRPAYDATA":                       "PROG",
		"SAPLZVSP_GIT":                     "FUGR",
	}
	for include, wantType := range cases {
		_, gotType, gotName := graph.NormalizeInclude(include)
		if gotType != wantType {
			t.Errorf("NormalizeInclude(%q) typed it as %q, want %q", include, gotType, wantType)
		}
		if gotName == "" {
			t.Errorf("NormalizeInclude(%q) returned an empty name", include)
		}
	}
}
