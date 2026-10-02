#!/usr/bin/env bash
# shellcheck disable=SC2012  # ls is fine: the directories hold only fixed asset names
# release.sh — build and verify a vsp release. The release workflow
# (.github/workflows/release.yml) calls exactly these subcommands, and so does
# the manual fallback (`make release-dist TAG=vX.Y.Z`), so a dry run on a laptop
# runs the same checks CI runs.
#
#   release.sh on-branch TAG               the tag is on origin/main, or on origin/release/X.Y of its own X.Y (LTS)
#   release.sh build   TAG [DIST]       one binary per PLATFORMS entry + checksums.txt + LICENSE + NOTICE
#   release.sh verify  TAG [DIST]          by content: names, headers, build info, checksums
#   release.sh run     TAG DIST SPEC...    execute binaries; each must print exactly TAG
#   release.sh notes   TAG [OUT]           release notes: README "What's New", else git-cliff
#   release.sh compare DIST DOWNLOADED     what the release serves is byte-identical to DIST
#   release.sh tag-at  TAG SHA             the tag on origin still points at SHA
#   release.sh digests TAG DIST            the release's asset set and GitHub sha256 digests match DIST
#   release.sh latest  TAG                 "true" if TAG is a final version above every published final release
#   release.sh publish TAG SHA DIST        guarded draft -> published (tag-at, digests, latest; back to draft on a late failure)
#
# Every check reads the files themselves. An exit 0 from `go build` says nothing
# about which source went in or which platform came out: v2.58.0 nearly shipped
# six binaries of the previous release, and v2.59.0's binaries carry a Go VCS
# stamp of a different, modified checkout than the commit their ldflags name.
#
# Portable to bash 3.2 (macOS runners) and Git Bash (Windows runners): no
# associative arrays, no mapfile, no GNU-only flags.
set -euo pipefail

# The six assets every release ships since v2.60.0 (linux/386, linux/arm and
# windows/386 were dropped then). `vsp update` downloads
# assetName(GOOS, GOARCH) = vsp-<os>-<arch>[.exe] (cmd/vsp/update.go) and refuses
# one without a checksums.txt entry, so these names are an interface. Every
# count below is derived from this list; there is no second copy of the number.
PLATFORMS="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"
# shellcheck disable=SC2086 # split on purpose: one platform per word
NPLATFORMS=$(set -- $PLATFORMS; echo $#)
EXTRA_FILES="LICENSE NOTICE" # Apache-2.0 s.4 (open-rfc-go is embedded); the binaries are bare
TAG_RE='^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'

die() { echo "release.sh: $*" >&2; exit 1; }
fail=0
bad() { echo "FAIL: $*" >&2; fail=1; }
ok() { echo "ok:   $*"; }
gh_warn() { if [ -n "${GITHUB_ACTIONS:-}" ]; then echo "::warning title=release::$*"; else echo "WARN: $*" >&2; fi; }

asset_of() { # os/arch -> asset file name, as cmd/vsp/update.go assetName
	local os=${1%/*} arch=${1#*/}
	if [ "$os" = windows ]; then echo "vsp-$os-$arch.exe"; else echo "vsp-$os-$arch"; fi
}

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
	else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

hexbytes() { # file offset count -> lowercase hex, no spaces
	od -An -tx1 -j "$2" -N "$3" "$1" | tr -d ' \n'
}

check_tag() {
	echo "$1" | grep -Eq "$TAG_RE" || die "'$1' is not a release tag (want vMAJOR.MINOR.PATCH[-pre])"
}

tag_commit() { git rev-parse --verify --quiet "refs/tags/$1^{commit}" || die "tag $1 does not exist locally"; }

# ------------------------------------------------------------------ on-branch
# Where a tag may come from. A release is cut from main, or, for an LTS line,
# from release/X.Y: v2.59.2 is cherry-picked onto release/2.59 and tagged there.
# Accepted when the tag's commit is reachable from origin/main, or from exactly
# one origin/release/X.Y branch and that branch's X.Y is the tag's major.minor.
# So v2.60.9 on release/2.59 is refused, and so is v2.59.2 on any other branch.
# It reads origin's branches as already fetched (refs/remotes/origin/*) and
# fetches nothing itself: the caller fetches main and release/*.
cmd_on_branch() {
	local tag=$1 sha xy b branches checked hits=
	check_tag "$tag"
	sha=$(tag_commit "$tag")
	git rev-parse --verify --quiet refs/remotes/origin/main >/dev/null ||
		die "origin/main is not fetched: git fetch origin '+refs/heads/main:refs/remotes/origin/main'"
	if git merge-base --is-ancestor "$sha" refs/remotes/origin/main; then
		ok "$tag ($sha) is on origin/main"; return
	fi
	xy=$(echo "$tag" | sed -E 's/^v([0-9]+\.[0-9]+)\..*$/\1/')
	branches=$(git for-each-ref --format='%(refname:lstrip=3)' refs/remotes/origin/release |
		grep -E '^release/[0-9]+\.[0-9]+$' || true)
	checked="origin/main"
	for b in $branches; do
		checked="$checked origin/$b"
		if git merge-base --is-ancestor "$sha" "refs/remotes/origin/$b"; then hits="$hits $b"; fi
	done
	hits=${hits# }
	case $hits in
	"release/$xy")
		ok "$tag ($sha) is on origin/release/$xy (LTS)" ;;
	"")
		die "$tag ($sha) is on none of the branches a release comes from (checked: $checked). Tag main, or tag an LTS patch on release/$xy." ;;
	*" "*)
		die "$tag ($sha) is on more than one release branch ($hits; checked: $checked). An LTS tag must be on exactly one: release/$xy." ;;
	*)
		die "$tag ($sha) is on origin/$hits, not on origin/main (checked: $checked). A v$xy.x tag belongs on main or on release/$xy, never on $hits." ;;
	esac
}

