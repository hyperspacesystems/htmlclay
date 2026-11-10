#!/usr/bin/env bash
# Proof that the managed trial host cannot reach a release build: no untagged
# file changed, every trial file is tagged, the release binary does not depend
# on the trial package, and the release binary is unchanged relative to the
# base commit.
set -euo pipefail

base=${1:-}
if [ -z "$base" ]; then
	echo "usage: release-exclusion.sh <base-commit>" >&2
	exit 2
fi

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$repo_root"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

status=0

# 1. No untagged file may change relative to the base commit.
{
	git diff --name-only "$base" -- .
	git ls-files --others --exclude-standard
} | sort -u >"$tmp/changed.txt"
: >"$tmp/untagged.txt"
while IFS= read -r file; do
	if [ -z "$file" ]; then
		continue
	fi
	case "$file" in
	internal/managedtrial/* | cmd/managed-trial/* | *_managedtrial.go | *_managedtrial_test.go) continue ;;
	esac
	echo "$file" >>"$tmp/untagged.txt"
done <"$tmp/changed.txt"
if [ -s "$tmp/untagged.txt" ]; then
	echo "untagged-changes:"
	sed 's/^/  /' "$tmp/untagged.txt"
	status=1
else
	echo "untagged-changes: none"
fi

# 2. Every trial file starts with the managedtrial build tag.
tag_failed=0
: >"$tmp/tag-lines.txt"
while IFS= read -r file; do
	first=$(head -n 1 "$file")
	if [ "$first" != "//go:build managedtrial" ]; then
		echo "  $file: ${first}" >>"$tmp/tag-lines.txt"
		tag_failed=1
	fi
done < <(find internal/managedtrial cmd/managed-trial -type f -name '*.go' | sort)
if [ "$tag_failed" -ne 0 ]; then
	echo "tag-lines: bad"
	cat "$tmp/tag-lines.txt"
	status=1
else
	echo "tag-lines: ok"
fi

# 3. The release command must not reach the trial package.
if ! go list -deps ./cmd/htmlclay >"$tmp/release-deps.txt" 2>"$tmp/release-deps.err"; then
	echo "release-deps: go list failed"
	cat "$tmp/release-deps.err"
	status=1
elif grep -q 'managedtrial' "$tmp/release-deps.txt"; then
	echo "release-deps: the release build reaches managedtrial"
	grep 'managedtrial' "$tmp/release-deps.txt" | sed 's/^/  /'
	status=1
else
	echo "release-deps: clean"
fi

# 4. The release binary must be byte-identical to the base commit's.
hash_file() {
	if command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		sha256sum "$1" | awk '{print $1}'
	fi
}

if ! go build -trimpath -buildvcs=false -o "$tmp/head-htmlclay" ./cmd/htmlclay; then
	echo "release-hash: HEAD build failed"
	status=1
else
	mkdir -p "$tmp/base"
	git archive "$base" | tar -x -C "$tmp/base"
	if ! (cd "$tmp/base" && go build -trimpath -buildvcs=false -o "$tmp/base-htmlclay-1" ./cmd/htmlclay); then
		echo "release-hash: base build failed"
		status=1
	elif ! (cd "$tmp/base" && go build -trimpath -buildvcs=false -o "$tmp/base-htmlclay-2" ./cmd/htmlclay); then
		echo "release-hash: base rebuild failed"
		status=1
	else
		head_hash=$(hash_file "$tmp/head-htmlclay")
		base_hash_1=$(hash_file "$tmp/base-htmlclay-1")
		base_hash_2=$(hash_file "$tmp/base-htmlclay-2")
		if [ "$base_hash_1" != "$base_hash_2" ]; then
			echo "release-hash: not reproducible on this machine, relying on untagged-changes and release-deps"
		elif [ "$head_hash" = "$base_hash_1" ]; then
			echo "release-hash: equal $head_hash"
		else
			echo "release-hash: HEAD $head_hash differs from base $base_hash_1"
			status=1
		fi
	fi
fi

if [ "$status" -ne 0 ]; then
	echo "RELEASE-EXCLUSION FAIL"
	exit 1
fi
echo "RELEASE-EXCLUSION PASS"
