// Command metrics measures how big and how tangled vsp's Go code is, and writes
// it as one JSON file that CI keeps per run and the PR report compares.
//
//	cd .github/ci/metrics
//	go run . -root ../../.. -baseline ../complexity-baseline.json -o -
//	go run . -root ../../.. -write-baseline ../complexity-baseline.json
//
// It is observation only and never fails on what it measures; it exits non-zero
// only when it cannot measure (a file it cannot read or parse, git missing).
//
// What counts (the same rules and thresholds as the 2026-10-02 complexity map
// report, so CI and the map agree):
//   - Go files tracked by git, outside testdata/, vendor/ and dot-directories.
//   - Generated files (the standard "Code generated ... DO NOT EDIT." header)
//     and files that are never built (//go:build ignore, i.e. go:generate
//     helpers such as embedded/abap/sync_from_src.go) are left out entirely.
//   - The headline is non-test code. _test.go files are measured too, and kept
//     apart under "test". That includes the `integration`-tagged live-SAP
//     tests: they are read like any other test. (So per-package test lines
//     run higher than `go list` counts, e.g. pkg/adt 33,096 vs 30,214; the
//     test threshold counts, 7/14/13 and 4 files over 1,000 lines, match the
//     complexity map, which counted them too.)
//   - The research packages (researchPrefixes) are measured into
//     research_totals and kept out of the headline: experiments, not product.
//
// Read-token cost is bytes/4 (integer division), the usual rule of thumb for code: an estimate of
// what it costs a model, or a person, to read the file, not a tokenizer count.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fzipp/gocyclo"
	"github.com/uudashr/gocognit"
)

// Thresholds. Something over one of these is counted, never failed.
const (
	maxCyclomatic  = 30     // gocyclo
	maxCognitive   = 40     // gocognit
	maxFuncLines   = 150    // `func` to the closing brace
	maxFileLines   = 1_000  // also the size ratchet's line: no new file past it
	maxFileTokens  = 25_000 // bytes/4, about 100 KB: an agent reading it burns its context
	warnFileTokens = 15_000
	topN           = 10
)

// researchPrefixes is empty since the transpilers moved to ABAPiti
// (github.com/oisee/abapiti); the mechanism stays for the next experiment.
var researchPrefixes = []string{}

type Thresholds struct {
	Cyclomatic     int `json:"cyclomatic"`
	Cognitive      int `json:"cognitive"`
	FuncLines      int `json:"func_lines"`
	FileLines      int `json:"file_lines"`
	FileTokens     int `json:"file_tokens"`
	FileTokensWarn int `json:"file_tokens_warn"`
}

// Counts is one set of numbers, for non-test or for test code.
type Counts struct {
	Files               int `json:"files"`
	Lines               int `json:"lines"`
	Bytes               int `json:"bytes"`
	Tokens              int `json:"tokens"`
	Funcs               int `json:"funcs"`
	Cognitive           int `json:"cognitive"` // sum over all functions
	OverCyclomatic      int `json:"over_cyclomatic"`
	OverCognitive       int `json:"over_cognitive"`
	OverFuncLines       int `json:"over_func_lines"`
	FilesOverLines      int `json:"files_over_lines"`
	FilesOverTokens     int `json:"files_over_tokens"`
	FilesOverTokensWarn int `json:"files_over_tokens_warn"`
}

func (c *Counts) add(o Counts) {
	c.Files += o.Files
	c.Lines += o.Lines
	c.Bytes += o.Bytes
	c.Tokens += o.Tokens
	c.Funcs += o.Funcs
	c.Cognitive += o.Cognitive
	c.OverCyclomatic += o.OverCyclomatic
	c.OverCognitive += o.OverCognitive
	c.OverFuncLines += o.OverFuncLines
	c.FilesOverLines += o.FilesOverLines
	c.FilesOverTokens += o.FilesOverTokens
	c.FilesOverTokensWarn += o.FilesOverTokensWarn
}

// Totals is the non-test numbers inline, test code under "test".
type Totals struct {
	Packages int `json:"packages"`
	Counts
	Test Counts `json:"test"`
}

type Package struct {
	Pkg      string `json:"pkg"`
	Research bool   `json:"research,omitempty"`
	Counts
	Test Counts `json:"test"`
}

