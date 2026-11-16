package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	hdb "github.com/SAP/go-hdb/driver"

	"github.com/oisee/vibing-steampunk/pkg/hanadump"
)

// Report statuses and skip reasons. Every object in scope gets exactly one
// status; "skipped" always carries a reason.
const (
	statusWritten = "written" // its sources are files under src/
	statusRows    = "rows"    // its content is repository rows under tables/
	statusSkipped = "skipped"

	reasonNotFound    = "not-found"   // asked for, not in the repository
	reasonNoSource    = "no-source"   // a source type without an active or inactive REPOSRC row
	reasonEncoding    = "encoding"    // stored, but not decodable; raw bytes under undecoded/
	reasonExcluded    = "excluded"    // its content needs an option the request left off (ddic)
	reasonUnsupported = "unsupported" // a type this dumper does not carry
	reasonTooLarge    = "too-large"   // reserved for table contents
)

var (
	sourceTypes = map[string]bool{"CLAS": true, "INTF": true, "PROG": true, "FUGR": true, "BDEF": true}
	// rowTypes: content wholly in the default tables, always dumped
	rowTypes  = map[string]bool{"MSAG": true}
	ddicTypes = map[string]bool{"TABL": true, "DTEL": true, "DOMA": true, "TTYP": true, "SHLP": true, "VIEW": true, "DDLS": true, "DDLX": true, "SRVD": true, "DCLS": true}
)

type File struct {
	Path   string `json:"path"`
	State  string `json:"state"`
	SHA256 string `json:"sha256,omitempty"`
	Bytes  int64  `json:"bytes"`
	Error  string `json:"error,omitempty"`
}

