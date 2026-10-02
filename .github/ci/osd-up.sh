#!/usr/bin/env bash
# Download a pinned open-steamgate release binary, start it on throwaway
# state, and wait until it serves. Used by .github/workflows/osd-integration.yml
# and runnable locally:
#
#   .github/ci/osd-up.sh <workdir> [tag]      # tag defaults to .github/ci/$OSD_BINARY.version
#
# OSD_BINARY picks the target by binary name: osd (default, the Bun build,
# ADT facade included) or osgo (the Go build, first shipped in a vscode-v0.5.N
# tag; OData only, no ADT yet). osd takes its state from the environment
# (STG_PORT, STG_DB_PATH, XDG_DATA_HOME); osgo from -home and -port, where a
# fresh -home is a full reset.
#
# Prints KEY=VALUE lines for the caller on success (SAP_URL, OSD_PID, OSD_TAG,
# OSD_WORKDIR, and OSD_ADT: the HTTP status of HEAD /sap/bc/adt/core/discovery,
# 200 when the target speaks ADT); also appends them to $GITHUB_ENV when set.
# Exit codes: 0 ready, 3 asset unavailable (caller skips), 4 checksum refused
# (the binary is never made executable), anything else is a real failure.
#
# Pinning: .github/ci/osd.sha256 holds the expected sha256 of every pinned
# asset, one "<sha256>  <tag>/<asset>" line per binary and platform. The
# download must match that committed value AND the release's own .sha256; a
# binary swapped together with its release checksum is refused. A tag with no
# committed line (the nightly "latest" row, a dispatch override) runs only
# when the caller sets OSD_ALLOW_UNPINNED=1, and is reported OSD_PINNED=false.
#
# Test hooks (.github/ci/osd-up-test.sh): OSD_DL_DIR takes the asset and its
# .sha256 from a directory instead of the release; OSD_VERIFY_ONLY=1 stops
# after the checksum verdict.
#
# Isolation: the release binary unpacks its seed system into
# $XDG_DATA_HOME/open-steamgate and writes ADT source changes there as files,
# and keeps rows in STG_DB_PATH. A fresh STG_DB_PATH alone does not reset it.
# Every run gets its own XDG_DATA_HOME, HOME, database and process, so a run
# never sees (or touches) the developer's ~/.local/share/open-steamgate.
# GH_TOKEN and GITHUB_TOKEN are unset before the binary starts.
set -euo pipefail

work=${1:?usage: osd-up.sh <workdir> [tag]}
here=$(cd "$(dirname "$0")" && pwd)
binary=${OSD_BINARY:-osd}
tag=${2:-$(tr -d '[:space:]' < "$here/$binary.version" 2>/dev/null || true)}
if [ -z "$tag" ]; then
  echo "osd-up: no $binary tag pinned in .github/ci/$binary.version" >&2
  exit 3
fi
repo=${OSD_REPO:-oisee/open-steamgate}
port=${STG_PORT:-3030}
timeout_s=${OSD_READY_TIMEOUT:-600}
pins=${OSD_PINS:-$here/osd.sha256}

case "$(uname -s)/$(uname -m)" in
  Linux/x86_64)  asset=$binary-linux-x64 ;;
  Linux/aarch64) asset=$binary-linux-arm64 ;;
  Darwin/arm64)  asset=$binary-darwin-arm64 ;;
  *) echo "osd-up: no OSD asset for $(uname -s)/$(uname -m)" >&2; exit 3 ;;
esac

# The committed hash for this tag and asset, if any.
pinned_sha=$(awk -v k="$tag/$asset" '$2 == k { print $1 }' "$pins" 2>/dev/null || true)
if [ -n "$pinned_sha" ]; then
  pinned=true
elif [ "${OSD_ALLOW_UNPINNED:-0}" = 1 ]; then
  pinned=false
  echo "osd-up: $tag/$asset has no committed sha256; running UNPINNED (OSD_ALLOW_UNPINNED=1)" >&2
else
  echo "osd-up: $tag/$asset has no committed sha256 in ${pins#"$here"/}; refusing (set OSD_ALLOW_UNPINNED=1 to run it unpinned)" >&2
  exit 4
fi

mkdir -p "$work"/{dl,xdg,home,db,cwd}
work=$(cd "$work" && pwd)

if [ -n "${OSD_DL_DIR:-}" ]; then
  cp "$OSD_DL_DIR/$asset" "$OSD_DL_DIR/$asset.sha256" "$work/dl/" || exit 3