# ---------------------------------------------------------------------- build
cmd_build() {
	local tag=$1 dist=${2:-dist}
	check_tag "$tag"
	local sha head
	sha=$(tag_commit "$tag")
	head=$(git rev-parse HEAD)
	[ "$sha" = "$head" ] || die "HEAD is $head but $tag is $sha: check out the tag first (git checkout $tag)"
	# A dirty tree builds something the tag does not contain; go build would
	# stamp vcs.modified=true and verify would refuse it later anyway.
	[ -z "$(git status --porcelain)" ] || die "the working tree is not clean: commit, or build from a fresh worktree of $tag"

	# The build date is the commit's, not the clock's, so two builds of one tag
	# with the same toolchain in the same directory give the same bytes.
	local date
	date=$(TZ=UTC git log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ "$sha")
	local repo=${RELEASE_REPO-${GITHUB_REPOSITORY:-}}
	local ldflags="-s -w -X main.Version=$tag -X main.Commit=$sha -X main.BuildDate=$date -X main.ReleaseRepo=$repo"

	rm -rf "$dist" # nothing from an earlier build can survive into this one
	mkdir -p "$dist"
	: > "$dist/.probe"
	# Inside the repository, an unignored output directory dirties the tree for
	# every build after the first (vcs.modified=true). dist/ is in .gitignore.
	[ -z "$(git status --porcelain)" ] || { rm -rf "$dist"; die "$dist is inside the repository and not ignored: use dist/ or a directory outside it"; }
	rm -f "$dist/.probe"
	local p os arch out
	for p in $PLATFORMS; do
		os=${p%/*}; arch=${p#*/}
		out="$dist/$(asset_of "$p")"
		echo "build $out"
		# CGO_ENABLED=0 pinned: a native build otherwise links glibc (v2.57.0-v2.59.0's
		# linux-amd64 needs glibc 2.34). No -trimpath: it drops -ldflags from
		# the build info, and verify reads main.Version from there for the
		# binaries no runner can start.
		CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
			go build -ldflags "$ldflags" -o "$out" ./cmd/vsp
	done
	(cd "$dist" && for p in $PLATFORMS; do a=$(asset_of "$p"); echo "$(sha256_of "$a")  $a"; done) > "$dist/checksums.txt"
	local f
	for f in $EXTRA_FILES; do cp "$f" "$dist/$f"; done
	echo "built $tag ($sha) into $dist/"
}

