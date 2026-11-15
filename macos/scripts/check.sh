#!/bin/bash
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD="${PREVIEW_BUILD_DIR:-$HOME/scratch/htmlclay-preview-cache}"
fail=0

if /usr/bin/grep -rn '"' "$ROOT/Sources/PreviewUI/Screens" "$ROOT/Sources/PreviewUI/Components" 2>/dev/null; then
  echo "CHECK FAILED: a view file holds a string literal. Text goes in Strings.swift, icon names in Symbols.swift."
  fail=1
fi
if /usr/bin/grep -rnE 'ObservableObject|@Published|@StateObject|@ObservedObject|@EnvironmentObject|NavigationSplitView|NavigationStack|MenuBarExtra|SettingsLink|import Combine' "$ROOT/Sources"; then
  echo "CHECK FAILED: a banned API is used. See docs/RULES.md."
  fail=1
fi
node "$ROOT/scripts/gen-tokens.mjs" "$ROOT/Design/kiln-styles.css" "$ROOT/Sources/PreviewUI/KilnPalette.generated.swift" --check || fail=1
xcrun swift build --package-path "$ROOT" --scratch-path "$BUILD" 2>&1 | tail -25
[ "${PIPESTATUS[0]}" -eq 0 ] || fail=1
"$(dirname "$0")/../../scripts/build-host-for-tests.sh" >/dev/null || fail=1
export HTMLCLAY_REQUIRE_LIVE_HOST=1
SAMPLE_ONLY_PERSONAS="syncedWithTeams localUser"
export SAMPLE_ONLY_PERSONAS
xcrun swift test --package-path "$ROOT" --scratch-path "$BUILD" 2>&1 | /usr/bin/grep -E 'error:|failed|Executed|Fatal error' | tail -15
[ "${PIPESTATUS[0]}" -eq 0 ] || fail=1

if [ "$(/usr/bin/grep -rn 'SMAppServiceLoginItem()' "$ROOT/Sources" | /usr/bin/grep -vc 'LoginItems.swift')" != "0" ]; then echo "CHECK FAILED: SMAppServiceLoginItem constructed outside LoginItems.make"; fail=1; fi

if [ "$fail" -eq 0 ]; then echo "CHECK OK"; else echo "CHECK FAILED"; fi
exit "$fail"
