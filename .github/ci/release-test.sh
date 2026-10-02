#!/usr/bin/env bash
# Offline checks for release.sh's two branch-sensitive decisions:
#
#   on-branch  a tag is released only from origin/main, or from exactly one
#              origin/release/X.Y whose X.Y is the tag's major.minor (LTS);
#   latest     an LTS patch published after a higher minor never becomes latest.
#
#   .github/ci/release-test.sh
#
# Everything happens in a throwaway repository with local refs standing in for
# origin's (refs/remotes/origin/*) and a fake `gh` on PATH, so nothing is
# fetched, pushed or published.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
rel="$here/release.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.invalid
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.invalid
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1

fails=0
pass() { echo "release-test: ok   $*"; }
flunk() { echo "release-test: FAIL $*"; fails=$((fails + 1)); }

# ---------------------------------------------------------------- on-branch
repo="$tmp/repo"
git init -q -b main "$repo"
cd "$repo"
commit() { git commit -q --allow-empty -m "$1"; git rev-parse HEAD; }
base=$(commit "v2.59.1")
git tag v2.59.1 "$base"
commit "main moves on" >/dev/null
main_tip=$(commit "v2.60.0")
git tag v2.60.0 "$main_tip"
git update-ref refs/remotes/origin/main "$main_tip"

git switch -q -c release/2.59 "$base"
lts=$(commit "fix (cherry picked from commit 0000000)")
git update-ref refs/remotes/origin/release/2.59 "$lts"

git switch -q -c topic "$base"
topic=$(commit "a random branch")
git update-ref refs/remotes/origin/topic "$topic"

# A branch under release/ that is not release/X.Y is not a release branch: it
# holds the LTS commit too, and must neither count nor be listed as checked.
git update-ref refs/remotes/origin/release/2.58-not-a-release "$lts"
git switch -q main

# expect <accept|refuse> <name> <tag> <commit> [stderr pattern]
expect() {
	local want=$1 name=$2 tag=$3 sha=$4 pat=${5:-} got out
	git tag -f "$tag" "$sha" >/dev/null
	if out=$("$rel" on-branch "$tag" 2>&1); then got=accept; else got=refuse; fi
	git tag -d "$tag" >/dev/null
	if [ "$got" != "$want" ]; then flunk "$name: $got, want $want: $out"; return; fi
	if [ -n "$pat" ] && ! echo "$out" | grep -q -- "$pat"; then flunk "$name: message lacks '$pat': $out"; return; fi
	pass "$name ($got): $out"
}

expect accept "LTS tag on release/2.59" v2.59.2 "$lts" "origin/release/2.59 (LTS)"
expect accept "tag on main" v2.60.1 "$main_tip" "is on origin/main"
expect accept "older tag already on main" v2.59.9 "$base" "is on origin/main"
expect refuse "v2.60.x on release/2.59" v2.60.9 "$lts" "never on release/2.59"
expect refuse "v2.59.x on a random branch" v2.59.3 "$topic" "checked: origin/main origin/release/2.59"
expect refuse "v3.0.0 on release/2.59" v3.0.0 "$lts" "belongs on main or on release/3.0"
expect refuse "malformed tag" v2.59 "$lts" "not a release tag"

git update-ref refs/remotes/origin/release/2.60 "$lts"
expect refuse "LTS commit on two release branches" v2.59.2 "$lts" "more than one release branch"
git update-ref -d refs/remotes/origin/release/2.60

git update-ref -d refs/remotes/origin/main
expect refuse "origin/main not fetched" v2.60.1 "$main_tip" "origin/main is not fetched"
cd "$here"