# --------------------------------------------------------------------- verify
expect_header() { # file os arch -> 0 if the executable header is that platform
	local f=$1 os=$2 arch=$3 magic cls mach off pe
	case $os in
	linux)
		magic=$(hexbytes "$f" 0 4); cls=$(hexbytes "$f" 4 1); mach=$(hexbytes "$f" 18 2)
		[ "$magic" = 7f454c46 ] || { echo "not ELF (magic $magic)"; return 1; }
		case $arch in
		amd64) [ "$cls$mach" = 023e00 ] ;;
		arm64) [ "$cls$mach" = 02b700 ] ;;
		*) false ;; # an architecture without a header rule is never a pass
		esac || { echo "ELF class $cls machine $mach is not $arch"; return 1; } ;;
	darwin)
		magic=$(hexbytes "$f" 0 4); mach=$(hexbytes "$f" 4 4)
		[ "$magic" = cffaedfe ] || { echo "not 64-bit Mach-O (magic $magic)"; return 1; }
		case $arch in
		amd64) [ "$mach" = 07000001 ] ;;
		arm64) [ "$mach" = 0c000001 ] ;;
		*) false ;; # an architecture without a header rule is never a pass
		esac || { echo "Mach-O cputype $mach is not $arch"; return 1; } ;;
	windows)
		[ "$(hexbytes "$f" 0 2)" = 4d5a ] || { echo "no MZ header"; return 1; }
		off=$(hexbytes "$f" 60 4) # e_lfanew, little-endian
		off=$((16#${off:6:2}${off:4:2}${off:2:2}${off:0:2}))
		pe=$(hexbytes "$f" "$off" 4); mach=$(hexbytes "$f" $((off + 4)) 2)
		[ "$pe" = 50450000 ] || { echo "no PE signature at $off"; return 1; }
		case $arch in
		amd64) [ "$mach" = 6486 ] ;;
		arm64) [ "$mach" = 64aa ] ;;
		*) false ;; # an architecture without a header rule is never a pass
		esac || { echo "PE machine $mach is not $arch"; return 1; } ;;
	*) echo "no header rule for $os"; return 1 ;;
	esac
}

buildinfo() { # file key -> value from `go version -m` (build settings)
	# Lines are "<tab>build<tab>key=value"; a value with spaces is Go-quoted.
	go version -m "$1" | awk -v k="$2" -F '\t' '$2 == "build" {
		i = index($3, "="); if (substr($3, 1, i - 1) != k) next
		v = substr($3, i + 1); if (v ~ /^".*"$/) v = substr(v, 2, length(v) - 2)
		print v; exit }'
}

