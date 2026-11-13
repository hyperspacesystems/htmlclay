#!/usr/bin/env bash
# Fetches the Hyperclay Local commit the served host's goldens were recorded from into
# <dest> and installs its dependencies, so `MANAGED_TRIAL_LOCAL_SOURCE=<dest>` runs the
# golden tests instead of skipping them. The favicon answers carry the file's own
# Last-Modified and weak ETag, so the three favicons are stamped with the times, to
# the millisecond, the static golden recorded. Re-running with the same commit only re-stamps.
#
# Re-recording the goldens against a newer Local: bump COMMIT, run this script, then
# node internal/served/testdata/http-golden.cjs <dest> internal/served/testdata/<case>-golden <out.json>
# (live-golden/gen.cjs for the live case) and review the diff.
set -euo pipefail

COMMIT=aff481b2d8db9332556d9ea716c1247afe45e643
REPO=https://github.com/hyperspacesystems/hyperclay-local.git

dest=${1:?usage: served-golden-local.sh <dest>}
marker="$dest/.served-golden-commit"

if [ ! -f "$marker" ] || [ "$(cat "$marker")" != "$COMMIT" ]; then
	if [ -e "$dest" ] && [ -n "$(ls -A "$dest")" ]; then
		echo "served-golden-local: $dest holds something other than Local $COMMIT; remove it first" >&2
		exit 1
	fi
	mkdir -p "$dest"
	git -C "$dest" init -q
	git -C "$dest" fetch -q --depth 1 "$REPO" "$COMMIT"
	git -C "$dest" -c advice.detachedHead=false checkout -q FETCH_HEAD
	(cd "$dest" && npm ci --no-audit --no-fund --ignore-scripts)
	echo "$COMMIT" >"$marker"
fi

touch -d 2026-09-25T14:16:37.200Z "$dest/assets/favicon.ico"
touch -d 2026-09-25T14:16:37.095Z "$dest/assets/favicon.svg"
touch -d 2026-09-25T14:16:37.112Z "$dest/assets/favicon.png"
echo "served-golden-local: Local ${COMMIT:0:7} at $dest"