# ------------------------------------------------------------------- latest
# A fake gh that answers the releases listing with $FAKE_RELEASES: the tag
# names of the published final releases, as release.sh's --jq filter leaves them.
mkdir -p "$tmp/bin"
cat > "$tmp/bin/gh" <<'EOF'
#!/usr/bin/env bash
case "$*" in
api\ --paginate\ repos/*/releases*) printf '%s\n' $FAKE_RELEASES ;;
*) echo "fake gh: unexpected call: $*" >&2; exit 2 ;;
esac
EOF
chmod +x "$tmp/bin/gh"

# latest_is <want> <name> <tag> <published final releases...>
latest_is() {
	local want=$1 name=$2 tag=$3 got; shift 3
	got=$(PATH="$tmp/bin:$PATH" FAKE_RELEASES="$*" "$rel" latest "$tag" 2>&1) || got="error: $got"
	if [ "$got" = "$want" ]; then pass "latest $name: $tag -> $got"; else flunk "latest $name: $tag -> $got, want $want"; fi
}

latest_is false "LTS after a higher minor" v2.59.2 v2.60.0 v2.59.1 v2.59.0
latest_is true "LTS while it is the top line" v2.59.2 v2.59.1 v2.59.0
latest_is true "next minor" v2.61.0 v2.60.0 v2.59.2
latest_is false "LTS after a higher major" v2.59.3 v3.0.0 v2.60.0
latest_is false "prerelease" v2.60.1-rc.1 v2.60.0

# -------------------------------------------------- publish, one after another
# release.yml runs publish jobs one at a time across tags (concurrency group
# release-publish). Two drafts, v2.60.0 and the LTS v2.59.2, are published in
# turn, in both orders, through the real `release.sh publish` (tag-at against a
# local "origin", digests and latest against a stateful fake gh). Whatever the
# order, v2.59.2 is never left as Latest, and it is not marked latest at all
# once v2.60.0 is public.
cat > "$tmp/bin/gh" <<'EOF'
#!/usr/bin/env bash
# State: $FAKE_GH/releases holds "<tag> draft|published"; edits go to $FAKE_GH/log.
set -euo pipefail
st=$FAKE_GH/releases
case "$1 $2" in
"api --paginate") awk '$2 == "published" { print $1 }' "$st" ;;
"release view")
	grep -q "^$3 " "$st" || exit 1
	case "$*" in *databaseId*) echo "id-$3" ;; *) echo "https://example.invalid/$3 draft=false" ;; esac ;;
"api repos/"*)
	for f in "$FAKE_DIST"/*; do echo "$(basename "$f") sha256:$(sha256sum "$f" | cut -d' ' -f1)"; done ;;
"release edit")
	tag=$3 draft= latest=-
	for a in "$@"; do case $a in --draft=*) draft=${a#--draft=} ;; --latest=*) latest=${a#--latest=} ;; esac; done
	echo "$tag draft=$draft latest=$latest" >> "$FAKE_GH/log"
	if [ "$draft" = false ]; then s=published; else s=draft; fi
	awk -v t="$tag" -v s="$s" '$1 == t { $2 = s } { print }' "$st" > "$st.new" && mv "$st.new" "$st" ;;
*) echo "fake gh: unexpected call: $*" >&2; exit 2 ;;
esac
EOF
chmod +x "$tmp/bin/gh"
export FAKE_GH="$tmp/gh" FAKE_DIST="$tmp/dist"
mkdir -p "$FAKE_GH" "$FAKE_DIST"
echo placeholder > "$FAKE_DIST/vsp-linux-amd64"
cd "$repo"
git remote add origin "$repo" # tag-at reads `git ls-remote origin`: this repository's own tags
git tag v2.59.2 "$lts"

# publish_in_order <name> <tag> <tag>: both start as drafts next to a published v2.59.1.
publish_in_order() {
	local name=$1 t want_lts out; shift
	printf '%s\n' "v2.59.1 published" "v2.60.0 draft" "v2.59.2 draft" > "$FAKE_GH/releases"
	: > "$FAKE_GH/log"
	for t in "$@"; do
		out=$(PATH="$tmp/bin:$PATH" "$rel" publish "$t" "$(git rev-parse "$t^{commit}")" "$FAKE_DIST" 2>&1) ||
			{ flunk "publish $name: $t failed: $out"; return; }
	done
	# v2.59.2 may be latest only if it went first, while v2.60.0 was a draft.
	if [ "$1" = v2.59.2 ]; then want_lts=true; else want_lts=false; fi
	grep -qx "v2.59.2 draft=false latest=$want_lts" "$FAKE_GH/log" ||
		{ flunk "publish $name: v2.59.2 not published with latest=$want_lts: $(tr '\n' ';' < "$FAKE_GH/log")"; return; }
	[ "$(grep 'latest=true' "$FAKE_GH/log" | tail -n 1 | cut -d' ' -f1)" = v2.60.0 ] ||
		{ flunk "publish $name: the last release marked latest is not v2.60.0: $(tr '\n' ';' < "$FAKE_GH/log")"; return; }
	pass "publish $name: $(tr '\n' ';' < "$FAKE_GH/log")"
}
publish_in_order "v2.60.0 then LTS v2.59.2" v2.60.0 v2.59.2
publish_in_order "LTS v2.59.2 then v2.60.0" v2.59.2 v2.60.0
git tag -d v2.59.2 >/dev/null
cd "$here"

cat > "$tmp/bin/gh" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
if PATH="$tmp/bin:$PATH" "$rel" latest v2.59.2 >/dev/null 2>&1; then flunk "latest with a failing gh did not fail"
else pass "latest with a failing gh fails (never 'latest')"; fi

if [ "$fails" != 0 ]; then echo "release-test: $fails failed"; exit 1; fi
echo "release-test: all checks passed"