cmd_verify() {
	local tag=$1 dist=${2:-dist}
	check_tag "$tag"
	local sha
	sha=$(tag_commit "$tag")
	[ -d "$dist" ] || die "$dist does not exist"

	# 1. Exactly the expected files: nothing missing, nothing stale left over.
	local want got
	want=$(for p in $PLATFORMS; do asset_of "$p"; done; echo checksums.txt; for f in $EXTRA_FILES; do echo "$f"; done)
	want=$(echo "$want" | LC_ALL=C sort)
	got=$(cd "$dist" && ls -1A | LC_ALL=C sort)
	if [ "$want" = "$got" ]; then ok "file set: $NPLATFORMS binaries, checksums.txt, $EXTRA_FILES"
	else bad "file set differs from the expected one:"; diff <(echo "$want") <(echo "$got") >&2 || true; fi

	# 2. Per binary: the header is the platform the name claims, and Go's own
	#    build record says this tag's commit, unmodified, with this version.
	local p os arch a f why v
	for p in $PLATFORMS; do
		os=${p%/*}; arch=${p#*/}; a=$(asset_of "$p"); f="$dist/$a"
		[ -f "$f" ] || { bad "$a missing"; continue; }
		if why=$(expect_header "$f" "$os" "$arch"); then ok "$a header is $os/$arch"; else bad "$a header: $why"; fi
		[ "$(buildinfo "$f" GOOS)" = "$os" ] || bad "$a build info GOOS=$(buildinfo "$f" GOOS)"
		[ "$(buildinfo "$f" GOARCH)" = "$arch" ] || bad "$a build info GOARCH=$(buildinfo "$f" GOARCH)"
		[ "$(buildinfo "$f" CGO_ENABLED)" = 0 ] || bad "$a CGO_ENABLED=$(buildinfo "$f" CGO_ENABLED)"
		v=$(buildinfo "$f" vcs.revision)
		[ "$v" = "$sha" ] || bad "$a was built from ${v:-an unknown revision}, not $tag ($sha)"
		v=$(buildinfo "$f" vcs.modified)
		[ "$v" = false ] || bad "$a was built from a modified tree (vcs.modified=${v:-unknown})"
		v=$(buildinfo "$f" -ldflags)
		case " $v " in *" -X main.Version=$tag "*) ;; *) bad "$a ldflags do not set main.Version=$tag: $v" ;; esac
		case " $v " in *" -X main.Commit=$sha "*) ;; *) bad "$a ldflags do not set main.Commit=$sha" ;; esac
	done
	[ "$fail" = 0 ] && ok "build info: every binary is $tag, $sha, unmodified"

	# 3. checksums.txt: one line per binary, no others, and every line holds.
	local sums="$dist/checksums.txt" n line hex name
	if [ -f "$sums" ]; then
		n=$(grep -c . "$sums" || true)
		[ "$n" = "$NPLATFORMS" ] || bad "checksums.txt has $n lines, want $NPLATFORMS"
		for p in $PLATFORMS; do
			a=$(asset_of "$p")
			line=$(grep -E "^[0-9a-f]{64}  \*?$a\$" "$sums" || true)
			[ -n "$line" ] || { bad "checksums.txt has no line for $a"; continue; }
			hex=${line%% *}
			[ "$hex" = "$(sha256_of "$dist/$a")" ] || bad "checksums.txt: $a does not match"
		done
		while IFS= read -r line; do
			name=${line##* }; name=${name#\*}
			case " $(for p in $PLATFORMS; do asset_of "$p"; done | tr '\n' ' ') " in
			*" $name "*) ;; *) bad "checksums.txt names an unexpected file: $name" ;; esac
		done < "$sums"
		[ "$fail" = 0 ] && ok "checksums.txt: $NPLATFORMS entries, all match"
	else
		bad "checksums.txt missing"
	fi

	for f in $EXTRA_FILES; do
		cmp -s "$f" "$dist/$f" || bad "$dist/$f is not the repository's $f"
	done

	[ "$fail" = 0 ] || die "verify $tag: FAILED"
	echo "verify $tag: all checks passed"
}