type File struct {
	Path           string `json:"path"`
	Pkg            string `json:"pkg"`
	Research       bool   `json:"research,omitempty"`
	Test           bool   `json:"test,omitempty"`
	Lines          int    `json:"lines"`
	Tokens         int    `json:"tokens"`
	Funcs          int    `json:"funcs"`
	Cognitive      int    `json:"cognitive"` // sum over its functions
	MaxCognitive   int    `json:"max_cognitive"`
	OverCyclomatic int    `json:"over_cyclomatic,omitempty"`
	OverCognitive  int    `json:"over_cognitive,omitempty"`
	OverFuncLines  int    `json:"over_func_lines,omitempty"`
}

type Func struct {
	// Key identifies a function across commits: package directory and
	// receiver-qualified name, so moving it to another file of the same
	// package is not a new function. Only a name clash within one package
	// (build-tagged twins) adds the file.
	Key        string `json:"key"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Cyclomatic int    `json:"cyclomatic"`
	Cognitive  int    `json:"cognitive"`
	Lines      int    `json:"lines"`
}

// RatchetItem is a file over the line threshold, or a package in the
// baseline, with what the committed baseline allows it (0: not in it).
type RatchetItem struct {
	Path     string `json:"path"`
	Lines    int    `json:"lines"`
	Baseline int    `json:"baseline_lines"`
}

type Ratchet struct {
	Baseline string        `json:"baseline"` // its date and commit
	Files    []RatchetItem `json:"files"`    // every non-test file over maxFileLines
	Packages []RatchetItem `json:"packages"` // every package in the baseline
}

type Metrics struct {
	Schema       int        `json:"schema"`
	Commit       string     `json:"commit"`
	GeneratedAt  string     `json:"generated_at"`
	Thresholds   Thresholds `json:"thresholds"`
	Research     []string   `json:"research_prefixes"`
	Totals       Totals     `json:"totals"`          // headline: product code
	ResearchTot  Totals     `json:"research_totals"` // the research packages, apart
	Packages     []Package  `json:"packages"`
	Files        []File     `json:"files"`          // non-test files; test files are in the counts only
	Over         []Func     `json:"over_threshold"` // non-test, non-research functions over any threshold
	TopCognitive []Func     `json:"top_cognitive"`  // non-test, non-research
	Ratchet      *Ratchet   `json:"ratchet,omitempty"`
}

// Baseline is the committed snapshot the size ratchet compares with.
type Baseline struct {
	Comment  string         `json:"_comment"`
	Commit   string         `json:"commit"`
	Date     string         `json:"date"`
	Files    map[string]int `json:"files"`    // lines, non-test files over maxFileLines
	Packages map[string]int `json:"packages"` // non-test lines
}

func main() {
	root := flag.String("root", ".", "repository root")
	out := flag.String("o", "metrics.json", "output file, - for stdout")
	baseline := flag.String("baseline", "", "size-ratchet baseline to compare with (optional)")
	writeBaseline := flag.String("write-baseline", "", "write a new size-ratchet baseline here instead")
	flag.Parse()

	if err := run(*root, *out, *baseline, *writeBaseline); err != nil {
		fmt.Fprintln(os.Stderr, "metrics:", err)
		os.Exit(1)
	}
}

func run(root, out, baseline, writeBaseline string) error {
	m, err := measure(root)
	if err != nil {
		return err
	}
	if writeBaseline != "" {
		return writeJSON(writeBaseline, newBaseline(m))
	}
	grew := 0
	if baseline != "" {
		if m.Ratchet, err = ratchet(m, baseline); err != nil {
			return err
		}
		for _, x := range append(m.Ratchet.Files, m.Ratchet.Packages...) {
			if x.Lines > x.Baseline {
				grew++
			}
		}
	}
	if err := writeJSON(out, m); err != nil {
		return err
	}
	t := m.Totals
	summary := fmt.Sprintf("~%dk read tokens in %d files · over cyclomatic %d: %d, cognitive %d: %d, %d lines: %d · files over %d lines: %d, over %dk tokens: %d (%dk: %d)",
		t.Tokens/1000, t.Files, maxCyclomatic, t.OverCyclomatic, maxCognitive, t.OverCognitive, maxFuncLines, t.OverFuncLines,
		maxFileLines, t.FilesOverLines, maxFileTokens/1000, t.FilesOverTokens, warnFileTokens/1000, t.FilesOverTokensWarn)
	if m.Ratchet != nil {
		summary += fmt.Sprintf(" · past the size baseline: %d", grew)
	}
	fmt.Fprintln(os.Stderr, summary)
	return nil
}

func writeJSON(name string, v any) error {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if name == "-" {
		_, err = os.Stdout.Write(b)
		return err
	}
	return os.WriteFile(name, b, 0o644)
}

func git(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, stderr.String())
	}
	return b, nil
}

func isResearch(dir string) bool {
	for _, p := range researchPrefixes {
		if dir == p || strings.HasPrefix(dir, p+"/") {
			return true
		}
	}
	return false
}

func skipped(p string) bool {
	for _, part := range strings.Split(path.Dir(p), "/") {
		if part == "testdata" || part == "vendor" || (strings.HasPrefix(part, ".") && part != ".") {
			return true
		}
	}
	return false
}

// buildExpr is the file's //go:build constraint, or nil.
func buildExpr(f *ast.File) constraint.Expr {
	for _, g := range f.Comments {
		if g.Pos() >= f.Package {
			break
		}
		for _, c := range g.List {
			if constraint.IsGoBuild(c.Text) {
				if expr, err := constraint.Parse(c.Text); err == nil {
					return expr
				}
			}
		}
	}
	return nil
}

// neverBuilt reports a `//go:build ignore` file: a go:generate helper or a
// scratch program, not part of any package.
func neverBuilt(f *ast.File) bool {
	tag, ok := buildExpr(f).(*constraint.TagExpr)
	return ok && tag.Tag == "ignore"
}

func measure(root string) (*Metrics, error) {
	ls, err := git(root, "ls-files", "-z", "--", "*.go")
	if err != nil {
		return nil, err
	}
	commit := ""
	if b, err := git(root, "rev-parse", "HEAD"); err == nil {
		commit = strings.TrimSpace(string(b))
	}
	return measureFiles(root, strings.Split(strings.TrimRight(string(ls), "\x00"), "\x00"), commit)
}

// measureFiles measures the given repository-relative Go files under root.
func measureFiles(root string, rels []string, commit string) (*Metrics, error) {

	m := &Metrics{
		Schema:      1,
		Commit:      commit,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Thresholds:  Thresholds{maxCyclomatic, maxCognitive, maxFuncLines, maxFileLines, maxFileTokens, warnFileTokens},
		Research:    researchPrefixes,
		Files:       []File{},
		Over:        []Func{},
	}
	pkgs := map[string]*Package{}
	var headline []Func
	seen := map[string]int{} // key -> index in headline, for clash detection

	for _, rel := range rels {
		if rel == "" || skipped(rel) {
			continue
		}
		src, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue // deleted in the work tree, not yet committed
			}
			return nil, err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, rel, src, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		test := strings.HasSuffix(rel, "_test.go")
		if ast.IsGenerated(f) || neverBuilt(f) {
			continue
		}
		dir := path.Dir(rel)
		fr := File{
			Path: rel, Pkg: dir, Research: isResearch(dir), Test: test,
			Lines: bytes.Count(src, []byte("\n")), Tokens: len(src) / 4,
		}
		if len(src) > 0 && src[len(src)-1] != '\n' {
			fr.Lines++
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			fn := Func{
				Key:        dir + " " + funcName(fd),
				File:       rel,
				Line:       fset.Position(fd.Pos()).Line,
				Cyclomatic: gocyclo.Complexity(fd),
				Cognitive:  gocognit.Complexity(fd),
				Lines:      fset.Position(fd.End()).Line - fset.Position(fd.Pos()).Line + 1,
			}
			fr.Funcs++
			fr.Cognitive += fn.Cognitive
			fr.MaxCognitive = max(fr.MaxCognitive, fn.Cognitive)
			if fn.Cyclomatic > maxCyclomatic {
				fr.OverCyclomatic++
			}
			if fn.Cognitive > maxCognitive {
				fr.OverCognitive++
			}
			if fn.Lines > maxFuncLines {
				fr.OverFuncLines++
			}
			if fr.Test || fr.Research {
				continue
			}
			if i, dup := seen[fn.Key]; dup {
				fn.Key += " @" + path.Base(rel)
				if !strings.Contains(headline[i].Key, " @") {
					headline[i].Key += " @" + path.Base(headline[i].File)
				}
			} else {
				seen[fn.Key] = len(headline)
			}
			headline = append(headline, fn)
		}

		p := pkgs[dir]
		if p == nil {
			p = &Package{Pkg: dir, Research: fr.Research}
			pkgs[dir] = p
		}
		c := Counts{
			Files: 1, Lines: fr.Lines, Bytes: len(src), Tokens: fr.Tokens, Funcs: fr.Funcs, Cognitive: fr.Cognitive,
			OverCyclomatic: fr.OverCyclomatic, OverCognitive: fr.OverCognitive, OverFuncLines: fr.OverFuncLines,
		}
		if fr.Lines > maxFileLines {
			c.FilesOverLines = 1
		}
		if fr.Tokens > maxFileTokens {
			c.FilesOverTokens = 1
		}
		if fr.Tokens > warnFileTokens {
			c.FilesOverTokensWarn = 1
		}
		if fr.Test {
			p.Test.add(c)
		} else {
			p.add(c)
			m.Files = append(m.Files, fr)
		}
	}

	for _, p := range pkgs {
		m.Packages = append(m.Packages, *p)
		t := &m.Totals
		if p.Research {
			t = &m.ResearchTot
		}
		t.Packages++
		t.add(p.Counts)
		t.Test.add(p.Test)
	}
	sort.Slice(m.Packages, func(i, j int) bool { return m.Packages[i].Pkg < m.Packages[j].Pkg })
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })

	for _, fn := range headline {
		if fn.Cyclomatic > maxCyclomatic || fn.Cognitive > maxCognitive || fn.Lines > maxFuncLines {
			m.Over = append(m.Over, fn)
		}
	}
	sort.Slice(m.Over, func(i, j int) bool { return m.Over[i].Key < m.Over[j].Key })
	sort.SliceStable(headline, func(i, j int) bool {
		if headline[i].Cognitive != headline[j].Cognitive {
			return headline[i].Cognitive > headline[j].Cognitive
		}
		return headline[i].Key < headline[j].Key
	})
	m.TopCognitive = headline[:min(topN, len(headline))]
	return m, nil
}

