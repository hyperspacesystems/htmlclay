#!/bin/bash
# Copies the app core's golden snapshots to the Swift tests, so the Mac app decodes
# exactly what the Go host builds. internal/appcore's TestShellFixturesMatch fails
# until this has been run after a snapshot changes.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
src="$root/internal/appcore/testdata/v1"
dst="$root/macos/Tests/PreviewCoreTests/Fixtures/v1"
mkdir -p "$dst"
rm -f "$dst"/*.snapshot.json
cp "$src"/*.snapshot.json "$dst"/
cp "$src"/actions.json "$dst"/
cp "$src"/menu.json "$dst"/
cp "$root/cmd/htmlclay/testdata/shell/events.v1.ndjson" "$dst"/
echo "Copied $(ls "$src"/*.snapshot.json | wc -l | tr -d ' ') snapshots, actions.json, menu.json and events.v1.ndjson to ${dst#"$root"/}"
