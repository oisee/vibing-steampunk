package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
// checksums text is chosen per call so a test can serve a wrong one.
func fakeRelease(t *testing.T, tag string, binary []byte, checksums func(asset, sum string) string) *httptest.Server {
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
	mux.HandleFunc("/releases/latest", serveRelease)
	mux.HandleFunc("/releases/tags/"+tag, serveRelease)
	mux.HandleFunc("/releases/tags/", func(w http.ResponseWriter, r *http.Request) {
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
	prev := updateAPIBase
	updateAPIBase = url
	t.Cleanup(func() { updateAPIBase = prev })
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
	srv := fakeRelease(t, "v2.57.0", binary, goodChecksums)
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
	want := "vsp 2.56.0 → 2.57.0 (" + rep.Asset + ", 1.7 KB) installed to " + target + "\n"
	if runtime.GOOS != "windows" && out.String() != want {
		t.Errorf("output = %q\nwant     %q", out.String(), want)
	}
}

func TestUpdateEndToEndSpecificTagJSON(t *testing.T) {
	binary := []byte("pinned release")
	srv := fakeRelease(t, "v2.50.0", binary, goodChecksums)
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
	if got, _ := os.ReadFile(target); !bytes.Equal(got, binary) {
		t.Error("target does not hold the pinned binary")
	}

	if _, err := runUpdate(context.Background(), updateOptions{Current: "2.56.0", Tag: "v9.9.9", Target: target, Force: true}, &out); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unknown tag: err = %v", err)
	}
}

func TestUpdateEndToEndCheckOnly(t *testing.T) {
	srv := fakeRelease(t, "v2.57.0", []byte("x"), goodChecksums)
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
	if _, err := runUpdate(context.Background(), updateOptions{Current: "v2.57.0", Target: target, Check: true}, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "vsp 2.57.0 is the latest\n" {
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
	srv := fakeRelease(t, "v2.57.0", []byte("x"), goodChecksums)
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
	if out.String() != "vsp 2.57.0 is the latest\n" {
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
	srv := fakeRelease(t, "v2.57.0", []byte("tampered"), func(asset, sum string) string {
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
	srv := fakeRelease(t, "v2.57.0", []byte("x"), func(asset, sum string) string {
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