// funcName is Name, or Recv.Name / (*Recv).Name for a method.
func funcName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	t := fd.Recv.List[0].Type
	ptr := false
	if s, ok := t.(*ast.StarExpr); ok {
		ptr, t = true, s.X
	}
	switch x := t.(type) { // drop type parameters: T[K] -> T
	case *ast.IndexExpr:
		t = x.X
	case *ast.IndexListExpr:
		t = x.X
	}
	name := "?"
	if id, ok := t.(*ast.Ident); ok {
		name = id.Name
	}
	if ptr {
		return "(*" + name + ")." + fd.Name.Name
	}
	return name + "." + fd.Name.Name
}

func newBaseline(m *Metrics) Baseline {
	b := Baseline{
		Comment: fmt.Sprintf("Size ratchet for .github/ci/metrics (advisory). files: lines of every non-test file over %d lines; "+
			"packages: non-test lines. The PR report warns when a file not listed here passes %d lines, or a listed file or "+
			"package grows past its number here or past its size on main, whichever is smaller (so a shrink tightens the "+
			"ratchet without an edit here). A PR with a reason to grow something raises its number here, so the reason "+
			"is in the diff.", maxFileLines, maxFileLines),
		Commit:   m.Commit,
		Date:     time.Now().UTC().Format("2006-01-02"),
		Files:    map[string]int{},
		Packages: map[string]int{},
	}
	for _, p := range m.Packages {
		if !p.Research && p.Lines > 0 {
			b.Packages[p.Pkg] = p.Lines
		}
	}
	for _, f := range m.Files {
		if !f.Research && f.Lines > maxFileLines {
			b.Files[f.Path] = f.Lines
		}
	}
	return b
}

func ratchet(m *Metrics, name string) (*Ratchet, error) {
	raw, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var b Baseline
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	r := &Ratchet{Baseline: b.Date + " (" + shortSHA(b.Commit) + ")", Files: []RatchetItem{}, Packages: []RatchetItem{}}
	for _, p := range m.Packages {
		if base, ok := b.Packages[p.Pkg]; ok && !p.Research {
			r.Packages = append(r.Packages, RatchetItem{p.Pkg, p.Lines, base})
		}
	}
	for _, f := range m.Files {
		if !f.Research && f.Lines > maxFileLines {
			r.Files = append(r.Files, RatchetItem{f.Path, f.Lines, b.Files[f.Path]})
		}
	}
	return r, nil
}

func shortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