elif ! gh release download "$tag" -R "$repo" -p "$asset" -p "$asset.sha256" -D "$work/dl" --clobber; then
  echo "osd-up: $repo $tag has no downloadable $asset" >&2
  exit 3
fi
actual=$(sha256sum "$work/dl/$asset" | cut -d' ' -f1)
release_sha=$(cut -d' ' -f1 < "$work/dl/$asset.sha256")
if [ "$actual" != "$release_sha" ]; then
  echo "osd-up: $asset does not match its release .sha256 ($actual != $release_sha); refusing" >&2
  exit 4
fi
if [ "$pinned" = true ] && [ "$actual" != "$pinned_sha" ]; then
  echo "osd-up: $asset does not match the committed sha256 for $tag ($actual != $pinned_sha); refusing" >&2
  exit 4
fi
echo "osd-up: $asset sha256 $actual matches the release${pinned_sha:+ and the committed pin}" >&2
if [ "${OSD_VERIFY_ONLY:-0}" = 1 ]; then
  echo "OSD_PINNED=$pinned"
  exit 0
fi
bin="$work/$binary-vsp-ci"
cp "$work/dl/$asset" "$bin"
chmod +x "$bin"
unset GH_TOKEN GITHUB_TOKEN

# Never the developer's home: XDG_DATA_HOME and HOME both point into $work.
export XDG_DATA_HOME="$work/xdg" HOME="$work/home" STG_DB_PATH="$work/db/osd.sqlite" STG_PORT="$port"
if [ "$binary" = osd ]; then
  # Warm activation (dell): a content edit of an existing class or interface
  # activates in about 0.5 s instead of a 25-30 s runtime rebuild. Creates,
  # PROG edits and INTERFACES changes stay cold; the X-OSD-Generation response
  # header says which path an activation took and why. OSD_WARM=0 STG_DEV=0
  # in the caller's environment restores the all-cold behaviour.
  export OSD_WARM="${OSD_WARM:-0}" STG_DEV="${STG_DEV:-0}"
  warm="; OSD_WARM=$OSD_WARM STG_DEV=$STG_DEV"
  (cd "$work/cwd" && "$bin" doctor) > "$work/doctor.log" 2>&1 || true
fi
# osd starts with `up`; osgo with its home and port as flags.
if [ "$binary" = osd ]; then
  start=(up)
else
  start=(-home "$work/osgo-home" -port "$port")
fi
(cd "$work/cwd" && exec "$bin" "${start[@]}") > "$work/osd.log" 2>&1 &
pid=$!

url="http://localhost:$port"
# OSD is ready when its build stamp answers and the generation the source
# built is the one being served (system.serving == system.live). The stamp is
# OSD's own endpoint, not ADT's: a real system answers 404 there.
# OSGo is ready when GET /health says status=ready (about 0.55 s); it has no
# build stamp.
ready() {
  if [ "$binary" = osd ]; then
    body=$(curl -fsS -m 5 "$url/sap/bc/adt/core/http/build" 2>/dev/null) &&
      jq -e '.system.serving != null and .system.serving == .system.live' <<<"$body" >/dev/null
  else
    body=$(curl -fsS -m 5 "$url/health" 2>/dev/null) &&
      jq -e '.status == "ready"' <<<"$body" >/dev/null
  fi
}
deadline=$((SECONDS + timeout_s))
until ready; do
  if ! kill -0 "$pid" 2>/dev/null; then
    echo "osd-up: OSD exited before it was ready" >&2
    tail -40 "$work/osd.log" >&2
    exit 1
  fi
  if (( SECONDS > deadline )); then
    echo "osd-up: not ready after ${timeout_s}s" >&2
    tail -40 "$work/osd.log" >&2
    kill "$pid" 2>/dev/null || true
    exit 1
  fi
  sleep 3
done
echo "$body" > "$work/build.json"
# Does it speak ADT? This is the switch between the full suite and a smoke run.
adt=$(curl -s -o /dev/null -I -m 10 -w '%{http_code}' "$url/sap/bc/adt/core/discovery" || true)
echo "osd-up: $binary $tag ready on $url after ${SECONDS}s; HEAD core/discovery: $adt${warm:-}" >&2

out=$(printf 'SAP_URL=%s\nOSD_PID=%s\nOSD_TAG=%s\nOSD_WORKDIR=%s\nOSD_ADT=%s\nOSD_PINNED=%s\n' "$url" "$pid" "$binary-$tag" "$work" "$adt" "$pinned")
echo "$out"
if [ -n "${GITHUB_ENV:-}" ]; then
  echo "$out" >> "$GITHUB_ENV"
fi
