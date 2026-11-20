package vsp

import "testing"

// A class is not one document. A reference recorded against CL_FOO=====CCAU is
// in that class's test include, and CL_FOO's main source does not contain it —
// so `vsp examples` on the most-called method in any ABAP codebase found 186
// callers, read 15, produced nothing, and printed "0 of 15 callers" as though
// that were the answer.
func TestAddressDistinguishesASectionFromTheObject(t *testing.T) {
	main := sourceRef{Type: "CLAS", Name: "CL_FOO"}
	tests := sourceRef{Type: "CLAS", Name: "CL_FOO", Section: "CCAU"}
	if main.Address() == tests.Address() {
		t.Fatalf("the main source and the test include share an address (%q), "+
			"so a class calling the target from both is read once and half the answer is dropped",
			main.Address())
	}
}

// Only four sections have an address of their own. The rest live in the main
// source, and giving them a path by pattern earns a 404 — which is then filed
// as an unreadable object rather than as a section that never had a path.
func TestSectionsWithoutTheirOwnAddressReadTheMainSource(t *testing.T) {
	mainAddr := sourceRef{Type: "CLAS", Name: "CL_FOO"}.Address()
	for _, section := range []string{"CP", "CU", "CO", "CI", "CM001", "CM00Q", "CT", ""} {
		got := sourceRef{Type: "CLAS", Name: "CL_FOO", Section: section}.Address()
		if got != mainAddr {
			t.Errorf("section %q was given its own address %q; it lives in the main source", section, got)
		}
	}
	for _, section := range []string{"CCAU", "CCDEF", "CCIMP", "CCMAC"} {
		got := sourceRef{Type: "CLAS", Name: "CL_FOO", Section: section}.Address()
		if got == mainAddr {
			t.Errorf("section %q shares the main source's address; its callers will be read from the wrong half", section)
		}
	}
}

// A section suffix on something that is not a class means nothing: a program is
// one document and has no includes addressed this way.
func TestSectionsOnlyApplyToClasses(t *testing.T) {
	plain := sourceRef{Type: "PROG", Name: "ZDEMO"}.Address()
	withSection := sourceRef{Type: "PROG", Name: "ZDEMO", Section: "CCAU"}.Address()
	if plain != withSection {
		t.Errorf("a program was given a class include address: %q vs %q", plain, withSection)
	}
}