# ------------------------------------------------------------------------ run
# SPEC is ASSET, ASSET@LAUNCHER (e.g. vsp-linux-arm64@qemu-aarch64-static), or
# either prefixed with '?' for best effort: a binary this machine cannot start is
# reported as a warning, never passed silently; one that starts must be right.
cmd_run() {
	local tag=$1 dist=$2; shift 2
	check_tag "$tag"
	local sha
	sha=$(tag_commit "$tag")
	[ $# -gt 0 ] || die "run: name at least one asset"
	local spec try a launcher out first status
	for spec in "$@"; do
		try=; case $spec in '?'*) try=1; spec=${spec#\?} ;; esac
		a=${spec%%@*}; launcher=; [ "$a" = "$spec" ] || launcher=${spec#*@}
		[ -f "$dist/$a" ] || { bad "$a missing from $dist"; continue; }
		chmod +x "$dist/$a" 2>/dev/null || true
		status=0
		out=$($launcher "$dist/$a" --version 2>&1) || status=$?
		first=$(echo "$out" | head -n 1 | tr -d '\r')
		if [ "$status" != 0 ] && [ -n "$try" ]; then
			gh_warn "$a was not executed on this runner (exit $status: $(echo "$first" | cut -c1-120)); its header and build info were checked, its --version was not"
			continue
		fi
		[ "$status" = 0 ] || { bad "$a --version exited $status: $first"; continue; }
		# vsp prints: vsp version <Version> (commit: <Commit>, built: <BuildDate>)
		case $first in
		"vsp version $tag (commit: $sha, built: "*) ok "$a${launcher:+ via $launcher}: $first" ;;
		*) bad "$a --version printed '$first', want 'vsp version $tag (commit: $sha, ...'" ;;
		esac
	done
	[ "$fail" = 0 ] || die "run $tag: FAILED"
}

