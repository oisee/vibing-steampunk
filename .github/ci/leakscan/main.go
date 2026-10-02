// Command leakscan looks for live SAP identifiers in what is about to become
// public: host names, addresses, SIDs, user names, session cookies, basic-auth
// headers, CSRF tokens and config passwords that an agent captured from a live
// system and that could reach a commit.
//
// The one idea, from open-steamgate's tools/osd-leak-scan.mjs: decode first,
// match second. Each file is read as every byte view it plausibly has (the
// text, the text as UTF-16LE, every hex run and base64 block decoded, each of
// those as ASCII and as UTF-16LE) and the patterns run over all of them. A NUL
// after each character is enough to hide a host name from grep and from an eye.
//
// The names in play cannot be written here, because a list of what must not be
// published is itself a thing that must not be published. They come from the
// VSP_LEAK_IDENTIFIERS environment variable (a masked CI secret), or from
// .local/leak-identifiers.txt (gitignored), one value per line, optionally
// `class: value` (host, ip, sid, user, ...).
//
//	cd .github/ci/leakscan
//	go run . -root ../../.. -all                  # every file in HEAD's tree
//	go run . -root ../../.. -diff origin/main     # files changed since the merge base
//	go run . -root ../../.. path/to/file dir/     # files on disk, tracked or not
//
// Exit codes. Build the binary and run it: `go run` turns every non-zero exit
// into 1.
//
//	0  full scan, clean
//	1  hits
//	2  fail closed: the identifier list was required (-require-identifiers) and
//	   is missing, nothing was read, the allow-file is malformed, a file could
//	   not be read
//	3  generic patterns only, clean: no identifier list, and none was required.
//	   This is not a pass; the caller decides (a fork PR in CI, a contributor's
//	   pre-push hook), and must say so.
//
// Output names file:line, the class that matched and the view it was found in,
// never the value: this prints to public CI logs.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	exitClean   = 0
	exitHits    = 1
	exitClosed  = 2
	exitPartial = 3

	maxFileSize = 16 << 20
	envList     = "VSP_LEAK_IDENTIFIERS"
	defaultList = ".local/leak-identifiers.txt"
	defaultAllw = ".github/ci/leakscan-allow.txt"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

type source struct {
	name string
	data []byte
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("leakscan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "repository root")
	all := fs.Bool("all", false, "scan every file in the tree of -rev")
	diff := fs.String("diff", "", "scan files changed between the merge base with this ref and -rev, and those commits' messages")
	rev := fs.String("rev", "HEAD", "the commit whose files are scanned with -all or -diff")
	idFile := fs.String("identifiers", "", "identifier list file (default: $"+envList+", else "+defaultList+")")
	require := fs.Bool("require-identifiers", false, "fail closed (exit 2) when no identifier list is available")
	allowFile := fs.String("allow", "", "allow-file (default: "+defaultAllw+" under -root)")
	if err := fs.Parse(args); err != nil {
		return exitClosed
	}
	fail := func(format string, a ...any) int {
		fmt.Fprintf(stderr, "leakscan: "+format+"\n", a...)
		return exitClosed
	}
	modes := 0
	for _, on := range []bool{*all, *diff != "", fs.NArg() > 0} {
		if on {
			modes++
		}
	}
	if modes != 1 {
		return fail("choose exactly one of -all, -diff <ref>, or paths")
	}

	ids, idSource, err := loadIdentifiers(*root, *idFile, getenv)
	if err != nil {
		return fail("%v", err)
	}
	rules, err := loadAllow(*root, *allowFile)
	if err != nil {
		return fail("allow-file: %v", err)
	}

	var sources []source
	explicit := false
	switch {
	case *all:
		explicit = true
		sources, err = treeFiles(*root, *rev)
	case *diff != "":
		sources, err = changedFiles(*root, *diff, *rev)
	default:
		explicit = true
		sources, err = diskFiles(*root, fs.Args())
	}
	if err != nil {
		return fail("%v", err)
	}

	var hits []Hit
	excused := 0
	for _, s := range sources {
		for _, h := range scanBytes(s.name, s.data, ids) {
			if allowed(rules, h) {
				excused++
				continue
			}
			hits = append(hits, h)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].File != hits[j].File {
			return hits[i].File < hits[j].File
		}
		return hits[i].Line < hits[j].Line
	})
	for _, h := range hits {
		fmt.Fprintf(stdout, "%s:%d: %s (%s)\n", h.File, h.Line, h.Class, h.View)
	}

	listNote := fmt.Sprintf("%d identifiers from %s", len(ids), idSource)
	if len(ids) == 0 {
		listNote = "NO identifier list: generic patterns only, host/user/SID names NOT checked"
	}
	fmt.Fprintf(stderr, "leakscan: %d files read, %d hits, %d excused by the allow-file; %s\n",
		len(sources), len(hits), excused, listNote)

	if len(sources) == 0 && explicit {
		return fail("no file was read; that is \"nothing looked at\", not \"clean\"")
	}
	if len(hits) > 0 {
		fmt.Fprintln(stderr, "leakscan: this is a public repository. Replace the value with a placeholder (see CLAUDE.md, Security), or move the file under .local/.")
		return exitHits
	}
	if len(ids) == 0 {
		if *require {
			return fail("the identifier list was required and is missing or empty (set $%s, or seed %s)", envList, defaultList)
		}
		return exitPartial
	}
	return exitClean
}

