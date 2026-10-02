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
//	go run . -root ../../.. -diff origin/main     # what HEAD adds since the merge base:
//	                                              # changed files, every commit's added
//	                                              # lines, every commit message
//	go run . -root ../../.. -all -range A..B      # the tree, plus a range of commits
//	go run . -root ../../.. -all -push B..A -push-base origin/main   # CI on a push
//	go run . -root ../../.. path/to/file dir/     # files on disk, tracked or not
//
// -allow-rev <rev> reads the allow-file from that revision (CI passes the base,
// so a pull request cannot excuse its own hits).
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
	"strconv"
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

// A source is one thing read: a file as it is at a revision, the lines one
// commit added to a file, or a commit message.
type source struct {
	name   string // shown: the path, or "commit abc1234 (message)"
	path   string // the repository path, for allow-file path rules; "" for a message
	commit string // the commit whose patch this is, "" for a whole file
	data   []byte
	lines  []int // for a patch: line i+1 of data is line lines[i] of the file
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("leakscan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "repository root")
	all := fs.Bool("all", false, "scan every file in the tree of -rev")
	diff := fs.String("diff", "", "scan what -rev adds since its merge base with this ref: the files as they are at -rev, the lines every commit in between added, and the commit messages")
	rng := fs.String("range", "", "scan the lines every commit in this A..B range added, and the commit messages (combine with -all on a push)")
	push := fs.String("push", "", "a push event's BEFORE..AFTER: scan the pushed commits like -range; an all-zero BEFORE (a new branch) scans every commit not on -push-base, and a BEFORE that is not a commit (a force push) fails closed")
	pushBase := fs.String("push-base", "", "with -push: the branch a new branch is compared with (e.g. origin/main)")
	rev := fs.String("rev", "HEAD", "the commit whose files are scanned with -all or -diff")
	idFile := fs.String("identifiers", "", "identifier list file (default: $"+envList+", else "+defaultList+")")
	require := fs.Bool("require-identifiers", false, "fail closed (exit 2) when no identifier list is available")
	allowFile := fs.String("allow", "", "allow-file on disk (default: "+defaultAllw+" under -root)")
	allowRev := fs.String("allow-rev", "", "read the allow-file as it is at this revision instead (CI: the base, so a pull request cannot excuse its own hits)")
	if err := fs.Parse(args); err != nil {
		return exitClosed
	}
	fail := func(format string, a ...any) int {
		fmt.Fprintf(stderr, "leakscan: "+format+"\n", a...)
		return exitClosed
	}
	paths := fs.NArg() > 0
	switch {
	case paths && (*all || *diff != "" || *rng != ""):
		return fail("paths cannot be combined with -all, -diff or -range")
	case paths && *push != "":
		return fail("paths cannot be combined with -push")
	case *diff != "" && (*all || *rng != "" || *push != ""):
		return fail("-diff cannot be combined with -all, -range or -push")
	case *rng != "" && *push != "":
		return fail("choose one of -range and -push")
	case !paths && !*all && *diff == "" && *rng == "" && *push == "":
		return fail("choose -all, -diff <ref>, -range <a..b> or -push <before..after> (with or without -all), or paths")
	case *allowFile != "" && *allowRev != "":
		return fail("choose one of -allow and -allow-rev")
	}

	if *push != "" {
		r, msg := pushRange(*root, *push, *pushBase)
		if msg != "" {
			return fail("%s", msg)
		}
		*rng = r
	}

	ids, idSource, err := loadIdentifiers(*root, *idFile, getenv)
	if err != nil {
		return fail("%v", err)
	}
	rules, err := loadAllow(*root, *allowFile, *allowRev)
	if err != nil {
		return fail("allow-file: %v", err)
	}

	var sources []source
	explicit := false
	add := func(more []source, err error) error {
		sources = append(sources, more...)
		return err
	}
	switch {
	case paths:
		explicit = true
		err = add(diskFiles(*root, fs.Args()))
	case *diff != "":
		var mb []byte
		if mb, err = git(*root, "merge-base", *diff, *rev); err == nil {
			base := strings.TrimSpace(string(mb))
			if err = add(changedFiles(*root, base, *rev)); err == nil {
				err = add(rangeSources(*root, base+".."+*rev))
			}
		}
	default:
		if *all {
			explicit = true
			err = add(treeFiles(*root, *rev))
		}
		if err == nil && *rng != "" {
			err = add(rangeSources(*root, *rng))
		}
	}
	if err != nil {
		return fail("%v", err)
	}

	var hits []Hit
	excused := 0
	seen := map[string]bool{}
	for _, s := range sources {
		for _, h := range scanBytes(s.name, s.data, ids) {
			if s.lines != nil && h.Line-1 < len(s.lines) {
				h.Line = s.lines[h.Line-1]
			}
			h.Path, h.Commit = s.path, s.commit
			if s.path != "" {
				h.File = s.path
				// The same value on the same line, met again in another view
				// of the history (the file at the tip, the commit that added
				// it, a merge's first-parent diff), is one finding.
				key := s.path + "\x00" + h.Class + "\x00" + h.value + "\x00" + strconv.Itoa(h.Line)
				if seen[key] {
					continue
				}
				seen[key] = true
			}
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
		where := ""
		if h.Commit != "" {
			where = " [added in " + h.Commit + "]"
		}
		fmt.Fprintf(stdout, "%s:%d: %s (%s)%s\n", h.File, h.Line, h.Class, h.View, where)
	}

	listNote := fmt.Sprintf("%d identifiers from %s", len(ids), idSource)
	if len(ids) == 0 {
		listNote = "NO identifier list: generic patterns only, host/user/SID names NOT checked"
	}
	fmt.Fprintf(stderr, "leakscan: %d sources read (files, patches, commit messages), %d hits, %d excused by the allow-file; %s\n",
		len(sources), len(hits), excused, listNote)

	if len(sources) == 0 && explicit {
		return fail("nothing was read; that is \"nothing looked at\", not \"clean\"")
	}
	if len(hits) > 0 {
		fmt.Fprintln(stderr, "leakscan: this is a public repository. Replace the value with a placeholder (see CLAUDE.md, Security), or move the file under .local/. A value that is in a commit, even one a later commit removed, is in the history: rewrite the branch before it is pushed or merged.")
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

func loadAllow(root, file, rev string) ([]allowRule, error) {
	if rev != "" {
		if _, err := git(root, "rev-parse", "--verify", "--quiet", rev+"^{commit}"); err != nil {
			return nil, fmt.Errorf("-allow-rev %s: not a commit", rev)
		}
		if _, err := git(root, "cat-file", "-e", rev+":"+defaultAllw); err != nil {
			return nil, nil // the base has no allow-file yet: nothing is excused
		}
		out, err := git(root, "show", rev+":"+defaultAllw)
		if err != nil {
			return nil, err
		}
		return parseAllow(bytes.NewReader(out))
	}
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

// pushRange turns a push event's BEFORE..AFTER into the range of commits the
// push published. A new branch has an all-zero BEFORE: every commit on it that
// is not on base. A force push leaves a BEFORE that is no longer in the
// repository, and the commits it replaced cannot be told from the ones it
// added; scanning only the tip would let a value added and deleted again in
// the rewritten history through, so that fails closed.
func pushRange(root, spec, base string) (string, string) {
	before, after, ok := strings.Cut(spec, "..")
	if !ok || after == "" {
		return "", fmt.Sprintf("-push %q: want BEFORE..AFTER", spec)
	}
	if _, err := git(root, "rev-parse", "--verify", "--quiet", after+"^{commit}"); err != nil {
		return "", fmt.Sprintf("-push: AFTER %s is not a commit here", after)
	}
	if strings.Trim(before, "0") == "" {
		if base == "" {
			return "", "-push: a new branch (all-zero BEFORE) needs -push-base to tell its commits from the base's"
		}
		if _, err := git(root, "rev-parse", "--verify", "--quiet", base+"^{commit}"); err != nil {
			return "", fmt.Sprintf("-push-base %s is not a commit here", base)
		}
		return base + ".." + after, ""
	}
	if _, err := git(root, "rev-parse", "--verify", "--quiet", before+"^{commit}"); err != nil {
		return "", fmt.Sprintf("-push: the previous tip %s is not in this clone, so this was a force push: the commits it replaced cannot be told from the ones it added, and none are scanned; this fails closed.\n"+
			"  A force push to main is blocked by the repository's ruleset (non_fast_forward on the default branch), so this should not happen.\n"+
			"  If it does, a maintainer scans the rewritten history by hand, where it is still available:\n"+
			"    leakscan -root . -require-identifiers -range <last commit you trust>..%s\n"+
			"  Re-running this job does not help: it gets the same BEFORE.", before, after)
	}
	return before + ".." + after, ""
}

// changedFiles is every file added or changed between base and rev, read as
// it is at rev: the whole file, so a binary is read too.
func changedFiles(root, base, rev string) ([]source, error) {
	out, err := git(root, "diff", "-z", "--name-only", "--no-renames", "--diff-filter=ACMRT", base, rev)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return readBlobs(root, rev, paths)
}

// rangeSources is what every commit in a range adds, commit by commit: a value
// that one commit added and a later one removed leaves a clean tip and a
// history that still carries it, and the history is what gets published. So
// each commit's added lines are a source of their own, each added binary is
// read whole as it is in that commit, and every commit message is read, since
// a message is published too and is the easiest place to paste an address.
// A merge is read as its first-parent diff, which includes what the merge
// itself resolved.
//
// Nothing here trusts a separator inside text that a commit controls. This
// once read one `git log -p --format=%x01%h%x02%B%x03` stream and cut it on
// those bytes, so a message or an added line holding 0x01 or 0x03 moved the
// cut and hid what followed it. Now the commits are listed by rev-list in a
// format of hex digits only; each message is read from the raw commit object
// through `git cat-file --batch`, which states its length before its bytes;
// and each commit's patch comes from a `git show` of its own. A message or an
// added line can hold any byte (0x01, NUL, a line that looks like a diff
// header) and is still read whole, as what it is.
func rangeSources(root, rng string) ([]source, error) {
	commits, err := listCommits(root, rng)
	if err != nil {
		return nil, err
	}
	msgs, err := readMessages(root, commits)
	if err != nil {
		return nil, err
	}
	var srcs []source
	for i, c := range commits {
		srcs = append(srcs, source{name: "commit " + c.short + " (message)", data: msgs[i]})
		patch, err := git(root, "-c", "core.quotePath=false", "show", "--format=", "--no-show-signature",
			"-p", "-U0", "--no-color", "--no-ext-diff", "--no-textconv", "--no-renames",
			"--diff-merges=first-parent", "--src-prefix=a/", "--dst-prefix=b/", c.full, "--")
		if err != nil {
			return nil, err
		}
		more, binaries := parsePatch(c.short, patch)
		srcs = append(srcs, more...)
		if len(binaries) > 0 {
			blobs, err := readBlobs(root, c.full, binaries)
			if err != nil {
				return nil, err
			}
			for j := range blobs {
				blobs[j].path, blobs[j].commit = blobs[j].name, c.short
			}
			srcs = append(srcs, blobs...)
		}
	}
	return srcs, nil
}

type commitID struct{ full, short string }

// listCommits is every commit in rng. The format holds two hex placeholders
// and nothing a commit can write, and every line is checked to be just that.
func listCommits(root, rng string) ([]commitID, error) {
	out, err := git(root, "rev-list", "--no-commit-header", "--format=%H %h", rng, "--")
	if err != nil {
		return nil, err
	}
	var ids []commitID
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		full, short, ok := strings.Cut(line, " ")
		if !ok || !isHex(full) || !isHex(short) || !strings.HasPrefix(full, short) {
			return nil, fmt.Errorf("git rev-list %s: unexpected line %q", rng, line)
		}
		ids = append(ids, commitID{full, short})
	}
	return ids, nil
}

func isHex(s string) bool {
	if len(s) < 4 || len(s) > 64 {
		return false
	}
	for _, c := range []byte(s) {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// readMessages reads every commit object through one `git cat-file --batch`
// and returns each one's message: the raw bytes after the header's blank line,
// with nothing re-encoded, cut or stripped (a NUL included).
func readMessages(root string, commits []commitID) ([][]byte, error) {
	if len(commits) == 0 {
		return nil, nil
	}
	cmd := exec.Command("git", "-C", root, "cat-file", "--batch")
	var in bytes.Buffer
	for _, c := range commits {
		fmt.Fprintf(&in, "%s\n", c.full)
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
	msgs := make([][]byte, 0, len(commits))
	for _, c := range commits {
		header, err := r.ReadString('\n')
		if err != nil {
			_ = cmd.Wait()
			return nil, fmt.Errorf("git cat-file %s: %v", c.full, err)
		}
		f := strings.Fields(header)
		if len(f) != 3 || f[0] != c.full || f[1] != "commit" {
			_ = cmd.Wait()
			return nil, fmt.Errorf("git cat-file %s: %s", c.full, strings.TrimSpace(header))
		}
		var size int
		if _, err := fmt.Sscan(f[2], &size); err != nil || size < 0 || size > maxFileSize {
			_ = cmd.Wait()
			return nil, fmt.Errorf("commit %s: %s bytes is over the %d-byte limit; it cannot be scanned, so it cannot pass", c.short, f[2], maxFileSize)
		}
		data := make([]byte, size+1) // the object, then a newline
		if _, err := io.ReadFull(r, data); err != nil {
			_ = cmd.Wait()
			return nil, fmt.Errorf("git cat-file %s: %v", c.full, err)
		}
		obj := data[:size]
		msg := []byte(nil)
		if i := bytes.Index(obj, []byte("\n\n")); i >= 0 {
			msg = obj[i+2:]
		}
		msgs = append(msgs, msg)
	}
	return msgs, cmd.Wait()
}

// parsePatch splits one commit's `git show -p -U0` output into one source per
// file holding the lines the commit added, and lists the binary files it added
// or changed. Every content line of a -U0 patch starts with '+', '-' or '\',
// so no content can pose as a header; and the output holds this one commit,
// so no content can pose as the start of another.
func parsePatch(commit string, patch []byte) ([]source, []string) {
	var srcs []source
	var binaries []string
	var cur *source
	var buf bytes.Buffer
	next := 0
	flush := func() {
		if cur != nil && len(cur.lines) > 0 {
			cur.data = append([]byte(nil), buf.Bytes()...)
			srcs = append(srcs, *cur)
		}
		cur = nil
		buf.Reset()
	}
	inHeader := false
	for _, line := range strings.Split(string(patch), "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git ") || strings.HasPrefix(line, "diff --cc "):
			flush()
			inHeader = true
		case inHeader && strings.HasPrefix(line, "+++ "):
			p := unquotePath(strings.TrimPrefix(line, "+++ "))
			if p != "/dev/null" {
				p = strings.TrimPrefix(p, "b/")
				cur = &source{name: p + " @ " + commit, path: p, commit: commit}
			}
		case inHeader && strings.HasPrefix(line, "Binary files ") && strings.HasSuffix(line, " differ"):
			i := strings.LastIndex(line, " and ")
			if i >= 0 {
				p := unquotePath(strings.TrimSuffix(line[i+len(" and "):], " differ"))
				if p != "/dev/null" {
					binaries = append(binaries, strings.TrimPrefix(p, "b/"))
				}
			}
		case strings.HasPrefix(line, "@@"):
			inHeader = false
			// @@ -a,b +c,d @@: added lines are numbered from c.
			if i := strings.Index(line, " +"); i >= 0 {
				f := strings.FieldsFunc(line[i+2:], func(r rune) bool { return r == ',' || r == ' ' })
				if len(f) > 0 {
					next, _ = strconv.Atoi(f[0])
				}
			}
		case !inHeader && cur != nil && strings.HasPrefix(line, "+"):
			buf.WriteString(line[1:])
			buf.WriteByte('\n')
			cur.lines = append(cur.lines, next)
			next++
		}
	}
	flush()
	return srcs, binaries
}

func unquotePath(p string) string {
	if strings.HasPrefix(p, "\"") {
		if u, err := strconv.Unquote(p); err == nil {
			return u
		}
	}
	return p
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
		out = append(out, source{name: p, path: p, data: data[:size]})
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
			out = append(out, source{name: name, path: name, data: data})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