# ---------------------------------------------------------------------- notes
# Headings inside a code fence are text, not section boundaries.
# The README's "What's New" carries hand-written sections per release, headed
# "### vX.Y.Z — ...". Every such section for TAG is taken, in order, up to the
# next heading of the same or higher level that is not for TAG. Without one,
# git-cliff renders the commits since the previous tag.
readme_section() {
	awk -v tag="$1" '
	function is_tag_heading(s) { return index(s, "### " tag) == 1 && (length(s) == length("### " tag) || substr(s, length("### " tag) + 1, 1) == " ") }
	# A fence closes only on its own character, at least as long, with nothing after it (CommonMark).
	function run(s,   c, n) { c = substr(s, 1, 1); n = 0; while (substr(s, n + 1, 1) == c) n++; return n }
	!fence && /^(```|~~~)/ { fence = 1; fc = substr($0, 1, 1); fl = run($0); if (taking) print; next }
	fence && substr($0, 1, 1) == fc && run($0) >= fl && substr($0, run($0) + 1) ~ /^[ \t]*$/ { fence = 0; if (taking) print; next }
	fence { if (taking) print; next }
	/^## / { inside = ($0 ~ /^## What.s New/); taking = 0; next }
	!inside { next }
	/^### / { taking = is_tag_heading($0) }
	taking { print }
	' README.md
}

cmd_notes() {
	local tag=$1 out=${2:-RELEASE_NOTES.md}
	check_tag "$tag"
	local repo=${GITHUB_REPOSITORY:-oisee/vibing-steampunk} body prev source
	body=$(readme_section "$tag")
	if [ -n "$(echo "$body" | tr -d '[:space:]')" ]; then
		source="README.md \"What's New\" $tag"
		# README links are relative to the repository; in a release they would
		# resolve against /releases/. Point them at the file as of the tag.
		body=$(echo "$body" | perl -pe 's{\]\((?!https?://|#|mailto:)(?:\./)?([^)\s]+)\)}{](https://github.com/'"$repo"'/blob/'"$tag"'/$1)}g')
	else
		command -v git-cliff >/dev/null 2>&1 || die "README has no '### $tag' section under What's New, and git-cliff is not installed for the fallback"
		prev=$(git describe --tags --abbrev=0 --match 'v[0-9]*' "$tag^" 2>/dev/null || true)
		source="git-cliff ${prev:+$prev..}$tag"
		body=$(git-cliff ${prev:+"$prev..$tag"} --strip all)
		[ -n "$(echo "$body" | tr -d '[:space:]')" ] ||
			die "no notes for $tag: README.md has no '### $tag' section under What's New, and git-cliff found no conventional commits in ${prev:+$prev..}$tag"
		gh_warn "README.md has no '### $tag' section under What's New; the notes are $source"
	fi
	{
		echo "$body"
		echo
		echo "## Downloads"
		echo
		echo "| Platform | Architecture | File |"
		echo "|----------|--------------|------|"
		echo "| Linux | x64 | vsp-linux-amd64 |"
		echo "| Linux | ARM64 | vsp-linux-arm64 |"
		echo "| macOS | x64 | vsp-darwin-amd64 |"
		echo "| macOS | Apple Silicon | vsp-darwin-arm64 |"
		echo "| Windows | x64 | vsp-windows-amd64.exe |"
		echo "| Windows | ARM64 | vsp-windows-arm64.exe |"
		echo
		echo "Checksums: \`checksums.txt\`. Or run \`vsp update\` from an older version on one of these platforms; elsewhere: \`go install github.com/oisee/vibing-steampunk/cmd/vsp@latest\`."
		echo "\`LICENSE\` and \`NOTICE\` travel with the binaries (Apache-2.0 components are embedded)."
		echo
		echo "**Changelog:** https://github.com/$repo/blob/$tag/CHANGELOG.md"
	} > "$out"
	echo "notes for $tag from $source -> $out"
}

# -------------------------------------------------------------------- compare
cmd_compare() {
	local dist=$1 dl=$2 a
	local want got
	want=$(cd "$dist" && ls -1A | LC_ALL=C sort)
	got=$(cd "$dl" && ls -1A | LC_ALL=C sort)
	[ "$want" = "$got" ] || { bad "the release's assets are not the built set:"; diff <(echo "$want") <(echo "$got") >&2 || true; }
	for a in $want; do
		[ -f "$dl/$a" ] || continue
		if cmp -s "$dist/$a" "$dl/$a"; then ok "$a downloaded byte-identical"; else bad "$a as served differs from the build"; fi
	done
	(cd "$dl" && while IFS= read -r line; do
		a=${line##* }; a=${a#\*}
		[ "${line%% *}" = "$(sha256_of "$a")" ] || { echo "FAIL: downloaded $a does not match the downloaded checksums.txt" >&2; exit 1; }
	done < checksums.txt) || fail=1
	[ "$fail" = 0 ] || die "compare: FAILED"
	echo "compare: the release serves exactly what was built and verified"
}

# ------------------------------------------------------------ publish guards
# The build jobs check one commit; the publish job must release that commit and
# those files, at the moment it acts. Each guard re-reads GitHub, not the runner.

cmd_tag_at() { # the tag on origin, peeled to its commit, is still SHA
	local tag=$1 sha=$2 remote
	check_tag "$tag"
	remote=$(git ls-remote origin "refs/tags/$tag" "refs/tags/$tag^{}" | awk '
		$2 ~ /\^\{\}$/ { peeled = $1 } { plain = $1 } END { print (peeled != "" ? peeled : plain) }')
	[ -n "$remote" ] || die "tag $tag is gone from origin"
	[ "$remote" = "$sha" ] || die "tag $tag moved on origin: it now points at $remote, the verified build is $sha"
	ok "tag $tag on origin is still $sha"
}

cmd_digests() { # the release (draft or not) holds exactly DIST, by GitHub's own sha256
	local tag=$1 dist=$2 repo=${GITHUB_REPOSITORY:-oisee/vibing-steampunk} id served a want got
	check_tag "$tag"
	id=$(gh release view "$tag" --repo "$repo" --json databaseId --jq .databaseId) || die "no release for $tag"
	served=$(gh api "repos/$repo/releases/$id" --jq '.assets[] | "\(.name) \(.digest // "none")"' | LC_ALL=C sort)
	want=$(cd "$dist" && ls -1A | LC_ALL=C sort)
	got=$(echo "$served" | cut -d' ' -f1)
	[ "$want" = "$got" ] || { bad "release $tag assets are not the built set:"; diff <(echo "$want") <(echo "$got") >&2 || true; }
	for a in $want; do
		[ -f "$dist/$a" ] || continue
		got=$(echo "$served" | awk -v n="$a" '$1 == n { print $2 }')
		[ "$got" = "sha256:$(sha256_of "$dist/$a")" ] || bad "$a on the release has digest ${got:-missing}, the build is sha256:$(sha256_of "$dist/$a")"
	done
	[ "$fail" = 0 ] || die "digests $tag: FAILED"
	ok "release $tag holds exactly the built assets (GitHub sha256 digests)"
}

cmd_latest() { # prints true when TAG is final and above every published final release
	local tag=$1 repo=${GITHUB_REPOSITORY:-oisee/vibing-steampunk} published top
	check_tag "$tag"
	case $tag in *-*) echo false; return ;; esac
	# Every published release, all pages; a failed listing is an error, never "latest".
	published=$(gh api --paginate "repos/$repo/releases?per_page=100" --jq '.[] | select(.draft | not) | select(.prerelease | not) | .tag_name') ||
		die "latest $tag: cannot list the published releases"
	top=$(printf '%s\n%s\n' "$published" "$tag" |
		grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sed 's/^v//' | sort -t. -k1,1n -k2,2n -k3,3n | tail -n 1)
	if [ "v$top" = "$tag" ]; then echo true; else echo false; fi
}

# The one publish sequence, for CI and the manual fallback alike. The draft
# must already exist and have passed `compare`.
cmd_publish() {
	local tag=$1 sha=$2 dist=$3 repo=${GITHUB_REPOSITORY:-oisee/vibing-steampunk} latest
	"$0" tag-at "$tag" "$sha"
	"$0" digests "$tag" "$dist"
	latest=$("$0" latest "$tag")
	gh release edit "$tag" --repo "$repo" --draft=false --latest="$latest"
	if ! { "$0" tag-at "$tag" "$sha" && "$0" digests "$tag" "$dist"; }; then
		gh release edit "$tag" --repo "$repo" --draft=true || true
		die "publish $tag: the tag or an asset changed while publishing; the release is a draft again"
	fi
	gh release view "$tag" --repo "$repo" --json url,isDraft,isPrerelease,assets \
		--jq '"\(.url) draft=\(.isDraft) prerelease=\(.isPrerelease) assets=\(.assets | length) latest='"$latest"'"'
}

[ $# -ge 1 ] || die "usage: release.sh on-branch|build|verify|run|notes|compare|tag-at|digests|latest|publish ..."
sub=$1; shift
case $sub in
on-branch) [ $# -eq 1 ] || die "usage: release.sh on-branch TAG"; cmd_on_branch "$@" ;;
build) [ $# -ge 1 ] || die "usage: release.sh build TAG [DIST]"; cmd_build "$@" ;;
verify) [ $# -ge 1 ] || die "usage: release.sh verify TAG [DIST]"; cmd_verify "$@" ;;
run) [ $# -ge 3 ] || die "usage: release.sh run TAG DIST SPEC..."; cmd_run "$@" ;;
notes) [ $# -ge 1 ] || die "usage: release.sh notes TAG [OUT]"; cmd_notes "$@" ;;
compare) [ $# -eq 2 ] || die "usage: release.sh compare DIST DOWNLOADED"; cmd_compare "$@" ;;
tag-at) [ $# -eq 2 ] || die "usage: release.sh tag-at TAG SHA"; cmd_tag_at "$@" ;;
digests) [ $# -eq 2 ] || die "usage: release.sh digests TAG DIST"; cmd_digests "$@" ;;
latest) [ $# -eq 1 ] || die "usage: release.sh latest TAG"; cmd_latest "$@" ;;
publish) [ $# -eq 3 ] || die "usage: release.sh publish TAG SHA DIST"; cmd_publish "$@" ;;
*) die "unknown subcommand $sub" ;;
esac
