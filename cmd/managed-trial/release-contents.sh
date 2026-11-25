#!/usr/bin/env bash
# Proof that a build without tags gets the served host while the fault hooks
# stay out of it: the fault files are missing from an untagged build and
# present in a tagged one, and the untagged package vets.
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$repo_root"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

status=0
fault_files="faults.go control_faults.go"
trial_files="boot_trial.go rootguard_trial.go tailwind_node.go"

# contains reports whether the space-separated list names file.
contains() {
	case " $1 " in
	*" $2 "*) return 0 ;;
	esac
	return 1
}

if ! untagged=$(go list -f '{{join .GoFiles " "}}' ./internal/served); then
	echo "untagged-files: go list failed"
	status=1
else
	leaked=""
	for file in $fault_files $trial_files; do
		if contains "$untagged" "$file"; then
			leaked="$leaked $file"
		fi
	done
	if [ -n "$leaked" ]; then
		echo "untagged-files: faults in an untagged build:$leaked"
		status=1
	else
		echo "untagged-files: none"
	fi
fi

if ! tagged=$(go list -tags managedtrial -f '{{join .GoFiles " "}}' ./internal/served); then
	echo "tagged-files: go list failed"
	status=1
else
	missing=""
	for file in $fault_files $trial_files; do
		if ! contains "$tagged" "$file"; then
			missing="$missing $file"
		fi
	done
	if [ -n "$missing" ]; then
		echo "tagged-files: faults missing from a tagged build:$missing"
		status=1
	else
		echo "tagged-files: both"
	fi
fi

if ! embeds=$(go list -f '{{join .EmbedFiles " "}}' ./internal/served); then
	echo "untagged-embeds: go list failed"
	status=1
elif contains "$embeds" "tailwind-compile.cjs"; then
	echo "untagged-embeds: tailwind-compile.cjs is embedded in an untagged build"
	status=1
else
	echo "untagged-embeds: none"
fi

if CGO_ENABLED=0 go vet ./internal/served >"$tmp/vet.log" 2>&1; then
	echo "untagged-vet: ok"
else
	echo "untagged-vet: failed"
	sed 's/^/  /' "$tmp/vet.log"
	status=1
fi

if go list -deps ./cmd/htmlclay >"$tmp/release-deps.txt" 2>"$tmp/release-deps.err"; then
	if grep -qx 'github.com/panphora/htmlclay/internal/served' "$tmp/release-deps.txt"; then
		echo "release-imports: the release build reaches internal/served"
	else
		echo "release-imports: the release build does not reach internal/served"
	fi
else
	echo "release-imports: go list failed, not checked"
	sed 's/^/  /' "$tmp/release-deps.err"
fi

# 08-sync.md decision 8: only a synctest build may point sync at another server,
# so a release binary carries no HTMLCLAY_SYNC_SERVER and a synctest one does
# (which proves the search can find it).
if go build -o "$tmp/htmlclay-release" ./cmd/htmlclay && go build -tags synctest -o "$tmp/htmlclay-synctest" ./cmd/htmlclay; then
	if grep -q HTMLCLAY_SYNC_SERVER "$tmp/htmlclay-release"; then
		echo "sync-server: the release build reads HTMLCLAY_SYNC_SERVER"
		status=1
	elif ! grep -q HTMLCLAY_SYNC_SERVER "$tmp/htmlclay-synctest"; then
		echo "sync-server: the synctest build lacks HTMLCLAY_SYNC_SERVER, so the search proves nothing"
		status=1
	else
		echo "sync-server: synctest only"
	fi
else
	echo "sync-server: go build failed"
	status=1
fi

if [ "$status" -ne 0 ]; then
	echo "RELEASE-CONTENTS FAIL"
	exit 1
fi
echo "RELEASE-CONTENTS PASS"
