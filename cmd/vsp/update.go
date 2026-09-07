package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// updateAPIBase is the GitHub REST prefix the release lookups are built on.
// It is a variable so a test can point the whole flow at an httptest server.
var updateAPIBase = "https://api.github.com/repos/oisee/vibing-steampunk"

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type release struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

// updateReport is what --json prints and what the tests inspect.
type updateReport struct {
	Current      string `json:"current"`
	CurrentKnown bool   `json:"current_known"`
	Latest       string `json:"latest"`
	Asset        string `json:"asset"`
	Available    bool   `json:"update_available"`
	Installed    bool   `json:"installed"`
	Path         string `json:"path,omitempty"`
	Size         int64  `json:"size,omitempty"`
}

type updateOptions struct {
	Current string // the running version, normally main.Version
	Tag     string // a specific release tag; empty means latest
	Target  string // the file to replace; empty means the running executable
	Check   bool
	Force   bool
	JSON    bool
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Replace this binary with the latest GitHub release",
	Long: `Download the release built for this OS and architecture from
github.com/oisee/vibing-steampunk, verify it against checksums.txt, and
swap it in place of the running executable.

  vsp update                  # install the latest release if it is newer
  vsp update --check          # only report what would happen
  vsp update --version v2.57.0
  vsp update --force          # install even if not newer, or from a dev build

The current binary is kept as <path>.old until the swap has succeeded; on
Windows the running executable cannot be deleted, so the .old file stays
behind for you to remove.

GITHUB_TOKEN or GH_TOKEN, when set, is sent to the GitHub API to lift the
anonymous rate limit.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := updateOptions{Current: Version}
		opts.Check, _ = cmd.Flags().GetBool("check")
		opts.Force, _ = cmd.Flags().GetBool("force")
		opts.JSON, _ = cmd.Flags().GetBool("json")
		opts.Tag, _ = cmd.Flags().GetString("version")
		_, err := runUpdate(cmd.Context(), opts, os.Stdout)
		return err
	},
}

func init() {
	updateCmd.Flags().Bool("check", false, "Only report the current and latest version")
	updateCmd.Flags().Bool("force", false, "Install even if the release is not newer or the current version is unknown")
	updateCmd.Flags().String("version", "", "Install this release tag instead of the latest (e.g. v2.57.0)")
	updateCmd.Flags().Bool("json", false, "Print the result as JSON")
	rootCmd.AddCommand(updateCmd)
}

// runUpdate is the whole command behind a testable seam: the release comes
// from updateAPIBase and the binary goes to opts.Target.
func runUpdate(ctx context.Context, opts updateOptions, out io.Writer) (*updateReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rel, err := fetchRelease(ctx, opts.Tag)
	if err != nil {
		return nil, err
	}

	asset := assetName(runtime.GOOS, runtime.GOARCH)
	rep := &updateReport{
		Current: strings.TrimPrefix(opts.Current, "v"),
		Latest:  strings.TrimPrefix(rel.TagName, "v"),
		Asset:   asset,
	}
	cur, curOK := parseVersion(opts.Current)
	latest, latestOK := parseVersion(rel.TagName)
	if !latestOK {
		return nil, fmt.Errorf("release tag %q is not a version", rel.TagName)
	}
	rep.CurrentKnown = curOK
	rep.Available = !curOK || newerThan(latest, cur)

	if opts.Check {
		if opts.JSON {
			return rep, printJSONTo(out, rep)
		}
		switch {
		case !curOK:
			fmt.Fprintf(out, "vsp %s: version unknown, latest is %s (%s); --force installs it\n", rep.Current, rep.Latest, asset)
		case rep.Available:
			fmt.Fprintf(out, "vsp %s, latest is %s (%s): update available\n", rep.Current, rep.Latest, asset)
		default:
			fmt.Fprintf(out, "vsp %s is the latest\n", rep.Current)
		}
		return rep, nil
	}

	if !opts.Force {
		if !curOK {
			return rep, fmt.Errorf("vsp %s is not a release version; --force installs %s (%s)", rep.Current, rep.Latest, asset)
		}
		if !rep.Available {
			if opts.JSON {
				return rep, printJSONTo(out, rep)
			}
			if newerThan(cur, latest) {
				fmt.Fprintf(out, "vsp %s is newer than %s; --force installs it anyway\n", rep.Current, rep.Latest)
			} else {
				fmt.Fprintf(out, "vsp %s is the latest\n", rep.Current)
			}
			return rep, nil
		}
	}

	var dl *releaseAsset
	for i := range rel.Assets {
		if rel.Assets[i].Name == asset {
			dl = &rel.Assets[i]
			break
		}
	}
	if dl == nil {
		return rep, fmt.Errorf("release %s has no asset %s for this platform", rel.TagName, asset)
	}
	var sums *releaseAsset
	for i := range rel.Assets {
		if rel.Assets[i].Name == "checksums.txt" {
			sums = &rel.Assets[i]
			break
		}
	}
	if sums == nil {
		return rep, fmt.Errorf("release %s has no checksums.txt; refusing to install unverified", rel.TagName)
	}
	sumsText, err := fetchBody(ctx, sums.URL)
	if err != nil {
		return rep, fmt.Errorf("checksums.txt: %w", err)
	}
	want, ok := checksumFor(string(sumsText), asset)
	if !ok {
		return rep, fmt.Errorf("checksums.txt of %s has no entry for %s; refusing to install unverified", rel.TagName, asset)
	}

	target := opts.Target
	if target == "" {
		target, err = executableTarget(out)
		if err != nil {
			return rep, err
		}
	}
	rep.Path = target

	tmp, err := os.CreateTemp(filepath.Dir(target), ".vsp-update-*")
	if err != nil {
		return rep, fmt.Errorf("cannot write next to %s: %w", target, err)
	}
	tmpPath := tmp.Name()
	size, got, err := downloadTo(ctx, dl.URL, tmp)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmpPath)
		return rep, fmt.Errorf("download %s: %w", asset, err)
	}
	if got != want {
		os.Remove(tmpPath)
		return rep, fmt.Errorf("%s: sha256 %s does not match checksums.txt (%s); nothing installed", asset, got, want)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0o755); err != nil {
			os.Remove(tmpPath)
			return rep, err
		}
	}
	if err := replaceExecutable(target, tmpPath); err != nil {
		os.Remove(tmpPath)
		return rep, err
	}
	rep.Installed = true
	rep.Size = size

	old := target + ".old"
	leftover := os.Remove(old) != nil
	if opts.JSON {
		return rep, printJSONTo(out, rep)
	}
	fmt.Fprintf(out, "vsp %s → %s (%s, %s) installed to %s\n", rep.Current, rep.Latest, asset, formatSize(size), target)
	if leftover {
		fmt.Fprintf(out, "%s can be deleted later\n", old)
	}
	return rep, nil
}

// executableTarget resolves the running binary; a symlink is followed and the
// file it points at is what gets replaced, since replacing the link would
// silently detach it from the install it was managing.
func executableTarget(out io.Writer) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	if real != exe {
		fmt.Fprintf(out, "%s is a symlink; replacing its target %s\n", exe, real)
	}
	return real, nil
}

// replaceExecutable moves target aside as target.old and puts tmp in its
// place. Both steps are renames within one directory, so nothing is ever
// half-written, and a running executable can be renamed on Windows where it
// cannot be overwritten. A failed second rename puts the original back.
func replaceExecutable(target, tmp string) error {
	old := target + ".old"
	// A leftover from an earlier update on Windows would block the first rename.
	_ = os.Remove(old)
	if err := os.Rename(target, old); err != nil {
		return fmt.Errorf("cannot move %s aside: %w", target, err)
	}
	if err := os.Rename(tmp, target); err != nil {
		if back := os.Rename(old, target); back != nil {
			return fmt.Errorf("installing %s failed: %v; and the previous binary is stuck at %s: %v", target, err, old, back)
		}
		return fmt.Errorf("installing %s failed: %w (previous binary restored)", target, err)
	}
	return nil
}

// parseVersion reads a release tag or ldflags version as major.minor.patch.
// A leading v and any -prerelease or +build suffix are tolerated; "dev" and
// other non-numeric strings are unknown, not zero.
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimSpace(strings.TrimPrefix(s, "v"))
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return v, false
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		v[i] = n
	}
	return v, true
}

func newerThan(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

// assetName follows the archives.name_template in .goreleaser.yml.
func assetName(goos, goarch string) string {
	name := "vsp-" + goos + "-" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// checksumFor finds the sha256 for asset in a checksums.txt of
// "<hex>  <name>" lines; the sha256sum binary-mode "*name" form is accepted.
func checksumFor(text, asset string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == asset {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

func fetchRelease(ctx context.Context, tag string) (*release, error) {
	url := updateAPIBase + "/releases/latest"
	if tag != "" {
		if _, ok := parseVersion(tag); ok && !strings.HasPrefix(tag, "v") {
			tag = "v" + tag
		}
		url = updateAPIBase + "/releases/tags/" + tag
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "vsp/"+Version)
	if tok := githubToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound && tag != "" {
		return nil, fmt.Errorf("release %s not found", tag)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GET %s: %s: %s", url, resp.Status, strings.TrimSpace(string(body)))
	}
	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("release JSON: %w", err)
	}
	if rel.TagName == "" {
		return nil, errors.New("release JSON has no tag_name")
	}
	return &rel, nil
}

// githubToken is only sent to the API host. Asset downloads redirect to a
// CDN where a bearer token is at best dropped, so they stay anonymous.
func githubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GH_TOKEN")
}

func openDownload(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "vsp/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp, nil
}

func fetchBody(ctx context.Context, url string) ([]byte, error) {
	resp, err := openDownload(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// downloadTo streams url into w and returns the byte count and the sha256
// of what was written, so the verification never re-reads the file.
func downloadTo(ctx context.Context, url string, w io.Writer) (int64, string, error) {
	resp, err := openDownload(ctx, url)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), resp.Body)
	if err != nil {
		return n, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func printJSONTo(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func formatSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
