package hanadump

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareOutRefusesGitTree(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(repo, "deep", "dump")
	if err := prepareOut(out, false); err == nil || !strings.Contains(err.Error(), "git working tree") {
		t.Fatalf("dump inside a repo was not refused: %v", err)
	}
	if err := prepareOut(out, true); err != nil {
		t.Fatalf("explicit override refused: %v", err)
	}
}

func TestPrepareOutRefusesNonEmpty(t *testing.T) {
	out := t.TempDir()
	if err := prepareOut(out, false); err != nil {
		t.Fatalf("empty dir refused: %v", err)
	}
	os.WriteFile(filepath.Join(out, "manifest.json"), []byte("{}"), 0o644)
	if err := prepareOut(out, false); err == nil {
		t.Fatal("non-empty dir accepted")
	}
}

func TestIsIdent(t *testing.T) {
	for _, ok := range []string{"SAPA4H", "REPOSRC", "/NS/TAB", "OBJ_NAME"} {
		if !isIdent(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", `A"B`, "a b", "X;DROP", "lower"} {
		if isIdent(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestLikeAny(t *testing.T) {
	where, args := likeAny("PROGNAME", []string{"Z%", "LZ%"})
	if where != `("PROGNAME" LIKE ? OR "PROGNAME" LIKE ?)` || len(args) != 2 {
		t.Errorf("got %s %v", where, args)
	}
}

func TestUniquePathKeepsBothSources(t *testing.T) {
	used := map[string]bool{}
	a := uniquePath(used, "src/"+FileName("Z/A"), ".abap", "Z/A", "A")
	b := uniquePath(used, "src/"+FileName("Z#A"), ".abap", "Z#A", "A")
	if a == b {
		t.Fatalf("Z/A and Z#A share %s", a)
	}
	if a != "src/Z#A.abap" || !strings.HasPrefix(b, "src/Z#A.") || !strings.HasSuffix(b, ".abap") {
		t.Errorf("got %s and %s", a, b)
	}
}

func TestPrepareOutFollowsSymlinks(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(repo, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(filepath.Join(repo, "sub"), link); err != nil {
		t.Fatal(err)
	}
	if err := prepareOut(filepath.Join(link, "dump"), false); err == nil || !strings.Contains(err.Error(), "git working tree") {
		t.Fatalf("a symlink into a repo was not refused: %v", err)
	}
}

func TestPrepareOutIsPrivate(t *testing.T) {
	out := filepath.Join(t.TempDir(), "dump")
	if err := prepareOut(out, false); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o077 != 0 {
		t.Errorf("dump directory is %v, readable by others", st.Mode().Perm())
	}
}
