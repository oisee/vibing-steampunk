package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUpdateParseVersion(t *testing.T) {
	cases := []struct {
		in   string
		want [3]int
		ok   bool
	}{
		{"v2.56.0", [3]int{2, 56, 0}, true},
		{"2.56.0", [3]int{2, 56, 0}, true},
		{"v2.57.1-rc1", [3]int{2, 57, 1}, true},
		{"2.57.1+build.5", [3]int{2, 57, 1}, true},
		{"v3", [3]int{3, 0, 0}, true},
		{"dev", [3]int{}, false},
		{"", [3]int{}, false},
		{"v", [3]int{}, false},
		{"1.2.3.4", [3]int{}, false},
		{"1.x.3", [3]int{}, false},
		{"v2.56.0 (commit: abc)", [3]int{}, false},
	}
	for _, c := range cases {
		got, ok := parseVersion(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("parseVersion(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestUpdateNewerThan(t *testing.T) {
	cases := []struct {
		a, b [3]int
		want bool
	}{
		{[3]int{2, 57, 0}, [3]int{2, 56, 0}, true},
		{[3]int{2, 56, 0}, [3]int{2, 57, 0}, false},
		{[3]int{2, 56, 0}, [3]int{2, 56, 0}, false},
		{[3]int{3, 0, 0}, [3]int{2, 99, 99}, true},
		{[3]int{2, 56, 10}, [3]int{2, 56, 9}, true},
		{[3]int{2, 10, 0}, [3]int{2, 9, 0}, true},
	}
	for _, c := range cases {
		if got := newerThan(c.a, c.b); got != c.want {
			t.Errorf("newerThan(%v, %v) = %v; want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestUpdateAssetName(t *testing.T) {
	cases := map[[2]string]string{
		{"linux", "amd64"}:   "vsp-linux-amd64",
		{"darwin", "arm64"}:  "vsp-darwin-arm64",
		{"windows", "amd64"}: "vsp-windows-amd64.exe",
		{"windows", "386"}:   "vsp-windows-386.exe",
		{"linux", "arm"}:     "vsp-linux-arm",
	}
	for in, want := range cases {
		if got := assetName(in[0], in[1]); got != want {
			t.Errorf("assetName(%q, %q) = %q; want %q", in[0], in[1], got, want)
		}
	}
}

func TestUpdateChecksumFor(t *testing.T) {
	text := "AAAA1111  vsp-linux-amd64\n" +
		"bbbb2222  vsp-windows-amd64.exe\r\n" +
		"cccc3333 *vsp-darwin-arm64\n" +
		"\n" +
		"garbage line with three fields\n"
	if got, ok := checksumFor(text, "vsp-linux-amd64"); !ok || got != "aaaa1111" {
		t.Errorf("linux: got %q, %v", got, ok)
	}
	if got, ok := checksumFor(text, "vsp-windows-amd64.exe"); !ok || got != "bbbb2222" {
		t.Errorf("windows (CRLF): got %q, %v", got, ok)
	}
	if got, ok := checksumFor(text, "vsp-darwin-arm64"); !ok || got != "cccc3333" {
		t.Errorf("binary-mode star: got %q, %v", got, ok)
	}
	if _, ok := checksumFor(text, "vsp-linux-arm64"); ok {
		t.Error("missing asset reported as present")
	}
	if _, ok := checksumFor(text, "vsp-linux"); ok {
		t.Error("prefix of an asset name must not match")
	}
}

func TestUpdateReplaceExecutable(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "vsp")
	tmp := filepath.Join(dir, ".vsp-update-1")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceExecutable(target, tmp); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "new" {
		t.Errorf("target holds %q, want new", b)
	}
	if b, _ := os.ReadFile(target + ".old"); string(b) != "old" {
		t.Errorf(".old holds %q, want old", b)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("temp file still present after rename")
	}

	// A second update must not trip over the .old left behind.
	if err := os.WriteFile(tmp, []byte("newer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceExecutable(target, tmp); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "newer" {
		t.Errorf("target holds %q, want newer", b)
	}
}

func TestUpdateReplaceExecutableRollback(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "vsp")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The temp file does not exist, so the second rename fails and the
	// original must come back under its own name.
	err := replaceExecutable(target, filepath.Join(dir, "missing"))
	if err == nil {
		t.Fatal("expected an error for a missing temp file")
	}
	if !strings.Contains(err.Error(), "restored") {
		t.Errorf("error should say the binary was restored: %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "old" {
		t.Errorf("target holds %q after rollback, want old", b)
	}
	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Error(".old should be gone after rollback")
	}
}

// fakeRelease serves a GitHub-shaped release for the running platform. The
// checksums text is chosen per call so a test can serve a wrong one. repo is
// the owner/name the release is served under, matching what the client is
// expected to resolve and request.
func fakeRelease(t *testing.T, repo, tag string, binary []byte, checksums func(asset, sum string) string) *httptest.Server {
	t.Helper()
	asset := assetName(runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(binary)
	sumHex := hex.EncodeToString(sum[:])
	var srv *httptest.Server
	mux := http.NewServeMux()
	serveRelease := func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "vsp/") {
			t.Errorf("User-Agent = %q, want vsp/...", r.Header.Get("User-Agent"))
		}
		rel := release{TagName: tag, Assets: []releaseAsset{
			{Name: asset, URL: srv.URL + "/dl/" + asset, Size: int64(len(binary))},
			{Name: "checksums.txt", URL: srv.URL + "/dl/checksums.txt"},
			{Name: "vsp-plan9-mips", URL: srv.URL + "/dl/vsp-plan9-mips"},
		}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rel)
	}
	repoPrefix := "/repos/" + repo
	mux.HandleFunc(repoPrefix+"/releases/latest", serveRelease)
	mux.HandleFunc(repoPrefix+"/releases/tags/"+tag, serveRelease)
	mux.HandleFunc(repoPrefix+"/releases/tags/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/dl/"+asset, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("token must not be sent to the asset download")
		}
		w.Write(binary)
	})
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(checksums(asset, sumHex)))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func goodChecksums(asset, sum string) string {
	return "0000000000000000000000000000000000000000000000000000000000000000  vsp-plan9-mips\n" +
		sum + "  " + asset + "\n"
}

func setUpdateBase(t *testing.T, url string) {
	t.Helper()
	prev := updateAPIRoot
	updateAPIRoot = url
	t.Cleanup(func() { updateAPIRoot = prev })
}

func writeTarget(t *testing.T) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "vsp")
	if err := os.WriteFile(target, []byte("the old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return target
}

func TestUpdateEndToEndInstall(t *testing.T) {
	binary := bytes.Repeat([]byte("new binary bytes "), 100)
	srv := fakeRelease(t, defaultReleaseRepo, "v2.57.0", binary, goodChecksums)
	setUpdateBase(t, srv.URL)
	t.Setenv("GITHUB_TOKEN", "test-token")
	target := writeTarget(t)

	var out bytes.Buffer
	rep, err := runUpdate(context.Background(), updateOptions{Current: "v2.56.0", Target: target}, &out)
	if err != nil {
		t.Fatalf("runUpdate: %v\n%s", err, out.String())
	}
	if !rep.Installed || !rep.Available || rep.Latest != "2.57.0" || rep.Current != "2.56.0" {
		t.Errorf("report = %+v", rep)
	}
	if rep.Size != int64(len(binary)) {
		t.Errorf("size = %d, want %d", rep.Size, len(binary))
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, binary) {
		t.Error("target does not hold the downloaded binary")
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(target); fi.Mode().Perm()&0o111 == 0 {
			t.Errorf("target mode %v is not executable", fi.Mode())
		}
		if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
			t.Error(".old should have been removed")
		}
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".vsp-update-*"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
	want := "vsp 2.56.0 → 2.57.0 (" + rep.Asset + ", 1.7 KB) installed to " + target + " from " + defaultReleaseRepo + "\n"
	if runtime.GOOS != "windows" && out.String() != want {
		t.Errorf("output = %q\nwant     %q", out.String(), want)
	}
}

func TestUpdateEndToEndSpecificTagJSON(t *testing.T) {
	binary := []byte("pinned release")
	srv := fakeRelease(t, defaultReleaseRepo, "v2.50.0", binary, goodChecksums)
	setUpdateBase(t, srv.URL)
	target := writeTarget(t)

	// Not newer than the running version, so --force is needed; the tag is
	// given without its v and must still resolve.
	var out bytes.Buffer
	rep, err := runUpdate(context.Background(), updateOptions{Current: "2.56.0", Tag: "2.50.0", Target: target, Force: true, JSON: true}, &out)
	if err != nil {
		t.Fatalf("runUpdate: %v\n%s", err, out.String())
	}
	if !rep.Installed || rep.Available {
		t.Errorf("report = %+v", rep)
	}
	var decoded updateReport
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if decoded.Latest != "2.50.0" || !decoded.Installed || decoded.Path != target {
		t.Errorf("decoded = %+v", decoded)
	}
	if decoded.Repo != defaultReleaseRepo {
		t.Errorf("decoded.Repo = %q, want %q", decoded.Repo, defaultReleaseRepo)
	}
	if got, _ := os.ReadFile(target); !bytes.Equal(got, binary) {
		t.Error("target does not hold the pinned binary")
	}

	if _, err := runUpdate(context.Background(), updateOptions{Current: "2.56.0", Tag: "v9.9.9", Target: target, Force: true}, &out); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unknown tag: err = %v", err)
	}
}

func TestUpdateEndToEndCheckOnly(t *testing.T) {
	srv := fakeRelease(t, defaultReleaseRepo, "v2.57.0", []byte("x"), goodChecksums)
	setUpdateBase(t, srv.URL)
	target := writeTarget(t)

	var out bytes.Buffer
	rep, err := runUpdate(context.Background(), updateOptions{Current: "v2.56.0", Target: target, Check: true}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Installed || !rep.Available {
		t.Errorf("report = %+v", rep)
	}
	if !strings.Contains(out.String(), "update available") {
		t.Errorf("output = %q", out.String())
	}
	if got, _ := os.ReadFile(target); string(got) != "the old binary" {
		t.Error("--check must not touch the binary")
	}

	out.Reset()
	if _, err = runUpdate(context.Background(), updateOptions{Current: "v2.57.0", Target: target, Check: true}, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "vsp 2.57.0 is the latest in "+defaultReleaseRepo+"\n" {
		t.Errorf("output = %q", out.String())
	}

	out.Reset()
	rep, err = runUpdate(context.Background(), updateOptions{Current: "dev", Target: target, Check: true, JSON: true}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if rep.CurrentKnown || !rep.Available {
		t.Errorf("dev report = %+v", rep)
	}
	if !strings.Contains(out.String(), `"current_known": false`) {
		t.Errorf("json output = %q", out.String())
	}
}

func TestUpdateEndToEndRefusals(t *testing.T) {
	srv := fakeRelease(t, defaultReleaseRepo, "v2.57.0", []byte("x"), goodChecksums)
	setUpdateBase(t, srv.URL)
	target := writeTarget(t)
	var out bytes.Buffer

	// A dev build does not install without --force.
	_, err := runUpdate(context.Background(), updateOptions{Current: "dev", Target: target}, &out)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("dev without --force: err = %v", err)
	}

	// Already current: no error, nothing written.
	out.Reset()
	rep, err := runUpdate(context.Background(), updateOptions{Current: "v2.57.0", Target: target}, &out)
	if err != nil || rep.Installed {
		t.Errorf("current: err = %v, report = %+v", err, rep)
	}
	if out.String() != "vsp 2.57.0 is the latest in "+defaultReleaseRepo+"\n" {
		t.Errorf("output = %q", out.String())
	}

	out.Reset()
	if _, err := runUpdate(context.Background(), updateOptions{Current: "v2.58.0", Target: target}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "newer than 2.57.0") {
		t.Errorf("output = %q", out.String())
	}
	if got, _ := os.ReadFile(target); string(got) != "the old binary" {
		t.Error("a refusal must not touch the binary")
	}
}

func TestUpdateEndToEndBadChecksum(t *testing.T) {
	srv := fakeRelease(t, defaultReleaseRepo, "v2.57.0", []byte("tampered"), func(asset, sum string) string {
		return strings.Repeat("ab", 32) + "  " + asset + "\n"
	})
	setUpdateBase(t, srv.URL)
	target := writeTarget(t)

	var out bytes.Buffer
	rep, err := runUpdate(context.Background(), updateOptions{Current: "v2.56.0", Target: target}, &out)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err = %v", err)
	}
	if rep.Installed {
		t.Error("reported installed after a checksum failure")
	}
	if got, _ := os.ReadFile(target); string(got) != "the old binary" {
		t.Error("a checksum failure must leave the binary alone")
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".vsp-update-*"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

func TestUpdateEndToEndMissingChecksum(t *testing.T) {
	srv := fakeRelease(t, defaultReleaseRepo, "v2.57.0", []byte("x"), func(asset, sum string) string {
		return sum + "  some-other-asset\n"
	})
	setUpdateBase(t, srv.URL)
	target := writeTarget(t)

	var out bytes.Buffer
	_, err := runUpdate(context.Background(), updateOptions{Current: "v2.56.0", Target: target}, &out)
	if err == nil || !strings.Contains(err.Error(), "no entry") {
		t.Fatalf("err = %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "the old binary" {
		t.Error("a missing checksum must leave the binary alone")
	}
}

func TestResolveReleaseRepo(t *testing.T) {
	cases := []struct {
		name    string
		flag    string
		stamped string
		want    string
	}{
		{"flag wins over stamped and default", "frd1201/vibing-steampunk", "someone-else/fork", "frd1201/vibing-steampunk"},
		{"stamped wins over default", "", "frd1201/vibing-steampunk", "frd1201/vibing-steampunk"},
		{"default when neither is set", "", "", defaultReleaseRepo},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolveReleaseRepo(c.flag, c.stamped)
			if err != nil {
				t.Fatalf("resolveReleaseRepo(%q, %q): %v", c.flag, c.stamped, err)
			}
			if got != c.want {
				t.Errorf("resolveReleaseRepo(%q, %q) = %q, want %q", c.flag, c.stamped, got, c.want)
			}
		})
	}
}

func TestResolveReleaseRepoValidation(t *testing.T) {
	good := []string{"oisee/vibing-steampunk", "frd1201/vibing-steampunk", "a-b/c.d_e"}
	for _, repo := range good {
		if got, err := resolveReleaseRepo(repo, ""); err != nil || got != repo {
			t.Errorf("resolveReleaseRepo(%q, \"\") = %q, %v; want %q, nil", repo, got, err, repo)
		}
	}

	// An empty slot falls through to the next one in the precedence chain,
	// so the empty string is checked against the regex directly below.
	bad := []string{"noslash", "a/b/c", "../x", "a/b?x", "a b/c", "x/..", "x/."}
	for _, repo := range bad {
		_, err := resolveReleaseRepo(repo, "")
		if err == nil {
			t.Errorf("resolveReleaseRepo(%q, \"\") unexpectedly succeeded", repo)
			continue
		}
		if want := fmt.Sprintf("repository %q is not owner/name", repo); err.Error() != want {
			t.Errorf("resolveReleaseRepo(%q, \"\") error = %q, want %q", repo, err.Error(), want)
		}
	}
	if releaseRepoRe.MatchString("") {
		t.Error("releaseRepoRe accepted an empty string")
	}
}

func TestUpdateStampedReleaseRepoIsUsed(t *testing.T) {
	prev := ReleaseRepo
	ReleaseRepo = "frd1201/vibing-steampunk"
	t.Cleanup(func() { ReleaseRepo = prev })

	srv := fakeRelease(t, ReleaseRepo, "v2.57.0", []byte("x"), goodChecksums)
	setUpdateBase(t, srv.URL)
	target := writeTarget(t)

	var out bytes.Buffer
	rep, err := runUpdate(context.Background(), updateOptions{Current: "v2.56.0", Target: target, Check: true}, &out)
	if err != nil {
		t.Fatalf("runUpdate: %v\n%s", err, out.String())
	}
	if rep.Repo != ReleaseRepo {
		t.Errorf("rep.Repo = %q, want %q", rep.Repo, ReleaseRepo)
	}
	if !strings.Contains(out.String(), ReleaseRepo) {
		t.Errorf("output = %q, want it to mention %q", out.String(), ReleaseRepo)
	}
}

func TestUpdateRepoFlagNoteAgainstStampedRepo(t *testing.T) {
	prev := ReleaseRepo
	ReleaseRepo = "frd1201/vibing-steampunk"
	t.Cleanup(func() { ReleaseRepo = prev })

	srv := fakeRelease(t, defaultReleaseRepo, "v2.57.0", []byte("x"), goodChecksums)
	setUpdateBase(t, srv.URL)
	target := writeTarget(t)

	var out bytes.Buffer
	if _, err := runUpdate(context.Background(), updateOptions{Current: "v2.56.0", Target: target, Check: true, Repo: defaultReleaseRepo}, &out); err != nil {
		t.Fatalf("runUpdate: %v\n%s", err, out.String())
	}
	wantNote := "note: this build was released from " + ReleaseRepo + "; --repo points at " + defaultReleaseRepo + "\n"
	if !strings.HasPrefix(out.String(), wantNote) {
		t.Errorf("output = %q, want it to start with %q", out.String(), wantNote)
	}

	out.Reset()
	rep, err := runUpdate(context.Background(), updateOptions{Current: "v2.56.0", Target: target, Check: true, Repo: defaultReleaseRepo, JSON: true}, &out)
	if err != nil {
		t.Fatalf("runUpdate JSON: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "note:") {
		t.Errorf("json output = %q, must not carry the note", out.String())
	}
	if rep.Repo != defaultReleaseRepo {
		t.Errorf("rep.Repo = %q, want %q", rep.Repo, defaultReleaseRepo)
	}
}

func TestUpdateNoNoteWhenRepoFlagMatchesStamp(t *testing.T) {
	prev := ReleaseRepo
	ReleaseRepo = "frd1201/vibing-steampunk"
	t.Cleanup(func() { ReleaseRepo = prev })

	srv := fakeRelease(t, ReleaseRepo, "v2.57.0", []byte("x"), goodChecksums)
	setUpdateBase(t, srv.URL)
	target := writeTarget(t)

	var out bytes.Buffer
	if _, err := runUpdate(context.Background(), updateOptions{Current: "v2.56.0", Target: target, Check: true, Repo: ReleaseRepo}, &out); err != nil {
		t.Fatalf("runUpdate: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "note:") {
		t.Errorf("output = %q, must not carry a note when --repo matches the stamp", out.String())
	}
}

func TestUpdateInvalidRepoFlagFailsBeforeHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP request to %s", r.URL)
	}))
	defer srv.Close()
	setUpdateBase(t, srv.URL)
	target := writeTarget(t)

	var out bytes.Buffer
	_, err := runUpdate(context.Background(), updateOptions{Current: "v2.56.0", Target: target, Repo: "not a valid repo"}, &out)
	want := `repository "not a valid repo" is not owner/name`
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

func TestUpdateRepoWithoutReleaseNamesTheRepo(t *testing.T) {
	srv := fakeRelease(t, defaultReleaseRepo, "v2.57.0", []byte("x"), goodChecksums)
	setUpdateBase(t, srv.URL)
	target := writeTarget(t)

	var out bytes.Buffer
	_, err := runUpdate(context.Background(), updateOptions{Current: "v2.56.0", Target: target, Check: true, Repo: "owner/no-releases"}, &out)
	want := "no published release found in owner/no-releases"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

func TestUpdateMissingAssetMessage(t *testing.T) {
	for _, p := range [][2]string{{"linux", "386"}, {"linux", "arm"}, {"windows", "386"}} {
		asset := assetName(p[0], p[1])
		got := missingAssetError("v2.60.0", asset, p[0], p[1]).Error()
		want := "release v2.60.0 has no asset " + asset + " for this platform: " + p[0] + "/" + p[1] +
			" is no longer built since v2.60.0; build from source with: go install github.com/oisee/vibing-steampunk/cmd/vsp@latest"
		if got != want {
			t.Errorf("%s/%s:\n got  %q\n want %q", p[0], p[1], got, want)
		}
	}
	// A platform that was never released is not called dropped, but gets the same way out.
	got := missingAssetError("v2.60.0", "vsp-freebsd-amd64", "freebsd", "amd64").Error()
	if strings.Contains(got, "no longer built") || !strings.Contains(got, "freebsd/amd64 is not built") ||
		!strings.HasSuffix(got, "build from source with: "+installFromSource) {
		t.Errorf("freebsd/amd64: %q", got)
	}
}

// A release without this platform's binary is refused with the way out, and
// the installed binary is left alone.
func TestUpdateEndToEndNoAssetForPlatform(t *testing.T) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/"+defaultReleaseRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		rel := release{TagName: "v2.60.0", Assets: []releaseAsset{
			{Name: "vsp-plan9-mips", URL: srv.URL + "/dl/vsp-plan9-mips"},
			{Name: "checksums.txt", URL: srv.URL + "/dl/checksums.txt"},
		}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rel)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	setUpdateBase(t, srv.URL)
	target := writeTarget(t)

	var out bytes.Buffer
	_, err := runUpdate(context.Background(), updateOptions{Current: "v2.59.1", Target: target}, &out)
	want := missingAssetError("v2.60.0", assetName(runtime.GOOS, runtime.GOARCH), runtime.GOOS, runtime.GOARCH)
	if err == nil || err.Error() != want.Error() {
		t.Fatalf("err = %v\nwant  %v", err, want)
	}
	if got, _ := os.ReadFile(target); string(got) != "the old binary" {
		t.Error("a release without this platform's asset must leave the binary alone")
	}
}
