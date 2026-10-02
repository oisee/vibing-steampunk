# Release process: two paths, and moving it to CI

**Date:** 2026-10-02
**Question:** vsp has a CI release workflow (last used for v2.56.0) and a manual
`/celebrate` procedure (v2.57.0, v2.58.0, v2.59.0). Which one should cut
releases, and can CI do it safely?
**Answer:** CI, on a tag push, with every check done by reading the files.
Implemented on `ci/release-on-tag`.
**Update (v2.60.0):** the release builds six platforms (linux, darwin,
windows x amd64, arm64); linux-386, linux-arm and windows-386 are no longer
built. The process below is described as it is now; the dry run is as it ran.

## How we got two paths

| Date | Event |
|------|-------|
| 2025-12-02 | `/celebrate` added: `make build-all` + `gh release create`. |
| 2026-03-13 | `release.yml` (#61): workflow_dispatch, auto-bump, git-cliff CHANGELOG pushed to main, goreleaser. |
| 2026-04-07 | Three goreleaser config fixes (changelog.use → skip → disable). v2.38.x released by CI. |
| v2.43–v2.54 | Released by hand. The CHANGELOG step never ran; CHANGELOG.md went twelve releases stale (agenda, 2026-09-04). |
| v2.55.0, v2.56.0 | Released by `gh workflow run release.yml`; title and notes then replaced by hand. |
| v2.57–v2.59 | By hand via `/celebrate`. `e8c5391` (2026-09-14) fixed the procedure after it nearly shipped stale binaries. |
| 2026-09-30 | `main` ruleset: no deletion, no non-fast-forward. No required checks, no bypass actors. |

No decision to leave CI is recorded anywhere (agenda, contexts, commits). The
likely cause is mundane: `/celebrate` predates `release.yml` and was never
pointed at it, so a session asked to "celebrate" followed the manual steps.
Both CI runs that were tried succeeded. The notes were rewritten by hand each
time because git-cliff's commit list is not what this project publishes.

## What the published assets say

These were checked with `go version -m`, `file` and `--version` on the
downloaded assets. Nothing was modified.

| | v2.56.0 (CI) | v2.57.0 | v2.58.0 | v2.59.0 |
|---|---|---|---|---|
| `--version` | `2.56.0` (no `v`) | `v2.57.0` | `v2.58.0` | `v2.59.0` |
| Commit stamp | full sha | short | short | short |
| Go `vcs.revision` = tag commit | yes | yes | yes | **no: `26807cf`** |
| `vcs.modified` | false | false | **true** | **true** |
| linux-amd64 linkage | static | **dynamic (glibc)** | **dynamic** | **dynamic, glibc ≥ 2.34** |
| `LICENSE`/`NOTICE` assets | yes | yes | **no** | **no** |
| `checksums.txt` | yes | yes | yes | yes |
| `ReleaseRepo` stamp (`vsp update`, #259) | no (predates it) | no | no | no (Makefile never sets it) |
| CHANGELOG.md section | yes | no | no | no |

The v2.59.0 findings matter most:

- **Provenance.** The ldflags say `Commit=2770c5e` (the tag), but Go's own VCS
  stamp records a modified tree at `26807cf`, the head of
  `fix/hyperfocused-dispatch` in the main checkout. The binary's module
  dependencies match the tag's `go.mod`, not 26807cf's, and v2.59.0 features
  are present (`check-abap`, `update --repo`). So the content was most likely
  the tag's tree checked out over another branch. What else was in that
  working tree cannot be proven now. A clean checkout of the tag makes that
  question impossible to ask.
- **glibc.** The Makefile never sets `CGO_ENABLED=0`, so a native `make` build
  of linux-amd64 links glibc. From v2.57.0 on it will not start on Alpine,
  Debian 11, RHEL 8 or Ubuntu 20.04. The other eight are cross-compiled and
  static.
- **Apache-2.0 §4.** The binaries embed open-rfc-go. goreleaser shipped
  `LICENSE`/`NOTICE` as release assets (`extra_files`, added for exactly this
  reason). The manual path dropped them from v2.58.0 on.

None of this is fixed retroactively. v2.59.0 stays as published. The next
release is built the new way.

## The two paths compared

| Concern | Old CI (`release.yml` + goreleaser) | Manual `/celebrate` (since `e8c5391`) | New CI (this branch) |
|---|---|---|---|
| Stale binaries | none (`--clean`) | possible: `rm` is a step a human can skip; `build-all` vs `build-all-all` trap | `build` deletes `dist/`; `verify` requires exactly the 9 files (6 binaries, checksums, LICENSE, NOTICE) |
| Version vs tag | creates the tag itself; prints `2.56.0`, not the tag | `git describe`: right only if tagged *before* building, on a clean tree | `main.Version` = the tag, by construction; checked in build info for all 6 and by `--version` for all 6 (darwin-amd64 best effort) |
| Built from the tag | yes | not enforced (v2.59.0: no) | `build` refuses HEAD ≠ tag or a dirty tree; `verify` checks `vcs.revision` and `vcs.modified` |
| Platforms | same 9 (goarm 7 implicit) | 9, but linux-amd64 cgo/dynamic | 6 since v2.60.0, `CGO_ENABLED=0`, header + build info checked |
| Asset names (`vsp update`) | match | match | match (`asset_of` = `assetName`) |
| checksums.txt | yes | hand-run `sha256sum` | generated; one line per platform exactly (6), each checked |
| After upload | not checked | not checked | draft → download → byte compare → publish |
| LICENSE/NOTICE | yes | dropped since v2.58.0 | yes, compared with the repo's |
| Tests / leak scan | `go test` / none | `go test` / staged-diff grep | `go test -race` / the CI leak scanner over the tag's tree and commits since the previous tag, fails closed |
| CHANGELOG | pushed to main *after* tagging the commit before it, GITHUB_TOKEN | not updated | not pushed; generated in the release-prep PR, inside the tag; CI warns if missing |
| Release notes | git-cliff commit list (replaced by hand every time) | handwritten | README "What's New" `### vX.Y.Z` sections, else git-cliff; empty fails |
| Who decides the version | workflow input / auto-bump | human | human, by pushing the tag |

## The CHANGELOG push and the ruleset

The ruleset forbids deletion and non-fast-forward pushes. A GITHUB_TOKEN push
of one commit on top of main is still allowed, so the old step would still
work. It is still the wrong design:

- **The tag cannot contain its own entry.** The commit is made after the
  tagged commit.
- **It races merges.** If main moved during the run, the push is rejected as
  non-fast-forward.
- **A PR is not an option without changing settings.**
  `can_approve_pull_request_reviews` is false, so Actions cannot open one. A
  PR opened with GITHUB_TOKEN would not trigger CI either.

So the CHANGELOG moves into the release-prep PR, the one that already updates
the README: `git-cliff --tag vX.Y.Z -o CHANGELOG.md`. That makes it reviewed,
inside the tag, and written by nobody but a human. CI writes to no branch.

## Recommendation, as implemented

`release.yml` runs on a `v*` tag push, or on workflow_dispatch with an
existing tag. Its jobs:

1. **prepare**: tag format; the tag commit is on main, or on its own
   release/X.Y for an LTS patch (see below); refuse if a release already
   exists. This is what keeps v2.59.0 untouchable.
2. **test**: `go test -race`. Runs in parallel with the next two jobs.
3. **leak-scan**: the ci.yml scanner, `-all -rev <tag> -range <prev>..<tag>`,
   `-require-identifiers`.
4. **build-and-verify**: `.github/ci/release.sh build` and `verify`. Then
   `run` executes linux amd64, and arm64 under qemu.
5. **run**: macOS (darwin-arm64 required; darwin-amd64 best effort via
   Rosetta, reported as a warning if it cannot start) and Windows (amd64).
   windows-arm64 runs on a Windows ARM runner.
6. **publish**: README/git-cliff notes, with the title from the annotated
   tag's subject. Creates a draft, downloads it back, compares byte for byte,
   then publishes. Immediately before and after publishing it re-reads the
   tag on origin (it must still be the built commit) and every asset's GitHub
   sha256 digest; a failure after publishing turns the release back into a
   draft. It is marked latest only above every published final release, so
   a higher tag whose release failed does not block it.

One script backs both CI and the fallback. `make release-dist TAG=vX.Y.Z`
runs the same build/verify/run locally. `.goreleaser.yml` is removed, so there
is one build definition, not two that drift.

### LTS (release/X.Y)

**Update:** v2.59.x is an LTS line. `release/2.59` starts at v2.59.1
(`8d897f3`); fixes are cherry-picked onto it and tagged v2.59.2 and up.

- **Where a tag may come from** (`release.sh on-branch TAG`, called by
  prepare): reachable from `origin/main`, or from exactly one
  `origin/release/X.Y` and that X.Y is the tag's major.minor. v2.59.2 on
  release/2.59 is accepted; v2.60.9 on release/2.59 is refused; a v2.59.x only
  on another branch is refused; a tag on main is accepted as before. The error
  names every branch checked. Only `release/<digits>.<digits>` counts as a
  release branch.
- **How to cut one:** a branch from `origin/release/X.Y`,
  `git cherry-pick -x <main-sha>` for each fix (the trailer names the main
  commit), `### vX.Y.Z` (e.g. `### v2.59.2`) in that branch's README
  "What's New" as the notes, PR into release/X.Y. After the merge,
  `git tag -a vX.Y.Z -m "Release vX.Y.Z: <title>"` on release/X.Y and
  `git push origin vX.Y.Z`. The full steps are in `/celebrate`.
- **Which code runs:** a tag push runs `release.yml` as it is at the tag,
  and prepare takes `on-branch` from the tag, which must then be on main or
  its release/X.Y. `workflow_dispatch` is accepted only from main or a
  release/X.Y branch, checked in the workflow itself before any checked-out
  code runs, and a dispatched prepare takes `on-branch` from main, not from
  the dispatched branch. Every gate after prepare checks out the tag's commit and uses
  its `release.sh`, so an LTS release ships its own line's platform set
  (release/2.59 still has the nine). `release/2.59` predates `on-branch`: a
  tag pushed there is refused by its older prepare until this change is
  carried onto the branch; until then
  `gh workflow run release.yml --ref main -f tag=v2.59.2` releases it with
  main's policy and the tag's build.
- **Latest:** `latest` compares with every published final release, so a
  v2.59.2 published after v2.60.0 is not latest, and `vsp update` (which reads
  releases/latest) keeps offering v2.60.x. The workflow-level concurrency
  group is per tag, so two tags could publish at the same moment and both
  compute latest=true; the publish job therefore has its own group,
  `release-publish`, shared by all tags, and `release.sh publish` reads the
  published set right before `--draft=false`. (GitHub keeps one pending job
  per group: a third queued publish is cancelled before anything goes public,
  and is dispatched again.) `.github/ci/release-test.sh` checks this with a
  stateful fake `gh`: v2.60.0 and v2.59.2 drafts published in either order
  leave v2.60.0 as the last one marked latest, and v2.59.2 after v2.60.0 gets
  latest=false. It also checks the branch rule in a throwaway repository; CI's
  build job runs it.

### Dry run (local, no tag pushed, no release touched)

This ran in a throwaway clone with a local-only tag `v2.60.0-dryrun.1`.

- **Build and verify:** all 9 built, all checks passed. Two builds of the same
  tag gave identical checksums.
- **Execution:** linux-amd64 and linux-386 ran. A darwin binary on Linux
  passed in best-effort mode and failed when required.
- **Negative cases, each refused:**
  - an arm64 binary under the amd64 name (header, GOARCH, checksum);
  - a stale extra file;
  - one appended byte;
  - v2.59.0's linux-amd64 under the new tag (revision, modified, cgo,
    version);
  - a dirty tree;
  - HEAD ≠ tag;
  - a malformed tag;
  - an unignored output directory;
  - a tampered or missing asset in `compare`.
- **The published v2.59.0 assets** fail `verify`: wrong revision, modified
  tree, cgo, no LICENSE/NOTICE.
- **Notes:** the v2.59.0 notes come out of the README (158 lines, both
  sections). The git-cliff fallback works.
- **Lint:** actionlint (with shellcheck) is clean on `release.yml`, and
  shellcheck is clean on `release.sh`.

**Not exercised locally:** the qemu runs, the macOS and Windows runners, and
the GitHub API steps (draft, download, publish). `act` is not installed, and
those steps need a real tag and release anyway. The first real tag push is
the first full run. If it fails, it fails before anything is public.

### Residual risks

- darwin-amd64 is only executed if the macOS image has Rosetta.
- A window remains between the last digest check and GitHub serving the
  release. Release immutability (a repository setting) closes it for
  published releases; enabling it is the owner's call.
- The README section becomes the release body verbatim. v2.59.0's was a
  hand-condensed summary, and it can still be edited after publishing.
- The leak scan needs the `VSP_LEAK_IDENTIFIERS` secret to be available to
  tag-push runs. It is a repository secret, so it should be.
