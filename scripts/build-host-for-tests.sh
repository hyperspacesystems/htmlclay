#!/bin/bash
# Builds the Go host for the Mac app's tests into macos/.build/host-for-tests/htmlclay.
# Step 6.7's HostProcess tests reuse it.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
out="$root/macos/.build/host-for-tests"
mkdir -p "$out"
cd "$root"
go build -o "$out/htmlclay" ./cmd/htmlclay
go build -o "$out/stubservice" ./tools/stubservice
echo "$out/htmlclay"
