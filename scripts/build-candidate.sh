#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD="$HOME/scratch/htmlclay-candidate-cache"
ID="com.htmlclay.HTMLClayCandidate"
USAGE="usage: scripts/build-candidate.sh --unsigned --output <dir>"
UNSIGNED=""
OUT=""
while [ $# -gt 0 ]; do
  case "$1" in
    --unsigned) UNSIGNED=1; shift ;;
    --output)
      [ $# -ge 2 ] || { echo "$USAGE" >&2; exit 1; }
      OUT="$2"; shift 2 ;;
    *) echo "$USAGE" >&2; exit 1 ;;
  esac
done
[ -n "$UNSIGNED" ] || { echo "BUILD FAILED: signing comes in milestone 7 (pass --unsigned)" >&2; exit 1; }
[ -n "$OUT" ] || { echo "$USAGE" >&2; exit 1; }
case "$OUT" in
  /Applications|/Applications/*)
    echo "BUILD FAILED: the candidate must not be built into /Applications ($OUT)" >&2
    exit 1 ;;
esac
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd -P)"
case "$OUT" in
  /Applications|/Applications/*)
    echo "BUILD FAILED: the candidate must not be built into /Applications ($OUT)" >&2
    exit 1 ;;
esac
INSTALLED="/Applications/HTMLClay.app"
if [ "$OUT" = "$INSTALLED" ] || [ "$OUT" = "$(dirname "$INSTALLED")" ]; then
  echo "BUILD FAILED: the candidate must not replace the installed HTML Clay ($OUT)" >&2
  exit 1
fi

xcrun swift build -c release --package-path "$ROOT/macos" --scratch-path "$BUILD" --product HTMLClay
BIN="$(swift build -c release --package-path "$ROOT/macos" --scratch-path "$BUILD" --show-bin-path)/HTMLClay"

STAGE="$(mktemp -d)/HTML Clay Candidate.app"
mkdir -p "$STAGE/Contents/MacOS" "$STAGE/Contents/Resources"
cp "$BIN" "$STAGE/Contents/MacOS/htmlclay"
(cd "$ROOT" && go build -o "$STAGE/Contents/MacOS/htmlclay-host" ./cmd/htmlclay)
cp "$ROOT/macos/Resources/Candidate/Info.plist" "$STAGE/Contents/Info.plist"
find "$ROOT/macos/Resources" -type f ! -name Info.plist ! -name 'dashboard-preview*' ! -path '*/Candidate/*' -exec cp {} "$STAGE/Contents/Resources/" \;
plutil -lint "$STAGE/Contents/Info.plist" >/dev/null
for key in CFBundleDocumentTypes CFBundleURLTypes UTExportedTypeDeclarations; do
  if /usr/libexec/PlistBuddy -c "Print :$key" "$STAGE/Contents/Info.plist" >/dev/null 2>&1; then
    echo "BUILD FAILED: the candidate must not claim file types or URL schemes ($key)" >&2
    exit 1
  fi
done
[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$STAGE/Contents/Info.plist")" = "$ID" ] || { echo "BUILD FAILED: wrong bundle id" >&2; exit 1; }

codesign --force --sign - "$STAGE"
codesign --verify --strict "$STAGE"

APP="$OUT/HTML Clay Candidate.app"
if [ -d "$APP" ] && [ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$APP/Contents/Info.plist" 2>/dev/null)" != "$ID" ]; then
  echo "BUILD FAILED: $APP is not the candidate" >&2
  exit 1
fi
if [ -d "$APP" ]; then
  mkdir -p "$HOME/scratch/htmlclay-candidate-old"
  mv "$APP" "$HOME/scratch/htmlclay-candidate-old/$(date +%Y%m%d-%H%M%S).app"
fi
mv "$STAGE" "$APP"
echo "CANDIDATE OK $APP"
