package dap

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// Source ↔ object mapping.
//
// A DAP client names code by file path; ADT names it by source URI. The two
// meet through vsp's file convention, which abapGit and `vsp export` already
// use and which adt.ParseABAPFile reads:
//
//	zcl_x.clas.abap               /sap/bc/adt/oo/classes/zcl_x/source/main
//	zcl_x.clas.testclasses.abap   /sap/bc/adt/oo/classes/zcl_x/includes/testclasses
//	zrep.prog.abap                /sap/bc/adt/programs/programs/zrep/source/main
//	zrep_top.incl.abap            /sap/bc/adt/programs/includes/zrep_top/source/main
//	zif_x.intf.abap               /sap/bc/adt/oo/interfaces/zif_x/source/main
//	zgrp.fugr.z_fm.abap           /sap/bc/adt/functions/groups/zgrp/fmodules/z_fm/source/main
//	zgrp.fugr.z_fm.func.abap      (the same)
//
// '#' in a file name stands for the namespace '/', as in abapGit.
//
// A path may instead be a vsp:// reference, for an object that has no local
// file: vsp:///sap/bc/adt/... names the source URI itself, and vsp://ZNAME
// names an object to look up in the repository.
//
// The way back — a stack frame's URI to a file — goes through the files the
// client has already set breakpoints in, then a scan of the launch's
// sourceRoot. A frame that maps to no file is served as a DAP source
// reference: the client asks for its text and the adapter reads it from SAP.

// vspScheme prefixes a source path that names SAP code rather than a file.
const vspScheme = "vsp://"

// normURI is the form two ADT URIs are compared in: no fragment, no query,
// unescaped, lower-case. ADT answers in lower case and accepts either. The one
// escape kept is %2f, the namespace slash inside an object name, which would
// otherwise read as a path separator.
func normURI(uri string) string {
	if i := strings.IndexAny(uri, "#?"); i >= 0 {
		uri = uri[:i]
	}
	const slash = "\x00"
	uri = strings.ReplaceAll(strings.ReplaceAll(uri, "%2F", slash), "%2f", slash)
	if u, err := url.PathUnescape(uri); err == nil {
		uri = u
	}
	uri = strings.ReplaceAll(uri, slash, "%2f")
	return strings.ToLower(strings.TrimSuffix(uri, "/"))
}

// uriLine reads the line out of an ADT URI fragment: #start=26 or #start=26,0.
func uriLine(uri string) int {
	_, frag, ok := strings.Cut(uri, "#")
	if !ok {
		return 0
	}
	for _, part := range strings.Split(frag, ";") {
		if v, ok := strings.CutPrefix(part, "start="); ok {
			v, _, _ = strings.Cut(v, ",")
			n, _ := strconv.Atoi(v)
			return n
		}
	}
	return 0
}

// fileTarget maps a local file to the ADT source URI a line breakpoint in it is
// addressed by, using vsp's own file parser.
func fileTarget(path string) (string, error) {
	info, err := adt.ParseABAPFile(path)
	if err != nil {
		return "", err
	}
	var uri string
	switch {
	case info.ObjectType == adt.ObjectTypeClass && info.ClassIncludeType != "" && info.ClassIncludeType != adt.ClassIncludeMain:
		uri = adt.GetClassIncludeSourceURL(info.ObjectName, info.ClassIncludeType)
	default:
		uri = adt.GetSourceURL(info.ObjectType, info.ObjectName, info.ParentName)
	}
	if uri == "" {
		return "", fmt.Errorf("%s is a %s, which has no ABAP lines to stop on", filepath.Base(path), info.ObjectType)
	}
	switch info.ObjectType {
	case adt.ObjectTypeDDLS, adt.ObjectTypeSRVD:
		return "", fmt.Errorf("%s is a %s, which has no ABAP lines to stop on", filepath.Base(path), info.ObjectType)
	}
	return normURI(uri), nil
}

