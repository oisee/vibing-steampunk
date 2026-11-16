package hanadump

import (
	"reflect"
	"testing"
)

func TestEscapeLike(t *testing.T) {
	if got := EscapeLike(`Z_A%B\C`); got != `Z\_A\%B\\C` {
		t.Errorf("got %s", got)
	}
}

func TestSourcePatterns(t *testing.T) {
	cases := []struct {
		o    PackageObject
		want []string
	}{
		// a class pool's includes: the name padded to 30 with '=', any suffix
		{PackageObject{Type: "CLAS", Name: "ZCL_X"}, []string{`ZCL\_X`, `ZCL\_X=========================%`}},
		{PackageObject{Type: "INTF", Name: "/NS/IF_Y"}, []string{`/NS/IF\_Y`, `/NS/IF\_Y======================%`}},
		// a behaviour definition's source is "<name>====...BD" (and BDI)
		{PackageObject{Type: "BDEF", Name: "/NS/R_T"}, []string{`/NS/R\_T`, `/NS/R\_T=======================%`}},
		{PackageObject{Type: "FUGR", Name: "ZFG"}, []string{"ZFG", "SAPLZFG", "LZFG%"}},
		// a namespace keeps its slashes in front of SAPL and L
		{PackageObject{Type: "FUGR", Name: "/NS/F_G"}, []string{`/NS/F\_G`, `/NS/SAPLF\_G`, `/NS/LF\_G%`}},
		{PackageObject{Type: "PROG", Name: "Z_REPORT"}, []string{`Z\_REPORT`}},
		{PackageObject{Type: "TABL", Name: "ZTAB"}, []string{"ZTAB"}},
	}
	for _, c := range cases {
		if got := SourcePatterns(c.o); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%+v: got %q, want %q", c.o, got, c.want)
		}
	}
	// a 30-character class name has no padding left
	long := "ZCL_ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if got := SourcePatterns(PackageObject{Type: "CLAS", Name: long}); got[1] != EscapeLike(long)+"%" {
		t.Errorf("got %q", got)
	}
}

func TestParseRawTable(t *testing.T) {
	got, err := ParseRawTable("/IWBEP/I_SBD_%")
	if err != nil || !reflect.DeepEqual(got, RawTable{Pattern: "/IWBEP/I_SBD_%"}) {
		t.Errorf("whole table: %+v %v", got, err)
	}
	got, err = ParseRawTable("/IWFND/I_MED_SRH:SRV_IDENTIFIER=Z%,/DMO/%")
	if err != nil || !reflect.DeepEqual(got, RawTable{Pattern: "/IWFND/I_MED_SRH", Column: "SRV_IDENTIFIER", Values: []string{"Z%", "/DMO/%"}}) {
		t.Errorf("filtered: %+v %v", got, err)
	}
	// a padded class name keeps its '=' after the first one
	got, err = ParseRawTable("TMDIR:CLASSNAME=ZCL_X=====%")
	if err != nil || got.Values[0] != "ZCL_X=====%" {
		t.Errorf("value with '=': %+v %v", got, err)
	}
	for _, bad := range []string{"", "a lower", `T";DROP`, "T:", "T:COL", "T:COL=", "T:col=X", "T:COL=A,,B"} {
		if _, err := ParseRawTable(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