// ReportEntry is one line of objects.jsonl.
type ReportEntry struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Package string `json:"package,omitempty"`
	Author  string `json:"author,omitempty"`
	Class   string `json:"class,omitempty"`
	// State: "A", "I", or "A+I" -- the versions written.
	State  string `json:"state,omitempty"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	Files  []File `json:"files,omitempty"`
}

type sidecarBlock struct {
	Name    string         `json:"name"`
	Version string         `json:"version"`
	Commit  string         `json:"commit"`
	Request *Request       `json:"request"`
	Report  string         `json:"objects_report"`
	Counts  map[string]int `json:"counts"`
	Errors  []string       `json:"errors"`
	// Unmatched are request name and package patterns that selected no
	// TADIR object.
	Unmatched []string `json:"unmatched_patterns,omitempty"`
}

type dumpSummary struct {
	OK       bool   `json:"ok"`
	Exit     int    `json:"exit"`
	Out      string `json:"out"`
	Objects  int    `json:"objects"`
	Written  int    `json:"written"`
	Rows     int    `json:"rows"`
	Skipped  int    `json:"skipped"`
	Files    int    `json:"files"`
	Bytes    int64  `json:"bytes"`
	Errors   int    `json:"errors"`
	Seconds  int64  `json:"seconds"`
	Manifest string `json:"manifest"`
}

func runDump(ctx context.Context, db *sql.DB, c Connection, schema string, req *Request, out string, allowInRepo bool, stdout, stderr io.Writer) error {
	if err := hanadump.PrepareOut(out, allowInRepo); err != nil {
		return fail(exitRefused, "%v", err)
	}
	t0 := time.Now()
	objs, err := scope(ctx, db, schema, req)
	if err != nil {
		return err
	}

	if len(objs) == 0 && len(req.Requested) == 0 {
		return fail(exitUsage, "the request selects no objects (unmatched: %s)", strings.Join(unmatchedPatterns(req, objs), "; "))
	}
	names := selectionNames(req, objs)
	if req.Local != nil && (req.Local.All || len(req.Local.Authors) > 0) {
		localOnly := &Request{Schema: 1, Local: req.Local}
		locals, err := scope(ctx, db, schema, localOnly)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, o := range locals {
			for _, p := range hanadump.SourcePatterns(hanadump.PackageObject{Type: o.Type, Name: o.Name, Package: o.Package}) {
				if !seen[p] {
					seen[p] = true
					names = append(names, p)
				}
			}
		}
	}
	var closure *hanadump.ClosureRequest
	if len(req.Requested) > 0 {
		closure = &hanadump.ClosureRequest{Schema: 1, Iteration: req.Iteration, Requested: req.Requested, Seen: req.Seen}
	}
	m, err := hanadump.Run(ctx, db, hanadump.Options{
		Schema: schema, Names: names, Packages: req.Packages, Inactive: req.Inactive, Catalog: req.Catalog,
		DDIC: req.DDIC, Closure: closure, SkipTestIncludes: req.SkipTestIncludes, Out: out,
		Provenance:  req.Provenance,
		Connection:  hanadump.Connection{Host: c.Host, Port: c.Port, User: c.User, Database: c.Database},
		AllowInRepo: allowInRepo,
		BlobInto:    func(dst *[]byte) any { return (*hdbBlob)(dst) },
		TextInto:    func(dst *string) any { return (*hdbText)(dst) },
		Tool:        "osd-hana-dump " + version,
		Log:         func(format string, args ...any) { fmt.Fprintf(stderr, format+"\n", args...) },
	})
	if err != nil {
		return fail(exitUnsupported, "%v", err)
	}

	recs, err := readSources(filepath.Join(out, "reposrc.jsonl"))
	if err != nil {
		return err
	}
	report, errs := buildReport(objs, recs, req, m)
	for i := range report {
		for j := range report[i].Files {
			if st, err := os.Stat(filepath.Join(out, report[i].Files[j].Path)); err == nil {
				report[i].Files[j].Bytes = st.Size()
			}
		}
	}
	if n := m.Counts["catalog_errors"]; n > 0 {
		errs = append(errs, fmt.Sprintf("catalog: %d definitions not readable, see catalog/objects.jsonl", n))
	}
	for _, t := range m.MissingTables {
		errs = append(errs, "table not readable: "+t)
	}
	unmatched := unmatchedPatterns(req, objs)

	if err := writeJSONL(filepath.Join(out, "objects.jsonl"), report); err != nil {
		return err
	}
	s := dumpSummary{Out: out, Objects: len(report), Manifest: filepath.Join(out, "manifest.json")}
	counts := map[string]int{}
	for _, e := range report {
		counts[e.Status]++
		if e.Reason != "" {
			counts["skipped_"+e.Reason]++
		}
		for _, f := range e.Files {
			s.Files++
			s.Bytes += f.Bytes
		}
	}
	s.Written, s.Rows, s.Skipped, s.Errors = counts[statusWritten], counts[statusRows], counts[statusSkipped], len(errs)
	if errs == nil {
		errs = []string{}
	}
	if err := addSidecar(s.Manifest, sidecarBlock{
		Name: "osd-hana-dump", Version: version, Commit: buildCommit(), Request: req,
		Report: "objects.jsonl", Counts: counts, Errors: errs, Unmatched: unmatched,
	}); err != nil {
		return err
	}
	s.Seconds = int64(time.Since(t0).Seconds())
	s.OK = len(errs) == 0
	if !s.OK {
		s.Exit = exitPartial
	}
	writeJSONLine(stdout, s)
	if !s.OK {
		return failPrinted(exitPartial, "%d errors, see %s", len(errs), s.Manifest)
	}
	return nil
}

// selectionNames: the request's name patterns, and for every object they
// match its includes (a class's padded names, a function group's SAPL/L
// programs); with catalog, each class's AMDP methods, which HANA names
// "<CLASS>=><METHOD>".
func selectionNames(req *Request, objs []Object) []string {
	names := append([]string{}, req.Names...)
	seenPat := map[string]bool{}
	for _, n := range names {
		seenPat[n] = true
	}
	addPat := func(p string) {
		if !seenPat[p] {
			seenPat[p] = true
			names = append(names, p)
		}
	}
	for _, o := range objs {
		if !anyPatternMatches(req.Names, o.Name) {
			continue
		}
		for _, p := range hanadump.SourcePatterns(hanadump.PackageObject{Type: o.Type, Name: o.Name, Package: o.Package}) {
			addPat(p)
		}
	}
	if req.Catalog {
		for _, o := range objs {
			if o.Type == "CLAS" {
				addPat(hanadump.EscapeLike(o.Name) + "=>%")
			}
		}
	}
	return names
}

func readSources(path string) ([]hanadump.SourceRecord, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var recs []hanadump.SourceRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r hanadump.SourceRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		recs = append(recs, r)
	}
	return recs, sc.Err()
}

// buildReport gives every object in scope, and every closure name the
// repository does not have, exactly one status.
func buildReport(objs []Object, recs []hanadump.SourceRecord, req *Request, m *hanadump.Manifest) ([]ReportEntry, []string) {
	progs := &progIndex{}
	for i, r := range recs {
		progs.names = append(progs.names, r.Program)
		progs.idx = append(progs.idx, i)
	}
	sort.Sort(progs)

	var report []ReportEntry
	var errs []string
	inScope := map[string]bool{}
	ddic := req.DDIC || len(req.Requested) > 0
	for _, o := range objs {
		inScope[o.Type+" "+o.Name] = true
		e := ReportEntry{Type: o.Type, Name: o.Name, Package: o.Package, Author: o.Author, Class: o.Class}
		switch {
		case sourceTypes[o.Type]:
			states := map[string]bool{}
			bad := 0
			for _, i := range progs.match(hanadump.SourcePatterns(hanadump.PackageObject{Type: o.Type, Name: o.Name, Package: o.Package})) {
				r := recs[i]
				if r.Empty {
					continue
				}
				e.Files = append(e.Files, File{Path: r.File, State: r.State, SHA256: r.SHA256, Error: r.Error})
				if r.Error != "" {
					bad++
				} else {
					states[r.State] = true
				}
			}
			switch {
			case len(e.Files) == 0:
				e.Status, e.Reason = statusSkipped, reasonNoSource
			case bad == len(e.Files):
				e.Status, e.Reason = statusSkipped, reasonEncoding
				errs = append(errs, fmt.Sprintf("%s %s: no source decodable", o.Type, o.Name))
			default:
				e.Status = statusWritten
				if bad > 0 {
					errs = append(errs, fmt.Sprintf("%s %s: %d of %d sources not decodable", o.Type, o.Name, bad, len(e.Files)))
				}
			}
			e.State = joinStates(states)
		case rowTypes[o.Type]:
			e.Status = statusRows
		case ddicTypes[o.Type] && ddic:
			e.Status = statusRows
		case ddicTypes[o.Type]:
			e.Status, e.Reason = statusSkipped, reasonExcluded
		default:
			e.Status, e.Reason = statusSkipped, reasonUnsupported
		}
		report = append(report, e)
	}
	if m.Closure != nil {
		for kind, names := range m.Closure.Missing {
			for _, n := range names {
				if !inScope[kind+" "+n] {
					report = append(report, ReportEntry{Type: kind, Name: n, Status: statusSkipped, Reason: reasonNotFound})
					errs = append(errs, fmt.Sprintf("%s %s: not found", kind, n))
				}
			}
		}
		for kind, names := range m.Closure.Unhandled {
			for _, n := range names {
				if !inScope[kind+" "+n] {
					report = append(report, ReportEntry{Type: kind, Name: n, Status: statusSkipped, Reason: reasonUnsupported})
				}
			}
		}
	}
	// a requested name TADIR does not know at all
	for kind, names := range req.Requested {
		for _, n := range names {
			if !inScope[kind+" "+n] && !inMissing(m, kind, n) {
				if m.Closure != nil && contains(m.Closure.Resolved[kind], n) {
					continue // resolved by the dictionary though not in TADIR (e.g. a generated name)
				}
				report = append(report, ReportEntry{Type: kind, Name: n, Status: statusSkipped, Reason: reasonNotFound})
				errs = append(errs, fmt.Sprintf("%s %s: not found", kind, n))
			}
		}
	}
	return report, errs
}

func inMissing(m *hanadump.Manifest, kind, name string) bool {
	return m.Closure != nil && contains(m.Closure.Missing[kind], name)
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func joinStates(s map[string]bool) string {
	var out []string
	for _, k := range []string{"A", "I"} {
		if s[k] {
			out = append(out, k)
		}
	}
	return strings.Join(out, "+")
}

// progIndex: program names of reposrc.jsonl, sorted, with their record index.
type progIndex struct {
	names []string
	idx   []int
}

func (p *progIndex) Len() int           { return len(p.names) }
func (p *progIndex) Less(a, b int) bool { return p.names[a] < p.names[b] }
func (p *progIndex) Swap(a, b int) {
	p.names[a], p.names[b] = p.names[b], p.names[a]
	p.idx[a], p.idx[b] = p.idx[b], p.idx[a]
}

func (p *progIndex) match(patterns []string) []int {
	var out []int
	seen := map[int]bool{}
	for _, pat := range patterns {
		lit, prefix := unlike(pat)
		for i := sort.SearchStrings(p.names, lit); i < len(p.names); i++ {
			if prefix && !strings.HasPrefix(p.names[i], lit) || !prefix && p.names[i] != lit {
				break
			}
			if !seen[p.idx[i]] {
				seen[p.idx[i]] = true
				out = append(out, p.idx[i])
			}
		}
	}
	sort.Ints(out)
	return out
}

// unmatchedPatterns are name and package patterns no object answered to.
func unmatchedPatterns(req *Request, objs []Object) []string {
	var out []string
	for _, p := range req.Names {
		if !anyLike(p, objs, func(o Object) string { return o.Name }) {
			out = append(out, "names: "+p)
		}
	}
	for _, p := range req.Packages {
		if !anyLike(p, objs, func(o Object) string { return o.Package }) {
			out = append(out, "packages: "+p)
		}
	}
	return out
}

func anyPatternMatches(pats []string, s string) bool {
	for _, p := range pats {
		if likeMatch(p, s) {
			return true
		}
	}
	return false
}

func anyLike(pat string, objs []Object, field func(Object) string) bool {
	for _, o := range objs {
		if likeMatch(pat, field(o)) {
			return true
		}
	}
	return false
}

// likeMatch is SQL LIKE with '\' as escape character.
func likeMatch(pat, s string) bool {
	if pat == "" {
		return s == ""
	}
	switch {
	case pat[0] == '\\' && len(pat) > 1:
		return s != "" && s[0] == pat[1] && likeMatch(pat[2:], s[1:])
	case pat[0] == '%':
		for i := 0; i <= len(s); i++ {
			if likeMatch(pat[1:], s[i:]) {
				return true
			}
		}
		return false
	case pat[0] == '_':
		return s != "" && likeMatch(pat[1:], s[1:])
	default:
		return s != "" && s[0] == pat[0] && likeMatch(pat[1:], s[1:])
	}
}

// addSidecar adds the sidecar's block to the manifest hanadump wrote.
func addSidecar(path string, b sidecarBlock) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	sb, err := json.Marshal(b)
	if err != nil {
		return err
	}
	m["sidecar"] = sb
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// hdbBlob receives a BLOB from go-hdb, which hands out LOB locators that only
// the driver can read.
type hdbBlob []byte

func (b *hdbBlob) Scan(src any) error {
	if src == nil {
		*b = nil
		return nil
	}
	return hdb.ScanLobBytes(src, (*[]byte)(b))
}

// hdbText receives a character LOB from go-hdb.
type hdbText string

func (t *hdbText) Scan(src any) error {
	if src == nil {
		*t = ""
		return nil
	}
	return hdb.ScanLobString(src, (*string)(t))
}