// candidateFiles lists the file names the convention gives an ADT source URI,
// most specific first.
func candidateFiles(uri string) []string {
	n := normURI(uri)
	rest, ok := strings.CutPrefix(n, "/sap/bc/adt/")
	if !ok {
		return nil
	}
	parts := strings.Split(rest, "/")
	file := func(name string) string { return strings.ReplaceAll(name, "%2f", "#") }
	switch {
	case len(parts) >= 3 && parts[0] == "programs" && parts[1] == "programs":
		return []string{file(parts[2]) + ".prog.abap", file(parts[2]) + ".abap"}
	case len(parts) >= 3 && parts[0] == "programs" && parts[1] == "includes":
		x := file(parts[2])
		return []string{x + ".prog.abap", x + ".incl.abap", x + ".abap"}
	case len(parts) >= 5 && parts[0] == "oo" && parts[1] == "classes" && parts[3] == "includes":
		include := map[string]string{
			"definitions": "locals_def", "implementations": "locals_imp",
			"testclasses": "testclasses", "macros": "macros",
		}[parts[4]]
		if include == "" {
			return nil
		}
		return []string{file(parts[2]) + ".clas." + include + ".abap"}
	case len(parts) >= 3 && parts[0] == "oo" && parts[1] == "classes":
		return []string{file(parts[2]) + ".clas.abap"}
	case len(parts) >= 3 && parts[0] == "oo" && parts[1] == "interfaces":
		return []string{file(parts[2]) + ".intf.abap"}
	case len(parts) >= 5 && parts[0] == "functions" && parts[1] == "groups" && parts[3] == "fmodules":
		g, f := file(parts[2]), file(parts[4])
		return []string{g + ".fugr." + f + ".abap", g + ".fugr." + f + ".func.abap", f + ".func.abap"}
	case len(parts) >= 5 && parts[0] == "functions" && parts[1] == "groups" && parts[3] == "includes":
		g, i := file(parts[2]), file(parts[4])
		return []string{g + ".fugr." + i + ".abap", i + ".abap"}
	case len(parts) >= 3 && parts[0] == "functions" && parts[1] == "groups":
		return []string{file(parts[2]) + ".fugr.abap"}
	}
	return nil
}

// sourceIndex finds local files for ADT URIs.
type sourceIndex struct {
	// known maps a normalised URI to the file the client set breakpoints in.
	known map[string]string
	// byName maps a lower-cased base name to a path under the source root.
	byName map[string]string
}

func newSourceIndex() *sourceIndex {
	return &sourceIndex{known: map[string]string{}}
}

// maxIndexedFiles bounds the source-root scan, so pointing sourceRoot at a
// home directory costs a moment rather than the session.
const maxIndexedFiles = 50000

// scan indexes the ABAP files under root by base name.
func (x *sourceIndex) scan(root string) error {
	if root == "" {
		return nil
	}
	if _, err := os.Stat(root); err != nil {
		return err
	}
	x.byName = map[string]string{}
	seen := 0
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable corners are skipped, not fatal
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", ".cache":
				return filepath.SkipDir
			}
			return nil
		}
		seen++
		if seen > maxIndexedFiles {
			return filepath.SkipAll
		}
		name := strings.ToLower(d.Name())
		if strings.HasSuffix(name, ".abap") {
			if _, dup := x.byName[name]; !dup {
				x.byName[name] = p
			}
		}
		return nil
	})
}

// remember records that path is the client's file for uri.
func (x *sourceIndex) remember(uri, path string) { x.known[normURI(uri)] = path }

// lookup returns the local file for an ADT URI, or "".
func (x *sourceIndex) lookup(uri string) string {
	n := normURI(uri)
	if p, ok := x.known[n]; ok {
		return p
	}
	for _, c := range candidateFiles(n) {
		if p, ok := x.byName[c]; ok {
			return p
		}
	}
	return ""
}
