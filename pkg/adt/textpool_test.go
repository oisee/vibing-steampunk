package adt

import (
	"strings"
	"testing"
)

func TestTextDocument(t *testing.T) {
	body := "@DDICReference\nP_CARRID=Airline\n\n@DDICReference\nP_CONNID=Connection\n\nP_DEEP  =?...\n"
	d := parseTextDocument(body)
	if len(d.entries) != 3 || d.entries[0].directives[0] != "@DDICReference" || d.entries[2].key != "P_DEEP" {
		t.Fatalf("parsed %+v", d.entries)
	}
	d.set("S", "P_CONNID", "Flight connection")
	d.set("S", "P_DEEP", "Follow includes")
	got := d.String("S")
	want := "@DDICReference\nP_CARRID=Airline\nP_CONNID=Flight connection\nP_DEEP  =Follow includes\n"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	sym := parseTextDocument("@MaxLength:40\n001=Options\n")
	sym.set("I", "B01", "Where to look")
	sym.set("I", "001", strings.Repeat("x", 50))
	if s := sym.String("I"); s != "@MaxLength:50\n001="+strings.Repeat("x", 50)+"\n@MaxLength:40\nB01=Where to look\n" {
		t.Errorf("symbols: %q", s)
	}
	head := parseTextDocument("listHeader=\r\n\r\ncolumnHeader_1=\r\ncolumnHeader_2=\r\n")
	head.set("H", "LISTHEADER", "Objects found")
	if s := head.String("H"); s != "listHeader=Objects found\ncolumnHeader_1=\ncolumnHeader_2=\n" {
		t.Errorf("headings: %q", s)
	}
}

func TestPlanKind(t *testing.T) {
	doc := parseTextDocument("P_DEEP  =?...\nP_DEVC  =Package to scan\nS_OBJ   =Objects\n")
	kp := planKind("S", doc, map[string]string{
		"P_DEEP": "Follow includes", "P_DEVC": "Package to scan", "S_OBJ": "Object names",
		"SELECTION": "too long a key", "P_GHOST": "not on the screen", "P_LONG": strings.Repeat("x", 31),
	}, TextPoolOptions{})
	if len(kp.Added) != 0 || len(kp.Changed) != 2 || len(kp.Unchanged) != 1 || len(kp.Unknown) != 1 || len(kp.Refused) != 2 {
		t.Errorf("plan: %+v", kp)
	}
	if kp.Changed[0].Key != "P_DEEP" || kp.Changed[0].Old != "?..." || kp.Changed[1].New != "Object names" {
		t.Errorf("changed: %+v", kp.Changed)
	}
	if kp.Unknown[0] != "P_GHOST" {
		t.Errorf("unknown: %v", kp.Unknown)
	}
	// Unknown keys are written when asked, and a symbol may always be new.
	if kp := planKind("S", doc, map[string]string{"P_GHOST": "x"}, TextPoolOptions{AllowUnknown: true}); len(kp.Added) != 1 {
		t.Errorf("allow unknown: %+v", kp)
	}
	if kp := planKind("I", parseTextDocument(""), map[string]string{"001": "New", "b01": "Block", "TOOLONG1": "x"}, TextPoolOptions{}); len(kp.Added) != 2 || len(kp.Refused) != 1 {
		t.Errorf("symbols: %+v", kp)
	}
	if kp := planKind("S", doc, map[string]string{"P_DEEP": "Follow includes"}, TextPoolOptions{}); len(kp.Untouched) != 2 {
		t.Errorf("untouched: %v", kp.Untouched)
	}
	if kp := planKind("S", doc, map[string]string{"P_DEEP": TextDelete, "P_GONE": TextDelete}, TextPoolOptions{}); len(kp.Removed) != 1 || kp.Removed[0] != "P_DEEP" || len(kp.Unchanged) != 1 {
		t.Errorf("delete: %+v", kp)
	}
	doc.remove("p_deep")
	if _, ok := doc.get("P_DEEP"); ok || len(doc.entries) != 2 {
		t.Errorf("remove: %+v", doc.entries)
	}
	head := parseTextDocument("listHeader=\ncolumnHeader_1=\n")
	if kp := planKind("H", head, map[string]string{"listheader": "Found", "columnHeader_1": "", "COLUMNHEADER_9": "x"}, TextPoolOptions{}); len(kp.Changed) != 1 || kp.Changed[0].Key != "listHeader" || len(kp.Unchanged) != 1 || len(kp.Refused) != 1 {
		t.Errorf("headings: %+v", kp)
	}
}

func TestTextPoolTarget(t *testing.T) {
	for in, want := range map[TextPoolTarget]string{
		{"", "zdemo"}:         "PROG ZDEMO",
		{"program", "zdemo"}:  "PROG ZDEMO",
		{"CLAS", "zcl_demo"}:  "CLAS ZCL_DEMO",
		{"class", "zcl_demo"}: "CLAS ZCL_DEMO",
	} {
		got, err := in.normalized()
		if err != nil || got.String() != want {
			t.Errorf("%+v: %v %v", in, got, err)
		}
	}
	if _, err := (TextPoolTarget{"FUGR", "ZX"}).normalized(); err == nil {
		t.Error("function group accepted")
	}
	if (TextPoolTarget{"CLAS", "ZCL_DEMO"}).resource() != "/sap/bc/adt/textelements/classes/zcl_demo" {
		t.Error("class resource")
	}
}

func TestSymbolsUsed(t *testing.T) {
	src := `REPORT zdemo.
* WRITE text-999.
WRITE: / text-001, text-b01. " not text-c01
MESSAGE text-002 TYPE 'I'.
WRITE 'text-003 in a literal'.
`
	got := strings.Join(symbolsUsed(src), ",")
	if got != "001,002,003,B01" {
		t.Errorf("symbols used: %s", got)
	}
}