// loadIdentifiers: the environment (a CI secret) first, then -identifiers, then
// .local/leak-identifiers.txt in the root, then in the main worktree (a linked
// worktree has no .local of its own and should not need one).
func loadIdentifiers(root, file string, getenv func(string) string) ([]Identifier, string, error) {
	if v := getenv(envList); strings.TrimSpace(v) != "" {
		ids, err := parseIdentifiers(v)
		return ids, "$" + envList, err
	}
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, "", fmt.Errorf("identifier list: %v", err)
		}
		ids, err := parseIdentifiers(string(b))
		return ids, file, err
	}
	candidates := []string{filepath.Join(root, defaultList)}
	if out, err := git(root, "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(strings.TrimSpace(string(out))), defaultList))
	}
	for _, c := range candidates {
		b, err := os.ReadFile(c)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, "", fmt.Errorf("identifier list: %v", err)
		}
		ids, err := parseIdentifiers(string(b))
		return ids, defaultList, err
	}
	return nil, "", nil
}

func loadAllow(root, file string) ([]allowRule, error) {
	path := file
	if path == "" {
		path = filepath.Join(root, defaultAllw)
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) && file == "" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseAllow(f)
}

func git(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out, nil
}

// treeFiles is every blob in rev's tree (submodules have no content here).
func treeFiles(root, rev string) ([]source, error) {
	out, err := git(root, "ls-tree", "-r", "-z", "--full-tree", rev)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, rec := range strings.Split(string(out), "\x00") {
		meta, path, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		if f := strings.Fields(meta); len(f) >= 2 && f[1] == "blob" {
			paths = append(paths, path)
		}
	}
	return readBlobs(root, rev, paths)
}

// changedFiles is what a push or a PR adds: files added or changed between the
// merge base and rev, read as they are at rev, plus the commit messages, which
// are published too and the easiest place to paste an address into.
func changedFiles(root, base, rev string) ([]source, error) {
	mb, err := git(root, "merge-base", base, rev)
	if err != nil {
		return nil, err
	}
	mbs := strings.TrimSpace(string(mb))
	out, err := git(root, "diff", "-z", "--name-only", "--no-renames", "--diff-filter=ACMRT", mbs, rev)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	srcs, err := readBlobs(root, rev, paths)
	if err != nil {
		return nil, err
	}
	logs, err := git(root, "log", "-z", "--format=%h%n%B", mbs+".."+rev)
	if err != nil {
		return nil, err
	}
	for _, rec := range strings.Split(string(logs), "\x00") {
		h, msg, ok := strings.Cut(rec, "\n")
		if ok {
			srcs = append(srcs, source{name: "commit " + strings.TrimSpace(h) + " (message)", data: []byte(msg)})
		}
	}
	return srcs, nil
}

// readBlobs reads rev:path for every path through one `git cat-file --batch`.
func readBlobs(root, rev string, paths []string) ([]source, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	cmd := exec.Command("git", "-C", root, "cat-file", "--batch")
	var in bytes.Buffer
	for _, p := range paths {
		if strings.ContainsAny(p, "\n") {
			return nil, fmt.Errorf("cannot read a path with a newline in it: %q", p)
		}
		fmt.Fprintf(&in, "%s:%s\n", rev, p)
	}
	cmd.Stdin = &in
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	r := bufio.NewReader(stdout)
	var out []source
	for _, p := range paths {
		header, err := r.ReadString('\n')
		if err != nil {
			_ = cmd.Wait()
			return nil, fmt.Errorf("git cat-file %s: %v", p, err)
		}
		f := strings.Fields(header)
		if len(f) != 3 || f[1] != "blob" {
			_ = cmd.Wait()
			return nil, fmt.Errorf("git cat-file %s:%s: %s", rev, p, strings.TrimSpace(header))
		}
		var size int
		if _, err := fmt.Sscan(f[2], &size); err != nil || size > maxFileSize {
			_ = cmd.Wait()
			return nil, fmt.Errorf("%s: %s bytes is over the %d-byte limit; it cannot be scanned, so it cannot pass", p, f[2], maxFileSize)
		}
		data := make([]byte, size+1) // the blob, then a newline
		if _, err := io.ReadFull(r, data); err != nil {
			_ = cmd.Wait()
			return nil, fmt.Errorf("git cat-file %s: %v", p, err)
		}
		out = append(out, source{name: p, data: data[:size]})
	}
	return out, cmd.Wait()
}

// diskFiles reads named files, walking directories, whether git knows them or
// not: the thing most likely to be pasted into a public tracker is a draft
// under .local/, and asking git about it would read nothing.
func diskFiles(root string, paths []string) ([]source, error) {
	var out []source
	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(p) {
			abs = filepath.Join(root, p)
		}
		err := filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Size() > maxFileSize {
				return fmt.Errorf("%s: %d bytes is over the %d-byte limit; it cannot be scanned, so it cannot pass", path, info.Size(), maxFileSize)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			name := path
			if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
				name = filepath.ToSlash(rel)
			}
			out = append(out, source{name: name, data: data})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
