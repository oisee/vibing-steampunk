# Celebrate Milestone Release

You are celebrating a project milestone. **Releases are cut by CI:** push the
tag, `.github/workflows/release.yml` does the rest, and you check what it did.
The manual path at the end is the fallback for when CI cannot run.

Why CI, in one paragraph: every manual release so far has had to be checked by
hand for the same things. v2.58.0 almost shipped six binaries of the previous
release (`build-all` builds three platforms). A binary built before the tag says
`v2.57.0-25-g977f968`. v2.58.0 and v2.59.0 went out without `LICENSE`/`NOTICE`.
v2.59.0's binaries record a Go VCS stamp of a modified checkout of another
branch, not the tag. The workflow builds from a clean checkout of the tag and
refuses all of those, by reading the files instead of trusting exit codes.

## 1. Gather Release Information

Ask the user for:
- **Version number** (e.g., v2.60.0) — suggest based on changes (major/minor/patch)
- **Release title** (e.g., "only what you saw") — it goes into the tag message
- **Key highlights** — they go into the README, which becomes the release notes

## 2. Release-prep PR (everything the tag should contain)

On a branch, in one PR:

- **README.md "What's New"**: add `### vX.Y.Z — new since vA.B.C` (and, if any,
  `### vX.Y.Z — behaviour changes since vA.B.C`). Every `### vX.Y.Z …` section
  there becomes the release notes, verbatim. Without one, CI falls back to
  git-cliff's commit list and warns.
- **CHANGELOG.md**: `git-cliff --tag vX.Y.Z -o CHANGELOG.md` (git-cliff 2.12.0,
  `uvx git-cliff` works). CI no longer pushes this to main after the fact, so
  it lands here, reviewed, and inside the tag.
- README/CLAUDE.md version references, if any.

Before merging: `go test ./...`, and the leak scan runs in CI on the PR.

## 3. Tag and push the tag

After the PR is merged, on an up-to-date `main`:

```bash
git switch main && git pull --ff-only
git tag -a vX.Y.Z -m "Release vX.Y.Z: <title>"
git push origin vX.Y.Z
```

The tag subject after `vX.Y.Z: ` becomes the release title (`vX.Y.Z: <title>`).
Push **only the tag**; CI pushes nothing anywhere.

## 4. What CI does

`release.yml`, on the tag push (or `gh workflow run release.yml -f tag=vX.Y.Z`
for an existing tag that has no release yet):

1. **prepare**: tag format, tag commit is on `main`, no release exists for it.
2. **test**: `go test -race ./...` at the tag.
3. **leak scan**: the tag's tree and the commits since the previous tag, with
   the `VSP_LEAK_IDENTIFIERS` list (fails closed without it).
4. **build and verify** (`.github/ci/release.sh`): six binaries + `checksums.txt`
   + `LICENSE` + `NOTICE`, then by content:
   - exactly that file set, nothing stale;
   - each executable header is the platform its name says (ELF/Mach-O/PE + arch);
   - Go's build info in each binary: GOOS/GOARCH, `vcs.revision` = the
     tag commit, `vcs.modified=false`, `main.Version` = the tag;
   - `checksums.txt` has exactly the six lines and every one matches;
   - `--version` prints exactly `vsp version vX.Y.Z (commit: <sha>, …)` for
     linux amd64, and arm64 under qemu.
5. **run**: the same `--version` check on macOS (darwin-arm64; darwin-amd64 if
   Rosetta is there, else a warning) and Windows (amd64; arm64 on a
   Windows ARM runner).
6. **publish**: notes from the README (else git-cliff), release created as a
   **draft**, every asset downloaded back and compared byte for byte. Just
   before and after publishing, the tag on origin must still be the built
   commit and every asset's GitHub sha256 digest must match the build; if the
   check after publishing fails, the release goes back to draft. It is marked
   latest only above every published final release.

## 5. What to check

```bash
gh run watch                                   # or: gh run list --workflow release.yml -L 1
gh release view vX.Y.Z --json name,isDraft,assets -q '.name, .isDraft, [.assets[].name]'
```

- 9 assets: 6 `vsp-*`, `checksums.txt`, `LICENSE`, `NOTICE`; not a draft.
- Read the run's warnings: README section missing, CHANGELOG section missing,
  darwin-amd64 not executed. None blocks; each is something to know.
- One look from outside, on your own platform:

```bash
vsp update --check                              # an older vsp sees the new release
gh release download vX.Y.Z -p "vsp-$(go env GOOS)-$(go env GOARCH)*" -D /tmp/vsp-new
chmod +x /tmp/vsp-new/vsp-* && /tmp/vsp-new/vsp-* --version
/tmp/vsp-new/vsp-* <a command added this release> --help   # the feature is really in there
```

The release body is the README section; edit it on GitHub afterwards if a
shorter summary reads better (v2.59.0's was hand-condensed).

**If a run fails**: nothing is public. Fix the cause; if a draft was left
behind, delete the draft (never the tag, never a published release), then
`gh workflow run release.yml -f tag=vX.Y.Z`. If the fix needs a code change,
it is a new tag (vX.Y.Z+1): a pushed tag is never moved.

## 6. Fallback: manual release (CI unavailable)

Same script, same checks, run by hand. Do it in a **fresh worktree of the tag**
— the script refuses a dirty tree or a HEAD that is not the tag, which is
exactly what went wrong with v2.59.0's build stamp.

```bash
git worktree add .local/worktrees/release-vX.Y.Z vX.Y.Z
cd .local/worktrees/release-vX.Y.Z
make release-dist TAG=vX.Y.Z           # build + verify + run this platform's binary
./.github/ci/release.sh notes vX.Y.Z /tmp/notes.md

pre=(); case vX.Y.Z in *-*) pre=(--prerelease) ;; esac
./.github/ci/release.sh tag-at vX.Y.Z "$(git rev-parse 'vX.Y.Z^{commit}')"
gh release create vX.Y.Z --draft --verify-tag "${pre[@]}" \
  --title "vX.Y.Z: <title>" --notes-file /tmp/notes.md dist/*
gh release download vX.Y.Z -D /tmp/served
./.github/ci/release.sh compare dist /tmp/served
./.github/ci/release.sh publish vX.Y.Z "$(git rev-parse 'vX.Y.Z^{commit}')" dist   # the same guarded publish CI runs

cd - && git worktree remove .local/worktrees/release-vX.Y.Z
```

`make build-all-all` still exists for development builds; it is not a release
build (no ReleaseRepo stamp, version from `git describe`, no checks).

## 7. Celebrate!

Report the release URL and a summary of what was shipped!
