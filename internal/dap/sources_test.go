package dap

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The file → source URI half of the mapping, through vsp's own file parser.
func TestFileTarget(t *testing.T) {
	dir := t.TempDir()
	files := map[string]struct{ content, uri string }{
		"zrep.prog.abap":              {"REPORT zrep.\nWRITE 1.\n", "/sap/bc/adt/programs/programs/zrep/source/main"},
		"zrep_top.incl.abap":          {"DATA x TYPE i.\n", "/sap/bc/adt/programs/includes/zrep_top/source/main"},
		"zcl_x.clas.abap":             {"CLASS zcl_x DEFINITION PUBLIC.\nENDCLASS.\nCLASS zcl_x IMPLEMENTATION.\nENDCLASS.\n", "/sap/bc/adt/oo/classes/zcl_x/source/main"},
		"zcl_x.clas.testclasses.abap": {"CLASS ltc DEFINITION FOR TESTING.\nENDCLASS.\n", "/sap/bc/adt/oo/classes/zcl_x/includes/testclasses"},
		"zcl_x.clas.locals_imp.abap":  {"CLASS lcl DEFINITION.\nENDCLASS.\n", "/sap/bc/adt/oo/classes/zcl_x/includes/implementations"},
		"#dmo#cl_y.clas.abap":         {"CLASS /dmo/cl_y DEFINITION PUBLIC.\nENDCLASS.\nCLASS /dmo/cl_y IMPLEMENTATION.\nENDCLASS.\n", "/sap/bc/adt/oo/classes/%2fdmo%2fcl_y/source/main"},
		"zif_x.intf.abap":             {"INTERFACE zif_x PUBLIC.\nENDINTERFACE.\n", "/sap/bc/adt/oo/interfaces/zif_x/source/main"},
		"zgrp.fugr.z_fm.abap":         {"FUNCTION z_fm.\nENDFUNCTION.\n", "/sap/bc/adt/functions/groups/zgrp/fmodules/z_fm/source/main"},
		"zgrp.fugr.z_fm2.func.abap":   {"FUNCTION z_fm2.\nENDFUNCTION.\n", "/sap/bc/adt/functions/groups/zgrp/fmodules/z_fm2/source/main"},
	}
	for name, f := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := fileTarget(path)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got != f.uri {
			t.Errorf("%s → %s, want %s", name, got, f.uri)
		}
	}

	// A file the convention cannot read is refused with a reason, not guessed.
	bad := filepath.Join(dir, "notes.txt")
	_ = os.WriteFile(bad, []byte("hello"), 0o644)
	if _, err := fileTarget(bad); err == nil {
		t.Error("notes.txt should not map to an object")
	}
}

// The URI → file half: the names the convention gives each kind of source.
func TestCandidateFiles(t *testing.T) {
	cases := map[string][]string{
		"/sap/bc/adt/programs/programs/zrep/source/main#start=26,0":          {"zrep.prog.abap", "zrep.abap"},
		"/sap/bc/adt/programs/includes/zrep_top/source/main":                 {"zrep_top.prog.abap", "zrep_top.incl.abap", "zrep_top.abap"},
		"/sap/bc/adt/oo/classes/ZCL_X/source/main#start=12":                  {"zcl_x.clas.abap"},
		"/sap/bc/adt/oo/classes/zcl_x/includes/testclasses":                  {"zcl_x.clas.testclasses.abap"},
		"/sap/bc/adt/oo/classes/zcl_x/includes/implementations":              {"zcl_x.clas.locals_imp.abap"},
		"/sap/bc/adt/oo/classes/%2Fdmo%2Fcl_y/source/main":                   {"#dmo#cl_y.clas.abap"},
		"/sap/bc/adt/oo/interfaces/zif_x/source/main":                        {"zif_x.intf.abap"},
		"/sap/bc/adt/functions/groups/zgrp/fmodules/z_fm/source/main":        {"zgrp.fugr.z_fm.abap", "zgrp.fugr.z_fm.func.abap", "z_fm.func.abap"},
		"/sap/bc/adt/functions/groups/zgrp/includes/lzgrpu01/source/main":    {"zgrp.fugr.lzgrpu01.abap", "lzgrpu01.abap"},
		"/sap/bc/adt/vit/wb/object_type/progps/object_name/SAPMSSY0#start=6": nil,
	}
	for uri, want := range cases {
		if got := candidateFiles(uri); !reflect.DeepEqual(got, want) {
			t.Errorf("%s → %v, want %v", uri, got, want)
		}
	}
}

func TestURIHelpers(t *testing.T) {
	if got := uriLine("/sap/bc/adt/programs/programs/zrep/source/main#start=26,0"); got != 26 {
		t.Errorf("uriLine: %d", got)
	}
	if got := uriLine("/sap/bc/adt/programs/programs/zrep/source/main"); got != 0 {
		t.Errorf("uriLine without a fragment: %d", got)
	}
	// SAP's case and the client's case meet; the namespace slash stays escaped.
	if a, b := normURI("/sap/bc/adt/oo/classes/%2FDMO%2FCL_Y/source/main#start=3"), normURI("/sap/bc/adt/oo/classes/%2fdmo%2fcl_y/source/main"); a != b {
		t.Errorf("%s != %s", a, b)
	}
}

// Frames resolve to the client's own file first, then to the source root.
func TestSourceIndex(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "src", "zcl_x.clas.abap")
	_ = os.MkdirAll(filepath.Dir(nested), 0o755)
	_ = os.WriteFile(nested, []byte("CLASS zcl_x DEFINITION.\nENDCLASS.\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".git", "zrep.prog.abap"), []byte("x"), 0o644)

	x := newSourceIndex()
	if err := x.scan(root); err != nil {
		t.Fatal(err)
	}
	if got := x.lookup("/sap/bc/adt/oo/classes/ZCL_X/source/main#start=4"); got != nested {
		t.Errorf("source root lookup: %q", got)
	}
	if got := x.lookup("/sap/bc/adt/programs/programs/zrep/source/main"); got != "" {
		t.Errorf(".git should not be indexed: %q", got)
	}
	x.remember("/sap/bc/adt/programs/programs/zrep/source/main", "/work/zrep.prog.abap")
	if got := x.lookup("/sap/bc/adt/programs/programs/ZREP/source/main#start=1"); got != "/work/zrep.prog.abap" {
		t.Errorf("remembered lookup: %q", got)
	}
}

// Two files with one name under the source root: nobody can say which one SAP
// is running, so neither is opened and the frame falls back to SAP's text.
func TestSourceIndexAmbiguousNames(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"a", "b"} {
		p := filepath.Join(root, dir, "zcl_x.clas.abap")
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("CLASS zcl_x DEFINITION.\nENDCLASS.\n"), 0o644)
	}
	x := newSourceIndex()
	if err := x.scan(root); err != nil {
		t.Fatal(err)
	}
	if got := x.lookup("/sap/bc/adt/oo/classes/zcl_x/source/main"); got != "" {
		t.Errorf("an ambiguous name resolved to %q", got)
	}
	// A file the client itself named still wins.
	x.remember("/sap/bc/adt/oo/classes/zcl_x/source/main", "/work/zcl_x.clas.abap")
	if got := x.lookup("/sap/bc/adt/oo/classes/zcl_x/source/main"); got != "/work/zcl_x.clas.abap" {
		t.Errorf("remembered file: %q", got)
	}
}
