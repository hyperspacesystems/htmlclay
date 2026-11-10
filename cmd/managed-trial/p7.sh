#!/usr/bin/env bash
# P7: the managed trial host contains no cgo, on every target platform.
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$repo_root"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

targets="darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64"
for target in $targets; do
	os=${target%%/*}
	arch=${target##*/}
	bin="$tmp/managed-trial-$os-$arch"
	if [ "$os" = "windows" ]; then
		bin="$bin.exe"
	fi
	if ! CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -tags managedtrial -trimpath -o "$bin" ./cmd/managed-trial >"$tmp/build-$os-$arch.log" 2>&1; then
		echo "P7 $target: BUILD-FAILED"
		cat "$tmp/build-$os-$arch.log"
		exit 1
	fi
	if ! go version -m "$bin" >"$tmp/version-$os-$arch.txt" 2>&1; then
		echo "P7 $target: VERSION-FAILED"
		cat "$tmp/version-$os-$arch.txt"
		exit 1
	fi
	if ! grep -Eq '^[[:space:]]+build[[:space:]]+CGO_ENABLED=0$' "$tmp/version-$os-$arch.txt"; then
		echo "P7 $target: BINARY-HAS-CGO"
		cat "$tmp/version-$os-$arch.txt"
		exit 1
	fi
	if ! CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go list -deps -tags managedtrial ./cmd/managed-trial >"$tmp/deps-$os-$arch.txt" 2>&1; then
		echo "P7 $target: DEPS-FAILED"
		cat "$tmp/deps-$os-$arch.txt"
		exit 1
	fi
	if grep -qx 'runtime/cgo' "$tmp/deps-$os-$arch.txt"; then
		echo "P7 $target: CGO-DEPS"
		exit 1
	fi
	echo "P7 $os/$arch: BUILD-OK BINARY-NO-CGO NO-CGO-DEPS"
done

import_c_rc=0
rg -l '^import "C"|^\s+"C"$' internal/managedtrial cmd/managed-trial >"$tmp/import-c.txt" 2>&1 || import_c_rc=$?
if [ "$import_c_rc" -eq 0 ]; then
	echo "P7 import-C: found"
	cat "$tmp/import-c.txt"
	exit 1
fi
if [ "$import_c_rc" -gt 1 ]; then
	echo "P7 import-C: search failed"
	cat "$tmp/import-c.txt"
	exit 1
fi
echo "P7 import-C: none"

echo "P7 PASS"
